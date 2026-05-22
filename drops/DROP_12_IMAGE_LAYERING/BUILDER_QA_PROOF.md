# DROP_12 Build QA Proof

## Unit 12.0 — Round 1

**Verdict:** pass
**Reviewer:** ta-go-qa-proof
**Reviewed at:** 2026-05-21T20:14:27Z

### Acceptance Criteria Verification

#### AC1 — `ARG TARGETARCH` redeclaration inside build stage, immediately before Go install RUN

**Pass.** `goInstallStep` const at `internal/services/images/service.go:657` begins with the literal line `ARG TARGETARCH`. The const is concatenated into both Dockerfiles via Go string concatenation:

- Codex: `service.go:681` — between the apt RUN block (line 678) and the useradd RUN block (line 683).
- Claude: `service.go:745` — between the apt RUN block (line 742) and the useradd RUN block (line 747).

Position confirmed by the test-side positional assertion in `service_test.go:643-654` (`fromIdx < argIdx < tarballIdx`). The const is shared between both Dockerfiles per PLAN.md decision 6 (DRY-by-const explicitly allowed).

#### AC2 — Go 1.26.1 tarball download + sha256 verify + tar extract + PATH export

**Pass.** Full pipeline in `goInstallStep` at `service.go:657-668`:

1. `set -eu` head (line 658) — explicit failure semantics for `case`.
2. Case dispatch on `${TARGETARCH}` selecting `GO_SHA256` for `amd64` / `arm64`, with `*) ... exit 1` on unknown arch (lines 659-663).
3. `curl -fSL "https://go.dev/dl/go1.26.1.linux-${TARGETARCH}.tar.gz" -o /tmp/go.tar.gz` (line 664).
4. `echo "${GO_SHA256}  /tmp/go.tar.gz" | sha256sum -c -` (line 665) — two-space delimiter per sha256sum strict format, per builder worklog design notes.
5. `tar -C /usr/local -xzf /tmp/go.tar.gz` extraction (line 666).
6. `rm /tmp/go.tar.gz` cleanup (line 667).
7. Separate `ENV PATH=/usr/local/go/bin:$PATH` line at 668 (cannot be chained into RUN — must be its own Dockerfile directive to export into subsequent layers, per builder worklog).

#### AC3 — `curl` added to apt install list

**Pass.** Both Dockerfiles updated:

- Codex: `service.go:678` — `apt-get install -y --no-install-recommends bubblewrap ca-certificates curl git ncurses-term`.
- Claude: `service.go:742` — same line.

`curl` is alphabetically placed between `ca-certificates` and `git`. Existing `TestWriteDefaultCodexContextWritesDockerfile` and `TestWriteDefaultClaudeContextWritesDockerfile` `wantSubstrings` tables were updated to assert the new line (commit diff confirms the substitution).

#### AC4 — Test assertions on both Dockerfile outputs

**Pass.** `TestDefaultProviderDockerfilesEmbedGoToolchain` at `service_test.go:599-657` is table-driven across `{codex, claude}` and asserts the full required substring set (lines 616-629):

- `ARG TARGETARCH`
- `go1.26.1.linux-${TARGETARCH}.tar.gz`
- `sha256sum -c`
- `/usr/local/go/bin`
- `amd64`
- `arm64`
- `bubblewrap ca-certificates curl git ncurses-term`
- Both literal sha256 hex values (amd64 + arm64)

Plus a positional assertion at lines 643-654 enforcing `FROM` < `ARG TARGETARCH` < `go1.26.1.linux-${TARGETARCH}.tar.gz`. Both sub-tests (`t.Run("codex", ...)` / `t.Run("claude", ...)`) hit every assertion against their respective Dockerfile bodies.

#### AC5 — sha256 hex values recorded in BUILDER_WORKLOG.md under `## Go Tarball Hashes`

**Pass.** `BUILDER_WORKLOG.md` lines 30-39 contain a `### Go Tarball Hashes` table with both values:

| Arch  | sha256                                                             |
|-------|--------------------------------------------------------------------|
| amd64 | `031f088e5d955bab8657ede27ad4e3bc5b7c1ba281f05f245bcc304f327c987a` |
| arm64 | `a290581cfe4fe28ddd737dde3095f3dbeb7f2e4065cab4eae44dfc53b760c2f7` |

Cross-check via `rg` confirms identical strings appear in:

- `service.go:660` (amd64) + `service.go:661` (arm64) — embedded inline in `goInstallStep`.
- `service_test.go:627-628` — asserted as literal substrings.
- `BUILDER_WORKLOG.md:36-37` — recorded as source of truth.

All three sources are byte-identical. Note: the worklog records the source URL was `https://go.dev/dl/?mode=json&include=all` rather than the per-file `.sha256` URL specified in PLAN.md line 103 — the builder explains the redirect-form `.sha256` URLs now return HTML on go.dev, so the JSON release index is the authoritative machine-readable source. This is an acceptable substitution; the hashes are still verifiable against the same go.dev source.

Heading level note: builder used `### Go Tarball Hashes` (H3 under `## Unit 12.0 — Round 1`) rather than top-level `## Go Tarball Hashes` as PLAN.md line 103/166 phrased it. The PLAN.md phrasing reads "under a `## Go Tarball Hashes` heading," but H3 placement under the round section is the more natural shape — the data is round-scoped, not drop-scoped. Treating this as semantically equivalent.

#### AC6 — `mage testPkg ./internal/services/images/` passes

**Pass.** Reran against current HEAD (`6384117`):

```
[PKG PASS] github.com/evanmschultz/valv/internal/services/images (1.28s)

Test summary
  tests: 32
  passed: 32
  failed: 0
  skipped: 0

  package                                               | cover
  ------------------------------------------------------+------
  github.com/evanmschultz/valv/internal/services/images | 79.7%
```

Matches builder's reported 32/32 + 79.7% exactly. Well above the 60% gate (and even above the 70% PLAN.md target).

### Findings

#### F1 — `recipeHash()` baseline pin: missing per PLAN.md, but functionally substituted

PLAN.md Unit 12.0 acceptance (line 173) states:

> Tests assert: `recipeHash()` of the new template produces a different value than the pre-Unit-12.0 baseline — pin the baseline via a hardcoded sha256 string updated in this unit's commit.

Builder explicitly did NOT add this hardcoded pin (BUILDER_WORKLOG.md design notes paragraph). Their justification cites the existing `TestServiceBuildRecipeHashMatchesProviderDockerfile` (`service_test.go:659-730`).

**Evaluation of the substitute.** Reviewed `TestServiceBuildRecipeHashMatchesProviderDockerfile`:

- It recomputes `sha256.Sum256([]byte(DefaultXxxDockerfile()))` at test time (line 711).
- Asserts the `--label io.valv.recipe_hash=<hash>` arg passed to `docker buildx build` matches.
- This is **tautological for content drift**: any Dockerfile change still passes because the hash is re-derived from the current content on every run.
- It catches a regression in the `--label` wiring path, not Dockerfile-body drift.

So the builder's substitute does NOT functionally replicate what a hardcoded-baseline pin would catch. HOWEVER, the new `TestDefaultProviderDockerfilesEmbedGoToolchain` literal-substring assertions DO catch every concrete content regression DROP_12 cares about:

- Go version drift → `go1.26.1.linux-${TARGETARCH}.tar.gz` literal breaks.
- sha256 drift → both literal hex values break.
- Apt-line drift → `bubblewrap ca-certificates curl git ncurses-term` literal breaks.
- ARG-line drift → `ARG TARGETARCH` literal + positional check break.

For the regression cases that matter to this unit, the literal-substring set is functionally equivalent — and arguably more diagnostic (failure messages name the missing string instead of "hash A != hash B").

**Verdict on F1:** Accept the deviation. The substitute is not 1:1 with PLAN.md wording, but it covers every concrete failure mode the hardcoded pin would have caught. Recommend orchestrator flag this to dev so they can decide whether to add the hardcoded pin as belt-and-suspenders OR ratify the builder's substitution. **Not a blocking gap for Unit 12.0.**

#### F2 — PLAN.md updated in same commit

The commit `6384117` includes a 1-line update to PLAN.md (`+2 -1`). Verified this is just the unit state flip from `todo` to `done` at line 149 — appropriate per WORKFLOW.md round semantics; not scope creep.

### Hylla Feedback

None applicable. The change is fully content-local to two functions and one new const inside one file. The builder's `Read` of `service.go` was the correct evidence path; Hylla queries on `DefaultCodexDockerfile`/`DefaultClaudeDockerfile`/`recipeHash` would only have returned the same lines.

## Unit 12.1 — Round 1

**Verdict:** PASS
**Reviewer:** ta-go-qa-proof
**Reviewed at:** 2026-05-21
**Commit under review:** `b1b196e` (`feat(images): unit 12.1 overlay hash + canonical manifest`)

**Mage targets run:** `mage testPkg ./internal/services/images/` → PASS, **41/41 tests**, **80.4% coverage** (≥60% gate; above the ≥70% per-package floor PLAN.md targets).

### Acceptance proof

| # | Acceptance bullet | Evidence (file:line) | Verdict |
|---|---|---|---|
| 1 | `canonicalManifest(manifest tools.ToolManifest) []canonicalTool` — unexported, sorted-by-name, `strings.TrimSpace` applied exactly once to `Source` and `Install` (not `Version`); single trim point feeding `OverlayHash` (and Unit 12.2 downstream). | `overlay.go:28-45` (lowercase `canonicalManifest`, `sort.Slice` lines 41-43, TrimSpace at 37-38 on Source/Install only, Version untouched at 36). Tests: `overlay_test.go:167-197` (`TestCanonicalManifest_TrimAppliedOnce` — verifies both fields trimmed AND internal whitespace survives) + `overlay_test.go:199-221` (`TestCanonicalManifest_SortedByName` — five-tool alpha-sort assertion). | pass |
| 2 | `OverlayHash(manifest tools.ToolManifest) string` — exported, computes `sha256(json.MarshalIndent(canonicalManifest(manifest), "", ""))`, returns lowercase hex. | `overlay.go:53-64` — exported (capital `O`), calls `canonicalManifest` at 54, `json.MarshalIndent(canonical, "", "")` at 55, `sha256.Sum256(payload)` at 62, `hex.EncodeToString(sum[:])` at 63 (Go stdlib `hex.EncodeToString` emits lowercase). Asserted hex length 64 in `overlay_test.go:17-19,44-46,64-66`. | pass |
| 3 | `shortOverlayHash(hash string) string` — unexported, takes the hash (not manifest), returns first 12 chars with defensive short-input handling. | `overlay.go:70-75` (lowercase `shortOverlayHash`, parameter is `hash string` not the manifest, `len < 12` short-circuit at 71-73 returns input unchanged, `hash[:12]` at 74). Tests: `overlay_test.go:147-165` — full 64-char input → `"9c3a7b1e8d4f"`, short `"abc"` input → `"abc"` (no panic). | pass |
| 4 | Table-driven tests cover: empty manifest, single string-form tool, single object-form tool, three tools out-of-order (hash matches reorder), whitespace trim equivalence. | All five cases present as named tests (not literally in one `t.Run` table, but each case is a discrete `Test*` function — equivalent coverage): empty `overlay_test.go:10-33`; string-form `35-53`; object-form `55-75` (also asserts string-form ≠ object-form hashes); out-of-order three-tool `77-104` (asserts `hashA == hashB` across two map-literal declaration orders); whitespace trim `106-123` (asserts `OverlayHash(clean) == OverlayHash(padded)`). | pass |
| 5 | Hash-stability snapshot pins a hardcoded manifest → hardcoded hex `7471483e6f2f684fa1054cdbb127dd744f1970c9f3c896428c5ca632574571c5`. | `overlay_test.go:130-145` (`TestOverlayHash_StabilitySnapshot`) — manifest at 133-138 (`ta` + `jq`), `const want` at 139 matches the appendix-specified hex exactly, fatal diff on drift at 142-144. Verified the snapshot passes against current implementation via `mage testPkg`. | pass |
| 6 | No changes to `service.go`; no overlay-Dockerfile generation in this unit. | `git diff b1b196e~1..b1b196e --stat` shows only four files: `BUILDER_WORKLOG.md`, `PLAN.md`, `overlay.go` (new, +75), `overlay_test.go` (new, +221). `git diff b1b196e~1..b1b196e -- internal/services/images/service.go` returns empty. No `BuildOverlayDockerfile` or Dockerfile-template helper present in `overlay.go`. | pass |
| 7 | `mage testPkg ./internal/services/images/` green; coverage ≥ 60% gate (PLAN.md targets ≥70%). | Reran independently against HEAD: `[PKG PASS] github.com/evanmschultz/valv/internal/services/images (1.29s)`, 41 tests / 41 passed, coverage **80.4%**. Matches builder's reported 41/41 + 80.4% exactly. Materially above both the 60% gate and the 70% target. | pass |

### Notes / deviations

- **`canonicalManifest` empty-input branch returns `[]canonicalTool{}` not `nil`.** `overlay.go:29-31` returns the typed empty slice when `len(manifest.Tools) == 0`. `json.MarshalIndent` of `[]canonicalTool{}` produces `"[]"`, of `nil` would produce `"null"` — choosing the empty slice keeps the empty-manifest hash a stable sha256 of `"[]"`. Confirmed empty-manifest hash is stable across calls and across nil-map / empty-map (`overlay_test.go:21-32`). Acceptable refinement; consistent with PLAN.md acceptance bullet 1's "sorted-by-name slice" wording.
- **`OverlayHash` error branch returns empty string rather than panicking.** `overlay.go:56-61` — `json.MarshalIndent` cannot fail on a `[]canonicalTool` (all string fields), so the branch is unreachable in practice. Returning `""` keeps the function total; downstream hash-comparison would see a mismatch and force a rebuild (conservative-opposite policy, per PLAN.md decision 5). Not a blocking deviation.
- **Tests use one `Test*` function per case rather than a single table-driven `t.Run` loop.** The acceptance bullet phrasing "Table-driven test exercises" reads as one test with sub-cases. The builder instead used per-case named functions. Coverage of the required five cases is complete and each function uses table-equivalent setup. Mechanically equivalent; arguably more diagnostic (named-test failures point straight at the broken case). Accepting as functionally identical.
- **Coverage delta vs. ≥70% target.** 80.4% — comfortably above. New `overlay.go` adds well-tested code; the +0.7% delta over the pre-unit 79.7% (Unit 12.0) reflects the high test density of the new helpers.
- **Builder ran the test-first-pin pattern for the stability snapshot** (placeholder constant → run → capture hex → edit). Documented in BUILDER_WORKLOG.md design notes. Acceptable per PLAN.md decision 5 spirit (do not transcribe sha256 by hand).
- **PLAN.md unit-state flip from `in_progress` → `done` is included in the commit.** Per WORKFLOW.md round semantics; not scope creep.
- **No `service.go` overlay-Dockerfile changes.** Unit 12.2 is the next unit per PLAN.md line 209; the `canonicalManifest` exported-shape contract is preserved so 12.2 can consume it without re-trimming. Single-trim-point invariant is now testably enforced.

