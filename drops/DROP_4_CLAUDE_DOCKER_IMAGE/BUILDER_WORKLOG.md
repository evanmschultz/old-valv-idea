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
