# DROP_6 — FORCE OAUTH ACCOUNT ADD

**State:** building
**Blocked by:** DROP_5 (done)
**Paths (expected):** `internal/cli/account_auth.go` (edit — flip `ensureManagedAccountReady` `case domain.ProviderClaude` no-op stub to real `ensureClaudeAccountReady` call; audit `ensureCodexAccountReady` for credential-reuse path), `internal/cli/claude_auth.go` (new — Claude-side container-based device-code auth flow; or fold into `account_auth.go` if small), `internal/adapters/providers/claude/account.go` (edit — bump `ReadAccountIdentity` from presence-only to parse-and-extract email from `.credentials.json`), `internal/adapters/providers/codex/account.go` (audit — verify JWT email extraction is using the actual file in the managed dir, not a cached/inherited identity), `internal/cli/manage.go` and/or `internal/cli/account.go` (edit — `account add` clears any pre-existing creds in target home before launching auth; add `--force-relogin` flag to `account login`), tests alongside each.
**Packages (expected):** `internal/cli` (edits + possible new file), `internal/adapters/providers/claude` (edit), `internal/adapters/providers/codex` (audit + possible edit).
**PLAN.md ref:** main/PLAN.md → DROP_6_FORCE_OAUTH_ACCOUNT_ADD row
**Workflow:** main/drops/WORKFLOW.md
**Started:** 2026-05-15
**Closed:** —

## Scope

**Identity correctness.** Today `valv account add codex hylla` produces an account that decodes to the same Auth0 sub as the host `~/.codex` account (confirmed via JWT inspection 2026-05-15). The bug is that account-add does not force a fresh OAuth flow — it copies / inherits an existing host session into the managed home. This drop fixes that for both providers: `valv account add <provider> <name>` always launches a browser OAuth flow at add-time, requires the user to interactively re-authenticate, and the resulting credentials are guaranteed to be from a fresh login (not a copy of any existing file on the host).

For Claude this is net-new code: an `ensureClaudeAccountReady` container-based device-code flow that mounts the named account home into a `valv-claude:dev` container, runs `claude` with a login-forcing entrypoint, the user sees a device-code URL on stdout, completes auth in browser, credentials get written to the bind-mounted dir, container exits, account is fully authed.

Also: bump Claude `ReadAccountIdentity` from presence-only check to parse-and-extract so `account list` and `account inspect` show real email instead of `(unavailable)`. Add `--force-relogin` flag to `account login` so existing accounts can be re-authed without renaming.

## Dev-Confirmed Decisions And Diagnosis (2026-05-15)

1. **Account-add always forces fresh OAuth.** No silent credential reuse from host or other accounts. Browser must open every time. User must complete auth interactively. Applies to both providers.
2. **`--force-relogin` flag** on `account login` extends the same semantics to existing accounts.
3. **Email must be visible AND correct everywhere** (`account list`, `account inspect`, post-add output). For both providers.
4. **Codex same-identity bug confirmed.** SQL inspection 2026-05-15:
   - 5 rows in `profiles` table; 3 are orphans (`host`, `host-codex`, duplicate `personal`) all pointing at `~/.codex` with no project bindings.
   - The Valv-managed Codex profile `hylla` has home `.../profiles/personal/` (directory name mismatches account name — stale artifact).
   - JWT inspection: `~/.codex/auth.json` and `.../profiles/personal/auth.json` differ byte-for-byte (different `last_refresh` timestamps and token strings) BUT both decode to `email: evan@hylla.io`, `sub: auth0|CiE2WP2jSqixM4aF9uLyrVv7`. Same OAuth identity. Confirms the credential-reuse / seed-from-host bug at `account add` time.
5. **DB orphan cleanup is out of scope for this drop.** The 3 dead profile rows + dir-name-mismatch on `hylla` are cosmetic and can be cleaned up with the existing `valv account cleanup` and `valv account delete` flows after DROP_6 ships, using the fresh `--force-relogin` path. Do not auto-mutate the user's local DB in this drop.

## Open Design Question Routed To Planner

