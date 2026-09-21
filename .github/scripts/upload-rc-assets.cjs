const fs = require('node:fs');
const path = require('node:path');
const assert = require('node:assert/strict');
const { execFileSync } = require('node:child_process');

function uploadAssets(tag, recordFile, chartsDirectory, execute = execFileSync) {
  assert.match(tag, /^v\d+\.\d+\.\d+-rc\.[1-9][0-9]*$/);
  const repository = process.env.GITHUB_REPOSITORY;
  assert.equal(repository, 'NorskHelsenett/ror-api');
  const release = JSON.parse(execute('gh', ['api', `repos/${repository}/releases/tags/${tag}`]));
  const files = [recordFile, ...fs.readdirSync(chartsDirectory).filter(name => name.endsWith('.tgz') || name === 'checksums.json').map(name => path.join(chartsDirectory, name))];
  for (const filename of files) {
    const name = path.basename(filename);
    const existing = release.assets.find(asset => asset.name === name);
    if (existing) {
      const bytes = execute('gh', ['api', '-H', 'Accept: application/octet-stream', `repos/${repository}/releases/assets/${existing.id}`], { maxBuffer: 32 * 1024 * 1024 });
      assert(Buffer.from(bytes).equals(fs.readFileSync(filename)), `existing release asset differs: ${name}`);
    } else {
      execute('gh', ['release', 'upload', tag, filename, '--repo', repository], { stdio: 'inherit' });
    }
  }
}

module.exports = uploadAssets;
if (require.main === module) uploadAssets(...process.argv.slice(2));