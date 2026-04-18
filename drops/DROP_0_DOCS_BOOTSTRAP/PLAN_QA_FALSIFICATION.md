# DROP_0 — PLAN QA FALSIFICATION (Round 4)

**Verdict:** PASS (no blockers)

## Findings

| ID | Severity | Unit / Item | Counterexample | Mitigation |
|---|---|---|---|---|
| F1 | concern | U0.2 crit 9 (Item 36 Tech Stack) | `grep -c 'charm.land/' <file>` counts matching **lines**, not match occurrences. A builder writing one prose line listing all four Charm imports (`charm.land/fang/v2, charm.land/bubbletea/v2, charm.land/lipgloss/v2, charm.land/bubbles/v2`) returns `1`, false-failing the floor-3 check even though intent was met. | Switch to `grep -o 'charm.land/' <file> \| wc -l` (counts matches). Or pin to a bulleted-list form via `grep -cE '^- .*charm\.land/'`. Accept as concern — rak original is bulleted, builder is likely to inherit that shape. |
| F2 | concern | U0.2 crit 11(b) (Item 31 polarity window) | Bidirectional `-B 2 -A 5` window is tight enough to reject a far-away polarity phrase, but a degenerate rewrite like `do NOT follow domain → adapters → services → cli — the correct approach is strictly layered via no cycles` still passes: arrow regex matches the negative-polarity DAG line, and the positive phrase lands within the window. | A phrase-level polarity enforcer (regex rejecting `do NOT follow.*domain.*→` on the matching line) would close this. Accept as concern — reviewer reads the Scope and would reject such a rewrite. |
| F3 | concern | U0.2 crit 13(b) (Item 35 negative-heading wordlist) | Wordlist `Legacy\|Historical\|Deprecated\|Obsolete\|Old\|Former` is incomplete: `Retired`, `Superseded`, `Past`, `Outdated`, `Archived`, `Previous` convey the same dismissive framing and all pass the zero-reject. `## Retired AGENTS.md` + body "previously authoritative, now defers to this file" passes all three sub-checks. | Extend the wordlist to `Legacy\|Historical\|Deprecated\|Obsolete\|Old\|Former\|Retired\|Superseded\|Past\|Outdated\|Archived\|Previous`. Accept as concern — builder adversary unlikely, reviewer would catch. |
| F4 | concern | U0.6 crit 5 (Item 33 `find -type f \| wc -l >= 2`) | Floor of 2 does not enforce "historical content preserved". A pathological builder could delete 46 of the 48 existing historical files and keep `.FROZEN` + 1 historical = 2 files, passing the floor. Or delete all 48 + write `.FROZEN` + any second unrelated file = 2, passing. | Snapshot the baseline count before the unit runs and require `find \| wc -l` equals (baseline + 1). Accept as concern — Scope paragraph explicitly forbids deletion and builder adversary is pathological. |
| F5 | accepted | U0.4 crit 2 (Item 28 BSD-sed drop + BOM via direct equality) | `head -1 <file>` on a BOM'd file returns `<EF><BB><BF># WIKI`, which is not string-equal to `# WIKI` — the equality check fails loudly as intended. UTF-16 BOM and leading-blank-line variants also fail loudly. | Accept — revert is correct; the builder-facing BOM-free directive + fail-loud equality is portable and correct on BSD and GNU. |
| F6 | accepted | U0.2 crit 1 (Item 29 H1 pin line-1-only) | Leading-blank-line, UTF-8 BOM, UTF-16 BOM, alternate comment styles (`<!-- … -->`), leading-space variants — all fail the `head -1 \| grep -c '^# Valv — Project CLAUDE.md'` check because either line 1 is not the H1 or the H1 is not at `^`. Line-1-only pinning resolves the Round-3 file-wide over-match risk without introducing new gaps. | Accept — tighter than the Round-3 file-wide form. |
| F7 | accepted | U0.5 crit 7 (Item 32 awk regex tightened) | `^\| *DROP_[0-9]+_` rejects header rows like `\| DROP_ID \| State \| … \|` (non-numeric after `DROP_`) and correctly accepts two-digit rows like `\| DROP_10_… \|`. Hybrid forms like `\| DROP_0A_… \|` are rejected (letter breaks the `[0-9]+_` required anchor). | Accept — extractor is correct. |
| F8 | accepted | U0.2 old crit 13 deletion (Item 34) | Removing the `mage install` prohibition as a standalone criterion is safe: adjacent crit 10 zero-rejects the string `mage install` in the file, fully subsuming the old criterion. No dangling references in other criteria. | Accept — deletion is clean. |
| F9 | accepted | U0.5 crit 3 English (Item 30 combined name-cell explicit) | The rewritten English explicitly calls out "drop number embedded in the name, not a separate cell" and matches the mechanical floors in crits 2, 6, 7. A builder reading the English is funneled toward the single-cell form that the greps require. | Accept — English is now unambiguous. |
| F10 | accepted | Ordering (all six units `blocked_by: —`) | U0.1 / U0.2 / U0.3 touch disjoint orchestrator-doc files. U0.4 creates six new files. U0.5 edits `main/PLAN.md`. U0.6 creates `.worklog/.FROZEN`. No file-level or symbol-level dependency. Genuinely parallel. | Accept. |
| F11 | accepted | Phase 6 obligation (`mage build` + `mage test`) | `mage test`'s format gate scans only `.go` files via `filepath.Ext == ".go"` and skips `.worklog/` as a directory (magefile line 311). None of U0.1-U0.6's outputs are `.go` files. `mage build` is unaffected by doc changes. Bare-root `CLAUDE.md` is above the `main/` worktree and outside `mage test`'s walk root. | Accept — no regression risk. |

