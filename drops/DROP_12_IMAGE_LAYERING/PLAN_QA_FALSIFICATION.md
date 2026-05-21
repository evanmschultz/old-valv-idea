# DROP_12 Plan QA Falsification — Round 2

**Verdict:** fail
**Reviewer:** ta-go-qa-falsification
**Reviewed at:** 2026-05-21T19:47:12Z

## Round 1 Attack Re-verification

### R1-Attack 2 (managed label cleanup-filter reachability) — VERIFIED MITIGATED

Live filter at `internal/cli/manage.go:1687` is `"label": "io.valv.managed=true"`. The same filter appears at `manage.go:1632`, `manage.go:1878`, `cleanup/service.go:227` (test). Round 2 decision 4 adds `io.valv.managed=true` to per-project images, so `valv image cleanup` will match and prune them — the gap Round 1 identified is closed.

Note (NOT a counterexample, but a contract clarity issue): `io.valv.scope` is **already** an established label key with multiple in-use values across the codebase:

- `service.go:336` — base provider images: `io.valv.scope=image`
- `cli/claude.go:150` / `cli/codex.go:158` — info containers: `io.valv.scope=info`
- `services/claude/service.go:316` / `services/codex/service.go:320` — interactive containers: `io.valv.scope=interactive`

Round 2 introduces a fourth value `io.valv.scope=project-overlay`. The plan does not enumerate the existing scope vocabulary or explain that `project-overlay` is one of N values rather than a fresh label. This is documentation completeness, not correctness — recommend the plan add a one-line cross-reference under decision 4 ("scope value joins existing values 'image' / 'info' / 'interactive'"). Reachability-wise, the cleanup filter operates on `io.valv.managed=true` only and does not inspect `io.valv.scope`, so cleanup behavior is unchanged: base + version + project-overlay images are ALL pruned together. The plan's decision 11 implicitly relies on this — make it explicit.

### R1-Attack 3 (exec-form RUN + ENV inheritance + JSON escaping) — VERIFIED MITIGATED

Three sub-claims verified:

1. **Exec-form RUN bypasses `/bin/sh -c`** — confirmed via Context7 `/docker/docs`: *"The exec form is parsed as a JSON array... It does not automatically invoke a command shell, so normal shell processing like variable substitution does not happen."* A malicious `Source = "...; rm -rf /"` becomes a single literal argv entry passed to `go install`, which will reject it as a malformed module path. No injection surface remains.

2. **ENV inheritance into exec-form RUN** — confirmed safe. `ENV` instructions set the **container process environment**, which the binary spawned by exec-form `RUN ["go", "install", ...]` inherits via the normal `execve` environment. The exec-form bypass concerns shell-level `$VAR` substitution **in the RUN line text**, NOT the process environment of the spawned binary. `GOBIN=/usr/local/bin` set via `ENV` at the top of the overlay reaches `go install` as expected. The plan's "no per-RUN env prefix" guidance at decision 6 is correct and important — `RUN ["GOBIN=...", "go", ...]` would attempt to exec a binary literally named `GOBIN=...`.

3. **JSON escaping of argv slice** — empirically verified via scratch program. `json.Marshal` correctly escapes `"`, `\`, `\n` and other control characters. Unicode passes through as literal UTF-8 (NOT `\uXXXX`-escaped). HTML-special chars (`<`, `>`, `&`) are escaped to `<` / `>` / `&` by default `json.Marshal` (HTML-safe mode), but Docker BuildKit's JSON parser reverses the escape when parsing the RUN array, so the argv passed to the binary is the original literal. Scratch reproducer used `encoding/json` directly with no `SetEscapeHTML(false)` configuration and confirmed determinism (Marshal of the same input twice produces byte-identical output). Reproducer deleted.

### R1-Attack 4 (Go toolchain in base) — PARTIALLY MITIGATED, NEW GAPS BELOW

Three sub-claims:

1. **Tarball URL existence** — confirmed via `curl -sI`:
   - `https://go.dev/dl/go1.26.1.linux-amd64.tar.gz` → 302 → `dl.google.com/go/...` → 200 (66,791,587 bytes)
   - `https://go.dev/dl/go1.26.1.linux-arm64.tar.gz` → 302 → 200 (63,690,198 bytes)
   - Image-size impact ~63-66MB compressed / ~150MB extracted — plan estimate accurate.

