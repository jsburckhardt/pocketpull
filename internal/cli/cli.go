package cli

import (
	"context"
	"crypto/rand"
	"flag"
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

const usage = `Usage: pocketpull --raw-dir DIRECTORY

Collect once from Pocket into DIRECTORY/<recording-id>/recording.json.
Requires POCKETPULL_API_KEY in the environment; no .env file is loaded.

Options:
  --raw-dir DIRECTORY  Required raw snapshot directory
  --help               Show help without collecting
  --version            Show version without collecting

Logs are JSON on stderr; collection leaves stdout free.
`

type dependencies struct {
	baseURL string
	client  *http.Client
}

// Run executes one collection. Its returned code is suitable for os.Exit.
func Run(ctx context.Context, args []string, getenv func(string) string, stdout, stderr io.Writer) int {
	return run(ctx, args, getenv, stdout, stderr, dependencies{
		baseURL: productionEndpoint,
		client:  &http.Client{Timeout: 30 * time.Second},
	})
}

func run(ctx context.Context, args []string, getenv func(string) string, stdout, stderr io.Writer, deps dependencies) int {
	key := strings.TrimSpace(getenv("POCKETPULL_API_KEY"))
	log := slog.New(slog.NewJSONHandler(stderr, &slog.HandlerOptions{
		ReplaceAttr: func(_ []string, attr slog.Attr) slog.Attr {
			if key != "" && attr.Value.Kind() == slog.KindString {
				attr.Value = slog.StringValue(strings.ReplaceAll(attr.Value.String(), key, "[redacted]"))
			}
			return attr
		},
	})).With("run_id", rand.Text())
	flags := flag.NewFlagSet("pocketpull", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	rawDir := flags.String("raw-dir", "", "")
	help := flags.Bool("help", false, "")
	showVersion := flags.Bool("version", false, "")
	if err := flags.Parse(args); err != nil {
		if err == flag.ErrHelp {
			fmt.Fprint(stdout, usage)
			return 0
		}
		log.Error("invalid arguments", "event", "configuration_failed")
		return 1
	}
	if *help {
		fmt.Fprint(stdout, usage)
		return 0
	}
	if *showVersion {
		fmt.Fprintln(stdout, "pocketpull "+version)
		return 0
	}
	if flags.NArg() != 0 || strings.TrimSpace(*rawDir) == "" || key == "" || strings.ContainsAny(key, "\r\n") {
		log.Error("raw directory and valid environment credential required", "event", "configuration_failed")
		return 1
	}
	start := time.Now()
	log.Info("collection started", "event", "run_started")
	summary, err := collector.Run(ctx, collector.Options{
		RawDir: *rawDir, APIKey: key, BaseURL: deps.baseURL, Client: deps.client, Log: log,
	})
	level := slog.LevelInfo
	if err != nil {
		level = slog.LevelError
		log.Error("collection incomplete", "event", "run_failed", "reason", err.Error())
	}
	log.Log(ctx, level, "collection summary", "event", "summary",
		"elapsed_ms", time.Since(start).Milliseconds(),
		"discovered", summary.Discovered, "fetched", summary.Fetched,
		"unchanged", summary.Unchanged, "cached", 0,
		"pending", summary.Pending, "failed", summary.Failed, "retries", 0)
	if err != nil {
		return 1
	}
	return 0
}
