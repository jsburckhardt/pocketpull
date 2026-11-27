# Pinned devcontainer with a read-only SSH mount for signing

- Status: accepted
- Date: 20261123
- Source: `docs/work/20261123-dev-environment.md` D12, D13

## Context
Development and agent sessions need consistent, verified tooling (Go, just, gh, Copilot CLI, Herdr), and the owner's workflow requires SSH-signed commits from inside the container.

## Decision
Provide a non-root Ubuntu devcontainer based on soft-factory patterns with checksum-pinned downloads and a locked feature set. Bind-mount only the host `~/.ssh`, read-only, for commit signing; persist only `gh`, Copilot and shell-history state in named volumes.

## Consequences
- Easier: identical tooling for humans and agents; signing works without copying keys.
- Harder: container processes can still read private keys; this is not a hardened sandbox.
- Forbidden: Docker socket, privileged mode, whole-home or production-data mounts, unrelated Azure tooling.

## Alternatives considered
- Whole-home mount: rejected; only the SSH directory is needed.
