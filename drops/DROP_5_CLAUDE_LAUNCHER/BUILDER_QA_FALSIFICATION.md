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
