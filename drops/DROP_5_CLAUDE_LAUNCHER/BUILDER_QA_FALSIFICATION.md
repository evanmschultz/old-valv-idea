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

## Unit 5.3 — Round 1

**Date:** 2026-05-14
**Verdict:** pass

### Summary

No unmitigated counterexample found across 15 spawn-prompt attacks plus 8 self-derived attacks. The `valv claude` pass-through CLI, root-command registration, and manage stub flip are structurally correct and faithful to the dev-confirmed isolated-first model. `EnsureLatest` → `Build` substitution is correctly applied. No first-run gating, no shared-home helper in `claude.go`, no Codex-provider leakage. The Unit 5.3 commit (`87f8cdd`) touches exactly the planned files; DROP_4 files (`claude_image.go`, `account_auth.go`, `operator_helpers.go`) remain untouched. One soft finding on `TestRunClaudeCommandRejectsWrongProvider` test-name semantics (test exercises "no Claude binding" path, not the direct provider-mismatch guard at `services/claude/service.go:222`). Test passes for the correct behavioral reason; the direct-mismatch path is exercised at the service-test layer (`services/claude/service_test.go::TestRunRejectsWrongBindingProvider`). Independent re-execution of `mage testPkg`, `mage build`, and `./valv --help` was blocked by tool permission in this session, so the worklog's green status is accepted on code-evidence grounds (no contradictions found between the worklog and the file contents).

### Attack Attempts

