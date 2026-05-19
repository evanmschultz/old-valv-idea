# DROP_9 — Builder QA Proof

## Unit 9.1 — Round 1

- **QA agent:** go-qa-proof-agent
- **Reviewed commit:** c298ea6 `refactor(cli): unit 9.1 delete valv manage namespace`
- **Verdict:** PASS

### Verification matrix

| AC | Claim | Evidence | Result |
|---|---|---|---|
| #1 | `newManageCommand` deleted | `git grep "func newManageCommand"` → 0 hits | PASS |
| #2 | `manageCmd` removed from root.go | `git grep "manageCmd" -- internal/cli/root.go` → 0 hits | PASS |
| #3 | Group `"manage"` → `"account"` re-homed | `root.go:108-112` AddGroup defines `{ID: "account", Title: "Account Commands"}`; `root.go:129` `accountCmd.GroupID = "account"`; `root.go:131` `globalCmd.GroupID = "account"`. No `"manage"` group ID remains. | PASS |
| #4 | root.go Example block scrubbed | `git grep "valv manage" -- internal/cli/root.go` → 0 hits | PASS |
| #5 | `newTestManageContainerCommand` routes correctly | `manage_test.go:232-248` constructs bare container Cmd with `Use: "manage"` then `AddCommand(newManageAccountCommand, newManageBindCommand, newManageProjectCommand, newManageStatusCommand, newManageUpdateCommand, newManageCleanupCommand)`. Test runners at `manage_test.go:255`, `:499`, `extended_test.go:207` route through this helper. Existing test args (`["account", "add", ...]`, `["bind", ...]` etc.) work unchanged. | PASS |
| #6 | 3 fmt.Errorf user-facing strings updated to `valv account …` | `manage.go:589` `valv account add %s %s` / `valv account list %s`; `manage.go:925` `valv account add codex %s` / `valv account list`; `manage.go:959` `valv account add codex <name>` / `valv account add claude <name>`. Verbatim match to builder worklog citations. | PASS |
| #7 | mage GREEN reproduced | `mage testPkg github.com/evanmschultz/valv/internal/cli` → 200/200 PASS, 72.9% coverage, `-race` on, 5.72s. `mage build` → SUCCESS Built `./valv`. | PASS |
| #8 (out-of-scope) | `valv manage` in manage.go Example blocks NOT touched | `git grep -c "valv manage" -- internal/cli/manage.go` → 58 (correctly preserved for 9.4.5) | PASS |

### Out-of-scope leakage (informational, NOT findings)

The following `valv manage` references survive in 9.1's non-touched files and are explicitly deferred:

- `internal/cli/claude.go:218`, `internal/cli/codex.go:220` — image-not-built error message.
- `internal/cli/claude_setup.go:18,100`, `internal/cli/codex_setup.go:94` — project-not-bound guidance.
- `internal/cli/operator_helpers.go:222` — `realPickProfile` no-accounts-found guidance (worklog § 65).
- `internal/cli/extended_test.go:400` — assertion paired with the operator_helpers.go string above.
- `internal/cli/*_setup_test.go`, `internal/cli/codex_test.go:254`, `internal/cli/manage_test.go:638` (comment) — assertions paired with the strings above.

All of these live outside 9.1's declared `paths` (`internal/cli/manage.go`, `root.go`, `manage_test.go`, `root_test.go`, `extended_test.go`'s 9.1-relevant tests). Worklog explicitly defers them to 9.4.5. Cascade-discipline compliant.

### Dead code (informational)

- `runManageHome` in `internal/cli/operator_helpers.go:124` is now unreachable from the CLI (the only runtime caller, `newManageCommand`, was deleted). The function is still called by `extended_test.go:81` (`TestRunManageHomeWithoutTTYShowsHelp`) which acts as the deliberate anchor. Worklog § 20, § 60 declares this for 9.4.5 / DROP_11. Cascade-discipline compliant.

### Hylla Feedback

None — Hylla was not required for this proof review; the verification is structural (grep + targeted Read) and committed-state evidence was sufficient via `git grep` / `git log` / `Read` / `mage`.

### Conclusion

All 7 acceptance criteria PASS. Out-of-scope verification PASS. Builder's cascade-discipline declarations (dead `runManageHome`, surviving Example blocks in `manage.go`, paired error strings in other files) are correctly scoped and explicitly deferred. mage GREEN reproduced.

**Verdict: PASS**

## Unit 9.1 — Round 2

