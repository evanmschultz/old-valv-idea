# DROP_12 Plan QA Proof — Round 2

**Verdict:** fail
**Reviewer:** ta-go-qa-proof
**Reviewed at:** 2026-05-21T19:03:35Z

Round 1 → Round 2 deltas verified one-by-one against `main/drops/DROP_12_IMAGE_LAYERING/PLAN.md` and against the cited source files. Eleven of twelve deltas land cleanly. One new finding (F5) on the Unit 12.0 Go install step blocks pass.

## Round 1 Delta Verification

### Delta 1 — F1 `base_recipe_hash` verbatim, no double-hash

**Status:** PASS

- PLAN.md:53 — "`io.valv.base_recipe_hash` — **verbatim copy** of the base image's `io.valv.recipe_hash` label value, captured at build-time via `docker image inspect`. **Stored as-is** — the label is already a sha256 hex string at `service.go:609`; double-hashing it would obscure the relationship to the base."
- PLAN.md:227 — Unit 12.3 acceptance step 2: "Computes `toolsHash := OverlayHash(request.Manifest)` and `baseRecipeHash := s.inspectLabel(ctx, request.BaseImage, recipeHashLabel)` (verbatim — no double-hash, F1)."
- PLAN.md:279 — Notes For Builder Agents: "Read it with `docker image inspect --format '{{ index .Config.Labels "io.valv.recipe_hash" }}'`, `strings.TrimSpace`, store as-is."
- PLAN.md:111 — Acceptance Criteria 5 references the label set with `base_recipe_hash`.
- Source check: `internal/services/images/service.go:609` returns `strings.TrimSpace(output) == s.recipeHash()` — confirms the label value at that line is already a sha256 hex string. Verbatim policy is correct.

### Delta 2 — F2 label-read failure forces rebuild (overlay)

**Status:** PASS

- PLAN.md:60 — Schema Decision 5: "When `s.runner` does not implement `outputRunner` (typecast failure), OR when `docker image inspect` returns ANY non-missing error, `EnsureProjectImage` treats it as a label mismatch and FORCES a rebuild. This is conservative-opposite of `imageRecipeMatches`'s base-image safe-skip behavior (`service.go:599-600` returns `true`/skip-rebuild)."
- PLAN.md:123 — Acceptance Criteria 5 final bullet: "When ANY freshness label mismatches OR cannot be read (typecast failure, inspect error other than image-missing): rebuilds."
- PLAN.md:229 — Unit 12.3 acceptance step 4: "On ANY read failure (typecast OR non-missing inspect error), treats as mismatch and rebuilds (F2)."
- Source check: `internal/services/images/service.go:597-610` confirms `imageRecipeMatches` returns `true` (skip-rebuild) on `runner.(outputRunner)` typecast failure at line 599-600. The plan's conservative-opposite policy for overlay is precisely the inversion described.

### Delta 3 — F3 `EnsureProjectRequest` has no `Pull` field

**Status:** PASS

- PLAN.md:95 — Schema Decision 8: "`EnsureProjectRequest{Manifest tools.ToolManifest; BaseImage docker.ImageRef; NoCache bool}`. **`Pull` field removed** — base images are locally built, never pulled from a registry; a `Pull` flag is misleading. `NoCache` remains for force-rebuild scenarios."
- PLAN.md:224 — Unit 12.3 acceptance: "New request/result types `EnsureProjectRequest{Manifest tools.ToolManifest; BaseImage docker.ImageRef; NoCache bool}` and `EnsureProjectResult{Image docker.ImageRef; Action EnsureAction; ToolsHash string; BaseRecipeHash string}`. **No `Pull` field** (F3)."
- No other occurrence of "Pull" as a field of `EnsureProjectRequest` anywhere in PLAN.md.

### Delta 4 — F4 obsolete Hylla-pin caveat removed

**Status:** PASS

- PLAN.md "Notes For Builder Agents" section (lines 278-290) reviewed end-to-end. No mention of pinning a Hylla artifact ref to a stale value. The bullets are all DROP_12-substantive (recipeHash, ImageRef parsing, buildx, test fakes, exec-form RUN, GOBIN placement, Go install, cleanup-filter, mage-integration rule, unit ordering, Phase 6 verify). Round 1's obsolete caveat is gone.

### Delta 5 — Attack 2 `io.valv.managed=true` label on per-project images

**Status:** PASS

