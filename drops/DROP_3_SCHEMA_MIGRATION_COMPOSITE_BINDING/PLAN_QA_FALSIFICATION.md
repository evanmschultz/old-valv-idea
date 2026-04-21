# DROP_3 — Plan QA Falsification (Round 1)

**Verdict:** `fail` (one CONFIRMED counterexample, three WEAK acceptance bullets, one underspecified test setup — all fixable in Phase 3 brief without restructuring the unit tree)

**Headline:** `PRAGMA user_version` is read BEFORE `BeginTx` — a TOCTOU race window exists against concurrent Bootstrap. Safer design: read user_version INSIDE the transaction.

## 1. Attack coverage matrix

| # | Attack surface | Result |
|---|---|---|
| 1 | Migration correctness / concurrent Bootstrap | **CONFIRMED** race window |
| 2 | Composite-PK semantics | REFUTED |
| 3 | Legacy row preservation — `user_version = 0` default realistic? | REFUTED (plan correct) |
| 4 | Call-site threading (9 paths vs hidden 10th) | REFUTED — grep matches plan exactly |
| 5 | Interface-change blast radius via consumer-side Store interfaces | REFUTED — embed transitively |
| 6 | Acceptance-bullet gameability | **WEAKNESS** in three bullets |
| 7 | YAGNI pressure — Claude profile in 3.2 test without DefaultHostProfile | REFUTED — test is sqlite-layer, bypasses service |
| 8 | Forward-only vs destructive recreate | REFUTED — dev can override in Phase 3 |

## 2. CONFIRMED counterexamples

### 2.1 Race window: `PRAGMA user_version` read outside the transaction

- **Plan text** (PLAN.md line 67): "Bootstrap reads `PRAGMA user_version` before `BeginTx`; if `< 1`, performs the copy-rename rebuild + sets `PRAGMA user_version = 1` inside a single transaction".
- **Counterexample trace** — two `valv` processes boot the same DB concurrently:
  1. Process A reads `user_version = 0` (outside tx). Process B reads `user_version = 0` (outside tx). Both decide to rebuild.
  2. A calls `BeginTx`, acquires SQLite's write lock, runs the rebuild (CREATE new → INSERT SELECT → DROP old → ALTER RENAME), sets `user_version = 1`, commits.
  3. B's `BeginTx` blocks on A's write lock. When A commits, B's tx begins. B now runs its already-decided rebuild path: `INSERT SELECT FROM project_bindings` where the SELECT pulls from the new-shape table A created, then `DROP TABLE project_bindings_old` — but there is no `project_bindings_old` (A already dropped it). B fails with `no such table: project_bindings_old`, bubbling up to `NewStore` error.
