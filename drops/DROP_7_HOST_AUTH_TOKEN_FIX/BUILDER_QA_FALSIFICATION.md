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

## Unit 7.5 — Round 2

**Date:** 2026-05-15
**Verdict:** pass

### Attack vectors probed

All 15 spawn-prompt attack vectors plus 4 invented (Codex-regression, `runManageAccountSwitch` test gap, empty-env-passthrough edge, `--no-bind` interaction).

- **A1 (`!skipLogin` gating off-by-one).** REFUTED. `TestManageAccountAddClaudeWithSkipLoginSkipsImageEnsure` (manage_test.go:385) explicitly does NOT install `installFakeDocker`. If the gate dropped the `!skipLogin` clause and called `ensureClaudeImageCurrent`, the test would fail at `findDockerBinary` lookup. The gate is real and behavior-pinned. Code at manage.go:497: `if provider == domain.ProviderClaude && !skipLogin { ... }`.
- **A2 (image-ensure on already-authed reinvocation).** REFUTED (NOTE). `ensureClaudeImageCurrent` runs BEFORE `ensureManagedAccountReady`, so a second `valv account add claude work` invocation against an authed profile still incurs a docker-inspect or images-service round-trip even though the auth path short-circuits at the already-authed check. Symmetric with Codex launch-path's always-running `ensureCodexImageCurrent`. Acceptable cost; documented.
- **A3 (`claudeImageRef()` evaluated at package init time).** REFUTED with caveat. `claudeImageRef()` (claude_image.go:13-25) reads `VALV_CLAUDE_IMAGE` at call-time. The package-level `hostClaudeAccountAuth` var (claude_auth.go:46-48) calls `claudeImageRef()` ONCE at package init. In production this captures the launch-time env (correct: CLI invocations set env before init). In tests, `t.Setenv` after package init does NOT retroactively change the package var — but the R2 test does NOT rely on it (constructs a fresh runner inline). Builder explicitly documented this caveat in BUILDER_WORKLOG.md:509. Not a counterexample.
- **A4 (FIX 2 test verifies helper, not package var).** REFUTED. `TestSystemClaudeAccountAuthRunnerUsesClaudeImageRef` (claude_auth_test.go:416) builds `systemClaudeAccountAuthRunner{image: claudeImageRef()}` inline AFTER `t.Setenv("VALV_CLAUDE_IMAGE", "test/myimg:v2")` — this verifies the construction PATTERN (the same `image: claudeImageRef()` literal used in the package-var initializer at claude_auth.go:47). Production behaviour follows by mechanical equivalence: package-init reads env at startup, test reads env at call. Pattern correctness is pinned; package-var init-time semantics rely on CLI semantics (env-before-launch).
- **A5 (`TerminalEnvPassthrough` returns mutable slice).** REFUTED. runtime.go:295-301 builds a fresh `out := make([]string, 0, len(names))` per call. Callers cannot mutate shared state. No append-with-grow leak. Defensive copy implicit in the construction.
- **A6 (env passthrough sufficiency for OAuth TUI).** REFUTED (NOTE). Context7 query against `/websites/code_claude` confirms claude OAuth+container is a documented terminal-rendering friction zone (WSL/SSH/devcontainer). The forwarded vars (runtime.go:288-294: `COLORTERM`, `TERM_PROGRAM`, `TERM_PROGRAM_VERSION`, `LANG`, `LC_CTYPE`) match the launch-path set. Anthropic docs do not mandate this specific list; the fix is defensive symmetry with the launch path. Whether this actually fixes the OAuth TUI rendering across pty is a live smoke-test question — not code-falsifiable.
- **A7 (Go doc comment convention).** REFUTED. `go doc github.com/evanmschultz/valv/internal/adapters/providers/claude TerminalEnvPassthrough` returns the comment starting with the function name (`TerminalEnvPassthrough returns ...`). godoc/golint convention met.
- **A8 (provider-dispatch placement).** REFUTED. `provider` is the original `runManageAccountAdd` argument (manage.go:462) — never zero-valued. The guard at manage.go:497 fires after `service.CreateProfile` (manage.go:488), so the profile exists before the image-ensure step. Codex calls skip the guard via the `provider == domain.ProviderClaude` clause. Verified by reading manage.go:462-501 inline.
- **A9 (error propagation from `ensureClaudeImageCurrent`).** REFUTED. manage.go:498-500 (`return fmt.Errorf("manage account add: %w", err)`) and manage.go:613-615 (`return fmt.Errorf("manage account switch: %w", err)`) both `%w`-wrap the underlying error. Errors propagate; no swallow.
- **A10 (latency cost on already-existing image).** REFUTED (covered by A2). Symmetric with Codex launch cost; acceptable.
- **A11 (SkipLogin + missing image + subsequent launch).** REFUTED. `runClaudeCommand` (claude.go:113) calls `ensureClaudeImageCurrent` unconditionally on every `valv claude` invocation. So a user who runs `valv account add claude work --skip-login` without the image, then later runs `valv claude`, gets the image build at launch time. Edge case covered upstream.
- **A12 (`t.Setenv` parallel-test race).** REFUTED. All three env-setting tests omit `t.Parallel()`:
  - `TestSystemClaudeAccountAuthRunnerUsesClaudeImageRef` (claude_auth_test.go:416) — no `t.Parallel()`.
  - `TestSystemClaudeAccountAuthRunnerPassesThroughTerminalEnv` (claude_auth_test.go:435) — no `t.Parallel()`.
  - `TestManageAccountAddClaudeWithExistingCredsAndImageOverrideSucceeds` (manage_test.go:418) — no `t.Parallel()`.
  - `TestManageAccountAddClaudeWithSkipLoginSkipsImageEnsure` (manage_test.go:385) DOES call `t.Parallel()` because it never sets env — safe.
  Go 1.26's panic-on-mismatch rule honored across the board.
- **A13 (`runManageAccountSwitch` target-provider dispatch).** REFUTED. `resolveProfileSwitchTarget` (manage.go:585) yields the TARGET `provider`. The guard at manage.go:612 (`if provider == domain.ProviderClaude && !skipLogin`) fires on the target. Switching FROM Codex TO Claude correctly ensures the Claude image; FROM Claude TO Codex correctly skips.
- **A14 (`mage` re-run flake check).** REFUTED. Independent re-runs of all three packages:
  - `mage testPkg github.com/evanmschultz/valv/internal/cli` → 154/154 pass, 71.9% coverage. Matches worklog.
  - `mage testPkg github.com/evanmschultz/valv/internal/adapters/providers/claude` → 21/21 pass, 78.4% coverage. Matches.
  - `mage testPkg github.com/evanmschultz/valv/internal/services/claude` → 17/17 pass, 81.0% coverage. Matches.
  No flakes; no race detector hits.
- **A15 (Anthropic devcontainer docs cross-check).** NOTE. Context7 docs confirm OAuth TTY rendering issues in container contexts but do not enumerate required env vars. FIX 3 is defensive symmetry with the launch-path passthrough — acceptable design choice, gated on live smoke test.
- **A16 (invented — Codex regression).** REFUTED. The guard short-circuits on `provider == domain.ProviderClaude`. Reading manage.go:462-535 confirms Codex's pre-existing flow (`service.CreateProfile` → `ensureManagedAccountReady` → bind/output) is byte-for-byte preserved. Existing Codex tests `TestManageAccountAddCreatesIsolatedNamedAccountAndBindsProject` etc. all pass per the GREEN re-run.
- **A17 (invented — `runManageAccountSwitch` has zero test coverage).** **CONCERN — not a counterexample.** The R2 guard was added to BOTH `runManageAccountAdd` AND `runManageAccountSwitch`, but only the add-path has test coverage. Hylla `hylla_search_keyword runManageAccountSwitch` (with `id_search_mode=tail_symbol`, `visibility_mode=include_private`) returns ONLY the production definition + cobra wrapper — zero test callers. A future regression that drops the switch-path guard or its `!skipLogin` clause would pass `mage test` green. Code is correct by mirror-construction (byte-for-byte parity with add-path verified by `git diff`), but symmetry-by-inspection is the only gate. NOT BLOCK because the add-path test pins the pattern; pattern is duplicated mechanically. Recommendation: a follow-up unit could add `TestManageAccountSwitchClaudeWithSkipLoginSkipsImageEnsure` to close the symmetry gap.
- **A18 (invented — empty env passthrough when no host vars set).** REFUTED. runtime.go:295-300 — when neither `COLORTERM`/`LANG`/`LC_CTYPE`/`TERM_PROGRAM`/`TERM_PROGRAM_VERSION` is set on host, `out` is `[]string{}` (zero-length slice from `make`). `ContainerRunRequest.EnvPassthrough = []string{}` is valid — `docker run -e VAR` only gets emitted for each name in the list, and the empty list emits nothing. No panic, no malformed argv. The new test explicitly sets `LANG`/`LC_CTYPE` and asserts they appear in the passthrough — happy path covered.
- **A19 (invented — `--no-bind` flag interaction).** REFUTED. `--no-bind` only affects whether `service.BindProject` runs (manage.go:506). The image-ensure + auth flow runs regardless. `TestManageAccountAddClaudeWithExistingCredsAndImageOverrideSucceeds` uses `--no-bind` and verifies the full flow including the image-ensure step succeeds.

### Findings

- **NOTE A2 (image-ensure latency on second add).** Already-authed reinvocations still incur a docker-inspect / images-service round-trip. Symmetric with Codex launch-path semantics. Acceptable. Document in release notes if relevant.
- **NOTE A3+A4 (FIX 2 test verifies pattern not package-var init).** R2 worklog Unknowns already routes this to dev (BUILDER_WORKLOG.md:509). Production correctness rests on the env-before-launch CLI invariant — not testable in-process. Symmetric with `claudeImageRef()`'s call-time semantics in `claude.go:96`. Acceptable.
- **NOTE A6 + A15 (env passthrough live verification).** Whether `LANG`/`LC_CTYPE` passthrough actually fixes the OAuth TUI rendering through Docker pty is a live smoke-test question. R1 CONCERN 2 anticipated this; FIX 3 is defensive symmetry with the launch path. Gate on smoke test.
- **CONCERN A17 (`runManageAccountSwitch` test gap).** Guard added to switch-path but no test exercises it. Pattern is byte-for-byte mirror of the add-path; add-path test pins the pattern. Recommendation: add `TestManageAccountSwitchClaudeWithSkipLoginSkipsImageEnsure` (and a positive-path twin mirroring `TestManageAccountAddClaudeWithExistingCredsAndImageOverrideSucceeds`) in a follow-up to close the symmetry gap. Not blocking R2 because the structural mirror is verifiable by code inspection.

### R1 finding closure

- **BLOCK 1 (missing `ensureClaudeImageCurrent` on auth path):** **CLOSED.** manage.go:497-501 (add) and manage.go:612-616 (switch) both call `ensureClaudeImageCurrent(cmd, paths)` gated on `provider == domain.ProviderClaude && !skipLogin`. `TestManageAccountAddClaudeWithExistingCredsAndImageOverrideSucceeds` exercises the path positively. `TestManageAccountAddClaudeWithSkipLoginSkipsImageEnsure` proves the SkipLogin gate by absence of fake docker (would fail at lookup if guard broken). Switch-path closure is by mirror-construction (A17 above).
- **CONCERN 1 (`VALV_CLAUDE_IMAGE` drift between auth and launch):** **CLOSED.** claude_auth.go:46-48 uses `claudeImageRef()` instead of the hardcoded `dockeradapter.NewImageRef("valv-claude", "dev")`. `TestSystemClaudeAccountAuthRunnerUsesClaudeImageRef` (claude_auth_test.go:416) pins the env-respect contract for the construction pattern. Production package-var init-timing caveat documented in worklog Unknowns and acceptable per CLI semantics (A3/A4 above).
- **CONCERN 2 (`EnvPassthrough` drift on auth container):** **CLOSED.** runtime.go:287-301 exports `TerminalEnvPassthrough` with Go-doc comment and is called by both PrepareRuntime (runtime.go:126) and the auth runner (claude_auth.go:88). `TestSystemClaudeAccountAuthRunnerPassesThroughTerminalEnv` (claude_auth_test.go:435) asserts `LANG`/`LC_CTYPE` are in the auth-container's `EnvPassthrough`. Live OAuth-render verification remains a smoke-test question (A6/A15 NOTE).

### Counterexamples

None CONFIRMED.

### Unknowns

- Live smoke test outcome (`mage install` + fresh-install `valv account add claude work`) — routed to dev per R2 worklog. Not code-falsifiable.
- Whether the env passthrough actually fixes the OAuth TUI rendering (A6/A15) — same as above, dev smoke test.
- A17 test gap (`runManageAccountSwitch`) — non-blocking, follow-up recommendation.

### Verdict

**PASS.** R2 closes all three R1 findings with file:line evidence and behavior-pinned tests:
- BLOCK 1: guard added to BOTH `runManageAccountAdd` and `runManageAccountSwitch`, gated on `!skipLogin`, positively tested via `TestManageAccountAddClaudeWithExistingCredsAndImageOverrideSucceeds` and negatively pinned via `TestManageAccountAddClaudeWithSkipLoginSkipsImageEnsure`.
- CONCERN 1: `claudeImageRef()` replaces hardcoded ref; helper pattern tested.
- CONCERN 2: `TerminalEnvPassthrough` exported with Go-doc convention; auth runner wires it; locale-var passthrough tested.

No introduced regressions: Codex flow untouched (mirror of pre-R2), test count delta consistent (151→154), coverage 71.9% (above 70% floor), all three package-level `mage testPkg` runs GREEN with matching counts. The three NOTEs and one CONCERN are recommendations for future polish, not R2 defects. Live smoke test remains the integration gate.

## Hylla Feedback

- **Query:** `hylla_search_keyword` for `installFakeDocker` (`fields=content`, `visibility_mode=include_private`, `test_mode=include_tests`).
  - **Worked correctly.** Hylla returned the test-helper definition in `extended_test.go` plus all caller tests (8 callers across extended_test.go + manage_test.go). The summary docstring described the shell-script content + log path return — sufficient on its own without reading the file. Confirms `installFakeDocker` is a `t.Setenv("PATH", ...)`-based fake docker that exits 0 unless specific env-overrides are set.
  - **Suggestion:** None — ideal hit.
- **Query:** `hylla_search_keyword` for `runManageAccountSwitch` (`tail_symbol`, `visibility_mode=include_private`).
  - **Worked correctly.** Returned exactly 2 results: the production definition + the cobra command wrapper. Zero test callers — directly evidenced the A17 test-gap concern.
  - **Suggestion:** None.
- Other code reads (claude_auth.go, runtime.go, manage.go, claude_auth_test.go, manage_test.go) used `Read` tool directly per mid-drop staleness protocol — these files were modified in `d5d1fa6` after the last Hylla snapshot. No fallback miss to log.

---

## Unit 7.8 — Round 1

**Date:** 2026-05-16
**Verdict:** pass

### Attack vectors probed

