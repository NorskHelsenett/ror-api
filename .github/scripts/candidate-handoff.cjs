const assert = require('node:assert/strict');
const crypto = require('node:crypto');
const fs = require('node:fs');
const path = require('node:path');

const HARNESS_SHA = '199e73ba8875775cc1329faabb7eea6475a02d4c';
const digestPattern = /^sha256:[0-9a-f]{64}$/;
const shaPattern = /^[0-9a-f]{40}$/;
const hash = data => 'sha256:' + crypto.createHash('sha256').update(data).digest('hex');

function prepareCandidate({ targetVersion, sourceSHA, runID, runAttempt, repository }) {
  assert.match(targetVersion, /^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$/, 'target must be vMAJOR.MINOR.PATCH');
  assert.match(sourceSHA, shaPattern, 'source must be a full commit SHA');
  assert.match(runID, /^[1-9][0-9]*$/, 'run ID required');
  assert.match(runAttempt, /^[1-9][0-9]*$/, 'run attempt required');
  assert.equal(repository, 'NorskHelsenett/ror-api', 'unexpected caller repository');
  return {
    schema: 1,
    rehearsal: true,
    targetVersion,
    binaryVersion: targetVersion,
    candidateVersion: `${targetVersion}-rc.${runID}.${runAttempt}`,
    sourceSHA,
    harnessSHA: HARNESS_SHA,
    repository,
    runID,
    runAttempt,
    platform: 'linux/amd64',
  };
}

function verifyHandoff({ expected, image, evidence, reportBytes, verified, suiteBytes, fixtureBytes, issuerBytes, exitCode, harnessCommit, harnessWorktree, artifactID, buildInfo, sharedModule }) {
  assert.equal(expected.schema, 1);
  assert.equal(expected.rehearsal, true);
  assert.equal(expected.harnessSHA, HARNESS_SHA);
  assert.deepEqual(expected, prepareCandidate(expected), 'candidate request metadata is inconsistent');
  assert.equal(sharedModule.Path, 'github.com/NorskHelsenett/ror');
  assert.equal(sharedModule.Replace, undefined, 'shared dependency was replaced');
  assert.match(sharedModule.Version, /^v[0-9][0-9A-Za-z.+-]*$/);
  for (const setting of [
    `github.com/NorskHelsenett/ror/pkg/config/rorversion.Version=${expected.targetVersion}`,
    `github.com/NorskHelsenett/ror/pkg/config/rorversion.Commit=${expected.sourceSHA}`,
    `github.com/NorskHelsenett/ror/pkg/config/rorversion.LibVer=${sharedModule.Version}`,
  ]) {
    assert(buildInfo.includes(`-X ${setting} `) || buildInfo.includes(`-X ${setting}"`), `missing exact build setting ${setting}`);
  }
  assert.equal(evidence.schema, 1);
  for (const key of ['sourceSHA', 'harnessSHA', 'repository', 'runID', 'runAttempt']) {
    assert.equal(evidence[key], expected[key], `evidence ${key} does not match build`);
  }
  assert.match(artifactID, /^[1-9][0-9]*$/);
  assert.equal(evidence.inputArtifactID, artifactID, 'evidence belongs to another artifact');
  assert.equal(image.platform, 'linux/amd64');
  assert.deepEqual(evidence.candidate, image, 'tested image differs from built candidate');
  for (const key of ['archiveSha256', 'indexDigest', 'manifestDigest', 'configDigest']) {
    assert.match(image[key], digestPattern, `invalid ${key}`);
  }
  assert.equal(evidence.baseline, null, 'unexpected baseline in rehearsal');
  assert.equal(exitCode.trim(), '0');
  assert.equal(harnessCommit.trim(), HARNESS_SHA);
  assert.equal(harnessWorktree.trim(), '', 'test harness was modified');
  assert.equal(evidence.suiteFileDigest, hash(suiteBytes));
  assert.equal(evidence.fixtureFileDigest, hash(fixtureBytes));
  assert.equal(evidence.issuerFileDigest, hash(issuerBytes));
  assert.equal(evidence.reportDigest, hash(reportBytes), 'report digest mismatch');
  assert.equal(verified.schema, 1);
  assert.equal(verified.reportDigest, evidence.reportDigest);
  assert.equal(verified.suite, evidence.suite);
  const suite = JSON.parse(suiteBytes);
  const report = JSON.parse(reportBytes);
  assert(suite.steps.length > 0, 'empty suite');
  assert.equal(report.schema, 1);
  assert.equal(report.target, 'candidate');
  assert.equal(report.suite, evidence.suite);
  assert.equal(report.expected, suite.steps.length);
  assert.equal(report.results.length, suite.steps.length);
  assert.equal(evidence.passed, suite.steps.length);
  assert.equal(verified.passed, suite.steps.length);
  suite.steps.forEach((step, index) => {
    const result = report.results[index];
    assert.equal(result.name, step.name);
    assert.equal(result.status, step.status);
    assert.equal((result.failures || []).length, 0);
    assert.match(result.digest, /^[0-9a-f]{64}$/);
  });
  return { ...expected, result: 'passed', published: false, candidate: image, artifactID, reportDigest: evidence.reportDigest, passed: suite.steps.length };
}

