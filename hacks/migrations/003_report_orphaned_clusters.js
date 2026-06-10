// Report: List orphaned clusters (no KubernetesCluster resource) with resource and ACL counts
//
// Usage:
//   mongosh 'mongodb://<user>:<pass>@<host>:<port>/nhn-ror?authSource=admin' --file 003_report_orphaned_clusters.js

const db = db.getSiblingDB("nhn-ror");

// Build set of clusterids that have a KubernetesCluster resource
const knownClusterIds = new Set();
db.resourcesv2
  .find(
    { "typemeta.kind": "KubernetesCluster" },
    { "metadata.name": 1, "rormeta.ownerref.subject": 1 }
  )
  .forEach((doc) => {
    if (doc.metadata && doc.metadata.name && doc.metadata.name !== "unknown-undefined") {
      knownClusterIds.add(doc.metadata.name);
    }
    const subject = doc.rormeta && doc.rormeta.ownerref && doc.rormeta.ownerref.subject;
    if (subject) {
      knownClusterIds.add(subject);
    }
  });

// Find all non-UUID subjects in resourcesv2 with KubernetesCluster scope
const orphans = {};
db.resourcesv2
  .aggregate([
    {
      $match: {
        "rormeta.ownerref.scope": "KubernetesCluster",
        "rormeta.ownerref.subject": {
          $not: /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i,
        },
      },
    },
    { $group: { _id: "$rormeta.ownerref.subject", resources: { $sum: 1 } } },
  ])
  .forEach((g) => {
    if (!knownClusterIds.has(g._id)) {
      orphans[g._id] = { resources: g.resources, acls: 0 };
    }
  });

// Count ACL entries per orphaned cluster
db.acl
  .aggregate([
    {
      $match: {
        scope: "KubernetesCluster",
        subject: {
          $not: /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i,
        },
      },
    },
    { $group: { _id: "$subject", acls: { $sum: 1 } } },
  ])
  .forEach((g) => {
    if (orphans[g._id]) {
      orphans[g._id].acls = g.acls;
    } else if (!knownClusterIds.has(g._id)) {
      orphans[g._id] = { resources: 0, acls: g.acls };
    }
  });

// Check if orphaned clusters exist in the legacy clusters collection
const inClustersCollection = new Set();
db.clusters.find({}, { clusterid: 1, identifier: 1 }).forEach((doc) => {
  const cid = doc.clusterid || doc.identifier;
  if (cid && orphans[cid]) {
    inClustersCollection.add(cid);
  }
});

// Sort by resource count descending
const sorted = Object.entries(orphans).sort((a, b) => b[1].resources - a[1].resources);

// Print report
print("=== Orphaned Clusters Report ===\n");
print(
  "ClusterId".padEnd(40) +
    "Resources".padStart(10) +
    "ACLs".padStart(8) +
    "  In clusters collection"
);
print("-".repeat(82));

let totalResources = 0;
let totalAcls = 0;
for (const [clusterId, counts] of sorted) {
  const inLegacy = inClustersCollection.has(clusterId) ? "yes" : "no";
  print(
    clusterId.padEnd(40) +
      String(counts.resources).padStart(10) +
      String(counts.acls).padStart(8) +
      "  " +
      inLegacy
  );
  totalResources += counts.resources;
  totalAcls += counts.acls;
}

print("-".repeat(82));
print(
  "TOTAL".padEnd(40) +
    String(totalResources).padStart(10) +
    String(totalAcls).padStart(8)
);
print(`\n${sorted.length} orphaned clusters`);
print(`${inClustersCollection.size} of them exist in the legacy clusters collection`);
