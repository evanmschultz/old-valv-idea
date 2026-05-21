# Valv — Project CLAUDE.md (main worktree)

This file lives in the **`main/` worktree** at `/Users/evanschultz/Documents/Code/hylla/valv/main/`. This is the primary work checkout — all real coding, building, testing, and committing happens here. **The dev launches work orchestrators from this directory.** Sessions launched from the bare-root one directory up are steward orchestrators with a different prompt (bare-root `CLAUDE.md`) and a different scope — cross-worktree oversight and merge-conflict help, not feature work.

**This file is the single source of truth for Valv.** Cross-cutting product/runtime/Go/CLI/MCP/testing rules and orchestrator/drop coordination rules are consolidated here. There is no separate `AGENTS.md`. Read this on every cold-start and after every compaction alongside `main/PLAN.md` and `main/drops/WORKFLOW.md`.

## Product Direction

`valv` is a macOS-first control plane for per-account isolated containerized agentic-dev workloads. AI CLI launching (Codex + Claude Code) is the first-class case shipping today; a generic `valv run --account <name> <command>` primitive backs the provider-specific launchers (planned DROP_13).

Current product direction:

- Two provider launchers ship today: `valv codex` (first primary target) and `valv claude`. Both are pass-through containerized launchers.
- **Cross-provider in-container routing**: when `valv claude` runs in a project, the claude image contains the `codex` CLI AND the project's pinned Codex profile is cross-mounted at `/home/valv/.codex` with `CODEX_HOME` set; mirror for `valv codex`. Per-project binding required.
- **Provider launchers will become thin adapters** over the generic per-account run primitive (planned DROP_13). The same isolation model (per-account credential homes + cross-mount + per-project binding) underlies every workload.
- **Declarative per-project toolchain** (`.valv/tools.toml`) determines what's available inside the container (planned DROP_11). Per-project images compose the base provider image with a tools overlay layer (planned DROP_12).
- **Per-account env var maps** thread into container launch alongside existing CLAUDE_CONFIG_DIR/CODEX_HOME (planned DROP_14). Same env-var name allowed across accounts with different values.
- **Closed-by-default outbound network with per-project allowlist** (planned DROP_15).
- **Sibling-path-aware mounts** for git worktrees and other cross-repo cases are shipped.
- Per-account credential isolation, multi-identity per project, and Docker-runtime sandbox are the core differentiators. Survey of devcontainer.json / mise / asdf / nix / Codespaces / Gitpod / ddev / claudebox (2026-05-20) found no project simultaneously offers (a) closed-network + allowlist, (b) declarative per-project toolchain in a container, (c) multi-identity isolation, AND (d) sibling-path-aware mounts. That intersection is Valv's niche.
- Valv management flows live outside the direct pass-through path.
- Global account switching remains optional convenience, not the core runtime model.

Follow `main/PLAN.md` as the active drop tree.

## Platform Scope

- macOS only.
- Docker required (Docker Desktop on macOS).
- No expectation of Linux or Windows support.

Do not add cross-platform abstractions or compatibility work unless the dev explicitly asks.

## Repository Topology

Bare-root Git layout with worktrees:

- The repository root is a **flat bare Git repository** at `/Users/evanschultz/Documents/Code/hylla/valv` (one level up from this file). `HEAD`, `config`, `objects/`, `refs/`, `worktrees/` live directly at the top. No `.bare/` wrapper. No top-level `.git` pointer.
- Day-to-day implementation work happens in the `main/` worktree. This worktree's `.git` pointer at `main/.git` reads `gitdir: /Users/evanschultz/Documents/Code/hylla/valv/worktrees/main`.
- Agent-local work logs live under `.worklog/` in the active worktree. `.worklog/` is gitignored.
- Root `.codex/config.toml` points MCP tools such as `gopls mcp` at the `main/` worktree.
- The bare root is **not** a coding checkout. It is a legitimate steward-orchestrator scope (cross-worktree oversight) but never edits source.

Confirm `pwd` is the visible checkout before edits, tests, commits, or gopls work. If checkout context is unclear, use `/select-checkout`.

## Runtime Model

Preserve these architectural boundaries:

- Go control plane.
- SQLite runtime metadata store via `modernc.org/sqlite` (pure Go). **Do not introduce CGO SQLite dependencies.**
- Valv-managed provider profile homes under `~/Library/Application Support/valv/providers/<provider>/profiles/<account>/`.
- Docker runtime isolation; Valv shells out to the `docker` CLI via `exec.Command`. **No Go Docker SDK dependency.**

For `valv codex` and `valv claude`:

- Pass all provider-CLI args through as directly as possible.
- Do not reinterpret normal provider CLI semantics unless the dev explicitly asks.
- Avoid replacing provider session/resume logic with Valv-specific logic.
- Let Valv provide runtime isolation, project/profile resolution, cross-provider mounting, and logging.

## Coordination Model — At a Glance

