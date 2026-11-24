package collector

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestTransientListAndDetailRetries(t *testing.T) {
	var lists, details atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/public/recordings" {
			if lists.Add(1) == 1 {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			fmt.Fprint(w, `{"success":true,"data":[{"id":"r"},{"id":"r"}],"pagination":{"page":1,"has_more":false}}`)
			return
		}
		if details.Add(1) == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		fmt.Fprint(w, `{"success":true,"data":{"id":"r","transcript":null,"future":9007199254740993}}`)
	}))
	defer server.Close()
	options := testOptions(t, server)
	var waits []time.Duration
	options.Sleep = func(_ context.Context, delay time.Duration) error {
		waits = append(waits, delay)
		return nil
	}
	summary, err := Run(context.Background(), options)
	if err != nil || summary.Discovered != 1 || summary.Fetched != 1 || summary.Pending != 1 ||
		summary.Failed != 0 || summary.Retries != 2 || lists.Load() != 2 || details.Load() != 2 ||
		!reflect.DeepEqual(waits, []time.Duration{500 * time.Millisecond, 500 * time.Millisecond}) {
		t.Fatalf("transient list/detail work did not recover: summary=%+v err=%v lists=%d details=%d",
			summary, err, lists.Load(), details.Load())
	}
}

func TestExhaustedDetailAndPageRetainProgressAndFreshness(t *testing.T) {
	var failing atomic.Bool
	var badDetails, secondPages atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/public/recordings" {
			if r.URL.Query().Get("page") == "2" {
				secondPages.Add(1)
				w.WriteHeader(http.StatusInternalServerError)
				fmt.Fprint(w, "private API error")
				return
			}
			fmt.Fprintf(w, `{"success":true,"data":[{"id":"good"},{"id":"bad"},{"id":"bad"},{"id":"next"}],"pagination":{"page":1,"has_more":%t}}`, failing.Load())
			return
		}
		id := strings.TrimPrefix(r.URL.Path, "/public/recordings/")
		if id == "bad" {
			badDetails.Add(1)
			if failing.Load() {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
		}
		fmt.Fprintf(w, `{"success":true,"data":{"id":%q,"transcript":{}}}`, id)
	}))
	defer server.Close()
	options := testOptions(t, server)
	var waits []time.Duration
	options.Sleep = func(_ context.Context, delay time.Duration) error {
		waits = append(waits, delay)
		return nil
	}
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	options.Now = func() time.Time { return now }
	if _, err := Run(context.Background(), options); err != nil {
		t.Fatal(err)
	}
	rawPath := filepath.Join(options.RawDir, "bad", "recording.json")
	statePath := filepath.Join(options.StateDir, pathHash(options.RawDir), pathHash("bad")+".json")
	beforeRaw, _ := os.ReadFile(rawPath)
	beforeState, _ := os.ReadFile(statePath)
	now = now.Add(24 * time.Hour)
	failing.Store(true)
	summary, err := Run(context.Background(), options)
	want := Summary{Discovered: 3, Fetched: 2, Unchanged: 2, Failed: 2, Retries: 6}
	if err == nil || summary != want || badDetails.Load() != 5 || secondPages.Load() != 4 ||
		!reflect.DeepEqual(waits, []time.Duration{500 * time.Millisecond, time.Second, 2 * time.Second,
			500 * time.Millisecond, time.Second, 2 * time.Second}) {
		t.Fatalf("exhausted retries lost progress or bounds: %+v, %v; bad=%d page=%d waits=%v",
			summary, err, badDetails.Load(), secondPages.Load(), waits)
	}
	afterRaw, _ := os.ReadFile(rawPath)
	afterState, _ := os.ReadFile(statePath)
	if len(beforeRaw) == 0 || len(beforeState) == 0 || !bytes.Equal(beforeRaw, afterRaw) || !bytes.Equal(beforeState, afterState) {
		t.Fatal("exhausted requests changed prior raw or freshness")
	}
	failing.Store(false)
	summary, err = Run(context.Background(), options)
	if err != nil || summary != (Summary{Discovered: 3, Fetched: 1, Unchanged: 1, Cached: 2}) || badDetails.Load() != 6 {
		t.Fatalf("restart did not preserve first/next successes and retry failed work: %+v, %v", summary, err)
	}
}

