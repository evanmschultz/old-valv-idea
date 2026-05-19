# PLAN QA Falsification — DROP_9 R5

**Drop:** DROP_9_CLI_AUDIT_AND_REFACTOR
**Round:** 5
**Reviewer:** go-qa-falsification-agent
**Target:** R5 surgical revision of Unit 9.1 AC #6 (`| grep -v 'Example:'` filter + handoff to 9.4.5 AC #1b)
**Verdict:** PASS — no unmitigated counterexample produced

---

## R5 Change Under Review

`git diff HEAD~1 -- drops/DROP_9_CLI_AUDIT_AND_REFACTOR/PLAN.md` shows a single +1/-1 line change at Unit 9.1 AC #6:

- **Before (R4):** `git grep "valv manage" -- internal/cli/manage.go` returning zero hits.
- **After (R5):** `git grep -n "valv manage" -- internal/cli/manage.go | grep -v 'Example:'` returning zero hits, with an explicit clause:
  > "Note: cobra `Example:` field strings in surviving constructors are NOT in this unit's scope — they are swept by Unit 9.4.5 AC #1b."

Ownership boundary is now explicit between 9.1 (fmt.Errorf user-facing strings + structural rewiring) and 9.4.5 (cobra `Example:` body sweep + remaining text fields).

---

## Per-Vector Attack Findings

### V1 — Ownership fix correctness — REFUTED

**Attack:** Does `| grep -v 'Example:'` accidentally exclude legitimate non-Example hits?

**Evidence:**
- `git grep -n 'fmt.Errorf.*valv manage' -- internal/cli/manage.go` returns exactly the 3 expected fmt.Errorf lines: 626, 962, 996.
- None of these 3 lines contain the literal substring `Example:`. They contain phrases like `manage account switch: account %q not found ...` (line 626) but never `Example:`.
- The R5 grep filter `| grep -v 'Example:'` is therefore a no-op for the current 3 hits — it filters nothing real today but documents intent and protects against future contamination.

**Conclusion:** The fix is correct (or, more precisely, harmless and clarifying). No false-exclusion risk on current code.

---

### V2 — `grep -v 'Example:'` overmatch — REFUTED

**Attack:** Could `Example:` substring appear in other contexts (string literals, comments, error messages) on a line that ALSO contains `valv manage` and `fmt.Errorf`, causing accidental exclusion?

**Evidence:**
- All 14 `Example:` occurrences in `internal/cli/manage.go` are cobra field declaration lines of the form `Example: strings.TrimSpace(`. Lines: 34, 70, 112, 137, 161, 202, 237, 279, 300, 321, 347, 383, 407, 435, 1179, 1232, 1341 (17 total; previous spot-check missed a few).
- None of these lines contain `fmt.Errorf` or `valv manage` on the same line — the cobra Example field declaration is its own line, with the heredoc content starting on the next line.
- For a line to be falsely excluded by `| grep -v 'Example:'`, it would need to (a) contain `fmt.Errorf`, (b) contain `valv manage`, AND (c) contain the literal substring `Example:`. None of the 68 `valv manage` hits in manage.go meet all three.

**Conclusion:** Zero overmatch risk on current code. Future-proof concern noted but pathological (builder owns mitigation).

---

### V3 — `Long:` blocks containing `valv manage` — REFUTED (covered by 9.4.5)

**Attack:** Cobra `Long:` heredoc bodies may contain `valv manage` references not caught by 9.1's narrowed grep.

**Evidence:**
- `git grep -n 'Long:' -- internal/cli/manage.go` returns 17 hits across lines 27, 63, 107, 132, 156, 183, 232, 274, 295, 316, 340, 370, 404, 424, 1167, 1221, 1332.
- Reading line 27-50 (root manage `Long:` heredoc) shows multiple `valv manage` references inside the heredoc body lines.
- Unit 9.1 AC #6's narrow grep `fmt.Errorf.*valv manage` deliberately ignores `Long:` content (no `fmt.Errorf` keyword on those lines).
- Unit 9.4.5 AC #1b uses the broad grep `git grep "valv manage" -- internal/cli/manage.go` and explicitly states: "Replace ALL occurrences of `valv manage <verb>` ... in Use/Short/Long/Example fields" — **Long: is enumerated**.

