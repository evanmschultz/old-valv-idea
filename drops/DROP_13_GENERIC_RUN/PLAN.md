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

Add a generic per-account launch primitive, `valv run --account <name> [--provider <provider>] <command>`, that preserves the existing isolation stack: provider-managed credential home, sibling-path-aware mounts, per-project overlay image selection, cross-provider in-container routing, and provider-specific auth semantics. Re-derive `valv claude` and `valv codex` as thin CLI and service adapters over the same launch path, without adding new persistence or broadening scope into DROP_14/15 concerns.

### Schema Decisions

- Use a new package, `internal/services/run`, as the shared launch primitive, and slim `internal/services/claude` / `internal/services/codex` to thin wrappers over it while preserving each package's public `Service.Run` API.
  Evidence: `internal/services/claude/service.go:128-222` and `internal/services/codex/service.go:121-214` duplicate the same launch orchestration, while provider-specific differences are limited to runtime prep, shared-home policy, label text, and notice copy; `CLAUDE.md:9-16` states the architectural pivot that the generic `valv run` primitive backs thin provider-specific launchers.
- Do **not** introduce a new per-account image model in DROP_13.
  Evidence: image selection is already provider-derived via `claudeImageRef` / `codexImageRef` plus `resolveProjectImage` (`internal/cli/claude.go:115`, `internal/cli/codex.go:120`, `internal/cli/operator_helpers.go:421-470`).
- `valv run` is explicit-account only and does **not** mutate project bindings or auto-create missing project rows.
  Evidence: existing `--account` launch paths already treat explicit account selection as read-only with respect to bindings (`internal/cli/claude_setup.go:23-54`, `internal/cli/codex_setup.go:14-52`), and the explicit-override launch path already converts a missing `ProjectByRoot` row into `domain.ErrUnboundProject` (`internal/services/claude/service.go:128-148`, `internal/services/codex/service.go:121-141`).
- Generic command execution should override the image entrypoint instead of changing base images.
  Evidence: base images hardcode `ENTRYPOINT ["codex"]` / `ENTRYPOINT ["claude"]` (`internal/services/images/service.go:976`, `1042`), and Docker request plumbing already supports `Extra` args before the image token (`internal/adapters/docker/types.go:43-60`, `141-218`), which is enough for `--entrypoint`.
- Cross-provider routing continues to mean "mount the other provider's bound profile for the same detected project when present; otherwise skip silently."
  Evidence: `internal/services/claude/service.go:162-187`, `internal/services/codex/service.go:155-178`.
- Reuse `resolveAccountByName` for cross-provider account resolution and `--provider` semantics rather than inventing a new lookup helper.
  Evidence: `internal/cli/manage.go:1118-1162` already handles unique match, explicit provider selection, not-found, and multi-provider collision cases.
- `valv run` local-flag extraction is prefix-only rather than whole-argv scanning: consume only `--account` / `--provider` flags that appear before the first non-flag positional, then treat the remaining argv as target-command passthrough even if later tokens spell Valv-local flags.
  Evidence: `internal/cli/account_flag.go:3-15` currently strips `--account` by scanning until `--`, and `internal/cli/account_flag_test.go:60-106` proves the helper currently extracts mid-argv `--account` occurrences; `go doc github.com/spf13/cobra.Command.DisableFlagParsing` says when `DisableFlagParsing` is true, "all flags will be passed to the command as arguments."
- Unbound-project recovery guidance from `valv run` preserves provider context from account resolution: explicit `--provider` should remain explicit in the suggested `valv account bind` command, while uniquely inferred providers can use the shorter provider-less suggestion.
  Evidence: `internal/cli/manage.go:343-410` shows `account bind` accepts either `<account>` plus optional `--provider` or explicit positional provider; existing provider-specific non-TTY guidance already uses provider-qualified bind suggestions in `internal/cli/claude_setup.go:96-101` and `internal/cli/codex_setup.go:90-95`.
- Preserve the existing `VALV_CLAUDE_IMAGE` / `VALV_CODEX_IMAGE` short-circuit behavior by keeping provider launchers on the existing `claudeImageRef` / `codexImageRef` -> `resolveProjectImage` path.
  Evidence: `internal/cli/claude.go:111-117` and `internal/cli/codex.go:116-123` still resolve provider base images before launch, and `internal/cli/operator_helpers.go:429-452` already returns the base ref unchanged when `VALV_<PROVIDER>_IMAGE` is set.

