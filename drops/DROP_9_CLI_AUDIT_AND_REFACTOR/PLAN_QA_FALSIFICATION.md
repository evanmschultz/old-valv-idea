# DROP_9 — Plan QA Falsification (Round 2)

**Round:** 2
**Verdict:** **FAIL** (2 CONFIRMED counterexamples + 1 CONFIRMED minor; 9 REFUTED)
**Reviewer scope:** Attack R2 revisions per 12-vector brief; evidence via `git grep`, source read, cobra docs (Context7), SQL FK inspection.

---

## Verdict Summary

| # | Vector | Verdict | Severity |
|---|---|---|---|
| R1-1 | Other binding-removal callers | REFUTED | — |
| R1-2 / Vec 2 | 9.4.5 path completeness | **CONFIRMED** | LOW |
| R1-3 | Other `GroupID = "manage"` lines | REFUTED | — |
| R1-4 | `--all` requires non-trivial service change | REFUTED | — |
| R1-5 | Flag-precedence internal consistency | REFUTED | — |
| Vec 6 | 9.4.5 chain bottleneck effect on other deps | REFUTED | — |
| Vec 7 | 9.1 vs 9.4.5 race on `extended_test.go` | REFUTED | — |
| Vec 8 | 9.4.5 scope creep — manage.go runtime error strings | **CONFIRMED** | MEDIUM |
| Vec 9 | 22-file paths header double-count | REFUTED | — |
| Vec 10 | CLI binary name regression | REFUTED | — |
| Vec 11 | Cobra orphan `GroupID` runtime behavior | REFUTED | — |
| Vec 12 | 9.4.5 numbering consistency in chain diagram | CONFIRMED (minor) | LOW |

**FAIL gates:** Vec 2 + Vec 8 are CONFIRMED with concrete reproductions. Vec 12 is a documentation inconsistency (low impact but real).

---

## CONFIRMED Counterexamples

### Vec 2 (R1-#2) — 9.4.5 path completeness gap: `operator_helpers_test.go` excluded

**Claim under attack:** Unit 9.4.5 AC #1: `git grep "valv manage" -- <11-file list>` returns zero hits. R2 brief says: "Are any stale-reference files outside the 11 listed?"

**Evidence:**

```
$ git grep -l "valv manage" -- internal cmd magefile.go README.md
internal/cli/claude_setup_test.go
internal/cli/claude_setup.go
internal/cli/claude.go
internal/cli/codex_setup_test.go
internal/cli/codex_setup.go
internal/cli/codex_test.go
internal/cli/codex.go
internal/cli/extended_test.go
internal/cli/manage_test.go
internal/cli/manage.go
internal/cli/operator_helpers_test.go     ← NOT in 9.4.5 scope
internal/cli/operator_helpers.go
internal/cli/root.go
```

13 files contain `valv manage`. The 11 files in 9.4.5 scope account for `claude_setup_test.go`, `claude_setup.go`, `claude.go`, `codex_setup_test.go`, `codex_setup.go`, `codex_test.go`, `codex.go`, `extended_test.go`, `operator_helpers.go`, `magefile.go`, `README.md`. `manage.go` is excluded by AC #6 (in-situ via 9.1–9.4). `manage_test.go`/`root.go` are 9.1 scope. **`operator_helpers_test.go` is in no unit's path list.**

Concrete stale hit:
```
internal/cli/operator_helpers_test.go:152:
    // contain "2.2.0" if the dev ran `valv manage update claude` and
```

**Reproduction:** After all 9 units close green, run:
```bash
git grep "valv manage" -- internal/cli/operator_helpers_test.go
```
Expected (per implicit drop intent): zero hits.
Actual: 1 hit on line 152 survives.

**Severity:** LOW. The stale reference is a test comment, not a user-visible string. No functional impact, but it falsifies the drop's implicit "every `valv manage` reference is purged" goal.

