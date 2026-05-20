# DROP_10 — Plan QA Proof — Round 1

Verifier: `go-qa-proof-agent` (proof axis only)
Evidence sources: Hylla committed snapshot `github.com/evanmschultz/valv@main` (snapshot 3, latest commit `56ea569`); direct `Read` of `internal/services/images/service.go`; cross-reference of planner claims against committed code.

## Verdict

**FAIL — 1 high-severity finding** (build-arg ordering claim in Unit 10.1 contradicts the committed implementation of `docker.BuildImageArgs`). Two low-severity / style notes recorded but do not block. Remainder of the plan is well-grounded: every cited symbol, test name, struct shape, and method signature has been verified against the committed tree.

## 1. Findings

- 1.1 [Axis: spec-conformance] [severity: high] Unit 10.1 says "the order is `providerVersionBuildArg()` first, then the cross-provider arg — verify this matches the implementation." `internal/adapters/docker/ops.go:BuildImageArgs` (Hylla `github.com/evanmschultz/valv/internal/adapters/docker/BuildImageArgs`) sorts `BuildArgs` map keys alphabetically via `sort.Strings(keys)` before emitting `--build-arg` flags. After the change, the expected slice ordering for BOTH providers is `CLAUDE_VERSION` → `CODEX_VERSION` → `VALV_GID` → `VALV_UID` (pure alphabetic). The planner's "primary-first then cross" ordering will fail `reflect.DeepEqual` in `TestServiceBuildAddsVersionAndUsesDefaultImageInfo` for both codex (`want` slice has `CODEX_VERSION` before `CLAUDE_VERSION` if anyone follows the planner's hint literally) and claude (mirror test). → Fix hint: planner should explicitly state acceptance language as "after the change, the `want` slice must list `--build-arg CLAUDE_VERSION=<v>` before `--build-arg CODEX_VERSION=<v>` regardless of provider, because `docker.BuildImageArgs` sorts keys alphabetically" — and the three cited tests (`TestServiceBuildAddsVersionAndUsesDefaultImageInfo`, `TestBuildIncludesExtraTags`, `TestServiceBuildFallsBackToLegacyBuildWhenBuildxUnavailable`) must be updated with this alphabetic order. Without that clarification a builder might insert one slot in the wrong sequence and waste a build-QA round.

## 2. Missing Evidence

- 2.1 [Axis: spec-conformance] [severity: low] Unit 10.2 + 10.3 hard-code container-path string literals (`"/home/valv/.codex"`, `"/home/valv/.claude"`) in the cross-mount snippet, while the existing code uses the exposed constants `ContainerCodexDir` (codex pkg) and `ContainerClaudeDir` (claude pkg). Cross-package references would introduce circular-dep risk, so the string-literal approach is defensible. Acceptance criteria should pin the expected literal — recommend "mount with target equal to `/home/valv/.codex` (i.e., codexruntime.ContainerCodexDir)" so the symbol intent is preserved even though the test asserts the raw string.

- 2.2 [Axis: spec-conformance] [severity: low] Unit 10.3 places the cross-mount snippet "after the primary codex mounts/env." `codex.PrepareRuntime` is much more complex than claude's — it builds initial `mounts` + `env`, then runs `newBridgeManager`, then `translateConfigFile` (which `append`s to `mounts` via project-overlay), then `os.Stat(auth.json)` / `copyFile`, then a second `translateConfigFile` for project config. The safest insertion site is immediately after the initial `mounts := []dockeradapter.MountSpec{...}` + `env := map[string]string{...}` block, before `newBridgeManager`. Recommend pinning that location in the plan — otherwise a builder could insert near the end and break the implicit ordering invariant `runtime_test.go:TestPrepareRuntimeUsesSharedHomeAndOverlaysAccountAuth` relies on.

## 3. Verification matrix

