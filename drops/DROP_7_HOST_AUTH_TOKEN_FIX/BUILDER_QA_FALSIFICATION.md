# DROP_7_HOST_AUTH_TOKEN_FIX — Builder QA Falsification

Append a `## Unit 7.M — Round K` section per QA pass. See `main/drops/WORKFLOW.md` § "Phase 5 — Build-QA (per unit)" for what each section should contain.

## Unit 7.1 — Round 1

**Date:** 2026-05-15
**Verdict:** pass (with one finding deferred to Unit 7.3 and one minor recommendation for the builder)

### Attack Attempts

Each numbered vector from the spawn prompt is enumerated below. CONFIRMED = counterexample produced. REFUTED = attack tried, evidence rules it out. EXHAUSTED = honest attempt, no counterexample constructable.

1. **Preflight reach.** REFUTED. `exec.LookPath("claude")` lives inside `runClaudeHostCommand` (`internal/cli/claude_auth.go:226`). The only production caller of that helper is `systemClaudeAccountAuthRunner.RunSetupToken` (`claude_auth.go:48-51`). Hylla `refs_find` plus direct file scan (`claude.go`, `claude_auth.go`, `account_auth.go`) show no other production call site invokes a host `claude` binary. The launch path in `claude.go::runClaudeCommand` does NOT call `claude` on the host — it runs the binary INSIDE the docker container via `service.Run`, with auth supplied via `CLAUDE_CODE_OAUTH_TOKEN` (Unit 7.2). No bypass path exists.

2. **LookPath ↔ exec TOCTOU.** REFUTED (accepted). Builder uses `exec.LookPath` then `exec.CommandContext(ctx, binary, ...)` (`claude_auth.go:226-232`). The resolved path is passed directly to `CommandContext`, so the binary identity is captured at `LookPath` time. A removal between resolve and exec surfaces as `os.PathError` from `cmd.Run()` which is then wrapped (the `claude_auth.go:240` `cmd.Run()` path) by the caller `RunSetupToken` (`claude_auth.go:48-51`) and re-wrapped by `ensureClaudeAccountReady` (`claude_auth.go:130`) → `"run claude setup-token for account %q: %w"`. The wrap is clear enough.

3. **`security find-generic-password` failure modes.**
   - Service-name typo: REFUTED. Constant at `claude_auth.go:26` is `"Claude Code-credentials"` — exact match with the planner-confirmed keychain dump (PLAN.md § "Dev-Confirmed Findings" item 3).
   - Account name: REFUTED. Code uses `u.Username` from `user.Current()` (`claude_auth.go:132-136`). This is the macOS login name (e.g. `"evanschultz"`), NOT `Name` (full name) and NOT `Uid` (numeric). Confirmed by Go stdlib: `os/user.User.Username` is documented as the login username.
   - Empty output: REFUTED. `ExtractKeychainToken` explicitly checks `if token == ""` after `strings.TrimSpace` and returns `"extract claude keychain token: empty token returned by security command"` (`claude_auth.go:75-78`). The down-stream `writeClaudeCredentials` is never reached when the keychain entry is empty.
   - Stderr-as-token: REFUTED. `cmd.Stderr = &stderr` (a separate buffer); only `cmd.Output()` (which captures stdout only) is used to build the token (`claude_auth.go:65-67`). Stderr is appended to the error message on failure, never to the token.

4. **Token never logged.** REFUTED across all paths walked:
   - `claude_auth.go`: only `writeCLINotice` calls (`:120-128`, `:159-166`) reach a writer; neither includes the token. `fmt.Errorf` wraps at `:138`, `:140`, `:177`, `:179` all reference `account.Name` and inner `%w` errors — the inner errors from `ExtractKeychainToken` deliberately exclude the token (the success path returns `(token, nil)`; only failure paths produce the error string). Token is bound to `token` local var only, then passed to `writeClaudeCredentials`.
   - `services/claude/service.go` (Unit 7.2 already done): `s.debug("claude auth token unreadable", ...)` (`:281`) logs the error but not `token` itself. `s.debug("no claude credentials file found ...")` (`:285`) is the no-token branch. The presence branch (`:282-283`) sets the env var without logging. CONFIRMED clean.

