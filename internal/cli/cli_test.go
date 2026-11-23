package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

const fixtureKey = "fixture-secret-not-a-real-key"
const fixtureDetail = `{"id":"recording-1","title":"private-title-sentinel","state":"completed","transcript":{"segments":[{"start":0,"end":1.25,"speaker":"s1","speakerName":"private-speaker-sentinel","text":"private-transcript-sentinel","originalText":"original"}]},"summarizations":{"summary":"private-summary-sentinel"},"speakers":[{"id":"s1"}],"tags":["work"],"future_field":{"integer":9007199254740993}}`

// The child executes the native CLI entry with an internal fixture seam, never
// a public flag or environment variable understood by the production binary.
func TestCLIProcess(t *testing.T) {
	if os.Getenv("POCKETPULL_TEST_PROCESS") != "1" {
		return
	}
	os.Exit(run(context.Background(), os.Args[stringsIndex(os.Args, "--")+1:], os.Getenv, os.Stdout, os.Stderr, dependencies{
		baseURL: os.Getenv("POCKETPULL_TEST_ENDPOINT"),
		client:  &http.Client{Timeout: 5 * time.Second},
	}))
}

func stringsIndex(values []string, target string) int {
	for i, value := range values {
		if value == target {
			return i
		}
	}
	panic("missing test argument separator")
}

