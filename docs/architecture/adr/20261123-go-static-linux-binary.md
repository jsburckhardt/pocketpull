# Go, CGO-free static Linux binary for amd64 and arm64

- Status: accepted
- Date: 20261123
- Source: `docs/work/20261123-collector.md` D4

## Context
The collector should be simple to distribute to a single Linux host with local persistent storage and run without installing a language runtime there.

## Decision
Implement the collector in Go and build CGO-free, statically linked Linux amd64 and arm64 binaries. Target one Pocket account and local storage.

## Consequences
- Easier: one file to install; no Go, Python or Node needed on the collection host.
- Harder: Windows/macOS have no release guarantee; multiple accounts and cloud storage need new decisions.
- Constrains tooling: new runtimes need owner approval (Python scripts and the Node release-policy check are already approved).

## Alternatives considered
- Rust: viable but not required.
