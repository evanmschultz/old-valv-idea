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
- The backup path for a pre-existing real `~/.claude` directory goes under `global-switch/claude/backups/<timestamp>`, not the existing `codex` subdirectory.
- A pre-existing `~/.claude` symlink is replaced without backup (mirrors the existing Codex behaviour).
- The host-process guard for Claude checks for a running `"claude"` process (not `"codex"`).
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
- `newManageAccountSwitchCommand` gains a `--provider <provider>` flag. When supplied, it overrides provider resolution entirely.
- `resolveProfileSwitchTarget` (or its replacement): when 1 arg (an account name, not a provider) is given and no `--provider` flag, and the name exists in exactly one provider, resolve to that provider. When the name exists in multiple providers, return a user-facing error that includes both provider names and instructs the user to use `--provider`.
- When 0 args and no `--provider`, the picker opens showing all providers' accounts (or the provider defaults to the current binding's provider when the project is bound).
- Existing `valv account switch codex work` (two-arg form) continues to work as before.
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

**Packages:** `github.com/evanmschultz/valv/internal/cli`

**Acceptance:**
- `ensureClaudeBindingReady(cmd *cobra.Command, paths config.Paths, workingDir string, accountOverride string) error` is a new function in `internal/cli/claude_setup.go`.
- When `accountOverride != ""`: the function resolves the named Claude account from the store (`service.ProfileByName`). It does NOT write a binding row. It returns the resolved profile to the caller so `runClaudeCommand` can use it as the launch account. If the account is not found, return a user-facing error.
- When `accountOverride == ""`:
  - If the project is already Claude-bound (`service` resolves a Claude binding for the project), return nil (proceed normally through the existing `service.ValidateBinding` path).
  - If not bound, call `service.ListProfiles(ctx, domain.ProviderClaude)`:
    - 0 accounts → return a user-facing error: `"project is not bound; no Claude accounts found — run \`valv manage account add claude\` to create one"`.
    - 1 account → auto-bind silently: call `service.BindProject(ctx, domain.ProviderClaude, profile.Name, workingDir)`, emit a `laslig.NoticeInfoLevel` notice to `cmd.ErrOrStderr()` confirming the binding, return nil.
    - 2+ accounts → if TTY available, launch picker via `pickProfile(cmd, domain.ProviderClaude, profiles)` then bind; if not TTY, return error asking user to run `valv manage bind claude <name>`.
- `runClaudeCommand` calls `ensureClaudeBindingReady` immediately after resolving `workingDir`, before `service.ValidateBinding`. When `accountOverride` is non-empty, skip `service.ValidateBinding` (the override profile is used directly).
- Note: Claude auth is in-container (device-code runs inside Docker). `ensureClaudeBindingReady` does NOT call any host-side auth check — no `ensureManagedAccountReady` call here.
- Note: `manage/service.Status()` is hardcoded to `ProviderCodex` for binding lookup. `ensureClaudeBindingReady` must NOT use `service.Status()` for Claude. It checks Claude binding directly via `service.ListProfiles` + store binding lookup. Use `service.ListProfiles(ctx, domain.ProviderClaude)` to get accounts; check binding via `store.BindingByProjectID` if needed, or simply attempt `service.Status` path and fall through on `ErrUnboundProject` to the 0/1/2+ branching.
- Tests in `claude_setup_test.go` cover: already-bound project (no-op), 0 accounts (error), 1 account (auto-bind + notice), 2+ accounts no-TTY (error), and `accountOverride` path (profile resolved, no bind written).
- `mage testPkg github.com/evanmschultz/valv/internal/cli` passes with `-race`.

**Blocked by:** 8.3

---

### Unit 8.5 — Binding UX parity: `valv codex` 0/1/2+ account handling

**State:** todo
**Paths:**
- `internal/cli/codex_setup.go`
- `internal/cli/codex_setup_test.go`

**Packages:** `github.com/evanmschultz/valv/internal/cli`

**Acceptance:**
- `ensureCodexBindingReady` is updated to implement the spec-mandated 0/1/2+ logic:
  - 0 Codex accounts and no TTY → return a user-facing error: `"project is not bound; no Codex accounts found — run \`valv manage account add codex\` to create one"`.
  - 1 Codex account → auto-bind the single account silently (call `service.CreateDefaultHostProfile` or `service.ProfileByName` + `service.BindProject`), emit a `laslig.NoticeInfoLevel` notice, return nil. This avoids the full interactive menu for the single-account case.
  - 2+ Codex accounts → existing interactive menu flow (`runCodexFirstRunSetup`) continues unchanged.
- The `accountOverride` from Unit 8.3 is accepted as a parameter to `ensureCodexBindingReady`. When non-empty, it resolves the named Codex account directly (no binding write, same semantics as Unit 8.4 Claude).
- Existing tests in `codex_setup_test.go` that verify the multi-account interactive flow must still pass. Tests for the 0-account and 1-account shortcuts are added.
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
- `mage golden` passes (all three golden tests green).
- `mage testPkg github.com/evanmschultz/valv/internal/tui/manage` passes with `-race`.

**Blocked by:** —

## Notes

- **Symmetry across providers is load-bearing.** Anything new added for Claude must have a Codex counterpart (and vice versa) unless explicitly justified. Globalswitch currently handles Codex via symlink to managed home; planner extends to Claude. `valv account list` and `valv account switch` need cross-provider semantics.
- **Pre-DROP_9 CLI surface assumption.** DROP_9 will normalize the command tree (delete `valv manage` namespace, add `valv account bind`/`unbind`, add `valv image` namespace, flatten `valv status`). DROP_8 lives BEFORE DROP_9, so it operates on the CURRENT command tree (`valv manage *` + `valv account *` registered at two paths via `newManageAccountCommand`). DROP_8 should NOT rename commands — DROP_9 owns that. DROP_8 adds NEW behavior to existing paths.
- **`--account <name>` flag is DROP_8 territory.** DROP_9 explicitly consumes it (per DROP_9 scope: "do NOT re-implement"). Planner ensures the flag is implemented cleanly so DROP_9 can route it through any rename.
- **Bubble Tea picker.** Existing `internal/tui/manage/picker.go` is the prior art. New picker for unbound-project bind selection may reuse, refactor, or be a sibling — planner's choice.
- **No re-introduction of auto-open.** Per `feedback_manual_workflow_is_the_decision.md`, the Claude OAuth flow stays manual (press `c` for clean copy + Ctrl-C × 2 to exit). DROP_8 does not touch that surface.
- **DROP_9 fold-in concerns.** Two non-blocking concerns from DROP_7 R3 close land in DROP_9, not DROP_8: (A) static `ContainerRunRequest` field assertions for auth tests; (B) harden `RunInContainer` against future non-zero exit on Ctrl-C × 2. DROP_8 stays focused on globalswitch + TUI parity + binding UX.