- PLAN.md:55 — Schema Decision 4: "`io.valv.managed=true` — **required for `valv image cleanup` reachability** (Attack 2 fix). Matches the existing label filter at `manage.go:1685-1689`."
- PLAN.md:121 — Acceptance Criteria 5 second bullet: "applies the five labels (`recipe_hash`, `base_recipe_hash`, `tools_hash`, `managed=true`, `scope=project-overlay`)."
- PLAN.md:230 — Unit 12.3 acceptance step 5 rebuild path lists all five labels including `managed=true` and `scope=project-overlay`.
- PLAN.md:241 — Unit 12.3 test assertion: "Test asserts the built image carries all five labels (recipe_hash + base_recipe_hash + tools_hash + managed=true + scope=project-overlay)."
- PLAN.md:287 — Notes For Builder Agents: "Per-project images MUST carry `io.valv.managed=true` or `valv image cleanup` cannot reach them (`manage.go:1685-1689`). Unit 12.3's label set covers this."
- Source check: `internal/cli/manage.go:1685-1689` confirms the filter is `{"label": "io.valv.managed=true"}` — exact match.

### Delta 6 — Attack 3 exec-form RUN with `ENV GOBIN` at top

**Status:** PASS

- PLAN.md:62-86 — Schema Decision 6 explicitly mandates exec-form (JSON array) RUN, places `GOBIN=/usr/local/bin` inside the overlay `ENV` block at the top, and explains why shell-form is forbidden (injection surface) and why `RUN ["GOBIN=...", "go", ...]` would fail (exec-form skips shell var expansion).
- PLAN.md:115-117 — Acceptance Criteria 4 mandates ENV GOBIN, exec-form RUN [...], JSON array, sorted-by-name ordering.
- PLAN.md:197-198 — Unit 12.2 acceptance: "Output structure per decision 6: `FROM <baseImage.String()>` → `USER root` → `ENV NPM_CONFIG_* + GOBIN=/usr/local/bin` → one exec-form RUN per tool (sorted by name) → `USER valv`. All RUN lines use **JSON array exec-form**." Marshalling via `json.Marshal` of the argv slice is required.
- PLAN.md:284-285 — Notes For Builder Agents both call out exec-form requirement and GOBIN placement reasoning. Explicit, repeated, unambiguous.

### Delta 7 — Attack 4 Go in base image is now Unit 12.0 (lands first)

