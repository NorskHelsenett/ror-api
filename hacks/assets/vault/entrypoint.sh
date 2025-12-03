#!/bin/sh

echo "### Installing prereqs"
apk update && apk add gettext jq curl

echo 
echo "### Starting server"

mkdir -p /vault/data
mkdir -p /vault/secret
vault server -config /policies/vault-config.hcl &

sleep 2s
export VAULT_ADDR="http://$(hostname):8200"
VAULT_INIT_STATUS=$(vault status -format=json | jq .initialized)

if [ "$VAULT_INIT_STATUS" = "false" ]; then
  echo "Vault is not initialized"
  vault operator init -key-shares=1 -key-threshold=1 -format=json > /vault/data/init.json
fi
VAULT_INIT_STATUS=$(vault status -format=json | jq .sealed)
if [ "$VAULT_INIT_STATUS" = "true" ]; then
  echo "Vault is sealed"
  vault operator unseal $(jq -r .unseal_keys_b64[0] /vault/data/init.json)
fi
export VAULT_ROOT_TOKEN=$(jq -r .root_token /vault/data/init.json)

MONGODB_PORT=27017

RABBITMQ_CONSOLE_PORT=15672

echo
echo "## Determining network"
nc -w 1 -v  mongodb ${MONGODB_PORT}
if [ "$?" -ne 0 ]; then
  echo "Mongodb set up using host network"
  MONGODB_HOST=host.docker.internal
else
  echo "Mongodb set up using docker network"
  MONGODB_HOST=mongodb
fi

nc -w 1 -v  rabbitmq ${RABBITMQ_CONSOLE_PORT}
if [ "$?" -ne 0 ]; then
  echo "Rabbitmq set up using host network"
  RABBITMQ_HOST=host.docker.internal

else
  echo "Rabbitmq set up using docker network"
  RABBITMQ_HOST=rabbitmq
fi

echo
echo "## Authenticating to vault"
vault login -no-print "${VAULT_ROOT_TOKEN}"

echo "## Writing policies"

vault policy write ror-api /policies/ror-api.hcl
vault policy write ror-ms-audit /policies/ror-ms-audit.hcl
vault policy write ror-ms-auth /policies/ror-ms-auth.hcl 
vault policy write ror-ms-nhn /policies/ror-ms-nhn.hcl
vault policy write ror-ms-slack /policies/ror-ms-slack.hcl
vault policy write ror-ms-switchboard /policies/ror-ms-switchboard.hcl
vault policy write ror-ms-tanzu /policies/ror-ms-tanzu.hcl
vault policy write ror-ms-tanzu-auth /policies/ror-ms-tanzu-auth.hcl
vault policy write ror-tanzu-agent /policies/ror-tanzu-agent.hcl
vault policy write ror-ms-kind /policies/ror-ms-kind.hcl
vault policy write ror-ms-vulnerability /policies/ror-ms-vulnerability.hcl
vault policy write ror-ms-talos /policies/ror-ms-talos.hcl

echo "Enabling app role"
vault auth enable approle

echo "## Setting up mongodb"

vault secrets enable -path=mongodb database

echo "# Setting secrets for mongodb"

echo "Creating a role for ror-api"
vault write mongodb/roles/ror-api \
    db_name=mongo-connection \
    creation_statements='{ "db": "nhn-ror", "roles": [{ "role": "roleRorApi"}] }' \
    default_ttl="1h" \
    max_ttl="24h"

vault write mongodb/roles/ror-ms-switchboard \
    db_name=mongo-connection \
    creation_statements='{ "db": "nhn-ror", "roles": [{ "role": "roleRorMs" }] }' \
    default_ttl="1h" \
    max_ttl="24h"

echo "Creating a role for ror-ms-audit"
vault write mongodb/roles/ror-ms-audit \
    db_name=mongo-connection \
    creation_statements='{ "db": "nhn-ror", "roles": [{ "role": "roleRorMs" }] }' \
    default_ttl="1h" \
    max_ttl="24h"

