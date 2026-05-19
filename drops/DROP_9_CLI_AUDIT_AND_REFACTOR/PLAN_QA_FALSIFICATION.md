# DROP_9 — Plan QA Falsification — Round 1

**Verdict:** `fail` — multiple confirmed gaps in the path/scope footprint of 9.1, 9.2, 9.3, 9.4 must be closed before build can start.

**Reviewer:** go-qa-falsification-agent
**Date:** 2026-05-18
**Round:** 1
**Working dir:** `/Users/evanschultz/Documents/Code/hylla/valv/main`
**Plan reviewed:** `drops/DROP_9_CLI_AUDIT_AND_REFACTOR/PLAN.md` (HEAD-1 commit)

---

## Per-vector findings

### V1 — 9.1 scope creep / test-surface coverage — REFUTED (with caveat)

**Claim under attack:** "Delete manage namespace + redistribute children" covers `manage.go` (52K) and `extended_test.go` (31K) adequately.

**Evidence:**
- `git grep -cn newManageCommand` per file:
  - `internal/cli/extended_test.go` — **11** uses (lines 68, 89, 150, 221, 290, 330, 355, 441, 485, 545, 601).
  - `internal/cli/manage_test.go` — **9** uses (lines 40, 193, 216, 233, 274, 328, 398, 444, 477).
  - `internal/cli/root_test.go` — 0 direct `newManageCommand` calls BUT 4 cases exercising the `manage`/`m` namespace via root (lines 94, 106, 110, 154).
- All three files ARE listed in 9.1's paths (PLAN.md lines 30–33).
- AC #6 (`mage testPkg ./internal/cli` passes) is a sufficient backstop for the test rewrite — every stale routing call will fail to compile or fail to execute, forcing the builder to fix it.

**Verdict:** REFUTED. The path list IS complete for the test-file rewrite surface.

**Caveat:** The PLAN.md "Risk note" on line 55 underestimates the test-surface size — it says builder must scan three files, but the real touched-line count is 20+ routing calls plus 4 root-test passes (each potentially containing assertions on help text). Builder must budget for this; orchestrator should flag this in the build-spawn prompt as a known risk.

---

### V2 — 9.1+9.2+9.3+9.4+9.5 race on `manage.go` — REFUTED

**Claim under attack:** All five units edit `manage.go` and may conflict within `manage.go` even though they're serialized post-9.1.

**Evidence:**
- The PLAN.md's dependency chain (lines 273–276) serializes ALL eight units linearly: `9.1 → 9.2 → 9.3 → 9.4 → 9.5 → 9.6 → 9.7 → 9.8`. No two units run concurrently.
- The "no parallelism possible within this drop due to the package lock" note on PLAN.md line 276 makes this explicit.

**Verdict:** REFUTED. The decomposition is fully serial; the planner already foreclosed the parallel-race concern.

---

### V3 — 9.3 `image inspect` cheapest path — UNKNOWN (acceptable)

**Claim under attack:** Builder discretion is too wide for a v0.1.0 surface.

**Evidence:**
- PLAN.md AC 9.3.3 (line 97) leaves the implementation choice to the builder: `service.EnsureLatest` with `AllowExistingOnCheckFail: true` OR a new `service.InspectImage` method.
- Pre-v0.1.0 timing means breaking changes are free — the surface can be adjusted in DROP_10/11 if the chosen path proves wrong.
- The design note (lines 109–110) gives the builder a concrete preference: reuse `EnsureLatest` first, only add a service method if insufficient.

**Verdict:** UNKNOWN but ACCEPTABLE. Pre-v0.1.0 latitude + clear builder default = low risk.

---

### V4 — 9.5 + 9.6 collapse — REFUTED

**Claim under attack:** 9.5 and 9.6 edit the same `resolveAccountByName` helper; should be combined.

**Evidence:**
- PLAN.md "Units 9.5 + 9.6 potential collapse" note (line 266–267) and AC 9.6 footnote (line 179) explicitly authorize the orchestrator to collapse 9.5 + 9.6 into one build at build time if the builder finds them fitting cleanly.
- The planner has already considered this and accommodated it.

**Verdict:** REFUTED.

---

### V5 — 9.7 could run earlier — REFUTED

**Claim under attack:** 9.7 is test-only (`claude_auth_test.go`); could run in parallel with 9.2–9.6.

