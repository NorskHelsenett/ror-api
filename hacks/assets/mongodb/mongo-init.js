let res = [
    db.createRole({ role: "roleRorApi",
        privileges: [
        { resource: { db: "nhn-ror", collection: "clusters" }, actions: [ "find", "update", "insert", "remove" ] },
        { resource: { db: "nhn-ror", collection: "workspaces" }, actions: [ "find", "update", "insert", "remove" ] },
        { resource: { db: "nhn-ror", collection: "desired_versions" }, actions: [ "find", "update", "insert", "remove" ] },
        { resource: { db: "nhn-ror", collection: "datacenters" }, actions: [ "find", "update", "insert", "remove" ] },
        { resource: { db: "nhn-ror", collection: "auditlogs" }, actions: [ "find", "update", "insert", "remove" ] },
        { resource: { db: "nhn-ror", collection: "apikeys" }, actions: [ "find", "update", "insert", "remove" ] },
        { resource: { db: "nhn-ror", collection: "prices" }, actions: [ "find", "update", "insert", "remove" ] },
        { resource: { db: "nhn-ror", collection: "projects" }, actions: [ "find", "update", "insert", "remove" ] },
        { resource: { db: "nhn-ror", collection: "operatorconfigs" }, actions: [ "find", "update", "insert", "remove" ] },
        { resource: { db: "nhn-ror", collection: "resources" }, actions: [ "find", "update", "insert", "remove" ] },
        { resource: { db: "nhn-ror", collection: "resourcesv2" }, actions: [ "find", "update", "insert", "remove" ] },
        { resource: { db: "nhn-ror", collection: "acl" }, actions: [ "find", "update", "insert", "remove" ] },
        { resource: { db: "nhn-ror", collection: "tasks" }, actions: [ "find", "update", "insert", "remove" ] },
        { resource: { db: "nhn-ror", collection: "messagerulesets" }, actions: [ "find", "update", "insert", "remove" ] },
        { resource: { db: "nhn-ror", collection: "metrics" }, actions: [ "find", "update", "insert", "remove" ] },
        ],
        roles: []
    }),
  
    db.createRole({ role: "roleRorMs",
        privileges: [
        { resource: { db: "nhn-ror", collection: "clusters" }, actions: [ "find", "update", "insert", "remove" ] },
        { resource: { db: "nhn-ror", collection: "workspaces" }, actions: [ "find", "update", "insert", "remove" ] },
        { resource: { db: "nhn-ror", collection: "desired_versions" }, actions: [ "find", "update", "insert", "remove" ] },
        { resource: { db: "nhn-ror", collection: "datacenters" }, actions: [ "find", "update", "insert", "remove" ] },
        { resource: { db: "nhn-ror", collection: "auditlogs" }, actions: [ "find", "update", "insert", "remove" ] },
        { resource: { db: "nhn-ror", collection: "prices" }, actions: [ "find", "update", "insert", "remove" ] },
        { resource: { db: "nhn-ror", collection: "projects" }, actions: [ "find", "update", "insert", "remove" ] },
        { resource: { db: "nhn-ror", collection: "operatorconfigs" }, actions: [ "find", "update", "insert", "remove" ] },
        { resource: { db: "nhn-ror", collection: "resources" }, actions: [ "find", "update", "insert", "remove" ] },
        { resource: { db: "nhn-ror", collection: "acl" }, actions: [ "find", "update", "insert", "remove" ] },
        { resource: { db: "nhn-ror", collection: "tasks" }, actions: [ "find", "update", "insert", "remove" ] },
        { resource: { db: "nhn-ror", collection: "messagerulesets" }, actions: [ "find", "update", "insert", "remove" ] },
        { resource: { db: "nhn-ror", collection: "metrics" }, actions: [ "find", "update", "insert", "remove" ] },
        ],
        roles: []
    })
]

printjson(res)

db.createCollection('clusters')

db.resourcesv2.createIndex(
  { "rormeta.ownerref.scope": 1, "rormeta.ownerref.subject": 1, "metadata.name": 1 },
  { name: "rormeta.ownerref.scope_1_rormeta.ownerref.subject_1_metadata.name_1" }
)
