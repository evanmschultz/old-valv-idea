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
DROP_14 adds an account-scoped env map persisted in SQLite and injected into container launches via `ContainerRunRequest.Env`. The revision is anchored on the verified seams that already exist today: repository contracts in `internal/domain/repository.go`, forward-only schema migration in `internal/adapters/sqlite/store.go`, account resolution in `internal/services/manage/service.go`, account command wiring in `internal/cli/manage.go`, and launch-time env assembly in `internal/services/codex/service.go` / `internal/services/claude/service.go`. Existing evidence: `internal/domain/repository.go:5-25`, `internal/adapters/sqlite/store.go:40-172`, `internal/adapters/sqlite/store_test.go:504-660`, `internal/services/manage/service.go:21-25,186-190`, `internal/cli/manage.go:22-63,200-239,1333-1365`, `internal/services/codex/service.go:155-185,302-332`, `internal/services/claude/service.go:162-194,298-329`.

### Unit 14.1
- state: todo
- blocked_by: none
- paths: `internal/domain/repository.go`, `internal/adapters/sqlite/store.go`, `internal/adapters/sqlite/store_test.go`
- packages: `internal/domain`, `internal/adapters/sqlite`
- change: Add a dedicated account-env repository contract (new, not yet in tree) in `internal/domain/repository.go` and implement it in `sqlite.Store`. Extend `Store.Bootstrap` from `user_version = 1` to `2` using the existing forward-only rebuild pattern already used for `project_bindings`, and persist env rows by `profile_id + env_key` so duplicate env names across accounts work while account renames stay stable because ownership follows `Profile.ID`, not account name. Evidence: `internal/domain/repository.go:5-25` has no env repository today; `internal/adapters/sqlite/store.go:40-172` owns bootstrap + `PRAGMA user_version`; `internal/adapters/sqlite/store_test.go:504-660` already proves migration and idempotence behavior for the current schema path.
- acceptance: `mage testPkg ./internal/domain` and `mage testPkg ./internal/adapters/sqlite` pass. Extend sqlite coverage to prove upgrade to `user_version = 2`, idempotence on repeated bootstrap, duplicate-key support across two different profiles, set/list/unset CRUD, and rename stability after `UpdateProfileName` because env ownership is keyed by unchanged `profile_id`.

### Unit 14.2
- state: todo
- blocked_by: 14.1
- paths: `internal/services/manage/service.go`, `internal/services/manage/service_test.go`
- packages: `internal/services/manage`
- change: Extend `manage.Service` and its `Store` aggregation with account-env CRUD methods (new, not yet in tree) that resolve accounts through the existing `ProfileByName` seam, wrap repository errors in the current manage-service style, validate env keys against the explicit regex `^[A-Za-z_][A-Za-z0-9_]*$`, and surface that literal pattern in the rejection error so operators see the constraint. Reject runtime-owned keys that Valv already injects into launch env maps: `CODEX_HOME`, `CLAUDE_CONFIG_DIR`, `HOME`, `LOGNAME`, `TERM`, and `USER`. `CLAUDE_CODE_OAUTH_TOKEN` is **not** currently part of the committed Claude launch env path and should only join the reserved list if Unit 14.5 verifies that it is again set into `ContainerRunRequest.Env`. Evidence: `internal/services/manage/service.go:21-25,186-190` shows the current service/store seam and `ProfileByName` lookup path; `internal/services/manage/service_test.go:229-239,660-703` shows rename stability is already a first-class invariant; `internal/adapters/providers/codex/runtime.go:117-132` and `internal/adapters/providers/claude/runtime.go:127-142` define the current runtime-owned env keys.
- acceptance: `mage testPkg ./internal/services/manage` passes with table-driven coverage for missing accounts, reserved-key rejection, same key with different values on different accounts, rename stability through `Profile.ID`, and env-key rejection for leading-digit, whitespace, hyphenated, dotted, control-character, and empty keys. The failing-key assertions must verify that the returned error wraps the literal regex `^[A-Za-z_][A-Za-z0-9_]*$`.

