package collector

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestThousandRecordFreshness(t *testing.T) {
	var mu sync.Mutex
	ids := make([]string, 1000)
	for i := range ids {
		ids[i] = fmt.Sprintf("r-%04d", i)
	}
	// One cross-page duplicate, still no page larger than the API limit.
	ids = append(ids[:500], append([]string{ids[0]}, ids[500:]...)...)
	var requests []string
	updated := "old"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		requests = append(requests, r.URL.RequestURI())
		if r.Header.Get("Authorization") != "Bearer fixture-key" {
			t.Error("missing Bearer authentication")
		}
		if r.URL.Path == "/public/recordings" {
			page, _ := strconv.Atoi(r.URL.Query().Get("page"))
			if r.URL.RawQuery != fmt.Sprintf("page=%d&limit=100", page) {
				t.Error("unexpected listing filter")
			}
			start, end := (page-1)*100, min(page*100, len(ids))
			records := make([]map[string]string, 0, end-start)
			for _, id := range ids[start:end] {
				records = append(records, map[string]string{"id": id, "updated_at": updated})
			}
			data, _ := json.Marshal(records)
			fmt.Fprintf(w, `{"success":true,"data":%s,"pagination":{"page":%d,"has_more":%t}}`, data, page, end < len(ids))
			return
		}
		id := strings.TrimPrefix(r.URL.Path, "/public/recordings/")
		if !safeID.MatchString(id) {
			t.Error("unexpected route, including audio")
			w.WriteHeader(404)
			return
		}
		transcript := `{"segments":[{"text":"private-content","speaker":"s","start":0,"end":1}]}`
		if id == "r-0000" || id == "new" {
			transcript = "null"
		}
		fmt.Fprintf(w, `{"success":true,"data":{"id":%q,"title":%q,"updated_at":%q,"transcript":%s,"future":{"integer":9007199254740993},"summarizations":{"summary":"private-summary"}}}`, id, updated, updated, transcript)
	}))
	defer server.Close()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	start := now
	options := testOptions(t, server)
	options.Now = func() time.Time { return now }
	run := func(fetched, cached, pending int) {
		t.Helper()
		mu.Lock()
		requests = nil
		mu.Unlock()
		summary, err := Run(context.Background(), options)
		if err != nil || summary.Fetched != fetched || summary.Cached != cached || summary.Pending != pending || summary.Discovered != fetched+cached {
			t.Fatalf("summary=%+v, err=%v; want fetched=%d cached=%d pending=%d", summary, err, fetched, cached, pending)
		}
		mu.Lock()
		defer mu.Unlock()
		pages, detailCounts := 0, make(map[string]int)
		for _, route := range requests {
			if strings.Contains(route, "?") {
				pages++
			} else {
				detailCounts[route]++
			}
		}
		if pages != 11 || len(detailCounts) != fetched {
			t.Fatalf("pages=%d unique details=%d", pages, len(detailCounts))
		}
		for route, count := range detailCounts {
			if count != 1 {
				t.Fatalf("duplicate detail %s: %d", route, count)
			}
		}
		if fetched == 1000 && requests[1] != "/public/recordings/r-0000" {
			t.Fatal("new detail was not fetched before later pages")
		}
		if fetched == 1 && requests[1] != "/public/recordings/new" {
			t.Fatal("new record was not fetched immediately among cached records")
		}
	}
	run(1000, 0, 1)
	for i := 0; i < 1000; i++ {
		id := fmt.Sprintf("r-%04d", i)
		data, err := os.ReadFile(filepath.Join(options.RawDir, id, "recording.json"))
		if err != nil || !strings.Contains(string(data), "9007199254740993") || !strings.Contains(string(data), `"summarizations"`) {
			t.Fatalf("incomplete output for %s: %v", id, err)
		}
	}
	path := filepath.Join(options.RawDir, "r-0000", "recording.json")
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	now = start.Add(24*time.Hour - time.Nanosecond)
	mu.Lock()
	updated = "changed"
	mu.Unlock()
	run(0, 1000, 0)
	mu.Lock()
	ids = append([]string{"new"}, ids...)
	mu.Unlock()
	run(1, 1000, 1)
	mu.Lock()
	updated = "old"
	mu.Unlock()
	now = start.Add(24 * time.Hour)
	run(1000, 1, 1)
	after, err := os.Stat(path)
	if err != nil || !os.SameFile(before, after) || !before.ModTime().Equal(after.ModTime()) {
		t.Fatal("unchanged refresh rewrote raw snapshot")
	}
	now = start.Add(25 * time.Hour)
	run(0, 1001, 0)
	options.RefreshAfter = 0
	run(1001, 0, 2)
}

