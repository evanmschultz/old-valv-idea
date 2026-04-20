# DROP_1 — Builder Worklog

Append a `## Unit N.M — Round K` section per build attempt. See `main/drops/WORKFLOW.md` § "Phase 4 — Build (per unit)" for what each section should contain.

## Unit 1.1 — Round 1

- **Builder:** go-builder-agent
- **Started:** 2026-04-19 21:07
- **Pre-change coverage (`internal/cli`):** 72.1% (`mage testPkg ./internal/cli`, 103 tests pass).
- **Post-change coverage (`internal/cli`):** 71.8% (`mage testPkg ./internal/cli`, 97 tests pass). Well above the 60% floor; 0.3 pp drop from removing 6 tests (5 wrapper tests + 1 orphan sweeper test) while deleting ~237 lines of production code + dependent test fixtures.
- **Files touched:**
  - `internal/cli/api.go` — DELETED (237 lines).
  - `internal/cli/root.go` — removed `valv api serve --runtime-ttl 2m` from root `Example` block; removed `apiCmd := newAPICommand(paths, opts)` + `apiCmd.GroupID = "runtime"` + `apiCmd` entry in `cmd.AddCommand(...)`.
  - `internal/cli/operator_helpers.go` — removed imports `openaihandler "github.com/evanmschultz/valv/internal/api/openai"`, `openaiapiservice "github.com/evanmschultz/valv/internal/services/openaiapi"`, and `time` (only referenced by `newOpenAIAPIService`); removed the 22-line `newOpenAIAPIService` function; removed the dangling `var _ = openaihandler.ChatCompletionsPath`.
  - `internal/cli/extended_test.go` — removed imports `net`, `time`, `openaiapi "github.com/evanmschultz/valv/internal/api/openai"` (`net` and `time` were only referenced by the deleted tests, `openaiapi` by `apiServeStubService.Complete`); removed `apiServeStubService` type + 5 methods (28 lines); removed `installStubOpenAIAPIServiceFactory` helper (10 lines); removed the 5 PLAN-listed tests `TestRunAPIServeStartsAndStopsCleanly` / `TestRunAPIServeReturnsShutdownError` / `TestRunAPIServeReturnsBindErrorBeforeAnnouncingSuccess` / `TestRunAPIServeRejectsNonPositiveRuntimeTTL` / `TestNewOpenAIAPIServiceCreatesService`; ALSO removed `pruneRecorder` type (8 lines, 53-60) and `TestRunAPIRuntimeSweeperPrunesUntilContextCancel` (18 lines, 781-798) — see plan-gap note above. Both exist solely to test symbols in the deleted `api.go` and leaving them in place yields `undefined: runAPIRuntimeSweeper` at compile time.
- **Mage targets run:**
  - `mage testPkg ./internal/cli` (pre-edit): exit 0, 103 tests, 72.1% coverage.
  - `mage testPkg ./internal/cli` (post-edit): exit 0, 97 tests, 71.8% coverage.
  - `mage test` (post-edit): exit 0, 353 tests across 21 packages; all packages ≥ 60% floor.
  - `mage build` (post-edit): exit 0, produced `./valv`.
  - `./valv --help`: zero occurrences of `api` anywhere (no `RUNTIME COMMANDS` entry, no `Example` mention, no mention in body).
- **Acceptance checklist (PLAN § Unit 1.1):**
  1. `test ! -e internal/cli/api.go` → PASS.
  2. `grep -n "valv api serve" internal/cli/root.go` → 0 matches. PASS.
  3. `grep -n "newAPICommand\|apiCmd" internal/cli/root.go` → 0 matches. PASS.
  4. `grep -n "openaihandler\|openaiapiservice\|newOpenAIAPIService\|ChatCompletionsPath" internal/cli/operator_helpers.go` → 0 matches. PASS.
  5. `grep -n "openaiapi\|apiServeStub\|installStubOpenAIAPIServiceFactory\|TestRunAPIServe\|TestNewOpenAIAPIServiceCreatesService" internal/cli/extended_test.go` → 0 matches. PASS.
  6. Non-API helpers preserved: `grep -cE "^func openManageService|^func openGlobalSwitchService|^func openImagesService|^func newCleanupService|^func runManageHome|^func pickProfile|^func readAccountIdentity" internal/cli/operator_helpers.go` → exactly 7. PASS.
  7. Non-API tests preserved: `grep -c "^func Test" internal/cli/extended_test.go` → 32 (≥ 15 lower bound). PASS.
  8. Pre/post coverage captured (72.1% → 71.8%); post ≥ 60% floor, no escalation. PASS.
  9. `mage test` exits 0. PASS.
  10. `mage build` exits 0 and `./valv --help` has no `api` group. PASS.