### Hylla Feedback

None. BUILDER_WORKLOG.md `## Hylla Feedback` for Unit 12.1 Round 2 records no fallback miss — the targeted `Read` of `service.go` (style/imports) and `tools.go` (`ToolManifest`/`ToolSpec` shape) was the fastest path for small known files. Confirmed during QA: no Hylla query was needed to verify the change set either.

## Unit 12.2 — Round 1

**Verdict:** PASS
**Reviewer:** ta-go-qa-proof
**Reviewed at:** 2026-05-21
**Commit under review:** `d819266` (`feat(images): unit 12.2 overlay dockerfile generator`)

**Mage targets run:** `mage testPkg ./internal/services/images/` → PASS, **48/48 tests**, **81.9% coverage** (≥60% gate; materially above the ≥70% per-package floor PLAN.md targets).

### Acceptance proof

| # | Acceptance bullet | Evidence (file:line) | Verdict |
|---|---|---|---|
| 1 | `BuildOverlayDockerfile(manifest tools.ToolManifest, baseImage docker.ImageRef) (string, error)` exact signature. | `overlay.go:95` — exported (capital `B`), parameter order and types match PLAN.md line 220 verbatim. Returns `(string, error)`. | pass |
| 2 | Consumes `canonicalManifest(manifest)` directly — NO `strings.TrimSpace` re-application inside `BuildOverlayDockerfile`. | `overlay.go:96` calls `canonicalManifest(manifest)` once; the function body (overlay.go:95-147) contains zero `strings.TrimSpace` calls — verified by reading every line. The `strings` import (overlay.go:9) is consumed only by `strings.Builder` here and by the existing `strings.TrimSpace` inside `canonicalManifest` (overlay.go:44-45). Single-trim invariant preserved per PLAN.md decision 3 + Unit 12.1 contract. | pass |
| 3 | Output structure: `FROM` (using `baseImage.String()`) → `USER root` → `ENV NPM_CONFIG_* + GOBIN=/usr/local/bin` block → one exec-form RUN per tool (sorted via canonicalManifest) → `USER valv`. | `overlay.go:99` (`FROM %s\n` with `baseImage.String()`); `overlay.go:101` (`USER root`); `overlay.go:103-106` (multi-line `ENV NPM_CONFIG_UPDATE_NOTIFIER=false \ ... GOBIN=/usr/local/bin`); loop at `overlay.go:109-142` emits one RUN per canonical-order tool; `overlay.go:145` (`USER valv`). `TestBuildOverlayDockerfile_ByteForByte` (`overlay_test.go:231-265`) pins exact bytes including blank-line separators. | pass |
| 4 | Exec-form JSON-array RUN lines emitted via `json.Marshal` on argv slice (not hand-constructed). | `overlay.go:137` — `encoded, err := json.Marshal(argv)`; `overlay.go:141` — `fmt.Fprintf(&buf, "RUN %s\n", encoded)`. Argv built as `[]string` at `overlay.go:127` (go install) and `overlay.go:129` (npm install -g). `TestBuildOverlayDockerfile_InjectionSafety` (`overlay_test.go:389-432`) round-trips RUN payload through `json.Unmarshal` and asserts the evil source `"github.com/x/y; rm -rf /"` survives as a single argv element — proves the encoding is structurally JSON-array, not hand-constructed string concatenation. | pass |
| 5 | Install vocabulary: `go install` → `RUN ["go", "install", "<source>"]`; `npm install -g` → `RUN ["npm", "install", "-g", "<source>"]`; any other → wrapped error. | `overlay.go:15-18` declares `installGoInstall = "go install"` and `installNpmInstall = "npm install -g"` constants. Switch at `overlay.go:125-132`: `case installGoInstall` → `[]string{"go", "install", tool.Source}`; `case installNpmInstall` → `[]string{"npm", "install", "-g", tool.Source}`; `default` returns wrapped error. Byte-for-byte snapshot at `overlay_test.go:256-257` confirms exact rendered form for both verbs. | pass |
| 6 | All four error paths emit `"build overlay dockerfile: tool %q ..."` wrapped errors. | `overlay.go:115` (string-form: `... uses unsupported string-form spec (v1 requires object form with source+install)`); `overlay.go:118` (`... has empty source`); `overlay.go:121` (`... has empty install`); `overlay.go:131` (`... has unsupported install verb %q`). All four use `fmt.Errorf("build overlay dockerfile: tool %q ...", tool.Name, ...)`. Order of checks (string-form first, then empty-source, then empty-install, then verb switch) ensures Version-only specs get the specific string-form error, not the generic empty-field one. | pass |
| 7 | Tests cover: byte-for-byte snapshot, sorting determinism, all four error paths, injection safety (round-trip parse). | (a) Snapshot: `TestBuildOverlayDockerfile_ByteForByte` `overlay_test.go:231-265`. (b) Sorting determinism: `TestBuildOverlayDockerfile_SortingDeterminism` `overlay_test.go:267-296` asserts `gotA == gotB` across two map-literal declaration orders. (c) String-form error: `TestBuildOverlayDockerfile_StringFormRejected` `overlay_test.go:298-316` with `wantPrefix` substring check. (d) Unsupported verb: `TestBuildOverlayDockerfile_UnsupportedInstallVerb` `overlay_test.go:318-342` asserts both `"unsupported install verb"`, the literal `"cargo install"` verb, and the tool name `"x"` appear in the error. (e) Empty source: `TestBuildOverlayDockerfile_EmptySource` `overlay_test.go:344-362` with `Source: "   "` (trimmed to empty post-canonicalManifest). (f) Empty install: `TestBuildOverlayDockerfile_EmptyInstall` `overlay_test.go:364-382` with `Install: "  "`. (g) Injection safety: `TestBuildOverlayDockerfile_InjectionSafety` `overlay_test.go:389-432` — parses RUN payload via `json.Unmarshal`, asserts argv length 3, asserts argv[2] == `evilSource` exactly. | pass |
| 8 | `mage testPkg ./internal/services/images/` green; coverage ≥ 60% gate (target 70%). | Reran against HEAD (`d819266`): `[PKG PASS] github.com/evanmschultz/valv/internal/services/images (1.25s)`, **48 tests / 48 passed**, coverage **81.9%**. Matches builder's reported figures exactly. Materially above the 60% gate and the 70% PLAN.md target. | pass |

### Notes / deviations

