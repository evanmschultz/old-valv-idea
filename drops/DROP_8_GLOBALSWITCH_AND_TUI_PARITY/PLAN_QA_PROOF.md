# Plan QA Proof — DROP_8 Round 2

**Verdict:** **PASS**

R2 addresses all 14 R1 findings (1 BLOCK + 5 CONCERN-class + 7 polish + 2 dev decisions C3/C4) plus introduces a well-formed Unit 8.7. No new R2-introduced gaps detected. The plan is ready for Phase 4 (build).

*Note on file authorship:* the `go-qa-proof-agent` ran its full review and returned this verdict + every cross-walk below. Its Write tool was denied in the agent context, so the orchestrator wrote this file from the agent's returned summary to preserve the WORKFLOW.md Phase 3 audit trail. All evidence anchors below are verbatim from the agent's response.

## R1 → R2 Resolution Cross-Walk

| R1 finding | R2 fix | Status |
|---|---|---|
| BLOCK B1 — Unit 8.1 target-path AC gap (`service.go:106` hardcoded `.codex`) | Explicit `prepareTarget` → `~/.claude` AC + `TestSwitchClaudeTargetPath`. PLAN.md:39. | RESOLVED |
| R1-2.1 — Unit 8.2 name-collision order not procedurally spelled out | 4-step ordered resolution (`--provider` → `ParseProvider` → cross-provider `ProfileByName` iter → ambiguity error). PLAN.md:62-66. | RESOLVED |
| R1-3.1 — Unit 8.4/8.5 `--account` call-graph surgery silent | Signatures changed to `(domain.Profile, error)` + override threading approach (a)/(b) documented. PLAN.md:116, 125, 147, 155. | RESOLVED |
| R1-4.1 — Unit 8.4 wrong fallback alternative ("fall through `service.Status`") | New `manage.Service.StatusForProvider` method introduced; wrong alternative deleted. PLAN.md:119, Notes:217. Six existing `service.Status` callers (codex.go:213, codex_setup.go:29, manage.go:702/828/877/1060) unaffected. | RESOLVED |
| R1-7.1 — Unit 8.1 CLI dispatch wiring scope gap | New **Unit 8.7** — provider-aware dispatch at `operator_helpers.go:151`. PLAN.md:184-200. `blocked_by: 8.1, 8.5` correct (8.1 for Claude switch capability; 8.5 to serialize `internal/cli` package writes). | RESOLVED |
| C3 dev decision — Codex 4-option menu replaced for 0/1-account cases | Explicit at PLAN.md:152-154, 212. Auto-bind + error replace `runCodexFirstRunSetup` for 0/1; picker replaces for 2+. | INTEGRATED |
| C4 dev decision — Add wiring sub-unit to DROP_8 | Unit 8.7 added (same as R1-7.1 above). | INTEGRATED |
| Polish 1 — `--` escape hatch (Unit 8.3 cobra `DisableFlagParsing`) | Documented + test case. | RESOLVED |
| Polish 2 — Shared 0-account error helper | `unboundProjectNoAccountsError` documented in Notes. | RESOLVED |
| Polish 3 — Test injection pattern note | Package-level var pattern documented in Notes (mirrors DROP_7's `hostClaudeAccountAuth`). | RESOLVED |
| Polish 4 — macOS `claude` process-name verification | AC bullet on Unit 8.1 requires `pgrep -l claude` verification + worklog documentation. | RESOLVED |
| Polish 5 — DROP_9 interlock note for `account_flag.go` | Notes line specifies single-file location for stable DROP_9 rename. | RESOLVED |
| Polish 6 — Golden-diff bounding (Unit 8.6) | AC bullet requires Claude golden mirrors Codex format modulo identity fields. | RESOLVED |
| Polish 7 — `BindProject` upsert idempotency | Notes line confirms `store.UpsertProjectBinding` (manage/service.go:223) is already idempotent; builder verifies. | RESOLVED |

## Factual Source-Grounding (Hylla Unavailable This Session)

All eight claims about source files verified by direct `Read` + `git grep`:

1. `internal/services/globalswitch/service.go:106` and `:137` — Codex-hardcoded paths confirmed.
2. `internal/services/manage/service.go:245` — Codex-hardcoded binding lookup confirmed.
3. `Status(ctx, startPath)` signature and `StatusResult` shape confirmed.
4. `ProjectBinding.Provider` field exists at `internal/domain/model.go:32`.
5. `internal/cli/global.go:39-66` confirms `valv global switch <provider>` already parses any provider — Unit 8.7's hands-off claim verified.
6. `internal/cli/operator_helpers.go:151` hardcoded `domain.ProviderCodex` confirmed as Unit 8.7's target.
7. Six existing `service.Status` callers (codex.go:213, codex_setup.go:29, manage.go:702/828/877/1060) unaffected by `StatusForProvider` addition.
8. `store.UpsertProjectBinding` is idempotent (`manage/service.go:223`).

## Constraint Adherence

- No command renames (DROP_9 territory) — clean.
- No auto-open reintroduction — clean.
- No Tillsyn calls — clean.
- No new providers beyond claude+codex — clean.
- Mage targets correct — verified.
- DROP_9 interlock holds — `account_flag.go` isolation + `unboundProjectNoAccountsError` helper give DROP_9 single touchpoints.
- Symmetry 8.4 ↔ 8.5 preserved on signature + return type + override threading. Asymmetry only where runtime structure demands it (Claude in-container auth vs Codex host-side auth + retired 4-option menu).

## Parallelization Graph

```
8.1, 8.2, 8.6  ────────────────────────► parallel (disjoint paths)
8.3 ──► 8.4 ──► 8.5 ──► 8.7              serial on internal/cli/*.go
                  (8.7 also blocked_by 8.1)
```

No cycles. Non-blocking observation: 8.4 → 8.5 could theoretically relax to share `blocked_by: 8.3` (disjoint files post-8.3), but planner's structural reason (build Claude first to establish the override-threading pattern Codex mirrors) is defensible.

## Hylla Feedback

N/A — Hylla MCP backend unreachable this session (known infrastructure outage). All verification via direct `Read` + `git grep`. Not a coverage gap.

## TL;DR

- T1 **Verdict: PASS.** All 14 R1 findings + 2 dev decisions resolved; new Unit 8.7 well-formed.
- T2 8 source-file claims independently verified non-Hylla.
- T3 Constraints clean; symmetry preserved; parallelization graph valid.
- T4 Plan is ready for Phase 4 (build) pending falsification verdict.
