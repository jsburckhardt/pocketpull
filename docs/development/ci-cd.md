# Packaging, CI and releases

## Local packaging

```sh
just package-release v1.2.3 /path/to/new-output-dir
```

`scripts/package-release.sh VERSION OUTPUT_DIR` accepts `X.Y.Z` or `vX.Y.Z`
(canonical SemVer core only), requires the pinned Go toolchain from `go.mod`
(`GOTOOLCHAIN=local`), and builds CGO-free, statically linked Linux amd64 and
arm64 binaries with the version injected into `internal/cli.version`. It writes
`pocketpull_<version>_linux_amd64.tar.gz`,
`pocketpull_<version>_linux_arm64.tar.gz` (each containing only `pocketpull`,
mode 0755) and `checksums.txt` (`sha256sum` format over both archives). Output
is staged beside `OUTPUT_DIR` and renamed into place only after every step
succeeds; an existing `OUTPUT_DIR` is refused and never modified, and invalid
versions or failed builds leave no output. Archive timestamps use
`SOURCE_DATE_EPOCH` (default 0).

## Checks and CI

```sh
just distribution-check
```

This syntax-checks both scripts, then packages two real versions in temporary
directories and verifies archive contents, checksums, static ELF architecture
and the native `--version`. It runs `install.sh` as a subprocess with fake
`curl`/`uname` commands on `PATH` serving those real archives (no network): a
latest and exact-version round trip with the real binary, upgrade, arm64
selection, and failure preservation/cleanup for wrong, missing, duplicate,
path-like or malformed checksum entries, missing downloads, non-gzip, extra,
traversal, nested, symlink, hardlink, directory and non-executable members,
unsupported OS/architecture, invalid versions, foreign/prerelease latest tags,
interruption, paths with spaces, and no sudo/crontab use.

`distribution-check` also runs `scripts/test_release.py`: it packages a real
release, publishes it through `scripts/release-assets.py` to a fake `gh` (no
network or token) and installs it back with `install.sh`, and covers partial
upload recovery, failed uploads, differing existing assets, latest selection,
incomplete asset directories and rejected tags/commits.

```sh
just release-policy-check   # needs Node >= 24 and npm (not in the devcontainer)
just workflow-lint          # pinned actionlint v1.7.12; uses shellcheck if on PATH
```

`release-policy-check` installs the locked tooling in `scripts/release-policy`
(`npm ci`, release-please 17.6.0, the version bundled by the pinned action) and
runs the committed Release Please config and current manifest, plus history
fixtures, through the real library against an in-memory GitHub. It covers the
automatic initial 1.0.0 proposal from collector-style history without tags,
fix/feat/breaking bumps before and after 1.0, no release for
docs/chore/test/ci/build-only changes, and the draft `vX.Y.Z` release with a
forced tag at the release PR merge commit. The initial-history fixture remains
empty after the real manifest advances.

[`.github/workflows/ci.yml`](../../.github/workflows/ci.yml) runs on pull requests
targeting `main`, pushes to `main` and manual `workflow_dispatch`. On a
non-root `ubuntu-24.04` runner it installs the same Go 1.27.4 and just 1.59.0
releases (with the same SHA-256 digests) as the devcontainer plus Node 24.22.3
(SHA-256 pinned, used only for release tooling), then runs
`just distribution-check`, `just collector-check`, `just collector-smoke`,
`just dev-check`, `just release-policy-check` and `just workflow-lint`. The
token is read-only (`contents: read`), checkout does not persist credentials,
there is no `pull_request_target` and no secrets or Pocket credentials, so fork
pull requests receive no write token or release secrets. Actions are pinned to
full commit SHAs. Newer runs of the same pull request cancel older ones; `main`
runs are not cancelled.

Local and CI checks are evidence about the code and fixtures only; they do not
publish, tag or prove a GitHub Release.

## Releases