### Acceptance Criteria (drop-level)

- `valv run --account <name> [--provider <provider>] <command> [args...]` resolves `<name>` across providers without mutating bindings, then launches inside the resolved provider image with the same mount/env/isolation behavior as current runtime launchers.
- When `--provider` is supplied, `valv run` uses the same disambiguation semantics as the existing account-resolution helper; tests cover `valv run --account work --provider claude <command>` resolving correctly when `work` exists in both Claude and Codex.
- If `valv run --account <name> [--provider <provider>]` is invoked from a directory whose detected project has no persisted Valv project row, the command errors with clear bind guidance but does **not** auto-create or auto-bind a project row. The suggestion preserves runtime provider context: `valv account bind <name> --provider <provider>` when `--provider` was explicitly supplied, and `valv account bind <name>` when the provider was inferred unambiguously from `resolveAccountByName`.
- The resolved provider still determines auth readiness, base image selection, overlay-image selection, and cross-provider mount behavior.
- `valv claude` and `valv codex` keep their current help/version image-only shortcuts and current `--account` semantics, but their main launch paths delegate to the new primitive.
- Existing DROP_10/12 behavior remains intact: sibling-path-aware mounts, project overlay images, and cross-provider in-container routing still work.
- `VALV_CLAUDE_IMAGE` / `VALV_CODEX_IMAGE` override behavior remains intact after the rewire: the override still short-circuits overlay-image building instead of being swallowed by the shared-launch extraction.
- README and CLI help reflect the product-direction language that `valv run` is the generic primitive and `valv claude` / `valv codex` are first-class adapters over it.

### Units

#### Unit 13.1 — Shared provider-agnostic launch service

State: `todo`

Paths: `internal/services/run/service.go` (new, not yet in tree), `internal/services/run/service_test.go` (new, not yet in tree)

Packages: `internal/services/run`

Evidence: `internal/services/claude/service.go:128-222` (`Service.Run`), `internal/services/codex/service.go:121-214` (`Service.Run`), `internal/services/images/service.go:976` / `1042` (`ENTRYPOINT ["codex"]` / `["claude"]`), `internal/adapters/docker/types.go:43-60` and `141-218` (`ContainerRunRequest.Extra` is emitted before the image)

Acceptance:
- Introduce one shared service that accepts a resolved `domain.Profile`, detected project cwd, provider descriptor, image ref, TTY/stdin/user/temp-root/logger/notices, and an optional explicit command override.
- When no explicit command override is provided, the service preserves current provider-image entrypoint behavior.
- When an explicit command override is provided, the service emits `--entrypoint <command[0]>` via `ContainerRunRequest.Extra` and forwards the remaining tokens unchanged as container args.
- The shared service preserves the current within-project-root guard, project/profile labels, mount/env passthrough, cross-provider binding lookup, and runtime cleanup behavior now implemented separately in `internal/services/claude` and `internal/services/codex`.
- Tests prove both provider descriptors work, cross-provider mount lookup still skips on `ErrNotFound`, and explicit-command mode does not require Docker type changes outside this package.

Blocked by: none

#### Unit 13.2 — Add `valv run` and root wiring

State: `todo`

Paths: `internal/cli/run.go` (new, not yet in tree), `internal/cli/run_test.go` (new, not yet in tree), `internal/cli/root.go`, `internal/cli/account_flag.go`, `internal/cli/account_flag_test.go`

Packages: `internal/cli`

Evidence: `internal/cli/manage.go:1118-1162` (`resolveAccountByName`), `internal/cli/account_flag.go:3-15` and `internal/cli/account_flag_test.go:60-106` (current helper scans until `--` and still strips mid-argv `--account`, which is the fragility this unit must remove), `internal/cli/root.go:120-139` (runtime command registration pattern), `internal/cli/claude.go:46-63` and `internal/cli/codex.go:51-68` (`DisableFlagParsing: true`, help/version fast paths), `go doc github.com/spf13/cobra.Command.DisableFlagParsing` ("all flags will be passed to the command as arguments"), `internal/cli/manage.go:343-410` (`account bind` accepts either `<account>` or `<account> --provider <provider>`), `internal/cli/claude_setup.go:96-101` and `internal/cli/codex_setup.go:90-95` (existing provider-specific bind guidance), `internal/services/claude/service.go:128-148` and `internal/services/codex/service.go:121-141` (explicit override path converts missing project rows to `domain.ErrUnboundProject`)

