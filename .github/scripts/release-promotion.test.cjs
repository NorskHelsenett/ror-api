const test = require('node:test');
const assert = require('node:assert/strict');
const {
  assertAdmin, compareVersions, selectCandidate, finalVersionOf,
  supersedesLatest, markFinal, publishFinalRecord,
} = require('./release-promotion.cjs');

const digest = 'sha256:' + 'a'.repeat(64);
const otherDigest = 'sha256:' + 'b'.repeat(64);

function ledger(overrides = {}) {
  const candidate = {
    targetVersion: 'v1.2.3', sourceSHA: 'a'.repeat(40), workflowSHA: 'b'.repeat(40), runID: '100',
    number: 1, candidateVersion: 'v1.2.3-rc.1', harnessSHA: 'c'.repeat(40), status: 'published',
    binding: {
      image: { platform: 'linux/amd64', indexDigest: digest, manifestDigest: digest, configDigest: digest, archiveSha256: digest },
      charts: { 'ror-api-1.2.3.tgz': digest, 'ror-api-1.2.3-rc.1.tgz': otherDigest },
    },
    ...overrides,
  };
  return { state: { schema: 1, versions: { 'v1.2.3': { next: 2, attempts: { 100: candidate } } } }, candidate };
}

test('only a uniquely reserved, published candidate can be promoted', () => {
  const { state, candidate } = ledger();
  assert.deepEqual(selectCandidate(state, 'v1.2.3-rc.1'), candidate);
  assert.equal(finalVersionOf('v1.2.3-rc.9'), 'v1.2.3');
  for (const version of ['v1.2.3', 'v1.2.3-rc.0', '1.2.3-rc.1', 'v1.2.3-rc.1\n', 'v1.2.3-rc.1;rm -rf /']) {
    assert.throws(() => selectCandidate(state, version), version);
  }
  assert.throws(() => selectCandidate(ledger().state, 'v9.9.9-rc.1'), /ever reserved/);
});

test('promotion refuses candidates that were not fully published', () => {
  const changes = {
    reserved: candidate => { candidate.status = 'reserved'; },
    publishing: candidate => { candidate.status = 'publishing'; },
    unbound: candidate => { delete candidate.binding; },
    missingChart: candidate => { delete candidate.binding.charts['ror-api-1.2.3.tgz']; },
    badDigest: candidate => { candidate.binding.image.indexDigest = 'sha256:nope'; },
    wrongPlatform: candidate => { candidate.binding.image.platform = 'linux/arm64'; },
  };
  for (const [name, change] of Object.entries(changes)) {
    const { state } = ledger();
    change(state.versions['v1.2.3'].attempts[100]);
    assert.throws(() => selectCandidate(state, 'v1.2.3-rc.1'), name);
  }
});

test('a version is promoted once, and only a resumed attempt may retry', () => {
  const { state } = ledger();
  state.versions['v1.2.3'].final = { candidateVersion: 'v1.2.3-rc.1', indexDigest: digest, status: 'promoting' };
  assert.equal(selectCandidate(state, 'v1.2.3-rc.1').candidateVersion, 'v1.2.3-rc.1');
  state.versions['v1.2.3'].final.status = 'promoted';
  assert.throws(() => selectCandidate(state, 'v1.2.3-rc.1'), /already been released/);
  const other = ledger().state;
  other.versions['v1.2.3'].attempts[101] = { ...other.versions['v1.2.3'].attempts[100], runID: '101', candidateVersion: 'v1.2.3-rc.2', number: 2 };
  other.versions['v1.2.3'].final = { candidateVersion: 'v1.2.3-rc.1', indexDigest: digest, status: 'promoting' };
  assert.throws(() => selectCandidate(other, 'v1.2.3-rc.2'), /different candidate/);
});

test('only repository admins may promote', () => {
  assertAdmin('admin', 'someone');
  for (const permission of ['write', 'maintain', 'triage', 'read', 'none', undefined]) {
    assert.throws(() => assertAdmin(permission, 'someone'), /not a repository admin/);
  }
});

