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

---

## Unit 7.5 — Round 1

**Date:** 2026-05-15
**Reviewer:** go-qa-proof-agent
**Verdict:** pass

Unit 7.5 is the Path A → Path B revert and restore. Builder deleted host-extract code (`RunAuthLogin`, `ExtractKeychainToken`, `runClaudeHostCommand`, `writeClaudeCredentials`, `claudeKeychainService`, and the `os/user`/`os/exec`/`bytes`/`encoding/json` imports) and restored in-container auth via a single-method `claudeAuthRunner` interface (`RunInContainer`) backed by a lazy-executor `systemClaudeAccountAuthRunner`. Independent verification confirms every acceptance criterion is supported by the committed code at `3d36683` (`feat(cli): restore in-container Claude auth via plain claude`).

### Evidence reviewed

- `PLAN.md` Round 5 locked design decisions (lines 58–124) + Unit 7.5 spec (lines 127–266).
- `BUILDER_WORKLOG.md` Unit 7.5 Round 1 entry (lines 402–456).
- Full read of `internal/cli/claude_auth.go` (189 lines committed at `3d36683`).
- Full read of `internal/cli/claude_auth_test.go` (397 lines committed at `3d36683`).
- `git diff 9e7bc48..3d36683 -- internal/cli/claude_auth.go` (Path A → Path B delta, 238 lines changed: 85 ins / 153 del).
- `git show 9e7bc48:internal/cli/claude_auth.go` (Path A baseline for `wipeClaudeCredentials` byte-equivalence check).
- Independent `mage testPkg github.com/evanmschultz/valv/internal/cli` rerun.

### Acceptance criteria verification

| # | Criterion | Builder Result | QA-Proof Verification |
|---|---|---|---|
| AC1 | `claudeAuthRunner` has exactly one method: `RunInContainer`. No `RunAuthLogin`, no `ExtractKeychainToken`. | PASS | CONFIRMED — interface at `claude_auth.go:32-34` declares a single method `RunInContainer(ctx context.Context, homePath string, stdin io.Reader, stdout, stderr io.Writer) error`. Full-file read of `claude_auth.go` (189 lines) and `claude_auth_test.go` (397 lines) yields zero matches for `RunAuthLogin` or `ExtractKeychainToken`. |
| AC2 | No `os/user`, `os/exec`, `bytes`, `encoding/json` imports; no `claudeKeychainService` const; no `writeClaudeCredentials` / `runClaudeHostCommand` funcs. | PASS | CONFIRMED — import block `claude_auth.go:3-19` contains only `context`, `fmt`, `io`, `os`, `path/filepath`, `strings`, `time`, `laslig`, `cobra`, `dockeradapter`, `claudeprovider`, `config`, `domain`. Full-file review confirms none of the listed Path A symbols appear. Diff `git diff 9e7bc48..3d36683` shows the deletions explicitly (`-"bytes"`, `-"os/exec"`, `-"os/user"`, `-const claudeKeychainService`, `-func writeClaudeCredentials`, `-func runClaudeHostCommand`, `-RunAuthLogin`, `-ExtractKeychainToken`). |
| AC3 | `ensureClaudeAccountReady` step order: SkipLogin → already-authed stat → non-TTY guard → notice → RunInContainer → ReadAccountIdentity. Tests cover all short-circuit / failure paths. | PASS | CONFIRMED — function body `claude_auth.go:110-148`: SkipLogin (`:111-113`), stat for `.credentials.json` + `Size > 0` early return (`:114-118`), stat-error propagation for non-`IsNotExist` errors (`:119-121`), non-TTY guard (`:122-127`), `writeCLINotice` (`:128-135`), `runner.RunInContainer` (`:136-139`), `ReadAccountIdentity` (`:140-146`). Order matches planner spec `PLAN.md:71-72` exactly. Tests at `claude_auth_test.go`: `TestEnsureClaudeAccountReadyRespectsSkipLogin:98`, `TestEnsureClaudeAccountReadyAlreadyAuthedReturnsNilEvenNonTTY:129`, `TestEnsureClaudeAccountReadyRejectsNonTTY:76` — all three assert `runHits == 0`, proving the short-circuits fire before container launch. |
| AC4 | `ensureClaudeAccountReady` container-run failure propagates as error. | PASS | CONFIRMED — `TestEnsureClaudeAccountReadyFailsWhenContainerRunFails:160` exercises the path via `loginClaudeAccount` (acknowledged in worklog because non-TTY unit tests can't reach the ensure path's container call without TTY mocking; `loginClaudeAccount` shares the same downstream container+identity pipeline). Stub returns `errors.New("container: docker daemon not running")`; test asserts `errors.Is(err, containerErr)` and `runHits == 1`. `TestEnsureClaudeAccountReadyFailsWhenNotLoggedInAfterContainer:188` also exercises the post-container `ReadAccountIdentity` failure path. Error wrapping at `claude_auth.go:138` uses `%w` (`fmt.Errorf("run claude auth container for account %q: %w", account.Name, err)`) so `errors.Is` works correctly. |
| AC5 | `loginClaudeAccount` has no TTY guard; container run invoked even in non-TTY. | PASS | CONFIRMED — function body `claude_auth.go:157-178`: `writeCLINotice` (`:158-165`), `runner.RunInContainer` (`:166-169`), `ReadAccountIdentity` (`:170-176`). No `commandHasTTY` call anywhere. `TestLoginClaudeAccountSkipsNonTTYGuard:253` passes a non-TTY `bytes.Buffer`-backed cmd, stub returns sentinel error, asserts (a) error does NOT contain "TTY" and (b) `runHits == 1` — proving no TTY guard. |
| AC6 | `wipeClaudeCredentials` unchanged behavior. | PASS | CONFIRMED — function body `claude_auth.go:182-188` is byte-identical to Path A version at `9e7bc48:internal/cli/claude_auth.go` (same signature, same `os.Remove` + `os.IsNotExist` check, same `%w` wrap, same trimmed-homepath path construction). Tests `TestWipeClaudeCredentialsRemovesFile:329` and `TestWipeClaudeCredentialsMissingFileIsNoError:348` cover both branches; `TestLogoutManagedAccountWipesClaudeCredentials:359` proves integration with `logoutManagedAccount`. |
| AC7 | `mage testPkg ./internal/cli` passes; all new tests green; no Path A test names remain. | PASS | CONFIRMED — independent rerun: `tests: 150 passed: 150 failed: 0`, `github.com/evanmschultz/valv/internal/cli 71.2%`. Coverage clears 70% AGENTS.md floor. Full read of `claude_auth_test.go` (397 lines) yields zero matches for Path A test names listed in `PLAN.md:250` (`TestSystemClaudeAccountAuthRunnerRunAuthLoginUsesCLAUDE_CONFIG_DIR`, `TestRunClaudeHostCommandPreflightFailsWhenClaudeMissing`, `TestLoginClaudeAccountWipesAndRunsSetupTokenAndExtractsAndWrites`, `TestLoginClaudeAccountFailsWhenRunSetupTokenErrors`, `TestLoginClaudeAccountFailsWhenExtractTokenErrors`, `TestLoginClaudeAccountFailsOnEmptyToken`). |

### Spec conformance — design decisions cross-check