**Remediation suggestion:** Add `internal/cli/operator_helpers_test.go` to Unit 9.4.5's `Paths` list and to AC #1's grep file list. One-character change to the comment (`valv image update claude`).

---

### Vec 8 — 9.4.5 scope creep: live `fmt.Errorf` strings in `manage.go` reference deleted commands

**Claim under attack:** R2 brief: "9.4.5 should be string-only. But error messages may be in `fmt.Errorf` calls that ALSO need provider-name updates or other API-shape changes. Is this strictly a string-substitution unit?" The R2 plan says `manage.go` error messages are out of scope for 9.4.5 (AC #6: "manage.go's own example blocks are OUT OF SCOPE — they are updated in-situ by units 9.1–9.4 as each constructor is modified or deleted") — but the "example blocks" carve-out does NOT cover live runtime error strings.

**Evidence:** Three live `fmt.Errorf` calls in `manage.go` reference deleted commands, none of which are enumerated by any unit's AC:

```
internal/cli/manage.go:626:
  return fmt.Errorf("manage account switch: account %q not found for provider %q;
     run `valv manage account add %s %s` or `valv manage account list %s`", ...)

internal/cli/manage.go:962:
  return "", "", fmt.Errorf("account %q not found in any provider;
     run `valv manage account add codex %s` or `valv manage account list`
     to see all available accounts", accountName, accountName)

internal/cli/manage.go:996:
  return "", "", fmt.Errorf("no accounts found across any provider;
     run `valv manage account add codex <name>` or
     `valv manage account add claude <name>` to create one")
```

These three strings:
- Are NOT cobra `Example:` example blocks (those are lines 32-50, 68-83, 111-117, 137-141, etc.).
- Are live runtime error message bodies that a user actually sees on `account` resolution failure.
- Reference `valv manage account add codex`, `valv manage account add claude`, `valv manage account list`, `valv manage account add <name> <name>` — all deleted by 9.1's namespace removal.

**Per-unit AC coverage analysis:**
- **9.1 AC #1-6**: Touches `manage.go` only to delete `newManageCommand` + `runManageHome`. Does not enumerate runtime error strings on lines 626/962/996.
- **9.2 AC #1-10**: Adds `account bind`/`unbind`. Does not enumerate these strings.
- **9.3 AC #1-6**: Adds `image` namespace. Does not enumerate these strings.
- **9.4 AC #1-7**: Flattens `status`. Does not enumerate these strings.
- **9.4.5 AC #6**: Explicitly EXCLUDES `manage.go` from scope.
- **9.5 design note**: Extracts shared helper `resolveAccountByName` from `resolveAccountSwitchTarget` (lines 880+). Line 626 is in `resolveProfileSwitchTarget` (different function); line 962/996 are in `resolveManagedAccount` (different function). 9.5 AC #1-8 does NOT require updating these strings, only flag-precedence wiring + collision error format.
- **9.6 AC #1-5**: Uniform collision enforcement. Same as 9.5 — does not require updating the stale strings.

**Reproduction:** After all 9 units close green, run:
```bash
git grep "valv manage" -- internal/cli/manage.go
```
Expected: zero hits (post-namespace-deletion).
Actual: at minimum 3 hits on lines 626, 962, 996. Real-world impact: a user who runs `valv account switch nonexistent` sees:
```
manage account switch: account "nonexistent" not found for provider "codex";
run `valv manage account add codex nonexistent` or `valv manage account list codex`
```
The user runs `valv manage account add codex nonexistent` → cobra fails because `manage` is gone. **User-facing UX regression.**

**Severity:** MEDIUM. Live error messages directing users to non-existent commands is a real regression — not just a documentation drift.

**Remediation suggestion:** Either
- (a) Extend Unit 9.4.5 Paths to include `internal/cli/manage.go` and add AC #2 substitutions for lines 626/962/996; OR
- (b) Add a new explicit AC item to Unit 9.5 (which already touches `manage.go`) requiring builder to update runtime error strings in `resolveAccountSwitchTarget`/`resolveManagedAccount` to reference the new command tree as part of the helper extraction; OR
- (c) Add a new unit 9.6.5 dedicated to runtime-error-string sweep over `manage.go` (analogue to 9.4.5 for non-example strings).

Option (b) is surgical — 9.5 already extracts the helper in this code region; adding "while you're here, update the error strings" is mechanically free.

---

### Vec 12 — Dependency chain diagram contradicts Unit 9.5 `Blocked by` row (minor)

**Claim under attack:** R2 brief: "Is the 9.4.5 numbering consistent throughout PLAN.md (including the dependency chain diagram in Notes)?"

**Evidence:**

PLAN.md Notes section (line 351-361):
```
9.1 (manage deletion)
  → 9.2 (account bind/unbind — cli + domain + store + service)
  → 9.3 (image namespace — cli)
  → 9.4 (status flatten + --all flag — cli)
  → 9.4.5 (stale string refresh — cli + magefile + README)
  → 9.5 (flag normalization — cli)
  → 9.6 (collision enforcement — cli)
  → 9.7 (CONCERN A tests — cli)
  → 9.8 (CONCERN B hardening — cli)
```

This implies 9.5 `blocked_by` 9.4.5.

But Unit 9.5 row (line 230): `**Blocked by:** 9.1`. Same for 9.7 (line 282): `**Blocked by:** 9.6` (correct, not contradicting diagram). 9.5's `blocked_by` field says it only depends on 9.1, but the diagram strings 9.5 after 9.4.5.

**Interpretation 1 (diagram = strict linear chain):** 9.5 should be `blocked_by: 9.4.5`. The row is wrong.
**Interpretation 2 (rows = strict, diagram = visual aid):** Some `→` edges in the diagram are not blockers, just reading order. Then the diagram is misleading.

The drop-end builder needs to know which interpretation is canonical because:
- If interpretation 1: 9.5 cannot start until 9.4.5 finishes — the orchestrator must wait.
- If interpretation 2: 9.5 can start after 9.1 alone, and 9.5 may execute concurrently or before 9.4.5 (orchestrator constraint per "all serial due to package lock" — but the row blocker chain matters for parallel orchestration in future work).

**Severity:** LOW. The package lock makes the question academic for this drop (all sequential anyway). But the diagram-row disagreement will cause planner-builder confusion.

**Reproduction:** Read PLAN.md lines 230, 351-360. The two sources of truth disagree.

**Remediation suggestion:** Make rows canonical and either:
- Update Unit 9.5 `Blocked by: 9.1, 9.4.5` to match the linear diagram; OR
- Annotate the diagram with "(serial reading order; not all `→` are blockers)".

The first is cleaner — runtime error strings make a 9.5-after-9.4.5 dependency natural (9.5 changes string templates, 9.4.5 expects strings to match the final command tree).

---

## REFUTED Vectors

### R1-#1 — Other binding-removal callers (e.g., `account delete` cascade)

**Concern:** Does `valv account delete` need to call `UnbindProject` first before deleting a bound account?

**Evidence:** `internal/adapters/sqlite/store.go:64`:
```sql
FOREIGN KEY(project_id) REFERENCES projects(id) ON DELETE CASCADE,
FOREIGN KEY(profile_id) REFERENCES profiles(id) ON DELETE CASCADE
```

`project_bindings` cascades on `profiles(id) DELETE`. `service.DeleteProfile` → SQLite auto-removes the binding row. **No `account delete` call site needs explicit `UnbindProject`** — the FK cascade handles it. The new `UnbindProject` is for explicit user-initiated unbind only.

The full chain `domain.BindingRepository.DeleteBinding` → `sqlite.Store.DeleteBinding` → `manage.Service.UnbindProject` → CLI is complete for the explicit-unbind use case. **REFUTED.**

---

### R1-#3 — Other `GroupID = "manage"` references

**Evidence:**
```
$ git grep "GroupID" -- internal/cli/root.go
internal/cli/root.go: pathsCmd.GroupID = "inspect"
internal/cli/root.go: versionCmd.GroupID = "inspect"
internal/cli/root.go: codexCmd.GroupID = "runtime"
internal/cli/root.go: claudeCmd.GroupID = "runtime"
internal/cli/root.go: accountCmd.GroupID = "manage"   ← 9.1 re-homes
internal/cli/root.go: manageCmd.GroupID = "manage"    ← 9.1 deletes manageCmd entirely
internal/cli/root.go: globalCmd.GroupID = "manage"    ← 9.1 re-homes (explicit AC)
```

Exactly 3 `GroupID = "manage"` references; all 3 are addressed by 9.1's expanded AC #3. `manageCmd.GroupID = "manage"` is cleaned up implicitly because `manageCmd` itself is deleted. **REFUTED.**

---

### R1-#4 — `valv status --all` requires non-trivial service change

**Concern:** Is `--all` data-source infrastructure missing?

**Evidence:** `internal/services/manage/service.go:364` already has:
```go
func (s Service) ListBindings(ctx context.Context, provider domain.Provider) ([]BindingView, error)
```

`runManageProjectList` already calls it at `manage.go:1134`. Unit 9.4 only needs to add a `BoolVar("all", false, ...)` flag on the status command, dispatch to the existing listing logic. **No service-layer change needed**; `internal/services/manage` was correctly excluded from 9.4's paths. **REFUTED.**

---

### R1-#5 — Flag-precedence internal consistency for `image cleanup`

**Concern:** "default (no flags) is `--all`" vs "--all exclusive with individual scope flags" — what if user passes `--apply` only?

**Evidence:** Re-reading R2 AC text: scope flags = `--images`, `--containers`, `--state`, `--build-cache`, `--all`. `--apply` is an orthogonal action flag (dry-run vs execute). Rules:

- `valv image cleanup` (no flags) → equivalent to `--all`, dry-run.
- `valv image cleanup --apply` → equivalent to `--all`, execute. Internally consistent.
- `valv image cleanup --images --apply` → cleanup images scope, execute. Consistent.
- `valv image cleanup --all --apply` → cleanup all, execute. Consistent.
- `valv image cleanup --all --images` → error (mutual exclusion). Consistent.

No internal contradiction. **REFUTED.**

---

### Vec 6 — 9.4.5 chain bottleneck affects other `blocked_by` fields

**Concern:** Does adding 9.4.5 mid-chain affect any other unit's `blocked_by`?

**Evidence:** Unit `blocked_by` rows post-9.4.5 insertion:
- 9.5 `Blocked by: 9.1` (unchanged from R1)
- 9.6 `Blocked by: 9.5` (unchanged)
- 9.7 `Blocked by: 9.6` (unchanged)
- 9.8 `Blocked by: 9.7` (unchanged)

Only 9.4.5 itself depends on 9.1+9.2+9.3+9.4. No other unit's blocker changed. (Diagram inconsistency = Vec 12, separate finding.) **REFUTED.**

---

### Vec 7 — 9.1 vs 9.4.5 race on `extended_test.go`

**Concern:** 9.1 rewrites `newManageCommand(...)` call sites in `extended_test.go`; 9.4.5 updates error-message substrings. Same file, sequential builders.

**Evidence:** `git grep "newManageCommand\|valv manage" -- internal/cli/extended_test.go`:
- 9.1-scope: 10 `newManageCommand(...)` call sites (lines 68, 89, 150, 221, 290, 330, 355, 441, 485, 545, 601) — rewrite to `newManageAccountCommand`/`newImageCommand`/etc.
- 9.4.5-scope: 2 error-string assertions (lines 230, 414) — `strings.Contains(err.Error(), "run \`valv manage account add codex work\`")` and `"run \`valv manage account add codex\`"`.

The 12 lines are logically distinct. 9.4.5 `blocked_by: 9.1` enforces strict serial execution; no concurrent write. Builder for 9.4.5 re-greps post-9.1 to find current line numbers. **REFUTED.**

---

### Vec 9 — Drop-level paths header double-counting

**Evidence:** Union of per-unit `Paths` lists = exactly 22 unique files. Header lists 22 files. Per-unit overlap (e.g., 9.1 + 9.4 + 9.3 all touch `root.go`) is correctly deduped in the header. **REFUTED.**

---

### Vec 10 — CLI binary name regression

**Evidence:** `git grep -n "cmd/valv" -- magefile.go .github/`:
- `magefile.go:54`: `Detail: "./cmd/valv"`
- `magefile.go:58`: `runGo("build", "-o", "./valv", "./cmd/valv")`
- `magefile.go:77`: `Detail: "./cmd/valv"`
- `magefile.go:81`: `runGo("install", "./cmd/valv")`
- `.github/workflows/release.yml:41`: `go build -trimpath ... -o "${BINARY}" ./cmd/valv`

No R2 unit touches `cmd/valv/` (only `internal/`) or modifies the binary output path in `magefile.go`. `magefile.go` IS in 9.4.5 scope but only for line 682's "bootstrap label" string (`"manage update"` → `"image update"`) — not the build target. **REFUTED.**

---

### Vec 11 — Cobra orphan `GroupID` runtime behavior

**Concern:** If a command has `GroupID = "manage"` but no group is registered with that ID, does cobra silently render the command as "ungrouped" (UX regression) or error?

**Evidence:** Per cobra docs (Context7): `Command.AddCommand` validates `GroupID` against parent's registered groups. The official cobra behavior is to PANIC at `Command.Add` time with `"Group id '%s' is not defined for subcommand"`. Per `Command.ContainsGroup(groupID string) bool` (verified via `go doc`), cobra exposes the validation primitive.

R2 9.1 explicitly re-homes both `accountCmd.GroupID` and `globalCmd.GroupID` to the new `"account"` group ID after renaming `AddGroup(&cobra.Group{ID: "manage", ...})` → `AddGroup(&cobra.Group{ID: "account", ...})`. No orphan state. If the builder forgets one assignment, `mage testPkg ./internal/cli` will panic on cobra wiring → caught by 9.1's AC #6.

**Safety net is automatic.** **REFUTED.**

---

## Unknowns

None — all 12 vectors resolved to CONFIRMED, CONFIRMED-minor, or REFUTED with evidence.

---

## Hylla Feedback

N/A — Hylla not exercised this round (PLAN.md inspection + `git grep` + `go doc` + Context7 sufficed for the structured-text falsification work; no Go-symbol blast-radius questions arose).

---

## Recommended Planner Action (R3 brief sketch)

1. **Vec 8 fix (MEDIUM):** Add `internal/cli/manage.go` to Unit 9.5's `Paths` (already listed there) + add an explicit AC to Unit 9.5 requiring builder to update `fmt.Errorf` strings on lines 626, 962, 996 in `resolveAccountSwitchTarget`/`resolveManagedAccount` to reference the new command tree (`valv account add codex`, `valv account list`, etc.). Alternative: extend 9.4.5 scope to include `manage.go` runtime error strings while keeping example-block carve-out.
2. **Vec 2 fix (LOW):** Add `internal/cli/operator_helpers_test.go` to Unit 9.4.5's `Paths` + AC #1 grep file list. One stale comment to update on line 152.
3. **Vec 12 fix (LOW):** Update the Dependency chain diagram in Notes to match unit `Blocked by` rows OR update 9.5's `Blocked by: 9.1` → `Blocked by: 9.1, 9.4.5` if the linear chain reading is the intended semantics.

The two CONFIRMED gaps are surgical. Neither requires a new unit; both can land as AC additions to existing units.
