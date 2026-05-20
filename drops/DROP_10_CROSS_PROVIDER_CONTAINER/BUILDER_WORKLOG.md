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

## Unit 10.2 — Round 1

- **Builder:** go-builder-agent
- **Started:** 2026-05-19
- **Files touched:**
  - `internal/adapters/providers/claude/runtime.go`
  - `internal/adapters/providers/claude/runtime_test.go`
  - `drops/DROP_10_CROSS_PROVIDER_CONTAINER/PLAN.md` (state flip)
  - `drops/DROP_10_CROSS_PROVIDER_CONTAINER/BUILDER_WORKLOG.md` (this file)

- **Symbols added:**
  - `PrepareRequest.OtherProviderProfileHome string` — optional host-side profile home of the other provider (codex). When non-empty, `PrepareRuntime` mounts it at `/home/valv/.codex` read-write and sets `CODEX_HOME=/home/valv/.codex` in the container env. When empty, cross-mount and env var are skipped.

- **Production changes (runtime.go):**
  - `PrepareRequest` struct: added `OtherProviderProfileHome string` field with doc comment, in a comment-separated group after `TempRoot` and before `Logger`.
  - Reformatted all four `debugLog(request.Logger, ...)` call sites to `debugLog(\n\trequest.Logger,\n\t...)` to satisfy gofumpt's multi-line call hugging rule — these were pre-existing calls that gofumpt re-evaluated once the struct field alignment changed.
  - `PrepareRuntime`: added conditional cross-mount block (lines ~133–140 after edit) after the primary `env` map is built and before `envPassthrough`. Block: normalize `OtherProviderProfileHome` via `pathutil.Normalize`, append `NewMountSpec(otherHome, "/home/valv/.codex", false)` to `mounts`, set `env["CODEX_HOME"] = "/home/valv/.codex"`.

- **Test changes (runtime_test.go):**
  - Renamed `TestPrepareRuntimeHasNoCodexEnv` → `TestPrepareRuntimeSkipsCodexMountWhenNotProvided`. Updated body: passes explicit `OtherProviderProfileHome: ""`, asserts `CODEX_HOME` absent from `Env` AND no mount with target `/home/valv/.codex`.
  - Added `TestPrepareRuntimeMountsCodexHomeWhenProvided`: passes `OtherProviderProfileHome: t.TempDir()`, asserts `Env["CODEX_HOME"] == "/home/valv/.codex"` AND `findMountTarget` finds a non-read-only mount at `/home/valv/.codex`.

- **Mage gate results:**
  - `mage testPkg github.com/evanmschultz/valv/internal/adapters/providers/claude`: 22 tests passed, 0 failed, coverage 78.6% (floor 60%). GREEN.
  - `mage build`: `./valv` produced cleanly. GREEN.

## Hylla Feedback (Unit 10.2)

- `hylla_search_keyword` with `query=NewMountSpec` returned the correct node (`github.com/evanmschultz/valv/internal/adapters/docker/NewMountSpec`) in one query — Hylla answered this lookup correctly. Followed up with a direct `Read` of `types.go` to confirm the `MountSpec` struct field names (`Source`, `Target`, `ReadOnly`) since Hylla returns the node ID but not the struct field layout inline. Suggestion: Hylla struct nodes could expose field names in their summary to save the follow-up `Read`.
- All other evidence (runtime.go current state, existing test names, imports) gathered via `Read` on files not yet in context — no Hylla misses on Go symbol queries.

## Unit 10.3 — Round 1

- **Builder:** go-builder-agent
- **Started:** 2026-05-19
- **Files touched:**
  - `internal/adapters/providers/codex/runtime.go`
  - `internal/adapters/providers/codex/runtime_test.go`
  - `drops/DROP_10_CROSS_PROVIDER_CONTAINER/PLAN.md` (state flip)
  - `drops/DROP_10_CROSS_PROVIDER_CONTAINER/BUILDER_WORKLOG.md` (this file)

- **Symbols added:**
  - `PrepareRequest.OtherProviderProfileHome string` — optional host-side profile home of the other provider (claude). When non-empty, `PrepareRuntime` mounts it at `/home/valv/.claude` read-write and sets `CLAUDE_CONFIG_DIR=/home/valv/.claude` in the container env. When empty, cross-mount and env var are skipped.

- **Production changes (runtime.go):**
  - `PrepareRequest` struct: added `OtherProviderProfileHome string` field with doc comment (mirroring Unit 10.2's claude runtime pattern), in a comment-separated group after `TempRoot` and before `Logger`.
  - `PrepareRuntime`: added conditional cross-mount block immediately after the `env := map[string]string{...}` block and before the `newBridgeManager` call (original lines 113→115). Block: normalize `OtherProviderProfileHome` via `pathutil.Normalize`, append `NewMountSpec(otherHome, "/home/valv/.claude", false)` to `mounts`, set `env["CLAUDE_CONFIG_DIR"] = "/home/valv/.claude"`. `strings` and `pathutil` were already imported — no import changes needed.

- **Insertion site:** Between the `env` map initialization (original line 113) and the `newBridgeManager` call (original line 115). This is in the early simple section of `PrepareRuntime`, before any complex bridge-manager or config-translation logic — correct per spec.

- **Test additions (runtime_test.go):**
  - `TestPrepareRuntimeSkipsClaudeMountWhenNotProvided`: passes explicit `OtherProviderProfileHome: ""`, asserts `CLAUDE_CONFIG_DIR` absent from `Env` AND no mount with target `/home/valv/.claude`. Marked `t.Parallel()`.
  - `TestPrepareRuntimeMountsClaudeHomeWhenProvided`: creates a real `t.TempDir()` for `claudeHome`, passes it as `OtherProviderProfileHome`, asserts `Env["CLAUDE_CONFIG_DIR"] == "/home/valv/.claude"` AND `findMountTarget` returns a non-read-only mount at `/home/valv/.claude` with the normalized `claudeHome` as source. Marked `t.Parallel()`.

- **TDD cycle:**
  - RED: tests added first; `mage testPkg` produced build error (field `OtherProviderProfileHome` undefined).
  - GREEN: production changes landed; `mage testPkg` — 24 tests passed, 0 failed, 74.9% coverage.

- **Mage gate results:**
  - `mage testPkg github.com/evanmschultz/valv/internal/adapters/providers/codex`: 24 tests passed (was 22), 0 failed, coverage 74.9% (floor 60%). GREEN.
  - `mage build`: `./valv` produced cleanly. GREEN.

## Hylla Feedback (Unit 10.3)

None — Hylla answered everything needed. The existing codex `runtime.go` and `runtime_test.go` structures were confirmed via `Read` (not Hylla) since Unit 10.2 had just been completed and the files were not yet re-ingested. The claude runtime (Unit 10.2's output) was confirmed via `Read` to validate the mirror pattern. No Hylla queries were issued for Go symbol lookups in this unit — evidence was gathered via file reads for files changed since last ingest. Categorized as N/A (changed files, Hylla stale until reingest).
