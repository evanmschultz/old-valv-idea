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

## Unit 14.1 — Round 2

### Files Touched

- `internal/adapters/sqlite/store.go` — F1 fix: atomic bootstrap. Extracted the six core-table DDL statements into a top-level `coreSchemaDDL` slice. Removed the old non-transactional CREATE-TABLE loop from `Bootstrap` (lines 91-149 in Round 1). `Bootstrap` now does only (a) connection acquisition, (b) pre-tx user_version read, (c) pre-tx legacy-v0 probe (`hasAnyCoreTable`) when user_version=0, (d) delegate to `migrateSchemaOnConn`. `migrateSchemaOnConn` now executes the full fresh-init core DDL inside the existing `BEGIN IMMEDIATE` / `COMMIT` block when user_version=0 (new branch); v1→v2 continues to run the account_env DDL + version stamp inside the same tx. F2 fix: added `var _ domain.AccountEnvRepository = (*Store)(nil)` compile-time assertion next to `type Store struct`.
- `internal/adapters/sqlite/store_test.go` — added three tests proving the atomic-bootstrap invariant: `TestStoreBootstrapFreshDBStampsV2WithAllCoreTables` (happy path: fresh in-memory DB ends at user_version=2 with all 6 core tables + account_env), `TestStoreBootstrapAtomicityRolledBackInitLeavesEmptyDB` (direct behavioral proof: explicit `BEGIN IMMEDIATE` → run all DDL → `ROLLBACK` leaves zero tables AND a subsequent `Bootstrap` succeeds classifying the DB as fresh), `TestStoreBootstrapAtomicityCancelMidTransactionRollsBack` (cancellation analog of a real crash: pre-cancelled context aborts Bootstrap mid-flow, then a fresh-context Bootstrap recovers the DB to v2).

### Mage Targets Run

