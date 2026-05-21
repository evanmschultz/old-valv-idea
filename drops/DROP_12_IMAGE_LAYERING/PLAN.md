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

Per-project layered Docker image build. Extend `internal/services/images` to compose `valv-<provider>:<project-hash>` from the base `valv-<provider>:dev` image plus a per-project overlay layer that installs the tools declared in the project's `.valv/tools.toml` (DROP_11). Cache the layered image by a tools-hash, mirroring the existing recipe-hash pattern used for base-image rebuild detection.

**This drop is the dogfood unblock.** Once it closes, `ta` CLI and similar per-project Go tools become installable inside Valv containers, which is the gap that surfaced 2026-05-20 when the dev tried to use `valv claude` against the `ta` worktree and the container had no `ta` binary.

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

## Schema decisions for the planner to nail down

These are the load-bearing design calls. The planner OWNs them; orch will route only the architectural ones to dev.

1. **Per-project image tag format.** `valv-claude:<project-hash>` vs `valv-claude:<project-name>-<tools-hash>` vs other. Survey claudebox: `claudebox-<project-name>`. Recommend `valv-<provider>:<short-tools-hash>` (deterministic, content-addressed; project name is for operator display, hash for cache key).
2. **What's "tools-hash"?** A stable hash over the parsed manifest content — order-independent for `Tools map`, includes versions and source/install strings. Use `crypto/sha256` against a canonical JSON or TOML re-encode of the manifest. Two projects with identical tool declarations share the same overlay image.
3. **Cache invalidation chain.** Three sources can trigger a rebuild: (a) base image rebuilt (existing recipe-hash logic), (b) tools.toml content changed (new tools-hash), (c) Dockerfile overlay template changed (separate "overlay-recipe-hash"). All three should compose into the per-project image's identity.
4. **Overlay Dockerfile template.** Single canonical template `Dockerfile.overlay` (or generated dynamically per manifest) that takes a list of tools and emits the install commands. For string-form tools (`mage = "latest"`): assume `go install`-style or pre-known registry; for object-form (`{ source, install }`): execute the `install` command verbatim against `source`. Network policy (closed-by-default outbound) is DROP_15's concern — DROP_12 assumes open network during build.
5. **What install commands does DROP_12 support?** Minimum viable: `go install <source>`, `npm install -g <source>`, `pip install <source>`. Object-form tools specify their own `install` command. String-form tools need a registry lookup — recommend deferring registry resolution to a later drop and supporting only string-form for **specific well-known names** (e.g. `mage`, `gh`, `go`) where DROP_12 hardcodes the install command, OR requiring all tools in DROP_12 to use object-form (verbose but unambiguous). Planner picks; orch routes to dev if ambiguous.
6. **Where does the per-project image build trigger?** Options: (a) lazily at `valv claude` / `valv codex` launch, mirroring the existing `EnsureLatest` recipe-hash check for the base image; (b) explicit `valv image build` command. Recommend (a) — auto-build-on-launch matches the existing UX where users don't think about images.
7. **Cleanup.** When does a per-project image get garbage collected? When the project is deleted? When tools-hash changes (orphan old images)? Recommend (b) — bump tools-hash means a NEW image; the old one becomes orphaned and is candidate for `valv image cleanup`.
8. **`valv claude` / `valv codex` launch path integration.** The launcher needs to use the per-project image (`valv-claude:<tools-hash>`) when one exists, falling back to base (`valv-claude:dev`) when `.valv/tools.toml` is absent. The fallback ensures projects without a tools.toml continue to work unchanged.

## Planner

### Objective

Extend `internal/services/images` to compose a per-project layered Docker image (`valv-<provider>:<short-tools-hash>`) on top of the base `valv-<provider>:dev` image. The overlay layer runs the install commands implied by `.valv/tools.toml` (DROP_11). The CLI launch path (`internal/cli/claude.go`, `internal/cli/codex.go`) selects the per-project tag when a manifest exists, falls back to base when absent. Cache by tools-hash + base-image-hash + overlay-recipe-hash, mirroring the existing `recipeHashLabel` pattern.

### Schema Decisions (locked by planner)

1. **Per-project image tag format.** `valv-<provider>:proj-<short-tools-hash>` where `short-tools-hash` is `sha256(canonical-manifest)[:12]` rendered as lowercase hex. The `proj-` prefix disambiguates from the base `:dev` tag and the existing `:<version>` semver tags (`service.go:580-582`). Tag charset stays within Docker reference grammar (`[a-z0-9._-]`, length ≤ 128). Example: `valv-claude:proj-9c3a7b1e8d4f`.