func TestTransientNetworkOnListAndResponseBody(t *testing.T) {
	var lists, details atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		list := r.URL.Path == "/public/recordings"
		var count int32
		if list {
			count = lists.Add(1)
		} else {
			count = details.Add(1)
		}
		if count == 1 {
			connection, _, err := w.(http.Hijacker).Hijack()
			if err != nil {
				t.Error(err)
				return
			}
			if !list {
				fmt.Fprint(connection, "HTTP/1.1 200 OK\r\nContent-Length: 1000\r\n\r\n{\"private\":\"sentinel")
			}
			connection.Close()
			return
		}
		if list {
			fmt.Fprint(w, `{"success":true,"data":[{"id":"r"}],"pagination":{"page":1,"has_more":false}}`)
		} else {
			fmt.Fprint(w, `{"success":true,"data":{"id":"r","transcript":null}}`)
		}
	}))
	defer server.Close()
	options := testOptions(t, server)
	options.Sleep = func(context.Context, time.Duration) error { return nil }
	summary, err := Run(context.Background(), options)
	if err != nil || summary != (Summary{Discovered: 1, Fetched: 1, Pending: 1, Retries: 2}) ||
		lists.Load() != 2 || details.Load() != 2 {
		t.Fatalf("network/short body failures were not retried: %+v, %v; lists=%d details=%d", summary, err, lists.Load(), details.Load())
	}
}

func TestAuthenticationNeverRetriesOrContinues(t *testing.T) {
	for _, status := range []int{401, 403} {
		for _, operation := range []string{"list", "detail"} {
			t.Run(fmt.Sprintf("%d/%s", status, operation), func(t *testing.T) {
				var requests atomic.Int32
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					requests.Add(1)
					if operation == "detail" && r.URL.Path == "/public/recordings" {
						fmt.Fprint(w, `{"success":true,"data":[{"id":"bad"},{"id":"later"}],"pagination":{"page":1,"has_more":true}}`)
						return
					}
					w.WriteHeader(status)
					fmt.Fprint(w, "private error")
				}))
				defer server.Close()
				options := testOptions(t, server)
				options.Sleep = func(context.Context, time.Duration) error {
					t.Fatal("authentication retry attempted")
					return nil
				}
				summary, err := Run(context.Background(), options)
				wantRequests, discovered := int32(1), 0
				if operation == "detail" {
					wantRequests, discovered = 2, 2
				}
				if !errors.Is(err, errAuth) || requests.Load() != wantRequests ||
					summary != (Summary{Discovered: discovered, Failed: 1}) {
					t.Fatalf("auth did not stop promptly: %+v, %v; requests=%d", summary, err, requests.Load())
				}
			})
		}
	}
}

func TestRetryAfterBounds(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, test := range []struct {
		value string
		want  time.Duration
	}{
		{"0", 0}, {"3", 3 * time.Second}, {"10", 10 * time.Second}, {"9999999999999999999", 10 * time.Second},
		{strings.Repeat("9", 200), 0}, {"-1", 0}, {"private-key/URL", 0},
		{now.Add(3 * time.Second).Format(http.TimeFormat), 3 * time.Second},
		{now.Add(100 * time.Hour).Format(http.TimeFormat), 10 * time.Second},
		{now.Add(-time.Hour).Format(http.TimeFormat), 0},
	} {
		if got := retryDelay(test.value, now); got != test.want {
			t.Fatalf("Retry-After %q: got %v want %v", test.value, got, test.want)
		}
	}
}

func TestRetryWaitAndInflightCancellation(t *testing.T) {
	for _, stage := range []string{"backoff", "inflight"} {
		t.Run(stage, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			reached := make(chan struct{})
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				if stage == "inflight" {
					close(reached)
					<-r.Context().Done()
					return
				}
				w.Header().Set("Retry-After", "9999999999999999999")
				w.WriteHeader(http.StatusTooManyRequests)
			}))
			defer server.Close()
			options := testOptions(t, server)
			options.Sleep = func(ctx context.Context, delay time.Duration) error {
				if delay != 10*time.Second {
					t.Errorf("production Retry-After wait was not capped: %v", delay)
				}
				close(reached)
				return sleep(ctx, delay)
			}
			done := make(chan struct{})
			var summary Summary
			var runErr error
			go func() {
				summary, runErr = Run(ctx, options)
				close(done)
			}()
			select {
			case <-reached:
			case <-time.After(5 * time.Second):
				t.Fatal("request did not reach cancellation stage")
			}
			cancel()
			select {
			case <-done:
			case <-time.After(time.Second):
				t.Fatal("cancellation did not promptly stop production request/backoff")
			}
			if !errors.Is(runErr, errInterrupted) || summary != (Summary{Failed: 1}) || requests.Load() != 1 {
				t.Fatalf("cancellation retried or lost failure: %+v, %v requests=%d", summary, runErr, requests.Load())
			}
		})
	}
}

type deadlineTransport struct{ t *testing.T }

