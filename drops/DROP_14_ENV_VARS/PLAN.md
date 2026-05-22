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

### Scope Confirmation
DROP_14 adds an account-scoped env map persisted in SQLite and injected into container launches via `ContainerRunRequest.Env`. The plan is anchored on the verified env seam, not on any assumed DROP_13 package layout.

### Unit 14.1
- state: todo
- blocked_by: none
- paths: `internal/domain/repository.go`, `internal/adapters/sqlite/store.go`, `internal/adapters/sqlite/store_test.go`
- packages: `internal/domain`, `internal/adapters/sqlite`
- change: Add a dedicated account-env repository contract in `internal/domain/repository.go` and implement it in `sqlite.Store`. Extend `Store.Bootstrap` from `user_version = 1` to `2` using the existing forward-only rebuild pattern. Persist rows by `profile_id + env_key` so duplicate env names across accounts work and account renames remain stable because ownership follows `Profile.ID`, not account name.
- acceptance: `go test ./internal/adapters/sqlite ./internal/domain` passes. Extend the existing sqlite migration coverage to prove upgrade to `user_version = 2`, idempotence on repeated bootstrap, duplicate-key support across two profiles, and set/list/unset CRUD.

### Unit 14.2
- state: todo
- blocked_by: 14.1
- paths: `internal/services/manage/service.go`, `internal/services/manage/service_test.go`
- packages: `internal/services/manage`
- change: Extend `manage.Service` with account-env CRUD methods that resolve accounts through the existing `ProfileByName` seam, wrap repository errors in the current manage-service style, validate env-key syntax, and reject Valv-owned runtime keys (`CODEX_HOME`, `CLAUDE_CONFIG_DIR`, `HOME`, `LOGNAME`, `TERM`, `USER`).
- acceptance: `go test ./internal/services/manage` passes with table-driven coverage for missing accounts, reserved-key rejection, same key with different values on different accounts, and rename stability through `Profile.ID`.

### Unit 14.3
- state: todo
- blocked_by: 14.2
- paths: `internal/cli/manage.go`, `internal/cli/manage_test.go`, `internal/cli/extended_test.go`
- packages: `internal/cli`
- change: Add a new `account env` branch under `newManageAccountCommand` with `set <name> <KEY=VALUE>`, `unset <name> <KEY>`, and `list <name>`. Reuse `openManageService`, existing output-mode handling, and the account-list/status output conventions for human and JSON rendering.
- acceptance: `go test ./internal/cli` passes with coverage for human and JSON list output, malformed `KEY=VALUE`, reserved-key errors, missing-account errors, and same env-key separation across two accounts.

### Unit 14.4
- state: todo
- blocked_by: 14.1
- paths: `internal/services/codex/service.go`, `internal/services/codex/service_test.go`
- packages: `internal/services/codex`
- change: After `codexruntime.PrepareRuntime`, load the bound profile's env map by `profile.ID`, clone `prepared.Env`, merge account env without overriding Valv-owned runtime keys, and pass the merged map into `ContainerRunRequest.Env`. Keep the merge in the service layer so plaintext values never enter provider-runtime debug logs.
- acceptance: `go test ./internal/services/codex` passes. Extend the existing `buildRequest`/run-request assertions so `executor.got.Env` contains account env plus preserved `CODEX_HOME`, `HOME`, and any cross-provider `CLAUDE_CONFIG_DIR`.

### Unit 14.5
- state: todo
- blocked_by: 14.1
- paths: `internal/services/claude/service.go`, `internal/services/claude/service_test.go`
- packages: `internal/services/claude`
- change: Mirror unit 14.4 for the Claude launcher: resolve the profile env map after `clauderuntime.PrepareRuntime`, merge into a cloned env map, preserve Valv-owned runtime keys, and thread the result into `ContainerRunRequest.Env`.
- acceptance: `go test ./internal/services/claude` passes with assertions that `executor.got.Env` contains account env while preserving `CLAUDE_CONFIG_DIR`, `HOME`, and any cross-provider `CODEX_HOME`.

### Notes

- Plaintext-in-SQLite is accepted for v0.1. The explicit threat model is: local DB readers and the caller of `valv account env list` can see values; debug logs must not.
- If DROP_13 moves launch ownership before build starts, keep units 14.1–14.3 unchanged and re-home 14.4–14.5 to the new code that assembles `ContainerRunRequest.Env`.
