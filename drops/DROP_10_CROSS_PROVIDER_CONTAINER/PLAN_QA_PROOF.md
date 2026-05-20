# DROP_10 Plan QA Proof — Round 2

**Round:** 2
**Reviewer:** go-qa-proof-agent
**Drop PLAN.md:** main/drops/DROP_10_CROSS_PROVIDER_CONTAINER/PLAN.md
**Scope:** Verify the four dev-accepted R1 findings (F1 build-arg ordering, F2 mount-target literals, F3 codex insertion site, F4 §Notes contradiction) are resolved in R2. Surface any new issues introduced by R2 edits.

## 1. R1 Finding Resolution Table

| Ref | Finding (R1) | Severity | R2 PLAN.md location | Status |
|---|---|---|---|---|
| F1 | Unit 10.1 build-arg ordering claim must cite alphabetic sort from `BuildImageArgs` and the `want` slice must match alphabetic order. | high | Lines 109–115: explicitly cites `docker.BuildImageArgs` `sort.Strings(keys)` (file `internal/adapters/docker/ops.go`) and lists the four keys in alphabetic order: `CLAUDE_VERSION < CODEX_VERSION < VALV_GID < VALV_UID`. Sample `want` slice on line 113 matches that order. | RESOLVED |
| F2 | Unit 10.2 + 10.3 acceptance criteria must state verbatim mount-target strings + env-var values. | low | 10.2 Acceptance (lines 185–193): verbatim `"/home/valv/.codex"` as mount `Target` and `CODEX_HOME` env key. 10.3 Acceptance (lines 235–243): verbatim `"/home/valv/.claude"` as mount `Target` and `CLAUDE_CONFIG_DIR` env key. Sample code blocks (lines 163, 220) also carry the literal strings so the builder cannot drift. | RESOLVED |
| F3 | Unit 10.3 insertion site must sharpen — "after the initial `mounts := ... ` + `env := ...` block, BEFORE `newBridgeManager`". | low | Line 211: verbatim phrasing "immediately after the initial `mounts := []dockeradapter.MountSpec{...}` + `env := map[string]string{...}` block, BEFORE the `newBridgeManager` call, add:". Verified against `internal/adapters/providers/codex/runtime.go` — `mounts :=` block is lines 104–106, `env :=` block is lines 107–113, `newBridgeManager` call is line 115. Insertion site (between line 113 and line 115) exists and is unambiguous. | RESOLVED |
| F4 | §Notes "Cache-busting for image rebuild" must not claim recipe-hash fakes need manual updates. | low | Lines 380–381: "Tests that delegate to `DefaultCodexDockerfile()` / `DefaultClaudeDockerfile()` (including `fakeClaudeRecipeHash` / `fakeCodexRecipeHash` in `internal/cli/extended_test.go`) **auto-update because they call the functions dynamically — no constant churn needed**." Contradiction removed. | RESOLVED |

## 2. Code-Evidence Cross-Check

**F1 — `BuildImageArgs` alphabetic sort exists.** Hylla `hylla_node_full` on `github.com/evanmschultz/valv/internal/adapters/docker/BuildImageArgs` returns content showing the `BuildArgs` loop:

```go
if len(request.BuildArgs) > 0 {
    keys := make([]string, 0, len(request.BuildArgs))
    for key := range request.BuildArgs {
        keys = append(keys, key)
    }
    sort.Strings(keys)
    for _, key := range keys {
        args = append(args, "--build-arg", fmt.Sprintf("%s=%s", key, request.BuildArgs[key]))
    }
}
```

File path per Hylla: `internal/adapters/docker/ops.go`. Plan cites the file correctly (the appendix prompt's "types.go" reference is a prompt-side typo, not a plan defect). `sort.Strings` is stdlib lexicographic byte-order sort; the alphabetic order `CLAUDE_VERSION < CODEX_VERSION < VALV_GID < VALV_UID` is correct under that policy.

**F3 — Insertion site exists.** `Read` of `internal/adapters/providers/codex/runtime.go` confirms:
- Lines 104–106: `mounts := []dockeradapter.MountSpec{ dockeradapter.NewMountSpec(runtimeCodexHome, ContainerCodexDir, false), }`
- Lines 107–113: `env := map[string]string{ ... }`
- Line 114: blank
- Line 115: `bridgeManager, err := newBridgeManager(ctx, request.Logger)`

The insertion point between line 113 and line 115 matches the plan's verbatim "after the initial mounts + env block, BEFORE the `newBridgeManager` call" description.

## 3. New Findings from R2 Edits

None. The R2 edits are scoped to the four findings; no collateral regressions or new contradictions surfaced. The Unit 10.4 / 10.5 service-layer scope is unchanged from R1 (was not in the R1 finding set).

## 4. Minor Observations (Not Findings)

- Plan uses literal `"/home/valv/.codex"` / `"/home/valv/.claude"` in code samples rather than referencing the existing `codex.ContainerCodexDir` constant or a new `claude.ContainerClaudeDir` constant. F2 (low) had explicitly accepted this — the verbatim-literal-in-acceptance-criteria approach is what was requested for QA-greppability. Builder may DRY this up if natural; not required.
- Appendix instruction to "spot-check `internal/adapters/docker/types.go`" is a prompt-side typo (the symbol lives in `ops.go`). The plan itself cites `ops.go` correctly. Flagging for orchestrator awareness only; no PLAN.md fix needed.

## 5. Verdict

**PASS.** All four R1 dev-accepted findings (F1 high, F2 low, F3 low, F4 low) are resolved with verifiable evidence in the R2 PLAN.md, cross-checked against `internal/adapters/docker/ops.go` (via Hylla) and `internal/adapters/providers/codex/runtime.go` (via Read). No new issues introduced by the R2 edits. Plan is ready for the builder cascade.

## Hylla Feedback

None — Hylla answered everything needed (`hylla_search_keyword` located `BuildImageArgs` in `ops.go`; `hylla_node_full` returned full content with `sort.Strings(keys)` proving the alphabetic-sort claim).
