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

## Unit 10.4 — Round 1

- **Builder:** go-builder-agent
- **Started:** 2026-05-19
- **Files touched:**
  - `internal/services/claude/service.go`
  - `internal/services/claude/service_test.go`
  - `drops/DROP_10_CROSS_PROVIDER_CONTAINER/PLAN.md` (state flip)
  - `drops/DROP_10_CROSS_PROVIDER_CONTAINER/BUILDER_WORKLOG.md` (this file)

- **Production changes (`service.go`):**
  - Added cross-binding lookup block in `Service.Run`, inserted between the `resolved` population convergence point and the `clauderuntime.PrepareRuntime` call (original line 162 area). Block: call `s.store.BindingByProjectID(ctx, resolved.project.ID, domain.ProviderCodex)`; on success, call `s.store.ProfileByID(ctx, otherBinding.ProfileID)` and set `otherProfileHome`; on `domain.ErrNotFound`, silently skip; on any other error, return `fmt.Errorf("run claude launch service: lookup codex binding for project %q: %w", ...)`.
  - `PrepareRuntime` call updated to pass `OtherProviderProfileHome: otherProfileHome` (and aligned multi-value struct literal with gofumpt-style tabs).

- **`fakeStore` extension (`service_test.go`):**
  - Added fields: `crossBinding domain.ProjectBinding`, `crossBindingErr error`, `crossProfile domain.Profile`, `crossProfileErr error`.
  - `BindingByProjectID` now dispatches on `provider`: returns `f.crossBinding, f.crossBindingErr` when `provider == domain.ProviderCodex`; returns primary `f.binding, f.bindingErr` otherwise.
  - `ProfileByID` now dispatches on `id`: returns `f.crossProfile, f.crossProfileErr` when `f.crossProfile.ID != "" && id == f.crossProfile.ID`; returns primary `f.profile, f.profileErr` otherwise.
  - `boundClaudeStore` helper updated to set `crossBindingErr: domain.ErrNotFound` — all existing tests using this helper continue to pass with no cross-mount side effects (opt-out by default).

- **Test additions:**
  - `TestRunCrossProviderMountWhenCodexBound` — table-driven, 3 rows:
    - `"codex bound"`: cross binding + profile resolve successfully → `PreparedRuntime` contains a mount with `Target == "/home/valv/.codex"` sourced from `codexProfileHome` AND `Env["CODEX_HOME"] == "/home/valv/.codex"`.
    - `"codex not bound (ErrNotFound)"`: `crossBindingErr = domain.ErrNotFound` → Run succeeds, no `/home/valv/.codex` mount, no `CODEX_HOME` env var.
    - `"codex store error"`: `crossBindingErr = errors.New("store unavailable")` → Run returns non-nil error.

- **TDD cycle:**
  - RED: tests written first; ran `mage testPkg` → 3 failures (correct reasons — no cross-lookup in service.go yet).
  - GREEN: cross-lookup + `OtherProviderProfileHome` pass added to `service.go`; ran `mage testPkg` → 23/23 pass, 80.9% coverage.

- **Mage gate results:**
  - `mage testPkg github.com/evanmschultz/valv/internal/services/claude`: 23 tests passed, 0 failed, coverage 80.9% (floor 60%). GREEN.
  - `mage build`: `./valv` produced cleanly. GREEN.

## Unit 10.5 — Round 1

- **Builder:** go-builder-agent
- **Started:** 2026-05-19
- **Files touched:**
  - `internal/services/codex/service.go`
  - `internal/services/codex/service_test.go`
  - `drops/DROP_10_CROSS_PROVIDER_CONTAINER/PLAN.md` (state flip)
  - `drops/DROP_10_CROSS_PROVIDER_CONTAINER/BUILDER_WORKLOG.md` (this file)

- **Production changes (`service.go`):**
  - Added cross-binding lookup block in `Service.Run`, inserted between the `resolved` convergence point (end of both `overrideProfile` and `else` branches, line 153) and the `sharedHome` + `PrepareRuntime` call (original line 155). Block: call `s.store.BindingByProjectID(ctx, resolved.project.ID, domain.ProviderClaude)`; on success, call `s.store.ProfileByID(ctx, otherBinding.ProfileID)` and set `otherProfileHome`; on `domain.ErrNotFound`, silently skip; on any other error, return `fmt.Errorf("run codex launch service: lookup claude binding for project %q: %w", ...)`.
  - `PrepareRuntime` call updated to pass `OtherProviderProfileHome: otherProfileHome` (aligned multi-value struct literal with gofumpt-style tabs).

