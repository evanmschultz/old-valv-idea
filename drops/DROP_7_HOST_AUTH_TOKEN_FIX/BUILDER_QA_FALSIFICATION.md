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

---

## Unit 7.3 — Round 1

**Date:** 2026-05-15
**Verdict:** pass (one minor test-coverage gap; not a counterexample)

### Attack Attempts

Each of the 14 spawn-prompt vectors is enumerated, plus 6 additional adversarial probes (V15-V20). CONFIRMED = counterexample produced. REFUTED = attack tried, evidence rules it out. EXHAUSTED = honest attempt, no counterexample constructable.

1. **TTY-guard widening regression.** REFUTED with a minor test-coverage gap. Production guard at `claude_auth.go:111` reads `!commandHasTTY(cmd.InOrStdin()) || !commandHasTTY(cmd.OutOrStdout())` — byte-for-byte identical to `account_auth.go:85` (Codex twin). Structural identity confirmed. **However:** `TestEnsureClaudeAccountReadyRejectsNonTTY` (line 94) and `TestEnsureClaudeAccountReadyNonTTYDoesNotWipe` (line 147) both use `newTestClaudeCmd()` which sets BOTH stdin (`bytes.NewBuffer(nil)`) AND stdout (`&bytes.Buffer{}`) — both non-TTY. The `||` short-circuits on the first failing clause, so the new `|| !commandHasTTY(cmd.OutOrStdout())` branch is NEVER exclusively exercised. No test covers "stdin-TTY-but-stdout-pipe". A future regression that reverts the stdout half of the OR would pass all current tests. This is a test-coverage gap, not a production bug.

2. **Empty-token sentinel error message clarity.** REFUTED. Error message at `claude_auth.go:140-142` reads `"extract claude token for account %q: keychain returned empty token"`. Reasonable diagnostic clarity — names the account, the operation (extract), and the failure mode (empty token). Not maximally actionable ("re-run setup-token" guidance is not present), but acceptable for v0.1.0. The deeper keychain-side message at `:77` says `"empty token returned by security command"` and would be visible if the call path went through there. The orchestration sentinel at `:140-142` exists for defensive completeness (a future runner that returns `("", nil)` from `ExtractKeychainToken`). Both messages are descriptive enough.

3. **`%w` wrap regression on `exec.LookPath`.** REFUTED. `runClaudeHostCommand` at `:232-237` now reads:
   ```go
   return "", fmt.Errorf(
       "claude CLI not found on PATH; install with: npm install -g @anthropic-ai/claude-code@2.1.89: %w",
       err,
   )
   ```
   `%w` wrap present. Test `TestRunClaudeHostCommandPreflightFailsWhenClaudeMissing` (`:419-436`) asserts BOTH the actionable string AND `errors.Is(err, exec.ErrNotFound)` at line 433. The `errors.Is` assertion is the load-bearing check — string-match alone would not catch a regression that drops `%w`. Verified clean.

4. **`TestSystemClaudeAccountAuthRunnerRunSetupTokenUsesCLAUDE_CONFIG_DIR` fake binary verification.** REFUTED:
   - Fake binary at `:71-81` is a real shell script (chmod 0o755 at `:82`) that writes `args:%s` and `CLAUDE_CONFIG_DIR=%s` to a log file. Not just a no-op exit.
   - Test reads log at `:461-464` and asserts BOTH `strings.Contains(content, "CLAUDE_CONFIG_DIR="+homePath)` (`:465-467`) AND `strings.Contains(content, "args:setup-token")` (`:468-470`). Both assertions are observable behavior checks, not "did it crash" smoke tests.
   - Cleanup: `t.TempDir()` (`:68`) auto-removes via testing.T's lifecycle. `t.Setenv("PATH", ...)` (`:86`) auto-restores on test exit.

5. **Test naming honesty.** REFUTED across all 14 test functions in the file. Walked each name against its body:
   - `TestEnsureClaudeAccountReadyRejectsNonTTY` — non-TTY cmd, asserts TTY-mention error AND `setupTokenHits == 0`. ✓
   - `TestEnsureClaudeAccountReadyRespectsSkipLogin` — SkipLogin=true, asserts `setupTokenHits == 0` AND pre-existing creds file preserved. ✓
   - `TestEnsureClaudeAccountReadyNonTTYDoesNotWipe` — non-TTY + pre-existing creds, asserts error AND file present after. ✓
   - `TestLoginClaudeAccountWipesAndRunsSetupTokenAndExtractsAndWrites` — 4 behaviors named, all asserted (wipe pre-existing, setup hit==1, extract hit==1, fresh token in file). ✓
   - `TestLoginClaudeAccountFailsWhenRunSetupTokenErrors` — setupTokenErr injected, asserts `errors.Is` AND `extractHits == 0`. ✓
   - `TestLoginClaudeAccountFailsWhenExtractTokenErrors` — extractTokenErr injected, asserts wrap. ✓
   - `TestLoginClaudeAccountFailsOnEmptyToken` — extractToken="" (zero), asserts error AND no file. ✓
   - `TestLoginClaudeAccountSkipsNonTTYGuard` — non-TTY cmd, asserts `setupTokenHits == 1` (proves no guard). ✓
   - `TestWipeClaudeCredentialsRemovesFile` — file written then wiped, asserts gone. ✓
   - `TestWipeClaudeCredentialsMissingFileIsNoError` — wipes empty dir, asserts nil err. ✓
   - `TestLogoutManagedAccountWipesClaudeCredentials` — integration with logout dispatch. ✓
   - `TestReadAccountIdentityReturnsLoggedOutWhenNoCreds` — adapter integration. ✓
   - `TestRunClaudeHostCommandPreflightFailsWhenClaudeMissing` — empty PATH, asserts npm install hint AND `errors.Is(err, exec.ErrNotFound)`. ✓
   - `TestSystemClaudeAccountAuthRunnerRunSetupTokenUsesCLAUDE_CONFIG_DIR` — fake binary, asserts env + args. ✓

   No name-vs-behavior drift. DROP_6's naming-honesty issue does not recur here.

6. **Coverage genuineness — 70.3% vs 68.4% baseline.** REFUTED. The +1.9 percentage points are achieved by:
   - `TestSystemClaudeAccountAuthRunnerRunSetupTokenUsesCLAUDE_CONFIG_DIR` exercises the production `systemClaudeAccountAuthRunner.RunSetupToken` AND `runClaudeHostCommand` happy path (stdin/stdout streaming branch).
   - `TestRunClaudeHostCommandPreflightFailsWhenClaudeMissing` exercises `runClaudeHostCommand`'s LookPath-failure branch with the new `%w` wrap.
   - `TestLoginClaudeAccountWipesAndRunsSetupTokenAndExtractsAndWrites` exercises the full success pipeline: `wipeClaudeCredentials` → `writeCLINotice` → `RunSetupToken` → `user.Current` → `ExtractKeychainToken` → `writeClaudeCredentials` → `ReadAccountIdentity` verification.
   - `TestLoginClaudeAccountFailsWhenRunSetupTokenErrors`, `TestLoginClaudeAccountFailsWhenExtractTokenErrors`, `TestLoginClaudeAccountFailsOnEmptyToken` exercise the three failure branches in the orchestration pipeline.

   These are real production paths, not trivial helpers or padding. No tautological assertions, no `if true` patterns.

7. **Codex/Claude TTY symmetry.** REFUTED. Post-widening, both guards read identically (`!commandHasTTY(cmd.InOrStdin()) || !commandHasTTY(cmd.OutOrStdout())`). The previous Unit 7.1 falsification finding (Vector 6) flagged this asymmetry; Unit 7.3 closed it. Symmetry achieved.

8. **`TestLoginClaudeAccountSkipsNonTTYGuard` correctness.** REFUTED. Test (`:316-338`) uses `newTestClaudeCmd()` (non-TTY) and injects an `extractTokenErr` so the function terminates after `ExtractKeychainToken`. Asserts:
   - `err != nil` (some error returned)
   - `!strings.Contains(err.Error(), "TTY")` — error is NOT the TTY guard
   - `stub.setupTokenHits == 1` — setup-token WAS reached (proves no upstream guard blocked)

   Reading `loginClaudeAccount` source (`claude_auth.go:158-196`): zero TTY checks anywhere. The widening of `ensureClaudeAccountReady`'s guard does NOT propagate to `loginClaudeAccount` (verified by source inspection). Correct.

9. **`TestRunOmitsClaudeCodeOAuthTokenWhenCredentialsMalformed` rigor.** REFUTED. `service_test.go:607-642`:
   - Writes `[]byte("NOT VALID JSON")` to `<profileHome>/.credentials.json` BEFORE calling `service.Run` (`:611-614`).
   - Calls real `service.Run` (`:635`) — not `readClaudeAuthToken` directly.
   - Asserts `err == nil` (graceful skip, NOT propagated error) (`:635-637`).
   - Asserts `executor.got.Env["CLAUDE_CODE_OAUTH_TOKEN"]` is absent or empty (`:639-641`).

   Goes through the full `Run` → `buildRequest` → `readClaudeAuthToken` → `json.Unmarshal` pipeline. Pins the AC4 graceful-skip behavior the Unit 7.2 falsification round flagged as un-pinned (V16 in that round).

10. **Production fix audit — line-by-line.** REFUTED. The 3 production changes (`git show c33da3c -- internal/cli/claude_auth.go`):
    - Line 111: TTY guard widened from single-stream to dual-stream. Identical to Codex template. No new bug surface.
    - Lines 140-142 (`ensureClaudeAccountReady`) and 182-184 (`loginClaudeAccount`): empty-token sentinel. Token IS empty by definition in this branch — no leak risk. Wipe at line 117 already happened upstream, so failing here leaves the user without credentials (pre-existing wiped, new one never written) — this matches the documented contract that wipe-first-fail-cleanly is the consistent failure mode for ALL post-wipe failures (setup-token error, extract error, write error, identity-verify error). Not a new behavior; consistent with the existing pipeline.
    - Lines 232-237: `%w` added to LookPath error wrap. Strict improvement — `errors.Is(err, exec.ErrNotFound)` now works for callers. No callers currently type-assert, so behaviorally neutral; test pins the new contract.

    SkipLogin behavior: `ensureClaudeAccountReady:108-110` is the first statement, returns nil before any wipe or extract. The empty-token sentinel at `:140-142` is downstream and cannot fire when SkipLogin=true. No interaction. Verified by `TestEnsureClaudeAccountReadyRespectsSkipLogin`.

