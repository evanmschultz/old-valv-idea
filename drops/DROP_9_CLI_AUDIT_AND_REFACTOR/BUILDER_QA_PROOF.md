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
