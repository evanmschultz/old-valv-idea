# DROP_9 — Build QA Falsification

## Unit 9.1 — Round 1

- **Reviewer:** go-qa-falsification-agent
- **Commit under review:** `c298ea6 refactor(cli): unit 9.1 delete valv manage namespace`
- **Mage targets exercised by reviewer:**
  - `mage testPkg ./internal/cli` — PASS (200 tests, 72.9% coverage)
  - `mage integration` — **FAIL (build error)**
- **Verdict:** FAIL — one CONFIRMED counterexample (vector 4); remediation required before unit 9.1 is `done`.

### Summary

The `valv manage` namespace deletion is structurally clean for the non-integration test surface (`mage testPkg ./internal/cli` is green), but the integration-tagged test file `internal/cli/codex_integration_test.go` still references the deleted symbol `newManageCommand` and also passes the obsolete `"manage"` argv token to the built binary at three call sites. The integration build tag is the safety net the unit-scoped `mage testPkg` cannot see, and the builder's verification list (`mage testPkg ./internal/cli` + `mage build`, both PASS) did not include `mage integration`. The breakage is mechanical and trivially reproducible.

### Per-vector findings

| # | Vector | Verdict | Notes |
|---|---|---|---|
| 1 | `runManageHome` dead code in `operator_helpers.go` | REFUTED | Only caller is `extended_test.go:81`. Production code has no `runManageHome(` call site. Builder disclosed this and routed cleanup to DROP_11. Consistent with cascade discipline (out of declared paths). |
| 2 | `newTestManageContainerCommand` test helper semantics | REFUTED | Bare container with `Args: cobra.ArbitraryArgs`, RunE returns Help, re-registers all 6 ex-children. Used by `runManage` + `runManageExpectError` helpers + `TestManageAccountSwitchMissingAccountShowsActionableGuidance`. No call site routed through `runManageHome` via this helper, so the bare RunE is sufficient. |
| 3 | GroupID rename collateral | REFUTED | `git grep "Management Commands"` returns zero. `git grep '"manage"'` in `internal/cli/` returns zero string-literal hits in production paths (the surviving `extended_test.go:70` and `manage_test.go:234` hits are inside test stubs whose `Use: "manage"` is irrelevant — they are not registered on the root tree). New title `"Account Commands"` is the only group title at `root.go:111`. |
| 4 | `extended_test.go` deletion + 8 updates — completeness | **CONFIRMED** | See "Counterexample" section below. `git grep "newManageCommand" -- 'internal/cli/*.go'` returns `internal/cli/codex_integration_test.go:357` — a survivor that the build-tag-gated test file. `mage integration` reproduces. |
| 5 | Cobra `Example:` fields preserved in surviving constructors | REFUTED | `git grep -c "valv manage" -- 'internal/cli/manage.go'` returns 58 (down from 68 pre-9.1). Drop count matches deletion of `newManageCommand`'s Example block (~12 lines `valv manage ...` content) less the 3 fmt.Errorf strings rewritten in-place at lines 589/925/959 (formerly 626/962/996). All 58 survivors are inside cobra `Example:` field string literals in surviving constructors — exactly the territory PLAN.md cedes to unit 9.4.5. |
| 6 | New `"account"` GroupID conflict | REFUTED | Group ID is the cobra group registry key, not a command name. `Use: "account"` on the promoted `newManageAccountCommand` and `GroupID: "account"` on the same command + `globalCmd` are orthogonal. Both register at `root.go:111` group + `root.go:128-131` GroupID assignments, no overlap with any other group. |
| 7 | Help-output regression — ungrouped commands at root | REFUTED | All four registered subcommands at `root.go:133` (`pathsCmd`, `versionCmd`, `codexCmd`, `claudeCmd`, `accountCmd`, `globalCmd`) get an explicit `GroupID` at `root.go:121-131`. No ungrouped command at root. Verified via `git grep "GroupID" -- 'internal/cli/'`. |
| 8 | Coverage delta (-3 tests, -0.2% coverage) accounting | REFUTED | Three deletions located: `TestManageCommandWithoutTTYShowsHelp` (extended_test.go), `TestManageAliasWorks` (root_test.go:91-105), `TestManageHelpSubcommandWorks` (root_test.go:130-146). `newTestManageContainerCommand` is a helper, not a `Test*` function, so doesn't contribute. Builder's worklog "Tests deleted" list (lines 28-31) names all 3 correctly. |

### Counterexample (vector 4)

**Claim under attack:** the deletion of `newManageCommand` is complete; `mage testPkg ./internal/cli` passing implies the unit is done.

**Concrete counterexample:**

- File: `internal/cli/codex_integration_test.go`
- Build tag: `//go:build integration` (line 1)
- Stale reference (compile-time):
  - **Line 357:** `cmd := newManageCommand(paths, &rootOptions{})` inside the `runManageForIntegration` test helper. `newManageCommand` was deleted in HEAD; this is now an undefined identifier under the `integration` build tag.
- Stale references (runtime — argv passed to built binary):
  - **Line 145:** `runValvBinaryCommand(t, binaryPath, …, "manage", "account", "add", "codex", "profile-name", …)`
  - **Line 146:** `runValvBinaryCommand(t, binaryPath, …, "manage", "bind", "codex", "profile-name")`
  - **Line 240:** `runValvBinaryCommand(t, binaryPath, …, "manage", "account", "add", "codex", "profile-name", …)`
  These invoke the built `./valv` binary with a `manage` subcommand that no longer exists in the root command tree (`root.go:133` no longer registers `manageCmd`). Even if the compile error at line 357 were fixed, these would fail at runtime with cobra's "unknown command" path.

