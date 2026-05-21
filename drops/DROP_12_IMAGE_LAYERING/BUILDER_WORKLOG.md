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