echo "Setting up connection to MongoDB"
echo "Creating a role for ror-ms-nhn"
vault write mongodb/roles/ror-ms-nhn \
    db_name=mongo-connection \
    creation_statements='{ "db": "nhn-ror", "roles": [{ "role": "roleRorMs" }] }' \
    default_ttl="1h" \
    max_ttl="24h"

echo "Creating a role for ror-ms-tanzu"
vault write mongodb/roles/ror-ms-tanzu \
    db_name=mongo-connection \
    creation_statements='{ "db": "nhn-ror", "roles": [{ "role": "roleRorMs" }] }' \
    default_ttl="1h" \
    max_ttl="24h"

echo "Creating a role for ror-ms-tanzu-auth"
vault write mongodb/roles/ror-ms-tanzu-auth \
    db_name=mongo-connection \
    creation_statements='{ "db": "nhn-ror", "roles": [{ "role": "roleRorMs" }] }' \
    default_ttl="1h" \
    max_ttl="24h"

echo "Creating a role for superadmin"
vault write mongodb/roles/superadmin \
    db_name=mongo-connection \
    creation_statements='{ "db": "admin", "roles": ["role": "root"] }' \
    default_ttl="1h" \
    max_ttl="24h"

vault write mongodb/config/mongo-connection \
      plugin_name=mongodb-database-plugin \
      allowed_roles="ror-api,ror-ms-switchboard,ror-ms-audit,ror-ms-nhn,ror-ms-tanzu,ror-ms-tanzu-auth" \
      connection_url="mongodb://{{username}}:{{password}}@${MONGODB_HOST}:${MONGODB_PORT}/" \
      username="someone" \
      password="S3cret!"

echo "## Setting up rabbitmq"
vault secrets enable rabbitmq

echo " - Setting ttl and max_ttl for rabbitmq (ttl: 3600s (1 hour), max_ttl: 86400s (1 day))"
vault write rabbitmq/config/lease \
    ttl=3600 \
    max_ttl=86400

vault write rabbitmq/config/connection \
    connection_uri="http://${RABBITMQ_HOST}:${RABBITMQ_CONSOLE_PORT}" \
    allowed_roles="ror-api" \
    username="admin" \
    password="S3cret!"

echo
echo "# Create valkey roles"

vault secrets enable -path=database database

vault write database/config/valkey \
  plugin_name="redis-database-plugin" \
  host="valkey" \
  port=6379 \
  tls=false \
  username="default" \
  password="S3cret!" \
  allowed_roles="valkey-*-role"

vault write database/roles/valkey-ror-api-role \
    db_name="valkey" \
    creation_statements='["~*", "+@string", "+PING"]' \
    default_ttl="5m" \
    max_ttl="1h"

vault write database/roles/valkey-ror-ms-tanzu-role \
    db_name="valkey" \
    creation_statements='["~*", "+@string", "+PING"]' \
    default_ttl="5m" \
    max_ttl="1h"

vault write database/roles/valkey-ror-ms-tanzu-auth-role \
    db_name="valkey" \
    creation_statements='["~*", "+@string", "+PING"]' \
    default_ttl="5m" \
    max_ttl="1h"

echo
echo "# Create rabbitmq roles"
vault write rabbitmq/roles/ror-api \
    vhosts='{"/":{"configure":".*","write": ".*", "read": ".*"}}'

echo "Create rabbitmq role for nhn micro service"
vault write rabbitmq/roles/ror-ms-nhn \
    vhosts='{"/":{"configure":".*","write": ".*", "read": ".*"}}'

echo "Create rabbitmq role for auth micro service"
vault write rabbitmq/roles/ror-ms-auth \
    vhosts='{"/":{"configure":".*","write": ".*", "read": ".*"}}'

echo "Create rabbitmq role for slack micro service"
vault write rabbitmq/roles/ror-ms-slack \
    vhosts='{"/":{"configure":".*","write": ".*", "read": ".*"}}'

