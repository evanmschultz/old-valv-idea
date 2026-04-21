# DROP_4 — CLAUDE DOCKER IMAGE

**State:** building
**Blocked by:** DROP_3 (done)
**Paths (expected):** `internal/services/images/` (edit — add Claude Dockerfile + context writer + constructor-tolerant resolver handling), `internal/cli/` (edit — `claudeImageRepository` / `claudeImageTag` helpers, extend `openImagesService` to dispatch on provider, wire `valv manage update --provider claude`), tests alongside each
**Packages (expected):** `internal/services/images` (add `DefaultClaudeDockerfile` + `WriteDefaultClaudeContext` + make `Resolver` optional when the caller supplies a pinned version), `internal/cli` (provider-dispatch in `openImagesService` + Claude repository/tag helpers + `runManageUpdate` Claude branch)
**PLAN.md ref:** main/PLAN.md → DROP_4_CLAUDE_DOCKER_IMAGE row
**Workflow:** main/drops/WORKFLOW.md
**Started:** 2026-04-20
**Closed:** —

## Scope

Add `DefaultClaudeDockerfile` plus `WriteDefaultClaudeContext` with a pinned Claude CLI version, wire `images.Service` provider dispatch, and teach `valv manage update --provider claude` to produce `valv-claude:dev` per `VALV_CLAUDE_CODE_FOCUS_PLAN.md` §6.3. MVP scope — image build only; no Claude provider adapter, no service, no `valv claude` CLI (those are DROP_5 and DROP_6).

## Planner

### Scope confirmation

In scope (MVP image-build surface only):

- New `DefaultClaudeDockerfile()` in `internal/services/images` — mirrors `DefaultCodexDockerfile()` shape (Debian slim, Node 22, uid/gid, npm global install of pinned Claude CLI).
- New `WriteDefaultClaudeContext(root string) (string, error)` in the same package — mirrors `WriteDefaultCodexContext`.
- `images.Service` provider dispatch: `New(Options)` currently only auto-assigns `NewCodexVersionResolver` when `provider == ProviderCodex` and a resolver is nil (service.go:204-207). For v1 Claude the plan pins the version in the Dockerfile with no dynamic resolver (`NewClaudeVersionResolver` is deferred to §8 per focus-plan). The service currently requires a resolver for `EnsureLatest` but NOT for `Build` — so Claude's v1 path uses `Build(...)` with the pinned version rather than `EnsureLatest(...)`. No changes to the core service state machine; only a new constructor-side relaxation so "provider=claude + resolver=nil" is accepted (today `New` silently leaves `resolver=nil` for non-Codex, which is already correct — verify no guard rejects it).
- `internal/cli` Claude image helpers (`claudeImageRepository`, `claudeImageTag`, `claudeImageRef`) mirroring the Codex trio at `codex.go:272-297`.
- `openImagesService` (`operator_helpers.go:68-95`) refactored to take a `domain.Provider` and dispatch on it: Codex path unchanged (still uses `WriteDefaultCodexContext` + `codexVersionResolverFactory`), Claude path writes the Claude context dir and passes `Resolver: nil`, `Repository: claudeImageRepository()`, `DefaultTag: claudeImageTag()`, `Provider: domain.ProviderClaude`.
- `runManageUpdate` (`manage.go:1082-1119`): remove the `provider != domain.ProviderCodex` early-return guard; for Claude, call the Build path with the Dockerfile-pinned version constant rather than `EnsureLatest`.
- New `DefaultClaudeCLIVersion` constant in `internal/services/images` holding the pin (proposed: `2.1.89`, the latest stable tag Context7 `/anthropics/claude-code` returns today 2026-04-21). Both the Dockerfile `ARG` resolution and `runManageUpdate`'s Claude branch read from this constant so the "pinned" claim has one source of truth.

Explicitly deferred (NOT in DROP_4):

