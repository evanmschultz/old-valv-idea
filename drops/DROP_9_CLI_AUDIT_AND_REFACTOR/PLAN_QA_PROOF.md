# PLAN QA Proof Review — DROP_9_CLI_AUDIT_AND_REFACTOR — R3

**Verdict:** PROOF PASS
**Round:** 3
**Reviewer:** go-qa-proof-agent (R3 mechanical-fix verification)
**Scope:** Verify each R2 falsification finding has a corresponding R3 fix in PLAN.md, and no new gaps were introduced.

---

## R2 Findings Cross-Walk

R2 verdicts: Proof PASS-with-one-finding; Falsification FAIL with 3 surgical findings. R3 brief targeted Vec 8 (manage.go grep), Vec 2 (operator_helpers_test.go paths), Vec 12 (Unit 9.5 blocked_by). R3 commit `4717ebc docs(drop-9): planner round 3 revision`; diff is +6/-3 LOC.

### Vec 8 MEDIUM — Unit 9.1 AC must enforce manage.go error-string updates — RESOLVED

**Required fix:** Unit 9.1 AC should include a bullet requiring `git grep "valv manage" -- internal/cli/manage.go` returns zero hits, with concrete substitutions named.

**Evidence in R3 PLAN.md (line 61):**

> 6. All `fmt.Errorf` and other user-facing error/help strings in `manage.go` (and any helpers moved out of it) reference the new normalized command tree. Concrete examples: `valv manage account add codex %s` → `valv account add codex %s` (line 962); `valv manage account list` → `valv account list` (line 962); `valv manage update` → `valv image update` (line 996); `valv manage account add %s %s` → `valv account add %s %s` (line 626). Verify via `git grep "valv manage" -- internal/cli/manage.go` returning zero hits after this unit lands.

**Source-file confirmation via `git grep "valv manage" -- internal/cli/manage.go`:**

- Line 626: `fmt.Errorf("manage account switch: account %q not found ... \`valv manage account add %s %s\` or \`valv manage account list %s\`"...)` — matches "(line 626)" substitution target.
- Line 962: `fmt.Errorf("account %q not found in any provider; run \`valv manage account add codex %s\` or \`valv manage account list\` ..."...)` — matches both "(line 962)" substitution targets.
- Line 996: `fmt.Errorf("no accounts found across any provider; run \`valv manage account add codex <name>\` or \`valv manage account add claude <name>\`...")` — matches "(line 996)" substitution target.

Plus AC #7 (renumbered from R2's AC #6) still enforces `mage testPkg ./internal/cli` passes.

**Verdict:** Vec 8 fixed. AC is concrete, line-number-anchored, and gated by the cited grep.

---

### Vec 2 LOW — operator_helpers_test.go stale comment coverage — RESOLVED

**Required fix:** Unit 9.4.5 paths should include `internal/cli/operator_helpers_test.go`; drop-level paths header should include it; AC #1 grep filter should cover it.

**Evidence in R3 PLAN.md:**

1. **Drop-level Paths header** (line 21):

   > - `internal/cli/operator_helpers_test.go`

   Inserted between `internal/cli/codex_test.go` and `internal/domain/repository.go`. Ordering is consistent with the existing test-file grouping.

2. **Unit 9.4.5 paths** (line 190):

   > - `internal/cli/operator_helpers_test.go`

   Inserted between `internal/cli/extended_test.go` and `magefile.go`.

3. **Unit 9.4.5 AC #1 grep filter** (line 195):

   > `git grep "valv manage" -- internal/cli/claude.go internal/cli/codex.go internal/cli/claude_setup.go internal/cli/codex_setup.go internal/cli/operator_helpers.go internal/cli/claude_setup_test.go internal/cli/codex_setup_test.go internal/cli/codex_test.go internal/cli/extended_test.go internal/cli/operator_helpers_test.go magefile.go README.md` returns zero hits.

   `internal/cli/operator_helpers_test.go` is now in the filter argument list.

**Source-file confirmation (Read `operator_helpers_test.go` lines 145–158):** line 152 contains the comment `// contain "2.2.0" if the dev ran \`valv manage update claude\` and`. The string `"valv manage update claude"` will hit the AC grep and be caught.