- **Empty-source detection works on trimmed values.** `canonicalManifest` (`overlay.go:35-52`) applies `strings.TrimSpace` to `Source` and `Install`. `TestBuildOverlayDockerfile_EmptySource` passes `Source: "   "` and `TestBuildOverlayDockerfile_EmptyInstall` passes `Install: "  "` — both whitespace-only — which become empty post-canonicalManifest and reach the respective error branches. This proves the trim-then-check chain works end-to-end and that the canonicalManifest trim semantics are load-bearing for the error contract, not just for hash stability.
- **Error-check ordering matters.** The string-form check (`Source == "" && Install == ""`) fires before the more-specific empty-source / empty-install checks, so a Version-only tool gets the dedicated string-form error rather than the generic `has empty source`. `TestBuildOverlayDockerfile_StringFormRejected` confirms by matching the longer `wantPrefix` substring. The ordering is documented at `overlay.go:110-113`.
- **`json.Marshal` error branch is unreachable but defensively wrapped.** `overlay.go:137-140` — marshalling `[]string` cannot fail in Go's stdlib (every element is a string with no non-marshalable types), but the error is wrapped with `%w` anyway. Acceptable defensive coding consistent with the builder worklog rationale (overlay.go:134-136 comment block).
- **`baseImage.String()` shape verified.** `docker.ImageRef.String()` at `internal/adapters/docker/types.go:22-31` returns `"<Repository>:<Tag>"` when both set, just `<Repository>` when tag empty, empty string when repository empty. The byte-for-byte snapshot uses `docker.NewImageRef("valv-claude", "dev")` → `"valv-claude:dev"`, matching the test's `FROM valv-claude:dev` line.
- **Two unexported `installXxx` constants** (`overlay.go:15-18`) are the install-verb vocabulary. A future v2 verb (`pip install --user`, `cargo install`, etc.) adds a const + case + tests with no other refactor.
- **`USER valv` restoration is load-bearing.** Both base Dockerfiles end at `USER valv` (`service.go:674`/`service.go:738` per Unit 12.0 review). The overlay's `USER root` would persist into runtime if the trailing `USER valv` were omitted. `overlay.go:145` + the byte-for-byte test guarantee it stays.
- **PLAN.md unit-state flip from `in_progress` → `done` is included in the commit.** Per WORKFLOW.md round semantics; not scope creep. The full diff under review (`git diff aacfbeb..HEAD -- internal/services/images/`) shows only the two source files plus the PLAN.md and BUILDER_WORKLOG.md updates.
- **Coverage delta vs. Unit 12.1.** 80.4% → 81.9%, +1.5%. New `BuildOverlayDockerfile` code is densely tested (7 new tests for ~50 new statements), pulling the package average up rather than down.
- **No `service.go` touch.** Confirmed via `git diff aacfbeb..HEAD --stat` — only `overlay.go` and `overlay_test.go` changed in `internal/services/images/`. Unit 12.3 (next) is the `service.go` extension per PLAN.md line 236.

### Hylla Feedback

None. BUILDER_WORKLOG.md `## Hylla Feedback` for Unit 12.2 records no fallback miss — the targeted `Read` of `docker/types.go` (for `ImageRef.String()` semantics) and the existing `overlay.go` was the fastest path. Confirmed during QA: no Hylla query was needed to verify the change set either; the source files are small and self-contained.

## Unit 12.3 — Round 1

**Verdict:** PASS
**Reviewer:** ta-go-qa-proof
**Reviewed at:** 2026-05-21
**Commit under review:** `ce259ea` (`feat(images): unit 12.3 ensureProjectImage + 5-label cache`)

**Mage targets run:** `mage testPkg ./internal/services/images/` → PASS, **61/61 tests**, **81.4% coverage** (≥60% gate; materially above the ≥70% per-package floor PLAN.md acceptance bullet 8 targets).

### Acceptance proof

