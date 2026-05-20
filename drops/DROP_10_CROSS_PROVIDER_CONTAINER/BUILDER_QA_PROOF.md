# DROP_10 — Build QA Proof

Per-round entries appended below. Each `## Unit N.M — Round K` section is a
durable proof certificate for that build round.

## Unit 10.1 — Round 1

- **Reviewer:** go-qa-proof-agent
- **Commit under review:** `47ecbc4` — `feat(images): unit 10.1 dual-CLI Dockerfiles + cross-provider build-arg`
- **Verdict:** PASS

### Acceptance criteria evidence

**AC1 — `DefaultCodexDockerfile()` contains both CLI installs + both home dirs.**
- `internal/services/images/service.go:658` — `mkdir -p /home/valv/.codex /home/valv/.claude /workspace` (both home dirs in a single RUN block, both owned by the valv user via the same `chown -R` on line 659).
- `internal/services/images/service.go:668-669` — `ARG CODEX_VERSION` + `RUN npm install --global "@openai/codex@${CODEX_VERSION}"` (primary).
- `internal/services/images/service.go:671-672` — `ARG CLAUDE_VERSION` + `RUN npm install --global "@anthropic-ai/claude-code@${CLAUDE_VERSION}"` (cross-provider).
- Entrypoint preserved at line 676: `ENTRYPOINT ["codex"]`.

**AC2 — `DefaultClaudeDockerfile()` mirrors symmetrically + `CODEX_HOME` env added.**
- `internal/services/images/service.go:720` — `mkdir -p /home/valv/.claude /home/valv/.codex /workspace` (both home dirs).
- `internal/services/images/service.go:723-730` — `ENV` block includes `CLAUDE_CONFIG_DIR=/home/valv/.claude` (existing, line 729) AND new `CODEX_HOME=/home/valv/.codex` (line 730).
- `internal/services/images/service.go:732-733` — `ARG CLAUDE_VERSION` + `RUN npm install --global "@anthropic-ai/claude-code@${CLAUDE_VERSION}"` (primary).
- `internal/services/images/service.go:735-736` — `ARG CODEX_VERSION` + `RUN npm install --global "@openai/codex@${CODEX_VERSION}"` (cross-provider).
- Entrypoint preserved at line 740: `ENTRYPOINT ["claude"]`.

**AC3 — `Build()` emits both `CODEX_VERSION` and `CLAUDE_VERSION` regardless of provider.**
- `internal/services/images/service.go:318-321` — empty `request.CrossProviderVersion` defaults to `"latest"`.
- `internal/services/images/service.go:327-332` — `BuildArgs` map ALWAYS populated with FOUR keys:
  - `s.providerVersionBuildArg()` — primary (CODEX_VERSION when provider=codex, CLAUDE_VERSION when provider=claude).
  - `s.crossProviderVersionBuildArg()` — cross-provider (returns the OTHER arg name; line 573-578).
  - `VALV_GID` / `VALV_UID`.
- No `if req.Provider == ...` branching around arg emission — both args always emitted; only the value-vs-key mapping differs by provider.
- `internal/adapters/docker/ops.go:78-87` — `BuildImageArgs` sorts the BuildArgs map keys via `sort.Strings(keys)` (line 83), so the emitted `--build-arg` order is deterministically alphabetic: `CLAUDE_VERSION` < `CODEX_VERSION` < `VALV_GID` < `VALV_UID`.

**AC4 — `BuildRequest.CrossProviderVersion` field; defaults to `"latest"` when empty.**
- `internal/services/images/service.go:90-100` — `BuildRequest` struct has `CrossProviderVersion string` (line 96) with a 4-line doc comment explaining the default-to-`"latest"` rule.
- `internal/services/images/service.go:318-321` — defaulting logic in `Build()`:
  ```
  crossVersion := strings.TrimSpace(request.CrossProviderVersion)
  if crossVersion == "" {
      crossVersion = "latest"
  }
  ```