2. **Tools-hash canonical form.** `sha256(json.MarshalIndent(canonical, "", ""))` where `canonical` is a sorted-keys representation: `{"tools": [{"name": <sorted>, "version": <v>, "source": <s>, "install": <i>}, ...]}`. JSON not TOML — Go's `encoding/json` provides deterministic output when the input map is reduced to a sorted slice. `[allowlist]` and `[env]` sections are deliberately EXCLUDED from the tools-hash because DROP_14/DROP_15 own those — including them now would force unnecessary image rebuilds when those sections land. The hash is hex-encoded; the first 12 hex chars are the short form used in the tag.

3. **Cache invalidation chain.** Three labels persisted on the per-project image:
   - `io.valv.recipe_hash` — existing label, sha256 of overlay Dockerfile template content (forces rebuild if the planner-generated overlay template changes).
   - `io.valv.base_image_hash` — sha256 of the base image's `io.valv.recipe_hash` label value, captured at build-time via `docker image inspect` of the resolved base. Forces rebuild when the base image is rebuilt.
   - `io.valv.tools_hash` — full (not truncated) sha256 of the canonical manifest from decision 2. Forces rebuild when manifest content changes.
   On launch, `EnsureProjectImage` reads all three labels from the existing per-project image; any mismatch triggers a rebuild.

4. **Overlay Dockerfile shape.** Dynamic generation per-project, not a static template file. Rationale: the overlay command set depends on per-tool install strategy; a static template forcing every project through identical RUN layers would mask which tools changed when the hash changes. Generator function `BuildOverlayDockerfile(manifest tools.ToolManifest, baseImage docker.ImageRef) (string, error)` lives in `internal/services/images/overlay.go` (new). Produces:

   ```
   FROM valv-<provider>:dev

   USER root
   ENV NPM_CONFIG_UPDATE_NOTIFIER=false \
       NPM_CONFIG_FUND=false \
       NPM_CONFIG_AUDIT=false

   # One RUN per tool, sorted by name for deterministic layering.
   RUN <install command for tool A>
   RUN <install command for tool B>

   USER valv
   ```

   The `USER root → USER valv` bracket is required because `DefaultCodexDockerfile` / `DefaultClaudeDockerfile` end with `USER valv` (service.go:674, 738). Tool installs that write to `/usr/local/bin` (npm global, go install with `GOBIN=/usr/local/bin`) need root.

5. **Install command vocabulary (v1, RECOMMENDED — dev call needed, U1).**
   - **Object-form tools** (`{ source, install }`): execute `install` verbatim against `source`. v1 supports two install strategies:
     - `install = "go install"` → emits `RUN GOBIN=/usr/local/bin go install <source>` (requires `go` available in base; **NOT in v1 base image** — see U1).
     - `install = "npm install -g"` → emits `RUN npm install -g <source>` (npm is in base, both Dockerfiles install via npm).
   - **String-form tools** (`name = "version"`): supported ONLY for hardcoded well-known names in v1. Initial well-known set: **none** — string-form is REJECTED at overlay-generation time in v1 with a clear error pointing the user at the object form. Rationale: registry resolution for arbitrary names (`mage = "latest"`, `gh = "latest"`) requires per-tool install strategies and version semantics that aren't pinned. Deferring to a future drop keeps DROP_12 minimal.

6. **Build trigger timing (RECOMMENDED — dev call needed, U2).** Auto-build on `valv claude` / `valv codex` launch, mirroring the existing `EnsureLatest` behavior at `ensureClaudeImageCurrent`/`ensureCodexImageCurrent` (`claude.go:189-206`, `codex.go:227-244`). Adds a new function `ensureProjectImage(cmd, paths, baseImage docker.ImageRef, manifest tools.ToolManifest) (docker.ImageRef, error)` that runs AFTER `ensureClaude/CodexImageCurrent` in both launchers. Falls back to base ref when manifest is empty. Recommend NOT adding an explicit `valv image build` command in this drop — overlay cleanup will go through `valv image cleanup` (already in CLI) once orphan detection lands (decision 7).

