# DROP_7 — HOST AUTH TOKEN FIX

**State:** building
**Blocked by:** DROP_6 (done)
**Paths (expected):** `internal/cli/claude_auth.go` (rewrite — replace container-launch path with host-subprocess `claude setup-token` runner; add keychain-extract step), `internal/cli/claude_auth_test.go` (rewrite tests for the new flow), `internal/services/claude/service.go` (edit — read stored token from managed home, set `CLAUDE_CODE_OAUTH_TOKEN` env on container launch), `internal/services/claude/service_test.go` (edit — verify env-var threading), `internal/adapters/providers/claude/account.go` (edit — `ReadAccountIdentity` recognizes the stored-token file as the `LoggedIn` signal; keep `.claude.json` email extraction as-is), `internal/adapters/providers/claude/account_test.go` (edit), `internal/cli/preflight.go` or similar (new pre-flight check that `claude` CLI is on host PATH — mirrors how Codex requires `codex` on PATH).
**Packages (expected):** `internal/cli` (rewrite + edits), `internal/services/claude` (edit), `internal/adapters/providers/claude` (edit).
**PLAN.md ref:** main/PLAN.md → DROP_7_HOST_AUTH_TOKEN_FIX row
**Workflow:** main/drops/WORKFLOW.md
**Started:** 2026-05-15
**Closed:** —

## Scope

**Codex parity for Claude auth.** DROP_6.2's container-side `claude auth login` flow has invisible paste prompt (TUI mode doesn't render through Docker pty). User verified on 2026-05-15 that host-side `claude setup-token` works perfectly: browser auto-opens, paste prompt clearly visible, completes cleanly. This drop replaces the container-auth flow with host-subprocess auth mirroring Codex's existing pattern.