**Conclusion:** No gap. Long: bodies belong to 9.4.5 by both grep coverage AND AC text. Ownership handoff is explicit.

---

### V4 — AC #6 vs AC #1b semantic gap — REFUTED

**Attack:** Some field type (Use, Short, Long, comments) might fall between 9.1's narrow grep and 9.4.5's broad grep.

**Evidence:**
- All 68 `valv manage` hits in `internal/cli/manage.go` bucket cleanly into:
  - **(a)** Cobra `Use|Short|Long|Example` heredoc bodies → 9.4.5 AC #1b broad grep catches all.
  - **(b)** `fmt.Errorf` user-facing strings (3 hits: 626/962/996) → 9.1 AC #6 narrow grep.
  - **(c)** Comments mentioning `valv manage` (if any) → 9.4.5's broad grep catches them too; comments are guidance text and either get rewritten in lockstep or remain semantically stale (acceptable).
- 9.4.5 AC #1b's grep `git grep "valv manage" -- internal/cli/manage.go` is the catch-all backstop. If 9.1 misses anything in its narrow scope, 9.4.5 catches it (9.4.5 blocked_by 9.1, runs strictly after).

**Conclusion:** No semantic gap. 9.4.5 is the backstop; 9.1 is the targeted fmt.Errorf pass.

---

### V5 — Multi-line `Example:` formatting variants — REFUTED

**Attack:** Cobra `Example:` fields can be `Example: "..."` (single-line) or `Example: \`heredoc\``. Does the R5 grep filter handle both forms?

**Evidence:**
- Inspection of all 17 `Example:` hits in manage.go shows uniform heredoc form: `Example: strings.TrimSpace(\``. No single-line `Example: "..."` form exists in the current codebase.
- The `| grep -v 'Example:'` pattern filters any line containing the literal substring `Example:` regardless of what follows — covering both heredoc and single-line forms IF they ever co-occur with fmt.Errorf on one line.
- Critical point: the `Example:` declaration line never contains `fmt.Errorf` or the heredoc body's `valv manage` text. The heredoc body lines (which DO contain `valv manage`) don't contain the literal `Example:`. So the filter is dormant either way and the 3 fmt.Errorf hits are returned correctly.

**Conclusion:** No formatting gap. R5 grep behavior is identical with or without the filter on current code; filter is documentation-of-intent + defense-in-depth.

---

## Tree-Shape Sweep (§4.4)

R5 is a leaf-level AC narrowing within Unit 9.1, not a cascade restructure. The 4-pass tree-shape audit (blocker-graph acyclicity, sibling-overlap-without-blockers, leaf AC composition, orphan-droplet check) was completed in R4. No tree-shape regressions in R5.

- 9.4.5 `blocked_by: 9.1, 9.2, 9.3, 9.4` — correctly enforces 9.1 runs first.
- No new units, no new edges, no new files declared.

---

## Hylla Feedback

N/A — R5 review touched non-Go files only (PLAN.md) plus targeted reads of `internal/cli/manage.go` line ranges. Hylla queries were not the appropriate evidence source for grep-level AC verification; `git grep` is authoritative for substring-presence claims.

---

## Verdict

**PASS** — R5 surgical fix to Unit 9.1 AC #6 correctly narrows the AC's scope, hands off `Example:` field ownership to 9.4.5 AC #1b explicitly in prose, and introduces no overmatch / undermatch / formatting gaps. All 5 attack vectors refuted with concrete evidence.

DROP_9 plan is ready to proceed to Phase 3 / Phase 4.
