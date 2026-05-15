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
