## Plan — Round 2

**Verdict:** PASS

Round 1 blockers F1 (`recipeHash()` Codex-hardcoded) and F2 (no test catches F1) are both credibly fixed. The revised Unit 4.1 adds `providerDockerfileContent()` on `Service`, rewires `recipeHash()` through it, and mandates a table-driven `TestServiceBuildRecipeHashMatchesProviderDockerfile` covering both provider cases. Attacks on the fixes each refute under evidence from `internal/services/images/service.go` (HEAD), `internal/cli/operator_helpers.go`, `internal/cli/manage.go`, `internal/cli/extended_test.go`, `internal/adapters/docker/ops.go`.

### Findings

#### 1.1 F1 fix: `providerDockerfileContent()` + `recipeHash()` rewire — REFUTED

**Attack:** Does the fix fully cover the recipe-hash path? `recipeHash()` currently reads `content := DefaultCodexDockerfile()` unconditionally (`service.go:442`). The revision replaces that with `s.providerDockerfileContent()`, which switches on `s.provider`. For this to produce the right hash in production, `s.provider` must be correctly set at `New()` time for both providers, through the entire caller chain.

**Trace — production call chain:**
- `New(Options)` sets `s.provider = options.Provider`; when empty it defaults to `ProviderCodex` (`service.go:200-203`). Verified.
- `openImagesService(cmd, paths, provider)` per the revised plan (PLAN.md:148-156) explicitly passes `Provider: domain.ProviderCodex` (Codex branch) or `Provider: domain.ProviderClaude` (Claude branch). Both paths thread through `Options.Provider` into `Service.provider`.
- `runManageUpdate` (PLAN.md:189-193) calls `openImagesService(cmd, paths, provider)` with the `provider` argument passed in from `parseOptionalProvider(args, domain.ProviderCodex)` at `manage.go:1072`. Claude path reaches `Service` with `s.provider == ProviderClaude`.
- `Service.Build()` calls `s.recipeHash()` at `service.go:257` to populate the `io.valv.recipe_hash` label. The Claude `Build()` call therefore exercises the `providerDockerfileContent()` helper with `s.provider == ProviderClaude` → returns `DefaultClaudeDockerfile()` → hash is correct.

No hole in the chain. REFUTED.

#### 1.2 Does `TestServiceBuildRecipeHashMatchesProviderDockerfile` actually run without a Docker daemon? — REFUTED

**Attack:** Plan says "reuse `*runnerRecorder`" and inspect the `--label` args. Does the fake runner capture `--label` with enough fidelity?

**Trace:** `runnerRecorder.Run` at `service_test.go:25-33` appends every args slice to `r.calls`. Existing test `TestServiceBuildAddsVersionAndUsesDefaultImageInfo` at `service_test.go:79-111` already asserts on `runner.calls[0]` via `reflect.DeepEqual`, with the expected slice containing `"--label", fmt.Sprintf("%s=%s", recipeHashLabel, svc.recipeHash())` at position 17-18 of the arg list (line 107). So the recipe-hash label IS captured as an arg and inspectable. `docker.BuildImageArgs` at `ops.go:89-98` sorts labels alphabetically before emitting `--label k=v` pairs, so ordering is stable. The Claude test case can inspect `runner.calls[0]` and find `--label io.valv.recipe_hash=<hex>` at a deterministic position (or simply scan for the key prefix). REFUTED.

#### 1.3 `s.provider` default when `Options.Provider` is empty — REFUTED

**Attack:** `New()` defaults `provider` to `ProviderCodex` when empty (`service.go:200-203`). Are there existing tests that construct `Options` without setting `Provider` AND expect a non-Codex recipe hash? If so, they would misclassify through the new `providerDockerfileContent()` default branch.

**Trace via `grep` `Options\{` in `internal/services/images`:**
- `service_test.go:83, 124, 177, 228, 275, 313, 390, 421, 464` — every call omits `Provider` or sets it implicitly through `providerImageStateStore.state.Provider = domain.ProviderCodex`.
- `service_integration_test.go:32, 71` — Codex integration tests only.

No existing test constructs a Service with `Provider: ProviderClaude` today. Plan adds Claude-specific tests (PLAN.md:113, 117) that DO set `Provider: domain.ProviderClaude` explicitly. The default branch therefore only handles Codex today, which is the correct behavior. REFUTED.

#### 1.4 Custom-Dockerfile basename sentinel interaction — REFUTED (both branches land correctly)

**Attack:** `recipeHash()` preserves the custom-Dockerfile fallback: `if filepath.Base(s.dockerfile) != defaultCodexDockerfile { read from disk }`. The sentinel constant `defaultCodexDockerfile = "Dockerfile"` at `service.go:26` is Codex-named but semantically is "default basename" regardless of provider. Does this misfire for Claude?

**Trace — two cases:**
- **Claude default path**: `WriteDefaultClaudeContext` writes to `filepath.Join(root, "Dockerfile")` (plan mirrors Codex shape exactly, PLAN.md:90). `openImagesService` Claude branch passes no `Dockerfile` override, so `dockerfile` defaults to `"Dockerfile"` (`service.go:184-187`). `filepath.Base(s.dockerfile) != "Dockerfile"` → FALSE → falls through to `s.providerDockerfileContent()` → returns `DefaultClaudeDockerfile()` → correct hash.
- **Hypothetical custom Claude Dockerfile** (e.g. `Claude.Dockerfile`): sentinel FALSE-match → TRUE branch → disk read → uses file content directly → correct. The default branch (`providerDockerfileContent()`) is only the fallback when the disk read fails.

