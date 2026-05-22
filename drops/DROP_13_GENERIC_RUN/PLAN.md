# DROP_13 — GENERIC_RUN

**State:** planning
**Blocked by:** DROP_12 (done)
**Paths (expected):** `cmd/valv/` (new `run` cobra command), `internal/cli/` (new `run.go`), `internal/services/` (possible new `run` service or reuse of existing claude/codex services), `internal/adapters/providers/` (potential generic provider adapter or refactor of claude/codex into shared core)
**Packages (expected):** `internal/cli/`, possibly `internal/services/run/` (new), possibly refactors to `internal/adapters/providers/claude/`, `internal/adapters/providers/codex/`
**PLAN.md ref:** main/PLAN.md → DROP_13_GENERIC_RUN row
**Workflow:** main/drops/WORKFLOW.md
**Started:** 2026-05-22
**Closed:** —

## Scope

**Architectural pivot.** Add `valv run --account <name> <command>` generic primitive. `valv codex` and `valv claude` are re-derived as thin adapters over this primitive — same per-account isolation, mount, env, cross-provider routing, network policy semantics. README + help reflect the Product Direction rewrite in `main/CLAUDE.md` (per-account isolated containerized agentic-dev workloads, AI CLI launching as first-class case but not the only one).

The generic primitive must preserve everything DROP_5/7/8/10/12 added:

- Per-account credential homes under `~/Library/Application Support/valv/providers/<provider>/profiles/<account>/`
- Sibling-path-aware mounts (worktree gitdir handling)
- Cross-provider in-container routing (when both providers bound to a project)
- Project-binding-aware account resolution
- Per-project overlay image build (DROP_12 `EnsureProjectImage`)
- `VALV_<PROVIDER>_IMAGE` env override path
- Provider-specific CLI argument pass-through

The challenge: most of the existing claude/codex launchers are tightly coupled to provider-specific knowledge (image refs, auth flows, MCP overlay generation, cross-provider mount targets). The generic primitive needs a clean seam where "what to run inside the container" is the variable input, while everything around it (isolation, mounts, env, policy) is shared.

## Planner

### Objective

Add a generic per-account launch primitive, `valv run --account <name> <command>`, that preserves the existing isolation stack: provider-managed credential home, sibling-path-aware mounts, per-project overlay image selection, cross-provider in-container routing, and provider-specific auth semantics. Re-derive `valv claude` and `valv codex` as thin CLI adapters over the same launch path, without adding new persistence or broadening scope into DROP_14/15 concerns.

### Schema Decisions

- Use a new package, `internal/services/run`, as the shared launch primitive.
  Evidence: `internal/services/claude/service.go:128-222` and `internal/services/codex/service.go:121-214` duplicate the same launch orchestration, while provider-specific differences are limited to runtime prep, shared-home policy, label text, and notice copy.
- Do **not** introduce a new per-account image model in DROP_13.
  Evidence: image selection is already provider-derived via `claudeImageRef` / `codexImageRef` plus `resolveProjectImage` (`internal/cli/claude.go:115`, `internal/cli/codex.go:120`, `internal/cli/operator_helpers.go:421-470`).
- `valv run` is explicit-account only and does **not** mutate project bindings.
  Evidence: existing `--account` launch paths already treat explicit account selection as read-only with respect to bindings (`internal/cli/claude_setup.go:47-54`, `internal/cli/codex_setup.go:45-52`).
- Generic command execution should override the image entrypoint instead of changing base images.
  Evidence: base images hardcode `ENTRYPOINT ["codex"]` / `ENTRYPOINT ["claude"]` (`internal/services/images/service.go:976`, `1042`), and Docker request plumbing already supports `Extra` args before the image token (`internal/adapters/docker/types.go:43-60`, `133-218`), which is enough for `--entrypoint`.
- Cross-provider routing continues to mean "mount the other provider's bound profile for the same detected project when present; otherwise skip silently."
  Evidence: `internal/services/claude/service.go:162-187`, `internal/services/codex/service.go:155-178`.
