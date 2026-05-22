# DROP_14 — ENV_VARS

**State:** planning
**Blocked by:** DROP_13 (todo)
**Paths (expected):** `internal/domain/` (env-var domain type), `internal/adapters/sqlite/` (env-var schema migration), `internal/services/manage/` or new `internal/services/accountenv/` (CRUD), `internal/cli/` (new `valv account env` subcommand tree), `internal/adapters/providers/` (thread env vars into ContainerRunRequest)
**Packages (expected):** `internal/domain/`, `internal/adapters/sqlite/`, `internal/services/manage/` or new package, `internal/cli/`, `internal/adapters/providers/claude/`, `internal/adapters/providers/codex/`
**PLAN.md ref:** main/PLAN.md → DROP_14_ENV_VARS row
**Workflow:** main/drops/WORKFLOW.md
**Started:** 2026-05-22
**Closed:** —

## Scope

**Theme 1 from `project_valv_future_planning_post_drop8.md`.** Per-account env var map — arbitrary `key→value` pairs scoped to a managed account. Same env-var name allowed across accounts with different values (e.g. `ANTHROPIC_API_KEY = sk-aaa` for `personal`, `sk-bbb` for `hylla`). Threaded into container launch via `ContainerRunRequest.Env` alongside the existing `CLAUDE_CONFIG_DIR` / `CODEX_HOME` env vars set by the provider runtime adapters.

CLI surface: `valv account env set <name> <KEY=VALUE>`, `valv account env unset <name> <KEY>`, `valv account env list <name>`. The set/unset/list operations are account-scoped (one account, one env-var map).

Storage: plaintext-in-SQLite at v0.1. Keychain integration deferred — flagged in the worklog as a follow-up so the threat model is documented even though the v0.1 implementation doesn't encrypt at rest.

The DROP_13 generic-run primitive must accept the resolved env-var map identically to how `valv codex` / `valv claude` do today (i.e. the env-var threading is a property of `ContainerRunRequest`, not provider-specific).

## Planner

(To be filled by planner — see WORKFLOW.md § "Phase 1 — Plan". Use Hylla MCP at the post-DROP_12 snapshot to map current ContainerRunRequest.Env wiring + sqlite schema-migration pattern from DROP_3 + account-scoped CRUD pattern from existing `account` subcommand tree.)

## Notes

(Filled during planning.)
