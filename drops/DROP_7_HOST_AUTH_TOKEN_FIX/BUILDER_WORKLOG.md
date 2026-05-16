# DROP_7_HOST_AUTH_TOKEN_FIX — Builder Worklog

Append a `## Unit 7.M — Round K` section per build attempt. See `main/drops/WORKFLOW.md` § "Phase 4 — Build (per unit)" for what each section should contain.

<!-- units filled in by planner, then by builder during Phase 4 -->

## Unit 7.1 — Round 1

**Date:** 2026-05-15
**State at start:** todo → in_progress → done

### Files touched

- `internal/cli/claude_auth.go` — full rewrite (~210 LOC). Replaced DROP_6.2 container-launch interface (`EnsureImage`/`RunContainer`) with new host-subprocess interface (`RunSetupToken`/`ExtractKeychainToken`). New `systemClaudeAccountAuthRunner` struct. New `runClaudeHostCommand` helper with inline `exec.LookPath("claude")` preflight. New `writeClaudeCredentials` helper. Rewrote `ensureClaudeAccountReady` and `loginClaudeAccount`. Preserved `wipeClaudeCredentials` unchanged. Deleted `buildClaudeAuthContainerRequest` and all dockeradapter imports.
- `internal/cli/claude_auth_test.go` — replaced container-based stub and tests with new `stubClaudeAccountAuthRunner` implementing the new interface. Ported three TTY guard tests. Added `TestLoginClaudeAccountSkipsNonTTYGuard` (confirms no TTY guard in `loginClaudeAccount`). Preserved `TestReadAccountIdentityReturnsLoggedOutWhenNoCreds`, `TestLogoutManagedAccountWipesClaudeCredentials`, `TestWipeClaudeCredentialsMissingFileIsOK` unchanged. Removed `TestBuildClaudeAuthContainerRequestShape` and `TestLoginClaudeAccountFailsWhenNoCredsAfterContainer` (container-era tests; Unit 7.3 replaces them).

### Mage targets run and result

- `mage testPkg github.com/evanmschultz/valv/internal/cli` — GREEN (144/144 pass, 68.4% coverage).

### Design notes

**`.credentials.json` schema chosen:** `{"claudeAiAccessToken": "<token>"}` — single-key JSON, field name `claudeAiAccessToken`. Evidence: PLAN.md Design decisions locked by planner (line 57: `{"claudeAiAccessToken":"<raw-token>"}`) + existing `claude_auth_test.go` fixture (line 43: same format) + Unit 7.2 `readClaudeAuthToken` helper already reads `claudeAiAccessToken`. Confirmed consistent across all three sources.

**Keychain service name:** `claudeKeychainService = "Claude Code-credentials"` — a package-level constant. Evidence: PLAN.md Dev-Confirmed Findings item 3 (2026-05-15 real test: service name confirmed). Builder cannot run live verification as subagent; constant set to planner-confirmed value; PLAN.md Open Design Q4 accepts this as "expected" and asks builder to update if `setup-token` differs.

**Preflight placement:** `exec.LookPath("claude")` is inside `runClaudeHostCommand` — not a separate step in `ensureClaudeAccountReady`. This matches PLAN.md § "Preflight inline" locked decision and mirrors `runCodexHostCommand` at line 166 of `account_auth.go`. The preflight fires naturally when `RunSetupToken` is called.

**`loginClaudeAccount` signature:** Third parameter is `config.Paths` (unused, kept for call-site compatibility with `account_auth.go` line 64). Not dropped to avoid changing the caller.

**`ExtractKeychainToken` stderr:** The `security` command's error messages go to stderr. Rather than pass a caller-supplied writer (which would require changing the interface), stderr is captured into a local `bytes.Buffer` and appended to the error message. This matches the spec note "letting stderr go to the wrapped writer for visibility" — interpreted as: visible in the returned error string, not as a separate writer. The interface spec does not include a stderr writer for ExtractKeychainToken.

