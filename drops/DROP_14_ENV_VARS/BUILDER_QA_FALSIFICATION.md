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

## Unit 14.1 — Round 3

Verdict: PASS

### Counterexamples

None confirmed. Twenty-two distinct attack angles were attempted against the Round 3 fixes (under-lock re-probe, `migrationHookBeforeCommit` hook, `context.Background()` rollback) and each either failed to construct a counterexample or yielded only mitigated/accepted findings recorded below. Independent `mage testPkg ./internal/adapters/sqlite` reproduced the builder's claim exactly: 37 tests, 0 failures, 80.8% coverage, `-race` clean.

### Attack Log

1. **Conn lifecycle across `migrateSchemaOnConn`.** Probed whether the `*sql.Conn` holding `BEGIN IMMEDIATE` could be released to the pool mid-tx. `Bootstrap` (`internal/adapters/sqlite/store.go:137-141`) acquires the conn via `s.db.Conn(ctx)`, `defer`s `conn.Close()`, and passes the SAME conn to `migrateSchemaOnConn` (`store.go:167`). `database/sql` semantics (verified via `go doc database/sql DB.Conn`: *"Queries run on the same Conn will be run in the same database session"*) guarantee the IMMEDIATE lock, the under-lock re-probe (`store.go:253-263`), all schema writes, the `ctx.Err()` check, the `COMMIT`, and the deferred `ROLLBACK` all execute on a single contiguous session. No pool-release window. Not a counterexample.

2. **Hook leak across tests.** `store.migrationHookBeforeCommit` is set per-`Store` instance (`store_test.go:1107`) and cleared per-test (`store_test.go:1170`). Both R3 tests create their own `NewStoreFromDB(db)` instance (`store_test.go:1103`, `1225`, `1268`), so a stale hook on one `Store` cannot bleed into another. Verified by reading the field declaration at `store.go:25` — instance-scoped, not package-scoped. Not a counterexample. (Note as future fragility: the field has no `sync.Mutex`; if a future test ever spawns two concurrent `Bootstrap` calls on the same `Store` with the hook set, that would be a race. No such test exists today.)

3. **Hook panic propagation.** If `migrationHookBeforeCommit()` panics, control unwinds through `migrateSchemaOnConn`'s `defer` block at `store.go:222-233`. `committed` is still `false`, so the deferred ROLLBACK on `context.Background()` fires before the panic propagates. The transaction's writes are rolled back even on a panicking hook. Not a counterexample.

4. **`context.Background()` rollback loses caller ctx tracing.** Confirmed real trade-off — OTel/trace contexts threaded through the caller would not propagate to the rollback exec. But the rollback is a low-level recovery operation on an error path, not a logical operation worth tracing, and the block-comment at `store.go:223-230` documents the explicit reason. Accepted, not a counterexample.

5. **Race test seeds post-race state instead of simulating the race.** Step 1-3 of `TestStoreBootstrapDetectsConcurrentV0TableCreationUnderLock` (`store_test.go:1223-1275`) intentionally seed the post-race fingerprint by rewinding `PRAGMA user_version = 0` while leaving the core tables present. That alone exercises only the outer pre-check at `store.go:154-165`, NOT the under-lock branch. The load-bearing assertion is Step 4 at lines 1283-1297: it opens a fresh `*sql.Conn` via `db.Conn(ctx)` and calls `racerStore.migrateSchemaOnConn(ctx, conn)` directly, bypassing the outer pre-check. The assertion `strings.Contains(underLockErr.Error(), "detected under lock")` at line 1295 distinguishes the under-lock error message (`store.go:259-262`) from the outer-probe error message (`store.go:161-163`) — the outer message does NOT contain `"detected under lock"`, so this assertion can only pass if the under-lock branch fired. Verified by inspection: Step 4 actually exercises the new code path, not just the pre-existing outer rejection. Not a counterexample.

6. **`errors.Is` wrap correctness for the new under-lock error.** `store.go:258-263` reads `fmt.Errorf("...: %w", domain.ErrUnsupportedSchema)` with `%w` at the trailing slot. Test `store_test.go:1292-1294` asserts `errors.Is(underLockErr, domain.ErrUnsupportedSchema)`. Mage run passes; the wrap is correct. Not a counterexample.

7. **`BEGIN IMMEDIATE` mode preserved.** `store.go:218` still reads `conn.ExecContext(ctx, BEGIN IMMEDIATE)`; the R3 delta only added code AFTER this point. Not a counterexample.

8. **Cancel-test goroutine cleanup if hook never fires.** Lines 1125-1132 wrap the `<-txEntered` receive in a 5-second `select`. On timeout, the test closes `releaseHook` and calls `t.Fatalf`. If the hook genuinely never fires, Bootstrap proceeds normally (no cancellation), commits successfully, and sends nil to `bootstrapErrCh` (buffered size 1, so the send does not block). The goroutine then exits cleanly. `t.Fatalf` exits the test goroutine before reading the channel, so the buffered send is harmless. No deadlock, no leaked goroutine past the package's test process. Not a counterexample.

