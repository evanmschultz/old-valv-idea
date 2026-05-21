# Valv Claude Code Focus Plan

Date: 2026-04-17

> **Historical artifact.** This plan drove DROP_1 through DROP_8 (all closed; see `main/PLAN.md`). The active drop tree is in `main/PLAN.md`; the per-drop lifecycle lives in `main/drops/WORKFLOW.md`. References to `AGENTS.md` in this file should be read as `CLAUDE.md` — `AGENTS.md` was consolidated into `CLAUDE.md` on 2026-05-20.

Goal: two providers (Codex + Claude Code) running inside Valv-managed Docker runtimes with per-project account binding. No API wrapper. No OpenAI compatibility surface. Valv's job is narrowed to "run `codex` or `claude` in a Valv-managed Docker container against a Valv-managed account home that the bound project selects." Cleanup and idiomatic-Go rework are explicitly deferred to the Later Drop at the bottom of this file.

---

## 1. Goal And Non-Goals

Locked-in scope:

- Valv runs two provider CLIs inside managed Docker runtimes: `codex` (existing) and `claude` (new).
- Per-project binding resolves the `(provider, account)` tuple for every launch via `(working dir -> project) -> binding`.
- Each account is a Valv-managed profile home on disk, mounted into the runtime container at a provider-idiomatic path.
- One image per provider, rebuilt via the existing `internal/services/images` model.
- Direct pass-through CLI only: `valv codex ...` and `valv claude ...` forward args to the provider CLI exactly the way `valv codex` already does today.

Explicit non-goals:

- No reimplemented HTTP API. No OpenAI-compat surface. No Anthropic-compat surface. No `/v1/chat/completions`. No SSE bridging.
- No translation layer between third-party HTTP callers and vendor CLIs. Third-party callers are out of scope; violates Codex/Anthropic ToS.
- No credential rewriting of the user's global `~/.codex` or `~/.claude` credentials as the primary model. Host login is allowed when a provider requires it (Codex does), but Valv does not rotate the global file as its core mechanism.
- No `huh`, no non-v2 Charm libraries, no cross-platform abstractions. Scope stays macOS-first per `AGENTS.md` §2.
- No idiomatic-Go cleanup in this fast path. Catalogued in §8 as a separate later drop.

Current-direction documents that contradict this plan and must be reconciled (not edited here, but flagged for §2 doc scrubs): `main/AGENTS.md` §1 and §4 still reference `valv/v1/api`, OpenAI-compat routing, and Anthropic-compat routing; `main/valv_architecture_notes.md` "Public API shape"; `main/PLAN.md` (Track F "OpenAI-compatible API surface", Agent 5); `main/API_COMPAT_EXECUTION_PLAN.md`; `main/VALV_REPO_PLAN.md` `api/openapi/` directory proposal; `main/CONTRIBUTING.md` "Compatibility change policy" bullets.

---

## 2. Required Removals (API Wrapper + OpenAI Compat)

Each item is individually addressable. A builder can pick one and do it in isolation.

### Source code

1. Delete package `main/internal/api/openai/` in its entirety (`doc.go`, `encode.go`, `errors.go`, `handler.go`, `handler_test.go`, `json.go`, `types.go`, `types_test.go`). This is the OpenAI-compat HTTP handler surface.
2. Delete package `main/internal/services/openaiapi/` in its entirety (`codex_events.go`, `codex_models.go`, `service.go`, `service_test.go`, `service_integration_test.go`). This is the Codex-to-OpenAI bridge service the handler calls.
3. Delete `main/internal/cli/api.go` — removes `valv api` and `valv api serve` commands (the `newAPICommand`, `newAPIServeCommand`, `runAPIServe`, `warmRuntimeForAPIServe`, `runAPIRuntimeSweeper` functions, and the `openAIAPIService` interface).
4. Remove API wiring in `main/internal/cli/root.go`:
   - Drop the `apiCmd := newAPICommand(paths, opts)` line and its `cmd.AddCommand(... apiCmd ...)` entry (currently lines 126-135).
   - Remove the `valv api serve --runtime-ttl 2m` example from the root `Example` string (currently line 63).
5. Remove all OpenAI-wrapper scaffolding from `main/internal/cli/operator_helpers.go`. Builder must remove **every** line in the list or §6.1 will fail to compile:
   - Line 17 import: `openaihandler "github.com/evanmschultz/valv/internal/api/openai"`.
   - Line 25 import: `openaiapiservice "github.com/evanmschultz/valv/internal/services/openaiapi"`.
   - Lines 107-128 `newOpenAIAPIService` helper in its entirety.
   - Line 315 dangling reference: `var _ = openaihandler.ChatCompletionsPath`.
   Verify all four exist at drop time (the import line numbers may shift after earlier edits in the same PR — grep by identifier).
6. Remove OpenAI-wrapper test fixtures from `main/internal/cli/extended_test.go`: the `apiServeStubService` type, the `openaiapi "github.com/evanmschultz/valv/internal/api/openai"` import (line 20), and any `TestAPIServe*` / `TestNewAPIServeCommand*` tests in that file. Verify by grepping `openaiapi`, `apiServeStub`, and `api serve` inside that file during the drop.
7. Delete `main/compatibility.go` and `main/compatibility_test.go`. Delete `main/codex-openai-compatibility.json`. These are the embedded OpenAI-compat manifest and the loader with its tests.
8. Delete `main/API_COMPAT_EXECUTION_PLAN.md` (the plan document driving the API direction that is being dropped).

### Docs

9. Strip the "API Compatibility Matrix" and "API Smoke Test Flow" sections from `main/README.md` (lines 69-129, verify exact range at drop time). Leave the rest of README untouched in this drop; a separate doc drop later rewrites it around the two-provider pass-through model.
10. In `main/AGENTS.md`, scrub or replace:
    - §1 "Product Direction" bullets that reference `valv/v1/api` as the canonical API namespace (line 12) — replace with "`valv codex` and `valv claude` are pass-through containerized launchers" without mentioning an HTTP API surface.
    - §4 "Runtime Model" block "For API compatibility surfaces:" (lines 61-65) — delete the whole block; neither OpenAI-compat nor Anthropic-compat routing applies under the new scope.
    - §8 bullets that reference `api serve` long-running behavior (lines 216-217) — delete the two `api serve` specific bullets.
    - "OpenAI Compatibility Contract" section at the bottom of `AGENTS.md` (lines 320-332) — delete the entire section.
