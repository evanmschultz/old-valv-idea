# DROP_0 — PLAN QA PROOF (Round 4)

**Verdict:** PASS

## Findings

| ID | Severity | Unit/Item | Evidence gap or supporting evidence | Recommendation |
|---|---|---|---|---|
| P1 | accepted | U0.2 crit 1 (H1 pin) | `head -1 <file> \| grep -c '^# Valv — Project CLAUDE.md'` returns 1 on a matching H1. Evidence: `head -1 /…/valv/main/magefile.go` shown returning `//go:build mage` confirms `head -1` behavior is single-line read. Line-1 pin resolves prior false-fail risk when H1 string appeared elsewhere in the body. | Accept. |
| P2 | accepted | U0.2 crit 9 (Tech Stack split) | Verified `grep -c 'charm.land/' go.mod` → 5 (floor of 3 achievable); `modernc.org/sqlite`, `testcontainers`, `modelcontextprotocol/go-sdk` all present in go.mod; `tiktoken-go` and `errgroup` both absent (zero-reject achievable). Category-split design defends against the single-alternation silent-drop failure mode. | Accept. |
| P3 | accepted | U0.2 crit 10 zero-reject subsumes old crit 13 | `grep -c 'mage install\|mage format\|mage lint\|mage ci\|mage coverage\|mage plan-check' /…/main/CLAUDE.md` returns 12 today (rak content still present) — crit 10's floor-0 requirement catches `mage install` elimination just as well as a dedicated crit would. Deletion of former crit 13 is sound. | Accept. |
| P4 | accepted | Plan Note 4 magefile claim | Confirmed by reading `magefile.go` — exported targets are `Build`, `Test`, `TestPkg`, `Integration`, `Golden`, `Run`, `GoldenUpdate`, plus `Dev.Home/Reset/Clean/Run`. No `Install` target exists. The softened phrasing "not inherited (no such target exists in Valv's magefile)" is factually correct. | Accept. |
| P5 | accepted | U0.4 crit 2 BSD-sed reversion | Round 4's reasoning is sound: BSD-sed on macOS silently no-ops `\x..` byte escapes; the pre-strip was cosmetic on the dev env. Direct equality check fails loudly on BOM presence, which is correct fail-closed semantics. Builder-facing BOM-free directive remains as real defense. | Accept. |
| P6 | accepted | U0.5 crit 7 awk regex tightening | `/^\| *DROP_[0-9]+_/` correctly excludes any header row whose first cell reads `DROP_ID` or non-numeric label. Round 3's looser `/^\| *DROP_/` would have admitted phantom rows; the Round 4 tightening is a genuine improvement. | Accept. |
| P7 | accepted | U0.6 crit 5 `find -type f` replacement | `find … -type f` correctly counts regular files only, excluding `.`, `..`, and subdirectories. Floor of 2 forces both the `.FROZEN` marker and at least one historical log to coexist — the "no deletion" invariant the original `ls \| wc -l > 1` failed to express. | Accept. |
| P8 | accepted | Round 4 item 30 (U0.5 crit 3 English) | New English "cell format `DROP_N_<SUFFIX>` — drop number embedded in the name, not a separate cell" is unambiguous; verifying grep `^\| *DROP_[0-9]+_` now rejects the `\| 0 \| DOCS_BOOTSTRAP \|` split-cell interpretation that the Round-3 text invited. English and grep now agree. | Accept. |
| P9 | accepted | U0.2 crit 11(b) bidirectional window | Round 4 broadening to `-B 2 -A 5` symmetrically accommodates a polarity-positive phrase placed as either a section lede above the arrow line or a follow-up sentence below it. Window sizes (2-before, 5-after) are generous enough for typical documentation rewrites. | Accept. |
| P10 | accepted | U0.2 crit 13 negative-heading wordlist | The (b) subcheck `^## (Legacy\|Historical\|Deprecated\|Obsolete\|Old\|Former) .*AGENTS\.md` closes the loophole where a dismissive heading would pass the structural check while body text innocently contained "authoritative" in a disclaiming past tense. Six-word list covers reasonable synonyms. | Accept. |

## Round 4 Item Verification