9. **PRAGMA user_version rollback on `ROLLBACK`.** Re-verified the R2 evidence stands: with the default `journal_mode=delete` set in `internal/adapters/sqlite/open.go`, `PRAGMA user_version = N` inside a `BEGIN IMMEDIATE` ... `ROLLBACK` reverts to the pre-tx value. The cancel-mid-tx test asserts `post-cancel user_version == 0` at lines 1150-1157 AND `account_env table count == 0` at lines 1158-1167 — both pass on the actual `mage testPkg` run. Not a counterexample.

10. **Race detector vs. hook field write.** `store.migrationHookBeforeCommit = func(){...}` at line 1107 executes before `go func() { ... store.Bootstrap(cancelCtx) }()` at lines 1117-1120. Go memory model: writes before `go` happen-before the spawned goroutine's reads. The bootstrap goroutine reads the hook at `store.go:289`. After the test goroutine receives from `bootstrapErrCh` at line 1140, the bootstrap goroutine has returned, so the clear at line 1170 happens-after the last read. Race detector confirms clean on the actual run. Not a counterexample.

11. **Outer probe and under-lock probe on the same `*sql.Conn`.** Both `Bootstrap`'s outer probe (`store.go:151-165`) and the under-lock re-probe (`store.go:253-263`) run on the SAME `*sql.Conn` because `Bootstrap` passes its locally-acquired `conn` directly to `migrateSchemaOnConn` (`store.go:167`). The "race" closed by the fix is therefore between THIS conn and OTHER conns/processes — exactly what the block comment at `store.go:203-211` describes. Not a counterexample, but confirms the fix's threat model.

12. **Premature cancellation in cancel-mid-tx test.** Line 1114 creates `cancelCtx` from `context.Background()` (not cancelled). Bootstrap launches at 1118 with this ctx. The pre-tx PRAGMA read at `store.go:151` runs with non-cancelled ctx and succeeds. The BEGIN IMMEDIATE at `store.go:218` and all schema writes succeed. Only when the hook fires (line 289) and the test goroutine reaches line 1137 does `cancel()` execute. The cancellation window is therefore strictly inside the open transaction, after writes, before COMMIT — exactly the documented condition. Not a counterexample.

13. **`Store.Close()` mid-test.** No call to `store.Close()` appears in either R3 test. `t.Cleanup(func() { _ = db.Close() })` (lines 1101, 1219) runs after `t.Fatalf` or normal exit. By that point, the bootstrap goroutine has finished (sync via `<-bootstrapErrCh`). Not a counterexample.

14. **Race on `s.migrationHookBeforeCommit != nil` read.** Already covered by attack #10. The unsync'd field read at `store.go:289` is safe under the test's actual call pattern. Annotated as future fragility, not a counterexample.

15. **PRAGMA user_version scope (file vs. connection).** Sanity-checked: `PRAGMA user_version` is a 32-bit slot in the SQLite file header. It is persistent and visible to every connection on the same file. So the rewind at `store_test.go:1234` (on whatever conn `db.ExecContext` picks) is fully observable to the subsequent `racerStore.Bootstrap(ctx)` call and to the direct `db.Conn(ctx)` at Step 4. The test's race-fingerprint state is genuinely visible to the conn under test. Not a counterexample.

16. **Step 3 outer-probe rejection coverage.** `racerStore.Bootstrap(ctx)` at line 1269 calls into `Bootstrap`, which acquires its own conn from the same `*sql.DB`. That conn reads `PRAGMA user_version` (file-scope = 0) and runs `hasAnyCoreTable` (sees the still-present core tables). The outer pre-check at `store.go:154-165` rejects with `ErrUnsupportedSchema`. Test asserts `errors.Is(racerErr, domain.ErrUnsupportedSchema)` at line 1273. The block-comment at lines 1257-1267 explicitly accepts EITHER rejection (outer probe or under-lock branch). Both code paths produce a wrapping that satisfies `errors.Is`. Not a counterexample.

17. **Hook field cleared before recovery Bootstrap.** Line 1170 `store.migrationHookBeforeCommit = nil` happens after `<-bootstrapErrCh` (line 1140), so the bootstrap goroutine has completed. Recovery Bootstrap at line 1172 sees `nil` and skips the hook invocation (`store.go:289`). Recovery commits successfully and stamps v2. Test asserts user_version == 2 at line 1178-1180. Verified by `mage testPkg` run. Not a counterexample.

