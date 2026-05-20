# DROP_10 — Build QA Falsification

## Unit 10.1 — Round 1

**Commit under review:** `47ecbc4` — `feat(images): unit 10.1 dual-CLI Dockerfiles + cross-provider build-arg`.

**Files in diff:**

- `internal/services/images/service.go` (+41 / -11)
- `internal/services/images/service_test.go` (+90 / -10)
- `drops/DROP_10_CROSS_PROVIDER_CONTAINER/{PLAN.md,BUILDER_WORKLOG.md}` (docs)

The orchestrator's appendix referenced a `Dockerfile` file plus changes to `internal/cli/extended_test.go` — the actual diff touches no standalone `Dockerfile` (recipes are Go heredoc strings inside `service.go`) and no `extended_test.go`. The `fakeCodexRecipeHash` / `fakeClaudeRecipeHash` helpers there are dynamic SHA-256 over `imagesservice.Default{Codex,Claude}Dockerfile()` results, so no `extended_test.go` edit was required.

### Attacks attempted

**1. Build-arg ordering correctness — REFUTED.**

`internal/adapters/docker/BuildImageArgs` (via Hylla `hylla_node_full`) collects `BuildArgs` map keys into `keys := make([]string, 0, len(request.BuildArgs))`, then `sort.Strings(keys)`, then emits `--build-arg <KEY>=<VAL>` in that order. The three updated `want` slices follow alphabetic order `CLAUDE_VERSION < CODEX_VERSION < VALV_GID < VALV_UID`. Confirmed exact match. No counterexample.

**2. `"latest"` default reproducibility — REFUTED.**

`service.go:90-100` declares `BuildRequest.CrossProviderVersion string` with godoc saying empty → `"latest"`. `service.go:316-318` (inside `Build`) does `crossVersion := strings.TrimSpace(request.CrossProviderVersion); if crossVersion == "" { crossVersion = "latest" }`. The only production caller, `Service.EnsureLatest` at `service.go:465-470`, constructs `BuildRequest{Version, Pull, NoCache, ExtraTags}` with no `CrossProviderVersion` — the empty-string path applies and `"latest"` is substituted. Repo-wide `rg "images\.BuildRequest"` finds zero external constructions. Existing tests that don't compare full `args` slices (e.g. `TestServiceBuildRecipeHashMatchesProviderDockerfile`) are unaffected because they only inspect the `recipe_hash` label. No counterexample.

**3. Recipe-hash drift — REFUTED.**

`internal/cli/extended_test.go:840-848`:

```go
func fakeCodexRecipeHash() string {
    sum := sha256.Sum256([]byte(imagesservice.DefaultCodexDockerfile()))
    return hex.EncodeToString(sum[:])
}
func fakeClaudeRecipeHash() string {
    sum := sha256.Sum256([]byte(imagesservice.DefaultClaudeDockerfile()))
    return hex.EncodeToString(sum[:])
}
```

Both helpers compute SHA-256 over the live function returns, so they auto-update when the Dockerfile text changes. No frozen literal hash. No counterexample.

**4. Dockerfile syntax — REFUTED.**

Read both heredocs (`service.go:646-677` codex, `service.go:708-741` claude).

Codex: `ARG CODEX_VERSION` → `RUN npm install --global "@openai/codex@${CODEX_VERSION}"` → `ARG CLAUDE_VERSION` → `RUN npm install --global "@anthropic-ai/claude-code@${CLAUDE_VERSION}"`. Each `ARG` is in scope for its immediately-following `RUN`. Variable substitution syntax `${...}` correct.

Claude: same shape with `CLAUDE_VERSION` first then `CODEX_VERSION`. Well-formed.

Both end with `USER valv` → `WORKDIR /workspace` → `ENTRYPOINT [...]`. No counterexample.

**5. `mkdir -p` extension preserves permissions — REFUTED.**

`service.go:658` (codex): `mkdir -p /home/valv/.codex /home/valv/.claude /workspace`. `service.go:720` (claude): `mkdir -p /home/valv/.claude /home/valv/.codex /workspace`. Both followed by `chown -R "${VALV_UID}:${VALV_GID}" /home/valv /workspace` in the same `RUN` step, so the recursive chown covers both subdirectories under `/home/valv` plus `/workspace`. No counterexample.

**6. `CODEX_HOME=/home/valv/.codex` ENV in claude Dockerfile conflict — REFUTED.**