| # | Attack | Result |
|---|---|---|
| 1 | `EnsureLatest` vs `Build` mistake — copy-paste leftover that would fail at runtime because Claude has no Resolver | REFUTED — `claude.go:192` reads `service.Build(cmd.Context(), imagesservice.BuildRequest{Version: imagesservice.DefaultClaudeCLIVersion})` |
| 2 | First-run setup leakage (`ensureClaudeBindingReady`, `ensureBoundClaudeAccountReady`, `errClaudeSetupCanceled`, `runClaudeFirstRunSetup`) | REFUTED — `claude.go:53-121` contains none of these; comment at `:67-69` documents the deliberate omission |
| 3 | `SharedHome` literal at service construction (any non-empty `SharedHome` would trigger the adapter's shared-home branch) | REFUTED — `claude.go:93-104` constructs `claudeservice.Options` which has no `SharedHome` field (`services/claude/service.go:42-55`); `SharedHome: ""` is set inside the service at `service.go:129` (literal empty string in struct literal) |
| 4 | `ProviderCodex` passed somewhere in `claude.go` | REFUTED — only `domain.ProviderClaude` appears (`claude.go:185`); zero `ProviderCodex` references |
| 5 | `runClaudeImageOnlyCommand` env keys wrong (`CODEX_HOME` instead of `CLAUDE_CONFIG_DIR`) or mount target wrong | REFUTED — `claude.go:144-149` reads `CLAUDE_CONFIG_DIR: claudeprovider.ContainerClaudeDir`, `HOME: claudeprovider.ContainerHomeDir`; no `CODEX_HOME` key. The image-only path is mountless by design (Codex equivalent at `codex.go:143-165` also has no profile mount) |
| 6 | Container image fallback uses `codexImageRef()` | REFUTED — `claude.go:96, 133, 183, 192` all call `claudeImageRef()` |
| 7 | Root.go edit correctness (declared but not added, missing GroupID, missing Example) | REFUTED — `root.go:127` declares `claudeCmd := newClaudeCommand(paths, nil)`, `:128` sets `GroupID = "runtime"`, `:136` adds `claudeCmd` to `cmd.AddCommand(...)`, `:63-64` adds `valv claude --help` and `valv claude --version` to root `Example` |
| 8 | Manage stub flip correctness (sentinel error remaining, wrong import alias, wrong error wrap) | REFUTED — `services/manage/service.go:160-165` switches `case domain.ProviderClaude:` to `claudeprovider.DefaultHostProfile(s.homeDir)` with `%w` wrap; import at `:15` is `claudeprovider "github.com/evanmschultz/valv/internal/adapters/providers/claude"`; no "not yet available" text in this case |
| 9 | Manage test update — leftover sentinel-error expectation | REFUTED — `services/manage/service_test.go:144-166` `TestDefaultHostProfileClaudeReturnsIsolatedPath` asserts non-stub return with path containing `.valv/providers/claude/profiles/default` and provider equality; no sentinel-error expectation |
| 10 | `claudeArgsSkipProjectBinding` correctness (wrong return values for documented patterns) | REFUTED — `claude.go:163-179` returns false for empty args; true for any arg `--help`/`-h`; true for single-arg `--version`/`-V`; true when `args[0] == "help"`; matches Codex template |
| 11 | Test reads runtime stdout (brittle, Cobra-version-dependent) | REFUTED — `claude_test.go:22-44` inspects `cmd.Use`, `cmd.Long`, and calls `cmd.Help()` for non-error invocation; does NOT execute the binary or parse stdout. Matches builder's red→green fix |
| 12 | Coverage gaming — assertion-light tests pushed package coverage to 72.3% | REFUTED — `TestClaudeArgsSkipProjectBinding` (`claude_test.go:171-198`) is 9-case table-driven with explicit `t.Fatalf`; `TestRunClaudeCommandUnboundProject` (`:106-130`) uses `errors.Is(err, domain.ErrUnboundProject)` at `:127`, not string match |
| 13 | `mage build` real output — `claude` silently absent from Runtime Commands | EXHAUSTED — independent re-execution of `./valv --help` was permission-denied in this session; `mage build` was permission-denied. The code path is mechanically sound (`root.go:127-128, 136` correctly registers `claudeCmd` with `GroupID="runtime"` and adds it to `AddCommand`). Worklog claim accepted on code-evidence grounds; no contradiction between worklog and file contents |
| 14 | DROP_4 files touched by Unit 5.3 commit | REFUTED — `git show --stat 87f8cdd` shows only `claude.go`, `claude_test.go`, `root.go`, `services/manage/service.go`, `services/manage/service_test.go`, drop docs. `claude_image.go`, `account_auth.go`, `operator_helpers.go` are NOT in the Unit 5.3 commit; their last commit hashes are `e3869f6` and `15df905` (both DROP_4) |
| 15 | Mage discipline violation (raw `go test`/`go build` in worklog) | REFUTED — worklog § "Mage Targets Run" lists only `mage testPkg ./internal/cli`, `mage testPkg ./internal/services/manage`, `mage build`; no raw `go` invocations |
| 16 (self) | `cmd.Execute()` with `DisableFlagParsing: true` strips args before reaching RunE | REFUTED — `claude.go:46-47` declares `DisableFlagParsing: true` and `Args: cobra.ArbitraryArgs`; `TestClaudeCommandPassesArgsThroughUnchanged` (`claude_test.go:202-227`) confirms args pass through unchanged including `--resume`, `--model`, positional `session-123`, `claude-opus-4-5` |
| 17 (self) | `defer store.Close()` ordering hazard — store opened, deferred close registered, then service-construction failure leaves store open | REFUTED — `claude.go:87-91` opens store and immediately registers `defer store.Close()` at `:91` before any later call that could fail. Service construction at `:93-104` either succeeds (deferred close fires on function exit) or returns an error (deferred close still fires). Safe |
| 18 (self) | `runClaudeImageOnlyCommand` calls `ValidateBinding` (which would fail for unbound projects in help/version paths) | REFUTED — `runClaudeImageOnlyCommand` at `claude.go:123-161` does not call `ValidateBinding`; the spec explicitly omits it because help/version shouldn't need a binding |
| 19 (self) | `TestNewClaudeCommandVersion` doesn't actually exercise image-only env wiring | REFUTED — `claude_test.go:50-101` invokes `runClaudeImageOnlyCommand` directly with a fake `docker` script and asserts the captured args contain `CLAUDE_CONFIG_DIR=/home/valv/.claude`, `HOME=/home/valv`, `USER=valv`, container name prefix `valv-claude-info-`, image `valv-claude-dev:dev`, and `--version`. End-to-end env wiring is verified |
| 20 (self) | `TestRunClaudeCommandRejectsWrongProvider` test-name accuracy — does it exercise the direct provider-mismatch guard at `services/claude/service.go:222`? | SOFT FINDING — test at `claude_test.go:135-167` sets up a project with a Codex binding (no Claude binding), then runs `claude`. Since the SQLite-backed `BindingByProjectID(projectID, ProviderClaude)` query filters by provider, it returns `ErrNotFound`, and the service returns wrapped `domain.ErrUnboundProject` — same path as "no project record." The direct `binding.Provider != domain.ProviderClaude` guard at `services/claude/service.go:222-224` is NOT exercised by this CLI-layer test. It IS exercised at the service-test layer by `TestRunRejectsWrongBindingProvider` in `services/claude/service_test.go`. Test passes for the right behavioral reason (no Claude binding exists for that project) but the test name implies a different path than what it actually exercises. Not unit-breaking — production behavior is correct; the guard at the service layer is independently tested |
| 21 (self) | `findDockerBinary` reference at `claude.go:204` is undefined | REFUTED — `findDockerBinary` is a package-level var declared at `codex.go:25` (`var findDockerBinary = exec.LookPath`); same package, valid reference. Comment at `claude.go:203` documents the delegation |
| 22 (self) | `dockerImageMissingError` reference at `claude.go:208` is undefined | REFUTED — `dockerImageMissingError` is declared at `codex.go:258-263`; same package, valid reference |
| 23 (self) | `timeNowUnixNano` reference at `claude.go:137` is undefined | REFUTED — `timeNowUnixNano` is a package-level var declared at `codex.go:172` (`var timeNowUnixNano = func() int64 { return time.Now().UTC().UnixNano() }`); same package, valid reference |

### Evidence Trace

Files read in full and cross-checked:

| Claude artifact | Codex counterpart | Comparison result |
|---|---|---|
| `internal/cli/claude.go` (215 LOC) | `internal/cli/codex.go` (303 LOC) | Net delta: removed `ensureCodexBindingReady` (first-run gate), removed `ensureBoundCodexAccountReady` (host-side auth gate), removed `codexArgsSkipAccountReady` (no equivalent — Claude auth runs in-container), removed `codexImageVersionRef` (no dynamic version compare in v1). `EnsureLatest` → `Build` swap at `:192`. `claude.go` is correctly leaner than `codex.go` by ~90 LOC. Reuses `findDockerBinary`, `dockerImageMissingError`, `timeNowUnixNano` from `codex.go` (same package) |
| `internal/cli/claude_test.go` (227 LOC) | `internal/cli/codex_test.go` (15K reference) | 6 tests covering: command metadata (`TestNewClaudeCommandHelp`), image-only env wiring (`TestNewClaudeCommandVersion`), unbound-project error wrap (`TestRunClaudeCommandUnboundProject`), no-claude-binding rejection (`TestRunClaudeCommandRejectsWrongProvider`), 9-case args-skip table (`TestClaudeArgsSkipProjectBinding`), args-pass-through (`TestClaudeCommandPassesArgsThroughUnchanged`) |
| `internal/cli/root.go` (264 LOC) | n/a — edit only | 6-line edit: `:127-128` declares `claudeCmd` + sets `GroupID`, `:136` includes `claudeCmd` in `AddCommand`, `:63-64` adds two `valv claude` lines to root `Example` |
| `internal/services/manage/service.go` (550 LOC) | n/a — edit only | Stub flip at `:160-165`: switch case `domain.ProviderClaude` now calls `claudeprovider.DefaultHostProfile(s.homeDir)` with `%w` wrap (mirrors `:154-159` Codex branch). Import added at `:15`: `claudeprovider "..."` |
| `internal/services/manage/service_test.go` (705 LOC) | n/a — edit only | Replaced `TestDefaultHostProfileClaudeReturnsSentinelError` with `TestDefaultHostProfileClaudeReturnsIsolatedPath` (`:144-166`) — asserts non-stub return, provider equality, and path containing `.valv/providers/claude/profiles/default` |

Key code sites verified by direct read:

- `claude.go:67-69` — comment explicitly documents the omission of `ensureClaudeBindingReady` (deliberate, not oversight)
- `claude.go:93-104` — service construction uses `claudeservice.Options` with no `SharedHome` field present in the struct; `RealHome: realHomeDir()` is passed for structural parity but unused in v1 service body
- `claude.go:108-115` — `ValidateBinding` is called before `ensureClaudeImageCurrent` (deliberate deviation from Codex order; ensures unbound errors surface without triggering docker build)
- `claude.go:141-143` — labels include `"io.valv.provider": "claude"`, `"io.valv.scope": "info"` (image-only path)
- `claude.go:144-149` — env map: `CLAUDE_CONFIG_DIR`, `HOME`, `LOGNAME`, `USER`; no `CODEX_HOME`
- `claude.go:163-179` — `claudeArgsSkipProjectBinding` returns false for empty, true for any `--help`/`-h`, true for single-arg `--version`/`-V`, true for `args[0] == "help"`
- `claude.go:182-197` — `ensureClaudeImageCurrent` checks `VALV_CLAUDE_IMAGE` env first, then dispatches `openImagesService(cmd, paths, domain.ProviderClaude)` and calls `service.Build(ctx, BuildRequest{Version: DefaultClaudeCLIVersion})`. The `_, err = service.Build(...)` pattern discards the unused `BuildResult` cleanly
- `claude.go:204-213` — `ensureClaudeImageAvailable` reuses `findDockerBinary` and `dockerImageMissingError` from `codex.go`; error message says "valv manage update claude" (Claude-specific)
- `root.go:55-65` — `Example` block contains `valv claude --help` and `valv claude --version` (lines 63-64)
- `root.go:109-113` — `AddGroup` declares `runtime` group at index 1; `claudeCmd.GroupID = "runtime"` at `:128` is valid
- `services/manage/service.go:152-169` — `DefaultHostProfile` switch covers `ProviderCodex` and `ProviderClaude` cases plus `default` unsupported-provider fallback; no leftover sentinel error
- `services/manage/service_test.go:144-166` — assertion includes `Provider == domain.ProviderClaude`, `Name == "default"`, and `strings.Contains(spec.HomePath, ".valv/providers/claude/profiles/default")`

### Soft Findings (not unit-breaking)

1. **`TestRunClaudeCommandRejectsWrongProvider` test-name accuracy.** The test name implies it exercises the `binding.Provider != domain.ProviderClaude` guard at `services/claude/service.go:222-224`. In practice, it sets up a project with a Codex binding (no Claude binding), and the test passes because `BindingByProjectID(projectID, ProviderClaude)` returns `ErrNotFound` — same path as "no project record" (test #3 in the same file). The direct provider-mismatch guard remains untested at the CLI layer (it IS tested at the service layer by `services/claude/service_test.go::TestRunRejectsWrongBindingProvider`). Suggestion: rename to `TestRunClaudeCommandNoBoundClaudeAccount` for clarity, or add a separate test that injects a fake store returning a binding with `Provider: domain.ProviderCodex` to exercise the guard at the CLI layer. Not unit-breaking; production behavior is correct.
2. **`TestNewClaudeCommandVersion` short-circuits via `VALV_CLAUDE_IMAGE` env-set path.** The test sets `VALV_CLAUDE_IMAGE`, which takes the `ensureClaudeImageAvailable` branch at `claude.go:183` rather than the `service.Build` branch at `:192`. The `service.Build` branch (env unset) is not exercised by any unit test in `claude_test.go`. The Codex equivalent `runCodexImageOnlyCommand` test pattern has the same gap, so this matches the established template. Worth a follow-up test once the dogfooded `valv claude --version` smoke command runs the env-unset path in CI. Not breaking; v1 dogfood workflow exercises this path manually.
3. **Independent re-execution of mage targets blocked.** `mage build`, `mage testPkg ./internal/cli`, `mage testPkg ./internal/services/manage`, and `./valv --help` were all permission-denied in this QA session. The worklog claims green status for all four. Verdict is based on code evidence (file reads + git diff) showing the implementation is structurally correct and consistent with the worklog claims. If a counterexample exists in mage output, it would surface in CI after push.

### Acceptance Criteria Verification

All 10 acceptance criteria from PLAN.md § Unit 5.3 verified:

| # | Criterion | Verified by |
|---|---|---|
| 1 | `mage testPkg ./internal/cli` green (72.3% > 60%) | Worklog § "Mage Targets Run" claims 132/132 tests pass; code review of test functions shows behavior-oriented assertions. Independent re-execution blocked by permission |
| 2 | `mage testPkg ./internal/services/manage` green (76.3% > 60%) | Worklog § "Mage Targets Run" claims 23/23 tests pass. Independent re-execution blocked by permission |
| 3 | `mage build` produces binary | Worklog § "Mage Targets Run" claims success; code path is mechanically sound. Independent re-execution blocked by permission |
| 4 | `./valv --help` shows `claude` in Runtime Commands group | `root.go:127-128, 136` correctly registers `claudeCmd` with `GroupID="runtime"` and includes in `AddCommand`. Independent invocation blocked by permission; worklog claims success |
| 5 | `./valv claude --help` exits 0 with usage text including "claude" | `claude.go:29-39` Long description contains "claude" and "Docker"; `TestNewClaudeCommandHelp` verifies `cmd.Help()` exits without error |
| 6 | `claudeArgsSkipProjectBinding(["--version"]) == true` | `claude.go:172-177` switch case `"--version"` returns true; `TestClaudeArgsSkipProjectBinding` table case "version flag" asserts true |
| 7 | `manage.DefaultHostProfile(ProviderClaude)` returns real path (not stub error) | `service.go:160-165` direct read confirms call to `claudeprovider.DefaultHostProfile(s.homeDir)`; `TestDefaultHostProfileClaudeReturnsIsolatedPath` asserts the contract |
| 8 | No reference to `ensureCodexBindingReady` or `runCodexFirstRunSetup` in `claude.go` | Direct read of `claude.go` end-to-end — no such identifiers present |
| 9 | No `sharedClaudeStateHome` in `claude.go` | Direct read of `claude.go` end-to-end — no such identifier present |
| 10 | `SharedHome: ""` passed to `PrepareRuntime` (inside service, not cli) | `services/claude/service.go:127-133` `PrepareRequest` struct literal has `SharedHome: ""` (literal empty string); `claude.go` does not have a `SharedHome` field in its `claudeservice.Options` construction (correct — the option doesn't exist) |

### Counterexamples

None.

### Verdict

**pass** — no unmitigated counterexample to the unit's claim that `internal/cli/claude.go`, `internal/cli/root.go`, `internal/services/manage/service.go`, and their tests correctly implement the Claude pass-through launcher per the dev-confirmed isolated-first model and copy-adapt template. All 10 acceptance criteria verified by direct code read; the 3 mage-execution criteria (1, 2, 3) and the `./valv --help` runtime criterion (4) are accepted on worklog evidence because independent re-execution was permission-denied in this QA session. One soft finding on test-name accuracy (`TestRunClaudeCommandRejectsWrongProvider` exercises "no Claude binding," not the direct provider-mismatch guard); the guard itself is tested at the service-test layer. Two additional soft observations on coverage scope (env-unset `service.Build` branch not exercised by claude unit test; matches Codex test pattern).

### Hylla Feedback

N/A for this unit. The QA review was conducted via direct `Read` of all five changed files (`claude.go`, `claude_test.go`, `root.go`, `services/manage/service.go`, `services/manage/service_test.go`), the Codex CLI template (`codex.go`, `codex_test.go`), the Claude service implementation (`services/claude/service.go`) for the `SharedHome: ""` verification, and the supporting CLI helpers (`claude_image.go`, `account_auth.go`, `operator_helpers.go`) for the DROP_4-files-untouched check. `git log` and `git show --stat` were used to confirm DROP_4 files were not in the Unit 5.3 commit. Hylla was not queried because (a) the files under review are post-ingest (Hylla index is stale relative to HEAD), and (b) the structural-diff review of a copy-adapt unit is more efficiently done via direct `Read` than Hylla vector/keyword search. Bash was permission-denied for `mage` and `./valv` execution.
