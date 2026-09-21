#!/usr/bin/env bash
set -euo pipefail
umask 077
root=$(cd "$(dirname "$0")/../.." && pwd)
harness=${ROR_TEST_PATH:-"$(dirname "$root")/ror-test"}
temporary=$(mktemp -d)
trap 'rm -rf "$temporary"' EXIT
export GOWORK=off GOFLAGS=-mod=readonly
export TARGET_VERSION=${TARGET_VERSION:-v0.0.0}
export SOURCE_SHA
SOURCE_SHA=$(git -C "$root" rev-parse HEAD)
export GITHUB_REPOSITORY=NorskHelsenett/ror-api
export GITHUB_RUN_ID
GITHUB_RUN_ID=$(date +%s)
export GITHUB_RUN_ATTEMPT=1
export HANDOFF_ROOT="$root" HANDOFF_OUTPUT="$temporary"
node <<'NODE'
const fs = require('node:fs');
const { prepareCandidate } = require(process.env.HANDOFF_ROOT + '/.github/scripts/candidate-handoff.cjs');
const expected = prepareCandidate({
  targetVersion: process.env.TARGET_VERSION, sourceSHA: process.env.SOURCE_SHA,
  runID: process.env.GITHUB_RUN_ID, runAttempt: process.env.GITHUB_RUN_ATTEMPT,
  repository: process.env.GITHUB_REPOSITORY,
});
fs.writeFileSync(process.env.HANDOFF_OUTPUT + '/candidate-request.json', JSON.stringify(expected));
NODE
test "$(git -C "$harness" rev-parse HEAD)" = "$(node -e 'console.log(require(process.argv[1]).HARNESS_SHA)' "$root/.github/scripts/candidate-handoff.cjs")"
test -z "$(git -C "$harness" status --porcelain)"
go -C "$root" list -m -json github.com/NorskHelsenett/ror > "$temporary/shared-module.json"
LIB_VER=$(node -e 'const fs=require("fs"),assert=require("assert"),module=JSON.parse(fs.readFileSync(process.argv[1])); assert(!module.Replace); console.log(module.Version);' "$temporary/shared-module.json")
mkdir -p "$temporary/context/dist"
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go -C "$root" build -v \
  -ldflags "-w -extldflags '-static' -X github.com/NorskHelsenett/ror/pkg/config/rorversion.Version=$TARGET_VERSION -X github.com/NorskHelsenett/ror/pkg/config/rorversion.Commit=$SOURCE_SHA -X github.com/NorskHelsenett/ror/pkg/config/rorversion.LibVer=$LIB_VER" \
  -o "$temporary/context/dist/ror-api-linux-amd64" ./cmd/api
chmod 0755 "$temporary/context/dist/ror-api-linux-amd64"
go version -m "$temporary/context/dist/ror-api-linux-amd64" > "$temporary/build.txt"
cp "$root/Dockerfile" "$temporary/context/Dockerfile"
docker buildx build --platform linux/amd64 --provenance=false --sbom=false \
  --output "type=oci,dest=$temporary/candidate.tar" "$temporary/context"
export E2E_CANDIDATE_ARCHIVE="$temporary/candidate.tar"
export E2E_CANDIDATE_PLATFORM=linux/amd64
export E2E_CANDIDATE_SHA256 E2E_CANDIDATE_DIGEST
E2E_CANDIDATE_SHA256="sha256:$(shasum -a 256 "$temporary/candidate.tar" | awk '{print $1}')"
E2E_CANDIDATE_DIGEST="sha256:$(tar -xOf "$temporary/candidate.tar" index.json | shasum -a 256 | awk '{print $1}')"
go -C "$harness" build -o "$temporary/verifier" ./cmd/e2e
"$temporary/verifier" inspect-candidate -archive "$temporary/candidate.tar" \
  -checksum "$E2E_CANDIDATE_SHA256" -digest "$E2E_CANDIDATE_DIGEST" \
  -platform linux/amd64 -out "$temporary/candidate-image.json"
COMPOSE_PROGRESS=plain bash "$harness/testenv/run.sh" run | tee "$temporary/run.log"
result=$(sed -n 's/^Reports: //p' "$temporary/run.log")
test -d "$result"
mkdir -p "$temporary/isolated/artifacts" "$temporary/isolated/testenv/scenarios"
cp -R "$result" "$temporary/isolated/artifacts/"
cp "$harness/testenv/scenarios/acl.json" "$temporary/isolated/testenv/scenarios/"
cp "$harness/testenv/seed.js" "$harness/testenv/oidc.json" "$temporary/isolated/testenv/"
export EXPECTED_ARCHIVE="$E2E_CANDIDATE_SHA256" EXPECTED_INDEX="$E2E_CANDIDATE_DIGEST" EXPECTED_PLATFORM=linux/amd64
export EXPECTED_MANIFEST
EXPECTED_MANIFEST=$(node -e 'console.log(require(process.argv[1]).manifestDigest)' "$temporary/candidate-image.json")
export EXPECTED_HARNESS
EXPECTED_HARNESS=$(git -C "$harness" rev-parse HEAD)
export INPUT_ARTIFACT_ID=1 BASELINE_IMAGE=''
node "$harness/.github/scripts/verify-candidate-evidence.cjs" "$temporary/isolated"
pushd "$temporary" >/dev/null
node "$root/.github/scripts/candidate-handoff.cjs" "$temporary" "$temporary/isolated/artifacts" "$harness" "$INPUT_ARTIFACT_ID"
popd >/dev/null
cp "$temporary/handoff-result.json" "$temporary/build.txt" "$temporary/candidate-request.json" "$result/"
printf 'Local amd64 handoff rehearsal passed. Reports: %s\n' "$result"