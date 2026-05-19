# DROP_9 — CLI_AUDIT_AND_REFACTOR

**State:** planning
**Blocked by:** DROP_8 (done)
**Paths (expected):**
- `internal/cli/root.go`
- `internal/cli/manage.go`
- `internal/cli/claude_auth.go`
- `internal/cli/claude_auth_test.go`
- `internal/cli/manage_test.go`
- `internal/cli/root_test.go`
- `internal/cli/extended_test.go`
**Packages (expected):** `github.com/evanmschultz/valv/internal/cli`
**PLAN.md ref:** main/PLAN.md → DROP_9 row
**Workflow:** main/drops/WORKFLOW.md
**Started:** 2026-05-18
**Closed:** —

## Scope

Normalize the Valv CLI surface to a clean tree: delete the `valv manage` namespace entirely and redistribute its children (`bind` → `account bind`, `update` → `image update`, `cleanup` → `image cleanup`, `status` → top-level `valv status`, `project list` content folded into `valv status`); add missing `valv account bind` + `valv account unbind` commands to fill the project-binding gap; add new `valv image` namespace with `update`, `cleanup` (flag-driven), and `inspect` subcommands; normalize the `--provider` flag with positional fallback across account subcommands; drop the `account inspect` `whoami` alias; require `--provider` flag on name collisions across providers; fix `manage_test.go` / `root_test.go` / `extended_test.go` test routing after namespace deletion; harden `ensureClaudeAccountReady` / `loginClaudeAccount` against future non-zero container exit when creds are present (CONCERN B); add static `ContainerRunRequest` field assertion tests (CONCERN A). Pre-v0.1.0 timing: breaking changes are free.

## Planner

### Unit 9.1 — Delete `valv manage` namespace and redistribute children

- **State:** todo
- **Paths:**
  - `internal/cli/root.go`
  - `internal/cli/manage.go`
  - `internal/cli/manage_test.go`
  - `internal/cli/root_test.go`
  - `internal/cli/extended_test.go`
- **Packages:** `github.com/evanmschultz/valv/internal/cli`
- **Acceptance:**
  1. `newManageCommand` function and its `RunE` (the `runManageHome` TUI call) are deleted from `manage.go`.
  2. `root.go` no longer registers `newManageCommand`; the `manage` group and the `manageCmd` local variable are removed.
  3. `root.go` still registers `accountCmd` pointing to `newManageAccountCommand` (unchanged — account namespace is already a top-level command; the duplicate registration under `manage` is the thing being removed).
  4. `root.go` examples block is updated to remove all `valv manage ...` examples.
  5. `manage_test.go` tests that previously routed via `newManageCommand(...)` `.Execute()` with `account ...` subargs are rewritten to use `newManageAccountCommand` directly.
  6. `mage testPkg ./internal/cli` passes (no compilation errors, all tests green).
- **Blocked by:** —

**Design notes for builder:**

The `account` namespace is already top-level (registered at `root.go:129` via `newManageAccountCommand`). The `manage` namespace registers the same function again at `manage.go:49`. Deletion means:
- Remove `newManageCommand` from `manage.go` (lines 22-57 approximately — the constructor and its `RunE` body that calls `runManageHome`).
- Remove `runManageHome` from `manage.go` (the TUI dispatch function).
- Remove the `manageCmd` local var + its `AddCommand` call from `root.go` `newRootCommandWithPaths`.
- Remove the `"manage"` group registration from `root.go` `cmd.AddGroup(...)` call (the `GroupID: "manage"` group and the `manageCmd.GroupID = "manage"` line). Keep `accountCmd.GroupID = "manage"` or rename the group to `"account"` — the GroupID label is a cosmetic concern; builder should rename to `"account"` since `manage` is gone.
- Keep ALL children of `newManageCommand` (the constructors `newManageAccountCommand`, `newManageBindCommand`, `newManageProjectCommand`, `newManageStatusCommand`, `newManageUpdateCommand`, `newManageCleanupCommand`) in `manage.go` — they will be wired to the new `account` and `image` top-level commands in units 9.2–9.4.
- `runManageHome` (the TUI dispatch for the manage home screen) is the only function that goes away permanently. The `internal/tui/manage` package itself is independent and survives — it may be used by `valv image` or removed in a later cleanup drop.
- Test fixes: tests that call `newManageCommand(paths, &rootOptions{})` and then `.SetArgs([]string{"account", ...})` must be rewritten to call `newManageAccountCommand(paths, &rootOptions{})` and set args to the subcommand name directly (e.g., `[]string{"add", "codex", "profile-name", "--project", workDir}`). Same pattern for bind, status, update, cleanup tests.

