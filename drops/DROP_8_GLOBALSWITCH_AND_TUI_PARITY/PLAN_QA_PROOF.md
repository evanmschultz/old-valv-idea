# PLAN QA PROOF — DROP_8 Round 3

**Verdict:** PASS (with 1 optional SURGICAL recommendation for R4 — non-blocking)

## R2-Finding → R3-Resolution Cross-Walk

| R2 item | Severity | R3 resolution | Source verification | Status |
|---|---|---|---|---|
| **BLOCK CX1** — Unit 8.7 dispatch broken by `service.Status` Codex-hardcoding | block | Unit 8.7 AC line 201 rewritten to use `service.StatusForProvider` two-probe pattern (Claude first, Codex second, both-unbound→Codex fallback). `blocked_by` updated to `[8.1, 8.4]`. | `operator_helpers.go:151` confirms current hardcoded `ProviderCodex`; `manage/service.go:245` confirms `Status()` hardcodes `domain.ProviderCodex` in `BindingByProjectID` call. R3 spec correctly routes around both. | RESOLVED |
| **CONCERN CX2** — `ensureBoundCodexAccountReady` breaks `--account` override | concern | Unit 8.5 retitled "Binding UX parity: `valv codex` merged launch-ready function". `ensureCodexBindingReady` + `ensureBoundCodexAccountReady` merged into `ensureCodexAccountReadyForLaunch(cmd, paths, workingDir, accountOverride, args)`. 5 explicit responsibility steps including `ensureManagedAccountReady` (step 4) and `codexArgsSkipAccountReady` skip (step 5). Unit 8.3 forward-reference updated at line 90. New Notes paragraph (line 226) justifies the Claude/Codex asymmetry. | `codex.go:204-220` confirms `ensureBoundCodexAccountReady` currently calls `ensureManagedAccountReady` at line 217 and `codexArgsSkipAccountReady` check at 205-207. Merged signature preserves both responsibilities. | RESOLVED |
| **CONCERN CX3** — Unit 8.7 `blocked_by` mis-justified | concern | `blocked_by: [8.1, 8.4]` (was `[8.1, 8.5]`). New rationale paragraph at line 225 explains 8.5 removal, with dev-accepted residual-risk acknowledgement (8.5 + 8.7 may run concurrently; corrective `blocked_by: [8.5]` documented as fallback). | Functional dependency on `StatusForProvider` (8.4) is correct per AC line 201. 8.5 has no API surface needed by 8.7 — only shared package boundary. | RESOLVED |
| **SURGICAL S1** — Unit 8.1 AC bullet wording inaccuracy | surgical | AC bullet 2 (line 39) rewritten — `Switch` constructs `TargetPath` at `service.go:106`. Bullet 3 (line 40) describes `prepareTarget` gaining `provider domain.Provider` parameter. | `service.go:102-106` confirms `TargetPath` is constructed in the `Result{}` literal inside `Switch`, not in `prepareTarget`. `service.go:137` is the `backupRoot` line. R3 spec is now source-accurate. | RESOLVED |
| **SURGICAL S2** — Unit 8.5 dead-code cleanup AC | surgical | New AC bullet (line 162) explicitly enumerates deletions: `runCodexFirstRunSetup`, `loginBindAndReportCodexSetup`, `writeCodexSetupIntro`, `writeCodexSetupResult`, + `readPrompt` (conditional on no other callers). | All five symbols verified present in `codex_setup.go` at lines 48, 130, 141, 154, 164 respectively. `readPrompt` is called only inside `runCodexFirstRunSetup` (lines 54, 95) — conditional deletion is sound. | RESOLVED |
| **SURGICAL S3** — `supportedProviders()` presence claim | surgical | Unit 8.3 AC line 96 cites verbatim function signature at `internal/cli/manage.go:1001`. Builder is directed to use the existing helper rather than create one. | Verified at `manage.go:1001-1003` — `func supportedProviders() []domain.Provider { return []domain.Provider{domain.ProviderCodex, domain.ProviderClaude} }` matches exactly. | RESOLVED |

## Verification of Refactor (CX2 Merge)

Per orchestrator brief item B — verify the merged `ensureCodexAccountReadyForLaunch` covers BOTH old functions' responsibilities without losing any:

**Old `ensureCodexBindingReady` responsibilities** (`codex_setup.go:22-46`):
1. Open manage service → covered by merged function (implicit via `openManageService` reuse).
2. Check `service.Status` → covered by step 2.
3. Detect project root → covered (still needed; step 3 `BindProject` requires `workingDir`).
4. TTY check + setup menu → REPLACED by 0/1/2+-account branching in step 3 (deliberate UX simplification per dev decision in Notes line 219).
5. `errCodexSetupCanceled` sentinel → retained per line 151.

**Old `ensureBoundCodexAccountReady` responsibilities** (`codex.go:204-221`):
1. `codexArgsSkipAccountReady` short-circuit → covered by step 5.
2. Open manage service → covered (implicit).
3. `service.Status` → covered by step 2.
4. `ensureManagedAccountReady(cmd, status.Profile.Provider, status.Profile, accountAuthOptions{})` → covered by step 4.

**No responsibility is dropped.** Step 4's "whether from override, existing binding, auto-bind, or picker" enumeration ensures `ensureManagedAccountReady` runs on all four resolution paths.

