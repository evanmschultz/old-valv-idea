# DROP_N — Builder Worklog

Append a `## Unit N.M — Round K` section per build attempt. See `main/drops/WORKFLOW.md` § "Phase 4 — Build (per unit)" for what each section should contain.

## Unit N.1 — Round 1

- **Builder:** go-builder-agent
- **Started:** YYYY-MM-DD HH:MM
- **Files touched:** <list>
- **Mage targets run:** mage build (pass), mage test (pass), …
- **Notes:** <design choices, surprises, library quirks>

<…repeat per unit + per round…>

## Unit 4.1 — Round 1

- **Builder:** go-builder-agent
- **Started:** 2026-04-21
- **Files touched:**
  - `internal/services/images/service.go` (added `DefaultClaudeCLIVersion`, `DefaultClaudeDockerfile`, `WriteDefaultClaudeContext`, `providerDockerfileContent`, `providerVersionBuildArg` helpers on `Service`; fixed `recipeHash()` to be provider-keyed; generalized the `CODEX_VERSION` build-arg key in `Build()` to route through `providerVersionBuildArg()`)
  - `internal/services/images/service_test.go` (added `TestWriteDefaultClaudeContextWritesDockerfile`, `TestDefaultClaudeCLIVersionIsNonEmpty`, `TestServiceBuildRecipeHashMatchesProviderDockerfile`; added `crypto/sha256`, `encoding/hex`, `regexp` imports)
  - `internal/services/images/service_integration_test.go` (new file, `//go:build integration`; preserved the two existing Codex integration tests that previously lived in this file and added `TestWriteDefaultClaudeContextBuildsWithExistingUIDAndGID` — the file already existed with the Codex tests, so "new file" means the Claude test was added; nothing Codex-side was changed)
- **Mage targets run:**
  - `mage testPkg ./internal/services/images` → PASS (16 tests, 74.8% coverage, above the 60% floor; gofumpt clean)
- **Claude CLI version pinned:** `2.1.89`
- **Context7 verification:** `/anthropics/claude-code` lookup on 2026-04-21. Available versions reported: `v2.1.39`, `v2.1.89`. Pinned to `2.1.89` — matches the Unit 4.1 spec proposal, no pivot required.
- **Design notes:**
  - `providerDockerfileContent()` is a `Service` method (not a package-level function) because it consults `s.provider`. Keeping it method-scoped matches the single-owner invariant for `recipeHash()`'s default branch.
  - The custom-Dockerfile fallback in `recipeHash()` is preserved byte-for-byte — if `filepath.Base(s.dockerfile) != defaultCodexDockerfile` and the file reads cleanly, the on-disk bytes win regardless of provider. A Claude service with a custom Dockerfile still hashes the custom file contents, not `DefaultClaudeDockerfile()`. This matches the Codex behavior today.
  - `providerVersionBuildArg()` was added to `service.go` alongside `providerDockerfileContent()`. The Unit 4.1 spec enumerates the Dockerfile ARG name change (`ARG CLAUDE_VERSION`) but does not explicitly call out the matching `Build()` build-arg key swap. Without it, a Claude build with `Provider: domain.ProviderClaude` would still pass `--build-arg CODEX_VERSION=<ver>` and the Claude Dockerfile's `ARG CLAUDE_VERSION` would resolve to empty, breaking the `@anthropic-ai/claude-code@` install. Unit 4.3's expected `--build-arg CLAUDE_VERSION=<DefaultClaudeCLIVersion>` assertion also depends on this. The helper is in scope (`internal/services/images/service.go`) per the Unit 4.1 `Paths`.
  - Alphabetical ordering of `BuildArgs` map keys: Codex order is `CODEX_VERSION`, `VALV_GID`, `VALV_UID`; Claude order is `CLAUDE_VERSION`, `VALV_GID`, `VALV_UID`. The version arg stays in position 0 of the sorted slice for both providers, so existing Codex test fixtures (positional `reflect.DeepEqual` comparisons) are unaffected.
  - Map-literal alignment in `Build()` shifted to `s.providerVersionBuildArg(): version` with `VALV_GID`/`VALV_UID` re-aligned; gofumpt accepted the result.

## Hylla Feedback

None — Hylla was not needed for this unit. The scope was tightly confined to `internal/services/images/service.go` + `service_test.go`, and all symbol lookups (existing `DefaultCodexDockerfile`, `WriteDefaultCodexContext`, `recipeHash`, `runnerRecorder`, `Build`, `New`, `Options`, `Service`, `BuildArgs`) were resolvable via direct `Read` of the two files the Unit description pointed at. Context7 answered the Claude CLI version pin verification. No Hylla query was needed; therefore no miss to record.

## Unit 4.2 — Round 1

