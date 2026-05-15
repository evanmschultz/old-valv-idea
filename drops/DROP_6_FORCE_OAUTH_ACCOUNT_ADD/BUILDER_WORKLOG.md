# DROP_6_FORCE_OAUTH_ACCOUNT_ADD — Builder Worklog

Append a `## Unit 6.M — Round K` section per build attempt. See `main/drops/WORKFLOW.md` § "Phase 4 — Build (per unit)" for what each section should contain.

<!-- units filled in by planner, then by builder during Phase 4 -->

## Unit 6.2 — Round 1

**Date:** 2026-05-14  
**State result:** done  
**Mage target:** `mage testPkg ./internal/cli`  
**Mage result:** PASS — 135 tests, 0 failures, 71.1% coverage (≥60% gate, ≥70% AGENTS.md target)

### Files touched

- `internal/cli/claude_auth.go` — **new** (148 LOC): `claudeAuthRunner` interface, `claudeAuthRunnerKey`, `systemClaudeAuthRunner`, `ensureClaudeAccountReady`, `loginClaudeAccount`, `wipeClaudeCredentials`, `buildClaudeAuthContainerRequest`.
- `internal/cli/claude_auth_test.go` — **new** (265 LOC): 7 tests covering non-TTY rejection, credential wipe, container request shape, no-creds error, login skips non-TTY guard, logout wipe, missing file is OK.
- `internal/cli/account_auth.go` — **edited**: added `config.Paths` to `accountAuthOptions`; flipped `ensureManagedAccountReady` Claude case → `ensureClaudeAccountReady`; flipped `logoutManagedAccount` Claude case → `wipeClaudeCredentials`; updated `loginManagedAccount` signature to carry `paths config.Paths`; flipped `loginManagedAccount` Claude case → `loginClaudeAccount`.
- `internal/cli/account_auth_test.go` — **edited**: removed `TestProviderClaudeAccountAuthStubs` (its 3 subtests tested dead stubs; behavior is now covered by real tests in `claude_auth_test.go`).
- `internal/cli/manage.go` — **edited**: updated `accountAuthOptions{}` → `accountAuthOptions{..., Paths: paths}` at `runManageAccountAdd` and `runManageAccountSwitch` call sites; updated `loginManagedAccount` call at `runManageAccountLogin` to pass `paths`.

### U1 resolution — Claude CLI login args

Confirmed via `docker run --rm --entrypoint sh valv-claude:dev -c 'claude auth --help'`:

```
Commands:
  login [options]   Sign in to your Anthropic account
```

The device-code OAuth flow is triggered by `claude auth login`. Container args: `["auth", "login"]`. Entrypoint is `claude`, so no entrypoint override needed.

### U2 resolution — `.credentials.json` location

Confirmed via `docker run --rm --entrypoint sh valv-claude:dev -c 'ls /home/valv/.claude/ || echo dir'`:
- Path: `/home/valv/.claude/.credentials.json` = `ContainerClaudeDir + "/.credentials.json"`.
- Image default env: `CLAUDE_CONFIG_DIR=/home/valv/.claude`.
- Host path after bind-mount: `account.HomePath + "/.credentials.json"`.

The bind-mount `account.HomePath → ContainerClaudeDir` (read-write) is correct. `ReadAccountIdentity` checks `filepath.Join(homePath, ".credentials.json")` — matches.

### Design decisions

**Test injection pattern:** Used `claudeAuthRunner` interface (2 methods: `EnsureImage`, `RunContainer`) injected via context key. Chose interface over env-var bypass because the interface covers both the image check and container launch cleanly, and gives tests precise control over what the stub does (writing creds vs not). Env-var bypass would only skip the full function — can't test wipe behavior or container request shape with it.

**`loginManagedAccount` signature change:** Added `paths config.Paths` parameter. Required because `loginClaudeAccount` calls `ensureClaudeImageCurrent(cmd, paths)`. Three callers updated: `runManageAccountLogin` in `manage.go` (the only call site in production code — passes its own `paths`). `ensureBoundCodexAccountReady` in `codex.go` does NOT call `loginManagedAccount` — no change needed there.

