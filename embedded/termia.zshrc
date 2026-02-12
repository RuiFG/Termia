# ~/.termia/shell/termia.zshrc
# Shim RC file for Termia wrapper. Loads user's RC, then Termia integration.

# Load user's zshrc if present
if [[ -f "$HOME/.zshrc" ]]; then
    source "$HOME/.zshrc"
fi

# Load Termia integration (inside wrapper only)
_termia_shell_dir="${TERMIA_SHELL_DIR:-$HOME/.termia/shell}"
if [[ -f "${_termia_shell_dir}/termia.zsh" ]]; then
    source "${_termia_shell_dir}/termia.zsh"
fi

# Disable capturing the wrapper's rc commands
unset TERMIA_INTERNAL
