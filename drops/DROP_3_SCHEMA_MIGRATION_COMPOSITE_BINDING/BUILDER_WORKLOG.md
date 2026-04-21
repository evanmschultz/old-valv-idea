# DROP_N — Builder Worklog

Append a `## Unit N.M — Round K` section per build attempt. See `main/drops/WORKFLOW.md` § "Phase 4 — Build (per unit)" for what each section should contain.

## Unit 3.1 — Round 1

- **Builder:** go-builder-agent
- **Started:** 2026-04-20
- **Files touched:**
  - `internal/domain/repository.go` — `BindingRepository.BindingByProjectID` gained trailing `Provider` argument
  - `internal/adapters/sqlite/store.go` — `Store.BindingByProjectID` signature gained `provider domain.Provider`; SELECT now `WHERE project_id = ? AND provider = ?` with `string(provider)` bound; `Bootstrap` and `UpsertProjectBinding` untouched (Unit 3.2 scope)
  - `internal/adapters/sqlite/store_test.go` — lines 82 + 178 call sites pass `domain.ProviderCodex`
  - `internal/services/codex/service.go` — line 216 call site passes `domain.ProviderCodex`; existing `binding.Provider != domain.ProviderCodex` defense-in-depth check at line 223 preserved per PLAN
  - `internal/services/codex/service_test.go` — `fakeStore.BindingByProjectID` signature gained trailing `domain.Provider` parameter
  - `internal/services/manage/service.go` — line 240 call site passes `domain.ProviderCodex`
  - `internal/services/manage/service_test.go` — line 411 call site passes `domain.ProviderCodex`
  - `internal/cli/manage_test.go` — line 79 call site passes `domain.ProviderCodex`
  - `internal/cli/codex_setup_test.go` — line 68 call site passes `domain.ProviderCodex`
- **Mage targets run:**
  - `mage testPkg ./internal/domain` — pass (26 tests, 85.2% coverage)
  - `mage testPkg ./internal/adapters/sqlite` — pass (16 tests, 79.2% coverage; all four acceptance-named tests `TestStoreProfileAndBindingLifecycle`, `TestStoreNotFoundErrors`, `TestStoreListsProjectsAndBindings`, `TestStoreForeignKeysRejectInvalidBindings` green)
  - `mage testPkg ./internal/services/codex` — pass (11 tests, 75.2% coverage; `fakeStore` compiles with new 3-param signature)
  - `mage testPkg ./internal/services/manage` — pass (23 tests, 76.4% coverage)
  - `mage testPkg ./internal/cli` — pass (101 tests, 72.0% coverage)
  - `mage test` — pass (316 tests across 18 packages, all package coverage ≥ 60% floor)
- **Notes:**
  - Signature change is purely additive at call sites — every call passes `domain.ProviderCodex` per DROP_3 plan §Notes ("DROP_3 routes `domain.ProviderCodex` through every call site for today"). No design judgment beyond the mechanical thread-through.
  - The SELECT filter change (`WHERE project_id = ?` → `WHERE project_id = ? AND provider = ?`) is compatible with the legacy single-PK schema that Unit 3.2 will rebuild: with only one binding per `project_id` pre-migration and `provider='codex'` already populated in existing rows, the added `AND provider = ?` filter still matches when callers pass `domain.ProviderCodex`. `TestStoreForeignKeysRejectInvalidBindings` still green, confirming FK-rejection path is unaffected.
  - Acceptance grep check 1 — `domain.Provider("")` zero-hit rule — passes: no matches anywhere under `internal/`.
  - Acceptance grep check 2 — `domain.Provider("` zero-hit-outside-domain-tests rule — has one pre-existing hit at `internal/services/globalswitch/service_test.go:100` (`domain.Provider("claude")` in `TestSwitchRejectsUnsupportedProvider`). This file is outside the Unit 3.1 9-path scope. Not touched in this unit. Routed to orchestrator as an Unknown.
- **Unknowns:**
  - `internal/services/globalswitch/service_test.go:100` uses a bare-string cast `domain.Provider("claude")` to exercise the unsupported-provider error path in `TestSwitchRejectsUnsupportedProvider`. Pre-existing, not introduced by Unit 3.1. Outside the 9-path list. Per the non-negotiable "Do NOT edit files outside the 9-file list without asking", not touched. Decision routed to orchestrator: either amend PLAN acceptance to scope the grep rule to the 9 paths, or open a follow-up unit to replace the bare-string cast with `domain.ProviderClaude` (which is a valid named constant per `internal/domain/value_types.go`).

## Hylla Feedback

