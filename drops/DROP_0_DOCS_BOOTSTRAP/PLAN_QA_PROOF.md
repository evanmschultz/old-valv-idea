# DROP_0 — PLAN QA PROOF (Round 6)

**Verdict:** PASS

## Findings

| ID | Severity | Unit/Item | Evidence | Recommendation |
|---|---|---|---|---|
| — | — | — | No findings. Round 6 cleanly applies F1-R5 + F2-R5 jointly. | — |

## Round 6 Scope Verification

**Narrow-diff confirmation.** `git diff HEAD~1 HEAD -- drops/DROP_0_DOCS_BOOTSTRAP/PLAN.md` shows exactly two hunks, matching the spawn contract:

1. **Line 55 (U0.2 crit 9, Charm sub-check).** The old form
   `grep -o 'charm.land/' <file> | wc -l` returns at least `3`
   is replaced by
   `grep -oE 'charm\.land/[a-z]+/v2' <file> | sort -u | wc -l` returns at least `3`.
   The accompanying rationale is fully rewritten to cite both the regex-anchor fix (escaped `\.` + `[a-z]+/v2` tail) and the distinct-import fix (`sort -u` dedup).
2. **Item 38 appended to the Round 6 revisions paragraph.** Four added lines: blank, the `Round 6 revisions (…)` heading paragraph, blank, item 38 itself. Existing Rounds 1–5 content is untouched.

No other changes. Rounds 1–5 content intact.

**Evidence check — go.mod lines 6–9 confirm all four Charm v2 direct imports exist in Valv:**

```
charm.land/bubbles/v2 v2.0.0
charm.land/bubbletea/v2 v2.0.2
charm.land/fang/v2 v2.0.0-20260228140200-f3b5a0bf202d
charm.land/lipgloss/v2 v2.0.2
```

All four package names (`fang`, `bubbletea`, `lipgloss`, `bubbles`) are pure lowercase-alpha — no digits, hyphens, or uppercase letters — so each one is matched by `[a-z]+` and the full import string is matched by `charm\.land/[a-z]+/v2`.

**Regex correctness (ERE semantics under `grep -E`):**

- `charm\.land/` — escaped `\.` forces a literal period, closing the Round-5 metachar hole where bare `.` would also match `charm-land/`, `charm/land/`, `charmAland/`, etc.
- `[a-z]+` — one or more lowercase ASCII letters. `+` is greedy but stops at the first non-matching byte; since `/` is not in `[a-z]`, it cannot cross the package-name boundary.
- `/v2` — literal three-byte suffix anchoring the match to the v2 import shape. No bare-prose `charm.land/` mention can satisfy the regex.

**Portability (BSD/macOS + GNU/Linux):**

- `grep -oE` — POSIX, identical semantics on BSD-grep and GNU-grep.
- `sort -u` — POSIX specified (IEEE Std 1003.1), identical semantics on BSD-sort and GNU-sort.
- `wc -l` — POSIX.

**Case trace (all four spawn-required traces + the F1-R5 original miss case):**

| Case | Input file content | `grep -oE` output | After `sort -u` | `wc -l` | Floor `≥ 3`? | Intended outcome | Match? |
|---|---|---|---|---|---|---|---|
| 1 (F1-R5 close) | Three bare prose mentions of `charm.land/` with zero distinct imports | (empty — no `/[a-z]+/v2` suffix) | (empty) | 0 | FAIL | FAIL | yes |
| 2 (F2-R5 close) | Typo `charm-land/fang/v2` in prose, no real imports | (empty — literal `.` required, `-` ≠ `.`) | (empty) | 0 | FAIL | FAIL | yes |
| 3 (happy path) | All four valid imports named once each | 4 lines (fang, bubbletea, lipgloss, bubbles) | 4 distinct lines | 4 | PASS | PASS | yes |
| 4 (boundary) | Three distinct imports (one Charm v2 import silently dropped) | 3 lines | 3 distinct lines | 3 | PASS (at floor) | PASS (intentional floor — three-of-four still satisfies; dropping two would fail) | yes |

**Item 38 content check.** Item 38 accurately describes:

- The two joint findings (F1-R5 block: raw-mention counting; F2-R5 concern: unescaped metachar).
- The Round-5 form being superseded (`grep -o 'charm.land/' | wc -l`).
- The new form (`grep -oE 'charm\.land/[a-z]+/v2' | sort -u | wc -l`).
- The two mechanical fixes: escaped `\.` + `[a-z]+/v2` tail (anchor), and `sort -u` (dedup).
- Portability claim (BSD/GNU POSIX).
- Preservation of the splitting-per-category structure from Round 4 item 36 (the sibling floor-0 and three floor-1 sub-checks are unchanged).
- Preservation of item 37's occurrence-counting-not-line-counting intent.

All claims in item 38 are supported by the diff and the Valv go.mod.

**U0.2 criterion count.** The U0.2 Acceptance block still enumerates criteria 1 through 15 (no renumbering, no additions, no deletions). Criterion 9's sub-bullet list is still five sub-checks: one zero-reject + four positive floors (Charm floor-3, sqlite floor-1, testcontainers floor-1, MCP SDK floor-1). Only the Charm sub-bullet's command changes — none of the others are touched.

## Verdict

Round 6 cleanly and minimally applies both F1-R5 (distinct-import dedup) and F2-R5 (escaped-dot regex anchor) in a single coordinated edit to U0.2 criterion 9's Charm sub-check, preserves the per-category splitting structure from Round 4, documents the change in item 38 of the newly added Round 6 revisions paragraph, and leaves Rounds 1–5 content and the overall criterion count (15) untouched. All four go.mod-confirmed Charm v2 imports match the new regex; all four traced cases (plus the F1-R5 originating miss case) produce the intended PASS/FAIL outcomes; portability is preserved on BSD/macOS. Verdict: PASS.
