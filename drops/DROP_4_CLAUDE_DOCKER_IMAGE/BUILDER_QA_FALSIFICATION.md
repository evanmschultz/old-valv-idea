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

## Unit 4.2 — Round 1

- **Reviewer:** go-qa-falsification-agent
- **Commit:** e3869f6
- **Verdict:** pass

### Attack probes

1. **Store leak on error paths — REFUTED.** Traced every return in `openImagesService` at `internal/cli/operator_helpers.go:68-112`. Six paths: (a) `openStore` fail at line 71 — no store to close; (b) Codex `WriteDefaultCodexContext` fail at line 87 — `_ = store.Close()` before return; (c) Claude `WriteDefaultClaudeContext` fail at line 96 — `_ = store.Close()`; (d) unsupported provider `default` at line 104 — `_ = store.Close()`; (e) `imagesservice.New` fail at line 109 — `_ = store.Close()`; (f) success at line 112 — caller owns closer. Every error path closes the store. No leak.

2. **Codex regression — REFUTED.** `Grep openImagesService(` returned three non-test call sites: `operator_helpers.go:68` (definition), `codex.go:243` (`ensureCodexImageCurrent`), `manage.go:1090` (`runManageUpdate`) — both callers pass `domain.ProviderCodex` per the commit diff. `extended_test.go:448` already references `filepath.Join(paths.BuildCacheDir, string(domain.ProviderCodex), "Dockerfile")` pre-refactor, so it resolves to the same codex-subdir path under the new layout. Fresh `mage testPkg ./internal/cli` run: 112 tests pass, 72.6% coverage, gofumpt clean. No Codex regression.

3. **Build-cache subdir collision — REFUTED.** `contextDir := filepath.Join(paths.BuildCacheDir, string(provider))` at `operator_helpers.go:73` uses the provider enum as subdir key. `TestOpenImagesServiceCodexContextWritesToCodexSubdir` at `claude_image_test.go:97-117` asserts the Codex call creates `.../codex/Dockerfile` AND no `.../claude/Dockerfile` exists afterward — explicit anti-collision guard. Test passes in the fresh mage run.

4. **Unused `claudeImageRepository` / `claudeImageTag` helpers — REFUTED.** Both called at `operator_helpers.go:99-100` inside the `domain.ProviderClaude` switch case. `TestOpenImagesServiceClaudeContextWritesDockerfile` at `claude_image_test.go:75-95` exercises the Claude branch and passes. The gopls "unused" flag referenced in the worklog is a false positive from switch-branch static analysis; runtime coverage confirms reachability.

5. **`VALV_CLAUDE_IMAGE` env parsing — REFUTED.** `claudeImageRef()` at `claude_image.go:13-25` mirrors `codexImageRef()` at `codex.go:273-285` byte-for-byte (modulo default repo). Eight dedicated tests in `claude_image_test.go:15-73` cover: empty env → `valv-claude:dev`; `repo:tag` override → parsed; `repo` without tag → empty tag parsed; `claudeImageRepository()` default + override; `claudeImageTag()` default + override + fallback when override lacks tag. All pass.

6. **Resolver-nil Claude branch — REFUTED.** `imagesservice.New` at `internal/services/images/service.go:210-213` auto-assigns a resolver only when `resolver == nil && provider == domain.ProviderCodex`. For `ProviderClaude` with `Resolver: nil`, the resolver stays nil — a valid pinned-version configuration. `TestOpenImagesServiceClaudeContextWritesDockerfile` constructs the Claude service and passes; no `imagesservice.New` error.

7. **Closure double-close — REFUTED (non-regression).** The returned closure `func() { _ = store.Close() }` inherits the same double-close semantics as the pre-change `openImagesService` and the sibling `openManageService` / `openGlobalSwitchService`. `modernc.org/sqlite`'s `Close` tolerance is unchanged by Unit 4.2; no new double-close risk introduced.

8. **Unsupported-provider error message — REFUTED.** `fmt.Errorf("initialize image service: unsupported provider %q", provider)` at `operator_helpers.go:104`. `TestOpenImagesServiceUnsupportedProvider` at `claude_image_test.go:119-135` asserts via `strings.Contains(err.Error(), "unsupported provider")` — substring match, future-safe. Test passes.

9. **gofumpt / race / coverage — REFUTED.** Fresh `mage testPkg ./internal/cli` output: `[PKG PASS] github.com/evanmschultz/valv/internal/cli (90.39s)`, 112 tests passed, 72.6% coverage, coverage threshold met, gofumpt clean, `-race` unconditional. No Codex test deleted or silently weakened.

10. **`manage.go` caller update correctness — REFUTED.** `runManageUpdate` at `manage.go:1090` now calls `openImagesService(cmd, paths, domain.ProviderCodex)`. Unit 4.3 will generalize further; Unit 4.2 preserves Codex-only behavior. Mage test confirms all existing `runManageUpdate` coverage stays green.

### Summary

All ten attack surfaces REFUTED. No counterexample constructed. No BLOCKER. Unit 4.2 at `e3869f6` passes falsification review.

### Unknowns routed

None.

## Unit 4.3 — Round 1

- **Reviewer:** go-qa-falsification-agent
- **Commit:** `413db81 feat(cli): add claude branch to valv manage update`
- **Working dir:** `/Users/evanschultz/Documents/Code/hylla/valv/main`
- **Verdict:** PASS (no unmitigated counterexample)

### Attack surfaces probed