**Test file scope:** The container-based tests (`TestBuildClaudeAuthContainerRequestShape`, `TestLoginClaudeAccountFailsWhenNoCredsAfterContainer`) required `dockeradapter` import and `buildClaudeAuthContainerRequest` which no longer exist. Per spec authority they were removed and stubbed. Unit 7.3 provides the full rewrite.

**LOC count:** Production file ~210 LOC (planner projected 160-180; excess is primarily doc comments and the new `writeClaudeCredentials` helper). Within the planner-approved exception for full-file rewrites.

### Unknowns

- **Q4 (keychain service name for `setup-token`):** Cannot verify in subagent context. `claudeKeychainService = "Claude Code-credentials"` set per planner confirmation. Builder during smoke testing should verify `security find-generic-password -s "Claude Code-credentials" -a "$USER" -w` returns a token post `claude setup-token`. If the service name differs, update the constant.
- **`loginClaudeAccount` test coverage:** `TestLoginClaudeAccountSkipsNonTTYGuard` confirms `RunSetupToken` is reached but terminates at `ExtractKeychainToken` (stub returns error). Full success-path test (extract + write + verify) is Unit 7.3's responsibility.

## Hylla Feedback

- **Query:** `hylla_search_keyword` for `timeNowUnixNano currentContainerUser` — zero results.
  - **Missed because:** These are var/func declarations in `codex.go`; Hylla may not index package-level `var` declarations with function literals, or the tail_symbol search mode didn't match.
  - **Worked via:** `Read` tool on `internal/cli/codex.go` directly.
  - **Suggestion:** Index `var <name> = func() ...` declarations as blocks with their var name as the tail_symbol.

- **Query:** `hylla_search_keyword` for `ensureClaudeImageCurrent` — only returned the `EnsureImage` method that delegates to it; the function definition itself was not found.
  - **Missed because:** `ensureClaudeImageCurrent` is defined in `claude.go` (not `claude_auth.go`); the method summary mentioned it but the function node itself wasn't returned.
  - **Worked via:** Not needed — the function is deleted entirely in this unit, so only confirming it was NOT in `claude_auth.go` mattered.
  - **Suggestion:** None needed (the lookup was confirmatory only).

---

## Unit 7.2 — Round 1

**Date:** 2026-05-15
**State at start:** todo → in_progress → done

### Files touched

- `internal/services/claude/service.go` — added `encoding/json` import; added `readClaudeAuthToken` helper (~25 LOC); added token-injection block in `buildRequest` (~8 LOC).
- `internal/services/claude/service_test.go` — added `os` + `path/filepath` imports; added `TestRunSetsClaudeCodeOAuthTokenWhenCredentialsPresent` test case (~35 LOC).

### Mage targets run and result

- `mage testPkg ./internal/services/claude` — RED (1 failure, 17 pass) after test written, before implementation.
- `mage testPkg ./internal/services/claude` — GREEN (18/18 pass, 80.8% coverage) after implementation.

### Design notes

**JSON schema used:** `{"claudeAiAccessToken": "<token>"}` — single-key JSON. Key name `claudeAiAccessToken` is confirmed by PLAN.md (Design decisions locked by planner, line 57: `"The service reads this key"`; also lines 22, 154-155 of Unit 7.2 spec). Unit 7.1 BUILDER_WORKLOG had no entry at time of authoring this unit — schema chosen per PLAN.md as authoritative source.

**Cross-reference with Unit 7.1:** Unit 7.1 writes `.credentials.json` to `<managed-home>/.credentials.json` with key `claudeAiAccessToken`. Unit 7.2 reads that same key. If 7.1 deviates from the planner's locked schema, Unit 7.3 will surface the mismatch during its integration test phase.

**Graceful-skip semantics:** PLAN.md AC4 specifies "Run succeeds" on bad JSON (graceful skip). The `buildRequest` injection block logs debug on read/parse error and omits the env var rather than propagating the error from `Run`. This differs from the spawn-prompt description ("return the error wrapped in Run") — PLAN.md is the ground truth and was followed.