**Non-TTY guard placement:** In `ensureClaudeAccountReady` only (after credential wipe, before image check). NOT in `loginClaudeAccount` per spec — explicit login is a user-initiated action that should always attempt the flow.

**`logoutManagedAccount` Claude case:** File-wipe only via `wipeClaudeCredentials`. No container launch needed — credentials are file-resident on the host bind-mount path.

**Removed `TestProviderClaudeAccountAuthStubs`:** This test verified the old no-op stubs returned nil. After the flip, those stubs are gone. The 3 sub-tests' behavior is superseded by real behavior tests in `claude_auth_test.go`.

### Real end-to-end auth verification

Real end-to-end auth verification (device-code URL rendered in terminal, user completes OAuth in browser, `.credentials.json` written to bind-mounted account home) is deferred to dev manual smoke after CI green. Cannot complete a device-code flow as a subagent — requires human browser interaction.

### Hylla Feedback

Hylla is Go-only and was queried for cross-package symbol reference confirmation. All key symbols (`ensureManagedAccountReady`, `logoutManagedAccount`, `loginManagedAccount`, `runManageAccountAdd`, `runManageAccountSwitch`, `runManageAccountLogin`) confirmed via `Read` tool (faster for full-file context than Hylla block queries). Hylla was not needed for any symbol that required graph navigation — all references are in one package (`internal/cli`).

None — Hylla answered all queries; no misses. Non-Go files (PLAN.md, WORKFLOW.md) read directly per Hylla's Go-only scope.

---

## Unit 6.2 — Round 2

**Date:** 2026-05-14
**State result:** done
**Mage target:** `mage testPkg ./internal/cli`
**Mage result:** PASS — 137 tests, 0 failures, 71.2% coverage (≥60% gate, ≥70% AGENTS.md target)
**Prior attempt:** Round 1 shipped working code; Round 2 fixes three QA Falsification findings (C1, C2, C3).

### Files touched

- `internal/cli/account_auth.go` — **edited** (line 42): changed `ensureClaudeAccountReady(cmd, account, options.Paths)` to `ensureClaudeAccountReady(cmd, account, options)` — passes full `accountAuthOptions` so `SkipLogin` is not dropped.
- `internal/cli/claude_auth.go` — **edited** (lines 52-99): changed `ensureClaudeAccountReady` signature from `(cmd, account, paths config.Paths)` to `(cmd, account, options accountAuthOptions)`; added `if options.SkipLogin { return nil }` as first check (C1); moved TTY guard to before `wipeClaudeCredentials` (C2); updated internal `paths` references to `options.Paths`.
- `internal/cli/claude_auth_test.go` — **rewritten**: removed 3 misnamed/indirect tests; added 5 new tests; test count 135 → 137 (net +2).

### Fix C1 — `options.SkipLogin` threaded to Claude path (`account_auth.go:42`, `claude_auth.go:63-66`)

**Problem:** dispatcher at `account_auth.go:42` forwarded only `options.Paths` to `ensureClaudeAccountReady`, silently dropping `options.SkipLogin`. `ensureClaudeAccountReady` had no `SkipLogin` parameter. `valv manage account add claude --skip-login` and `valv manage account switch claude --skip-login` both ignored the flag.

**Fix:**
- `account_auth.go:42`: `ensureClaudeAccountReady(cmd, account, options)` — full struct forwarded.
- `claude_auth.go:63-66`: signature changed to `options accountAuthOptions`; `if options.SkipLogin { return nil }` added as first statement; internal `paths` usages replaced with `options.Paths`.

**Test:** `TestEnsureClaudeAccountReadyRespectsSkipLogin` (`claude_auth_test.go:88-113`) — verifies `SkipLogin=true` → returns nil, no container hit, no image hit, pre-existing creds file preserved.

