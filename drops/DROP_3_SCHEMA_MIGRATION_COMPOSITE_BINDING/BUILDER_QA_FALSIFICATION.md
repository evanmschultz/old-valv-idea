# DROP_3 — Builder QA Falsification

Append a `## Unit N.M — Round K` section per QA attempt. See `main/drops/WORKFLOW.md` § "Phase 5 — Build QA (per unit)" for what each section should contain.

## Unit 3.1 — Round 1

- **QA agent:** go-qa-falsification-agent
- **Commit under review:** `2ad62a4 feat(drop-3): thread provider arg through BindingByProjectID`
- **Verdict:** pass (with 1 ADVISORY)

### Attacks attempted

1. **Missed `BindingByProjectID` call sites — REFUTED.** `grep -rn 'BindingByProjectID' internal/` returns exactly 16 lines across 10 files (2 are duplicate `t.Fatalf` error-message references, 2 are interface/method declarations at `internal/domain/repository.go:22` and `internal/services/codex/service_test.go:76`, 1 is a `ProfileID` compare log line). Every function-call invocation — `internal/services/codex/service.go:216`, `internal/services/manage/service.go:240`, `internal/adapters/sqlite/store_test.go:82`, `:178`, `internal/services/manage/service_test.go:411`, `internal/cli/codex_setup_test.go:68`, `internal/cli/manage_test.go:79` — passes `domain.ProviderCodex` as the third argument. The interface decl in `domain/repository.go:22` carries the new `Provider` type; the method decl in `codex/service_test.go:76` carries the new `domain.Provider` parameter. Grep over `cmd/` returns zero hits — no CLI-entry call sites. No missed caller.

2. **Bare-string-cast escape via `domain.Provider("..."`) — REFUTED (no new violations).** `grep -F 'domain.Provider("' internal/` returns exactly one hit: `internal/services/globalswitch/service_test.go:100` (`domain.Provider("claude")` inside `TestSwitchRejectsUnsupportedProvider`). Pre-existing; the orchestrator has already deferred this as out-of-scope per the builder worklog Unknowns and the spawn prompt attack-7. No new bare-string casts introduced by Unit 3.1. The other `domain.Provider(...)` conversions that `grep -n 'domain\.Provider\b'` turns up — `internal/adapters/sqlite/store.go:202,227,255,347,382,496,554` — are `domain.Provider(providerValue)` where `providerValue` is a `string` variable scanned from a DB column inside `Row.Scan`; these are legitimate deserialization conversions, not ad-hoc literal casts at call sites, and match the rule's stated intent ("this bars ad-hoc string-literal casts at call sites"). Unit 3.1 acceptance met on substance. See ADVISORY A-1 below for the literal-reading ambiguity.

3. **Zero-value `domain.Provider` variable passed to `BindingByProjectID` — REFUTED.** `grep -nE 'var\s+\w+\s+domain\.Provider\b' internal/` returns zero hits. Every one of the 7 call-site invocations passes the named constant `domain.ProviderCodex` (verified via attack 1 grep). No declared-but-unassigned variable sneaks in via a trailing arg.

4. **SELECT `WHERE project_id = ? AND provider = ?` arg-order bug — REFUTED.** `internal/adapters/sqlite/store.go:333-336`: placeholders appear left-to-right as `project_id = ?` then `AND provider = ?`; positional args passed are `projectID` (line 334) then `string(provider)` (line 335). Order matches. `modernc.org/sqlite` positional `?` binding is 1-based left-to-right per `database/sql` contract. The scan-target order at line 341 (`&binding.ProjectID, &binding.ProfileID, &providerValue, &createdAt, &modifiedAt`) matches the SELECT column order — independent check, also correct.

5. **Scope creep into Unit 3.2 territory — REFUTED.** `git diff 2ad62a4^..2ad62a4 -- internal/adapters/sqlite/store.go` shows exactly one hunk at `@@ -327,11 +327,12 @@`: (a) `BindingByProjectID` signature, (b) SELECT string, (c) added `string(provider)` bind arg. `Bootstrap` (`store.go:40-111`), `UpsertProjectBinding` (`store.go:310-328`), and the `project_bindings` DDL inside the `Bootstrap` statements slice (`store.go:57-65`) are **not** touched. Unit 3.2 territory clean.

