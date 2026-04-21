# DROP_3 — PLAN_QA_PROOF — Round 2

**Drop:** DROP_3_SCHEMA_MIGRATION_COMPOSITE_BINDING
**Round:** 2
**Verdict:** PASS
**Role:** proof-oriented plan-QA
**Plan commit under review:** `78fcb7f docs(drop-3): planner revise round 2 per plan qa findings`

## Headline

Round 2 revisions cleanly address all six Round 1 findings (F1 race, F2 gameable grep, F3 gameable re-run, F4 non-load-bearing comment, F5 underspec test setup, F6 fresh-vs-legacy control flow) with no structural regressions; audit claim about `projects`/`profiles` DDL verified against `internal/adapters/sqlite/store.go:43-56`.

## 1. Finding-by-Finding Verification

- 1.1 **F1 (race — `PRAGMA user_version` inside `BeginTx`) — FIXED.** Unit 3.2 paths description (`PLAN.md:69`) step (2) puts `BeginTx` AFTER the DDL loop commit; step (3) makes the `user_version` read the FIRST statement inside that tx. Acceptance bullet `PLAN.md:74` makes this observable via `git diff`: "the `PRAGMA user_version` read is the FIRST executed statement inside a `BeginTx` block that also performs the rebuild and sets `user_version = 1` — NOT read before `BeginTx`." Yes/no-verifiable. Concurrent-Bootstrap race narrowed to intra-tx `BEGIN IMMEDIATE` serialization, which is SQLite's documented behavior.
- 1.2 **F2 (gameable grep — named-constant enforcement) — FIXED.** Unit 3.1 acceptance now has two distinct bullets. `PLAN.md:60` requires every `BindingByProjectID` call site's trailing arg be a named `domain.Provider` constant (not `domain.Provider("")` and not a bare string cast). `PLAN.md:61` nails down two grep patterns: `grep -F 'domain.Provider("")' internal/` zero hits, `grep -F 'domain.Provider("' internal/` zero hits outside `internal/domain`'s own tests. Both patterns are precise, `-F`-flagged, and gofumpt-compatible (gofumpt strips inside-paren whitespace, so `domain.Provider( "")` can't slip through).
- 1.3 **F3 (gameable re-run — observable idempotence) — FIXED.** `PLAN.md:77` introduces `TestStoreBootstrapIsIdempotentAfterMigration` with three observable post-conditions after Bootstrap #2: (i) `sqlite_master` has no `project_bindings_new` (staging table gone); (ii) `BindingByProjectID` returns identical `profile_id` + `created_at` + `modified_at` to post-Bootstrap-#1; (iii) `COUNT(*)` on `project_bindings` equals 1. Each is yes/no-checkable; none are satisfiable by a no-op `mage test` that skips the scenario.
- 1.4 **F4 (non-load-bearing comment) — FIXED.** The prior "migration-comment-present" acceptance bullet has been removed from Unit 3.2 (`PLAN.md:73–79`). No grep-for-comment acceptance remains.
- 1.5 **F5 (underspec test setup — legacy preservation) — FIXED.** `PLAN.md:76` enumerates the complete raw-DB setup: (a) manual `CREATE TABLE projects` + `CREATE TABLE profiles` with production DDL; (b) INSERT one projects row (`id / root / name / created_at`) + one profiles row (`id / provider='codex' / name / home_path / created_at`) as FK prerequisites; (c) manual `CREATE TABLE project_bindings` with LEGACY shape (`project_id TEXT PRIMARY KEY`, `profile_id`, `provider`, `created_at`, `modified_at`, both FKs); (d) INSERT one Codex binding with all five columns; (e) verify `user_version = 0` pre-Bootstrap; (f) run Bootstrap, assert preservation. FK prerequisites for `projects` + `profiles` are explicit, `foreign_keys = ON` stays on throughout (no pragma toggle needed — the explicit note "mirrors the existing lifecycle-test style, keeps `foreign_keys = ON` throughout, no pragma toggle needed" matches SQLite's rule that FK pragma can change inside a tx-free scope).
- 1.6 **F6 (fresh-vs-legacy control flow) — FIXED.** `PLAN.md:69` step (1) rewrites the inline `project_bindings` DDL to the NEW composite-PK shape so fresh DBs land on the new shape via `CREATE TABLE IF NOT EXISTS`. Steps (5)–(7) probe `sqlite_master.sql` and branch: legacy-shaped (single-column `project_id PRIMARY KEY`) → run rebuild; fresh-shaped (composite PK visible) → skip rebuild. Both paths reach step (8) `PRAGMA user_version = 1` and commit. Fresh-DB path no longer executes a no-op copy-rename on an already-new-shape table.

## 2. Evidence-Grounding Spot-Check

- 2.1 **Audit line 31 factual claim verified.** The claim that `projects` has required cols `id / root / name / created_at` and `profiles` has required cols `id / provider / name / home_path / created_at` + `UNIQUE(provider, name)` matches `/Users/evanschultz/Documents/Code/hylla/valv/main/internal/adapters/sqlite/store.go:43-56` exactly. `projects` DDL lines 43–48 has the 4 cols (no extras); `profiles` DDL lines 49–56 has the 5 cols plus the `UNIQUE(provider, name)` constraint. The FK-prerequisite seeding in `TestStoreMigrationPreservesLegacyCodexBinding` (`PLAN.md:76`) will compile against these shapes.
- 2.2 **No other factual claims added in Round 2 that weren't grounded in Round 1.** The rewrite did not introduce new code-shape assertions beyond what the Round 1 audit already established.

## 3. Structural-Invariant Check

- 3.1 **Unit IDs preserved.** `3.1` (`PLAN.md:35`) and `3.2` (`PLAN.md:65`) unchanged.
- 3.2 **`blocked_by` edges preserved.** Unit 3.1 `blocked_by: —` (`PLAN.md:63`); Unit 3.2 `blocked_by: 3.1` (`PLAN.md:80`). DAG intact.
- 3.3 **Scope-guard list preserved.** Five items (`PLAN.md:88–94`): Claude adapter (DROP_5), Claude services (DROP_6), images Claude resolver (DROP_4), CLI `claude` command (DROP_6), `account_auth.go` ProviderClaude branches (DROP_2 already landed). Unchanged from Round 1.
- 3.4 **Notes section preserved and updated.** Seven items (`PLAN.md:96–102`). Prior Round 1 "Unknown routed to orchestrator" line is updated with a parenthetical noting the `user_version`-timing race question is resolved — correct reflection of the F1 fix.

## 4. Falsification Passes Attempted (All Mitigated)

- 4.1 **Could F1's acceptance be satisfied by a diff where `BeginTx` wraps only the rebuild but not the `user_version` read?** No — `PLAN.md:74` explicitly requires "the `PRAGMA user_version` read is the FIRST executed statement inside a `BeginTx` block that also performs the rebuild and sets `user_version = 1`." The diff-check is explicit about both the tx-membership AND the statement ordering.
- 4.2 **Could F2's grep miss a whitespace-inserted `domain.Provider( "")` form?** No — gofumpt (enforced by `mage test` per `main/CLAUDE.md` § "Build Verification") strips inside-paren whitespace, so the builder cannot land a formatted diff containing `domain.Provider( "...")`. The `grep -F` patterns are tight for any gofumpt-formatted Go.
- 4.3 **Could F3's staging-table check mismatch the actual rebuild name?** No — `PLAN.md:69` step (6) names the staging table `project_bindings_new` and acceptance `PLAN.md:77` checks for that exact name. If the builder renames the staging table, the paths-edit description would mismatch and build-QA would catch it.
- 4.4 **Could F5's raw-DB test be satisfied without actually exercising the rebuild?** No — step (e) requires `PRAGMA user_version = 0` pre-Bootstrap (confirming no migration yet), step (f) runs Bootstrap (which per Unit 3.2 paths observes `user_version < 1`, probes legacy shape, runs rebuild, sets `user_version = 1`). Assertions in step (f) check `profile_id` + both timestamps are unchanged, which exercises the rebuild's INSERT SELECT column copy.
- 4.5 **Could F6's fresh-DB path mistakenly run the rebuild on a freshly-created new-shape table?** No — `PLAN.md:69` step (7) "if fresh-shaped (probe sees composite PK), skip the rebuild" explicitly handles the fresh case. Step (8) still sets `user_version = 1` so subsequent Bootstraps take the fast path at step (4).
- 4.6 **Could the audit-line-31 factual claim be wrong in a way I missed?** Cross-checked directly against store.go lines 43–56: no missing columns, no extra columns, `UNIQUE(provider, name)` present. Claim is tight.
- 4.7 **Did the Round 2 revision drop a scope-guard item?** No — five items still present covering the same DROP_2/4/5/6 boundaries.
- 4.8 **Is `PLAN.md:102`'s "Unknown resolved" claim overclaiming?** No — Unit 3.2 paths step (3) + acceptance line 74 together pin the `user_version` read inside the tx, which exactly addresses the Round 1 F1 race concern.

## 5. Summary

Round 2 revisions address all six Round 1 findings with evidence-grounded, yes/no-verifiable acceptance criteria. No structural regressions to unit IDs, `blocked_by` edges, scope-guards, or Notes. The committed-state audit's factual claim about `projects` / `profiles` DDL verifies against `internal/adapters/sqlite/store.go:43-56`. No unmitigated falsification attack. The single residual Unknown (`mage testPkg ./internal/adapters/sqlite` coverage clearing the 70% floor after 3.2 lands) is properly routed to the builder as a post-build verification gate per `PLAN.md:102`.

**Verdict: PASS. Plan is ready for Phase 4 (build).**

## TL;DR

- T1 F1–F6 all fixed: race mitigation observable via diff, grep acceptance uses named-constant + two negative `-F` patterns, idempotence has three observable post-conditions, non-load-bearing comment dropped, legacy-preservation test fully specifies DDL + FK pre-seed + all five legacy cols, fresh-vs-legacy control flow explicitly branches on `sqlite_master` probe.
- T2 Audit-line-31 `projects` + `profiles` DDL claim verified against `internal/adapters/sqlite/store.go:43-56` — 4 + 5 columns respectively with `UNIQUE(provider, name)` present; FK pre-seed in `TestStoreMigrationPreservesLegacyCodexBinding` will compile.
- T3 Unit IDs (`3.1`, `3.2`), `blocked_by` edges (`—`, `3.1`), scope-guard list (5 items), and Notes section all preserved; no regressions.
- T4 Eight falsification attacks launched — all mitigated. Most concerning (F1 race observability, F2 whitespace-grep bypass, F3 staging-table name drift) neutralized by explicit acceptance wording and gofumpt enforcement.
- T5 **PASS.** Plan is ready for Phase 4 build.