- **QA agent:** go-qa-proof-agent
- **Reviewed commit:** 1b5f712 `fix(cli): unit 9.1 r2 integration test fix-up + skip pending 9.2`
- **Scope:** R2+R3 combined fix-up addressing the R1 falsification finding (3 argv strips + 1 symbol rename + 1 t.Skip).
- **Verdict:** PASS

### R1 findings → R2/R3 verification

| # | R1 finding | R2/R3 claimed fix | Evidence | Result |
|---|---|---|---|---|
| 1 | Compile break at `codex_integration_test.go:357` referencing dead `newManageCommand` | Renamed to `newTestManageContainerCommand` | Line 358 (post-edit) reads `cmd := newTestManageContainerCommand(paths, &rootOptions{})`. `rg "newManageCommand" internal/cli/` → only 1 hit, a doc-comment in `manage_test.go:231` (not a call site). | PASS |
| 2 | 3 `runValvBinaryCommand` call sites still pass `"manage"` as first argv | Strip `"manage"` token from lines 145, 146 (now 146, 147), and 240 (now 241) | Line 146: `..., "account", "add", "codex", ...` — leading `"manage"` removed. Line 147: `..., "account", "bind", "codex", ...` — leading `"manage"` removed (and `"bind"` rewritten as `"account", "bind"` to match the new flat surface). Line 241: `..., "account", "add", "codex", ...` — leading `"manage"` removed. | PASS |
| 3 | End-to-end TTY test would still fail because `valv account bind` is not implemented until 9.2 | Add `t.Skip(...)` as first statement of `TestCodexCommandRunsFixtureImageWithTTYEndToEnd` | Line 113 (function body begins line 112 `func ...`): `t.Skip("requires valv account bind from DROP_9 Unit 9.2 — re-enable when 9.2 lands")`. Skip is the FIRST statement, before `paths := testCodexPaths(t)`. | PASS |
| 4 | (implicit) Skip must name 9.2 so future builders can re-enable | Skip string explicitly references 9.2 | Skip message: `"requires valv account bind from DROP_9 Unit 9.2 — re-enable when 9.2 lands"`. Names both `9.2` and the command (`valv account bind`) and the re-enable trigger. | PASS |
| 5 | (implicit) All 3 mage gates must reproduce GREEN | Re-run | `mage testPkg ./internal/cli` → 200/200 passed, 72.9% coverage, 5.39s. `mage integration` → 202 passed + 1 skipped + 0 failed, 26.52s; the 1 skip is exactly `TestCodexCommandRunsFixtureImageWithTTYEndToEnd`. `mage build` → SUCCESS Built `./valv`. | PASS |

### Falsification probes (each mitigated)

- **Probe:** Is the Skip on the right function? It must guard `TestCodexCommandRunsFixtureImageWithTTYEndToEnd` specifically. **Mitigation:** Read confirms line 112 `func TestCodexCommandRunsFixtureImageWithTTYEndToEnd(t *testing.T) {` immediately followed by line 113 `t.Skip(...)`. Correct function. `mage integration` skip output names exactly this test.
- **Probe:** Could there be a different test function in `codex_integration_test.go` still calling `valv ... bind` that needs the same skip? **Mitigation:** `runValvBinaryCommand` on line 241 (the non-TTY variant `TestCodexCommandRunsFixtureImageWithNoAltScreenEndToEnd` or sibling) calls only `"account", "add"` post-R2, NOT `"account", "bind"`. So no other test reaches the unimplemented bind command. Confirmed by the diff: only one `bind` argv call site exists, and it's inside the skipped TTY test.
- **Probe:** Could `newManageCommand` still be referenced elsewhere causing a hidden compile break under `-tags=integration`? **Mitigation:** `rg "newManageCommand" internal/cli/` → 1 hit, `manage_test.go:231`, a `//` doc-comment. Not a call site. `mage integration` recompiled the package and passed — confirming no integration-tag-gated reference remains.
- **Probe:** Could the leftover `"manage"` string literals in `extended_test.go:70` and `manage_test.go:234` be argv tokens we missed? **Mitigation:** Both occurrences are inside `cobra.Command{Use: "manage"}` literal-struct definitions inside `newTestManageContainerCommand` and a similar test helper — they declare the test-only container command's Use-string, not argv passed to a production CLI. Correct.
- **Probe:** Did the fix-up introduce a new gap elsewhere (e.g., `TestCodexCommandRunsFixtureImageEndToEnd` at line 110 also got a Skip but for `account add` — does it need different treatment)? **Mitigation:** That Skip was already in place pre-R2 (R1 worklog covered it) — verified by reading the R2 diff, which shows only ONE `t.Skip` added at line 113. The line 110 Skip is unrelated to this round. Not a new gap.

