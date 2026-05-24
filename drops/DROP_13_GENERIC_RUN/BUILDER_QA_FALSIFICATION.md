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

## Unit 13.1 — Round 3

**Verdict:** pass
**Reviewer:** ta-go-qa-falsification
**Reviewed at:** 2026-05-24T00:00:00Z

Reviewed against `drops/DROP_13_GENERIC_RUN/PLAN.md`, `drops/DROP_13_GENERIC_RUN/BUILDER_WORKLOG.md` (Rounds 1+2+3), `drops/DROP_13_GENERIC_RUN/BUILDER_QA_PROOF.md`, direct reads of `internal/services/run/service.go` (Round 3, 51-test build, 86.4% coverage per worklog), `internal/services/run/service_test.go` (the new `TestRunNormalizesSymlinkedProjectRootWithMissingLeaf` at lines 745-815), `internal/pathutil/pathutil.go`, plus `go doc os.IsNotExist` and `go doc io/fs.ErrNotExist` for ENOTDIR/ENOENT class-mapping behavior. Hylla MCP was not consulted for this round — the targeted attack surface (`resolveWithinProjectRoot` in a single file) was small enough that direct `Read` + line-by-line trace was the lowest-latency evidence path. Per repo rule I did NOT re-run `mage testPkg` (sandbox prohibits raw `go test` / `GOCACHE` overrides and `mage testPkg` was already executed by the builder per BUILDER_WORKLOG.md Round 3 § "Mage targets run").

### Counterexamples / Attacks

No new counterexamples confirmed. All eight dispatch-listed attack vectors were traced against the Round 3 `resolveWithinProjectRoot` (`internal/services/run/service.go:363-439`). Each either resolves correctly, hits a Normalize-error short-circuit safely, or maps to an accepted/pre-existing limitation.

#### Attack 1 — Deeply nested missing tail

`projectRoot=/realProject`, `workingDir=/projectLink/a/b/c/d/missing/grand/great`, where `projectLink -> realProject`, `a/b` exist under realProject, `c` onwards do not. Traced eight iterations of the walk: `great`/`grand`/`missing`/`d`/`c` peel onto `missingTail` (5 ENOENT peels); Stat succeeds on `/projectLink/a/b` and `/projectLink/a` (SameFile mismatch — append `b`, then `a` to `existingSubparts`); Stat on `/projectLink` resolves through the symlink, `os.SameFile` matches `realProject`. Rebuild loops both accumulators in root-first order: `realProject + a + b + c + d + missing + grand + great`. Correct.

#### Attack 2 — All-missing WorkingDir under existing project link

`projectRoot=/realProject`, `workingDir=/projectLink/totally-missing`. iter1 peels `totally-missing`, current=`/projectLink`. iter2 Stat succeeds (symlink follows to realProject), SameFile matches. Rebuild: `realProject + totally-missing`. Correct — this is the canonical R3 case, exactly mirrored by the new `TestRunNormalizesSymlinkedProjectRootWithMissingLeaf` test (`service_test.go:753-815`).

#### Attack 3 — Symlink in middle of path (target inside project)

`projectRoot=/realProject`, `workingDir=/realProject/sub-link/missing-child`, `sub-link -> /realProject/actual`. `pathutil.Normalize` of workingDir hits `fs.ErrNotExist` (leaf missing) and falls back to the raw absolute path. Lexical `filepath.Rel("/realProject", "/realProject/sub-link/missing-child")` returns `sub-link/missing-child` (no `..` prefix) → fast path at `service.go:368-373` returns `(true, workingDir, nil)` unchanged. The in-container `--workdir` becomes `/realProject/sub-link/missing-child`; `sub-link` is a directory entry inside the bind-mounted project root and Docker handles the symlink follow inside the container. No false reject, no canonical-rebuild needed.

#### Attack 4 — Cyclic symlink at project root

`projectRoot=/cyclic`, `/cyclic -> /cyclic/sub -> /cyclic`. `pathutil.Normalize(projectRoot)` calls `filepath.EvalSymlinks`, which detects the symlink cycle and returns `too many links` (not wrapped as `fs.ErrNotExist`). `pathutil.Normalize` then takes the third branch (`pathutil.go:34`), wrapping the error as `normalize path %q: resolve symlinks: %w`. `buildRequest` returns at `service.go:236-238` with `normalize project root %q: %w`. The error bubbles out cleanly — there is no path into `resolveWithinProjectRoot`, so no infinite loop is possible. Mitigated by Normalize's explicit `fs.ErrNotExist`-only fallback.

#### Attack 5 — Permission denied mid-walk

Parent directory chmod 000 in the ancestry. `os.Stat` returns EACCES, which does NOT satisfy `errors.Is(err, fs.ErrNotExist)` (per `go doc os.IsNotExist` — EACCES is "permission denied", distinct from ENOENT/ENOTDIR). The `service.go:397-405` branch short-circuits to `return false, "", nil`, `withinRoot` is false, `buildRequest` returns `working directory %q is outside project root %q`. No panic, no infinite loop. The user-facing error is misleading (says "outside project root" when the actual cause was permission denied), but this is the **intentional, documented** R3 design choice (`service.go:396-404`: "fall back to the lexical rejection without surfacing a hard error so the guard behaviour mirrors the previous implementation"). Worth noting as a UX-clarity limitation, not as a behavioral falsifier of the R3 missing-leaf fix. Accepted.

#### Attack 6 — TOCTOU race between Normalize and inode walk