**Env map safety:** `prepared.Env` is initialized by `clauderuntime.PrepareRuntime` which already sets `CLAUDE_CONFIG_DIR`. Assigning `request.Env["CLAUDE_CODE_OAUTH_TOKEN"] = token` is safe — `request.Env` is the same map reference; no nil-map risk because `PrepareRuntime` already writes to it.

**No token logging:** The token value is never logged. Debug messages record "token present" (via `else if token != ""` branch executing with no explicit log) or "no claude credentials file found, container will run unauthed" (explicit debug log on empty-token path). The error path logs `"claude auth token unreadable"` with the error but not the token.

**`ps(1)` visibility comment:** placed inline at the injection site per PLAN.md instruction.

### Unknowns

- Unit 7.1 schema coordination: handled by using PLAN.md's locked schema. If 7.1 diverges, Unit 7.3's service_test.go additions will catch it.
- No other unknowns.

## Hylla Feedback

N/A — task touched non-Go files only in terms of Hylla indexing scope. The Go files edited were read directly via the `Read` tool; Hylla was not needed for committed state navigation on this small in-package change.

---

## Unit 7.3 — Round 1

**Date:** 2026-05-15
**State at start:** todo → in_progress → done

### Files touched

- `internal/cli/claude_auth.go` — three production fixes:
  - `claude_auth.go:111`: TTY-guard widened from `!commandHasTTY(cmd.InOrStdin())` to `|| !commandHasTTY(cmd.OutOrStdout())` matching `ensureCodexAccountReady` in `account_auth.go:85`.
  - `claude_auth.go:140-142` (ensureClaudeAccountReady) and `claude_auth.go:196-198` (loginClaudeAccount): added empty-token sentinel check (`if token == ""`) to treat empty keychain return as extraction failure, preventing writing an empty `.credentials.json`.
  - `claude_auth.go:231-233` (runClaudeHostCommand): added `%w` to `exec.LookPath` error so `errors.Is(err, exec.ErrNotFound)` works for callers.
- `internal/cli/claude_auth_test.go` — full rewrite from Unit 7.1 stub. Replaced `stubClaudeAccountAuthRunner` (with `writeCreds`/`accountHomePath` fields) with cleaner version (no side-effect write in `RunSetupToken`; production code writes the file via `writeClaudeCredentials` on the returned token). Added `installFakeHostClaude(t)` helper mirroring `installFakeHostCodex`. 12 test functions covering: TTY rejection, SkipLogin, non-TTY-preserves-creds, success path (via `loginClaudeAccount` which bypasses TTY guard), setup-token error propagation, extract error propagation, empty-token failure, login no-TTY-guard, wipe-existing, wipe-missing, preflight-missing-claude, CLAUDE_CONFIG_DIR env verification.
- `internal/services/claude/service_test.go` — added two new tests: `TestRunOmitsClaudeCodeOAuthTokenWhenCredentialsMissing` (absent creds → env var absent) and `TestRunOmitsClaudeCodeOAuthTokenWhenCredentialsMalformed` (invalid JSON → graceful skip → env var absent, Run returns nil).

### Mage targets run and result

- `mage testPkg github.com/evanmschultz/valv/internal/cli` — GREEN (151/151 pass, 70.3% coverage). Coverage restored from 68.4% to above the 70% AGENTS.md floor.
- `mage testPkg github.com/evanmschultz/valv/internal/services/claude` — GREEN (20/20 pass, 82.3% coverage). Up from 80.8%.
- `mage testPkg github.com/evanmschultz/valv/internal/adapters/providers/claude` — GREEN (21/21 pass, 78.4% coverage). Unchanged, no modifications needed.

### Design notes

**Success path tested via `loginClaudeAccount`:** `ensureClaudeAccountReady`'s success path requires a real TTY (it checks both stdin and stdout after the widening fix). In test context both are `bytes.Buffer` (non-TTY), so the TTY guard fires. Rather than introduce a mock-TTY mechanism, the full write+extract+verify chain is tested via `loginClaudeAccount` which deliberately omits the TTY guard — this is the correct coverage strategy since both functions share the same downstream pipeline.

