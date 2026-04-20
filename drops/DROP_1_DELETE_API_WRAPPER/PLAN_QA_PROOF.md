# DROP_1 — Plan QA Proof, Round 1

- Verdict: pass
- Reviewed: main/drops/DROP_1_DELETE_API_WRAPPER/PLAN.md @ bfebb85

## Findings

1. **Loose prose in the Scope line describing `root.go` edits.** PLAN.md line 19 says `drop `apiCmd := newAPICommand(...)` + `.GroupID` + `AddCommand` entries (lines 126-135 current numbering)`. Verified against `internal/cli/root.go`: `apiCmd := newAPICommand(paths, opts)` sits at line 126, `apiCmd.GroupID = "runtime"` at line 127, and the final `cmd.AddCommand(pathsCmd, versionCmd, codexCmd, apiCmd, accountCmd, manageCmd, globalCmd)` at line 135 (lines 128-134 are wiring for UNRELATED commands — `accountCmd`, `manageCmd`, `globalCmd`, and so on). A builder reading `lines 126-135` literally could delete unrelated command wiring. Mitigation: Unit 1.1 acceptance 3 (`grep -n "newAPICommand\|apiCmd" main/internal/cli/root.go` returns zero matches) catches any over-deletion mechanically. Not a blocker, but tightening the prose to `delete lines 126 and 127 and remove only the \`apiCmd\` token from the \`cmd.AddCommand(...)\` call on line 135` would remove ambiguity.

2. **Unit 1.1 acceptance 9 depends on cobra help indentation remaining exactly two leading spaces.** Criterion: `./valv --help 2>&1 | grep -c "^  api\b" | grep -qx 0`. If `fang/v2` or cobra bumps the help formatter to a different indent (tab, four spaces, command tree reformat), the grep returns `0` trivially — passing the check without any semantic validation. Low risk because no such formatter change is in flight, but the criterion is fragile. Recommended loosener: `! ./valv --help 2>&1 | grep -qE "^[[:space:]]+api[[:space:]]"` or even just `./valv --help 2>&1 | grep -qv "\bapi\b  *Run the Valv API surface"` (anchored on the `Short` string). Not a blocker — mitigated by Unit 1.5's equivalent `--help` check which tests a different assertion shape.

3. **Unit 1.1 acceptance 7 coverage floor is very loose.** Criterion: `grep -cE "^func Test" main/internal/cli/extended_test.go` returns at least 15. Verified current count is 38; Unit 1.1 deletes 5, leaving 33. A floor of `15` is too generous — it would still pass if the builder accidentally deleted 23 tests. Tightening to at least 30 (or exactly 33 as a spot-check) would catch accidental over-deletion. Planner's acknowledgement in the criterion text (`spot-check lower bound; pre-drop count is higher`) makes this an intentional loosener, so it is acceptable as-is, but the looseness is worth flagging for Round 1 dev review.

4. **`blocked_by: 1.4` on Unit 1.5 is conservative but defensible.** Units 1.3 and 1.4 already cover every Go deletion; Unit 1.5 is pure Markdown scrubbing and could technically run in parallel with either 1.3 or 1.4 (or as early as after 1.1 for most doc scrubs — the `internal/api/` Package Map bullet in `CLAUDE.md` only strictly needs the directory gone, which happens at 1.3). Planner defends the strict linear chain in the Notes section (`one-unit-one-commit per Phase 4`). Not a blocker; called out for transparency.

5. **No orphan inbound references after 1.4.** Cross-checked via `grep -rn "valvcompat\|CodexOpenAICompatibility\|compat \"github.com/evanmschultz/valv\"" main` + `grep -rn "internal/api/openai\|openaihandler\|ChatCompletionsPath" main` + `grep -rn "internal/services/openaiapi" main`. Every Go reference is confined to files either inside the packages being deleted (self-references) or inside files already covered by Unit 1.1 / 1.2 edits. No orphan importer would survive the chain. One advisory: `internal/api/openai/doc.go` line 5 contains the string `codex-openai-compatibility.json` as **comment prose only**, not an import — `doc.go` is deleted by Unit 1.3, so it falls out naturally.

## Evidence Summary

Verified independently (all claims back-referenced to planner assertions; all passed):

- **File counts**:
  - `internal/api/openai/`: 8 files (doc.go, encode.go, errors.go, handler.go, handler_test.go, json.go, types.go, types_test.go). Match.
  - `internal/services/openaiapi/`: 5 files (codex_events.go, codex_models.go, service.go, service_integration_test.go, service_test.go). Match.
  - `service_integration_test.go` carries `//go:build integration` at line 1 — confirms planner's Unit 1.2 ac 4 rationale (deleting it takes the sole `openaiapi` integration path with it).
- **Line counts**: `internal/cli/api.go` = 237 lines (match). `internal/cli/extended_test.go` = 1115 lines (match). `compatibility.go` = 71 lines (match). `compatibility_test.go` = 108 lines (match). `API_COMPAT_EXECUTION_PLAN.md` = 76 lines (match).
- **`internal/cli/root.go` edit lines**:
  - Line 63 `valv api serve --runtime-ttl 2m` in `Example` — confirmed.
  - Line 126 `apiCmd := newAPICommand(paths, opts)` — confirmed.
  - Line 127 `apiCmd.GroupID = "runtime"` — confirmed.
  - Line 135 final `cmd.AddCommand(... apiCmd ...)` — confirmed.
