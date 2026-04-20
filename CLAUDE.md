# Valv — Project CLAUDE.md (main worktree)

This file lives in the **`main/` worktree** at `/Users/evanschultz/Documents/Code/hylla/valv/main/`. This is the primary work checkout — all real coding, building, testing, and committing happens here. **The dev launches work orchestrators from this directory.** Sessions launched from the bare-root one directory up are steward orchestrators with a different prompt (bare-root `CLAUDE.md`) and a different scope — cross-worktree oversight and merge-conflict help, not feature work.

## AGENTS.md — Authoritative For Cross-Cutting Rules

`/Users/evanschultz/Documents/Code/hylla/valv/main/AGENTS.md` is the **authoritative source of truth** for every cross-cutting Valv rule: product direction, macOS+Docker platform scope, repository topology, runtime model (Go control plane + `modernc.org/sqlite` + Docker isolation + Valv-managed profile homes), Go standards, error handling, logging (`github.com/charmbracelet/log`), the Charm v2 CLI/TUI stack, containerized Codex runtime rules, MCP translation rules, auth UX, Context7-first workflow, shared foundations, delivery standards (including the 70% per-package coverage floor), testing standards (real integration over mocks, `testcontainers-go`), repository standards, sandbox and Go tooling rules, and the OpenAI compatibility contract. This CLAUDE.md **defers to AGENTS.md** for all of those topics — it is the canonical spec and wins any conflict. This CLAUDE.md covers only the work-orchestrator role shape, agent bindings, drop coordination, and Go workflow discipline that sit above AGENTS.md.

Read AGENTS.md on every cold-start and after every compaction alongside WIKI.md, PLAN.md, and `main/drops/WORKFLOW.md`. When AGENTS.md and any other doc disagree, AGENTS.md wins and the other doc gets fixed.

## Coordination Model — At a Glance

Valv does **not** use Tillsyn. Three documents own the coordination model; they do not duplicate each other:

- **`main/PLAN.md`** — overarching drop tree (container drops + state + `blocked_by` + per-drop dir link). Updated *after* a drop closes or *after* a planner restructures the tree. Not edited mid-build.
- **`main/drops/WORKFLOW.md`** — canonical per-drop lifecycle (planner → plan-QA → discuss → revise → builder → build-QA → verify → closeout). Owns: drop directory shape, file lifecycles, phase order, the **Agent Spawn Contract** (preamble pasted into every subagent spawn), restart recovery.
- **`main/CLAUDE.md`** (this file) — orchestrator role boundaries, agent bindings, evidence sources, Go quality rules, mage discipline, commit format, safety. Does not own per-phase mechanics — those live in WORKFLOW.md. Does not own cross-cutting product/runtime rules — those live in AGENTS.md.

Per-drop work artifacts live under `main/drops/DROP_N_<NAME>/`. The directory is stamped from `main/drops/_TEMPLATE/` at Phase 1 start and persists through closeout.

- **Read `main/AGENTS.md` + `main/WIKI.md` + `main/PLAN.md` + `main/drops/WORKFLOW.md` at session start and after every compaction.** CLAUDE.md auto-loads; the others do not — read them deliberately on the first turn after cold-start or compaction before substantive orchestration.
- **Use Tillsyn-style trackers for nothing.** Do NOT use Claude Code's built-in `TaskCreate` / `TaskUpdate` / `TaskList` / `TaskGet` / `TaskStop` / `TaskOutput` — they evaporate on compaction/restart. Decompose finer procedural granularity into atomic units inside the active drop's `PLAN.md` instead.
- **No markdown files outside `main/drops/` for work tracking.** Per-drop dirs are the worklog substrate.

## Drops

A **drop** is a unit of work — one entry in PLAN.md, one directory under `main/drops/`. Drops are declared in PLAN.md and refined in their own dir.

- Atomic granularity: a drop is "atomic" when one builder subagent can finish a single unit cleanly, the unit's acceptance criteria are yes/no-verifiable by a QA subagent, and its `paths` / `packages` footprint is clear. If a drop is too large, **add more units inside its `PLAN.md`** rather than stretching one unit.
- Ordering: parent-child nesting (a drop cannot close while any of its units is incomplete) + `blocked_by` for sibling and cross-unit ordering. No `depends_on` field.
- State: per-drop `state` lives in the drop dir's `PLAN.md` header (`planning` / `building` / `done` / `blocked`); per-unit `state` lives in the Planner section's unit row inside that file (`todo` / `in_progress` / `done` / `blocked`); container-level `state` lives in `main/PLAN.md`'s drop tree table.