- **A1: `ensureClaudeImageCurrent` registry-down behavior with `AllowExistingOnCheckFail: true`.** REFUTED. Read `EnsureLatest` at `internal/services/images/service.go:367-388`. When `resolver.LatestVersion` errors AND `AllowExistingOnCheckFail` is true, `EnsureLatest` checks `imageAvailable(defaultRef)`. If image exists locally → returns `EnsureActionUsingExistingImage` with `Action`, `LatestVersion: state.LatestVersion`, `PreviousVersion: state.InstalledVersion`, `LatestCheckedAt: state.LatestCheckedAt` (state-store values) and `nil` error. If image does NOT exist locally → returns wrapped error `"ensure latest image: resolve latest claude version: <error>"`. On a fresh install with registry down, user sees a clear error; on an existing install with registry down, launch proceeds silently with the installed image. Contract is correct.
- **A2: `runManageUpdateClaude` no `AllowExistingOnCheckFail` (zero-value `EnsureRequest{}`).** REFUTED. Registry failure surfaces with the actionable wrapped error chain `manage update: ensure latest image: resolve latest claude version: <http details>`. The CLI heading is `"Provider image update failed"` (spinner failure label). Matches Codex behavior at `manage.go:1126`.
- **A3: Latency cost — 24h cache promise from PLAN.md round-5 design.** **CONCERN** (not BLOCK). PLAN.md line 20 explicitly promises: "Wire latest-check into launch path for BOTH providers with a 24h cache to avoid per-launch latency." Unit 7.8 wires the launch path but ships NO cache layer. Every `valv claude` launch makes an HTTPS round-trip to `registry.npmjs.org/@anthropic-ai/claude-code/latest` with a 10s HTTP client timeout. The Codex side has the same gap (Codex resolver also has no cache) pre-DROP_7, so this is a planner-locked design promise that Unit 7.8 silently inherited from the Codex baseline. UX impact: every Claude/Codex launch is up to 10s slower on flaky/offline networks. Not a BLOCK because the `AllowExistingOnCheckFail: true` fallback masks the failure case — but the latency is real on slow-but-not-failing networks. Falls under YAGNI-deferred-but-promised. Routes to orchestrator/dev for decision (drop scope-creep into a follow-on, or accept).
- **A4: `checked at` label rendering as `checked_at=` in plain output.** REFUTED. Read `internal/output/output.go:62`: `strings.ReplaceAll(strings.ToLower(field.Label), " ", "_")` — the underscore conversion is package-level convention applied to ALL fields with spaces (e.g. `"git marker"` → `git_marker` in `status`). Both Codex (`manage.go:1144`) and Claude (`manage.go:1176`) use identical label `"checked at"` and render identically as `checked_at=...`. Symmetric. JSON output applies the same conversion at line 43. Consistent across formats.
- **A5: `result.LatestCheckedAt.Format(time.RFC3339)` precision and zero-value risk.** REFUTED for the manage-update path. `EnsureRequest{}` (zero-value, `AllowExistingOnCheckFail: false`) means registry failure errors out before constructing a result — no zero-value timestamp surfaces. The two success paths populate `LatestCheckedAt`: `EnsureActionUpToDate` at `service.go:408` writes `state.LatestCheckedAt = checkedAt` (where `checkedAt := time.Now().UTC()` at line 391), and `EnsureActionUpdated` at line 470 returns `LatestCheckedAt: checkedAt` directly. Both are UTC-normalized. No zero-value `"0001-01-01T00:00:00Z"` can reach the user output via this path.
- **A6: Heading branch logic — other `EnsureAction` values.** REFUTED. Three values exist: `EnsureActionUpdated`, `EnsureActionUpToDate`, `EnsureActionUsingExistingImage` (`service.go:111-115`). The `runManageUpdateClaude` path uses `EnsureRequest{}` which forbids `EnsureActionUsingExistingImage` (that action only fires when `AllowExistingOnCheckFail: true`). So `runManageUpdateClaude` can only see `Updated` or `UpToDate` — both correctly handled by the conditional. `ensureClaudeImageCurrent` (the launch path) uses `AllowExistingOnCheckFail: true` so `EnsureActionUsingExistingImage` IS reachable there; but it discards the result with `_`. Behavior is sound. NOTE: Codex side logs a debug message for `EnsureActionUsingExistingImage` at `codex.go:252-254`; Claude side at `claude.go:190-194` does NOT. Divergence flagged as NOTE below.
- **A7: `claudeVersionResolverFactory` invocation timing.** REFUTED for correctness, but reinforces A3. Each `openImagesService(cmd, paths, ProviderClaude)` call invokes `claudeVersionResolverFactory(nil)` once, constructing a fresh `claudeVersionResolver{client: &http.Client{Timeout: 10s}, url: defaultClaudeLatestURL}`. The resolver's `LatestVersion(ctx)` makes the HTTP call when called. The service is constructed per `ensureClaudeImageCurrent` invocation (per launch) — so per-launch construction is real. No staleness, but also no cache (A3).
- **A8: `VALV_CLAUDE_IMAGE` override early-exit ordering.** REFUTED. `claude.go:182-184`: `if strings.TrimSpace(os.Getenv("VALV_CLAUDE_IMAGE")) != ""` is the FIRST statement in `ensureClaudeImageCurrent` and returns immediately via `ensureClaudeImageAvailable`. The npm registry call is never reached. Symmetric with `codex.go:240-242`.
- **A9: Concurrent `valv claude` invocations.** NOT A FINDING (out of Unit 7.8 scope). The PLAN.md Round 5 design explicitly defers concurrency control. Two simultaneous `valv claude` calls in different terminals will each hit the npm registry; docker image-inspect / build calls are not Valv-locked. This was acceptable pre-DROP_7 and remains acceptable post-Unit-7.8.
- **A10: Integration / golden coverage.** REFUTED. Builder noted "No golden fixtures exist for `runManageUpdateClaude` — no `mage goldenUpdate` needed." Confirmed by inspection of `internal/cli/testdata/` — only `TestCodexInteractiveMCPGolden.golden` exists (Codex interactive surface, not manage-update). The unit's user-facing output is plain-text record output, covered by string-contains assertions in `TestRunManageUpdateClaudeBuildsImage`. AGENTS.md § 11 requires `mage integration` for Docker-backed paths; the unit's docker calls go through `installFakeDocker` (no real docker), so `mage integration` adds no Unit 7.8 coverage. No gap.
- **A11: Spinner text mirror.** REFUTED. Both `runManageUpdateCodex` (`manage.go:1124-1126`) and `runManageUpdateClaude` (`manage.go:1156-1158`) pass identical spinner strings: `"Checking provider image"` / `"Provider image check complete"` / `"Provider image update failed"`. Identical. The spinner messaging is generic ("provider") so it doesn't lie about Claude specifically.
- **A12: Output field ordering.** REFUTED. Codex (`manage.go:1144`): `provider, image, tags, version, checked at, context`. Claude (`manage.go:1176`): `provider, image, tags, version, checked at, context`. Identical field order, identical Muted/Identifier flags. Fully symmetric.
- **A13: `DefaultClaudeCLIVersion` fallback usage after Unit 7.8.** **NOTE**. Hylla inbound-ref search (stale snapshot) returned five references, of which two were production callers (`ensureClaudeImageCurrent`, `runManageUpdateClaude`). Post-Unit-7.8, both production callers no longer reference the constant — verified by reading the current `claude.go` and `manage.go` files. Remaining references are only test files (`TestDefaultClaudeCLIVersionIsNonEmpty`, `TestServiceBuildRecipeHashMatchesProviderDockerfile`) and the value is dead in production paths. The constant + the `TestDefaultClaudeCLIVersionIsNonEmpty` test are now YAGNI residue. Not a BLOCK — single constant, low maintenance cost. Suggest deletion in a follow-on cleanup unit (the parallel Codex constant `DefaultCodexCLIVersion`, if it exists, deserves the same treatment for symmetry).
- **A14: Test stub coverage of EnsureLatest pathways.** **CONCERN**. `TestRunManageUpdateClaudeBuildsImage` (`manage_test.go:265-316`) exercises ONLY the `EnsureActionUpdated` heading branch (fresh install with fake docker forces a rebuild). It does NOT exercise: (a) `EnsureActionUpToDate` heading branch — the second-run case where the conditional `if result.Action == imagesservice.EnsureActionUpToDate { heading = "Provider image up to date" }` (line 1173-1175) fires. Codex side has a corresponding test `TestManageUpdateSecondRunReportsUpToDate` (`extended_test.go:503-519`) that sets `VALV_DOCKER_IMAGE_INSPECT_OUTPUT=fakeCodexRecipeHash()`, runs `update` twice, and asserts "Provider image up to date". Claude has NO such test. The new conditional heading is shipped but uncovered by a Claude-specific test. The branch IS covered by the Codex test exercising the same EnsureLatest code path through the provider-agnostic `images.Service`, so the conditional itself is provably correct — but the Claude-specific assertion (heading string `"Provider image up to date"`) is unverified.
- **A14b: Test stub coverage of `ensureClaudeImageCurrent`.** **CONCERN**. The `runClaudeCommand` launch path's new `EnsureLatest` call (`claude.go:190`) has NO unit test. Codex has `TestEnsureCodexImageCurrentAutoUpdatesWhenNoOverrideIsSet` (`extended_test.go:661-682`). No `TestEnsureClaudeImageCurrentAutoUpdatesWhenNoOverrideIsSet` exists. Build is green because `runClaudeCommand` integration is not exercised by any test — but Unit 7.8 introduced the per-launch network call. Builder noted "No test changes needed" per spec line 411, which matches AC4/AC5 — but the spec authority gap propagates to test coverage. Not a BLOCK (no new branch-without-test in `claude.go` — the conditional is just `if err != nil { return err }`), but the symmetry to Codex is asymmetric in test coverage. Falsifies the symmetry claim, not the correctness claim.
- **A15: Mutation of installed-version registry — version-direction coverage.** REFUTED. The `stubClaudeVersionResolver` returns a static version string regardless of installed state. The Codex test `TestManageUpdateSecondRunReportsUpToDate` exercises the up-to-date path; the analogous Claude path is uncovered (see A14) but uses the same provider-agnostic `images.Service`. No production-code branch is provider-specific, so the same code IS tested via the Codex variant. Defensible.
- **A16: `LatestCheckedAt` time-zone determinism.** REFUTED. `checkedAt := time.Now().UTC()` (`service.go:391`) UTC-normalizes the value. `time.RFC3339` format renders deterministically as `"YYYY-MM-DDTHH:MM:SSZ"`. Cross-host consistent.
- **A17: HTTP response body close in `claudeVersionResolver.LatestVersion`.** REFUTED. `defer resp.Body.Close()` at `service.go:211`. Body closed unconditionally before any non-200 branch reads it (via `io.LimitReader`, line 214). Standard pattern. No leak.
- **A18: Context cancellation during npm fetch (self-invented).** REFUTED. `http.NewRequestWithContext(ctx, ...)` (line 200) propagates the context. User Ctrl-C during the 10s registry timeout would cancel the request via the cobra root context. `cmd.Context()` flows from `RunE` → `runClaudeCommand` → `ensureClaudeImageCurrent` → `service.EnsureLatest` → `resolver.LatestVersion(ctx)`. Cancellation works.
- **A19: Spinner stop on error path (self-invented).** REFUTED. `runWithCLIQuietSpinner` (presumed laslig helper, not read directly) takes three labels — "Building..." / "Built" / "Build failed" — and stops the spinner with the failure label when the closure returns an error. The `runManageUpdateClaude` failure path returns the wrapped error from inside the closure, so the spinner correctly transitions to its failure label `"Provider image update failed"`. Standard laslig pattern, matches Codex usage.
- **A20: `-race` coverage of new concurrent paths (self-invented).** REFUTED. The new `claudeVersionResolverFactory` package-level `var` is read once per `openImagesService` call. `stubClaudeVersionResolver` mutates the var inside a single-goroutine test setup phase before `cmd.Execute()` runs, then `t.Cleanup` restores. No concurrent writers. `-race` would not flag this.
- **A21: PLAN.md Unit 7.8 acceptance criteria coverage.** REFUTED — all 5 ACs satisfied per builder worklog: AC1 (no `BuildRequest`/`DefaultClaudeCLIVersion` in `ensureClaudeImageCurrent` non-env path) — verified at `claude.go:190`. AC2 (`EnsureLatest` with `AllowExistingOnCheckFail: true`) — verified. AC3 (`runManageUpdateClaude` calls `EnsureLatest`, output includes `checked at` field) — verified at `manage.go:1161,1176`. AC4 (`mage testPkg internal/cli` green) — confirmed by builder + by my live re-run: 154/154 green, 71.9% coverage. AC5 (`mage test` green) — builder claim 421/421.

### Findings

- **NOTE 1 (A6 / launch-path debug-log asymmetry):** `ensureClaudeImageCurrent` discards the `EnsureResult` with `_, err = service.EnsureLatest(...)`. The Codex counterpart `ensureCodexImageCurrent` (`codex.go:248-254`) captures the result and emits `LoggerFromContext(...).Debug("using existing codex image after latest-version check failed", ...)` on `EnsureActionUsingExistingImage`. This is a small operator-visibility gap on the Claude side: when npm is down and the launch falls back to the installed image, there's no debug breadcrumb. Not user-facing (only `--debug` logs), but breaks symmetry with Codex. Suggest mirroring the Codex pattern in a follow-on touch.

- **NOTE 2 (A14 / A14b — Claude-specific test coverage gap):** `runManageUpdateClaude`'s `EnsureActionUpToDate` heading branch and `ensureClaudeImageCurrent`'s `EnsureLatest` call have NO Claude-specific tests; Codex has both (`TestManageUpdateSecondRunReportsUpToDate` + `TestEnsureCodexImageCurrentAutoUpdatesWhenNoOverrideIsSet`). Production behavior is correct (same code path is tested via the Codex variants through the provider-agnostic `images.Service`), but Claude-side coverage is missing the symmetric assertions. Not a BLOCK; Unit 7.8 spec line 411 explicitly said "No test changes needed". The planner gap rolls forward.

- **NOTE 3 (A13 — `DefaultClaudeCLIVersion` is now dead in production):** Post-Unit-7.8, the constant has zero production-code references. It survives only as a test fixture for `TestDefaultClaudeCLIVersionIsNonEmpty` (a self-test) and as recipe-hash material for `TestServiceBuildRecipeHashMatchesProviderDockerfile`. The constant + its self-test could be deleted in a follow-on cleanup. Low priority. Note: the docstring still claims "Verified against Context7 /anthropics/claude-code at build time" but the value will go stale unless someone maintains it — the new resolver is the authoritative source now.

- **NOTE 4 (heading wording divergence Codex vs Claude — A14 related):** Codex's `runManageUpdateCodex` default heading is `"Provider image updated"` (`manage.go:1140`); Claude's `runManageUpdateClaude` default heading is `"Provider image built"` (`manage.go:1172`). Both switch to `"Provider image up to date"` on the UpToDate branch. The PLAN.md Unit 7.8 spec said `runManageUpdateClaude` should "match `runManageUpdateCodex` shape exactly — same fields, same field order." Builder preserved the prior Claude wording `"Provider image built"` rather than mirroring Codex's `"Provider image updated"`. Field order matches; only heading wording differs. User-visible. Minor — both are accurate (the action IS a rebuild when not up-to-date) but the asymmetry is visible across providers. Suggest unifying to `"Provider image updated"` for consistency.

- **CONCERN 1 (A3 — 24h cache promise NOT shipped):** PLAN.md line 20 (Round 5 always-latest design): "Wire latest-check into launch path with a 24h cache to avoid per-launch latency." Unit 7.8 ships the wiring but NOT the cache. Every `valv claude` launch performs an HTTPS round-trip to npm with a 10s timeout. On flaky/slow networks this adds visible latency to launch. Note: Codex has the same gap pre-DROP_7 so this is a planner-locked promise that the unit silently inherited. Routes to orchestrator: either ship the cache in a follow-on unit (deferred-promise discharge), or update PLAN.md to remove the promise.

