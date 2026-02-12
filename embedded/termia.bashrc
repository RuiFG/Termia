# ~/.termia/shell/termia.bashrc
# Shim RC file for Termia wrapper. Loads user's RC, then Termia integration.

# Load user's bashrc if present
if [[ -f "$HOME/.bashrc" ]]; then
    source "$HOME/.bashrc"
fi

# Load Termia integration (inside wrapper only)
_termia_shell_dir="${TERMIA_SHELL_DIR:-$HOME/.termia/shell}"
if [[ -f "${_termia_shell_dir}/termia.bash" ]]; then
    source "${_termia_shell_dir}/termia.bash"
fi

# Disable capturing the wrapper's rc commands
unset TERMIA_INTERNAL
