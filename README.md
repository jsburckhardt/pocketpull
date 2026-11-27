# pocketpull

Scheduled Pocket transcript collector: a native, run-once Go CLI that copies
every Pocket recording's transcript and summaries into a raw folder, meant to
be run by cron.

## Quickstart

Install the latest Linux amd64/arm64 release into `~/.local/bin` (review the
script first; no release is published yet, so
[build from source](docs/getting-started.md#build-from-source) until then):

```sh
curl -fsSL --proto '=https' -o install.sh \
  https://raw.githubusercontent.com/jsburckhardt/pocketpull/main/install.sh
bash install.sh
```

Collect once with a key read from the terminal:

```sh
read -r -s -p 'Pocket API key: ' POCKETPULL_API_KEY; printf '\n'
export POCKETPULL_API_KEY
pocketpull --raw-dir "$HOME/pocket-raw"
unset POCKETPULL_API_KEY
```

## Docs

- [Getting started](docs/getting-started.md): build, install, versions.
- [Configuration and credentials](docs/usage/configuration.md) and [collection behaviour](docs/usage/collection.md).
- [Cron scheduling](docs/operations/scheduling.md) and [logs and exit status](docs/operations/logging.md).
- [Development workspace](docs/development/environment.md), [testing](docs/development/testing.md), [packaging, CI and releases](docs/development/ci-cd.md), [telemetry](docs/development/telemetry.md).
- [All docs](docs/index.md). Contributors and agents: see [AGENTS.md](AGENTS.md).