### Unit 14.3
- state: todo
- blocked_by: 14.2
- paths: `internal/cli/manage.go`, `internal/cli/manage_test.go`, `internal/cli/extended_test.go`
- packages: `internal/cli`
- change: Add a new `account env` branch under `newManageAccountCommand` (new subcommands, not yet in tree) with `set <name> <KEY=VALUE>`, `unset <name> <KEY>`, and `list <name>`. Reuse `openManageService`, current output-mode handling, and the existing manage-list JSON conventions. For `valv account env list <name>`, the explicit JSON top-level key must be `env` (new, not yet in tree): single-resource manage lists already use short command-owned keys like `accounts` and `projects`, while grouped compound payloads use snake_case names like `accounts_by_provider`. Evidence: `internal/cli/manage.go:22-63` wires the existing account tree; `internal/cli/manage.go:220-236,1333-1365,1475` shows current list/output conventions; `internal/cli/extended_test.go:136-153` pins the single-provider `"accounts"` JSON shape; `internal/output/output.go:82-93,188-192` shows caller-owned JSON list keys.
- acceptance: `mage testPkg ./internal/cli` passes with coverage for human and JSON `list` output, explicit JSON top-level key `"env"`, malformed `KEY=VALUE`, reserved-key errors propagated from the service layer, missing-account errors, and same env-key separation across two accounts.

### Unit 14.4
- state: todo
- blocked_by: 14.1
- paths: `internal/services/codex/service.go`, `internal/services/codex/service_test.go`
- packages: `internal/services/codex`
- change: After `codexruntime.PrepareRuntime`, load the bound profile's env map by `profile.ID`, clone `prepared.Env`, merge account env without overriding Valv-owned runtime keys, and pass the merged map into `ContainerRunRequest.Env`. Keep the merge in the service layer so adapter-prepared env stays deterministic and plaintext values do not get introduced into adapter-level debug logging paths. Evidence: `internal/services/codex/service.go:155-185` performs the binding lookup and calls `PrepareRuntime`; `internal/services/codex/service.go:311-316` currently passes `prepared.Env` straight through; `internal/adapters/providers/codex/runtime.go:117-132` shows the protected key set owned by the runtime adapter; `internal/services/codex/service_test.go:269-274,792-798` already asserts `CODEX_HOME`, `HOME`, and optional cross-provider `CLAUDE_CONFIG_DIR`.
- acceptance: `mage testPkg ./internal/services/codex` passes. Extend the existing request assertions so `executor.got.Env` contains account env while preserving `CODEX_HOME`, `HOME`, and any cross-provider `CLAUDE_CONFIG_DIR`; attempts to store reserved keys must not override the runtime-owned values that `PrepareRuntime` produced.

### Unit 14.5
- state: todo
- blocked_by: 14.1
- paths: `internal/services/claude/service.go`, `internal/services/claude/service_test.go`
- packages: `internal/services/claude`
- change: Mirror Unit 14.4 for the Claude launcher: resolve the profile env map after `clauderuntime.PrepareRuntime`, merge into a cloned env map, preserve Valv-owned runtime keys, and thread the result into `ContainerRunRequest.Env`. As part of this unit, explicitly verify whether `CLAUDE_CODE_OAUTH_TOKEN` is set into `ContainerRunRequest.Env` for normal `valv claude` launches. If it is, add it to the reserved-key set enforced by Unit 14.2. If it is only used in host-side auth subprocess env and not in the container launch request, no reserved-key change is needed. Current committed evidence points to the latter: `internal/services/claude/service.go:162-194,307-312` builds launch env from `prepared.Env`, `internal/adapters/providers/claude/runtime.go:127-142` defines the current launch-time keys, and repo-wide search shows no production `CLAUDE_CODE_OAUTH_TOKEN` string in the current launch path.
- acceptance: `mage testPkg ./internal/services/claude` passes with assertions that `executor.got.Env` contains account env while preserving `CLAUDE_CONFIG_DIR`, `HOME`, and any cross-provider `CODEX_HOME`. The builder must also record the `CLAUDE_CODE_OAUTH_TOKEN` verification outcome in the unit worklog: if launch-path env includes it, add reserved-key tests for it; if not, leave the reserved-key list unchanged.

### Notes

- Plaintext-in-SQLite is accepted for v0.1. No code-level privacy claim belongs in this plan. The host DB-path threat-model detail (`~/Library/Application Support/valv/db/valv.sqlite3` and filesystem-permission note) belongs in the worklog, not in `PLAN.md`.
- If DROP_13 moves launch ownership before build starts, keep Units 14.1–14.3 unchanged and re-home Units 14.4–14.5 to whichever package then assembles `ContainerRunRequest.Env`.
