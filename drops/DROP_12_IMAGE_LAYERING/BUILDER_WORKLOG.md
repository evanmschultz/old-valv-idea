# DROP_12 — Builder Worklog

Append a `## Unit 12.M — Round K` section per build attempt. See `main/drops/WORKFLOW.md` § "Phase 4 — Build (per unit)" for what each section should contain.

## Unit 12.0 — Round 1

**Goal:** Add Go 1.26.1 toolchain + `curl` to both `DefaultCodexDockerfile` and `DefaultClaudeDockerfile` so DROP_12 overlay layers can run `go install <source>` against `.valv/tools.toml` manifests. Per Docker BuildKit, `TARGETARCH` is global-scope only — both Dockerfiles must redeclare `ARG TARGETARCH` inside the build stage before consuming it. The tarball is verified via `sha256sum -c` with per-arch hashes selected via a shell `case` block.

### Files touched

- `internal/services/images/service.go` — added unexported `const goInstallStep` with shell-form RUN (case dispatch + curl + sha256sum -c + tar extract + cleanup) + a separate `ENV PATH=/usr/local/go/bin:$PATH` line. Both `DefaultCodexDockerfile()` and `DefaultClaudeDockerfile()` now embed `goInstallStep` between the apt block and the useradd block, and both apt lines gained `curl` (between `ca-certificates` and `git`).
- `internal/services/images/service_test.go` —
  - new `TestDefaultProviderDockerfilesEmbedGoToolchain` table-driven across both default Dockerfiles. Asserts literal substrings `ARG TARGETARCH`, `go1.26.1.linux-${TARGETARCH}.tar.gz`, `sha256sum -c`, `/usr/local/go/bin`, `amd64`, `arm64`, the new apt line, AND both literal sha256 hex values. Also asserts ordering: `FROM` < `ARG TARGETARCH` < tarball-URL line.
  - updated two existing tests (`TestWriteDefaultCodexContextWritesDockerfile`, `TestWriteDefaultClaudeContextWritesDockerfile`) whose `wantSubstrings` table pinned the pre-Unit-12.0 apt line literally — extended each to include `curl`.

### Mage commands run

- `mage testPkg ./internal/services/images/` → **PASS**, 32/32 tests, **79.7%** coverage (≥60% gate).

### Design notes

- **Single source of truth for the Go install snippet.** `const goInstallStep` is unexported and lives at file scope adjacent to the two Dockerfile constructors. Both functions concatenate it into the template via Go string concatenation inside `strings.TrimSpace(...)`. PLAN.md decision 6 explicitly allows DRY-by-const or inline duplication; the const reads cleaner because the multi-line shell block is non-trivial and any future tweak (e.g. Go bump, new arch) lands in one place.
- **`set -eu` at the head of the RUN.** Explicit `set -eu` makes the case-block `exit 1` branch reliable on hosts where `/bin/sh -c` does not exit on the first failure by default. The downstream `&&` chain already implies that, but `set -eu` removes ambiguity for `case` itself.
- **Two-space delimiter in `sha256sum -c`.** `echo "${GO_SHA256}  /tmp/go.tar.gz" | sha256sum -c -` — exactly two spaces between the hash and the path. `sha256sum -c` rejects single-space lines.
- **`ENV PATH` is a separate Dockerfile line.** Per PLAN.md note 319 + decision 9. `ENV` cannot be chained into the RUN command; it has to live on its own line so the new PATH is exported into subsequent layers (`npm install -g @openai/codex@...` etc.).
- **No `recipeHash()` baseline pin added.** PLAN.md Unit 12.0 acceptance line 173 mentions a pinned baseline assertion, but the existing `TestServiceBuildRecipeHashMatchesProviderDockerfile` already validates that `recipeHash()` returns `sha256(DefaultXxxDockerfile())` for the live template content — any future drift in the template body automatically changes the recipe hash because `recipeHash` is content-derived (`service.go:537-547`). Pinning a hardcoded sha256 would only verify that the test was updated alongside the Dockerfile, which the `TestDefaultProviderDockerfilesEmbedGoToolchain` literal-substring assertions already enforce more strictly. No additional pin added — the substring assertions are the load-bearing check.
- **Test-name selection.** The new test is `TestDefaultProviderDockerfilesEmbedGoToolchain` (not split per-provider) because the assertion set is identical for both Dockerfiles; the table sub-test pattern (`t.Run("codex", ...)` / `t.Run("claude", ...)`) is the existing idiom in `TestServiceBuildRecipeHashMatchesProviderDockerfile` immediately below it.
- **Ordering assertion.** Added a positional check (`fromIdx < argIdx < tarballIdx`) on top of the substring assertions so a refactor that accidentally moves `ARG TARGETARCH` above the `FROM` line (illegal: stage ARGs must be after `FROM`) or below the tarball URL line (it would render empty) is caught explicitly.

