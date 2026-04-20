# DROP_1 — DELETE API WRAPPER

**State:** planning
**Blocked by:** —
**Paths (expected):** `internal/api/openai/`, `internal/services/openaiapi/`, `internal/cli/api.go`, `internal/cli/root.go` (edit), `internal/cli/operator_helpers.go` (edit), `internal/cli/extended_test.go` (edit), `compatibility.go`, `compatibility_test.go`, `codex-openai-compatibility.json`, `API_COMPAT_EXECUTION_PLAN.md`, plus doc scrubs in `README.md`, `AGENTS.md`, `CLAUDE.md`, `VALV_REPO_PLAN.md`, `valv_architecture_notes.md`, `CONTRIBUTING.md`
**Packages (expected):** `internal/api/openai` (deleted), `internal/services/openaiapi` (deleted), `valvcompat` (root package, deleted); `internal/cli` (edits only — remove wiring + stubs + tests, keep the rest)
**PLAN.md ref:** main/PLAN.md → DROP_1_DELETE_API_WRAPPER row
**Workflow:** main/drops/WORKFLOW.md
**Started:** 2026-04-19
**Closed:** —

## Scope

Strip the legacy OpenAI-compat API wrapper and its doc trail per `VALV_CLAUDE_CODE_FOCUS_PLAN.md` §2 so `valv api` is gone and tests are green. Verified filesystem footprint:

- `internal/api/openai/` — 8 files (`doc.go`, `encode.go`, `errors.go`, `handler.go`, `handler_test.go`, `json.go`, `types.go`, `types_test.go`). Deleted whole.
- `internal/services/openaiapi/` — 5 files (`codex_events.go`, `codex_models.go`, `service.go`, `service_test.go`, `service_integration_test.go`). Deleted whole.
- `internal/cli/api.go` (237 lines) — deleted whole.
- `internal/cli/root.go` — edits: remove `valv api serve --runtime-ttl 2m` from `Example` (line 63), drop `apiCmd := newAPICommand(...)` + `.GroupID` + `AddCommand` entries (lines 126-135 current numbering).
- `internal/cli/operator_helpers.go` — edits: remove imports `openaihandler "github.com/evanmschultz/valv/internal/api/openai"` (line 17) and `openaiapiservice "github.com/evanmschultz/valv/internal/services/openaiapi"` (line 25); remove `newOpenAIAPIService` func (lines 107-128); remove dangling `var _ = openaihandler.ChatCompletionsPath` (line 315).
- `internal/cli/extended_test.go` (1115 lines) — edits: remove `openaiapi "github.com/evanmschultz/valv/internal/api/openai"` import (line 20), `apiServeStubService` type + its five methods (lines 62-89), `installStubOpenAIAPIServiceFactory` helper (lines 91-100), and the five wrapper tests `TestRunAPIServeStartsAndStopsCleanly` / `TestRunAPIServeReturnsShutdownError` / `TestRunAPIServeReturnsBindErrorBeforeAnnouncingSuccess` / `TestRunAPIServeRejectsNonPositiveRuntimeTTL` / `TestNewOpenAIAPIServiceCreatesService` (lines 884-1059).
- Root-level `valvcompat` package — `compatibility.go` (71 lines, `package valvcompat`), `compatibility_test.go` (108 lines, `package valvcompat`), embedded manifest `codex-openai-compatibility.json` (verified sole importer is `internal/services/openaiapi/service.go`). All three deleted.
- `API_COMPAT_EXECUTION_PLAN.md` — deleted (76 lines).
- Doc scrubs: `README.md` §"API Compatibility Matrix" + §"API Smoke Test Flow" (lines 69-129); `CONTRIBUTING.md` "Compatibility change policy" bullets (lines 13-16); `AGENTS.md` §1 line 12 (`valv/v1/api` bullet), §4 lines 61-65 ("For API compatibility surfaces" block), §8 lines 216-217 (`api serve` bullets), §"OpenAI Compatibility Contract" (lines 320-332); `CLAUDE.md` line 122 (`valv api serve` mention) and line 124 (`internal/api/` bullet); `VALV_REPO_PLAN.md` line 78 (`api/openapi/` tree entry, plus parent `├── api/` line 77 if orphaned); `valv_architecture_notes.md` "Public API shape" section (lines 250-264) gets a supersede note.

Preserve every other code path: Codex-in-Docker with profiles, Claude runtime scaffolding (future drops), `internal/adapters/docker`, `internal/services/codex`, `internal/services/images`, `internal/services/manage`, `internal/adapters/providers/codex`, account management, TUI, and mage targets all stay. Codex-branded strings (e.g. "OpenAI Codex" in `testdata/*`, `@openai/codex` npm package in `internal/services/images/service.go`, `OPENAI_API_KEY` in `internal/adapters/providers/codex/account.go`, "OpenAI-compatible model" testing-cost guidance in `AGENTS.md` §11 line 281) are Codex-product branding, NOT wrapper prose — they stay untouched.

