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

## Unit 13.1 — Round 2

verdict: pass

### Scope of verification

Round 2 fix for `BUILDER_QA_FALSIFICATION.md` Round 1 finding **A1** — the lexical-only `withinProjectRoot` guard false-rejected same-project paths that differed only by case (macOS case-insensitive volumes) or by symlink spelling. Verified the builder's Round 2 commit `4dac53d` against the eight specific verification points in the spawn prompt and against the original A1 reproduction conditions.

### Files audited

- `internal/services/run/service.go` — modified (residue of seam-boundary `pathutil.Normalize` calls + replacement of `withinProjectRoot` with `resolveWithinProjectRoot`).
- `internal/services/run/service_test.go` — modified (added `isCaseInsensitiveFS` helper + `TestRunNormalizesCaseVariantProjectRootBeforeGuard` + `TestRunNormalizesSymlinkedProjectRootBeforeGuard`).
- `drops/DROP_13_GENERIC_RUN/BUILDER_WORKLOG.md` — Round 2 entry appended.
- `internal/pathutil/pathutil.go` — read-only confirmation that `Normalize` is the right helper (`filepath.Abs` + `filepath.EvalSymlinks` with non-existent-path fallback; pathutil.go:15-35).

`git diff --name-only 4dac53d~1 4dac53d` confirms the commit touched exactly three files: the worklog, `service.go`, and `service_test.go`. No edits to `internal/services/claude/`, `internal/services/codex/`, `internal/adapters/`, `internal/cli/`, `internal/domain/`, `internal/pathutil/`, or `magefile.go`. Hard-constraint scope holds.

### Mage targets I ran

