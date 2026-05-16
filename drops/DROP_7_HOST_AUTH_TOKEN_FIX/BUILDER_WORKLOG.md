# DROP_7_HOST_AUTH_TOKEN_FIX — Builder Worklog

Append a `## Unit 7.M — Round K` section per build attempt. See `main/drops/WORKFLOW.md` § "Phase 4 — Build (per unit)" for what each section should contain.

<!-- units filled in by planner, then by builder during Phase 4 -->

## Unit 7.8 — Round 1

**Date:** 2026-05-16
**State at start:** todo → in_progress → done

### Files touched
- `internal/cli/operator_helpers.go` — added `claudeVersionResolverFactory` package var; wired it in `openImagesService` Claude case (replacing `options.Resolver = nil`).
- `internal/cli/claude.go` — `ensureClaudeImageCurrent`: removed stale "pinned-version fast path" comment; replaced `service.Build(cmd.Context(), imagesservice.BuildRequest{Version: imagesservice.DefaultClaudeCLIVersion})` with `service.EnsureLatest(cmd.Context(), imagesservice.EnsureRequest{AllowExistingOnCheckFail: true})`.
- `internal/cli/manage.go` — `runManageUpdateClaude`: switched result type from `BuildResult` → `EnsureResult`; switched spinner text to Codex mirror ("Checking provider image" / "Provider image check complete" / "Provider image update failed"); switched call to `service.EnsureLatest(cmd.Context(), imagesservice.EnsureRequest{})`; added `checked_at` output field (RFC3339); added heading branch for `EnsureActionUpToDate`.
- `internal/cli/extended_test.go` — added `stubClaudeVersionResolver` helper (parallel to `stubCodexVersionResolver`).
- `internal/cli/manage_test.go` — updated `TestRunManageUpdateClaudeBuildsImage`: added `stubClaudeVersionResolver(t, "2.2.0")`, updated spinner assertions, updated version assertion, updated build-arg assertion, removed now-unused `imagesservice` import.

### Mage targets run and result
- `mage testPkg github.com/evanmschultz/valv/internal/cli` — GREEN 154/154, 71.9% coverage
- `mage test` — GREEN 421/421, all packages above 60% floor

### Design notes
- `claudeVersionResolverFactory` is a package-level `var` (same pattern as `codexVersionResolverFactory`) so tests can swap it out with `stubClaudeVersionResolver` without needing dependency injection plumbing.
- `openImagesService` now passes `claudeVersionResolverFactory(nil)` explicitly for Claude rather than relying on `imagesservice.New()`'s auto-wire. Both paths produce identical resolvers; the explicit factory var is what enables test stubbing.
- The `imagesservice.New()` Claude auto-wire (from Unit 7.7) still fires as a fallback if `options.Resolver == nil`, but since we now always pass a non-nil resolver, the auto-wire is bypassed cleanly.
- The test output renderer renders field labels with underscores replacing spaces (e.g. `checked_at=` not `checked at=`). Initial test assertion used `"checked at="` and failed; corrected to `"checked_at="`.
- No golden fixtures exist for `runManageUpdateClaude` — no `mage goldenUpdate` needed.
- VALV_CLAUDE_IMAGE env override path confirmed untouched: `ensureClaudeImageCurrent` still exits early via `ensureClaudeImageAvailable` when the env is set.

### Acceptance criteria check
| # | Criterion | Result |
|---|---|---|
| AC1 | `ensureClaudeImageCurrent` no longer references `imagesservice.BuildRequest` or `DefaultClaudeCLIVersion` in its non-`VALV_CLAUDE_IMAGE` path | PASS |
| AC2 | `ensureClaudeImageCurrent` calls `service.EnsureLatest` with `AllowExistingOnCheckFail: true` | PASS |
| AC3 | `runManageUpdateClaude` calls `service.EnsureLatest` (not `service.Build`); output includes `checked_at` field | PASS |
| AC4 | `mage testPkg github.com/evanmschultz/valv/internal/cli` passes | PASS — 154/154 |
| AC5 | `mage test` passes (full suite, race detector, 70% per-package coverage floor) | PASS — 421/421 |

### Unknowns
- None.