- **Device-code URL UX through Valv's attached Docker subprocess.** Claude's device-code flow prints a URL + code to stdout and waits for browser completion. Valv's `runClaudeImageOnlyCommand` and `services/claude/service.go` `Run` already do TTY-attached pass-through, so this should "just work" — but the planner MUST verify by reading the actual flow in `services/codex/service.go::Run` and `internal/adapters/dockeradapter` to confirm: (a) container stdin/stdout/stderr are wired to host TTY, (b) the user sees the URL in their terminal, (c) the container blocks on the device-code wait (not on TTY read), and (d) when auth completes, the container's claude process detects it and exits, releasing the parent. If any of (a)–(d) is wrong, the planner surfaces it as an issue to be resolved in the planner pass before the builder spawns.

## Planner

### Scope Narrowing (2026-05-15)

After the dev wiped their local Valv DB and re-ran `valv account add codex personal` + `valv account add codex work` against two distinct ChatGPT accounts, `valv account list` showed two distinct emails (`evanmschultz@gmail.com`, `evan@hylla.io`). **This proves the Codex code path is correct as-is** — the prior same-identity observation was 100% stale local artifacts, not an ongoing code bug.

Consequently this drop ships only the Claude work (Units 6.2 + 6.3). Units 6.1 (Codex wipe at `account add`) and 6.4 (`--force-relogin` flag) are **deferred** as defensive hardening; they belong in DROP_9 cleanup or a later drop. Their full specifications below are preserved as-written for the deferred work — do NOT delete them.

### Scope Confirmation

Two active workstreams for this drop:

2. **Claude container-based auth flow** (active) — net-new `ensureClaudeAccountReady` and `loginClaudeAccount` in a new file `internal/cli/claude_auth.go`. Wires the drop sequence: wipe credentials → launch container attached → user sees device-code URL → credentials land in bind-mounted dir → container exits.
3. **Claude credentials parsing** (active) — extend `ReadAccountIdentity` in `internal/adapters/providers/claude/account.go` from presence-only to parse-and-extract, then wire the new data into `readAccountIdentity` in `internal/cli/operator_helpers.go`.

Deferred:

1. ~~Codex credential-reuse fix~~ — deferred. Codex code path is correct; prior issue was stale local state, not a bug.
4. ~~Force-relogin flag~~ — deferred. Useful hardening but not required for v0.1.0 Claude dogfood.

### Committed-State Audit (2026-05-15)

Key findings from direct code reads (no Hylla miss needed — all Go sources read
via the Read tool, Hylla keyword search confirmed alignment):

- `internal/services/manage/service.go::seedProfileConfig` (lines 454–482):
  copies ONLY `config.toml` from the default host home to the new managed home.
  No `auth.json` / credentials file is copied here. The credential-reuse bug
  was a manual dev action during early development, not a code path that runs
  on every `account add`.

- `internal/cli/account_auth.go::ensureCodexAccountReady` (lines 68–105):
  calls `runner.LoginStatus(ctx, account.HomePath)` first; if it returns true,
  returns nil immediately. This is the functional bug for `account add`: if any
  previous action planted a valid token in the managed home, this check silently
  accepts it. Fix: wipe `auth.json` from the managed home BEFORE calling
  `ensureManagedAccountReady` in the `account add` path.

- `internal/cli/manage.go::runManageAccountAdd` (lines 462–527): calls
  `service.CreateProfile` then `ensureManagedAccountReady`. The wipe goes
  between these two calls. `runManageAccountSwitch` (line 600) also calls
  `ensureManagedAccountReady` but must NOT get the wipe — switching to an
  existing account with a valid session should not destroy it.

- `internal/cli/manage.go::runManageAccountLogin` (lines 606–631): calls
  `loginManagedAccount` directly (no status check). The `--force-relogin` flag
  adds a wipe before this call. The Claude no-op at
  `loginManagedAccount`→`case domain.ProviderClaude: return nil`
  (`account_auth.go:62-64`) must be flipped to call `loginClaudeAccount`.

- `internal/cli/account_auth.go`: contains `ensureManagedAccountReady`,
  `logoutManagedAccount`, `loginManagedAccount` dispatchers (lines 35–66).
  All three have `case domain.ProviderClaude: return nil` stubs.
  `ensureManagedAccountReady` stub at line 39-40.
  `logoutManagedAccount` stub at line 50-51.
  `loginManagedAccount` stub at line 62-63.

- `internal/cli/operator_helpers.go::readAccountIdentity` (lines 219–238):
  has `case domain.ProviderCodex:` and a `default:` returning
  `"unknown"` / `"(unavailable)"`. Claude case must be added once
  `ReadAccountIdentity` can extract email.

