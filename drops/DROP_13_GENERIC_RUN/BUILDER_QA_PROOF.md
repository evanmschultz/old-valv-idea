# DROP_13 — Builder QA Proof

Append a `## Unit 13.M — Round K` section per QA pass. See `main/drops/WORKFLOW.md` § "Phase 5 — Build QA (per unit)" for what each section should contain.

## Unit 13.1 — Round 1

verdict: pass

### Scope of verification

Unit 13.1 — Shared provider-agnostic launch service. New package `internal/services/run/`. Audited against the twelve checks supplied in the spawn prompt and the eight bullets under PLAN.md "Acceptance" for Unit 13.1 (`drops/DROP_13_GENERIC_RUN/PLAN.md:80-86`).

### Files audited

- `internal/services/run/service.go` — new (357 lines).
- `internal/services/run/service_test.go` — new (670 lines).
- `drops/DROP_13_GENERIC_RUN/PLAN.md` — state flip `todo` → `done` on Unit 13.1 row.
- `drops/DROP_13_GENERIC_RUN/BUILDER_WORKLOG.md` — Round 1 entry appended.

`git show --stat c810331` confirms the unit-13.1 commit touched **exactly** these four files. No edits to `internal/adapters/docker/`, `internal/services/claude/`, `internal/services/codex/`, `internal/cli/`, `internal/domain/`, or `magefile.go`. `git diff c810331~1 c810331 -- internal/adapters/docker/` is empty.

### Mage targets I ran

- `mage testPkg ./internal/services/run`
  - `42 tests passed`, `0 failed`, `0 skipped`
  - Package coverage: `82.4%` (above the 70% per-package floor required by `main/CLAUDE.md` and the 60% mage-enforced floor)
  - `-race` enabled by the mage target — race-detector clean.

Numbers match the builder's claim exactly.

### Per-acceptance audit

**1. `Service` dependency set.**
`Options` struct at `internal/services/run/service.go:86-96` accepts `Executor`, `Image`, `User`, `TTY`, `Stdin`, `Logger`, `Notices`, `Now`, `Provider`. `LaunchRequest` at `service.go:118-133` accepts `ProjectRoot`, `WorkingDir`, `ProjectID`, `ProfileID`, `ProjectName`, `Prepared`, `Args`, `Command`. Together this covers Profile (via `ProfileID` label stamping), project cwd (`WorkingDir` + `ProjectRoot`), provider descriptor (`Provider`), image ref (`Image`), TTY/stdin/user/logger/notices, provider-prepared runtime state (`Prepared`), and optional command override (`Command`). Pass.

**2. PreparedRuntime contract minimum surface — no `ContainerHome`.**
`PreparedRuntime` declared at `service.go:67-73` with exactly five fields: `Env map[string]string`, `EnvPassthrough []string`, `Mounts []docker.MountSpec`, `Warnings []string`, `Cleanup func() error`. `Close()` wrapper at `service.go:77-82` invokes `Cleanup` when non-nil. No `ContainerHome` field anywhere in the file — confirmed by full read of `service.go` lines 1–358 and `service_test.go` lines 1–670. Round 5 falsification finding honored. Pass.

**3. Override mode emits `--entrypoint <command[0]>` via Extra.**
`applyCommandOverride` at `service.go:273-280` returns `extra = ["--entrypoint", command[0]]` and `args = command[1:]` when `len(command) > 0`. Threaded into the request at `service.go:262`. Test `TestRunWithCommandOverrideInjectsEntrypoint` (`service_test.go:201-242`) asserts `executor.got.Extra == ["--entrypoint", "bash"]` and `executor.got.Args == ["-lc", "echo hi"]` via `reflect.DeepEqual`, parameterized over both providers. Pass.

**4. No-override mode preserves provider-image entrypoint.**
`applyCommandOverride` early-return at `service.go:274-276` returns `extra = nil` when `len(command) == 0`. Test `TestRunNoCommandOverridePreservesEntrypoint` (`service_test.go:153-196`) iterates `executor.got.Extra` and fails if any token equals `"--entrypoint"`, parameterized over both providers. Pass.

**5. EnvPassthrough flows from prepared runtime.**
`buildRequest` copies `EnvPassthrough: launch.Prepared.EnvPassthrough` at `service.go:247`. Test `TestRunEnvPassthroughFlowsFromPrepared` (`service_test.go:247-278`) asserts `reflect.DeepEqual(executor.got.EnvPassthrough, passthrough)` for `[]string{"COLORTERM", "LANG", "LC_CTYPE"}`, parameterized over both providers. Pass.

**6. Warnings propagate to notices.**
`emitNotices` at `service.go:282-299` writes each warning to `s.notices` with the provider-specific `NoticePrefix`. Called from `Run` at `service.go:184`. Test `TestRunPropagatesWarningsToNotices` (`service_test.go:282-320`) uses a `recordingNotices` writer and asserts every warning string and the provider prefix appears in the output, parameterized over both providers. Additional test `TestRunSuppressesNoticesOnTTY` (`service_test.go:518-560`) confirms the TTY-suppression branch — visible-output sink stays empty under TTY mode. Pass.

