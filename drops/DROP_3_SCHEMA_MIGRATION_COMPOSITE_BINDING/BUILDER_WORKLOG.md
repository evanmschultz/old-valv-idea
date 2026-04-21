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