**Evidence:**
- 9.7's path list is `internal/cli/claude_auth_test.go` — package-scope-locked to `internal/cli`, same package as 9.1–9.6.
- Per PLAN.md line 276, the WHOLE package is serialized — Go's compiler treats a package as a unit; concurrent edits would race at the file-system / build level.
- Theoretical reordering 9.7 ahead of 9.5/9.6 would be free of conflict (different symbols), but the gain is zero on a solo-dev linear pipeline.
- The chosen ordering matches the spec's narrative arc (CLI surface first, then concerns A & B).

**Verdict:** REFUTED. Per-package serialization rules out parallelism; reordering offers no value.

---

### V6 — 9.7 vs 9.8 file conflict — REFUTED

**Claim under attack:** 9.8 changes `claude_auth.go` production code, potentially breaking 9.7's tests.

**Evidence:**
- 9.7's NEW test `TestSystemClaudeAccountAuthRunnerBuildsExpectedContainerRequest` (PLAN.md lines 190–203) exercises `systemClaudeAccountAuthRunner.RunInContainer` directly using `stubAuthContainerExecutor`. It asserts `ContainerRunRequest` fields.
- 9.8 modifies `ensureClaudeAccountReady` and `loginClaudeAccount` (the CALLERS of `RunInContainer`), NOT the runner struct itself.
- 9.8's logic: only suppress `RunInContainer` error when `.credentials.json` is present; propagate when absent. The existing `TestEnsureClaudeAccountReadyFailsWhenContainerRunFails` (claude_auth_test.go:174) exercises the no-creds branch which 9.8 preserves explicitly (AC 9.8.4 lines 227–228).
- 9.7 and 9.8 touch DIFFERENT functions in `claude_auth.go`; no overlap.

**Verdict:** REFUTED. Tests are independent; preservation is explicit in AC 9.8.4.

---

### V7 — `UnbindProject` service method missing — CONFIRMED (blocking)

**Claim under attack:** 9.2 adds a method to `internal/services/manage/Service` but `internal/services/manage/service.go` is NOT in 9.2's declared paths.

**Evidence:**
- `git grep -n "UnbindProject\|DeleteBinding\|RemoveBinding" -- internal/**` returns **zero hits** — no such method exists in `internal/services/manage/Service`.
- `internal/services/manage/service.go` has `BindProject` (line 194) but no inverse.
- 9.2 AC #2 (PLAN.md line 68) says: "If the manage service does not expose `UnbindProject` yet, the builder may stub as `not yet implemented` and note the gap — but the preferred path is to implement it against `service.DeleteBinding` or equivalent (builder verifies service surface)."
- 9.2's declared paths (PLAN.md lines 63–64) are `internal/cli/manage.go` and `internal/cli/manage_test.go` ONLY. The service file is OUT OF SCOPE.

**Implication:** If the builder takes the "preferred path," they must edit `internal/services/manage/service.go` — outside declared paths — and likely add tests in `internal/services/manage/service_test.go` — also outside scope. The "stub as not yet implemented" fallback is a half-measure that ships a broken `valv account unbind` command on master (pre-v0.1.0 is forgiving, but documenting "stub command" in v0.1.0 release notes is poor form).

**Verdict:** CONFIRMED. The planner must either:
1. Add `internal/services/manage/service.go` + `internal/services/manage/service_test.go` to 9.2's paths and revise the AC to require a real `UnbindProject` implementation, OR
2. Split into a new sub-unit (e.g., 9.2a Service `UnbindProject` method + tests; 9.2b CLI `account unbind` command).

Option (1) is cleaner. The PLAN.md "preferred path" language already hints at this — formalize it.

---

### V8 — `runManageBindCommand` ownership blur between 9.1 and 9.2 — REFUTED

**Claim under attack:** 9.1 "redistributes" `newManageBindCommand` but 9.2 owns the new `account bind`.

**Evidence:**
- PLAN.md 9.1 design note (line 51) explicitly keeps `newManageBindCommand` after manage deletion: "Keep ALL children of `newManageCommand` ... in `manage.go` — they will be wired to the new `account` and `image` top-level commands in units 9.2–9.4."
- 9.2 design note (lines 76–80) gives the builder two implementation options for `account bind`: rename the existing constructor + adjust positional, OR create a thin new constructor that wraps `runManageBind`. Option (b) is preferred and surgical.
- The two units are CLEANLY separated: 9.1 leaves `newManageBindCommand` IN PLACE; 9.2 adds the new constructor `newManageAccountBindCommand` alongside.