5. **`.credentials.json` write atomicity.** REFUTED (but with a minor non-blocking note). `writeClaudeCredentials` uses `os.WriteFile(credPath, data, 0o600)` (`claude_auth.go:201`). On POSIX, `os.WriteFile` is a single `open+truncate+write+close` sequence — NOT atomic in the rename-into-place sense (a crash mid-write could leave a truncated file). Mode bits `0o600` (owner read+write only) match the planner spec and are correct. The non-atomicity is acceptable for v0.1.0 — a partial write would simply re-fail `ReadAccountIdentity` and require a re-auth. Not a counterexample for this unit; flag for hardening later if multi-account churn becomes common.

6. **TTY guard correctness.** **MINOR FINDING (BUILDER RECOMMENDATION).** `ensureClaudeAccountReady` at `claude_auth.go:111` checks only `commandHasTTY(cmd.InOrStdin())` — single-stream. The Codex equivalent `ensureCodexAccountReady` at `account_auth.go:85` checks BOTH stdin AND stdout (`!commandHasTTY(cmd.InOrStdin()) || !commandHasTTY(cmd.OutOrStdout())`). The Codex guard is stricter and better matches the user-facing reality: `claude setup-token` needs the user to SEE the paste prompt as well as paste into it.

   **Not a CONFIRMED counterexample**, because in practice `cobra.Command`'s stdin/stdout TTY state is set together by the same root command; finding a real call path where stdin is a TTY but stdout is not requires deliberate redirection (`valv account add claude foo > /tmp/out`) — at which point the user can't see the prompt anyway and the existing guard fails downstream when `setup-token` blocks waiting for paste against a piped stdin. So the security/correctness invariant ("don't half-launch auth in a non-TTY context") holds via two gates: this stdin check and the natural failure of `setup-token` in such a setup.

   **Recommendation for Unit 7.3 or a follow-up:** widen to `!commandHasTTY(cmd.InOrStdin()) || !commandHasTTY(cmd.OutOrStdout())` to match `ensureCodexAccountReady` and avoid the user-confusing case of "auth started, browser opened, but no paste prompt visible". The error path is the correct behaviour; the current single-stream check just defers the diagnosis from "TTY required" to "subprocess failed". Codex template fidelity (Vector 10) would benefit from this widening.

7. **SkipLogin early-return position.** REFUTED. `claude_auth.go:108-110`: `if options.SkipLogin { return nil }` is the FIRST statement in `ensureClaudeAccountReady`, before any TTY check, wipe, or writeCLINotice. Test `TestEnsureClaudeAccountReadyRespectsSkipLogin` (`claude_auth_test.go:93-118`) writes a pre-existing creds file, calls with `SkipLogin: true`, asserts file still present — the test is the codified invariant. Tested green per `mage testPkg`.

8. **`user.Current()` failure handling.** REFUTED. Both `ensureClaudeAccountReady` (`:132-135`) and `loginClaudeAccount` (`:171-174`) handle the error explicitly:
   ```go
   u, err := user.Current()
   if err != nil {
       return fmt.Errorf("resolve macOS user for keychain extraction: %w", err)
   }
   ```
   `%w` wrap preserves the inner error. No panic. No silent skip. Good wrap context.