7. **Cleanup of stale per-project images (RECOMMENDED — dev call needed, U3).** Bump-tools-hash means a NEW image; the old one becomes orphaned. Mark per-project images with label `io.valv.scope=project-overlay` so existing `valv image cleanup` can target them. Garbage-collection policy is deferred (a future drop adds "delete `project-overlay`-scoped images not used by any binding" semantics). v1 leaves orphans on disk — they're harmless beyond disk usage.

8. **Fallback when `.valv/tools.toml` absent.** When `tools.Resolve(projectDir)` returns `len(manifest.Tools) == 0` (file absent OR file present but no `[tools]` entries), the launcher uses base `valv-<provider>:dev` unchanged. No overlay build is attempted. This preserves the current behavior for every existing project.

### Acceptance Criteria (drop-level)

1. `internal/services/images/` compiles with zero vet warnings after `mage testPkg ./internal/services/images/`.
2. New helper `OverlayHash(manifest tools.ToolManifest) string` returns a deterministic sha256 hex string for the canonical manifest; two manifests with semantically identical `[tools]` content (different declaration order, whitespace variations in source paths after `strings.TrimSpace`) produce identical hashes.
3. New helper `BuildOverlayDockerfile(manifest, baseImage) (string, error)` produces a Dockerfile whose:
   - `FROM` line references `baseImage.String()`.
   - Contains one `RUN` per tool, sorted by tool name.
   - Returns a wrapped error when manifest contains a string-form tool (v1 reject — see decision 5 / U1).
   - Returns a wrapped error when manifest contains an object-form tool whose `install` is outside the v1-supported set (`go install`, `npm install -g`).
4. New `Service.EnsureProjectImage(ctx, request EnsureProjectRequest)` method:
   - When `request.Manifest.Tools` is empty: returns `EnsureProjectResult{Image: baseRef, Action: EnsureActionUsingExistingImage}` without touching docker.
   - When manifest has tools and per-project image doesn't exist: builds via `docker buildx build --load`, applies `recipe_hash + base_image_hash + tools_hash + scope=project-overlay` labels, returns built ref with `Action: EnsureActionUpdated`.
   - When manifest has tools and per-project image exists with matching all-three labels: returns existing ref with `Action: EnsureActionUpToDate` without rebuilding.
   - When manifest has tools and per-project image exists but ANY of the three labels mismatch: rebuilds.
5. CLI launch path: `runClaudeCommand` and `runCodexCommand` resolve a per-project image via `ensureProjectImage` AFTER `ensureClaude/CodexImageCurrent` succeeds. The resolved per-project ref is passed into `claudeservice.New(Options{Image: projectImage})` / `codexservice.New(...)` instead of the base ref. `runClaudeImageOnlyCommand` / `runCodexImageOnlyCommand` (help/version paths) keep using base ref unchanged.
6. `mage testPkg ./internal/services/images/` reports ≥ 70% coverage including the new overlay code.
7. `mage testPkg ./internal/cli/` does not regress below its existing 67.6% floor.
8. `mage test` passes clean across all packages after all units done (at the current 60% gate; the 70% gate bump is deferred to DROP_17).
9. Integration test (or table-driven service-level test with mocked Runner) exercises the three-label cache match/mismatch matrix: all-match → no rebuild, tools-hash-mismatch → rebuild, base-image-hash-mismatch → rebuild, recipe-hash-mismatch → rebuild.

### Units

---

#### Unit 12.1 — Overlay hash + canonical manifest

**State:** todo

**Paths:**
- `internal/services/images/overlay.go` (new — hashing helpers + types)
- `internal/services/images/overlay_test.go` (new)

**Packages:** `internal/services/images/`

**Acceptance:**
- Add helpers `OverlayHash(manifest tools.ToolManifest) string` and `canonicalManifest(manifest)` (unexported) that emit deterministic JSON over a sorted-by-name slice of `{name, version, source, install}` records. `OverlayHash` returns full lowercase hex sha256.
- Add helper `ShortOverlayHash(manifest) string` returning the first 12 hex chars (used in the tag).
- Table-driven test exercises: empty manifest, single string-form tool, single object-form tool, three tools declared out of order (hash matches re-ordered input), whitespace in source/install (`strings.TrimSpace` applied before hashing).
- Test exercises a hash-stability snapshot: hardcoded manifest → hardcoded expected hex. Pins the canonical form against accidental future drift.
- No changes to `service.go`, no overlay-Dockerfile generation in this unit.
- `mage testPkg ./internal/services/images/` green.

**Blocked by:** —

---

