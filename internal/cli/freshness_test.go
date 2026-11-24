package cli

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestRepeatedCollectionUsesFreshness(t *testing.T) {
	var details atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/public/recordings" {
			fmt.Fprint(w, listPage([]string{"recording-1"}, 1, false))
			return
		}
		details.Add(1)
		fmt.Fprintf(w, `{"success":true,"data":%s}`, fixtureDetail)
	}))
	defer server.Close()
	raw, state := t.TempDir(), t.TempDir()
	for i := 0; i < 2; i++ {
		code, _, logs := invoke(t, server.URL, raw, "--state-dir", state)
		if code != 0 {
			t.Fatal(logs)
		}
		if i == 1 && !strings.Contains(logs, `"cached":1`) {
			t.Fatalf("repeat collection did not use successful-fetch freshness: %s", logs)
		}
	}
	if details.Load() != 1 {
		t.Fatalf("fresh recording fetched %d times", details.Load())
	}
}

func TestNativeOverlapAndKillRestart(t *testing.T) {
	var requests atomic.Int32
	var blocked atomic.Bool
	stalled := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		switch r.URL.Path {
		case "/public/recordings":
			fmt.Fprint(w, listPage([]string{"good", "later"}, 1, false))
		case "/public/recordings/good":
			fmt.Fprint(w, `{"success":true,"data":{"id":"good","future":9007199254740993}}`)
		case "/public/recordings/later":
			if blocked.CompareAndSwap(false, true) {
				close(stalled)
				<-r.Context().Done()
				return
			}
			fmt.Fprint(w, `{"success":true,"data":{"id":"later","transcript":null}}`)
		default:
			t.Error("unexpected fixture route")
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	raw, state, home := t.TempDir(), t.TempDir(), t.TempDir()
	process := func(raw, state string) (*exec.Cmd, *bytes.Buffer, *bytes.Buffer) {
		t.Helper()
		command := exec.CommandContext(testContext(t), os.Args[0],
			"-test.run=^TestCLIProcess$", "--", "--raw-dir", raw, "--state-dir", state, "--log-level", "debug")
		command.Dir = home
		command.Env = append(cleanEnvironment(), "HOME="+home, "POCKETPULL_API_KEY="+fixtureKey,
			"POCKETPULL_TEST_PROCESS=1", "POCKETPULL_TEST_ENDPOINT="+server.URL)
		stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
		command.Stdout, command.Stderr = stdout, stderr
		return command, stdout, stderr
	}
	first, firstOut, firstErr := process(raw, state)
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
		t.Fatal("native child did not reach blocked request")
	}
	prior, err := os.ReadFile(filepath.Join(raw, "good", "recording.json"))
	if err != nil {
		t.Fatal("first child's committed progress missing")
	}
	alias := filepath.Join(t.TempDir(), "raw-alias")
	if err := os.Symlink(raw, alias); err != nil {
		t.Fatal(err)
	}
	for _, variant := range []struct {
		raw, state string
	}{
		{raw, state},
		{raw, t.TempDir()},
		{alias, t.TempDir()},
	} {
		second, stdout, stderr := process(variant.raw, variant.state)
		before := requests.Load()
		if err := second.Run(); err == nil {
			t.Fatal("overlapping process unexpectedly succeeded")
		}
		if requests.Load() != before || stdout.Len() != 0 || !strings.Contains(stderr.String(), "collection already running") {
			t.Fatalf("overlap waited, made requests or lacked failure: %s", stderr)
		}
		assertRedacted(t, stderr.String())
	}
	unrelated, stdout, stderr := process(t.TempDir(), state)
	if err := unrelated.Run(); err != nil || stdout.Len() != 0 {
		t.Fatalf("unrelated raw blocked: %v; %s", err, stderr)
	}
	assertRedacted(t, stderr.String())
	// Kill only this test's exact child PID, not a worker or process group.
	if err := first.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	if err := first.Wait(); err == nil {
		t.Fatal("killed child unexpectedly succeeded")
	}
	waited = true
	if firstOut.Len() != 0 {
		t.Fatal("interrupted collection wrote stdout")
	}
	assertRedacted(t, firstErr.String())
	restart, stdout, stderr := process(alias, state)
	if err := restart.Run(); err != nil || stdout.Len() != 0 {
		t.Fatalf("restart failed after SIGKILL: %v; %s", err, stderr)
	}
	if !strings.Contains(stderr.String(), `"fetched":1,"unchanged":0,"cached":1,"pending":1,"failed":0`) {
		t.Fatalf("restart lost committed freshness or skipped unfinished work: %s", stderr)
	}
	assertRedacted(t, stderr.String())
	current, err := os.ReadFile(filepath.Join(raw, "good", "recording.json"))
	if err != nil || !bytes.Equal(prior, current) {
		t.Fatal("interruption/restart changed committed snapshot")
	}
	if _, err := os.ReadFile(filepath.Join(raw, "later", "recording.json")); err != nil {
		t.Fatal("restart did not finish pending work")
	}
	entries, err := os.ReadDir(raw)
	if err != nil || len(entries) != 2 {
		t.Fatal("raw contains cache or lock markers")
	}
	for _, id := range []string{"good", "later"} {
		assertSnapshotFiles(t, filepath.Join(raw, id))
	}
	normalRestart, stdout, stderr := process(raw, state)
	if err := normalRestart.Run(); err != nil || stdout.Len() != 0 || !strings.Contains(stderr.String(), `"cached":2`) {
		t.Fatalf("normal exit left a stale lock or lost freshness: %v; %s", err, stderr)
	}
	assertRedacted(t, stderr.String())
}
