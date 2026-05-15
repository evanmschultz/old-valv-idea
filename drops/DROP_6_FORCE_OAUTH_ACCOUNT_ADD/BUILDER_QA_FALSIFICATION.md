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

## Unit 6.2 — Round 2

**Date:** 2026-05-14
**Verdict:** pass
**Mage target re-run:** `mage testPkg ./internal/cli` — PASS (137 tests, 71.2% coverage, matches worklog)

### Summary

All three Round 1 counterexamples (C1 SkipLogin drop, C2 wipe-before-TTY-check,
C3 misnamed tests) are fully repaired. The fixes are concrete, ordered
correctly, and backed by tests that actually exercise the production code path
they claim. Coverage delta (71.1% → 71.2%, +2 tests net) is internally
consistent with the scope of changes (small refactor + test swap + one new
short-circuit). No new counterexamples constructed across 14 attack vectors.

### Counterexamples

None.

### Attack Attempts

| # | Vector | Outcome | Note |
|---|---|---|---|
| 1 | C1 fix — `SkipLogin` check is the first executable statement of `ensureClaudeAccountReady`, no mutation before it | REFUTED | `claude_auth.go:63-66`: `func ensureClaudeAccountReady(cmd, account, options accountAuthOptions) error { if options.SkipLogin { return nil }` — literal first statement; clean `return nil`. |
| 2 | C1 fix — dispatcher forwards the FULL `accountAuthOptions` (not selected fields) so future field additions don't silently drop | REFUTED | `account_auth.go:42`: `return ensureClaudeAccountReady(cmd, account, options)`. `manage.go:493,600` both build `accountAuthOptions{SkipLogin: skipLogin, Paths: paths}` end-to-end. |
| 3 | `loginClaudeAccount` should NOT check `SkipLogin` (explicit user-initiated login always attempts) | REFUTED | `claude_auth.go:103`: signature is `(cmd, account, paths config.Paths)` — no `options`, no SkipLogin gate. Correct by design. Worklog § "Non-TTY guard placement" matches. |
| 4 | C2 fix — exact ordering SkipLogin → TTY check → wipe → image → container → verify | REFUTED | `claude_auth.go:63-98`: SkipLogin (63-66) → TTY (67-72) → wipe (73-75) → EnsureImage (76-79) → notice (80-87) → RunContainer (88-90) → ReadAccountIdentity verify (91-97). Strict order. |
| 5 | C2 fix — non-TTY error string mentions "TTY" / interactive terminal | REFUTED | `claude_auth.go:69`: `"account %q is not logged in; rerun in a TTY to complete Claude device-code login"`. Mentions TTY + action. |
| 6 | C2 test (`TestEnsureClaudeAccountReadyNonTTYDoesNotWipe`) writes creds BEFORE the call, asserts file presence AFTER | REFUTED | `claude_auth_test.go:124-128` writes `.credentials.json` before call; lines 135-141 call function + assert error contains "TTY"; lines 143-145 `os.Stat(credPath)` + assert NOT `IsNotExist`. Genuine, not theater. |
| 7 | Renamed `TestBuildClaudeAuthContainerRequestShape` preserves the assertion surface of the old `*SucceedsAfterContainerWrite` test | REFUTED | `claude_auth_test.go:151-173` asserts mount count=1, mount source=tempdir, args=`[auth, login]`, Interactive=true, TTY=true. Same surface as the old test; name now matches behavior. |
| 8 | `TestLoginClaudeAccountFailsWhenNoCredsAfterContainer` stub does NOT write creds → exercises the genuine no-creds branch | REFUTED | `claude_auth_test.go:235` constructs stub with `writeCreds: false`; line 239 calls `loginClaudeAccount`; lines 243-244 assert error contains `"no credentials file found after login"`; lines 246-247 assert `containerHits == 1`. Drives the production branch at `claude_auth.go:126-128`. |
| 9 | Removed wipe-positive test — is the TTY-positive wipe path STILL covered through `loginClaudeAccount`? | REFUTED | `TestLoginClaudeAccountSkipsNonTTYGuard` (claude_auth_test.go:197-221) drives the wipe→image→container→verify path with `writeCreds: true`. `loginClaudeAccount` shares the identical body as the TTY-positive branch of `ensureClaudeAccountReady` (sans the TTY guard). Production wipe behavior IS covered via the symmetric path. Worklog § "Why test no creds after container via `loginClaudeAccount`" calls this out explicitly. |
| 10 | Coverage delta 71.1% → 71.2% is too small to be real progress | REFUTED | Removed 3 tests, added 5. New tests all have real assertions tied to production paths: SkipLogin short-circuit (new code, new test), C2 reordering (no new lines, replaced one test 1:1), C3 rename + one added test for the genuine no-creds branch. 0.1pp delta is internally consistent with the +2 net test count and the small SkipLogin code addition. No padding observed. |
| 11 | `mage testPkg ./internal/cli` re-run matches worklog | REFUTED | Re-ran in QA session: 137 tests pass, 0 failures, 71.2% coverage, 60.0% gate met. Exact match. |
| 12 | No raw `go test` / `go build` / `go vet` / `go run` in worklog or build instructions | REFUTED | Worklog cites `mage testPkg ./internal/cli` only. No raw `go` invocations. |
| 13 | Signature change of `ensureClaudeAccountReady` to `(cmd, account, options accountAuthOptions)` — no orphan caller compiles broken | REFUTED | Sole production caller is dispatcher at `account_auth.go:42` (already updated). Test callers (`claude_auth_test.go:73, 102, 135`) all use the new `accountAuthOptions{...}` form. Compile clean per `mage testPkg`. |
| 14 | Logout regression — `logoutManagedAccount` Claude case still pure wipe (not affected by SkipLogin refactor) | REFUTED | `account_auth.go:52-53`: `case domain.ProviderClaude: return wipeClaudeCredentials(account.HomePath)`. Still wipe-only, no container, no signature change. `TestLogoutManagedAccountWipesClaudeCredentials` (claude_auth_test.go:251-269) covers. |

