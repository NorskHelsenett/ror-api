// DRY-RUN report: cluster uid backfill + resourcesv2 dedupe (NO WRITES)
//
// This script ONLY reads and reports. It performs NO updates or deletes. It
// describes exactly what a future apply-script (005) would change so the impact
// can be reviewed before any destructive operation is run.
//
// It covers the cleanup for the "cluster created multiple times" problem:
//   A. Backfill the authoritative cluster uid onto apikeys (apikeys.uid).
//   B. De-duplicate KubernetesCluster resources in resourcesv2 (one canonical
//      document per clusterid; the rest are orphans to delete).
//   C. Re-point child resources (rormeta.ownerref.subject) from orphan uids to
//      the canonical uid.
//   D. Re-point ACL entries (acl.subject) from orphan uids to the canonical uid.
//   E. Backfill the canonical uid onto the legacy clusters collection
//      (clusters.uid).
//
// Canonical selection per clusterid (matches the API's deterministic resolver):
//   1. Prefer a self-owning document where uid == rormeta.ownerref.subject.
//   2. Among ties (or if none self-own) pick the oldest by
//      metadata.creationtimestamp.time, then _id.
// The chosen document is KEPT; all other documents for that clusterid are
// orphans. The kept document's uid is the authoritative cluster uid.
//
// Usage:
//   mongosh 'mongodb://<user>:<pass>@<host>:<port>/nhn-ror?authSource=admin' --file 004_dryrun_uid_cleanup.js

const db = db.getSiblingDB("nhn-ror");

const UUID_RE = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

// Sentinel clusterids that are NOT real clusters and must be excluded from
// dedupe/backfill. "unknown-undefined" is emitted by agents that never learned
// their real clusterid; collapsing it would merge unrelated junk documents.
const SKIP_CLUSTERIDS = new Set(["unknown-undefined"]);
let sentinelDocs = 0;

print("=== DRY-RUN: cluster uid backfill + resourcesv2 dedupe (NO WRITES) ===\n");

// ---------------------------------------------------------------------------
// Build the per-clusterid view of KubernetesCluster documents.
// ---------------------------------------------------------------------------
// clusterid -> { kept: {uid, subject, id, selfOwning, created},
//                orphans: [{uid, ...}],
//                selfOwningCount }
const clusters = {};
// uid -> clusterid, only for orphan uids (uids of documents that will be deleted)
const orphanUidToCanonical = {};
const orphanUids = [];

let kcDocs = 0;
let kcDocsNoClusterId = 0;

const cursor = db.resourcesv2.find(
  { "typemeta.kind": "KubernetesCluster" },
  {
    uid: 1,
    "rormeta.ownerref.subject": 1,
    "kubernetescluster.status.agentstatus.clusterid": 1,
    "metadata.creationtimestamp.time": 1,
  }
);

const byCluster = {};
cursor.forEach((doc) => {
  kcDocs++;
  const clusterid =
    doc.kubernetescluster &&
    doc.kubernetescluster.status &&
    doc.kubernetescluster.status.agentstatus &&
    doc.kubernetescluster.status.agentstatus.clusterid;
  if (!clusterid) {
    kcDocsNoClusterId++;
    return;
  }
  if (SKIP_CLUSTERIDS.has(clusterid)) {
    sentinelDocs++;
    return;
  }
  const subject =
    (doc.rormeta && doc.rormeta.ownerref && doc.rormeta.ownerref.subject) || "";
  const created =
    (doc.metadata &&
      doc.metadata.creationtimestamp &&
      doc.metadata.creationtimestamp.time) ||
    null;
  if (!byCluster[clusterid]) byCluster[clusterid] = [];
  byCluster[clusterid].push({
    uid: doc.uid || "",
    subject: subject,
    id: doc._id,
    selfOwning: !!doc.uid && doc.uid === subject,
    created: created ? new Date(created).getTime() : Number.MAX_SAFE_INTEGER,
  });
});