### Counterexamples

None of the probed attacks produced a CONFIRMED counterexample that breaks the unit's claim. Concerns and notes flag asymmetry / deferred-promise / coverage-gap concerns but the production code matches the unit's acceptance criteria and `mage testPkg internal/cli` is green at 154/154 + 71.9% coverage (independently re-verified during this review).

### Unknowns

- **Live smoke test outcome:** whether `valv claude` in a fresh shell with no installed image and a working npm registry actually triggers a rebuild via the new launch-path EnsureLatest call. Build coverage exercises the unit-test path; the wired-end-to-end behavior is a dev smoke-test concern.
- **24h cache decision:** whether to ship the cache (planner-locked promise) in a follow-on unit or update PLAN.md to discharge the promise as YAGNI. Routes to orchestrator/dev.
- **Heading wording standardization:** whether to unify Claude's `"Provider image built"` default heading to Codex's `"Provider image updated"`. Cosmetic but cross-provider symmetric. Routes to orchestrator/dev.
- **`DefaultClaudeCLIVersion` deletion:** whether to delete the now-unused production constant + its self-test in a follow-on cleanup. Routes to orchestrator/dev.

## Hylla Feedback

- **Query:** `hylla_search_keyword` for `DefaultClaudeCLIVersion` (`tail_symbol`).
  - **Worked partially.** Returned the constant with a stale summary value (`"2.1.89"` — but the constant is actually `"2.1.143"` per current `service.go:37`). The Hylla ingest predates the Unit 7.4 / 7.7 / 7.8 commits, so the snapshot is stale by several commits. Expected per mid-drop reingest policy.
  - **Worked correctly via:** `hylla_refs_find` returned all 5 inbound references (3 production callers from the pre-Unit-7.8 state: `ensureClaudeImageCurrent`, `runManageUpdateClaude`, `TestRunManageUpdateClaudeBuildsImage`; 2 test self-references). Cross-referenced against my current-file `Read` of `claude.go` + `manage.go` to confirm that 2 of the 3 production callers no longer reference the constant post-Unit-7.8 — directly evidencing the A13 NOTE about residual dead constant.
  - **Suggestion:** None — stale data is expected per mid-drop reingest policy (Hylla reingest is drop-end only per `main/CLAUDE.md` § "Hylla Baseline"). The `hylla_refs_find` "give me the old call graph, I'll diff against current source" pattern is the right protocol mid-drop.
- All other code reads (claude.go, manage.go, operator_helpers.go, codex.go, output.go, manage_test.go, extended_test.go, service.go) used `Read` tool directly per mid-drop staleness protocol — these files were modified in the Unit 7.4 / 7.7 / 7.8 commits after the last Hylla snapshot. No fallback miss to log.

---

## Unit 7.9 — Round 1

**Date:** 2026-05-16
**Verdict:** pass (one CONCERN about dead integration test, several NOTEs about claims and UX; no findings block Phase 6 advancement for Unit 7.9 itself)

### Attack vectors probed

Each vector from the spawn prompt enumerated with verdict (CONFIRMED / REFUTED / EXHAUSTED / NOTE / CONCERN).

- **A1 — Cache file permission leak (0o644).** NOTE. `os.UserCacheDir()` on macOS returns `~/Library/Caches`, which is per-user private. The valv subdir is created with 0o755 (`cache.go:98`) and the file with 0o644 (`cache.go:101`). On a single-user Mac this is fine. On a shared multi-user host the cached version string + check timestamp is world-readable. Not a secret; not a real exposure. **NOTE only — document the assumption in a follow-on if Valv ever ships to multi-tenant hosts.**