11. In `main/PLAN.md`, stop treating the API surface as in-scope. The fastest fix is to add a top-of-file note: "Superseded by `VALV_CLAUDE_CODE_FOCUS_PLAN.md` dated 2026-04-17. API surface sections (Track F, Agent 5, OpenAI streaming sections) are obsolete." Do not rewrite `PLAN.md` wholesale in this drop — that is §8 material.
12. In `main/valv_architecture_notes.md`, the "Public API shape" section (lines 250-264) is obsolete; add a one-line note at the top that it is superseded by this plan and do not delete it yet (§8 will decide whether to preserve or remove in the later doc sweep).
13. In `main/CONTRIBUTING.md`, remove the "Compatibility change policy" block at lines 13-16 (the bullets referencing `valv api` and `codex-openai-compatibility.json`). Leave the surrounding "Workflow" and "Tooling" sections untouched.
14. In `main/VALV_REPO_PLAN.md` line 78 (`│   └── openapi/         # optional public API descriptions`), remove the `api/openapi/` tree entry from the directory diagram. If removing the line breaks the diagram's visual structure (trailing `├── api/` with no children), also remove the parent `├── api/` line (line 77). Mirror the `valv_architecture_notes.md` treatment by adding a one-line supersede header at the top of `VALV_REPO_PLAN.md` noting that it is superseded by this focus plan for the public-API-surface sections.

### Config

15. Verify `main/internal/config/config.go` and `main/internal/config/paths.go` contain no API-listener fields. Grep for `listen`, `APIServe`, `api_serve`, `api.listen` during the drop. If any exist, remove them; if none exist, nothing to do here beyond the verification step.

### Mage

16. No changes required in `main/magefile.go`. Verified: there is no API-specific mage target. `Test`, `TestPkg`, `Integration`, `Golden`, `GoldenUpdate`, `Build`, `Run`, `Dev.*` are all provider-generic. Once §§1-8 land the deleted tests will simply disappear from `mage test` output.

### Gate for this section

- `mage test` passes after deletions (no dangling imports, no orphan tests).
- `mage integration` passes (no `valv api serve` integration path).
- `mage build` produces a binary with no `valv api` group under `valv --help`.
- `gh run watch` on the pushed branch is green per `AGENTS.md` §12.

---

## 3. Claude Code Provider Addition

### 3.1 Provider identity changes

- `main/internal/domain/types.go` currently defines exactly one provider constant: `ProviderCodex Provider = "codex"` (line 11), and `ParseProvider` only accepts `codex` (lines 14-21). Verified — no other providers are wired.
- Add `ProviderClaude Provider = "claude"` alongside `ProviderCodex`.
- Extend `ParseProvider` to accept both `codex` and `claude`.
- Callers to audit for missing branches (all currently assume Codex only, per grep of `domain.ProviderCodex` across `main/internal`):
  - `main/internal/cli/account_auth.go`: `ensureManagedAccountReady`, `logoutManagedAccount`, `loginManagedAccount` all `switch provider { case domain.ProviderCodex }` (lines 37, 46, 55). Add `case domain.ProviderClaude` returning Claude-specific auth runners.
  - `main/internal/services/manage/service.go` `DefaultHostProfile` (line 152) only handles `ProviderCodex`. **Staging rule:** in §6.2 the `ProviderClaude` branch returns a stub `(domain.Profile{}, errors.New("claude host profile not yet available"))` — or an equivalent sentinel — so domain plumbing compiles without depending on a package that does not exist yet. §6.4 flips this stub to the real `claudeprovider.DefaultHostProfile(homeDir)` call once the adapter package lands. This sequencing is required because §6.4 (adapter scaffold) lands before §6.5 (launch), but §6.2 (domain plumbing) ships first to keep the slice graph linear.
  - `main/internal/services/codex/service.go` `resolveBinding` asserts `binding.Provider == domain.ProviderCodex` (line 223) and `profile.Provider == domain.ProviderCodex` (line 234). These must stay strict — the Claude launch service (§3.4) is a separate service in a separate package, not a case branch inside the Codex service.
  - `main/internal/services/globalswitch/service.go` `Switch` asserts `provider != domain.ProviderCodex` returns unsupported (line 86). Decision needed (§7): extend to Claude or stage globalswitch for later.
  - `main/internal/tui/manage/picker.go` already takes a `domain.Provider` parameter; no code change needed.
  - `main/internal/cli/manage.go`: several `domain.ProviderCodex` defaults in `parseOptionalProvider` and the hardcoded `return []domain.Provider{domain.ProviderCodex}` at line 985. Extend to return both providers.

### 3.2 Claude runtime model

Evidence for Claude Code auth and config semantics is already captured in `main/VALV_ACCOUNT_SWITCH_PLAN.md` (read during preparation). Headline facts:

- `$CLAUDE_CONFIG_DIR` controls the config/credential root the CLI reads. This is the Claude-side equivalent of `$CODEX_HOME`.
- Project settings files are supported.
- `/logout` is the official re-auth path.
- On the macOS host, Claude's native credential store is the OS keychain — which is why Valv does not run Claude on the bare host. Instead, Valv runs the Claude CLI inside a Debian-slim Linux container, where `$CLAUDE_CONFIG_DIR` deterministically points at an on-disk credential file inside the container's bind-mounted account home. This is a container-mount contract, not a host-OS branch; there is no Linux-vs-Windows conditional in the Valv code path.

Runtime plan:

- New Docker image recipe `valv-claude:dev`, built from Debian slim (mirror the existing Codex recipe in `main/internal/services/images/service.go` `DefaultCodexDockerfile`, lines 518-548). Install Node + the Claude Code CLI pinned to a specific version in the Dockerfile (fast path — no dynamic resolver; see §3.5). Note: the Claude CLI install channel, package name, and pinned-version choice are explicitly an Unknown routed in §7 — do not guess in the image recipe. Verify via Context7 (`/anthropics/claude-code` if it exists) or fall back to the official claude.com docs at build-drop time.
- Mount target: `/home/valv/.claude` inside the container (pattern matches the existing `ContainerCodexDir = "/home/valv/.codex"` constant in `main/internal/adapters/providers/codex/runtime.go`, line 21).
- Env: `CLAUDE_CONFIG_DIR=/home/valv/.claude`, `HOME=/home/valv`, `USER=valv`, `LOGNAME=valv`, plus the terminal env passthrough list already used by Codex (`main/internal/adapters/providers/codex/runtime.go` `terminalEnvPassthrough`, lines 538-553). **Decision:** duplicate `terminalEnvPassthrough` (and any other Codex-adapter helper the Claude adapter needs) inside `main/internal/adapters/providers/claude/runtime.go` for v1; dedupe into a shared helper during §8. Single decision — no factoring branch.
- Because Claude credentials live in the macOS keychain on the host, host-side `claude login` cannot seed a Valv-managed account home the way `codex login --home <path>` does. The plan is:
  1. Default to device-code auth inside the container. The Claude CLI's device-code flow writes credentials under `$CLAUDE_CONFIG_DIR`, which is the mounted Valv-managed account home. No macOS keychain interaction required.
  2. Explicitly warn in help text that strict per-account OAuth isolation on macOS is only reliable through the containerized path (the `VALV_ACCOUNT_SWITCH_PLAN.md` caveat already says this).
  3. Do not replicate Codex's pre-launch host-login flow (`main/internal/cli/account_auth.go` `ensureCodexAccountReady`, lines 62-99) for Claude. The Claude equivalent of `ensureManagedAccountReady` should be a no-op that returns `nil` until/unless a concrete Claude host-login path is designed.

### 3.3 Account home layout

Mirror Codex exactly:

- Current Codex layout: `~/.valv/providers/codex/profiles/<account>/` — resolved inside `main/internal/services/manage/service.go` `CreateProfile` (lines 102-109) using `filepath.Join(s.providerRoot, string(provider), "profiles", strings.TrimSpace(name))`.
- Claude layout follows the same code path: `~/.valv/providers/claude/profiles/<account>/`. Because `CreateProfile` is provider-parameterized, no code change is needed to get the directory right — it just happens once `ProviderClaude` exists and `DefaultHostProfile` handles `ProviderClaude`.
- Add `main/internal/adapters/providers/claude/profile.go` mirroring `main/internal/adapters/providers/codex/profile.go` (the `DefaultHostProfile(homeDir)` helper returns `(name "default", homePath "<home>/.claude", err)`). Decision note for §7: because Valv runs Claude inside a Linux container and the host `~/.claude` on macOS does not contain credentials (they live in the keychain), the host-default account on macOS is nearly useless for Claude. Consider making Claude's `DefaultHostProfile` return an isolated-first account such as `~/.valv/providers/claude/profiles/default/` instead of `~/.claude`.

### 3.4 CLI surface

- Add `main/internal/cli/claude.go` mirroring `main/internal/cli/codex.go` (verified: 301 lines; key symbols `newCodexCommand`, `runCodexCommand`, `runCodexImageOnlyCommand`, `ensureCodexImageCurrent`, `ensureBoundCodexAccountReady`). Factor common plumbing into shared helpers during the later drop — for the fast path, copy-adapt is acceptable.
- Add `main/internal/services/claude/service.go` mirroring `main/internal/services/codex/service.go` (`Store`, `Executor`, `DetectFunc`, `Options`, `New`, `Run`, `ValidateBinding`, `resolveBinding`, `buildRequest`). `resolveBinding` must assert `profile.Provider == domain.ProviderClaude` and `binding.Provider == domain.ProviderClaude` the same way Codex does at lines 223-236.
- Add `main/internal/adapters/providers/claude/` containing `profile.go`, `account.go`, and `runtime.go` mirroring the Codex equivalents in `main/internal/adapters/providers/codex/`. **Omission note:** the Codex adapter also has `bridge.go` (the MCP host bridge). Claude adapter v1 omits a `bridge.go` equivalent — MCP bridging is Codex-specific for now; revisit if/when Claude MCP config translation is needed.
- Claude `account.go` account-identity contract (v1, container-mount terms): `ReadAccountIdentity(homePath string)` returns `AccountIdentity{LoggedIn: true}` iff the managed account home (which is mounted into the container as `$CLAUDE_CONFIG_DIR`) contains `.credentials.json`. This is a container-mount contract — the managed account home is always a Linux-container-visible directory under `~/.valv/providers/claude/profiles/<account>/`, regardless of which host OS created it. Do not port the Codex JWT parsing; Claude's auth artifact is different. The macOS keychain is mentioned here only as a v1 trade-off note: Valv specifically runs the Claude CLI inside the Linux container so credentials land on disk inside the container home, sidestepping the host keychain.
- Register `claude` in `main/internal/cli/root.go` alongside `codex` in the `runtime` group (line 111 onward): `claudeCmd := newClaudeCommand(paths, nil)` with `claudeCmd.GroupID = "runtime"`, then add it to `cmd.AddCommand(...)`.
- Root command `Example` extension (`main/internal/cli/root.go` lines 55-64): after §2 item 4 removes the `valv api serve` line, this drop (§6.5 builder edit, or §6.6 if `valv account add claude` lands before the launcher) must add at least the following representative Claude examples so the `Short`/`Long`/`Example` mandatory-field rule from `AGENTS.md` §8 lines 204-207 stays satisfied for the new subcommand family: `valv claude --help`, `valv account add claude`, `valv account add claude work`. Order and exact wording left to the builder; the gate is "root `--help` shows at least one Claude example."
- `valv account add claude [name]` — plumbs into `main/internal/cli/manage.go` and `main/internal/services/manage/service.go`. The binding flow is already provider-parameterized; the only changes are provider-gate switches (see §3.1).
- `valv account list` and `valv account switch` — `main/internal/services/manage/service.go` `ListProfiles` and `ListBindings` already take a provider parameter. Extend `manage.go` to list both providers by default when no provider is specified, and keep provider-scoped filtering when one is.