Acceptance:
- Register a new runtime command, `valv run`, in `internal/cli/root.go`.
- `valv run` manually consumes only its local `--account` and `--provider` flags that appear before the first non-flag positional; once the first non-flag positional is encountered, all subsequent args, including later `--account` / `--provider`, are preserved as target-command args. Do not require a `--` separator.
- `valv run --help` prints `valv run`'s own help when no target command remains after leading-local-flag stripping, while `valv run <command> --help` passes `--help` through to the target command.
- The command requires `--account`, accepts optional `--provider`, resolves the named account via `resolveAccountByName`, runs provider-specific auth readiness via `ensureManagedAccountReady`, resolves the provider image via existing helpers, and then invokes the new shared run service with an explicit command override.
- The command does not auto-bind, pick, or mutate project bindings; explicit account selection remains read-only with respect to bindings.
- When the resolved launch path reports `domain.ErrUnboundProject` because the detected project has no persisted project row, the command returns a clear user-facing error suggesting `valv account bind <name>` to create the project record. Preserve provider context in that suggestion: use `valv account bind <name> --provider <provider>` when `--provider` was explicitly supplied at runtime, and use `valv account bind <name>` when the provider came unambiguously from the resolved account.
- Tests cover missing `--account`, missing target command, unknown account, multi-provider name collision, explicit `--provider` disambiguation, the no-project-row error path with both bind-suggestion variants, `valv run --help` versus target `--help`, and passthrough preservation for target-command args and flags.
- Tests explicitly cover: (a) `valv run --account A cmd --account B` passes `--account B` to the target command unchanged, and (b) `valv run cmd --account A` treats `--account A` as a target arg and fails only because `valv run` itself is missing the required leading `--account`.

Blocked by: `13.1`

#### Unit 13.3 — Re-derive `valv claude` and Claude service as thin adapters

State: `todo`

Paths: `internal/cli/claude.go`, `internal/cli/claude_test.go`, `internal/services/claude/service.go`, `internal/services/claude/service_test.go`

Packages: `internal/cli`, `internal/services/claude`

Evidence: `internal/cli/claude.go:53-139` (`runClaudeCommand`), `internal/cli/claude.go:143-180` (`runClaudeImageOnlyCommand`), `internal/cli/claude_setup.go:23-112` (`ensureClaudeBindingReady`), `internal/services/claude/service.go:128-222` (`Service.Run`), `CLAUDE.md:9-16` (provider launchers become thin adapters over the generic primitive)

Acceptance:
- Keep `newClaudeCommand`, `claudeArgsSkipProjectBinding`, and `runClaudeImageOnlyCommand` as the image-only help/version path.
- Replace the current main launch body with a thin adapter that still resolves binding/account via `ensureClaudeBindingReady`, still consumes `--account` before skip-binding checks, but delegates the actual container launch to the shared primitive from Unit 13.1.
- Reduce `internal/services/claude/service.go` to a thin wrapper around `internal/services/run` while preserving the existing public `claudeservice.Service.Run(ctx, cwd, args)` API. The wrapper owns only Claude-specific runtime prep / notice / label policy, not a second copy of generic launch orchestration.
- Preserve Claude-specific behavior: in-container auth, provider label/help copy, Claude image resolution, and other-provider Codex cross-mount semantics.
- Existing `internal/cli/claude_test.go` and `internal/services/claude/service_test.go` coverage remains green after the rewire.

Blocked by: `13.2`

#### Unit 13.4 — Re-derive `valv codex` and Codex service as thin adapters

State: `todo`

Paths: `internal/cli/codex.go`, `internal/cli/codex_test.go`, `internal/services/codex/service.go`, `internal/services/codex/service_test.go`

Packages: `internal/cli`, `internal/services/codex`

