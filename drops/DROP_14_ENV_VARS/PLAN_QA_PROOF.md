# DROP_14 Unit 14.4 — Plan-QA Proof — Round 1

Agent: `ta-go-plan-qa-proof` (built-in via Agent tool, model=opus). Transcribed by orchestrator — the persona is READ-ONLY (no Edit/Write). Tool-call audit by orch: Read + read-only Bash (rg/git log/ls) + Context7 resolve only; 26 tool uses, all within persona allowlist + TA_ALLOWLIST embed. No Edit/Write, no mage, no git mutations.

Audits the Round-2 decomposition of Unit 14.4 (commit `21e402b`).

## 1. Verdict

**PASS-WITH-NITS** — the decomposition is sound on atomicity, blocker graph, and parallelization, and every cited symbol is grounded in committed code. Two NITs concern unstated production-surface implications in 14.4.2 (wrapper Store interface widening + fakeStore impact) and an ambiguity in 14.4.1's merge semantics. Both are tightenable in PLAN.md without re-decomposing.

## 2. Coverage Check

### 2.1 Atomic decomposition (measured per aa130dd)

**Unit 14.4.1 — internal/services/run/service.go**
- Distinct new/changed production symbols: **1** = `LaunchRequest` (field add); the merge edit lives inside the existing `buildRequest` function as an edit cluster (line 260-264 area) — same-purpose cluster, not a new symbol. **Under budget (<3).**
- Prod LOC estimate: **~25** (field decl + merge block + sort-stable iteration if applicable). **Under budget (≤80).**
- Test LOC estimate: **~80-120** (3 table-driven cases per acceptance bullet × runtime-key collision matrix). Test LOC is *separately* reported and not counted against the prod budget per the methodology rule. **OK.**
- Prod files: **1** (`internal/services/run/service.go`). **Under budget (≤3).**

**Unit 14.4.2 — internal/services/{claude,codex}/service.go**
- Distinct new/changed production symbols: **2** = `claude.Service.Run` edit cluster + `codex.Service.Run` edit cluster. Plan correctly forbids adding a per-wrapper helper symbol (PLAN.md line 110). **Under budget (<3).**
- Prod LOC estimate: **~50** (~25 per wrapper for Store accessor call + slice→map conversion + LaunchRequest field set). **Under budget (≤80).**
- Test LOC estimate: **~120-160** (one new test per wrapper × table-driven seeded entries + no-entries case, plus fakeStore method additions if widened — those are test-side). Reported separately. **OK.**
- Prod files: **2**. **Under budget (≤3).**

### 2.2 Symbol grounding (Read on committed checkout — Hylla MCP unavailable in this session)

- `internal/services/run.LaunchRequest` — confirmed at service.go:121-136; exact fields: `ProjectRoot, WorkingDir, ProjectID, ProfileID, ProjectName, Prepared, Args, Command`. **No `AccountEnv` today** → plan correctly marks new.
- `internal/services/run.Options` — confirmed at service.go:89-99; fields: `Executor, Image, User, TTY, Stdin, Logger, Notices, Now, Provider`. **No `Store`** → matches plan claim "Store-FREE".
- `claude.Service.Run` — confirmed at services/claude/service.go:127; passes `runservice.LaunchRequest{...}` at line 225-234.
- `codex.Service.Run` — confirmed at services/codex/service.go:120; passes `runservice.LaunchRequest{...}` at line 216-225.
- `manage.Service.ListAccountEnv(ctx, provider, accountName) ([]domain.AccountEnvEntry, error)` — confirmed at services/manage/service.go:683-694, ordered by `env_key ASC` via store contract (repository.go:50-52).
- `domain.AccountEnvEntry{ProfileID, EnvKey, EnvValue, CreatedAt, UpdatedAt}` — confirmed at domain/types.go:9-18.
- `manage.reservedAccountEnvKeys` — confirmed at services/manage/service.go:629-636, exactly the six keys.
- Runtime-owned 6 keys set in `prepared.Env` — confirmed at adapters/providers/claude/runtime.go:127-133 + 141 (`CLAUDE_CONFIG_DIR, HOME, LOGNAME, TERM, USER`, and conditionally `CODEX_HOME` on cross-mount) and adapters/providers/codex/runtime.go:117-123 + 131 (`CODEX_HOME, HOME, LOGNAME, TERM, USER`, conditionally `CLAUDE_CONFIG_DIR`).

### 2.3 Blocker graph

