# DROP_7 — HOST AUTH TOKEN FIX → ROUND 5 PIVOT: PATH B (IN-CONTAINER AUTH) + ALWAYS-LATEST

**State:** building
**Blocked by:** DROP_6 (done)
**Pivot (2026-05-15):** Rounds 1–4 (Path A, host-subprocess `claude setup-token`/`auth login` + macOS keychain extract + write to `<managed-home>/.credentials.json`) **failed end-to-end**: even verbatim keychain blob write does NOT authenticate container-side `claude`. Path A is rejected. **Round 5 = Path B + Always-Latest**, bundled per dev directive 2026-05-15.
**Round 5 paths (expected):** `internal/cli/claude_auth.go` (rewrite — delete host-extract, restore in-container plain `claude` auto-prompt), `internal/cli/claude_auth_test.go` (rewrite — in-container flow with fake docker executor), `internal/services/images/service.go` (edit — add `NewClaudeVersionResolver` mirroring `NewCodexVersionResolver` shape), `internal/services/images/service_test.go` (edit), launch path wiring for both providers (TBD by planner).
**Round 5 packages (expected):** `internal/cli`, `internal/services/images`, `internal/services/claude`, `internal/services/codex` (planner to confirm).
**PLAN.md ref:** main/PLAN.md → DROP_7_HOST_AUTH_TOKEN_FIX row
**Workflow:** main/drops/WORKFLOW.md
**Started:** 2026-05-15
**Round 5 started:** 2026-05-15
**Closed:** —

## Round 5 Scope (current — Path B + Always-Latest)

