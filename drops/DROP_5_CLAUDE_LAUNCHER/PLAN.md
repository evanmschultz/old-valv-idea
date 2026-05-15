# DROP_5 — CLAUDE LAUNCHER

**State:** planning
**Blocked by:** DROP_4 (done)
**Paths (expected):** `internal/adapters/providers/claude/` (new package — `profile.go`, `account.go`, `runtime.go` + tests), `internal/services/claude/` (new package — `service.go` + tests), `internal/cli/claude.go` (new), `internal/cli/claude_test.go` (new), `internal/cli/root.go` (edit — register `newClaudeCommand` in the `runtime` group + add Claude examples to root `Example` string), `internal/services/manage/service.go` (edit — flip the DROP_2 `DefaultHostProfile(ProviderClaude)` sentinel stub to `claudeprovider.DefaultHostProfile(homeDir)`), `internal/cli/account_auth.go` (edit — flip the DROP_2 `case domain.ProviderClaude: return nil` no-op stubs to real Claude-aware bodies where the design calls for it; expectation is mostly still no-op for v1 since the focus plan §3.2 specifies device-code auth happens inside the container at launch time), `magefile.go` (edit — add small `Install` target)
**Packages (expected):** `internal/adapters/providers/claude` (new), `internal/services/claude` (new), `internal/cli` (edits)
**PLAN.md ref:** main/PLAN.md → DROP_5_CLAUDE_LAUNCHER row
**Workflow:** main/drops/WORKFLOW.md
**Started:** 2026-05-14
**Closed:** —

## Scope

**Dogfood milestone.** Land everything needed to run `valv claude` end-to-end on a per-project basis: the Claude provider adapter trio (`profile.go`, `account.go`, `runtime.go`) mirroring `internal/adapters/providers/codex/`, the Claude launch service mirroring `internal/services/codex/service.go`, the `valv claude` pass-through CLI mirroring `internal/cli/codex.go`, and the wiring that makes `newClaudeCommand` discoverable under the `runtime` group. After this drop closes, the dev can `valv claude` from inside a project bound to a Claude managed account, `claude --resume` works because session state lives in the bind-mounted `/home/valv/.claude` dir, and Codex behavior is unchanged.

Collapses old DROP_5 (adapter) + DROP_6 (service+CLI) from the prior plan numbering. Globalswitch parity, `valv account list` cross-provider, `valv account switch` cross-provider, and full TUI picker golden parity are NOT in this drop — they live in DROP_7. `valv account add claude [name]` is NOT in this drop — that's DROP_6 (a single-feature drop right after this one).

Includes a 5-line `mage install` target so the dev can `mage install` after this drop closes and have `valv` on PATH for dogfooding.

## Dev-Confirmed Decisions (2026-05-14)

1. **Claude host-default account = isolated-first.** `claudeprovider.DefaultHostProfile(homeDir)` returns `(name "default", homePath filepath.Join(homeDir, ".valv", "providers", "claude", "profiles", "default"), nil)`, NOT `~/.claude`. Reason: macOS Claude credentials live in keychain; `~/.claude` on host is functionally empty for auth purposes. The Valv-managed path starts empty and gets device-code-auth on first `valv claude` run inside the container.
2. **Cross-provider account name collision = require `--provider` when ambiguous.** Not in scope for this drop's CLI — relevant when `valv account switch` lands in DROP_7. Recorded here so the adapter / service layer doesn't bake in a different assumption.
3. **Globalswitch is NOT extended to Claude in this drop.** Deferred to DROP_7. This drop must compile cleanly with `internal/services/globalswitch/service.go` still rejecting non-Codex providers (existing behavior at `service.go:86`). No edit to globalswitch in this drop.

## Planner

<Filled by go-planning-agent in Phase 1. Atomic units of work below. The planner should decompose this drop into ~3–5 units along package boundaries (adapter package as one unit, service package as one unit, CLI + wiring as one or two units, mage install target as one small unit). Per-unit acceptance criteria must be yes/no-verifiable. See main/drops/WORKFLOW.md § "Phase 1 — Plan" for deliverable rules.>

## Notes

- **Mechanical drop — trimmed cascade applies** per memory `feedback_trimmed_cascade_for_mechanical_drops.md`. Single planner spawn, no parallel plan-QA proof+falsification subagent spawn. Orchestrator + dev review the planner output before approval. Per-unit build-QA (both proof and falsification) stays in place — that gate is universal.
- **Copy-adapt template.** The Codex adapter (`internal/adapters/providers/codex/{profile.go, account.go, runtime.go}`) is the structural template. The Claude versions are the same shape with these substitutions: `CODEX_HOME` → `CLAUDE_CONFIG_DIR`, `ContainerCodexDir = "/home/valv/.codex"` → `ContainerClaudeDir = "/home/valv/.claude"`, `codex` entrypoint → `claude` entrypoint, Codex JWT credential parsing → Claude `.credentials.json` presence check (per focus-plan §3.4). The Codex adapter's `bridge.go` (MCP host bridge) is intentionally NOT ported — Claude MCP translation is out of scope for v1.
- **`terminalEnvPassthrough` duplication.** Per focus-plan §3.2 v1 decision: duplicate `terminalEnvPassthrough` (and any other Codex-adapter helper the Claude adapter needs) into `internal/adapters/providers/claude/runtime.go`. Dedupe into a shared helper is DROP_9 cleanup.
- **`claude --resume` works for free.** Claude CLI persists session state under `$CLAUDE_CONFIG_DIR/projects/<hash>/`. Since `$CLAUDE_CONFIG_DIR=/home/valv/.claude` inside the container and `/home/valv/.claude` is bind-mounted to the Valv-managed account home on the host (`~/.valv/providers/claude/profiles/<account>/`), sessions persist across container restarts automatically. No special adapter code required beyond the standard bind-mount. Planner should NOT invent special `--resume` handling — verify by inspecting how Codex's runtime handles its own state dir (it doesn't; same pattern applies).
- **`mage install` sub-step.** Add an `Install` mage target to `magefile.go` that runs `go install ./cmd/valv` from the `main/` working dir. ~5 lines. The dev uses `mage install` post-drop-close to put `valv` on PATH for dogfooding. Builder should mirror the `Build` target's structure (currently `go build -o ./valv ./cmd/valv`).
- **First-launch flow for verification.** On first `valv claude` in a fresh bound project, device-code auth runs inside the container, writes credentials to the bind-mounted account home, subsequent launches reuse them. This is the dogfood acceptance test — `mage build`, bind a project to a fresh Claude account, run `valv claude`, complete device-code auth, verify a follow-up `valv claude --version` does not re-prompt. The dev runs this manually after CI is green per AGENTS.md §12.
- **Coverage floor.** 70% per package (AGENTS.md §11). Adapter and service packages are new, so plan test coverage up front. The existing test fixtures in `internal/adapters/providers/codex/` are the structural template for the Claude versions.
