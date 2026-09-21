const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const crypto = require('node:crypto');
const { execFileSync } = require('node:child_process');

function verifyCharts(directory) {
  const checksums = JSON.parse(fs.readFileSync(path.join(directory, 'checksums.json')));
  assert.equal(Object.keys(checksums).length, 2, 'expected RC and final chart packages');
  for (const [filename, expected] of Object.entries(checksums)) {
    assert.match(filename, /^ror-api-(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-rc\.[1-9][0-9]*)?\.tgz$/);
    assert.match(expected, /^sha256:[0-9a-f]{64}$/);
    const file = path.join(directory, filename);
    assert(fs.lstatSync(file).isFile(), 'chart must be a regular file');
    assert.equal('sha256:' + crypto.createHash('sha256').update(fs.readFileSync(file)).digest('hex'), expected, 'chart bytes changed');
  }
  return checksums;
}

function verifyBundle(bundle, result, verifier) {
  const image = JSON.parse(fs.readFileSync(path.join(bundle, 'candidate-image.json')));
  assert.deepEqual(result.candidate, image, 'verified image differs from build metadata');
  assert.equal(result.result, 'passed');
  assert.equal(result.published, false);
  const archive = path.resolve(bundle, 'release.tar');
  const releaseImage = JSON.parse(fs.readFileSync(path.join(bundle, 'release-image.json')));
  assert.equal(releaseImage.manifestDigest, image.manifestDigest, 'release index changed the tested amd64 manifest');
  assert.equal(releaseImage.configDigest, image.configDigest, 'release index changed the tested amd64 config');
  assert.equal(releaseImage.platform, 'linux/amd64');
  assert(fs.lstatSync(archive).isFile());
  for (const platform of ['linux/amd64', 'linux/arm64']) {
    const output = path.resolve(bundle, platform.endsWith('amd64') ? 'rechecked-image.json' : 'rechecked-arm64.json');
    execFileSync(verifier, ['inspect-candidate', '-archive', archive, '-checksum', releaseImage.archiveSha256, '-digest', releaseImage.indexDigest, '-platform', platform, '-out', output], { stdio: 'inherit' });
    if (platform === 'linux/amd64') assert.deepEqual(JSON.parse(fs.readFileSync(output)), releaseImage);
  }
  const armBuild = fs.readFileSync(path.join(bundle, 'build-arm64.txt'), 'utf8');
  assert(armBuild.includes('GOARCH=arm64'));
  for (const setting of [
    `github.com/NorskHelsenett/ror/pkg/config/rorversion.Version=${result.targetVersion}`,
    `github.com/NorskHelsenett/ror/pkg/config/rorversion.Commit=${result.sourceSHA}`,
  ]) assert(armBuild.includes(`-X ${setting} `) || armBuild.includes(`-X ${setting}"`));
  return verifyCharts(path.join(bundle, 'charts'));
}

module.exports = { verifyBundle, verifyCharts };
if (require.main === module) {
  const [bundle, resultFile, verifier] = process.argv.slice(2);
  verifyBundle(bundle, JSON.parse(fs.readFileSync(resultFile)), verifier);
}