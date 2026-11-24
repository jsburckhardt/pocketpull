// Release Please version-policy checks: the committed config and manifest are
// loaded by the real release-please library (the version bundled in the pinned
// action) against an in-memory GitHub, so no network, token or repository writes.
'use strict';

const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const test = require('node:test');
const {Manifest, GitHub, Errors, VERSION, configSchema, setLogger} = require('release-please');

const ROOT = path.resolve(__dirname, '../..');
const CONFIG = 'release-please-config.json';
const MANIFEST = '.release-please-manifest.json';
const committed = name => fs.readFileSync(path.join(ROOT, name), 'utf8');

// release-please-action v5.0.0 bundles release-please 17.6.0 (its package-lock).
const ACTION = 'googleapis/release-please-action@45996ed1f6d02564a971a2fa1b5860e934307cf7 # v5.0.0';
const BUNDLED = '17.6.0';

const quiet = {error() {}, warn() {}, info() {}, debug() {}, trace() {}};
setLogger(quiet);

function fakeGitHub({manifest, commits, releases = [], tags = [], pullRequests = []}) {
  const files = {[CONFIG]: committed(CONFIG), [MANIFEST]: manifest ?? committed(MANIFEST)};
  const github = new GitHub({
    repository: {owner: 'jsburckhardt', repo: 'pocketpull', defaultBranch: 'main'},
    octokitAPIs: {octokit: {}, request: {}, graphql: {}},
    logger: quiet,
  });
  github.getFileContentsOnBranch = async name => {
    if (!(name in files)) throw new Errors.FileNotFoundError(name);
    const text = files[name];
    return {sha: 'blob', content: Buffer.from(text).toString('base64'), parsedContent: text, mode: '100644'};
  };
  github.releaseIterator = async function* () { yield* releases; };
  github.tagIterator = async function* () { yield* tags; };
  github.mergeCommitIterator = async function* () { yield* commits; };
  github.pullRequestIterator = async function* (_branch, status) {
    yield* pullRequests.filter(pr => (status === 'MERGED') === !!pr.merged);
  };
  return github;
}

let counter = 0;
const commit = message => ({sha: `c${++counter}`.padEnd(40, '0'), message, files: ['internal/x.go']});

// Newest first, as GitHub returns them; the release commit ends the search.
function afterRelease(version, messages) {
  const releaseSha = `r${version}`.padEnd(40, '0');
  return {
    manifest: JSON.stringify({'.': version}),
    releases: [{tagName: `v${version}`, sha: releaseSha, name: `v${version}`, notes: ''}],
    commits: [
      ...messages.map(commit),
      {sha: releaseSha, message: `chore(main): release ${version}`, files: ['CHANGELOG.md'],
       pullRequest: {number: 1, title: `chore(main): release ${version}`, body: '', labels: [],
                     headBranchName: 'release-please--branches--main', baseBranchName: 'main', files: []}},
    ],
  };
}

async function proposed(fixture) {
  const manifest = await Manifest.fromManifest(fakeGitHub(fixture), 'main', CONFIG, MANIFEST);
  const prs = await manifest.buildPullRequests();
  assert.ok(prs.length <= 1, 'single root component must produce at most one release PR');
  return prs[0];
}

// Representative subjects from the collector history up to b446965 (oldest last).
const COLLECTOR_HISTORY = [
  'docs: accept installable archive slice and unlock release PR automation',
  'feat: package checked Linux archives, add safe installer and CI',
  "Merge pull request #4 from jsburckhardt/delivery/1-collector",
  'feat(collector): bound retries and report interrupted outcomes',
  'fix: accept expanded raw snapshots on cache and refresh',
  'feat: add safe collection freshness and process locking',
  'Implement T2 strict configuration and cron-safe secret handling',
  'feat: collect Pocket recording snapshots through native CLI',
  'dev: add non-root workspace with SSH signing checks',
  'chore: vendor soft-factory-v2 skills from toolio a8900d7',
];

test('pinned action bundles the tested release-please version', () => {
  assert.equal(VERSION, BUNDLED);
  const lock = JSON.parse(fs.readFileSync(path.join(__dirname, 'package-lock.json'), 'utf8'));
  assert.equal(lock.packages['node_modules/release-please'].version, BUNDLED);
  assert.ok(committed('.github/workflows/release.yml').includes(`uses: ${ACTION}`));
});

