# PLAN QA PROOF — DROP_9 (Round 4)

**Drop:** DROP_9_CLI_AUDIT_AND_REFACTOR
**Round:** 4
**Reviewer role:** plan-qa-proof
**Target reviewed:** `drops/DROP_9_CLI_AUDIT_AND_REFACTOR/PLAN.md` after commit `361c330` (`docs(drop-9): planner round 4 revision`)

---

## Verdict

**PASS-with-finding.**

The R4 revision cleanly closes both R3 falsification findings (F1 load-bearing regex; F2 manage.go coverage gap). However the R4 edit introduced a *new* cross-AC contradiction between Unit 9.1 AC #6 and Unit 9.4.5 AC #1b that prevents the plan from being internally consistent as written.

This is one surgical finding, mechanical to fix. R5 should be small.

---

## R3 Finding Resolution Cross-Walk

### F1 (load-bearing) — AC #1 grep false-pass

| Check | Status | Evidence |
|---|---|---|
| New AC #1c regex added to 9.4.5 | PASS | PLAN.md lines 219–223 |
| Regex catches bare-`manage` form in `magefile.go` + `README.md` | PASS | `git grep -E '"manage [a-z]+\|manage [a-z]+"' -- magefile.go README.md` → 4 hits (README.md ×3 + magefile.go ×1) |
| AC #1c body cites exact lines (README.md 36, 44, 45; magefile.go 682) | PASS | PLAN.md line 223 |
| No bare-`manage` form escapes the regex (single-quote / no-quote variants) | PASS | Broader sweep `git grep "manage" -- magefile.go README.md` returns only the 4 caught hits + non-actionable prose ("Valv-managed", "./internal/tui/manage" Go package path) |

**F1 resolved.**

### F2 (coverage gap) — manage.go Example block sweep

| Check | Status | Evidence |
|---|---|---|
| `internal/cli/manage.go` added to 9.4.5 Paths | PASS | PLAN.md line 186 |
| New AC #1b requires `git grep "valv manage" -- internal/cli/manage.go` → zero hits | PASS | PLAN.md lines 213–217 |
| AC #1b body documents the 68-hit baseline | PASS | PLAN.md line 217 ("manage.go contains 68 `valv manage` occurrences") |
| Live baseline count matches | PASS | `git grep -c "valv manage" -- internal/cli/manage.go` → 68 |
| OUT-OF-SCOPE language removed | PASS | `git grep "OUT OF SCOPE" -- drops/DROP_9_CLI_AUDIT_AND_REFACTOR/PLAN.md` returns empty (old line 6 deleted in R4 diff) |
| Ownership-boundary doc inverted to put Example blocks IN-SCOPE for 9.4.5 | PASS | PLAN.md lines 243–245 ("Unit 9.4.5 owns the sweep of cobra `Example:` string literals in ALL surviving manage.go constructors") |
| Ownership split between 9.1 and 9.4.5 explicit | PASS | PLAN.md lines 243–245 |

**F2 resolved.**

---

## Verification Checklist (from R4 brief)

| Item | Result |
|---|---|
| A. PLAN.md readable end-to-end after R4 | PASS |
| B. R3 findings each map to a specific R4 AC bullet | PASS (F1 → AC #1c; F2 → AC #1b + paths) |
| C. F1 regex returns 4 hits today | PASS (README.md ×3 + magefile.go ×1) |
| D. F2 grep returns ~68 hits today | PASS (exactly 68) |
| E. 9.4.5 paths include `internal/cli/manage.go` | PASS (line 186) |
| F. 9.4.5 OUT-OF-SCOPE language removed/inverted | PASS |
| G. Ownership-boundary doc separates 9.1 from 9.4.5 | PASS (lines 243–245) |
| H. AC count change (7 → 6) internally consistent, no orphan refs | PASS (six top-level AC items; no orphan numbers) |

All eight R4-brief checks land green.

---

## New R4 Findings

### Finding R4-P1 — [Axis: spec-conformance] [severity: high] Unit 9.1 AC #6 grep-gate contradicts Unit 9.4.5 ownership split

**Claim:** Unit 9.1's acceptance gate is unachievable as written. It demands that `internal/cli/manage.go` contain zero `valv manage` strings the moment 9.1 closes, but R4 explicitly transfers the ~16 surviving constructor `Example:` block sweep to Unit 9.4.5 (which runs AFTER 9.1 per the blocker chain).

**Evidence pointer:** PLAN.md

- Unit 9.1 AC #6 (line 61): *"All `fmt.Errorf` and other user-facing error/help strings in `manage.go` (and any helpers moved out of it) reference the new normalized command tree. … Verify via `git grep \"valv manage\" -- internal/cli/manage.go` returning zero hits after this unit lands."*
- Unit 9.4.5 AC #1b (lines 213–217): manage.go must reach zero hits **after** 9.4.5 lands; explicitly states ~16 Example-block strings SURVIVE 9.1.
- Unit 9.4.5 Design notes (lines 243–245): *"unit 9.1 owns … (b) the 3 `fmt.Errorf` runtime strings at manage.go:626/962/996 … Unit 9.4.5 owns the sweep of cobra `Example:` string literals in ALL surviving manage.go constructors."*
- Blocker chain: 9.4.5 is `Blocked by: 9.1, 9.2, 9.3, 9.4` (line 237) — 9.1 closes FIRST.

**Concrete contradiction:** If 9.1 enforces `git grep "valv manage" -- internal/cli/manage.go` → 0 hits, 9.1 cannot close without also doing 9.4.5's ~16 Example-block sweep. But R4's design notes explicitly assign that sweep to 9.4.5. Result: either 9.1 cannot close, or 9.1 silently absorbs 9.4.5's work and 9.4.5's AC #1b becomes trivially satisfied at zero.

**Fix hint:** Narrow Unit 9.1 AC #6's grep gate to the work 9.1 owns. Recommended replacement (lines 626/962/996 are the fmt.Errorf sites — the only manage.go strings 9.1 is responsible for):

```
Verify via `git grep -n "valv manage" -- internal/cli/manage.go` that the 3 fmt.Errorf strings at lines 626/962/996 no longer reference the old command tree (each rewritten per the substitutions above). Surviving `Example:` field hits are out of scope for 9.1 and are swept in 9.4.5.
```

Alternatively, the gate can be deleted from 9.1 entirely and 9.4.5's AC #1b becomes the sole manage.go zero-hits gate. Both options preserve the R4 ownership split.

**Severity:** High because if the builder takes 9.1 AC #6 literally, they will either (a) refuse to close 9.1 until they do 9.4.5's work too — wasting the unit boundary — or (b) interpret the gate loosely and risk a real coverage miss in 9.4.5.

---

## Summary

R4 cleanly fixes both R3 falsification findings. The single new finding above is a surgical R4 regression introduced by the same edit that inverted ownership of manage.go Example blocks: the planner inverted the 9.4.5 side but forgot to narrow the matching gate on 9.1.

R5 should be a small targeted edit on PLAN.md line 61 to scope 9.1 AC #6's grep to the work 9.1 actually owns.

## TL;DR

- T1 PASS-with-finding — R4 closes both R3 findings (F1 regex + F2 manage.go sweep) cleanly; live greps confirm gates fire today (4 hits / 68 hits).
- T2 ONE surgical R4 regression: Unit 9.1 AC #6 demands manage.go has zero `valv manage` hits after 9.1 closes, but R4 transferred ~16 Example-block sweeps to 9.4.5 (which runs AFTER 9.1). Narrow 9.1's gate to the 3 fmt.Errorf strings it owns.