Symlink swap or directory removal between the `pathutil.Normalize` call and the `os.Stat` in the inode walker. Concern is real but is **not new in Round 3** — Round 2 already introduced both the Normalize call and the inode walk, with the same gap. The R3 worklog does not claim atomicity, and the existing Claude/Codex services have identical TOCTOU exposure (`internal/services/claude/service.go` lines 332-344, called out as a "verbatim port" in BUILDER_WORKLOG R1). No new R3 counterexample.

#### Attack 7 — Trailing slash semantics

`workingDir=/projectLink/missing/`. `pathutil.Normalize` calls `filepath.Abs`, which internally Cleans the path and strips trailing separators (per `go doc path/filepath.Clean`). So Normalize produces `/projectLink/missing` (or the EvalSymlinks-resolved form). The walker sees a normalized leaf-form path and proceeds identically to Attack 2. No false reject.

#### Attack 8 — Tilde in path

`~/projects/foo/missing`. `filepath.Abs` does **not** expand `~` — tilde expansion is a shell construct, not a filesystem one. `Abs` would return `<cwd>/~/projects/foo/missing`, a nonsense literal path. The walker would correctly false-reject because no ancestor SameFile-matches the project root. This is a **caller precondition violation**, not a falsifier of the R3 missing-leaf fix — the same false-reject would occur in Round 1 and Round 2, and is consistent with the upstream Claude/Codex services that also never tilde-expand. Not new in Round 3.

### Additional probes beyond the dispatch list

- **ENOTDIR mapping.** `errors.Is(err, fs.ErrNotExist)` per Go stdlib syscall-error mapping returns true for both ENOENT and ENOTDIR (`go doc os.IsNotExist`: "satisfied by ErrNotExist as well as some syscall errors"). A path like `/projectLink/somefile.txt/missing` (treating a regular file as a directory) hits ENOTDIR; the walker peels `missing` and walks to `somefile.txt`. Stat there succeeds (it's a file), SameFile mismatches, walk continues to `/projectLink` and matches. Rebuild produces `realProject/somefile.txt/missing` — a nonsensical canonical path that Docker `--workdir` would reject at runtime, but this is garbage-in / garbage-out, not a falsifier of the R3 fix.
- **NFC/NFD Unicode (macOS).** Filesystem stores names in NFD, user can spell in NFC. `EvalSymlinks` returns on-disk (NFD) form; `os.SameFile` compares inodes (Unicode-spelling-independent). The R3 walker survives NFC/NFD spelling drift because SameFile is the ancestry-match operation.
- **Rebuild `..` injection via missingTail.** `filepath.Base` on each iteration returns only the final component (`go doc path/filepath.Base`), so `missingTail` cannot contain `..` or path separators. The subsequent `filepath.Join` in the rebuild therefore cannot collapse the canonical path back outside `projectRoot`. Safe.
- **`/` filesystem root reached without match.** `workingDir=/foo/missing` with `/foo` also missing and `projectRoot=/realProject`. Walk peels `missing`, peels `foo`, lands on `/`. Stat `/` succeeds, SameFile mismatches `realProject`. `parent := filepath.Dir("/")` returns `/`; `parent == current` triggers the `return false, "", nil` sentinel at `service.go:432-434`. No infinite loop.
- **Symlink-escape via lexical fast path.** `projectRoot=/projectA`, `workingDir=/projectA/escape-link/missing` where `escape-link -> /projectB`. Lexical Rel returns `escape-link/missing` (no `..` prefix), fast path passes, canonical = workingDir unchanged. Docker bind-mounts only `/projectA`; inside the container, `escape-link` is a dangling symlink (target `/projectB` is not mounted). This is a **pre-existing concern in Round 1's "verbatim port"** of the Claude/Codex `withinProjectRoot` helper — BUILDER_WORKLOG R1 § "Within-project guard preserved" explicitly notes the verbatim port. Not a Round 3 regression and not in the dispatch attack list; noted here for context, not as a counterexample.

### YAGNI check

No YAGNI blocker. The missing-tail peeling machinery is justified by the R2 falsification finding (the A2 counterexample is concrete and reproducible). The two accumulators (`missingTail`, `existingSubparts`) are minimal — no additional abstractions or generic-walker layers introduced. The walk terminates in at most O(path-depth) iterations and is gated behind a lexical fast path so the common already-canonical case never enters it.

### Hidden dep check

No new hidden dependencies introduced in Round 3 relative to Round 2. The fix depends on:

- `os.Stat` following symlinks transparently (stdlib contract, verified).
- `errors.Is(err, fs.ErrNotExist)` matching both ENOENT and ENOTDIR (stdlib contract, verified via `go doc`).
- `filepath.Base` never returning `..` or path separators (stdlib contract, verified).
- `os.SameFile` comparing inodes only (stdlib contract, verified).

All four are documented stdlib semantics — same dependency surface as the upstream Claude/Codex services. The R2 hidden-dep concern (caller pre-normalizes paths) is now resolved by the missing-tail peeling; the R3 walker no longer assumes the working-dir leaf already exists on disk.

### Falsification summary

- Confirmed counterexamples blocking PASS: 0.
- All eight dispatch-listed NEW attack vectors traced; each either resolves correctly, hits a safe Normalize-error short-circuit, or is an accepted documented limitation / pre-existing concern.
- Beyond the dispatch list: ENOTDIR, NFC/NFD, rebuild `..` injection, filesystem-root sentinel, and symlink-escape fast path also examined — no new R3 falsifier.

Verdict: **pass**.
