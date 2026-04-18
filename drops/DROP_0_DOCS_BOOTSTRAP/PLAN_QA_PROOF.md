# Plan QA Proof — DROP_0_DOCS_BOOTSTRAP — Round 2

**Verdict:** pass
**Reviewed:** 2026-04-18

## Findings

1. `pass` — **U0.1 crit 7 invariants are real and verbatim.** Grep confirms all three target sentences exist in current bare-root `/Users/evanschultz/Documents/Code/hylla/valv/CLAUDE.md` at exactly lines 68, 69, and 71: "Never edits Go source, tests, `magefile.go`, or any `.go` file." / "Never runs mage targets" / "Never commits or pushes on behalf of a work orchestrator's drop." The three per-grep floor checks are mechanically executable as written.
2. `pass` — **U0.1 crit 3 `/hylla/rak` pattern is correctly unterminated.** Direct Grep for `/hylla/rak` in bare-root shows 5 occurrences, and line 118 `Never run commands outside /Users/evanschultz/Documents/Code/hylla/rak` genuinely has no trailing slash — so dropping the trailing slash in the pattern (per Round 2 finding F2) is correct and the grep will catch it.
3. `pass` — **U0.1 crit 4 stale "Go source lands here in Drop 1" annotation confirmed present.** Line 40 of bare-root `CLAUDE.md` reads `└── (Go source lands here in Drop 1)`. Grep matches exactly once; negative criterion after rebrand is mechanically executable.
4. `pass` — **U0.2 crit 5 `cmd/rak` → `cmd/valv` grounded.** Grep reports 11 occurrences of `cmd/rak` in current `main/CLAUDE.md` (lines 112, 124, 130–132, 159, 211, 212, 216, 247, 248). `main/magefile.go` lines 53 and 57 both reference `./cmd/valv`. The positive grep `cmd/valv` floor of at least 1 is satisfiable.
5. `pass` — **U0.2 crit 6 word-boundary `\b(counting|fileset)\b` is tight enough.** No Valv package uses either name, and `\b` prevents false-positives on `accounting`, `file_setup`, etc. Direct count in current `main/CLAUDE.md` shows 33 matches across the negative-grep family (covers the rak `internal/<pkg>` map). Minor note: the English noun "counting" could in principle appear in the rebranded file as prose; the builder should simply avoid prose like "counting packages" in the rewrite, which is easy.
6. `pass` — **U0.2 crit 9 single-line DAG regex matches Valv reality.** Current rak line 124 uses the arrow glyph `→` (U+2192), so the builder will naturally copy the idiom with the correct character. The regex `domain.*→.*(adapters|adapt).*→.*services.*→.*(cli|tui)` permits both `internal/` prefixes and either `cli` or `tui` as the root, which matches VALV_CLAUDE_CODE_FOCUS_PLAN.md §6.1–§6.7 (domain → adapters → services → cli/tui).
7. `pass` — **U0.2 crit 12 structural AGENTS.md heading check.** `grep -cE '^## .*AGENTS\.md'` is well-formed (line-anchored, escaped dot, permissive middle). Matches `## Deferral to AGENTS.md` or `## AGENTS.md — Authoritative …` as promised. `main/AGENTS.md` itself exists (ls confirmed).
8. `pass` — **U0.3 crit 2 case-insensitive `rak` word-boundary grep catches all rak survivors.** Direct Grep of WORKFLOW.md shows 3 occurrences matching `\brak\b` case-insensitive: line 1 (title `Rak`), line 54 (`Rak does not use`), and line 80 (`durable rak artifact`). After rebrand all 3 should flip, and crit 1 (Title-case `Rak`) + crit 6 (Tillsyn-override sentence) already stack-verify this. Lines 54 and 80 are correctly cited in the planner's note.
9. `pass` — **U0.4 crit 2 six exact-match heading checks are mechanically executable.** `head -1 <file>` returns the first line, and a shell string equality against `# WIKI` / `# LEDGER` / `# REFINEMENTS` / `# HYLLA_FEEDBACK` / `# HYLLA_REFINEMENTS` / `# WIKI_CHANGELOG` is unambiguous. ALL-CAPS filename-casing match eliminates the Round 1 "either casing acceptable" ambiguity (finding P4).
10. `pass` — **U0.5 crit 6 strict linear blocked_by chain has 10 distinct well-formed greps.** Each grep pattern `DROP_N_<NAME>.*blocked_by: *DROP_{N-1}` is row-local (table rows are single lines) and the `.*` is bounded by the pipe-cell format. Minor cosmetic: the first grep's alternation `(—|-|none|—)` lists em-dash twice (character repeated) — harmless, still matches. Linear chain `DROP_0 ← DROP_1 ← … ← DROP_9` aligns with VALV_CLAUDE_CODE_FOCUS_PLAN.md line 230 `§6.1 → §6.2 → §6.2a → §6.3 → §6.4 → §6.5 → §6.6 → §6.7` plus the drop-0 dry-run-first intent.
11. `pass` — **U0.6 Scope line 14 no longer contradicts criterion 3.** Line 14 reads "`.gitignore` line 4 already lists `.worklog/`, so no append is needed" — consistent with crit 3's "MUST NOT add a duplicate line." Cross-checked against current `main/.gitignore` line 4: `.worklog/` is present exactly once.
12. `pass` — **U0.2 Tech Stack grounded against `main/go.mod`.** Verified: `tiktoken-go/tokenizer` and `golang.org/x/sync/errgroup` are NOT in `main/go.mod` (rak-only deps). Verified present in go.mod: `charm.land/fang/v2`, `charm.land/bubbletea/v2`, `charm.land/lipgloss/v2`, `modernc.org/sqlite`, `testcontainers/testcontainers-go`, `modelcontextprotocol/go-sdk`. Docker SDK absent (Valv shells out via `exec.Command` per `main/magefile.go` line 213 `run("docker", args...)`). Planner note 2 correctly warns against inheriting a Docker SDK claim.
13. `pass` — **U0.2 Mage targets table grounded against `main/magefile.go`.** Verified absent: `mage install`, `mage format`, `mage lint`, `mage ci`, `mage coverage`, `mage plan-check`. Verified present: `Build`, `Test`, `TestPkg`, `Integration`, `Golden`, `GoldenUpdate`, `Run`, `Dev.Home/Reset/Clean/Run`. The `Test` target does inline gofumpt + test + coverage (lines 71–90). Criteria 8 + 11 match this reality.
14. `pass` — **U0.3 crit 7 template CLOSEOUT.md rak artifact bug exists.** Direct read of `main/drops/_TEMPLATE/CLOSEOUT.md` line 29 shows `Source: github.com/evanmschultz/rak@main`. The negative+positive grep pair is mechanically executable and targets the real bug.
15. `pass` — **U0.6 crit 5 `.worklog/` has pre-existing content.** `ls` confirms many historical `.md` + `.log` files. `wc -l > 1` (actually `>> 1`) floor after adding `.FROZEN` is satisfiable.