**Risk note:** This unit touches three large test files (`manage_test.go` 26K, `extended_test.go` 31.8K, `root_test.go` 9.4K). The acceptance criterion `mage testPkg ./internal/cli` catches all breakage. Builder must scan all three for `newManageCommand` and `valv manage` routing before declaring done.

---

### Unit 9.2 — Add `valv account bind` and `valv account unbind`

- **State:** todo
- **Paths:**
  - `internal/cli/manage.go`
  - `internal/cli/manage_test.go`
- **Packages:** `github.com/evanmschultz/valv/internal/cli`
- **Acceptance:**
  1. `valv account bind <name> [--provider <p>]` binds the current project to the named account. When `--provider` is absent, provider is inferred from the existing binding or defaults to Codex. Calls `runManageBind` (the existing run function, already present in `manage.go`).
  2. `valv account unbind [--provider <p>]` unbinds the current project from its bound account. Implementation: call `service.UnbindProject(ctx, provider, projectPath)`. If the manage service does not expose `UnbindProject` yet, the builder may stub as `not yet implemented` and note the gap — but the preferred path is to implement it against `service.DeleteBinding` or equivalent (builder verifies service surface).
  3. `newManageAccountCommand` in `manage.go` adds both new subcommands via `cmd.AddCommand(newManageAccountBindCommand(...), newManageAccountUnbindCommand(...))`.
  4. At least two new tests in `manage_test.go` cover (a) bind succeeds and records the project-profile row; (b) unbind succeeds and removes the row. Table-driven is fine.
  5. `mage testPkg ./internal/cli` passes.
- **Blocked by:** 9.1

**Design notes for builder:**

`newManageBindCommand` (the old `manage bind` constructor) already contains the correct logic for binding — its positional was `<provider> <account>`. The new `account bind` takes `<name> [--provider]`. The builder should either:
(a) Rename the constructor and adjust positional parsing to accept `<name>` with `--provider` flag, or
(b) Create a new thin `newManageAccountBindCommand` that wraps `runManageBind` with the new flag/arg shape.

Option (b) is preferred (surgical, avoids touching the existing manage bind run path during a transition period).

For `unbind`: check whether `internal/services/manage.Service` already has a method that removes a project binding row. Hylla search on `UnbindProject` or `DeleteBinding` before implementing. If absent, add it to the service (new, not yet in tree).

---

### Unit 9.3 — Add `valv image` namespace (update, cleanup, inspect)

- **State:** todo
- **Paths:**
  - `internal/cli/manage.go`
  - `internal/cli/root.go`
  - `internal/cli/manage_test.go`
- **Packages:** `github.com/evanmschultz/valv/internal/cli`
- **Acceptance:**
  1. `valv image update [provider]` rebuilds the provider image. Calls `runManageUpdate` (existing run function, unchanged). Provider defaults to Codex when omitted.
  2. `valv image cleanup` prunes Valv-managed Docker artifacts. Uses flag-driven interface: `--images` (provider images), `--containers` (leaked Valv containers), `--state` (local logs/caches), `--build-cache` (Docker builder cache), `--all` (everything). No-arg default: dry-run preview (list what would be cleaned, print count, do not delete). Calls `runManageCleanup` logic internally, adapted for flag dispatch.
  3. `valv image inspect [provider]` shows the current provider image version, last-checked-at timestamp, and installed state. Provider defaults to Codex. Implementation reads from the images service state (builder may use `service.EnsureLatest` with `AllowExistingOnCheckFail: true` and no-rebuild semantics, OR call a new `service.InspectImage(ctx)` method if one exists — builder decides the cheapest approach after checking the service surface).
  4. `newImageCommand` is a new function in `manage.go` (new, not yet in tree) that constructs the `image` namespace cobra command with the three subcommands above. Registered in `root.go` under the `manage` group (or a renamed group).
  5. `providerCleanupImageFilters()` filter (`io.valv.managed=true`) is already sufficient — no new Dockerfile label instructions are needed because the images service sets this label via `docker build --label` args in `Build()`.
  6. At least one test in `manage_test.go` covers `valv image update` routing.
  7. `mage testPkg ./internal/cli` passes.
- **Blocked by:** 9.1

**Design notes for builder:**

