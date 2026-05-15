# DROP_5 — CLAUDE LAUNCHER

**State:** done
**Blocked by:** DROP_4 (done)
**Paths (expected):** `internal/adapters/providers/claude/` (new package — `profile.go`, `account.go`, `runtime.go` + tests), `internal/services/claude/` (new package — `service.go` + tests), `internal/cli/claude.go` (new), `internal/cli/claude_test.go` (new), `internal/cli/root.go` (edit — register `newClaudeCommand` in the `runtime` group + add Claude examples to root `Example` string), `internal/services/manage/service.go` (edit — flip the DROP_2 `DefaultHostProfile(ProviderClaude)` sentinel stub to `claudeprovider.DefaultHostProfile(homeDir)`), `internal/cli/account_auth.go` (edit — flip the DROP_2 `case domain.ProviderClaude: return nil` no-op stubs to real Claude-aware bodies where the design calls for it; expectation is mostly still no-op for v1 since the focus plan §3.2 specifies device-code auth happens inside the container at launch time), `magefile.go` (edit — add small `Install` target)
**Packages (expected):** `internal/adapters/providers/claude` (new), `internal/services/claude` (new), `internal/cli` (edits)
**PLAN.md ref:** main/PLAN.md → DROP_5_CLAUDE_LAUNCHER row
**Workflow:** main/drops/WORKFLOW.md
**Started:** 2026-05-14
**Closed:** 2026-05-15

## Scope

**Dogfood milestone.** Land everything needed to run `valv claude` end-to-end on a per-project basis: the Claude provider adapter trio (`profile.go`, `account.go`, `runtime.go`) mirroring `internal/adapters/providers/codex/`, the Claude launch service mirroring `internal/services/codex/service.go`, the `valv claude` pass-through CLI mirroring `internal/cli/codex.go`, and the wiring that makes `newClaudeCommand` discoverable under the `runtime` group. After this drop closes, the dev can `valv claude` from inside a project bound to a Claude managed account, `claude --resume` works because session state lives in the bind-mounted `/home/valv/.claude` dir, and Codex behavior is unchanged.

Collapses old DROP_5 (adapter) + DROP_6 (service+CLI) from the prior plan numbering. Globalswitch parity, `valv account list` cross-provider, `valv account switch` cross-provider, and full TUI picker golden parity are NOT in this drop — they live in DROP_7. `valv account add claude [name]` is NOT in this drop — that's DROP_6 (a single-feature drop right after this one).

Includes a 5-line `mage install` target so the dev can `mage install` after this drop closes and have `valv` on PATH for dogfooding.

## Dev-Confirmed Decisions (2026-05-14)

1. **Claude host-default account = isolated-first.** `claudeprovider.DefaultHostProfile(homeDir)` returns `(name "default", homePath filepath.Join(homeDir, ".valv", "providers", "claude", "profiles", "default"), nil)`, NOT `~/.claude`. Reason: macOS Claude credentials live in keychain; `~/.claude` on host is functionally empty for auth purposes. The Valv-managed path starts empty and gets device-code-auth on first `valv claude` run inside the container.
2. **Cross-provider account name collision = require `--provider` when ambiguous.** Not in scope for this drop's CLI — relevant when `valv account switch` lands in DROP_7. Recorded here so the adapter / service layer doesn't bake in a different assumption.
3. **Globalswitch is NOT extended to Claude in this drop.** Deferred to DROP_7. This drop must compile cleanly with `internal/services/globalswitch/service.go` still rejecting non-Codex providers (existing behavior at `service.go:86`). No edit to globalswitch in this drop.

## Planner

### Scope confirmation

In scope for DROP_5:

- `internal/adapters/providers/claude/` — new package: `profile.go` (`DefaultHostProfile`, `IsDefaultHostHome`), `account.go` (`ReadAccountIdentity` — presence check for `.credentials.json`, no JWT parsing), `runtime.go` (`PrepareRuntime`, `PreparedRuntime`, `PrepareRequest`, `ContainerHomeDir = "/home/valv"`, `ContainerClaudeDir = "/home/valv/.claude"`, `terminalEnvPassthrough` duplicated). NO `bridge.go`.
- `internal/services/claude/` — new package: `service.go` mirroring `internal/services/codex/service.go`. Key delta: no `sharedClaudeStateHome` helper (isolated-first model — profile home is the mount home directly). Provider guards use `domain.ProviderClaude`. Container label `"io.valv.provider": "claude"`.
- `internal/cli/claude.go` — new file mirroring `internal/cli/codex.go`. No `ensureClaudeBindingReady` first-run setup (out of scope for v1). `ensureClaudeImageCurrent` uses `service.Build(ctx, imagesservice.BuildRequest{Version: imagesservice.DefaultClaudeCLIVersion})` instead of `EnsureLatest`.
- `internal/cli/root.go` — edit: insert `claudeCmd := newClaudeCommand(paths, nil)` + `claudeCmd.GroupID = "runtime"` and add `claudeCmd` to `cmd.AddCommand(...)`. Add `valv claude --help` to root `Example` string.
- `internal/services/manage/service.go` — edit: flip `case domain.ProviderClaude` stub (line 159-163) to call `claudeprovider.DefaultHostProfile(s.homeDir)` and import the new package.
- `magefile.go` — edit: add `Install` target (~5 LOC) calling `runGo("install", "./cmd/valv")`, mirroring `Build`.

