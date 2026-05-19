# PLAN QA Falsification — DROP_9 R6

**Round:** 6
**Target:** `drops/DROP_9_CLI_AUDIT_AND_REFACTOR/PLAN.md` (R6 revision; planner commit `12efed0`)
**Scope:** attack the R6 single-line fix to Unit 9.1 AC #6 and the 5 fresh attack vectors enumerated by orchestrator.

**Verdict: PASS** (with one cosmetic / documentation-polish note on Vector 3).

---

## R6 Delta Recap

R6 changed exactly one line of Unit 9.1 AC #6 — replacing the unfulfillable
`git grep -n "valv manage" -- internal/cli/manage.go | grep -v 'Example:'`
zero-hits gate with prose-only Read-based verification:

> Verify: after this unit lands, the 3 `fmt.Errorf` runtime strings at
> `internal/cli/manage.go:626`, `:962`, `:996` are updated to reference the new
> normalized command tree. Builder verifies each line via direct `Read`
> post-edit and cites the post-edit text in `BUILDER_WORKLOG.md`. Note: cobra
> `Example:` field strings in surviving constructors are NOT in this unit's
> scope — they are swept by Unit 9.4.5 AC #1b (the global manage.go zero-hits
> gate after the full chain lands).

No other lines changed (`git diff HEAD~1`: 1 insertion, 1 deletion).

---

## Per-Vector Findings

### Vector 1 — Fix verification (does R6 actually close Proof R5's finding?)

- **Attack:** does R6 AC #6 still contain any grep/script gate that would be
  unfulfillable?
- **Evidence:** R6 text (PLAN.md line 61). The string `git grep` does not appear
  in AC #6 anymore. Verification language is strictly prose + `Read`-based.
- **Conclusion:** **REFUTED.** Proof R5's AC-behavior inconsistency finding is
  closed. The fix lands as advertised.

### Vector 2 — "Cite post-edit text" — verifiable by whom?

- **Attack:** is "builder cites text" a falsifiable gate, or just a process
  expectation? Could the builder forget to cite and the unit still pass
  build-QA?
- **Evidence:** the citation is observable evidence. Build-QA (per
  `WORKFLOW.md` phase 5) reads BUILDER_WORKLOG.md as primary evidence and
  cross-checks against source. A missing citation block is a missing required
  artifact, identical in failure mode to any AC bullet that says "test X is
  present in test file Y" — build-QA verifies by `Read`-ing the named
  artifact. Build-QA absence-of-check is a generic discipline failure, not
  a planner gap.
- **Conclusion:** **REFUTED.** The citation gate is mechanical and observable.

### Vector 3 — Line number drift

- **Attack:** Unit 9.1 also deletes `newManageCommand` (~lines 22-57) and
  `runManageHome` from manage.go. After those deletions, the named target
  lines 626/962/996 will shift upward by the deletion size. The line
  references in AC #6 will be stale by the time the builder reaches the
  `fmt.Errorf` edits.
- **Evidence:** confirmed via direct `Read` of current `internal/cli/manage.go`:
  - Line 626: `return fmt.Errorf("manage account switch: account %q not found for provider %q; run \`valv manage account add %s %s\` ...`
  - Line 962: `return "", "", fmt.Errorf("account %q not found in any provider; run \`valv manage account add codex %s\` ...`
  - Line 996: `return "", "", fmt.Errorf("no accounts found across any provider; run \`valv manage account add codex <name>\` ...`

  All three target lines match the AC #6 content exactly *as of the current
  ingest*. But Unit 9.1's `newManageCommand` deletion (above line 626) will
  shift these target lines upward.
- **Mitigation:** AC #6 also names the *content* of each substitution (e.g.,
  `valv manage account add codex %s` → `valv account add codex %s`). The three
  format strings are clearly distinct from each other (different message
  prefixes: `"manage account switch:"`, `"account %q not found in any provider"`,
  `"no accounts found across any provider"`), so content-based search remains
  unambiguous even when line numbers drift. The line numbers function as a
  locator hint, not a load-bearing gate.
- **Conclusion:** **CONFIRMED-MINOR.** Does NOT falsify the unit — content-based
  verification still succeeds. Worth a planner-polish note: AC #6 could clarify
  "line numbers are pre-deletion references; locate by content post-deletion."
  Not a R7 blocker.

### Vector 4 — 9.4.5 AC #1b dependency

- **Attack:** 9.4.5 AC #1b is now the SOLE zero-hits gate for manage.go. If
  9.4.5 is ever skipped or partially completed, the final state has stale
  strings.
- **Evidence:** PLAN.md line 237: `Blocked by: 9.1, 9.2, 9.3, 9.4`. Lines
  386-396 (the explicit dependency chain block) list 9.4.5 between 9.4 and
  9.5. Units 9.5 through 9.8 transitively block on 9.4.5 (9.5 `blocked_by:
  9.4.5`; 9.6 `blocked_by: 9.5`; etc.). WORKFLOW.md Phase 6 (drop close) gates
  on all units `state: done`.
- **Conclusion:** **REFUTED.** 9.4.5 cannot be silently skipped — every
  downstream unit blocks on it, and the drop cannot close while it is `todo`.

### Vector 5 — `Read` tool semantics for build-QA independence

- **Attack:** is `Read` of lines 626/962/996 in BUILDER_WORKLOG.md sufficient
  for build-QA to confirm correctness, or does build-QA need to re-Read
  independently?
- **Evidence:** the R6 AC text mandates the builder cite the post-edit text;
  it does not preclude build-QA from independently re-Reading source. Standard
  `go-qa-proof-agent` behavior is to verify claims against actual source via
  `Read`, not to trust worklog assertions blindly. The worklog citation is the
  *builder's* evidence-of-work; build-QA cross-checks against source as part
  of its normal pass.
- **Conclusion:** **REFUTED.** No semantic gap. The citation gives build-QA a
  start-point; build-QA's standard cross-check confirms correctness against
  disk.

---

## Findings Summary

| Vector | Finding | Class | Blocking? |
|--------|---------|-------|-----------|
| V1 | R6 closes Proof R5's grep-gate inconsistency | REFUTED | — |
| V2 | Citation gate is mechanical + observable | REFUTED | — |
| V3 | Line numbers will drift post-`newManageCommand`-delete; content-based search still works | CONFIRMED-MINOR | No (cosmetic) |
| V4 | 9.4.5 cannot be silently skipped (blocker chain) | REFUTED | — |
| V5 | Build-QA cross-checks source independently | REFUTED | — |

---

## Optional Polish (planner note, not a R7 blocker)

AC #6 could append a one-line clarifier:

> Note: line numbers 626/962/996 are as of the pre-deletion ingest. Locate by
> content (the named `fmt.Errorf` format strings) post-`newManageCommand`
> deletion — line numbers will shift upward by the deletion size.

This is a documentation-polish improvement, not a falsification. The unit is
completable and verifiable as currently written.

---

## Verdict

**PASS.** R6 closes Proof R5's AC-behavior inconsistency without introducing
new falsifiable gaps. The one CONFIRMED-MINOR finding (V3 line-number drift)
does not falsify the unit — content-based search remains unambiguous and the
named format strings are distinct enough to locate post-deletion. Recommend
the orchestrator either accept as-is or apply the optional polish note in a
trivial planner sweep, not a R7 cycle.