N/A — task touched only Go files whose committed state was already audited by the planner in Unit 3.1's "Committed-state audit" section of `PLAN.md`. Builder used `Read` on the 9 scoped files (the planner's path list was exact: no surprise call sites, all line numbers matched). No Hylla queries attempted, no fallbacks needed.

## Unit 3.2 — Round 1

- **Builder:** go-builder-agent
- **Started:** 2026-04-20
- **Files touched:**
  - `internal/adapters/sqlite/store.go` — (1) DDL for `project_bindings` rewritten in the existing `Bootstrap` DDL-loop statements slice to composite-PK shape (`PRIMARY KEY (project_id, provider)`, two `FOREIGN KEY` clauses preserved); (2) new `migrateProjectBindings` method invoked at the tail of `Bootstrap` after the DDL-loop transaction commits; (3) migration opens a second `BeginTx`, reads `PRAGMA user_version` via `tx.QueryRowContext` as the FIRST executed statement inside the tx, short-circuits on `user_version >= 1`, probes `pragma_table_info('project_bindings')` via `isLegacyProjectBindingsShape` helper (inspects `name` + `pk` columns: `pk=1` on `project_id` with `pk=0` on `provider` → legacy), runs the `CREATE project_bindings_new` → `INSERT … SELECT` → `DROP project_bindings` → `ALTER … RENAME` rebuild when legacy-shaped, sets `PRAGMA user_version = 1`, commits; (4) `UpsertProjectBinding` ON CONFLICT target rewritten from `(project_id)` to `(project_id, provider)` — the stale `provider = excluded.provider` SET clause removed because provider is now part of the conflict key so it never differs on update.
  - `internal/adapters/sqlite/store_test.go` — added `TestStoreCompositeBindingsCoexistByProvider`, `TestStoreMigrationPreservesLegacyCodexBinding`, `TestStoreBootstrapIsIdempotentAfterMigration`. Legacy-preservation and idempotence tests use raw `*sql.DB` via `sqlite.Open` (connection-level `foreign_keys = ON`), pre-seed `projects` + `profiles` rows as FK prerequisites, then seed a legacy `project_bindings` row with the single-column PK shape before invoking `Store.Bootstrap`.
- **Mage targets run:**
  - `mage testPkg ./internal/adapters/sqlite` — pass (19 tests, 78.5% coverage; `-race -cover -count=1`)
  - `mage test` — pass (319 tests across 18 packages; `TestStoreForeignKeysRejectInvalidBindings` still green — FK enforcement survives the rebuild; all per-package coverages ≥ 60% floor; sqlite package 78.5% clears the 70% acceptance floor)
- **Design notes:**
  - Migration-tx shape: a second `BeginTx` is opened AFTER the DDL-loop tx commits. The very first statement inside that second tx is `PRAGMA user_version` (via `tx.QueryRowContext`). The probe, conditional rebuild (`CREATE` + `INSERT SELECT` + `DROP` + `ALTER RENAME`), and `PRAGMA user_version = 1` all run inside this same tx. Commit completes the migration atomically. Bootstrap #2 hits the `user_version >= 1` branch on its first statement and commits a no-op tx, so the idempotence test observes no staging table and no row mutation.
  - FK handling during rebuild: no tables reference `project_bindings` (FKs point FROM project_bindings TO projects/profiles), so `DROP TABLE project_bindings` does not violate any inbound FK even with `foreign_keys = ON` at the connection level. The rebuild runs entirely with FK on — no `PRAGMA foreign_keys = OFF` toggle needed. The `TestStoreMigrationPreservesLegacyCodexBinding` test explicitly keeps `foreign_keys = ON` throughout by pre-seeding `projects` + `profiles` rows that match the legacy binding's FKs, proving the rebuild survives FK enforcement.
  - Legacy-shape probe via `pragma_table_info`: structured column metadata from `SELECT name, pk FROM pragma_table_info('project_bindings')` is unaffected by whitespace, column-order permutations, or DDL comment noise, so the probe is robust across any past DDL formatting. The single short comment on `isLegacyProjectBindingsShape` documents the pk-column interpretation (pk=1 on project_id alone + pk=0 on provider = legacy; pk>0 on both = already composite).
- **Unknowns:** none routed back.

## Hylla Feedback

N/A — task touched only Go files whose committed state was already audited in the Unit 3.2 paths section of the drop `PLAN.md`. Builder used `Read` on `store.go`, `store_test.go`, `open.go`, and `internal/domain/model.go` (all committed; paths exact). No Hylla queries attempted, no fallbacks needed.