Explicitly out of scope (deferred):
- First-run interactive setup for Claude (DROP_6 or later).
- `claude_integration_test.go` (DROP_7 per focus-plan §6.7).
- Globalswitch extension to Claude (DROP_7).
- `bridge.go` equivalent (not needed for v1).

DO NOT edit:
- `internal/cli/account_auth.go` — already has `case domain.ProviderClaude: return nil` at lines 39, 51, 63. No changes needed.
- `internal/cli/claude_image.go` — already committed by DROP_4 (`claudeImageRef`, `claudeImageRepository`, `claudeImageTag`). Do not touch.
- `internal/cli/operator_helpers.go` — `openImagesService` already dispatches `ProviderClaude` with `Resolver: nil`. No changes needed.
- `internal/services/globalswitch/service.go` — stays Codex-only per decision #3.

### Committed-state audit

Evidence verified by direct file reads (2026-05-14, HEAD post-DROP_4 commit).

- `internal/adapters/providers/codex/profile.go:13-23` — `DefaultHostProfile(homeDir string) (name, homePath string, err error)` uses `filepath.Join(trimmedHome, ".codex")`. Claude version uses `filepath.Join(trimmedHome, ".valv", "providers", "claude", "profiles", "default")` per dev decision #1.
- `internal/adapters/providers/codex/account.go:32-62` — reads `auth.json` + JWT decoding. Claude `ReadAccountIdentity` replaces this with: check `os.Stat(filepath.Join(homePath, ".credentials.json"))`; `LoggedIn: true` iff stat succeeds and is not a dir; no JSON parse, no JWT decode.
- `internal/adapters/providers/codex/runtime.go:19-22` — `ContainerHomeDir = "/home/valv"`, `ContainerCodexDir = "/home/valv/.codex"`. Claude constants: same `ContainerHomeDir`; `ContainerClaudeDir = "/home/valv/.claude"`.
- `internal/adapters/providers/codex/runtime.go:48-218` — `PrepareRuntime`. Claude version omits: `newBridgeManager`, `translateConfigFile` (profile config), project config overlay (`translateConfigFile` on `.codex/config.toml`). Claude version keeps: `tempRoot` creation, `runtimeDir` creation, shared-home copy pattern (fires only when `SharedHome != ProfileHome` — Claude v1 always passes equal paths so the copy is skipped), mounts, env map, `terminalEnvPassthrough`, cleanup func with `os.RemoveAll(runtimeDir)`. Sync-back exclusion list changes from `{"auth.json": {}, "config.toml": {}}` to `{".credentials.json": {}}`.
- `internal/adapters/providers/codex/runtime.go:538-553` — `terminalEnvPassthrough` (16 LOC). Duplicated verbatim into `claude/runtime.go` per focus-plan §3.2 v1 decision.
- `internal/adapters/providers/codex/runtime.go:227-563` — helper funcs (`copyDirContents`, `syncDirContents`, `copyFile`, `appendUniqueStrings`, `errorsJoin`, `normalizedContainerTERM`, `debugLog`). All ported verbatim to Claude adapter (they are purely mechanical, provider-agnostic helpers).
- `internal/services/codex/service.go:171-183` — `sharedCodexStateHome` — NOT ported. Claude service passes `profile.HomePath` directly as the mount home. No "merge with global ~/.claude" logic exists in v1.
- `internal/services/codex/service.go:196-243` — `resolveBinding` calls `s.store.BindingByProjectID(ctx, projectRecord.ID, domain.ProviderCodex)` and asserts `binding.Provider == domain.ProviderCodex`, `profile.Provider == domain.ProviderCodex`. Claude version: `domain.ProviderClaude` in all three places.
- `internal/services/codex/service.go:314-323` — `containerName` returns `valv-codex-interactive-<name>-<ns>`. Claude version returns `valv-claude-interactive-<name>-<ns>`.
- `internal/cli/codex.go:27-57` — `newCodexCommand`. Claude version: `Use: "claude"`, Long description adapted to Claude.
- `internal/cli/codex.go:59-130` — `runCodexCommand`. Claude version: no `ensureCodexBindingReady` call. Unbound-project error surfaces from `service.ValidateBinding` directly.
- `internal/cli/codex.go:239-256` — `ensureCodexImageCurrent`. Claude version: when `VALV_CLAUDE_IMAGE` env is unset, calls `openImagesService(cmd, paths, domain.ProviderClaude)` then `service.Build(ctx, imagesservice.BuildRequest{Version: imagesservice.DefaultClaudeCLIVersion})` (not `EnsureLatest` — Claude uses pinned-version fast path).
- `internal/cli/root.go:119-132` — `codexCmd` is registered; Claude slot is absent. Add `claudeCmd := newClaudeCommand(paths, nil)` + `claudeCmd.GroupID = "runtime"` + include in `cmd.AddCommand(...)`.
- `internal/cli/root.go:55-63` — root `Example` has no Claude entries. Add `valv claude --help` (at minimum) per focus-plan §3.4 mandatory field rule.
- `internal/services/manage/service.go:151-163` — `DefaultHostProfile` switch. `case domain.ProviderClaude:` currently returns stub error. Flip to call `claudeprovider.DefaultHostProfile(s.homeDir)`; add `claudeprovider` import.
- `magefile.go:49-69` — `Build` target uses `runGo("build", "-o", "./valv", "./cmd/valv")`. `Install` target uses `runGo("install", "./cmd/valv")`. No `-o` flag for install.