Both cases land correctly. The sentinel is provider-agnostic in effect, only provider-named by history. REFUTED. Plan notes this implicitly by preserving the fallback unchanged (PLAN.md:97-106).

#### 1.5 Does `recipeHash()` execute during `Build()` or only `EnsureLatest()`? — REFUTED

**Attack:** If `recipeHash()` is only called during `EnsureLatest`, the Claude test (which uses `Build`, not `EnsureLatest`) would never exercise the hash path.

**Trace:** `service.go:257` — `Service.Build()` populates `buildRequest.Labels[recipeHashLabel] = s.recipeHash()` directly. Confirmed: `recipeHash()` is called during `Build()`. The Claude `Build()` path at test time will emit `--label io.valv.recipe_hash=<hex>` in `runner.calls[0]`, and the test can assert on it. REFUTED.

#### 1.6 TUI deferral wording — REFUTED

**Attack:** Does "`ActionUpdate` dispatch remains Codex-hardcoded after DROP_4" read as an obligation?

**Trace:** The sentence lives inside the "Explicitly deferred (NOT in DROP_4)" block (PLAN.md:30-37). The block header establishes the deferral context unambiguously. No obligation implied. REFUTED.

#### 1.7 Spinner-wrapper acceptance testability — MILD UNDER-SPEC (not a blocker)

**Attack:** Plan acceptance (PLAN.md:207) requires the Claude branch to wrap `service.Build(...)` in `runWithCLIQuietSpinner(...)`. The explicitly enumerated tests (PLAN.md:198-200) do not assert stderr for the Claude spinner strings. Can a QA agent verify the wrapper is there?

**Trace:** Two verification paths work:
- **Source inspection**: QA can grep `internal/cli/manage.go` for `runWithCLIQuietSpinner` inside the Claude branch. Fine.
- **Functional**: the existing Codex harness at `extended_test.go:442-446` asserts stderr contains `"Checking provider image"` and `"Provider image check complete"`. The Claude harness (PLAN.md:197-198 — "TestRunManageUpdateClaudeBuildsImage") doesn't spell this out, but nothing prevents the builder from adding `"Building provider image"` / `"Provider image built"` substring assertions on stderr.

Polish note for the builder, not a plan blocker. The acceptance criterion is verifiable either way. REFUTED as a blocker.

#### 1.8 R2-did-not-break-R1 — REFUTED

**Attack:** Did the R2 revision violate the package-lock rule or break the `blocked_by` chain?

**Trace:**
- Unit 4.1 paths are strictly within `internal/services/images` (PLAN.md:74). Unit 4.2 is `internal/cli` (PLAN.md:136). Unit 4.3 is `internal/cli` (PLAN.md:183). 4.2 `blocked_by: 4.1` and 4.3 `blocked_by: 4.2` serialize the `internal/cli` edits. No regression.
- `supportedProviders` drift noted in R1 is addressed at PLAN.md:51 with current-source-verified line number.
- Notes section (PLAN.md:213-219) preserved and extended with npm fallback.
- No other fields churned unnecessarily.

REFUTED.

#### 1.9 `providerDockerfileContent()` vs `WriteDefaultClaudeContext` edge case — REFUTED

**Attack:** What if a Service is constructed with `Provider: ProviderCodex` but `WriteDefaultClaudeContext` was used to populate the context dir (misconfiguration)? `providerDockerfileContent()` returns `DefaultCodexDockerfile()` → hash mismatches what's actually on disk.

**Trace:** This is only possible if a caller mixes providers (e.g. calls `WriteDefaultClaudeContext` then `New(Options{Provider: ProviderCodex, ContextDir: <claude-dir>})`). The plan's `openImagesService` dispatches on provider and keys both the context-dir path (`filepath.Join(paths.BuildCacheDir, string(provider))`, PLAN.md:152) and the writer call on the same provider, so they cannot drift. No production caller can reach the mixed-provider state. A caller who bypasses `openImagesService` would be responsible for its own consistency — out of scope for the plan. REFUTED.

### 2. Unmitigated counterexamples

None.

### 3. Routed unknowns

- **Spinner-wrapper test rigor** (finding 1.7). Builder may strengthen `TestRunManageUpdateClaudeBuildsImage` by asserting stderr contains the Claude-specific spinner strings (`"Building provider image"` / `"Provider image built"`), mirroring the Codex coverage at `internal/cli/extended_test.go:442-446`. Optional polish — plan passes without it.

### 4. Verdict

**PASS.** Round 2 remediates F1 and F2 with provider-keyed `providerDockerfileContent()` + table-driven `TestServiceBuildRecipeHashMatchesProviderDockerfile` covering both providers. Every attack angle — production call-chain coverage, fake-runner fidelity, default-provider interaction, custom-Dockerfile sentinel, `Build`-vs-`EnsureLatest` hash path, TUI deferral wording, package-lock regression, mixed-provider misconfiguration — refutes under current-HEAD evidence. Plan is ready for Phase 3 discuss-and-advance.
