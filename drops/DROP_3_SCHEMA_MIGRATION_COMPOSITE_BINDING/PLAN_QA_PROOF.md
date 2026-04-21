# PLAN QA PROOF — DROP_3 SCHEMA MIGRATION COMPOSITE BINDING

**Round:** 1
**Verdict:** pass (with one advisory finding on migration control-flow precision)
**Reviewer role:** proof-oriented plan-QA

## 1. Evidence Audit

Every file-path + line-number claim in the Planner's "Committed-state audit" section was independently re-verified against the committed tree at `/Users/evanschultz/Documents/Code/hylla/valv/main/`:

- 1.1 `internal/adapters/sqlite/store.go:57-65` — `project_bindings` DDL `(project_id TEXT PRIMARY KEY, profile_id TEXT NOT NULL, provider TEXT NOT NULL, created_at TEXT NOT NULL, modified_at TEXT NOT NULL, FK project_id → projects.id, FK profile_id → profiles.id)`. Verified line-exact. `UpsertProjectBinding` at line 310, `ON CONFLICT(project_id)` at line 315. PLAN cites "line 315" for the ON CONFLICT clause — exact.
- 1.2 `internal/adapters/sqlite/open.go` is 73 lines total. Only connection-level pragma is `foreign_keys(1)` (lines 38, 47). No `PRAGMA user_version` handling. Confirmed via full Read of open.go + grep across `main/` for `PRAGMA user_version` — only matches in drop and focus-plan markdown, zero in production code.
- 1.3 `internal/domain/repository.go:20-24` defines `BindingRepository`. Line 22 is `BindingByProjectID(context.Context, string) (ProjectBinding, error)`. Exact.
- 1.4 `internal/domain/model.go:29-35` — `ProjectBinding` struct already has `Provider Provider` at line 32. No domain-type change required; PLAN correctly flags this.
- 1.5 Consumer-side embeddings: `internal/services/codex/service.go:25` (inside `Store` interface at line 23) and `internal/services/manage/service.go:22` (inside `Store` interface at line 20) embed `domain.BindingRepository`. Two direct embeddings. `internal/services/globalswitch/service.go` has its own narrow `Store` interface (only `ProfileByName`) and does **not** embed `BindingRepository` — correctly out of scope.
- 1.6 Production callers: `internal/services/codex/service.go:216` (`resolveBinding`), `internal/services/manage/service.go:240` (`Status`). Exact.
- 1.7 Test-double + test callers: `internal/adapters/sqlite/store_test.go:82, 178`; `internal/services/manage/service_test.go:411`; `internal/cli/manage_test.go:79`; `internal/cli/codex_setup_test.go:68`; `internal/services/codex/service_test.go:76` (`fakeStore.BindingByProjectID`). Every line number matches committed code exactly.
- 1.8 Grep `BindingByProjectID` across `internal/**/*.go` returns exactly 10 hits across 9 distinct files. Unit 3.1's path list enumerates all 9 files. No call site missed.
- 1.9 Hylla corroboration: `hylla_search_keyword` on `BindingByProjectID` returns the `BindingRepository` interface block + `Store.BindingByProjectID` method block with `file_path` fields matching the Planner's cites. Consistent.

**Evidence verdict:** every cite is grounded. No stale, wrong, or unsupported line numbers.

## 2. Unit 3.1 Scope — Call-Site Coverage

- 2.1 Prompt focus-area notes "5 consumer-side embeddings that transitively depend on `domain.BindingRepository`". I could find only **two** direct Go interface embeddings (`codex.Store`, `manage.Store`). The remaining coverage is via structural implementations — `fakeStore` in `codex/service_test.go` is the one `BindingByProjectID` method implementation outside `sqlite.Store`. The prompt's count appears imprecise; it does not translate to a PLAN finding because the PLAN's 9-path edit list covers every actual call site and every actual method implementation regardless of the headline count.
- 2.2 PLAN 3.1 correctly identifies that the consumer-side `Store` interfaces do not need direct edits — Go embedded-interface semantics propagate the `domain.BindingRepository` method signature change automatically. Verified by inspecting `codex.Store` / `manage.Store` declarations (no explicit `BindingByProjectID` method listed; they rely on the embed).
- 2.3 Unit 3.1 path list (9 entries) matches the 9 distinct files where `BindingByProjectID` appears, plus `internal/domain/repository.go` as the signature authority. Exhaustive.
- 2.4 Note: `VALV_CLAUDE_CODE_FOCUS_PLAN.md` §6.2a line 216 mentions `internal/cli/codex_setup.go` (not `_test.go`), but `codex_setup.go` does **not** itself call `BindingByProjectID` — only the test does. The drop PLAN correctly lists `codex_setup_test.go` and does not blindly copy the focus-plan's imprecise file reference. Good.
- 2.5 `internal/services/globalswitch/` has its own `Store` interface but scopes to `ProfileByName` only; correctly excluded from Unit 3.1 (no edit needed).