**Verdict:** REFUTED.

---

### V9 — `extended_test.go` pre-flight scan — REFUTED

**Claim under attack:** 31K of test code is too high-risk for build-time discovery.

**Evidence:**
- Evidence under V1: `extended_test.go` has 11 `newManageCommand` call sites + 2 hard-coded `valv manage ...` assertions (lines 230, 414).
- 9.1 AC #6 (`mage testPkg ./internal/cli` passes) is a strict gate: any uncaught path will fail compile or fail test.
- The builder is required to scan all test files for routing call sites before declaring done (PLAN.md "Risk note" line 55).

**Verdict:** REFUTED. AC #6 + builder discipline cover this. The 31K is intimidating but mechanical.

---

### V10 — Coverage delta below 70% floor — UNKNOWN (acceptable)

**Claim under attack:** Test code rewrites may drop `internal/cli` coverage below the AGENTS.md § 11 70% floor.

**Evidence:**
- DROP_8 closed `internal/cli` at **73.1%** (203 tests).
- `mage test` enforces the 70% per-package gate — CI will fail if coverage drops below.
- The deletions in 9.1 remove test code AND source code (e.g., `runManageHome`). If the deletion ratio is preserved, coverage stays roughly constant.
- New tests in 9.2 (bind/unbind), 9.3 (image update routing), 9.4 (valv status), 9.5 (collision error path), 9.7 (new container-request assertion), 9.8 (creds-present-despite-error) ADD coverage.

**Verdict:** UNKNOWN but ACCEPTABLE. The `mage test` gate is authoritative. Recommend adding a per-unit AC: "Final `mage testPkg ./internal/cli` reports coverage ≥ 70%". The drop-end `mage test` gate already enforces this for the package, so this is a quality-of-life ask, not blocking.

---

### V11 — DROP_8 `--account` override preservation — REFUTED

**Claim under attack:** DROP_9 might regress `stripAccountFlag` / `ensureClaudeBindingReady` / `ensureCodexAccountReadyForLaunch`.

**Evidence:**
- DROP_8 symbols live in: `account_flag.go`, `account_flag_test.go`, `claude.go` (line 60, 76 — uses `stripAccountFlag` + `ensureClaudeBindingReady`), `codex.go`, `claude_setup.go`, `codex_setup.go`.
- DROP_9's paths cover ONLY: `root.go`, `manage.go`, `claude_auth.go`, `claude_auth_test.go`, `manage_test.go`, `root_test.go`, `extended_test.go`.
- The DROP_8 symbol surface (`account_flag.go`, `claude.go`, `codex.go`, `claude_setup.go`, `codex_setup.go`) is OUTSIDE DROP_9's declared paths.
- No DROP_9 unit AC mentions touching `stripAccountFlag` or the `Ensure*ReadyForLaunch` helpers.

**Verdict:** REFUTED. DROP_9's path discipline already excludes the `--account` override surface.

---

### V12 — Mage-target stability — CONFIRMED (blocking-LITE)

**Claim under attack:** `magefile.go` shells out to `valv manage ...` via dev helpers.

**Evidence:**
- `git grep "valv manage\|manage update" magefile.go` returns **1 hit** at line 682:
  ```
  {Label: "bootstrap", Value: `mage dev:run "manage update"`, Identifier: true},
  ```
- This is a `printDevHomeMessage` field shown to the dev when they run `mage dev:home` or `mage dev:reset`. It instructs them to bootstrap their disposable dev-home by running `valv manage update`.
- Post-DROP_9, `valv manage update` doesn't exist — the bootstrap instruction is dead.
- `magefile.go` is NOT in any DROP_9 unit's declared paths.

**Verdict:** CONFIRMED. Either:
1. Add `magefile.go` to 9.3's paths (since 9.3 adds `valv image update`), and update the bootstrap label to `mage dev:run "image update"`, OR
2. Add a dedicated sub-unit (or close-out task in 9.3) to update the magefile dev-helper string.

The "bootstrap label" is dev-tooling-only (not a CI target), so this is BLOCKING-LITE — must be fixed before drop close, but doesn't gate per-unit build-QA.

---

### V13 — Hylla dependence — REFUTED

**Claim under attack:** Plan-QA should not depend on Hylla.