- **`fakeStore` extension (`service_test.go`):**
  - Removed tracking fields `projectRoot` and `profileID` (were never read in assertions; removed to avoid unused-field writes).
  - Added fields: `crossBinding domain.ProjectBinding`, `crossBindingErr error`, `crossProfile domain.Profile`, `crossProfileErr error`.
  - `BindingByProjectID` now dispatches on `provider`: returns `f.crossBinding, f.crossBindingErr` when `provider == domain.ProviderClaude`; returns primary `f.binding, f.bindingErr` otherwise.
  - `ProfileByID` now dispatches on `id`: returns `f.crossProfile, f.crossProfileErr` when `f.crossProfile.ID != "" && id == f.crossProfile.ID`; returns primary `f.profile, f.profileErr` otherwise.
  - Added `boundCodexStore` helper: pre-wires project + codex binding + profile with `crossBindingErr: domain.ErrNotFound` (opt-out default, mirrors `boundClaudeStore` pattern from claude package).
  - Added `detectAlways` helper: returns a `DetectFunc` that always resolves to the given root with `HasGitMarker: true`.

- **Opt-out defaults applied to all existing inline `fakeStore` Run-path tests:**
  - `TestRunReturnsUnboundProjectWhenProjectMissing` — added `crossBindingErr: domain.ErrNotFound`.
  - `TestRunBuildsDockerRequestFromProjectBindingAndProfile` — added `crossBindingErr: domain.ErrNotFound`.
  - `TestRunRejectsOverrideProfileWithWrongProvider` — added `crossBindingErr: domain.ErrNotFound`.
  - `TestRunBubblesExecutorErrors` — added `crossBindingErr: domain.ErrNotFound`.
  - `TestRunBuildsNonInteractiveDockerRequestWhenTTYDisabled` — added `crossBindingErr: domain.ErrNotFound`.
  - `TestRunRejectsWorkingDirectoryOutsideProjectRoot` — added `crossBindingErr: domain.ErrNotFound`.
  - `TestRunUsesSharedHostHomeForCodexStateWhenRealHomeIsSet` — added `crossBindingErr: domain.ErrNotFound`.
  - `TestRunRejectsSiblingPathThatSharesProjectPrefix` — added `crossBindingErr: domain.ErrNotFound`.
  - `TestRunUsesOverrideProfileHomePath` — added `crossBindingErr: domain.ErrNotFound`.

- **Test additions:**
  - `TestRunCrossProviderMountWhenClaudeBound` — table-driven, 3 rows:
    - `"claude bound"`: cross binding + profile resolve → mount with `Target == "/home/valv/.claude"` sourced from `claudeProfileHome` (EvalSymlinks-normalized) AND `Env["CLAUDE_CONFIG_DIR"] == "/home/valv/.claude"`.
    - `"claude not bound (ErrNotFound)"`: `crossBindingErr = domain.ErrNotFound` → Run succeeds, no `/home/valv/.claude` mount, no `CLAUDE_CONFIG_DIR` env var.
    - `"claude store error"`: `crossBindingErr = errors.New("store unavailable")` → Run returns non-nil error.
  - macOS symlink fix: `claudeProfileHome` obtained via `filepath.EvalSymlinks(t.TempDir())` to match `pathutil.Normalize`'s resolved path used in mount source.

- **TDD cycle:**
  - RED: tests written first; ran `mage testPkg` → 3 failures (correct reasons — no cross-lookup in service.go yet).
  - GREEN (attempt 1): production change landed; `mage testPkg` → 2 failures (macOS `/var` vs `/private/var` symlink mismatch in mount source comparison).
  - GREEN (attempt 2): applied `filepath.EvalSymlinks` to `claudeProfileHome`; `mage testPkg` → 17/17 pass, 76.0% coverage.

- **Mage gate results:**
  - `mage testPkg github.com/evanmschultz/valv/internal/services/codex`: 17 tests passed, 0 failed, coverage 76.0% (floor 60%). GREEN.
  - `mage build`: `./valv` produced cleanly. GREEN.

## Hylla Feedback (Unit 10.4)

N/A — task touched files changed since last ingest (`service.go` and `service_test.go` are new edits in DROP_10; runtime files modified in 10.2–10.3 are also stale in Hylla). All evidence gathered via `Read` directly. No Hylla queries issued for Go symbol lookups. Domain constants (`ProviderCodex`, `ErrNotFound`) confirmed via `Read` of `internal/domain/types.go` and `internal/domain/errors.go`.

## Hylla Feedback (Unit 10.5)

N/A — task touched files changed since last ingest (both `codex/service.go` and `codex/service_test.go` are new DROP_10 edits; the codex runtime `PrepareRequest.OtherProviderProfileHome` field landed in 10.3 which is also stale in Hylla). All evidence gathered via `Read` directly — codex service, claude service (for mirror pattern), codex runtime, claude service test (for fakeStore extension pattern). No Hylla queries issued.