for (const clusterid of Object.keys(byCluster)) {
  const docs = byCluster[clusterid];
  // Sort: self-owning first, then oldest, then _id for stability.
  docs.sort((a, b) => {
    if (a.selfOwning !== b.selfOwning) return a.selfOwning ? -1 : 1;
    if (a.created !== b.created) return a.created - b.created;
    return String(a.id).localeCompare(String(b.id));
  });
  const kept = docs[0];
  const orphans = docs.slice(1);
  const selfOwningCount = docs.filter((d) => d.selfOwning).length;
  clusters[clusterid] = { kept, orphans, selfOwningCount };
  for (const o of orphans) {
    if (o.uid && o.uid !== kept.uid) {
      orphanUidToCanonical[o.uid] = kept.uid;
      orphanUids.push(o.uid);
    }
  }
}

// ---------------------------------------------------------------------------
// Section A: apikeys.uid backfill
// ---------------------------------------------------------------------------
print("--- A. apikeys.uid backfill ---");
const clusterApikeys = db.apikeys
  .find({ type: "Cluster" }, { identifier: 1, uid: 1 })
  .toArray();

let akTotal = clusterApikeys.length;
let akAlreadySet = 0;
let akWouldBackfill = 0;
let akNoResource = 0;
let akAlreadyCorrect = 0;
let akWouldChange = 0;
let akSentinel = 0;
const akNoResourceList = [];

for (const ak of clusterApikeys) {
  if (SKIP_CLUSTERIDS.has(ak.identifier)) {
    akSentinel++;
    continue;
  }
  const cluster = clusters[ak.identifier];
  const canonicalUid = cluster ? cluster.kept.uid : null;
  if (ak.uid && ak.uid !== "") akAlreadySet++;
  if (!canonicalUid) {
    akNoResource++;
    if (akNoResourceList.length < 50) akNoResourceList.push(ak.identifier);
    continue;
  }
  if (ak.uid === canonicalUid) {
    akAlreadyCorrect++;
  } else if (ak.uid && ak.uid !== "") {
    akWouldChange++; // already has a (different) uid — review
  } else {
    akWouldBackfill++;
  }
}

print(`  cluster apikeys total:            ${akTotal}`);
print(`  already have a uid set:           ${akAlreadySet}`);
print(`  would backfill (uid empty -> set):${akWouldBackfill}`);
print(`  already correct (uid == canonical):${akAlreadyCorrect}`);
print(`  uid set but differs (REVIEW):     ${akWouldChange}`);
print(`  no KubernetesCluster resource:    ${akNoResource}`);
print(`  sentinel identifiers skipped:     ${akSentinel}`);
if (akNoResourceList.length > 0) {
  print(`    clusterids without a resource (first ${akNoResourceList.length}):`);
  for (const id of akNoResourceList) print(`      - ${id}`);
}
print("");

// ---------------------------------------------------------------------------
// Section B: resourcesv2 KubernetesCluster dedupe
// ---------------------------------------------------------------------------
print("--- B. resourcesv2 KubernetesCluster dedupe ---");
let distinctClusterIds = Object.keys(clusters).length;
let clustersWithDup = 0;
let docsToDelete = 0;
let keptNeedsNormalize = 0; // kept.subject != kept.uid -> would set subject = uid
let multiSelfOwning = 0;
const multiSelfOwningList = [];

for (const clusterid of Object.keys(clusters)) {
  const c = clusters[clusterid];
  if (c.orphans.length > 0) clustersWithDup++;
  docsToDelete += c.orphans.length;
  if (c.kept.uid && c.kept.subject !== c.kept.uid) keptNeedsNormalize++;
  if (c.selfOwningCount > 1) {
    multiSelfOwning++;
    if (multiSelfOwningList.length < 50) multiSelfOwningList.push(clusterid);
  }
}

print(`  KubernetesCluster docs scanned:   ${kcDocs}`);
print(`  docs without a clusterid (skip):  ${kcDocsNoClusterId}`);
print(`  sentinel docs skipped:            ${sentinelDocs}  (${[...SKIP_CLUSTERIDS].join(", ")})`);
print(`  distinct clusterids:              ${distinctClusterIds}`);
print(`  clusters with duplicates:         ${clustersWithDup}`);
print(`  orphan docs to DELETE:            ${docsToDelete}`);
print(`  kept docs needing ownerref fix:   ${keptNeedsNormalize}  (subject -> own uid)`);
print(`  clusters with >1 self-owning doc: ${multiSelfOwning}  (manual review)`);
if (multiSelfOwningList.length > 0) {
  for (const id of multiSelfOwningList) print(`      - ${id}`);
}
print("");

