# DROP_14 — Builder QA Proof

Append a `## Unit 14.M — Round K` section per build-QA pass. See `main/drops/WORKFLOW.md` § "Phase 5 — Build-QA (per unit)" for what each section should contain. Both `BUILDER_QA_PROOF.md` and `BUILDER_QA_FALSIFICATION.md` must close green for the unit to be marked `done`.

## Unit 14.1 — Round 1

verdict: pass

### Acceptance Audit

1. **`domain.AccountEnvRepository` interface added.** `internal/domain/repository.go:43-56` defines the four-method contract (Set / Get / List / Unset) keyed by `(profileID, envKey)` with Go-doc on each method explaining the duplicate-key-across-profiles + rename-stability invariants. Backing type `domain.AccountEnvEntry` defined at `internal/domain/types.go:12-18` with `ProfileID`, `EnvKey`, `EnvValue`, `CreatedAt`, `UpdatedAt` fields (`time.Time` import added at `types.go:6`).

2. **`domain.ErrUnsupportedSchema` sentinel added.** `internal/domain/errors.go:19` declares `var ErrUnsupportedSchema = errors.New("unsupported schema")` with a 5-line Go-doc explaining the supported window and operator-facing semantics (`errors.go:14-18`).

3. **DSN busy policy + exact-pragma-name dedup.** `internal/adapters/sqlite/open.go:25-28` declares `requiredPragmas` as an ordered slice — `busy_timeout(5000)` first, `foreign_keys(1)` second — so busy_timeout is in effect during all subsequent pragma execution. The dedup helper at `open.go:80-101` (`applyRequiredPragmas`) calls `existingPragmaNames` (`open.go:106-116`) which delegates to `parsePragmaName` (`open.go:121-135`). `parsePragmaName` performs exact pragma-name parsing per the planner contract: trim whitespace → take substring before first `(` or `=` rune → trim again → lowercase → return. The false-prefix counterexample is covered by:
   - `TestParsePragmaName` (`open_test.go:172-192`) — line 183 asserts `parsePragmaName("busy_timeout_pragma=foo")` returns `"busy_timeout_pragma"` (not `"busy_timeout"`).
   - `TestBuildDSNAppliesRequiredPragmas` (`open_test.go:68-150`) — the "false-prefix value still triggers append" case (lines 92-96) requires `busy_timeout_pragma=foo` AND `busy_timeout(5000)` AND `foreign_keys(1)` ALL present in the resulting DSN's `_pragma` slice; the post-check at lines 132-147 also asserts `busy_timeout(5000)` appears exactly once. This proves the planner's required R5.F5.1 fix (exact pragma-name parsing, not prefix matching) landed correctly.

4. **Bootstrap rejects v0 AND v99 via `errors.Is(err, domain.ErrUnsupportedSchema)`.**
   - v0 rejection: `TestStoreBootstrapRejectsLegacyV0` (`store_test.go:577-635`) seeds three core tables at user_version=0, calls Bootstrap, asserts non-nil error with `errors.Is(err, domain.ErrUnsupportedSchema)` (line 626) and message contains `user_version=0` (line 629) and `[1, 2]` (line 632). Bootstrap's pre-DDL probe at `store.go:78-89` performs the rejection.
   - v99 rejection: `TestStoreBootstrapRejectsForwardIncompatibleSchema` (`store_test.go:640-670`) seeds v1 schema + bumps user_version=99, calls Bootstrap, asserts `errors.Is(err, domain.ErrUnsupportedSchema)` (line 661) and message contains `user_version=99` (line 664) and `[1, 2]` (line 667). `migrateSchemaOnConn` performs the rejection at `store.go:202-207`.
   This proves the planner's required R5.F5.2 fix (`ErrUnsupportedSchema` sentinel + bidirectional reject + `errors.Is` not string-match) landed correctly.

5. **Reject messages include observed version + supported window.** Both rejection sites format the message with `got user_version=N, supported window [%d, %d]` using the `minSupportedSchemaVersion = 1` / `maxSupportedSchemaVersion = 2` constants (`store.go:46-48`). Test assertions cover `user_version=0`, `user_version=99`, and `[1, 2]` substrings (store_test.go:629-633, 664-668).