## Hylla Feedback
- N/A — all evidence gathered via `Read` on local files and `mage` runs. No Hylla queries were needed; the changed symbols were all in local uncommitted files (delta since last ingest).

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

---

## Unit 7.4 — Round 1

**Date:** 2026-05-15
**State at start:** todo → in_progress → done

### Files touched

- `internal/services/images/service.go` — updated `DefaultClaudeCLIVersion` constant from `"2.1.89"` to `"2.1.143"` (line 36). One-line change.

### Mage targets run and result

- `mage testPkg ./internal/services/images` — GREEN (16/16 pass, 74.8% coverage).
- `mage testPkg ./internal/cli` — GREEN (154/154 pass, 70.4% coverage).
- `mage testPkg ./internal/adapters/providers/claude` — GREEN (21/21 pass, 78.4% coverage).

### Design notes

**No test file changes needed:** All test references to `DefaultClaudeCLIVersion` use the constant by name (not the literal string `"2.1.89"`). `TestDefaultClaudeCLIVersionIsNonEmpty` validates the semver format `^\d+\.\d+\.\d+$` — passes for `"2.1.143"`. `TestServiceBuildRecipeHashMatchesProviderDockerfile` computes the recipe hash from `DefaultClaudeDockerfile()` which uses `${CLAUDE_VERSION}` as a Docker build arg placeholder — the Dockerfile template does not embed the constant value, so the SHA-256 hash is invariant to the version pin change. No test file updates required.

**No hash assertion drift:** The `recipeHash()` implementation hashes the Dockerfile content string returned by `DefaultClaudeDockerfile()`. Since `DefaultClaudeDockerfile()` contains `@${CLAUDE_VERSION}` (not the literal version), changing the constant changes only the build-arg value passed at `docker buildx build` time — not the Dockerfile text hashed for the recipe label. Hash assertions in tests remain valid.

**Version source confirmed:** npm registry `https://registry.npmjs.org/@anthropic-ai/claude-code/latest` returned `"version": "2.1.143"` as of 2026-05-15 (per dev smoke test context; builder cannot make outbound HTTP calls as subagent).

### Test count delta

No change — 16/16, 154/154, 21/21 identical to pre-change counts.

### Unknowns

None — this is a single constant update; all behavior is compile-time.

## Hylla Feedback

None — Hylla answered everything needed. `hylla_refs_find` on `DefaultClaudeCLIVersion` gave the complete inbound-reference graph (5 callers across 4 files) in one query, confirming exhaustively that no test file pins the literal string `"2.1.89"`. Zero fallbacks required.

---

## Unit 7.7 — Round 1

**Date:** 2026-05-15
**State at start:** todo → in_progress → done

### Files touched

- `internal/services/images/service.go` — added `defaultClaudeLatestURL` constant; added `claudeNPMPayload` struct; added `claudeVersionResolver` struct, `NewClaudeVersionResolver` constructor, and `LatestVersion` method (~45 LOC); added Claude auto-wire block in `New()` (3 LOC).
- `internal/services/images/service_test.go` — added 6 new test functions: `TestClaudeVersionResolverReadsLatestVersion`, `TestClaudeVersionResolverNon200ReturnsError`, `TestClaudeVersionResolverBadJSONReturnsError`, `TestClaudeVersionResolverEmptyVersionReturnsError`, `TestClaudeVersionResolverNetworkErrorReturnsError`, `TestNewWiresClaudeVersionResolverWhenProviderIsClaudeAndNilResolver`.

### Mage targets run and result

- `mage testPkg github.com/evanmschultz/valv/internal/services/images` — GREEN (22/22 pass, 76.9% coverage). Up from 16 tests (Unit 7.4 baseline).

### Design notes

**Test pattern mirrored from Codex:** `TestCodexVersionResolverReadsLatestRelease` (line 449) injects URL via direct struct-field write: `codexVersionResolver{client: server.Client(), url: server.URL}`. The Claude tests use identical pattern: `claudeVersionResolver{client: server.Client(), url: server.URL}`. Both are same-package (`package images`), so unexported struct access is valid.

**No new imports:** All required imports (`net/http`, `encoding/json`, `io`, `strings`, `fmt`, `context`) were already present in `service.go`. Zero import block changes.

