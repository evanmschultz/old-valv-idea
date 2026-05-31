# Valv — Project CLAUDE.md (main worktree)

Lives in the **`main/` worktree** at `/Users/evanschultz/Documents/Code/hylla/valv/main/` — the primary work checkout (all real coding/building/testing/committing). The dev launches work orchestrators here; sessions from the bare-root one level up are steward orchestrators (cross-worktree oversight, not feature work). Global rules (Section 0 reasoning, output style) live at `~/.claude/CLAUDE.md`.

**This file is Valv's single source of truth for coordination + cross-cutting product/runtime/Go/CLI/MCP/testing rules** (no separate `AGENTS.md`). The per-drop *process* lives in `main/drops/WORKFLOW.md`; the drop *tree* in `main/PLAN.md`; the cascade *methodology* in `main/CASCADE_METHODOLOGY.md`. Read all three at cold-start + after every compaction — this file does NOT duplicate them.

## Product Direction

`valv` is a **macOS-first control plane for per-account isolated containerized agentic-dev workloads**. AI CLI launching (Codex + Claude Code) ships today; a generic `valv run --account <name> <command>` primitive will back the provider launchers (DROP_13). Follow `main/PLAN.md` for the active drop tree.

- Two pass-through containerized launchers ship: `valv codex` (primary) + `valv claude`.
- **Cross-provider in-container routing:** `valv claude`'s image contains the `codex` CLI AND cross-mounts the project's pinned Codex profile at `/home/valv/.codex` (`CODEX_HOME` set); mirror for `valv codex`. Per-project binding required.
- Roadmap: provider launchers → thin adapters over the generic per-account run primitive (DROP_13); declarative per-project toolchain `.valv/tools.toml` (DROP_11) + per-project composed images (DROP_12); per-account env-var maps threaded into launch (DROP_14, same name allowed across accounts); closed-by-default outbound network + per-project allowlist (DROP_15). Sibling-path-aware mounts (git worktrees) shipped.
- **Niche (2026-05-20 survey):** no competitor simultaneously offers closed-network+allowlist, declarative per-project toolchain-in-container, multi-identity isolation, AND sibling-path-aware mounts. That intersection + per-account credential isolation + Docker-runtime sandbox is Valv's differentiator. Management flows live outside the pass-through path; global account switching is optional convenience, not the core model.

## Platform Scope

macOS only. Docker required (Docker Desktop). No Linux/Windows support. Do NOT add cross-platform abstractions unless the dev explicitly asks.

## Repository Topology + Worktree Discipline

Flat **bare** Git repo at `/Users/evanschultz/Documents/Code/hylla/valv` (one level up — `HEAD`/`config`/`objects/`/`refs/`/`worktrees/` at top, no `.bare/` wrapper, no top-level `.git`). Day-to-day work is the `main/` worktree (`main/.git` → `gitdir: …/valv/worktrees/main`). Agent worklogs under `.worklog/` (gitignored). Root `.codex/config.toml` points `gopls mcp` at `main/`.

- The bare root is a steward-orchestrator scope (cross-worktree oversight), **never a coding checkout / never edits source.**
- **Always confirm `pwd` is `main/`** before edits/tests/commits/gopls. If unclear, `/select-checkout`; after switching/creating/retiring a checkout, `/gopls-sync`. Single visible checkout today; lanes only matter if a multi-lane setup is later introduced.

## Runtime Model

Preserve these boundaries:

- Go control plane. SQLite runtime metadata via `modernc.org/sqlite` (pure Go — **no CGO SQLite**). Docker isolation via `exec.Command` shelling to the `docker` CLI (**no Go Docker SDK**). Valv-managed profile homes under `~/Library/Application Support/valv/providers/<provider>/profiles/<account>/`.
- For `valv codex`/`valv claude`: pass provider-CLI args through as directly as possible; do NOT reinterpret provider CLI semantics or replace its session/resume logic unless the dev asks. Valv provides runtime isolation, project/profile resolution, cross-provider mounting, logging — nothing more.

## Coordination Model

