package cli

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
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

func TestConfigSecretFileCollection(t *testing.T) {
	dir := t.TempDir()
	writeFixtureFile(t, filepath.Join(dir, "api-key"), fixtureKey+"\n", 0o600)
	config := filepath.Join(dir, "config.yaml")
	writeFixtureFile(t, config, "raw_dir: raw\napi_key_file: api-key\nlog_format: text\nlog_level: debug\nrefresh_after: \"0\"\nstate_dir: state\n", 0o600)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+fixtureKey {
			t.Error("secret-file authentication missing")
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.URL.Path == "/public/recordings" {
			fmt.Fprint(w, listPage([]string{"recording-1"}, 1, false))
		} else {
			fmt.Fprintf(w, `{"success":true,"data":%s}`, fixtureDetail)
		}
	}))
	defer server.Close()
	var stdout, stderr bytes.Buffer
	code := run(context.Background(), []string{"--config", config}, func(string) (string, bool) { return "", false },
		&stdout, &stderr, dependencies{baseURL: server.URL, client: server.Client()})
	if code != 0 || stdout.Len() != 0 {
		t.Fatalf("configuration collection failed: code=%d, stdout=%s, stderr=%s", code, &stdout, &stderr)
	}
	if _, err := os.ReadFile(filepath.Join(dir, "raw", "recording-1", "recording.json")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "state")); !os.IsNotExist(err) {
		t.Fatalf("T2 unexpectedly created operational state: %v", err)
	}
	assertRedacted(t, stderr.String())
}

func writeFixtureFile(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
}

func fixtureEnvironment(values map[string]string) environment {
	return func(name string) (string, bool) {
		value, present := values[name]
		return value, present
	}
}

func TestConfigSelection(t *testing.T) {
	for _, scenario := range []string{"explicit", "cwd", "xdg", "home fallback", "no merge", "invalid cwd", "invalid xdg", "unreadable cwd", "explicit unreadable", "dangling cwd", "explicit missing"} {
		t.Run(scenario, func(t *testing.T) {
			dir := t.TempDir()
			t.Chdir(dir)
			env := map[string]string{"HOME": filepath.Join(dir, "home"), "XDG_CONFIG_HOME": filepath.Join(dir, "xdg"), "POCKETPULL_API_KEY": fixtureKey}
			xdg := filepath.Join(env["XDG_CONFIG_HOME"], "pocketpull", "config.yaml")
			cwd := filepath.Join(dir, "pocketpull.yaml")
			writeFixtureFile(t, xdg, "raw_dir: xdg-raw\n", 0o600)
			var args []string
			want := filepath.Join(filepath.Dir(xdg), "xdg-raw")
			wantError := false
			switch scenario {
			case "explicit":
				writeFixtureFile(t, cwd, "unknown: invalid\n", 0o600)
				writeFixtureFile(t, "selected/config.yaml", "raw_dir: selected-raw\n", 0o600)
				args = []string{"--config", "selected/config.yaml"}
				want = filepath.Join(dir, "selected", "selected-raw")
			case "cwd":
				writeFixtureFile(t, cwd, "raw_dir: cwd-raw\n", 0o600)
				writeFixtureFile(t, xdg, "unknown: invalid\n", 0o600)
				want = filepath.Join(dir, "cwd-raw")
			case "home fallback":
				delete(env, "XDG_CONFIG_HOME")
				homeConfig := filepath.Join(env["HOME"], ".config", "pocketpull", "config.yaml")
				writeFixtureFile(t, homeConfig, "raw_dir: home-raw\n", 0o600)
				want = filepath.Join(filepath.Dir(homeConfig), "home-raw")
			case "no merge":
				writeFixtureFile(t, cwd, "log_format: text\n", 0o600)
				wantError = true
			case "invalid cwd":
				writeFixtureFile(t, cwd, "unknown: invalid\n", 0o600)
				wantError = true
			case "invalid xdg":
				writeFixtureFile(t, xdg, "unknown: invalid\n", 0o600)
				wantError = true
			case "unreadable cwd", "explicit unreadable":
				if os.Geteuid() == 0 {
					t.Skip("permission check requires non-root")
				}
				writeFixtureFile(t, cwd, "raw_dir: cwd-raw\n", 0o000)
				if scenario == "explicit unreadable" {
					args = []string{"--config", cwd}
				}
				wantError = true
			case "dangling cwd":
				if err := os.Symlink("missing", cwd); err != nil {
					t.Fatal(err)
				}
				wantError = true
			case "explicit missing":
				args = []string{"--config", "missing.yaml"}
				wantError = true
			}
			config, _, err := configure(args, fixtureEnvironment(env))
			if wantError {
				if err == nil {
					t.Fatal("unusable selected configuration silently fell back")
				}
			} else if err != nil || config.rawDir != want {
				t.Fatalf("selection: raw=%q want=%q error=%v", config.rawDir, want, err)
			}
		})
	}
}

