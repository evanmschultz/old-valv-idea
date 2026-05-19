# Plan QA Proof — DROP_9_CLI_AUDIT_AND_REFACTOR

**Round:** 6
**Reviewer:** go-qa-proof-agent (subagent)
**Verdict:** PASS

## R5 Finding Cross-Walk

R5 PROOF FAIL flagged exactly one surgical issue (dev sided with Proof). R6 lands a +1/-1 edit on Unit 9.1 AC #6 only. Cross-walk:

| R5 Finding | R6 Fix | Verified |
|---|---|---|
| Unit 9.1 AC #6 grep gate `git grep "valv manage" -- internal/cli/manage.go \| grep -v 'Example:'` was unfulfillable mid-9.1 (surviving cobra `Example:` fields in the constructors not in 9.1's scope would still match the grep; 9.4.5 sweeps them later) | Grep gate REMOVED. Replaced with prose: "after this unit lands, the 3 `fmt.Errorf` runtime strings at `internal/cli/manage.go:626`, `:962`, `:996` are updated to reference the new normalized command tree. Builder verifies each line via direct `Read` post-edit and cites the post-edit text in `BUILDER_WORKLOG.md`." | PLAN.md line 61 (diff +1/-1) |

## Verification Checks (A–E from Appendix)

| Check | Evidence | Result |
|---|---|---|
| A. Read drop's PLAN.md Unit 9.1 AC #6 | PLAN.md line 61 read | done |
| B. Grep gate gone (`git grep ... \| grep -v 'Example:'`) | Searched line 61 — no `git grep` token present | PASS |
| C. New AC requires Read-based verification of 3 lines + post-edit text in BUILDER_WORKLOG.md | Line 61 exact phrase: "Builder verifies each line via direct `Read` post-edit and cites the post-edit text in `BUILDER_WORKLOG.md`." | PASS |
| D. 9.4.5 AC #1b unchanged (global zero-hits gate, fulfillable after full chain) | PLAN.md line 215: `git grep "valv manage" -- internal/cli/manage.go` zero-hits gate intact; "Blocked by: 9.1, 9.2, 9.3, 9.4" preserved at line 237 | PASS |
| E. Substitution examples in AC #6 opening sentence preserved | Line 61 contains all four: `valv manage account add codex %s` → `valv account add codex %s` (line 962); `valv manage account list` → `valv account list` (line 962); `valv manage update` → `valv image update` (line 996); `valv manage account add %s %s` → `valv account add %s %s` (line 626) | PASS |

## Live-Source Cross-Check (lines 626 / 962 / 996)

Read `internal/cli/manage.go` at the cited line numbers to confirm AC #6 anchors on real targets:

- Line 626: `return fmt.Errorf("manage account switch: account %q not found for provider %q; run \`valv manage account add %s %s\` or \`valv manage account list %s\`", ...)` — contains `valv manage account add %s %s` (AC #6 substitution example #4) plus a secondary `valv manage account list %s` form. Both are stale; the general "updated to reference the new normalized command tree" wording covers both.
- Line 962: `return "", "", fmt.Errorf("account %q not found in any provider; run \`valv manage account add codex %s\` or \`valv manage account list\` to see all available accounts", ...)` — contains both `valv manage account add codex %s` (AC #6 example #1) and `valv manage account list` (AC #6 example #2). Two substitutions on one source line — AC #6's "3 fmt.Errorf runtime strings" + 4 substitution examples is internally consistent.
- Line 996: `return "", "", fmt.Errorf("no accounts found across any provider; run \`valv manage account add codex <name>\` or \`valv manage account add claude <name>\` to create one")` — contains `valv manage account add codex <name>` and `valv manage account add claude <name>`. AC #6 explicitly lists `valv manage update` → `valv image update` for line 996, but the actual line 996 text is two `valv manage account add` forms, NOT `valv manage update`. **See Findings §1.1.**

## 1. Findings

- 1.1 [Axis: spec-conformance] [severity: low] Unit 9.1 AC #6 substitution example "`valv manage update` → `valv image update` (line 996)" does not match the live line 996 text (which is two `valv manage account add` forms, not `valv manage update`) → `internal/cli/manage.go:996` reads `"no accounts found across any provider; run \`valv manage account add codex <name>\` or \`valv manage account add claude <name>\` to create one"` → not a blocker for R6 because the AC's general phrase "updated to reference the new normalized command tree" + the BUILDER_WORKLOG cite-post-edit-text requirement still forces a correct fix, but the example pairing is mis-labeled. Builder reading AC #6 will still find and fix the line 996 string semantically. **Severity low** — verbal accuracy issue in the line/example mapping, not a gate failure. Could be cleaned up in a future revision but does not block R6.

## 2. Missing Evidence

None. All five appendix checks (A–E) have direct evidence; R5 finding is resolved.

## 3. Summary

**PASS.**

R6 cleanly resolves the R5 PROOF finding with a minimal +1/-1 edit. The grep gate is gone; Read-based verification of lines 626/962/996 + post-edit text citation in BUILDER_WORKLOG.md is fulfillable mid-Unit 9.1; 9.4.5 AC #1b remains the global zero-hits gate after the full 9.1–9.4 chain. Substitution examples preserved. One low-severity verbal-accuracy nit on the line 996 example pairing (logged in §1.1) is not a blocker.

## TL;DR

- T1: Low-severity verbal mismatch on AC #6 line 996 example pairing — not a blocker, builder will still fix correctly via the Read-and-cite workflow.
- T2: No missing evidence.
- T3: PASS — R6 resolves the R5 finding cleanly with a +1/-1 surgical edit.
