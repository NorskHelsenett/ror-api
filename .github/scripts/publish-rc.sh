#!/usr/bin/env bash
set -euo pipefail
bundle=$1
[[ "$RC_VERSION" =~ ^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)-rc\.[1-9][0-9]*$ ]] || exit 2
[[ "$INDEX_DIGEST" =~ ^sha256:[0-9a-f]{64}$ ]] || exit 2
target_version=${RC_VERSION%-rc.*}
image=ghcr.io/norskhelsenett/ror-api
chart_repository=ghcr.io/norskhelsenett/helm/ror-api
temporary=$(mktemp -d)
trap 'rm -rf "$temporary"' EXIT
has_tag() {
  skopeo list-tags "docker://$1" > "$temporary/tags.json" || return "$?"
  node - "$temporary/tags.json" "$2" <<'NODE'
const fs = require('node:fs');
const tags = JSON.parse(fs.readFileSync(process.argv[2])).Tags;
if (!Array.isArray(tags)) throw new Error('registry did not return a tag list');
process.exit(tags.includes(process.argv[3]) ? 0 : 3);
NODE
}
require_absent() {
  if has_tag "$1" "$2"; then
    printf 'Final version already exists; refusing new RC publication\n' >&2
    exit 1
  else
    local code=$?
    [[ "$code" == 3 ]] || exit "$code"
  fi
}
require_absent "$image" "$target_version"
require_absent "$chart_repository" "${target_version#v}"
image_exists=false
verify_image() {
  local actual
  actual=$(skopeo inspect --format '{{.Digest}}' "docker://$image:$RC_VERSION")
  if [[ "$actual" != "$INDEX_DIGEST" ]]; then
    printf 'Refusing RC image with unexpected digest\n' >&2
    exit 1
  fi
}
if has_tag "$image" "$RC_VERSION"; then
  image_exists=true
else
  code=$?
  [[ "$code" == 3 ]] || exit "$code"
fi
if [[ "$image_exists" == true ]]; then
  verify_image
else
  skopeo copy --all --preserve-digests "oci-archive:$bundle/release.tar" "docker://$image:$RC_VERSION"
fi
verify_image
chart_version=${RC_VERSION#v}
chart_file="$bundle/charts/ror-api-$chart_version.tgz"
chart_exists=false
if has_tag "$chart_repository" "$chart_version"; then
  chart_exists=true
else
  code=$?
  [[ "$code" == 3 ]] || exit "$code"
fi
if [[ "$chart_exists" == false ]]; then
  helm push "$chart_file" oci://ghcr.io/norskhelsenett/helm
fi
helm pull "oci://$chart_repository" --version "$chart_version" --destination "$temporary"
cmp "$chart_file" "$temporary/ror-api-$chart_version.tgz"
printf 'Published and verified immutable RC image and chart: %s\n' "$RC_VERSION"