const test = require('node:test');
const assert = require('node:assert/strict');
const { reserve, assertEligible, verifyBuild, allocate } = require('./release-candidate.cjs');
const request = { targetVersion: 'v1.26.0', sourceSHA: 'a'.repeat(40), workflowSHA: 'b'.repeat(40), runID: '100' };

test('sequential per-version reservations are immutable and rerun-idempotent', () => {
  const first = reserve({ schema: 1, versions: {} }, request);
  assert.equal(first.candidate.candidateVersion, 'v1.26.0-rc.1');
  assertEligible(first.state, first.candidate);
  assert.deepEqual(reserve(first.state, request), first);
  assert.throws(() => reserve(first.state, { ...request, sourceSHA: 'c'.repeat(40) }));
  const second = reserve(first.state, { ...request, sourceSHA: 'c'.repeat(40), runID: '101' });
  assert.equal(second.candidate.candidateVersion, 'v1.26.0-rc.2');
  assert.throws(() => assertEligible(second.state, first.candidate));
  assertEligible(second.state, second.candidate);
  const other = reserve(second.state, { ...request, targetVersion: 'v1.27.0', runID: '102' });
  assert.equal(other.candidate.number, 1);
  second.state.versions[request.targetVersion].final = true;
  assert.throws(() => reserve(second.state, { ...request, runID: '103' }));
});

test('publication requires verified metadata for the reserved candidate', () => {
  const { candidate } = reserve({ schema: 1, versions: {} }, request);
  const digest = 'sha256:' + 'a'.repeat(64);
  const result = { ...candidate, schema: 1, rehearsal: true, published: false, result: 'passed', binaryVersion: candidate.targetVersion, platform: 'linux/amd64', passed: 34, reportDigest: digest, candidate: { platform: 'linux/amd64', archiveSha256: digest, indexDigest: digest, manifestDigest: digest, configDigest: digest } };
  assert.equal(verifyBuild(candidate, result), result.candidate);
  for (const patch of [{ result: 'failed' }, { published: true }, { sourceSHA: 'c'.repeat(40) }, { harnessSHA: 'c'.repeat(40) }, { runID: '999' }, { passed: 0 }, { binaryVersion: candidate.candidateVersion }]) {
    assert.throws(() => verifyBuild(candidate, { ...result, ...patch }));
  }
});

test('allocator retries conflicting state writes without creating public tags', async () => {
  const calls = [];
  let writes = 0;
  const github = { rest: {
    git: { getRef: async args => { calls.push(args.ref); if (args.ref.startsWith('tags/')) throw { status: 404 }; } },
    repos: {
      getCommit: async () => ({ data: { sha: request.sourceSHA } }),
      compareCommitsWithBasehead: async () => ({ data: { status: 'ahead' } }),
      getContent: async () => { throw { status: 404 }; },
      createOrUpdateFileContents: async args => { calls.push(args); if (++writes === 1) throw { status: 409 }; },
    },
  } };
  const context = { ref: 'refs/heads/main', repo: { owner: 'NorskHelsenett', repo: 'ror-api' }, sha: request.workflowSHA, runId: 100 };
  const candidate = await allocate({ github, context, targetVersion: request.targetVersion });
  assert.equal(candidate.number, 1);
  assert.equal(writes, 2);
  assert(calls.filter(value => typeof value === 'object').every(value => value.branch === 'ror-release-state'));
  await assert.rejects(() => allocate({ github, context: { ...context, ref: 'refs/heads/feature' }, targetVersion: request.targetVersion }));
});