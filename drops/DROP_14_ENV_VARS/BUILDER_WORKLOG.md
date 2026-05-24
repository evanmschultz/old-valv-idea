# DROP_14 — Builder Worklog

Append a `## Unit 14.M — Round K` section per build attempt. See `main/drops/WORKFLOW.md` § "Phase 4 — Build (per unit)" for what each section should contain.

## Unit 14.1 — Round 1

### Files Touched

- `internal/domain/errors.go` — added `ErrUnsupportedSchema` sentinel with Go-doc comment describing the [1, 2] supported window and the operator-facing semantics ("too old" vs "too new").
- `internal/domain/types.go` — added `AccountEnvEntry` type (ProfileID, EnvKey, EnvValue, CreatedAt, UpdatedAt) plus the `time` import.
- `internal/domain/repository.go` — added `AccountEnvRepository` interface (set / get / list / unset) keyed by `profile_id + env_key`. Plaintext `env_value` field. List ordered by `env_key ASC`.
- `internal/adapters/sqlite/open.go` — rewrote DSN policy: ordered `requiredPragmas` slice (`busy_timeout(5000)` first, then `foreign_keys(1)`); replaced `withPragma` with `applyRequiredPragmas` + `existingPragmaNames` + `parsePragmaName` helpers that do exact pragma-name parsing (trim whitespace, isolate substring before first `(` or `=`, lowercase, equality compare). Dedup rule explicitly handles the `busy_timeout_pragma=foo` false-prefix counterexample.
- `internal/adapters/sqlite/open_test.go` — replaced the two old dedup tests with `TestBuildDSNAppliesRequiredPragmas` (table-driven, 4 cases per Unit 14.1 acceptance) plus `TestBuildDSNFilePathAppliesBothPragmas` and `TestParsePragmaName`.
- `internal/adapters/sqlite/store.go` — deleted `migrateProjectBindings` + `isLegacyProjectBindingsShape` (legacy v0 migration logic). Added `minSupportedSchemaVersion=1`, `maxSupportedSchemaVersion=2`, `coreSchemaTables`, `hasAnyCoreTable`. New `Bootstrap` performs pre-DDL legacy-v0 detection (user_version=0 + any core table present → reject) before idempotent DDL. New `migrateSchemaOnConn` uses an explicit `BEGIN IMMEDIATE` on a dedicated `*sql.Conn` so the DSN-level `busy_timeout(5000)` absorbs concurrent first-open waits without surfacing `SQLITE_BUSY`. Both too-old (v0 with legacy tables) and too-new (>v2) cases wrap `domain.ErrUnsupportedSchema` with a message containing `user_version=N` and `[1, 2]`.
- `internal/adapters/sqlite/account_env.go` — new file. `accountEnvCreateTable` v2 DDL (PK on `profile_id + env_key`, FK to `profiles.id` ON DELETE CASCADE). `SetAccountEnv` (UPSERT preserving `created_at`, advancing `updated_at`), `GetAccountEnv`, `ListAccountEnv` (ORDER BY env_key ASC), `UnsetAccountEnv` (wraps ErrNotFound when no row affected). `scanAccountEnv` shared scanner.
- `internal/adapters/sqlite/store_test.go` — deleted the two v0-seeded tests (`TestStoreMigrationPreservesLegacyCodexBinding`, `TestStoreBootstrapIsIdempotentAfterMigration`'s v0 variant). Added `seedV1Schema` helper (writes v1 DDL + `PRAGMA user_version = 1`). New tests: `TestStoreBootstrapAdvancesV1ToV2`, `TestStoreBootstrapRejectsLegacyV0`, `TestStoreBootstrapRejectsForwardIncompatibleSchema`, `TestStoreBootstrapIsIdempotentAtV2`, `TestStoreBootstrapV1ToV2PreservesBindings`, `TestStoreAccountEnvCRUD`, `TestStoreAccountEnvDuplicateKeyAcrossProfiles`, `TestStoreAccountEnvSurvivesProfileRename`, `TestStoreBootstrapConcurrentFirstOpen` (sync.WaitGroup, two opens on a shared file path, asserts both succeed and final `user_version == 2`).

### Mage Targets Run

- `mage testPkg ./internal/domain` — 26 tests passed; coverage 85.2%.
- `mage testPkg ./internal/adapters/sqlite` — 33 tests passed; coverage 80.6%.
- `mage testPkg ./internal/cli` — 240 tests passed; coverage 67.6% (sanity check that downstream consumers still compile and pass).
- `mage testPkg ./internal/services/manage` — 30 tests passed; coverage 76.9% (sanity check).

### Design Notes

- **Busy policy at the DSN, not in code.** Per planner F1, busy_timeout lives in the DSN so every connection inherits it; we do not add a second retry layer in Go. Context7's modernc.org/sqlite docs confirm `_pragma=busy_timeout(5000)` is a documented DSN parameter applied to every new connection.
- **Pragma ordering matters.** `busy_timeout(5000)` is appended before `foreign_keys(1)` so the busy_timeout is already in effect when later pragmas execute. The modernc.org/sqlite driver applies DSN pragmas in declaration order.
- **Exact pragma-name dedup, not prefix matching.** The Round-1 implementation isolates the parsed pragma name by splitting at the first `(` or `=` rune, lowercasing, and equality-comparing. The false-prefix counterexample `_pragma=busy_timeout_pragma=foo` parses to name `busy_timeout_pragma`, which is NOT equal to `busy_timeout`, so `busy_timeout(5000)` is still appended. A dedicated table-driven test (`TestBuildDSNAppliesRequiredPragmas`) covers all four documented cases.
- **Legacy v0 detection happens BEFORE DDL.** A naive "check user_version after CREATE TABLE IF NOT EXISTS" would treat legacy v0 databases as fresh (because the DDL is idempotent and user_version stays at 0). Bootstrap now probes `sqlite_master` BEFORE running DDL: if any of the five core tables is already present and `user_version == 0`, the database is legacy v0 and rejected with `ErrUnsupportedSchema`. Brand-new databases (no tables, user_version=0) pass through and bootstrap straight to v2.
- **Concurrent first-open serialization.** `migrateSchemaOnConn` acquires a dedicated `*sql.Conn` via `s.db.Conn(ctx)`, then issues `BEGIN IMMEDIATE`. The reserved lock blocks any second writer until the first commits; the DSN `busy_timeout(5000)` absorbs the wait. The `TestStoreBootstrapConcurrentFirstOpen` test races two goroutines on two `*sql.DB` instances pointing at the same on-disk file and asserts both succeed with no SQLITE_BUSY and final `user_version == 2`. Database files are used here (not shared-cache memory DBs) because shared-cache short-circuits the file-level locking we want to exercise.
- **Idempotency at v2.** `migrateSchemaOnConn` only runs the account_env DDL + version bump when `user_version < 2`. Re-running Bootstrap on an already-v2 database is a no-op transaction that commits cleanly.
- **AccountEnvRepository keying.** Plan F1a / Unit 14.1 mandates ownership by `profile_id`, not account name, so duplicate `env_key` values across accounts are legal AND account renames preserve env entries. Both invariants are explicitly tested (`TestStoreAccountEnvDuplicateKeyAcrossProfiles`, `TestStoreAccountEnvSurvivesProfileRename`).
- **Deterministic list ordering.** `ListAccountEnv` uses `ORDER BY env_key ASC` so callers (especially `valv account env list` in Unit 14.3) get alphabetical output without re-sorting.

### Hylla Feedback

No Hylla miss this round — Context7's `/modernc-org/sqlite` entry covered DSN pragma semantics (`_pragma=busy_timeout(5000)` + multi-pragma ordering) directly. All Valv-local Go evidence came from `Read` of `open.go`, `store.go`, `store_test.go`, `repository.go`, `errors.go` plus the planner's explicit line references in `DROP_14_ENV_VARS/PLAN.md`.
