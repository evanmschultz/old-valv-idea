## Unit 3.1 — Round 1

- **Reviewer:** go-qa-proof-agent
- **Commit under review:** 2ad62a4 feat(drop-3): thread provider arg through BindingByProjectID
- **Verdict:** pass

### Acceptance criteria verification

- **`BindingByProjectID` three-arg domain signature.** `internal/domain/repository.go:22` reads `BindingByProjectID(context.Context, string, Provider) (ProjectBinding, error)`. Diff hunk confirms the trailing `Provider` arg addition. Pass.
- **Adapter signature + composite-key SELECT filter.** `internal/adapters/sqlite/store.go:330-336`:
  ```go
  func (s *Store) BindingByProjectID(ctx context.Context, projectID string, provider domain.Provider) (domain.ProjectBinding, error) {
      row := s.db.QueryRowContext(
          ctx,
          `SELECT project_id, profile_id, provider, created_at, modified_at FROM project_bindings WHERE project_id = ? AND provider = ?`,
          projectID,
          string(provider),
      )
  ```
  Matches spec — added `AND provider = ?` filter with `string(provider)` bound. Pass.
- **9 call sites thread `domain.ProviderCodex`.** Verified by grep on `BindingByProjectID` under `internal/` + per-file read of each diff hunk:
  - `internal/adapters/sqlite/store_test.go:82` — `store.BindingByProjectID(..., project.ID, domain.ProviderCodex)`
  - `internal/adapters/sqlite/store_test.go:178` — `store.BindingByProjectID(..., "missing", domain.ProviderCodex)`
  - `internal/services/codex/service.go:216` — `s.store.BindingByProjectID(ctx, projectRecord.ID, domain.ProviderCodex)`
  - `internal/services/codex/service_test.go:76` — `fakeStore.BindingByProjectID` signature gained `domain.Provider` parameter
  - `internal/services/manage/service.go:240` — `s.store.BindingByProjectID(ctx, projectRecord.ID, domain.ProviderCodex)`
  - `internal/services/manage/service_test.go:411` — `store.BindingByProjectID(..., storedProject.ID, domain.ProviderCodex)`
  - `internal/cli/manage_test.go:79` — `store.BindingByProjectID(..., project.ID, domain.ProviderCodex)`
  - `internal/cli/codex_setup_test.go:68` — `store.BindingByProjectID(..., project.ID, domain.ProviderCodex)`
  - `internal/domain/repository.go:22` — interface declaration (3-arg form).
  No bare-string casts, no empty-literal casts, no zero-value variable passes. Pass.
- **DDL untouched (Unit 3.2 scope).** `git show 2ad62a4 -- internal/adapters/sqlite/store.go` shows a single 5-line hunk on the `BindingByProjectID` method only. Bootstrap's `project_bindings` DDL at lines 57-65 retains legacy `project_id TEXT PRIMARY KEY`. `UpsertProjectBinding` at lines 310-328 retains `ON CONFLICT(project_id)`. Pass.
- **Mage testPkg re-run on all 5 acceptance packages** (this reviewer re-ran; coverage gate is mage-enforced "Minimum package coverage: 60.0%" — AGENTS.md § 11 70% floor was cleared separately by every package):
  - `mage testPkg ./internal/domain` — 26 tests pass, 85.2% coverage.
  - `mage testPkg ./internal/adapters/sqlite` — 16 tests pass, 79.2% coverage. All four named acceptance tests (`TestStoreProfileAndBindingLifecycle`, `TestStoreNotFoundErrors`, `TestStoreListsProjectsAndBindings`, `TestStoreForeignKeysRejectInvalidBindings`) green.
  - `mage testPkg ./internal/services/codex` — 11 tests pass, 75.2% coverage. `fakeStore` compiles with new 3-param signature.
  - `mage testPkg ./internal/services/manage` — 23 tests pass, 76.4% coverage.
  - `mage testPkg ./internal/cli` — 101 tests pass, 72.0% coverage.
  Pass.