function evidenceDirectory(root) {
  const found = [];
  function walk(directory) {
    for (const entry of fs.readdirSync(directory, { withFileTypes: true })) {
      assert(!entry.isSymbolicLink(), 'evidence must not contain symlinks');
      const filename = path.join(directory, entry.name);
      if (entry.isDirectory()) walk(filename);
      else if (entry.name === 'release-evidence.json') found.push(directory);
    }
  }
  walk(root);
  assert.equal(found.length, 1, 'expected exactly one candidate evidence directory');
  return found[0];
}

module.exports = { HARNESS_SHA, prepareCandidate, verifyHandoff, evidenceDirectory };

if (require.main === module) {
  const [buildDirectory, evidenceRoot, harnessRoot, artifactID] = process.argv.slice(2);
  const directory = evidenceDirectory(evidenceRoot);
  const read = name => fs.readFileSync(path.join(directory, name));
  const summary = verifyHandoff({
    expected: JSON.parse(fs.readFileSync(path.join(buildDirectory, 'candidate-request.json'))),
    image: JSON.parse(fs.readFileSync(path.join(buildDirectory, 'candidate-image.json'))),
    evidence: JSON.parse(read('release-evidence.json')),
    reportBytes: read('candidate.json'),
    verified: JSON.parse(read('candidate-verified.json')),
    suiteBytes: fs.readFileSync(path.join(harnessRoot, 'testenv/scenarios/acl.json')),
    fixtureBytes: fs.readFileSync(path.join(harnessRoot, 'testenv/seed.js')),
    issuerBytes: fs.readFileSync(path.join(harnessRoot, 'testenv/oidc.json')),
    exitCode: read('exit-code.txt').toString(),
    harnessCommit: read('harness-commit.txt').toString(),
    harnessWorktree: read('harness-worktree.txt').toString(),
    artifactID,
    buildInfo: fs.readFileSync(path.join(buildDirectory, 'build.txt'), 'utf8'),
    sharedModule: JSON.parse(fs.readFileSync(path.join(buildDirectory, 'shared-module.json'))),
  });
  assert.equal(summary.repository, process.env.GITHUB_REPOSITORY);
  assert.equal(summary.runID, process.env.GITHUB_RUN_ID);
  assert.equal(summary.runAttempt, process.env.GITHUB_RUN_ATTEMPT);
  fs.writeFileSync('handoff-result.json', JSON.stringify(summary, null, 2) + '\n', { mode: 0o600 });
  if (process.env.GITHUB_STEP_SUMMARY) {
    fs.appendFileSync(process.env.GITHUB_STEP_SUMMARY,
      `## Candidate handoff verified\n\n- Target/binary version: ${summary.targetVersion}\n- Rehearsal RC: ${summary.candidateVersion}\n- Source: ${summary.sourceSHA}\n- Harness: ${summary.harnessSHA}\n- amd64 manifest: ${summary.candidate.manifestDigest}\n- Scenarios passed: ${summary.passed}\n\n**Test only: nothing was published.**\n`);
  }
  console.log(`Verified ${summary.passed} scenarios for ${summary.candidateVersion}; nothing published.`);
}