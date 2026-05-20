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
