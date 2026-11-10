// Run as the bootstrap administrator; the API role cannot manage indexes.
const resourceIndexResult = db.getSiblingDB("nhn-ror").runCommand({
    createIndexes: "resourcesv2",
    indexes: [
        { key: { uid: 1 }, name: "uid_1", unique: true },
        { key: { "rormeta.ownerref": 1 }, name: "rormeta.ownerref_1" },
        { key: { "rormeta.ownerref.subject": 1 }, name: "rormeta.ownerref.subject_1" },
        {
            key: { "rormeta.ownerref.scope": 1, "rormeta.ownerref.subject": 1, "metadata.name": 1 },
            name: "rormeta.ownerref.scope_1_rormeta.ownerref.subject_1_metadata.name_1"
        },
        {
            key: { "typemeta.kind": 1, "typemeta.apiversion": 1, "metadata.name": 1 },
            name: "typemeta.kind_1_typemeta.apiversion_1_metadata.name_1"
        }
    ]
});
assert(resourceIndexResult.ok === 1,
    "Could not ensure resourcesv2 indexes: " + JSON.stringify(resourceIndexResult));
