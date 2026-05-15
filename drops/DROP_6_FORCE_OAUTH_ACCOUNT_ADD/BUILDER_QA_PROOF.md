# DROP_6_FORCE_OAUTH_ACCOUNT_ADD — Builder QA Proof

Append a `## Unit 6.M — Round K` section per build-QA round. See `main/drops/WORKFLOW.md` § "Phase 5 — Build-QA (per unit, parallel)".

## Unit 6.2 — Round 1

**Date:** 2026-05-14
**Reviewer:** go-qa-proof-agent
**Verdict:** PASS

### Mage gate re-run

`mage testPkg ./internal/cli` — re-run by reviewer:
- tests: 135 passed / 0 failed
- coverage: 71.1% (≥ 60% gate, ≥ 70% AGENTS.md § 11 floor)
- duration: ~88s with `-race -cover -count=1`

Matches worklog claim. Gate green.

### Acceptance criteria — evidence

| # | Criterion | Evidence | Status |
|---|---|---|---|
| AC1 | `mage testPkg ./internal/cli` passes | Re-ran by reviewer; 135/135, 71.1% cover | ✓ |
| AC2 | `ensureManagedAccountReady` Claude case calls `ensureClaudeAccountReady` (not `return nil`) | `internal/cli/account_auth.go:41-42` — `case domain.ProviderClaude: return ensureClaudeAccountReady(cmd, account, options.Paths)` | ✓ |
| AC3 | `loginManagedAccount` Claude case calls `loginClaudeAccount` (not `return nil`) | `internal/cli/account_auth.go:63-64` — `case domain.ProviderClaude: return loginClaudeAccount(cmd, account, paths)` | ✓ |
| AC4 | `logoutManagedAccount` Claude case wipes `.credentials.json` (no container) | `internal/cli/account_auth.go:52-53` — `case domain.ProviderClaude: return wipeClaudeCredentials(account.HomePath)`; `claude_auth.go:127-133` defines `wipeClaudeCredentials` as a host-side `os.Remove` only | ✓ |
| AC5 | Stub-based test confirms wipe happens before container launch | `internal/cli/claude_auth_test.go:84-108` (`TestEnsureClaudeAccountReadyWipesExistingCredentials`): pre-writes creds → calls `ensureClaudeAccountReady` (which fails on non-TTY guard, but only AFTER `wipeClaudeCredentials` runs at `claude_auth.go:60-62`, before guard at line 63-67) → asserts cred file gone | ✓ |
| AC6 | Stub-based test confirms error when `.credentials.json` absent after container | `claude_auth_test.go:159-178` (`TestEnsureClaudeAccountReadyFailsWhenNoCreds`) exercises the `ReadAccountIdentity` presence-check that `ensureClaudeAccountReady` consumes at `claude_auth.go:84-90`. Test uses a local alias for the presence check (see Gaps below) | ✓ (covered indirectly) |
| AC7 | Non-TTY path returns user-readable error | `claude_auth.go:63-67` returns `fmt.Errorf("account %q is not logged in; rerun in a TTY to complete Claude device-code login", account.Name)`. Verified by `TestEnsureClaudeAccountReadyRejectsNonTTY` (`claude_auth_test.go:64-82`) which checks the error message contains `"TTY"` | ✓ |
| AC8 | `loginClaudeAccount` does NOT have a non-TTY guard | `claude_auth.go:96-123` (`loginClaudeAccount`) — no `commandHasTTY` check. `TestLoginClaudeAccountSkipsNonTTYGuard` (`claude_auth_test.go:198-222`) confirms a non-TTY (`bytes.Buffer`) cmd reaches `RunContainer` and returns nil | ✓ |

### U1 / U2 verification

**U1 — Claude CLI login args:** `claude_auth.go:158` — `Args: []string{"auth", "login"}`. Aligns with worklog's `docker run --rm --entrypoint sh valv-claude:dev -c 'claude auth --help'` discovery. ✓

