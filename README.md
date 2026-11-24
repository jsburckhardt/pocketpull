# pocketpull

Scheduled Pocket transcript collector.

Delivery skills in `.github/skills/` are vendored from jsburckhardt/toolio `soft-factory-v2` at `3d07eb5`.

## Collector: run-once native collection

`pocketpull` is a native run-once Go CLI, not a scheduler. It lists every account
page from `https://public.heypocketai.com/api/v1/public/recordings?page=1&limit=100`,
increments `page` while `pagination.has_more` is true, deduplicates IDs, and
fetches `/public/recordings/<id>` with Bearer authentication. Details include
transcript and summarizations by the API's defaults. No audio route, date/tag
filter, extra content service, remote edit, or local pruning is used.

Build into a temporary directory and invoke with an explicitly exported secret:

```sh
build_dir="$(mktemp -d)"
trap 'rm -f -- "$build_dir/pocketpull"; rmdir -- "$build_dir"' EXIT
CGO_ENABLED=0 go build -o "$build_dir/pocketpull" ./cmd/pocketpull
"$build_dir/pocketpull" --help
"$build_dir/pocketpull" --version
# Bash: supply your key without placing it in shell history.
read -r -s -p 'Pocket API key: ' POCKETPULL_API_KEY
printf '\n'
export POCKETPULL_API_KEY
"$build_dir/pocketpull" --raw-dir "$HOME/pocket-raw"
unset POCKETPULL_API_KEY
```

The binary needs no Go/Python/Node runtime on the collection host. To build
either Linux target, use `CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o
pocketpull-amd64 ./cmd/pocketpull` or the same command with `GOARCH=arm64` and
`-o pocketpull-arm64`. Build tools are needed only on the build host; choose the
binary matching the collection host's architecture.

### Configuration and credentials

The public collection flags are `--config`, `--raw-dir`, `--api-key-file`,
`--state-dir`, `--refresh-after`, `--log-format`, and `--log-level`. Help/version
read no configuration or credentials and send no requests; the unversioned
development build reports `pocketpull dev`. There is no literal-key flag or
public API endpoint override.

Select **exactly one** configuration file: explicit `--config FILE`, otherwise
`./pocketpull.yaml`, otherwise `$XDG_CONFIG_HOME/pocketpull/config.yaml`
(or `$HOME/.config/pocketpull/config.yaml` when the XDG base is unset/empty).
No files are merged. A missing explicit file is an error. An unreadable or
invalid selected/discovered file is an error, never permission to fall back.
If neither discovery file exists, flags and defaults may supply configuration.
Explicit flags override the selected file, including `--refresh-after 0`.
File-configured paths resolve against that file's directory; CLI paths resolve
against the working directory. Paths are not shell-expanded: use absolute paths
or relative paths, not literal `~` or `$HOME` inside YAML.

YAML must be one strict mapping/document with only these six keys:

```yaml
raw_dir: ../../pocket-raw
api_key_file: api-key
state_dir: ../../.local/state/pocketpull
refresh_after: 24h
log_format: json
log_level: info
```

These example paths assume the file is in `$HOME/.config/pocketpull/`.
[`examples/pocketpull.yaml`](examples/pocketpull.yaml) is a copyable version.
All supplied values must be nonempty strings; unknown/duplicate keys, multiple
documents, malformed YAML, aliases and incorrect types are rejected, even when
a CLI override would replace an invalid file value. `raw_dir` is mandatory:
there is no implicit output folder. `refresh_after` defaults to `24h` and uses
Go duration syntax, such as `30m`, `1h30m`, or `"0"` (quote YAML zero to keep it
a string). Zero selects full refresh; negative/invalid durations are rejected.
The default state base is `$XDG_STATE_HOME/pocketpull`, or
`$HOME/.local/state/pocketpull` when the XDG base is unset/empty.

