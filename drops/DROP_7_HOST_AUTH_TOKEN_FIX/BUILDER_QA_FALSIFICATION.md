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

---

## Unit 7.2 — Round 1

**Date:** 2026-05-15
**Verdict:** pass (one minor test-coverage note for AC4; not a counterexample)

### Attack Attempts

Each numbered attack vector from the spawn prompt is enumerated. CONFIRMED = counterexample produced. REFUTED = attack tried, evidence rules it out. EXHAUSTED = honest attempt, no counterexample constructable.

1. **JSON schema mismatch with Unit 7.1.** REFUTED. Unit 7.1 writes via `claudeCredentials { AccessToken string \`json:"claudeAiAccessToken"\` }` (`claude_auth.go:92-94`). Unit 7.2 reads via anonymous struct `struct { Token string \`json:"claudeAiAccessToken"\` }` (`service.go:309-311`). Both JSON tags are byte-for-byte `claudeAiAccessToken` (lowercase 'i' in "Ai"). No casing drift, no `ClaudeAiAccessToken`, no `claudeAIAccessToken`. The new test (`service_test.go:536`) writes `{"claudeAiAccessToken":"test-oauth-token"}` and asserts the round-trip — would catch any divergence.

2. **`CLAUDE_CODE_OAUTH_TOKEN` `ps` visibility comment.** REFUTED. Lines 276-279 of `service.go` carry the required acknowledgement: `// CLAUDE_CODE_OAUTH_TOKEN is passed as an environment variable and is visible to ps(1). Acceptable for v0.1.0.` Matches PLAN.md Q3 (line 43) and the planner's instruction at PLAN.md line 165.

3. **Token never logged.** REFUTED across all code paths:
   - Presence branch (`service.go:282-283`): `request.Env["CLAUDE_CODE_OAUTH_TOKEN"] = token` — no logger call.
   - Empty-file branch (`:284-285`): `s.debug("no claude credentials file found, container will run unauthed", "home", profile.HomePath)` — does NOT include `token` (which would be empty string anyway).
   - Error branch (`:280-281`): `s.debug("claude auth token unreadable", "home", profile.HomePath, "err", err)` — `err` originates from `fmt.Errorf("read claude credentials: %w", err)` or `fmt.Errorf("parse claude credentials: %w", err)` (`:306`, `:313`). Neither inner error embeds the JSON contents — `os.ReadFile` errors wrap the path, `json.Unmarshal` errors wrap a position offset but NOT the buffer.
   - The earlier service-wide debug log at `service.go:145-158` logs `env_passthrough` (variable-name list) but NOT the `Env` map values. The token never flows to a logger.

4. **Empty token / missing file UX.** EXHAUSTED — not a counterexample. `readClaudeAuthToken` returns `("", nil)` on `os.IsNotExist` (`:303-304`). The else-branch at `:284-285` logs at DebugLevel only, so under default log level the user sees no warning before the container starts unauthed. The container's `claude` will then surface its own auth-required error. This is the planner's chosen behavior per PLAN.md line 58: "Do NOT fail Run. This preserves backward compatibility with manually-authed accounts." Surfacing-loudly would be an enhancement; silence is per spec.

5. **Malformed JSON UX.** EXHAUSTED — not a counterexample. Same DebugLevel observability as V4. The error path at `:280-281` logs `claude auth token unreadable` with the wrapped err. Per PLAN.md AC4 ("logs a debug message"), this is the chosen behavior. Builder followed planner spec faithfully.

6. **Nil-map panic on `prepared.Env`.** REFUTED. Verified `clauderuntime.PrepareRuntime` (`internal/adapters/providers/claude/runtime.go:118-121`): `env := map[string]string{"CLAUDE_CONFIG_DIR": ContainerClaudeDir, "HOME": ContainerHomeDir, ...}` — initialized as a non-nil literal with seed entries on every successful return. The returned `PreparedRuntime.Env` is therefore guaranteed non-nil. `buildRequest` (`service.go:258`) assigns `Env: prepared.Env` (same reference), and `:283` writes to it. No nil-map panic possible.

7. **Path traversal via `homePath`.** REFUTED. `filepath.Join(homePath, ".credentials.json")` does NOT validate the prefix, but the threat model here is benign: `profile.HomePath` is set by the account-add flow (under managed providers root) or by test factories. The file read is always for the literal filename `.credentials.json`. A user creating an account with `name=../../etc` would have to plant their own valid JSON token file at the traversed location — self-foot-shot, not privilege escalation. Account naming validation belongs upstream (account-add service), not in this read helper. No 7.2 counterexample.

8. **Concurrent reads race.** REFUTED. `os.ReadFile` is concurrent-safe on POSIX (kernel handles atomic single-file reads under page-cache semantics for file <= one block; this file is ~50 bytes). `readClaudeAuthToken` holds no shared in-process state. Multiple parallel `valv claude` invocations on the same managed home are safe.