## Attacks Attempted And Refuted

- **Item 28 (BSD-sed drop + direct equality):** BOM-prefixed file's `head -1` output does NOT string-equal the bare heading — fail-loud behavior preserved. UTF-16 BOM also fails. Refuted.
- **Item 29 (H1 line-1 pin):** Multi-leading-blank-lines, UTF-16 BOM, HTML comment, leading-space H1 all correctly false-fail the `head -1 \| grep -c '^# Valv…'` check. Refuted.
- **Item 30 (U0.5 crit 3 English):** Literal `\| 0 \| DOCS_BOOTSTRAP \|` reading is funneled away by the explicit "drop number embedded in the name, not a separate cell" phrase plus crits 2/6/7 anchoring to `DROP_N_<SUFFIX>` as first data cell. Refuted.
- **Item 32 (awk regex):** Two-digit `DROP_10`, `DROP_0A`, header row `DROP_ID` all behave correctly under the `^\| *DROP_[0-9]+_` pattern. Refuted.
- **Item 34 (old crit 13 delete):** Adjacent crit 10's zero-reject of `mage install` fully subsumes the prior prohibition. No criterion orphaned. Refuted.
- **Plan Note 4:** Line "NEVER run `mage install`" correctly softened to "not inherited (no such target exists in Valv's magefile)". No contradiction with magefile.go (verified: no `Install` function). Refuted.
- **Ordering assumptions:** U0.1-U0.6 touch disjoint paths at disjoint symbols. Parallelism safe. Refuted.
- **Phase 6 obligation:** `mage test`'s go-only format gate skips `.worklog/` and ignores `.md` files. Refuted.

## Scope-vs-Criteria Drift Check

Scope paragraph's load-bearing requirements — (a) rebrand orchestrator docs to Valv, (b) rewrite package map / DAG / file table / Tech Stack / Mage table against Valv reality, (c) bootstrap six Phase-7 artifacts, (d) seed `main/PLAN.md` as the ten-drop tree seeded from `main/VALV_CLAUDE_CODE_FOCUS_PLAN.md`, (e) freeze `.worklog/`, (f) keep `mage build`/`mage test` runnable at drop-end — all six are covered by U0.1-U0.6 acceptance criteria with no load-bearing gap. The four concerns above (F1-F4) are latent false-negative / bypass surfaces, not missing coverage.

## Verdict

**PASS.** No blocking counterexample. Four concerns (F1-F4) are candidates for a Round-5 tightening pass but do not warrant blocking the plan — the builder rewrite that would exploit any one of them would also be visibly adversarial and the Phase 3 dev + build-QA review would catch it. The plan is sufficiently tight to proceed to Phase 4 build.
