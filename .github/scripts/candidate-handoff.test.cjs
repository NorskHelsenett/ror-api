const test = require('node:test');
const assert = require('node:assert/strict');
const crypto = require('node:crypto');
const fs = require('node:fs');
const path = require('node:path');
const { HARNESS_SHA, prepareCandidate, verifyHandoff } = require('./candidate-handoff.cjs');
const hash = data => 'sha256:' + crypto.createHash('sha256').update(data).digest('hex');

function fixture() {
  const expected = prepareCandidate({ targetVersion: 'v1.25.0', sourceSHA: 'a'.repeat(40), runID: '123', runAttempt: '1', repository: 'NorskHelsenett/ror-api' });
  const image = { archiveSha256: hash('archive'), indexDigest: hash('index'), manifestDigest: hash('manifest'), configDigest: hash('config'), platform: 'linux/amd64' };
  const suiteBytes = Buffer.from(JSON.stringify({ steps: [{ name: 'deny', status: 403 }] }));
  const fixtureBytes = Buffer.from('fixture'), issuerBytes = Buffer.from('{}');
  const reportBytes = Buffer.from(JSON.stringify({ schema: 1, target: 'candidate', suite: 'suite', expected: 1, results: [{ name: 'deny', status: 403, digest: 'b'.repeat(64) }] }));
  const evidence = { schema: 1, candidate: { ...image }, sourceSHA: expected.sourceSHA, harnessSHA: HARNESS_SHA, repository: expected.repository, runID: '123', runAttempt: '1', inputArtifactID: '456', baseline: null, suite: 'suite', passed: 1, reportDigest: hash(reportBytes), suiteFileDigest: hash(suiteBytes), fixtureFileDigest: hash(fixtureBytes), issuerFileDigest: hash(issuerBytes) };
  const sharedModule = { Path: 'github.com/NorskHelsenett/ror', Version: 'v1.25.1' };
  const buildInfo = `build -ldflags="-X github.com/NorskHelsenett/ror/pkg/config/rorversion.Version=${expected.targetVersion} -X github.com/NorskHelsenett/ror/pkg/config/rorversion.Commit=${expected.sourceSHA} -X github.com/NorskHelsenett/ror/pkg/config/rorversion.LibVer=${sharedModule.Version}"`;
  return { expected, image, evidence, reportBytes, suiteBytes, fixtureBytes, issuerBytes, buildInfo, sharedModule, verified: { schema: 1, suite: 'suite', passed: 1, reportDigest: evidence.reportDigest }, exitCode: '0', harnessCommit: HARNESS_SHA, harnessWorktree: '', artifactID: '456' };
}

test('RC attempt uses final binary version and changes with source/run identity', () => {
  const input = fixture().expected;
  assert.equal(input.candidateVersion, 'v1.25.0-rc.123.1');
  assert.equal(input.binaryVersion, 'v1.25.0');
  const replacement = prepareCandidate({ ...input, sourceSHA: 'b'.repeat(40), runID: '124' });
  assert.equal(replacement.targetVersion, input.targetVersion);
  assert.notEqual(replacement.candidateVersion, input.candidateVersion);
  for (const targetVersion of ['1.25.0', 'v01.2.3', 'v1.2.3-rc.1', 'v1.2.3\n', 'v1.2.3;echo bad']) {
    assert.throws(() => prepareCandidate({ ...input, targetVersion }));
  }
});

test('passing evidence produces an unpublished rehearsal record', () => {
  const result = verifyHandoff(fixture());
  assert.equal(result.published, false);
  assert.equal(result.result, 'passed');
  assert.equal(result.passed, 1);
  assert.equal(result.reservedVersion, null);
});

test('a reserved RC is recorded only when it belongs to the target version', () => {
  assert.equal(verifyHandoff({ ...fixture(), reservedVersion: 'v1.25.0-rc.7' }).reservedVersion, 'v1.25.0-rc.7');
  for (const reservedVersion of ['v1.26.0-rc.1', 'v1.25.0', 'v1.25.0-rc.0', 'v1.25.0-rc.1\n', '1.25.0-rc.1']) {
    assert.throws(() => verifyHandoff({ ...fixture(), reservedVersion }), reservedVersion);
  }
});

test('mismatched, incomplete and failed evidence never passes', () => {
  const changes = {
    source: input => { input.evidence.sourceSHA = 'b'.repeat(40); },
    harness: input => { input.evidence.harnessSHA = 'b'.repeat(40); },
    run: input => { input.evidence.runID = '999'; },
    attempt: input => { input.evidence.runAttempt = '2'; },
    artifact: input => { input.evidence.inputArtifactID = '999'; },
    image: input => { input.evidence.candidate.manifestDigest = hash('other'); },
    platform: input => { input.image.platform = input.evidence.candidate.platform = 'linux/arm64'; },
    failed: input => { input.exitCode = '1'; },
    dirty: input => { input.harnessWorktree = 'modified'; },
    fixture: input => { input.fixtureBytes = Buffer.from('changed'); },
    count: input => { input.evidence.passed = 0; },
    report: input => { input.reportBytes = Buffer.from('{}'); },
    buildVersion: input => { input.buildInfo = input.buildInfo.replace('Version=v1.25.0 ', 'Version=v1.25.0-rc.1 '); },
    replacedModule: input => { input.sharedModule.Replace = { Path: '../ror' }; },
    candidateVersion: input => { input.expected.candidateVersion = 'v1.25.0-rc.999'; },
    assertion: input => {
      const report = JSON.parse(input.reportBytes);
      report.results[0].failures = ['denied access was allowed'];
      input.reportBytes = Buffer.from(JSON.stringify(report));
      input.verified.reportDigest = input.evidence.reportDigest = hash(input.reportBytes);
    },
  };
  for (const [name, change] of Object.entries(changes)) {
    const input = fixture();
    change(input);
    assert.throws(() => verifyHandoff(input), { name: /Error/ }, name);
  }
});

test('caller remains explicit, read-only, pinned, and publication-free', () => {
  const workflow = fs.readFileSync(path.join(__dirname, '../workflows/candidate-handoff.yml'), 'utf8');
  assert.match(workflow, /^  workflow_dispatch:/m);
  assert.match(workflow, /^  workflow_call:/m);
  assert(!/^\s+(push|pull_request|schedule):/m.test(workflow));
  assert(!/:\s*write\s*$/m.test(workflow), 'rehearsal must not grant write permissions');
  assert(!/docker\s+(?:buildx\s+imagetools\s+create|push)|helm\s+push|git\s+push|gh\s+release|--push/.test(workflow));
  assert(workflow.includes(`uses: NorskHelsenett/ror-test/.github/workflows/candidate-e2e.yml@${HARNESS_SHA}`));
  assert(workflow.includes(`harness_sha: ${HARNESS_SHA}`));
  assert.match(workflow, /verify-handoff:\n    needs: \[prepare, build, integration\]/);
  assert(!workflow.includes('always()'), 'success gate must not run after failed tests');
});