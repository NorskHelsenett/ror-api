// Migration: Replace clusterid with UID in ownerref subjects
//
// Collections affected:
//   - resourcesv2: rormeta.ownerref.subject (where scope=KubernetesCluster)
//   - acl: subject (where scope=KubernetesCluster)
//
// The lookup table is built from KubernetesCluster resources in resourcesv2,
// mapping kubernetescluster.status.agentstatus.clusterid → uid.
//
// Usage:
//   mongosh 'mongodb://<user>:<pass>@<host>:<port>/nhn-ror?authSource=admin' --file 002_subject_clusterid_to_uid.js
//
// This migration is idempotent — UIDs are UUIDv4/v5 format and will not match
// any clusterid pattern, so re-running will not double-migrate.

const db = db.getSiblingDB("nhn-ror");

print("=== Subject Migration: clusterid → UID ===\n");

// --- Build lookup map: clusterid → uid ---
// The authoritative source for clusterid is the agent-reported state field
// kubernetescluster.status.agentstatus.clusterid. This is stable across
// ownerref normalization (unlike rormeta.ownerref.subject which gets migrated).
print("--- Building clusterid → UID lookup map ---");
const clusterIdToUid = {};

db.resourcesv2
  .find(
    { "typemeta.kind": "KubernetesCluster" },
    { "kubernetescluster.status.agentstatus.clusterid": 1, uid: 1 }
  )
  .forEach((doc) => {
    if (!doc.uid) return;
    const clusterid =
      doc.kubernetescluster &&
      doc.kubernetescluster.status &&
      doc.kubernetescluster.status.agentstatus &&
      doc.kubernetescluster.status.agentstatus.clusterid;
    if (clusterid) {
      clusterIdToUid[clusterid] = doc.uid;
    }
  });

const clusterCount = Object.keys(clusterIdToUid).length;
print(`  Found ${clusterCount} KubernetesCluster resources\n`);

if (clusterCount === 0) {
  print("ERROR: No KubernetesCluster resources found. Aborting.");
  quit(1);
}

// --- 1. Migrate resourcesv2.rormeta.ownerref.subject ---
print("--- Step 1: Migrate resourcesv2.rormeta.ownerref.subject ---");
let totalUpdated = 0;
let orphaned = 0;

// Get distinct subjects to iterate (avoids scanning 734k docs per cluster)
const distinctSubjects = db.resourcesv2.distinct("rormeta.ownerref.subject", {
  "rormeta.ownerref.scope": "KubernetesCluster",
});

for (const subject of distinctSubjects) {
  // Skip if subject is already a UUID (already migrated)
  if (/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(subject)) {
    continue;
  }

  const uid = clusterIdToUid[subject];
  if (!uid) {
    const count = db.resourcesv2.countDocuments({
      "rormeta.ownerref.scope": "KubernetesCluster",
      "rormeta.ownerref.subject": subject,
    });
    print(`  WARNING: No UID found for clusterid "${subject}" (${count} docs) — skipped`);
    orphaned += count;
    continue;
  }

  const result = db.resourcesv2.updateMany(
    {
      "rormeta.ownerref.scope": "KubernetesCluster",
      "rormeta.ownerref.subject": subject,
    },
    { $set: { "rormeta.ownerref.subject": uid } }
  );
  print(`  ${subject} → ${uid}: ${result.modifiedCount} docs`);
  totalUpdated += result.modifiedCount;
}
print(`  Total: ${totalUpdated} docs updated, ${orphaned} orphaned\n`);

// --- 2. Migrate acl.subject for scope=KubernetesCluster ---
print("--- Step 2: Migrate acl.subject (scope=KubernetesCluster) ---");
let aclUpdated = 0;
let aclOrphaned = 0;

const aclSubjects = db.acl.distinct("subject", {
  scope: "KubernetesCluster",
});

for (const subject of aclSubjects) {
  // Skip if already a UUID
  if (/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(subject)) {
    continue;
  }

  const uid = clusterIdToUid[subject];
  if (!uid) {
    const count = db.acl.countDocuments({
      scope: "KubernetesCluster",
      subject: subject,
    });
    print(`  WARNING: No UID found for clusterid "${subject}" (${count} ACL docs) — skipped`);
    aclOrphaned += count;
    continue;
  }

  const result = db.acl.updateMany(
    { scope: "KubernetesCluster", subject: subject },
    { $set: { subject: uid } }
  );
  print(`  ${subject} → ${uid}: ${result.modifiedCount} ACL docs`);
  aclUpdated += result.modifiedCount;
}
print(`  Total: ${aclUpdated} ACL docs updated, ${aclOrphaned} orphaned\n`);

// --- 3. Fix KubernetesCluster self-referencing ownerrefs ---
print("--- Step 3: Fix KubernetesCluster self-referencing ownerrefs ---");
let selfRefFixed = 0;
db.resourcesv2
  .find(
    {
      "typemeta.kind": "KubernetesCluster",
      "rormeta.ownerref.scope": "KubernetesCluster",
      "rormeta.ownerref.subject": { $not: /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i },
    },
    { uid: 1, "rormeta.ownerref.subject": 1 }
  )
  .forEach((doc) => {
    if (doc.uid) {
      db.resourcesv2.updateOne(
        { _id: doc._id },
        { $set: { "rormeta.ownerref.subject": doc.uid } }
      );
      selfRefFixed++;
    }
  });
print(`  Fixed ${selfRefFixed} self-referencing KubernetesCluster ownerrefs\n`);

print("=== Migration complete ===");
print(`  resourcesv2: ${totalUpdated} updated, ${orphaned} orphaned`);
print(`  acl: ${aclUpdated} updated, ${aclOrphaned} orphaned`);