func testOptions(t *testing.T, server *httptest.Server) Options {
	t.Helper()
	return Options{
		RawDir: t.TempDir(), StateDir: t.TempDir(), RefreshAfter: 24 * time.Hour,
		APIKey: "fixture-key", BaseURL: server.URL, Client: server.Client(),
		Log: slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil)),
	}
}

func TestExpandedSnapshotFreshnessAndReplacement(t *testing.T) {
	// Tiny values expand past the transport limit when indented, without a
	// large HTTP body or millions of decoded numbers.
	values := strings.Repeat("[", 32) + strings.Repeat("null,", 499999) + "null" + strings.Repeat("]", 32)
	data := func(title string) string {
		return fmt.Sprintf(`{"id":"r","title":%q,"future":9007199254740993,"tiny":%s}`, title, values)
	}
	var changed atomic.Bool
	var details, listings atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer fixture-key" {
			t.Error("missing Bearer authentication")
		}
		switch r.URL.RequestURI() {
		case "/public/recordings?page=1&limit=100":
			listings.Add(1)
			fmt.Fprint(w, `{"success":true,"data":[{"id":"r"}],"pagination":{"page":1,"has_more":false}}`)
		case "/public/recordings/r":
			details.Add(1)
			title := "old"
			if changed.Load() {
				title = "new"
			}
			body := `{"success":true,"data":` + data(title) + "}"
			if len(body) >= 32<<20 {
				t.Error("fixture exceeds HTTP response limit")
			}
			fmt.Fprint(w, body)
		default:
			t.Error("unexpected request, including audio")
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	options := testOptions(t, server)
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	options.Now = func() time.Time { return now }
	runs, fetches := int32(0), int32(0)
	run := func(stage string, want Summary) {
		t.Helper()
		runs++
		fetches += int32(want.Fetched)
		summary, err := Run(context.Background(), options)
		if err != nil || summary != want || listings.Load() != runs || details.Load() != fetches {
			t.Fatalf("%s: summary=%+v err=%v listings=%d details=%d; want %+v listings=%d details=%d",
				stage, summary, err, listings.Load(), details.Load(), want, runs, fetches)
		}
	}
	path := filepath.Join(options.RawDir, "r", "recording.json")
	checkOutput := func(title string) os.FileInfo {
		t.Helper()
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Size() <= 32<<20 {
			t.Fatalf("fixture did not expand past HTTP limit: %d bytes", info.Size())
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var compact bytes.Buffer
		if err := json.Compact(&compact, raw); err != nil || compact.String() != data(title) {
			t.Fatalf("snapshot lost detail data: %v", err)
		}
		entries, err := os.ReadDir(filepath.Dir(path))
		if err != nil || len(entries) != 1 || entries[0].Name() != "recording.json" {
			t.Fatalf("unexpected snapshot files: %v, %v", entries, err)
		}
		return info
	}
	unchanged := func(before os.FileInfo) {
		t.Helper()
		after, err := os.Stat(path)
		if err != nil || !os.SameFile(before, after) || !before.ModTime().Equal(after.ModTime()) {
			t.Fatal("unchanged snapshot inode or mtime changed")
		}
	}
	fetched := Summary{Discovered: 1, Fetched: 1, Pending: 1}
	run("initial collection", fetched)
	before := checkOutput("old")
	t.Logf("expanded snapshot: %d bytes", before.Size())
	run("fresh repeat", Summary{Discovered: 1, Cached: 1})
	unchanged(before)
	options.RefreshAfter = 0
	refreshed := fetched
	refreshed.Unchanged = 1
	run("forced unchanged refresh", refreshed)
	unchanged(before)
	options.RefreshAfter = 24 * time.Hour
	now = now.Add(24 * time.Hour)
	run("expired unchanged refresh", refreshed)
	unchanged(before)
	changed.Store(true)
	options.RefreshAfter = 0
	run("changed refresh", fetched)
	after := checkOutput("new")
	if os.SameFile(before, after) {
		t.Fatal("changed snapshot was not atomically replaced")
	}
	options.RefreshAfter = 24 * time.Hour
	run("fresh changed repeat", Summary{Discovered: 1, Cached: 1})
	unchanged(after)
}

func TestFreshnessUsesCompletedFetchAndCommitsOnlySuccess(t *testing.T) {
	var clock atomic.Int64
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	clock.Store(start.UnixNano())
	var status atomic.Int32
	status.Store(200)
	var invalid atomic.Bool
	var details atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/public/recordings" {
			fmt.Fprint(w, `{"success":true,"data":[{"id":"r"}],"pagination":{"page":1,"has_more":false}}`)
			return
		}
		details.Add(1)
		clock.Add(int64(time.Hour))
		w.WriteHeader(int(status.Load()))
		if invalid.Load() {
			fmt.Fprint(w, `{"success":true,"data":null}`)
		} else {
			fmt.Fprint(w, `{"success":true,"data":{"id":"r","transcript":null}}`)
		}
	}))
	defer server.Close()
	options := testOptions(t, server)
	options.Now = func() time.Time { return time.Unix(0, clock.Load()).UTC() }
	run := func(wantError bool, fetched, cached int) {
		t.Helper()
		summary, err := Run(context.Background(), options)
		if (err != nil) != wantError || summary.Fetched != fetched || summary.Cached != cached {
			t.Fatalf("summary=%+v, err=%v", summary, err)
		}
	}
	run(false, 1, 0)
	statePath := filepath.Join(options.StateDir, pathHash(options.RawDir), pathHash("r")+".json")
	original, err := os.ReadFile(statePath)
	if err != nil || !strings.Contains(string(original), "2026-01-01T01:00:00Z") {
		t.Fatalf("fetch completion not recorded: %s, %v", original, err)
	}
	clock.Store(start.Add(24 * time.Hour).UnixNano())
	run(false, 0, 1)
	clock.Store(start.Add(25 * time.Hour).UnixNano())
	for _, fault := range []string{"404", "invalid", "write"} {
		t.Run(fault, func(t *testing.T) {
			status.Store(200)
			invalid.Store(false)
			rawPath := filepath.Join(options.RawDir, "r")
			if fault == "404" {
				status.Store(404)
			} else if fault == "invalid" {
				invalid.Store(true)
			} else {
				// Force a changed response by making the previous valid snapshot different.
				if err := os.WriteFile(filepath.Join(rawPath, "recording.json"), []byte(`{"id":"r","old":true}`), 0o600); err != nil {
					t.Fatal(err)
				}
				if err := os.Chmod(rawPath, 0o500); err != nil {
					t.Fatal(err)
				}
				defer os.Chmod(rawPath, 0o700)
			}
			prior, err := os.ReadFile(filepath.Join(rawPath, "recording.json"))
			if err != nil {
				t.Fatal(err)
			}
			run(true, 0, 0)
			current, err := os.ReadFile(statePath)
			if err != nil || !bytes.Equal(original, current) {
				t.Fatal("failed work advanced freshness")
			}
			current, err = os.ReadFile(filepath.Join(rawPath, "recording.json"))
			if err != nil || !bytes.Equal(prior, current) {
				t.Fatal("failed work changed committed raw")
			}
		})
	}
	status.Store(200)
	invalid.Store(false)
	run(false, 1, 0)
	run(false, 0, 1)
	if details.Load() != 5 {
		t.Fatalf("details=%d", details.Load())
	}
}