2. **`${TARGETARCH}` substitution** — **NEW PLAN GAP IDENTIFIED, SEE COUNTEREXAMPLE 1 BELOW.**

3. **sha256 trust model** — **PLAN GAP, see Counterexample 2 below.**

## New Counterexamples (Round 2)

### Counterexample 1 (CONFIRMED): `${TARGETARCH}` requires explicit `ARG TARGETARCH` declaration inside the stage

**Severity:** medium — `mage testPkg` passes but `docker build` 404s at runtime.

Per Context7 `/docker/docs` ("Automatic platform ARGs in the global scope"):

> Automatic platform ARG variables provided by BuildKit are defined in the **global scope** and are **not automatically available inside build stages or for RUN commands**. To make these arguments accessible within a build stage, they must be redefined without a value, for example, by adding `ARG TARGETPLATFORM` in the Dockerfile.

The plan's Unit 12.0 acceptance and Notes line 286 say: *"`${TARGETARCH}` is auto-supplied by buildx for amd64/arm64. Add `curl` to the apt list..."* — this is **incorrect** for use inside a RUN command. A builder following the plan literally could write:

```dockerfile
FROM node:22-bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends bubblewrap ca-certificates curl git ncurses-term
RUN curl -fsSL https://go.dev/dl/go1.26.1.linux-${TARGETARCH}.tar.gz | tar -xz -C /usr/local
```

With this Dockerfile, `${TARGETARCH}` evaluates to empty inside the shell-form RUN (Dockerfile ARG scope is empty for this name), producing URL `https://go.dev/dl/go1.26.1.linux-.tar.gz` → 404. Unit 12.0's text-substring test (`go1.26.1.linux`, `/usr/local/go/bin`) PASSES because those substrings are present, but real `docker build` fails.

**Fix:** Unit 12.0 acceptance must require an `ARG TARGETARCH` declaration inside the Dockerfile stage before the `RUN curl ... ${TARGETARCH} ...` line. Add the corresponding test assertion that the Dockerfile contains a literal `ARG TARGETARCH` line. Also: shell-form RUN (with `${TARGETARCH}` expansion) is required here — exec-form RUN does NOT do `${VAR}` expansion (per the exec-form vs shell-form contract Round 2 already relies on for the overlay). The plan should explicitly note that the **base** Dockerfile uses shell-form for the Go install step while the **overlay** Dockerfile uses exec-form for tool installs.

### Counterexample 2 (ACCEPTED-DOCUMENT): No sha256 verification of the Go tarball

**Severity:** low — TLS-only trust; defensible but should be explicit.

The plan downloads `go1.26.1.linux-${TARGETARCH}.tar.gz` over HTTPS and extracts without sha256 verification. Trust chain: dl.google.com TLS cert. If dl.google.com is compromised OR the Docker build host's CA store is compromised, a malicious tarball could be substituted and a backdoored Go toolchain runs `go install` during every per-project overlay build, exfiltrating credentials from the container or producing malicious binaries.

Counter-argument the plan implicitly makes: the base image also `npm install -g`s two CLIs from the npm registry without sha256 pinning, so a marginal threat increase from one more unpinned download is minimal. DROP_15 (closed-by-default network) will eventually moot this by blocking outbound builds from contacting the open internet.

**Recommendation:** either pin sha256 (the Go team publishes `https://go.dev/dl/?mode=json` with sha256 values) OR add a one-line note to decision 9 explicitly accepting "TLS-only trust for the Go tarball, consistent with existing npm install trust; DROP_15 will harden". Pick one — the current plan does neither.

### Counterexample 3 (CONFIRMED): Unit 12.4 reorder of `claudeservice.New` / `codexservice.New` is implicit

**Severity:** medium — easy to land Unit 12.4 with stale base ref still passed to service constructor.

