# Plan QA Falsification — DROP_8 Round 2

**Verdict:** **FAIL** — 1 BLOCK + 2 CONCERN + 3 SURGICAL

*Note on file authorship:* the `go-qa-falsification-agent` ran its full review and returned this verdict plus every counterexample below. Its Write tool was denied in the agent context, so the orchestrator wrote this file from the agent's returned summary to preserve the WORKFLOW.md Phase 3 audit trail. All evidence anchors below are verbatim from the agent's response.

## R1 Refutation Re-Attack (Verifying R2 Fixes Hold)

- **B1 (Unit 8.1 hardcoded-if-else hack)** — REFUTED with caveat. `TestSwitchClaudeTargetPath` + Codex test continuity boxes the builder in. **Caveat (becomes Surgical S1 below):** AC bullet 1 mis-attributes `TargetPath` construction to `prepareTarget` — actual construction is at `Switch` (`internal/services/globalswitch/service.go:106`); `prepareTarget` (line 126) takes only a `target string`. Wording fix only.
- **C1 (accountOverride signature blast radius)** — REFUTED for explicit signature swap. Verified `runClaudeCommand` (`internal/cli/claude.go:53-121`) and `runCodexCommand` (`internal/cli/codex.go:59`) are the only direct callers. BUT a missed sibling caller surfaces in Counterexample 2 below.
- **C2 (`StatusForProvider` correctness)** — REFUTED. `manage/service.go:231-262` shows `Status` flow; `StatusForProvider` is a clean surgical clone with `provider` parameterized. `manage.Service` is a concrete struct (`manage/service.go:37-43`, value receivers throughout), no interface to update.
- **C3 dev decision (Codex menu removal callsites)** — REFUTED. `runCodexFirstRunSetup` is only called from `codex_setup.go:42`. The 1-account auto-bind path correctly delegates host-side auth to `ensureBoundCodexAccountReady` at `codex.go:78` (which calls `ensureManagedAccountReady` at line 217), so no auth gap — **assuming Counterexample 2 is fixed**.

## Counterexamples (CONFIRMED)

### Counterexample 1 — BLOCK — Unit 8.7 dispatch logic is broken by `Status` codex-hardcoding

**File evidence:** PLAN.md:194 (Unit 8.7 AC); `manage/service.go:245`; `operator_helpers.go:151`.

**Repro trace:** Claude-bound project at `/foo` (binding row exists with `provider=claude`, no Codex binding).

1. TUI `ActionGlobalSwitch` → Unit 8.7 dispatch at `operator_helpers.go:151`.
2. Dispatch calls `service.Status(ctx, workingDir)`.
3. `manage/service.go:245`: `s.store.BindingByProjectID(ctx, projectRecord.ID, domain.ProviderCodex)` returns `ErrNotFound` (no Codex binding).
4. Wrapped as `ErrUnboundProject` (lines 247-249).
5. Unit 8.7 AC fallback: `runGlobalSwitch(cmd, paths, opts, domain.ProviderCodex, "")` — same as today.
6. **Result:** Claude-bound project resolves to `ProviderCodex`. Unit 8.7's entire purpose (provider-aware dispatch for Claude) is broken.

