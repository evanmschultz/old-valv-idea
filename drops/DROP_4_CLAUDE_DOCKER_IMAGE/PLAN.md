# DROP_4 — CLAUDE DOCKER IMAGE

**State:** planning
**Blocked by:** DROP_3 (done)
**Paths (expected):** `internal/adapters/docker/` (likely edit — new `DefaultClaudeDockerfile` + `WriteDefaultClaudeContext`), `internal/services/images/` (edit — provider dispatch for claude), `internal/cli/manage*.go` or equivalent (edit — `valv manage update --provider claude` route), tests alongside each
**Packages (expected):** `internal/adapters/docker` (real edit — add Claude Dockerfile constant + writer), `internal/services/images` (real edit — provider dispatch), `internal/cli` (possible edit — if the provider flag is not already wired)
**PLAN.md ref:** main/PLAN.md → DROP_4_CLAUDE_DOCKER_IMAGE row
**Workflow:** main/drops/WORKFLOW.md
**Started:** 2026-04-20
**Closed:** —

## Scope

Add `DefaultClaudeDockerfile` plus `WriteDefaultClaudeContext` with a pinned Claude CLI version, wire `images.Service` provider dispatch, and teach `valv manage update --provider claude` to produce `valv-claude:dev` per `VALV_CLAUDE_CODE_FOCUS_PLAN.md` §6.3. MVP scope — image build only; no Claude provider adapter, no service, no `valv claude` CLI (those are DROP_5 and DROP_6).

## Planner

<Filled by go-planning-agent in Phase 1. Atomic units of work below. Each unit's state is mutated in place by the builder during Phase 4. See main/drops/WORKFLOW.md § "Phase 1 — Plan" for deliverable rules.>