6. **Test coverage of the new `AND provider = ?` filter — REFUTED (no false claim).** Builder worklog does not claim provider-mismatch-rejection test coverage; PLAN.md Unit 3.1 acceptance also does not require it (explicitly Unit 3.2 scope per `TestStoreCompositeBindingsCoexistByProvider`). `TestStoreProfileAndBindingLifecycle` and `TestStoreNotFoundErrors` exercise the happy path (matching provider) and the not-found path (with matching provider) — both pass `domain.ProviderCodex`. No test falsely claims provider-mismatch-rejection coverage. Intent matches behavior.

7. **Interface-satisfaction regression (`*sqlite.Store` vs `domain.BindingRepository`, `services/codex.Store`, `services/manage.Store`) — REFUTED.** `domain.BindingRepository` at `internal/domain/repository.go:22` is `BindingByProjectID(context.Context, string, Provider) (ProjectBinding, error)`. `*Store.BindingByProjectID` at `internal/adapters/sqlite/store.go:330` is `(ctx context.Context, projectID string, provider domain.Provider) (domain.ProjectBinding, error)` — matches (named vs unnamed parameters identical at the type level). `services/codex.Store` (line 23-27) and `services/manage.Store` (line 20-24) both embed `domain.BindingRepository`, so the contract cascades. Compile-time proof: `mage testPkg ./internal/adapters/sqlite` green (16 tests); `mage testPkg ./internal/services/codex` green (11 tests, production `s.store.BindingByProjectID` call at `service.go:216` compiles against the real store); `mage testPkg ./internal/services/manage` green (23 tests, same at `service.go:240`).

8. **PLAN.md line 61 ambiguity creates future blind spot — ADVISORY (not a blocker).** The acceptance rule reads: ``grep -F 'domain.Provider("' internal/ returns zero hits outside of the `domain` package's own tests``. The pre-existing hit at `internal/services/globalswitch/service_test.go:100` is outside `domain`, so the rule as literally written is **violated** on `main` after this commit — not satisfied. The orchestrator has accepted this as "pre-existing, deferred" per the builder worklog Unknown. Risk: future grep runs that scope the rule literally will keep hitting this one, and a real new violation in a later drop blends into the noise. Remediation options: (a) amend PLAN.md acceptance to scope the grep to the Unit 3.1 9-path list; (b) tiny follow-up unit replacing `domain.Provider("claude")` with `domain.ProviderClaude` (both constants exist at `internal/domain/types.go:11-12`); (c) add an explicit "known pre-existing" carve-out in PLAN.md wording. Recommended: (b) — smallest diff, eliminates the ambiguity at the source. Not a blocker because the hit is pre-existing and the builder correctly surfaced it.

9. **Defense-in-depth `binding.Provider != domain.ProviderCodex` check at `services/codex/service.go:223` — REFUTED (preserved as planned).** PLAN.md Unit 3.1 paths note: "the existing `binding.Provider != domain.ProviderCodex` check at line 223 stays as-is". Confirmed unchanged — the `git show 2ad62a4 -- internal/services/codex/service.go` diff touches only line 216. With the new `AND provider = ?` filter the post-fetch check is redundant-but-harmless; matches PLAN intent.