## Verification of Two-Probe Pattern (Unit 8.7)

Per orchestrator brief item C — what happens if `StatusForProvider(Claude)` returns an error other than `ErrUnboundProject`?

R3 spec at line 201: *"Probe `StatusForProvider(ctx, workingDir, domain.ProviderClaude)` first; if it returns without error, use `domain.ProviderClaude`. Otherwise probe `StatusForProvider(ctx, workingDir, domain.ProviderCodex)`; if it returns without error, use `domain.ProviderCodex`. If both return `ErrUnboundProject`, fall back to `domain.ProviderCodex`."*

Trace cases:
- Claude bound, no error → use Claude.
- Claude `ErrUnboundProject`, Codex bound → fall through to Codex probe, no error → use Codex.
- Both `ErrUnboundProject` → fallback to `ProviderCodex`. Matches current behavior.
- Claude returns non-`ErrUnboundProject` error (e.g., store I/O) → falls through to Codex probe per "otherwise" branch (any non-nil error). Codex probe outcome decides final provider; the Claude-side error is silently dropped.

The last case is a minor logging gap but functionally equivalent to current `ProviderCodex`-hardcoded behavior. Not a blocker — accepted as residual.

## Newly-Discovered R3 Findings

### 3.1 [Axis: spec-conformance] [severity: low] Unit 8.5 step-1 wording ambiguity

**Claim:** Unit 8.5 step 1 reads *"Resolve account: if `accountOverride != ""`, resolve via `service.ProfileByName(...)`; does NOT write a binding row; returns the resolved profile."*

The phrase "returns the resolved profile" is ambiguous when read in isolation — a careless builder could interpret it as "function returns immediately at step 1," which would skip step 4's `ensureManagedAccountReady` call and break the `--account` override + bound-project case for Codex.

Step 4's prose ("whether from override, existing binding, auto-bind, or picker") disambiguates by enumerating override as a valid input to `ensureManagedAccountReady`. The 5-step sequence is coherent when read holistically.

**Evidence:** drops/DROP_8_GLOBALSWITCH_AND_TUI_PARITY/PLAN.md:153 vs 159.

**Recommended fix (optional, non-blocking):** Replace "returns the resolved profile" in step 1 with "stores the resolved profile and proceeds to step 4." Same disambiguation could be applied to steps 2 and 3 ("proceed to step 4 with the resolved profile").

**Severity rationale:** Builder reading all 5 steps as a sequence will catch the intent. The risk is real but recoverable in build-QA. Not worth blocking R3.

## Atomic-Unit Granularity Re-verification

| Unit | Files touched | Single-builder, single-round? |
|---|---|---|
| 8.1 | `globalswitch/service.go`, test | Yes |
| 8.2 | `cli/manage.go`, test | Yes |
| 8.3 | new `cli/account_flag.go` + 2 file edits | Yes |
| 8.4 | new `cli/claude_setup.go`, 3 file edits | Largest unit; introduces `StatusForProvider` + override threading. Still atomic — single PR-sized change. |
| 8.5 | `cli/codex_setup.go` (merge + delete 4 fns), `cli/codex.go`, test | Yes — single package, single refactor target |
| 8.6 | golden test + new fixture | Yes |
| 8.7 | `cli/operator_helpers.go` + test | Yes |

All units are atomic per WORKFLOW.md "atomic granularity" definition.

## `blocked_by` Graph Re-verification

```
8.1 ──┬─────────────────── 8.7
      │                     │
8.2 ─ 8.3 ─ 8.4 ─ 8.5      │
                  │         │
                  └─────────┘
8.6 (independent)
```

- Parallel start: {8.1, 8.2, 8.6}.
- 8.7 unblocks after {8.1, 8.4}.
- 8.5 + 8.7 may run concurrently (both `internal/cli`); dev-accepted per line 225 Notes.

No cycles. No misordered dependencies.

## Constraint Adherence

- **Symmetry across `claude` + `codex`:** Maintained. Single asymmetry (Codex `ensureManagedAccountReady`, Claude in-container OAuth) is justified at line 226.
- **CURRENT command tree:** DROP_8 spec uses `valv manage *` namespace as existing today. DROP_9 fold-in concerns deferred (line 218).
- **`--account` is DROP_8 territory:** Unit 8.3 introduces it; isolated in its own file (`account_flag.go`) for DROP_9 rename hygiene (line 222).
- **No auto-open re-introduction:** No reference to OAuth auto-open machinery anywhere in PLAN.md (line 217 forbids it). Confirmed absent.
- **No Tillsyn:** Confirmed.

## Hylla Feedback

N/A — action item touched markdown + Go files; Hylla MCP backend was specified as unreachable per brief and not attempted. Source reads via direct `Read` tool.

## Summary

R3 cleanly resolves all 6 R2 findings against verified source. The CX2 merge refactor preserves both old functions' responsibilities. The Unit 8.7 two-probe pattern is well-formed for the typical bound/unbound cases. Only one new finding surfaced — a low-severity wording ambiguity in Unit 8.5 step 1 that does not block builder execution.

**Verdict: PASS.**

Optional R4 polish: tighten step-1 wording in Unit 8.5 to remove ambiguity. Not a gate.