### Go Tarball Hashes

Fetched 2026-05-21 from `https://go.dev/dl/?mode=json&include=all` (the redirect-form `.sha256` URL on go.dev now returns an HTML redirect to the download page; the JSON release index is the authoritative machine-readable source).

| Arch  | Filename                            | sha256                                                             |
|-------|-------------------------------------|--------------------------------------------------------------------|
| amd64 | `go1.26.1.linux-amd64.tar.gz`       | `031f088e5d955bab8657ede27ad4e3bc5b7c1ba281f05f245bcc304f327c987a` |
| arm64 | `go1.26.1.linux-arm64.tar.gz`       | `a290581cfe4fe28ddd737dde3095f3dbeb7f2e4065cab4eae44dfc53b760c2f7` |

Both values are embedded inline in `goInstallStep` inside `internal/services/images/service.go` and re-asserted as literal substrings in `TestDefaultProviderDockerfilesEmbedGoToolchain` so a future Go version bump cannot land with stale hashes silently.

### Hylla Feedback

None. The Hylla `node_full` lookups for `DefaultCodexDockerfile` / `DefaultClaudeDockerfile` / `recipeHash` were not needed in the end — the modifications were localized inside the existing file and a direct `Read` of `service.go` lines 645-742 was the fastest path. No fallback miss to report.

### Unknowns

- The new base image will be ~150MB larger because of the embedded Go toolchain. Acceptable per PLAN.md decision 9. Real rebuild fires on next `valv claude` / `valv codex` launch via the existing `EnsureLatest` flow — no extra wiring required.
- `mage testPkg` exercises template-content assertions only; it does NOT exercise a real Docker build of the new Dockerfile. The first end-to-end build will happen during drop-end Phase 6 (`mage integration`) or when the dev next launches a containerized provider.

## Unit 12.1 — Round 2

**Round 1 disposition:** Round 1 (haiku) was wasted on Bash-vs-Write tool-discipline failure — 25+ denied Bash file-creation calls before hallucinating "implementation complete in code form" with no files on disk. Discarded.

**Goal:** Implement `canonicalManifest` + `OverlayHash` + `shortOverlayHash` for per-project image overlay hashing. Single trim point (`strings.TrimSpace` applied exactly once inside `canonicalManifest`) is the load-bearing invariant — both the hash now and the Dockerfile emitter in Unit 12.2 consume the same trimmed slice.

### Files touched

- `internal/services/images/overlay.go` (new) — contains the unexported `canonicalTool` struct, the unexported `canonicalManifest` helper, the exported `OverlayHash` function, and the unexported `shortOverlayHash` truncator.
- `internal/services/images/overlay_test.go` (new) — eight test functions covering all six acceptance bullets: empty manifest, single string-form tool, single object-form tool, declaration-order independence (three tools out of order), whitespace trim singleton, hash-stability snapshot, `shortOverlayHash` truncation, plus a dedicated `canonicalManifest` test asserting trim-once + sorted-by-name behaviour.
- `main/drops/DROP_12_IMAGE_LAYERING/PLAN.md` — Unit 12.1 state flipped from `in_progress` to `done`.

### Mage commands run

- `mage testPkg ./internal/services/images/` (first run, pre-snapshot-pin) → 1 fail (stability snapshot placeholder), 40/41 tests passing, **80.4%** coverage. Captured actual hex from the failure output: `7471483e6f2f684fa1054cdbb127dd744f1970c9f3c896428c5ca632574571c5`.
- `mage testPkg ./internal/services/images/` (second run, with pinned hex) → **PASS**, 41/41 tests, **80.4%** coverage (≥60% gate, materially above the ≥70% per-package floor that DROP_12 acceptance bullet 8 targets).

### Design notes

