# DROP_12 Build QA Falsification

## Unit 12.0 — Round 1

**Verdict:** pass
**Reviewer:** ta-go-qa-falsification
**Reviewed at:** 2026-05-21T20:56:10Z

### Counterexamples / Attacks

#### A1 — `ARG TARGETARCH` position (MITIGATED)

**Construction:** Per Docker BuildKit docs (confirmed via Context7 `/docker/docs`, sources `_vendor/github.com/moby/buildkit/frontend/dockerfile/docs/reference.md` and `content/manuals/build/building/variables.md`), automatic platform ARGs live in **global scope only** and are NOT injected into build stages. The stage must redeclare `ARG TARGETARCH` (no default value) before consuming `${TARGETARCH}`.

**Inspection:** `service.go:657` declares `ARG TARGETARCH` as the first line of `goInstallStep`. The constant is concatenated into both Dockerfiles AFTER `FROM node:22-bookworm-slim` (line 671/735) and BEFORE the RUN consuming `${TARGETARCH}`. Test `TestDefaultProviderDockerfilesEmbedGoToolchain` (service_test.go new test) explicitly checks `fromIdx < argIdx < tarballIdx`.

**Disposition:** Mitigated. Position is correct and asserted.

#### A2 — `case ${TARGETARCH}` shell semantics in dash (MITIGATED)

**Construction:** `node:22-bookworm-slim` is Debian-based; `/bin/sh` symlinks to dash. Verified empirically that the exact case block works in dash with `set -eu`: `set -eu; case "${ARCH}" in amd64) X=foo ;; arm64) X=bar ;; *) exit 1 ;; esac` correctly assigns and persists `X` outside the case block; unmatched arch exits 1; empty/unset `${TARGETARCH}` under `set -u` triggers "parameter not set" (exit 2) before reaching the `*)` branch — that is, a forgotten ARG redeclaration would fail loud.

**Disposition:** Mitigated. dash-compatible syntax; multi-mode failure surface (set -u catches unset, *) catches unknown).

#### A3 — `sha256sum -c` two-space delimiter (MITIGATED)

**Construction:** GNU coreutils `sha256sum -c` requires exactly TWO spaces between hash and filename. Single space → "no properly formatted SHA checksum lines found" (exit 1). Verified empirically via `shasum -a 256 -c -` on macOS (BSD coreutils-compatible) — single-space rejected, two-space accepted.

**Inspection:** `service.go:665` — `echo "${GO_SHA256}  /tmp/go.tar.gz" | sha256sum -c -`. Between `${GO_SHA256}` and `/tmp/go.tar.gz` are EXACTLY two ASCII spaces (verified by reading the raw source line and the diff context, no tabs, no other whitespace).

**Disposition:** Mitigated. Two-space format honored.

#### A4 — sha256 hex value legitimacy (MITIGATED, EMPIRICALLY VERIFIED)

**Construction:** Fetched `https://go.dev/dl/?mode=json&include=all`, queried via `jq` for `version=="go1.26.1"`, filtered linux + (amd64|arm64). Authoritative go.dev release index returns:
- amd64 `go1.26.1.linux-amd64.tar.gz` → `031f088e5d955bab8657ede27ad4e3bc5b7c1ba281f05f245bcc304f327c987a`
- arm64 `go1.26.1.linux-arm64.tar.gz` → `a290581cfe4fe28ddd737dde3095f3dbeb7f2e4065cab4eae44dfc53b760c2f7`

Both values match the embedded constants at `service.go:660-661` byte-for-byte (lowercase hex). The `.sha256` URL endpoints both return HTML redirects (verified via `curl -fsSL https://go.dev/dl/go1.26.1.linux-amd64.tar.gz.sha256` returning a `<meta http-equiv="refresh">` page) — Builder's note that the JSON index is the authoritative machine-readable source is correct.

**Disposition:** Mitigated. Hashes verified against the official go.dev JSON release index 2026-05-21.

#### A5 — Missing `recipeHash` baseline pin (CONFIRMED GAP vs PLAN, NON-BLOCKING)

**Construction:** PLAN.md Unit 12.0 acceptance line 173 explicitly requires: *"Tests assert: `recipeHash()` of the new template produces a different value than the pre-Unit-12.0 baseline — pin the baseline via a hardcoded sha256 string updated in this unit's commit."* Builder skipped the pin (worklog § "No recipeHash() baseline pin added").

**Counterexample reproducing the gap:** Suppose a future drop adds a no-op trailing comment line `# rebuild cache` to `goInstallStep`. The change:
1. Modifies `goInstallStep` byte content.
2. Causes `recipeHash() = sha256(DefaultXxxDockerfile())` to change → every operator's existing base image gets rebuilt at next launch (potential surprise reflow).
3. PASSES every existing test:
   - `TestDefaultProviderDockerfilesEmbedGoToolchain` — all asserted substrings still present, ordering unchanged.
   - `TestServiceBuildRecipeHashMatchesProviderDockerfile` — verifies live `recipeHash()` matches `sha256(DefaultXxxDockerfile())`, which it would by construction (both sides change in lockstep).
   - `TestWriteDefault*ContextWritesDockerfile` — substring tests only check for presence of expected strings, not absence of additions.

A pinned baseline `const expectedRecipeHashCodex = "..."` would fail this scenario and force the future builder to explicitly justify the rebuild reflow.

