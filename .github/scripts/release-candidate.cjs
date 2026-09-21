const assert = require('node:assert/strict');
const { HARNESS_SHA } = require('./candidate-handoff.cjs');

const VERSION = /^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$/;
const SHA = /^[0-9a-f]{40}$/;
const DIGEST = /^sha256:[0-9a-f]{64}$/;
const STATE_BRANCH = 'ror-release-state';

function reserve(state, request) {
  assert.equal(state.schema, 1);
  assert.match(request.targetVersion, VERSION);
  assert.match(request.sourceSHA, SHA);
  assert.match(request.workflowSHA, SHA);
  assert.match(request.runID, /^[1-9][0-9]*$/);
  const next = structuredClone(state);
  const release = next.versions[request.targetVersion] ||= { next: 1, attempts: {} };
  assert(!release.final, 'final version already published');
  const existing = release.attempts[request.runID];
  if (existing) {
    assert.equal(existing.sourceSHA, request.sourceSHA, 'rerun cannot change source');
    assert.equal(existing.workflowSHA, request.workflowSHA, 'rerun cannot change workflow');
    return { state: next, candidate: existing };
  }
  assert(Number.isSafeInteger(release.next) && release.next > 0);
  const number = release.next++;
  const candidate = {
    ...request, number, candidateVersion: `${request.targetVersion}-rc.${number}`,
    harnessSHA: HARNESS_SHA, status: 'reserved',
  };
  release.attempts[request.runID] = candidate;
  return { state: next, candidate };
}

function assertEligible(state, candidate) {
  const release = state.versions[candidate.targetVersion];
  assert(release && !release.final, 'target version is unavailable');
  assert.deepEqual(release.attempts[candidate.runID], candidate, 'reservation changed');
  assert.equal(candidate.number, release.next - 1, 'candidate superseded by a newer attempt');
  assert(['reserved', 'publishing', 'published'].includes(candidate.status));
}

function verifyBuild(candidate, result) {
  assert.equal(result.schema, 1);
  assert.equal(result.rehearsal, true);
  assert.equal(result.published, false);
  assert.equal(result.result, 'passed');
  for (const field of ['targetVersion', 'sourceSHA', 'runID', 'harnessSHA']) {
    assert.equal(result[field], candidate[field], `verified build ${field} mismatch`);
  }
  assert.equal(result.binaryVersion, candidate.targetVersion);
  assert.equal(result.platform, 'linux/amd64');
  assert.equal(result.candidate.platform, 'linux/amd64');
  assert(Number.isSafeInteger(result.passed) && result.passed > 0);
  for (const field of ['archiveSha256', 'indexDigest', 'manifestDigest', 'configDigest']) {
    assert.match(result.candidate[field], DIGEST);
  }
  assert.match(result.reportDigest, DIGEST);
  return result.candidate;
}

async function getState(github, repo) {
  try {
    const { data } = await github.rest.repos.getContent({ ...repo, path: 'release-state.json', ref: STATE_BRANCH });
    const state = JSON.parse(Buffer.from(data.content, 'base64').toString());
    assert.equal(state.schema, 1);
    return { state, sha: data.sha };
  } catch (error) {
    if (error.status !== 404) throw error;
    return { state: { schema: 1, versions: {} }, sha: undefined };
  }
}

async function ensureStateBranch(github, repo, mainSHA) {
  try {
    await github.rest.git.getRef({ ...repo, ref: `heads/${STATE_BRANCH}` });
  } catch (error) {
    if (error.status !== 404) throw error;
    try {
      await github.rest.git.createRef({ ...repo, ref: `refs/heads/${STATE_BRANCH}`, sha: mainSHA });
    } catch (createError) {
      if (createError.status !== 422) throw createError;
      await github.rest.git.getRef({ ...repo, ref: `heads/${STATE_BRANCH}` });
    }
  }
}

async function assertNoFinalTag(github, repo, targetVersion) {
  try {
    await github.rest.git.getRef({ ...repo, ref: `tags/${targetVersion}` });
  } catch (error) {
    if (error.status === 404) return;
    throw error;
  }
  throw new Error('Final version tag already exists');
}

