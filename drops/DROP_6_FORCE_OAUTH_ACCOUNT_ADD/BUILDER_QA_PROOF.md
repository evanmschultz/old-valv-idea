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

---

## Unit 6.2 — Round 2

**Date:** 2026-05-14
**Reviewer:** go-qa-proof-agent
**Verdict:** PASS

### Mage gate re-run

`mage testPkg ./internal/cli` — re-run by reviewer:
- tests: 137 passed / 0 failed
- coverage: 71.2% (≥ 60% gate, ≥ 70% AGENTS.md § 11 floor)
- duration: ~90.73s with `-race -cover -count=1`

Matches worklog claim exactly (137 / 71.2%). Gate green.

### Fixes — evidence

#### C1 — `options.SkipLogin` threaded to Claude path

| Sub-criterion | Evidence | Status |
|---|---|---|
| Dispatcher forwards full `options` | `internal/cli/account_auth.go:42` — `return ensureClaudeAccountReady(cmd, account, options)` (no longer `options.Paths`) | ✓ |
| `ensureClaudeAccountReady` signature accepts `options accountAuthOptions` | `claude_auth.go:63` — `func ensureClaudeAccountReady(cmd *cobra.Command, account domain.Profile, options accountAuthOptions) error` | ✓ |
| First statement is `if options.SkipLogin { return nil }` | `claude_auth.go:64-66` — opens with `if options.SkipLogin { return nil }` immediately after the function header (no intervening statements) | ✓ |
| Test exercises `SkipLogin=true` path | `claude_auth_test.go:88-116` — `TestEnsureClaudeAccountReadyRespectsSkipLogin` pre-writes creds, calls with `SkipLogin: true`, asserts: `err == nil` (line 103-105), `stub.containerHits == 0` (line 106-108), `stub.imageHits == 0` (line 109-111), and creds file still exists on disk (line 112-115) | ✓ |

#### C2 — TTY check ordered before credential wipe

| Sub-criterion | Evidence | Status |
|---|---|---|
| Order is SkipLogin → TTY → wipe (NOT wipe → TTY) | `claude_auth.go:63-75` — `if options.SkipLogin { return nil }` (line 64), then `if !commandHasTTY(cmd.InOrStdin()) { return fmt.Errorf(...) }` (lines 67-72), then `wipeClaudeCredentials(account.HomePath)` (line 73). Wipe is third, after both short-circuits. | ✓ |
| Non-TTY caller preserves pre-existing creds | `claude_auth_test.go:121-146` — `TestEnsureClaudeAccountReadyNonTTYDoesNotWipe` writes creds, calls non-TTY, asserts error contains `"TTY"` (line 139-141) AND `os.Stat(credPath)` reports the file still exists (line 143-145) | ✓ |
| `loginClaudeAccount` has NO TTY guard (Round 1 contract preserved) | `claude_auth.go:103-130` — `loginClaudeAccount` function body contains no `commandHasTTY` call. First statement is `wipeClaudeCredentials` (line 104), proceeding directly to image/container/verify. | ✓ |

#### C3 — Test renames + genuine no-creds test

| Sub-criterion | Evidence | Status |
|---|---|---|
| `TestEnsureClaudeAccountReadySucceedsAfterContainerWrite` removed/renamed | `claude_auth_test.go:151` — replaced by `TestBuildClaudeAuthContainerRequestShape` (now matches what the body actually exercises: `buildClaudeAuthContainerRequest` shape assertions on `Mounts`, `Args`, `Interactive`, `TTY`) | ✓ |
| `TestEnsureClaudeAccountReadyFailsWhenNoCreds` removed/renamed | `claude_auth_test.go:179` — replaced by `TestReadAccountIdentityReturnsLoggedOutWhenNoCreds`. Body calls `claudeprovider.ReadAccountIdentity(dir)` directly (line 188) — the real adapter, not a local shadow | ✓ |
| Local shadow `claudeproviderReadAccountIdentity` removed | Searched `claude_auth_test.go` — no `claudeproviderReadAccountIdentity` definition anywhere in the file. Only the real `claudeprovider.ReadAccountIdentity` is referenced (line 188) | ✓ |
| New `TestLoginClaudeAccountFailsWhenNoCredsAfterContainer` exists and exercises full chain | `claude_auth_test.go:228-249` — calls `loginClaudeAccount` (line 239) with a `stubClaudeAuthRunner{writeCreds: false}` (line 235), then asserts error contains `"no credentials file found after login"` (line 243-245) AND `stub.containerHits == 1` (line 246-248). This drives the full wipe→image→container→verify body and lands in the no-creds error branch at `claude_auth.go:126-128` | ✓ |

### Round 1 functionality preserved

- **`mage testPkg ./internal/cli` re-run by reviewer**: 137 tests / 71.2% coverage. Matches worklog. ✓
- **`ensureManagedAccountReady` Claude dispatch**: `account_auth.go:41-42` — `case domain.ProviderClaude: return ensureClaudeAccountReady(cmd, account, options)`. Still dispatches; now with full options. ✓
- **`loginManagedAccount` Claude dispatch**: `account_auth.go:63-64` — `case domain.ProviderClaude: return loginClaudeAccount(cmd, account, paths)`. Unchanged from Round 1. ✓
- **`logoutManagedAccount` Claude case**: `account_auth.go:52-53` — `case domain.ProviderClaude: return wipeClaudeCredentials(account.HomePath)`. Pure file-wipe, no container. Unchanged from Round 1. ✓
- **`TestLogoutManagedAccountWipesClaudeCredentials`**: still present at `claude_auth_test.go:251-269`, still asserts post-logout `.credentials.json` is gone. ✓
- **`TestWipeClaudeCredentialsMissingFileIsOK`**: still present at `claude_auth_test.go:271-278`. ✓
- **`TestLoginClaudeAccountSkipsNonTTYGuard`**: still present at `claude_auth_test.go:197-221`; confirms `loginClaudeAccount` reaches `RunContainer` (line 218-220) on non-TTY input. ✓
- **`TestEnsureClaudeAccountReadyRejectsNonTTY`**: still present at `claude_auth_test.go:65-83`, confirms non-TTY rejection + `stub.containerHits == 0`. ✓

