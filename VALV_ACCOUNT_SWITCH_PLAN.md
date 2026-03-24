# Valv Account-Switching Research Plan

Date: 2026-03-23

## What I Cloned

Added under `.tmp/`:
- `.tmp/ccx` (`evanmschultz/ccx`)
- `.tmp/claude-accounts` (`danmana/claude-accounts`)
- `.tmp/ccswitch-account` (`BNhashem16/ccswitch`, commit `43345e0`)

Notes:
- `ccx` is Go/TUI architecture-heavy, not a minimal shell switcher.
- `claude-accounts` is Node-based.
- `BNhashem16/ccswitch` matches your description best: a shell-first Claude account switcher.

## What The Shell Switcher Does Well (Inspiration)

From `.tmp/ccswitch-account`:
- Stores multiple account snapshots and rotates/switches accounts.
- Uses lock file for concurrency safety (`~/.claude-switch-backup/.lock`).
- Backs up and restores both credentials and account config (`oauthAccount`).
- Kills running Claude process before switching to avoid mixed state.
- Supports macOS Keychain and file fallback.

Relevant files:
- `.tmp/ccswitch-account/README.md`
- `.tmp/ccswitch-account/ccswitch.sh`

## Official Platform Facts That Matter

### Codex (official)

Sources:
- https://developers.openai.com/codex/auth
- https://developers.openai.com/codex/config-basic
- https://developers.openai.com/codex/config-reference

Key facts:
- Login cache lives in `~/.codex/auth.json` or OS keychain.
- `cli_auth_credentials_store = "file|keyring|auto"` controls storage.
- File storage uses `auth.json` under `CODEX_HOME` (default `~/.codex`).
- Project-scoped config is supported via `.codex/config.toml`.
- Config precedence includes `--profile`, then project `.codex/config.toml`, then user config.

Implication:
- Codex is straightforward to project-scope by controlling `CODEX_HOME` and profile config.

### Claude Code (official)

Sources:
- https://code.claude.com/docs/en/authentication
- https://code.claude.com/docs/en/settings

Key facts:
- Credential location:
  - macOS: keychain
  - Linux/Windows: `~/.claude/.credentials.json` (or `$CLAUDE_CONFIG_DIR`)
- Claude supports config scoping and project-level settings files.
- Auth precedence includes cloud/env/API key helper before OAuth login credentials.
- `/logout` is the official re-auth path.

Implication:
- Linux/Windows can be cleanly project-scoped with `CLAUDE_CONFIG_DIR`.
- macOS is trickier for OAuth isolation due to keychain behavior.

## Recommended Valv Design For Project-Scoped Accounts

## 1) Core model

- Keep credentials in Valv-managed profile homes:
  - `~/.valv/providers/codex/profiles/<profile>/`
  - `~/.valv/providers/claude/profiles/<profile>/`
- Keep project binding separate:
  - `~/.valv/projects/<project-id>/project.toml`
- Binding schema:
  - `provider = codex|claude`
  - `profile = work|personal|...`
  - `mode = fresh|resume|ephemeral`

## 2) Launch wrappers (important)

Do not rely on raw `codex` / `claude` invocations for isolation.
Use:
- `valv codex tui`
- `valv claude tui`

Wrapper behavior:
- Resolve current project from CWD.
- Load bound profile.
- Export provider-specific env before launching CLI:
  - Codex: `CODEX_HOME=<profile_home>`
  - Claude: `CLAUDE_CONFIG_DIR=<profile_home>` (plus policy vars as needed)

## 3) Directory-aware auto-switch

Use shell integration:
- `eval "$(valv shell hook zsh)"` or bash equivalent.
- On `cd`, detect project root and set `VALV_PROJECT` + provider env.

Optional mode:
- generate `.envrc` via `valv project envrc` for users already using `direnv`.

## 4) Login UX

- `valv profile login codex <profile>`
  - run `codex login` with `CODEX_HOME` set to profile home
  - enforce `cli_auth_credentials_store="file"` when strict per-project isolation is required
- `valv profile login claude <profile>`
  - run Claude login flow in profile-scoped environment
  - on Linux/Windows this maps cleanly to file-based credentials

Switching project account:
- `valv project set-profile --provider codex --profile work`
- `valv project set-profile --provider claude --profile personal`

No credential copy required; just change binding.

## 5) macOS caveat (Claude)

Because Claude OAuth is keychain-backed on macOS, strict per-project OAuth isolation may be imperfect with CLI-only env switching.

Mitigations:
- Prefer containerized Linux runtime for hard isolation.
- Or use API-key/API-helper based auth per project instead of shared OAuth session.

## Difficulty / Risk

- Codex per-project switching: low to medium.
- Claude Linux/Windows switching: medium.
- Claude macOS strict OAuth isolation: medium to high unless using containerized provider runtime.

## Phased Plan

1. Implement Valv profile/project binding model (no runtime changes yet).
2. Implement wrapper launchers for `codex` and `claude` with env scoping.
3. Add shell hook for automatic project-context activation on `cd`.
4. Add `valv profile login` and `valv project set-profile` commands.
5. Add containerized fallback path for strict Claude isolation on macOS.

## What To Reuse vs Avoid From ccswitch-account

Reuse:
- lock discipline
- safe backup/write patterns
- explicit process-stop before switch
- clear switch UX

Avoid:
- mutating global home credentials in place as the primary model
- provider-specific hardcoding in Valv core

Valv should isolate by profile home + binding, not by repeatedly rewriting one global credential file.
