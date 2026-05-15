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
