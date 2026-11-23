#!/usr/bin/env bash
set -euo pipefail

test "$(id -un)" = vscode
for directory in "$HOME/.config/gh" "$HOME/.copilot" "$HOME/.shell-history"; do
    sudo chown "$(id -u):$(id -g)" "$directory"
    chmod 0700 "$directory"
done

for rc in "$HOME/.bashrc" "$HOME/.zshrc"; do
    touch "$rc"
    line='. /etc/profile.d/pocketpull-history.sh'
    if ! grep -Fxq "$line" "$rc"; then
        printf '\n%s\n' "$line" >> "$rc"
    fi
done
