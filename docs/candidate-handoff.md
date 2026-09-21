# Test-only RC candidate handoff

[Candidate handoff rehearsal](../.github/workflows/candidate-handoff.yml) is a
manual, publication-free check of the ror-api -> ror-test integration. It builds
an RC candidate, tests the exact image, and verifies the evidence returned by the
reusable workflow. It never pushes an image, creates a Git tag, publishes a chart
or GitHub release, updates an RC alias/`latest`, or invokes deployment automation.
Nothing from this workflow is discoverable by RC-tracking CD tools.

## What runs

```text
prepare -> build -> integration (reusable ror-test workflow) -> verify-handoff
```

1. Resolve `api_ref` to a full commit SHA in this repository and validate the final
   target version. An empty ref selects the commit of the dispatched workflow.
2. Create internal candidate identity `vX.Y.Z-rc.<run-id>.<run-attempt>`. This is a
   rehearsal identifier, not the production RC-number allocator and not a Git tag.
3. Build linux/amd64 with `GOWORK=off` and readonly module resolution. The binary
   version is **the final target version**, not the RC identifier; commit and
   library-version ldflags identify the selected API source and pinned dependency.
4. Package the binary using the selected source's API Dockerfile. Export
   `candidate.tar` as OCI with no external push. Upload that archive and expected
   metadata as separate immutable Actions artifacts.
5. Call `NorskHelsenett/ror-test/.github/workflows/candidate-e2e.yml` at
   `199e73ba8875775cc1329faabb7eea6475a02d4c`, with the same harness checkout SHA.
   The reusable job downloads the build artifact from this Actions run, checks
   archive/index/platform digests, runs all 34 scenarios on native amd64, and
   uploads success evidence only after complete verification.
6. Download the expected build metadata and returned evidence by artifact ID.
   Recheck the report against the pinned suite using the Go verifier and the
   API-side identity checks. Require matching repository, run/attempt, source,
   harness, artifact, image digests, fixture hashes, final-version build flags,
   unmodified harness, and all scenario results.
7. Retain `handoff-result.json` with `result: "passed"` and `published: false`.
   This records a successful rehearsal, not permission to publish an RC.

The test gate is amd64-only. This rehearsal builds amd64 only; it does not change
the existing release workflow's build architecture matrix. It builds committed
generated API sources, without generator execution or dependency updates. The
selected API commit must contain the necessary generated code and published
library fixes; the local Go workspace cannot supply missing dependency changes.

The linker flags are checked using `go version -m` output. Like the existing
release workflow, this build does not use `-trimpath`, which omits linker flags
from that output. Binary permissions are explicitly set to `0755` for the
distroless non-root runtime.

## Run in GitHub Actions

The workflow definition must first be available on the repository's default
branch to enable `workflow_dispatch`. After that, a trusted branch containing
the workflow can be selected for rehearsal. The workflow/tooling revision and
the API source ref are separate; only select trusted code because builds execute
the selected source and Dockerfile.

In **ror-api -> Actions -> Candidate handoff rehearsal -> Run workflow**, set:

| Input | Example | Meaning |
|---|---|---|
| `target_version` | `v1.26.0` | Required final binary version; strict `vMAJOR.MINOR.PATCH` |
| `api_ref` | Full commit SHA, branch, tag, or blank | API source; resolved through GitHub before checkout |
| `test_fault` | `none` | Normal passing-handoff rehearsal |

Equivalent CLI after the workflow is published:

```sh
gh workflow run candidate-handoff.yml --repo NorskHelsenett/ror-api \
  --ref main -f target_version=v1.26.0 -f api_ref=<API_COMMIT_SHA> -f test_fault=none
```

Expected: all four jobs succeed; the integration job passes 34/34 scenarios;
`verified-rc-rehearsal-<run-id>-<attempt>` contains the result record. Images,
charts, tags, releases, RC pointers, and stable `latest` remain unchanged.

To test the failure path, start another run with the same target/source and
`test_fault=checksum`. The caller deliberately supplies an incorrect expected
archive checksum. Expected: integration fails before API startup, `verify-handoff`
is skipped, and no verified rehearsal result is uploaded. This workflow failure
is the intended result of that negative test; nothing is published either way.

For a different source commit, start a **new manual run** with the same target
version. Reruns cannot select another commit. Use **Re-run all jobs** for a retry:
the gate binds artifacts and evidence to the current run attempt and deliberately
rejects evidence mixed with a build from a previous attempt.

## Permissions and access

- Caller and called workflows use `contents: read`, `actions: read`, and
  `packages: read`. There are no repository/package write permissions or release
  environment approvals, because there is no publication job.
- If ror-test is private, enable reusable-workflow access for ror-api and set
  `CROSS_REPO_READ_TOKEN` to a scoped read-only credential for the explicit harness
  checkouts. Reusable-workflow access and source-checkout authorization are
  separate requirements. Do not grant Actions write just to call the workflow.
- Keep the workflow `uses` SHA, explicit harness checkouts, and the helper's
  `HARNESS_SHA` constant aligned when updating the harness.
- Build archives and metadata expire after seven days; success evidence/results
  after thirty. Missing or expired inputs require a fresh rehearsal, not fallback
  to mutable artifact names or a different image.

## Local verification

From ror-api:

```sh
node --test .github/scripts/candidate-handoff.test.cjs
TARGET_VERSION=v0.0.0 bash .github/scripts/rehearse-candidate.sh
```

The local script requires Docker Buildx, OCI/containerd image loading, Go, Node,
and a clean sibling ror-test checkout at the pinned SHA (`ROR_TEST_PATH` overrides
its location). On an arm64 workstation, amd64 emulation must already work. It
builds the API's local working tree using published module dependencies, so source
files must match HEAD for accurate source provenance. Local rehearsal uses
synthetic run/artifact IDs and does not verify GitHub's transport or permissions.

The script preserves the loaded input image and the test reports in ror-test's
ignored artifacts directory, removes its temporary build/archive data, and never
pushes or creates a release. Its final result includes the successful API-side
evidence verification. Unit tests separately reject failed/incomplete evidence,
wrong commits/digests/artifacts/runs, modified fixtures, and RC-version build flags.

## Validation status and next gate

Locally verified on 2026-09-21: helper tests, workflow lint, published harness pin,
and a complete linux/amd64 OCI rehearsal under Docker emulation on arm64. All 34
scenarios and both evidence verifiers passed, including exact final-version flags.

Still pending: publish the API caller and run both normal and checksum-failure
cases in GitHub on native amd64. No remote Actions run was triggered by this
implementation. The current tag-triggered production release workflow remains
unchanged; disable that bypass before implementing real RC tags/publication.
Successful rehearsal alone does not enable RC publication or final promotion.