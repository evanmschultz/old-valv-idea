## Unit 14.1 — Round 1

Verdict: FAIL

### Counterexamples

1. Interrupted first bootstrap is misclassified as unsupported legacy v0. `Bootstrap` rejects `user_version=0` when any core table already exists (`internal/adapters/sqlite/store.go:74-89`), but it creates those same core tables before entering the migration transaction that stamps `PRAGMA user_version = 2` (`internal/adapters/sqlite/store.go:91-149`, `internal/adapters/sqlite/store.go:171-226`). A crash after any early `CREATE TABLE IF NOT EXISTS` leaves a fresh database in the exact fingerprint that the next process rejects forever. I reproduced this with a temporary test that seeded only `projects` and then called `Bootstrap`; `GOCACHE=/private/tmp/valv-gocache go test ./internal/adapters/sqlite -run TestReproInterruptedFreshBootstrapRejectedAsLegacyV0 -count=1` passed, confirming `errors.Is(err, domain.ErrUnsupportedSchema)`. Narrow fix: make fresh-db bootstrap atomic by moving the core-table DDL and `user_version` stamp under the same locked transaction, or persist an explicit bootstrap marker that distinguishes "interrupted fresh init" from true legacy v0.

2. The new `domain.AccountEnvRepository` contract has no package-boundary assertion and no consumers yet. The interface is declared at `internal/domain/repository.go:38-55`, and `sqlite.Store` grows the matching methods at `internal/adapters/sqlite/account_env.go:31-120`, but there is no `var _ domain.AccountEnvRepository = (*Store)(nil)` assertion in `internal/adapters/sqlite`, and `gopls` reports no references to `AccountEnvRepository` outside its own declaration. Narrow fix: add the compile-time assertion next to `type Store struct` if this interface must exist before Unit 14.2, or defer the interface until a consumer actually binds to it.

### YAGNI check

- `AccountEnvRepository` is currently one interface, one implementation, zero consumers. That is premature abstraction unless Unit 14.2 immediately binds manage-service code to the domain seam. Preferred shape: either keep the concrete `sqlite.Store` API for Unit 14.1 and extract the interface when the service layer needs it, or keep the interface but add the explicit compile-time assertion so the abstraction at least buys drift protection.

### Hidden dep check

- `Bootstrap` now depends on an implicit invariant: the very first bootstrap must survive long enough to reach `migrateSchemaOnConn` once. That dependency is hidden behind `hasAnyCoreTable` (`internal/adapters/sqlite/store.go:152-169`) instead of a durable marker, which is why a partially initialized fresh DB is indistinguishable from legacy v0 on restart.
- I did not find `init()` side effects, package-level mutable state, or test-order coupling in the changed files. The shipped two-connection race test is real: it launches two goroutines, gates them on a shared start channel, waits with `sync.WaitGroup`, and uses a shared file-backed database (`internal/adapters/sqlite/store_test.go:951-1015`).

## Unit 14.1 — Round 2

Verdict: FAIL

### Counterexamples

1. The legacy-v0 rejection is still bypassable in the unlocked gap between the read-only probe and `BEGIN IMMEDIATE`. `Bootstrap` reads `PRAGMA user_version` and runs `hasAnyCoreTable` before taking any write lock (`internal/adapters/sqlite/store.go:132-156`). `migrateSchemaOnConn` later begins the transaction and decides the v0 path solely from the locked `user_version` value (`internal/adapters/sqlite/store.go:200-245`); it does **not** re-check `sqlite_master` under lock. Concrete trace:
   - Connection A on a fresh DB executes the same pre-check as `Bootstrap`: `PRAGMA user_version` returns `0`, `hasAnyCoreTable` sees no core tables.
   - Before A reaches line 201, connection B commits `CREATE TABLE projects (...)`, leaving the exact unsupported fingerprint this code means to reject: core tables present, `user_version=0`.
   - A then enters `BEGIN IMMEDIATE`; inside the tx `PRAGMA user_version` is still `0`, so lines 223-244 take the "fresh database" branch and stamp `user_version = 2` instead of returning `domain.ErrUnsupportedSchema`.
   I verified separately that the transactional assumptions themselves are sound on this runtime: with the default `PRAGMA journal_mode=delete` from `Open` (`internal/adapters/sqlite/open.go:37-52`), `BEGIN IMMEDIATE; PRAGMA user_version = 2; ROLLBACK; PRAGMA user_version;` returns `0`, and rolled-back `CREATE TABLE` state is absent afterward. The remaining bug is the stale classification window, not rollback semantics. Narrow fix: after `BEGIN IMMEDIATE`, if `userVersion == 0`, re-run `hasAnyCoreTable` under the reserved lock before taking the fresh-init branch, or move the legacy/fresh classification under the same lock acquisition entirely.

2. `TestStoreBootstrapAtomicityCancelMidTransactionRollsBack` does not establish the condition in its own name. The test cancels the context **before** calling `Bootstrap` (`internal/adapters/sqlite/store_test.go:1099-1105`). But `Bootstrap` first does `s.db.Conn(ctx)` and a pre-transaction `PRAGMA user_version` read before it ever attempts `BEGIN IMMEDIATE` (`internal/adapters/sqlite/store.go:125-156`). So the test can pass even if cancellation never reaches an open transaction at all. That is a build/test bypass counterexample against the claimed evidence for "ctx-cancel mid-tx rolls back," not necessarily a production bug in the rollback path itself. Narrow fix: introduce a deterministic blocking point after `BEGIN IMMEDIATE` but before `COMMIT`, cancel only after that point is reached, and assert the first call actually entered the transaction rather than discarding its error.

### YAGNI check

- The new compile-assert is justified. `var _ domain.AccountEnvRepository = (*Store)(nil)` (`internal/adapters/sqlite/store.go:17-21`) buys concrete drift detection against the four-method interface in `internal/domain/repository.go:43-55`; I did not find a new unnecessary abstraction in the Round 2 delta.

### Hidden dep check

- The correctness of the "legacy v0 probe stays outside the tx" design now depends on an unstated environmental assumption: no other writer may create core-schema tables between the unlocked probe and `BEGIN IMMEDIATE`. That assumption is not enforced by the code and is the root cause of counterexample 1.
- `coreSchemaDDL` and `coreSchemaTables` are package-level mutable vars (`internal/adapters/sqlite/store.go:53-123`). Nothing in-tree mutates them today, so I am not filing a separate failure on test-order coupling, but the bootstrap path and the new rollback test both implicitly depend on those vars remaining immutable and ordered.