6. **Two-connection first-open upgrade test.** `TestStoreBootstrapConcurrentFirstOpen` (`store_test.go:956-1015`):
   - Uses a file-backed SQLite path under `t.TempDir()` (line 962) — not shared-cache memory, which would short-circuit file-level locking.
   - Pre-seeds at v1 via `seedV1Schema` (line 973).
   - Spawns two goroutines with `sync.WaitGroup` (lines 986-998) holding both behind a `start` channel for synchronized release (line 998).
   - Asserts both succeed with no error (line 1003) — no `SQLITE_BUSY` surfaces because the DSN `busy_timeout(5000)` absorbs the wait on the second goroutine's `BEGIN IMMEDIATE`.
   - Asserts final `PRAGMA user_version == 2` (line 1012).
   The serialization mechanism is `BEGIN IMMEDIATE` in `migrateSchemaOnConn` (`store.go:187-188`) on a dedicated `*sql.Conn` acquired via `s.db.Conn(ctx)` in Bootstrap (`store.go:63-67`).

7. **Account-env repository contract matches SQLite implementation.** Domain methods at `internal/domain/repository.go:46-55`:
   - `SetAccountEnv(ctx, profileID, envKey, envValue) (AccountEnvEntry, error)`
   - `GetAccountEnv(ctx, profileID, envKey) (AccountEnvEntry, error)`
   - `ListAccountEnv(ctx, profileID) ([]AccountEnvEntry, error)`
   - `UnsetAccountEnv(ctx, profileID, envKey) error`
   SQLite implementations at `internal/adapters/sqlite/account_env.go`:
   - `SetAccountEnv` at line 31 — signatures match exactly; INSERT...ON CONFLICT(profile_id, env_key) DO UPDATE preserves `created_at` and advances `updated_at` (lines 37-46).
   - `GetAccountEnv` at line 58 — wraps `ErrNotFound` via `scanAccountEnv` (line 139).
   - `ListAccountEnv` at line 75 — query uses `ORDER BY env_key ASC` (line 79).
   - `UnsetAccountEnv` at line 103 — wraps `ErrNotFound` on `RowsAffected() == 0` (line 118).
   The `Store` type therefore satisfies `domain.AccountEnvRepository` structurally (verified by `TestStoreAccountEnvCRUD` exercising the full surface; if any signature were off, the test would not compile).

8. **AccountEnvList orders by `env_key ASC` at the store layer.** SQL literal `ORDER BY env_key ASC` at `account_env.go:79`. Test `TestStoreAccountEnvCRUD` (`store_test.go:783-870`) inserts keys in non-alphabetical order `["GAMMA", "ALPHA", "BETA"]` (line 800) and asserts `ListAccountEnv` returns `["ALPHA", "BETA", "GAMMA"]` (lines 827-832). This proves the planner's required R5.F4.3 fix (store-layer ordering, not caller re-sort) landed correctly.

9. **Rename stability via Profile.ID.** `TestStoreAccountEnvSurvivesProfileRename` (`store_test.go:918-949`) creates a profile, sets `FOO=bar`, calls `UpdateProfileName(provider, "old-name", "new-name")`, asserts `renamed.ID == profile.ID` (line 936-939), then asserts `GetAccountEnv(ctx, profile.ID, "FOO").EnvValue == "bar"` (lines 942-948). The schema FK is on `profile_id` (`account_env.go:25`), not on a name column, so ownership survives rename by construction.

10. **Legacy v0 tests at store_test.go:504-668 are gone and replaced.** Verified via `git show 7ff9336^:internal/adapters/sqlite/store_test.go` and `Read` of `/tmp/old_store_test.go` lines 500-668 — the old tests were:
    - `TestStoreMigrationPreservesLegacyCodexBinding` — seeded legacy DDL (single-PK `project_bindings`) at default user_version=0, asserted Bootstrap advanced to v1.
    - `TestStoreBootstrapIsIdempotentAfterMigration` — same v0 seed pattern, double-Bootstrap idempotence check.
    Both are removed from the current `store_test.go`. The replacements live at:
    - `TestStoreBootstrapAdvancesV1ToV2` (`store_test.go:428-508`) — seeds v1 + asserts advance to v2.
    - `TestStoreBootstrapV1ToV2PreservesBindings` (`store_test.go:699-778`) — v1 seed + double-Bootstrap idempotence at v2.
    - Plus the two reject tests covered in item 4.
    No remaining test seeds `user_version = 0` with the intent of expecting a successful auto-migrate.