10. **Compilation-only false pass (tests thread the arg but don't exercise semantics) — REFUTED for Unit 3.1 acceptance.** Unit 3.1's acceptance bar is signature-threading + call-site correctness + grep-rule compliance, NOT provider-mismatch-rejection coverage (that is Unit 3.2). The 4 named sqlite tests all still pass — PLAN.md-specified bar met.

### Blockers

None.

### Advisories

- **A-1:** PLAN.md line 61 grep-rule ambiguity vs pre-existing `internal/services/globalswitch/service_test.go:100` bare-string cast (`domain.Provider("claude")`). See attack 8. Recommend a 1-line follow-up unit that replaces the literal with `domain.ProviderClaude` before DROP_3 closes, so the Unit 3.1-acceptance invariant actually holds on `main`. Not a blocker for Unit 3.1 — pre-existing, explicitly deferred by the orchestrator.

### Evidence summary

- **Premises:** Signature threading is atomic; every call site passes a named `domain.Provider` constant; no bare-string casts or zero-value vars; SELECT arg order correct; Unit 3.2 territory untouched; interface satisfaction holds; all existing named tests still green.
- **Evidence:** `git show 2ad62a4` (11 files, +42/-18); `grep -rn 'BindingByProjectID' internal/ cmd/` (16 lines internal, 0 in cmd); `grep -F 'domain.Provider("' internal/` (1 pre-existing hit, no new); `grep -nE 'var\s+\w+\s+domain\.Provider\b' internal/` (0 hits); `Read` on `internal/adapters/sqlite/store.go:40-111,310-359` (Bootstrap/Upsert/Select bodies); `mage testPkg` on all 6 affected packages (sqlite 16/16 79.2%, codex 11/11 75.2%, manage 23/23 76.4%, cli 101/101 72.0%, domain 26/26 85.2%, globalswitch 8/8 80.8%).
- **Trace or cases:** 10 attacks — 9 REFUTED, 1 ADVISORY (attack 8, A-1), 0 CONFIRMED.
- **Conclusion:** pass.
- **Unknowns:** none. A-1 routed as an advisory.

## Hylla Feedback

N/A — task touched only Go files whose committed state was fully visible via `Read` / `Grep` on the 10 scoped files plus `git show`. No Hylla queries attempted, no fallbacks needed for this review's purpose (signature-threading + grep-rule compliance + scope-creep check are all directly answerable from `git show` + `grep`). Hylla MCP is more useful for call-site blast-radius questions than for single-commit diff review.

## Unit 3.2 — Round 1

- **QA agent:** go-qa-falsification-agent
- **Commit under review:** `47fd5ee feat(drop-3): migrate project_bindings to composite primary key`
- **Verdict:** pass (with 2 ADVISORIES)

### Attacks attempted

1. **user_version race under concurrent Bootstrap — ADVISORY (A-1), not a blocker.** `BeginTx(ctx, nil)` passes nil `*sql.TxOptions`, so `modernc.org/sqlite` uses its default `_txlock` mode — **DEFERRED** per Context7 `/gitlab_cznic/sqlite` documentation. The migration's first statement is `PRAGMA user_version` (SHARED-lock read only). Two concurrent `valv` CLI processes on the same legacy DB can both read `user_version=0` inside their own DEFERRED tx before either writes. The first to execute `CREATE TABLE project_bindings_new` upgrades to RESERVED; the second returns `SQLITE_BUSY` (Valv's `open.go:35-72` sets only `foreign_keys(1)` — no `busy_timeout`). Outcome: one process completes the migration; the other errors out of Bootstrap — **not corruption, recoverable via retry**. Migration is one-time; post-success every subsequent Bootstrap short-circuits on the read-only `user_version >= 1` check. The PLAN's literal race-mitigation acceptance ("`user_version` read is the FIRST executed statement inside a `BeginTx` block") is satisfied — reading inside the tx is strictly narrower than reading outside. Recommended follow-up (not a Unit 3.2 blocker): either change `s.db.BeginTx(ctx, nil)` to `s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})` (promotes to `BEGIN IMMEDIATE`) or add `_pragma=busy_timeout(5000)` + `_txlock=immediate` to `open.go`'s DSN. See A-1 below.

2. **Shape-probe correctness — REFUTED.** `pragma_table_info('project_bindings')` over a sane post-DDL-loop DB returns one row per column. Zero rows is impossible after the DDL loop (`CREATE TABLE IF NOT EXISTS project_bindings ...` at `store.go:57-66` runs unconditionally before migration). Extra columns added by a future developer would be ignored — the `switch name { case "project_id": ... case "provider": ... }` at `store.go:192-199` only branches on the two columns it cares about. Classification logic at `store.go:207` (`projectIDPK == 1 && providerPK == 0`) correctly identifies legacy (single-PK gives project_id pk=1, provider pk=0) from composite (project_id pk=1, provider pk=2, both `> 0`).