- **`internal/cli/operator_helpers.go` edit lines**:
  - Line 17 `openaihandler "github.com/evanmschultz/valv/internal/api/openai"` — confirmed.
  - Line 25 `openaiapiservice "github.com/evanmschultz/valv/internal/services/openaiapi"` — confirmed.
  - Lines 107-128 `newOpenAIAPIService` function — confirmed (opens line 107, closes line 128).
  - Line 315 `var _ = openaihandler.ChatCompletionsPath` — confirmed.
  - Seven non-API helpers preserved: `openManageService:34`, `openGlobalSwitchService:52`, `openImagesService:71`, `newCleanupService:100`, `runManageHome:130`, `pickProfile:163`, `readAccountIdentity:228` — exactly 7, matches Unit 1.1 ac 6.
- **`internal/cli/extended_test.go` edit lines**:
  - Line 20 `openaiapi "github.com/evanmschultz/valv/internal/api/openai"` — confirmed.
  - Lines 62-89 `apiServeStubService` type + 5 methods — confirmed (opens line 62, final method closes line 89).
  - Lines 91-100 `installStubOpenAIAPIServiceFactory` — confirmed.
  - Lines 884-1059 five wrapper tests — confirmed (TestRunAPIServeStartsAndStopsCleanly line 884; TestNewOpenAIAPIServiceCreatesService ends line 1059).
  - Current `func Test*` count: 38 (`grep -cE "^func Test" main/internal/cli/extended_test.go`). Post-delta count: 33 (five tests removed).
- **LSP `findReferences` on `internal/api/openai/types.go` package declaration** (line 1, char 9): returned exactly 15 references across 15 files. Matches planner's claim verbatim. Files: 8 inside `internal/api/openai/` (self-refs), 3 inside `internal/cli/` (api.go, extended_test.go, operator_helpers.go), 4 inside `internal/services/openaiapi/` (codex_events.go, codex_models.go, service.go, service_test.go). Every reference is owned by a file already in the unit plan (self-ref gets deleted with its parent package; cross-refs get pruned by Unit 1.1 CLI edits or Unit 1.2 package delete).
- **`valvcompat` sole Go importer**: `grep -rn "compat \"github.com/evanmschultz/valv\"\|valvcompat" main/internal main/cmd main/magefile.go` returns exactly one external importer line — `internal/services/openaiapi/service.go:16`. All other matches are inside `compatibility.go` / `compatibility_test.go` themselves. Confirms planner's claim that after Unit 1.2 deletes `openaiapi`, `valvcompat` is orphan-ready for Unit 1.4.
- **Doc scrubs verified**:
  - `AGENTS.md`: line 12 `valv/v1/api` present; line 61 `For API compatibility surfaces:` present; lines 216-217 `api serve` bullets present; line 320 `## OpenAI Compatibility Contract` section header present. All four scrub targets exist.
  - `AGENTS.md` line 281 `cheapest viable OpenAI-compatible model` present — confirms planner's PRESERVE-this claim (Unit 1.5 ac 4 last bullet).
  - `CLAUDE.md`: line 122 contains `valv api serve`; line 124 contains `internal/api/`. Both scrub targets exist.
  - `README.md`: section `### API Compatibility Matrix` starts line 69; section `### API Smoke Test Flow` starts line 87; last API-related line `manage cleanup docker` at line 129. Range 69-129 planner gives is correct.
  - `CONTRIBUTING.md`: `Compatibility change policy:` header at line 13, through line 16. Correct.
  - `VALV_REPO_PLAN.md`: line 77 `├── api/`; line 78 `│   └── openapi/`. Correct.
  - `valv_architecture_notes.md`: `## Public API shape` starts line 250. Correct.
- **Skip justifications** (P7):
  - §2 item 11 (PLAN.md supersede note): `grep -n "Track-F\|Agent 5\|Agent-5\|/v1/chat" main/PLAN.md` returns zero matches. PLAN.md is the ten-container drop tree rewrite — planner's skip is justified.
  - §2 item 15 (config fields): `grep -rnE "APIServe|api\.listen|api_serve|^\s*Listen\b" main/internal/config` returns zero matches. Skip justified.
  - §2 item 16 (magefile api target): `grep -in "api\b" main/magefile.go` returns zero matches. Skip justified.
- **Compile-safe ordering audit**:
  - After 1.1: `internal/cli` has no `openaihandler` / `openaiapiservice` imports. `openaiapi` package still in tree, still compiles (its own deps — `openai`, `valvcompat`, `domain`, `adapters` — are intact). Tree compiles.
  - After 1.2: `openaiapi` deleted. `openai` imports only `domain` (self-contained). `valvcompat` has zero importers. Tree compiles.
  - After 1.3: `openai` deleted. `valvcompat` still alive with zero Go importers. Tree compiles.
  - After 1.4: `valvcompat` deleted. Tree compiles.
  - After 1.5: docs-only, no Go change.
- **`internal/cli/codex_integration_test.go` API-reference freedom**: `grep -n "api serve\|openai\|openaihandler\|openaiapi" main/internal/cli/codex_integration_test.go` returns zero matches. Confirms Unit 1.2 ac 4 parenthetical claim (`internal/cli/codex_integration_test.go` contains zero `api serve` / `openai` references).
- **Coverage-floor context**: `magefile.go:24 coverageThreshold = 60.0` confirmed. Aligns with `REFINEMENTS.md` DROP_0 entry 1 and planner's cross-cutting note about Unit 1.1 possibly pushing `internal/cli` below 60% when 5 tests get stripped. Coverage escalation path (planner → orchestrator) is documented in the cross-cutting note — reasonable.

Unknowns routed back to orchestrator: none that affect the verdict. Advisory findings above are non-blocking prose-tightening opportunities, not evidence gaps.
