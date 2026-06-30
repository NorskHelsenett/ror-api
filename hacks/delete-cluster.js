// delete-cluster.js
//
// Deletes a single cluster and all of its related data from the ROR MongoDB.
// Input is the cluster UID (the uid used in the `clusters` and `resourcesv2`
// collections). The clusterid needed for the legacy `resources` (v1) collection
// is resolved automatically from the `clusters` document.
//
// Deletes:
//   clusters     - the cluster document                     (uid)
//   resources    - all v1 resources owned by the cluster    (owner.subject = clusterid)
//   resourcesv2  - the KubernetesCluster doc + all children (uid OR ownerref.subject = uid)
//   acl          - all acl entries for the cluster           (scope=KubernetesCluster, subject=uid)
//
// Usage (DRY RUN first — reports counts, deletes nothing):
//   mongosh "<conn-string>/nhn-ror" --eval 'var UID="<cluster-uid>"' --file delete-cluster.js
//
// Apply for real:
//   mongosh "<conn-string>/nhn-ror" --eval 'var UID="<cluster-uid>"; var DRY_RUN=false' --file delete-cluster.js
//
// You can also set UID below instead of passing --eval.

(function () {
	if (typeof UID === "undefined" || !UID) {
		UID = ""; // <-- optionally hard-code the cluster uid here
	}
	if (typeof DRY_RUN === "undefined") {
		DRY_RUN = true; // safe default: report only
	}

	const KIND = "KubernetesCluster";

	if (!UID) {
		print("ERROR: UID is not set. Pass it with --eval 'var UID=\"<uid>\"' or edit the script.");
		quit(1);
	}

	print("=== delete-cluster ===");
	print("UID:     " + UID);
	print("DRY_RUN: " + DRY_RUN);

	// Resolve the cluster document. clusterid is required for the v1 `resources`
	// collection, which keys ownership by clusterid rather than uid.
	const clusterDoc = db.clusters.findOne({ uid: UID });
	if (!clusterDoc) {
		print("ERROR: no document in 'clusters' with uid=" + UID + ". Aborting.");
		quit(1);
	}
	const clusterId = clusterDoc.clusterid;
	if (!clusterId) {
		print("ERROR: cluster document is missing 'clusterid' for uid=" + UID + ". Aborting.");
		quit(1);
	}
	print("clusterid: " + clusterId + "  (clustername: " + (clusterDoc.clustername || "") + ")");

	// Filters.
	const fClusters = { uid: UID };
	const fResources = { "owner.scope": KIND, "owner.subject": clusterId };           // v1, keyed by clusterid
	const fResourcesV2 = { $or: [{ uid: UID }, { "rormeta.ownerref.subject": UID }] }; // own doc + children
	const fAcl = { scope: KIND, subject: UID };

	// Report matched counts.
	const counts = {
		clusters: db.clusters.countDocuments(fClusters),
		resources: db.resources.countDocuments(fResources),
		resourcesv2: db.resourcesv2.countDocuments(fResourcesV2),
		acl: db.acl.countDocuments(fAcl),
	};

	print("\n-- documents matched --");
	print("clusters:    " + counts.clusters);
	print("resources:   " + counts.resources);
	print("resourcesv2: " + counts.resourcesv2);
	print("acl:         " + counts.acl);

	// Informational: acl entries that reference this cluster by clusterid are NOT
	// deleted (this script removes acls by uid only, per spec).
	const aclByClusterId = db.acl.countDocuments({ scope: KIND, subject: clusterId });
	if (aclByClusterId > 0) {
		print(
			"NOTE: " + aclByClusterId + " acl doc(s) reference this cluster by clusterid (subject=" +
				clusterId + ") and are NOT deleted by this script."
		);
	}

	if (DRY_RUN) {
		print("\nDRY_RUN=true — nothing deleted. Re-run with 'var DRY_RUN=false' to apply.");
		return;
	}

	// Delete.
	print("\n-- deleting --");
	print("resources:   " + db.resources.deleteMany(fResources).deletedCount);
	print("resourcesv2: " + db.resourcesv2.deleteMany(fResourcesV2).deletedCount);
	print("acl:         " + db.acl.deleteMany(fAcl).deletedCount);
	print("clusters:    " + db.clusters.deleteMany(fClusters).deletedCount);

	print("\nDone.");
})();