Claude `ENV` block (`service.go:723-730`) now sets both `CLAUDE_CONFIG_DIR=/home/valv/.claude` and `CODEX_HOME=/home/valv/.codex`. The two env vars serve distinct CLIs (`claude` reads `CLAUDE_CONFIG_DIR`, `codex` reads `CODEX_HOME`) and point at distinct directories (both `mkdir -p`'d in the preceding `RUN`). No collision. No counterexample.

**7. Symbol drift / production callers — REFUTED.**

Repo-wide `rg "images\.BuildRequest"` returns zero non-doc hits. Within the package, the only `BuildRequest{...}` construction outside tests is `EnsureLatest` (`service.go:465`), which leaves `CrossProviderVersion` empty and relies on the `"latest"` default. No counterexample.

**8. Coverage delta plausibility — REFUTED.**

`mage testPkg .../internal/services/images` reports 79.7% coverage, threshold 60.0%, 29/29 tests pass. The pre-10.1 baseline was at the same gate (`mage test` blocks below threshold), and the unit added new branches plus matching test coverage. Plausible.

### Required gates

- `mage testPkg github.com/evanmschultz/valv/internal/services/images` → **PASS**, 29/29 tests, 79.7% coverage (≥ 60% gate).
- `mage build` → **PASS**, produced `./valv`.

### Verdict

**PASS.** No CONFIRMED counterexample across the eight enumerated attack vectors. Both required mage gates re-ran green on HEAD.

## Unit 10.2 — Round 1

**Commit under review:** `2387655` — `feat(claude): unit 10.2 add cross-provider mount + CODEX_HOME env`.

**Files in diff:**

- `internal/adapters/providers/claude/runtime.go` (+25 / -5)
- `internal/adapters/providers/claude/runtime_test.go` (+50 / -5)
- `drops/DROP_10_CROSS_PROVIDER_CONTAINER/{PLAN.md,BUILDER_WORKLOG.md}` (docs)

Plan-aligned: `paths` declared in `PLAN.md:9-10` are exactly the two production files touched. No file-scope drift.

### Attacks attempted

**1. Conditional placement — REFUTED.**

`runtime.go:124-133` initializes the primary `mounts := []dockeradapter.MountSpec{ NewMountSpec(runtimeClaudeHome, ContainerClaudeDir, false) }` and the primary `env := map[string]string{...}` BEFORE the new conditional block at `runtime.go:135-142`. The conditional uses `append` and map-key write, both additive — primary mount and existing env keys (`CLAUDE_CONFIG_DIR`, `HOME`, `LOGNAME`, `TERM`, `USER`) are preserved. No clobbering. No counterexample.

**2. `os.MkdirAll` for the cross-home path — REFUTED.**

Code does NOT pre-create `otherHome`. Plan 10.2 spec (`PLAN.md:155-172`) doesn't require it: the mount target is "the raw profile home, not a staged copy; no sync-back is needed." In production, the codex-bound profile's `HomePath` was created by `valv account bind` for codex; it always exists. Test passes `t.TempDir()`, which exists. If a future caller passes a non-existent path, Docker bind-mount creates it as root-owned — that's the docker-adapter contract layer, not Unit 10.2's responsibility. Out of scope. No counterexample.

**3. Mount read-write vs read-only — REFUTED.**

Via Hylla `hylla_node_full` on `github.com/evanmschultz/valv/internal/adapters/docker/NewMountSpec`: `func NewMountSpec(source, target string, readOnly bool) MountSpec { return MountSpec{..., ReadOnly: readOnly} }`. The third argument `false` in `runtime.go:140` sets `MountSpec.ReadOnly = false`, i.e. read-write. `runtime.go:31-37` docstring confirms intent: "mounts it at /home/valv/.codex read-write … so the other CLI can authenticate using its native auth-file layout." Codex CLI writes session/state — read-write is correct. The new test (`runtime_test.go:158-161`) asserts `mount.ReadOnly` is false. No counterexample.

**4. gofumpt reformatting cosmetic-only — REFUTED.**

The four `debugLog` call-site changes (diff lines around 92-93, 117-118, 149-150, 167-168) are pure whitespace: the first arg `request.Logger` is wrapped onto its own line. Same logger, same message string, identical key-value pairs in identical order. No behavior change. No counterexample.

**5. Existing test count reconciliation — REFUTED.**

`mage testPkg .../internal/adapters/providers/claude` reports 22 tests. The claude package contains test files beyond `runtime_test.go` (auth tests, container tests, etc.) which contribute to the 22-count base. Unit 10.2's net delta: one rename (`TestPrepareRuntimeHasNoCodexEnv` → `TestPrepareRuntimeSkipsCodexMountWhenNotProvided`, no count change) plus one new test (`TestPrepareRuntimeMountsCodexHomeWhenProvided`) = +1 net. Builder's claim consistent. No counterexample.

**6. Empty whitespace string — REFUTED.**

`runtime.go:135`: `strings.TrimSpace(request.OtherProviderProfileHome) != ""`. Whitespace-only input (`"   "`) trims to `""`, condition is false, cross-mount skipped. Matches plan's Option A semantics ("not provided" treated as "not set"). Mirrors the existing `SharedHome` whitespace handling at `runtime.go:83`. No counterexample.

**7. `pathutil.Normalize` error branch test coverage — EXHAUSTED, no counterexample found.**

`runtime.go:136-139` returns `fmt.Errorf("prepare claude runtime: normalize other provider home: %w", err)` on normalize failure. No dedicated test exercises this branch. However: (a) the four existing pathutil-normalize call sites in `PrepareRuntime` (lines 67, 71, 75, 84) for `ProfileHome`/`ProjectRoot`/`TempRoot`/`SharedHome` similarly have no per-field error tests — pre-existing pattern, not a 10.2 regression; (b) coverage threshold is 78.6%, well above the 60%-per-package gate enforced by `mage testPkg`; (c) `pathutil.Normalize` failure modes are well-covered in `pathutil`'s own tests. Minor noted gap, consistent with package precedent. Not a falsification.

**8. Mount target collision — REFUTED.**

Pre-10.2 `mounts` slice has one entry with target `ContainerClaudeDir = "/home/valv/.claude"`. The new mount target is `/home/valv/.codex` — different path. No target collision. The Unit 10.1 claude Dockerfile now has `mkdir -p /home/valv/.codex` in the user-creation `RUN` block so the target directory exists in-container at bind time. No counterexample.

### Required gates

- `mage testPkg github.com/evanmschultz/valv/internal/adapters/providers/claude` → **PASS**, 22/22 tests, 78.6% coverage (≥ 60% gate).
- `mage build` → **PASS**, produced `./valv`.

### Hylla Feedback

- **Query**: `hylla_search_keyword` for `OtherProviderProfileHome` against `github.com/evanmschultz/valv@main`, `fields=["content"]`.
  - **Missed because**: snapshot is pre-drop-end (latest_commit `eecf29d` on baseline; HEAD is `2387655` — uncommitted-at-ingest territory). Hylla reingest is drop-end-only per project rules, so newly-added symbols in Unit 10.1/10.2 are not yet indexed.
  - **Worked via**: `git diff HEAD~1 HEAD` plus direct `Read` on `internal/adapters/providers/claude/runtime.go` and `runtime_test.go`.
  - **Suggestion**: this is expected per the drop-end-only reingest policy, not a Hylla bug. No action needed; recording per the protocol.

### Verdict

**PASS.** No CONFIRMED counterexample across the eight enumerated attack vectors. Attack 7 (normalize-error coverage gap) flagged as EXHAUSTED with a noted minor gap consistent with the package's existing pattern — not a falsification. Both required mage gates re-ran green on HEAD (`mage testPkg .../claude`: 22/22 tests, 78.6% coverage; `mage build`: green).

## Unit 10.3 — Round 1

**Commit under review:** `90ac73a` — `feat(codex): unit 10.3 add cross-provider mount + CLAUDE_CONFIG_DIR env`.

**Files in diff:**

- `internal/adapters/providers/codex/runtime.go` (+17 / -1)
- `internal/adapters/providers/codex/runtime_test.go` (+88 / -0)
- `drops/DROP_10_CROSS_PROVIDER_CONTAINER/{PLAN.md,BUILDER_WORKLOG.md}` (docs)

Plan-aligned: `PLAN.md:202` declares `paths` = the two production files touched. No file-scope drift.

### Attacks attempted

**1. Insertion site correctness — REFUTED.**

The new conditional block at `runtime.go:122-129` is placed AFTER `mounts := []dockeradapter.MountSpec{...}` (lines 111-113) and `env := map[string]string{...}` (lines 114-120) are initialized, and BEFORE `bridgeManager, err := newBridgeManager(...)` at line 131. The block uses `append` on `mounts` and a map-key write on `env` — both purely additive against the primary entries. Primary mount `runtimeCodexHome → /home/valv/.codex` (line 112) is preserved; primary env keys `CODEX_HOME`, `HOME`, `LOGNAME`, `TERM`, `USER` (lines 115-119) are preserved. No clobbering. No counterexample.

**2. Downstream mount-slice append ordering — REFUTED.**

After the cross-mount block, `PrepareRuntime` appends additional mounts at `runtime.go:195` (profile overlay → `/home/valv/.codex/config.toml`, read-only) and `runtime.go:215` (project overlay → `<projectRoot>/.codex/config.toml`, read-only). These appends extend the slice; the cross-mount lives at index 1. Tests use target-based lookup (`findMountTarget`) not index-based access, and downstream Docker consumes the slice as unordered `-v` flags (Docker bind-mount semantics are order-insensitive across distinct targets). No counterexample.

**3. bridgeManager interaction with env/mounts — REFUTED.**

`bridgeManager` is created at `runtime.go:131` AFTER the cross-mount block. Per `bridge.go:22-167`, the manager owns its own state (HTTP listeners, bridge command processes) and never reads or modifies the outer `env` map or `mounts` slice — `translateConfigFile` passes the manager to translate MCP server entries inside config TOML, which writes to the overlay file, not to the outer `env`/`mounts`. No conflict.

**4. Cleanup contract on cross-mount-normalize error — REFUTED (inherited pre-existing pattern).**

When `OtherProviderProfileHome` is set but `pathutil.Normalize` fails at `runtime.go:124`, `PrepareRuntime` returns at line 125 — the `runtimeDir` created at line 91 (`os.MkdirTemp`) is NOT cleaned up on this early-return path. However: (a) this is the identical pattern already present in the file for the pre-existing early-returns at lines 99-101 (`os.MkdirAll(runtimeCodexHome)` failure) and 102-104 (`copyDirContents` failure), neither of which removes `runtimeDir`; (b) the same pattern exists in `claude/runtime.go` (lines 111-113, 114-116) and was accepted by Unit 10.2 falsification attack 7 as "consistent with the package's existing pattern, not a 10.2 regression"; (c) `pathutil.Normalize` failure modes (per `pathutil` tests) are essentially impossible for valid host-side paths the upstream wiring layer would pass. Not a 10.3-introduced regression. No counterexample.

**5. config.toml translation interaction with /home/valv/.claude — REFUTED.**

`translateConfigFile` calls at `runtime.go:162-173` (profile) and `runtime.go:200-211` (project) write overlay TOML to `runtimeDir`-rooted paths and then either stage into `runtimeCodexHome` (lines 188-193) or mount at `ContainerCodexDir/config.toml` (line 195) or at `<projectConfigPath>` (line 215). All overlay targets are under `/home/valv/.codex` or `<projectRoot>/.codex`. The cross-mount target `/home/valv/.claude` is a disjoint container path. Zero interaction. No counterexample.

**6. auth.json cross-pollination — REFUTED.**

The cross-mount points the host's claude profile home (containing `.credentials.json` — claude's auth shape) at `/home/valv/.claude` in the container. The codex CLI inside the container reads its auth from `CODEX_HOME` (set to `/home/valv/.codex` at line 115), which maps to the primary mount at line 112 (`runtimeCodexHome` with codex's `auth.json`). Codex CLI does NOT look under `/home/valv/.claude` for auth. `CLAUDE_CONFIG_DIR=/home/valv/.claude` (set at line 128) is read by the claude CLI when invoked-from-inside-the-codex-container, pointing it at claude's native auth-file layout. Two CLIs, two distinct env vars, two distinct mount points, two distinct auth-file shapes. No cross-pollination. No counterexample.

**7. Test isolation — REFUTED.**

Both new tests (`TestPrepareRuntimeSkipsClaudeMountWhenNotProvided` at `runtime_test.go:420`, `TestPrepareRuntimeMountsClaudeHomeWhenProvided` at `runtime_test.go:459`) use independent `t.TempDir()` allocations for `profileHome`, `projectRoot`, `tempRoot`, and `claudeHome`. Both call `t.Parallel()`. No shared state, no env-var leakage between them. No counterexample.

**8. Test count delta — REFUTED.**

Builder claims 22 → 24 (+2) in the codex package. Verified: `runtime_test.go` now contains 9 top-level `Test*` functions (was 7; the diff adds exactly 2 — `TestPrepareRuntimeSkipsClaudeMountWhenNotProvided` and `TestPrepareRuntimeMountsClaudeHomeWhenProvided`, no renames). Package-wide test count comes from three files: `account_test.go` (3 `Test*` funcs), `runtime_test.go` (9), `bridge_test.go` (8) = 20 top-level — but mage reports 24, indicating 4 `t.Run` subtests across `bridge_test.go`. Re-run `mage testPkg .../codex` confirms `tests: 24 / passed: 24 / failed: 0`. Delta matches claim. No counterexample.

### Required gates

- `mage testPkg github.com/evanmschultz/valv/internal/adapters/providers/codex` → **PASS**, 24/24 tests, 74.9% coverage (≥ 60% gate).
- `mage build` → **PASS**, produced `./valv`.

### Hylla Feedback

None — Hylla not queried for this round. All Go-symbol evidence (current `runtime.go` shape, test function inventory, `bridgeManager` surface in `bridge.go`, callers via `rg`, comparison against `claude/runtime.go`) was gathered via `Read` and `rg` because (a) HEAD `90ac73a` is mid-drop with three commits post-baseline `eecf29d` — Hylla index is necessarily stale for the touched files until drop-end reingest, and (b) all evidence-bearing files were either in the diff itself or directly adjacent under `git status`. Per protocol this is N/A (changed files, Hylla stale until reingest), not a Hylla miss.

### Verdict

**PASS.** No CONFIRMED counterexample across the eight enumerated attack vectors. Attack 4 (cleanup-on-normalize-error) flagged as an inherited pre-existing pattern shared by the claude package (already accepted at Unit 10.2 falsification) and by other early-return sites in the same function — not a 10.3 regression. Both required mage gates re-ran green on HEAD (`mage testPkg .../codex`: 24/24 tests, 74.9% coverage; `mage build`: green).

## Unit 10.4 — Round 1

**Commit under review:** `d449416` — `feat(claude): unit 10.4 wire codex cross-binding lookup in service`.

**Files in diff:**

- `internal/services/claude/service.go` (cross-binding lookup block + `OtherProviderProfileHome` pass-through)
- `internal/services/claude/service_test.go` (`fakeStore` cross-fields + `ProfileByID` / `BindingByProjectID` dispatch + `boundClaudeStore` default + `TestRunCrossProviderMountWhenCodexBound` table-driven)
- `drops/DROP_10_CROSS_PROVIDER_CONTAINER/{PLAN.md,BUILDER_WORKLOG.md}` (docs)

### Attacks attempted

**1. `fakeStore` zero-value cross-binding leak across pre-existing fixtures — REFUTED.**

The Plan-QA R2 advisory required every pre-existing fixture using `boundClaudeStore` to inherit `crossBindingErr: domain.ErrNotFound` so the cross lookup short-circuits without dereferencing a zero-value `crossBinding`. Confirmed at `service_test.go:170` — the helper sets `crossBindingErr: domain.ErrNotFound` exactly once, and every reachable cross-lookup site is therefore safe. Flow-traced the remaining inline `fakeStore{}` literals:

- `TestNewRequiresDependencies` (line 186-191): `New()` rejects at construction → `Run` never reached → cross-lookup unreached.
- `TestRunReturnsUnboundProjectWhenNoProject` (line 287): `projectErr` triggers inside `resolveBinding` (service.go:144-148) → returns before cross-lookup.
- `TestRunReturnsUnboundProjectWhenNoBinding` (line 313): `bindingErr` triggers inside `resolveBinding` (service.go:269-275) → returns before cross-lookup.
- `TestRunRejectsWrongBindingProvider` (line 339): wrong-provider guard at service.go:276-278 → exits before cross-lookup.
- `TestRunRejectsWrongProfileProvider` (line 374): wrong-profile guard at service.go:287-289 → exits before cross-lookup.

All tests that do reach the cross-lookup go through `boundClaudeStore`, which carries the ErrNotFound default. No counterexample.

**2. `ProfileByID` dispatch correctness with zero/empty IDs — REFUTED.**

`service_test.go:57` guards the cross-profile branch with `if f.crossProfile.ID != "" && id == f.crossProfile.ID`. The non-empty cross-ID requirement is explicit; when `crossProfile` is the zero value (`ID == ""`), the guard fails and dispatch falls through to the primary profile. There is no path where an empty requested ID coincidentally matches a zero-value cross profile.

**3. Insertion site — REFUTED.**

`service.go:160` closes the `else { resolveBinding }` branch; both branches converge into a fully-populated `resolved`. Lines 162-175 perform the cross-lookup. Line 180 calls `clauderuntime.PrepareRuntime` with `OtherProviderProfileHome: otherProfileHome`. The insertion site is exactly between resolution convergence and `PrepareRuntime`, as required.

**4. `resolved.project.ID` reference — REFUTED.**

`resolvedLaunchBinding` (service.go:243-247) embeds `project domain.Project`. `domain.Project.ID` exists as a string field (verified by grepping the domain package — `Project` struct carries `ID string`, used throughout `resolveBinding` at line 269 with `projectRecord.ID`). The cross-lookup at line 167 uses `resolved.project.ID` correctly.

**5. `ProfileByID` error handling parity with Option A — REFUTED.**

`service.go:169-172` uses a nested `if profileErr == nil` block. When `ProfileByID` returns ANY non-nil error (including ErrNotFound or arbitrary store errors), `otherProfileHome` remains empty and the cross-mount is silently skipped. This is consistent with the dev-decided Option A semantics (orphaned binding tolerated, silent skip). The outer binding-lookup is strictly typed (ErrNotFound silent, anything else fatal), but the inner profile lookup is uniformly silent — the deliberate asymmetry matches the spec.

**6. `otherProfileHome` empty-string passthrough — REFUTED.**

`internal/adapters/providers/claude/runtime.go:135` guards the cross-mount block with `if strings.TrimSpace(request.OtherProviderProfileHome) != ""`. When `Service.Run` passes the empty default, the runtime layer skips both the mount append and the `CODEX_HOME` env var. The contract is symmetric end-to-end.

**7. Test row coverage — REFUTED.**

`TestRunCrossProviderMountWhenCodexBound` (service_test.go:617-754):

- Row `"codex bound"` (line 663-669): asserts `wantCodexMount` (mount target `/home/valv/.codex` sourced from `codexProfileHome`) AND `wantCodexEnv` (`CODEX_HOME == /home/valv/.codex`).
- Row `"codex not bound (ErrNotFound)"` (line 670-675): asserts NO `/home/valv/.codex` mount AND empty `CODEX_HOME` env.
- Row `"codex store error"` (line 677-683): asserts `Run` returns non-nil error; early-returns at line 720 to avoid stale-state checks.

Each row has independent assertions matching its semantics. No row is a false positive.

**8. Service mock dependencies / caller signature stability — REFUTED.**

`Service.Run` signature unchanged: `(ctx context.Context, cwd string, claudeArgs []string) error`. The cross-lookup is purely internal store-call expansion. No external caller (cli, tui) sees a different surface.

**9. Stripping `ProfileByID` orphaned-binding path — REFUTED.**

Confirmed by the dev-decided Option A spec: when a binding row resolves but its `ProfileID` no longer exists, `ProfileByID` returns ErrNotFound, the inner `if profileErr == nil` skips, and `otherProfileHome` stays empty → cross-mount silently skipped. Identical UX to "codex not bound". Consistent with the planner's stated semantics.

**10. Cross-pollution from existing test fixtures — REFUTED.**

Re-verified attack 1's flow trace. All five inline `fakeStore{}` literals exit before reaching the cross-lookup; the single helper `boundClaudeStore` carries the ErrNotFound default. No fixture leaves `crossBindingErr` as nil at a site that can reach the cross-lookup. Adversarial steelman: could a future test author add an inline `fakeStore{}` literal whose flow reaches cross-lookup? Yes, but that future risk is not a 10.4 regression — the doc comment at `service_test.go:29-31` explicitly tells future authors to set `crossBindingErr: domain.ErrNotFound` to opt out. Mitigation documented.

### Mage gates (re-run on HEAD `d449416`)

- `mage testPkg github.com/evanmschultz/valv/internal/services/claude`: 23 tests passed, 0 failed, coverage 80.9% (floor 60%). GREEN.
- `mage build`: `./valv` produced cleanly. GREEN.

### Hylla Feedback

None — Hylla not queried for this round. All Go-symbol evidence (current `service.go` shape, `service_test.go` fixtures, `runtime.go` cross-mount contract, `resolvedLaunchBinding` struct, `domain.Project.ID` field) was gathered via `Read` because HEAD `d449416` is mid-drop with four commits post-baseline — Hylla index is necessarily stale for the touched files until drop-end reingest. Per protocol this is N/A (changed files, Hylla stale until reingest), not a Hylla miss.

### Verdict

**PASS.** No CONFIRMED counterexample across the ten enumerated attack vectors. Plan-QA R2's promoted advisory (default `crossBindingErr: domain.ErrNotFound` in `boundClaudeStore`) was applied correctly; the single-helper consolidation means no inline-literal fixture leaks. Cross-binding lookup placement, profile-dispatch correctness, runtime empty-string passthrough, and test-row coverage all confirmed via direct code reads. Mage gates re-ran green (`mage testPkg .../claude`: 23/23 tests, 80.9% coverage; `mage build`: green).

## Unit 10.5 — Round 1

**Commit under review:** `5bcaecc` — `feat(codex): unit 10.5 wire claude cross-binding lookup in service`.

**Files in diff:**

- `internal/services/codex/service.go` (+21 / -5)
- `internal/services/codex/service_test.go` (+234 / -37)
- `drops/DROP_10_CROSS_PROVIDER_CONTAINER/{PLAN.md,BUILDER_WORKLOG.md}` (docs)

### Attacks attempted

**1. fakeStore opt-out completeness — REFUTED.**

Plan-QA R2 advisory + 10.4 lesson: every inline `fakeStore{...}` literal whose Run path reaches the new cross-lookup must set `crossBindingErr: domain.ErrNotFound`. Enumerated all 11 inline `fakeStore{` sites in `service_test.go` (lines 166, 194, 222, 337, 365, 394, 424, 502, 547, 606, 739):

- Line 166 is inside `boundCodexStore`'s own body — has `crossBindingErr: domain.ErrNotFound` at line 170.
- Lines 194, 222, 337, 365, 394, 424, 502, 547, 606 are existing Run-path tests — each has `crossBindingErr: domain.ErrNotFound` added (verified at lines 196, 226, 341, 369, 398, 428, 506, 551, 610 respectively).
- Line 739 is the new `TestRunCrossProviderMountWhenClaudeBound` fixture — table-driven, populates `crossBindingErr` from `tc.crossBindingErr` per row (`ErrNotFound`, `nil`, and a custom store error).

All Run-path fakeStore literals correctly opt out. No site leaves `crossBindingErr` as the zero-value `nil` at a position the cross-lookup can reach. No counterexample.

**2. macOS symlink in temp dirs — REFUTED.**

`service_test.go:681-685` calls `filepath.EvalSymlinks(t.TempDir())` to normalize the macOS `/var/folders/...` → `/private/var/folders/...` symlink. On linux CI where `t.TempDir()` returns a path without symlinks, `EvalSymlinks` is effectively a no-op (returns the input path unchanged, modulo cleaning). Cross-platform safe. The expected mount source uses `claudeProfileHome` directly (same EvalSymlinks-normalized value) so the comparison at line 779-780 matches what `pathutil.Normalize` produces inside `codexruntime.PrepareRuntime`. No counterexample.

**3. Insertion site — REFUTED.**

The cross-binding lookup at `service.go:155-168` sits after both `overrideProfile` (lines 122-146) and `else { resolved = resolveBinding(...) }` (lines 147-153) — i.e. `resolved` is fully populated. Lines 170-178 call `codexruntime.PrepareRuntime` with `OtherProviderProfileHome: otherProfileHome`. The lookup is between `resolved` convergence and runtime preparation as required. Downstream logic (`sharedHome = s.sharedCodexStateHome(resolved.profile)` at line 170, `s.emitNotices(...)` at line 183, request building at 185-188, `runAttached` at 206-209) is untouched. The `sharedHome` value is computed from `resolved.profile` (primary codex profile), not cross profile — so cross-binding has no effect on shared-state pathing. No counterexample.

**4. `detectAlways` helper conflict — REFUTED.**

`detectAlways` is defined at `internal/services/codex/service_test.go:174` and a sibling defined at `internal/services/claude/service_test.go:174`. Different packages = different scopes; Go allows the same name in different packages. No conflict; no test compilation issue. Both functions return their respective package's `DetectFunc` type (since both packages define an unrelated `DetectFunc` type alias).

**5. Test count delta — REFUTED.**

Pre-10.5 service_test.go had 13 top-level `Test*` funcs (verified via `git show HEAD~1:internal/services/codex/service_test.go | grep -c '^func Test'`). Post-10.5 has 14 (one new: `TestRunCrossProviderMountWhenClaudeBound`). `mage testPkg` reports 17 tests because the new test runs three table-driven sub-rows ("claude bound", "claude not bound (ErrNotFound)", "claude store error") and `go test -json` counts sub-tests independently. 14 parent funcs + 3 reported sub-rows from the new table-driven test = 17 reported tests. Reconciled.

**6. `ProfileByID` dispatch — REFUTED.**

`fakeStore.ProfileByID` at lines 57-62 dispatches on `id`: when `f.crossProfile.ID != "" && id == f.crossProfile.ID`, returns `(f.crossProfile, f.crossProfileErr)`; otherwise returns primary `(f.profile, f.profileErr)`. The guard `f.crossProfile.ID != ""` short-circuits the cross check when fixtures don't set `crossProfile` — so existing tests that lookup primary `profile.ID` always return the primary profile. The new cross-mount test sets `crossProfile.ID = "profile-claude-1"` and `binding.ProfileID = "profile-codex-1"` so the two ID spaces never collide. Dispatch correct.

**7. `resolved.project.ID` reference — REFUTED.**

`resolvedLaunchBinding` at `service.go:247-251` carries `project domain.Project`; `domain.Project` exposes `ID string` field (used by existing `BindingByProjectID(ctx, projectRecord.ID, ...)` in `resolveBinding` at line 273). `resolved.project.ID` accesses the same field; no nil-pointer risk because `resolved.project` is a value type, not a pointer. Correct reference.

**8. Symmetry with 10.4 — REFUTED.**

Side-by-side compare of `internal/services/claude/service.go:162-175` vs `internal/services/codex/service.go:155-168`:

- claude: `BindingByProjectID(..., domain.ProviderCodex)` + error wrap `"run claude launch service: lookup codex binding for project %q: %w"`.
- codex: `BindingByProjectID(..., domain.ProviderClaude)` + error wrap `"run codex launch service: lookup claude binding for project %q: %w"`.
- Both share the identical inner structure: `if err == nil { otherProfile, profileErr := s.store.ProfileByID(...); if profileErr == nil { otherProfileHome = otherProfile.HomePath } } else if !errors.Is(err, domain.ErrNotFound) { return ... }`.
- Both pass `OtherProviderProfileHome: otherProfileHome` to their respective `PrepareRuntime`.

Provider-swap symmetric. The codex side additionally carries `SharedHome: sharedHome` (the codex-specific shared state pattern from before DROP_10) which the claude side intentionally omits (`SharedHome: ""`, claude's isolated-first model). That asymmetry predates DROP_10 and is unrelated to the cross-binding feature. No counterexample.

**9. Error message scope drift — REFUTED.**

The new wrap `"run codex launch service: lookup claude binding for project %q: %w"` matches the package's existing wrap style: every error in `service.go` is prefixed `"run codex launch service: …"` (verified via `rg 'run codex launch service' service.go` → 11 matches, all consistent). The format mirrors the existing `"run codex launch service: lookup project %q: %w"` and `"run codex launch service: lookup binding for project %q: %w"` patterns at lines 140 and 278. No scope drift.

**10. Coverage decline plausibility — REFUTED.**

Codex package coverage 76.0% vs claude 80.9% (Δ = 4.9 points). The codex package has additional code paths not present in the claude package: (a) `sharedCodexStateHome` (lines 228-240) computes a shared-host home via `codexruntime.DefaultHostProfile`; (b) `emitNotices` (lines 356-369) — codex-specific MCP-warning surface that claude doesn't have; (c) `runAttached` (lines 217-226) — codex-specific attached-run wrapper. Each adds untested branches (e.g. `s.realHome == ""` skip in `sharedCodexStateHome`, `DefaultHostProfile` error path). The delta is plausible. No counterexample.

### Additional findings (not counterexamples)

**Dead helper `boundCodexStore` defined but unused.** The builder added `boundCodexStore` at `service_test.go:154-172` (mirroring `boundClaudeStore` from the claude package) but did NOT refactor any of the 10 inline `fakeStore{...}` Run-path literals to use it. In the claude package, `boundClaudeStore` is invoked at 8+ call sites (lines 230, 413, 436, 461, 486, 557, 591, 777). In the codex package, the helper has zero call sites. This is asymmetric with 10.4's pattern and leaves the helper as dead code. The current magefile does not run `staticcheck`/U1000, so this does not break the build — `mage testPkg` passes — and the inline opt-outs are functionally correct. Flagging as a minor code-hygiene concern, not a counterexample to functional correctness. If a future drop enables staticcheck, this will trip U1000.

**Silent profile-error swallow on cross-lookup.** The cross-binding lookup at `service.go:159-168` silently swallows any `ProfileByID` error (e.g. orphan binding pointing at a deleted profile, transient store error). The PLAN.md prescribed exactly this pattern and the claude side at `service.go:166-175` (10.4) carries the identical shape. Symmetric across both providers; matches PLAN.md prescribed code at PLAN.md:322-333. Not a 10.5 regression — the design choice predates 10.5 in 10.4. Flagging for awareness only; if surfaced as a future concern, both providers must be updated together.

### Mage gates (re-run on HEAD `5bcaecc`)

- `mage testPkg github.com/evanmschultz/valv/internal/services/codex`: 17 tests passed, 0 failed, coverage 76.0% (floor 60%). GREEN.
- `mage build`: `./valv` produced cleanly. GREEN.

### Hylla Feedback

N/A — task touched files changed since last ingest (codex `service.go` and `service_test.go` are new DROP_10 edits, codex `runtime.go` updated in 10.3 is also stale until drop-end reingest). All evidence gathered via `Read` + `rtk grep` on the current checkout. No Hylla queries issued.

### Verdict

**PASS.** No CONFIRMED counterexample across the ten enumerated attack vectors. The fakeStore opt-out coverage is complete across all 10 inline Run-path literals; the macOS symlink fix (`filepath.EvalSymlinks`) is cross-platform safe; insertion-site placement, dispatch correctness, symmetry with 10.4, and error-wrap style all confirmed. Mage gates green on HEAD `5bcaecc` (17/17 tests, 76.0% coverage; build green). Two minor non-counterexample findings recorded for awareness: dead `boundCodexStore` helper (asymmetric with 10.4 usage; harmless until staticcheck lands) and silent profile-error swallow (symmetric design carried from 10.4; matches PLAN.md prescription).
