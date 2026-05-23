# DROP_14 — ENV_VARS

**State:** planning
**Blocked by:** DROP_13 (todo)
**Paths (expected):** `internal/domain/` (env-var domain type), `internal/adapters/sqlite/` (env-var schema migration + DSN busy policy), `internal/services/manage/` (account-scoped CRUD), `internal/cli/` (new `valv account env` subcommand tree), `internal/services/run/` (shared launch owner after DROP_13)
**Packages (expected):** `internal/domain/`, `internal/adapters/sqlite/`, `internal/services/manage/`, `internal/cli/`, `internal/services/run/`
**PLAN.md ref:** main/PLAN.md → DROP_14_ENV_VARS row
**Workflow:** main/drops/WORKFLOW.md
**Started:** 2026-05-22
**Closed:** —

## Scope

**Theme 1 from `project_valv_future_planning_post_drop8.md`.** Per-account env var map — arbitrary `key→value` pairs scoped to a managed account. Same env-var name allowed across accounts with different values (e.g. `ANTHROPIC_API_KEY = sk-aaa` for `personal`, `sk-bbb` for `hylla`). Threaded into container launch via `ContainerRunRequest.Env` alongside the existing `CLAUDE_CONFIG_DIR` / `CODEX_HOME` env vars set by the provider runtime adapters.

CLI surface: `valv account env set <name> <KEY=VALUE>`, `valv account env unset <name> <KEY>`, `valv account env list <name>`. The set/unset/list operations are account-scoped (one account, one env-var map).

Storage: plaintext-in-SQLite at v0.1. Keychain integration deferred — flagged in the worklog as a follow-up so the threat model is documented even though the v0.1 implementation doesn't encrypt at rest.

The DROP_13 generic-run primitive must accept the resolved env-var map identically to how `valv codex` / `valv claude` do today. In this revision that requirement is promoted from note to gate: DROP_14 assumes DROP_13 re-homes request construction into the shared launch owner and will not silently revive provider-specific env threading if DROP_13 closes differently.

## Planner

### Scope Confirmation
DROP_14 adds an account-scoped env map persisted in SQLite and injected into container launches via `ContainerRunRequest.Env`. The current committed seams are verified at `internal/domain/repository.go:5-32`, `internal/cli/store.go:11-24`, `internal/adapters/sqlite/open.go:32-70`, `internal/adapters/sqlite/store.go:35-153`, `internal/adapters/sqlite/store_test.go:504-668`, `internal/services/manage/service.go:19-25,166-190`, `internal/cli/manage.go:22-63,200-239,1348-1365`, `internal/services/codex/service.go:121-215,302-332`, `internal/services/claude/service.go:123-223,298-329`, `internal/adapters/providers/codex/runtime.go:117-132`, and `internal/adapters/providers/claude/runtime.go:127-142`. Hylla at `github.com/evanmschultz/valv@main` pinned to `1759e64` confirms the same callers/tests for `Store.Bootstrap`, `newManageAccountCommand`, and both provider `Service.Run` paths.

### Schema Decisions

- **F1: handle the SQLITE_BUSY gap inside Unit 14.1; do not add Unit 14.0.** The DSN builder and the v2 migration both live in the same sqlite/domain boundary, so splitting them into a standalone precondition unit would create a fifth task without creating an independently buildable seam. The fix is DSN-level busy policy, not a separate retry loop: add `_pragma=busy_timeout(5000)` alongside `_pragma=foreign_keys(1)` in `internal/adapters/sqlite/open.go`, then advance `Store.Bootstrap` to `user_version = 2` for the new env-var table. This is directly supported by the driver contract that `_pragma` query params are executed as `PRAGMA ...` statements (`modernc.org/sqlite/driver.go:45-52`) and that `busy_timeout` is intentionally applied first when multiple pragmas are present (`modernc.org/sqlite/sqlite.go:143-165`). The acceptance for Unit 14.1 therefore includes a two-connection first-open migration test against a v1 DB needing v2 upgrade, asserting "one waits, both succeed" instead of allowing `SQLITE_BUSY`.
- **F2: choose Option A and replace provider-specific launch units with one shared-launch unit against DROP_13's launch owner.** DROP_13's accepted architecture moves shared launch orchestration into `internal/services/run` (`drops/DROP_13_GENERIC_RUN/PLAN.md:36-37,72-82,113-143`), so DROP_14 must add env threading at that shared owner rather than at `internal/services/codex` and `internal/services/claude`. This promotes the cross-drop dependency from note-only to acceptance gate: Unit 14.4 is blocked by DROP_13, and if DROP_13 lands with a different `ContainerRunRequest.Env` owner, the builder must stop and re-plan instead of patching stale provider-specific services.
- **F3: JSON list output is redacted by default with explicit metadata.** `valv account env list <name>` uses top-level key `env` (new, not yet in tree) whose entries are `{"key":"FOO","value":"***","redacted":true}` by default and `{"key":"FOO","value":"raw","redacted":false}` when `--reveal` is present. This is intentionally more specific than the generic list-item envelope used by existing manage commands (`internal/output/output.go:82-93`, `internal/cli/extended_test.go:136-153`): env values need machine-readable redaction state so consumers do not have to infer policy from the literal string `"***"`.

