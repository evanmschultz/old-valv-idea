#!/bin/sh
set -eu

result_dir="$PWD/.valv-fixture"
mkdir -p "$result_dir"
result_file="$result_dir/codex-run.txt"
all_args="$*"
stdin_tty="false"
stdout_tty="false"

if [ -t 0 ]; then
  stdin_tty="true"
fi
if [ -t 1 ]; then
  stdout_tty="true"
fi

{
  printf 'pwd=%s\n' "$(pwd)"
  printf 'codex_home=%s\n' "${CODEX_HOME:-}"
  printf 'home=%s\n' "${HOME:-}"
  printf 'user=%s\n' "${USER:-}"
  printf 'stdin_tty=%s\n' "$stdin_tty"
  printf 'stdout_tty=%s\n' "$stdout_tty"
  printf 'arg_count=%s\n' "$#"
  index=0
  for arg in "$@"; do
    printf 'arg_%s=%s\n' "$index" "$arg"
    index=$((index + 1))
  done
} > "$result_file"

if [ -n "${CODEX_HOME:-}" ]; then
  mkdir -p "$CODEX_HOME"
  printf 'pwd=%s\n' "$(pwd)" > "$CODEX_HOME/.valv-fixture-home.txt"
fi

mode=""
output_last_message=""
while [ "$#" -gt 0 ]; do
  case "$1" in
    exec)
      mode="exec"
      ;;
    -o|--output-last-message)
      shift
      output_last_message="${1:-}"
      ;;
  esac
  shift
done

if [ "$mode" = "exec" ] && [ -n "$output_last_message" ]; then
  mkdir -p "$(dirname "$output_last_message")"
  printf 'fixture exec response: %s\n' "$all_args" > "$output_last_message"
  printf '{"type":"message","role":"assistant","content":"fixture exec response"}\n'
fi