- `internal/cli/claude.go::runClaudeImageOnlyCommand` (lines 123–161):
  constructs a `docker.ContainerRunRequest` with `Interactive` and `TTY` flags
  wired from `commandHasTTY(cmd.InOrStdin())`. This is the exact pattern to
  reuse for `ensureClaudeAccountReady` — spawn a container, pass through TTY,
  wait for exit.

- TTY-wiring design question: RESOLVED. `SystemRunner.Run` (docker/os_runner.go
  line 46-58) sets `cmd.Stdin = r.Stdin`, `cmd.Stdout = r.Stdout`,
  `cmd.Stderr = r.Stderr`, which ARE the host TTY descriptors when
  `Interactive: true, TTY: true`. Docker's `-it` flags wire the container's
  stdio to the host pseudo-TTY. The container's stdout (the device-code URL)
  is visible in the user's terminal. Docker attached mode blocks until the
  container exits. Clean exit when claude auth completes. All four TTY
  criteria (a)–(d) from the open design question are satisfied. No issue to
  route.

- `internal/adapters/providers/claude/account.go::ReadAccountIdentity`:
  presence-only check on `.credentials.json`. The test at
  `account_test.go:14` uses `{"claudeAiAccessToken":"tok"}`, which reveals the
  credentials file's likely top-level key. However the actual email field name
  is NOT confirmed in any committed file. Builder must examine a real
  `.credentials.json` from a live auth run or Anthropic docs.

### Atomic Units

---

#### Unit 6.1 — Codex account-add credential wipe (DEFERRED 2026-05-15)

**State:** deferred  
**Paths:** `internal/cli/manage.go`, `internal/cli/manage_test.go`  
**Packages:** `internal/cli`  
**Blocked by:** —

**Deferred because:** dev verified clean Codex behavior by wiping local DB and re-doing `account add codex` with distinct ChatGPT accounts → two distinct emails confirmed. No current code bug. This unit's defensive wipe is hardening, not a fix. Revisit in DROP_9 cleanup or as a standalone hardening drop.

**Description:**

In `runManageAccountAdd` (`manage.go:462`), add a credential wipe between the
`CreateProfile` call and the `ensureManagedAccountReady` call. The wipe removes
`auth.json` from `profile.HomePath` if it exists. This guarantees `LoginStatus`
returns false at the next check, forcing a fresh browser login.

Wipe contract:
- Target: `filepath.Join(profile.HomePath, "auth.json")` only.
- Use `os.Remove`. If the file does not exist, treat as success (not an error).
- Do NOT wipe the entire home dir.
- Do NOT touch the host `~/.codex/auth.json` or any other profile's home.

New helper (may be a package-private function in `manage.go` or a new
`credential_wipe.go` in `internal/cli`):

```go
// wipeCodexCredentials removes auth.json from homePath if present.
// A missing file is not an error.
func wipeCodexCredentials(homePath string) error { ... }
```

The helper is called in `runManageAccountAdd` only, not in
`runManageAccountSwitch`.

Tests to add in `manage_test.go`:
- Table-driven test of `runManageAccountAdd` (via stub manage service and stub
  auth runner): verify that when the managed home already contains `auth.json`,
  it is removed before `ensureManagedAccountReady` is called.
- Verify that a missing `auth.json` does not produce an error.
- Verify that `runManageAccountSwitch` does NOT wipe `auth.json`.

**Acceptance:**
- `mage testPkg internal/cli` passes.
- Code review: `runManageAccountAdd` calls wipe helper before
  `ensureManagedAccountReady`; `runManageAccountSwitch` does not.
- A temp-dir test confirming `auth.json` is absent after the wipe path.

---

#### Unit 6.2 — Claude container auth flow

**State:** todo  
**Paths:** `internal/cli/claude_auth.go` (new), `internal/cli/account_auth.go`  
**Packages:** `internal/cli`  
**Blocked by:** —

**Description:**

New file `internal/cli/claude_auth.go` in `package cli`. Contains:

1. `ensureClaudeAccountReady(cmd *cobra.Command, account domain.Profile) error`
   — the Claude-specific auth flow called from `ensureManagedAccountReady`'s
   `case domain.ProviderClaude:` branch. Sequence:
   a. Wipe `.credentials.json` from `account.HomePath` if present (same
      pattern as 6.1 but for the Claude credentials file).
   b. Ensure the Claude image is available (`ensureClaudeImageCurrent`).
   c. Launch a Docker container with the following shape (mirror of
      `runClaudeImageOnlyCommand` but with the account home bind-mounted):
      - `Interactive: commandHasTTY(cmd.InOrStdin())`
      - `TTY: commandHasTTY(cmd.InOrStdin()) && commandHasTTY(cmd.OutOrStdout())`
      - `Init: commandHasTTY(cmd.InOrStdin()) || commandHasTTY(cmd.OutOrStdout())`
      - Mount: `account.HomePath → ContainerClaudeDir` (bind, read-write)
      - Env: `CLAUDE_CONFIG_DIR=ContainerClaudeDir`, `HOME=ContainerHomeDir`
      - Args: builder must determine the correct Claude CLI invocation that
        triggers device-code / OAuth login (not yet known at planning time —
        mark as **new, not yet verified**: likely one of `claude /login`,
        `claude auth login`, or running `claude` with no credentials present).
      - `Remove: true`
   d. After container exits: re-read `account.HomePath` via
      `claude.ReadAccountIdentity` (from `internal/adapters/providers/claude`)
      to verify `.credentials.json` now exists and `LoggedIn == true`.
   e. Return nil on success; error if no credentials file after container exit.
   f. Non-TTY guard: if no TTY, return a clear error message instructing the
      user to rerun in a TTY (mirrors `ensureCodexAccountReady`'s non-TTY
      guard at `account_auth.go:83-85`).

2. `loginClaudeAccount(cmd *cobra.Command, account domain.Profile) error`
   — same flow as `ensureClaudeAccountReady` minus the non-TTY guard (because
   `loginManagedAccount` callers expect to be invoked explicitly). Used by
   `loginManagedAccount`'s Claude case.

3. `claudeAccountAuthRunnerKey` context-key and interface for test injection
   (mirror of `codexAccountAuthRunnerKey` pattern). The injectable interface
   wraps the actual Docker executor calls so tests can stub the container
   launch. Alternatively, use an `VALV_TEST_SKIP_CLAUDE_AUTH_ENV` env-var
   bypass (same pattern as `valvTestSkipHostCodexLoginEnv`) if the interface
   pattern is too large for this droplet. Builder chooses; both are acceptable.

Flip in `account_auth.go`:
- `ensureManagedAccountReady` `case domain.ProviderClaude:` → call
  `ensureClaudeAccountReady(cmd, account)`.
- `loginManagedAccount` `case domain.ProviderClaude:` → call
  `loginClaudeAccount(cmd, account)`.
- `logoutManagedAccount` `case domain.ProviderClaude:` → for now, wipe
  `.credentials.json` from the account home (no container needed; credentials
  are file-resident). This keeps the logout semantics consistent.

**Build-time unknowns (builder must resolve):**
- U1: Exact Claude CLI args that trigger device-code login. Verify by running
  `docker run --rm valv-claude:dev claude --help` and reading the output, or
  by running a live auth flow in the dev container. Document the resolved
  invocation in the BUILDER_WORKLOG.md.
- U2: Where `.credentials.json` lands inside the container. Expected:
  `$CLAUDE_CONFIG_DIR/.credentials.json` = `/home/valv/.claude/.credentials.json`.
  Verify this matches the bind-mount target. If Claude writes elsewhere,
  the mount and the host-side verification path must be adjusted.

Tests in `claude_auth.go` (or `claude_auth_test.go`):
- `TestEnsureClaudeAccountReadyRejectsNonTTY` — stub executor, non-TTY cmd,
  verify error mentions TTY.
- `TestEnsureClaudeAccountReadyWipesExistingCredentials` — stub executor that
  does nothing; verify `.credentials.json` is absent at launch; stub
  verification logic so the test does not require a real container.
- `TestEnsureClaudeAccountReadySucceedsAfterContainerWrite` — stub executor
  that writes a fake `.credentials.json` to the account home before returning;
  verify the function returns nil.
- `TestLoginClaudeAccountSkipsNonTTYGuard` — verify it calls the container
  launch even without TTY.
- Test that `logoutManagedAccount` for Claude wipes `.credentials.json`.

