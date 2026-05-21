# DROP_12 Plan QA Proof — Round 3

**Verdict:** pass
**Reviewer:** ta-go-qa-proof
**Reviewed at:** 2026-05-21T00:00:00Z

## Round 2 Delta Verification

### Delta 1 — F5/2.1 `ARG TARGETARCH` in-stage redeclaration

**Verdict:** pass

Evidence:
- Decision 9, PLAN.md:101-102 — "`ARG TARGETARCH` required inside stage (F5 fix). Per Docker BuildKit docs, automatic platform ARGs ... live in **global scope only** ... Both `DefaultCodexDockerfile` and `DefaultClaudeDockerfile` MUST redeclare `ARG TARGETARCH` (no default value) inside the stage immediately before the Go install RUN line."
- Acceptance Criterion 2, PLAN.md:117 — "The literal `ARG TARGETARCH` line (no default value) declared inside the build stage, positioned immediately before the Go install RUN line."
- Unit 12.0 acceptance, PLAN.md:159 — "Both Dockerfiles declare the literal line `ARG TARGETARCH` (no default value) inside the build stage, positioned immediately before the Go install RUN line."
- Unit 12.0 test assertion, PLAN.md:168 — "The literal `ARG TARGETARCH` line."
- Notes For Builder Agents, PLAN.md:319 — "**`TARGETARCH` is NOT auto-injected into build stages** per Docker BuildKit docs — both `DefaultCodexDockerfile` and `DefaultClaudeDockerfile` MUST redeclare `ARG TARGETARCH` (no default value) inside the stage."

Coverage: 5 redundant call-outs across decision, acceptance, unit acceptance, test assertion, and builder notes. The literal `ARG TARGETARCH` substring is enforced by the test step at PLAN.md:168.

### Delta 2 — Falsification 2.2 sha256 pin (5-step install)

**Verdict:** pass

Evidence:
- Decision 9, PLAN.md:103 — "sha256 pin (Falsification 2.2 fix). Tarball integrity is verified via `sha256sum -c` before extraction. The Dockerfile uses a shell-form `case ${TARGETARCH}` block (or equivalent shell conditional) to select the right hash for amd64 vs arm64, then pipes the expected hash + tarball path into `sha256sum -c -`."
- Unit 12.0 acceptance enumerated 5 steps, PLAN.md:161-165:
  1. case dispatch
  2. curl
  3. sha256sum -c verification
  4. tar extract + cleanup
  5. ENV PATH
- Literal hash deferral, PLAN.md:166 — "Literal sha256 hex values are fetched at implementation time ... and recorded in `BUILDER_WORKLOG.md` under a `## Go Tarball Hashes` heading."
- Test substrings, PLAN.md:170-172 — `sha256sum -c`, `/usr/local/go/bin`, `amd64`, `arm64` all required to appear in Dockerfile output.
- Acceptance Criterion 2, PLAN.md:119-120 — case-block + `sha256sum -c` + `ENV PATH=/usr/local/go/bin:$PATH` extraction all listed.

### Delta 3 — Falsification 2.3 constructor reorder

**Verdict:** pass

Evidence:
- Unit 12.4 acceptance, PLAN.md:291-296 — explicit five-step reorder with file:line citation `internal/cli/claude.go:103-123`:
  1. `openStore` (unchanged)
  2. `ensureClaudeImageCurrent` moved up
  3. `resolveProjectImage` new call
  4. `claudeservice.New` moved down with `Image: projectImage`
  5. `service.Run` unchanged
- Unit 12.4 acceptance, PLAN.md:297 — mirror for `runCodexCommand` at `internal/cli/codex.go:110-...`.
- Verified live code at `internal/cli/claude.go`:
  - Line 103: `service, err := claudeservice.New(claudeservice.Options{`
  - Line 106: `Image: claudeImageRef(),`
  - Line 121: `if err := ensureClaudeImageCurrent(cmd, paths); err != nil {`
  - Confirms the plan's "constructor before ensure" diagnosis is accurate.
