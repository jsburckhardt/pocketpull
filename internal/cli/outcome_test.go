package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

type retryingTransport struct {
	transport http.RoundTripper
	attempts  atomic.Int32
}

func (transport *retryingTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if transport.attempts.Add(1) == 1 {
		return nil, fmt.Errorf("http://user:%s@fixture/ private-transcript-sentinel: %w", fixtureKey, syscall.ECONNRESET)
	}
	return transport.transport.RoundTrip(request)
}

func TestDebugRetryOperationalOutcomeAndRedaction(t *testing.T) {
	for _, format := range []string{"json", "text"} {
		t.Run(format, func(t *testing.T) {
			var lists, details atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/public/recordings" {
					if lists.Add(1) == 1 {
						w.Header().Set("Retry-After", "http://user:"+fixtureKey+"@fixture/")
						w.WriteHeader(http.StatusTooManyRequests)
						fmt.Fprint(w, fixtureKey+" private-transcript-sentinel")
						return
					}
					fmt.Fprint(w, listPage([]string{fixtureKey, "recording-1", "pending", "bad"}, 1, false))
					return
				}
				switch strings.TrimPrefix(r.URL.Path, "/public/recordings/") {
				case fixtureKey:
					fmt.Fprintf(w, `{"success":true,"data":{"id":%q,"transcript":{}}}`, fixtureKey)
				case "recording-1":
					if details.Add(1) == 1 {
						w.WriteHeader(http.StatusServiceUnavailable)
						fmt.Fprint(w, `{"error":"Authorization: Bearer `+fixtureKey+` private-title-sentinel private-summary-sentinel"}`)
						return
					}
					fmt.Fprintf(w, `{"success":true,"data":%s}`, fixtureDetail)
				case "pending":
					fmt.Fprint(w, `{"success":true,"data":{"id":"pending","transcript":null}}`)
				case "bad":
					w.WriteHeader(http.StatusNotFound)
					fmt.Fprint(w, fixtureDetail)
				default:
					t.Error("unexpected request")
				}
			}))
			defer server.Close()
			client := server.Client()
			client.Transport = &retryingTransport{transport: client.Transport}
			var stdout, stderr bytes.Buffer
			code := run(testContext(t), []string{"--raw-dir", t.TempDir(), "--state-dir", t.TempDir(),
				"--log-format", format, "--log-level", "debug"},
				fixtureEnvironment(map[string]string{"POCKETPULL_API_KEY": fixtureKey}),
				&stdout, &stderr, dependencies{baseURL: server.URL, client: client})
			logs := stderr.String()
			assertRedacted(t, logs)
			if code != 1 || stdout.Len() != 0 || strings.Contains(logs, "run_end") || !strings.Contains(logs, "run_failed") ||
				!strings.Contains(logs, "request_retry") || !strings.Contains(logs, "elapsed_ms") {
				t.Fatalf("missing truthful partial operational outcome: code=%d stdout=%s logs=%s", code, &stdout, logs)
			}
			if format == "json" {
				var runID string
				retries := 0
				for _, line := range strings.Split(strings.TrimSpace(logs), "\n") {
					event := decodeJSON(t, []byte(line)).(map[string]any)
					if event["run_id"] == "" || event["run_id"] == nil {
						t.Fatal("event missing run ID")
					}
					if runID == "" {
						runID = event["run_id"].(string)
					}
					if event["run_id"] != runID {
						t.Fatal("event run IDs differ")
					}
					if event["event"] == "request_retry" {
						retries++
						attempt, delay := json.Number("1"), json.Number("500")
						if retries == 2 {
							attempt, delay = "2", "1000"
						}
						if event["attempt"] != attempt || event["max_attempts"] != json.Number("4") ||
							event["delay_ms"] != delay || event["operation"] == nil {
							t.Fatalf("retry lacks bounded metadata: %s", line)
						}
					}
					if event["event"] == "summary" {
						if event["discovered"] != json.Number("4") || event["fetched"] != json.Number("3") ||
							event["unchanged"] != json.Number("0") || event["cached"] != json.Number("0") ||
							event["pending"] != json.Number("1") || event["failed"] != json.Number("1") ||
							event["retries"] != json.Number("3") || event["complete"] != false || event["level"] != "ERROR" {
							t.Fatalf("partial summary misreported work: %s", line)
						}
					}
				}
				if retries != 3 {
					t.Fatalf("retry events=%d", retries)
				}
			} else if !strings.Contains(logs, "discovered=4 fetched=3 unchanged=0 cached=0 pending=1 failed=1 retries=3 complete=false") {
				t.Fatalf("text summary lost partial counts: %s", logs)
			}
		})
	}
}