Valv does NOT use Tillsyn. Three docs own coordination (no duplication): **`main/PLAN.md`** (drop tree + state + `blocked_by`), **`main/drops/WORKFLOW.md`** (per-drop lifecycle, drop dir shape, the **Agent Spawn Contract** pasted into every spawn, restart recovery), **this file** (orchestrator boundaries, agent bindings, cross-cutting rules). Per-drop artifacts under `main/drops/DROP_N_<NAME>/` (stamped from `_TEMPLATE/`, persists after close).

- **Read `main/PLAN.md` + `main/drops/WORKFLOW.md` at session start + after every compaction.**
- **Drop dir mds are the work-tracking source of truth.** Built-in `TaskCreate`/`TaskUpdate` are fine for a subagent's granular sub-steps or tiny orch reminders — anything durable goes in the active drop's `PLAN.md` (in-session tools evaporate on compaction). No markdown for work-tracking outside `main/drops/`.

## Drops

A **drop** = one PLAN.md entry + one `main/drops/` dir. Atomic when one builder can finish a unit cleanly, its acceptance is yes/no-verifiable, and its `paths`/`packages` footprint is clear — too large → add more units inside its `PLAN.md`, don't stretch one. Ordering: parent-child nesting (a drop can't close while a unit is incomplete) + `blocked_by` (no `depends_on`). State: per-drop in the drop dir `PLAN.md` header (`planning`/`building`/`done`/`blocked`); per-unit in the Planner section row (`todo`/`in_progress`/`done`/`blocked`); container-level in `main/PLAN.md`. Full lifecycle in WORKFLOW.md.

## Cascade Methodology + Orchestrator Autonomy

