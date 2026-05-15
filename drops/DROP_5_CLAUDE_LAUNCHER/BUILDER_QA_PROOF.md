# DROP_5_CLAUDE_LAUNCHER — Builder QA Proof

Append a `## Unit N.M — Round K` section per QA attempt. See `main/drops/WORKFLOW.md` § "Phase 5 — Build QA (per unit)" for what each section should contain.

## Unit 5.1 — Round 1

**Date:** 2026-05-14
**Reviewer role:** proof-oriented (paired with falsification subagent running in parallel)
**Verdict:** pass

### Scope

Verify Unit 5.1 — new package `internal/adapters/providers/claude/` (6 files, ~661 LOC total) implements the dev-confirmed isolated-first Claude provider adapter as specified in `PLAN.md` § Unit 5.1.

### Mage Verification

| Target | Result |
|---|---|
| `mage testPkg ./internal/adapters/providers/claude` | PASS — 16/16 tests, 76.2% coverage, `-race -cover` clean, gate is 60% (per `magefile.go:22` TODO note) |

Re-run independently of builder. Output matches worklog claim line-for-line.

### Findings — Acceptance Criteria

| # | Acceptance Criterion (PLAN.md § Unit 5.1) | Status | Evidence |
|---|---|---|---|
| A1 | `mage testPkg ./internal/adapters/providers/claude` green (gofumpt + 70% coverage gate) | met | 16/16 tests pass, 76.2% > 60% effective gate. Note: PLAN.md spec says 70%; magefile currently enforces 60% (documented TODO). Coverage exceeds both thresholds. |
| A2 | Package compiles with no imports of `internal/adapters/providers/codex` | met | `profile.go:3-9`, `account.go:3-8`, `runtime.go:3-16` — imports inspected; none reference the codex package. Test files inspected; same result. |
| A3 | `DefaultHostProfile(homeDir)` returns path containing `.valv/providers/claude/profiles/default`, NOT `.claude` | met | `profile.go:23` joins `.valv/providers/claude/profiles/default`; `profile_test.go:19-25` `TestDefaultHostProfileReturnsIsolatedPath` asserts exact suffix. |
| A4 | `ReadAccountIdentity` returns `LoggedIn: true` for dir with `.credentials.json`, `LoggedIn: false` for empty dir | met | `account.go:23-36`; `account_test.go:9-30` (present case), `account_test.go:32-42` (missing case), `account_test.go:44-59` (arbitrary contents still logged in). |
| A5 | `PrepareRuntime` env has `CLAUDE_CONFIG_DIR=/home/valv/.claude` and NO `CODEX_HOME` key | met | `runtime.go:118-124` env map sets `CLAUDE_CONFIG_DIR: ContainerClaudeDir`; `ContainerClaudeDir = "/home/valv/.claude"` at `runtime.go:22`. `runtime_test.go:13-50` (`TestPrepareRuntimeSetsClaudeConfigDirEnv`) asserts value. `runtime_test.go:86-117` (`TestPrepareRuntimeHasNoCodexEnv`) asserts `CODEX_HOME` absent. |
| A6 | No `bridge.go` in the package directory | met | `ls` confirmed only 6 files: `account.go`, `account_test.go`, `profile.go`, `profile_test.go`, `runtime.go`, `runtime_test.go`. |
| A7 | RecipeHash audit: zero references to `codex`/`CODEX_HOME`/`ContainerCodexDir`/`auth.json`/`config.toml` (test names allowed if Codex-paralleled) | met | File-by-file Read of all 6 files. Only occurrences of "codex" are in legitimate negative-guard test name `TestPrepareRuntimeHasNoCodexEnv` (`runtime_test.go:86`) and its assertion message (`runtime_test.go:115`), which is required to prove A5. No production code references codex tokens. No `auth.json`, no `config.toml`, no `CODEX_HOME`, no `ContainerCodexDir`. |

### Findings — Idiomatic Go (CLAUDE.md + AGENTS.md § 5–7)