**Design summary:**
- `valv account add claude <name>` runs `claude setup-token` as a host subprocess with `CLAUDE_CONFIG_DIR=<managed-home>` env set (structural mirror of Codex's `systemCodexAccountAuthRunner.Login` which runs `codex login` with `CODEX_HOME=<managed-home>`).
- Host browser opens automatically (claude CLI's `open` works because we're on host, not container).
- User completes auth in browser, pastes code back into terminal — paste prompt is visible (verified 2026-05-15).
- `claude setup-token` writes credentials to macOS keychain (`Claude Code-credentials` service, account = macOS username) — same behavior as `claude auth login`. `CLAUDE_CONFIG_DIR` only controls config/state location, NOT credentials on macOS (verified 2026-05-15 — `.credentials.json` did not appear in the test dir).
- **Immediately after the auth subprocess exits**, Valv extracts the token from keychain via `security find-generic-password -s "Claude Code-credentials" -a "$(id -un)" -w` and writes it to `<managed-home>/.credentials.json` as a JSON file (single-key `{"claudeAiAccessToken": "<token>"}` — matches the format DROP_5's `.credentials.json` presence-check assumed).
- Multi-account isolation: each `valv account add claude <name>` does auth → extract → store in a sequence, BEFORE the next auth would overwrite the keychain entry. Per-Valv-account `.credentials.json` files survive on disk independent of the keychain.
- `valv claude` launch (`services/claude/service.go::Run`) reads `<managed-home>/.credentials.json`, extracts the token, sets `CLAUDE_CODE_OAUTH_TOKEN=<token>` env var on the container.
- Pre-flight check: at `account add claude` time, verify `claude` is on host PATH; if not, print remediation `npm install -g @anthropic-ai/claude-code@2.1.89`. Mirrors how Codex's preflight requires `codex` on PATH.

**Removes from DROP_6.2:** the container-launch code path (the `valv-claude:dev` container running `claude auth login`). The new host-subprocess approach replaces it entirely. `claude_auth.go` is heavily reworked, NOT extended.

**Keeps from DROP_6.3:** `.claude.json` parsing for email extraction in `ReadAccountIdentity`. Host `claude setup-token` still writes `.claude.json` with `oauthAccount.emailAddress` to `$CLAUDE_CONFIG_DIR` — that data is still useful for `account list` display.

## Dev-Confirmed Findings (2026-05-15)

1. **Host-side `claude auth login` works correctly** with `CLAUDE_CONFIG_DIR` set: browser opens, "Paste code here if prompted >" prompt is visible, `Login successful.` printed.
2. **`CLAUDE_CONFIG_DIR` does NOT redirect credentials on macOS.** Credentials land in keychain regardless. `.claude.json` (identity + state, NO tokens) is written to the env-var-set dir.
3. **Keychain confirmed**: `Claude Code-credentials` service, account = macOS username (`evanschultz`), class `genp`. Extractable via `security find-generic-password -s "Claude Code-credentials" -a "$USER" -w`.
4. **Per-OS-user keychain scope**: one claude credential per macOS user — overwrites on each `claude auth login`. Multi-account isolation requires Valv to extract+store between auths.
5. **`claude setup-token` is the better UX**: ASCII art welcome, browser opens, URL fallback printed, paste prompt visible. Long-lived headless token with `scope=user:inference`. Anthropic's blessed headless path per focus-plan §7.
6. **claudebox (https://github.com/RchGrav/claudebox) uses in-container auth**: bind-mounts `.claude/` + `.claude.json` per slot, uses `-it` only if TTY, runs `claude` (no subcommand) and lets it auto-prompt. We are NOT following that approach — host-side auth gives cleaner Codex parity.

## Open Design Questions Routed To Planner

- **Q1 — What does `claude setup-token` actually print to stdout after the user pastes the code?** Possibilities: (a) just `Success` with token written to keychain only — extract via `security`; (b) prints `Token: ...` on stdout — capture there too; (c) writes to a file in `$CLAUDE_CONFIG_DIR`. The planner should NOT assume; the builder will resolve via real run during build verification. Per the 2026-05-15 test pattern, prefer keychain extraction as the primary capture method since we know it ends up there regardless.
- **Q2 — `setup-token` vs `auth login`?** `setup-token` produces a long-lived scoped token (`user:inference`); `auth login` produces a regular session token. For Valv's "per-account persistent credential in managed home" pattern, the long-lived token is the right choice. Confirm with the builder that `CLAUDE_CODE_OAUTH_TOKEN=<setup-token-output>` works to authenticate `claude` invocations inside the container.
- **Q3 — `CLAUDE_CODE_OAUTH_TOKEN` env-var visibility in `ps`.** Passing the token via env var makes it visible to anyone who can `ps eww` the container's process. Acceptable for v0.1.0 (same constraint Codex has with `CODEX_HOME` mount). Document.
- **Q4 — What service name does `claude setup-token` use in keychain?** Probably the same `Claude Code-credentials` as `auth login` but the planner should verify by having the builder check post-auth keychain state.

## Planner

Three atomic units. Units 7.1 and 7.2 may run in parallel (disjoint packages). Unit 7.3 is blocked by both.

### Design decisions locked by planner

- **Interface shape:** `claudeAuthRunner` is redefined with two methods — `RunSetupToken(ctx, homePath, stdin, stdout, stderr) error` and `ExtractKeychainToken(ctx, macOSUser) (string, error)`. The old `EnsureImage` / `RunContainer` methods are gone. Tests get a new stub.
- **Preflight inline:** `exec.LookPath("claude")` lives inside `runClaudeHostCommand` — same pattern as Codex's `exec.LookPath("codex")` in `runCodexHostCommand`. No separate preflight function.
- **Keychain user:** `os/user.Current().Username` — NOT `os.Getenv("USER")`. Error from `user.Current()` propagates immediately.
- **`CLAUDE_CONFIG_DIR` env for setup-token:** Set to `homePath` (managed home) so `.claude.json` identity data lands in the right place. Does NOT redirect keychain.
- **Token extraction timing:** Immediately after `RunSetupToken` returns nil. Single `security find-generic-password -s "Claude Code-credentials" -a <username> -w` call. If the builder discovers `setup-token` uses a different service name, they update the constant and document the actual name in the worklog.
- **`.credentials.json` format we write:** `{"claudeAiAccessToken":"<raw-token>"}` — single-key JSON. The service reads this key; `ReadAccountIdentity` only stat-checks the file, so the format is irrelevant to LoggedIn.
- **Service graceful skip:** If `.credentials.json` is absent or unreadable during `buildRequest`, log a debug message and omit `CLAUDE_CODE_OAUTH_TOKEN` from env. Do NOT fail `Run`. This preserves backward compatibility with manually-authed accounts.
- **Logout:** `wipeClaudeCredentials` stays as file-wipe only. Do NOT call `security delete-generic-password` — too destructive to user's host claude sessions.
- **`loginClaudeAccount`:** Keeps its no-TTY-guard semantics (explicit re-login, matches Codex's `loginCodexAccount`). Updates to new runner interface.
- **Q4 accepted Unknown:** Builder must verify and document which keychain service name `claude setup-token` actually writes (expected: `Claude Code-credentials`).
- **7.1 LOC exception:** `claude_auth.go` is a full-file rewrite (~160-180 LOC), exceeding the 80-120 soft target. Justified: splitting the interface change from its callers in the same file would leave the package uncompilable between units.

---

### Unit 7.1 — Rewrite `claude_auth.go`: new host-subprocess runner + reworked orchestration

**state:** in_progress
**blocked_by:** —
**paths:** `internal/cli/claude_auth.go`
**packages:** `internal/cli`

**What to build:**

Rewrite `claude_auth.go` in full. Keep the file; replace its entire content. Do NOT extend the existing interface — redefine it.

New `claudeAuthRunner` interface (replaces the old EnsureImage/RunContainer interface):

```go
type claudeAuthRunner interface {
    RunSetupToken(ctx context.Context, homePath string, stdin io.Reader, stdout, stderr io.Writer) error
    ExtractKeychainToken(ctx context.Context, macOSUser string) (string, error)
}
```

New `systemClaudeAccountAuthRunner` struct (the production implementation):

- `RunSetupToken`: calls `runClaudeHostCommand(ctx, homePath, stdin, stdout, stderr, "setup-token")`. Mirror of `systemCodexAccountAuthRunner.Login`.
- `ExtractKeychainToken`: calls `runClaudeHostCommand(ctx, "", nil, nil, nil, "extract", macOSUser)` — but NOT via `runClaudeHostCommand` (that's for the `claude` binary). Instead: use `exec.CommandContext(ctx, "security", "find-generic-password", "-s", claudeKeychainService, "-a", macOSUser, "-w")` directly, capturing stdout. Trim the output. Return error if exit non-zero. `claudeKeychainService` is a package-level constant set to `"Claude Code-credentials"` (builder verifies and updates if `setup-token` uses a different service name).

New `runClaudeHostCommand(ctx, homePath, stdin, stdout, stderr io.Writer, args ...string) (string, error)`:
- Mirror of `runCodexHostCommand`. `exec.LookPath("claude")` for the binary. `appendOrReplaceEnv(os.Environ(), "CLAUDE_CONFIG_DIR", homePath)`. If homePath is empty (for keychain extraction calls) — wait, `ExtractKeychainToken` does NOT call `runClaudeHostCommand` (it calls `security` directly). So `runClaudeHostCommand` always has a non-empty homePath.

New `claudeAuthRunnerFromContext(ctx, cmd)`: same pattern as `codexAccountAuthFromContext` — reads from context via `claudeAuthRunnerKey{}`, falls back to `hostClaudeAccountAuth` (a `var` holding `systemClaudeAccountAuthRunner{}`).

New `ensureClaudeAccountReady(cmd, account, options)`:
1. `options.SkipLogin` → return nil immediately (no wipe, no auth).
2. Non-TTY guard: `!commandHasTTY(cmd.InOrStdin())` → return actionable error. Fires BEFORE wipe (preserves existing creds if non-TTY call).
3. Pre-flight: `exec.LookPath("claude")` → if err, return: `"claude CLI not found on PATH; install with: npm install -g @anthropic-ai/claude-code@2.1.89"`.
4. Wipe: `wipeClaudeCredentials(account.HomePath)`.
5. `writeCLINotice(...)` announce auth.
6. `runner.RunSetupToken(ctx, account.HomePath, stdin, stdout, stderr)` — streams to TTY.
7. `user.Current()` → extract macOS username. Error → return.
8. `token, err := runner.ExtractKeychainToken(ctx, username)` → error → return.
9. Write `{"claudeAiAccessToken": "<token>"}` as JSON to `filepath.Join(account.HomePath, ".credentials.json")` with mode 0o600.
10. `claudeprovider.ReadAccountIdentity(account.HomePath)` → verify `identity.LoggedIn`. If false → return error.

New `loginClaudeAccount(cmd, account, paths)`:
- Same as `ensureClaudeAccountReady` but WITHOUT the non-TTY guard and WITHOUT the preflight check (same no-guard semantics as `loginCodexAccount`). Steps 4-10 above.

Keep `wipeClaudeCredentials` unchanged. Delete `buildClaudeAuthContainerRequest` and all Docker import usage.

**Imports lost:** `dockeradapter`, `claudeprovider` from the `EnsureImage` side — but `claudeprovider` is still used for `ReadAccountIdentity` + `ContainerClaudeDir`? Check: `buildClaudeAuthContainerRequest` used `claudeprovider.ContainerClaudeDir` and `claudeprovider.ContainerHomeDir`. Those are gone. `ReadAccountIdentity` is still called in steps 10. So `claudeprovider` import stays. The docker import is dropped.

New imports needed: `encoding/json`, `os/user`.

**`appendOrReplaceEnv` stays** — it is in `account_auth.go`, shared across the package. No move needed.

**Acceptance criteria:**
- AC1: `mage testPkg github.com/evanmschultz/valv/internal/cli` passes (all existing non-Claude tests still pass; new runner compiles).
- AC2: `claudeAuthRunner` interface has exactly two methods: `RunSetupToken` and `ExtractKeychainToken`. Verified by inspection.
- AC3: `ensureClaudeAccountReady` SkipLogin returns nil without touching the filesystem (same invariant as before).
- AC4: `ensureClaudeAccountReady` non-TTY guard fires BEFORE `wipeClaudeCredentials`. Verified by test: call with non-TTY + pre-existing creds file → file still present after error return.
- AC5: `runClaudeHostCommand` sets `CLAUDE_CONFIG_DIR=<homePath>` in the subprocess environment. Verified by fake `claude` script test similar to `TestSystemCodexAccountAuthRunnerLoginUsesCODEXHOME`.
- AC6: `ExtractKeychainToken` propagates `os/user.Current()` errors (verified by test with a stub that returns error from that method).
- AC7: No Docker adapter import remains in `claude_auth.go`.

**Verification target:** `mage testPkg github.com/evanmschultz/valv/internal/cli`

---

### Unit 7.2 — Service-layer `CLAUDE_CODE_OAUTH_TOKEN` env propagation

**state:** done
**blocked_by:** —
**paths:** `internal/services/claude/service.go`
**packages:** `internal/services/claude`

**What to build:**

Edit `services/claude/service.go`. Add token injection into `buildRequest`.

After the `docker.ContainerRunRequest{...}` literal is constructed (currently line 253-274), add:

```go
if token, err := readClaudeAuthToken(profile.HomePath); err != nil {
    s.debug("claude auth token unreadable", "home", profile.HomePath, "err", err)
} else if token != "" {
    request.Env["CLAUDE_CODE_OAUTH_TOKEN"] = token
}
```

New unexported helper `readClaudeAuthToken(homePath string) (string, error)` in `service.go`:
- Opens `filepath.Join(homePath, ".credentials.json")`.
- Unmarshals into `struct { Token string \`json:"claudeAiAccessToken"\` }`.
- Returns `Token` (may be empty string if key absent). Returns error only on read/parse failure.
- If file not found (`os.IsNotExist`): return `"", nil` (not an error — silent skip).

This is ~30 LOC of production code.

No new imports (already imports `encoding/json`, `os`, `path/filepath`). Verify via `go.mod` that these are available — they are stdlib.

The `Env` map in `buildRequest` is initialized from `prepared.Env` (line 257). `prepared.Env` is a `map[string]string`. Adding to it directly is fine since `buildRequest` is the exclusive constructor.

**`CLAUDE_CODE_OAUTH_TOKEN` env var in `ps` visibility:** Add comment: `// CLAUDE_CODE_OAUTH_TOKEN is passed as an environment variable and is visible to ps(1). Acceptable for v0.1.0.`

**Acceptance criteria:**
- AC1: `mage testPkg github.com/evanmschultz/valv/internal/services/claude` passes.
- AC2: When a profile home contains a valid `.credentials.json` with `claudeAiAccessToken`, `Run` produces a `ContainerRunRequest` with `Env["CLAUDE_CODE_OAUTH_TOKEN"] == <token>`. Verified by new test case in `service_test.go`.
- AC3: When `.credentials.json` is absent, `Run` succeeds and `Env["CLAUDE_CODE_OAUTH_TOKEN"]` is not set. Verified by existing `TestRunSucceedsWithBoundProject` (profile home is `t.TempDir()` with no creds file) — must still pass unchanged.
- AC4: When `.credentials.json` is present but unreadable (e.g. bad JSON), `Run` succeeds (graceful skip) and logs a debug message. Token env var omitted.
- AC5: `readClaudeAuthToken` is unexported. Verified by inspection.

**Verification target:** `mage testPkg github.com/evanmschultz/valv/internal/services/claude`

---

### Unit 7.3 — Tests: rewrite Claude auth tests + service env propagation tests

**state:** todo
**blocked_by:** 7.1, 7.2
**paths:** `internal/cli/claude_auth_test.go`, `internal/services/claude/service_test.go`, `internal/adapters/providers/claude/account_test.go`
**packages:** `internal/cli`, `internal/services/claude`, `internal/adapters/providers/claude`

**What to build:**

**`claude_auth_test.go` — full rewrite:**

New `stubClaudeAccountAuthRunner` struct matching the new interface:
```go
type stubClaudeAccountAuthRunner struct {
    setupTokenErr   error
    setupTokenHits  int
    extractTokenErr error
    extractToken    string // returned by ExtractKeychainToken
    extractHits     int
}
func (s *stubClaudeAccountAuthRunner) RunSetupToken(_ context.Context, _ string, _, _, _ io.Writer) error { ... }
func (s *stubClaudeAccountAuthRunner) ExtractKeychainToken(_ context.Context, _ string) (string, error) { ... }
```

New `installStubClaudeAccountAuth(t, cmd, stub)`: injects via `claudeAuthRunnerKey{}` context value.

New `newTestClaudeCmd()`: keep as-is (minimal cobra.Command with bytes buffers — non-TTY).

Test cases for `ensureClaudeAccountReady`:
- `TestEnsureClaudeAccountReadyRejectsNonTTY` — non-TTY → error containing "TTY"; no `RunSetupToken` hit.
- `TestEnsureClaudeAccountReadyRespectsSkipLogin` — SkipLogin=true → nil; no wipe; no `RunSetupToken` hit; pre-existing creds file preserved.
- `TestEnsureClaudeAccountReadyNonTTYDoesNotWipe` — non-TTY + pre-existing creds → error; file preserved.
- `TestEnsureClaudeAccountReadyRunsSetupTokenAndWritesCreds` — stub returns token "test-token"; function writes `.credentials.json`; verify `LoggedIn=true` from `ReadAccountIdentity`.
- `TestEnsureClaudeAccountReadyFailsWhenSetupTokenFails` — `RunSetupToken` returns error; function returns error; no creds written.
- `TestEnsureClaudeAccountReadyFailsWhenExtractFails` — `RunSetupToken` succeeds; `ExtractKeychainToken` returns error; function returns error.
- `TestEnsureClaudeAccountReadyFailsWhenNoCredsAfterWrite` — stub returns empty token ""; `json.Marshal` produces `{"claudeAiAccessToken":""}` but file exists — `LoggedIn=true`. Hmm. Actually if the token is empty we should fail before writing. Builder: return error if extracted token is empty string (treat empty as extraction failure).

Test cases for `loginClaudeAccount`:
- `TestLoginClaudeAccountProceedsWithoutTTYGuard` — non-TTY cmd, stub returns token "tok"; succeeds; `RunSetupToken` hit = 1.
- `TestLoginClaudeAccountFailsWhenExtractFails` — extraction fails; returns error.

Test for system runner (fake `claude` binary):
- `TestSystemClaudeAccountAuthRunnerRunSetupTokenUsesCLAUDE_CONFIG_DIR` — install fake `claude` binary that logs env; call `systemClaudeAccountAuthRunner{}.RunSetupToken(ctx, "/tmp/home", ...)`; verify log contains `CLAUDE_CONFIG_DIR=/tmp/home`. Mirror of `TestSystemCodexAccountAuthRunnerLoginUsesCODEXHOME`.
- `TestRunClaudeHostCommandFailsWhenClaudeMissing` — PATH cleared; verify error contains "claude" and is actionable.

`installFakeHostClaude(t, mode)`: mirror of `installFakeHostCodex`. Script logs args + `CLAUDE_CONFIG_DIR`.

**`service_test.go` — add two test cases (append to existing file):**

- `TestRunSetsClaudeCodeOAuthTokenWhenCredentialsPresent`: write `{"claudeAiAccessToken":"svc-token"}` to profile home before `service.Run`; verify `executor.got.Env["CLAUDE_CODE_OAUTH_TOKEN"] == "svc-token"`.
- `TestRunOmitsClaudeCodeOAuthTokenWhenCredentialsMissing`: profile home is empty `t.TempDir()`; `Run` succeeds; `executor.got.Env["CLAUDE_CODE_OAUTH_TOKEN"]` is `""` (zero value for absent key). Existing `TestRunSucceedsWithBoundProject` already covers this case; add an explicit assertion to it OR add this as a separate test.

**`account_test.go` — no changes.** Fixtures use `{"claudeAiOauth":{"accessToken":"tok"}}` format; `ReadAccountIdentity` only stat-checks presence — tests pass as-is. Builder must NOT alter fixtures.

**Acceptance criteria:**
- AC1: `mage testPkg github.com/evanmschultz/valv/internal/cli` passes with the new stub and all new test cases green.
- AC2: `mage testPkg github.com/evanmschultz/valv/internal/services/claude` passes with the two new service test cases.
- AC3: `mage testPkg github.com/evanmschultz/valv/internal/adapters/providers/claude` passes unchanged.
- AC4: `mage test` passes (full suite, race detector, 70% coverage floor).
- AC5: Old test names `TestEnsureClaudeAccountReadyRejectsNonTTY`, `TestBuildClaudeAuthContainerRequestShape`, `TestLoginClaudeAccountSkipsNonTTYGuard`, `TestLoginClaudeAccountFailsWhenNoCredsAfterContainer` are REPLACED (not just augmented) — the old names belonged to the container-based flow and must not appear in the final test file.

**Verification target:** `mage testPkg github.com/evanmschultz/valv/internal/cli` then `mage testPkg github.com/evanmschultz/valv/internal/services/claude` then `mage testPkg github.com/evanmschultz/valv/internal/adapters/providers/claude` then `mage test`

## Notes

- **Mechanical drop — trimmed cascade applies** per memory `feedback_trimmed_cascade_for_mechanical_drops.md`. Single planner spawn. Per-unit build-QA stays. The "novel" part is keychain extraction, which is shell-out + parse — no concurrent/async/state-machine logic.
- **Codex auth template to mirror**: `internal/cli/account_auth.go::systemCodexAccountAuthRunner.Login` runs `codex login` as a host subprocess with `CODEX_HOME=<path>` env. Our new `systemClaudeAccountAuthRunner.Login` should mirror this shape with `claude setup-token` + `CLAUDE_CONFIG_DIR=<path>` env, plus the post-exit keychain-extract step.
- **Test injection pattern**: keep the `claudeAccountAuthRunnerKey` context-key + interface pattern from DROP_6.2. The interface methods need updating from `EnsureImage` / `RunContainer` (container-based) to something like `RunSetupToken(ctx, homePath, stdin/out/err)` + `ExtractKeychainToken(ctx, account)`. Tests inject a stub that simulates both.
- **Code to DELETE in this drop**: the container-launch path in `claude_auth.go` (the `valv-claude:dev` invocation in `ensureClaudeAccountReady`). The image is still needed for `valv claude` LAUNCH, just not for auth.
- **`logoutManagedAccount` Claude case**: keep as file-wipe of `.credentials.json` from DROP_6.2. But also consider clearing the keychain entry — possibly with `security delete-generic-password -s "Claude Code-credentials" -a "$(id -un)"`. Planner should decide whether to do this (defensive cleanup) or leave it (less destructive of user's host-claude state).
- **`.credentials.json` schema**: write as `{"claudeAiAccessToken": "<token>"}` — single-key JSON matching the test fixture at `internal/adapters/providers/claude/account_test.go:14`. The Linux container's claude CLI reads this format. `ReadAccountIdentity` already checks file presence; it doesn't need to parse the JSON to determine LoggedIn — but DROP_6.3 added `.claude.json` parsing for email, which we keep.
- **`CLAUDE_CODE_OAUTH_TOKEN` env at launch**: when `valv claude` launches the container, the existing `services/claude/service.go::Run` builds an env map. Add a step that reads `<managed-home>/.credentials.json`, extracts `claudeAiAccessToken`, sets `CLAUDE_CODE_OAUTH_TOKEN=<value>` in the env map. Container claude will use the env-var auth.
- **Pre-flight host PATH check**: at `valv account add claude` time, before launching the subprocess, verify `claude` is on PATH. If not, error with: `claude CLI not found on PATH. Install with: npm install -g @anthropic-ai/claude-code@2.1.89`.
- **Out of scope**:
  - The container-LAUNCH path (`valv claude` from a bound project) keeps working as DROP_5 designed. We just add the env-var threading.
  - Cross-provider `account switch` semantics, TUI parity, globalswitch — all DROP_8.
  - Codex hardening (force-relogin flag, etc.) — DROP_10 cleanup.
  - GitHub Actions release + brew formula — DROP_9.