func TestUnusableStateAndRawRefetchSafely(t *testing.T) {
	for _, fault := range []string{
		"missing state", "corrupt state", "null state", "missing timestamp", "future timestamp",
		"unreadable state", "state file directory", "state namespace file", "unreadable namespace",
		"missing raw", "invalid raw", "null raw", "mismatched raw", "raw directory", "unreadable raw", "raw symlink",
		"state commit failure",
	} {
		t.Run(fault, func(t *testing.T) {
			var details atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/public/recordings" {
					fmt.Fprint(w, `{"success":true,"data":[{"id":"r"}],"pagination":{"page":1,"has_more":false}}`)
					return
				}
				details.Add(1)
				fmt.Fprint(w, `{"success":true,"data":{"id":"r","transcript":null}}`)
			}))
			defer server.Close()
			options := testOptions(t, server)
			now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
			options.Now = func() time.Time { return now }
			if _, err := Run(context.Background(), options); err != nil {
				t.Fatal(err)
			}
			namespace := filepath.Join(options.StateDir, pathHash(options.RawDir))
			statePath := filepath.Join(namespace, pathHash("r")+".json")
			rawPath := filepath.Join(options.RawDir, "r", "recording.json")
			remove := func(path string) {
				t.Helper()
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
			}
			write := func(path, data string) {
				t.Helper()
				if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			chmod := func(path string, mode os.FileMode) {
				t.Helper()
				if err := os.Chmod(path, mode); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { os.Chmod(path, 0o700) })
			}
			wantError := false
			switch fault {
			case "missing state":
				remove(statePath)
				remove(namespace)
			case "corrupt state":
				write(statePath, "{")
			case "null state":
				write(statePath, "null")
			case "missing timestamp":
				write(statePath, "{}")
			case "future timestamp":
				write(statePath, `{"last_success":"9999-01-01T00:00:00Z"}`)
			case "unreadable state":
				chmod(statePath, 0)
			case "state file directory":
				remove(statePath)
				if err := os.Mkdir(statePath, 0o700); err != nil {
					t.Fatal(err)
				}
				wantError = true
			case "state namespace file":
				remove(statePath)
				remove(namespace)
				write(namespace, "invalid")
				wantError = true
			case "unreadable namespace":
				chmod(namespace, 0)
				wantError = true
			case "missing raw":
				remove(rawPath)
			case "invalid raw":
				write(rawPath, "{")
			case "null raw":
				write(rawPath, "null")
			case "mismatched raw":
				write(rawPath, `{"id":"other"}`)
			case "raw directory":
				remove(rawPath)
				if err := os.Mkdir(rawPath, 0o700); err != nil {
					t.Fatal(err)
				}
				wantError = true
			case "unreadable raw":
				chmod(rawPath, 0)
				wantError = true
			case "raw symlink":
				remove(rawPath)
				if err := os.Symlink(statePath, rawPath); err != nil {
					t.Fatal(err)
				}
				wantError = true
			case "state commit failure":
				chmod(namespace, 0o500)
				now = now.Add(24 * time.Hour)
				wantError = true
			}
			summary, err := Run(context.Background(), options)
			if details.Load() != 2 || summary.Cached != 0 || (err != nil) != wantError {
				t.Fatalf("unsafe skip or unexpected failure: details=%d summary=%+v err=%v", details.Load(), summary, err)
			}
			if wantError && summary.Failed == 0 {
				t.Fatal("failure was not surfaced")
			}
			if fault == "state commit failure" {
				chmod(namespace, 0o700)
				summary, err := Run(context.Background(), options)
				if err != nil || summary.Fetched != 1 || details.Load() != 3 {
					t.Fatalf("raw commit without state commit claimed freshness: %+v, %v", summary, err)
				}
			}
		})
	}
}