func TestNativeSmoke(t *testing.T) {
	var mu sync.Mutex
	var requests []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requests = append(requests, r.Method+" "+r.URL.RequestURI())
		mu.Unlock()
		if r.Header.Get("Authorization") != "Bearer "+fixtureKey {
			t.Error("missing fixture Bearer authentication")
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.RequestURI() {
		case "/public/recordings?page=1&limit=100":
			fmt.Fprint(w, `{"success":true,"data":[{"id":"recording-1"}],"pagination":{"has_more":false,"page":1,"limit":100,"total":1,"total_pages":1}}`)
		case "/public/recordings/recording-1":
			fmt.Fprintf(w, `{"success":true,"data":%s}`, fixtureDetail)
		default:
			t.Errorf("unexpected route (including audio): %s", r.URL.RequestURI())
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	raw := t.TempDir()
	command := exec.CommandContext(testContext(t), os.Args[0], "-test.run=^TestCLIProcess$", "--", "--raw-dir", raw)
	command.Env = append(cleanEnvironment(), "POCKETPULL_TEST_PROCESS=1", "POCKETPULL_TEST_ENDPOINT="+server.URL, "POCKETPULL_API_KEY="+fixtureKey)
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	if err := command.Run(); err != nil {
		t.Fatalf("native collection failed: %v; stderr=%s", err, &stderr)
	}
	if stdout.Len() != 0 {
		t.Fatalf("collection wrote stdout: %q", &stdout)
	}
	got, err := os.ReadFile(filepath.Join(raw, "recording-1", "recording.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decodeJSON(t, got), decodeJSON(t, []byte(fixtureDetail))) {
		t.Fatalf("snapshot lost detail fields: %s", got)
	}
	mu.Lock()
	defer mu.Unlock()
	if want := []string{"GET /public/recordings?page=1&limit=100", "GET /public/recordings/recording-1"}; !reflect.DeepEqual(requests, want) {
		t.Fatalf("requests=%v; want=%v", requests, want)
	}
	assertRedacted(t, stderr.String())
	var summary map[string]any
	for _, line := range strings.Split(strings.TrimSpace(stderr.String()), "\n") {
		event := decodeJSON(t, []byte(line)).(map[string]any)
		if event["event"] == "summary" {
			summary = event
		}
	}
	if summary["fetched"] != json.Number("1") || summary["failed"] != json.Number("0") || summary["run_id"] == "" {
		t.Fatalf("missing successful structured summary: %s", &stderr)
	}
}

func testContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func cleanEnvironment() []string {
	var result []string
	for _, value := range os.Environ() {
		if !strings.HasPrefix(value, "POCKETPULL_") {
			result = append(result, value)
		}
	}
	return result
}

func decodeJSON(t *testing.T, data []byte) any {
	t.Helper()
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var result any
	if err := decoder.Decode(&result); err != nil {
		t.Fatal(err)
	}
	return result
}

func assertRedacted(t *testing.T, logs string) {
	t.Helper()
	for _, secret := range []string{fixtureKey, "private-title-sentinel", "private-speaker-sentinel", "private-transcript-sentinel", "private-summary-sentinel", "Authorization", "Bearer", "http://"} {
		if strings.Contains(logs, secret) {
			t.Errorf("logs leaked %q: %s", secret, logs)
		}
	}
}

func invoke(t *testing.T, endpoint, raw string, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := run(testContext(t), append([]string{"--raw-dir", raw}, args...), func(name string) string {
		if name == "POCKETPULL_API_KEY" {
			return fixtureKey
		}
		return ""
	}, &stdout, &stderr, dependencies{baseURL: endpoint, client: &http.Client{Timeout: 5 * time.Second}})
	if stdout.Len() != 0 {
		t.Errorf("collection wrote stdout: %q", &stdout)
	}
	assertRedacted(t, stderr.String())
	return code, stdout.String(), stderr.String()
}

func listPage(ids []string, page int, more bool) string {
	records := make([]map[string]string, len(ids))
	for i, id := range ids {
		records[i] = map[string]string{"id": id}
	}
	data, _ := json.Marshal(records)
	return fmt.Sprintf(`{"success":true,"data":%s,"pagination":{"has_more":%t,"page":%d,"limit":100}}`, data, more, page)
}

func TestTraversalDeduplicationAndPending(t *testing.T) {
	var mu sync.Mutex
	requests := make(map[string]int)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requests[r.URL.RequestURI()]++
		mu.Unlock()
		switch r.URL.RequestURI() {
		case "/public/recordings?page=1&limit=100":
			fmt.Fprint(w, listPage([]string{"recording-1", "recording-1"}, 1, true))
		case "/public/recordings?page=2&limit=100":
			fmt.Fprint(w, listPage([]string{"recording-1", "pending"}, 2, true))
		case "/public/recordings?page=3&limit=100":
			fmt.Fprint(w, listPage([]string{}, 3, false))
		case "/public/recordings/recording-1":
			fmt.Fprintf(w, `{"success":true,"data":%s}`, fixtureDetail)
		case "/public/recordings/pending":
			fmt.Fprint(w, `{"success":true,"data":{"id":"pending","transcript":null,"future_field":true}}`)
		default:
			t.Errorf("unexpected route: %s", r.URL.RequestURI())
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	raw := t.TempDir()
	code, _, logs := invoke(t, server.URL, raw)
	if code != 0 || !strings.Contains(logs, `"discovered":2,"fetched":2,"unchanged":0,"cached":0,"pending":1,"failed":0`) {
		t.Fatalf("code=%d; logs=%s", code, logs)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(requests) != 5 {
		t.Fatalf("unexpected requests: %v", requests)
	}
	for route, count := range requests {
		if count != 1 {
			t.Errorf("duplicate fetch %s: %d", route, count)
		}
	}
	for _, id := range []string{"recording-1", "pending"} {
		if _, err := os.ReadFile(filepath.Join(raw, id, "recording.json")); err != nil {
			t.Fatal(err)
		}
	}
}

func TestChangedOnlySnapshots(t *testing.T) {
	var mu sync.Mutex
	detail := fixtureDetail
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		switch r.URL.Path {
		case "/public/recordings":
			fmt.Fprint(w, listPage([]string{"recording-1"}, 1, false))
		case "/public/recordings/recording-1":
			fmt.Fprintf(w, `{"success":true,"data":%s}`, detail)
		default:
			t.Error("unexpected route")
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	raw := t.TempDir()
	if code, _, logs := invoke(t, server.URL, raw); code != 0 {
		t.Fatal(logs)
	}
	path := filepath.Join(raw, "recording-1", "recording.json")
	old, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	stamp := time.Unix(946684800, 0)
	if err := os.Chtimes(path, stamp, stamp); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	object := decodeJSON(t, []byte(fixtureDetail)).(map[string]any)
	object["transcript"].(map[string]any)["segments"].([]any)[0].(map[string]any)["start"] = json.Number("0.0")
	object["future_field"].(map[string]any)["integer"] = json.Number("9007199254740993.0")
	reordered, err := json.Marshal(object)
	if err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	detail = string(reordered)
	mu.Unlock()
	code, _, logs := invoke(t, server.URL, raw)
	if code != 0 || !strings.Contains(logs, `"unchanged":1`) {
		t.Fatalf("code=%d; logs=%s", code, logs)
	}
	after, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(before, after) || !after.ModTime().Equal(stamp) || !bytes.Equal(old, content) {
		t.Fatal("semantically unchanged snapshot was rewritten")
	}
	mu.Lock()
	detail = strings.Replace(fixtureDetail, "private-title-sentinel", "changed-title", 1)
	mu.Unlock()
	if code, _, logs := invoke(t, server.URL, raw); code != 0 {
		t.Fatal(logs)
	}
	changed, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	content, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if os.SameFile(before, changed) || !strings.Contains(string(content), "changed-title") || !strings.Contains(string(content), "9007199254740993") {
		t.Fatal("changed snapshot was not replaced completely")
	}
	assertSnapshotFiles(t, filepath.Dir(path))
}

func assertSnapshotFiles(t *testing.T, directory string) {
	t.Helper()
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "recording.json" {
		t.Fatalf("unexpected snapshot files, including temporary residue: %v", entries)
	}
}

func TestFailuresPreservePriorSnapshots(t *testing.T) {
	for _, scenario := range []struct {
		name   string
		status int
		body   string
	}{
		{"malformed", 200, `{"success":true,"data":`},
		{"trailing JSON", 200, `{"success":true,"data":{"id":"recording-1"}} {}`},
		{"mismatched ID", 200, `{"success":true,"data":{"id":"other"}}`},
		{"missing ID", 200, `{"success":true,"data":{"title":"private-title-sentinel"}}`},
		{"null data", 200, `{"success":true,"data":null}`},
		{"array data", 200, `{"success":true,"data":[]}`},
		{"unsuccessful envelope", 200, `{"success":false,"data":{"id":"recording-1"}}`},
		{"absent", 404, "private-transcript-sentinel"},
		{"server failure", 500, fixtureKey},
		{"write failure", 200, `{"success":true,"data":{"id":"recording-1","transcript":null}}`},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			raw := t.TempDir()
			directory := filepath.Join(raw, "recording-1")
			if err := os.Mkdir(directory, 0o700); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(directory, "recording.json")
			if err := os.WriteFile(path, []byte(fixtureDetail), 0o600); err != nil {
				t.Fatal(err)
			}
			if scenario.name == "write failure" {
				if os.Geteuid() == 0 {
					t.Skip("write-permission failure requires non-root user")
				}
				if err := os.Chmod(directory, 0o500); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { os.Chmod(directory, 0o700) })
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/public/recordings" {
					fmt.Fprint(w, listPage([]string{"recording-1"}, 1, false))
				} else {
					w.WriteHeader(scenario.status)
					fmt.Fprint(w, scenario.body)
				}
			}))
			defer server.Close()
			code, _, logs := invoke(t, server.URL, raw)
			if code != 1 || !strings.Contains(logs, `"fetched":0`) || !strings.Contains(logs, `"failed":1`) {
				t.Fatalf("code=%d; logs=%s", code, logs)
			}
			got, err := os.ReadFile(path)
			if err != nil || string(got) != fixtureDetail {
				t.Fatalf("prior snapshot changed: err=%v, content=%s", err, got)
			}
			assertSnapshotFiles(t, directory)
		})
	}
}

