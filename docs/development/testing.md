# Testing

## Collector checks

```sh
just collector-check
just collector-smoke
```

These use synthetic loopback HTTP fixtures and temporary raw/state directories only.
`collector-check` rejects unformatted Go (it does not silently format), runs
vet/race tests, and builds CGO-free Linux amd64 and arm64 binaries with no
installed application runtime requirement. `collector-smoke` runs a native
subprocess through the real CLI entry using an internal, test-only fixture seam,
covering environment/flags and config-file/secret-file with CLI overrides;
neither command contacts Pocket or needs real credentials.

## Distribution and release checks
See [Checks and CI](ci-cd.md#checks-and-ci) for `just distribution-check`, `just release-policy-check` and `just workflow-lint`.

## Workspace checks without real credentials

Run `just dev-check` on the host (Python 3 and just required) or in the container
to validate the configuration boundary, feature lock, shell syntax and recipes.
Inside a running devcontainer, `just dev-smoke` exercises the non-root user,
pinned tool binaries, read-only SSH mount, writable auth/history volumes,
persistent shell-history setup and absence of a Docker socket or Herdr server.
These are workspace checks, not placeholder collector/application checks.

For the full disposable signing check, run **on the host**:

```sh
just dev-signing-smoke
```

This creates a temporary host HOME/`.ssh` and an unencrypted **test-only**
Ed25519 key at runtime. It boots a task-specific devcontainer using the real
configuration and a unique label, runs `devcontainer exec ... just dev-smoke`,
inspects the actual user, security options and bind mounts, and proves that a
write to the mounted SSH directory fails with `EROFS`. It makes and verifies
an SSH-signed commit in a temporary repository inside the container, checks
post-create idempotence, rejects root execution of the smoke check, and confirms
the host key/directory are unchanged. It removes its container, newly created
volumes and disposable keys even on ordinary test failure. It never reads or
copies your real SSH directory, changes this repository's Git configuration,
or authenticates with GitHub. Interrupted/forcibly killed checks may require
cleanup of resources bearing their exact `pocketpull.signing-smoke` label.

## Telemetry smoke check
See [Real telemetry smoke check](telemetry.md#real-telemetry-smoke-check).