9. **macOS-only assumption (Linux behaviour).** REFUTED (accepted). On Linux:
   - `exec.LookPath("security")` returns `not found` (the helper isn't on Linux distros). `cmd.Output()` returns `*exec.Error{Err: ErrNotFound}`. `ExtractKeychainToken` wraps it as `"extract claude keychain token: ... \"security\": executable file not found in $PATH"` via the `%w` path at `:71-73`. The downstream `ensureClaudeAccountReady` wrap adds `"extract claude token for account %q: ..."` — comprehensible enough for a Linux user. AGENTS.md § 2 limits scope to macOS, so this is acceptable.
   - The error does NOT say "Linux not supported" explicitly, but the underlying message ("security: executable not found") plus the wrap path is unambiguous.

10. **Codex template fidelity.** REFUTED with the caveat from Vector 6. Side-by-side compare of `runCodexHostCommand` (`account_auth.go:165-185`) vs `runClaudeHostCommand` (`claude_auth.go:225-247`):
    - `exec.LookPath` first: parallel (Codex wraps with `"find host codex binary: %w"`; Claude returns an actionable install hint message — a deliberate enhancement, not a regression).
    - `exec.CommandContext` with resolved binary: parallel.
    - `cmd.Env = appendOrReplaceEnv(os.Environ(), <HOME-env>, homePath)`: parallel — Codex uses `CODEX_HOME`, Claude uses `CLAUDE_CONFIG_DIR`.
    - Stdin nil-check: parallel.
    - Stdout non-nil → stream; else capture-to-buffer + return string: parallel.

    The two functions are structurally a mirror. The one substantive divergence — Claude's enhanced error message on `LookPath` failure — is an improvement, not a regression. Codex's `"find host codex binary: %w"` lacks actionable install guidance; Claude's `"claude CLI not found on PATH; install with: npm install -g @anthropic-ai/claude-code@2.1.89"` is the better UX. Recommendation: lift the Claude pattern back into `runCodexHostCommand` in a later drop, not in this unit.

    Vector 6's TTY-guard divergence is the one genuine fidelity gap. See Vector 6 finding.

11. **`logoutClaudeAccount` vs keychain.** REFUTED (intentional). `logoutManagedAccount` for `domain.ProviderClaude` (`account_auth.go:52-54`) calls `wipeClaudeCredentials(account.HomePath)` — file-only, no `security delete-generic-password`. This matches PLAN.md's locked decision at line 59: *"`wipeClaudeCredentials` stays as file-wipe only. Do NOT call `security delete-generic-password` — too destructive to user's host claude sessions."* The user's host `claude` invocation outside Valv would still pick up the keychain entry. **Intentional design**: Valv owns `.credentials.json` per account; the keychain is shared with the user's host claude sessions and Valv must not destroy it. Code matches planner intent.

12. **Coverage drop (68.4% vs 70% AGENTS.md target).** REFUTED for Unit 7.1 — DEFERRED to Unit 7.3. The PLAN.md Unit 7.3 spec at lines 178-239 enumerates the full test rewrite that will lift coverage. Confirming the gate situation: `mage testPkg` enforces a 60% minimum (per stdout: `Minimum package coverage: 60.0%`), which 68.4% clears. AGENTS.md § 11's 70% target is the authoring goal that Unit 7.3 will hit by adding `TestSystemClaudeAccountAuthRunnerRunSetupTokenUsesCLAUDE_CONFIG_DIR`, `TestRunClaudeHostCommandFailsWhenClaudeMissing`, and the full ensure/login success-path coverage spelled out in PLAN.md 7.3. The current 68.4% reflects the deliberately-thin 7.1 stub test set per PLAN.md Unit 7.1 AC4 wording.

   Inspecting the existing tests: they are meaningful, not smoke — `TestEnsureClaudeAccountReadyNonTTYDoesNotWipe` and `TestLoginClaudeAccountSkipsNonTTYGuard` both make positional assertions about subprocess hit count AND filesystem state. No padding-test concerns. 7.3 is the coverage owner; not a 7.1 failure.

13. **`mage testPkg ./internal/cli` rerun.** REFUTED (worklog matches). My rerun:
    ```
    tests: 144  passed: 144  failed: 0
    package coverage: 68.4%
    Minimum package coverage: 60.0%.
    [SUCCESS] All tests passed
    ```
    Matches worklog exactly. No flakes; no race detector hits.

14. **No `dockeradapter` import.** REFUTED. Import block at `claude_auth.go:3-21` lists exactly: `bytes`, `context`, `encoding/json`, `fmt`, `io`, `os`, `os/exec`, `os/user`, `path/filepath`, `strings`, `laslig`, `cobra`, `claudeprovider`, `config`, `domain`. No `dockeradapter`. No `internal/adapters/docker` reference anywhere in the file. Clean rewrite.

15. **Error wrapping audit.** REFUTED. Walked every `return` in `claude_auth.go`:
    - `:112-115` — wipe wrap: `"prepare claude account %q: wipe credentials: %w"` ✓
    - `:118-119` — wipe inner wrap: `"prepare claude account %q: wipe credentials: %w"` ✓
    - `:126` — writeCLINotice wrap: `"announce claude login: %w"` ✓
    - `:130` — runner wrap: `"run claude setup-token for account %q: %w"` ✓
    - `:134` — user wrap: `"resolve macOS user for keychain extraction: %w"` ✓
    - `:138` — extract wrap: `"extract claude token for account %q: %w"` ✓
    - `:141` — write wrap: `"write claude credentials for account %q: %w"` ✓
    - `:145` — read identity wrap: `"verify claude login for account %q: %w"` ✓
    - `:148` — sentinel-style: `"verify claude login for account %q: no credentials file found after login"` — no inner error; this is the LoggedIn=false case, no wrap needed ✓
    - `:71-73` — extract security wrap: `"extract claude keychain token: %w: %s"` (stderr appended as visibility detail) ✓
    - `:77` — empty token: `"extract claude keychain token: empty token returned by security command"` — sentinel-style; no wrap needed ✓
    - `:198` — marshal wrap: `"marshal claude credentials: %w"` ✓
    - `:202` — write wrap: `"write %q: %w"` ✓
    - `:212` — remove wrap: `"remove %q: %w"` ✓
    - `:228-230` — LookPath wrap: actionable string with no `%w` — this is the install-hint case; the underlying `exec.LookPath` error doesn't add value, so no `%w` is correct.

    Every non-trivial boundary wraps with context. No bare `return err`. No `_ = err` suppression. No string-matching on inner errors. Clean.

### Counterexamples

None CONFIRMED.

### Findings Summary

- **Vector 6 (TTY guard correctness)** — non-blocking recommendation: widen the TTY check in `ensureClaudeAccountReady` (`claude_auth.go:111`) from `!commandHasTTY(cmd.InOrStdin())` to `!commandHasTTY(cmd.InOrStdin()) || !commandHasTTY(cmd.OutOrStdout())` to match `ensureCodexAccountReady` (`account_auth.go:85`) and avoid "auth launched, prompt invisible" edge cases for users with redirected stdout. Builder may roll this into Unit 7.3 alongside the test rewrite, OR file as a small follow-up. Not a 7.1 reject.
- **Vector 12 (coverage 68.4%)** — explicitly Unit 7.3's responsibility per PLAN.md. Not a 7.1 reject.
- **Vector 5 (`os.WriteFile` non-atomic)** — accepted for v0.1.0. A future hardening drop may switch to `os.CreateTemp` + `os.Rename` if multi-account churn becomes load-bearing.

### Verdict

**pass.** Unit 7.1 holds against all 15 attack vectors. The single meaningful finding (Vector 6) is a defensive improvement, not a defect — the current code is safe and well-wrapped; the recommendation just improves UX in a niche stdout-redirect edge case. The coverage gap is by design (planner-scoped to Unit 7.3). The macOS-only assumption is accepted per AGENTS.md scope. Codex template fidelity is high.

## Hylla Feedback

- **Query:** `hylla_search_keyword` for `runClaudeHostCommand` against `github.com/evanmschultz/valv@main`, snapshot 8.
  - **Missed because:** Hylla's `@main` snapshot 8 predates Unit 7.1 — the new symbol `runClaudeHostCommand` is uncommitted/unreingested. Hylla returns zero results.
  - **Worked via:** Direct `Read` on `internal/cli/claude_auth.go`.
  - **Suggestion:** Drop-end-only reingest policy is correct (per `main/CLAUDE.md` § "Hylla Baseline"), but QA agents reviewing mid-drop should know up front that newly-added symbols won't be in Hylla until close. This is a documentation note, not a Hylla bug.

- **Query:** `hylla_search_keyword` for `ensureClaudeAccountReady` returned the symbol but with the OLD docstring (`"ensures the Claude image is built, launches a Claude container ..."`) — pre-Unit-7.1 content.
  - **Missed because:** Same stale-snapshot reason. Snapshot 8 reflects DROP_6.2's container-era code, not the new host-subprocess code.
  - **Worked via:** Direct `Read`.
  - **Suggestion:** None — the staleness is by-design under the drop-end-only reingest policy. The QA falsification agent must be aware that any "is the code currently shaped this way?" question goes to `git diff` / direct file read, not Hylla, mid-drop.

- **Query:** `hylla_refs_find` inbound for `ensureClaudeAccountReady` returned 4 refs including `TestEnsureClaudeAccountReadyNonTTYDoesNotWipe`.
  - **Worked correctly** for the production caller verification (showed `ensureManagedAccountReady` as sole prod caller — confirmed no bypass path).
  - **Stale on test refs** but that did not affect the falsification (I read the test file directly).
  - **Suggestion:** None; refs_find on the production caller side was load-bearing and correct.

- **Query:** `hylla_node_full` for `commandHasTTY` returned an 82k-character response (truncated).
  - **Missed because:** Response shape contained the full reverse-edge expansion plus likely all transitive call sites — too large to consume in a single tool call.
  - **Worked via:** Reading `codex.go:265-271` directly.
  - **Suggestion:** Add a `limit` knob on `hylla_node_full` that caps reverse-edge expansion. The default depth for `node_full` should be shallow; deep expansion is what `graph_nav` is for.