Read exactly one nonempty secret source: `POCKETPULL_API_KEY` **or** the selected
`api_key_file`/`--api-key-file`. Both sources conflict even if equal, empty, or
whitespace-only; an explicitly empty environment key or unusable file never
falls back to another source. Leading/trailing whitespace is trimmed; embedded
newlines are rejected. Secret files must be regular, usable, owned by the
effective user, and have no group/other read permission. Provision the secret
through your secret workflow and use `chmod 600 /path/to/api-key`; the collector
does not change permissions. Do not place a literal `api_key` in YAML.

No `.env` file is automatically loaded. A shell or development tool may
explicitly export the environment key, but keep `.env` gitignored. Keep all
secrets out of command arguments, tracked files, images and build arguments.
Configuration/parser/file errors report only redacted operational categories,
not supplied values or paths.

### Refresh policy and private state

Every run lists **all** pages and deduplicates IDs. New recordings are fetched
immediately, before requesting later pages. Existing recordings are skipped only
when their raw snapshot is usable and the last successful detail fetch is
strictly less than `refresh_after` old. At the exact boundary they are fetched;
`--refresh-after 0` always fetches details. Valid pending recordings use the same
interval. Titles and `updated_at` are not freshness signals.

Successful unchanged refreshes advance freshness without rewriting raw data.
The timestamp is measured when the detail fetch completes, not at run start;
HTTP, validation and raw-write failures never advance it. Missing, malformed,
unreadable or otherwise unusable state/output forces a safe refetch. State is
expendable: removing it does not remove the archive. If state cannot be persisted,
collection still retains successful raw snapshots, logs the failure and exits
nonzero; restoring usable state storage permits recovery on the next run.

Private state lives under `<state-base>/<sha256(canonical-raw-path)>/`, with
one `<sha256(recording-id)>.json` successful-fetch timestamp per recording.
Namespaces are owner-only directories (0700) and timestamp files are 0600.
The configured state base must be outside raw, including symlink aliases;
no cache or lock markers are written into raw. Raw is safely created before
canonicalisation, so spelling it through a symlink reuses the same namespace.
Different raw directories have independent freshness even with one state base.
Changing the state base forces refetching unless it already holds usable state.

One collection per canonical raw directory is enforced by a nonwaiting Linux
`flock` on an open raw-directory descriptor. Overlap fails nonzero before any
collection request, even with a different state base or a raw-path alias.
Unrelated raw directories can run concurrently. Normal exit or process death,
including SIGKILL, releases the lock automatically; there is no stale PID file.
Raw snapshots are committed before atomic state writes, and successful progress
is persisted per recording, not only at the end of traversal. Interruption can
require a repeat fetch but cannot make an uncommitted raw snapshot fresh.

### Raw artifacts and failure recovery

Each snapshot is `<raw-dir>/<recording-id>/recording.json`: the **entire detail
`data` object**, not its envelope. Unknown fields and exact large JSON integers
survive alongside segments, speakers, timing, tags and API-provided summaries.
IDs must be 1–255 ASCII characters, start with a letter/digit, and thereafter
contain only letters/digits, dots, underscores or hyphens. Unsafe IDs and
mismatched detail IDs fail before snapshot writes. Recording-folder symlinks
are rejected. New directories are owner-only and new snapshot files are mode
0600; run the downstream consumer as the same user or explicitly arrange access.

Changed content replaces the previous file with a synced, same-directory
temporary file and atomic rename. Equivalent JSON (including reordered keys,
whitespace and numeric spelling) is not rewritten. Invalid/failed responses
and unsuccessful writes retain prior snapshots; remote absence/404 never
removes them. Valid missing/null transcript data is pending, not an error.
An upstream edit or completed transcript can take up to `refresh_after`
(24 hours by default) plus the cron interval and run time to appear. No guarantee
applies to content that disappears before a successful fetch. Zero refresh
reduces this latency to the cron interval plus run time, at the cost of fetching
every detail on every run.