test('stable pointer only moves forward and fails closed', async () => {
  assert.equal(compareVersions('v1.3.0', 'v1.2.9'), 1);
  assert.equal(compareVersions('v1.2.3', 'v1.10.0'), -1);
  assert.equal(compareVersions('v2.0.0', 'v2.0.0'), 0);
  const repo = { owner: 'NorskHelsenett', repo: 'ror-api' };
  const latest = data => ({ rest: { repos: { getLatestRelease: async () => ({ data }) } } });
  assert.equal(await supersedesLatest(latest({ tag_name: 'v1.2.0', draft: false, prerelease: false }), repo, 'v1.3.0'), true);
  assert.equal(await supersedesLatest(latest({ tag_name: 'v1.4.0', draft: false, prerelease: false }), repo, 'v1.3.0'), false);
  assert.equal(await supersedesLatest(latest({ tag_name: 'v1.2.0', draft: false, prerelease: true }), repo, 'v1.3.0'), false);
  assert.equal(await supersedesLatest(latest({ tag_name: 'nightly', draft: false, prerelease: false }), repo, 'v1.3.0'), false);
  const missing = { rest: { repos: { getLatestRelease: async () => { throw { status: 404 }; } } } };
  assert.equal(await supersedesLatest(missing, repo, 'v1.3.0'), true);
  const broken = { rest: { repos: { getLatestRelease: async () => { throw { status: 500 }; } } } };
  await assert.rejects(() => supersedesLatest(broken, repo, 'v1.3.0'));
});

function promotionGithub(state, writes) {
  return { rest: { repos: {
    getContent: async () => ({ data: { content: Buffer.from(JSON.stringify(state)).toString('base64'), sha: 'f'.repeat(40) } }),
    createOrUpdateFileContents: async args => { writes.push(JSON.parse(Buffer.from(args.content, 'base64').toString())); },
  } } };
}

test('the ledger records promotion start and completion for the same image', async () => {
  const { state, candidate } = ledger();
  const writes = [];
  const context = { ref: 'refs/heads/main', repo: { owner: 'NorskHelsenett', repo: 'ror-api' }, runId: 500, actor: 'admin-user' };
  const github = promotionGithub(state, writes);
  const started = await markFinal({ github, context, candidate, phase: 'promoting' });
  assert.equal(started.status, 'promoting');
  assert.equal(started.promotedBy, 'admin-user');
  assert.equal(writes[0].versions['v1.2.3'].final.indexDigest, digest);
  const github2 = promotionGithub(writes[0], writes);
  const done = await markFinal({ github: github2, context, candidate, phase: 'promoted' });
  assert.equal(done.status, 'promoted');
  await assert.rejects(() => markFinal({ github: promotionGithub(state, writes), context, candidate, phase: 'promoted' }), /before it completes/);
  const switched = { ...candidate, binding: { ...candidate.binding, image: { ...candidate.binding.image, indexDigest: otherDigest } } };
  await assert.rejects(() => markFinal({ github: promotionGithub(writes[0], writes), context, candidate: switched, phase: 'promoted' }));
});

test('the final release is never a prerelease and only claims latest when newest', async () => {
  const { candidate } = ledger();
  const context = { ref: 'refs/heads/main', repo: { owner: 'NorskHelsenett', repo: 'ror-api' }, runId: 500, actor: 'admin-user' };
  for (const updateLatest of [true, false]) {
    const calls = [];
    const github = { rest: {
      git: { getRef: async () => { throw { status: 404 }; }, createRef: async args => calls.push(args) },
      repos: {
        getReleaseByTag: async () => { throw { status: 404 }; },
        getLatestRelease: async () => ({ data: { tag_name: 'v1.1.0', draft: false, prerelease: false } }),
        generateReleaseNotes: async () => ({ data: { body: "## What's Changed\n* a change" } }),
        createRelease: async args => { calls.push(args); return { data: { tag_name: args.tag_name } }; },
      },
    } };
    await publishFinalRecord({ github, context, candidate, updateLatest });
    assert.equal(calls[0].ref, 'refs/tags/v1.2.3');
    assert.equal(calls[1].prerelease, false);
    assert.equal(calls[1].make_latest, updateLatest ? 'true' : 'false');
    assert.match(calls[1].body, /## What's Changed/);
    assert.match(calls[1].body, /Promoted from v1\.2\.3-rc\.1/);
  }
});

test('an existing final tag for another commit stops promotion', async () => {
  const { candidate } = ledger();
  const context = { ref: 'refs/heads/main', repo: { owner: 'NorskHelsenett', repo: 'ror-api' }, runId: 500, actor: 'admin-user' };
  const github = { rest: {
    git: { getRef: async () => ({ data: { object: { type: 'commit', sha: 'd'.repeat(40) } } }), createRef: async () => { throw new Error('must not tag'); } },
    repos: { getReleaseByTag: async () => { throw { status: 404 }; }, createRelease: async () => { throw new Error('must not release'); } },
  } };
  await assert.rejects(() => publishFinalRecord({ github, context, candidate, updateLatest: true }), /another commit/);
});
