# Test-gated release candidates

Implementation status: local workflow/helper implementation and private bundle
rehearsal tested. No production RC publication, GitHub state-branch write, or new
workflow dispatch has been performed. Review and merge before using it remotely.
Approval-gated final promotion and automatic post-merge integration are separate
pending stages. The previous tag-triggered publisher has been removed locally;
after merge, direct version-tag pushes no longer publish anything.

## Operator steps

1. Ensure the selected API commit is on main and contains digest-capable chart
   templates and the required published shared-library dependency.
2. Open **Actions -> Release Candidate -> Run workflow**, on `main`.
3. Set `target_version` (for example `v1.26.0`) and optionally `api_ref` (a commit
   on main history). Leave `publish=false` for the first rehearsal.
4. Inspect reservation, build, Helm checks, amd64 test results and verified handoff.
   A rehearsal consumes an RC number but publishes no image/chart/tag/release.
5. When publication is authorized, start a new run with the desired source and
   target version and `publish=true`. This is explicit permission to expose an RC
   to CD after tests pass. It does not bypass any build/test/packaging check.
6. Verify the RC image digest, chart, prerelease and evidence links. Stable
   `latest` and the final version remain unchanged. There is no mutable RC alias:
   CD should select immutable `vX.Y.Z-rc.N` tags/chart versions by SemVer.

The source and desired version are resolved once. Both architecture binaries are
built with the **final** target version and actual source SHA. The candidate tag
is external metadata. A passing `v1.26.0-rc.2` contains binary version `v1.26.0`.

## Job sequence and permissions

```text
reserve (internal ledger only)
  -> test: reusable candidate-handoff
       prepare -> build both architectures + package both charts
               -> ror-test: 34 scenarios on amd64
               -> verify-handoff
  -> publish-rc (only when publish=true and all required jobs succeeded)
       verify bytes -> bind digests -> push immutable RC -> sign -> prerelease
```

Build/test jobs have only read permissions. The reservation job has contents-write
for its dedicated ledger; it has no registry write permission and creates no RC
Git tags. Only the gated publisher has package-write permission. No workflow here
has cluster credentials or deployment commands. CD can act as soon as a passing
RC image/chart appears, so every publication write comes after all test gates.
Cross-registry/GitHub publication is not atomic: a registry RC can exist even if
a later signature, prerelease, or attachment operation fails. It is still tested;
the workflow must not report completed publication until all operations succeed.

## Sequential numbering and state

The workflow creates branch `ror-release-state` if needed, with `release-state.json`.
It is an internal control ledger, **not confidential storage** (the API repository
is public). It contains versions, run IDs, source/workflow/harness SHAs, attempt
numbers, and publication bindings; no tokens or production data.

- Each target version starts at `rc.1`. New workflow runs reserve increasing numbers.
- Retries of one run reuse the reservation and cannot change its source/workflow SHA.
- Failed or rehearsal-only runs may leave gaps; numbers are never deliberately reused.
- A newer reservation supersedes older attempts for that target. An older slow build
  cannot publish after a newer attempt was reserved, even if the newer one fails.
- State allocation uses GitHub contents SHA compare-and-swap with bounded conflict
  retries. Reservation and publication jobs share a concurrency group; neither
  cancels a running state operation. GitHub scheduling is not a FIFO release queue.
- Do not delete/reset/force-push the ledger branch or edit its counters manually.
  Protect it against human deletion and forced updates while permitting the trusted
  workflow's normal updates. Ledger repair after corruption is an operator task.

Production final-version reuse is rejected when a final Git tag exists, when the
ledger marks it final, or before RC push when a final image/chart tag exists.
Successful final promotion will need to update this ledger in its own gated path.

## Exact tested bytes

The build produces a full multi-platform `release.tar` OCI archive. The selected
amd64 manifest is also exposed in `candidate.tar` by rewriting only the archive's
root index; manifest, configuration and layer bytes are unchanged. This supports
the existing pinned harness without depending on Docker's multi-platform image-ID
behavior. Both archives remain Actions artifacts until tests succeed.