Evidence: `internal/cli/codex.go:58-145` (`runCodexCommand`), `internal/cli/codex.go:148-185` (`runCodexImageOnlyCommand`), `internal/cli/codex_setup.go:14-118` (`ensureCodexAccountReadyForLaunch`), `internal/cli/codex.go:208-218` (`codexArgsSkipAccountReady`), `internal/services/codex/service.go:121-214` (`Service.Run`), `CLAUDE.md:9-16` (provider launchers become thin adapters over the generic primitive)

Acceptance:
- Keep `newCodexCommand`, `codexArgsSkipProjectBinding`, `codexArgsSkipAccountReady`, and `runCodexImageOnlyCommand`.
- Replace the current main launch body with a thin adapter that still resolves binding/account via `ensureCodexAccountReadyForLaunch`, still consumes `--account` before skip-binding checks, but delegates the actual container launch to the shared primitive from Unit 13.1.
- Reduce `internal/services/codex/service.go` to a thin wrapper around `internal/services/run` while preserving the existing public `codexservice.Service.Run(ctx, cwd, args)` API. The wrapper owns only Codex-specific runtime prep / shared-home / notice policy, not a third copy of generic launch orchestration.
- Preserve Codex-specific behavior: host-side auth readiness, provider help/version behavior, Codex image resolution, shared-host-home logic where applicable, and other-provider Claude cross-mount semantics.
- Existing `internal/cli/codex_test.go` and `internal/services/codex/service_test.go` coverage remains green after the rewire.

Blocked by: `13.3`

#### Unit 13.5 — Rewrite README around the generic primitive

State: `todo`

Paths: `README.md`

Packages: none

Evidence: `README.md:3-10` still describes Valv mainly as a control plane for AI CLIs and only calls out `valv codex`; `CLAUDE.md:9-20` already states the updated product direction with `valv run --account <name> <command>` as the generic primitive

Acceptance:
- README intro explicitly describes Valv as the per-account isolated containerized workload runner, with AI CLIs as the first shipped case rather than the whole product.
- README examples include `valv run --account <name> <command>` and still show `valv codex` / `valv claude` as adapter commands.
- README text does not contradict shipped behavior from DROP_5/7/8/10/12 around per-account homes, cross-provider routing, overlay images, and provider-specific adapters over the generic primitive.

Blocked by: `13.4`

### Notes For Builder Agents

- Use `resolveAccountByName` as-is from `internal/cli/manage.go`; do not add a second cross-provider account lookup helper unless a compile boundary forces it.
- Preserve the public `Service.Run` surfaces in `internal/services/claude` and `internal/services/codex`; the goal is to move shared orchestration into `internal/services/run`, not to change downstream call signatures.
- Do not add new SQLite tables, columns, config fields, or per-account image settings in DROP_13.
- Treat `valv run` as explicit-account launch only. It must not auto-bind projects, write binding rows, or auto-create a missing project record; surface bind guidance instead.
- Keep `--provider` on `valv run` semantically aligned with the existing account-switch/account-resolution behavior: explicit provider narrows lookup, absent provider performs cross-provider resolution with collision errors.
- For `valv run`, local flag stripping is prefix-only: consume `--account` / `--provider` only before the first non-flag positional, then leave the rest of argv untouched even if later tokens match Valv-local flag names.
- When surfacing the unbound-project bind hint from `valv run`, preserve explicit `--provider` in the suggested bind command; when the provider was inferred uniquely from `resolveAccountByName`, use the shorter `valv account bind <name>` form.
- Preserve the existing project-sensitive semantics: provider image selection stays provider-derived, overlay images still come from `resolveProjectImage`, and other-provider cross-mounts still depend on the bound other-provider profile for the detected project.
- Preserve `VALV_CLAUDE_IMAGE` / `VALV_CODEX_IMAGE` short-circuit behavior by keeping provider launchers on the existing image-resolution path; do not accidentally force overlay-image work when an override is set.
- Reuse `ContainerRunRequest.Extra` for `--entrypoint`; do not widen the Docker adapter surface unless the existing field proves insufficient in code.
- Keep tests table-driven where the behavior is parse-heavy (`--account` / `--provider` extraction, collision handling, help routing, passthrough preservation).
- Because Units 13.2-13.4 all touch `internal/cli`, respect the strict order above even though the file paths differ.