func TestConfigPathsPrecedenceAndDefaults(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeFixtureFile(t, "settings/key", fixtureKey, 0o600)
	writeFixtureFile(t, "cli-key", fixtureKey, 0o600)
	writeFixtureFile(t, "settings/config.yaml", "raw_dir: raw\napi_key_file: key\nstate_dir: state\nrefresh_after: 12h\nlog_format: text\nlog_level: warn\n", 0o600)
	env := fixtureEnvironment(map[string]string{"HOME": filepath.Join(dir, "home")})
	config, _, err := configure([]string{"--config", "settings/config.yaml"}, env)
	if err != nil || config.rawDir != filepath.Join(dir, "settings", "raw") ||
		config.stateDir != filepath.Join(dir, "settings", "state") ||
		config.apiKeyFile != filepath.Join(dir, "settings", "key") || config.refreshAfter != 12*time.Hour ||
		config.logFormat != "text" || config.logLevel != slog.LevelWarn {
		t.Fatalf("file paths/values: %+v error=%v", configWithoutSecret(config), err)
	}
	config, _, err = configure([]string{"--config", "settings/config.yaml", "--raw-dir", "cli-raw",
		"--api-key-file", "cli-key", "--state-dir", "cli-state", "--refresh-after", "0",
		"--log-format", "json", "--log-level", "debug"}, env)
	if err != nil || config.rawDir != filepath.Join(dir, "cli-raw") || config.stateDir != filepath.Join(dir, "cli-state") ||
		config.apiKeyFile != filepath.Join(dir, "cli-key") || config.refreshAfter != 0 ||
		config.logFormat != "json" || config.logLevel != slog.LevelDebug {
		t.Fatalf("CLI overrides: %+v error=%v", configWithoutSecret(config), err)
	}
	for _, xdg := range []bool{false, true} {
		values := map[string]string{"HOME": filepath.Join(dir, "home"), "POCKETPULL_API_KEY": fixtureKey}
		wantState := filepath.Join(values["HOME"], ".local", "state", "pocketpull")
		if xdg {
			values["XDG_STATE_HOME"] = filepath.Join(dir, "xdg-state")
			wantState = filepath.Join(values["XDG_STATE_HOME"], "pocketpull")
		}
		config, _, err = configure([]string{"--raw-dir", "raw"}, fixtureEnvironment(values))
		if err != nil || config.rawDir != filepath.Join(dir, "raw") || config.stateDir != wantState ||
			config.refreshAfter != 24*time.Hour || config.logFormat != "json" || config.logLevel != slog.LevelInfo {
			t.Fatalf("flags/defaults: %+v error=%v", configWithoutSecret(config), err)
		}
	}
}

func configWithoutSecret(config configuration) configuration {
	config.apiKey = ""
	return config
}

