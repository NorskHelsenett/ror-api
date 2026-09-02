// Migration: Move cluster metadata into resourcesv2 KubernetesCluster resources
//
// Moves the following from the legacy `clusters` collection into the
// `resourcesv2` KubernetesCluster documents:
//   clusters.metadata.criticality  -> kubernetescluster.spec.clustermetadata.criticality
//   clusters.metadata.sensitivity  -> kubernetescluster.spec.clustermetadata.sensitivity
//   clusters.metadata.description  -> kubernetescluster.spec.clustermetadata.description
//   clusters.metadata.roles[]      -> kubernetescluster.spec.clustermetadata.contacts[]
//
// Join key: clusters.uid == resourcesv2.metadata.uid (typemeta.kind = KubernetesCluster)
//
// Value mapping (source stores integer levels, target expects named strings):
//   criticality: 1=Open, 2=Intern, 3=Shielded, 4=HighlyShielded
//   sensitivity: 1=Normal, 2=Moderate, 3=High, 4=Critical
//   contact role: roledefinition copied as-is (Owner / Responsible / TechnicalContact)
//   contact name: taken from contactinfo.upn (source has no separate name field)
//
// Behaviour: overwrites the target criticality/sensitivity/description/contacts
// fields unconditionally. clustermetadata.slackchannels is left untouched.
// The migration is idempotent — re-running produces the same result.
//
// Usage:
//   mongosh 'mongodb://<user>:<pass>@<host>:<port>/nhn-ror?authSource=admin' --file 005_cluster_metadata_to_resource.js

const db = db.getSiblingDB("nhn-ror");

print("=== Cluster metadata → resourcesv2 KubernetesCluster ===\n");

const CRITICALITY = { 1: "Open", 2: "Intern", 3: "Shielded", 4: "HighlyShielded" };
const SENSITIVITY = { 1: "Normal", 2: "Moderate", 3: "High", 4: "Critical" };

function mapContacts(roles) {
  if (!Array.isArray(roles)) return [];
  return roles
    .filter((r) => r && r.contactinfo)
    .map((r) => ({
      name: r.contactinfo.upn || "",
      email: r.contactinfo.email || "",
      phone: r.contactinfo.phone || "",
      role: r.roledefinition || "",
    }));
}

let processed = 0;
let updated = 0;
let unmatched = 0;
const unmatchedIds = [];

db.clusters
  .find(
    { uid: { $exists: true, $ne: null } },
    {
      uid: 1,
      clusterid: 1,
      "metadata.criticality": 1,
      "metadata.sensitivity": 1,
      "metadata.description": 1,
      "metadata.roles": 1,
    }
  )
  .forEach((cluster) => {
    processed++;

    const meta = cluster.metadata || {};
    const clustermetadata = {
      "kubernetescluster.spec.clustermetadata.criticality":
        CRITICALITY[meta.criticality] || "",
      "kubernetescluster.spec.clustermetadata.sensitivity":
        SENSITIVITY[meta.sensitivity] || "",
      "kubernetescluster.spec.clustermetadata.description": meta.description || "",
      "kubernetescluster.spec.clustermetadata.contacts": mapContacts(meta.roles),
    };

    const result = db.resourcesv2.updateMany(
      { "typemeta.kind": "KubernetesCluster", "metadata.uid": cluster.uid },
      { $set: clustermetadata }
    );

    if (result.matchedCount === 0) {
      unmatched++;
      unmatchedIds.push(cluster.clusterid || cluster.uid);
    } else {
      updated += result.modifiedCount;
    }
  });

print(`  Clusters processed:        ${processed}`);
print(`  resourcesv2 docs updated:  ${updated}`);
print(`  Clusters without a match:  ${unmatched}`);
if (unmatchedIds.length > 0) {
  print(`  Unmatched cluster ids:     ${unmatchedIds.join(", ")}`);
}
print("\n=== Done ===");
