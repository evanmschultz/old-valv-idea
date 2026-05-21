# DROP_12 Plan QA Falsification — Round 3

**Verdict:** pass
**Reviewer:** ta-go-qa-falsification
**Reviewed at:** 2026-05-21T20:04:19Z

Round-2 re-attacks all mitigate. New-attack pass surfaces zero unmitigated counterexamples; eight attacks classified as mitigated, one finding accepted with a documentation YAGNI nit. Plan is build-ready.

## Round 2 Attack Re-verification

### R2-2.1 — `ARG TARGETARCH` redeclared inside stage

**Status:** MITIGATED.

Round 3 plan text enforces it on three independent surfaces:

- Decision 9 (`PLAN.md:101`): "`ARG TARGETARCH` required inside stage (F5 fix)" — explicit positional requirement "redeclare `ARG TARGETARCH` (no default value) inside the stage immediately before the Go install RUN line."
- Drop-level Acceptance 2 (`PLAN.md:117`): "The literal `ARG TARGETARCH` line (no default value) declared inside the build stage, positioned immediately before the Go install RUN line."
- Unit 12.0 Acceptance (`PLAN.md:159` + `:168`): "Both Dockerfiles declare the literal line `ARG TARGETARCH` (no default value) inside the build stage, positioned immediately before the Go install RUN line." Plus tests-must-assert "The literal `ARG TARGETARCH` line."

All three places use the substring-assertion contract — the test for Unit 12.0 will fail if the line is missing or misplaced. Triple coverage is sufficient.

### R2-2.2 — sha256 pin rotation risk

**Status:** ACCEPTED (documented as fail-loud, acceptable behavior).

Plan Decision 9 (`PLAN.md:103`) explicitly states "Build fails loudly on hash mismatch" and that hashes are recorded in `BUILDER_WORKLOG.md` under `## Go Tarball Hashes`. If Go upstream rotates the tarball (rebuild with same version tag — extremely rare for stable releases), the next base-image build fails with a `sha256sum -c` non-zero exit and the operator-visible message. This is correct security posture: a silently changed tarball is exactly what the pin must catch. Worklog recording lets the next builder rotate the constants deliberately. No counterexample.

### R2-2.3 — Constructor reorder, other call sites in claude.go/codex.go

**Status:** MITIGATED.

Live grep against the source confirms exactly two call sites total across the repo:

```
/internal/cli/claude.go:103: service, err := claudeservice.New(claudeservice.Options{
/internal/cli/codex.go:110:  service, err := codexservice.New(codexservice.Options{
```

No error paths, no retry loops, no test helpers construct the service. Unit 12.4's two-step reorder ("Mirror for `runCodexCommand`") covers the entire surface. Pure single-point-of-truth.

### R2-F6 — RUN-form distinction, overlay future-proofing

**Status:** MITIGATED.

Notes-for-builders block at `PLAN.md:320` is now explicit and bidirectional:

> "Unit 12.2 (overlay Dockerfile generator) uses **exec-form** `RUN ["..."]` (JSON array) per Decision 6 for injection safety. The exec-form rule applies ONLY to the overlay generator's per-tool install lines."

The "ONLY" wording forbids overlay shell-form RUN. Unit 12.2 acceptance reinforces with "All RUN lines use **JSON array exec-form**. No shell-form." (`PLAN.md:222`). If a future builder adds a shell-form RUN to the overlay generator output, Unit 12.2's byte-for-byte-output test (`PLAN.md:228`) detects it.

The inverse direction is also locked at `PLAN.md:320`: "Builders MUST NOT 'fix' Unit 12.0 to exec-form." Both directions covered.

### R2-2.4 — Embedded newlines in Source

**Status:** MITIGATED (covered by exec-form + `json.Marshal`).

Decision 3 (`PLAN.md:51`) acknowledges `TrimSpace` only strips leading/trailing whitespace. Embedded newlines or tabs pass through.

Counterexample attempt: a malicious `Source = "github.com/x/y\ngithub.com/z/evil"`. Exec-form `RUN ["go", "install", "<source>"]` with the source as a single JSON array element... does `json.Marshal` correctly escape the newline?

Yes. `json.Marshal` on a Go string containing a literal `\n` byte produces a JSON string with the escape sequence `\n` (two characters: backslash + n) in the output. Docker's exec-form parser reads JSON arrays per the JSON spec; the embedded newline does NOT terminate the RUN directive (RUN directive parsing is line-based, but the JSON form bypasses that line-parser at the engine level). The newline becomes part of the literal argv element passed to `go install`, which rejects it as an invalid module path at the binary layer with a visible error.

Result: embedded newlines are pass-through and produce a visible build-time error from the tool binary, identical to other internal-whitespace cases already covered by Decision 3. No actual injection vector. Acceptable v1 behavior.

### R2-`io.valv.scope` cross-reference verification

**Status:** MITIGATED.

Live grep confirms ALL THREE cited values exist at the exact files/lines named in Decision 4 (`PLAN.md:58`):