func TestStrictConfigAndArgumentErrorsAreRedacted(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	env := fixtureEnvironment(map[string]string{"HOME": dir, "POCKETPULL_API_KEY": fixtureKey})
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		t.Error("invalid configuration reached HTTP")
	}))
	defer server.Close()
	for _, input := range []struct {
		name string
		yaml string
		args []string
	}{
		{name: "malformed", yaml: "raw_dir: [" + fixtureKey},
		{name: "unknown", yaml: "raw_dir: raw\n" + fixtureKey + ": private-transcript-sentinel\n"},
		{name: "literal key", yaml: "raw_dir: raw\napi_key: " + fixtureKey + "\n"},
		{name: "duplicate", yaml: "raw_dir: raw\nraw_dir: " + fixtureKey + "\n"},
		{name: "multiple docs", yaml: "raw_dir: raw\n---\nraw_dir: " + fixtureKey + "\n"},
		{name: "empty second doc", yaml: "raw_dir: raw\n---\n"},
		{name: "sequence", yaml: "- " + fixtureKey + "\n"},
		{name: "empty document", yaml: ""},
		{name: "null", yaml: "raw_dir: null\n"},
		{name: "boolean", yaml: "raw_dir: true\n"},
		{name: "integer path", yaml: "raw_dir: 123\n"},
		{name: "mapping value", yaml: "raw_dir: {key: " + fixtureKey + "}\n"},
		{name: "sequence value", yaml: "raw_dir: [" + fixtureKey + "]\n"},
		{name: "empty value", yaml: "raw_dir: \"\"\n"},
		{name: "whitespace value", yaml: "raw_dir: \"  \"\n"},
		{name: "unknown tag", yaml: "raw_dir: !secret " + fixtureKey + "\n"},
		{name: "alias", yaml: "raw_dir: &path raw\nstate_dir: *path\n"},
		{name: "missing raw", yaml: "log_format: text\n"},
		{name: "invalid duration", yaml: "raw_dir: raw\nrefresh_after: " + fixtureKey + "\n"},
		{name: "negative duration", yaml: "raw_dir: raw\nrefresh_after: -1s\n"},
		{name: "numeric duration", yaml: "raw_dir: raw\nrefresh_after: 0\n"},
		{name: "bad format", yaml: "raw_dir: raw\nlog_format: " + fixtureKey + "\n"},
		{name: "bad level", yaml: "raw_dir: raw\nlog_level: " + fixtureKey + "\n"},
		{name: "invalid overridden file", yaml: "raw_dir: raw\nrefresh_after: bad\n", args: []string{"--refresh-after", "0"}},
		{name: "missing explicit", args: []string{"--config", filepath.Join(dir, fixtureKey)}},
		{name: "unknown flag", args: []string{"--" + fixtureKey + "=private-transcript-sentinel"}},
		{name: "literal key flag", args: []string{"--api-key", fixtureKey}},
		{name: "endpoint flag", args: []string{"--api-url", "http://user:" + fixtureKey + "@fixture"}},
		{name: "positional", args: []string{fixtureKey}},
		{name: "empty raw flag", args: []string{"--raw-dir="}},
		{name: "empty state flag", args: []string{"--state-dir="}},
		{name: "empty secret path flag", args: []string{"--api-key-file="}},
		{name: "invalid state path", args: []string{"--state-dir", "bad\x00" + fixtureKey}},
		{name: "empty config flag", args: []string{"--config="}},
		{name: "bad duration flag", args: []string{"--refresh-after", fixtureKey}},
		{name: "negative duration flag", args: []string{"--refresh-after=-1s"}},
		{name: "bad format flag", args: []string{"--log-format", fixtureKey}},
		{name: "bad level flag", args: []string{"--log-level", fixtureKey}},
		{name: "bad help flag", args: []string{"--help=" + fixtureKey}},
	} {
		t.Run(input.name, func(t *testing.T) {
			args := []string{"--raw-dir", "raw", "--log-level", "debug"}
			if input.yaml != "" || input.name == "empty document" {
				path := filepath.Join(t.TempDir(), fixtureKey+".yaml")
				writeFixtureFile(t, path, input.yaml, 0o600)
				args = []string{"--config", path}
				// Missing raw must fail without the flags supplying it.
			}
			args = append(args, input.args...)
			var stdout, stderr bytes.Buffer
			code := run(testContext(t), args, env, &stdout, &stderr, dependencies{baseURL: server.URL, client: server.Client()})
			if code != 1 || stdout.Len() != 0 || !strings.Contains(stderr.String(), `"event":"configuration_failed"`) {
				t.Fatalf("invalid input accepted: code=%d stdout=%s stderr=%s", code, &stdout, &stderr)
			}
			assertRedacted(t, stderr.String())
			decodeJSON(t, bytes.TrimSpace(stderr.Bytes()))
		})
	}
	if requests.Load() != 0 {
		t.Fatal("HTTP before validation")
	}
}