11. **`mage testPkg` re-runs.** REFUTED. Re-ran all three packages:
    - `mage testPkg ./internal/cli` → 151/151 pass, 70.3% coverage. Matches worklog exactly.
    - `mage testPkg ./internal/services/claude` → 20/20 pass, 82.3% coverage. Matches.
    - `mage testPkg ./internal/adapters/providers/claude` → 21/21 pass, 78.4% coverage. Matches.

    No flakes, no race detector hits.

12. **No raw `go` invocations.** REFUTED. Worklog Round 1 (lines 112-114) cites only `mage testPkg ./internal/cli`, `mage testPkg ./internal/services/claude`, `mage testPkg ./internal/adapters/providers/claude`. No `go test`, `go build`, `go run`, `go vet`, `gofumpt` invocations.

13. **`account_test.go` regression.** REFUTED. `git log --oneline -5 internal/adapters/providers/claude/account_test.go` shows last commit was `a0eda74 feat: Claude credentials parsing and account-list display` (DROP 6.3 era) and `557bad3 feat(adapters): add Claude provider adapter package` (initial). Unit 7.3 commit `c33da3c` does NOT appear. `git show --stat c33da3c` confirms only `claude_auth.go`, `claude_auth_test.go`, `service_test.go`, and drop docs were touched — no `account_test.go`.

14. **Hylla feedback honesty.** REFUTED. Worklog Round 1 line 146: `"None — Hylla answered everything needed. ... all Go code reads went directly via Read tool per mid-drop evidence protocol."` Builder claims no Hylla queries; reads via `Read` directly. This matches the documented mid-drop staleness pattern (Unit 7.1 falsification round documented the same protocol at this file's lines 114-122). Honest about not using Hylla. The `## Hylla Feedback` section is present as required by CLAUDE.md.

### Additional Adversarial Probes

15. **Widening only differentiated by stdin in the test suite.** MINOR FINDING — not a counterexample. The new `|| !commandHasTTY(cmd.OutOrStdout())` branch in `ensureClaudeAccountReady` (`:111`) is structurally correct, but no test exercises the "stdin-TTY-but-stdout-pipe" case where ONLY the stdout half would fire. `newTestClaudeCmd()` sets both to `bytes.Buffer` (both non-TTY), so the OR short-circuits on the first failing clause. A future regression that reverts ONLY the stdout half (`|| !commandHasTTY(cmd.OutOrStdout())` → deleted) would pass all current tests. **Recommendation:** add a future test that wraps `cmd.SetIn(os.Stdin)` (potential real TTY) or otherwise sets stdin TTY-like while stdout is a pipe, then asserts the guard still fires. Not blocking for Unit 7.3 because the Codex twin is the structural ground truth and matches byte-for-byte; the widening is correct by construction.

16. **Empty-token sentinel is defensive-only with current production runner.** REFUTED (accepted as defensive). `systemClaudeAccountAuthRunner.ExtractKeychainToken` at `:75-78` already errors on empty token rather than returning `("", nil)`. So the orchestration-level `if token == ""` check at `:140-142` and `:182-184` is unreachable under the production runner. It IS reachable under a stub runner that returns `("", nil)` — which is exactly what `TestLoginClaudeAccountFailsOnEmptyToken` (`:289-308`) exercises. Belt-and-suspenders defensive coding; the sentinel is correct.

17. **Test count claim (12) vs file content (14).** MINOR FINDING — not a counterexample. Builder worklog (line 96-107) describes "12 test functions covering: TTY rejection, SkipLogin, non-TTY-preserves-creds, success path, setup-token error propagation, extract error propagation, empty-token failure, login no-TTY-guard, wipe-existing, wipe-missing, preflight-missing-claude, CLAUDE_CONFIG_DIR env verification". That enumerates 12 categories. The file contains 14 test functions; the worklog's prose count groups the 2 wipe tests and 2 integration tests slightly. Empirically 151 tests pass in mage — the count is internally consistent. Builder's "12" is a category count, not a function count. Not load-bearing.

18. **Wipe-then-fail leaves user without credentials.** REFUTED (intentional). On any post-wipe failure (`RunSetupToken` error, `user.Current` error, `ExtractKeychainToken` error, empty-token sentinel, `writeClaudeCredentials` error, or `ReadAccountIdentity` verify failure), `wipeClaudeCredentials` at `:117` has ALREADY removed the pre-existing creds file. The user is left logged-out. This is the consistent contract across Unit 7.1's full pipeline; the empty-token addition does NOT change the failure-mode shape. Tests `TestLoginClaudeAccountFailsOnEmptyToken` (`:289-308`) and `TestLoginClaudeAccountFailsWhenRunSetupTokenErrors` (`:234-259`) both assert "no `.credentials.json` written after failure" — confirming the intentional failure mode.

19. **`installFakeHostClaude` PATH ordering.** REFUTED. `:86` prepends the fake dir to PATH: `t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))`. Order matters because `exec.LookPath` returns the FIRST match. Prepending guarantees the fake `claude` wins over any real one on the host. Correct.

20. **`TestRunClaudeHostCommandPreflightFailsWhenClaudeMissing` PATH isolation.** REFUTED. `:421-422`: `emptyDir := t.TempDir(); t.Setenv("PATH", emptyDir)` — PATH is set to ONLY the empty dir for this test. No `claude` binary anywhere on PATH. `exec.LookPath("claude")` MUST fail. `t.Setenv` auto-restores at test end. Test cannot accidentally pick up a host-installed claude.

### Counterexamples

None CONFIRMED.

### Findings Summary

- **V1 / V15 (TTY-widening test-coverage gap)** — MINOR. The new `|| !commandHasTTY(cmd.OutOrStdout())` branch in `ensureClaudeAccountReady` is structurally correct (matches Codex twin byte-for-byte) but is not exclusively exercised by any test. All current TTY tests use `newTestClaudeCmd()` where BOTH stdin and stdout are `bytes.Buffer` (both non-TTY), so the OR short-circuits on the first clause and the new clause is effectively shadowed. Future-regression risk: a builder who reverts ONLY the stdout half would pass all tests. **Recommendation for a follow-up:** add a focused test (or split the existing widened tests) that exercises a stdin-TTY-stdout-pipe configuration to pin the stricter guard. Not blocking Unit 7.3 because the Codex template fidelity is the structural ground truth.
- **V2 (empty-token error message UX)** — MINOR. Error message is descriptive but not maximally actionable. Acceptable for v0.1.0; a future polish pass could add "re-run `valv account add claude <name>`" guidance.
- **V17 (test count claim 12 vs file content 14)** — TRIVIAL prose-counting wobble; not load-bearing because mage's 151-test count is the authoritative measure.

### Verdict

**pass.** Unit 7.3 holds against all 14 spawn-prompt vectors plus 6 additional adversarial probes. Three production hardening fixes (TTY widening, empty-token sentinel, `%w` wrap) are byte-for-byte correct and each is pinned by a behavior-asserting test (`errors.Is` for `%w`, fake-binary for env var, empty-extractToken stub for sentinel). Coverage genuinely restored from 68.4% to 70.3% via real production-path exercise. Codex/Claude TTY symmetry achieved. No `account_test.go` regression. No raw `go` invocations. The one minor finding — that the widened TTY guard is not exclusively differentiated by any current test — is a test-coverage gap, not a production defect: the Codex twin remains the structural ground truth and the widening matches it byte-for-byte.

## Hylla Feedback

None — Hylla was not queried for this Unit 7.3 falsification round. All evidence came from direct `Read` of files committed AFTER snapshot 8 (`c33da3c` and `c3642f3` are both post-ingest), so Hylla would have served stale results for the symbols-under-review. The mid-drop staleness pattern documented in Unit 7.1's falsification round (this file's lines 114-122) and Unit 7.2's (lines 242) continues to apply: any "is the code currently shaped this way?" question goes to `git diff` / direct file read, not Hylla, until drop-end reingest. Not a Hylla bug.

---

## Unit 7.1 — Round 2

**Date:** 2026-05-15
**Verdict:** pass

### Attack Attempts

Each numbered vector from the spawn prompt is enumerated below. CONFIRMED = counterexample produced. REFUTED = attack tried, evidence rules it out. EXHAUSTED = honest attempt, no counterexample constructable.

1. **Empty `.credentials.json` (0 bytes).** REFUTED. `claude_auth.go:115-118` reads `info, statErr := os.Stat(credPath)` then `if statErr == nil && info.Size() > 0`. A zero-byte file passes the stat-no-error check but FAILS `info.Size() > 0` (`0 > 0` is false), so the function falls through to the TTY guard and the auth path. The size check correctly distinguishes "file exists but empty" from "file exists with content". No counterexample.