- Verified live code at `internal/cli/codex.go`:
  - Line 110: `service, err := codexservice.New(codexservice.Options{`
  - Line 113: `Image: codexImageRef(),`
  - Line 126: `if err := ensureCodexImageCurrent(cmd, paths); err != nil {`
  - Matches plan's `codex.go:110-...` citation.

### Delta 4 — F6 RUN-form distinction

**Verdict:** pass

Evidence:
- Notes For Builder Agents, PLAN.md:320 — "**RUN-form distinction across units (F6 clarification):** Unit 12.0 (base Dockerfile) uses **shell-form** `RUN <cmd>` because the Go install step is intrinsically multi-step (case dispatch + curl + sha256 verify + tar extract + cleanup) and shell control flow + variable expansion (`${TARGETARCH}`) are required. Unit 12.2 (overlay Dockerfile generator) uses **exec-form** `RUN [\"...\"]` (JSON array) per Decision 6 for injection safety. The exec-form rule applies ONLY to the overlay generator's per-tool install lines, NOT to Unit 12.0's base-image Go install. Builders MUST NOT 'fix' Unit 12.0 to exec-form."
- Decision 6, PLAN.md:64-83 — establishes exec-form for overlay with rationale.
- Unit 12.2 acceptance, PLAN.md:222 — "All RUN lines use **JSON array exec-form** ... No shell-form."

Coverage: explicit cross-unit distinction prevents builder confusion.

### Delta 5 — Falsification 2.4 internal whitespace acknowledgement

**Verdict:** pass

Evidence:
- Decision 3, PLAN.md:51 — "**Internal whitespace pass-through (Falsification 2.4 acknowledged).** `strings.TrimSpace` strips leading/trailing whitespace only — internal whitespace ... survives both the trim and the exec-form RUN emission as a single argv element. DROP_11's `validate.go` regex validates only the tool map key, not `Source`/`Install` values, so an internally-whitespaced source reaches the overlay generator unchanged. Downstream behavior: `go install` (or `npm install -g`) receives the malformed path as one argv entry and rejects it at the binary layer with an explicit module-path error visible in `docker build` output. DROP_12 deliberately does NOT add stricter validation here."
- Notes For Builder Agents, PLAN.md:321 — "Internal whitespace in `Source`/`Install` is pass-through (Falsification 2.4 acknowledged)" with full rationale repeated.

Decision treats this as accepted-risk v1 behavior, not a fix. Acceptable Round 2 → 3 resolution.

### Delta 6 — `io.valv.scope` cross-reference

**Verdict:** pass

Evidence:
- Decision 4, PLAN.md:58 — "Existing `io.valv.scope` values in the codebase (confirmed via grep 2026-05-21): `image` (base build artifact, `internal/services/images/service.go:336`), `info` (one-shot info container, `internal/cli/claude.go:150` + `internal/cli/codex.go:158`), `interactive` (running provider container, `internal/services/claude/service.go:316` + `internal/services/codex/service.go:320`)."
- Live-code verification (Read tool, 2026-05-21):
  - `internal/services/images/service.go:336` — `"io.valv.scope": "image",` ✓
  - `internal/cli/claude.go:150` — `"io.valv.scope": "info",` ✓
  - `internal/cli/codex.go:158` — `"io.valv.scope": "info",` ✓
  - `internal/services/claude/service.go:316` — `"io.valv.scope": "interactive",` ✓
  - `internal/services/codex/service.go:320` — `"io.valv.scope": "interactive",` ✓

All five line citations are byte-accurate. `project-overlay` is lexically distinct from `image`/`info`/`interactive`.

### Delta 7 — YAGNI items (`shortOverlayHash` + `BaseRecipeHash` drop)

**Verdict:** pass

