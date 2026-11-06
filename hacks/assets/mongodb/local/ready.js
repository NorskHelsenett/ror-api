// Init scripts do not run again for existing data directories.
const app = db.getSiblingDB("nhn-ror");
const role = app.getRole("roleRorApi", { showPrivileges: true });
assert(role, "roleRorApi is missing; check the MongoDB initialization logs");
const collection = "acl_snapshot_publication";
const actions = ["find", "update", "insert", "remove"];
const granted = role.privileges.filter(p =>
    p.resource.db === "nhn-ror" && p.resource.collection === collection
).flatMap(p => p.actions);
if (!actions.every(action => granted.includes(action))) {
    app.grantPrivilegesToRole("roleRorApi", [{
        resource: { db: "nhn-ror", collection },
        actions
    }]);
}