- 14.4.2 `blocked_by: 14.4.1` — REAL dep. `LaunchRequest.AccountEnv` must exist as a field before the wrappers can set it. Confirmed correct.
- No spurious blockers. 14.1 cross-unit reference is correct (Store accessor already lives in 14.1's committed code).

### 2.4 Parallelization

- 14.4.1 touches `internal/services/run/service.go` only. 14.4.2 touches `internal/services/claude/service.go` + `internal/services/codex/service.go` only. **Disjoint files; disjoint packages.** The only real ordering edge is the field-existence dep, correctly modeled as `blocked_by`. **OK.**

### 2.5 Acceptance testability

- `TestRunMergesAccountEnvIntoContainerEnv` — verifiable via `mage testPkg ./internal/services/run`; table-driven over collision cases. **OK.**
- `TestRunPreservesRuntimeOwnedEnvOnAccountEnvCollision` — verifiable, exhaustive over all 6 keys. **OK.**
- nil/empty `AccountEnv` case — explicit; **OK.**
- `TestRunPassesAccountEnvToSharedRunService` × 2 (one per wrapper) — verifiable via the named per-package `mage testPkg` targets. **OK.**

### 2.6 Open-question routing

Planner reported "None" — close to true but see NIT 3.2 (the merge-direction ambiguity should be a stated invariant, not an inferred one).

### 2.7 Hylla feedback (planner reported a miss on `run.Run` / `buildRequest`)

Confirmed the env-assembly seam exists at `internal/services/run/service.go:229-283` (`buildRequest`); line 264 assigns `Env: launch.Prepared.Env` — the exact line 14.4.1's edit cluster targets. The seam is real, in committed code, and the planner's design is implementable.

## 3. NITs

### 3.1 (14.4.2) Wrapper Store interface widening is implied but not stated

The committed wrapper Store interfaces — `claude.Store` (services/claude/service.go:23-27) and `codex.Store` (services/codex/service.go:22-26) — do NOT embed `domain.AccountEnvRepository`. Yet 14.4.2 says the wrappers "call the account-env accessor available in Store scope". That requires one of:
- (a) widening the wrapper Store interface to embed `domain.AccountEnvRepository`, OR
- (b) injecting a `*manage.Service` (or a minimal `AccountEnvLister` interface) into wrapper `Options`.

Either choice has cascading test-fixture work (`fakeStore` updates in both `services/claude/service_test.go` and `services/codex/service_test.go`). The plan should NAME the choice + acknowledge the fakeStore update is test-side. The methodology memory `feedback_interface_change_runs_full_mage_test.md` warns that interface-method additions break sibling-package mocks — 14.4.2's drop-end gate must run `mage test` (not just `mage testPkg`) to surface this; PLAN.md already requires `mage test` at the cross-drop level, so the gate is in place, but make the Store-widening decision explicit.

### 3.2 (14.4.1) Merge direction wording is imprecise

PLAN.md says the merge happens "before/around prepared-runtime env assembly, with the six runtime-owned keys winning on collision." Today `buildRequest` (line 264) sets `Env: launch.Prepared.Env` — `Prepared.Env` IS the source of truth. The actual edit is "combine `launch.AccountEnv` with `launch.Prepared.Env` such that any key already present in `Prepared.Env` overrides the account-env value." State this as an explicit invariant in the acceptance bullet so the builder doesn't accidentally let account-env keys clobber the runtime-owned ones (e.g. by writing `for k, v := range prepared.Env { merged[k] = v }; for k, v := range accountEnv { merged[k] = v }` — wrong order).

### 3.3 (14.4.1) Test name `TestRunPreservesRuntimeOwnedEnvOnAccountEnvCollision` should exercise the conditional 6th key

`CODEX_HOME` (claude runtime) and `CLAUDE_CONFIG_DIR` (codex runtime) are only set when `OtherProviderProfileHome` is non-empty (runtime.go:135-142 claude / runtime.go:125-132 codex). The run-service test seeds `Prepared.Env` directly — the test will pass all 6 keys deterministically without depending on cross-mount. The plan does not say this explicitly. Worth one bullet so the builder doesn't try to test the conditional cross-mount inside the run-service layer.

## 4. Failures

None on the proof axis. Verdict is PASS-WITH-NITS.

## 5. Hylla Feedback

Hylla MCP unavailable in this session — proof agent fell back to Read + rg. Planner's Round-2 Hylla miss on `run.Run` / `buildRequest` maps to a real seam confirmed via local file Read at `internal/services/run/service.go:229-283`. The miss likely traces to Hylla's `node_full` query not surfacing function-body LOC for editable seams.

## 6. Tools Used

- `Read` — `main/drops/DROP_14_ENV_VARS/PLAN.md`, `internal/services/run/service.go` (full), `internal/services/{claude,codex,manage}/service.go` (relevant slices), `internal/domain/repository.go`, `internal/domain/types.go`, `internal/adapters/providers/{claude,codex}/runtime.go` (env-assembly slice).
- `Bash` (read-only) — `git rev-parse HEAD`, `git log --oneline -5`, `ls`, `rg -n` for `ListAccountEnv` / `AccountEnvEntry` / `AccountEnvRepository` / `reservedAccountEnvKeys` / fakeStore.
- Context7 `resolve-library-id` for Hylla — not relevant (only Vaadin/Hilla matched); skipped `query-docs`.
- No `Edit`/`Write`. No git mutations. No mage.

## TL;DR

- T1: PASS-WITH-NITS — 3 NITs, no failures, decomposition is sound on the proof axis.
- T2: Both droplets are under budget on symbols, prod LOC, and prod files; every cited symbol is grounded in committed code; blocker graph and parallelization are correct.
- T3: NIT 3.1 — make the Store-widening-vs-injection choice explicit in 14.4.2; NIT 3.2 — pin the merge direction wording; NIT 3.3 — clarify the 6-key test seeds `Prepared.Env` directly.
- T4: None.
- T5: Hylla `keyword` worked for account-env symbols; `node_full` likely missed because the edit target is a body-level cluster in `buildRequest`, not a top-level symbol.
- T6: Read + read-only Bash + ripgrep + Context7 resolve only.