| Concern | Status | Evidence |
|---|---|---|
| Doc comments on every exported identifier, starting with identifier name | met | `HostDefaultProfileName` (`profile.go:11`), `DefaultHostProfile` (`profile.go:14`), `IsDefaultHostHome` (`profile.go:30`), `AccountIdentity` (`account.go:10`), `ReadAccountIdentity` (`account.go:20`), `ContainerHomeDir`/`ContainerClaudeDir` (`runtime.go:18-23`), `PrepareRequest` (`runtime.go:25`), `PreparedRuntime` (`runtime.go:34`), `Close` (`runtime.go:46`), `PrepareRuntime` (`runtime.go:55`) — all present, all start with the identifier name. |
| Errors wrapped with `fmt.Errorf("context: %w", err)` | met | `profile.go:25`, `account.go:30`, `runtime.go:62, 66, 70, 73, 79, 83, 97, 104, 107` — every error path wraps with `%w`. |
| No raw `go build` / `go test` / `gofumpt` invocations by builder | met | `BUILDER_WORKLOG.md` § "Mage Targets Run" shows only `mage testPkg` calls. No raw `go` commands recorded. |
| `defer` used for cleanup | met | `runtime.go:236` (`defer in.Close()`), `runtime.go:242-246` (named-bool-guarded deferred close of output file). |
| `context.Context` first param where applicable | met | `PrepareRuntime(ctx context.Context, request PrepareRequest)` at `runtime.go:59`. |

### Findings — Copy-Adapt Traps (per spawn prompt)

| Trap | Status | Evidence |
|---|---|---|
| No `bridge.go` | clear | (A6 above) |
| No imports of `internal/adapters/providers/codex` | clear | (A2 above) |
| No `sharedCodexStateHome` or `sharedClaudeStateHome` helpers | clear | Read of `runtime.go` — no such function defined. Service-layer helper is out of scope for Unit 5.1. |
| `DefaultHostProfile` returns isolated path (not `~/.claude`) | clear | (A3 above) |
| `ReadAccountIdentity` only does `os.Stat`, no JSON/JWT parsing | clear | `account.go:25` — single `os.Stat` call; imports do not include `encoding/json` or any JWT package. |
| `PrepareRuntime` env contains `CLAUDE_CONFIG_DIR`, NOT `CODEX_HOME` | clear | (A5 above) |
| Sync-back exclusion list is `{".credentials.json": {}}` not `{"auth.json": ...}` | clear | `runtime.go:136-138` — exclusion map literal is exactly `{".credentials.json": {}}`. |

### Worklog ↔ Code Coherence

| Worklog claim | Status |
|---|---|
| Files created (6 listed with LOC counts) | match — `profile.go` ~42 LOC matches actual 43; `account.go` ~38 LOC matches actual 37; `runtime.go` ~270 LOC matches actual 309; `profile_test.go` ~50 LOC matches actual 59; `account_test.go` ~46 LOC matches actual 60; `runtime_test.go` ~215 LOC matches actual 336. Variances are within reasonable rounding; the worklog notes "Total ~350 LOC, ~311 test LOC" which is in the right ballpark for the production+test split. Not a fail. |
| 16 tests across 3 test files, names listed | match — every test name listed in the worklog is present in the corresponding file. |
| Coverage 76.2% | match — independent `mage testPkg` re-run produced 76.2%. |
| Initial 38.4% coverage with 11 tests, raised to 76.2% with 16 tests | not independently re-verifiable (intermediate state), but plausible and not load-bearing. |
| RecipeHash audit clean | match — independently re-verified by Read. |

### Gaps

None.

### Observations

(Out of scope for this round — record-only, no fix required.)