### 3.5 Image update flow (fast path: pinned version)

- Existing structure: `main/internal/services/images/service.go` already accepts a `Provider` field in `Options` (line 57) and a `VersionResolver` (line 49). It hardcodes `NewCodexVersionResolver` as the default when provider is Codex (lines 204-207) and uses the GitHub Releases API for openai/codex (lines 27, 132). The image repo + context dir + Dockerfile are constructor-injected.
- **Fast-path scope (v1):** pin a specific Claude CLI version in the Claude Dockerfile and install it verbatim. No dynamic version resolver in the fast path. Provide `WriteDefaultClaudeContext` and `DefaultClaudeDockerfile` helpers on the images package mirroring the Codex pair. Add `ensureClaudeImageCurrent` in `main/internal/cli/claude.go` that checks the recorded installed version against the pinned version in the Dockerfile (not against a network source) and rebuilds when they diverge. `NewClaudeVersionResolver` is explicitly deferred to §8.
- Construct a separate `images.Service` instance per provider. `main/internal/cli/codex.go` `ensureCodexImageCurrent` (line 238) already shows the pattern: one service, one repo, one Dockerfile. Add the Claude analogue in `main/internal/cli/claude.go`. Because there is no dynamic resolver in v1, wire a trivial pinned-version stub (resolver returns the Dockerfile-pinned constant) where `images.Service.Options` requires a `VersionResolver`, or pass `nil` if the package accepts that for pinned-only use.
- `mage manage update` call path: the manage-update command routes through `openImagesService` (see `main/internal/cli/manage.go`). Extend it to loop over both providers by default and update both images when no provider is specified, and accept `--provider codex|claude` for targeted runs. Under v1 the Claude update path only rebuilds the pinned image; a dynamic "check latest upstream" pass is §8 material.

### 3.6 Project binding

- Verified: `main/internal/project/project.go` is provider-agnostic — it detects a project root from a working directory. No changes needed.
- Verified: `main/internal/domain/model.go` `ProjectBinding` already carries `Provider` (line 32). **But** the SQLite schema in `main/internal/adapters/sqlite/store.go` currently defines `project_bindings` with `project_id TEXT PRIMARY KEY` (lines 57-58) and `UpsertProjectBinding` uses `ON CONFLICT(project_id) DO UPDATE` (lines 310-327), which means: under the new `(provider, account, project)` routing, binding Claude on a project that already has a Codex binding silently overwrites the Codex binding. This is data-destructive.
- Landing a Claude launch slice before the schema migration is therefore not acceptable. The migration ships as a dedicated slice §6.2a **before** §6.5 (Claude launcher) — see §4 and §6.2a below.
- Open question for §7 is now narrow: not "should bindings be per-(project, provider)?" — that is locked in as yes — but "do we need a one-shot data migration for existing Codex-only rows, or is a pure schema swap acceptable given the current user base?" Default assumption: a forward-only migration that converts existing rows to the new composite key with their current `provider` value.

---

## 4. Per-Project Account Binding (Both Providers)

Routing key: `(provider, account, project)`. The `provider` component is already present on `ProjectBinding` but **not** on the SQLite primary key — §6.2a fixes that before the Claude launcher ships, otherwise binding Claude on a Codex-bound project silently overwrites the Codex binding (see §3.6).

Expected behavior:

- `valv codex` from project root A uses binding `(codex, account-X, project-A)`.
- `valv claude` from project root A uses binding `(claude, account-Y, project-A)` (different account from Codex; allowed because provider is part of the key).
- `valv codex` from project root B uses a different `(codex, account-?, project-B)` binding independent from A.
- If a project has no binding for the requested provider, the pass-through launch path returns `ErrUnboundProject` with a remediation hint, the same way `main/internal/services/codex/service.go` does today at lines 210-222. The first-run flow in `main/internal/cli/codex_setup.go` `runCodexFirstRunSetup` (lines 48+) handles binding creation interactively; a symmetric `runClaudeFirstRunSetup` is needed.

`valv account list` treats the two providers uniformly:

- Without `--provider`, list accounts across both providers, grouped by provider.
- With `--provider codex|claude`, filter to that provider.

`valv account switch <name>`:

- Keeps provider implicit when unambiguous. If an account name exists under only one provider, it switches that one. If the name exists under both (rare), require `--provider`.
- "Switch" here means rebind the current project to the named account for the relevant provider, not global home symlinking. Global symlinking is `main/internal/services/globalswitch` and is orthogonal (see §7).

Schema-migration prerequisite: §6.2a must land before §6.5. See §6.2a for the migration scope. Until §6.2a ships, `BindingByProjectID` has a single-binding-per-project contract and cannot safely distinguish Codex from Claude for the same project.

---

## 5. Tests And Gates

Minimum new test coverage for the Claude path:

- `main/internal/adapters/providers/claude/profile_test.go` — `DefaultHostProfile` resolution and `IsDefaultHostHome` equivalence, mirroring `main/internal/adapters/providers/codex/account_test.go` / `profile.go` coverage.
- `main/internal/adapters/providers/claude/runtime_test.go` — `PrepareRuntime` generates correct env (`CLAUDE_CONFIG_DIR`), correct mount target (`/home/valv/.claude`), no Codex-specific config-translation logic bleeds in. Follow the Codex pattern in `main/internal/adapters/providers/codex/runtime_test.go`.
- `main/internal/services/claude/service_test.go` — `Run`, `ValidateBinding`, `resolveBinding` correctness including the provider-mismatch guard (binding/profile must be `ProviderClaude`).
- `main/internal/services/images/service_test.go` — extend with a Claude pinned-version fake. The existing table-driven structure already supports multi-provider easily.
- `main/internal/cli/claude_test.go` — mirror `main/internal/cli/codex_test.go`: help pass-through, `--version` pass-through, unbound-project error, `claude` subcommands forwarded verbatim.
- `main/internal/cli/manage_test.go` — extend to cover `valv account add claude [name]`, `valv account list` with both providers present, and `valv account switch` when only one provider matches.
- `main/internal/adapters/sqlite/store_test.go` — new coverage for the composite-key `project_bindings` schema in §6.2a: two bindings on the same `project_id` with different `provider` values coexist; `BindingByProjectID(projectID, provider)` returns the right one; the forward-only migration converts a pre-existing Codex-only row to the composite shape without data loss.
- Integration: `main/internal/cli/codex_integration_test.go` already exists (~15k bytes, 492 lines). Add a sibling `claude_integration_test.go` that exercises `valv claude --version` / `valv claude --help` through a real Docker subprocess with a minimal image. Keep build-tag `//go:build integration` to stay out of default `mage test`.
- Coverage gate: 70% per package (enforced by `main/magefile.go` `coverageThreshold = 70.0` at line 23). Plan for this up front in the services/claude and providers/claude packages.

Mage targets to add/update:

- No new top-level mage targets. The existing `Test`, `TestPkg`, `Integration`, `Build`, `Run`, `Dev.Run` targets all operate on `./...` or accept a pattern; new packages are picked up automatically.
- Verify after the drop: `mage build` produces a binary with `valv codex` and `valv claude` groups visible under `valv --help`, and no `valv api` group.

Gate for "done":

- `mage test` green (70% per package).
- `mage integration` green including the new Claude integration test.
- `mage build` produces a binary that supports `valv codex ...` and `valv claude ...` end-to-end from two bound accounts on a local macOS Docker Desktop.
- `gh run watch` green on the pushed branch.

---

## 6. Rollout Order

Small slices. Each is one PR. Ordered to minimize rebase pain and keep every slice boundary compilable.

1. **6.1 Delete API wrapper + docs.** All of §2 (items 1-16). No new features. No Claude work. Goal: `valv api` is gone, tests are green. This drop is purely subtractive. Builder must remove every line in §2 item 5 together (import, helper, dangling `var _` reference) — partial removal breaks compile.
2. **6.2 Extend `Provider` type + domain plumbing (compile-safe stubs).** Add `ProviderClaude` to `main/internal/domain/types.go`, update `ParseProvider`, and add `case domain.ProviderClaude` branches in `main/internal/cli/account_auth.go` (no-op returns are fine for v1). `main/internal/services/manage/service.go` `DefaultHostProfile`'s `ProviderClaude` branch returns a stub error (`errors.New("claude host profile not yet available")` or equivalent sentinel) — it does **not** import `claudeprovider` yet because that package does not exist until §6.4. Extend `main/internal/cli/manage.go` `allProviders` list. No Docker, no CLI `claude` command yet. Ship alone so the domain change is a reviewable unit.
3. **6.2a Project binding schema migration.** Dedicated slice landing before any Claude launch path. Scope:
   - Rewrite the `project_bindings` table definition in `main/internal/adapters/sqlite/store.go` (lines 57-65) to use composite primary key `PRIMARY KEY (project_id, provider)`.
   - Rewrite `UpsertProjectBinding` (lines 310-327) so the `ON CONFLICT` target is `(project_id, provider)`, not `(project_id)`.
   - Update `BindingByProjectID` (verified at `main/internal/adapters/sqlite/store.go:330`, current signature `BindingByProjectID(ctx context.Context, projectID string) (domain.ProjectBinding, error)`) — add a `provider domain.Provider` parameter and filter the SELECT on both `project_id` and `provider`. Prerequisite: change the interface declaration first. Specifically:
     - `main/internal/domain/repository.go:20-24` defines `BindingRepository`; line 22 is `BindingByProjectID(context.Context, string) (ProjectBinding, error)`. This is the interface signature change and must land first in the slice — without it, the `sqlite.Store` method signature cannot satisfy `BindingRepository` and every consumer-side interface definition that mirrors it goes out of sync.
     - After the interface update, thread the provider argument through every call site: `main/internal/services/codex/service.go`, `main/internal/services/manage/service.go`, `main/internal/cli/codex_setup.go`, and related test doubles (mocks / fakes implementing `BindingRepository`). Because no Claude launch path exists yet at this slice, every call site passes `domain.ProviderCodex` explicitly — that is fine; §6.5 changes nothing further on the store side.
   - Forward-only data migration, mechanics nailed down:
     - Fact: SQLite cannot `ALTER TABLE` to drop, add, or change a `PRIMARY KEY` in place. The only supported path is the copy-and-rename pattern — `CREATE TABLE project_bindings_new (... PRIMARY KEY (project_id, provider) ...)`, `INSERT INTO project_bindings_new SELECT project_id, profile_id, provider, created_at, modified_at FROM project_bindings` (every existing row already carries `provider`; no `COALESCE` needed, but the `INSERT` explicitly defaults `provider` to `ProviderCodex` for any legacy row where the column is empty or NULL), `DROP TABLE project_bindings`, `ALTER TABLE project_bindings_new RENAME TO project_bindings`. Recreate any needed indexes after the rename.
     - Fact: `main/internal/adapters/sqlite/open.go` (73 lines, full read verified) and `main/internal/adapters/sqlite/store.go` `Bootstrap` (lines 40-111) contain NO schema-version mechanism today — no `schema_version` table, no `PRAGMA user_version` read, only `CREATE TABLE IF NOT EXISTS` plus the one `foreign_keys(1)` pragma in `open.go`. Dropping `ALTER TABLE` statements into the `statements` slice will no-op on existing DBs (the tables already exist under the old shape), so §6.2a must introduce a version mechanism as part of this slice.
     - Mechanism: use `PRAGMA user_version` (single integer in the DB header; no extra table). In `Bootstrap`, before `BeginTx`, read the current `user_version` on the raw DB handle; if `< 1`, perform the rebuild as follows: SQLite documents `PRAGMA foreign_keys` as a no-op inside an open transaction, so any FK toggle must bracket the transaction, not sit inside it. Grep-verified there are **no inbound FKs to `project_bindings`** (the `runtimes` table references `projects(id)` and `profiles(id)` only — nothing points at `project_bindings`), so the FK toggle is not functionally required for this migration. Recommended shape: keep connection-level `foreign_keys = ON` as set in `open.go`, skip the FK toggle, and run the copy-rename rebuild inside a single `BeginTx` block (CREATE new → INSERT SELECT → DROP old → ALTER RENAME → `PRAGMA user_version = 1`) so the rebuild commits atomically. Second startup reads `user_version = 1` and skips the rebuild. If a future schema change adds an inbound FK to `project_bindings`, the bracketing toggle must be reintroduced outside the transaction at that point — note this in the migration comment so the invariant is not lost.
     - Guarantee: the rebuild runs exactly once per DB (version-guarded), and every pre-existing Codex binding survives with `Provider = ProviderCodex` populated explicitly. Zero data loss for existing Codex rows is a hard requirement and is the subject of the migration unit test in §5.
   - Unit tests in `main/internal/adapters/sqlite/store_test.go` per §5 must prove (a) two bindings with the same `project_id` and different `provider` coexist, (b) the `BindingByProjectID(projectID, provider)` call resolves each correctly, (c) migration from a Codex-only legacy row preserves `profile_id` and `created_at`.
   - Why this slice is not optional: without it, §6.5 would silently overwrite the Codex binding on a project the moment the user binds Claude. That is data-destructive, and the plan treats data destruction as blocking.
