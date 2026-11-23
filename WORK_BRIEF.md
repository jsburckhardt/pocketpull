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
- The owner authorised resuming #1 with factory-slice followed by factory-deliver as Lead on 2026-11-23. The first slice must collect one recording end-to-end into the raw folder against an HTTP fixture. This supersedes the earlier collector delivery gate; implementation and necessary application dependencies within this agreement are authorised.
- Deliver #1 on one integration branch and one independently accepted PR. The owner retains merge and release authority. Development-environment scope remains exclusively in the merged #2 delivery.
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
| D22 | Resume collector #1; use factory-slice then factory-deliver as Lead, starting with one recording end-to-end into raw storage against an HTTP fixture. | Explicit owner go on 2026-11-23 supersedes D18 for #1; #2 remains separately delivered and merged. | owner |

## Open questions and spikes
None. No spike is approved or required for the conservative agreed collection strategy. A live API incompatibility or another consequential collector contract change must reopen alignment.

## Tasks
Tasks are sequential because they extend the same CLI/collection contracts and shared files. Partial AC mappings below identify incremental evidence, not acceptance of the complete AC before its final owning task integrates. The Lead owns this brief and review records; implementers must not edit either brief or development-environment files.

### T1 One recording reaches raw storage through a native CLI HTTP-fixture run
- ACs: AC1, AC3, AC4, AC5, AC6, AC9 (thin-path evidence; remaining coverage in T2-T4)
- Depends on: None
- Contracts: Native run-once CLI; --help/--version; required --raw-dir and environment-only credential for this slice; documented Bearer list/detail routes; page traversal and ID deduplication; complete detail data preserving unknown fields; stable safe ID paths; atomic changed-only snapshots; no audio/pruning; baseline redacted slog stderr and summary. HTTP endpoint injection is internal/test-only, not an added public flag or environment contract.
- Write scope: go.mod, go.sum, cmd/**, internal/**, justfile, AGENTS.md, README.md, .gitignore
- Shared files: cmd/**, internal/**, go.mod, go.sum, justfile, AGENTS.md, README.md owned by T1 until integration, then transferred serially to its dependant
- Resources: Loopback HTTP fixture on an OS-assigned port; temporary raw/test/build directories only
- Checks: just collector-check; just collector-smoke; just dev-check
- Status: integrated
- Worker: factory/collector-t1; base 08773f6427fc2a5ace2d5b70888ed23f74193a96; accepted head 0cbe3da4421708372e41081a139cf85bcecb8aad; integrated as 8c30365da87c333b68aea753fe475533cbaab719; PID 887 exited; log .trees/collector-t1.log

### T2 Config-driven and cron-safe collection with strict secret handling
- ACs: AC2, AC6, AC9; AC1, AC4, AC5 regression
- Depends on: T1
- Contracts: Complete invocation/configuration/authentication contracts; single strict YAML file, exact discovery and CLI precedence, relative paths, no .env loading, owner-only secret files and conflicts; all collection flags; logging formats/severities and redaction even on malformed configuration; executable usage/cron/configuration examples. Preserve T1's fixture-to-raw path.
- Write scope: go.mod, go.sum, cmd/**, internal/**, justfile, AGENTS.md, README.md, examples/**, .gitignore
- Shared files: cmd/**, internal/**, go.mod, go.sum, justfile, AGENTS.md, README.md owned by T2 until integration, then transferred serially to its dependant
- Resources: Loopback HTTP fixture on an OS-assigned port; temporary config/secret/raw directories only
- Checks: just collector-check; just collector-smoke; just dev-check
- Status: integrated
- Worker: factory/collector-t2; base 7649fb6448124dd30171658ee6fe96c56e3ac7a0; accepted head 56c9f6890e57b1d7ebe6783a24d4e2e4c29cb507; integrated as af443b07838778b8c3c0eeee791de32d5540e025; PID 21922 exited; log .trees/collector-t2.log

### T3 Thousand-record repeated collections refresh safely without overlap
- ACs: AC3, AC4, AC5, AC9; AC2, AC6 regression
- Depends on: T2
- Contracts: Full 1,000-record multipage traversal and deduplication; new records immediate, existing/pending records refreshed by successful fetch time with injected clock; zero refresh; missing/unusable state or output refetch; canonical-raw-path state namespace outside raw; nonwaiting process-recoverable overlap lock; changed-only snapshots, missing/404 records never prune; document artifact layout and refresh latency.
- Write scope: cmd/**, internal/**, justfile, AGENTS.md, README.md, examples/**
- Shared files: cmd/**, internal/**, justfile, AGENTS.md, README.md owned by T3 until integration, then transferred serially to its dependant
- Resources: Loopback HTTP fixtures on OS-assigned ports; independent temporary state/raw directories and subprocesses; no production archive
- Checks: just collector-check; just collector-smoke; just dev-check
- Status: failed
- Worker: factory/collector-t3; base d2124a623fe49c1fe16a670ef407ef1d9e28dd9a; rejected head c00750a4ab07ca733372e6ffea0f3f79acc6a3d0; PID 27136 exited; correction pending; log .trees/collector-t3.log

### T4 Interrupted and failing runs retain progress and report a complete operational outcome
- ACs: AC1, AC2, AC3, AC4, AC5, AC6, AC9 (complete final coverage)
- Depends on: T3
- Contracts: Bounded transient network/429/server retries and timeouts; prompt auth stop; individual failures preserve partial progress and exit nonzero; malformed/failed-write cases preserve snapshots and never advance freshness; interruption/restart safety; pending is not an error; complete run/event/count logging, redacted at every severity, stdout free; final documentation covers all collector contracts and native amd64/arm64 builds.
- Write scope: cmd/**, internal/**, justfile, AGENTS.md, README.md, examples/**
- Shared files: cmd/**, internal/**, justfile, AGENTS.md, README.md owned by T4; Lead owns final evidence/publication in this brief and PR
- Resources: Fault-injected loopback HTTP fixtures on OS-assigned ports; temporary state/raw/build directories and identified subprocesses only
- Checks: just collector-check; just collector-smoke; just dev-check
- Status: blocked
- Worker: None

## Status
delivering

Collector AC1-AC6/AC9 and delivery are authorised. Development AC7/AC8 and development AC9 remain exclusively in #2, merged by #3 at 548d269e38b3f332c8d4bbf2b4fc3870f8362630. Lead session: c5fac65e-0ea9-4288-aa93-7440ab7136d6. Integration branch: delivery/1-collector. Start with T1; unlock each dependant only after independent task acceptance, serial integration and combined checks. One final accepted PR targets main; the owner merges/releases.

## Acceptance evidence
### T1 independent task acceptance
- Verdict: pass for T1-owned portions only; complete issue acceptance remains pending T2-T4.
- Reviewer: fresh-context accept-collector-t1, factory-accept/factory-design; no repairs or builder self-certification.
- Base: 08773f6427fc2a5ace2d5b70888ed23f74193a96. Reviewed head: 0cbe3da4421708372e41081a139cf85bcecb8aad. Integration: 8c30365da87c333b68aea753fe475533cbaab719.
- AC1: both CGO-free target builds, native fixture subprocess and application help/version pass. AC3: multipage deduplication/immediate details pass; cache/1,000-record evidence deferred. AC4: lossless complete data, safe folders, changed-only atomic writes and failure preservation pass. AC5: partial progress/auth-stop/pending/cancellation cases pass; locks/retries deferred. AC6: redacted JSON stderr, empty stdout and summary pass; full formats/severities deferred. AC9: current behavior documented with explicit limitations; complete configuration/cache/retries deferred.
- Reviewer checks: just collector-check; just collector-smoke; just dev-check; go test -race -count=1 ./...; credential-free native help/version. All exit 0 in the non-root devcontainer. Findings: none.
- Lead combined checks after exact serial cherry-pick: just collector-check; just collector-smoke; just dev-check. All exit 0. Signed worker commit verified; no surviving processes in PGID 887. T2 unlocked.

### T2 independent task acceptance
- Verdict: pass for complete AC2 and T2-owned AC6/AC9 plus accepted T1 regressions; state/cache/locking/retries remain deferred to T3/T4.
- Reviewer: fresh-context accept-collector-t2, factory-accept/factory-design; no repairs or record edits.
- Base: 7649fb6448124dd30171658ee6fe96c56e3ac7a0. Reviewed head: 56c9f6890e57b1d7ebe6783a24d4e2e4c29cb507. Integration: af443b07838778b8c3c0eeee791de32d5540e025.
- AC2: single strict YAML discovery/precedence/path/type/default validation, help/version bypass, nonempty unique credential source and owner-only regular secret files pass. AC6: JSON/text/severity options and malicious-input/transport/content redaction pass. AC9: executable config/secret examples and accurate cron/limits documentation pass. AC1/AC4/AC5: native builds/smokes and snapshot/partial-progress regressions pass.
- Reviewer checks: just collector-check; just collector-smoke; just dev-check; go test -race -count=1 ./...; git diff --check. All exit 0 in non-root devcontainer; exact clean head verified. Findings: none.
- Lead combined checks after exact serial cherry-pick: just collector-check; just collector-smoke; just dev-check. All exit 0. Signed worker head verified; no surviving processes in PGID 21922. T3 unlocked.

### T3 initial independent task rejection
- Verdict: fail. Reviewer: fresh-context accept-collector-t3, factory-accept/factory-design. Base d2124a623fe49c1fe16a670ef407ef1d9e28dd9a; reviewed head c00750a4ab07ca733372e6ffea0f3f79acc6a3d0. Not integrated; T4 remains blocked.
- F1 (major, implementation): internal/collector/snapshot.go:31-36 and state.go:161,191 reuse the 32 MiB HTTP-response limit for indented raw snapshots. A valid 10,000,064-byte response writes a 35,000,057-byte snapshot; subsequent fresh and zero-refresh runs fail with prior snapshot unreadable. Raw readers must accommodate every accepted writer output, preserving semantic unchanged detection and prior-file safety.
- AC3/AC4 fail due to F1. T3-owned AC5/AC9/AC2/AC6 pass: canonical namespaces, safe state, native overlap/alias/different-state/SIGKILL recovery and documentation/logging verified.
- Reviewer checks: just collector-check; just collector-smoke; just dev-check; git diff --check all exit 0; independent native loopback compact-response/expanded-output fixture reproduces F1. Exact worktree remained clean; scratch removed. Narrow correction requires regression, current checks and fresh acceptance of corrected exact head.