### Atomic decomposition

Package-lock chain:

```
5.1 (internal/adapters/providers/claude — new package)
  └─▶ 5.2 (internal/services/claude — new package, imports 5.1)
        └─▶ 5.3 (internal/cli/claude.go + root.go + manage/service.go stub flip — imports 5.1 + 5.2)
                  └─▶ 5.4 (magefile.go Install target — policy dep on 5.3 for correct binary, not compile dep)
```

5.1 and 5.4 touch disjoint packages but 5.4 is `blocked_by: [5.3]` to ensure the installed binary includes `valv claude` before dogfooding.

---

#### Unit 5.1 — Claude provider adapter package (new package)

**State:** done
**Paths:**
- `internal/adapters/providers/claude/profile.go` (new)
- `internal/adapters/providers/claude/profile_test.go` (new)
- `internal/adapters/providers/claude/account.go` (new)
- `internal/adapters/providers/claude/account_test.go` (new)
- `internal/adapters/providers/claude/runtime.go` (new)
- `internal/adapters/providers/claude/runtime_test.go` (new)
**Packages:** `internal/adapters/providers/claude` (new)
**Blocked by:** —

**Description**

Create `internal/adapters/providers/claude/` as a new Go package mirroring the Codex adapter with Claude-specific substitutions. No `bridge.go` — MCP bridging is out of scope for v1.

`profile.go`:

- `HostDefaultProfileName = "default"` (constant, same as Codex).
- `DefaultHostProfile(homeDir string) (name string, homePath string, err error)` — returns `(HostDefaultProfileName, filepath.Join(trimmedHome, ".valv", "providers", "claude", "profiles", "default"), nil)` after `pathutil.Normalize`. This is the dev-confirmed isolated-first path (NOT `~/.claude`). Error message: `"resolve claude host profile: ..."`.
- `IsDefaultHostHome(profileHome, homeDir string) bool` — same pattern as Codex: normalize both, compare.

`account.go`:

- `AccountIdentity` struct: `Email`, `Name`, `AuthMode`, `LoggedIn` (same shape as Codex — keeps consumer-side compatibility).
- `ReadAccountIdentity(homePath string) (AccountIdentity, error)` — Claude credentials are not in `auth.json`. Check `os.Stat(filepath.Join(strings.TrimSpace(homePath), ".credentials.json"))`; if file exists and is not a dir, return `AccountIdentity{LoggedIn: true}`; if `os.IsNotExist`, return zero value and nil error; other stat errors wrap and return. Do NOT parse the file content — the focus-plan §3.4 contract is "presence check only". `AuthMode`, `Email`, `Name` remain zero-valued (Claude v1 does not parse credential metadata from disk).

`runtime.go`:

- Constants: `ContainerHomeDir = "/home/valv"` (same as Codex), `ContainerClaudeDir = "/home/valv/.claude"`.
- `PrepareRequest` struct: identical fields to `codex.PrepareRequest` (ProfileHome, SharedHome, ProjectRoot, TempRoot, Logger).
- `PreparedRuntime` struct: identical fields + `Close() error` method.
- `PrepareRuntime(ctx context.Context, request PrepareRequest) (PreparedRuntime, error)` — same structure as `codex.PrepareRuntime` with these substitutions:
  - No `newBridgeManager` call.
  - No `translateConfigFile` calls (no profile config or project config translation in v1).
  - `envPassthrough := terminalEnvPassthrough()` (direct call, no merge from config translation).
  - Env map uses `"CLAUDE_CONFIG_DIR": ContainerClaudeDir` instead of `"CODEX_HOME": ContainerCodexDir`.
  - Mount: `dockeradapter.NewMountSpec(runtimeClaudeHome, ContainerClaudeDir, false)`.
  - Cleanup sync-back exclusion: `{".credentials.json": {}}` instead of `{"auth.json": {}, "config.toml": {}}`.
  - Temp dir prefix: `"claude-runtime-"`.
  - Runtime home variable: `runtimeClaudeHome` (analogous to `runtimeCodexHome`).
  - Debug log messages use "claude" not "codex".
- Duplicate all mechanical helpers from Codex runtime verbatim: `copyDirContents`, `syncDirContents`, `copyFile`, `terminalEnvPassthrough`, `normalizedContainerTERM`, `debugLog`, `appendUniqueStrings`, `errorsJoin`. These are provider-agnostic; they will be deduped in DROP_9.
- No `translateRequest`, `translateResult`, `translateConfigFile`, `translateMCPServers`, or any bridge types.

Tests in `profile_test.go`:

- `TestDefaultHostProfileReturnsIsolatedPath` — verify `DefaultHostProfile(homeDir)` returns `(name="default", path=filepath.Join(homeDir, ".valv", "providers", "claude", "profiles", "default"), nil)`.
- `TestDefaultHostProfileRejectsEmptyHome` — verify error returned for empty homeDir.
- `TestIsDefaultHostHomeReturnsTrue` — construct the isolated path from a known homeDir, verify `IsDefaultHostHome` returns true.
- `TestIsDefaultHostHomeReturnsFalse` — supply a different path.

Tests in `account_test.go`:

- `TestReadAccountIdentityCredentialsFilePresent` — write a minimal `.credentials.json` to a temp dir; verify `LoggedIn: true`, `Email: ""`, `Name: ""` (presence-only check).
- `TestReadAccountIdentityMissingCredentialsReturnsNotLoggedIn` — temp dir with no credentials file; verify `LoggedIn: false`, nil error.
- `TestReadAccountIdentityIrrelevantContentsStillLoggedIn` — write `.credentials.json` with arbitrary bytes (e.g. `{}`); verify `LoggedIn: true` (format is irrelevant — presence only).

Tests in `runtime_test.go`:

- `TestPrepareRuntimeSetsClaudeConfigDirEnv` — call `PrepareRuntime` with valid temp dirs; verify `prepared.Env["CLAUDE_CONFIG_DIR"] == ContainerClaudeDir`, `prepared.Env["HOME"] == ContainerHomeDir`, `prepared.Env["USER"] == "valv"`.
- `TestPrepareRuntimeMountsClaudeDir` — verify `prepared.Mounts` contains a mount with `Target == ContainerClaudeDir`.
- `TestPrepareRuntimeHasNoCodexEnv` — verify `prepared.Env` does NOT contain key `"CODEX_HOME"` (guard against copy-paste drift).
- `TestPrepareRuntimeCleanupRemovesTempDir` — call `prepared.Close()`; verify the temp runtime dir is removed.

**Acceptance:**

- `mage testPkg ./internal/adapters/providers/claude` green (gofumpt + 70% coverage per-package gate).
- `internal/adapters/providers/claude` compiles with no imports of `internal/adapters/providers/codex` (no cross-contamination — verified by `go build ./internal/adapters/providers/claude`).
- `DefaultHostProfile(homeDir)` returns path containing `.valv/providers/claude/profiles/default` — NOT `.claude` directly.
- `ReadAccountIdentity` returns `LoggedIn: true` for any dir containing `.credentials.json` (file present), `LoggedIn: false` for empty dir.
- `PrepareRuntime` produces env with `CLAUDE_CONFIG_DIR=/home/valv/.claude` and NO `CODEX_HOME` key.
- No `bridge.go` in the package directory.
- **RecipeHash-style audit:** builder must grep the new package for any reference to `codex`, `CODEX_HOME`, `ContainerCodexDir`, `auth.json`, or `config.toml` and confirm none exist (except in test helper names if paralleling Codex test structure — but even there prefer Claude-native naming). Record findings in `BUILDER_WORKLOG.md`.

