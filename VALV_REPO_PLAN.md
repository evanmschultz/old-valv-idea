# Valv Repo Plan (Style + Structure + Automation)

Date: 2026-03-23

## Context Read

Primary local context read:
- `valv_architecture_notes.md`

External reference repos cloned into `.tmp/`:
- `.tmp/autent` (`evanmschultz/autent`, HEAD `fd5053e`)
- `.tmp/ccswitch` (`ksred/ccswitch`, HEAD `36bb548`)
- `.tmp/claudebox` (`RchGrav/claudebox`, HEAD `a7799bb`)

Note on selection:
- `evanmschultz/ccswitch` and `evanmschultz/claudebox` were not found publicly.
- I used the top public matches above to proceed.

## What To Reuse vs Ignore

### Reuse from `autent`

- README style discipline:
  - Clear mission + non-goals
  - Layered architecture explanation
  - Explicit local command section (`just check`, `just ci`)
  - Versioning/release expectations
- `AGENTS.md` style:
  - Crisp boundaries and terminology
  - Architecture guardrails
  - Test/CI expectations
  - Clear do-not-do list
- `Justfile` and CI pattern:
  - `check` as fast gate
  - `ci` as canonical full gate
  - Bootstrap validation
  - Matrix compatibility checks + one canonical full gate job

### Reuse from `ccswitch`

- Human-readable README tone and examples
- Good "project structure" section for quick orientation
- Practical usage snippets

### Reuse from `claudebox`

- Strong docs discoverability (features, prerequisites, install, usage)
- "Multi-instance/project isolation" storytelling
- CI idea: separate test/lint/build concerns

### Explicitly ignore for Valv

- `autent` auth-domain semantics and package model (auth/session/grant/audit)
- Any architecture that couples Valv to auth-specific APIs
- Overly broad or clutter-prone repo layout patterns

## Proposed Clean Valv Directory Structure

```text
valv/
├── README.md
├── AGENTS.md
├── Justfile
├── .gitignore
├── .github/
│   └── workflows/
│       ├── ci.yml
│       └── release.yml
├── cmd/
│   └── valv/
├── internal/
│   ├── domain/          # provider-neutral core types + invariants
│   ├── app/             # use-cases and orchestration
│   ├── adapters/        # provider adapters (claude/codex/gemini)
│   ├── runtime/         # sandbox/container runtime integration
│   └── config/          # config loading/validation
├── api/
│   └── openapi/         # optional public API descriptions
├── docs/
│   ├── architecture/
│   ├── decisions/
│   └── operations/
├── examples/
└── scripts/
```

Why this stays clean:
- Separation between core behavior (`internal/domain`, `internal/app`) and integrations (`internal/adapters`, `internal/runtime`).
- Docs and operational guidance live under `docs/` rather than bloating root.
- Minimal root surface for fast onboarding.

## README Plan (Style Borrowed, Content Valv-Specific)

Planned README sections:
1. What Valv is (single paragraph)
2. What it does / does not do
3. Architecture summary (fresh/resume/ephemeral from local notes)
4. Provider model (profiles, projects, sessions)
5. Quick start commands
6. Local automation (`just check`, `just ci`)
7. Runtime and persistence notes
8. Terms/compliance note (single-user account usage)

## AGENTS.md Plan for Valv

Planned Valv `AGENTS.md` sections:
- Mission and boundaries (provider-router, not auth framework)
- Terminology and naming consistency
- Architecture rules (domain/app/adapters/runtime boundaries)
- Runtime safety defaults (fresh by default, explicit resume)
- Engineering standards
- Testing and CI expectations
- Workflow and release expectations
- Scope note for `.tmp/` (research only)

## Justfile + CI Plan

### Justfile targets (initial)

- `verify-bootstrap` (required files exist)
- `fmt` / `fmt-check`
- `test`
- `lint`
- `build`
- `check` (fast local/PR gate)
- `ci` (canonical full gate)

### CI workflow shape

- `ci.yml`
  - Trigger: push + pull_request
  - Matrix compatibility job(s): ubuntu + macOS (+ Windows if needed)
  - Canonical full gate job on ubuntu using `just ci`
- `release.yml`
  - Trigger on SemVer tags (`v*.*.*`)
  - Re-run canonical CI gate
  - Publish artifacts (if/when release tooling is added)

## Execution Plan (No Code Yet)

1. Confirm reference repos and style priorities.
2. Draft Valv `README.md` using the section template above.
3. Draft Valv `AGENTS.md` with strict boundary rules and CI/test expectations.
4. Add Valv `Justfile` and two minimal GitHub workflows (`ci.yml`, `release.yml`).
5. Sanity pass for root cleanliness and consistency.

## Clarifying Questions

1. Repo sources: should I keep `ksred/ccswitch` and `RchGrav/claudebox`, or do you want different owners for those two references?
2. CI matrix: do you want Valv CI on `ubuntu + macOS + windows`, or only `ubuntu + macOS` initially?
3. Release flow: do you want tag-triggered releases in the first pass, or CI-only until the API surface settles?
4. Root strictness: do you want any additional top-level files beyond `README.md`, `AGENTS.md`, `Justfile`, `.gitignore`, and `.github/`?
5. Docs split: should architecture decisions start as ADR files under `docs/decisions/`, or stay in one architecture doc at first?
