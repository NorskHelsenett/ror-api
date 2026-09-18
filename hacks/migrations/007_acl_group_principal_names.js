// Migration: Rename service ACL groups to the hierarchical principal form
//
// Collections affected:
//   - acl: group
//
// Group names are principal names in an email/DNS hierarchy:
//
//   service-<id>@ror.system   ->   <id>@service.ror.system
//
// ror-api emits BOTH names for a service identity while this migration is
// rolling out, so grants keep resolving before and after it runs.
//
// Usage:
//   mongosh 'mongodb://<user>:<pass>@<host>:<port>/nhn-ror?authSource=admin' --file 007_acl_group_principal_names.js
//
// Idempotent: only documents whose group still matches the legacy pattern are
// touched, and a legacy entry is dropped rather than renamed when an equivalent
// entry already exists under the new name.

const db = db.getSiblingDB("nhn-ror");

print("=== ACL group rename: service-<id>@ror.system -> <id>@service.ror.system ===\n");

const legacyPattern = /^service-(.+)@ror\.system$/;

const legacyEntries = db.acl
  .find({ group: { $regex: "^service-.+@ror\\.system$" } })
  .toArray();

print(`Found ${legacyEntries.length} legacy service group entries\n`);

let renamed = 0;
let removedDuplicate = 0;

legacyEntries.forEach((entry) => {
  const match = legacyPattern.exec(entry.group);
  if (!match) return;

  const serviceId = match[1];
  const newGroup = `${serviceId}@service.ror.system`;

  // An equivalent grant may already exist under the new name (e.g. re-seeded
  // before this migration ran). Renaming would then create a duplicate.
  const duplicate = db.acl.findOne({
    _id: { $ne: entry._id },
    group: newGroup,
    scope: entry.scope,
    subject: entry.subject,
  });

  if (duplicate) {
    db.acl.deleteOne({ _id: entry._id });
    removedDuplicate++;
    print(`  removed duplicate legacy entry ${entry.group} (${entry.scope}/${entry.subject})`);
    return;
  }

  db.acl.updateOne({ _id: entry._id }, { $set: { group: newGroup } });
  renamed++;
  print(`  ${entry.group} -> ${newGroup} (${entry.scope}/${entry.subject})`);
});

print(`\n--- Summary ---`);
print(`  renamed:            ${renamed}`);
print(`  duplicates removed: ${removedDuplicate}`);

const remaining = db.acl.countDocuments({
  group: { $regex: "^service-.+@ror\\.system$" },
});
print(`  legacy remaining:   ${remaining}`);
print("\n=== Done ===");
