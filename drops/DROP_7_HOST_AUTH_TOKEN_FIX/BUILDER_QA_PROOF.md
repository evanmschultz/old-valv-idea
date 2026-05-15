# DROP_7_HOST_AUTH_TOKEN_FIX — Builder QA Proof Review

Append a `## Unit 7.M — Round K` section per build-QA proof round. Asymmetric pair with `BUILDER_QA_FALSIFICATION.md`. See `main/drops/WORKFLOW.md` § "Phase 5 — Build-QA (per unit)".

---

## Unit 7.1 — Round 1

**Date:** 2026-05-15
**Reviewer:** go-qa-proof-agent
**Verdict:** pass

### Acceptance criteria verification

| # | Criterion | Evidence (file:line) | Result |
|---|---|---|---|
| 1 | `mage testPkg ./internal/cli` green; 144 tests; 68.4% coverage | `mage testPkg ./internal/cli` rerun produced `tests: 144 passed: 144`, `Minimum package coverage: 60.0%`, `github.com/evanmschultz/valv/internal/cli 68.4%` | pass |
| 2 | `claudeAuthRunner` interface has exactly two methods (`RunSetupToken`, `ExtractKeychainToken`) | `internal/cli/claude_auth.go:31-34` | pass |
| 3 | `systemClaudeAccountAuthRunner` struct implements both methods | struct decl `claude_auth.go:42`; `RunSetupToken` body `:48-51`; `ExtractKeychainToken` body `:58-80` | pass |
| 4 | Inline `exec.LookPath("claude")` preflight with actionable remediation message | `claude_auth.go:226-231` returns `"claude CLI not found on PATH; install with: npm install -g @anthropic-ai/claude-code@2.1.89"` | pass |
| 5 | `ensureClaudeAccountReady` ordering: SkipLogin → TTY → wipe → notice → RunSetupToken → user.Current → ExtractKeychainToken → write `.credentials.json` | `claude_auth.go:107-151` (full function). SkipLogin `:108-110`; TTY `:111-116`; wipe `:117-119`; notice `:120-127`; RunSetupToken `:128-131`; user.Current `:132-135`; ExtractKeychainToken `:136-139`; writeClaudeCredentials `:140-142`. Bonus `ReadAccountIdentity` verify step at `:143-149` (matches PLAN.md Unit 7.1 spec step 10). | pass |
| 6 | `loginClaudeAccount` mirrors `ensure` MINUS TTY guard AND MINUS SkipLogin | `claude_auth.go:155-190` — starts at `wipeClaudeCredentials`, no SkipLogin/TTY checks, same downstream pipeline | pass |
| 7 | `wipeClaudeCredentials` preserved as file-wipe only, no keychain delete | `claude_auth.go:209-215` — only `os.Remove(credPath)`; no `security delete-generic-password` shell-out anywhere in file | pass |
| 8 | All `dockeradapter` imports removed | Import block `claude_auth.go:3-21` — no `dockeradapter`. Block contains only stdlib + `laslig`, `cobra`, `claudeprovider`, `config`, `domain` | pass |
| 9 | `.credentials.json` JSON shape `{"claudeAiAccessToken": "<token>"}` (single key) | `claudeCredentials` struct `claude_auth.go:92-94` with single field `AccessToken string \`json:"claudeAiAccessToken"\``. Marshaled `:195-196`, written via `os.WriteFile` mode 0o600 at `:200-202` | pass |
| 10 | `security` args: `find-generic-password -s "Claude Code-credentials" -a <username> -w` | `claude_auth.go:59-64` — args slice `"find-generic-password", "-s", claudeKeychainService, "-a", macOSUser, "-w"`. Constant `claudeKeychainService = "Claude Code-credentials"` at `:26` | pass |

### Idiomatic Go checks

- **Doc comments on exported identifiers** — all start with identifier name:
  - `claudeKeychainService` at `:23-26` ✓
  - `claudeAuthRunner` at `:28-30` ✓
  - `systemClaudeAccountAuthRunner` at `:40-41` ✓
  - `RunSetupToken` at `:44-47` ✓
  - `ExtractKeychainToken` at `:53-57` ✓
  - `claudeCredentials` at `:89-91` ✓
  - `ensureClaudeAccountReady` at `:96-106` ✓
  - `loginClaudeAccount` at `:153-154` ✓
  - `writeClaudeCredentials` at `:192-193` ✓
  - `wipeClaudeCredentials` at `:207-208` ✓
  - `runClaudeHostCommand` at `:217-224` ✓
- **Error wrapping** — every boundary uses `fmt.Errorf("ctx: %w", err)` (`:71`, `:73`, `:118`, `:130`, `:134`, `:138`, `:141`, `:145`, `:157`, `:165`, `:169`, `:173`, `:177`, `:180`, `:184`, `:198`, `:202`, `:212`). One exception flagged in Observations.
- **No token logging** — exhaustive scan of `fmt.Errorf` and writer calls in the file: the extracted token value at `:75` is only assigned to disk via `writeClaudeCredentials`; never appears in any error message, log call, or writer write. Stderr from `security` (`:69`) is diagnostic text from macOS, never the token itself.
- **Mage-only verification** — re-ran `mage testPkg ./internal/cli`; no raw `go test`/`go build`/`gofumpt` used.