**Acceptance:**
- `mage testPkg internal/cli` passes.
- `ensureManagedAccountReady(cmd, ProviderClaude, account, opts)` calls
  `ensureClaudeAccountReady` (not `return nil`).
- `loginManagedAccount(cmd, ProviderClaude, account)` calls `loginClaudeAccount`
  (not `return nil`).
- Stub-based test confirms credentials wipe happens before container launch.
- Stub-based test confirms the function returns error if `.credentials.json`
  is absent after container exits.
- Non-TTY path returns a user-readable error.

---

#### Unit 6.3 — Claude credentials parsing + display wiring

**State:** todo  
**Paths:** `internal/adapters/providers/claude/account.go`,
  `internal/adapters/providers/claude/account_test.go`,
  `internal/cli/operator_helpers.go`,
  `internal/cli/operator_helpers_test.go` (new or existing)  
**Packages:** `internal/adapters/providers/claude`, `internal/cli`  
**Blocked by:** —

**Description:**

Two-part unit. Part A (adapter package) and Part B (CLI display) share no
package boundary conflict — `internal/adapters/providers/claude` and
`internal/cli` are disjoint. Both parts go in one build droplet because they
are a tight end-to-end pair: parsing produces nothing useful until the display
layer consumes it.

**Part A — `internal/adapters/providers/claude/account.go`:**

Extend `ReadAccountIdentity` from presence-only to parse-and-extract.

Builder must first determine the actual `.credentials.json` format by either:
- Running a real Claude device-code auth flow in the dev container and
  examining the resulting file, OR
- Checking Anthropic's documentation.

Known partial evidence: `account_test.go:14` uses
`{"claudeAiAccessToken":"tok"}`, suggesting the token field name. An email
field may or may not exist. Parsing approach based on what the builder finds:

If the file contains an email field directly:
```go
type credentialsFile struct {
    // builder fills in actual field names from real file inspection
    Email string `json:"<field_name>"`
}
```

If the file contains a JWT-style access token (as Codex does):
- Decode using the same base64 + JSON approach as `decodeIDTokenClaims`.
- Extract `email` from the JWT claims.

If neither is present:
- `ReadAccountIdentity` returns `AccountIdentity{LoggedIn: true}` with empty
  Email/Name (graceful degradation — same as current presence-only behavior,
  just with the `LoggedIn` flag accurate).

The parsing must not error on unexpected formats — log the shape and degrade
gracefully. The function signature stays identical: `ReadAccountIdentity(homePath string) (AccountIdentity, error)`.

Remove the existing test `TestReadAccountIdentityIrrelevantContentsStillLoggedIn`
and replace with tests that cover the actual format the builder discovers.

**Part B — `internal/cli/operator_helpers.go`:**

Add `case domain.ProviderClaude:` to `readAccountIdentity` (line 219):
```go
case domain.ProviderClaude:
    identity, err := claudeprovider.ReadAccountIdentity(profile.HomePath)
    if err != nil {
        return accountDisplayIdentity{
            authDisplay:  "unavailable",
            emailDisplay: "(unavailable)",
        }
    }
    return accountDisplayIdentity{
        authDisplay:  claudeAuthDisplay(identity),
        emailDisplay: claudeEmailDisplay(identity),
    }
```

Add `claudeAuthDisplay` and `claudeEmailDisplay` helpers in `operator_helpers.go`
(same style as `codexAuthDisplay` and `codexEmailDisplay`):
- `claudeAuthDisplay`: if `LoggedIn`, return `"logged in"`; else `"not logged in"`.
- `claudeEmailDisplay`: if `Email` non-empty, return it; if `LoggedIn`, return
  `"(identity unavailable)"`; else `"(not logged in)"`.

Also add the `claudeprovider` import to `operator_helpers.go` (it may not be
imported yet — builder confirms).

Tests:
- Part A: table-driven tests for `ReadAccountIdentity` covering: (a) file
  present with expected format → LoggedIn=true, Email extracted; (b) file
  present with unexpected format → LoggedIn=true, Email empty (graceful); (c)
  file absent → LoggedIn=false; (d) file is a directory → LoggedIn=false.
- Part B: unit tests for `claudeAuthDisplay` and `claudeEmailDisplay` covering
  the same edge cases as `codexAuthDisplay`/`codexEmailDisplay`.