async function allocate({ github, context, targetVersion, apiRef }) {
  assert.equal(context.ref, 'refs/heads/main', 'release workflow must run from main');
  assert.equal(`${context.repo.owner}/${context.repo.repo}`, 'NorskHelsenett/ror-api');
  assert.match(targetVersion, VERSION);
  const { data: source } = await github.rest.repos.getCommit({ ...context.repo, ref: apiRef || context.sha });
  const { data: comparison } = await github.rest.repos.compareCommitsWithBasehead({ ...context.repo, basehead: `${source.sha}...main` });
  assert(['ahead', 'identical'].includes(comparison.status), 'source must be on main history');
  await assertNoFinalTag(github, context.repo, targetVersion);
  await ensureStateBranch(github, context.repo, context.sha);
  const request = { targetVersion, sourceSHA: source.sha, workflowSHA: context.sha, runID: String(context.runId) };
  for (let attempt = 0; attempt < 5; attempt++) {
    const current = await getState(github, context.repo);
    const allocation = reserve(current.state, request);
    if (current.state.versions[targetVersion]?.attempts[request.runID]) return allocation.candidate;
    try {
      await github.rest.repos.createOrUpdateFileContents({
        ...context.repo, branch: STATE_BRANCH, path: 'release-state.json', sha: current.sha,
        message: `Reserve ${allocation.candidate.candidateVersion} for run ${request.runID}`,
        content: Buffer.from(JSON.stringify(allocation.state, null, 2) + '\n').toString('base64'),
      });
      return allocation.candidate;
    } catch (error) {
      if (![409, 422].includes(error.status)) throw error;
    }
  }
  throw new Error('Concurrent RC allocation failed; no public tag was created');
}

module.exports = { reserve, assertEligible, verifyBuild, getState, allocate, assertNoFinalTag, STATE_BRANCH };

async function bindPublication({ github, context, candidate, result, releaseImage, charts, phase }) {
  assert.equal(context.ref, 'refs/heads/main');
  assert.equal(candidate.workflowSHA, context.sha);
  assert.equal(candidate.runID, String(context.runId));
  assert.equal(result.runAttempt, process.env.GITHUB_RUN_ATTEMPT);
  assert.equal(result.repository, 'NorskHelsenett/ror-api');
  verifyBuild(candidate, result);
  assert.equal(releaseImage.platform, 'linux/amd64');
  assert.equal(releaseImage.manifestDigest, result.candidate.manifestDigest, 'release must contain the tested manifest');
  assert.equal(releaseImage.configDigest, result.candidate.configDigest);
  for (const key of ['archiveSha256', 'indexDigest', 'manifestDigest', 'configDigest']) assert.match(releaseImage[key], DIGEST);
  assert.deepEqual(Object.keys(charts).sort(), [
    `ror-api-${candidate.targetVersion.slice(1)}.tgz`,
    `ror-api-${candidate.candidateVersion.slice(1)}.tgz`,
  ].sort());
  for (const digest of Object.values(charts)) assert.match(digest, DIGEST);
  await assertNoFinalTag(github, context.repo, candidate.targetVersion);
  try {
    const { data } = await github.rest.git.getRef({ ...context.repo, ref: `tags/${candidate.candidateVersion}` });
    assert.equal(data.object.type, 'commit', 'existing RC tag must be a direct commit reference');
    assert.equal(data.object.sha, candidate.sourceSHA, 'existing RC tag belongs to another source');
  } catch (error) {
    if (error.status !== 404) throw error;
  }
  const current = await getState(github, context.repo);
  const stored = current.state.versions[candidate.targetVersion]?.attempts[candidate.runID];
  assert(stored, 'reservation missing');
  for (const key of ['sourceSHA', 'workflowSHA', 'harnessSHA', 'candidateVersion']) assert.equal(stored[key], candidate[key]);
  assertEligible(current.state, stored);
  const binding = { image: releaseImage, testedImage: result.candidate, charts, reportDigest: result.reportDigest, artifactID: result.artifactID };
  if (stored.binding) assert.deepEqual(stored.binding, binding, 'retry changed already bound publication content');
  assert(['publishing', 'published'].includes(phase));
  assert(phase !== 'published' || stored.binding, 'publication must be bound before completion');
  stored.binding = binding;
  stored.status = phase === 'publishing' && stored.status === 'published' ? 'published' : phase;
  await github.rest.repos.createOrUpdateFileContents({
    ...context.repo, branch: STATE_BRANCH, path: 'release-state.json', sha: current.sha,
    message: `${stored.status}: ${stored.candidateVersion}`,
    content: Buffer.from(JSON.stringify(current.state, null, 2) + '\n').toString('base64'),
  });
  return stored;
}

module.exports.bindPublication = bindPublication;