Valv does **not** use Tillsyn. Three documents own the coordination model; they do not duplicate each other:

- **`main/PLAN.md`** — overarching drop tree (container drops + state + `blocked_by` + per-drop dir link). Updated *after* a drop closes or *after* a planner restructures the tree. Not edited mid-build.
- **`main/drops/WORKFLOW.md`** — canonical per-drop lifecycle (planner → plan-QA → discuss → revise → builder → build-QA → verify → close). Owns: drop directory shape, file lifecycles, phase order, the **Agent Spawn Contract** (preamble pasted into every subagent spawn), restart recovery.
- **`main/CLAUDE.md`** (this file) — orchestrator role boundaries, agent bindings, drop coordination, cross-cutting product/runtime/Go/CLI/MCP/testing rules.

Per-drop work artifacts live under `main/drops/DROP_N_<NAME>/`. The directory is stamped from `main/drops/_TEMPLATE/` at Phase 1 start and persists after close as the drop's historical record.

- **Read `main/PLAN.md` + `main/drops/WORKFLOW.md` at session start and after every compaction.** CLAUDE.md auto-loads; the others do not — read them deliberately on the first turn after cold-start or compaction before substantive orchestration.
- **Use Tillsyn-style trackers for nothing.** Do NOT use Claude Code's built-in `TaskCreate` / `TaskUpdate` / `TaskList` / `TaskGet` / `TaskStop` / `TaskOutput` — they evaporate on compaction/restart. Decompose finer procedural granularity into atomic units inside the active drop's `PLAN.md` instead.
- **No markdown files outside `main/drops/` for work tracking.** Per-drop dirs are the worklog substrate.

## Drops

A **drop** is a unit of work — one entry in PLAN.md, one directory under `main/drops/`. Drops are declared in PLAN.md and refined in their own dir.

- Atomic granularity: a drop is "atomic" when one builder subagent can finish a single unit cleanly, the unit's acceptance criteria are yes/no-verifiable by a QA subagent, and its `paths` / `packages` footprint is clear. If a drop is too large, **add more units inside its `PLAN.md`** rather than stretching one unit.
- Ordering: parent-child nesting (a drop cannot close while any of its units is incomplete) + `blocked_by` for sibling and cross-unit ordering. No `depends_on` field.
- State: per-drop `state` lives in the drop dir's `PLAN.md` header (`planning` / `building` / `done` / `blocked`); per-unit `state` lives in the Planner section's unit row inside that file (`todo` / `in_progress` / `done` / `blocked`); container-level `state` lives in `main/PLAN.md`'s drop tree table.

Full lifecycle in `main/drops/WORKFLOW.md`. Drop tree in `main/PLAN.md`.

## Orchestrator-as-Hub

The parent Claude Code session launched by the dev from this directory is always **the orchestrator**. Every other role (builder, qa-proof, qa-falsification, planning, research) is a subagent dispatched via role-appropriate backend routing (see "Backend Routing" below).

**CRITICAL: The orchestrator NEVER writes Go code.** The parent session must not use `Edit`, `Write`, or any other tool to modify `.go` source, test, or `magefile.go` files. Every code change — every single one — goes through a `ta-go-builder` subagent. Orchestrator reads code for planning/research; edits markdown only (this file, `PLAN.md`, drop dir mds, `README.md`, agent `.md` files).

### Backend Routing — Hybrid Model

Each role has a tier chain. Tiers route to backends via two dispatch mechanisms:

- **Anthropic-backed tiers** (opus, sonnet, haiku) → Claude built-in `Agent` tool with `subagent_type=<persona>`. Reads `.claude/agents/<persona>.md`. Runs against the Anthropic subscription.
- **Codex-exec tier** (gpt-5.5 via `~/.codex/config.toml`) and **ollama-local tier** (qwen2.5-coder:7b) → `bin/agent-dispatch.sh --role <persona> --cwd "$(pwd)" --prompt "<short pointer>"`. Reads `.claude/agent-chains.sh` for tier definitions. Runs against the ChatGPT subscription (codex) or local Ollama daemon (zero Anthropic tokens).

This mirrors ta's routing model documented in `/Users/evanschultz/Documents/Code/hylla/ta/main/docs/agent-backend-routing.md`. **`bin/agent-dispatch.sh` and `.claude/agent-chains.sh` are not yet installed in valv** (planned addition — copy from ta and adapt). Until then, every role spawns via the `Agent` tool (Anthropic-only) and falls back transparently to the persona's primary Anthropic tier from the table below.

### Agent Bindings

