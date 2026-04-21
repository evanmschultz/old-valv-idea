# DROP_3 — SCHEMA MIGRATION COMPOSITE BINDING

**State:** building
**Blocked by:** —
**Paths (expected):** `internal/adapters/sqlite/store.go` (edit — migration + `BindingRepository` methods), `internal/adapters/sqlite/store_test.go` (edit — migration and repository tests), plus any call-site updates where the binding row shape is assumed to be project-keyed
**Packages (expected):** `internal/adapters/sqlite` (real edits — schema + repository), `internal/domain` (possible edit — if `ProjectBinding` needs a `Provider` field), plus any downstream caller (`internal/services/*`, `internal/cli/*`) that indexes bindings by `project_id` alone
**PLAN.md ref:** main/PLAN.md → DROP_3_SCHEMA_MIGRATION_COMPOSITE_BINDING row
**Workflow:** main/drops/WORKFLOW.md
**Started:** 2026-04-20
**Closed:** —

## Scope

Migrate the `project_bindings` table to a composite primary key `(project_id, provider)` with a forward-only `PRAGMA user_version`-gated rebuild, update `BindingRepository` and every call site, and prove the migration preserves existing Codex rows per `VALV_CLAUDE_CODE_FOCUS_PLAN.md` §6.2a. One project may bind one account per provider — today it is Codex-only (pre-migration state has at most one binding per project); after DROP_3 the shape supports simultaneous Codex + Claude bindings per project.

## Planner

**Scope confirmed:** rewrite `project_bindings` as a composite-key table `(project_id, provider)`, introduce a `PRAGMA user_version`-gated forward-only rebuild in `Bootstrap`, update `BindingRepository.BindingByProjectID` to take a trailing `provider` argument, thread that argument through every call site, and prove the migration preserves existing Codex rows per `VALV_CLAUDE_CODE_FOCUS_PLAN.md` §6.2a (lines 211-223). MVP scope — no Claude adapter / image / CLI / switch UX work.

**Committed-state audit** (evidence: Hylla `hylla_search_keyword` + `hylla_refs_find` + `hylla_node_full` on `BindingRepository` / `Store.BindingByProjectID` / `Store.Bootstrap` / `Service.resolveBinding` / `Service.Status`, corroborated by direct Read on `internal/adapters/sqlite/store.go`, `internal/domain/repository.go`, `internal/domain/model.go`, `internal/adapters/sqlite/open.go`):

- `internal/adapters/sqlite/store.go:57-65` — `Bootstrap` declares `project_bindings (project_id TEXT PRIMARY KEY, profile_id TEXT NOT NULL, provider TEXT NOT NULL, created_at TEXT NOT NULL, modified_at TEXT NOT NULL, FK project_id → projects.id, FK profile_id → profiles.id)`. `UpsertProjectBinding` uses `ON CONFLICT(project_id)` at line 315.
- No migration machinery exists — grep `PRAGMA user_version` returns zero hits; `Bootstrap` is `CREATE TABLE IF NOT EXISTS`-only; `open.go` sets only the connection-level `foreign_keys(1)` pragma.
- `domain.ProjectBinding` (`internal/domain/model.go:29-35`) already carries `Provider Provider`. No domain-type change needed.
- `domain.BindingRepository` (`internal/domain/repository.go:20-24`) — today line 22 is `BindingByProjectID(context.Context, string) (ProjectBinding, error)`. Must gain a trailing `Provider` arg.
- Consumer-side `Store` interfaces in `codex` + `manage` embed `domain.BindingRepository` transitively — the domain signature change cascades without direct interface-declaration edits there.
- Production callers of `BindingByProjectID`: `internal/services/codex/service.go:216` (`resolveBinding`), `internal/services/manage/service.go:240` (`Status`). Two production sites.
- Test-double + test callers: `internal/adapters/sqlite/store_test.go:82,178`; `internal/services/manage/service_test.go:411`; `internal/cli/manage_test.go:79`; `internal/cli/codex_setup_test.go:68`; `internal/services/codex/service_test.go:76` (`fakeStore` method).
- SQLite constraint: `ALTER TABLE` cannot change a `PRIMARY KEY` in place; the copy-rename rebuild idiom (`CREATE new → INSERT SELECT → DROP old → ALTER RENAME`) is the only supported path. §6.2a line 218 locks this in.
- No inbound FK references `project_bindings` today — rebuild can run inside one `BeginTx` block; a future inbound FK would force the standard SQLite `foreign_keys = OFF` → rebuild → `foreign_keys = ON` toggle pattern (which must sit outside any transaction).
- `projects` (required cols `id / root / name / created_at`) and `profiles` (required cols `id / provider / name / home_path / created_at` + `UNIQUE(provider, name)`) are the FK targets from `project_bindings`. Any raw-DB test that inserts a legacy binding row with FK enforcement on must pre-seed one row in each.

