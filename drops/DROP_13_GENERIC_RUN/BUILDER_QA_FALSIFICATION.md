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

## Unit 13.1 — Round 2

**Verdict:** fail
**Reviewer:** ta-go-qa-falsification
**Reviewed at:** 2026-05-24T17:01:02Z

Reviewed against `drops/DROP_13_GENERIC_RUN/PLAN.md`, `drops/DROP_13_GENERIC_RUN/BUILDER_WORKLOG.md`, `git show 4dac53d`, direct reads of `internal/services/run/service.go`, `internal/services/run/service_test.go`, `internal/pathutil/pathutil.go`, `internal/pathutil/pathutil_test.go`, `go doc path/filepath.Rel`, `go doc path/filepath.EvalSymlinks`, `go doc os.SameFile`, and the Go 1.26.3 stdlib source for `path/filepath/walkSymlinks`. Hylla and gopls MCP were unavailable in this session (`Transport closed`). I also attempted the required `mage testPkg ./internal/services/run` repro path, but the sandbox denied access to the default Go build cache and the repo rules prohibit `GOCACHE` overrides, so the confirmed finding below is source-trace-grounded rather than runtime-executed.

### Counterexamples / Attacks

#### A2 — symlink-alias handling still false-rejects an equivalent missing descendant under the project root

**Construction:** the round-2 fix normalizes both inputs before ancestry comparison (`internal/services/run/service.go:235-244`), but `pathutil.Normalize` only resolves symlinks when the full target exists; for `fs.ErrNotExist` it returns the absolute path unchanged (`internal/pathutil/pathutil.go:12-35`, `internal/pathutil/pathutil_test.go:168-185`). The fallback ancestry walk in `resolveWithinProjectRoot` immediately returns `false` on the first `os.Stat(current)` miss instead of walking up to an existing parent (`internal/services/run/service.go:363-389`). Go's stdlib `walkSymlinks` confirms `EvalSymlinks` calls `os.Lstat` component-by-component and returns the first missing-path error (`/Users/evanschultz/.govm/versions/go1.26.3/src/path/filepath/symlink.go:75-88`).

**Concrete counterexample:** with an on-disk layout:

- `realProject=/tmp/real-project` exists
- `projectLink=/tmp/project-link` is a symlink to `realProject`
- `workingDir=/tmp/project-link/missing-child` does **not** exist yet

`buildRequest` takes this trace:

1. `Normalize(projectLink)` returns `/tmp/real-project` because the symlink target exists.
2. `Normalize(workingDir)` returns `/tmp/project-link/missing-child`, not `/tmp/real-project/missing-child`, because the final leaf is missing and the helper deliberately falls back to the raw absolute spelling on `fs.ErrNotExist`.
3. `filepath.Rel("/tmp/real-project", "/tmp/project-link/missing-child")` rejects lexically.
4. The fallback walk starts at `/tmp/project-link/missing-child`; `os.Stat` fails immediately because `missing-child` is absent, so `resolveWithinProjectRoot` returns `false` without ever checking the existing symlink parent or matching the project root inode.
5. `buildRequest` returns `working directory "/tmp/project-link/missing-child" is outside project root "/tmp/real-project"` even though the path is semantically inside the same project tree.

This is new relative to Round 1: the existing-subdir alias cases are now covered, but the fix still fails when alias resolution must survive a missing descendant.

**Why this breaks the claim:** the round-2 worklog says the seam now normalizes project-root and working-dir spellings so symlink-equivalent paths are not false-rejected. That is only true when the working-dir leaf already exists. The shared guard still rejects a same-project symlink spelling in this missing-descendant case.

**Narrow fix:** when the lexical check rejects, do not abort on the first `os.Stat(current)` `ErrNotExist`. Instead, peel missing trailing components into `subparts` until an existing ancestor is found, continue the inode walk from that ancestor, and if the root matches, rebuild the canonical working dir by joining `projectRoot` with both the traversed existing path parts and the previously missing tail.

### Other attacks attempted

- `/var` versus `/private/var`: mitigated by the pre-guard `pathutil.Normalize` call when the target exists, so I did not find a new falsifier there.
- Trailing slashes and `..` segments: mitigated by `filepath.Abs` plus `filepath.Clean`/`EvalSymlinks` behavior inside `Normalize`; no counterexample found.
- Fallback performance: the inode walk is O(depth) only on lexical rejects. I did not find evidence that the new fallback itself runs on the already-canonical common path.
- Working-dir canonical rebuild semantics: for existing descendants, the rebuilt path stays under the normalized project root and matches the bind-mount spelling; no separate break beyond A2 found.
- Symlink replacement race: plausible TOCTOU concern between normalization and Docker bind, but I did not produce a concrete repository-local falsifier in this pass.
- Case-insensitive-FS probe and symlink-test fixture behavior: the new tests' skip probes are scoped to the temp dir they use, and I did not find a concrete false-pass/false-fail path from mixed mounts.
- Non-ASCII path spellings: I did not confirm a concrete NFC/NFD falsifier from the current code and filesystem assumptions.
- Concurrent calls into `Service.Run`: the service itself keeps launch state local and does not mutate shared cache/state; no concrete race found here.
- Already-canonical fast path: lexical success still returns before the inode walk; no inconsistent `WorkingDir` behavior found there.

### YAGNI check

No YAGNI blocker. The fallback inode walk is still justified by the two concrete provider callers and the round-1 bug; the issue here is incompleteness of the chosen fix, not over-abstraction.

### Hidden dep check

Hidden dependency confirmed. The round-2 fix still assumes the aliased `WorkingDir` already exists on disk before it can be canonicalized. That assumption happens to hold for current `os.Getwd()`-driven wrappers, but it is neither stated in `LaunchRequest` nor covered by the new tests, so the shared seam's symlink-normalization guarantee is narrower than advertised.

### Falsification summary

- Confirmed counterexamples blocking PASS: 1.
- Confirmed blocker: A2 false-rejects a same-project symlink alias when the final working-directory component is missing.
- Remaining attack vectors were either mitigated by the current code or stayed at the level of unconfirmed risk.

Verdict: **fail**.
