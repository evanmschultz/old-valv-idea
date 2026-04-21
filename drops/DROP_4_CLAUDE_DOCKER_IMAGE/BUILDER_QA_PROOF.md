# DROP_4 — Builder QA Proof

Proof-oriented QA appends per `## Unit N.M — Round K` section. See `main/drops/WORKFLOW.md` § "Phase 5 — Build-QA (per unit)".

## Unit 4.1 — Round 1

**Verdict:** pass

**Evidence:**
- AC1 `DefaultClaudeDockerfile` godoc: `go doc github.com/evanmschultz/valv/internal/services/images DefaultClaudeDockerfile` returns non-empty multi-line godoc mirroring Codex (`service.go:595-600`).
- AC2 `WriteDefaultClaudeContext` godoc: `go doc ... WriteDefaultClaudeContext` returns non-empty godoc (`service.go:577-581`).
- AC3 `DefaultClaudeCLIVersion` godoc + value: `go doc ... DefaultClaudeCLIVersion` reports `const DefaultClaudeCLIVersion = "2.1.89"` with godoc body present (`service.go:32-36`). Matches PLAN's proposed pin and builder's Context7 re-verification (BUILDER_WORKLOG.md L26).
- AC4 `mage testPkg ./internal/services/images` green: fresh run reports `16 tests passed across 1 package`, coverage 74.8% (above 70% spec target and the 60% mage floor), gofumpt clean. `-race -cover -count=1` enforced.
- AC5 provider-keyed `recipeHash()`: verified in `TestServiceBuildRecipeHashMatchesProviderDockerfile` (`service_test.go:519-589`). Table has `ProviderCodex → sha256(DefaultCodexDockerfile())` and `ProviderClaude → sha256(DefaultClaudeDockerfile())`; test extracts the `io.valv.recipe_hash=<hex>` label from the recorded `buildx` args and asserts equality. Both subcases pass. Underlying implementation at `service.go:447-457` uses `s.providerDockerfileContent()` for the default-Dockerfile branch.
- AC6 `providerDockerfileContent()` helper: defined at `service.go:462-467` on value receiver `Service`, unexported. Called from `recipeHash()` at `service.go:448`. Diff confirms the unconditional `content := DefaultCodexDockerfile()` seed was replaced by `content := s.providerDockerfileContent()` (git diff HEAD~1 HEAD, service.go line 447-448). No remaining unconditional Codex seed in `recipeHash()`.
- AC7 Dockerfile markers: `DefaultClaudeDockerfile()` at `service.go:601-631` contains `CLAUDE_CONFIG_DIR=/home/valv/.claude` (L623), `ARG CLAUDE_VERSION` + `RUN npm install --global "@anthropic-ai/claude-code@${CLAUDE_VERSION}"` (L625-626), `ENTRYPOINT ["claude"]` (L630). Asserted by `TestWriteDefaultClaudeContextWritesDockerfile` at `service_test.go:478-507` (all three markers plus the shared Codex-parity lines for base image, apt packages, useradd/chown, NPM_CONFIG env).
- AC8 no Codex regression: `git diff HEAD~1 HEAD --stat` shows 4 deletions total in `service.go` — those 4 lines are the old 3-line BuildArgs map literal + the single `content := DefaultCodexDockerfile()` seed, all replaced in-place. `DefaultCodexDockerfile`, `WriteDefaultCodexContext`, and the pre-existing Codex unit tests are byte-identical pre/post. `mage testPkg` runs all 16 tests including the prior Codex assertions (notably `service_test.go:110` which expects sorted build-args `CODEX_VERSION`, `VALV_GID`, `VALV_UID`) — all pass.
- AC9 integration gating: `service_integration_test.go:1` carries `//go:build integration`. Fresh `mage testPkg ./internal/services/images` completed without Docker; integration tests are excluded by the non-tagged invocation.

**Scope-attention finding (non-blocking):** builder added an unenumerated helper `providerVersionBuildArg()` on `Service` (`service.go:472-477`) to generalize `Build()`'s build-arg key from `CODEX_VERSION` to provider-keyed (`service.go:254`).
- Scope: within Unit 4.1 `Paths` (`internal/services/images/service.go`). PASS.
- Necessity: without it, `Build()` would emit `--build-arg CODEX_VERSION=<ver>` on a Claude service, leaving the Claude Dockerfile's `ARG CLAUDE_VERSION` unset. PASS.
- Codex regression risk: `internal/adapters/docker/ops.go:80-85` sorts build-arg keys alphabetically. Codex order is `CODEX_VERSION` < `VALV_GID` < `VALV_UID`; Claude order is `CLAUDE_VERSION` < `VALV_GID` < `VALV_UID` — both providers keep the version key at sorted-position 0. The positional `reflect.DeepEqual` assertion at `service_test.go:110-113` (Codex) remains correct; `mage testPkg` green confirms this. PASS.
- Test coverage of `providerVersionBuildArg` specifically: implicit only. `TestServiceBuildRecipeHashMatchesProviderDockerfile` Claude case exercises the helper end-to-end (calls `svc.Build` on `ProviderClaude`) and passes, but asserts on the recipe-hash label rather than the build-arg key string. No direct assertion that Claude emits `--build-arg CLAUDE_VERSION=<ver>` (that is explicitly scoped to Unit 4.3 per BUILDER_WORKLOG.md L30). The helper is a simple 4-line switch — correctness is obvious on inspection — but a targeted unit assertion here would strengthen. Not a blocker for Unit 4.1; flag for the orchestrator in case Unit 4.3 is expected to land the assertion. ACCEPTED.

**Findings:**
- None blocking. One non-blocking observation surfaced under "Scope-attention finding" above: `providerVersionBuildArg` is correct, necessary, in-scope, and does not regress Codex, but lacks a direct unit assertion on the Claude build-arg key string. Unit 4.3 is the natural home for that assertion per BUILDER_WORKLOG.md L30 — orchestrator should confirm Unit 4.3's spec includes it.