- **A2 — Atomic write claim.** NOTE (false claim, benign in practice). `cache.go:101` uses naive `os.WriteFile(path, data, 0o644)` — `OpenFile(O_WRONLY|O_CREATE|O_TRUNC) + Write + Close`. POSIX does NOT guarantee atomicity for regular-file writes regardless of size (`write(2)`'s atomicity guarantee applies only to pipes/FIFOs ≤ `PIPE_BUF`). The builder's worklog at `BUILDER_WORKLOG.md:65` claims "POSIX WriteFile is atomic at the kernel level for this size" — this is **factually incorrect**. A concurrent reader catching the file mid-truncate sees zero bytes; `json.Unmarshal("")` returns an error. However, `readVersionCache` at `cache.go:49-58` silently swallows BOTH read errors AND parse errors → returns empty cache → falls through to resolver call → writes again. The implementation's defensive error-swallowing makes the consequences benign (one extra network call in a rare race window). **NOTE — false claim in worklog; runtime correctness is OK due to the swallow.** PLAN.md:442 even acknowledged the correct pattern ("for stricter safety use write-temp-then-rename via `os.CreateTemp` + `os.Rename`") but the builder chose simpler.

- **A3 — Cache key collision.** REFUTED. `providerKey` at `cache.go:42-44` is `string(p)` where `p` is `domain.Provider`. `domain.ProviderClaude` = `"claude"`, `domain.ProviderCodex` = `"codex"`. No unicode trickery, no case ambiguity, no prefix collision risk in current code.

- **A4 — Cache hit does not update `EnsureResult.LatestCheckedAt`.** CONCERN (UX). `service.go:411` sets `checkedAt := now.UTC()` UNCONDITIONALLY after the cache-vs-resolver decision. Both the cache-hit and cache-miss branches set `LatestCheckedAt = now.UTC()`. The user-facing `manage update` output at `manage.go:1144` (Codex) and `manage.go:1176` (Claude) renders `checked at <result.LatestCheckedAt>` formatted as RFC3339. **The user cannot tell** whether a fresh network check happened or whether the system used a 23h59m-old cached value — both display the current invocation's wall clock. For `valv manage update claude` this is mildly surprising: the user explicitly asked to "update" and the output reports "checked at <now>" even when no network call occurred. **CONCERN — the field's name (`LatestCheckedAt`) is misleading post-7.9.** Two reasonable fixes: (a) on cache hit, set `LatestCheckedAt = entry.CheckedAt` (the real check timestamp); or (b) add a separate field like `result.UsedCache bool` + render `"cached at"` vs `"checked at"`. Not a BLOCK but the planner should rule on the user-visible contract before drop close.

- **A5 — TTL boundary with future timestamp.** CONCERN (low risk). `cachedVersion` at `cache.go:75` uses `now.Sub(entry.CheckedAt) >= versionCacheTTL`. If `CheckedAt` is in the FUTURE (clock skew correction, manual file edit, NTP large adjustment), `now.Sub(future)` is negative, which is `< 24h`, so cache HIT. User is stuck with that cached version until `CheckedAt` < now+TTL OR the file is manually deleted. Real-world scenario: user's clock skews +1 day, app writes cache, clock corrects backward → cache valid for ~48h instead of 24h. Or laptop hibernate/wake with significant clock adjustment. **CONCERN — defensive code would clamp negative durations: `delta := now.Sub(entry.CheckedAt); if delta < 0 || delta >= versionCacheTTL { return ("", false) }`. Low priority — bound exposure is one extra day of stale version data.**

- **A6 — TempDir fallback path security/persistence.** EXHAUSTED. `defaultCachePath` at `cache.go:32-39` falls back to `os.TempDir()/valv-version-cache.json` only when `os.UserCacheDir()` errors. On macOS `os.UserCacheDir` returns `$HOME/Library/Caches` and only fails if `$HOME` is unset — virtually impossible in normal user sessions. The fallback path (`$TMPDIR` or `/tmp`) is world-readable and not persistent across reboots, but the cache content has no secrets and a 24h TTL anyway. Acceptable.

- **A7 — `TestEnsureLatestSurvivesCacheWriteError` implementation.** REFUTED. The test at `service_test.go:913-935` creates a DIRECTORY at the cache file path (`os.MkdirAll(cachePath, 0o755)`) then expects `os.WriteFile` to fail with "is a directory". This is a reliable, portable way to force `os.WriteFile` to error. The test then asserts `EnsureLatest` returns nil error and correct `LatestVersion`. Exercises the write-error swallow path at `service.go:406-408` (`s.debug("version cache write failed", "error", writeErr)`).

- **A8 — `DefaultClaudeCLIVersion` deletion ripple effects.** CONCERN (silently broken dead integration test). `git grep` for `DefaultClaudeCLIVersion` finds one surviving production-code reference: `internal/services/images/service_integration_test.go:127` — `result, err := svc.Build(context.Background(), BuildRequest{Version: DefaultClaudeCLIVersion})`. File is tagged `//go:build integration` (line 1). The builder's worklog at `BUILDER_WORKLOG.md:69` claims "confirmed 0 production callers" but the integration-tagged file was not checked. **Compile breakage IS real** — `go test -tags=integration ./internal/services/images` would fail with `undefined: DefaultClaudeCLIVersion`. **Mitigating fact:** `magefile.go:147` (mage `Integration` target) only runs `-tags=integration -count=1 ./internal/cli`, NOT `./internal/services/images`. No mage target compiles this file. The integration test is functionally orphaned and has been since at least the introduction of build-tag gating. **Degraded from BLOCK to CONCERN — no mage gate fails, no Phase 6 blockage from this finding alone. But:** the file is a dead Docker-backed integration test that no longer compiles. Either (a) delete it as dead code, (b) replace the literal `DefaultClaudeCLIVersion` with a local test constant (e.g. `const testClaudeCLIVersion = "2.1.143"` like `service_test.go:516`), or (c) add `./internal/services/images` to the `Integration` mage target if Docker-backed image-build coverage is desired. **Routes to orchestrator/dev for decision.**

- **A9 — Pre-existing tmpfs failure reproduction.** REPRODUCED. My `mage test` run produced exactly 427/428 with `TestRootDebugFlagIsNotPassedThroughToInteractiveCodexLaunch` failing on `no space left on device` during stage-shared-home. Identical failure mode to builder's report. This is environmental — `/private/var/folders/...` tmpfs is full. **Confirmed pre-existing, not introduced by Unit 7.9.** Phase 6 cannot exit until disk is cleared and `mage test` rerun shows 428/428. Orthogonal to 7.9 correctness.

- **A10 — Clock injection TZ semantics.** REFUTED. `writeVersionCache` at `cache.go:92` normalizes `now.UTC().Truncate(time.Second)` before encoding. `cachedVersion` at `cache.go:75` uses `now.Sub(entry.CheckedAt)` — `time.Sub` is independent of TZ on `time.Time` values (it operates on the underlying monotonic / wall instant). Even if a test injects a non-UTC clock, the written file is UTC-normalized. Confirmed safe.

- **A11 — Concurrent `valv claude` writers race.** NOTE (benign by design). Two simultaneous invocations both miss cache, both resolve, both `os.WriteFile`. Per A2, the writes are NOT atomic at kernel level for regular files, but the cache shape is "single small JSON file, last writer wins" and the version content is from the same npm endpoint within seconds — both writes are valid. Worst case: a third invocation reads mid-truncate, hits the `json.Unmarshal` error path in `readVersionCache`, falls through to resolver. **NOTE — concurrent-writer rare race produces at most one extra resolver call. Acceptable for a 24h-TTL cache.**

- **A12 — Cache deletion-by-orphan / dir creation.** REFUTED. `writeVersionCache` at `cache.go:98` calls `os.MkdirAll(filepath.Dir(path), 0o755)` BEFORE the `os.WriteFile`. If user `rm -rf`s `~/Library/Caches/valv/`, the next call recreates the directory cleanly. Confirmed safe.

- **A13 — JSON injection from npm response.** REFUTED. `claudeVersionResolver.LatestVersion` at `service.go:201-229` validates `versionPattern.MatchString(version)` (`\d+\.\d+\.\d+(?:[-+][0-9A-Za-z.-]+)?`) before returning. Cache write at `service.go:406` only happens AFTER `latestVersion` is set from the resolver — i.e., AFTER pattern validation. Malicious npm responses with control chars / JSON escapes are rejected before reaching the cache encoder. Plus `json.MarshalIndent` would escape any unusual byte content correctly anyway. Confirmed safe.

- **A14 — Cache file size growth.** EXHAUSTED. Each provider entry is ~80 bytes JSON (version + RFC3339 timestamp). With 10 providers the file is <1 KB. Linear growth, no concern at reasonable scale.

- **A15 — Empty `providers` field / missing key.** REFUTED. `cachedVersion` at `cache.go:64-79` handles both `c.Providers == nil` (returns `("", false)`) and missing key (`entry, ok := c.Providers[providerKey(p)]; if !ok return ("", false)`). Test `TestEnsureLatestPreservesOtherProviderEntries` at `service_test.go:846-877` exercises the "file exists with Codex entry, Claude entry absent" path. Confirmed safe.

- **A16 — Default `Clock` mutability.** REFUTED. `service.go:273-276`: `clock := options.Clock; if clock == nil { clock = time.Now }`. Captured by value in the `Service` struct's unexported `clock` field. No mutation paths. Each `Service` instance gets its own copy. Safe.

- **A17 (new) — `EnsureLatest` early-`now` capture vs `time.Now()` later in same path.** NOTE. `service.go:377` sets `now := s.clock()` once. `service.go:411` reuses `now.UTC()` for `checkedAt`. But `service.go:429` (the `up-to-date` branch's state-store upsert) calls `time.Now().UTC()` for `state.UpdatedAt`, AND `service.go:479` (the build-then-upsert path) ALSO calls `time.Now().UTC()` for `state.UpdatedAt`. The injected clock does NOT govern these two `UpdatedAt` writes. Tests that inject a fixed clock to assert deterministic state-store timestamps could be flaky if they assert on `UpdatedAt`. **NOTE — not a current bug (existing tests don't assert on `UpdatedAt`), but the clock-injection contract has a latent gap if anyone tries to add such a test later. Suggest `state.UpdatedAt = now.UTC()` (replace `time.Now()` with `s.clock()`) to fully honor clock injection.**

- **A18 (new) — Cache invariant on resolver error path.** REFUTED. When `s.resolver.LatestVersion` errors at `service.go:381`, the code goes through the `AllowExistingOnCheckFail` branch (uses `state.LatestCheckedAt`) OR returns the error wrapped (`service.go:404`). In NEITHER case is `writeVersionCache` called with an empty/error version string. Good — the cache only ever contains successfully-resolved versions.

### Findings

- **CONCERN: A4 — `LatestCheckedAt` masks cache hits.** On a cache hit, the CLI output renders `checked at <now>` regardless. User cannot distinguish fresh check from cached. Mild UX surprise. Routes to orchestrator/dev for contract decision.
- **CONCERN: A5 — Future-timestamp permanence.** Cache hits forever if `CheckedAt` is in the future. Add `delta < 0` guard in `cachedVersion`. Low priority.
- **CONCERN: A8 — Dead integration test broken by deletion.** `service_integration_test.go:127` references the deleted `DefaultClaudeCLIVersion`. No mage target compiles it, so no gate fails — but file is now dead-on-arrival. Dev should pick: delete, fix, or wire into mage.
- **NOTE: A2 — False atomic-write claim in `BUILDER_WORKLOG.md:65`.** Implementation is correct due to error-swallowing read path, but the technical claim is wrong. Update worklog or upgrade to temp+rename if a stricter contract is desired.
- **NOTE: A17 — `time.Now()` survives in two `UpdatedAt` writes inside `EnsureLatest`.** Latent clock-injection gap; no current test depends on it. Suggest one-line fix: replace `time.Now()` at `service.go:429` and `service.go:479` with `s.clock()`.
- **NOTE: A1 — 0o644 file mode.** Cache content readable by other users on shared hosts. Not a current concern.
- **NOTE: A11 — Concurrent-writer race is benign.** Worst case: one extra resolver call. Document if it ever becomes a load concern.

### Pre-existing failure status

- Builder reported 427/428; my run confirms 427/428 with the **same** failing test `TestRootDebugFlagIsNotPassedThroughToInteractiveCodexLaunch` failing on `no space left on device` during stage-shared-home setup at `/private/var/folders/...`. **Environmental — disk full on the macOS tmpfs.** Reproduced independently. Not introduced by Unit 7.9. **Phase 6 advancement requires clearing disk and rerunning `mage test` until 428/428 — orthogonal to 7.9 correctness.**

### Verdict

`pass` — no BLOCK findings. The single A8 finding was downgraded to CONCERN after verifying that no mage target compiles `service_integration_test.go` (only `./internal/cli` runs under `-tags=integration`). The integration test is dead in the mage workflow, so its compile break does not gate the drop.

All A4 / A5 / A8 CONCERNs and A2 / A17 / A1 / A11 NOTEs should be surfaced to orchestrator/dev for routing — they are not regression-causing but document real semantic / claim / hygiene gaps.

### Unknowns

- **Dev's preference on A8 dead integration test.** Delete vs fix vs wire-to-mage. Routes to orchestrator.
- **Dev's contract intent on A4.** Should `LatestCheckedAt` reflect the cache entry's check time (true semantics) or the current invocation time (current behavior)? Routes to orchestrator.
- **Future-timestamp guard on A5.** Add the negative-delta check, or accept the rare-clock-skew exposure? Routes to orchestrator.
- **`mage test` 428/428 rerun.** Disk-space failure pre-exists 7.9. Phase 6 cannot exit until cleared. Routes to dev for environment cleanup.

## Hylla Feedback

- **Hylla daemon unreachable** (`dial tcp 127.0.0.1:9080: connect: connection refused`) during this review. Fell back to `git grep` + direct `Read` for all symbol / reference lookups. Two queries attempted: `hylla_search_keyword` for `DefaultClaudeCLIVersion` and `CachePath` — both failed with the connection-refused error.
- **Worked via:** `git grep -n "DefaultClaudeCLIVersion" HEAD` returned all 1 surviving production-code reference (`service_integration_test.go:127`) plus drop-doc references. `git grep -n "version-cache"` confirmed cache path references. Direct `Read` on `service.go`, `cache.go`, `service_test.go`, `service_integration_test.go`, `magefile.go`.
- **Suggestion:** None for Hylla itself — daemon down is an infra issue, not a tool defect. The `git grep` + `Read` fallback is fully adequate for this scope; a re-ingest after the drop closes will resolve the staleness for future readers.

---

## Unit 7.10 — Round 1

**Date:** 2026-05-16
**Verdict:** pass (with one CONCERN routed to follow-up and two NOTEs)

### Attack vectors probed

Each numbered vector from the spawn prompt enumerated. CONFIRMED = counterexample produced. REFUTED = attack tried, evidence rules it out. NOTE/CONCERN = real finding but not BLOCK.

1. **A1 — Orphan "Provider image built" assertions.** REFUTED. `Read` on `manage.go:1100-1180` confirms both `runManageUpdateCodex` (line 1140) and `runManageUpdateClaude` (line 1172) now read `heading := "Provider image updated"`. `Read` on `TestCodexInteractiveMCPGolden.golden` (the only golden fixture in `internal/cli/testdata/`) confirms it does not contain the old heading. `mage test` GREEN proves no orphan test assertions remain.
2. **A2 — Shell-script consumer regression from heading change.** NOTE. No documented contract pinning "Provider image built" — output-format design treats human heading as informational, not API. `--format=json` output uses structured fields, not the heading. Acceptable for v0.1.0.
3. **A3 — Debug log `.String()` / key alignment with Codex.** REFUTED. Byte-for-byte symmetric: Codex `codex.go:253` reads `LoggerFromContext(cmd.Context()).Debug("using existing codex image after latest-version check failed", "image", result.Image.String(), "version", result.Version)`. Claude `claude.go:195` reads `LoggerFromContext(cmd.Context()).Debug("using existing claude image after latest-version check failed", "image", result.Image.String(), "version", result.Version)`. Same keys, same `.String()` call.
4. **A4 — `EnsureActionUsingExistingImage` constant existence + correctness.** REFUTED. `Read` on `service.go:111-117` confirms the constant exists; `service.go:397` confirms it is returned ONLY when `request.AllowExistingOnCheckFail==true` AND resolver fails AND image is available. Both `claude.go:190` and `codex.go:248` pass `AllowExistingOnCheckFail: true`. Branch is reachable; assertion correct.
5. **A5 — `LoggerFromContext` nil-context behavior.** NOTE (pre-existing). `Read` on `root.go:142-148`: `LoggerFromContext` returns nil when `ctx==nil` OR when the context has no logger value (type assertion discards `ok`). Calling `(*log.Logger)(nil).Debug(...)` would panic if the receiver is nil-checked, OR no-op if log.Logger's Debug method handles nil-receiver safely. Cannot definitively confirm without running the panic case. **Not exercised in production**: `PersistentPreRunE` on the root command (`root.go:70-98`) installs the logger before any `RunE` runs. Production calls always have the logger present. **Not exercised in the new test**: `TestEnsureClaudeImageCurrentAutoUpdatesWhenNoOverrideIsSet` uses a bare cobra.Command with `context.Background()` (no logger), but stubs the resolver to succeed → action is `EnsureActionUpdated`, not `EnsureActionUsingExistingImage`. Latent risk is real, BOTH for Codex and Claude — pre-existing from Codex's earlier parity work, not introduced by Unit 7.10. Routes to a future hardening drop (defensive nil-check in `LoggerFromContext` or in the call site).
6. **A6 — `CLAUDE_VERSION` build-arg source.** REFUTED. `Read` on `service.go:542-550`: `providerVersionBuildArg()` returns `"CLAUDE_VERSION"` for `domain.ProviderClaude`. Test asserts `--build-arg CLAUDE_VERSION=2.2.0` (`extended_test.go:899`). The `2.2.0` comes from the resolver stub (`extended_test.go:864`), not from the deleted `DefaultClaudeCLIVersion` constant. No drift.
7. **A7 — `fakeClaudeRecipeHash()` helper redundancy.** REFUTED. `Read` on `extended_test.go:853-861`: the helper computes SHA256 of `imagesservice.DefaultClaudeDockerfile()` — Claude-specific, parallel to the existing `fakeCodexRecipeHash()`. Necessary for `TestManageUpdateClaudeSecondRunReportsUpToDate` to seed `VALV_DOCKER_IMAGE_INSPECT_OUTPUT` so the recipe-hash check passes on the second invocation.
8. **A8 — Test cache-file isolation.** **CONFIRMED COUNTEREXAMPLE — downgraded to CONCERN.** `Read` on `operator_helpers.go:70-114`: `openImagesService` does NOT set `Options.CachePath` when constructing the images service. `Read` on `cache.go:32-39`: empty `CachePath` falls back to `defaultCachePath()` which calls `os.UserCacheDir()` returning `$HOME/Library/Caches/valv/version-cache.json` on macOS. `testCodexPaths` (codex_test.go:453-474) does NOT call `t.Setenv("HOME", ...)` or `t.Setenv("XDG_CACHE_HOME", ...)`. Direct verification: after running `mage test`, the dev's real `~/Library/Caches/valv/version-cache.json` was polluted with `{"providers":{"claude":{"version":"2.2.0","checked_at":"2026-05-16T07:22:51Z"},"codex":{"version":"0.117.0","checked_at":"2026-05-16T07:21:35Z"}}}`. **Production impact**: next real `valv manage update claude` will cache-hit at `2.2.0` for 24h and skip resolving the actual latest version. **Test correctness impact**: tests pass green (resolver stub overrides the cache anyway for cache-miss paths), so no test-failure regression. **Pre-existing**: bug is rooted in Unit 7.9's cache wiring + `openImagesService` not threading `CachePath` through. Unit 7.10's new `TestManageUpdateClaudeSecondRunReportsUpToDate` is the first test to write a Claude entry to the cache. **Not a BLOCK for Unit 7.10's stated scope (cosmetic parity)**; routes as follow-up: set `Options.CachePath = filepath.Join(paths.CachesDir, "version-cache.json")` in `openImagesService`.
9. **A9 — Second-run up-to-date semantics.** REFUTED. Test seeds `VALV_DOCKER_IMAGE_INSPECT_OUTPUT=fakeClaudeRecipeHash()` so the recipe-hash check at `service.go:418-424` matches on second run. State store records `InstalledVersion=2.2.0` after first run; second run finds `InstalledVersion==latestVersion` and `recipeMatches==true`, returning `EnsureActionUpToDate` (`service.go:441`). Heading becomes "Provider image up to date" (`manage.go:1174`). Assertion `strings.Contains(output, "Provider image up to date")` passes. Build count assertion (`strings.Count(logContent, "buildx build --load") == 1`) holds because only the first run builds. Logically sound.
10. **A10 — Codex baseline log invocation match.** REFUTED. `Read` on `codex.go:248-254`: builder mirrored exactly. No additional conditional, no resolver-error log field, no asymmetry.
11. **A11 — Mage test reproducibility.** REFUTED. Re-ran `mage test` from `main/` — **428/428 tests GREEN across 20 packages**, all coverage thresholds met (`internal/services/images=78.9%`, `internal/cli=72.4%`). No flaky disk-space failures; the 427/428 issue cited for Unit 7.9 is not present in HEAD.
12. **A12 — Golden fixtures with old heading.** REFUTED. `Read` on `internal/cli/testdata/TestCodexInteractiveMCPGolden.golden` — only golden fixture in `internal/cli`. Does not contain "Provider image built". No `mage goldenUpdate` regeneration needed.
13. **A13 — Symmetric up-to-date headings.** REFUTED. `Read` on `manage.go:1140-1144` (Codex) and `manage.go:1172-1176` (Claude): both `runManageUpdate*` paths use identical heading logic: default `"Provider image updated"`, fallback `"Provider image up to date"` on `EnsureActionUpToDate`.
14. **A14 — AC1 verification via inspection vs test.** NOTE. AC1 (PLAN.md:498) reads `"Verified by test (capture debug logger output or inspect via mock)"` — disjunction permits inspection. Builder used byte-for-byte Codex mirror as inspection evidence. The new debug-log branch (`EnsureActionUsingExistingImage`) is NOT exercised by either new test: `TestManageUpdateClaudeSecondRunReportsUpToDate` stubs a successful resolver (never hits the fallback); `TestEnsureClaudeImageCurrentAutoUpdatesWhenNoOverrideIsSet` likewise stubs success. The branch is reachable in production only when the resolver fails AND an existing image is available — not currently tested. AC1 technically satisfied via inspection; future hardening drop should add a test that simulates resolver failure + pre-built image to verify both the `EnsureActionUsingExistingImage` return path AND the debug-log emission (closing the A5 nil-logger latent gap simultaneously).

### Findings

- **CONCERN A8: Test cache pollution + production cache poisoning.** `openImagesService` (`operator_helpers.go:70-114`) does not thread `paths.CachesDir` into `imagesservice.Options.CachePath`. Tests + production both fall back to `os.UserCacheDir()`. Confirmed dev's `~/Library/Caches/valv/version-cache.json` was overwritten with test values (`claude=2.2.0`, `codex=0.117.0`) during mage test execution. Pre-existing from Unit 7.9; Unit 7.10's new Claude test is the first to write a Claude entry. Fix: add `CachePath: filepath.Join(paths.CachesDir, "version-cache.json")` to `Options` in `openImagesService`. **Not a Unit 7.10 BLOCK** — outside cosmetic-parity scope.
- **NOTE A5: Latent `LoggerFromContext` nil-receiver call.** `LoggerFromContext(...).Debug(...)` at `claude.go:195` and `codex.go:253` will panic (or no-op, depending on `log.Logger.Debug` nil-receiver behavior — unverified) if the call site has no logger in context AND `EnsureActionUsingExistingImage` is returned. Production paths always install the logger via `PersistentPreRunE`; tests don't exercise the branch. Pre-existing — applies to BOTH providers, not introduced by Unit 7.10. Fix candidate: nil-guard at the call site or in `LoggerFromContext` return.
- **NOTE A14: Debug-log branch untested.** New `EnsureActionUsingExistingImage` debug log at `claude.go:195` has no test coverage. AC1 satisfied via inspection per AC wording, but a future test should simulate resolver failure + pre-built image.
- **NOTE A2: Shell-consumer wording regression risk.** "Provider image built" → "Provider image updated" could break users grepping for the old string. Not a documented contract; acceptable for v0.1.0.

### Mage test result

`mage test` from `main/`: **428/428 GREEN** across 20 packages. All coverage thresholds met (`internal/cli=72.4%`, `internal/services/images=78.9%`). Race detector clean. No flaky failures.

### Verdict

`pass` — no BLOCK findings. Unit 7.10's three sub-fixes (debug log + tests + heading wording) land cleanly with byte-for-byte Codex parity. A8 is a pre-existing infra concern unrelated to Unit 7.10's cosmetic scope. A5 and A14 are pre-existing parity-bug routes for future hardening.

### Unknowns

- **A8 cache-isolation fix priority.** Should this be addressed before any 7.x close or routed to a follow-up drop? Routes to orchestrator/dev.
- **A5 nil-logger panic vs no-op behavior.** Need a direct test (`var l *log.Logger; l.Debug(...)`) to confirm whether `charmbracelet/log.Logger`'s Debug method nil-checks the receiver. Routes to orchestrator.
- **A14 future-test plan.** Add an explicit `EnsureActionUsingExistingImage` test that covers debug-log emission. Routes to follow-up drop.

## Hylla Feedback

- **Hylla daemon unreachable** (`dial tcp 127.0.0.1:9080: connect: connection refused`) again during this review. Fell back to direct `Read` for all symbol / reference lookups. One `hylla_search_keyword` query attempted (for `LoggerFromContext`) — failed with the connection-refused error.
- **Worked via:** direct `Read` of `internal/cli/root.go`, `internal/cli/claude.go`, `internal/cli/codex.go`, `internal/cli/manage.go`, `internal/cli/operator_helpers.go`, `internal/cli/extended_test.go`, `internal/cli/manage_test.go`, `internal/cli/codex_test.go`, `internal/services/images/service.go`, `internal/services/images/cache.go`. `git diff HEAD~1 HEAD` for the unit's change set.
- **Suggestion:** None — daemon down is infra; fallback `Read` covers Go-only review scope adequately when files are known.

---

## Unit 7.9 — Round 2

**Date:** 2026-05-16
**Verdict:** pass

### Attack vectors probed

Each attack vector from the spawn prompt enumerated with verdict (CONFIRMED / REFUTED / EXHAUSTED / NOTE / OBSERVATION).

- **A1 — `cachedVersion` signature change ripple.** REFUTED. `git grep cachedVersion -- '*.go'` returns exactly one production caller at `internal/services/images/service.go:379`: `latestVersion, cachedCheckedAt, fromCache := cachedVersion(cacheData, s.provider, now)` — three-receiver destructure matching the new `(string, time.Time, bool)` signature. No second caller exists in production OR tests (cache_test.go does not exist; only `service_test.go` exercises `EnsureLatest` end-to-end, never calling `cachedVersion` directly). Zero compile-break surface from the signature change.

- **A2 — `LatestCheckedAt` cache-hit UTC consistency.** OBSERVATION (non-blocking). On a cache hit, `service.go:415` sets `checkedAt = cachedCheckedAt` — the value returned from `cachedVersion`, which is the JSON-deserialized `entry.CheckedAt` from the on-disk file. The cache WRITE path at `cache.go:94` always normalizes via `now.UTC().Truncate(time.Second)`, so a Valv-written cache file always contains UTC timestamps. A user manually editing the file to insert e.g. `"checked_at":"2025-01-01T00:00:00-05:00"` would round-trip a `time.Time` with a fixed-offset zone; downstream `result.LatestCheckedAt.Format(time.RFC3339)` at `manage.go:1144` / `manage.go:1176` would render it WITH that offset (`2025-01-01T00:00:00-05:00`) — a valid RFC3339 string representing the same instant, but visually non-UTC. **Not a CONFIRMED counterexample** because (a) the production writer always writes UTC, (b) RFC3339-with-offset is still spec-correct, (c) `time.Time.Equal` (used in the test assertion at line 973) correctly compares the same wall instant regardless of zone. Documented as OBSERVATION: a defensive `.UTC()` normalization on `cachedCheckedAt` at `service.go:415` would harden against hand-edited cache files. Low priority.

- **A3 — Future-timestamp guard boundary (`delta == 0`).** REFUTED. `cache.go:77` reads `if delta < 0 || delta >= versionCacheTTL`. When `delta == 0` (clock hasn't ticked since the entry was written, e.g. an injected fixed clock equals `entry.CheckedAt`), the predicate is `false || false` → falls through to `return entry.Version, entry.CheckedAt, true`. Treats it as fresh, which is correct: a zero-delta cache is the freshest possible. The fix's guard is `<` not `<=`, so the equality boundary is not erroneously rejected.

- **A4 — Test fixture clock injection race.** REFUTED. Both new tests (`TestEnsureLatestReportsCachedCheckedAtOnCacheHit` at `service_test.go:940-976`; `TestEnsureLatestRejectsCacheWithFutureTimestamp` at `service_test.go:982-1023`) construct a fresh `clockNow` closure via `func() time.Time { return clockNow }`. Each test gets its own `cacheDir := t.TempDir()` and its own `svc` via `newCacheTestService`. NEITHER test calls `t.Parallel()` (verified by direct read of lines 940 and 982 — only `t.Cleanup(...)` on findDockerBinary). The injected clock is per-Service, immutable for the test lifetime, never shared. No race surface.

- **A5 — FIX 3 clock-injection completeness.** REFUTED. `git grep "time.Now" -- internal/services/images/service.go` returns exactly 2 hits, both in the `Options` defaulting at lines 69 (doc comment) and 275 (`clock = time.Now` — the default-when-nil fallback). NO `time.Now()` call survives inside `EnsureLatest` body (lines 364-510). The two original `state.UpdatedAt = time.Now().UTC()` writes at the up-to-date branch and the build-then-upsert branch are now `s.clock().UTC()` at `service.go:436` and `:486` respectively. Verified by diff `git show 6b4ea4f -- internal/services/images/service.go`. The injected clock fully governs all time writes in `EnsureLatest`.

- **A6 — Integration test value (FYI for orchestrator).** NOTE. `service_integration_test.go` now compiles cleanly under `-tags=integration` (verified by direct read — `const testClaudeCLIVersion = "2.1.143"` declared at line 20, used at line 132). But `magefile.go::Integration` runs only `-tags=integration -count=1 ./internal/cli` — it does NOT include `./internal/services/images`. So the file still has no mage target that exercises it. Same status as before R2 (file is dead-on-arrival in the mage workflow). Fix is correct in scope; orchestrator/dev decision whether to wire the file in or delete it is unchanged from R1 finding A8. NOTE — not a R2 regression.

- **A7 — `TestEnsureLatestReportsCachedCheckedAtOnCacheHit` assertion strength.** REFUTED. Line 973 uses `result.LatestCheckedAt.Equal(cachedAt)` — `time.Time.Equal` is the correct comparator. Per Go stdlib (`time.Time.Equal` documentation): "Equal reports whether t and u represent the same time instant. Two times can be equal even if they are in different locations. For example, 6:00 +0200 and 4:00 UTC are Equal. See the documentation on the Time type for the pitfalls of using == with Time values; most code should use Equal instead." Both `result.LatestCheckedAt` (from the cache-hit branch) and `cachedAt` (the literal constructed in the test) are in `time.UTC`, but the test would survive a defensive `.UTC()` change to the production code path because `Equal` compares instants, not struct fields. No `==` comparator anywhere in the new tests (verified by reading lines 940-1023). Correct.

- **A8 — `TestEnsureLatestRejectsCacheWithFutureTimestamp` future-trigger correctness.** REFUTED. Line 991 sets `futureTime := time.Date(2030, 1, 1, ...)`; line 992 sets `clockNow := time.Date(2026, 5, 16, ...)`. `delta = clockNow.Sub(futureTime) = -4 years approx`, which is well negative, triggering the `delta < 0` branch in `cachedVersion`. Resolver IS called (line 1008 asserts `counter.called == 1`); cache file is updated to `2.1.200` (line 1018 asserts `entry.Version == "2.1.200"`). The future-vs-clock gap is 4 years — not "within minutes". Trigger is unambiguous.

- **A9 — `cachedVersion` zero-value `time.Time` on cache miss.** REFUTED. `cache.go:67,71,74,78` all return `time.Time{}` (zero value) when fromCache is false. The single caller at `service.go:379` destructures into `cachedCheckedAt`. `cachedCheckedAt` is then read ONLY at line 415 inside `if fromCache { checkedAt = cachedCheckedAt }`. The variable is never read when `fromCache == false`. No latent zero-value bug.

- **A10 — Atomic-write JUSTIFICATION fix vs IMPLEMENTATION upgrade.** NOTE (builder chose option a — doc-only). Builder Worklog line 119 documents the choice: "The implementation remains acceptable because `readVersionCache` silently swallows both read and parse errors, so a torn read falls through to the resolver (one extra network call at worst). The correct justification is: 'self-healing via parse-error swallowing makes the non-atomic write acceptable for a 24h-TTL cache at v0.1.0.' The temp+rename pattern (`os.CreateTemp` + `os.Rename`) would be strictly safer but adds complexity for a file whose corruption is gracefully handled." The race (concurrent reader catching a mid-truncate state) is therefore still POSSIBLE but BENIGN — readVersionCache returns empty cache on `json.Unmarshal` error, fromCache is false, resolver runs, cache rewritten. Builder's reasoning is technically correct: the production cost is at-worst one extra network call per race. The previously-incorrect "POSIX kernel-level atomic" claim is now correctly described. Acceptable trade-off for v0.1.0. NOTE — orchestrator/dev may revisit if multi-process Valv invocations become common.

- **A11 — `mage test` count discrepancy.** RESOLVED with ground truth. Builder 7.9 R2 worklog reported 430/430. Builder 7.10 R2 worklog reported 428/428. My independent `mage test` run from `main/` reports **431/431 GREEN across 20 packages**. The +1 vs builder 7.9 R2's 430 most likely reflects the test added in 7.10 R2 (`TestOpenImagesServiceWritesCacheToCachesDir`) being committed AFTER builder 7.9 R2's mage run — the two R2 commits landed sequentially (`6b4ea4f` 7.9 R2 then `6b55d59` 7.10 R2). With BOTH R2 commits on HEAD, ground truth is 431/431. The +3 vs builder 7.10 R2's 428 is harder to explain — that count came from a tmpfs-affected machine. Both R2 changes are now committed and `mage test` is fully green. No flakes, no race detector hits.

  Coverage summary (relevant packages):
  - `internal/services/images` — 79.2% (was 78.9% in R1; +2 tests; +0.3pp consistent with new branch coverage)
  - `internal/cli` — 72.5%
  - All 20 packages ≥60% gate, including the AGENTS.md § 11 70%-per-package target met on every package that matters for Unit 7.9 scope.

### Additional Adversarial Probes (beyond the spawn-prompt's A1-A11)

- **A12 (new) — `LatestCheckedAt` field on `AllowExistingOnCheckFail` path.** REFUTED. `service.go:400` (inside `if !fromCache` → resolver-error → AllowExistingOnCheckFail branch) sets `LatestCheckedAt: state.LatestCheckedAt`. `state` here is the existing state-store entry from a previous successful run, NOT from the cache. The cache hit/miss semantics don't apply on this path because we're already in the `!fromCache` branch (resolver was called and failed). Correct — uses the durable state-store value, which was UTC-normalized at write time.

- **A13 (new) — `state.LatestCheckedAt` write source consistency in the build-then-upsert path.** REFUTED. `service.go:482` writes `LatestCheckedAt: checkedAt` into the new `state` after a build. `checkedAt` was computed at line 413-418 conditional on `fromCache`. On a cache-hit + build (cache says version is current, but image is missing or recipe drifted), `checkedAt = cachedCheckedAt` — the cache's check timestamp gets persisted into the state store. This is correct semantically: the state store records when the version was last verified against npm, regardless of whether the image build was forced for other reasons. On a cache-miss + build (resolver was called), `checkedAt = now.UTC()` — the current invocation's check timestamp. Both branches are coherent. The state store always receives a meaningful timestamp.

- **A14 (new) — `state.UpdatedAt` vs `state.LatestCheckedAt` separation under cache-hit.** REFUTED. `state.UpdatedAt = s.clock().UTC()` at lines 436 and 486 reflects when the state-store row was last touched (always "now"), distinct from `state.LatestCheckedAt = checkedAt` (when version was last verified against npm). On a cache-hit-up-to-date case, `UpdatedAt = now` but `LatestCheckedAt = cachedCheckedAt` (potentially many hours ago). These two fields semantically diverge by design — that's the entire point of caching. Schema is coherent.

- **A15 (new) — `Equal` vs `==` time comparison in tests.** REFUTED across all new and modified tests. Line 973 (`TestEnsureLatestReportsCachedCheckedAtOnCacheHit`): `result.LatestCheckedAt.Equal(cachedAt)`. Line 1020 (`TestEnsureLatestRejectsCacheWithFutureTimestamp`): `entry.CheckedAt.After(clockNow.Add(time.Second))` — uses `After`, not `==`. No `==` time comparisons in new test code. Monotonic-clock pitfall avoided.

- **A16 (new) — Truncation-to-second precision and `Equal` interaction.** REFUTED. The writer truncates to second precision at `cache.go:94` (`now.UTC().Truncate(time.Second)`). The new test fixture at `service_test.go:949` uses `time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)` — already second-aligned with zero nanoseconds. So `cachedAt` written via `writeCacheFile`, round-tripped through JSON, and re-read as `entry.CheckedAt` yields a value `Equal` to the original `cachedAt`. The truncation is idempotent on already-second-aligned values. Test assertion is correct.

- **A17 (new) — Cache file unwritten when resolver errors.** REFUTED. `service.go:404` returns the wrapped error without calling `writeVersionCache`. The cache write at `service.go:406` is inside the `if !fromCache` branch, after the resolver succeeds — never reached on resolver error. Cache will only ever contain successfully-resolved versions. Verified by direct read of lines 380-409.

- **A18 (new) — Test counter `resolverCallCounter` assertions.** REFUTED. `service_test.go:958` (cache-hit test) asserts `counter.called == 0` — proves resolver was NOT invoked, confirming cache-hit short-circuit. `service_test.go:1008` (future-timestamp test) asserts `counter.called == 1` — proves resolver was invoked exactly once, confirming the future-ts entry was rejected as a miss. Both assertions are positively-specified (exact count, not `>= N`), so a future regression that double-calls the resolver or skips it would fail the test.

### Findings

- **A2 OBSERVATION:** `cachedCheckedAt` on cache hit comes verbatim from the JSON-deserialized entry. The Valv-written cache file is always UTC, but a hand-edited cache file with non-UTC timezone offsets would render with that offset in the `manage update` output's RFC3339 string. Visually non-UTC but instant-correct. Defensive fix: `checkedAt = cachedCheckedAt.UTC()` at `service.go:415`. Low priority; documented for orchestrator/dev.
- **A6 NOTE:** Integration test now compiles but is still not in any mage target. Same as R1 finding A8 — orchestrator/dev decision (delete vs wire to mage) is unchanged from R1.
- **A10 NOTE:** Builder chose option (a) — doc-only correction in worklog. The race is now correctly documented but not defended against by temp+rename. Acceptable for v0.1.0 because `readVersionCache` swallows torn-read errors. Orchestrator may revisit later.

No CONFIRMED counterexamples.
No BLOCK findings.
No regression-causing CONCERNs.

### R1 finding closure

- **R1 Proof Finding 1.1 (integration test compile breakage):** **CLOSED.** `service_integration_test.go:132` now uses `testClaudeCLIVersion` (declared at line 20). Source compiles cleanly under `-tags=integration`.
- **R1 Proof Finding 1.2 (`LatestCheckedAt` cache-hit semantic):** **CLOSED.** `service.go:413-418` branches on `fromCache`; cache hits return `cachedCheckedAt` not `now`. `TestEnsureLatestReportsCachedCheckedAtOnCacheHit` pins the contract.
- **R1 Falsification A2 (false atomic-write claim in worklog):** **CLOSED.** R2 worklog explicitly corrects the claim (BUILDER_WORKLOG.md:119). Implementation remains non-atomic but justification is now accurate.
- **R1 Falsification A4 (cache-hit `LatestCheckedAt` masks cache):** **CLOSED.** Same fix as Proof 1.2 — cache hits now report the cached timestamp.
- **R1 Falsification A5 (future-timestamp permanence):** **CLOSED.** `cache.go:77` adds `delta < 0` guard; `TestEnsureLatestRejectsCacheWithFutureTimestamp` pins the rejection behavior.
- **R1 Falsification A17 (clock-injection completeness):** **CLOSED.** Two surviving `time.Now()` calls in `EnsureLatest` replaced with `s.clock()` (lines 436, 486). Verified by `git grep "time.Now"`: only the default-fallback at line 275 remains, outside `EnsureLatest`.

### Mage test count (with both R2 sets committed)

**431/431 GREEN across 20 packages.** Independent re-run from `main/` confirms full suite passes. Coverage threshold met for all packages (minimum 60%; `internal/services/images=79.2%`, `internal/cli=72.5%`). No flakes, no race detector hits.

Builder 7.9 R2 reported 430/430 and 7.10 R2 reported 428/428 — both predate one of the R2 commits being on disk at the time their respective mage runs landed. Ground truth with both R2 commits on HEAD is 431/431.

### Unknowns

- **A2 OBSERVATION (manual cache-file timezone edit):** Acceptable for v0.1.0; production writer always writes UTC. Defensive `.UTC()` at the cache-hit branch would harden against hand-edited files. Routes to orchestrator for prioritization.
- **A6 NOTE (integration test wiring):** Same status as R1 A8 — file compiles but no mage target runs it. Orchestrator/dev decision (delete vs wire) is unchanged from R1.
- **A10 NOTE (atomic-write deferred):** Worklog now correctly documents the non-atomic write. Race is benign due to error-swallowing in `readVersionCache`. Routes to orchestrator if multi-process Valv invocations become common.

### Verdict

`pass` — no CONFIRMED counterexamples. All five R2 fixes (integration test reference, `LatestCheckedAt` cache-hit semantic, clock-injection completeness, future-timestamp guard, worklog correction) land cleanly with file:line evidence and behavior-pinned tests. `cachedVersion` signature change has exactly one production caller, correctly updated. New tests use `time.Time.Equal` (no monotonic-clock pitfall), positively-specified resolver-call counts, and unambiguous future-vs-clock gaps. `mage test` is 431/431 GREEN with both R2 commits on disk.

The three remaining items (A2 OBSERVATION on manual cache-file edits, A6 NOTE on integration test wiring, A10 NOTE on deferred atomic-write upgrade) are documented for orchestrator routing but do not block Phase 6 advancement for Unit 7.9 R2.

## Hylla Feedback

- **Hylla MCP not consulted for this round.** All reviewed code was committed in `6b4ea4f` (Unit 7.9 R2), which post-dates the most recent Hylla ingest. Per CLAUDE.md § "Code Understanding Rules" item 2 (changed-since-ingest files use `git diff` / `Read`), I went directly to `git show 6b4ea4f -- <file>`, `Read` on current source, and `git grep` for symbol audits. No Hylla queries attempted; no fallback miss to log. Drop-end reingest will refresh the snapshot for future readers.

---

## Unit 7.10 — Round 2

**Date:** 2026-05-16
**Verdict:** pass

### Attack vectors probed

Each numbered vector from the spawn prompt enumerated. CONFIRMED = counterexample produced. REFUTED = attack tried, evidence rules it out. NOTE/CONCERN = real finding but not BLOCK.

- **A1 — Other paths bypassing `openImagesService`.** REFUTED. Every production-code construction of the images service goes through `openImagesService(cmd, paths, provider)`. Confirmed by direct `Read` of the four files that import `imagesservice`:
  - `internal/cli/operator_helpers.go:70-115` — sole site that constructs `imagesservice.Options{}` and calls `imagesservice.New(options)`. FIX 1 lives here at line 84.
  - `internal/cli/manage.go:1116,1148` — `runManageUpdateCodex` and `runManageUpdateClaude` both call `openImagesService(cmd, paths, provider)`; neither constructs `imagesservice.Options{}` directly.
  - `internal/cli/codex.go:243` — `ensureCodexImageCurrent` calls `openImagesService(cmd, paths, domain.ProviderCodex)`.
  - `internal/cli/claude.go:185` — `ensureClaudeImageCurrent` calls `openImagesService(cmd, paths, domain.ProviderClaude)`.
  - `cmd/valv/main.go` — does not reference `imagesservice` at all (verified by full `Read`).
  - `internal/cli/claude_auth.go` — does not reference `imagesservice` (verified by full `Read`; only `dockeradapter`, not the higher-level images service).
  No production path bypasses FIX 1. All four call sites land on the patched constructor.

- **A2 — `paths.CachesDir` field name correctness.** REFUTED. `internal/config/paths.go:22` declares `CachesDir string` in the `Paths` struct. `ResolvePaths` populates it at line 56 as `filepath.Join(homeDir, "Library", "Caches", "valv")`. `Paths.Ensure()` at line 73 `MkdirAll`s the directory. The field name in FIX 1 matches the struct field exactly. Symmetric with how `BuildCacheDir` and other path fields are threaded through.

- **A3 — Empty `paths.CachesDir` fallback to CWD-relative path.** **REFUTED in production, NOTE for partial-`Paths` constructions in tests.** Production: `cmd/valv/main.go` → `cli.NewRootCommand` → `config.ResolvePaths(homeDir)` always populates `CachesDir`. Tests: `testCodexPaths(t)` at `codex_test.go:461-482` populates `CachesDir = filepath.Join(root, "caches")` for every cli test. ONE construction at `codex_test.go:81-96` (`TestRunCodexCommandReturnsEnsureError`) builds a hand-rolled `config.Paths{...}` literal with `CachesDir: filepath.Join(root, "caches")` populated. No production or test path constructs `Paths` with empty `CachesDir`. If a future test ever builds a partial `Paths` without `CachesDir`, `filepath.Join("", "version-cache.json")` returns `"version-cache.json"` (relative) — but that's a future-test hazard, not a current bug. NOTE only.

- **A4 — FIX 2 `VALV_REAL_HOME=t.TempDir()` interaction with other env state.** REFUTED. The test still sets `VALV_CODEX_IMAGE`, `valvTestSkipHostCodexLoginEnv=1`, and `PATH` (via fake docker install). `prepareCodexRuntime` at `runtime.go:71` calls `os.MkdirAll(sharedHome, 0o755)` to ensure the empty dir exists before `copyDirContents`. `copyDirContents` at `runtime.go:227-235` returns `nil` if the source doesn't exist; on an empty dir, `filepath.Walk` only visits the root and skips it (line 242-244). Zero files copied. The fake docker binary handles `image inspect` and `run --rm` paths (both exit 0 via the test's shell script at `codex_test.go:401-406`). `CODEX_HOME` is not set on the host — `runCodexHostCommand` would set it but the test bypasses host codex via `valvTestSkipHostCodexLoginEnv=1` (`account_auth.go:74,215-217`). Full chain works.

- **A5 — FIX 2: does the test still test what it was supposed to?** REFUTED. The test's assertion is at `codex_test.go:443-445`: `if bytes.Contains(got, []byte("--debug")) { t.Fatalf(...) }`. It reads the captured docker-run args from `logPath` and asserts the root `--debug` flag was NOT forwarded. With `VALV_REAL_HOME=t.TempDir()`, the test now reaches `service.Run` → `executor.Run` → fake docker, which logs the actual run args to `logPath`. Before FIX 2, the test would fail at `PrepareRuntime`'s `copyDirContents` (ENOSPC) BEFORE reaching the docker exec. So FIX 2 doesn't change WHAT is tested — it removes the disk-space dependency that prevented the test from running at all. The `--debug` passthrough verification is preserved.

- **A6 — Mage test count anomaly.** **CONFIRMED — builder reporting inaccuracy, not a test failure.** Builder reported `mage test GREEN 428/428` × 3 runs in `BUILDER_WORKLOG.md:55` (AC6) and `BUILDER_WORKLOG.md:38-40`. Independent re-run from `main/` on HEAD (commit `6b55d59`): **`mage test` produces 431/431 GREEN** across 20 packages (two consecutive runs confirmed). Coverage thresholds met for every package; `internal/cli=72.5%`, `internal/services/images=79.2%`. The 431/431 number aligns with the orchestrator's expected range (430-431) and contradicts the builder's 428/428 figure. The R2 QA-PROOF appendix at `BUILDER_QA_FALSIFICATION.md:1135` already noted this exact discrepancy for Unit 7.9 R2 ("Builder 7.9 R2 reported 430/430 and 7.10 R2 reported 428/428 — both predate one of the R2 commits being on disk at the time their respective mage runs landed"). Same pattern reproduced here. **No tests fail; the count is just mis-reported.** Not a BLOCK — production correctness is intact, but builder reports should be re-verified.

- **A7 — New test `TestOpenImagesServiceWritesCacheToCachesDir` exercises the write path.** REFUTED. The test at `operator_helpers_test.go:97-156` constructs the service, then calls `svc.EnsureLatest(ctx, imagesservice.EnsureRequest{AllowExistingOnCheckFail: true})` at line 128. Trace through `service.go:367-419`:
  1. `currentState` returns empty + `stateFound=false` (new test store).
  2. `readVersionCache(s.cachePath)` returns empty cache (fresh path).
  3. Cache miss → `s.resolver.LatestVersion(ctx)` calls the stubbed resolver returning `"2.2.0"`.
  4. `writeVersionCache(s.cachePath, "2.2.0", ProviderClaude, now)` writes the file at `s.cachePath` = `paths.CachesDir/version-cache.json` (line 406).
  5. `imageAvailable` runs `docker image inspect valv-claude:dev` → fake docker exits 0 → returns true.
  6. `imageRecipeMatches` runs `docker image inspect --format ...` → fake docker outputs empty (`VALV_DOCKER_IMAGE_INSPECT_OUTPUT` unset) → recipe mismatch.
  7. `Build()` runs `docker buildx build --load ...` → fake docker exits 0.
  Cache write happens at step 4, BEFORE the docker-build steps. The test's `os.Stat(wantCachePath)` at line 131 confirms the file exists at the expected path. End-to-end write path exercised — not just a field-value check.

- **A8 — Concurrent-test pollution between mage test runs.** REFUTED. `TestOpenImagesServiceWritesCacheToCachesDir` does NOT call `t.Parallel()` (verified at line 97 of operator_helpers_test.go — no `t.Parallel()` invocation). It cannot, because it calls `installFakeDocker` which uses `t.Setenv` (incompatible with `t.Parallel`). Same constraint binds every other test that calls `installFakeDocker` (verified across `manage_test.go` and `extended_test.go` — none of the `installFakeDocker` callers use `t.Parallel`). Per-test cache isolation via `t.TempDir()` (which is itself per-test-unique) means even if these tests ran in parallel they'd each write to their own `paths.CachesDir`. No cross-test cache state leak.

- **A9 — `paths` not threaded through tests.** REFUTED. Every cli test that constructs the images service does so via `openImagesService(cmd, paths, provider)` with `paths` from `testCodexPaths(t)`. The new FIX 1 makes `paths.CachesDir` flow into the cache. No "indirect" path bypasses this — see A1.

- **A10 — FIX 2 `t.Setenv` ordering vs subprocess.** REFUTED. The env-set sequence in `TestRootDebugFlagIsNotPassedThroughToInteractiveCodexLaunch` (codex_test.go:379-411):
  1. line 380: `t.Setenv("VALV_CODEX_IMAGE", "valv-codex-dev:dev")`
  2. line 381: `t.Setenv(valvTestSkipHostCodexLoginEnv, "1")`
  3. line 389: `t.Setenv("VALV_REAL_HOME", t.TempDir())` ← FIX 2
  4. line 391: `paths := testCodexPaths(t)`
  5. line 396: `runManage(...)` (account add)
  6. line 411: `t.Setenv("PATH", binDir+...)` (fake docker)
  7. line 435: `cmd.Execute()` → `runCodexCommand` → eventually `realHomeDir()`
  `VALV_REAL_HOME` is set FIRST (step 3) before any code that calls `realHomeDir()`. `realHomeDir()` (`operator_helpers.go:304`) reads the env var on every call, no caching. Subprocess (fake docker) inherits the env after step 6. Order is correct; FIX 2 sequence works.

### R1 finding closure

- **A8 cache pollution: CLOSED.** R1 confirmed pollution of dev's `~/Library/Caches/valv/version-cache.json` with test values. R2 FIX 1 threads `paths.CachesDir` through. The new test `TestOpenImagesServiceWritesCacheToCachesDir` positively verifies cache lands at `paths.CachesDir/version-cache.json`. Mechanism: `openImagesService` (the SINGLE construction site, per A1) always sets `CachePath`, so `imagesservice.New` never falls back to `defaultCachePath()` for cli-originated callers. Builder's `BUILDER_WORKLOG.md:43-45` reports the dev's real cache file mtime unchanged across 3 consecutive `mage test` runs — independent corroboration via filesystem observation.
- **Tmpfs flake: CLOSED.** R1 reproduced a 427/428 tmpfs ENOSPC failure of `TestRootDebugFlagIsNotPassedThroughToInteractiveCodexLaunch` due to copying the dev's real `~/.codex` into a tmpfs-backed `t.TempDir()`. R2 FIX 2 short-circuits the copy via `VALV_REAL_HOME=t.TempDir()`. `PrepareRuntime`'s copy step now walks an empty dir (zero bytes copied). Verified live: `mage test` GREEN 431/431, including this specific test, on two consecutive runs without any environmental disk-space prep.

### Mage test ground-truth

- **count: 431/431** (two independent `mage test` runs from `main/` on HEAD `6b55d59`).
- **failures: none.**
- **coverage:** all 20 packages above the 60% floor. Specific cover values for the most-touched packages: `internal/cli=72.5%`, `internal/services/images=79.2%`.
- **race detector:** clean.
- **Builder reported count (428/428) does NOT match ground truth (431/431).** Same "mage run predates final commit" pattern as 7.9 R2 (per the R2 QA-PROOF appendix). Production correctness intact; reporting accuracy concern only.

### Findings

- **CONCERN (A6): Builder test-count mis-reporting recurrence.** Builder reported 428/428 in `BUILDER_WORKLOG.md:38-40,55` and the AC6 acceptance criterion explicitly cites "428/428 GREEN". Ground truth (after both FIX 1 and FIX 2 commits land on HEAD) is **431/431 GREEN**. Same pattern as Unit 7.9 R2 (per the R2 QA-PROOF appendix). No tests fail; the discrepancy is in reporting only. Routes to orchestrator: builders should re-run `mage test` AFTER their final commit, not before. The QA-PROOF appendix from the prior unit had already flagged this pattern; recurrence indicates the lesson did not propagate.
- **NOTE (A3): Empty-`paths.CachesDir` future-test hazard.** No current code path produces an empty `CachesDir`, but if a future test builds a partial `config.Paths{}` literal without populating `CachesDir`, FIX 1 would produce `"version-cache.json"` (relative path) and write to CWD. Not exploitable today. Routes to orchestrator as a defensive-hardening candidate: add `if strings.TrimSpace(paths.CachesDir) == ""` guard inside `openImagesService` OR `Paths.Ensure()` validate non-empty `CachesDir`.
- **NOTE (test-design):** The new test's "Confirm the real platform cache path was NOT written" block at `operator_helpers_test.go:135-155` is intentionally non-fatal (the comment at line 148-150 explains the rationale). The real existence-of-isolation proof is the `os.Stat(wantCachePath)` assertion at line 131 + the mechanism (FIX 1 forces `CachePath` to a fresh `t.TempDir()` path, so `defaultCachePath()` is unreachable from this caller). Defensible test design; documenting for future readers.

### Counterexamples

None CONFIRMED. The A6 mage-test-count mismatch is a reporting accuracy concern, not a behavior counterexample — all 431 tests pass, including the new test pinning FIX 1 and the previously-flaky test pinned by FIX 2.

### Unknowns

- **Live smoke test outcome:** whether `valv manage update claude` on a fresh install actually writes to `~/Library/Caches/valv/version-cache.json` (NOT `$XDG_CACHE_HOME` overrides or other edge paths). Routes to orchestrator for dev smoke test at drop close.
- **A3 follow-up:** whether to add an empty-`CachesDir` guard inside `openImagesService` (or move the validation into `Paths.Ensure()`). Routes to orchestrator/dev.
- **Builder test-count reporting:** procedural fix to prevent recurrence. Routes to orchestrator.

### Verdict

`pass` — no CONFIRMED counterexamples. FIX 1 (`openImagesService` threads `paths.CachesDir` into `Options.CachePath`) and FIX 2 (`VALV_REAL_HOME=t.TempDir()` in the interactive codex launch test) both land cleanly with file:line evidence and behavior-pinned tests. Both R1 findings (A8 cache pollution + tmpfs flake) are CLOSED with positive test coverage AND filesystem-observation corroboration. Mage test ground truth is 431/431 GREEN — three tests higher than the builder reported but consistent with the orchestrator's expected delta range. The builder-reporting accuracy concern (A6) is a procedural NOTE for orchestrator routing, not a regression-causing finding.

## Hylla Feedback

- **Hylla daemon unreachable** (`dial tcp 127.0.0.1:9080: connect: connection refused`) during this review. Three `hylla_search_keyword` queries attempted (`imagesservice.New imagesservice.Options`, `CachesDir`, `CachePath`) — all failed with the connection-refused error. Per CLAUDE.md § "Code Understanding Rules" item 2, files modified since the last Hylla snapshot use direct `Read` regardless of daemon state, so this round's primary evidence path was unaffected.
- **Worked via:** direct `Read` of `internal/cli/operator_helpers.go`, `internal/cli/operator_helpers_test.go`, `internal/cli/codex_test.go`, `internal/cli/codex.go`, `internal/cli/claude.go`, `internal/cli/claude_image.go`, `internal/cli/claude_auth.go`, `internal/cli/manage.go`, `internal/cli/manage_test.go`, `internal/cli/extended_test.go`, `internal/cli/claude_image_test.go`, `internal/services/codex/service.go`, `internal/adapters/providers/codex/runtime.go`, `internal/services/images/service.go`, `internal/services/images/cache.go`, `internal/config/paths.go`, `cmd/valv/main.go`. `git diff HEAD~1 HEAD -- internal/cli/` for the unit's exact change set. `mage testPkg ./internal/cli` and `mage test` × 2 for ground-truth count verification.
- **Suggestion:** None for Hylla itself — daemon down is infra. The A1 attack (find-all-call-sites of `imagesservice.New` / `imagesservice.Options{}`) is the textbook use case for `hylla_search_keyword` + `hylla_refs_find`; falling back to per-file `Read` is feasible only because the cli package has a small import surface. On a larger codebase the A1 audit without Hylla would be substantially more expensive.

---

## Unit 7.11 — Round 1

**Date:** 2026-05-16
**Verdict:** fail (one BLOCK + three CONCERNs)

### Attack vectors probed

- **A1 — lineScanner forwarding correctness (trailing partial line and Write contract).** CONFIRMED CONCERN. `lineScanner.Write` (`claude_auth.go:108-126`) only forwards bytes once a `\n` is seen. Bytes after the last `\n` accumulate in `s.buf` and are NEVER flushed — there is no `Flush()` method, no `Close()` drain, no end-of-stream hook. When the docker container exits and the reader goroutine returns, any trailing partial line is permanently lost. Concrete failure: claude's TUI cleanup phase commonly emits final cursor-reset / cursor-home bytes without a terminating `\n` — the user's terminal misses them. Severity: CONCERN, not BLOCK — real claude output is `\n`-heavy and the loss is cosmetic, but it IS a real bug. Recommendation: add a `Flush()` method that writes `s.buf` to inner and clears it; call it after `containerExec.Run` returns. Minor sibling issue: the Write contract is violated when `s.inner.Write` partially succeeds — code returns `(0, err)` even though some prior lines already reached inner; the docker reader does not retry from offset so practical impact is nil, but `io.Writer` godoc says `0 <= n <= len(p)` should reflect bytes consumed.

- **A2 — URL match across `\n` boundary (TUI line-wrap).** CONFIRMED CONCERN. If claude's TUI wraps a long OAuth URL across `\n` boundaries (Ink-based renderers wrap at terminal width), `oauthURLRegex.FindString` (`claude_auth.go:121`) matches only the prefix up to the first `\n`. `\S` includes `\n` in its complement so the regex stops there. The `sync.Once` in `RunInContainer` (`claude_auth.go:193-198`) then permanently latches on the broken URL — `open` is called with the truncated URL and the auto-open feature is irreversibly broken for the session. Real-world likelihood: medium-low (depends on terminal width vs URL length), but irreversible-per-process when it fires.

- **A3 — Regex over-match through ANSI escape codes.** CONFIRMED CONCERN. The OAuth URL is typically printed inside an ANSI-styled box (cyan/blue) by claude's TUI. The regex `\S*` (`claude_auth.go:30`) is non-greedy on whitespace only — ESC (`\x1b`, 0x1b) is non-whitespace per Go regexp (`\S` is `[^\t\n\f\r ]`), so trailing escape sequences like `\x1b[0m` get consumed into the captured URL. Counterexample: claude emits `https://claude.com/cai/oauth/authorize?code=foo\x1b[0m\n` on a single TUI line; `FindString` returns `https://claude.com/cai/oauth/authorize?code=foo\x1b[0m`. `exec.Command("open", url).Start()` (`claude_auth.go:63`) hands the URL with literal ESC bytes to macOS `open`, which silently fails or opens a malformed URL. The `sync.Once` then permanently latches on this broken URL — the user falls back to manual paste, defeating FIX A's entire purpose. **This is the most-likely-to-actually-fire failure mode** because claude TUI uses ANSI heavily by default. Recommendation: ANSI-strip the captured URL before passing to `urlOpener.Open` (e.g. regex out `\x1b\[[0-9;]*[A-Za-z]`), OR tighten the regex character class to URL-legal chars only (`[A-Za-z0-9._~:/?#\[\]@!$&'()*+,;=%-]*`).

- **A4 — sync.Once reset across multiple RunInContainer calls.** REFUTED. `var once sync.Once` is declared INSIDE `RunInContainer` (`claude_auth.go:193`). Each invocation gets a fresh `Once`. Second call from the same runner instance is not affected.

- **A5 — atomic.Bool credDetected memory ordering.** REFUTED. Goroutine path: `credDetected.Store(true)` (line 248) happens BEFORE `defer wg.Done()` (line 235) fires (since deferred `Done` runs at goroutine return). Main path: `wg.Wait()` (line 259) happens-before `credDetected.Load()` (line 261). Go memory model: `wg.Done` synchronizes-with `wg.Wait` → Store happens-before Load. The WaitGroup actually makes the atomic ordering-redundant (a plain bool with WG fence would also be correct); the atomic is defensible defensive programming.

- **A6 — `docker stop --time 5` adequacy.** REFUTED for creds correctness. The poller detects the creds file at non-zero size BEFORE sending the signal. The file is durable on the bind-mount. SIGKILL after 5s only kills in-flight network/cleanup, not the creds file. The 5s timeout is sufficient for the unit's stated goal (don't leave the user staring at a TUI after auth completes).

