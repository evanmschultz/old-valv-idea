# DROP_8 — GLOBALSWITCH AND TUI PARITY (PLUS BINDING UX)

**State:** planning
**Blocked by:** DROP_7 (done)
**Paths (expected):** `internal/services/globalswitch/`, `internal/cli/manage.go` (or post-DROP_9 successor), `internal/cli/claude.go`, `internal/cli/codex.go`, `internal/tui/manage/picker.go` (and new picker for unbound-project bind selection), tests + golden fixtures. Planner refines.
**Packages (expected):** `internal/services/globalswitch`, `internal/cli`, `internal/tui/manage` (or new `internal/tui/bind/`). Planner confirms.
**PLAN.md ref:** main/PLAN.md → DROP_8_GLOBALSWITCH_AND_TUI_PARITY row
**Workflow:** main/drops/WORKFLOW.md
**Started:** 2026-05-18
**Closed:** —

## Scope

Lifted from `main/PLAN.md` DROP_8 row + the `project_valv_binding_ux_spec.md` memory:

**v0.1.0 requirement.** Extend `internal/services/globalswitch/service.go` to handle Claude (symlinks `~/.claude` to the active managed account home mirroring Codex), plus `valv account list` cross-provider, `valv account switch` cross-provider with `--provider` required on name collision, and full `tui/manage/picker.go` golden parity.

**Plus binding UX (added 2026-05-16):** when `valv claude` / `valv codex` runs in an unbound project — if exactly 1 account exists for the provider, auto-bind it; if 2+ exist, launch a Bubble Tea picker that binds the selection; if 0 exist, error pointing to `valv account add`. Add `--account <name>` flag to both commands for one-shot override that does NOT mutate the binding row. All behavior symmetric across providers.

Closes the remainder of focus-plan §6.6 + the dogfood binding-UX gap (dev hit "project is not bound" running `valv claude` in an unbound dir during DROP_7 R3 smoke test on 2026-05-17).

## Planner

<Filled by go-planning-agent in Phase 1. Atomic units of work below. Each unit's state is mutated in place by the builder during Phase 4. See main/drops/WORKFLOW.md § "Phase 1 — Plan" for deliverable rules.>

<!-- Planner: append `### Unit 8.N — <title>` blocks under this comment. Order them by likely build sequence; use `blocked_by` to encode dependencies. Each unit must list paths, packages, acceptance criteria, and state. -->

### Unit 8.1 — Globalswitch Claude extension

**State:** todo
**Paths:**
- `internal/services/globalswitch/service.go`
- `internal/services/globalswitch/service_test.go`

**Packages:** `github.com/evanmschultz/valv/internal/services/globalswitch`