- **Grep acceptance check 1** (`grep -F 'domain.Provider("")' internal/`) — zero hits. Pass.
- **Grep acceptance check 2** (`grep -F 'domain.Provider("' internal/` outside `domain` package tests) — one hit at `internal/services/globalswitch/service_test.go:100` (`domain.Provider("claude")` in `TestSwitchRejectsUnsupportedProvider`). Verified pre-existing via `git log --follow` (commit `a9dcbe6`, pre-DROP_3). Outside the 9-path Unit 3.1 scope. Routed as Unknown per builder worklog. Orchestrator has already decided out-of-scope for Unit 3.1. Does not affect verdict.
- **BUILDER_WORKLOG.md Round 1 completeness.** Files touched (9), mage targets run (6), notes (design rationale + grep results), unknowns (globalswitch routed), Hylla Feedback (`N/A — task touched only Go files whose committed state was already audited by the planner`) — all sections present.

### Certificate

- **Premises.** (1) Domain interface gains trailing `Provider` arg. (2) Adapter implements 3-arg signature with composite-key SELECT filter. (3) All 9 call sites thread `domain.ProviderCodex`. (4) DDL + Bootstrap + UpsertProjectBinding untouched (Unit 3.2 scope). (5) All five acceptance packages pass mage testPkg with coverage ≥ 70%. (6) Both grep checks pass.
- **Evidence.** `git show 2ad62a4` diff, `Read` on `internal/adapters/sqlite/store.go` lines 57-65 (DDL untouched) and lines 310-328 (Upsert untouched) and lines 330-358 (new BindingByProjectID body), `Grep BindingByProjectID` over `internal/`, `Grep domain.Provider("")` zero-hit, `Grep domain.Provider("` single pre-existing hit, and direct `mage testPkg` re-runs on all five packages.
- **Trace.** Every acceptance criterion → diff citation + file-read citation + mage-run output. No premise uncited.
- **Conclusion.** PASS. Unit 3.1 is implemented as specified; the commit is safe to proceed into Unit 3.2 (which owns the DDL rebuild + composite-PK upsert).
- **Unknowns (routed to orchestrator).** `internal/services/globalswitch/service_test.go:100` uses a pre-existing bare-string cast `domain.Provider("claude")` that violates grep acceptance check 2 when grep is run globally. Pre-existing (commit `a9dcbe6`, pre-DROP_3). Outside the 9-path Unit 3.1 scope. Orchestrator has already ruled this out-of-scope; route for DROP_2 follow-up or PLAN-acceptance clarification.

## Unit 3.2 — Round 1

- **Reviewer:** go-qa-proof-agent
- **Commit under review:** 47fd5ee feat(drop-3): migrate project_bindings to composite primary key
- **Verdict:** pass

### Acceptance criteria verification

- **Fresh-DB DDL at composite shape.** `internal/adapters/sqlite/store.go:57-66` shows `CREATE TABLE IF NOT EXISTS project_bindings` with inline `PRIMARY KEY (project_id, provider)` on line 63, two FK clauses preserved on lines 64-65. No single-column `project_id TEXT PRIMARY KEY`. A fresh DB lands directly on the composite shape via this `CREATE TABLE IF NOT EXISTS`. Pass.
- **Migration-tx shape.** `git show 47fd5ee -- internal/adapters/sqlite/store.go` evidence:
  - Lines 93-113 in `Bootstrap`: existing DDL-loop `BeginTx` / `Commit` block unchanged except the inlined `project_bindings` DDL swap (acceptance 1), then line 112 `return s.migrateProjectBindings(ctx)`.
  - Lines 115-172 new `migrateProjectBindings` method: line 116 opens a SECOND `BeginTx`, line 127 `tx.QueryRowContext(ctx, ` + "`PRAGMA user_version`" + `).Scan(&userVersion)` is the first executed statement inside the tx (only the `defer` rollback sits between `BeginTx` and the read — no other SQL).
  - `grep -n user_version internal/adapters/sqlite/store.go` returns exactly two hits (lines 127 and 165), both `tx.*Context` calls inside `migrateProjectBindings`. No production read of `user_version` outside the migration transaction — matches PLAN.md §Unit 3.2 line 74. Pass.
