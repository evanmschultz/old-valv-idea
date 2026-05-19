# DROP_N — <NAME>

**State:** planning
**Blocked by:** <DROP_M> | —
**Paths (expected):** <broad file footprint at start; refined by planner>
**Packages (expected):** <broad Go package footprint>
**PLAN.md ref:** main/PLAN.md → DROP_N row
**Workflow:** main/drops/WORKFLOW.md
**Started:** YYYY-MM-DD
**Closed:** —

## Scope

<One paragraph. Lifted from main/PLAN.md container row + dev confirmation during Phase 1.>

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