## 3. Unit 3.2 Scope — PRAGMA user_version Rebuild Pattern

- 3.1 PLAN 3.2 mechanics ("reads `PRAGMA user_version` before `BeginTx`; if `< 1`, performs the copy-rename rebuild + sets `PRAGMA user_version = 1` inside a single transaction") matches `VALV_CLAUDE_CODE_FOCUS_PLAN.md` §6.2a line 220 verbatim on intent — CREATE new → INSERT SELECT → DROP old → ALTER RENAME → `PRAGMA user_version = 1`, atomic within one `BeginTx` block, no FK toggle because no inbound FK to `project_bindings`.
- 3.2 `TestStoreCompositeBindingsCoexistByProvider` as specified proves (a) two bindings with same `project_id` but different `provider` coexist and (b) `BindingByProjectID(ctx, projectID, provider)` resolves each correctly. This matches §6.2a line 222 items (a) and (b).
- 3.3 `TestStoreMigrationPreservesLegacyCodexBinding` as specified sets up a raw DB handle with legacy-shape `project_bindings` (`project_id PRIMARY KEY` + `provider`), inserts one Codex row with specific `profile_id` + `created_at`, leaves `user_version = 0`, runs `Bootstrap`, asserts the fetched row has the same `profile_id` and original `created_at`. This proves §6.2a line 222 item (c) "migration from a Codex-only legacy row preserves `profile_id` and `created_at`" — the central migration-preservation invariant of §6.2a.
- 3.4 Test feasibility: `sqlite.Store` exposes `NewStoreFromDB(db *sql.DB) *Store` at `store.go:25`, which means the migration-preservation test can create a raw `sql.Open` handle, manually build the legacy-shape schema + insert a row + set `user_version = 0`, then wrap with `NewStoreFromDB` and invoke `Bootstrap`. Feasible with current API — no new adapter plumbing needed.
- 3.5 FK regression: PLAN 3.2 includes `TestStoreForeignKeysRejectInvalidBindings` still failing to insert a binding with non-existent project/profile after the rebuild. Good — proves FK enforcement survives the `DROP + RENAME` sequence. (Fresh-DB FKs live on the new table; legacy FKs carry across by the rebuild's DDL definition including them.)

## 4. Acceptance Criteria Verifiability

- 4.1 Unit 3.1 acceptance bullets are all yes/no-verifiable via `mage testPkg <pkg>` output + `git diff` + grep. Specifically:
  - "mage testPkg ..." passes → yes/no from exit code.
  - "every existing test name still green" → named tests enumerated.
  - "Grep `BindingByProjectID` across `internal/**/*.go` shows every call site passes a `domain.Provider`" → yes/no via grep.
  - "No change yet to `Store.Bootstrap`, `Store.UpsertProjectBinding`, or the `project_bindings` DDL" → yes/no via `git diff internal/adapters/sqlite/store.go`.
- 4.2 Unit 3.2 acceptance bullets are all yes/no-verifiable:
  - PRAGMA user_version returns 1 post-Bootstrap → single-query assertion.
  - Re-Bootstrap keeps user_version=1 and doesn't re-run rebuild → test with insert + close + reopen + assert binding survives.
  - Both new test names present and green → grep + test output.
  - FK test still green → test output.
  - `mage testPkg ./internal/adapters/sqlite` passes with 70% coverage floor → mage target already enforces this per AGENTS.md §11.
  - Migration comment content → `Read` of the committed file.
- 4.3 No vague or unfalsifiable acceptance bullet detected.

## 5. Blocked_by Ordering

- 5.1 `Unit 3.2 blocked_by: 3.1` is correct and mandatory. Both units edit `internal/adapters/sqlite/store.go` + `store_test.go`, so the package-lock rule from `main/CLAUDE.md` forbids parallelization.
- 5.2 3.1 cannot be further split into sub-units without breaking slice-boundary compile: changing `domain.BindingRepository.BindingByProjectID` signature without simultaneously updating `sqlite.Store.BindingByProjectID` and every call site leaves `sqlite.Store` unable to satisfy `domain.BindingRepository` (and every consumer-side `Store` embed transitively broken). 3.1's atomicity is load-bearing.
- 5.3 3.1 and 3.2 could be merged into one unit, but the split keeps the interface-plumbing change reviewable separately from migration logic. Reasonable decomposition; not a finding.

## 6. No-Touch Scope-Guard List vs DROP_4–DROP_7 Boundaries

Cross-checked against `main/PLAN.md` rows 26-31:

- 6.1 DROP_4 Claude Docker image → `internal/services/images/**` → PLAN 3 scope-guard explicitly calls this out (line 90: "Claude resolver is DROP_4 scope"). ✓
- 6.2 DROP_5 Claude provider adapter → `internal/adapters/providers/claude/**` → scope-guard line 88 explicitly forbids (DROP_5). ✓
- 6.3 DROP_6 Claude service + CLI → `internal/services/claude/**` (scope-guard line 89) + `internal/cli/claude.go` (scope-guard line 91) → both explicitly forbidden (DROP_6). ✓
- 6.4 DROP_7 account surface parity → `internal/tui/manage/picker.go` + `valv account` surface are not explicitly in the no-touch list, but DROP_3's actual path list does not touch any of those files, so the scope is implicitly clean. Minor: an explicit scope-guard bullet for DROP_7 surfaces would harden intent but is not required.
- 6.5 `internal/cli/account_auth.go` ProviderClaude branches → scope-guard line 92 correctly notes "already landed in DROP_2; no re-edit in DROP_3". ✓

## 7. Package-Lock Discipline

- 7.1 Unit 3.1 edits packages `internal/domain`, `internal/adapters/sqlite`, `internal/services/codex`, `internal/services/manage`, `internal/cli`. Unit 3.2 edits `internal/adapters/sqlite`.
- 7.2 Both units edit `internal/adapters/sqlite/store.go` + `internal/adapters/sqlite/store_test.go`. Per `main/CLAUDE.md`'s implicit package-lock rule ("two builder units cannot parallelize if they edit the same Go package"), 3.2 must be strictly sequential after 3.1. PLAN sets `3.2 blocked_by: 3.1`, respecting the rule. ✓

## 8. Finding (Advisory, Non-Blocking)

- 8.1 **Migration control-flow precision on fresh DB vs legacy DB at `user_version = 0`.** A fresh database starts at `user_version = 0`. Unit 3.2's bullet says "if `< 1`, performs the copy-rename rebuild". But on a fresh DB, `project_bindings` may not yet exist (Bootstrap's own `CREATE TABLE IF NOT EXISTS` runs as part of the same `statements` slice), and the rebuild's `INSERT SELECT FROM project_bindings` / `DROP TABLE project_bindings` sequence depends on the old-shape table existing. PLAN leaves the sequencing choice under-specified:
  - Option A: CREATE TABLE IF NOT EXISTS (new shape) first, then on user_version<1 inspect `sqlite_master` to check whether the table is new-shape already and skip the rebuild; set `user_version = 1` regardless.
  - Option B: CREATE TABLE IF NOT EXISTS (new shape) first, then on user_version<1 unconditionally run the rebuild — the CREATE-new-from-new is valid, INSERT SELECT reads zero rows on a fresh DB, DROP empties a new-shape table, RENAME yields the same new-shape table; correct but wasteful.
  - Option C: On user_version<1, probe `sqlite_master` for the legacy-shape signature (`project_id TEXT PRIMARY KEY` without a composite constraint) and branch on that.
  - The builder can choose in implementation, but the PLAN would be sharper if it names the chosen approach and states the freshness-detection invariant explicitly. This is a precision gap, not a correctness gap.

