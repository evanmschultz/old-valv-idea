## Plan — Round 1

**Verdict:** fail

### Findings

1. **`recipeHash()` silently hardcodes Codex content for Claude — hidden dependency + contract mismatch (blocker).**

   **Attack:** Build a Claude image via the plan's `openImagesService(cmd, paths, domain.ProviderClaude)` path. The service calls `recipeHash()` at `/Users/evanschultz/Documents/Code/hylla/valv/main/internal/services/images/service.go:441-451`. That function's logic:

   ```go
   func (s Service) recipeHash() string {
       content := DefaultCodexDockerfile()
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

   `defaultCodexDockerfile = "Dockerfile"` (service.go:26). The plan's `WriteDefaultClaudeContext` writes `filepath.Join(root, "Dockerfile")` — basename `"Dockerfile"` — so the sentinel comparison is FALSE and the fallback disk-read branch never runs. Result: the Claude image is built with label `io.valv.recipe_hash=<sha256(DefaultCodexDockerfile())>`. The label is a lie and will silently poison any future `imageRecipeMatches` comparison (`service.go:470-483`) that a Claude-configured service makes. DROP_4 uses only `Build`, so the current flow "works" in that no error is returned — but the built image is mislabeled, and the moment DROP_5 or any later drop wires a Claude ensure-latest-equivalent, recipe-matching will yield false positives and skip rebuilds on genuine Claude recipe changes.

   **Evidence:**
   - `recipeHash()` body cited above — `main/internal/services/images/service.go:441-451`.
   - Sentinel constant — `main/internal/services/images/service.go:26`.
   - `WriteDefaultCodexContext` uses basename `"Dockerfile"` — `service.go:511`; plan Unit 4.1 item 3 says `WriteDefaultClaudeContext` mirrors this with the same `"Dockerfile"` basename — `PLAN.md:89`.
   - Label write site — `service.go:257` (`recipeHashLabel: s.recipeHash()`).
   - Recipe-match consumer that will misread the label — `service.go:470-483`.

   **What the plan must change:** either (a) Unit 4.1 adds a Claude-aware path to `recipeHash()` (the simplest shape: add a `providerDockerfileContent()` helper that keys off `s.provider` and returns `DefaultClaudeDockerfile()` when `provider == ProviderClaude`, falling back to disk-read as today), or (b) Unit 4.1 acceptance adds a test `TestServiceBuildRecipeHashUsesClaudeDockerfileForClaudeProvider` that calls `svc.Build(...)` on a Claude-configured service and asserts the recorded `--label io.valv.recipe_hash=…` equals `sha256(DefaultClaudeDockerfile())`, then relies on the builder to make that test pass. Plan as written produces a silent failure.

   **Severity:** blocker.

2. **Test strategy does not exercise the `io.valv.recipe_hash` label on the Claude-built image (blocker).**

   **Attack:** Enumerate every test the plan prescribes for Claude:
   - Unit 4.1: `TestWriteDefaultClaudeContextWritesDockerfile` asserts file contents; `TestDefaultClaudeCLIVersionIsNonEmpty` asserts the constant; integration test `TestWriteDefaultClaudeContextBuildsWithExistingUIDAndGID` only asserts `image inspect` succeeds (existence check).
   - Unit 4.2: asserts build-cache path + dockerfile-contents match.
   - Unit 4.3: asserts `--build-arg CLAUDE_VERSION=…` matches `DefaultClaudeCLIVersion` and output heading is `"Provider image built"`.

   None of these assertions inspect the `--label io.valv.recipe_hash=<value>` recorded on the Claude build. The Codex test fixture at `extended_test.go:459` checks `--label io.valv.managed=true`, `io.valv.provider=codex`, `io.valv.scope=image`, `io.valv.version=…` but explicitly skips the recipe-hash label because its value is a computed sha — which is precisely why Finding 1's regression sails through unnoticed.

   **Evidence:**
   - Tests enumerated above — `PLAN.md:92-98, 144-147, 176-180`.
   - Existing Codex label assertions omit recipe_hash — `main/internal/cli/extended_test.go:459`.
   - The Codex unit test that DOES check the recipe hash — `main/internal/services/images/service_test.go:107` uses `svc.recipeHash()` (the service's own computation), which would also be "correct" against the Codex baseline for a buggy Claude variant because the bug matches the test's assumption.

   **What the plan must change:** Unit 4.1 adds `TestServiceBuildRecipeHashMatchesProviderDockerfile` that instantiates a Claude-provider service, runs `Build`, captures the `--label io.valv.recipe_hash=…` argument from the fake runner, and asserts it equals `hex(sha256(DefaultClaudeDockerfile()))`. The test is cheap (no docker needed — it uses the existing `runnerRecorder` shape) and locks the contract that "provider=claude builds carry Claude's recipe hash."

   **Severity:** blocker — Finding 1 remains latent without it.

3. **`runManageHome` TUI path still dispatches `runManageUpdate` with a hardcoded `domain.ProviderCodex` (concern).**

   **Attack:** Plan Unit 4.3 verifies the Claude branch via the cobra command path (`valv manage update claude`). But `internal/cli/operator_helpers.go:127` reads:

   ```go
   case managetui.ActionUpdate:
       return runManageUpdate(cmd, paths, opts, domain.ProviderCodex)
   ```

   A user invoking the update action via `valv manage` (TUI home) after DROP_4 still lands on the Codex branch — there is no provider selection. The plan does not mention this surface. Not a correctness bug (the Codex path still works), but it means "DROP_4 makes `valv manage update claude` work" is not equivalent to "DROP_4 lets users update the Claude image from every entrypoint." If the dev's operational mental model is "the TUI is the real UI," the drop will feel half-delivered.

   **Evidence:**
   - Hardcoded Codex dispatch — `main/internal/cli/operator_helpers.go:122-127`.
   - Plan doesn't touch `operator_helpers.go:104-135` — `PLAN.md:121-127`.
   - Plan scope explicitly calls `valv manage update --provider claude` the MVP surface — `PLAN.md:14, 195`.

   **What the plan must do:** either (a) scope this TUI gap explicitly as out-of-scope for DROP_4 (one sentence in "Explicitly deferred"), or (b) add a quarter-unit to Unit 4.3 that teaches the TUI flow to pick a provider (larger lift — likely defers). Preferring (a).

   **Severity:** concern — scope clarity, not correctness.

4. **`@anthropic-ai/claude-code` npm install path is listed as deprecated by upstream (advisory).**

   **Attack:** Context7 `/anthropics/claude-code` README section "Install Claude Code on various operating systems" lists the npm command last and describes it as "the deprecated npm method." The upstream recommended path is `curl -fsSL https://claude.ai/install.sh | bash` (or Homebrew on macOS). The plan mirrors the Codex pattern (`npm install --global …`) because the Codex CLI is published the same way. For Codex this is stable; for Claude, upstream is signaling they may stop publishing to npm. Not broken today — `v2.1.89` is present on npm — but a latent pin-staleness / install-break risk that doesn't apply to Codex.

   **Evidence:**
   - Context7 `/anthropics/claude-code` install section — retrieved 2026-04-21.
   - Plan pins npm-based install — `PLAN.md:83`.

   **What the plan could do (non-blocking):** Note in `BUILDER_WORKLOG.md` that if `npm install --global @anthropic-ai/claude-code@${CLAUDE_VERSION}` starts failing at build time, the fallback is to switch the Dockerfile to the native installer. Optional.

   **Severity:** advisory.

