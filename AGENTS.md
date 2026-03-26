# Valv Agent Guide

This file defines required behavior for coding agents working in the `valv` repository.
Scope: this repository root and every child path beneath it.

## 1) Product Direction

`valv` is a macOS-first control plane for running AI CLIs inside Valv-managed Docker runtimes.

Current product direction:

- `valv/v1/api` is the canonical API namespace
- Codex is the first primary provider target
- `valv codex` is a pass-through containerized Codex launcher
- Valv management flows live outside the direct `valv codex` pass-through path
- global account switching is optional convenience functionality, not the core runtime model

Follow [PLAN.md](./PLAN.md) as the current product source of truth. If another local doc conflicts with `PLAN.md`, follow `PLAN.md` and surface the conflict.

## 2) Platform Scope

Current supported scope:

- macOS only
- Docker required
- no expectation of Linux or Windows support yet

Do not add cross-platform abstractions or compatibility work unless the user explicitly asks for it.

## 3) Repository Topology

This repo uses a bare-root Git layout with worktrees.

Required shape:

- the repository root is a bare Git repository
- day-to-day implementation work happens in the `main/` worktree
- agent-local work logs live under `.worklog/` in the active worktree
- `.worklog/` must stay gitignored
- root `.codex/config.toml` should point MCP tools such as `gopls mcp` at the `main/` worktree

Do not treat the bare root as the normal coding worktree.

## 4) Runtime Model

Preserve these architectural boundaries:

- Go control plane
- SQLite runtime metadata store
- `modernc.org/sqlite` only; do not introduce CGO SQLite dependencies
- Valv-managed provider profile homes
- Docker runtime isolation

For `valv codex`:

- pass all Codex args through as directly as possible
- do not reinterpret normal Codex CLI semantics unless the user explicitly asks
- avoid replacing Codex session/resume logic with Valv-specific logic
- let Valv provide runtime isolation, project/profile resolution, and logging

For API compatibility surfaces:

- route OpenAI-compatible providers through the OpenAI-compatible handler surface
- route Anthropic-compatible providers through a separate Anthropic-compatible handler surface
- do not conflate provider identity with protocol compatibility

## 5) Go Standards

Use clear, idiomatic Go.

Requirements:

- prefer small packages with focused responsibilities
- keep interfaces near the consumer, not in shared dump packages
- pass `context.Context` through runtime, API, storage, and long-running operations
- prefer explicit constructors over package-global mutable state
- prefer standard library types and behaviors unless there is a concrete reason not to
- keep structs and functions small enough to understand without scrolling through unrelated concerns
- use table-driven tests where they improve coverage and readability
- do not add abstraction layers that are not pulling real weight yet

## 6) Error Handling

Go error handling must be idiomatic and explicit.

Requirements:

- wrap errors with `%w` and useful context
- bubble errors up to the command/runtime boundary
- do not swallow failures
- do not hide underlying failures behind vague generic errors
- preserve enough context that logs and user-visible failures can be traced back to the root cause

Valv should prefer full boundary-context errors rather than partially handled silent failures.

## 7) Logging

Valv must have strong logging from the start.

Requirements:

- use `github.com/charmbracelet/log`
- add structured logs throughout runtime-critical paths
- include debug logging for profile resolution, project resolution, Docker lifecycle, storage operations, API execution, and provider process launch
- maintain practical local log cleanup/retention
- keep logs useful for troubleshooting local control-plane and MCP-style runtime issues

Users should usually see clean Fang/Lipgloss-rendered failures, while detailed operational context remains in structured logs.

## 8) CLI And TUI Stack

Use Charm v2 libraries only for modern CLI/TUI work in this repo.

Allowed:

- `charm.land/fang/v2`
- `charm.land/bubbletea/v2`
- `charm.land/lipgloss/v2`
- `bubbles` directly when needed

Do not use:

- `huh`
- legacy non-v2 Charm module paths

The direct CLI path should stay clean and predictable. The management surface may use Bubble Tea selectors and views.
Use `.tmp/blick` as the implementation reference for Fang v2 command structure, output policy, renderer separation, and consistent human/plain/json behavior.

