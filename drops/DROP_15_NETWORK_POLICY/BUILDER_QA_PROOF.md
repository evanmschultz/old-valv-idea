# DROP_15 — Builder QA Proof

Append a `## Unit 15.M — Round K` section per QA Proof review. See `main/drops/WORKFLOW.md` § "Phase 5 — Build QA (per unit)".

## Unit 15.0 — Round 1

**Date:** 2026-05-24
**QA Proof backend:** claude-native (orchestrator dispatch, opus)
**Verdict:** `pass`

### Per-Acceptance Audit

**A1. `resolveProjectImage` resolves the project root via `project.DetectFrom(workingDir)` before `tools.Resolve`; raw `tools.Resolve(workingDir)` is removed.**

- Verified at `internal/cli/operator_helpers.go:448-457`:
  ```
  func resolveProjectImage(cmd *cobra.Command, paths config.Paths, provider domain.Provider, workingDir string, baseRef dockeradapter.ImageRef) (dockeradapter.ImageRef, error) {
      detected, err := project.DetectFrom(workingDir)
      if err != nil {
          return dockeradapter.ImageRef{}, fmt.Errorf("resolve project image: %w", err)
      }

      manifest, err := tools.Resolve(detected.Root)
  ```
- `project` import added at `internal/cli/operator_helpers.go:20`.
- Removal verified by ripgrep: `rg "tools.Resolve\(workingDir\)" internal/cli/` returns 0 matches. The only remaining `tools.Resolve(...)` call site in the file is the new `tools.Resolve(detected.Root)` at line 454.
- API signature confirmed: `project.DetectFrom(start string) (Result, error)` at `internal/project/project.go:28`, with `Result.Root` field at `internal/project/project.go:13`. Fallback semantics (no `.git` marker → returns start path) at `internal/project/project.go:49-55`.

**A2. Empty-manifest short-circuit and overlay-build path both consult `.valv/tools.toml` at the detected project root.**

- Code path: `tools.Resolve(detected.Root)` → if `len(manifest.Tools) == 0` short-circuits at `internal/cli/operator_helpers.go:458-460`; otherwise the override branch and `EnsureProjectImage` branch consume the same `manifest`.
- Verified by `TestResolveProjectImageClaudeNestedSubdirFindsRootManifest` at `internal/cli/claude_project_image_test.go:234-267`:
  - Fixture: `writeClaudeNestedToolsManifest` creates `<root>/.git/`, `<root>/.valv/tools.toml`, and nested `<root>/pkg/sub/` (lines 184-208).
  - Asserts `got.String() != baseRef.String()` (line 253), `proj-` tag prefix (line 256), and `buildx build --load` recorded (line 264).
- Mirror at `TestResolveProjectImageCodexNestedSubdirFindsRootManifest` at `internal/cli/codex_project_image_test.go:213-246`.
- Absent-manifest-at-root case verified by `TestResolveProjectImageClaudeNestedSubdirNoManifestReturnsBase` at `internal/cli/claude_project_image_test.go:274-301`: `writeClaudeNestedNoManifest` creates `<root>/.git/` and `<root>/pkg/sub/` but no `.valv/tools.toml` (lines 215-226); asserts `got.String() == baseRef.String()` (line 292), zero stderr (line 295), and zero docker calls (lines 298-300). Mirror at `internal/cli/codex_project_image_test.go:251-278`.

**A3. Override semantics unchanged: `VALV_CLAUDE_IMAGE` / `VALV_CODEX_IMAGE` still emit one warning and skip overlay AFTER manifest resolution from the detected root.**

- Code: override check at `internal/cli/operator_helpers.go:462-467` runs AFTER `tools.Resolve(detected.Root)` (line 454) and AFTER the empty-manifest short-circuit (line 458). One `fmt.Fprintln` warning + early `return baseRef, nil`.
- Verified by `TestResolveProjectImageClaudeNestedSubdirOverrideAfterRootResolve` at `internal/cli/claude_project_image_test.go:310-343`:
  - `t.Setenv("VALV_CLAUDE_IMAGE", "test/claude:override")` (line 311) + nested subdir fixture (line 313).
  - Asserts `got.String() == baseRef.String()` (line 329), the exact warning string (lines 333-336), `strings.Count(stderr, wantWarning) == 1` (line 337-339), and zero docker calls (lines 340-342). All four QA-spec checks present.
- Mirror at `TestResolveProjectImageCodexNestedSubdirOverrideAfterRootResolve` at `internal/cli/codex_project_image_test.go:284-317` with identical `strings.Count == 1` assertion at line 311-313.
- Pre-existing override tests (non-nested) also retained: `TestResolveProjectImageClaudeOverrideSkipsOverlay` at lines 118-154 and `TestResolveProjectImageCodexOverrideSkipsOverlay` at codex test lines 103-137, both with `strings.Count == 1` assertions.

