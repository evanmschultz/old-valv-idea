# DROP_0 — DOCS BOOTSTRAP

**State:** planning
**Blocked by:** —
**Paths (expected):** `CLAUDE.md` (bare-root), `main/CLAUDE.md`, `main/drops/WORKFLOW.md`, `main/drops/_TEMPLATE/CLOSEOUT.md`, `main/PLAN.md`, `main/WIKI.md` (new), `main/LEDGER.md` (new), `main/REFINEMENTS.md` (new), `main/HYLLA_FEEDBACK.md` (new), `main/HYLLA_REFINEMENTS.md` (new), `main/WIKI_CHANGELOG.md` (new), `main/.gitignore` (append), `main/.worklog/.FROZEN` (new)
**Packages (expected):** none — docs-only drop, no Go packages touched
**PLAN.md ref:** main/PLAN.md → DROP_0 row (U0.5 will create the row in this same drop; until then this drop is self-seeded from dev direction)
**Workflow:** main/drops/WORKFLOW.md
**Started:** 2026-04-18
**Closed:** —

## Scope

Rebrand the three orchestrator docs that were copied verbatim from the `rak` project (a different Go codebase — a line-counting CLI) so they accurately describe Valv: bare-root `CLAUDE.md` (steward prompt), `main/CLAUDE.md` (work-orch prompt), and `main/drops/WORKFLOW.md` (per-drop lifecycle). Rewrite the sections of `main/CLAUDE.md` that are rak-specific (Project Structure package map, Import DAG, File Breakdown table, Tech Stack, Mage targets table) against Valv's actual layout, dependencies, and `magefile.go` targets. Bootstrap the six durable Phase 7 closeout artifacts (`WIKI.md`, `LEDGER.md`, `REFINEMENTS.md`, `HYLLA_FEEDBACK.md`, `HYLLA_REFINEMENTS.md`, `WIKI_CHANGELOG.md`) that WORKFLOW.md Phase 7 requires but that do not yet exist in Valv, so the first closeout (Drop 1) does not fail on missing files. Rewrite `main/PLAN.md` as the rak-shape ten-container drop tree seeded from `main/VALV_CLAUDE_CODE_FOCUS_PLAN.md` §6 slices (DROP_0 docs bootstrap, DROP_1 delete API wrapper, DROP_2 domain plumbing, DROP_3 schema migration, DROP_4 Claude image, DROP_5 Claude adapter, DROP_6 Claude service + CLI, DROP_7 account surface parity, DROP_8 e2e + docs, DROP_9 cleanup backlog). Freeze `main/.worklog/` as historical by appending it to `.gitignore` and dropping a `.FROZEN` marker, since the rak workflow replaces it with `main/drops/` as the tracking substrate and `magefile.go` already skips `.worklog/` from format checks. Drop 0 is an intentional dry-run of the workflow on a docs-only surface so any phase-mechanics bugs (Phase 7 file-append failures, Agent Spawn Contract preamble gaps) surface before the first code-affecting drop ships. No Go source is touched. `mage build` and `mage test` must still be runnable at drop-end even though no Go files change.

## Planner

Six atomic units. Each is docs-only (no Go package footprint). All units are independent — `blocked_by: —` across the board because they operate on disjoint file sets. Ordering is soft: U0.1/U0.2/U0.3 can land in any sequence (three rebrands), U0.4/U0.5/U0.6 can land in any sequence (three new-substrate units). The builder can pick any `todo` unit per Phase 4.

### Unit 0.1 — Rebrand bare-root `CLAUDE.md` (steward prompt)

