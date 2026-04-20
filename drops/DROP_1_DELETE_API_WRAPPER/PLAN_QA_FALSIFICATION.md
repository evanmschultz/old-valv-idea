# DROP_1 — Plan QA Falsification, Round 1

- Verdict: pass (with recorded informational findings + one minor plan gap)
- Reviewed: main/drops/DROP_1_DELETE_API_WRAPPER/PLAN.md @ bfebb85

## Attacks

### A1 — Missed importers
- Probe: `Grep(pattern="openai|openaiapi|valvcompat|ChatCompletions|codex-openai-compatibility|compatibility\.go", path=main/internal)` and `main/cmd` and `main/magefile.go` and `git ls-files`. Checked `//go:embed` directives, testdata fixtures, integration-tag files, internal/config/, internal/cli/testdata/.
- Result: mitigated
- Evidence:
  - Every committed importer of `internal/api/openai`, `internal/services/openaiapi`, or root `valvcompat` is inside a file the plan either deletes whole or edits with an identified line-range scrub: `internal/cli/api.go` (deleted 1.1); `internal/cli/operator_helpers.go` lines 17, 25, 107-128, 315 (edited 1.1); `internal/cli/extended_test.go` lines 20, 62-89, 91-100, 884-1059 (edited 1.1); `internal/services/openaiapi/*.go` (deleted 1.2); `internal/api/openai/*.go` (deleted 1.3); `compatibility.go`, `compatibility_test.go`, `codex-openai-compatibility.json` (deleted 1.4).
  - `internal/cli/codex_integration_test.go`: zero matches for `openai|openaiapi|api serve|ChatCompletions|valvcompat|compatibility` — planner's claim verified against current HEAD.
  - `//go:embed` directive only appears in `compatibility.go` itself (deleted 1.4) — no orphan embeds after deletion.
  - `internal/cli/testdata/` has only Codex-branded fixture output (`codex-fixture.sh`, `TestCodexInteractiveMCPGolden.golden`) which the plan correctly classifies as Codex-product branding (preserved).
  - `cmd/valv/`, `magefile.go`, `internal/config/`: zero matches for all API-wrapper identifiers.
  - `git ls-files | grep -iE "openai|valvcompat|compatibility"` returns exactly 17 files, every one of which the plan deletes or edits.

### A2 — Compile safety gaps
- Probe: Trace the import DAG to confirm each unit's boundary compiles.
- Result: mitigated
- Evidence:
  - Import chain: `internal/cli/api.go` → `internal/api/openai` (handler); `internal/cli/operator_helpers.go` → both `openai` and `openaiapi`; `internal/cli/extended_test.go` → `openai`; `internal/services/openaiapi/*` → both `openai` and `valvcompat`; `valvcompat` is root-only, no inbound imports except `openaiapi`.
  - 1.1 removes every `internal/cli` consumer of both `openai` and `openaiapi`. After 1.1, the only live consumer of `openaiapi` is 1.2's target (`openaiapi/` itself — self-referential). `openai` still has `openaiapi` as consumer.
  - 1.2 deletes `openaiapi/`. After 1.2, `openai` has zero consumers (1.1 already cleared cli consumers). `valvcompat` has zero consumers (was only imported by `openaiapi`).
  - 1.3 deletes `openai/`. No inbound imports remain to break.
  - 1.4 deletes `valvcompat`. No inbound imports remain.
  - 1.5 is docs-only.
  - Every boundary is compile-clean per the import DAG. `mage test` should pass at each boundary.

### A3 — Integration tests (build-tag paths)
- Probe: `Glob(**/*integration_test.go)`, inspect `//go:build` tags and directory ownership.
- Result: mitigated
- Evidence:
  - Four integration-tagged files exist: `internal/cli/codex_integration_test.go` (API-wrapper free, verified), `internal/services/openaiapi/service_integration_test.go` (deleted in 1.2), `internal/services/cleanup/service_integration_test.go` (zero API refs), `internal/services/images/service_integration_test.go` (zero API refs).
  - `mage integration` runs only `./internal/cli` with `-tags=integration` (magefile.go:124) — so `openaiapi/service_integration_test.go` is currently orphaned from both `mage test` (no build tag) and `mage integration` (wrong package path). Its deletion in 1.2 is cleanup-of-orphan, not a gate-changing path.
  - Plan 1.2 acceptance criterion 4 (`mage integration` exits 0) is defensible.

