# CLI

- Code: `internal/cli`, entry point `cmd/pocketpull`
- Status: active

## Responsibility
Turns arguments, one configuration file and one secret source into collector options, runs a single collection, writes operational logs and returns the process exit code.

## Interface
```go
// Run executes one collection. Its returned code is suitable for os.Exit.
func Run(ctx context.Context, args []string, lookup func(string) (string, bool), stdout, stderr io.Writer) int
```
`cmd/pocketpull` passes `os.Args[1:]`, `os.LookupEnv` and a context canceled by SIGINT/SIGTERM. User-facing flags and configuration are documented in [configuration](../../usage/configuration.md).

## Invariants and errors
- `--help` and `--version` print to stdout, read no configuration or secrets, send no requests and return 0.
- Collection leaves stdout empty; logs go to stderr as JSON (default) or text, with the API key redacted from every string attribute.
- Invalid configuration logs a redacted category with a JSON error baseline and returns 1.
- Returns 0 only when the collection completed; incomplete, interrupted or overlapping runs return 1 after logging a summary.

## Configuration
Flags `--config`, `--raw-dir`, `--api-key-file`, `--state-dir`, `--refresh-after`, `--log-format`, `--log-level`; one strict YAML file; `POCKETPULL_API_KEY` or an owner-only secret file. See [configuration](../../usage/configuration.md).

## Depends on
[Collector](collector.md); `go.yaml.in/yaml/v3`.

## Tests
- `internal/cli/cli_test.go`: native subprocess smoke, traversal, snapshots, failures, unsafe IDs, help/version, redaction.
- `internal/cli/config_test.go`: config selection, precedence, strict parsing, secret sources and permissions, no `.env`, log formats.
- `internal/cli/freshness_test.go` and `outcome_test.go`: repeated runs, native overlap, kill and graceful-signal restart.

## Decisions
- [Run once; cron owns scheduling](../adr/20261123-run-once-collector.md)
- [Strict single YAML config](../adr/20261123-strict-single-yaml-config.md)
- [API key from environment or secret file](../adr/20261123-api-key-env-or-secret-file.md)
- [Redacted stderr logs](../adr/20261123-redacted-stderr-logs.md)
- [Go static Linux binary](../adr/20261123-go-static-linux-binary.md)
