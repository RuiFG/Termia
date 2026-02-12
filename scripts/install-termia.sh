#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF'
Usage: install-termia.sh [--bin <path>] [--target <dir>]

Options:
  --bin     Path to the compiled termia binary (default: ./termia)
  --target  Install directory (default: /usr/local/bin on macOS, /bin on Linux/WSL)
  -h, --help  Show this help
EOF
}

bin_path="./termia"
target_dir=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --bin)
      bin_path="$2"
      shift 2
      ;;
    --target)
      target_dir="$2"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "Unknown argument: $1" >&2
      usage >&2
      exit 1
      ;;
  esac
done

if [[ -z "$target_dir" ]]; then
  uname_out="$(uname -s)"
  case "$uname_out" in
    Darwin)
      target_dir="/usr/local/bin"
      ;;
    *)
      target_dir="/bin"
      ;;
  esac
fi

if [[ ! -f "$bin_path" ]]; then
  echo "Binary not found at: $bin_path" >&2
  exit 1
fi

mkdir -p "$target_dir"
install -m 0755 "$bin_path" "$target_dir/termia"

echo "Installed termia to $target_dir/termia"