### A4 — Mage-target side effects
- Probe: `Grep(magefile.go for API-wrapper refs)`.
- Result: mitigated
- Evidence: zero matches. Magefile.go has no embed, no hardcoded `./internal/api/openai` or `./internal/services/openaiapi` path, no golden-update entry, no test-enumeration-by-name that touches the deleted packages.

### A5 — Coverage-floor risk quantified
- Probe: Line accounting across deletions + floor-gate mechanics (magefile.go:24 `coverageThreshold = 60.0`).
- Result: open (plan acknowledges via escape hatch; quantification below tightens the risk statement)
- Evidence:
  - `internal/cli` production-code lines today (non-_test.go): account_auth.go + api.go + codex_setup.go + codex.go + global.go + manage.go + notice.go + operator_helpers.go + paths.go + root.go + spinner.go + store.go + version.go = 209+237+176+301+103+1221+23+315+47+262+33+25+29 = 2981 lines.
  - 1.1 deletes `api.go` (237) plus ~25 lines from `operator_helpers.go` (imports + `newOpenAIAPIService` 107-128 + `var _` line 315). Post-1.1 production-code ≈ 2719 lines.
  - 1.1 deletes from `extended_test.go`: line 20 (1) + 62-89 (28) + 91-100 (10) + 884-1059 (176) = 215 lines. Plus root.go line 63 edit (~1 line). Post-1.1 test lines ≈ tests-2847 across internal/cli.
  - Risk channel: the deleted production code (api.go + the 25 lines in operator_helpers.go) was ~100% covered by the deleted tests (TestRunAPIServe*, TestNewOpenAIAPIServiceCreatesService). Removing fully-covered code AND its tests keeps the coverage RATIO approximately stable at first order. The second-order concern is that remaining internal/cli code may be covered at a lower rate, so removing a high-coverage block lowers the weighted average — by an unknown delta.
  - Current internal/cli coverage is NOT recorded in DROP_0 CLOSEOUT.md or REFINEMENTS.md (DROP_0 only called out `openaiapi` at 61.3% and `docker` at 64.7% — not `internal/cli`). So we cannot algebraically predict whether internal/cli lands above or below 60.0 after 1.1.
  - The plan's existing note ("if coverage fails, builder escalates to orch") is a real escape hatch, but it depends on orch availability and converts 1.1 into a potential multi-round unit. Informational: Phase 4 builders should record internal/cli coverage pre- and post-1.1 in `BUILDER_WORKLOG.md` so the coverage trajectory becomes visible before it becomes a failure. Recommend the planner add a pre-1.1 coverage baseline measurement (capture `mage test` output on the pre-1.1 tree; record the `internal/cli=X%` row) to `BUILDER_WORKLOG.md` as the first sub-step of 1.1 so the floor risk is quantified in-flight rather than latent.
  - This does not break the plan — the escalation path is declared — but the planner could reduce surprise by front-loading a measurement.

### A6 — Doc scrub completeness
- Probe: `Grep(pattern="valv api|openai-compat|OpenAI-compatible|OpenAI compatible|ChatCompletionsPath|openaiapi|valvcompat|api/openapi|internal/api/openai|compatibility\.go|API_COMPAT_EXECUTION_PLAN|codex-openai-compatibility|OpenAI Compatibility", glob="**/*.md")` recursively under main/ and bare-root.
- Result: **one plan gap + two informational findings**
- Evidence (plan gap — CONFIRMED counterexample against plan scope):
  - `main/CLAUDE.md` line 133 contains the string `internal/api` in the Import DAG paragraph: *"Linear layered flow: `internal/domain → internal/adapters → internal/services → (internal/cli | internal/tui)`. `internal/api` sits at the same consumer tier as `cli`/`tui`. `cmd/valv` wires `cli` (and its sub-commands that mount `tui` / `api`) at the top. No cycles — strictly layered."*
  - The plan's 1.5 scope for `CLAUDE.md` covers only line 122 (`valv api serve`) and line 124 (the `internal/api/` package-map bullet). The acceptance check `grep -n "internal/api/" main/CLAUDE.md` requires zero matches — which catches line 124 but NOT line 133 (which has `internal/api` without trailing slash).
  - After 1.1-1.4 delete the `internal/api/openai/` package and its parent `internal/api/` directory, the line 133 Import-DAG claim about `internal/api` "sits at the same consumer tier as cli/tui" becomes FALSE (the directory no longer exists). This is live documentation pointing at a deleted concept.
  - Suggested plan patch: 1.5 `CLAUDE.md` scope should also scrub the `internal/api` reference on line 133 (either delete the sentence or rewrite the Import DAG without it), and the acceptance criterion should add `grep -n "internal/api" main/CLAUDE.md` (no trailing slash) returns zero matches.