Versions and the changelog come from
[Conventional Commits](https://www.conventionalcommits.org/) on `main`, via
[Release Please](https://github.com/googleapis/release-please)
([`release-please-config.json`](../../release-please-config.json),
[`.release-please-manifest.json`](../../.release-please-manifest.json)). Nobody
chooses or edits version numbers.

With no previous release, the manifest starts empty and Release Please proposes
its automatic initial version, **1.0.0**, when it finds releasable commits. The
collector's features and fixes appear in the first changelog. Non-release changes
alone do not create a release. No manual seed or tag is used, and no `release-as`
or `initial-version` override is set. The release PR records the version in the
manifest; subsequent versions follow these rules:

| Change on `main` | Next version |
|---|---|
| `fix: …` | patch (`0.1.0` → `0.1.1`) |
| `feat: …` | minor (`0.1.0` → `0.2.0`) |
| `feat!: …`, `fix!: …` or a `BREAKING CHANGE:` footer | major, also before 1.0 (`0.1.0` → `1.0.0`) |
| only non-breaking `docs`, `chore`, `test`, `ci`, `build`, `style`, `refactor` | no release |

`bump-minor-pre-major` and `bump-patch-for-minor-pre-major` are `false`, so
breaking changes are never downgraded below 1.0. Contributors: the commit that
lands on `main` decides the version, so give squash-merge commits (or every
commit of a merge/rebase) a Conventional Commit subject; with squash merging,
the PR title is normally the commit subject. Non-conventional subjects are
ignored.

Owner flow:

1. Each push to `main` runs [`.github/workflows/release.yml`](../../.github/workflows/release.yml).
   Release Please opens or updates one PR, `chore(main): release X.Y.Z`, which
   bumps the manifest and `CHANGELOG.md`. Merging ordinary PRs only updates it;
   nothing is released.
2. Review that PR. Because it is created with the built-in `GITHUB_TOKEN`, its CI
   runs wait for a maintainer to select **Approve workflows to run** in the PR
   (or run CI manually: `gh workflow run ci.yml --ref release-please--branches--main`).
   Merge it when ready; no one ever merges it automatically.
3. The merge push runs the same workflow: Release Please creates a **draft**
   GitHub Release and its `vX.Y.Z` tag at the merge commit (`draft` +
   `force-tag-creation`, so the tag exists for the next release's history).
   In the same run (no tag-triggered workflow, no extra token), `resolve`
   checks the tag is `vX.Y.Z`, points to the reported commit, is on `main` and
   that commit's manifest records `X.Y.Z`. `build` runs every CI check on that
   exact commit with a read-only token and packages it with
   `just package-release` (checking the native `--version` and checksums).
   `publish` uploads the two archives and `checksums.txt`, then publishes the
   draft (marked latest only if it is the highest stable version). Installers
   never see a release before all assets are attached.

Before the first run, ensure Settings → Actions → General → Workflow permissions
keeps the default **read** token and enables **Allow GitHub Actions to create
and approve pull requests**, which Release Please needs to open its PR. Jobs
request only what they use: CI and `build` read; `release-please` writes
contents/issues/pull requests; `resolve` and
`publish` write contents (drafts are visible only with push access) and run
only the workflow's own `main` script, never code from the tag, before the
read-only build. Release operations share one non-cancelling concurrency group.
If [immutable releases](https://docs.github.com/repositories/releasing-projects-on-github/about-releases)
are enabled, the draft-until-complete order still allows the asset upload.

Retrying a failed asset upload: `publish` never deletes, retags or replaces
anything. It uploads only missing assets, requires any existing asset to be
byte-identical to the rebuilt one (builds are reproducible) and publishes the
draft last, so rerunning is safe. If `build` or `publish` fails, use **Re-run
failed jobs** on that run (it reuses the earlier release outputs; **Re-run all
jobs** would find nothing new to release). Otherwise, for an existing Release
Please release whose assets are incomplete:

```sh
gh workflow run release.yml --ref main -f tag=vX.Y.Z
```

This only accepts a `vX.Y.Z` tag that already has a GitHub Release created from
a merged release PR (on `main`, matching manifest); it cannot create tags,
releases or versions, or release other commits. If an existing asset differs,
the job fails naming it; inspect it, delete that asset by hand, and rerun.
Until it succeeds the release stays a draft, or, for an already published
release, the missing assets are listed in the failure.

Locally tested here: packaging, publication logic against a fake `gh`, the
Release Please policy and workflow lint. GitHub-hosted runs, the first release
PR and the first published release are only evidenced by the real runs after
the owner merges them; none is claimed yet.
