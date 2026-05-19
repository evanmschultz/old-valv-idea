# PLAN QA Falsification — DROP_9 CLI Audit and Refactor — Round 3

**Verdict:** FAIL (2 surgical findings — 1 strong CONFIRMED, 1 mild CONFIRMED)

R3 fixed all three R2 findings cleanly. R3 also surfaced one fresh structural flaw that was latent in R2 and persists in R3: the `git grep "valv manage" -- magefile.go README.md` verification command in 9.4.5 AC #1 cannot detect the stale `manage update` / `manage status` strings inside `magefile.go` and `README.md`, because those strings lack the `valv` prefix. A builder who runs only the AC #1 grep can declare the unit done without touching either file.

## R2-Fix Verification

### Vec 2 (9.4.5 paths — `operator_helpers_test.go`) — REFUTED

Path list in drop-level header (line 21) and unit 9.4.5 (line 190) both list `internal/cli/operator_helpers_test.go`. The AC #1 grep filter (line 195) explicitly includes `internal/cli/operator_helpers_test.go` in its `-- ...` pathspec. Coverage holds end-to-end. Fix complete.

### Vec 8 (9.1 manage.go error strings) — REFUTED

R3 AC bullet 6 (line 61) names the exact lines (626, 962, 996 — the third 962 is the second hit on the same line) and gives the verification command `git grep "valv manage" -- internal/cli/manage.go` returning zero hits. A builder cannot reasonably miss the lines: each is named with the substitution. Fix complete.

### Vec 12 (9.5 blocked_by) — REFUTED

Chain end-to-end:

| Unit | Blocked by |
|---|---|
| 9.1 | — |
| 9.2 | 9.1 |
| 9.3 | 9.1 |
| 9.4 | 9.1 |
| 9.4.5 | 9.1, 9.2, 9.3, 9.4 |
| 9.5 | 9.4.5 |
| 9.6 | 9.5 |
| 9.7 | 9.6 |
| 9.8 | 9.7 |

Linear from 9.4.5 onward. Fix complete.

## R3-Introduced Fresh Findings

### Finding F1 — CONFIRMED (Vec 7): 9.4.5 AC #1 grep cannot detect magefile.go + README.md hits

**File:** `drops/DROP_9_CLI_AUDIT_AND_REFACTOR/PLAN.md` line 195 (Unit 9.4.5 AC bullet 1).
**Severity:** Structural — a builder following only the acceptance criteria can leave AC #4 and AC #5 unfulfilled while AC #1 reports green.

**Evidence:**

`git grep "valv manage" -- magefile.go README.md` returns ZERO hits today, before any work is done:

```
$ git grep "valv manage" -- magefile.go README.md
(no output)
```

The actual stale references in those files use the bare `manage` form:

```
README.md:36:mage run "manage status"
README.md:44:mage dev:run "manage update"
README.md:45:mage dev:run "manage status"
magefile.go:682: Value: `mage dev:run "manage update"`
```

The 9.4.5 AC #4 and AC #5 bullets correctly call out these specific lines. But AC #1 — the gating verification — uses a literal-string grep for `valv manage`, which never matches these substrings. A builder runs the grep, sees zero hits in `magefile.go` and `README.md`, and treats AC #1 as satisfied without touching either file. AC #4 and AC #5 then become "soft" — relying on the builder to read prose bullets rather than on a mechanical gate.

**Remediation:** Replace 9.4.5 AC #1 with a two-part check, OR extend it to also grep for the bare `manage` substring scoped only to the two files that use the bare form. Proposed wording:

```
1. The following two checks both pass:
   (a) `git grep "valv manage" -- internal/cli/claude.go internal/cli/codex.go internal/cli/claude_setup.go internal/cli/codex_setup.go internal/cli/operator_helpers.go internal/cli/claude_setup_test.go internal/cli/codex_setup_test.go internal/cli/codex_test.go internal/cli/extended_test.go internal/cli/operator_helpers_test.go` returns zero hits.
   (b) `git grep -nE "\"manage (update|status|account|bind|cleanup|project)\"" -- magefile.go README.md` returns zero hits.
```

