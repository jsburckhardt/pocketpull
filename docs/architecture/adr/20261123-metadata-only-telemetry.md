# Optional metadata-only telemetry; only enduser.pseudo.id permitted

- Status: accepted
- Date: 20261123
- Source: `docs/work/20261123-dev-environment.md` D14, D21

## Context
Development observability is useful, but must not add infrastructure to this repo or export content or identity. Independent review found the Copilot SDK exports `enduser.pseudo.id` even with identity capture disabled.

## Decision
Pass an owner-supplied OTLP/HTTP protobuf endpoint (and optional headers) into devcontainer sessions with content capture set to `NO_CONTENT`; telemetry is off without an endpoint. Permit only the automatic `enduser.pseudo.id` identity field; it can correlate sessions and is not assumed anonymous. Keep every other identity and content capture disabled.

## Consequences
- Easier: plug into any external collector without repository changes.
- Harder: the smoke check must prove the exact allowed key and reject everything else.
- Forbidden: in-repo bridges or sidecars, Azure provisioning, connection-string substitution, other `enduser.*` fields, names, raw IDs or host identity.

## Alternatives considered
- Application Insights connection string: an undocumented substitution for OTLP ingestion.
- Broad relaxation of identity gates: rejected in favour of the exact exception.