1. **Codex regression (REFUTED).** Pre-existing Codex manage-update tests live in `internal/cli/extended_test.go` (`TestManageUpdateUsesFakeDockerAndWritesBuildContext` at line 423, `TestManageUpdateUsesOverrideImageRepository` at line 466, `TestManageUpdateSecondRunReportsUpToDate` at line 492). `git diff HEAD~1 HEAD -- internal/cli/extended_test.go` is empty — these tests were not touched. Fresh `mage testPkg ./internal/cli` reports 117 pass / 0 fail. Additionally, the new `TestRunManageUpdateCodexRegression` (manage_test.go:314) drives both `update` and `update codex` argv paths through `newManageCommand` and asserts the same shape (`"Provider image updated"`, `provider=codex`, spinner strings, Dockerfile path, build args). Codex behavior is preserved.
2. **Spinner wrapper symmetry (REFUTED).** `runManageUpdateClaude` (manage.go:1130-1156) wraps `service.Build(...)` inside `runWithCLIQuietSpinner("Building provider image", "Provider image built", "Provider image build failed", func() error { ... })` at lines 1137-1147. Symmetric to the Codex branch's spinner wrap of `EnsureLatest`. Stderr assertions in `TestRunManageUpdateClaudeBuildsImage` (manage_test.go:295-299) confirm the spinner strings are actually emitted.
3. **Output-field correctness (REFUTED).** Claude `output.WriteRecord` at manage.go:1155 emits exactly 5 fields: `provider`, `image`, `tags`, `version`, `context` — `checked at` is absent. Rationale is sound: `imagesservice.BuildResult` has no `LatestCheckedAt` field (that lives on `EnsureResult`, resolver-populated), so there is no timestamp to render for the pinned-version Build path.
4. **`forvar` diagnostic at `manage_test.go:316` (REFUTED).** Context reads: `for _, args := range [][]string{{"update"}, {"update", "codex"}} { args := args; t.Run(...) }`. Under Go 1.22+ per-iteration scoping the `args := args` copy is redundant but not wrong — pure cosmetic. No correctness impact. Does not block PASS.
5. **Build-arg key correctness for Claude (REFUTED; closes Unit 4.1 gap).** `TestRunManageUpdateClaudeBuildsImage` (manage_test.go:305-311) reads the fake-docker log and asserts `fmt.Sprintf("--build-arg CLAUDE_VERSION=%s", imagesservice.DefaultClaudeCLIVersion)` is present, alongside `buildx build`, `-t valv-claude:dev`, `--label io.valv.provider=claude`. This is the first test that asserts the Claude build-arg KEY is `CLAUDE_VERSION` (not `CODEX_VERSION`), closing the gap Unit 4.1 falsification flagged.
6. **Cobra `Example` string (REFUTED).** `newManageUpdateCommand.Example` at manage.go:1066-1070 reads `valv manage update` / `valv manage update codex` / `valv manage update claude`. The third line was added by this commit. Renders correctly as a trimmed multi-line example.
7. **Unsupported-provider path (REFUTED).** `TestRunManageUpdateUnsupportedProvider` (manage_test.go:360-379) calls `runManageUpdate(cmd, paths, &rootOptions{}, domain.Provider("foo"))` **directly**, bypassing `parseOptionalProvider` entirely. It does not go through cobra. The `default:` branch at manage.go:1094 IS exercised; assertions verify both the error substring `"not supported yet"` and that the offending provider name `"foo"` appears in the message.
8. **Coverage (REFUTED).** `mage testPkg ./internal/cli` reports 72.9% — above both the 60% mage floor and the 70% AGENTS.md § 11 per-package coverage requirement. Coverage went from 72.6% (post-Unit-4.2) to 72.9% — slight uptick, no regression.
9. **Fresh `mage testPkg ./internal/cli` (REFUTED).** Ran from `main/`: 117 tests / 117 pass / 0 fail / 0 skip; gofumpt clean; coverage 72.9% ≥ 60.0% floor. No raw `go` bypass; command executed via mage.
10. **Scope discipline (REFUTED).** `git diff HEAD~1 HEAD --name-only` → `drops/DROP_4_CLAUDE_DOCKER_IMAGE/BUILDER_WORKLOG.md`, `drops/DROP_4_CLAUDE_DOCKER_IMAGE/PLAN.md`, `internal/cli/manage.go`, `internal/cli/manage_test.go`. PLAN.md declared `Paths: internal/cli/manage.go, internal/cli/manage_test.go`. Scope honored exactly; drop md edits are permitted orchestrator housekeeping (PLAN.md state flip, worklog append).

### Certificate

- **Premises.** Unit 4.3 must: switch-dispatch `runManageUpdate` on provider; preserve Codex behavior identically; add Claude branch calling `service.Build` with `DefaultClaudeCLIVersion` inside a spinner wrapper; emit 5-field Claude output (no `checked at`); cover all three branches with tests; pass `mage testPkg ./internal/cli` with ≥70% coverage and gofumpt clean.
- **Evidence.** `git diff HEAD~1 HEAD` on `413db81`; `Read` of `manage.go:1062-1156` and `manage_test.go:266-379`; `Grep` across `internal/cli/` for `TestManageUpdate` / `TestRunManageUpdate`; fresh `mage testPkg ./internal/cli` (117 pass, 72.9%, gofumpt clean).
- **Trace or cases.** Ten attack surfaces, each REFUTED with file-and-line evidence.
- **Conclusion.** PASS. No counterexample constructed. Unit 4.3 implementation matches the PLAN.md spec and the acceptance criteria.
- **Unknowns.** None — all surfaces probed deterministically.

### Hylla Feedback

None — Hylla was not queried for this falsification pass. The scope was tightly confined to two files that PLAN.md named explicitly; `Read` / `Grep` / `git diff` + a fresh `mage testPkg` run answered every probe. No Hylla miss to record.
