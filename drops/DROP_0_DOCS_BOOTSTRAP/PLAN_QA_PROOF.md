# DROP_0 — PLAN QA PROOF (Round 5)

**Verdict:** PASS

## Findings

| ID | Severity | Unit/Item | Evidence | Recommendation |
|---|---|---|---|---|
| — | — | — | No new findings in Round 5. All four proof-QA questions resolved affirmatively; prior-round state preserved. | None. |

## Round 5 Scope Verification

Round 5 is a narrow, one-finding revision. The diff (`git diff HEAD~1 HEAD -- drops/DROP_0_DOCS_BOOTSTRAP/PLAN.md`) shows exactly two hunks, both claimed by item 37:

1. **Line 55 swap** — U0.2 criterion 9 sub-check 2 (Charm imports) changed from `grep -c 'charm.land/' <file>` returns at least `3` to `grep -o 'charm.land/' <file> | wc -l` returns at least `3`. The surrounding rationale parenthetical is expanded to explain the line-count vs occurrence-count distinction and the single-prose-line satisfaction case.
2. **Appended Round 5 revisions block** — new header `Round 5 revisions (applied against dev-accepted plan-QA Round 4 findings …)` plus item 37 documenting the line-55 change, citing accepted finding F1-R4, and explicitly deferring F2 / F3 / F4 per orchestrator direction.

No other line of PLAN.md is modified. Items 1–36 (Rounds 1–4) are preserved byte-for-byte; all six unit headers, the Scope and Notes sections, and all fifteen U0.2 criteria remain in place.

**Question-by-question resolution:**

1. **Item 37 matches the diff.** Item 37 describes precisely the line-55 substitution (old form → new form, floor 3 retained), cites the single-prose-line collapse case that motivated F1-R4, claims BSD/GNU portability, documents the sibling-sub-check re-audit, and lists the deferred F2/F3/F4. The diff shows exactly the line-55 change plus the appended Round 5 block containing item 37. One-to-one correspondence.
2. **POSIX portability confirmed.** `grep -o` is specified by POSIX.1-2017 (Issue 7) and implemented identically on BSD grep (shipped with macOS) and GNU grep. `wc -l` is POSIX.1-2001+. The pipeline `grep -o PATTERN FILE | wc -l` counts match occurrences on both platforms — BSD grep prints each match on its own line just as GNU grep does. Item 37's portability claim is accurate.
3. **Four cases produce the claimed outcomes.** (a) A multi-line table with N ≥ 3 rows each containing one `charm.land/` occurrence: old form returns N (passes), new form returns N (passes) — no regression. (b) A single prose line listing all four `charm.land/*` imports: old form returns 1 (false-fail), new form returns 4 (passes) — the bug F1-R4 identified is fixed. (c) Silent partial deletion leaving only 2 of 4 imports: old form returns 2 (correctly fails), new form returns 2 (correctly fails) — floor still guards against dropping imports. (d) Criterion count: U0.2 still has exactly 15 criteria (verified by inspecting lines 45–70). No renumbering, no additions. All four cases confirm.
4. **Sibling sub-check re-audit is correct.** U0.2 criterion 9 has five sub-checks (lines 54–58): (i) line 54, floor 0 zero-reject for `tiktoken-go/tokenizer|golang.org/x/sync/errgroup` — `grep -c` and `grep -o | wc -l` both agree on `== 0` (any presence fails either way); (ii) line 55, floor 3 Charm — the converted case; (iii) line 56, floor 1 `modernc.org/sqlite` — `grep -c` and `grep -o | wc -l` agree on `>= 1`; (iv) line 57, floor 1 `testcontainers` — same; (v) line 58, floor 1 `modelcontextprotocol/go-sdk` — same. Only sub-check (ii) has floor > 1 where the single-prose-line collapse case is realistic. Item 37's claim that only the Charm sub-check needs conversion is correct.
5. **U0.2 criterion count preserved at 15.** Verified by Grep of the criterion-list header pattern across lines 45–70. Criteria 1–15 all present, in order, with no renumbering.
6. **Items 1–36 intact.** The diff shows only two hunks — line 55 and the appended Round 5 block. No other lines touched. Round 4 items 1–36 are preserved verbatim; their semantic content is unchanged. Item 36's phrasing ("splitting-per-category rationale remains correct") is explicitly reconciled in item 37 via the "Round 4 item 36's description of the post-split Charm sub-check as `grep -c 'charm.land/'` is superseded by this item; the splitting-per-category rationale remains correct — only the counting mechanic for the one floor-3 sub-check changes" sentence.

Deferred findings F2 (U0.2 crit 11(b) polarity window bypass), F3 (U0.2 crit 13(b) negative-heading wordlist incomplete), F4 (U0.6 crit 5 deletion-floor bypass) are explicitly listed as not-applied in Round 5 per orchestrator direction; the audit trail is preserved in the git log of `PLAN_QA_FALSIFICATION.md`. The deferral is explicit, routed, and correctly scoped to F2/F3/F4 (not F1, which was the accepted finding applied here).

## Verdict

PASS. Round 5 is a narrow, correctly scoped, one-finding application: F1-R4 (the Charm sub-check line-count vs occurrence-count bug) is resolved by swapping `grep -c 'charm.land/'` for `grep -o 'charm.land/' | wc -l`, with the floor-3 constraint retained; the new command form is POSIX-portable across BSD and GNU grep; the sibling sub-checks were correctly re-audited and confirmed not to need the same conversion (only the Charm sub-check has floor > 1 with realistic single-line-collapse risk); U0.2 retains exactly 15 criteria; Rounds 1–4 content (items 1–36) is preserved byte-for-byte; and the three deferred findings F2/F3/F4 are explicitly acknowledged and routed to their audit trail. No new blocking, concern, or accepted findings. Ready for Phase 3 discussion or Phase 4 build dispatch.