4. **6.3 Claude Docker image recipe + `images.Service` provider dispatch (pinned version, fast path).** Add `DefaultClaudeDockerfile` (Claude CLI pinned to a specific version — version choice routed in §7) and `WriteDefaultClaudeContext` to `main/internal/services/images/service.go`. Wire the pinned-version path through `main/internal/cli/claude.go` `ensureClaudeImageCurrent` (added in §6.5) and teach `mage manage update` (via `main/internal/cli/manage.go`) to update the Claude image when requested. **No `NewClaudeVersionResolver` in this slice** — dynamic resolution is §8 backlog. No launch path yet; this slice only builds an image and records pinned-version state. Smoke test: `valv manage update --provider claude` produces `valv-claude:dev` on the host at the pinned version.
5. **6.4 Claude provider adapter.** Add `main/internal/adapters/providers/claude/{profile.go, account.go, runtime.go}` mirroring the Codex adapter. Unit-test in isolation. Flip the §6.2 `ProviderClaude` stub in `main/internal/services/manage/service.go` `DefaultHostProfile` from the sentinel error to the real `claudeprovider.DefaultHostProfile(homeDir)` call in this slice. No CLI wiring, no launcher. `bridge.go` intentionally not ported; see §3.4.
6. **6.5 `services/claude` service + `valv claude` CLI pass-through.** Add `main/internal/services/claude/service.go`, add `main/internal/cli/claude.go`, wire `newClaudeCommand` into `main/internal/cli/root.go` `runtime` group. Add Claude examples to the root command `Example` string per §3.4 (`valv claude --help`, `valv account add claude`, `valv account add claude work`). End-to-end: `valv claude --help` works, `valv claude` inside a bound project root launches the CLI with the correct account home mounted. This is the slice where the product becomes demonstrable. Safe to ship only because §6.2a already landed — `(project, provider)` composite key prevents the Claude binding from overwriting an existing Codex binding.
7. **6.6 `valv account add claude` + manage/TUI surface parity.** Extend `main/internal/cli/manage.go` and `main/internal/services/manage/service.go` so `valv account add claude [name]` works, and `valv account list` / `valv account switch` handle both providers uniformly. Extend `main/internal/tui/manage/picker.go` goldens to cover Claude. Reconsider `main/internal/services/globalswitch/` scope here (§7 unknown).
8. **6.7 End-to-end smoke + doc updates.** Add `main/internal/cli/claude_integration_test.go`. Rewrite `main/README.md` around "two providers, pass-through only." Scrub residual OpenAI-compat references from `main/AGENTS.md` and `main/valv_architecture_notes.md` per §2 items 10-12. Tag this slice as the v1-Claude milestone.

Slice-boundary compile guarantee: §6.1 → §6.2 → §6.2a → §6.3 → §6.4 → §6.5 → §6.6 → §6.7. Every boundary compiles: §6.2 stubs the Claude `DefaultHostProfile` branch with a sentinel error (no missing import), §6.2a rewrites the schema and threads the provider argument through every existing call site with `ProviderCodex` passed explicitly (no Claude call site yet), §6.4 introduces `claudeprovider.DefaultHostProfile` and §6.4's builder update flips the §6.2 stub to the real call in the same slice (single PR, not cross-slice). §6.5 is the first slice where `valv claude` is runnable end-to-end.

---

## 7. Unknowns, ToS Design Notes, And Explicit Decisions Needed From The Dev

### ToS / acceptable-use design constraints (documentation / design hygiene; not blockers, but must be recorded before the Claude launcher lands)

- **Anthropic "ordinary, individual use" language.** Anthropic's usage policies and subscription terms frame the Pro, Max, Team, and Enterprise plans as being for ordinary, individual human use by the subscribed user. Valv must document this constraint in user-facing help for `valv claude` and must not position itself as account-sharing or account-proxying infrastructure. A Valv binding ties one Claude account to one human operator.
- **One human per bound Claude account.** Explicit design constraint: a Valv-managed Claude account is not meant to be redistributed across multiple humans. Multi-user account sharing is out of scope and contrary to the plan provider's terms.
- **`CLAUDE_CODE_OAUTH_TOKEN` as the Anthropic-blessed headless path.** The v1 fast path uses device-code auth inside the container (see §3.2), which is appropriate for an interactive CLI. For fully headless, non-interactive use cases, Anthropic's documented path is the `CLAUDE_CODE_OAUTH_TOKEN` environment variable (inference-only, scoped) generated by the `claude` CLI. Record this as an alternative / future consideration; Valv does not need to wire it in v1, but the option should be visible in the plan so the dev knows the interactive device-code flow is not the only sanctioned path.