func TestRemoteAbsenceNeverPrunes(t *testing.T) {
	raw := t.TempDir()
	directory := filepath.Join(raw, "recording-1")
	if err := os.Mkdir(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, "recording.json")
	if err := os.WriteFile(path, []byte(fixtureDetail), 0o600); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RequestURI() != "/public/recordings?page=1&limit=100" {
			t.Error("unexpected detail fetch")
		}
		fmt.Fprint(w, listPage([]string{}, 1, false))
	}))
	defer server.Close()
	if code, _, logs := invoke(t, server.URL, raw); code != 0 {
		t.Fatal(logs)
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != fixtureDetail {
		t.Fatal("absent recording was pruned or changed")
	}
}

func TestUnsafeIDs(t *testing.T) {
	for _, id := range []string{"", ".", "..", "../escape", "/absolute", "a/b", `a\b`, "a?audio", "a\nb", strings.Repeat("a", 256)} {
		t.Run(fmt.Sprintf("%q", id), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/public/recordings" {
					t.Error("unsafe ID was fetched")
				}
				fmt.Fprint(w, listPage([]string{id}, 1, false))
			}))
			defer server.Close()
			raw := t.TempDir()
			if code, _, logs := invoke(t, server.URL, raw); code != 1 {
				t.Fatal(logs)
			}
			entries, err := os.ReadDir(raw)
			if err != nil || len(entries) != 0 {
				t.Fatal("unsafe ID wrote a snapshot")
			}
		})
	}
}