9. **Token mutability after `Run` starts.** REFUTED. Token read happens once in `buildRequest`, before container start. A concurrent `valv account add` rewriting `.credentials.json` after the read means the container gets the captured snapshot (old token). That is acceptable snapshot semantics for v0.1.0. No panic, no torn-write — `os.WriteFile` in Unit 7.1's writer is single-call (so an in-progress write either has not started yet or completes before next read). Per planner V9 prompt: "Acceptable trade-off for v0.1.0; verify no panic" — verified.

10. **Test rigor — `TestRunSetsClaudeCodeOAuthTokenWhenCredentialsPresent`.** REFUTED:
    - Real temp profile home (`t.TempDir()`, `service_test.go:534`).
    - `.credentials.json` written BEFORE `service.Run` call (`:535-538`).
    - Correct JSON shape `{"claudeAiAccessToken":"test-oauth-token"}` (`:536`) — matches Unit 7.1's writer schema.
    - Asserts `executor.got.Env["CLAUDE_CODE_OAUTH_TOKEN"] == "test-oauth-token"` (`:563-566`) — equality check would fail on empty or wrong value.
    - `t.TempDir()` auto-cleans on test exit via testing.T's lifecycle.
    - Test uses real `boundClaudeStore`, real `service.New`, real `service.Run`, real `buildRequest`, real `readClaudeAuthToken` — no mocked-out internals. Genuine integration-style behavior assertion.

11. **Codex regression.** REFUTED. All edits live in `internal/services/claude/service.go` + `service_test.go`. No imports of codex packages, no cross-package mutations. `git show --stat c3642f3` confirms only those two files plus drop docs. Other tests in the same package still pass per mage rerun.

12. **`mage testPkg ./internal/services/claude` rerun.** REFUTED — worklog matches. My rerun output:
    ```
    [PKG PASS] github.com/evanmschultz/valv/internal/services/claude (1.27s)
    tests: 18  passed: 18  failed: 0
    package coverage: 80.8%
    Minimum package coverage: 60.0%.
    [SUCCESS] All tests passed
    ```
    Exactly 18/18 pass, 80.8% coverage — matches worklog claim (`BUILDER_WORKLOG.md:69`) exactly. No flakes, no race detector hits.

13. **No raw `go` invocations.** REFUTED. Worklog (lines 68-69) cites only `mage testPkg ./internal/services/claude`. No `go test`, `go build`, `go run`, `go vet`, `gofumpt` invocations anywhere in the worklog or commit message. Clean.

14. **Coverage genuineness (80.8%).** REFUTED. The new test exercises:
    - Real filesystem write (`os.WriteFile` to a real temp dir).
    - Real `service.Run` → real `buildRequest` → real `readClaudeAuthToken` → real `os.ReadFile` + `json.Unmarshal`.
    - Observable side-effect assertion (the env map entry that the container would receive).

    Not coverage padding. The 80.8% number is above the 70% AGENTS.md target and well above the 60% mage gate. Existing tests in the file remain meaningful (provider-mismatch checks, path-traversal rejection, etc.). No tautological assertions, no `if true` style padding.

### Additional Adversarial Probes (Beyond the Prompt's 14 Vectors)

15. **`TestRunSucceedsWithBoundProject` regression (AC3 absent-creds case).** REFUTED. Line 208 of `service_test.go` instantiates `boundClaudeStore(project, t.TempDir())` — empty temp dir, no `.credentials.json`. With Unit 7.2's edit, `readClaudeAuthToken` returns `("", nil)` and the else-branch debug log fires. The test's existing assertions (`CLAUDE_CONFIG_DIR`, mount, label, name prefix) do NOT check `CLAUDE_CODE_OAUTH_TOKEN`, so they remain valid. The implicit guarantee — `service.Run` returns nil without crashing in the no-creds case — is verified by the test passing in the mage rerun.

16. **AC4 (malformed JSON) test pinning.** MINOR FINDING — not a counterexample. PLAN.md AC4 (line 171): "When `.credentials.json` is present but unreadable (e.g. bad JSON), `Run` succeeds (graceful skip) and logs a debug message." The worklog acknowledges this AC but the new test only covers the happy path (valid JSON, token present). There is no test that:
    - Writes garbage to `.credentials.json`,
    - Calls `service.Run`,
    - Asserts `executor.got.Env["CLAUDE_CODE_OAUTH_TOKEN"]` is empty/absent AND `Run` returned nil.

    A future regression could silently flip the error branch from "log + omit" to "return err from buildRequest" without breaking any test. The implementation is correct today (`service.go:280-286` clearly does graceful skip), and the AC4 behavior is one if-branch — low regression risk. But the AC is not test-pinned. **Recommendation for Unit 7.3:** add `TestRunOmitsClaudeCodeOAuthTokenWhenCredentialsMalformed` to pin AC4. Note: PLAN.md Unit 7.3 spec (line 228) mentions a `TestRunOmitsClaudeCodeOAuthTokenWhenCredentialsMissing` for the absent case, but not a malformed case. 7.3 builder may want to add both.