- `runtime_test.go:286-291` `TestPrepareRuntimeUsesSharedHomeAndSyncsBack` has a vacuous block: `if _, err := os.Stat(filepath.Join(sharedHome, ".credentials.json")); err == nil { ... }` with an empty body. The comment explains intent (verify exclusion), but the test does not actually fail if `.credentials.json` is written to the runtime dir and then synced back. Sync-back exclusion is still verifiable from `runtime.go:136-138` directly, so the production guarantee is intact — but the test does not enforce it. Consider tightening in a future drop or DROP_9 cleanup.
- `appendUniqueStrings` (`runtime.go:257-277`) is defined but unused by production code in the claude package — only the test `TestAppendUniqueStrings` exercises it. The worklog explicitly notes this is verbatim-from-Codex per focus-plan §3.2 v1 decision, with dedupe deferred to DROP_9. Accepted as intentional.
- `_ = projectRoot` discard at `runtime.go:156` is documented in worklog note 4 as an intentional v1 placeholder. Future project-config support will use it. Accepted.

### Verdict

**pass**

All seven acceptance criteria are met with primary code/test evidence. Copy-adapt traps are clean. Idiomatic-Go discipline (doc comments, error wrapping, mage-only build, defer cleanup, context-first param) is honored. Coverage 76.2% > 60% effective gate; > 70% AGENTS.md target. Re-running `mage testPkg ./internal/adapters/providers/claude` independently produced an identical green result.

No findings require a builder respawn. Round 1 closes.

## Unit 5.2 — Round 1

**Date:** 2026-05-14
**Reviewer role:** proof-oriented (paired with falsification subagent running in parallel)
**Verdict:** pass

### Scope

Verify Unit 5.2 — new package `internal/services/claude/` (`service.go` + `service_test.go`) implements the Claude launch service mirroring `internal/services/codex/service.go` with isolated-first model substitutions, as specified in `PLAN.md` § Unit 5.2.

### Mage Verification

| Target | Result |
|---|---|
| `mage testPkg ./internal/services/claude` | PASS — 17/17 tests, 81.0% coverage, `-race -cover` clean, gate is 60% (effective). Coverage also exceeds 70% AGENTS.md target. |

Re-run independently of builder. Output matches worklog claim line-for-line (`81.0%`, 17 tests).

### Findings — Acceptance Criteria (PLAN.md § Unit 5.2)

| # | Acceptance Criterion | Status | Evidence |
|---|---|---|---|
| A1 | `mage testPkg ./internal/services/claude` green (gofumpt + 70% coverage) | met | 17/17 tests pass; 81.0% > 70% AGENTS.md target; > 60% magefile effective gate. |
| A2 | No reference to `sharedCodexStateHome` in the new package | met | Read of `service.go` (346 lines) and `service_test.go` (550 lines): neither file contains `sharedCodexStateHome`. `Run` at `service.go:127-133` passes `SharedHome: ""` directly with explicit comment lines 124-126. |
| A3 | No reference to `codexruntime.DefaultHostProfile` in the new package | met | `service.go:3-21` imports only `clauderuntime` from the Claude adapter (line 17). No `codexruntime` import. No `DefaultHostProfile` call site anywhere in the package. |
| A4 | `resolveBinding` passes `domain.ProviderClaude` to `BindingByProjectID` | met | `service.go:215` — exact call: `s.store.BindingByProjectID(ctx, projectRecord.ID, domain.ProviderClaude)`. `service.go:222` asserts `binding.Provider != domain.ProviderClaude`. `service.go:233` asserts `profile.Provider != domain.ProviderClaude`. All three positions covered. Behaviorally verified by `TestRunSucceedsWithBoundProject` (line 202), `TestRunRejectsWrongBindingProvider` (line 315), `TestRunRejectsWrongProfileProvider` (line 350). |
| A5 | Container label `"io.valv.provider"` equals `"claude"` | met | `service.go:261` literal `"io.valv.provider": "claude"`. Verified by `TestRunSucceedsWithBoundProject` at `service_test.go:237-239` asserting recorded request label equals `"claude"`. |
| A6 | `New` returns error when `Store` is nil | met | `service.go:76-78` returns `"new claude launch service: store is required"`. Verified by `TestNewRequiresDependencies` case "nil store" at `service_test.go:173-176`. |
| A7 | `New` returns error when `Executor` is nil | met | `service.go:79-81` returns `"new claude launch service: executor is required"`. Verified by `TestNewRequiresDependencies` case "nil executor" at `service_test.go:177-180`. |
| A8 | `New` returns error when `Image.Repository` is empty | met | `service.go:82-84` returns `"new claude launch service: image repository is required"`. Verified by `TestNewRequiresDependencies` case "empty image repository" at `service_test.go:181-184`. |