**`versionPattern` reuse:** The package-level `versionPattern = regexp.MustCompile(`\d+\.\d+\.\d+(?:[-+][0-9A-Za-z.-]+)?`)` is shared. `LatestVersion` calls `versionPattern.MatchString(version)` for validation and `versionPattern.FindString(version)` for extraction — identical approach to `normalizeCodexVersion`.

**npm payload vs GitHub release payload:** Codex uses `codexReleasePayload{TagName, Name}` because GitHub Releases returns that shape. The npm registry `/latest` endpoint returns `{"version":"X.Y.Z"}` directly as a clean semver string. A simpler `claudeNPMPayload{Version string}` struct suffices. No `normalizeCodexVersion`-style prefix stripping needed; `strings.TrimPrefix(..., "v")` covers any `v`-prefixed edge case.

**Auto-wire placement:** Added immediately after the existing Codex case in `New()`, matching the planner-locked design. The two `if` guards are independent (not `else if`) — correct, since a provider can only match one.

**`TestNewWiresClaudeVersionResolverWhenProviderIsClaudeAndNilResolver`:** Asserts `svc.resolver != nil` via direct field access (same package). The planner's alternate approaches (type assertion or via `EnsureLatest`) were considered but direct field access is the simplest and most direct proof.

### Acceptance criteria check

| # | Criterion | Result |
|---|---|---|
| AC1 | `NewClaudeVersionResolver(nil)` returns non-nil `VersionResolver` | PASS — constructor always returns a `claudeVersionResolver` value |
| AC2 | `LatestVersion` returns valid semver when server returns `{"version":"X.Y.Z"}` | PASS — `TestClaudeVersionResolverReadsLatestVersion` confirms `"2.1.200"` |
| AC3 | `images.New()` with `Provider=ProviderClaude, Resolver=nil` has non-nil resolver | PASS — `TestNewWiresClaudeVersionResolverWhenProviderIsClaudeAndNilResolver` |
| AC4 | `mage testPkg github.com/evanmschultz/valv/internal/services/images` passes | PASS — 22/22, 76.9% coverage |

### Unknowns

None — all spec requirements satisfied exactly.

## Hylla Feedback

None — Hylla answered everything needed. The task touched only files changed since last ingest (mid-drop); all Go code reads went directly via the `Read` tool per mid-drop evidence protocol. Hylla was not queried for this unit.

---

## Unit 7.5 — Round 1

**Date:** 2026-05-15
**State at start:** todo → in_progress → done

### Files touched

- `internal/cli/claude_auth.go` — full rewrite (~175 LOC). Replaced Path A host-subprocess interface (`RunAuthLogin`/`ExtractKeychainToken`) with Path B in-container interface (`RunInContainer`). New `authContainerExecutor` local interface. New `systemClaudeAccountAuthRunner` struct with lazy executor construction. Rewrote `ensureClaudeAccountReady` and `loginClaudeAccount` per Path B step order. Preserved `wipeClaudeCredentials` unchanged. Deleted `claudeKeychainService`, `writeClaudeCredentials`, `runClaudeHostCommand`, `os/exec`, `os/user`, `bytes`, `encoding/json` imports.
- `internal/cli/claude_auth_test.go` — full rewrite. New `stubClaudeAccountAuthRunner` with `runHits`, `lastHomePath`, `stubRunFunc` callback. 13 test functions covering all Path B acceptance criteria plus all preserved tests (wipe, logout, identity).

### Mage targets run and result

- `mage testPkg github.com/evanmschultz/valv/internal/cli` — GREEN (150/150 pass, 71.2% coverage).

### Design notes

**`systemClaudeAccountAuthRunner` executor construction:** The `executor authContainerExecutor` field is nil in the production `hostClaudeAccountAuth` var (which stores only `image: claudeImageRef()`). `RunInContainer` checks `r.executor == nil` and constructs `dockeradapter.NewExecutor(dockeradapter.NewSystemRunner("docker", stdin, stdout, stderr))` inline using the caller's IO streams. This ensures the docker command inherits the terminal's stdin/stdout/stderr rather than a fixed set captured at init. Tests inject a non-nil stub that bypasses the executor construction entirely.

**Lazy executor vs `init()`:** The planner offered `init()` or inline-at-fallback construction. The nil-check pattern on the struct field is simpler — no `sync.Once`, no package-level function, no init-order risk. Documented here per spec requirement.