**Atomic decomposition:**

### Unit 3.1 — BindingRepository composite-key signature + call-site threading

- **State:** done
- **Paths:**
  - `internal/domain/repository.go` (edit — `BindingRepository.BindingByProjectID` gains trailing `Provider` argument)
  - `internal/adapters/sqlite/store.go` (edit — `Store.BindingByProjectID` signature + `WHERE project_id = ? AND provider = ?` filter; do NOT touch `UpsertProjectBinding` or `Bootstrap` in this unit)
  - `internal/adapters/sqlite/store_test.go` (edit — existing `BindingByProjectID` calls at lines 82 and 178 pass `domain.ProviderCodex`)
  - `internal/services/codex/service.go` (edit — `resolveBinding` line 216 passes `domain.ProviderCodex`; the existing `binding.Provider != domain.ProviderCodex` check at line 223 stays as-is)
  - `internal/services/codex/service_test.go` (edit — `fakeStore.BindingByProjectID` line 76 gains the trailing `domain.Provider` parameter)
  - `internal/services/manage/service.go` (edit — `Status` line 240 passes `domain.ProviderCodex`)
  - `internal/services/manage/service_test.go` (edit — line 411 passes `domain.ProviderCodex`)
  - `internal/cli/manage_test.go` (edit — line 79 passes `domain.ProviderCodex`)
  - `internal/cli/codex_setup_test.go` (edit — line 68 passes `domain.ProviderCodex`)
- **Packages:**
  - `internal/domain` (real edit — interface change)
  - `internal/adapters/sqlite` (real edit — signature + SELECT filter; Bootstrap + Upsert left alone in this unit)
  - `internal/services/codex` (real edit — caller + test fake)
  - `internal/services/manage` (real edit — caller)
  - `internal/cli` (test-only edit — two test files pass the extra arg)
- **Acceptance:**
  - `mage testPkg ./internal/domain` passes.
  - `mage testPkg ./internal/adapters/sqlite` passes with every existing test name still green (`TestStoreProfileAndBindingLifecycle`, `TestStoreNotFoundErrors`, `TestStoreListsProjectsAndBindings`, `TestStoreForeignKeysRejectInvalidBindings`).
  - `mage testPkg ./internal/services/codex` passes; `fakeStore` still compiles.
  - `mage testPkg ./internal/services/manage` passes.
  - `mage testPkg ./internal/cli` passes.
  - Grep `BindingByProjectID` across `internal/**/*.go` shows every call-site invocation (i.e. every function-call expression, NOT interface/method declarations at `internal/domain/repository.go:22` and `internal/services/codex/service_test.go:76` which will also match the grep) passes as its trailing argument a named `domain.Provider` constant — `domain.ProviderCodex` or `domain.ProviderClaude`. Empty typed literals (`domain.Provider("")`), bare string casts (`domain.Provider("codex")`), and zero-value variables (e.g. `var p domain.Provider` then passing `p`) are all forbidden.
  - `grep -F 'domain.Provider("")' internal/` returns zero hits. `grep -F 'domain.Provider("' internal/` returns zero hits outside of the `domain` package's own tests (this bars ad-hoc string-literal casts at call sites; the constant definitions in `internal/domain` are the only permitted origin).
  - No change yet to `Store.Bootstrap`, `Store.UpsertProjectBinding`, or the `project_bindings` DDL — unit 3.2 owns those.