**Path B — Strategic revert to in-container auth (Anthropic's blessed pattern).** Per Anthropic's official devcontainer docs (https://code.claude.com/docs/en/devcontainer) and claudebox (https://github.com/RchGrav/claudebox)'s working implementation, the correct multi-account Docker pattern is: container starts with `<managed-home>` bind-mounted to `/home/USER/.claude`; container runs **plain `claude` (NO subcommand)**; claude auto-detects missing `.credentials.json` and prompts in-terminal; host browser opens via URL print; user pastes code; container `claude` writes `.credentials.json` natively to the bind-mounted dir. No host keychain extraction. No env-var injection. Per-Valv-account isolation = separate managed home per account, identical to DROP_5's existing design.

**Why Path A failed:** macOS keychain blob format and Linux container `.credentials.json` format are NOT 1:1 interchangeable. Verbatim copy from host keychain to `<managed-home>/.credentials.json` did NOT authenticate container-side claude (verified 2026-05-15 smoke test). The credentials handshake includes machine/session bindings that don't transfer cross-platform.

**Always-Latest version policy** (bundled per dev directive 2026-05-15): every `valv claude` and `valv codex` launch should use the latest provider CLI version unless dev explicitly pins. Currently only `valv manage update <provider>` runs the latest-check; launches use the pinned constant. Mirror Codex's existing `NewCodexVersionResolver` (queries GitHub Releases for `openai/codex`) with a new `NewClaudeVersionResolver` (queries `https://registry.npmjs.org/@anthropic-ai/claude-code/latest`). Wire both into launch path with a 24h cache to avoid per-launch latency. Dev can override via flag/env (TBD by planner).

**Round 5 design summary (planner will detail):**
- DELETE in `claude_auth.go`: host `claude` subprocess invocation, `runClaudeHostCommand`, `systemClaudeAccountAuthRunner.RunAuthLogin`, `ExtractKeychainToken`, `writeClaudeCredentials` verbatim-write logic, host `claude` PATH preflight, `security` shell-out, `claudeKeychainService` constant.
- RESTORE in-container auth: `ensureClaudeAccountReady` spins up `valv-claude:dev` container with the managed home bind-mounted (DROP_5 design already works) and runs **plain `claude`** (NO subcommand). Container auto-prompts; user completes OAuth in terminal; container claude writes `.credentials.json` natively; container exits.
- KEEP: Round 2's already-authed check (`os.Stat + Size > 0` → return nil — works correctly, verified 2026-05-15). DROP_5 bind-mount design (unchanged). DROP_6.3 `.claude.json` email parsing in `ReadAccountIdentity` (unchanged — host `claude` writes `.claude.json` independently of credentials handshake). R4 version pin bump to 2.1.143 (will be superseded by always-latest resolver but no harm).
- ADD: `NewClaudeVersionResolver` mirroring `NewCodexVersionResolver`. Wire latest-check into launch path for BOTH providers (Codex resolver currently only invoked by `EnsureLatest` from `manage update`). 24h cache. Override capability.

**Critical post-build smoke test:**

```bash
mage install
# Nuke claude/work state via cleanup SQL:
DB="$HOME/Library/Application Support/valv/db/valv.sqlite3"
sqlite3 "$DB" "DELETE FROM project_bindings WHERE provider='claude' AND profile_id IN (SELECT id FROM profiles WHERE provider='claude' AND name='work');"
sqlite3 "$DB" "DELETE FROM profiles WHERE provider='claude' AND name='work';"
rm -rf "$HOME/Library/Application Support/valv/providers/claude/profiles/work"
# DO NOT delete macOS keychain entry — that breaks the user's host Claude Code session.
valv account add claude work    # Expect: container starts, claude prompts in-terminal, browser opens via URL print, user pastes code, .credentials.json written to bind-mounted dir.
valv claude                     # Expect: container starts, reads .credentials.json natively, NO re-auth.
```

## Path A Status (Rounds 1–4, superseded 2026-05-15)

**Path A failed end-to-end despite all four rounds landing green CI + green QA.** The unit-level acceptance criteria were met (interface contracts, test coverage, file format) but the integration assumption (macOS keychain blob ≈ Linux container `.credentials.json`) was wrong. Code from Rounds 1–4 is on `origin/main`; Round 5 replaces it. Unit 7.1–7.4 definitions below are preserved as historical record — DO NOT use them as the spec for Round 5.

**Iteration recap:**
- Round 1: host `claude setup-token` + wrapped JSON `{"claudeAiAccessToken":"<token>"}` + `CLAUDE_CODE_OAUTH_TOKEN` env injection. Wrong scope (`user:inference` insufficient for interactive sessions), wrong file format.
- Round 2: already-authed early-return check in `ensureClaudeAccountReady`. **Works correctly — preserve in Round 5.**
- Round 3: pivoted to host `claude auth login` + verbatim keychain blob write + dropped env injection. Still re-auths in container. Wrong layer.
- Round 4: bumped `DefaultClaudeCLIVersion` 2.1.89 → 2.1.143. Did not fix auth bug (version was a red herring).

## Round 5 Planner — TO BE FILLED BY `go-planning-agent`

The `go-planning-agent` will append unit definitions for Round 5 below this header. Expected shape: 2–4 atomic units covering (a) Path B revert + in-container auth restore, (b) `NewClaudeVersionResolver` add, (c) launch-path wiring for always-latest on both providers. Planner grounds design in Anthropic devcontainer docs + claudebox source + Codex resolver template. Single planner pass per trimmed-cascade rule for copy-adapt drops.

<!-- Round 5 unit definitions land here -->

### Design decisions locked by planner (Round 5)

- **`claudeAuthRunner` interface — in-container, one method:**
  ```go
  type claudeAuthRunner interface {
      RunInContainer(ctx context.Context, homePath string, stdin io.Reader, stdout, stderr io.Writer) error
  }
  ```
  Replaces Path A's two-method interface (`RunAuthLogin` + `ExtractKeychainToken`). No host subprocess. No keychain. The single method spins up the `valv-claude:dev` container with `homePath` bind-mounted to `/home/valv/.claude`, runs plain `claude` (no subcommand) with `-it` if stdin is a TTY, waits for the container to exit.

- **`systemClaudeAccountAuthRunner` — embeds executor + image:**
  The production implementation holds an unexported `authContainerExecutor interface { Run(context.Context, docker.ContainerRunRequest) error }` (defined locally in `claude_auth.go` to avoid importing `claudeservice`) and a `docker.ImageRef`. Constructed at the `hostClaudeAccountAuth` package-level var using `dockeradapter.NewExecutor(dockeradapter.NewSystemRunner(...))` and `claudeImageRef()` — same callsite pattern as `claude.go`. The `RunInContainer` method constructs a `docker.ContainerRunRequest` with: bind mount `homePath → /home/valv/.claude`, env `CLAUDE_CONFIG_DIR=/home/valv/.claude`, `HOME=/home/valv`, `LOGNAME=valv`, `USER=valv`, `TERM=<normalized>`, entrypoint is the container default (`claude`), args empty (no subcommand). Sets `Interactive: stdin is not nil`, `TTY: stdin is a TTY` (check via `term.IsTerminal`), `Init: true`, `Remove: true`.

- **`ensureClaudeAccountReady` (Path B) — no wipe before container:**
  Order: (1) SkipLogin → return nil. (2) `os.Stat` creds file → if exists and `Size > 0` → return nil (already-authed fast path, kept from Round 2). (3) non-TTY guard: `!commandHasTTY(cmd.InOrStdin())` → return error with "TTY" mention. (4) `writeCLINotice` announce. (5) `runner.RunInContainer(...)` → if error → return. (6) `claudeprovider.ReadAccountIdentity(account.HomePath)` → if `!identity.LoggedIn` → return error. Return nil. **No wipe step** — container writes `.credentials.json` natively; there is nothing to wipe when we reach this path.

- **`loginClaudeAccount` (Path B) — same but no TTY guard:**
  Order: (1) `writeCLINotice`. (2) `runner.RunInContainer(...)`. (3) `ReadAccountIdentity` → verify `LoggedIn`. No TTY guard (same "no guard" semantics as `loginCodexAccount`).

- **`writeClaudeCredentials` and `claudeKeychainService` — DELETE both:**
  `writeClaudeCredentials` writes the host-extracted blob verbatim to disk — not needed in Path B. `claudeKeychainService` constant references the macOS keychain service — not needed. Remove both. Remove `encoding/json` import if no longer needed (it won't be).

- **`runClaudeHostCommand` — DELETE:**
  Was the host subprocess launcher. Not needed in Path B.

- **`ExtractKeychainToken` — DELETE:**
  Was keychain extraction. Not needed in Path B.

- **`RunAuthLogin` — DELETE:**
  Replaced by `RunInContainer`.

- **`os/user` import — DELETE:**
  Was used for `user.Current()` in keychain username lookup. Not needed in Path B.

- **`claude/service.go` state — CONFIRMED CLEAN:**
  Reading the file confirms `readClaudeAuthToken` does NOT exist and `buildRequest` does NOT set `CLAUDE_CODE_OAUTH_TOKEN`. Unit 7.2 (Path A) either was never applied or was reverted. No cleanup needed in the service layer. Path B relies on container claude reading `.credentials.json` natively from the bind-mounted managed home — `CLAUDE_CONFIG_DIR=/home/valv/.claude` is already set by `PrepareRuntime`. No env injection needed.

- **`NewClaudeVersionResolver` — queries npm registry:**
  ```go
  const defaultClaudeLatestURL = "https://registry.npmjs.org/@anthropic-ai/claude-code/latest"
  type claudeNPMPayload struct {
      Version string `json:"version"`
  }
  type claudeVersionResolver struct {
      client *http.Client
      url    string
  }
  func NewClaudeVersionResolver(client *http.Client) VersionResolver { ... }
  func (r claudeVersionResolver) LatestVersion(ctx context.Context) (string, error) { ... }
  ```
  Queries the URL, decodes `.version` field. Returns the version string (already semver, no `rust-v` prefix needed). `normalizeCodexVersion` can be reused if the result matches the semver pattern, or a simpler `strings.TrimPrefix(v, "v")` suffices.

- **`images.New()` — auto-wire for Claude provider:**
  Add case: `if resolver == nil && provider == domain.ProviderClaude { resolver = NewClaudeVersionResolver(nil) }`. Mirrors the existing Codex case. Placed immediately after the Codex case.

- **`runManageUpdateClaude` — switch to `EnsureLatest`:**
  Matches `runManageUpdateCodex` shape exactly. Uses `EnsureResult` not `BuildResult`. Output adds `checked at` field. Builder must verify no golden tests assert the old Claude update output format.

- **`ensureClaudeImageCurrent` — switch to `EnsureLatest`:**
  Replaces `service.Build(...)` with `service.EnsureLatest(cmd.Context(), imagesservice.EnsureRequest{AllowExistingOnCheckFail: true})`. Matches `ensureCodexImageCurrent` behavior exactly — offline/registry-down uses installed image.

- **Codex launch path — already wired:** `ensureCodexImageCurrent` already calls `EnsureLatest`. No Codex changes needed in Round 5.

- **`--version` flag and `VALV_CLAUDE_VERSION` env override — deferred to DROP_8.** The per-launch version pin is a CLI surface change out of scope for this pivot drop.

- **`VALV_CLAUDE_IMAGE` override — already handled:** `ensureClaudeImageCurrent` already checks `os.Getenv("VALV_CLAUDE_IMAGE")` before calling `EnsureLatest`. No change needed.

---

### Unit 7.5 — Rewrite `claude_auth.go`: restore in-container runner + rewrite tests

**state:** done
**blocked_by:** —
**paths:** `internal/cli/claude_auth.go`, `internal/cli/claude_auth_test.go`
**packages:** `internal/cli`

**What to build:**

**`claude_auth.go` — full rewrite.** Keep the file; replace its entire content.

Delete: `claudeKeychainService` constant, `RunAuthLogin` method, `ExtractKeychainToken` method, `runClaudeHostCommand` function, `writeClaudeCredentials` function. Remove `os/user` and `encoding/json` imports. Remove `bytes` import if no longer needed after deleting `runClaudeHostCommand`.

New `authContainerExecutor` interface (unexported, defined in this file):
```go
type authContainerExecutor interface {
    Run(context.Context, docker.ContainerRunRequest) error
}
```

New `claudeAuthRunner` interface:
```go
type claudeAuthRunner interface {
    RunInContainer(ctx context.Context, homePath string, stdin io.Reader, stdout, stderr io.Writer) error
}
```

New `systemClaudeAccountAuthRunner` struct:
```go
type systemClaudeAccountAuthRunner struct {
    executor authContainerExecutor
    image    docker.ImageRef
}
```

New `hostClaudeAccountAuth` var — initialized in an `init()` function or lazily, because it needs `dockeradapter.NewExecutor` and `claudeImageRef()`. The simplest pattern: make `hostClaudeAccountAuth` a `claudeAuthRunner` computed via a package-level function `defaultClaudeAuthRunner()` that constructs on first call, mirroring the pattern `claude.go` uses inline. Alternatively — and simpler — make `hostClaudeAccountAuth` a sentinel value and construct it inline in `claudeAuthRunnerFromContext` fallback. Builder chooses the simpler option; document in worklog.

`RunInContainer` implementation:
```go
func (r systemClaudeAccountAuthRunner) RunInContainer(ctx context.Context, homePath string, stdin io.Reader, stdout, stderr io.Writer) error {
    isTTY := stdin != nil && term.IsTerminal(os.Stdin.Fd()) // or pass a Fd — see note
    request := docker.ContainerRunRequest{
        Name:  fmt.Sprintf("valv-claude-auth-%d", time.Now().UTC().UnixNano()),
        Image: r.image,
        Env: map[string]string{
            "CLAUDE_CONFIG_DIR": claudeprovider.ContainerClaudeDir,
            "HOME":              claudeprovider.ContainerHomeDir,
            "LOGNAME":           "valv",
            "TERM":              normalizedContainerTERM(), // reuse from runtime.go? or inline
            "USER":              "valv",
        },
        Mounts: []docker.MountSpec{
            docker.NewMountSpec(homePath, claudeprovider.ContainerClaudeDir, false),
        },
        Args:        []string{},       // plain `claude` — no subcommand
        Interactive: stdin != nil,
        TTY:         isTTY,
        Init:        true,
        Remove:      true,
        User:        currentContainerUser(), // reuse from shared.go/utils
    }
    return r.executor.Run(ctx, request)
}
```
Note on TTY detection: use `commandHasTTY(stdin)` (already defined in the `cli` package) rather than raw `term.IsTerminal` — builder uses the package's existing TTY helper.

Note on `normalizedContainerTERM()`: defined in `internal/adapters/providers/claude/runtime.go`. The `cli` package does NOT import that function; inline equivalent: `strings.TrimSpace(os.Getenv("TERM"))` defaulting to `"xterm-256color"`. Builder inlines the logic.

`claudeAuthRunnerFromContext` — same pattern as before, reads context key, falls back to a constructed `systemClaudeAccountAuthRunner`. The fallback must be constructed lazily or via a `sync.Once` since it needs `claudeImageRef()` which is already defined in `claude.go` (same package). No `init()` needed — just construct inline at the fallback site.

`ensureClaudeAccountReady` — rewrite per design decisions:
1. `options.SkipLogin` → return nil.
2. Stat creds file (already-authed check from Round 2) — keep exactly as-is.
3. Non-TTY guard: `!commandHasTTY(cmd.InOrStdin())` → return error containing "TTY".
4. `writeCLINotice(...)` — keep.
5. `runner.RunInContainer(ctx, account.HomePath, cmd.InOrStdin(), cmd.OutOrStdout(), cmd.ErrOrStderr())` → error → return.
6. `claudeprovider.ReadAccountIdentity(account.HomePath)` → `!identity.LoggedIn` → return error.
7. Return nil.

`loginClaudeAccount` — rewrite per design decisions (no TTY guard):
1. `writeCLINotice(...)`.
2. `runner.RunInContainer(ctx, account.HomePath, cmd.InOrStdin(), cmd.OutOrStdout(), cmd.ErrOrStderr())` → error → return.
3. `ReadAccountIdentity` → `!identity.LoggedIn` → return error.
4. Return nil.

`wipeClaudeCredentials` — KEEP unchanged (still called from `logoutManagedAccount` via `logoutManagedAccount`→`wipeClaudeCredentials` switch case in `account_auth.go`).

**`claude_auth_test.go` — full rewrite.**

New `stubClaudeAccountAuthRunner`:
```go
type stubClaudeAccountAuthRunner struct {
    runErr   error
    runHits  int
    // for inspecting what was passed:
    lastHomePath string
}
func (s *stubClaudeAccountAuthRunner) RunInContainer(_ context.Context, homePath string, _, _, _ io.Writer) error {
    s.runHits++
    s.lastHomePath = homePath
    return s.runErr
}
```

New `installStubClaudeAuth(t, cmd, stub)` — inject via context, same pattern.

New `newTestClaudeCmd()` — keep same non-TTY bytes-buffer pattern.

Test cases:
- `TestEnsureClaudeAccountReadyRejectsNonTTY` — non-TTY cmd + no creds file → error containing "TTY"; `runHits == 0`.
- `TestEnsureClaudeAccountReadyRespectsSkipLogin` — SkipLogin=true → nil; `runHits == 0`; pre-existing creds preserved.
- `TestEnsureClaudeAccountReadyAlreadyAuthedReturnsNilEvenNonTTY` — creds file present + non-empty → nil; `runHits == 0`. File preserved.
- `TestEnsureClaudeAccountReadyFailsWhenContainerRunFails` — stub returns error → error returned; `runHits == 1`.
- `TestEnsureClaudeAccountReadyFailsWhenNotLoggedInAfterContainer` — stub returns nil but no `.credentials.json` written to temp dir → `ReadAccountIdentity` returns `LoggedIn=false` → error returned. (Stub does not write a creds file, so `ReadAccountIdentity` stat finds nothing.)
- `TestEnsureClaudeAccountReadySucceeds` — stub returns nil AND builder writes `.credentials.json` to the temp homePath in the stub's `RunInContainer` (to simulate container auth) → function returns nil. Note: the stub needs to write the file to simulate the container's behavior. Alternative: the stub simply creates the file as a side-effect. Builder implements `runHits`-based write in stub or uses a wrapping callback. **Simplest approach**: a `stubRunFunc func(homePath string)` field on the stub that tests can set; default is nil (no side effect). Tests that need the file written set `stub.stubRunFunc = func(hp string) { os.WriteFile(filepath.Join(hp, ".credentials.json"), []byte(`{"claudeAiAccessToken":"tok"}`), 0o600) }`.
- `TestLoginClaudeAccountSkipsNonTTYGuard` — non-TTY + stub returns error → error does NOT contain "TTY"; `runHits == 1`.
- `TestLoginClaudeAccountFailsWhenContainerRunFails` — stub returns error → propagated.
- `TestLoginClaudeAccountSucceeds` — stub writes creds file → `ReadAccountIdentity` returns `LoggedIn=true` → nil.
- `TestWipeClaudeCredentialsRemovesFile` — keep from Round 4 tests.
- `TestWipeClaudeCredentialsMissingFileIsNoError` — keep.
- `TestLogoutManagedAccountWipesClaudeCredentials` — keep.
- `TestReadAccountIdentityReturnsLoggedOutWhenNoCreds` — keep.

Old test names that existed for Path A (delete, do not port): `TestSystemClaudeAccountAuthRunnerRunAuthLoginUsesCLAUDE_CONFIG_DIR`, `TestRunClaudeHostCommandPreflightFailsWhenClaudeMissing`, `TestLoginClaudeAccountWipesAndRunsSetupTokenAndExtractsAndWrites`, `TestLoginClaudeAccountFailsWhenRunSetupTokenErrors`, `TestLoginClaudeAccountFailsWhenExtractTokenErrors`, `TestLoginClaudeAccountFailsOnEmptyToken`, `TestLoginClaudeAccountSkipsNonTTYGuard` (old), `TestEnsureClaudeAccountReadySkipsWhenAlreadyAuthed` (rename/keep as `TestEnsureClaudeAccountReadyAlreadyAuthedReturnsNilEvenNonTTY`).

**Imports for `claude_auth.go`:**
Keep: `context`, `fmt`, `io`, `os`, `path/filepath`, `strings`, `github.com/evanmschultz/laslig`, `github.com/spf13/cobra`, `claudeprovider`, `github.com/evanmschultz/valv/internal/config`, `github.com/evanmschultz/valv/internal/domain`, `dockeradapter "github.com/evanmschultz/valv/internal/adapters/docker"`, `"time"` (for container name timestamp).
Remove: `bytes`, `os/exec`, `os/user`, `encoding/json`.

**Acceptance criteria:**
- AC1: `claudeAuthRunner` interface has exactly one method: `RunInContainer`. No `RunAuthLogin`, no `ExtractKeychainToken`. Verified by inspection.
- AC2: `claude_auth.go` has no import of `os/user`, no import of `os/exec` (exec.LookPath gone), no `claudeKeychainService` constant, no `writeClaudeCredentials` function, no `runClaudeHostCommand` function.
- AC3: `ensureClaudeAccountReady` — SkipLogin returns nil with no container run. Non-TTY (no existing creds) returns error containing "TTY" with no container run. Already-authed (creds exist + non-empty) returns nil with no container run, even in non-TTY context.
- AC4: `ensureClaudeAccountReady` — container run failure propagates as error.
- AC5: `loginClaudeAccount` — no TTY guard; container run is invoked even in non-TTY context.
- AC6: `wipeClaudeCredentials` — unchanged behavior; still removes file and tolerates missing file.
- AC7: `mage testPkg github.com/evanmschultz/valv/internal/cli` passes with all new tests green and no old Path-A test names remaining.

**Verification target:** `mage testPkg github.com/evanmschultz/valv/internal/cli`

---

### Unit 7.7 — Add `NewClaudeVersionResolver` + wire in `images/service.go`

**state:** done
**blocked_by:** —
**paths:** `internal/services/images/service.go`, `internal/services/images/service_test.go`
**packages:** `internal/services/images`

**What to build:**

**`images/service.go` — add resolver type + constructor + auto-wire.**

New constant:
```go
const defaultClaudeLatestURL = "https://registry.npmjs.org/@anthropic-ai/claude-code/latest"
```

New payload type (unexported):
```go
type claudeNPMPayload struct {
    Version string `json:"version"`
}
```

New resolver type and constructor:
```go
type claudeVersionResolver struct {
    client *http.Client
    url    string
}

// NewClaudeVersionResolver returns a VersionResolver that queries the npm
// registry for the latest @anthropic-ai/claude-code version. If client is
// nil, a default client with a 10-second timeout is used.
func NewClaudeVersionResolver(client *http.Client) VersionResolver {
    if client == nil {
        client = &http.Client{Timeout: defaultVersionRequestTTL}
    }
    return claudeVersionResolver{client: client, url: defaultClaudeLatestURL}
}

func (r claudeVersionResolver) LatestVersion(ctx context.Context) (string, error) {
    req, err := http.NewRequestWithContext(ctx, http.MethodGet, r.url, nil)
    if err != nil {
        return "", fmt.Errorf("latest claude version: new request: %w", err)
    }
    req.Header.Set("Accept", "application/json")
    req.Header.Set("User-Agent", "valv")
    resp, err := r.client.Do(req)
    if err != nil {
        return "", fmt.Errorf("latest claude version: send request: %w", err)
    }
    defer resp.Body.Close()
    if resp.StatusCode != http.StatusOK {
        body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
        return "", fmt.Errorf("latest claude version: unexpected status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
    }
    var payload claudeNPMPayload
    if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
        return "", fmt.Errorf("latest claude version: decode response: %w", err)
    }
    version := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(payload.Version), "v"))
    if version == "" || !versionPattern.MatchString(version) {
        return "", fmt.Errorf("latest claude version: no valid version in npm response")
    }
    return versionPattern.FindString(version), nil
}
```

**`images.New()` — add Claude auto-wire** immediately after the existing Codex auto-wire:
```go
if resolver == nil && provider == domain.ProviderClaude {
    resolver = NewClaudeVersionResolver(nil)
}
```

No other changes to `New()`.

**`images/service_test.go` — add tests for `claudeVersionResolver`.**

New test `TestNewClaudeVersionResolverLatestVersion`:
- Start `httptest.NewServer` returning `{"version":"2.1.200"}` with status 200.
- Call `NewClaudeVersionResolver(server.Client())` with the test URL injected... builder discovers how to inject the URL. The `claudeVersionResolver` struct has a `url` field — tests can set it directly (same package). Mirror the existing Codex resolver test pattern (look for `TestNewCodexVersionResolverLatestVersion` or similar in service_test.go — builder reads the full test file to find the pattern).

Test cases:
- Happy path: server returns `{"version":"2.1.200"}` → resolver returns `"2.1.200"`.
- Non-200 status → resolver returns error.
- Bad JSON → resolver returns error.
- Empty version in payload → resolver returns error.
- Network error → resolver returns error.

**`images.New()` tests** — add:
- `TestNewWiresClaudeVersionResolverWhenProviderIsClaudeAndNilResolver`: construct with `Provider=ProviderClaude`, `Resolver=nil`. Verify `service.resolver != nil` by calling a method that uses it... or verify by type assertion. Simplest: call `service.EnsureLatest` with a `staticResolver` injected via `Resolver` field in a separate test — the auto-wire test just verifies `New()` succeeds with nil resolver for Claude provider.

**Imports added** (all already present in the file): none new — `net/http`, `encoding/json`, `io`, `strings`, `fmt` already imported.

**Acceptance criteria:**
- AC1: `NewClaudeVersionResolver(nil)` returns a non-nil `VersionResolver`. Verified by test.
- AC2: `claudeVersionResolver.LatestVersion` returns a valid semver string matching `\d+\.\d+\.\d+` when the server returns `{"version":"X.Y.Z"}`. Verified by httptest.
- AC3: `images.New()` with `Provider=ProviderClaude` and `Resolver=nil` returns a service with a non-nil resolver (does not panic on `EnsureLatest`). Verified by test.
- AC4: `mage testPkg github.com/evanmschultz/valv/internal/services/images` passes.

**Verification target:** `mage testPkg github.com/evanmschultz/valv/internal/services/images`

---

### Unit 7.8 — Wire always-latest into `ensureClaudeImageCurrent` + `runManageUpdateClaude`

**state:** done
**blocked_by:** 7.5, 7.7
**paths:** `internal/cli/claude.go`, `internal/cli/manage.go`
**packages:** `internal/cli`

**What to build:**

**`claude.go` — `ensureClaudeImageCurrent`: switch `Build` → `EnsureLatest`.**

Current code:
```go
// Claude uses pinned-version fast path: Build with DefaultClaudeCLIVersion,
// not EnsureLatest (which requires a resolver Claude does not have).
_, err = service.Build(cmd.Context(), imagesservice.BuildRequest{Version: imagesservice.DefaultClaudeCLIVersion})
```

Replace with:
```go
_, err = service.EnsureLatest(cmd.Context(), imagesservice.EnsureRequest{AllowExistingOnCheckFail: true})
```

Remove the stale comment. Update the variable name: `service.EnsureLatest` returns `(EnsureResult, error)` — use `_` since we don't need the result in this function.

The `openImagesService` call above this already passes `domain.ProviderClaude`, so the service is constructed with the Claude resolver auto-wired (per Unit 7.7). No other changes in `runClaudeCommand` or `runClaudeImageOnlyCommand`.

**`manage.go` — `runManageUpdateClaude`: switch `Build` → `EnsureLatest`.**

Current code calls `service.Build(...)` returning a `BuildResult`. New code calls `service.EnsureLatest(...)` returning an `EnsureResult`. Match the output shape of `runManageUpdateCodex` exactly — same fields, same field order. The key changes:
- Variable type changes from `imagesservice.BuildResult` to `imagesservice.EnsureResult`.
- The call inside the spinner changes to `service.EnsureLatest(cmd.Context(), imagesservice.EnsureRequest{})`.
- The output fields: add `{Label: "checked at", Value: result.LatestCheckedAt.Format(time.RFC3339), Muted: true}` between `version` and `context`.
- The heading logic: `heading := "Provider image built"` → add conditional: if `result.Action == imagesservice.EnsureActionUpToDate { heading = "Provider image up to date" }`.

Builder must verify: does any test or golden fixture assert on `runManageUpdateClaude` output format? If yes, update the fixture. If a golden file exists, run `mage goldenUpdate` after the change.

**No test changes needed** for this unit: the CLI command paths are integration-tested (if at all) via the integration test suite, and the unit behavior is covered by the images-service tests (Unit 7.7). The builder should run `mage testPkg github.com/evanmschultz/valv/internal/cli` to confirm no package-level compilation errors.

**Acceptance criteria:**
- AC1: `ensureClaudeImageCurrent` no longer references `imagesservice.BuildRequest` or `DefaultClaudeCLIVersion` in its non-`VALV_CLAUDE_IMAGE` path. Verified by inspection.
- AC2: `ensureClaudeImageCurrent` calls `service.EnsureLatest` with `AllowExistingOnCheckFail: true`. Verified by inspection.
- AC3: `runManageUpdateClaude` calls `service.EnsureLatest` (not `service.Build`). Output includes a `checked at` field. Verified by inspection.
- AC4: `mage testPkg github.com/evanmschultz/valv/internal/cli` passes (package compiles and existing tests pass).
- AC5: `mage test` passes (full suite, race detector, 70% per-package coverage floor).

**Verification target:** `mage testPkg github.com/evanmschultz/valv/internal/cli` then `mage test`

---

### Unit 7.9 — 24h on-disk version cache for `EnsureLatest`

**state:** done
**blocked_by:** 7.7, 7.8
**paths:** `internal/services/images/service.go`, `internal/services/images/service_test.go`, possibly new `internal/services/images/cache.go`
**packages:** `internal/services/images`

**Why this unit exists:** Unit 7.8 R1 falsification CONCERN 1 — Round 5 Scope explicitly promised "24h cache to avoid per-launch latency" but Unit 7.8 only shipped the wiring. Every `valv claude` / `valv codex` launch now makes a ~10s-timeout HTTPS round-trip to npm/GitHub. Dev directed "fix all 5 findings before smoke test" 2026-05-16.

**What to build:**

Add a disk-based version cache to `images.Service` so `EnsureLatest` skips the registry network call when a fresh cached version exists.

**Cache design:**
- Storage: `$XDG_CACHE_HOME/valv/version-cache.json` (fall back to `$HOME/.cache/valv/version-cache.json` if `XDG_CACHE_HOME` unset). Use Go stdlib `os.UserCacheDir()` — it handles the platform-correct path.
- Schema: `{"providers": {"claude": {"version": "2.1.143", "checked_at": "2026-05-16T18:00:00Z"}, "codex": {...}}}`. JSON-encoded, mode 0o644.
- TTL: 24h (`const versionCacheTTL = 24 * time.Hour`).
- API on `Service`: extend `EnsureLatest` to consult the cache before calling `resolver.LatestVersion`. If cached + fresh: return the cached version. If cached + stale or missing: call resolver, on success update cache, on failure honor `AllowExistingOnCheckFail`.
- Concurrency safety: file write should be `os.WriteFile` (full-file atomic at the kernel level for small files); for stricter safety use write-temp-then-rename pattern via `os.CreateTemp` + `os.Rename`. Reads are read-only — no locking needed.
- Failure isolation: cache read errors (file missing, malformed JSON, permission denied) MUST NOT block `EnsureLatest`. Log debug, proceed with normal resolver path.

**API touch points:**
- Constructor `New()`: accept an optional `CachePath string` field on `Options`. Empty → default to `os.UserCacheDir()`-based path. Tests can override to `t.TempDir()`.
- `EnsureLatest` body: before `resolver.LatestVersion`, attempt cache read keyed by provider. After successful resolver call, write back. Provider key derives from the `Provider` field passed to `New()` (`domain.ProviderClaude` / `domain.ProviderCodex` map to string keys `"claude"` / `"codex"`).
- Override capability for tests: a `clock func() time.Time` field on Service for deterministic `checked_at` timestamps; default `time.Now`.

**Tests:**
- Cache miss → resolver invoked → cache written.
- Cache hit fresh (within 24h) → resolver NOT invoked → cached version returned.
- Cache hit stale (older than 24h) → resolver invoked → cache updated.
- Cache read error (malformed JSON file) → resolver invoked, no error propagated.
- Cache write error (e.g. parent dir non-writable) → resolver still returns success, error logged.
- Both providers in one file: Claude write doesn't trample Codex entry.

**Plus SUB-FIX from NOTE 3 — resolve `DefaultClaudeCLIVersion` residue:** absorbed into this unit since it touches the same `service.go` file. After Unit 7.8, `DefaultClaudeCLIVersion` has zero production callers. Builder options (pick based on actual reference scan):
- (preferred) Delete `DefaultClaudeCLIVersion`. Update tests to use a fresh test constant (e.g. `const testClaudeCLIVersion = "2.1.143"` local to the test file) where needed. Delete `TestDefaultClaudeCLIVersionIsNonEmpty` if its only purpose was guarding the deleted constant. Update recipe-hash tests that reference it.
- (fallback) If deletion creates ripple effects (e.g. external integrations rely on the export), document why kept in a doc comment on the constant.

**Acceptance criteria:**
- AC1: `EnsureLatest` reads from `versionCachePath` before invoking resolver; returns cached version when within TTL. Verified by test injecting a fake clock + pre-populated cache file.
- AC2: After a fresh resolver call, the cache file contains `{provider: {version, checked_at}}` entry. Verified by test reading the file.
- AC3: Both Claude and Codex entries coexist in the same file. Verified by test writing a Codex entry then triggering a Claude `EnsureLatest`.
- AC4: Cache read errors do not propagate to `EnsureLatest` callers. Verified by test seeding malformed JSON.
- AC5: `DefaultClaudeCLIVersion` deletion (preferred) or kept-with-doc-comment (fallback). Builder picks and documents in worklog.
- AC6: `mage testPkg ./internal/services/images` GREEN, coverage ≥70%.
- AC7: `mage test` GREEN (full suite, race detector).

**Verification target:** `mage testPkg github.com/evanmschultz/valv/internal/services/images` then `mage test`

---

### Unit 7.10 — Polish: Codex parity for debug log, tests, heading wording

**state:** done
**blocked_by:** 7.7, 7.8
**paths:** `internal/cli/claude.go`, `internal/cli/manage.go`, `internal/cli/manage_test.go`, possibly `internal/cli/extended_test.go`
**packages:** `internal/cli`

**Why this unit exists:** Unit 7.8 R1 falsification NOTE 1, NOTE 2, NOTE 4 — surface-level Codex-parity gaps in `internal/cli` that the dev requested be addressed before smoke test 2026-05-16. NOTE 3 (`DefaultClaudeCLIVersion` cleanup) moved to Unit 7.9 to keep package boundaries disjoint.

**What to build (three sub-fixes):**

**SUB-FIX A — NOTE 1: Capture `EnsureResult` in `ensureClaudeImageCurrent` for debug log.**
Match `ensureCodexImageCurrent` (`internal/cli/codex.go:248–254`). Currently `ensureClaudeImageCurrent` (`claude.go:190`) discards the result with `_`. Capture it, and on `result.Action == imagesservice.EnsureActionUsingExistingImage` emit a debug log mirroring Codex's exact log key/format. Use the same logger reference Codex uses.

**SUB-FIX B — NOTE 2: Add Claude-equivalent tests.**
Codex has `TestManageUpdateSecondRunReportsUpToDate` (or similarly named — find via grep) and `TestEnsureCodexImageCurrentAutoUpdatesWhenNoOverrideIsSet`. Add Claude mirrors:
- `TestManageUpdateClaudeSecondRunReportsUpToDate` — invoke `runManageUpdateClaude` twice, second invocation should report up-to-date heading.
- `TestEnsureClaudeImageCurrentAutoUpdatesWhenNoOverrideIsSet` — confirms `ensureClaudeImageCurrent` fires `EnsureLatest` (not a no-op or pinned `Build`) when `VALV_CLAUDE_IMAGE` is unset.

**SUB-FIX D — NOTE 4: Heading wording symmetry.**
Codex default `manage update` heading is `"Provider image updated"` (`manage.go:1140`). Claude default is `"Provider image built"` (`manage.go:1172`). Change Claude's default to `"Provider image updated"` to match. Update any test that asserts the old wording.

**Acceptance criteria:**
- AC1: `ensureClaudeImageCurrent` emits a debug log on `EnsureActionUsingExistingImage` matching Codex's pattern. Verified by test (capture debug logger output or inspect via mock).
- AC2: `TestManageUpdateClaudeSecondRunReportsUpToDate` and `TestEnsureClaudeImageCurrentAutoUpdatesWhenNoOverrideIsSet` exist and pass.
- AC3: Claude's `runManageUpdateClaude` default heading is `"Provider image updated"`. Old-wording test assertions updated.
- AC4: `mage testPkg ./internal/cli` GREEN. Coverage ≥70%.
- AC5: `mage test` GREEN full suite.

**Verification target:** `mage testPkg github.com/evanmschultz/valv/internal/cli` then `mage test`

**Coordination with Unit 7.9 (parallel-safe):** Unit 7.9 owns `internal/services/images/`. Unit 7.10 owns `internal/cli/`. Zero file overlap. Both BUILDER_WORKLOG.md appends go to the same file; distinct `## Unit N.M — Round 1` headings prevent collision.

---

### Unit 7.9 — Round 2 scope (added 2026-05-16)

R1 QA proof FAIL + R1 QA falsification PASS with concerns. Dev approved bundled R2 fixes. Builder appends `## Unit 7.9 — Round 2` to BUILDER_WORKLOG.md. State flips done → in_progress → done.

**Required fixes:**

1. **Integration-test reference (proof Finding 1.1 + falsification A8):** `internal/services/images/service_integration_test.go:127` still references the deleted `DefaultClaudeCLIVersion`. The file is build-tagged `//go:build integration` so no current mage target compiles it, but the source is broken. **Fix:** mirror `service_test.go:516` — define a local `const testClaudeCLIVersion = "2.1.143"` at the top of `service_integration_test.go` (or in the same test fn) and use it in place of `imagesservice.DefaultClaudeCLIVersion`.

2. **`LatestCheckedAt` semantic (proof Finding 1.2 + falsification A4):** `service.go:411` unconditionally sets `EnsureResult.LatestCheckedAt = clock().UTC()` regardless of whether the version came from cache or resolver. **Fix:** on cache hit, return the CACHED entry's `CheckedAt` as `LatestCheckedAt` (option a, dev-approved 2026-05-16). On cache miss + resolver-success, keep current behavior (return `clock().UTC()`). Add a new test `TestEnsureLatestReportsCachedCheckedAtOnCacheHit` that pre-populates a cache entry with `checked_at = "2025-01-01T00:00:00Z"`, sets clock to `2025-01-01T12:00:00Z` (12h later, within TTL), calls `EnsureLatest`, asserts `result.LatestCheckedAt.Equal(2025-01-01T00:00:00Z)`.

3. **Clock-injection completeness (falsification A17):** `service.go:429` and `service.go:479` still call `time.Now()` directly for `state.UpdatedAt` inside `EnsureLatest`, bypassing the injected `s.clock`. **Fix:** replace both with `s.clock()`. This makes the test fixture's deterministic-clock injection actually deterministic for all `EnsureLatest` paths, not just the cache check.

4. **Future-timestamp guard (falsification A5):** `versionCacheTTL` check is `clock() - checkedAt < TTL`. If `checkedAt` is in the future (clock skew, manual edit), the delta is negative, less than TTL, treats as fresh — cache permanently stuck. **Fix:** in `cachedVersion` (or wherever the TTL check happens), reject cache entries with `delta < 0` (cache miss). Add a test `TestEnsureLatestRejectsCacheWithFutureTimestamp`.

5. **Worklog correction (falsification A2):** Builder's R1 worklog note about `os.WriteFile` being "POSIX kernel-level atomic" is factually wrong. `os.WriteFile` truncates-then-writes — a reader can catch a partial file. The current implementation is acceptable because `readVersionCache` swallows all parse errors, but the JUSTIFICATION in the worklog is wrong. **Fix:** correct the worklog note in the R2 entry, optionally upgrade to `os.CreateTemp` + `os.Rename` pattern (defensive — choose based on iteration cost vs current safety mitigation). Either choice is acceptable; document the decision.

**Acceptance criteria (R2):**

- AC1: `service_integration_test.go:127` no longer references `DefaultClaudeCLIVersion`. Verified by inspection + `grep -rn DefaultClaudeCLIVersion main/` returning zero matches.
- AC2: `LatestCheckedAt` semantic test added and passes; cache-hit path returns cached `CheckedAt` not `now`.
- AC3: `service.go:429, :479` use `s.clock()` not `time.Now()`. Verified by inspection.
- AC4: Future-timestamp cache guard implemented + test added.
- AC5: Worklog corrects atomic-write justification.
- AC6: All R1 ACs still pass.
- AC7: `mage testPkg ./internal/services/images` GREEN, coverage ≥70%.
- AC8: `mage test` GREEN if disk-space environmental flake doesn't recur (orthogonal to this unit).

**Verification target:** `mage testPkg github.com/evanmschultz/valv/internal/services/images` then `mage test`.

---

### Unit 7.10 — Round 2 scope (added 2026-05-16)

R1 QA proof PASS + R1 QA falsification PASS with one CONFIRMED counterexample (A8) downgraded to CONCERN. Dev approved R2 fixes + bundled tmpfs-test fix. Builder appends `## Unit 7.10 — Round 2` to BUILDER_WORKLOG.md. State flips done → in_progress → done.

**Required fixes:**

1. **Cache-path isolation (falsification A8 — CRITICAL):** `internal/cli/operator_helpers.go::openImagesService` does NOT thread `paths.CachesDir` into `imagesservice.Options.CachePath`. Result: tests + production both use the default `os.UserCacheDir()` path. QA runs polluted the dev's real `~/Library/Caches/valv/version-cache.json` with stubbed test values. Production impact: next real `valv manage update claude` cache-hits at the stubbed version for 24h. **Fix:** in `openImagesService`, set `Options.CachePath = filepath.Join(paths.CachesDir, "version-cache.json")` (or whatever the canonical valv cache subpath is — `paths.CachesDir` is the per-invocation cache root, `t.TempDir()`-based in tests). Add a test confirming `openImagesService(..., paths)` returns a service whose `CachePath` resolves under `paths.CachesDir`.

2. **Tmpfs disk-space test (`TestRootDebugFlagIsNotPassedThroughToInteractiveCodexLaunch`):** Located in `internal/cli/codex_test.go` (~line 428 per QA findings). The test stages the dev's real `~/.codex` to tmpfs and fails when tmpfs lacks space. Reproduced 427/428 by 2 of 3 QA mage runs; one run was 428/428 (intermittent). **Fix:** investigate what payload is being staged. Options to consider, in order of preference: (a) the test SHOULD use a minimal `t.TempDir()`-based fake CODEX_HOME rather than copying the dev's real `~/.codex` — confirm and fix; (b) if the real-home stage is intentional (e.g., golden-fixture testing), add `t.Skip` when the staging dir's `Statfs` reports insufficient free space; (c) shrink the staged payload. Builder reads the test in full to pick the right approach. Document the choice in the worklog.

**Acceptance criteria (R2):**

- AC1: `openImagesService` threads `paths.CachesDir` into `Options.CachePath`. Verified by inspection + new test.
- AC2: Tests no longer pollute the dev's `~/Library/Caches/valv/version-cache.json` on `mage test` run. Verified by running `mage test` from a clean state and confirming no real cache mutation.
- AC3: `TestRootDebugFlagIsNotPassedThroughToInteractiveCodexLaunch` either runs cleanly on a constrained-tmpfs dev box OR skips correctly with `t.Skip` + reason message. Verified by `mage test` GREEN 428/428 across 3 consecutive runs.
- AC4: All R1 ACs still pass.
- AC5: `mage testPkg ./internal/cli` GREEN, coverage ≥70%.
- AC6: `mage test` GREEN 428/428.

**Verification target:** `mage testPkg ./internal/cli` then `mage test` (×3 for AC3 reliability).

**Coordination with Unit 7.9 R2 (parallel-safe):** Same package-boundary discipline as R1 — 7.9 R2 owns `internal/services/images/`, 7.10 R2 owns `internal/cli/`. Zero file overlap. Append distinct `## Unit N.M — Round 2` headings to BUILDER_WORKLOG.md.

---

### Unit 7.11 — Auth UX polish: host browser auto-open + clean exit on creds-write

**state:** done
**blocked_by:** 7.5 R2 (done) — depends on the `RunInContainer` orchestration code
**paths:** `internal/cli/claude_auth.go`, `internal/cli/claude_auth_test.go`, possibly a small new helper file in `internal/cli/` (planner's choice)
**packages:** `internal/cli`
**Round 5 scope addition (added 2026-05-16):** UX polish identified during dogfood smoke test. Auth flow works end-to-end; two ergonomic gaps remain.

**Orchestrator-locked scope (planner decomposes):**

Two bundled UX fixes inside the auth-container orchestration in `claude_auth.go::RunInContainer`. Bundled (not split into two units) because both modify the same `RunInContainer` flow — file overlap forces serial work either way; bundling saves one build-QA cycle.

**FIX A — Host browser auto-open on OAuth URL detection.**

- Wrap the auth container's stdout with an `io.TeeReader` (or equivalent multiplexed pipe) that:
  - Continues to forward output to the user's terminal (so claude's `Browser didn't open? Use the URL below` + `Paste code here` prompts still render unchanged).
  - Scans incoming bytes for OAuth URL patterns: `https://claude.com/cai/oauth/authorize` (subscription path) and `https://platform.claude.com/oauth/authorize` (Console path).
  - On first match per session, asynchronously fires `exec.Command("open", url).Start()` (macOS host) to launch the dev's default browser.
- macOS-only for v0.1.0 per AGENTS.md macOS+Docker scope. Add a TODO comment for cross-platform (`xdg-open` Linux, `cmd /c start` Windows).
- No retry on `open` failure — claude's URL-fallback prompt is still visible, so user has manual recourse. YAGNI hard.

**FIX B — `.credentials.json` watcher → SIGTERM container on completion.**

- After spawning the container, start a goroutine that polls `filepath.Join(homePath, ".credentials.json")` every ~500ms for existence + non-zero size.
- When detected, send SIGTERM to the container subprocess. Container exits cleanly. `--rm` removes it.
- Function returns nil. User sees a one-line success notice (`writeCLINotice`).
- No manual Ctrl-C needed. (Until this fix lands, users must Ctrl-C twice after auth completes — claude TUI doesn't auto-exit; this is documented in §"Ctrl-C UX interim guidance" below.)
- Polling cadence: 500ms is fast enough for human-paced OAuth (10-30s typical) and slow enough to be negligible CPU.
- `fsnotify` is over-engineering for this single file watch; polling is simpler with no new deps.

**Cancellation + cleanup correctness:**

- Polling goroutine must exit cleanly in three cases:
  1. Creds file appears → SIGTERM container → done.
  2. Container exits on its own (user Ctrl-C'd before creds appeared) → goroutine sees `os.IsNotExist` indefinitely; must NOT leak. Use a `context.Context` derived from the run; cancel when container subprocess exits.
  3. Build-side test injection: pollable via interface or `Clock` injection so tests don't actually `time.Sleep`.

**Tests:**

- `TestRunInContainerOpensBrowserOnURLDetect` — feed a fake stdout with the OAuth URL; assert a fake `opener` got called once with the URL.
- `TestRunInContainerDoesNotOpenWhenNoURL` — feed unrelated stdout; assert opener NOT called.
- `TestRunInContainerSigtermsOnCredsWrite` — fake docker executor; write `.credentials.json` after a short delay in the test; assert SIGTERM was sent and function returned nil.
- `TestRunInContainerSurvivesContainerExitBeforeCreds` — fake docker executor returns immediately (user Ctrl-C'd); assert poller goroutine doesn't leak; assert function returns appropriate error.
- `TestRunInContainerCancelsPollerOnContextCancel` — context-cancellation hygiene.

**Interface shape (planner refines):**

Likely introduce two unexported interfaces in `claude_auth.go`:
- `urlOpener interface { Open(ctx context.Context, url string) error }` — production: `exec.Command("open", url).Start()`.
- `credsWatcher interface { WaitForCreds(ctx context.Context, path string) error }` — production: poll loop.

Inject via the existing `claudeAuthRunner` interface or extended runner type. Tests provide stubs.

**Acceptance criteria:**

- AC1: `RunInContainer` detects OAuth URL in container stdout and fires `urlOpener.Open(url)` exactly once. Verified by test.
- AC2: `RunInContainer` polls for `<homePath>/.credentials.json`; on appearance, SIGTERMs the container. Verified by test.
- AC3: User-terminal output is unaffected — stdout still flows through. Verified by test capturing terminal-side bytes.
- AC4: Poller goroutine does not leak when container exits before creds appear. Verified by test + `go test -race`.
- AC5: `mage testPkg github.com/evanmschultz/valv/internal/cli` GREEN, coverage ≥70%.
- AC6: `mage test` GREEN full suite.

**Verification target:** `mage testPkg github.com/evanmschultz/valv/internal/cli` then `mage test`.

**Coordination:** Unit 7.11 owns `internal/cli/`. No parallel unit currently in flight in the same package.

---

### Design decisions locked by planner (Unit 7.11)

Added 2026-05-16. Builder must NOT relitigate these decisions; raise concerns as a worklog note if a decision proves wrong during implementation.

---

#### D1 — Stdout pipe ownership: wrap inside `RunInContainer`

**Decision:** Wrap the `stdout` and `stderr` writers INSIDE `RunInContainer`, before passing them to `NewSystemRunner`. Specifically, construct a `lineScanner` writer that tees bytes to the original terminal writer AND scans complete lines for the OAuth URL regex. Pass the `lineScanner` as `stdout` (and a parallel instance for `stderr`) to `NewSystemRunner`.

**Rationale:** `RunInContainer` already constructs `NewSystemRunner` inline. Wrapping at that call site requires no changes to the docker adapter. The auth-specific logic stays fully contained in `claude_auth.go`. Option (b) — adding a field to `ContainerRunRequest` — would contaminate the shared adapter with auth-domain concerns.

**Critical refinement (from QA Falsification attack 4 + attack 8):** Wrap BOTH the `stdout` AND `stderr` writers with the same `sync.Once`-guarded URL scanner, because claude may emit the OAuth URL on either stream depending on TTY mode. The line scanner must buffer incomplete lines across `Write` calls and only scan on newline flush — URL may straddle two write chunks and per-write regex scanning would miss it.

---

#### D2 — URL regex pattern

**Decision (locked):**

```go
var oauthURLRegex = regexp.MustCompile(
    `https://(?:claude\.com/cai|platform\.claude\.com)/oauth/authorize\S*`,
)
```

`\S*` captures the rest of the non-whitespace URL token (query params, fragments) without consuming newlines or trailing whitespace.

**Builder verification required:** After the first real auth run, check the worklog and confirm the exact URL emitted by the container matches this pattern. If the URL format differs (e.g. different subdomain), update the regex and document in worklog.

---

#### D3 — Once-per-session URL-open semantics

**Decision:** Use `sync.Once` to guarantee `urlOpener.Open` is called at most once per `RunInContainer` invocation. The `once.Do` wraps the open call inside the scanning writer's line-match handler.

---

#### D4 — `urlOpener` interface + injection

**Decision:** Define:

```go
type urlOpener interface {
    Open(ctx context.Context, url string) error
}
```

Add a `urlOpener urlOpener` field to `systemClaudeAccountAuthRunner` (unexported field name, unexported type). Production nil-guard: if `r.urlOpener == nil`, use `defaultURLOpener{}` which calls `exec.Command("open", url).Start()` (non-blocking — `Start`, not `Run`, so it returns immediately after OS hands off to the default browser handler). Add macOS-only `// TODO: xdg-open (Linux), cmd /c start (Windows)` comment on `defaultURLOpener`.

No retry on `open` failure. If `open` errors, swallow silently — claude's printed URL is the user's fallback.

**Test injection:** Tests set `systemClaudeAccountAuthRunner{executor: stub, urlOpener: &stubURLOpener{}, credsWatcher: &stubCredsWatcher{}}`.

---

#### D5 — `credsWatcher` interface + injection

**Decision:** Define:

```go
type credsWatcher interface {
    WaitForCreds(ctx context.Context, path string) error
}
```

Add a `credsWatcher credsWatcher` field to `systemClaudeAccountAuthRunner`. Production nil-guard: if `r.credsWatcher == nil`, use `defaultCredsWatcher{}`.

`defaultCredsWatcher.WaitForCreds` implementation:
- `ticker := time.NewTicker(500 * time.Millisecond)` + `defer ticker.Stop()`
- `select { case <-ticker.C: check file; case <-ctx.Done(): return ctx.Err() }`
- File check: `os.Stat(path)` → if no error and `info.Size() > 0` → return nil (creds present). Otherwise continue.

---

#### D6 — SIGTERM delivery via `docker stop`

**Decision:** The goroutine fires `exec.Command("docker", "stop", "--time", "5", containerName).Run()` when creds are detected.

**Rationale:** The docker `Executor.Run` wraps `cmd.Run()` synchronously and does not expose `cmd.Process`. Adding a process handle to the adapter is invasive. Using `docker stop <name>` is a clean CLI call — Valv already owns the container name before calling `exec.Run`, so the goroutine has it via closure. This keeps the adapter unchanged.

**Error handling:** If `docker stop` returns a non-zero exit (e.g. container already gone after user Ctrl-C before goroutine fires), swallow the error — `exec.Run` will return its own result independently.

---

#### D7 — Goroutine lifecycle and cancellation

**Decision:**

```
ctx, cancel := context.WithCancel(parentCtx)
defer cancel()
```

at top of `RunInContainer`. The creds-watcher goroutine receives this `ctx`. When `RunInContainer` returns — for any reason — `defer cancel()` fires and the goroutine's `WaitForCreds` returns `ctx.Err()` on the next tick check. No goroutine leak in any of the three cases:

1. Creds appear → goroutine calls `docker stop` → goroutine returns → `exec.Run` returns → `defer cancel()` fires (goroutine already gone).
2. Container exits independently → `exec.Run` returns → `defer cancel()` fires → goroutine exits on next `ctx.Done()` check within ≤500ms.
3. Parent context cancelled → `exec.Run` returns (context cancellation propagates to docker run) → `defer cancel()` fires → goroutine exits.

---

#### D8 — Concurrency: URL once-guard and creds notification

**Decision:**

- URL detection uses `sync.Once` (goroutine-safe, correct for the one-shot fire).
- Creds notification to the main goroutine: NOT a channel. The goroutine fires `docker stop` directly and sets an `atomic.Bool` (`credDetected.Store(true)`) before calling `docker stop`. After `exec.Run` returns, `RunInContainer` reads `credDetected.Load()` to decide whether to emit the success notice and return nil vs. propagate the run error.
- No shared mutable state other than the `sync.Once` and the `atomic.Bool`. Both are safe.

---

#### D9 — Error semantics on `credsWatcher.WaitForCreds` failure

**Decision:** If `WaitForCreds` returns a non-nil error that is NOT `context.Canceled` / `context.DeadlineExceeded`:
- Log at debug level: `"creds watcher error, continuing without auto-exit"`.
- Do NOT call `docker stop`.
- Let the container run normally. User retains the Ctrl-C fallback.
- `RunInContainer` returns whatever `exec.Run` returns.

Context cancellation errors (`ctx.Err()`) are normal — they indicate container exited or parent cancelled; swallow silently.

---

#### D10 — Test injection seam and test name refinements

**Decision:** All three injectable fields (`executor`, `urlOpener`, `credsWatcher`) are fields on `systemClaudeAccountAuthRunner`. Tests construct the struct directly with all three set.

**Test stubs (new, not yet in tree):**

```go
type stubURLOpener struct {
    openedURLs []string
    err        error
}
func (s *stubURLOpener) Open(_ context.Context, url string) error {
    s.openedURLs = append(s.openedURLs, url)
    return s.err
}

type stubCredsWatcher struct {
    err       error
    callCount int
}
func (s *stubCredsWatcher) WaitForCreds(_ context.Context, _ string) error {
    s.callCount++
    return s.err
}
```

**Revised test names (builder writes these, all new — not yet in tree):**

- `TestRunInContainerOpensBrowserOnURLDetect` — stub `urlOpener`; feed a fake stdout writer (via `stubAuthContainerExecutor` that writes the OAuth URL into the `stdout` writer it receives); assert `stub.openedURLs` contains the URL. Also wrap stderr to confirm both streams are scanned.
- `TestRunInContainerDoesNotOpenWhenNoURL` — feed unrelated stdout via stub executor; assert `stub.openedURLs` is empty.
- `TestRunInContainerSigtermsOnCredsWrite` — stub `credsWatcher` returns nil immediately (creds "found"); stub executor records the `ContainerRunRequest.Name`; verify `RunInContainer` returns nil and success notice was written to stderr (check stderr buffer).
- `TestRunInContainerSurvivesContainerExitBeforeCreds` — stub executor returns immediately; stub `credsWatcher` returns `ctx.Err()` (simulating context cancel); verify `RunInContainer` does NOT return nil (no creds written → `exec.Run` returned with container's exit error or nil, then identity check fails → but wait, the identity check lives in `ensureClaudeAccountReady` / `loginClaudeAccount`, not in `RunInContainer` itself). Clarification: `RunInContainer` itself returns what `exec.Run` returns. The test verifies the goroutine doesn't leak (test completes promptly, `-race` clean).
- `TestRunInContainerCancelsPollerOnContextCancel` — cancel the context; verify `stubCredsWatcher.callCount` stops incrementing and `RunInContainer` returns promptly.

**Builder note:** `TestRunInContainerSigtermsOnCredsWrite` cannot verify the actual `docker stop` subprocess (that would require a real Docker daemon). It verifies the observable effect: `RunInContainer` returns nil after `stubCredsWatcher` returns nil, and `credDetected` path emits the success notice. The `docker stop` call is the production behavior; in tests `stubCredsWatcher` returning nil is sufficient to trigger the post-detection path.

---

#### Additional implementation note: success notice and nil-return semantics

After `exec.Run` returns: if `credDetected.Load() == true`, treat the run as a success regardless of `exec.Run`'s error value (docker may report a non-zero exit from SIGTERM even on clean stop). Call `writeCLINotice(stderr, laslig.NoticeInfoLevel, "Claude auth complete", "Browser authentication complete. Credentials saved.")` and return nil. If `credDetected.Load() == false`, propagate `exec.Run`'s error normally.

**Open question for builder to resolve:** Does `docker run --rm` exit with code 0 or non-zero when the container is stopped via `docker stop`? If non-zero, the `credDetected` guard is required. Builder must verify via worklog note from a real run.

---

### Unit 7.11 — Round 2 scope (added 2026-05-16)

R1 QA falsification returned `fail` with one BLOCK + three CONCERNs. Dev directed pause to do CLI surface audit (now captured as DROP_9). R2 finishes Unit 7.11 — close DROP_7 cannot proceed without it. Builder appends `## Unit 7.11 — Round 2` to BUILDER_WORKLOG.md. Flip `state: R1 done; R2 required` → `in_progress` at R2 start; → `done` at R2 close.

**Three fixes (dev-approved bundle 2026-05-16):**

**FIX 1 — BLOCK 1: `loginClaudeAccount` re-login regression.**

Pre-existing `.credentials.json` causes the new creds-watcher's first 500ms tick to short-circuit the re-auth: `docker stop` fires before claude can write fresh creds, function returns nil + "Claude auth complete" notice, but credentials are STALE. Affects `valv manage account login claude <name>` on already-authed accounts (and post-DROP_9, the renamed equivalent path).

**Fix:** add `if err := wipeClaudeCredentials(account.HomePath); err != nil { return err }` at the start of `loginClaudeAccount` (`claude_auth.go` — currently around the function's existing body start, after `writeCLINotice`). This restores force-fresh semantics. The auth-container then starts with an empty `.credentials.json` slot, watcher correctly waits for the new file. `ensureClaudeAccountReady` already has the already-authed early-return so its semantics are unchanged.

**Test:** new `TestLoginClaudeAccountWipesExistingCredsBeforeRunning` — pre-write `.credentials.json` with `"stale"`, call `loginClaudeAccount` with a stub executor that returns immediately, stub watcher that detects new creds after a brief delay; assert pre-existing creds were wiped (file content is the new write, not `"stale"`).

**FIX 2 — CONCERN 2: ANSI escape sequences captured in OAuth URL.**

R1 regex is `https://(?:claude\.com/cai|platform\.claude\.com)/oauth/authorize\S*`. The `\S*` non-whitespace-greedy match consumes ANSI escape bytes (`\x1b[0m` etc.) when claude's TUI styles the URL line. `urlOpener.Open` then receives a URL with literal ESC bytes; `exec.Command("open", url).Start()` silently fails; `sync.Once` permanently latches.

**Fix (option a — preferred):** tighten the regex character class to URL-valid characters only. New pattern: `https://(?:claude\.com/cai|platform\.claude\.com)/oauth/authorize[A-Za-z0-9._~:/?#\[\]@!$&'()*+,;=%-]*`. Same OAuth-URL coverage, no ANSI ingest.

**Fix (option b — fallback if regex change is fragile):** strip ANSI before regex match using a simple `ansiStripper` step in the `lineScanner` pipeline.

Builder picks option (a) unless real claude output trips a corner case the test misses. Document the choice.

**Test:** new `TestLineScannerStripsANSIFromOAuthURL` — feed `"https://claude.com/cai/oauth/authorize?code=foo\x1b[0m\n"` to the lineScanner; assert `urlOpener.Open` was called with a URL that does NOT contain `\x1b`.

**FIX 3 — CONCERN 3 (dev-locked 2026-05-16 to option a): OAuth URL split by terminal line-wrap.**

Long OAuth URLs wrapped at terminal width are split by `\n` mid-URL. R1 regex matches only the prefix → broken URL → `sync.Once` latches → broken `open` call. Same class of bug as the Anthropic `Unknown scope` line-wrap issue we already hit.

**Locked approach (option a — full buffering + test):** buffer multiple lines in `lineScanner`; on every newline-terminated chunk, also try matching the regex across the accumulated buffer with whitespace stripped. ~30 LOC. Adds buffered-state to the scanner.

**Implementation notes:**
- Maintain a `urlBuffer strings.Builder` field on `lineScanner` (or equivalent), populated on every `Write`.
- After each `\n`-terminated emission, run the tightened FIX-2 regex against `strings.Map(stripWhitespace, urlBuffer.String())` to detect a URL that crossed a line boundary.
- On match → `sync.Once.Do(urlOpener.Open(joined))` as before. Whitespace stripping is the join operation (terminal wrap inserts `\n` but no other URL-breaking characters mid-URL).
- Cap buffer growth at a reasonable bound (e.g. 4 KiB) and discard once a non-URL line has been seen long enough — keeps memory bounded if claude never prints an OAuth URL.

**Test:** new `TestLineScannerDetectsURLAcrossMultipleLines` — feed `"https://claude.com/cai/oauth/authorize?code=foo\nbar&baz=qux\n"` across two `Write` calls; assert `urlOpener.Open` was called with the full joined URL.

**Optional addition from QA falsification N12:** add an AC requiring the dev-smoke-test confirmation as a pre-Phase-7 gate, encoded in DROP_7 PLAN.md. Captured here as: AC4-extra. Builder may add it inline if light; otherwise it's an orchestrator-managed Phase-6 → Phase-7 gate.

**Acceptance criteria (R2, in addition to R1's preserved ACs):**

- AC1-R2: `loginClaudeAccount` wipes `.credentials.json` before invoking the runner. Verified by test.
- AC2-R2: ANSI escape sequences in the OAuth URL stream do not corrupt the URL passed to `urlOpener.Open`. Verified by test.
- AC3-R2: URL detection survives terminal line-wrap (option a buffering). Verified by `TestLineScannerDetectsURLAcrossMultipleLines`.
- AC4-R2: `mage testPkg ./internal/cli` GREEN, coverage ≥70%.
- AC5-R2: `mage test` GREEN full suite (assumes no return of pre-existing tmpfs flake; if it returns, environmental).
- AC6-R2 (orchestrator-managed): dev smoke test on `valv account add claude hylla` shows browser auto-opens AND `valv claude` launches without re-auth — encoded as gate for DROP_7 Phase-7 close.

**Verification target:** `mage testPkg github.com/evanmschultz/valv/internal/cli` then `mage test` then dev smoke test.

**YAGNI hardened:** three fixes only (with FIX 3 conditional on dev triage). Do NOT introduce new interfaces, new cross-platform `open` branching (TODO sufficient), or fsnotify-based watcher migration.

---

### Ctrl-C UX interim guidance (until Unit 7.11 R2 lands)

Confirmed working pattern from dogfood 2026-05-16: after completing OAuth in browser and pasting code back, claude does NOT auto-exit. Users must press Ctrl-C twice:

1. First Ctrl-C → claude prints `Press Ctrl-C again to exit`.
2. Second Ctrl-C → claude exits cleanly, container removes (`--rm`).

Three to four total presses sometimes needed if claude's TUI is mid-render. Wait for the `Press Ctrl-C again to exit` line before the second tap.

**Unit 7.11's FIX B obsoletes this guidance** — once it lands, claude's container auto-exits when `.credentials.json` is written. Document this as obsolete in the user-facing docs (when those exist) post-7.11.

---

## Path A Scope (superseded 2026-05-15 — preserved as historical record)

**Codex parity for Claude auth.** DROP_6.2's container-side `claude auth login` flow has invisible paste prompt (TUI mode doesn't render through Docker pty). User verified on 2026-05-15 that host-side `claude setup-token` works perfectly: browser auto-opens, paste prompt clearly visible, completes cleanly. This drop replaces the container-auth flow with host-subprocess auth mirroring Codex's existing pattern.

**Design summary:**
- `valv account add claude <name>` runs `claude setup-token` as a host subprocess with `CLAUDE_CONFIG_DIR=<managed-home>` env set (structural mirror of Codex's `systemCodexAccountAuthRunner.Login` which runs `codex login` with `CODEX_HOME=<managed-home>`).
- Host browser opens automatically (claude CLI's `open` works because we're on host, not container).
- User completes auth in browser, pastes code back into terminal — paste prompt is visible (verified 2026-05-15).
- `claude setup-token` writes credentials to macOS keychain (`Claude Code-credentials` service, account = macOS username) — same behavior as `claude auth login`. `CLAUDE_CONFIG_DIR` only controls config/state location, NOT credentials on macOS (verified 2026-05-15 — `.credentials.json` did not appear in the test dir).
- **Immediately after the auth subprocess exits**, Valv extracts the token from keychain via `security find-generic-password -s "Claude Code-credentials" -a "$(id -un)" -w` and writes it to `<managed-home>/.credentials.json` as a JSON file (single-key `{"claudeAiAccessToken": "<token>"}` — matches the format DROP_5's `.credentials.json` presence-check assumed).
- Multi-account isolation: each `valv account add claude <name>` does auth → extract → store in a sequence, BEFORE the next auth would overwrite the keychain entry. Per-Valv-account `.credentials.json` files survive on disk independent of the keychain.
- `valv claude` launch (`services/claude/service.go::Run`) reads `<managed-home>/.credentials.json`, extracts the token, sets `CLAUDE_CODE_OAUTH_TOKEN=<token>` env var on the container.
- Pre-flight check: at `account add claude` time, verify `claude` is on host PATH; if not, print remediation `npm install -g @anthropic-ai/claude-code@2.1.89`. Mirrors how Codex's preflight requires `codex` on PATH.

**Removes from DROP_6.2:** the container-launch code path (the `valv-claude:dev` container running `claude auth login`). The new host-subprocess approach replaces it entirely. `claude_auth.go` is heavily reworked, NOT extended.

**Keeps from DROP_6.3:** `.claude.json` parsing for email extraction in `ReadAccountIdentity`. Host `claude setup-token` still writes `.claude.json` with `oauthAccount.emailAddress` to `$CLAUDE_CONFIG_DIR` — that data is still useful for `account list` display.

## Dev-Confirmed Findings (2026-05-15)

1. **Host-side `claude auth login` works correctly** with `CLAUDE_CONFIG_DIR` set: browser opens, "Paste code here if prompted >" prompt is visible, `Login successful.` printed.
2. **`CLAUDE_CONFIG_DIR` does NOT redirect credentials on macOS.** Credentials land in keychain regardless. `.claude.json` (identity + state, NO tokens) is written to the env-var-set dir.
3. **Keychain confirmed**: `Claude Code-credentials` service, account = macOS username (`evanschultz`), class `genp`. Extractable via `security find-generic-password -s "Claude Code-credentials" -a "$USER" -w`.
4. **Per-OS-user keychain scope**: one claude credential per macOS user — overwrites on each `claude auth login`. Multi-account isolation requires Valv to extract+store between auths.
5. **`claude setup-token` is the better UX**: ASCII art welcome, browser opens, URL fallback printed, paste prompt visible. Long-lived headless token with `scope=user:inference`. Anthropic's blessed headless path per focus-plan §7.
6. **claudebox (https://github.com/RchGrav/claudebox) uses in-container auth**: bind-mounts `.claude/` + `.claude.json` per slot, uses `-it` only if TTY, runs `claude` (no subcommand) and lets it auto-prompt. We are NOT following that approach — host-side auth gives cleaner Codex parity.

## Open Design Questions Routed To Planner

- **Q1 — What does `claude setup-token` actually print to stdout after the user pastes the code?** Possibilities: (a) just `Success` with token written to keychain only — extract via `security`; (b) prints `Token: ...` on stdout — capture there too; (c) writes to a file in `$CLAUDE_CONFIG_DIR`. The planner should NOT assume; the builder will resolve via real run during build verification. Per the 2026-05-15 test pattern, prefer keychain extraction as the primary capture method since we know it ends up there regardless.
- **Q2 — `setup-token` vs `auth login`?** `setup-token` produces a long-lived scoped token (`user:inference`); `auth login` produces a regular session token. For Valv's "per-account persistent credential in managed home" pattern, the long-lived token is the right choice. Confirm with the builder that `CLAUDE_CODE_OAUTH_TOKEN=<setup-token-output>` works to authenticate `claude` invocations inside the container.
- **Q3 — `CLAUDE_CODE_OAUTH_TOKEN` env-var visibility in `ps`.** Passing the token via env var makes it visible to anyone who can `ps eww` the container's process. Acceptable for v0.1.0 (same constraint Codex has with `CODEX_HOME` mount). Document.
- **Q4 — What service name does `claude setup-token` use in keychain?** Probably the same `Claude Code-credentials` as `auth login` but the planner should verify by having the builder check post-auth keychain state.

## Path A Planner (superseded 2026-05-15 — preserved as historical record)

Three atomic units. Units 7.1 and 7.2 may run in parallel (disjoint packages). Unit 7.3 is blocked by both.

### Design decisions locked by planner

- **Interface shape:** `claudeAuthRunner` is redefined with two methods — `RunSetupToken(ctx, homePath, stdin, stdout, stderr) error` and `ExtractKeychainToken(ctx, macOSUser) (string, error)`. The old `EnsureImage` / `RunContainer` methods are gone. Tests get a new stub.
- **Preflight inline:** `exec.LookPath("claude")` lives inside `runClaudeHostCommand` — same pattern as Codex's `exec.LookPath("codex")` in `runCodexHostCommand`. No separate preflight function.
- **Keychain user:** `os/user.Current().Username` — NOT `os.Getenv("USER")`. Error from `user.Current()` propagates immediately.
- **`CLAUDE_CONFIG_DIR` env for setup-token:** Set to `homePath` (managed home) so `.claude.json` identity data lands in the right place. Does NOT redirect keychain.
- **Token extraction timing:** Immediately after `RunSetupToken` returns nil. Single `security find-generic-password -s "Claude Code-credentials" -a <username> -w` call. If the builder discovers `setup-token` uses a different service name, they update the constant and document the actual name in the worklog.
- **`.credentials.json` format we write:** `{"claudeAiAccessToken":"<raw-token>"}` — single-key JSON. The service reads this key; `ReadAccountIdentity` only stat-checks the file, so the format is irrelevant to LoggedIn.
- **Service graceful skip:** If `.credentials.json` is absent or unreadable during `buildRequest`, log a debug message and omit `CLAUDE_CODE_OAUTH_TOKEN` from env. Do NOT fail `Run`. This preserves backward compatibility with manually-authed accounts.
- **Logout:** `wipeClaudeCredentials` stays as file-wipe only. Do NOT call `security delete-generic-password` — too destructive to user's host claude sessions.
- **`loginClaudeAccount`:** Keeps its no-TTY-guard semantics (explicit re-login, matches Codex's `loginCodexAccount`). Updates to new runner interface.
- **Q4 accepted Unknown:** Builder must verify and document which keychain service name `claude setup-token` actually writes (expected: `Claude Code-credentials`).
- **7.1 LOC exception:** `claude_auth.go` is a full-file rewrite (~160-180 LOC), exceeding the 80-120 soft target. Justified: splitting the interface change from its callers in the same file would leave the package uncompilable between units.

---

### Unit 7.1 — Rewrite `claude_auth.go`: new host-subprocess runner + reworked orchestration

**state:** done
**blocked_by:** —
**paths:** `internal/cli/claude_auth.go`
**packages:** `internal/cli`

**What to build:**

Rewrite `claude_auth.go` in full. Keep the file; replace its entire content. Do NOT extend the existing interface — redefine it.

New `claudeAuthRunner` interface (replaces the old EnsureImage/RunContainer interface):

```go
type claudeAuthRunner interface {
    RunSetupToken(ctx context.Context, homePath string, stdin io.Reader, stdout, stderr io.Writer) error
    ExtractKeychainToken(ctx context.Context, macOSUser string) (string, error)
}
```

New `systemClaudeAccountAuthRunner` struct (the production implementation):

- `RunSetupToken`: calls `runClaudeHostCommand(ctx, homePath, stdin, stdout, stderr, "setup-token")`. Mirror of `systemCodexAccountAuthRunner.Login`.
- `ExtractKeychainToken`: calls `runClaudeHostCommand(ctx, "", nil, nil, nil, "extract", macOSUser)` — but NOT via `runClaudeHostCommand` (that's for the `claude` binary). Instead: use `exec.CommandContext(ctx, "security", "find-generic-password", "-s", claudeKeychainService, "-a", macOSUser, "-w")` directly, capturing stdout. Trim the output. Return error if exit non-zero. `claudeKeychainService` is a package-level constant set to `"Claude Code-credentials"` (builder verifies and updates if `setup-token` uses a different service name).

New `runClaudeHostCommand(ctx, homePath, stdin, stdout, stderr io.Writer, args ...string) (string, error)`:
- Mirror of `runCodexHostCommand`. `exec.LookPath("claude")` for the binary. `appendOrReplaceEnv(os.Environ(), "CLAUDE_CONFIG_DIR", homePath)`. If homePath is empty (for keychain extraction calls) — wait, `ExtractKeychainToken` does NOT call `runClaudeHostCommand` (it calls `security` directly). So `runClaudeHostCommand` always has a non-empty homePath.

New `claudeAuthRunnerFromContext(ctx, cmd)`: same pattern as `codexAccountAuthFromContext` — reads from context via `claudeAuthRunnerKey{}`, falls back to `hostClaudeAccountAuth` (a `var` holding `systemClaudeAccountAuthRunner{}`).

New `ensureClaudeAccountReady(cmd, account, options)`:
1. `options.SkipLogin` → return nil immediately (no wipe, no auth).
2. Non-TTY guard: `!commandHasTTY(cmd.InOrStdin())` → return actionable error. Fires BEFORE wipe (preserves existing creds if non-TTY call).
3. Pre-flight: `exec.LookPath("claude")` → if err, return: `"claude CLI not found on PATH; install with: npm install -g @anthropic-ai/claude-code@2.1.89"`.
4. Wipe: `wipeClaudeCredentials(account.HomePath)`.
5. `writeCLINotice(...)` announce auth.
6. `runner.RunSetupToken(ctx, account.HomePath, stdin, stdout, stderr)` — streams to TTY.
7. `user.Current()` → extract macOS username. Error → return.
8. `token, err := runner.ExtractKeychainToken(ctx, username)` → error → return.
9. Write `{"claudeAiAccessToken": "<token>"}` as JSON to `filepath.Join(account.HomePath, ".credentials.json")` with mode 0o600.
10. `claudeprovider.ReadAccountIdentity(account.HomePath)` → verify `identity.LoggedIn`. If false → return error.

New `loginClaudeAccount(cmd, account, paths)`:
- Same as `ensureClaudeAccountReady` but WITHOUT the non-TTY guard and WITHOUT the preflight check (same no-guard semantics as `loginCodexAccount`). Steps 4-10 above.

Keep `wipeClaudeCredentials` unchanged. Delete `buildClaudeAuthContainerRequest` and all Docker import usage.

**Imports lost:** `dockeradapter`, `claudeprovider` from the `EnsureImage` side — but `claudeprovider` is still used for `ReadAccountIdentity` + `ContainerClaudeDir`? Check: `buildClaudeAuthContainerRequest` used `claudeprovider.ContainerClaudeDir` and `claudeprovider.ContainerHomeDir`. Those are gone. `ReadAccountIdentity` is still called in steps 10. So `claudeprovider` import stays. The docker import is dropped.

New imports needed: `encoding/json`, `os/user`.

**`appendOrReplaceEnv` stays** — it is in `account_auth.go`, shared across the package. No move needed.

**Acceptance criteria:**
- AC1: `mage testPkg github.com/evanmschultz/valv/internal/cli` passes (all existing non-Claude tests still pass; new runner compiles).
- AC2: `claudeAuthRunner` interface has exactly two methods: `RunSetupToken` and `ExtractKeychainToken`. Verified by inspection.
- AC3: `ensureClaudeAccountReady` SkipLogin returns nil without touching the filesystem (same invariant as before).
- AC4: `ensureClaudeAccountReady` non-TTY guard fires BEFORE `wipeClaudeCredentials`. Verified by test: call with non-TTY + pre-existing creds file → file still present after error return.
- AC5: `runClaudeHostCommand` sets `CLAUDE_CONFIG_DIR=<homePath>` in the subprocess environment. Verified by fake `claude` script test similar to `TestSystemCodexAccountAuthRunnerLoginUsesCODEXHOME`.
- AC6: `ExtractKeychainToken` propagates `os/user.Current()` errors (verified by test with a stub that returns error from that method).
- AC7: No Docker adapter import remains in `claude_auth.go`.

**Verification target:** `mage testPkg github.com/evanmschultz/valv/internal/cli`

---

### Unit 7.2 — Service-layer `CLAUDE_CODE_OAUTH_TOKEN` env propagation

**state:** done
**blocked_by:** —
**paths:** `internal/services/claude/service.go`
**packages:** `internal/services/claude`

**What to build:**

Edit `services/claude/service.go`. Add token injection into `buildRequest`.

After the `docker.ContainerRunRequest{...}` literal is constructed (currently line 253-274), add:

```go
if token, err := readClaudeAuthToken(profile.HomePath); err != nil {
    s.debug("claude auth token unreadable", "home", profile.HomePath, "err", err)
} else if token != "" {
    request.Env["CLAUDE_CODE_OAUTH_TOKEN"] = token
}
```

New unexported helper `readClaudeAuthToken(homePath string) (string, error)` in `service.go`:
- Opens `filepath.Join(homePath, ".credentials.json")`.
- Unmarshals into `struct { Token string \`json:"claudeAiAccessToken"\` }`.
- Returns `Token` (may be empty string if key absent). Returns error only on read/parse failure.
- If file not found (`os.IsNotExist`): return `"", nil` (not an error — silent skip).

This is ~30 LOC of production code.

No new imports (already imports `encoding/json`, `os`, `path/filepath`). Verify via `go.mod` that these are available — they are stdlib.

The `Env` map in `buildRequest` is initialized from `prepared.Env` (line 257). `prepared.Env` is a `map[string]string`. Adding to it directly is fine since `buildRequest` is the exclusive constructor.

**`CLAUDE_CODE_OAUTH_TOKEN` env var in `ps` visibility:** Add comment: `// CLAUDE_CODE_OAUTH_TOKEN is passed as an environment variable and is visible to ps(1). Acceptable for v0.1.0.`

**Acceptance criteria:**
- AC1: `mage testPkg github.com/evanmschultz/valv/internal/services/claude` passes.
- AC2: When a profile home contains a valid `.credentials.json` with `claudeAiAccessToken`, `Run` produces a `ContainerRunRequest` with `Env["CLAUDE_CODE_OAUTH_TOKEN"] == <token>`. Verified by new test case in `service_test.go`.
- AC3: When `.credentials.json` is absent, `Run` succeeds and `Env["CLAUDE_CODE_OAUTH_TOKEN"]` is not set. Verified by existing `TestRunSucceedsWithBoundProject` (profile home is `t.TempDir()` with no creds file) — must still pass unchanged.
- AC4: When `.credentials.json` is present but unreadable (e.g. bad JSON), `Run` succeeds (graceful skip) and logs a debug message. Token env var omitted.
- AC5: `readClaudeAuthToken` is unexported. Verified by inspection.

**Verification target:** `mage testPkg github.com/evanmschultz/valv/internal/services/claude`

---

### Unit 7.3 — Tests: rewrite Claude auth tests + service env propagation tests

**state:** done
**blocked_by:** 7.1, 7.2
**paths:** `internal/cli/claude_auth_test.go`, `internal/services/claude/service_test.go`, `internal/adapters/providers/claude/account_test.go`
**packages:** `internal/cli`, `internal/services/claude`, `internal/adapters/providers/claude`

**What to build:**

**`claude_auth_test.go` — full rewrite:**

New `stubClaudeAccountAuthRunner` struct matching the new interface:
```go
type stubClaudeAccountAuthRunner struct {
    setupTokenErr   error
    setupTokenHits  int
    extractTokenErr error
    extractToken    string // returned by ExtractKeychainToken
    extractHits     int
}
func (s *stubClaudeAccountAuthRunner) RunSetupToken(_ context.Context, _ string, _, _, _ io.Writer) error { ... }
func (s *stubClaudeAccountAuthRunner) ExtractKeychainToken(_ context.Context, _ string) (string, error) { ... }
```

New `installStubClaudeAccountAuth(t, cmd, stub)`: injects via `claudeAuthRunnerKey{}` context value.

New `newTestClaudeCmd()`: keep as-is (minimal cobra.Command with bytes buffers — non-TTY).

Test cases for `ensureClaudeAccountReady`:
- `TestEnsureClaudeAccountReadyRejectsNonTTY` — non-TTY → error containing "TTY"; no `RunSetupToken` hit.
- `TestEnsureClaudeAccountReadyRespectsSkipLogin` — SkipLogin=true → nil; no wipe; no `RunSetupToken` hit; pre-existing creds file preserved.
- `TestEnsureClaudeAccountReadyNonTTYDoesNotWipe` — non-TTY + pre-existing creds → error; file preserved.
- `TestEnsureClaudeAccountReadyRunsSetupTokenAndWritesCreds` — stub returns token "test-token"; function writes `.credentials.json`; verify `LoggedIn=true` from `ReadAccountIdentity`.
- `TestEnsureClaudeAccountReadyFailsWhenSetupTokenFails` — `RunSetupToken` returns error; function returns error; no creds written.
- `TestEnsureClaudeAccountReadyFailsWhenExtractFails` — `RunSetupToken` succeeds; `ExtractKeychainToken` returns error; function returns error.
- `TestEnsureClaudeAccountReadyFailsWhenNoCredsAfterWrite` — stub returns empty token ""; `json.Marshal` produces `{"claudeAiAccessToken":""}` but file exists — `LoggedIn=true`. Hmm. Actually if the token is empty we should fail before writing. Builder: return error if extracted token is empty string (treat empty as extraction failure).

Test cases for `loginClaudeAccount`:
- `TestLoginClaudeAccountProceedsWithoutTTYGuard` — non-TTY cmd, stub returns token "tok"; succeeds; `RunSetupToken` hit = 1.
- `TestLoginClaudeAccountFailsWhenExtractFails` — extraction fails; returns error.

Test for system runner (fake `claude` binary):
- `TestSystemClaudeAccountAuthRunnerRunSetupTokenUsesCLAUDE_CONFIG_DIR` — install fake `claude` binary that logs env; call `systemClaudeAccountAuthRunner{}.RunSetupToken(ctx, "/tmp/home", ...)`; verify log contains `CLAUDE_CONFIG_DIR=/tmp/home`. Mirror of `TestSystemCodexAccountAuthRunnerLoginUsesCODEXHOME`.
- `TestRunClaudeHostCommandFailsWhenClaudeMissing` — PATH cleared; verify error contains "claude" and is actionable.

`installFakeHostClaude(t, mode)`: mirror of `installFakeHostCodex`. Script logs args + `CLAUDE_CONFIG_DIR`.

**`service_test.go` — add two test cases (append to existing file):**

- `TestRunSetsClaudeCodeOAuthTokenWhenCredentialsPresent`: write `{"claudeAiAccessToken":"svc-token"}` to profile home before `service.Run`; verify `executor.got.Env["CLAUDE_CODE_OAUTH_TOKEN"] == "svc-token"`.
- `TestRunOmitsClaudeCodeOAuthTokenWhenCredentialsMissing`: profile home is empty `t.TempDir()`; `Run` succeeds; `executor.got.Env["CLAUDE_CODE_OAUTH_TOKEN"]` is `""` (zero value for absent key). Existing `TestRunSucceedsWithBoundProject` already covers this case; add an explicit assertion to it OR add this as a separate test.

**`account_test.go` — no changes.** Fixtures use `{"claudeAiOauth":{"accessToken":"tok"}}` format; `ReadAccountIdentity` only stat-checks presence — tests pass as-is. Builder must NOT alter fixtures.

**Acceptance criteria:**
- AC1: `mage testPkg github.com/evanmschultz/valv/internal/cli` passes with the new stub and all new test cases green.
- AC2: `mage testPkg github.com/evanmschultz/valv/internal/services/claude` passes with the two new service test cases.
- AC3: `mage testPkg github.com/evanmschultz/valv/internal/adapters/providers/claude` passes unchanged.
- AC4: `mage test` passes (full suite, race detector, 70% coverage floor).
- AC5: Old test names `TestEnsureClaudeAccountReadyRejectsNonTTY`, `TestBuildClaudeAuthContainerRequestShape`, `TestLoginClaudeAccountSkipsNonTTYGuard`, `TestLoginClaudeAccountFailsWhenNoCredsAfterContainer` are REPLACED (not just augmented) — the old names belonged to the container-based flow and must not appear in the final test file.

**Verification target:** `mage testPkg github.com/evanmschultz/valv/internal/cli` then `mage testPkg github.com/evanmschultz/valv/internal/services/claude` then `mage testPkg github.com/evanmschultz/valv/internal/adapters/providers/claude` then `mage test`

## Notes

- **Mechanical drop — trimmed cascade applies** per memory `feedback_trimmed_cascade_for_mechanical_drops.md`. Single planner spawn. Per-unit build-QA stays. The "novel" part is keychain extraction, which is shell-out + parse — no concurrent/async/state-machine logic.
- **Codex auth template to mirror**: `internal/cli/account_auth.go::systemCodexAccountAuthRunner.Login` runs `codex login` as a host subprocess with `CODEX_HOME=<path>` env. Our new `systemClaudeAccountAuthRunner.Login` should mirror this shape with `claude setup-token` + `CLAUDE_CONFIG_DIR=<path>` env, plus the post-exit keychain-extract step.
- **Test injection pattern**: keep the `claudeAccountAuthRunnerKey` context-key + interface pattern from DROP_6.2. The interface methods need updating from `EnsureImage` / `RunContainer` (container-based) to something like `RunSetupToken(ctx, homePath, stdin/out/err)` + `ExtractKeychainToken(ctx, account)`. Tests inject a stub that simulates both.
- **Code to DELETE in this drop**: the container-launch path in `claude_auth.go` (the `valv-claude:dev` invocation in `ensureClaudeAccountReady`). The image is still needed for `valv claude` LAUNCH, just not for auth.
- **`logoutManagedAccount` Claude case**: keep as file-wipe of `.credentials.json` from DROP_6.2. But also consider clearing the keychain entry — possibly with `security delete-generic-password -s "Claude Code-credentials" -a "$(id -un)"`. Planner should decide whether to do this (defensive cleanup) or leave it (less destructive of user's host-claude state).
- **`.credentials.json` schema**: write as `{"claudeAiAccessToken": "<token>"}` — single-key JSON matching the test fixture at `internal/adapters/providers/claude/account_test.go:14`. The Linux container's claude CLI reads this format. `ReadAccountIdentity` already checks file presence; it doesn't need to parse the JSON to determine LoggedIn — but DROP_6.3 added `.claude.json` parsing for email, which we keep.
- **`CLAUDE_CODE_OAUTH_TOKEN` env at launch**: when `valv claude` launches the container, the existing `services/claude/service.go::Run` builds an env map. Add a step that reads `<managed-home>/.credentials.json`, extracts `claudeAiAccessToken`, sets `CLAUDE_CODE_OAUTH_TOKEN=<value>` in the env map. Container claude will use the env-var auth.
- **Pre-flight host PATH check**: at `valv account add claude` time, before launching the subprocess, verify `claude` is on PATH. If not, error with: `claude CLI not found on PATH. Install with: npm install -g @anthropic-ai/claude-code@2.1.89`.
- **Out of scope**:
  - The container-LAUNCH path (`valv claude` from a bound project) keeps working as DROP_5 designed. We just add the env-var threading.
  - Cross-provider `account switch` semantics, TUI parity, globalswitch — all DROP_8.
  - Codex hardening (force-relogin flag, etc.) — DROP_10 cleanup.
  - GitHub Actions release + brew formula — DROP_9.