- Evidence (informational — `TOS_COMPLIANCE.md` mentions retracted shape):
  - `main/TOS_COMPLIANCE.md` references "OpenAI-compatible HTTP API" / "OpenAI-compatibility layer" on lines 5, 14, 73, 85, 239. These are RETRACTION-CONTEXT references — they explicitly name the retracted shape and explain why it's out of scope. Substantively different from live documentation. Arguably they should stay (retraction context has durable value). Plan doesn't address them either way; plan is defensible by silence, but a builder-time judgment call is unavoidable. Not a blocker.
- Evidence (informational — bare-root AGENTS.md stale):
  - `/Users/evanschultz/Documents/Code/hylla/valv/AGENTS.md` line 63 still carries "route OpenAI-compatible providers through the OpenAI-compatible handler surface" language — this is the bare-root (steward-scope) AGENTS.md, DIFFERENT from `main/AGENTS.md`. The plan only scrubs `main/AGENTS.md`. The bare-root file is steward-scope per the steward CLAUDE.md, but it's still authoritative for cross-cutting agent behavior. Leaving it stale creates a drift risk. Plan-scope is defensible (steward-owned file, bare-root changes are bare-root scope); recording as informational so the dev can decide whether to escalate to the steward.
- Evidence (informational — LEDGER.md / REFINEMENTS.md historical mentions):
  - `main/LEDGER.md` and `main/REFINEMENTS.md` mention `internal/services/openaiapi` in past-tense DROP_0 context. Historical ledger entries should stay; no scrub needed.

### A7 — Scope-creep attack
- Probe: Are any 1.5 scrubs over-zealous?
- Result: mitigated
- Evidence:
  - Every 1.5 scrub target is wrapper-related. `AGENTS.md` §11 line 281 is EXPLICITLY preserved by the plan (Codex-as-model-product testing-cost guidance). `testdata/codex-fixture.sh` banner strings ("OpenAI Codex") are preserved (Codex-product branding). `@openai/codex` npm refs in `internal/services/images/service.go` are preserved (distribution metadata). The preservation list is correct.
  - Minor observation: AGENTS.md line 281 ("cheapest viable OpenAI-compatible model") will feel stale after 1.5 — it's testing guidance for compat tests Valv no longer runs. But the planner's preservation choice is defensible as Codex-product-branding scope; rewording is legitimately §8 backlog, not §2. Informational.

### A8 — Planner evidence quality
- Probe: Verify every planner-cited line number against HEAD.
- Result: mitigated
- Evidence:
  - `root.go`: line 63 `valv api serve --runtime-ttl 2m` — verified.
  - `root.go`: lines 126-135 (`apiCmd := newAPICommand(...)`, `.GroupID`, `AddCommand`) — verified (line 126 is the assignment, 127 is GroupID, 135 is the AddCommand entry).
  - `operator_helpers.go`: line 17 openaihandler import, line 25 openaiapiservice import, lines 107-128 newOpenAIAPIService func (128 is `}`), line 315 dangling `var _` — all verified.
  - `extended_test.go`: line 20 import, 62-89 apiServeStubService type+methods, 91-100 installStubOpenAIAPIServiceFactory, 884-1059 tests — verified.
  - `compatibility.go` 71 lines, `compatibility_test.go` 108 lines, `codex-openai-compatibility.json` 13KB — verified.
  - `api.go` 237 lines — verified.
  - Every cited line number is accurate at commit bfebb85.

