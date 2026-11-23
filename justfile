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