func TestSecretSourcesAndPermissions(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	for _, scenario := range []struct {
		name    string
		env     map[string]string
		file    bool
		content string
		mode    os.FileMode
		valid   bool
	}{
		{"environment", map[string]string{"POCKETPULL_API_KEY": fixtureKey}, false, "", 0, true},
		{"trim environment", map[string]string{"POCKETPULL_API_KEY": " " + fixtureKey + "\n"}, false, "", 0, true},
		{"absent", nil, false, "", 0, false},
		{"empty environment", map[string]string{"POCKETPULL_API_KEY": ""}, false, "", 0, false},
		{"whitespace environment", map[string]string{"POCKETPULL_API_KEY": " \t\n"}, false, "", 0, false},
		{"multiline environment", map[string]string{"POCKETPULL_API_KEY": fixtureKey + "\nother"}, false, "", 0, false},
		{"secret file", nil, true, fixtureKey + "\n", 0o600, true},
		{"read only file", nil, true, fixtureKey, 0o400, true},
		{"empty file", nil, true, "", 0o600, false},
		{"whitespace file", nil, true, " \n\t", 0o600, false},
		{"conflict same key", map[string]string{"POCKETPULL_API_KEY": fixtureKey}, true, fixtureKey, 0o600, false},
		{"conflict empty env", map[string]string{"POCKETPULL_API_KEY": ""}, true, fixtureKey, 0o600, false},
		{"conflict whitespace env", map[string]string{"POCKETPULL_API_KEY": " "}, true, fixtureKey, 0o600, false},
		{"group readable", nil, true, fixtureKey, 0o640, false},
		{"other readable", nil, true, fixtureKey, 0o604, false},
		{"unreadable file", nil, true, fixtureKey, 0o000, false},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			if scenario.name == "unreadable file" && os.Geteuid() == 0 {
				t.Skip("permission check requires non-root")
			}
			args := []string{"--raw-dir", "raw", "--state-dir", "state"}
			if scenario.file {
				path := filepath.Join(t.TempDir(), "key")
				writeFixtureFile(t, path, scenario.content, scenario.mode)
				args = append(args, "--api-key-file", path)
			}
			config, _, err := configure(args, fixtureEnvironment(scenario.env))
			if (err == nil) != scenario.valid {
				t.Fatalf("credential validation: valid=%t error=%v", scenario.valid, err)
			}
			if scenario.valid && config.apiKey != fixtureKey {
				t.Fatal("credential not loaded/trimmed correctly")
			}
		})
	}
	for _, kind := range []string{"missing", "directory", "fifo"} {
		t.Run(kind, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "key")
			switch kind {
			case "directory":
				if err := os.Mkdir(path, 0o700); err != nil {
					t.Fatal(err)
				}
			case "fifo":
				if err := syscall.Mkfifo(path, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			_, _, err := configure([]string{"--raw-dir", "raw", "--state-dir", "state", "--api-key-file", path}, fixtureEnvironment(nil))
			if err == nil {
				t.Fatal("unusable selected secret fell back or was accepted")
			}
		})
	}
	path := filepath.Join(dir, "owner-test")
	writeFixtureFile(t, path, fixtureKey, 0o600)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if validateSecretInfo(info, os.Geteuid()) != nil || validateSecretInfo(info, os.Geteuid()+1) == nil {
		t.Fatal("effective uid ownership validation failed")
	}
}

func TestNoDotEnvAndInformationalBypass(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeFixtureFile(t, ".env", "POCKETPULL_API_KEY="+fixtureKey+"\n", 0o600)
	var stdout, stderr bytes.Buffer
	code := run(testContext(t), []string{"--raw-dir", "raw", "--state-dir", "state"}, fixtureEnvironment(nil),
		&stdout, &stderr, dependencies{})
	if code != 1 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "one credential source required") {
		t.Fatalf(".env was loaded: code=%d stderr=%s", code, &stderr)
	}
	writeFixtureFile(t, "pocketpull.yaml", "malformed: ["+fixtureKey, 0o600)
	for _, args := range [][]string{
		{"--help"}, {"-h"}, {"--version"},
		{"--config", fixtureKey, "--api-key-file", fixtureKey, "--help"},
		{"--config", fixtureKey, "--api-key-file", fixtureKey, "--version"},
	} {
		stdout.Reset()
		stderr.Reset()
		code := run(testContext(t), args, func(string) (string, bool) {
			t.Fatal("help/version read environment")
			return "", false
		}, &stdout, &stderr, dependencies{})
		if code != 0 || stdout.Len() == 0 || stderr.Len() != 0 {
			t.Fatalf("help/version did work: code=%d stdout=%s stderr=%s", code, &stdout, &stderr)
		}
	}
}

