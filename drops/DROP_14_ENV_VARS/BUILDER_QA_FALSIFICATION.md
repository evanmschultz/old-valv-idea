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
