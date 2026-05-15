# DROP_5_CLAUDE_LAUNCHER — Build QA Falsification

Append a `## Unit N.M — Round K` section per QA attempt. Falsification-oriented review: actively try to break the unit's claim via counterexamples.

## Unit 5.1 — Round 1

**Date:** 2026-05-14
**Verdict:** pass

### Summary

No unmitigated counterexample found across 16 attack vectors (12 from the spawn prompt + 4 self-derived). The Claude adapter package is structurally clean against the Codex template, substitutions are exhaustive and correct, env keys/mount target/sync exclusion match the spec, no `codex` identifiers leak into production or test code, and acceptance criteria are demonstrably satisfied. Two soft observations recorded under Soft Findings (mild test weakness, no code defect).

### Attack Attempts

Each attempt is REFUTED (no counterexample found) or noted under Soft Findings (weak but not breaking).

| # | Attack | Result |
|---|---|---|
| 1 | Copy-paste residue — `codex` / `CODEX` / `auth.json` / `config.toml` strings in production code | REFUTED |
| 2 | Wrong substitution direction in `DefaultHostProfile` (returns `~/.claude` instead of isolated path) | REFUTED |
| 3 | Hidden import of `internal/adapters/providers/codex` from claude package | REFUTED |
| 4 | `ReadAccountIdentity` overreach (parses content beyond stat) | REFUTED |
| 5 | `PrepareRuntime` env leakage (`CODEX_HOME` set or invented `CLAUDE_HOME` key) | REFUTED |
| 6 | Sync-back exclusion wrong (`.credentials.json` not excluded) | REFUTED |
| 7 | `projectRoot` silently ignored without `_ =` guard | REFUTED |
| 8 | Coverage gaming (5 new tests being coverage theater rather than behavior assertions) | REFUTED with soft finding on `TestErrorsJoin` |
| 9 | `SharedHome: ""` semantics broken (crash on empty string) | REFUTED |
| 10 | Error wrapping non-compliance (missing `%w` or string concat at error boundaries) | REFUTED |
| 11 | `IsDefaultHostHome` edge cases (empty paths, `..`, relative) | REFUTED at code level; thin test coverage noted |
| 12 | Mage discipline violation (raw `go test` in worklog) | REFUTED |
| 13 | Dead `profileHome != sharedHome` branch (production never sets distinct shared home) | REFUTED — documented design choice in PLAN.md Design Note 6 |
| 14 | `TestPrepareRuntimeUsesSharedHomeAndSyncsBack` exclusion assertion has no failure path | Soft Finding — weak test, code is correct |
| 15 | `ctx context.Context` unused (no cancellation threading) | REFUTED — matches Codex template, not a v1 spec requirement |
| 16 | `pathutil.Normalize("")` behavior on empty `ProjectRoot` | REFUTED — value is `_ =`'d, service spec mandates non-empty input |

### Evidence Trace

Files read in full and cross-checked against the Codex template:

| Claude file | Codex counterpart | Comparison result |
|---|---|---|
| `internal/adapters/providers/claude/profile.go` | `internal/adapters/providers/codex/profile.go` | Substitutions correct: error message says "claude" not "codex"; path is `.valv/providers/claude/profiles/default` not `.codex` |
| `internal/adapters/providers/claude/account.go` | `internal/adapters/providers/codex/account.go` | Correctly stripped: no `auth.json`, no JWT decode, no `json.Unmarshal`, no `encoding/base64`. Only `os.Stat` on `.credentials.json` |
| `internal/adapters/providers/claude/runtime.go` | `internal/adapters/providers/codex/runtime.go` | Correctly stripped: no `BurntSushi/toml` import, no `newBridgeManager`, no `translateConfigFile`, no `translateMCPServers`, no `cloneMap`/`stringValue`/`stringSlice`/`stringMap`/`passthroughEnvFromEntry`. Env uses `CLAUDE_CONFIG_DIR`; mount target uses `ContainerClaudeDir`; temp prefix is `"claude-runtime-"`; subdir is `"claude-home"`; sync exclusion is `{".credentials.json": {}}` |
| `profile_test.go` | n/a — fresh Claude tests | 4 tests, all behavior-oriented, no codex references |
| `account_test.go` | n/a — fresh Claude tests | 3 tests, all presence-only assertions |
| `runtime_test.go` | n/a — fresh Claude tests | 9 tests including `TestPrepareRuntimeHasNoCodexEnv` guard test |

