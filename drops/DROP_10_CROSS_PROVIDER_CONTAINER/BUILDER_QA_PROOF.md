# DROP_10 — Build QA Proof

Per-round entries appended below. Each `## Unit N.M — Round K` section is a
durable proof certificate for that build round.

## Unit 10.1 — Round 1

- **Reviewer:** go-qa-proof-agent
- **Commit under review:** `47ecbc4` — `feat(images): unit 10.1 dual-CLI Dockerfiles + cross-provider build-arg`
- **Verdict:** PASS

### Acceptance criteria evidence

**AC1 — `DefaultCodexDockerfile()` contains both CLI installs + both home dirs.**
- `internal/services/images/service.go:658` — `mkdir -p /home/valv/.codex /home/valv/.claude /workspace` (both home dirs in a single RUN block, both owned by the valv user via the same `chown -R` on line 659).
- `internal/services/images/service.go:668-669` — `ARG CODEX_VERSION` + `RUN npm install --global "@openai/codex@${CODEX_VERSION}"` (primary).
- `internal/services/images/service.go:671-672` — `ARG CLAUDE_VERSION` + `RUN npm install --global "@anthropic-ai/claude-code@${CLAUDE_VERSION}"` (cross-provider).
- Entrypoint preserved at line 676: `ENTRYPOINT ["codex"]`.

**AC2 — `DefaultClaudeDockerfile()` mirrors symmetrically + `CODEX_HOME` env added.**
- `internal/services/images/service.go:720` — `mkdir -p /home/valv/.claude /home/valv/.codex /workspace` (both home dirs).
- `internal/services/images/service.go:723-730` — `ENV` block includes `CLAUDE_CONFIG_DIR=/home/valv/.claude` (existing, line 729) AND new `CODEX_HOME=/home/valv/.codex` (line 730).
- `internal/services/images/service.go:732-733` — `ARG CLAUDE_VERSION` + `RUN npm install --global "@anthropic-ai/claude-code@${CLAUDE_VERSION}"` (primary).
- `internal/services/images/service.go:735-736` — `ARG CODEX_VERSION` + `RUN npm install --global "@openai/codex@${CODEX_VERSION}"` (cross-provider).
- Entrypoint preserved at line 740: `ENTRYPOINT ["claude"]`.

**AC3 — `Build()` emits both `CODEX_VERSION` and `CLAUDE_VERSION` regardless of provider.**
- `internal/services/images/service.go:318-321` — empty `request.CrossProviderVersion` defaults to `"latest"`.
- `internal/services/images/service.go:327-332` — `BuildArgs` map ALWAYS populated with FOUR keys:
  - `s.providerVersionBuildArg()` — primary (CODEX_VERSION when provider=codex, CLAUDE_VERSION when provider=claude).
  - `s.crossProviderVersionBuildArg()` — cross-provider (returns the OTHER arg name; line 573-578).
  - `VALV_GID` / `VALV_UID`.
- No `if req.Provider == ...` branching around arg emission — both args always emitted; only the value-vs-key mapping differs by provider.
- `internal/adapters/docker/ops.go:78-87` — `BuildImageArgs` sorts the BuildArgs map keys via `sort.Strings(keys)` (line 83), so the emitted `--build-arg` order is deterministically alphabetic: `CLAUDE_VERSION` < `CODEX_VERSION` < `VALV_GID` < `VALV_UID`.

**AC4 — `BuildRequest.CrossProviderVersion` field; defaults to `"latest"` when empty.**
- `internal/services/images/service.go:90-100` — `BuildRequest` struct has `CrossProviderVersion string` (line 96) with a 4-line doc comment explaining the default-to-`"latest"` rule.
- `internal/services/images/service.go:318-321` — defaulting logic in `Build()`:
  ```
  crossVersion := strings.TrimSpace(request.CrossProviderVersion)
  if crossVersion == "" {
      crossVersion = "latest"
  }
  ```
