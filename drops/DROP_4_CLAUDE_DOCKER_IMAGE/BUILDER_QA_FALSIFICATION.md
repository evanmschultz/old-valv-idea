# DROP_4 — Build QA Falsification

Durable append log of falsification-oriented build-QA findings. One `## Unit N.M — Round K` section per QA attempt. Verdicts: `pass` (no unmitigated counterexample) or `fail` (BLOCKER found).

## Unit 4.1 — Round 1

**Verdict:** pass

**Counterexamples attempted:**

- Custom-Dockerfile fallback under Claude provider (attack 1). Mitigated. `recipeHash()` at `service.go:447-457` seeds `content := s.providerDockerfileContent()` before the fallback. For `Provider=Claude` with a custom dockerfile name and successful disk read, on-disk bytes win. On silent read failure, the seed remains `DefaultClaudeDockerfile()` (not Codex). Strictly better than pre-fix.
- `providerVersionBuildArg()` out-of-spec addition (attack 2). Mitigated; justified scope creep. PLAN.md:93-107 scoped only `recipeHash()`; the builder also edited `Build()` at `service.go:253-256` to route the version-arg key through `providerVersionBuildArg()`. Without this, a Claude build would pass `CODEX_VERSION=<ver>` to a Dockerfile that declares `ARG CLAUDE_VERSION`, and the npm install would resolve to `@anthropic-ai/claude-code@` (empty version). Documented at BUILDER_WORKLOG.md:30. Deferring to Unit 4.3 would have forced a cross-package edit and violated the atomic-unit invariant.
- `BuildArgs` map alphabetical ordering vs existing positional test (attack 2). Mitigated. `internal/adapters/docker/ops.go:79-85` sorts `BuildArgs` keys alphabetically. Codex order: `CODEX_VERSION, VALV_GID, VALV_UID`. Claude order: `CLAUDE_VERSION, VALV_GID, VALV_UID`. Version arg stays at sorted-position 0 in both cases. `TestServiceBuildAddsVersionAndUsesDefaultImageInfo` at `service_test.go:82-114` uses default (Codex) provider; its positional `reflect.DeepEqual` at line 110 still holds — verified by fresh `mage testPkg`.
- Hypothetical future provider version-arg sorting between `VALV_*` (attack 2). Accepted (YAGNI). A provider naming its version arg e.g. `VAST_VERSION` would sort between `VALV_GID` and `VALV_UID`, breaking positional tests. Not a current failure. Flag for future planners.
- Dockerfile chown of `.claude` (attack 3). Mitigated. `chown -R "${VALV_UID}:${VALV_GID}" /home/valv /workspace` recursively covers `/home/valv/.claude`.
- `CLAUDE_CONFIG_DIR` env (attack 3). Mitigated. ENV block sets `CLAUDE_CONFIG_DIR=/home/valv/.claude`. Cross-referenced against `VALV_CLAUDE_CODE_FOCUS_PLAN.md:102` and `VALV_ACCOUNT_SWITCH_PLAN.md:58`. Context7 did not surface a direct Anthropic doc but internal design evidence is consistent and planner-accepted.
- Entrypoint binary name (attack 3). Mitigated. Context7 `/anthropics/claude-code` README: "navigate to any project directory and run the claude command". After `npm install --global @anthropic-ai/claude-code`, `claude` is on `$PATH` via npm's global `bin/` symlink.
- chown order vs `USER` (attack 3). Mitigated. Order: mkdir, chown, `ARG`+`npm install`, `USER valv`, `WORKDIR`, `ENTRYPOINT`. chown precedes `USER` switch.
- `defaultCodexDockerfile` constant reuse (attack 4). Mitigated. Value `"Dockerfile"` is semantically provider-agnostic. `recipeHash()` default-branch check correctly recognizes Claude's default Dockerfile as "the default". Cosmetic rename candidate (`defaultDockerfileName`) — low priority.
- `providerDockerfileContent()` coverage (attack 5). Mitigated. `TestServiceBuildRecipeHashMatchesProviderDockerfile` at `service_test.go:500-590` exercises both branches (codex + claude) via `Build()` → buildx args → `--label` extraction. 74.8% package coverage confirmed.
- `providerVersionBuildArg()` coverage gap (attack 5). Accepted. New `TestServiceBuildRecipeHashMatchesProviderDockerfile` asserts only the recipe-hash label, not `--build-arg CLAUDE_VERSION=<pin>`. No test in the images package directly asserts that a Claude build emits `--build-arg CLAUDE_VERSION=<…>`. Planner explicitly scoped that assertion to Unit 4.3 (`TestRunManageUpdateClaudeBuildsImage` at PLAN.md:198). Unit 4.1 acceptance criteria do not require the build-arg assertion. Correct end-to-end coverage arrives at Unit 4.3.
- Integration test build-tag coverage (attack 6). Mitigated with an observation. `//go:build integration` keeps the test out of `mage testPkg`. HOWEVER, `mage integration` at `magefile.go:124` runs `-tags=integration` only against `./internal/cli` — NOT `./internal/services/images`. The new Claude integration test (and pre-existing Codex integration tests `TestServiceBuildRealDockerImage`, `TestWriteDefaultCodexContextBuildsWithExistingUIDAndGID`) are never executed by any mage target today. Pre-existing condition inherited from Codex, not introduced by Unit 4.1. Unit 4.1 acceptance criterion #9 ("mage integration exercises the new test when Docker is available") is false as written. Route to orchestrator as low-priority follow-up for DROP_9 cleanup backlog.
- `TestServiceBuildRecipeHashMatchesProviderDockerfile` structure (attack 7). Mitigated. Table-driven, `t.Run` per case. Fresh `t.TempDir` + `runnerRecorder` per case. Reuses existing `*runnerRecorder` at `service_test.go:22-52`. Label-extraction loop scans for `--label` followed by `io.valv.recipe_hash=<…>`. No docker daemon required. No state sharing.
- gofumpt / go vet / race (attack 8). Mitigated. Fresh `mage testPkg ./internal/services/images`: "[SUCCESS] All tests passed. 16 tests passed. 74.8% coverage, above 60.0% floor." `mage testPkg` runs gofumpt + `go test -race -cover -count=1` + coverage gate. All green.
- Codex regression (attack 9). Mitigated. Diffed `service_test.go` — no Codex assertions deleted or modified. `TestServiceBuildAddsVersionAndUsesDefaultImageInfo` still asserts `CODEX_VERSION=0.117.0` positionally. `TestServiceBuildFallsBackToLegacyBuildWhenBuildxUnavailable` at `service_test.go:444` still asserts `CODEX_VERSION=0.117.0`. Pre-existing Codex integration tests in `service_integration_test.go` retained. 13 pre-existing Codex tests + 3 new Claude tests = 16. Zero Codex regression.
- `go doc` verification (acceptance criteria). Mitigated. `go doc` returns non-empty for `DefaultClaudeDockerfile`, `WriteDefaultClaudeContext`, and `DefaultClaudeCLIVersion` (= `"2.1.89"`). All three acceptance criteria at PLAN.md:121-123 satisfied.
- Claude npm install deprecation (Context7 discovery). Accepted. Context7 `/anthropics/claude-code` lists native installer (`curl ... | bash`) as primary and npm as deprecated-but-functional. Planner explicitly accepted the npm path until proven broken (PLAN.md:219). Pin `"2.1.89"` is currently valid.

**BLOCKERs:** None.

**Routed unknowns (for orchestrator):**

- `mage integration` does not run `./internal/services/images`. Pre-existing — inherited from Codex. Acceptance criterion #9 overstates reality. Low-priority follow-up — route to DROP_9 cleanup backlog.
- Future-provider version-build-arg whose name sorts after `VALV_*` would break `TestServiceBuildAddsVersionAndUsesDefaultImageInfo` positional assertion. Latent risk, not current. Flag for DROP_5+ planners.
- Shared constant `defaultCodexDockerfile` (value `"Dockerfile"`) used by both Codex and Claude context writers. Cosmetic rename candidate (`defaultDockerfileName`). Low priority — route to DROP_9.

## Hylla Feedback

None — Hylla was not needed for this review. Commit `ee1fc99` is post-last-ingest and stale in Hylla. Evidence came from `git show`, direct `Read` of `service.go` / `service_test.go` / `service_integration_test.go` / `ops.go` / `magefile.go`, and from a fresh `mage testPkg ./internal/services/images` run. External verification came from Context7 `/anthropics/claude-code`. No Hylla query was issued.