**Side checks (self-attack):** error wrapping at every `%w` boundary
(claude_auth.go:74, 78, 86, 89, 93, 96 + 105, 109, 117, 120, 124, 127) — all
wrap with context. No concurrency surface (no goroutines, no shared mutable
state). Test isolation good (`t.TempDir()` + `t.Parallel()` everywhere). End-to-end
SkipLogin plumbing intact from `manage.go` flag → `accountAuthOptions{}` →
dispatcher → `ensureClaudeAccountReady` first-statement short-circuit.

### Hylla Feedback

None — Hylla was not queried for this round. All evidence came from `Read` on
the touched Go files (`claude_auth.go`, `claude_auth_test.go`, `account_auth.go`,
`account_auth_test.go`, `manage.go`) plus a `mage testPkg ./internal/cli`
re-run. Hylla ingest happens at drop-end, so Round 2's edits aren't in the
index yet — fall-through is expected, not a miss.

---

## Unit 6.3 — Round 1

**Date:** 2026-05-15
**Verdict:** pass
**Mage targets re-run:**
- `mage testPkg ./internal/adapters/providers/claude` — PASS (21 tests, 78.4% coverage, matches worklog)
- `mage testPkg ./internal/cli` — PASS (146 tests, 71.1% coverage, matches worklog)

### Summary

Sixteen attack vectors run against Unit 6.3's two-part change (Claude credentials
parsing in the adapter + CLI display wiring). No counterexamples constructed.
The implementation is structurally clean: presence-only `LoggedIn` semantics
preserved, optional email best-effort with graceful degradation, dispatcher
explicit-case (no fallthrough), Codex case untouched, no new dependencies, doc
comments on exported identifiers. Worklog numbers (21/78.4% adapter, 146/71.1%
CLI) re-confirmed in this session.