- Test evidence: `TestServiceBuildAddsVersionAndUsesDefaultImageInfo` (line 99) passes `BuildRequest{Version: "0.117.0"}` (no `CrossProviderVersion`), and asserts the emitted slice contains `--build-arg CLAUDE_VERSION=latest` (line 116). Default applied; behaviour observable.

**AC5 — Three updated arg-comparison tests use alphabetic ordering.**
- `internal/services/images/service_test.go:112-119` — `TestServiceBuildAddsVersionAndUsesDefaultImageInfo` `want` slice: `CLAUDE_VERSION=latest` → `CODEX_VERSION=0.117.0` → `VALV_GID=<gid>` → `VALV_UID=<uid>`. Comment on lines 110-111 explicitly documents the alphabetic order.
- `internal/services/images/service_test.go:437-445` — `TestBuildIncludesExtraTags` `wantArgs` slice: same alphabetic order, with `--build-arg CLAUDE_VERSION=latest` preceding `--build-arg CODEX_VERSION=0.117.0`. Inline comment on line 436 documents the order.
- `internal/services/images/service_test.go:464-477` — `TestServiceBuildFallsBackToLegacyBuildWhenBuildxUnavailable` `firstCallParts` slice: same alphabetic order. Also `internal/services/images/service_test.go:506-519` — `wantFallback` slice for the legacy-mode second call mirrors the order. Inline comment on lines 462-463 documents the order.

**WriteDefault context tests (auxiliary evidence).**
- `internal/services/images/service_test.go:387-404` — `TestWriteDefaultCodexContextWritesDockerfile` asserts presence of `@openai/codex@${CODEX_VERSION}`, `@anthropic-ai/claude-code@${CLAUDE_VERSION}`, `/home/valv/.codex`, and `/home/valv/.claude` — proves AC1 at the file-write boundary.
- `internal/services/images/service_test.go:563-584` — `TestWriteDefaultClaudeContextWritesDockerfile` asserts presence of `@anthropic-ai/claude-code@${CLAUDE_VERSION}`, `@openai/codex@${CODEX_VERSION}`, `CLAUDE_CONFIG_DIR=/home/valv/.claude`, `CODEX_HOME=/home/valv/.codex`, `/home/valv/.claude`, `/home/valv/.codex` — proves AC2 (mirror + new `CODEX_HOME`) at the file-write boundary.

### Mage gate re-runs

- `mage testPkg github.com/evanmschultz/valv/internal/services/images`:
  ```
  [PKG PASS] github.com/evanmschultz/valv/internal/services/images (1.28s)
  tests: 29 / passed: 29 / failed: 0
  cover: 79.7% (floor 60.0%) — threshold met
  ```
  Matches the worklog claim exactly (29/29 @ 79.7%).
- `mage build`:
  ```
  [INFO] Building valv (./cmd/valv)
  [SUCCESS] Built valv (./valv)
  ```
  Binary produced; no compile breaks introduced by the new field or arg-emission shape.

### Certificate

- **Premises**
  - Both Dockerfiles install both CLIs and create both home dirs.
  - `Build()` always emits both version build-args regardless of provider.
  - `BuildRequest.CrossProviderVersion` field exists and defaults to `"latest"`.
  - The three arg-comparison tests reflect the sorted slice shape.
  - `mage testPkg` + `mage build` are green.
- **Evidence** — `internal/services/images/service.go:90-100,318-332,562-578,645-742`; `internal/services/images/service_test.go:82-130,377-413,415-456,458-524,553-593`; `internal/adapters/docker/ops.go:78-87`; mage outputs above.
- **Trace or cases** — Every AC mapped to file:line + behaviour assertion; both provider branches of `providerVersionBuildArg()` and `crossProviderVersionBuildArg()` covered by the existing recipe-hash test matrix (codex + claude rows in `TestServiceBuildRecipeHashMatchesProviderDockerfile`, line 599+).
- **Conclusion** — PASS. All five ACs supported by citation-grade evidence; both mage gates re-run green.
- **Unknowns** — None.

## Unit 10.2 — Round 1

- **Reviewer:** go-qa-proof-agent
- **Commit under review:** `2387655` — `feat(claude): unit 10.2 add cross-provider mount + CODEX_HOME env`
- **Verdict:** PASS