### Fix C2 — TTY check reordered before credential wipe (`claude_auth.go:67-75`)

**Problem:** `wipeClaudeCredentials` (line 60 in Round 1) executed before the TTY check (line 63). Non-TTY callers with valid pre-existing credentials had them destroyed before receiving the "rerun in TTY" error.

**Fix:** Sequence is now: `SkipLogin` check → TTY check → `wipeClaudeCredentials` → `EnsureImage` → notice → `RunContainer` → verify. The wipe only occurs when the function is committed to running the full auth flow.

**Test:** `TestEnsureClaudeAccountReadyNonTTYDoesNotWipe` (`claude_auth_test.go:117-143`) — writes pre-existing creds, calls with non-TTY, asserts error contains "TTY" AND creds file still exists on disk.

**Secondary effect:** `TestEnsureClaudeAccountReadyWipesExistingCredentials` from Round 1 was removed — it asserted that creds are gone after a non-TTY call, which is now the wrong behavior (TTY guard fires first). The new `TestEnsureClaudeAccountReadyNonTTYDoesNotWipe` replaces it with the correct assertion.

### Fix C3 — Renamed/replaced two misnamed tests (`claude_auth_test.go`)

**Problem:** Two Round 1 tests were named for behavior of `ensureClaudeAccountReady` but did not call it:
- `TestEnsureClaudeAccountReadySucceedsAfterContainerWrite` called `buildClaudeAuthContainerRequest` only.
- `TestEnsureClaudeAccountReadyFailsWhenNoCreds` called a test-local shadow `claudeproviderReadAccountIdentity`.

**Fix:**
- Renamed `TestEnsureClaudeAccountReadySucceedsAfterContainerWrite` → `TestBuildClaudeAuthContainerRequestShape` (`claude_auth_test.go:147-172`): accurately describes the container-request shape assertions.
- Renamed `TestEnsureClaudeAccountReadyFailsWhenNoCreds` → `TestReadAccountIdentityReturnsLoggedOutWhenNoCreds` (`claude_auth_test.go:175-195`): now calls `claudeprovider.ReadAccountIdentity` directly (real adapter, not shadow). Shadow function `claudeproviderReadAccountIdentity` removed.
- Added `TestLoginClaudeAccountFailsWhenNoCredsAfterContainer` (`claude_auth_test.go:198-226`): exercises the genuine "no credentials file found after login" error path via `loginClaudeAccount` (which has no TTY guard), using a stub that does NOT write creds. This is the correct way to test the no-creds-after-container branch — `loginClaudeAccount` shares the same wipe→image→container→verify body as `ensureClaudeAccountReady` but without the TTY gate.

### Test count

| Round | Tests | Coverage |
|---|---|---|
| Round 1 | 135 | 71.1% |
| Round 2 | 137 | 71.2% |
| Delta | +2 | +0.1pp |

### Design decisions

**Why test "no creds after container" via `loginClaudeAccount` not `ensureClaudeAccountReady`:** The post-container verify branch of `ensureClaudeAccountReady` is unreachable with a non-TTY stub because the TTY guard fires first (C2 fix). `loginClaudeAccount` has no TTY guard and shares the identical wipe→image→container→verify body. Testing the error path via `loginClaudeAccount` with a no-write stub covers the same production code branch without requiring TTY-faking infrastructure.

**`TestEnsureClaudeAccountReadyWipesExistingCredentials` removal:** This test asserted that creds are wiped on a non-TTY call. After C2, this assertion became wrong (creds are preserved on non-TTY). The test was removed and replaced by `TestEnsureClaudeAccountReadyNonTTYDoesNotWipe` which asserts the new correct behavior.

### Hylla Feedback

Hylla returned 0 results for `ensureClaudeAccountReady` keyword search (`visibility_mode=include_private`) — expected, as this symbol was introduced in Round 1 and Hylla is ingest-at-drop-end. All symbol lookups done via `Read` on the production files. No unexpected misses.

N/A for cross-package references — all changes are within `internal/cli`.
