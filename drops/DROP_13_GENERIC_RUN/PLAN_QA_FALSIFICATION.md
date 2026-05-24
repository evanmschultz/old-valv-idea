verdict: fail

## Counterexamples

### F1. The unhappy collision path still lacks an override-side-effect test

Evidence:
- `resolveAccountByName` returns a duplicate-name collision when `--provider` is absent and the same account exists in multiple providers (`internal/cli/manage.go:1118-1162`).
- The current provider launchers resolve binding/account before image work (`internal/cli/claude.go:76-118`, `internal/cli/codex.go:83-123`).
- Unit 13.2 now covers the explicit-`--provider` + override happy path, but its acceptance does not pin the duplicate-name + no-`--provider` + override unhappy path (`drops/DROP_13_GENERIC_RUN/PLAN.md:107-110`).

Counterexample:
- Create account name `work` in both Claude and Codex.
- Invoke `valv run --account work sh -lc true` from a repo with a non-empty `.valv/tools.toml`.
- Set `VALV_CLAUDE_IMAGE`, leave `VALV_CODEX_IMAGE` unset.
- A builder can resolve project image or emit override warnings before calling `resolveAccountByName`, then still return the duplicate-name collision. The current Round 5 plan would accept that implementation if its tests cover only:
  - generic duplicate-name collision, and
  - explicit-`--provider` override success.

Narrow fix:
- Add one Unit 13.2 test for duplicate account name + no `--provider` + non-empty manifest + one provider override env set.
- Assert the command fails with the collision error before any override warning is emitted and before any overlay-image Docker call occurs.

### F2. Unit 13.1 still allows a mount-superset regression

Evidence:
- Both current service implementations prepend exactly one project-root mount and then append provider-prepared mounts verbatim (`internal/services/claude/service.go:307-327`, `internal/services/codex/service.go:311-331`).
- Unit 13.1 says provider-prepared mounts "pass through unchanged" and calls out the worktree gitdir mount specifically (`drops/DROP_13_GENERIC_RUN/PLAN.md:84-86`).
- Hylla's committed summaries for `internal/adapters/providers/codex` and `internal/pathutil` confirm the provider runtime owns mount assembly, including worktree gitdir handling.

Counterexample:
- The shared run service forwards every prepared mount unchanged, so the worktree gitdir mount is still present, but it also injects an extra default mount such as `/tmp`.
- A subset-style test would still pass even though DROP_10 isolation behavior changed: the container now sees an extra host path that the current provider services do not expose.

Narrow fix:
- Tighten Unit 13.1's test requirement from "prepared mounts are present unchanged" to exact equality:
  - the request mounts must equal `[]MountSpec{projectRootMount} + prepared.Mounts`
  - no extra mounts beyond that sequence are allowed.

### F3. `13.4` is still over-serialized behind `13.3`

Evidence:
- After `13.1`, the shared run seam is explicitly defined; after `13.2`, root wiring and `valv run` land.
- The planned file sets for `13.3` and `13.4` are disjoint except for sharing the `internal/cli` package namespace (`drops/DROP_13_GENERIC_RUN/PLAN.md:114-152`).
- `gopls` references show each provider service `Run` is called from its own command file (`internal/cli/claude.go:137`, `internal/cli/codex.go:142`), not through a shared provider launcher.

Counterexample:
- Once `13.1` and `13.2` are complete, a Codex adapter rewire can proceed without waiting on Claude adapter edits. The current `13.1 -> 13.2 -> 13.3 -> 13.4 -> 13.5` chain blocks that parallelizable work for no semantic reason.

Narrow fix:
- Change `13.4` to `Blocked by: 13.2`.
- Keep a note that `13.3` and `13.4` both touch `internal/cli`, so builders should coordinate on merge order, but do not encode that as a hard semantic dependency.

## YAGNI Check

- The new `internal/services/run` seam is still justified. The committed Claude and Codex `Service.Run` bodies duplicate the same request-build, attached/non-attached execution, warning handling, and cleanup orchestration.
- No extra abstraction is justified beyond that shared seam. The remaining fixes are acceptance/test tightening and dependency-graph cleanup, not new packages or new provider lookup helpers.

## Hidden Dependency Check

- `PreparedRuntime.ContainerHome` is not a surviving counterexample. The current service layer never reads `prepared.ContainerHome`; it consumes `prepared.Env`, `prepared.EnvPassthrough`, `prepared.Mounts`, `prepared.Warnings`, and `prepared.Close()` (`internal/services/claude/service.go:191-220`, `internal/services/codex/service.go:182-212`). Omitting `ContainerHome` from the shared-service minimum contract is therefore consistent with the committed callers.
- The worktree-gitdir evidence lines resolve. The plan's cited ranges cover the actual `pathutil.ResolveWorktreeGitDir` mount append sites in Claude (`internal/adapters/providers/claude/runtime.go:144-145`) and Codex (`internal/adapters/providers/codex/runtime.go:114-115`).
- Cross-provider mount behavior still depends on provider runtime prep and the silent-skip `ProfileByID` branch in each provider service, so keeping other-provider lookup on the wrapper side remains necessary (`internal/services/claude/service.go:166-187`, `internal/services/codex/service.go:159-178`).
