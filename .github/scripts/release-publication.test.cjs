const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const crypto = require('node:crypto');
const { publishReleaseRecord } = require('./publish-release-record.cjs');
const { verifyCharts } = require('./verify-rc-bundle.cjs');
const { bindPublication, reserve } = require('./release-candidate.cjs');

test('only successful opted-in RC workflows can publish; legacy tags cannot bypass tests', () => {
  const workflow = fs.readFileSync(path.join(__dirname, '../workflows/release-candidate.yml'), 'utf8');
  assert.match(workflow, /publish:\n[\s\S]*?default: false/);
  assert.match(workflow, /publish-rc:\n    needs: \[reserve, test\]\n    if: inputs.publish && needs.reserve.result == 'success' && needs.test.result == 'success'/);
  assert(!workflow.includes('always()'));
  assert(!workflow.includes('value=latest'));
  assert.match(workflow, /uses: \.\/\.github\/workflows\/candidate-handoff.yml/);
  const legacy = fs.readFileSync(path.join(__dirname, '../workflows/release.yml'), 'utf8');
  assert(!/^  push:/m.test(legacy));
  assert(!/:\s*write\s*$/m.test(legacy));
  assert(!legacy.includes('build-push-action'));
  assert.match(legacy, /exit 1/);
  const handoff = fs.readFileSync(path.join(__dirname, '../workflows/candidate-handoff.yml'), 'utf8');
  assert(!/:\s*write\s*$/m.test(handoff));
  assert.match(handoff, /Prepare and validate both chart packages/);
});

test('chart bytes must match the manifest and archive paths cannot escape', () => {
  const directory = fs.mkdtempSync(path.join(os.tmpdir(), 'rc-charts-'));
  try {
    const checksums = {};
    for (const name of ['ror-api-1.2.3.tgz', 'ror-api-1.2.3-rc.1.tgz']) {
      fs.writeFileSync(path.join(directory, name), name);
      checksums[name] = 'sha256:' + crypto.createHash('sha256').update(name).digest('hex');
    }
    fs.writeFileSync(path.join(directory, 'checksums.json'), JSON.stringify(checksums));
    assert.deepEqual(verifyCharts(directory), checksums);
    fs.writeFileSync(path.join(directory, 'ror-api-1.2.3.tgz'), 'changed');
    assert.throws(() => verifyCharts(directory));
    fs.writeFileSync(path.join(directory, 'checksums.json'), JSON.stringify({ '../escape': 'sha256:' + 'a'.repeat(64), ...checksums }));
    assert.throws(() => verifyCharts(directory));
  } finally { fs.rmSync(directory, { recursive: true, force: true }); }
});

test('RC record is a prerelease only; conflicting tags are never overwritten', async () => {
  const context = { ref: 'refs/heads/main', sha: 'b'.repeat(40), runId: 100, repo: { owner: 'NorskHelsenett', repo: 'ror-api' } };
  const record = { candidateVersion: 'v1.2.3-rc.1', sourceSHA: 'a'.repeat(40), workflowSHA: context.sha, runID: '100', targetVersion: 'v1.2.3', status: 'publishing', binding: { image: { indexDigest: 'sha256:' + 'c'.repeat(64) } } };
  const writes = [];
  const github = { rest: {
    git: { getRef: async () => { throw { status: 404 }; }, createRef: async args => writes.push(args) },
    repos: { getReleaseByTag: async () => { throw { status: 404 }; }, createRelease: async args => { writes.push(args); return { data: { id: 1 } }; } },
  } };
  await publishReleaseRecord({ github, context, record });
  assert.equal(writes[0].ref, 'refs/tags/v1.2.3-rc.1');
  assert.equal(writes[1].prerelease, true);
  assert.equal(writes[1].make_latest, 'false');
  github.rest.git.getRef = async () => ({ data: { object: { type: 'commit', sha: 'd'.repeat(40) } } });
  await assert.rejects(() => publishReleaseRecord({ github, context, record }));
  assert.equal(writes.length, 2);
});

test('publication binds tested digests once and rejects changed rerun content', async () => {
  const context = { ref: 'refs/heads/main', sha: 'b'.repeat(40), runId: 100, repo: { owner: 'NorskHelsenett', repo: 'ror-api' } };
  const allocation = reserve({ schema: 1, versions: {} }, { targetVersion: 'v1.2.3', sourceSHA: 'a'.repeat(40), workflowSHA: context.sha, runID: '100' });
  let state = allocation.state;
  const digest = 'sha256:' + 'a'.repeat(64);
  const result = { ...allocation.candidate, schema: 1, rehearsal: true, published: false, result: 'passed', runAttempt: '1', repository: 'NorskHelsenett/ror-api', binaryVersion: 'v1.2.3', reservedVersion: allocation.candidate.candidateVersion, platform: 'linux/amd64', passed: 34, reportDigest: digest, artifactID: '123', candidate: { platform: 'linux/amd64', archiveSha256: digest, indexDigest: digest, manifestDigest: digest, configDigest: digest } };
  const charts = { 'ror-api-1.2.3.tgz': digest, 'ror-api-1.2.3-rc.1.tgz': digest };
  const github = { rest: {
    git: { getRef: async () => { throw { status: 404 }; } },
    repos: {
      getContent: async () => ({ data: { sha: 'state-sha', content: Buffer.from(JSON.stringify(state)).toString('base64') } }),
      createOrUpdateFileContents: async args => { state = JSON.parse(Buffer.from(args.content, 'base64')); },
    },
  } };
  const before = process.env.GITHUB_RUN_ATTEMPT;
  process.env.GITHUB_RUN_ATTEMPT = '1';
  try {
    const args = { github, context, candidate: allocation.candidate, result, releaseImage: { ...result.candidate }, charts, phase: 'publishing' };
    await bindPublication(args);
    await bindPublication(args);
    await assert.rejects(() => bindPublication({ ...args, result: { ...result, candidate: { ...result.candidate, indexDigest: 'sha256:' + 'b'.repeat(64) } } }));
    await bindPublication({ ...args, phase: 'published' });
    assert.equal(state.versions['v1.2.3'].attempts['100'].status, 'published');
  } finally {
    if (before === undefined) delete process.env.GITHUB_RUN_ATTEMPT; else process.env.GITHUB_RUN_ATTEMPT = before;
  }
});