echo "Create rabbitmq role for switchboard micro service"
vault write rabbitmq/roles/ror-ms-switchboard \
    vhosts='{"/":{"configure":".*","write": ".*", "read": ".*"}}'

echo "Create rabbitmq role for audit micro service"
vault write rabbitmq/roles/ror-ms-audit \
    vhosts='{"/":{"configure":".*","write": ".*", "read": ".*"}}'

echo "Create rabbitmq role for tanzu auth micro service"
vault write rabbitmq/roles/ror-ms-tanzu-auth \
    vhosts='{"/":{"configure":".*","write": ".*", "read": ".*"}}'

echo "Create rabbitmq role for tanzu micro service"
vault write rabbitmq/roles/ror-ms-tanzu \
    vhosts='{"/":{"configure":".*","write": ".*", "read": ".*"}}'

echo "Create rabbitmq role for tanzu agent"
vault write rabbitmq/roles/ror-tanzu-agent \
    vhosts='{"/":{"configure":".*","write": ".*", "read": ".*"}}'

echo "Create rabbitmq role for kind micro service"
vault write rabbitmq/roles/ror-ms-kind \
    vhosts='{"/":{"configure":".*","write": ".*", "read": ".*"}}'

echo "Create rabbitmq role for vulnerability micro service"
vault write rabbitmq/roles/ror-ms-vulnerability \
    vhosts='{"/":{"configure":".*","write": ".*", "read": ".*"}}'

echo "Create rabbitmq role for talos micro service"
vault write rabbitmq/roles/ror-ms-talos \
    vhosts='{"/":{"configure":".*","write": ".*", "read": ".*"}}'

echo
echo "## Enabeling KV secrets engine"
vault secrets enable -path=secret kv-v2

echo
echo "## Seeding data"

echo "Adding dex placholder"
vault kv put secret/v1.0/ror/dex/dummy dummy="placeholder"  > /dev/null

echo "Adding password for argocd sdi password"
vault kv put secret/v1.0/ror/config/common argocdSdiPassword="$ARGOCD_SDI_PASSWORD"  > /dev/null

echo "Adding access token for helsegitlab"
vault kv patch secret/v1.0/ror/config/common helsegitlabToken="$HELSEGITLAB_ACCESSTOKEN" > /dev/null

echo "Adding access token for splunkHecToken"
vault kv patch secret/v1.0/ror/config/common splunkHecToken="" > /dev/null

echo "Adding apikeySalt"
vault kv patch secret/v1.0/ror/config/common apikeySalt="1318918c-0071-4013-982b-1780e555ae7a" > /dev/null

echo "Adding ms-slack credentials"
vault kv put secret/v1.0/ror/config/ms-slack app_token=app123 bot_token=bot123 > /dev/null

echo "Adding password for argocd sdi password"
vault kv put secret/v1.0/ror/tanzu/agent tanzuUsername="$TANZU_USERNAME"  > /dev/null
vault kv patch secret/v1.0/ror/tanzu/agent tanzuPassword="$TANZU_PASSWORD"  > /dev/null

echo "Adding access token for helsegitlab for Audit micro service"
vault kv put secret/v1.0/ror/config/ms-audit helsegitlabAclToken="$HELSEGITLAB_ACL_ACCESSTOKEN" aclRepoNumber=620 branch=main > /dev/null

echo "Adding open ldap config"
vault kv put -format=json secret/v1.0/ror/config/auth @/vault/config/ldapconfig.json  > /dev/null

echo
echo "# INITIALIZATION COMPLETE"
echo "ROOT TOKEN: ${VAULT_ROOT_TOKEN}"

sed "s|^VAULT_TOKEN=.*|VAULT_TOKEN=\"${VAULT_ROOT_TOKEN}\"|" /vault/config/envfile > /tmp/envfile.tmp
cat /tmp/envfile.tmp > /vault/config/envfile
rm /tmp/envfile.tmp


# This container is now healthy
touch /tmp/healthy

# Keep container alive
tail -f /dev/null & trap 'kill %1' TERM ; wait