18. **Hook fires twice?** Single call site at `store.go:289`, single straight-line code path. The `if s.migrationHookBeforeCommit != nil` block is not in any loop and not duplicated. One `migrateSchemaOnConn` call → at most one hook invocation. The test's `close(txEntered)` therefore never double-closes. Not a counterexample.

19. **Test count drift.** Independent `mage testPkg ./internal/adapters/sqlite` (this falsification session, working directory `/Users/evanschultz/Documents/Code/hylla/valv/main`): `tests: 37, passed: 37, failed: 0, skipped: 0, Coverage: 80.8%`. Matches builder claim exactly. Not a counterexample.

20. **`%w` placement in new error.** `store.go:258-263`: format string has six `%`-verbs; the trailing one is `%w` with `domain.ErrUnsupportedSchema` as the argument. Single `%w` per `fmt.Errorf` (Go's wrap rules). Not a counterexample.

21. **Interface assertion still holds with new hook field.** Line 32 `var _ domain.AccountEnvRepository = (*Store)(nil)` is interface-shape assertion only; adding a non-method field (`migrationHookBeforeCommit`) cannot affect it. `mage testPkg` reaches the test phase, which means compile succeeded, which means the assertion held. Not a counterexample.

22. **`close(releaseHook)` then `t.Fatalf` order.** Timeout path at lines 1127-1131: `close(releaseHook)` runs BEFORE `t.Fatalf`. If a regression caused the hook to fire AFTER the timeout (timing fluke), the hook would land on an already-closed `releaseHook` channel and unblock immediately (closed-channel receive returns zero value). Bootstrap would proceed, attempt COMMIT (ctx still non-cancelled in this regression path), succeed, send to buffered `bootstrapErrCh`. No panic, no deadlock. `t.Fatalf` correctly signals the regression. Not a counterexample.

### YAGNI check

- `migrationHookBeforeCommit` is an unexported `func()` field on `Store` with one call site and one test consumer. Its purpose is documented (`store.go:16-25`) as a test-only synchronization seam to replace a timing-based race. The field has no constructor parameter, is `nil`-default no-op in production, and is the minimum surface needed to make the cancel-mid-tx test deterministic. Alternative approaches considered: (a) injecting a `chan struct{}` would require an exported accessor or a constructor change; (b) wiring a global package-level hook would create test-order coupling. The chosen shape buys deterministic mid-tx synchronization at the cost of one unexported field with prominent docstring. Acceptable. Not over-engineered.
- The under-lock re-probe at `store.go:253-263` is six lines of code in the same migration function on the same connection. No new helper, no new abstraction. Minimal fix for a real race window documented in R2 counterexample #1. YAGNI-clean.

### Hidden dep check

- `Store.migrationHookBeforeCommit` is unsynchronized mutable state on the `Store` receiver. The R3 tests are careful about set-before-spawn / clear-after-receive ordering, but the type system does not enforce this. If a future caller (test or production) mutates the field from one goroutine while another goroutine runs `Bootstrap` concurrently, the race detector will flag it. Recording as a fragility to surface in the closeout cycle, not a counterexample for this round.
- `context.Background()` rollback breaks the caller's `context` chain for telemetry purposes. Already covered under attack #4 — accepted trade-off, documented.
- The under-lock re-probe assumes `hasAnyCoreTable` is safe to call on a connection holding `BEGIN IMMEDIATE`. SQLite's RESERVED lock blocks other writers but allows same-connection reads on the connection's own snapshot. Verified by the live `mage testPkg` run (Step 4 of the race test successfully runs the read inside the open IMMEDIATE tx without `SQLITE_BUSY`). No hidden deadlock risk.
- I did not find any new `init()` side effects, package-level state mutation, or test-order coupling in the R3 delta beyond what was already noted in R2 (the `coreSchemaDDL`/`coreSchemaTables` immutable-vars caveat from R2 still applies, but R3 does not introduce new instances).

### Independent Mage Verification (falsification session)

```
mage testPkg ./internal/adapters/sqlite
[INFO] Started go test -json (-count=1 -race -cover ./internal/adapters/sqlite)
[PKG PASS] github.com/evanmschultz/valv/internal/adapters/sqlite (1.49s)
  tests: 37
  passed: 37
  failed: 0
  skipped: 0
  Coverage: 80.8% (gate: 60.0%)
[SUCCESS] All tests passed
```

Discipline clean: no `GOCACHE`, no raw `go test`, no sandbox bypass. Only `mage testPkg` invoked.

### Verdict Justification

Twenty-two attack angles attempted against the R3 fixes; none produced a confirmed counterexample. The under-lock re-probe closes the race window from R2 counterexample #1, the `migrationHookBeforeCommit` hook closes the false-coverage gap from R2 counterexample #2, and the `context.Background()` rollback patches a real driver-behavior subtlety (uncovered by builder's own test failure, per worklog) that would have leaked tx writes on cancel. Tests target the actually-fixed code paths, not just the surrounding scaffolding. Independent `mage testPkg` reproduces 37/37 at 80.8% coverage with `-race` clean. Two fragilities recorded for future attention (hook field is unsynchronized; rollback ctx loses telemetry chain) — both acceptable as documented trade-offs, neither rises to a falsification. Verdict: PASS.

## Unit 14.2 — Round 1

verdict: pass

Nine attack vectors attempted; none produced a confirmed counterexample.

### Attack log

1. **Regex anchoring** — both `^` and `$` present (`service.go:619`); Go RE2 requires full-string match. Internal-whitespace / newline / control-char cases rejected. Mitigated.
2. **Reserved-key lowercase bypass** — reserved keys are exact-case uppercase; `codex_home` (lowercase) is a distinct user var. Runtime adapters inject only uppercase forms, so no privilege-escalation path. Accepted design.
3. **Empty value** — key-only validation; empty VALUE accepted (correct POSIX `VAR=` semantics). Accepted.
4. **Unicode keys** — ASCII `[A-Za-z]` regex rejects `é`/`Ω`. Mitigated structurally.
5. **No length cap** — no max on key/value; no PLAN requirement. Accepted gap (noted).
6. **Idempotent Set** — UPSERT `ON CONFLICT(profile_id, env_key) DO UPDATE`; `TestSetAccountEnvOverwritesExistingValue` (`service_test.go:941-964`). Mitigated.
7. **Unset missing key** — `TestUnsetAccountEnvReturnsErrNotFoundWhenKeyMissing` (`service_test.go:995-1012`); store wraps ErrNotFound on RowsAffected==0. Mitigated.
8. **Error wrapping** — `%w` at all boundaries (Set :648/652/656, Unset :667/671/674, List :686/690). Mitigated.
9. **Reserved-key completeness** — all 6 present; verified against both runtime adapters. Mitigated.

### YAGNI

`AccountEnvEntryView` zero-cost alias, documented forward-stub for 14.3. `validateAccountEnvKey` has 3 call sites (pulls weight). No over-engineering.

### Hidden dep

No init() side effects beyond `regexp.MustCompile` (valid RE2, confirmed by suite). `reservedAccountEnvKeys` literal map, effectively immutable. No shared mutable state.

Verdict: pass.

## Unit 14.3 — Round 1

**Verdict:** pass-with-findings
**Reviewer:** ta-go-build-qa-falsification (codex gpt-5.5, `--sandbox read-only`, network=false; static analysis only); recorded by orchestrator. Audit: `.claude/agent-runs/20260526-163917-ta-go-build-qa-falsification-46543.tier1.codex-exec.out`.
**Reviewed at:** 2026-05-26

### Scope

Static counterexample review of commit `fe88a0d` (CLI `account env` subtree). `git show fe88a0d -- internal/cli/manage.go` + targeted test/service/store reads.

### Counterexamples / Attacks (all mitigated)

- **Default-path value leak:** mitigated. `writeEnvListJSON` + `writeEnvListLines` emit `***` unless `reveal` is true — human, plain, and JSON defaults all redact.
- **Generic JSON fallback:** mitigated. `runManageAccountEnvList` routes JSON to the dedicated `writeEnvListJSON` (top-level `env` + `redacted` bool), not `output.WriteListWithKey`.
- **Embedded `=` parsing:** mitigated. `strings.IndexByte` splits at the FIRST `=` only → `FOO=a=b` ⇒ key `FOO`, value `a=b`. No-`=` and empty-key rejected CLI-side before the service call.
- **Sort nondeterminism:** mitigated. CLI does not sort; the store query (`internal/adapters/sqlite/account_env.go`) uses `ORDER BY env_key ASC`, inherited by all three formats.
- **Leading `--reveal` (`list --reveal <name>`):** mitigated. Local cobra bool flag on `list`; the leading-position case is pinned by a test.
- **Empty list:** mitigated. Encodes as `{"env":[]}` (non-nil slice), not `null`.

### NIT (accepted — no fix round)

- N: reserved-key/key-regex service errors are double-context-wrapped — `runManageAccountEnvSet` wraps as `account env set: %w` while `Service.SetAccountEnv` already wraps as `set account env: %w`, giving `account env set: set account env: … HOME … reserved …`. `%w` identity is preserved (`errors.Is` works) and the error is not swallowed; the doubled prefix is cosmetic. Noted for DROP_17 polish.

### Falsification summary

- Confirmed counterexamples blocking PASS: 0. Core acceptance (redaction, dedicated JSON shape, first-`=` split, alpha sort, `--reveal` positions, empty-list) all hold.

**Verdict: pass-with-findings.**