func TestPartialProgressAndAuthStop(t *testing.T) {
	for _, status := range []int{http.StatusNotFound, http.StatusUnauthorized, http.StatusForbidden} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			var mu sync.Mutex
			var fetched []string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/public/recordings" {
					fmt.Fprint(w, listPage([]string{"good", "bad", "later"}, 1, false))
					return
				}
				id := strings.TrimPrefix(r.URL.Path, "/public/recordings/")
				mu.Lock()
				fetched = append(fetched, id)
				mu.Unlock()
				if id == "bad" {
					w.WriteHeader(status)
					fmt.Fprint(w, fixtureKey+" private-transcript-sentinel")
					return
				}
				fmt.Fprintf(w, `{"success":true,"data":{"id":%q}}`, id)
			}))
			defer server.Close()
			raw := t.TempDir()
			code, _, logs := invoke(t, server.URL, raw)
			if code != 1 || !strings.Contains(logs, `"failed":1`) {
				t.Fatalf("code=%d; logs=%s", code, logs)
			}
			if _, err := os.ReadFile(filepath.Join(raw, "good", "recording.json")); err != nil {
				t.Fatal("successful partial work missing")
			}
			mu.Lock()
			defer mu.Unlock()
			want := []string{"good", "bad"}
			if status == http.StatusNotFound {
				want = append(want, "later")
			}
			if !reflect.DeepEqual(fetched, want) {
				t.Fatalf("fetched=%v; want=%v", fetched, want)
			}
		})
	}
}

func TestHelpVersionAndInvalidConfiguration(t *testing.T) {
	for _, args := range [][]string{
		{"--help"}, {"--version"}, {"-h"}, {}, {"--raw-dir", "somewhere"},
		{"--api-url", "http://fixture"}, {"--api-key", fixtureKey}, {"--raw-dir", "somewhere", fixtureKey},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := run(testContext(t), args, func(string) string { return "" }, &stdout, &stderr, dependencies{})
			informational := len(args) == 1 && (args[0] == "--help" || args[0] == "-h" || args[0] == "--version")
			if informational {
				if code != 0 || stdout.Len() == 0 || stderr.Len() != 0 {
					t.Fatalf("help/version collected or failed: code=%d; stdout=%s; stderr=%s", code, &stdout, &stderr)
				}
			} else if code != 1 || stdout.Len() != 0 {
				t.Fatalf("invalid configuration accepted: code=%d; stdout=%s", code, &stdout)
			}
			assertRedacted(t, stderr.String())
		})
	}
}

type failingTransport struct{}

func (failingTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("http://user:" + fixtureKey + "@fixture/ private-transcript-sentinel")
}

func TestTransportErrorsAreRedacted(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run(testContext(t), []string{"--raw-dir", t.TempDir()}, func(string) string {
		return fixtureKey
	}, &stdout, &stderr, dependencies{
		baseURL: "http://fixture", client: &http.Client{Transport: failingTransport{}},
	})
	if code != 1 || stdout.Len() != 0 || !strings.Contains(stderr.String(), `"reason":"transport failed"`) ||
		!strings.Contains(stderr.String(), `"failed":1`) {
		t.Fatalf("missing sanitized failure summary: code=%d; logs=%s", code, &stderr)
	}
	assertRedacted(t, stderr.String())
}

func TestInvalidListingsStopWithoutWriting(t *testing.T) {
	for _, body := range []string{
		`{`,
		`{"success":false,"data":[],"pagination":{"page":1,"has_more":false}}`,
		`{"success":true,"data":null,"pagination":{"page":1,"has_more":false}}`,
		`{"success":true,"data":[],"pagination":{"page":1}}`,
		`{"success":true,"data":[],"pagination":{"page":2,"has_more":true}}`,
	} {
		t.Run(body, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.RequestURI() != "/public/recordings?page=1&limit=100" {
					t.Error("continued after invalid listing")
				}
				fmt.Fprint(w, body)
			}))
			defer server.Close()
			raw := t.TempDir()
			code, _, logs := invoke(t, server.URL, raw)
			if code != 1 || !strings.Contains(logs, `"failed":1`) {
				t.Fatalf("invalid listing accepted: code=%d; logs=%s", code, logs)
			}
			entries, err := os.ReadDir(raw)
			if err != nil || len(entries) != 0 {
				t.Fatal("invalid listing caused snapshot writes")
			}
		})
	}
}

func TestRedirectDoesNotRequestAudio(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/public/recordings":
			fmt.Fprint(w, listPage([]string{"recording-1"}, 1, false))
		case "/public/recordings/recording-1":
			http.Redirect(w, r, "/public/recordings/recording-1/audio", http.StatusFound)
		default:
			t.Error("followed redirect to unexpected content route")
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	code, _, logs := invoke(t, server.URL, t.TempDir())
	if code != 1 || !strings.Contains(logs, `"reason":"http status 302"`) {
		t.Fatalf("redirect did not fail safely: code=%d; logs=%s", code, logs)
	}
}