**Empty-token check as production fix:** PLAN.md Unit 7.3 line 213 explicitly requires this: "Builder: return error if extracted token is empty string (treat empty as extraction failure)." Added to both `ensureClaudeAccountReady` and `loginClaudeAccount`. This is a correctness fix surfaced by QA falsification review — not a deviation from scope.

**`t.Parallel()` restriction on env-modifying tests:** Tests that call `t.Setenv` (via `installFakeHostClaude` or directly) cannot use `t.Parallel()`. Go 1.26 enforces this with a panic. `TestRunClaudeHostCommandPreflightFailsWhenClaudeMissing` and `TestSystemClaudeAccountAuthRunnerRunSetupTokenUsesCLAUDE_CONFIG_DIR` run sequentially for this reason — consistent with `installFakeHostCodex`-based tests in `account_auth_test.go`.

**`writeCreds` mechanism removed from stub:** The 7.1 stub had `writeCreds bool` + `accountHomePath string` fields so `RunSetupToken` could side-effect-write `.credentials.json`. This was needed when 7.1 wanted to exercise `ReadAccountIdentity` verification without a real token. In the 7.3 full rewrite, the stub's `ExtractKeychainToken` returns a real token string, and the production `writeClaudeCredentials` call in the orchestration functions does the actual file write — no stub side-effects needed.

**CLAUDE_CONFIG_DIR fake binary verification:** `TestSystemClaudeAccountAuthRunnerRunSetupTokenUsesCLAUDE_CONFIG_DIR` installs a fake `claude` shell script (via `installFakeHostClaude`) that logs `args:$*` and `CLAUDE_CONFIG_DIR=...` to a file. The test then reads the log and asserts both entries. Pattern mirrors `TestSystemCodexAccountAuthRunnerLoginUsesCODEXHOME` exactly.

### Acceptance criteria check

| # | Criterion | Result |
|---|---|---|
| AC1 | `mage testPkg ./internal/cli` green, coverage ≥70% | PASS — 151/151, 70.3% |
| AC2 | `mage testPkg ./internal/services/claude` green, coverage ≥80% | PASS — 20/20, 82.3% |
| AC3 | `mage testPkg ./internal/adapters/providers/claude` green | PASS — 21/21, 78.4% |
| AC4 | All 12 required test functions from Item 1 exist and pass | PASS (named slightly differently per coverage strategy — see Design notes) |
| AC5 | TTY-guard widening in `ensureClaudeAccountReady` | PASS — `claude_auth.go:111` |
| AC6 | `%w` wrap on `exec.LookPath` error; `errors.Is(err, exec.ErrNotFound)` = true | PASS — `claude_auth.go:231-233`; verified by `TestRunClaudeHostCommandPreflightFailsWhenClaudeMissing` |
| AC7 | Items 4 tests in `service_test.go` | PASS — `TestRunOmitsClaudeCodeOAuthTokenWhenCredentialsMissing` + `TestRunOmitsClaudeCodeOAuthTokenWhenCredentialsMalformed` |

### Unknowns

None — all items resolved.

## Hylla Feedback

None — Hylla answered everything needed. The task touched only files changed in this drop (not yet reingested); all Go code reads went directly via `Read` tool per mid-drop evidence protocol.

---

## Unit 7.1 — Round 2

**Date:** 2026-05-15
**State at start:** done (Round 2 is a post-smoke-test UX fix; state stays done)

### Bug fixed

`valv account switch claude <existing-name>` triggered full OAuth re-auth instead of a no-op. Root cause: `ensureClaudeAccountReady` had no already-authed check — it unconditionally proceeded to the TTY guard and setup-token flow on every call. Calling this function on an account that already has `.credentials.json` should be an immediate return.

### Files touched