Both listing and detail requests retry transient network failures (including
timeouts, connection resets and truncated response bodies), HTTP 429, and HTTP
500-599. Each request has at most **four attempts**, with backoff of 500 ms,
1 second and 2 seconds. A valid `Retry-After` integer in seconds or HTTP date
can lengthen a backoff, but is capped at **10 seconds**. Invalid, negative,
overflowing or past values do not extend the backoff. Each attempt has a
**30-second timeout**, covering response-body reading, and a **32 MiB response
limit**. The complete request therefore has at most 120 seconds of request
time plus 30 seconds of retry waits (excluding local processing); no upstream
numeric quota is assumed. Oversized or malformed successes, permanent 4xx and
redirects are not retried; redirects are never followed. 401/403 stop all
further list/detail work immediately.

Individual detail/validation/storage failures do not prevent other distinct
IDs from being processed; a failed listing stops traversal rather than claiming
it finished. Required failures exit nonzero while retaining successful partial
work and prior snapshots. SIGINT/SIGTERM cancel in-flight requests and backoff
promptly, report an incomplete summary, and release the lock. SIGKILL cannot
report a summary but also releases the lock. Restart uses per-record committed
freshness, safely refetches unfinished work, and never needs a lock-file removal.
A killed write can leave an uncommitted `.recording-*.tmp` file; it is not an
archive snapshot or freshness entry and is never used on restart. Gracefully
canceled writes clean up their temporary files.

### Logs and exit status

Go `slog` operational logs go to stderr; collection/error stdout remains empty.
`log_format`/`--log-format` accepts `json` (default) or `text`.
`log_level`/`--log-level` accepts `info` (default), `debug`, `warn`, or `error`:
`info` includes start, fetched-record, summary and successful `run_end` events;
`debug` additionally includes page-request and cache-hit events without request
contents; `warn` includes retry and state warnings but suppresses info/debug;
`error` shows errors only. Successful summaries are info-level,
incomplete-run summaries are error-level. Invalid configuration always uses a
safe JSON/error baseline, even if it attempted to select another format/level.
Events include a random, identity-free run ID, recording ID where safe, elapsed
time for detail/retry/run outcomes, and a final summary with `elapsed_ms` and
`complete`. Only successful runs emit `run_end`. `request_retry` warnings include
the failed attempt number, maximum attempts, operation, redacted reason and
planned delay; canceled waits do not count as additional attempts.
`discovered` counts unique IDs in validated listing pages, including rejected
unsafe IDs and IDs not attempted after interruption/authentication failure;
it does not claim unseen later pages were traversed. `fetched` counts
successfully validated and stored/unchanged details, `unchanged` is a subset
of `fetched`, and `pending` counts fetched records with missing/null transcript.
`cached` counts usable snapshots skipped by freshness (not fetched this run).
`pending` is based only on fetched detail data, never inferred for cache skips.
`failed` counts failed listing/detail/storage/state work, rejected IDs and
run-level coordination/interruption failures, not each failed retry attempt;
`retries` counts additional request attempts actually started after backoff.
A fetched snapshot can also have a state-write failure, and listing/run failures
have no recording ID, so counts need not partition `discovered`.
Logs exclude titles, transcript/summary content,
HTTP bodies, headers, keys, URLs and raw transport/filesystem errors.
Individual detail failures retain partial progress and exit nonzero; 401/403
stop promptly. Exit zero includes valid pending records, not necessarily
finished upstream transcripts. Exit 1 means invalid configuration, overlap,
interruption or incomplete required work. Help/version exit zero without
collecting; normal successful collection also exits zero.

### Cron

Install the binary at a stable absolute path yourself. The secret-file option
is recommended for cron, whose environment/working directory may differ from
your shell. From this checkout, prepare the configuration:

```sh
install -d -m 700 "$HOME/.config/pocketpull"
cp examples/pocketpull.yaml "$HOME/.config/pocketpull/config.yaml"
# Provision $HOME/.config/pocketpull/api-key through your secret workflow first.
chmod 600 "$HOME/.config/pocketpull/api-key"
unset POCKETPULL_API_KEY
/usr/local/bin/pocketpull --config "$HOME/.config/pocketpull/config.yaml"
```

Example crontab for a user whose home is `/home/collector` (hourly; native
nonwaiting overlap protection is built in):

```cron
15 * * * * /usr/local/bin/pocketpull --config /home/collector/.config/pocketpull/config.yaml 2>>/home/collector/pocketpull.log
```