| # | Acceptance bullet | Evidence (file:line) | Verdict |
|---|---|---|---|
| 1 | Six PLAN-named constants exist with exact names + values inside the existing top-of-file const block. | `service.go:34` (`tagPrefixProjectOverlay = "proj-"`), `service.go:35` (`toolsHashLabel = "io.valv.tools_hash"`), `service.go:36` (`baseRecipeHashLabel = "io.valv.base_recipe_hash"`), `service.go:37` (`managedLabel = "io.valv.managed"`), `service.go:38` (`scopeLabel = "io.valv.scope"`), `service.go:39` (`scopeValueProjectOverlay = "project-overlay"`). All six placed inside the pre-existing `const (...)` block at `service.go:26-40` — single source-of-truth for the label vocabulary. | pass |
| 2 | `EnsureProjectRequest{Manifest tools.ToolManifest; BaseImage docker.ImageRef; NoCache bool}` — **NO `Pull` field** (F3). | `service.go:155-159` — fields are exactly `Manifest tools.ToolManifest`, `BaseImage docker.ImageRef`, `NoCache bool`. Doc comment at 149-154 explicitly cites the deliberate F3 absence of `Pull`. Grepped the struct for `Pull` — absent. | pass |
| 3 | `EnsureProjectResult{Image docker.ImageRef; Action EnsureAction; ToolsHash string}` — **NO `BaseRecipeHash` field** (PLAN.md L248 YAGNI). | `service.go:167-171` — fields are exactly `Image docker.ImageRef`, `Action EnsureAction`, `ToolsHash string`. Doc comment at 161-166 cites the YAGNI dropped field. Grepped the struct for `BaseRecipeHash` — absent. | pass |
| 4a | `EnsureProjectImage` empty-manifest short-circuit returning `{Image: BaseImage, Action: EnsureActionUsingExistingImage}` with zero docker calls. | `service.go:721-726`. Test `TestEnsureProjectImage_EmptyManifestShortCircuits` `service_test.go:1224-1247` asserts `result.Image == baseRef`, `result.Action == EnsureActionUsingExistingImage`, AND `len(runner.calls) == 0`. | pass |
| 4b | Computes `toolsHash := OverlayHash(request.Manifest)`. | `service.go:734` — `toolsHash := OverlayHash(request.Manifest)`. Calls Unit 12.1's exported `OverlayHash` at `overlay.go:60` — no re-hashing inside `EnsureProjectImage`. | pass |
| 4c | Reads `baseRecipeHash` via `inspectLabel` VERBATIM (PLAN.md decision 4 + Notes L312 — no re-hash). | `service.go:749` — `baseRecipeHash, baseErr := s.inspectLabel(ctx, request.BaseImage, recipeHashLabel)`. Value flows untouched into `buildRequest.Labels[baseRecipeHashLabel] = baseRecipeHash` at `service.go:801`. No `sha256Hex(baseRecipeHash)` or `sha256.Sum256([]byte(baseRecipeHash))` call anywhere. | pass |
| 4d | Constructs target tag via `projectImageRef(toolsHash)` returning `<repo>:proj-<short>`. | `service.go:761` — `targetRef := s.projectImageRef(toolsHash)`. Helper at `service.go:659-661` — `tagPrefixProjectOverlay + shortOverlayHash(toolsHash)`. Test `TestProjectImageRef_TagFormat` `service_test.go:1653-1670` pins exact 17-char output `proj-9c3a7b1e8d4f` for a 64-char input. | pass |
| 4e | Inspects target tag for all three freshness labels. | `service.go:842-869` (`projectImageNeedsBuild`) — reads `recipeHashLabel` (line 845), `toolsHashLabel` (line 853), `baseRecipeHashLabel` (line 861) sequentially. All three label probes assert exact match with the in-memory expected values; any mismatch returns `true` (rebuild). | pass |
| 4f | Rebuilds on ANY read failure — typecast (errLabelUnreadable) OR non-missing inspect error. | `service.go:846-848,854-856,862-864` — each of the three `inspectLabel` calls inside `projectImageNeedsBuild` returns true unconditionally on `err != nil`. Tests cover both failure modes: `TestEnsureProjectImage_TypecastFailureForcesRebuild` `service_test.go:1464-1502` (typecast via `nonOutputRunner`); `TestEnsureProjectImage_InspectErrorForcesRebuild` `service_test.go:1504-1533` (non-missing inspect error `"dockerd is not responding"`). | pass |
| 4g | Rebuild path generates overlay via `BuildOverlayDockerfile`, writes temp dir, invokes `docker.BuildImageArgs` with FIVE labels. | `service.go:735` (`BuildOverlayDockerfile`), `service.go:783` (`os.MkdirTemp("", "valv-overlay-*")`), `service.go:787` (`defer os.RemoveAll(tempDir)`), `service.go:790-792` (`os.WriteFile(dockerfilePath, ...)`), `service.go:799-805` (Labels map carries `recipeHashLabel`, `baseRecipeHashLabel`, `toolsHashLabel`, `managedLabel="true"`, `scopeLabel=scopeValueProjectOverlay`). `service.go:808` (`docker.BuildImageArgs(buildRequest)`). Test `TestEnsureProjectImage_TargetMissingTriggersBuild` `service_test.go:1249-1337` asserts all five labels land on the build args + the `-t <wantTag>` flag. | pass |
| 5 | `inspectLabel(ctx, ref, label) (string, error)` exists; typecast failure returns `errLabelUnreadable` sentinel; `dockerImageMissingError` wrapped through. | `service.go:680-693`. Typecast check at 681-684 returns `errLabelUnreadable` sentinel (declared at `service.go:48`). `dockerImageMissingError(err)` branch at 687-689 wraps and returns. Test exercises typecast path via `nonOutputRunner` — and base-image-missing path via `TestEnsureProjectImage_BaseImageMissingReturnsError` `service_test.go:1595-1618`. | pass |
| 6 | Table-driven test coverage spans all eight required scenarios. | (a) empty manifest skip → `TestEnsureProjectImage_EmptyManifestShortCircuits` `service_test.go:1224-1247`. (b) target missing builds → `TestEnsureProjectImage_TargetMissingTriggersBuild` `service_test.go:1249-1337` (uses `dockerImageMissingError`-matching `"no such image"`). (c) all-match no-rebuild → `TestEnsureProjectImage_AllLabelsMatchSkipsBuild` `service_test.go:1339-1383`. (d) tools_hash mismatch → `TestEnsureProjectImage_FreshnessMismatchTriggersRebuild` sub-test `tools_hash_mismatch` `service_test.go:1407`. (e) base_recipe_hash mismatch → sub-test `base_recipe_hash_mismatch` `service_test.go:1408`. (f) recipe_hash mismatch → sub-test `recipe_hash_mismatch` `service_test.go:1406`. (g) typecast failure rebuild → `TestEnsureProjectImage_TypecastFailureForcesRebuild` `service_test.go:1464-1502`. (h) inspect error (non-missing) rebuild → `TestEnsureProjectImage_InspectErrorForcesRebuild` `service_test.go:1504-1533`. | pass |
| 7 | Test asserts ALL FIVE labels on built image. | `service_test.go:1296-1324` — `wantLabels[]` carries `baseRecipeHashLabel=base-recipe-sha256-hex`, `managedLabel=true`, `recipeHashLabel=<sha256Hex(overlayContent)>`, `scopeLabel=project-overlay`, `toolsHashLabel=<OverlayHash(manifest)>`. Loop at 1313-1324 scans the build args for `--label <expected-pair>` substrings and `t.Errorf`s on any missing label. Five-label set verified. | pass |
| 8 | `mage testPkg ./internal/services/images/` green; coverage ≥60% gate (PLAN.md targets ≥70%). | Reran independently against HEAD (`ce259ea`): `[PKG PASS] github.com/evanmschultz/valv/internal/services/images (1.27s)`, **61 tests / 61 passed**, coverage **81.4%**. Matches builder's reported 61/61 + 81.4% exactly. Materially above the 60% gate and 70% PLAN.md target. | pass |

### Deviation audit

