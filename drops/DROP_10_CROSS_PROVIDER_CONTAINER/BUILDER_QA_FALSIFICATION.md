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
