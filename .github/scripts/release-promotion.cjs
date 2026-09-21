const assert = require('node:assert/strict');
const { getState, STATE_BRANCH } = require('./release-candidate.cjs');
const { changelogSinceStable } = require('./publish-release-record.cjs');

const FINAL = /^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$/;
const CANDIDATE = /^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)-rc\.[1-9][0-9]*$/;
const DIGEST = /^sha256:[0-9a-f]{64}$/;

function finalVersionOf(candidateVersion) {
  assert.match(candidateVersion, CANDIDATE, 'promote a published candidate such as v1.16.0-rc.1');
  return candidateVersion.replace(/-rc\.[0-9]+$/, '');
}

function assertAdmin(permission, actor) {
  assert.equal(permission, 'admin', `${actor} is not a repository admin; only admins may promote a release`);
}

function compareVersions(left, right) {
  const parts = version => version.slice(1).split('.').map(Number);
  const [a, b] = [parts(left), parts(right)];
  for (let index = 0; index < 3; index++) {
    if (a[index] !== b[index]) return a[index] < b[index] ? -1 : 1;
  }
  return 0;
}

function selectCandidate(state, candidateVersion) {
  assert.equal(state.schema, 1);
  const targetVersion = finalVersionOf(candidateVersion);
  const release = state.versions[targetVersion];
  assert(release, `no candidate was ever reserved for ${targetVersion}`);
  if (release.final) {
    assert.equal(release.final.candidateVersion, candidateVersion, 'a different candidate is already being promoted');
    assert.notEqual(release.final.status, 'promoted', `${targetVersion} has already been released`);
  }
  const matches = Object.values(release.attempts).filter(attempt => attempt.candidateVersion === candidateVersion);
  assert.equal(matches.length, 1, 'candidate is not uniquely reserved');
  const candidate = matches[0];
  assert.equal(candidate.targetVersion, targetVersion);
  assert.equal(candidate.status, 'published', 'only a published candidate can be promoted');
  assert(candidate.binding, 'candidate has no bound publication');
  assert.equal(candidate.binding.image.platform, 'linux/amd64');
  assert.match(candidate.binding.image.indexDigest, DIGEST);
  const chart = `ror-api-${targetVersion.slice(1)}.tgz`;
  assert.match(candidate.binding.charts?.[chart] ?? '', DIGEST, `candidate never produced the final chart ${chart}`);
  return candidate;
}

// A retried promotion may find its own tag from an earlier failed attempt.
async function assertFinalTagFree(github, repo, targetVersion, sourceSHA) {
  try {
    const { data } = await github.rest.git.getRef({ ...repo, ref: `tags/${targetVersion}` });
    assert.equal(data.object.type, 'commit', 'final tag must be a direct commit reference');
    assert.equal(data.object.sha, sourceSHA, 'final tag already exists for another commit');
  } catch (error) {
    if (error.status !== 404) throw error;
  }
}

// Fails closed: a version that cannot be proven newest must never move the stable pointer.
async function supersedesLatest(github, repo, targetVersion) {
  try {
    const { data } = await github.rest.repos.getLatestRelease(repo);
    if (data.draft || data.prerelease || !FINAL.test(data.tag_name)) return false;
    return compareVersions(targetVersion, data.tag_name) > 0;
  } catch (error) {
    if (error.status === 404) return true;
    throw error;
  }
}

async function authorizePromotion({ github, context, candidateVersion }) {
  assert.equal(context.ref, 'refs/heads/main', 'promotion must run from main');
  assert.equal(`${context.repo.owner}/${context.repo.repo}`, 'NorskHelsenett/ror-api');
  const { data: access } = await github.rest.repos.getCollaboratorPermissionLevel({ ...context.repo, username: context.actor });
  assertAdmin(access.permission, context.actor);
  const { state } = await getState(github, context.repo);
  const candidate = selectCandidate(state, candidateVersion);
  await assertFinalTagFree(github, context.repo, candidate.targetVersion, candidate.sourceSHA);
  return { candidate, updateLatest: await supersedesLatest(github, context.repo, candidate.targetVersion) };
}

async function markFinal({ github, context, candidate, phase }) {
  assert(['promoting', 'promoted'].includes(phase));
  const current = await getState(github, context.repo);
  const release = current.state.versions[candidate.targetVersion];
  assert(release, 'reservation missing');
  assert.deepEqual(release.attempts[candidate.runID], candidate, 'candidate changed during promotion');
  const promotion = {
    candidateVersion: candidate.candidateVersion,
    indexDigest: candidate.binding.image.indexDigest,
    promotedBy: context.actor,
    runID: String(context.runId),
    status: phase,
  };
  if (release.final) {
    assert.equal(release.final.candidateVersion, promotion.candidateVersion, 'promotion switched candidates');
    assert.equal(release.final.indexDigest, promotion.indexDigest, 'promotion switched images');
  }
  assert(phase !== 'promoted' || release.final, 'promotion must be recorded before it completes');
  release.final = promotion;
  await github.rest.repos.createOrUpdateFileContents({
    ...context.repo, branch: STATE_BRANCH, path: 'release-state.json', sha: current.sha,
    message: `${phase}: ${candidate.targetVersion} from ${candidate.candidateVersion}`,
    content: Buffer.from(JSON.stringify(current.state, null, 2) + '\n').toString('base64'),
  });
  return promotion;
}

async function publishFinalRecord({ github, context, candidate, updateLatest }) {
  const targetVersion = candidate.targetVersion;
  await assertFinalTagFree(github, context.repo, targetVersion, candidate.sourceSHA);
  try {
    await github.rest.git.getRef({ ...context.repo, ref: `tags/${targetVersion}` });
  } catch (error) {
    if (error.status !== 404) throw error;
    await github.rest.git.createRef({ ...context.repo, ref: `refs/tags/${targetVersion}`, sha: candidate.sourceSHA });
  }
  try {
    const { data } = await github.rest.repos.getReleaseByTag({ ...context.repo, tag: targetVersion });
    assert.equal(data.draft, false);
    assert.equal(data.prerelease, false, 'existing final release is marked as a prerelease');
    return data;
  } catch (error) {
    if (error.status !== 404) throw error;
  }
  const provenance = `Promoted from ${candidate.candidateVersion} without rebuilding.\n\nImage index: ${candidate.binding.image.indexDigest}\nSource: ${candidate.sourceSHA}\nCandidate evidence: https://github.com/${context.repo.owner}/${context.repo.repo}/releases/tag/${candidate.candidateVersion}`;
  const changelog = await changelogSinceStable(github, context.repo, {
    tagName: targetVersion, sourceSHA: candidate.sourceSHA,
  });
  const { data: release } = await github.rest.repos.createRelease({
    ...context.repo, tag_name: targetVersion, target_commitish: candidate.sourceSHA,
    name: targetVersion, prerelease: false, draft: false, make_latest: updateLatest ? 'true' : 'false',
    ...(changelog ? { body: `${changelog}\n\n---\n\n${provenance}` } : { body: provenance, generate_release_notes: true }),
  });
  return release;
}

module.exports = {
  assertAdmin, compareVersions, selectCandidate, finalVersionOf,
  supersedesLatest, authorizePromotion, markFinal, publishFinalRecord,
};
