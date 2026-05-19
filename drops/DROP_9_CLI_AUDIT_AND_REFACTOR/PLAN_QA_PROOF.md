# DROP_9 — Plan QA Proof Review (Round 2)

**Verdict:** PASS-with-one-finding

R2 successfully resolves all 5 R1 findings. One new gap surfaced: `internal/cli/operator_helpers_test.go` contains a `valv manage update claude` reference (line 152, comment string) and is not in any unit's paths nor in the drop-level paths header. This is low-severity (comment text only, not an assertion) but the 9.4.5 grep AC will not catch it.

## 1. R1 Finding Resolution Cross-Walk

### F-9.2 path expansion → RESOLVED

R2 PLAN.md `Paths` for unit 9.2 now includes (lines 81-88):

- `internal/cli/manage.go`
- `internal/cli/manage_test.go`
- `internal/domain/repository.go`
- `internal/adapters/sqlite/store.go`
- `internal/adapters/sqlite/store_test.go`
- `internal/services/manage/service.go`
- `internal/services/manage/service_test.go`

That's 7 files. R2 `Packages` (lines 89-93) covers 4 packages: `internal/cli`, `internal/domain`, `internal/adapters/sqlite`, `internal/services/manage`. Matches R1 requirement exactly.

R2 AC #1–#3 (lines 95-97) mandate:
- `DeleteBinding(ctx, projectID string, provider Provider) error` added to `BindingRepository` interface in `internal/domain/repository.go` — verified absent in current source (line 20-24 of repository.go).
- `sqlite.Store.DeleteBinding` implemented via `DELETE FROM project_bindings WHERE project_id = ? AND provider = ?` returning `domain.ErrNotFound`-wrapped on zero rows — mirrors `DeleteProfile` at `store.go:387` (verified present).
- `manage.Service.UnbindProject(ctx, provider, startPath) error` added — verified absent in current source.