- **Design notes:**
  - The six deleted tests and one deleted helper (`pruneRecorder`) are all fixture-level — behavior coverage they provided was for `runAPIServe` / `runAPIRuntimeSweeper`, both deleted in this unit. Nothing in the remaining production code references them.
  - `internal/api/openai` package still exists and remains imported by `internal/services/openaiapi/service.go`; its tests still run green on post-change `mage test`. Units 1.2 and 1.3 will remove those packages in subsequent commits, at which point coverage for those packages disappears from the report entirely.
  - Left untouched as per PLAN: `cmd.AddGroup(..., "runtime", ...)` declaration stays because `codex` still uses the `runtime` group.

### Plan gap flagged

The PLAN § Scope spec lists the 5 `TestRunAPIServe*` / `TestNewOpenAIAPIServiceCreatesService` tests for deletion but misses two other symbols that become orphans once `api.go` is deleted:

- `TestRunAPIRuntimeSweeperPrunesUntilContextCancel` (`extended_test.go:781-798`) calls `runAPIRuntimeSweeper`, which only lived in `api.go`.
- `pruneRecorder` type (`extended_test.go:53-60`) is the stub used only by that sweeper test.

Leaving either in place makes `mage test` fail with an undefined-symbol compile error. Neither matches the acceptance-5 grep regex `TestRunAPIServe|TestNewOpenAIAPIServiceCreatesService`. Per agent instructions ("report any additional files or packages you needed to touch back to the orchestrator via comment (don't silently expand scope)"): the builder is deleting both as compile-necessity collateral of the `api.go` deletion called out in the PLAN, and flagging this to the orch for PLAN refinement / QA awareness. No code outside the four listed `internal/cli` files is touched.

### Hylla Feedback

N/A — no Hylla query issued. All evidence came from Read/Grep/LSP against uncommitted + committed state in the four target files, which is correct per main/CLAUDE.md § "Code Understanding Rules" item 2 (`git diff` / direct Read for files changed / changing since last ingest).

## Unit 1.2 — Round 1

