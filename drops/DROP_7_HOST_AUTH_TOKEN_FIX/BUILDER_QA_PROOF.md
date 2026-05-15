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