2. **`.credentials.json` is a directory.** REFUTED with nuance. If `<homePath>/.credentials.json` is a directory, `os.Stat` returns no error and `info.Size()` for a directory is implementation-dependent on macOS (typically the directory entry size, which is non-zero — usually 64+ bytes on APFS). So `info.Size() > 0` is true and the function returns nil — treating the directory as "already authed". This is a real edge case BUT the next consumer (Unit 7.2's `readClaudeAuthToken`) opens the path with `os.ReadFile` which returns `EISDIR` for directories; the service then logs "claude auth token unreadable" and gracefully skips the env var injection (PLAN.md AC4). The container launches unauthed; the user sees a clean auth-failure inside the container. Not a panic, not a silent token-with-garbage. Acceptable for v0.1.0 — the failure surfaces, just one layer deeper. The `ensureCodexAccountReady` twin has the same shape (it trusts `LoginStatus` to make the binary determination). Not a counterexample; flag for a future hardening pass if dir-shaped corruption becomes plausible.

3. **`.credentials.json` exists but contains garbage.** REFUTED. Size > 0 check passes and `ensureClaudeAccountReady` returns nil — exactly as the design intends ("if creds appear to exist, trust them; let the downstream consumer decide whether the contents parse"). The user-visible failure mode: Unit 7.2's `readClaudeAuthToken` (`internal/services/claude/service.go`) hits a JSON parse error, `service_test.go::TestRunOmitsClaudeCodeOAuthTokenWhenCredentialsMalformed` confirms `Run` returns nil and the env var is absent. Container launches unauthed; `claude` inside the container then surfaces the unauth state to the user with its own messaging. Layered failure surfacing is acceptable for v0.1.0. No counterexample.

4. **Race between stat and subsequent launch.** REFUTED (acceptable). `ensureClaudeAccountReady` does `os.Stat` at `:115` and returns nil at `:117`. A concurrent `valv account logout claude work` between this stat and the subsequent container launch could `wipeClaudeCredentials` the file. `readClaudeAuthToken` in `services/claude/service.go` handles missing file gracefully (PLAN.md AC4: graceful skip → container launches unauthed). No panic, no token-stale state. Acceptable race window for v0.1.0 single-user macOS desktop UX. No counterexample.

5. **Stat error other than IsNotExist.** REFUTED. `claude_auth.go:119-121` reads `if statErr != nil && !os.IsNotExist(statErr) { return fmt.Errorf("check claude credentials for account %q: %w", account.Name, statErr) }`. The `%w` verb wraps the underlying error so `errors.Is(err, os.ErrPermission)` works for callers. EACCES, EIO, ENOTDIR (when an intermediate path component is a regular file), and any other non-NotExist stat failure all propagate wrapped. No swallow. No counterexample.

6. **SkipLogin ordering.** REFUTED. Reading `ensureClaudeAccountReady` body (`claude_auth.go:110-162`):
   - `:111-113`: `if options.SkipLogin { return nil }` — FIRST gate.
   - `:114-118`: stat + already-authed check — SECOND gate.
   - `:119-121`: stat-error propagation.
   - `:122-127`: TTY guard.
   - `:128+`: setup-token flow.
   Ordering is correct: SkipLogin short-circuits before any filesystem touch (no unnecessary stat). An already-authed account with `SkipLogin=true` correctly returns nil at `:112` without statting. A not-authed account with `SkipLogin=true` also correctly returns nil — SkipLogin takes priority over the stat-then-auth path. Confirmed by `TestEnsureClaudeAccountReadyRespectsSkipLogin` (`claude_auth_test.go:117-142`) which writes creds + sets `SkipLogin=true` and asserts nil + zero runner hits + creds preserved. No counterexample.

7. **`TestEnsureClaudeAccountReadyMissingCredsAndNonTTYFailsTTY` assertions.** REFUTED. The test (`claude_auth_test.go:182-203`) does:
   - `dir := t.TempDir()` — empty profile home (line 185). No creds file written.
   - `err := ensureClaudeAccountReady(cmd, account, accountAuthOptions{})` (line 193) — non-TTY cmd.
   - `if !strings.Contains(err.Error(), "TTY")` (line 197) — asserts the SPECIFIC TTY error string, not just "any error".
   - `if stub.setupTokenHits != 0` (line 200) — asserts runner was NOT invoked.
   Empty home → stat returns IsNotExist → already-authed check fails (passes through) → TTY guard fires. Test correctly exercises the "auth needed but cannot prompt" path. No counterexample.

8. **`TestEnsureClaudeAccountReadySkipsWhenAlreadyAuthed` assertions.** REFUTED. The test (`claude_auth_test.go:208-228`) does:
   - `os.WriteFile(credPath, []byte(`{"claudeAiAccessToken":"tok"}`), 0o600)` (line 213) — writes a NON-EMPTY creds file BEFORE the call.
   - `ensureClaudeAccountReady(cmd, account, accountAuthOptions{})` (line 222) — non-SkipLogin.
   - `if err != nil { t.Fatalf("...want nil", err) }` (lines 222-224) — asserts return is nil.
   - `if stub.setupTokenHits != 0 || stub.extractHits != 0` (line 225) — asserts BOTH runner methods uninvoked.
   This is exactly the Round 2 fix's primary coverage test. No counterexample.

9. **`loginClaudeAccount` regression.** REFUTED. `git diff HEAD~1 HEAD -- internal/cli/claude_auth.go` shows ONLY `ensureClaudeAccountReady` (the doc comment + the function body around lines 100-127) changed. `loginClaudeAccount` body (lines 166-204 in current state) is byte-for-byte identical to its Round 1 state: `wipeClaudeCredentials` at `:167-169`, notice at `:170-177`, `RunSetupToken` at `:178-181`, `user.Current` at `:182-185`, `ExtractKeychainToken` at `:186-189`, empty-token sentinel at `:190-192`, `writeClaudeCredentials` at `:193-195`, `ReadAccountIdentity` verify at `:196-202`. The wipe + force-fresh flow for explicit `valv account login` is preserved. No counterexample.

10. **`account add` semantic shift.** REFUTED, confirmed intentional. `runManageAccountAdd` in `manage.go` calls `ensureManagedAccountReady` (Hylla `runManageAccountAdd` node confirms the `code.depends_on` edge to `ensureManagedAccountReady`). After Round 2, `valv account add claude work` against an existing authed `work` profile now returns nil at the already-authed check — no re-auth. Compare with `ensureCodexAccountReady` (`account_auth.go:70-107`): it calls `LoginStatus`, returns nil if `loggedIn==true` at `:82-84`. Same semantic. The Round 2 fix brings Claude to parity with Codex. The behavior is also defensible standalone: `account add` is a create-or-ensure idempotent operation; re-running it on an existing authed account should not invalidate the auth. Confirmed as intended, not a regression.

11. **`mage testPkg ./internal/cli` re-run.** EXHAUSTED — cannot re-run from QA subagent context (per safety / `mage`-only orchestration policy). Relying on the builder's worklog claim (worklog line 176: "GREEN (154/154 pass, 70.4% coverage)") and on the test additions visible in `claude_auth_test.go`. The four new tests are concretely visible in source:
   - `TestEnsureClaudeAccountReadyAlreadyAuthedReturnsNilEvenNonTTY` (`:149-177`),
   - `TestEnsureClaudeAccountReadyMissingCredsAndNonTTYFailsTTY` (`:182-203`),
   - `TestEnsureClaudeAccountReadySkipsWhenAlreadyAuthed` (`:208-228`),
   - `TestEnsureClaudeAccountReadyAuthsWhenCredentialsMissing` (`:236-255`).
   Plus one removal (`TestEnsureClaudeAccountReadyNonTTYDoesNotWipe` — worklog line 167-168). Net +3. Count is consistent with 151→154. No counterexample.

12. **No raw `go` invocations.** REFUTED. Worklog lines 175-176 explicitly cite `mage testPkg ./internal/cli` for both the RED and GREEN runs. No `go test`, `go vet`, `go build` strings appear anywhere in the Round 2 worklog section. Compliant with AGENTS.md § 13 / project CLAUDE.md mage-only rule. No counterexample.

13. **Coverage genuine.** REFUTED. The 4 new tests are behavior-asserting, not trivial:
   - `TestEnsureClaudeAccountReadyAlreadyAuthedReturnsNilEvenNonTTY`: asserts nil return + `setupTokenHits == 0` + `extractHits == 0` + creds file preserved. Three orthogonal assertions over the new code path.
   - `TestEnsureClaudeAccountReadyMissingCredsAndNonTTYFailsTTY`: asserts TTY error mention + `setupTokenHits == 0`. Pins the fall-through-to-TTY-guard path.
   - `TestEnsureClaudeAccountReadySkipsWhenAlreadyAuthed`: dual-counter assertion `setup=0 AND extract=0`. Pins the early-return invariant.
   - `TestEnsureClaudeAccountReadyAuthsWhenCredentialsMissing`: asserts "creds-missing reaches auth gate" via TTY-error proxy. Distinguishes "early-return swallowed the case" from "case reached auth and failed correctly".
   Each test pins a distinct branch of the new 4-line stat+size check (`:114-121`). Coverage delta from 70.3% → 70.4% is small in proportion but the new statements ARE the new branch — small delta is correct, not suspicious. No counterexample.

14. **`TestEnsureClaudeAccountReadyRejectsNonTTY` (Round 1 inherited).** REFUTED. The test (`claude_auth_test.go:94-112`) does:
   - `account := domain.Profile{Name: "personal", HomePath: t.TempDir()}` (line 101) — `t.TempDir()` returns a freshly-created EMPTY directory. No `os.WriteFile` for `.credentials.json` precedes the call.
   - Empty profile home → `os.Stat` returns IsNotExist → already-authed check passes through (falls through to TTY guard) → TTY guard fires.
   - Test asserts TTY-error + `setupTokenHits == 0`.
   The builder's claim that this test "required no modification" is verified: empty home + non-TTY produces a TTY error regardless of Round 1 vs Round 2 behavior. The already-authed check has zero observable effect on this test because the precondition (creds present) is absent. No counterexample.

### Additional Adversarial Probes

Three extra attacks beyond the spawn-prompt's 14 vectors:

A1. **`strings.TrimSpace(account.HomePath)` consistency.** REFUTED. Round 2 uses `filepath.Join(strings.TrimSpace(account.HomePath), ".credentials.json")` at `:114`. This matches `wipeClaudeCredentials` (`:224`) and `writeClaudeCredentials` (`:214`) — all three resolve the same credPath. A trailing-whitespace HomePath would resolve consistently across stat/write/wipe. No mismatch counterexample.

A2. **Race between stat and read by Unit 7.2's `readClaudeAuthToken`.** REFUTED. Even if the stat passes at `ensureClaudeAccountReady` time and the file is deleted before Unit 7.2's read, `readClaudeAuthToken` handles missing file as "no token → omit env var → log debug" (PLAN.md AC4). No panic. Same race-handling shape as Attack 4 — no counterexample.

A3. **`info.Mode()` not checked.** REFUTED but flagged. The stat check only inspects `Size()`. A file with mode `0o000` (no read permission) would pass the size > 0 check and `ensureClaudeAccountReady` would return nil — but Unit 7.2's `readClaudeAuthToken` would then fail with EACCES on `os.ReadFile`, gracefully degrading to unauthed container. Same surfacing pattern as Attack 2 (directory case). Not a counterexample for THIS unit; layered defense at the consumer.

### Counterexamples

None CONFIRMED.

### Findings Summary

- **Attack 2 (directory case)** — minor edge case. Defense-in-depth at Unit 7.2's `readClaudeAuthToken` covers it (returns `EISDIR` → graceful skip → container launches unauthed). Not blocking.
- **Attack A3 (mode 0o000 case)** — same shape. Same layered defense covers it. Not blocking.
- **Attack 10 (`account add` semantic shift)** — INTENTIONAL semantic change, brings Claude to parity with Codex. Confirmed as intended via Codex-twin comparison. Worth noting in the release notes if this drop ships standalone.

### Verdict

**pass.** Unit 7.1 Round 2 holds against all 14 spawn-prompt vectors plus 3 additional adversarial probes. The fix is byte-for-byte structurally correct:
- Unconditional wipe removed from `ensureClaudeAccountReady` (`git diff` confirms the deletion).
- Already-authed check correctly placed AFTER SkipLogin (no unnecessary stat) and BEFORE TTY guard (no spurious TTY error for already-authed accounts).
- `info.Size() > 0` correctly distinguishes empty-file corruption from real authed state.
- Non-IsNotExist stat errors wrap with `%w` and propagate (no swallow).
- `loginClaudeAccount` unchanged — wipe + force-fresh preserved for explicit `valv account login` (`git diff` confirms).
- `account add` semantic now mirrors Codex (idempotent ensure, not force re-auth).

Four new behavior-asserting tests pin every branch of the new check. Test count delta (+3 net = 151→154) is consistent. Coverage delta (70.3% → 70.4%) is small in proportion but proportional to the small new-branch surface — not suspicious. Mage-only invocation honored.

The original `valv account switch claude <existing-name>` forced re-auth bug is correctly fixed at the right layer with the right semantics.

## Hylla Feedback

- **Query:** `hylla_search_keyword` for `runManageAccountSwitch` (fields=summary/docstring/content).
  - **Missed because:** First attempt with default fields returned zero results — Hylla's `hide_tests` + `public_only` defaults plus content-search filter on a private function in a package whose summaries don't mention "runManageAccountSwitch" by name yielded nothing. Adding `visibility_mode=include_private` and `fields=["content"]` returned the node immediately.
  - **Worked via:** `hylla_search_keyword` with `visibility_mode=include_private` + `fields=["content"]`, then `hylla_node_full` for the dependency edges.
  - **Suggestion:** Default `visibility_mode` to `include_private` for `tail_symbol`-style queries (the symbol name alone signals deliberate intent). Or surface a hint in the response when zero results came back due to a visibility filter.

---

## Unit 7.1 — Round 3

**Date:** 2026-05-15
**Verdict:** pass (with two non-blocking observations on test fixture realism and Codex template fidelity)

### Attack Attempts

Each numbered vector from the spawn prompt is enumerated below. CONFIRMED = counterexample produced. REFUTED = attack tried, evidence rules it out. EXHAUSTED = honest attempt, no counterexample constructable. OBSERVATION = real finding that does not invalidate the fix.

1. **Token format mismatch hypothesis — verbatim preservation.** REFUTED. `writeClaudeCredentials` at `internal/cli/claude_auth.go:207-213` is exactly:
   ```go
   func writeClaudeCredentials(homePath, credentialsBlob string) error {
       credPath := filepath.Join(strings.TrimSpace(homePath), ".credentials.json")
       if err := os.WriteFile(credPath, []byte(credentialsBlob), 0o600); err != nil {
           return fmt.Errorf("write %q: %w", credPath, err)
       }
       return nil
   }
   ```
   No `json.Marshal`, no struct wrap, no transformation. Bytes from `ExtractKeychainToken` go straight to disk. Whether the keychain shape matches `.credentials.json` shape end-to-end is the dev's live smoke test — verbatim preservation is mechanically guaranteed by the implementation.

2. **`auth login` vs `setup-token` UX.** REFUTED. The `RunAuthLogin` argv at `claude_auth.go:50` is `"auth", "login"`. Browser-redirect + paste-prompt UX semantics are owned by the upstream `claude` CLI; both `setup-token` and `auth login` produce the same paste-callback flow. No `valv`-side UX regression.

3. **`auth login` scope sufficiency — literal argv.** REFUTED. Verified at `claude_auth.go:49-52`:
   ```go
   func (systemClaudeAccountAuthRunner) RunAuthLogin(ctx context.Context, homePath string, ...) error {
       _, err := runClaudeHostCommand(ctx, homePath, stdin, stdout, stderr, "auth", "login")
       return err
   }
   ```
   No `"setup-token"` anywhere in production argv. The `git grep -n "setup-token"` audit returns only test-file comments and error-message string literals (all in test scaffolding); no production argv reference survived the pivot.

4. **Empty keychain output sentinel preserved.** REFUTED. Three sentinel layers exist:
   - `ExtractKeychainToken` (`claude_auth.go:78-81`): `if token == ""` → returns explicit error `"extract claude keychain token: empty token returned by security command"`.
   - `ensureClaudeAccountReady` (`claude_auth.go:144-146`): defense-in-depth re-check after `ExtractKeychainToken` returns — `if token == ""` → error.
   - `loginClaudeAccount` (`claude_auth.go:186-188`): same defense-in-depth check.

   Test `TestLoginClaudeAccountFailsOnEmptyToken` (`claude_auth_test.go:372-391`) verifies this end-to-end with `stub := &stubClaudeAccountAuthRunner{}` (extractToken zero-value): asserts error returned AND `.credentials.json` NOT written. The `writeClaudeCredentials` call is never reached on empty.

5. **File mode 0o600.** REFUTED. `claude_auth.go:209`: `os.WriteFile(credPath, []byte(credentialsBlob), 0o600)`. Owner read+write only, no group, no world. Matches `wipeClaudeCredentials` removal mode of the same file. No regression.

6. **No JSON marshal/unmarshal on write path.** REFUTED. `git grep encoding/json -- internal/cli/claude_auth.go internal/services/claude/service.go` returns ZERO matches. Both files have the import removed. `writeClaudeCredentials` is a single `os.WriteFile` with `[]byte(credentialsBlob)` — no encoder anywhere on the write path.

7. **0-byte file write edge case.** REFUTED via sentinel layering. The only path to `writeClaudeCredentials` from a successful `ExtractKeychainToken` return is gated by the `token == ""` check at `claude_auth.go:144` (and `:186` for `loginClaudeAccount`). If keychain ever returned empty string with `nil` error (it cannot — `ExtractKeychainToken` itself errors first at `:78-81`), the redundant ensure/login check catches it. A 0-byte `.credentials.json` cannot be written by the production code path. Tests `TestLoginClaudeAccountFailsOnEmptyToken` and (existing) verify both gates.

8. **`encoding/json` import audit + dead-symbol audit.** REFUTED.
   - `internal/cli/claude_auth.go` imports: `bytes / context / fmt / io / os / os/exec / os/user / path/filepath / strings / laslig / cobra / claudeprovider / config / domain`. No `encoding/json`. Audit clean.
   - `internal/services/claude/service.go` imports: `context / errors / fmt / io / os / path/filepath / strings / time / unicode / charmbracelet/log / docker / clauderuntime / domain / pathutil / projectdetect`. No `encoding/json`. Audit clean.
   - `git grep claudeCredentials\\b`: zero matches across `internal/`. Struct deleted, no stragglers.
   - `git grep readClaudeAuthToken`: zero matches across `internal/`. Function deleted, no stragglers.
   - Compile cleanliness is implicit (`mage testPkg` GREEN, see vectors 13-14) but the grep audit is the direct evidence.

9. **Test fixtures realism — multi-key vs single-key shape.** **OBSERVATION (non-blocking, not a CONFIRMED counterexample).**

   The Round 3 design pivot rationale (worklog line 238) is: "the macOS keychain stores the full session JSON blob (e.g. `{"accessToken":"...","refreshToken":"...","expiresAt":"..."}`) as a single string. Container claude on Linux reads `.credentials.json` natively — it expects exactly this format."

   But the test fixtures in `internal/cli/claude_auth_test.go` still use the OLD single-key wrapper shape that the pivot explicitly rejects:
   - Line 122: `os.WriteFile(credPath, []byte(\`{"claudeAiAccessToken":"existing"}\`), 0o600)`
   - Line 154: same shape
   - Line 213: `{"claudeAiAccessToken":"tok"}`
   - Line 275: `{"claudeAiAccessToken":"old"}`
   - Line 432: same shape
   - Line 462: same shape
   - Plus `TestLoginClaudeAccountWipesAndRunsSetupTokenAndExtractsAndWrites` (`:269-312`) uses `extractToken: "fresh-token"` — a bare unbalanced string, neither valid keychain JSON nor the old wrapper. The test assertion `strings.Contains(data, "fresh-token")` passes only because the production code now writes verbatim.

   **Why this is not a CONFIRMED counterexample**: production code is fully shape-agnostic. `ReadAccountIdentity` checks only file existence (`internal/adapters/providers/claude/account.go:40-56`); `os.Stat(...).Size() > 0` checks only non-empty bytes. The fixtures pass these gates regardless of internal shape. So the tests still validate the production invariants ("already-authed short-circuit fires when file exists & non-empty", "write happens verbatim", etc.).

   **Why it is a real OBSERVATION**: the worklog Round 3 design notes (line 242) acknowledge this directly — "the stub's `extractToken` field (type `string`) now notionally returns a JSON blob. The test ... uses `extractToken: "fresh-token"` — a bare string, not a real JSON blob ... This is acceptable: the stub isolates the write path; live keychain integration is a smoke-test concern." Builder is aware. Recommendation for a future polish pass: switch fixtures to a realistic shape (e.g. `{"accessToken":"abc","refreshToken":"def","expiresAt":"2026-12-31T00:00:00Z"}`) to document the contract the production code targets. Not a build blocker.

10. **Bind-mount pipeline unchanged.** REFUTED. Verified end-to-end:
    - `internal/services/claude/service.go::buildRequest` (`:244-276`) constructs the request with `Mounts: append([]docker.MountSpec{docker.NewMountSpec(project.Root, project.Root, false)}, prepared.Mounts...)` at line 266.
    - `prepared.Mounts` comes from `clauderuntime.PrepareRuntime` (`service.go:127-133`).
    - `clauderuntime.PrepareRuntime` (`internal/adapters/providers/claude/runtime.go:59-116`) adds `dockeradapter.NewMountSpec(runtimeClaudeHome, ContainerClaudeDir, false)` to its returned `Mounts` at line 116, where `ContainerClaudeDir = "/home/valv/.claude"` (`:22`).
    - `TestRunSucceedsWithBoundProject` (`service_test.go:241-249`) asserts a mount with target `ContainerClaudeDir` exists in the executor's received request. GREEN per re-run.

    Round 2 of Unit 7.2 deleted ONLY the env-injection block in `buildRequest`; mount construction was not touched. Verified by `git diff HEAD~1 HEAD --stat` showing only `service.go: -40` lines and `service_test.go: -118` lines (no `Mount`-touching changes).

11. **3 deleted tests — coverage gap?** REFUTED. The three tests deleted in Unit 7.2 Round 2:
    - `TestRunSetsClaudeCodeOAuthTokenWhenCredentialsPresent` — asserted env-var injection. No longer applicable: the production code no longer injects the env var. Deletion correct.
    - `TestRunOmitsClaudeCodeOAuthTokenWhenCredentialsMissing` — asserted absent creds → env var absent. No longer applicable: env var is never present in any case. Deletion correct.
    - `TestRunOmitsClaudeCodeOAuthTokenWhenCredentialsMalformed` — asserted bad JSON → graceful skip → env var absent. No longer applicable: no parsing happens. Deletion correct.

    The behaviors these tests gated have been deleted along with them. The retained tests (`TestRunSucceedsWithBoundProject` at minimum) still verify the mount-based authentication path: profile home → `/home/valv/.claude` mount carries `.credentials.json` to the container. No coverage gap on the new design.

12. **`CLAUDE_CODE_OAUTH_TOKEN` env-var leak audit.** REFUTED. `git grep -n "CLAUDE_CODE_OAUTH_TOKEN" -- internal/ cmd/ magefile.go` returns ZERO matches. Variable name is gone from production AND test code. Not even a comment-form reference remains. Audit clean.

13. **Mage discipline.** REFUTED. Worklog Round 3 (line 234) reports only `mage testPkg ./internal/cli`. Round 2 of Unit 7.2 reports `mage testPkg ./internal/services/claude` and `mage testPkg ./internal/adapters/providers/claude`. No raw `go test`, `go build`, `go vet`, `go run` references in the worklog. AGENTS.md § 13 / WORKFLOW.md `mage`-first rule observed.

14. **`mage testPkg` re-runs (independent verification).** REFUTED.
    - `mage testPkg ./internal/cli` re-run: 154 tests passed, coverage 70.4%. Matches worklog Round 3 line 234 exactly. (Worklog claimed "70.3%" in Unit 7.3, "70.4%" in Round 2/Round 3 — re-run confirms 70.4%.)
    - `mage testPkg ./internal/services/claude` re-run: 17 tests passed, coverage 81.0%. Matches Round 2 line 287 exactly.
    - `mage testPkg ./internal/adapters/providers/claude` re-run: 21 tests passed, coverage 78.4%. Matches Round 2 line 288 exactly.

    Coverage threshold report from mage output: `Minimum package coverage: 60.0%` — note this is mage's hardcoded internal floor, not the AGENTS.md § 11 70%-per-package target the worklog cites. All three packages exceed both floors. Non-blocking nit: mage's threshold (60%) is more permissive than AGENTS.md (70%); a future drop could tighten the mage gate to match AGENTS.md.

15. **Interface rename propagation — `RunSetupToken` → `RunAuthLogin`.** REFUTED. Audit:
    - Interface declaration: `claude_auth.go:30-33`. Single method `RunAuthLogin`, no stale `RunSetupToken`.
    - Production implementation: `systemClaudeAccountAuthRunner.RunAuthLogin` (`claude_auth.go:49-52`). Interface satisfied.
    - Production callers: `ensureClaudeAccountReady` at `claude_auth.go:133`; `loginClaudeAccount` at `claude_auth.go:175`. Both call `runner.RunAuthLogin(...)`. No stale `runner.RunSetupToken(...)` remains.
    - Test stub: `stubClaudeAccountAuthRunner.RunAuthLogin` (`claude_auth_test.go:35-38`). Interface satisfied (compile would fail otherwise; `mage testPkg ./internal/cli` GREEN).
    - Test field names still use `setupTokenHits` (`:28, 30`) and the test assertions/comments still reference "RunSetupToken" in error messages (16 occurrences per `git grep`). **OBSERVATION (cosmetic)**: the production rename was thorough; the test-side field/comment naming is stale-but-functionally-correct. The struct field `setupTokenHits` increments inside `RunAuthLogin` and is asserted as `setupTokenHits` — the name is now misleading, not wrong. Not a build blocker; recommend renaming `setupTokenHits` → `authLoginHits` (and updating the 16 stale comment/string references) in the next test polish pass.

### Counterexamples

None CONFIRMED.

### Verdict

**PASS.** Round 3 fix correctly:
- Switches argv from `setup-token` to `auth login`.
- Removes the `claudeCredentials` JSON wrapper struct and `encoding/json` import.
- Writes the keychain blob verbatim with `0o600` mode.
- Propagates the `RunSetupToken` → `RunAuthLogin` rename consistently across interface, implementation, callers, stub, and CLAUDE_CONFIG_DIR-verifying test.
- Preserves all defense-in-depth sentinel checks (empty-token, file-mode, error-wrap, TTY guard, already-authed short-circuit).

Two non-blocking observations for a future polish pass:
- Vector 9: test fixtures still use the old single-key wrapper shape (`{"claudeAiAccessToken":"..."}`); production code is shape-agnostic so tests pass, but the realism gap weakens the test-as-documentation value of the fixtures.
- Vector 15: test-side field/comment names (`setupTokenHits`, 16 `RunSetupToken`/`setup-token` string/comment references in `claude_auth_test.go`) are stale-but-correct after the interface rename.

End-to-end correctness of the `keychain-shape == .credentials.json-shape` hypothesis cannot be verified from the orchestrator/QA seat; it requires a live dev smoke test against a real Claude account and a containerized Linux `claude` binary. That is explicitly the dev's smoke-test job per worklog Round 3 design notes. Within the QA-falsifiable surface, no counterexample lands.

### Unknowns

- Live keychain-blob shape on the dev's macOS keychain (smoke test only).
- Whether Linux container `claude` actually accepts the verbatim macOS keychain JSON shape (smoke test only).
- Both are explicit per worklog Round 3 and routed to the dev.

## Hylla Feedback

None — Hylla answered everything needed via direct `git grep` and `Read`. The reviewed files are uncommitted-to-baseline (Round 3 commit `9e7bc48` post-dates last Hylla ingest); per CLAUDE.md § "Code Understanding Rules" item 2, `git diff` / `Read` / `Grep` is the correct evidence path for changed-since-ingest files.

---

## Unit 7.2 — Round 2

**Date:** 2026-05-15
**Verdict:** pass (no findings)

### Attack Attempts

Unit 7.2 Round 2 deletes the `readClaudeAuthToken` helper, the `encoding/json` import, the env-injection block in `buildRequest`, and three associated test functions. Attack vectors below are scoped to that delta. Vectors that overlap with Unit 7.1 Round 3 (which is the broader pivot) are cross-referenced rather than duplicated.

1. **`readClaudeAuthToken` fully deleted.** REFUTED. `git grep readClaudeAuthToken -- internal/ cmd/`: zero matches. No production reference, no test reference, no string-literal mention. Deletion complete.

2. **`CLAUDE_CODE_OAUTH_TOKEN` fully deleted.** REFUTED. `git grep CLAUDE_CODE_OAUTH_TOKEN -- internal/ cmd/ magefile.go`: zero matches. No env-var assignment, no env-var read, no comment reference. Audit clean.

3. **`encoding/json` import removed from `service.go`.** REFUTED. `git grep "encoding/json" -- internal/services/claude/service.go`: zero matches. Import block of `service.go` (lines 1-21) shows only: `context / errors / fmt / io / os / path/filepath / strings / time / unicode / charmbracelet/log / docker / clauderuntime / domain / pathutil / projectdetect`. No stale import.

4. **`os` and `path/filepath` imports retained where still used.** REFUTED. `service.go` line 8 imports `"os"` (used at `:96` for `os.TempDir()`), line 9 imports `"path/filepath"` (used at `:279` for `filepath.Rel` and `:286` for `filepath.Separator`). Both still pull their weight. No dead import.

5. **Three deleted test functions — coverage shadow.** REFUTED. The three deletions:
   - `TestRunSetsClaudeCodeOAuthTokenWhenCredentialsPresent` — asserted env-var injection on valid creds.
   - `TestRunOmitsClaudeCodeOAuthTokenWhenCredentialsMissing` — asserted graceful skip on missing creds.
   - `TestRunOmitsClaudeCodeOAuthTokenWhenCredentialsMalformed` — asserted graceful skip on bad JSON.

   All three tested behavior that no longer exists in production code (env-var injection is gone in Round 2; bad-JSON handling is gone because no JSON parsing happens). Deleting the tests along with the deleted production code is correct. No shadow coverage remains — the retained `TestRunSucceedsWithBoundProject` covers the mount-based authentication path which is the new (and now sole) auth-delivery mechanism.

6. **`service_test.go` import cleanup.** REFUTED. Worklog line 280 claims `"os"` and `"path/filepath"` imports were removed. Verified by reading the import block (`service_test.go:3-18`): `context / errors / fmt / io / strings / testing / time / charmbracelet/log / docker / clauderuntime / domain / projectdetect`. No `os`, no `path/filepath`. Both correctly removed (no longer used after the three test deletions). Audit clean.

7. **Bind-mount pipeline unchanged.** REFUTED. Already covered in Unit 7.1 Round 3 vector 10 above. `clauderuntime.PrepareRuntime` adds the `<managed-home>` → `/home/valv/.claude` mount at `runtime.go:116`; `service.go::buildRequest` propagates it at `:266`; `TestRunSucceedsWithBoundProject` (`service_test.go:241-249`) verifies the mount in the executor's received request. None of this code was touched in Round 2 of Unit 7.2 — `git diff HEAD~1 HEAD service.go` shows only the env-injection block deleted, no `Mount`-line changes. Verified.

8. **`mage testPkg ./internal/services/claude` re-run.** REFUTED. Re-ran independently: 17 tests passed, 81.0% coverage. Matches worklog line 287 exactly. (Pre-deletion was 20 tests at 82.3% — coverage drop of 1.3 percentage points across the three deleted tests is consistent and stays well above both the mage floor of 60% and the AGENTS.md § 11 floor of 70%.)

9. **`mage testPkg ./internal/adapters/providers/claude` re-run.** REFUTED. Re-ran independently: 21 tests passed, 78.4% coverage. Matches worklog line 288 exactly. No regression from the Round 2 deletes — the adapters package was not modified.

10. **No collateral edits to other files.** REFUTED. `git diff HEAD~1 HEAD --stat` output:
    ```
    .../DROP_7_HOST_AUTH_TOKEN_FIX/BUILDER_WORKLOG.md  | 113 ++++++++++++++++++++
    internal/cli/claude_auth.go                        |  78 +++++++-------
    internal/cli/claude_auth_test.go                   |  26 ++---
    internal/services/claude/service.go                |  40 -------
    internal/services/claude/service_test.go           | 118 ---------------------
    ```
    Five files. Three production-relevant: `claude_auth.go` (Round 3), `service.go` (Round 2), `service_test.go` (Round 2). One test: `claude_auth_test.go` (Round 3). One doc: `BUILDER_WORKLOG.md`. No collateral edits to `magefile.go`, `cmd/valv/`, `internal/adapters/`, `internal/domain/`, or other packages.

### Counterexamples

None CONFIRMED.

### Verdict

**PASS.** Round 2 fix correctly:
- Deletes `readClaudeAuthToken` helper and `CLAUDE_CODE_OAUTH_TOKEN` env-injection block (the env-var workaround for the wrong file format is no longer needed; container claude now reads `.credentials.json` natively via the existing DROP_5 bind-mount).
- Removes the `encoding/json` import from `service.go`.
- Removes the three associated tests whose subject behavior was deleted.
- Retains all imports (`os`, `path/filepath`) where they are still used elsewhere in the file.
- Preserves the DROP_5 bind-mount pipeline intact (verified by reading `service.go::buildRequest` and `runtime.go::PrepareRuntime`).
- Maintains coverage at 81.0% (above both the mage 60% gate and AGENTS.md 70% gate).

The Round 2 change is a clean negative-delta — it removes code that became dead after Round 3 of Unit 7.1 fixed the file format. The combined Round 3 + Round 2 change correctly implements the design pivot: macOS keychain blob is now written verbatim to `<managed-home>/.credentials.json` at `0o600`, container claude reads it natively via the bind-mount, and the env-var injection that was a workaround for the broken wrapper format is gone.

### Unknowns

None — all 10 attack vectors REFUTED with file:line evidence and re-run mage output.

## Hylla Feedback

None — Hylla answered everything needed via direct `git grep` and `Read`. The reviewed files (`service.go`, `service_test.go`) are uncommitted-to-baseline (Round 2 commit `9e7bc48` post-dates last Hylla ingest); per CLAUDE.md § "Code Understanding Rules" item 2, `git diff` / `Read` / `Grep` is the correct evidence path for changed-since-ingest files.

---

## Unit 7.7 — Round 1

**Date:** 2026-05-15
**Verdict:** pass

### Attack vectors probed

- **A1: npm `/latest` URL semantics.** REFUTED. `https://registry.npmjs.org/<pkg>/latest` is npm's documented endpoint that returns the manifest for whatever version is dist-tagged `latest`. Context7 docs (`/websites/npmjs`) confirm: `dist-tags.latest` is the standard publisher channel; pre-release versions ship under separate tags (`beta`, `next`) by convention, so a `next`/`beta` release WITHOUT a `latest` bump is the documented npm behavior — not a resolver bug. The endpoint does not 404 when `latest` exists (which it must for any published Anthropic release). Code at `service.go:28` constant + `:200` request matches the endpoint name correctly.

- **A2: Pre-release versions.** REFUTED (with NOTE). `versionPattern` at `service.go:41` is `\d+\.\d+\.\d+(?:[-+][0-9A-Za-z.-]+)?` — explicitly admits a `-prerelease`/`+build` suffix. If Anthropic shipped `2.2.0-beta.1` as `latest` (unusual but allowed), `MatchString` returns true, `FindString` returns `"2.2.0-beta.1"` verbatim. The Claude Dockerfile at `service.go:679` runs `npm install --global "@anthropic-ai/claude-code@${CLAUDE_VERSION}"` which accepts pre-release version strings. So end-to-end behavior is correct. Symmetric with Codex's `normalizeCodexVersion` (`service.go:173`). Not a counterexample to Unit 7.7's claim.

- **A3: `versionPattern` not anchored.** REFUTED (with CONCERN below). The regex is NOT anchored with `^...$`. `MatchString` returns true on any substring match; `FindString` extracts the first match. So `"prefix-1.2.3-suffix"` would `MatchString=true` AND `FindString="1.2.3"` (the suffix `-suffix` matches the optional prerelease group). The validation+extraction divergence means the resolver could silently strip garbage from malformed npm responses. In practice npm-published versions are normalized at publish-time, so this is theoretical. **Symmetric with the Codex resolver** (same `versionPattern` used by `normalizeCodexVersion`), so anchoring is a global change not specific to Unit 7.7. Recorded as CONCERN.

- **A4: User-Agent header value `"valv"`.** REFUTED. Tests round-trip the request via `httptest.NewServer`; npm registry historically accepts any UA. Codex resolver uses the same `"valv"` string at `service.go:148`. Symmetric. Not a counterexample.

- **A5: 10-second timeout.** REFUTED at this unit's scope. Unit 7.7 ONLY adds the resolver. `ensureClaudeImageCurrent` at `claude.go:181-197` STILL calls `service.Build(...)` not `service.EnsureLatest(...)` — verified by direct read of the committed file. The launch path does NOT invoke the resolver in Unit 7.7. Wiring is Unit 7.8 (`state: todo` per PLAN.md). 10s timeout latency is correctly out of scope for THIS unit's claim. The timeout reuses `defaultVersionRequestTTL = 10 * time.Second` at `service.go:29`.

- **A6: `defaultVersionRequestTTL` reuse.** REFUTED. Both Codex and Claude resolvers share the constant. 10s is conservative for an internet-reachable JSON GET. Matches the Codex precedent. Not a counterexample.

- **A7: `claudeNPMPayload` struct minimalism.** REFUTED. `claudeNPMPayload{Version string}` at `service.go:180-182` decodes only the `version` field. Go's `encoding/json` ignores unknown fields by default (stdlib contract). npm's `/latest` payload includes many fields (`name`, `description`, `dist`, etc.) — all silently ignored. Correct.

- **A8: Cross-platform npm fetch behavior.** REFUTED. `http.Client` is platform-independent stdlib. No CGO. macOS arm64/amd64 identical.

- **A9: Empty version string vs null.** REFUTED. Test `TestClaudeVersionResolverEmptyVersionReturnsError` at `service_test.go:637-649` covers `{"version":""}`. For `{"version":null}`, Go's JSON decoder maps JSON null to zero-value string `""` (stdlib contract). Same code path. Same test coverage.

- **A10: Auto-wire ordering.** REFUTED. `service.go:260-266` shows two INDEPENDENT `if` blocks (not `else if`):
  ```go
  if resolver == nil && provider == domain.ProviderCodex {
      resolver = NewCodexVersionResolver(nil)
  }
  if resolver == nil && provider == domain.ProviderClaude {
      resolver = NewClaudeVersionResolver(nil)
  }
  ```
  Future providers added between these get their own independent guards. No fallthrough trap. `provider` is a single value, so only one branch fires per `New()` call.

- **A11: nil-resolver protection in `New()`.** REFUTED. The `Service` struct at `service.go:74-86` is exported (`Service`, capital S) but ALL fields are unexported (`runner`, `stateStore`, `resolver`, etc.). External callers cannot construct `images.Service{resolver: ...}` directly. They could write `images.Service{}` (zero-value), but `EnsureLatest` explicitly nil-checks at `service.go:353-355`:
  ```go
  if s.resolver == nil {
      return EnsureResult{}, fmt.Errorf("ensure latest image: latest-version resolver is required")
  }
  ```
  Returns a wrapped error, NOT a nil-pointer panic. Defensive.

- **A12: httptest test isolation.** REFUTED. All 5 new tests use `defer server.Close()`:
  - `service_test.go:597` (happy path)
  - `service_test.go:614` (non-200)
  - `service_test.go:628` (bad JSON)
  - `service_test.go:642` (empty version)
  - `service_test.go:656` (network error — `Close()` called manually before the resolver call to force the failure)
  No goroutine leak.

- **A13: Network error test reliability.** REFUTED. `service_test.go:651-663` `TestClaudeVersionResolverNetworkErrorReturnsError` captures `server.URL`, calls `server.Close()` to free the port, then invokes `LatestVersion`. The test asserts only `err == nil` → `t.Fatal`, NOT a specific error class. Robust against dial-tcp vs timeout variations across platforms.

- **A14: `EnsureLatest` integration with auto-wired resolver.** NOTE (not a counterexample). `TestNewWiresClaudeVersionResolverWhenProviderIsClaudeAndNilResolver` at `service_test.go:665-678` asserts `svc.resolver != nil` via direct field access (same package). It does NOT exercise `EnsureLatest` with the auto-wired Claude resolver. The existing `EnsureLatest` tests inject `staticResolver` via `Options.Resolver`. So the integration of `claudeVersionResolver.LatestVersion` → `EnsureLatest` flow is tested transitively (resolver returns `"X.Y.Z"` via httptest; `EnsureLatest` handles strings produced by any `VersionResolver`) but not end-to-end in a single test. Coverage is sufficient. NOTE recorded.

- **A15: Concurrency on the resolver.** REFUTED. `http.Client` is documented goroutine-safe in net/http. The `claudeVersionResolver` struct is value-typed and `LatestVersion` is value-receiver — each call has a fresh request struct. No shared mutable state.

- **A16: Always-latest scope completeness.** REFUTED. Unit 7.7's acceptance criteria (PLAN.md lines 364-368) cover:
  - AC1: resolver constructor returns non-nil
  - AC2: `LatestVersion` returns valid semver
  - AC3: auto-wire in `New()` produces non-nil resolver for Claude provider
  - AC4: package tests pass

  Unit 7.7 does NOT claim "every `valv claude` launch uses latest". That claim belongs to Unit 7.8 (still `state: todo` per PLAN.md). Verified by direct read of `claude.go:181-197` — `ensureClaudeImageCurrent` STILL calls `service.Build(...)` with `DefaultClaudeCLIVersion`, NOT `service.EnsureLatest(...)`. The Unit 7.7 resolver is dormant in the launch path until Unit 7.8 lands. This is the planner's intentional decomposition.

- **A17: Anthropic's actual package name.** NOTE. Hardcoded at `service.go:28`. If Anthropic renamed to `@anthropic/claude-cli` (hypothetical), the resolver would 404. Not a current bug; acceptable to handle reactively. NOTE.

- **A18 (new): `MatchString` then `FindString` redundancy.** REFUTED. At `service.go:223-226`, after the regex match check, `FindString` is called to extract. Given the un-anchored pattern (A3), `MatchString` and `FindString` can return different STRINGS for substring inputs — but both confirm a match exists. Defensive and consistent with Codex's `normalizeCodexVersion`. Not a counterexample.

- **A19 (new): Double `TrimSpace` in version extraction.** REFUTED. Line 222: `strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(payload.Version), "v"))`. Outer trim handles whitespace from a hypothetical TrimPrefix output that shouldn't introduce any. Defensive, correct, no behavior change.

- **A20 (new): HTTP redirect handling.** REFUTED. Default `http.Client` follows up to 10 redirects. npm registry uses redirects sparingly. Behavior is stdlib-default.

- **A21 (new): Body close on early Do() error.** REFUTED. `defer resp.Body.Close()` is at `service.go:211` AFTER the err check at `:207`. Per `net/http` docs: when `Do` returns an error, the response body is closed by the package itself. No leak.

- **A22 (new): No `normalizeClaudeVersion` helper.** NOTE. Claude resolver inlines the trim+prefix+extract logic at `service.go:222-226`. Codex uses a helper `normalizeCodexVersion` (`:173-178`). Code-shape asymmetry. Not a correctness bug.

- **A23 (new): Empty body Decode failure.** REFUTED. Empty body → `json.NewDecoder(...).Decode(...)` returns `io.EOF`. Code at `:219` wraps as `"decode response: %w"` and returns. Correct.

- **A24 (new): npm 200 with HTML content-type.** REFUTED. Code checks only StatusCode (`:213`). HTML body with 200 → JSON Decode fails → error returned. Correct.

- **A25 (new): Empty version explicit check.** REFUTED. Line 223: `version == "" || !versionPattern.MatchString(version)`. Empty version (post-trim) fails the first clause. Test coverage at `service_test.go:637-649`.

- **A26 (new): Codex parity for `Accept` header.** REFUTED. Codex uses `application/vnd.github+json` (GitHub API media type); Claude uses `application/json` (generic). Both endpoints accept these. Not a counterexample.

- **A27 (new): `defaultClaudeLatestURL` is a hardcoded constant.** NOTE. External consumers cannot inject the URL via `NewClaudeVersionResolver(client)`. Tests use direct struct-field injection because they're in the same package. If a fork/adopter needs to point at a private npm mirror, they'd have to construct `claudeVersionResolver{client, url}` directly — but that's unexported. NOTE.

### Findings

- **NOTE 7.7-1 (regex anchoring symmetry):** `versionPattern` at `service.go:41` is shared with Codex and is not anchored. Theoretical risk of silent garbage stripping from malformed npm payloads. Not specific to Unit 7.7. Hardening would be a global change anchoring `versionPattern` with `^...$` AND splitting it into separate `validatePattern` (anchored) vs `extractPattern` (unanchored) — or just relying on the trim-and-trimprefix pipeline that already cleans inputs. NOT a Unit 7.7 fix.
- **NOTE 7.7-2 (auto-wire integration test gap):** `TestNewWiresClaudeVersionResolverWhenProviderIsClaudeAndNilResolver` (`service_test.go:665`) only asserts `svc.resolver != nil`. No end-to-end test exercises the auto-wired `claudeVersionResolver` through `EnsureLatest`. Coverage is transitive (resolver tested in isolation; `EnsureLatest` tested with `staticResolver`). Acceptable for Unit 7.7's scope; the gap closes naturally when Unit 7.8 wires `EnsureLatest` into the launch path with the auto-wired resolver active.
- **NOTE 7.7-3 (no `normalizeClaudeVersion` helper):** Code-shape asymmetry vs Codex. Inline trim+prefix+extract at `service.go:222-226` works correctly. Stylistic, not a correctness issue.
- **NOTE 7.7-4 (package rename risk):** Hardcoded `@anthropic-ai/claude-code` at `service.go:28`. If Anthropic renames the package, resolver 404s. Acceptable to handle reactively.
- **NOTE 7.7-5 (private mirror not supported):** `defaultClaudeLatestURL` is hardcoded; external callers cannot inject. Symmetric with Codex (`defaultCodexLatestURL` also hardcoded). Acceptable for v0.1.0.

No BLOCK findings.
No CONCERN findings worth blocking — A3's CONCERN about regex anchoring is symmetric with Codex (preexisting risk pattern, not introduced by Unit 7.7).

### Unknowns

None — all 27 attack vectors REFUTED with file:line evidence or marked NOTE (acceptable, recorded for future hardening). The 7.7-vs-7.8 scope split is honored by the committed code (verified via `claude.go:192` still calling `service.Build` not `service.EnsureLatest`); Unit 7.7's claim ("resolver exists, is auto-wired in `New()` for Claude provider, tested") survives all attacks.

## Hylla Feedback

- **Query:** `hylla_search_keyword` for `ensureClaudeImageCurrent` and `openImagesService` — zero results.
  - **Missed because:** Both symbols were either renamed/modified during DROP_7 work after the last Hylla ingest (snapshot 11 predates the Unit 7.7 commit) OR the Hylla index doesn't cover these specific tail symbols. The file `internal/cli/claude.go` itself is indexed (per other queries) but these function names didn't surface.
  - **Worked via:** `Read` tool on `internal/cli/claude.go` directly per CLAUDE.md § "Code Understanding Rules" item 2 (changed-since-ingest files).
  - **Suggestion:** None needed — mid-drop `Read` is the documented protocol. Hylla reingest at drop-end will pick these up.

---

## Unit 7.5 — Round 1

**Date:** 2026-05-15
**Verdict:** fail (one BLOCK + multiple CONCERN findings — Path B launch-vs-auth shape mismatch class is the exact failure mode that killed Rounds 1–4)

### Attack vectors probed

All 15 spawn-prompt attack vectors plus three invented (env-passthrough drift, missing image-ensure on auth path, USER override vs Dockerfile UID/GID).

- **A1 (bind-mount path mismatch).** REFUTED. Auth sets `Mounts: dockeradapter.NewMountSpec(homePath, claudeprovider.ContainerClaudeDir, false)` (claude_auth.go:78-80) targeting `/home/valv/.claude`. Launch sets the SAME target via runtime.go:115-117 (`dockeradapter.NewMountSpec(runtimeClaudeHome, ContainerClaudeDir, false)`). Both go to `claudeprovider.ContainerClaudeDir = "/home/valv/.claude"` (runtime.go:22). Identical target. Auth source = profileHome directly. Launch source = profileHome (when SharedHome empty) or a staged copy. Per service.go:127-133 SharedHome is intentionally empty so runtime collapses to profileHome — same as auth. No mismatch.
- **A2 (Container User mismatch).** REFUTED. Both auth and launch use `currentContainerUser()` which returns `"${UID}:${GID}"` of host process (operator_helpers.go:313-315). Launch via claude.go:97 (`User: currentContainerUser()`) → service.go Options.User → buildRequest User. Auth via claude_auth.go:86 (`User: currentContainerUser()`). Same UID:GID. Files written by auth-container land on host with the same UID that launch-container reads with. (Sub-note: this UID overrides the Dockerfile's `USER valv` UID 1000 — both auth and launch share this characteristic, so no drift.)
- **A3 (TERM env).** REFUTED. Auth inlines `strings.TrimSpace(os.Getenv("TERM"))` defaulting to `"xterm-256color"` (claude_auth.go:64-67). Launch calls `normalizedContainerTERM()` (runtime.go:122 → runtime.go:300-308) with identical logic. Match.
- **A4 (Args empty / entrypoint).** REFUTED. Dockerfile `ENTRYPOINT ["claude"]` (images/service.go:683). `Args: []string{}` (claude_auth.go:81) → container runs `claude` with zero arguments. Matches Anthropic devcontainer pattern + claudebox approach.
- **A5 (Interactive=true semantics for TTY stdin).** REFUTED. Auth sets `Interactive: stdin != nil` AND `TTY: commandHasTTY(stdin)` (claude_auth.go:82-83). When stdin is the real TTY (`os.Stdin` via cobra's InOrStdin) both are true → `docker run -i -t` (types.go:153-158). Correct for interactive OAuth paste prompt.
- **A6 (already-authed false positive on corrupt creds).** CONCERN-level. claude_auth.go:115-118 checks `info.Size() > 0` only. A partial multi-byte write (truncated JSON) would have `Size > 0` and short-circuit to nil → broken creds masquerade as authed. Smoke-test `rm -rf` reset path is fine; live users hitting container crash mid-write would have to manually wipe. Not a regression from prior rounds (Round 2 introduced this check with same shape).
- **A7 (docker adapter stdin/stdout/stderr forwarding).** REFUTED. `SystemRunner.Run` (os_runner.go:35-58) sets `cmd.Stdin = r.Stdin`, `cmd.Stdout = r.Stdout`, `cmd.Stderr = r.Stderr` before `cmd.Run()`. `NewSystemRunner` captures the caller's streams (os_runner.go:26-33). Auth path passes `cmd.InOrStdin(), cmd.OutOrStdout(), cmd.ErrOrStderr()` (claude_auth.go:62) which are `os.Stdin`/`os.Stdout`/`os.Stderr` for real CLI. The `os/exec` mechanism wires these to the docker-CLI subprocess fds directly (no buffer copy) → docker run pty forwarding works.
- **A8 (VALV_CLAUDE_IMAGE override bypassed for auth).** **CONFIRMED — CONCERN-level.** claude_auth.go:41-43 hardcodes `dockeradapter.NewImageRef("valv-claude", "dev")` in the package-level `hostClaudeAccountAuth` var, IGNORING `VALV_CLAUDE_IMAGE`. Launch path uses `claudeImageRef()` (claude.go:96 + claude_image.go:13-25) which DOES honor `VALV_CLAUDE_IMAGE`. Result: if a dev sets `VALV_CLAUDE_IMAGE=myorg/custom-claude:v3`, `valv account add claude` runs auth against `valv-claude:dev` (potentially different CLI version, potentially not built) while `valv claude` launches `myorg/custom-claude:v3`. Creds written by image A may be incompatible with image B. The builder's worklog (line 430) calls this "correct" but provides no rationale — PLAN.md does NOT lock this design choice (planner spec at PLAN.md:69 says use `claudeImageRef()` for the auth executor, mirroring claude.go's pattern). Builder DEVIATED from spec.
- **A9 (container name uniqueness).** REFUTED. Nanosecond-precision timestamp + `Remove: true` makes name reuse astronomically unlikely. Same shape as `runClaudeImageOnlyCommand` (claude.go:137) — established pattern.
- **A10 (Init: true).** REFUTED. tini wraps entrypoint for signal handling; launch sets `Init: s.tty || s.stdin` (service.go:270) which is true in the same conditions auth runs in. No divergence.
- **A11 (test stub vs production runner gap).** CONCERN-level (test-coverage). `stubClaudeAccountAuthRunner` with `stubRunFunc` writes `.credentials.json` directly to homePath (claude_auth_test.go:35-42). Production runner spawns a real docker container that internally writes the file via the bind-mount. The stub proves "if container writes credentials, the function succeeds" but NOT "the production runner actually causes container to write credentials." This is the exact integration gap that masked Path A's keychain-format mismatch through 4 green-CI rounds. Acceptable for unit-test layer; the verification gate MUST be the dev's smoke test. PLAN.md line 28-40 prescribes the smoke test — orchestrator should not mark drop done until that smoke test passes.
- **A12 (no test for production-runner construction).** REFUTED. `systemClaudeAccountAuthRunner.RunInContainer` is only exercised when stub injection is absent. No unit test covers the docker-adapter construction path. Same as Codex's `systemCodexAccountAuthRunner` pattern (account_auth.go:132) — established norm. Init failure surfaces at smoke-test time.
- **A13 (claudeImageRef reuse).** Tied to A8 — see CONFIRMED finding above. Auth does NOT call `claudeImageRef()`; launch does. Drift exists.
- **A14 (SkipLogin caller coverage).** REFUTED. `git grep SkipLogin`: callers are manage.go:493 (`runManageAccountAdd`) and manage.go:600 (`runManageAccountSwitch`), both threading a user-supplied `--skip-login` flag. Both pre-existed Path A. Both correctly route to early-return. No new caller introduced unsafe SkipLogin.
- **A15 (plain `claude` auto-prompt assumption).** EXHAUSTED, no counterexample found. Per PLAN.md "Path B" rationale and Anthropic devcontainer docs (https://code.claude.com/docs/en/devcontainer), plain `claude` in a bind-mounted container with TTY + missing credentials prompts for OAuth in-terminal. claudebox confirms working impl. No code-level counterexample available; live smoke test is the gate.
- **Invented: ENV PASSTHROUGH DRIFT.** **CONFIRMED — CONCERN-level.** Launch sets `EnvPassthrough: prepared.EnvPassthrough` via runtime.go:161 → `terminalEnvPassthrough()` at runtime.go:283-298, which passes `COLORTERM`, `TERM_PROGRAM`, `TERM_PROGRAM_VERSION`, `LANG`, `LC_CTYPE` THROUGH to the container (when set on host). Auth's `ContainerRunRequest` at claude_auth.go:68-87 sets ZERO `EnvPassthrough` — only the explicit Env map (CLAUDE_CONFIG_DIR, HOME, LOGNAME, TERM, USER) is forwarded. Inside the auth container: no `LANG`, no `LC_CTYPE`, no `COLORTERM`, no `TERM_PROGRAM`. Claude's TUI may render the OAuth prompt with broken color or incorrect locale. The DROP_6.2 invisible-paste-prompt failure was a TUI-through-Docker-pty class bug — this is a sibling risk. The smoke test will either work or fail visibly; logged as CONCERN because PLAN.md does not explicitly require parity with launch's EnvPassthrough.
- **Invented: MISSING ensureClaudeImageCurrent ON AUTH PATH.** **CONFIRMED — BLOCK-level.** `runManageAccountAdd` (manage.go:462-527) calls `ensureManagedAccountReady` (manage.go:493) → `ensureClaudeAccountReady` → `runner.RunInContainer` → `docker run --rm -i -t valv-claude:dev claude`. There is NO `ensureClaudeImageCurrent` call anywhere in this chain. On a fresh install (`mage install` + no prior `valv claude` ever invoked), `valv-claude:dev` does NOT exist locally → `docker run` fails with `"Unable to find image 'valv-claude:dev' locally"` (no implicit pull, since it's a local-only tag). Path A's host-subprocess flow did NOT need the image, so this gap is a Path B REGRESSION introduced by Unit 7.5. The dev's smoke test in PLAN.md line 28-40 is `mage install` then `valv account add claude work` — if no prior image build exists on that machine, this smoke test fails BEFORE the OAuth prompt ever appears. Even if the dev's machine happens to have a cached image from prior smoke tests, a fresh CI runner or a new contributor's machine will hit this. PLAN.md "Round 5 scope" says "first launch" auths in-container — but `valv account add` precedes `valv claude`, so there is no prior launch to seed the image. Fix: `runManageAccountAdd` must call `ensureClaudeImageCurrent` (or an equivalent build/pull guard) before `ensureManagedAccountReady` when provider == ProviderClaude.
- **Invented: USER override vs Dockerfile UID/GID.** REFUTED (with caveat). Dockerfile sets `USER valv` (UID 1000); auth/launch both override with `currentContainerUser()` = host `"${UID}:${GID}"` (typically 501:20 on Mac). Result: `/home/valv/` is owned by 1000:1000 inside the image but the process runs as 501. The bind-mounted subdir `/home/valv/.claude` is owned by host UID 501 (file ownership flows through bind-mount) — claude writes `.credentials.json` succeed. Both auth and launch share this characteristic so no drift. Caveat: if claude tries to write to `/home/valv/` itself (not under `.claude/`), it would fail (perm denied) — but Anthropic CLI scopes writes to `$CLAUDE_CONFIG_DIR`.

### Findings

- **BLOCK 1 — Missing image-ensure on auth path.** `runManageAccountAdd` (internal/cli/manage.go:462-494) does NOT call `ensureClaudeImageCurrent` before `ensureManagedAccountReady` for Claude. `valv account add claude work` on a fresh install (the PLAN.md smoke test) will fail at `docker run valv-claude:dev` with image-not-found before the OAuth prompt can render. Path A did not have this dependency because it auth'd on the host. Fix: gate `ensureManagedAccountReady` on a `provider == ProviderClaude` image-ensure step. Smallest fix: in `runManageAccountAdd`, after profile creation, if provider is Claude and not SkipLogin, call `ensureClaudeImageCurrent(cmd, paths)`. Same pattern in `runManageAccountSwitch` (manage.go:600) needs the same guard.
- **CONCERN 1 — VALV_CLAUDE_IMAGE drift.** Auth runner var hardcodes `valv-claude:dev` (claude_auth.go:41-43) instead of calling `claudeImageRef()`. Launch path honors `VALV_CLAUDE_IMAGE`. Per PLAN.md:69 the spec lock says use `claudeImageRef()` ("same callsite pattern as claude.go"); builder deviated. If a contributor sets `VALV_CLAUDE_IMAGE` for testing, auth and launch will use different images → potentially different CLI versions writing/reading the same credentials file. Fix: replace the hardcoded `NewImageRef("valv-claude", "dev")` with a call to `claudeImageRef()`. If hardcoding is intentional, document the rationale in claude_auth.go AND in PLAN.md design decisions.
- **CONCERN 2 — EnvPassthrough drift.** Auth container does not pass through `COLORTERM`, `TERM_PROGRAM`, `TERM_PROGRAM_VERSION`, `LANG`, `LC_CTYPE` to the auth container; launch does (runtime.go:283-298 via service.go:258). Without LANG/LC_CTYPE, the claude TUI prompt may render badly through the docker pty — same class as DROP_6.2's invisible-paste-prompt failure. Smoke test will surface this if it triggers; CONCERN until confirmed in live test. Fix: add `EnvPassthrough` field to the auth `ContainerRunRequest` mirroring `terminalEnvPassthrough()`'s output. Inline duplication or, better, export `terminalEnvPassthrough` from clauderuntime.
- **CONCERN 3 — Test stub masks production-runner integration gap.** `stubClaudeAccountAuthRunner.stubRunFunc` (claude_auth_test.go:35-42) writes `.credentials.json` directly, bypassing the real docker container path. The unit-test success path proves "if container writes creds the function succeeds" but cannot prove "the production runner actually causes container to write creds." This is the exact integration gap that let Path A pass 4 green-QA rounds with broken end-to-end behavior. The orchestrator MUST gate drop-complete on the dev's live smoke test (PLAN.md line 28-40), not just `mage testPkg`.
- **NOTE 1 — Already-authed corrupt creds.** claude_auth.go:115-118's `info.Size() > 0` early-return treats a truncated/partial `.credentials.json` as authed. Smoke-test reset handles dev path; live users hitting container crash mid-write must manually wipe. Pre-existing from Round 2; not a Unit 7.5 regression.

### Unknowns

- Whether claude's in-container OAuth prompt actually renders correctly without LANG/LC_CTYPE passthrough — only the dev's live smoke test can resolve this. Routed to orchestrator + dev.
- Whether `valv-claude:dev` happens to be present on the dev's machine from a prior smoke test, masking BLOCK 1 in the immediate retest but leaving the regression in place for fresh installs / CI. Orchestrator should consider explicitly `docker image rm valv-claude:dev` before re-running the smoke test to expose the gap.

## Hylla Feedback

- **Query**: `hylla_search_keyword` for `SkipLogin: true` with `id_search_mode=exact_full_id` and `field=content`.
  - **Missed because**: Hylla returned only the top-10 lexical matches on summary/docstring, none of which contained "SkipLogin" — Hylla does not appear to index literal field-assignment text (`{SkipLogin: true, ...}`) in block summaries/docstrings.
  - **Worked via**: `git grep -n SkipLogin -- 'internal/**/*.go'` (Bash, allowed).
  - **Suggestion**: index struct-literal field references (or full content text) in block embeddings so call-site searches for `Field: value` patterns return the surrounding block.
- **Query**: `hylla_search_keyword` for `claudeImageRef` and `currentContainerUser` — both returned zero hits.
  - **Missed because**: Both are unexported package-private functions in `internal/cli`. The `tail_symbol` search mode didn't surface them; `visibility_mode=public_only` default may have hidden internal symbols even though `internal_mode=include_internal` was specified.
  - **Worked via**: `Read` tool on `claude_image.go` and `operator_helpers.go` directly after listing the directory.
  - **Suggestion**: when both visibility filters and internal-mode flags are set, a same-package unexported function should still be discoverable by `tail_symbol`. Either tighten the docs on which flag combination returns unexported symbols, or surface them by default for in-artifact queries.

---