**Acceptance:**
- `mage testPkg internal/adapters/providers/claude` passes.
- `mage testPkg internal/cli` passes.
- `ReadAccountIdentity` returns a non-empty `Email` when the credentials file
  contains one (as determined by builder's real-file inspection).
- `readAccountIdentity` no longer falls to `default` for Claude profiles;
  `account list` output shows email when available.

---

#### Unit 6.4 — Force-relogin flag on `account login` (DEFERRED 2026-05-15)

**State:** deferred  
**Paths:** `internal/cli/manage.go`, `internal/cli/manage_test.go`,
  `internal/cli/account_auth.go`  
**Packages:** `internal/cli`  
**Blocked by:** 6.1, 6.2, 6.3

**Deferred because:** scope narrowing eliminated 6.1; this unit's primary value is the `--force-relogin` flag for re-auth without delete+recreate. Useful hardening but not required for v0.1.0 Claude dogfood. Revisit in DROP_9 cleanup.

**Description:**

Blocked by 6.1 and 6.2 (same package, `account_auth.go` and `manage.go` both
touched) and by 6.3 (operator_helpers.go Claude case must be present for the
complete display chain to work when testing).

Three changes, all in `internal/cli`:

**1. `--force-relogin` flag on `account login`:**

In `newManageAccountLoginCommand` (`manage.go:127`), add:
```go
var forceRelogin bool
cmd.Flags().BoolVar(&forceRelogin, "force-relogin", false, "wipe existing credentials before logging in")
```

Pass `forceRelogin` into `runManageAccountLogin`. In that function, before
calling `loginManagedAccount`, if `forceRelogin == true`:
- Call a new helper `wipeManagedCredentials(provider, profile.HomePath)` that
  dispatches to `wipeCodexCredentials` (from 6.1) for Codex, or wipes
  `.credentials.json` for Claude.

```go
// wipeManagedCredentials removes the provider-specific credentials file from
// homePath if it exists. Missing file is not an error.
func wipeManagedCredentials(provider domain.Provider, homePath string) error { ... }
```

**2. Account-add unconditional Claude credential wipe:**

In `runManageAccountAdd` (`manage.go:462`), extend the wipe from 6.1 (which
only wiped `auth.json`) to also wipe `.credentials.json` for Claude. Options:
- A single `wipeManagedCredentials(provider, profile.HomePath)` call at the
  wipe point replaces the per-provider calls from 6.1. This is cleaner than
  separate wipe helpers. Builder decides whether to refactor 6.1's wipe into
  this shared helper or keep them separate.

**3. `account login` example update in `newManageAccountLoginCommand`:**

Update the command `Long` description to mention `--force-relogin`.

Tests to add:
- `TestRunManageAccountLoginForceReloginWipesCodexCredentials` — set up a
  temp account home with an `auth.json` present; call the login path with
  `forceRelogin=true`; verify `auth.json` is gone before login runs.
- `TestRunManageAccountLoginWithoutForceReloginKeepsCredentials` — verify that
  without the flag, `auth.json` is NOT wiped.
- Claude analogues if the builder can stub the container launch cleanly.

**Acceptance:**
- `mage testPkg internal/cli` passes.
- `valv manage account login --help` shows `--force-relogin` in the flags list.
- Unit test confirms `auth.json` is wiped when `--force-relogin` is set and not
  wiped when it is unset.
- `account list` and `account inspect` show real email for Claude accounts
  when credentials are present (end-to-end: 6.3 parse + 6.4 display wiring).

---

### Notes

**Why 6.2 can run in parallel with 6.1 even though both touch `internal/cli`:**
6.1 touches `manage.go` and adds a helper (possibly a new file). 6.2 adds a
NEW file `claude_auth.go` and edits `account_auth.go`. If 6.1 ALSO edits
`account_auth.go` (it does not — 6.1 only edits `manage.go`), there would be a
conflict. Checking: 6.1 edits `manage.go` only. 6.2 edits `account_auth.go`
and adds `claude_auth.go`. No file-level conflict. However, both are in the
same Go package (`internal/cli`), so a compile-level conflict could occur if
they have overlapping symbols. Since 6.1 adds a `wipeCodexCredentials` function
and 6.2 adds `ensureClaudeAccountReady`/`loginClaudeAccount`, there is NO
symbol overlap. They can run in parallel. The `blocked_by` graph above is
correct: 6.4 blocks on 6.1 AND 6.2 (both touch `account_auth.go` — 6.2 flips
the stubs, 6.4 adds the flag handler that calls the flipped stubs indirectly
via `loginManagedAccount`).

**Claude credentials file path in container:**
The bind-mount target is `ContainerClaudeDir = "/home/valv/.claude"`.
The `CLAUDE_CONFIG_DIR` env var is set to the same value. Claude CLI uses
`$CLAUDE_CONFIG_DIR` or `~/.claude` as the credentials directory. The expected
credential file path inside the container is
`/home/valv/.claude/.credentials.json`. This maps to the host at
`account.HomePath + "/.credentials.json"`. Builder must verify this is correct
before finalizing the mount spec and the post-run credential check.

**`logoutManagedAccount` for Claude:**
Unit 6.2 flips this to wipe `.credentials.json`. Logout for Claude does not
need a container — credentials are file-resident on the host bind-mount. A
simple `os.Remove` of `.credentials.json` from the account home is correct.
This is symmetric with Codex's host-side `codex logout` approach (which writes
a state change to the home dir).

