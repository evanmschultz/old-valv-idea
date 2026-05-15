# DROP_7 — HOST AUTH TOKEN FIX

**State:** planning
**Blocked by:** DROP_6 (done)
**Paths (expected):** `internal/cli/claude_auth.go` (rewrite — replace container-launch path with host-subprocess `claude setup-token` runner; add keychain-extract step), `internal/cli/claude_auth_test.go` (rewrite tests for the new flow), `internal/services/claude/service.go` (edit — read stored token from managed home, set `CLAUDE_CODE_OAUTH_TOKEN` env on container launch), `internal/services/claude/service_test.go` (edit — verify env-var threading), `internal/adapters/providers/claude/account.go` (edit — `ReadAccountIdentity` recognizes the stored-token file as the `LoggedIn` signal; keep `.claude.json` email extraction as-is), `internal/adapters/providers/claude/account_test.go` (edit), `internal/cli/preflight.go` or similar (new pre-flight check that `claude` CLI is on host PATH — mirrors how Codex requires `codex` on PATH).
**Packages (expected):** `internal/cli` (rewrite + edits), `internal/services/claude` (edit), `internal/adapters/providers/claude` (edit).
**PLAN.md ref:** main/PLAN.md → DROP_7_HOST_AUTH_TOKEN_FIX row
**Workflow:** main/drops/WORKFLOW.md
**Started:** 2026-05-15
**Closed:** —

## Scope

**Codex parity for Claude auth.** DROP_6.2's container-side `claude auth login` flow has invisible paste prompt (TUI mode doesn't render through Docker pty). User verified on 2026-05-15 that host-side `claude setup-token` works perfectly: browser auto-opens, paste prompt clearly visible, completes cleanly. This drop replaces the container-auth flow with host-subprocess auth mirroring Codex's existing pattern.

