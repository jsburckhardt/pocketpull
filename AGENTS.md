<!-- soft-factory-v3:begin -->
## How we deliver
Start sessions with the `foreman` agent (`copilot --agent foreman` or `/agent`). It syncs state, picks the phase and runs
the `factory-*` skills in `.github/skills/`. Vendored from jsburckhardt/toolio plugins/soft-factory-v3 at bd0eaa7.

## Docs
- Keep README short: what it is, quickstart, links. Everything else goes in `docs/`.
- Read `docs/architecture/decisions.md` first; it indexes ADRs and components.
- Dated files are `YYYYMMDD-<slug>.md` (work, adr, spikes).

## Rules
- Questions are not authorisation to change things.
- The owner merges and releases unless delegated.
- Never commit secrets; read credentials from the environment or owner-approved secret files.
- Tooling uses Go or plain bash; another runtime needs owner approval (existing Python scripts and the Node release-policy check are already approved).
- Do not create unrequested side files; clean up what you create.
- When a task changes build/test/lint commands, update `## Commands` in the same task.

## Commands
- `just collector-check`: reject unformatted Go, run `go vet` and `go test -race`, and build CGO-free Linux amd64/arm64 binaries in a cleaned temporary directory.
- `just collector-smoke`: native CLI subprocess collects one recording from a loopback HTTP fixture using environment/flags and config-file/secret-file with CLI overrides, without real credentials.
- `just distribution-check`: syntax-check `install.sh` and `scripts/package-release.sh`, package two real versions into temporary directories (archives, checksums, static ELF arch, native `--version`), run installer subprocess tests against fake `curl`/`uname` release downloads, and run `scripts/test_release.py` (real packaging published by `scripts/release-assets.py` to a fake `gh`, recovery/rejection cases and an install round trip), without network or credentials.
- `just release-policy-check`: requires Node >= 24 and npm (CI pins Node 24.22.3; not in the devcontainer, use a temporary checksum-verified Node locally); `npm ci` of the locked `scripts/release-policy` tooling (release-please 17.6.0, as bundled by the pinned action) and real Release Please version-policy checks of the committed config/manifest against an in-memory GitHub.
- `just workflow-lint`: pinned `actionlint` v1.7.12 via `go run` over `.github/workflows` (runs shellcheck on `run:` scripts when shellcheck is on PATH, as on CI runners).
- `scripts/release-assets.py resolve|publish` (release workflow only; needs `gh`, `GH_REPO`, `GH_TOKEN`): validate an existing Release Please release tag and upload/verify its assets; see README Releases.
- `just package-release VERSION OUTPUT_DIR` (`scripts/package-release.sh`): build `pocketpull_<version>_linux_{amd64,arm64}.tar.gz` and `checksums.txt` with the pinned Go, CGO-free and version-injected, into a new directory created only on success.
- CI (`.github/workflows/ci.yml`, PRs to/pushes on `main`, also called by release builds): read-only token runs `just distribution-check`, `just collector-check`, `just collector-smoke`, `just dev-check`, `just release-policy-check` and `just workflow-lint`.
- Release (`.github/workflows/release.yml`, pushes on `main`, dispatch `-f tag=vX.Y.Z` for asset retry): Release Please PR/draft release, exact-commit CI build and asset publication; see README Releases.
- `just dev-check`: host/container configuration, feature-lock, shell-syntax and recipe checks.
- `devcontainer build --workspace-folder .`: build the pinned Ubuntu workspace.
- `devcontainer up --workspace-folder .`: start the non-root workspace (requires host `~/.ssh`).
- `devcontainer exec --workspace-folder . just dev-smoke`: exercise installed tools, user, mounts and shell history.
- `just dev-signing-smoke`: host-only disposable-key devcontainer integration; verifies SSH-signed commits through the read-only SSH mount and cleans up its own resources.
- `devcontainer exec --workspace-folder . just dev-telemetry-smoke`: authenticated, container-only clean-profile Copilot/loopback OTLP protobuf check; permits only the automatic `enduser.pseudo.id` identity field and rejects other identity/content capture (see README).

See README.md for collector usage, development authentication/signing, optional external telemetry, scope boundaries and prerequisites. Collector checks cover native builds and fixture-based collection, configuration/secrets, freshness/state/locking, bounded retries, partial outcomes and interruption/restart; live Pocket experiments, publication and independent acceptance are separate. Telemetry is off without a supplied endpoint. Under #2 D21, only automatic `enduser.pseudo.id` is permitted; it can correlate sessions and is not assumed anonymous. Keep identity/content capture disabled; all other enduser fields, names, raw IDs and OS/host identity remain prohibited.
<!-- soft-factory-v3:end -->