- **A7 — docker stop race with main container exit.** REFUTED. Error from `externalCommand("docker", "stop", ...)` is explicitly swallowed via `_ =` (`claude_auth.go:251`). If claude exited cleanly before the watcher's poll tick, the stop call errors with "no such container" and the swallow is correct per spec D6.

- **A8 — WaitGroup deadlock on goroutine panic.** REFUTED. `defer wg.Done()` is the first statement of the goroutine body (`claude_auth.go:235`). Panics inside `WaitForCreds` or the docker stop call still fire `Done`. The production `defaultCredsWatcher` has no panic surface (`os.Stat` + ticker + select). Injected watchers could panic but tests don't exercise it.

- **A9 — Context cancellation propagation.** REFUTED. `defer cancel()` (line 172) fires when `RunInContainer` returns. Watcher's `select { ctx.Done; ticker.C }` (line 75-83) sees `ctx.Done` on next iteration. If the goroutine is inside `externalCommand(...).Run()` it doesn't see ctx.Done() until the subprocess returns, but `docker stop --time 5` has a bounded ≤5s lifetime so the goroutine cannot leak.

- **A10 — `externalCommand` package-level test seam pollution.** REFUTED. `TestRunInContainerSigtermsOnCredsWrite` (`claude_auth_test.go:741`) mutates `externalCommand` and registers `t.Cleanup` to restore it. The mutation happens under `t.Parallel()`, but the only other parallel test that could read `externalCommand` is one whose goroutine reaches the docker stop call — and those parallel tests use `stubCredsWatcher{err: context.Canceled}` so their goroutine returns BEFORE reaching `externalCommand`. `mage test -race` GREEN 441/441 (run 2026-05-16 mid-review) — if a real race existed it would have been caught.

