# DROP_6 — FORCE OAUTH ACCOUNT ADD

**State:** planning
**Blocked by:** DROP_5 (done)
**Paths (expected):** `internal/cli/account_auth.go` (edit — flip `ensureManagedAccountReady` `case domain.ProviderClaude` no-op stub to real `ensureClaudeAccountReady` call; audit `ensureCodexAccountReady` for credential-reuse path), `internal/cli/claude_auth.go` (new — Claude-side container-based device-code auth flow; or fold into `account_auth.go` if small), `internal/adapters/providers/claude/account.go` (edit — bump `ReadAccountIdentity` from presence-only to parse-and-extract email from `.credentials.json`), `internal/adapters/providers/codex/account.go` (audit — verify JWT email extraction is using the actual file in the managed dir, not a cached/inherited identity), `internal/cli/manage.go` and/or `internal/cli/account.go` (edit — `account add` clears any pre-existing creds in target home before launching auth; add `--force-relogin` flag to `account login`), tests alongside each.
**Packages (expected):** `internal/cli` (edits + possible new file), `internal/adapters/providers/claude` (edit), `internal/adapters/providers/codex` (audit + possible edit).
**PLAN.md ref:** main/PLAN.md → DROP_6_FORCE_OAUTH_ACCOUNT_ADD row
**Workflow:** main/drops/WORKFLOW.md
**Started:** 2026-05-15
**Closed:** —

## Scope

**Identity correctness.** Today `valv account add codex hylla` produces an account that decodes to the same Auth0 sub as the host `~/.codex` account (confirmed via JWT inspection 2026-05-15). The bug is that account-add does not force a fresh OAuth flow — it copies / inherits an existing host session into the managed home. This drop fixes that for both providers: `valv account add <provider> <name>` always launches a browser OAuth flow at add-time, requires the user to interactively re-authenticate, and the resulting credentials are guaranteed to be from a fresh login (not a copy of any existing file on the host).

For Claude this is net-new code: an `ensureClaudeAccountReady` container-based device-code flow that mounts the named account home into a `valv-claude:dev` container, runs `claude` with a login-forcing entrypoint, the user sees a device-code URL on stdout, completes auth in browser, credentials get written to the bind-mounted dir, container exits, account is fully authed.

Also: bump Claude `ReadAccountIdentity` from presence-only check to parse-and-extract so `account list` and `account inspect` show real email instead of `(unavailable)`. Add `--force-relogin` flag to `account login` so existing accounts can be re-authed without renaming.

## Dev-Confirmed Decisions And Diagnosis (2026-05-15)

1. **Account-add always forces fresh OAuth.** No silent credential reuse from host or other accounts. Browser must open every time. User must complete auth interactively. Applies to both providers.
2. **`--force-relogin` flag** on `account login` extends the same semantics to existing accounts.
3. **Email must be visible AND correct everywhere** (`account list`, `account inspect`, post-add output). For both providers.
4. **Codex same-identity bug confirmed.** SQL inspection 2026-05-15:
   - 5 rows in `profiles` table; 3 are orphans (`host`, `host-codex`, duplicate `personal`) all pointing at `~/.codex` with no project bindings.
   - The Valv-managed Codex profile `hylla` has home `.../profiles/personal/` (directory name mismatches account name — stale artifact).
   - JWT inspection: `~/.codex/auth.json` and `.../profiles/personal/auth.json` differ byte-for-byte (different `last_refresh` timestamps and token strings) BUT both decode to `email: evan@hylla.io`, `sub: auth0|CiE2WP2jSqixM4aF9uLyrVv7`. Same OAuth identity. Confirms the credential-reuse / seed-from-host bug at `account add` time.
5. **DB orphan cleanup is out of scope for this drop.** The 3 dead profile rows + dir-name-mismatch on `hylla` are cosmetic and can be cleaned up with the existing `valv account cleanup` and `valv account delete` flows after DROP_6 ships, using the fresh `--force-relogin` path. Do not auto-mutate the user's local DB in this drop.

## Open Design Question Routed To Planner

- **Device-code URL UX through Valv's attached Docker subprocess.** Claude's device-code flow prints a URL + code to stdout and waits for browser completion. Valv's `runClaudeImageOnlyCommand` and `services/claude/service.go` `Run` already do TTY-attached pass-through, so this should "just work" — but the planner MUST verify by reading the actual flow in `services/codex/service.go::Run` and `internal/adapters/dockeradapter` to confirm: (a) container stdin/stdout/stderr are wired to host TTY, (b) the user sees the URL in their terminal, (c) the container blocks on the device-code wait (not on TTY read), and (d) when auth completes, the container's claude process detects it and exits, releasing the parent. If any of (a)–(d) is wrong, the planner surfaces it as an issue to be resolved in the planner pass before the builder spawns.

## Planner

<Filled by go-planning-agent in Phase 1. Atomic units of work below. Trimmed cascade applies — single planner pass + per-unit build-QA. Orchestrator + dev review planner output before approval.>

## Notes

- **Mechanical-ish drop — trimmed cascade applies** per memory `feedback_trimmed_cascade_for_mechanical_drops.md`. Claude container-auth flow is new code, but it mirrors the existing Codex `ensureCodexAccountReady` shape adapted for in-container interaction. Single planner spawn, no parallel plan-QA. Per-unit build-QA stays in place.
- **Copy-adapt template for Claude auth flow.** `internal/cli/account_auth.go::ensureCodexAccountReady` is the structural template. Key differences: (a) Claude runs in a container, not on host; (b) auth is device-code, not browser-OAuth-from-host; (c) success criterion is `.credentials.json` appearing in the mounted dir.
- **Identity-extraction format for Claude `.credentials.json`.** v1 Claude adapter does presence-only check. Builder needs to read an actual `.credentials.json` (after running a real Claude device-code auth) to see the JSON shape. If the file contains a JWT, decode the same way Codex does. If it's a different shape (just an API key + email), parse accordingly. Builder may need to do the first real Claude auth as part of this work to capture the format.
- **Force-relogin semantics.** Before launching the auth flow, delete any pre-existing `auth.json` (Codex) or `.credentials.json` (Claude) in the target managed home. This guarantees no credential reuse. Applies whether the entry point is `account add <provider> <name>` (always force-fresh) or `account login` with `--force-relogin` flag (opt-in force-fresh on existing accounts).
- **Codex audit.** The current Codex flow may already do the right thing for fresh accounts — the bug is in how `hylla` was originally created (probably via early dev paths that copied `~/.codex/`). The planner MUST audit `ensureCodexAccountReady` for any code path that copies, symlinks, or otherwise inherits an existing host `auth.json` into the managed home, and excise it.
- **Coverage floor.** 70% per AGENTS.md target. Per-package floor enforced by mage at the current 60% (per `magefile.go:22` TODO note). New code must clear the 70% target.
- **Out of scope:** DB orphan cleanup (use existing `valv account cleanup`), TUI picker golden parity (DROP_7), globalswitch (DROP_7), integration test (DROP_8), brew formula / GitHub releases workflow (DROP_8).
