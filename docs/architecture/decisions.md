# Decisions

Index of ADRs and components. Read this first.

| Date | Kind | Title | Status | Applies to | Link |
|---|---|---|---|---|---|
| 20261123 | ADR | Run once; cron owns scheduling | accepted | internal/cli, cmd/pocketpull | [ADR](adr/20261123-run-once-collector.md) |
| 20261123 | ADR | One-way latest-state local archive of full detail JSON | accepted | internal/collector, raw directory | [ADR](adr/20261123-one-way-local-archive.md) |
| 20261123 | ADR | Go, CGO-free static Linux binary for amd64 and arm64 | accepted | whole repository, packaging | [ADR](adr/20261123-go-static-linux-binary.md) |
| 20261123 | ADR | One strict YAML file plus explicit CLI overrides | accepted | internal/cli | [ADR](adr/20261123-strict-single-yaml-config.md) |
| 20261123 | ADR | API key from the environment or an owner-only secret file | accepted | internal/cli | [ADR](adr/20261123-api-key-env-or-secret-file.md) |
| 20261123 | ADR | Full listing every run with bounded detail freshness and private state | accepted | internal/collector, state directory | [ADR](adr/20261123-bounded-freshness-private-state.md) |
| 20261123 | ADR | Reject overlap, retry within bounds, keep partial progress | accepted | internal/collector, internal/cli | [ADR](adr/20261123-overlap-retries-partial-progress.md) |
| 20261123 | ADR | JSON operational logs on stderr with content and credential redaction | accepted | internal/cli, internal/collector | [ADR](adr/20261123-redacted-stderr-logs.md) |
| 20261123 | ADR | Pinned devcontainer with a read-only SSH mount for signing | accepted | .devcontainer | [ADR](adr/20261123-pinned-devcontainer-ssh-signing.md) |
| 20261123 | ADR | Optional metadata-only telemetry; only enduser.pseudo.id permitted | accepted | .devcontainer, Copilot sessions | [ADR](adr/20261123-metadata-only-telemetry.md) |
| 20261124 | ADR | Checksum-verified Linux installer without sudo | accepted | install.sh, scripts/package-release.sh | [ADR](adr/20261124-verified-linux-installer.md) |
| 20261124 | ADR | Versions from Conventional Commits via an owner-merged Release Please PR | accepted | .github/workflows/release.yml, release-please-config.json | [ADR](adr/20261124-release-please-conventional-commits.md) |
| 20261123 | component | Collector | active | internal/collector | [component](components/collector.md) |
| 20261123 | component | CLI | active | internal/cli, cmd/pocketpull | [component](components/cli.md) |