`internal/cli/claude.go:103-115` currently constructs `claudeservice.New(claudeservice.Options{Image: claudeImageRef(), ...})` at line 103 **BEFORE** calling `ensureClaudeImageCurrent` at line 121. The plan acceptance says: *"`runClaudeCommand` calls `resolveProjectImage` AFTER `ensureClaudeImageCurrent`, passes the resolved ref into `claudeservice.New(Options{Image: projectImage, ...})`"*. To make this work, the `claudeservice.New(...)` call must MOVE to after both `ensureClaudeImageCurrent` and `resolveProjectImage`. The plan does not explicitly call out this reorder as a step — a builder reading the plan literally could add the `resolveProjectImage` call but leave the existing `claudeservice.New(Image: claudeImageRef())` untouched, then mutate `service` in-place (which is impossible — `Options.Image` is consumed at construction time). The result would be the base ref still being used at run time, with `resolveProjectImage` having no effect.

Same pattern in `internal/cli/codex.go:110` for `codexservice.New`.

**Fix:** Unit 12.4 acceptance must explicitly say "reorder `claudeservice.New(...)` to AFTER `ensureClaudeImageCurrent(...)` and AFTER the new `resolveProjectImage(...)` call so the resolved ref is passed at construction time". Same wording for `runCodexCommand`. Add a test assertion that the constructed service receives the project-overlay ref (not the base ref) when manifest is non-empty.

### Counterexample 4 (CONFIRMED, LOW): Internal-whitespace `Source` produces a confusing build-time error

**Severity:** low — UX issue, not security.

`internal/tools/Validate` rejects only empty Source/Install for object-form tools; the regex applies only to the map key. A user with a typo like `source = "github.com/x/y\tcommit-msg"` (literal tab in the middle) passes `Validate` (Source non-empty), survives `canonicalManifest` (TrimSpace only strips leading/trailing), and reaches `BuildOverlayDockerfile`. Plan acceptance says "Empty `Source` or `Install` (after canonicalManifest trim) → wrapped error" — which catches whitespace-only strings but NOT internal whitespace.

Net result: Docker build proceeds, `RUN ["go", "install", "github.com/x/y\tcommit-msg"]` is emitted, BuildKit parses it as a literal JSON string, `go install` receives the tab-containing argument and produces an opaque "malformed module path" error at image-build time — diagnosed only by reading container build logs.

**Fix (optional, low priority):** add a `strings.ContainsAny(source, " \t\n\r")` rejection at overlay-generation time with a wrapped error like `"build overlay dockerfile: tool %q source contains whitespace"`. OR explicitly accept and document that whitespace-internal Source values produce build-time errors. The plan currently does neither; resolution is a one-line plan addendum.

### Counterexample 5 (ACCEPTED): Concurrent `EnsureProjectImage` race

**Severity:** low — inherits existing Valv concurrency model.

Two concurrent `valv claude` launches against the same project (e.g. two terminal panes) both call `tools.Resolve` → same manifest → same `OverlayHash` → both call `EnsureProjectImage`. If the target tag doesn't exist yet, both invoke `docker buildx build --load` for the same tag. Docker handles concurrent identical tag writes — typically the second `--load` succeeds because the image already exists by the time it lands, OR both succeed with the second being a no-op. Worst case: one of them fails with a transient error and the user retries.

This race **already exists** for `EnsureLatest` on the base image — the plan inherits Valv's prior choice not to lock around image builds. The plan does not explicitly accept this; recommend a one-line note under decision 10: "concurrent launches share the cache race already accepted in `EnsureLatest`; no per-project lock added".

### Counterexample 6 (NOT A BUG, NOTE ONLY): Double Docker round-trip at launch

`runClaudeCommand` calls `ensureClaudeImageCurrent` (which calls `EnsureLatest` → `docker image inspect`) then `resolveProjectImage` (which calls `EnsureProjectImage` → another `docker image inspect`). Two inspects per launch when both images are up-to-date. ~50-200ms latency cost on a warm Docker daemon. Acceptable. Plan does not call this out but it is not a blocker.

### Counterexample 7 (TAG COLLISION — STATISTICALLY NEGLIGIBLE BUT WORTH DOCUMENTING)