Two soft observations recorded but not promoted to counterexamples: (a) the
real-world `.config.json` / `oauthAccount.emailAddress` field layout is asserted
from CLI documentation rather than from a live auth run; if Anthropic's CLI ever
writes the email under a different path, tests will still pass under the wrong
assumption (this is acknowledged in the worklog's "Credentials-format
resolution" note); (b) `readClaudeConfigEmail` short-circuits on non-`IsNotExist`
read errors without trying the second filename, but per the documented "any
read/parse failure → empty email" graceful-degradation spec this is intended,
not a defect. Neither rises to a counterexample.

### Counterexamples

None.

### Attack Attempts

| # | Vector | Outcome | Note |
|---|---|---|---|
| 1 | Credentials-format assumption (`oauthAccount.emailAddress` in `.config.json`/`.claude.json`) | EXHAUSTED | Cannot construct a counterexample without a real Claude auth run. Builder acknowledged in worklog "Credentials-format resolution" section that the field name + location are based on documented CLI behavior, not a live capture. Test fixtures assume the documented shape — if shape is wrong, the parsing still degrades gracefully (empty email, LoggedIn=true). Honest unknown, not falsifiable here. |
| 2 | File-fallback ordering + permission-denied early-return | REFUTED (order) / SOFT-FINDING (permission-denied) | `account.go:63` iterates `[.config.json, .claude.json]` — matches worklog claim. Permission-denied on `.config.json` returns `""` immediately (line 71) without trying `.claude.json`. Per spec "any read/parse failure → empty email" this is documented graceful degradation, not a bug. Soft finding only — not a counterexample. |
| 3 | `LoggedIn` semantics on non-`IsNotExist` stat error | REFUTED | `account.go:47`: returns `fmt.Errorf("read claude credentials %q: %w", credPath, err)` — does NOT swallow into `LoggedIn=false`. Caller (`operator_helpers.go:236-240`) treats non-nil error as "unavailable", separate from "not logged in". Correct. |
| 4 | Symlink credentials file (to file / to directory) | REFUTED | `os.Stat` follows symlinks. Symlink-to-file: stat succeeds, `IsDir()=false`, `LoggedIn=true`. Symlink-to-dir: `IsDir()=true`, `LoggedIn=false` per line 49-51. Both correct. |
| 5 | Empty `homePath` footgun | REFUTED | `strings.TrimSpace("")="" `, `filepath.Join("", ".credentials.json")=".credentials.json"` (relative). `os.Stat` on relative path uses test CWD. Production callers: only `listItemsForAccounts` / `listItemsForBindings` in `operator_helpers.go:180-213`, both pass `profile.HomePath` from a `domain.Profile` returned by the manage store — never empty in practice. No production footgun. |
| 6 | `readClaudeConfigEmail` silent failures | REFUTED | Lines 66-77: every error path (`os.IsNotExist`, other read error, JSON unmarshal error) returns `""` per spec. No `log.Error`, no fail-stop. Matches "graceful degradation" intent. |
| 7 | `claudeEmailDisplay` returning email when `LoggedIn=false` is inconsistent with Codex | REFUTED | Cross-check: `codexEmailDisplay` (operator_helpers.go:267-279) first checks `if email := strings.TrimSpace(identity.Email); email != "" { return email }` — returns email regardless of LoggedIn flag. `claudeEmailDisplay` (line 292-300) mirrors exactly: `if email := ...; email != "" { return email }` first. Behavior is symmetric with Codex. Not an inconsistency. |
| 8 | Test fixtures realistic vs actual Claude CLI output | EXHAUSTED | Same root cause as #1 — cannot verify without real auth. Worklog acknowledges the resolution is documentation-based. Adapter package has 78.4% coverage from real file I/O via `t.TempDir()` — the parsing logic itself is exercised; only the assumed JSON shape is unverified. |
| 9 | No new dependencies in `account.go` import block | REFUTED | Imports (lines 3-9): `encoding/json`, `fmt`, `os`, `path/filepath`, `strings`. All stdlib. No new third-party imports. |
| 10 | `readAccountIdentity` dispatcher Claude case correctly added | REFUTED | `operator_helpers.go:234-245`: explicit `case domain.ProviderClaude:` calling `claudeprovider.ReadAccountIdentity`; error path returns `unavailable`; success path returns `claudeAuthDisplay` + `claudeEmailDisplay`. No fallthrough to `default`. |
| 11 | `claudeprovider` import present in `operator_helpers.go` | REFUTED | Line 15: `claudeprovider "github.com/evanmschultz/valv/internal/adapters/providers/claude"`. Aliased import in place. |
| 12 | Coverage gaming — new tests in `operator_helpers_test.go` | REFUTED | `TestClaudeAuthDisplay` (3 cases) and `TestClaudeEmailDisplay` (4 cases) each call the helper under test and assert with `if got != tc.want { t.Errorf(...) }`. Adapter tests (`account_test.go`) use `t.TempDir()` + real `os.WriteFile` + real `os.Mkdir` + real `ReadAccountIdentity` calls — genuine file I/O, not stubs. No coverage padding. |
| 13 | `mage testPkg ./internal/adapters/providers/claude` re-run matches worklog | REFUTED | Re-ran in QA session: 21 tests pass, 0 failures, 78.4% coverage, 60.0% gate met. Exact match to worklog's "21 tests, 78.4%". |
| 14 | `mage testPkg ./internal/cli` re-run matches worklog | REFUTED | Re-ran in QA session: 146 tests pass, 0 failures, 71.1% coverage, 60.0% gate met. Exact match to worklog's "146 tests, 71.1%". |
| 15 | Codex behavior unchanged (no inadvertent edit to codex case in dispatcher or codex account.go) | REFUTED | `git diff a0eda74~1 a0eda74 -- internal/adapters/providers/codex/account.go` returns empty diff (only file header line). `readAccountIdentity` Codex case (operator_helpers.go:222-233) and `codexAuthDisplay`/`codexEmailDisplay` (lines 254-279) are textually unchanged from pre-Unit-6.3 state. |
| 16 | Doc comments on exported identifiers | REFUTED | `AccountIdentity` documented (account.go:11-14); `ReadAccountIdentity` doc starts with "ReadAccountIdentity" (lines 34-39); `claudeAuthDisplay` doc starts with "claudeAuthDisplay" (lines 281-282); `claudeEmailDisplay` doc starts with "claudeEmailDisplay" (lines 290-291). Per Go convention. |

**Supplemental Go-falsification families:**

- **Concurrency.** No new goroutines, no shared mutable state, no channels, no mutex. EXHAUSTED, no counterexample.
- **Interface misuse.** No new type assertions, no new interface declarations. EXHAUSTED, no counterexample.
- **Error swallowing.** Every error from `os.Stat` / `os.ReadFile` / `json.Unmarshal` is either wrapped with `%w` and returned (line 47), or deliberately collapsed to `""` per documented "graceful degradation → empty email" spec (lines 66-77). No swallowed `_ = err` patterns. REFUTED.
- **Goroutine leaks.** N/A — no goroutines. EXHAUSTED.
- **Raw `go` commands.** Worklog cites `mage testPkg` only. No `go test`/`go build`/`go vet`/`go run` invocations. REFUTED.
- **`mage install`.** Not invoked. REFUTED.
- **Hidden dependencies / init side effects.** No `init()` functions, no package-level state mutation, no test-order coupling (all tests use `t.TempDir()` + `t.Parallel()`). REFUTED.
- **File/package gating.** Paths touched (`internal/adapters/providers/claude/account.go` + `_test.go`, `internal/cli/operator_helpers.go` + `_test.go`) match `Paths` declared in PLAN.md Unit 6.3. No edits outside declared paths. REFUTED.

### Hylla Feedback

None — Hylla was not queried for this round. All evidence came from `Read` on
the touched files plus `mage testPkg` re-runs and one `git diff a0eda74~1
a0eda74 -- codex/account.go` to confirm Codex untouched. Hylla ingest happens
at drop-end, so Unit 6.3's edits aren't in the index yet — fall-through to
`Read` is expected, not a miss.

N/A for cross-package references — both changed packages were small and fully
readable in-session.

---
