# DROP_1 — Build QA Proof

## Unit 1.1 — Round 1

- **Reviewer:** go-qa-proof-agent
- **Commit reviewed:** `265be0d refactor(cli): detach valv api command wiring`
- **Working dir:** `/Users/evanschultz/Documents/Code/hylla/valv/main`
- **Completed:** 2026-04-19

**Verdict:** pass

### Acceptance-by-acceptance re-verification

| # | Command run | Actual result | Status |
|---|---|---|---|
| 1 | `test ! -e internal/cli/api.go` | file absent | PASS |
| 2 | `Grep "valv api serve" internal/cli/root.go` | 0 matches | PASS |
| 3 | `Grep "newAPICommand\|apiCmd" internal/cli/root.go` | 0 matches | PASS |
| 4 | `Grep "openaihandler\|openaiapiservice\|newOpenAIAPIService\|ChatCompletionsPath" internal/cli/operator_helpers.go` | 0 matches | PASS |
| 5 | `Grep "openaiapi\|apiServeStub\|installStubOpenAIAPIServiceFactory\|TestRunAPIServe\|TestNewOpenAIAPIServiceCreatesService" internal/cli/extended_test.go` | 0 matches | PASS |
| 6 | `Grep -cE "^func openManageService\|...\|^func readAccountIdentity" internal/cli/operator_helpers.go` | count = 7 (exact) | PASS |
| 7 | `Grep -cE "^func Test" internal/cli/extended_test.go` | count = 32 (≥ 15 lower bound) | PASS |
| 8 | Builder worklog records pre 72.1% → post 71.8% (from `BUILDER_WORKLOG.md § Unit 1.1 — Round 1`); post ≥ 60% floor, no escalation | captured and above floor | PASS |
| 9 | `mage test` | exit 0; 353/353 tests pass across 21 packages; all packages ≥ 60% floor | PASS |
| 10 | `mage build` → `./valv --help` | exit 0; `--help` shows codex / account / global / manage / paths / version only; no `api` group | PASS |

### Coverage re-measurement

Re-ran `mage testPkg ./internal/cli` from `main/`:

- Exit 0.
- 97/97 tests pass.
- `github.com/evanmschultz/valv/internal/cli` coverage **71.8%**.
- Matches builder's reported post-change coverage exactly. Pre-change 72.1% is trusted from the worklog (not re-measurable from current tree).

### Scope verification

`git show --stat 265be0d` reports exactly the expected 6 files:

```
drops/DROP_1_DELETE_API_WRAPPER/BUILDER_WORKLOG.md |  49 +++-
drops/DROP_1_DELETE_API_WRAPPER/PLAN.md            |   2 +-
internal/cli/api.go                                | 237 --------------------
internal/cli/extended_test.go                      | 248 ---------------------
internal/cli/operator_helpers.go                   |  28 ---
internal/cli/root.go                               |   5 +-
6 files changed, 43 insertions(+), 526 deletions(-)
```

- 4 code files match PLAN Unit 1.1 `Paths` exactly (`internal/cli/api.go`, `internal/cli/root.go`, `internal/cli/operator_helpers.go`, `internal/cli/extended_test.go`).
- 2 coordination files (drop `PLAN.md` state flip + `BUILDER_WORKLOG.md` round append) are expected per WORKFLOW.md.
- No out-of-scope files touched.

### Plan-gap discovery verification

Builder flagged two orphan symbols as compile-necessity collateral of `api.go` deletion: `TestRunAPIRuntimeSweeperPrunesUntilContextCancel` and `pruneRecorder`. Re-grepped current tree:

- `Grep "TestRunAPIRuntimeSweeper|pruneRecorder" internal/cli/extended_test.go` → 0 matches.

Both are gone. The flag is accurate and the handling (deletion as compile collateral, flag to orch rather than silent scope creep) matches `main/CLAUDE.md` § "Orchestrator Role Boundaries" and the builder's spawn contract.

### BUILDER_WORKLOG.md completeness

Confirmed Unit 1.1 Round 1 section contains every required field:

- Builder agent name.
- Started timestamp.
- Files touched (with deltas).
- Mage targets + exit codes (pre + post testPkg, mage test, mage build, ./valv --help).
- Pre / post coverage (72.1% → 71.8%).
- Acceptance checklist 1-10 with PASS annotations.
- Design notes.
- Plan-gap flag.
- `## Hylla Feedback` subsection present (`N/A — no Hylla query issued`, correct per CLAUDE.md § "Code Understanding Rules" item 2 for files changing in-flight).

### Additional findings

None. Coverage stayed well above the 60% floor. The test-count drop (103 → 97) matches the six deleted tests exactly (5 PLAN-listed + 1 orphan sweeper). No unexpected packages were touched. No `api` artifacts surface in the built binary's help output.

### Hylla Feedback

N/A — this review touched only non-Go artifacts (commit metadata, markdown worklog) plus Go files covered via Grep / mage re-run. Hylla query not needed.

### Proof certificate

- **Premises:** acceptance criteria 1-10 hold; scope limited to declared `Paths` + two coordination docs; coverage above floor; orphan symbols fully removed; worklog documents evidence.
- **Evidence:** `git show --stat 265be0d`; Grep hits (all zero where required, exact 7 / 32 where a count was required); `mage testPkg ./internal/cli` rerun reports 71.8% / 97 passing; `mage test` rerun reports 353/353 / 21 packages all ≥ 60% floor; `mage build` exit 0; `./valv --help` output lacks `api` group.
- **Trace or cases:** One check per criterion; each criterion backed by its own command + output cited above; scope verified by commit diff stat.
- **Conclusion:** PASS. Unit 1.1 is complete and the builder's 10/10 claim is independently re-verifiable from current tree state.
- **Unknowns:** Pre-change 72.1% coverage figure cannot be re-measured from current state — trusted from the worklog. No other unknowns.

