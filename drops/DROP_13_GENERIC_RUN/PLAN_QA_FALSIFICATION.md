verdict: fail

## Counterexamples

### F1. Shared runtime seam is still underspecified relative to the committed provider runtime contract

Evidence:
- `internal/adapters/providers/claude/runtime.go:41-59` and `internal/adapters/providers/codex/runtime.go:39-52` show the current provider-prepared runtime surface is not just mounts and env. Both runtimes carry `Env`, `EnvPassthrough`, `Mounts`, `Warnings`, and cleanup via `Close()`.
- `internal/services/claude/service.go:180-194` and `internal/services/codex/service.go:171-185` consume that richer surface today: they defer cleanup, emit warning notices, and thread `request.EnvPassthrough` into the Docker request.

Counterexample:
- Unit 13.1 currently says the new shared service accepts a new "provider-prepared runtime state" and tests prove only that mounts/env are honored unchanged and cleanup runs.
- A builder can satisfy that text with a narrower shared state such as `{Env, Mounts, Close}` and still meet the written tests, while silently dropping `EnvPassthrough` and warning propagation. That would regress current behavior for Codex MCP env passthrough and runtime notice emission without violating the current Round 4 acceptance.

Narrow fix:
- In Unit 13.1, define the minimum shared runtime contract explicitly: `Env`, `EnvPassthrough`, `Mounts`, `Warnings`, and `Close() error` (or an equivalent interface with those semantics).
- Extend Unit 13.1 tests to assert passthrough vars and warning notice propagation, not just mounts/env plus cleanup.

### F2. `valv run` still lacks a collision-plus-override test that proves explicit `--provider` controls image override resolution

Evidence:
- `resolveAccountByName` resolves duplicate account names by explicit provider selection in `internal/cli/manage.go:1118-1162`, and the committed table test already covers that collision path in `internal/cli/manage_test.go:1003-1079`.
- `resolveProjectImage` chooses the override env var strictly from the provider argument in `internal/cli/operator_helpers.go:421-470`.
- Round 4 Unit 13.2 adds override-path coverage "for both providers", but only as separate provider cases; it does not combine that with the duplicate-name `--provider` path.

Counterexample:
- Create account name `work` in both providers.
- Run `valv run --account work --provider claude bash -lc true` from a project with non-empty `.valv/tools.toml`, set `VALV_CLAUDE_IMAGE`, and leave `VALV_CODEX_IMAGE` unset.
- An implementation can resolve the profile correctly through `resolveAccountByName`, then still route image resolution through the wrong provider-specific helper if the resolved provider is not the value threaded into `resolveProjectImage` and the shared run service call. The current Unit 13.2 acceptance would still pass if it only runs isolated Claude and Codex override cases without the collision path.

Narrow fix:
- Add one Unit 13.2 acceptance test that combines:
  - a duplicated account name across Claude and Codex,
  - explicit `--provider`,
  - a non-empty `.valv/tools.toml`,
  - only the selected provider's `VALV_<PROVIDER>_IMAGE` set.
- Assert one warning for the selected provider, zero overlay-image Docker calls, and launch using the selected provider's override-derived base image.

## YAGNI Pressure Check

- The new `internal/services/run` package is justified. The committed `Service.Run` bodies in `internal/services/claude/service.go` and `internal/services/codex/service.go` still duplicate the same launch orchestration around provider prep, request build, attached/non-attached execution, and cleanup.
- The strict `13.1 -> 13.2 -> 13.3 -> 13.4 -> 13.5` chain is conservative but tolerable. `13.3` and `13.4` could likely be parallel after `13.2`, but serializing them does not force a new abstraction or broaden scope; it mainly trades some throughput for less concurrent churn in `internal/cli` and the new shared seam.

## Hidden Dependency Check

- Worktree-subdir behavior is still provider-runtime-owned, not CLI-owned. The actual worktree gitdir mount comes from `pathutil.ResolveWorktreeGitDir` inside `internal/adapters/providers/claude/runtime.go` and `internal/adapters/providers/codex/runtime.go`, so Unit 13.1 must preserve provider-supplied mounts unchanged when `valv run` adds `--entrypoint`.
- Cross-provider mount success still implicitly depends on a valid `Profile.HomePath` whenever `ProfileByID` succeeds. `domain.NewProfile` normalizes home paths on creation in `internal/domain/model.go:77-90`, but `internal/adapters/sqlite/store.go:284-305` trusts stored `home_path` on read. If the planner wants silent-skip semantics to cover corrupt profile rows instead of only lookup failures, that needs to be made explicit; the current repo contract does not prove it.
