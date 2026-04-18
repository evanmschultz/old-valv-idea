# Plan QA Proof — DROP_0_DOCS_BOOTSTRAP — Round 3

**Verdict:** pass
**Reviewed:** 2026-04-18

## Findings

1. `pass` — **U0.1 crit 7 sub-anchors resolve against the pre-rebrand bare-root file.** Direct `grep -cE '^- \*\*Never edits Go source' /Users/evanschultz/Documents/Code/hylla/valv/CLAUDE.md` returns `1`; `^- \*\*Never runs mage` returns `1`; `^- \*\*Never commits or pushes` returns `1`; `^## What The Steward Does NOT Do` returns `1`. Anchors correspond to bare-root lines 68, 69, 71, and 66 respectively (read-verified). F1-R2 fix is mechanically sound: the line-9 summary bullet starts `- **Steward` (not `- **Never`), so the anchor rejects it cleanly.
2. `pass` — **U0.2 crit 1 H1 title pin works as specified.** `grep -c '^# Valv — Project CLAUDE.md' /Users/evanschultz/Documents/Code/hylla/valv/main/CLAUDE.md` forces exactly `1`. The current file H1 is `# Rak — Project CLAUDE.md (main worktree)` (system-reminder cited live content), so grep currently returns `0` and returns `1` only after the rebrand. Anchor is POSIX ERE/BRE compatible and unambiguous.
3. `pass` — **U0.2 crit 6 `cmd/rak` → `cmd/valv` binary path rewrite is grounded in magefile reality.** `main/magefile.go:53` carries `Detail: \"./cmd/valv\"` and `:57` carries `runGo(\"build\", \"-o\", \"./valv\", \"./cmd/valv\")`. The crit pair is faithful to the codebase.
4. `pass` — **U0.2 crit 7 `./cmd/valv` path-style pin (C6 fix) is executable and disjoint from crit 6.** `grep -c '\./cmd/valv' ...` with escaped dot returns ≥1 only when a path-style reference is present. Escape is POSIX BRE/ERE correct; distinct from crit 6 bare match.
5. `pass` — **U0.2 crit 8 package map aligns with `main/internal/` ground truth.** Actual packages: adapters, api, cli, config, domain, logging, output, pathutil, progress, project, services, tui. Positive grep for `internal/domain|internal/adapters|internal/services|internal/cli|internal/tui` returning ≥5 matches exactly five existing packages. Negative grep for rak-only roots (lang, render, summary, tokens, ignore) returning 0 catches rak survivors. Word-boundary `\b(counting|fileset)\b` avoids false positives on \"accounting\".
6. `pass` — **U0.2 crit 9 Tech Stack is grounded in `main/go.mod`.** Direct requires include `charm.land/fang/v2`, `charm.land/bubbletea/v2`, `charm.land/lipgloss/v2`, `modernc.org/sqlite`, `testcontainers/testcontainers-go`, `modelcontextprotocol/go-sdk`. No `tiktoken-go/tokenizer` (rak-only), no direct `golang.org/x/sync` (errgroup is indirect only). Docker SDK is absent as a Valv direct dep — `docker/docker` appears as transitive via testcontainers only; Valv code shells out via `exec.Command`. ≥4 positive-match count on six candidates is achievable.
7. `pass` — **U0.2 crit 10 Mage targets are grounded in magefile reality.** Top-level: `Build`, `Test`, `TestPkg`, `Integration`, `Golden`, `Run`, `GoldenUpdate`. Namespaced: `Dev.Home`, `Dev.Reset`, `Dev.Clean`, `Dev.Run`. Positive grep for `mage test\b|mage build\b|mage testPkg|mage integration|mage golden|mage goldenUpdate|mage run|mage dev:` returning ≥6 is satisfiable. Negative grep rejecting `mage install|mage format|mage lint|mage ci|mage coverage|mage plan-check` is faithful — none exist in Valv magefile.
8. `pass` — **U0.2 crit 11 (a)+(b) two-part DAG check is mechanically sound (F3-R2 fix).** (a) `grep -cE 'domain.*→.*(adapters|adapt).*→.*services.*→.*(cli|tui)'` permits one-line arrow-chain rewrites. (b) `grep -A 5 <regex> | grep -cE 'no cycles|strictly layered'` runs within 5 lines of the DAG and pins positive polarity. `-A` is supported on macOS BSD grep and GNU grep. Polarity subcheck defeats the \"do NOT follow\" counterexample.
9. `pass` — **U0.2 crit 14 (a)+(b) AGENTS.md deferral check is mechanically sound (F4-R2 fix).** (a) `grep -cE '^## .*AGENTS\.md'` finds any H2 naming the file. (b) `grep -A 10 <heading> | grep -cE 'authoritative|defers to|source of truth'` forces positive-polarity framing within 10 lines. Together they reject `## Ignore Legacy AGENTS.md` counterexamples that pass (a) alone.
10. `pass` — **U0.3 crit 5 `rak artifact` → `Valv artifact` rewrite matches current WORKFLOW.md line 80.** Direct grep shows line 80: `PLAN_QA_*.md, CLOSEOUT.md, or any other durable rak artifact.` The crit pair aligns exactly with the live target string.
11. `pass` — **U0.3 crit 7 `_TEMPLATE/CLOSEOUT.md` rak@main fix is grounded.** Direct grep shows line 29: `- **Source:** github.com/evanmschultz/rak@main`. Crit targets the known bug.
12. `pass` — **U0.3 crit 8 phase-count assertion is grounded.** Direct `grep -cE '^## Phase [1-7] — '` on current WORKFLOW.md returns `7` — matches expected count exactly.
13. `pass` — **U0.4 crit 2 BOM pre-strip is correctly formulated (C8 fix).** `sed '1s/^\xef\xbb\xbf//'` strips leading UTF-8 BOM (3 bytes: 0xEF 0xBB 0xBF) on line 1 only. Inside single quotes the \x escapes pass unchanged to sed; GNU sed and macOS BSD sed both accept them. Plain-ASCII output makes sed a no-op; BOM-prefixed file still passes. Builder-facing BOM-free directive recorded at line 91.
14. `pass` — **U0.5 crit 6 per-edge pipe-cell anchors work as specified (C9 fix).** Spot-check: against synthetic row `| DROP_1_DELETE_API_WRAPPER | todo | blocked_by: DROP_0 | scope | link |`, anchor `^\|.*DROP_1_DELETE_API_WRAPPER.*\|.*blocked_by: *DROP_0.*\|` returns `1` (live-tested via Grep tool). Against free-text prose `DROP_1 blocked_by: DROP_0`, anchor returns `0` because leading `^\|` is absent. Anchor correctly forces blocked_by edge into the table row. Cleaned alternation `(—|-|none)` on the DROP_0 grep (cosmetic 3.1) removes duplicate em-dash without changing semantics.
15. `pass` — **U0.5 crit 7 row-order awk enforcement is mechanically sound (F5-R2 fix).** Extractor `awk -F'[_|]' '/^\| *DROP_/ {gsub(/[^0-9]/,\"\",$3); print $3}'` correctly pulls drop numbers from pipe-cell rows. Live-validated against a 3-row sample: stage 1 emitted `0`, `1`, `2` as expected. Stage 2 `awk 'BEGIN{prev=-1} {if ($1 != prev+1) exit 1; prev=$1} END{if (prev != 9) exit 1}'` is a POSIX-standard running-ascending assertion: `prev=-1` forces first value = 0; each subsequent value = prev+1; END guard forces final = 9. Reordered permutation (0,2,1,…) fails at first out-of-order pair; short sequence (ends at 8) fails END guard.