- **Branch on user_version.** `store.go:130-170`:
  - Lines 130-134: `if userVersion >= 1 { commit; return nil }` — short-circuits before probe or rebuild.
  - Line 137: `isLegacyProjectBindingsShape(ctx, tx)` probe via `SELECT name, pk FROM pragma_table_info('project_bindings')` (helper at lines 177-208) — inspects the `pk` column across the returned rows: `projectIDPK == 1 && providerPK == 0` returns `true` (legacy single-column PK on project_id), composite `pk>0` on both returns `false`. Structurally robust against whitespace / column-order / comment variants in past DDL.
  - Lines 141-163: if legacy, runs the four-statement rebuild — `CREATE TABLE project_bindings_new` with composite PK and both FKs, `INSERT INTO project_bindings_new (…) SELECT project_id, profile_id, provider, created_at, modified_at FROM project_bindings`, `DROP TABLE project_bindings`, `ALTER TABLE project_bindings_new RENAME TO project_bindings`. If `legacy == false` (fresh-shape composite), skips the rebuild block entirely.
  - Lines 165-170: unconditionally sets `PRAGMA user_version = 1` and commits.
  Pass.
- **`UpsertProjectBinding` ON CONFLICT target.** `store.go:407-424` — `ON CONFLICT(project_id, provider) DO UPDATE SET profile_id = excluded.profile_id, modified_at = excluded.modified_at`. Target rewritten from `(project_id)` → `(project_id, provider)`; the stale `provider = excluded.provider` SET clause is correctly removed (provider is now part of the conflict key so it cannot differ on conflict). Pass.
- **`TestStoreCompositeBindingsCoexistByProvider`** (`internal/adapters/sqlite/store_test.go:326-381`). Seeds one project + one codex profile + one claude profile, upserts two bindings against the SAME `project.ID` with different `provider` values, then fetches each via `BindingByProjectID(ctx, project.ID, ProviderCodex)` and `BindingByProjectID(ctx, project.ID, ProviderClaude)`. Asserts both bindings exist, each returns the correct `ProfileID` and `Provider`. Real behavioral assertion on composite-PK coexistence (not a compilation-only probe). Pass.
- **`TestStoreMigrationPreservesLegacyCodexBinding`** (`store_test.go:383-499`). Opens raw `*sql.DB` via `Open(OpenOptions{URI: …shared})` — `open.go:37-38` confirms `URI` DSNs attach `_pragma=foreign_keys(1)`, so FK enforcement is active throughout. Seeds LEGACY DDL (`project_id TEXT PRIMARY KEY`), pre-seeds one `projects` row and one `profiles` row as FK prerequisites, seeds one legacy `project_bindings` row with all five columns. Asserts `PRAGMA user_version == 0` pre-bootstrap, calls `Store.Bootstrap`, asserts `PRAGMA user_version == 1` post-bootstrap, asserts `BindingByProjectID(ctx, legacyProjectID, ProviderCodex)` returns the same `ProfileID` and the same `CreatedAt` / `ModifiedAt` (formatted as RFC3339Nano and compared string-equal against `"2025-01-02T03:04:05Z"`). Matches PLAN.md §Unit 3.2 acceptance verbatim, including the FK-prerequisite approach with `foreign_keys = ON` throughout. Pass.
- **`TestStoreBootstrapIsIdempotentAfterMigration`** (`store_test.go:501-629`). Same legacy seed, calls `Bootstrap` TWICE. After the second call: (i) `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='project_bindings_new'` returns 0 — the staging table was renamed away during Bootstrap #1 and is NOT re-created by Bootstrap #2 (because `user_version >= 1` short-circuits). (ii) `SELECT COUNT(*) FROM project_bindings` returns 1 — no duplication, no loss. (iii) `BindingByProjectID` returns same `ProfileID`, and `CreatedAt` / `ModifiedAt` compared via `time.Time.Equal` — no mutation across the second Bootstrap. Covers PLAN.md §Unit 3.2 acceptance line 77 clauses (i) / (ii) / (iii) exactly. Pass.
- **`TestStoreForeignKeysRejectInvalidBindings` still green** (`store_test.go:189-201`). Unchanged; pre-existing test that attempts a binding insert with non-existent project_id + profile_id and expects an FK failure. Re-run via `mage test` shows 319/319 tests pass, this test included — FK enforcement survives the rebuild. Pass.
- **`mage testPkg ./internal/adapters/sqlite` (re-run by reviewer).** 19 tests pass, 0 failed, 0 skipped; coverage 78.5%; runs `-count=1 -race -cover`; clears the 70% AGENTS.md § 11 coverage floor for this package. Pass.
- **`mage test` (re-run by reviewer).** 319 tests pass across 18 packages, 0 failed, 0 skipped; minimum package coverage 60.0% (mage gate) met by every package; the four consumer packages potentially affected by the ON CONFLICT change (`services/codex` 75.2%, `services/manage` 76.4%, `cli` 72.0%, `adapters/sqlite` 78.5%) all green. No other-package regressions. Pass.
- **BUILDER_WORKLOG.md Unit 3.2 Round 1 entry completeness.** Covers Builder, Started, Files touched, Mage targets run with coverage numbers, Design notes (migration-tx ordering, FK-on rebuild rationale, `pragma_table_info` robustness rationale), Unknowns (none), Hylla Feedback (N/A with justification). Complete. Pass.

