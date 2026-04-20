# DROP_1 — DELETE API WRAPPER

**State:** planning
**Blocked by:** —
**Paths (expected):** `internal/api/openai/`, `internal/services/openaiapi/`, `internal/cli/api.go`, `internal/cli/compatibility.go`, `internal/cli/compatibility_test.go`, `codex-openai-compatibility.json`, any `cmd/valv` wiring that mounts `valv api`, any README/AGENTS/WIKI prose referring to the API wrapper
**Packages (expected):** `internal/api/openai`, `internal/services/openaiapi` (both deleted); `internal/cli`, `cmd/valv` (edits only — remove wiring, keep the rest)
**PLAN.md ref:** main/PLAN.md → DROP_1_DELETE_API_WRAPPER row
**Workflow:** main/drops/WORKFLOW.md
**Started:** 2026-04-19
**Closed:** —

## Scope

Strip the legacy OpenAI-compat API wrapper and its doc trail (`internal/api/openai`, `internal/services/openaiapi`, `internal/cli/api.go`, `compatibility.go` / `compatibility_test.go`, `codex-openai-compatibility.json`, and any `cmd/valv` wiring that mounts `valv api`) per `VALV_CLAUDE_CODE_FOCUS_PLAN.md` §2 so `valv api` is gone and tests are green. Preserve every other code path: Codex-in-Docker with profiles, Claude runtime scaffolding, `internal/adapters/docker`, `internal/services/codex`, `internal/services/images`, `internal/services/manage`, `internal/adapters/providers/codex`, account management, TUI, and mage targets all stay. Scrub residual API-wrapper prose from `README.md`, `AGENTS.md`, `main/CLAUDE.md`, `main/WIKI.md`, `main/drops/WORKFLOW.md`, and `valv_architecture_notes.md` — any mention of "OpenAI-compatible HTTP", the `openai` HTTP surface, `valv api serve`, the compatibility manifest, or the `openaiapi` service goes. Planner confirms exact path list against current `git ls-files` before decomposing into units. `mage test` must be green at drop-end (Phase 6); deleting `internal/services/openaiapi` self-resolves one of DROP_0's two coverage-gate failures. Driver for this drop is TOS: running a local OpenAI-compat proxy conflicts with provider terms; pass-through to each vendor's own CLI (Codex today, Claude next drop) replaces it.

## Planner

<Filled by go-planning-agent in Phase 1. Atomic units of work below. Each unit's state is mutated in place by the builder during Phase 4. See main/drops/WORKFLOW.md § "Phase 1 — Plan" for deliverable rules.>

### Unit N.1 — <title>

- **State:** todo
- **Paths:** <file-level footprint>
- **Packages:** <Go package footprint>
- **Acceptance:** <yes/no-verifiable criteria a QA subagent can call>
- **Blocked by:** —

### Unit N.2 — <title>

- **State:** todo
- **Paths:**
- **Packages:**
- **Acceptance:**
- **Blocked by:** N.1

<…repeat per unit…>

## Notes

<Optional. Cross-unit decisions, library choices made during planning, deferrals to later drops.>