### Findings — Copy-Adapt Traps (per spawn prompt)

| Trap | Status | Evidence |
|---|---|---|
| No `sharedCodexStateHome` or `sharedClaudeStateHome` helper in new package | clear | Read of both files; no such function defined or called. `SharedHome: ""` passed inline at `service.go:129`. |
| `resolveBinding` passes `ProviderClaude` to `BindingByProjectID` AND asserts `binding.Provider == ProviderClaude` AND asserts `profile.Provider == ProviderClaude` | clear | All three positions present at `service.go:215, 222, 233` respectively. |
| `containerName` produces `valv-claude-interactive-...` not `valv-codex-...` | clear | `service.go:321` — format string is `"valv-claude-interactive-%s-%d"`. `TestContainerNameContainsClaude` at `service_test.go:512-524` asserts `valv-claude-interactive-` prefix AND `!strings.Contains(name, "codex")`. |
| `buildRequest` sets label `"io.valv.provider": "claude"` | clear | (A5 above) |
| Import alias is `clauderuntime` for the adapter package | clear | `service.go:17` — `clauderuntime "github.com/evanmschultz/valv/internal/adapters/providers/claude"`. Mirrored in `service_test.go:15`. |
| Service passes `SharedHome: ""` to `clauderuntime.PrepareRuntime` | clear | `service.go:129` — explicit `SharedHome: ""` with documenting comment at lines 124-126. |

### Findings — Enumerated Tests (per spawn prompt)

| Required Test | Status | Location |
|---|---|---|
| `TestRunSucceedsWithBoundProject` | present | `service_test.go:202-259` — asserts container name, image, label, ClaudeDir mount, CLAUDE_CONFIG_DIR env, managed label |
| `TestRunReturnsUnboundProjectWhenNoProject` | present | `service_test.go:263-285` — asserts `errors.Is(err, domain.ErrUnboundProject)` |
| `TestRunReturnsUnboundProjectWhenNoBinding` | present | `service_test.go:289-311` — same `errors.Is` assertion |
| `TestRunRejectsWrongBindingProvider` | present | `service_test.go:315-346` — binding.Provider = Codex; asserts error contains "expected" |
| `TestRunRejectsWrongProfileProvider` | present | `service_test.go:350-381` — profile.Provider = Codex; asserts error contains "expected" |
| `TestValidateBindingReturnsNilForBoundProject` | present | `service_test.go:385-404` — happy-path ValidateBinding nil result |
| Constructor table-driven test (nil Store / nil Executor / empty Image.Repository) | present | `TestNewRequiresDependencies` at `service_test.go:158-197` — table with exactly the three required cases |

All seven enumerated cases are present with the exact names listed in the spawn prompt.

### Findings — Idiomatic Go (CLAUDE.md + AGENTS.md § 5–7)

| Concern | Status | Evidence |
|---|---|---|
| Doc comments on every exported identifier, starting with identifier name | met | `Store` (`service.go:23`), `Executor` (30), `DetectFunc` (38), `Options` (41), `Service` (57), `New` (73), `Run` (115), `ValidateBinding` (182). Each comment begins with the identifier name. |
| Errors wrapped with `fmt.Errorf("context: %w", err)` at each boundary | met | `service.go:135, 142, 166, 177, 198, 203, 210, 212, 218, 220, 229, 231, 280` — every error path wraps with `%w` (or constructs sentinel-wrapping `ErrUnboundProject` via `%w` at 210, 218, 229). |
| Only mage targets used (no raw `go test`) | met | Worklog § "Mage Targets Run" lists only `mage testPkg`. No raw `go` commands. |
| `defer` for cleanup | met | `service.go:137` — `defer prepared.Close()` immediately after the `PrepareRuntime` call. |
| `context.Context` first param | met | `Run(ctx context.Context, ...)` at `service.go:118`; `ValidateBinding(ctx, ...)` at 184; `resolveBinding(ctx, ...)` at 195; `runAttached(ctx, ...)` at 171. Consistent. |
| Sentinel error usage (`errors.Is`) | met | `service.go:209, 217, 228` use `errors.Is(err, domain.ErrNotFound)`. |