**Acceptance:**
- `service.Switch(ctx, domain.ProviderClaude, profileName)` succeeds and creates a symlink at `~/.claude` (relative to the configured `HomeDir`) pointing to the selected profile's `HomePath`.
- `prepareTarget` returns `result.TargetPath = filepath.Join(s.homeDir, ".claude")` when `provider == domain.ProviderClaude`, and `".codex"` when `domain.ProviderCodex`. The existing hardcoded `filepath.Join(s.homeDir, ".codex")` at `service.go:106` is replaced by a provider-switch. A new test `TestSwitchClaudeTargetPath` asserts that calling `service.Switch(ctx, ProviderClaude, profileName)` lands a symlink at `~/.claude` (not `~/.codex`), AND the existing Codex test continues to pass.
- The backup path for a pre-existing real `~/.claude` directory goes under `global-switch/claude/backups/<timestamp>`, not the existing `codex` subdirectory. The hardcoded `"codex"` in `prepareTarget`'s `backupRoot` (line 137 of service.go) is replaced by `string(provider)` so each provider's backups land in their own subdir.
- A pre-existing `~/.claude` symlink is replaced without backup (mirrors the existing Codex behaviour).
- The host-process guard for Claude checks for a running `"claude"` process (not `"codex"`). Builder runs `pgrep -l claude` or equivalent on a host with the Claude CLI installed and documents the actual process name in BUILDER_WORKLOG.md. The host-process guard uses that verified string. If the name differs from `"claude"` (the planner's assumption), update the constant.
- `service.Switch(ctx, domain.ProviderCodex, profileName)` still passes all existing Codex tests unchanged.
- `TestSwitchRejectsUnsupportedProvider` is updated: it must no longer expect an error for Claude, and must verify that a truly unsupported provider (e.g. `domain.Provider("unknown")`) returns an error.
- `mage testPkg github.com/evanmschultz/valv/internal/services/globalswitch` passes with `-race`.

**Blocked by:** —

---

### Unit 8.2 — `valv account switch` cross-provider with `--provider` flag

**State:** todo
**Paths:**
- `internal/cli/manage.go`
- `internal/cli/manage_test.go`

**Packages:** `github.com/evanmschultz/valv/internal/cli`

**Acceptance:**
- `newManageAccountSwitchCommand` gains a `--provider <provider>` flag. When supplied it overrides provider resolution entirely (call `domain.ParseProvider(flagValue)`; error if invalid).
- Provider+account resolution follows this explicit order (replaces `resolveProfileSwitchTarget`):
  1. `--provider <p>` flag present → use `domain.ParseProvider(p)`, ignore positional arg for provider. Error if invalid.
  2. No flag, positional arg parses as a provider via `domain.ParseProvider` → treat it as the provider (0 or 1 remaining positional = account name or picker).
  3. No flag, positional arg is an account name (does not parse as provider) → call `service.ProfileByName` across `supportedProviders()`: if exactly one provider has the name, resolve to that provider; if multiple providers have the name, return a user-facing error listing all `(provider, account)` matches and instructing the user to use `--provider`; if no provider has the name, return a not-found error.
  4. No flag, no positional arg → picker opens cross-provider showing all accounts.
- Existing `valv account switch codex work` (two-arg form where `codex` parses as a provider) continues to work via step 1 or step 2.
- `valv account list` (no args) already shows cross-provider output via `writeAccountsByProvider`. Verify this in the test suite — no functional change needed if it is already correct; add a test assertion if the test coverage is absent.
- `mage testPkg github.com/evanmschultz/valv/internal/cli` passes with `-race`.

**Blocked by:** —

---

### Unit 8.3 — `--account` raw-arg interception on `valv claude` + `valv codex`

**State:** todo
**Paths:**
- `internal/cli/account_flag.go` (new file)
- `internal/cli/account_flag_test.go` (new file)
- `internal/cli/claude.go`
- `internal/cli/codex.go`

**Packages:** `github.com/evanmschultz/valv/internal/cli`

**Acceptance:**
- `stripAccountFlag(args []string) (accountName string, remaining []string)` is a new unexported function in `internal/cli/account_flag.go`. It scans `args` for `--account <value>` or `--account=<value>`, strips the first match, and returns the extracted name plus the remaining slice. If no `--account` is present, it returns `("", args)`.
- `runClaudeCommand` calls `stripAccountFlag` before the `claudeArgsSkipProjectBinding` check. The extracted `accountName` is threaded through to `runClaudeCommand`'s binding-ready call (introduced in Unit 8.4).
- `runCodexCommand` calls `stripAccountFlag` before `codexArgsSkipProjectBinding`. The extracted `accountName` is threaded through to `ensureCodexBindingReady` (updated in Unit 8.5).
- When `accountName` is non-empty, the binding-ready calls use that account directly (resolving it from the store) instead of the DB-persisted binding. They do NOT write a new binding row.
- `account_flag_test.go` covers: no flag present, `--account work`, `--account=work`, `--account` as the last token (malformed — returns empty), `--account` in the middle of other args, and the case where `--account` appears multiple times (first match wins).
- `--` escape hatch: cobra's `DisableFlagParsing: true` does NOT process `--`. `stripAccountFlag` treats `--` as a literal scan terminator — if `--` appears before `--account`, scanning stops and no account name is extracted. Test case: `["--", "--account", "work"]` → returns `("", ["--", "--account", "work"])`.
- Note: `valv claude` and `valv codex` both have `DisableFlagParsing: true` on their cobra commands. This flag MUST remain `true` so that Claude and Codex subcommand args pass through unmolested. The `stripAccountFlag` function operates on the raw `args []string` before any cobra parsing, which is the correct interception point.
- `mage testPkg github.com/evanmschultz/valv/internal/cli` passes with `-race`.

**Blocked by:** 8.2

---

### Unit 8.4 — Binding UX: `valv claude` auto-bind / picker / 0-account error

**State:** todo
**Paths:**
- `internal/cli/claude_setup.go` (new file)
- `internal/cli/claude_setup_test.go` (new file)
- `internal/cli/claude.go`
- `internal/services/manage/service.go` (add `StatusForProvider` method)
- `internal/services/claude/service.go` (add override-profile support)

**Packages:**
- `github.com/evanmschultz/valv/internal/cli`
- `github.com/evanmschultz/valv/internal/services/manage`
- `github.com/evanmschultz/valv/internal/services/claude`

**Acceptance:**
- `ensureClaudeBindingReady(cmd *cobra.Command, paths config.Paths, workingDir string, accountOverride string) (domain.Profile, error)` is a new function in `internal/cli/claude_setup.go`. It returns the resolved `domain.Profile` (override OR auto-bound OR existing binding's profile) so `runClaudeCommand` can thread it into the runtime.
- When `accountOverride != ""`: resolves the named Claude account via `service.ProfileByName(ctx, domain.ProviderClaude, accountOverride)`. Does NOT write a binding row. Returns the resolved profile. If not found, return a user-facing error.
- When `accountOverride == ""`:
  - Check Claude binding via `service.StatusForProvider(ctx, workingDir, domain.ProviderClaude)` — a new method added to `manage.Service` that calls `store.BindingByProjectID(ctx, projectRecord.ID, provider)` for the given provider (NOT hardcoded to Codex). This is the correct fix for the `manage/service.go:245` Codex-hardcoding. Do NOT use `service.Status()` here — it internally calls `store.BindingByProjectID(ctx, projectRecord.ID, domain.ProviderCodex)` and would always return `ErrUnboundProject` for Claude-bound projects.
  - If Claude-bound (StatusForProvider returns without error): return the resolved profile, nil — caller uses this profile directly, skipping `service.ValidateBinding`.
  - If not bound, call `service.ListProfiles(ctx, domain.ProviderClaude)`:
    - 0 accounts → return zero Profile and error: `"project is not bound; no Claude accounts found — run \`valv manage account add claude\` to create one"`.
    - 1 account → auto-bind silently: call `service.BindProject(ctx, domain.ProviderClaude, profile.Name, workingDir)`, emit a `laslig.NoticeInfoLevel` notice to `cmd.ErrOrStderr()` confirming the binding, return the bound profile.
    - 2+ accounts → if TTY available, launch picker via `pickProfile(cmd, domain.ProviderClaude, profiles)` then call `service.BindProject` and return the bound profile; if not TTY, return zero Profile and error asking user to run `valv manage bind claude <name>`.
- Override-profile threading into the runtime: when `ensureClaudeBindingReady` returns a non-zero profile, `runClaudeCommand` skips `service.ValidateBinding` and passes the resolved profile into `claudeservice`. Builder evaluates two approaches and documents the choice in BUILDER_WORKLOG.md: (a) add `OverrideProfile *domain.Profile` to `claudeservice.Options` and re-construct the service when non-nil; (b) add a profile parameter to `service.Run`. Either is acceptable; builder picks the cleaner one.
- Note: Claude auth is in-container. `ensureClaudeBindingReady` does NOT call any host-side auth check — no `ensureManagedAccountReady`.
- Tests in `claude_setup_test.go` cover: already-bound project (returns existing profile, no-op bind), 0 accounts (error), 1 account (auto-bind + notice + returns profile), 2+ accounts no-TTY (error), and `accountOverride` path (profile resolved, no bind written, correct profile returned). Tests assert the returned profile is correct in each branch.
- `mage testPkg github.com/evanmschultz/valv/internal/cli` passes with `-race`.
- `mage testPkg github.com/evanmschultz/valv/internal/services/manage` passes with `-race`.
- `mage testPkg github.com/evanmschultz/valv/internal/services/claude` passes with `-race`.

**Blocked by:** 8.3

---

### Unit 8.5 — Binding UX parity: `valv codex` 0/1/2+ account handling

**State:** todo
**Paths:**
- `internal/cli/codex_setup.go`
- `internal/cli/codex_setup_test.go`
- `internal/cli/codex.go`

**Packages:** `github.com/evanmschultz/valv/internal/cli`

**Acceptance:**
- `ensureCodexBindingReady(cmd *cobra.Command, paths config.Paths, workingDir string, accountOverride string) (domain.Profile, error)` — signature updated to match Unit 8.4's Claude pattern. Returns the resolved `domain.Profile` to the caller.
- When `accountOverride != ""`: resolves the named Codex account via `service.ProfileByName(ctx, domain.ProviderCodex, accountOverride)`. Does NOT write a binding row. Returns the resolved profile.
- When `accountOverride == ""`:
  - Check if already Codex-bound via `service.Status(ctx, workingDir)` (this call is correct for Codex because `manage/service.go:245` hardcodes `ProviderCodex`). If bound, return the existing profile.
  - If not bound, call `service.ListProfiles(ctx, domain.ProviderCodex)`:
    - 0 accounts → return zero Profile and error: `"project is not bound; no Codex accounts found — run \`valv manage account add codex\` to create one"`. This replaces the 4-option menu for the 0-account case.
    - 1 account → auto-bind silently (call `service.BindProject(ctx, domain.ProviderCodex, profile.Name, workingDir)`), emit a `laslig.NoticeInfoLevel` notice, return the bound profile. The 4-option interactive menu (`runCodexFirstRunSetup`) is REMOVED for the 1-account case. Users wanting to create a new account or choose differently run `valv manage account add codex <name>` directly.
    - 2+ accounts → launch the existing `pickProfile(cmd, domain.ProviderCodex, profiles)` picker, then call `service.BindProject` and return the bound profile. The 4-option menu is REMOVED; the TUI profile picker replaces it for the multi-account case. If not TTY, return zero Profile and error asking user to run `valv manage bind codex <name>`.
- `runCodexCommand` is updated: call `ensureCodexBindingReady` with the `accountOverride` from Unit 8.3. When a non-zero profile is returned, skip `service.ValidateBinding` and use the resolved profile (matching Unit 8.4's override-threading approach for symmetry).
- Existing tests in `codex_setup_test.go` that verify the multi-account picker flow must still pass. Tests for the 0-account and 1-account shortcuts are added. Test for `accountOverride` path (profile resolved, no bind written) is added.
- `mage testPkg github.com/evanmschultz/valv/internal/cli` passes with `-race`.

**Blocked by:** 8.4

---

### Unit 8.6 — `tui/manage/picker.go` Claude golden parity

**State:** todo
**Paths:**
- `internal/tui/manage/golden_test.go`
- `internal/tui/manage/testdata/TestProfilePickerGoldenClaude.golden` (new)

**Packages:** `github.com/evanmschultz/valv/internal/tui/manage`

**Acceptance:**
- `TestProfilePickerGoldenClaude` is added to `golden_test.go`. It calls `NewProfilePicker(domain.ProviderClaude, []domain.Profile{{Name: "alpha-profile", HomePath: "/tmp/alpha-profile"}, {Name: "beta-profile", HomePath: "/tmp/beta-profile"}})`, runs the model through `teatest`, and calls `teatest.RequireEqualOutput` on `final.View().Content`.
- The new golden file `testdata/TestProfilePickerGoldenClaude.golden` is generated via `mage goldenUpdate` and committed.
- `TestManageHomeGolden` and `TestProfilePickerGolden` (existing Codex variant) continue to pass unchanged.
- Golden-diff bounding: the Claude golden file format mirrors the Codex golden file format exactly, modulo provider-specific identity fields (e.g., account names, provider label). The test fails if the format diverges beyond those identity-field substitutions.
- `mage golden` passes (all three golden tests green).
- `mage testPkg github.com/evanmschultz/valv/internal/tui/manage` passes with `-race`.

**Blocked by:** —

---

### Unit 8.7 — Wire globalswitch dispatch for Claude

**State:** todo
**Paths:**
- `internal/cli/operator_helpers.go`
- `internal/cli/operator_helpers_test.go` (or existing test file for this path)

**Packages:** `github.com/evanmschultz/valv/internal/cli`

**Acceptance:**
- The `ActionGlobalSwitch` case in `runManageHome` (`operator_helpers.go:151`) is changed from `runGlobalSwitch(cmd, paths, opts, domain.ProviderCodex, "")` to a provider-aware dispatch: detect the current project's bound provider via `openManageService` + `service.Status(ctx, workingDir)`. If bound, use `status.Binding.Provider` as the provider for `runGlobalSwitch`. If `service.Status` returns `ErrUnboundProject` or any error, fall back to `domain.ProviderCodex` for backwards compatibility.
- `valv global switch claude <account>` (the CLI path, already correct in `global.go`) is NOT modified — it already parses the provider from args correctly. This unit is narrowly scoped to the TUI dispatch in `runManageHome`.
- `valv global switch codex <account>` continues to work unchanged end-to-end.
- A test asserts the provider-aware dispatch logic: bound-to-claude project → `runGlobalSwitch` receives `ProviderClaude`; unbound project → `runGlobalSwitch` receives `ProviderCodex` fallback.
- `mage testPkg github.com/evanmschultz/valv/internal/cli` passes with `-race`.

**Blocked by:** 8.1, 8.5

---

## Notes

- **Symmetry across providers is load-bearing.** Anything new added for Claude must have a Codex counterpart (and vice versa) unless explicitly justified. Globalswitch currently handles Codex via symlink to managed home; planner extends to Claude. `valv account list` and `valv account switch` need cross-provider semantics.
- **Pre-DROP_9 CLI surface assumption.** DROP_9 will normalize the command tree (delete `valv manage` namespace, add `valv account bind`/`unbind`, add `valv image` namespace, flatten `valv status`). DROP_8 lives BEFORE DROP_9, so it operates on the CURRENT command tree (`valv manage *` + `valv account *` registered at two paths via `newManageAccountCommand`). DROP_8 should NOT rename commands — DROP_9 owns that. DROP_8 adds NEW behavior to existing paths.
- **`--account <name>` flag is DROP_8 territory.** DROP_9 explicitly consumes it (per DROP_9 scope: "do NOT re-implement"). Planner ensures the flag is implemented cleanly so DROP_9 can route it through any rename.
- **Bubble Tea picker.** Existing `internal/tui/manage/picker.go` is the prior art. New picker for unbound-project bind selection may reuse, refactor, or be a sibling — planner's choice.
- **No re-introduction of auto-open.** Per `feedback_manual_workflow_is_the_decision.md`, the Claude OAuth flow stays manual (press `c` for clean copy + Ctrl-C × 2 to exit). DROP_8 does not touch that surface.
- **DROP_9 fold-in concerns.** Two non-blocking concerns from DROP_7 R3 close land in DROP_9, not DROP_8: (A) static `ContainerRunRequest` field assertions for auth tests; (B) harden `RunInContainer` against future non-zero exit on Ctrl-C × 2. DROP_8 stays focused on globalswitch + TUI parity + binding UX.
- **Codex 1-account menu removal (R2 decision).** Dev decided: the existing 4-option Codex menu (`runCodexFirstRunSetup`) is REMOVED for both 0-account and 1-account cases and replaced with the auto-bind shortcut (matching Claude UX). The 4-option menu remains only as the 2+ accounts fallback, where it is replaced by the TUI `pickProfile` picker. Users wanting to create a new isolated account run `valv manage account add codex <name>` directly. Builder must not keep the 4-option menu for 0/1 account cases.
- **Shared 0-account error helper.** Units 8.4 and 8.5 use structurally identical "no accounts found" errors. Add a single helper function `unboundProjectNoAccountsError(provider domain.Provider) error` in `internal/cli/claude_setup.go` (or a shared `internal/cli/bind_errors.go`) so DROP_9's command-tree rename has one touchpoint to update error copy.
- **Test injection pattern.** Units 8.4 and 8.5 need injectable seams for testing the auto-bind and picker paths without live Docker or real SQLite. Use package-level `var` overrides (matching the `hostClaudeAccountAuth` pattern from DROP_7) for `openManageService`, `pickProfile`, and `ensureManagedAccountReady`. This keeps the injection pattern consistent with existing CLI tests.
- **`account_flag.go` file isolation (DROP_9 touchpoint).** Unit 8.3's `stripAccountFlag` lives in `internal/cli/account_flag.go` (its own file) so DROP_9's command-tree rename has a single touchpoint. Do not inline the function into `claude.go` or `codex.go`.
- **`BindProject` upsert idempotency.** Concurrent `valv claude` or `valv codex` invocations in the same project may both attempt `BindProject` on first run. `service.BindProject` uses `store.UpsertProjectBinding` (confirmed in `manage/service.go:223`) which is already idempotent. Builder verifies this covers the concurrent-call case and adds a note in BUILDER_WORKLOG.md.
- **`StatusForProvider` method (Unit 8.4).** The new `manage.Service.StatusForProvider(ctx, startPath, provider)` method mirrors `Status` exactly but passes `provider` to `store.BindingByProjectID` instead of hardcoding `domain.ProviderCodex`. This is a minimal surgical extension — do not refactor `Status` itself, as DROP_9 may restructure the status surface.
- **Unit 8.7 `blocked_by` rationale.** Unit 8.7 touches `internal/cli` (same package as 8.3, 8.4, 8.5) and depends on `globalswitch.Service.Switch` accepting Claude (Unit 8.1). It is blocked by both 8.1 (Claude branch must exist) and 8.5 (last unit that writes to `internal/cli`). Running 8.7 concurrently with any `internal/cli` unit would break package compilation isolation.