- **`projectImageNeedsBuild` intermediate helper.** Not named in PLAN.md acceptance but introduced by the builder to keep `EnsureProjectImage` readable. `service.go:842-870` — pure-function-of-expectations style, returns `bool` only (no error channel, per builder design note #5 in BUILDER_WORKLOG.md L165). Swallows inspect errors → `return true` (rebuild) by intent. Verified the helper does not hide a bug: the three label probes are in canonical order (recipe → tools → base), each compares against the pre-computed expected value, and short-circuits to rebuild on first mismatch. No state shared between the helper and the caller; the caller still controls all docker-side effects. **Acceptable design call.**

- **`sha256Hex` helper (CRITICAL audit).** New helper at `service.go:875-878`. Total callsites via grep:
  - `service.go:739` — `expectedRecipeHash := sha256Hex(dockerfileContent)` — hashes the OVERLAY Dockerfile bytes (correct: the overlay `recipe_hash` label is sha256-of-overlay-content per PLAN.md decision 4, mirrors base `recipeHash()` at `service.go:586-588`).
  - `service_test.go:1311,1350,1396,1546` — test-side uses, all on `overlayContent` returned by `BuildOverlayDockerfile`.
  - **Zero callsites apply `sha256Hex` to `baseRecipeHash`.** Trace at `service.go:749 → 757 → 801` confirms `baseRecipeHash` flows verbatim from `inspectLabel` → into the `Labels` map without any hashing call. **The PLAN.md decision 4 + Notes L312 "DO NOT call sha256 on it again" invariant holds.** This was the central risk — verified clean.

- **Legacy buildx-unavailable fallback.** `service.go:813-822` mirrors the existing `Service.Build` pattern at `service.go:389-401` exactly — same `isBuildxUnavailable(err)` detector (defined at `service.go:1048-1055`), same `buildRequest.Builder = "legacy"` flip, same re-call. Not a fabrication: `Service.Build` already treats this as canonical, and skipping it would mean per-project overlays fail on hosts where the base build succeeds. Builder rationale documented in BUILDER_WORKLOG.md L164. **Symmetric with existing pattern, not a deviation.**

- **Up-front overlay generation (before docker calls).** `service.go:735-738` generates the overlay dockerfile content via `BuildOverlayDockerfile(...)` BEFORE the freshness probe. Two reasons documented in BUILDER_WORKLOG.md L162: (1) manifest-shape errors (string-form / unsupported verb / empty source) surface before docker is touched, so a caller with malformed `tools.toml` never gets a half-built image; (2) the freshness comparison and the rebuild write use the SAME content bytes, so "cache says matches but rebuild would have produced different content" is impossible by construction. `TestEnsureProjectImage_OverlayGeneratorErrorWraps` `service_test.go:1620-1651` confirms the error-before-docker contract (asserts `len(runner.calls) == 0` when overlay gen fails). **Reasonable design call; arguably more correct than rebuild-branch-only generation.**

### Hylla Feedback

None. Builder's BUILDER_WORKLOG.md `## Hylla Feedback` for Unit 12.3 records no fallback miss — every read was inside the active drop scope (`service.go`, `service_test.go`, `overlay.go`, `internal/adapters/docker/ops.go`, `internal/adapters/docker/types.go`, `internal/tools/tools.go`). Confirmed during QA: targeted `Read` + LSP would have been equivalent; no Hylla query was needed to verify the change set.

### Notes / non-blocking observations

- **Goroutine / context discipline.** `EnsureProjectImage` accepts `ctx context.Context` as first param (`service.go:718`) and threads it through every `s.runner.Run(ctx, ...)` and `s.runner.Output(ctx, ...)` call. No goroutines spawned, no `select` on `ctx.Done()` needed — synchronous flow. `defer os.RemoveAll(tempDir)` runs on every exit path including the legacy-fallback error branches, so no temp-dir leaks on failure.
- **Error wrapping.** All `fmt.Errorf` boundary additions use `%w` for the underlying error: `service.go:737` (overlay gen), 752 (base missing), 755 (base inspect), 785 (mkdir), 791 (write), 810 (BuildImageArgs), 818/821 (legacy fallback), 824 (primary build). The pattern matches the package's existing convention from `Service.Build`.
- **`projectImageRef` truncation contract.** `service.go:659-661` calls `shortOverlayHash(toolsHash)` from Unit 12.1 (`overlay.go:73-77`). `TestProjectImageRef_TagFormat` `service_test.go:1653-1670` asserts the 12-char truncation against a known 64-char input. Tag length 17 chars (`proj-` + 12) well within Docker's 128-char tag-grammar limit per PLAN.md Notes L313.
- **`projectImageNeedsBuild` argument order:** `(ctx, targetRef, expectedRecipeHash, expectedToolsHash, expectedBaseRecipeHash)` — recipe before tools before base. Probe order matches argument order, so any reordering at the call site (`service.go:768`) would be a compile error rather than silent label-mismatch. Defensive ordering.
- **`errLabelUnreadable` tolerance in base-image inspect.** `service.go:754-758` — when `inspectLabel` returns `errLabelUnreadable` for the BASE image (not for the target tag), `baseRecipeHash` is set to `""` and execution continues into the rebuild path. The empty value lands as `baseRecipeHashLabel=""` on the new overlay; the next call (with a working `outputRunner`) sees `"" != expectedBaseRecipe` → mismatch → rebuild. Safe-but-wasteful: one extra rebuild on the first launch after the runner gains Output capability. Builder rationale in BUILDER_WORKLOG.md L167.
- **PLAN.md unit-state flip from `in_progress` → `done` is included in the commit.** Per WORKFLOW.md round semantics; not scope creep. The diff under review (`git diff 8419dab..HEAD -- internal/services/images/`) shows only `service.go` and `service_test.go` changed (775 LOC added).

## Unit 12.3 — Round 2

**Verdict:** PASS

**Mage targets run:** `mage testPkg ./internal/services/images/` → `[PKG PASS] github.com/evanmschultz/valv/internal/services/images (1.30s)`, **62 tests / 62 passed**, package coverage **81.6%** (above 60% gate and 70% PLAN.md target).

### Round 1 blocker resolution

- **Blocker:** Round 1 falsification identified that at `internal/services/images/service.go` Step 2b, the `if !errors.Is(baseErr, errLabelUnreadable) { return ... }` carve-out caused generic non-missing base-inspect errors (e.g. `"permission denied"`, `"dockerd is not responding"`) to abort `EnsureProjectImage` with `"ensure project image: inspect base recipe hash: ..."` instead of falling through to the rebuild path. Only the typecast sentinel was tolerated.
- **Fix:** `internal/services/images/service.go:756-763` (commit `d85e4a8` — `fix(images): unit 12.3 broaden base-inspect error tolerance`). The `errors.Is(baseErr, errLabelUnreadable)` branch is **removed entirely**. The error-handling block now reduces to: (a) `dockerImageMissingError(baseErr)` → fatal with `"base image %q missing"` (preserved), (b) any other non-nil `baseErr` → `s.debug(...)` log + `baseRecipeHash = ""` + fall through. Verified via `git diff ce259ea..HEAD -- internal/services/images/service.go` (12 +, 7 -, single function body).
- **Regression test:** `TestEnsureProjectImage_GenericBaseInspectErrorFallsThroughToRebuild` at `internal/services/images/service_test.go:1672-1741` (71 lines, +1 to test count: 61 → 62). Uses `runnerRecorder.errs[projectInspectKey(baseRef.String(), recipeHashLabel)] = fmt.Errorf("permission denied")` to force the generic non-missing branch on the BASE image, then asserts: (a) `err == nil`, (b) `result.Action == EnsureActionUpdated`, (c) the recorded `buildx build` call carries `--label io.valv.base_recipe_hash=` (trailing equals, empty value) confirming the empty sentinel propagates verbatim onto the new overlay image.

### Round 2 verification

| Item | Evidence | Verdict |
|---|---|---|
| 1. Step 2b generic-error fallthrough applied | `service.go:756-763` — `if dockerImageMissingError(baseErr) { return ... }; s.debug("base recipe hash unreadable; falling through to rebuild", ...); baseRecipeHash = ""` | pass |
| 2. `dockerImageMissingError` on base image still fatal | `service.go:758-760` — `return EnsureProjectResult{}, fmt.Errorf("ensure project image: base image %q missing: %w", request.BaseImage.String(), baseErr)`. Error string `"base image %q missing"` matches appendix spec verbatim. | pass |
| 3. `errors.Is(baseErr, errLabelUnreadable)` carve-out removed | `git diff ce259ea..HEAD -- internal/services/images/service.go` shows the 2-line `if !errors.Is(baseErr, errLabelUnreadable) { return ..., fmt.Errorf("ensure project image: inspect base recipe hash: %w", baseErr) }` block deleted; replaced with single-line `s.debug(...)`. Typecast and generic errors now traverse the identical fall-through path. | pass |
| 4. Regression test present | `service_test.go:1672-1741` — `TestEnsureProjectImage_GenericBaseInspectErrorFallsThroughToRebuild`. Header doc-comment explicitly cites PLAN.md decision 5 Round 2 clarification. | pass |
| 5. New test asserts no error + `EnsureActionUpdated` + empty `base_recipe_hash` label on build call | `service_test.go:1705-1740` — three assertion blocks: (a) `t.Fatalf` on non-nil `err`, (b) `t.Fatalf` on `result.Action != EnsureActionUpdated`, (c) loops `buildCall` searching for `--label` followed by `wantBaseLabel = fmt.Sprintf("%s=", baseRecipeHashLabel)`. Empty-trailing-equals matches the live label-formatter contract from `internal/adapters/docker/ops.go` `BuildImageArgs`. | pass |
| 6. PLAN.md decision 5 updated wording is consistent with new behavior | `PLAN.md:62` — "When `s.runner` does not implement `outputRunner` (typecast failure → `errLabelUnreadable`), OR when `docker image inspect` returns ANY non-missing error, `EnsureProjectImage` treats it as a label-read miss and falls through to the rebuild path with the unreadable label populated as `""`." Decision 5 explicitly enumerates BOTH the typecast and generic-error branches as a single equivalence class. "The one exception: `dockerImageMissingError` on the BASE image read remains fatal" matches `service.go:758-760` verbatim. | pass |
| 7. All Round 1 acceptance still holds | `mage testPkg` output shows 62/62 passed — the original 61 tests from Round 1 plus the new regression. No deletions or modifications to the existing 61 test bodies in the Round 2 diff (verified `git diff ce259ea..HEAD -- internal/services/images/service_test.go` — pure 71-line append at EOF starting after the closing `}` of `TestProjectImageRef_TagFormat`). | pass |
| 8. `mage testPkg ./internal/services/images/` green; coverage ≥60% gate | Run reproduced: `[PKG PASS] github.com/evanmschultz/valv/internal/services/images (1.30s)`, **62 tests / 62 passed**, coverage **81.6%**. Coverage went from 81.4% (Round 1) → 81.6% (Round 2) — the 71-line regression test exercised previously-uncovered branches in the rebuild-on-generic-base-error path. | pass |

### Notes / non-blocking observations

- **Symmetry argument is sound.** The target-tag freshness probe at `projectImageNeedsBuild` (`service.go:842-867`) already swallows ALL inspect errors and returns `true` (rebuild). The Round 2 fix brings the base-image inspect path into structural symmetry with that existing pattern — missing → fatal, everything-else → fall-through — closing the asymmetry that Round 1 falsification flagged. BUILDER_WORKLOG.md L206 captures this rationale.
- **Daemon-transient-failure recovery property gained.** Before Round 2: a transient `dockerd not responding` on base inspect cascaded into a CLI launch failure for the user. After Round 2: same scenario logs to debug and produces an overlay with empty `base_recipe_hash`; next healthy launch sees the mismatch and rebuilds cleanly. One extra rebuild on the next healthy launch is the cost — same trade-off PLAN.md decision 5 already accepted for the target-tag probe.
- **Assertion choice locks the wire format.** The test asserts the `--label io.valv.base_recipe_hash=` token appears in the recorded `buildx build` arg list (not just that an empty value flows into the `Labels` map). This catches a hypothetical future refactor that decides to skip empty-value labels at the `BuildImageArgs` level — that refactor would silently drop the empty sentinel from the wire and break the eventual-consistency property. Defensive coverage.
- **`errLabelUnreadable` still defined and still used elsewhere.** Round 2 only removed the `errors.Is` call at the base-inspect site; the sentinel is still produced by `inspectLabel` when the runner typecast fails and is still used elsewhere (e.g. the target-tag probe path). No dead code introduced. The Round 1 QA note at L255 about `errLabelUnreadable` tolerance now applies to ALL non-missing base-inspect errors, not just the typecast sentinel — superseded but not contradicted.
- **Diff footprint matches builder claim.** `git diff ce259ea..HEAD -- internal/services/images/` totals 12 + / 7 - in `service.go` (function-body + comment-block change) and 71 + / 0 - in `service_test.go` (pure-append regression test). No drift outside the two named files.
- **Round 1 QA Proof verdict ("PASS") is not invalidated.** Round 1 proof verified evidence completeness for the Round 1 code, and that code still exists at HEAD with the Step 2b clarification layered on top. The Round 1 findings remain accurate for the un-touched code paths (rebuild path, freshness probe, label set, `sha256Hex` callsite audit, legacy-buildx fallback, up-front overlay generation). Round 2 only touches the single error-handling block.

## Unit 12.4 — Round 1

**Verdict:** PASS

**Mage targets run:** `mage testPkg ./internal/cli/` → 234/234 tests passed, coverage **67.6%** (PLAN.md acceptance bullet 9 floor: "does not regress below 67.6%" — held at exactly the pinned floor).

### Acceptance proof

| # | Bullet (PLAN.md lines 286-304) | Evidence | Verdict |
|---|---|---|---|
| 1 | `resolveProjectImage(cmd, paths, provider, workingDir, baseRef)` exists with empty-manifest short-circuit | `operator_helpers.go:440-447` — signature matches PLAN.md L286 verbatim; `manifest, err := tools.Resolve(workingDir)`; `if len(manifest.Tools) == 0 { return baseRef, nil }`. No docker calls before the manifest check. | pass |
| 1a | `tools.Resolve` error other than empty-manifest is wrapped and returned | `operator_helpers.go:441-444` — `if err != nil { return dockeradapter.ImageRef{}, fmt.Errorf("resolve project image: %w", err) }`. Uses `%w` per Go error-wrapping rule. | pass |
| 1b | Env-override path emits stderr warning + returns `baseRef` | `operator_helpers.go:449-454` — `if envName := projectImageOverrideEnvName(provider); envName != "" { if strings.TrimSpace(os.Getenv(envName)) != "" { fmt.Fprintln(cmd.ErrOrStderr(), "warning: "+envName+" override active; .valv/tools.toml overlay skipped"); return baseRef, nil } }`. Warning string matches PLAN.md L288 verbatim. `fmt.Fprintln` writes to `cmd.ErrOrStderr()` as required. | pass |
| 1c | Otherwise opens images service + calls `EnsureProjectImage` + returns `result.Image` | `operator_helpers.go:456-469` — `service, closeImages, err := openImagesService(cmd, paths, provider)`; `defer closeImages()`; `result, err := service.EnsureProjectImage(cmd.Context(), imagesservice.EnsureProjectRequest{Manifest: manifest, BaseImage: baseRef})`; `return result.Image, nil`. EnsureProjectImage error path also wrapped at L466-468. | pass |
| 2 | `runClaudeCommand` constructor reorder applied (5-step) | `claude.go:111-132` — sequence is: `ensureClaudeImageCurrent` (L111-113, moved up from old L121) → `resolveProjectImage(..., domain.ProviderClaude, workingDir, claudeImageRef())` (L115-118) → `claudeservice.New(claudeservice.Options{... Image: projectImage ...})` (L120-132, moved down). The diff `git diff c4bb49b..HEAD -- internal/cli/claude.go` confirms `Image: claudeImageRef()` deleted, `Image: projectImage` inserted, and the post-construction `ensureClaudeImageCurrent` block deleted at old L116-121. `defer store.Close()` retained immediately after `openStore` (claude.go:101). | pass |
| 3 | `runCodexCommand` mirror reorder applied | `codex.go:116-137` — sequence: `ensureCodexImageCurrent` (L116-118) → `resolveProjectImage(..., domain.ProviderCodex, workingDir, codexImageRef())` (L120-123) → `codexservice.New(codexservice.Options{... Image: projectImage ...})` (L125-137). Diff confirms `Image: codexImageRef()` → `Image: projectImage` at L128 and the old post-constructor `ensureCodexImageCurrent` block deleted from old L123-129. | pass |
| 4 | `runClaudeImageOnlyCommand` + `runCodexImageOnlyCommand` UNCHANGED | `git diff c4bb49b..HEAD -- internal/cli/claude.go internal/cli/codex.go` shows two hunks in claude.go (L100, L116 ranges) and two hunks in codex.go (L107, L123 ranges); image-only commands at `claude.go:143-` and `codex.go:148-` fall outside every hunk. Both functions still call `ensureClaudeImageCurrent(cmd, paths)` / `ensureCodexImageCurrent(cmd, paths)` followed by `image := claudeImageRef()` / `image := codexImageRef()` directly — no `resolveProjectImage` call inserted. Acceptance bullet 4 (PLAN.md L298) satisfied. | pass |
| 5a | Test table — empty manifest case (claude) | `claude_project_image_test.go:20-51` — `TestResolveProjectImageClaudeEmptyManifestReturnsBaseRef`. Asserts `got.String() == baseRef.String()`, `stderr.Len() == 0`, and `os.ReadFile(logPath)` returns no data (no docker calls). Pins PLAN.md L300. | pass |
| 5b | Test table — non-empty + no override (claude) | `claude_project_image_test.go:57-112` — `TestResolveProjectImageClaudeOverlayBuildInvoked`. Asserts `got.String() != baseRef.String()`, `got.Tag` has `"proj-"` prefix (matches Unit 12.3's `ProjectImageRef` tag format), `got.Repository == baseRef.Repository`, stderr empty, docker log contains `"buildx build --load"` + labels `io.valv.managed=true`, `io.valv.scope=project-overlay`, `io.valv.tools_hash=`. Pins PLAN.md L301. | pass |
| 5c | Test table — non-empty + env-override (claude) | `claude_project_image_test.go:118-154` — `TestResolveProjectImageClaudeOverrideSkipsOverlay`. `t.Setenv("VALV_CLAUDE_IMAGE", "test/claude:override")`. Asserts `got.String() == baseRef.String()`, exact warning string `"warning: VALV_CLAUDE_IMAGE override active; .valv/tools.toml overlay skipped"` appears in stderr exactly once (`strings.Count(...) == 1`), and zero docker calls. Pins PLAN.md L302. | pass |
| 5d | Test table — empty manifest case (codex) | `codex_project_image_test.go:19-47` — `TestResolveProjectImageCodexEmptyManifestReturnsBaseRef`. Mirror assertions of 5a with `codexImageRef()` + `domain.ProviderCodex`. | pass |
| 5e | Test table — non-empty + no override (codex) | `codex_project_image_test.go:53-98` — `TestResolveProjectImageCodexOverlayBuildInvoked`. Mirror of 5b — proj-prefix tag, repo match, buildx label assertions. | pass |
| 5f | Test table — non-empty + env-override (codex) | `codex_project_image_test.go:103-137` — `TestResolveProjectImageCodexOverrideSkipsOverlay`. Warning string `"warning: VALV_CODEX_IMAGE override active; .valv/tools.toml overlay skipped"`, count exactly 1, zero docker calls. | pass |
| 6 | Tests use `installFakeDocker` + real `.valv/tools.toml` fixtures | `claude_project_image_test.go:24` + `:69` + `:123` all call `installFakeDocker(t)`; `writeClaudeToolsManifest(t)` at `:160-176` writes a real object-form `.valv/tools.toml` to a `t.TempDir()` (single object-form go-install tool — same shape Unit 12.2 `BuildOverlayDockerfile` accepts). Mirror helpers in `codex_project_image_test.go:142-158`. PLAN.md L303 spec met. | pass |
| 7 | `mage testPkg ./internal/cli/` green; coverage does not regress below 67.6% | Output reproduced: `[PKG PASS] github.com/evanmschultz/valv/internal/cli (5.82s)` / `234 tests passed across 1 package` / coverage line `github.com/evanmschultz/valv/internal/cli | 67.6%` / `[SUCCESS] Coverage threshold met`. Held at exactly the pinned floor (no regression). | pass |
| 8 | Diff scope = exactly the 5 PLAN.md-named files | `git diff c4bb49b..HEAD --stat -- internal/cli/` reports `claude.go (+18 -6)`, `codex.go (+16 -7)`, `operator_helpers.go (+67 -0)`, `claude_project_image_test.go (new, 176 lines)`, `codex_project_image_test.go (new, 158 lines)`. Matches PLAN.md "Paths" L277-281 with no drift. | pass |

### Notes / non-blocking observations

- **Constructor reorder rationale captured in code comments.** `claude.go:103-110` and `codex.go:110-115` both carry a block-comment explaining the DROP_12 Unit 12.4 reorder and why `claudeservice.New` / `codexservice.New` could not simply have `Image` patched post-construction (the constructor validates the field). This anchors the load-bearing change in the source so future readers do not "tidy" the order back.
- **Image-only command safety confirmed by two independent signals.** (1) Diff hunks in `claude.go` / `codex.go` cover only the `runClaudeCommand` / `runCodexCommand` line ranges, not the image-only ranges. (2) Both image-only functions still resolve their image via direct `claudeImageRef()` / `codexImageRef()` calls (`claude.go:153`, `codex.go:158`) — no per-project resolution leaked in. PLAN.md decision to leave image-only paths alone is preserved.
- **Override semantics preserved.** The override path returns `baseRef` (which `claudeImageRef()` / `codexImageRef()` already resolved from `VALV_*_IMAGE` upstream). The on-wire image ref is therefore identical to the pre-DROP_12 override behavior; only the new stderr warning is added. BUILDER_WORKLOG L254 explains this — verified at `claude.go:115` and `codex.go:120` where `baseRef` is the literal `claudeImageRef()`/`codexImageRef()` return.
- **`domain.Provider` typed dispatch.** `projectImageOverrideEnvName` at `operator_helpers.go:410-419` uses a typed `switch` on `domain.Provider` rather than a string — catches typos at compile time and matches `openImagesService`'s signature at the same call site. The `""` fallback for unknown providers is defensive: if a future drop introduces a third provider, the env-override branch is skipped until `projectImageOverrideEnvName` is extended.
- **Test fixture decision.** Both test files write a real minimal `.valv/tools.toml` (`ta = { source = "github.com/evanmschultz/ta@main", install = "go install" }`) into a `t.TempDir()` rather than mocking `tools.Resolve`. This exercises the real loader, validator, and canonical-manifest path through `OverlayHash` — load-bearing because the downstream overlay-build assertion relies on Unit 12.2's `BuildOverlayDockerfile` accepting object-form tools.
- **Empty-manifest short-circuit ordering verified.** `operator_helpers.go:445-447` runs BEFORE the env-name check at L449-454. This matches PLAN.md decision 13's spec that the warning fires only when manifest is non-empty AND env override is set (not when override is set with no manifest — the normal pre-DROP_12 case). BUILDER_WORKLOG L253 captures this rationale.
- **Coverage held at floor, not raised.** PLAN.md acceptance bullet 9 says "does not regress below 67.6%" — the new helper + 6 tests add ~30 LOC + ~334 LOC test but the reorder shifted existing lines around without adding net-new covered statements in the runCommand bodies. DROP_17 owns the coverage-floor bump from 60% to 70%; Unit 12.4 explicitly did not target it. Acceptance met.