- `NewClaudeVersionResolver` — dynamic upstream resolution. §8 backlog per focus-plan §3.5.
- `internal/adapters/providers/claude/` — profile/account/runtime adapter. DROP_5.
- `internal/services/claude/service.go` + `internal/cli/claude.go` — Claude launch service and pass-through CLI. DROP_6.
- `valv account add claude` / `valv account list` / account-switch — DROP_7 per focus-plan §6.6/§6.7.
- Any edits under `internal/adapters/docker/` — the Codex counterpart never used that package for its Dockerfile constant (the PLAN.md container row's "expected paths" note was wrong; Codex's Dockerfile/context writer live in `internal/services/images`, not `internal/adapters/docker`). Mirror that shape, do not invent a new docker-adapter package.
- TUI home `ActionUpdate` dispatch (`internal/cli/operator_helpers.go:122-127`) remains Codex-hardcoded after DROP_4. The MVP Claude update path is `valv manage update claude` via cobra; a provider-select TUI integration is follow-up scope — not required for focus-plan §6.3.

### Committed-state audit

Evidence verified via Hylla `hylla_search_keyword`, Grep for line numbers, and direct Read of `internal/services/images/service.go`, `internal/cli/operator_helpers.go`, `internal/cli/codex.go`, `internal/cli/manage.go`, and `internal/services/manage/service.go`. Line numbers current 2026-04-21 against `main` HEAD.