## 9. TL;DR

- T1 Every line-number cite in the Planner's committed-state audit matches committed code exactly; all 10 `BindingByProjectID` hits across 9 files are covered by Unit 3.1's path list.
- T2 Unit 3.1's 9-path edit list is exhaustive; the prompt's "5 consumer-side embeddings" count is loose (there are 2 direct embeddings plus 1 fakeStore implementation) but does not translate to a PLAN coverage gap.
- T3 Unit 3.2's PRAGMA user_version rebuild pattern + two new test cases precisely encode §6.2a line 220 mechanics and line 222 invariants; `NewStoreFromDB` makes the legacy-shape raw-DB test feasible.
- T4 All acceptance criteria are yes/no-verifiable from `mage testPkg` output + `git diff` + grep; no vague bullets detected.
- T5 `3.2 blocked_by: 3.1` is mandatory and correct; the package-lock rule on `internal/adapters/sqlite` forbids parallelization.
- T6 No-touch scope-guard list correctly draws DROP_4 / DROP_5 / DROP_6 boundaries; DROP_7 surfaces are not in scope by omission, which is acceptable but could be hardened with an explicit bullet.
- T7 Package-lock discipline is respected — 3.1 and 3.2 share `internal/adapters/sqlite`, sequenced accordingly.
- T8 One advisory finding: migration control-flow sequencing on fresh DB vs legacy DB at `user_version = 0` could be stated more precisely (Options A/B/C outlined above). Non-blocking — builder can resolve in Phase 4.

**Pass.**