---

#### Unit 5.2 — Claude launch service package (new package)

**State:** done
**Paths:**
- `internal/services/claude/service.go` (new)
- `internal/services/claude/service_test.go` (new)
**Packages:** `internal/services/claude` (new)
**Blocked by:** 5.1

**Description**

Create `internal/services/claude/service.go` mirroring `internal/services/codex/service.go` (10.2K, 347 LOC) with these substitutions:

- Package name: `claude`.
- Import: `clauderuntime "github.com/evanmschultz/valv/internal/adapters/providers/claude"` instead of `codexruntime "...codex"`.
- `New` error messages: `"new claude launch service: ..."`.
- `Run` calls `clauderuntime.PrepareRuntime(ctx, clauderuntime.PrepareRequest{...})`.
- `sharedCodexStateHome` is NOT ported. Instead, `service.Run` passes `ProfileHome: resolved.profile.HomePath` and `SharedHome: ""` (empty) to `PrepareRuntime`. The Claude adapter treats empty `SharedHome` the same as Codex does when profile == shared: no temp copy, direct mount of `profileHome`. This is the isolated-first model.
- `resolveBinding` uses `domain.ProviderClaude` in all three provider checks: `BindingByProjectID(ctx, projectRecord.ID, domain.ProviderClaude)`, `binding.Provider != domain.ProviderClaude`, `profile.Provider != domain.ProviderClaude`.
- `buildRequest` sets label `"io.valv.provider": "claude"`.
- `containerName` returns `fmt.Sprintf("valv-claude-interactive-%s-%d", base, s.now().UnixNano())`.
- `emitNotices` debug string: `"claude runtime warning"`.
- `debug` messages: `"launching claude container"`, etc.
- `runAttached` log: `"starting interactive claude container"`.
- Error messages throughout use `"run claude launch service: ..."`.

`Options` and `Service` struct fields: identical to Codex service (Store, Executor, Detect, Image, User, TTY, Stdin, TempRoot, Now, RealHome, Logger, Notices). `RealHome` field is present for structural parity but unused in v1 (no shared-home merging logic).

Tests in `service_test.go` (mirror `internal/services/codex/service_test.go:16.7K` structure):

- Reuse fake types: `fakeStore` (same interface — implements `domain.ProjectRepository`, `domain.BindingRepository`, `domain.ProfileRepository`), `fakeExecutor` (implements `Executor`). Builder may adapt the Codex service test's fakes directly; the interface is identical.
- `TestRunSucceedsWithBoundProject` — wires a `fakeStore` returning a valid project + `ProviderClaude` binding + `ProviderClaude` profile; `fakeExecutor` records the `ContainerRunRequest`; verifies the recorded request has `Image`, mounts containing `ContainerClaudeDir`, and labels `"io.valv.provider": "claude"`.
- `TestRunReturnsUnboundProjectWhenNoProject` — `fakeStore.projectErr = domain.ErrNotFound`; verify error wraps `domain.ErrUnboundProject`.
- `TestRunReturnsUnboundProjectWhenNoBinding` — `fakeStore.bindingErr = domain.ErrNotFound`; verify error wraps `domain.ErrUnboundProject`.
- `TestRunRejectsWrongBindingProvider` — `fakeStore.binding.Provider = domain.ProviderCodex`; verify error contains "expected".
- `TestRunRejectsWrongProfileProvider` — `fakeStore.profile.Provider = domain.ProviderCodex`; verify error contains "expected".
- `TestValidateBindingReturnsNilForBoundProject` — identical to Codex service test, with `ProviderClaude` store.

**Acceptance:**

- `mage testPkg ./internal/services/claude` green (gofumpt + 70% coverage).
- No reference to `sharedCodexStateHome` or `codexruntime.DefaultHostProfile` in the new package — verified by builder in `BUILDER_WORKLOG.md`.
- `resolveBinding` correctly passes `domain.ProviderClaude` to `BindingByProjectID` — verified by `TestRunSucceedsWithBoundProject` observing the fakeStore call.
- Container label `"io.valv.provider"` equals `"claude"` — verified by `TestRunSucceedsWithBoundProject` asserting the recorded request labels.
- `New` returns error when `Store` is nil, when `Executor` is nil, when `Image.Repository` is empty — verified by table-driven constructor test.

---