`proj-<short-tools-hash>` uses the first 12 hex chars = 48 bits. Birthday collision at ~2^24 ≈ 16M distinct manifests on the same host. For a single-developer workflow, negligible. The plan's `tools_hash` label stores the FULL 64-char sha256, so on a collision, `EnsureProjectImage` reads the existing image's full `tools_hash`, sees a mismatch with the new manifest's full hash, and rebuilds — overwriting the prior `proj-<short-hash>` tag. Net effect: each project always sees its own correct image; the orphaned previous-tenant image is leaked (until `valv image cleanup`). **Plan is correct via the full-hash safety net.** Note for completeness, not a counterexample.

### Counterexample 8 (TARBALL-FETCH FAILURE MODES, LOW)

If `curl https://go.dev/dl/...` fails during `docker build` (network blip, dl.google.com 5xx, host DNS issue), the build fails with an opaque shell-level error. Plan acceptance does not specify a retry policy. The `EnsureLatest` flow at the orchestrator level retries the entire build path naturally on the next `valv claude` launch, so transient failures self-heal. Acceptable as-is. Note: if a builder uses `&& \` line continuations for the Go install step (e.g. `curl ... && tar -xz ... && rm ...`), any failed step fails the layer cleanly — no partial-state risk.

## YAGNI Pressure

### YAGNI 1: `BaseRecipeHash` field on `EnsureProjectResult`

The result struct has fields `{Image docker.ImageRef; Action EnsureAction; ToolsHash string; BaseRecipeHash string}`. Plan acceptance #11 tests **the labels on the built image**, not these result-struct fields. No CLI guidance reads `BaseRecipeHash` from the result. Recommend either:

- drop `BaseRecipeHash` and `ToolsHash` from the result entirely (the labels are the source of truth), OR
- document explicitly in the plan WHY these are returned (debug logging via `logger.Debug`?).

### YAGNI 2: `ShortOverlayHash` as an exported helper

Plan acceptance for Unit 12.1: "Add helper `ShortOverlayHash(manifest) string` returning the first 12 hex chars (used in the tag)." Only call site identified in the plan is inside `Service.projectImageRef(toolsHash)` (Unit 12.3 step 3). If it has only one in-package caller, it should be lowercase / unexported (`shortOverlayHash` or just inlined as `toolsHash[:12]`). Exported API surface should justify itself; this currently doesn't.

### YAGNI 3: Defensive empty-check on Source/Install in `BuildOverlayDockerfile`

`tools.Validate` already rejects empty Source/Install (object form). The plan's "Empty `Source` or `Install` (after canonicalManifest trim) → wrapped error" is needed ONLY for the whitespace-only edge case (`"   "` passes Validate but trims to empty). Plan is correct to keep it — but the rationale should be one-lined ("catches whitespace-only inputs that pass Validate"). Without that, a future reader will see redundant-looking validation and consider deleting it.

## Hylla Feedback

Hylla `hylla_search_keyword` returned a near-empty result set for queries `io.valv.managed`, `io.valv.scope`, `recipeHashLabel` — only `Service.Build` surfaced once. The labels are string literals scattered across multiple files (cli/, services/claude/, services/codex/, services/images/, adapters/docker/), and Hylla's keyword index appears not to surface block-content matches for raw string literals embedded in map literals. Fallback to `/usr/bin/grep -rn` was required to enumerate the existing scope-value vocabulary (Counterexample R1-Attack 2 note). Recommend Hylla index string-literal matches inside map composite literals more aggressively, OR provide a content-search mode that does not require the literal to be associated with a named symbol.

## Summary

Verdict **fail** due to **three CONFIRMED counterexamples** the builder is likely to hit:

1. **Counterexample 1** — `${TARGETARCH}` requires `ARG TARGETARCH` inside the stage. Plan's text-substring test cannot catch the missing ARG; runtime `docker build` will 404. Must fix before build.
2. **Counterexample 3** — Unit 12.4 must explicitly reorder `claudeservice.New` / `codexservice.New` to AFTER the new `resolveProjectImage` call. A literal reading of the current plan can leave the base ref hard-wired at service construction time.
3. **Counterexample 2** — sha256 trust model should be explicit (either pin OR accept-and-document).

The remaining counterexamples (4-8) and YAGNI items are minor and can land as one-line plan addenda alongside the three blocking fixes.