**7. Mounts exact equality via reflect.DeepEqual.**
`buildRequest` builds `mounts` at `service.go:235-238` as `append([]docker.MountSpec{docker.NewMountSpec(projectRoot, projectRoot, false)}, launch.Prepared.Mounts...)`. Threaded into request at `service.go:255`. Test `TestRunMountsExactEqualityProjectRootThenPrepared` (`service_test.go:326-366`) constructs three prepared mounts (provider home, cross-mount, worktree-gitdir-shaped) and asserts `reflect.DeepEqual(executor.got.Mounts, want)` where `want = []MountSpec{projectRootMount} ++ preparedMounts`. No `/tmp` or any other default mount inserted — the deep-equal check would fail if so. Parameterized over both providers. Pass.

**8. Within-project-root guard preserved.**
`withinProjectRoot` at `service.go:322-334` mirrors the helper in the existing claude/codex services: handles `rel == "."`, rejects `rel == ".."` and `rel == "..<separator>...`. `buildRequest` calls it at `service.go:227-233` and returns `working directory %q is outside project root %q` on rejection. Tests `TestRunRejectsWorkingDirectoryOutsideProjectRoot` (`service_test.go:370-396`, `/tmp/project` root vs `/tmp/other` cwd) and `TestRunRejectsSiblingPathThatSharesProjectPrefix` (`service_test.go:400-426`, `/tmp/project` vs `/tmp/project2`) both assert `outside project root` error, parameterized over both providers. The sibling test directly proves the rejection is not a naive `HasPrefix` check. Pass.

**9. Close() cleanup on success AND failure.**
`Run` body at `service.go:178-182` registers `defer func() { ... request.Prepared.Close() ... }()` BEFORE `buildRequest` or `executor.Run` are called. The defer fires on:
- successful return (covered by `TestRunNoCommandOverridePreservesEntrypoint:191-193` — asserts `prepared.Closed() == 1` after success);
- executor failure (covered by `TestRunPreparedCloseRunsOnExecutorFailure` at `service_test.go:430-462` — sets `executor.runErr = errors.New("docker exploded")`, asserts the error bubbles wrapped AND `prepared.Closed() == 1`).
The cleanup counter uses `atomic.AddInt32`/`atomic.LoadInt32` (`service_test.go:53, 61`) so the race detector stays clean. Pass.

**10. Provider-agnostic test parameterization.**
`providerDescriptors()` at `service_test.go:66-79` returns Claude (`Name:"claude"`, `ContainerNamePrefix:"valv-claude-interactive"`, `NoticePrefix:"Valv note"`) and Codex (`Name:"codex"`, `ContainerNamePrefix:"valv-codex-interactive"`, `NoticePrefix:"Valv MCP note"`) descriptors. Every behavioral test in the file loops `for _, provider := range providerDescriptors()` and runs the assertion as a subtest under `t.Run(provider.Name, ...)`. The only non-parameterized test is `TestRunValidatesLaunchRequest` (descriptor-agnostic validation). Both Claude and Codex subtests appear in the `mage testPkg` 42-test count. Pass.

**11. No Docker adapter surface widening.**
`git diff c810331~1 c810331 -- internal/adapters/docker/` is empty (`wc -l` returns 1, accounting for the trailing newline; substantive change count is 0). `internal/adapters/docker/types.go` was not touched. `applyCommandOverride` reuses the existing `ContainerRunRequest.Extra` field that, per PLAN.md evidence, already places tokens before the image token in `BuildRunArgs`. Pass.

**12. Scope: no edits outside `internal/services/run/` + state flip + worklog append.**
`git show --stat c810331` (above) confirms exactly four files changed in this unit's commit, matching the WORKFLOW.md Phase 4 contract for Unit 13.1:
- `internal/services/run/service.go` (new) — owned by this unit
- `internal/services/run/service_test.go` (new) — owned by this unit
- `drops/DROP_13_GENERIC_RUN/PLAN.md` — only change is the `todo` → `done` state flip on the Unit 13.1 row (verified via `git diff c810331~1 c810331 -- drops/DROP_13_GENERIC_RUN/PLAN.md`)
- `drops/DROP_13_GENERIC_RUN/BUILDER_WORKLOG.md` — Round 1 entry appended
Pass.

### Cross-reference: PLAN.md Acceptance bullets

All eight bullets at `drops/DROP_13_GENERIC_RUN/PLAN.md:80-86` map onto the checks above:

- bullet 1 (shared service with dep set) → check 1, 2
- bullet 2 (no-override preserves entrypoint) → check 4
- bullet 3 (override emits `--entrypoint` via Extra) → check 3
- bullet 4 (within-project guard, labels, Env, EnvPassthrough, Mounts, attached, warnings, cleanup) → checks 5, 6, 7, 8, 9, plus label coverage in `TestRunPropagatesProviderLabels` (`service_test.go:466-512`)
- bullet 5 (provider wrappers own provider-specific prep) → check 2 (PreparedRuntime contract leaves Codex shared-home derivation to provider side; service consumes only the five-field contract)
- bullet 6 (tests prove Extra, EnvPassthrough flow, warning propagation, mount exact-equality including worktree gitdir, cleanup on success+failure, no Docker type changes) → checks 3, 5, 6, 7, 9, 11

### Findings

No findings. All twelve verification checks plus all eight PLAN.md acceptance bullets are satisfied. Mage independently confirms 42/42 pass at 82.4% coverage with `-race` enabled.
