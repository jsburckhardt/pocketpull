# Logs and exit status

Go `slog` operational logs go to stderr; collection/error stdout remains empty.
`log_format`/`--log-format` accepts `json` (default) or `text`.
`log_level`/`--log-level` accepts `info` (default), `debug`, `warn`, or `error`:
`info` includes start, fetched-record, summary and successful `run_end` events;
`debug` additionally includes page-request and cache-hit events without request
contents; `warn` includes retry and state warnings but suppresses info/debug;
`error` shows errors only. Successful summaries are info-level,
incomplete-run summaries are error-level. Invalid configuration always uses a
safe JSON/error baseline, even if it attempted to select another format/level.
Events include a random, identity-free run ID, recording ID where safe, elapsed
time for detail/retry/run outcomes, and a final summary with `elapsed_ms` and
`complete`. Only successful runs emit `run_end`. `request_retry` warnings include
the failed attempt number, maximum attempts, operation, redacted reason and
planned delay; canceled waits do not count as additional attempts.
`discovered` counts unique IDs in validated listing pages, including rejected
unsafe IDs and IDs not attempted after interruption/authentication failure;
it does not claim unseen later pages were traversed. `fetched` counts
successfully validated and stored/unchanged details, `unchanged` is a subset
of `fetched`, and `pending` counts fetched records with missing/null transcript.
`cached` counts usable snapshots skipped by freshness (not fetched this run).
`pending` is based only on fetched detail data, never inferred for cache skips.
`failed` counts failed listing/detail/storage/state work, rejected IDs and
run-level coordination/interruption failures, not each failed retry attempt;
`retries` counts additional request attempts actually started after backoff.
A fetched snapshot can also have a state-write failure, and listing/run failures
have no recording ID, so counts need not partition `discovered`.
Logs exclude titles, transcript/summary content,
HTTP bodies, headers, keys, URLs and raw transport/filesystem errors.
Individual detail failures retain partial progress and exit nonzero; 401/403
stop promptly. Exit zero includes valid pending records, not necessarily
finished upstream transcripts. Exit 1 means invalid configuration, overlap,
interruption or incomplete required work. Help/version exit zero without
collecting; normal successful collection also exits zero.
