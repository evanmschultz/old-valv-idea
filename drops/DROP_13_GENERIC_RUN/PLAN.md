# DROP_13 — GENERIC_RUN

**State:** planning
**Blocked by:** DROP_12 (done)
**Paths (expected):** `cmd/valv/` (new `run` cobra command), `internal/cli/` (new `run.go`), `internal/services/` (possible new `run` service or reuse of existing claude/codex services), `internal/adapters/providers/` (potential generic provider adapter or refactor of claude/codex into shared core)
**Packages (expected):** `internal/cli/`, possibly `internal/services/run/` (new), possibly refactors to `internal/adapters/providers/claude/`, `internal/adapters/providers/codex/`
**PLAN.md ref:** main/PLAN.md → DROP_13_GENERIC_RUN row
**Workflow:** main/drops/WORKFLOW.md
**Started:** 2026-05-22
**Closed:** —

## Scope

**Architectural pivot.** Add `valv run --account <name> <command>` generic primitive. `valv codex` and `valv claude` are re-derived as thin adapters over this primitive — same per-account isolation, mount, env, cross-provider routing, network policy semantics. README + help reflect the Product Direction rewrite in `main/CLAUDE.md` (per-account isolated containerized agentic-dev workloads, AI CLI launching as first-class case but not the only one).

The generic primitive must preserve everything DROP_5/7/8/10/12 added:

- Per-account credential homes under `~/Library/Application Support/valv/providers/<provider>/profiles/<account>/`
- Sibling-path-aware mounts (worktree gitdir handling)
- Cross-provider in-container routing (when both providers bound to a project)
- Project-binding-aware account resolution
- Per-project overlay image build (DROP_12 `EnsureProjectImage`)
- `VALV_<PROVIDER>_IMAGE` env override path
- Provider-specific CLI argument pass-through

The challenge: most of the existing claude/codex launchers are tightly coupled to provider-specific knowledge (image refs, auth flows, MCP overlay generation, cross-provider mount targets). The generic primitive needs a clean seam where "what to run inside the container" is the variable input, while everything around it (isolation, mounts, env, policy) is shared.

## Planner

(To be filled by planner — see WORKFLOW.md § "Phase 1 — Plan". Use Hylla MCP `github.com/evanmschultz/valv@main` at the post-DROP_12 ingest snapshot to map current launch-path coupling before proposing the generic seam.)

## Notes

(Filled during planning.)
