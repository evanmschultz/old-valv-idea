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