### Unit 14.1
- state: todo
- blocked_by: none
- paths: `internal/domain/repository.go`, `internal/adapters/sqlite/open.go`, `internal/adapters/sqlite/open_test.go`, `internal/adapters/sqlite/store.go`, `internal/adapters/sqlite/store_test.go`
- packages: `internal/domain`, `internal/adapters/sqlite`
- change: Add an account-env repository contract (new, not yet in tree) in `internal/domain/repository.go` and implement it in `sqlite.Store`. Extend `Open` so every SQLite connection carries `_pragma=busy_timeout(5000)` as well as `_pragma=foreign_keys(1)` for both path- and URI-based opens, then advance `Store.Bootstrap` from `user_version = 1` to `2` with a forward-only new account-env table (new, not yet in tree) keyed by `profile_id + env_key` and storing plaintext `env_value`. The table must be owned by `profile_id`, not account name, so duplicate keys across accounts are legal and account renames remain stable because `Profile.ID` does not change. Evidence: `internal/cli/store.go:11-24` bootstraps on every CLI store open; `internal/adapters/sqlite/open.go:32-70` currently adds only `_pragma=foreign_keys(1)`; `modernc.org/sqlite/driver.go:45-52` and `modernc.org/sqlite/sqlite.go:143-165` prove `_pragma` support and `busy_timeout` ordering; `internal/adapters/sqlite/store.go:35-153` owns bootstrap + `PRAGMA user_version`; `internal/adapters/sqlite/store_test.go:504-668` already proves migration/idempotence behavior for the current schema path.
- acceptance: `mage testPkg ./internal/domain` and `mage testPkg ./internal/adapters/sqlite` pass. Extend sqlite coverage to prove:
  - upgrade from `user_version = 1` to `2`;
  - idempotence on repeated bootstrap at v2;
  - duplicate-key support across two different profiles;
  - set/list/unset CRUD at the store layer;
  - rename stability after `UpdateProfileName` because env ownership is keyed by unchanged `profile_id`;
  - DSN construction adds both `_pragma=busy_timeout(5000)` and `_pragma=foreign_keys(1)` without duplicating either pragma when already present;
  - a two-connection first-open upgrade from a v1 DB to v2 results in "one waits, both succeed", no `SQLITE_BUSY`, and final `PRAGMA user_version = 2`.

### Unit 14.2
- state: todo
- blocked_by: 14.1
- paths: `internal/services/manage/service.go`, `internal/services/manage/service_test.go`
- packages: `internal/services/manage`
- change: Extend `manage.Service` and its `Store` aggregation with account-env CRUD methods (new, not yet in tree) that resolve accounts through the existing `ProfileByName` seam, wrap repository errors in the current manage-service style, validate env keys against the explicit regex `^[A-Za-z_][A-Za-z0-9_]*$`, and surface that literal pattern in the rejection error so operators see the constraint. Reject the six runtime-owned keys that the committed launch env builders already own: `CODEX_HOME`, `CLAUDE_CONFIG_DIR`, `HOME`, `LOGNAME`, `TERM`, and `USER`. Evidence: `internal/services/manage/service.go:19-25,166-190` shows the current service/store seam and `ProfileByName` lookup path; `internal/services/manage/service_test.go:660-703` shows rename stability is already a first-class invariant; `internal/adapters/providers/codex/runtime.go:117-132` and `internal/adapters/providers/claude/runtime.go:127-142` define the current runtime-owned env keys that DROP_14 must reserve.
- acceptance: `mage testPkg ./internal/services/manage` passes with table-driven coverage for:
  - set/unset/list happy paths on one account;
  - missing-account errors;
  - reserved-key rejection for all six protected keys;
  - same key with different values on different accounts;
  - rename stability through `Profile.ID`;
  - env-key rejection for leading-digit, whitespace, hyphenated, dotted, control-character, and empty keys.
  The failing-key assertions must verify that the returned error wraps the literal regex `^[A-Za-z_][A-Za-z0-9_]*$`.

