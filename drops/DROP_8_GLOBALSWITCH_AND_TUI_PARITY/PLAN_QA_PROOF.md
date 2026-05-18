# DROP_8 Plan QA Proof — Round 4

**Verdict:** PASS

**Reviewed:** drop's `PLAN.md` at commit `51fa0d3 docs(drop-8): planner round 4 revision`
**Diff scope:** `+16/-4 LOC` on PLAN.md (matches brief).
**Source files cross-checked:** `internal/cli/codex_setup.go`, `internal/cli/codex.go`, `internal/cli/codex_test.go`.

## R3 → R4 Cross-Walk

| R3 finding | Severity | R4 fix | Location in PLAN.md | Verified |
|---|---|---|---|---|
| R3-B1 — Unit 8.5 paths missing `codex_test.go` + no AC for migrating the two test callers | BLOCK | Path added; new dedicated AC bullet "Test migration from `codex_test.go`" describing migration-or-deletion of the two callers at `codex_test.go:194` and `:223`, with `git grep` verification | PLAN.md line 145 (Paths), line 164 (AC) | yes |
| R3-C1 — Unit 8.7 two-probe error swallowing | CONCERN | Two-probe logic rewritten with explicit three-case enumeration per step; non-`ErrUnboundProject` errors wrap with `fmt.Errorf("detect globalswitch provider: %w", err)` and return; `errors.Is(err, domain.ErrUnboundProject)` discriminates the unbound case | PLAN.md lines 203–213 | yes |
| R3-S1 — Both-bound dispatch policy undocumented | SURGICAL | "Dispatch policy: when both Claude and Codex bindings exist for the same project, Claude wins. The step-1-first probe order encodes this policy." | PLAN.md line 213 | yes |
| R3-S2 — `errCodexSetupCanceled` sentinel deletion AC | SURGICAL | Explicit two-part deletion bullet: (a) delete sentinel at `codex_setup.go:20`; (b) delete `errors.Is(err, errCodexSetupCanceled)` branch at `codex.go:72-74`. `git grep` verification specified | PLAN.md line 152 | yes |
| R3-Polish — Unit 8.5 step-1/step-2 ambiguous "returns the resolved profile" wording | POLISH | Step 1 rewritten: "Store the resolved profile and proceed to step 4 (`ensureManagedAccountReady`)." Step 2 rewritten: "store the bound profile and proceed to step 4." | PLAN.md lines 154, 155 | yes |

## Source-File Citation Verification

All file:line references in the R4 PLAN.md edits map to real code:

- `internal/cli/codex_setup.go:20` — `var errCodexSetupCanceled = errors.New("codex setup canceled")` confirmed.
- `internal/cli/codex.go:72-74` — `ensureCodexBindingReady` call at line 72; `errors.Is(err, errCodexSetupCanceled)` branch at line 73; `return nil` at line 74. The R4 cite covers the full three-line block correctly (brief mentioned `:73-74` which is the branch body; planner's `:72-74` includes the call site, which is the deletion target).
- `internal/cli/codex_test.go:194` — `if err := ensureBoundCodexAccountReady(cmd, paths, projectRoot, []string{"resume", "--last"}); err != nil {` confirmed.
- `internal/cli/codex_test.go:223` — `if err := ensureBoundCodexAccountReady(cmd, paths, projectRoot, []string{"login"}); err != nil {` confirmed.

`git grep ensureBoundCodexAccountReady` confirms the only test callers are at `codex_test.go:194` and `:223` (production callers at `codex.go:78` and definition at `codex.go:204` — already covered by the merge AC).

## New Findings (R4)

None.

## Falsification Pass — Counterexamples Considered and Mitigated

1. Step-numbering after R4 rewrites — checked: steps 1→4, 2→3 or 2→4, 3→4 form a coherent DAG; no renumbering needed.
2. Both-bound policy reachability — `valv` schema allows a binding row per (project, provider), so both can coexist; step-1-first probe correctly implements Claude-wins.
3. Wrap-message symmetry between Step 1 and Step 2 of Unit 8.7 — identical `fmt.Errorf("detect globalswitch provider: %w", err)` in both; users see consistent wrapping regardless of which probe fails.
4. AC for "migrated OR deleted" tests — judgment is bounded by the unambiguous `git grep` verification step.
5. PLAN.md residual references to `ensureBoundCodexAccountReady` — only appear inside the merge/deletion AC and the asymmetry justification note; no stale references.
6. Symmetry with Unit 8.4 — preserved; the asymmetry justification at Notes line 238 already covers why Claude does not need the analogous merge.

## Hylla Feedback

N/A — review touched only PLAN.md (markdown) and Go source files for line citations. Hylla not consulted for this round; standard `Read` + `git grep` sufficient.

## Verdict

PASS. R4 is the tightest round of the four. Every R3 finding has a specific R4 edit with verifiable evidence in PLAN.md and matching real source lines. No new findings.
