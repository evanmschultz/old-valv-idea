# DROP_8 — GLOBALSWITCH AND TUI PARITY (PLUS BINDING UX)

**State:** planning
**Blocked by:** DROP_7 (done)
**Paths (expected):** `internal/services/globalswitch/`, `internal/cli/manage.go` (or post-DROP_9 successor), `internal/cli/claude.go`, `internal/cli/codex.go`, `internal/tui/manage/picker.go` (and new picker for unbound-project bind selection), tests + golden fixtures. Planner refines.
**Packages (expected):** `internal/services/globalswitch`, `internal/cli`, `internal/tui/manage` (or new `internal/tui/bind/`). Planner confirms.
**PLAN.md ref:** main/PLAN.md → DROP_8_GLOBALSWITCH_AND_TUI_PARITY row
**Workflow:** main/drops/WORKFLOW.md
**Started:** 2026-05-18
**Closed:** —

## Scope

Lifted from `main/PLAN.md` DROP_8 row + the `project_valv_binding_ux_spec.md` memory:

**v0.1.0 requirement.** Extend `internal/services/globalswitch/service.go` to handle Claude (symlinks `~/.claude` to the active managed account home mirroring Codex), plus `valv account list` cross-provider, `valv account switch` cross-provider with `--provider` required on name collision, and full `tui/manage/picker.go` golden parity.

**Plus binding UX (added 2026-05-16):** when `valv claude` / `valv codex` runs in an unbound project — if exactly 1 account exists for the provider, auto-bind it; if 2+ exist, launch a Bubble Tea picker that binds the selection; if 0 exist, error pointing to `valv account add`. Add `--account <name>` flag to both commands for one-shot override that does NOT mutate the binding row. All behavior symmetric across providers.

Closes the remainder of focus-plan §6.6 + the dogfood binding-UX gap (dev hit "project is not bound" running `valv claude` in an unbound dir during DROP_7 R3 smoke test on 2026-05-17).

## Planner

<Filled by go-planning-agent in Phase 1. Atomic units of work below. Each unit's state is mutated in place by the builder during Phase 4. See main/drops/WORKFLOW.md § "Phase 1 — Plan" for deliverable rules.>

<!-- Planner: append `### Unit 8.N — <title>` blocks under this comment. Order them by likely build sequence; use `blocked_by` to encode dependencies. Each unit must list paths, packages, acceptance criteria, and state. -->

## Notes

- **Symmetry across providers is load-bearing.** Anything new added for Claude must have a Codex counterpart (and vice versa) unless explicitly justified. Globalswitch currently handles Codex via symlink to managed home; planner extends to Claude. `valv account list` and `valv account switch` need cross-provider semantics.
- **Pre-DROP_9 CLI surface assumption.** DROP_9 will normalize the command tree (delete `valv manage` namespace, add `valv account bind`/`unbind`, add `valv image` namespace, flatten `valv status`). DROP_8 lives BEFORE DROP_9, so it operates on the CURRENT command tree (`valv manage *` + `valv account *` registered at two paths via `newManageAccountCommand`). DROP_8 should NOT rename commands — DROP_9 owns that. DROP_8 adds NEW behavior to existing paths.
- **`--account <name>` flag is DROP_8 territory.** DROP_9 explicitly consumes it (per DROP_9 scope: "do NOT re-implement"). Planner ensures the flag is implemented cleanly so DROP_9 can route it through any rename.
- **Bubble Tea picker.** Existing `internal/tui/manage/picker.go` is the prior art. New picker for unbound-project bind selection may reuse, refactor, or be a sibling — planner's choice.
- **No re-introduction of auto-open.** Per `feedback_manual_workflow_is_the_decision.md`, the Claude OAuth flow stays manual (press `c` for clean copy + Ctrl-C × 2 to exit). DROP_8 does not touch that surface.
- **DROP_9 fold-in concerns.** Two non-blocking concerns from DROP_7 R3 close land in DROP_9, not DROP_8: (A) static `ContainerRunRequest` field assertions for auth tests; (B) harden `RunInContainer` against future non-zero exit on Ctrl-C × 2. DROP_8 stays focused on globalswitch + TUI parity + binding UX.
