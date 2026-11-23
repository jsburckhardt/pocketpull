package cli

import (
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/jsburckhardt/pocketpull/internal/collector"
)

const productionEndpoint = "https://public.heypocketai.com/api/v1"

var version = "dev"

const usage = `Usage: pocketpull [OPTIONS]

Collect once from Pocket into DIRECTORY/<recording-id>/recording.json.
Requires POCKETPULL_API_KEY OR an owner-only API key file; no .env is loaded.

Options:
  --config FILE          Select one strict YAML configuration
  --raw-dir DIRECTORY    Required raw snapshot directory (or raw_dir in YAML)
  --api-key-file FILE     User-owned secret file with no group/other read access
  --state-dir DIRECTORY  Private state base outside raw (canonical namespace)
  --refresh-after TIME   Nonnegative Go duration, default 24h; 0 = full refresh
  --log-format FORMAT    json (default) or text, on stderr
  --log-level LEVEL      info (default), debug, warn or error
  --help                 Show help without reading config/secrets or collecting
  --version              Show version without reading config/secrets or collecting

Discovery: ./pocketpull.yaml, then XDG_CONFIG_HOME/pocketpull/config.yaml
(or HOME/.config/pocketpull/config.yaml). No merging; explicit flags win.
YAML paths are file-relative; CLI paths are working-directory-relative.
Default state base: XDG_STATE_HOME/pocketpull or HOME/.local/state/pocketpull.
Collection runs once and leaves stdout free; cron owns scheduling.
`

type dependencies struct {
	baseURL string
	client  *http.Client
}

// Run executes one collection. Its returned code is suitable for os.Exit.
func Run(ctx context.Context, args []string, lookup func(string) (string, bool), stdout, stderr io.Writer) int {
	return run(ctx, args, lookup, stdout, stderr, dependencies{
		baseURL: productionEndpoint,
		client:  &http.Client{Timeout: 30 * time.Second},
	})
}

func run(ctx context.Context, args []string, lookup environment, stdout, stderr io.Writer, deps dependencies) int {
	config, information, err := configure(args, lookup)
	if information != "" {
		fmt.Fprint(stdout, information)
		return 0
	}
	if err != nil {
		slog.New(slog.NewJSONHandler(stderr, nil)).Error("invalid configuration",
			"event", "configuration_failed", "reason", err.Error())
		return 1
	}
	options := &slog.HandlerOptions{
		Level: config.logLevel,
		ReplaceAttr: func(_ []string, attr slog.Attr) slog.Attr {
			if attr.Value.Kind() == slog.KindString {
				attr.Value = slog.StringValue(strings.ReplaceAll(attr.Value.String(), config.apiKey, "[redacted]"))
			}
			return attr
		},
	}
	var handler slog.Handler = slog.NewJSONHandler(stderr, options)
	if config.logFormat == "text" {
		handler = slog.NewTextHandler(stderr, options)
	}
	log := slog.New(handler).With("run_id", rand.Text())
	start := time.Now()
	log.Info("collection started", "event", "run_started", "elapsed_ms", 0)
	summary, err := collector.Run(ctx, collector.Options{
		RawDir: config.rawDir, APIKey: config.apiKey, StateDir: config.stateDir, RefreshAfter: config.refreshAfter,
		BaseURL: deps.baseURL, Client: deps.client, Log: log,
	})
	level := slog.LevelInfo
	if err != nil {
		level = slog.LevelError
		log.Error("collection incomplete", "event", "run_failed",
			"elapsed_ms", time.Since(start).Milliseconds(), "reason", err.Error())
	}
	log.Log(ctx, level, "collection summary", "event", "summary",
		"elapsed_ms", time.Since(start).Milliseconds(),
		"discovered", summary.Discovered, "fetched", summary.Fetched,
		"unchanged", summary.Unchanged, "cached", summary.Cached,
		"pending", summary.Pending, "failed", summary.Failed, "retries", summary.Retries,
		"complete", err == nil)
	if err != nil {
		return 1
	}
	log.Info("collection completed", "event", "run_end", "elapsed_ms", time.Since(start).Milliseconds())
	return 0
}