**Status:** PASS with sub-finding (see F5 below for a separate gap in this unit's content)

- PLAN.md:135-158 — Unit 12.0 exists with `Blocked by: —`, lands before 12.1.
- PLAN.md:97 — Schema Decision 9: "Both `DefaultCodexDockerfile` and `DefaultClaudeDockerfile` gain a Go 1.26.1 install step. **Method:** download official binary tarball from `https://go.dev/dl/go1.26.1.linux-${TARGETARCH}.tar.gz`, extract into `/usr/local/go`, append `/usr/local/go/bin` to PATH. Aligns container Go version with `go.mod`'s `go 1.26.1`."
- PLAN.md:147 — Unit 12.0 acceptance specifies `https://go.dev/dl/go1.26.1.linux-${TARGETARCH}.tar.gz`, extraction into `/usr/local/go`, `curl` added to apt list.
- PLAN.md:289 — Unit-ordering note: "Strict linear (12.0 → 12.1 → 12.2 → 12.3 → 12.4). No parallel-eligible units. Unit 12.0 must land first because Unit 12.2's `go install` RUN lines fail at build-time without Go in the base image."
- Ordering, version, source URL, and motivation all present. Sequencing landed cleanly.

### Delta 8 — Caveat 1 single trim point (`strings.TrimSpace` once)

**Status:** PASS

- PLAN.md:49 — Schema Decision 3: "`canonicalManifest(manifest)` applies `strings.TrimSpace` to each tool's `Source` and `Install` field exactly once, before either hashing OR Dockerfile emission. Both code paths (hash computation in Unit 12.1, RUN-line generation in Unit 12.2) consume the same already-trimmed strings via `canonicalManifest`."
- PLAN.md:173 — Unit 12.1 acceptance: "Add helper `canonicalManifest(manifest tools.ToolManifest) []canonicalTool` (unexported) that returns a sorted-by-name slice of `{Name, Version, Source, Install}` records with `strings.TrimSpace` applied **once** to `Source` and `Install`. Single trim point — both hashing and Dockerfile emission consume this slice."
- PLAN.md:176 — Unit 12.1 tests cover the whitespace-trim case: "whitespace in source/install (trim applied → identical hash)."

### Delta 9 — Caveat 8 `VALV_*_IMAGE` override warning

**Status:** PASS

- PLAN.md:105 — Schema Decision 13: When override env var is set AND non-empty manifest, "the launcher emits one stderr warning before launch: `"warning: VALV_<PROVIDER>_IMAGE override active; .valv/tools.toml overlay skipped"`. The override still applies (no overlay built, no overlay tag used)."
- PLAN.md:125 — Acceptance Criteria 7: stderr exactly once message verbatim.
- PLAN.md:264 — Unit 12.4 acceptance bullet covers `resolveProjectImage` warning emission via `fmt.Fprintln(cmd.ErrOrStderr(), ...)` and notes override is applied upstream.
- PLAN.md:270 — Unit 12.4 tests include "manifest with one tool + `VALV_CLAUDE_IMAGE` set → overlay skipped + stderr warning printed exactly once."

### Delta 10 — Caveat 10 Unit 12.5 collapsed; final unit count = 5

**Status:** PASS

- Units inventoried: 12.0 (PLAN.md:135), 12.1 (162), 12.2 (185), 12.3 (212), 12.4 (248). Total = 5. No `#### Unit 12.5` header anywhere in the file.
- PLAN.md:290 — Notes For Builder Agents: "**Phase 6 drop-end verify (replaces former Unit 12.5):** `mage test` clean from `main/`, then `mage integration` (Docker-backed), then `mage golden` ... This is standard Phase 6 — no dedicated unit needed (Caveat 10)." The historical reference is intentional and consistent with Caveat 10.
- A focused grep for `12.5` returns only this one historical-context match. No leftover acceptance/schema/notes pointing at a missing unit.

### Delta 11 — U2 confirmation (auto-build on launch)

**Status:** PASS

- PLAN.md:99 — Schema Decision 10: "Auto-build on `valv claude` / `valv codex` launch, mirroring the existing `EnsureLatest` behavior at `ensureClaudeImageCurrent`/`ensureCodexImageCurrent` (`claude.go:189-206`, `codex.go:227-244`). New function `ensureProjectImage(...)` runs AFTER `ensureClaude/CodexImageCurrent` in both launchers. Falls back to base ref when manifest is empty. No explicit `valv image build` command in this drop."
- PLAN.md:124 — Acceptance Criteria 6 enforces post-EnsureLatest ordering in `runClaudeCommand`/`runCodexCommand`.

### Delta 12 — U3 confirmation (label-only cleanup)

**Status:** PASS

- PLAN.md:101 — Schema Decision 11: "Per-project images carry `io.valv.managed=true` (already filtered by `valv image cleanup`) AND `io.valv.scope=project-overlay` (for future targeted cleanup). v1 leaves orphans on disk after a tools-hash bump; `valv image cleanup` can already reach them via the `managed=true` filter — operator can prune manually until a future drop adds bind-tracking GC."
- Schema Decision 4 (line 55-56) duplicates the managed/scope label declarations for completeness.
- This is consistent with Delta 5's evidence.

## New Findings (Round 2)

### F5 — Unit 12.0 missing `ARG TARGETARCH` redeclaration inside build stage (BLOCKING)

**Severity:** blocking

**Where:**
- PLAN.md:97 (Schema Decision 9) — "download official binary tarball from `https://go.dev/dl/go1.26.1.linux-${TARGETARCH}.tar.gz`"
- PLAN.md:147 (Unit 12.0 acceptance) — "The step downloads `https://go.dev/dl/go1.26.1.linux-${TARGETARCH}.tar.gz` ... Use `${TARGETARCH}` so amd64 + arm64 hosts both work (buildx auto-injects this)."
- PLAN.md:286 (Notes For Builder Agents) — "`${TARGETARCH}` is auto-supplied by buildx for amd64/arm64."

**Problem:**
Per the BuildKit / Docker reference (Context7 `/docker/docs`):
- "Automatic platform ARG variables provided by BuildKit are defined in the global scope and **are not automatically available inside build stages or for RUN commands**. To make these arguments accessible within a build stage, they must be redefined without a value, for example, by adding `ARG TARGETPLATFORM` in the Dockerfile."

The plan tells the builder to inline `go1.26.1.linux-${TARGETARCH}.tar.gz` directly into a `RUN curl ...` command, but does NOT instruct the builder to add `ARG TARGETARCH` inside the build stage of `DefaultCodexDockerfile` / `DefaultClaudeDockerfile`. Without that redeclaration, `${TARGETARCH}` expands to the empty string inside the RUN, the URL becomes `https://go.dev/dl/go1.26.1.linux-.tar.gz`, and the download 404s at build time.

**Verifiable by re-reading source:** `DefaultCodexDockerfile()` at `internal/services/images/service.go:645-678` shows the existing pattern is `ARG VALV_UID=1000` / `ARG VALV_GID=1000` / `ARG CODEX_VERSION` / `ARG CLAUDE_VERSION` all explicitly declared in-stage. There is no current `ARG TARGETARCH`. The new step needs an explicit `ARG TARGETARCH` line inserted before the `RUN` that consumes it.

**Required fix:** Unit 12.0 acceptance must explicitly require a stage-local `ARG TARGETARCH` declaration (no default value) before the `curl ... ${TARGETARCH}` RUN, OR pin to `linux-amd64` (single-arch) and accept the macOS-Apple-Silicon-via-Rosetta hit — the latter contradicts the plan's stated arm64 support. Either fix is small. Without it, Unit 12.0 lands broken and the entire drop's first-launch path fails for users.

**Suggested phrasing to add to Unit 12.0 acceptance:**
"Insert `ARG TARGETARCH` (no default value) inside the build stage before the Go install RUN line. Per Docker BuildKit reference, automatic platform ARGs are global-scope only and require in-stage redeclaration to be visible to RUN commands. Tests assert the substring `ARG TARGETARCH` appears in both Dockerfile outputs."

### F6 — Unit 12.0 Go install RUN sh-vs-exec form unspecified (NON-BLOCKING but should clarify)

**Severity:** non-blocking, recommended

**Where:** PLAN.md:147 — "The step downloads ... extracts cleanly into `/usr/local/go`, removes the tarball, and emits the resulting `go` binary on PATH via `ENV PATH=/usr/local/go/bin:$PATH`."

**Observation:** Unit 12.2 carefully mandates exec-form for the overlay (injection-safety rationale, lines 81-83). Unit 12.0 leaves the base-image Go install RUN form unspecified. The Go download step inherently needs shell features (`curl ... | tar ...` pipe, multi-command `&& rm -rf`), so shell-form is the only practical choice for the Go install RUN itself. The plan implicitly expects shell-form here but does not say so. Clarifying this would prevent a builder from over-applying the exec-form rule and producing a broken multi-step Go install. Recommend a one-line note: "Unit 12.0's Go install RUN is shell-form (existing apt block at `service.go:652-654` is the precedent); the exec-form rule in Unit 12.2 applies only to the overlay-generator output, not to base-Dockerfile static RUNs."

### F7 — Unit 12.1 hash-stability snapshot pin will break on Unit 12.0 baseline shift (NON-BLOCKING but worth flagging)

**Severity:** non-blocking, advisory

**Where:** PLAN.md:149 (Unit 12.0 acceptance: "pin the baseline via a hardcoded sha256 string updated in this unit's commit") + PLAN.md:177 (Unit 12.1 acceptance: "Hash-stability snapshot: hardcoded manifest → hardcoded expected hex").

**Observation:** These are independent hashes (Unit 12.0 pins `recipeHash()` of the base Dockerfile; Unit 12.1 pins `OverlayHash()` of a sample manifest). They don't actually depend on each other — verified by re-reading both acceptance bullets. No real coupling exists, so this is just a clarifying note: if a future drop changes `canonicalManifest` shape, Unit 12.1's pin must update; if a future drop changes the base Dockerfile, Unit 12.0's pin must update. Both pins are independently correct.

## Hylla Feedback

No Hylla queries were issued for this Round 2 review — the verification surface was (a) re-reading the revised `PLAN.md` end-to-end and (b) re-reading the specific source-file line ranges the plan cites (`internal/services/images/service.go:537-742`, `597-610`, `645-742`; `internal/cli/manage.go:1685-1689`; `go.mod`). Source-file line-range Reads are the correct evidence source for "does the plan's cited line range say what the plan claims it says" — Hylla would not improve on that. The Context7 lookup on Docker BuildKit `TARGETARCH` semantics produced finding F5 directly. No miss to record.

## Reviewer summary

Round 2 cleanly addressed eleven of twelve Round 1 carry-forwards (F1–F4, Attacks 2–4, Caveats 1/8/10, U1/U2/U3 confirmations). The new Unit 12.0 grounding correctly cites `service.go:645-742` for `DefaultCodexDockerfile` / `DefaultClaudeDockerfile`, `go.mod` line 3 confirms `go 1.26.1`, `service.go:537-549` is verified as `recipeHash()` (Dockerfile-content sha256), and `manage.go:1685-1689` is verified as the `io.valv.managed=true` cleanup filter. Schema Decisions 4–6, 8, 10, 11, 13 are tight.

The single blocking gap is F5: the plan tells the builder to use `${TARGETARCH}` inside a RUN command but does not require the matching in-stage `ARG TARGETARCH` redeclaration, which is mandatory per the Docker BuildKit reference. Without that one extra line in each Dockerfile, the Go install URL builds to `linux-.tar.gz` and 404s. F6 (shell vs exec-form clarification for Unit 12.0) and F7 (snapshot-pin independence note) are recommended-but-non-blocking polish.

Recommend planner adds the `ARG TARGETARCH` redeclaration to Unit 12.0's acceptance criteria and optionally addresses F6/F7, then re-routes for Round 3 plan-QA.
