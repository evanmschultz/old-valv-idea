## Unit 1.1 — Round 1

**Verdict:** pass

**Commit under review:** `265be0d refactor(cli): detach valv api command wiring`

**Summary.** Eleven attack angles attempted. Every CONFIRMED-level attack ended up MITIGATED with concrete repro evidence. Two findings are informational (not counterexamples): a PLAN recommendation for acceptance-5 regex expansion, and an observation about `mage integration` timing. The unit is green.

### A1 — Orphan code search widened

- **Attack:** Grep for any remaining reference to symbols defined only in the deleted `api.go` (`runAPIRuntimeSweeper`, `runAPIServe`, `newAPICommand`, `newAPIServeCommand`, `warmRuntimeForAPIServe`, `openAIAPIServiceFactory`, `openAIAPIService` interface, `apiExecutor`, `pruneRecorder`, `apiServeStubService`, `installStubOpenAIAPIServiceFactory`) anywhere in `main/internal/cli/`.
- **Probe:** `Grep(pattern="runAPIRuntimeSweeper|runAPIServe|newAPICommand|apiExecutor|apiCmd|openAIAPIServiceFactory|openAIAPIService\b|newOpenAIAPIService", path=main/internal/cli)` → zero matches. `Grep(pattern="ChatCompletionsPath|newOpenAIAPIService|apiServeStub|openaihandler|openaiapiservice", path=main/internal/cli)` → zero matches. `Grep(pattern="pruneRecorder|RuntimeSweeper", path=main/internal/cli, type=go)` → zero matches.
- **Status:** MITIGATED. All deleted symbols are surgically removed from `internal/cli`. No residue.

### A2 — Cross-package orphans (units 1.2–1.5 not yet run)

