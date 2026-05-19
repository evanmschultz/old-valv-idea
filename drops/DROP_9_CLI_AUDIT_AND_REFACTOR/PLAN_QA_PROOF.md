# PLAN QA PROOF — DROP_9 Round 5

**Drop:** DROP_9_CLI_AUDIT_AND_REFACTOR
**Round:** 5
**Mode:** Proof (R4 single-finding verification)
**R4 finding being verified:** Unit 9.1 AC #6 grep gate ownership contradiction — should now exclude cobra `Example:` fields and carry an explicit handoff to 9.4.5 AC #1b.

## 1. Verdict

**FAIL — 1 NEW finding** (different from the R4 finding; that one IS resolved).

R5's two-part fix split cleanly:
- Handoff note added to 9.1 AC #6 → ownership contradiction **resolved**.
- Grep command changed to `| grep -v 'Example:'` → introduces a **new defect**: the filter does not implement what the prose claims.

Net: R4's finding is resolved, but the R5 patch creates a new, surgical, planner-fixable defect. Route to planner for R6.

## 2. R4 Finding Cross-Walk

### 2.1 R4 finding — RESOLVED

**Claim:** AC #6 grep gate must explicitly exclude cobra `Example:` fields; handoff to 9.4.5 AC #1b must be visible.

**Evidence:**

- `git diff HEAD~1 -- drops/DROP_9_CLI_AUDIT_AND_REFACTOR/PLAN.md` shows the +1/-1 change at line 61.
- `drops/DROP_9_CLI_AUDIT_AND_REFACTOR/PLAN.md:61` (new R5 text):

  > Verify via `git grep -n "valv manage" -- internal/cli/manage.go | grep -v 'Example:'` returning zero hits after this unit lands. Note: cobra `Example:` field strings in surviving constructors are NOT in this unit's scope — they are swept by Unit 9.4.5 AC #1b.

- `drops/DROP_9_CLI_AUDIT_AND_REFACTOR/PLAN.md:213-217` (9.4.5 AC #1b unchanged):

  > **AC #1b — manage.go (including surviving constructor Example blocks):**
  > ```
  > git grep "valv manage" -- internal/cli/manage.go
  > ```
  > Must return **zero hits**. … This unit sweeps the remaining stale strings in surviving cobra constructor `Example:` fields — the ~16 account-* and image-* constructors whose Example blocks still reference the old `valv manage ...` form.

The ownership split is now explicit on both sides (9.1 carves the Example: blocks out and points to 9.4.5; 9.4.5 still owns the final zero-hits gate). Verdict: RESOLVED.

## 3. New Findings (introduced by R5)

### 3.1 [Axis: specify-block-well-formedness] [severity: high] AC #6 grep filter does not exclude Example-body lines — gate's "zero hits" is unreachable post-9.1

**Claim:** The new grep gate `git grep -n "valv manage" -- internal/cli/manage.go | grep -v 'Example:'` does NOT filter out the cobra Example-block body lines the handoff note carves out of 9.1's scope. After 9.1 lands, the command will still return ~60 hits, contradicting the "returning zero hits" prose.

**Evidence pointer:**

- Live evidence from `internal/cli/manage.go` HEAD:
  - 68 total `valv manage` hits (per `git grep -n "valv manage" -- internal/cli/manage.go | wc -l`).
  - 17 cobra `Example: strings.TrimSpace(` field-declaration lines (per `git grep -nE "^[[:space:]]+Examp" -- internal/cli/manage.go`): lines 34, 70, 112, 137, 161, 202, 237, 279, 300, 321, 347, 383, 407, 435, 1179, 1232, 1341.
  - NONE of those 17 field-declaration lines contain the substring `valv manage` (the body of each backtick literal is on the FOLLOWING lines, e.g. 35–41 for the 34-anchored block).
  - The body lines (35–41, 71–82, 113–116, etc.) contain `valv manage` but do NOT contain the literal substring `Example:`.
- Consequence: `git grep "valv manage" … | grep -v 'Example:'` removes zero lines from the upstream `git grep` output, because no line in that output contains the literal text `Example:`.
- After 9.1: the 3 fmt.Errorf strings at lines 626/962/996 get rewritten and the `newManageCommand` block (lines ~22–46) is deleted, eliminating the 7 lines in that Example block. That leaves ~58 surviving hits in the 16 surviving constructor Example blocks plus the Long: docstring body lines (e.g. lines 187–188). All of them pass the `grep -v 'Example:'` filter unchanged.
- The gate's stated success condition ("returning zero hits") is therefore unreachable inside 9.1's scope.

**Fix hint (planner R6 — pick one):**

1. Drop the in-line grep gate from 9.1 AC #6 entirely. Let 9.4.5 AC #1b be the sole zero-hits gate for manage.go. Reword 9.1 AC #6 to specify the three concrete substitutions at lines 626/962/996 and stop at "verify those three lines now reference the new tree."
2. Replace the gate with a per-line-number check, e.g.: `git grep -nE "valv manage" -- internal/cli/manage.go | awk -F: '$2 == 626 || $2 == 962 || $2 == 996'` must return zero hits.
3. Replace the filter with one that actually excludes backtick-body lines — but this requires line-range / state-aware filtering (awk with `BEGIN{inblock=0}` toggling on backticks) that is heavyweight for an AC verification gate. Option 1 is simpler.

**Why this is finding-grade:** AC items are mechanical pass/fail gates checked by build-QA. A QA subagent running the literal command will see ~60 hits, NOT zero, and either (a) mark 9.1 not-done (false-negative blocking close-out) or (b) ignore the gate and accept the handoff note as authoritative (gate becomes ceremonial and the AC drift compounds). Both outcomes degrade the plan's verifiability.

## 4. Summary

| Item | Status |
|---|---|
| R4 ownership contradiction | RESOLVED (handoff note present at line 61, downstream gate at 9.4.5 AC #1b intact) |
| R5 grep semantics | DEFECTIVE (filter does not exclude what prose carves out) |
| Overall verdict | FAIL — route to planner R6 |

Single-line fix in R6 should land this. Likely the cleanest is option 1 (drop the gate from 9.1, let 9.4.5 own zero-hits exclusively).

## 5. Hylla Feedback

N/A — round-5 review touched only markdown (PLAN.md) and Go file `git grep` line-range evidence. Hylla is Go-only and was not the right surface for this verification.