- **Builder:** go-builder-agent
- **Started:** 2026-04-19
- **Leaf-status re-verification:**
  - `Grep openaiapi *.go` across `main/` — all matches confined to `internal/services/openaiapi/*.go` (self-refs only: the 5 files' own `package openaiapi` declarations, `service.go`/`service_test.go`/`codex_models.go` importing the leaf `internal/api/openai` package under the alias `openaiapi`).
  - `Grep openaiapi|services/openaiapi main/cmd` — 0 matches.
  - `Grep openaiapi|services/openaiapi main/magefile.go` — 0 matches.
  - `Grep openaiapi|openai|api serve main/internal/cli/codex_integration_test.go` — 0 matches (acceptance-4 precondition).
  - Conclusion: `internal/services/openaiapi` is a leaf consumer of `internal/api/openai` + `valvcompat`, with zero outgoing edges into Codex runtime/provider/adapter packages. Safe to delete as specified.
- **Files deleted (5):**
  - `internal/services/openaiapi/codex_events.go`
  - `internal/services/openaiapi/codex_models.go`
  - `internal/services/openaiapi/service.go`
  - `internal/services/openaiapi/service_test.go`
  - `internal/services/openaiapi/service_integration_test.go`
  - Empty parent directory `internal/services/openaiapi/` also removed via `rmdir` (no leftover artifacts).
- **Mage targets run:**
  - `mage test` (post-edit): exit 0, 334 tests across 20 packages, all packages ≥ 60% floor. Package count dropped from 21 → 20 (openaiapi removed). Notable cover figures: `internal/cli` 71.8% (unchanged from Unit 1.1 post), `internal/api/openai` 72.3% (still green, to be removed in Unit 1.3), `internal/services/codex` 75.2%.
  - `mage integration` (post-edit): exit 0, 100 tests across `./internal/cli`.
- **Acceptance checklist (PLAN § Unit 1.2):**
  1. `test ! -d main/internal/services/openaiapi` → PASS.
  2. `grep -rn "openaiapi\|services/openaiapi" main/internal main/cmd main/magefile.go` → 0 matches (verified via `Grep` over each scope separately). PASS.
  3. `mage test` from `main/` exits 0 (334/334 passed, coverage floor met). PASS.
  4. `mage integration` from `main/` exits 0 (100/100 passed). PASS.
- **Design notes:**
  - No production code remained to delete beyond the 5 files — no callers, no interface impls elsewhere.
  - `internal/api/openai` package is still present and compiles (its own tests pass, 72.3% cover). Scheduled for deletion in Unit 1.3.
  - `valvcompat` root package still present (its sole importer was `internal/services/openaiapi/service.go`, now gone). Scheduled for deletion in Unit 1.4.
  - No scope expansion — only the 5 listed files were touched. No plan gap flagged for this unit.

### Hylla Feedback

N/A — task touched Go-file deletion only; evidence came entirely from Grep over uncommitted local state (the file set is changing relative to the latest ingest, so per main/CLAUDE.md § "Code Understanding Rules" item 2, direct Grep/Read is the correct evidence source).

## Unit 1.3 — Round 1

- **Builder:** go-builder-agent
- **Started:** 2026-04-19
- **Leaf-status re-verification:**
  - `Grep "internal/api/openai|openaihandler|openai.ChatCompletionsPath"` across `main/internal`, `main/cmd`, `main/magefile.go` → 0 matches in each scope (three separate `Grep` calls).
  - Broader `main/` grep surfaces only the 8 target files themselves plus drop-artifact markdown (PLAN.md, BUILDER_WORKLOG.md, BUILDER_QA_*.md, PLAN_QA_*.md, focus plan, bare-root PLAN.md) and the root PLAN.md drop-tree row — expected documentation residue, no live Go consumer. `internal/services/openaiapi` (the only Go consumer) was removed in Unit 1.2 at commit `97bc656`.
  - Directory audit: `internal/api/` had only `openai/` as a child prior to deletion, so removing `openai/` renders the parent empty and removable.
- **Files deleted (8):**
  - `internal/api/openai/doc.go`
  - `internal/api/openai/encode.go`
  - `internal/api/openai/errors.go`
  - `internal/api/openai/handler.go`
  - `internal/api/openai/handler_test.go`
  - `internal/api/openai/json.go`
  - `internal/api/openai/types.go`
  - `internal/api/openai/types_test.go`
  - Empty parent directory `internal/api/openai/` removed via `rmdir`; then empty grandparent `internal/api/` also removed via `rmdir` (no siblings remained).
- **Mage targets run:**
  - `mage test` (post-edit): exit 0, 312 tests across 19 packages, all packages ≥ 60% floor. Package count dropped 20 → 19 (`internal/api/openai` removed). Notable cover figures: `internal/cli` 71.8% (unchanged from Unit 1.2 post), `internal/adapters/docker` 64.7% (the other 60%-bend package, still within floor), `internal/services/codex` 75.2%.
- **Acceptance checklist (PLAN § Unit 1.3):**
  1. `test ! -d internal/api/openai` → "AC1 PASS: internal/api/openai does not exist". PASS.
  2. `test ! -d internal/api` → "AC2 PASS: internal/api does not exist". PASS.
  3. `Grep "internal/api/openai|openaihandler|ChatCompletionsPath"` across `main/internal`, `main/cmd`, `main/magefile.go` — three separate scopes, each returned "No matches found". PASS.
  4. `mage test` from `main/` → exit 0, 312/312 passed, coverage floor (60%) met by every package. PASS.
- **Design notes:**
  - `valvcompat` root-level package (`compatibility.go`, `compatibility_test.go`, `codex-openai-compatibility.json`) is still present. Its sole importer (`internal/services/openaiapi/service.go`) is already gone as of Unit 1.2; it is now an orphan package with its own tests. Scheduled for deletion in Unit 1.4. The 3 files still compile and the `valvcompat` tests still pass green (evidenced by `mage test` showing `github.com/evanmschultz/valv` at 100.0% cover — that is the `valvcompat`-hosting root package).
  - `mage integration` was not re-run — Unit 1.2's closing run was the last deletion of an integration-test file (`internal/services/openaiapi/service_integration_test.go`). Unit 1.3 deletes no integration files; `internal/api/openai/handler_test.go` is a unit test without a build tag. Running `mage integration` here is not an acceptance requirement per PLAN § Unit 1.3 and adds no signal the completed `mage test` doesn't already provide.
  - No scope expansion — only the 8 listed files + the two empty parent directories were touched in source. PLAN.md (state flip) and BUILDER_WORKLOG.md (this section) edits are the drop-coordination writes called for by WORKFLOW.md Phase 4.

### Hylla Feedback

N/A — task was pure file deletion. Evidence came entirely from `Grep` over the current uncommitted local state (the file set is changing relative to the latest ingest, so per `main/CLAUDE.md` § "Code Understanding Rules" item 2, direct `Grep` / `Read` is the correct evidence source). No Hylla query was attempted or needed.

## Unit 1.4 — Round 1

- **Builder:** go-builder-agent
- **Started:** 2026-04-19
- **Leaf-status re-verification:**
  - `Grep "valvcompat|CodexOpenAICompatibility|codex-openai-compatibility"` scoped to `main/internal`, `main/cmd`, `main/magefile.go` — 0 matches in each scope (three separate `Grep` calls). Confirms `internal/services/openaiapi` (removed in Unit 1.2 at commit `97bc656`) was indeed the sole importer; `valvcompat` has been orphan since 1.2.
  - Broader `Grep` across all of `main/` returned 13 matches: the 3 target files themselves (`compatibility.go`, `compatibility_test.go`, embedded JSON referenced by filename inside `compatibility_test.go`), plus drop-coordination markdown (DROP_1 `PLAN.md`, `BUILDER_WORKLOG.md`, `BUILDER_QA_*.md`, `PLAN_QA_*.md`), root `PLAN.md`, `README.md`, `AGENTS.md`, `CONTRIBUTING.md`, `VALV_CLAUDE_CODE_FOCUS_PLAN.md`. All markdown residue is scheduled for scrub in Unit 1.5. No live Go consumer.
  - `Grep` scoped to `*.go` files under `main/` showed every matching line lives in `compatibility.go` or `compatibility_test.go` (the 2 files being deleted). No embed, import, or symbol reference in any other `.go` file.
- **Files deleted (3):**
  - `compatibility.go` (71 lines, `package valvcompat`)
  - `compatibility_test.go` (108 lines, `package valvcompat`)
  - `codex-openai-compatibility.json` (13 KB manifest previously embedded via `//go:embed`)
- **Mage targets run:**
  - `mage test` (post-edit): exit 0, 309 tests across 18 packages. Package count dropped 19 → 18 (root-level `valvcompat` / `github.com/evanmschultz/valv` package removed — the 100.0% cover line seen in Unit 1.3 for the root package was `valvcompat`'s coverage; now gone from the report). Minimum package coverage 64.7% (`internal/adapters/docker`, unchanged), well above the 60% floor. `internal/cli` 71.8% (unchanged from Unit 1.3 post).
- **Acceptance checklist (PLAN § Unit 1.4):**
  1. `test ! -e main/compatibility.go` → PASS.
  2. `test ! -e main/compatibility_test.go` → PASS.
  3. `test ! -e main/codex-openai-compatibility.json` → PASS.
  4. `grep -rn "valvcompat\|CodexOpenAICompatibility\|codex-openai-compatibility" main/internal main/cmd main/magefile.go` → 0 matches across all three scopes (three separate `Grep` calls). PASS.
  5. `mage test` from `main/` exits 0 (309/309 passed, all packages ≥ 60% floor). PASS.
- **Design notes:**
  - The `mage test` output no longer includes a `github.com/evanmschultz/valv` (root package) line because the root package has zero remaining `.go` files after this deletion. This is the expected consequence of removing the only three files in the root package — `valvcompat` was the root package's entire surface.
  - `cmd/valv/main.go` still compiles and runs cleanly — it only ever depended on `internal/*` packages, never on the root `valvcompat` package.
  - No scope expansion. Only the 3 listed files were deleted. No plan gap flagged for this unit. PLAN.md state flip + this `BUILDER_WORKLOG.md` append are the drop-coordination writes called for by WORKFLOW.md Phase 4.

### Hylla Feedback

N/A — task was pure file deletion. Evidence came from `Grep` over the current uncommitted local state (file set is changing relative to the latest ingest, so per `main/CLAUDE.md` § "Code Understanding Rules" item 2, direct `Grep` / `Read` is the correct evidence source). No Hylla query was attempted or needed.