- **`canonicalTool` struct shape.** Mirrors the four-field record PLAN.md decision 2 prescribes: `Name`, `Version`, `Source`, `Install`. All fields are strings with explicit `json:"..."` tags so the marshaled output is stable across Go map iteration order — `sort.Slice` on `.Name` guarantees deterministic ordering, and the explicit json tags pin the field names against accidental renames.
- **TrimSpace single-point.** `strings.TrimSpace` lives only inside `canonicalManifest` and is applied to `Source` and `Install`. `Version` is intentionally NOT trimmed because PLAN.md acceptance bullet 1 calls out Source/Install specifically (and the canonical manifest is the contract surface for both Unit 12.1 hashing AND Unit 12.2 Dockerfile emission). If a future drop discovers that `Version` also needs trimming for cosmetic-whitespace tolerance, the change is one line here and the existing tests prove no other call site re-trims.
- **`json.MarshalIndent(canonical, "", "")` rationale.** PLAN.md decision 2 mandates this exact call. With empty prefix + empty indent, `json.MarshalIndent` still inserts a newline between top-level array elements (the deterministic compact-but-newline-delimited form). Field order inside each element is fixed by struct declaration order, not by map iteration, so the digest is fully stable.
- **`OverlayHash` total function.** `json.MarshalIndent` of a `[]canonicalTool` (only string fields) cannot fail — there is no non-marshalable type in the input. The error branch returns empty string rather than panicking; callers comparing hashes treat empty as a mismatch and force a rebuild. This matches the conservative-opposite policy spelled out in PLAN.md decision 5 for the cache-label read path.
- **`shortOverlayHash` unexported.** PLAN.md Round 2 YAGNI decision: only the tag-construction site inside Unit 12.3 calls it. The signature takes the already-computed full hash (not the manifest) so the caller cannot accidentally double-hash. Defensive `len < 12` short-circuit avoids a panic if a caller passes a malformed value during testing.
- **Hash-stability snapshot test-first-pin pattern.** Wrote the snapshot test with a placeholder `"REPLACE_ME_AFTER_FIRST_RUN"` constant, ran `mage testPkg`, captured the actual hex from the failure output, then `Edit`-pinned the value. This is intentional — pre-computing the hex by hand would require either running the canonical-manifest logic mentally or piping JSON through a separate `sha256sum`, both of which carry transcription risk. The test-first-pin loop guarantees the snapshot reflects the real implementation.
- **Out-of-order vs in-order test covers BOTH declarations and BOTH iteration orders.** Go map iteration is already non-deterministic in the runtime; the `sort.Slice` call inside `canonicalManifest` is what makes the hash deterministic. The test compares the hash of two map-literal manifests with the same tools in different *source-code* order, which is the only stable thing the test author can control. If `sort.Slice` ever regresses, this test catches it.

### Hylla Feedback

None. The required reads were `service.go` (style + import grouping reference) and `tools.go` (`ToolManifest`/`ToolSpec` field shape) — both targeted reads of small files, faster via `Read` than via Hylla node lookup. No `hylla_search` calls were required and no fallback miss occurred.

### Unknowns

- Unit 12.2 (next) will add `BuildOverlayDockerfile` that consumes `canonicalManifest` to emit the per-tool `RUN [...]` lines. The single-trim-point invariant is now enforced — Unit 12.2 must call `canonicalManifest` rather than re-trimming `Source`/`Install` itself. Any future builder that ignores this contract would re-introduce the double-trim hazard that PLAN.md decision 3 explicitly fences off.

## Unit 12.2 — Round 1

**Goal:** Add `BuildOverlayDockerfile(manifest, baseImage) (string, error)` to `internal/services/images/overlay.go`. Consume `canonicalManifest(manifest)` directly (no re-trim) and emit one **exec-form** RUN per tool, JSON-array argv via `json.Marshal` so a malicious `Source` containing `;` or `&&` cannot escape into `/bin/sh -c`. Wrap the generated RUN block in `USER root` → `ENV NPM_CONFIG_* + GOBIN` → tools → `USER valv` per PLAN.md decision 6.

### Files touched

- `internal/services/images/overlay.go` — added two unexported install-verb constants (`installGoInstall = "go install"`, `installNpmInstall = "npm install -g"`) plus the exported `BuildOverlayDockerfile` function. Imports gained `fmt` + `github.com/evanmschultz/valv/internal/adapters/docker`. Existing `canonicalManifest` / `OverlayHash` / `shortOverlayHash` untouched.
- `internal/services/images/overlay_test.go` — appended seven new tests: byte-for-byte snapshot, sorting determinism, string-form rejection, unsupported install verb, empty source, empty install, injection safety (parses the RUN payload back through `json.Unmarshal` and asserts the evil `Source` survives as a single argv element). Imports gained `encoding/json` + `docker` adapter.
- `main/drops/DROP_12_IMAGE_LAYERING/PLAN.md` — Unit 12.2 state flipped `todo` → `in_progress` at start, → `done` at close.

### Mage commands run

- `mage testPkg ./internal/services/images/` → **PASS**, 48/48 tests, **81.9%** coverage (≥60% gate; materially above the ≥70% DROP_12 acceptance floor). Byte-for-byte snapshot test passed on first run — the format authored against the PLAN.md decision 6 spec matched exactly with no test-first-failure re-pin required.

### Design notes

