# Telemetry

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

### Real telemetry smoke check

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