- **Blocked by:** —

### Unit 3.2 — project_bindings forward-only rebuild + composite-PK upsert

- **State:** done
- **Paths:**
  - `internal/adapters/sqlite/store.go` (edit — `Bootstrap` control flow becomes: (1) existing DDL loop runs unchanged except the inline `project_bindings` DDL is rewritten to the NEW composite-PK shape — `PRIMARY KEY (project_id, provider)` — so fresh DBs land on the new shape via `CREATE TABLE IF NOT EXISTS`; (2) a NEW `BeginTx` block opens AFTER the DDL loop commits; (3) as the FIRST statement inside this new tx, read `PRAGMA user_version`; (4) if `user_version >= 1`, commit and return — migration already ran; (5) if `user_version < 1`, probe `pragma_table_info('project_bindings')` and inspect the `pk` column across the returned rows to determine shape — a single row with `pk = 1` on `project_id` alone (and `pk = 0` on `provider`) indicates the LEGACY single-column PK shape; two rows both with `pk > 0` (one on `project_id`, one on `provider`) indicate the NEW composite PK shape. This is more robust than string-matching `sqlite_master.sql` because `pragma_table_info` returns structured column metadata unaffected by CREATE TABLE whitespace, column-order permutations, comment noise, or case variation; (6) if legacy-shaped, run the copy-rename rebuild — `CREATE TABLE project_bindings_new` with the new composite-PK shape, `INSERT INTO project_bindings_new SELECT project_id, profile_id, provider, created_at, modified_at FROM project_bindings`, `DROP TABLE project_bindings`, `ALTER TABLE project_bindings_new RENAME TO project_bindings`; (7) if fresh-shaped (probe sees composite PK), skip the rebuild; (8) `PRAGMA user_version = 1`; (9) commit. Rewrite `UpsertProjectBinding` `ON CONFLICT` target to `(project_id, provider)`.)
  - `internal/adapters/sqlite/store_test.go` (edit — add `TestStoreCompositeBindingsCoexistByProvider`, `TestStoreMigrationPreservesLegacyCodexBinding`, and `TestStoreBootstrapIsIdempotentAfterMigration`; existing lifecycle/FK tests unaffected)