## Scope Coverage Audit

Mapping each Scope-paragraph (line 14) promise to its covering criterion:

- *"Rebrand ... bare-root `CLAUDE.md`"* → U0.1 crits 1-7.
- *"Rebrand ... `main/CLAUDE.md`"* → U0.2 crits 1-5 (branding) + 15-16 (workflow/agent preservation).
- *"Rebrand ... `main/drops/WORKFLOW.md`"* → U0.3 crits 1-6 + 8-9.
- *"Rewrite the sections of `main/CLAUDE.md` that are rak-specific (Project Structure package map, Import DAG, File Breakdown table, Tech Stack, Mage targets table)"* → U0.2 crits 6 (cmd path), 7 (cmd path pin), 8 (package map), 9 (Tech Stack), 10 (Mage targets), 11 (Import DAG), 12 (File Breakdown removal), 13 (mage install prohibition), 14 (AGENTS.md deferral).
- *"Bootstrap the six durable Phase 7 closeout artifacts"* → U0.4 crits 1-5.
- *"Rewrite `main/PLAN.md` as the rak-shape ten-container drop tree"* → U0.5 crits 1-10.
- *"Freeze `main/.worklog/` as historical by dropping a `.FROZEN` marker inside it"* → U0.6 crits 1-5.
- *"`.gitignore` line 4 already lists `.worklog/`, so no append is needed"* → U0.6 crit 3 explicit no-duplicate guard.
- *"Drop 0 is an intentional dry-run"* → DROP_1 `blocked_by: DROP_0` edge in U0.5 crit 6 encodes dry-run-first order, reinforced by Notes item 1.
- *"`mage build` and `mage test` must still be runnable at drop-end"* → Phase-6 note at line 197 routes this to Phase 6, not U-level acceptance. Intentional per plan reasoning.
- *"U0.3 bug that the orchestrator prompt preamble already corrects inline"* → U0.3 crit 5 targets it exactly.
- *"Fix `_TEMPLATE/CLOSEOUT.md` line 29 rak@main reference"* → U0.3 crit 7 targets it explicitly.

