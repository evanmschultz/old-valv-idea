# DROP_15 — NETWORK_POLICY

**State:** planning
**Blocked by:** DROP_14 (todo)
**Paths (expected):** `internal/domain/` (network-policy domain type — allowlist shape), `internal/tools/` (extend `.valv/tools.toml` `[allowlist]` parsing — currently `toml.Primitive` placeholder), `internal/adapters/providers/` (Docker `--network` / `--add-host` wiring), `internal/services/networkpolicy/` (new — resolution + closed-default enforcement), `internal/cli/` (allowlist edit subcommands or `valv network` namespace)
**Packages (expected):** `internal/domain/`, `internal/tools/`, `internal/adapters/providers/claude/`, `internal/adapters/providers/codex/`, possibly new `internal/services/networkpolicy/`, `internal/cli/`
**PLAN.md ref:** main/PLAN.md → DROP_15_NETWORK_POLICY row
**Workflow:** main/drops/WORKFLOW.md
**Started:** 2026-05-22
**Closed:** —

## Scope

Closed-by-default outbound network for the container, with per-project allowlist (claudebox pattern identified in the 2026-05-20 OSS survey — see `main/CLAUDE.md` § "Product Direction"). The allowlist is editable two ways:

- Via CLI command (e.g. `valv network allow <host>` / `valv network deny <host>` / `valv network list`).
- Via `.valv/tools.toml` `[allowlist]` section (DROP_11 reserved this section's parsing as `toml.Primitive` — DROP_15 promotes it to a typed schema).

Opt-out flag for projects that need open egress (`--network open` or equivalent on `valv claude` / `valv codex` / `valv run`).

Implementation surface: Docker container runtime accepts `--network` and `--add-host` flags via the existing `ContainerRunRequest` plumbing. The closed-default behavior likely requires either a custom Docker network or `--network none` plus an HTTP/HTTPS proxy sidecar with allowlist filtering. The planner must choose between approaches based on operational simplicity vs. Docker-Desktop-on-macOS quirks.

## Planner

(To be filled by planner — see WORKFLOW.md § "Phase 1 — Plan". Use Hylla MCP at the post-DROP_12 snapshot to map current Docker `--network` usage + ContainerRunRequest flag wiring + DROP_11's `[allowlist]` `toml.Primitive` placeholder location. Also: query the OSS sandbox survey notes in `main/CLAUDE.md` for the claudebox closed-default + allowlist pattern reference.)

## Notes

(Filled during planning.)