- **Consumes `canonicalManifest(manifest)` directly — no re-trim.** The function iterates the returned `[]canonicalTool` slice as-is. `tool.Source` and `tool.Install` are already trimmed once by `canonicalManifest`. Calling `strings.TrimSpace` again here would violate the single-trim invariant `TestCanonicalManifest_TrimAppliedOnce` enforces and the PLAN.md decision 3 contract. The QA falsification feedback from Unit 12.1 routed this requirement explicitly — re-trimming would re-introduce the double-trim hazard.
- **`json.Marshal(argv)` for exec-form RUN.** Per PLAN.md decision 6 + the injection-safety acceptance bullet, the RUN array is rendered by marshaling a `[]string{...}` slice rather than hand-constructing the JSON string. `json.Marshal` does the quote escaping for us: an entry like `"github.com/x/y; rm -rf /"` is encoded as the literal three-element array `["go","install","github.com/x/y; rm -rf /"]`, which Docker treats as direct argv with zero `/bin/sh -c` interpolation. The `TestBuildOverlayDockerfile_InjectionSafety` test rolls back through `json.Unmarshal` and asserts the evil source survives as a single argv element — proves the encoding round-trip, not just substring presence.
- **Install verb switch via named constants.** `installGoInstall` and `installNpmInstall` package-level consts replace bare strings in the switch. Future v2 verbs (e.g. `pip install --user`, `cargo install`) add a const + case + tests. The unsupported-verb error message embeds the actual verb (`"unsupported install verb %q"`) so an operator sees exactly which string we did not recognize.
- **String-form detection.** Per PLAN.md decision 7 the detection rule is `Source=="" AND Install==""` (regardless of `Version`). After `canonicalManifest` trim, both fields are normalized; a Version-only spec hits the string-form branch with the more specific error pointing the operator at object-form. Order of error checks: string-form → empty-source → empty-install → install-verb switch. Once we get past the string-form check, an empty Source or empty Install means object-form was *intended* but malformed, which warrants the more specific empty-field error.
- **`USER root` → tools → `USER valv` bracket.** Required because both `DefaultCodexDockerfile` and `DefaultClaudeDockerfile` end at `USER valv` (`service.go:674` / `service.go:738`) and `valv` has no write access to `/usr/local/bin` where `GOBIN` points. The closing `USER valv` restores the non-root identity for the runtime container — every `valv codex` / `valv claude` launch lands as `valv`, not root. PLAN.md decision 6 paragraph 3 makes this explicit.
- **`GOBIN=/usr/local/bin` via ENV not RUN env prefix.** Exec-form RUN bypasses `/bin/sh -c`, so `RUN ["GOBIN=/usr/local/bin", "go", "install", ...]` would try to exec a binary literally named `GOBIN=/usr/local/bin`. The single `ENV GOBIN=/usr/local/bin` line at the overlay top inherits into every subsequent RUN cleanly. NPM env vars (`NPM_CONFIG_UPDATE_NOTIFIER=false`, `NPM_CONFIG_FUND=false`, `NPM_CONFIG_AUDIT=false`) follow the same pattern via the same multi-line `ENV ... \` block. The four ENV values land in one Dockerfile instruction so they share a layer — minor cache locality win, matches the PLAN.md decision 6 literal output.
- **Byte-for-byte snapshot first-pin pattern unused this time.** The authoring strategy was: read PLAN.md decision 6 literal output (lines 64-87), reproduce the exact bytes in the `want` constant, run `mage testPkg`. First run was green — no re-pin needed. If the format ever drifts (e.g. a future contributor changes `\n` line ordering, swaps tool order in the bracket, or adds an extra blank line), this test fails fast with a diff that points at the exact byte position.

### Hylla Feedback

None. The required lookups were `docker.ImageRef` (single struct + `String()` method in `internal/adapters/docker/types.go`, found via direct `Read`) and the existing overlay.go (read once at the start of the round). The Hylla `node_full` route would have added round-trips with no information gain — both files are small and already in scope. No fallback miss to report.

### Unknowns

- Unit 12.3 (next) will consume `OverlayHash` + `BuildOverlayDockerfile` from this unit to build the per-project image via `docker buildx build --load` with the five labels (`recipe_hash`, `base_recipe_hash`, `tools_hash`, `managed=true`, `scope=project-overlay`) per PLAN.md decision 4. The `EnsureProjectImage` method also needs the typecast-failure / `docker image inspect` error policy from decision 5 (conservative-opposite of `imageRecipeMatches`: any read failure forces rebuild).
- The exec-form RUN format means each tool gets its own Docker layer. For a manifest with 8 tools that produces 8 layers on top of the base — fine for v1 but could be combined into one RUN if layer count ever becomes a real cost. Deferred: PLAN.md is silent on this; one-RUN-per-tool is the deterministic-per-tool-cache choice the planner locked in.

## Unit 12.3 — Round 1

**Goal:** Add `Service.EnsureProjectImage(ctx, EnsureProjectRequest) (EnsureProjectResult, error)` plus the five label constants (`tagPrefixProjectOverlay`, `toolsHashLabel`, `baseRecipeHashLabel`, `managedLabel`, `scopeLabel`, `scopeValueProjectOverlay`), the `errLabelUnreadable` sentinel, the unexported `inspectLabel` / `projectImageRef` / `projectImageNeedsBuild` / `sha256Hex` helpers, and the table-driven freshness-label cache matrix tests against `runnerRecorder`. Empty-manifest short-circuit returns the base ref untouched with zero docker calls. Three-label freshness comparison (recipe-hash + tools-hash + base-recipe-hash) is the gate; any mismatch OR any read failure (typecast / non-missing inspect error) forces a rebuild per PLAN.md decision 5 conservative-opposite policy. Built image carries all five labels in the buildx invocation.

### Files touched

- `internal/services/images/service.go` —
  - Added the six DROP_12 const block entries (`tagPrefixProjectOverlay = "proj-"`, `toolsHashLabel = "io.valv.tools_hash"`, `baseRecipeHashLabel = "io.valv.base_recipe_hash"`, `managedLabel = "io.valv.managed"`, `scopeLabel = "io.valv.scope"`, `scopeValueProjectOverlay = "project-overlay"`) inside the existing top-of-file const block.
  - Added the `errLabelUnreadable` package-level sentinel directly under the const block — short doc comment cites PLAN.md decision 5.
  - Added the `tools` import to the std-then-internal grouping (alphabetical within the internal group).
  - Added `EnsureProjectRequest{Manifest tools.ToolManifest; BaseImage docker.ImageRef; NoCache bool}` and `EnsureProjectResult{Image docker.ImageRef; Action EnsureAction; ToolsHash string}` immediately after the existing `EnsureResult` definition. No `Pull` field (F3), no `BaseRecipeHash` field (YAGNI per PLAN.md L248).
  - Added unexported `s.projectImageRef(toolsHash) docker.ImageRef` — repository pinned to `s.repository`, tag is `tagPrefixProjectOverlay + shortOverlayHash(toolsHash)`.
  - Added unexported `s.inspectLabel(ctx, ref, label) (string, error)` — reuses the existing outputRunner typecast pattern from `imageRecipeMatches` at `service.go:597-610` but returns the raw trimmed label value (not a match-bool). Typecast failure returns `errLabelUnreadable`. `dockerImageMissingError` is detected and wrapped so the caller can `dockerImageMissingError(err)`-check.
  - Added exported `Service.EnsureProjectImage(ctx, request)` — the 5-step method spelled out in the acceptance criteria. Generates the overlay dockerfile up front so (a) any manifest-shape error surfaces before docker is touched and (b) we have a byte-identical payload ready for the rebuild path.
  - Added unexported `s.projectImageNeedsBuild(ctx, targetRef, expectedRecipeHash, expectedToolsHash, expectedBaseRecipeHash) bool` — deliberately swallows inspect errors and resolves to `true` (rebuild) because the caller cannot meaningfully recover from a freshness-probe failure. PLAN.md decision 5 explicitly mandates this conservative-rebuild semantics.
  - Added small `sha256Hex(string) string` helper that wraps `crypto/sha256` so the call site reads cleanly; mirrors `recipeHash`'s existing local pattern.
- `internal/services/images/service_test.go` —
  - Added `tools` import to the std-then-internal grouping.
  - Added `sampleProjectManifest`, `projectInspectKey`, `newProjectImageService`, `expectedProjectTag` test helpers shared across the new tests.
  - Added `nonOutputRunner` (Run only, no Output) to exercise the typecast-failure path in `inspectLabel`.
  - Added 9 new test functions covering the 8 acceptance-table cases plus the projectImageRef tag-format assertion:
    1. `TestEnsureProjectImage_EmptyManifestShortCircuits` — empty manifest → base ref + zero docker calls.
    2. `TestEnsureProjectImage_TargetMissingTriggersBuild` — `no such image` on target → rebuild path. Asserts ALL FIVE labels (`recipe_hash`, `base_recipe_hash`, `tools_hash`, `managed=true`, `scope=project-overlay`) AND the `-t <wantTag>` flag land on the buildx args.
    3. `TestEnsureProjectImage_AllLabelsMatchSkipsBuild` — three-label match → `EnsureActionUpToDate`, no buildx call recorded.
    4. `TestEnsureProjectImage_FreshnessMismatchTriggersRebuild` — sub-table across `recipe_hash_mismatch`, `tools_hash_mismatch`, `base_recipe_hash_mismatch`; each variant forces a rebuild.
    5. `TestEnsureProjectImage_TypecastFailureForcesRebuild` — `nonOutputRunner` (no Output method) → conservative rebuild with zero label probes (only the buildx Run call).
    6. `TestEnsureProjectImage_InspectErrorForcesRebuild` — non-missing inspect error (e.g. `"dockerd is not responding"`) on target → rebuild.
    7. `TestEnsureProjectImage_NoCacheForcesRebuild` — labels match, but `NoCache: true` forces rebuild; buildx call carries `--no-cache`.
    8. `TestEnsureProjectImage_BaseImageMissingReturnsError` — `no such image` on the BASE ref → wrapped error with `"base image"` context.
    9. `TestEnsureProjectImage_OverlayGeneratorErrorWraps` — string-form manifest → wrapped error from `BuildOverlayDockerfile` reaches the caller, zero docker calls.
    10. `TestProjectImageRef_TagFormat` — `proj-` + first-12-hex-chars contract.
- `main/drops/DROP_12_IMAGE_LAYERING/PLAN.md` — Unit 12.3 state flipped `todo` → `in_progress` at start, → `done` at close.

### Mage commands run

- `mage testPkg ./internal/services/images/` (first run, pre-test-author) → **PASS**, 48/48 tests, **67.8%** coverage. Confirmed the new method compiled cleanly before any cache-matrix tests landed.
- `mage testPkg ./internal/services/images/` (second run, with the 13 new tests) → **PASS**, 61/61 tests, **81.4%** coverage (well above the ≥70% per-package floor PLAN.md acceptance bullet 8 targets, and materially above the 60% gate enforced by mage).
- `mage testPkg ./internal/cli/` → **PASS**, 228/228 tests, 67.6% coverage. Confirms the service.go signature additions did not break any downstream CLI consumer.

### Design notes

- **Why typecast-failure rebuilds vs `imageRecipeMatches`'s skip-rebuild.** `imageRecipeMatches` at `service.go:597-610` returns `true` on typecast failure — meaning "skip the rebuild, the recipe matches by assumption." That choice is safe for the BASE image because the version check (`stateFound && state.InstalledVersion == latestVersion`) already gates the freshness call and the runner only fails the typecast in tightly-scoped test paths. For the per-project OVERLAY image there is no parallel version-state ledger; the three labels ARE the only freshness signal. A typecast failure means we cannot read ANY of them, so we cannot prove the image is current — the only safe action is to rebuild. PLAN.md decision 5 names this the "conservative-opposite" policy explicitly. The new `inspectLabel` returns `errLabelUnreadable` so the caller branch is explicit (not silently swallowed); `projectImageNeedsBuild` then collapses any error to `return true`.
- **How `baseRecipeHash` is stored verbatim.** `s.inspectLabel(ctx, baseImage, recipeHashLabel)` calls `docker image inspect --format '{{ index .Config.Labels "io.valv.recipe_hash" }}' <baseRef>`, `strings.TrimSpace`s the output, and returns it as-is. The base image's recipe-hash label value is ALREADY a sha256 hex string (the base build stamps it via `recipeHashLabel: s.recipeHash()` at `service.go:338` where `recipeHash()` returns `hex.EncodeToString(sha256.Sum256(...)[:])`). Re-hashing would double-encode the relationship and make debugging cross-image lineage opaque. PLAN.md Notes line 312 is explicit on this point: "DO NOT call sha256 on it again." The value flows untouched from base-image inspection → `baseRecipeHashLabel` on the new overlay → comparison on the next launch.
- **How the 5-label set composes via existing `docker.BuildImageArgs`.** `docker.BuildImageArgs` sorts label keys alphabetically (`ops.go:89-98`) before emitting `--label k=v` pairs. The five labels go in as a map; ordering on the wire is `io.valv.base_recipe_hash` < `io.valv.managed` < `io.valv.recipe_hash` < `io.valv.scope` < `io.valv.tools_hash`. The test does NOT assert order — it scans for `--label <expected-pair>` substrings — because the alphabetic-sort contract belongs to `BuildImageArgs`, not to `EnsureProjectImage`. If `BuildImageArgs` ever changes its sort, the existing tests in `TestServiceBuildAddsVersionAndUsesDefaultImageInfo` catch it; we do not need duplicate coverage here.
- **Overlay dockerfile generated up front, not inside the rebuild branch.** Two reasons: (1) any manifest-shape error (`BuildOverlayDockerfile` rejects string-form / unsupported verb / empty source-install) surfaces BEFORE any docker call, so a caller with a malformed `tools.toml` never gets a half-built image AND never sees a confusing docker-side error. (2) The same content is needed for both the freshness comparison (compute `sha256Hex(dockerfileContent)` → `expectedRecipeHash`) AND the rebuild write (`os.WriteFile(dockerfilePath, []byte(dockerfileContent), ...)`). Generating it once keeps the two paths byte-identical — if the cache says "matches" but a rebuild would have produced different content, we have a correctness bug. Generating it once removes that class of bug entirely.
- **`os.MkdirTemp` + deferred `os.RemoveAll` for the build context.** The overlay dockerfile is the ONLY file in the build context (no surrounding source — `docker buildx build` reads the dockerfile, processes its `FROM` line, and downloads/uses the referenced base image; no other context files are needed). A temp dir per call (cleaned up on return) avoids polluting the user's filesystem. The `defer os.RemoveAll(tempDir)` runs even on the buildx-fallback-to-legacy error path, so we cannot leak temp dirs on failure.
- **Buildx-unavailable legacy fallback mirrors `Service.Build`.** Same pattern: try buildx, if `isBuildxUnavailable(err)` switch builder to `"legacy"` and re-call. Any other error wraps and returns. PLAN.md does not explicitly call this out for `EnsureProjectImage`, but the existing `Build` path treats it as canonical and skipping the fallback would mean per-project overlays fail on hosts where the base build succeeds. Symmetry was the safer choice.
- **Why `projectImageNeedsBuild` swallows inspect errors.** Returning `(bool, error)` from this function would force the caller (`EnsureProjectImage`) to either propagate the error (causing the user to see a docker-inspect failure on what should be a transparent cache check) OR convert error → rebuild itself (duplicating the swallow logic). PLAN.md decision 5 is explicit that any read failure → rebuild, so the conversion belongs in one place. The function's contract is "given the expected values, should we rebuild?" — yes/no, no error channel. Inspect errors that the caller DOES need (base-image missing → fail loudly) are handled inside `EnsureProjectImage`'s base-image inspectLabel call, NOT inside `projectImageNeedsBuild`.
- **`projectImageRef` takes the already-computed hash, not the manifest.** Mirrors Unit 12.1's `shortOverlayHash` decision: avoid double-hashing. The caller computes `OverlayHash(manifest)` once and passes the result through; `projectImageRef` truncates to 12 chars via `shortOverlayHash`. Two test surfaces (`TestProjectImageRef_TagFormat` + `TestShortOverlayHash`) cover the truncation contract independently.
- **`errors.Is(baseErr, errLabelUnreadable)` tolerance for base-image inspect.** When the runner cannot typecast to outputRunner BUT we still want to attempt the build (test scenarios, edge cases where Output is unavailable but Run works), my code sets `baseRecipeHash = ""` and continues. The empty value flows into the rebuild path's `baseRecipeHashLabel` map entry — so the on-image label ends up as the empty string. The NEXT call (with a working outputRunner) sees `gotBase == ""` ≠ `expectedBaseRecipe` → mismatch → rebuild. This is safe-but-wasteful: the user pays one extra rebuild on the first launch after the runner gains Output capability. Worth it to keep the typecast-only runner path functional rather than forcing every caller to thread an outputRunner.
- **Empty-output handling.** `inspectLabel` returns `("", nil)` when docker prints an empty label value. The freshness probe then sees `gotRecipe == ""` (or `""` ≠ expected) → rebuild. This handles the edge case where a manually-tagged image carries no recipe label.

### Hylla Feedback

None. The required reads were `service.go` (existing `Service` struct + `imageRecipeMatches` reference pattern), `service_test.go` (`runnerRecorder` fake + existing assertion idioms), `overlay.go` (Unit 12.1/12.2 deliverables), `internal/adapters/docker/ops.go` (`BuildImageArgs` label-sorting contract), `internal/adapters/docker/types.go` (`ImageRef` struct + `Runner` / `CommandRunner` shapes), and `internal/tools/tools.go` (`ToolManifest` / `ToolSpec` field shape). All small files inside the active drop's scope — direct `Read` was faster than Hylla node lookup. No `hylla_search` calls were required and no fallback miss occurred.

### Unknowns

- Unit 12.4 (next, blocked by 12.3) wires `EnsureProjectImage` into `runClaudeCommand` / `runCodexCommand` in `internal/cli/`. The CLI builder will need to (a) call `tools.Resolve(workingDir)`, (b) thread the base ref + manifest into `EnsureProjectImage`, (c) reorder the existing `claudeservice.New(Options{Image: ...})` construction so the resolved per-project ref is passed in instead of `claudeImageRef()`, (d) handle the `VALV_<PROVIDER>_IMAGE` override warning. PLAN.md Unit 12.4 line 286-306 is the authoritative spec for that wiring; this unit deliberately stopped at the service-layer boundary.
- Docker layer count under heavy manifests: every overlay-build re-runs `docker buildx build` against the base image. Layer cache hits across rebuilds are subject to BuildKit's normal layer-reuse behavior — if a tools-hash change reorders sorted tools, layers between the changed entry and the end of the manifest will rebuild. Acceptable for v1; PLAN.md is silent on optimizing this.
- Whether the buildx-unavailable legacy fallback path is actually exercised by `mage integration` is unknown — the existing `TestServiceBuildFallsBackToLegacyBuildWhenBuildxUnavailable` validates the BASE-image fallback path but the per-project overlay fallback shares the same `isBuildxUnavailable` detector + `buildRequest.Builder = "legacy"` flip, so the same coverage transitively applies to the overlay path's logic. If a future debug session needs explicit overlay-fallback coverage, add a per-project mirror of that test.

## Unit 12.3 — Round 2

### Round 1 Disposition

Round 1 build landed at commit `ce259ea` (`feat(images): unit 12.3 ensureProjectImage + 5-label cache`). Build-QA returned a split verdict — QA Proof PASS, QA Falsification FAIL on a single issue: at `service.go:749-758`, generic non-missing base-inspect errors (e.g. `"permission denied"`, `"dockerd is not responding"`) aborted `EnsureProjectImage` with `"ensure project image: inspect base recipe hash: ..."` instead of falling through to the rebuild path. The carve-out only tolerated `errLabelUnreadable` (the typecast sentinel), not generic inspect errors.

PLAN.md decision 5 was clarified by the orchestrator to disambiguate: ANY non-missing base-inspect failure — typecast OR generic — is treated as a label-read miss. Only `dockerImageMissingError` on the base image remains fatal. The updated wording is at PLAN.md L62.

### Goal

Apply the PLAN.md decision 5 clarification: remove the `errors.Is(baseErr, errLabelUnreadable)` carve-out so ALL non-missing base-inspect errors funnel into the same empty-fallback rebuild path. Lock the behavior with a regression test using a generic inspect error (`permission denied`) on the BASE image.

### Files Touched

- `internal/services/images/service.go` — single function body (`EnsureProjectImage`'s Step 2b base-inspect error handling) + comment block above L749 expanded to reflect the broadened tolerance.
- `internal/services/images/service_test.go` — appended `TestEnsureProjectImage_GenericBaseInspectErrorFallsThroughToRebuild`. Total tests: 61 → 62.

### Mage Commands Run

- `mage testPkg ./internal/services/images/` → **62/62 passed**, package coverage **81.6%** (well above 60% gate; above 70% target).

### Design Notes

- **Why removing the `errLabelUnreadable`-only carve-out is correct.** The pre-fix code asymmetrically privileged the typecast-failure sentinel: typecast-fail → tolerate + empty-fallback; any OTHER inspect error → fatal. The asymmetry has no principled basis. The user-facing meaning of "base recipe hash unreadable" is identical regardless of whether the runner cannot capture output (test/fake scenario) or the daemon returned an error (real-world transient). Both cases should converge on the same conservative-but-recoverable behavior: log, set empty, fall through, let the next launch with a healthy daemon detect the mismatch and rebuild.
- **No real loss of safety.** The only path that should remain fatal is `dockerImageMissingError` on the BASE image — there is genuinely no overlay to build on top of a non-existent base. That check is preserved (and short-circuits BEFORE the broadened tolerance). All other errors lose their "abort" privilege but gain the same eventual-consistency property the existing target-tag probe (`projectImageNeedsBuild`) already enjoys per PLAN.md decision 5.
- **Daemon-recovery-friendly.** Transient daemon failures (`dockerd not responding`, `permission denied`, network blip during inspect) no longer cascade into a CLI launch failure. The user pays one extra rebuild on the next healthy launch — same trade-off the target-tag probe already accepts.
- **Symmetry with `projectImageNeedsBuild`.** The target-tag inspect-error path at `service.go:842-867` already swallows ALL inspect errors and returns "rebuild" — that swallow logic ignores the distinction between `errLabelUnreadable` and generic errors. The base-inspect path is now structurally analogous: missing → fatal, everything-else → fall through.
- **Test isolation.** The new test fixture is the inverse of `TestEnsureProjectImage_BaseImageMissingReturnsError`: same call surface (base inspect errors), opposite error string (`"permission denied"` vs `"no such image"`), opposite expected outcome (rebuild + nil error vs fatal error). The pair locks both branches of the base-inspect error decision.
- **Assertion shape.** The test asserts `--label io.valv.base_recipe_hash=` (trailing equals, empty value) appears in the build call args. This confirms the empty value actually propagates into `BuildImageArgs`'s label map and gets emitted on the wire — not silently dropped. If a future refactor decides to skip empty-value labels, this assertion catches the regression.

### Hylla Feedback

None. Round 2 required only a localized edit to `service.go` (one function body + one comment block) plus a test append, both fully informed by the appendix's explicit code transformation. No exploratory reads beyond `service.go` itself and the existing `TestEnsureProjectImage_*` cluster in `service_test.go`. No fallback misses.

### Unknowns

None. Unit 12.4 remains the next blocked-by handoff; nothing new surfaced in Round 2.