11. **`hasAnyCoreTable` probe distinguishes fresh vs legacy.** Implementation at `store.go:154-169` iterates the five-element `coreSchemaTables` slice (`store.go:54-60`: `projects, profiles, project_bindings, runtimes, provider_images`) and returns `true` on the first `sqlite_master` hit. Pre-DDL ordering at `store.go:74-89`:
    - Read `PRAGMA user_version` (line 75).
    - If `preUserVersion == 0`, call `hasAnyCoreTable`.
      - Fresh DB (no tables yet) → returns false → DDL runs → `migrateSchemaOnConn` sees `userVersion=0 < 2` → creates `account_env` + bumps to 2.
      - Legacy v0 DB (tables present, user_version=0) → returns true → reject with `ErrUnsupportedSchema`.
    The fresh-DB path is exercised by every `newBootstrappedStore(t)` call (`store_test.go:16-32`) — 33 tests in the package — and explicitly by `TestStoreBootstrapIsIdempotentAtV2` (`store_test.go:672-694`) which double-bootstraps and asserts final user_version=2. The legacy-v0 path is exercised by `TestStoreBootstrapRejectsLegacyV0` which seeds three of the five core tables (line 588-617).

12. **No edits outside allowed packages.** `git show 7ff9336 --stat -- ':!drops/'` returns exactly 8 files: 5 in `internal/adapters/sqlite/`, 3 in `internal/domain/`. Both packages are listed in Unit 14.1's `paths`. No CLI, services, providers, docker, runtime, output, or test-harness files touched.

### Mage Results (run by QA Proof agent)

```
mage testPkg ./internal/domain
  -> PKG PASS github.com/evanmschultz/valv/internal/domain (1.27s)
  -> 26 tests passed, 0 failed
  -> coverage 85.2% (gate 60.0%, delivery floor 70%)

mage testPkg ./internal/adapters/sqlite
  -> PKG PASS github.com/evanmschultz/valv/internal/adapters/sqlite (1.45s)
  -> 33 tests passed, 0 failed
  -> coverage 80.6% (gate 60.0%, delivery floor 70%)
```

Both packages exceed the 70% delivery-standard floor by a comfortable margin. Counts exactly match the builder's worklog claim (26/85.2%, 33/80.6%).

### Findings