Two focus-plan §2 items are obsolete and will be skipped with a note rather than a unit: item 11 (supersede note at top of `main/PLAN.md`) because the current `main/PLAN.md` is already the rewritten drop-tree index, not the old Track-F document with API content; item 15 (config-field grep) was verified in-planning as a no-op — `internal/config/` has zero `listen`/`APIServe`/`api.listen` fields.

`mage test` must be green at every unit boundary; compile-safe ordering is enforced by `blocked_by`. Driver for this drop is TOS: running a local OpenAI-compat proxy conflicts with provider terms; pass-through to each vendor's own CLI (Codex today, Claude next drop) replaces it.

## Planner

Evidence sources: Hylla ingest at commit `1bd5f98` (queryable but not needed for deletions — direct Read/Grep/LSP covered all cases), `git ls-files`, `Grep` across `main/` recursively, LSP `findReferences` on `internal/api/openai/types.go` (returned 15 references across 15 files, all accounted for in this decomposition). Focus-plan §2 is the authoritative scope contract; this decomposition maps §2 items 1-16 onto 5 mechanically-verifiable units with compile-safe ordering.

**Decomposition rationale (ordering).** Import graph goes `openaiapi → openai + valvcompat`, `cli/api.go → openai + openaiapi (via operator_helpers)`, `cli/root.go → newAPICommand (in api.go)`. Deletions must remove consumers before callees or `mage test` breaks mid-drop. Unit order: (1) detach `internal/cli` consumers, (2) delete `openaiapi` (leaf consumer of `openai`), (3) delete `openai` (HTTP handler), (4) delete `valvcompat` (lost its sole importer in step 2), (5) doc scrubs (no compile impact). Each sequential step compiles cleanly; `blocked_by` makes the dependency explicit.

**Cross-cutting note on coverage floor.** Unit 1.1 removes ~175 lines of tests from `internal/cli/extended_test.go`. `magefile.go` currently enforces a 60% per-package coverage floor (temp bend from DROP_0, tracked in `REFINEMENTS.md`). If the test deletion drops `internal/cli` below 60%, `mage test` will fail at the 1.1 boundary and the builder must respond. The plan does NOT pre-emptively add tests — adding tests inside a deletion drop violates single-concern. If coverage fails, the drop's builder escalates back to the orch; the orch decides whether to add a test-restoration unit or further temp-bend the floor with a REFINEMENTS entry. This is surfaced here so the builder and QA are not surprised.

### Unit 1.1 — Detach `valv api` wiring from CLI

- **State:** todo
- **Paths:**
  - `internal/cli/api.go` (DELETE entire file, 237 lines)
  - `internal/cli/root.go` (EDIT)
  - `internal/cli/operator_helpers.go` (EDIT)
  - `internal/cli/extended_test.go` (EDIT)
- **Packages:** `internal/cli` (edits only)
- **Acceptance:**
  1. File `internal/cli/api.go` does not exist (`test ! -e main/internal/cli/api.go`).
  2. `grep -n "valv api serve" main/internal/cli/root.go` returns zero matches.
  3. `grep -n "newAPICommand\|apiCmd" main/internal/cli/root.go` returns zero matches.
  4. `grep -n "openaihandler\|openaiapiservice\|newOpenAIAPIService\|ChatCompletionsPath" main/internal/cli/operator_helpers.go` returns zero matches.
  5. `grep -n "openaiapi\|apiServeStub\|installStubOpenAIAPIServiceFactory\|TestRunAPIServe\|TestNewOpenAIAPIServiceCreatesService" main/internal/cli/extended_test.go` returns zero matches.
  6. Non-API helpers in `operator_helpers.go` remain intact: `grep -n "^func openManageService\|^func openGlobalSwitchService\|^func openImagesService\|^func newCleanupService\|^func runManageHome\|^func pickProfile\|^func readAccountIdentity" main/internal/cli/operator_helpers.go` returns exactly 7 matches.
  7. Non-API tests in `extended_test.go` remain intact: `grep -cE "^func Test" main/internal/cli/extended_test.go` returns at least 15 (spot-check lower bound; pre-drop count is higher — the builder records the exact post-delta figure in the worklog).
  8. `mage test` from `main/` exits 0. (Per-package coverage floor is currently 60%; if the deletion pushes `internal/cli` below, the builder escalates — see planner note above.)
  9. `mage build` from `main/` exits 0 and the produced `./valv --help` output contains no `api` group (verified by `./valv --help 2>&1 | grep -c "^  api\b" | grep -qx 0`).
