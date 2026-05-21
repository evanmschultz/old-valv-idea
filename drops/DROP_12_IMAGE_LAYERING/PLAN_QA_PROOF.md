# DROP_12 Plan QA Proof — Round 1

**Verdict:** pass
**Reviewer:** ta-go-qa-proof
**Reviewed at:** 2026-05-21T18:47:54Z

Plan passes proof review. The 5-unit decomposition is grounded in verified file:line evidence, all DROP_11 outputs exist as claimed, the cache-label scheme cleanly extends the existing `recipeHashLabel` pattern, and the tag format satisfies Docker reference grammar. Four findings below are clarifications and one technical refinement, not structural defects — they should be addressed in Phase 3 discuss/revise before building, but they do not block the unit ordering or scope.

## Findings

### F1 — `base_image_hash` double-hashes the base recipe value (technical refinement)

**Plan reference:** Schema Decision 3 (PLAN.md:62-66), Notes For Builder #2 (PLAN.md:249).

**Claim under review:** "`io.valv.base_image_hash` — sha256 of the base image's `io.valv.recipe_hash` label value, captured at build-time via `docker image inspect` of the resolved base."

**Evidence:** `internal/services/images/service.go:537-547` shows `recipeHash()` already returns `hex.EncodeToString(sha256.Sum256(content))` — a 64-char lowercase hex sha256. The base image's `io.valv.recipe_hash` label is already a deterministic content hash.

**Issue:** Hashing a sha256-hex with another sha256 yields no semantic benefit. Two bases with identical `recipe_hash` values produce identical `base_image_hash` values either way; two bases with different `recipe_hash` values produce different `base_image_hash` values either way. The double-hash adds CPU + a layer of indirection that complicates debugging (the per-project image's `base_image_hash` label cannot be eyeball-compared to the base's `recipe_hash` label).

**Recommendation:** Store the base's `recipe_hash` value verbatim as the per-project image's `io.valv.base_image_hash` label. `inspectLabel(ctx, base, "io.valv.recipe_hash")` returns the hex string directly; persist that hex string. Cache match check is then a direct string equality between the base's current `recipe_hash` and the persisted `base_image_hash`.

**Impact:** Affects U3's `EnsureProjectImage` implementation. Planner should patch decision 3 wording.

### F2 — `inspectLabel` typecast-failure semantics undefined for overlay rebuild policy

**Plan reference:** Schema Decision 3 + Unit 12.3 acceptance (PLAN.md:188).

**Claim under review:** "New unexported helper `s.inspectLabel(ctx, ref, label string) (string, error)` reuses the existing `outputRunner` typecast pattern from `imageRecipeMatches`."

**Evidence:** `internal/services/images/service.go:597-601` shows `imageRecipeMatches` does `runner, ok := s.runner.(outputRunner); if !ok { return true, nil }` — when the runner does NOT implement Output, the function returns "matches=true" (which signals "no rebuild needed"). This is a safe default for the existing flow because a non-output-capable runner cannot prove drift, so the conservative choice is to trust the existing image.

**Issue:** For overlay rebuild detection, the safe default is the OPPOSITE — if the runner cannot read labels, the conservative choice is to REBUILD (assume drift), not to trust an unverified overlay tag. Adopting the existing pattern verbatim would silently skip overlay rebuilds against runners that don't expose Output, leading to stale overlay images.

**Recommendation:** Add an explicit policy line to decision 3 (or U3 acceptance): "When the runner does not implement `outputRunner`, `EnsureProjectImage` must force rebuild." Implementation can either (a) have `inspectLabel` return a sentinel ("runner does not support Output") that `EnsureProjectImage` interprets as "rebuild" or (b) require `outputRunner` at construction time for the overlay path. Either choice is fine; the plan needs to pick one.

**Impact:** Affects U3's implementation. Planner should add the policy.

### F3 — `EnsureProjectRequest{Pull, NoCache}` semantics undefined for overlay path

**Plan reference:** Unit 12.3 acceptance (PLAN.md:181).

**Claim under review:** `EnsureProjectRequest{Manifest tools.ToolManifest; BaseImage docker.ImageRef; Pull, NoCache bool}`.

**Evidence:** `Pull` and `NoCache` on `EnsureRequest` (`service.go:111-114`) flow into `BuildRequest` and then into `docker.ImageBuildRequest`'s `Pull`/`NoCache` (`ops.go:9-21`). `Pull` maps to `docker build --pull` (pull base layers from registry).

**Issue:** For overlay builds, the FROM directive references `valv-<provider>:dev` — a LOCAL image, not a registry image. `--pull` against a locally-built image is a no-op at best, an error at worst (depending on docker version). The plan inherits the Pull/NoCache shape from EnsureRequest without addressing whether they apply.

**Recommendation:** Either (a) omit `Pull` from `EnsureProjectRequest` (overlay never pulls from registry) and keep only `NoCache`, or (b) explicitly document that `Pull` is forwarded verbatim and the caller is responsible for understanding it's typically a no-op. Recommend (a) for surface simplicity.