`runManageUpdate` and `runManageCleanup` already exist in `manage.go`. Unit 9.3 creates a new `newImageCommand` function that wraps them with the new namespace. The existing `newManageUpdateCommand` and `newManageCleanupCommand` constructors become the inner constructors for `image update` and `image cleanup` — or the builder creates thin wrappers. Either path is fine; prefer reuse.

For `image cleanup` flag-driven dispatch: the current `runManageCleanup` uses a string switch on `scope`. Adapt it to accept a struct of flags (or add a new `runImageCleanup` with flag parameters). The cleanup service already supports all scope modes (`CleanLocal`, `CleanDocker`, `Clean`).

`valv image inspect` is a NEW command (not yet in tree). The builder should check `images.Service` for existing "inspect current state" surface before adding new methods. The `EnsureLatest` with `AllowExistingOnCheckFail: true` returns a result with `Action: EnsureActionUsingExistingImage` when the image exists — this may be sufficient for inspect output. Alternatively, a simpler `imageAvailable` check plus reading the state store is enough. Builder decides the minimal path.

---

### Unit 9.4 — Flatten `valv status` to top level

- **State:** todo
- **Paths:**
  - `internal/cli/root.go`
  - `internal/cli/manage_test.go`
- **Packages:** `github.com/evanmschultz/valv/internal/cli`
- **Acceptance:**
  1. `valv status` (no `manage` prefix) shows the current project's binding status. Same output as the current `manage status`.
  2. `newManageStatusCommand` is registered directly in `root.go` under the inspect group (or a new `status` group).
  3. `valv status --project /path` still works.
  4. The old `manage status` path no longer exists (because unit 9.1 deleted `manage`).
  5. At least one test covers `valv status` via the root command.
  6. `mage testPkg ./internal/cli` passes.
- **Blocked by:** 9.1

**Design notes for builder:**

`newManageStatusCommand` already exists in `manage.go`. Unit 9.4 just registers it in `root.go` as `statusCmd`. The `runManageStatus` run function needs no changes. The `manage project list` content (list of all project bindings) is separate — it becomes accessible via `valv status --all-projects` or remains a standalone path. Per the spec, `manage project list` content is folded into `valv status`; the simplest interpretation is: `valv status` shows the current project binding, and a new `--all` or `--projects` flag shows all bindings. Builder decides the minimal path; can keep it as a separate `valv account projects` command or fold as a flag. Spec's §5 shows `valv status` replacing both `manage status` + `manage project list` — builder adds a `--list` or `--all` flag that calls `runManageProjectList` logic.

---

### Unit 9.5 — Normalize `--provider` flag with positional fallback + `whoami` alias removal

- **State:** todo
- **Paths:**
  - `internal/cli/manage.go`
  - `internal/cli/manage_test.go`
- **Packages:** `github.com/evanmschultz/valv/internal/cli`
- **Acceptance:**
  1. `newManageAccountInspectCommand`: `whoami` alias removed from `Aliases`.
  2. `newManageAccountLoginCommand`, `newManageAccountLogoutCommand`, `newManageAccountDeleteCommand`, `newManageAccountRenameCommand`: each gains a `--provider <p>` flag. When `--provider` is present, it overrides positional provider parsing.
  3. For `delete`, `rename`, `inspect`, `login`, `logout`: when the account name matches multiple providers and `--provider` is absent, the command returns an error listing the `(provider, account)` pairs and instructing the user to add `--provider`. This mirrors the existing behavior in `resolveAccountSwitchTarget`.
  4. Positional `[provider]` fallback is preserved for backward ergonomics when no ambiguity exists (single-provider environments work unchanged).
  5. `newManageAccountAddCommand` positional `<provider> [name]` is unchanged (provider is always required for `add`).
  6. `newManageAccountListCommand` positional `[provider]` is unchanged.
  7. At least two tests cover the name-collision error path for one of the modified commands.
  8. `mage testPkg ./internal/cli` passes.
- **Blocked by:** 9.1

**Design notes for builder:**

The flag normalization is additive to the constructors — add `cmd.Flags().StringVar(&providerFlag, "provider", "", "...")` and pass it through to the run function. The run functions (`runManageAccountLogin`, `runManageAccountLogout`, etc.) call `resolveManagedAccount` → `resolveProfileSwitchTarget`. To add the `--provider` flag-priority path: either pass `providerFlag` into `resolveManagedAccount` and thread it into `resolveProfileSwitchTarget`, or add a pre-resolution step in each RunE that substitutes the flag-resolved provider before handing off.

