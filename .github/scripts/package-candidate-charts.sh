#!/usr/bin/env bash
set -euo pipefail
source_dir=$1
bundle=$2
[[ "$TARGET_VERSION" =~ ^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$ ]]
[[ "$RC_VERSION" =~ ^${TARGET_VERSION//./\.}-rc\.[1-9][0-9]*$ ]]
export IMAGE_DIGEST
IMAGE_DIGEST=$(node -e 'console.log(require(require("path").resolve(process.argv[1])).indexDigest)' "$bundle/release-image.json")
[[ "$IMAGE_DIGEST" =~ ^sha256:[0-9a-f]{64}$ ]]
export IMAGE_REPOSITORY=ghcr.io/norskhelsenett/ror-api
temporary=$(mktemp -d)
trap 'rm -rf "$temporary"' EXIT
mkdir -p "$bundle/charts"
for version in "${RC_VERSION#v}" "${TARGET_VERSION#v}"; do
  cp -R "$source_dir/charts/ror-api" "$temporary/$version"
  export CHART_VERSION="$version"
  yq -i '.version = strenv(CHART_VERSION) | .appVersion = strenv(TARGET_VERSION)' "$temporary/$version/Chart.yaml"
  yq -i '.image.repository = strenv(IMAGE_REPOSITORY) | .image.digest = strenv(IMAGE_DIGEST) | .image.tag = ""' "$temporary/$version/values.yaml"
  helm lint "$temporary/$version"
  helm template ror-api "$temporary/$version" > "$temporary/$version.yaml"
  rendered_image=$(yq 'select(.kind == "Deployment") | .spec.template.spec.containers[0].image' "$temporary/$version.yaml")
  [[ "$rendered_image" == "$IMAGE_REPOSITORY@$IMAGE_DIGEST" ]]
  yq 'del(.metadata.labels."helm.sh/chart")' "$temporary/$version.yaml" > "$temporary/$version.normalized.yaml"
  helm package "$temporary/$version" --destination "$bundle/charts"
done
diff -u "$temporary/${RC_VERSION#v}.normalized.yaml" "$temporary/${TARGET_VERSION#v}.normalized.yaml"
node - "$bundle/charts" <<'NODE'
const fs = require('node:fs'), path = require('node:path'), crypto = require('node:crypto');
const directory = process.argv[2];
const files = fs.readdirSync(directory).filter(name => name.endsWith('.tgz'));
if (files.length !== 2) throw new Error('expected exactly two chart archives');
const manifest = Object.fromEntries(files.map(name => [name, 'sha256:' + crypto.createHash('sha256').update(fs.readFileSync(path.join(directory, name))).digest('hex')]));
fs.writeFileSync(path.join(directory, 'checksums.json'), JSON.stringify(manifest, null, 2) + '\n');
NODE