- Test evidence: `TestServiceBuildAddsVersionAndUsesDefaultImageInfo` (line 99) passes `BuildRequest{Version: "0.117.0"}` (no `CrossProviderVersion`), and asserts the emitted slice contains `--build-arg CLAUDE_VERSION=latest` (line 116). Default applied; behaviour observable.

**AC5 — Three updated arg-comparison tests use alphabetic ordering.**
- `internal/services/images/service_test.go:112-119` — `TestServiceBuildAddsVersionAndUsesDefaultImageInfo` `want` slice: `CLAUDE_VERSION=latest` → `CODEX_VERSION=0.117.0` → `VALV_GID=<gid>` → `VALV_UID=<uid>`. Comment on lines 110-111 explicitly documents the alphabetic order.
- `internal/services/images/service_test.go:437-445` — `TestBuildIncludesExtraTags` `wantArgs` slice: same alphabetic order, with `--build-arg CLAUDE_VERSION=latest` preceding `--build-arg CODEX_VERSION=0.117.0`. Inline comment on line 436 documents the order.
- `internal/services/images/service_test.go:464-477` — `TestServiceBuildFallsBackToLegacyBuildWhenBuildxUnavailable` `firstCallParts` slice: same alphabetic order. Also `internal/services/images/service_test.go:506-519` — `wantFallback` slice for the legacy-mode second call mirrors the order. Inline comment on lines 462-463 documents the order.

**WriteDefault context tests (auxiliary evidence).**
- `internal/services/images/service_test.go:387-404` — `TestWriteDefaultCodexContextWritesDockerfile` asserts presence of `@openai/codex@${CODEX_VERSION}`, `@anthropic-ai/claude-code@${CLAUDE_VERSION}`, `/home/valv/.codex`, and `/home/valv/.claude` — proves AC1 at the file-write boundary.
- `internal/services/images/service_test.go:563-584` — `TestWriteDefaultClaudeContextWritesDockerfile` asserts presence of `@anthropic-ai/claude-code@${CLAUDE_VERSION}`, `@openai/codex@${CODEX_VERSION}`, `CLAUDE_CONFIG_DIR=/home/valv/.claude`, `CODEX_HOME=/home/valv/.codex`, `/home/valv/.claude`, `/home/valv/.codex` — proves AC2 (mirror + new `CODEX_HOME`) at the file-write boundary.

### Mage gate re-runs

- `mage testPkg github.com/evanmschultz/valv/internal/services/images`:
  ```
  [PKG PASS] github.com/evanmschultz/valv/internal/services/images (1.28s)
  tests: 29 / passed: 29 / failed: 0
  cover: 79.7% (floor 60.0%) — threshold met
  ```
  Matches the worklog claim exactly (29/29 @ 79.7%).
- `mage build`:
  ```
  [INFO] Building valv (./cmd/valv)
  [SUCCESS] Built valv (./valv)
  ```
  Binary produced; no compile breaks introduced by the new field or arg-emission shape.

### Certificate

- **Premises**
  - Both Dockerfiles install both CLIs and create both home dirs.
  - `Build()` always emits both version build-args regardless of provider.
  - `BuildRequest.CrossProviderVersion` field exists and defaults to `"latest"`.
  - The three arg-comparison tests reflect the sorted slice shape.
  - `mage testPkg` + `mage build` are green.
- **Evidence** — `internal/services/images/service.go:90-100,318-332,562-578,645-742`; `internal/services/images/service_test.go:82-130,377-413,415-456,458-524,553-593`; `internal/adapters/docker/ops.go:78-87`; mage outputs above.
- **Trace or cases** — Every AC mapped to file:line + behaviour assertion; both provider branches of `providerVersionBuildArg()` and `crossProviderVersionBuildArg()` covered by the existing recipe-hash test matrix (codex + claude rows in `TestServiceBuildRecipeHashMatchesProviderDockerfile`, line 599+).
- **Conclusion** — PASS. All five ACs supported by citation-grade evidence; both mage gates re-run green.
- **Unknowns** — None.