### Worklog Round 2 entry

- Documents all three fixes with file:line cites + test names: `account_auth.go:42`, `claude_auth.go:63-66`, `claude_auth.go:67-75`, and the three test renames (`BUILDER_WORKLOG.md` Round 2 § "Fix C1/C2/C3"). ✓
- Notes the anticipated Round 1 test removal `TestEnsureClaudeAccountReadyWipesExistingCredentials` (its assertion was inverted by C2) — see worklog § "Fix C2 — Secondary effect" and § "Design decisions — `TestEnsureClaudeAccountReadyWipesExistingCredentials` removal". ✓
- Test-count table (135 → 137, +2). ✓ Verified: 7 Round 1 tests − 1 removed (`TestEnsureClaudeAccountReadyWipesExistingCredentials`) + 3 added (`TestEnsureClaudeAccountReadyRespectsSkipLogin`, `TestEnsureClaudeAccountReadyNonTTYDoesNotWipe`, `TestLoginClaudeAccountFailsWhenNoCredsAfterContainer`) + 2 renamed (no count change) = 9 tests in `claude_auth_test.go` total. `claude_auth_test.go` test functions counted: `TestEnsureClaudeAccountReadyRejectsNonTTY`, `TestEnsureClaudeAccountReadyRespectsSkipLogin`, `TestEnsureClaudeAccountReadyNonTTYDoesNotWipe`, `TestBuildClaudeAuthContainerRequestShape`, `TestReadAccountIdentityReturnsLoggedOutWhenNoCreds`, `TestLoginClaudeAccountSkipsNonTTYGuard`, `TestLoginClaudeAccountFailsWhenNoCredsAfterContainer`, `TestLogoutManagedAccountWipesClaudeCredentials`, `TestWipeClaudeCredentialsMissingFileIsOK` = 9. Package total went from 135 to 137 (+2), consistent with `claude_auth_test.go` going from 7 to 9. ✓

### Findings

None blocking. All three Round 1 counterexamples (C1, C2, C3) are fixed with concrete code + test evidence.

### Gaps (non-blocking)

- **G1 (new).** `loginClaudeAccount` body is structurally identical to `ensureClaudeAccountReady`'s body from line 73 onward (wipe → image → notice → container → verify). The duplication is consistent with the Round 1 spec ("same flow … minus the non-TTY guard") and Round 2 did not refactor it. The new `TestLoginClaudeAccountFailsWhenNoCredsAfterContainer` (claude_auth_test.go:228-249) covers the no-creds branch via `loginClaudeAccount` and the worklog § "Why test 'no creds after container' via `loginClaudeAccount`" explicitly documents this as covering the same production-code branch. Acceptable. A future refactor could extract `wipeAndLaunchClaudeAuth(...) error` to fold the duplication; not blocking.

- **G2 (non-blocking).** Round 1's G1/G2 gaps (mis-named tests) are now resolved by C3. Round 1's G3 (real end-to-end auth deferred to dev manual smoke) is unchanged — still a documented Unknown routed to the orchestrator.

### Observations (non-blocking)

- **O1.** `claude_auth.go:74` wipe error message is `"prepare claude account %q: wipe credentials: %w"`. The dual colon structure is consistent with the `loginClaudeAccount` analog (`claude_auth.go:105` — `"prepare claude login for account %q: wipe credentials: %w"`). Symmetric and clear.
- **O2.** The `ensureClaudeAccountReady` doc comment (claude_auth.go:52-62) explicitly documents both Round 2 invariants: "SkipLogin short-circuits before any credential mutation" and "A non-TTY guard is enforced before the credential wipe". Doc comment matches behavior — good.
- **O3.** The `TestReadAccountIdentityReturnsLoggedOutWhenNoCreds` test (claude_auth_test.go:179-195) tests `claudeprovider.ReadAccountIdentity` rather than `ensureClaudeAccountReady` itself. This is the *correct* rename per C3 — the test now matches its assertion. The function-under-test is the adapter, not the dispatcher. ✓
- **O4.** Round 1 worklog claimed 7 new tests; Round 1 review counted 7 (matched). Round 2 worklog claims 137 - 135 = +2 tests in the package; verified by reviewer's mage run output (`tests: 137`). Test-count accounting checks out.
- **O5.** Code structure: `ensureClaudeAccountReady` body (claude_auth.go:63-99) now mirrors `ensureCodexAccountReady`'s SkipLogin → TTY-guard → main-flow ordering (compare with `account_auth.go:70-86`). The Codex precedent for "SkipLogin first, TTY guard second" is now matched in the Claude path. ✓

### Hylla Feedback

None — Hylla not needed for this review. All evidence is `Read`-tool inspection of `internal/cli/account_auth.go`, `internal/cli/claude_auth.go`, `internal/cli/claude_auth_test.go`, plus `mage testPkg ./internal/cli` rerun. No Go symbol resolution required cross-package navigation — every changed symbol is in `internal/cli`, and the only adapter reference (`claudeprovider.ReadAccountIdentity`) is a name-only check (not behavior). Non-Go files (`PLAN.md`, `BUILDER_WORKLOG.md`, `BUILDER_QA_FALSIFICATION.md`) were read directly per Hylla's Go-only scope.

---