**Impact:** Affects U3's request-type shape. Planner should resolve.

### F4 — Hylla pin caveat in Notes For Builder Agents is obsolete

**Plan reference:** Notes For Builder Agents #1 (PLAN.md:248).

**Claim under review:** "`internal/tools/{tools,resolve,validate}.go` is committed but Hylla artifact `github.com/evanmschultz/valv@main` may be pinned pre-DROP_11-merge if running before reingest."

**Evidence:** Hylla search for `ToolManifest` returns `internal/tools/ToolManifest`, `internal/tools/Load`, `internal/tools/Resolve`, `internal/tools/Validate` from snapshot 7 with full docstrings — confirming Hylla IS reingested post-DROP_11.

**Issue:** The cautionary note is no longer accurate as of this review and may mislead builder agents into doing unnecessary `Read` fallbacks instead of Hylla queries (which Hylla discipline rules require to be primary for committed Go).

**Recommendation:** Drop or update note #1 to confirm Hylla coverage. Minor doc fix.

**Impact:** Documentation hygiene only — no implementation impact.

## Verification Summary

The following claims were positively verified:

- DROP_11 outputs at `internal/tools/{tools.go, resolve.go, validate.go}` with `ToolManifest`, `ToolSpec`, `Resolve`, `ToolsFilePath = ".valv/tools.toml"` matching the planner's described shape.
- `recipeHashLabel = "io.valv.recipe_hash"` at `service.go:30`. Existing label namespace (`io.valv.managed`, `io.valv.provider`, `io.valv.scope`, `io.valv.version`, `io.valv.recipe_hash` at `service.go:333-339`) does NOT conflict with the proposed new labels `io.valv.tools_hash`, `io.valv.base_image_hash`. Note: `io.valv.scope` is already used (value `"image"` for base images at `service.go:336`); the plan's `io.valv.scope=project-overlay` reuses the SAME label key with a different value — correct and intentional.
- `versionImageRef` at `service.go:580-582` produces the existing `:<dashed-version>` tag form. The proposed `:proj-<12-hex>` tag is disambiguated by the `proj-` prefix.
- `imageRecipeMatches` pattern at `service.go:597-610` is reusable as cited.
- `DefaultCodexDockerfile` (`service.go:645-678`) and `DefaultClaudeDockerfile` (`service.go:707-742`) install `bubblewrap ca-certificates git ncurses-term` plus `npm install -g` for both CLIs. `go` is NOT installed — confirming U1's "go install base-image gap" load-bearing premise.
- Both base Dockerfiles end with `USER valv` (lines 674 and 738), justifying the plan's `USER root → ... → USER valv` overlay bracket.
- `docker.BuildImageArgs` at `ops.go:38-103` accepts `ContextDir`, `Dockerfile`, `Tags`, `Builder`, `BuildArgs`, `Labels`, `Pull`, `NoCache` — the fields U3 requires.
- `runnerRecorder` test fake at `internal/services/images/service_test.go:22-52` provides both `Run` and `Output` methods keyed by joined-arg-string — exactly what U3's three-label cache matrix needs.
- `installFakeDocker(t)` exists at `internal/cli/manage_test.go:599` — usable for U4's CLI tests.
- `claudeservice.Options{Image: ...}` and `codexservice.Options{Image: ...}` exist at `claude.go:103-106` and `codex.go:110-113` — confirming U4 can swap in the per-project ref.
- `ensureClaudeImageCurrent` at `claude.go:189-206` and `ensureCodexImageCurrent` at `codex.go:227-244` are the existing pre-launch image-current hooks U4 will mirror.
- `internal/cli/operator_helpers.go` exists with `openImagesService` at line 70 — U4's `resolveProjectImage` helper has a natural home there.
- Tag charset/length: `proj-<12-hex>` is 17 chars, lowercase alnum + `-`, starts with alnum — satisfies Docker reference grammar `[a-zA-Z0-9_][a-zA-Z0-9_.-]{0,127}` trivially. The 17-char claim in Notes For Builder #3 is correct.
- Unit boundaries are compile-safe: U1 introduces `OverlayHash`/`ShortOverlayHash`/`canonicalManifest` in new `overlay.go` (imports `internal/tools`); U2 extends `overlay.go` with `BuildOverlayDockerfile` (imports `internal/adapters/docker`); U3 adds `Service.EnsureProjectImage` calling U2's helper; U4 calls U3 from CLI. No forward references.
- Acceptance criteria 1-9 all map to at least one unit's acceptance.
- `valv image` cobra subtree exists in `internal/cli/manage.go` (e.g. `valv image update`, `valv image cleanup`) — U5's smoke check `./valv image --help` is feasible.

## Hylla Feedback

Hylla snapshot 7 has full coverage of `internal/tools` package (DROP_11 outputs). Plan's Note #1 caveat is obsolete — should be updated in revise.

No Hylla misses during this review.