**Evidence:** This entire review was conducted via `git grep`, `Read`, file-system inspection. Zero Hylla calls. The planner's `## Hylla Feedback` will be a separate concern; not relevant to plan-QA verdict.

**Verdict:** REFUTED (not a real attack vector; included for completeness).

---

## Additional findings beyond the 13 vectors

### F1 — Stale `valv manage ...` error/help strings in non-test source — CONFIRMED (blocking)

**Discovered via:** `git grep "valv manage" -- internal cmd`.

**Evidence:**
- `internal/cli/claude.go:218` — `"claude image %q is not built locally; run `valv manage update claude` first"`
- `internal/cli/codex.go:220` — `"codex image %q is not built locally; run `valv manage update` first"`
- `internal/cli/claude_setup.go:18` — `"... run `valv manage account add %s` to create one"`
- `internal/cli/claude_setup.go:100` — `"... run `valv manage bind claude <name>` to bind one"`
- `internal/cli/codex_setup.go:94` — `"... run `valv manage bind codex <name>` to bind one"`
- `internal/cli/operator_helpers.go:222` — `"... run `valv manage account add %s` for the default host-backed account..."`
- `internal/cli/extended_test.go:230` and `:414` — test assertions on these exact strings (will keep passing because they assert substring of source string)

**Test-file assertions** that will keep passing (because source still emits the substring) — these are tests that LOCK IN the stale UX:
- `claude_setup_test.go:119, 198, 199, 276` — five assertions on `valv manage account add claude` / `valv manage bind claude`.
- `codex_setup_test.go:121, 203, 204` — three assertions on `valv manage account add codex` / `valv manage bind codex`.
- `codex_test.go:254` — one assertion on `valv manage update`.

**Implication:** Post-DROP_9, `mage test` will pass green even though every user-facing error message directs the user to `valv manage ...`, which no longer exists. This is the **highest-impact gap** in the decomposition.

**Affected files NOT in any DROP_9 unit's paths:**
- `internal/cli/claude.go`
- `internal/cli/claude_setup.go`
- `internal/cli/claude_setup_test.go`
- `internal/cli/codex.go`
- `internal/cli/codex_setup.go`
- `internal/cli/codex_setup_test.go`
- `internal/cli/codex_test.go`
- `internal/cli/operator_helpers.go`
- `internal/cli/operator_helpers_test.go` (comment-only, low priority)

**Remediation:** Add a NEW unit (call it 9.X "Refresh user-facing error/help strings post-namespace-rename") with paths covering all six source files + three test files. AC: every `valv manage ...` literal in non-deleted source is replaced with its DROP_9 equivalent (`valv account add` / `valv account bind` / `valv image update`), and test assertions are updated in parallel.

This unit can run AFTER 9.4 (status flatten) once the new command surface is stable. Suggested position: 9.5 → 9.6 → **9.X (error-string refresh)** → 9.7 → 9.8.

### F2 — README.md stale `mage` examples — CONFIRMED (blocking-LITE)

**Discovered via:** `git grep "manage update\|manage cleanup\|manage status\|manage bind\|manage account" README.md`.

**Evidence:**
- `README.md:36` — `mage run "manage status"`
- `README.md:44` — `mage dev:run "manage update"`
- `README.md:45` — `mage dev:run "manage status"`

**Implication:** Post-DROP_9, the README directs users to commands that don't exist.

**Remediation:** Fold a README refresh into the same 9.X unit above, or treat it as a close-out task in DROP_9's final phase. README.md is NOT in any DROP_9 unit's paths.

### F3 — Service-side `internal/services/manage/service.go` extension scope — CONFIRMED (see V7)

Same finding as V7; just re-emphasizing the path-list addition is required for 9.2.

### F4 — `newGlobalCommand` NOT addressed in plan — UNKNOWN

**Discovered via:** Reading `internal/cli/root.go:133`.

**Evidence:**
- `root.go` registers THREE top-level commands under the `manage` group: `accountCmd`, `manageCmd`, `globalCmd`.
- DROP_9's plan addresses `accountCmd` (kept, line 38 of PLAN.md) and `manageCmd` (deleted, AC 9.1.2).
- `globalCmd` (`valv global switch ...`) is NOT mentioned in the plan.

**Implication:** After 9.1 deletes the `manage` group, `globalCmd.GroupID = "manage"` (root.go:134) will point at a defunct group ID. Cobra likely tolerates a stale GroupID silently (renders ungrouped), but this is sloppy.