3. **Staging-table orphan on crash-between-steps — REFUTED.** The `CREATE project_bindings_new` → `INSERT SELECT` → `DROP project_bindings` → `ALTER RENAME` sequence runs entirely inside one `BeginTx` at `store.go:116-170`. Process crash mid-sequence aborts the transaction — SQLite's rollback journal rolls back every change atomically. No orphan `project_bindings_new` can survive a crash. `Grep` for `project_bindings_new` returns zero hits outside the migration code + idempotence test, so no other code path can create the staging table.

4. **INSERT SELECT data coercion — REFUTED.** Legacy DDL (preserved in `store_test.go:411-419` and `:529-537`) already declared `provider TEXT NOT NULL`; any legacy row with `provider = NULL` would have been rejected at insert time against the legacy schema. Type-affinity is identical between legacy and new shapes (all 5 columns `TEXT NOT NULL`). The `INSERT INTO project_bindings_new SELECT ...` at `store.go:153-154` copies legacy rows verbatim; SQLite's NOT NULL + affinity checks pass trivially.

5. **Idempotent Bootstrap short-circuit — REFUTED.** `TestStoreBootstrapIsIdempotentAfterMigration` (`store_test.go:501`) runs Bootstrap twice and asserts: (i) `COUNT(*) FROM sqlite_master WHERE name='project_bindings_new'` = 0 (staging table never re-created), (ii) `COUNT(*) FROM project_bindings` = 1 (no duplication), (iii) `BindingByProjectID` returns identical `ProfileID` + `CreatedAt` + `ModifiedAt` across both Bootstraps. The `user_version >= 1` branch at `store.go:130-135` fires on the second call — without it, the probe would see composite shape → return `false` (not legacy) → skip rebuild anyway, so user_version would be re-set but the staging-count assertion would still hold. Both branches are live code paths under test.

6. **UpsertProjectBinding ON CONFLICT semantics change — REFUTED.** Pre-DROP-3: second `BindProject` with different provider on the same project REPLACED the first binding. Post-DROP-3: it creates a second row. This is the explicit stated scope of DROP_3 (PLAN §Scope line 14: "One project may bind one account per provider"). Every `BindProject` in `manage/service_test.go` uses `domain.ProviderCodex` only (lines 254, 280, 396, 439, 670) — no test calls `BindProject` twice with different providers and asserts replacement semantics. No silent inconsistency introduced.

7. **FK enforcement during rebuild — REFUTED.** `Grep` for `REFERENCES project_bindings` across `main/` returns zero hits — no table has an inbound FK to `project_bindings`. `DROP TABLE project_bindings` therefore violates no FK even with `PRAGMA foreign_keys = ON` (connection-level in `open.go:38,47`). `ALTER TABLE ... RENAME TO project_bindings` rewrites dependent FK references in other tables' DDL — there are none. `TestStoreForeignKeysRejectInvalidBindings` (`store_test.go:190-201`) uses `newBootstrappedStore` which hits the new composite shape with both FKs intact; test still green proves FKs survive.

8. **PRAGMA user_version = 1 durability — REFUTED.** SQLite persists `user_version` in the database header. `PRAGMA user_version = N` inside a transaction stages the header change; on commit, the header write is durable. Context7 `/gitlab_cznic/sqlite` documents pragma pass-through as standard (no driver-specific quirks). The idempotence test's second Bootstrap reads `user_version = 1` via `tx.QueryRowContext(ctx, 'PRAGMA user_version')` — confirming durability across tx boundaries.

9. **DROP_1 / DROP_2 cross-dependency — REFUTED.** `Grep` for `project_bindings` across `internal/` + `cmd/` returns hits only in `internal/adapters/sqlite/store.go` (schema + migration + queries) and `internal/adapters/sqlite/store_test.go` (tests). No CLI, service, or non-sqlite-adapter code issues raw SQL against `project_bindings`. All access goes through the `domain.BindingRepository` interface; schema-shape coupling is encapsulated in the sqlite package.

