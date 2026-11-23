<!-- soft-factory-v2:begin -->
## How we deliver
Skills: `.github/skills/`. Use them in this order:
1. `factory-align`: grill before building; one issue-linked work brief; wait for the owner's "go".
2. `factory-spike`: only when approved; bounded; written to `docs/spikes/`.
3. `factory-slice`: vertical tasks in the brief.
4. `factory-deliver`: one lead session; at most two implementers in `.trees/` via its `scripts/worker.sh`; serial integration; one PR per issue.
5. `factory-accept`: independent review per task and for the final SHA.
`factory-design` applies throughout: deep modules, small interfaces, integration tests first.

## Rules
- Questions are not authorisation to change things.
- The owner merges and releases unless delegated.
- Never commit secrets; read credentials from the environment or owner-approved secret files.
- When a task introduces or changes build/test/lint commands, update `## Commands` in the same task.

## Commands
- `just dev-check`: host/container configuration, feature-lock, shell-syntax and recipe checks.
- `devcontainer build --workspace-folder .`: build the pinned Ubuntu workspace.
- `devcontainer up --workspace-folder .`: start the non-root workspace (requires host `~/.ssh`).
- `devcontainer exec --workspace-folder . just dev-smoke`: exercise installed tools, user, mounts and shell history.
- `just dev-signing-smoke`: host-only disposable-key devcontainer integration; verifies SSH-signed commits through the read-only SSH mount and cleans up its own resources.
- `devcontainer exec --workspace-folder . just dev-telemetry-smoke`: authenticated, container-only clean-profile Copilot/loopback OTLP protobuf check; permits only the automatic `enduser.pseudo.id` identity field and rejects other identity/content capture (see README).

See README.md for development authentication/signing, optional external telemetry, scope boundaries and prerequisites. No collector/application checks exist. Telemetry is off without a supplied endpoint. Under #2 D21, only automatic `enduser.pseudo.id` is permitted; it can correlate sessions and is not assumed anonymous. Keep identity/content capture disabled; all other enduser fields, names, raw IDs and OS/host identity remain prohibited.
<!-- soft-factory-v2:end -->
