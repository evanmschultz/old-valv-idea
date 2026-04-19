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