#### Unit 12.2 — Overlay Dockerfile generator

**State:** todo

**Paths:**
- `internal/services/images/overlay.go` (extend — add `BuildOverlayDockerfile`)
- `internal/services/images/overlay_test.go` (extend)

**Packages:** `internal/services/images/`

**Acceptance:**
- Add `BuildOverlayDockerfile(manifest tools.ToolManifest, baseImage docker.ImageRef) (string, error)`.
- Output starts with `FROM <baseImage.String()>`, includes the `USER root → ENV NPM_CONFIG_* → RUN per tool (sorted by name) → USER valv` skeleton from decision 4.
- String-form tools (Version set, Source/Install empty) cause the function to return a wrapped error (`"build overlay dockerfile: tool %q uses unsupported string-form spec (v1 requires object form with source+install)"`).
- Object-form tools with `install = "go install"` emit `RUN GOBIN=/usr/local/bin go install <source>`.
- Object-form tools with `install = "npm install -g"` emit `RUN npm install -g <source>`.
- Object-form tools with any other `install` value return a wrapped error.
- Tests assert: exact byte-for-byte Dockerfile output for a sample manifest with two object-form tools; error path for string-form; error path for unsupported install verb; sorting determinism across two same-manifest-different-order inputs.
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
- New constants: `tagPrefixProjectOverlay = "proj-"`, `toolsHashLabel = "io.valv.tools_hash"`, `baseImageHashLabel = "io.valv.base_image_hash"`, `scopeLabelProjectOverlay = "project-overlay"`.
- New request/result types `EnsureProjectRequest{Manifest tools.ToolManifest; BaseImage docker.ImageRef; Pull, NoCache bool}` and `EnsureProjectResult{Image docker.ImageRef; Action EnsureAction; ToolsHash string; BaseImageHash string}`.
- New `Service.EnsureProjectImage(ctx, request)` method that:
  1. Returns `{Image: request.BaseImage, Action: EnsureActionUsingExistingImage}` when `len(request.Manifest.Tools) == 0`.
  2. Computes `toolsHash := OverlayHash(request.Manifest)` and `baseImageHash := s.inspectLabel(ctx, request.BaseImage, recipeHashLabel)`.
  3. Constructs target tag via `s.projectImageRef(toolsHash)` returning `<repo>:proj-<short-hash>`.
  4. Inspects target tag; reads all three labels; rebuilds when any mismatch.
  5. Rebuild path: generates overlay via `BuildOverlayDockerfile`, writes to ephemeral `os.MkdirTemp` (cleanup deferred), calls existing `docker.BuildImageArgs` flow with the four labels (`recipe_hash`, `base_image_hash`, `tools_hash`, `scope=project-overlay`).
- New unexported helper `s.inspectLabel(ctx, ref, label string) (string, error)` reuses the existing `outputRunner` typecast pattern from `imageRecipeMatches`.
- Table-driven test against `runnerRecorder` fake covers the three-label cache matrix.
- `mage testPkg ./internal/services/images/` green; package coverage ≥ 70%.

**Blocked by:** Unit 12.2

---

#### Unit 12.4 — CLI launch wiring (claude + codex)

**State:** todo

**Paths:**
- `internal/cli/claude.go` (extend — add `ensureClaudeProjectImage`; call it in `runClaudeCommand` after `ensureClaudeImageCurrent`)
- `internal/cli/codex.go` (extend — add `ensureCodexProjectImage`; call it in `runCodexCommand`)
- `internal/cli/operator_helpers.go` (extend — small helper `resolveProjectImage(cmd, paths, provider, workingDir) (docker.ImageRef, error)`)
- `internal/cli/claude_project_image_test.go` (new)
- `internal/cli/codex_project_image_test.go` (new)

**Packages:** `internal/cli/`

**Acceptance:**
- `resolveProjectImage`:
  - calls `tools.Resolve(workingDir)`; on `len(manifest.Tools) == 0` returns base ref + nil error.
  - otherwise opens images service for `provider`, calls `EnsureProjectImage`, returns the result's `Image`.
  - any `tools.Resolve` error other than empty-manifest is wrapped and returned.
