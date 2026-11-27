# Reject overlap, retry within bounds, keep partial progress

- Status: accepted
- Date: 20261123
- Source: `docs/work/20261123-collector.md` D10

## Context
Cron can start a run while the previous one is still going, the API can fail transiently, and runs can be interrupted. None of this may corrupt collected data, and failures must be visible to the scheduler.

## Decision
Hold a non-waiting `flock` per canonical raw directory and fail an overlapping run before any request. Retry transient failures (network errors, 429, 5xx) at most four attempts per request with capped backoff and per-attempt timeouts. Commit each recording's snapshot and state as it succeeds, keep going past individual failures, stop immediately on 401/403, and exit nonzero when any required work is incomplete.

## Consequences
- Easier: kill or interrupt at any point and the next run recovers; no stale lock files.
- Harder: a run can exit 1 while still having saved useful work; operators read the summary.
- Forbidden: unbounded retries, following redirects, marking an uncommitted snapshot fresh.

## Alternatives considered
None recorded.
