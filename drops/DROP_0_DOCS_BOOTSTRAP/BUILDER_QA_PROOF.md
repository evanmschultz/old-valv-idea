# DROP_0 — Builder QA Proof

## Unit 0.4 — Round 1

**Verdict:** PASS

### Findings

| ID | Severity | Criterion | Evidence | Recommendation |
|---|---|---|---|---|
| — | — | — | No findings — all acceptance criteria and scope checks pass cleanly. | — |

### Acceptance Criteria Verification

| Crit | Description | Result | Evidence |
|---|---|---|---|
| 1 | `ls` of all six paths exits `0` | PASS | `/bin/ls` of the six paths printed all six and returned `EXIT=0`. |
| 2 | Each file's `head -1` byte-exactly matches expected heading; BOM-free | PASS | Per-file table below — all six headings match; `xxd -l 3` returns `23 20 <letter>` on every file (no `ef bb bf` BOM). |
| 3 | `wc -l` ≥ 3 on every file | PASS | Per-file table below — every file reports exactly `3` lines (meets the floor). |
| 4 | `grep -c '^## DROP_0'` returns `0` on every file | PASS | Per-file table below — every file reports `0`. |
| 5 | All six Phase 7 append targets resolve to existing files | PASS | Covered by criterion 1 (same six paths). |

Per-file evidence for criteria 2/3/4:

| File | `head -1` | First 3 bytes (xxd) | `wc -l` | `grep -c '^## DROP_0'` | Line 3 (purpose) |
|---|---|---|---|---|---|
| `WIKI.md` | `# WIKI` | `23 20 57` (`# W`) | 3 | 0 | `Living best-practice snapshot for Valv.` |
| `LEDGER.md` | `# LEDGER` | `23 20 4c` (`# L`) | 3 | 0 | `Drop-by-drop changelog for Valv — what shipped, when, and why.` |
| `REFINEMENTS.md` | `# REFINEMENTS` | `23 20 52` (`# R`) | 3 | 0 | `Post-drop refinement backlog — deferred findings and future-round work.` |
| `HYLLA_FEEDBACK.md` | `# HYLLA_FEEDBACK` | `23 20 48` (`# H`) | 3 | 0 | `Per-drop record of Hylla misses — searches that forced fallback to Read/Grep/Glob.` |
| `HYLLA_REFINEMENTS.md` | `# HYLLA_REFINEMENTS` | `23 20 48` (`# H`) | 3 | 0 | `Proposed Hylla improvements derived from recorded misses.` |
| `WIKI_CHANGELOG.md` | `# WIKI_CHANGELOG` | `23 20 57` (`# W`) | 3 | 0 | `Per-drop WIKI.md change summaries.` |

Additional (non-acceptance) gate checks:

| Check | Result | Evidence |
|---|---|---|
| Purpose subheading present on line 3 of each file | PASS | Line 3 column above — all six populated with a meaningful purpose line. |
| PLAN.md U0.4 state flipped to `done` | PASS | PLAN.md line 92: `- **State:** done`. |
| BUILDER_WORKLOG.md has `## Unit 0.4 — Round 1` with required fields | PASS | WORKLOG line 7 opens `## Unit 0.4 — Round 1`; lines 9–12 carry `**Scope:**`, `**Files created:**`, `**Mage targets run:**`, `**Design notes:**` in order. |

### Scope / Commit Hygiene

Commit `7c4c9b627f07b6f68c7179ea6ebf261d7645f523` — `docs(drop-0): bootstrap phase-7 closeout artifacts` — stat:

| Expected (8 files) | Actual (8 files) | Match |
|---|---|---|
| `main/WIKI.md` (new) | `WIKI.md` +3 | OK |
| `main/LEDGER.md` (new) | `LEDGER.md` +3 | OK |
| `main/REFINEMENTS.md` (new) | `REFINEMENTS.md` +3 | OK |
| `main/HYLLA_FEEDBACK.md` (new) | `HYLLA_FEEDBACK.md` +3 | OK |
| `main/HYLLA_REFINEMENTS.md` (new) | `HYLLA_REFINEMENTS.md` +3 | OK |
| `main/WIKI_CHANGELOG.md` (new) | `WIKI_CHANGELOG.md` +3 | OK |
| `main/drops/DROP_0_DOCS_BOOTSTRAP/BUILDER_WORKLOG.md` (modified) | `drops/DROP_0_DOCS_BOOTSTRAP/BUILDER_WORKLOG.md` +7 | OK |
| `main/drops/DROP_0_DOCS_BOOTSTRAP/PLAN.md` (modified) | `drops/DROP_0_DOCS_BOOTSTRAP/PLAN.md` +1/-1 | OK |

Eight files expected, eight files touched, zero extras. Total `+26 -1` matches the 6×3 new-scaffold writes + 7-line WORKLOG append + 1-line PLAN state flip. Commit subject conforms to the `type(scope): message` convention. No body, no bullet lists, no trailers.

### Verdict

All five U0.4 acceptance criteria pass on-disk, all three additional gate checks (purpose subheading on line 3, PLAN.md state flipped to `done`, BUILDER_WORKLOG.md Round 1 section well-formed) pass, and the bootstrap commit's scope is exactly the eight expected files with nothing else. The six durable Phase 7 closeout artifacts are in place, BOM-free, 3-line minimal scaffolds, and free of pre-populated `## DROP_0` stubs that could collide with real Phase 7 appends. Proof-QA verdict: PASS.

## Unit 0.6 — Round 1

**Verdict:** PASS

### Findings

| ID | Severity | Criterion | Evidence | Recommendation |
|---|---|---|---|---|
| — | — | — | No findings — all five acceptance criteria and all four additional hygiene checks pass cleanly. | — |

### Acceptance Criteria Verification