#### Unit 5.3 — CLI + root wiring + manage stub flip

**State:** done
**Paths:**
- `internal/cli/claude.go` (new)
- `internal/cli/claude_test.go` (new)
- `internal/cli/root.go` (edit)
- `internal/services/manage/service.go` (edit — stub flip only, ~8 LOC change + import)
**Packages:** `internal/cli` (edits + new file), `internal/services/manage` (edit)
**Blocked by:** 5.1, 5.2

**Description**

`internal/cli/claude.go`:

Mirror `internal/cli/codex.go` (9.2K, 303 LOC) with these substitutions:

- `newClaudeCommand(paths config.Paths, run claudeRunFunc) *cobra.Command` — `Use: "claude"`, Long/Short/Example adapted to Claude. `Example` must include at least `valv claude --help` and `valv claude --version`.
- `runClaudeCommand(cmd *cobra.Command, paths config.Paths, args []string) error`:
  - If `claudeArgsSkipProjectBinding(args)`, call `runClaudeImageOnlyCommand`.
  - Get `workingDir` via `os.Getwd()`.
  - No `ensureClaudeBindingReady` call (no first-run interactive setup — unbound projects surface as `ErrUnboundProject` from `service.ValidateBinding`).
  - No `ensureBoundClaudeAccountReady` call (Claude auth is in-container; `account_auth.go` no-op already handles provider=claude).
  - Construct `claudeservice.Service` from `openStore`, `dockeradapter.NewExecutor`, `claudeImageRef()`, TTY flags, etc.
  - Call `ensureClaudeImageCurrent(cmd, paths)`.
  - Call `service.ValidateBinding(cmd.Context(), workingDir)`.
  - Call `service.Run(cmd.Context(), workingDir, args)`.
- `runClaudeImageOnlyCommand` — parallels `runCodexImageOnlyCommand`: builds a `ContainerRunRequest` with `claudeprovider.ContainerClaudeDir` and `claudeprovider.ContainerHomeDir` constants, runs without project binding. Env uses `CLAUDE_CONFIG_DIR`.
- `claudeArgsSkipProjectBinding(args []string) bool` — identical logic to `codexArgsSkipProjectBinding` (checks `--help`, `-h`, `--version`, `-V`, `help`).
- `ensureClaudeImageCurrent(cmd *cobra.Command, paths config.Paths) error`:
  - If `VALV_CLAUDE_IMAGE` env is non-empty, call `ensureClaudeImageAvailable(cmd.Context(), runner, claudeImageRef())` (parallel to `ensureCodexImageAvailable`).
  - Otherwise: `openImagesService(cmd, paths, domain.ProviderClaude)` → `service.Build(ctx, imagesservice.BuildRequest{Version: imagesservice.DefaultClaudeCLIVersion})`. Note: `service.Build` returns a `BuildResult` not used here — only error matters.
- `claudeImageVersionRef` is NOT needed (no dynamic version comparison in v1).
- `type claudeRunFunc func(*cobra.Command, []string) error` — parallel to `codexRunFunc`.

`internal/cli/root.go` edits:

- After `codexCmd := newCodexCommand(paths, nil)` at line 123, add: `claudeCmd := newClaudeCommand(paths, nil)` + `claudeCmd.GroupID = "runtime"`.
- Update `cmd.AddCommand(...)` at line 132 to include `claudeCmd`.
- Add `valv claude --help` to the root `Example` string (line 55-63). Suggested addition: `valv claude --help` and `valv claude --version` after the `valv codex --help` line.

`internal/services/manage/service.go` edit:

- Flip `case domain.ProviderClaude:` (lines 159-163) to call `claudeprovider.DefaultHostProfile(s.homeDir)` and return a proper `HostProfileSpec`. Add `claudeprovider "github.com/evanmschultz/valv/internal/adapters/providers/claude"` to the import block.
- The stub error text `"resolve default host profile for provider %q: not yet available"` is replaced by the real call. All error wrapping follows the existing pattern at lines 154-158.

`internal/cli/claude_test.go` (new):

Mirror `internal/cli/codex_test.go` (15.2K). Key cases:
- `TestNewClaudeCommandHelp` — `valv claude --help` exits 0 and output contains "claude" and "Docker".
- `TestNewClaudeCommandVersion` — `valv claude --version` routes through `runClaudeImageOnlyCommand`; with a fake runner that captures args, verify the docker run call includes `claude` in the image label env or command.
- `TestRunClaudeCommandUnboundProject` — with a fake store returning `domain.ErrNotFound` for the project, verify error wraps `domain.ErrUnboundProject`.
- `TestRunClaudeCommandRejectsWrongProvider` — fakeStore with a Codex-provider binding; verify error surfaces provider mismatch.
- `TestClaudeArgsSkipProjectBinding` — table-driven: `["--help"]` → true, `["--version"]` → true, `["help"]` → true, `[]` → false, `["--prompt", "foo"]` → false.