| Item | Change | Current Criterion | Verdict |
|---|---|---|---|
| 28 | U0.4 crit 2 dropped BSD-sed BOM pre-strip | U0.4 crit 2 now uses direct `head -1 <file>` equality across all six files (lines 98-103). Prose explains BSD-sed non-portability (line 97). | PASS |
| 29 | U0.2 crit 1 H1 rewritten as `head -1 <file>` pin | U0.2 crit 1 (line 45) reads `head -1 /…/main/CLAUDE.md \| grep -c '^# Valv — Project CLAUDE.md' returns 1` with explicit justification. | PASS |
| 30 | U0.5 crit 3 English states drop number embedded in name | U0.5 crit 3 (line 117) reads "name (cell format `DROP_N_<SUFFIX>` — drop number embedded in the name, not a separate cell)" with the load-bearing anchor to crits 2/6/7 justifying the shape. | PASS |
| 31 | U0.2 crit 11(b) window broadened to `-B 2 -A 5` | U0.2 crit 11(b) (line 63) reads `grep -B 2 -A 5 -E 'domain.*→.*(adapters\|adapt).*→.*services.*→.*(cli\|tui)' … \| grep -cE 'no cycles\|strictly layered'`. Prose explains the bidirectional window rationale. | PASS |
| 32 | U0.5 crit 7 awk regex tightened to `/^\| *DROP_[0-9]+_/` | U0.5 crit 7 (line 143) extractor regex is exactly `/^\| *DROP_[0-9]+_/`. Rationale in adjacent prose (line 144) is correct. | PASS |
| 33 | U0.6 crit 5 replaced with `find -type f \| wc -l` floor of 2 | U0.6 crit 5 (lines 160-162) uses `find /…/main/.worklog -type f \| wc -l` returning at least 2, with the dual-invariant interpretation (marker written + historical preserved). | PASS |
| 34 | U0.2 old crit 13 (`mage install`) deleted | U0.2 now has 15 criteria (verified count). Crit 13 is "Authoritative-deferral to AGENTS.md" (the old crit 14). No `## AGENTS.md` prohibition line exists as a standalone criterion. Plan Note 4 softened accordingly (line 174). | PASS |
| 35 | U0.2 crit 13 added negative-heading wordlist | U0.2 crit 13 (lines 65-68) now has three sub-checks (a)/(b)/(c): structural heading / negative-word rejection / positive-polarity body. The (b) wordlist `Legacy\|Historical\|Deprecated\|Obsolete\|Old\|Former` is present. | PASS |
| 36 | U0.2 crit 9 Tech Stack split into 5 sub-checks | U0.2 crit 9 (lines 53-59) has five sub-checks: zero-reject (tiktoken/errgroup), `charm.land/` floor 3, `modernc.org/sqlite` floor 1, `testcontainers` floor 1, `modelcontextprotocol/go-sdk` floor 1. Docker-SDK-absent note retained. | PASS |

## Evidence Grounding

Six sampled criteria with their exact mechanical checks, confirmed self-contained (no external state beyond the file under test):

1. **U0.2 crit 1 (H1 pin, line 45)**:
   `head -1 /Users/evanschultz/Documents/Code/hylla/valv/main/CLAUDE.md | grep -c '^# Valv — Project CLAUDE.md'` returns `1`.
   Self-contained: reads one file, pipes to grep count, no network/env dependency.

2. **U0.2 crit 9 sub-check 2 (Charm floor, line 55)**:
   `grep -c 'charm.land/' /Users/evanschultz/Documents/Code/hylla/valv/main/CLAUDE.md` returns at least `3`.
   Confirmed achievable: Valv's go.mod carries 5 `charm.land/` import paths (verified by Grep tool).

3. **U0.4 crit 2 equality check for WIKI.md (line 98)**:
   `head -1 /Users/evanschultz/Documents/Code/hylla/valv/main/WIKI.md` equals `# WIKI` exactly.
   Self-contained: single `head -1` against a file the builder creates. BSD-sed portability risk eliminated.