- `DefaultCodexDockerfile()` lives in `internal/services/images/service.go:518-548`. Returns a single-line-trimmed string with trailing `\n`. Base image `node:22-bookworm-slim`; `ARG VALV_UID=1000` / `VALV_GID=1000`; `apt-get install -y --no-install-recommends bubblewrap ca-certificates git ncurses-term`; `getent group` + `useradd -o -m -u … -g … -s /bin/sh valv`; creates `/home/valv/.codex /workspace` and chowns; sets `NPM_CONFIG_UPDATE_NOTIFIER=false` / `_FUND=false` / `_AUDIT=false` + `HOME=/home/valv` + `LOGNAME=valv` + `USER=valv`; `ARG CODEX_VERSION` + `RUN npm install --global "@openai/codex@${CODEX_VERSION}"`; `USER valv`; `WORKDIR /workspace`; `ENTRYPOINT ["codex"]`.
- `WriteDefaultCodexContext(root string) (string, error)` at `service.go:503-516`. `os.MkdirAll(root, 0o755)`, writes `filepath.Join(root, "Dockerfile")` with `0o644`, returns the dockerfile path.
- `images.Service.New` at `service.go:173-221`. Guards: `Runner` required, `Repository` required, `ContextDir` required. Defaults: `Dockerfile` → `"Dockerfile"`, `DefaultTag` → `"dev"`, `UserID`/`GroupID` → current, `Provider` → `ProviderCodex` when empty, `Resolver` auto-assigned to `NewCodexVersionResolver(nil)` only when provider is Codex AND resolver is nil (service.go:204-207). For any other provider with `Resolver == nil`, the service stores `resolver: nil` without error — matches Claude v1 need.
- `Service.Build` (service.go:223-291) does NOT touch `s.resolver` — safe for pinned Claude path. `Service.EnsureLatest` (service.go:293) explicitly returns `"ensure latest image: latest-version resolver is required"` at service.go:294-296 when resolver is nil — so Claude v1 must NOT go through `EnsureLatest`.
- `openImagesService` at `internal/cli/operator_helpers.go:68-95`. Hardcoded Codex today: `contextDir := filepath.Join(paths.BuildCacheDir, string(domain.ProviderCodex))`, calls `imagesservice.WriteDefaultCodexContext`, passes `Resolver: codexVersionResolverFactory(nil)` (`var codexVersionResolverFactory = imagesservice.NewCodexVersionResolver` at operator_helpers.go:26-29, kept as a function pointer for tests), `Repository: codexImageRepository()`, `DefaultTag: codexImageTag()`. No `Provider:` field set explicitly (defaults to `ProviderCodex` via `New`).
- `codexImageRepository()` / `codexImageTag()` / `codexImageRef()` at `internal/cli/codex.go:272-297`. Honors `VALV_CODEX_IMAGE` env override; falls back to `("valv-codex", "dev")`. Claude equivalent reads `VALV_CLAUDE_IMAGE` with fallback `("valv-claude", "dev")`.
- `runManageUpdate` at `internal/cli/manage.go:1082-1119`. Line 1083-1085: `if provider != domain.ProviderCodex { return fmt.Errorf("manage update: provider %q is not supported yet", provider) }`. Calls `openImagesService(cmd, paths)` (which is Codex-hardcoded) then `service.EnsureLatest(...)`. For Claude, must (a) stop rejecting, (b) pass the provider into `openImagesService`, (c) use `Build` with the pinned version constant instead of `EnsureLatest` because there is no resolver.
- `newManageUpdateCommand` at `manage.go:1051-1080`. `Args: cobra.MaximumNArgs(1)`; `parseOptionalProvider(args, domain.ProviderCodex)` — accepts `valv manage update claude` as the positional form today. Focus-plan §6.3 phrasing "`valv manage update --provider claude`" is shorthand; the existing positional shape is what the builder should target. Builder should extend `Example:` (manage.go:1067-1069) to include `valv manage update claude`.
- `manage.supportedProviders` at `internal/cli/manage.go:984` already returns `[]domain.Provider{domain.ProviderCodex, domain.ProviderClaude}` (DROP_2 added). No change needed here.
- `parseOptionalProvider` (`operator_helpers.go:274-283`) already accepts "claude" — DROP_2 extended `domain.ParseProvider` at `internal/domain/types.go:14-21`. Verified via Grep: `types.go:19 case ProviderClaude`, `types_test.go:87-88` covers `claude` / ` Claude `.
- `internal/cli/extended_test.go:448` references `filepath.Join(paths.BuildCacheDir, string(domain.ProviderCodex), "Dockerfile")` in an existing Codex-only `openImagesService` fixture — no change forced on Codex tests, but the refactor to provider-dispatch MUST keep this path working. Verified test targets Codex explicitly, so Codex-branch integrity is the sole compatibility contract here.
- `internal/cli/extended_test.go:843` computes `sha256.Sum256([]byte(imagesservice.DefaultCodexDockerfile()))` as `fakeCodexRecipeHash`. No Claude-side test fixture exists yet; the builder adds one as part of the unit tests in 4.1.
- Claude CLI source-of-truth verification — Context7 `/anthropics/claude-code` (resolve + query verified 2026-04-21): npm package `@anthropic-ai/claude-code`, installable via `npm install -g @anthropic-ai/claude-code`. Context7 lists `v2.1.39` and `v2.1.89` among its tagged versions; `v2.1.89` is the most recent stable tag and is the proposed pin. Builder MUST re-verify the pin at build time — the pin is a choice, not load-bearing on the decomposition. If Context7's tag listing moves ahead before DROP_4 ships, builder pins whatever is current-stable at build time and documents the choice in `BUILDER_WORKLOG.md`.
- Target mount path per focus-plan §3.2: inside the container the Claude config dir is `/home/valv/.claude` (mirror Codex's `/home/valv/.codex`). The Dockerfile in 4.1 creates `/home/valv/.claude` and chowns it.
- focus-plan §3.2 also lists Claude container env: `CLAUDE_CONFIG_DIR=/home/valv/.claude`, `HOME=/home/valv`, `USER=valv`, `LOGNAME=valv`. DROP_4 sets those in the Dockerfile `ENV` block since they are launch-time constants for this container shape; the adapter in DROP_5 will supplement with terminal-passthrough env at runtime.

Evidence for focus-plan references:

- `VALV_CLAUDE_CODE_FOCUS_PLAN.md:109` (§3.2 "New Docker image recipe `valv-claude:dev`…") — sets base image, Node, Claude Code CLI pinned, mirror codex service.go:518-548.
- `VALV_CLAUDE_CODE_FOCUS_PLAN.md:139` (§3.5) — fast-path scope: pin in Dockerfile, no dynamic resolver in v1, add `WriteDefaultClaudeContext` + `DefaultClaudeDockerfile`, separate `images.Service` per provider.
- `VALV_CLAUDE_CODE_FOCUS_PLAN.md:224` (§6.3) — teach `mage manage update` (via `internal/cli/manage.go`) to update the Claude image when requested.

### Atomic decomposition

Package-lock chain: 4.1 (images package) → 4.2 (cli package — operator_helpers refactor) → 4.3 (cli package — manage.go update branch). 4.2 and 4.3 both touch `internal/cli`; they are serialized. 4.1 is on a disjoint package so it could start first but 4.2 and 4.3 both consume 4.1's exported symbols, which enforces the order anyway.

---

#### Unit 4.1 — Claude Dockerfile + context writer + pinned-version constant (images package)

**State:** done
**Paths:** `internal/services/images/service.go` (adds `DefaultClaudeCLIVersion`, `DefaultClaudeDockerfile`, `WriteDefaultClaudeContext`, `providerDockerfileContent` helper on `Service`; edits `recipeHash()` to call the new helper — all changes confined to this single file in the images package), `internal/services/images/service_test.go`, `internal/services/images/service_integration_test.go`
**Packages:** `internal/services/images`
**Blocked by:** —

**Description**

Add to `internal/services/images/service.go`:

1. Package-level constant `DefaultClaudeCLIVersion` holding the pinned Claude CLI version string. Proposed `"2.1.89"` per Context7 `/anthropics/claude-code` at plan time. Builder re-verifies at build time; document any change in `BUILDER_WORKLOG.md`.
2. Function `DefaultClaudeDockerfile() string` mirroring the `DefaultCodexDockerfile()` body at `service.go:518-548` with these differences:
   - `RUN npm install --global "@anthropic-ai/claude-code@${CLAUDE_VERSION}"` (package name per Context7 verification above).
   - `mkdir -p /home/valv/.claude /workspace` + chown the `.claude` path instead of `.codex`.
   - `ENV` block additionally sets `CLAUDE_CONFIG_DIR=/home/valv/.claude` alongside `HOME`/`LOGNAME`/`USER` (per focus-plan §3.2).
   - `ARG CLAUDE_VERSION` (not `CODEX_VERSION`).
   - `ENTRYPOINT ["claude"]`.
   - Everything else (base image, apt packages, valv user creation, NPM_CONFIG env, `USER valv`, `WORKDIR /workspace`) is character-identical to the Codex recipe — the apt package list `bubblewrap ca-certificates git ncurses-term` applies equally to Claude.
3. Function `WriteDefaultClaudeContext(root string) (string, error)` mirroring `WriteDefaultCodexContext` at `service.go:503-516` — same `os.MkdirAll(root, 0o755)` + `os.WriteFile(filepath.Join(root, "Dockerfile"), []byte(DefaultClaudeDockerfile()), 0o644)` semantics, returning the dockerfile path.

4. New unexported helper `providerDockerfileContent() string` on `Service`. Returns `DefaultClaudeDockerfile()` when `s.provider == domain.ProviderClaude`; returns `DefaultCodexDockerfile()` otherwise (default / Codex). Keep the switch provider-keyed so DROP_5+ providers slot in without reopening this logic.
5. Fix `recipeHash()` at `service.go:441-451`. Current implementation seeds `content := DefaultCodexDockerfile()` unconditionally before the custom-Dockerfile fallback, so a Claude build with the default Dockerfile basename silently labels the image with `sha256(DefaultCodexDockerfile())` — a wrong recipe hash that would poison DROP_5+ ensure-latest comparisons. Replace the unconditional Codex seed with a call to the new `providerDockerfileContent()` helper. Preserve the disk-read fallback for custom `s.dockerfile` paths (`filepath.Base(s.dockerfile) != defaultCodexDockerfile` branch) unchanged — it already short-circuits to file content when a user supplies a custom Dockerfile. Resulting shape:
   ```go
   func (s Service) recipeHash() string {
       content := s.providerDockerfileContent()
       if filepath.Base(s.dockerfile) != defaultCodexDockerfile {
           path := filepath.Join(s.contextDir, s.dockerfile)
           if fileContent, err := os.ReadFile(path); err == nil {
               content = string(fileContent)
           }
       }
       sum := sha256.Sum256([]byte(content))
       return hex.EncodeToString(sum[:])
   }
   ```
   Builder may refine (e.g. inline the switch), provided the provider-keyed branching is preserved and verified by the test in the next block.

Add to `internal/services/images/service_test.go`:

- Test `TestWriteDefaultClaudeContextWritesDockerfile` mirroring `TestWriteDefaultCodexContextWritesDockerfile` at `service_test.go:353-384`. Must assert the file contains at minimum: `@anthropic-ai/claude-code@${CLAUDE_VERSION}`, `CLAUDE_CONFIG_DIR=/home/valv/.claude`, `ENTRYPOINT ["claude"]`, `NPM_CONFIG_UPDATE_NOTIFIER=false`, `ARG VALV_UID=1000`, `ARG VALV_GID=1000`, and the shared apt-install / useradd / chown lines already checked for Codex. File basename must be `Dockerfile`.
- Test `TestDefaultClaudeCLIVersionIsNonEmpty` asserting the constant is non-empty and matches the `\d+\.\d+\.\d+` pattern of `versionPattern` (service.go:34) so recipe-hash computation stays stable.
- Test `TestServiceBuildRecipeHashMatchesProviderDockerfile` covering the F1 fix. Table-driven with two cases (`ProviderCodex`, `ProviderClaude`). For each case: construct an `images.Service` via `New(Options{...})` with the appropriate `Provider`, `Repository`, `ContextDir` (writing the provider's default Dockerfile to that dir via the corresponding `WriteDefault*Context`), and a `runner` of type `*runnerRecorder` (existing fake runner at `service_test.go:19` — reuse, do not rewrite). Call `svc.Build(ctx, BuildRequest{Version: <pinned version const>})`, inspect the recorded `buildx build` args, extract the `--label io.valv.recipe_hash=<hex>` argument value, and assert it equals `hex.EncodeToString(sha256.Sum256([]byte(<provider-default-dockerfile-content>)))`. Codex case expects `sha256(DefaultCodexDockerfile())`; Claude case expects `sha256(DefaultClaudeDockerfile())`. No docker daemon required — the fake runner records args in memory.

Add to `internal/services/images/service_integration_test.go` (behind `//go:build integration`):

- Test `TestWriteDefaultClaudeContextBuildsWithExistingUIDAndGID` mirroring the Codex integration test at `service_integration_test.go:56-95` — writes the Claude context, instantiates `images.Service` with `Repository: "valv-test/claude-default"`, `Provider: domain.ProviderClaude`, and calls `svc.Build(ctx, BuildRequest{Version: DefaultClaudeCLIVersion})`, asserting the resulting image inspects cleanly. Cleanup removes the built image with `--force`.

**Acceptance**

- `go doc github.com/evanmschultz/valv/internal/services/images DefaultClaudeDockerfile` returns non-empty.
- `go doc github.com/evanmschultz/valv/internal/services/images WriteDefaultClaudeContext` returns non-empty.
- `go doc github.com/evanmschultz/valv/internal/services/images DefaultClaudeCLIVersion` returns the pinned version string.
- `mage testPkg ./internal/services/images` green (includes gofumpt check + 70% per-package coverage).
- `recipeHash()` returns a provider-keyed hash: for `Provider: domain.ProviderClaude` with the default Dockerfile basename, the returned hash equals `hex(sha256(DefaultClaudeDockerfile()))`; for `Provider: domain.ProviderCodex`, it equals `hex(sha256(DefaultCodexDockerfile()))`. Verified by `TestServiceBuildRecipeHashMatchesProviderDockerfile`.
- `providerDockerfileContent()` helper (unexported) exists on `Service` and is the single source of truth the `recipeHash()` default branch consults — no remaining unconditional `DefaultCodexDockerfile()` seed in `recipeHash()`.
- Search in the generated Dockerfile string for `@anthropic-ai/claude-code@${CLAUDE_VERSION}`, `CLAUDE_CONFIG_DIR=/home/valv/.claude`, and `ENTRYPOINT ["claude"]` — all three present.
- No changes to `DefaultCodexDockerfile` / `WriteDefaultCodexContext` / existing Codex tests — regression protected by `mage testPkg ./internal/services/images` still passing all prior Codex assertions.
- Integration test intentionally gated behind `//go:build integration` so `mage testPkg` runs green on hosts without Docker; `mage integration` exercises the new test when Docker is available.

---

#### Unit 4.2 — `openImagesService` provider dispatch + Claude image helpers (cli package)

**State:** done
**Paths:** `internal/cli/operator_helpers.go`, `internal/cli/codex.go` (refactor-only — new helpers alongside existing Codex ones), `internal/cli/extended_test.go` (augment existing fixtures to cover Claude path), new file `internal/cli/claude_image.go` permitted if builder prefers to keep Claude helpers separate from `codex.go`
**Packages:** `internal/cli`
**Blocked by:** 4.1

**Description**

Refactor `openImagesService` at `internal/cli/operator_helpers.go:68-95` to take a `domain.Provider` argument and dispatch. Keep the zero-argument "Codex default" call sites compiling by either (a) adding a second exported helper `openCodexImagesService` / `openClaudeImagesService` that each delegate to the new provider-dispatching core, or (b) threading `provider` through every existing caller. Evidence of existing callers (Grep for `openImagesService(`):

- `internal/cli/operator_helpers.go:74` is the definition site.
- `internal/cli/manage.go:1090` (`runManageUpdate`).
- `internal/cli/codex.go:242` (`ensureCodexImageCurrent`).

Recommend option (b) — simpler static analysis, matches focus-plan §3.5 "Construct a separate `images.Service` instance per provider." Change the signature to `openImagesService(cmd *cobra.Command, paths config.Paths, provider domain.Provider) (imagesservice.Service, func(), error)`. Each existing call site passes `domain.ProviderCodex` explicitly (codex.go:242, manage.go:1090 for the Codex path).

Inside the new body:

- Compute `contextDir := filepath.Join(paths.BuildCacheDir, string(provider))`.
- `switch provider { case domain.ProviderCodex: … case domain.ProviderClaude: … default: return imagesservice.Service{}, nil, fmt.Errorf("initialize image service: unsupported provider %q", provider) }`.
- Codex case: preserve current behavior exactly — `imagesservice.WriteDefaultCodexContext(contextDir)`, `Resolver: codexVersionResolverFactory(nil)`, `Repository: codexImageRepository()`, `DefaultTag: codexImageTag()`, `Provider: domain.ProviderCodex`.
- Claude case: `imagesservice.WriteDefaultClaudeContext(contextDir)`, `Resolver: nil` (pinned-version fast path; no auto-assignment for Claude inside `images.New`), `Repository: claudeImageRepository()`, `DefaultTag: claudeImageTag()`, `Provider: domain.ProviderClaude`.

Add Claude image helpers (builder choice on file placement — `internal/cli/codex.go` alongside Codex ones, or a new sibling `internal/cli/claude_image.go`):

- `claudeImageRef() dockeradapter.ImageRef` — honors `VALV_CLAUDE_IMAGE` env; fallback `("valv-claude", "dev")`. Body parallels `codexImageRef()` at `codex.go:272-284`.
- `claudeImageRepository() string` — returns `claudeImageRef().Repository`. Parallels `codexImageRepository()` at `codex.go:286-289`.
- `claudeImageTag() string` — returns `claudeImageRef().Tag`, defaulting to `"dev"` when empty. Parallels `codexImageTag()` at `codex.go:291-297`.

Update tests:

- Existing callers in `extended_test.go` that invoke `openImagesService` must pass `domain.ProviderCodex` explicitly — builder fixes compile errors in `extended_test.go` as part of this unit.
- Add `TestOpenImagesServiceClaudeContextWritesDockerfile` in `extended_test.go` (or new `claude_image_test.go`): calls `openImagesService(cmd, paths, domain.ProviderClaude)`, asserts `filepath.Join(paths.BuildCacheDir, "claude", "Dockerfile")` exists and contains the `@anthropic-ai/claude-code` install line. Mirrors the Codex fixture at `extended_test.go:448`.
- Add `TestOpenImagesServiceUnsupportedProvider`: passing `domain.Provider("nonsense")` returns an error matching `"unsupported provider"`.
- Add unit tests for `claudeImageRef` / `claudeImageRepository` / `claudeImageTag` covering the `VALV_CLAUDE_IMAGE` env fallback (mirrors whatever Codex coverage already exists for those helpers).

**Acceptance**

- `openImagesService(cmd, paths, domain.ProviderClaude)` returns a non-zero service and writes `filepath.Join(paths.BuildCacheDir, "claude", "Dockerfile")` with content matching `DefaultClaudeDockerfile()`.
- `openImagesService(cmd, paths, domain.ProviderCodex)` preserves every existing behavior — Codex regression suite (`TestOpenImagesService*`, `ensureCodexImageCurrent` coverage) stays green with no changes to expected outputs.
- `go doc github.com/evanmschultz/valv/internal/cli claudeImageRepository` returns non-empty (unexported in Go source, so use `grep -n 'func claudeImageRepository' internal/cli/*.go` for existence instead; `go doc` does not list unexported). Builder evidence in worklog: cite the file:line of the new helpers.
- `mage testPkg ./internal/cli` green (gofumpt + 70% coverage + includes all new Claude path assertions AND preserves Codex regressions).
- No build-cache-dir collisions: Codex writes to `…/codex/Dockerfile`, Claude writes to `…/claude/Dockerfile` — verified by two distinct `filepath.Join` results.

---

#### Unit 4.3 — `valv manage update` Claude provider branch (cli package)

**State:** todo
**Paths:** `internal/cli/manage.go`, `internal/cli/manage_test.go`
**Packages:** `internal/cli`
**Blocked by:** 4.2

**Description**

Modify `runManageUpdate` at `internal/cli/manage.go:1082-1119`:

1. Replace the `if provider != domain.ProviderCodex { return fmt.Errorf("manage update: provider %q is not supported yet", provider) }` guard at manage.go:1083-1085 with a `switch provider` dispatch. `default:` returns the existing unsupported-provider error.
2. Codex branch: preserve exact current behavior — call `openImagesService(cmd, paths, domain.ProviderCodex)` and `service.EnsureLatest(cmd.Context(), imagesservice.EnsureRequest{})`. Output fields and headings unchanged.
3. Claude branch: call `openImagesService(cmd, paths, domain.ProviderClaude)`. Because Claude uses the pinned-version fast path with no resolver, call `service.Build(cmd.Context(), imagesservice.BuildRequest{Version: imagesservice.DefaultClaudeCLIVersion})` instead of `EnsureLatest`. Emit the output record with heading `"Provider image built"` (distinct from Codex's `"Provider image updated"`/`"Provider image up to date"` because the Claude path is not comparing-then-rebuilding yet) and fields: `provider`, `image`, `tags`, `version`, `context`. The `checked at` field is Codex-only (resolver-derived) and is omitted for Claude. The Claude branch MUST preserve the `runWithCLIQuietSpinner` wrapper pattern the Codex branch uses at `manage.go:1096-1106`. Codex's current spinner strings (`"Checking provider image"` / `"Provider image check complete"` / `"Provider image update failed"`) remain unchanged. Suggested Claude strings: `"Building provider image"` (start), `"Provider image built"` (success), `"Provider image build failed"` (failure). Wrap the `service.Build(...)` call inside the spinner closure — same shape as the Codex `EnsureLatest` wrapper.
4. Update `newManageUpdateCommand` `Example` (manage.go:1067-1069) to include `valv manage update claude` — the existing `parseOptionalProvider(args, domain.ProviderCodex)` at manage.go:1072 already accepts `"claude"`, and `supportedProviders` at manage.go:984 already lists it, so no flag plumbing is needed.

Tests in `internal/cli/manage_test.go`:

- `TestRunManageUpdateClaudeBuildsImage` — uses the existing manage-test harness (Grep for pattern `TestRunManageUpdate` / `newManageUpdateCommand` in `manage_test.go` to find the harness). Invoke `valv manage update claude` through the cobra command, assert: (a) the Claude build-cache Dockerfile exists at `<BuildCacheDir>/claude/Dockerfile`, (b) the fake docker runner recorded a `buildx build` call whose `--build-arg CLAUDE_VERSION=<DefaultClaudeCLIVersion>` matches the pinned constant, (c) the output record heading is `"Provider image built"` with a `provider=claude` field.
- `TestRunManageUpdateCodexRegression` — invoke `valv manage update` (no positional arg, defaults to Codex) and `valv manage update codex` and assert identical behavior to current Codex tests. Builder confirms existing Codex tests in `manage_test.go` still pass; if the harness is not factored to reuse, the builder may copy the minimum assertions.
- `TestRunManageUpdateUnsupportedProvider` — passing an unparseable provider via raw `runManageUpdate(..., domain.Provider("foo"))` returns the `default:` error. `parseOptionalProvider` already rejects `"foo"` before reaching this code path, so this is a defensive unit test, not a user-facing path.

**Acceptance**

- `valv manage update claude` (invoked through `newManageUpdateCommand`'s cobra command inside a test harness) produces an `EnsureResult`-equivalent output with `image` = `valv-claude:dev` and `version` = `DefaultClaudeCLIVersion`.
- `valv manage update` (no arg) and `valv manage update codex` behavior identical to prior — no test regression in existing manage-test coverage.
- `mage testPkg ./internal/cli` green (gofumpt + 70% coverage + all three new tests above).
- Claude branch of `runManageUpdate` wraps the `service.Build(...)` call in `runWithCLIQuietSpinner(...)` — symmetric to Codex's `EnsureLatest` wrapper at `manage.go:1096-1106` — preserving spinner UX parity. Codex branch spinner call is unchanged.
- `go doc github.com/evanmschultz/valv/internal/cli` does NOT expose a new unexported symbol as public API (sanity check that the change stayed internal).
- Builder records the pinned Claude CLI version in `BUILDER_WORKLOG.md` — if it differs from `2.1.89`, the rationale + Context7 query timestamp go in the worklog.

### Notes

- **Package-lock rule.** 4.2 and 4.3 both touch `internal/cli`; they are serialized via `blocked_by` to prevent parallel-builder conflicts on `operator_helpers.go` + `manage.go`. 4.1 is on the disjoint `internal/services/images` package — in principle parallelizable, but 4.2/4.3 import its new exports so the chain is effectively linear.
- **Dockerfile-shape-as-MVP rule.** 4.1 copies the Codex recipe verbatim except for the identified Claude-specific deltas (npm package name, entrypoint, config-dir env, mount path). Resist any urge to drop unused apt packages (e.g. `bubblewrap` — Claude doesn't use the sandbox yet, but pruning is §8 cleanup, not fast-path scope) or change the base image. YAGNI pressure absorbed.
- **Resolver-nil handling.** `images.Service.New` already tolerates `Resolver == nil` for non-Codex providers (service.go:204-207 auto-assigns only for Codex). Verified via direct read — no guard rejects a nil resolver at constructor time. Claude's Build path does not touch `s.resolver`. Builder confirms this in the worklog rather than changing the constructor.
- **Pin-drift tolerance.** The constant `DefaultClaudeCLIVersion` gives one source of truth; if the dev wants to bump the pin later, it is a one-line change plus a test regeneration. No multi-file churn.
- **`valv manage update --provider claude` vs `valv manage update claude`.** Focus-plan §6.3 uses `--provider claude` shorthand; existing CLI shape uses positional `valv manage update [provider]` via `parseOptionalProvider`. Plan targets the positional form (already wired) to keep DROP_4 strictly additive. Adding a `--provider` flag is a UX polish left for DROP_7 manage/TUI parity work.
- **No `internal/adapters/docker/` edits.** The PLAN.md container row's "expected paths" mention of that package was incorrect — the Codex Dockerfile and context writer live in `internal/services/images`, not `internal/adapters/docker`. Mirrored shape exactly; DROP_4 does not touch `internal/adapters/docker/`.
- **Claude CLI install path — npm deprecation fallback.** Upstream Claude Code is moving away from npm publish toward a native installer (`curl -fsSL https://claude.ai/install.sh | bash`). The npm path (`npm install --global @anthropic-ai/claude-code@${CLAUDE_VERSION}`) is still functional for `@anthropic-ai/claude-code@2.1.89` today. If the npm install step fails at build time because of upstream removal, the fallback is to switch `DefaultClaudeDockerfile` to the native installer. Builder records the pivot (Dockerfile change + Context7 recheck timestamp) in `BUILDER_WORKLOG.md`. Primary path remains npm until proven broken.

### Hylla Feedback

Hylla answered symbol-location and caller-graph queries well. One mild friction to record:

- **Query**: `hylla_search_keyword` on `DefaultCodexDockerfile` initially returned only service.go hits with summaries — useful but the Grep fallback surfaced `extended_test.go:843` (`fakeCodexRecipeHash`) and the magefile-less layout quickly. Not a Hylla bug; a reminder that for "every call site + every fixture" work the Grep sweep remains necessary alongside Hylla.
- **Suggestion**: expose a convenience refs-find shape that also pulls test-fixture references by default when the target is an exported function in a service package, so planners don't forget to check `_test.go` call sites. Low priority; current pattern (Grep-after-Hylla) works.

No other Hylla misses in this planning pass. Non-Go evidence (Dockerfile strings, focus-plan markdown, mage-target discovery) used `Read` / `Grep` / `Bash mage -l` directly per protocol — not a Hylla miss.