- Reuse `resolveAccountByName` for cross-provider account resolution rather than inventing a new lookup helper.
  Evidence: `internal/cli/manage.go:1118-1162` already handles unique match, not-found, and multi-provider collision cases.

### Acceptance Criteria (drop-level)

- `valv run --account <name> <command> [args...]` resolves `<name>` across providers without mutating bindings, then launches inside the resolved provider image with the same mount/env/isolation behavior as current runtime launchers.
- The resolved provider still determines auth readiness, base image selection, overlay-image selection, and cross-provider mount behavior.
- `valv claude` and `valv codex` keep their current help/version image-only shortcuts and current `--account` semantics, but their main launch paths delegate to the new primitive.
- Existing DROP_10/12 behavior remains intact: sibling-path-aware mounts, project overlay images, and cross-provider in-container routing still work.
- README and CLI help reflect the product-direction language that `valv run` is the generic primitive and `valv claude` / `valv codex` are first-class adapters over it.

### Units

#### Unit 13.1 — Shared provider-agnostic launch service

State: `todo`

Paths: `internal/services/run/service.go` (new, not yet in tree), `internal/services/run/service_test.go` (new, not yet in tree)

Packages: `internal/services/run`

Evidence: `internal/services/claude/service.go:128-222` (`Service.Run`), `internal/services/codex/service.go:121-214` (`Service.Run`), `internal/services/images/service.go:976` / `1042` (`ENTRYPOINT ["codex"]` / `["claude"]`), `internal/adapters/docker/types.go:43-60` and `133-218` (`ContainerRunRequest.Extra` is emitted before the image)

Acceptance:
- Introduce one shared service that accepts a resolved `domain.Profile`, detected project cwd, provider descriptor, image ref, TTY/stdin/user/temp-root/logger/notices, and an optional explicit command override.
- When no explicit command override is provided, the service preserves current provider-image entrypoint behavior.
- When an explicit command override is provided, the service emits `--entrypoint <command[0]>` via `ContainerRunRequest.Extra` and forwards the remaining tokens unchanged as container args.
- The shared service preserves the current within-project-root guard, project/profile labels, mount/env passthrough, cross-provider binding lookup, and runtime cleanup behavior now implemented separately in `internal/services/claude` and `internal/services/codex`.
- Tests prove both provider descriptors work, cross-provider mount lookup still skips on `ErrNotFound`, and explicit-command mode does not require Docker type changes outside this package.

Blocked by: none

#### Unit 13.2 — Add `valv run` and root wiring

State: `todo`

Paths: `internal/cli/run.go` (new, not yet in tree), `internal/cli/run_test.go` (new, not yet in tree), `internal/cli/root.go`

Packages: `internal/cli`

Evidence: `internal/cli/manage.go:1118-1162` (`resolveAccountByName`), `internal/cli/account_auth.go:37-46` (`ensureManagedAccountReady` provider dispatch), `internal/cli/claude.go:28-50` and `internal/cli/codex.go:33-55` (`DisableFlagParsing: true`, `cobra.ArbitraryArgs`), `go doc github.com/spf13/cobra.Command.DisableFlagParsing` ("all flags will be passed to the command as arguments")

Acceptance:
- Register a new runtime command, `valv run`, in `internal/cli/root.go`.
- `valv run` manually consumes only its local `--account` flag while leaving the target command and its flags untouched; do not require a `--` separator.
- The command requires `--account`, resolves the named account across providers via `resolveAccountByName`, runs provider-specific auth readiness via `ensureManagedAccountReady`, resolves the provider image via existing helpers, and then invokes the new shared run service with an explicit command override.
- The command does not auto-bind, pick, or mutate project bindings; explicit account selection remains read-only with respect to bindings.
- Tests cover missing `--account`, unknown account, multi-provider name collision, and passthrough preservation for target-command args and flags.

Blocked by: `13.1`

#### Unit 13.3 — Re-derive `valv claude` as a thin adapter

State: `todo`

Paths: `internal/cli/claude.go`, `internal/cli/claude_test.go`