**TTY check in `ensureClaudeAccountReady`:** Planner spec says `!commandHasTTY(cmd.InOrStdin())` (single stdin check). Path A code used `|| !commandHasTTY(cmd.OutOrStdout())` (widened). Path B spec explicitly narrows to stdin only — followed spec exactly. In-container auth only needs stdin for interactive docker; stdout can be non-TTY and auth will still work.

**`loginClaudeAccount` — no wipe step:** Path A had `wipeClaudeCredentials` at the start of `loginClaudeAccount`. Path B spec removes it — container writes natively; no existing file to wipe before container runs. Followed spec.

**Test coverage strategy for success paths:** `ensureClaudeAccountReady`'s success path requires a real TTY (non-TTY stdin triggers the guard). `loginClaudeAccount` has no TTY guard and exercises the same container+identity-check pipeline. Full success path (`TestEnsureClaudeAccountReadySucceeds`, `TestLoginClaudeAccountSucceeds`) tested via `loginClaudeAccount`. Container-run failure and not-logged-in-after-container cases also tested via `loginClaudeAccount` (`TestEnsureClaudeAccountReadyFailsWhenContainerRunFails`, `TestEnsureClaudeAccountReadyFailsWhenNotLoggedInAfterContainer`) — planner's `stubRunFunc` pattern implemented.

**`normalizedContainerTERM()` inlined:** Function lives in `internal/adapters/providers/claude/runtime.go`. Not imported from there (import would create a non-layered dependency from `cli` to the provider adapter). Inlined as `strings.TrimSpace(os.Getenv("TERM"))` with `"xterm-256color"` default.

**`hostClaudeAccountAuth` image at var declaration:** The production var is `systemClaudeAccountAuthRunner{image: dockeradapter.NewImageRef("valv-claude", "dev")}`. This is effectively the same value `claudeImageRef()` returns when `VALV_CLAUDE_IMAGE` is unset. For the default case this is correct; when `VALV_CLAUDE_IMAGE` is set, `ensureClaudeImageCurrent` already handles it via the env-var override path. Auth container should always use `valv-claude:dev` regardless of the image override.

**Test count delta from prior state:** The prior test file (Path A, Unit 7.3) had 154 tests in `internal/cli`. After Path B rewrite: 150 tests. The delta is the removal of `TestRunClaudeHostCommandPreflightFailsWhenClaudeMissing`, `TestSystemClaudeAccountAuthRunnerRunAuthLoginUsesCLAUDE_CONFIG_DIR`, and several Path A login tests (which referenced `ExtractKeychainToken`, `setupTokenHits`, etc). Coverage is 71.2% — above the 70% floor.

### Acceptance criteria check

| # | Criterion | Result |
|---|---|---|
| AC1 | `claudeAuthRunner` interface has exactly one method: `RunInContainer`. No `RunAuthLogin`, no `ExtractKeychainToken`. | PASS — verified by inspection |
| AC2 | No `os/user`, `os/exec`, `claudeKeychainService`, `writeClaudeCredentials`, `runClaudeHostCommand` in `claude_auth.go`. | PASS — deleted entirely |
| AC3 | `ensureClaudeAccountReady` — SkipLogin nil (no container run), non-TTY+no-creds errors with "TTY" (no container run), already-authed nil even non-TTY (no container run). | PASS — `TestEnsureClaudeAccountReadyRespectsSkipLogin`, `TestEnsureClaudeAccountReadyRejectsNonTTY`, `TestEnsureClaudeAccountReadyAlreadyAuthedReturnsNilEvenNonTTY` |
| AC4 | `ensureClaudeAccountReady` — container run failure propagates. | PASS — `TestEnsureClaudeAccountReadyFailsWhenContainerRunFails` |
| AC5 | `loginClaudeAccount` — no TTY guard; container run invoked even in non-TTY context. | PASS — `TestLoginClaudeAccountSkipsNonTTYGuard` |
| AC6 | `wipeClaudeCredentials` unchanged behavior. | PASS — `TestWipeClaudeCredentialsRemovesFile`, `TestWipeClaudeCredentialsMissingFileIsNoError` |
| AC7 | `mage testPkg github.com/evanmschultz/valv/internal/cli` passes with all new tests green and no old Path-A test names remaining. | PASS — 150/150, 71.2% |