// ---------------------------------------------------------------------------
// Section C: child resource re-point (rormeta.ownerref.subject)
// ---------------------------------------------------------------------------
print("--- C. child resource re-point (rormeta.ownerref.subject) ---");
print(`  distinct orphan uids to re-point: ${orphanUids.length}`);

let childResourcesToRepoint = 0;
if (orphanUids.length > 0) {
  // Non-KubernetesCluster resources whose owner is an orphan uid (the orphan
  // KubernetesCluster docs themselves are deleted, not re-pointed).
  childResourcesToRepoint = db.resourcesv2.countDocuments({
    "typemeta.kind": { $ne: "KubernetesCluster" },
    "rormeta.ownerref.subject": { $in: orphanUids },
  });
}
print(`  child resources to re-point:      ${childResourcesToRepoint}`);
print("");

// ---------------------------------------------------------------------------
// Section D: ACL re-point (acl.subject)
// ---------------------------------------------------------------------------
print("--- D. ACL re-point (acl.subject) ---");
let aclToRepoint = 0;
if (orphanUids.length > 0) {
  aclToRepoint = db.acl.countDocuments({
    scope: "KubernetesCluster",
    subject: { $in: orphanUids },
  });
}
print(`  ACL entries to re-point:          ${aclToRepoint}`);
print("");

// ---------------------------------------------------------------------------
// Section E: legacy clusters collection uid backfill (clusters.uid)
// ---------------------------------------------------------------------------
// The legacy `clusters` collection carries its own `uid` field. It must be set
// to the same canonical uid so downstream consumers reading that collection
// agree with resourcesv2 / apikeys.
print("--- E. clusters.uid backfill (legacy clusters collection) ---");
const clusterDocs = db.clusters
  .find({}, { clusterid: 1, uid: 1 })
  .toArray();

let clTotal = clusterDocs.length;
let clAlreadyCorrect = 0;
let clWouldBackfill = 0;
let clWouldChange = 0;
let clNoResource = 0;
let clSentinel = 0;
const clWouldChangeList = [];

for (const cl of clusterDocs) {
  if (SKIP_CLUSTERIDS.has(cl.clusterid)) {
    clSentinel++;
    continue;
  }
  const cluster = clusters[cl.clusterid];
  const canonicalUid = cluster ? cluster.kept.uid : null;
  if (!canonicalUid) {
    clNoResource++;
    continue;
  }
  if (cl.uid === canonicalUid) {
    clAlreadyCorrect++;
  } else if (cl.uid && cl.uid !== "") {
    clWouldChange++; // has a different uid -> would be overwritten with canonical
    if (clWouldChangeList.length < 50) clWouldChangeList.push(cl.clusterid);
  } else {
    clWouldBackfill++;
  }
}

print(`  clusters total:                   ${clTotal}`);
print(`  already correct (uid == canonical):${clAlreadyCorrect}`);
print(`  would backfill (uid empty -> set):${clWouldBackfill}`);
print(`  uid set but differs (overwrite):  ${clWouldChange}`);
print(`  no KubernetesCluster resource:    ${clNoResource}`);
print(`  sentinel clusterids skipped:      ${clSentinel}`);
if (clWouldChangeList.length > 0) {
  print(`    clusterids whose uid would change (first ${clWouldChangeList.length}):`);
  for (const id of clWouldChangeList) print(`      - ${id}`);
}
print("");

// ---------------------------------------------------------------------------
// Summary
// ---------------------------------------------------------------------------
print("=== SUMMARY (no changes applied) ===");
print(`  apikeys.uid to backfill:          ${akWouldBackfill}`);
print(`  apikeys.uid differing (review):   ${akWouldChange}`);
print(`  KubernetesCluster docs to delete: ${docsToDelete}`);
print(`  kept docs to normalize ownerref:  ${keptNeedsNormalize}`);
print(`  child resources to re-point:      ${childResourcesToRepoint}`);
print(`  ACL entries to re-point:          ${aclToRepoint}`);
print(`  clusters.uid to backfill:         ${clWouldBackfill}`);
print(`  clusters.uid differing (overwrite):${clWouldChange}`);
print("\nDRY-RUN complete. No documents were modified.");