- **Packages:** `internal/adapters/sqlite` (real edit — migration + DDL + upsert)
- **Acceptance:**
  - `PRAGMA user_version` on a freshly `Bootstrap`-ed DB returns `1`.
  - `git diff` on `internal/adapters/sqlite/store.go` shows the `PRAGMA user_version` read is the FIRST executed statement inside a `BeginTx` block that also performs the rebuild and sets `user_version = 1` — NOT read before `BeginTx`. Concurrent-Bootstrap race mitigation is observable in the diff: no read of `user_version` outside the migration transaction.
  - `TestStoreCompositeBindingsCoexistByProvider` — one project, two profiles (Codex + Claude), two bindings for the same `project_id` with different `provider` values both succeed; `BindingByProjectID(ctx, projectID, ProviderCodex)` returns the Codex binding and `BindingByProjectID(ctx, projectID, ProviderClaude)` returns the Claude binding.
  - `TestStoreMigrationPreservesLegacyCodexBinding` — set up a raw `*sql.DB` handle via `sqlite.Open` (connection-level `foreign_keys = ON`), then (a) `CREATE TABLE projects` + `CREATE TABLE profiles` with the production DDL, (b) `INSERT` one `projects` row (`id / root / name / created_at`) and one `profiles` row (`id / provider='codex' / name / home_path / created_at`) as FK prerequisites, (c) `CREATE TABLE project_bindings` with the LEGACY shape (`project_id TEXT PRIMARY KEY`, `profile_id TEXT NOT NULL`, `provider TEXT NOT NULL`, `created_at TEXT NOT NULL`, `modified_at TEXT NOT NULL`, plus both FKs), (d) `INSERT` one Codex binding with all five columns (`project_id`, `profile_id`, `provider='codex'`, fixed `created_at`, fixed `modified_at`) matching the seeded project + profile rows, (e) confirm `PRAGMA user_version` is `0`, (f) run `Store.Bootstrap`; assert `BindingByProjectID(ctx, legacyProjectID, ProviderCodex)` returns the same `profile_id`, original `created_at`, and original `modified_at` unchanged. (FK-prerequisite approach (a) from the `projects` + `profiles` pre-seed — mirrors the existing lifecycle-test style, keeps `foreign_keys = ON` throughout, no pragma toggle needed.)
  - `TestStoreBootstrapIsIdempotentAfterMigration` — after the legacy-preservation test's Bootstrap completes (`user_version = 1`, legacy row preserved in the rebuilt table), run `Store.Bootstrap` a SECOND time. Observable proof that the rebuild did NOT re-run: (i) query `sqlite_master` for a table named `project_bindings_new` and assert it does NOT exist (the staging table is renamed away during the first rebuild and never created during a no-op second call); (ii) `BindingByProjectID(ctx, legacyProjectID, ProviderCodex)` returns the same `profile_id` + `created_at` + `modified_at` as after Bootstrap #1 — no duplication, no data loss, no mutation; (iii) `COUNT(*)` on `project_bindings` equals 1 after Bootstrap #2.
  - `TestStoreForeignKeysRejectInvalidBindings` still fails to insert a binding with non-existent project_id + profile_id (FK enforcement survives the rebuild).
  - `mage testPkg ./internal/adapters/sqlite` passes with `-race -cover` and coverage ≥ 70% per AGENTS.md § 11.
- **Blocked by:** 3.1

**Unit ordering + blocking rationale:**

- 3.1 lands the interface-signature change + call-site threading as one atomic unit because a partial change would leave `sqlite.Store` mismatching `domain.BindingRepository`, breaking every consumer-side `Store` embedding transitively.
- 3.2 adds the migration + DDL + upsert `ON CONFLICT` rewrite entirely within `internal/adapters/sqlite` + its test file. Package-lock rule forbids parallel edits on the same package — 3.2 `blocked_by: 3.1` is mandatory, not negotiable.
- Both units pass Phase 6 drop-end verification with plain `mage test` (plus the default `-race -cover` that mage uses). No `mage integration` diff; no Docker-backed changes.

**No-touch scope-guard list:**

- `internal/adapters/providers/claude/**` — forbidden (DROP_5).
- `internal/services/claude/**` — forbidden (DROP_6).
- `internal/services/images/**` — Claude resolver is DROP_4 scope.
- `internal/cli/claude.go` + any `valv claude` command registration — DROP_6.
- `internal/cli/account_auth.go` ProviderClaude branches — already landed in DROP_2; no re-edit in DROP_3.

## Notes

- DROP_3 routes `domain.ProviderCodex` through every call site for today. Threading a real `provider domain.Provider` from CLI (`valv codex` / future `valv claude`) into `Status` / `resolveBinding` is DROP_6 / DROP_7 scope — DROP_3 is a domain-plumbing-safe rebuild, not a UX change.
- Migration is forward-only (`user_version` 0→1). No rollback path; matches §6.2a line 221 + dev "just get this mvp done" directive.
- Focus-plan §7 item 5 (destructive recreate vs forward-only rebuild) defaults to forward-only rebuild; dev can override in Phase 3 discuss.
- `modernc.org/sqlite` pragma passthrough is standard — no driver-specific quirks expected.
- Unknown routed to orchestrator: builder must verify `mage testPkg ./internal/adapters/sqlite` coverage clears the 70% floor after 3.2 lands. (The `user_version`-timing question from Round 1 is resolved — the read sits inside `BeginTx` per Unit 3.2's paths-edit description; no remaining ambiguity on the race.)