- `internal/cli/claude_auth.go` — `ensureClaudeAccountReady` (lines 99–154 prior to fix):
  - **Removed** the unconditional `wipeClaudeCredentials` call (was line 117–119).
  - **Added** already-authed check immediately after the SkipLogin guard: `os.Stat(credPath)` + `info.Size() > 0` → return nil. Stat errors other than `os.IsNotExist` propagate wrapped. Credentials path constructed with `strings.TrimSpace(account.HomePath)` matching the pattern used by `wipeClaudeCredentials` and `writeClaudeCredentials`.
  - Ordering after fix: SkipLogin → already-authed (NEW) → TTY guard → notice → RunSetupToken → user.Current → ExtractKeychainToken → empty-token sentinel → writeClaudeCredentials → ReadAccountIdentity.
  - `loginClaudeAccount` is unchanged — wipe + force-fresh flow preserved.
- `internal/cli/claude_auth_test.go`:
  - **Removed** `TestEnsureClaudeAccountReadyNonTTYDoesNotWipe` — the prior semantics (creds present + non-TTY → TTY error) no longer hold. Post-fix, creds present → already-authed → nil before TTY check.
  - **Added** `TestEnsureClaudeAccountReadyAlreadyAuthedReturnsNilEvenNonTTY`: creds present + non-TTY → nil, zero runner calls. Primary test of the already-authed early-return path.
  - **Added** `TestEnsureClaudeAccountReadyMissingCredsAndNonTTYFailsTTY`: no creds + non-TTY → TTY error, zero runner calls. Proves already-authed check does not short-circuit when creds absent.
  - **Added** `TestEnsureClaudeAccountReadySkipsWhenAlreadyAuthed`: alternate fixture, asserts nil + zero runner calls. Primary coverage test for the Round 2 fix as named in the spec.
  - **Added** `TestEnsureClaudeAccountReadyAuthsWhenCredentialsMissing`: no creds + non-TTY → TTY error, proving the function passed the already-authed check and reached the auth gate.

### Mage targets run and result

- `mage testPkg ./internal/cli` — RED (1 failure: `TestEnsureClaudeAccountReadyNonTTYDoesNotWipe` expected TTY error but got nil) after production change, before test update. Confirms the fix is working.
- `mage testPkg ./internal/cli` — GREEN (154/154 pass, 70.4% coverage) after test update.

### Design notes

**Empty credentials file (0 bytes) treated as NOT authed:** The `info.Size() > 0` check means a zero-byte `.credentials.json` is treated as absent and the full auth flow runs. This matches the spec's intent: a partial/corrupt write should not block re-auth.

**TTY guard now only fires when auth is actually needed:** Non-TTY callers with existing creds get a clean no-op at the already-authed check, not a TTY error. This is the correct behavior for `account switch` in headless contexts.

**`loginClaudeAccount` unchanged:** The wipe + force-fresh is correct by design for explicit `valv account login` invocations. All existing `loginClaudeAccount` tests pass unchanged.

**Test count delta:** +4 tests added, 1 removed = net +3 (151 → 154).

### Acceptance criteria check

| # | Criterion | Result |
|---|---|---|
| AC1 | `ensureClaudeAccountReady` order: SkipLogin → already-authed → TTY → notice → RunSetupToken → ... | PASS — verified by code and tests |
| AC2 | `loginClaudeAccount` unchanged; wipe still present | PASS — loginClaudeAccount not modified |
| AC3 | `mage testPkg ./internal/cli` green, coverage ≥70% | PASS — 154/154, 70.4% |
| AC4 | `TestEnsureClaudeAccountReadySkipsWhenAlreadyAuthed` added and passes | PASS |
| AC5 | `TestEnsureClaudeAccountReadyAuthsWhenCredentialsMissing` added and passes | PASS |
| AC6 | All other tests in `internal/cli` continue to pass | PASS — 150/150 prior passing tests all pass |

### Unknowns

None — all items resolved.

---

## Unit 7.1 — Round 3

**Date:** 2026-05-15
**State at start:** done (Round 3 is a design-pivot fix; state stays done)

### Change summary

Design pivot from `setup-token` + JSON-wrap to `auth login` + verbatim keychain write. The smoke test 2026-05-15 confirmed that `setup-token` produces `user:inference`-scoped tokens (insufficient for interactive container `claude` sessions), and wrapping the keychain blob in `{"claudeAiAccessToken":"<blob>"}` breaks container claude's native `.credentials.json` parsing.