The harness tests `candidate.tar`. Before external publication, the API verifier
checks `release.tar` against its checksum, validates both platform manifests,
and requires its amd64 manifest/config digests to equal the tested ones. Arm64
must build and have valid metadata, but is **not integration-tested**.

Skopeo copies the release archive with `--all --preserve-digests`; the resulting
RC tag must resolve to the recorded release index digest. The API is not rebuilt
in the publishing job. Existing RC image tags are accepted only if their digests
match. Registry lookup failures, including authorization failures, abort rather
than being treated as missing tags. The image and chart repositories must already
be provisioned and accessible for tag listing; bootstrap is not automated here.

Helm packages:

| Artifact | Version | appVersion | Image |
|---|---|---|---|
| RC chart | `X.Y.Z-rc.N` | `vX.Y.Z` | Verified multi-platform image index digest |
| Prepared final chart | `X.Y.Z` | `vX.Y.Z` | Same digest |

Both charts are linted/rendered before integration completes. Rendered resources
must match after removing the top-level `helm.sh/chart` label. Additional differences
fail packaging; they are not silently normalized. Existing tag-only values retain
their fallback behavior. The final chart is an artifact/prerelease attachment, not
a published final OCI chart. The archive checksum is rechecked before publishing;
existing RC chart tags/assets must contain identical bytes, never overwritten.

## Retries and recovery

Before publication is bound, rerun **all jobs** to produce internally consistent
run-attempt evidence. Failed-job-only retries may mix evidence from different
attempts and are intentionally rejected.

Once publishing begins, the ledger binds the image archive/index/configuration,
tested image, report digest, build artifact ID and both chart checksums. A retry
cannot replace those values. Already-existing identical image/chart/tag/assets
are verified and reused; conflicting ones stop the run without overwrite.

Rebuilding a bound attempt can change archive bytes, artifact IDs or report hashes.
Such reruns fail closed rather than resuming with different content. Do not force
an overwrite or reset the ledger. Prefer a **new run and next RC number** after
reviewing any partially published passing RC. A dedicated publish-only recovery
workflow is not implemented. Final-version promotion is also not implemented yet.

## Validation and rollout

Local checks:

```sh
node --test .github/scripts/*.test.cjs
PYTHONDONTWRITEBYTECODE=1 python3 .github/scripts/test_split_oci.py
TARGET_VERSION=v0.0.0 RC_VERSION=v0.0.0-rc.1 bash .github/scripts/rehearse-candidate.sh
```

The rehearsal builds both platforms, validates RC/final charts, runs 34 amd64
scenarios using the exact pinned harness in a disposable checkout, and rechecks
the release bundle. It changes no remote state and publishes nothing. On arm64
machines, running the amd64 API requires configured Docker emulation. Dependencies
are resolved with `GOWORK=off` and `-mod=readonly`; no `go get` or local shared
module replacement is used. Builds use committed generated code, not regeneration.

Verified locally: sequential allocation/supersession and conflict mocks, publication
permission/dependency checks, registry failure/no-overwrite tests, asset retry
tests, archive selection, chart lint/render/equivalence, and the private dual-platform
bundle rehearsal (34/34 amd64 scenarios). Actual GHCR writes, cosign signing,
GitHub state-branch permissions and prerelease creation remain unverified remotely.

Before the first `publish=true`:

1. Review and merge the old publisher removal together with the new RC workflow.
2. Confirm Actions permissions, GHCR package ownership and ledger branch controls.
3. Run `publish=false` on main with a source that includes the chart change.
4. Rehearse failed tests, state supersession and partial writes in an isolated
   repository/registry not watched by CD. Registry destinations are intentionally
   fixed in the production script; review a rehearsal fork instead of redirecting
   an authorized production run through untrusted inputs.
5. Authorize the first actual RC explicitly. Do not use a fake version in the real
   repository merely to test publication: CD watches these artifacts.

The `RC workflow` README badge reports the latest workflow result, which may be a
non-publishing rehearsal. It is not proof that a particular RC exists or is eligible
for final promotion. Use the immutable candidate record and per-run evidence.