# DROP_10 — Builder Worklog

Append a `## Unit N.M — Round K` section per build attempt. See `main/drops/WORKFLOW.md` § "Phase 4 — Build (per unit)" for what each section should contain.

## Unit 10.1 — Round 1

- **Builder:** go-builder-agent
- **Started:** 2026-05-19
- **Files touched:**
  - `internal/services/images/service.go`
  - `internal/services/images/service_test.go`
  - `drops/DROP_10_CROSS_PROVIDER_CONTAINER/PLAN.md` (state flip)
  - `drops/DROP_10_CROSS_PROVIDER_CONTAINER/BUILDER_WORKLOG.md` (this file)

- **Symbols added:**
  - `BuildRequest.CrossProviderVersion string` — optional secondary CLI version for the cross-provider npm install; defaults to `"latest"` in `Build()` when empty.
  - `Service.crossProviderVersionBuildArg() string` — returns the OTHER provider's build-arg key (`CLAUDE_VERSION` when building codex, `CODEX_VERSION` when building claude).

- **Dockerfile changes:**
  - `DefaultCodexDockerfile()`: extended `mkdir -p` to `mkdir -p /home/valv/.codex /home/valv/.claude /workspace`; added `ARG CLAUDE_VERSION` + `RUN npm install --global "@anthropic-ai/claude-code@${CLAUDE_VERSION}"` layer after the codex install.
  - `DefaultClaudeDockerfile()`: extended `mkdir -p` to `mkdir -p /home/valv/.claude /home/valv/.codex /workspace`; added `CODEX_HOME=/home/valv/.codex` to `ENV` block; added `ARG CODEX_VERSION` + `RUN npm install --global "@openai/codex@${CODEX_VERSION}"` layer after the claude install.

- **Build() changes:**
  - Computes `crossVersion` (defaults to `"latest"` when `request.CrossProviderVersion` is empty).
  - Inserts `s.crossProviderVersionBuildArg(): crossVersion` into `BuildArgs` map alongside the primary version arg. `docker.BuildImageArgs` sorts map keys alphabetically → emitted order is always `CLAUDE_VERSION` < `CODEX_VERSION` < `VALV_GID` < `VALV_UID`.

- **Test updates:**
  - `TestServiceBuildAddsVersionAndUsesDefaultImageInfo`: updated `want` slice to include `--build-arg CLAUDE_VERSION=latest` before `--build-arg CODEX_VERSION=0.117.0`.
  - `TestBuildIncludesExtraTags`: same pattern; extracted to named `wantArgs` variable for readability.
  - `TestServiceBuildFallsBackToLegacyBuildWhenBuildxUnavailable`: rebuilt `firstCall` and `wantFallback` slices to include both build-args in alphabetical order; `firstCall` key now matches the actual first-call args so the error triggers correctly.
  - `TestWriteDefaultCodexContextWritesDockerfile`: added assertions for `@anthropic-ai/claude-code@${CLAUDE_VERSION}`, `/home/valv/.codex`, and `/home/valv/.claude`.
  - `TestWriteDefaultClaudeContextWritesDockerfile`: added assertions for `@openai/codex@${CODEX_VERSION}`, `CODEX_HOME=/home/valv/.codex`, `/home/valv/.claude`, and `/home/valv/.codex`.
  - Recipe-hash tests (`TestServiceBuildRecipeHashMatchesProviderDockerfile`, `svcRecipeHashForTest`) auto-updated — they call the Dockerfile functions dynamically.

- **Mage gate results:**
  - `mage testPkg github.com/evanmschultz/valv/internal/services/images`: 29 tests passed, 0 failed, coverage 79.7% (floor 60%).
  - `mage build`: PASS — `./valv` produced cleanly.

## Hylla Feedback

Hylla was queried to confirm the `docker.BuildImageArgs` sort behavior. The function is in `internal/adapters/docker/ops.go`, which is a Go file covered by Hylla. The information was confirmed via direct `Read` of `ops.go` rather than Hylla because the relevant detail (the `sort.Strings(keys)` call on line 83) is more efficiently verified by reading the file directly. No Hylla query was issued that produced a miss — the task was scoped to files I already had in context from the file reads. Categorized as: N/A (evidence gathered via Read for a file already loaded in context; Hylla not queried).