**Acceptance:**

- `mage testPkg ./internal/cli` green (gofumpt + 70% coverage — note: the 70% gate applies to the whole `internal/cli` package; new Claude code must not drag the per-package coverage below the threshold).
- `mage testPkg ./internal/services/manage` green (gofumpt + 70% coverage — stub flip is a small change; existing test coverage remains valid).
- `mage build` produces a binary where `./valv --help` shows `claude` in the Runtime Commands group.
- `./valv claude --help` exits 0 and prints usage text including "claude" and "Docker".
- `./valv claude --version` triggers `runClaudeImageOnlyCommand` (no project binding needed) — verified by `claudeArgsSkipProjectBinding(["--version"]) == true` in unit tests.
- `internal/services/manage.Service.DefaultHostProfile(domain.ProviderClaude)` returns a `HostProfileSpec` with `Name: "default"` and `HomePath` containing `.valv/providers/claude/profiles/default` — not a stub error. Verified by `mage testPkg ./internal/services/manage` (existing test coverage or new test added by builder to cover the non-stub path).
- No reference to `ensureCodexBindingReady` or `runCodexFirstRunSetup` in `claude.go` — verified by builder in `BUILDER_WORKLOG.md`.
- **Shared-home audit:** builder confirms `claude.go` passes `SharedHome: ""` (or omits it, letting the adapter default to empty) to `clauderuntime.PrepareRuntime`, and that no `sharedClaudeStateHome` helper is created in `claude.go`. Record in `BUILDER_WORKLOG.md`.

---

#### Unit 5.4 — `mage install` target

**State:** done
**Paths:** `magefile.go`
**Packages:** (mage build system — not a Go package)
**Blocked by:** 5.3

**Description**

Add an `Install` mage target to `magefile.go` that installs the `valv` binary via `go install`. Mirrors the `Build` target shape at lines 49-69, but calls `runGo("install", "./cmd/valv")` instead of `runGo("build", "-o", "./valv", "./cmd/valv")`.

Suggested implementation (builder may refine):

```go
// Install installs the valv binary to $GOBIN (or $GOPATH/bin).
func Install() error {
	printer := newMagePrinter(os.Stdout)
	if err := printer.StatusLine(laslig.StatusLine{
		Level:  laslig.NoticeInfoLevel,
		Text:   "Installing valv",
		Detail: "./cmd/valv",
	}); err != nil {
		return fmt.Errorf("write install start: %w", err)
	}
	if err := runGo("install", "./cmd/valv"); err != nil {
		return err
	}
	if err := printer.StatusLine(laslig.StatusLine{
		Level:  laslig.NoticeSuccessLevel,
		Text:   "Installed valv",
		Detail: "$(go env GOBIN)",
	}); err != nil {
		return fmt.Errorf("write install success: %w", err)
	}
	return nil
}
```

The `$(go env GOBIN)` detail is a documentation string only — it is never evaluated at runtime. Builder may use a static string like `"$GOBIN"` or `"go env GOBIN"` for the Detail if preferred.

**Acceptance:**

- `mage -l` output includes `install` in the target list.
- `mage install` runs without error on the dev machine (requires `GOBIN` or `GOPATH/bin` to be on `PATH`).
- After `mage install`, running `valv --help` from a terminal without `./` prefix finds the newly installed binary.
- `mage test` continues to pass after the `Install` function is added (format + test gate not affected by a new mage target).

### Notes

- **No first-run setup for Claude v1.** `runClaudeCommand` does not call `ensureClaudeBindingReady`. If the project is unbound, `service.ValidateBinding` returns `ErrUnboundProject` with the existing cobra error rendering. The interactive first-run flow is DROP_6 scope.
- **`sharedClaudeStateHome` must NOT be ported.** Codex has this helper to merge the real `~/.codex` into the runtime profile home when the profile is the default host profile. Claude's isolated-first model has no equivalent real host home to merge — the profile IS the state store. If a builder copies this blindly, the helper would call `claudeprovider.DefaultHostProfile(s.realHome)` which returns the default isolated path, not a useful shared state. The acceptance criterion for 5.2 explicitly calls this out.
- **Coverage floor is 70% per package** (AGENTS.md §11; note `magefile.go` sets `coverageThreshold = 60.0` currently — a TODO comment at line 22 says this will be restored to 70.0 after adapters/docker coverage is raised; plan to the AGENTS.md 70% standard for new packages; `mage testPkg` will enforce whatever the current threshold is at build time).
- **`claude_image.go` and `claude_image_test.go` already exist.** Unit 5.3 must NOT recreate them. Builder verifies these files are present before writing `claude.go` and confirms the test file imports are consistent.
- **`runGo` helper in magefile.** Builder must use the existing `runGo` helper (already used by `Build` at line 58); do not call `exec.Command("go", ...)` directly in the new `Install` target.
- **Import alias for Claude adapter in `manage/service.go`:** use `claudeprovider` as the alias (mirrors `codexprovider` at line 15 of that file) to maintain consistency.