**A4. Repo-subdirectory coverage added to both `claude_project_image_test.go` and `codex_project_image_test.go`.**

- Three new tests + two new fixture helpers per file:
  - Claude: `TestResolveProjectImageClaudeNestedSubdirFindsRootManifest` (line 234), `TestResolveProjectImageClaudeNestedSubdirNoManifestReturnsBase` (line 274), `TestResolveProjectImageClaudeNestedSubdirOverrideAfterRootResolve` (line 310); helpers `writeClaudeNestedToolsManifest` (line 184) and `writeClaudeNestedNoManifest` (line 215).
  - Codex: `TestResolveProjectImageCodexNestedSubdirFindsRootManifest` (line 213), `TestResolveProjectImageCodexNestedSubdirNoManifestReturnsBase` (line 251), `TestResolveProjectImageCodexNestedSubdirOverrideAfterRootResolve` (line 284); helpers `writeCodexNestedToolsManifest` (line 166) and `writeCodexNestedNoManifest` (line 196).
- Fixture pattern confirmed: each helper creates `.git/` dir at projectRoot so `project.DetectFrom` climbs back, then invokes from `<projectRoot>/pkg/sub`. Matches the documented `project.DetectFrom` behavior verified in `internal/project/project_test.go:11-30`.

### Mage Results (run by QA Proof)

```
mage testPkg ./internal/cli
[PKG PASS] github.com/evanmschultz/valv/internal/cli (6.79s)
  tests: 240
  passed: 240
  failed: 0
  package coverage: 67.6% (above 60% gate)
```

Matches builder's claim of 240 tests / 67.6% coverage exactly.

### Scope Compliance

`git diff-tree --no-commit-id --name-only -r 01ba002` returns exactly:

- `drops/DROP_15_NETWORK_POLICY/BUILDER_WORKLOG.md`
- `drops/DROP_15_NETWORK_POLICY/PLAN.md`
- `internal/cli/claude_project_image_test.go`
- `internal/cli/codex_project_image_test.go`
- `internal/cli/operator_helpers.go`

No edits to `internal/tools/`, `internal/adapters/docker/`, `internal/services/networkpolicy/`, `internal/services/images/`, or `internal/services/run/`. Hard-constraint compliance holds.

### Findings

- F1. `project.DetectFrom` is the correct API surface for Unit 15.0: it takes an explicit start path (vs `Detect()` which uses `os.Getwd()`), is goroutine-safe, walks up to nearest `.git` marker, and falls back to the start path when no marker is found. Confirmed by `internal/project/project.go:28-58` and existing test coverage at `internal/project/project_test.go:11-165` (5 cases including nested subdir, linked worktree, no-marker fallback, file path, symlinked root).
- F2. The two pre-existing empty-manifest tests (`TestResolveProjectImageClaudeEmptyManifestReturnsBaseRef`, `TestResolveProjectImageCodexEmptyManifestReturnsBaseRef`) continue to pass because `project.DetectFrom` on a bare `t.TempDir()` (no `.git`) falls back to returning the start path itself, preserving the original semantic.
- F3. TDD red-then-green claim is plausible but not independently re-verifiable post-commit: the change is squashed into commit `01ba002`. Builder worklog at `BUILDER_WORKLOG.md:66` documents the red signal explicitly (4 of 6 new tests failed with the expected "manifest at root should have triggered overlay" + "missing override warning" diagnostics) — this is consistent with the diff (the nested-FindsRoot and nested-Override tests genuinely depend on the new `project.DetectFrom` behavior to pass).
- F4. The override warning assertion uses `strings.Count(stderr.String(), wantWarning) == 1` in all four override tests (`claude` lines 145-147 + 337-339, `codex` lines 130-132 + 311-313), satisfying the "exactly one warning" specification verbatim.
- F5. No control-flow reorder of the override path. The override check still runs AFTER manifest resolution but before `openImagesService`. This matches Acceptance Criterion 3 ("after the manifest has been resolved from the detected root") and the pre-existing DROP_12 Unit 12.4 semantics.

### Missing Evidence

None. All four acceptance criteria are satisfied with file:line citations, scope is clean, mage gate passes at the documented coverage level, and the test fixtures exercise the precise pre/post code path that Unit 15.0 changed.

### Summary

Verdict: **pass**.

Unit 15.0 cleanly re-roots `resolveProjectImage` from raw-cwd manifest resolution to detected-project-root manifest resolution via the existing `project.DetectFrom` API. The change is minimal (one new import + four-line function preamble), preserves override semantics structurally (no control-flow reorder), and is exhaustively covered by six new tests (three per provider) plus the four pre-existing tests left in place as regression guards. Hard-constraint compliance (no edits outside `internal/cli/`) holds. `mage testPkg ./internal/cli` reproduces 240 tests passing at 67.6% coverage.