**Reproduction:**

```
mage integration
```

Output (truncated):
```
[INFO] Started go test -json (-tags=integration -count=1 ./internal/cli)
[PKG FAIL] github.com/evanmschultz/valv/internal/cli (0.00s)
…
build errors: 1
…
Error: go test -tags=integration -count=1 ./internal/cli: exit status 1
```

**Why builder missed it:** the builder's worklog "Mage targets run" line states `mage testPkg ./internal/cli` + `mage build`. Neither exercises the `-tags=integration` build path. `mage testPkg` compiles only the default build-tag set; the integration-tagged file is excluded. `mage build` only compiles `./cmd/valv` (the binary), not test files. PLAN.md's acceptance criteria for unit 9.1 only listed `mage testPkg ./internal/cli`, so this gap is partially the planner's — but a deletion of an exported-within-package symbol owes the builder a `git grep <symbol>` on the full tree, which would have surfaced line 357 immediately. Per AGENTS.md § 12 ("Add `mage integration` when the change touches Docker-backed or external-transcript paths") this deletion does in fact touch the integration-tagged test surface and should have warranted that target.

**Severity:** BLOCK. Pre-v0.1.0 breaking changes are free in production CLI, but the test surface is not "production output"; it gates `mage integration` which is the AGENTS.md § 12 step the orchestrator must run before `mage build` + dev handoff at drop close. A failing `mage integration` blocks DROP_9 close even if every later unit passes.

### Related findings (out of 9.1's declared scope, queued for later units)

These are NOT counterexamples against unit 9.1 — they were already stale before 9.1 — but the falsification pass surfaces them because the renamed CLI surface makes them visibly inconsistent and they belong on the 9.4.5 / cleanup unit's radar.

- `internal/cli/operator_helpers.go:222` — `fmt.Errorf("no %s accounts found; run \`valv manage account add %s\` …")`. Stale `valv manage` reference in a runtime error message. Sister of the 3 manage.go strings the builder rewrote.
- `internal/cli/claude.go:218` — `fmt.Errorf("claude image %q is not built locally; run \`valv manage update claude\` first", …)`.
- `internal/cli/claude_setup.go:18, 100` — string-literal references to `valv manage account add %s` and `valv manage bind claude <name>` in error/guidance text.
- `internal/cli/codex.go:220` — `fmt.Errorf("codex image %q is not built locally; run \`valv manage update\` first", …)`.
- `internal/cli/codex_setup.go:94` — `"project is not bound to a Codex account; run \`valv manage bind codex <name>\` to bind one"`.
- Test-side assertions in `claude_setup_test.go:119, 198, 199, 276`, `codex_setup_test.go:121, 203, 204`, `codex_test.go:254`, `extended_test.go:400` — each asserts on the stale `valv manage …` substring in an error message. These will silently keep passing until the underlying production strings are rewritten; then they will start failing in lockstep. Mention in PLAN.md unit 9.4.5 scope (or open a sibling unit) so the production rewrites and test-side assertions are touched in the same diff.

### Remediation for unit 9.1

1. Update `internal/cli/codex_integration_test.go:357` (`runManageForIntegration` helper) to use `newTestManageContainerCommand(paths, &rootOptions{})` — same pattern the builder applied in `manage_test.go:255, 499` — or, cleaner, rewrite the helper to call `newManageAccountCommand` / `newManageBindCommand` directly and update the call sites at lines 51/145/146/240 to strip the leading `"manage"` argv token. The container-helper path is the minimum diff.
2. Update the three `runValvBinaryCommand(...)` invocations (lines 145, 146, 240) to drop the `"manage"` token: `"manage", "account", "add", ...` → `"account", "add", ...`; `"manage", "bind", ...` → `"bind", ...`. These exercise the built `./valv` binary, so they MUST use the new top-level command names.
3. Re-run `mage integration` until green. AGENTS.md § 12 + § 11 require this for any change that touches the integration-tagged surface.
4. Optional but recommended: extend unit 9.1's PLAN.md acceptance criteria to include `mage integration` PASS, so future builders catch this class of symbol-removal regression without relying on the QA falsification pass.

### Constraints respected by reviewer

- Did NOT edit Go code. Counterexample reported via this file.
- Did NOT edit `PLAN.md` or any sibling QA / WORKLOG file.
- Mage-only test invocations (`mage testPkg`, `mage integration`). No raw `go test` / `go build`.
- Hylla queried first; snapshot is stale (snapshot 2 = DROP_8 close), so `hylla_search_keyword "newManageCommand"` returned empty. Worked via `git grep`. Recorded in Hylla Feedback.

## Hylla Feedback

- **Query:** `hylla_search_keyword "newManageCommand"` with `artifact_ref=github.com/evanmschultz/valv@main`, `node_type=block`. **Missed because:** snapshot 2 (DROP_8 close) is stale relative to HEAD — `newManageCommand` was deleted in the commit under review and the snapshot does not yet reflect that, AND the surviving stale call site lives in a `//go:build integration`-gated file which `hide_tests` mode may filter. **Worked via:** `git grep -n "newManageCommand" -- 'internal/cli/*.go'` — returned `internal/cli/codex_integration_test.go:357` immediately. **Suggestion:** when an `id_search_mode=tail_symbol` query for a known-deleted symbol returns empty, surface the snapshot timestamp + a "this symbol was deleted in commit X" hint if the graph_ref has a fresher commit reference. Today the empty result is indistinguishable from "no such symbol ever existed."