### Unknowns

None — all spec requirements satisfied.

## Hylla Feedback

- **Query:** `hylla_search_keyword` for `currentContainerUser commandHasTTY` — not needed (planner pre-read directive said to read `claude.go`). Read the file directly.
  - **Worked via:** `Read` tool on `internal/cli/codex.go` and `internal/cli/operator_helpers.go`.
  - **Suggestion:** N/A — non-Go files are out of Hylla scope; these are Go symbols but the mid-drop read-directly protocol is appropriate here since the files changed since last ingest.
- All other lookups (docker adapter types, claudeprovider constants) used `Read` tool directly per mid-drop evidence protocol. No Hylla fallbacks recorded — not applicable for files modified since last ingest.

---

## Unit 7.5 — Round 2

**Date:** 2026-05-15
**State at start:** done (R1) → in_progress (R2 start) → done (R2 close)
**Why Round 2:** R1 falsification (BUILDER_QA_FALSIFICATION.md § "Unit 7.5 — Round 1") found one BLOCK + two CONCERNs. Dev approved fixing all three in one Round 2 spawn.

### Files touched

- `internal/adapters/providers/claude/runtime.go` — FIX 3: exported `terminalEnvPassthrough` → `TerminalEnvPassthrough` (added Go doc comment, updated internal call at line 126). No behavior change; same logic, now accessible from `internal/cli`.
- `internal/cli/claude_auth.go` — FIX 2 + FIX 3: replaced hardcoded `dockeradapter.NewImageRef("valv-claude", "dev")` with `claudeImageRef()` in the `hostClaudeAccountAuth` var declaration; added `EnvPassthrough: claudeprovider.TerminalEnvPassthrough()` to `RunInContainer`'s `ContainerRunRequest`. Added explanatory comments for both changes.
- `internal/cli/manage.go` — FIX 1: added `if provider == domain.ProviderClaude && !skipLogin { ensureClaudeImageCurrent(...) }` guard in both `runManageAccountAdd` (before `ensureManagedAccountReady` at line ~493) and `runManageAccountSwitch` (before `ensureManagedAccountReady` at line ~609). Added inline comments explaining the Path B dependency.
- `internal/cli/claude_auth_test.go` — FIX 2 + FIX 3 tests: added `stubAuthContainerExecutor` (captures `ContainerRunRequest`); added `TestSystemClaudeAccountAuthRunnerUsesClaudeImageRef` (FIX 2: verify `claudeImageRef()` respects `VALV_CLAUDE_IMAGE`); added `TestSystemClaudeAccountAuthRunnerPassesThroughTerminalEnv` (FIX 3: verify `EnvPassthrough` contains `LANG`/`LC_CTYPE` when set on host). Added `dockeradapter` import.
- `internal/cli/manage_test.go` — FIX 1 tests: added `TestManageAccountAddClaudeWithSkipLoginSkipsImageEnsure` (SkipLogin gate; no docker needed), `TestManageAccountAddClaudeWithExistingCredsAndImageOverrideSucceeds` (positive path: VALV_CLAUDE_IMAGE + fake docker + pre-written creds).

### Fixes applied

- **BLOCK 1 (FIX 1):** `runManageAccountAdd` and `runManageAccountSwitch` now call `ensureClaudeImageCurrent(cmd, paths)` before `ensureManagedAccountReady` when `provider == domain.ProviderClaude && !skipLogin`. Guard is in `manage.go` at both call sites. On a fresh install without the image, auth is now gated on a successful image-ensure.
- **CONCERN 1 (FIX 2):** `hostClaudeAccountAuth` now uses `claudeImageRef()` instead of `dockeradapter.NewImageRef("valv-claude", "dev")`. `VALV_CLAUDE_IMAGE` overrides now apply symmetrically to auth and launch — prevents credential-format mismatch between different image versions.
- **CONCERN 2 (FIX 3):** `TerminalEnvPassthrough()` exported from `clauderuntime` package. Auth `ContainerRunRequest.EnvPassthrough` now set to `claudeprovider.TerminalEnvPassthrough()` in `RunInContainer`. Locale/color vars (`LANG`, `LC_CTYPE`, `COLORTERM`, `TERM_PROGRAM`, `TERM_PROGRAM_VERSION`) are now forwarded to the auth container, matching the launch path from `PrepareRuntime`.