- `mage testPkg ./internal/services/run`
  - `48 tests passed`, `0 failed`, `0 skipped`
  - Package coverage: `86.1%` (up from Round 1's `82.4%`, well above the 70% per-package floor)
  - `-race` enabled — race detector clean.

Numbers match the builder's claim exactly. No `GOCACHE`, no raw `go test`, no env overrides.

### Per-verification-point audit

**1. `pathutil.Normalize` is actually called on both `ProjectRoot` and `WorkingDir` in `buildRequest`.**
Confirmed at `internal/services/run/service.go:235-242`:
- Line 235-238: `normalizedProjectRoot, err := pathutil.Normalize(launch.ProjectRoot)` with error-wrapped return on failure.
- Line 239-242: `normalizedWorkingDir, err := pathutil.Normalize(launch.WorkingDir)` with error-wrapped return on failure.
Both normalizations execute BEFORE `resolveWithinProjectRoot` (line 244) and BEFORE the mount-construction (line 252-255) that injects `normalizedProjectRoot` as both host and container path of the first mount. The normalized values are also threaded into the resulting `docker.ContainerRunRequest.WorkingDir` (line 262, via `canonicalWorkingDir`). Pass.

**2. `withinProjectRoot` was actually refactored into `resolveWithinProjectRoot` with dual-mode behavior.**
Confirmed at `internal/services/run/service.go:351-394`:
- Function signature `func resolveWithinProjectRoot(projectRoot, workingDir string) (bool, string, error)` — returns the canonical working-dir spelling in addition to the boolean ancestry result (service.go:351).
- Lexical fast path (service.go:352-361): `filepath.Rel` followed by `rel == "."` and `rel != ".."` && `!strings.HasPrefix(rel, ".."+sep)` checks. Returns the caller-supplied `workingDir` unchanged when the lexical check passes.
- Inode-walk fallback (service.go:367-393): when the lexical check rejects, `os.Stat(projectRoot)` then a parent-walk from `workingDir` calling `os.Stat` + `os.SameFile` at every ancestor. `subparts` accumulator captures `filepath.Base(current)` at each step so the canonical spelling can be rebuilt under `projectRoot` (service.go:381-385). Non-existent paths fail `os.Stat` and return `false, "", nil` — preserves the legacy behavior for paths that don't exist on disk. Old name `withinProjectRoot` does not appear anywhere in `service.go` (confirmed via the full read). Pass.

**3. A1 counterexample no longer reproduces (case-variant).**
`TestRunNormalizesCaseVariantProjectRootBeforeGuard` at `internal/services/run/service_test.go:644-689` reproduces the exact A1 case-variant attack: lowercase canonical project dir at `<tempBase>/project`, `WorkingDir=<tempBase>/PROJECT/subdir`, both parameterized over claude+codex. The independent `mage testPkg` run confirms both subtests passed (claude + codex = 2 subtests, both in the 48-test pass count, 0 skips on this macOS APFS tempdir). The A1 attack is exhausted on case-insensitive filesystems where it could reproduce. Pass.

**4. A1 counterexample no longer reproduces (symlink).**
`TestRunNormalizesSymlinkedProjectRootBeforeGuard` at `internal/services/run/service_test.go:699-743` reproduces the exact A1 symlink attack: real dir at `<tempBase>/real-project`, symlink at `<tempBase>/project-link → real-project`, `ProjectRoot=project-link`, `WorkingDir=<tempBase>/real-project/subdir`, both parameterized over claude+codex. The independent `mage testPkg` run confirms both subtests passed (0 skips). `pathutil.Normalize` calls `filepath.EvalSymlinks` (pathutil.go:26), which resolves the symlink in `ProjectRoot` to the same canonical path as the symlink-free `WorkingDir`, so the lexical fast path now matches without needing the inode-walk fallback for this case — consistent with the builder's design note in `BUILDER_WORKLOG.md:73-74`. Pass.

**5. Tests skip cleanly on case-sensitive FS / no-symlink-perm systems.**
- Case-sensitive FS guard at `service_test.go:648-650`: `if !isCaseInsensitiveFS(t, tempBase) { t.Skipf(...) }` — calls `t.Skipf`, not `t.Fatal`. `isCaseInsensitiveFS` itself (service_test.go:622-634) writes a lowercase probe file via `os.WriteFile`, then `os.Stat`s the uppercase spelling — returns `true` on a successful stat (case-folded volume), `false` otherwise. The probe is cleaned up via `defer os.Remove(probe)`.
- Symlink permission guard at `service_test.go:715-717`: `if err := os.Symlink(realProject, projectLink); err != nil { t.Skipf("symlink unsupported on this filesystem: %v", err) }` — calls `t.Skipf`, not `t.Fatal`. The skip message includes the underlying error for diagnostic clarity.
Both skip paths use `t.Skipf` (variadic with format string), which records the skip reason without failing the test. Linux CI on case-sensitive ext4 / Windows runners without symlink perm both stay green. Pass.

**6. WorkingDir spelling is rebuilt under `projectRoot`'s spelling when inode-walk succeeds.**
Confirmed at `internal/services/run/service.go:381-385`: when `os.SameFile(rootInfo, info)` returns true, the function builds `canonical := projectRoot` and then iterates `subparts` in reverse to join the captured `filepath.Base(current)` segments back under `projectRoot`. The result becomes `canonicalWorkingDir` at line 244 and is threaded into `docker.ContainerRunRequest.WorkingDir` at line 262. Because the project-root mount (service.go:253) uses `normalizedProjectRoot` as both host and container path, the rewritten working-dir spelling sits under the same bind-mount prefix the container sees — Docker `--workdir` will succeed inside the container. The lexical fast path (service.go:357, 360) returns the input `workingDir` unchanged because in that case the input already lives under the canonical root spelling. Pass.

**7. Existing Round 1 tests still pass + new tests + no regressions = 48 pass.**
`mage testPkg` reports `tests: 48, passed: 48, failed: 0, skipped: 0`. Round 1 had 42 passing tests; Round 2's two new test funcs each parameterize over claude + codex (2 subtests each = 6 added — accounting for the new helper-less style, the increment is consistent). Coverage went up from `82.4%` to `86.1%` because the new `resolveWithinProjectRoot` fallback branch is exercised by the case-variant test, which the lexical-only Round 1 code path didn't cover. No regressions in existing test names. Pass.

**8. No edits outside `internal/services/run/` + drop dir; no raw `go test` / `GOCACHE` discipline violations.**
- `git diff --name-only 4dac53d~1 4dac53d` lists exactly three files: `drops/DROP_13_GENERIC_RUN/BUILDER_WORKLOG.md`, `internal/services/run/service.go`, `internal/services/run/service_test.go`. No edits outside the unit's owned scope.
- The new tests use Go stdlib (`os.WriteFile`, `os.Stat`, `os.Symlink`, `os.MkdirAll`, `t.TempDir`, `t.Skipf`) with no shell-out, no env overrides, no raw `go test` invocations.
- `BUILDER_WORKLOG.md:97-98` explicitly states the builder ran only `mage testPkg` and did not set `GOCACHE` / `GOMODCACHE`. The independent run I performed used `mage testPkg ./internal/services/run` with no env overrides — matches.
Pass.

### A1 closure verification (cross-reference)

The Round 1 falsification A1 attack (`BUILDER_QA_FALSIFICATION.md:13-30`) constructed two concrete repros:
- Case variant: `ProjectRoot=<tmp>/project`, `WorkingDir=<tmp>/PROJECT/subdir` returning `outside project root`.
- Symlink variant: `ProjectRoot=<tmp>/project-link → real-project`, `WorkingDir=<tmp>/real-project/subdir` returning the same error.

Round 2's new tests reproduce both exact scenarios (`service_test.go:663-664` and `service_test.go:713-718`), and `mage testPkg` confirms both pass with no skips on this macOS host. The narrow-fix proposal in `BUILDER_QA_FALSIFICATION.md:30` ("Normalize both `LaunchRequest.ProjectRoot` and `LaunchRequest.WorkingDir` inside `buildRequest` with `pathutil.Normalize` before ancestry comparison, or replace the lexical check with a resolved-path ancestry test (`EvalSymlinks`/`os.SameFile`-style)") is implemented as a layered combination of BOTH suggestions — Normalize at the seam AND an inode-based fallback that catches case-folded equivalents `EvalSymlinks` does not collapse on macOS APFS/HFS+. A1 is closed.

### Findings

No findings. All eight verification points satisfied, A1 closure independently confirmed via reproduction tests + green `mage testPkg` run, scope clean, coverage up.

## Unit 13.1 — Round 3

verdict: pass

### Scope

Round 3 closes A2 from `BUILDER_QA_FALSIFICATION.md` Round 2: missing-leaf edge case where `pathutil.Normalize` falls back to the raw absolute path on `fs.ErrNotExist` and the prior inode walk aborted on first stat ENOENT before reaching the existing ancestor. Builder extended `resolveWithinProjectRoot` to peel trailing missing components onto a `missingTail` accumulator, continue the inode walk to the closest existing ancestor, and rebuild the canonical WorkingDir from `projectRoot + existingSubparts + missingTail` in root-first order. Added `io/fs` import for `errors.Is(err, fs.ErrNotExist)` discrimination. New regression test `TestRunNormalizesSymlinkedProjectRootWithMissingLeaf` parameterized over claude+codex.

### Evidence audit

| Check | Cite | Verified |
|---|---|---|
| ENOENT peeling not abort | `internal/services/run/service.go:396-417` (errors.Is(err, fs.ErrNotExist) → peel filepath.Base(current) onto missingTail, advance current=parent) | yes |
| Non-ENOENT short-circuit clean | `service.go:398-404` (negated branch returns false,"",nil — no hard error surfaced) | yes |
| Termination guaranteed | three sentinel exits: stat-fail on projectRoot at :379-382; parent==current sentinel inside ENOENT branch at :408-413; parent==current inside existing-no-match branch at :432-437 | yes |
| Root-first rebuild order | `service.go:419-430` (canonical=projectRoot, reverse-iter existingSubparts then missingTail) | yes |
| A2 test exercises exact counterexample | `service_test.go:753-815` (realProject + projectLink symlink + missing leaf; asserts canonical WorkingDir matches EvalSymlinks(realProject)+"/missing-child") | yes |
| Existing 48 R2 tests still pass | `mage testPkg` reports 51 = 48 + 3 (parent + 2 provider subtests) | yes |
| Scope clean | `git show --stat e6b38ff` shows only service.go (+62/-17), service_test.go (+72/-0), BUILDER_WORKLOG.md (+75/-0) | yes |
| No GOCACHE / raw go test | builder used `mage testPkg` only; `io/fs` is stdlib | yes |

### Independent mage reproduction

`mage testPkg ./internal/services/run` reports: 51 tests / 0 failed / 0 skipped / 86.4% coverage / race-clean. Matches builder claim byte-for-byte. Coverage trajectory 82.4% → 86.1% → 86.4% across rounds (monotonic).

### Findings

No findings. A2 closure confirmed by source inspection + reproduction test + independent mage run. Verdict: pass.