- **State:** todo
- **Paths:** `/Users/evanschultz/Documents/Code/hylla/valv/CLAUDE.md`
- **Packages:** none — docs-only
- **Acceptance:**
  1. `grep -c 'Rak' /Users/evanschultz/Documents/Code/hylla/valv/CLAUDE.md` returns `0` (case-sensitive; all "Rak" replaced with "Valv").
  2. `grep -c '/hylla/rak/' /Users/evanschultz/Documents/Code/hylla/valv/CLAUDE.md` returns `0` (all rak filesystem paths replaced with `/hylla/valv/`).
  3. `grep -c 'Go source lands here in Drop 1' /Users/evanschultz/Documents/Code/hylla/valv/CLAUDE.md` returns `0` (stale annotation removed; Valv already has Go source under `main/cmd/valv/` and `main/internal/`).
  4. `grep -c '^# Valv — Steward Orchestrator' /Users/evanschultz/Documents/Code/hylla/valv/CLAUDE.md` returns `1` (title rebranded).
  5. `grep -c 'AGENTS.md\|VALV_ACCOUNT_SWITCH_PLAN.md\|VALV_CLAUDE_CODE_FOCUS_PLAN.md' /Users/evanschultz/Documents/Code/hylla/valv/CLAUDE.md` returns at least `1` (bare-root pointer to Valv's existing plan docs present).
  6. Steward-vs-work-orch boundary rules (never edits Go source, never runs mage, never spawns builder/QA/planning subagents for feature work) are preserved verbatim in spirit from the rak original — this is the invariant that makes this file load-bearing for DROP_1+.
- **Blocked by:** —

### Unit 0.2 — Rewrite `main/CLAUDE.md` (work-orch prompt) against Valv reality

- **State:** todo
- **Paths:** `/Users/evanschultz/Documents/Code/hylla/valv/main/CLAUDE.md`
- **Packages:** none — docs-only
- **Acceptance:**
  1. `grep -c 'Rak\b' /Users/evanschultz/Documents/Code/hylla/valv/main/CLAUDE.md` returns `0` (case-sensitive word-boundary match; all "Rak" replaced with "Valv").
  2. `grep -c 'github.com/evanmschultz/rak@main' /Users/evanschultz/Documents/Code/hylla/valv/main/CLAUDE.md` returns `0`; `grep -c 'github.com/evanmschultz/valv@main' /Users/evanschultz/Documents/Code/hylla/valv/main/CLAUDE.md` returns at least `1` (Hylla artifact ref rebranded).
  3. `grep -c '/hylla/rak' /Users/evanschultz/Documents/Code/hylla/valv/main/CLAUDE.md` returns `0` (all filesystem paths use `/hylla/valv/`).
  4. **Package map rewritten.** `grep -c 'internal/counting\|internal/fileset\|internal/lang\|internal/render\|internal/summary\|internal/tokens' /Users/evanschultz/Documents/Code/hylla/valv/main/CLAUDE.md` returns `0` (rak-only package names gone). `grep -c 'internal/domain\|internal/adapters\|internal/services\|internal/cli\|internal/tui' /Users/evanschultz/Documents/Code/hylla/valv/main/CLAUDE.md` returns at least `5` (Valv's actual package roots present).
  5. **Tech Stack rewritten against `main/go.mod`.** `grep -c 'tiktoken-go/tokenizer\|golang.org/x/sync/errgroup' /Users/evanschultz/Documents/Code/hylla/valv/main/CLAUDE.md` returns `0` (rak-only deps removed — Valv has neither in go.mod). `grep -c 'charm.land/fang/v2\|charm.land/bubbletea/v2\|charm.land/lipgloss/v2\|modernc.org/sqlite\|testcontainers\|modelcontextprotocol/go-sdk' /Users/evanschultz/Documents/Code/hylla/valv/main/CLAUDE.md` returns at least `4` (Valv's actual v2 Charm prefix, pure-Go SQLite driver, testcontainers, MCP SDK present — note: Docker SDK is NOT in go.mod, Valv shells out via `exec.Command` to the `docker` CLI, so the Tech Stack must not claim a Docker SDK dep).
  6. **Mage targets table rewritten against `main/magefile.go`.** `grep -c 'mage install\|mage format\|mage lint\|mage ci\|mage coverage\|mage plan-check' /Users/evanschultz/Documents/Code/hylla/valv/main/CLAUDE.md` returns `0` (rak-only targets removed — none exist in Valv's magefile). `grep -c 'mage test\b\|mage build\b\|mage testPkg\|mage integration\|mage golden\|mage goldenUpdate\|mage run\|mage dev:' /Users/evanschultz/Documents/Code/hylla/valv/main/CLAUDE.md` returns at least `6` (Valv's actual mage targets from `main/magefile.go` present; `Test` already runs gofumpt + tests + 70% coverage inline).
  7. **Import DAG rewritten.** `grep -ci 'counting\|fileset\|lang' /Users/evanschultz/Documents/Code/hylla/valv/main/CLAUDE.md` (rak DAG terms) returns `0`. A DAG description referencing `domain`, `adapters`, `services`, `cli`, `tui` appears (verified by `grep -c 'domain.*adapters\|adapters.*services\|services.*cli' /Users/evanschultz/Documents/Code/hylla/valv/main/CLAUDE.md` returning at least `1`, or an equivalent layered-description phrase).
  8. **Per-file LOC table removed.** `grep -c '^| File | Role | LOC |' /Users/evanschultz/Documents/Code/hylla/valv/main/CLAUDE.md` returns `0` (the rak File Breakdown table is inapplicable to Valv and is deleted, not rewritten).
  9. **`mage install` prohibition removed.** `grep -c 'NEVER run .mage install\|mage install' /Users/evanschultz/Documents/Code/hylla/valv/main/CLAUDE.md` returns `0` (Valv has no such target; the prohibition is rak-specific).
  10. **Authoritative-deferral statement to AGENTS.md present.** The new `main/CLAUDE.md` contains a section or sentence that defers to `main/AGENTS.md` for Product Direction, Platform Scope, Delivery Standards, and Repository Standards — i.e. `grep -c 'AGENTS.md' /Users/evanschultz/Documents/Code/hylla/valv/main/CLAUDE.md` returns at least `1` AND that reference frames AGENTS.md as authoritative for the cross-cutting rules CLAUDE.md does not own. CLAUDE.md owns: drop workflow routing, orchestrator role boundaries, Go naming rules (the 12-rule list), commit format, safety.
  11. **Workflow link preserved.** `grep -c 'main/drops/WORKFLOW.md' /Users/evanschultz/Documents/Code/hylla/valv/main/CLAUDE.md` returns at least `1` (Phase 1-7 mechanics stay delegated to WORKFLOW.md).
  12. **Agent bindings table intact.** `grep -c 'go-builder-agent\|go-planning-agent\|go-qa-proof-agent\|go-qa-falsification-agent' /Users/evanschultz/Documents/Code/hylla/valv/main/CLAUDE.md` returns at least `4` (role-to-agent mapping preserved).
- **Blocked by:** —

### Unit 0.3 — Rebrand `main/drops/WORKFLOW.md` + fix `_TEMPLATE/CLOSEOUT.md` rak reference

- **State:** todo
- **Paths:** `/Users/evanschultz/Documents/Code/hylla/valv/main/drops/WORKFLOW.md`, `/Users/evanschultz/Documents/Code/hylla/valv/main/drops/_TEMPLATE/CLOSEOUT.md`
- **Packages:** none — docs-only
- **Acceptance:**
  1. `grep -c 'Rak\b' /Users/evanschultz/Documents/Code/hylla/valv/main/drops/WORKFLOW.md` returns `0` (all "Rak" in prose replaced with "Valv").
  2. `grep -c '^# Valv — Per-Drop Workflow' /Users/evanschultz/Documents/Code/hylla/valv/main/drops/WORKFLOW.md` returns `1` (title rebranded).
  3. `grep -c 'DROP_1_CODE_SCAFFOLD_MAGE_CI' /Users/evanschultz/Documents/Code/hylla/valv/main/drops/WORKFLOW.md` returns `0`; `grep -c 'DROP_1_DELETE_API_WRAPPER' /Users/evanschultz/Documents/Code/hylla/valv/main/drops/WORKFLOW.md` returns at least `1` (example drop name aligned with VALV_CLAUDE_CODE_FOCUS_PLAN.md §6.1).
  4. `grep -c 'rak artifact' /Users/evanschultz/Documents/Code/hylla/valv/main/drops/WORKFLOW.md` returns `0`; `grep -c 'Valv artifact' /Users/evanschultz/Documents/Code/hylla/valv/main/drops/WORKFLOW.md` returns at least `1` (Agent Spawn Contract preamble last line fixed per the documented U0.3 bug — this is the bug the orchestrator prompt's preamble already corrects inline for this spawn).
  5. `grep -ci 'rak does not use' /Users/evanschultz/Documents/Code/hylla/valv/main/drops/WORKFLOW.md` returns `0`; `grep -c 'Valv does not use' /Users/evanschultz/Documents/Code/hylla/valv/main/drops/WORKFLOW.md` returns at least `1` (Tillsyn-override sentence rebranded).
  6. `grep -c 'github.com/evanmschultz/rak@main' /Users/evanschultz/Documents/Code/hylla/valv/main/drops/_TEMPLATE/CLOSEOUT.md` returns `0`; `grep -c 'github.com/evanmschultz/valv@main' /Users/evanschultz/Documents/Code/hylla/valv/main/drops/_TEMPLATE/CLOSEOUT.md` returns at least `1` (template bug at line 29 fixed so future drop-scaffolds do not regress).
  7. Phase order (Plan → Plan QA → Discuss + Cleanup → Build → Build QA → Verify → Closeout) is preserved verbatim in phase semantics — no phase added, removed, or reordered. Verified by `grep -c '^## Phase [1-7] — ' /Users/evanschultz/Documents/Code/hylla/valv/main/drops/WORKFLOW.md` returning `7`.
  8. Agent Spawn Contract preamble remains the single canonical source: `grep -c 'Agent Spawn Contract' /Users/evanschultz/Documents/Code/hylla/valv/main/drops/WORKFLOW.md` returns at least `1`; no per-phase duplication of the preamble text.
- **Blocked by:** —

### Unit 0.4 — Bootstrap six durable Phase 7 closeout artifacts

- **State:** todo
- **Paths:** `/Users/evanschultz/Documents/Code/hylla/valv/main/WIKI.md` (new), `/Users/evanschultz/Documents/Code/hylla/valv/main/LEDGER.md` (new), `/Users/evanschultz/Documents/Code/hylla/valv/main/REFINEMENTS.md` (new), `/Users/evanschultz/Documents/Code/hylla/valv/main/HYLLA_FEEDBACK.md` (new), `/Users/evanschultz/Documents/Code/hylla/valv/main/HYLLA_REFINEMENTS.md` (new), `/Users/evanschultz/Documents/Code/hylla/valv/main/WIKI_CHANGELOG.md` (new)
- **Packages:** none — docs-only
- **Acceptance:**
  1. `ls /Users/evanschultz/Documents/Code/hylla/valv/main/WIKI.md /Users/evanschultz/Documents/Code/hylla/valv/main/LEDGER.md /Users/evanschultz/Documents/Code/hylla/valv/main/REFINEMENTS.md /Users/evanschultz/Documents/Code/hylla/valv/main/HYLLA_FEEDBACK.md /Users/evanschultz/Documents/Code/hylla/valv/main/HYLLA_REFINEMENTS.md /Users/evanschultz/Documents/Code/hylla/valv/main/WIKI_CHANGELOG.md` exits `0` (all six files exist).
  2. Each of the six files has a level-1 heading matching its filename (without the `.md` extension) — verified by `head -1 <path>` returning `# WIKI`, `# LEDGER`, `# REFINEMENTS`, `# HYLLA_FEEDBACK`, `# HYLLA_REFINEMENTS`, `# WIKI_CHANGELOG` respectively (or the Title-Cased equivalents `# Wiki`, `# Ledger`, `# Refinements`, `# Hylla Feedback`, `# Hylla Refinements`, `# Wiki Changelog` — either casing acceptable; builder picks one and is consistent).
  3. Each file has a one-line purpose subheading directly after the title (e.g. "Living best-practice snapshot for Valv." for `WIKI.md`) — verified by `wc -l` returning at least `3` on every file (title + blank + purpose line minimum).
  4. **No `## DROP_0` stub headings** that would collide with real Phase 7 appends — verified by `grep -c '^## DROP_0' <each-path>` returning `0`. The files are empty scaffolds, not pre-populated with drop entries.
  5. Each file is committable as-is (valid markdown, no trailing TODO markers that would fail `mage test`'s format check — though since none of these are Go files, `mage test` is indifferent; the check is cosmetic markdown validity).
  6. WORKFLOW.md Phase 7 append targets (lines `main/HYLLA_FEEDBACK.md`, `main/REFINEMENTS.md`, `main/HYLLA_REFINEMENTS.md`, `main/LEDGER.md`, `main/WIKI_CHANGELOG.md`, `main/WIKI.md`) all resolve to existing files after this unit — verified by the `ls` in step 1.
- **Blocked by:** —

### Unit 0.5 — Rewrite `main/PLAN.md` as the Valv drop tree (10 containers)

- **State:** todo
- **Paths:** `/Users/evanschultz/Documents/Code/hylla/valv/main/PLAN.md`
- **Packages:** none — docs-only
- **Acceptance:**
  1. `grep -c '^# Valv' /Users/evanschultz/Documents/Code/hylla/valv/main/PLAN.md` returns at least `1` (new top-level Valv plan title; rak-derived title gone).
  2. `grep -cE '^\| *DROP_[0-9]+' /Users/evanschultz/Documents/Code/hylla/valv/main/PLAN.md` returns exactly `10` (ten drop rows — the table body). Row names, in order: `DROP_0_DOCS_BOOTSTRAP`, `DROP_1_DELETE_API_WRAPPER`, `DROP_2_DOMAIN_PLUMBING_PROVIDER_CLAUDE`, `DROP_3_SCHEMA_MIGRATION_COMPOSITE_BINDING`, `DROP_4_CLAUDE_DOCKER_IMAGE`, `DROP_5_CLAUDE_PROVIDER_ADAPTER`, `DROP_6_CLAUDE_SERVICE_AND_CLI`, `DROP_7_ACCOUNT_SURFACE_PARITY`, `DROP_8_E2E_AND_DOCS`, `DROP_9_CLEANUP_BACKLOG`.
  3. Each row has columns for at minimum: drop number, name, state, blocked_by, one-line scope, drop-dir link. Verified by `grep -cE '^\| *DROP_[0-9]+ *\|' /Users/evanschultz/Documents/Code/hylla/valv/main/PLAN.md` returning `10` (pipe-bounded cell format).
  4. **DROP_0 row shows `state: building`** (active drop): `grep 'DROP_0_DOCS_BOOTSTRAP' /Users/evanschultz/Documents/Code/hylla/valv/main/PLAN.md | grep -c 'building'` returns at least `1`. (Phase 7 of this drop will flip it to `done`; this unit only seeds `building`.)
  5. **DROP_1 through DROP_9 show `state: todo`** (or a synonym like `planning`): `grep -cE 'DROP_[1-9]' /Users/evanschultz/Documents/Code/hylla/valv/main/PLAN.md` returns at least `9`, and none of those rows carry a `state: done` cell — verified by `grep -cE 'DROP_[1-9].*state: done' /Users/evanschultz/Documents/Code/hylla/valv/main/PLAN.md` returning `0`.
  6. **`blocked_by` edges reflect §6 ordering guarantee** (from `main/VALV_CLAUDE_CODE_FOCUS_PLAN.md` §6, "§6.1 → §6.2 → §6.2a → §6.3 → §6.4 → §6.5 → §6.6 → §6.7"):
     - DROP_0: `blocked_by: —`
     - DROP_1: `blocked_by: —` (DELETE_API_WRAPPER can land independent of DROP_0 in principle; §6.1 declares subtractive-only)
     - DROP_2: `blocked_by: DROP_1`
     - DROP_3: `blocked_by: DROP_2`
     - DROP_4: `blocked_by: DROP_2` (image + domain plumbing, not yet dependent on schema migration)
     - DROP_5: `blocked_by: DROP_2, DROP_4`
     - DROP_6: `blocked_by: DROP_3, DROP_5` (schema + adapter both required before service/CLI)
     - DROP_7: `blocked_by: DROP_6`
     - DROP_8: `blocked_by: DROP_7`
     - DROP_9: `blocked_by: DROP_8`
     Builder must render these edges as typed cells. Verified by `grep -cE 'blocked_by:' /Users/evanschultz/Documents/Code/hylla/valv/main/PLAN.md` returning at least `10`.
  7. **Per-drop dir link present for DROP_0**: `grep -c 'main/drops/DROP_0_DOCS_BOOTSTRAP' /Users/evanschultz/Documents/Code/hylla/valv/main/PLAN.md` returns at least `1`. Future-drop dir links may be stubs (directory does not yet exist) but the path convention is encoded.
  8. **The 47k rak-era / Track-F content is gone.** `wc -l /Users/evanschultz/Documents/Code/hylla/valv/main/PLAN.md` returns a line count well below the previous size (target: under 500 lines — the new file is a compact drop-tree, not a full execution plan; execution details live in per-drop `PLAN.md` files plus `VALV_CLAUDE_CODE_FOCUS_PLAN.md`). `grep -cE 'Track [A-F]|Agent 5' /Users/evanschultz/Documents/Code/hylla/valv/main/PLAN.md` returns `0` (Track-F / Agent-5 shape vocabulary from the old pre-rak-workflow doc is fully superseded).
  9. File links to `main/drops/WORKFLOW.md` and `main/VALV_CLAUDE_CODE_FOCUS_PLAN.md` as the durable companions — `grep -c 'WORKFLOW.md\|VALV_CLAUDE_CODE_FOCUS_PLAN.md' /Users/evanschultz/Documents/Code/hylla/valv/main/PLAN.md` returns at least `2`.
- **Blocked by:** —

### Unit 0.6 — Freeze `main/.worklog/` with a marker file

- **State:** todo
- **Paths:** `/Users/evanschultz/Documents/Code/hylla/valv/main/.worklog/.FROZEN` (new)
- **Packages:** none — docs-only
- **Acceptance:**
  1. `ls /Users/evanschultz/Documents/Code/hylla/valv/main/.worklog/.FROZEN` exits `0` (marker file exists).
  2. The marker file explains that `main/drops/` is the new tracking substrate as of DROP_0 and that `.worklog/` is historical — verified by `grep -c 'main/drops/' /Users/evanschultz/Documents/Code/hylla/valv/main/.worklog/.FROZEN` returning at least `1` AND `grep -ci 'frozen\|historical' /Users/evanschultz/Documents/Code/hylla/valv/main/.worklog/.FROZEN` returning at least `1`.
  3. **`.worklog/` is ALREADY in `main/.gitignore` (line 4)** — orchestrator seed's "append to .gitignore" step is a no-op and this unit MUST NOT add a duplicate line. Verified by `grep -cE '^\.worklog/$' /Users/evanschultz/Documents/Code/hylla/valv/main/.gitignore` returning exactly `1` (unchanged: one line, not two).
  4. `main/magefile.go:311` `case ".git", ".tmp", ".worklog":` in `goFiles` remains intact (verified by `grep -c '\.worklog' /Users/evanschultz/Documents/Code/hylla/valv/main/magefile.go` returning at least `1`) — the marker file lives under `.worklog/` and is therefore never seen by `mage test`'s format check, so no regression risk.
  5. **No deletion of existing `.worklog/` content** — historical logs stay on disk for archival reference. Verified by `ls /Users/evanschultz/Documents/Code/hylla/valv/main/.worklog/ | wc -l` returning a count greater than `1` (at least the pre-existing `.md` + `.log` files plus the new `.FROZEN` marker).
- **Blocked by:** —

## Notes

Orchestrator-override note (Phase 1, Round 1): this drop was scaffolded from a **steward orchestrator** session at the bare-root per an explicit dev override, because the file that defines the steward-vs-work-orch boundary (`valv/CLAUDE.md`, currently copied verbatim from rak) is itself the subject of U0.1 in this drop. Starting with Drop 1, the rule is enforced normally: work orchestrators run from `main/`, stewards run from the bare-root and never spawn planner / builder / QA subagents.

Planner deviations from the orchestrator's six-unit seed (Phase 1, Round 1):

1. **U0.6 gitignore step is a no-op.** The orchestrator seed said to "append `.worklog/` to `main/.gitignore`". Direct read of `main/.gitignore` line 4 shows `.worklog/` is already present. U0.6 acceptance criterion 3 explicitly guards against a duplicate line. Only the `.FROZEN` marker file is genuinely new work in U0.6.
2. **U0.2 Tech Stack must not inherit rak-only deps.** The orchestrator seed listed "Docker SDK" as a Valv dep — `main/go.mod` does not contain one. Valv shells out to the `docker` CLI via `exec.Command` (see `main/magefile.go` line 213 and the codex adapter). Similarly `tiktoken-go/tokenizer` and `golang.org/x/sync/errgroup` are rak-era deps not present in Valv's go.mod. U0.2 acceptance criterion 5 explicitly forbids inheriting them.
3. **U0.2 Charm library import path.** Valv uses `charm.land/fang/v2`, `charm.land/bubbletea/v2`, `charm.land/lipgloss/v2`, `charm.land/bubbles/v2` (verified in `main/go.mod`), not the older `github.com/charmbracelet/` v1 paths. Tech Stack rewrite must use the v2 `charm.land/` prefix.
4. **U0.2 Mage targets are Valv-specific.** Rak's table (`mage ci`, `mage format`, `mage lint`, `mage coverage`, `mage install`, `mage plan-check`) is fully inapplicable. Valv's actual targets (verified in `main/magefile.go`): `Build`, `Test`, `TestPkg`, `Integration`, `Golden`, `GoldenUpdate`, `Run`, `Dev.Home`, `Dev.Reset`, `Dev.Clean`, `Dev.Run`. `Test` already runs gofumpt + tests + 70% inline coverage — no separate `ci`/`format`/`lint` targets exist. The "NEVER run `mage install`" rule is rak-specific and deleted.
5. **U0.4 stub format is title + purpose only.** The orchestrator seed suggested a `## <Current drop>` placeholder subheading; acceptance criterion 4 rejects that because it would collide with real Phase 7 appends like `## DROP_1 …`. Stubs are title + blank + one-line purpose only.
6. **Ordering is soft, not blocked.** All six units are independent — no `blocked_by` edges between them. The builder can pick any `todo` unit per Phase 4.