### A9 — Skipped-item validity
- Probe: Verify each of the three skipped §2 items is genuinely a no-op.
- Result: mitigated
- Evidence:
  - Item 11 (`main/PLAN.md` supersede note): verified — main/PLAN.md is the DROP_0 drop-tree index with zero Track-F / API content; skipping a supersede note is defensible.
  - Item 15 (`internal/config/` listen/APIServe grep): `Grep(pattern="listen|APIServe|api_serve|api\.listen", path=main/internal/config, -i)` returns zero matches. Verified.
  - Item 16 (`magefile.go` changes): `Grep(pattern="openai|openaiapi|valvcompat|ChatCompletions|compatibility|api/openai", path=magefile.go)` returns zero matches. Verified.
  - All three skips are defensible.

### A10 — `blocked_by` overconstraint
- Probe: Can 1.3 and 1.4 actually run in parallel since both depend only on 1.2?
- Result: informational (not a plan break)
- Evidence:
  - 1.3 (`internal/api/openai/*.go` deletion) and 1.4 (`compatibility.go`, `compatibility_test.go`, `codex-openai-compatibility.json` deletion) touch DISJOINT files and DISJOINT packages. No file shared, no package shared, no symbol shared. Neither imports the other. Running them in parallel would not break compile.
  - The planner's stated reason for linearization — "two sibling units must not share a file/package without `blocked_by`" — does NOT apply here because 1.3 and 1.4 share NEITHER a file NOR a package. The linearization is stricter than the stated rule requires.
  - BUT: Valv's Phase 4/5 workflow iterates ONE unit at a time anyway. Parallelism would only matter with concurrent builders, which isn't the workflow. Linear ordering is safe and operationally equivalent.
  - Classification: informational — plan is over-constrained but the extra constraint is harmless under Valv's single-builder-at-a-time Phase 4.

### A11 — Drop-end `mage test` vs `mage integration`
- Probe: Does `mage integration` stay green after each unit?
- Result: mitigated
- Evidence:
  - `mage integration` runs `./internal/cli -tags=integration` only. The only integration test in that package is `codex_integration_test.go`, which is verified API-wrapper-free.
  - `service_integration_test.go` in `openaiapi/` is orphan (not reached by any mage target) pre-1.2 and deleted in 1.2; no `mage integration` impact.
  - Plan 1.2 criterion 4 (`mage integration` exits 0) is defensible.

### A12 — YAGNI on unit boundaries
- Probe: Is 1.5 too large (6 doc files) — should it split?
- Result: informational
- Evidence: 1.5 is conceptually one concern (doc-prose scrub of API-wrapper references). Builder can finish it cleanly as one unit. Acceptance criteria are all greps (yes/no verifiable). Atomic-enough argument holds. Not a blocker, not worth splitting.

## Verdict Summary

Plan passes falsification. Every meaningful attack either mitigates against the plan's existing content (A1, A2, A3, A4, A7, A8, A9, A11, A12) or surfaces informational context that the plan's existing escalation paths already cover (A5 coverage-floor risk is pre-declared by the plan; A10 over-linearization is harmless under Valv's Phase 4 cadence).

**One plan gap worth closing (A6, sub-finding 1):** `main/CLAUDE.md` line 133 contains an `internal/api` reference in the Import DAG paragraph that survives the plan's current 1.5 acceptance criteria. After 1.3 deletes `internal/api/`, this becomes a stale documentation reference. The planner should (a) extend 1.5's `CLAUDE.md` scrub scope to include the line 133 sentence, and (b) strengthen the acceptance criterion from `grep -n "internal/api/" main/CLAUDE.md` returns zero to `grep -n "internal/api\b" main/CLAUDE.md` returns zero. This is a minor, surgical patch — it does not reshape the plan and can be rolled into round 2 or handled as a clarification during Phase 3 discuss.

**Two informational items worth noting (not blockers):** (b) `TOS_COMPLIANCE.md` retraction-context references to "OpenAI-compatible HTTP API" are defensible as historical/retraction documentation; dev may decide whether 1.5 also touches this file. (c) The bare-root `/Users/evanschultz/Documents/Code/hylla/valv/AGENTS.md` line 63 still carries OpenAI-compatible routing language — it's steward-scope and out of DROP_1's main/ scope, but the drift between bare-root and main/ AGENTS.md should be flagged to the steward.

Net verdict: **pass** — no counterexample blocks the plan's decomposition, ordering, or acceptance criteria from proceeding to Phase 3 (Discuss). One plan gap (CLAUDE.md line 133) recommended for patching; two informational items routed to the dev.