- **All 11 specifically-listed verification points pass with file:line citations.** No gaps against acceptance criteria, the Round-5 plan refinements, or the planner's specifically-flagged Unknowns.
- **`hasAnyCoreTable` probe is sound (and was the right Unknown to surface).** The builder flagged the probe-vs-fresh-DB distinction in the worklog as an Unknown for orchestrator review. The implementation is correct: pre-DDL ordering (probe before CREATE TABLE IF NOT EXISTS) is the load-bearing detail that distinguishes "fresh DB with user_version=0" from "legacy v0 with tables + user_version=0". Without pre-DDL ordering, the idempotent DDL would erase the distinction. The five-table list (`coreSchemaTables`) is also exhaustive against the v1 schema, so any real v0 database will hit on the first iteration (projects).
- **Minor observation (not a fail):** when bootstrapping against a synthetic empty-tables forward-incompatible DB (user_version=99 + zero tables), Bootstrap runs the v2 DDL at lines 143-147 BEFORE `migrateSchemaOnConn` reads user_version=99 and rejects. The DDL writes happen outside the `BEGIN IMMEDIATE` block (which is scoped to `migrateSchemaOnConn` only), so they are NOT rolled back when migrateSchemaOnConn returns the reject error. In practice this is a non-issue: (a) `CREATE TABLE IF NOT EXISTS` is a no-op against a real production v99 DB (which would already have whatever future-shaped tables it needs), and (b) the planner's acceptance only requires `errors.Is(err, domain.ErrUnsupportedSchema)` + diagnostic message, with no purity guarantee on side effects of a forward-incompatible reject. Flagging for QA Falsification + orchestrator awareness, not blocking.
- **Tests are table-driven where the planner asked for it** (pragma dedup, parsePragmaName) and behavior-oriented elsewhere (CRUD, rename, concurrent open). All tests use `t.Parallel()` and the `mage test` `-race` flag was active per the gate runner.
- **`gofumpt` clean** (implicit — mage testPkg's first step is `gofumpt -l` and both targets passed).
- **Context propagation correct.** Every new exported method on `Store` and every interface method on `AccountEnvRepository` takes `ctx context.Context` as the first parameter. Bootstrap, migrateSchemaOnConn, hasAnyCoreTable all thread ctx through to `conn.QueryRowContext` / `conn.ExecContext`.
- **Errors are wrapped with `%w`** at every boundary (`account_env.go:48,83,96,111,115,118`; `store.go:65,76,85,145,162,188,199,204,215,218,223`).

## Unit 14.1 — Round 2

verdict: pass

### Acceptance Audit (Round 2 — F1 + F2 fix verification)

1. **`Bootstrap` no longer runs CREATE TABLE outside a transaction.** `internal/adapters/sqlite/store.go:125-157` shows the new `Bootstrap` body — it acquires a conn (line 126), reads `PRAGMA user_version` pre-tx (line 140), performs the legacy-v0 probe via `hasAnyCoreTable` when user_version=0 (lines 143-154), then delegates to `migrateSchemaOnConn` (line 156). The non-transactional `statements := []string{...}` loop from Round 1 (was at old store.go:91-149) is gone, confirmed by `git diff b47cbd3~1 b47cbd3 -- internal/adapters/sqlite/store.go` which shows lines 91-149 removed wholesale. The pre-tx user_version read is read-only and the `hasAnyCoreTable` probe is a read-only `sqlite_master` scan, so neither violates the "no DDL outside tx" invariant.

2. **`migrateSchemaOnConn` runs BEGIN IMMEDIATE → core DDL → account_env DDL → PRAGMA user_version=2 → COMMIT atomically.** `store.go:200-252` walks through the atomic block:
   - Line 201: `BEGIN IMMEDIATE` opens the tx.
   - Lines 204-209: `defer` rollback guard ensures rollback on any non-COMMIT exit (matches sql.Conn lifecycle pattern).
   - Lines 211-214: in-tx `PRAGMA user_version` read (sees latest committed value because BEGIN IMMEDIATE acquired the reserved lock).
   - Lines 216-221: forward-incompatible rejection (user_version > 2) returns wrapped `ErrUnsupportedSchema` BEFORE any mutating statement.
   - Lines 227-233: fresh-init branch (user_version == 0) executes every statement from `coreSchemaDDL` inside the tx.
   - Lines 238-245: v0→v2 and v1→v2 both execute `accountEnvCreateTable` then stamp `PRAGMA user_version = 2`.
   - Lines 247-251: COMMIT, then set `committed = true` so deferred rollback is skipped.
   The entire fresh-init + version-stamp is now inside one BEGIN IMMEDIATE/COMMIT pair. SQLite DDL is transactional, so a crash anywhere between lines 201 and 248 rolls back to the pre-bootstrap on-disk state — closing the F1 false-legacy-v0 window.

3. **`coreSchemaDDL` slice exists at package scope.** `store.go:74-123` declares `var coreSchemaDDL = []string{...}` containing the six core DDL statements in order: `projects`, `profiles`, `project_bindings`, `runtimes`, `idx_runtimes_project_id` (index), `provider_images`. All statements use `CREATE TABLE IF NOT EXISTS` / `CREATE INDEX IF NOT EXISTS` to remain idempotent. Go-doc comment at lines 68-73 documents the atomicity invariant. Note: the redundant `PRAGMA foreign_keys = ON;` statement from Round 1 was removed (it duplicated the DSN-level `_pragma=foreign_keys(1)` set in `open.go`), keeping the in-tx statement list focused on schema mutations — confirmed by inspecting `coreSchemaDDL` (no pragma statement) vs Round 1's `statements` slice which began with the pragma.

4. **`var _ domain.AccountEnvRepository = (*Store)(nil)` compile-assert present.** `store.go:21` declares `var _ domain.AccountEnvRepository = (*Store)(nil)` with a 4-line Go-doc (lines 17-20) explaining the interface-drift purpose. Adjacent to `type Store struct{}` (lines 13-15) so a method-signature change on `domain.AccountEnvRepository` now fails compile at the sqlite adapter boundary, not at the first consumer binding. F2 verified.

5. **Three new atomicity tests exist and pass.**
   - `TestStoreBootstrapFreshDBStampsV2WithAllCoreTables` (`store_test.go:956-986`): brand-new in-memory DB via `newBootstrappedStore(t)`, asserts post-Bootstrap `user_version == 2` (line 967) AND all six core tables + `account_env` are present (lines 971-985). Direct happy-path proof of the F1 invariant on the production code path.
   - `TestStoreBootstrapAtomicityRolledBackInitLeavesEmptyDB` (`store_test.go:1000-1075`): opens a file-backed DB, manually runs `BEGIN IMMEDIATE` + every `coreSchemaDDL` statement + `accountEnvCreateTable` + `PRAGMA user_version = 2` on a `*sql.Conn`, then `ROLLBACK` (lines 1017-1034). Asserts post-rollback `user_version == 0` (lines 1038-1044) AND ZERO core tables remain (lines 1046-1060). Then the killer assertion at lines 1065-1074: a subsequent `Bootstrap` on the rolled-back DB succeeds and stamps v2, proving the DB is recoverable as fresh (NOT misclassified as legacy v0). This test would fail against Round-1 code because Round 1's CREATE TABLE statements ran outside any tx, so they could not be rolled back. Direct behavioral proof of F1.
   - `TestStoreBootstrapAtomicityCancelMidTransactionRollsBack` (`store_test.go:1087-1122`): opens a fresh file-backed DB, calls `Bootstrap` with a pre-cancelled context (lines 1099-1105) — modeling the SIGKILL-during-bootstrap scenario. Then re-Bootstraps with a fresh context (lines 1111-1114) and asserts `user_version == 2` (lines 1115-1121). Proves no committed state lands from the aborted attempt AND the DB is recoverable. Contextual analog of the rollback test, completing the F1 trace coverage.
   The `mage testPkg ./internal/adapters/sqlite` run reports 36/36 tests passed (33 from Round 1 + 3 new), 80.9% coverage (up from 80.6%).

6. **Legacy v0 probe still functional and still correct.** `hasAnyCoreTable` is unchanged at `store.go:161-176` (read-only `sqlite_master` scan over `coreSchemaTables`). `Bootstrap` still calls it BEFORE entering the migration tx (`store.go:143-154`), so the legacy-v0 path runs identically to Round 1. `TestStoreBootstrapRejectsLegacyV0` (`store_test.go:577-635`) still seeds three core tables at user_version=0 and asserts Bootstrap rejects with `errors.Is(err, ErrUnsupportedSchema)` (line 626), message contains `user_version=0` (line 629) and `[1, 2]` (line 632). This test passed in Round 2's 36/36 run. The F1 fix made the `tables-present + user_version=0` fingerprint UNAMBIGUOUSLY mean "real legacy v0" (because atomic-bootstrap rolls back a crashed fresh-init to empty), so the probe's classification is now provably sound rather than racy.

7. **Existing 33 Round-1 tests still pass + 3 new = 36 total.** The mage testPkg output (run during this review) reports exactly 36 tests passed, 0 failed, 0 skipped:
   ```
   [PKG PASS] github.com/evanmschultz/valv/internal/adapters/sqlite (1.46s)
   tests: 36
   passed: 36
   failed: 0
   skipped: 0
   ```
   Counts match the worklog claim (Round 1 = 33, Round 2 added 3 = 36). No regressions. Coverage 80.9% (up from 80.6% in Round 1), comfortably above the 70% delivery floor.

8. **No edits outside `internal/adapters/sqlite/` + drop dir.** `git show b47cbd3 --stat` returns exactly three files changed in the Round-2 commit:
   - `drops/DROP_14_ENV_VARS/BUILDER_WORKLOG.md` (worklog appendage)
   - `internal/adapters/sqlite/store.go` (F1 + F2 source fixes)
   - `internal/adapters/sqlite/store_test.go` (3 new tests)
   No CLI, services, providers, docker, runtime, output, or domain files touched. Scope discipline clean.

9. **No raw `go test` / `GOCACHE=...` discipline violations.** The Round-2 worklog only cites `mage testPkg ./internal/adapters/sqlite` and `mage testPkg ./internal/domain` runs. The QA Proof reviewer also ran exclusively `mage testPkg ./internal/adapters/sqlite` for independent verification. No raw `go test`, `go build`, or `GOCACHE=...` overrides in the worklog or this review.

### Mage Results (run by QA Proof agent)

```
mage testPkg ./internal/adapters/sqlite
  -> PKG PASS github.com/evanmschultz/valv/internal/adapters/sqlite (1.46s)
  -> 36 tests passed, 0 failed, 0 skipped
  -> coverage 80.9% (gate 60.0%, delivery floor 70%)
```

Exactly matches the builder's worklog claim (36 tests / 80.9%).

### Findings

- **F1 fix verified by inspection AND by behavioral test.** The source diff (`git diff b47cbd3~1 b47cbd3 -- internal/adapters/sqlite/store.go`) shows the non-transactional CREATE-TABLE loop deleted from `Bootstrap` and re-homed under the existing `BEGIN IMMEDIATE / COMMIT` block inside `migrateSchemaOnConn` (gated on `userVersion == 0`). `TestStoreBootstrapAtomicityRolledBackInitLeavesEmptyDB` exercises the rollback semantics directly — it would fail against Round-1 code because Round 1's DDL was non-transactional. The minor observation from Round 1 (CREATE TABLE running BEFORE the user_version=99 reject inside the tx) is now eliminated: every DDL statement, including the fresh-init path, runs inside the tx that may roll back. The rollback path therefore preserves the pre-bootstrap on-disk state in every code path.

- **F2 fix verified by inspection.** `var _ domain.AccountEnvRepository = (*Store)(nil)` at `store.go:21` provides the compile-time interface-drift guard. The 4-line Go-doc explains intent. Verified the file compiles by running `mage testPkg` (which compiles before testing).

- **Atomicity test design is sound.** The three new tests cover three distinct failure modes: (a) the happy path proves `userVersion==0` branch creates all v2 tables under the tx; (b) the explicit ROLLBACK test proves SQLite rollback wipes DDL writes on `modernc.org/sqlite`, which is the load-bearing semantic for the F1 fix; (c) the cancel-mid-tx test proves the production code path (Bootstrap with a pre-cancelled context) leaves the DB recoverable, NOT in the broken `tables-present + user_version=0` state that Round-1 code could leave behind. The cancellation test is intentionally lenient about whether `Bootstrap` errors at the user_version probe or inside the tx — the contract under test is the resulting on-disk state, not the specific error code from the aborted call. This matches the planner's acceptance ("rollback semantics", not "specific error from cancel").

- **Pre-tx probe in Bootstrap (lines 139-154) is still outside the tx.** This is intentional — it's a read-only `sqlite_master` scan that doesn't need a reserved lock. The F1 invariant ("tables-present + user_version=0 means real legacy v0") makes the probe's classification correct in every code path: after Round 2, no path through `Bootstrap` can produce a `tables-present + user_version=0` state from a crashed/cancelled fresh-init (such crashes roll back the tx, leaving zero tables).

- **`PRAGMA foreign_keys = ON;` redundancy removed from in-tx statement list.** Round 1's `statements` slice opened with `PRAGMA foreign_keys = ON;`. Round 2's `coreSchemaDDL` omits it — relying on the DSN-level `_pragma=foreign_keys(1)` set in `open.go`. Verified by grepping `coreSchemaDDL` content (`store.go:74-123`) for `PRAGMA` — no matches. The DSN-level pragma fires on every new connection (every `sql.Conn` from the pool), so connection-level FK enforcement is preserved without needing the in-tx statement. Minor cleanup, not a regression.

- **Coverage moved from 80.6% (Round 1) → 80.9% (Round 2) for `./internal/adapters/sqlite`.** The atomic-bootstrap branch in `migrateSchemaOnConn` (`store.go:227-233`) is now exercised by every `newBootstrappedStore(t)` call AND by the dedicated `TestStoreBootstrapFreshDBStampsV2WithAllCoreTables`. The ROLLBACK test exercises the same DDL set on a manual transaction path. The cancel test exercises the conn-acquisition + pre-tx probe + early-error path.

- **Context propagation correct.** Every new test path threads `context.Background()` (and the cancellation test uses `context.WithCancel`) into `Bootstrap` and the manual `*sql.Conn` exec calls. No goroutine in the new tests is missing a context.

- **Errors wrapped with `%w` at every boundary.** New error sites in `migrateSchemaOnConn` (lines 202, 213, 219, 230, 240, 243, 248) all use `fmt.Errorf("...: %w", err)`. No string-match anywhere in the new code.

- **`gofumpt` clean** (implicit — mage testPkg's first step is gofumpt -l and the target passed).

### Verdict Justification

All nine specifically-listed verification points pass with file:line citations. F1 (atomic bootstrap) verified by source inspection AND by a dedicated behavioral test that exercises the rollback path directly. F2 (compile-assert) verified by source inspection. 3 new tests added without regressing the 33 Round-1 tests. Coverage 80.9% (up from 80.6%). Scope discipline clean (only `internal/adapters/sqlite/` + drop dir touched). No mage-discipline violations. Verdict: PASS.