func TestNativeGracefulSignalAndRestart(t *testing.T) {
	for _, termination := range []os.Signal{syscall.SIGTERM, os.Interrupt} {
		t.Run(termination.String(), func(t *testing.T) {
			stalled := make(chan struct{})
			var firstRequest atomic.Bool
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/public/recordings":
					fmt.Fprint(w, listPage([]string{"good", "later"}, 1, false))
				case "/public/recordings/good":
					fmt.Fprint(w, `{"success":true,"data":{"id":"good","transcript":{}}}`)
				case "/public/recordings/later":
					if firstRequest.CompareAndSwap(false, true) {
						close(stalled)
						<-r.Context().Done()
						return
					}
					fmt.Fprint(w, `{"success":true,"data":{"id":"later","transcript":null}}`)
				default:
					t.Error("unexpected request")
				}
			}))
			defer server.Close()
			raw, state, home := t.TempDir(), t.TempDir(), t.TempDir()
			process := func() (*exec.Cmd, *bytes.Buffer, *bytes.Buffer) {
				command := exec.CommandContext(testContext(t), os.Args[0], "-test.run=^TestCLIProcess$", "--",
					"--raw-dir", raw, "--state-dir", state, "--log-level", "error")
				command.Dir = home
				command.Env = append(cleanEnvironment(), "HOME="+home, "POCKETPULL_API_KEY="+fixtureKey,
					"POCKETPULL_TEST_PROCESS=1", "POCKETPULL_TEST_ENDPOINT="+server.URL)
				stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
				command.Stdout, command.Stderr = stdout, stderr
				return command, stdout, stderr
			}
			first, stdout, stderr := process()
			if err := first.Start(); err != nil {
				t.Fatal(err)
			}
			waited := false
			t.Cleanup(func() {
				if !waited {
					first.Process.Kill()
					first.Wait()
				}
			})
			select {
			case <-stalled:
			case <-time.After(10 * time.Second):
				t.Fatal("native child did not commit first record and reach inflight request")
			}
			path := filepath.Join(raw, "good", "recording.json")
			prior, err := os.ReadFile(path)
			if err != nil {
				t.Fatal("partial committed snapshot missing")
			}
			before, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			if err := first.Process.Signal(termination); err != nil {
				t.Fatal(err)
			}
			done := make(chan error, 1)
			go func() { done <- first.Wait() }()
			select {
			case err := <-done:
				waited = true
				if exit, ok := err.(*exec.ExitError); !ok || exit.ExitCode() != 1 {
					t.Fatalf("signal did not gracefully return incomplete code: %v", err)
				}
			case <-time.After(2 * time.Second):
				first.Process.Kill()
				<-done
				waited = true
				t.Fatal("signal did not promptly cancel inflight request")
			}
			logs := stderr.String()
			assertRedacted(t, logs)
			if stdout.Len() != 0 || strings.Contains(logs, "run_end") ||
				!strings.Contains(logs, `"discovered":2,"fetched":1,"unchanged":0,"cached":0,"pending":0,"failed":1,"retries":0,"complete":false`) ||
				!strings.Contains(logs, `"reason":"collection interrupted"`) {
				t.Fatalf("interruption lacked truthful partial error-level summary: %s", logs)
			}
			// Use debug on restart to observe freshness and completed outcome.
			restart, stdout, stderr := process()
			restart.Args[len(restart.Args)-1] = "debug"
			if err := restart.Run(); err != nil || stdout.Len() != 0 {
				t.Fatalf("graceful restart failed: %v %s", err, stderr)
			}
			if !strings.Contains(stderr.String(), `"fetched":1,"unchanged":0,"cached":1,"pending":1,"failed":0,"retries":0,"complete":true`) ||
				!strings.Contains(stderr.String(), `"event":"run_end"`) {
				t.Fatalf("restart lost freshness or completed outcome: %s", stderr)
			}
			assertRedacted(t, stderr.String())
			after, err := os.Stat(path)
			current, readErr := os.ReadFile(path)
			if err != nil || readErr != nil || !os.SameFile(before, after) || !bytes.Equal(prior, current) ||
				!before.ModTime().Equal(after.ModTime()) {
				t.Fatal("signal/restart changed committed snapshot")
			}
			assertSnapshotFiles(t, filepath.Join(raw, "good"))
			assertSnapshotFiles(t, filepath.Join(raw, "later"))
		})
	}
}