Cross-provider name collision detection for non-switch verbs: the pattern in `resolveAccountSwitchTarget` (lines 900-973) searches all providers. This pattern should be extracted into a shared helper `resolveAccountByName(ctx, service, name, providerFlag)` that returns `(domain.Provider, domain.Profile, error)` and handles the collision error message consistently. (New helper, not yet in tree — the builder adds it.)

The `whoami` alias removal is a one-line change: remove `Aliases: []string{"whoami"}` from `newManageAccountInspectCommand`.

---

### Unit 9.6 — Require `--provider` on name collision in cross-provider commands (uniform enforcement)

- **State:** todo
- **Paths:**
  - `internal/cli/manage.go`
  - `internal/cli/manage_test.go`
- **Packages:** `github.com/evanmschultz/valv/internal/cli`
- **Acceptance:**
  1. `valv account delete`, `valv account rename`, `valv account inspect`, `valv account login`, `valv account logout`, and `valv account bind` each return a descriptive error when the account name is ambiguous across providers and `--provider` is absent. Error message lists the `(provider, account)` pairs.
  2. `valv account switch` already implements this (via `resolveAccountSwitchTarget`) — no change needed; builder verifies it still works.
  3. The shared helper introduced in unit 9.5 (`resolveAccountByName` or equivalent) is the implementation vehicle.
  4. Table-driven tests covering: (a) single-provider — resolves without `--provider`; (b) multi-provider collision — error with pair listing; (c) `--provider` present — resolves to the specified provider.
  5. `mage testPkg ./internal/cli` passes.
- **Blocked by:** 9.5

**Note:** Units 9.5 and 9.6 are separated because 9.5 introduces the helper + individual flag additions, and 9.6 ensures uniform collision enforcement across all verbs. If the builder can do both in one pass, 9.6 may be collapsed into 9.5 (orchestrator decides at build time based on unit size).

---

### Unit 9.7 — CONCERN A: Static `ContainerRunRequest` field assertion tests

- **State:** todo
- **Paths:**
  - `internal/cli/claude_auth_test.go`
- **Packages:** `github.com/evanmschultz/valv/internal/cli`
- **Acceptance:**
  1. A new test `TestSystemClaudeAccountAuthRunnerBuildsExpectedContainerRequest` (new, not yet in tree) constructs `systemClaudeAccountAuthRunner` with an injected `stubAuthContainerExecutor` (already defined in `claude_auth_test.go`), calls `RunInContainer` against a `t.TempDir()` home, captures `stub.lastRequest`, and asserts each field:
     - `Env["CLAUDE_CONFIG_DIR"]` equals `claudeprovider.ContainerClaudeDir`.
     - `Env["HOME"]` equals `claudeprovider.ContainerHomeDir`.
     - `Env["LOGNAME"]` equals `"valv"`.
     - `Env["USER"]` equals `"valv"`.
     - `Env["TERM"]` is non-empty (default `"xterm-256color"` when `TERM` env is absent).
     - `len(Mounts)` equals 1 and `Mounts[0].Source` equals the injected `homePath` and `Mounts[0].Target` equals `claudeprovider.ContainerClaudeDir`.
     - `Args` is empty (`[]string{}`).
     - `Init` is `true`.
     - `Remove` is `true`.
     - `User` equals `currentContainerUser()` result.
  2. A parallel codex-equivalent test `TestSystemCodexAccountAuthRunnerBuildsExpectedContainerRequest` is added if the codex path has an analogous runner struct (builder checks `codex_setup.go` / `codex.go` — if no analogous injectable runner exists, document the gap in the worklog and skip).
  3. Both tests use `t.Parallel()`.
  4. `mage testPkg ./internal/cli` passes.
- **Blocked by:** 9.6

**Design notes for builder:**

`stubAuthContainerExecutor` is already defined (captures `lastRequest dockeradapter.ContainerRunRequest`). The `systemClaudeAccountAuthRunner` struct has a public `executor` field. Inject by constructing `systemClaudeAccountAuthRunner{executor: &stubAuthContainerExecutor{}, image: claudeImageRef()}` and calling `.RunInContainer(ctx, tmpHome, nil, &bytes.Buffer{}, &bytes.Buffer{})`. The run will succeed (executor returns nil by default) and `stub.lastRequest` will have the full field set.

The `claudeprovider.ContainerClaudeDir` and `claudeprovider.ContainerHomeDir` constants are in `internal/adapters/providers/claude` — import them as the test already does.

---

### Unit 9.8 — CONCERN B: Ctrl-C exit hardening (`ensureClaudeAccountReady` / `loginClaudeAccount`)

