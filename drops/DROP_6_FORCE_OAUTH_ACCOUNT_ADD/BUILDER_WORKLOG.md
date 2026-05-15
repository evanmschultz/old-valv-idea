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
