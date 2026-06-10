// Migration: Rename legacy scope values to resource Kind names
//
// Collections affected:
//   - acl: scope field + subject field (for scope="ror" docs)
//   - resourcesv2: rormeta.ownerref.scope field
//
// Usage:
//   mongosh 'mongodb://<user>:<pass>@<host>:<port>/nhn-ror?authSource=admin' --file 001_scope_to_kind.js
//
// This migration is idempotent — running it multiple times is safe.
// It only renames values that match the legacy names.

const scopeMap = {
  cluster: "KubernetesCluster",
  project: "Project",
  workspace: "Workspace",
  virtualmachine: "VirtualMachine",
  backup: "BackupJob",
  datacenter: "Datacenter",
  machine: "Machine",
};

// Subjects in "ror"-scoped ACL docs that represent type-level grants
const subjectMap = {
  cluster: "KubernetesCluster",
  project: "Project",
  workspace: "Workspace",
  virtualmachine: "VirtualMachine",
  backup: "BackupJob",
  datacenter: "Datacenter",
  machine: "Machine",
};

const db = db.getSiblingDB("nhn-ror");

print("=== ACL Scope Migration: legacy names → Kind names ===\n");

// --- 1. Migrate acl.scope ---
print("--- Step 1: Migrate acl.scope ---");
for (const [legacy, kind] of Object.entries(scopeMap)) {
  const filter = { scope: legacy };
  const count = db.acl.countDocuments(filter);
  if (count === 0) {
    print(`  ${legacy} → ${kind}: 0 docs (skip)`);
    continue;
  }
  const result = db.acl.updateMany(filter, { $set: { scope: kind } });
  print(`  ${legacy} → ${kind}: ${result.modifiedCount} docs updated`);
}

// --- 2. Migrate acl.subject for scope="ror" docs ---
print("\n--- Step 2: Migrate acl.subject (scope=ror) ---");
// After step 1, there are no scope="ror" docs to rename the scope itself
// (ror is not in scopeMap), but subjects within ror-scoped docs need updating.
for (const [legacy, kind] of Object.entries(subjectMap)) {
  const filter = { scope: "ror", subject: legacy };
  const count = db.acl.countDocuments(filter);
  if (count === 0) {
    print(`  subject ${legacy} → ${kind}: 0 docs (skip)`);
    continue;
  }
  const result = db.acl.updateMany(filter, { $set: { subject: kind } });
  print(`  subject ${legacy} → ${kind}: ${result.modifiedCount} docs updated`);
}

// --- 3. Migrate resourcesv2.rormeta.ownerref.scope ---
print("\n--- Step 3: Migrate resourcesv2.rormeta.ownerref.scope ---");
for (const [legacy, kind] of Object.entries(scopeMap)) {
  const filter = { "rormeta.ownerref.scope": legacy };
  const count = db.resourcesv2.countDocuments(filter);
  if (count === 0) {
    print(`  ${legacy} → ${kind}: 0 docs (skip)`);
    continue;
  }
  const result = db.resourcesv2.updateMany(filter, {
    $set: { "rormeta.ownerref.scope": kind },
  });
  print(`  ${legacy} → ${kind}: ${result.modifiedCount} docs updated`);
}

print("\n=== Migration complete ===");
