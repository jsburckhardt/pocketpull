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
None yet.
<!-- soft-factory-v2:end -->
