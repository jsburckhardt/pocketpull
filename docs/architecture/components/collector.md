# Collector

- Code: `internal/collector`
- Status: active

## Responsibility
Runs one collection against the Pocket public API: lists every recording, fetches details that are new or stale, and commits raw snapshots and freshness state safely. It hides traversal, retries, validation, locking and atomic storage behind one call.

## Interface
```go
func Run(ctx context.Context, options Options) (Summary, error)

type Options struct {
    RawDir       string        // raw archive root; created if missing
    APIKey       string        // Bearer token
    BaseURL      string        // Pocket API base; fixtures substitute a loopback server
    Client       *http.Client
    Log          *slog.Logger
    StateDir     string        // private state base, must be outside RawDir
    RefreshAfter time.Duration // 0 = always fetch details
    Now          func() time.Time                          // optional; default time.Now
    Sleep        func(context.Context, time.Duration) error // optional; backoff waits
}

type Summary struct {
    Discovered, Fetched, Unchanged, Cached, Pending, Failed, Retries int
}
```
`Summary` is always returned, including on error, so callers can report partial progress.

## Invariants and errors
- One run per canonical raw directory: a non-waiting `flock` rejects overlap before any request.
- Snapshots are `<RawDir>/<id>/recording.json`, replaced atomically only when content changes; failures and remote absence never remove or damage them.
- Freshness advances only after a snapshot is committed; unusable state or snapshots force a refetch.
- At most four attempts per request with capped backoff and 30-second attempt timeouts; 401/403 stop all work immediately.
- Cancellation stops promptly and returns an incomplete error.
- Errors carry operational categories only, never HTTP bodies, URLs or OS paths.

## Configuration
None directly; the [CLI](cli.md) supplies `Options`.

## Depends on
Pocket public API (`/public/recordings` list and detail routes); local filesystem with Linux `flock`.

## Tests
- `internal/collector/collector_test.go`: freshness at scale, replacement, unusable state, state namespaces and raw aliases.
- `internal/collector/retry_test.go`: retry bounds, `Retry-After`, timeouts, cancellation, authentication stop.
- `internal/collector/snapshot_test.go`: canceled replacement and restart, symlink rejection.

## Decisions
- [One-way local archive](../adr/20261123-one-way-local-archive.md)
- [Bounded freshness and private state](../adr/20261123-bounded-freshness-private-state.md)
- [Overlap, retries and partial progress](../adr/20261123-overlap-retries-partial-progress.md)
- [Redacted stderr logs](../adr/20261123-redacted-stderr-logs.md)
