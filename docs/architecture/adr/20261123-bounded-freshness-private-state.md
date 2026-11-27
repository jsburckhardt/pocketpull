# Full listing every run with bounded detail freshness and private state

- Status: accepted
- Date: 20261123
- Source: `docs/work/20261123-collector.md` D7, D9

## Context
An account can hold thousands of small recordings, so refetching every detail each run is costly. Pocket's update semantics are undocumented, so `updated_at` or titles cannot be trusted as change signals.

## Decision
List every page on every run. Fetch a recording's detail only when it is new, its snapshot is unusable, or its last successful fetch is at least `refresh_after` old (default 24h; `0` forces full refresh). Keep fetch timestamps as expendable private state outside the raw directory, namespaced by the canonical raw path, and refetch safely when state is missing or unusable.

## Consequences
- Easier: runs are cheap after the first; deleting state never deletes data.
- Harder: an upstream edit can take up to `refresh_after` plus the cron interval to appear.
- Forbidden: cache or lock markers inside raw; update-watermark optimisations without a new decision.

## Alternatives considered
- Full detail refresh every run: superseded by this decision as too costly.
- Trusting `updated_at`/title changes: undocumented semantics.
