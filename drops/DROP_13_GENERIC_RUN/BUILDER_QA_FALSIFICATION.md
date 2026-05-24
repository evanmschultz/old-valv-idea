# DROP_13 Build QA Falsification

## Unit 13.1 — Round 1

**Verdict:** fail
**Reviewer:** ta-go-qa-falsification
**Reviewed at:** 2026-05-24T16:22:32Z

Reviewed against `drops/DROP_13_GENERIC_RUN/PLAN.md`, `drops/DROP_13_GENERIC_RUN/BUILDER_WORKLOG.md`, `git show c810331`, direct reads of `internal/services/run/*`, `internal/adapters/docker/types.go`, `internal/services/{claude,codex}/service.go`, `internal/adapters/providers/{claude,codex}/runtime.go`, and LSP reference checks. The dispatch requested Hylla MCP grounding, but no `mcp__hylla__hylla_*` tools were available in this session, so committed grounding came from `git show` plus direct source reads.

### Counterexamples / Attacks

#### A1 — within-project-root guard falsely rejects same-project paths when the spelling differs by case or symlink

**Construction:** `Service.Run` defers cleanup, emits notices, then calls `buildRequest` (`internal/services/run/service.go:173-189`). `buildRequest` gates execution on `withinProjectRoot(launch.ProjectRoot, launch.WorkingDir)` (`internal/services/run/service.go:226-233`). `withinProjectRoot` is a purely lexical `filepath.Rel` check (`internal/services/run/service.go:322-333`) with no path normalization or symlink resolution.

**Reproduction:** I added a disposable package-local test file and ran:

`mkdir -p /private/tmp/valv-gocache && GOCACHE=/private/tmp/valv-gocache go test ./internal/services/run -run 'TestFalsificationWithinProjectRootRejects(CaseVariantOfSamePath|SymlinkedSameProject)$' -count=1 -v`

Both repros passed, confirming the false rejection:

- Case-variant path on this checkout's case-insensitive filesystem: `ProjectRoot=<tmp>/project`, `WorkingDir=<tmp>/PROJECT/subdir` returned `outside project root` even though both paths resolve to the same directory tree.
- Symlink variant: `ProjectRoot=<tmp>/project-link`, `WorkingDir=<tmp>/real-project/subdir` with `project-link -> real-project` returned the same `outside project root` error.

The scratch file was deleted after the run; the working tree returned to its pre-attack state.

**Why this breaks the claim:** Attack vector 9 explicitly asked whether the guard is correct for symlinks, `..`, and case-insensitive volume mounts. The new shared service preserves the old lexical helper, so `..` and sibling-prefix rejection work, but symlink-equivalent and case-equivalent paths are still rejected incorrectly. That is a concrete false negative in Unit 13.1 itself.

**Narrow fix:** Normalize both `LaunchRequest.ProjectRoot` and `LaunchRequest.WorkingDir` inside `buildRequest` with `pathutil.Normalize` before ancestry comparison, or replace the lexical check with a resolved-path ancestry test (`EvalSymlinks`/`os.SameFile`-style).

### Other attacks attempted

- Prepared runtime contract shape: no `ContainerHome` or shared-home field leaked into `run.PreparedRuntime`; only `Env`, `EnvPassthrough`, `Mounts`, `Warnings`, and cleanup plumbing are present (`internal/services/run/service.go:67-82`). I did not find a concrete wrapper-owns-shared-home break here.
- Mount exact-equality test: confirmed the test uses `reflect.DeepEqual`, not a subset check (`internal/services/run/service_test.go:322-363`).
- Cleanup on executor failure: confirmed coverage exists and the defer sits before `buildRequest`, so both executor-error and build-request-error paths run cleanup (`internal/services/run/service.go:178-188`, `internal/services/run/service_test.go:428-462`). No counterexample found there.
- Provider-agnostic parameterization: core request-shape tests do loop over both providers; I did not find a branch that only Claude or only Codex exercises where provider-specific behavior matters.
- `ContainerRunRequest.Extra` ordering: confirmed `BuildRunArgs` appends `request.Extra` before the image token (`internal/adapters/docker/types.go:215-217`), so `--entrypoint` injection is valid Docker CLI syntax.
- Empty `Command` plus empty `Args`: no bug here. The service leaves `Extra` nil and relies on the image's baked entrypoint, which is valid for this unit's shared-launch seam.
- Error wrapping: service-level build and executor failures use `%w` (`internal/services/run/service.go:188`, `208`). Notice write errors are ignored, but that matches the pre-existing Claude/Codex services (`internal/services/{claude,codex}/service.go:364-367`).
- Concurrent reuse of the same `PreparedRuntime`: I did not confirm a counterexample in the shared service alone. The service itself does not mutate `PreparedRuntime`, but provider-specific cleanup may still make reuse unsafe. That remains a caution, not a confirmed Unit 13.1 falsifier.

### YAGNI check

No blocker on YAGNI. The `Executor` seam is justified by two existing concrete callers (`internal/services/claude` and `internal/services/codex`), and the unit does not introduce a second unnecessary abstraction layer beyond the provider descriptor and launch request.

### Hidden dep check

Hidden dependency confirmed. The new API silently depends on callers pre-normalizing `ProjectRoot` and `WorkingDir` into the same resolved spelling before calling `Run`. That precondition exists in the legacy provider services, but Unit 13.1 does not encode it in the API contract or tests. The confirmed A1 counterexample is the consequence of that hidden dependency leaking into the shared seam.

### Falsification summary

- Confirmed counterexamples blocking PASS: 1.
- Confirmed blocker: A1 false-rejects same-project paths when the root/working-dir pair differs only by symlink spelling or path case.
- Remaining attacks were either mitigated by code/test evidence or did not yield a concrete break.

Verdict: **fail**.
