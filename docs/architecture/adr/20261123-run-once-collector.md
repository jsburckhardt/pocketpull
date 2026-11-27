# Run once; cron owns scheduling

- Status: accepted
- Date: 20261123
- Source: `docs/work/20261123-collector.md` D3

## Context
The collector needs to run unattended on a schedule. A resident scheduler or daemon would add process supervision, configuration and failure modes the owner considered unnecessary.

## Decision
Make `pocketpull` a run-once CLI: each invocation performs one collection and exits. Leave all scheduling, log rotation and supervision to cron or other external tooling.

## Consequences
- Easier: no service lifecycle, no in-process timers; restarts are just the next run.
- Harder: overlap and interruption must be handled per run (see [overlap and failures](20261123-overlap-retries-partial-progress.md)).
- Forbidden: internal scheduling, daemon/service operation and webhooks.

## Alternatives considered
- Resident scheduler/daemon: rejected by the owner as unnecessary.