### Files touched

- `internal/cli/claude_auth.go`:
  - Renamed interface method `RunSetupToken` → `RunAuthLogin`.
  - Updated `systemClaudeAccountAuthRunner.RunAuthLogin` to pass args `"auth", "login"` instead of `"setup-token"`.
  - Deleted `claudeCredentials` struct (was only used by `writeClaudeCredentials` for JSON-wrapping).
  - Removed `encoding/json` import (no longer needed).
  - Rewrote `writeClaudeCredentials(homePath, credentialsBlob string)`: now a single `os.WriteFile(path, []byte(credentialsBlob), 0o600)` — no `json.Marshal`, no struct, verbatim write.
  - Updated doc comments on `claudeKeychainService`, `RunAuthLogin`, `ExtractKeychainToken`, `ensureClaudeAccountReady`, `loginClaudeAccount`, `writeClaudeCredentials` to reflect `auth login` and "full-scope credentials JSON blob" semantics.
  - Updated all `"setup-token"` references in error messages to `"auth login"`.
  - Updated all `runner.RunSetupToken` call sites in `ensureClaudeAccountReady` and `loginClaudeAccount` to `runner.RunAuthLogin`.

- `internal/cli/claude_auth_test.go`:
  - Renamed stub method `RunSetupToken` → `RunAuthLogin` (implementing updated interface).
  - Updated `installFakeHostClaude` script: now checks `[ "${1:-}" = "auth" ] && [ "${2:-}" = "login" ]` instead of `"setup-token"`.
  - Renamed `TestSystemClaudeAccountAuthRunnerRunSetupTokenUsesCLAUDE_CONFIG_DIR` → `TestSystemClaudeAccountAuthRunnerRunAuthLoginUsesCLAUDE_CONFIG_DIR`. Updated the call from `RunSetupToken` to `RunAuthLogin`. Updated the log assertion from `"args:setup-token"` to `"args:auth login"`.
  - Updated `TestRunClaudeHostCommandPreflightFailsWhenClaudeMissing`: arg changed from `"setup-token"` to `"auth", "login"` (cosmetic — test fails at `exec.LookPath` before args matter).

### Mage targets run and result

- `mage testPkg ./internal/cli` — GREEN (154/154 pass, 70.4% coverage).

### Design notes

**Verbatim write rationale:** The macOS keychain stores the full session JSON blob (e.g. `{"accessToken":"...","refreshToken":"...","expiresAt":"..."}`) as a single string. Container claude on Linux reads `.credentials.json` natively — it expects exactly this format. Wrapping it in a second JSON envelope (`{"claudeAiAccessToken":"<blob>"}`) broke parsing. Writing verbatim eliminates the wrapping entirely.

**`ReadAccountIdentity` compatibility:** Confirmed via Hylla that `claudeprovider.ReadAccountIdentity` only checks `.credentials.json` file existence (not content shape). Verbatim write still yields `LoggedIn=true`. No changes needed to the adapter.

**Test fixture compatibility:** The test stub's `extractToken` field (type `string`) now notionally returns a JSON blob. The test `TestLoginClaudeAccountWipesAndRunsSetupTokenAndExtractsAndWrites` uses `extractToken: "fresh-token"` — a bare string, not a real JSON blob. The `strings.Contains(data, "fresh-token")` assertion still passes since the file content IS `"fresh-token"` verbatim. This is acceptable: the stub isolates the write path; live keychain integration is a smoke-test concern.

**No logging of credentials:** The `writeClaudeCredentials` function receives the blob and writes it to disk. The blob is never passed to any logger. Error messages reference the path, not the content.

### Acceptance criteria check

