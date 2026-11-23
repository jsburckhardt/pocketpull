set shell := ["bash", "-eu", "-o", "pipefail", "-c"]

# Host or container: validate workspace configuration and shell syntax.
dev-check:
    python3 .devcontainer/test_workspace.py

# Inside the devcontainer: exercise tools, user, mounts and shell history.
dev-smoke:
    bash .devcontainer/dev-smoke.sh

# Host: boot an isolated devcontainer with disposable SSH keys; clean up on exit.
dev-signing-smoke:
    python3 .devcontainer/signing-smoke.py

# Inside the devcontainer: real authenticated CLI emission to an ephemeral loopback receiver.
dev-telemetry-smoke:
    python3 .devcontainer/telemetry-smoke.py

# Collector: reject unformatted Go, vet, race-test and build native Linux targets.
collector-check:
    #!/usr/bin/env bash
    set -euo pipefail
    unformatted="$(gofmt -l cmd internal)"
    if [[ -n "$unformatted" ]]; then
        printf 'Unformatted Go files:\n%s\n' "$unformatted" >&2
        exit 1
    fi
    go vet ./...
    go test -race ./...
    build_dir="$(mktemp -d)"
    trap 'rm -f -- "$build_dir/pocketpull-amd64" "$build_dir/pocketpull-arm64"; rmdir -- "$build_dir"' EXIT
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o "$build_dir/pocketpull-amd64" ./cmd/pocketpull
    CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o "$build_dir/pocketpull-arm64" ./cmd/pocketpull

# Collector: native subprocess, one-recording loopback HTTP fixture, no real key.
collector-smoke:
    go test ./internal/cli -run '^TestNativeSmoke$' -count=1