| Crit | Description | Result | Evidence |
|---|---|---|---|
| 1 | `ls .worklog/.FROZEN` exits 0 (marker exists) | PASS | `test -f /Users/evanschultz/Documents/Code/hylla/valv/main/.worklog/.FROZEN` returned `EXISTS`. |
| 2a | `grep -c 'main/drops/' .FROZEN` ≥ 1 | PASS | Grep count = **3** (lines 9, 10, 14 of `.FROZEN` all reference `main/drops/`). |
| 2b | `grep -ci 'frozen\|historical' .FROZEN` ≥ 1 | PASS | Grep count = **3** (heading + body). |
| 3 | `grep -cE '^\.worklog/$' .gitignore` exactly 1 (no duplicate) | PASS | Grep count = **1**. Direct `Read` confirms `.gitignore` line 4 is `.worklog/` (unchanged). |
| 4 | `grep -c '\.worklog' magefile.go` ≥ 1 (skip intact) | PASS | Grep count = **1** at line 311: `case ".git", ".tmp", ".worklog":` in `goFiles`. |
| 5 | `find .worklog -type f \| wc -l` ≥ 2 (marker + ≥1 historical) | PASS | Glob enumeration returned **48 files** (47 historical logs + `.FROZEN` marker). |

### Additional Hygiene Checks

| Check | Result | Evidence |
|---|---|---|
| `.FROZEN` content is informative (not empty / placeholder) | PASS | 19 lines of substantive prose — heading + 3 body paragraphs explaining freeze rationale + pointer to `main/drops/WORKFLOW.md` + gitignore note. |
| PLAN.md U0.6 state is `done` | PASS | `drops/DROP_0_DOCS_BOOTSTRAP/PLAN.md` line 152: `- **State:** done`. |
| BUILDER_WORKLOG.md has `## Unit 0.6 — Round 1` with required fields | PASS | Lines 14–19: heading + `**Scope:**` + `**Files created:**` + `**Mage targets run:**` + `**Design notes:**` in order. |
| `.gitignore` line 4 literal read — `.worklog/` unchanged | PASS | Direct `Read` of `.gitignore` line 4: `.worklog/` (exact, not just a count match). |

### Scope / Commit Hygiene

Commit `97168a1e68e950fa5282130acb813d8ac577bb69` — `docs(drop-0): freeze .worklog with marker file` — `git show --stat` result:

| Expected | Actual | Match |
|---|---|---|
| `drops/DROP_0_DOCS_BOOTSTRAP/BUILDER_WORKLOG.md` (modified, +7) | `drops/DROP_0_DOCS_BOOTSTRAP/BUILDER_WORKLOG.md` +7 | OK |
| `drops/DROP_0_DOCS_BOOTSTRAP/PLAN.md` (modified, +1/-1) | `drops/DROP_0_DOCS_BOOTSTRAP/PLAN.md` +1/-1 | OK |
| `.worklog/.FROZEN` MUST NOT appear (gitignored by design) | not present | OK |

Two files expected, two files touched, zero extras. Total `+8 -1` matches the 7-line WORKLOG append + 1-line PLAN state flip. `.FROZEN` correctly absent from the commit because `.gitignore` line 4 (`.worklog/`) excludes it — spec's "marker is local-only, acceptance is disk-state not git-state" invariant holds. Commit subject conforms to `type(scope): message`, no body, no trailers. HEAD = 97168a1.

### Verdict

All five U0.6 acceptance criteria pass on-disk, all four additional hygiene checks (`.FROZEN` content substantive, PLAN.md state flipped to `done`, BUILDER_WORKLOG.md Round 1 section well-formed, `.gitignore` line 4 literal unchanged) pass, and commit 97168a1's scope is exactly the two expected tracked files — with `.FROZEN` correctly omitted from git per the spec's "local-only marker" design. No gitignore duplicate was added; `magefile.go` `.worklog` skip at line 311 remains intact; 48 files under `.worklog/` confirm historical content was preserved alongside the new marker. Proof-QA verdict: PASS.

## Unit 0.1 — Round 1

**Verdict:** PASS

### Findings

| ID | Severity | Criterion | Evidence | Recommendation |
|---|---|---|---|---|
| — | — | — | No findings — all 7 acceptance greps return the expected counts and the rebranded file is structurally coherent. | — |

### Acceptance Criteria Verification

| Crit | Pattern | Expected | Observed | Result |
|---|---|---|---|---|
| 1 | `Rak` (case-sensitive substring) | `0` | `0` | PASS |
| 2 | `\brak\b` (case-insensitive, word-boundary) | `0` | `0` | PASS |
| 3 | `/hylla/rak` | `0` | `0` | PASS |
| 4 | `Go source lands here in Drop 1` | `0` | `0` | PASS |
| 5 | `^# Valv — Steward Orchestrator` | `1` | `1` | PASS |
| 6 | `AGENTS.md\|VALV_ACCOUNT_SWITCH_PLAN.md\|VALV_CLAUDE_CODE_FOCUS_PLAN.md` | `≥1` | `5` | PASS |
| 7a | `^- \*\*Never edits Go source` | `≥1` | `1` | PASS |
| 7b | `^- \*\*Never runs mage` | `≥1` | `1` | PASS |
| 7c | `^- \*\*Never commits or pushes` | `≥1` | `1` | PASS |
| 7d | `^## What The Steward Does NOT Do` | `≥1` | `1` | PASS |

All 10 grep checks (7 criteria, one of which expands to four sub-checks) match the builder's self-reported counts exactly. Executed via the `Grep` MCP tool against `/Users/evanschultz/Documents/Code/hylla/valv/CLAUDE.md`.

### Structural Hygiene