- **Attack:** Does `internal/services/openaiapi/` or `internal/api/openai/` still compile after unit 1.1 given that their sole `internal/cli` consumer is gone?
- **Probe:** `mage testPkg ./internal/cli` exits 0 with 97 tests passing. The wider `mage test` (run in the builder's worklog) was green across 353 tests / 21 packages. `internal/services/openaiapi/service_integration_test.go` still references `openaihandler` but that's **within** the openaiapi package's own test, pointing at the still-live `internal/api/openai` package. No external `internal/cli`-shape reference to either exists.
- **Probe:** `Grep(pattern="runAPIServe|runAPIRuntimeSweeper|newAPICommand|newAPIServeCommand|warmRuntimeForAPIServe", path=main, glob=**/*.go)` → zero matches across the whole tree.
- **Status:** MITIGATED. Units 1.2–1.5 will excise the dangling packages; unit 1.1 leaves them compile-clean.

### A3 — Preservation integrity (helper + test counts)

- **Attack:** Builder claims 7 preserved non-API helpers in `operator_helpers.go` and 32 non-API `Test*` in `extended_test.go`. Verify both, and verify nothing removed that shouldn't have been.
- **Probe helpers:** `Grep(pattern="^func openManageService|^func openGlobalSwitchService|^func openImagesService|^func newCleanupService|^func runManageHome|^func pickProfile|^func readAccountIdentity", path=operator_helpers.go)` → 7 matches at lines 31, 49, 68, 97, 104, 137, 202. `LSP documentSymbol` on `operator_helpers.go` lists all 7 plus additional helpers (`listItemsForAccounts`, `listItemsForBindings`, `codexAuthDisplay`, `codexEmailDisplay`, `realHomeDir`, `currentContainerUser`, `commandOutputMode`, `parseOptionalProvider`, `requireProjectPath`) — every non-API helper that existed pre-change is still present.
- **Probe tests:** `Grep(pattern="^func Test", path=internal/cli/extended_test.go, output_mode=count)` → 32. Matches builder claim.
- **Cross-check with pre-change:** pre-change worklog lists 103 tests across `internal/cli` (via `mage testPkg`). Post-change lists 97. Delta = 6 deletions: the 5 PLAN-listed tests + `TestRunAPIRuntimeSweeperPrunesUntilContextCancel` = exactly 6.
- **Status:** MITIGATED. Counts match. No collateral deletion.

### A4 — Coverage cliff

- **Attack:** Re-measure `internal/cli` coverage and confirm 71.8%. Is there a specific uncovered block introduced by the removal that matters?
- **Probe:** `mage testPkg ./internal/cli` → `[PKG PASS] github.com/evanmschultz/valv/internal/cli (88.07s)` / `cover: 71.8%` / 97 tests pass / minimum package coverage 60.0% met.
- **Reasoning:** Pre-change 72.1% → post-change 71.8% (-0.3pp). The drop is vanishingly small because the deleted tests only exercised code that was *also* deleted. The uncovered region introduced by the delta is effectively zero — no "live code without its only test" cliff.
- **Status:** MITIGATED. 71.8% well above the 60% temp-bend floor. No silent coverage cliff.

### A5 — CLI surface verification

- **Attack:** `./valv --help` lacks `api`. Does `./valv api` error cleanly? Is there any stale help reference?
- **Probe `./valv --help`:** output shows INSPECT (paths, version), RUNTIME (codex only — no api), MANAGEMENT (account, global, manage). Zero `api` mentions anywhere. Examples list no `valv api`.
- **Probe `./valv api`:** exits 1 with `ERROR / unknown command "api" for "valv"`. Clean cobra-standard unknown-command response.
- **Status:** MITIGATED. Binary surface matches the deletion claim.

### A6 — Integration test collateral

- **Attack:** `internal/cli/codex_integration_test.go` exists (`//go:build integration`) — does it transitively depend on deleted `api.go` symbols?
- **Probe:** `Grep(pattern="api serve|openai|openaihandler|openaiapi", path=internal/cli/codex_integration_test.go)` → zero matches. The only `internal/cli` reference in the file is line 331 `go build … ./internal/cli/testdata/fake-mcp` — a test-fixture binary compile path unrelated to api.go.
- **Status:** MITIGATED. `mage integration` would not break on unit 1.1 deletion. (Note: the other integration test affected by DROP_1 is `internal/services/openaiapi/service_integration_test.go`, but that file is deleted in unit 1.2 scope, not 1.1.)

### A7 — Gofumpt / vet drift

- **Attack:** The worklog mentions an `operator_helpers.go:97 unused parameter: paths` hint. Pre-existing or newly introduced?
- **Probe:** `git show HEAD~1:internal/cli/operator_helpers.go` shows `newCleanupService(cmd *cobra.Command, paths config.Paths)` at the pre-change equivalent of line 97 with the same unused-param pattern — `paths` is not referenced inside the function body, only `cmd` is (for `LoggerFromContext(cmd.Context())`). The signature was identical before the commit.
- **Status:** MITIGATED (informational). Pre-existing smell, not introduced by unit 1.1. Out of scope for this unit; flag for a future REFINEMENTS entry if desired.

### A8 — Commit hygiene

- **Attack:** `git show --stat 265be0d` — exactly the expected 6 files with no collateral?
- **Probe:** Files in commit:
  - `drops/DROP_1_DELETE_API_WRAPPER/BUILDER_WORKLOG.md` (worklog append)
  - `drops/DROP_1_DELETE_API_WRAPPER/PLAN.md` (unit state todo → done)
  - `internal/cli/api.go` (237 deletions, file deleted)
  - `internal/cli/extended_test.go` (248 deletions net)
  - `internal/cli/operator_helpers.go` (28 deletions net)
  - `internal/cli/root.go` (5 changes: example line + 4 wiring lines)
- **Status:** MITIGATED. Exactly the expected 6 files. Commit message `refactor(cli): detach valv api command wiring` is Conventional and accurate.

### A9 — `mage integration` gate (PLAN observation, not a counterexample)

- **Attack:** PLAN Unit 1.1 acceptance-10 stops at `mage build` + `./valv --help`. No `mage integration`. Is this a PLAN miss?
- **Analysis:** Per `main/CLAUDE.md` § "Build Verification" + AGENTS.md § 12, integration runs "when relevant" at drop-end (Phase 6 of WORKFLOW.md), not per unit. For unit 1.1 specifically, the only integration file touched by the drop as a whole is `internal/services/openaiapi/service_integration_test.go`, deleted in unit 1.2. Running `mage integration` at the unit 1.1 boundary would PASS because the openaiapi package still exists and its integration test still compiles. Skipping it here is defensible.
- **Status:** informational. NOT a counterexample. Recommendation: the drop-end Phase 6 orchestrator run should include `mage integration` before the closeout — the PLAN's Unit 1.2 acceptance-4 already carries that gate.

### A10 — Binary inspection

- **Attack:** `./valv --version` / `file ./valv` — sane?
- **Probe:** `./valv --version` is not valid (no `--version` flag — Fang v2 exposes `valv version` subcommand). `./valv version` → `Valv version / version=dev`. `file ./valv` → `Mach-O 64-bit executable arm64`. Binary built cleanly.
- **Status:** MITIGATED.

### A11 — Escaped wrapper references

- **Attack:** `grep -r "ChatCompletionsPath|newOpenAIAPIService|apiServeStub|openaihandler|openaiapiservice" main/internal/cli` — expected zero.
- **Probe:** `Grep(pattern="ChatCompletionsPath|newOpenAIAPIService|apiServeStub|openaihandler|openaiapiservice", path=main/internal/cli)` → zero matches. The wider `main/` search shows those strings ONLY inside `internal/api/openai/` (unit 1.3 target), `internal/services/openaiapi/` (unit 1.2 target), `VALV_CLAUDE_CODE_FOCUS_PLAN.md` (planning doc — unit 1.5 doc scrub), and drop artifacts (PLAN.md, BUILDER_WORKLOG.md, PLAN_QA_*.md — expected documentation residue, not live references).
- **Status:** MITIGATED. All production residue confined to packages slated for deletion in later units.

### Recommendations / PLAN refinement notes

1. **Accept the builder's acceptance-5 regex-expansion suggestion** for future similar deletion drops. The regex `TestRunAPIServe|TestNewOpenAIAPIServiceCreatesService` underspecified the orphan sweep. A fuller expansion like `TestRunAPIServe|TestRunAPIRuntimeSweeper|TestNewOpenAIAPIServiceCreatesService|apiServeStub|pruneRecorder` would have pre-flagged the two symbols the builder escalated. Not a blocker for unit 1.1 — the builder correctly escalated. Route as a REFINEMENTS note for planner regex discipline.
2. **`newCleanupService(cmd, paths)` unused-param smell** (line 97) is pre-existing and out of unit 1.1 scope. Not a regression. Optional future cleanup entry if desired.
3. **Drop-end Phase 6** should include `mage integration` before closeout to exercise the `internal/cli/codex_integration_test.go` + `internal/services/cleanup` / `images` integration paths that survive the deletion. Already covered by the WORKFLOW.md phase spec and by PLAN unit 1.2 acceptance-4; calling it out here for completeness.

### Hylla Feedback

N/A — no Hylla query issued for this falsification round. All evidence came from `git show` / `git log` / `Read` / `Grep` / `LSP documentSymbol` / `./valv` binary probes / `mage testPkg` run. Unit 1.1 changes files modified in the last 4 commits (post-ingest), so `git diff` / direct Read is the correct source per `main/CLAUDE.md` § "Code Understanding Rules" item 2. Hylla would have been stale for these files.
