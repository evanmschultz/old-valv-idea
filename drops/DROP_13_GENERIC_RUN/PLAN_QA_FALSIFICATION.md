# DROP_13 — Plan QA Falsification — Round 1

**Verdict:** FAIL (concrete blockers)

## Attacks attempted

### A1 — `internal/services/run` duplicates the current seam unless the old services are retired
- Hypothesis:
  Unit 13.1 adds a third launch service while Units 13.3/13.4 only rewire CLI entrypoints, so the current `internal/services/claude` and `internal/services/codex` launch stacks can survive unchanged.
- Evidence:
  `drops/DROP_13_GENERIC_RUN/PLAN.md:59-129` adds `internal/services/run` and rewires `internal/cli/{claude,codex}.go`, but no unit requires slimming or deleting the existing provider launch services. The current provider services each already contain full launch orchestration: override-path project lookup, cross-provider binding lookup, provider runtime prep, docker request assembly, and notice emission in `internal/services/claude/service.go:123-197` and `internal/services/codex/service.go:116-188`.
- Outcome:
  BLOCKER
- Detail:
  A builder can satisfy the current unit text by calling the new shared service from the CLI while leaving both old service packages compiled and duplicated. That misses the architectural-pivot claim. The plan needs an explicit acceptance point that either makes the old services thin wrappers over the shared primitive or removes/shrinks them so there is one launch seam, not three.

### A2 — `domain.Profile` alone is not rich enough, but the plan can mitigate that
- Hypothesis:
  A shared launch path cannot be driven by `domain.Profile` alone because the two providers need different runtime policy.
- Evidence:
  `internal/domain/model.go:21-27` shows `domain.Profile` only carries `ID`, `Provider`, `Name`, `HomePath`, and `CreatedAt`. Codex-specific shared-home policy is computed separately in `internal/services/codex/service.go:170-178`, while Claude and Codex use different runtime preparers and notice/env behavior.
- Outcome:
  mitigated
- Detail:
  Unit 13.1 already says the shared service also accepts a provider descriptor. That descriptor must carry runtime-prep, shared-home, label, and notice policy; otherwise the abstraction will collapse. This is acceptable if the planner makes that descriptor concrete during implementation.

### A3 — Docker `--entrypoint` ordering is valid through `ContainerRunRequest.Extra`
- Hypothesis:
  `--entrypoint` could land after the image token and fail, which would invalidate the plan's "override the entrypoint" approach.
- Evidence:
  `internal/adapters/docker/types.go:141-218` appends `request.Extra` before `request.Image.String()` and then appends `request.Args`. `internal/adapters/docker/command.go:15-21` confirms `Executor.Run` goes through `BuildRunArgs`. Existing test `TestBuildRunArgs` passed locally via `go test ./internal/adapters/docker -run 'TestBuildRunArgs'`.
- Outcome:
  mitigated
- Detail:
  The entrypoint override path is mechanically sound with the current Docker adapter shape. No blocker here.

### A4 — Claude/Codex auth asymmetry can stay above the shared service
- Hypothesis:
  Rewiring both launchers to one shared run path will break because Claude auth is in-container while Codex auth is host-side.
- Evidence:
  `internal/cli/account_auth.go:32-39` dispatches provider-specific readiness checks. `internal/cli/claude_setup.go:23-110` intentionally does not call `ensureManagedAccountReady`, while `internal/cli/codex_setup.go:14-116` does. Unit 13.2 plans to call `ensureManagedAccountReady` after cross-provider account resolution for `valv run`; Units 13.3/13.4 keep the existing provider-specific setup helpers.
- Outcome:
  mitigated
- Detail:
  The asymmetry is real, but it belongs in CLI/setup resolution, not in the shared container-launch core. The plan is acceptable on this point.

### A5 — README scope is narrow enough for one unit, but help-text work is elsewhere
- Hypothesis:
  Unit 13.5 is too small because the current docs surface still frames `valv codex` as the primary product.
- Evidence:
  `README.md:3-10` still describes Valv as an AI-CLI control plane centered on `valv codex`, and `README.md:68` still talks about `valv claude`. The root and provider help text lives in CLI files already touched by Units 13.2-13.4 (`internal/cli/root.go:120-139`, `internal/cli/claude.go`, `internal/cli/codex.go`).
- Outcome:
  accepted
- Detail:
  One README unit is enough for README itself. CLI/help drift should be handled in Units 13.2-13.4, so this is not a blocker.

### A6 — Explicit `--account` launch is not actually proven to work without an existing project record
- Hypothesis:
  The planner cites current `--account` behavior as evidence for "read-only with respect to bindings," but the current launch services still require a persisted project row, so `valv run --account ...` can fail on a fresh/unbound project or outside a project root.
- Evidence:
  The planner relies on `internal/cli/claude_setup.go:47-54` and `internal/cli/codex_setup.go:45-52` via `drops/DROP_13_GENERIC_RUN/PLAN.md:40-41`, but those paths only prove override resolution returns a profile without writing a binding row. Existing tests confirm that no project row is written for override resolution in `internal/cli/claude_setup_test.go:16-59` and `internal/cli/codex_setup_test.go:17-63`. Both provider services then still do `DetectFrom` followed by `ProjectByRoot`, and return `domain.ErrUnboundProject` if the project record is missing in `internal/services/claude/service.go:128-148` and `internal/services/codex/service.go:121-141`. `internal/project/project.go:27-52` shows detection falls back to the current directory even when no `.git` marker exists.
- Outcome:
  BLOCKER
- Detail:
  Concrete counterexample: from a fresh repo or any directory with no persisted Valv project row, `valv run --account work <command>` can resolve the account and still fail at launch because the shared path has no project record to attach isolation state to, while the plan also forbids mutating bindings. The plan needs to choose one of these explicitly:
  1. `valv run` still requires an existing Valv project record / bound project context.
  2. Explicit-account launch creates or uses a project record without writing a binding row.
  3. `valv run` has a real no-project mode with reduced semantics.

### A7 — Cross-provider account-name collision has no planned disambiguation path
- Hypothesis:
  The planner wants to reuse `resolveAccountByName`, but Unit 13.2 only specifies `--account`, so collisions across providers become unrecoverable user errors.
- Evidence:
  `internal/cli/manage.go:1123-1132` supports explicit disambiguation only when the caller passes `providerFlag`. `internal/cli/manage.go:1156-1161` otherwise raises a collision error that explicitly tells the user to use `--provider`. Unit 13.2 in `drops/DROP_13_GENERIC_RUN/PLAN.md:88-93` requires `--account` and tests a multi-provider collision, but does not add a `--provider` flag or any alternate selector.
- Outcome:
  BLOCKER
- Detail:
  Concrete counterexample: if account `work` exists under both Codex and Claude, `valv run --account work <command>` must fail with "use --provider to specify which one", but the planned command surface gives the user no way to do that. The plan must add `--provider` or some equivalent disambiguation mechanism.

## Non-blocking gaps

- The session's `gopls` attachment is pointed at the wrong workspace and returned `hylla` symbols instead of `valv`, so live references/definitions were unavailable for this pass. I used Hylla, repo reads, `git diff`/`git show`, `go doc`, and targeted local tests instead.
