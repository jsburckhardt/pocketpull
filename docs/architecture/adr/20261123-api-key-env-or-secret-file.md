# API key from the environment or an owner-only secret file

- Status: accepted
- Date: 20261123
- Source: `docs/work/20261123-collector.md` D6

## Context
The Pocket API key must reach the collector in development and under cron without appearing in process lists, tracked configuration or logs.

## Decision
Read exactly one secret source: `POCKETPULL_API_KEY` or an `api_key_file` that is a regular file owned by the effective user with no group/other read permission. Treat both sources being set as a conflict. Do not load `.env` automatically, and do not accept a literal key on the command line or in YAML.

## Consequences
- Easier: cron uses a secret file; development exports the variable explicitly.
- Harder: users must set file ownership and `chmod 600` themselves; the collector never fixes permissions.
- Forbidden: CLI key flags, literal `api_key` in YAML, automatic `.env` loading, unredacted secrets in logs or errors.

## Alternatives considered
- Automatic `.env` loading: out of scope; secrets must be exported explicitly.
- Literal key flag or YAML key: leaks through process lists and tracked files.