- **State:** todo
- **Paths:**
  - `internal/cli/claude_auth.go`
  - `internal/cli/claude_auth_test.go`
- **Packages:** `github.com/evanmschultz/valv/internal/cli`
- **Acceptance:**
  1. In `ensureClaudeAccountReady`: after `runner.RunInContainer(...)` returns a non-nil error, the function checks whether `.credentials.json` exists in `account.HomePath` and is non-empty. If credentials ARE present: log the container error at debug level (do NOT propagate), continue to the `ReadAccountIdentity` step. If credentials are absent: propagate the original container error as before.
  2. In `loginClaudeAccount`: same logic applied after the `runner.RunInContainer(...)` call.
  3. Two new tests cover the new behavior:
     - `TestEnsureClaudeAccountReadySucceedsWhenCredsLandDespiteContainerError`: container run returns a non-nil error but the `stubRunFunc` writes `.credentials.json`. Asserts `ensureClaudeAccountReady` returns nil (not an error).
     - `TestLoginClaudeAccountSucceedsWhenCredsLandDespiteContainerError`: same pattern for `loginClaudeAccount`.
  4. Existing tests that assert container error propagation (e.g., `TestEnsureClaudeAccountReadyPropagatesContainerError`) must still pass — the hardening only suppresses errors when creds ARE present, not always.
  5. `mage testPkg ./internal/cli` passes.
- **Blocked by:** 9.7

**Design notes for builder:**

The credentials check logic already exists in `ensureClaudeAccountReady` before the `RunInContainer` call (lines 141-148 approximately — `os.Stat(credPath)` + size check). Extract or inline the same check as a post-run fallback. Pseudocode:

```go
if runErr := runner.RunInContainer(...); runErr != nil {
    info, statErr := os.Stat(credPath)
    if statErr == nil && info.Size() > 0 {
        // Creds landed despite non-zero exit (e.g. Ctrl-C × 2).
        logger.Debug("container exited non-zero but credentials present; treating as success", "error", runErr)
        // Fall through to ReadAccountIdentity.
    } else {
        return fmt.Errorf("run claude auth container for account %q: %w", account.Name, runErr)
    }
}
```

The `logger` is available via `LoggerFromContext(cmd.Context())`.

---

## Notes

### `account cleanup` decision
`valv account cleanup` is already under the `account` namespace (not under `manage`). The spec's "absorbed into account delete semantics" means the `manage account cleanup` DUPLICATE PATH goes away when `manage` is deleted (unit 9.1). The `account cleanup` command itself survives as-is under the top-level `account` namespace. No change to its behavior or location.

### Label filter for `valv image cleanup`
The images service `Build()` already sets `io.valv.managed=true` via `docker build --label` args. The `providerCleanupImageFilters()` function already uses this label for filtering. No new Dockerfile `LABEL` instruction is needed in the Dockerfile constants. Unit 9.3 confirms this is sufficient and uses the existing filter.

### `manage project list` → `valv status`
Per spec, `valv status` replaces both `manage status` and `manage project list`. The simplest path: `valv status` shows current project binding (current behavior), plus a `--all` flag that shows all project bindings (calling `runManageProjectList` logic). The builder may also introduce `valv account projects` as an alternative; the spec is not prescriptive here. Builder decides the minimal path.

### `image inspect` service surface
`valv image inspect` is a new command. The builder checks `images.Service` for an inspect method before adding one. The `EnsureLatest` with `AllowExistingOnCheckFail: true` returns `EnsureActionUsingExistingImage` when the image is present — this suffices for displaying current version + last-checked timestamp. No new service method should be added unless the existing path is truly insufficient.

### Units 9.5 + 9.6 potential collapse
If the builder finds that `--provider` flag normalization + uniform collision enforcement fit cleanly within a single round, the orchestrator may direct the builder to combine 9.5 and 9.6 into one unit at build time.

### `internal/tui/manage` package
`internal/tui/manage` (the Bubble Tea home screen model) is no longer reached from the CLI after unit 9.1 deletes `runManageHome`. It becomes dead code. This drop does NOT delete the TUI package — that cleanup belongs in DROP_11. Builder notes this in the worklog.

### Dependency chain
```
9.1 (manage deletion) → 9.2 (account bind/unbind) → 9.3 (image namespace) → 9.4 (status flatten) → 9.5 (flag normalization) → 9.6 (collision enforcement) → 9.7 (CONCERN A tests) → 9.8 (CONCERN B hardening)
```
All units in one package (`internal/cli`). No parallelism possible within this drop due to the package lock. All units are serial.