| Check | Result | Evidence |
|---|---|---|
| No `rak` substring anywhere (case-insensitive, non-word-boundary) | PASS | `Grep -i` for `rak` returned zero matches — covers not just `\brak\b` but any embedded occurrence (e.g. `rakish`, `Krak`, `tracker`). Confirms criterion 2 has no evasive near-miss. |
| Heading tree well-formed | PASS | 12 `#`/`##`/`###` headings — single H1 (`# Valv — Steward Orchestrator CLAUDE.md (bare-root)`), eight H2 sections (Two Orchestrator Shapes, Bare-Root Plan Docs, Git Setup, Steward Responsibilities, What The Steward Does NOT Do, Coordination Model, Evidence Sources, Git Commit Format, Safety, Recovery After Session Restart — ten H2s in fact, not eight; recounted: 10 H2 + 1 H3 nested under "Git Setup" = 11 sub-H1s, plus the H1 itself = 12 total), one H3 (`### Git Commands From The Steward` nested correctly under `## Git Setup`). No orphan heading levels. |
| Em-dash (U+2014) preserved | PASS | 37 occurrences of `—` throughout file — matches builder's claim of em-dash preservation in title + prose. Title line 1 renders em-dash natively (`Valv — Steward Orchestrator`). |
| BOM-free | PASS | Criterion 5 regex `^# Valv — Steward Orchestrator` matches with count 1 — a leading U+FEFF BOM byte would push `#` off column 1 and break the `^#` anchor. Builder used `Write` tool (no BOM emission). Line 1 reads clean in `Read` cat-n output. |
| Markdown-only (no HTML) | PASS | No `<` / `>` angle-bracket HTML tags in grep results; only markdown constructs (headings, bullet lists, code fences, a fenced tree diagram). |
| Bullet integrity under "What The Steward Does NOT Do" | PASS | Five `- **Never ...**` bullets at that section, four of which are the canonical invariants checked (edits Go source, runs mage, runs hylla_ingest, commits or pushes, spawns builder/QA/planning). Criterion 7 covers three of the four plus the section heading — the uncovered two (`hylla_ingest`, `spawns builder/QA/planning`) are preserved intact per builder claim and visible in the Read output (lines 87, 89). |
| Tillsyn disclaimer preserved | PASS | Line 93: `Valv does **not** use Tillsyn.` + line 142: `Filesystem + git, no Tillsyn calls.` — the "does not use Tillsyn" assertion carried through the rebrand cleanly. |

### Bare-Root Plan Docs Cross-Check

Criterion 6 requires ≥1 mention of any of `AGENTS.md`, `VALV_ACCOUNT_SWITCH_PLAN.md`, `VALV_CLAUDE_CODE_FOCUS_PLAN.md`. Observed count is `5`. Verified each cited doc actually exists on disk at the bare-root via `Glob`:

| Cited doc | Exists on disk | Evidence |
|---|---|---|
| `AGENTS.md` | PASS | `Glob` returned `/Users/evanschultz/Documents/Code/hylla/valv/AGENTS.md`. |
| `VALV_ACCOUNT_SWITCH_PLAN.md` | PASS | `Glob` returned `/Users/evanschultz/Documents/Code/hylla/valv/VALV_ACCOUNT_SWITCH_PLAN.md`. |
| `VALV_REPO_PLAN.md` | PASS | `Glob` returned `/Users/evanschultz/Documents/Code/hylla/valv/VALV_REPO_PLAN.md`. |
| `valv_architecture_notes.md` | PASS | `Glob` returned `/Users/evanschultz/Documents/Code/hylla/valv/valv_architecture_notes.md`. |
| `PLAN.md` (bare-root scratchpad) | PASS | `Glob` returned `/Users/evanschultz/Documents/Code/hylla/valv/PLAN.md`. |

Note: `VALV_CLAUDE_CODE_FOCUS_PLAN.md` (one of the criterion-6 alternatives) is NOT referenced in the rebranded file — but criterion 6 is an OR across three alternatives, and `AGENTS.md` + `VALV_ACCOUNT_SWITCH_PLAN.md` are both cited, so the OR is satisfied twice over. No finding.

### Scope / Ownership Hygiene

| Check | Result | Evidence |
|---|---|---|
| Only `/Users/evanschultz/Documents/Code/hylla/valv/CLAUDE.md` rewritten in this round | PASS | Builder's worklog line 24 names `/Users/evanschultz/Documents/Code/hylla/valv/CLAUDE.md` as the only file changed via `Write`. No other Unit 0.1 file mutations claimed. |
| PLAN.md U0.1 state (expected `done` per Phase 4 step 3 of WORKFLOW.md) | Unknown — not in scope for proof-QA | PLAN.md state flip is verified at Phase 5 exit by orchestrator, not in proof-QA scope. Flagged for orch. |
| BUILDER_WORKLOG.md U0.1 Round 1 section present + well-formed | PASS | WORKLOG lines 21–39 carry `## Unit 0.1 — Round 1` + `**Scope:**` + `**Files changed:**` + `**Mage targets run:**` + `**Acceptance self-check:**` + `**Design notes:**` + `**Unknowns:**` in order. All 7 criterion self-checks numerically correct, as re-verified here. |

### Builder Design-Note Cross-Check

Spot-verified three substantive design decisions the builder called out in the worklog:

1. **Tree diagram rebrand from `FETCH_HEAD` + `packed-refs` + "Go source lands here in Drop 1" to concrete Valv layout.** Confirmed — lines 30–58 of the rebranded file show the tree diagram with `cmd/valv/` + `internal/` + `magefile.go` entries and no `FETCH_HEAD` / `packed-refs` / "Go source lands here in Drop 1" annotation (criterion 4 = 0 corroborates).
2. **Third commit example rebranded from `chore(plan): reconcile plan.md with tillsyn after drop 1 lands` to `chore(plan): reconcile plan.md with drop tree after drop 1 lands`.** Confirmed — line 128 of the rebranded file reads `chore(plan): reconcile plan.md with drop tree after drop 1 lands`. The rak-era `tillsyn` reference is gone.
3. **"What The Steward Does NOT Do" section retains all five canonical bullets (edits Go source / runs mage / runs hylla_ingest / commits or pushes / spawns builder/QA/planning).** Confirmed — lines 85–89 carry all five bullets in order, bold-prefixed, each starting with `- **Never`.

No drift between builder self-report and on-disk reality.

### Hylla Feedback

N/A — task touched non-Go files only (bare-root `CLAUDE.md` is markdown; Hylla today indexes Go only). No Hylla queries attempted, no fallback needed.

### Proof Certificate