CLI alias policy:

- support selective aliases only
- preferred short aliases are `h`, `m`, and `g`
- do not invent blanket one-letter aliases for every command
- avoid alias schemes that create ambiguity across commands like `status`, `serve`, and `switch`

Output policy:

- follow `blick` output patterns as closely as practical
- prefer deterministic, minimal human output
- prefer explicit empty states over silent emptiness
- prefer slim machine-readable JSON payloads over human-style wrapper envelopes
- reduce raw subprocess noise unless that subprocess output is the actual user-facing payload

## 9) Context7 First

Before planning, writing code, writing tests, doing QA, or fixing failed tests, each agent must use Context7 for the relevant library or framework documentation when a Context7 entry exists for the technology being touched.

Minimum rule:

- resolve the relevant library id first
- query the relevant docs before implementation or review
- if no relevant Context7 entry exists, say so briefly in the worklog and continue with the best primary source available

This applies to build agents and QA agents.

## 10) Shared Foundations First

Before broad parallel implementation, establish these shared packages and conventions:

- config loading and defaults
- logger setup and log path policy
- output/error rendering contracts
- core domain interfaces

Reason:

- all parallel tracks need the same config, logging, and rendering behavior
- these should be stable shared packages early, not retrofitted later
- consistency matters more than short-term speed here

## 11) Delivery Standards

Implementation is not done until all of these are true:

- the applicable plan scope is fully implemented
- nothing intentionally deferred is left undocumented
- all tests pass
- each package reaches at least 70% test coverage
- the implementation follows TDD expectations for new work
- user-visible behavior, logs, and docs are aligned

QA requirements:

- every build agent must have two independent QA subagents review completeness and quality
- QA must verify behavior, tests, and edge cases, not just read diffs
- failed QA or failed tests must be fixed before work is considered complete

Worklog requirements:

- every agent keeps its own running worklog under `.worklog/` in the active worktree
- worklogs should record plan, assumptions, commands, findings, and open issues concisely
- `.worklog/` is local-only and must remain gitignored

Testing standards:

- prefer real end-to-end and integration tests over mocks
- use `testcontainers-go` for real Docker-backed integration tests when the behavior under test crosses the runtime boundary
- keep Docker-backed integration tests in CI on Linux runners where Docker is the normal hosted path
- use real SQLite, real filesystem state, and real process execution where practical
- mocks, fakes, and stubs are allowed only when there is no practical real-environment option or when isolating a narrow pure-domain concern
- do not default to mock-heavy unit tests for runtime, provider, storage, Docker, or CLI launch-path behavior
- if a mock is introduced, document briefly in the worklog why a real test was not practical

## 12) Repository Standards

- keep root docs and guidance aligned with implementation
- prefer a clean repo layout with minimal root clutter
- use `Justfile` as the command source of truth when present
- keep CI and local command recipes aligned
- keep a clean dev-mode path that does not dirty the developer's real home directory during normal local checks
- prefer `just dev ...` flows for disposable local validation and `just build` for normal binary creation
- when using disposable dev-mode home directories, preserve access to the host Docker CLI configuration/plugins so Docker Desktop features such as `buildx` keep working
- after every push, run `gh run watch` for the triggered workflow and confirm the result before considering the push complete

Docker build standards:

- prefer modern `docker buildx build --load` behavior over legacy builder paths
- keep dev and default image tags separable when local development needs to avoid dirtying normal runtime state
- suppress avoidable package-manager noise in Docker image builds where practical, including npm update-notifier chatter

## 13) Sandbox And Go Tooling

Do not alter Go cache or module environment variables to work around sandbox restrictions.

Prohibited examples:

- `GOCACHE=... just ci`
- `GOCACHE=... go test ./...`
- ad hoc overrides of `GOCACHE`, `GOMODCACHE`, `GOPATH`, or similar Go env paths to bypass local environment constraints

Required behavior:

- use the normal system Go cache and normal local command paths
- keep `Justfile` recipes correct rather than wrapping them in sandbox workarounds
- if a Go command or test run fails because of sandbox restrictions, stop, report that clearly, and let the user run it or decide the next step