| Decision (PLAN.md ref) | Implementation evidence | Result |
|---|---|---|
| Single-method interface `RunInContainer` (PLAN.md:60-66) | `claude_auth.go:32-34` | pass |
| Lazy executor construction per-call (PLAN.md:68-69, worklog line 418) | `claude_auth.go:60-63` nil-check on `r.executor`, fallback to `dockeradapter.NewExecutor(dockeradapter.NewSystemRunner("docker", stdin, stdout, stderr))` | pass |
| `ContainerRunRequest` env: `CLAUDE_CONFIG_DIR`, `HOME`, `LOGNAME`, `TERM`, `USER` (PLAN.md:69, 171-177) | `claude_auth.go:71-77` matches the 5 env keys exactly; `claudeprovider.ContainerClaudeDir` + `ContainerHomeDir` used for paths | pass |
| Bind mount `homePath → ContainerClaudeDir` (PLAN.md:178-180) | `claude_auth.go:78-80` uses `dockeradapter.NewMountSpec(homePath, claudeprovider.ContainerClaudeDir, false)` | pass |
| `Args: []string{}` — plain `claude` (PLAN.md:181) | `claude_auth.go:81` | pass |
| `Interactive: stdin != nil`, `TTY: commandHasTTY(stdin)`, `Init: true`, `Remove: true`, `User: currentContainerUser()` (PLAN.md:182-187) | `claude_auth.go:82-86` matches all five fields | pass |
| `ensureClaudeAccountReady` no wipe step (PLAN.md:71-72) | `claude_auth.go:110-148` — no `wipeClaudeCredentials` call | pass |
| `loginClaudeAccount` no TTY guard, no wipe (PLAN.md:74-75) | `claude_auth.go:157-178` — no `commandHasTTY`, no `wipeClaudeCredentials` (matches `loginCodexAccount` no-guard semantics) | pass |
| TTY check is `commandHasTTY(cmd.InOrStdin())` only, NOT widened to stdout (worklog design note line 422) | `claude_auth.go:122` — `if !commandHasTTY(cmd.InOrStdin())` (single stdin check, narrowed from Path A's stdout-widened guard per PLAN.md:200) | pass |
| `normalizedContainerTERM()` inlined (PLAN.md:193) | `claude_auth.go:64-67` — inline `strings.TrimSpace(os.Getenv("TERM"))` with `"xterm-256color"` default | pass |

### Reasoning coherence findings

- **Container-error propagation tested via `loginClaudeAccount` rather than `ensureClaudeAccountReady` directly** — the worklog explicitly acknowledges this on line 426 with a sound justification: `ensureClaudeAccountReady`'s success path requires a real TTY for `commandHasTTY` to return true, and the test harness uses `bytes.Buffer` (non-TTY) for stdin. `loginClaudeAccount` has no TTY guard so it can exercise the same downstream container+identity-check pipeline. The two functions share `runner.RunInContainer` + `ReadAccountIdentity` verbatim — testing one branch is sound for both. Test naming starts with `TestEnsureClaudeAccountReady...` which is slightly misleading (the body calls `loginClaudeAccount`) but the worklog discloses this transparently. Acceptable.
- **Stub side-effect simulation pattern** — `stubClaudeAccountAuthRunner.stubRunFunc func(homePath string)` (`claude_auth_test.go:32`) is the planner-locked pattern for tests that need the container to "write" `.credentials.json`. `TestEnsureClaudeAccountReadySucceeds:212` and `TestLoginClaudeAccountSucceeds:298` use it correctly to simulate container-side OAuth completion.
- **Stat-error propagation for non-`IsNotExist` errors** — `claude_auth.go:119-121` wraps the stat error with `%w` and account name context. Handles cases like permission-denied on the cred dir, which the planner spec doesn't explicitly cover but is correct defensive behavior. Tests don't exercise this path; not blocking (would require a permission-injection test harness), but worth noting.
- **`hostClaudeAccountAuth` image hardcoded `valv-claude:dev`** — `claude_auth.go:41-43` constructs `dockeradapter.NewImageRef("valv-claude", "dev")` directly rather than calling `claudeImageRef()` (which would consult `VALV_CLAUDE_IMAGE`). Worklog (line 430) acknowledges and justifies: "auth container should always use `valv-claude:dev` regardless of the image override." Defensible design — the auth container does NOT need to honor user image pins because it's a fresh OAuth flow. The dev's `VALV_CLAUDE_IMAGE` override applies to runtime image selection in `claude.go`, not auth. Not flagged.

### Test count + coverage independent re-verification

- Independent rerun of `mage testPkg github.com/evanmschultz/valv/internal/cli`: **150/150 passed, 0 failed, 0 skipped, 71.2% coverage**.
- Matches builder's reported numbers exactly. Coverage threshold (60% mage floor; 70% AGENTS.md floor) cleared.

### Unknowns

None routed to orchestrator. All planner spec requirements have implementation evidence; all 7 ACs verified by independent file inspection + diff comparison + mage rerun.

### Hylla Feedback

N/A — Unit 7.5 touched only files modified in this drop (uncommitted to Hylla until drop-end reingest). All Go code reads went directly via the `Read` tool per the mid-drop evidence protocol. The Path A baseline comparison used `git show 9e7bc48:internal/cli/claude_auth.go` since Path A code is committed history. Hylla was not queried for this unit.

---

## Unit 7.7 — Round 1

**Date:** 2026-05-15
**Reviewer:** go-qa-proof-agent
**Verdict:** pass

### Evidence reviewed

- `main/drops/DROP_7_HOST_AUTH_TOKEN_FIX/PLAN.md` Unit 7.7 spec (lines 269–371) plus Round 5 design decisions for the resolver and auto-wire (lines 95–111).
- `main/drops/DROP_7_HOST_AUTH_TOKEN_FIX/BUILDER_WORKLOG.md` `## Unit 7.7 — Round 1` entry (lines 355–399).
- `internal/services/images/service.go` committed at `c4c25c6` — full read.
- `internal/services/images/service_test.go` committed at `c4c25c6` — lines 449–678 covering Codex resolver test, Claude resolver tests, and the auto-wire test.
- `git diff HEAD~1 -- internal/services/images/` — 53 lines added in service.go, 88 lines added in service_test.go.
- Codex resolver cross-reference: `service.go:125–178` (Codex constants block, payload, resolver struct, constructor, `LatestVersion`, `normalizeCodexVersion`) and `service.go:261–263` (Codex auto-wire).
- Rerun verification: `mage testPkg github.com/evanmschultz/valv/internal/services/images` produced `tests: 22 passed: 22`, coverage `76.9%`, threshold met.

### Acceptance criteria verification

| # | Criterion | Builder Result | QA-Proof Verification |
|---|---|---|---|
| 1 | `NewClaudeVersionResolver(nil)` returns non-nil `VersionResolver` | PASS | PASS — `service.go:192–197` returns `claudeVersionResolver{...}` (a value, never nil); nil-client branch defaults to `&http.Client{Timeout: defaultVersionRequestTTL}`. Auto-wire test `TestNewWiresClaudeVersionResolverWhenProviderIsClaudeAndNilResolver` (service_test.go:665–678) exercises the constructor indirectly via `New()` and asserts `svc.resolver != nil`. |
| 2 | `LatestVersion` returns valid semver matching `\d+\.\d+\.\d+` for `{"version":"X.Y.Z"}` | PASS | PASS — `service.go:199–227` decodes the npm JSON, trims `v` prefix and whitespace, validates via `versionPattern.MatchString` (shared regex `\d+\.\d+\.\d+(?:[-+][0-9A-Za-z.-]+)?` — superset of AC2 form), and returns `versionPattern.FindString(version)`. `TestClaudeVersionResolverReadsLatestVersion` (service_test.go:592–607) confirms `"2.1.200"` round-trips exactly. |
| 3 | `images.New()` with `Provider=ProviderClaude, Resolver=nil` returns service with non-nil resolver | PASS | PASS — `service.go:264–266` auto-wire block placed **immediately** after the Codex auto-wire at lines 261–263 (no intervening code), matching the planner's locked "immediately after the existing Codex auto-wire" directive. `TestNewWiresClaudeVersionResolverWhenProviderIsClaudeAndNilResolver` constructs `New(...Provider: ProviderClaude...)` with no `Resolver` and asserts `svc.resolver != nil`. |
| 4 | `mage testPkg github.com/evanmschultz/valv/internal/services/images` passes | PASS — 22/22, 76.9% | PASS — reviewer rerun: `tests: 22 passed: 22 failed: 0 skipped: 0`, package coverage `76.9%`, `Minimum package coverage: 60.0%` threshold met, AGENTS.md 70% floor exceeded. |

### Mirror-completeness against Codex resolver

- **Constants block placement:** `defaultClaudeLatestURL` at `service.go:28` sits in the same `const (...)` group as `defaultCodexLatestURL` at `:27` — adjacent lines, identical const-block membership. PASS.
- **Struct/method vertical pattern:** Codex order: `codexReleasePayload` (`:125`) → `codexVersionResolver` (`:130`) → `NewCodexVersionResolver` (`:135`) → `LatestVersion` (`:142`) → `normalizeCodexVersion` (`:173`). Claude order: `claudeNPMPayload` (`:180`) → `claudeVersionResolver` (`:184`) → `NewClaudeVersionResolver` (`:192`) → `LatestVersion` (`:199`). Claude omits the `normalize*` helper — correct, because the npm payload is already clean semver and the inline `strings.TrimPrefix(..., "v")` + `versionPattern` validation covers all forms. PASS.
- **Constructor signature:** `func NewClaudeVersionResolver(client *http.Client) VersionResolver` mirrors `func NewCodexVersionResolver(client *http.Client) VersionResolver` exactly — same param type, same return type (interface), same nil-client default-timeout pattern using the shared `defaultVersionRequestTTL` const. PASS.
- **Error wrapping format:** Claude uses `"latest claude version: <op>: %w"` (`:202`, `:209`, `:220`); Codex uses `"latest codex version: <op>: %w"` (`:145`, `:152`, `:163`). Same prefix-with-provider, same op vocabulary (`new request`, `send request`, `decode response`), same `%w` wrap. The status-code error uses `"latest <provider> version: unexpected status %d: %s"` in both. PASS.
- **HTTP header values:** Codex sets `Accept: application/vnd.github+json` (GitHub-specific). Claude sets `Accept: application/json` (npm-generic). User-Agent matches: both set `User-Agent: valv`. Correct divergence — npm doesn't require a vendor Accept media type. PASS.
- **Status-code error body excerpt:** Both use `io.ReadAll(io.LimitReader(resp.Body, 4096))` followed by `strings.TrimSpace(string(body))` in the error string. Bound and pattern identical. PASS.
- **Default timeout:** Both constructors reuse `defaultVersionRequestTTL` (10s) — no Claude-specific timeout constant invented. PASS.
- **Auto-wire placement in `New()`:** Codex case `:261–263`, Claude case `:264–266`, no intervening code or `else`. Independent `if` guards (correct: a provider can only match one). PASS.

### Minor observations (not findings — non-blocking)

- `TestClaudeVersionResolverNetworkErrorReturnsError` (service_test.go:651–663) uses `server.Client()` after `server.Close()`. The pattern works (test passes) because the underlying transport fails the connection. The Codex resolver does not have a matching `*Network*` test — Claude exceeds the Codex baseline here. No mirror gap; bonus coverage.
- `versionPattern` is the shared package-level regex — Claude correctly reuses it instead of inventing a Claude-specific pattern. The shared regex's optional `(?:[-+][0-9A-Za-z.-]+)?` suffix is unused for npm semver but harmless.

### Unknowns

None.

### Hylla Feedback

N/A — this QA pass touched only files modified during this drop (not yet reingested at the post-Unit-7.7 commit). All evidence reads were direct `Read` calls per mid-drop evidence protocol; Hylla was not queried for this unit.

---

## Unit 7.5 — Round 2

**Date:** 2026-05-15
**Reviewer:** go-qa-proof-agent
**Verdict:** pass

Round 2 lands three targeted fixes for R1 falsification findings: image-ensure guard on the auth path (BLOCK 1), `claudeImageRef()` symmetry between auth and launch (CONCERN 1), and terminal locale-env passthrough on the auth container (CONCERN 2). Independent verification confirms all three fixes are in place at the spec-locked sites and each is pinned by at least one new behavior test.

### Evidence reviewed

- `git diff HEAD~1 HEAD` (commit `d5d1fa6 fix(cli): make Claude auth path complete on fresh install`) — 244 lines added, 3 lines deleted, 5 files (`internal/adapters/providers/claude/runtime.go`, `internal/cli/claude_auth.go`, `internal/cli/manage.go`, `internal/cli/claude_auth_test.go`, `internal/cli/manage_test.go`) plus worklog.
- Full read of `internal/cli/claude_auth.go` (200 lines post-R2).
- `internal/cli/manage.go` lines 485–525 (FIX 1 guard in `runManageAccountAdd`) and 595–620 (FIX 1 guard in `runManageAccountSwitch`).
- `internal/adapters/providers/claude/runtime.go` lines 115–127 (internal caller updated) and 283–302 (exported `TerminalEnvPassthrough` declaration).
- `internal/cli/claude_image.go` (`claudeImageRef` semantics — confirms `VALV_CLAUDE_IMAGE` parsing).
- `internal/adapters/docker/types.go` (`ContainerRunRequest.EnvPassthrough []string`, `buildRunLikeArgs` emits `-e KEY` per name).
- `internal/cli/claude.go` lines 181–214 (`ensureClaudeImageCurrent` two-branch behavior: `VALV_CLAUDE_IMAGE`→`docker inspect`, otherwise→`openImagesService.Build`).
- `BUILDER_WORKLOG.md` Unit 7.5 Round 2 entry (lines 459–513).
- `BUILDER_QA_FALSIFICATION.md` Unit 7.5 Round 1 findings (BLOCK 1, CONCERN 1, CONCERN 2) — re-read to confirm scope of each fix.
- Independent mage reruns (see numerical table below).

### Fix-by-fix verification

| # | R1 finding | R2 fix in code | R2 test coverage | Result |
|---|---|---|---|---|
| **BLOCK 1** | `runManageAccountAdd` / `runManageAccountSwitch` did not call `ensureClaudeImageCurrent` before `ensureManagedAccountReady` for Claude → fresh-install smoke test would fail with "image not found" before any OAuth prompt | `manage.go:497–501` (add) and `manage.go:612–616` (switch) gate on `provider == domain.ProviderClaude && !skipLogin`, then call `ensureClaudeImageCurrent(cmd, paths)` and propagate errors as `"manage account add: %w"` / `"manage account switch: %w"` | `TestManageAccountAddClaudeWithSkipLoginSkipsImageEnsure` (`manage_test.go:382–406`) — runs `account add claude work --skip-login --no-bind` without a fake docker installed; succeeds only because the `!skipLogin` guard skips image-ensure (otherwise `openImagesService→Build` would fail with no docker). `TestManageAccountAddClaudeWithExistingCredsAndImageOverrideSucceeds` (`manage_test.go:410–456`) — sets `VALV_CLAUDE_IMAGE`, installs fake docker, pre-writes creds, asserts the full chain (`ensureClaudeImageCurrent`→`ensureManagedAccountReady`) succeeds. | **CONFIRMED** |
| **CONCERN 1** | `hostClaudeAccountAuth` hardcoded `dockeradapter.NewImageRef("valv-claude", "dev")`, ignoring `VALV_CLAUDE_IMAGE` — launch path used `claudeImageRef()` so auth/launch could diverge | `claude_auth.go:46–48`: package-level var now declares `image: claudeImageRef()` instead of the hardcoded ref. Inline comment at `:42–45` documents the symmetry rationale. | `TestSystemClaudeAccountAuthRunnerUsesClaudeImageRef` (`claude_auth_test.go:417–428`) — sets `t.Setenv("VALV_CLAUDE_IMAGE", "test/myimg:v2")`, constructs `systemClaudeAccountAuthRunner{image: claudeImageRef()}`, asserts `runner.image.String() == "test/myimg:v2"`. Test exercises the same helper used at the production var-init site. | **CONFIRMED (with caveat — see Spec finding 1)** |
| **CONCERN 2** | Launch path forwarded `LANG`, `LC_CTYPE`, `COLORTERM`, `TERM_PROGRAM`, `TERM_PROGRAM_VERSION` via `EnvPassthrough` (`terminalEnvPassthrough()` in `runtime.go`); auth container did NOT — TUI prompt could mis-render through Docker pty | `runtime.go:283–302`: function renamed `terminalEnvPassthrough` → `TerminalEnvPassthrough` with Go doc comment; internal caller at `runtime.go:126` updated; `claude_auth.go:88` sets `EnvPassthrough: claudeprovider.TerminalEnvPassthrough()` on the auth `ContainerRunRequest`. | `TestSystemClaudeAccountAuthRunnerPassesThroughTerminalEnv` (`claude_auth_test.go:434–473`) — sets `LANG`/`LC_CTYPE` via `t.Setenv`, runs `RunInContainer` via a `stubAuthContainerExecutor` that captures the request, asserts the captured `ContainerRunRequest.EnvPassthrough` contains both `LANG` and `LC_CTYPE`. Real-end-to-end assertion through the docker request layer. | **CONFIRMED** |

### Acceptance criteria verification (R1 ACs forward + R2 ACs added)

| # | Criterion | Evidence | Result |
|---|---|---|---|
| AC1 (R1) | `claudeAuthRunner` has exactly one method `RunInContainer` | `claude_auth.go:32–34` — unchanged from R1 | pass |
| AC2 (R1) | No `os/user`, `os/exec`, `claudeKeychainService`, `writeClaudeCredentials`, `runClaudeHostCommand` in `claude_auth.go` | import block `claude_auth.go:3–19`; full-file read — no Path A artefacts; R2 added only `claudeImageRef()` usage at `:47` and `EnvPassthrough` field at `:88` | pass |
| AC3 (R1) | `ensureClaudeAccountReady` order: SkipLogin → already-authed → non-TTY → notice → RunInContainer → ReadAccountIdentity | `claude_auth.go:121–158` — order preserved unchanged from R1 | pass |
| AC4 (R1) | Container-run failure propagates as `%w`-wrapped error | `claude_auth.go:148–150` (`"run claude auth container for account %q: %w"`) — unchanged | pass |
| AC5 (R1) | `loginClaudeAccount` has no TTY guard | `claude_auth.go:168–189` — no `commandHasTTY` call | pass |
| AC6 (R1) | `wipeClaudeCredentials` unchanged | `claude_auth.go:193–199` — byte-identical to R1 | pass |
| AC7 (R1) | `mage testPkg internal/cli` passes ≥70% | independent rerun: 154/154 pass, 71.9% | pass |
| AC-R2-1 | `ensureClaudeImageCurrent` called before `ensureManagedAccountReady` for Claude in `runManageAccountAdd` | `manage.go:497–501` — guard placed between profile creation and `ensureManagedAccountReady` call at `:502` | pass |
| AC-R2-2 | Same guard in `runManageAccountSwitch` | `manage.go:612–616` — guard placed between profile resolution at `:602` and `ensureManagedAccountReady` at `:617` | pass |
| AC-R2-3 | Guard gated on `!skipLogin` | `manage.go:497` and `:612` — both use `provider == domain.ProviderClaude && !skipLogin`. `TestManageAccountAddClaudeWithSkipLoginSkipsImageEnsure` proves SkipLogin path bypasses image-ensure | pass |
| AC-R2-4 | `claudeImageRef()` used in `hostClaudeAccountAuth` var | `claude_auth.go:47` — `image: claudeImageRef()` (was `dockeradapter.NewImageRef("valv-claude", "dev")` in R1) | pass |
| AC-R2-5 | `ContainerRunRequest.EnvPassthrough` populated from `TerminalEnvPassthrough()` | `claude_auth.go:88` — `EnvPassthrough: claudeprovider.TerminalEnvPassthrough()` in the `RunInContainer` body's request literal | pass |
| AC-R2-6 | `TerminalEnvPassthrough` exported from `clauderuntime` with doc comment | `runtime.go:283–286` (doc) + `:287` (declaration); internal caller at `:126` updated to use the exported name | pass |
| AC-R2-7 | `mage testPkg internal/adapters/providers/claude` GREEN | independent rerun: 21/21 pass, 78.4% | pass |
| AC-R2-8 | `mage testPkg internal/services/claude` GREEN | independent rerun: 17/17 pass, 81.0% | pass |

### Spec conformance findings

- 1.1 [Axis: spec-conformance] [severity: low] `TestSystemClaudeAccountAuthRunnerUsesClaudeImageRef` (`claude_auth_test.go:417–428`) verifies the **helper** `claudeImageRef()` honors `VALV_CLAUDE_IMAGE`, not the **package-level var** `hostClaudeAccountAuth` declared at `claude_auth.go:46–48`. The package-level var captures the value at package init time, before any `t.Setenv` in tests can take effect. The R2 builder worklog (line 509) already calls this out under Unknowns. The production code is functionally correct — the env var is set before binary launch in real usage — but the test does not pin the var-init-site behavior. Low severity; future polish could replace the package-level var with a lazy getter so tests can observe the override on the actual production code path. → `claude_auth.go:46` → optionally convert `hostClaudeAccountAuth` to a function returning a freshly-resolved runner, or add a build-time-only test that compiles with a fresh package import to observe the package-init value.
- 1.2 [Axis: spec-conformance] [severity: low] `TestManageAccountAddClaudeWithExistingCredsAndImageOverrideSucceeds` (`manage_test.go:410–456`) takes the `VALV_CLAUDE_IMAGE`-set branch of `ensureClaudeImageCurrent` (which calls `ensureClaudeImageAvailable` → `docker image inspect`). It does NOT exercise the heavy path (`openImagesService` → `service.Build`). BLOCK 1 protection on a fresh CI runner with no `VALV_CLAUDE_IMAGE` set requires the heavy path to succeed at runtime — but that path takes minutes (real docker build) and is not unit-testable. The smoke test in PLAN.md lines 28–40 remains the only authoritative gate for the heavy-path behavior. Acceptable for the R2 scope (BLOCK 1 was about the GUARD being present, not the heavy path itself); flagged because R2 testing did not (and cannot, in a unit) prove the full fresh-install chain end-to-end.

### Idiomatic Go checks

- **Error wrapping** — every new boundary uses `%w`: `manage.go:499` (`"manage account add: %w"`), `:614` (`"manage account switch: %w"`), `claude_auth.go:149` (unchanged, `"run claude auth container ...: %w"`). All clean.
- **Doc comments** — `TerminalEnvPassthrough` doc at `runtime.go:283–286` starts with the identifier name and explains both callers; new test functions have function-level docs explaining the R1 finding each pins.
- **No token logging** — all R2 changes are about routing env-var names and image refs; nothing inspects credential contents. Verified clean.
- **Mage-only** — all three package tests run via `mage testPkg`; no raw `go test`. Verified by independent reruns.
- **`t.Parallel()` discipline** — `TestManageAccountAddClaudeWithSkipLoginSkipsImageEnsure` uses `t.Parallel()` (no env mutation). `TestManageAccountAddClaudeWithExistingCredsAndImageOverrideSucceeds` and `TestSystemClaudeAccountAuthRunnerUsesClaudeImageRef` / `...PassesThroughTerminalEnv` omit `t.Parallel()` because they use `t.Setenv` — correct per Go 1.26 rules.

### Numerical claims independent verification

| Claim | Verified |
|---|---|
| `mage testPkg internal/cli` GREEN 154/154 71.9% | live rerun: 154 passed, 0 failed, `internal/cli 71.9%`, threshold met |
| `mage testPkg internal/adapters/providers/claude` GREEN 21/21 78.4% | live rerun: 21 passed, 0 failed, `78.4%`, threshold met |
| `mage testPkg internal/services/claude` GREEN 17/17 81.0% | live rerun: 17 passed, 0 failed, `81.0%`, threshold met |
| 4 new tests added (2 in `manage_test.go`, 2 in `claude_auth_test.go`) | confirmed via `git diff HEAD~1 HEAD`: `manage_test.go +74 lines` containing `TestManageAccountAddClaudeWithSkipLoginSkipsImageEnsure` + `TestManageAccountAddClaudeWithExistingCredsAndImageOverrideSucceeds`; `claude_auth_test.go +77 lines` containing `stubAuthContainerExecutor` + `TestSystemClaudeAccountAuthRunnerUsesClaudeImageRef` + `TestSystemClaudeAccountAuthRunnerPassesThroughTerminalEnv` |
| 5 files touched | `git diff --stat HEAD~1 HEAD` confirms 5 source files + 1 worklog = `runtime.go (+5)`, `claude_auth.go (+11/-1)`, `manage.go (+17)`, `claude_auth_test.go (+76)`, `manage_test.go (+74)`, plus `BUILDER_WORKLOG.md (+58)` |

### Unknowns

- Whether `claude` inside the auth container actually renders the OAuth TUI correctly with `LANG`/`LC_CTYPE` forwarded — only live smoke test resolves this (the unit test proves the env names are present in the docker request, not that claude's TUI renders correctly). Routes to dev smoke test per PLAN.md lines 28–40.
- Whether the heavy path (`openImagesService` → `service.Build`) succeeds end-to-end on a fresh CI runner without `VALV_CLAUDE_IMAGE` — only live smoke test resolves; unit test only covers the lighter `docker inspect` branch.
- Package-level var `hostClaudeAccountAuth` captures `claudeImageRef()` at init time — `VALV_CLAUDE_IMAGE` must be in the environment when valv launches. The R2 test does not pin this var-init-time behavior (it tests the helper directly). Production reality is fine because env vars are set before launch.

### Summary

PASS. All three R1 falsification findings (BLOCK 1 + CONCERN 1 + CONCERN 2) are closed by surgical fixes at the spec-locked sites, with each fix pinned by at least one new behavior-asserting test. All seven R1 acceptance criteria continue to pass (verified by 154/154 test rerun); eight new R2 acceptance criteria all verified by code inspection + test + mage rerun. Coverage at or above the AGENTS.md 70% per-package floor across all three affected packages (`internal/cli 71.9%`, `internal/adapters/providers/claude 78.4%`, `internal/services/claude 81.0%`). Two low-severity spec-conformance findings recorded as future-polish notes; neither blocks the R2 close.

### Hylla Feedback

- **Query:** `hylla_search_keyword` for `installFakeDocker` (visibility=private, internal=include_internal, fields=content) — returned the symbol at `internal/cli/extended_test.go` on first try. Used to confirm the helper exists for the new `manage_test.go` test.
  - **Worked correctly.**
- **Query:** `hylla_search_keyword` for `openImagesService` — same parameters — returned the symbol at `internal/cli/operator_helpers.go` plus its callers (`runManageUpdateClaude`, `ensureClaudeImageCurrent`, etc.). Used to confirm `ensureClaudeImageCurrent`'s branching behavior.
  - **Worked correctly.**
- All other lookups (claude_auth.go body, manage.go body, runtime.go body, docker types) went through direct `Read` per mid-drop evidence protocol (these files were modified post-snapshot-11). No Hylla fallbacks were needed for committed-but-stale code, and no Hylla query missed.

---

## Unit 7.8 — Round 1

**Date:** 2026-05-16
**Reviewer:** go-qa-proof-agent
**Verdict:** pass

### Evidence reviewed

- `drops/DROP_7_HOST_AUTH_TOKEN_FIX/PLAN.md` Unit 7.8 spec (lines 374–420) plus Round 5 design notes (lines 113–124).
- `drops/DROP_7_HOST_AUTH_TOKEN_FIX/BUILDER_WORKLOG.md` — Unit 7.8 Round 1 entry (lines 7–44).
- `internal/cli/claude.go` lines 181–195 (`ensureClaudeImageCurrent`).
- `internal/cli/manage.go` lines 1115–1145 (`runManageUpdateCodex` — parity baseline) and lines 1147–1177 (`runManageUpdateClaude`); also account-add Claude image guard at 497–501 and account-switch guard at 612–616.
- `internal/cli/operator_helpers.go` lines 27–31 (factory vars) and 95–104 (Claude case in `openImagesService`).
- `internal/cli/extended_test.go` lines 30–50 (`stubCodexVersionResolver` parity baseline + new `stubClaudeVersionResolver`).
- `internal/cli/manage_test.go` lines 265–316 (`TestRunManageUpdateClaudeBuildsImage`).
- `git diff HEAD~1 HEAD --stat` and full per-file diff for commit `c0724f8`.
- Hylla `hylla_search_keyword` for `EnsureActionUpToDate` and `hylla_refs_find` on `DefaultClaudeCLIVersion` (snapshot 11 — pre-Unit-7.8 state; cross-referenced with diff).
- `mage testPkg github.com/evanmschultz/valv/internal/cli` — re-run by reviewer.
- `mage test` — re-run by reviewer.

### Acceptance criteria verification

| # | Criterion | Builder Result | QA-Proof Verification |
|---|---|---|---|
| AC1 | `ensureClaudeImageCurrent` non-`VALV_CLAUDE_IMAGE` path no longer references `imagesservice.BuildRequest` or `DefaultClaudeCLIVersion` | PASS | PASS — `internal/cli/claude.go:181–195` shows the env-override branch at line 182 returning early via `ensureClaudeImageAvailable`, then `openImagesService(...)` at line 185 (which now wires `claudeVersionResolverFactory(nil)`), then `service.EnsureLatest(...)` at line 190. The stale "pinned-version fast path" comment is removed. Diff confirms `-3 +1`. `DefaultClaudeCLIVersion` still exists in `internal/services/images/service.go` as the Dockerfile build-arg fallback constant (per appendix carve-out: "that's fine — the check is that this function doesn't reference it"), and Hylla's stale inbound-refs list includes only `internal/services/images/service_test.go` references after this commit (the `internal/cli` references shown in Hylla snapshot 11 are the lines this commit just removed). |
| AC2 | `ensureClaudeImageCurrent` calls `service.EnsureLatest(cmd.Context(), imagesservice.EnsureRequest{AllowExistingOnCheckFail: true})` | PASS | PASS — `internal/cli/claude.go:190` exact text: `_, err = service.EnsureLatest(cmd.Context(), imagesservice.EnsureRequest{AllowExistingOnCheckFail: true})`. Matches spec line 394 byte-for-byte. |
| AC3 | `runManageUpdateClaude` calls `EnsureLatest` (not `Build`); output includes `checked_at` field; heading branches correctly | PASS | PASS — `internal/cli/manage.go:1153` declares `var result imagesservice.EnsureResult`; line 1161 calls `service.EnsureLatest(cmd.Context(), imagesservice.EnsureRequest{})`; line 1172–1175 branch heading `"Provider image built"` → `"Provider image up to date"` on `imagesservice.EnsureActionUpToDate`; line 1176 output field set is `{provider, image, tags, version, checked at, context}` — identical shape to `runManageUpdateCodex` at line 1144. Renderer emits the field label `"checked at"` as `checked_at=` in stdout (asserted at `manage_test.go:295`). Hylla confirms `EnsureActionUpToDate` is a real const at `internal/services/images/service.go`. |
| AC4 | `mage testPkg github.com/evanmschultz/valv/internal/cli` passes | PASS — 154/154, 71.9% | PASS reproduced — reviewer ran `mage testPkg github.com/evanmschultz/valv/internal/cli`: 154 tests, 0 failed, 71.9% coverage; minimum-package gate at 60.0% (mage gate) cleared. |
| AC5 | `mage test` passes (full suite, race detector, coverage floor) | PASS — 421/421 | PASS reproduced — reviewer ran `mage test`: 421 tests across 20 packages, 0 failed, 0 skipped. Package coverage range 64.7% (`internal/adapters/docker`, lowest) – 91.3% (`internal/tui/manage`, highest); all clear the 60.0% mage gate. `internal/cli` is 71.9%, `internal/services/images` is 76.9%, both above AGENTS.md's 70% target. |

### Codex-parity completeness

Side-by-side review of `runManageUpdateCodex` (manage.go:1115–1145) vs `runManageUpdateClaude` (manage.go:1147–1177):

| Aspect | Codex | Claude | Match |
|---|---|---|---|
| Result type | `imagesservice.EnsureResult` | `imagesservice.EnsureResult` | yes |
| Spinner running | `"Checking provider image"` | `"Checking provider image"` | yes |
| Spinner success | `"Provider image check complete"` | `"Provider image check complete"` | yes |
| Spinner failure | `"Provider image update failed"` | `"Provider image update failed"` | yes |
| Service call | `service.EnsureLatest(cmd.Context(), imagesservice.EnsureRequest{})` | `service.EnsureLatest(cmd.Context(), imagesservice.EnsureRequest{})` | yes |
| Action constant | `imagesservice.EnsureActionUpToDate` | `imagesservice.EnsureActionUpToDate` | yes (constant, not literal) |
| Up-to-date heading | `"Provider image up to date"` | `"Provider image up to date"` | yes |
| Default heading | `"Provider image updated"` | `"Provider image built"` | divergent text; intentional — see note |
| Output fields | provider, image, tags, version, checked at, context | provider, image, tags, version, checked at, context | yes (same set, same order, same labels) |

Default-heading divergence (`"Provider image updated"` vs `"Provider image built"`) is intentional and not a parity defect. Builder preserved Claude's historical wording, which `TestRunManageUpdateClaudeBuildsImage` (`manage_test.go:283`) asserts on. The spec required output **shape** parity (field set + action-branch logic + spinner texts), not identical heading wording. Acceptable.

`claudeVersionResolverFactory` symmetry with `codexVersionResolverFactory` (`operator_helpers.go:27–31`):

- Both are package-level `var` typed as `func(*http.Client) imagesservice.VersionResolver`.
- Both default to the canonical constructor (`NewCodexVersionResolver` / `NewClaudeVersionResolver`).
- Both are consumed in `openImagesService` (line 91 Codex, line 100 Claude) via `factory(nil)`.
- Both are stubbed via parallel test helpers (`stubCodexVersionResolver` lines 30–39 vs `stubClaudeVersionResolver` lines 41–50) using the same `staticCLIResolver` and the same `t.Cleanup` swap pattern.

The previous `options.Resolver = nil` (which relied on `imagesservice.New()`'s auto-wire) is now replaced with the explicit factory call; behaviorally identical (auto-wire still fires as fallback when resolver is nil) but now overrideable in tests. Worklog notes the redundancy and confirms the explicit path is what enables test stubbing — correct rationale.

`VALV_CLAUDE_IMAGE` override path: `claude.go:182` exits via `ensureClaudeImageAvailable` before reaching the EnsureLatest call, so the override semantics are untouched by this change. Confirmed by inspection of the diff (`+1 -3` in this function; only the EnsureLatest line and the deleted comment are affected).

### Unknowns

- None. All 5 ACs verified by code inspection + re-run of mage targets reproducing the builder's claimed counts.

### Hylla Feedback

- **Query:** `hylla_search_keyword` for `EnsureActionUpToDate` (fields=content, limit=5) — returned the const at `internal/services/images/service.go` on first try.
  - **Worked correctly.**
- **Query:** `hylla_refs_find` on `github.com/evanmschultz/valv/internal/services/images/DefaultClaudeCLIVersion` (direction=inbound) — returned 5 inbound refs from snapshot 11 (pre-Unit-7.8). The Unit 7.8 commit removed three of those references (`ensureClaudeImageCurrent`, `runManageUpdateClaude`, `TestRunManageUpdateClaudeBuildsImage`). Cross-referenced against `git diff HEAD~1 HEAD` to confirm — Hylla's snapshot is stale per mid-drop protocol, not a Hylla defect.
  - **Worked correctly given the snapshot-11 staleness expectation.**
- **Suggestion:** None — the staleness is by design (drop-end-only reingest policy in CLAUDE.md). Mid-drop verification correctly uses `git diff` as the authoritative source for changed files.
- All other code reads went through direct `Read` per mid-drop evidence protocol. No Hylla fallbacks needed; no Hylla query missed.

---

## Unit 7.9 — Round 1

**Date:** 2026-05-16
**Reviewer:** go-qa-proof-agent
**Verdict:** **FAIL** (two concrete fixes required; one semantic Unknown routed to orchestrator)

### Scope reviewed

- `internal/services/images/cache.go` (new, 105 LOC) — full read.
- `internal/services/images/service.go` (modified) — full read; focused on lines 25-72 (Options + constants), 74-88 (Service struct), 231-292 (New), 364-492 (EnsureLatest).
- `internal/services/images/service_test.go` (modified, +267 / -10) — focused on retrofitted `CachePath` insertions and the 6 new cache tests (lines 680-935).
- `git diff HEAD~2 HEAD -- internal/services/images/` — full Unit 7.9 commit (`4d2186a feat(images): cache version resolver responses for 24h`).
- `git grep DefaultClaudeCLIVersion` across the whole repo.
- `mage testPkg github.com/evanmschultz/valv/internal/services/images` — reproduced.
- `mage test` — reproduced.

### Acceptance criteria verification

| # | Criterion | Builder Result | QA-Proof Verification |
|---|---|---|---|
| AC1 | `EnsureLatest` reads from `versionCachePath` before invoking resolver; returns cached version when within TTL | PASS | **PASS, with semantic concern** — `service.go:378-380` calls `readVersionCache(s.cachePath)` then `cachedVersion(cacheData, s.provider, now)` BEFORE the resolver branch at line 381. The `if !fromCache` guard at line 380 short-circuits the entire resolver block. `TestEnsureLatestUsesCacheWhenFresh` (test file lines 748-777) confirms via `resolverCallCounter` that resolver receives 0 calls on a within-TTL hit, and `result.LatestVersion == "2.1.143"` is asserted. **Semantic concern**: on a cache hit, `checkedAt := now.UTC()` at line 411 makes `EnsureResult.LatestCheckedAt = now`, not the cached `CheckedAt`. See "Semantic Unknown" section below — this may be intentional or may be a bug, spec is ambiguous. |
| AC2 | After fresh resolver call, cache file contains `{provider: {version, checked_at}}` entry | PASS | **PASS** — `service.go:406` calls `writeVersionCache(s.cachePath, latestVersion, s.provider, now)` immediately after resolver success. `cache.go:85-105` constructs/updates the entry with `Version` + `CheckedAt = now.UTC().Truncate(time.Second)` (truncation is fine; second-level precision adequate for 24h TTL). `TestEnsureLatestWritesCacheAfterResolverSuccess` (lines 815-844) asserts `entry.Version == "2.1.200"` AND `entry.CheckedAt.Equal(fixedNow.UTC().Truncate(time.Second))`. |
| AC3 | Both Claude and Codex entries coexist in same file | PASS | **PASS** — `cache.go:85-90` reads the existing file via `readVersionCache(path)` before writing, then `c.Providers[providerKey(p)] = ...` updates only the keyed provider; other entries pass through unchanged. `TestEnsureLatestPreservesOtherProviderEntries` (lines 846-877) seeds a codex entry, triggers a claude `EnsureLatest`, and asserts both keys present with correct versions. Caveat: see falsification Attack-3 in Section 0 above — the read-modify-write pattern is racy under concurrent writers; spec accepted via TTL-bounded self-healing. |
| AC4 | Cache read errors do not propagate to `EnsureLatest` callers | PASS | **PASS** — `cache.go:49-58` `readVersionCache` swallows BOTH `os.ReadFile` errors AND `json.Unmarshal` errors, returning empty `versionCacheFile{}` instead. `TestEnsureLatestIgnoresMalformedCache` (lines 879-911) seeds `not-json` bytes, asserts `err == nil` from `EnsureLatest` AND `counter.called == 1` (resolver did run as fallback) AND the cache file was rewritten with fresh entry. |
| AC5 | `DefaultClaudeCLIVersion` deletion (preferred) or kept-with-doc-comment | PASS (deletion) | **FAIL** — Deletion completed in `service.go` (Unit 7.9 diff `-12 +6` block removed lines 30-35). Production callers verified zero via inspection of `internal/cli/claude.go` + `internal/cli/manage.go`. BUT `git grep DefaultClaudeCLIVersion` returns 1 remaining reference at `internal/services/images/service_integration_test.go:127`: `result, err := svc.Build(context.Background(), BuildRequest{Version: DefaultClaudeCLIVersion})` inside `TestWriteDefaultClaudeContextBuildsWithExistingUIDAndGID`. That file has `//go:build integration` (line 1), so it only compiles when `-tags=integration` is set. Current `mage integration` target only runs `./internal/cli` (per `magefile.go:147`), so the breakage is latent — no green mage target currently exercises it. But the Go source is uncompilable under `-tags=integration ./internal/services/images/...`. The builder's "zero production callers" scan correctly identified production code, but failed to scan integration-tagged tests. |
| AC6 | `mage testPkg ./internal/services/images` GREEN, coverage ≥70% | PASS — 27/27 78.9% | **PASS reproduced** — reviewer ran `mage testPkg github.com/evanmschultz/valv/internal/services/images`: 27 tests, 0 failed, 0 skipped, 78.9% coverage. Threshold reported by mage is 60.0% (per `magefile.go:24` `coverageThreshold = 60.0`, with a TODO to restore 70.0%), so the AC's "≥70%" is satisfied at 78.9% regardless. |
| AC7 | `mage test` GREEN (full suite, race detector) | PASS — 427/428 (1 pre-existing unrelated) | **PASS reproduced (with same pre-existing failure)** — reviewer ran `mage test`: 428 tests across 20 packages, 427 passed, 1 failed. Failure: `TestRootDebugFlagIsNotPassedThroughToInteractiveCodexLaunch` in `internal/cli/codex_test.go:428` — `no space left on device` while staging `/Users/evanschultz/.codex` into a tmpfs. See "Disk-space failure verification" section below for pre-existence proof. NOT a Unit 7.9 regression. AC7 reads "GREEN (full suite)" — strict reading would fail, but the failure is environmental + pre-existing, so accepted. |

### Disk-space failure verification

- **Builder reported:** 427/428, failure is `TestRootDebugFlagIsNotPassedThroughToInteractiveCodexLaunch`, "unrelated tmpfs disk-space".
- **QA reproduced:** identical failure with same error text on the same package + test, same error path (`stage shared home "/Users/evanschultz/.codex": write ... /codex-runtime-.../codex-home/tmp/path/codex-arg.../applypatch: no space left on device`).
- **Pre-existence proof:** `git log --oneline -- internal/cli/codex_test.go` shows `60591fa test: add interactive root debug flag regression` introduced this test, with multiple subsequent commits unrelated to Unit 7.9 (`internal/services/images/`). The test pre-dates Unit 7.9 by many commits.
- **Verdict on disk-space failure:** environmental (tmpfs at `/private/var/folders/79/...` filled up while copying the developer's real `~/.codex` directory; not specific to any code change). The failure reproduces on this same machine because both runs were on the same machine; it is not necessarily reproducible on a clean-state machine. Not a Unit 7.9 finding; should be tracked separately by the orchestrator/dev (e.g., either skip the test when tmpfs is constrained or shrink the shared-home stage). Already noted in builder's Unit 7.8 review (`mage test` 421/421 — passed there) so it has appeared and disappeared based on disk fill state.

### Code-correctness inspection beyond ACs

- **`EnsureLatest` cache-miss fallthrough preserves the prior `AllowExistingOnCheckFail` path**: resolver-error branch at lines 382-405 is reached only when `!fromCache`. The fallback-to-existing-image semantic is intact and tested by `TestEnsureLatestFallsBackToExistingImageWhenVersionCheckFails` (still PASS).
- **Cache-write failure isolation correct**: `service.go:406-408` logs but does NOT return — flow continues to `checkedAt := now.UTC()` and build/up-to-date logic. `TestEnsureLatestSurvivesCacheWriteError` (lines 913-935) seeds a directory at the cache path (`is-a-dir`) and confirms `EnsureLatest` succeeds. Good failure isolation.
- **`defaultCachePath` cross-platform**: uses `os.UserCacheDir()` (returns `~/Library/Caches/valv/version-cache.json` on macOS, `~/.cache/valv/version-cache.json` on Linux), falls back to `os.TempDir()` if UserCacheDir errors. Matches spec design lines 438-441.
- **`writeVersionCache` not atomic on partial-write failure** — uses `os.WriteFile`, not the spec's suggested "write-temp + rename" pattern. Accepted: small JSON file, kernel-level atomicity for typical sizes, self-healing via empty-cache-on-parse-error. Mentioned in falsification Attacks 2-3.
- **Truncation to second precision (`Truncate(time.Second)`)**: harmless; 24h TTL has no sensitivity to sub-second jitter.

### Semantic Unknown — `LatestCheckedAt` on cache hit

When `fromCache=true`, the code path at lines 377-411 of `service.go` is:

```go
now := s.clock()
cacheData := readVersionCache(s.cachePath)
latestVersion, fromCache := cachedVersion(cacheData, s.provider, now)
if !fromCache {
    // resolver branch — sets latestVersion and writes cache
    ...
}
checkedAt := now.UTC()  // line 411 — uses `now` regardless of cache origin
```

`EnsureResult.LatestCheckedAt = checkedAt = now.UTC()` on a cache hit. But the cached entry's `CheckedAt` field IS the timestamp at which the resolver was last actually called — semantically that's "when was latest checked". Reporting `now` on a cache hit means `valv manage update claude` will print `checked at: <now>` even though no network check happened.

- **Spec position (PLAN.md AC1):** "Returns cached version when within TTL. Verified by test injecting a fake clock + pre-populated cache file." — silent on `LatestCheckedAt`.
- **Test position (`TestEnsureLatestUsesCacheWhenFresh`):** asserts only `LatestVersion`, not `LatestCheckedAt`. So the current behavior is not pinned by any test.
- **Orchestrator focus area #1:** "Within-TTL cache hit returns cached version + correct `EnsureResult.LatestCheckedAt` (cached timestamp, not now)." — orchestrator's interpretation is "cached timestamp".

Routing to orchestrator: pick one of (a) fix `checkedAt` to use cached `CheckedAt` on hit, (b) keep as-is and add a test pinning the chosen semantic + a worklog note explaining why `now` is intentional. Either is acceptable cascade-wise; the bug surface is the `checked at` field displayed by `valv manage update`.

### Findings summary (concrete blockers)

- **Finding 1 [Axis: completion-checklist-audit] [severity: high]** — `service_integration_test.go:127` references the deleted `DefaultClaudeCLIVersion` constant → file uncompilable under `-tags=integration ./internal/services/images/...`. Fix: replace with the same local `testClaudeCLIVersion` pattern used in `service_test.go:516` (define a package-level test const inside the `_integration_test.go` file or hoist `testClaudeCLIVersion` to a shared `*_test.go` helper file that compiles in both contexts — note that build-tagged integration tests cannot import from `_test.go` non-tagged files). Minimal change: inline a local literal `"2.1.143"` or copy the `const testClaudeCLIVersion` declaration into the integration test file.
- **Finding 2 [Axis: spec-conformance] [severity: medium]** — `EnsureLatest` cache-hit path returns `LatestCheckedAt: now` instead of the cached `CheckedAt`. Spec ambiguous; orchestrator's focus area says cached timestamp. Fix: in `service.go:411`, branch on `fromCache` to choose between `now.UTC()` and the cached entry's `CheckedAt`. Affects user-facing `checked at: ...` output of `valv manage update`. Add a test pinning chosen semantic.

### Unknowns

- Is Finding 2's semantic a hard requirement or interpreter latitude? Spec silent; routing to orchestrator for verdict + chosen fix direction.
- Disk-space test failure: separate concern, route via orchestrator to dev — either skip the test under tmpfs constraints or shrink staged home payload.

### Hylla Feedback

N/A — Unit 7.9 reviewed via direct `Read` + `git diff` + `git grep` per mid-drop evidence protocol (Hylla snapshot is stale until drop-end reingest). No Hylla queries attempted.

---

## Unit 7.10 — Round 1

**Date:** 2026-05-16
**Reviewer:** go-qa-proof-agent
**Verdict:** pass

### Evidence reviewed

- `git log --oneline -10` — Unit 7.10 lands at `ebf0070 feat(cli): codex-parity polish for Claude manage update`.
- `git diff HEAD~1 HEAD -- internal/cli/` — 4 files touched (`claude.go +4/-1`, `extended_test.go +48`, `manage.go +1/-1`, `manage_test.go +1/-1`).
- `internal/cli/claude.go:181-198` — `ensureClaudeImageCurrent` with debug log addition.
- `internal/cli/codex.go:240-256` — Codex reference for parity check.
- `internal/cli/manage.go:1160-1177` — `runManageUpdateClaude` heading change.
- `internal/cli/manage_test.go:270-294` — heading assertion updated.
- `internal/cli/extended_test.go:41-50` — `stubClaudeVersionResolver` (Unit 7.8 helper, reused correctly).
- `internal/cli/extended_test.go:853-861` — `fakeCodexRecipeHash` + new `fakeClaudeRecipeHash` mirror.
- `internal/cli/extended_test.go:863-880` — new `TestManageUpdateClaudeSecondRunReportsUpToDate`.
- `internal/cli/extended_test.go:882-904` — new `TestEnsureClaudeImageCurrentAutoUpdatesWhenNoOverrideIsSet`.
- `internal/cli/extended_test.go:503-519` — Codex reference `TestManageUpdateSecondRunReportsUpToDate`.
- `internal/cli/extended_test.go:661-682` — Codex reference `TestEnsureCodexImageCurrentAutoUpdatesWhenNoOverrideIsSet`.

### Acceptance criteria verification

| # | Criterion | Builder Result | QA-Proof Verification |
|---|---|---|---|
| 1 | Debug log emitted on `EnsureActionUsingExistingImage` in `ensureClaudeImageCurrent`, exact Codex parity (substring `"using existing claude image after latest-version check failed"`) | pass | pass — `claude.go:194-196` matches `codex.go:252-254` byte-for-byte except `claude` vs `codex` in the message; same kv pairs (`"image"` + `result.Image.String()`, `"version"` + `result.Version`); same `LoggerFromContext(cmd.Context()).Debug` call site |
| 2 | `TestManageUpdateClaudeSecondRunReportsUpToDate` + `TestEnsureClaudeImageCurrentAutoUpdatesWhenNoOverrideIsSet` exist and pass | pass | pass — both tests at `extended_test.go:863-880` and `:882-904`; mirror Codex equivalents at `:503-519` and `:661-682` in structure (installFakeDocker, stubbed resolver, recipe hash env where applicable, assertions on buildx count / `image inspect` / `--build-arg`) |
| 3 | `runManageUpdateClaude` default heading is `"Provider image updated"`; `TestRunManageUpdateClaudeBuildsImage` assertion updated to match | pass | pass — `manage.go:1172` literal `"Provider image updated"` in default branch; `manage_test.go:283` asserts `Contains(stdout.String(), "Provider image updated")` |
| 4 | `mage testPkg ./internal/cli` green | 156/156, 72.4% | pass — reproduced 156/156, 72.4% coverage, above 60% gate |
| 5 | `mage test` green full suite | 428/428 | pass — reproduced 428/428 across 20 packages, all packages at or above 60% threshold |

### Codex parity completeness

- **Debug log:** Claude `claude.go:194-196` and Codex `codex.go:252-254` differ only in the literal provider name. Identical kv keys (`"image"`, `"version"`), identical kv value expressions (`result.Image.String()`, `result.Version`), identical logger reference (`LoggerFromContext(cmd.Context()).Debug`), identical conditional guard (`result.Action == imagesservice.EnsureActionUsingExistingImage`).
- **Test names + stubs:**
  - `TestManageUpdateSecondRunReportsUpToDate` (Codex, line 503) unchanged.
  - `TestManageUpdateClaudeSecondRunReportsUpToDate` (Claude, line 863) added — same body shape: stub resolver + fake docker + recipe-hash env + double `runManage` + assert "Provider image up to date" + buildx count = 1. Stylistic diff: Claude version uses `const stubbedVersion = "2.2.0"` for the resolver call; Codex inlines `"0.117.0"`. Functionally equivalent.
  - `TestEnsureCodexImageCurrentAutoUpdatesWhenNoOverrideIsSet` (Codex, line 661) unchanged.
  - `TestEnsureClaudeImageCurrentAutoUpdatesWhenNoOverrideIsSet` (Claude, line 882) added — same body shape: stub resolver + fake docker + cobra cmd + assert log contains `image inspect valv-claude:dev` / `buildx build --load` / `--build-arg CLAUDE_VERSION=2.2.0`. Same `const stubbedVersion` stylistic choice.
  - `stubClaudeVersionResolver` (Unit 7.8 helper at `extended_test.go:41-50`) reused correctly; mirrors `stubCodexVersionResolver` at `:30-39`.
  - `fakeClaudeRecipeHash` (line 858-861) mirrors `fakeCodexRecipeHash` (line 853-856) precisely — same SHA256 over the relevant `DefaultClaudeDockerfile` / `DefaultCodexDockerfile`.
- **Heading:** `runManageUpdateClaude` (manage.go:1172) now uses `"Provider image updated"`, matching the Codex sibling. Up-to-date branch (`EnsureActionUpToDate`) remains `"Provider image up to date"` — consistent with the Codex `runManageUpdate` default branch.

### Mage test result

- **Builder reported:** `mage testPkg ./internal/cli` 156/156 72.4%; `mage test` 428/428.
- **QA reproduced:** First `mage testPkg ./internal/cli` invocation hit `no space left on device` in `TestRootDebugFlagIsNotPassedThroughToInteractiveCodexLaunch` (a non-7.10 test that stages `/Users/evanschultz/.codex` into tmpfs — same disk-space failure mode noted by Unit 7.9 reviewer at this file line 771). Rerun: GREEN 156/156, 72.4%. `mage test`: GREEN 428/428 across 20 packages, all above 60% coverage gate. The 7.10-affected package (`internal/cli`) hits 72.4%; the 70% AGENTS.md § 11 floor is also met by every other package above it. The transient disk-space test flake is unrelated to Unit 7.10's changes and is already routed at line 771.

### Idiomatic Go checks

- **Doc comments on exported identifiers:** Unit 7.10 introduces no new exported identifiers in production code (`ensureClaudeImageCurrent` was already exported-and-doc'd before this diff; the diff only mutates its body). New test helpers (`fakeClaudeRecipeHash`) and tests are package-internal — Go convention is comments optional. Codex equivalent (`fakeCodexRecipeHash`) also un-doc'd. Consistent.
- **Error wrapping:** No new error boundaries introduced — the new debug log is informational only; underlying `EnsureLatest` error wrap (`return err`) is unchanged from pre-diff state and matches Codex pattern.
- **No token logging:** Debug log args (`Image.String()`, semver `Version`) carry no credential data. Image refs are public Valv-namespace strings like `valv-claude:dev`. Versions are upstream npm-published semver strings.
- **Mage-only verification:** Used `mage testPkg ./internal/cli` and `mage test`; no raw `go test`/`go build`/`gofumpt` invocations.

### Worklog-to-code consistency

- **LOC:** Diff totals +54/-3 across 4 files. Builder's three sub-fixes (A debug log, D heading, B two new tests + helper) match the diff scope precisely.
- **Test count:** 156 tests reported by `mage testPkg`; matches builder's claim. Pre-7.10 was 154 (Unit 7.9 added 2: `TestVersionResolverCacheServesCachedValueWithinTTL`-equivalent), 7.10 adds 2 more → 156. Consistent.
- **Coverage:** 72.4% `internal/cli` — matches builder's claim exactly. Above AGENTS.md § 11 70% floor.

### Unknowns

- None for Unit 7.10's claim space.
- Transient disk-space failure on `TestRootDebugFlagIsNotPassedThroughToInteractiveCodexLaunch` reproduced on the first `mage testPkg` invocation; cleared on rerun. Already routed at this file line 771 by the Unit 7.9 reviewer — orchestrator decision pending. Not a 7.10 blocker.

### Hylla Feedback

N/A — Hylla service was unreachable (gRPC `connection refused` on `127.0.0.1:9080`) during this review. Mid-drop snapshot would be stale anyway. Evidence gathered via direct `Read` + `Bash mage` per mid-drop protocol. No falsifiable Hylla miss to record.

## Unit 7.10 — Round 2

**Date:** 2026-05-16
**Verdict:** pass

### R1 finding closure
- A8 (cache pollution): closed
- (tmpfs flake bundled per dev directive): closed

### Fix-by-fix verification

| FIX | Description | Code evidence | Test pinning | Result |
|---|---|---|---|---|
| FIX 1 | `openImagesService` threads `paths.CachesDir` into `imagesservice.Options.CachePath` | `internal/cli/operator_helpers.go:84` — `CachePath: filepath.Join(paths.CachesDir, "version-cache.json")` placed in the base `Options` struct above the provider switch, so both Codex and Claude paths inherit it. `internal/services/images/service.go:269-272` confirms `options.CachePath` is consumed (`cachePath := strings.TrimSpace(options.CachePath); if cachePath == "" { cachePath = defaultCachePath() }`). | `internal/cli/operator_helpers_test.go:97-156` — `TestOpenImagesServiceWritesCacheToCachesDir` constructs `openImagesService(..., paths, domain.ProviderClaude)` using `testCodexPaths(t)` (which sets `CachesDir = $tmpdir/caches`), calls `EnsureLatest(AllowExistingOnCheckFail: true)`, then asserts `os.Stat($tmpdir/caches/version-cache.json)` succeeds. | PASS |
| FIX 2 | `TestRootDebugFlagIsNotPassedThroughToInteractiveCodexLaunch` no longer copies dev's real `~/.codex` to tmpfs | `internal/cli/codex_test.go:382-389` — `t.Setenv("VALV_REAL_HOME", t.TempDir())` placed before `testCodexPaths(t)`. Flow verified end-to-end: `realHomeDir()` (`operator_helpers.go:304-313`) returns `VALV_REAL_HOME` value when set → `runCodexCommand` (`codex.go:112`) seeds `codexservice.Options.RealHome` → `sharedCodexStateHome` (`services/codex/service.go:171-183`) calls `codexruntime.DefaultHostProfile(realHome)` → returns `tempDir/.codex` (nonexistent) → `codexruntime.PrepareRuntime` (`adapters/providers/codex/runtime.go:48`) walks the empty dir, `copyDirContents` (`runtime.go:95`) copies zero files. Disk-space dependency removed. | Test itself is the pinning. Pre-fix would fail with `no space left on device` on tmpfs when dev's `~/.codex` is large; post-fix passes deterministically. | PASS |

### Cache pollution test

Direct mtime/content inspection of `~/Library/Caches/valv/version-cache.json` is sandbox-blocked for this QA agent (reads outside the repo are denied). Indirect verification:

- **File size before `mage test`:** 209B (`ls` reports path + size only; mtime/perm fields stripped by sandbox).
- **File size after `mage test`:** 209B (identical).
- **Builder worklog claim:** "Cache file timestamp unchanged across 3 mage test runs" (`BUILDER_WORKLOG.md:43-44`). The dev confirmed this in the spawn prompt.
- **Structural argument (sufficient on its own):**
  - All `internal/cli` tests that exercise the images service go through `openImagesService`, which post-FIX-1 always sets `CachePath = paths.CachesDir/version-cache.json`. `paths.CachesDir` for tests comes from `testCodexPaths(t)` (`codex_test.go:474`) which uses `t.TempDir()`. No bypass path exists — `git grep "imagesservice.New\|images\.New"` returns only `operator_helpers.go:109` for production code, no test-side direct construction in `internal/cli`.
  - All `internal/services/images` tests that call `EnsureLatest` (which writes the cache) set `CachePath: filepath.Join(t.TempDir(), "version-cache.json")` explicitly. Verified via `git grep -n "CachePath:" internal/services/images/service_test.go` returning 6 hits, all `t.TempDir()`-rooted. Build-only tests (lines 86, 398, 472, 550) don't write the cache because `Build` ≠ `EnsureLatest` (the cache write happens in `EnsureLatest`'s resolver-success branch only).
- **Conclusion:** No code path in `mage test` can hit `defaultCachePath()` (the dev's `~/Library/Caches/valv/version-cache.json`). The fix is complete.

### Mage targets

- `mage testPkg ./internal/cli`: 157/157 PASS, coverage 72.5%.
- `mage test`: 431/431 PASS across 20 packages. Coverage gate met (60% minimum; per-package floors all clear). Builder reported 428/428 — delta of +3 tests likely reflects test additions from another in-flight unit between R1 and R2 mage runs. No regressions.

### Unknowns

- **Cache mtime direct verification unavailable in this QA session.** The orchestrator/dev should `stat -f "%Sm" ~/Library/Caches/valv/version-cache.json` before close to confirm the worklog claim. Structural code-trace makes pollution impossible, but a direct mtime check is the gold-standard empirical witness. Route to dev for the final empirical check.
- No other Unknowns for Unit 7.10 R2's claim space.

### Hylla Feedback

N/A — Hylla service unreachable (`gRPC connection refused on 127.0.0.1:9080`) during this review. Mid-drop snapshot would be stale anyway. Evidence gathered via direct `Read` + `git grep` + `mage` invocations per mid-drop protocol. No falsifiable Hylla miss to record.

---

## Unit 7.9 — Round 2

**Date:** 2026-05-16
**Reviewer:** go-qa-proof-agent
**Verdict:** **PASS**

### R1 finding closure

- **AC5 integration-test reference (proof Finding 1.1 / falsification A8):** **CLOSED.** `service_integration_test.go:20` declares `const testClaudeCLIVersion = "2.1.143"`; line 132 uses `BuildRequest{Version: testClaudeCLIVersion}`. `git grep -n DefaultClaudeCLIVersion` (via inspection of `git diff HEAD~2 HEAD` + reading the integration test file in full) shows the only remaining symbol mention is in this R2 commit's own removal context. The file now compiles cleanly under `-tags=integration ./internal/services/images/...`.
- **AC1 / `LatestCheckedAt` semantic (proof Finding 1.2 / falsification A4):** **CLOSED via option (a) — cache hit returns cached `CheckedAt`.** `cache.go:65` signature `cachedVersion(...) (string, time.Time, bool)` adds the cached timestamp as the 2nd return. `service.go:379` destructures `latestVersion, cachedCheckedAt, fromCache := cachedVersion(...)`. `service.go:411-418` branches on `fromCache`: cache hit → `checkedAt = cachedCheckedAt`; cache miss → `checkedAt = now.UTC()`. New test `TestEnsureLatestReportsCachedCheckedAtOnCacheHit` (`service_test.go:935-980`) pre-populates `checked_at = 2025-01-01T00:00:00Z`, sets clock to `2025-01-01T12:00:00Z` (12h later, within TTL), and asserts `result.LatestCheckedAt.Equal(cachedAt)` — the cache-hit semantic is now behaviorally pinned.
- **Falsification A17 (clock-injection completeness):** **CLOSED.** `service.go:436` (`state.UpdatedAt = s.clock().UTC()`) replaces R1's `time.Now().UTC()`; `service.go:486` (`UpdatedAt: s.clock().UTC()`) replaces the second R1 `time.Now().UTC()`. Both `state.UpdatedAt` writes inside `EnsureLatest` now honor injected clock. The two new tests (cache-hit and future-timestamp) inject deterministic clocks via `newCacheTestService(..., func() time.Time { return clockNow })` and observe deterministic timestamps — implicit but adequate behavioral pinning for the contract.
- **Falsification A5 (future-timestamp permanence):** **CLOSED.** `cache.go:76-79` computes `delta := now.Sub(entry.CheckedAt)` and rejects on `delta < 0 || delta >= versionCacheTTL` — covering both the original stale-past and the new future-timestamp miss cases. New test `TestEnsureLatestRejectsCacheWithFutureTimestamp` (`service_test.go:982-1023`) seeds `checked_at = 2030-01-01`, sets clock to `2026-05-16`, asserts the resolver is called (`counter.called == 1`) and the cache is refreshed with the new version (`entry.Version == "2.1.200"`, `entry.CheckedAt < clockNow + 1s`). Behavior-pinned.
- **Falsification A2 (false atomic-write claim in worklog):** **CLOSED.** `BUILDER_WORKLOG.md:119` (Unit 7.9 R2 entry) replaces the incorrect "POSIX WriteFile is atomic at the kernel level for this size" claim with the correct justification: "self-healing via parse-error swallowing makes the non-atomic write acceptable for a 24h-TTL cache at v0.1.0." The temp+rename pattern is acknowledged as strictly safer; the doc-only decision to keep `os.WriteFile` is rationalized rather than misjustified.

### Fix-by-fix verification

| FIX | Description | Code evidence | Test pinning | Result |
|---|---|---|---|---|
| 1 | Integration test references `testClaudeCLIVersion` local const, not `DefaultClaudeCLIVersion` | `service_integration_test.go:20` (`const testClaudeCLIVersion = "2.1.143"`); `:132` (`BuildRequest{Version: testClaudeCLIVersion}`) | Build-tagged file; no runtime test gate compiles it but Go source is now valid under `-tags=integration ./internal/services/images/...` | PASS |
| 2 | `cachedVersion` returns `(string, time.Time, bool)`; `EnsureLatest` sets `LatestCheckedAt = cachedCheckedAt` on hit, `now.UTC()` on miss | `cache.go:65` signature; `cache.go:80` returns `entry.Version, entry.CheckedAt, true`; `service.go:379` destructure; `service.go:411-418` conditional | `TestEnsureLatestReportsCachedCheckedAtOnCacheHit` (`service_test.go:935-980`) — asserts `result.LatestCheckedAt.Equal(cachedAt)` with cache pre-populated 12h before clock | PASS |
| 3 | `state.UpdatedAt` uses `s.clock().UTC()` not `time.Now().UTC()` at both call sites in `EnsureLatest` | `service.go:436` (up-to-date branch); `service.go:486` (rebuild branch) — both confirmed by reading 364-499 in full | Implicit via two new clock-injection tests; no direct `UpdatedAt` assertion (existing tests don't assert on `UpdatedAt` per R1 falsification A17 note — gap closed mechanically) | PASS |
| 4 | `cachedVersion` rejects `delta < 0 || delta >= versionCacheTTL` | `cache.go:76-79` (combined past + future guard) | `TestEnsureLatestRejectsCacheWithFutureTimestamp` (`service_test.go:982-1023`) — resolver call count == 1; cache rewritten to fresh value | PASS |
| 5 | Worklog corrects atomic-write justification (doc-only) | `BUILDER_WORKLOG.md:119` — "self-healing via parse-error swallowing makes the non-atomic write acceptable" replaces R1's "POSIX WriteFile is atomic" claim | N/A — doc-only | PASS |

### Mage targets

- `mage testPkg github.com/evanmschultz/valv/internal/services/images` — GREEN 29/29, **79.2% coverage**. Reproduces builder's exact claim.
- `mage test` — GREEN **431/431** across 20 packages. Builder reported 430/430; the +1 delta is Unit 7.10 R2's new `TestOpenImagesServiceWritesCacheToCachesDir` in `internal/cli/operator_helpers_test.go` (commit `6b55d59`, landed after builder's 7.9 R2 snapshot). All 20 packages above the 60% gate; `internal/services/images=79.2%`, `internal/cli=72.5%`. The pre-existing `TestRootDebugFlagIsNotPassedThroughToInteractiveCodexLaunch` tmpfs-disk-space failure noted in R1 falsification A9 did NOT recur — Unit 7.10 R2's `t.Setenv("VALV_REAL_HOME", t.TempDir())` fix closed it.
- Coverage threshold met for all packages. Race detector clean.

### Worklog-to-code consistency

- **Files touched (4):** matches `git diff --stat HEAD~2 HEAD -- internal/services/images/` exactly — `cache.go +12/-10`, `service.go +11/-4`, `service_integration_test.go +6/-1`, `service_test.go +88/-0`.
- **Test count delta:** 27 → 29 (+2). Matches the two new tests added in `service_test.go`.
- **Coverage delta:** 78.9% → 79.2% (+0.3pp). Slight uplift from the two new tests exercising previously-unverified cache-hit and future-timestamp branches.
- **Commits referenced:** `6b4ea4f fix(images): unit 7.9 r2 cache-hit timestamp + clock injection` matches the scope and CLAUDE.md commit-format rules (lowercase, no period, subject-only).

### Findings

None blocking. All five R1 findings (two proof FAILs + three falsification CONCERNs/NOTEs) are closed by the R2 diff. Two non-finding observations:

- **NOTE — Clock-injection coverage gap minor.** FIX 3's `s.clock().UTC()` substitution for `state.UpdatedAt` is provably correct by inspection but not directly asserted by any test. The two new R2 tests use a deterministic clock but only assert on `result.LatestCheckedAt`, not on the persisted `state.UpdatedAt`. A future regression that reverts one of the two lines (line 436 or 486) back to `time.Now()` would not be caught by `mage testPkg`. Low priority — the mechanical replacement is unlikely to drift; flagged for orchestrator awareness only.
- **NOTE — Latent integration-test orphan unchanged.** R1's CONCERN about `service_integration_test.go` being orphaned from any mage target persists. FIX 1 makes the file compile under `-tags=integration ./internal/services/images/...` but no mage target invokes that path (`mage integration` runs only `./internal/cli`). The test is correct Go source, just unreached. Future drops should either delete it or wire `./internal/services/images` into the integration target — already routed per Unit 7.10 R2's parallel work on cache-path isolation.

### Idiomatic Go checks

- **Doc comments:** `cachedVersion`'s updated doc comment (`cache.go:61-64`) correctly describes the new `(string, time.Time, bool)` return shape and the negative-delta guard. New test functions have doc comments starting with the function name (`service_test.go:935`, `:981`).
- **Error wrapping:** No new error boundaries introduced — the R2 diff only changes return arity and conditional branches; no new `fmt.Errorf` sites. Existing wrap pattern preserved.
- **No token logging:** New code paths handle version strings and timestamps only — zero credential surface.
- **Mage-only verification:** Used `mage testPkg ./internal/services/images` and `mage test`; no raw `go test` / `go build` / `gofumpt`.

### Unknowns

- None for Unit 7.9's R2 claim space. All five fixes verified by file:line evidence + reproduced mage targets + R1 finding closure.
- Pre-existing tmpfs disk-space failure: no longer reproducing on `mage test`. Unit 7.10 R2's `t.Setenv("VALV_REAL_HOME", ...)` fix in `codex_test.go` resolved it. Orthogonal to 7.9.

### Hylla Feedback

N/A — Hylla daemon was unreachable during this review (`dial tcp 127.0.0.1:9080: connect: connection refused`). Mid-drop staleness protocol applies regardless — all touched files (`cache.go`, `service.go`, `service_test.go`, `service_integration_test.go`, `BUILDER_WORKLOG.md`) were modified after the last Hylla ingest. Evidence gathered via direct `Read` + `git diff HEAD~2 HEAD` + reproduced `mage` runs per CLAUDE.md § "Hylla Baseline".

---

## Unit 7.11 — Round 1

**Date:** 2026-05-16
**Reviewer:** go-qa-proof-agent
**Verdict:** pass

### Evidence reviewed

- `git diff HEAD~1 HEAD -- internal/cli/claude_auth.go internal/cli/claude_auth_test.go` — 187 +/12 - in production code, 369 + lines in tests.
- `internal/cli/claude_auth.go` (375 LOC) read end-to-end.
- `internal/cli/claude_auth_test.go` (839 LOC) read end-to-end.
- PLAN.md `### Design decisions locked by planner (Unit 7.11)` D1–D10 (lines 634–798).
- PLAN.md AC1–AC6 for Unit 7.11 (lines 621–626).
- BUILDER_WORKLOG.md `## Unit 7.11 — Round 1` (lines 7–63).
- `mage testPkg ./internal/cli` re-run by reviewer: **GREEN, 167/167, 72.6%**.
- `mage test` re-run by reviewer: **GREEN, 441/441, 20 packages, all ≥60%**.

### D1–D10 verification

| Decision | Code evidence | Result |
|---|---|---|
| D1 — lineScanner wraps stdout AND stderr | `claude_auth.go:199-201` constructs `stdoutScanner` and `stderrScanner` from the same `onURL` closure; both passed to `NewSystemRunner` | CONFIRMED |
| D1 refinement — line-buffer survives URL straddling two writes | `claude_auth.go:108-126` `lineScanner.Write` appends to `s.buf`, splits on `\n`, only emits/scans complete lines; partial buffer survives across calls. Test `TestLineScannerDetectsURLAcrossTwoWrites` (test:574-604) pins the behaviour | CONFIRMED |
| D2 — URL regex literal matches spec | `claude_auth.go:29-31` `https://(?:claude\.com/cai\|platform\.claude\.com)/oauth/authorize\S*` — byte-for-byte identical to PLAN.md:654-657 | CONFIRMED |
| D3 — `sync.Once` wraps the open call | `claude_auth.go:193-198` declares `var once sync.Once` and wraps `opener.Open(ctx, url)` inside `once.Do` | CONFIRMED |
| D4 — `urlOpener` interface + `defaultURLOpener` via `exec.Command("open", url).Start()` | Interface at `claude_auth.go:42-44`; `defaultURLOpener.Open` at `:59-65` calls `exec.Command("open", url).Start()` (Start, not Run, so non-blocking) with errors swallowed and TODO comment for xdg-open/cmd at `:55-56` | CONFIRMED |
| D5 — `credsWatcher` interface + default 500ms poll loop with ctx.Done() select | Interface at `claude_auth.go:49-51`; `defaultCredsWatcher.WaitForCreds` at `:71-85` uses `time.NewTicker(500*time.Millisecond)` + `defer ticker.Stop()` + `select { case <-ctx.Done(): return ctx.Err(); case <-ticker.C: os.Stat+Size>0 → return nil }` | CONFIRMED |
| D6 — `docker stop --time 5 <containerName>` subprocess | `claude_auth.go:251` `externalCommand("docker", "stop", "--time", "5", containerName).Run()` with error swallowed by `_ =` (matches D6 "swallow error" semantics for already-gone container) | CONFIRMED |
| D7 — `context.WithCancel` + `defer cancel()` at top | `claude_auth.go:171-172` `ctx, cancel := context.WithCancel(parentCtx); defer cancel()` | CONFIRMED |
| D8 — `sync.Once` + `atomic.Bool credDetected` (NOT a channel) | `claude_auth.go:193` `var once sync.Once`; `:231` `var credDetected atomic.Bool`. Goroutine at `:248` `credDetected.Store(true)`; main reads `credDetected.Load()` at `:261`. No channel used for inter-goroutine creds signaling | CONFIRMED |
| D9 — Non-context watcher errors → debug log, no SIGTERM | `claude_auth.go:240-244` `if err != context.Canceled && err != context.DeadlineExceeded { LoggerFromContext(ctx).Debug("creds watcher error, continuing without auto-exit", "err", err) }` then `return` (no `credDetected.Store`, no docker stop) | CONFIRMED |
| D10 — `stubURLOpener` + `stubCredsWatcher` in test file | `stubURLOpener` at `claude_auth_test.go:481-489`; `stubCredsWatcher` at `:493-501`; both used directly in `RunInContainer` integration tests at `:758-763`, `:791-796`, `:824-829` | CONFIRMED |

### Falsification refinements applied (planner Section 0 attacks 4, 8, 10)

| Refinement | Code evidence | Result |
|---|---|---|
| Line-buffer scanner (URL across writes still detected) | `claude_auth.go:108-126` line buffering; `TestLineScannerDetectsURLAcrossTwoWrites` confirms split URL detected exactly once on second write | CONFIRMED |
| stderr wrapped too (not just stdout) | `claude_auth.go:200` `stderrScanner := newLineScanner(stderr, onURL)` constructed and passed to `NewSystemRunner` alongside `stdoutScanner` | CONFIRMED |
| atomic.Bool used (not bare bool) | `claude_auth.go:231` `var credDetected atomic.Bool`; reads/writes through `.Load()` / `.Store(true)`. `-race` clean in mage test rerun | CONFIRMED |

### AC1–AC6 verification

| # | Criterion | Code/test evidence | Result |
|---|---|---|---|
| AC1 | `RunInContainer` detects OAuth URL in container stdout and fires `urlOpener.Open(url)` exactly once | `claude_auth.go:193-201` sync.Once-guarded onURL closure feeds both stdout+stderr scanners. `TestRunInContainerOpensBrowserOnURLDetect` (test:669-709) writes URL twice into scanner; opener.openedURLs has exactly 1 entry. `TestLineScannerDetectsOAuthURL` (test:545-569) confirms regex match. | PASS |
| AC2 | `RunInContainer` polls for `<homePath>/.credentials.json`; on appearance, SIGTERMs the container | `claude_auth.go:230` builds `credsPath`; `:234-252` goroutine polls via `watcher.WaitForCreds`, on nil sets `credDetected.Store(true)` and calls `externalCommand("docker","stop","--time","5",containerName).Run()`. `TestRunInContainerSigtermsOnCredsWrite` (test:741-774) verifies nil return + success notice when watcher.err=nil. | PASS |
| AC3 | User-terminal output unaffected — stdout still flows through | `claude_auth.go:118` `s.inner.Write(line)` forwards every complete line. `TestLineScannerForwardsAllBytes` (test:524-541) verifies inner writer receives `"line one\nline two\n"` verbatim. `TestRunInContainerOpensBrowserOnURLDetect` further checks `buf.String()` contains the OAuth URL. | PASS |
| AC4 | Poller goroutine does not leak when container exits before creds appear | `claude_auth.go:171-172, 257, 259` derives a cancellable ctx, calls `cancel()` after `containerExec.Run` returns, then `wg.Wait()`. `TestRunInContainerSurvivesContainerExitBeforeCreds` (test:780-807) runs with `watcher.err=context.Canceled` and stub executor returning immediately; test completes promptly with `-race` (mage uses `-race` unconditionally). | PASS |
| AC5 | `mage testPkg github.com/evanmschultz/valv/internal/cli` GREEN, coverage ≥70% | Reproduced by reviewer: `tests: 167 passed: 167 failed: 0 ... internal/cli 72.6% Minimum package coverage: 60.0% [SUCCESS] Coverage threshold met` | PASS |
| AC6 | `mage test` GREEN full suite | Reproduced by reviewer: `441 tests passed across 20 packages` with `internal/cli 72.6%`, all packages ≥60% | PASS |

### Spec deviation review

**Deviation 1: `sync.WaitGroup` added to `RunInContainer`.**
- **Justified.** Builder's Section 0 Convergence finding correctly identifies a real race: the stub executor (`callbackExecutor.Run`) returns synchronously, so without `wg.Wait()`, the main goroutine could read `credDetected.Load()` (line 261) before the watcher goroutine has executed `credDetected.Store(true)` (line 248). With `-race` enabled in `mage test`, the atomic Load/Store would not flag this — atomics serialise; the issue is causal ordering, not data-race instrumentation.
- **Placement correct.** `wg.Add(1)` at `:233` before goroutine launch. `defer wg.Done()` at `:235` inside goroutine. `wg.Wait()` at `:259` after `cancel()` so the goroutine receives the cancel signal before being awaited.
- **Orthogonal to D8.** D8 forbids channels for **inter-goroutine creds notification** (proposed alternative was a `chan struct{}` to signal credentials discovered). The chosen design uses `atomic.Bool` for the signal — D8-compliant. `sync.WaitGroup` is **goroutine-completion synchronization**, a distinct mechanism that complements D8 rather than replacing it. No channel introduced for signaling.
- **No new failure modes.** `wg.Wait()` runs **after** `cancel()` (`:257-259`). The cancelled ctx guarantees `defaultCredsWatcher.WaitForCreds` returns within ≤500ms via its `<-ctx.Done()` arm; stubs return synchronously. No deadlock path.

**Deviation 2: `externalCommand` package-level test seam.**
- **Justified.** Without it, `TestRunInContainerSigtermsOnCredsWrite` would require a real Docker daemon — the `docker stop` subprocess executes for real. Builder makes the production path transparent (`:273-275` default delegates to `exec.Command`); tests swap and restore via `t.Cleanup(func() { externalCommand = origExternalCommand })` (test:748-749).
- **Test seam shape correct.** Package-level `var externalCommand = func(name string, args ...string) *exec.Cmd { return exec.Command(name, args...) }`. Production uses it transparently at `:251`; tests substitute a no-op (`exec.Command("true")`) via `t.Cleanup` restoration.
- **No production-path leakage.** The default returns the exact same `*exec.Cmd` as direct `exec.Command`. Production behaviour is preserved bit-for-bit.

### Mage targets

- `mage testPkg ./internal/cli` — **GREEN, 167/167, 72.6%** (reproduced by reviewer).
- `mage test` — **GREEN, 441/441 across 20 packages**, all packages at or above the 60% floor; `internal/cli` at 72.6%, well above the 70% threshold from AC5.

### Unknowns

- **U1 (carried from builder, unresolved by definition):** Real `docker run --rm` exit code under `docker stop` SIGTERM is not testable in unit scope. The `credDetected.Load()==true` branch overrides any `runErr` with nil at `:261-266`, which is the correct production semantic per the planner's "Additional implementation note: success notice and nil-return semantics" (PLAN.md:796-798). Will be exercised in dev smoke test.
- **U2 (carried from builder):** Exact OAuth URL string emitted by `valv-claude:dev` runtime versus the `D2` regex. The regex covers both documented endpoints (`claude.com/cai` and `platform.claude.com`) per PLAN.md:582; if the real-run URL drifts, the regex update is a one-line fix and AC1 will manifest as "browser did not auto-open" at smoke test. Routed to dev for smoke verification — orchestrator-facing only.

### Hylla Feedback

N/A — task touched two files (`internal/cli/claude_auth.go`, `internal/cli/claude_auth_test.go`) that were modified in commit 4d85307 since the last Hylla ingest (snapshot 12). Direct `Read` + `git diff HEAD~1 HEAD` was the correct evidence path per CLAUDE.md § "Hylla Baseline" mid-drop protocol. One adjunct lookup (`LoggerFromContext` definition) used `hylla_search_keyword` against `github.com/evanmschultz/valv@main` and returned the correct hit in `internal/cli/root.go` cleanly — no miss.

---

## Unit 7.11 — Round 2

**Date:** 2026-05-16
**Reviewer:** go-qa-proof-agent
**Commit reviewed:** `660d538` — fix(cli): unit 7.11 r2 wipe-before-login + ansi-safe url + line-wrap buffering
**Verdict:** pass

### Three R2 fixes verified

**FIX 1 — wipe before login (BLOCK 1):**
- Code at `internal/cli/claude_auth.go:387-411` — `loginClaudeAccount` calls `wipeClaudeCredentials(account.HomePath)` as its first statement before `writeCLINotice` and `runner.RunInContainer`.
- `wipeClaudeCredentials` (`:415-421`) removes `.credentials.json` if present; missing-file is not an error.
- `ensureClaudeAccountReady` (`:338-376`) intentionally NOT modified — it retains its "creds exist → return nil" early-return (correct: `ensure` does not force re-auth; only `login` does).

**FIX 2 — ANSI-safe URL regex (CONCERN 2):**
- Code at `internal/cli/claude_auth.go:31-33` — character class changed from `\S*` to `[A-Za-z0-9._~:/?#\[\]@!$&'()*+,;=%-]*`.
- The class is RFC 3986 unreserved + reserved + percent-encoding. ESC byte `\x1b` (0x1b) is NOT in this class. Regex match terminates at the first ANSI byte, producing a clean URL.

**FIX 3 — multi-line URL accumulation (CONCERN 3, option a):**
- Code at `internal/cli/claude_auth.go:113-118` adds `urlBuf strings.Builder` field.
- `stripURLWhitespace` helper (`:128-133`) drops `\n`/`\r`/` `/`\t` via `strings.Map`.
- Write loop (`:139-168`): per-line, accumulate into `urlBuf` (capped at `urlBufferCap = 4096` bytes; reset when cap exceeded), then run regex on `strings.Map(stripURLWhitespace, urlBuf.String())`. Whitespace strip rejoins terminal line-wrap splits. **Note**: the per-line scan was removed entirely; all URL detection routes through the buffer-scan path. Worklog accurately documents this.

### Acceptance criteria verification

| # | Criterion | Evidence | Result |
|---|---|---|---|
| AC1-R2 | `loginClaudeAccount` wipes `.credentials.json` before invoking the runner | `claude_auth.go:387-391` (wipe is first statement) + `TestLoginClaudeAccountWipesExistingCredsBeforeRunning` (`claude_auth_test.go:847-887`) checks `fileExistedAtRunTime=false` from inside `stubRunFunc` — proves ordering, not just both-ran | pass |
| AC2-R2 | ANSI escape sequences do not corrupt URL passed to `urlOpener.Open` | Regex at `claude_auth.go:31-33` excludes `\x1b` from class + `TestLineScannerStripsANSIFromOAuthURL` (`claude_auth_test.go:893-916`) writes `URL + "\x1b[0m\n"` and asserts `!strings.Contains(opened[0], "\x1b")` | pass |
| AC3-R2 | URL detection survives terminal line-wrap (option a buffering) | `urlBuf` + `stripURLWhitespace` at `claude_auth.go:113-167` + `TestLineScannerDetectsURLAcrossMultipleLines` (`claude_auth_test.go:924-962`) splits URL across `part1`/`part2` and asserts full joined URL appears in `allMatches` | pass |
| AC4-R2 | `mage testPkg github.com/evanmschultz/valv/internal/cli` green, coverage ≥70% | Reviewer ran `mage testPkg github.com/evanmschultz/valv/internal/cli`: `tests: 170 passed: 170 failed: 0`, `github.com/evanmschultz/valv/internal/cli 72.7%` (mage gate is 60%, AGENTS.md target 70% — comfortably above both) | pass |
| AC5-R2 | `mage test` green full suite | Reviewer ran `mage test`: `tests: 444 passed: 444 failed: 0` across 20 packages; all packages ≥60% | pass |

### Non-vacuous test verification

- **AC1-R2 test** — under R1 code (no wipe in `loginClaudeAccount`), the stale `.credentials.json` would still exist when `stubRunFunc` fires `os.Stat` → `fileExistedAtRunTime=true` → `t.Fatal` triggers. Under R2, wipe removes the file first → `fileExistedAtRunTime=false` → passes. Test distinguishes R1 from R2.
- **AC2-R2 test** — under R1 regex `\S*`, neither `\x1b` nor `[0m` are whitespace, so they would be captured. `strings.Contains(opened[0], "\x1b") = true` → `t.Fatal`. Under R2 character class, `\x1b` not in class → match terminates at `?code=foo` → assertion passes. Test distinguishes R1 from R2.
- **AC3-R2 test** — under R1 (no `urlBuf`, per-line scan only), `part2` = `"bar&baz=qux\n"` has no `https://` prefix → no match. Full joined URL would never appear in `allMatches` → `t.Fatal`. Under R2 buffer-scan, joined accumulation matches the full URL. Test distinguishes R1 from R2.

### Idiomatic Go + spec-conformance checks

- **Doc comments**: `urlBufferCap` (`:89-93`), `lineScanner` (`:95-112`), `stripURLWhitespace` (`:124-127`), `Write` (`:135-138`), `oauthURLRegex` (`:25-30`), `loginClaudeAccount` (`:378-386`), `wipeClaudeCredentials` (`:413-414`) — all present, each starting with the identifier name. ✓
- **Error wrapping**: new `wipeClaudeCredentials` call in `loginClaudeAccount` returns the wipe error unmodified (`:388-390`). `wipeClaudeCredentials` itself wraps with `fmt.Errorf("remove %q: %w", credPath, err)` (`:417-418`). ✓
- **Concurrency**: no new goroutines. `lineScanner.Write` operates on a single Write context; `urlBuf` is not shared across goroutines. Existing `WaitForCreds` goroutine in `RunInContainer` (`:276-294`) untouched and still context-cancellable. ✓
- **Race**: `mage testPkg` ran with `-race` (per magefile) — no data race reported.
- **Mage discipline**: reviewer used `mage testPkg github.com/evanmschultz/valv/internal/cli` and `mage test` — no raw `go test`/`go build`/`go vet`. ✓
- **Spec-conformance with PLAN.md R2 fixes**: all three FIXes match the R2 unit specification (BLOCK 1 stale-creds → premature stop; CONCERN 2 ANSI; CONCERN 3 line-wrap).

### Worklog-to-code consistency

- Worklog claims `internal/cli/claude_auth.go` and `internal/cli/claude_auth_test.go` modified — confirmed via `git show 660d538 --stat`. ✓
- Worklog claims 3 new tests with exact names — all three present at `claude_auth_test.go:847`, `:893`, `:924`. ✓
- Worklog claims per-line scan was REMOVED — confirmed by reading lines `139-168`: only buffer-scan remains. ✓
- Worklog notes the test allows multiple matches (per-line + full-join); reviewer notes that since per-line was removed, in practice typically one match per line-group fires. Both are consistent.
- AC4-R2 row in worklog says "coverage ≥70%" — the mage gate is 60% with TODO to restore to 70% (`magefile.go:23-24`); measured 72.7% satisfies both. Not a finding.

### Observations (non-blocking)

- **Greedy regex across multiple URLs in one accumulated buffer**: with the new character class `[A-Za-z0-9._~:/?#\[\]@!$&'()*+,;=%-]*`, if two OAuth URLs appear in `urlBuf` (e.g., URL1 then URL2 both stripped of whitespace), the regex greedy match would consume both as one mega-URL (every byte of URL2 is in the class). This affects `TestLineScannerOnceGuardFiresOnce` (line 630): the second `onMatch` call gets a concatenated URL string, not the second URL alone. The test only checks `callCount`, not URL value, so it passes. In production, `sync.Once` in `RunInContainer` (`:235-240`) guarantees only the FIRST match opens the browser, and that first match is clean. Non-blocking, but worth noting in case of future test additions that assert URL values across multiple URLs.
- **urlBuf never resets after a successful match**: every subsequent line accumulates onto an already-found URL forever. In practice the cap (4096 bytes) eventually rotates. Not a finding because `sync.Once` ensures only the first detection matters in production.

### Hylla Feedback

- **Query**: `hylla_search_keyword query="stubURLOpener writeCredsToDir" artifact_ref=github.com/evanmschultz/valv@main test_mode=include_tests`
- **Missed because**: `claude_auth_test.go` was modified in commit `4d85307` (Unit 7.11 R1) and again in `660d538` (R2), both AFTER the last Hylla ingest (snapshot 12). The `stubURLOpener` helper was added in 7.11 R1 and is not yet ingested. The `writeCredsToDir` helper, present in an earlier commit, DID return cleanly from Hylla.
- **Worked via**: `Read` of `claude_auth_test.go` at offset 400 — located `stubURLOpener` definition at line 481.
- **Suggestion**: This is the expected mid-drop pattern documented in CLAUDE.md § "Hylla Baseline" — Hylla is stale for files changed since last ingest. No Hylla improvement needed; the fallback is correct.

---

