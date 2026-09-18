// Migration: Backfill cluster self-access ACL grants
//
// Collections affected:
//   - acl: inserts one grant per cluster
//
// Clusters used to have hardcoded implicit access to their own resources. That
// code is removed: cluster access is now ACL data, resolved through the same
// group mechanism as users and services. Each cluster is a member of the group
//
//   <cluster-uid>@cluster.ror.system
//
// and this migration creates the matching grant:
//
//   { scope: "KubernetesCluster", subject: <uid>,
//     access: ["ror:read", "ror:create", "ror:update"] }
//
// RUN THIS BEFORE DEPLOYING the ror-api version that removes the implicit
// cluster access, otherwise every agent loses access to its own resources.
//
// Usage:
//   mongosh 'mongodb://<user>:<pass>@<host>:<port>/nhn-ror?authSource=admin' --file 008_cluster_self_acl.js
//
// Idempotent: a cluster that already has a self grant is skipped.

const db = db.getSiblingDB("nhn-ror");

print("=== Backfill cluster self-access grants ===\n");

// Cluster uids come from two sources: the apikey (authoritative for cluster
// auth) and the KubernetesCluster resources. Union both so no cluster is missed.
const uids = new Set();

db.apikeys
  .find(
    { type: { $in: ["Cluster", "cluster"] }, uid: { $type: "string", $ne: "" } },
    { uid: 1 }
  )
  .forEach((doc) => uids.add(doc.uid));

const apikeyCount = uids.size;
print(`  ${apikeyCount} cluster uids from apikeys`);

db.resourcesv2
  .find({ "typemeta.kind": "KubernetesCluster", uid: { $type: "string", $ne: "" } }, { uid: 1 })
  .forEach((doc) => uids.add(doc.uid));

print(`  ${uids.size - apikeyCount} additional uids from resourcesv2`);
print(`  ${uids.size} cluster uids total\n`);

if (uids.size === 0) {
  print("ERROR: No cluster uids found. Aborting.");
  quit(1);
}

const access = ["ror:read", "ror:create", "ror:update"];
const now = new Date();

let created = 0;
let skipped = 0;

uids.forEach((uid) => {
  const group = `${uid}@cluster.ror.system`;

  const existing = db.acl.findOne({
    group: group,
    scope: "KubernetesCluster",
    subject: uid,
  });

  if (existing) {
    skipped++;
    return;
  }

  db.acl.insertOne({
    version: 3,
    group: group,
    scope: "KubernetesCluster",
    subject: uid,
    access: access,
    created: now,
    issuedBy: "system@ror.dev",
  });
  created++;
});

print(`--- Summary ---`);
print(`  grants created: ${created}`);
print(`  already present: ${skipped}`);

// Verification: every cluster uid must now have a self grant.
let missing = 0;
uids.forEach((uid) => {
  const found = db.acl.countDocuments({
    group: `${uid}@cluster.ror.system`,
    scope: "KubernetesCluster",
    subject: uid,
  });
  if (found === 0) {
    missing++;
    print(`  MISSING: ${uid}`);
  }
});

print(`  missing after run: ${missing}`);
if (missing > 0) {
  print("\nERROR: some clusters have no self grant. Do NOT deploy the removal of implicit cluster access.");
  quit(1);
}

print("\n=== Done ===");