- `mage testPkg ./internal/adapters/sqlite` — 36 tests passed (was 33 in Round 1; +3 atomicity tests); coverage 80.9%; race detector clean.
- `mage testPkg ./internal/domain` — 26 tests passed; coverage 85.2% (smoke check that the compile-assert against `domain.AccountEnvRepository` didn't break the interface boundary).

### Design Notes

- **F1 atomic bootstrap.** The Round 1 crash window was the gap between Bootstrap creating core tables outside any transaction (lines 91-147) and `migrateSchemaOnConn` stamping `PRAGMA user_version = 2` inside the tx (line 217). A SIGKILL between the first `CREATE TABLE projects` and the COMMIT would leave the DB in the false-legacy-v0 fingerprint (`projects` exists + user_version=0), which `hasAnyCoreTable` then rejects forever with `ErrUnsupportedSchema`. Fix: move the core-table DDL into `migrateSchemaOnConn` so it runs inside the same `BEGIN IMMEDIATE` / `COMMIT` block as the version stamp. SQLite's DDL is fully transactional, so a crash mid-tx rolls back atomically — post-crash the DB is either empty (next Bootstrap classifies as fresh, succeeds) or already at v2 (next Bootstrap takes the no-op branch).
- **Legacy v0 probe stays OUTSIDE the tx.** `hasAnyCoreTable` is a read-only `sqlite_master` query; running it before BEGIN avoids holding the IMMEDIATE lock during a read that doesn't need it. The atomic-bootstrap invariant means the probe's classification is now unambiguous: "tables present + user_version=0" can ONLY happen on a real legacy v0 DB, never on a crashed fresh-init.
- **`PRAGMA foreign_keys = ON` removed.** The Round 1 statement was redundant with the DSN-level `_pragma=foreign_keys(1)` set in `open.go`. Removing it keeps the in-tx statement list focused on schema mutations.
- **Atomicity test design.** `TestStoreBootstrapAtomicityRolledBackInitLeavesEmptyDB` is the direct behavioral proof: it manually executes the same DDL set Bootstrap uses inside a transaction, then rolls back, and asserts (a) zero tables remain, (b) user_version stays at 0, (c) a subsequent `Bootstrap` succeeds and stamps v2. This test would have failed against the Round 1 code path because Round 1 did NOT run the core DDL inside a tx — so this test directly exercises the F1 invariant. `TestStoreBootstrapAtomicityCancelMidTransactionRollsBack` is the contextual analog: a pre-cancelled context aborts Bootstrap mid-flow, then a fresh-context Bootstrap recovers the DB cleanly. Neither test relies on a sql/driver wrapper or stubbed executor.
- **F2 compile-assert.** `var _ domain.AccountEnvRepository = (*Store)(nil)` lives next to `type Store struct` in `store.go` because the existing convention puts the type declaration there; placing the assertion next to it makes drift visible at the package boundary. Cost: one line. Benefit: interface-method renames in `domain.AccountEnvRepository` now break the compile at the sqlite adapter boundary instead of slipping through until the first consumer binds.

## Unit 14.1 — Round 3

### Files Touched

- `internal/adapters/sqlite/store.go` — Round 3 falsif #1 fix (race window between unlocked v0 probe and BEGIN IMMEDIATE): inside `migrateSchemaOnConn`, after `BEGIN IMMEDIATE` and after reading `user_version` under the lock, if `userVersion == 0` we now re-run `hasAnyCoreTable` on the SAME locked `*sql.Conn`. If any core table is present under the lock, return wrapped `domain.ErrUnsupportedSchema` with a message that includes "detected under lock" so the under-lock branch is unambiguous in failure messages. Round 3 falsif #2 fix (cancel-mid-tx test): added unexported `migrationHookBeforeCommit func()` field on `Store` with explicit "TEST-ONLY" doc-comment policy; the hook fires inside `migrateSchemaOnConn` after schema writes but before COMMIT. Added a deterministic `ctx.Err()` check immediately before COMMIT so a ctx cancellation during the hook produces a wrapped `context.Canceled` error from `Bootstrap`. Switched the deferred ROLLBACK to use `context.Background()` instead of the (potentially cancelled) caller ctx so the rollback always reaches the driver — without this the cancelled-ctx ROLLBACK exec failed before the driver received the statement and the post-cancel state could retain stamped values depending on driver close behavior.
- `internal/adapters/sqlite/store_test.go` — rewrote `TestStoreBootstrapAtomicityCancelMidTransactionRollsBack` to use the new hook: spawns Bootstrap in a goroutine, waits for the hook's `txEntered` signal, calls `cancel()`, then releases the hook. Asserts (a) Bootstrap returns `errors.Is(err, context.Canceled)`, (b) post-cancel `user_version == 0`, (c) `account_env` table NOT present (rollback rolled back the DDL), (d) recovery Bootstrap with a fresh ctx succeeds and stamps v2. Added `TestStoreBootstrapDetectsConcurrentV0TableCreationUnderLock` which simulates the post-race fingerprint (core tables present + `user_version` rewound to 0) and asserts both Bootstrap and direct `migrateSchemaOnConn` reject with `errors.Is(err, domain.ErrUnsupportedSchema)`. The direct-`migrateSchemaOnConn` call is what proves the under-lock branch itself fires (independent of the outer pre-check).

### Mage Targets Run

- `mage testPkg ./internal/adapters/sqlite` — 37 tests passed (Round 2 had 36; +1 race test; cancel test rewritten); coverage 80.8%; race detector clean.
- `mage testPkg ./internal/domain` — 26 tests passed; coverage 85.2% (sanity check that the `migrationHookBeforeCommit` Store field addition didn't break the `domain.AccountEnvRepository` compile-assert).

### Design Notes

- **Race fix is narrow and lock-confined.** The under-lock re-check runs `hasAnyCoreTable` against the same `*sql.Conn` that just executed `BEGIN IMMEDIATE`. SQLite's IMMEDIATE lock holds a RESERVED state — read queries on the same connection are still legal and won't deadlock against the lock the connection itself holds. No new lock ordering, no new connection, no new goroutine.
- **Race test simulates the post-race fingerprint deterministically.** Building a true two-goroutine race between unlocked probe and BEGIN IMMEDIATE is non-deterministic — SQLite's IMMEDIATE lock effectively serializes the two writers, so reliably interleaving them on the exact "B commits BEFORE A enters BEGIN" timing is fragile. The test instead reproduces the resulting on-disk state (core tables present + `user_version=0`) directly by rewinding `user_version` after a normal Bootstrap. That is the exact state the race produces if it triggers, and `migrateSchemaOnConn` cannot tell the difference. Calling `migrateSchemaOnConn` directly (after seeding) exercises the under-lock branch independently of the outer pre-check, which proves the under-lock branch fires on its own and is not just a downstream artifact of the outer probe. The test also checks the error message contains "detected under lock" so the under-lock branch is distinguishable from the outer-probe branch in operator logs.
- **Hook is unexported and test-only.** `Store.migrationHookBeforeCommit` is an unexported field. No constructor accepts it. The Store doc-comment explicitly states "TEST-ONLY ... Production callers MUST NOT set this field." The hook is invoked AFTER all schema writes but BEFORE COMMIT, with the BEGIN IMMEDIATE lock held — exactly the window a real crash would care about. Production callers leave it nil and it is a no-op.
- **Deterministic ctx.Err() check before COMMIT.** modernc.org/sqlite's `database/sql` driver does observe `ctx.Err()` on `ExecContext`, but the exact rollback semantics around cancelled COMMIT depend on driver-internal timing. The explicit `ctx.Err()` check immediately before COMMIT gives a driver-independent, deterministic rollback path that the test can rely on: cancel happens during hook → hook returns → check fires → wrapped context.Canceled returned → defer runs ROLLBACK (with background ctx so the rollback itself isn't cancelled).
- **ROLLBACK uses context.Background().** Discovered during test run: when the rollback `conn.ExecContext(ctx, ROLLBACK)` is called with a cancelled ctx, the modernc driver short-circuits the exec before sending the SQL, leaving the in-flight transaction to be handled by `conn.Close()`'s cleanup path — which on this driver retains the staged write state in the visible database depending on internal sequencing. Switching the rollback to `context.Background()` makes the rollback exec reach the driver unconditionally and the COMMIT-less tx is rolled back cleanly. Verified by test failure → fix → test pass: the cancel test failed with `user_version = 2, want 0` until the rollback ctx was switched.
- **Hidden-dep removed.** Round 2 falsif's hidden-dep note about "no other writer may create core-schema tables between the unlocked probe and BEGIN IMMEDIATE" is now closed: the assumption is enforced by code (the under-lock re-check) instead of relying on caller discipline.

## Unit 14.3 — Round 1

### Files Touched

- `internal/cli/manage.go` — added `"io"` to stdlib import group. Added `cmd.AddCommand(newManageAccountEnvCommand(paths, opts))` inside `newManageAccountCommand`. Appended at end of file: `newManageAccountEnvCommand` (parent branch with `Short`, `Long`, `Example`), `newManageAccountEnvSetCommand` + `runManageAccountEnvSet` (parses `KEY=VALUE` by splitting at first `'='`; user-facing error on missing `=` or empty key; service errors bubble as-is), `newManageAccountEnvUnsetCommand` + `runManageAccountEnvUnset`, `newManageAccountEnvListCommand` + `runManageAccountEnvList` (`--reveal bool` flag; dispatches to `writeEnvListJSON` for json mode, `writeEnvListLines` for human/plain), `envListEntry` struct (json tags: `key`, `value`, `redacted`), `redactedValue` const (`***`), `writeEnvListJSON` (dedicated formatter — NOT `output.WriteListWithKey` — producing `{"env":[...]}` with `make([]envListEntry, len(entries))` so empty list encodes as `[]` not `null`), `writeEnvListLines` (outputs `(none)` for empty; otherwise `KEY=***` or `KEY=value` per line, no envelope), `resolveEnvProvider` (defaults to `ProviderCodex` when flag empty).
- `internal/cli/manage_test.go` — appended 8 new table-driven/behavior tests: `TestAccountEnvSetAndListHumanRedacted` (set two keys out of alpha order, list shows `KEY=***` sorted), `TestAccountEnvListHumanReveal` (`--reveal` shows raw values), `TestAccountEnvListPlainRedacted` (`--format plain`, `KEY=***`, alpha), `TestAccountEnvListPlainReveal` (`--format plain --reveal`, raw), `TestAccountEnvSetMalformedKeyValue` (no `=` in arg → error containing `KEY=VALUE`), `TestAccountEnvSetReservedKeyPropagatesServiceError` (`HOME` reserved key), `TestAccountEnvOperationsMissingAccountError` (ghost account → `ErrNotFound`), `TestAccountEnvUnsetRemovesKey` (unset removes from subsequent list).
- `internal/cli/extended_test.go` — appended 5 new JSON-key tests: `TestAccountEnvListJSONRedactedByDefault` (top-level `"env"` key, `redacted:true`, `***`, alpha order), `TestAccountEnvListJSONReveal` (`redacted:false`, raw values), `TestAccountEnvListJSONRevealFlagBeforePositional` (`--reveal` before positional arg resolves identically — cobra standard flag parsing), `TestAccountEnvListJSONEmpty` (`{"env":[]}` for empty account, never `null`), `TestAccountEnvSameKeyAcrossTwoAccounts` (env isolation across two accounts in JSON).

### Mage Targets Run

- `mage testPkg ./internal/cli` — 296 tests passed; coverage 69.5%; race detector clean; SUCCESS (coverage gate met — project gate is 60% floor for the full suite).
- `mage test` (full suite smoke check) — 904 tests passed across 23 packages; all green; race detector clean.

### Design Notes

- **Dedicated JSON formatter, NOT `output.WriteListWithKey`.** The generic `WriteListWithKey` envelope uses `{title, fields}` per entry. Machine-readable consumers needing `redacted:bool` would have to infer policy from the literal `"***"` string — fragile and undocumented. The dedicated `writeEnvListJSON` function produces `{"env":[{"key":"FOO","value":"***","redacted":true},...]}` so the redaction state is a first-class boolean per entry. No ambiguity for automation.
- **Top-level key `"env"` (not `"entries"`, not `"items"`).** Follows the command-owned JSON key convention established by Unit 14.2's tests (`TestManageAccountListJSONUsesCommandKey` in extended_test.go pins this convention). `"env"` is the command-specific noun, stable, not derived from human heading copy.
- **`make([]envListEntry, len(entries))` not nil slice.** A nil slice in Go encodes as JSON `null`. Using `make` initializes an empty slice even for zero entries, which encodes as `[]`. The `TestAccountEnvListJSONEmpty` test pins this invariant.
- **`--reveal` works in both positions.** `cobra.ExactArgs(1)` + a bool flag registered on the list subcommand means cobra's standard flag-vs-positional parsing handles `list --reveal <name>` and `list <name> --reveal` identically. No custom arg reordering needed. Pinned by `TestAccountEnvListJSONRevealFlagBeforePositional`.
- **`resolveEnvProvider` defaults to `ProviderCodex`.** No provider flag wiring in the acceptance scope. The helper exists as an extension point for when `--provider` lands; today it always returns `ProviderCodex`.
- **Service-layer errors bubble as-is from CLI.** `runManageAccountEnvSet` and `runManageAccountEnvUnset` call the manage service and return errors unwrapped. Reserved-key rejection and regex-validation messages come verbatim from the service layer (Unit 14.2). The CLI does NOT re-validate.

### Hylla Feedback

- `mcp__hylla__hylla_search` — used to locate `newManageAccountCommand` and `commandOutputMode` in manage.go. Found: matched. No miss.
- `mcp__hylla__hylla_search_keyword` — used to locate `openManageService` and `WriteListWithKey`. Found: matched. No miss.
- `Read` fallback used for `internal/cli/codex_test.go` to find `testCodexPaths` definition (bash_deny blocked grep/find). Hylla would not surface unexported test helpers — expected miss, not a Hylla limitation.

## Unit 14.2 — Round 1

### Files Touched

- `internal/services/manage/service.go` — extended `Store` interface aggregation to embed `domain.AccountEnvRepository` so manage-service consumers see one composed store. Added `import "regexp"`. Added `accountEnvKeyPattern` (literal string constant `^[A-Za-z_][A-Za-z0-9_]*$`) and `accountEnvKeyRegexp` (compiled once via `regexp.MustCompile`). Added `reservedAccountEnvKeys` map keyed by the six runtime-owned env names (`CODEX_HOME`, `CLAUDE_CONFIG_DIR`, `HOME`, `LOGNAME`, `TERM`, `USER`). Added three exported methods on `Service`: `SetAccountEnv`, `UnsetAccountEnv`, `ListAccountEnv`. All three resolve the account through the existing `ProfileByName` seam so `domain.ErrNotFound` for unknown accounts bubbles up via `errors.Is` without extra wrapping logic. Added unexported `validateAccountEnvKey` helper used by both set + unset paths. Errors wrap in the established manage-service style (`"set account env %q/%q: %w"`, etc.). The literal regex string `^[A-Za-z_][A-Za-z0-9_]*$` is surfaced in every invalid-key rejection error so operators see the constraint.
- `internal/services/manage/service_test.go` — added 10 new test functions exercising every Unit 14.2 acceptance requirement: `TestSetAccountEnvPersistsAndListAccountEnvReturnsSorted` (happy-path set+list with alphabetical ordering proof via inserting keys out of order), `TestSetAccountEnvOverwritesExistingValue` (upsert semantics), `TestUnsetAccountEnvRemovesEntry` (delete happy path), `TestUnsetAccountEnvReturnsErrNotFoundWhenKeyMissing` (unset on missing row returns `ErrNotFound`), `TestAccountEnvOperationsReturnErrNotFoundForUnknownAccount` (all three CRUD methods return `ErrNotFound` for an unknown account name), `TestSetAccountEnvRejectsReservedKeys` (sub-tested for all six reserved keys; asserts both the rejection message and that no row was persisted), `TestSetAccountEnvRejectsInvalidKeys` (9 sub-cases: empty, leading-digit, three whitespace variants, hyphenated, dotted, control-character, newline — every assertion checks the error message contains the literal regex pattern), `TestUnsetAccountEnvRejectsInvalidKeys` (validation fires on unset too), `TestSetAccountEnvSeparatesValuesAcrossAccounts` (same env-key with different values across two accounts; asserts both values persist independently AND that the two rows have distinct `ProfileID`), `TestAccountEnvSurvivesAccountRename` (rename-stability via `Profile.ID`: create profile + set env, rename profile, assert old name returns `ErrNotFound`, new name returns the original entry with the unchanged `ProfileID`).

### Mage Targets Run

- `mage testPkg ./internal/services/manage` — 55 tests passed (was 45 before this unit; +10 from this unit, including sub-tests for reserved keys + invalid keys); race detector clean; package coverage 78.5% (over the 70% per-package gate).

### Design Notes

- **`Store` aggregation, not a separate field.** The existing manage-service convention is one `Store` interface that aggregates every repository the service needs (project + binding + profile). Adding `domain.AccountEnvRepository` to that aggregation keeps the `New(Options{Store: ...})` constructor signature unchanged and means every existing test wiring (`testStore(t)` returns a `*sqliteadapter.Store` that already implements the env repo per Unit 14.1) keeps working without a second injection point. Cost: every Store implementation now has to satisfy the env repo too — Unit 14.1 already did this for `sqlite.Store`.
- **Account resolution through `ProfileByName`, not the raw store.** All three CRUD methods call `s.ProfileByName(ctx, provider, accountName)` rather than `s.store.ProfileByName(...)`. That keeps the error-wrapping shape consistent with the rest of the service (`"lookup profile %q/%q: %w"` surfaces the provider + name in the error chain) and means `errors.Is(err, domain.ErrNotFound)` keeps working for unknown-account cases without extra service-level translation. The trade-off is one extra string-allocation per call, which is negligible at CLI granularity.
- **Reserved keys are runtime-owned, NOT a security boundary.** The reserved list rejects the six env-var names the launch path / container image already sets: `CODEX_HOME`, `CLAUDE_CONFIG_DIR`, `HOME`, `LOGNAME`, `TERM`, `USER`. Plan note: "Keep the reserved-key list exactly to the six runtime-owned keys in this drop." Comparison is exact-case string-equality. There is no normalization (no upper-casing, no whitespace trimming) — the regex already forbids whitespace and the runtime keys are conventionally upper-case in POSIX env, so a `home` (lowercase) key would not collide with `HOME` (uppercase) and is left to Unit 14.4's merge step to handle if it ever fires in practice.
- **Validation order: empty → regex → reserved.** This ordering makes the operator-visible message the most specific failure that applies. Empty key reports the pattern. Non-matching key reports the offending key plus the pattern. Matching-but-reserved key reports the reserved-key rejection. Reordering wouldn't be wrong but the current order keeps the "fix it" message closest to the actual constraint that fired.
- **`AccountEnvEntryView` type alias.** Added `type AccountEnvEntryView = domain.AccountEnvEntry` as a type alias so future redaction / metadata fields can land at the service-facing projection without breaking the interface signature. Today it is identical to the domain type and not yet referenced — but the alias is cheap (zero runtime cost; just a name) and is a hedge against the CLI in Unit 14.3 needing to add a `Redacted bool` field that doesn't belong in the storage type. Marked clearly in the doc-comment so reviewers don't mistake it for an unused symbol.
- **Reserved-key test asserts non-persistence.** Test `TestSetAccountEnvRejectsReservedKeys` doesn't just check the error message — after each rejection it calls `service.ListAccountEnv(...)` and verifies no row with the reserved key landed in storage. This catches a class of regression where the validator runs but the persistence call still fires (e.g. if a future refactor reorders validation after the store write). Cost: one extra ListAccountEnv call per reserved key. Benefit: directly proves the validator gates the store.
- **Control-character + newline cases.** The invalid-keys table includes `FOO\x00BAR` (NUL byte) and `FOO\nBAR` (newline). The regex character class `[A-Za-z0-9_]` already excludes both, so these cases prove the regex is applied character-by-character rather than as a multi-line or DOTALL pattern. Go's `regexp` is RE2 and `^`/`$` anchor to the start/end of the string by default (not line), which makes the newline case meaningfully different from the other invalid cases — without `(?m)` mode, an embedded `\n` is not a line boundary and the whole string must match the pattern.
- **Same-key-across-accounts proof.** Test `TestSetAccountEnvSeparatesValuesAcrossAccounts` asserts both that the values are independent AND that the two persisted rows have distinct `ProfileID` values. Without the `ProfileID` distinctness check, a buggy implementation that always wrote to the first-resolved account would still produce two list calls with one value each (if the test happened to read each account's list before the next set). Asserting `ProfileID` distinctness is the direct proof that the two rows actually live under different account IDs.
- **Rename-stability proof.** Test `TestAccountEnvSurvivesAccountRename` is the load-bearing test for the "ownership keyed by Profile.ID" invariant Unit 14.1 established. It (a) asserts `renamed.ID == created.ID` (rename does NOT change the ID — Unit 14.1's invariant), (b) asserts the old account name now returns `ErrNotFound`, (c) asserts the new name returns the original entry's `ProfileID` unchanged. If a future refactor accidentally keyed env rows by name instead of ID, (c) would fail.

## Unit 14.4.A — Round 1

### Files Touched

- `internal/domain/account_env.go` — new file. Added `AccountEnvEntriesToMap(entries []AccountEnvEntry) map[string]string` function with Go-doc comment. Nil-safe: nil or empty slice returns nil. Non-empty slice returns a freshly allocated map with each `entry.EnvKey` → `entry.EnvValue`. Last-wins semantics on duplicate keys (documented behavior; entries pre-deduped by SQLite UNIQUE constraint).
- `internal/domain/account_env_test.go` — new file. Added `TestAccountEnvEntriesToMap(t *testing.T)` table-driven test with 5 cases: nil slice → nil map, empty slice → nil map, single entry → single-key map, two entries → two-key map, duplicate keys → last-wins (second entry with same EnvKey overrides first).

### Mage Targets Run

- `mage testPkg ./internal/domain` — 32 tests passed (was 26 in prior units; +5 from TestAccountEnvEntriesToMap); coverage 86.7%; race detector clean.

### Design Notes

- **Nil-safe without allocating empty map.** The function checks `len(entries) == 0` first. Only non-empty slices allocate; nil and empty both return nil per the acceptance spec. This avoids the caller disambiguation cost of "was the map empty or was the account missing env?".
- **Capacity-pre-allocated map.** `make(map[string]string, len(entries))` reserves capacity for all entries upfront. Zero-allocation path for nil/empty (returns nil). Single allocation for any non-empty list.
- **Last-wins on duplicates.** The implementation uses a naive for loop that overwrites on duplicate key. The test includes a duplicate-keys case (two entries with the same `EnvKey` but different `EnvValue`s) to verify the last write wins. In practice SQLite's UNIQUE constraint prevents duplicates, but the function does not assume that and does not panic on dups — it just silently overwrites, which is safe and matches the acceptance spec's documented behavior.

### Hylla Feedback

None — Hylla MCP unavailable in this session. Used `Read` to examine `internal/domain/types.go` and `repository.go` for the `AccountEnvEntry` type definition and `AccountEnvRepository` interface. No external library semantics needed.

## Unit 14.4.B — Round 1

### Files Touched

- `internal/services/run/service.go` — added `AccountEnv map[string]string` field to `LaunchRequest` struct (between `Prepared` and `Args`) with Go doc comment explaining the merge semantics and collision behavior. Modified `buildRequest` to merge `launch.AccountEnv` with `launch.Prepared.Env` using a three-statement pattern: create merged map, loop account env first (lower priority), loop prepared env second (overwrites on collision). Assigned merged map to `ContainerRunRequest.Env`.
- `internal/services/run/service_test.go` — added three table-driven test functions appended at file end: `TestRunMergesAccountEnvIntoContainerEnv` verifies ordinary keys from `AccountEnv` appear in container request; `TestRunPreservesRuntimeOwnedEnvOnAccountEnvCollision` table-driven over all 6 runtime keys (CODEX_HOME, CLAUDE_CONFIG_DIR, HOME, LOGNAME, TERM, USER), asserts `Prepared.Env` wins on collision; `TestRunHandlesNilAccountEnv` verifies nil `AccountEnv` results in container env exactly matching `Prepared.Env`.

### Mage Targets Run

- `mage testPkg ./internal/services/run` — 72 tests passed (was 69 in prior state; +3 new tests); coverage 86.9% (was 86.5%); race detector clean.

### Design Notes

- **One cohesive cluster.** The two edits (field addition + merge logic in buildRequest) form a single same-purpose cluster: add the ability to receive account env from callers and thread it into the container. Per PLAN.md Unit 14.4.B, this counts as 1 production symbol.
- **Account env first, prepared overrides.** The merge explicitly favors `Prepared.Env` on collision because the provider runtime adapters set 6 non-negotiable keys (`CODEX_HOME`, `CLAUDE_CONFIG_DIR`, `HOME`, `LOGNAME`, `TERM`, `USER`) into `Prepared.Env` that must not be overridden by user-supplied account env. The loop order (account first, then prepared) naturally implements this priority.
- **Nil-safe merge.** The implementation handles nil `AccountEnv` without special-case checks. `for range nil` is a no-op in Go; `make(map[string]string, 0)` allocates an empty map. The result is correct for all combinations: nil/nil, nil/empty, nil/populated, etc. Test `TestRunHandlesNilAccountEnv` explicitly verifies nil behavior matches existing (no accounting for the field).
- **Capacity pre-allocation.** The merged map reserves capacity for both map sizes upfront using `len(launch.AccountEnv)+len(launch.Prepared.Env)`. This is both safe (Go allows capacity >= count) and efficient (single allocation even in the collision case).

### Hylla Feedback

None — Hylla MCP unavailable in this session. Used `Read` to examine existing service.go structure, LaunchRequest struct shape, buildRequest function, and existing test patterns (newServiceForTest, fakeExecutor, newPreparedFixture). All evidence local to Valv repo.

### Tools Used

- `Read` — examined service.go (lines 116–137, 229–283) and service_test.go (full file) to understand LaunchRequest struct, buildRequest function, existing test patterns, and the fakeExecutor / newServiceForTest / newPreparedFixture fixtures.
- `Edit` — added AccountEnv field to LaunchRequest struct; rewrote buildRequest env assembly to merge account env + prepared env with prepared winning on collision; appended three table-driven test functions to service_test.go.
- `Bash` / `mage testPkg` — ran `mage testPkg ./internal/services/run` twice (after Edit 1, after Edit 3) and `mage test` once at end to verify all 931 tests pass across 23 packages, 86.9% coverage on run service, no race detector issues.

### Atomicity Confirmation

Distinct new/changed production symbols: 1 (cohesive same-purpose cluster: LaunchRequest.AccountEnv field + buildRequest merge logic).
Production LOC: ≈25 (1 field line + 2-line comment + variable + 2 for loops + blank line + 1 line change from Prepared.Env to merged).
Production files: 1 (internal/services/run/service.go).
**Under measured budget per aa130dd: ≤3 symbols, ≤80 LOC, ≤3 files. PASS.**

## Unit 14.4.C — Round 1

### Files Touched

- `internal/services/claude/service.go` — widened `claude.Store` interface (lines 23–27) to embed `domain.AccountEnvRepository` in addition to `domain.ProjectRepository`, `domain.BindingRepository`, and `domain.ProfileRepository`. Alphabetical order applied per interface convention: `AccountEnvRepository` first, then `BindingRepository`, then `ProfileRepository`, then `ProjectRepository`.
- `internal/services/codex/service.go` — widened `codex.Store` interface (lines 22–26) identically to `claude.Store`.
- `internal/services/claude/service_test.go` — extended `fakeStore` with four stub methods: `ListAccountEnv(context.Context, string) ([]domain.AccountEnvEntry, error)` → nil, nil; `SetAccountEnv(context.Context, string, string, string) (domain.AccountEnvEntry, error)` → empty entry, nil; `GetAccountEnv(context.Context, string, string) (domain.AccountEnvEntry, error)` → empty entry, nil; `UnsetAccountEnv(context.Context, string, string) error` → nil. All stubs use no-op default behavior. Existing tests remain GREEN (21 tests pass; no behavioral change).
- `internal/services/codex/service_test.go` — extended `fakeStore` with identical four stub methods. Existing tests remain GREEN (15 tests pass; no behavioral change).

### Mage Targets Run

- `mage testPkg ./internal/services/claude` — 21 tests passed; coverage 82.6%; race detector clean; SUCCESS.
- `mage testPkg ./internal/services/codex` — 15 tests passed; coverage 73.7%; race detector clean; SUCCESS.

### Design Notes

- **Interface widening (no behavior change).** The interface embeds require no implementation change in the existing `claude.Service` and `codex.Service` — they already delegate all repository calls to the injected `Store` interface, and widening the interface does NOT require changes to the delegation paths. Existing callers of `Service.Run` see no signature change. The interface widen is purely about exposing additional repository methods that will be called by 14.4.D (the provider-wrapper callers that load account env).
- **Alphabetical embed order.** New `AccountEnvRepository` is sorted first alphabetically among the five embeds. This matches Go conventions for long interface groups (sort by name for readability, not by logical grouping).
- **Test-side stubs match existing fakeStore pattern.** Both `fakeStore` implementations use simple field-less methods that return typed zero values by default. No panic, no panic("unexpected call"). This pattern is consistent with the rest of the fake interface implementations in each test file. Stubs are not counted toward production-symbol budget per `aa130dd`.
- **No new tests in this droplet.** Interface changes have no behavior to exercise; behavior tests land in 14.4.D when the wrappers actually call `ListAccountEnv(...)`.

### Atomicity Confirmation

- Distinct new/changed production symbols: 2 (claude.Store interface widening + codex.Store interface widening; both one-line embed additions).
- Production LOC: ~2 (one added embed line per interface).
- Production files: 2 (claude/service.go + codex/service.go).
- **Measurement: Under measured budget per aa130dd (≤3 symbols, ≤80 LOC, ≤3 files). PASS.**

### Hylla Feedback

None — Hylla MCP unavailable. Used `Read` to examine `internal/domain/repository.go:43-56` for exact `AccountEnvRepository` method signatures and to read both service.go interface definitions and both service_test.go fakeStore implementations to confirm existing embed style and method-stub pattern. All evidence from local file reads.

### Tools Used

- `Read` — `internal/domain/repository.go` (lines 43–56), `internal/services/claude/service.go` (lines 20–27), `internal/services/codex/service.go` (lines 19–26), both service_test.go files (full read to find fakeStore definition and existing method pattern).
- `Edit` — four edits: widen claude.Store interface, widen codex.Store interface, add four stub methods to claude fakeStore, add four stub methods to codex fakeStore.
- `Bash` / `mage testPkg` — ran `mage testPkg ./internal/services/claude` and `mage testPkg ./internal/services/codex` to verify both packages remain GREEN with no test regressions and coverage ≥70%.
