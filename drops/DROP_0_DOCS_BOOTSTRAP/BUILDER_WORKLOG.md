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

## Unit 0.5 — Round 1

- **Scope:** rewrite `main/PLAN.md` from its pre-rak-workflow Track-A/Track-F shape (~995 lines) into the compact ten-container Valv drop-tree index (38 lines) mandated by U0.5.
- **Files changed:** `/Users/evanschultz/Documents/Code/hylla/valv/main/PLAN.md` (full rewrite via `Write`).
- **Mage targets run:** none (docs-only; no Go code touched — `mage build` + `mage test` run at Phase 6 per WORKFLOW.md).
- **Acceptance self-check (all 10 criteria, observed values):**
  1. `grep -c '^# Valv' PLAN.md` → `1` (expected `>=1`, pass — H1 is `# Valv — Drop-Tree Index`).
  2. `grep -cE '^\| *DROP_[0-9]+' PLAN.md` → `10` (expected exactly `10`, pass).
  3. `grep -cE '^\| *DROP_[0-9]+_' PLAN.md` → `10` (expected `10`, pass — name-cell format `| DROP_N_<SUFFIX> |`).
  4. `grep 'DROP_0_DOCS_BOOTSTRAP' | grep -c 'building'` → `1` (expected `>=1`, pass — DROP_0 row carries `state: building`).
  5a. `grep -cE 'DROP_[1-9]' PLAN.md` → `11` (expected `>=9`, pass — 9 data rows + 2 prose references in the intro).
  5b. `grep -cE 'DROP_[1-9].*state: done' PLAN.md` → `0` (expected `0`, pass — no premature `done` marks).
  6. Ten `blocked_by` edge greps (each expected `>=1`, all observed `1`):
     - DROP_0 `blocked_by: —` → `1` (em-dash U+2014).
     - DROP_1 `blocked_by: DROP_0` → `1`.
     - DROP_2 `blocked_by: DROP_1` → `1`.
     - DROP_3 `blocked_by: DROP_2` → `1`.
     - DROP_4 `blocked_by: DROP_3` → `1`.
     - DROP_5 `blocked_by: DROP_4` → `1`.
     - DROP_6 `blocked_by: DROP_5` → `1`.
     - DROP_7 `blocked_by: DROP_6` → `1`.
     - DROP_8 `blocked_by: DROP_7` → `1`.
     - DROP_9 `blocked_by: DROP_8` → `1`.
  7. `awk -F'[_|]' '/^\| *DROP_[0-9]+_/ {gsub(/[^0-9]/,"",$3); print $3}' PLAN.md | awk 'BEGIN{prev=-1} {if ($1 != prev+1) exit 1; prev=$1} END{if (prev != 9) exit 1}'` → exit `0` (expected `0`, pass). Extractor emitted the sequence `0,1,2,3,4,5,6,7,8,9` exactly.
  8. `grep -c 'main/drops/DROP_0_DOCS_BOOTSTRAP' PLAN.md` → `1` (expected `>=1`, pass — concrete reference in the Reading Order section).
  9. `wc -l PLAN.md` → `38` (expected `< 500`, pass). `grep -cE 'Track [A-F]|Agent 5' PLAN.md` → `0` (expected `0`, pass — no pre-rak-workflow vocabulary).
  10. `grep -c 'WORKFLOW.md\|VALV_CLAUDE_CODE_FOCUS_PLAN.md' PLAN.md` → `15` (expected `>=2`, pass — both durable companions cited extensively).