Variant (b) targets the bare-`manage` form that the 9.4.5 design-prose actually describes. Without this fix, the unit's verification gate has a known false-pass.

### Finding F2 — CONFIRMED (Vec 6 extension): Surviving manage.go example blocks have no enforced AC

**Files:** `drops/DROP_9_CLI_AUDIT_AND_REFACTOR/PLAN.md` Unit 9.1 (lines 56–62) + Unit 9.4.5 AC #6 (line 207).
**Severity:** Mild — design-note prose covers the gap, but no acceptance criterion enforces it.

**Evidence:**

Unit 9.1 AC bullets 1–7 require:
- delete `newManageCommand` (AC 1, 2)
- update `root.go` examples + group ID (AC 3, 4)
- rewrite `manage_test.go` routing (AC 5)
- update `fmt.Errorf` strings in `manage.go` at lines 626, 962, 996 (AC 6)
- `mage testPkg ./internal/cli` passes (AC 7)

The surviving manage constructors (`newManageAccountCommand` at line 65, `newManageAccountInspectCommand` at line 110, `newManageAccountLoginCommand` at line 135, `newManageAccountLogoutCommand` at line 159, `newManageAccountAddCommand` at line 200, `newManageAccountListCommand` at line 235, `newManageAccountRenameCommand` at line 278, `newManageAccountDeleteCommand` at line 298, `newManageAccountCleanupCommand` at line 319, `newManageAccountSwitchCommand` at line 345, `newManageBindCommand` at line 380, `newManageProjectCommand` at line 405, `newManageProjectListCommand` at line 433, `newManageStatusCommand` at line 1175, `newManageUpdateCommand` at line 1227, `newManageCleanupCommand` at line 1338) carry cobra `Example:` blocks with `valv manage ...` strings. Verified by `git grep -n "valv manage" -- internal/cli/manage.go` — 51 of the 91 total `valv manage` hits in the repo are inside these example blocks.

Unit 9.4.5 AC #6 (line 207) explicitly excludes `manage.go`'s example blocks: "manage.go's own example blocks (cobra `Example:` string literals) are OUT OF SCOPE for this unit — they are updated in-situ by units 9.1–9.4 as each constructor is modified or deleted."

But 9.1's AC bullets do not require updating the example blocks of the **surviving** constructors. 9.1 only modifies `newManageCommand` (deletion), `root.go` (group rename + example block), and `manage.go` error strings at lines 626/962/996. The constructors that survive — `newManageAccountCommand` through `newManageCleanupCommand` — are described in 9.1's design notes (line 72) as being kept in place to be rewired in 9.2–9.4. But 9.2–9.4 AC bullets focus on adding new commands and wiring; none of them require sweeping `valv manage` out of the surviving constructors' Example blocks.

Concrete coverage gap: lines 71–82, 113–116, 138–140, 162–164, 187–188, 203–206, 238–240, 280–281, 301–302, 322–323, 348–352, 384–385, 408–409, 436–438, 1180–1181, 1233–1235, 1342–1345 in `manage.go` are all `valv manage ...` strings inside Example blocks of constructors that survive 9.1 and may or may not be rewritten during 9.2–9.4. A builder strictly following AC bullets can leave them stale, and `mage testPkg ./internal/cli` will pass because cobra Example strings are not test assertions.

**Remediation:** Add to Unit 9.4 (or wherever the constructors are last touched, before 9.5) an explicit AC bullet:

```
N. `git grep "valv manage" -- internal/cli/manage.go` returns zero hits — including all surviving constructor `Example:` blocks (`newManageAccountCommand`, `newManageAccountInspectCommand`, `newManageAccountLoginCommand`, `newManageAccountLogoutCommand`, `newManageAccountAddCommand`, `newManageAccountListCommand`, `newManageAccountRenameCommand`, `newManageAccountDeleteCommand`, `newManageAccountCleanupCommand`, `newManageAccountSwitchCommand`, `newManageBindCommand`, `newManageStatusCommand`, `newManageUpdateCommand`, `newManageCleanupCommand`).
```

OR fold the bare-`manage.go` zero-hit check into 9.4.5 AC #1 alongside the F1 remediation (cleaner — 9.4.5 becomes the single mechanical sweep gate for all `manage.go` Example blocks plus all other files plus magefile.go + README.md).