func TestLogFormatsLevelsAndPartialSummary(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	for _, format := range []string{"json", "text"} {
		for _, level := range []string{"debug", "info", "warn", "error"} {
			for _, failure := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/failure=%t", format, level, failure), func(t *testing.T) {
					server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						if r.URL.Path == "/public/recordings" {
							ids := []string{fixtureKey}
							if failure {
								ids = append(ids, "bad")
							}
							fmt.Fprint(w, listPage(ids, 1, false))
						} else if strings.HasSuffix(r.URL.Path, "/bad") {
							w.WriteHeader(http.StatusNotFound)
							fmt.Fprint(w, fixtureKey+" Authorization: Bearer private-transcript-sentinel")
						} else {
							fmt.Fprintf(w, `{"success":true,"data":{"id":%q,"title":"private-title-sentinel","transcript":{"text":"private-transcript-sentinel"},"summary":"private-summary-sentinel"}}`, fixtureKey)
						}
					}))
					defer server.Close()
					code, _, logs := invoke(t, server.URL, t.TempDir(), "--log-format", format, "--log-level", level)
					if (code == 1) != failure {
						t.Fatalf("unexpected outcome: code=%d logs=%s", code, logs)
					}
					if strings.Contains(logs, "page_requested") != (level == "debug") {
						t.Fatalf("debug filtering failed: %s", logs)
					}
					if !failure && (level == "warn" || level == "error") {
						if logs != "" {
							t.Fatalf("info events not filtered: %s", logs)
						}
						return
					}
					if !strings.Contains(logs, "summary") || !strings.Contains(logs, "[redacted]") && (level == "info" || level == "debug") {
						t.Fatalf("missing redacted operational events/summary: %s", logs)
					}
					if format == "json" {
						for _, line := range strings.Split(strings.TrimSpace(logs), "\n") {
							event := decodeJSON(t, []byte(line)).(map[string]any)
							if event["run_id"] == "" {
								t.Fatal("event missing run context")
							}
							if event["event"] == "summary" && (event["fetched"] != json.Number("1") || event["failed"] != json.Number(fmt.Sprint(boolCount(failure)))) {
								t.Fatalf("partial summary lost counts: %s", line)
							}
						}
					} else if !strings.Contains(logs, "event=summary") || !strings.Contains(logs, "fetched=1") ||
						!strings.Contains(logs, fmt.Sprintf("failed=%d", boolCount(failure))) {
						t.Fatalf("missing text summary counts: %s", logs)
					}
				})
			}
		}
	}
}

func boolCount(value bool) int {
	if value {
		return 1
	}
	return 0
}

func TestDebugTransportAndPathErrorsAreRedacted(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	for _, format := range []string{"json", "text"} {
		for _, failure := range []string{"transport", "secret path", "raw path"} {
			t.Run(format+"/"+failure, func(t *testing.T) {
				env := fixtureEnvironment(map[string]string{"POCKETPULL_API_KEY": fixtureKey})
				args := []string{"--raw-dir", "raw", "--state-dir", "state", "--log-format", format, "--log-level", "debug"}
				switch failure {
				case "secret path":
					env = fixtureEnvironment(nil)
					args = append(args, "--api-key-file", filepath.Join(dir, fixtureKey+"-private-transcript-sentinel"))
				case "raw path":
					path := filepath.Join(t.TempDir(), fixtureKey+"-private-transcript-sentinel")
					writeFixtureFile(t, path, "not a directory", 0o600)
					args = append(args, "--raw-dir", path)
				}

				var stdout, stderr bytes.Buffer
				code := run(testContext(t), args, env, &stdout, &stderr, dependencies{
					baseURL: "http://fixture", client: &http.Client{Transport: failingTransport{}},
				})
				if code != 1 || stdout.Len() != 0 || stderr.Len() == 0 {
					t.Fatalf("failure not reported: code=%d stdout=%s stderr=%s", code, &stdout, &stderr)
				}
				assertRedacted(t, stderr.String())
			})
		}
	}
}

func TestDocumentedConfigurationExample(t *testing.T) {
	example, err := os.ReadFile("../../examples/pocketpull.yaml")
	if err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	dir := filepath.Join(home, ".config", "pocketpull")
	path := filepath.Join(dir, "config.yaml")
	writeFixtureFile(t, path, string(example), 0o600)
	writeFixtureFile(t, filepath.Join(dir, "api-key"), fixtureKey, 0o600)
	t.Chdir(t.TempDir())
	config, _, err := configure([]string{"--config", path}, fixtureEnvironment(map[string]string{"HOME": home}))
	if err != nil || config.rawDir != filepath.Join(home, "pocket-raw") ||
		config.stateDir != filepath.Join(home, ".local", "state", "pocketpull") ||
		config.apiKey != fixtureKey || config.refreshAfter != 24*time.Hour {
		t.Fatalf("documented example unusable: %+v error=%v", configWithoutSecret(config), err)
	}
}