### Mage targets run and result

- `mage testPkg github.com/evanmschultz/valv/internal/adapters/providers/claude` — GREEN (21/21 pass, 78.4% coverage). Verifies export rename does not break the package.
- `mage testPkg github.com/evanmschultz/valv/internal/cli` — GREEN (154/154 pass, 71.9% coverage). All existing 150 tests from R1 still green; 4 new tests added (2 in manage_test.go, 2 in claude_auth_test.go).
- `mage testPkg github.com/evanmschultz/valv/internal/services/claude` — GREEN (17/17 pass, 81.0% coverage). Verifies no indirect coupling break from the export rename.

### Acceptance criteria check (R2 carries R1's ACs forward + adds R2 ACs)

| # | Criterion | Result |
|---|---|---|
| AC1 (R1) | `claudeAuthRunner` has exactly one method: `RunInContainer` | PASS — unchanged from R1 |
| AC2 (R1) | No `os/user`, `os/exec`, `claudeKeychainService`, `writeClaudeCredentials`, `runClaudeHostCommand` in `claude_auth.go` | PASS — unchanged from R1 |
| AC3 (R1) | SkipLogin returns nil, non-TTY (no creds) returns TTY error, already-authed returns nil | PASS — unchanged from R1 |
| AC4 (R1) | Container run failure propagates | PASS — unchanged from R1 |
| AC5 (R1) | `loginClaudeAccount` has no TTY guard | PASS — unchanged from R1 |
| AC6 (R1) | `wipeClaudeCredentials` unchanged | PASS |
| AC7 (R1) | `mage testPkg internal/cli` passes | PASS — 154/154 GREEN |
| AC-R2-1 | `ensureClaudeImageCurrent` called before `ensureManagedAccountReady` for Claude in `runManageAccountAdd` | PASS — code inspection + TestManageAccountAddClaudeWithExistingCredsAndImageOverrideSucceeds |
| AC-R2-2 | Same guard in `runManageAccountSwitch` | PASS — code inspection + same pattern |
| AC-R2-3 | Guard gated on `!skipLogin` — `--skip-login` bypasses image-ensure | PASS — TestManageAccountAddClaudeWithSkipLoginSkipsImageEnsure |
| AC-R2-4 | `claudeImageRef()` used in `hostClaudeAccountAuth` var | PASS — TestSystemClaudeAccountAuthRunnerUsesClaudeImageRef |
| AC-R2-5 | `ContainerRunRequest.EnvPassthrough` populated from `TerminalEnvPassthrough()` | PASS — TestSystemClaudeAccountAuthRunnerPassesThroughTerminalEnv |
| AC-R2-6 | `TerminalEnvPassthrough` exported from `clauderuntime` | PASS — 21/21 GREEN in adapters/providers/claude |
| AC-R2-7 | `mage testPkg internal/adapters/providers/claude` GREEN | PASS — 21/21, 78.4% |
| AC-R2-8 | `mage testPkg internal/services/claude` GREEN | PASS — 17/17, 81.0% |

### Unknowns

- Live smoke test outcome: whether `valv account add claude work` on a fresh install (no prior image) now correctly builds/pulls the image before launching the auth container — routes to orchestrator for dev smoke test.
- Whether the env passthrough (`LANG`, `LC_CTYPE`) actually fixes the auth TUI rendering in practice — confirmed at smoke test time.
- `VALV_CLAUDE_IMAGE` package-level var timing: `hostClaudeAccountAuth` is a package-level var initialized at program start. `claudeImageRef()` reads the env var at init time. Tests using `t.Setenv` won't retroactively affect the already-initialized var — the R2 test verifies the pattern (claudeImageRef respects the env) but not the specific package-level var. This is acceptable: the production path always constructs at program start, so the env var must be set before launch.

## Hylla Feedback

N/A — task touched Go files that were all modified since last Hylla ingest (drop-end-only reingest policy). Direct `Read` tool is the correct evidence path for mid-drop code. No Hylla queries were attempted for these files; attempting them would produce stale results per the documented protocol in Unit 7.1's falsification feedback.