The cleanest fix combines F1 and F2: 9.4.5 AC #1 becomes a 3-part check covering (a) the cli helper files outside manage.go, (b) manage.go itself, (c) magefile.go + README.md with the bare-`manage` regex. Then 9.4.5 owns the entire mechanical-sweep gate and the design-note "in-situ" language becomes belt-and-suspenders rather than load-bearing.

## R3-Introduced Vectors Resolved

### Vec 4 — Renumbered AC bullets — REFUTED

`git grep` for `9\.1` and AC-bullet-number cross-references throughout `drops/DROP_9_CLI_AUDIT_AND_REFACTOR/PLAN.md` shows zero references to "9.1 AC bullet N" or "9.1 #N" or equivalent. The bullet renumber (old 6 → new 7 for `mage testPkg`) is internal-only; no downstream unit depends on the bullet number. Fix clean.

### Vec 5 — Drop-level paths header sync — REFUTED

Line 21 (drop-level paths header) and line 190 (unit 9.4.5 paths) both list `internal/cli/operator_helpers_test.go`. Both updated. Fix clean.

### Vec 7 — File coverage outside 9.1 + 9.4.5 — REFUTED at file level, but see F1

`git grep -l "valv manage"` across `internal cmd magefile.go README.md` returns 13 unique files. Coverage map:

| File | Covered by |
|---|---|
| `internal/cli/manage.go` | 9.1 (AC 6) |
| `internal/cli/manage_test.go` | 9.1 (AC 5) |
| `internal/cli/root.go` | 9.1 (AC 4) |
| `internal/cli/extended_test.go` | 9.1 (paths) + 9.4.5 (paths + AC 3) |
| `internal/cli/claude.go` | 9.4.5 |
| `internal/cli/claude_setup.go` | 9.4.5 |
| `internal/cli/claude_setup_test.go` | 9.4.5 |
| `internal/cli/codex.go` | 9.4.5 |
| `internal/cli/codex_setup.go` | 9.4.5 |
| `internal/cli/codex_setup_test.go` | 9.4.5 |
| `internal/cli/codex_test.go` | 9.4.5 |
| `internal/cli/operator_helpers.go` | 9.4.5 |
| `internal/cli/operator_helpers_test.go` | 9.4.5 (R3 fix) |

All 13 files have unit owners. The `magefile.go` and `README.md` cases are covered only by 9.4.5 AC #4 and AC #5 — but as F1 shows, AC #1's grep cannot detect their stale strings, weakening the gate.

Vec 7 closes at the file-coverage level. The structural verification gap is F1.

## Hylla Feedback

N/A — this is a plan-level QA pass on a markdown PLAN.md file. No Go code reviewed in this round. Evidence sources: `git diff HEAD~1`, `git grep`, `Read` on `PLAN.md`.

## Summary

| Item | Verdict |
|---|---|
| Vec 2 (operator_helpers_test.go in 9.4.5 paths) | REFUTED — fix clean |
| Vec 8 (9.1 manage.go error string AC) | REFUTED — fix clean |
| Vec 12 (9.5 blocked_by → 9.4.5) | REFUTED — fix clean |
| Vec 4 (renumber breakage) | REFUTED — no cross-references |
| Vec 5 (drop-level header sync) | REFUTED — both updated |
| Vec 6 (9.1 AC vs 9.4.5 exclusion gap for manage.go Example blocks) | CONFIRMED — F2 |
| Vec 7 (stale hits outside 9.1 + 9.4.5) | REFUTED at file level; F1 surfaced as structural flaw in AC #1 grep |

**FAIL** with two surgical findings: F1 (strong — AC #1 grep cannot detect bare-`manage` strings in magefile.go + README.md) and F2 (mild — surviving manage.go constructor Example blocks have no enforced AC). The cleanest combined remediation is rewriting 9.4.5 AC #1 as a 3-part check that covers (a) cli helper files outside manage.go, (b) manage.go zero-hit including all surviving constructor Example blocks, (c) bare-`manage` regex against magefile.go + README.md. That single edit closes both findings.