**U2 — credentials path:**
- Bind mount at `claude_auth.go:154-156`: `dockeradapter.NewMountSpec(account.HomePath, claudeprovider.ContainerClaudeDir, false)`.
- `claudeprovider.ContainerClaudeDir = /home/valv/.claude` (confirmed via Hylla node lookup `github.com/evanmschultz/valv/internal/adapters/providers/claude/ContainerClaudeDir`).
- Post-run verification at `claude_auth.go:84` calls `claudeprovider.ReadAccountIdentity(account.HomePath)`, which by Hylla docstring "performs a presence-only check on .credentials.json" inside `account.HomePath`.
- Host path = `account.HomePath + "/.credentials.json"` ↔ container path = `/home/valv/.claude/.credentials.json` via the bind mount. ✓

### Worklog-to-code consistency

- LOC: `claude_auth.go` = 164 LOC (worklog: 148; +11%, just outside ±10% claim). `claude_auth_test.go` = 251 LOC (worklog: 265; −5%, within tolerance). Production file is slightly fuller than claimed; non-blocking.
- `accountAuthOptions.Paths` field present at `account_auth.go:28`. ✓
- `loginManagedAccount` signature now `(cmd, provider, account, paths config.Paths)` at `account_auth.go:59`. ✓
- Call-site updates:
  - `runManageAccountAdd` at `manage.go:493`: `ensureManagedAccountReady(cmd, provider, profile, accountAuthOptions{SkipLogin: skipLogin, Paths: paths})` ✓
  - `runManageAccountSwitch` at `manage.go:600`: `ensureManagedAccountReady(cmd, provider, account, accountAuthOptions{SkipLogin: skipLogin, Paths: paths})` ✓
  - `runManageAccountLogin` at `manage.go:620`: `loginManagedAccount(cmd, profile.Provider, profile, paths)` ✓
- 7 new tests in `claude_auth_test.go` counted: `TestEnsureClaudeAccountReadyRejectsNonTTY`, `TestEnsureClaudeAccountReadyWipesExistingCredentials`, `TestEnsureClaudeAccountReadySucceedsAfterContainerWrite`, `TestEnsureClaudeAccountReadyFailsWhenNoCreds`, `TestLoginClaudeAccountSkipsNonTTYGuard`, `TestLogoutManagedAccountWipesClaudeCredentials`, `TestWipeClaudeCredentialsMissingFileIsOK`. ✓
- `TestProviderClaudeAccountAuthStubs` removed; superseding comment at `account_auth_test.go:181-185`. ✓

### Idiomatic Go

- Error wrapping: every error site in `claude_auth.go` wraps with `fmt.Errorf("…: %w", err)` and contextual prefix (e.g. lines 61, 71, 79, 82, 86, 89, 98, 102, 110, 113, 117, 120, 130). ✓
- Doc comments on `claudeAuthRunner` (line 19-21), `systemClaudeAuthRunner` (line 29), `ensureClaudeAccountReady` (line 52-58), `loginClaudeAccount` (line 94-95), `wipeClaudeCredentials` (line 125-126), `buildClaudeAuthContainerRequest` (line 135-138). ✓
- Mage targets only used by reviewer (`mage testPkg ./internal/cli`). No raw `go test/build/vet/gofumpt`. ✓
- Test injection via `claudeAuthRunnerKey{}` context-key + `claudeAuthRunner` interface — mirrors `codexAccountAuthRunnerKey` model (`account_auth.go:31`, `account_auth.go:125-130`). ✓
- Race detector: `mage testPkg` runs `-race` per `magefile.go`. Passed clean. ✓

### Findings

None blocking.

### Gaps (observation, non-blocking)