Canonical source: `main/CASCADE_METHODOLOGY.md` (cp'd from tillsyn — the SOURCE; reconcile toward it, never overwrite) + `main/HYLLA_BIN.md §5` (valv = use-only, Go agents, no FE). These rules UPGRADE WORKFLOW.md's linear per-phase reading toward an autonomous plan-down/build-up cascade; where a rule here conflicts with a literal WORKFLOW.md phase, **the rule here wins**. A **drop/plan node** decomposes into child plans + atomic **build droplets** (≈ units).

1. **Plan down, build up** — plan top-down into child plans + atomic droplets; build bottom-up (atoms first, integration follows once inputs green). Every plan node auto-gets a plan-QA pair (proof ∥ falsification); every build droplet auto-gets a build-QA gate.
2. **Recurse on atomicity — NO child cap.** A droplet = **1-2 small code blocks (≤80 LOC incl. tests, ≤3 files)**; a *code block* = one new/changed top-level production symbol OR one cohesive same-purpose edit cluster (a new type + a new helper + a different-function rewrite = SEPARATE blocks). ≥3 distinct production symbols (tests excluded), >80 LOC, or >3 files = OVER BUDGET → emit a `kind=plan` sub-plan, never an oversize build. **Plan-QA-falsification MEASURES this per droplet (never trusts the label) and re-measures EVERY droplet on any amendment.** "One coherent concern" is NOT a budget exception. Depth is multi-level + ASYMMETRIC (a shared interface sits as a shallow leaf with `blocked_by` from deeper consumers).
3. **Per-branch parallelism** — every code-independent node of every kind runs at once (sub-planners, plan-QA pairs, builders, build-QA gates); QA twins always a parallel pair; only `blocked_by` (a real shared file/package or must-exist-first symbol) serializes. A spurious `blocked_by` is an anti-pattern.
4. **`blocked_by` on a plan node gates its BUILDS, not its decomposition** — a planner decomposes against a dependency's spec'd shape; only builders wait on the built symbol. Sibling sub-planners launch as soon as the parent's plan-QA is green.
5. **Descent gate (per branch)** — a plan node's plan-QA pair must BOTH pass before it launches child planners or builds; plan-QA FAIL → wipe-and-replan that subtree.
6. **Droplet-level QA = the automated `mage` gate (NOT LLM)** — builder → mage gate green (`mage test`; add `mage integration`/`golden` for Docker/TUI paths) → orch closes the build-QA twins + commits (no push). LLM proof/falsification runs at the planner/integration level.
7. **Orch auto-advance** — drive to completion; never ask per tick. STOP only for: an unresolvable fork, a hard blocker, a QA-FAIL needing a design ruling, or a destructive/outward action (push/PR/Hylla ingest). "Should I fire the next level / the builders" is always yes.

## Orchestrator-as-Hub + Role Boundaries

The parent session is **the orchestrator**; every other role (builder, qa-proof, qa-falsification, planning, research) is a subagent. **The orchestrator NEVER writes Go code** — every `.go`/test/`magefile.go` change goes through a `ta-go-builder` subagent. Orch reads code for planning; edits markdown only (this file, `PLAN.md`, drop dir mds, `README.md`, agent `.md` files).

**Git is orchestrator-only.** Built-in agents are blocked from git-mutation by the `ta_action_gate.py` hook baseline (hardcoded, independent of `bash_deny`); codex agents by `--sandbox read-only` + hermetic execpolicy `prefix_rule(forbidden)`. The orch is the SOLE committer/pusher. Subagents get read-only git (`diff`/`status`/`log`/`show`) only; an agent that thinks it needs a commit STOPS and returns control. Rationale + proof: `main/BIN_AGENT_REPAIR_TRACKING.md`.

**Backend routing (hybrid):** Anthropic tiers (opus/sonnet/haiku) → built-in `Agent` tool `subagent_type=<persona>`, subscription billing. Codex tier → `bin/agent-dispatch.sh --role <persona> --cwd "$(pwd)" --prompt "<short pointer>"` (reads `.claude/agent-chains.sh`), ChatGPT subscription; on failure the dispatcher exits `CODEX_EXHAUSTED` on stderr → orch re-dispatches via the Agent tool `model=sonnet` (never a `claude -p` subprocess — retired 2026-05-21 to keep billing off `ANTHROPIC_API_KEY`). Mirrors `ta/main/docs/agent-backend-routing.md`. **`bin/agent-dispatch.sh` + `.claude/agent-chains.sh` are not yet installed in valv** (planned cp from ta) — until then every role spawns via the `Agent` tool on its primary Anthropic tier.

| Role | Persona | Anthropic tier (Agent tool) | Codex tier (dispatcher) | Edits Go? |
|---|---|---|---|---|
| Builder | `ta-go-builder` | haiku (primary), sonnet (orch escalation) | n/a | **Yes** (only role) |
| QA Proof | `ta-go-qa-proof` | opus | n/a | No |
| QA Falsification | `ta-go-qa-falsification` | sonnet (orch fallback), opus (escalation) | codex gpt-5.4 effort=high (primary) | No |
| Planning | `ta-go-planning` | sonnet (orch fallback), opus (escalation) | codex gpt-5.4 effort=high (primary) | No |
| Closeout | `ta-closeout` | opus | n/a | No |
| Research | built-in `Explore` | n/a | n/a | No |

Personas live in project-local `.claude/agents/` (not global) and reference Tillsyn tooling Valv doesn't use — every spawn carries the override preamble from WORKFLOW.md § "Agent Spawn Contract" (single source, not duplicated here); per-role appendix fields are in WORKFLOW.md § "Per-Role Spawn Appendices".

Role boundaries: **Builder** = only role that edits Go, via the Agent tool with the spawn contract; read-only git. **QA** (`ta-go-qa-proof`/`ta-go-qa-falsification`) = read/verify/write their own `*_QA_*.md`/return verdict, never edit code; read-only git. **Planner** (`ta-go-planning`) = fills + revises the drop's `PLAN.md` Planner section, never edits code; read-only git. **Closeout** proposes the commit message (orch runs it). **Dev** approves design calls in plan-QA discussion (Phase 3), reviews build-QA findings (Phase 5).

**Tool-call audit after every dispatch:** verify each claim against the actual stream — codex `mcp: <server>/<tool> (completed)` + claude-native JSON `tool_use` events. Self-reported "verdict: pass" is NOT authoritative; if the stream doesn't show the required calls, the work didn't happen → re-dispatch or finish orch-direct. Flag out-of-scope tool calls. (Once the dispatcher lands: Hylla `artifact_ref github.com/evanmschultz/valv@main`, pinned; inline `--prompt`/stdin only, NEVER `--prompt-file`.)

**Code is NEVER committed without per-unit QA passing; Hylla reingest is drop-end only** — both enforced inside WORKFLOW.md's phases. Follow WORKFLOW.md's phases in order, no skips/reorders; if a phase looks redundant, return the question to the dev.

## QA Discipline

No build unit is `done` without per-unit QA passing (gate, not suggestion). Two asymmetric parallel passes: **QA Proof** (`ta-go-qa-proof`) — evidence completeness, reasoning coherence, trace coverage (*does the evidence support the claim?*); **QA Falsification** (`ta-go-qa-falsification`) — counterexamples, alternate traces, hidden deps, contract mismatches, YAGNI (*can I construct a case where this is wrong?*). Plan-QA writes transient `PLAN_QA_*.md` (orch `git rm`s between rounds); build-QA appends rounds to durable `BUILDER_QA_*.md`. Full file-lifecycle table in WORKFLOW.md.

## Hylla + Evidence Sources

Evidence order: (1) **Hylla** (`mcp__hylla__*`) for committed Go — exhaust every mode (vector/keyword/graph-nav/refs) before `LSP`/`Read`/`Grep`/`Glob`; record any miss-forcing-fallback under `## Hylla Feedback` in `BUILDER_WORKLOG.md`; (2) **`git diff`** for changed-since-ingest; (3) **Context7 + `go doc` + gopls `LSP`** for external/language semantics. Hylla is Go-only (non-Go → Read/Grep/Glob). `laslig` isn't in Context7 — use Hylla (`artifact_ref=github.com/evanmschultz/laslig@main`) or `go doc`.

- **Artifact ref:** `github.com/evanmschultz/valv@main` (`@main` → latest ingest). **Hylla ingest is drop-end only**, orch-only, `enrichment_mode=full_enrichment`, from the GitHub remote, never before `git push` + `gh run watch` green. Subagents never ingest.
- **Context7 first:** before planning/coding/tests/QA/fixes, resolve the library id + query docs when an entry exists; if none, note it in the worklog and continue. Applies to build + QA agents.

## Semi-Formal Reasoning

For semantic/high-risk/ambiguous work, render an explicit `# Section 0` block (Premises / Evidence / Trace or cases / Conclusion / Unknowns). Full spec: `~/.claude/CLAUDE.md § "Semi-Formal Reasoning"`. The Agent Spawn Contract requires Section 0 from every subagent — but it stays in the orchestrator-facing response ONLY, never inside `PLAN.md`/`BUILDER_WORKLOG.md`/`*_QA_*.md`.

## Project Structure

Small Go-idiomatic layout; every internal package is an implementation detail (nothing public beyond the binary). Linear layered DAG: `internal/domain → internal/adapters → internal/services → (internal/cli | internal/tui)`; `cmd/valv` wires `cli` at the top. No cycles.

- `cmd/valv/` — cobra + Fang v2 entry; flag wiring, `RunE` dispatches into internal packages.
- `internal/domain/` — pure domain types + consumer-side interfaces (zero internal deps).
- `internal/adapters/` — concrete impls (SQLite store, Docker runtime, provider launcher, filesystem).
- `internal/services/` — use-case layer composing adapters.
- `internal/cli/` — cobra commands (`valv codex`/`claude`/`account …`/`image …`) + pass-through launcher.
- `internal/tui/` — Bubble Tea v2 management surfaces.
- `internal/{config,logging,output,pathutil,progress,project}/` — config (TOML via `BurntSushi/toml`), `charmbracelet/log` setup, render contracts, focused helpers.
- `magefile.go` — mage automation at root.

## Tech Stack

Production deps (authoritative list in `main/go.mod`): Go 1.26+ · `charm.land/fang/v2` (CLI polish, signal-aware root ctx) · `charm.land/bubbletea/v2` + `lipgloss/v2` + `bubbles/v2` (TUI) · `github.com/spf13/cobra` · `github.com/charmbracelet/log` · `github.com/evanmschultz/laslig` (TTY-aware output) · `modernc.org/sqlite` (pure-Go, **CGO SQLite forbidden**) · `github.com/modelcontextprotocol/go-sdk` (MCP) · `github.com/BurntSushi/toml` · `github.com/magefile/mage` · `github.com/testcontainers/testcontainers-go` (real Docker integration) · `charmbracelet/x/exp/teatest/v2` + `.../golden` + `creack/pty/v2` (TUI goldens + PTY transcripts). Dev tool: `mvdan.cc/gofumpt` (invoked from mage). **No Go Docker SDK** — Valv shells to `docker` via `exec.Command`; adding one belongs in this file as a new rule, not silently in a drop.

## CLI / TUI / Output

**Charm v2 only** — `fang/v2`, `bubbletea/v2`, `lipgloss/v2`, `bubbles` directly when needed. Do NOT use `huh` or legacy non-v2 Charm paths. Keep the direct CLI path clean; the management surface may use Bubble Tea. Use `.tmp/blick` as the Fang v2 reference (command structure, output policy, renderer separation, human/plain/json behavior).

- **Bubble Tea surfaces:** `teatest/v2` + golden regression on final rendered views; update goldens on intentional layout/style change. Valv-owned goldens cover Valv screens; attached external CLIs (Codex) ALSO need transcript-style golden coverage exercising the real subprocess path (steady-state Codex screen + interactive `/mcp` pass). Keep mage golden targets aligned with actual test packages. Docker-backed external golden/integration tests MUST clean up fixture containers (no leftover `valv-codex:test*`).
- **Aliases:** selective only (prefer `h`/`m`/`g`); no blanket one-letter aliases; keep terminology `account` (not `profile`) in help/examples/output; trailing `help`/`h` on branch commands when unambiguous; do NOT reinterpret trailing args on leaf/pass-through commands (`valv codex`); avoid ambiguity across `status`/`serve`/`switch`.
- **Output:** follow `blick` patterns; `Short`/`Long`/`Example` mandatory per visible command; deeper help is more explanatory + explains key output fields with realistic examples; clear placeholders (`personal`/`work`/`alternate-account`/`host-codex`, not `dev`); deterministic minimal human output; explicit empty states; slim machine-JSON with command-owned stable keys (not derived from heading copy); reduce raw subprocess noise unless that output IS the payload; mark dev-mode temp-home artifacts disposable + explain cleanup; cleanup targets Valv-managed artifacts by label (a `valv-` name prefix alone is not authority to delete); never remove anonymous volumes / unrelated unlabeled containers unless the dev asks for destructive cleanup; signal-aware root context so shutdown hooks run on `Ctrl-C`/`SIGTERM`.

## Containerized Provider Runtime

For interactive `valv codex` / `valv claude`:

- Do NOT run the provider CLI inside a Valv Bubble Tea wrapper — launch it as an **attached subprocess through Docker with the real terminal attached**, using Docker's init process (signal handling + child reaping behave like a normal terminal app).
- Ensure a coherent in-container user + `HOME` (don't rely on the host absolute profile path doubling as the Linux home); normalize the in-container `TERM` (don't pass host-only values the slim image can't interpret).
- Preserve provider auth/session/memory by mounting the selected Valv profile home, normalizing the in-container mount target to a normal Linux home layout; if generating container-only config overlays, keep auth/session files durable while making runtime config container-safe.
- Attached Docker subprocesses use direct stdio attachment, validated with PTY-backed integration tests (don't rely on unsupported controlling-terminal syscalls on the Docker CLI process).
- **Cross-provider mount:** `valv claude` mounts the project's pinned Codex profile at `/home/valv/.codex` (`CODEX_HOME` set) so an in-container `codex exec`/`codex mcp-serve` auto-routes to that project's login; mirror for `valv codex`. Skipped when the other provider isn't bound → cross-call fails with the CLI's native "not logged in".
- **Worktree gitdir mount:** when the project root has a `.git` linkfile (git worktree), resolve via `pathutil.ResolveWorktreeGitDir`, follow `gitdir:`, read `commondir`, and bind-mount the bare-repo path (path-transparent) so in-container git follows the linkfile. Both `claude` + `codex` `PrepareRuntime` do this.

## MCP Translation

Host Codex config can't be reused blindly inside Linux containers. For containerized Codex runs:

- Inspect profile-level + (when present) project-level Codex config; generate a **container-safe runtime overlay** rather than executing host-only MCP entries unchanged.
- Translate host-loopback MCP URLs (`127.0.0.1`/`localhost`) → `host.docker.internal` where the target is the macOS host. Don't hardcode user-specific MCP server names/paths. Preserve project-level precedence over profile-level when merging.
- Treat stdio MCP entries generically: command exists in-image → run in-container; depends on a host-only path/binary → expose via a Valv-managed host bridge (don't run the macOS binary in Linux). Keep host-bridge subprocess lifetime bound to the prepared-runtime lifecycle, not a single request. If a bridge can't be created, omit that entry + emit a warning (never write malformed MCP stanzas).
- If a warm runtime record persisted in `starting` later fails startup, mark it failed / clean it up — no orphan `starting` rows.

## Auth UX

For containerized interactive auth:

- Support the device-code flow cleanly for isolated container profiles; do NOT assume browser-localhost callbacks work from inside Docker without explicit port-publishing/relay. Keep help/error text explicit about which flows work in disposable containerized profiles vs host-bound `~/.codex`/`~/.claude` reuse.
- When creating isolated accounts, seed baseline provider config from the default host-backed account when available (so MCP/tool config isn't silently dropped) while keeping auth/session state isolated. Prefer host-side provider login before launching the container (browser login completes on macOS without Docker callback issues). Keep steady-state launches quiet — pre-launch notices move to setup/login/failure paths.
- **Terminology:** user-facing management/help/output prefers `account` over `profile` (internal storage may keep `Profile`; don't advertise `profile` as a user-facing alias). `account add` is the one-command default (create/reuse home, ensure host login when needed, bind the current project unless `--no-bind`); `--home`/`--skip-login` are expert overrides. `account list`/inspect surfaces auth identity (incl. email for ChatGPT-backed Codex) when safely inferable.
- **Account deletion:** `Service.DeleteProfile` removes the on-disk managed-home dir + the DB row, guarded by `EvalSymlinks`-resolved `HasPrefix` against the canonical `providerRoot`; custom `--home` paths outside `providerRoot` are left untouched (prevents the next `account add` short-circuiting on leftover `.credentials.json`).

## Logging

`github.com/charmbracelet/log`. Structured logs throughout runtime-critical paths; debug logging for profile/project resolution, Docker lifecycle, storage, API execution, provider launch. Practical local retention/cleanup. Users see clean Fang/Lipgloss failures; detailed operational context lives in structured logs.

## Go Development Rules

- **Structure/style:** interface-first boundaries near the consumer (not shared dump packages); smallest concrete design (no abstraction not pulling real weight); TDD-first where practical; idiomatic Go (`gofumpt` via `mage test`, `go vet` via `go test`); Go doc on every exported identifier (start with its name); thread `context.Context` through runtime/API/storage/long-running ops; explicit constructors over package-global mutable state; prefer stdlib; small structs/functions; table-driven tests.
- **Errors:** wrap with `fmt.Errorf("…: %w", err)` at every info-adding boundary; bubble to the command/runtime boundary, never swallow; sentinels `ErrFoo` + `errors.Is`/`As` (never string-match); preserve root-cause context for logs; no vague generic errors hiding failures.
- **Concurrency:** every goroutine `ctx.Done()`-cancellable (`RunE` threads `cmd.Context()`; main uses a signal-aware root ctx); `defer` cleanup (closers/unlocks/`cancel()`) on the line after acquisition; no shared mutable state without sync (prefer channels; unexported `sync.Mutex` on the owning struct); race detector always on (`mage test` runs `-race`).
- **Tests:** co-located `*_test.go`, table-driven, behavior-oriented; `-race -cover` via mage; prefer **real** SQLite/filesystem/Docker (`testcontainers-go`) over mocks — mocks/fakes ONLY when no practical real option or isolating a narrow pure-domain concern (document why in the worklog); **do NOT default to mock-heavy unit tests for runtime/provider/storage/Docker/CLI launch-path**; Bubble Tea via `teatest/v2` + goldens, attached subprocess paths (`valv codex`) via transcript goldens; `testdata/` next to the test (no shared top-level). No placeholder test files.
- **Mage discipline:** plain `mage <target>` from `main/`; **no `GOCACHE`/`GOMODCACHE`/`GOPATH` ad-hoc overrides** (if a Go command fails on sandbox restriction, stop + report — don't wrap in workarounds); missing/broken target → add/fix it, never bypass with raw `go`.
- **Dependencies:** ask the dev to run `go get`/module updates; no `GOPROXY=direct`/`GOSUMDB=off`/checksum bypass.
- **Shared foundations first:** establish config loading, logger + log-path policy, output/error render contracts, and core domain interfaces as stable shared packages BEFORE broad parallel implementation — all tracks need consistent config/logging/rendering.

## Build Verification

**Per-unit** (build-QA, Phase 5): builder runs `mage testPkg <pkg>` for touched packages (or the golden/integration target when touching TUI/Docker paths). **Drop-end** (Phase 6): `mage test` from `main/` → `mage integration`/`golden` when relevant → `git push` → `gh run watch --exit-status` until green → `mage build` before handing CLI testing back. **NEVER raw `go test`/`build`/`run`/`vet`/`gofumpt`** — always `mage <target>`; fix a buggy target, never bypass. `mage test` enforces the 70%-per-package coverage floor. After every push run `gh run watch` directly (not via repo `bin` wrappers) and confirm before considering the push complete.

| Target | Command | When |
|---|---|---|
| `mage build` | `go build -o ./valv ./cmd/valv` | final local binary check |
| `mage test` | gofumpt check + `go test -race -cover -count=1 ./...` + 70%-per-pkg gate | canonical local gate |
| `mage testPkg <pkg>` | gofumpt check + `go test -race -cover -count=1 <pkg>` + gate | per-package build-QA |
| `mage integration` | `go test -tags=integration -count=1 ./internal/cli` | Docker integration + external golden |
| `mage golden` / `goldenUpdate` | Bubble Tea + external Codex transcript goldens (`-args -update` to refresh) | golden regression |
| `mage run "…"` | build then run with args | smoke |
| `mage dev:home` / `dev:reset` / `dev:clean` / `dev:run "…"` | disposable dev-home lifecycle (preserves host Docker CLI config so `buildx` works) | dev-mode without dirtying real `$HOME` |

**Done means:** plan scope fully implemented; nothing deferred left undocumented; all tests pass; each package ≥70% coverage; TDD followed for new work; behavior/logs/docs aligned; every build unit reviewed by both QA passes (behavior + edge cases, not just diffs); failed QA/tests fixed before complete. Keep root docs aligned with implementation, `magefile.go` as the command source of truth, CI + local recipes aligned, and a clean dev-mode path. Docker builds: prefer `docker buildx build --load`; keep dev/default image tags separable; suppress avoidable package-manager noise.

## Skill and Slash Command Routing

| Command | When |
|---|---|
| `/qa-proof` / `/qa-falsification` | proof / falsification QA (inside subagent defs; orch just spawns) |
| `/select-checkout` | confirm the active visible checkout |
| `/gopls-sync` | verify gopls targets `main/` |
| `semi-formal-reasoning` | explicit Section 0 certificate |

`/plan-from-hylla` is Tillsyn-coupled — Valv does not use it. Planner work happens via `ta-go-planning` per WORKFLOW.md § "Phase 1".

## Git Commit Format

Conventional-commit `type(scope): message`. All lowercase except proper nouns/acronyms (HTTP, CLI, JSON, TUI, MCP, SDK). **Subject-line only — no body, no bullet lists, no co-authored-by trailers, no trailing period.** Per-file detail belongs in the PR description, not `git log`. Types: `feat`, `fix`, `refactor`, `chore`, `docs`, `test`, `ci`, `style`, `perf`. Keep under ~72 chars — if it won't fit, split the change. Examples: `feat(codex): attach Docker subprocess with normalized TERM` · `fix(adapters): clean up orphan starting runtime rows on boot failure` · `docs(drop-3): clear plan qa round 2, route to planner`.

## Safety

- Never delete files/directories without explicit dev approval. Never run commands outside the repo root `/Users/evanschultz/Documents/Code/hylla/valv`. Never push without explicit request. Keep secrets out of committed config.

## Recovery After Session Restart

Filesystem + git, no Tillsyn. Full procedure in WORKFLOW.md § "Recovery After Restart". Quick form: 1. `git status`. 2. `git log --oneline -20`. 3. read `main/PLAN.md` (container states). 4. list `main/drops/*/PLAN.md` headers (per-drop phase). 5. per active drop: `PLAN_QA_*.md` present = mid-plan-QA; absent + `BUILDER_WORKLOG.md` exists = mid-build; header `state: done` = closed. 6. per active unit: scan the latest `## Unit N.M — Round K` in `BUILDER_WORKLOG.md` + both `BUILDER_QA_*.md` for the next step.