- **A11 — stderr URL detection.** REFUTED for correctness. Wrapping both stdout and stderr is harmless because `sync.Once` dedups. Whether claude actually emits the URL on stderr is unverified but the wrapping itself doesn't introduce bugs.

- **A12 — lineScanner unbounded buffer growth.** CONFIRMED NOTE. `s.buf = append(s.buf, p...)` (`claude_auth.go:109`) has no cap. An adversarial container emitting bytes without any `\n` would grow the buffer unboundedly → OOM. Real claude TUI is `\n`-heavy so realistic risk is low. Recommendation: add a `maxBufferSize` const (e.g. 1MB) — when exceeded, flush the partial buffer to inner without scanning and reset. Filed as NOTE because no concrete production trigger exists, but it's a hardening gap.

- **A13 — 500ms poll vs claude's file-write atomicity / pre-existing creds.** CONFIRMED BLOCK. The `Size > 0` check (`claude_auth.go:80`) correctly filters zero-byte transient writes, so atomicity per se is fine. BUT — the same check makes the poller succeed IMMEDIATELY when `.credentials.json` already exists (size > 0). `loginClaudeAccount` (`claude_auth.go:343-364`) calls `RunInContainer` WITHOUT first wiping the existing creds. Pre-Unit-7.11 this worked because the container ran until the user Ctrl-C'd (overwriting creds via claude's natural re-auth). Post-Unit-7.11, the new poller sees the EXISTING creds file on the first 500ms tick, fires `docker stop`, kills the container before claude's TUI can complete the re-auth, and `RunInContainer` returns nil with "Claude auth complete" — but the credentials are STALE.

  - **Caller chain (verified via Hylla):** `runManageAccountLogin` (`manage.go`) → `loginManagedAccount` (`account_auth.go`) → `loginClaudeAccount` (`claude_auth.go:343`) → `RunInContainer`. No wipe at any layer.
  - **Concrete user repro:** `valv manage account login claude work` on an account that has an existing `.credentials.json`. Expected: user gets a fresh OAuth flow in browser, new creds replace old. Actual: container starts, poller's first 500ms tick detects the existing file, docker stop fires, user sees a 1-2s flash and "Claude auth complete" — but the credentials never refreshed.
  - **`ensureClaudeAccountReady` is NOT affected** because it has an already-authed early-return (`claude_auth.go:302-304`) that fires before `RunInContainer`. The bug is specific to the explicit-re-login path.
  - **Fix options:** (a) add `wipeClaudeCredentials(account.HomePath)` at the start of `loginClaudeAccount` (before `writeCLINotice`) — restores force-fresh semantics; (b) thread a `forceFresh bool` flag through `RunInContainer` that bypasses the poller. Option (a) is the cleaner fix and matches the documented intent of `loginClaudeAccount` ("explicit re-login").
  - **Why this is BLOCK not CONCERN:** it's a real user-facing regression of an existing CLI surface (`valv manage account login`). The unit's test suite never exercises `loginClaudeAccount` against a pre-existing creds file with the real `defaultCredsWatcher`, so the regression was not surfaced by the build-test gates.

- **A14 — `docker stop --time 5` flag spelling on Docker Desktop.** REFUTED. Modern Docker (since 1.13) supports `--time` for `docker stop`. Docker Desktop ships current Docker engine. Flag is correct.

- **A15 — Test injection vs production parity.** CONFIRMED NOTE. All `RunInContainer` integration tests use `stubCredsWatcher` (`claude_auth_test.go:493-501`). The production `defaultCredsWatcher` (`claude_auth.go:71-85`) has zero direct test coverage. The logic is trivial (ticker → select → stat → size check → return), so visual inspection suffices for v0.1.0 — but a single integration-style test that exercises the real watcher against a temp file with a short ctx timeout would close the gap and would have CAUGHT the A13 BLOCK by writing a file before calling `RunInContainer`.

- **A16 — Goroutine leak verification.** REFUTED. `TestRunInContainerSurvivesContainerExitBeforeCreds` does not use `runtime.NumGoroutine()`, but its mechanism IS valid leak proof: if the goroutine leaked, `wg.Wait()` (`claude_auth.go:259`) would block forever and the test would hang on `runner.RunInContainer`. Test completes promptly → goroutine returned. For the production `defaultCredsWatcher` path, the goroutine returns within ≤500ms of `defer cancel()` firing.

- **A17 — `go test -race` execution.** REFUTED. `magefile.go:138` (`mage testPkg`) and `magefile.go:196` (`mage test`'s `runRepoTests`) both pass `-race`. `mage testPkg ./internal/cli` GREEN 167/167 (run 2026-05-16). `mage test` GREEN 441/441 across 20 packages. Race detector is clean for all atomic + WaitGroup + sync.Once + goroutine + externalCommand-mutation choreography exercised by the unit's tests.

### Findings

- **BLOCK 1 (from A13): `loginClaudeAccount` re-login regression.** `RunInContainer`'s new creds-poller short-circuits the explicit re-login flow when an account already has `.credentials.json`. Fix: add `wipeClaudeCredentials(account.HomePath)` at the start of `loginClaudeAccount` (before `writeCLINotice`). Optionally add a `loginClaudeAccount`-targeted test that pre-writes a creds file, calls `loginClaudeAccount` with a stub runner that records whether `RunInContainer` was actually reached (and `RunInContainer` should subsequently re-write the creds via the stub watcher). Severity: real user-facing regression of `valv manage account login claude <name>` on already-authed accounts.

- **CONCERN 1 (from A1): Trailing partial line never flushed to terminal.** Bytes after the final `\n` sit in `s.buf` and are dropped when the docker reader returns. Realistic exposure: 1-10 final cleanup bytes from claude's TUI. Recommendation: add a `Flush()` method on `lineScanner` and call it after `containerExec.Run` returns (and before reading `credDetected`).

- **CONCERN 2 (from A3): ANSI escape codes captured into OAuth URL.** Claude's TUI styles the OAuth URL with ANSI codes. The `\S*` regex captures trailing `\x1b[0m` (color reset) bytes into the URL. `open` receives a URL with literal ESC bytes and silently fails. `sync.Once` then permanently latches on the broken URL — auto-open is silently dead for the session. Most-likely-to-actually-fire failure mode in production. Recommendation: strip ANSI escape sequences before passing the URL to `urlOpener.Open`, OR tighten the regex character class to URL-legal characters only.

- **CONCERN 3 (from A2): URL split by TUI line-wrap.** If claude's TUI wraps a long OAuth URL at terminal width, only the prefix matches and `sync.Once` latches on the truncated URL. Lower probability than CONCERN 2 but irreversible-per-process when it fires. Mitigation may need to be deferred — proper fix requires the TUI not to wrap, which Valv cannot control. At minimum, log a debug line with the captured URL so post-hoc diagnosis is possible.

- **NOTE 1 (from A12): `lineScanner` buffer is unbounded.** No `maxBufferSize` cap. An adversarial / buggy container emitting bytes without `\n` would OOM. Realistic trigger absent. Optional hardening.

- **NOTE 2 (from A15): Production `defaultCredsWatcher` has no direct unit coverage.** All RunInContainer integration tests stub the watcher. A single test that writes a real temp file and exercises the production `defaultCredsWatcher` would close the gap AND would have caught BLOCK 1 (by simply writing a creds file before calling `RunInContainer`).

- **NOTE 3 (from A1b): `lineScanner.Write` returns 0 on partial-inner-write failure.** Code returns `(0, err)` even when prior lines were already forwarded successfully. Technically a violation of `io.Writer` contract (`0 <= n <= len(p)` should reflect bytes consumed), but the docker reader does not retry from offset so practical impact is nil.

### Mage rerun

- `mage testPkg ./internal/cli` (with `-race`) — GREEN 167/167, 72.6% coverage (matches builder's reported number).
- `mage test` (with `-race`) — GREEN 441/441 across 20 packages, all packages ≥60% (matches builder's reported number).

The green test suite does NOT exercise BLOCK 1 because:
- `RunInContainer` integration tests use `stubCredsWatcher` that returns `nil` or `context.Canceled` directly and never touches the filesystem.
- No test calls `loginClaudeAccount` with a pre-existing `.credentials.json` file under the real `defaultCredsWatcher` path.

### Unknowns

- **U1 — Whether claude 2.1.143's TUI actually emits the OAuth URL embedded in ANSI styling.** Probability assessed as high (TUI is Ink-based, styled output is the default), but unverified without a live dogfood run. CONCERN 2 hinges on this — if the URL is emitted as plain text outside styled context, the regex over-match is moot.
- **U2 — Whether claude 2.1.143's TUI wraps long URLs at terminal width.** CONCERN 3 hinges on this.
- **U3 — Exit code from `docker run --rm` under `docker stop` (builder's Unknown U1 from BUILDER_WORKLOG).** The `credDetected` guard handles non-zero exit correctly per the implementation, but the actual exit semantics are unverified. Not a blocker for this round.

## Hylla Feedback

- **Query:** `hylla_search_keyword` for `loginClaudeAccount` with `internal_mode=include_internal`, `visibility_mode=public_only` — zero results.
  - **Missed because:** `loginClaudeAccount` is an unexported function (visibility=private) in `internal/cli`. The default `visibility_mode=public_only` filters it out. A QA-falsification review needs to reach unexported symbols routinely.
  - **Worked via:** Re-ran with `visibility_mode=include_private` — returned the node successfully (alongside `loginManagedAccount` which calls it). Then `hylla_node_full` on `loginManagedAccount` and `runManageAccountLogin` to walk the caller chain.
  - **Suggestion:** Consider making `visibility_mode=include_private` the default for `hylla_search_keyword` when `internal_mode=include_internal` is also set — the combination "internal-only symbols, but only public" is rarely what a caller wants and is the cause of frequent zero-result re-tries. Alternatively, surface a clearer error/hint in the response when the search would have matched a private symbol that the visibility filter excluded.
- The Hylla snapshot is otherwise STALE for Unit 7.11's `claude_auth.go` changes (no reingest since Unit 7.5). All direct content reads went through `Read` per mid-drop evidence protocol. No further misses recorded.

---

## Unit 7.11 — Round 2

**Date:** 2026-05-16
**Verdict:** fail (1 BLOCK confirmed, 1 CONCERN, 1 test-design gap)
**Commit reviewed:** `660d538 fix(cli): unit 7.11 r2 wipe-before-login + ansi-safe url + line-wrap buffering`

### Summary

The orchestrator's hypothesized attack against FIX 3 (line-wrap buffering) is **CONFIRMED**. The buffer-scan correctly accumulates wrapped URL segments, but the production `sync.Once.Do` wrapper around the lineScanner's `onMatch` callback latches on the **first** match — which is the **partial** URL produced after part1's terminating newline. The longer, joined-via-buffer match fired after part2 is silently discarded because `sync.Once` is a no-op on subsequent calls. The unit test `TestLineScannerDetectsURLAcrossMultipleLines` passes only because it collects all matches into a slice without a `sync.Once` guard — production reality differs. FIX 1 and FIX 2 are correct.

### Attack Attempts

CONFIRMED = counterexample produced. REFUTED = attack tried, evidence rules it out. EXHAUSTED = honest attempt, no counterexample constructable.

1. **FIX 3 — sync.Once latches on partial URL (orchestrator's primary attack).** **CONFIRMED — BLOCK 1.**

   Evidence trail:
   - **Production wiring** (`internal/cli/claude_auth.go:235-243`):
     ```go
     var once sync.Once
     onURL := func(url string) {
         once.Do(func() {
             _ = opener.Open(ctx, url)
         })
     }
     stdoutScanner := newLineScanner(stdout, onURL)
     stderrScanner := newLineScanner(stderr, onURL)
     ```
     Both stdout and stderr scanners share the SAME closure with the SAME `sync.Once`.
   - **`lineScanner.Write` fires onMatch on EVERY newline-terminated regex match** (`claude_auth.go:139-167`), no "is this longer than the previous match?" guard, no "did the previous line already match?" suppression.
   - **Regex `[A-Za-z0-9._~:/?#\[\]@!$&'()*+,;=%-]*` is zero-or-more greedy** — `part1 = "https://claude.com/cai/oauth/authorize?code=foo"` matches by itself because every char after `/authorize` is in the class.
   - **`sync.Once` semantics confirmed via Context7** (`/golang/go` — `go_mem.html`): function runs exactly once on first call; subsequent `Do` calls return without re-invoking.
   - **Test author acknowledges the gap** at `claude_auth_test.go:922-923`: *"The single-line match on the first line may also fire with a partial URL; deduplication is the caller's responsibility via sync.Once in RunInContainer."* — but that "deduplication" is precisely the bug, because sync.Once latches the FIRST (shortest) match, not the LONGEST.

   Concrete trace under wrapped URL:
   1. Claude TUI prints `https://claude.com/cai/oauth/authorize?code=foo\n` (terminal wrapped at width N).
   2. `stdoutScanner.Write` → `urlBuf = "https://claude.com/cai/oauth/authorize?code=foo\n"` → stripped → regex matches `"https://claude.com/cai/oauth/authorize?code=foo"`.
   3. `onMatch(partial)` fires → `sync.Once.Do` runs → `opener.Open(ctx, partial)`. **Host browser opens partial URL.**
   4. Claude TUI prints `bar&baz=qux\n`.
   5. `stdoutScanner.Write` → `urlBuf = "https://claude.com/cai/oauth/authorize?code=foo\nbar&baz=qux\n"` → stripped → regex matches `"https://claude.com/cai/oauth/authorize?code=foobar&baz=qux"`.
   6. `onMatch(full)` fires → `sync.Once.Do` is a no-op. **Full URL silently discarded.**
   7. End state: browser at partial URL, OAuth flow rejected because `code=foo` is not the true code emitted by the server. Same observable failure as the pre-R2 line-wrap bug.

   Test fidelity gap: `TestLineScannerDetectsURLAcrossMultipleLines` (`claude_auth_test.go:924-962`) passes because it collects `allMatches` without sync.Once. The full URL appears in the slice on the SECOND onMatch call. Production discards that second call.

   **Remediation suggestions (from orchestrator's prompt, plus my additions):**
   - **(c) Match-on-extension only** — only fire `onMatch` when the line just appended did NOT itself match the regex on its own (i.e., we're confident we're extending a wrapped URL rather than introducing a fresh single-line URL). Single-line URLs still hit the wrap-buffer's match path because their terminating newline triggers the same `FindString` over the just-appended single line, which IS a complete URL.
     - This is the cleanest fix IMO. Concretely:
       ```go
       prevStripped := strings.Map(stripURLWhitespace, /* urlBuf BEFORE appending lineStr */)
       prevMatch := oauthURLRegex.FindString(prevStripped)
       s.urlBuf.WriteString(lineStr)
       stripped := strings.Map(stripURLWhitespace, s.urlBuf.String())
       m := oauthURLRegex.FindString(stripped)
       lineOnlyMatch := oauthURLRegex.FindString(strings.Map(stripURLWhitespace, lineStr))
       // Fire only the LONGEST match seen so far for this URL session.
       if m != "" && m != prevMatch {
           s.onMatch(m)
       }
       ```
       But this still triggers sync.Once on the first full-line URL — which is what we WANT. The issue is only when a SECOND, longer match arrives later. Naive "only fire if new match is longer" inside the scanner + drop `sync.Once` in `RunInContainer` (or replace sync.Once with "always invoke; opener is idempotent / no-op on duplicate URL") is the cleanest path.
   - **(b) Defer-by-timeout (debounce)** — collect the longest match within a 100-200ms window, then fire. Pragmatic but adds timing complexity. Likely the right shape for a TUI that prints the URL line then ANSI-styles around it.
   - **(d) Cleaner alternative**: drop `sync.Once` from `RunInContainer` entirely; have `lineScanner` track the longest match it's seen and only call `onMatch` when the longest grows. Move the "open exactly once" guard into the `opener` or a small wrapper that compares `url` to the last URL opened. This separates the two concerns: scanner = "tell me the BEST URL you've seen", opener wrapper = "open exactly one URL per session, idempotent".

2. **FIX 3 — 4 KiB buffer cap interacts badly with terminal-wrapped URLs after a long preamble.** **CONFIRMED as CONCERN 2** (lower severity than BLOCK 1).

   - `urlBufferCap = 4096`. If claude prints >4 KiB of preamble (warnings, banner, ANSI-styled help text) before the URL, the wrap detection between part1 and part2 of a subsequent wrapped URL fails: the cap-overflow branch (`claude_auth.go:154-159`) `Reset()`s `urlBuf` between part1 and part2 if their combined append straddles the cap.
   - Specifically, if part1 is the line that pushes urlBuf past 4096, urlBuf resets and `urlBuf.WriteString(part1)` makes urlBuf = part1 alone. So part1 itself still gets matched (as a partial URL) and sync.Once latches the partial — falls into BLOCK 1's trace anyway, so this CONCERN is mostly subsumed.
   - A NARROWER scenario where this CONCERN bites independently: if the cap-overflow happens BETWEEN part1 and part2 (urlBuf has accumulated part1 plus prior content, exceeds cap, resets — then part2 arrives and urlBuf = part2 alone). Then part2's regex scan against `bar&baz=qux` finds no `https://...` prefix → no match fires. The full URL is never seen at all.
   - **Mitigation:** raise the cap, or make the reset behavior smarter (keep the last N bytes of urlBuf to preserve cross-line URL context). Real OAuth URLs from claude are ~150-300 chars; cap at 4 KiB has ample room IF preamble is small. The pre-R2 dogfood log would help calibrate.

3. **FIX 3 — Test fidelity gap.** **CONFIRMED as CONCERN 3** (test design issue, not a production bug per se but a process gap).

   - `TestLineScannerDetectsURLAcrossMultipleLines` deliberately does NOT model the production `sync.Once.Do` wrapper around `onMatch`. The test asserts the full URL is somewhere in the `allMatches` slice (line 952-960) but does not assert which match was FIRST. A test that mirrored production wiring — `var once sync.Once; scanner := newLineScanner(&buf, func(url string){ once.Do(func(){ opened = append(opened, url) }) })` — would have caught BLOCK 1 by asserting `opened[0] == wantURL`.
   - **Remediation**: add a `TestLineScannerWithSyncOnceLatchesLongest` regression test in the R3 cycle that mirrors the production wiring exactly, asserts the URL passed to `opener.Open` is the FULL URL not the partial.

4. **FIX 1 — wipeClaudeCredentials failure-mode coverage.** REFUTED. `os.Remove` errors (other than `IsNotExist`) propagate cleanly via `fmt.Errorf("remove %q: %w", ...)` at `claude_auth.go:418`. Test `TestWipeClaudeCredentialsRemovesFile` covers the success path; `TestWipeClaudeCredentialsMissingFileIsNoError` covers the IsNotExist branch. Minor test gap: no test exercises the read-only-directory / EROFS branch, but the error wrapping path is straightforward and not a blocker. The wipe also runs BEFORE `RunInContainer` (verified by `TestLoginClaudeAccountWipesExistingCredsBeforeRunning` at `claude_auth_test.go:847-887`) — so if wipe fails, the runner is never invoked, which is the correct fail-closed behavior.

5. **FIX 1 — Edge case: empty/whitespace `account.HomePath`.** REFUTED as a FIX 1 defect; routed as upstream concern. If `account.HomePath == ""`, `filepath.Join("", ".credentials.json")` = `".credentials.json"` (relative to cwd) — `os.Remove(".credentials.json")` could either remove a file in the current working directory (security concern) or return `IsNotExist`. This is an upstream path-validation gap, not a regression introduced by FIX 1. Recommend deferring to a future hardening pass; not a blocker for this round.

6. **FIX 2 — Regex character class correctness.** REFUTED. The class `[A-Za-z0-9._~:/?#\[\]@!$&'()*+,;=%-]*` covers:
   - All RFC 3986 `unreserved`: `A-Za-z0-9` + `-._~` (hyphen at end of class, idiomatic Go regex).
   - All `gen-delims`: `:/?#[]@`.
   - All `sub-delims`: `!$&'()*+,;=`.
   - `%` for percent-encoding.
   The class deliberately excludes whitespace and ESC (`\x1b`) so ANSI sequences appended by claude's TUI styling do not pollute the URL. **No counterexample found.** `TestLineScannerStripsANSIFromOAuthURL` (`claude_auth_test.go:893-916`) pins the ANSI-strip behavior with the input `cleanURL + "\x1b[0m\n"`. Test passes; the `\x1b` byte is outside the class so the regex correctly terminates the match at the first invalid char.

7. **FIX 2 — Regex ReDoS / catastrophic backtracking.** REFUTED. Go's `regexp` package uses RE2 (linear-time, no backtracking). A `[…]*` class can never produce catastrophic backtracking. EXHAUSTED.

8. **FIX 3 — Race condition under concurrent stdout+stderr writes.** REFUTED. Two separate `lineScanner` instances each own their own `buf` / `urlBuf` (no shared mutable state between scanners). The shared closure mutates only `sync.Once` (goroutine-safe by definition) and calls `opener.Open` (assumed goroutine-safe). `mage testPkg ./internal/cli` ran with `-race` and passed cleanly (170/170 tests, race-clean).

9. **mage gate.** All evidence-gathering used `mage testPkg ./internal/cli` — no raw `go test` invocations. Result: 170 tests pass, 72.7% coverage, race-clean. The green test suite does NOT exercise BLOCK 1 because the existing line-wrap test bypasses the production `sync.Once` wrapper.

### Routing

- **BLOCK 1 → builder for Unit 7.11 R3.** The production `sync.Once` + lineScanner `onMatch` interaction defeats the line-wrap fix. Builder must restructure the open-exactly-once guard to fire on the LONGEST match, not the FIRST. Suggested remediation (d) above is the cleanest IMO; let the builder pick.
- **CONCERN 2 → builder for R3 OR triage with dev.** 4 KiB buffer cap + long preamble + line-wrap interaction. Likely fine in practice but worth empirical validation against the dogfood log before deciding.
- **CONCERN 3 → builder for R3.** Add a `TestLineScannerWithSyncOnceLatchesLongest`-style regression test that mirrors production wiring so the fix can't silently regress.
- **FIX 1 + FIX 2 → PASS.** No remediation needed.

### Unknowns

- **U1 — Empirical claude-CLI wrap behavior.** Confirming the bug as user-observable requires reproducing in a narrow TTY (terminal width < URL length). The PLAN.md "Path B in-container auth WORKS end-to-end" note suggests the dev's terminal is wide enough that wrapping doesn't trigger. A narrow-pty dogfood pass would harden the evidence — but the code-level counterexample is independent of empirical verification.
- **U2 — Whether the partial URL `?code=foo` (or its real equivalent) opens a valid-looking but broken claude.com page**, or whether claude.com returns a friendly error. UX impact gradient TBD; the correctness gap is independent of UX gradient.

### Hylla Feedback

N/A — Hylla snapshot remains stale for Unit 7.11 R2's `claude_auth.go` changes (no reingest since Unit 7.5). All direct content reads went through `Read` and `git show` per mid-drop evidence protocol. Stdlib `sync.Once` semantics verified via Context7 `/golang/go`. No Hylla query attempted for this round; no misses to record.

---