### Hylla Feedback

Hylla was used for committed Go code discovery. Findings:

- **Query:** `hylla_search_keyword` on `DefaultHostProfile`, `sharedCodexStateHome`, `BindingByProjectID` — returned useful hits pointing to the correct files and line ranges. No misses on these queries.
- **Non-Go files** (PLAN.md scaffold, WORKFLOW.md, focus plan markdown, magefile.go) were read directly per protocol — not Hylla queries, so no misses to report for those.
- **`claude_image.go` discovery:** Hylla was not queried for this (it was found via `ls`); the file is new since the last Hylla ingest so Hylla would not have indexed it yet. This is expected behavior — `git diff` / `ls` is the correct fallback for post-ingest files.
- **Overall:** Hylla answered the Go-committed-code queries well. The main planning relied on direct reads because the scope of "copy-adapt" planning requires reading the full source of the template files, which Hylla summarizes but does not expose line-by-line as efficiently as `Read`.

## Notes

- **Mechanical drop — trimmed cascade applies** per memory `feedback_trimmed_cascade_for_mechanical_drops.md`. Single planner spawn, no parallel plan-QA proof+falsification subagent spawn. Orchestrator + dev review the planner output before approval. Per-unit build-QA (both proof and falsification) stays in place — that gate is universal.
- **Copy-adapt template.** The Codex adapter (`internal/adapters/providers/codex/{profile.go, account.go, runtime.go}`) is the structural template. The Claude versions are the same shape with these substitutions: `CODEX_HOME` → `CLAUDE_CONFIG_DIR`, `ContainerCodexDir = "/home/valv/.codex"` → `ContainerClaudeDir = "/home/valv/.claude"`, `codex` entrypoint → `claude` entrypoint, Codex JWT credential parsing → Claude `.credentials.json` presence check (per focus-plan §3.4). The Codex adapter's `bridge.go` (MCP host bridge) is intentionally NOT ported — Claude MCP translation is out of scope for v1.
- **`terminalEnvPassthrough` duplication.** Per focus-plan §3.2 v1 decision: duplicate `terminalEnvPassthrough` (and any other Codex-adapter helper the Claude adapter needs) into `internal/adapters/providers/claude/runtime.go`. Dedupe into a shared helper is DROP_9 cleanup.
- **`claude --resume` works for free.** Claude CLI persists session state under `$CLAUDE_CONFIG_DIR/projects/<hash>/`. Since `$CLAUDE_CONFIG_DIR=/home/valv/.claude` inside the container and `/home/valv/.claude` is bind-mounted to the Valv-managed account home on the host (`~/.valv/providers/claude/profiles/<account>/`), sessions persist across container restarts automatically. No special adapter code required beyond the standard bind-mount. Planner should NOT invent special `--resume` handling — verify by inspecting how Codex's runtime handles its own state dir (it doesn't; same pattern applies).
- **`mage install` sub-step.** Add an `Install` mage target to `magefile.go` that runs `go install ./cmd/valv` from the `main/` working dir. ~5 lines. The dev uses `mage install` post-drop-close to put `valv` on PATH for dogfooding. Builder should mirror the `Build` target's structure (currently `go build -o ./valv ./cmd/valv`).
- **First-launch flow for verification.** On first `valv claude` in a fresh bound project, device-code auth runs inside the container, writes credentials to the bind-mounted account home, subsequent launches reuse them. This is the dogfood acceptance test — `mage build`, bind a project to a fresh Claude account, run `valv claude`, complete device-code auth, verify a follow-up `valv claude --version` does not re-prompt. The dev runs this manually after CI is green per AGENTS.md §12.
- **Coverage floor.** 70% per package (AGENTS.md §11). Adapter and service packages are new, so plan test coverage up front. The existing test fixtures in `internal/adapters/providers/codex/` are the structural template for the Claude versions.
