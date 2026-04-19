# DROP_0 — Builder QA Falsification

Append a `## Unit N.M — Round K` section per falsification pass. See `main/drops/WORKFLOW.md` § "Phase 5 — Build QA (per unit)" for what each section should contain.

## Unit 0.4 — Round 1

**Verdict:** PASS

### Findings

| ID | Severity | Criterion | Counterexample | Mitigation |
|---|---|---|---|---|
| F1 | info | WORKFLOW.md Phase 4 step 3 (unit state should transit todo→in_progress→done) | Commit `7c4c9b6` flips U0.4 state directly from `todo` → `done` in a single diff; no intervening `in_progress` commit exists for U0.4 in `git log --follow drops/DROP_0_DOCS_BOOTSTRAP/PLAN.md`. | Soft issue — end state is correct and all acceptance criteria materially hold. Attack brief explicitly labels this non-fatal. Recommend future units commit the `in_progress` transition as a standalone step so the worklog + PLAN.md match phase order. |

### Attacks Attempted And Refuted

- **A1 — BOM smuggling.** `od -c` on all six files shows byte 0 is `#` (ASCII 0x23) followed by space and the heading glyphs. No `357 273 277` (U+FEFF) sequence anywhere. REFUTED.
- **A2 — Trailing whitespace / final-newline drift.** `tail -c 1 | od -c` returns `\n` for every file. No trailing CR, no no-newline-at-EOF. REFUTED.
- **A3 — Encoding drift.** `iconv -f UTF-8 -t UTF-8 … > /dev/null` exits 0 for all six. `file(1)` classifies WIKI.md / HYLLA_REFINEMENTS.md / WIKI_CHANGELOG.md as `ASCII text`, and LEDGER.md / REFINEMENTS.md / HYLLA_FEEDBACK.md as `Unicode text, UTF-8 text` (em-dash `—` U+2014 in purpose lines — still valid UTF-8, strict subset). REFUTED.
- **A4 — Heading substring collision.** `head -1 | od -c` shows each heading terminates immediately at `\n` with no trailing space or hidden character: `# WIKI\n` (7 bytes), `# LEDGER\n` (9 bytes), `# REFINEMENTS\n` (14 bytes), `# HYLLA_FEEDBACK\n` (17 bytes), `# HYLLA_REFINEMENTS\n` (20 bytes), `# WIKI_CHANGELOG\n` (17 bytes). All match spec heading exactly. REFUTED.
- **A5 — Line count overshoot.** `wc -l` returns exactly `3` for every file. Minimum is met without bloat. REFUTED.
- **A6 — Purpose-line quality.** Each purpose line describes the file's role, not a placeholder: WIKI.md="Living best-practice snapshot for Valv."; LEDGER.md="Drop-by-drop changelog for Valv — what shipped, when, and why."; REFINEMENTS.md="Post-drop refinement backlog — deferred findings and future-round work."; HYLLA_FEEDBACK.md="Per-drop record of Hylla misses — searches that forced fallback to Read/Grep/Glob."; HYLLA_REFINEMENTS.md="Proposed Hylla improvements derived from recorded misses."; WIKI_CHANGELOG.md="Per-drop WIKI.md change summaries.". Each matches filename role. REFUTED.
- **A7 — Phase-7 append-target interaction.** Every file ends with exactly one `\n` (A2). A subsequent `## DROP_N — …` append will land on a fresh line with one blank-line separator if the appender writes `\n## DROP_N …` or lands flush if it writes `## DROP_N …`. No trailing-whitespace corruption risk. REFUTED.
- **A8 — Scope drift.** `git show --stat HEAD` touches exactly 8 files: the 6 new scaffolds (`WIKI.md`, `LEDGER.md`, `REFINEMENTS.md`, `HYLLA_FEEDBACK.md`, `HYLLA_REFINEMENTS.md`, `WIKI_CHANGELOG.md`), plus `drops/DROP_0_DOCS_BOOTSTRAP/BUILDER_WORKLOG.md` (+7 lines — Round 1 entry) and `drops/DROP_0_DOCS_BOOTSTRAP/PLAN.md` (1 line — state flip). All expected, none outside scope. REFUTED.

### Verdict

PASS. All nine attack surfaces either refuted outright or flagged as a soft informational-only issue (F1: missing `in_progress` transition). The scaffolds are byte-correct, encoding-clean, scope-bounded, Phase-7-append-safe, and the HEAD commit is tidy. The sole finding is lifecycle bookkeeping, not spec-breaking. U0.4 builder output holds against falsification.