- **Evidence:** `PRAGMA user_version` reads are transaction-aware under SQLite (https://www.sqlite.org/pragma.html#pragma_schema_version). `BeginTx` with default isolation in `modernc.org/sqlite` takes DEFERRED by default — write lock acquired on first write.
- **Blast radius:** multi-orchestrator scenarios on the same user DB, or CI parallel tests hitting `file:` URIs. Valv CLI is normally single-process, but `newBootstrappedStore` in store_test.go uses `t.Parallel()` with `cache=shared` memory DBs keyed by test name — same-named parallel runs would collide. Unlikely in practice but trivially fixable.
- **Fix:** read `PRAGMA user_version` **inside** the transaction (after `BeginTx`), before deciding to rebuild. One-line design change; no unit-tree restructure needed.
- **Routing:** Phase 3 brief → planner tweaks Unit 3.2 paths-edit description + the migration-comment invariant to read "inside BeginTx".

## 3. Weakened acceptance bullets (gameable without violating the letter)

### 3.1 Unit 3.1 — "every call site passes a `domain.Provider` value"

- **Gameable form:** a builder passes `domain.Provider("")` (empty typed literal), satisfying the grep but breaking the `WHERE provider = ?` filter.
- **Strengthening:** require the trailing argument be either `domain.ProviderCodex` or a named `domain.Provider` variable whose value is `"codex"` — NOT an empty literal. Add: `grep -F 'domain.Provider("")' internal/**/*.go` returns zero hits.

### 3.2 Unit 3.2 — "re-running `Bootstrap` ... does not re-run the rebuild"

- **Gameable form:** a builder implements the rebuild via `CREATE TABLE IF NOT EXISTS project_bindings_new` + `INSERT OR IGNORE` + `DROP TABLE IF EXISTS project_bindings`, omits the `user_version` guard entirely, and the test still passes because the second Bootstrap finds the table already at the new shape and no-ops. The `user_version` gate becomes decorative.
- **Strengthening:** confirm the second Bootstrap does NOT execute `CREATE TABLE project_bindings_new` via a `*sql.DB` wrapper that counts statements, or an observable side-effect assertion.

### 3.3 Unit 3.2 — "Migration comment in `Bootstrap` calls out: exactly-once guard via `user_version`"

- **Gameable form:** comments are prose. A builder can write the comment while the code skips the `user_version` check. Documentation bullet, not a behavioral gate.
- **Strengthening:** drop as non-load-bearing and let the behavioral bullets do the work.

## 4. Underspecified test setup

### 4.1 `TestStoreMigrationPreservesLegacyCodexBinding` column list

- **Plan text** (PLAN.md line 74): "set up a raw DB handle with legacy-shape `project_bindings` (`project_id PRIMARY KEY` + `provider TEXT NOT NULL`), insert one Codex row with a specific `profile_id` + `created_at`".
- **Gap:** the legacy DDL has five columns (`project_id`, `profile_id`, `provider`, `created_at`, `modified_at`) plus two FK constraints (to `projects.id` and `profiles.id`). Test must:
  1. Create a `projects` row + a `profiles` row first (FK prerequisites), OR disable FK via `PRAGMA foreign_keys = OFF` in raw setup.
  2. Supply all five binding columns.
  3. Match the legacy DDL exactly — including FK clauses — so the rebuild's INSERT SELECT reads the expected column list.

## 5. YAGNI / scope audit

### 5.1 Two-provider test without DefaultHostProfile — not a scope creep

The sqlite layer's test creates profiles directly via `domain.NewProfile(domain.ProviderClaude, ...)` + `store.CreateProfile(...)` (pattern from `TestStoreProfileAndBindingLifecycle` at store_test.go:58, 63). Neither call touches `manage.Service.DefaultHostProfile`. ProviderClaude stub at `internal/services/manage/service.go:159-160` is orthogonal.

### 5.2 Destructive recreate alternative

Focus-plan §7 item 5 flags destructive recreate vs forward-only. Plan chose forward-only; Notes line 98 routes the override to dev in Phase 3. Reasonable posture.

## 6. Call-site blast-radius audit (Unit 3.1)

- `grep -rn BindingByProjectID internal/**/*.go` returns **exactly 9 Go files** matching the 9 paths in Unit 3.1:
  - `internal/domain/repository.go` (interface)
  - `internal/adapters/sqlite/store.go` (concrete method)
  - `internal/adapters/sqlite/store_test.go` (2 call sites: 82, 178)
  - `internal/services/codex/service.go` (1 production caller: 216)
  - `internal/services/codex/service_test.go` (1 fakeStore method: 76)
  - `internal/services/manage/service.go` (1 production caller: 240)
  - `internal/services/manage/service_test.go` (1 test caller: 411)
  - `internal/cli/manage_test.go` (1 test caller: 79)
  - `internal/cli/codex_setup_test.go` (1 test caller: 68)
- No 10th caller in `cmd/valv/`, `magefile.go`, generated code, or embedded interfaces. `codex.Store` + `manage.Store` embed `domain.BindingRepository` transitively at service.go:25 and service.go:22.

## 7. Interface-change consumer-side audit

- `type Store interface` grep returns three hits: `globalswitch`, `manage`, `codex`. Only manage + codex embed `domain.BindingRepository`; `globalswitch.Store` declares only `ProfileByName` and is unaffected.
- `fakeStore.BindingByProjectID` at service_test.go:76 is a concrete method, not a re-declared interface. Plan correctly lists it in Unit 3.1.

## 8. Composite-PK semantics audit

- `ON CONFLICT(project_id, provider)` two-simultaneous-insert (Codex + Claude for same project, zero prior bindings): two distinct rows, no conflict, both inserts succeed.
- Re-upserting same `(project_id, provider)` tuple: `ON CONFLICT` branch fires, updates `profile_id` + `modified_at`.
- `grep REFERENCES project_bindings` returns zero hits. Rebuild-inside-tx posture safe today.

## 9. Unknowns routed to orchestrator

- **U1.** `PRAGMA user_version` inside or outside `BeginTx`? Recommend inside. Route: Phase 3.
- **U2.** Strengthen acceptance bullets to exclude `domain.Provider("")` gaming? Route: Phase 3 brief.
- **U3.** Drop non-load-bearing migration-comment bullet? Route: Phase 3.
- **U4.** Enumerate 5 binding columns + FK prerequisites in migration-preservation test? Route: Phase 3 brief.

## 10. Summary

Plan is **mostly correct, not restructure-fail**. One legitimate race window (§2.1), three weakened acceptance bullets (§3), one underspecified test setup (§4.1). Call-site enumeration, composite-PK semantics, interface blast radius, legacy `user_version = 0` assumption, and YAGNI posture all survive adversarial attack. Race window fix is a one-line design tweak ("read inside tx"), not a unit-tree change. Unit 3.1 / 3.2 boundary + blocking rationale correct.

**Verdict: `fail` — round 2 expected after Phase 3 brief addresses §2.1 + §3 + §4.1.**
