const assert = require('node:assert/strict');

// Notes are cosmetic, so a lookup failure must never abort a publication that already pushed artifacts.
async function changelogSinceStable(github, repo, { tagName, sourceSHA }) {
  try {
    const { data: stable } = await github.rest.repos.getLatestRelease(repo);
    if (stable.draft || stable.prerelease) return undefined;
    const { data } = await github.rest.repos.generateReleaseNotes({
      ...repo, tag_name: tagName, target_commitish: sourceSHA, previous_tag_name: stable.tag_name,
    });
    return data.body || undefined;
  } catch {
    return undefined;
  }
}

async function publishReleaseRecord({ github, context, record }) {
  assert.equal(context.ref, 'refs/heads/main');
  assert.match(record.candidateVersion, /^v\d+\.\d+\.\d+-rc\.[1-9][0-9]*$/);
  assert.match(record.sourceSHA, /^[0-9a-f]{40}$/);
  assert.equal(record.runID, String(context.runId));
  assert.equal(record.workflowSHA, context.sha);
  assert(record.binding && ['publishing', 'published'].includes(record.status));
  const ref = `tags/${record.candidateVersion}`;
  try {
    const { data } = await github.rest.git.getRef({ ...context.repo, ref });
    assert.equal(data.object.type, 'commit', 'RC tag must be a direct commit reference');
    assert.equal(data.object.sha, record.sourceSHA, 'RC tag belongs to another commit');
  } catch (error) {
    if (error.status !== 404) throw error;
    await github.rest.git.createRef({ ...context.repo, ref: `refs/${ref}`, sha: record.sourceSHA });
  }
  let release;
  try {
    ({ data: release } = await github.rest.repos.getReleaseByTag({ ...context.repo, tag: record.candidateVersion }));
    assert.equal(release.prerelease, true, 'existing release is not a prerelease');
    assert.equal(release.draft, false);
    assert.equal(release.target_commitish, record.sourceSHA, 'release targets another commit');
  } catch (error) {
    if (error.status !== 404) throw error;
    const provenance = `Passed amd64 integration tests before publication.\n\nTarget final/binary version: ${record.targetVersion}\nSource: ${record.sourceSHA}\nImage index: ${record.binding.image.indexDigest}\nHarness: ${record.harnessSHA}\nEvidence: https://github.com/${context.repo.owner}/${context.repo.repo}/actions/runs/${record.runID}\n\nBoth architectures built; only amd64 integration-tested. No stable latest or final release was updated.`;
    const changelog = await changelogSinceStable(github, context.repo, {
      tagName: record.candidateVersion, sourceSHA: record.sourceSHA,
    });
    ({ data: release } = await github.rest.repos.createRelease({
      ...context.repo, tag_name: record.candidateVersion, target_commitish: record.sourceSHA,
      name: record.candidateVersion, prerelease: true, draft: false, make_latest: 'false',
      // Without a changelog GitHub generates one against the closest preceding tag instead.
      ...(changelog ? { body: `${provenance}\n\n${changelog}` } : { body: provenance, generate_release_notes: true }),
    }));
  }
  return release;
}

module.exports = { publishReleaseRecord, changelogSinceStable };