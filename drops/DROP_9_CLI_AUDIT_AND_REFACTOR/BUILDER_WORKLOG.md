# DROP_9 — Builder Worklog

## Unit 9.1 — Round 1

- **Builder:** go-builder-agent
- **Started:** 2026-05-18
- **Files touched:**
  - `internal/cli/manage.go`
  - `internal/cli/root.go`
  - `internal/cli/manage_test.go`
  - `internal/cli/root_test.go`
  - `internal/cli/extended_test.go`
  - `drops/DROP_9_CLI_AUDIT_AND_REFACTOR/PLAN.md`
- **Mage targets run:** `mage testPkg github.com/evanmschultz/valv/internal/cli` (PASS, 200 tests, 72.9% coverage), `mage build` (PASS)

### Design notes

**`newManageCommand` deletion (manage.go):** Removed the entire `newManageCommand` function and its `RunE` closure (which called `runManageHome`). All 35 lines of the constructor plus its `AddCommand` chain were deleted. The function was the only thing binding the old `manage` cobra command to the root tree.

**`runManageHome` location:** `runManageHome` lives in `operator_helpers.go`, not `manage.go`. `operator_helpers.go` is outside 9.1's declared paths. Left it as dead code per cascade discipline — 9.4.5 or a later unit sweeps it. Tests that called `runManageHome` were updated to use a bare stub `&cobra.Command{}` instead of `newManageCommand`.

**root.go group rename:** Group `"manage"` renamed to `"account"` throughout: `cmd.AddGroup(...)`, `accountCmd.GroupID`, `globalCmd.GroupID`. `manageCmd` variable and its `AddCommand` call removed.

**root.go Example update:** Removed `valv manage update`, `valv manage account add codex`, `valv manage account add codex work` lines. Replaced with `valv account add codex`, `valv account add codex work`.

**`newTestManageContainerCommand` (manage_test.go):** Created a test-only container command that re-registers all ex-manage children (account, bind, project, status, update, cleanup) so existing test routing via `runManage(t, paths, []string{"account", "add", ...})` continues to work unchanged.

**Tests deleted:**
- `TestManageCommandWithoutTTYShowsHelp` (extended_test.go) — exercised the deleted `newManageCommand`
- `TestManageAliasWorks` (root_test.go) — exercised the `"m"` alias on the deleted manage command
- `TestManageHelpSubcommandWorks` (root_test.go) — exercised `manage help` on the deleted manage command

**Tests updated:**
- `TestHelpAliasDisplaysRootHelp` (root_test.go): removed `"manage"` from expected substrings, added `"account"`.
- `TestManageAccountSwitchMissingAccountShowsActionableGuidance` (extended_test.go): updated error assertion from `"run \`valv manage account add codex work\`"` to `"run \`valv account add codex work\`"` — matches updated manage.go line 626 fmt.Errorf string.
- `TestRunManageUpdateClaudeBuildsImage`, `TestRunManageUpdateCodexRegression` (manage_test.go): switched from `newManageCommand` + args `["update", ...]` to `newManageUpdateCommand` + args `["claude"]` / `["codex"]` / `[]`.
- `TestManageAccountListJSONUsesCommandKey`, `TestManageAccountLoginUsesExistingAccount`, `TestManageAccountLogoutUsesExistingAccount`, `TestManageAccountDeleteRejectsBoundAccount` (extended_test.go): switched from `newManageCommand` to appropriate specific constructors with stripped args.
- `TestManageCleanupAllRemovesLocalStateAndInvokesDocker`, `TestManageCleanupImagesRemovesProviderImagesOnly` (extended_test.go): switched to `newManageCleanupCommand`.
- `TestRunManageHomeWithoutTTYShowsHelp` (extended_test.go): replaced `newManageCommand` with bare `&cobra.Command{}` stub; `runManageHome` call preserved.
- `TestManageAccountAddCreatesIsolatedNamedAccountAndBindsProject`, `TestManageAccountHelpSubcommandWorks`, `TestManageProfileAliasNoLongerWorks`, `TestManageAccountAddClaudeWithSkipLoginSkipsImageEnsure`, `TestManageAccountAddClaudeWithExistingCredsAndImageOverrideSucceeds` (manage_test.go): switched from `newManageCommand` to `newManageAccountCommand` with `"account"` prefix stripped from args.

### 3 fmt.Errorf strings (post-edit text — AC #6 verification)

**manage.go line ~590 (formerly 626):**
```go
return fmt.Errorf("manage account switch: account %q not found for provider %q; run `valv account add %s %s` or `valv account list %s`", profileName, provider, provider, profileName, provider)
```

**manage.go line ~924 (formerly 962):**
```go
return "", "", fmt.Errorf("account %q not found in any provider; run `valv account add codex %s` or `valv account list` to see all available accounts", accountName, accountName)
```

**manage.go line ~958 (formerly 996):**
```go
return "", "", fmt.Errorf("no accounts found across any provider; run `valv account add codex <name>` or `valv account add claude <name>` to create one")
```

### `internal/tui/manage` dead-code note

`internal/tui/manage` (the Bubble Tea home screen model) is now unreachable from the CLI — `runManageHome` in `operator_helpers.go` is the only caller and that function is now dead code. Both are left in tree per cascade discipline. DROP_11 or a dedicated cleanup drop handles removal.

### `TestRunManageBindInteractiveShowsGuidanceWhenNoAccountsExist` note

The error assertion `"run \`valv manage account add codex\`"` in extended_test.go line ~413 originates from `realPickProfile` in `operator_helpers.go` (outside 9.1's paths). Left for 9.4.5 to sweep alongside the operator_helpers.go string update.

## Hylla Feedback

Hylla indexes Go files and is the first evidence source. Several queries returned zero results (Hylla snapshot 2 is from DROP_8 close, so the `runManageHome` symbol in operator_helpers.go would be in the index). The empty Hylla results may reflect snapshot age or the `test_mode: hide_tests` filter hiding test-only symbols. All needed evidence was obtained via direct `Read` of source files, which is appropriate for this unit's straightforward structural deletion work.

- **Query:** `hylla_search_keyword` with `"runManageHome"`, node_type=block. **Missed because:** zero results returned — possibly stale snapshot or indexing gap. **Worked via:** `Read` on `operator_helpers.go`. **Suggestion:** Include function bodies in keyword index even when summary is empty.
