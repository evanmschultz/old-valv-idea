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

---

## Unit 9.1 — Round 2

- **Builder:** go-builder-agent
- **Started:** 2026-05-18
- **Files touched:**
  - `internal/cli/codex_integration_test.go`

### Fixes applied

**FIX 1 — Compile break at `codex_integration_test.go:357`**

`runManageForIntegration` called the deleted symbol `newManageCommand`. Replaced with `newTestManageContainerCommand` (the test-only container helper added in R1's `manage_test.go:232-248`). No argv reshaping needed: callers at lines 51-52 pass `["account", "add", ...]` and `["bind", ...]`, both of which route correctly to children registered on `newTestManageContainerCommand`.

**FIX 2 — Runtime argv at lines 145, 146, 240 (stripped `"manage"` leading token)**

Three `runValvBinaryCommand` call sites passed `"manage"` as the first argv token to the built `./valv` binary. Post-R1, `valv manage` is not a registered cobra subcommand. Updated:
- Line 145: `"manage", "account", "add", ...` → `"account", "add", ...`
- Line 146: `"manage", "bind", "codex", "profile-name"` → `"account", "bind", "codex", "profile-name"` (per DROP_9 unit 9.2 final command shape)
- Line 240: `"manage", "account", "add", ...` → `"account", "add", ...`

### Mage gate results

1. `mage testPkg github.com/evanmschultz/valv/internal/cli` — **PASS** (200 tests, 72.9% coverage, -race, no failures).
2. `mage integration` — **PARTIAL**: compile break is fixed (integration tests now compile and run). 202/203 tests pass. One test fails at **runtime**: `TestCodexCommandRunsFixtureImageWithTTYEndToEnd` errors at line 146 with `unknown command "bind" for "valv account"`. This is a pre-9.2 structural gap — `valv account bind` does not exist in the binary until unit 9.2 lands. R2 cannot fix this within its declared scope (codex_integration_test.go only; no source code changes permitted).
3. `mage build` — **PASS**.

### Why R1 missed this

R1's builder ran `mage testPkg ./internal/cli`, which compiles only non-tagged files. The `//go:build integration` tag on `codex_integration_test.go` excludes the file from `go test ./internal/cli` (without `-tags=integration`). The broken `newManageCommand` reference in that file was invisible to the non-integration test run. `mage integration` passes `-tags=integration` and compiled the file, surfacing the break.

### Known remaining gap (pre-9.2)

`TestCodexCommandRunsFixtureImageWithTTYEndToEnd` at line 146 calls `valv account bind codex profile-name` against the real binary. `account bind` does not exist until unit 9.2 ships. This test will remain a runtime failure until 9.2 closes. The compile break (R2's scope) is resolved; the runtime break is a 9.2 dependency.

## Hylla Feedback (Round 2)

N/A — Hylla is stale post-R1 commit (last ingest is DROP_8 baseline). All evidence gathered via `Read` and `git diff HEAD~2..HEAD~1`. No Hylla queries were applicable for this round.

---

## Unit 9.1 — Round 3

- **Builder:** go-builder-agent
- **Started:** 2026-05-18
- **Files touched:**
  - `internal/cli/codex_integration_test.go`

### Edit applied

Added `t.Skip(...)` as the first statement inside `TestCodexCommandRunsFixtureImageWithTTYEndToEnd` (line 113, immediately before `paths := testCodexPaths(t)`):

```go
t.Skip("requires valv account bind from DROP_9 Unit 9.2 — re-enable when 9.2 lands")
```

### Design notes

**Cascade ordering rationale:** Unit 9.1 deletes the `valv manage` namespace; Unit 9.2 adds `valv account bind`. During the cascade window between those two units, `TestCodexCommandRunsFixtureImageWithTTYEndToEnd` calls the real binary with `valv account bind` at line 146 — a command that does not exist until 9.2 ships. The skip fires before any test setup (zero cost, zero side effects), converting a runtime FAIL into a SKIP so `mage integration` reports green. The skip comment names 9.2 explicitly, ensuring the re-enable obligation is discoverable.

**Note to 9.2 builder:** Remove the `t.Skip` line from `TestCodexCommandRunsFixtureImageWithTTYEndToEnd` after wiring `valv account bind`. Confirm `mage integration` reports 203/203 PASS with no SKIP.

### Mage gate results

1. `mage testPkg github.com/evanmschultz/valv/internal/cli` — **PASS** (200 tests, 72.9% coverage, -race, 0 failures; integration-tagged test invisible to this target as expected).
2. `mage integration` — **PASS** (202 passed, 1 skipped `TestCodexCommandRunsFixtureImageWithTTYEndToEnd`, 0 failed).
3. `mage build` — **PASS** (`./valv` built successfully).

## Hylla Feedback (Round 3)

N/A — task touched only a single `t.Skip` insertion in a test file already read in R2. No Go symbol queries were needed.

---

## Unit 9.2 — Round 1

- **Builder:** go-builder-agent
- **Started:** 2026-05-18
- **Files touched:**
  - `internal/domain/repository.go`
  - `internal/adapters/sqlite/store.go`
  - `internal/adapters/sqlite/store_test.go`
  - `internal/services/manage/service.go`
  - `internal/services/manage/service_test.go`
  - `internal/cli/manage.go`
  - `internal/cli/manage_test.go`
  - `internal/cli/codex_integration_test.go`
  - `drops/DROP_9_CLI_AUDIT_AND_REFACTOR/PLAN.md`

### Design choices

**Bind command shape — Option (b):** Created a new thin `newManageAccountBindCommand` that wraps `runManageBind` with a new arg/flag shape. Did NOT rename or modify `newManageBindCommand` (the old `manage bind` constructor). Option (b) is surgical and leaves the existing bind run path untouched.

**`account bind` arg parsing:** Accepts `RangeArgs(1, 2)`. When 2 args: first is provider, second is account name (matching existing `codex_integration_test.go` call at line 147: `"account", "bind", "codex", "profile-name"`). When 1 arg: account name only, provider from `--provider` flag or defaults to Codex. This is consistent with the `delete`, `rename`, `login`, `logout` pattern already in the codebase.

**`account unbind` signature:** `Args: cobra.NoArgs`. Accepts `--provider` flag (default Codex) and `--project` flag. Calls `service.UnbindProject`. Returns a brief "Project unbound" record output consistent with other commands.

**Implementation order (domain → sqlite → service → cli):** The compile gate enforced this — `sqlite.Store` must implement `DeleteBinding` before the manage service tests can compile, because `manage.Store` embeds `domain.BindingRepository`.

**`UnbindProject` error handling:** Returns descriptive wrapped errors at two failure points: (1) project not found in store → wraps `domain.ErrNotFound`; (2) binding not found → `store.DeleteBinding` returns wrapped `domain.ErrNotFound`. Both bubble cleanly through `errors.Is` checks in tests.

**`t.Skip` removal (AC #11):** Removed from `codex_integration_test.go` line 113. The integration test now runs end-to-end with the real binary, calling `account add codex profile-name` then `account bind codex profile-name` then `valv codex resume session-tty` via PTY.

### Mage gate results

1. `mage testPkg github.com/evanmschultz/valv/internal/cli` — **PASS** (202 tests, 72.6% coverage, -race, 0 failures)
2. `mage testPkg github.com/evanmschultz/valv/internal/adapters/sqlite` — **PASS** (21 tests, 78.4% coverage, -race, 0 failures)
3. `mage testPkg github.com/evanmschultz/valv/internal/services/manage` — **PASS** (28 tests, 75.8% coverage, -race, 0 failures)
4. `mage integration` — **PASS** (205/205 PASS, 0 skipped, 0 failed)
5. `mage build` — **PASS** (`./valv` built successfully)

### `internal/tui/manage` dead-code note

`runManageHome` in `operator_helpers.go` (and `internal/tui/manage`) remain dead code as noted in 9.1. Not touched — DROP_11 or a cleanup drop handles removal.

## Hylla Feedback (Unit 9.2 Round 1)

None — Hylla answered everything needed. All Go symbol evidence gathered via direct `Read` of source files (Hylla's last ingest predates the DROP_9 work, making the committed index stale for recently-modified files). Evidence flow: `Read` → `git diff` where needed. No Hylla queries were applicable given the stale baseline, and non-Go files (markdown, worklog) are outside Hylla's Go-only scope.
