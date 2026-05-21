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
