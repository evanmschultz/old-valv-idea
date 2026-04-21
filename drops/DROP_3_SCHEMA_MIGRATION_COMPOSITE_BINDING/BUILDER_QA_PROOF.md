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