Evidence:
- Unit 12.1, PLAN.md:199 — "Add helper `shortOverlayHash(hash string) string` (**unexported**, YAGNI per Round 2) returning the first 12 hex chars of an already-computed full hash, used internally by `s.projectImageRef`. Takes a string (not the manifest) so callers do not re-hash. Not exported — only the tag-construction site inside Unit 12.3 calls it."
- Unit 12.3, PLAN.md:248 — "**`BaseRecipeHash` field dropped (Round 2 YAGNI):** the base-recipe-hash is consumed only inside `EnsureProjectImage` for cache comparison; no Unit 12.4 caller reads it. If a future debug-logging or telemetry consumer needs it, add the field at that drop."
- Unit 12.3 result type, PLAN.md:248 — `EnsureProjectResult{Image docker.ImageRef; Action EnsureAction; ToolsHash string}` (no `BaseRecipeHash`).
- Unit 12.3 caller flow, PLAN.md:251-252 — `toolsHash := OverlayHash(request.Manifest)` then `s.projectImageRef(toolsHash)` which "internally `projectImageRef` calls the unexported `shortOverlayHash(toolsHash)` from Unit 12.1 to truncate to 12 hex chars." The string-signature is correct for this caller — no re-hashing, single computation.

Signature compatibility verified: Unit 12.3 has the full hex hash in hand and passes the string into `shortOverlayHash`. Matches declared signature.

## Round 1 Earlier-Fix Persistence Check

**F1 (cache invalidation chain — five labels):** Decision 4 (PLAN.md:53-58) retains all five labels: `recipe_hash`, `base_recipe_hash`, `tools_hash`, `managed=true`, `scope=project-overlay`. PASS.

**F2 (label-read failure policy — conservative rebuild):** Decision 5 (PLAN.md:62) retained verbatim — typecast failure OR non-missing inspect error forces rebuild; `dockerImageMissingError` still short-circuits to build. PASS.

**F3 (EnsureProjectRequest shape — no `Pull`):** Decision 8 (PLAN.md:97) — "`EnsureProjectRequest{Manifest tools.ToolManifest; BaseImage docker.ImageRef; NoCache bool}`. **`Pull` field removed** — base images are locally built, never pulled from a registry." PASS.

**Attack 2 (cleanup reachability):** `io.valv.managed=true` retained in Decision 4 (PLAN.md:57), Unit 12.3 acceptance (PLAN.md:247, 254, 265), and Notes For Builder Agents (PLAN.md:322). PASS.

**Attack 3 (exec-form RUN + empty-value validation):** Decision 6 (PLAN.md:64-87) retains exec-form rationale. Decision 7 empty-value validation (PLAN.md:95) — "verifies that each tool's `Source` and `Install` are non-empty after `canonicalManifest` trimming. Empty values return a wrapped error." Unit 12.2 acceptance (PLAN.md:227) — "Empty `Source` or `Install` (after canonicalManifest trim) → wrapped error." PASS.

**Attack 4:** Not explicitly tagged in the current PLAN.md text; presumed resolved in earlier rounds and absorbed into accepted material. No contradictory evidence in current state.

## New Findings (Round 3)

None blocking.

Minor observations (not findings):

- Unit 12.1 (PLAN.md:198) describes `OverlayHash` as "consumed externally by Unit 12.3 callers". Both Unit 12.1 and Unit 12.3 live in the same `internal/services/images/` package, so "external" is technically imprecise — though export remains justified for future debug-logging/telemetry consumers in other packages. Wording-only; no behavioral impact.

- Unit 12.0 test assertions (PLAN.md:167-172) require `amd64` and `arm64` substrings + `sha256sum -c` + `/usr/local/go/bin` + the literal `ARG TARGETARCH` line + the templated `go1.26.1.linux-${TARGETARCH}.tar.gz` filename. They do NOT require the literal substring `case ${TARGETARCH}` — Decision 9 and Unit 12.0 acceptance both phrase the shell-conditional as "case ${TARGETARCH}` block (or equivalent shell conditional)", which leaves room for `if`/`elif` constructs. The test assertions accommodate this flexibility correctly. Not a contradiction.

## Hylla Feedback

None. No Hylla searches were required for Round 3 verification — all evidence came from direct PLAN.md reads + live source-file reads at cited line numbers.