Full lifecycle in `main/drops/WORKFLOW.md`. Drop tree in `main/PLAN.md`.

## Orchestrator-as-Hub

The parent Claude Code session launched by the dev from this directory is always **the orchestrator**. Every other role (builder, qa-proof, qa-falsification, planning, research) is a subagent spawned via the `Agent` tool.

**CRITICAL: The orchestrator NEVER writes Go code.** The parent session must not use `Edit`, `Write`, or any other tool to modify `.go` source, test, or `magefile.go` files. Every code change — every single one — goes through a `go-builder-agent` subagent. Orchestrator reads code for planning/research; edits markdown only (this file, `WIKI.md`, `PLAN.md`, drop dir mds, `LEDGER.md`, `README.md`, `AGENTS.md`, agent `.md` files).

### Agent Bindings

| Role | Agent | Edits Go? |
|---|---|---|
| Builder | `go-builder-agent` | **Yes** (only role that does) |
| QA Proof | `go-qa-proof-agent` | No |
| QA Falsification | `go-qa-falsification-agent` | No |
| Planning | `go-planning-agent` | No |
| Research | Claude's built-in `Explore` subagent | No |

The agents are **global** (`~/.claude/agents/`) and reference Tillsyn tooling that Valv does not use. Every spawn carries the override preamble from `main/drops/WORKFLOW.md` § "Agent Spawn Contract" — single canonical source, do not duplicate it here. Per-role appendix fields (drop's PLAN.md path, unit ID, target output file, round number, working dir) are listed in WORKFLOW.md § "Per-Role Spawn Appendices".

## Build-QA-Commit Loop

Per-drop lifecycle is canonical in `main/drops/WORKFLOW.md` (Phases 1–7: plan, plan-QA, discuss + cleanup, build, build-QA, verify, closeout). This file does not duplicate the phase steps.

**Follow WORKFLOW.md's phases in order, exactly as written. No skipped phases. No reordered phases. No shortcut paths.** If a phase looks redundant for a particular drop, return the question to the dev — do not unilaterally drop it. Phase exits gate the next phase (see WORKFLOW.md § "Phase Order").

**Code is NEVER committed or pushed without per-unit QA passing first**, and **Hylla reingest is drop-end only** — both rules are enforced inside WORKFLOW.md's phases. Subagents never call `hylla_ingest`.

## Hylla Baseline

- **Artifact ref**: `github.com/evanmschultz/valv@main` — Hylla resolves `@main` to the latest ingest.
- **Hylla ingest is drop-end only**, not per-unit. Only the orchestrator calls `hylla_ingest`. Always `enrichment_mode=full_enrichment`, always from the GitHub remote, never before `git push` + `gh run watch --exit-status` green. Subagents never call `hylla_ingest`.

### Code Understanding Rules

1. **All Go code**: use Hylla MCP first (`hylla_search`, `hylla_node_full`, `hylla_search_keyword`, `hylla_refs_find`, `hylla_graph_nav`). Exhaust every Hylla search mode — vector, keyword, graph-nav, refs — before falling back to `LSP`, `Read`, `Grep`, `Glob`. **Whenever a Hylla miss forces a fallback, the subagent records the miss in its closing comment** under a `## Hylla Feedback` heading inside the drop's `BUILDER_WORKLOG.md`.
2. **Changed since last ingest**: use `git diff`. Hylla is stale for those files until reingest.
3. **Non-Go code** (markdown, TOML, YAML, magefile, SQL): use `Read`, `Grep`, `Glob`, `Bash` directly.
4. **External semantics**: Context7 + `go doc` + `LSP` for library and language questions the repo can't answer itself. Per AGENTS.md § 9 (Context7 First), resolve the library id and query docs before writing or reviewing code that touches an external library.
5. **`LSP` tool** (gopls-backed): symbol search, references, diagnostics, rename safety for live / uncommitted code. Auto-targets the active checkout (`main/`).
6. **Laslig note**: `github.com/evanmschultz/laslig` is not yet in Context7. Use Hylla (`hylla_search` with `artifact_ref=github.com/evanmschultz/laslig@main`) or `go doc github.com/evanmschultz/laslig` as the primary laslig evidence sources.

## Evidence Sources

In order:

1. **Hylla** — committed repo-local Go code.
2. **`git diff`** — uncommitted local deltas / files changed since last ingest.
3. **Context7 + `go doc` + gopls `LSP`** — external / language / tooling semantics.

## Semi-Formal Reasoning

For semantic, high-risk, or ambiguous work:

- **Premises** — what must be true.
- **Evidence** — grounded in Hylla / `git diff` / Context7 / `go doc` / gopls.
- **Trace or cases** — concrete paths through the code.
- **Conclusion** — the claim.
- **Unknowns** — what remains uncertain. Routed to the orchestrator (subagents return Unknowns in their final response; orchestrator surfaces to dev).

Short and inspectable. Full Section 0 spec lives in `~/.claude/CLAUDE.md` § "Semi-Formal Reasoning — Section 0 Response Shape". The Agent Spawn Contract preamble (in WORKFLOW.md) requires Section 0 from every subagent — but Section 0 stays in the orchestrator-facing response **only**, never inside `PLAN.md` / `BUILDER_WORKLOG.md` / `BUILDER_QA_*.md` / `PLAN_QA_*.md` / `CLOSEOUT.md`.

## QA Discipline

**No build unit is `done` without per-unit QA passing.** This is a gate, not a suggestion. Two asymmetric passes, not duplicates:

- **QA Proof** (`go-qa-proof-agent`) — evidence completeness, reasoning coherence, trace coverage. Asks: *"does the evidence support the claim?"*
- **QA Falsification** (`go-qa-falsification-agent`) — counterexamples, alternate traces, hidden dependencies, contract mismatches, YAGNI pressure. Asks: *"can I construct a case where this is wrong?"*

Plan-QA and build-QA both run as parallel proof + falsification spawns. Plan-QA writes transient files (`PLAN_QA_PROOF.md`, `PLAN_QA_FALSIFICATION.md`) that orch `git rm`s between rounds. Build-QA appends rounds to durable files (`BUILDER_QA_PROOF.md`, `BUILDER_QA_FALSIFICATION.md`). Full file-lifecycle table in `main/drops/WORKFLOW.md`. Per-drop delivery standards (70% per-package coverage, real integration over mocks, `testcontainers-go` for Docker-backed behavior) come from AGENTS.md § 11 — this section does not restate them.

## Orchestrator Role Boundaries

- **Orchestrator** (this parent Claude Code session) — plans, routes, delegates, cleans up. **Never edits Go code or `magefile.go`.** May edit markdown docs (this file, `WIKI.md`, `PLAN.md`, drop dir mds, `README.md`, `LEDGER.md`, `AGENTS.md`, `REFINEMENTS.md`, agent `.md` files).
- **Builder subagent** (`go-builder-agent`) — the ONLY role that edits Go code. Spawned via the `Agent` tool with the spawn contract preamble + builder appendix.
- **QA subagents** (`go-qa-proof-agent`, `go-qa-falsification-agent`) — gated to QA roles. Read, verify, write to their own `*_QA_*.md` file, return verdict to orch, die. Never edit code.
- **Planner subagent** (`go-planning-agent`) — fills the drop's `PLAN.md` Planner section (Phase 1) and revises it across plan-QA rounds (Phase 3). Never edits code.
- **Dev / human** — approves design calls during plan-QA discussion (Phase 3), reviews build-QA findings (Phase 5).

## Project Structure

Small, Go-idiomatic layout. Every internal package is an implementation detail — nothing here is a public API beyond the binary. Detailed runtime-model boundaries (Go control plane + `modernc.org/sqlite` + Docker isolation + Valv-managed profile homes) live in AGENTS.md § 4.

### Package Map

- `cmd/valv/` — cobra + Fang v2 entry. All flag wiring here; `RunE` dispatches into internal packages. Built via `./cmd/valv`.
- `internal/domain/` — pure domain types and consumer-side interfaces. Zero dependencies on other internal packages.
- `internal/adapters/` — concrete implementations of domain interfaces (SQLite store, Docker runtime adapter, provider-process launcher, filesystem). Depends on `internal/domain`.
- `internal/services/` — orchestration/use-case layer composing adapters to fulfill domain operations. Depends on `internal/domain` and `internal/adapters`.
- `internal/cli/` — cobra command implementations (`valv codex`, `valv manage …`) plus pass-through launcher for attached subprocesses. Depends on `internal/services`.
- `internal/tui/` — Bubble Tea v2 surfaces for the management views (selectors, status, account admin). Depends on `internal/services`.
- `internal/config/` — config loading and defaults (TOML via `BurntSushi/toml`).
- `internal/logging/` — `charmbracelet/log` setup and log-path policy.
- `internal/output/` — output/error rendering contracts shared across CLI and TUI.
- `internal/pathutil/`, `internal/progress/`, `internal/project/` — focused supporting helpers with narrow surfaces.
- `magefile.go` at repo root — mage build automation.

### Import DAG

Linear layered flow: `internal/domain → internal/adapters → internal/services → (internal/cli | internal/tui)`. `cmd/valv` wires `cli` (and its sub-commands that mount `tui`) at the top. No cycles — strictly layered.

## Tech Stack

Production deps (authoritative list in `main/go.mod`):

- Go 1.26+.
- `charm.land/fang/v2` — Fang CLI polish (help/version/error rendering, signal-aware root context).
- `charm.land/bubbletea/v2` — Bubble Tea TUI runtime for Valv-owned management surfaces.
- `charm.land/lipgloss/v2` — styling for Lipgloss + Fang output.
- `charm.land/bubbles/v2` — reusable Bubble Tea components where needed.
- `github.com/spf13/cobra` — command tree.
- `github.com/charmbracelet/log` — structured logging (AGENTS.md § 7).
- `github.com/evanmschultz/laslig` — structured output, TTY-aware rendering, mage-side status lines.
- `modernc.org/sqlite` — pure-Go SQLite driver for the runtime metadata store (AGENTS.md § 4 forbids CGO SQLite).
- `github.com/modelcontextprotocol/go-sdk` — MCP SDK used by the MCP translation layer described in AGENTS.md § 9.2.
- `github.com/BurntSushi/toml` — config loading.
- `github.com/magefile/mage` — build automation.
- `github.com/testcontainers/testcontainers-go` — real Docker-backed integration tests (AGENTS.md § 11 testing standards).
- `github.com/charmbracelet/x/exp/teatest/v2` + `github.com/charmbracelet/x/exp/golden` + `github.com/creack/pty/v2` — Bubble Tea golden fixtures and PTY-backed transcript tests.

Docker integration: Valv shells out to the `docker` CLI via `exec.Command` (see `magefile.go`'s `Dev` namespace and `internal/adapters`). There is **no** Go Docker SDK dependency — attempts to add one belong in AGENTS.md, not silently in a drop.

Dev tooling (installed as a Go tool, invoked from mage):

- `mvdan.cc/gofumpt` — stricter `gofmt` superset. `mage test` runs `gofumpt -l` and fails if any file is unformatted.

## Build Verification

Per-unit verification (during build-QA, Phase 5 of WORKFLOW.md): builder runs `mage testPkg <pkg>` for the touched packages, or the appropriate golden/integration target when the change touches a TUI or Docker-backed path. Drop-end verification (after all units pass build-QA, Phase 6 of WORKFLOW.md): `mage test` from `main/`, then — when relevant — `mage integration` and/or `mage golden`, then `git push`, then `gh run watch --exit-status` until green, then `mage build` before handing CLI testing back to the user (per AGENTS.md § 12).

1. All relevant mage targets pass (discover via `mage -l`).
2. **NEVER run raw `go test`, `go build`, `go run`, `go vet`, `gofumpt`** — always `mage <target>`. If a mage target has a bug, fix the target — don't bypass. No exceptions, orchestrator or subagent. AGENTS.md § 13 additionally bans ad hoc `GOCACHE` / `GOMODCACHE` / `GOPATH` overrides.
3. All build-QA rounds for every unit have closed green.

Mage targets from `magefile.go`:

| Target | Command | When |
|---|---|---|
| `mage build` | `go build -o ./valv ./cmd/valv` | final local binary creation check |
| `mage test` | gofumpt format check + `go test -race -cover -count=1 ./...` + 70%-per-package coverage gate | canonical local verification gate |
| `mage testPkg <pkg>` | gofumpt check + `go test -race -cover -count=1 <pkg>` + coverage gate | per-package build-QA signoff |
| `mage integration` | `go test -tags=integration -count=1 ./internal/cli` | Docker-backed integration + external golden coverage |
| `mage golden` | Tracked Bubble Tea goldens + external Codex transcript golden | golden regression suite |
| `mage goldenUpdate` | same packages as `mage golden` with `-args -update` | refresh tracked golden fixtures |
| `mage run "…"` | `mage build` then runs the built `./valv` binary with the quoted args | smoke check |
| `mage dev:home` | print the disposable dev-home path | inspect dev-mode state |
| `mage dev:reset` | recreate the disposable dev-home path | reset dev-mode state |
| `mage dev:clean` | remove the disposable dev-home + dev-tagged images | dev-mode teardown |
| `mage dev:run "…"` | `mage build` then runs the binary under a disposable dev-home with host Docker config preserved | local validation without dirtying real `$HOME` |

Final local signoff for normal work is `mage test`. Add `mage integration` when the change touches Docker-backed or external-transcript paths (AGENTS.md § 11). After `gh run watch` reports green, run `mage build` before handing CLI testing back to the user. `mage test` already enforces the 70% per-package coverage floor from AGENTS.md § 11.

## Go Development Rules

AGENTS.md § 5–7 are authoritative for Go standards, error handling, and logging. This section records only the workflow discipline that sits above those rules.

### Structure + Style

- **Interface-first boundaries**, dependency inversion where warranted; keep interfaces near the consumer (AGENTS.md § 5).
- **Smallest concrete design.** No abstraction for hypothetical future variation (AGENTS.md § 5).
- **TDD-first** where practical. Ship small tested increments.
- **Idiomatic Go** — `gofumpt` enforces layout via `mage test`; `go vet` runs as part of `go test`.
- **Go doc comments** on every exported identifier, starting with the identifier name.

### Errors

AGENTS.md § 6 is authoritative: wrap with `fmt.Errorf("context: %w", err)` at every boundary that adds information, bubble up to the command/runtime boundary, never swallow, preserve enough context for logs to trace back to root cause. Sentinels `ErrFoo`; inspect with `errors.Is` / `errors.As`; never string-match an error.

### Concurrency

- **Every goroutine is context-cancellable.** Long-running loops check `ctx.Done()`. The `RunE` entrypoint threads `ctx` from `cmd.Context()` down; AGENTS.md § 9.3 additionally requires the main process entrypoint to use a signal-aware root context so `Ctrl-C` / `SIGTERM` run shutdown hooks.
- **`defer` for cleanup.** File closers, mutex unlocks, `cancel()` funcs — always `defer` the cleanup on the line after the resource acquisition.
- **No shared mutable state without synchronization.** Prefer channels for ownership transfer; keep any needed `sync.Mutex` unexported on the owning struct.
- **Race detector always on** — `mage test` runs `-race` unconditionally. CI fails if a race is detected.

### Tests

- `*_test.go` co-located with source. Table-driven for anything with input variants. Behavior-oriented assertions.
- `-race` and `-cover` via mage (`mage test` / `mage testPkg`).
- Prefer **real** SQLite, real filesystem state, real Docker (via `testcontainers-go`) over mocks — AGENTS.md § 11 bans mock-heavy unit tests for runtime, provider, storage, Docker, or CLI launch-path behavior.
- Bubble Tea surfaces use `github.com/charmbracelet/x/exp/teatest/v2` with golden fixtures; attached subprocess paths (`valv codex`) use transcript-style golden coverage (AGENTS.md § 8).
- `testdata/` next to the test that reads it — Go stdlib idiom (`go help test` documents the `testdata` directory convention). No shared top-level `testdata/`.

### Mage Discipline

- Plain `mage <target>` from `main/`. No `GOCACHE=...` overrides (AGENTS.md § 13).
- If a target is missing or broken, add/fix the target — never bypass with a raw `go` command.

### After Touching Go Code

- `mage test` before handoff at drop-end (Phase 6 of WORKFLOW.md). Add `mage integration` when the change touches Docker-backed paths. After pushing: `gh run watch --exit-status` until green, then `mage build` per AGENTS.md § 12.

### Dependencies

- Ask the dev to run `go get` / module updates. No `GOPROXY=direct`, `GOSUMDB=off`, or checksum bypass.

### Reference Lookups

- **Context7** + `go doc` + gopls `LSP` before any unfamiliar external API usage, after any test failure (AGENTS.md § 9 is authoritative on Context7-first behavior).

### Markdown Authoring

- Drop dir mds (`PLAN.md`, `BUILDER_WORKLOG.md`, `*_QA_*.md`, `CLOSEOUT.md`) are markdown-first. Use fenced code blocks for snippets, tables for structured data, headings per `main/drops/WORKFLOW.md`. No HTML.

## Skill and Slash Command Routing

| Command | When to Use |
|---|---|
| `/qa-proof` | Proof-oriented QA (used inside subagent definitions; orchestrator typically just spawns the agent) |
| `/qa-falsification` | Falsification-oriented QA (same) |
| `/select-checkout` | Confirm the active visible checkout |
| `/gopls-sync` | Verify gopls targets `main/` |
| `semi-formal-reasoning` | Explicit reasoning certificate (Section 0 shape) |

Note: `/plan-from-hylla` is a Tillsyn-coupled global skill — Valv does not use it. Planner work happens via `go-planning-agent` spawned per `main/drops/WORKFLOW.md` § "Phase 1".

## Git Commit Format

Conventional-commit: `type(scope): message`. All lowercase except proper nouns, acronyms (HTTP, CLI, JSON, TUI, MCP, SDK). Concise — describe what changed, not how.

**Subject-line only. No body. No bullet lists in the commit message.** The diff records what changed file-by-file; the subject line carries the human summary. Do not enumerate per-file changes in a body — that content belongs in the PR description, WIKI changelog, or LEDGER entry, not in `git log`.

Types: `feat`, `fix`, `refactor`, `chore`, `docs`, `test`, `ci`, `style`, `perf`.

Examples:
- `feat(codex): attach Docker subprocess with normalized TERM`
- `fix(adapters): clean up orphan starting runtime rows on boot failure`
- `chore(deps): bump charm.land/bubbletea to v2.0.2`
- `docs(drop-0): rewrite project docs for Valv`
- `docs(drop-3): clear plan qa round 2, route to planner`

No co-authored-by trailers. No period at end. No capitalized first word after the colon unless proper noun/acronym. Keep the subject under ~72 chars when possible — if it won't fit, the change is probably too bundled and should be two commits.

## Safety

- Never delete files or directories without explicit dev approval.
- Never run commands outside the repo root `/Users/evanschultz/Documents/Code/hylla/valv`.
- Never push to any remote without explicit request.
- Keep secrets out of committed config files.

## Bare-Root and Worktree Discipline

- The bare repo at `/Users/evanschultz/Documents/Code/hylla/valv` (one level up) is the **steward orchestrator** root — not a coding checkout. It is a **flat bare repo**: `HEAD`, `config`, `objects/`, `refs/`, `worktrees/`, etc. live directly at the top level. There is no `.bare/` wrapper and no top-level `.git` pointer. This worktree's own `.git` pointer at `main/.git` reads `gitdir: /Users/evanschultz/Documents/Code/hylla/valv/worktrees/main`.
- This directory (`main/`) is the primary work checkout. Real coding / building / testing / committing happens here.
- Always confirm `pwd` is this checkout before edits, tests, commits, or gopls work.
- **Dev launches work orchestrators from here.** Steward orchestrators (project oversight, merge-conflict help) launch from the bare-root one level up and never edit source.
- If checkout context is unclear, use `/select-checkout`.
- Valv uses a single visible checkout (`main/`). Additional lane worktrees and gopls-sync only matter if a multi-lane setup is introduced later — the steward coordinates across them.

## Recovery After Session Restart

Filesystem + git, no Tillsyn calls. Full procedure in `main/drops/WORKFLOW.md` § "Recovery After Restart". Quick form:

1. `git status` — uncommitted work.
2. `git log --oneline -20` — recent commits.
3. Read `main/AGENTS.md` + `main/PLAN.md` — authoritative rules + container states.
4. List `main/drops/*/PLAN.md` headers — per-drop phase state.
5. Per active drop: presence of `PLAN_QA_*.md` = mid-plan-QA loop; absence + `BUILDER_WORKLOG.md` exists = mid-build; `CLOSEOUT.md` with `state: done` = drop closed.
6. Per active unit: scan latest `## Unit N.M — Round K` heading in `BUILDER_WORKLOG.md` + both `BUILDER_QA_*.md` to figure out next step.
