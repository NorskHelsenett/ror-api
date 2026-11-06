#!/bin/sh

# Preserve local overrides and the credential across Compose restarts.
set_local_default() {
    key=$1
    value=$2
    if ! grep -Eq "^${key}=.+" "$envfile" ||
        grep -Eq "^${key}=(\"\"|'')$" "$envfile"; then
        sed "/^${key}=/d" "$envfile" > "$tmpfile"
        printf '%s="%s"\n' "$key" "$value" >> "$tmpfile"
        cat "$tmpfile" > "$envfile"
    fi
}

configure_local_snapshot() (
    set -eu
    umask 077
    envfile=$1
    chmod 600 "$envfile"
    tmpfile=$(mktemp)
    trap 'rm -f "$tmpfile"' EXIT

    # Never print the generated development credential into container logs.
    credential=$(od -An -N32 -tx1 /dev/urandom | tr -d ' \n')
    [ "${#credential}" -eq 64 ]
    set_local_default DEV_ACL_SNAPSHOT_API_KEY "$credential"
)

configure_local_snapshot "$1"