### Hylla Feedback

None — Hylla was not required for this proof review. Verification is structural (`git diff` + `Read` + `rg` over committed code) and committed-state evidence sufficed via `git diff HEAD~1` / `Read` / `mage`. The reviewed file is `_test.go` which Hylla deprioritizes anyway.

### Conclusion

R1 falsification finding fully resolved. All 5 sub-criteria for the R2/R3 combined fix-up have evidence. mage 3-gate GREEN reproduced (`testPkg` 200/200 at 72.9%, `integration` 202+1skip+0fail, `build` SUCCESS). No new gaps introduced. The single test skip is correctly scoped, correctly named, and explicitly references Unit 9.2 as the re-enable trigger.

**Verdict: PASS**

## Unit 9.2 — Round 1

- **QA agent:** go-qa-proof-agent
- **Reviewed commit:** `828d575` `feat(cli): unit 9.2 add valv account bind and unbind`
- **Verdict:** PASS

### Verification matrix

| AC | Claim | Evidence | Result |
|---|---|---|---|
| #1 | `BindingRepository.DeleteBinding` added | `git diff HEAD~1 -- internal/domain/repository.go` → `+DeleteBinding(ctx context.Context, projectID string, provider Provider) error` at line 22. Signature matches composite PK `(project_id, provider)`. | PASS |
| #2 | `sqlite.Store.DeleteBinding` impl | `store.go:494-514` (read post-edit): `DELETE FROM project_bindings WHERE project_id = ? AND provider = ?`, `RowsAffected()` check, `affected == 0` → wraps `domain.ErrNotFound`. Pattern matches `DeleteProfile`. Error wrapping via `%w`. | PASS |
| #3 | `manage.Service.UnbindProject` impl | `service.go:228-249` (read post-edit): detects project via `s.detect(startPath)` → `s.store.ProjectByRoot(ctx, projectResult.Root)` → `s.store.DeleteBinding(ctx, projectRecord.ID, provider)`. Errors wrapped descriptively at each boundary. `ErrNotFound` preserved through `errors.Is` chain. | PASS |
| #4 | `valv account bind <name> [--provider]` | `manage.go:330-386` (`newManageAccountBindCommand`). Use: `"bind [provider] <account>"`, `Args: cobra.RangeArgs(1, 2)`. Two-positional form (matches integration test pattern); one-positional + `--provider` flag form; provider defaults to Codex when both absent. Calls `runManageBind`. | PASS |
| #5 | `valv account unbind [--provider]` | `manage.go:388-413` (`newManageAccountUnbindCommand`) + `manage.go:415-450` (`runManageAccountUnbind`). `Args: cobra.NoArgs`, `--provider` defaults to Codex, calls `service.UnbindProject`. Error paths wrapped via `fmt.Errorf("account unbind: %w", err)`. Output via `output.WriteRecord` with provider + project fields. | PASS |
| #6 | Wired into `newManageAccountCommand` | `manage.go:51-52` (read): `cmd.AddCommand(newManageAccountBindCommand(paths, opts))` + `cmd.AddCommand(newManageAccountUnbindCommand(paths, opts))` registered before existing inspect/login/etc. subcommands. | PASS |
| #7 | CLI tests cover bind+unbind happy paths | `manage_test.go:184-225` (`TestManageAccountBindWithTwoPositionalsBindsProject`): real `.git` marker dir + `runManage(account add … --no-bind)` + `runManage(account bind codex profile-name --project workDir)` → asserts output contains `account=profile-name` + `provider=codex`, then verifies `BindingByProjectID` returns a binding via real sqlite store. `manage_test.go:227-260` (`TestManageAccountUnbindRemovesBinding`): same pattern, then unbinds, asserts `BindingByProjectID` returns error post-unbind. Both `t.Parallel()`. | PASS |
| #8 | sqlite tests for DeleteBinding | `store_test.go:381-409` (`TestStoreDeleteBindingRemovesBoundRow`): creates project + profile + binding via real store, calls `DeleteBinding`, asserts `BindingByProjectID` → `errors.Is(err, domain.ErrNotFound)`. `store_test.go:411-419` (`TestStoreDeleteBindingReturnsErrNotFoundWhenAbsent`): calls `DeleteBinding` against non-existent project, asserts `errors.Is(err, domain.ErrNotFound)`. Both `t.Parallel()`. | PASS |
| #9 | manage service tests for UnbindProject | `service_test.go:751-835` adds three tests: `TestUnbindProjectRemovesBoundProjectBinding` (happy path; verifies via `Status() → ErrUnboundProject`), `TestUnbindProjectReturnsErrNotFoundWhenProjectHasNoBinding` (project record exists, binding absent), `TestUnbindProjectReturnsErrWhenProjectNotKnown` (project entirely unknown). All three `t.Parallel()`, real sqlite store, `errors.Is` against `domain.ErrNotFound`. Provider default verified implicitly via direct `ProviderCodex` calls. | PASS |
| #10 | 70% coverage floor on all 3 packages | `mage testPkg ./internal/cli` → 202/202 PASS @ 72.6%. `mage testPkg ./internal/adapters/sqlite` → 21/21 PASS @ 78.4%. `mage testPkg ./internal/services/manage` → 28/28 PASS @ 75.8%. All `-race` on, all ≥70%. | PASS |
| #11 | `t.Skip` removed + integration GREEN | `codex_integration_test.go` line 113 (post-edit): `t.Skip(...)` line deleted (`git diff HEAD~1 -- internal/cli/codex_integration_test.go` shows `-` on that line, no replacement). Line 146 invokes `runValvBinaryCommand(t, …, "account", "bind", "codex", "profile-name")` — exercises the newly-wired bind command path end-to-end via the built `./valv` binary inside a real Docker container. `mage integration` → 205/205 PASS, 0 skipped, 0 failed (38.07s). | PASS |
| Build sanity | `mage build` GREEN | `mage build` → `[SUCCESS] Built valv (./valv)`. | PASS |