## Scope Coverage Audit

Scope paragraph (line 14) promises:

- Rebrand three orchestrator docs (bare-root CLAUDE.md, main/CLAUDE.md, main/drops/WORKFLOW.md) — covered by **U0.1**, **U0.2**, **U0.3**.
- Rewrite rak-specific sections of `main/CLAUDE.md` (Project Structure package map, Import DAG, File Breakdown table, Tech Stack, Mage targets table) against Valv reality — covered by **U0.2** criteria 6, 7, 8, 9, 10, 11.
- Bootstrap six durable Phase 7 closeout artifacts (`WIKI.md`, `LEDGER.md`, `REFINEMENTS.md`, `HYLLA_FEEDBACK.md`, `HYLLA_REFINEMENTS.md`, `WIKI_CHANGELOG.md`) — covered by **U0.4**.
- Rewrite `main/PLAN.md` as the ten-container drop tree seeded from focus-plan §6 slices — covered by **U0.5**.
- Freeze `main/.worklog/` with a `.FROZEN` marker (gitignore append a no-op) — covered by **U0.6**.
- Template CLOSEOUT.md rak artifact bug fix — covered by **U0.3** crit 7.
- Phase-mechanics dry run (implicit — DROP_0 itself is the dry run) — inherent in the drop structure, called out in Scope + Notes.

No Scope promise left uncovered. Every rak-specific section of `main/CLAUDE.md` that Scope names is covered by at least one U0.2 criterion.

## Evidence

- `/Users/evanschultz/Documents/Code/hylla/valv/CLAUDE.md` lines 1, 3, 7, 16, 19, 33, 40, 45, 68, 69, 71, 76, 118 (pre-rebrand bare-root).
- `/Users/evanschultz/Documents/Code/hylla/valv/main/CLAUDE.md` lines 1, 3, 7, 45, 57, 112, 116, 124, 130–132, 142–145, 159, 163, 174, 184, 186, 190–191, 199, 208–214, 216, 247–248, 281, 294, 303, 314 (pre-rebrand work-orch).
- `/Users/evanschultz/Documents/Code/hylla/valv/main/drops/WORKFLOW.md` lines 1, 54, 80 (pre-rebrand workflow + Agent Spawn Contract survivors).
- `/Users/evanschultz/Documents/Code/hylla/valv/main/drops/_TEMPLATE/CLOSEOUT.md` line 29 (rak artifact bug in template).
- `/Users/evanschultz/Documents/Code/hylla/valv/main/magefile.go` lines 22–29 (valv-codex-dev constants), 48–67 (Build targeting `./cmd/valv`), 70–90 (Test inline), 93–164 (TestPkg/Integration/Golden/Run/GoldenUpdate), 180–235 (Dev namespace), 213 (`run("docker", args...)` shell-out), 303–325 (goFiles with `.worklog` skip at 311).
- `/Users/evanschultz/Documents/Code/hylla/valv/main/go.mod` lines 1–22 (direct deps — no tiktoken/errgroup/Docker SDK; charm.land v2, modernc sqlite, testcontainers, MCP SDK all present).
- `/Users/evanschultz/Documents/Code/hylla/valv/main/.gitignore` line 4 (`.worklog/`).
- `/Users/evanschultz/Documents/Code/hylla/valv/main/VALV_CLAUDE_CODE_FOCUS_PLAN.md` line 230 (linear compile-guarantee chain §6.1 → §6.2 → §6.2a → §6.3 → §6.4 → §6.5 → §6.6 → §6.7).
- `main/.worklog/` listing: 25+ pre-existing historical `.md` / `.log` files.

## Notes

Round 2 is clean from a proof-review angle. All 13 dev-accepted Round 1 findings have been applied and the resulting criteria are mechanically executable against current file state. Two very-minor cosmetic notes (not blockers, not concerns — flagged for planner awareness only):

- U0.5 crit 6 first grep alternation `(—|-|none|—)` contains the em-dash character twice. Harmless regex redundancy; grep will still match either once. If the planner touches this line again it can be collapsed to `(—|-|none)`.
- U0.2 crit 6 word-boundary `\bcounting\b` in theory could false-positive on English prose like "counting lines". In practice the rebranded `main/CLAUDE.md` has no reason to contain that word, but the builder can be mindful.

No Round 3 required based on proof review. If falsification sibling also clears, this drop can flip to `state: building` and proceed to Phase 4.