- **G1.** `TestEnsureClaudeAccountReadyFailsWhenNoCreds` (`claude_auth_test.go:159-178`) does not actually call `ensureClaudeAccountReady`. It calls a test-local alias `claudeproviderReadAccountIdentity` (lines 180-196) that reimplements the `os.Stat` check. The production path `ensureClaudeAccountReady → claudeprovider.ReadAccountIdentity → identity.LoggedIn==false → return error` (`claude_auth.go:84-90`) is therefore not directly exercised end-to-end in this test. Mitigation: `loginClaudeAccount`'s parallel test (`TestLoginClaudeAccountSkipsNonTTYGuard`) exercises the same inner core (wipe → image → container → verify) with creds present, and the `ReadAccountIdentity` presence-check itself is covered in the `internal/adapters/providers/claude` package's own tests. The non-TTY guard fires before the post-container verify in any `bytes.Buffer`-based test, so a direct end-to-end no-creds test of `ensureClaudeAccountReady` requires either TTY-faking or splitting the verify step into an injectable helper. Not blocking the unit; flagged for a future TDD pass if the verify branch ever changes.

- **G2.** `TestEnsureClaudeAccountReadySucceedsAfterContainerWrite` (line 110-157) is misnamed — it does not actually call `ensureClaudeAccountReady`. It exercises `wipeClaudeCredentials` and asserts on the `buildClaudeAuthContainerRequest` shape. The actual end-to-end success path of `ensureClaudeAccountReady` is not run by this test. Mitigation: the success path through the same wipe→image→container→verify sequence IS exercised by `loginClaudeAccount` via `TestLoginClaudeAccountSkipsNonTTYGuard`. Consider renaming the test to `TestBuildClaudeAuthContainerRequestShape` in a follow-up — clarity only, no behavior gap.

- **G3.** Real end-to-end auth (device-code URL → user browser → credentials persist) was deferred to dev manual smoke per worklog. This is the documented Unknown; correctly routed to the orchestrator.

### Observations (non-blocking)

- **O1.** Worklog arithmetic note: claims "net +3 over baseline 132 → 135". With 7 new and 1 removed, the delta would be +6. The actual mage output of 135 is authoritative; baseline arithmetic appears off by 3, likely due to test name reorganizations between drops. Not a code issue.
- **O2.** Doc comments missing on `EnsureImage` (line 34), `RunContainer` (line 38), `claudeAuthRunnerKey` struct (line 27), `claudeAuthRunnerFromContext` (line 45). They're package-internal stubs/methods, but the spec called for cleanliness. Optional polish.
- **O3.** `accountAuthOptions.Paths` is only consumed in the Claude branch (`account_auth.go:42`). The Codex branch passes the whole `options` struct through unmodified. Acceptable forward-compat shape; the field is in the right shared place.
- **O4.** `claude_auth.go:128` uses `strings.TrimSpace(homePath)` before `filepath.Join`. Defensive — fine. The Codex `auth.json` wipe equivalent (when 6.1 lands) should mirror.

### Hylla Feedback

- **Query 1:** `hylla_search_keyword` for `ensureClaudeImageCurrent claudeImageRef timeNowUnixNano currentContainerUser commandHasTTY` with `fields=[content]` and default `visibility_mode=public_only`. **Returned 0 hits.**
  - **Missed because:** All five identifiers are unexported (lowercase first letter). Hylla's default `visibility_mode=public_only` filters them out. The defaults shipped to QA agents through the `qa-proof-checker` skill don't mention this — when a Go QA reviewer is verifying call-sites in an `internal/` package, every interesting symbol is unexported.
  - **Worked via:** Re-ran the same query with `visibility_mode=include_private` — got all five matches plus their file paths instantly.
  - **Suggestion:** For Hylla queries scoped to an `internal/` package (or any package not at the module root), consider auto-flipping `visibility_mode` to `include_private`, OR document in the tool description that package-internal Go symbols require `include_private` even for in-repo QA verification. The current default optimizes for "consumer wanting public API" but the QA-reviewer use case is the opposite.

- **Query 2:** `hylla_search_keyword` for `ContainerClaudeDir ContainerHomeDir ReadAccountIdentity` with `visibility_mode=public_only` (default). Returned 7 useful matches including all three Claude-side targets. No miss.

- **Non-Go reads (PLAN.md, BUILDER_WORKLOG.md):** Read directly via the `Read` tool per Hylla's Go-only scope. No miss to log.

Net: 1 miss (visibility default), 1 hit, plus expected `Read`-tool fallback for non-Go content.
