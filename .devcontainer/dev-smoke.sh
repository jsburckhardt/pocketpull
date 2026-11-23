#!/usr/bin/env bash
set -euo pipefail
trap 'echo "Workspace smoke failed at line $LINENO: $BASH_COMMAND" >&2' ERR

test "$(id -u)" -ne 0
test "$(id -un)" = vscode
test "$(getent passwd vscode | cut -d: -f7)" = /bin/zsh
test "$(go version | awk '{print $3}')" = go1.27.4
test "$(git --version)" = 'git version 2.56.1'
test "$(gh --version | head -1 | awk '{print $3}')" = 2.105.0
copilot --version | head -1 | grep -F '1.0.114'
herdr --version | grep -Fx 'herdr 0.9.5'
test "$(just --version)" = 'just 1.59.0'
test "$(dpkg-query -W -f='${Version}' zsh)" = 5.9-6ubuntu2
test "$(go env GOTOOLCHAIN)" = local
test "${COPILOT_AUTO_UPDATE:-}" = false

findmnt -rn -M "$HOME/.ssh" -o OPTIONS | tr ',' '\n' | grep -Fx ro
test ! -S /var/run/docker.sock
test ! -S /run/docker.sock
if pgrep -x herdr; then
    echo 'Herdr must not auto-start' >&2
    exit 1
fi
for directory in "$HOME/.config/gh" "$HOME/.copilot" "$HOME/.shell-history"; do
    mountpoint -q "$directory"
    test -w "$directory"
    test "$(stat -c %u "$directory")" = "$(id -u)"
done
(cd "$HOME" && zsh -ic 'print -s pocketpull-history-smoke; fc -AI' </dev/null)
grep -q pocketpull-history-smoke "$HOME/.shell-history/zsh_history"
echo 'PASS: non-root, pinned tools, read-only SSH, writable state/history, no Docker socket or Herdr server'
