# Valv — Drop-Tree Index

This file is the **drop-tree index** for Valv. It names the ten containers, records each drop's state plus `blocked_by` edge, and points at the drop's durable work directory. It is **not** an execution plan — per-drop atomic units, acceptance criteria, and round history live in each drop dir's own `PLAN.md`. The canonical per-drop lifecycle (plan → plan-QA → discuss → build → build-QA → verify → closeout) lives in `main/drops/WORKFLOW.md`. Cross-drop implementation detail (slice ordering, scope derivation, ToS constraints, later-drop cleanup backlog) lives in `main/VALV_CLAUDE_CODE_FOCUS_PLAN.md`.

## Reading Order

- **This file** — what drops exist, what state they are in, what blocks what.
- **`main/drops/WORKFLOW.md`** — how one drop runs, start to finish. Authoritative for phase order, file lifecycles, agent spawn contract, restart recovery.
- **`main/VALV_CLAUDE_CODE_FOCUS_PLAN.md`** — the focus plan that seeded the DROP_1..DROP_7 scope text below (see its §6 "Rollout Order"); DROP_8 and DROP_9 cover integration plus the §8 cleanup backlog respectively.
- **`main/drops/DROP_N_<NAME>/PLAN.md`** — atomic units, paths, packages, acceptance criteria, `blocked_by` edges for one drop. The active drop today is `main/drops/DROP_0_DOCS_BOOTSTRAP/`.

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
| DROP_2_DOMAIN_PLUMBING_PROVIDER_CLAUDE | state: todo | blocked_by: DROP_1 | Extend the `Provider` domain type with `ProviderClaude` and thread compile-safe stubs through `ParseProvider`, `DefaultHostProfile`, `account_auth.go`, and `manage.go` `allProviders` per `VALV_CLAUDE_CODE_FOCUS_PLAN.md` §6.2 — no Docker, no Claude CLI yet. | [dir](drops/DROP_2_DOMAIN_PLUMBING_PROVIDER_CLAUDE/) |
| DROP_3_SCHEMA_MIGRATION_COMPOSITE_BINDING | state: todo | blocked_by: DROP_2 | Migrate `project_bindings` to composite primary key `(project_id, provider)` with a forward-only `PRAGMA user_version`-gated rebuild, update `BindingRepository` and every call site, and prove the migration preserves existing Codex rows per `VALV_CLAUDE_CODE_FOCUS_PLAN.md` §6.2a. | [dir](drops/DROP_3_SCHEMA_MIGRATION_COMPOSITE_BINDING/) |
| DROP_4_CLAUDE_DOCKER_IMAGE | state: todo | blocked_by: DROP_3 | Add `DefaultClaudeDockerfile` plus `WriteDefaultClaudeContext` with a pinned Claude CLI version, wire `images.Service` provider dispatch, and teach `valv manage update --provider claude` to produce `valv-claude:dev` per `VALV_CLAUDE_CODE_FOCUS_PLAN.md` §6.3. | [dir](drops/DROP_4_CLAUDE_DOCKER_IMAGE/) |
| DROP_5_CLAUDE_PROVIDER_ADAPTER | state: todo | blocked_by: DROP_4 | Add `internal/adapters/providers/claude/{profile.go, account.go, runtime.go}` mirroring the Codex adapter, unit-test in isolation, and flip DROP_2's `DefaultHostProfile` stub to the real `claudeprovider.DefaultHostProfile` per `VALV_CLAUDE_CODE_FOCUS_PLAN.md` §6.4. | [dir](drops/DROP_5_CLAUDE_PROVIDER_ADAPTER/) |
| DROP_6_CLAUDE_SERVICE_AND_CLI | state: todo | blocked_by: DROP_5 | Add `internal/services/claude/service.go` plus `internal/cli/claude.go`, wire `newClaudeCommand` into the `runtime` group, and make `valv claude` pass-through launch end-to-end from a bound project root per `VALV_CLAUDE_CODE_FOCUS_PLAN.md` §6.5. | [dir](drops/DROP_6_CLAUDE_SERVICE_AND_CLI/) |
| DROP_7_ACCOUNT_SURFACE_PARITY | state: todo | blocked_by: DROP_6 | Extend `valv account add claude`, `valv account list`, `valv account switch`, and the `tui/manage/picker.go` goldens so both providers are symmetric at the user-facing surface per `VALV_CLAUDE_CODE_FOCUS_PLAN.md` §6.6. | [dir](drops/DROP_7_ACCOUNT_SURFACE_PARITY/) |
| DROP_8_E2E_AND_DOCS | state: todo | blocked_by: DROP_7 | Add `internal/cli/claude_integration_test.go`, rewrite `README.md` around "two providers, pass-through only", scrub residual OpenAI-compat prose from `AGENTS.md` and `valv_architecture_notes.md`, and tag this as the v1-Claude milestone per `VALV_CLAUDE_CODE_FOCUS_PLAN.md` §6.7. | [dir](drops/DROP_8_E2E_AND_DOCS/) |
| DROP_9_CLEANUP_BACKLOG | state: todo | blocked_by: DROP_8 | Work down the `VALV_CLAUDE_CODE_FOCUS_PLAN.md` §8 backlog (dynamic Claude version resolver, consumer-side interface placement, `internal/cli/manage.go` and `extended_test.go` splits, mock-heavy-test audit, `NewCodexVersionResolver` pattern mirror) as a single cleanup pass. | [dir](drops/DROP_9_CLEANUP_BACKLOG/) |

## Maintenance

- Update a drop's `state` cell here only at the moments specified in `main/drops/WORKFLOW.md` — Phase 1 (`todo` → `planning`), Phase 3 exit (`planning` → `building`), Phase 7 closeout (`building` → `done`), or on blocker discovery (`blocked`).
- Do **not** edit this file mid-build. Unit-level state lives in the drop dir's own `PLAN.md`; this file tracks container state only.
- The `mage plan-check` target (landing later per `main/CLAUDE.md`) diffs this table's container titles and states against `main/drops/*/PLAN.md` header states — keep them in lockstep.
