#!/usr/bin/env bash
set -euo pipefail
chart=$1
[[ "$FINAL_VERSION" =~ ^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$ ]] || exit 2
[[ "$INDEX_DIGEST" =~ ^sha256:[0-9a-f]{64}$ ]] || exit 2
[[ "$CHART_DIGEST" =~ ^sha256:[0-9a-f]{64}$ ]] || exit 2
[[ "$UPDATE_LATEST" == "true" || "$UPDATE_LATEST" == "false" ]] || exit 2
image=ghcr.io/norskhelsenett/ror-api
chart_repository=ghcr.io/norskhelsenett/helm/ror-api
chart_version=${FINAL_VERSION#v}
temporary=$(mktemp -d)
trap 'rm -rf "$temporary"' EXIT

actual_chart="sha256:$(sha256sum "$chart" | awk '{print $1}')"
if [[ "$actual_chart" != "$CHART_DIGEST" ]]; then
  printf 'Refusing chart that differs from the tested candidate\n' >&2
  exit 1
fi

digest_of() {
  skopeo inspect --format '{{.Digest}}' "docker://$1" 2>/dev/null
}

# The version tag is immutable: publish it once, never repoint it.
current=$(digest_of "$image:$FINAL_VERSION" || true)
if [[ -z "$current" ]]; then
  skopeo copy --all --preserve-digests "docker://$image@$INDEX_DIGEST" "docker://$image:$FINAL_VERSION"
  current=$(digest_of "$image:$FINAL_VERSION")
fi
if [[ "$current" != "$INDEX_DIGEST" ]]; then
  printf 'Refusing to repoint existing %s from %s\n' "$FINAL_VERSION" "$current" >&2
  exit 1
fi

has_chart() {
  skopeo list-tags "docker://$chart_repository" > "$temporary/tags.json" || return "$?"
  node - "$temporary/tags.json" "$chart_version" <<'NODE'
const fs = require('node:fs');
const tags = JSON.parse(fs.readFileSync(process.argv[2])).Tags;
if (!Array.isArray(tags)) throw new Error('registry did not return a tag list');
process.exit(tags.includes(process.argv[3]) ? 0 : 3);
NODE
}

chart_exists=false
if has_chart; then
  chart_exists=true
else
  code=$?
  [[ "$code" == 3 ]] || exit "$code"
fi
if [[ "$chart_exists" == false ]]; then
  helm push "$chart" oci://ghcr.io/norskhelsenett/helm
fi
helm pull "oci://$chart_repository" --version "$chart_version" --destination "$temporary"
cmp "$chart" "$temporary/ror-api-$chart_version.tgz"

# Moving the stable pointer is the last and only mutable write.
if [[ "$UPDATE_LATEST" == "true" ]]; then
  skopeo copy --all --preserve-digests "docker://$image@$INDEX_DIGEST" "docker://$image:latest"
  moved=$(digest_of "$image:latest")
  [[ "$moved" == "$INDEX_DIGEST" ]] || exit 1
  printf 'Promoted %s and moved latest to the tested image\n' "$FINAL_VERSION"
else
  printf 'Promoted %s; latest still points at a newer release\n' "$FINAL_VERSION"
fi