- **Design notes:**
  - **Column shape.** Chose the five required columns (`name | state | blocked_by | scope | drop_dir_link`) with the drop name embedded in the first cell (`| DROP_0_DOCS_BOOTSTRAP |`) per the spec's mandate — splitting the drop number into its own cell would fail criteria 2, 3, 6, and 7 (all anchor on the pipe-bounded name cell). Header row is `| Drop | State | Blocked By | Scope | Dir |` — reads naturally in rendered markdown but doesn't match the data-row greps.
  - **Dir link format.** Used relative-from-PLAN links (`[dir](drops/DROP_0_DOCS_BOOTSTRAP/)`) because PLAN.md itself lives at `main/PLAN.md`, so the `drops/…` prefix is correct for link resolution. To satisfy criterion 8's literal substring `main/drops/DROP_0_DOCS_BOOTSTRAP`, added one prose mention of the absolute path in the Reading Order section ("The active drop today is `main/drops/DROP_0_DOCS_BOOTSTRAP/`"). This keeps link semantics correct AND passes the grep.
  - **DROP_0 state choice.** Seeded `state: building` per the prompt (U0.5 is running inside the drop; Phase 7 flips to `done` later). Criterion 4 asserts at least one `building` match on the DROP_0 row; observed exactly `1`.
  - **DROP_1..DROP_9 dir link stubs.** Directories do not yet exist — they get scaffolded from `main/drops/_TEMPLATE/` at each drop's Phase 1. The criterion only requires the *path convention* encoded, not live directories; noted explicitly in the Conventions section so readers understand the forward-looking stub behavior.
  - **Scope judgment calls for DROP_8 / DROP_9.** `VALV_CLAUDE_CODE_FOCUS_PLAN.md` §6.7 explicitly defines DROP_8 (E2E + docs rewrite + OpenAI-compat scrub + v1-Claude tag) — scope phrased as one sentence mirroring the §6.7 bullets. DROP_9 is not an explicit §6 item; the spec says "infer from the drop name and supporting context." Mapped it onto the `VALV_CLAUDE_CODE_FOCUS_PLAN.md` §8 "Cleanup, Idiomatic Go, Refactor Backlog" section — that is the natural home for dynamic Claude version resolver work, interface placement cleanup, the `manage.go` and `extended_test.go` splits, and the mock-heavy-test audit. Phrased the scope as a single cleanup pass, naming the largest concrete items so readers can see the drop has a bounded shape rather than an open-ended "everything left over" handle.
  - **DROP_3 name.** Spec row name is `DROP_3_SCHEMA_MIGRATION_COMPOSITE_BINDING` — preserved verbatim. Underlying scope (composite PK + `BindingRepository` signature change + forward-only rebuild) is §6.2a in the focus plan; the drop name reads cleanly without a literal "6.2a" numeric suffix.
  - **Line count aggression.** 38 lines (intro + reading order + conventions + table + maintenance). Well under the 500-line target. Every row's scope cell is a single sentence — any additional prose belongs in the drop dir's own `PLAN.md`.
  - **Companion references.** `WORKFLOW.md` mentioned 5 times, `VALV_CLAUDE_CODE_FOCUS_PLAN.md` mentioned 10 times (total 15 matches on the `WORKFLOW.md\|VALV_CLAUDE_CODE_FOCUS_PLAN.md` grep) — criterion 10 only requires `>=2`, but each drop's scope sentence anchors its derivation back to the focus plan section that seeded it, which is load-bearing for future readers trying to reconcile this index against the focus plan.
  - **No `hylla_ingest` or mage invocation.** Docs-only per U0.5 spec. File was written via the `Write` tool; UTF-8 without BOM; no HTML; markdown only.
- **Hylla Feedback:** N/A — task touched non-Go files only.
- **Unknowns:** none — all 10 acceptance criteria verified numerically, awk pipeline exit 0 confirmed, file is syntactically valid markdown, drop-tree shape matches the focus plan's §6 slice ordering, DROP_8 / DROP_9 scope text documented above as design judgment calls in case a later planner round wants to sharpen them.

## Unit 0.3 — Round 1

- **Scope:** rebrand `main/drops/WORKFLOW.md` from "Rak" to "Valv" (title + prose + example drop name) and fix the `github.com/evanmschultz/rak@main` Hylla artifact ref on line 29 of `main/drops/_TEMPLATE/CLOSEOUT.md`. Structural content (phase order, file lifecycles, Agent Spawn Contract preamble structure, Per-Role Spawn Appendices) preserved verbatim.
- **Files changed:**
  - `/Users/evanschultz/Documents/Code/hylla/valv/main/drops/WORKFLOW.md` (four targeted `Edit` calls — H1 title, example drop directory name in the tree diagram, two rak-branding sentences inside the Agent Spawn Contract section, and the closing Section-0 reminder line).
  - `/Users/evanschultz/Documents/Code/hylla/valv/main/drops/_TEMPLATE/CLOSEOUT.md` (single `Edit` on the Hylla Ingest Source line).