func (transport deadlineTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	deadline, present := request.Context().Deadline()
	if !present || time.Until(deadline) > 30*time.Second || time.Until(deadline) <= 0 {
		transport.t.Error("request lacked finite production deadline")
	}
	return nil, io.ErrUnexpectedEOF
}

func TestEveryAttemptHasFiniteTimeout(t *testing.T) {
	options := Options{
		RawDir: t.TempDir(), StateDir: t.TempDir(), BaseURL: "http://fixture",
		Client: &http.Client{Transport: deadlineTransport{t}, Timeout: time.Hour},
		Log:    slog.New(slog.NewJSONHandler(io.Discard, nil)),
		Sleep:  func(context.Context, time.Duration) error { return nil },
	}
	summary, err := Run(context.Background(), options)
	if err == nil || summary != (Summary{Failed: 1, Retries: 3}) {
		t.Fatalf("attempts were not bounded: %+v, %v", summary, err)
	}
}

func TestTimedOutRequestsExhaustWithinBounds(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		<-r.Context().Done()
	}))
	defer server.Close()
	options := testOptions(t, server)
	options.Client.Timeout = 20 * time.Millisecond
	options.Sleep = func(context.Context, time.Duration) error { return nil }
	summary, err := Run(context.Background(), options)
	if err == nil || summary != (Summary{Failed: 1, Retries: 3}) || requests.Load() != 4 {
		t.Fatalf("timeout did not exhaust four bounded attempts: %+v, %v requests=%d", summary, err, requests.Load())
	}
}

func TestMalformedDetailsPreserveSnapshotAndSuccessfulFetchTime(t *testing.T) {
	for _, body := range []string{
		`{"success":true,"data":`,
		`{"success":true,"data":null}`,
		`{"success":true,"data":[]}`,
		`{"success":false,"data":{"id":"r"},"error":"private API content"}`,
		`{"success":true,"data":{"id":"other"}}`,
	} {
		t.Run(body, func(t *testing.T) {
			var fail atomic.Bool
			var details atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/public/recordings" {
					fmt.Fprint(w, `{"success":true,"data":[{"id":"r"}],"pagination":{"page":1,"has_more":false}}`)
					return
				}
				details.Add(1)
				if fail.Load() {
					fmt.Fprint(w, body)
				} else {
					fmt.Fprint(w, `{"success":true,"data":{"id":"r","transcript":null}}`)
				}
			}))
			defer server.Close()
			options := testOptions(t, server)
			now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
			options.Now = func() time.Time { return now }
			if _, err := Run(context.Background(), options); err != nil {
				t.Fatal(err)
			}
			rawPath := filepath.Join(options.RawDir, "r", "recording.json")
			statePath := filepath.Join(options.StateDir, pathHash(options.RawDir), pathHash("r")+".json")
			beforeRaw, _ := os.ReadFile(rawPath)
			beforeState, _ := os.ReadFile(statePath)
			now = now.Add(24 * time.Hour)
			fail.Store(true)
			summary, err := Run(context.Background(), options)
			if err == nil || summary != (Summary{Discovered: 1, Failed: 1}) || details.Load() != 2 {
				t.Fatalf("malformed success was retried or accepted: %+v, %v details=%d", summary, err, details.Load())
			}
			afterRaw, _ := os.ReadFile(rawPath)
			afterState, _ := os.ReadFile(statePath)
			if len(beforeRaw) == 0 || len(beforeState) == 0 || !bytes.Equal(beforeRaw, afterRaw) || !bytes.Equal(beforeState, afterState) {
				t.Fatal("malformed detail changed raw or successful fetch time")
			}
			fail.Store(false)
			summary, err = Run(context.Background(), options)
			if err != nil || summary != (Summary{Discovered: 1, Fetched: 1, Unchanged: 1, Pending: 1}) || details.Load() != 3 {
				t.Fatalf("malformed success claimed freshness or valid pending recovery failed: %+v, %v", summary, err)
			}
		})
	}
}

func TestCancellationDuringFinalCacheCheckIsIncomplete(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/public/recordings" {
			fmt.Fprint(w, `{"success":true,"data":[{"id":"r"}],"pagination":{"page":1,"has_more":false}}`)
		} else {
			fmt.Fprint(w, `{"success":true,"data":{"id":"r","transcript":null}}`)
		}
	}))
	defer server.Close()
	options := testOptions(t, server)
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	options.Now = func() time.Time { return now }
	if _, err := Run(context.Background(), options); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	options.Now = func() time.Time {
		cancel()
		return now
	}
	summary, err := Run(ctx, options)
	if !errors.Is(err, errInterrupted) || summary != (Summary{Discovered: 1, Cached: 1, Failed: 1}) {
		t.Fatalf("final cache check falsely completed canceled run: %+v, %v", summary, err)
	}
}
