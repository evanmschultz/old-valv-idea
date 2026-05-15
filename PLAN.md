# Valv — Drop-Tree Index

This file is the **drop-tree index** for Valv. It names the ten containers, records each drop's state plus `blocked_by` edge, and points at the drop's durable work directory. It is **not** an execution plan — per-drop atomic units, acceptance criteria, and round history live in each drop dir's own `PLAN.md`. The canonical per-drop lifecycle (plan → plan-QA → discuss → build → build-QA → verify → closeout) lives in `main/drops/WORKFLOW.md`. Cross-drop implementation detail (slice ordering, scope derivation, ToS constraints, later-drop cleanup backlog) lives in `main/VALV_CLAUDE_CODE_FOCUS_PLAN.md`.

## Reading Order

- **This file** — what drops exist, what state they are in, what blocks what.
- **`main/drops/WORKFLOW.md`** — how one drop runs, start to finish. Authoritative for phase order, file lifecycles, agent spawn contract, restart recovery.
- **`main/VALV_CLAUDE_CODE_FOCUS_PLAN.md`** — the focus plan that seeded the original scope text. The renumbering below (DROP_5..DROP_8 collapsed around a dogfood-vs-v0.1.0 milestone split per dev directive 2026-05-14) supersedes focus-plan §6's slice numbering; the substantive scope of each focus-plan section is preserved.
- **`main/drops/DROP_N_<NAME>/PLAN.md`** — atomic units, paths, packages, acceptance criteria, `blocked_by` edges for one drop. Active drop: `main/drops/DROP_5_CLAUDE_LAUNCHER/`.

## Conventions