- **Mage targets run:** none (docs-only; no Go code touched — `mage build` + `mage test` run at Phase 6 per WORKFLOW.md).
- **Acceptance self-check (all 9 criteria, observed via `Grep` count mode):**
  1. `grep -c 'Rak' WORKFLOW.md` → `0` (expected `0`, pass).
  2. `grep -ciE '\brak\b' WORKFLOW.md` → `0` (expected `0`, pass).
  3. `grep -c '^# Valv — Per-Drop Workflow' WORKFLOW.md` → `1` (expected `1`, pass — em-dash U+2014 preserved).
  4. `grep -c 'DROP_1_CODE_SCAFFOLD_MAGE_CI' WORKFLOW.md` → `0` (expected `0`, pass); `grep -c 'DROP_1_DELETE_API_WRAPPER' WORKFLOW.md` → `1` (expected `>=1`, pass — example drop name aligned with focus plan §6.1).
  5. `grep -c 'rak artifact' WORKFLOW.md` → `0` (expected `0`, pass); `grep -c 'Valv artifact' WORKFLOW.md` → `1` (expected `>=1`, pass — Agent Spawn Contract preamble last line rebranded).
  6. `grep -ci 'rak does not use' WORKFLOW.md` → `0` (expected `0`, pass); `grep -c 'Valv does not use' WORKFLOW.md` → `1` (expected `>=1`, pass — Tillsyn-override sentence rebranded).
  7. `grep -c 'github.com/evanmschultz/rak@main' _TEMPLATE/CLOSEOUT.md` → `0` (expected `0`, pass); `grep -c 'github.com/evanmschultz/valv@main' _TEMPLATE/CLOSEOUT.md` → `1` (expected `>=1`, pass — template line-29 bug fixed).
  8. `grep -c '^## Phase [1-7] — ' WORKFLOW.md` → `7` (expected `7`, pass — phase order preserved exactly).
  9. `grep -c 'Agent Spawn Contract' WORKFLOW.md` → `6` (expected `>=1`, pass — single canonical preamble source preserved; the six occurrences are the section heading plus the five cross-references from Phases 1, 2, 4, 5, and the "Per-Role Spawn Appendices" pointer).
- **Design notes:**
  - **Targeted `Edit` over `Write`.** The rebrand surface in WORKFLOW.md is tiny — four lines total (1, 18, 54, 80) plus one line in `_TEMPLATE/CLOSEOUT.md`. Chose five surgical `Edit` calls over a full rewrite because the file is 217 lines and a rewrite risks UTF-8 artifact drift (em-dash U+2014 appears in the H1 title and throughout; Write-tool round-trip tends to normalize things I did not intend to normalize).
  - **Brand-token audit before editing.** Ran `Grep` for `[Rr]ak`, `DROP_1_CODE_SCAFFOLD`, `evanmschultz/rak`, `/hylla/rak|cmd/rak` across WORKFLOW.md to enumerate every rebrand target. Only the four line locations above matched — no `/hylla/rak` paths, no `cmd/rak` binary references, no `evanmschultz/rak` URLs in WORKFLOW.md. The compound-identifier forms called out in the prompt ("rak artifact", "rak orchestrator", "rak source code") resolved to two tokens on one line (line 54: "Rak does not use" + "rak orchestrator") plus one token on line 80 ("durable rak artifact"). This confirmed `Edit` with `replace_all: false` on each distinct full-line context was safe — no substring-collision risk from "Raku" / "strake" / "drake" / etc.
  - **Ordering of edits.** Performed all five `Edit` calls in a single message (parallel tool batch) since each targeted a unique `old_string` and the file was read fresh once at the top of this round. No ordering dependency between them.
  - **Structural preservation.** Verified after editing that Phase 1–7 section headings count exactly `7`, the `Agent Spawn Contract` section remains the single canonical preamble source (6 occurrences: section heading + 5 cross-references from phases), and the Per-Role Spawn Appendices section is untouched. The Tillsyn-override preamble inside the fenced code block on lines 61–80 is unchanged except for the final word on line 80 (`rak artifact` → `Valv artifact`), which keeps the preamble reading naturally when pasted into future spawn prompts (Valv is now the durable artifact name).
  - **Casing choice for replacement.** Used `Valv` (Title case) everywhere rather than `valv` (lowercase). The prompt's override text the orchestrator is using right now for U0.3 itself says "Valv does not use Tillsyn", "Valv orchestrator", "Valv artifact" — all Title case — so this matches the casing already established upstream in the orchestrator prompt and in the bare-root + main CLAUDE.md files.
  - **`_TEMPLATE/CLOSEOUT.md` scope discipline.** Changed only the single Hylla artifact line (line 29). Did not touch the four other placeholder sections (Hylla Feedback Aggregation, Refinements, Ledger Entry, Wiki Changelog) — they contain no rak references and their shape is template-driven, not scoped to U0.3.