**`ensureClaudeAccountReady` skips image check path:**
The Claude image check (`ensureClaudeImageCurrent`) involves a spinner and
potentially a build. For the auth flow, the builder should ensure the image is
present before launching the auth container, using the same
`ensureClaudeImageCurrent` path. If the image is not built, the error message
from `ensureClaudeImageCurrent` is sufficient.

**Coverage:**
All new functions are covered by stub-based unit tests. The 70% per-package
floor applies to `internal/cli` and `internal/adapters/providers/claude`.

### Hylla Feedback

Hylla answered all Go symbol queries correctly. Two Hylla queries were run
(`auth.json copy seed credentials` and `force relogin credentials wipe delete`)
— both returned relevant results. No misses. All committed-state reads were done
via the Read tool (faster and more complete for full file content than Hylla
block queries). Hylla was used for cross-package blast-radius confirmation and
confirmed the set of functions that touch credentials.

Non-Go files (PLAN.md, WORKFLOW.md, CLAUDE.md) were read directly via the Read
tool per Hylla's Go-only scope — no misses to report for those.

## Notes

- **Mechanical-ish drop — trimmed cascade applies** per memory `feedback_trimmed_cascade_for_mechanical_drops.md`. Claude container-auth flow is new code, but it mirrors the existing Codex `ensureCodexAccountReady` shape adapted for in-container interaction. Single planner spawn, no parallel plan-QA. Per-unit build-QA stays in place.
- **Copy-adapt template for Claude auth flow.** `internal/cli/account_auth.go::ensureCodexAccountReady` is the structural template. Key differences: (a) Claude runs in a container, not on host; (b) auth is device-code, not browser-OAuth-from-host; (c) success criterion is `.credentials.json` appearing in the mounted dir.
- **Identity-extraction format for Claude `.credentials.json`.** v1 Claude adapter does presence-only check. Builder needs to read an actual `.credentials.json` (after running a real Claude device-code auth) to see the JSON shape. If the file contains a JWT, decode the same way Codex does. If it's a different shape (just an API key + email), parse accordingly. Builder may need to do the first real Claude auth as part of this work to capture the format.
- **Force-relogin semantics.** Before launching the auth flow, delete any pre-existing `auth.json` (Codex) or `.credentials.json` (Claude) in the target managed home. This guarantees no credential reuse. Applies whether the entry point is `account add <provider> <name>` (always force-fresh) or `account login` with `--force-relogin` flag (opt-in force-fresh on existing accounts).
- **Codex audit.** The current Codex flow may already do the right thing for fresh accounts — the bug is in how `hylla` was originally created (probably via early dev paths that copied `~/.codex/`). The planner MUST audit `ensureCodexAccountReady` for any code path that copies, symlinks, or otherwise inherits an existing host `auth.json` into the managed home, and excise it.
- **Coverage floor.** 70% per AGENTS.md target. Per-package floor enforced by mage at the current 60% (per `magefile.go:22` TODO note). New code must clear the 70% target.
- **Out of scope:** DB orphan cleanup (use existing `valv account cleanup`), TUI picker golden parity (DROP_7), globalswitch (DROP_7), integration test (DROP_8), brew formula / GitHub releases workflow (DROP_8).
