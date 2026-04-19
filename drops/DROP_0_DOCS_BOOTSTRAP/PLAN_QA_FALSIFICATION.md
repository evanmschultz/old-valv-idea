# DROP_0 — PLAN QA FALSIFICATION (Round 6)

**Verdict:** PASS

Narrow Round-6 review. The single new delta under attack is the Charm sub-check at line 55 of `/Users/evanschultz/Documents/Code/hylla/valv/main/drops/DROP_0_DOCS_BOOTSTRAP/PLAN.md`:

```
grep -oE 'charm\.land/[a-z]+/v2' /Users/evanschultz/Documents/Code/hylla/valv/main/CLAUDE.md | sort -u | wc -l returns at least 3
```

Plus item 38 at lines 221–223 documenting the Round-6 revision. Deferred Round-4 findings F2/F3/F4 remain out of scope per orchestrator direction.

## Findings

| ID | Severity | Unit/Item | Counterexample | Mitigation |
|---|---|---|---|---|
| F1-R6 | low | U0.2 crit 9 Charm sub-check (line 55) | A prose line like `See https://charm.land/docs/v2 and https://charm.land/blog/v2 and https://charm.land/tutorials/v2 for docs.` produces `grep -oE 'charm\.land/[a-z]+/v2'` output of three distinct strings, `sort -u \| wc -l` = 3, passes the floor with **zero** real Charm imports. Empirically verified via `/usr/bin/grep -oE 'charm\.land/[a-z]+/v2' /tmp/r6_t2.md \| /usr/bin/sort -u \| /usr/bin/wc -l` → `3`. | Accept as low-severity. The prose-URL shape requires three implausible URLs (`/docs/v2`, `/blog/v2`, `/tutorials/v2`) in the Tech Stack section. Charm's module paths ARE `charm.land/<pkg>/v2` (the domain is the module namespace), so a builder citing the module page URL would *naturally* name real distinct imports — making the bypass essentially self-defeating. A stricter regex anchoring to known Charm package names (`charm\.land/(fang\|bubbletea\|lipgloss\|bubbles)/v2`) would close the gap, but would make the check brittle against legitimate future Charm packages and is rejected on YAGNI grounds. The Phase 3 / Phase 5 human review catches contrived prose URLs trivially. |
| F2-R6 | informational | U0.2 crit 9 Charm sub-check (line 55) | Text of item 38 promises "three *distinct* v2 imports"; the mechanical check only asserts "three distinct shape-matches" — no guarantee they are actual imports. Overlaps with F1-R6. | Accept. Documentation drift between prose intent and mechanical floor is bounded by F1-R6's analysis; no separate blocker. |

No blocking findings. Verdict PASS.

## Attacks Attempted And Refuted

1. **Shape-regex bypass (prose URLs):** CONFIRMED as a technical counterexample but mitigated as low-severity (see F1-R6). A `charm.land/docs/v2` / `charm.land/blog/v2` / `charm.land/tutorials/v2` triple in prose satisfies the floor with zero real imports. Realism is low because (a) the Tech Stack section lists deps, not marketing links; (b) `charm.land`'s actual URL shape for module pages IS `charm.land/<pkg>/v2`, so natural citation recovers genuine import paths; (c) a prose-URL triple with non-package subpaths ending in `/v2` is contrived.

2. **Character class loopholes (digits / hyphens / underscores):** REFUTED for current state. Empirically verified: `charm.land/bubbletea2/v2`, `charm.land/bubble-tea/v2`, `charm.land/bubble_tea/v2` all fail to match `[a-z]+` (digits break the class; hyphens and underscores are not in `[a-z]`). For current Valv state this is irrelevant — all four real direct imports (`fang`, `bubbletea`, `lipgloss`, `bubbles`) are pure lowercase `[a-z]+`. The forward-compat concern (a future Charm package named e.g. `bubbletea2`) would cause the floor to under-count, but Drop 0 is a one-shot CLAUDE.md rewrite against *current* `main/go.mod`, not a future-proof check. Not a blocker.

3. **Sort-u pathological input:** REFUTED. `sort -u` is deterministic line-ordered deduplication; it cannot produce more distinct output lines than distinct input lines. `grep -oE` emits one match per line. No whitespace / encoding trick inflates the count. No counterexample found.

4. **Regex dot-regression:** REFUTED empirically. `/usr/bin/grep -oE 'charm\.land/[a-z]+/v2' /tmp/r6_t3.md` (containing `charm-land/fang/v2`, `charm/land/fang/v2`, `charmAland/fang/v2`) returns zero matches. The escaped `\.` correctly forces literal-period matching.

5. **Deduplication edge (same-name triple):** REFUTED empirically. A file with three mentions of `charm.land/fang/v2` produces `grep -oE` output of three identical lines; `sort -u` collapses to 1; `wc -l` returns 1; floor of 3 fails → correct fail-loud behavior. Code-fence wrapping does not change matching (grep is line-oriented regardless of markdown semantics) — verified by inspection of the regex (no fence-aware constructs).

6. **Round 5 content supersession:** REFUTED. Item 38 text contains the explicit statement "Round 5 item 37's description of the post-Round-5 Charm sub-check as `grep -o 'charm.land/' | wc -l` is superseded by this item". A reader following items 36 → 37 → 38 in order is led to the Round-6 form, not the retired Round-5 form. Supersession is stated, not merely implied.

7. **Item 38 internal consistency:** REFUTED. Item 38 names the four Charm packages as `fang`, `bubbletea`, `lipgloss`, `bubbles`. All four match `[a-z]+` (pure lowercase, no digits / hyphens / underscores). Consistent.

8. **U0.2 count stability:** REFUTED. Read of lines 69-70 confirms U0.2 acceptance criteria still terminate at `15. **Agent bindings table intact.**`. No renumbering introduced by Round 6. Count = 15, matches Round 4 item 34's documented target.

9. **Cross-round content stability:** REFUTED. `git diff HEAD~1 HEAD -- drops/DROP_0_DOCS_BOOTSTRAP/PLAN.md` shows exactly the expected delta: one line rewritten at line 55 (old Round-5 `grep -o` form → new Round-6 `grep -oE … | sort -u` form) + four lines appended at the end of the file (blank line, "Round 6 revisions" lede, blank line, item 38). No silent edits to any prior-round item (1–37), acceptance criteria, unit descriptions, or other sections. Confirmed `git log` shows commit `fac7859 docs(drop-0): plan round 6 applies accepted round 5 findings` with stat "1 file changed, 5 insertions(+), 1 deletion(-)" — matches the expected narrow change.

## Verdict

Round 6 correctly applies the R5 blocking finding (F1-R5, counting-mechanic + regex-dot fix) and the R5 concern (F2-R5, unescaped-dot regex-metachar) together in a single narrow sub-check revision. The new form `grep -oE 'charm\.land/[a-z]+/v2' <file> | sort -u | wc -l >= 3` correctly (a) escapes the dot so `charm-land/` and `charm/land/` typo forms no longer match, (b) constrains matches to import-path-shape strings via the `[a-z]+/v2` tail so bare `charm.land/` prose tokens no longer contribute, (c) deduplicates via `sort -u` so the floor of 3 requires three *distinct* shape-matches rather than three raw mentions. One remaining prose-URL-triple counterexample exists in principle (F1-R6) but is mitigated by its contrived shape and the human-review gate at Phase 3 / Phase 5; no blocking severity. U0.2 criterion count remains 15; supersession chain R4 item 36 → R5 item 37 → R6 item 38 is textually explicit; the Round-6 git diff is narrow and touches only the two expected locations (line 55 + lines 221–223 appended). PASS.
