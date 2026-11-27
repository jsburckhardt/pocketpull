# Checksum-verified Linux installer without sudo

- Status: accepted
- Date: 20261124
- Source: `docs/work/20261124-release-distribution.md` D2

## Context
Users need a safe way to install the latest or an exact release on Linux amd64/arm64 without trusting an unverified download or granting root.

## Decision
Publish per-architecture archives plus `checksums.txt` on each GitHub Release. `install.sh` resolves `latest` to an exact tag, verifies the archive's SHA-256 and contents before anything runs, and atomically installs into `~/.local/bin` (or `--install-dir`) without sudo.

## Consequences
- Easier: failed downloads or checks leave an existing install untouched.
- Harder: checksums prove integrity relative to the release, not publisher identity; there is no signing infrastructure.
- Forbidden: sudo, writing credentials, config, profiles or cron entries; other OSes, package managers and container images without a new decision.

## Alternatives considered
None recorded.