### Open unknowns needing a dev decision

1. **Claude CLI source of truth + pinned version choice.** Is the Claude Code CLI installed from npm (`@anthropic-ai/claude-code`) or a different channel? For the v1 pinned-version fast path (§3.5, §6.3), which specific version should be baked into the Dockerfile? Without an answer, the §6.3 Dockerfile is blocked on a guess. Recommend: verify via Context7 `/anthropics/claude-code` or official claude.com docs during §6.3; if still ambiguous, pin a recent stable release and document the version in the Dockerfile comment.
2. **Claude on macOS host auth.** `VALV_ACCOUNT_SWITCH_PLAN.md` already flags the macOS keychain observation. Is the v1 stance "containerized-only device-code auth, no macOS host login path" acceptable? If yes, §3.2 stands. If no, a separate design pass for keychain-aware account rotation is required and §6.5 grows.
3. **`valv api` removal staging.** §2 groups deletion of `internal/api/openai`, `internal/services/openaiapi`, `internal/cli/api.go`, `compatibility.go`, `compatibility_test.go`, and `codex-openai-compatibility.json` in one drop. Is that acceptable, or split per-package (one PR per folder) for easier review/revert?
4. **`internal/services/globalswitch/` disposition.** Verified code is Codex-only (`main/internal/services/globalswitch/service.go` line 86 rejects non-Codex). Options: (a) extend to Claude now in 6.6; (b) leave Codex-only and flag as a known gap; (c) delete entirely because it mutates global `~/.codex` in place, which runs counter to `VALV_ACCOUNT_SWITCH_PLAN.md` "Avoid mutating global home credentials in place as the primary model" (lines 152-156). Dev choice needed.
5. **Binding migration style for §6.2a.** The schema migration is locked in as required (§3.6, §6.2a). The open choice is how to execute it: (a) bootstrap-time schema-version bump with a one-shot rebuild of `project_bindings`, or (b) destructive recreate with warning (acceptable only if no user data exists yet). Recommend (a); confirm acceptable or specify (b) if the user base is still empty.
6. **Account-name collision across providers.** If `valv account add claude work` and `valv account add codex work` both create an account named `work`, is `valv account switch work` ambiguous? Plan default in §4: yes, require `--provider` when ambiguous. Confirm or override.
7. **Host-default Claude account on macOS.** §3.3 raises this: `~/.claude` on macOS does not hold credentials. Should `DefaultHostProfile(ProviderClaude)` return `~/.claude` for symmetry with Codex, or an isolated Valv-managed path? Default the plan chooses the former for symmetry and documents the container-mount rationale in help text; dev can override.

---

## 8. Later Drop — Cleanup, Idiomatic Go, Refactor Backlog

Not in scope for the fast path. Catalogued so nothing is forgotten. All file paths verified to exist.

### Dynamic version resolution for Claude

- **`NewClaudeVersionResolver` dynamic resolver.** Fast path (§3.5, §6.3) pins a Claude CLI version in the Dockerfile. A dynamic resolver — whether via npm registry (`@anthropic-ai/claude-code` dist-tags), an Anthropic release endpoint, or a bundled "pinned latest" refresh script — is a separate design and implementation effort. Pattern to mirror: `NewCodexVersionResolver` in `main/internal/services/images/service.go` (GitHub Releases API for openai/codex). (Effort: M.)

### Error handling

- **`main/internal/cli/api.go` has clean `%w` wrapping but the file is going away.** No action.
- **Vague `fmt.Errorf` without `%w` in launcher paths.** Audit `main/internal/cli/codex.go` lines 59-128 for places the inner error is already `%w`-chained — it mostly is. Cross-check at rewrite time. (Effort: S — audit only; likely no changes.)
- **Bridge/MCP translation error swallowing.** `main/internal/adapters/providers/codex/runtime.go` `translateMCPServers` (lines 360-420) collects warnings into a `[]string` and keeps going; one of them uses `fmt.Sprintf("%s: %v", name, err)` losing the underlying error chain. Rationale: warnings are user-surfaced and error identity matters for debugging. (Effort: S.)
- **Silent bridge shutdown errors.** `main/internal/adapters/providers/codex/runtime.go` `cleanup` joins errors via `errors.Join` (line 141) — good. Verify no caller discards the resulting joined error. (Effort: S.)

### Interface placement

- **Consumer-side interfaces.** `main/internal/services/codex/service.go` declares `Store` and `Executor` interfaces at lines 23-34 next to their consumer — good, matches `AGENTS.md` §5. But `main/internal/services/images/service.go` declares `Runner` (line 37), `outputRunner` (line 41), `StateStore` (line 45), `VersionResolver` (line 49) — `Runner` and `outputRunner` are both near the consumer, `StateStore` depends on `domain.ProviderImageRepository` which lives in `main/internal/domain/repository.go`. Decide whether domain-level repositories belong in `domain` or should move into their respective service packages. (Effort: M.)
- **`main/internal/domain/repository.go` is a shared dumping ground for all repository interfaces.** Opposite of `AGENTS.md` §5 guidance. Candidate refactor: move each repository interface to the package that consumes it. Watch for circular imports via `main/internal/adapters/sqlite/store.go`. (Effort: L.)

### Package structure smells

- **`main/internal/cli/manage.go` is 45k / ~1100 lines.** Dumping ground — `account add`, `account list`, `account switch`, `manage update`, `manage status`, `manage cleanup`, all in one file. Split by command group. (Effort: M.)
- **`main/internal/cli/extended_test.go` is 38k.** Test dumping ground. Split by command-under-test. (Effort: M.)
- **`main/compatibility.go` and `main/compatibility_test.go` at the module root.** Going away in §2. No cleanup action.
- **`main/internal/adapters/providers/codex/` is internally structured** — keep as the template for `claude/`. (Effort: N/A.)
- **Duplicated `terminalEnvPassthrough` in Claude adapter.** §3.2 locks in duplicating the helper into `main/internal/adapters/providers/claude/runtime.go` for v1. Dedupe into a shared adapter-level helper here. (Effort: S.)