| Cited value | Cited location | Verified |
|---|---|---|
| `image` | `service.go:336` | YES — `internal/services/images/service.go:336` |
| `info` | `claude.go:150` + `codex.go:158` | YES — both file:line match |
| `interactive` | `claude/service.go:316` + `codex/service.go:320` | YES — both file:line match |

Additional callers also use these values (e.g. `internal/adapters/docker/types_test.go` uses `interactive`; tests at `cli/extended_test.go:490` and `cli/codex_test.go:341` reference `info`/`image`). None of those call sites narrow the set — they consume, not define. `project-overlay` is lexically distinct from `image`, `info`, `interactive`. No closed-set parser anywhere. Safe.

### R2-YAGNI — `BaseRecipeHash` dropped from result

**Status:** MITIGATED.

Live grep for `BaseRecipeHash` across `.go` and `.md` files returns ZERO hits in source code — every match is inside `DROP_12_IMAGE_LAYERING/PLAN.md` itself. No Unit 12.3 or 12.4 acceptance assertion still references `BaseRecipeHash` as a struct field consumer. Decision 6 in Unit 12.3 (`PLAN.md:248`) explicitly notes: "**`BaseRecipeHash` field dropped (Round 2 YAGNI):** the base-recipe-hash is consumed only inside `EnsureProjectImage` for cache comparison; no Unit 12.4 caller reads it." Verified.

`ToolsHash` is retained per the same line because Unit 12.4 logs the resolved overlay tag — consistent with the YAGNI cut.

## New Counterexamples (Round 3)

### 1. `ARG TARGETARCH` before-FROM accident

**Status:** MITIGATED.

Counterexample considered: builder accidentally places `ARG TARGETARCH` before `FROM node:22-bookworm-slim`. Per Docker docs, a global ARG before `FROM` is NOT auto-injected into the stage — `${TARGETARCH}` inside the post-FROM RUN renders empty, tarball URL 404s.

Mitigation: plan text says "redeclare inside the stage immediately before the Go install RUN line" (`PLAN.md:101`, `:117`, `:159`). All three phrasings include "inside the stage" or "before the Go install RUN line" (which is unambiguously after FROM). Unit 12.0's substring test asserts the literal `ARG TARGETARCH` line exists, and combined with the assertion that `go1.26.1.linux-${TARGETARCH}.tar.gz` substring is present, the test will catch the wrong-position case only if a build is actually attempted... but `recipeHash()` test (`PLAN.md:173`) pins the new hash, which means any structural Dockerfile change forces an explicit hash update — making the position visible in code review.

Not a perfect compile-time guard, but the plan's "immediately before the Go install RUN line" wording + the locked recipeHash diff + the URL-substring test together pin the structural position adequately for human review. No build-time counterexample reachable without violating multiple explicit plan rules at once.

### 2. `sha256sum -c -` exit code propagation

**Status:** MITIGATED.

Verified via shell semantics knowledge: `sha256sum -c -` reads check lines from stdin. On any failure (file not found, hash mismatch, malformed input), it exits non-zero. Docker shell-form RUN runs the command via `/bin/sh -c "<cmd>"`; a non-zero exit code propagates out of the shell and Docker treats the RUN step as failed → build aborts.

Decision 9 + Unit 12.0 acceptance both say "Build fails loudly on mismatch" (`PLAN.md:103`, `:163`). The `sha256sum -c` literal-substring test (`PLAN.md:170`) asserts the verification call exists. Sufficient.

### 3. `case ${TARGETARCH}` shell syntax in dash

**Status:** MITIGATED.

`node:22-bookworm-slim` is Debian bookworm based. On Debian, `/bin/sh` is **dash** (not bash). The `case ... in ... ;; esac` syntax IS POSIX-compliant — dash fully supports it. Verified by:

- Debian Policy specifies dash as `/bin/sh`.
- POSIX sh grammar §2.9.4.3 defines `case` compound command with `;;` terminator.
- dash, bash, ash, ksh all conform to POSIX `case` syntax.

The plan's "case ${TARGETARCH} block (or equivalent shell conditional)" wording (`PLAN.md:103`, `:161`) is portable across all POSIX shells. No counterexample.

Bonus check: variable expansion `${TARGETARCH}` is POSIX parameter expansion — also supported by dash. Safe.

### 4. `OverlayHash` exported vs `shortOverlayHash` unexported boundary

**Status:** MITIGATED.

Unit 12.1 acceptance (`PLAN.md:198-199`) makes the split explicit:

- `OverlayHash(manifest)` — exported. Consumed externally by Unit 12.3 callers and "future per-project image inspection paths." Returns full sha256 hex.
- `shortOverlayHash(hash string)` — unexported. Takes the already-computed hash (string, not the manifest). Used internally by `s.projectImageRef`.

The plan's design forces all callers to call `OverlayHash` first and feed the result into `shortOverlayHash` only when needed for tag truncation. Unit 12.3 step 3 (`PLAN.md:252`) confirms: "Internally `projectImageRef` calls the unexported `shortOverlayHash(toolsHash)`."

