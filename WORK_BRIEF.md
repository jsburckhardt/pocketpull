# #1 Pocketpull v1 collector

## Goal
Provide a single Go binary that performs one config-driven collection of Pocket transcripts and wiki-useful API content into a configurable raw folder, suitable for invocation by cron.

Tracking issue: [#1](https://github.com/jsburckhardt/pocketpull/issues/1). This file is the canonical collector agreement. Development-environment ACs and contracts are owned exclusively by [#2](https://github.com/jsburckhardt/pocketpull/issues/2) in [WORK_BRIEF_DEV_ENV.md](WORK_BRIEF_DEV_ENV.md).

## Acceptance criteria
- AC1: Linux amd64 and arm64 binaries build without requiring an installed application runtime; a native smoke run collects once and exits, with no scheduler. | evidence: both target builds and a native CLI smoke test against an HTTP fixture.
- AC2: Configuration discovery, single-file selection, CLI precedence, relative paths, strict YAML, required raw directory, secret sources, and redaction follow the contracts below; no automatic `.env` loading occurs. | evidence: focused CLI/configuration tests, including malformed/unknown values, missing explicit files, secret-source conflicts, and unsafe secret-file permissions.
- AC3: A 1,000-record multipage fixture is fully traversed, duplicate IDs are fetched once, new records are fetched immediately, and existing records use the 24-hour detail-refresh policy without relying on `updated_at`. Missing output or missing/unusable cache causes safe refetching; successfully fetched pending records use the same refresh interval. | evidence: deterministic HTTP/filesystem integration tests with an injected clock and assertions on requests and output.
- AC4: Stable ID folders contain complete recording-detail data objects, preserving unknown fields and requesting no audio. Changed snapshots replace atomically, unchanged content is not rewritten, failures preserve prior files, and remote absence never prunes local records. | evidence: filesystem/API integration tests covering unchanged/changed payloads, interrupted or failed writes, invalid responses, and absent/404 recordings.
- AC5: Transient failures receive bounded retries, authentication failures stop promptly, individual failures retain partial progress and cause a nonzero exit, overlapping runs are rejected, and interruption/restart does not corrupt committed snapshots or freshness. Pending processing alone is not an error. | evidence: fault-injected HTTP, subprocess, locking, and restart tests.
- AC6: Application logs contain structured, redacted operational events and summary counts, support text and severity selection, and leave stdout free during collection. | evidence: captured-log CLI tests, including debug-level secret/content redaction and partial-failure summaries.
- AC9: Collector usage documentation/examples cover cron, configuration, secrets, refresh latency, artifact layout, and logs. | evidence: documentation review against the collector contracts and executable examples where applicable. Development-authentication/signing/telemetry documentation belongs to #2.

## Contracts
### Invocation and configuration
- `pocketpull` performs exactly one collection. Cron or another external scheduler owns timing; there is no resident process, internal interval, cron parser, or missed-run scheduler.
- `--help` and `--version` do not collect. Collection flags are `--config`, `--raw-dir`, `--api-key-file`, `--state-dir`, `--refresh-after`, `--log-format`, and `--log-level`.
- Select exactly one YAML file: explicit `--config`, otherwise `./pocketpull.yaml`, otherwise `$XDG_CONFIG_HOME/pocketpull/config.yaml`, falling back to `~/.config/pocketpull/config.yaml` when the XDG base is unset. Do not merge discovered files. An explicitly requested missing/unreadable file is an error. Without a discovered file, defaults and explicit flags may supply configuration.
- Explicit CLI values override the selected file. Resolve relative file-configured paths against the config file's directory and CLI paths against the working directory. Reject unknown keys, invalid values, and unusable configuration before collection.
- YAML keys are `raw_dir`, `api_key_file`, `state_dir`, `refresh_after`, `log_format`, and `log_level`. `raw_dir` is required; there is no implicit output directory. Default `refresh_after` is `24h`; `0` forces full detail refresh. Duration parsing details are delegated but must be documented and reject negative/invalid durations.
- Target one Pocket account, an always-on Linux host, and persistent local storage. Build Linux amd64 and arm64 application binaries without requiring Python, Node, or another installed application runtime.

### Authentication
- Read exactly one nonempty credential source: `POCKETPULL_API_KEY` or the file selected by `api_key_file`/`--api-key-file`. Conflicting sources fail rather than silently choosing one.
- Secret files must be owned by the current user and unreadable by group/others. There is no literal `api_key` YAML key or `--api-key` flag.
- No automatic `.env` discovery/loading. Development tooling or the shell may explicitly export the environment variable; keep `.env` gitignored. Recommend the secret-file option for cron.
- Never log keys, authorization headers, transcript bodies, raw API responses, or credential-bearing URLs, including at debug level. Never put secrets in tracked files, image layers, or build arguments.

### Collection and wiki-facing files
- Use the public Pocket API with Bearer authentication. List all account pages on each invocation, without date/tag filtering, and deduplicate recording IDs.
- Fetch details immediately for new recordings. Refresh existing records when their last successful detail fetch is at least `refresh_after` old. This policy also applies to valid records whose transcript is still processing. Do not infer freshness or completeness from undocumented `updated_at` semantics.
- A later transcript/edit can appear up to the refresh interval plus the cron interval and run time later. No guarantee is made for content that disappears upstream before a successful fetch.
- Write `<raw-dir>/<recording-id>/recording.json`, containing the API's complete recording-detail `data` object rather than the transport envelope. Preserve unknown fields, transcript segments, speaker/timing information, metadata/tags, summarizations, and other content already included in that response.
- Folder identity is the recording ID, not its mutable title. Do not download audio or call additional content services. No derived Markdown/plaintext is required for the wiki consumer.
- Keep the latest successfully fetched snapshot, not an edit history. Atomically replace only changed content; unchanged content is not rewritten. Failed requests, malformed responses, and failed writes must not destroy the previous committed file. Valid pending records are not malformed responses.
- Never prune local recordings when Pocket stops listing them or returns 404. Treat a detail-request failure as an incomplete run, retaining the local snapshot where present.

### State, concurrency, and failures
- Private state lives outside the wiki-facing raw folder. The default state base is `$XDG_STATE_HOME/pocketpull`, falling back to `~/.local/state/pocketpull`. `state_dir` overrides the base. Namespace state by the canonical raw-folder path.
- State records successful detail-fetch times; failed fetches never advance freshness. Missing/unusable state or missing raw output causes safe refetching rather than skipping. State is expendable and not the archive itself.
- Allow one collection per canonical raw directory at a time; an overlapping invocation exits nonzero rather than waiting. Recover from process interruption without leaving a permanent stale lock.
- Retry transient network failures, HTTP 429, and server failures with bounded backoff. Exact pacing, retry limits, and HTTP timeouts are delegated; no numeric upstream quota is assumed.
- Fail authentication/configuration errors promptly. Continue past individual recording failures, preserve successful work, and return nonzero for an incomplete run. Exit zero means complete traversal and successful required work, including valid cache skips; it does not mean every transcript has finished upstream processing.

### Operational logs
- Use Go `slog`, JSON on stderr by default. `log_format`/`--log-format` accepts `json` or `text`; `log_level`/`--log-level` accepts `info`, `debug`, `warn`, or `error`, defaulting to `info`.
- Include run ID, event, recording ID where relevant, elapsed time, retries, and final discovered/fetched/unchanged/cached/pending/failed counts. Document count meanings without implying cached records were fetched during this run.
- Collection leaves stdout free. No application log-file management, rotation service, or telemetry exporter is included.

## Constraints
- Collector delivery remains alignment-only: no collector implementation or application dependency installation is authorised. Separate development-environment delivery is authorised only under #2.
- The owner authorised committing AGENTS.md and both root briefs to main and pushing them. This replaces the earlier local-only record restriction, not the collector delivery gate.
- Use Go and prefer the simplest implementation satisfying these contracts. Cobra is a suitable delegated CLI choice; avoid Viper's implicit discovery/merging in favour of explicit strict YAML parsing. Exact supported pinned tool versions, package structure, minimal dependencies, request pacing/retries/timeouts, and detailed event formatting are delegated within the agreed boundaries.
- Consequential changes to data completeness, credential handling, public contracts, dependency/cost/privacy boundaries, or scope require renewed alignment. No private inputs are sent to external research services.
- Decisive delivery evidence may use fixtures derived from public documentation. No authenticated Pocket experiment is required or authorised here. An incompatible live API discovery reopens alignment instead of silently changing behaviour.
- Public API evidence: [overview/authentication](https://docs.heypocketai.com/docs/api), [list recordings](https://docs.heypocketai.com/docs/api/recordings/list-recordings), and [recording details](https://docs.heypocketai.com/docs/api/recordings/get-recording-details). The documented base URL is `https://public.heypocketai.com/api/v1`; recording routes are `/public/recordings` and `/public/recordings/{id}`. Listing documents page-number pagination, a maximum page size of 100, and `has_more`; details default to including transcript and summarizations. No documented update cursor, reliable content-update watermark, or numeric request quota was established.

## Out of scope
- Internal scheduling, daemon/service operation, webhooks, uploads, remote edits/deletions, local pruning, and edit-history retention.
- Audio downloads, transcription, summarisation, wiki/entity generation, embeddings, generated Markdown/plaintext, and extra content integrations. Preserve summaries already returned by Pocket; do not generate them.
- Multiple accounts/users, cloud/object storage backends, Windows/macOS release guarantees, date/tag filters, and update-watermark optimisations.
- Native `.env` loading, credentials on the CLI or in literal YAML, hardened untrusted-agent sandboxing, host Docker integration, Azure infrastructure, an in-repository telemetry bridge, and collector-application Application Insights instrumentation.
- Development tooling and its authentication/telemetry documentation are delivered separately under #2; release publication and live Pocket/Azure credential experiments are not authorised for #1.

## Decisions
| # | Decision | Rationale | Decided by |
|---|---|---|---|
| D1 | One-way durable latest-state local archive; never mutate Pocket or prune on remote absence. | Protect locally collected data while avoiding sync/delete semantics. | owner |
| D2 | Retain raw transcript, metadata, and API-included wiki-useful content; exclude audio and processing. | The downstream wiki agent needs source content, not audio or collector-generated knowledge. | owner |
| D3 | Run once; cron owns all scheduling. | The owner rejected a resident scheduler as unnecessary. | owner |
| D4 | Linux-first Go binary, amd64/arm64, one account and local persistent storage. | Straightforward native distribution and a bounded first deployment; Rust is viable but not required. | owner |
| D5 | One selected YAML file with explicit CLI overrides and file-relative paths. | Predictable configuration without hidden multi-file merges. | owner |
| D6 | API key from environment or an owner-only secret file; no automatic `.env` loading. | Support development and cron without process-list/config credential leakage. | owner |
| D7 | Full account listing every run; 24-hour detail cache, configurable to zero. | Thousands of small recordings make every-run detail refresh costly; explicit bounded freshness avoids trusting undocumented update semantics. This supersedes the earlier full-detail-refresh proposal. | owner |
| D8 | Complete detail-data JSON under stable recording IDs; atomic latest snapshot, no revision history. | Keep the wiki input raw and resilient to title changes and interrupted writes. | owner |
| D9 | Expendable private state outside raw data, namespaced by canonical output path. | Isolate operational bookkeeping from wiki input and recover safely by refetching. | owner |
| D10 | Reject overlap, retry transient failures within bounds, retain partial progress, and signal incomplete runs nonzero. | Make cron execution recoverable and observable without corrupting collected data. | owner |
| D11 | JSON stderr operational logs with text/severity options and content/credential redaction. | Support machine operations and local diagnosis independently of development telemetry. | owner |
| D15 | Choose exact versions, package layout, minimal dependencies, pacing/timeouts/retry bounds, and event formatting within these contracts. | Delegate mechanical choices while preserving consequential owner decisions. | owner |
| D16 | Prefer Cobra and explicit strict YAML over Viper's implicit merging/discovery. | Match the CLI/config contract without unnecessary configuration machinery. | delegated |
| D17 | Publish AGENTS.md, WORK_BRIEF.md for #1, and WORK_BRIEF_DEV_ENV.md for #2 to main with a signed commit and push. | Explicit owner instruction replaces the earlier local-only/uncommitted restriction; one brief per issue. | owner |
| D18 | Keep collector #1 alignment-only; grant delivery go for #2 only. | The owner explicitly limited the superseding delivery authority to the development environment. | owner |
| D19 | Move AC7, AC8, development contracts, and the development-auth/signing/telemetry portion of AC9 to #2. | Separate a useful development delivery from unapproved collector implementation; retain original AC IDs for traceability. | owner |

## Open questions and spikes
None. No spike is approved or required for the conservative agreed collection strategy. A live API incompatibility or another consequential collector contract change must reopen alignment.

## Tasks
None. Owned by factory-slice after explicit delivery authorisation; this alignment does not create implementation tasks.

## Status
aligning

Shared understanding and collector AC1-AC6/AC9 are approved. Collector delivery is NOT authorised. The owner authorised publishing this agreement to main; only #2 has delivery go.
