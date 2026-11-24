# #5 CI, automated release PRs and Linux installer

## Goal
Make the completed collector continuously checked and installable from versioned Linux releases. Derive versions/changelogs from Conventional Commits; the owner releases by merging an automatically maintained Release Please PR rather than selecting version numbers.

Tracking issue: [#5](https://github.com/jsburckhardt/pocketpull/issues/5). Collector agreement remains in WORK_BRIEF.md; development scope remains in WORK_BRIEF_DEV_ENV.md.

## Acceptance criteria
- AC1: main is the repository default, master is preserved, and collector #1 is closed with evidence from merged #4. | evidence: repository/issue API reads and ancestry check.
- AC2: PRs targeting main and pushes to main run formatting, vet, race tests, native HTTP-fixture collection, CGO-free Linux amd64/arm64 builds and distribution checks without Pocket credentials. Fork code receives no write token or release secrets. | evidence: workflow review/lint and a real PR CI run; existing local commands remain green.
- AC3: A local release build produces two versioned Linux tar.gz archives and SHA-256 checksums; extracted native binary reports its exact version and both binaries target the stated architecture without an application runtime. Bad versions/builds fail without presenting a partial release as complete. | evidence: temporary-directory packaging integration tests, checksums, archive inspection and native --version.
- AC4: install.sh installs the latest published release or an explicitly selected version on Linux amd64/arm64, verifies the matching archive checksum before installation, defaults to ~/.local/bin, supports --install-dir, and never invokes sudo or changes credentials/configuration/cron. Unsupported targets, download/checksum/archive failures leave an existing installation intact and clean temporary files. | evidence: fixture-driven installer subprocess tests using fake release downloads, including wrong checksum, missing/unsafe archives, upgrade/failure preservation and paths with spaces.
- AC5: Release Please maintains a version/changelog PR based on Conventional Commits; fix increments patch, feat increments minor, breaking change increments major (including pre-1.0), and non-release changes alone create no release. With no existing release, the first proposed release uses Release Please's automatic 1.0.0 baseline; later bumps derive from Conventional Commits. No manual tag or recurring version selection is required. | evidence: official tool/config validation and deterministic version-policy examples, workflow trigger/output review, documented contributor/owner flow.
- AC6: Merging the release PR causes checked source at the generated tag/SHA to build and attach both archives/checksums to the corresponding GitHub Release, without requiring a second token or depending on a bot-created tag event starting another workflow. Normal main merges update a PR, not immediately release. | evidence: workflow/action semantics, local packaging/install round trip, failure/retry/recovery path review; actual first publication remains owner-controlled and is explicitly not claimed before that merge.
- AC7: README documents installation, checksum trust/requirements, CI, Conventional Commit and release-PR policy, first-run repository permissions, version/reporting, retrying failed asset uploads and the distinction between local/CI evidence and actual release publication. AGENTS Commands contains all new build/test/lint commands. | evidence: review against executable commands/configuration and fixtures.

## Contracts
- Use main as default and PR base; retain master without force-push/deletion. Close completed #1, do not redefine collector acceptance. One issue branch delivery/5-release-distribution and one final PR; owner merges and releases.
- Go application/runtime behavior and Pocket/raw/state contracts are unchanged except release builds inject the already existing CLI version variable. Development Go/tool pins remain unchanged.
- Assets: pocketpull_<version>_linux_amd64.tar.gz and pocketpull_<version>_linux_arm64.tar.gz, plus checksums.txt. Tags use v<major>.<minor>.<patch>; archive versions omit v. Each archive contains one executable named pocketpull. Checksums cover the archives. Build with CGO_ENABLED=0; use the pinned toolchain and exact release source. No package registries, containers or extra platform guarantees.
- Installer interface: install.sh [--version vX.Y.Z|X.Y.Z|latest] [--install-dir DIRECTORY]; default latest and ~/.local/bin. Linux x86_64/amd64 and aarch64/arm64 normalize to release architecture. Prerelease/arbitrary path-like versions are rejected for this delivery. Download from this repository's GitHub Releases over HTTPS, with finite/failing curl behavior. Resolve latest once to an exact tag before downloading the version-specific archive/checksum. No insecure TLS flags, automatic privileged writes, executable download before verification, or credentials/cron setup. Document dependencies. Checksum verification protects integrity relative to the repository release, not independent publisher identity. Download/verify/extract in temporary storage; install via destination-local atomic replacement, preserving an old binary on failure. Testing may inject fake commands; do not add an arbitrary untrusted download origin as a public installer option.
- Release Please is the selected version/changelog PR mechanism. Configure normal SemVer behavior even below 1.0 (breaking change to major, feat minor, fix patch); explicitly test the configured policy. Bootstrap from no published release using the tool's automatic 1.0.0 initial-release default and collector feature history for the changelog; no manually seeded tag, fixed release-as override or recurring version selection. Docs/chore/test-only work alone does not publish. No automatic merging of either implementation or release PR.
- Use scoped built-in GITHUB_TOKEN, with default read-only CI and job-specific write permissions only for release PR/tag/assets. Enable the repository's Actions PR-creation permission if necessary, retaining read-default token permissions. Do not copy host tokens or require a new PAT/App secret. Use same workflow release-created outputs to attach assets: bot-generated tags/releases may not trigger a downstream tag workflow. Generated PR CI may need GitHub's documented workflow approval or an explicit workflow_dispatch; document/handle that boundary accurately without unsafe pull_request_target execution.
- Pin third-party actions to reviewed immutable SHAs with readable versions. Bind downloaded artifacts and release uploads to exact built source/version; serialize release operations and retain a bounded recovery path when upload fails after tag creation. No successful public release with knowingly missing assets claimed. No live Pocket data/keys, credential dumping or application telemetry.

## Constraints
- Owner approved the recommended branch/CI/Linux/installer defaults, amended manual tagging to Conventional Commit-derived versions, and selected release-pr on 2026-11-24. This grants factory-slice then factory-deliver as Lead; it does not delegate implementation merge or first release.
- Work in the existing non-root devcontainer, one implementer at a time for shared release/config/docs files, signed commits and independent task/final acceptance. Lead session c5fac65e-0ea9-4288-aa93-7440ab7136d6.
- Use the smallest coherent scripts/workflows and existing Go checks; no general-purpose release framework or duplicate command logic. Necessary tooling dependencies must be pinned and confined to tooling, not app runtime.
- Local/offline tests cannot certify GitHub-hosted execution/publication. Preserve actual cloud outcomes where available; first release requires the owner's later release PR merge and must not be reported completed in this delivery.

## Out of scope
- Collector feature changes, live Pocket pilot, automatic deployment, system services/cron provisioning, credentials/configuration provisioning, master deletion, branch-protection rewrites, other operating systems/architectures, package managers and container-image releases.
- Automatically merging PRs, manually choosing recurring versions, manually tagging an initial release during implementation, publishing binaries before owner release approval, signing infrastructure or new PAT/App-secret provisioning.

## Decisions
| # | Decision | Rationale | Decided by |
|---|---|---|---|
| D1 | main default, preserve master, close collector #1; separate #5 for distribution. | main contains all master history and accepted collector; auto-close previously missed default branch. | owner |
| D2 | CI on PR/main, Linux amd64/arm64 archives/checksums, safe latest/exact installer default ~/.local/bin with --install-dir and no sudo. | Agreed recommended distribution scope. | owner |
| D3 | Conventional Commits calculate version/changelog automatically; Release Please maintains an owner-merged release PR. | Owner does not want to keep checking/selecting new versions; retains release boundary. | owner |
| D4 | Explicit normal SemVer below 1.0; no recurring release-as override. Initial default clarified by D7. | Preserve the explained fix/feat/breaking mappings without hidden pre-major exceptions. | delegated |
| D5 | Built-in token and same-workflow release outputs, not tag-trigger chaining or a new credential. | Avoid token-event suppression and unnecessary secret provisioning. | delegated |
| D6 | First live release remains deferred until the owner merges the generated release PR. | Owner retained merge/release; do not fabricate publication evidence. | owner |
| D7 | Use Release Please's automatic first release 1.0.0 with no manual bootstrap tag; subsequent versions derive from Conventional Commits. | Owner explicitly selected this after the worker proposed an unapproved manual v0.0.0-tag prerequisite. Removes manual version/tag work. | owner |

## Open questions and spikes
None. GitHub documents GITHUB_TOKEN-generated PR workflows may require approval, and bot tag events do not start ordinary chained workflows; implement/document that explicitly. No spike or extra credential provisioning is approved.

## Tasks
### T1 A checked Linux archive installs and reports its version through a release fixture
- ACs: AC2, AC3, AC4, AC7
- Depends on: None
- Contracts: Versioned two-architecture archives/checksums, version-injected binary, safe latest/exact installer, read-only PR/main CI using the same real checks and packaging/install fixtures
- Write scope: scripts/**, install.sh, .github/workflows/ci.yml, justfile, AGENTS.md, README.md, .gitignore; internal/cli/** only if version injection needs a tightly coupled correction
- Shared files: scripts/**, justfile, AGENTS.md, README.md owned by T1 until integration then T2; Lead owns all work briefs
- Resources: Temporary output/install/fixture directories; loopback ports only if needed; no public release, real credentials or cron
- Checks: just distribution-check; just collector-check; just collector-smoke; just dev-check
- Status: integrated
- Worker: factory/release-t1; base dd5ec0af28011ad14b0df1e6bc9847a4077875d4; accepted head a4dab870d43307fac0d27dd5f813e52886f51134; integrated unchanged as 5c605f891f464a2a1f567aad741e7176a7e0dd6d; PID 324 exited; log .trees/release-t1.log

### T2 Conventional Commits produce an owner-merged release PR and tested installable assets
- ACs: AC1, AC2, AC3, AC4, AC5, AC6, AC7
- Depends on: T1
- Contracts: Release Please config/manifest, automatic version/changelog PR, normal SemVer examples, same-workflow exact-source packaging/upload, finite recovery path, scoped built-in token, documentation and complete regression evidence
- Write scope: .github/workflows/**, release-please-config.json, .release-please-manifest.json, scripts/**, justfile, AGENTS.md, README.md, .gitignore; CHANGELOG.md/version.txt only if selected strategy requires them
- Shared files: scripts/**, justfile, AGENTS.md, README.md owned by T2; Lead owns briefs/final evidence/publication
- Resources: Temporary packaging/install/version-policy fixtures; GitHub PR CI only after reviewed publication; no real release/tag/merge
- Checks: just distribution-check; just collector-check; just collector-smoke; just dev-check
- Status: blocked
- Worker: factory/release-t2; base b446965ac3770ce4948259a9f46eaf2f10ce71d2; unintegrated head f538f9e06f4c0092584576b1272ca44c1fd9800c (contains unapproved manual bootstrap tag requirement); PID 16719 exited. Narrow correction for approved D7 pending; log .trees/release-t2.log

## Status
delivering

Owner scope and go agreed. main default and collector #1 closure verified; master preserved. Base afed54e4b03d139d13d5b989ab04dadcf9c46925. Lead branch delivery/5-release-distribution. Follow independent review and serial integration; one final PR, no merge/release. Record final verdict/publication in the existing issue/PR to preserve exact reviewed SHA.

T1 task acceptance: fresh-context accept-release-t1 (factory-accept/factory-design), verdict pass for AC2 local/config portion, AC3, AC4 and AC7 T1 portion. Base dd5ec0af28011ad14b0df1e6bc9847a4077875d4; reviewed head a4dab870d43307fac0d27dd5f813e52886f51134. All four required just checks pass non-root; actionlint v1.7.12 passes via cached source. Reviewer additionally verified actual archive-download TERM cleanup and architecture aliases. Minor F1: scripts/test_distribution.py:347-356 signals during latest resolution before scratch exists; T2 owns strengthening it to an exact-version download with scratch-exists assertion. No blocker/major finding. Cloud PR CI remains final publication evidence, not locally claimed. Lead verified signature, exited PGID 324, integrated exact unchanged patch as 5c605f891f464a2a1f567aad741e7176a7e0dd6d, and reran just distribution-check; just collector-check; just collector-smoke; just dev-check: all exit 0. T2 unlocked.
