const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const os = require('node:os');
const { spawnSync } = require('node:child_process');
const uploadAssets = require('./upload-rc-assets.cjs');

test('registry publishing fails closed and never overwrites immutable tags', () => {
  for (const mode of ['new', 'existing', 'registry denied', 'image conflict', 'chart conflict', 'final exists']) {
    const directory = fs.mkdtempSync(path.join(os.tmpdir(), 'rc-registry-'));
    try {
      const chart = 'ror-api-1.2.3-rc.1.tgz';
      fs.mkdirSync(path.join(directory, 'bundle/charts'), { recursive: true });
      fs.writeFileSync(path.join(directory, 'bundle/charts', chart), 'tested-chart');
      const tool = `#!${process.execPath}
const fs=require('node:fs'),path=require('node:path');
const tool=path.basename(process.argv[1]),args=process.argv.slice(2),mode=process.env.TEST_MODE;
fs.appendFileSync(process.env.TEST_LOG,JSON.stringify({tool,args})+'\\n');
if(tool==='skopeo') {
 if(args[0]==='list-tags') { if(mode==='registry denied') process.exit(1); console.log(JSON.stringify({Tags: mode==='new'?[]:mode==='final exists'?['v1.2.3']:['v1.2.3-rc.1','1.2.3-rc.1']})); }
 else if(args[0]==='inspect') console.log(mode==='image conflict'?'sha256:'+'b'.repeat(64):process.env.INDEX_DIGEST);
} else if(tool==='helm' && args[0]==='pull') {
 const dest=args[args.indexOf('--destination')+1];
 fs.writeFileSync(path.join(dest,'${chart}'),mode==='chart conflict'?'changed-chart':'tested-chart');
}
`;
      for (const name of ['skopeo', 'helm']) fs.writeFileSync(path.join(directory, name), tool, { mode: 0o755 });
      const log = path.join(directory, 'calls.jsonl');
      const result = spawnSync('/bin/bash', [path.join(__dirname, 'publish-rc.sh'), path.join(directory, 'bundle')], {
        env: { ...process.env, PATH: directory + ':' + process.env.PATH, RC_VERSION: 'v1.2.3-rc.1', INDEX_DIGEST: 'sha256:' + 'a'.repeat(64), TEST_MODE: mode, TEST_LOG: log }, encoding: 'utf8',
      });
      assert.equal(result.status === 0, ['new', 'existing'].includes(mode), `${mode}: ${result.stderr}\n${result.stdout}\n${fs.readFileSync(log, 'utf8')}`);
      const calls = fs.readFileSync(log, 'utf8').trim().split('\n').map(line => JSON.parse(line));
      const writes = calls.filter(call => ['copy', 'push'].includes(call.args[0]));
      assert.equal(writes.length, mode === 'new' ? 2 : 0, mode);
      if (mode === 'new') {
        assert(writes[0].args.includes('--all'));
        assert(writes[0].args.includes('--preserve-digests'));
        assert(writes[0].args.some(value => value.endsWith('/release.tar')));
      }
      assert(!calls.some(call => call.args.some(value => value.includes(':latest'))));
    } finally { fs.rmSync(directory, { recursive: true, force: true }); }
  }
});

test('release asset upload resumes matching assets but refuses conflicting bytes', () => {
  const directory = fs.mkdtempSync(path.join(os.tmpdir(), 'rc-assets-'));
  const previous = process.env.GITHUB_REPOSITORY;
  process.env.GITHUB_REPOSITORY = 'NorskHelsenett/ror-api';
  try {
    const record = path.join(directory, 'release-candidate.json');
    const charts = path.join(directory, 'charts');
    fs.mkdirSync(charts);
    fs.writeFileSync(record, '{}');
    fs.writeFileSync(path.join(charts, 'ror-api-1.2.3-rc.1.tgz'), 'chart');
    let uploads = 0;
    const execute = (_tool, args) => {
      if (args[0] === 'release') { uploads++; assert(!args.includes('--clobber')); return ''; }
      if (args.some(value => value.endsWith('/assets/123'))) return Buffer.from('{}');
      return Buffer.from(JSON.stringify({ assets: [{ name: 'release-candidate.json', id: 123 }] }));
    };
    uploadAssets('v1.2.3-rc.1', record, charts, execute);
    assert.equal(uploads, 1);
    fs.writeFileSync(record, '{"changed":true}');
    assert.throws(() => uploadAssets('v1.2.3-rc.1', record, charts, execute));
    assert.equal(uploads, 1);
  } finally {
    if (previous === undefined) delete process.env.GITHUB_REPOSITORY; else process.env.GITHUB_REPOSITORY = previous;
    fs.rmSync(directory, { recursive: true, force: true });
  }
});