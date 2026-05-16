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

---

## Unit 7.2 — Round 1

**Date:** 2026-05-15
**Reviewer:** go-qa-proof-agent
**Verdict:** pass

### Acceptance criteria verification

| # | Criterion | Evidence (file:line) | Result |
|---|---|---|---|
| 1 | `mage testPkg ./internal/services/claude` green; 18 tests; 80.8% coverage | Re-run produced `tests: 18`, `passed: 18`, `cover 80.8%`, `Minimum package coverage: 60.0%`. Exact match with worklog. | pass |
| 2 | Helper `readClaudeAuthToken(homePath string) (string, error)` exists with doc comment starting with identifier name | `internal/services/claude/service.go:300` signature; doc comment `:291-299` starts with `readClaudeAuthToken` | pass |
| 3a | Reads `<homePath>/.credentials.json` via `os.ReadFile` | `service.go:301` — `os.ReadFile(filepath.Join(homePath, ".credentials.json"))` | pass |
| 3b | Parses JSON, extracts `claudeAiAccessToken` key | `service.go:309-312` — anonymous struct with tag `json:"claudeAiAccessToken"` unmarshaled via `json.Unmarshal(data, &creds)` | pass |
| 3c | Returns `"", nil` when file absent | `service.go:303-305` — `if os.IsNotExist(err) { return "", nil }` | pass |
| 3d | Returns `"", wrapped error` on read/parse failure | `service.go:306` (read err) `fmt.Errorf("read claude credentials: %w", err)`; `:313` (parse err) `fmt.Errorf("parse claude credentials: %w", err)` | pass |
| 4a | After `PrepareRuntime` succeeds, helper called with `profile.HomePath` | `service.go:280` inside `buildRequest`, called after the `ContainerRunRequest{...}` literal at `:254-274` (which itself runs only after `PrepareRuntime` returned nil at `:128-137`) | pass |
| 4b | Non-empty token → env map gets `CLAUDE_CODE_OAUTH_TOKEN` | `service.go:282-283` — `else if token != "" { request.Env["CLAUDE_CODE_OAUTH_TOKEN"] = token }` | pass |
| 4c | Empty token → env var NOT set (container claude fails loudly) | `service.go:284-286` — `else { s.debug("no claude credentials file found, container will run unauthed", ...) }`. No assignment to env. | pass |
| 4d | Helper-returned error → graceful skip + debug log (per PLAN.md AC4, not spawn-prompt's "return error") | `service.go:280-281` — `if token, err := readClaudeAuthToken(...); err != nil { s.debug("claude auth token unreadable", "home", profile.HomePath, "err", err) }`. Matches PLAN.md Design Decisions line 58 "log a debug message and omit ... Do NOT fail Run". Builder's deviation note in worklog confirmed (PLAN.md wins over spawn). | pass |
| 5 | `TestRunSetsClaudeCodeOAuthTokenWhenCredentialsPresent` writes fixture, calls Run, asserts Env key | `service_test.go:531-567` — writes `{"claudeAiAccessToken":"test-oauth-token"}` (`:535-538`); calls `service.Run` (`:559`); asserts `executor.got.Env["CLAUDE_CODE_OAUTH_TOKEN"] == "test-oauth-token"` (`:563-566`). Test does exactly what its name claims. | pass |
| 6 | Token never logged in any path | Exhaustive scan: `:280` error path logs `home` + `err` only (helper wraps `os.ReadFile`/`json.Unmarshal` errors which do not embed token content); `:282-283` assignment, no log; `:285` empty-token debug logs `home` only; `readClaudeAuthToken` returns `read claude credentials: %w` and `parse claude credentials: %w` — neither path could contain the token because token extraction follows error returns | pass |
| 7 | `prepared.Env` writable map (no nil-map crash) | Pre-existing `TestRunSucceedsWithBoundProject` at `service_test.go:204-261` asserts `executor.got.Env["CLAUDE_CONFIG_DIR"] == clauderuntime.ContainerClaudeDir` (`:254-256`) — proves `prepared.Env` is non-nil and writable. Injection at `service.go:283` writes the same map. Worklog notes `clauderuntime.PrepareRuntime` already writes to `prepared.Env` (sets `CLAUDE_CONFIG_DIR`). | pass |

### Idiomatic Go checks

- **Doc comment** on `readClaudeAuthToken` (`service.go:291-299`) starts with the identifier name and explains absent-file vs parse-failure semantics. Standard godoc shape.
- **Error wrapping with `%w`** at every boundary: `:306`, `:313`. Both preserve the underlying `os.ReadFile` / `json.Unmarshal` error chain.
- **`ps(1)` visibility comment** at `:276-279` documents the security trade-off per PLAN.md instruction.
- **Mage-only verification** — re-ran `mage testPkg ./internal/services/claude`; no raw `go test`/`go build`/`gofumpt` invoked.

### Worklog-to-code consistency

- **Production LOC:** helper at `:291-316` = 26 LOC; injection block at `:276-286` = 11 LOC. Combined ~37. Worklog claimed ~33 (~25 helper + ~8 injection). Within margin.
- **Test LOC:** `service_test.go:528-567` = 40 LOC including doc comment. Worklog claimed ~35. Within margin.
- **Schema cross-reference Unit 7.1:** `claude_auth.go:92-94` defines `claudeCredentials` with `AccessToken string \`json:"claudeAiAccessToken"\``; 7.1 writes that shape via `writeClaudeCredentials`. Unit 7.2 `service.go:309-311` reads the identical key via anonymous struct `Token string \`json:"claudeAiAccessToken"\``. Schemas match exactly. Test fixture at `service_test.go:536` writes the same key. Three-way consistent.
- **Spawn-vs-PLAN deviation acknowledged:** Worklog line 77 records that builder followed PLAN.md AC4 (graceful skip on bad JSON) over spawn-prompt's error-propagation hint. Code at `service.go:280-281` confirms graceful skip. PLAN.md is correctly treated as authoritative.
- **`mage testPkg` rerun:** 18/18 pass, 80.8% coverage — exact match with worklog claim.

### Findings

None — all 7 acceptance criteria verified end-to-end with file:line evidence.

### Gaps

- 2.1 [Axis: acceptance-criteria-coverage] [severity: low] PLAN.md AC4 ("When `.credentials.json` is present but unreadable, `Run` succeeds with debug log; env var omitted") has no dedicated test that writes a corrupt JSON file and asserts the env var is absent. The parse-error branch at `service.go:312-314` and the graceful-skip-on-helper-error branch at `service.go:280-281` are covered only by the package-level 80.8% coverage measurement, not by behavior-asserting tests. → spawn brief and PLAN.md Unit 7.3 plan call for `TestRunOmitsClaudeCodeOAuthTokenWhenCredentialsMissing` (existing scenario via `TestRunSucceedsWithBoundProject`'s empty `t.TempDir()`) → Unit 7.3 should add a `TestRunGracefullySkipsTokenInjectionWhenCredentialsCorrupt` test writing malformed JSON and asserting `executor.got.Env["CLAUDE_CODE_OAUTH_TOKEN"] == ""`. Low severity because the coverage gate passes and the code path is straightforward.
- 2.2 [Axis: acceptance-criteria-coverage] [severity: low] No test verifies that the existing `CLAUDE_CONFIG_DIR` env (set by `PrepareRuntime`) survives the injection. `TestRunSetsClaudeCodeOAuthTokenWhenCredentialsPresent` only checks the new key, not that the prior keys are preserved. → trivially mitigated by `TestRunSucceedsWithBoundProject` still passing (which asserts `CLAUDE_CONFIG_DIR` in a no-creds scenario), but a fixture-with-creds variant asserting both keys together would tighten the contract. Optional Unit 7.3 polish.

### Observations

- The injection lives at the tail of `buildRequest` (`service.go:276-286`), AFTER the `ContainerRunRequest` literal is constructed, NOT inside the literal. This is correct because `request.Env` aliases `prepared.Env` (the same map reference) — mutating `request.Env` post-construction is equivalent to mutating `prepared.Env` directly, but reads more clearly at the call site.
- The three-branch dispatch (`err != nil` / `token != ""` / `else`) at `:280-286` gives clean separation of "unreadable", "have token", "no creds file" — each with distinct debug log signatures, which helps operators diagnose why a container ran unauthed.
- `readClaudeAuthToken` is unexported (lowercase `r`), satisfying PLAN.md AC5 verified by inspection at `:300`.
- Coverage jumped from 7.1's package-cli 68.4% → 7.2's package-services-claude 80.8% (different package). Both packages still need Unit 7.3 to lift the `internal/cli` package above the 70% AGENTS.md floor; `internal/services/claude` already exceeds it.

### Summary

PASS. All 7 acceptance criteria verified by file:line citation. `mage testPkg ./internal/services/claude` rerun confirms 18/18 tests, 80.8% coverage — exact match with worklog. Schema (`claudeAiAccessToken`) is consistent across Unit 7.1 writer, Unit 7.2 reader, and test fixtures. Builder's spawn-vs-PLAN deviation (graceful skip on parse error) is correctly resolved in favor of PLAN.md authority. Two low-severity Gaps deferred to Unit 7.3 expansion — neither blocks Unit 7.2 closeout.

### Hylla Feedback

N/A — review touched non-Go files (`PLAN.md`, `BUILDER_WORKLOG.md`) and Go files navigated directly via `Read` per spawn brief's explicit file pointers. No Hylla queries were needed for committed-state lookup on this small in-package review.

---

## Unit 7.3 — Round 1

**Date:** 2026-05-15
**Reviewer:** go-qa-proof-agent
**Verdict:** pass

### Acceptance criteria verification

| # | Criterion | Evidence (file:line) | Result |
|---|---|---|---|
| 1 | `mage testPkg ./internal/cli` green, 151 tests, 70.3% coverage — RESTORED above 70% AGENTS.md floor (up from 7.1 baseline 68.4%) | Rerun produced `tests: 151 passed: 151`, `github.com/evanmschultz/valv/internal/cli 70.3%`, `[SUCCESS] Coverage threshold met` | pass |
| 2 | `mage testPkg ./internal/services/claude` green, 20 tests, 82.3% coverage (up from 7.2's 80.8%) | Rerun produced `tests: 20 passed: 20`, `github.com/evanmschultz/valv/internal/services/claude 82.3%` | pass |
| 3 | `mage testPkg ./internal/adapters/providers/claude` green, 21 tests, 78.4% coverage (unchanged) | Rerun produced `tests: 21 passed: 21`, `github.com/evanmschultz/valv/internal/adapters/providers/claude 78.4%`. No source edits to this package — confirmed by worklog "Files touched" list. | pass |
| 4 | TTY-guard widening: `ensureClaudeAccountReady` checks BOTH `cmd.InOrStdin()` AND `cmd.OutOrStdout()` | `internal/cli/claude_auth.go:111`: `if !commandHasTTY(cmd.InOrStdin()) || !commandHasTTY(cmd.OutOrStdout())`. Exact mirror of `internal/cli/account_auth.go:85` Codex template (`if !commandHasTTY(cmd.InOrStdin()) || !commandHasTTY(cmd.OutOrStdout())`). Identical predicate structure. | pass |
| 5 | Empty-token sentinel in `ensureClaudeAccountReady` returns error before writing | `claude_auth.go:140-142`: `if token == "" { return fmt.Errorf("extract claude token for account %q: keychain returned empty token", account.Name) }`. Sits between `ExtractKeychainToken` return and `writeClaudeCredentials` call — so an empty token never reaches disk. Error message names the account and the failure mode informatively. | pass |
| 6 | Empty-token sentinel in `loginClaudeAccount` returns error before writing | `claude_auth.go:182-184`: identical sentinel block in the no-TTY-guard path. Worklog cited `:196-198`; the actual line is `:182-184` (small numeric drift, function-level position correct). Same informative message format. | pass |
| 7 | `%w` wrap on `exec.LookPath` error so `errors.Is(err, exec.ErrNotFound)` is true; actionable npm-install message preserved | `claude_auth.go:232-238`: `return "", fmt.Errorf("claude CLI not found on PATH; install with: npm install -g @anthropic-ai/claude-code@2.1.89: %w", err)`. The `%w` verb wraps `err` (return from `exec.LookPath`) into the chain. The actionable hint is preserved in the same string. Verified by behavior test `TestRunClaudeHostCommandPreflightFailsWhenClaudeMissing` at `claude_auth_test.go:419-436` asserting both (a) `strings.Contains(err.Error(), "npm install -g @anthropic-ai/claude-code")` and (b) `errors.Is(err, exec.ErrNotFound)`. | pass |
| 8 | 12 new/rewritten tests in `claude_auth_test.go` with meaningful assertions, all using stub via context-key | See test-walk table below — 12 functions identified, each asserts behavior. | pass |
| 9 | `TestSystemClaudeAccountAuthRunnerRunSetupTokenUsesCLAUDE_CONFIG_DIR` proves env reaches subprocess via fake binary | `claude_auth_test.go:443-471` installs fake `claude` shell script at `claude_auth_test.go:65-88` (`installFakeHostClaude`) that writes `CLAUDE_CONFIG_DIR=...` line to a log file from within the subprocess. Test then `os.ReadFile`s the log and asserts `strings.Contains(content, "CLAUDE_CONFIG_DIR="+homePath)` (`:465-467`) and `strings.Contains(content, "args:setup-token")` (`:468-470`). This proves the env var reached subprocess scope (the script reads `${CLAUDE_CONFIG_DIR:-}` at runtime), not just that the parent function was called. Pattern mirrors `TestSystemCodexAccountAuthRunnerLoginUsesCODEXHOME`. | pass |
| 10 | `TestRunOmitsClaudeCodeOAuthTokenWhenCredentialsMissing` exists and asserts Run succeeds + env absent | `service_test.go:572-601` — empty `t.TempDir()` as profile home, calls `service.Run`, asserts `err == nil` (`:594-596`), then asserts `got, ok := executor.got.Env["CLAUDE_CODE_OAUTH_TOKEN"]; ok && got != ""` is false (`:598-600`). Two-axis check (succeeds + absent). | pass |
| 11 | `TestRunOmitsClaudeCodeOAuthTokenWhenCredentialsMalformed` exists and asserts Run succeeds + env absent | `service_test.go:607-642` — writes `NOT VALID JSON` to `.credentials.json` (`:611-614`); calls `service.Run`; asserts `err == nil` ("graceful skip on malformed credentials") (`:635-637`); asserts env var absent (`:639-641`). Pins the graceful-skip-on-parse-error behavior — closes Gap 2.1 from Unit 7.2 Round 1. | pass |

### Test-walk: 12 new/rewritten tests in `claude_auth_test.go`

| # | Test name | File:line | Behavior asserted | Stub-via-context? |
|---|---|---|---|---|
| 1 | `TestEnsureClaudeAccountReadyRejectsNonTTY` | `:94-112` | Non-TTY (bytes.Buffer stdin/stdout) returns error containing "TTY"; `setupTokenHits == 0` (rejected before subprocess) | yes (`installStubClaudeAuth`) |
| 2 | `TestEnsureClaudeAccountReadyRespectsSkipLogin` | `:117-142` | `SkipLogin=true` returns nil; `setupTokenHits == 0`; pre-existing `.credentials.json` preserved (not wiped) | yes |
| 3 | `TestEnsureClaudeAccountReadyNonTTYDoesNotWipe` | `:147-172` | TTY check fires BEFORE wipe — pre-existing creds preserved when non-TTY error returns | yes |
| 4 | `TestLoginClaudeAccountWipesAndRunsSetupTokenAndExtractsAndWrites` | `:186-229` | Full success path via login (bypasses TTY guard). Asserts: setupTokenHits==1, extractHits==1, `.credentials.json` written with `fresh-token`, `ReadAccountIdentity` returns `LoggedIn=true` | yes |
| 5 | `TestLoginClaudeAccountFailsWhenRunSetupTokenErrors` | `:234-259` | `RunSetupToken` error propagates wrapped (`errors.Is(err, setupErr)`); `extractHits == 0`; no `.credentials.json` written | yes |
| 6 | `TestLoginClaudeAccountFailsWhenExtractTokenErrors` | `:263-285` | `ExtractKeychainToken` error propagates wrapped (`errors.Is(err, extractErr)`); no file written | yes |
| 7 | `TestLoginClaudeAccountFailsOnEmptyToken` | `:289-308` | Empty-token sentinel fires (stub returns `("", nil)`); function returns error; no `.credentials.json` written. Proves production fix #5/#6 actually rejects the case. | yes |
| 8 | `TestLoginClaudeAccountSkipsNonTTYGuard` | `:316-338` | `loginClaudeAccount` reaches `RunSetupToken` (`setupTokenHits == 1`) even with non-TTY cmd; error from `ExtractKeychainToken` is returned (not a TTY error). Confirms login bypasses TTY guard. | yes |
| 9 | `TestWipeClaudeCredentialsRemovesFile` | `:344-359` | Existing `.credentials.json` is removed by `wipeClaudeCredentials` | n/a (no runner) |
| 10 | `TestWipeClaudeCredentialsMissingFileIsNoError` | `:363-370` | Missing file is not an error | n/a |
| 11 | `TestRunClaudeHostCommandPreflightFailsWhenClaudeMissing` | `:419-436` | PATH cleared via `t.Setenv("PATH", emptyDir)`; asserts (a) actionable npm-install hint in message, (b) `errors.Is(err, exec.ErrNotFound)` true — directly verifies production fix #7 (the `%w` wrap). | n/a (real exec) |
| 12 | `TestSystemClaudeAccountAuthRunnerRunSetupTokenUsesCLAUDE_CONFIG_DIR` | `:443-471` | Fake `claude` script logs env at subprocess scope; test reads log + asserts both `CLAUDE_CONFIG_DIR=<homePath>` and `args:setup-token` entries present. Proves env reaches subprocess. | n/a (real exec) |

Plus retained helpers from 7.1 (`TestLogoutManagedAccountWipesClaudeCredentials` at `:374-392`, `TestReadAccountIdentityReturnsLoggedOutWhenNoCreds` at `:396-411`) — these were already there in 7.1 and remain green. Net cli package count 144 → 151 (delta +7) reconciles with 12 new minus 5 net replacements: the 7.1 stub-based tests `TestEnsureClaudeAccountReadyFailsWhenRunSetupTokenErrors`, `TestEnsureClaudeAccountReadyFailsWhenExtractTokenErrors`, `TestEnsureClaudeAccountReadyFailsOnEmptyToken`, `TestLoginClaudeAccountSkipsNonTTYGuard`, and `TestEnsureClaudeAccountReadyWipesAndRunsSetupTokenAndExtractsAndWrites` were replaced by their cleaner `Login*` counterparts (success path moved to `loginClaudeAccount` per the design note about TTY mocking).

### Production-fix verification cross-reference

| Fix | Production site | Test that proves it | Codex template mirror |
|---|---|---|---|
| TTY-guard widening | `claude_auth.go:111` (`InOrStdin() OR OutOrStdout()`) | `TestEnsureClaudeAccountReadyRejectsNonTTY` + `TestEnsureClaudeAccountReadyNonTTYDoesNotWipe` — both use `newTestClaudeCmd` which sets both as bytes.Buffer, so widening or non-widening would still trip; coverage is mostly that the existing tests still pass with the wider guard | `account_auth.go:85` identical |
| Empty-token sentinel in ensure | `claude_auth.go:140-142` | Indirectly via `TestLoginClaudeAccountFailsOnEmptyToken` (same logic in login path) — design-note says success path tested via login due to TTY mocking constraint; the ensure-path sentinel is structurally identical to the login-path sentinel | n/a (Codex uses `LoginStatus` recheck) |
| Empty-token sentinel in login | `claude_auth.go:182-184` | `TestLoginClaudeAccountFailsOnEmptyToken` at `:289-308` — directly tests this code path | n/a |
| `%w` on LookPath | `claude_auth.go:232-238` | `TestRunClaudeHostCommandPreflightFailsWhenClaudeMissing` at `:419-436` asserts `errors.Is(err, exec.ErrNotFound)` — would FAIL without the `%w` verb | `account_auth.go:165-169` — note Codex's `runCodexHostCommand` does NOT use `%w` (just `: %w` was added to Claude as a 7.3 hardening). Codex's wrap at `:168` is `fmt.Errorf("find host codex binary: %w", err)` — also uses `%w`. So template is already consistent. |

### Idiomatic Go checks

- **Doc comments** on the new helpers (`installFakeHostClaude` `:62-64`, `newTestClaudeCmd` `:51-52`, `installStubClaudeAuth` `:45`) follow godoc shape.
- **Error wrapping** with `%w` at every boundary in `claude_auth.go`: `:71`, `:73`, `:118`, `:126`, `:130`, `:134`, `:138`, `:144`, `:148`, `:160`, `:168`, `:172`, `:176`, `:180`, `:186`, `:190`, `:204`, `:208`, `:235`. All chain-preserve.
- **No raw `go test`/`go build`/`gofumpt`** — all verification via `mage testPkg`. Confirmed by command logs above (each starts with `[INFO] Started go test -json` which mage drives).
- **No token logging** in production code or tests. Test fixtures use opaque tokens (`"fresh-token"`, `"test-oauth-token"`, `"existing"`, `"old"`) — these are test-only values, not real credentials, and even so are only ever read or asserted-against, never logged.
- **`t.Parallel()` discipline** — env-modifying tests (#11, #12 above) correctly omit `t.Parallel()` per Go 1.26 `t.Setenv` panic-on-parallel rule. Worklog Design notes call this out explicitly. All other tests use `t.Parallel()`.
- **Stub via context-key** — all 8 stub-using tests inject via `claudeAuthRunnerKey{}` (`installStubClaudeAuth` at `:46-49`). No global mutation; cleanly scoped to each cmd.Context.

### Worklog-to-code consistency

- **Test count delta:** worklog claims 144→151 (+7 net). Confirmed by mage rerun: 151 tests. Reconciliation in test-walk paragraph above: 12 new minus 5 replacements of 7.1 stubs = +7 net. Coherent.
- **Coverage claim:** 70.3% confirmed by rerun. Up from 7.1's 68.4% (per worklog); ABOVE the 70% AGENTS.md per-package floor — restoration goal achieved. (Mage's gate is 60% per the `Minimum package coverage: 60.0%` output line — the 70% AGENTS.md target is policy-level, not gate-enforced; either way, 70.3% clears both.)
- **3-production-fix line cites:** Worklog's `:111`, `:140-142`/`:196-198`, `:231-233` correspond to verified `:111`, `:140-142`/`:182-184`, `:232-238`. The empty-token in `loginClaudeAccount` and the LookPath `%w` line counts are slightly off (off-by-14 for login, off-by-1 for LookPath), but the function-level placement is correct and the predicates are exactly what the worklog describes.
- **Schema continuity:** `claude_auth.go:92-94` (`claudeCredentials.AccessToken \`json:"claudeAiAccessToken"\``) ↔ `service.go:309-311` (`Token \`json:"claudeAiAccessToken"\``) ↔ `service_test.go:536`, `:612` (raw JSON fixtures). Four-way consistent across writer + reader + two test fixtures.
- **AC4 in worklog**: marked "PASS (named slightly differently per coverage strategy)" — verified above. The 12 required behaviors from PLAN.md are all covered; some are routed through `loginClaudeAccount` instead of `ensureClaudeAccountReady` due to the non-TTY-bytes.Buffer test constraint. Acceptable per worklog design note + the empty-token sentinel being structurally identical in both functions.

### Findings

None — all 11 spawn-brief criteria verified end-to-end with file:line evidence.

### Gaps

- 2.1 [Axis: spec-conformance] [severity: low] Empty-token sentinel in `ensureClaudeAccountReady` at `claude_auth.go:140-142` has no direct unit test (the equivalent path in `loginClaudeAccount` is covered by `TestLoginClaudeAccountFailsOnEmptyToken`). Structural duplication means the two paths cannot drift independently in practice — the same predicate, the same error message format. The design-note rationale (TTY mocking gap) is acceptable. Not blocking; a future Unit could fold both functions into a shared helper to remove the duplication and the gap simultaneously.
- 2.2 [Axis: spec-conformance] [severity: low] TTY widening fix at `claude_auth.go:111` is exercised only indirectly. `newTestClaudeCmd` sets both stdin AND stdout as bytes.Buffer (`:56-58`) — so the widened predicate trips on stdin alone, and a single-axis stdout-only-non-TTY case isn't explicitly tested. Practical risk is near-zero (any non-interactive cmd has bytes.Buffer for both), but a dedicated test setting stdin = `os.Stdin` (real TTY in interactive runner) + stdout = bytes.Buffer would tighten the contract. Optional polish. Falsification sibling may flag the same.

### Observations

- The success path for `ensureClaudeAccountReady` is intentionally tested via `loginClaudeAccount` because the TTY guard cannot be bypassed in a bytes.Buffer-based test. This is correct coverage strategy: the two functions share identical downstream pipelines (wipe → notice → setup-token → user.Current → extract → empty-check → writeCreds → ReadAccountIdentity), so testing one exercises the other. The TTY guard itself is tested separately by `TestEnsureClaudeAccountReady{RejectsNonTTY,NonTTYDoesNotWipe,RespectsSkipLogin}`.
- Coverage restoration is the main deliverable: 68.4% (7.1 baseline) → 70.3% (7.3) is exactly the kind of focused QA-driven test backfill the AGENTS.md 70% floor exists to enforce. The 12 new tests target the previously-uncovered orchestration functions (`ensureClaudeAccountReady`, `loginClaudeAccount`) and the runner production code (`runClaudeHostCommand`, `systemClaudeAccountAuthRunner.RunSetupToken`).
- `TestRunOmitsClaudeCodeOAuthTokenWhenCredentialsMalformed` at `service_test.go:607-642` directly closes Gap 2.1 from Unit 7.2 Round 1's proof — that gap explicitly called for a corrupt-JSON test, and 7.3 added it. Process discipline working as designed.
- Worklog Hylla Feedback section reads "Hylla answered everything needed" but then says "all Go code reads went directly via `Read`". The correct interpretation per the drop-mid evidence protocol (files just edited may not be in latest ingest) is that Hylla was deliberately skipped rather than queried-and-found. The "N/A" framing in the previous unit's worklog is more precise; minor doc nit, not a finding.
- Test counts: cli 144→151 (+7 net), services/claude 18→20 (+2), adapter 21→21 (0). Sum of 7.3 test additions: 12 cli (with 5 replacements of 7.1 stubs) + 2 service = 14 new functions. Worklog framing is consistent.

### Summary

PASS. All 11 spawn-brief acceptance criteria verified by file:line citation and live mage rerun. Three production hardening fixes from 7.1 QA findings (TTY widening, empty-token sentinel x2, `%w` on LookPath) are correctly placed and behaviorally tested. Coverage RESTORED above AGENTS.md 70% floor (68.4% → 70.3%). Two low-severity Gaps are structural-coverage notes, not behavioral defects — both deferred to optional future polish.

### Hylla Feedback

None — Hylla answered everything needed. Files in this drop are mid-stream (post-edit, pre-ingest) so `Read` is the canonical evidence path per the drop-mid protocol. No Hylla queries fell back.

---

## Unit 7.1 — Round 2

**Date:** 2026-05-15
**Reviewer:** go-qa-proof-agent
**Verdict:** pass

### Bug fix scope

Round 2 fixes a Phase 6 smoke-test UX bug: `valv account switch claude <existing-name>` triggered full OAuth re-auth instead of binding to an already-authed account. Root cause: `ensureClaudeAccountReady` had no already-authed early-return — it unconditionally wiped `.credentials.json` and ran `setup-token` on every call. Fix mirrors the `ensureCodexAccountReady` early-return-when-authed pattern (`account_auth.go:78-83`): check existing credential state first, return nil when authed, only run the auth flow when credentials are missing.

### Acceptance criteria verification

| # | Criterion | Evidence (file:line) | Result |
|---|---|---|---|
| 1 | `ensureClaudeAccountReady` ordering: SkipLogin → already-authed → TTY guard → notice → RunSetupToken → user.Current → ExtractKeychainToken → empty-token sentinel → write | `internal/cli/claude_auth.go:110-162` walked step-by-step: SkipLogin `:111-113`; already-authed stat+size+error-propagate `:114-121`; TTY guard `:122-127`; notice `:128-135`; RunSetupToken `:136-139`; user.Current `:140-143`; ExtractKeychainToken `:144-147`; empty-token sentinel `:148-150`; writeClaudeCredentials `:151-153`; ReadAccountIdentity verify `:154-160` | pass |
| 2 | Already-authed check uses `os.Stat(<homePath>/.credentials.json)` + `info.Size() > 0`; non-NotExist stat errors propagate wrapped | `claude_auth.go:114-121` — `credPath := filepath.Join(strings.TrimSpace(account.HomePath), ".credentials.json")`; `info, statErr := os.Stat(credPath)`; `if statErr == nil && info.Size() > 0 { return nil }`; `if statErr != nil && !os.IsNotExist(statErr) { return fmt.Errorf("check claude credentials for account %q: %w", account.Name, statErr) }` | pass |
| 3 | `wipeClaudeCredentials` call REMOVED from `ensureClaudeAccountReady` body | Walked `claude_auth.go:110-162` end-to-end — zero `wipeClaudeCredentials` invocations remain. Function `wipeClaudeCredentials` itself preserved at `:223-229` for use by `logoutManagedAccount` and `loginClaudeAccount` | pass |
| 4 | `loginClaudeAccount` UNCHANGED — still wipes then runs setup-token then extracts and writes | `claude_auth.go:166-204`: wipe `:167-169`; notice `:170-177`; RunSetupToken `:179-181`; user.Current `:182-185`; ExtractKeychainToken `:186-189`; empty-token sentinel `:190-192`; writeClaudeCredentials `:193-195`; ReadAccountIdentity `:196-203`. Matches Unit 7.1/7.3 Round 1 pipeline exactly. No TTY guard, no SkipLogin — confirmed by `TestLoginClaudeAccountSkipsNonTTYGuard` `claude_auth_test.go:399-421` | pass |
| 5 | `mage testPkg ./internal/cli` green; 154 tests; ≥70% coverage | Live rerun: `tests: 154 passed: 154 failed: 0`; `Minimum package coverage: 60.0%`; `github.com/evanmschultz/valv/internal/cli 70.4%`; `[SUCCESS] All tests passed` (87.01s) | pass |
| 6 | `TestEnsureClaudeAccountReadySkipsWhenAlreadyAuthed` added; asserts nil return AND zero runner calls | `claude_auth_test.go:208-228` — writes fixture `{"claudeAiAccessToken":"tok"}`, calls function with non-TTY cmd, asserts `err == nil` then `stub.setupTokenHits != 0 \|\| stub.extractHits != 0` rejected. Both runner methods proven unreachable | pass |
| 7 | `TestEnsureClaudeAccountReadyAuthsWhenCredentialsMissing` added; proves already-authed check does NOT short-circuit when creds absent | `claude_auth_test.go:236-255` — empty `t.TempDir()` (no creds), non-TTY cmd, asserts error contains "TTY". Per inline docstring at `:230-235`: TTY mention proves the function passed the already-authed check and reached the next gate. The test name "Auths" describes intent of the success path; the assertion verifies the gate ordering via the TTY guard (success path itself is covered by `TestLoginClaudeAccountWipesAndRunsSetupTokenAndExtractsAndWrites`) | pass |
| 8 | `TestEnsureClaudeAccountReadyAlreadyAuthedReturnsNilEvenNonTTY` added; proves already-authed check fires BEFORE the TTY guard | `claude_auth_test.go:149-177` — writes fixture creds, non-TTY cmd (`bytes.Buffer` in+out), asserts `err == nil`, both runner methods zero hits, AND creds file preserved post-call | pass |
| 9 | `TestEnsureClaudeAccountReadyMissingCredsAndNonTTYFailsTTY` added; proves TTY guard still fires when auth is actually needed | `claude_auth_test.go:182-203` — empty `t.TempDir()`, non-TTY cmd, asserts error contains "TTY" AND `stub.setupTokenHits == 0` (TTY guard blocks before runner) | pass |
| 10 | `TestEnsureClaudeAccountReadyNonTTYDoesNotWipe` REMOVED | Full file scan `claude_auth_test.go:1-555` — no test function with that name. Its prior assertion ("non-TTY caller with existing creds gets TTY error AND creds preserved") is invalidated by the new contract: under Round 2, that exact scenario returns nil at the already-authed check before the TTY guard runs. Replacement tests `TestEnsureClaudeAccountReadyAlreadyAuthedReturnsNilEvenNonTTY` (creds present + non-TTY → nil) and `TestEnsureClaudeAccountReadyMissingCredsAndNonTTYFailsTTY` (creds absent + non-TTY → TTY error) together cover both halves of the old test's matrix correctly | pass |

### Call-site verification (switch-no-op behavior)

Walked the path from `valv account switch claude <name>` to `ensureClaudeAccountReady`:

- `runManageAccountSwitch` (`internal/cli/manage.go:561-604`) resolves the target profile then calls `ensureManagedAccountReady(cmd, provider, account, accountAuthOptions{SkipLogin: skipLogin, Paths: paths})` at `:600`.
- `ensureManagedAccountReady` (`internal/cli/account_auth.go:37-46`) dispatches on provider and routes `domain.ProviderClaude` to `ensureClaudeAccountReady` at `:41-42`.
- `ensureClaudeAccountReady` Round 2 body: with creds already on disk in `<account.HomePath>/.credentials.json`, the stat-and-size check at `claude_auth.go:114-118` fires and returns nil before TTY guard or any runner invocation.
- Net effect: `valv account switch claude work` on an already-authed account is now a clean no-op through to `runManageBind` (`manage.go:603`), which performs the binding update without any re-auth flow. This matches the Codex behavior produced by `ensureCodexAccountReady`'s `loggedIn` early-return at `account_auth.go:82-84`.

The switch-no-op behavior is correctly tested at the unit level by `TestEnsureClaudeAccountReadySkipsWhenAlreadyAuthed` and `TestEnsureClaudeAccountReadyAlreadyAuthedReturnsNilEvenNonTTY` — both fixtures place `{"claudeAiAccessToken":"..."}` at the account's `HomePath/.credentials.json` and prove zero runner invocations.

### Worklog numerical claims

| Claim | Verified |
|---|---|
| Production: ~+10 LOC | Net code delta in `ensureClaudeAccountReady`: 8 lines added (stat + size-check return + non-NotExist error propagation), 3 lines removed (the prior `wipeClaudeCredentials` call + its `if err` guard). Net ~+5 to ~+8 LOC against the function body; including the inline doc comment update at `:103-107` totals ~+10. Matches. |
| Test: ~+80 LOC net | Test additions span `claude_auth_test.go:149-255` (`TestEnsureClaudeAccountReadyAlreadyAuthedReturnsNilEvenNonTTY` ~29 LOC, `TestEnsureClaudeAccountReadyMissingCredsAndNonTTYFailsTTY` ~22 LOC, `TestEnsureClaudeAccountReadySkipsWhenAlreadyAuthed` ~21 LOC, `TestEnsureClaudeAccountReadyAuthsWhenCredentialsMissing` ~20 LOC) = ~92 LOC added; removed `TestEnsureClaudeAccountReadyNonTTYDoesNotWipe` was ~12 LOC. Net ~+80 LOC. Matches. |
| Test count: 151 → 154 (+3 net) | Live mage: `tests: 154`. Round 1 (Unit 7.3) closed at 151. Delta = +3 (4 added, 1 removed). Matches. |
| Coverage: 70.3% → 70.4% | Live mage: `github.com/evanmschultz/valv/internal/cli 70.4%`. Matches. |

### Findings

None.

### Gaps

None.

### Observations

- **Already-authed check is presence-only, not validity-only.** The stat+size check at `claude_auth.go:114-118` treats any non-empty `.credentials.json` as "authed" — it does NOT parse the JSON or verify the token field is non-empty. A malformed-but-non-empty file would short-circuit ensure and return nil. The downstream consumer (`internal/services/claude/service.go::readClaudeAuthToken`) handles malformed JSON via graceful-skip (Unit 7.2 design), so the container would launch unauthed rather than fail. This is weaker than the Codex template (which uses `LoginStatus` to actually query codex CLI state), but the spawn-prompt explicitly framed Round 2 as a presence-style mirror and the worklog Design Notes acknowledge this trade-off ("partial/corrupt write should not block re-auth"). Acceptable for Round 2 scope; revisit if Phase 6 smoke testing surfaces malformed-creds edge cases.
- **`TestEnsureClaudeAccountReadyAuthsWhenCredentialsMissing` test name vs. assertion.** The test name promises "Auths When Credentials Missing", but the actual assertion verifies the TTY guard fires (since the test runs in a non-TTY context). The inline docstring at `:230-235` explains this clearly: the TTY-error mention proves the function passed the already-authed check and reached the next gate. The full success path (RunSetupToken + extract + write) is covered by `TestLoginClaudeAccountWipesAndRunsSetupTokenAndExtractsAndWrites` at `:269-312` which uses `loginClaudeAccount` to bypass the TTY guard. Coverage strategy is sound; test name slightly misleading but docstring rescues it. Non-blocking style nit.
- **SkipLogin precedence preserved.** SkipLogin still runs BEFORE the already-authed check (`claude_auth.go:111-113` before `:114-121`), so a caller with SkipLogin=true and no creds still returns nil (consistent with Unit 7.1 Round 1 semantics). This is the correct ordering: SkipLogin is an explicit caller override, already-authed is a state-derived shortcut. Both reach the same nil-return.
- **`loginClaudeAccount` deliberately retains the wipe-first contract.** This is correct by design: explicit `valv account login` is a force-fresh action that should always re-authenticate, even when creds already exist. Only the implicit `account switch` path needed the early-return — and that path goes through `ensureClaudeAccountReady`, not `loginClaudeAccount`.

### Summary

PASS. All 10 acceptance criteria verified by file:line citation and live `mage testPkg ./internal/cli` rerun (154/154 pass, 70.4% coverage). Round 2 fix correctly mirrors the Codex `ensureCodexAccountReady` early-return-when-authed pattern at a behavioral level: stat+size presence check replaces a real `LoginStatus` call, with the trade-offs documented in Observations. The switch-no-op UX bug is now closed: `valv account switch claude <name>` on an already-authed account returns at `claude_auth.go:117` before any wipe, setup-token, or extract step runs. `loginClaudeAccount`'s force-fresh wipe contract is preserved unchanged. Test matrix correctly covers both halves (creds-present → nil vs. creds-absent → TTY error) via the four new tests plus the removed test's matrix replaced.

### Hylla Feedback

None — task touched only files mid-stream in the drop (post-edit, pre-ingest), so `Read` is the canonical evidence path per the drop-mid protocol. No Hylla queries fell back. One grep/rg invocation was sandbox-blocked when locating `runManageAccountSwitch` in `manage.go`; resolved by reading a 300-line offset window directly (`manage.go:400-700` covered the call site). Not a Hylla issue — orchestrator harness tool restriction.

---

## Unit 7.1 — Round 3

**Date:** 2026-05-15
**Reviewer:** go-qa-proof-agent
**Verdict:** pass

Round 3 is a design pivot following the dev smoke test 2026-05-15: `claude setup-token` produces `user:inference`-scoped tokens (insufficient for interactive container claude), and wrapping the keychain blob in `{"claudeAiAccessToken":"<blob>"}` broke container claude's native `.credentials.json` parsing. The fix: (a) switch container args to `auth login` (full session scope), (b) write the keychain blob VERBATIM to `.credentials.json` with no wrapping, no `json.Marshal`, no struct.

### Acceptance criteria verification

| # | Criterion | Evidence (file:line) | Result |
|---|---|---|---|
| 1 | `mage testPkg ./internal/cli` green, ≥70% coverage | live rerun: `tests: 154 passed: 154`, `github.com/evanmschultz/valv/internal/cli 70.4%` | pass |
| 2 | Container args literal `"auth", "login"` (not `"setup-token"`) | `claude_auth.go:50` — `runClaudeHostCommand(ctx, homePath, stdin, stdout, stderr, "auth", "login")` | pass |
| 3 | Interface method renamed `RunSetupToken` → `RunAuthLogin` | interface `claude_auth.go:30-33`; method body `:43-52`; call sites `:133` (`ensureClaudeAccountReady`) and `:175` (`loginClaudeAccount`); test stub `claude_auth_test.go:35-38` | pass |
| 4 | `writeClaudeCredentials` writes verbatim via single `os.WriteFile` (no `json.Marshal`, no wrapping) | `claude_auth.go:207-213` — function body is exactly `credPath := filepath.Join(...); os.WriteFile(credPath, []byte(credentialsBlob), 0o600)` plus error wrap. No `json.Marshal`. No struct alloc. | pass |
| 5 | `encoding/json` import removed from `claude_auth.go` | import block `claude_auth.go:3-20` — `bytes`, `context`, `fmt`, `io`, `os`, `os/exec`, `os/user`, `path/filepath`, `strings` + three external packages. No `encoding/json` | pass |
| 6 | `claudeCredentials` struct deleted | full-file scan confirms no `type claudeCredentials` declaration anywhere in `claude_auth.go` | pass |
| 7 | Empty-token sentinel preserved | `ExtractKeychainToken` body `claude_auth.go:78-81` returns error when trimmed token is empty; downstream re-check at `:144-146` and `:186-188` returns wrapped error if extractor returns `("", nil)` | pass |
| 8 | File mode `0o600` (owner-only) | `claude_auth.go:209` — `os.WriteFile(credPath, []byte(credentialsBlob), 0o600)` | pass |
| 9 | Test fake claude script expects `auth login` args | `claude_auth_test.go:75` — `if [ "${1:-}" = "auth" ] && [ "${2:-}" = "login" ]; then` | pass |
| 10 | Renamed test function + updated args/log assertion | `claude_auth_test.go:526` — `TestSystemClaudeAccountAuthRunnerRunAuthLoginUsesCLAUDE_CONFIG_DIR`. Method call line 533 — `systemClaudeAccountAuthRunner{}.RunAuthLogin(...)`. Log assertion line 551 — `strings.Contains(string(content), "args:auth login")` | pass |
| 11 | All `runner.RunAuthLogin` call sites match the renamed method | `ensureClaudeAccountReady` line 133; `loginClaudeAccount` line 175; preflight test still uses `"auth", "login"` args at `:508`. No residual `RunSetupToken` reference in `internal/cli/` (verified by full file scan of `claude_auth.go`+`claude_auth_test.go`) | pass |

### Idiomatic Go checks

- **No `json.Marshal` / `json.Unmarshal` in `claude_auth.go`** — full file scan confirms zero matches. The verbatim-write design eliminates the json package dependency entirely from this file.
- **No token logging** — `writeClaudeCredentials` only references `credPath` in error messages, never the blob. `ExtractKeychainToken` errors reference `security` command output, never the token value.
- **Doc comments updated** — `claudeKeychainService:23-24`, `RunAuthLogin:43-48`, `ExtractKeychainToken:54-60`, `ensureClaudeAccountReady:92-105`, `writeClaudeCredentials:202-206` all reflect the new `auth login` + verbatim semantics. Each starts with the identifier name per Go style.
- **Imports cleanly pruned** — `encoding/json` removed; `os`, `os/exec`, `os/user`, `path/filepath`, `bytes` retained because still used (`os.WriteFile`, `os.Remove`, `os.Stat`, `os.IsNotExist`, `exec.LookPath`, `exec.CommandContext`, `user.Current`, `filepath.Join`, `bytes.Buffer`).
- **Error wrapping** — every error path uses `%w` (`claude_auth.go:74, 76, 80, 117, 119-121, 134, 138, 142, 148, 152, 164, 176, 180, 184, 190, 194, 211, 220, 237-240`).

### Observations

- **Stale test naming** — three existing tests retain "SetupToken" in their names: `TestLoginClaudeAccountWipesAndRunsSetupTokenAndExtractsAndWrites` (line 269), `TestLoginClaudeAccountFailsWhenRunSetupTokenErrors` (line 317), and the stub struct's `setupTokenErr` / `setupTokenHits` fields (line 27-33). The worklog acknowledges this implicitly (only the one explicit rename was performed). All three tests still assert correct behavior against the renamed `RunAuthLogin` method via the stub. **This is a code-cleanliness gripe, not a verdict-blocker** — production interface + method name + call sites are all correctly renamed; only stale identifiers remain inside tests. A future refactor pass should rename them for consistency, but no functional impact.
- **`TestLoginClaudeAccountWipesAndRunsSetupTokenAndExtractsAndWrites` correctness under verbatim write** — stub returns `extractToken: "fresh-token"` (a bare string, not real JSON). With the old wrapping write, file content was `{"claudeAiAccessToken":"fresh-token"}`; with the new verbatim write, file content is exactly `fresh-token`. Both pass the test's `strings.Contains(string(data), "fresh-token")` assertion. Worklog explicitly calls this out — acceptable since the stub isolates the write path and live keychain integration is smoke-test territory.

### Verdict rationale

PASS. All 11 acceptance criteria verified by file:line citation and live `mage testPkg ./internal/cli` rerun (154/154 pass, 70.4% coverage — unchanged test count vs Round 2 since this is a behavioral pivot, not an addition/removal). Container args `"auth", "login"` (line 50) + verbatim `os.WriteFile` (line 209) + deleted `claudeCredentials` struct + deleted `encoding/json` import together close the design pivot. Empty-token sentinel preserved. File mode `0o600` preserved. Interface rename propagated cleanly across production and the one test that explicitly references the method name.

### Hylla Feedback

None — Hylla answered the bind-mount question on first query. `hylla_node_full` on `ContainerClaudeDir` confirmed the constant value `/home/valv/.claude`; `hylla_node_full` on `PrepareRuntime` returned the full function body showing `dockeradapter.NewMountSpec(runtimeClaudeHome, ContainerClaudeDir, false)` in the mounts slice. One earlier `hylla_search_keyword` query for `PrepareRuntime claude mount` ranked codex-side variants higher than the claude-side function (probably because the codex package was indexed first and the summary keywords cluster); a more specific `ContainerClaudeDir` query resolved the miss. Suggestion: when a query mentions a provider name (`claude`), keyword search could weight summaries containing the same provider token.

---

## Unit 7.2 — Round 2

**Date:** 2026-05-15
**Reviewer:** go-qa-proof-agent
**Verdict:** pass

Round 2 is a follow-on cleanup to Unit 7.1 Round 3's design pivot. With `.credentials.json` now written verbatim in the format container claude reads natively, the `CLAUDE_CODE_OAUTH_TOKEN` env-var injection layer is no longer needed and is fully removed.

### Acceptance criteria verification

| # | Criterion | Evidence (file:line) | Result |
|---|---|---|---|
| 1 | `readClaudeAuthToken` helper deleted from `service.go` | full-file scan of `internal/services/claude/service.go` (346 lines) confirms no `readClaudeAuthToken` function declaration anywhere. Last surviving function in the file is `sanitizeContainerPart:325` | pass |
| 2 | `CLAUDE_CODE_OAUTH_TOKEN` env-var injection block deleted from `buildRequest` | `buildRequest` body `service.go:244-276` contains no env-injection code path. The `request.Env` field is set once at line 257 to `prepared.Env` (from `PrepareRuntime`) and never mutated for token injection. No `CLAUDE_CODE_OAUTH_TOKEN` string literal anywhere in file | pass |
| 3 | `encoding/json` import removed from `service.go` | import block `service.go:3-21` — `context`, `errors`, `fmt`, `io`, `os`, `path/filepath`, `strings`, `time`, `unicode`, plus `log`, `docker`, `clauderuntime`, `domain`, `pathutil`, `projectdetect`. No `encoding/json` | pass |
| 4 | `os` + `path/filepath` retained (still used elsewhere) | `os.TempDir()` at `service.go:96`; `filepath.Rel` at `:279`, `filepath.Separator` at `:286`, `filepath.Base` at `:317` | pass |
| 5 | Three deleted tests removed from `service_test.go` | full-file scan of `internal/services/claude/service_test.go` (550 lines) confirms none of the three function names appear: `TestRunSetsClaudeCodeOAuthTokenWhenCredentialsPresent`, `TestRunOmitsClaudeCodeOAuthTokenWhenCredentialsMalformed`, `TestRunOmitsClaudeCodeOAuthTokenWhenCredentialsMissing` — all three absent | pass |
| 6 | Test count = 17 | 14 top-level `func Test*` + 3 subtests in `TestNewRequiresDependencies` (cases "nil store", "nil executor", "empty image repository", lines 174-185) = 17. Matches mage output `tests: 17 passed: 17` | pass |
| 7 | `service_test.go` imports cleanly pruned (no unused `os` / `path/filepath` / `encoding/json`) | import block `service_test.go:3-18` — `context`, `errors`, `fmt`, `io`, `strings`, `testing`, `time`, `log`, `docker`, `clauderuntime`, `domain`, `projectdetect`. No `os`, no `path/filepath`, no `encoding/json` | pass |
| 8 | `mage testPkg ./internal/services/claude` green, ≥70% coverage | live rerun: `tests: 17 passed: 17`, `github.com/evanmschultz/valv/internal/services/claude 81.0%` | pass |
| 9 | `mage testPkg ./internal/adapters/providers/claude` green, unchanged | live rerun: `tests: 21 passed: 21`, `github.com/evanmschultz/valv/internal/adapters/providers/claude 78.4%` | pass |
| 10 | Bind-mount of `<managed-home>` → `/home/valv/.claude` unchanged | `PrepareRuntime` content (via Hylla `hylla_node_full`): `mounts := []dockeradapter.MountSpec{dockeradapter.NewMountSpec(runtimeClaudeHome, ContainerClaudeDir, false)}` where `ContainerClaudeDir = "/home/valv/.claude"` (const at `runtime.go`). `Service.Run` `service.go:127-130` passes `SharedHome: ""`, so `PrepareRuntime`'s `sharedHome := profileHome` path runs and `runtimeClaudeHome = sharedHome = profileHome`. Mount target unchanged from DROP_5 | pass |

### Idiomatic Go checks

- **Coverage delta** — 82.3% → 81.0% (−1.3 pp), explained by removing the three env-injection tests. Worklog acknowledges this; the floor (60% by mage's `Minimum package coverage`, 70% per AGENTS.md guidance) is comfortably cleared.
- **`buildRequest` returns the runtime's env as-is** — `service.go:257` (`Env: prepared.Env`) — `prepared.Env` from `PrepareRuntime` includes `CLAUDE_CONFIG_DIR=/home/valv/.claude`, `HOME=/home/valv`, `LOGNAME=valv`, `TERM=...`, `USER=valv`. No token-injection step.
- **No raw `go test` / `go build`** — verified via mage targets only.
- **Error wrapping** — all errors in `service.go` use `%w` (`:78, 81, 84, 135, 142, 167, 178, 198, 203, 211, 213, 218, 220, 230, 232, 281`).

### Observations

- **Coverage gate alignment** — `mage testPkg` reports `Minimum package coverage: 60.0%`. AGENTS.md § 11 states the 70% per-package floor. Both packages clear 70% (cli 70.4%, services/claude 81.0%, adapters/providers/claude 78.4%). The 60% line in mage output is the per-target floor for testPkg; the 70% drop-end gate runs via `mage test`. No discrepancy with policy.
- **Bind-mount path correctness end-to-end** — confirmed via two-step trace: (1) `service.go:127-133` builds `clauderuntime.PrepareRequest{ProfileHome: resolved.profile.HomePath, SharedHome: ""}`. (2) `PrepareRuntime` `runtime.go` body shows `sharedHome := profileHome` when `request.SharedHome` is whitespace-only, then mounts `runtimeClaudeHome` (= `profileHome`) at `ContainerClaudeDir`. So `<homePath>/.credentials.json` becomes `/home/valv/.claude/.credentials.json` inside the container — exactly where container claude expects it.
- **`TestRunSucceedsWithBoundProject` still validates the mount** — `service_test.go:241-249` iterates `executor.got.Mounts` checking `m.Target == clauderuntime.ContainerClaudeDir`. This is the integration assertion that the mount is still present after Unit 7.2's deletions — and it passes (in the 17-test run).

### Verdict rationale

PASS. All 10 acceptance criteria verified by file:line citation and live mage reruns (services/claude 17/17 / 81.0%; adapters/providers/claude 21/21 / 78.4%). `readClaudeAuthToken` is gone; `CLAUDE_CODE_OAUTH_TOKEN` injection is gone; `encoding/json` is gone from both files; the three obsoleted tests are gone; imports pruned. The bind-mount mechanism (DROP_5) carrying `.credentials.json` from `profileHome` to `/home/valv/.claude` is unchanged and still asserted by `TestRunSucceedsWithBoundProject`.

### Final design summary

End-to-end auth flow after Unit 7.1 Round 3 + Unit 7.2 Round 2:

1. **Host trigger** — `valv claude` → `ensureClaudeAccountReady` checks `.credentials.json` presence.
2. **Host auth** — if missing + TTY: `runner.RunAuthLogin(homePath)` shells out to host `claude auth login` with `CLAUDE_CONFIG_DIR=homePath` (full session scope, not headless inference scope).
3. **Keychain extraction** — `runner.ExtractKeychainToken(macOSUser)` reads the JSON blob from macOS keychain service `"Claude Code-credentials"`.
4. **Verbatim write** — `writeClaudeCredentials(homePath, blob)` writes blob as-is to `<homePath>/.credentials.json` (mode `0o600`).
5. **Container launch** — `Service.Run` → `PrepareRuntime` bind-mounts `homePath` → `/home/valv/.claude` (unchanged DROP_5 mechanism).
6. **Container reads natively** — container `claude` reads `/home/valv/.claude/.credentials.json` in the exact format the keychain stored it. No env-var, no second JSON envelope.

### Hylla Feedback

None — `hylla_node_full` on `PrepareRuntime` returned the full function body including the mounts slice; `hylla_node_full` on `ContainerClaudeDir` confirmed the const value. One earlier keyword search for "PrepareRuntime claude mount" ranked codex-side variants higher; resolved by direct `node_full` on the claude-side const + function via known IDs. Mild ergonomics suggestion: when keyword search includes a provider/package qualifier, prefer summaries containing that token over more-popular cross-package hits.