**Disposition:** **CONFIRMED gap vs PLAN.md line 173.** Builder's rationale (worklog § Design notes) — that substring assertions plus the existing `TestServiceBuildRecipeHashMatchesProviderDockerfile` are "more strictly enforced" — is **incorrect**: substring assertions catch deletion of expected strings but cannot detect content **additions/reorderings** outside the asserted set. The existing recipeHash test is tautological (`recipeHash() == sha256(template)` where both sides derive from the same template variable).

Falsification verdict treatment: **non-blocking for this round.** The gap does not produce a runtime bug; it weakens the future change-control canary. PLAN.md is the authority — orchestrator may route to builder for a fix-forward (add a `const expectedRecipeHashCodex` / `expectedRecipeHashClaude` plus a `TestRecipeHashStability` test) without re-running the build, or accept the deviation. Flagging for orchestrator decision rather than asserting FAIL because: (a) build correctness is intact, (b) all other Unit 12.0 acceptance items pass, (c) the dev/orchestrator may judge the substring + ordering assertions sufficient hygiene given the cost/benefit.

#### A6 — `set -eu` interaction with `case` (MITIGATED)

**Construction:** `set -e` does not exit on a case with no matching branch unless the `*)` branch itself exits non-zero. The Dockerfile's `*)` arm explicitly runs `exit 1`. Verified in dash: matching branches succeed; non-matching arch exits 1; unset `${TARGETARCH}` under `set -u` fails at parameter substitution before reaching case.

**Disposition:** Mitigated. Triple-layered defense (set -u, case match, *) catchall).

#### A7 — `curl -fSL` flags (MITIGATED)

**Construction:** `-f` fail on 4xx/5xx (no error-body-as-success), `-S` show errors, `-L` follow redirects. All correct for an HTTPS tarball download from go.dev (which serves a redirect chain via storage.googleapis.com). No `-s` means progress meter shows, but that is acceptable docker-build output.

**Disposition:** Mitigated.

#### A8 — `ENV PATH=/usr/local/go/bin:$PATH` inheritance (MITIGATED)