- **Premises** — (1) The 7 acceptance greps must return the counts the builder claimed. (2) The rebranded file must be structurally coherent markdown — single H1, no orphan heading levels, preserved em-dashes, BOM-free, no HTML. (3) The bare-root plan docs the file now references must actually exist on disk. (4) Steward-boundary invariants (the "Never ..." bullets under `## What The Steward Does NOT Do`) must be preserved verbatim.
- **Evidence** — 10 `Grep` MCP queries against the rebranded file produced the exact counts claimed in the worklog (0/0/0/0/1/5/1/1/1/1). 11-entry heading inventory from `Grep '^#{1,4} '` shows a clean H1 → H2 → H3 tree. 37 em-dash (U+2014) occurrences corroborate the builder's "em-dash preserved" claim. `Grep -i rak` returned zero matches — stronger than criterion 2's word-boundary check. `Glob` confirmed all five cited bare-root docs exist. `Read` of line 1 shows `# Valv — Steward Orchestrator CLAUDE.md (bare-root)` — BOM-free.
- **Trace or cases** — For each of the 7 acceptance criteria, I executed the exact grep the criterion specifies (one `Grep` tool call per criterion, four calls for criterion 7's sub-checks), compared the observed count to the expected, and recorded PASS. Additionally, I ran a case-insensitive `rak` substring search to rule out non-word-boundary residuals (e.g. `rakish`, `tracker`), a heading-tree inventory, an em-dash count, and a BOM probe via the `^#` anchor behavior of criterion 5.
- **Conclusion** — All 7 acceptance criteria pass, structural hygiene passes, bare-root plan doc cross-references resolve, no residual rak substrings anywhere in the file (case-insensitive), boundary invariants preserved. Unit 0.1 Round 1 proof-QA verdict: **PASS**.
- **Unknowns** — (1) PLAN.md U0.1 state flip to `done` was not verified in this pass — orchestrator scope per WORKFLOW.md Phase 4. (2) The sibling `BUILDER_QA_FALSIFICATION.md` Round 1 section is being written in parallel and may surface counterexamples this proof pass does not consider; routed to orchestrator for Phase 5 synthesis.

### Verdict

All 7 U0.1 acceptance criteria pass (verified numerically via `Grep` MCP, counts exactly match builder's self-report). Structural hygiene passes (single H1, 10 H2s, 1 H3 nested correctly, 37 em-dashes preserved, BOM-free, markdown-only, zero `rak` substrings case-insensitive). Bare-root plan docs cross-check passes (all five cited docs exist on disk). Steward-boundary invariants preserved intact (five `- **Never` bullets under the preserved section heading). Design-note cross-checks (tree diagram rebrand, commit-example rebrand, boundary-bullet preservation) all corroborated against on-disk content. Proof-QA verdict: PASS.

## Unit 0.5 — Round 1

**Verdict:** PASS

### Findings

| ID | Severity | Criterion | Evidence | Recommendation |
|---|---|---|---|---|
| — | — | — | No findings — all 10 U0.5 acceptance criteria and every additional hygiene check pass cleanly. | — |

### Acceptance Criteria Verification

All counts observed via `Grep` MCP tool and direct `Read` / `Glob` inspection against `/Users/evanschultz/Documents/Code/hylla/valv/main/PLAN.md`. Exact criterion regex from drop PLAN.md lines 115–147.

| Crit | Check | Expected | Observed | Result |
|---|---|---|---|---|
| 1 | `grep -c '^# Valv' PLAN.md` | `>= 1` | `1` | PASS |
| 2 | `grep -cE '^\| *DROP_[0-9]+' PLAN.md` | `10` | `10` | PASS |
| 3 | `grep -cE '^\| *DROP_[0-9]+_' PLAN.md` | `10` | `10` | PASS |
| 4 | `grep 'DROP_0_DOCS_BOOTSTRAP' \| grep -c 'building'` | `>= 1` | `1` | PASS |
| 5a | `grep -cE 'DROP_[1-9]' PLAN.md` | `>= 9` | `11` (9 data rows + 2 prose refs in intro) | PASS |
| 5b | `grep -cE 'DROP_[1-9].*state: done' PLAN.md` | `0` | `0` | PASS |
| 6.0 | `^\|.*DROP_0_DOCS_BOOTSTRAP.*\|.*blocked_by: *(—\|-\|none).*\|` | `>= 1` | `1` | PASS |
| 6.1 | `^\|.*DROP_1_DELETE_API_WRAPPER.*\|.*blocked_by: *DROP_0.*\|` | `>= 1` | `1` | PASS |
| 6.2 | `^\|.*DROP_2_DOMAIN_PLUMBING_PROVIDER_CLAUDE.*\|.*blocked_by: *DROP_1.*\|` | `>= 1` | `1` | PASS |
| 6.3 | `^\|.*DROP_3_SCHEMA_MIGRATION_COMPOSITE_BINDING.*\|.*blocked_by: *DROP_2.*\|` | `>= 1` | `1` | PASS |
| 6.4 | `^\|.*DROP_4_CLAUDE_DOCKER_IMAGE.*\|.*blocked_by: *DROP_3.*\|` | `>= 1` | `1` | PASS |
| 6.5 | `^\|.*DROP_5_CLAUDE_PROVIDER_ADAPTER.*\|.*blocked_by: *DROP_4.*\|` | `>= 1` | `1` | PASS |
| 6.6 | `^\|.*DROP_6_CLAUDE_SERVICE_AND_CLI.*\|.*blocked_by: *DROP_5.*\|` | `>= 1` | `1` | PASS |
| 6.7 | `^\|.*DROP_7_ACCOUNT_SURFACE_PARITY.*\|.*blocked_by: *DROP_6.*\|` | `>= 1` | `1` | PASS |
| 6.8 | `^\|.*DROP_8_E2E_AND_DOCS.*\|.*blocked_by: *DROP_7.*\|` | `>= 1` | `1` | PASS |
| 6.9 | `^\|.*DROP_9_CLEANUP_BACKLOG.*\|.*blocked_by: *DROP_8.*\|` | `>= 1` | `1` | PASS |
| 7 | awk strict-ascending `0..9` drop-number sequence | exit `0`, sequence `0,1,...,9` | Row inspection at lines 23–32 shows `DROP_0` through `DROP_9` in order; builder self-check reports awk pipeline exit `0` (WORKLOG L64) | PASS |
| 8 | `grep -c 'main/drops/DROP_0_DOCS_BOOTSTRAP' PLAN.md` | `>= 1` | `1` | PASS |
| 9a | `wc -l PLAN.md` | `< 500` | `38` | PASS |
| 9b | `grep -cE 'Track [A-F]\|Agent 5' PLAN.md` | `0` | `0` | PASS |
| 10 | `grep -c 'WORKFLOW.md\|VALV_CLAUDE_CODE_FOCUS_PLAN.md' PLAN.md` | `>= 2` | `15` | PASS |

### Criterion 7 Verification Detail

Independent re-derivation of the drop-number sequence by direct row inspection of `PLAN.md` lines 23–32:

| Line | First cell | Extracted drop number |
|---|---|---|
| 23 | `DROP_0_DOCS_BOOTSTRAP` | 0 |
| 24 | `DROP_1_DELETE_API_WRAPPER` | 1 |
| 25 | `DROP_2_DOMAIN_PLUMBING_PROVIDER_CLAUDE` | 2 |
| 26 | `DROP_3_SCHEMA_MIGRATION_COMPOSITE_BINDING` | 3 |
| 27 | `DROP_4_CLAUDE_DOCKER_IMAGE` | 4 |
| 28 | `DROP_5_CLAUDE_PROVIDER_ADAPTER` | 5 |
| 29 | `DROP_6_CLAUDE_SERVICE_AND_CLI` | 6 |
| 30 | `DROP_7_ACCOUNT_SURFACE_PARITY` | 7 |
| 31 | `DROP_8_E2E_AND_DOCS` | 8 |
| 32 | `DROP_9_CLEANUP_BACKLOG` | 9 |

Sequence `0,1,2,3,4,5,6,7,8,9` — ten values, strictly increasing by one, starting at 0, ending at 9. Builder's awk pipeline self-report (WORKLOG L64) matches.

### Additional Hygiene Checks

| Check | Result | Evidence |
|---|---|---|
| File is UTF-8, BOM-free | PASS | `file PLAN.md` reports `Unicode text, UTF-8 text` (not `UTF-8 Unicode (with BOM) text`). `xxd -l 3 PLAN.md` returns `23 20 56` (`# V`), not `ef bb bf`. Criterion 1's `^# Valv` grep matched count `1` — a leading U+FEFF BOM would displace `#` off column 1 and break the `^#` anchor. |
| Line 1 H1 title starts with `# Valv` | PASS | `Read` of PLAN.md line 1: `# Valv — Drop-Tree Index`. Single H1 per standard markdown idiom. |
| Markdown-only (no HTML) | PASS | `Read` of all 38 lines shows only markdown constructs (H1/H2, prose, bullet list, one table, bracketed links). No `<...>` HTML tags. |
| Table pipe parity (parseable markdown) | PASS | `Grep '^\|'` returned 12 rows (header L21, separator L22, 10 data rows L23–L32). Each row has exactly 6 `\|` characters → 5 columns per row — uniform across header, separator, and all ten data rows. Parseable as a 5-column markdown table. |
| `main/drops/DROP_0_DOCS_BOOTSTRAP/` path resolves to a real directory | PASS | `Glob` on `main/drops/DROP_0_DOCS_BOOTSTRAP/*` returned five files: `CLOSEOUT.md`, `PLAN.md`, `BUILDER_QA_PROOF.md`, `BUILDER_QA_FALSIFICATION.md`, `BUILDER_WORKLOG.md`. Directory exists and is populated with the expected DROP_0 artifact set. |
| Forward-looking dir links for DROP_1..DROP_9 are stubs (explicitly documented) | PASS | `Conventions` section L17: "Per-drop dir links for DROP_1..DROP_9 are forward-looking stubs — the directories get scaffolded from `main/drops/_TEMPLATE/` at that drop's Phase 1 per `main/drops/WORKFLOW.md`." Reader will not be surprised that `drops/DROP_1_DELETE_API_WRAPPER/` etc. do not yet exist. |

### BUILDER_WORKLOG U0.5 Round 1 Section Check

`/Users/evanschultz/Documents/Code/hylla/valv/main/drops/DROP_0_DOCS_BOOTSTRAP/BUILDER_WORKLOG.md` lines 41–79:

| Required field | Present? | Evidence |
|---|---|---|
| `## Unit 0.5 — Round 1` heading | PASS | Line 41: `## Unit 0.5 — Round 1`. |
| Scope | PASS | Line 43: `- **Scope:** rewrite \`main/PLAN.md\`…` |
| Files | PASS | Line 44: `- **Files changed:** /Users/evanschultz/Documents/Code/hylla/valv/main/PLAN.md (full rewrite via \`Write\`).` |
| Acceptance self-check | PASS | Lines 46–67: `- **Acceptance self-check (all 10 criteria, observed values):**` followed by per-criterion observed counts; every claimed count matches this proof pass numerically. |
| Design notes | PASS | Lines 68–77: `- **Design notes:**` with substantive subsidiary bullets (column shape, dir link format, DROP_0 state choice, DROP_1..9 dir stubs, DROP_8/9 scope judgment calls, DROP_3 name, line-count aggression, companion references, no `hylla_ingest` / mage). |

### Structural Cross-Check

| Section | Result | Evidence |
|---|---|---|
| Reading Order | PASS | Lines 5–10: four-bullet reading order covers `this file` / `WORKFLOW.md` / `VALV_CLAUDE_CODE_FOCUS_PLAN.md` / per-drop `PLAN.md`. Active-drop pointer names `main/drops/DROP_0_DOCS_BOOTSTRAP/` (satisfies crit 8). |
| Conventions | PASS | Lines 12–17: container-state vocabulary, `blocked_by` linear-chain rationale (ties to focus plan §6), one-sentence scope rule, forward-looking dir-stub explanation. |
| Drop Tree table | PASS | Lines 19–32: `## Drop Tree` heading + 5-column header + separator + 10 data rows, uniform pipe parity. |
| Maintenance | PASS | Lines 34–38: three bullets — state-flip phase anchors into WORKFLOW.md, "do not edit mid-build" guard, `mage plan-check` parity note (forward-looking per `main/CLAUDE.md`). |

### Builder Design-Note Cross-Check

Spot-verified three substantive design decisions the builder called out in the worklog:

1. **Column shape: drop name embedded in first cell (`| DROP_N_<SUFFIX> |`)** — Confirmed. Header row at L21 reads `| Drop | State | Blocked By | Scope | Dir |`; data rows embed the drop name as the first cell's content (`DROP_0_DOCS_BOOTSTRAP`, not a `0 | DOCS_BOOTSTRAP` split). This is exactly what criteria 2, 3, 6, and 7 anchor on.
2. **Dir link format: relative `drops/DROP_N_<NAME>/` from PLAN.md's location** — Confirmed. Link format `[dir](drops/DROP_0_DOCS_BOOTSTRAP/)` at L23 resolves correctly from `main/PLAN.md` to `main/drops/DROP_0_DOCS_BOOTSTRAP/` (verified via `Glob`). Absolute-path prose mention in Reading Order satisfies criterion 8 literal grep.
3. **DROP_0 state seeded as `building`, all others `todo`** — Confirmed. L23 carries `state: building`; L24–L32 each carry `state: todo`. Criterion 4 passes (`building` on DROP_0 row), criterion 5b passes (no `state: done` on any DROP_[1-9]).

No drift between builder self-report and on-disk reality.

### Hylla Feedback

N/A — task touched non-Go files only (`main/PLAN.md` is markdown; Hylla today indexes Go only). No Hylla queries attempted, no fallback needed.

### Proof Certificate

- **Premises** — (1) Every one of the ten U0.5 acceptance criteria greps/awk returns the expected count / exit. (2) The file is syntactically valid markdown: UTF-8, BOM-free, line-1 H1 starting `# Valv`, pipe-parity on every table row. (3) The `main/drops/DROP_0_DOCS_BOOTSTRAP/` path referenced in the file resolves to a real populated directory. (4) `BUILDER_WORKLOG.md` `## Unit 0.5 — Round 1` section carries the four required fields (Scope / Files / Acceptance self-check / Design notes) and its self-reported counts match the on-disk counts.
- **Evidence** — 17 `Grep` MCP queries (one per criterion sub-check, plus literal substring checks), direct line-by-line `Read` inspection of PLAN.md (all 38 lines), `Read` inspection of BUILDER_WORKLOG.md lines 41–79, `Glob` enumeration of `main/drops/DROP_0_DOCS_BOOTSTRAP/*`, `xxd -l 3` + `file` for BOM / UTF-8 probe, `wc -l` for line-count, direct row-inspection-driven re-derivation of the drop-number sequence for criterion 7 as a cross-check on the builder's awk pipeline self-report.
- **Trace or cases** — For each of the 10 acceptance criteria (plus criterion 6's ten sub-greps and criterion 7's sequence re-derivation), executed the exact check the criterion specifies, compared observed to expected, and recorded PASS. Additionally: BOM probe (`xxd` + `file`), encoding probe, pipe-parity inspection across all 12 table rows, directory-resolution check via `Glob`, worklog-field presence check.
- **Conclusion** — All 10 U0.5 acceptance criteria pass on-disk numerically (17 of 17 sub-checks green). File is UTF-8, BOM-free, markdown-only, structurally coherent. Drop tree table has uniform 6-pipe parity across header + separator + 10 data rows. DROP_0 directory resolves. Builder worklog section is well-formed. Unit 0.5 Round 1 proof-QA verdict: **PASS**.
- **Unknowns** — (1) PLAN.md U0.5 state flip to `done` at the drop-dir `PLAN.md` level was not verified in this pass — that is Phase 5 exit / Phase 7 closeout orchestrator scope, not proof-QA. (2) The parallel `BUILDER_QA_FALSIFICATION.md` Round 1 pass may surface counterexamples that this proof review does not consider — routed to orchestrator for Phase 5 synthesis.

### Verdict

All 10 U0.5 acceptance criteria pass (17 of 17 sub-checks green, verified numerically via `Grep` MCP). File hygiene passes (UTF-8, BOM-free, `# Valv` H1 on line 1, markdown-only, uniform 6-pipe parity across all 12 table rows). `main/drops/DROP_0_DOCS_BOOTSTRAP/` path resolves to a populated drop directory. `BUILDER_WORKLOG.md` `## Unit 0.5 — Round 1` section carries Scope / Files / Acceptance self-check / Design notes as required. Builder design-note cross-checks (column shape, dir link format, DROP_0 state seeding) all corroborated against on-disk content. Zero drift between builder self-report and observed state. Proof-QA verdict: PASS.

## Unit 0.3 — Round 1

**Verdict:** PASS

### Findings

| ID | Severity | Criterion | Evidence | Recommendation |
|---|---|---|---|---|
| — | — | — | No findings — all 9 U0.3 acceptance criteria pass and every extra gate (markdown-only / UTF-8 / BOM-free / phase-heading ascending order / preamble internal consistency / CLOSEOUT minimal-diff reasoning / worklog field set) passes. | — |

### Acceptance Criteria Verification

All counts observed via `Grep` MCP tool against `/Users/evanschultz/Documents/Code/hylla/valv/main/drops/WORKFLOW.md` and `/Users/evanschultz/Documents/Code/hylla/valv/main/drops/_TEMPLATE/CLOSEOUT.md`. Exact criterion regex from drop PLAN.md lines 79–87.

| Crit | Check | Expected | Observed | Result |
|---|---|---|---|---|
| 1 | `grep -c 'Rak' WORKFLOW.md` | `0` | `0` | PASS |
| 2 | `grep -ciE '\brak\b' WORKFLOW.md` | `0` | `0` | PASS |
| 3 | `grep -c '^# Valv — Per-Drop Workflow' WORKFLOW.md` | `1` | `1` | PASS |
| 4a | `grep -c 'DROP_1_CODE_SCAFFOLD_MAGE_CI' WORKFLOW.md` | `0` | `0` | PASS |
| 4b | `grep -c 'DROP_1_DELETE_API_WRAPPER' WORKFLOW.md` | `>= 1` | `1` | PASS |
| 5a | `grep -c 'rak artifact' WORKFLOW.md` | `0` | `0` | PASS |
| 5b | `grep -c 'Valv artifact' WORKFLOW.md` | `>= 1` | `1` | PASS |
| 6a | `grep -ci 'rak does not use' WORKFLOW.md` | `0` | `0` | PASS |
| 6b | `grep -c 'Valv does not use' WORKFLOW.md` | `>= 1` | `1` | PASS |
| 7a | `grep -c 'github.com/evanmschultz/rak@main' _TEMPLATE/CLOSEOUT.md` | `0` | `0` | PASS |
| 7b | `grep -c 'github.com/evanmschultz/valv@main' _TEMPLATE/CLOSEOUT.md` | `>= 1` | `1` | PASS |
| 8 | `grep -c '^## Phase [1-7] — ' WORKFLOW.md` | `7` | `7` | PASS |
| 9 | `grep -c 'Agent Spawn Contract' WORKFLOW.md` | `>= 1` | `6` | PASS |

All 13 sub-checks green. Counts match builder's self-report in BUILDER_WORKLOG.md lines 88–97 exactly.

### Phase Heading Ordering (extra gate)

Grep `^## Phase [1-7] — ` returned seven hits in strictly ascending order:

| Line | Heading |
|---|---|
| 93 | `## Phase 1 — Plan` |
| 102 | `## Phase 2 — Plan QA` |
| 112 | `## Phase 3 — Discuss + Cleanup` |
| 123 | `## Phase 4 — Build (per unit)` |
| 132 | `## Phase 5 — Build QA (per unit)` |
| 142 | `## Phase 6 — Verify` |
| 154 | `## Phase 7 — Closeout` |

Sequence `1,2,3,4,5,6,7` — monotone, no gaps, no reorder. Phase names match the canonical order declared on line 48 (`Plan → Plan QA → Discuss + Cleanup → … → Build → Build QA → Verify → Closeout → next drop`).

### Extra Hygiene Gates

| Check | Result | Evidence |
|---|---|---|
| WORKFLOW.md is markdown-only | PASS | `Read` of all 216 lines shows only markdown constructs (H1/H2/H3/H4 headings, fenced code blocks, bullet lists, table pipe rows, bold/italic inline). No `<…>` HTML tags. |
| WORKFLOW.md is UTF-8, BOM-free | PASS | `file` reports `Unicode text, UTF-8 text`. `xxd -l 6` returns `23 20 56 61 6c 76` (`# Valv`) — no `ef bb bf` BOM prefix. Criterion 3's `^# Valv` grep matched count `1`, which a leading BOM would have broken. Em-dash U+2014 preserved in the H1 title. |
| _TEMPLATE/CLOSEOUT.md is UTF-8, BOM-free | PASS | `file` reports `Unicode text, UTF-8 text`. `xxd -l 6` returns `23 20 44 52 4f 50` (`# DROP`) — no BOM. |
| Preamble fenced block (lines 60–81) internally consistent post-rebrand — no orphan `rak`/`Rak` | PASS | `Read` of lines 58–82 shows: line 61 paradigm-override opener mentions "Tillsyn" (correct — this is the project's non-use of Tillsyn, not a rak reference), line 80 closes with "durable Valv artifact" (rebrand target), fenced block opens at line 60 and closes at line 81. No `rak`/`Rak` tokens inside the fenced preamble. Case-insensitive `rak` grep across the whole file returned zero hits, which is stronger than the criterion-2 word-boundary check and covers embedded substrings (e.g. `rakish`, `drake`, `Prague`). |
| Phase ordering preserved (no insertion, deletion, or reorder) | PASS | 7 headings, ascending 1→7 (table above). Heading set matches canonical phase order declared on line 48. |
| Agent Spawn Contract is the single canonical preamble source — no per-phase duplication | PASS | 6 occurrences of the literal string: `## Agent Spawn Contract` section heading at line 52, plus cross-references from Phases 1, 2, 4, 5 and from the Per-Role Spawn Appendices section at line 170. Matches criterion 9's `>= 1` and rules out per-phase preamble duplication. |
| `_TEMPLATE/CLOSEOUT.md` is otherwise unchanged except line 29 | PASS (inferred) | `_TEMPLATE/CLOSEOUT.md` is currently untracked in git, so no committed baseline diff is available. As a proxy, diffed the scaffold-stamped copy at `drops/DROP_0_DOCS_BOOTSTRAP/CLOSEOUT.md` (committed in `6445ab9`, 2026-04-18 01:09:47) against the current `_TEMPLATE/CLOSEOUT.md`: the only differences are the `DROP_0` vs `DROP_N` title token and the placeholder-value fills (`—` vs `YYYY-MM-DD`/`<sha>`/`<gh run url>`/`<ingest run id + outcome>`). These are hand-edits the orchestrator made to the scaffold-stamped copy AFTER the template was cloned, not template-side edits. Body structure (H1, metadata block, five H2 sections in order, `Source:` line, `WIKI.md Updates` section) is identical between the two — confirming the template was not restructured beyond line 29. The source line itself (line 29) reads `- **Source:** github.com/evanmschultz/valv@main` in the current template, satisfying criterion 7b. |
| Builder worklog Unit 0.3 Round 1 section has required fields | PASS | BUILDER_WORKLOG.md lines 81–106 carry `## Unit 0.3 — Round 1` heading + `**Scope:**` (L83) + `**Files changed:**` (L84–86) + `**Mage targets run:**` (L87) + `**Acceptance self-check:**` (L88–97) + `**Design notes:**` (L98–104) + `**Hylla Feedback:**` (L105) + `**Unknowns:**` (L106). Every self-reported observed count (0/0/1/0/1/0/1/0/1/0/1/7/6) matches the on-disk counts verified in this pass numerically. |

### BUILDER_WORKLOG Cross-Check

| Builder claim (L89–97) | Observed this round | Match |
|---|---|---|
| crit 1 `Rak` count = 0 | 0 | OK |
| crit 2 `\brak\b` case-insensitive count = 0 | 0 | OK |
| crit 3 `^# Valv — Per-Drop Workflow` count = 1 | 1 | OK |
| crit 4 `DROP_1_CODE_SCAFFOLD_MAGE_CI` = 0, `DROP_1_DELETE_API_WRAPPER` = 1 | 0, 1 | OK |
| crit 5 `rak artifact` = 0, `Valv artifact` = 1 | 0, 1 | OK |
| crit 6 `rak does not use` (case-i) = 0, `Valv does not use` = 1 | 0, 1 | OK |
| crit 7 `github.com/evanmschultz/rak@main` = 0, `github.com/evanmschultz/valv@main` = 1 | 0, 1 | OK |
| crit 8 `^## Phase [1-7] — ` count = 7 | 7 | OK |
| crit 9 `Agent Spawn Contract` count = 6 | 6 | OK |

Zero drift between builder self-report and independent on-disk verification.

### Builder Design-Note Cross-Check

Spot-verified three substantive design decisions the builder called out in BUILDER_WORKLOG.md lines 98–104:

1. **Targeted `Edit` calls (four lines in WORKFLOW.md + one in CLOSEOUT.md).** Corroborated indirectly — the only rebrand surfaces are the H1 title (line 1), the example drop directory name in the tree diagram (line 18, `DROP_1_DELETE_API_WRAPPER`), the Tillsyn-override sentence (line 54, `Valv does not use any of that`), and the preamble closing line (line 80, `durable Valv artifact`). CLOSEOUT.md line 29 carries `github.com/evanmschultz/valv@main`. No stray rebrand edits elsewhere (confirmed by case-insensitive `rak` grep returning zero).
2. **Em-dash (U+2014) preserved.** H1 line 1 reads `# Valv — Per-Drop Workflow` with a literal em-dash (`xxd` shows `e2 80 94` bytes for the em-dash). The em-dash is also preserved across the phase headings at lines 93/102/112/123/132/142/154 (each uses `— ` before the phase name).
3. **Tillsyn-override fenced preamble on lines 61–80 unchanged except for the final word on line 80.** Verified — lines 61–79 read as standard Tillsyn-override + Section 0 scaffolding prose with no rebrand targets; line 80 closes with `… or any other durable Valv artifact.` — rebrand landed on the terminal token only.

### Hylla Feedback

N/A — task touched non-Go files only (`main/drops/WORKFLOW.md` and `main/drops/_TEMPLATE/CLOSEOUT.md` are markdown; Hylla today indexes Go code only). No Hylla queries attempted, no fallback needed.

### Proof Certificate

- **Premises** — (1) Every one of the nine U0.3 acceptance criteria greps returns the expected count. (2) Both files are UTF-8, BOM-free, markdown-only. (3) Phase headings appear exactly seven times in strictly ascending 1→7 order. (4) The Agent Spawn Contract fenced preamble block (lines 60–81) is internally consistent post-rebrand — no residual `rak`/`Rak` tokens orphaned inside the fenced block. (5) `_TEMPLATE/CLOSEOUT.md` is structurally unchanged except for the line-29 rebrand (reasoned via scaffold-copy proxy since the template is untracked). (6) `BUILDER_WORKLOG.md` `## Unit 0.3 — Round 1` section carries the required field set (Scope / Files changed / Mage targets / Acceptance self-check / Design notes / Hylla Feedback / Unknowns) and every self-reported count matches observed.
- **Evidence** — 13 `Grep` MCP count-mode queries (one per acceptance sub-check) against both files. Full-text `Grep -i 'rak'` sweeps across both files returning zero hits — stronger than the criterion-2 word-boundary check. `Read` of WORKFLOW.md lines 52–82 (preamble fenced block), lines 210–217 (tail), line 1 (H1 title). `file` + `xxd -l 6` for encoding + BOM probe on both files. Scaffold-copy-vs-current-template `diff` to reason about CLOSEOUT.md minimal-diff invariant (git blame unavailable because `_TEMPLATE/CLOSEOUT.md` is currently untracked). `Read` of BUILDER_WORKLOG.md lines 81–106 for worklog field-set check.
- **Trace or cases** — For each of the 9 acceptance criteria (13 sub-checks counting the two-part criteria), executed the exact grep the criterion specifies, compared observed to expected, recorded PASS. For the extras: enumerated all 7 phase headings and confirmed ascending order; read the fenced preamble block line-by-line and confirmed no `rak` orphan; ran case-insensitive full-file `rak` sweep across both files; probed encoding + BOM via `file` + `xxd`; compared scaffold-stamped CLOSEOUT copy against current template to isolate post-scaffold edits.
- **Conclusion** — All 9 U0.3 acceptance criteria pass on-disk numerically (13 of 13 sub-checks green). All five extra gates (markdown-only / UTF-8 BOM-free on both files / phase-heading ordering / preamble internal consistency / CLOSEOUT minimal-change reasoning / worklog field set) pass. Builder's PASS claim holds. Unit 0.3 Round 1 proof-QA verdict: **PASS**.
- **Unknowns** — (1) `_TEMPLATE/CLOSEOUT.md` is currently untracked in git, so the minimal-change invariant for criterion 7 is verified via a scaffold-copy-proxy diff rather than a direct committed-baseline diff. The proxy is strong enough to support PASS (body structure identical modulo hand-edited placeholders in the stamped copy), but a stricter invariant would require the template to be committed first. Routed to orchestrator — this is likely intended (the template is source-controlled at closeout), but flagged in case it matters for a future audit. (2) PLAN.md U0.3 state flip to `done` at the drop-dir `PLAN.md` level is Phase 5 exit / orchestrator scope, not proof-QA — not verified in this pass. (3) The parallel `BUILDER_QA_FALSIFICATION.md` Round 1 section may surface counterexamples this proof pass does not consider — routed to orchestrator for Phase 5 synthesis.

### Verdict

All 9 U0.3 acceptance criteria pass (13 of 13 sub-checks green, verified numerically via `Grep` MCP, counts match builder's self-report exactly). Phase heading ordering passes (seven `## Phase [1-7] — ` headings at lines 93/102/112/123/132/142/154, strictly ascending 1→7, names match the canonical phase order on line 48). Both files are UTF-8, BOM-free, markdown-only. The Agent Spawn Contract fenced preamble block (lines 60–81) is internally consistent post-rebrand with no orphan `rak`/`Rak` tokens, and a case-insensitive `rak` sweep across the whole file returns zero. `_TEMPLATE/CLOSEOUT.md` is structurally unchanged except for the line-29 rebrand (reasoned via scaffold-copy proxy — template is untracked). Builder worklog `## Unit 0.3 — Round 1` section carries every required field and zero self-report-vs-observed drift. Proof-QA verdict: PASS.