### Acceptance criteria evidence

**AC1 — `PrepareRequest.OtherProviderProfileHome string` field added.**
- `internal/adapters/providers/claude/runtime.go:31-37` — field declared inside the `PrepareRequest` struct with a six-line doc comment that explicitly describes both halves of the behaviour (mount at `/home/valv/.codex` read-write + set `CODEX_HOME=/home/valv/.codex` when non-empty; skip when empty). Field is in a comment-separated group after `TempRoot` and before `Logger`, matching the planner's placement guidance.

**AC2 — `PrepareRuntime` conditionally appends cross-mount + env when field non-empty.**
- `internal/adapters/providers/claude/runtime.go:135-142` — conditional block fires when `strings.TrimSpace(request.OtherProviderProfileHome) != ""`:
  - Normalizes the path via `pathutil.Normalize` with a wrapped error (`"prepare claude runtime: normalize other provider home: %w"`).
  - Appends `dockeradapter.NewMountSpec(otherHome, "/home/valv/.codex", false)` (`false` = read-write, per planner's design constraint that codex must be able to write session state back).
  - Sets `env["CODEX_HOME"] = "/home/valv/.codex"`.
- Block placement is correct: after the primary `mounts` slice and `env` map are initialized (lines 124-133) and BEFORE `envPassthrough` is built and the cleanup closure is defined, so the cross-env entry is present in the final returned `PreparedRuntime.Env`.
- Both halves of AC2 live inside the same `if`-block — there is no path where one fires without the other (falsification-relevant: rule out partial behavior).

**AC3 — `TestPrepareRuntimeHasNoCodexEnv` renamed + extended to assert no mount with target `/home/valv/.codex`.**
- `internal/adapters/providers/claude/runtime_test.go:86-123` — `TestPrepareRuntimeSkipsCodexMountWhenNotProvided` is the renamed test. Body:
  - Passes explicit `OtherProviderProfileHome: ""` (line 104) — the cross-mount branch is therefore exercised at its "skip" side.
  - Asserts `CODEX_HOME` is absent from `Env` (lines 115-117). Original AC3 environment assertion preserved.
  - Adds the NEW mount assertion (lines 118-122): iterates over `prepared.Mounts` and fatals if any mount has `Target == "/home/valv/.codex"`. This is the "extension" mandated by AC3.

**AC4 — `TestPrepareRuntimeMountsCodexHomeWhenProvided` added; verifies AC2.**
- `internal/adapters/providers/claude/runtime_test.go:125-162` — new test. Body:
  - Creates `otherHome := t.TempDir()` (line 132).
  - Passes non-empty `OtherProviderProfileHome: otherHome` (line 144) — fires the cross-mount branch.
  - Asserts `prepared.Env["CODEX_HOME"] == "/home/valv/.codex"` (lines 155-157).
  - Asserts a mount exists with target `/home/valv/.codex` via `findMountTarget` (line 158) and that the mount is NOT read-only (lines 159-161). The read-only assertion is an over-AC defensive check that confirms the planner's read-write design constraint is honored.

**AC5 — Existing tests still pass.**
- Pre-existing tests still present and untouched in body: `TestPrepareRuntimeSetsClaudeConfigDirEnv` (line 13), `TestPrepareRuntimeMountsClaudeDir` (line 52), `TestPrepareRuntimeCleanupRemovesTempDir` (line 164), `TestPrepareRuntimePassesThroughTerminalEnv` (line 198), `TestPrepareRuntimeFallsBackWhenTERMEmpty` (line 238), `TestPrepareRuntimeUsesSharedHomeAndSyncsBack` (line 271), `TestAppendUniqueStrings` (line 339), `TestErrorsJoin` (line 349).
- mage gate output (below) confirms all 22 tests pass: 8 pre-existing tests + 2 new/renamed cross-mount tests + 12 others (auth-container helpers, etc., in adjacent files).
- The `TestPrepareRuntimeSetsClaudeConfigDirEnv` test does NOT pass `OtherProviderProfileHome` (line 27-31 omits the field, defaulting to `""`) — therefore the new conditional block is bypassed and the pre-DROP_10 environment assertions remain valid. Confirms no regression in the unset-path semantics.

**AC6 — `mage testPkg .../providers/claude` GREEN with 22 tests @ 78.6% (claim).**
- Re-run output (full, just-now):
  ```
  [PKG PASS] github.com/evanmschultz/valv/internal/adapters/providers/claude (1.27s)
  Test summary
    tests: 22
    passed: 22
    failed: 0
  cover: 78.6% (floor 60.0%) — threshold met
  ```
  Exactly matches the worklog claim (22 / 22 / 78.6%). GREEN.

### Mage gate re-runs

- `mage testPkg github.com/evanmschultz/valv/internal/adapters/providers/claude`: 22/22 pass, 78.6% coverage, floor 60% — GREEN (output captured above).
- `mage build`: `./valv` produced cleanly; no compile breaks anywhere in the binary's transitive dependency graph from the new field or conditional block. GREEN.

### Certificate

- **Premises**
  - The `PrepareRequest` struct exposes a new optional `OtherProviderProfileHome string` field with a clear doc comment.
  - `PrepareRuntime` conditionally adds a `/home/valv/.codex` mount AND sets `CODEX_HOME=/home/valv/.codex` when the field is non-empty, and skips both when empty.
  - The previously-named `TestPrepareRuntimeHasNoCodexEnv` is now `TestPrepareRuntimeSkipsCodexMountWhenNotProvided` AND asserts both the env-absent and mount-absent invariants.
  - A new `TestPrepareRuntimeMountsCodexHomeWhenProvided` test exists and asserts the env-set and mount-present invariants.
  - All pre-existing claude runtime tests still pass.
  - `mage testPkg` + `mage build` are green.
- **Evidence** — `internal/adapters/providers/claude/runtime.go:26-39, 124-142`; `internal/adapters/providers/claude/runtime_test.go:86-162`; mage outputs above (`22/22 @ 78.6%` + `[SUCCESS] Built valv`).
- **Trace or cases**
  - Empty `OtherProviderProfileHome` → `TestPrepareRuntimeSkipsCodexMountWhenNotProvided` exercises lines 124-133 + skips lines 135-142 → env has no `CODEX_HOME`, mounts has no `/home/valv/.codex` target. PASS.
  - Non-empty `OtherProviderProfileHome` (a `t.TempDir()`) → `TestPrepareRuntimeMountsCodexHomeWhenProvided` exercises lines 124-142 → env has `CODEX_HOME=/home/valv/.codex`, mounts contains a read-write `MountSpec` with that target. PASS.
  - Regression path (no field passed at all, as in `TestPrepareRuntimeSetsClaudeConfigDirEnv` and the existing terminal/TERM/shared-home tests) → struct zero-value of `OtherProviderProfileHome` is `""` → conditional branch is bypassed → all pre-existing assertions remain valid. PASS.
- **Conclusion** — PASS. Every AC has direct file:line citation evidence; both mage gates re-run green and match the worklog claim exactly (22 tests @ 78.6%, `./valv` built).
- **Unknowns** — None.

## Unit 10.3 — Round 1

- **Reviewer:** go-qa-proof-agent
- **Commit under review:** HEAD (Unit 10.3 codex.PrepareRuntime cross-provider mount)
- **Verdict:** PASS

### Acceptance criteria evidence

**AC1 — `PrepareRequest.OtherProviderProfileHome string` field added in `codex/runtime.go`.**
- `internal/adapters/providers/codex/runtime.go:24-37` — `PrepareRequest` struct now carries `OtherProviderProfileHome string` (line 35) with a six-line doc comment (lines 29-34) that explicitly describes both halves of the behaviour: mount at `/home/valv/.claude` read-write + set `CLAUDE_CONFIG_DIR=/home/valv/.claude` when non-empty; skip both when empty. Field is positioned after `TempRoot` (line 28) and before `Logger` (line 36), matching the placement style used in the symmetric claude-side Unit 10.2.

**AC2 — `PrepareRuntime` conditionally appends cross-mount + env when field non-empty.**
- `internal/adapters/providers/codex/runtime.go:122-129` — conditional block fires when `strings.TrimSpace(request.OtherProviderProfileHome) != ""`:
  - Normalizes the path via `pathutil.Normalize` with a wrapped error (`"prepare codex runtime: normalize other provider home: %w"`, line 125).
  - Appends `dockeradapter.NewMountSpec(otherHome, "/home/valv/.claude", false)` (line 127) — `false` = read-write, symmetric to the claude-side design constraint so claude can write session state back if needed.
  - Sets `env["CLAUDE_CONFIG_DIR"] = "/home/valv/.claude"` (line 128).
- Both halves of AC2 live inside the same `if`-block — there is no path where one fires without the other, except the normalize-error path which returns early with neither mount nor env applied.

**AC3 (CRITICAL — R2 spec) — Insertion is BEFORE `newBridgeManager` call.**
- Conditional block ends at `internal/adapters/providers/codex/runtime.go:129`.
- `newBridgeManager` call lives at `internal/adapters/providers/codex/runtime.go:131` — `bridgeManager, err := newBridgeManager(ctx, request.Logger)`.
- The conditional is therefore in the simple early section (after `mounts` and `env` are initialized at lines 111-120; before any bridge manager / cleanup closure / translateConfigFile machinery starts). Matches R2 spec exactly.
- This ordering avoids two failure modes the planner flagged: (a) the cross-mount must not depend on bridge manager state, and (b) if a downstream error returns before reaching the conditional, the cross-mount must not be silently dropped.

**AC4 — `TestPrepareRuntimeSkipsClaudeMountWhenNotProvided` exists and asserts both invariants.**
- `internal/adapters/providers/codex/runtime_test.go:420-457` — new test. Body:
  - `t.Parallel()` at line 421.
  - Builds `profileHome` + `projectRoot` + `tempRoot` under `t.TempDir()` (lines 423-432).
  - Passes explicit `OtherProviderProfileHome: ""` (line 438) — the cross-mount branch is therefore exercised at its "skip" side.
  - Asserts `CLAUDE_CONFIG_DIR` is absent from `prepared.Env` (lines 449-451). The map-existence check (`_, ok := prepared.Env["CLAUDE_CONFIG_DIR"]`) is strictly stronger than a `== ""` value check.
  - Asserts no mount has target `/home/valv/.claude` (lines 452-456) — iterates the slice and fatals on any match. Captures the AC's mount-absent invariant directly.

**AC5 — `TestPrepareRuntimeMountsClaudeHomeWhenProvided` exists and asserts mount + env present.**
- `internal/adapters/providers/codex/runtime_test.go:459-506` — new test. Body:
  - `t.Parallel()` at line 460.
  - Creates `claudeHome := filepath.Join(root, "claude-profile")` (line 466) and `os.MkdirAll`s it (lines 473-475).
  - Passes non-empty `OtherProviderProfileHome: claudeHome` (line 481) — fires the cross-mount branch.
  - Asserts `prepared.Env["CLAUDE_CONFIG_DIR"] == "/home/valv/.claude"` (lines 492-494).
  - Uses `findMountTarget` (line 499) to locate the mount with target `/home/valv/.claude`; the helper fatals on miss, so a missing mount fails the test.
  - Asserts the mount source matches the normalized `claudeHome` path (lines 500-502) — `pathutil.Normalize(claudeHome)` is used as the expected value, mirroring the runtime's normalization on the input side.
  - Asserts the mount is NOT read-only (lines 503-505) — over-AC defensive check that confirms the read-write design constraint is honored.

**AC6 — All existing codex runtime tests still pass.**
- Pre-existing tests in `runtime_test.go` (still present, bodies unchanged): `TestPrepareRuntimeNormalizesEnvAndTranslatesConfig` (line 14), `TestPrepareRuntimeOmitsBrokenHostCommandBridgeEntries` (line 126), `TestPrepareRuntimePassesThroughRemoteMCPHeaderEnvWithoutOverlay` (line 180), `TestPrepareRuntimePassesThroughTerminalEnv` (line 225), `TestPrepareRuntimePreservesHostTERM` (line 268), `TestPrepareRuntimeFallsBackWhenTERMEmpty` (line 301), `TestPrepareRuntimeUsesSharedHomeAndOverlaysAccountAuth` (line 334).
- None of those tests pass `OtherProviderProfileHome` — they rely on the zero-value `""`, which deterministically takes the skip branch. So pre-existing semantics are preserved.
- `mage testPkg` output reports `tests: 24 / passed: 24 / failed: 0` — 22 pre-existing + 2 new = 24 total. No regressions.

**AC7 — `mage testPkg github.com/evanmschultz/valv/internal/adapters/providers/codex` GREEN with 24 tests @ 74.9%.**
- Re-run output (full, just-now):
  ```
  [PKG PASS] github.com/evanmschultz/valv/internal/adapters/providers/codex (3.38s)
  Test summary
    tests: 24
    passed: 24
    failed: 0
  cover: 74.9% (floor 60.0%) — threshold met
  ```
  Matches the worklog claim EXACTLY (24 tests @ 74.9%). GREEN.

### Mage gate re-runs

- `mage testPkg github.com/evanmschultz/valv/internal/adapters/providers/codex`: 24/24 pass, 74.9% coverage, floor 60% — GREEN (output captured above).
- `mage build`: `[SUCCESS] Built valv (./valv)` — binary produced cleanly; no compile breaks anywhere in the transitive dependency graph from the new field or conditional block.

### Certificate

- **Premises**
  - The `PrepareRequest` struct exposes a new optional `OtherProviderProfileHome string` field with a clear doc comment.
  - `PrepareRuntime` conditionally adds a `/home/valv/.claude` mount AND sets `CLAUDE_CONFIG_DIR=/home/valv/.claude` when the field is non-empty, and skips both when empty.
  - The conditional block is inserted BEFORE the `newBridgeManager` call, in the simple early section of `PrepareRuntime`.
  - `TestPrepareRuntimeSkipsClaudeMountWhenNotProvided` exists and asserts both the env-absent and mount-absent invariants.
  - `TestPrepareRuntimeMountsClaudeHomeWhenProvided` exists and asserts the env-set and mount-present (read-write) invariants.
  - All pre-existing codex runtime tests still pass.
  - `mage testPkg` + `mage build` are green.
- **Evidence** — `internal/adapters/providers/codex/runtime.go:24-37, 111-129, 131`; `internal/adapters/providers/codex/runtime_test.go:420-506`; mage outputs above (`24/24 @ 74.9%` + `[SUCCESS] Built valv`).
- **Trace or cases**
  - Empty `OtherProviderProfileHome` → `TestPrepareRuntimeSkipsClaudeMountWhenNotProvided` exercises lines 111-120 + skips lines 122-129 → env has no `CLAUDE_CONFIG_DIR`, mounts has no `/home/valv/.claude` target. PASS.
  - Non-empty `OtherProviderProfileHome` (a `t.TempDir()` subdir) → `TestPrepareRuntimeMountsClaudeHomeWhenProvided` exercises lines 111-129 → env has `CLAUDE_CONFIG_DIR=/home/valv/.claude`, mounts contains a read-write `MountSpec` with that target and the normalized source. PASS.
  - Regression path (no field passed, as in every pre-existing test) → struct zero-value of `OtherProviderProfileHome` is `""` → conditional branch is bypassed at line 122 → all pre-existing assertions remain valid. PASS.
  - Placement-before-newBridgeManager (line 129 ends conditional; line 131 starts bridge manager) → no failure path between mount-append and bridge-init can drop the cross-mount silently. PASS.
- **Conclusion** — PASS. Every AC has direct file:line citation evidence; both mage gates re-run green and match the worklog claim exactly (24 tests @ 74.9%, `./valv` built). Critical R2 placement constraint (insertion BEFORE `newBridgeManager`) verified at lines 122-129 vs 131.
- **Unknowns** — None.

### Hylla Feedback

N/A — proof review used `Read` directly on the two Go files plus `mage testPkg` / `mage build` outputs. No Hylla query was attempted, so there is no miss to report.

## Unit 10.4 — Round 1

- **Reviewer:** go-qa-proof-agent
- **Verdict:** PASS
- **Files reviewed:**
  - `internal/services/claude/service.go`
  - `internal/services/claude/service_test.go`
- **Re-run mage gates:**
  - `mage testPkg github.com/evanmschultz/valv/internal/services/claude`: 23/23 pass, 80.9% coverage, floor 60% — GREEN. Matches the worklog claim exactly.
  - `mage build`: `[SUCCESS] Built valv (./valv)` — GREEN.

### AC-by-AC evidence

1. **Cross-binding lookup inserted in `Service.Run` AFTER `resolved` populated AND BEFORE `clauderuntime.PrepareRuntime` call.** `service.go:162-175` sits immediately after the `if s.overrideProfile != nil { ... } else { resolved, err = s.resolveBinding(...) }` convergence block (ends line 160) and before the `clauderuntime.PrepareRuntime(...)` call at line 180. Placement is correct.
2. **Lookup uses `BindingByProjectID(ctx, resolved.project.ID, domain.ProviderCodex)` + `ProfileByID(ctx, otherBinding.ProfileID)`.** `service.go:167` (`s.store.BindingByProjectID(ctx, resolved.project.ID, domain.ProviderCodex)`) and `service.go:169` (`s.store.ProfileByID(ctx, otherBinding.ProfileID)`). Exact symbol + arg match to spec.
3. **ErrNotFound silently skipped; other errors fatal.** `service.go:173-175`: `else if !errors.Is(err, domain.ErrNotFound) { return fmt.Errorf("run claude launch service: lookup codex binding for project %q: %w", resolved.project.Root, err) }`. The success path leaves `otherProfileHome=""` when `ProfileByID` fails too — the nested `profileErr == nil` guard means a profile lookup failure also silently degrades to no cross-mount. Spec says "other errors fatal" applies to the binding lookup; profile-lookup degrade is a defensible Option A behaviour (matches the "fail cleanly, no cross-mount" intent) and is not contradicted by the AC text.
4. **`OtherProviderProfileHome: otherProfileHome` passed to `PrepareRequest`.** `service.go:185` — explicit named field in the `clauderuntime.PrepareRequest{...}` struct literal.
5. **`fakeStore` extended with cross-keyed dispatch (`crossBinding`, `crossBindingErr`, `crossProfile`, `crossProfileErr`).** `service_test.go:29-38` declares all four fields with doc comments. `service_test.go:83-88` (`BindingByProjectID`) dispatches on `provider == domain.ProviderCodex` → returns `crossBinding/crossBindingErr`. `service_test.go:56-61` (`ProfileByID`) dispatches on `id == f.crossProfile.ID` → returns `crossProfile/crossProfileErr`. Dispatch is correct and avoids contaminating the primary path.
6. **CRITICAL — `boundClaudeStore` sets `crossBindingErr: domain.ErrNotFound` as default.** `service_test.go:166-171`: returned `fakeStore` literal includes `crossBindingErr: domain.ErrNotFound`. Every existing test using `boundClaudeStore` opts out of cross-mount via ErrNotFound, so the new lookup is silently skipped and the runtime request remains unchanged from pre-10.4 behaviour. Verified: `TestRunSucceedsWithBoundProject`, `TestValidateBindingReturnsNilForBoundProject`, `TestRunRejectsWorkingDirectoryOutsideProjectRoot`, `TestRunRejectsSiblingPathThatSharesProjectPrefix`, `TestRunBuildsNonInteractiveDockerRequestWhenTTYDisabled`, `TestRunRejectsOverrideProfileWithWrongProvider`, `TestRunBubblesExecutorErrors`, `TestRunUsesOverrideProfileHomePath` all consume `boundClaudeStore` and all pass under the re-run.
7. **`TestRunCrossProviderMountWhenCodexBound` exists with 3 rows (bound, ErrNotFound, store-error).** `service_test.go:617-754`. Rows: `"codex bound"` (line 663) wires `crossBinding=codexBinding`, `crossProfile=codexProfile`, expects mount + env; `"codex not bound (ErrNotFound)"` (line 671) wires `crossBindingErr=domain.ErrNotFound`, expects success + no mount + no env; `"codex store error"` (line 678) wires `crossBindingErr=storeError`, expects non-nil error. Each row's assertions match spec.
8. **All existing tests pass.** Confirmed by the re-run: 23/23 pass. The pre-existing 22 tests + `TestRunCrossProviderMountWhenCodexBound`'s 3-row subtest = 22 base + 1 parent = 23 reported by mage (subtests collapse to the parent in the test count). No regressions.
9. **`mage testPkg .../services/claude` GREEN (claim: 23 @ 80.9%).** Re-run captured above matches exactly: `23 tests passed`, `80.9%` coverage.

### Certificate

- **Premises**
  - The cross-binding lookup sits at the exact convergence point (after `resolved` populated, before `clauderuntime.PrepareRuntime`).
  - Lookup arg + symbol shape matches the spec (`BindingByProjectID(ctx, resolved.project.ID, ProviderCodex)` then `ProfileByID(ctx, otherBinding.ProfileID)`).
  - `ErrNotFound` is silently skipped; other errors return as wrapped fatal.
  - `OtherProviderProfileHome` is passed into `PrepareRequest`.
  - `fakeStore` exposes four cross-keyed fields with correct dispatch.
  - `boundClaudeStore` defaults `crossBindingErr: domain.ErrNotFound`, opting all pre-existing tests out of cross-mount.
  - `TestRunCrossProviderMountWhenCodexBound` covers all three rows.
  - `mage testPkg` + `mage build` are green and match the worklog claim exactly.
- **Evidence** — `service.go:162-187`, `service_test.go:29-38`, `service_test.go:56-61`, `service_test.go:83-88`, `service_test.go:154-172`, `service_test.go:617-754`; mage outputs (`23/23 @ 80.9%`, `[SUCCESS] Built valv`).
- **Trace or cases**
  - "Codex bound" → `BindingByProjectID(ProviderCodex)` returns `codexBinding,nil` → `ProfileByID(codexBinding.ProfileID)` returns `codexProfile,nil` → `otherProfileHome=codexProfileHome` → `PrepareRequest.OtherProviderProfileHome=codexProfileHome` → `PrepareRuntime` appends `/home/valv/.codex` mount + sets `CODEX_HOME` → assertions PASS.
  - "Codex not bound (ErrNotFound)" → `BindingByProjectID(ProviderCodex)` returns `_,ErrNotFound` → `errors.Is(err, ErrNotFound)` true → silent skip → `otherProfileHome=""` → `PrepareRuntime` conditional bypassed → no `/home/valv/.codex` mount, no `CODEX_HOME` → assertions PASS.
  - "Codex store error" → `BindingByProjectID(ProviderCodex)` returns `_,storeError` → `errors.Is(err, ErrNotFound)` false → `return fmt.Errorf(...)` → Run returns non-nil → assertion PASS.
  - Regression path: every existing test goes through `boundClaudeStore` → `crossBindingErr=ErrNotFound` → silent skip → request unchanged → all pre-existing assertions remain valid. Confirmed by 23/23 green.
- **Conclusion** — PASS. All nine ACs have direct file:line citation evidence. Both mage gates re-run green and match the worklog claim exactly (23 tests @ 80.9%, `./valv` built). The plan-QA promoted advisory (`boundClaudeStore` default) is honored — no regression on any pre-existing test.
- **Unknowns** — None.

### Hylla Feedback

N/A — proof review used `Read` directly on the two Go files (both modified in this drop, so necessarily stale in Hylla) plus `mage testPkg` / `mage build` outputs. No Hylla query was attempted, so there is no miss to report.
