# Collection behaviour

## Refresh policy and private state

Every run lists **all** pages and deduplicates IDs. New recordings are fetched
immediately, before requesting later pages. Existing recordings are skipped only
when their raw snapshot is usable and the last successful detail fetch is
strictly less than `refresh_after` old. At the exact boundary they are fetched;
`--refresh-after 0` always fetches details. Valid pending recordings use the same
interval. Titles and `updated_at` are not freshness signals.

Successful unchanged refreshes advance freshness without rewriting raw data.
The timestamp is measured when the detail fetch completes, not at run start;
HTTP, validation and raw-write failures never advance it. Missing, malformed,
unreadable or otherwise unusable state/output forces a safe refetch. State is
expendable: removing it does not remove the archive. If state cannot be persisted,
collection still retains successful raw snapshots, logs the failure and exits
nonzero; restoring usable state storage permits recovery on the next run.

Private state lives under `<state-base>/<sha256(canonical-raw-path)>/`, with
one `<sha256(recording-id)>.json` successful-fetch timestamp per recording.
Namespaces are owner-only directories (0700) and timestamp files are 0600.
The configured state base must be outside raw, including symlink aliases;
no cache or lock markers are written into raw. Raw is safely created before
canonicalisation, so spelling it through a symlink reuses the same namespace.
Different raw directories have independent freshness even with one state base.
Changing the state base forces refetching unless it already holds usable state.

One collection per canonical raw directory is enforced by a nonwaiting Linux
`flock` on an open raw-directory descriptor. Overlap fails nonzero before any
collection request, even with a different state base or a raw-path alias.
Unrelated raw directories can run concurrently. Normal exit or process death,
including SIGKILL, releases the lock automatically; there is no stale PID file.
Raw snapshots are committed before atomic state writes, and successful progress
is persisted per recording, not only at the end of traversal. Interruption can
require a repeat fetch but cannot make an uncommitted raw snapshot fresh.

## Raw artifacts and failure recovery

Each snapshot is `<raw-dir>/<recording-id>/recording.json`: the **entire detail
`data` object**, not its envelope. Unknown fields and exact large JSON integers
survive alongside segments, speakers, timing, tags and API-provided summaries.
IDs must be 1–255 ASCII characters, start with a letter/digit, and thereafter
contain only letters/digits, dots, underscores or hyphens. Unsafe IDs and
mismatched detail IDs fail before snapshot writes. Recording-folder symlinks
are rejected. New directories are owner-only and new snapshot files are mode
0600; run the downstream consumer as the same user or explicitly arrange access.

Changed content replaces the previous file with a synced, same-directory
temporary file and atomic rename. Equivalent JSON (including reordered keys,
whitespace and numeric spelling) is not rewritten. Invalid/failed responses
and unsuccessful writes retain prior snapshots; remote absence/404 never
removes them. Valid missing/null transcript data is pending, not an error.
An upstream edit or completed transcript can take up to `refresh_after`
(24 hours by default) plus the cron interval and run time to appear. No guarantee
applies to content that disappears before a successful fetch. Zero refresh
reduces this latency to the cron interval plus run time, at the cost of fetching
every detail on every run.

Both listing and detail requests retry transient network failures (including
timeouts, connection resets and truncated response bodies), HTTP 429, and HTTP
500-599. Each request has at most **four attempts**, with backoff of 500 ms,
1 second and 2 seconds. A valid `Retry-After` integer in seconds or HTTP date
can lengthen a backoff, but is capped at **10 seconds**. Invalid, negative,
overflowing or past values do not extend the backoff. Each attempt has a
**30-second timeout**, covering response-body reading, and a **32 MiB response
limit**. The complete request therefore has at most 120 seconds of request
time plus 30 seconds of retry waits (excluding local processing); no upstream
numeric quota is assumed. Oversized or malformed successes, permanent 4xx and
redirects are not retried; redirects are never followed. 401/403 stop all
further list/detail work immediately.

Individual detail/validation/storage failures do not prevent other distinct
IDs from being processed; a failed listing stops traversal rather than claiming
it finished. Required failures exit nonzero while retaining successful partial
work and prior snapshots. SIGINT/SIGTERM cancel in-flight requests and backoff
promptly, report an incomplete summary, and release the lock. SIGKILL cannot
report a summary but also releases the lock. Restart uses per-record committed
freshness, safely refetches unfinished work, and never needs a lock-file removal.
A killed write can leave an uncommitted `.recording-*.tmp` file; it is not an
archive snapshot or freshness entry and is never used on restart. Gracefully
canceled writes clean up their temporary files.