5. **Planner cites `manage.allProviders` at `manage.go:985`; actual symbol is `supportedProviders` (advisory).**

   **Attack:** PLAN.md:50 says "`manage.allProviders` at `internal/cli/manage.go:985`". Grep confirms the actual function name at that line is `supportedProviders`. A builder reading the plan literally and then running `grep allProviders internal/cli/manage.go` will find nothing and may second-guess the claim. Minor — but planner symbol names should match source.

   **Evidence:**
   - `main/internal/cli/manage.go:984` defines `func supportedProviders() []domain.Provider`.
   - PLAN.md:50 references `allProviders`.

   **What the plan should do:** one-word fix — replace `allProviders` with `supportedProviders`.

   **Severity:** advisory.

6. **`bubblewrap` in the apt-install list for a Claude image that does not use it (advisory).**

   **Attack:** Plan Unit 4.1 item "Everything else … is character-identical to the Codex recipe — the apt package list `bubblewrap ca-certificates git ncurses-term` applies equally to Claude." `bubblewrap` is a Codex sandbox dependency. Claude Code's Context7 docs and `/anthropics/claude-code` requirements make no mention of needing bubblewrap. The planner explicitly absorbs this YAGNI pressure in the Notes section (PLAN.md:192), calling pruning "§8 cleanup." That is a defensible call — mirror-the-Codex-shape is a clarity win for DROP_4 and stripping packages creates divergence that invites drift. Noted for the record, not a blocker.

   **Severity:** advisory.

### Routed Unknowns

- Whether `npm install --global @anthropic-ai/claude-code@2.1.89` succeeds inside `node:22-bookworm-slim` without additional system deps is a builder / integration-time concern. Not resolvable at plan time. Builder re-verifies in the integration test path.
- Whether DROP_5's ensure-latest-equivalent for Claude will include a real resolver or will stay pinned-only. PLAN.md:32 says "§8 backlog per focus-plan §3.5" — deferral is explicit. This falsification pass does not assume DROP_5 semantics, only that Finding 1's mislabeled image will survive into whatever DROP_5 builds.
