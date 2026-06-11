# ror-mongodb-backup

A Helm chart for scheduled MongoDB backups using Vault for short-lived credentials.

## How it works

### Backup

1. A Kubernetes CronJob triggers on the configured schedule (default: daily at 02:00)
2. An init container (`curlimages/curl`) authenticates to Vault using the Kubernetes service account token
3. Short-lived MongoDB credentials are fetched from Vault's database secrets engine and written to a shared in-memory volume
4. The main container (`mongo`) runs `mongodump --gzip` against the target database
5. The dump is archived with `tar` and old backups beyond the retention period are cleaned up

### Restore

A restore Job is included but disabled by default. When triggered, it:

1. Authenticates to Vault the same way as the backup
2. Extracts the specified (or most recent) backup archive
3. Runs `mongorestore --gzip --drop` to replace existing collections

## Prerequisites

Before installing, the following must be configured in Vault:

### Kubernetes auth role

```bash
vault write auth/kubernetes/role/ror-backup \
  bound_service_account_names=ror-mongodb-backup \
  bound_service_account_namespaces=nhn-ror \
  policies=ror-backup \
  ttl=1h
```

### Database credentials role

```bash
vault write mongodb/roles/ror-backup \
  db_name=nhn-ror \
  creation_statements='{ "db": "admin", "roles": [{ "role": "readAnyDatabase", "db": "admin" }, { "role": "backup", "db": "admin" }] }' \
  default_ttl=15m \
  max_ttl=1h
```

### Vault policy

```hcl
path "mongodb/creds/ror-backup" {
  capabilities = ["read"]
}

path "auth/token/revoke-self" {
  capabilities = ["update"]
}
```

## Installation

```bash
helm install ror-mongodb-backup ./charts/ror-mongodb-backup -n nhn-ror
```

With custom values:

```bash
helm install ror-mongodb-backup ./charts/ror-mongodb-backup -n nhn-ror \
  --set schedule="0 3 * * *" \
  --set backup.retentionDays=14 \
  --set backup.storage.size=50Gi
```

## Manual backup

Trigger a one-off backup outside the schedule:

```bash
kubectl create job --from=cronjob/ror-mongodb-backup manual-backup-$(date +%s) -n nhn-ror
```

## Restoring from backup

### Option 1: In-cluster restore Job (recommended)

The chart includes a restore Job that uses Vault credentials automatically.

**Restore the most recent backup:**

```bash
helm upgrade ror-mongodb-backup ./charts/ror-mongodb-backup -n nhn-ror \
  --set restore.enabled=true
```

**Restore a specific backup:**

```bash
# List available backups
kubectl exec -it deploy/any-pod-with-pvc -- ls -lh /backup/

# Restore a specific one
helm upgrade ror-mongodb-backup ./charts/ror-mongodb-backup -n nhn-ror \
  --set restore.enabled=true \
  --set restore.backup="20260611-124346.tar.gz"
```

**After the restore completes, disable the restore Job:**

```bash
helm upgrade ror-mongodb-backup ./charts/ror-mongodb-backup -n nhn-ror \
  --set restore.enabled=false
```

> **Warning:** The restore uses `--drop` which replaces existing collections. Make sure you have a recent backup before restoring.

**Check restore status:**

```bash
kubectl get job -n nhn-ror -l app.kubernetes.io/component=restore
kubectl logs -n nhn-ror -l app.kubernetes.io/component=restore --all-containers
```

### Option 2: Manual restore from local machine

```bash
# Copy backup from PVC to local machine
kubectl cp nhn-ror/<backup-pod>:/backup/20260611-020000.tar.gz ./backup.tar.gz

# Extract
tar xf backup.tar.gz

# Restore (backups use --gzip, so mongorestore needs it too)
mongorestore --host=<host> --port=27017 \
  --username=<user> --password=<pass> \
  --authenticationDatabase=admin \
  --db=nhn-ror \
  --gzip --drop \
  ./20260611-020000/nhn-ror/
```

## Configuration

| Parameter | Description | Default |
|---|---|---|
| `schedule` | Cron schedule expression | `0 2 * * *` |
| `mongodb.host` | MongoDB hostname | `ror-mongodb.nhn-ror.svc` |
| `mongodb.port` | MongoDB port | `27017` |
| `mongodb.database` | Database to back up | `nhn-ror` |
| `vault.url` | Vault server URL | `http://ror-vault-active.nhn-ror.svc:8200` |
| `vault.role` | Vault role for auth and DB creds | `ror-backup` |
| `vault.authPath` | Vault Kubernetes auth mount path | `kubernetes` |
| `vault.dbMountPath` | Vault database secrets mount path | `mongodb` |
| `backup.storage.size` | PVC size for backup storage | `100Gi` |
| `backup.storage.storageClassName` | Storage class (empty for default) | `""` |
| `backup.retentionDays` | Days to keep old backups | `30` |
| `backup.extraArgs` | Extra arguments passed to mongodump | `""` |
| `image.repository` | Container image | `mongo` |
| `image.tag` | Image tag | `7.0` |
| `serviceAccount.name` | Service account name | `ror-mongodb-backup` |
| `successfulJobsHistoryLimit` | Successful jobs to retain | `3` |
| `failedJobsHistoryLimit` | Failed jobs to retain | `3` |
| `suspend` | Suspend the CronJob | `false` |
| `restore.enabled` | Enable the restore Job | `false` |
| `restore.backup` | Backup filename to restore (empty = latest) | `""` |