### Section 0 — Semi-Formal Reasoning (orchestrator-facing summary)

**Premises:**
- `domain.BindingRepository.DeleteBinding` is a NEW method (was not in tree pre-9.2).
- `sqlite.Store` must implement it (interface contract).
- `manage.Service.UnbindProject` is NEW; must thread project detection → store.DeleteBinding.
- AC #11 requires the previously-skipped integration test to (a) be unskipped and (b) actually exercise the new bind path.

**Evidence:** `git diff HEAD~1` deltas (10 files, +416/-2); `Read` over post-edit source at the cited line ranges; 5 mage gates reproduced GREEN locally.

**Trace:**
- Domain interface adds method → sqlite adapter implements it (delete SQL + rows-affected check + ErrNotFound wrap) → manage service wires detection→lookup→delete → CLI thin wrappers `newManageAccountBindCommand` (Option b — preferred per design notes) + `newManageAccountUnbindCommand` → registered in `newManageAccountCommand` → exercised by unit tests (3 packages) + end-to-end integration test against real Docker fixture image.
- Integration test at line 146 calls `valv account bind codex profile-name` against the built binary; the test reaches container-run (asserts stdin/stdout TTY + CODEX_HOME) which is unreachable without a successful prior bind. ⇒ AC #11 sub-clause "actually runs the command path" is proven by the test's downstream Docker assertions.

**Conclusion:** All 11 ACs have direct evidence. Coverage floors met. Mage gates green. The wrapping/error-handling pattern is idiomatic Go (`%w`, `errors.Is`). Provider default-to-Codex semantics consistent across bind and unbind. No dead code introduced.

**Unknowns:** none.

### Hylla Feedback

None — Hylla was not required for this proof review. All verification is structural (`git diff HEAD~1` + `Read` post-edit + `mage` reproduction) over committed code at HEAD. The newly-introduced symbols (`DeleteBinding`, `UnbindProject`, `newManageAccountBindCommand`, `newManageAccountUnbindCommand`) are not yet ingested; relying on Hylla for them would have been a miss by construction.

### Conclusion

R1 lands clean. All 11 ACs proven. All 5 mage gates green (testPkg cli 202/202@72.6%, testPkg sqlite 21/21@78.4%, testPkg manage 28/28@75.8%, integration 205/205, build SUCCESS). The drop's AC #11 — re-enabling the previously-skipped `TestCodexCommandRunsFixtureImageWithTTYEndToEnd` — is the load-bearing end-to-end proof: it exercises `valv account bind codex profile-name` against a real Docker fixture and downstream TTY/CODEX_HOME assertions confirm the bind path landed. Builder selected Option (b) thin-wrapper approach as the design notes preferred.

**Verdict: PASS**