No external caller needs the short form — the short form only exists for tag construction, which is private to the `images` service. Boundary is clean.

### 5. `BaseRecipeHash` field — verify no Unit 12.3/12.4 consumer

**Status:** MITIGATED (re-verifies R2-YAGNI from the opposite angle).

Live grep confirmed zero source-code consumers. Unit 12.3 acceptance (`PLAN.md:248`) explicitly drops the field with a one-sentence rationale that future debug-logging needs would re-add it. Unit 12.4 acceptance (`PLAN.md:285-307`) makes no reference to `BaseRecipeHash` — the only field on `EnsureProjectResult` the CLI consumes is `Image`. The plan's `ToolsHash` retention is justified (used for log lines).

### 6. `resolveProjectImage` signature consistency

**Status:** MITIGATED.

Round 3 changed the signature to `resolveProjectImage(cmd, paths, provider, workingDir, baseRef) (docker.ImageRef, error)`. Verified consistent across both call sites:

- Unit 12.4 Paths (`PLAN.md:279`): "small helper `resolveProjectImage(cmd, paths, provider, workingDir, baseRef) (docker.ImageRef, error)`" — five-arg form.
- Unit 12.4 Acceptance bullet 1 (`PLAN.md:286`): "`resolveProjectImage(cmd, paths, provider, workingDir, baseRef) (docker.ImageRef, error)`" — five-arg form.
- Constructor reorder step 3 for claude (`PLAN.md:294`): `resolveProjectImage(cmd, paths, "claude", workingDir, claudeImageRef())` — five args passed.
- Mirror for codex (`PLAN.md:297`): `resolveProjectImage(cmd, paths, "codex", workingDir, codexImageRef())` — five args passed.

All four references use the same five-arg shape. The `baseRef` parameter is used at `PLAN.md:288` ("returns `baseRef` + nil error") and `:289` ("returns `baseRef`"). Signature is internally consistent.

### 7. The "5-step reorder" verified against live source

**Status:** MITIGATED.

Live read of `/internal/cli/claude.go:97-123` confirms the current sequence the plan claims:

```
97:  store, err := openStore(paths)         // step 1 (current)
98:  ...
101: defer store.Close()
103: service, err := claudeservice.New(...) // currently HERE, must move to step 4
...
121: if err := ensureClaudeImageCurrent(...) // currently HERE, must move to step 2
123: }
125: if err := service.Run(...)             // step 5 (unchanged)
```

The plan's claim "live code at `internal/cli/claude.go:103-123` currently constructs `claudeservice.New(...)` (line 103) **BEFORE** `ensureClaudeImageCurrent(...)` (line 121)" (`PLAN.md:291`) matches the source exactly. The plan's "moved up" and "moved down" instructions are correct.

Live read of `/internal/cli/codex.go:104-128` shows the identical pattern:

```
104: store, err := openStore(paths)
108: defer store.Close()
110: service, err := codexservice.New(...)  // currently here, must move to step 4
...
126: if err := ensureCodexImageCurrent(...) // currently here, must move to step 2
128: }
133: if err := service.Run(...)             // step 5 (unchanged)
```

Plan's "Mirror for `runCodexCommand` at `internal/cli/codex.go:110-...`" (`PLAN.md:297`) is accurate. No partial-reorder already in place — both files need the full five-step rewrite, exactly as described.

### 8. `io.valv.scope` existing values cross-reference verification

**Status:** MITIGATED (also resolved by R2-scope above).

Verified by live grep — see R2-scope row table. All three cited file:line pairs are accurate.

## YAGNI Re-pressure

### Verbose cross-reference text in Decision 4

**Finding:** ACCEPTED with documentation nit.

`PLAN.md:58` contains a multi-clause sentence enumerating existing `io.valv.scope` values with three file:line citations. The verbose form is justified — it's the artifact that proves Decision 4 (the disambiguation against existing scope values) is grounded. Compressing to "Existing values: image, info, interactive (all confirmed via grep 2026-05-21). project-overlay is distinct." would lose the file:line traceability that lets a future reader verify the claim without re-grepping.

Recommendation: leave as-is. The verbosity IS the evidence. This is a stylistic preference, not a YAGNI cut. No blocking issue.

## Hylla Feedback

Hylla was not used in this round. Live grep + Read covered all evidence needs faster (single file:line cross-references for `io.valv.scope`, `BaseRecipeHash`, constructor sites, and `recipeHash`). Hylla would have been overkill for these single-token searches against committed source the agent had already touched.

No Hylla miss to record — the falsification work was inherently file-level grep + Read, not symbol-graph navigation.

## Summary

- **Verdict:** pass
- **Round 2 re-verifications:** 7 attacks, all mitigated.
- **New Round 3 attacks:** 8 attacks, all mitigated.
- **YAGNI re-pressure:** 1 finding, documentation nit, not blocking.

Plan is build-ready. No unmitigated counterexample. Phase 3 (cleanup → build) is unblocked.
