# DROP_13 — Builder Worklog

Append a `## Unit 13.M — Round K` section per build attempt. See `main/drops/WORKFLOW.md` § "Phase 4 — Build (per unit)" for what each section should contain.

## Unit 13.1 — Round 1

### Files touched

- `internal/services/run/service.go` (new) — shared provider-agnostic launch primitive.
- `internal/services/run/service_test.go` (new) — table-driven tests parameterized over claude+codex provider descriptors.
- `drops/DROP_13_GENERIC_RUN/PLAN.md` — unit state flip `todo` → `in_progress` → `done`.
- `drops/DROP_13_GENERIC_RUN/BUILDER_WORKLOG.md` — this entry.

### Mage targets run

- `mage testPkg ./internal/services/run` — 42 tests pass, 82.4% coverage (well above the 70% per-package floor from `main/CLAUDE.md`; mage's enforced minimum is 60%).

### Design notes on the shared seam shape

The shared seam in `internal/services/run` exposes four types:

- `Provider` — descriptor carrying only what the service needs to label and notice: `Name` (the `io.valv.provider` label value), `ContainerNamePrefix` (e.g. `valv-claude-interactive`), `NoticePrefix` (e.g. `Valv note` / `Valv MCP note`). No runtime-prep concerns leak into this descriptor per the Schema Decisions in `PLAN.md` lines 38-39 and the Notes-for-Builders bullet on `internal/adapters/providers/{claude,codex}/runtime.go:41-59`/`39-52`.
- `PreparedRuntime` — minimum contract `Env`, `EnvPassthrough`, `Mounts`, `Warnings`, `Cleanup` (exported `func() error` field). Deliberately omits `ContainerHome` per Round 5 falsification finding (service layer never reads it). Provider wrappers will hand a `*run.PreparedRuntime` derived from their own `clauderuntime.PreparedRuntime` / `codexruntime.PreparedRuntime` values.
- `Options` + `Service` + `New(Options) (Service, error)` — constructor validates non-nil executor, non-empty image repository, non-empty provider name and container-name prefix. Mirrors the validation shape in the existing claude/codex `New` functions.
- `LaunchRequest` + `Run(ctx, LaunchRequest) error` — per-call payload (`ProjectRoot`, `WorkingDir`, `ProjectID`, `ProfileID`, `ProjectName`, `Prepared`, `Args`, `Command`). `Command` is the optional explicit-override slice; non-empty triggers the `--entrypoint <command[0]>` injection via `ContainerRunRequest.Extra` per Schema Decision in `PLAN.md` line 45.

Key design properties:

1. **Mount equality is byte-for-byte.** `request.Mounts == []MountSpec{projectRoot} ++ prepared.Mounts`. Tested via `reflect.DeepEqual` in `TestRunMountsExactEqualityProjectRootThenPrepared` with three prepared mounts (`provider home`, `cross-mount`, `worktree gitdir`-shaped) so the assertion catches any future drift that inserts a default mount.
2. **Entrypoint injection uses `Extra`.** `applyCommandOverride` emits `["--entrypoint", command[0]]` into `Extra`, which the existing `BuildRunArgs` already places before the image token (`internal/adapters/docker/types.go:215-217`). No Docker adapter surface widening.
3. **Cleanup is defer-driven.** A single `defer request.Prepared.Close()` runs on success, executor error, and `buildRequest` error. The cleanup-on-failure test (`TestRunPreparedCloseRunsOnExecutorFailure`) asserts the `atomic.Int32` counter is `1` after a wrapped error bubbles.
4. **Notice routing is provider-parameterized.** `emitNotices` uses `s.provider.NoticePrefix` for the visible-text prefix and routes through the existing TTY-suppression rule (`s.notices == nil || s.tty` short-circuits to debug logs only). Parameterized over both providers and asserted in `TestRunPropagatesWarningsToNotices` + `TestRunSuppressesNoticesOnTTY`.
5. **Within-project guard preserved.** `withinProjectRoot` is a verbatim port of the helper in `internal/services/claude/service.go:332-344` and `internal/services/codex/service.go:335-347`, including the `..` and sibling-prefix rejection. Tested for both providers.

### Test fixture rationale

- **Provider parameterization.** `providerDescriptors()` returns claude + codex descriptors. Almost every test loops over both — the only test that doesn't is `TestRunValidatesLaunchRequest`, which is descriptor-agnostic. This proves the seam is genuinely provider-agnostic and not silently claude-shaped.
- **`preparedRuntimeFixture` with atomic counter.** The `Closed()` accessor returns a snapshot of `atomic.Int32` (the field is `int32` accessed via `atomic.AddInt32` / `atomic.LoadInt32`, race-safe under `mage testPkg`'s `-race` flag). Lets the success-path and failure-path tests share the same fixture shape.
- **`reflect.DeepEqual` for mounts and args.** The plan's "exact equality" / "byte-for-byte" language rules out `Contains`-style assertions. The mount test deliberately includes a worktree-gitdir-shaped third mount so the test fails if anyone re-sorts, dedups, or inserts default mounts.
- **`recordingNotices` writer.** A small `strings.Builder`-backed `io.Writer` implementation captures notice output so warning-vs-TTY behavior is asserted directly.

### Hard-constraint adherence

- Only touched `internal/services/run/` plus this drop's `PLAN.md` (state flip only) and `BUILDER_WORKLOG.md`.
- Did **not** modify `internal/services/claude/`, `internal/services/codex/`, `internal/cli/`, `magefile.go`, `internal/adapters/docker/`, or `internal/domain/`. The shared service consumes `docker.ContainerRunRequest` / `docker.MountSpec` / `docker.ImageRef` exactly as they exist today.
- Reused `ContainerRunRequest.Extra` for the entrypoint override — no widening of the Docker adapter surface.

### Hylla Feedback

No Hylla MCP query was made: the unit's scope was completely self-contained in evidence already cited by the planner (`internal/services/claude/service.go`, `internal/services/codex/service.go`, both provider runtime files, and `internal/adapters/docker/types.go`). Reading those files directly via the `Read` tool was the lowest-latency path, and the plan's evidence cites covered every file I needed. No Hylla miss to record — this is the "non-Hylla path taken intentionally" case rather than the "Hylla missed something" case.
