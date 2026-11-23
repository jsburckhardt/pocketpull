# #2 Dev environment

## Goal
Provide a consistent, project-scoped development and agent devcontainer for Pocketpull, including Go, Herdr, SSH commit signing, and optional metadata-only Copilot observability through an external OTLP collector.

Tracking issue: [#2](https://github.com/jsburckhardt/pocketpull/issues/2). This file is the canonical development-environment agreement. Split from [#1](https://github.com/jsburckhardt/pocketpull/issues/1); [WORK_BRIEF.md](WORK_BRIEF.md) exclusively owns collector scope. Original AC IDs are retained for traceability.

## Acceptance criteria
- AC7: The devcontainer builds and runs as non-root with Go, Git/GitHub CLI, Copilot CLI, pinned Herdr, just, and zsh; SSH commit signing works through the approved read-only SSH-directory mount. It has no host Docker socket or production-data mount, and does not auto-start Herdr. | evidence: devcontainer build/tool smoke checks, mount/user inspection, and signing verification with a disposable test key rather than production credentials.
- AC8: Supplied host OTLP endpoint/optional headers let Copilot emit metadata-only OTLP/HTTP protobuf to a test collector; without telemetry configuration the workspace remains usable. The SDK's automatic `enduser.pseudo.id` is the only permitted account-identity field; other identity/content capture remains prohibited. | evidence: a clean-profile Copilot/test-collector smoke check of traces/metrics, protocol, the narrow field exception, and absence of other identity/content capture; unconfigured workspace smoke check. External Azure forwarding is not claimed by this check.
- AC9: Development usage documentation/examples cover development authentication, SSH commit signing, and external telemetry setup. | evidence: documentation review against these contracts and executable examples where applicable. Collector usage documentation remains in #1.

## Contracts
### Development workspace
- Use an Ubuntu-based Dockerfile, non-root `vscode` user, Dev Container features and lockfile, and an idempotent post-create pattern informed by soft-factory. Include a pinned Go toolchain, Git/GitHub CLI, Copilot CLI, Herdr CLI, just, zsh, and shell-history support.
- Pin reviewed tool versions. Install Herdr from a versioned upstream Linux binary and verify a pinned SHA-256 digest; support Linux amd64 and arm64 development architectures. Herdr may start its local session server when invoked, but the container must not auto-start it.
- Bind-mount host `~/.ssh` to `/home/vscode/.ssh` read-only as an explicit owner-approved exception for SSH commit signing. Read-only prevents modification, not access/use by container processes. Do not copy SSH keys into the image; document the required Git SSH signing configuration.
- No privileged mode, host Docker socket, whole-home mount, Azure CLI requirement, production archive mount, or production API-key mount. This is a project development boundary, not a hardened sandbox for untrusted agents.
- Persist only deliberately selected CLI authentication/tool state in named volumes. Authenticate interactively, not during image build.
- Keep production tooling out of scope: this issue does not add collector implementation, an application module, placeholder application checks, or an internal scheduler.

### Copilot observability
- Optional, development-only Copilot export uses host-supplied `OTEL_EXPORTER_OTLP_ENDPOINT` and optional `OTEL_EXPORTER_OTLP_HEADERS`, OTLP/HTTP with `http/protobuf`, and metadata-only capture.
- Do not hard-code `host.docker.internal` or enable prompt/response, tool-argument/result, policy-detail, or identity capture in repository-owned configuration. Verification uses a clean profile; external user/organization telemetry policy is outside repository control.
- Owner permits only the automatically exported `enduser.pseudo.id` as operational metadata. It may correlate sessions and is not assumed anonymous. Keep identity capture disabled; names, raw user IDs, OS/host identity, and other `enduser.*` fields remain prohibited. The gate must allow this exact key only, not the whole namespace.
- An externally managed OTLP collector owns forwarding to Application Insights. An Application Insights connection string is not substituted for an OTLP URL. No collector sidecar, Azure provisioning, or connection-string bridge is included. Missing telemetry configuration must not block development.
- Supply connection details through explicit host environment variables, never checked-in files or image layers. Never capture credentials, private prompts, source content, or production data in test telemetry.

### Delivery shape
- One Lead session owns #2, one integration branch targets main, and one final PR closes #2. At most two implementers may run, and the two planned slices are sequential because configuration and documentation have shared ownership.
- Use scoped worktree workers via factory-deliver's worker.sh, task-level independent acceptance, serial integration, combined checks, and independent acceptance of the exact final SHA. No merge or release authority is delegated.
- Update AGENTS.md Commands only after verifying new commands. Keep checks focused on real development configuration and integration behaviour, not nonexistent collector functionality.

## Constraints
- Owner explicitly granted delivery go for #2 only, superseding D18 only for this issue. #1 remains alignment-only.
- Owner explicitly authorised committing AGENTS.md and both briefs to main and pushing them, replacing D17's local-only restriction. Preserve SSH commit signing and configured identity; include required commit trailers.
- Exact supported pinned tool versions, minimal development dependencies, and internal implementation details are delegated within the agreed contracts. Consequential behaviour, dependency/cost/privacy boundary, or scope changes return to alignment.
- Local environment at handoff: Docker daemon 29.10.0, devcontainer CLI 0.86.1, and Copilot CLI 1.0.115-7 are available; the Lead is not already inside a devcontainer. Build and verify the requested container rather than claiming host checks prove container behaviour. No existing application or CI checks exist.
- Git identity/signing are configured locally and an SSH signer is available. Production credentials must not be used as disposable signing-test material or committed to the repository.
- Copilot interactive authentication may be required for a real telemetry smoke. Missing access must be reported as blocked, not replaced with synthetic evidence claiming actual CLI emission. Do not claim live Azure delivery from a local collector check.
- Reference: [soft-factory at 19568bcea34b62f313e0c7ac0fdbb42926744d96](https://github.com/jsburckhardt/soft-factory/tree/19568bcea34b62f313e0c7ac0fdbb42926744d96/.devcontainer). Adopt its relevant Dockerfile/features/lockfile/post-create, SSH-mount, and external-OTLP patterns, not its Docker socket or unrelated Azure tooling. Herdr was not present in that tree.
- Herdr feasibility evidence: [v0.9.5 metadata](https://api.github.com/repos/herdrdev/herdr/releases/tags/v0.9.5) publishes `herdr-linux-x86_64` and `herdr-linux-aarch64` with SHA-256 digests; [versioned installation documentation](https://github.com/herdrdev/herdr/blob/v0.9.5/docs/next/website/src/content/docs/install.mdx) supports direct downloads. The exact delivery version is delegated, not fixed by this feasibility observation.
- Verify Copilot environment variables, capture controls, and protocol against its pinned version. [Azure guidance](https://learn.microsoft.com/en-us/azure/azure-monitor/containers/opentelemetry-protocol-ingestion) distinguishes configured OTLP ingestion from an Application Insights connection string.

## Out of scope
- Collector implementation and collector usage documentation, including cron, application config/secrets, refresh/cache behaviour, raw artifacts, and application logs; those remain in #1 without delivery go.
- Host Docker integration, privileged mode, broad home mounts, production credentials/archive mounts, and hardened untrusted-agent sandboxing.
- An in-repository telemetry bridge/sidecar, Azure provisioning, direct connection-string integration, content/identity capture beyond the exact permitted `enduser.pseudo.id` field, live Azure acceptance, or collector-application Application Insights instrumentation.
- Automatic Herdr startup, release publication, PR merging, and changes to the repository default branch or deletion of master.

## Decisions
| # | Decision | Rationale | Decided by |
|---|---|---|---|
| D12 | Reuse relevant soft-factory devcontainer patterns, add verified pinned Herdr, and exclude unrelated tooling. | Consistent development and agent tooling without changing the collector. | owner |
| D13 | Read-only host SSH-directory mount for signing despite readable-key exposure. | Required owner signing workflow; no whole-home mount. | owner |
| D14 | Optional metadata-only Copilot OTLP/HTTP protobuf to an external collector. | Integrate development observability without infrastructure or an undocumented connection-string substitution. | owner |
| D15 | Delegate exact pinned versions and minimal internal tooling choices within these contracts. | Avoid unnecessary owner decisions while preserving consequential boundaries. | owner |
| D17 | Publish corrected AGENTS.md and one brief per issue to main with a signed commit and push. | Explicitly replaces the local-only/uncommitted restriction. | owner |
| D18 | Authorise #2 delivery only; keep collector #1 alignment-only. | Explicit owner go supersedes the previous no-go only for this issue. | owner |
| D19 | Own AC7, AC8, and development-auth/signing/telemetry AC9 separately from collector #1. | A useful independent delivery with its own acceptance and PR. | owner |
| D20 | Sequential workspace/signing and telemetry slices with fresh independent reviews. | Shared configuration/docs require one owner at a time; parallelism is not a goal. | delegated |
| D21 | Permit only the SDK's automatic enduser.pseudo.id; keep every other identity/content gate unchanged and continue #2. | Independent review reproduced this account-correlating metadata despite disabled capture; the owner explicitly approved the exact exception rather than a broad relaxation or unverified CLI fix. | owner |

## Open questions and spikes
None. Owner resolved the pseudonymous-field boundary in D21. No spike, CLI version change, or broader capture is authorised.

## Tasks
### T1 A usable development workspace with signed commits
- ACs: AC7, AC9
- Depends on: None
- Contracts: Development workspace, relevant delivery shape, and authentication/signing documentation; telemetry must remain unconfigured and development must work without it.
- Write scope: .devcontainer/**, justfile, .gitignore, README.md, AGENTS.md; do not modify briefs, factory skills, collector files, or repository Git configuration.
- Shared files: .devcontainer/**, justfile, .gitignore, README.md, AGENTS.md are owned by T1 until integration; T2 may change them only after T1 integrates.
- Resources: One task-specific devcontainer and disposable local signing-test directory; no production keys copied or services left running.
- Checks: just dev-check; devcontainer build --workspace-folder .; devcontainer up --workspace-folder .; devcontainer exec --workspace-folder . just dev-smoke. Document and run a disposable-key check that proves SSH signing through a read-only host-directory mount, plus non-root/tool/mount/no-auto-start checks. Planned recipes must exercise actual behaviour, not placeholders.
- Status: integrated
- Worker: branch factory/dev-env-t1; base 19d192e45ab8524530d1fb0b49a6b96b4c85a547; accepted head da30c5322a2b56b6931838bc063482531ecd8968; PID 3283528 exited with no surviving group processes; log .trees/dev-env-t1.log. Independent acceptance passed AC7 and T1-owned AC9 with no findings; integrated unchanged by signed merge.

### T2 Observable Copilot sessions without content capture
- ACs: AC8, AC9
- Depends on: T1
- Contracts: Copilot observability and external-collector documentation; preserve the accepted workspace and SSH-signing behaviour from T1.
- Write scope: .devcontainer/**, justfile, .gitignore, README.md, AGENTS.md; do not modify briefs, factory skills, collector files, or repository Git configuration.
- Shared files: T2 owns changes to .devcontainer/**, justfile, .gitignore, README.md, AGENTS.md after T1 integrates; no concurrent writer.
- Resources: One task-specific devcontainer and ephemeral loopback test collector on a dynamically assigned port; only synthetic prompts/test credentials, no private-content export, no detached services after checks.
- Checks: just dev-check; devcontainer build --workspace-folder .; devcontainer up --workspace-folder .; devcontainer exec --workspace-folder . just dev-smoke; devcontainer exec --workspace-folder . just dev-telemetry-smoke. Prove actual pinned Copilot trace/metric emission to the test collector with supplied headers and no content capture, and prove the unconfigured path remains usable. Escalate missing authentication or unsupported exporter behaviour instead of mocking actual CLI emission.
- Status: integrated
- Worker: correction branch factory/dev-env-t2-r1; base 6c956ca8782335c911fa43956d5150625e95ab62; accepted head 011992788a514be2cb33091fb8136450c04d175e; worker PID 3583019 exited with no surviving group processes; log .trees/dev-env-t2-r1.log. Lead exclusively repaired implementation F2 after independent failure at c63a313, without amending commits. Independent acceptance passed revised AC8, AC9 and preserved AC7 with no unresolved findings; integrated unchanged by signed merge.

## Status
delivering

Both tasks are independently accepted and integrated unchanged. T1 passed at da30c5322a2b56b6931838bc063482531ecd8968; T2 passed at 011992788a514be2cb33091fb8136450c04d175e after owner D21 resolved F1 and the Lead repaired F2 with persistent regressions. Final combined checks passed; independent final-SHA acceptance is pending before PR publication. Collector #1 remains alignment-only. Merge and release remain owner-controlled.

T1 evidence: builder and independent reviewer verified configuration checks, native amd64 image startup/build evidence, disposable-key signing/mount/runtime integration, and just formatting. Lead reran `just dev-check`, `just --fmt --check --unstable`, and `just dev-signing-smoke` after integration; all passed and owned test resources were removed. ARM64 artifact digests were verified; ARM64 runtime was not executed. Local Git signer-trust configuration is absent; the signed worker commit's integrity was separately checked without changing host trust.

Final combined evidence at 1015a8fc85f521ae69125b0d951e308f3974fbe9: six `just dev-check` regressions, just formatting, native pinned image build/startup, non-root/tools/mount/history checks, idempotent setup, read-only disposable SSH signing/verification, host endpoint/header forwarding, and actual clean-profile Copilot HTTP/protobuf emission all passed. The receiver counted two spans and twelve metric data points, rejected prohibited content/identity, permitted only enduser.pseudo.id, and observed successful unconfigured inference. Only synthetic input and transient development authentication were used; all owned containers, new volumes, receiver/CLI processes and disposable keys were removed. No ARM64 runtime or live Azure forwarding is certified. Subsequent record-only changes do not alter the tested implementation.