| Role | Persona file | Anthropic tier(s) (via `Agent` tool) | Codex / Ollama tier(s) (via `bin/agent-dispatch.sh`) | Edits Go? |
|---|---|---|---|---|
| Builder | `ta-go-builder` | haiku (fallback) | ollama qwen2.5-coder:7b (primary), codex gpt-5.5 effort=low | **Yes** (only role that does) |
| QA Proof | `ta-go-qa-proof` | opus (primary), sonnet (fallback) | codex gpt-5.5 effort=medium | No |
| QA Falsification | `ta-go-qa-falsification` | opus, sonnet (fallbacks) | codex gpt-5.5 effort=xhigh / high / medium | No |
| Planning | `ta-go-planning` | sonnet, opus (fallbacks) | codex gpt-5.5 effort=low (primary), effort=medium | No |
| Closeout | `ta-closeout` | opus (primary), sonnet (fallback) | codex gpt-5.5 effort=medium | No |
| Research | Claude's built-in `Explore` subagent | n/a | n/a | No |

Planning chain order is **cheap-first escalation**: codex gpt-5.5 effort=low → effort=medium → claude sonnet → claude opus. Same cheap-first pattern applies to QA-proof and closeout (start with the cheapest viable tier). QA-falsification keeps deepest reasoning at tier 1 (effort=xhigh) since adversarial review benefits most from more compute.