10. **Test semantics vs test compilation — REFUTED.**
   - `TestStoreCompositeBindingsCoexistByProvider` (`store_test.go:324`): inserts TWO bindings with same project, different providers; fetches EACH by provider; asserts each has the correct `ProfileID` AND `Provider`. A single-PK regression would cause the Codex fetch to return the Claude profile_id; assertion would fail.
   - `TestStoreMigrationPreservesLegacyCodexBinding` (`store_test.go:383`): seeds legacy-shape table + row + FK prerequisites; confirms `user_version=0` pre- and `=1` post-Bootstrap; fetches binding AFTER rebuild and asserts ProfileID + CreatedAt + ModifiedAt unchanged via RFC3339Nano comparison. Lost data would surface as `ErrNotFound`; mutated timestamps would fail the comparison.
   - `TestStoreBootstrapIsIdempotentAfterMigration` (`store_test.go:501`): explicit `COUNT(*) FROM project_bindings` assertion at `:613` (`bindingCount != 1` → fatal). Real post-second-Bootstrap COUNT check.

11. **Coverage floor — REFUTED.** `mage testPkg ./internal/adapters/sqlite` reports **78.5%** (floor 70%). All three migration branches exercised: legacy-rebuild (preservation test), fresh-composite-shape (all `newBootstrappedStore` tests), short-circuit (idempotence test). One branch NOT specifically test-exercised is `UpsertProjectBinding` hitting the `ON CONFLICT(project_id, provider) DO UPDATE` update path — no test calls `UpsertProjectBinding` twice with identical `(project_id, provider)`. LATENT gap recorded as A-2 below; not required by Unit 3.2 acceptance.

### Blockers

None.

### Advisories

- **A-1:** `BeginTx(ctx, nil)` uses DEFERRED per `modernc.org/sqlite` default. Two concurrent CLI processes on the same legacy DB can both read `user_version=0` before one acquires RESERVED for the rebuild. The second fails with `SQLITE_BUSY` (no `busy_timeout` configured) — user-visible error, no corruption. The PLAN's literal acceptance ("`user_version` read is the FIRST executed statement inside `BeginTx`") is met; full race-proofing would require either `sql.TxOptions{Isolation: sql.LevelSerializable}` at the `BeginTx` call site (promotes to `BEGIN IMMEDIATE` in modernc.org/sqlite) or a `_pragma=busy_timeout(5000)` + `_txlock=immediate` DSN addition in `open.go`. Recommended as a trivial follow-up unit — not a blocker for Unit 3.2.

- **A-2:** No test exercises `UpsertProjectBinding` twice with identical `(project_id, provider)`, so the new `ON CONFLICT(project_id, provider) DO UPDATE SET profile_id = excluded.profile_id, modified_at = excluded.modified_at` branch is never asserted. The PLAN's acceptance list does not require this branch to be covered by a dedicated test, so this is informational. A one-test follow-up (or extending `TestStoreCompositeBindingsCoexistByProvider` with a re-upsert + re-fetch asserting the updated `ProfileID` + `ModifiedAt`) would close the gap.

### Evidence summary

- **Premises:** migration moves `project_bindings` from single-column PK `(project_id)` to composite `(project_id, provider)` via forward-only `PRAGMA user_version`-gated rebuild; legacy rows preserved; idempotent; FK-safe; coverage floor met.
- **Evidence:** `git show 47fd5ee` (4 files, +425/-4); `Read` on `internal/adapters/sqlite/store.go` (full file), `store_test.go:1-100,192-324,329-629`, `open.go` (full), `internal/cli/store.go`, `internal/services/manage/service.go:189-250`; `Grep` for `project_bindings` / `UpsertProjectBinding` / `REFERENCES project_bindings`; Context7 `/gitlab_cznic/sqlite` query on default `_txlock` + pragma pass-through; `mage testPkg ./internal/adapters/sqlite` (19/19 pass, 78.5% cover).
- **Trace or cases:** 11 attacks — 9 REFUTED, 2 ADVISORY (A-1 concurrent-Bootstrap DEFERRED race; A-2 UPDATE-path coverage gap), 0 CONFIRMED.
- **Conclusion:** pass.
- **Unknowns:** none. A-1 + A-2 routed as advisories to the orchestrator.

## Hylla Feedback

N/A — this review worked over a single commit diff on `.go` files. `git show 47fd5ee`, `Read` on the 5 scoped files, and `Grep` across `internal/` + `cmd/` covered the full evidence surface. Context7 `/gitlab_cznic/sqlite` answered the driver-semantics question on default `_txlock`. No Hylla queries attempted; none needed for single-commit diff review of known file paths.