- **Blocked by:** —

### Unit 1.2 — Delete `internal/services/openaiapi/` package

- **State:** todo
- **Paths:**
  - `internal/services/openaiapi/codex_events.go` (DELETE)
  - `internal/services/openaiapi/codex_models.go` (DELETE)
  - `internal/services/openaiapi/service.go` (DELETE)
  - `internal/services/openaiapi/service_test.go` (DELETE)
  - `internal/services/openaiapi/service_integration_test.go` (DELETE)
- **Packages:** `internal/services/openaiapi` (deleted entirely)
- **Acceptance:**
  1. Directory `main/internal/services/openaiapi/` does not exist (`test ! -d main/internal/services/openaiapi`).
  2. `grep -rn "openaiapi\|services/openaiapi" main/internal main/cmd main/magefile.go` returns zero matches.
  3. `mage test` from `main/` exits 0.
  4. `mage integration` from `main/` exits 0 (the only openaiapi integration file is deleted in this unit; `internal/cli/codex_integration_test.go` contains zero `api serve` / `openai` references — verified).
- **Blocked by:** 1.1

### Unit 1.3 — Delete `internal/api/openai/` package

- **State:** todo
- **Paths:**
  - `internal/api/openai/doc.go` (DELETE)
  - `internal/api/openai/encode.go` (DELETE)
  - `internal/api/openai/errors.go` (DELETE)
  - `internal/api/openai/handler.go` (DELETE)
  - `internal/api/openai/handler_test.go` (DELETE)
  - `internal/api/openai/json.go` (DELETE)
  - `internal/api/openai/types.go` (DELETE)
  - `internal/api/openai/types_test.go` (DELETE)
- **Packages:** `internal/api/openai` (deleted entirely); `internal/api/` parent directory becomes empty and is also removed.
- **Acceptance:**
  1. Directory `main/internal/api/openai/` does not exist (`test ! -d main/internal/api/openai`).
  2. Directory `main/internal/api/` does not exist (`test ! -d main/internal/api`) — `internal/api/openai` was its only subpackage.
  3. `grep -rn "internal/api/openai\|openaihandler\|ChatCompletionsPath" main/internal main/cmd main/magefile.go` returns zero matches.
  4. `mage test` from `main/` exits 0.
- **Blocked by:** 1.2

### Unit 1.4 — Delete root-level `valvcompat` package and embedded manifest

- **State:** todo
- **Paths:**
  - `compatibility.go` (DELETE, `package valvcompat`)
  - `compatibility_test.go` (DELETE, `package valvcompat`)
  - `codex-openai-compatibility.json` (DELETE, 13KB manifest embedded via `//go:embed`)
- **Packages:** `valvcompat` (root-level package, deleted entirely). `internal/services/openaiapi` was the sole importer — already removed in 1.2.
- **Acceptance:**
  1. File `main/compatibility.go` does not exist (`test ! -e main/compatibility.go`).
  2. File `main/compatibility_test.go` does not exist (`test ! -e main/compatibility_test.go`).
  3. File `main/codex-openai-compatibility.json` does not exist (`test ! -e main/codex-openai-compatibility.json`).
  4. `grep -rn "valvcompat\|CodexOpenAICompatibility\|codex-openai-compatibility" main/internal main/cmd main/magefile.go` returns zero matches.
  5. `mage test` from `main/` exits 0.
- **Blocked by:** 1.2

### Unit 1.5 — Delete `API_COMPAT_EXECUTION_PLAN.md` and scrub doc prose

- **State:** todo
- **Paths:**
  - `API_COMPAT_EXECUTION_PLAN.md` (DELETE)
  - `README.md` (EDIT — drop §"API Compatibility Matrix" and §"API Smoke Test Flow", lines ~69-129)
  - `CONTRIBUTING.md` (EDIT — drop "Compatibility change policy" block, lines 13-16)
  - `AGENTS.md` (EDIT — four scrubs detailed below)
  - `CLAUDE.md` (EDIT — two scrubs in § "Project Structure" / "Package Map")
  - `VALV_REPO_PLAN.md` (EDIT — drop `api/openapi/` line 78; drop parent `├── api/` line 77 if it becomes an empty container after child removal)
  - `valv_architecture_notes.md` (EDIT — add one-line supersede note above §"Public API shape" at line 250 pointing at `VALV_CLAUDE_CODE_FOCUS_PLAN.md`; do NOT delete the section — focus plan §2.12 says defer deletion to a later doc sweep)
