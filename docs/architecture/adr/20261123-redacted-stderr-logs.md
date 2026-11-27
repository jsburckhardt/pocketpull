# JSON operational logs on stderr with content and credential redaction

- Status: accepted
- Date: 20261123
- Source: `docs/work/20261123-collector.md` D11

## Context
Operators need machine-readable run outcomes and local diagnosis, separate from development telemetry, without leaking transcripts or credentials into log files.

## Decision
Write `slog` operational logs to stderr, JSON by default with a text option and info/debug/warn/error levels, ending with a summary event. Exclude titles, transcript and summary content, HTTP bodies, headers, keys, URLs and raw transport or filesystem errors; report only redacted operational categories.

## Consequences
- Easier: cron can append stderr to a file and tools can parse it.
- Harder: diagnosing a failure relies on categories and IDs, not raw error text.
- Forbidden: content or secrets in logs; application telemetry exporters.

## Alternatives considered
None recorded.