### Certificate

- **Premises.** (1) Fresh-DB DDL loop lands `project_bindings` at composite PK. (2) Migration tx opens AFTER the DDL-loop tx commits, with `PRAGMA user_version` as first statement. (3) Branch on user_version: `>= 1` commits and returns; `< 1` probes `pragma_table_info`, runs copy-rename rebuild if legacy-shaped else skips, sets `user_version = 1`, commits. (4) `UpsertProjectBinding` ON CONFLICT target is `(project_id, provider)`. (5) Three new tests exist and assert real behavior matching PLAN acceptance (coexistence, legacy preservation with `user_version` 0→1 + timestamp equality, idempotence with staging-table absence + COUNT=1 + no-mutation). (6) `TestStoreForeignKeysRejectInvalidBindings` still passes unchanged. (7) `mage testPkg ./internal/adapters/sqlite` passes with `-race -cover` at ≥ 70% coverage. (8) `mage test` passes with no other-package regressions. (9) BUILDER_WORKLOG entry complete.
- **Evidence.** `git show 47fd5ee --stat`, `git show 47fd5ee -- internal/adapters/sqlite/store.go`, `git show 47fd5ee -- internal/adapters/sqlite/store_test.go`, `Read` on `store.go` lines 40-208 (Bootstrap + migrateProjectBindings + isLegacyProjectBindingsShape) and lines 407-455 (UpsertProjectBinding + BindingByProjectID), `Read` on `store_test.go` lines 1-629 (full test file), `Read` on `open.go:14-51` (URI DSN attaches `foreign_keys(1)` — confirms FK enforcement during legacy-preservation/idempotence tests), `Grep user_version internal/adapters/sqlite/store.go` returning exactly the two in-migration-tx hits, reviewer-run `mage testPkg ./internal/adapters/sqlite` (19/19 pass, 78.5% cover) and `mage test` (319/319 pass, 18 packages, all ≥ 60% mage floor and ≥ 70% AGENTS.md § 11 floor for sqlite).
- **Trace.** Every PLAN.md §Unit 3.2 acceptance bullet (lines 73-79) → specific line range in `store.go` or `store_test.go` → reviewer-run mage output line. No premise uncited. Migration-tx ordering trace: `Bootstrap` line 93 BeginTx#1 → line 108 Commit#1 → line 112 call → `migrateProjectBindings` line 116 BeginTx#2 → line 127 `PRAGMA user_version` read → line 130 branch → line 137 legacy probe → lines 141-163 conditional rebuild → line 165 `PRAGMA user_version = 1` → line 168 Commit#2. Idempotence trace: Bootstrap #2 re-enters `migrateProjectBindings`, line 127 reads user_version=1, line 130 branch short-circuits to commit+return on line 131-134 — probe + rebuild paths are dead.
- **Conclusion.** PASS. Unit 3.2 implements the forward-only composite-PK migration exactly as PLAN.md §Unit 3.2 specifies.
- **Unknowns (routed to orchestrator).** None.