- Container-level `state`: `todo` | `planning` | `building` | `done` | `blocked`. Only the active drop carries a non-`todo` / non-`done` value at any given time.
- `blocked_by` is the cross-drop dependency edge. Every drop past DROP_0 is blocked by the drop immediately preceding it (dry-run-first: DROP_0 must close before any code-affecting drop begins, and every subsequent slice depends on the previous slice's compile-safe boundary per `main/VALV_CLAUDE_CODE_FOCUS_PLAN.md` §6).
- Scope text is one sentence — the briefest accurate handle. The drop dir is the place to expand on it.
- Per-drop dir links for DROP_1..DROP_9 are forward-looking stubs — the directories get scaffolded from `main/drops/_TEMPLATE/` at that drop's Phase 1 per `main/drops/WORKFLOW.md`.

## Drop Tree

| Drop | State | Blocked By | Scope | Dir |
|---|---|---|---|---|
| DROP_0_DOCS_BOOTSTRAP | state: done | blocked_by: — | Rebrand orchestrator docs and seed the six durable Phase-7 artifacts so every later drop has a landing spot. | [dir](drops/DROP_0_DOCS_BOOTSTRAP/) |
| DROP_1_DELETE_API_WRAPPER | state: done | blocked_by: DROP_0 | Strip the legacy OpenAI-compat API wrapper and its doc trail (`internal/api/openai`, `internal/services/openaiapi`, `internal/cli/api.go`, `compatibility.go` / `compatibility_test.go`, `codex-openai-compatibility.json`) per `VALV_CLAUDE_CODE_FOCUS_PLAN.md` §2 so `valv api` is gone and tests are green. | [dir](drops/DROP_1_DELETE_API_WRAPPER/) |
| DROP_2_DOMAIN_PLUMBING_PROVIDER_CLAUDE | state: done | blocked_by: DROP_1 | Extend the `Provider` domain type with `ProviderClaude` and thread compile-safe stubs through `ParseProvider`, `DefaultHostProfile`, `account_auth.go`, and `manage.go` `allProviders` per `VALV_CLAUDE_CODE_FOCUS_PLAN.md` §6.2 — no Docker, no Claude CLI yet. | [dir](drops/DROP_2_DOMAIN_PLUMBING_PROVIDER_CLAUDE/) |
| DROP_3_SCHEMA_MIGRATION_COMPOSITE_BINDING | state: done | blocked_by: DROP_2 | Migrate `project_bindings` to composite primary key `(project_id, provider)` with a forward-only `PRAGMA user_version`-gated rebuild, update `BindingRepository` and every call site, and prove the migration preserves existing Codex rows per `VALV_CLAUDE_CODE_FOCUS_PLAN.md` §6.2a. | [dir](drops/DROP_3_SCHEMA_MIGRATION_COMPOSITE_BINDING/) |
| DROP_4_CLAUDE_DOCKER_IMAGE | state: done | blocked_by: DROP_3 | Add `DefaultClaudeDockerfile` plus `WriteDefaultClaudeContext` with a pinned Claude CLI version, wire `images.Service` provider dispatch, and teach `valv manage update --provider claude` to produce `valv-claude:dev` per `VALV_CLAUDE_CODE_FOCUS_PLAN.md` §6.3. | [dir](drops/DROP_4_CLAUDE_DOCKER_IMAGE/) |
| DROP_5_CLAUDE_LAUNCHER | state: done | blocked_by: DROP_4 | **Dogfood milestone.** Add `internal/adapters/providers/claude/{profile.go, account.go, runtime.go}` + `internal/services/claude/service.go` + `internal/cli/claude.go`, wire `newClaudeCommand` into the `runtime` group, flip the DROP_2 `DefaultHostProfile` stub to real `claudeprovider.DefaultHostProfile`, and add a small `mage install` target. After close: `valv claude` runs per-project with `claude --resume` working via session state in the bind-mounted account home. Collapses focus-plan §6.4 + §6.5. | [dir](drops/DROP_5_CLAUDE_LAUNCHER/) |
| DROP_6_FORCE_OAUTH_ACCOUNT_ADD | state: building | blocked_by: DROP_5 | **Claude auth flow + credentials display** (narrowed scope 2026-05-15). Adds `ensureClaudeAccountReady` (container-based device-code auth for Claude — replaces the DROP_2 `return nil` stub in `account_auth.go`) and extends Claude `ReadAccountIdentity` from presence-only to parse-and-extract so `account list` / `account inspect` show real email. Codex hardening (force-wipe at `account add`, `--force-relogin` flag) deferred to DROP_9 after dev verified Codex code is correct as-is via clean DB re-add showing two distinct ChatGPT identities. | [dir](drops/DROP_6_FORCE_OAUTH_ACCOUNT_ADD/) |
| DROP_7_GLOBALSWITCH_AND_TUI_PARITY | state: todo | blocked_by: DROP_6 | **v0.1.0 requirement.** Extend `internal/services/globalswitch/service.go` to handle Claude (symlinks `~/.claude` to the active managed account home mirroring Codex), plus `valv account list` cross-provider, `valv account switch` cross-provider with `--provider` required on name collision, and full `tui/manage/picker.go` golden parity. Closes the remainder of focus-plan §6.6. | [dir](drops/DROP_7_GLOBALSWITCH_AND_TUI_PARITY/) |
| DROP_8_E2E_AND_RELEASE | state: todo | blocked_by: DROP_7 | **v0.1.0 tag.** Add `internal/cli/claude_integration_test.go`, rewrite `README.md` around "two providers, pass-through only", scrub residual OpenAI-compat prose from `AGENTS.md` and `valv_architecture_notes.md`, verify `go install github.com/evanmschultz/valv/cmd/valv@v0.1.0` produces a working binary, optionally seed a homebrew formula, tag `v0.1.0`. Per `VALV_CLAUDE_CODE_FOCUS_PLAN.md` §6.7. | [dir](drops/DROP_8_E2E_AND_RELEASE/) |
| DROP_9_CLEANUP_BACKLOG | state: todo | blocked_by: DROP_8 | Post-v0.1.0. Work down the `VALV_CLAUDE_CODE_FOCUS_PLAN.md` §8 backlog (dynamic Claude version resolver, consumer-side interface placement, `internal/cli/manage.go` and `extended_test.go` splits, mock-heavy-test audit, `NewCodexVersionResolver` pattern mirror). | [dir](drops/DROP_9_CLEANUP_BACKLOG/) |

## Maintenance

- Update a drop's `state` cell here only at the moments specified in `main/drops/WORKFLOW.md` — Phase 1 (`todo` → `planning`), Phase 3 exit (`planning` → `building`), Phase 7 closeout (`building` → `done`), or on blocker discovery (`blocked`).
- Do **not** edit this file mid-build. Unit-level state lives in the drop dir's own `PLAN.md`; this file tracks container state only.
- The `mage plan-check` target (landing later per `main/CLAUDE.md`) diffs this table's container titles and states against `main/drops/*/PLAN.md` header states — keep them in lockstep.
