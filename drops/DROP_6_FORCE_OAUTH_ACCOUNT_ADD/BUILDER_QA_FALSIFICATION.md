# DROP_6_FORCE_OAUTH_ACCOUNT_ADD — Builder QA Falsification

Append a `## Unit 6.M — Round K` section per QA falsification pass.

## Unit 6.2 — Round 1

**Date:** 2026-05-14
**Verdict:** fail
**Mage target re-run:** `mage testPkg ./internal/cli` — PASS (135 tests, 71.1% coverage, matches worklog)

### Summary

Three counterexamples found. The most serious is a behavioral-asymmetry bug:
`ensureClaudeAccountReady` silently drops `options.SkipLogin`, so
`valv manage account add claude … --skip-login` and
`valv manage account switch claude … --skip-login` still wipe credentials and
force the auth flow. This is a contract drift from `ensureCodexAccountReady`,
which honors `SkipLogin`. Two secondary issues: (a) the wipe happens BEFORE the
non-TTY guard, so a non-TTY caller silently destroys valid pre-existing
credentials before being told to "rerun in a TTY", and (b) two new tests in
`claude_auth_test.go` are coverage-padding — their names assert behavior on
`ensureClaudeAccountReady` but the test bodies never call the function under
test.

### Counterexamples

#### C1 — `options.SkipLogin` silently ignored for Claude (CRITICAL contract drift)

**Location:** `internal/cli/account_auth.go:41-42`

```go
case domain.ProviderClaude:
    return ensureClaudeAccountReady(cmd, account, options.Paths)
```

Only `options.Paths` is forwarded; `options.SkipLogin` is dropped on the floor.
Compare with the Codex branch (line 40) which forwards the full `options` value
into `ensureCodexAccountReady`, which then short-circuits on `SkipLogin` at
lines 71–73:

```go
if options.SkipLogin {
    return nil
}
```

`ensureClaudeAccountReady` (claude_auth.go:59-92) has no `SkipLogin` parameter
or branch at all. The function unconditionally:

1. Wipes `.credentials.json` (line 60-62).
2. Errors out non-TTY callers (line 63-68).
3. Ensures the Claude image (which may trigger a docker build).
4. Launches the auth container.
5. Verifies credentials.

**Trigger:**
- `valv manage account add claude personal --skip-login` (the `--skip-login`
  flag is registered for `account add` per `newManageAccountAddCommand` at
  `manage.go:223`).
- `valv manage account switch claude personal --skip-login` (the `--skip-login`
  flag is registered for `account switch` per `newManageAccountSwitchCommand`
  at `manage.go:355`).