**Construction:** Per Context7 `/docker/docs` excerpt (`content/manuals/build/building/best-practices.md`): *"The ENV instruction can update the PATH environment variable to make new software easier to run, such as `ENV PATH=/usr/local/nginx/bin:$PATH`."* `$PATH` expands at ENV-instruction-evaluation time to the value set by the previous layer (node:22-bookworm-slim's default `/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin`). Result is persisted into runtime ENV.

**Disposition:** Mitigated. Officially documented pattern, identical to Docker's nginx example.

#### A9 — Multi-step RUN reliability (`&&` chaining + `set -eu`) (MITIGATED)

**Construction:** Every step in `goInstallStep`'s RUN is joined by `&&`. If any step (curl, sha256sum, tar, rm) returns non-zero, the chain short-circuits and the RUN layer fails. `set -eu` reinforces this even where `&&` is not present (the case block uses `;;` between branches, not `&&`).

**Disposition:** Mitigated. Belt-and-suspenders.

#### A10 — Tar extract location produces `/usr/local/go` (MITIGATED)

**Construction:** Official Go release tarballs since go1.0 have always extracted to a top-level `go/` directory; `tar -C /usr/local -xzf go<ver>.linux-<arch>.tar.gz` produces `/usr/local/go`. This is the universal install pattern documented at https://go.dev/doc/install. Builder's `ENV PATH=/usr/local/go/bin:$PATH` therefore correctly points at the resulting `go` binary.

**Disposition:** Mitigated. Universal convention; matches go.dev install doc.

#### A11 — Image size ~150MB claim (MITIGATED, NOT LOAD-BEARING)

**Construction:** Go 1.26.1 linux-amd64 tarball is ~70MB compressed (verified via JSON `size` field in go.dev release index would confirm; not strictly needed). Extracted size ~150MB. Builder's claim aligns with the published archive size and is plausible. Not a correctness invariant — purely informational in PLAN.md decision 9.

**Disposition:** Not load-bearing for verdict. Plausible estimate.

#### Additional attacks attempted

- **A12 — riscv64 / unknown arch handling:** `*)` arm exits 1 with explicit message. dash verified: `set -eu; ARCH=riscv64; case ... *) echo bad >&2; exit 1 ;; esac` → exits 1 cleanly. Mitigated.
- **A13 — `set -u` masking `GO_SHA256` reference:** After `case` block exits unsuccessfully, the post-esac chain (`&& curl ... && echo "${GO_SHA256}..."`) is never reached because case's `exit 1` already terminates the shell. Even under hypothetical no-branch fall-through (impossible with `*)`), `${GO_SHA256}` under `set -u` would loudly fail with "unbound variable." Mitigated.
- **A14 — Tarball cleanup on failure:** `rm /tmp/go.tar.gz` is the last step; if any prior step fails, the tarball remains in `/tmp` inside the FAILED layer, which is discarded by docker. No persistent leak. Mitigated.
- **A15 — buildx vs legacy `docker build`:** Automatic `TARGETARCH` is a BuildKit feature. Legacy non-BuildKit `docker build` does not set it. PLAN.md note 314 declares `docker buildx build --load` canonical; `images.Service.Build` uses buildx exclusively (`ops.go`). A user invoking legacy `docker build` directly would hit `set -u` + "TARGETARCH: parameter not set" and fail loudly. Acceptable behavior — not a silent corruption path. Mitigated.
- **A16 — arm64 vs aarch64 naming mismatch:** Docker's TARGETARCH normalizes Linux arm64 to literal `arm64` (not `aarch64`). Builder uses `arm64` — matches Docker's canonical TARGETARCH values. Mitigated.
- **A17 — `npm install --global` PATH precedence collision:** After `ENV PATH=/usr/local/go/bin:$PATH`, the leading prefix is the Go bin dir. `npm` is at `/usr/local/bin/npm` and unaffected (no shadowing binary in `/usr/local/go/bin`). Subsequent npm RUN lines still resolve correctly. Mitigated.
- **A18 — `USER` context during install:** No `USER` directive precedes `goInstallStep`; install runs as root, which is required for writing `/usr/local/go` and `/usr/local/bin` (GOBIN). The final `USER valv` switch happens after the install. Mitigated.

### YAGNI Pressure

None worth flagging. The `const goInstallStep` DRY pattern (single source of truth for both Dockerfiles) is appropriate — PLAN.md decision 6 explicitly allowed either inline duplication OR a shared const, and the const choice is justified by the non-trivial multi-line shell block.

### Hylla Feedback

No fallback misses. Hylla was not consulted in this falsification round — the targets were a localized 30-line diff in `service.go` and external semantics (Docker BuildKit ARG rules, dash `case` semantics, GNU sha256sum format, go.dev release manifest), all of which are external to the Go code graph.

### Summary

Eighteen attacks attempted (eleven from the appendix's primary list + seven additional). Seventeen mitigated by build evidence or empirical reproduction. One CONFIRMED gap (A5 — missing `recipeHash` baseline pin) representing a literal deviation from PLAN.md Unit 12.0 acceptance line 173. The gap does not produce a runtime defect; it weakens the change-control canary against future no-op-style template drift. Flagged as non-blocking for falsification verdict but routed to orchestrator for fix-forward decision (add `const expectedRecipeHash{Codex,Claude}` + `TestRecipeHashStability` in a follow-up commit).

**Verdict: PASS** with one PLAN-deviation flag (A5) routed to orchestrator.

## Unit 12.1 — Round 1

**Verdict:** PASS (no unmitigated blocker; required full package mage target could not be confirmed in this sandbox because an unrelated existing `httptest.NewServer` test cannot bind a local port)

**Mage targets run:** `mage testPkg ./internal/services/images/` → FAIL in sandbox: `TestCodexVersionResolverReadsLatestRelease` panicked at `httptest: failed to listen on a port: listen tcp6 [::1]:0: bind: operation not permitted`. Scoped Unit 12.1 verification `go test -count=1 -race -run 'Test(OverlayHash|ShortOverlayHash|CanonicalManifest)' -coverprofile /private/tmp/valv-overlay-cover.out ./internal/services/images/` → PASS. Temporary falsification repro `TestFalsificationOverlayHashEdgeCases` with `GOCACHE=/private/tmp/valv-go-cache` → PASS, then removed.

### Attacks attempted

#### A1 — Hash determinism for nil/empty manifests

- **Hypothesis:** Nil `Tools` map and empty `Tools` map could hash differently (`null` vs `[]`), causing semantically identical "no tools" manifests to diverge.
- **Evidence:** `overlay.go:29-31` returns `[]canonicalTool{}` when `len(manifest.Tools) == 0`; `overlay_test.go:10-33` asserts nil-map and empty-map hashes match.
- **Outcome:** mitigated.
- **Detail:** `OverlayHash` always hashes canonical JSON for an empty slice, not a nil slice.

#### A2 — Hash collision / semantic drift in tool records

- **Hypothesis:** Two semantically distinct manifests could collapse to one canonical record, or two equivalent declaration orders could diverge.
- **Evidence:** `canonicalTool` includes `Name`, `Version`, `Source`, and `Install` (`overlay.go:14-20`), and `canonicalManifest` sorts by `Name` (`overlay.go:41-43`). `overlay_test.go:77-104` asserts out-of-order three-tool manifests hash identically; `overlay_test.go:55-75` asserts object-form and string-form examples differ. Context7 `/websites/pkg_go_dev_go1_25_3` and local `go doc` confirm `json.MarshalIndent` formats deterministic JSON for a supplied value; local `go doc crypto/sha256.Sum256` confirms SHA256 over the payload.
- **Outcome:** mitigated.
- **Detail:** The only intentional equivalence class is leading/trailing whitespace normalization of `Source`/`Install`. Cryptographic SHA256 collision resistance is assumed; no constructed collision exists.

#### A3 — Unicode and multiline TrimSpace asymmetry

- **Hypothesis:** Tabs/newlines or multi-byte whitespace such as NBSP could behave differently from ASCII spaces, diverging hashes unexpectedly.
- **Evidence:** Local `go doc strings.TrimSpace` says leading/trailing whitespace is removed as defined by Unicode. The temporary repro verified `"\u00a0github.com/x/y\u00a0"` and `"\n\tgo install\r\n"` hash identically to clean values.
- **Outcome:** mitigated.
- **Detail:** Unicode leading/trailing whitespace normalizes; internal whitespace is intentionally preserved and tested in `overlay_test.go:187-196`.

#### A4 — JSON escaping edge cases

- **Hypothesis:** Quotes, backslashes, or control-like characters in `Source`/`Install` could create ambiguous canonical JSON or round-trip incorrectly.
- **Evidence:** The temporary repro compared `Source: github.com/x/"y"` against `Source: github.com/x/\"y\"`, confirmed different hashes, then unmarshaled the `json.MarshalIndent(canonicalManifest(...), "", "")` payload back into `[]canonicalTool` without data loss.
- **Outcome:** mitigated.
- **Detail:** JSON escaping is byte-stable for the canonical struct slice; distinct source strings remain distinct before hashing.

#### A5 — Sort stability, duplicate names, and empty names

- **Hypothesis:** `sort.Slice` is unstable, duplicate names could reorder nondeterministically, and an empty tool name could sort first and produce a surprising hash.
- **Evidence:** Local `go doc sort.Slice` confirms instability for equal elements. Hylla snapshot 7 and local `tools.ToolManifest` show `Tools map[string]ToolSpec`, so duplicate names cannot coexist in the in-memory manifest. `tools.Validate` rejects empty names via `toolNameRE` (`internal/tools/validate.go:48-50`), though `canonicalManifest` does not revalidate.
- **Outcome:** accepted as non-blocking.
- **Detail:** Duplicate-name instability is impossible with the map shape. Empty-name hashing is possible only if a caller bypasses `tools.Validate`; Unit 12.1 acceptance does not require validation inside the hashing helper.

#### A6 — `shortOverlayHash` wrong-argument misuse

- **Hypothesis:** A caller could pass a Go-formatted manifest string or arbitrary short string to `shortOverlayHash`, silently receiving a 12-char prefix that looks tag-like.
- **Evidence:** LSP references show `shortOverlayHash` is currently referenced only by its declaration and tests. `overlay.go:70-75` accepts any string and truncates if length is at least 12; `overlay_test.go:147-165` explicitly covers truncation and defensive short input.
- **Outcome:** accepted as non-blocking.
- **Detail:** This is real API footgun potential, but the helper is unexported and has no production caller until Unit 12.3. Routing note below tells Unit 12.3 to pass only `OverlayHash` output.

#### A7 — Single-trim invariant

- **Hypothesis:** `Source`/`Install` might be consumed before trimming or trimmed twice, breaking the Unit 12.2 contract.
- **Evidence:** `rg` found `strings.TrimSpace` in `overlay.go` only at the `Source` and `Install` assignments inside `canonicalManifest`; LSP references show `canonicalManifest` is currently consumed by `OverlayHash` and tests only.
- **Outcome:** mitigated for Unit 12.1.
- **Detail:** There is one trim point in current code. The future Dockerfile generator must consume `canonicalManifest` rather than re-trimming.

#### A8 — Unreachable JSON error branch returns empty string

- **Hypothesis:** If `json.MarshalIndent` fails, `OverlayHash` returns `""`, which a caller could misread as a valid "empty manifest" hash.
- **Evidence:** `canonicalTool` contains only strings, so the current payload has no unsupported value such as a channel, function, NaN, or cyclic pointer. Overlay-only coverage showed the only uncovered `overlay.go` block is lines `56-61`, the impossible error branch.
- **Outcome:** accepted as non-blocking.
- **Detail:** The branch is defensive and unreachable with current types. The empty string behavior should be treated as "malformed/unexpected" by future callers, not as a valid digest.

#### A9 — Coverage delta hides meaningful behavior

- **Hypothesis:** The reported 80.4% package coverage may hide important untested overlay behavior.
- **Evidence:** Scoped coverprofile reports `canonicalManifest` 100%, `shortOverlayHash` 100%, and `OverlayHash` 83.3%; raw coverage entries show only `overlay.go:56-61` has count 0.
- **Outcome:** mitigated.
- **Detail:** Uncovered overlay behavior is the unreachable `json.MarshalIndent` error return. No meaningful sorting, trimming, empty-map, or truncation path is uncovered.

#### A10 — YAGNI, hidden dependencies, concurrency, and interface misuse

- **Hypothesis:** Unit 12.1 could introduce unnecessary abstractions, shared state, init-time side effects, goroutines, panicking type assertions, or swallowed errors.
- **Evidence:** `overlay.go` adds one struct and three functions, no interfaces, no package vars, no `init`, no goroutines/channels/context, no type assertions, and no ignored error except the handled `json.MarshalIndent` branch. LSP diagnostics report no issues.
- **Outcome:** mitigated.
- **Detail:** The implementation is deterministic pure data transformation plus hashing.

#### A11 — Snapshot fragility

- **Hypothesis:** The pinned hex snapshot could change while no other test fails, or the fixture could be edited accidentally.
- **Evidence:** `overlay_test.go:130-145` pins both a concrete two-tool fixture and a hardcoded full digest. The test fails on any canonical-form drift. Other behavior tests cover the semantic invariants separately.
- **Outcome:** mitigated with normal snapshot limitations.
- **Detail:** If a future builder edits both the fixture and expected hex together, the snapshot cannot prove intent. That is an ordinary review/process limitation, not a Unit 12.1 blocker.

### Non-blocking gaps

- Full `mage testPkg ./internal/services/images/` did not pass in this sandbox because an existing HTTP test cannot bind a localhost port. This is an environment verification gap, not a constructed counterexample against `overlay.go`.
- `shortOverlayHash` is intentionally unvalidated and unexported; Unit 12.3 must avoid passing arbitrary strings.
- `canonicalManifest` assumes a validated manifest for name/schema correctness; direct construction can hash invalid tool names, including the empty string.

### Routing

- Unit 12.2 builder: consume `canonicalManifest` directly for Dockerfile emission and do not re-trim `Source` or `Install`.
- Unit 12.3 builder: compute `toolsHash := OverlayHash(request.Manifest)` once and pass only that full hash to `shortOverlayHash`; treat `""` as unexpected/mismatch, never as a valid hash.

## Unit 12.2 — Round 1

**Verdict:** PASS (no unmitigated blocker; full package mage target could not be confirmed in this sandbox because an unrelated existing `httptest.NewServer` test cannot bind a local port)

**Mage targets run:** `mage testPkg ./internal/services/images/` → FAIL in sandbox: `TestCodexVersionResolverReadsLatestRelease` panicked at `httptest: failed to listen on a port: listen tcp6 [::1]:0: bind: operation not permitted`. Scoped overlay verification `GOCACHE=/private/tmp/valv-go-build-cache go test ./internal/services/images -run 'TestBuildOverlayDockerfile|TestCanonicalManifest|TestOverlayHash|TestShortOverlayHash' -count=1` → PASS. Temporary falsification repro `TestFalsificationJSONActiveSourceStaysSingleArg` with JSON-active characters in `Source` → PASS, then removed.

### Attacks attempted

#### A1 — JSON-active source injection
- **Hypothesis:** A `Source` containing quotes, backslashes, control bytes, embedded newline/tab, `$`, or backticks could break the Dockerfile JSON array or split into multiple argv elements.
- **Evidence:** `BuildOverlayDockerfile` constructs `argv []string` and emits `RUN %s` from `json.Marshal(argv)` (`overlay.go:124-141`). Local `go doc encoding/json.Marshal` confirms strings are encoded as JSON strings with escaping. The committed injection test parses the generated RUN payload back with `json.Unmarshal` and asserts the semicolon source survives as one argv element (`overlay_test.go:384-432`). The temporary repro used `github.com/x/"quoted"\path\n\t$HOME` + control byte and round-tripped it as one argv element.
- **Outcome:** mitigated.
- **Detail:** No counterexample found. JSON-active and shell-active characters stay inside one JSON string; downstream `go install` receives one literal argument.

#### A2 — Docker exec-form RUN parse and shell bypass
- **Hypothesis:** Docker might still invoke a shell for `RUN ["go","install","..."]`, allowing `$HOME`, backticks, semicolons, or command substitution to execute.
- **Evidence:** Context7 `/docker/docs` for Dockerfile reference says exec-form `RUN` is parsed as a JSON array, requires double quotes, and does not automatically invoke a command shell unless one is explicitly executed. The generator never emits `["sh","-c",...]`; it emits `["go","install",source]` or `["npm","install","-g",source]` (`overlay.go:125-129`).
- **Outcome:** mitigated.
- **Detail:** `$HOME` and backticks in `Source` remain literal argv content. This attack fails unless a future change wraps commands in `sh -c`.

#### A3 — Sorting determinism and semantically equivalent names
- **Hypothesis:** Two manifests with equivalent tools could produce nondeterministic RUN order, or two name variants could sort unexpectedly.
- **Evidence:** `canonicalManifest` iterates the `map[string]tools.ToolSpec`, appends every entry, and sorts by `Name` (`overlay.go:35-51`). `TestBuildOverlayDockerfile_SortingDeterminism` asserts same output for two declaration orders (`overlay_test.go:267-295`). Local `go doc sort.Slice` confirms instability only matters for equal keys; duplicate keys cannot coexist in a Go map.
- **Outcome:** mitigated for declared acceptance; accepted for semantic aliases.
- **Detail:** If two different tool names are "semantically equivalent" to a human, they are still distinct map keys and intentionally produce distinct layers/order by bytewise name sort. No nondeterministic duplicate-name counterexample exists.

#### A4 — Error message coverage and wrapping
- **Hypothesis:** Error paths could swallow cause details or use `%v` where `%w` is needed for sentinel checks.
- **Evidence:** Validation-style errors for unsupported string-form, empty source/install, and unsupported install verb are direct `fmt.Errorf` messages (`overlay.go:114-131`); there is no underlying sentinel error in these branches. The only real underlying error is `json.Marshal(argv)`, and it is wrapped with `%w` (`overlay.go:137-139`). Tests assert key message details for string-form, unsupported verb, empty source, and empty install (`overlay_test.go:298-381`).
- **Outcome:** mitigated.
- **Detail:** There is no lost sentinel for `errors.Is/As` in the expected validation branches. The only wrapped lower-level error uses `%w`.

#### A5 — canonicalManifest re-call and full iteration
- **Hypothesis:** `BuildOverlayDockerfile` might skip a tool due to short-circuiting, re-trimming, or using only a subset of the manifest.
- **Evidence:** The function calls `canonicalManifest(manifest)` once (`overlay.go:95-96`) and ranges over every `tool := range canonical` (`overlay.go:109`). LSP references show `canonicalManifest` is used by `OverlayHash`, `BuildOverlayDockerfile`, and tests. The byte-for-byte test verifies two tools are both emitted (`overlay_test.go:247-260`).
- **Outcome:** mitigated.
- **Detail:** A malformed earlier-sorted tool can stop generation with an error before later tools are emitted, but that is the correct all-or-error behavior for an invalid manifest.

#### A6 — String-form and zero-value shape detection
- **Hypothesis:** `Source=="" && Install==""` may misclassify all-three-empty specs as string-form, or `Version==""` alone may need different treatment.
- **Evidence:** DROP_11 `validateSpec` defines string-form as only `Version` set, object-form as only `Source + Install` set, and rejects all three empty (`validate.go:65-89`). `BuildOverlayDockerfile` only receives a manifest value and chooses the more specific unsupported-string-form error when source/install are both empty (`overlay.go:110-116`), matching Unit 12.2 acceptance line 223. `TestBuildOverlayDockerfile_StringFormRejected` covers the valid string-form case (`overlay_test.go:298-315`).
- **Outcome:** accepted as non-blocking.
- **Detail:** A direct unit caller can construct `ToolSpec{}` and get the string-form error rather than "empty spec." That is imprecise wording for an invalid bypassed-Validate manifest, not a runtime blocker under the DROP_11 call contract.

#### A7 — USER root to USER valv bracket
- **Hypothesis:** The overlay Dockerfile could leave the final image running as root, or a failed tool RUN could persist root state.
- **Evidence:** The generator writes `USER root`, then ENV, then all RUN lines, then a final `USER valv` (`overlay.go:101-145`). The byte-for-byte test pins this exact order (`overlay_test.go:247-260`). Base provider Dockerfiles end with `USER valv` after installing CLIs (`service.go:701` and analogous Claude path), and PLAN decision 6 requires the bracket.
- **Outcome:** mitigated.
- **Detail:** If a tool RUN fails, Docker build fails and no successful final overlay image is produced. There is no "retained root state" image from a failed build.

#### A8 — ENV ordering and first RUN visibility
- **Hypothesis:** `GOBIN=/usr/local/bin` might only affect RUN lines after the first tool RUN, not the first one.
- **Evidence:** The emitted order is `ENV ... GOBIN=/usr/local/bin`, blank line, then RUN lines (`overlay.go:103-141`). Context7 `/docker/docs` confirms ENV values persist into subsequent instructions/layers. The byte-for-byte test pins ENV before the first RUN (`overlay_test.go:247-257`).
- **Outcome:** mitigated.
- **Detail:** The first `go install` RUN is subsequent to the ENV instruction and receives `GOBIN`.

#### A9 — Empty manifest behavior
- **Hypothesis:** An empty manifest might emit an invalid Dockerfile or an unexpected error.
- **Evidence:** `canonicalManifest` returns an empty slice for nil/empty tools (`overlay.go:35-38`). The RUN loop then emits zero RUN lines and still writes the final `USER valv` (`overlay.go:109-145`). PLAN Unit 12.2 does not mandate an error for empty manifests; the dispatch notes expect Unit 12.3 to short-circuit empty manifests before calling this function.
- **Outcome:** accepted as non-blocking.
- **Detail:** Zero-RUN output is syntactically valid and harmless but may be unnecessary work. Unit 12.3 should avoid calling it for empty manifests if that remains the intended control flow.

#### A10 — npm install prefix vs GOBIN
- **Hypothesis:** `GOBIN=/usr/local/bin` fixes Go installs but does nothing for `npm install -g`, so npm tools could land somewhere outside PATH unless `NPM_CONFIG_PREFIX` is also set.
- **Evidence:** Context7 `/npm/cli` says npm global installs use the `prefix` configuration, defaulting on Unix to the Node installation location such as `/usr/local`; npm config can be set via `NPM_CONFIG_*`. The base image is `node:22-bookworm-slim` (`service.go:672`/`736`), and the existing base Dockerfiles already use `npm install --global` without setting `NPM_CONFIG_PREFIX` (`service.go:695-699`). The overlay sets npm noninteractive/audit envs but not prefix (`overlay.go:103-106`).
- **Outcome:** accepted as non-blocking.
- **Detail:** `GOBIN` does not affect npm, but this is consistent with existing provider Dockerfile behavior and npm's default global prefix. If future runtime evidence shows global npm binaries are not on PATH, add `NPM_CONFIG_PREFIX=/usr/local`; no current counterexample was constructed.

#### A11 — Base image reference validity
- **Hypothesis:** `baseImage.String()` could return an empty string, yielding `FROM ` and an invalid Dockerfile.
- **Evidence:** `docker.ImageRef.String` returns `""` when `Repository == ""` (`types.go:22-25`). `BuildOverlayDockerfile` does not validate `baseImage` (`overlay.go:98-99`).
- **Outcome:** accepted as non-blocking.
- **Detail:** This is a direct-call footgun but outside Unit 12.2 acceptance; callers should pass a validated base image ref. It is worth keeping in mind for Unit 12.3 wiring.

#### A12 — Concurrency, interfaces, hidden state, and YAGNI
- **Hypothesis:** The unit could introduce goroutine leaks, interface traps, package state, init side effects, or premature abstractions.
- **Evidence:** `overlay.go` adds constants and a pure string-rendering function; no goroutines, channels, contexts, mutexes, type assertions, interfaces, `init`, package vars, or ignored errors. LSP references show `BuildOverlayDockerfile` has no production caller yet beyond tests.
- **Outcome:** mitigated.
- **Detail:** No concurrency or hidden-dependency counterexample exists in this unit.

### Non-blocking gaps

- Full `mage testPkg ./internal/services/images/` could not pass in this sandbox because an existing HTTP test cannot bind `[::1]:0`; scoped overlay tests passed.
- Empty manifest handling returns a valid zero-RUN overlay Dockerfile. Unit 12.3 should short-circuit empty manifests if the orchestrator wants to avoid unnecessary builds.
- `BuildOverlayDockerfile` does not validate `baseImage.Repository`; Unit 12.3 should pass a validated non-empty `docker.ImageRef`.
- `NPM_CONFIG_PREFIX` is not set. Current evidence says npm defaults are acceptable for the Node base image, but this should be revisited if an end-to-end overlay npm install lands binaries outside PATH.

### Routing

- Unit 12.3 builder: short-circuit empty tool manifests before calling `BuildOverlayDockerfile`, and pass only a validated non-empty base image ref.
- Drop-end verifier: run the full `mage testPkg ./internal/services/images/` outside the network-restricted sandbox to confirm the existing `httptest` case.

## Unit 12.3 — Round 1

**Verdict:** FAIL

**Mage targets run:** `mage testPkg ./internal/services/images/` → FAIL in sandbox: unrelated existing `TestCodexVersionResolverReadsLatestRelease` panicked at `httptest: failed to listen on a port: listen tcp6 [::1]:0: bind: operation not permitted`. Scoped Unit 12.3 verification `go test ./internal/services/images -run 'TestEnsureProjectImage_|TestProjectImageRef_TagFormat' -count=1` → PASS. Temporary falsification repro `env GOCACHE=/private/tmp/valv-go-build-cache go test ./internal/services/images -run TestFalsification_BaseInspectErrorBubblesInsteadOfRebuild -count=1` → PASS, then removed.

### Attacks attempted

#### A1 — `baseRecipeHash` double-hash contract
- **Hypothesis:** `baseRecipeHash` might be passed through `sha256Hex`, silently re-hashing the already-hashed base label and forcing perpetual rebuilds.
- **Evidence:** `rg -n 'sha256Hex\\('` found only `service.go:739` (`sha256Hex(dockerfileContent)`) plus test callers. `service.go:749-757` reads `baseRecipeHash` via `inspectLabel` and never hashes it.
- **Outcome:** mitigated.
- **Detail:** No caller passes `baseRecipeHash` to `sha256Hex`; the value is copied verbatim into `buildRequest.Labels[baseRecipeHashLabel]` at `service.go:799-805`.

#### A2 — Five-label completeness on actual build args
- **Hypothesis:** One or more required labels could be missing from the real `docker.BuildImageArgs` call even if the method returns success.
- **Evidence:** `service.go:799-805` populates all five labels in the `ImageBuildRequest`. `service_test.go:1285-1324` inspects the recorded `buildx build` args and checks for `recipe_hash`, `base_recipe_hash`, `tools_hash`, `managed=true`, and `scope=project-overlay`.
- **Outcome:** mitigated on the primary buildx path.
- **Detail:** The test asserts the recorded CLI args, not just return success. Legacy fallback uses the same `buildRequest` object with only `Builder` flipped to `"legacy"` (`service.go:813-821`), so the label set stays identical there too.

#### A3 — Empty-manifest short-circuit
- **Hypothesis:** Empty manifests might still generate an overlay, inspect docker, or return the wrong action.
- **Evidence:** `service.go:721-725` returns immediately with `EnsureActionUsingExistingImage`. `service_test.go:1224-1246` asserts zero runner calls.
- **Outcome:** mitigated.
- **Detail:** This path neither builds nor inspects anything.

#### A4 — Conservative-opposite policy on base label inspect errors
- **Hypothesis:** PLAN decision 5 says any non-missing `docker image inspect` label-read failure should be treated as mismatch and force rebuild, but the implementation may instead return an error before rebuilding.
- **Evidence:** `service.go:749-757` returns `ensure project image: inspect base recipe hash: ...` for any base-label inspect error except `dockerImageMissingError` and `errLabelUnreadable`. A temporary repro test using `runnerRecorder` with `projectInspectKey(baseRef.String(), recipeHashLabel): fmt.Errorf("permission denied")` confirmed `EnsureProjectImage` returns an error after a single base-inspect call and never attempts build. The repro passed under `GOCACHE=/private/tmp/valv-go-build-cache` and was then deleted.
- **Outcome:** BLOCKER.
- **Detail:** This is a concrete contract mismatch against PLAN.md decision 5's "ANY non-missing error ... forces a rebuild" rule. The current code only applies conservative rebuild to target-label reads, not to the base-label read.

#### A5 — Target-image missing short-circuit
- **Hypothesis:** A missing target image could bypass the intended rebuild path or be confused with typecast failure.
- **Evidence:** `projectImageNeedsBuild` calls `inspectLabel` on `recipe_hash` first (`service.go:845-850`) and returns `true` on any error. `service_test.go:1249-1337` simulates `no such image` on the target tag and confirms a build occurs.
- **Outcome:** mitigated.
- **Detail:** Missing target image rebuilds through the freshness-probe path as intended.

#### A6 — Type-assertion failure vs `imageRecipeMatches`
- **Hypothesis:** The overlay path could accidentally inherit `imageRecipeMatches`'s safe-skip behavior when the runner does not implement `outputRunner`.
- **Evidence:** `imageRecipeMatches` returns `true, nil` on typecast failure at `service.go:639-641`, while `inspectLabel` returns `errLabelUnreadable` at `service.go:681-684` and `projectImageNeedsBuild` treats any such error as rebuild. `service_test.go:1464-1501` confirms the typecast-failure path rebuilds.
- **Outcome:** mitigated.
- **Detail:** The overlay code correctly does the conservative opposite on target freshness probes.

#### A7 — `dockerImageMissingError` sentinel use
- **Hypothesis:** The missing-image sentinel might be dead code or collapse into the generic unreadable-label path.
- **Evidence:** Hylla snapshot 7 shows `dockerImageMissingError` is the existing string-match predicate used by `imageAvailable` and `imageRecipeMatches`. Live code uses it in the base inspect path (`service.go:751-752`) and in `inspectLabel` wrapping (`service.go:687-690`).
- **Outcome:** mitigated.
- **Detail:** The sentinel still matters: missing base image returns a specific error, while missing target image becomes "rebuild".

#### A8 — Tag charset / length
- **Hypothesis:** `projectImageRef` could emit an invalid Docker tag, especially if repository length is already large.
- **Evidence:** `service.go:659-660` emits `proj-` plus `shortOverlayHash(...)`; `overlay.go:77-81` truncates to 12 hex chars. The tag portion is therefore always 17 characters from `[a-z0-9-]`. Context7 Docker docs did not surface a stronger contrary constraint.
- **Outcome:** mitigated for the tag portion; repository validation remains an inherited assumption.
- **Detail:** The new code does not worsen repo-name validity. It only appends a short lowercase-hex tag suffix.

#### A9 — Up-front overlay generation before any docker call
- **Hypothesis:** Generating the overlay before freshness checks could create correctness drift or mask cache hits.
- **Evidence:** `service.go:734-739` generates the Dockerfile once, hashes it, and reuses the exact bytes for rebuild. NoCache and label-match behavior are decided later at `service.go:766-777`.
- **Outcome:** accepted as non-blocking.
- **Detail:** This does extra work on cache hits, but it keeps the compared `recipe_hash` and rebuilt Dockerfile byte-identical.

#### A10 — Legacy buildx fallback label symmetry
- **Hypothesis:** The legacy fallback path might drop one of the five labels even if buildx carries them.
- **Evidence:** `service.go:794-821` mutates only `buildRequest.Builder` before regenerating args. Existing base-image fallback coverage (`service_test.go:459-520`) proves the service pattern keeps labels when switching from `buildx build --load` to `build`.
- **Outcome:** mitigated by code trace, but direct overlay fallback coverage is still missing.
- **Detail:** I did not find a concrete divergence path because the same `Labels` map is reused unchanged.

#### A11 — `NoCache=true` forcing rebuild
- **Hypothesis:** Matching labels might still skip rebuild when `NoCache` is true.
- **Evidence:** `service.go:766-769` seeds `rebuild := request.NoCache`. `service_test.go:1535-1592` confirms a rebuild occurs and `--no-cache` is present in the recorded build args.
- **Outcome:** mitigated.
- **Detail:** This path behaves as specified.

#### A12 — `errLabelUnreadable` sentinel usefulness
- **Hypothesis:** The sentinel could be decoration only, with no caller behavior change.
- **Evidence:** `service.go:754-757` uses `errors.Is(baseErr, errLabelUnreadable)` to tolerate only that case for the base label read. Target-image freshness reads collapse all errors to rebuild inside `projectImageNeedsBuild`.
- **Outcome:** accepted, but it is the mechanism that exposes A4.
- **Detail:** The sentinel is not dead, but its special treatment is asymmetric and currently too narrow for the broader PLAN.md wording.

#### A13 — Shared-service concurrency / same-manifest TOCTOU
- **Hypothesis:** Two concurrent `EnsureProjectImage` calls for the same manifest could race and corrupt state.
- **Evidence:** `Service` holds immutable config only (`service.go:91-103`), and `EnsureProjectImage` uses local variables plus a temp dir per call. There is no shared mutable Go state or goroutine spawned by the method.
- **Outcome:** accepted as non-blocking.
- **Detail:** Two callers can still both decide to rebuild and race on the same Docker tag, but that is an external Docker-level TOCTOU and not an in-process data race introduced by this unit.

#### A14 — YAGNI helpers
- **Hypothesis:** `projectImageNeedsBuild` and `sha256Hex` could be premature abstractions.
- **Evidence:** `projectImageNeedsBuild` centralizes the three-label comparison and rebuild-on-error policy; `sha256Hex` has one production caller and mirrors existing `recipeHash` behavior. Neither introduces indirection beyond the immediate unit.
- **Outcome:** mitigated.
- **Detail:** Small helpers are justified here; no extra interface or hidden dependency was introduced.

### Non-blocking gaps

- Full `mage testPkg ./internal/services/images/` could not be confirmed in this sandbox because an unrelated existing resolver test cannot bind `[::1]:0`.
- Overlay-specific legacy fallback does not have its own recorded-args test, even though the code path reuses the same `buildRequest` label map.
- Docker repo-name validity still relies on existing `Service` configuration; Unit 12.3 only constrains the tag suffix.

### Routing for Unit 12.4

- Do not wire CLI callers onto `EnsureProjectImage` until Unit 12.3 is fixed or the planner explicitly relaxes decision 5.
- Fix path: change the base-label error branch so non-missing inspect failures are treated as rebuild-triggering unreadable labels, then add a regression test covering "base inspect generic error still rebuilds".
- Optional follow-up coverage: add an explicit overlay fallback test that records both `buildx` and legacy build args and reasserts the full five-label set on the fallback call.