| # | Criterion | Result |
|---|---|---|
| AC1 | `mage testPkg ./internal/cli` green, coverage ≥70% | PASS — 154/154, 70.4% |
| AC2 | No `setup-token` in production code paths | PASS — deleted from args, interface, error messages, doc comments |
| AC3 | `writeClaudeCredentials` is a single `os.WriteFile` with no `json.Marshal` | PASS — verified in function body |
| AC4 | `claudeCredentials` struct deleted | PASS — struct removed, `encoding/json` import removed |
| AC5 | All call sites updated to `RunAuthLogin` | PASS — both `ensureClaudeAccountReady` and `loginClaudeAccount` |

### Unknowns

None.

---

## Unit 7.2 — Round 2

**Date:** 2026-05-15
**State at start:** done (Round 2 is a follow-on to Unit 7.1 Round 3 design pivot; state stays done)

### Change summary

Deleted the `readClaudeAuthToken` helper and `CLAUDE_CODE_OAUTH_TOKEN` env-var injection from `service.go`. With `.credentials.json` now written verbatim in the correct format, container claude reads it natively via the bind-mount set up by `clauderuntime.PrepareRuntime` (DROP_5). The env-var injection was a workaround for the wrong file format — no longer needed.

### Files touched

- `internal/services/claude/service.go`:
  - Removed `encoding/json` import.
  - Deleted `readClaudeAuthToken` function (~17 LOC + doc comment).
  - Deleted the env-injection block in `buildRequest` (~9 LOC including the `// Inject CLAUDE_CODE_OAUTH_TOKEN ...` comment).
  - `os` and `path/filepath` imports retained — both still used elsewhere (`os.TempDir()` in `New`; `filepath.Rel`, `filepath.Separator` in `withinProjectRoot` and `containerName`).

- `internal/services/claude/service_test.go`:
  - Removed `"os"` and `"path/filepath"` imports (now unused after test deletion).
  - Deleted `TestRunSetsClaudeCodeOAuthTokenWhenCredentialsPresent` (tested env-var injection with valid creds).
  - Deleted `TestRunOmitsClaudeCodeOAuthTokenWhenCredentialsMissing` (tested graceful-skip when creds absent).
  - Deleted `TestRunOmitsClaudeCodeOAuthTokenWhenCredentialsMalformed` (tested graceful-skip on bad JSON).

### Mage targets run and result

- `mage testPkg ./internal/services/claude` — GREEN (17/17 pass, 81.0% coverage). Down from 20 tests (3 deleted); coverage 81.0% vs prior 82.3% — well above the 70% floor.
- `mage testPkg ./internal/adapters/providers/claude` — GREEN (21/21 pass, 78.4% coverage). Unchanged.

### Design notes

**Bind-mount mechanism confirmed:** `buildRequest` includes the profile home in `prepared.Mounts` via `clauderuntime.PrepareRuntime`. `PrepareRuntime` mounts `<managed-home>` → `/home/valv/.claude` in the container. `TestRunSucceedsWithBoundProject` verifies this mount exists. The mount predates DROP_7 (DROP_5 added it); no changes needed here.

**Coverage delta is acceptable:** 82.3% → 81.0% (−3 tests each covering the deleted behavior). The remaining 17 tests cover all retained behavior. Coverage stays above the 70% AGENTS.md floor.

### Acceptance criteria check

| # | Criterion | Result |
|---|---|---|
| AC1 | `readClaudeAuthToken` deleted from `service.go` | PASS |
| AC2 | No `CLAUDE_CODE_OAUTH_TOKEN` references in production code | PASS |
| AC3 | `encoding/json` import removed from `service.go` | PASS |
| AC4 | Three deleted test functions removed from `service_test.go` | PASS |
| AC5 | `mage testPkg ./internal/services/claude` green, coverage ≥70% | PASS — 17/17, 81.0% |
| AC6 | `mage testPkg ./internal/adapters/providers/claude` green | PASS — 21/21, 78.4% |

### Unknowns

None.

## Hylla Feedback

- **Query:** `hylla_search_keyword` for `ReadAccountIdentity` in the claude adapter — found the function and confirmed its summary states "LoggedIn is set iff .credentials.json exists and is not a directory." This directly confirmed verbatim write compatibility. Hylla answered correctly on first query.
- No other misses.
