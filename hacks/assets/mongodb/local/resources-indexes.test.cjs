const assert = require("node:assert/strict");
const { readFileSync } = require("node:fs");
const { join } = require("node:path");
const { test } = require("node:test");
const vm = require("node:vm");

const source = readFileSync(join(__dirname, "resources-indexes.js"), "utf8");

test("resource bootstrap declares all indexes, including unique UID", () => {
    let indexes;
    vm.runInNewContext(source, {
        assert,
        db: {
            getSiblingDB(name) {
                assert.equal(name, "nhn-ror");
                return {
                    runCommand(command) {
                        assert.equal(command.createIndexes, "resourcesv2");
                        indexes = command.indexes;
                        return { ok: 1 };
                    }
                };
            }
        }
    });
    assert.deepEqual(JSON.parse(JSON.stringify(indexes)), [
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
    ]);
});

test("readiness reapplies indexes and propagates index build failures", () => {
    const ready = readFileSync(join(__dirname, "ready.js"), "utf8");
    for (const fail of [false, true]) {
        let builds = 0;
        const context = vm.createContext({
            assert,
            db: {
                getSiblingDB() {
                    return {
                        getRole() {
                            return { privileges: [{
                                resource: { db: "nhn-ror", collection: "acl_snapshot_publication" },
                                actions: ["find", "update", "insert", "remove"]
                            }] };
                        },
                        runCommand() {
                            builds++;
                            return fail ? { ok: 0, errmsg: "duplicate UID" } : { ok: 1 };
                        }
                    };
                }
            },
            load(path) {
                assert.equal(path, "/local-mongo/local/resources-indexes.js");
                vm.runInContext(source, context);
            }
        });
        if (fail) {
            assert.throws(() => vm.runInContext(ready, context), /duplicate UID/);
        } else {
            vm.runInContext(ready, context);
        }
        assert.equal(builds, 1);
    }
});
