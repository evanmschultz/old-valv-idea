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
  if [ -f "$CODEX_HOME/config.toml" ]; then
    cp "$CODEX_HOME/config.toml" "$result_dir/codex-config.toml"
  else
    printf 'missing\n' > "$result_dir/codex-config.toml"
  fi
fi

mode=""
output_last_message=""
no_alt_screen="false"
interactive_fixture="true"
while [ "$#" -gt 0 ]; do
  case "$1" in
    exec)
      mode="exec"
      interactive_fixture="false"
      ;;
    --no-alt-screen)
      no_alt_screen="true"
      ;;
    -o|--output-last-message)
      interactive_fixture="false"
      shift
      output_last_message="${1:-}"
      ;;
    -*)
      ;;
    *)
      interactive_fixture="false"
      ;;
  esac
  shift
done

if [ "$mode" = "exec" ] && [ -n "$output_last_message" ]; then
  mkdir -p "$(dirname "$output_last_message")"
  printf 'fixture exec response: %s\n' "$all_args" > "$output_last_message"
  printf '{"type":"message","role":"assistant","content":"fixture exec response"}\n'
fi

list_mcp_servers() {
  config_path="${CODEX_HOME:-}/config.toml"
  if [ ! -f "$config_path" ]; then
    return 0
  fi
  awk '
    /^[[:space:]]*\[mcp_servers\.[^]]+\][[:space:]]*$/ {
      name = $0
      sub(/^[[:space:]]*\[mcp_servers\./, "", name)
      sub(/\][[:space:]]*$/, "", name)
      gsub(/^"/, "", name)
      gsub(/"$/, "", name)
      if (index(name, ".") == 0) {
        print name
      }
    }
  ' "$config_path"
}

mcp_field() {
  server_name="$1"
  field_name="$2"
  config_path="${CODEX_HOME:-}/config.toml"
  if [ ! -f "$config_path" ]; then
    return 0
  fi
  awk -v target="$server_name" -v field="$field_name" '
    function section_name(raw,    name) {
      name = raw
      sub(/^[[:space:]]*\[mcp_servers\./, "", name)
      sub(/\][[:space:]]*$/, "", name)
      gsub(/^"/, "", name)
      gsub(/"$/, "", name)
      return name
    }

    /^[[:space:]]*\[mcp_servers\.[^]]+\][[:space:]]*$/ {
      current = section_name($0)
      next
    }

    /^[[:space:]]*\[/ {
      next
    }

    current == target {
      line = $0
      sub(/^[[:space:]]*/, "", line)
      if (line ~ ("^" field "[[:space:]]*=")) {
        sub(("^" field "[[:space:]]*=[[:space:]]*"), "", line)
        gsub(/^"/, "", line)
        gsub(/"$/, "", line)
        print line
        exit
      }
    }
  ' "$config_path"
}

has_context7_headers() {
  config_path="${CODEX_HOME:-}/config.toml"
  if [ ! -f "$config_path" ]; then
    return 1
  fi
  grep -q 'CONTEXT7_API_KEY = "CONTEXT7_API_KEY"' "$config_path"
}

print_mcp_screen() {
  printf '\n🔌  MCP Tools\n\n'
  listed="false"
  for server in $(list_mcp_servers); do
    listed="true"
    printf '  • %s\n' "$server"
    url="$(mcp_field "$server" "url")"
    case "$server" in
      context7-mcp)
        printf '    • Auth: Not logged in\n'
        if [ -n "$url" ]; then
          printf '    • URL: %s\n' "$url"
        fi
        if has_context7_headers; then
          printf '    • Env HTTP headers: CONTEXT7_API_KEY=CONTEXT7_API_KEY\n'
        fi
        printf '    • Tools: (none)\n'
        ;;
      *)
        printf '    • Auth: Unsupported\n'
        if [ -n "$url" ]; then
          printf '    • URL: %s\n' "$url"
        fi
        printf '    • Tools: fixture-tool\n'
        ;;
    esac
    printf '    • Resources: (none)\n'
    printf '    • Resource templates: (none)\n\n'
  done
  if [ "$listed" = "false" ]; then
    printf '  • (none)\n\n'
  fi
}

if [ "$stdin_tty" = "true" ] && [ "$stdout_tty" = "true" ] && [ "$mode" != "exec" ] && [ "$interactive_fixture" = "true" ]; then
  printf '╭──────────────────────────────────────────────╮\n'
  printf '│ >_ OpenAI Codex (fixture)                    │\n'
  printf '│                                              │\n'
  printf '│ model:     gpt-5.4 xhigh   /model to change  │\n'
  printf '│ directory: /workspace/fixture                │\n'
  printf '╰──────────────────────────────────────────────╯\n'
  printf '\n'
  printf '  Tip: Fixture ready.\n'
  printf '\n'
  while IFS= read -r line; do
    case "$line" in
      /mcp)
        printf '› /mcp\n'
        print_mcp_screen
        ;;
      /quit|exit|quit)
        printf '› %s\n' "$line"
        exit 0
        ;;
      *)
        printf '› %s\n' "$line"
        ;;
    esac
  done
  exit 0
fi