- **Builder:** go-builder-agent
- **Started:** 2026-04-21
- **Files touched:**
  - `internal/cli/operator_helpers.go` (refactored `openImagesService` to take a `domain.Provider` argument; switch on provider with a `default` returning `fmt.Errorf("initialize image service: unsupported provider %q", provider)`; Codex branch preserves exact previous behavior — `WriteDefaultCodexContext`, `codexVersionResolverFactory(nil)`, `codexImageRepository()`, `codexImageTag()`, now also passes `Provider: domain.ProviderCodex` explicitly into `imagesservice.Options`; Claude branch calls `WriteDefaultClaudeContext`, `Resolver: nil`, `claudeImageRepository()`, `claudeImageTag()`, `Provider: domain.ProviderClaude`; `contextDir` now derived from `filepath.Join(paths.BuildCacheDir, string(provider))` so Codex writes to `.../codex/Dockerfile` and Claude to `.../claude/Dockerfile`; store closed on every error path before returning)
  - `internal/cli/claude_image.go` (new file; added `claudeImageRef() dockeradapter.ImageRef`, `claudeImageRepository() string`, `claudeImageTag() string` mirroring `codexImageRef`/`codexImageRepository`/`codexImageTag`; honors `VALV_CLAUDE_IMAGE` env with fallback `("valv-claude", "dev")`)
  - `internal/cli/claude_image_test.go` (new file; added `TestClaudeImageRefDefaults`, `TestClaudeImageRefParsesOverrideWithTag`, `TestClaudeImageRefParsesOverrideWithoutTag`, `TestClaudeImageRepositoryUsesDefault`, `TestClaudeImageRepositoryUsesOverride`, `TestClaudeImageTagDefaults`, `TestClaudeImageTagUsesOverride`, `TestClaudeImageTagFallsBackWhenOverrideHasNoTag`, `TestOpenImagesServiceClaudeContextWritesDockerfile`, `TestOpenImagesServiceCodexContextWritesToCodexSubdir`, `TestOpenImagesServiceUnsupportedProvider`)
  - `internal/cli/codex.go` (added `"github.com/evanmschultz/valv/internal/domain"` import; `ensureCodexImageCurrent` now calls `openImagesService(cmd, paths, domain.ProviderCodex)`)
  - `internal/cli/manage.go` (`runManageUpdate` now calls `openImagesService(cmd, paths, domain.ProviderCodex)`; Unit 4.3 will refactor further)
- **Mage targets run:**
  - `mage testPkg ./internal/cli` → PASS (112 tests, 72.6% coverage; 11 new tests added across `claude_image_test.go`; gofumpt clean)
- **Design notes:**
  - Placed Claude image helpers in new `internal/cli/claude_image.go` rather than appending to `codex.go`. Rationale: keeps provider-specific helpers file-separated — `codex.go` already houses Codex command wiring, image helpers, and auth flow; adding Claude helpers there would muddy the file. The drop's Unit 4.2 spec explicitly permits this placement ("new file `internal/cli/claude_image.go` permitted if builder prefers to keep Claude helpers separate from `codex.go`").
  - Used a single shared `imagesservice.Options` struct populated with defaults then switch-customized per provider, rather than two fully-separate constructions. Reason: the `Runner`, `StateStore`, `ContextDir`, `Dockerfile`, `UserID`, `GroupID`, `Logger` fields are identical across both providers; a shared base + per-provider override is more compact without introducing new abstraction.
  - Codex branch passes `Provider: domain.ProviderCodex` explicitly. Previously the Options literal omitted the Provider field — `imagesservice.New` defaulted an empty Provider to `ProviderCodex`, so the external behavior is unchanged, but the call site now states intent explicitly for readability and symmetry with the Claude branch.
  - Caller update list (grep `openImagesService(` across `internal/cli/*.go` + `internal/cli/*_test.go` pre-edit): `operator_helpers.go:68` (definition), `manage.go:1090`, `codex.go:242`. Zero existing test-side call sites — tests exercised `openImagesService` transitively through `newManageCommand` / `runCodexCommand`. All three direct callers updated to pass `domain.ProviderCodex` explicitly; `extended_test.go:448` continues to verify the Codex-subdir Dockerfile path (`filepath.Join(paths.BuildCacheDir, string(domain.ProviderCodex), "Dockerfile")`) which is already the correct shape for the refactor.
  - Store close discipline preserved across every error branch, including the new `default` case (`_ = store.Close()` before returning the unsupported-provider error).
  - `TestOpenImagesServiceCodexContextWritesToCodexSubdir` added beyond the Unit spec's two required tests: it explicitly asserts that calling the Codex provider leaves no Claude Dockerfile lying around, guarding against future regressions that accidentally collapse the two provider subdirs back onto each other.

## Hylla Feedback

None — Hylla was not needed for Unit 4.2. All required symbols (`openImagesService`, `codexImageRef`, `codexImageRepository`, `codexImageTag`, `ensureCodexImageCurrent`, `codexVersionResolverFactory`, `domain.ProviderCodex`, `domain.ProviderClaude`, `imagesservice.Options`, `imagesservice.WriteDefaultCodexContext`, `imagesservice.WriteDefaultClaudeContext`) were resolvable via direct `Read` + `Grep` of the files the Unit spec pointed at. No fallback from a failed Hylla query; nothing to record.