**Substitution coverage (Unit 9.4.5 AC #2):** Line 198 explicitly names `valv manage update claude` → `valv image update claude`. The substitution target is concrete.

**Verdict:** Vec 2 fixed. All three required edits landed (drop-level paths, unit 9.4.5 paths, AC grep filter), and the operator_helpers_test.go:152 comment is covered by both the grep gate and an explicit named substitution.

---

### Vec 12 LOW — Unit 9.5 blocked_by chain inconsistency — RESOLVED

**Required fix:** Unit 9.5 `Blocked by` should say `9.4.5` (not `9.1`), matching the chain diagram in Notes.

**Evidence in R3 PLAN.md (line 233):**

> - **Blocked by:** 9.4.5

R2 value was `9.1`; R3 changes it to `9.4.5`.

**Chain consistency check** (Notes section, lines 354–364):

```
9.1 (manage deletion)
  → 9.2 (account bind/unbind — cli + domain + store + service)
  → 9.3 (image namespace — cli)
  → 9.4 (status flatten + --all flag — cli)
  → 9.4.5 (stale string refresh — cli + magefile + README)
  → 9.5 (flag normalization — cli)
  → 9.6 (collision enforcement — cli)
  → 9.7 (CONCERN A tests — cli)
  → 9.8 (CONCERN B hardening — cli)
```

Unit 9.5's `Blocked by: 9.4.5` now matches `9.4.5 → 9.5` in the diagram.

**Transitive-dependency check:** 9.4.5's `Blocked by: 9.1, 9.2, 9.3, 9.4`. So 9.5 → 9.4.5 → 9.1 preserves the "9.5 needs the manage deletion first" semantics — no regression in the actual ordering, just a tighter and more accurate predecessor.

**Tail check:** 9.6 `Blocked by: 9.5`, 9.7 `Blocked by: 9.6`, 9.8 `Blocked by: 9.7` — unchanged. Tail unchanged, chain coherent.

**Verdict:** Vec 12 fixed. Chain consistency restored.

---

## No New Gaps Introduced

- **Diff size:** +6/-3 LOC, all in named target locations (drop-level Paths header, Unit 9.1 AC, Unit 9.4.5 paths + AC #1, Unit 9.5 Blocked by). No collateral edits.
- **AC renumbering (Unit 9.1):** AC #6 (`mage testPkg`) → AC #7. Searched PLAN.md for cross-references to specific AC numbers; none found elsewhere. No drift.
- **Sort/ordering preserved:** Drop-level Paths header keeps source-then-test grouping; Unit 9.4.5 paths preserve internal/cli/ → magefile/README ordering.
- **No regressions** in other units' blocked_by, AC counts, or paths/packages headers.

---

## Premises / Evidence / Trace / Conclusion / Unknowns

**Premises:**
1. R3 must mechanically fix Vec 8, Vec 2, Vec 12.
2. No new gaps may be introduced.

**Evidence:**
- `git diff HEAD~1 -- drops/DROP_9_CLI_AUDIT_AND_REFACTOR/PLAN.md` showed +6/-3 LOC across the four named locations.
- `git grep "valv manage" -- internal/cli/manage.go` confirmed hits at lines 626, 962, 996 (the Unit 9.1 AC #6 substitution targets).
- Read of `internal/cli/operator_helpers_test.go` lines 145–158 confirmed `"valv manage update claude"` at line 152 (Unit 9.4.5 AC target).
- Full re-read of PLAN.md confirmed AC counts, paths headers, blocked_by chain, and Notes-section diagram all consistent.

**Trace:** Per-finding cross-walk above maps each R2 finding → exact PLAN.md line(s) in R3 → source-file confirmation.

**Conclusion:** PROOF PASS. All three R2 falsification findings have concrete, verifiable R3 fixes in PLAN.md. No new gaps detected.

**Unknowns:** None.

---

## Hylla Feedback

N/A — R3 review touched only PLAN.md (markdown) and source-file text (`internal/cli/manage.go`, `internal/cli/operator_helpers_test.go`) via direct Read/grep. No Hylla queries were needed; PLAN.md is not in scope for Hylla (markdown), and source-file text searches were string-literal greps where `git grep` is the correct tool.

---

## TL;DR

PROOF PASS for R3. Vec 8 (Unit 9.1 AC adds manage.go grep + named substitutions at lines 626/962/996), Vec 2 (operator_helpers_test.go added to drop-level paths, Unit 9.4.5 paths, and AC #1 grep filter), Vec 12 (Unit 9.5 blocked_by now 9.4.5) are all fixed. Diff is +6/-3 LOC, no collateral edits, no new gaps.
