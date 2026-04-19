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

## Unit 0.6 — Round 1

**Verdict:** PASS

### Findings

| ID | Severity | Criterion | Counterexample | Mitigation |
|---|---|---|---|---|

_No findings — all attacks refuted._

### Attacks Attempted And Refuted

- **A1 — Hidden `.gitignore` mutation.** `git show HEAD -- main/.gitignore` returns empty; `git show --stat HEAD` lists exactly two files (`drops/DROP_0_DOCS_BOOTSTRAP/PLAN.md`, `drops/DROP_0_DOCS_BOOTSTRAP/BUILDER_WORKLOG.md`). `.gitignore` is untouched. REFUTED.
- **A2 — Hidden `magefile.go` mutation.** `git show HEAD -- main/magefile.go` returns empty; commit stat confirms no Go files changed. `magefile.go:311` `.worklog` skip is intact. REFUTED.
- **A3 — Historical content deletion / mtime rewrite.** `/usr/bin/find main/.worklog -type f | wc -l` returns `49` — 48 pre-existing historical files + 1 new `.FROZEN` marker. Matches builder's claim exactly. No historical file was deleted or touched (would have shown up in commit stat or required separate mutations; commit touched zero `.worklog/` paths because the directory is gitignored). REFUTED.
- **A4 — `.FROZEN` content-quality attack (tokenistic vs semantic).** Full-file read (20 lines) shows the heading is `# .worklog/ — FROZEN (historical archive)` and body opens with "This directory is frozen as of DROP_0_DOCS_BOOTSTRAP." and "All pre-existing `.worklog/` files are historical artifacts from the pre-rak-workflow era and remain on disk for archival reference only." Both `frozen` and `historical` are used as direct descriptors of the directory and its contents, not in distracting phrases like "frozen in some other sense." Content also correctly routes readers to `main/drops/` and `main/drops/WORKFLOW.md` per spec criterion 2. REFUTED.
- **A5 — BOM smuggling on `.FROZEN`.** `xxd` first 16 bytes: `2320 2e77 6f72 6b6c 6f67 2f20 e280 9420` — i.e. `#`, space, `.worklog/`, space, then the em-dash UTF-8 triplet (`e2 80 94`). No `ef bb bf` (U+FEFF) prefix. File is raw UTF-8 starting with ASCII `# `. REFUTED.
- **A6 — Semantic `.gitignore` drift.** `Grep` of `.gitignore` for `^\.worklog/$` returns exactly one match on line 4. `Grep` for `\.worklog` anywhere returns the same single line 4 match. Direct read of `.gitignore` confirms `.worklog/` sits on line 4 between `.cache/` (line 3) and the blank line (line 5) introducing the `# OS clutter` block. No duplicate, no move. REFUTED.
- **A7 — PLAN.md off-unit edits.** `git diff HEAD~1 HEAD -- drops/DROP_0_DOCS_BOOTSTRAP/PLAN.md` is a single hunk (`@@ -149,7 +149,7 @@`) flipping one line: `- **State:** todo` → `- **State:** done` on the U0.6 row. Zero other lines changed. U0.1/U0.2/U0.3/U0.5 state cells and all acceptance criteria untouched. REFUTED.
- **A8 — BUILDER_WORKLOG.md off-unit edits.** `git diff HEAD~1 HEAD -- drops/DROP_0_DOCS_BOOTSTRAP/BUILDER_WORKLOG.md` is a single hunk `@@ -10,3 +10,10 @@` — pure append of 7 lines after the existing U0.4 Round 1 block. No deletions, no edits to U0.4 content. REFUTED.
- **A9 — find-count edge (symlinks / device files).** `/usr/bin/find main/.worklog -type l` returns empty — no symlinks. `/usr/bin/find main/.worklog -not -type f -not -type d` returns empty — no device files, sockets, or pipes. All 49 counted entries are regular files. The `49` count is honest. REFUTED.
- **A10 — Commit scope violation.** `git show --stat HEAD` touches exactly two files: `drops/DROP_0_DOCS_BOOTSTRAP/BUILDER_WORKLOG.md` (+7) and `drops/DROP_0_DOCS_BOOTSTRAP/PLAN.md` (+1/-1). No Go files, no `.gitignore`, no `magefile.go`, no other markdown. Commit subject `docs(drop-0): freeze .worklog with marker file` is accurate and in spec format (lowercase, no period, under 72 chars, no body). REFUTED.
- **A11 — `.FROZEN` leaked into git history.** `git log --all -- main/.worklog/.FROZEN` returns empty — the file is nowhere in git history, matching spec intent that the marker is local-only (disk-state, not git-state). `git check-ignore -v` confirms match on `.gitignore:4:.worklog/`. REFUTED.

### Verdict

PASS. Eleven attack surfaces probed, all refuted. Commit scope is bounded to the two expected files (PLAN.md state flip + BUILDER_WORKLOG.md append). `.FROZEN` content is 786 bytes of clean UTF-8 (no BOM), content semantically matches "frozen / historical" intent and correctly points readers at `main/drops/` as the new substrate. Historical `.worklog/` content preserved (48 + 1 = 49 regular files). `.gitignore` line 4 and `magefile.go:311` untouched. `.FROZEN` correctly gitignored and never committed. U0.6 builder output holds against falsification.
