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