**Observable behavior:** the flag is silently a no-op for Claude. Worse, on
account switch the user expects `--skip-login` to leave existing credentials
alone (the documented help text says "skip host-side account login for advanced
automation"); instead, valid credentials are wiped and an unrelated TTY error
is returned.

**One-line fix:** thread the full options through:

```go
case domain.ProviderClaude:
    return ensureClaudeAccountReady(cmd, account, options)
```

…and add an early `if options.SkipLogin { return nil }` at the top of
`ensureClaudeAccountReady`, mirroring Codex. Same change for
`loginClaudeAccount` is not required (explicit `account login` is user-initiated
per the existing spec).

---

#### C2 — Non-TTY guard ordered AFTER credential wipe (destructive UX)

**Location:** `internal/cli/claude_auth.go:60-68`

```go
if err := wipeClaudeCredentials(account.HomePath); err != nil {
    return fmt.Errorf("prepare claude account %q: wipe credentials: %w", account.Name, err)
}
if !commandHasTTY(cmd.InOrStdin()) {
    return fmt.Errorf(
        "account %q is not logged in; rerun in a TTY to complete Claude device-code login",
        account.Name,
    )
}
```

`wipeClaudeCredentials` runs unconditionally before the TTY check. A non-TTY
caller with valid pre-existing credentials (typical scenario: CI runner, or a
user piping `valv` output) loses their credentials and only THEN receives the
"rerun in TTY" message. The user must then re-auth in a TTY to recover state
that was just destroyed.

Worth noting this is partly mitigated by the `account add` semantics from PLAN
("always force fresh OAuth") — if the only caller were `account add`, the wipe
is intended. But the same dispatcher is reached via `account switch` (which
should NOT destroy existing creds before failing) and indirectly via
`ensureBoundCodexAccountReady` (codex.go:217, when a project is bound to a
Claude profile — admittedly a degenerate case).

**One-line fix:** reorder — TTY check first, then wipe + container. The wipe
should only occur when the function is committed to running the full auth
flow.

---

#### C3 — Two tests do not exercise the function their names claim (coverage padding)

**Location:** `internal/cli/claude_auth_test.go:110-157` (`TestEnsureClaudeAccountReadySucceedsAfterContainerWrite`)
and `claude_auth_test.go:159-178` (`TestEnsureClaudeAccountReadyFailsWhenNoCreds`).

Both tests are named for behavior of `ensureClaudeAccountReady`, but neither
test body invokes `ensureClaudeAccountReady`:

- `TestEnsureClaudeAccountReadySucceedsAfterContainerWrite` calls
  `wipeClaudeCredentials`, then `os.WriteFile`, then constructs a stub, then
  calls `buildClaudeAuthContainerRequest` and asserts on its return shape.
  Nothing calls `ensureClaudeAccountReady`. The success path of the
  named-under-test function is never exercised.
- `TestEnsureClaudeAccountReadyFailsWhenNoCreds` calls `wipeClaudeCredentials`,
  then calls a test-local shadow `claudeproviderReadAccountIdentity`
  (defined inline at lines 182-196). The comment claims "to avoid import cycle
  issues" but no cycle exists — the production code at `claude_auth.go:84`
  calls `claudeprovider.ReadAccountIdentity` from the same `package cli` and
  compiles fine. The test is asserting behavior on its own duplicated helper
  function rather than the production code.

Net coverage effect: the `ensureClaudeAccountReady` "no credentials after
container exit" error branch (lines 88-89) is never directly tested. The
asserted lines are the wipe helper, the inline shadow, and `buildClaudeAuth
ContainerRequest`, all of which would be covered by simpler, accurately-named
tests.

**One-line fix:** either (a) rename the tests to reflect what they actually
exercise (e.g. `TestBuildClaudeAuthContainerRequestShape`,
`TestReadAccountIdentityReturnsLoggedOutWhenAbsent`), or (b) refactor
`ensureClaudeAccountReady` so the TTY check is pluggable, and write a real
end-to-end stub-based test that drives the full function through wipe →
runner.EnsureImage → runner.RunContainer → identity verify.

### Attack Attempts

| # | Vector | Outcome | Note |
|---|---|---|---|
| 1 | `loginManagedAccount` signature change risk — all call sites updated | REFUTED | `manage.go:620` passes `paths`; `claude_auth.go` calls `loginClaudeAccount(cmd, account, paths)` via dispatcher; no other prod call sites. `mage testPkg ./internal/cli` compiles + 135 pass. |
| 2 | Container args — `["auth", "login"]` literal | REFUTED | `claude_auth.go:158` — exact literal `Args: []string{"auth", "login"}`. Worklog U1 resolution confirmed against `valv-claude:dev` container help output. |
| 3 | Credentials wipe target — must be `.credentials.json` only, not RemoveAll on home | REFUTED | `claude_auth.go:127-133` — `os.Remove(filepath.Join(homePath, ".credentials.json"))`, not `os.RemoveAll`, single file target. |
| 4 | Mount target — must be `ContainerClaudeDir`, not `/home/valv` or `/root/.claude` | REFUTED | `claude_auth.go:155` — `NewMountSpec(account.HomePath, claudeprovider.ContainerClaudeDir, false)`. `ContainerClaudeDir = "/home/valv/.claude"` per `runtime.go:22`. NewMountSpec signature `(source, target, readOnly)` per `internal/adapters/docker/types.go::NewMountSpec`. |
| 5 | Post-exit verification path — must match credentials file location | REFUTED | `claude_auth.go:84` calls `claudeprovider.ReadAccountIdentity(account.HomePath)`. `ReadAccountIdentity` (account.go:24) reads `homePath + "/.credentials.json"`. Container writes to `/home/valv/.claude/.credentials.json` which is the same host file via bind mount. Paths align. |
| 6 | Non-TTY guard placement — `ensureClaudeAccountReady` only, not `loginClaudeAccount` | PARTIAL-REFUTED | Guard placement is correct (claude_auth.go:63-68 has it; `loginClaudeAccount` lines 96-123 does not). But the guard sits AFTER the wipe — see C2. |
| 7 | `logoutManagedAccount` Claude case — file wipe only, no container | REFUTED | `account_auth.go:52-53` — returns `wipeClaudeCredentials(account.HomePath)` with no container launch. |
| 8 | `TestProviderClaudeAccountAuthStubs` removal correctness | REFUTED | The removed test asserted the no-op stubs returned nil. After flip the stubs are real implementations; the assertions made no sense. New behavior is covered (imperfectly per C3, but covered) by `TestEnsureClaudeAccountReadyRejectsNonTTY`, `TestLoginClaudeAccountSkipsNonTTYGuard`, `TestLogoutManagedAccountWipesClaudeCredentials`. |
| 9 | Test injection isolation — production path falls back to `systemClaudeAuthRunner` | REFUTED | `claude_auth.go:45-50` — `claudeAuthRunnerFromContext` returns `systemClaudeAuthRunner{cmd: cmd}` when no context value or value is nil. Type assertion + nil check both gate the test-stub branch. |
| 10 | Image-only path regression (`valv claude --help`) | REFUTED | `runClaudeImageOnlyCommand` (claude.go:123-161) unchanged. It calls `ensureClaudeImageCurrent` independently; no interference from the new auth flow. |
| 11 | Coverage gaming — 71.1% achieved, are new tests meaningful? | CONFIRMED | See C3 — two tests are mis-named / don't exercise the function under test. |
| 12 | Error wrapping in `claude_auth.go` — every boundary `%w`-wrapped | REFUTED | Lines 61, 71, 79, 82, 86, 89, 98, 102, 110, 113, 117, 120, 130 all wrap with context + `%w`. No bare returns at boundaries. |
| 13 | Re-run `mage testPkg ./internal/cli` — verify 135 pass / 71.1% | REFUTED | Re-ran in QA session. 135 tests pass, 71.1% coverage, 60.0% gate met. Worklog accurate. |
| 14 | No raw `go` invocations in worklog or tests | REFUTED | Worklog cites `mage testPkg ./internal/cli` only. U1 resolution used `docker run` (legitimate, not a Go build tool bypass). |
| 15 (added) | `options.SkipLogin` honored for Claude branch | CONFIRMED | See C1 — `options.SkipLogin` is dropped at the dispatcher; Claude path never honors the flag. |
| 16 (added) | Wipe ordering vs TTY guard — destructive failure mode | CONFIRMED | See C2 — wipe runs before TTY check; non-TTY callers lose valid credentials before being told to rerun in TTY. |

### Hylla Feedback

Hylla was queried for `loginManagedAccount`, `NewMountSpec`, and
`currentContainerUser`. The first query returned stale pre-builder-change data
(the `loginManagedAccount` summary still describes the old 3-arg dispatcher
with the Claude no-op stub). This is expected — Hylla is ingest-at-drop-end,
and Unit 6.2's changes haven't been ingested yet. Falling back to `Read` on
`internal/cli/account_auth.go` returned the current shape.

- **Miss:** stale `loginManagedAccount` summary describing pre-Unit-6.2 stubs.
- **Worked via:** `Read` on `account_auth.go`.
- **Not actionable for Hylla** — this is the documented "stale until drop-end
  reingest" behavior, not a Hylla defect.

Other queries (`NewMountSpec`, `currentContainerUser`) returned accurate
summaries since those symbols were unchanged in Unit 6.2.

No Hylla miss on currently-committed Go that warrants a feedback entry. The
stale-summary issue is the expected pre-reingest state, not a real miss.

---