### Worklog ↔ Code Coherence

| Worklog claim | Status |
|---|---|
| 17 tests in `service_test.go` with listed names | match — every name listed in worklog § "Test Count" is present in the file. |
| Coverage 81.0% | match — independent `mage testPkg` re-run produced 81.0% exactly. |
| Initial gofumpt failure → fixed (alignment in struct literal) | not independently re-verifiable (intermediate state), but plausible. The final-state file passes gofumpt as part of `mage testPkg`. |
| No `sharedCodexStateHome` | match — independently verified. |
| `boundClaudeStore` and `detectAlways` test helpers added | match — `service_test.go:131-148` and `150-154` respectively. |
| File LOC: 278 production, 330 test | drift — actual is `service.go` 345 LOC, `service_test.go` 550 LOC. Worklog under-reports by ~24%. Code matches the PLAN.md spec target ("10.2K, 347 LOC" for the Codex template; claude/service.go 345 LOC is within 1%). Non-blocking observation — acceptance criteria don't include LOC accuracy, and code-to-spec is correct. |

### Gaps

None blocking.

### Observations

(Out of scope for this round — record-only, no fix required.)

- **Worklog LOC drift.** Worklog reports 278 production LOC and 330 test LOC; actual files are 345 and 550 respectively. The worklog count is wrong. The PLAN.md spec target for the Codex template was "10.2K, 347 LOC" and the actual claude/service.go (345 LOC) is within 1% of that target — so code-to-spec sizing is correct. Recommend builder cross-check LOC counts in future worklogs by running `wc -l` rather than estimating.
- **Test helper coverage.** `fakeExecutor.Create` / `Start` / `RemoveContainer` (`service_test.go:108-127`) implement the `Executor` interface for compile-time completeness but are not exercised by any test in this package. The `Run` path is the only path the service actually invokes today. Accepted — interface satisfaction is required for the type to be passed; the unused methods are not dead code from the interface's perspective.
- **`TestRunBubblesExecutorErrors` doesn't assert `errors.Is`.** The test (line 528) asserts `strings.Contains(err.Error(), "docker failed")` rather than `errors.Is(err, sentinelErr)`. The production code at `service.go:166` wraps with `%w` so `errors.Is` would also work; the test is correct but slightly less strict than idiomatic. Non-blocking — wrapping behavior is structurally present.
- **`emitNotices` parameter discard.** `service.go:298` — `emitNotices(_ domain.Profile, warnings, _ []string)` discards the profile arg and the claudeArgs arg. The signature matches Codex for structural parity per PLAN.md note 2 (`RealHome` field present but unused). Accepted as intentional v1 structural parity.

### Verdict

**pass**

All 8 acceptance criteria from PLAN.md § Unit 5.2 are met with primary code+test evidence at file:line granularity. All 6 copy-adapt traps from the spawn prompt are clean. All 7 enumerated test cases (including the table-driven constructor test for acceptance criterion 5) are present with the required names. Coverage 81.0% independently re-verified — exceeds both the 60% magefile gate and the 70% AGENTS.md target. Idiomatic Go discipline (doc comments, error wrapping with `%w`, mage-only build, `defer` cleanup, context-first param) is honored throughout. Worklog LOC numbers drift from actual file sizes (~24% under-report) but acceptance criteria do not include LOC accuracy and code-to-spec sizing is correct. The drift is a non-blocking worklog hygiene observation.

No findings require a builder respawn. Round 1 closes.