**Design summary:**
- `valv account add claude <name>` runs `claude setup-token` as a host subprocess with `CLAUDE_CONFIG_DIR=<managed-home>` env set (structural mirror of Codex's `systemCodexAccountAuthRunner.Login` which runs `codex login` with `CODEX_HOME=<managed-home>`).
- Host browser opens automatically (claude CLI's `open` works because we're on host, not container).
- User completes auth in browser, pastes code back into terminal — paste prompt is visible (verified 2026-05-15).
- `claude setup-token` writes credentials to macOS keychain (`Claude Code-credentials` service, account = macOS username) — same behavior as `claude auth login`. `CLAUDE_CONFIG_DIR` only controls config/state location, NOT credentials on macOS (verified 2026-05-15 — `.credentials.json` did not appear in the test dir).
- **Immediately after the auth subprocess exits**, Valv extracts the token from keychain via `security find-generic-password -s "Claude Code-credentials" -a "$(id -un)" -w` and writes it to `<managed-home>/.credentials.json` as a JSON file (single-key `{"claudeAiAccessToken": "<token>"}` — matches the format DROP_5's `.credentials.json` presence-check assumed).
- Multi-account isolation: each `valv account add claude <name>` does auth → extract → store in a sequence, BEFORE the next auth would overwrite the keychain entry. Per-Valv-account `.credentials.json` files survive on disk independent of the keychain.
- `valv claude` launch (`services/claude/service.go::Run`) reads `<managed-home>/.credentials.json`, extracts the token, sets `CLAUDE_CODE_OAUTH_TOKEN=<token>` env var on the container.
- Pre-flight check: at `account add claude` time, verify `claude` is on host PATH; if not, print remediation `npm install -g @anthropic-ai/claude-code@2.1.89`. Mirrors how Codex's preflight requires `codex` on PATH.

**Removes from DROP_6.2:** the container-launch code path (the `valv-claude:dev` container running `claude auth login`). The new host-subprocess approach replaces it entirely. `claude_auth.go` is heavily reworked, NOT extended.

**Keeps from DROP_6.3:** `.claude.json` parsing for email extraction in `ReadAccountIdentity`. Host `claude setup-token` still writes `.claude.json` with `oauthAccount.emailAddress` to `$CLAUDE_CONFIG_DIR` — that data is still useful for `account list` display.

## Dev-Confirmed Findings (2026-05-15)

1. **Host-side `claude auth login` works correctly** with `CLAUDE_CONFIG_DIR` set: browser opens, "Paste code here if prompted >" prompt is visible, `Login successful.` printed.
2. **`CLAUDE_CONFIG_DIR` does NOT redirect credentials on macOS.** Credentials land in keychain regardless. `.claude.json` (identity + state, NO tokens) is written to the env-var-set dir.
3. **Keychain confirmed**: `Claude Code-credentials` service, account = macOS username (`evanschultz`), class `genp`. Extractable via `security find-generic-password -s "Claude Code-credentials" -a "$USER" -w`.
4. **Per-OS-user keychain scope**: one claude credential per macOS user — overwrites on each `claude auth login`. Multi-account isolation requires Valv to extract+store between auths.
5. **`claude setup-token` is the better UX**: ASCII art welcome, browser opens, URL fallback printed, paste prompt visible. Long-lived headless token with `scope=user:inference`. Anthropic's blessed headless path per focus-plan §7.
6. **claudebox (https://github.com/RchGrav/claudebox) uses in-container auth**: bind-mounts `.claude/` + `.claude.json` per slot, uses `-it` only if TTY, runs `claude` (no subcommand) and lets it auto-prompt. We are NOT following that approach — host-side auth gives cleaner Codex parity.

## Open Design Questions Routed To Planner

- **Q1 — What does `claude setup-token` actually print to stdout after the user pastes the code?** Possibilities: (a) just `Success` with token written to keychain only — extract via `security`; (b) prints `Token: ...` on stdout — capture there too; (c) writes to a file in `$CLAUDE_CONFIG_DIR`. The planner should NOT assume; the builder will resolve via real run during build verification. Per the 2026-05-15 test pattern, prefer keychain extraction as the primary capture method since we know it ends up there regardless.
- **Q2 — `setup-token` vs `auth login`?** `setup-token` produces a long-lived scoped token (`user:inference`); `auth login` produces a regular session token. For Valv's "per-account persistent credential in managed home" pattern, the long-lived token is the right choice. Confirm with the builder that `CLAUDE_CODE_OAUTH_TOKEN=<setup-token-output>` works to authenticate `claude` invocations inside the container.
- **Q3 — `CLAUDE_CODE_OAUTH_TOKEN` env-var visibility in `ps`.** Passing the token via env var makes it visible to anyone who can `ps eww` the container's process. Acceptable for v0.1.0 (same constraint Codex has with `CODEX_HOME` mount). Document.
- **Q4 — What service name does `claude setup-token` use in keychain?** Probably the same `Claude Code-credentials` as `auth login` but the planner should verify by having the builder check post-auth keychain state.

## Planner

<Filled by go-planning-agent in Phase 1. Atomic units of work below. Single planner pass per the trimmed cascade rule (this is a mechanical drop — replacing one host-subprocess pattern with another, mirroring an existing Codex template). Per-unit build-QA still applies in full.>

## Notes

- **Mechanical drop — trimmed cascade applies** per memory `feedback_trimmed_cascade_for_mechanical_drops.md`. Single planner spawn. Per-unit build-QA stays. The "novel" part is keychain extraction, which is shell-out + parse — no concurrent/async/state-machine logic.
- **Codex auth template to mirror**: `internal/cli/account_auth.go::systemCodexAccountAuthRunner.Login` runs `codex login` as a host subprocess with `CODEX_HOME=<path>` env. Our new `systemClaudeAccountAuthRunner.Login` should mirror this shape with `claude setup-token` + `CLAUDE_CONFIG_DIR=<path>` env, plus the post-exit keychain-extract step.
- **Test injection pattern**: keep the `claudeAccountAuthRunnerKey` context-key + interface pattern from DROP_6.2. The interface methods need updating from `EnsureImage` / `RunContainer` (container-based) to something like `RunSetupToken(ctx, homePath, stdin/out/err)` + `ExtractKeychainToken(ctx, account)`. Tests inject a stub that simulates both.
- **Code to DELETE in this drop**: the container-launch path in `claude_auth.go` (the `valv-claude:dev` invocation in `ensureClaudeAccountReady`). The image is still needed for `valv claude` LAUNCH, just not for auth.
- **`logoutManagedAccount` Claude case**: keep as file-wipe of `.credentials.json` from DROP_6.2. But also consider clearing the keychain entry — possibly with `security delete-generic-password -s "Claude Code-credentials" -a "$(id -un)"`. Planner should decide whether to do this (defensive cleanup) or leave it (less destructive of user's host-claude state).
- **`.credentials.json` schema**: write as `{"claudeAiAccessToken": "<token>"}` — single-key JSON matching the test fixture at `internal/adapters/providers/claude/account_test.go:14`. The Linux container's claude CLI reads this format. `ReadAccountIdentity` already checks file presence; it doesn't need to parse the JSON to determine LoggedIn — but DROP_6.3 added `.claude.json` parsing for email, which we keep.
- **`CLAUDE_CODE_OAUTH_TOKEN` env at launch**: when `valv claude` launches the container, the existing `services/claude/service.go::Run` builds an env map. Add a step that reads `<managed-home>/.credentials.json`, extracts `claudeAiAccessToken`, sets `CLAUDE_CODE_OAUTH_TOKEN=<value>` in the env map. Container claude will use the env-var auth.
- **Pre-flight host PATH check**: at `valv account add claude` time, before launching the subprocess, verify `claude` is on PATH. If not, error with: `claude CLI not found on PATH. Install with: npm install -g @anthropic-ai/claude-code@2.1.89`.
- **Out of scope**:
  - The container-LAUNCH path (`valv claude` from a bound project) keeps working as DROP_5 designed. We just add the env-var threading.
  - Cross-provider `account switch` semantics, TUI parity, globalswitch — all DROP_8.
  - Codex hardening (force-relogin flag, etc.) — DROP_10 cleanup.
  - GitHub Actions release + brew formula — DROP_9.
