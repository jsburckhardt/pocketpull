# pocketpull

Scheduled Pocket transcript collector.

Delivery skills in `.github/skills/` are vendored from jsburckhardt/toolio `soft-factory-v2` at `3d07eb5`.

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