- `runClaudeCommand` calls `resolveProjectImage` AFTER `ensureClaudeImageCurrent`, passes the resolved ref into `claudeservice.New(Options{Image: projectImage, ...})`.
- Mirror for `runCodexCommand`.
- `runClaudeImageOnlyCommand` and `runCodexImageOnlyCommand` UNCHANGED — they keep using base ref since they don't need project context.
- `VALV_CLAUDE_IMAGE` / `VALV_CODEX_IMAGE` override behavior preserved: when env var set, overlay resolution is skipped.
- Tests: stub `tools.Resolve` via testdata dir, install `installFakeDocker(t)` fixture, exercise: empty manifest → base ref used; manifest with one tool → overlay-build invocation in fake docker calls; `VALV_CLAUDE_IMAGE` set → overlay skipped.
- `mage testPkg ./internal/cli/` green; coverage does not regress below 67.6%.

**Blocked by:** Unit 12.3

---

#### Unit 12.5 — Drop-end integration verification

**State:** todo

**Paths:**
- no source changes — verification unit
- optional: extend `internal/cli/codex_integration_test.go` if needed (assess at unit start; don't create files speculatively)

**Packages:** `internal/cli/`, `internal/services/images/`

**Acceptance:**
- `mage test` clean from `main/`.
- `mage integration` clean (Docker-backed).
- `mage golden` clean — integration suite covers external transcript goldens which could regress if per-project image insertion changes container args ordering.
- `mage build` produces working binary; smoke-run `./valv image --help` returns without error.
- QA verdict captures: per-project image tag observed in `docker image ls` output during a smoke build of a fixture `.valv/tools.toml`; base image still works for tools-toml-less projects.

**Blocked by:** Unit 12.4

---

### Notes For Builder Agents

- **Hylla pin caveat:** `internal/tools/{tools,resolve,validate}.go` is committed but Hylla artifact `github.com/evanmschultz/valv@main` may be pinned pre-DROP_11-merge if running before reingest. Direct `Read` is the authoritative evidence source until next drop-close reingest.
- **`recipeHash` interaction:** Unit 12.3's `base_image_hash` label captures the BASE image's `io.valv.recipe_hash` value at overlay-build time. Inspecting the base via `docker image inspect --format '{{ index .Config.Labels "io.valv.recipe_hash" }}'` reuses the existing `outputRunner` typecast pattern at `service.go:597-610`.
- **`ImageRef` parsing constraint:** Tags must satisfy Docker's reference grammar (`[a-z0-9._-]`, length ≤ 128). The `proj-` prefix + 12 hex chars produces 17-char tags — well within bounds.
- **`docker buildx build --load` is canonical:** `images.Service.Build` already uses it; `BuildImageArgs` at `ops.go:38-103` is the call surface.
- **Test fakes available:** `runnerRecorder` in `service_test.go:22-52` records calls and returns canned outputs by joined-arg-string key. Use that for the three-label cache matrix in Unit 12.3.
- **CLI fixture pattern:** `installFakeDocker(t)` (cited in `manage_test.go:594-600`) installs a fake docker binary that exits 0 — use for Unit 12.4 CLI tests.
- **`go install` placement constraint (decision 5):** `go install` needs `go` available in the base image. Current `DefaultCodexDockerfile` / `DefaultClaudeDockerfile` (`service.go:645-742`) install `bubblewrap ca-certificates git ncurses-term` — no Go toolchain. **If dev approves `go install` shortcut in U1, the base Dockerfile must add `golang-go` to apt** OR the overlay's `RUN` must install Go itself before `go install <tool>`. **This is U1 below — needs dev decision.**
- **Symbol-deletion / interface-change mage-integration rule:** No interface changes in DROP_12 (the `Service` struct gains a new method but no existing interface changes). Unit 12.5's `mage integration` step catches hidden compile-breaks in `//go:build integration` files.
- **Per-unit ordering:** Strict linear (12.1 → 12.2 → 12.3 → 12.4 → 12.5). No parallel-eligible units.

## Notes

- DROP_11 establishes the schema + parser foundation; DROP_12 consumes it.
- Network policy enforcement (closed-by-default + allowlist) is DROP_15's concern. DROP_12 assumes Docker build can reach the open internet to install tools.
- `valv run --account X <cmd>` generic primitive is DROP_13's concern. DROP_12's launcher changes are scoped to existing `valv claude` / `valv codex` providers.
- Per-account env var injection is DROP_14's concern. DROP_12 doesn't thread custom env into the build or runtime.
- The 70% per-package coverage floor still applies (and is met at 60% gate per the deferred-from-DROP_11 Unit 11.5). New packages added in DROP_12 should ship at ≥70% local coverage from day one.