### context.Context threading

- **Magefile subprocess threading.** `main/magefile.go` uses `exec.Command` without contexts (lines 362, 378, 413). Mage targets are short-lived CLI operations; probably acceptable. Flag anyway. (Effort: S.)
- **`main/internal/services/globalswitch/service.go` `processRunning`** builds an `exec.CommandContext` correctly (line 178). Good.
- **`main/internal/services/images/service.go` HTTP client** uses `http.NewRequestWithContext` (line 136). Good.
- **No obvious context threading gaps** in the code surfaces read. Low priority. (Effort: S — audit only.)

### Dead code after API removal

- After §2 lands, grep for orphan imports and helpers: `openaiapi`, `openaihandler`, `ChatCompletionsPath`, `RequestError`, anything referencing `ReasoningEffort` that only fed the OpenAI layer. Should surface during compile but list explicitly. (Effort: S — falls out of §2 gate.)
- **`main/internal/cli/operator_helpers.go` lines 17, 25, 107-128, 315** (the `newOpenAIAPIService` function, its imports, and the dangling `var _ = openaihandler.ChatCompletionsPath` line) — covered by §2 item 5. No additional action.

### Test quality

- **Mock-heavy unit tests.** `AGENTS.md` §11 says "do not default to mock-heavy unit tests for runtime, provider, storage, Docker, or CLI launch-path behavior." `main/internal/cli/extended_test.go` has the `apiServeStubService` type (going away) and similar stub-heavy patterns for the codex launch path. Audit after §2 lands. (Effort: M.)
- **Integration coverage for `valv claude` pass-through.** Only noted — 6.7 adds it.

### Naming / API ergonomics

- **`Profile` vs `Account` terminology.** `AGENTS.md` (lines 182-186) says user-facing surfaces should prefer `account`, internal storage may keep `Profile`. Current internal code still uses `Profile` heavily in service signatures (`ProfileByName`, `ListProfiles`). This is fine per the rule, but the mixed prose is confusing. Consider a later rename pass to make the boundary explicit via wrapper types. (Effort: L.)
- **`Provider` as a string-typed enum.** `main/internal/domain/types.go` uses `type Provider string` plus constants. Stringly-typed; easy to typo. Consider a tagged struct or at minimum a `Provider.Validate()` helper to centralize validity checks. (Effort: S.)

### TUI / CLI consistency

- **`main/internal/tui/manage/picker.go` currently provider-parameterized.** Good. Extend goldens for the Claude case in 6.6.
- **`--provider` flag consistency.** Several commands in `main/internal/cli/manage.go` use `parseOptionalProvider` and default to `ProviderCodex`. After 6.2, defaulting should be explicit-fail when ambiguous, not silent fallback to Codex. (Effort: S.)

### Violations of stated rules

- **`AGENTS.md` §5 "keep interfaces near the consumer"** — partially violated by `main/internal/domain/repository.go` (see above). (Effort: L, same as repository-move item.)
- **`AGENTS.md` §11 "prefer real end-to-end and integration tests over mocks"** — partially violated by mock-heavy CLI tests (see above). (Effort: M.)
- **`AGENTS.md` §12 "keep CI and local command recipes aligned"** — `main/magefile.go` and `.github/workflows/ci.yml` not read in this pass. Verify alignment in a later doc/CI sweep. (Effort: S.)
- **`AGENTS.md` §5 "prefer explicit constructors over package-global mutable state"** — `main/internal/cli/codex.go` uses `var findDockerBinary = exec.LookPath` (line 24) and `main/internal/cli/api.go` `var openAIAPIServiceFactory = ...` — package-level vars for dependency injection in tests. Idiomatic-enough today, but a cleaner constructor-threaded pattern would remove the globals. (Effort: M.)

---

## 9. References

Files read to produce this plan (all verified to exist):

- `main/README.md`
- `main/AGENTS.md`
- `main/CONTRIBUTING.md`
- `main/PLAN.md` (partial, API-relevant grep + opening lines)
- `main/VALV_REPO_PLAN.md`
- `main/VALV_ACCOUNT_SWITCH_PLAN.md`
- `main/valv_architecture_notes.md`
- `main/API_COMPAT_EXECUTION_PLAN.md`
- `main/compatibility.go`
- `main/magefile.go`
- `main/internal/domain/types.go`
- `main/internal/domain/model.go`
- `main/internal/cli/root.go`
- `main/internal/cli/api.go`
- `main/internal/cli/codex.go`
- `main/internal/cli/codex_setup.go`
- `main/internal/cli/account_auth.go`
- `main/internal/cli/operator_helpers.go`
- `main/internal/cli/extended_test.go` (grep-only)
- `main/internal/cli/manage.go` (grep-only)
- `main/internal/cli/global.go` (grep-only)
- `main/internal/services/codex/service.go`
- `main/internal/services/manage/service.go`
- `main/internal/services/images/service.go`
- `main/internal/services/globalswitch/service.go`
- `main/internal/services/cleanup/service.go` (partial)
- `main/internal/adapters/providers/codex/profile.go`
- `main/internal/adapters/providers/codex/account.go`
- `main/internal/adapters/providers/codex/runtime.go`
- `main/internal/adapters/sqlite/store.go`
- `main/internal/tui/manage/picker.go` (grep-only)

Directory listings verified for:

- `main/`
- `main/cmd/valv/`
- `main/internal/`
- `main/internal/api/openai/`
- `main/internal/services/openaiapi/`
- `main/internal/services/codex/`
- `main/internal/services/images/`
- `main/internal/services/manage/`
- `main/internal/services/globalswitch/`
- `main/internal/services/cleanup/`
- `main/internal/adapters/docker/`
- `main/internal/adapters/providers/codex/`
- `main/internal/adapters/sqlite/`
- `main/internal/domain/`
- `main/internal/config/`
- `main/internal/project/`
- `main/internal/tui/`
- `main/internal/cli/`
