#!/bin/sh
set -eu

result_dir="$PWD/.valv-fixture"
mkdir -p "$result_dir"
result_file="$result_dir/codex-run.txt"

{
  printf 'pwd=%s\n' "$(pwd)"
  printf 'codex_home=%s\n' "${CODEX_HOME:-}"
  if [ -t 0 ]; then
    printf 'stdin_tty=true\n'
  else
    printf 'stdin_tty=false\n'
  fi
  if [ -t 1 ]; then
    printf 'stdout_tty=true\n'
  else
    printf 'stdout_tty=false\n'
  fi
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
