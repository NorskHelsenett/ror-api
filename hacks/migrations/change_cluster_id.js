// mongosh script to change a cluster ID across clusters, resources, and acl collections.
//
// Usage:
//   mongosh "mongodb://localhost:27017/ror" change_cluster_id.js --eval 'oldId="old-cluster-id"; newId="new-cluster-id";'
//
// Or interactively:
//   mongosh "mongodb://localhost:27017/ror"
//   > oldId = "old-cluster-id"
//   > newId = "new-cluster-id"
//   > load("change_cluster_id.js")
//
// Dry run (default): set dryRun = false to apply changes.
//   mongosh ... --eval 'oldId="old"; newId="new"; dryRun=false;'

if (typeof oldId === "undefined" || typeof newId === "undefined") {
  print("ERROR: You must set 'oldId' and 'newId' before running this script.");
  print('Example: mongosh "mongodb://host/ror" --eval \'oldId="old-id"; newId="new-id";\' change_cluster_id.js');
  quit(1);
}

if (oldId === newId) {
  print("ERROR: oldId and newId are the same. Nothing to do.");
  quit(1);
}

const isDryRun = typeof dryRun === "undefined" ? true : dryRun;

print("=".repeat(60));
print("Change Cluster ID");
print("=".repeat(60));
print(`Old cluster ID: ${oldId}`);
print(`New cluster ID: ${newId}`);
print(`Mode:           ${isDryRun ? "DRY RUN (no changes will be made)" : "LIVE"}`);
print("=".repeat(60));

// --- Pre-flight checks ---

// Check that the old cluster exists
const existingCluster = db.clusters.findOne({ clusterid: oldId });
if (!existingCluster) {
  print(`ERROR: No cluster found with clusterid="${oldId}"`);
  quit(1);
}
print(`\nFound cluster: ${existingCluster.clustername || existingCluster._id} (clusterid: ${oldId})`);

// Check that no cluster already uses the new ID
const conflictCluster = db.clusters.findOne({ clusterid: newId });
if (conflictCluster) {
  print(`ERROR: A cluster with clusterid="${newId}" already exists. Aborting to prevent conflicts.`);
  quit(1);
}

// --- Count affected documents ---

const clusterCount = db.clusters.countDocuments({ clusterid: oldId });
const resourceCount = db.resources.countDocuments({ "owner.scope": "cluster", "owner.subject": oldId });
const aclCount = db.acl.countDocuments({ scope: "cluster", subject: oldId });

print(`\nDocuments to update:`);
print(`  clusters:  ${clusterCount}`);
print(`  resources: ${resourceCount}`);
print(`  acl:       ${aclCount}`);

if (isDryRun) {
  print("\n--- DRY RUN complete. Set dryRun=false to apply changes. ---");
  quit(0);
}

// --- Apply changes ---

// 1. Update cluster document
const clusterResult = db.clusters.updateMany(
  { clusterid: oldId },
  { $set: { clusterid: newId } }
);
print(`\n[clusters]  matched: ${clusterResult.matchedCount}, modified: ${clusterResult.modifiedCount}`);

// 2. Update resources (v1 only, not resourcesv2)
const resourceResult = db.resources.updateMany(
  { "owner.scope": "cluster", "owner.subject": oldId },
  { $set: { "owner.subject": newId } }
);
print(`[resources] matched: ${resourceResult.matchedCount}, modified: ${resourceResult.modifiedCount}`);

// 3. Update ACL entries
const aclResult = db.acl.updateMany(
  { scope: "cluster", subject: oldId },
  { $set: { subject: newId } }
);
print(`[acl]       matched: ${aclResult.matchedCount}, modified: ${aclResult.modifiedCount}`);

// --- Verify no stale references remain ---
print("\n--- Verification ---");
const remainingClusters = db.clusters.countDocuments({ clusterid: oldId });
const remainingResources = db.resources.countDocuments({ "owner.scope": "cluster", "owner.subject": oldId });
const remainingAcl = db.acl.countDocuments({ scope: "cluster", subject: oldId });

if (remainingClusters + remainingResources + remainingAcl > 0) {
  print(`WARNING: Stale references remain! clusters: ${remainingClusters}, resources: ${remainingResources}, acl: ${remainingAcl}`);
} else {
  print("OK: No remaining references to the old cluster ID.");
}

print("\nDone. Cluster ID changed from " + oldId + " to " + newId);
