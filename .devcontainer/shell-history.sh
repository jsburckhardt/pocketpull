if [ -n "${ZSH_VERSION:-}" ]; then
    HISTFILE="$HOME/.shell-history/zsh_history"
    HISTSIZE=10000
    SAVEHIST=10000
    setopt APPEND_HISTORY SHARE_HISTORY HIST_IGNORE_SPACE
elif [ -n "${BASH_VERSION:-}" ]; then
    HISTFILE="$HOME/.shell-history/bash_history"
    HISTSIZE=10000
    HISTFILESIZE=10000
    HISTCONTROL=ignoreboth
    shopt -s histappend
fi
