# DROP_0 — DOCS BOOTSTRAP

**State:** planning
**Blocked by:** —
**Paths (expected):** `CLAUDE.md` (bare-root), `main/CLAUDE.md`, `main/drops/WORKFLOW.md`, `main/drops/_TEMPLATE/CLOSEOUT.md`, `main/PLAN.md`, `main/WIKI.md` (new), `main/LEDGER.md` (new), `main/REFINEMENTS.md` (new), `main/HYLLA_FEEDBACK.md` (new), `main/HYLLA_REFINEMENTS.md` (new), `main/WIKI_CHANGELOG.md` (new), `main/.gitignore` (append), `main/.worklog/.FROZEN` (new)
**Packages (expected):** none — docs-only drop, no Go packages touched
**PLAN.md ref:** main/PLAN.md → DROP_0 row (U0.5 will create the row in this same drop; until then this drop is self-seeded from dev direction)
**Workflow:** main/drops/WORKFLOW.md
**Started:** 2026-04-18
**Closed:** —

## Scope

Rebrand the three orchestrator docs that were copied verbatim from the `rak` project (a different Go codebase — a line-counting CLI) so they accurately describe Valv: bare-root `CLAUDE.md` (steward prompt), `main/CLAUDE.md` (work-orch prompt), and `main/drops/WORKFLOW.md` (per-drop lifecycle). Rewrite the sections of `main/CLAUDE.md` that are rak-specific (Project Structure package map, Import DAG, File Breakdown table, Tech Stack, Mage targets table) against Valv's actual layout, dependencies, and `magefile.go` targets. Bootstrap the six durable Phase 7 closeout artifacts (`WIKI.md`, `LEDGER.md`, `REFINEMENTS.md`, `HYLLA_FEEDBACK.md`, `HYLLA_REFINEMENTS.md`, `WIKI_CHANGELOG.md`) that WORKFLOW.md Phase 7 requires but that do not yet exist in Valv, so the first closeout (Drop 1) does not fail on missing files. Rewrite `main/PLAN.md` as the rak-shape ten-container drop tree seeded from `main/VALV_CLAUDE_CODE_FOCUS_PLAN.md` §6 slices (DROP_0 docs bootstrap, DROP_1 delete API wrapper, DROP_2 domain plumbing, DROP_3 schema migration, DROP_4 Claude image, DROP_5 Claude adapter, DROP_6 Claude service + CLI, DROP_7 account surface parity, DROP_8 e2e + docs, DROP_9 cleanup backlog). Freeze `main/.worklog/` as historical by appending it to `.gitignore` and dropping a `.FROZEN` marker, since the rak workflow replaces it with `main/drops/` as the tracking substrate and `magefile.go` already skips `.worklog/` from format checks. Drop 0 is an intentional dry-run of the workflow on a docs-only surface so any phase-mechanics bugs (Phase 7 file-append failures, Agent Spawn Contract preamble gaps) surface before the first code-affecting drop ships. No Go source is touched. `mage build` and `mage test` must still be runnable at drop-end even though no Go files change.

## Planner

<Filled by go-planning-agent in Phase 1. Atomic units of work below. Each unit's state is mutated in place by the builder during Phase 4. See main/drops/WORKFLOW.md § "Phase 1 — Plan" for deliverable rules.>

## Notes

Orchestrator-override note (Phase 1, Round 1): this drop was scaffolded from a **steward orchestrator** session at the bare-root per an explicit dev override, because the file that defines the steward-vs-work-orch boundary (`valv/CLAUDE.md`, currently copied verbatim from rak) is itself the subject of U0.1 in this drop. Starting with Drop 1, the rule is enforced normally: work orchestrators run from `main/`, stewards run from the bare-root and never spawn planner / builder / QA subagents.
