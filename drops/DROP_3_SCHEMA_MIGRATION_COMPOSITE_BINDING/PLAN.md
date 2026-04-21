# DROP_3 — SCHEMA MIGRATION COMPOSITE BINDING

**State:** planning
**Blocked by:** —
**Paths (expected):** `internal/adapters/sqlite/store.go` (edit — migration + `BindingRepository` methods), `internal/adapters/sqlite/store_test.go` (edit — migration and repository tests), plus any call-site updates where the binding row shape is assumed to be project-keyed
**Packages (expected):** `internal/adapters/sqlite` (real edits — schema + repository), `internal/domain` (possible edit — if `ProjectBinding` needs a `Provider` field), plus any downstream caller (`internal/services/*`, `internal/cli/*`) that indexes bindings by `project_id` alone
**PLAN.md ref:** main/PLAN.md → DROP_3_SCHEMA_MIGRATION_COMPOSITE_BINDING row
**Workflow:** main/drops/WORKFLOW.md
**Started:** 2026-04-20
**Closed:** —

## Scope

Migrate the `project_bindings` table to a composite primary key `(project_id, provider)` with a forward-only `PRAGMA user_version`-gated rebuild, update `BindingRepository` and every call site, and prove the migration preserves existing Codex rows per `VALV_CLAUDE_CODE_FOCUS_PLAN.md` §6.2a. One project may bind one account per provider — today it is Codex-only (pre-migration state has at most one binding per project); after DROP_3 the shape supports simultaneous Codex + Claude bindings per project.

## Planner

<Filled by go-planning-agent in Phase 1. Atomic units of work below. Each unit's state is mutated in place by the builder during Phase 4.>