4. **U0.5 crit 6 DROP_1 edge grep (line 133)**:
   `grep -cE '^\|.*DROP_1_DELETE_API_WRAPPER.*\|.*blocked_by: *DROP_0.*\|' /Users/evanschultz/Documents/Code/hylla/valv/main/PLAN.md` returns at least `1`.
   Self-contained: ERE anchored to pipe-cell format rejects prose-form edges outside table rows.

5. **U0.5 crit 7 row-order awk (line 143)**:
   `awk -F'[_|]' '/^\| *DROP_[0-9]+_/ {gsub(/[^0-9]/,"",$3); print $3}' /…/PLAN.md | awk 'BEGIN{prev=-1} {if ($1 != prev+1) exit 1; prev=$1} END{if (prev != 9) exit 1}'` exits `0`.
   Self-contained: two-stage awk pipeline extracts drop numbers and asserts strictly increasing 0→9.

6. **U0.6 crit 5 find floor (line 161)**:
   `find /Users/evanschultz/Documents/Code/hylla/valv/main/.worklog -type f | wc -l` returns at least `2`.
   Self-contained: find recursively visits files; `-type f` excludes directories. Floor of 2 enforces marker + preserved history.

All six use only `head`, `grep`, `awk`, `find`, `wc` — no `go`, `mage`, or external service calls.

## Scope Coverage Check

Scope paragraph (line 14) enumerates these load-bearing clauses:

| Scope clause | Unit / Criterion |
|---|---|
| Rebrand bare-root `CLAUDE.md` (steward prompt) | U0.1 crits 1-7 |
| Rebrand `main/CLAUDE.md` (work-orch prompt) | U0.2 crits 1-15 |
| Rebrand `main/drops/WORKFLOW.md` (per-drop lifecycle) | U0.3 crits 1-6, 8-9 |
| Rewrite `main/CLAUDE.md` Package map | U0.2 crit 8 |
| Rewrite `main/CLAUDE.md` Import DAG | U0.2 crit 11 (a) + (b) |
| Rewrite `main/CLAUDE.md` File Breakdown table (delete) | U0.2 crit 12 |
| Rewrite `main/CLAUDE.md` Tech Stack | U0.2 crit 9 (5 sub-checks) |
| Rewrite `main/CLAUDE.md` Mage targets table | U0.2 crit 10 |
| Bootstrap six Phase 7 artifacts (WIKI, LEDGER, REFINEMENTS, HYLLA_FEEDBACK, HYLLA_REFINEMENTS, WIKI_CHANGELOG) | U0.4 crits 1-5 |
| Rewrite `main/PLAN.md` as Valv drop tree (10 containers) | U0.5 crits 1-10 |
| Freeze `main/.worklog/` with `.FROZEN` marker | U0.6 crits 1-5 |
| `.gitignore` no-append (already present) | U0.6 crit 3 |
| Intentional dry-run of workflow on docs-only surface | Covered by U0.1-U0.6 all-independent (`blocked_by: —`) plus Phase-6 note (line 217) |
| No Go source touched | All six units declare `Packages: none — docs-only`; no `.go` path in any `Paths:` list |
| `mage build` + `mage test` runnable at drop-end | Explicitly out-of-scope per criterion (line 217) — Phase 6 obligation, orchestrator-owned |
| Fix `_TEMPLATE/CLOSEOUT.md` rak reference | U0.3 crit 7 |

Every load-bearing scope clause maps to at least one criterion or to the explicit Phase-6 note. No uncovered scope element detected.

## Verdict

Round 4's nine items (28-36) all landed correctly in the committed PLAN.md: each maps to a specific criterion or count change, each resolves a real Round-3 concern (BSD-sed portability, H1 false-fail, crit-3 ambiguity, polarity-window one-sidedness, awk regex over-match, `ls` dotfile/pseudo-entry bug, old crit 13 redundancy, negative-heading loophole, Tech Stack silent-drop). Criterion counts are self-consistent (U0.1=7, U0.2=15, U0.3=9, U0.4=5, U0.5=10, U0.6=5). Every sampled criterion is mechanically executable with Read/Grep/Bash only. Scope coverage is complete. Plan Note 4's claim that Valv's magefile has no `Install` target is verified correct. No blocking findings.

**Verdict: PASS.**