func TestStateNamespacesAndRawAliases(t *testing.T) {
	var details atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/public/recordings" {
			fmt.Fprint(w, `{"success":true,"data":[{"id":"r"}],"pagination":{"page":1,"has_more":false}}`)
			return
		}
		details.Add(1)
		fmt.Fprint(w, `{"success":true,"data":{"id":"r"}}`)
	}))
	defer server.Close()
	options := testOptions(t, server)
	original := options.RawDir
	if _, err := Run(context.Background(), options); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(original, alias); err != nil {
		t.Fatal(err)
	}
	options.RawDir = alias
	if summary, err := Run(context.Background(), options); err != nil || summary.Cached != 1 {
		t.Fatalf("alias did not reuse canonical freshness: %+v, %v", summary, err)
	}
	options.RawDir = t.TempDir()
	if summary, err := Run(context.Background(), options); err != nil || summary.Fetched != 1 {
		t.Fatalf("separate raw reused another namespace: %+v, %v", summary, err)
	}
	entries, err := os.ReadDir(options.StateDir)
	if err != nil || len(entries) != 2 || details.Load() != 2 {
		t.Fatalf("namespaces=%v details=%d err=%v", entries, details.Load(), err)
	}
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil || info.Mode().Perm() != 0o700 {
			t.Fatal("namespace is not private")
		}
	}
	options.RawDir = original
	for _, state := range []string{original, alias, filepath.Join(original, "new", "state"), filepath.Join(alias, "state")} {
		options.StateDir = state
		if _, err := Run(context.Background(), options); err == nil || err.Error() != "state directory must be outside raw" {
			t.Fatalf("state inside raw accepted: %v", err)
		}
	}
	rawEntries, err := os.ReadDir(original)
	if err != nil || len(rawEntries) != 1 || rawEntries[0].Name() != "r" || details.Load() != 2 {
		t.Fatal("invalid state placement wrote raw markers or made requests")
	}
}