Create the parent directories and provision the secret before enabling this
example. Cron and
external tooling own scheduling and log rotation. There is no application log
manager or telemetry exporter. At the default refresh interval this hourly
schedule can take up to 25 hours plus run time to capture an edit. To force an
immediate full refresh, invoke the same configuration with `--refresh-after 0`.
These examples do not imply a live Pocket pilot, release publication or cloud
validation.

```sh
just collector-check
just collector-smoke
```

These use synthetic loopback HTTP fixtures and temporary raw/state directories only.
`collector-check` rejects unformatted Go (it does not silently format), runs
vet/race tests, and builds CGO-free Linux amd64 and arm64 binaries with no
installed application runtime requirement. `collector-smoke` runs a native
subprocess through the real CLI entry using an internal, test-only fixture seam,
covering environment/flags and config-file/secret-file with CLI overrides;
neither command contacts Pocket or needs real credentials.

## Development workspace

Use Docker and the [Dev Container CLI](https://github.com/devcontainers/cli)
(`devcontainer`; verified with 0.86.1), or VS Code's Dev Containers extension.
Linux amd64 and arm64 are supported by the pinned downloads. The Ubuntu 24.04
image uses non-root `vscode`, Go 1.27.4, Git 2.56.1, GitHub CLI 2.105.0,
Copilot CLI 1.0.114, Herdr 0.9.5, just 1.59.0 and zsh 5.9-6ubuntu2.
Go, Herdr and just downloads have reviewed, architecture-specific SHA-256
checks; official Dev Container features are locked in `devcontainer-lock.json`.
Copilot automatic updates and Go's automatic toolchain downloads are disabled.
To upgrade, review upstream releases/digests, update pins and smoke expectations,
regenerate the feature lock with
`devcontainer build --workspace-folder . --experimental-lockfile`, and rerun checks.

On the host, ensure `$HOME/.ssh` exists before opening the container. This is
the **only credential directory bind mount**, mounted read-only at
`/home/vscode/.ssh`. Read-only prevents modification, **not reading or use of
private keys by container processes**. Use only a trusted workspace and tools.
There is no Docker socket, whole-home, production-data or Pocket credential
mount, privileged mode, Azure CLI or service sidecar. This is not a hardened
sandbox. Herdr is installed but never automatically started; invoking normal
Herdr commands may start its local session server.

From a normal checkout, run on the host:

```sh
devcontainer build --workspace-folder .
devcontainer up --workspace-folder .
devcontainer exec --workspace-folder . just dev-smoke
```

The CLI returns the created container ID; stop that specific container with
`docker stop <containerId>` when finished. Do not stop other developers' containers.
For linked Git worktrees, Git's common directory must also be accessible for
repository operations; this config intentionally does not mount parent
repositories or other worktrees. Use a normal checkout for routine development.

Named volumes scoped by Dev Container ID persist **only** `~/.config/gh`,
`~/.copilot` and `~/.shell-history`. Bash/zsh history is retained; do not enter
tokens in command lines. Post-create fixes these volume roots' ownership and
adds shell-history setup idempotently; it does not authenticate or start services.
Authentication volumes can contain plaintext tokens, and Copilot state can
contain session content. Treat them as sensitive, log out before disposal,
and remove only the corresponding named volumes after removing their container.
No host authentication directory other than the approved SSH mount is imported.

### Development authentication

Authenticate **interactively inside the container**, never in the Dockerfile
or build arguments. Use a development account with only the permissions needed:

```sh
gh auth login --hostname github.com --git-protocol https --web
gh auth setup-git
copilot login --device-code
```

GitHub CLI authentication and Copilot access are separate: Copilot requires an
account with Copilot access. The device-code flow lets you authorize in a host
browser without mounting browser or host credential state. No access token,
production Pocket key or production archive belongs in repository files or
image layers. `gh auth logout` and `/logout` in an interactive `copilot` session
remove saved login state.
Git identity/signing settings below are not in the persisted auth volumes;
reapply them after rebuilding the container.

### SSH commit signing

On the host, provision your own SSH signing key, protect its directory/key
permissions, and register **only its public key** as a signing key in your
GitHub account's SSH and GPG key settings. Nothing generates, copies or changes
your real keys in this setup. Then configure Git inside the container, replacing
the example identity and key filename with yours:

```sh
git config --global user.name "Your Name"
git config --global user.email "you@example.com"
git config --global gpg.format ssh
git config --global user.signingkey "$HOME/.ssh/id_ed25519"
git config --global commit.gpgsign true
mkdir -p "$HOME/.config/git"
printf 'you@example.com %s\n' "$(cat "$HOME/.ssh/id_ed25519.pub")" \
  > "$HOME/.config/git/allowed_signers"
git config --global gpg.ssh.allowedSignersFile "$HOME/.config/git/allowed_signers"
```

`allowed_signers` enables **local verification**, not signing authorization on
GitHub; use the email/principal matching the commit identity. Keep passphrase
protection; an encrypted private key may prompt on the terminal. This setup
does not forward an SSH agent or weaken signing when a key is unavailable.
Do not try to chmod or rewrite keys from the read-only container mount.
After making a signed commit, `git verify-commit HEAD` verifies it locally.

Repository-local signing settings override global settings. If the shared
checkout already selects an agent-backed public key, use a command-scoped
private-key path inside the container rather than changing the host checkout's
Git configuration (replace the example key filename):

```sh
git -c user.signingkey="$HOME/.ssh/id_ed25519" commit
```

### Workspace checks without real credentials

Run `just dev-check` on the host (Python 3 and just required) or in the container
to validate the configuration boundary, feature lock, shell syntax and recipes.
Inside a running devcontainer, `just dev-smoke` exercises the non-root user,
pinned tool binaries, read-only SSH mount, writable auth/history volumes,
persistent shell-history setup and absence of a Docker socket or Herdr server.
These are workspace checks, not placeholder collector/application checks.

For the full disposable signing check, run **on the host**:

```sh
just dev-signing-smoke
```

This creates a temporary host HOME/`.ssh` and an unencrypted **test-only**
Ed25519 key at runtime. It boots a task-specific devcontainer using the real
configuration and a unique label, runs `devcontainer exec ... just dev-smoke`,
inspects the actual user, security options and bind mounts, and proves that a
write to the mounted SSH directory fails with `EROFS`. It makes and verifies
an SSH-signed commit in a temporary repository inside the container, checks
post-create idempotence, rejects root execution of the smoke check, and confirms
the host key/directory are unchanged. It removes its container, newly created
volumes and disposable keys even on ordinary test failure. It never reads or
copies your real SSH directory, changes this repository's Git configuration,
or authenticates with GitHub. Interrupted/forcibly killed checks may require
cleanup of resources bearing their exact `pocketpull.signing-smoke` label.

### Telemetry

Telemetry is **optional and off without an endpoint**. No telemetry configuration
is needed to build, authenticate, sign commits or use Copilot. The workspace
passes only the host's `OTEL_EXPORTER_OTLP_ENDPOINT` and optional
`OTEL_EXPORTER_OTLP_HEADERS` into Dev Container sessions (`remoteEnv`, not the
image), and selects `http/protobuf` and `NO_CONTENT`. It does not set
`COPILOT_OTEL_ENABLED`, a file exporter, or any capture-enabling settings.

Under the owner's explicit privacy decision (#2 D21), only the SDK's automatically
exported `enduser.pseudo.id` is permitted as account-identity metadata. It can
correlate sessions and is **not assumed anonymous**. Keep identity and content
capture disabled: names, raw user IDs, OS/host identity and every other
`enduser.*` field remain prohibited. The smoke gate allows that exact key only,
not the namespace.

An externally managed OTLP/HTTP collector must be reachable from
the container. Use its base OTLP URL, not a gRPC endpoint or an Application
Insights connection string. Copilot appends `/v1/traces` and `/v1/metrics`.
No host alias is assumed: choose the address/DNS and network appropriate for
your environment. Prefer HTTPS for a non-loopback receiver. The collector,
not this workspace, owns any forwarding to Application Insights; this setup
does not provision Azure resources, substitute connection strings, add a
sidecar/bridge, or instrument a Pocketpull application.

In the **host shell** used to start the Dev Container CLI (or launch VS Code),
supply an owner-approved endpoint and optional headers:

```sh
# Bash: read without putting values in command history.
read -r -p 'OTLP base URL: ' OTEL_EXPORTER_OTLP_ENDPOINT
export OTEL_EXPORTER_OTLP_ENDPOINT
read -r -s -p 'OTLP headers (empty if none): ' OTEL_EXPORTER_OTLP_HEADERS
printf '\n'
export OTEL_EXPORTER_OTLP_HEADERS
devcontainer up --workspace-folder .
devcontainer exec --workspace-folder . copilot
unset OTEL_EXPORTER_OTLP_ENDPOINT OTEL_EXPORTER_OTLP_HEADERS
```

Headers use comma-separated `name=value` pairs (for example
`x-development-source=pocketpull`); obtain authentication values through your
approved secret workflow. Do not put credentials in command arguments, checked-in
files or build arguments. `.env` is ignored as a safeguard, but nothing here
automatically loads it. Export explicitly; do not assume creating that file
configures the workspace. Reopen/reconnect VS Code after host environment
changes. Unset the endpoint/headers and start a fresh session to disable this
optional path.

The pinned CLI's `copilot help environment`, `copilot help config` and
`copilot help monitoring` document these controls. A nonempty endpoint enables
OTel automatically; `COPILOT_OTEL_ENABLED=true` or a file-export path can also
enable it independently. `OTEL_EXPORTER_OTLP_PROTOCOL=http/protobuf` selects
HTTP protobuf, never gRPC. `OTEL_INSTRUMENTATION_GENAI_CAPTURE_CONTENT=NO_CONTENT`
disables content by default. Explicit `telemetry.captureContent` and individual
`telemetry.capture.prompts`, `responses`, `toolArguments`, `toolOutput` and
`policyDetail` settings can override the environment choice.
`telemetry.capture.identity` separately controls account/OS/machine names and
defaults off; it did **not** suppress the observed `enduser.pseudo.id`.
Do not enable these captures. External user/organization settings, per-signal
exporter overrides and organization-managed policy are outside repository
control: review them before exporting. This configuration is not a guarantee
that an existing customized profile is metadata-only.

#### Real telemetry smoke check

After an interactive `gh auth login` **inside the container** with a development
account that has Copilot access, run from the host:

```sh
devcontainer exec --workspace-folder . just dev-telemetry-smoke
```

The recipe itself runs **inside the container**, requires no external collector,
and deliberately replaces any external endpoint/headers for its child process.
It uses Python's standard library, a dynamically assigned loopback port, fresh
scratch working directories, separate temporary `COPILOT_HOME` profiles, disabled
hooks/extensions/custom instructions/MCPs, and no available model tools.
It never runs Copilot in the repository or reads the host's Copilot profile.
It obtains an existing `gh` development OAuth token in memory (or uses an already
supplied `GH_TOKEN`, `GITHUB_TOKEN` or `COPILOT_GITHUB_TOKEN`); no credentials are
written into the scratch profile by the check. A saved `copilot login` alone is
not imported into these isolated profiles.

Two actual Copilot prompts, each bounded to 150 seconds, use only randomly
generated synthetic prompt/response markers. They consume normal Copilot
inference/credits under your account's policy. One proves the unconfigured path
is usable; the other targets the loopback receiver with a synthetic header.
The check decodes actual protobuf resource/scope/span and metric data-point
envelopes, requires both signals and the header, rejects content sentinels and
prohibited content/identity attribute keys (allowing only `enduser.pseudo.id`),
and stops the receiver/CLI and removes scratch
state in `finally` blocks. Authentication, emission or privacy failures are
nonzero failures, never skips.
This is loopback evidence only, not live Azure delivery or a general audit of
all tool/content paths. Collector implementation and collector usage
documentation remain in #1.
