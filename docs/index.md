# Docs

- [Getting started](getting-started.md): what the collector does, build from source, install a release, versions.
- Usage
  - [Configuration and credentials](usage/configuration.md): flags, config file discovery, YAML keys, secret sources.
  - [Collection behaviour](usage/collection.md): refresh policy, private state, locking, raw artifacts, retries and recovery.
- Operations
  - [Cron scheduling](operations/scheduling.md): unattended setup and crontab example.
  - [Logs and exit status](operations/logging.md): log formats, levels, summary fields, exit codes.
- Development
  - [Development workspace](development/environment.md): devcontainer, authentication, SSH commit signing.
  - [Testing](development/testing.md): collector and workspace checks.
  - [Packaging, CI and releases](development/ci-cd.md): local packaging, CI checks, Release Please flow.
  - [Telemetry](development/telemetry.md): optional OTLP export and its privacy limits.
- Architecture: [decisions](architecture/decisions.md), index of ADRs and components; read first.
- [Work briefs](work/): delivery briefs (`YYYYMMDD-<slug>.md`).