- **Packages:** none (docs only)
- **Acceptance:**
  1. File `main/API_COMPAT_EXECUTION_PLAN.md` does not exist.
  2. `grep -n "API Compatibility Matrix\|API Smoke Test Flow\|valv api serve" main/README.md` returns zero matches.
  3. `grep -n "Compatibility change policy\|codex-openai-compatibility\|valv api" main/CONTRIBUTING.md` returns zero matches.
  4. `AGENTS.md` scrubs verified by grep:
     - `grep -n "valv/v1/api" main/AGENTS.md` returns zero matches (§1 line 12 removed/replaced).
     - `grep -nE "For API compatibility surfaces|OpenAI-compatible handler surface|Anthropic-compatible handler surface" main/AGENTS.md` returns zero matches (§4 lines 61-65 block removed).
     - `grep -n "api serve" main/AGENTS.md` returns zero matches (§8 lines 216-217 bullets removed).
     - `grep -n "^## OpenAI Compatibility Contract" main/AGENTS.md` returns zero matches (bottom section removed).
     - PRESERVED (do NOT scrub): `grep -n "cheapest viable OpenAI-compatible model" main/AGENTS.md` returns exactly 1 match (§11 line 281 is Codex-as-OpenAI-product testing-cost guidance, not wrapper prose).
  5. `CLAUDE.md` scrubs verified by grep:
     - `grep -n "valv api serve" main/CLAUDE.md` returns zero matches.
     - `grep -n "internal/api/" main/CLAUDE.md` returns zero matches (the `internal/api/` Package Map bullet is removed because the directory no longer exists).
  6. `VALV_REPO_PLAN.md` scrubs verified by grep:
     - `grep -n "api/openapi/\|openapi/" main/VALV_REPO_PLAN.md` returns zero matches.
  7. `valv_architecture_notes.md` scrub verified by grep:
     - `grep -n "Superseded by VALV_CLAUDE_CODE_FOCUS_PLAN" main/valv_architecture_notes.md` returns at least 1 match near the "Public API shape" section (supersede note present); the section itself stays intact per focus plan §2.12.
  8. `mage test` from `main/` exits 0 (no Go files changed; guards against accidental `.go` edits).
  9. `mage build` from `main/` exits 0 and binary `./valv --help` shows `codex` and `account` / `manage` / `global` groups but no `api` group.
- **Blocked by:** 1.4

## Notes

- **Focus plan §2 items skipped as obsolete** (one-line justifications, no units needed):
  - §2 item 11 — supersede note at top of `main/PLAN.md`: current `main/PLAN.md` is already the rewritten drop-tree index (DROP_0 deliverable), with zero Track-F / Agent-5 content. Adding a supersede note would be meaningless.
  - §2 item 15 — config-field grep: verified during planning that `internal/config/` contains zero `listen` / `APIServe` / `api.listen` / `api_serve` fields; nothing to remove.
  - §2 item 16 — magefile changes: verified no `api`-specific mage target exists (`Test`, `TestPkg`, `Integration`, `Golden`, `GoldenUpdate`, `Build`, `Run`, `Dev.*` are all provider-generic).
- **Codex-product branding stays.** `internal/services/images/service.go` line 27 GitHub URL `api.github.com/repos/openai/codex/releases/latest` and line 542 Dockerfile `@openai/codex@${CODEX_VERSION}` are Codex distribution metadata. `internal/adapters/providers/codex/account.go` / `account_test.go` reference `OPENAI_API_KEY` because that is Codex's auth env-var name. `internal/cli/testdata/TestCodexInteractiveMCPGolden.golden` and `codex-fixture/codex-fixture.sh` contain the string "OpenAI Codex" as Codex's own banner. None of these are wrapper prose — all stay.
- **Coverage floor.** `magefile.go:24` currently pins `coverageThreshold = 60.0` (DROP_0 temp bend tracked in `REFINEMENTS.md`). Unit 1.2 self-resolves one of the two packages that forced the bend (`internal/services/openaiapi` at 61.3%, now deleted). The other (`internal/adapters/docker` at 64.7%) is out of scope here — DO NOT raise the floor back to 70 in this drop; that is REFINEMENTS-tracked future work.
- **Blocked-by rationale.** 1.1 → 1.2 → (1.3 and 1.4 could be parallel since both depend only on 1.2, but sequential keeps one package-level edit scope per unit, matching the planner rule that two sibling units must not share a file/package without `blocked_by`) → 1.5. The chain is strictly linear to keep `mage test` green at each step and to keep commit granularity one-unit-one-commit per Phase 4 of `main/drops/WORKFLOW.md`.
- **Verification discipline.** Every unit ends with `mage test` passing (not `go test`). Units 1.1 and 1.5 additionally confirm `mage build` produces a binary whose `--help` lacks the `api` group — this is the user-visible acceptance of "`valv api` is gone" from the PLAN.md row.