17. **`prepared.Env` aliasing — shared mutable state.** REFUTED (accepted). `buildRequest` assigns `Env: prepared.Env` (`service.go:258`) — same map reference. Then writes `request.Env["CLAUDE_CODE_OAUTH_TOKEN"] = token` (`:283`). This mutation is visible to anyone else holding a reference to `prepared.Env`. In practice:
    - `prepared` is returned from `clauderuntime.PrepareRuntime` to a single owner (`Service.Run`'s local `prepared` variable).
    - `Service.Run` immediately defers `prepared.Close()` and passes `prepared` by value to `s.buildRequest`. The Env map reference is shared but no other code reads it.
    - On `prepared.Close()`, the cleanup func runs but does not iterate `prepared.Env`.

    No aliasing bug. Single-writer single-reader. Safe.

18. **Token-bearing error in fmt.Errorf wraps.** REFUTED. Walked `readClaudeAuthToken` error paths:
    - `:306` `fmt.Errorf("read claude credentials: %w", err)` — `err` is the `os.ReadFile` error, which contains the path but not file contents.
    - `:313` `fmt.Errorf("parse claude credentials: %w", err)` — `err` is `*json.SyntaxError` or `*json.UnmarshalTypeError`, which contain position/offset/type info but NOT the raw input.
    - Neither error carries the token. Safe to bubble to callers.

19. **Env var name collision.** REFUTED. `prepared.Env` seeds `CLAUDE_CONFIG_DIR` and `HOME` (runtime.go:118-121). Unit 7.2 adds `CLAUDE_CODE_OAUTH_TOKEN`. No collision; three distinct keys. Container's `claude` reads `CLAUDE_CODE_OAUTH_TOKEN` per Anthropic docs (verified by planner Q2 acceptance + dev's 2026-05-15 live test).

20. **Error from `buildRequest` other than token-read.** REFUTED. The only error return from `buildRequest` is the `withinProjectRoot` check (`:246-252`). The token-read block at `:280-286` does NOT propagate errors from `readClaudeAuthToken`; it logs and continues. This means a corrupted creds file CANNOT fail `Run` — matches AC4 graceful-skip semantics.

### Counterexamples

None CONFIRMED.

### Findings Summary

- **V16 (AC4 not test-pinned)** — minor test-coverage gap. Unit 7.3 should add `TestRunOmitsClaudeCodeOAuthTokenWhenCredentialsMalformed` to pin the graceful-skip behavior on bad JSON. Not a Unit 7.2 reject — the implementation is correct.
- **V4 / V5 (debug-level logging on auth-skip paths)** — accepted per PLAN.md line 58. UX-only consideration; future polish could promote these to Info or Warn level so users see "no creds, container starting unauthed" without `--log-level=debug`. Not in scope for v0.1.0.

### Verdict

**pass.** Unit 7.2 holds against all 14 spawn-prompt vectors plus 6 additional adversarial probes (V15-V20). JSON schema matches Unit 7.1 exactly. Token never logged on any path. `prepared.Env` is provably non-nil (verified against `runtime.go`). New test is a genuine behavior assertion (token round-trips disk → container env). Mage rerun confirms 18/18 pass at 80.8% coverage. Graceful-skip semantics for missing/malformed credentials match the planner's locked decision. The one minor finding (AC4 not test-pinned) is a Unit 7.3 recommendation, not a 7.2 defect.

## Hylla Feedback

- **Query:** `hylla_search_keyword` for `PrepareRuntime` against `github.com/evanmschultz/valv@main`, snapshot 8.
  - **Worked correctly.** Returned the Claude provider's `PrepareRuntime` symbol immediately, allowing me to navigate to `runtime.go` and verify the `Env` map non-nil contract for V6. The summary docstring was sufficient on its own — `"normalizes and ensures paths, stages shared home if needed by copying to a temp runtime dir, creates mount specs and environment variables, and returns a PreparedRuntime with a cleanup function ..."` — but I still opened the source to confirm the literal `map[string]string{...}` initialization.
  - **Suggestion:** None — this was an ideal Hylla hit.

- **Mid-drop staleness note:** Unit 7.2's `readClaudeAuthToken` was committed in `c3642f3` AFTER snapshot 8 ingested. Hylla would not have found the new symbol if I had searched for it. I did not need to — direct `Read` was the right tool for the implementation-under-review. This is the same drop-end-only reingest pattern noted in Unit 7.1's feedback, not a Hylla bug.
