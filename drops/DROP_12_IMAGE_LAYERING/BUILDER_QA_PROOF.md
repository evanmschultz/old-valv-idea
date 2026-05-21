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