test('config uses only known keys and pins normal SemVer below 1.0', () => {
  const config = JSON.parse(committed(CONFIG));
  const known = new Set(Object.keys(configSchema.properties));
  assert.deepEqual(Object.keys(config).filter(key => !known.has(key)), []);
  assert.deepEqual(Object.keys(config.packages), ['.']);
  assert.equal(config['release-type'], 'go');
  assert.equal(config['bump-minor-pre-major'], false);
  assert.equal(config['bump-patch-for-minor-pre-major'], false);
  assert.equal(config['include-component-in-tag'], false);
  assert.equal(config['draft'], true);
  assert.equal(config['force-tag-creation'], true);
  for (const fixed of ['release-as', 'initial-version', 'versioning', 'last-release-sha', 'bootstrap-sha']) {
    assert.ok(!(fixed in config), `${fixed} must not pin or reshape versions`);
  }
});

test('first proposal is derived from collector history, not a fixed version', async () => {
  const bootstrap = JSON.parse(committed(MANIFEST))['.'];
  assert.equal(bootstrap, '0.0.1', 'manifest holds only the bootstrap baseline until the first release');
  const pr = await proposed({commits: COLLECTOR_HISTORY.map(commit)});
  assert.equal(pr.version.toString(), '0.1.0');
  assert.equal(pr.title.toString(), 'chore(main): release 0.1.0');
  assert.equal(pr.headRefName, 'release-please--branches--main');
  const notes = pr.body.toString();
  assert.match(notes, /### Features/);
  assert.match(notes, /collect Pocket recording snapshots through native CLI/);
  assert.match(notes, /### Bug Fixes/);
  assert.doesNotMatch(notes, /vendor soft-factory|Implement T2|non-root workspace/);

  const fixesOnly = await proposed({commits: ['fix: one', 'docs: two'].map(commit)});
  assert.equal(fixesOnly.version.toString(), '0.0.2');
  const breaking = await proposed({commits: ['feat!: incompatible', ...COLLECTOR_HISTORY].map(commit)});
  assert.equal(breaking.version.toString(), '1.0.0');

  // Why the baseline is not 0.0.0: release-please then ignores the commits.
  const zero = await proposed({manifest: '{".": "0.0.0"}', commits: ['fix: one'].map(commit)});
  assert.equal(zero.version.toString(), '1.0.0');
});

test('fix, feat and breaking changes bump patch, minor and major before and after 1.0', async () => {
  const cases = [
    ['0.1.0', ['fix: retry'], '0.1.1'],
    ['0.1.0', ['feat: new flag', 'fix: retry'], '0.2.0'],
    ['0.1.0', ['feat!: rename flag'], '1.0.0'],
    ['0.1.0', ['fix: rename\n\nBREAKING CHANGE: state format changed'], '1.0.0'],
    ['1.2.3', ['fix(collector): retry'], '1.2.4'],
    ['1.2.3', ['feat: new flag'], '1.3.0'],
    ['1.2.3', ['refactor!: drop flag', 'feat: other'], '2.0.0'],
  ];
  for (const [from, messages, expected] of cases) {
    const pr = await proposed(afterRelease(from, messages));
    assert.ok(pr, `${from} + ${messages} should propose a release`);
    assert.equal(pr.version.toString(), expected, `${from} + ${messages}`);
    assert.ok(pr.updates.some(update => update.path === 'CHANGELOG.md'));
  }
});

test('docs, chore, test, ci and build changes alone propose no release', async () => {
  const pr = await proposed(afterRelease('0.1.0', [
    'docs: explain install', 'chore: tidy', 'test: more cases', 'ci: tweak', 'build: pin', 'style: fmt',
    'refactor: simplify',
  ]));
  assert.equal(pr, undefined);
});

test('merging the release PR yields a draft v-tag release with forced tag at the merge commit', async () => {
  const pr = await proposed(afterRelease('0.1.0', ['feat: new flag']));
  const mergeSha = 'f'.repeat(40);
  const fixture = afterRelease('0.1.0', []);
  fixture.pullRequests = [{
    merged: true, number: 7, sha: mergeSha, title: pr.title.toString(), body: pr.body.toString(),
    labels: pr.labels, headBranchName: pr.headRefName, baseBranchName: 'main', files: [],
  }];
  const manifest = await Manifest.fromManifest(fakeGitHub(fixture), 'main', CONFIG, MANIFEST);
  const releases = await manifest.buildReleases();
  assert.equal(releases.length, 1);
  const [release] = releases;
  assert.equal(release.tag.toString(), 'v0.2.0');
  assert.equal(release.sha, mergeSha);
  assert.equal(release.draft, true);
  assert.equal(release.forceTag, true);
  assert.ok(!release.prerelease);
  assert.equal(release.path, '.');
});

test('a draft release with its forced tag is found as the previous release', async () => {
  const fixture = afterRelease('0.2.0', ['fix: retry']);
  fixture.releases[0].draft = true;
  const pr = await proposed(fixture);
  assert.equal(pr.version.toString(), '0.2.1');
});