- **Hylla Feedback:** N/A — task touched non-Go files only.
- **Unknowns:** none — all 9 acceptance criteria verified numerically via `Grep` count mode, structural invariants confirmed intact, brand-token sweep confirmed zero residue.

## Unit 0.2 — Round 1

- **Scope:** rewrite `main/CLAUDE.md` (the work-orchestrator prompt) from its rak-copied source into a Valv-grounded version against ground-truth `go.mod`, `magefile.go`, `AGENTS.md`, `internal/`, and `cmd/`.
- **Files changed:** `/Users/evanschultz/Documents/Code/hylla/valv/main/CLAUDE.md` (full rewrite via `Write`).
- **Mage targets run:** none (docs-only; no Go code touched — `mage test` runs at Phase 6 per WORKFLOW.md).
- **Acceptance self-check (all 15 criteria via `Grep` count + content mode, observed values):**
  1. `head -1 CLAUDE.md | grep -c '^# Valv — Project CLAUDE.md'` → `1` (expected `1`, pass — em-dash U+2014 preserved on line 1).
  2. `grep -c 'Rak' CLAUDE.md` → `0` (expected `0`, pass).
  3. `grep -ciE '\brak\b' CLAUDE.md` → `0` (expected `0`, pass).
  4. `grep -c 'github.com/evanmschultz/rak@main' CLAUDE.md` → `0` (expected `0`, pass); `grep -c 'github.com/evanmschultz/valv@main' CLAUDE.md` → `1` (expected `>=1`, pass).
  5. `grep -c '/hylla/rak' CLAUDE.md` → `0` (expected `0`, pass).
  6. `grep -c 'cmd/rak' CLAUDE.md` → `0` (expected `0`, pass); `grep -c 'cmd/valv' CLAUDE.md` → `3` (expected `>=1`, pass — package-map row, build target, and `go build ./cmd/valv` example).
  7. `grep -c '\./cmd/valv' CLAUDE.md` → `2` (expected `>=1`, pass — appears in the package-map `./cmd/valv` note and the `mage build` `go build -o ./valv ./cmd/valv` command column).
  8. `grep -cE '\b(counting|fileset)\b' CLAUDE.md` → `0` (expected `0`, pass); `grep -c 'internal/domain\|internal/adapters\|internal/services\|internal/cli\|internal/tui' CLAUDE.md` → `9` (expected `>=5`, pass); `grep -c 'internal/lang\b\|internal/render\b\|internal/summary\b\|internal/tokens\b\|internal/ignore\b' CLAUDE.md` → `0` (expected `0`, pass).
  9. Tech-stack five sub-checks:
     - `grep -c 'tiktoken-go/tokenizer\|golang.org/x/sync/errgroup' CLAUDE.md` → `0` (expected `0`, pass).
     - `grep -oE 'charm\.land/[a-z]+/v2' CLAUDE.md | sort -u | wc -l` → `4` (expected `>=3`, pass — `charm.land/fang/v2`, `charm.land/bubbletea/v2`, `charm.land/lipgloss/v2`, `charm.land/bubbles/v2`, all four of Valv's direct Charm v2 imports present).
     - `grep -c 'modernc.org/sqlite' CLAUDE.md` → `3` (expected `>=1`, pass).
     - `grep -c 'testcontainers' CLAUDE.md` → `4` (expected `>=1`, pass).
     - `grep -c 'modelcontextprotocol/go-sdk' CLAUDE.md` → `1` (expected `>=1`, pass).
  10. `grep -c 'mage install\|mage format\|mage lint\|mage ci\|mage coverage\|mage plan-check' CLAUDE.md` → `0` (expected `0`, pass — every rak-only target removed); `grep -c 'mage test\b\|mage build\b\|mage testPkg\|mage integration\|mage golden\|mage goldenUpdate\|mage run\|mage dev:' CLAUDE.md` → `19` (expected `>=6`, pass — table rows plus prose mentions).
  11. `grep -cE 'domain.*→.*(adapters|adapt).*→.*services.*→.*(cli|tui)' CLAUDE.md` → `1` (expected `>=1`, pass — single-line Import DAG at line 133: `internal/domain → internal/adapters → internal/services → (internal/cli | internal/tui)`); `grep -B 2 -A 5 -E 'domain.*→.*(adapters|adapt).*→.*services.*→.*(cli|tui)' CLAUDE.md | grep -cE 'no cycles|strictly layered'` → `1` (expected `>=1`, pass — the same line terminates with "No cycles — strictly layered." inside the 2B/5A window).
  12. `grep -c '^| File | Role | LOC |' CLAUDE.md` → `0` (expected `0`, pass — rak File-Breakdown LOC table deleted entirely).
  13. AGENTS.md three-part check:
     - `grep -cE '^## .*AGENTS\.md' CLAUDE.md` → `1` (expected `>=1`, pass — line 5: `## AGENTS.md — Authoritative For Cross-Cutting Rules`).
     - `grep -cE '^## (Legacy|Historical|Deprecated|Obsolete|Old|Former) .*AGENTS\.md' CLAUDE.md` → `0` (expected `0`, pass — no negative-polarity heading words).
     - `grep -A 10 -E '^## .*AGENTS\.md' CLAUDE.md | grep -cE 'authoritative|defers to|source of truth'` → `1` (expected `>=1`, pass — line 7 body opens with "authoritative source of truth" and closes with "defers to AGENTS.md"; both phrases land inside the 10-line window after the heading).
  14. `grep -c 'main/drops/WORKFLOW.md' CLAUDE.md` → `10` (expected `>=1`, pass — phase-mechanics pointer preserved throughout).
  15. `grep -c 'go-builder-agent\|go-planning-agent\|go-qa-proof-agent\|go-qa-falsification-agent' CLAUDE.md` → `11` (expected `>=4`, pass — all four agent names present in the Agent Bindings table, Build-QA-Commit Loop prose, QA Discipline section, and Orchestrator Role Boundaries section).
- **Design notes:**
  - **`Write` over serial `Edit`.** The rak file had ~40 rak-specific references, a whole obsolete LOC table, an entire tech-stack section referencing tiktoken + errgroup, and a package map listing seven packages that do not exist in Valv. Serial `Edit` would have meant 20+ touch points, each risking ordering bugs. Full rewrite via `Write` was safer and produced a cleanly-scoped, UTF-8-no-BOM file.
  - **AGENTS.md deferral shape.** Placed the "AGENTS.md — Authoritative For Cross-Cutting Rules" section as the second `##` section after the H1 intro paragraph (criterion 13's three sub-checks all pass on this placement). The body cites the concrete AGENTS.md content areas (§ 4 runtime model, § 5–7 Go standards / errors / logging, § 8 CLI/TUI stack, § 9 Context7-first, § 11 delivery standards, § 12 repo standards, § 13 sandbox rules) so the deferral is not hollow — readers can see AGENTS.md is actually authoritative for specific scopes, not just name-dropped. Positive-polarity framing uses both "authoritative source of truth" and "defers to AGENTS.md" inside the first paragraph (both within the 10-line window per 13c).
  - **`cli` vs `tui` handling.** Valv's `internal/` has both `internal/cli` and `internal/tui` directories (verified via `ls /Users/evanschultz/Documents/Code/hylla/valv/main/internal/`). The Import DAG arrow chain lists both: `internal/domain → internal/adapters → internal/services → (internal/cli | internal/tui)` — satisfies criterion 11a's `(cli|tui)` alternation. The package map describes both packages separately, with `internal/api` noted as a sibling consumer tier. `cmd/valv` wires in at the top.
  - **Package map grounding.** Every package listed in the map is a real directory under `main/internal/` — confirmed via the `ls` output (`adapters`, `api`, `cli`, `config`, `domain`, `logging`, `output`, `pathutil`, `progress`, `project`, `services`, `tui`). Did not invent `internal/counting` / `internal/fileset` / `internal/lang` / `internal/render` / `internal/summary` / `internal/tokens` / `internal/ignore` — all rak-era. Criterion 8c specifically guards against those seven names surviving, and they do not.
  - **Tech Stack grounding.** Every production dep listed is a direct require in `main/go.mod` (confirmed via reading go.mod lines 5–22). Did not list indirect deps. Explicitly called out "no Go Docker SDK" — Valv shells out via `exec.Command` to the `docker` CLI (seen in `magefile.go`'s `Dev` namespace targets like `Dev.Clean`'s `docker image rm` call). This defends against a future planner hallucinating a Docker SDK dependency; criterion 9 does not require the claim but it is an easy trap to avoid.
  - **Mage targets grounding.** Every target in the mage table is a real exported function in `main/magefile.go` (`Build` line 48, `Test` line 71, `TestPkg` line 93, `Integration` line 122, `Golden` line 127, `Run` line 143, `GoldenUpdate` line 151, `Dev.Home` line 183, `Dev.Reset` line 192, `Dev.Clean` line 201, `Dev.Run` line 217). Noted that `mage test` already runs gofumpt check + tests + 70% coverage gate inline (seen at `magefile.go` lines 71–90 calling `checkRepoFormatting` → `runRepoTests` → `renderCoverage`), so there is no separate `mage format` / `mage lint` / `mage ci` / `mage coverage` needed — the rak-era targets criterion 10a guards against are genuinely absent from Valv's design, not just absent from the magefile.
  - **Preserved-from-rak shape.** Kept the "Coordination Model — At a Glance", "Drops", "Orchestrator-as-Hub" + "Agent Bindings" table, "Build-QA-Commit Loop", "Hylla Baseline" + "Code Understanding Rules", "Evidence Sources", "Semi-Formal Reasoning", "QA Discipline", "Orchestrator Role Boundaries", "Skill and Slash Command Routing", "Git Commit Format", "Safety", "Bare-Root and Worktree Discipline", and "Recovery After Session Restart" sections (all still structurally applicable to Valv's work-orch role). Rewrote the in-section content to reference Valv paths (`/hylla/valv/...`), Valv artifact ref (`github.com/evanmschultz/valv@main`), Valv mage targets, and Valv package roots.
  - **Deleted-from-rak shape.** Removed the "File Breakdown (expected sizes — no file exceeds ~400 LOC)" table entirely (criterion 12 — the LOC table is a rak-Drop-1 planning artifact that has no bearing on Valv). Removed the "Go-Idiomatic Naming Rules" enumerated list — AGENTS.md § 5 covers idiomatic Go at the required level of depth, so duplicating 12 naming rules here would drift from AGENTS.md over time. Removed the rak-era Drop-7.1 / Drop-8.1 / Drop-9.3 references inside the tech-stack and coverage paragraphs — those drop numbers do not exist in Valv's drop tree (`main/PLAN.md` has DROP_0..DROP_9 with completely different scope).
  - **Commit-format examples rebranded.** Kept the commit-format section structurally intact (conventional-commit, subject-line-only rule, lowercase-except-proper-nouns) but rewrote every example to be Valv-appropriate (`feat(codex): ...`, `fix(adapters): ...`, `chore(deps): bump charm.land/bubbletea to v2.0.2`, `docs(drop-0): rewrite project docs for Valv`, `docs(drop-3): ...`). Initial draft had `docs(drop-0): rebrand project docs from rak to Valv` which contained a bareword `rak` and would have failed criterion 3 on a shell-grep re-check; caught on a second post-Write pass and reworded to `rewrite project docs for Valv`. Re-ran criterion 2 (`grep -c 'Rak'`) and criterion 3 (`grep -ciE '\brak\b'`) after the fix — both observed `0`, both pass.
- **Hylla Feedback:** N/A — task touched non-Go files only.
- **Unknowns:** none — all 15 acceptance criteria verified numerically, final post-fix re-checks on criteria 2 and 3 both returned `0` as expected, AGENTS.md deferral heading + body framing confirmed against criterion 13's three sub-parts, Import DAG arrow chain + polarity-window confirmed on criterion 11's two-part check, package map + tech stack + mage targets all grounded in the actual Valv ground-truth files.