The personas live in **project-local** `.claude/agents/` (not global `~/.claude/agents/`) and reference Tillsyn tooling that Valv does not use. Every spawn carries the override preamble from `main/drops/WORKFLOW.md` § "Agent Spawn Contract" — single canonical source, do not duplicate it here. Per-role appendix fields (drop's PLAN.md path, unit ID, target output file, round number, working dir) are listed in WORKFLOW.md § "Per-Role Spawn Appendices".

### Dispatched Agent Discipline (once `bin/agent-dispatch.sh` lands in valv)

- **Hylla artifact_ref**: spawn prompts that invoke `mcp__hylla__*` for valv's own code MUST pass `github.com/evanmschultz/valv@main` — pinned to branch, no float. Verify ingest currency via `mcp__hylla__hylla_artifact_metadata` before dispatching when Hylla evidence is critical.
- **Inline `--prompt` or stdin only — NEVER `--prompt-file`**: the dispatcher accepts the prompt directly. Temp files obscure the call site and don't reproduce. `--prompt-file` exists only as a last-resort fallback for pathological shell quoting and should not appear in normal use.
- **Tool-call audit after every dispatch**: open the dispatch output and verify each agent claim against the actual stream — codex `mcp: <server>/<tool> (completed)` lines plus claude-native/ollama JSON envelope `tool_use` events. Self-reported "verdict: pass" or "tool X succeeded" is not authoritative; if the stream doesn't show the required tool calls (record updates, file edits, tests), the work didn't happen — re-dispatch or finish orchestrator-direct. Flag out-of-scope tool calls (anything outside the persona's `tools:` allowlist) as a discipline violation.

## Build-QA-Commit Loop

Per-drop lifecycle is canonical in `main/drops/WORKFLOW.md` (Phases 1–7: plan, plan-QA, discuss + cleanup, build, build-QA, verify, close). This file does not duplicate the phase steps.

**Follow WORKFLOW.md's phases in order, exactly as written. No skipped phases. No reordered phases. No shortcut paths.** If a phase looks redundant for a particular drop, return the question to the dev — do not unilaterally drop it. Phase exits gate the next phase (see WORKFLOW.md § "Phase Order").

**Code is NEVER committed or pushed without per-unit QA passing first**, and **Hylla reingest is drop-end only** — both rules are enforced inside WORKFLOW.md's phases. Subagents never call `hylla_ingest`.

## Hylla Baseline

- **Artifact ref**: `github.com/evanmschultz/valv@main` — Hylla resolves `@main` to the latest ingest.
- **Hylla ingest is drop-end only**, not per-unit. Only the orchestrator calls `hylla_ingest`. Always `enrichment_mode=full_enrichment`, always from the GitHub remote, never before `git push` + `gh run watch --exit-status` green. Subagents never call `hylla_ingest`.

### Code Understanding Rules

1. **All Go code**: use Hylla MCP first (`hylla_search`, `hylla_node_full`, `hylla_search_keyword`, `hylla_refs_find`, `hylla_graph_nav`). Exhaust every Hylla search mode — vector, keyword, graph-nav, refs — before falling back to `LSP`, `Read`, `Grep`, `Glob`. **Whenever a Hylla miss forces a fallback, the subagent records the miss in its closing comment** under a `## Hylla Feedback` heading inside the drop's `BUILDER_WORKLOG.md`.
2. **Changed since last ingest**: use `git diff`. Hylla is stale for those files until reingest.
3. **Non-Go code** (markdown, TOML, YAML, magefile, SQL): use `Read`, `Grep`, `Glob`, `Bash` directly.
4. **External semantics**: Context7 + `go doc` + `LSP` for library and language questions the repo can't answer itself.
5. **`LSP` tool** (gopls-backed): symbol search, references, diagnostics, rename safety for live / uncommitted code. Auto-targets the active checkout (`main/`).
6. **Laslig note**: `github.com/evanmschultz/laslig` is not yet in Context7. Use Hylla (`hylla_search` with `artifact_ref=github.com/evanmschultz/laslig@main`) or `go doc github.com/evanmschultz/laslig` as the primary laslig evidence sources.

## Evidence Sources

In order:

1. **Hylla** — committed repo-local Go code.
2. **`git diff`** — uncommitted local deltas / files changed since last ingest.
3. **Context7 + `go doc` + gopls `LSP`** — external / language / tooling semantics.

### Context7 First

Before planning, writing code, writing tests, doing QA, or fixing failed tests, each agent uses Context7 for the relevant library or framework documentation when a Context7 entry exists.

Minimum rule:

- resolve the relevant library id first
- query the relevant docs before implementation or review
- if no relevant Context7 entry exists, say so briefly in the worklog and continue with the best primary source available

Applies to build agents and QA agents.

## Semi-Formal Reasoning

For semantic, high-risk, or ambiguous work:

- **Premises** — what must be true.
- **Evidence** — grounded in Hylla / `git diff` / Context7 / `go doc` / gopls.
- **Trace or cases** — concrete paths through the code.
- **Conclusion** — the claim.
- **Unknowns** — what remains uncertain. Routed to the orchestrator (subagents return Unknowns in their final response; orchestrator surfaces to dev).

Short and inspectable. Full Section 0 spec lives in `~/.claude/CLAUDE.md` § "Semi-Formal Reasoning — Section 0 Response Shape". The Agent Spawn Contract preamble (in WORKFLOW.md) requires Section 0 from every subagent — but Section 0 stays in the orchestrator-facing response **only**, never inside `PLAN.md` / `BUILDER_WORKLOG.md` / `BUILDER_QA_*.md` / `PLAN_QA_*.md`.

## QA Discipline

**No build unit is `done` without per-unit QA passing.** This is a gate, not a suggestion. Two asymmetric passes, not duplicates:

- **QA Proof** (`go-qa-proof-agent`) — evidence completeness, reasoning coherence, trace coverage. Asks: *"does the evidence support the claim?"*
- **QA Falsification** (`go-qa-falsification-agent`) — counterexamples, alternate traces, hidden dependencies, contract mismatches, YAGNI pressure. Asks: *"can I construct a case where this is wrong?"*

Plan-QA and build-QA both run as parallel proof + falsification spawns. Plan-QA writes transient files (`PLAN_QA_PROOF.md`, `PLAN_QA_FALSIFICATION.md`) that orch `git rm`s between rounds. Build-QA appends rounds to durable files (`BUILDER_QA_PROOF.md`, `BUILDER_QA_FALSIFICATION.md`). Full file-lifecycle table in `main/drops/WORKFLOW.md`.

## Orchestrator Role Boundaries

- **Orchestrator** (this parent Claude Code session) — plans, routes, delegates, cleans up. **Never edits Go code or `magefile.go`.** May edit markdown docs (this file, `PLAN.md`, drop dir mds, `README.md`, agent `.md` files).
- **Builder subagent** (`go-builder-agent`) — the ONLY role that edits Go code. Spawned via the `Agent` tool with the spawn contract preamble + builder appendix.
- **QA subagents** (`go-qa-proof-agent`, `go-qa-falsification-agent`) — gated to QA roles. Read, verify, write to their own `*_QA_*.md` file, return verdict to orch, die. Never edit code.
- **Planner subagent** (`go-planning-agent`) — fills the drop's `PLAN.md` Planner section (Phase 1) and revises it across plan-QA rounds (Phase 3). Never edits code.
- **Dev / human** — approves design calls during plan-QA discussion (Phase 3), reviews build-QA findings (Phase 5).

## Project Structure

Small, Go-idiomatic layout. Every internal package is an implementation detail — nothing here is a public API beyond the binary.

### Package Map

- `cmd/valv/` — cobra + Fang v2 entry. All flag wiring here; `RunE` dispatches into internal packages. Built via `./cmd/valv`.
- `internal/domain/` — pure domain types and consumer-side interfaces. Zero dependencies on other internal packages.
- `internal/adapters/` — concrete implementations of domain interfaces (SQLite store, Docker runtime adapter, provider-process launcher, filesystem). Depends on `internal/domain`.
- `internal/services/` — orchestration/use-case layer composing adapters to fulfill domain operations. Depends on `internal/domain` and `internal/adapters`.
- `internal/cli/` — cobra command implementations (`valv codex`, `valv claude`, `valv account …`, `valv image …`) plus pass-through launcher for attached subprocesses. Depends on `internal/services`.
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
- `github.com/charmbracelet/log` — structured logging.
- `github.com/evanmschultz/laslig` — structured output, TTY-aware rendering, mage-side status lines.
- `modernc.org/sqlite` — pure-Go SQLite driver (CGO SQLite is forbidden).
- `github.com/modelcontextprotocol/go-sdk` — MCP SDK used by the MCP translation layer.
- `github.com/BurntSushi/toml` — config loading.
- `github.com/magefile/mage` — build automation.
- `github.com/testcontainers/testcontainers-go` — real Docker-backed integration tests.
- `github.com/charmbracelet/x/exp/teatest/v2` + `github.com/charmbracelet/x/exp/golden` + `github.com/creack/pty/v2` — Bubble Tea golden fixtures and PTY-backed transcript tests.

Docker integration: Valv shells out to the `docker` CLI via `exec.Command`. There is **no** Go Docker SDK dependency — attempts to add one belong in this file under a new rule, not silently in a drop.

Dev tooling (installed as a Go tool, invoked from mage):

- `mvdan.cc/gofumpt` — stricter `gofmt` superset. `mage test` runs `gofumpt -l` and fails if any file is unformatted.

## CLI And TUI Stack

Charm v2 libraries only.

Allowed:

- `charm.land/fang/v2`
- `charm.land/bubbletea/v2`
- `charm.land/lipgloss/v2`
- `bubbles` directly when needed

Do not use:

- `huh`
- legacy non-v2 Charm module paths

The direct CLI path stays clean and predictable. The management surface may use Bubble Tea selectors and views.
Use `.tmp/blick` as the implementation reference for Fang v2 command structure, output policy, renderer separation, and consistent human/plain/json behavior.

For every user-visible Bubble Tea surface:

- use `github.com/charmbracelet/x/exp/teatest/v2`
- keep golden regression coverage for the final rendered view where layout/styling matters
- add or update golden fixtures whenever a TUI layout or style change is intentional
- user-visible Bubble Tea goldens only cover Valv-owned screens; attached external CLIs like Codex also need terminal-integration coverage that exercises the real subprocess path
- use transcript-style golden coverage for attached `valv codex` visual regressions, including the steady-state Codex screen and an interactive `/mcp` pass through the real subprocess path
- keep Mage targets for Bubble Tea and external transcript goldens aligned with the actual test packages; do not claim a golden test workflow that the repo cannot run
- when a change can affect visible Codex runtime behavior, run the external transcript golden path in addition to the Bubble Tea goldens
- Docker-backed external golden and integration tests must clean up any Valv-managed fixture containers they start; passing tests must not leave `valv-codex:test*` containers running on the host

### Alias Policy

- support selective aliases only
- preferred short aliases are `h`, `m`, and `g`
- do not invent blanket one-letter aliases for every command
- keep user-facing account management terminology as `account`; do not reintroduce `profile` in primary help, examples, or operator output surface
- explicit account lifecycle commands exist for existing accounts; do not rely only on `account add` side effects for login/logout management
- avoid alias schemes that create ambiguity across commands like `status`, `serve`, `switch`
- support trailing `help` / `h` on branch commands when unambiguous
- do not reinterpret trailing args on leaf commands or pass-through commands like `valv codex`

### Output Policy

- follow `blick` output patterns as closely as practical
- treat `Short`, `Long`, and `Example` as mandatory for every visible command
- help screens become more explanatory deeper in the command tree
- help for output-producing commands explains the meaning of key output fields and shows realistic examples
- prefer clear placeholder names such as `personal`, `work`, `alternate-account`, `host-codex`; avoid ambiguous example names like `dev` that read like environment modes instead of account identifiers
- prefer deterministic, minimal human output
- prefer explicit empty states over silent emptiness
- prefer slim machine-readable JSON payloads over human-style wrapper envelopes
- keep machine-readable JSON keys command-owned and stable; do not derive API-like keys from human heading copy
- reduce raw subprocess noise unless that subprocess output is the actual user-facing payload
- disposable dev-mode commands clearly mark temp-home paths and dev-only artifacts as disposable and explain how to clean them up
- cleanup commands target Valv-managed artifacts by label or equivalent authoritative metadata; a visible `valv-...` name prefix is for operator clarity, not by itself enough authority to delete containers
- normal cleanup flows must not remove anonymous Docker volumes or unrelated unlabeled containers unless the dev explicitly asks for destructive host cleanup
- the main process entrypoint uses a signal-aware root context so command shutdown hooks run on `Ctrl-C` and `SIGTERM`

## Containerized Codex Runtime Rules

For interactive `valv codex` and `valv claude` runs:

- do not run the provider CLI inside a Valv Bubble Tea wrapper
- launch the provider CLI as an attached subprocess through Docker with the real terminal attached
- use Docker's init process for interactive attached launches so signal handling and child reaping behave like a normal terminal app
- ensure the container has a coherent in-container user and `HOME`; do not rely on the host absolute profile path doubling as the Linux home directory
- normalize the in-container terminal environment; do not blindly pass host-only `TERM` values that the slim Linux image cannot interpret
- preserve provider auth/session/memory by mounting the selected Valv profile home, but normalize the in-container mount target so the CLI behaves like a normal Linux home layout
- if Valv generates container-only config overlays, keep auth/session files durable while making the runtime config container-safe
- attached Docker subprocesses use direct stdio attachment and are validated with PTY-backed integration tests; do not rely on unsupported controlling-terminal syscalls on the Docker CLI process itself

### Cross-Provider Mount

`valv claude` mounts the project's pinned Codex profile at `/home/valv/.codex` with `CODEX_HOME=/home/valv/.codex` so a Claude Code agent that runs `codex exec` / `codex mcp-serve` auto-routes to that project's pinned Codex login. Mirror for `valv codex` → claude. Cross-mount is skipped when the other provider is not bound to the project; the cross-call then fails with the CLI's native "not logged in" message.

### Worktree gitdir mount

When the project root contains a `.git` linkfile (git worktree), Valv resolves it via `pathutil.ResolveWorktreeGitDir`, follows `gitdir:`, reads `commondir`, and appends a path-transparent bind mount for the bare-repo path so git inside the container can follow the linkfile. Both claude and codex `PrepareRuntime` do this.

## MCP Translation Rules

Host Codex config cannot be reused blindly inside Linux containers.

Required behavior for containerized Codex runs:

- inspect the selected profile-level Codex config plus project-level Codex config when present
- generate a container-safe runtime overlay instead of executing host-only MCP entries unchanged
- translate host-loopback MCP URLs such as `127.0.0.1` and `localhost` to `host.docker.internal` where the target is intended to be the macOS host
- do not hardcode user-specific MCP server names or paths
- keep host-bridge subprocess lifetime bound to the prepared runtime lifecycle, not to a single API request context
- if a host-side stdio MCP bridge cannot be created, omit that translated entry from the overlay and emit a warning; do not write malformed MCP server stanzas
- treat stdio MCP entries generically:
  - if the command exists inside the image, it may run in-container
  - if it depends on a host-only path or host-only binary, expose it through a Valv-managed host bridge instead of trying to execute the macOS binary inside Linux
- preserve project-level precedence over profile-level MCP entries when merging overlays
- if a warm runtime record is persisted in `starting` state and startup later fails, mark that record failed or otherwise clean it up; do not leave orphan `starting` rows behind

## Auth UX Rules

For containerized interactive auth:

- support the device-code flow cleanly for isolated container profiles
- do not assume browser localhost callbacks will work from inside Docker without explicit port publishing/relay support
- keep help text and error guidance explicit about which auth flows are expected to work in disposable containerized profiles versus host-bound `~/.codex` / `~/.claude` reuse
- when creating isolated provider accounts, seed baseline provider config from the default host-backed account when available so MCP/tool configuration is not silently dropped even though auth/session state remains isolated
- prefer host-side provider login for Valv-managed accounts before launching the container so normal browser login completes on macOS without Docker callback issues
- keep steady-state interactive launches quiet; pre-launch notices move to explicit setup, login, or failure paths instead of cluttering attached TTY handoff

User-facing account terminology:

- user-facing management/help/output prefers `account` over `profile`
- do not advertise `profile` as a user-facing compatibility alias once the account-first surface exists; internal storage terms may still use `Profile`, but command/help/output is account-first
- `account add` is the one-command default flow: create or reuse the account home, ensure host-side login when needed, and bind the current project unless `--no-bind` is explicitly requested
- raw `--home` and `--skip-login` are expert overrides, not the primary UX
- account list/inspect output surfaces auth identity details when they can be inferred safely from the managed account home, including email for ChatGPT-backed Codex logins

### Account Deletion

`Service.DeleteProfile` removes the on-disk managed-home dir in addition to the DB row, guarded by `EvalSymlinks`-resolved `HasPrefix` check against the canonical providerRoot. Custom `--home` paths outside providerRoot are left untouched. This prevents the next `account add` from short-circuiting on leftover `.credentials.json` files.

## Logging

- use `github.com/charmbracelet/log`
- add structured logs throughout runtime-critical paths
- include debug logging for profile resolution, project resolution, Docker lifecycle, storage operations, API execution, and provider process launch
- maintain practical local log cleanup/retention
- keep logs useful for troubleshooting local control-plane and MCP-style runtime issues

Users see clean Fang/Lipgloss-rendered failures; detailed operational context lives in structured logs.

## Shared Foundations First

Before broad parallel implementation, establish these shared packages and conventions:

- config loading and defaults
- logger setup and log-path policy
- output/error rendering contracts
- core domain interfaces

Reason:

- all parallel tracks need the same config, logging, and rendering behavior
- these should be stable shared packages early, not retrofitted later
- consistency matters more than short-term speed here

## Go Development Rules

### Structure + Style

- **Interface-first boundaries**, dependency inversion where warranted; keep interfaces near the consumer (NOT in shared dump packages).
- **Smallest concrete design.** No abstraction for hypothetical future variation. Do not add abstraction layers that are not pulling real weight yet.
- **TDD-first** where practical. Ship small tested increments.
- **Idiomatic Go** — `gofumpt` enforces layout via `mage test`; `go vet` runs as part of `go test`.
- **Go doc comments** on every exported identifier, starting with the identifier name.
- Pass `context.Context` through runtime, API, storage, and long-running operations.
- Prefer explicit constructors over package-global mutable state.
- Prefer standard library types and behaviors unless there is a concrete reason not to.
- Keep structs and functions small enough to understand without scrolling through unrelated concerns.
- Use table-driven tests where they improve coverage and readability.

### Errors

- Wrap with `fmt.Errorf("context: %w", err)` at every boundary that adds information.
- Bubble errors up to the command/runtime boundary; never swallow.
- Preserve enough context for logs to trace back to root cause.
- Do not hide underlying failures behind vague generic errors.
- Sentinels `ErrFoo`; inspect with `errors.Is` / `errors.As`; never string-match an error.
- Prefer full boundary-context errors rather than partially handled silent failures.

### Concurrency

- **Every goroutine is context-cancellable.** Long-running loops check `ctx.Done()`. The `RunE` entrypoint threads `ctx` from `cmd.Context()` down; the main process entrypoint uses a signal-aware root context so `Ctrl-C` / `SIGTERM` run shutdown hooks.
- **`defer` for cleanup.** File closers, mutex unlocks, `cancel()` funcs — always `defer` the cleanup on the line after the resource acquisition.
- **No shared mutable state without synchronization.** Prefer channels for ownership transfer; keep any needed `sync.Mutex` unexported on the owning struct.
- **Race detector always on** — `mage test` runs `-race` unconditionally. CI fails if a race is detected.

### Tests

- `*_test.go` co-located with source. Table-driven for anything with input variants. Behavior-oriented assertions.
- `-race` and `-cover` via mage (`mage test` / `mage testPkg`).
- Prefer **real** SQLite, real filesystem state, real Docker (via `testcontainers-go`) over mocks.
- Bubble Tea surfaces use `github.com/charmbracelet/x/exp/teatest/v2` with golden fixtures; attached subprocess paths (`valv codex`) use transcript-style golden coverage.
- `testdata/` next to the test that reads it — Go stdlib idiom. No shared top-level `testdata/`.

### Mage Discipline

- Plain `mage <target>` from `main/`. **No `GOCACHE` / `GOMODCACHE` / `GOPATH` ad-hoc overrides.**
- If a target is missing or broken, add/fix the target — never bypass with a raw `go` command.

### After Touching Go Code

- `mage test` before handoff at drop-end (Phase 6 of WORKFLOW.md). Add `mage integration` when the change touches Docker-backed paths. After pushing: `gh run watch --exit-status` until green, then `mage build`.

### Dependencies

- Ask the dev to run `go get` / module updates. No `GOPROXY=direct`, `GOSUMDB=off`, or checksum bypass.

### Reference Lookups

- **Context7** + `go doc` + gopls `LSP` before any unfamiliar external API usage, after any test failure.

### Markdown Authoring

- Drop dir mds (`PLAN.md`, `BUILDER_WORKLOG.md`, `*_QA_*.md`) are markdown-first. Use fenced code blocks for snippets, tables for structured data, headings per `main/drops/WORKFLOW.md`. No HTML.

## Build Verification

**Per-unit verification** (during build-QA, Phase 5 of WORKFLOW.md): builder runs `mage testPkg <pkg>` for the touched packages, or the appropriate golden/integration target when the change touches a TUI or Docker-backed path.

**Drop-end verification** (after all units pass build-QA, Phase 6 of WORKFLOW.md): `mage test` from `main/`, then — when relevant — `mage integration` and/or `mage golden`, then `git push`, then `gh run watch --exit-status` until green, then `mage build` before handing CLI testing back to the user.

1. All relevant mage targets pass (discover via `mage -l`).
2. **NEVER run raw `go test`, `go build`, `go run`, `go vet`, `gofumpt`** — always `mage <target>`. If a mage target has a bug, fix the target — don't bypass. No exceptions, orchestrator or subagent.
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

Final local signoff for normal work is `mage test`. Add `mage integration` when the change touches Docker-backed or external-transcript paths. After `gh run watch` reports green, run `mage build` before handing CLI testing back to the user. `mage test` already enforces the 70% per-package coverage floor.

## Delivery Standards

Implementation is not done until all of these are true:

- the applicable plan scope is fully implemented
- nothing intentionally deferred is left undocumented
- all tests pass
- each package reaches at least 70% test coverage
- the implementation follows TDD expectations for new work
- user-visible behavior, logs, and docs are aligned

QA requirements:

- every build unit has two independent QA subagents review completeness and quality
- QA verifies behavior, tests, and edge cases, not just diffs
- failed QA or failed tests are fixed before work is considered complete

Worklog requirements:

- every agent keeps its own running worklog under `.worklog/` in the active worktree
- worklogs record plan, assumptions, commands, findings, open issues concisely
- `.worklog/` is local-only and stays gitignored

Testing standards:

- prefer real end-to-end and integration tests over mocks
- use `testcontainers-go` for real Docker-backed integration tests when the behavior under test crosses the runtime boundary
- keep Docker-backed integration tests in CI on Linux runners where Docker is the normal hosted path
- use real SQLite, real filesystem state, and real process execution where practical
- mocks, fakes, and stubs are allowed only when there is no practical real-environment option or when isolating a narrow pure-domain concern
- **do not default to mock-heavy unit tests for runtime, provider, storage, Docker, or CLI launch-path behavior**
- if a mock is introduced, document briefly in the worklog why a real test was not practical
- use targeted `go test` or `go build` only for debugging narrow failures; final local signoff runs the canonical Mage gates
- final local signoff for normal work includes `mage test`; when Docker-backed or external transcript coverage is relevant, final signoff also includes `mage integration`
- for CLI/model compatibility smoke tests use the cheapest viable OpenAI-compatible model (`gpt-5-nano`, or the smallest available default model from the local client when unavailable)
- keep reasoning effort explicit and low for these tests (`--reasoning-effort low` when supported)

## Repository Standards

- keep root docs and guidance aligned with implementation
- prefer a clean repo layout with minimal root clutter
- use `magefile.go` as the command source of truth
- keep CI and local command recipes aligned
- keep a clean dev-mode path that does not dirty the developer's real home directory during normal local checks
- prefer `mage dev:run "..."` flows for disposable local validation and `mage build` for normal binary creation
- keep `mage test` separate from `mage build`; `mage test` is the local verification gate, `mage build` is the final local binary creation check
- after local `mage test` passes and after the pushed GitHub run passes, run `mage build` before handing local CLI testing back to the user
- when using disposable dev-mode home directories, preserve access to the host Docker CLI configuration/plugins so Docker Desktop features such as `buildx` keep working
- **after every push, run `gh run watch` for the triggered workflow and confirm the result before considering the push complete**
- use `gh run watch` directly; do not route GitHub run watching through repo-local `bin` helpers, wrapper scripts, or workaround commands

Docker build standards:

- prefer modern `docker buildx build --load` behavior over legacy builder paths
- keep dev and default image tags separable when local development needs to avoid dirtying normal runtime state
- suppress avoidable package-manager noise in Docker image builds where practical, including npm update-notifier chatter

## Sandbox And Go Tooling

Do not alter Go cache or module environment variables to work around sandbox restrictions.

Prohibited:

- `GOCACHE=... mage test`
- `GOCACHE=... go test ./...`
- ad hoc overrides of `GOCACHE`, `GOMODCACHE`, `GOPATH`, or similar Go env paths to bypass local environment constraints

Required behavior:

- use the normal system Go cache and normal local command paths
- keep Mage targets correct rather than wrapping them in sandbox workarounds
- if a Go command or test run fails because of sandbox restrictions, stop, report that clearly, and let the dev run it or decide the next step

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

**Subject-line only. No body. No bullet lists in the commit message.** The diff records what changed file-by-file; the subject line carries the human summary. Do not enumerate per-file changes in a body — that content belongs in the PR description if one exists, not in `git log`.

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

- The bare repo at `/Users/evanschultz/Documents/Code/hylla/valv` (one level up) is the **steward orchestrator** root — not a coding checkout. Flat bare repo: `HEAD`, `config`, `objects/`, `refs/`, `worktrees/` at the top level. No `.bare/` wrapper, no top-level `.git` pointer. This worktree's `.git` pointer at `main/.git` reads `gitdir: /Users/evanschultz/Documents/Code/hylla/valv/worktrees/main`.
- This directory (`main/`) is the primary work checkout. Real coding / building / testing / committing happens here.
- Always confirm `pwd` is this checkout before edits, tests, commits, or gopls work.
- **Dev launches work orchestrators from here.** Steward orchestrators (project oversight, merge-conflict help) launch from the bare-root one level up and never edit source.
- If checkout context is unclear, use `/select-checkout`.
- Valv uses a single visible checkout (`main/`). Additional lane worktrees and gopls-sync only matter if a multi-lane setup is introduced later — the steward coordinates across them.

## Recovery After Session Restart

Filesystem + git, no Tillsyn calls. Full procedure in `main/drops/WORKFLOW.md` § "Recovery After Restart". Quick form:

1. `git status` — uncommitted work.
2. `git log --oneline -20` — recent commits.
3. Read `main/PLAN.md` — container states.
4. List `main/drops/*/PLAN.md` headers — per-drop phase state.
5. Per active drop: presence of `PLAN_QA_*.md` = mid-plan-QA loop; absence + `BUILDER_WORKLOG.md` exists = mid-build; drop's `PLAN.md` header `state: done` = drop closed.
6. Per active unit: scan latest `## Unit N.M — Round K` heading in `BUILDER_WORKLOG.md` + both `BUILDER_QA_*.md` to figure out next step.