Packages: `internal/cli`

Evidence: `internal/cli/claude.go:53-139` (`runClaudeCommand`), `internal/cli/claude.go:143-180` (`runClaudeImageOnlyCommand`), `internal/cli/claude_setup.go:23-112` (`ensureClaudeBindingReady`), Hylla/gopls caller evidence shows the non-test blast radius of the Claude launch path is the CLI entrypoint itself

Acceptance:
- Keep `newClaudeCommand`, `claudeArgsSkipProjectBinding`, and `runClaudeImageOnlyCommand` as the image-only help/version path.
- Replace the current main launch body with a thin adapter that still resolves binding/account via `ensureClaudeBindingReady`, still consumes `--account` before skip-binding checks, but delegates the actual container launch to the shared primitive from Unit 13.1.
- Preserve Claude-specific behavior: in-container auth, provider label/help copy, Claude image resolution, and other-provider codex cross-mount semantics.
- Existing `internal/cli/claude_test.go` coverage remains green after the rewire.

Blocked by: `13.2`

#### Unit 13.4 — Re-derive `valv codex` as a thin adapter

State: `todo`

Paths: `internal/cli/codex.go`, `internal/cli/codex_test.go`

Packages: `internal/cli`

Evidence: `internal/cli/codex.go:58-145` (`runCodexCommand`), `internal/cli/codex.go:148-185` (`runCodexImageOnlyCommand`), `internal/cli/codex_setup.go:14-118` (`ensureCodexAccountReadyForLaunch`), `internal/cli/codex.go:208-218` (`codexArgsSkipAccountReady`)

Acceptance:
- Keep `newCodexCommand`, `codexArgsSkipProjectBinding`, `codexArgsSkipAccountReady`, and `runCodexImageOnlyCommand`.
- Replace the current main launch body with a thin adapter that still resolves binding/account via `ensureCodexAccountReadyForLaunch`, still consumes `--account` before skip-binding checks, but delegates the actual container launch to the shared primitive from Unit 13.1.
- Preserve Codex-specific behavior: host-side auth readiness, provider help/version behavior, Codex image resolution, shared-host-home logic where applicable, and other-provider Claude cross-mount semantics.
- Existing `internal/cli/codex_test.go` coverage remains green after the rewire.

Blocked by: `13.3`

#### Unit 13.5 — Rewrite README around the generic primitive

State: `todo`

Paths: `README.md`

Packages: none

Evidence: `README.md:3-10` still describes Valv mainly as a control plane for AI CLIs and only calls out `valv codex`; `CLAUDE.md:9-20` already states the updated product direction with `valv run --account <name> <command>` as the generic primitive

Acceptance:
- README intro explicitly describes Valv as the per-account isolated containerized workload runner, with AI CLIs as the first shipped case rather than the whole product.
- README examples include `valv run --account <name> <command>` and still show `valv codex` / `valv claude` as adapter commands.
- README text does not contradict shipped behavior from DROP_5/7/8/10/12 around per-account homes, cross-provider routing, and overlay images.

Blocked by: `13.4`

### Notes For Builder Agents

- Use `resolveAccountByName` as-is from `internal/cli/manage.go`; do not add a second cross-provider account lookup helper unless a compile boundary forces it.
- Do not add new SQLite tables, columns, config fields, or per-account image settings in DROP_13.
- Treat `valv run` as explicit-account launch only. It should not auto-bind projects or write binding rows.
- Preserve the existing project-sensitive semantics: provider image selection stays provider-derived, overlay images still come from `resolveProjectImage`, and other-provider cross-mounts still depend on the bound other-provider profile for the detected project.
- Reuse `ContainerRunRequest.Extra` for `--entrypoint`; do not widen the Docker adapter surface unless the existing field proves insufficient in code.
- Keep tests table-driven where the behavior is parse-heavy (`--account` extraction, collision handling, passthrough preservation).
- Because Units 13.2-13.4 all touch `internal/cli`, respect the strict order above even though the file paths differ.
