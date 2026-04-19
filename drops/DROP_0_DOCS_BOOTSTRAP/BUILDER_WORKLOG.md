# DROP_0 — Builder Worklog

Append a `## Unit N.M — Round K` section per build attempt. See `main/drops/WORKFLOW.md` § "Phase 4 — Build (per unit)" for what each section should contain.

<!-- First unit entry appended by the builder subagent during Phase 4. -->

## Unit 0.4 — Round 1

- **Scope:** six new Phase-7 durable artifacts at main/ root.
- **Files created:** WIKI.md, LEDGER.md, REFINEMENTS.md, HYLLA_FEEDBACK.md, HYLLA_REFINEMENTS.md, WIKI_CHANGELOG.md.
- **Mage targets run:** none (docs-only; no Go code touched — mage build + mage test will run at Phase 6 per WORKFLOW.md).
- **Design notes:** Each file is the minimal 3-line scaffold mandated by the spec — ALL-CAPS heading, blank line, one-line purpose subheading — written via the `Write` tool to guarantee no leading U+FEFF BOM. No `## DROP_0` stubs pre-populated; first Phase-7 closeout appends will create those headings cleanly.

## Unit 0.6 — Round 1

- **Scope:** freeze main/.worklog/ with a marker file.
- **Files created:** main/.worklog/.FROZEN
- **Mage targets run:** none (docs-only; no Go code touched)
- **Design notes:** Marker is 17 lines of plain UTF-8 prose written via the `Write` tool (no BOM). Content covers: (a) the directory is frozen as of DROP_0, (b) pre-existing files stay on disk as historical archive, (c) `main/drops/` is the new coordination substrate going forward, (d) pointer to `main/drops/WORKFLOW.md` for the full per-drop lifecycle, (e) note that `.worklog/` is gitignored so the marker is local-only. Per the spec caveat, the marker file will never be committed — acceptance is disk-state, not git-state. No edits to `.gitignore` (line 4 `.worklog/` already present) or `magefile.go` (line 311 `.worklog` skip intact). Verified all 5 acceptance criteria post-write: `.FROZEN` exists (`ls` exit 0); `grep -c 'main/drops/'` returns `3` (≥ 1 — pass); `grep -ci 'frozen\|historical'` returns `3` (≥ 1 — pass); `grep -cE '^\.worklog/$' .gitignore` returns exactly `1` (unchanged — pass); `grep -c '\.worklog' magefile.go` returns `1` (unchanged — pass); `ls -1A .worklog | wc -l` returns `49` (48 pre-existing + 1 new marker, ≥ 2 — pass). Single-step state flip todo→done per spec's explicit allowance for one-shot marker-file units.

## Unit 0.1 — Round 1

- **Scope:** rebrand bare-root `CLAUDE.md` (steward orchestrator prompt) from rak → Valv.
- **Files changed:** `/Users/evanschultz/Documents/Code/hylla/valv/CLAUDE.md` (full rewrite via `Write`).
- **Mage targets run:** none (docs-only; no Go code touched — `mage build` + `mage test` run at Phase 6 per WORKFLOW.md).
- **Acceptance self-check (all 7 greps via Grep tool, counts observed):**
  1. `Rak` (case-sensitive) → `0` (expected `0`, pass).
  2. `\brak\b` (case-insensitive word-boundary) → `0` (expected `0`, pass).
  3. `/hylla/rak` → `0` (expected `0`, pass).
  4. `Go source lands here in Drop 1` → `0` (expected `0`, pass).
  5. `^# Valv — Steward Orchestrator` → `1` (expected `1`, pass; em-dash U+2014 preserved).
  6. `AGENTS.md\|VALV_ACCOUNT_SWITCH_PLAN.md\|VALV_CLAUDE_CODE_FOCUS_PLAN.md` → `5` (expected ≥ `1`, pass — `AGENTS.md` and `VALV_ACCOUNT_SWITCH_PLAN.md` cited, both verified to exist in bare-root via `ls`).
  7. Boundary invariants:
     - `^- \*\*Never edits Go source` → `1` (expected ≥ `1`, pass).
     - `^- \*\*Never runs mage` → `1` (expected ≥ `1`, pass).
     - `^- \*\*Never commits or pushes` → `1` (expected ≥ `1`, pass).
     - `^## What The Steward Does NOT Do` → `1` (expected ≥ `1`, pass).
- **Design notes:** Chose `Write` (full rewrite) over piecewise `Edit` because the file had ~40 rak references spread across title, prose, tree diagram, paths, examples, and commit-scope placeholders — a clean rewrite was less error-prone than serial edits. Preserved the structural shape verbatim: H1 title → "Two Orchestrator Shapes" → Git Setup / tree diagram / git commands → Steward Responsibilities → "What The Steward Does NOT Do" (four canonical "Never" bullets intact, including the two beyond the three checked — "Never runs hylla_ingest" and "Never spawns builder/QA/planning subagents") → Coordination Model → Evidence Sources → Git Commit Format → Safety → Recovery. Added a new "Bare-Root Plan Docs" section after "Two Orchestrator Shapes" to satisfy criterion 6 with context (lists `AGENTS.md`, `VALV_ACCOUNT_SWITCH_PLAN.md`, `VALV_REPO_PLAN.md`, `valv_architecture_notes.md`, bare-root `PLAN.md` — all verified present via `ls`). Updated the tree diagram to reflect Valv reality: removed `FETCH_HEAD` + `packed-refs` references (they're transient/not always present) and added Valv's bare-root plan-doc files; replaced the stale "Go source lands here in Drop 1" annotation with concrete entries (`cmd/valv/`, `internal/`, `magefile.go`) matching Valv's actual layout. The third example commit under "Git Commit Format" — originally `chore(plan): reconcile plan.md with tillsyn after drop 1 lands` — was rebranded to `chore(plan): reconcile plan.md with drop tree after drop 1 lands` to drop the rak-era tillsyn reference (the bare-root `CLAUDE.md` itself, like the rest of Valv, does not use tillsyn). UTF-8 em-dash (U+2014) preserved in title and prose.
- **Unknowns:** none — all 7 acceptance criteria verified numerically, file is syntactically valid markdown, no rak references remain, steward-vs-work-orch boundary invariants preserved as checked bullets under the preserved section heading.