**Remediation:** Unit 8.7 must use `StatusForProvider` from Unit 8.4 (probe Claude then Codex, or add a `BindingByProject` method returning any provider's binding). Update `blocked_by` to `[8.1, 8.4]`.

### Counterexample 2 — CONCERN — `ensureBoundCodexAccountReady` breaks `--account` override path

**File evidence:** Unit 8.5 AC (PLAN.md:146-156); `internal/cli/codex.go:78` and `:213`.

**Repro trace:** `valv codex --account work` in a project that is NOT Codex-bound. Account `work` exists.

1. `runCodexCommand` strips `--account work`, calls `ensureCodexBindingReady` with `accountOverride="work"`.
2. `ensureCodexBindingReady` resolves the profile via `service.ProfileByName`, returns `(profile, nil)` without writing a binding row (Unit 8.5 AC explicit).
3. `runCodexCommand` then calls `ensureBoundCodexAccountReady` at `codex.go:78`.
4. Inside `ensureBoundCodexAccountReady` (`codex.go:213`): `service.Status(cmd.Context(), workingDir)`.
5. No binding row exists → `service.Status` returns `ErrUnboundProject`.
6. Line 214-216 errors out — `valv codex --account work` fails on unbound projects.

**Remediation:** Unit 8.5 AC must update `ensureBoundCodexAccountReady` to accept the resolved profile (or `accountOverride`) and use it directly for `ensureManagedAccountReady`, skipping `service.Status` when the profile is already resolved. Add a test: `valv codex --account work` in an unbound project invokes `ensureManagedAccountReady` against the override profile and proceeds. Claude is unaffected — `runClaudeCommand` has no analog (`claude.go:67-69` comment notes in-container auth).

### Counterexample 3 — CONCERN — Unit 8.7 `blocked_by: [8.1, 8.5]` is mis-justified

**File evidence:** PLAN.md:200, 218.

Unit 8.7 functionally needs Unit 8.4 (`StatusForProvider`, per Counterexample 1) plus Unit 8.1 (Claude branch). It does NOT functionally need Unit 8.5. The "package compilation isolation" rationale at PLAN.md:218 is artificial serialization (the same argument would force every `internal/cli` unit to serialize, which is already implied by the package boundary).

**Remediation:** Change `blocked_by` on Unit 8.7 to `[8.1, 8.4]`. Update the rationale prose in Notes.

## Surgicals (Low Severity)

- **S1 — Unit 8.1 AC bullet 1 wording.** Current text mis-attributes `TargetPath` construction to `prepareTarget`. Rewrite: "`Switch` constructs `result.TargetPath` via a provider switch at the location of `service.go:106`; `prepareTarget` gains a `provider domain.Provider` parameter so its `backupRoot` literal (line 137) becomes `string(provider)`."
- **S2 — Unit 8.5 dead-code cleanup AC.** Add bullet: "Delete `runCodexFirstRunSetup`, `loginBindAndReportCodexSetup`, `writeCodexSetupIntro`, `writeCodexSetupResult`, and any private helpers consumed only by those functions. The 1-account auto-bind path replaces them." Prevents dead-code accumulation.
- **S3 — Unit 8.2 `supportedProviders()` helper presence.** UNKNOWN. Could not verify the helper exists today (no `grep` available in agent context). Suggest AC bullet: "If `supportedProviders()` doesn't already exist in `internal/cli`, add it returning `[]domain.Provider{domain.ProviderCodex, domain.ProviderClaude}` for deterministic iteration."

## REFUTED Vectors (R2 Holds)

- `--` escape hatch semantics — test case explicit, choice documented.
- Symmetry asymmetry 8.4 (`StatusForProvider`) vs 8.5 (`Status`) — justified by R2 note line 217 ("do not refactor `Status` itself").
- Unit 8.4 path coverage for `internal/services/claude/service.go` — listed at PLAN.md:108.
- `StatusForProvider` interface blast radius — `manage.Service` is a concrete struct.

## Hylla Feedback

N/A — Hylla MCP backend unreachable this session. All verification via direct `Read` + `git grep`.

## Summary

**FAIL.** Route to planner for R3. Primary fixes required:

1. Unit 8.7 must use `StatusForProvider` (or a new `BindingByProject`); update `blocked_by` to `[8.1, 8.4]`.
2. Unit 8.5 must explicitly update `ensureBoundCodexAccountReady` to use the resolved profile in the `--account` override path.
3. Unit 8.1 AC bullet 1 wording fix.
4. Unit 8.5 dead-code cleanup AC.
5. Unit 8.2 `supportedProviders()` helper presence check.

R2 successfully addressed R1's BLOCK + 3 of 4 R1 CONCERNs. The remaining gap is the consequence of `service.Status`'s codex-hardcoding propagating into Unit 8.7 (new R2 unit) and into the missed `ensureBoundCodexAccountReady` caller (R2 oversight).

## TL;DR

- T1 Verdict: **fail** (1 BLOCK + 2 CONCERN + 3 SURGICAL).
- T2 BLOCK Counterexample 1: Unit 8.7 inherits `service.Status` Codex-hardcoding → Claude-bound project falls back to Codex. Fix: use `StatusForProvider`, `blocked_by: [8.1, 8.4]`.
- T3 CONCERN Counterexample 2: `ensureBoundCodexAccountReady` at `codex.go:78` breaks `--account` override path. Fix: thread resolved profile through.
- T4 CONCERN Counterexample 3: Unit 8.7's `blocked_by: [8.1, 8.5]` is artificial; should be `[8.1, 8.4]`.
- T5 Three surgical wording/cleanup fixes (Units 8.1, 8.5, 8.2).