Key code sites verified:

- `profile.go:21,25` — error messages: `"resolve claude host profile: ..."` (correct provider name)
- `profile.go:23` — path: `filepath.Join(trimmedHome, ".valv", "providers", "claude", "profiles", "default")` (isolated-first, matches dev decision #1)
- `account.go:23-36` — only `os.Stat`; no content-read primitive; returns `LoggedIn: true` iff stat ok + not dir
- `runtime.go:18-23` — constants: `ContainerHomeDir = "/home/valv"`, `ContainerClaudeDir = "/home/valv/.claude"`
- `runtime.go:75-84` — `SharedHome` defaults to `profileHome` when empty (no crash path)
- `runtime.go:95` — `os.MkdirTemp(tempRoot, "claude-runtime-")` (correct prefix)
- `runtime.go:102` — runtime subdir `"claude-home"` (Codex uses `"codex-home"`)
- `runtime.go:115-117` — mount: `dockeradapter.NewMountSpec(runtimeClaudeHome, ContainerClaudeDir, false)`
- `runtime.go:118-124` — env map: `CLAUDE_CONFIG_DIR`, `HOME`, `LOGNAME`, `TERM`, `USER` — no `CODEX_HOME`, no invented `CLAUDE_HOME`
- `runtime.go:136-138` — sync exclusion `{".credentials.json": {}}` (matches spec)
- `runtime.go:156` — `_ = projectRoot // used for future project-config support; not translated in v1` (explicit unused guard with comment)

### Soft Findings (not unit-breaking)

These do NOT counterexample the unit's pass verdict, but are recorded for future hardening or dev awareness:

1. **`TestErrorsJoin` is thin coverage.** `runtime_test.go:304-313` only asserts `errorsJoin() == nil` and `errorsJoin(nil) == nil`. The function is a 1-line wrapper around `errors.Join` from stdlib, so the bug surface is essentially zero — but the test does not exercise the multi-error join behavior at all. It exists primarily to cover the function line for the coverage gate. Not breaking; mild coverage theater.
2. **`TestPrepareRuntimeUsesSharedHomeAndSyncsBack` `.credentials.json` exclusion check is dead.** `runtime_test.go:287-291`:
   ```go
   if _, err := os.Stat(filepath.Join(sharedHome, ".credentials.json")); err == nil {
       // The original exists; check it wasn't overwritten via sync-back of a different copy.
       // (Sync-back excludes .credentials.json, so the shared one stays, but no new one appears.)
   }
   ```
   There is no `t.Fatalf` or assertion inside the `if` block — only a comment. The test does not actually fail if `.credentials.json` gets overwritten or removed during sync-back. The functional behavior is correct (the code excludes `.credentials.json` from sync), but the test's exclusion-guarantee assertion is non-functional. Strengthen: write a `.credentials.json` to the staged mount source with different content, call `Close`, assert the file in `sharedHome` either remains the original content OR does not exist (depending on intended semantics).
3. **`IsDefaultHostHome` edge-case test coverage thin.** Only 2 cases tested (positive + negative on a sibling path). Adding cases for empty `profileHome`, empty `homeDir`, and `..`-containing paths would harden against future refactors of `pathutil.Normalize`. Not breaking — current code delegates correctly to Normalize and returns `false` on err.
4. **Helper functions in `runtime.go` `return err` without context.** `copyDirContents`, `copyFile`, the walk closure return bare `err` in several places (lines 188, 190, 195, 211, 224, 247, 250). This matches the Codex template verbatim and is internal to the package, so wrapping at every internal boundary would be noise. The outer-boundary wrapping in `PrepareRuntime` itself is correct per AGENTS.md § 6. Acceptable for v1; flag for DROP_9 cleanup if helpers are deduped.

### Acceptance Criteria Verification

All 8 acceptance criteria from PLAN.md § Unit 5.1 verified:

| # | Criterion | Verified by |
|---|---|---|
| 1 | `mage testPkg ./internal/adapters/providers/claude` green | Worklog: 16/16 tests pass, 76.2% coverage > 60% gate |
| 2 | No imports of `internal/adapters/providers/codex` | Import block inspection of all three production files |
| 3 | `DefaultHostProfile(homeDir)` returns `.valv/providers/claude/profiles/default` | `profile.go:23` direct read + `TestDefaultHostProfileReturnsIsolatedPath` assertion |
| 4 | `ReadAccountIdentity` returns `LoggedIn: true` for present `.credentials.json`, false for empty dir | `account.go:23-36` direct read + 3 account tests |
| 5 | `PrepareRuntime` env has `CLAUDE_CONFIG_DIR=/home/valv/.claude` | `runtime.go:118-124` direct read + `TestPrepareRuntimeSetsClaudeConfigDirEnv` |
| 6 | `PrepareRuntime` env has NO `CODEX_HOME` key | `runtime.go:118-124` direct read (no such key) + `TestPrepareRuntimeHasNoCodexEnv` guard test |
| 7 | No `bridge.go` in package directory | `ls` of `internal/adapters/providers/claude/` shows only 6 files (3 production + 3 test) |
| 8 | RecipeHash audit: zero non-test codex identifiers | Manual inspection of all 6 files; sole codex reference is the test name `TestPrepareRuntimeHasNoCodexEnv` which is a deliberate guard test |

### Counterexamples

None.

### Verdict

**pass** — no unmitigated counterexample to the unit's claim that the Claude adapter package mirrors the Codex template with correct Claude-specific substitutions, satisfies all 8 acceptance criteria, and adds no codex contamination in production or test code.

### Hylla Feedback

N/A for this unit. The QA review was conducted via direct `Read` of the 6 created files and the 3 Codex template files. Hylla was not queried because (a) the files under review are post-ingest (Hylla index would be stale), and (b) the Codex template files are short enough that direct Read is more efficient than Hylla summarization for a structural diff review. Bash grep was permission-denied during the review, but file sizes (42–270 LOC) made manual content scanning a complete substitute.

## Unit 5.2 — Round 1

**Date:** 2026-05-14
**Verdict:** pass

### Summary

No unmitigated counterexample found across 15 attack vectors from the spawn prompt plus 3 self-derived attacks. The Claude launch service is a structurally correct port of `internal/services/codex/service.go` with every Claude-specific substitution applied: provider guards flipped to `domain.ProviderClaude`, container name format flipped to `valv-claude-interactive-`, label flipped to `"io.valv.provider": "claude"`, `sharedCodexStateHome` deliberately not ported, `SharedHome: ""` correctly handled by the adapter collapse path. Two soft findings recorded against test rigor — neither breaks the unit.

### Attack Attempts

| # | Attack | Result |
|---|---|---|
| 1 | Copy-paste residue (`codex`/`Codex`/`CODEX`/`valv-codex`) in production code | REFUTED — `service.go` reads end-to-end show zero `codex`/`Codex`/`CODEX` substrings in production source |
| 2 | `sharedCodexStateHome` helper ported into Claude service | REFUTED — not present at any line in `service.go`; `Run` directly passes `SharedHome: ""` at line 129 |
| 3 | Provider guard direction reversed (rejects Claude bindings) | REFUTED — `service.go:222` reads `binding.Provider != domain.ProviderClaude`; `service.go:233` reads `profile.Provider != domain.ProviderClaude` — correct |
| 4 | `BindingByProjectID` call passes `domain.ProviderCodex` (copy-paste leftover) | REFUTED — `service.go:215` reads `s.store.BindingByProjectID(ctx, projectRecord.ID, domain.ProviderClaude)` directly; production call is correct |
| 5 | `containerName` produces `valv-codex-interactive-` (silent collision risk) | REFUTED — `service.go:321` reads `fmt.Sprintf("valv-claude-interactive-%s-%d", base, s.now().UnixNano())`; format is `valv-claude-interactive-…` |
| 6 | `buildRequest` sets `"io.valv.provider": "codex"` | REFUTED — `service.go:261` reads `"io.valv.provider": "claude"` as a literal in the labels map |
| 7 | `SharedHome` accidentally set to `realHome` or `profile.HomePath` instead of `""` | REFUTED — `service.go:129` reads `SharedHome: ""` literal in the `PrepareRequest` struct literal; adapter `runtime.go:75-81` confirms empty `SharedHome` collapses to `profileHome` and the temp-copy branch is skipped (line 101 `if profileHome != sharedHome` is false in this case) |
| 8 | `New` does not reject nil Store / nil Executor / empty Image.Repository | REFUTED — `service.go:76-84` reads three explicit `nil`/`""` checks with distinct error messages; `TestNewRequiresDependencies` exercises all three table cases |
| 9 | Error wrapping non-compliance at outer entry points (`Run`, `resolveBinding`, `runAttached`) | REFUTED — every error return in `Run`, `runAttached`, and `resolveBinding` is `fmt.Errorf("run claude launch service: …: %w", err)`; constructor uses validation-style messages without `%w` because there is no upstream `err` |
| 10 | `Unbound project` tests verify error wraps `domain.ErrUnboundProject` via `errors.Is` | REFUTED — `service_test.go:282` and `:308` both use `errors.Is(err, domain.ErrUnboundProject)`; tests pass only when wrapping is correct |
| 11 | Coverage gaming (81% reached via assertion-free tests) | REFUTED — every test in `service_test.go` has at least one `t.Fatalf` triggered on a specific value or error; `TestEmitNoticesSuppressesWarningsOnTTY` and `TestEmitNoticesWritesWarningsWithoutTTY` are pair-asserted on opposite behaviors, not padding; soft finding on weak provider-argument assertion in `TestRunSucceedsWithBoundProject` recorded below |
| 12 | `Now` injection missing (flaky container-name test) | REFUTED — `TestRunSucceedsWithBoundProject:217` injects `Now: func() time.Time { return time.Unix(0, 123456789) }`; `TestContainerNameContainsClaude:515` uses `time.Unix(0, 42)`; container-name behavior is deterministic in tests |
| 13 | `RealHome` field claimed unused but actually referenced in service body | REFUTED — `realHome` appears only at `service.go:52` (Options field), `:68` (Service field), and `:109` (copy in `New`); zero reads inside `Run`/`resolveBinding`/`buildRequest`/`runAttached`/`containerName`/`emitNotices`/`debug`; worklog claim verified |
| 14 | Mage discipline violation (raw `go test` in worklog) | REFUTED — worklog § "Mage Targets Run" lists only `mage testPkg ./internal/services/claude`; no raw `go` invocations referenced |
| 15 | `Detect` field unused or not invoked | REFUTED — `Detect` wired in `New` at `service.go:86-89` (defaults to `projectdetect.DetectFrom` when nil), invoked at `service.go:201` (`s.detect(workingDir)`); used |
| 16 (self) | `runAttached` swallows error info (bare wrap without project context) | REFUTED — `service.go:177` wraps with `project %q: %w` for project root context |
| 17 (self) | `errors.Is(err, domain.ErrUnboundProject)` chain broken because `resolveBinding` wraps `ErrUnboundProject` inside a `fmt.Errorf` that loses the sentinel | REFUTED — `service.go:210, 218, 229` each use `: %w` wrapping `domain.ErrUnboundProject` directly; `errors.Is` unwraps cleanly; tests at `service_test.go:282/:308` are exactly this chain and pass |
| 18 (self) | `TestRunSucceedsWithBoundProject` does not actually exercise the TTY+Stdin attach path; the `request.Interactive && request.TTY` branch (service.go:159) might never run in tests | REFUTED — test sets `TTY: true, Stdin: true` at lines 215-216, which makes `request.Interactive = s.stdin = true` and `request.TTY = s.tty = true`; the attach branch fires; executor's `Run` is called via `runAttached`; assertions on `executor.got` succeed because `runAttached` calls `s.executor.Run` |

### Evidence Trace

Files read in full and cross-checked:

| Claude file | Codex counterpart | Comparison result |
|---|---|---|
| `internal/services/claude/service.go` (346 LOC) | `internal/services/codex/service.go` (347 LOC) | Net delta: removed `sharedCodexStateHome` (13 LOC), removed `shared_home` debug key, removed `sharedHome` argument to `PrepareRequest`. Substitutions: `codex → claude` in package, imports, error messages, label, container-name format, debug strings. Notice prefix changed from `"Valv MCP note: %s\n"` to `"Valv note: %s\n"` (deliberate — Claude v1 has no MCP bridging). All other 320 LOC structurally parallel |
| `internal/services/claude/service_test.go` (550 LOC) | `internal/services/codex/service_test.go` (16.7K reference) | 14 test functions; uses `boundClaudeStore` and `detectAlways` helpers; covers happy path, unbound-project (no project), unbound-project (no binding), wrong-binding-provider, wrong-profile-provider, validate-binding, outside-project-root, sibling-prefix path, non-interactive TTY, emit-notices on-TTY suppression, emit-notices off-TTY write, container-name no-codex assertion, executor error bubbling |

Key code sites verified by direct read:

- `service.go:17` — import is `clauderuntime "…/providers/claude"` (correct alias, correct package)
- `service.go:75-84` — `New` validation: nil Store → error, nil Executor → error, empty Image.Repository → error
- `service.go:127-133` — `PrepareRequest` struct literal uses `ProfileHome: resolved.profile.HomePath`, `SharedHome: ""` (literal empty string), `ProjectRoot: resolved.project.Root`
- `service.go:135, 142, 166, 177` — outer error returns all wrap with `fmt.Errorf("run claude launch service: …: %w", err)`
- `service.go:145` — debug message: `"launching claude container"` (correct)
- `service.go:174` — debug message: `"starting interactive claude container"` (correct)
- `service.go:201` — `s.detect(workingDir)` invocation (Detect is used)
- `service.go:210, 218, 229` — three `%w` wrappings of `domain.ErrUnboundProject` (preserves `errors.Is` chain)
- `service.go:215` — `s.store.BindingByProjectID(ctx, projectRecord.ID, domain.ProviderClaude)` (correct provider constant)
- `service.go:222, 233` — provider mismatch checks use `!= domain.ProviderClaude` (correct direction)
- `service.go:261` — label map has `"io.valv.provider": "claude"` (correct)
- `service.go:303` — debug message: `"claude runtime warning"` (correct)
- `service.go:309` — notice prefix: `"Valv note: %s\n"` (intentional adaptation from Codex's `"Valv MCP note: …"`)
- `service.go:321` — container name format: `"valv-claude-interactive-%s-%d"` (correct)
- Adapter `runtime.go:75-81` — empty `SharedHome` falls through to `sharedHome := profileHome`; `runtime.go:101` `if profileHome != sharedHome` is false; temp-copy branch skipped as designed

### Soft Findings (not unit-breaking)

1. **`TestRunSucceedsWithBoundProject` does not directly assert the provider argument passed to `BindingByProjectID`.** Acceptance criterion phrasing implies "verified by `TestRunSucceedsWithBoundProject` observing the fakeStore call." In reality, `fakeStore.BindingByProjectID` (`service_test.go:70-72`) is a value-receiver method that ignores its `provider` argument — it returns `f.binding, f.bindingErr` unconditionally. The actual safety net is at `service.go:215` (production hard-codes `domain.ProviderClaude`) plus `TestRunRejectsWrongBindingProvider` (which exercises the `!=` guard). Production behavior is correct; the test's assertion power is weaker than the acceptance criterion implies. Strengthen: change `fakeStore.BindingByProjectID` to a pointer receiver and record the `provider` argument; assert `gotProvider == domain.ProviderClaude` in `TestRunSucceedsWithBoundProject`. Not unit-breaking — the production call is correctly written by direct read of `service.go:215`.
2. **`TestRunBuildsNonInteractiveDockerRequestWhenTTYDisabled` is mis-named in assertion content.** The test sets neither `TTY` nor `Stdin` (defaults to false), so `request.Interactive == false` and `request.TTY == false`. Both `if executor.got.Interactive || executor.got.TTY` checks are the same field-truth check. Doesn't break — but the test would still pass if the production code accidentally set `Interactive = !s.stdin` (it would then be true when stdin is false, failing the test — actually that would catch the inversion, so the test is fine). Withdrawn — on re-read this attack is moot.
3. **Soft finding on `fakeStore` value-receiver capture.** `fakeStore.ProjectByRoot`, `ProfileByID`, `BindingByProjectID` are value receivers (`service_test.go:34, 46, 70`). Any assignments to receiver fields (like the Codex test's `f.projectRoot = root`) are lost. Claude's `fakeStore` doesn't try to capture call-arg fields (unlike Codex's), so this isn't a defect — but if a future builder adds capture-on-call assertions following the Codex pattern, the captures will silently no-op. Worth a code comment, not a fix.

### Acceptance Criteria Verification

All 8 acceptance criteria from PLAN.md § Unit 5.2 verified:

| # | Criterion | Verified by |
|---|---|---|
| 1 | `mage testPkg ./internal/services/claude` green | Worklog § "Mage Targets Run": 17/17 tests pass, 81.0% coverage > 60% gate |
| 2 | No reference to `sharedCodexStateHome` | Direct read of `service.go` end-to-end — no such identifier exists |
| 3 | No reference to `codexruntime.DefaultHostProfile` | Direct read of `service.go` imports + body — no such call exists |
| 4 | `resolveBinding` passes `domain.ProviderClaude` to `BindingByProjectID` | `service.go:215` direct read; production call hard-codes correct constant |
| 5 | Container label `"io.valv.provider"` equals `"claude"` | `service.go:261` direct read; `TestRunSucceedsWithBoundProject:237` asserts equality |
| 6 | `New` returns error when Store nil | `service.go:76-78` + `TestNewRequiresDependencies` table case "nil store" |
| 7 | `New` returns error when Executor nil | `service.go:79-81` + `TestNewRequiresDependencies` table case "nil executor" |
| 8 | `New` returns error when Image.Repository empty | `service.go:82-84` + `TestNewRequiresDependencies` table case "empty image repository" |

### Counterexamples

None.

### Verdict

**pass** — no unmitigated counterexample to the unit's claim that the Claude launch service mirrors the Codex template with the correct dev-confirmed isolated-first deltas (no `sharedCodexStateHome`, `SharedHome: ""` in `PrepareRequest`, `domain.ProviderClaude` guards, `valv-claude-interactive-` container name format, `"io.valv.provider": "claude"` label). All 8 acceptance criteria verified by direct code read plus test assertions. Soft findings recorded against test rigor (provider-argument capture, value-receiver fakeStore) do not break the unit and are appropriate to address in DROP_9 dedupe or as drive-by improvements in a future drop.

### Hylla Feedback

N/A for this unit. The QA review was conducted via direct `Read` of the two Claude service files (`service.go` 346 LOC, `service_test.go` 550 LOC), the Codex template file (`codex/service.go` 347 LOC), the adapter `runtime.go` for the `SharedHome` collapse path, and a partial read of `codex/service_test.go` for fakeStore shape comparison. Bash grep was permission-denied; manual content scan of the cat-n output was a complete substitute given the file sizes. Hylla was not queried because (a) the files under review are post-ingest (Hylla index is stale relative to HEAD), and (b) the Codex template is short enough that direct Read is more efficient than Hylla summarization for structural-diff review of this class.