No stubs. Tests required at all three layers (AC #7-#9). Coverage gate ≥70% per package via mage (AC #10). **F-9.2 resolved.**

### F-stale-strings new unit → RESOLVED

Unit 9.4.5 inserted at PLAN.md lines 175-212. Paths (lines 178-189) list 11 files:

- `internal/cli/claude.go`
- `internal/cli/codex.go`
- `internal/cli/claude_setup.go`
- `internal/cli/codex_setup.go`
- `internal/cli/operator_helpers.go`
- `internal/cli/claude_setup_test.go`
- `internal/cli/codex_setup_test.go`
- `internal/cli/codex_test.go`
- `internal/cli/extended_test.go`
- `magefile.go`
- `README.md`

AC #1 (line 192) bakes the zero-residue `git grep "valv manage"` check across exactly those 11 files. AC #2 (lines 193-200) gives concrete substitutions. AC #4 nails magefile line 682 (`"manage update"` → `"image update"` — verified present in source). AC #5 nails README lines 36/44/45 (verified: `README.md:36 mage run "manage status"`, `README.md:44 mage dev:run "manage update"`, `README.md:45 mage dev:run "manage status"`). AC #6 explicitly excludes `manage.go`'s own example blocks (handled by 9.1-9.4). **F-stale-strings resolved.**

Cross-check counts against current source via `git grep -c "valv manage"`:
- claude.go:1, codex.go:1, claude_setup.go:2, codex_setup.go:1, claude_setup_test.go:5, codex_setup_test.go:4, codex_test.go:1, extended_test.go:2, operator_helpers.go:1 — totals 18 hits in the 9.4.5 cli scope.
- magefile.go:1, README.md:3 — totals 4 hits in the support file scope.

22 hits total in 9.4.5's path scope; AC #1 demands zero after the unit lands. Consistent.

### F-globalCmd extension → RESOLVED

R2 PLAN.md unit 9.1 AC #3 (line 57) now reads:

> "The builder renames the group ID to `account` (since `manage` is gone) and updates `accountCmd.GroupID` and `globalCmd.GroupID` to match the new group ID. `globalCmd.GroupID = "manage"` MUST be re-homed — after `manageCmd` is deleted, any command still carrying `GroupID = "manage"` references a non-existent group. Builder scans `root.go` for ALL lines assigning `GroupID = "manage"` (currently: `root.go:130` for `accountCmd`, `root.go:134` for `globalCmd`) and updates each one."

Verified against `root.go`:
- Line 130: `accountCmd.GroupID = "manage"` ✓
- Line 134: `globalCmd.GroupID = "manage"` ✓

Risk note at PLAN.md line 74 also re-anchors the two line numbers. Design notes (line 69) repeat the explicit pair. **F-globalCmd resolved.**

### F-9.4 dev-decided `valv status --all` flag → RESOLVED

R2 PLAN.md unit 9.4 AC #5 (line 164):

> "`valv status --all` (new flag, not yet in tree) shows all project bindings across all providers. Implementation calls `runManageProjectList` logic. `manage project list` is DELETED (no replacement command — users get the same data via `valv status --all`)."

AC #6 (line 165) requires a test for the new `--all` path. Design notes (line 171) reinforce: "`newManageProjectCommand` and `runManageProjectList` are deleted (or left as dead code for cleanup in DROP_11 — builder decides based on effort; deleting is preferred). The `--all` flag is the canonical replacement. No other command replaces `manage project list`." **F-9.4 resolved.**

### F-9.3 flag-precedence rules → RESOLVED

R2 PLAN.md unit 9.3 AC #2 (lines 129-136):

1. `--all` mutually exclusive with `--images`/`--containers`/`--state`/`--build-cache`, with the explicit error message bound.
2. Individual scope flags are additive.
3. No-flag default equals `--all`.
4. Default is dry-run; `--apply` (or `--yes`) executes deletion.

Four explicit bullets. AC #5 (line 138) adds a test for the conflict path: `valv image cleanup --all --images` returns a flag-conflict error. **F-9.3 resolved.**

## 2. Drop-level header + chain verification

### Drop-level paths header (PLAN.md lines 5-27)

Comparing against per-unit paths:

| File | Added in R2 | Used in unit |
|---|---|---|
| `internal/cli/claude.go` | yes (line 8) | 9.4.5 |
| `internal/cli/codex.go` | yes (line 9) | 9.4.5 |
| `internal/cli/claude_setup.go` | yes (line 10) | 9.4.5 |
| `internal/cli/codex_setup.go` | yes (line 11) | 9.4.5 |
| `internal/cli/operator_helpers.go` | yes (line 12) | 9.4.5 |
| `internal/cli/claude_setup_test.go` | yes (line 18) | 9.4.5 |
| `internal/cli/codex_setup_test.go` | yes (line 19) | 9.4.5 |
| `internal/cli/codex_test.go` | yes (line 20) | 9.4.5 |
| `internal/domain/repository.go` | yes (line 21) | 9.2 |
| `internal/adapters/sqlite/store.go` | yes (line 22) | 9.2 |
| `internal/adapters/sqlite/store_test.go` | yes (line 23) | 9.2 |
| `internal/services/manage/service.go` | yes (line 24) | 9.2 |
| `internal/services/manage/service_test.go` | yes (line 25) | 9.2 |
| `magefile.go` | yes (line 26) | 9.4.5 |
| `README.md` | yes (line 27) | 9.4.5 |

Drop-level packages (lines 29-32) updated to 4 packages. Matches per-unit packages. Header is internally consistent.

### `blocked_by` chain

R2 chain (PLAN.md lines 351-361):

```
9.1 → 9.2, 9.3, 9.4 → 9.4.5 → 9.5 → 9.6 → 9.7 → 9.8
```

Per-unit `Blocked by` values:
- 9.1: — (line 61)
- 9.2: 9.1 (line 105)
- 9.3: 9.1 (line 140)
- 9.4: 9.1 (line 167)
- 9.4.5: 9.1, 9.2, 9.3, 9.4 (line 206) ✓ matches R2 brief
- 9.5: 9.1 (line 230)
- 9.6: 9.5 (line 255)
- 9.7: 9.6 (line 282)
- 9.8: 9.7 (line 307)

Chain is acyclic, fully topologically sound. 9.4.5's blockers correctly include all four upstream units. 9.5 is blocked by 9.1 alone (NOT 9.4.5) — this is correct because 9.5 touches `manage.go` + `manage_test.go` only and doesn't depend on the string-refresh landing first. The Notes block (line 362) acknowledges `internal/cli` is the shared serial spine; that's consistent. **Chain verified.**

## 3. New finding (R2-introduced gap)

### F-R2-1 — `internal/cli/operator_helpers_test.go` not in 9.4.5 scope

`git grep -c "valv manage" -- internal/cli/operator_helpers_test.go` returns 1 hit at line 152:

```go
// "2.2.0" if the dev ran `valv manage update claude` and
```

The string lives inside a comment, not in an assertion or runtime path. It is NOT in:
- Unit 9.4.5's paths (lines 178-189 omit it).
- The drop-level paths header (lines 5-27 list `operator_helpers.go` but not `operator_helpers_test.go`).
- Any other unit's paths.

After the drop closes, the comment will read `valv manage update claude` while the actual command will be `valv image update claude`. Low severity (comment-only, doesn't affect runtime or tests), but inconsistent with the "zero stale strings post-drop" intent.

**Fix:** Add `internal/cli/operator_helpers_test.go` to unit 9.4.5's paths and to the drop-level paths header. The AC #1 grep should be extended to include this file in its filter.

Severity: low.

## 4. Source-existence spot checks

- `internal/cli/root.go:130` — `accountCmd.GroupID = "manage"` confirmed.
- `internal/cli/root.go:134` — `globalCmd.GroupID = "manage"` confirmed.
- `internal/domain/repository.go:20-24` — `BindingRepository` has no `DeleteBinding`. Confirmed gap matches 9.2 AC #1.
- `internal/adapters/sqlite/store.go:387` — `DeleteProfile` present. Pattern referenced in 9.2 design notes confirmed.
- `internal/services/manage/service.go:339` — `DeleteProfile` service method present. `UnbindProject` absent. Confirmed gap matches 9.2 AC #3.
- `magefile.go:682` — `"manage update"` bootstrap label present. Confirmed target for 9.4.5 AC #4.
- `README.md:36,44,45` — three `manage status` / `manage update` mage-run lines present. Confirmed targets for 9.4.5 AC #5.

## 5. Constraints validation

- DROP_8 `--account` override: 9.5 + 9.6 normalize `--provider` only; no mention of re-implementing `--account` on `claude` / `codex`. PASS.
- No auto-open machinery: no plan elements mention auto-open or background TUI launch. PASS.
- No Tillsyn: no Tillsyn references anywhere in the plan. PASS.
- Hylla: not directly invoked in the plan (Hylla is a runtime concern, not a plan structure concern). N/A.

## 6. Verdict

**PASS-with-one-finding.**

R2 resolves all 5 R1 findings. The new chain is correct. The drop-level header is updated to reflect the expanded scope. F-R2-1 (`operator_helpers_test.go` not in scope) is a low-severity gap easily fixed by adding one path entry to unit 9.4.5 and the drop-level paths header.

## Hylla Feedback

N/A — this review touched only markdown (PLAN.md + drop docs) and live Go source for spot-checks via `Read` / `git grep`. Hylla is Go-committed-code only and not the right tool for plan-doc verification.