### Unit 14.3
- state: todo
- blocked_by: 14.2
- paths: `internal/cli/manage.go`, `internal/cli/manage_test.go`, `internal/cli/extended_test.go`
- packages: `internal/cli`
- change: Add a new `account env` branch under `newManageAccountCommand` (new subcommands, not yet in tree) with `set <name> <KEY=VALUE>`, `unset <name> <KEY>`, and `list <name>`. Reuse `openManageService`, the existing output-mode detection, and the current account-command wiring, but use a dedicated env-list formatter for JSON instead of `output.WriteListWithKey`: env listing needs explicit `key/value/redacted` entries rather than the generic `title/fields` envelope. Human output for `valv account env list <name>` is `KEY=***` by default and raw `KEY=value` only with `--reveal`. JSON output uses top-level key `env` with entries shaped as `{"key":"FOO","value":"***","redacted":true}` by default and raw values plus `redacted:false` when `--reveal` is set. Evidence: `internal/cli/manage.go:22-63` wires the existing account tree; `internal/cli/manage.go:200-239,1348-1365` shows current output-mode and command-owned JSON-key conventions; `internal/cli/extended_test.go:136-153` pins the existing command-owned JSON key pattern; `internal/output/output.go:82-93` shows why the generic list envelope is not sufficient for machine-readable redaction metadata.
- acceptance: `mage testPkg ./internal/cli` passes with coverage for:
  - human `list` output redacted by default as `KEY=***`;
  - human `list --reveal` output showing raw values;
  - JSON `list` output redacted by default with top-level key `"env"` and per-entry `redacted: true`;
  - JSON `list --reveal` output showing raw values with `redacted: false`;
  - malformed `KEY=VALUE` input;
  - reserved-key errors propagated from the service layer;
  - missing-account errors;
  - same env-key separation across two accounts.

### Unit 14.4
- state: todo
- blocked_by: 14.1, DROP_13
- paths: `internal/services/run/service.go` (new after DROP_13, not yet in the current tree), `internal/services/run/service_test.go` (new after DROP_13, not yet in the current tree), optionally `internal/services/codex/service_test.go` and `internal/services/claude/service_test.go` if thin-wrapper assertions are needed after the re-home
- packages: `internal/services/run`, optionally `internal/services/codex`, optionally `internal/services/claude`
- change: Add account-env loading and merge at the shared launch owner that constructs `ContainerRunRequest.Env` after DROP_13 lands. The shared launch path must resolve the bound or override profile ID, load that account's env map from the new repository contract, clone the provider-prepared env map, merge account env without overriding the runtime-owned keys, and pass the merged result into `ContainerRunRequest.Env`. This is the acceptance gate that makes `valv run --account <name> ...`, `valv codex`, and `valv claude` behave identically once DROP_13's shared launch owner exists. If DROP_13 closes without `internal/services/run` owning `ContainerRunRequest.Env`, this unit must stop and hand control back for re-planning; provider-specific fallback edits are out of scope for DROP_14. Evidence: `drops/DROP_13_GENERIC_RUN/PLAN.md:36-37,72-82,113-143` makes `internal/services/run` the shared owner; current committed provider services still pass `prepared.Env` straight into `ContainerRunRequest.Env` at `internal/services/codex/service.go:302-332` and `internal/services/claude/service.go:298-329`; `internal/adapters/docker/types.go:43-60` defines the target `ContainerRunRequest.Env` field; `internal/adapters/providers/codex/runtime.go:117-132` and `internal/adapters/providers/claude/runtime.go:127-142` define the runtime-owned keys that must win on merge.
- acceptance: `mage testPkg ./internal/services/run` passes after DROP_13 lands, and any thin wrapper packages touched by the re-home still pass their existing launch-package tests. Coverage must prove:
  - the shared launch owner used by `valv run --account <name>` receives and forwards the account env map into `ContainerRunRequest.Env`;
  - provider-owned runtime keys (`CODEX_HOME`, `CLAUDE_CONFIG_DIR`, `HOME`, `LOGNAME`, `TERM`, `USER`) are preserved exactly as prepared by the runtime adapters;
  - directly seeded reserved-key rows in storage do not override runtime-owned values even when CLI/service-layer validation is bypassed;
  - the same merged path is exercised for both generic run and provider-wrapper launches once DROP_13's shared owner exists.
  If build-time ownership differs from DROP_13's `internal/services/run`, the unit remains blocked and no provider-specific substitute implementation is accepted.

### Notes For Builder Agents

- Plaintext-in-SQLite is accepted for v0.1. Do not add keychain work, at-rest encryption, or secrets-manager plumbing in this drop.
- Redact `valv account env list` by default in both human and JSON output. `--reveal` is the only opt-in to raw values. Terminal scrollback, copied transcripts, redirected stdout/stderr, and shell capture after `--reveal` are operator responsibility; the plan's safety bar is "no accidental disclosure in default paths", not "no disclosure when explicitly requested".
- Keep the reserved-key list exactly to the six runtime-owned keys in this drop: `CODEX_HOME`, `CLAUDE_CONFIG_DIR`, `HOME`, `LOGNAME`, `TERM`, and `USER`.
- Keep the busy policy at the SQLite open boundary (`open.go`) so every caller that opens the DB inherits it. Do not add a second ad hoc retry layer unless the code proves `busy_timeout` alone is insufficient.
- Treat DROP_13's shared launch owner as a hard gate. If `ContainerRunRequest.Env` is no longer assembled in `internal/services/run` when DROP_14 build starts, stop and re-plan rather than reviving provider-specific env threading.
- Keep tests table-driven where the behavior is parse-heavy or policy-heavy (regex validation, reserved keys, redaction/reveal output, concurrent first-open migration).