Coverage is complete. No Scope-paragraph promise has gone stale across Round 2 → Round 3 — the renumbering preserved every earlier coverage, and the three new criteria (U0.2 crit 1, U0.2 crit 7, U0.5 crit 7) add strength without removing anything.

## Evidence

- `/Users/evanschultz/Documents/Code/hylla/valv/main/drops/DROP_0_DOCS_BOOTSTRAP/PLAN.md:1-198` — the target plan.
- `/Users/evanschultz/Documents/Code/hylla/valv/CLAUDE.md:9,66,68,69,71` — bare-root canonical-bullet + section-heading anchors for F1-R2.
- `/Users/evanschultz/Documents/Code/hylla/valv/main/CLAUDE.md:1` — current H1 `# Rak — Project CLAUDE.md (main worktree)` (via system reminder).
- `/Users/evanschultz/Documents/Code/hylla/valv/main/go.mod:6-21` — direct deps grounding U0.2 crit 9 Tech Stack.
- `/Users/evanschultz/Documents/Code/hylla/valv/main/magefile.go:48-217` — mage targets grounding U0.2 crit 10 (Build/Test/TestPkg/Integration/Golden/Run/GoldenUpdate + Dev namespace).
- `/Users/evanschultz/Documents/Code/hylla/valv/main/magefile.go:53,57` — `./cmd/valv` path grounding U0.2 crits 6-7.
- `/Users/evanschultz/Documents/Code/hylla/valv/main/magefile.go:311` — `.worklog` directory skip grounding U0.6 crit 4.
- `/Users/evanschultz/Documents/Code/hylla/valv/main/.gitignore:4` — `.worklog/` line grounding U0.6 crit 3.
- `/Users/evanschultz/Documents/Code/hylla/valv/main/internal/` — package roots grounding U0.2 crits 8, 11.
- `/Users/evanschultz/Documents/Code/hylla/valv/main/drops/WORKFLOW.md:80` — `durable rak artifact` text grounding U0.3 crit 5.
- `/Users/evanschultz/Documents/Code/hylla/valv/main/drops/_TEMPLATE/CLOSEOUT.md:29` — `rak@main` text grounding U0.3 crit 7.

## Notes

- Non-blocking polish: U0.5 crit 7 awk narration inline-explains the field-index derivation, which the builder can read as a pedagogical aid. If a future plan adds a leading column (e.g. `| # | DROP_N_… | …`) the $3 field would shift and the extractor would need a companion update — but the criterion fails loudly rather than silently, so the regression surfaces.
- Non-blocking polish: U0.4 crit 2 BOM strip uses `\xef\xbb\xbf` byte escapes that rely on sed `\x` extension. POSIX sed does not guarantee support, but GNU sed (Linux) and BSD sed (macOS, the dev platform per env) both accept it. Valv development is macOS/Linux only — no action required.
- Unknown: U0.2 crit 10 `mage dev:` grep is lowercase `dev:`. Mage namespace invocation is case-insensitive (`mage Dev:Home` and `mage dev:home` both work). If builder uses uppercase `Dev` prose in the table, `mage dev:` grep misses it and the ≥6 floor relies on the other seven candidates — `Build`, `Test`, `TestPkg`, `Integration`, `Golden`, `GoldenUpdate`, `Run` — which is enough. Not blocking; flagging for dev awareness.
- Cosmetic: PLAN.md line 171 says "post-Round-3 numbering" — the plan correctly prospectively anchors to the final numbering. Good documentation hygiene.
