// Migration: Convert ACL documents from version 2 to version 3
//
// STATUS: OPTIONAL / not required for correctness. The stores read mixed
// version:2/version:3 documents indefinitely via the on-read V2ToV3 shim
// (aclstore.decodeCursor / pkg/acl/aclstore), and all new writes are V3. This
// migration exists ONLY to enable the eventual teardown of the V2 model: run it
// (or verify `db.acl.countDocuments({version: 2}) == 0`) before removing the
// on-read shim + deleting aclModelsV2*/aclconvert. Until then it is a no-op to
// skip. See roadmap 1c.
//
// Collection affected:
//   - acl: rewrites every { version: 2 } document to the canonical V3 shape.
//
// V2 stores access as a sub-document of booleans plus a `kubernetes.logon`
// flag; V3 stores `access` as an array of AccessTypeV3 capability strings.
// The mapping mirrors aclmodels.V2ToV3 (the v3 struct tags on the V2 model):
//
//   access.read   -> "ror:read"
//   access.create -> "ror:create"
//   access.update -> "ror:update"
//   access.delete -> "ror:delete"
//   access.owner  -> "ror:owner"
//   kubernetes.logon -> "kubernetes:logon"
//
// access.kuberneteslogon has no V3 equivalent and is intentionally dropped
// (matches V2ToV3, which only maps fields carrying a `v3` struct tag).
//
// After this runs and is verified in every environment, the on-read V2->V3
// conversion (aclstore.decodeCursor / pkg/acl/aclstore) can be removed
// (roadmap 1c) and the V2 model deleted (roadmap 1d / Phase 5).
//
// Usage:
//   mongosh 'mongodb://<user>:<pass>@<host>:<port>/nhn-ror?authSource=admin' --file 006_acl_v2_to_v3.js
//
// This migration is idempotent: the filter selects only { version: 2 }
// documents, which become { version: 3 } after conversion, so re-running is a
// no-op.

const db = db.getSiblingDB("nhn-ror");

print("=== ACL Migration: version 2 -> 3 ===\n");

const total = db.acl.countDocuments({ version: 2 });
print(`Found ${total} v2 ACL document(s) to migrate\n`);

if (total === 0) {
  print("Nothing to do.");
  quit(0);
}

// Map a V2 access sub-document (+ kubernetes sub-document) to the V3 access
// capability array, mirroring aclmodels.V2ToV3.
function toV3Access(access, kubernetes) {
  const out = [];
  if (access) {
    if (access.read) out.push("ror:read");
    if (access.create) out.push("ror:create");
    if (access.update) out.push("ror:update");
    if (access.delete) out.push("ror:delete");
    if (access.owner) out.push("ror:owner");
  }
  if (kubernetes && kubernetes.logon) out.push("kubernetes:logon");
  return out;
}

let migrated = 0;
let emptyAccess = 0;

db.acl.find({ version: 2 }).forEach((doc) => {
  const accessArray = toV3Access(doc.access, doc.kubernetes);
  if (accessArray.length === 0) {
    emptyAccess++;
    print(
      `  WARN: acl ${doc._id} (group=${doc.group}, scope=${doc.scope}, subject=${doc.subject}) has no mappable access; writing empty array`
    );
  }

  db.acl.updateOne(
    { _id: doc._id },
    {
      $set: { version: 3, access: accessArray },
      $unset: { kubernetes: "" },
    }
  );
  migrated++;
});

print(`\n--- Summary ---`);
print(`  Migrated: ${migrated}`);
print(`  Empty access (review these): ${emptyAccess}`);

const remaining = db.acl.countDocuments({ version: 2 });
if (remaining !== 0) {
  print(`ERROR: ${remaining} v2 document(s) still remain after migration.`);
  quit(1);
}
print(`  Remaining v2 documents: 0`);
print("\n=== Done ===");