| Item | Planner claim | Evidence | Status |
|---|---|---|---|
| Unit 10.1 — `DefaultCodexDockerfile` mkdir + npm install layer | Recipe at `internal/services/images/service.go:624-654`, current install layer is `RUN npm install --global "@openai/codex@${CODEX_VERSION}"`, mkdir is `/home/valv/.codex /workspace`. Planner adds `/home/valv/.claude` + claude install. | `Read` `internal/services/images/service.go:624-654` matches verbatim. | PASS |
| Unit 10.1 — `DefaultClaudeDockerfile` mirror | Recipe at `service.go:681-712`, current install is claude only; ENV already has `CLAUDE_CONFIG_DIR`. Planner adds `CODEX_HOME` env, `/home/valv/.codex` mkdir, codex install. | `Read` lines 681-712 confirms. | PASS |
| Unit 10.1 — `BuildRequest.CrossProviderVersion` field is new | New, not yet in tree. | Hylla `github.com/evanmschultz/valv/internal/services/images/BuildRequest` content shows only `Version Pull NoCache ExtraTags` fields today. | PASS (correctly tagged "new") |
| Unit 10.1 — `Build()` defaults `CrossProviderVersion` to `"latest"` when empty | npm registry resolves `@pkg@latest` correctly; deferred reproducibility OK at dogfood stage. | npm semantics standard. | PASS |
| Unit 10.1 — Three arg-comparison tests use `reflect.DeepEqual` | `TestServiceBuildAddsVersionAndUsesDefaultImageInfo` confirmed via Hylla content — uses `reflect.DeepEqual(runner.calls[0], want)` and `want` slice includes `--build-arg CODEX_VERSION=0.117.0`. | Hylla content. | PASS |
| Unit 10.1 — Build-arg slice ordering | "providerVersionBuildArg() first, then cross-provider arg" | `docker.BuildImageArgs` sorts keys alphabetically via `sort.Strings(keys)`. | **FAIL — Finding 1.1** |
| Unit 10.1 — `WriteDefaultCodexContext` / `WriteDefaultClaudeContext` exist | At `service.go:609-622` and `660-673`. | `Read`. | PASS |
| Unit 10.1 — `fakeCodexRecipeHash` / `fakeClaudeRecipeHash` auto-update via dynamic Dockerfile call | Hylla `github.com/evanmschultz/valv/internal/cli/fakeCodexRecipeHash` content = `func fakeCodexRecipeHash() string { sum := sha256.Sum256([]byte(imagesservice.DefaultCodexDockerfile())); return hex.EncodeToString(sum[:]) }`. | Hylla content. | PASS (dynamic — no constant to update) |
| Unit 10.2 — claude `PrepareRequest` shape | Today: `ProfileHome, SharedHome, ProjectRoot, TempRoot, Logger`. Planner adds `OtherProviderProfileHome string`. | Hylla node `github.com/evanmschultz/valv/internal/adapters/providers/claude/PrepareRequest` content matches. | PASS |
| Unit 10.2 — `TestPrepareRuntimeHasNoCodexEnv` exists and asserts no codex env | Hylla content shows test body checks `prepared.Env["CODEX_HOME"]` is absent only — no mount-target check today. | Hylla content. | PASS |
| Unit 10.2 — `pathutil.Normalize` available + `dockeradapter.NewMountSpec` available | `PrepareRuntime` already calls both. | Hylla `code.depends_on`. | PASS |
| Unit 10.2 — cleanup is NOT needed for cross-mount | Cross-mount target is raw profile home (RW, no staging). | Logically consistent with the existing cleanup that only handles `runtimeClaudeHome != sharedHome`. | PASS |
| Unit 10.3 — codex `PrepareRequest` shape | Same five fields as claude. Planner adds `OtherProviderProfileHome string`. | Hylla content. | PASS |
| Unit 10.3 — no `TestPrepareRuntimeHasNoClaude*` exists in codex pkg | Confirmed via Hylla keyword search (zero hits). | Hylla. | PASS |
| Unit 10.3 — insertion point in codex `PrepareRuntime` | "after primary codex mounts/env" — but codex runtime has bridge manager + 2 translateConfigFile calls afterward. | Hylla content shows complex flow. | Low-severity Finding 2.2 (clarify insertion site) |
| Unit 10.4 — `claude.Service.Run` shape | `resolved` populated at end of both branches before `clauderuntime.PrepareRuntime` call. | Hylla `Service.Run` content. | PASS |
| Unit 10.4 — `s.store.BindingByProjectID(ctx, projectID, provider)` signature | `BindingRepository.BindingByProjectID(context.Context, string, Provider) (ProjectBinding, error)`. Planner's call shape matches exactly. | Hylla `BindingRepository` content. | PASS |
| Unit 10.4 — `s.store.ProfileByID(ctx, profileID)` signature | `ProfileRepository.ProfileByID(context.Context, string) (Profile, error)`. Matches planner. | Hylla `ProfileRepository` content. | PASS |
| Unit 10.4 — `claude.Store` embeds all three repositories | `type Store interface { domain.ProjectRepository; domain.BindingRepository; domain.ProfileRepository }`. | Hylla. | PASS |
| Unit 10.4 — `errors.Is(err, domain.ErrNotFound)` is the right pattern | Already used in `Service.Run`'s project lookup branch. | Hylla content shows `errors.Is(err, domain.ErrNotFound)`. | PASS |
| Unit 10.4 — `domain.ProviderCodex` constant exists | `internal/domain/types.go:ProviderCodex`. | Hylla. | PASS |
| Unit 10.4 — fakeStore extension needed for provider-keyed dispatch | Test extension is mechanical and clearly described. | Plan-internal mechanic — verifiable post-build. | PASS |
| Unit 10.5 — codex.Service.Run mirror | `codex.Service.Run` has identical structural shape; `domain.ProviderClaude` exists. | Hylla. | PASS |
| Unit 10.5 — `CLAUDE_CONFIG_DIR` env var is the right Claude env | Claude Dockerfile sets `CLAUDE_CONFIG_DIR=/home/valv/.claude` (line 703 of service.go). Cross-mount setting `CLAUDE_CONFIG_DIR=/home/valv/.claude` in the codex container env is consistent. | `Read` confirms. | PASS |
| Open question — version-resolver shape | Add `CrossProviderVersion string` to `BuildRequest`; default `"latest"`. Avoids touching `Options`, `Service.EnsureLatest`, `VersionResolver`. | Internal to plan; coherent. | PASS |
| Open question — integration test deferred to DROP_11 | Real in-container cross-call requires both CLIs in a live container; matches AGENTS.md § 11 + memory `feedback_drop_ceremony_trim`. | Coherent. | PASS |
| Open question — README deferred to DROP_11 | DROP_11 owns README rewrite. | Coherent. | PASS |
| Drop-end verification — `mage test` only, no `mage integration` | No symbol deletions (additive only); no CLI argv changes; matches memory `feedback_mage_integration_when_deleting_symbols` (the rule fires on deletions). | All planned changes are additive (new struct fields, new test, new code paths). | PASS |
| `blocked_by` graph | 10.1 / 10.2 / 10.3 unblocked. 10.4 blocked by 10.2 (needs `OtherProviderProfileHome` field). 10.5 blocked by 10.3. | Correct — planner's edges accurately reflect data-flow dependencies. | PASS |
| Atomicity — each unit fits one builder spawn | Unit 10.1 is the heaviest (Dockerfile + Build() + three test fixups); still single-file pair, mechanical, ~120 LOC including tests. Units 10.2 / 10.3 ~80 LOC each. Units 10.4 / 10.5 ~100 LOC each including fakeStore extension. All within builder budget. | Manual estimate from change descriptions. | PASS |

## 4. Hylla Feedback

- **Miss**: initial keyword search for `TestServiceBuildAddsVersionAndUsesDefaultImageInfo` and `fakeCodexRecipeHash` returned empty results with default `test_mode=hide_tests`. Had to re-issue with `test_mode=include_tests`.
- **Worked via**: same `hylla_search_keyword` with `test_mode: "include_tests"`.
- **Suggestion**: when the query string is an explicit `Test*` identifier (matches the canonical Go `Test` prefix), Hylla could auto-relax `test_mode` to `include_tests` for that single query, or surface a hint in the empty-result response shape saying "0 hits with `hide_tests` — retry with `include_tests`?" The current silent-empty result wastes a turn.

Otherwise Hylla covered every Go-code claim in this plan — `BuildImageArgs` sort behavior, `BindingByProjectID` signature, `ProfileRepository.ProfileByID` signature, `PrepareRequest` field list, `Service.Run` body, `PrepareRuntime` insertion-site analysis — all answered from one query each.
