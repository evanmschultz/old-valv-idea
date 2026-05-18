# DROP_8 — PLAN QA FALSIFICATION (Round 3)

**Verdict:** **fail** (1 BLOCK + 1 CONCERN + 2 SURGICAL)

**Round:** 3
**Reviewer:** go-qa-falsification-agent
**Target:** R3 revision of DROP_8 PLAN.md (commit `c6affcc docs(drop-8): planner round 3 revision`)

R2's BLOCK (CX1), CONCERN×2 (CX2, CX3), and 6 brief items were refuted in R3 for most surfaces. Two NEW issues surface from R3's revisions: one BLOCK (Unit 8.5 silently breaks `codex_test.go`) and one CONCERN (Unit 8.7's two-probe error handling is binary instead of `errors.Is(..., ErrUnboundProject)`-discriminating). Plus two surgical items.

---

## R2 Carry-Over Refutation Attempts

### CX1 (R2 BLOCK) — Unit 8.7 two-probe dispatch

**Primary attack: project bound to BOTH Claude and Codex.**
**REFUTED with acceptance.** The schema at `internal/adapters/sqlite/store.go:63` defines `PRIMARY KEY (project_id, provider)` — a project CAN be bound to both providers simultaneously. R3's two-probe dispatch DOES hit Claude first by design ("Probe `StatusForProvider(ctx, workingDir, domain.ProviderClaude)` first; if it returns without error, use `domain.ProviderClaude`"). Claude-wins-when-both-bound is the dispatch policy. R3 surfaces this via the explicit "first … otherwise" wording in Unit 8.7's AC line 201, and the test plan in line 204 covers "bound-to-claude project → ProviderClaude; bound-to-codex project → ProviderCodex". The "both bound" case is implicit but the policy is unambiguous from the AC: Claude-first.

**Surgical (S1):** Add one sentence to Unit 8.7's AC or the Notes section calling out the both-bound case explicitly: *"If the project is bound to BOTH Claude and Codex, the dispatch resolves to Claude (Claude-first probe wins)."* The schema permits it; the AC's two-probe order decides it; document it so the builder doesn't silently pick a different order.

**Secondary attack: error-type discrimination in `StatusForProvider` failure.**
**CONFIRMED (C1).** `BindingByProjectID` returns `ErrNotFound` for missing rows (wrapped to `ErrUnboundProject` by `Status` / `StatusForProvider`), but bubbles up RAW errors for scan / time-parse / DB-connection failures (see `internal/adapters/sqlite/store.go:437-454`). R3's Unit 8.7 AC says literally: *"Probe `StatusForProvider(ctx, workingDir, domain.ProviderClaude)` first; if it returns without error, use `domain.ProviderClaude`. **Otherwise** probe `StatusForProvider(ctx, workingDir, domain.ProviderCodex)`."* The word "otherwise" silently swallows ALL Claude errors — including transient DB errors — and probes Codex.

**Concrete scenario:** Claude is the bound provider, but `StatusForProvider(Claude)` fails with a transient DB read error (e.g., locked DB, transient I/O). The code reads "otherwise" and probes Codex. If Codex returns `ErrUnboundProject` (no Codex binding), the final fallback ("if both return `ErrUnboundProject`") evaluates: the Claude probe didn't return `ErrUnboundProject` — it returned a transient error. So the final condition (`errors.Is(err, ErrUnboundProject)` for BOTH) is false. What does the code do then? The AC doesn't say. Two readings:

1. *"Otherwise probe Codex"* swallows any Claude error including transient → "if both return `ErrUnboundProject`" interpreted as binary fallback → silent Codex dispatch despite Claude-binding being the truth.
2. The AC implicitly distinguishes `errors.Is(err, ErrUnboundProject)` (probe Codex) from "any other error" (return the error). Builder may pick either reading.

Without explicit error-type discrimination in the AC, the builder may implement either. The clean fix: require `errors.Is(err, domain.ErrUnboundProject)` for both the "probe Codex next" branch AND the "fall back to Codex" final branch; any other error type returns immediately.

### CX2 (R2 CONCERN) — `ensureCodexAccountReadyForLaunch` merge correctness

**Attack: does the merged function preserve every responsibility from both old functions?**
**REFUTED.** R3 line 152-164 lays out the 5 numbered responsibilities explicitly:

1. Override resolution (covers `ensureCodexBindingReady`'s override branch).
2. Bound-project check via `service.Status` (covers `ensureCodexBindingReady`'s line 29 + `ensureBoundCodexAccountReady`'s line 213, deduplicated).
3. 0/1/2+ branch (replaces `ensureCodexBindingReady`'s `runCodexFirstRunSetup` call).
4. `ensureManagedAccountReady` call (preserves `ensureBoundCodexAccountReady`'s line 217 responsibility).
5. `codexArgsSkipAccountReady` skip path (preserves `ensureBoundCodexAccountReady`'s line 205-207 skip).

`errCodexSetupCanceled` sentinel is explicitly retained per line 151. The error-category difference between the two old functions (one returned `errCodexSetupCanceled`, the other plain errors) is preserved: only the 4-option menu raised `errCodexSetupCanceled` and that menu is deleted, so the sentinel's only producer goes away — the AC's "retained or renamed; no callers outside this file after the merge" wording covers this. The merged function does NOT need to emit `errCodexSetupCanceled` itself since the menu is gone. **Note S2:** since `errCodexSetupCanceled` has no producer post-merge, the AC should explicitly say "DELETE `errCodexSetupCanceled`" rather than "retained or renamed; no callers outside this file" — leaving a dead sentinel is dead code. Also remove the `errors.Is(err, errCodexSetupCanceled)` check at `codex.go:73` (becomes unreachable).

### CX3 (R2 CONCERN) — `blocked_by` correction for Unit 8.7

**Attack: does removing the 8.5 blocker cause concurrent-write conflicts?**
**REFUTED.** Unit 8.5's paths are `internal/cli/codex_setup.go`, `internal/cli/codex_setup_test.go`, `internal/cli/codex.go`. Unit 8.7's paths are `internal/cli/operator_helpers.go`, `internal/cli/operator_helpers_test.go`. The two file sets are disjoint — no shared file. Concurrent-write safe at the file level. R3's residual note at line 225 ("the cascade dispatcher may run them concurrently … If a concurrent-build failure occurs in practice, add `blocked_by: [8.5]` to Unit 8.7 as a corrective") is correct, and the dev's earlier rejection of the "package-compilation isolation" argument stands — the same logic would serialize every `internal/cli` unit.

Caveat: this REFUTATION is conditional on Unit 8.5 staying within its declared path list. The B1 finding below (Unit 8.5 must also touch `codex_test.go`) breaks that assumption. Once 8.5's paths expand to include `codex_test.go`, the disjoint-file analysis still holds — 8.7 doesn't touch `codex_test.go` either. So 8.5↔8.7 stays parallel-safe after the B1 fix.

---

## R3-Introduced Fresh Findings

### B1 — BLOCK — Unit 8.5 missing `codex_test.go` from path list

**Severity:** BLOCK.

**Evidence:** `git grep -n "ensureBoundCodexAccountReady" -- 'internal/cli/*.go'` reports:
- `internal/cli/codex.go:78` — call site (deleted in 8.5).
- `internal/cli/codex.go:204` — definition (deleted in 8.5).
- `internal/cli/codex_test.go:194` — `if err := ensureBoundCodexAccountReady(cmd, paths, projectRoot, []string{"resume", "--last"}); err != nil { … }`
- `internal/cli/codex_test.go:223` — `if err := ensureBoundCodexAccountReady(cmd, paths, projectRoot, []string{"login"}); err != nil { … }`

**Counterexample:** Unit 8.5 deletes `ensureBoundCodexAccountReady` (line 204 of codex.go) but its declared paths (`internal/cli/codex_setup.go`, `internal/cli/codex_setup_test.go`, `internal/cli/codex.go`) do NOT include `internal/cli/codex_test.go`. The builder will follow the path list, edit only the three declared files, delete the symbol — and `mage testPkg github.com/evanmschultz/valv/internal/cli` will fail with a compile error because `codex_test.go` lines 194 and 223 reference the deleted symbol.

The R3 AC at line 163 says: "consolidate `ensureCodexBindingReady` + `ensureBoundCodexAccountReady` coverage into `ensureCodexAccountReadyForLaunch`'s test suite **in `codex_setup_test.go`**." Migrating coverage to `codex_setup_test.go` is fine, but the OLD tests in `codex_test.go` must also be deleted OR rewritten — and `codex_test.go` is not in the path list, so the builder has no contract authorization to touch it.

**Remediation:** Add `internal/cli/codex_test.go` to Unit 8.5's paths list. Explicit AC bullet: *"Delete or migrate the `ensureBoundCodexAccountReady` test callers at `internal/cli/codex_test.go:194` and `:223`. Old test functions belong in the consolidated `codex_setup_test.go` suite or are removed if redundant with the new override-bound-0-1-2+-already-bound matrix."*

### C1 — CONCERN — Unit 8.7 two-probe error discrimination

**Severity:** CONCERN (described in detail under CX1 secondary above).

**Counterexample:** transient DB error from `StatusForProvider(Claude)` on a Claude-bound project silently dispatches to Codex.

**Remediation:** Tighten Unit 8.7's AC dispatch logic to discriminate error types explicitly. Replace the current "if it returns without error, use Claude. Otherwise probe Codex" with:

> Probe `StatusForProvider(ctx, workingDir, domain.ProviderClaude)`:
> - If it returns no error → use `domain.ProviderClaude`.
> - If `errors.Is(err, domain.ErrUnboundProject)` → probe `StatusForProvider(ctx, workingDir, domain.ProviderCodex)`:
>   - If it returns no error → use `domain.ProviderCodex`.
>   - If `errors.Is(err, domain.ErrUnboundProject)` → fall back to `domain.ProviderCodex` (unbound-project default).
>   - Otherwise → return the error wrapped.
> - Otherwise (Claude probe returned a non-`ErrUnboundProject` error) → return the error wrapped.

Add a test case: Claude `StatusForProvider` returns a synthetic non-`ErrUnboundProject` error → dispatch returns that error, does NOT silently fall back to Codex.

### S1 — SURGICAL — Both-bound dispatch policy documentation

**Severity:** surgical (described in detail under CX1 primary above).

**Remediation:** Add one sentence to Unit 8.7's AC or the Notes section: *"If the project is bound to BOTH Claude and Codex (schema permits via `PRIMARY KEY (project_id, provider)` at `internal/adapters/sqlite/store.go:63`), the dispatch resolves to Claude (Claude-first probe wins)."*

### S2 — SURGICAL — `errCodexSetupCanceled` post-merge fate

**Severity:** surgical.

**Evidence:** R3 line 151 says: *"The old `errCodexSetupCanceled` sentinel in `codex_setup.go:20` is retained or renamed; no callers outside this file after the merge."* But the only producer (`runCodexFirstRunSetup`) is deleted, and the only consumer (`codex.go:73-74`'s `if errors.Is(err, errCodexSetupCanceled) { return nil }`) is also gone once `ensureCodexBindingReady` is replaced by the merged function.

**Counterexample:** "retained or renamed" leaves a dead sentinel — neither produced nor inspected — flagged by `go vet` / `staticcheck` / `golangci-lint` as unused. Even if the linter doesn't catch it, it's dead code in production.

**Remediation:** Change line 151 to explicitly delete the sentinel: *"Delete the `errCodexSetupCanceled` sentinel at `codex_setup.go:20` — its only producer (`runCodexFirstRunSetup`) is removed in this unit, and its only consumer at `codex.go:73-74` is removed when the merged `ensureCodexAccountReadyForLaunch` replaces both old calls."* Also: explicitly note in the caller-change bullet (line 161) that the `errors.Is(err, errCodexSetupCanceled)` branch at codex.go:73-74 is removed.

---

## Other R3-Introduced Vectors — REFUTED

### V1 — `readPrompt` deletion gate

**REFUTED.** `git grep -n readPrompt` confirms exactly 2 callers:
- `internal/cli/codex_setup.go:54` (inside `runCodexFirstRunSetup`)
- `internal/cli/codex_setup.go:95` (inside `runCodexFirstRunSetup`)

Both inside `runCodexFirstRunSetup`, which Unit 8.5 deletes. After deletion, `readPrompt` has zero callers and is safe to delete. AC line 162's wording — *"any private helpers consumed only by those functions (e.g., `readPrompt` if no other callers). … Verify no other callers via source search before deletion"* — is correct and bounded.

### V2 — `StatusForProvider` introduction timing

**REFUTED.** Unit 8.4 AC line 121 explicitly defines `StatusForProvider`: *"a new method added to `manage.Service` that calls `store.BindingByProjectID(ctx, projectRecord.ID, provider)` for the given provider (NOT hardcoded to Codex)."* Unit 8.7 `blocked_by: [8.1, 8.4]` is correct — 8.4 must land first.

### V3 — Multi-bound project schema check

**Covered by S1 above.** Schema permits; documentation gap addressed by the surgical recommendation.

### V4 — `ensureCodexAccountReadyForLaunch` naming asymmetry vs Claude

**REFUTED with acceptance.** Asymmetric naming (`ensureClaudeBindingReady` for Claude vs `ensureCodexAccountReadyForLaunch` for Codex) is justified by R3's Notes line 226: Claude has no host-side `ensureManagedAccountReady` step (auth is in-container), so its function is shorter and the binding-only naming is accurate; Codex has the merged binding + host-auth responsibilities, so the longer name reflects the larger function. Acceptable.

### V5 — Unit 8.5 → Unit 8.7 test-file accidental coupling

**REFUTED.** Confirmed via `git grep -n "ensureCodexAccountReadyForLaunch\|ensureCodexBindingReady\|ensureBoundCodexAccountReady" -- 'internal/cli/*.go'`. No tests in 8.7's files (`operator_helpers.go`, `operator_helpers_test.go`) reference any symbol 8.5 deletes. Concurrent execution of 8.5 and 8.7 is safe at the symbol-reference layer.

### V6 — `globalswitch.Service.Switch` interface check

**REFUTED.** `prepareTarget` is unexported (lowercase). The only exported surface is `globalswitch.New(Options{})` and `Service.Switch(ctx, provider, profileName)` — Unit 8.1's signature change to `prepareTarget` is internal and invisible to consumers. `git grep globalswitch` confirms the only external caller is `internal/cli/operator_helpers.go` calling `globalswitchservice.New(...)`. No external `prepareTarget` consumer.

### V7 — Test coverage of merged function's 5 responsibilities

**REFUTED.** Unit 8.5 AC line 163 enumerates explicit test cases: override (unbound project), override (bound project), no-override + 0 accounts (error), no-override + 1 account (auto-bind + notice + returns profile), no-override + 2+ accounts TTY (picker path), no-override + 2+ accounts non-TTY (error), already-bound short-circuit. Plus assertion that `ensureManagedAccountReady` is called in non-skip paths. All 5 responsibilities covered — override-resolution, bound-check, 0/1/2+ branch, `ensureManagedAccountReady` call, skip-path — with at least one test per responsibility.

### V8 — `BindProject` upsert idempotency for concurrent calls

**REFUTED.** Notes section line 223 already documents this: `service.BindProject` uses `store.UpsertProjectBinding` which is idempotent. Builder verifies in BUILDER_WORKLOG.md.

---

## Summary

R3 successfully refuted R2's BLOCK (CX1 primary), both CONCERNs (CX2, CX3), and most R3-introduced attack vectors. Two genuine new issues surface:

| Item | Severity | Vector |
|---|---|---|
| B1 | BLOCK | Unit 8.5 paths missing `codex_test.go` — compile failure on builder execution |
| C1 | CONCERN | Unit 8.7 two-probe error discrimination silently swallows non-`ErrUnboundProject` errors |
| S1 | surgical | Both-bound dispatch policy (Claude wins) not documented |
| S2 | surgical | `errCodexSetupCanceled` should be explicitly deleted, not "retained or renamed" |

R4 unblocks if:
1. Unit 8.5 paths gain `internal/cli/codex_test.go`, with an explicit AC bullet covering the migration/deletion of the test callers at lines 194 and 223.
2. Unit 8.7's dispatch AC uses `errors.Is(err, domain.ErrUnboundProject)` to discriminate the fall-through branches from raw-error returns.
3. S1 + S2 applied (small AC edits).

No other findings. Recommend route back to planner for R4 revision.
