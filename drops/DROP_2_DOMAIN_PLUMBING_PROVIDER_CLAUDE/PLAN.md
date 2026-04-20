# DROP_2 — DOMAIN PLUMBING PROVIDER CLAUDE

**State:** planning
**Blocked by:** —
**Paths (expected):** `internal/domain/provider.go` (edit), `internal/domain/account_auth.go` (edit), `internal/domain/*_test.go` (edits + new tests), `internal/cli/manage.go` (edit — `allProviders`), plus any compile-sibling touches forced by the new enum value (`ParseProvider`, `DefaultHostProfile` stub, provider switch exhaustiveness)
**Packages (expected):** `internal/domain` (real edits — new enum value + stub), `internal/cli` (edit — `allProviders` extended), provider adapter packages NOT touched this drop (Codex unchanged, Claude adapter arrives in DROP_5)
**PLAN.md ref:** main/PLAN.md → DROP_2_DOMAIN_PLUMBING_PROVIDER_CLAUDE row
**Workflow:** main/drops/WORKFLOW.md
**Started:** 2026-04-19
**Closed:** —

## Scope

Extend the `Provider` domain type with `ProviderClaude` and thread compile-safe stubs through `ParseProvider`, `DefaultHostProfile`, `account_auth.go`, and `manage.go` `allProviders` per `VALV_CLAUDE_CODE_FOCUS_PLAN.md` §6.2 — no Docker work, no Claude CLI wiring, no provider-adapter package. Deliberately small: the goal is a tree that compiles and tests green with `ProviderClaude` as a recognized enum value plus whatever stub behavior is needed for every provider-dispatching switch to stay exhaustive without exploding at runtime for the new value. DROP_5 flips the stubs to real Claude adapter calls.

## Planner

<Filled by go-planning-agent in Phase 1. Atomic units of work below. Each unit's state is mutated in place by the builder during Phase 4. See main/drops/WORKFLOW.md § "Phase 1 — Plan" for deliverable rules.>

## Notes

- `VALV_CLAUDE_CODE_FOCUS_PLAN.md` §6.2 is the authoritative scope source; read it before decomposing.
- Stubs for `ProviderClaude` must be compile-safe AND test-safe: any existing test that enumerates `allProviders` or iterates `Provider` values must still pass, either by accepting the new value or by being extended with a table-driven entry. Planner should audit existing tests in `internal/domain` + `internal/cli` that enumerate providers and decide which need extension.
- `DefaultHostProfile` for Claude: this drop only needs a compile-safe stub that returns something reasonable (e.g. a zero-or-placeholder `domain.Profile` value, or an error like "claude host profile not implemented until DROP_5"). Planner should decide which form is less risky for Phase 5 QA — erroring is more honest but may break any call path that currently assumes `DefaultHostProfile` always succeeds; returning a placeholder may hide bugs. Default: error return, with every caller updated to propagate the error.
- Nothing in `internal/adapters/providers/` gets a `claude/` subdir in this drop — that is DROP_5 scope per the container plan.
- `valv-claude:dev` Docker image is NOT built in this drop — that is DROP_4 scope.
- Schema migration (`project_bindings` composite key) is NOT touched in this drop — that is DROP_3 scope.