**Remediation:** Add to 9.1 AC: "After removing the `manage` group, re-home `globalCmd.GroupID` to an appropriate group (likely `inspect` since `global` is host-state inspection/manipulation), OR rename the existing `manage` group ID to `account` and keep `globalCmd` under it." The planner already anticipates the GroupID rename in 9.1 design note line 50 ("rename to `'account'` since `manage` is gone") but only for `accountCmd`. Extend the AC to cover `globalCmd` too.

---

## Verdict summary

**Falsification verdict:** `fail`

**Counterexamples (CONFIRMED, blocking):**
- V7 — `UnbindProject` service method missing; 9.2 paths must include `internal/services/manage/service.go` + test file.
- F1 — Stale `valv manage ...` error/help strings in `claude.go`, `claude_setup.go`, `codex.go`, `codex_setup.go`, `operator_helpers.go`, plus 9 test assertions in `*_setup_test.go` + `codex_test.go`. Add a new unit or extend an existing one.
- F4 — `globalCmd.GroupID` left dangling post-9.1.

**Counterexamples (CONFIRMED, blocking-LITE):**
- V12 — `magefile.go:682` bootstrap label says `manage update`; fix in 9.3 or via close-out task.
- F2 — `README.md:36, 44, 45` reference stale `manage` commands; fold into 9.X.

**Refuted:** V1, V2, V4, V5, V6, V8, V9, V11, V13.

**Unknown but acceptable:** V3 (image inspect implementation), V10 (coverage gate is enforced by mage).

---

## Remediation suggestions for next planner round

1. **Extend 9.2's paths:**
   - Add `internal/services/manage/service.go` and `internal/services/manage/service_test.go`.
   - Revise AC #2 to require a real `UnbindProject` implementation (not a stub).
   - Add an AC for the new service-side test: at least one test covers `UnbindProject` happy path + at least one error case.

2. **Introduce a new unit 9.X — "Refresh user-facing strings post-namespace-rename":**
   - **State:** todo
   - **Paths:**
     - `internal/cli/claude.go`
     - `internal/cli/claude_setup.go`
     - `internal/cli/claude_setup_test.go`
     - `internal/cli/codex.go`
     - `internal/cli/codex_setup.go`
     - `internal/cli/codex_setup_test.go`
     - `internal/cli/codex_test.go`
     - `internal/cli/operator_helpers.go`
     - `internal/cli/operator_helpers_test.go` (comment-only)
     - `magefile.go` (bootstrap label)
     - `README.md` (mage examples)
   - **Acceptance:**
     1. Every `valv manage ...` literal in `internal/cli/*.go` (non-deleted) is replaced with its DROP_9 equivalent: `valv account add` / `valv account bind` / `valv image update`.
     2. Test assertions in `claude_setup_test.go`, `codex_setup_test.go`, `codex_test.go` are updated to match.
     3. `magefile.go:682` bootstrap label updated to `mage dev:run "image update"`.
     4. README.md mage examples updated.
     5. `git grep "valv manage" -- internal cmd magefile.go README.md` returns zero hits.
     6. `mage test` passes.
   - **Blocked by:** 9.4 (status flatten — last unit that introduces a new command surface)
   - **Position in chain:** 9.4 → 9.X → 9.5 → 9.6 → 9.7 → 9.8 (OR collapse with 9.4 if planner prefers).

3. **Extend 9.1 AC #2 / design notes:**
   - Add: "After removing the `manage` group, re-home `globalCmd.GroupID` to `inspect` or rename the existing `manage` group ID to `account` and keep `globalCmd` under it."

4. **Add a per-unit AC for coverage:** "Final `mage testPkg ./internal/cli` reports coverage ≥ 70%" — pro-forma since the drop-end `mage test` gate enforces it, but explicit makes the floor unambiguous.

5. **Update 9.1's risk note:** mention the 9 + 11 + 4 = 24 routing/assertion call sites across the three test files so the builder budgets accordingly.

---

## Hylla Feedback

N/A — review used `git grep` + `Read` exclusively; no Hylla queries attempted. The planner's reported Hylla noise on cobra command-tree lookups suggests Hylla's keyword search struggles with cobra-style `cmd.AddCommand(...)` patterns. `Read`-first was the right call for this drop's verification and remains so until Hylla's Go-cobra summarization improves.