### Worklog-to-code consistency

- **LOC:** `wc -l internal/cli/claude_auth.go` = 247 lines. Worklog claimed ~210; planner projected 160-180 with full-file-rewrite exception. Slight overshoot, within planner-granted exception scope.
- **Test count:** 144 tests reported by mage — matches worklog claim exactly.
- **Coverage:** 68.4% — matches worklog claim exactly. Below AGENTS.md § 11 70% floor, above the mage-enforced 60% gate. **Expected for stub-test round per spawn brief.**
- **Schema cross-package consistency:** Unit 7.1 writes `claudeAiAccessToken` (key tag at `:93`). Unit 7.2's `readClaudeAuthToken` (per worklog line 73-75) reads the same key. Test fixture at `claude_auth_test.go:43` writes the same key in stub success path. Three-way consistent.
- **Container-era code deletion:** No occurrence of `buildClaudeAuthContainerRequest`, `EnsureImage`, or `RunContainer` in `claude_auth.go`. No `valv-claude:dev` container references in this file. No `dockeradapter` import (criterion 8).

### Findings

- 1.1 [Axis: acceptance-criteria-coverage] [severity: low] `exec.LookPath("claude")` error returned at `claude_auth.go:228-231` builds the message via `fmt.Errorf(...)` WITHOUT `%w`, dropping the underlying `exec.ErrNotFound` from the error chain. The codex template at `account_auth.go:167-169` uses `fmt.Errorf("find host codex binary: %w", err)` and preserves the cause. → file:line `claude_auth.go:228-231` → consider rewriting to `fmt.Errorf("claude CLI not found on PATH; install with: npm install -g @anthropic-ai/claude-code@2.1.89: %w", err)` or restructuring to surface both the remediation and wrap the cause. Low severity because no caller is currently type-asserting on `exec.ErrNotFound` and the user-facing remediation message dominates.

### Gaps

- 2.1 [Axis: acceptance-criteria-coverage] [severity: medium] Package coverage at 68.4% sits below AGENTS.md § 11 per-package 70% floor (passes the mage 60% gate). Stub tests cover SkipLogin, non-TTY guard, non-TTY-preserves-creds, logout-wipes-creds, wipe-missing-OK, login-no-TTY-guard — but `runClaudeHostCommand`, `writeClaudeCredentials`, `ExtractKeychainToken`, and the success path of `ensureClaudeAccountReady`/`loginClaudeAccount` are not directly exercised. → spawn brief confirms "test coverage will be restored in 7.3" → Unit 7.3 must add `TestSystemClaudeAccountAuthRunnerRunSetupTokenUsesCLAUDE_CONFIG_DIR`, `TestRunClaudeHostCommandFailsWhenClaudeMissing`, `TestEnsureClaudeAccountReadyRunsSetupTokenAndWritesCreds`, etc. per PLAN.md Unit 7.3 plan.
- 2.2 [Axis: acceptance-criteria-coverage] [severity: low] Q4 (keychain service name verification — does `setup-token` actually use `"Claude Code-credentials"`?) remains an open item that the orchestrator/dev must resolve via live smoke test. Builder set the constant to the planner-confirmed value; cannot live-verify from subagent context. → flagged in worklog Unknowns; resolution belongs to drop-end smoke testing, not Unit 7.1 build-QA.

### Observations

- The `ensureClaudeAccountReady` orchestration includes an extra step beyond the appendix's 8-step list: `claudeprovider.ReadAccountIdentity` verification at `:143-149` confirms `LoggedIn=true` after credential write. This matches PLAN.md Unit 7.1 spec step 10 and `loginCodexAccount`'s post-login verification pattern. Reads as a strengthening, not a deviation.
- `runClaudeHostCommand` is defined but is only called by `RunSetupToken` (line 49); `ExtractKeychainToken` uses `exec.CommandContext` against `security` directly (line 59). This matches PLAN.md line 89 spec exactly — `runClaudeHostCommand` is the `claude`-binary helper, `ExtractKeychainToken` shells out to the OS keychain tool.
- `loginClaudeAccount` has an unused `_ config.Paths` third parameter — intentional per worklog (preserves caller-site compatibility with `account_auth.go:64`).
- Coverage gap is consistent with the spawn brief's statement that Unit 7.3 will restore test coverage to the 70% floor.

### Summary

PASS. All 10 acceptance criteria verified by file:line citation. mage rerun confirms 144 tests pass and coverage matches worklog (68.4%). One low-severity finding on a missing `%w` wrap at the `exec.LookPath` error path — non-blocking. Coverage gap is expected for the stub-test round and explicitly deferred to Unit 7.3.

### Hylla Feedback

N/A — review touched non-Go files (`PLAN.md`, `BUILDER_WORKLOG.md`) and Go files navigated directly via `Read` based on the spawn brief's explicit file pointers. No Hylla query was needed for committed-state lookup on this small in-package review.
