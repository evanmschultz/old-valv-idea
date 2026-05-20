# DROP_10 — CROSS_PROVIDER_CONTAINER

**State:** planning
**Blocked by:** DROP_9 (done)
**Paths (expected):**
- `internal/services/images/service.go` (Dockerfile recipes — add OTHER CLI to each)
- `internal/services/images/service_test.go` (recipe-hash test updates)
- `internal/cli/extended_test.go` (recipe-hash fakes — `fakeClaudeRecipeHash`, `fakeCodexRecipeHash`)
- `internal/adapters/providers/claude/runtime.go` (`PrepareRequest` + `PrepareRuntime` — optional OTHER profile home)
- `internal/adapters/providers/claude/runtime_test.go` (flip `TestPrepareRuntimeHasNoCodexEnv` to mount-present test + skip-when-absent test)
- `internal/adapters/providers/codex/runtime.go` (mirror — `PrepareRequest` + `PrepareRuntime` for claude cross-mount)
- `internal/adapters/providers/codex/runtime_test.go` (mirror tests)
- `internal/services/claude/service.go` (`Service.Run` — look up codex binding for the project, pass to PrepareRequest)
- `internal/services/claude/service_test.go` (cross-provider binding lookup table-driven tests)
- `internal/services/codex/service.go` (mirror — look up claude binding)
- `internal/services/codex/service_test.go` (mirror tests)
- (Possibly) `internal/services/images/codex_version_resolver.go` and `claude_version_resolver.go` — version-resolver awareness so both images build with both pinned versions

**Packages (expected):**
- `github.com/evanmschultz/valv/internal/services/images`
- `github.com/evanmschultz/valv/internal/adapters/providers/claude`
- `github.com/evanmschultz/valv/internal/adapters/providers/codex`
- `github.com/evanmschultz/valv/internal/services/claude`
- `github.com/evanmschultz/valv/internal/services/codex`
- `github.com/evanmschultz/valv/internal/cli` (recipe-hash fakes only)

**PLAN.md ref:** main/PLAN.md → DROP_10_CROSS_PROVIDER_CONTAINER row
**Workflow:** main/drops/WORKFLOW.md
**Started:** 2026-05-19
**Closed:** —

## Scope

**Dogfood blocker.** Enable in-container cross-provider tool use: when `valv claude` runs in a project, the claude container has the `codex` CLI installed AND the project's pinned Codex profile home is mounted at `/home/valv/.codex` with `CODEX_HOME=/home/valv/.codex` set — so a Claude Code agent that runs `codex exec` auto-routes to that project's pinned Codex login. Mirror for `valv codex` containers. Three layers of change: (a) Dockerfile — both images install BOTH `@anthropic-ai/claude-code` + `@openai/codex`; (b) `PrepareRuntime` (claude + codex) — accept optional OTHER-provider profile home, add cross-mount + cross-env var when present; (c) `Service.Run` (claude + codex) — look up the project's binding for the OTHER provider via `BindingByProjectID`, pass the resolved profile home into `PrepareRequest`. Binding-fallback policy: when the OTHER provider is NOT bound to the project, skip the cross-mount and cross-env — the OTHER CLI is installed but unauthed, so cross-calls fail with the CLI's native "not logged in" message (Option A from 2026-05-19 dev discussion). One-time `valv account bind` per provider per project unlocks full auto-routing. Estimated ~300–500 LOC including tests. Image size and build time both grow ~2× (one-time, per-machine, at `mage image update`). Inserted ahead of the v0.1.0 release work because the dev's stated next action is `mage install` + dogfood-build other tools with Valv.

## Planner

<Filled by go-planning-agent in Phase 1.>

## Notes

### Design constraint — cross-provider auth file shapes

- **Claude side stores credentials as `<managed-home>/.credentials.json`** per DROP_7 (host-side `claude setup-token` + keychain extraction).
- **Codex side stores credentials as `<managed-home>/auth.json`** per DROP_5 (`codex login --home <managed-home>` writes there) plus optional `<managed-home>/config.toml` for project config.
- Mounting the OTHER provider's managed-home at the container's standard config dir (`/home/valv/.claude` or `/home/valv/.codex`) gives the OTHER CLI access to its native auth file shape — no extraction or transformation needed.

### `TestPrepareRuntimeHasNoCodexEnv` is a v1 boundary check that must flip

`internal/adapters/providers/claude/runtime_test.go:TestPrepareRuntimeHasNoCodexEnv` deliberately asserts the claude runtime has NO codex env. DROP_10 inverts this: when an OTHER profile home is provided, the runtime MUST set `CODEX_HOME` and add the cross-mount. New test: `TestPrepareRuntimeMountsCodexHomeWhenProvided`. Keep an asymmetric `TestPrepareRuntimeSkipsCodexMountWhenNotProvided` to cover the "OTHER provider not bound" path.

### Cache-busting for image rebuild

Both Dockerfile recipes change → recipe-hash changes → `mage image update claude` / `mage image update codex` naturally rebuilds. Tests that hard-code `fakeClaudeRecipeHash` / `fakeCodexRecipeHash` (in `internal/cli/extended_test.go`) need constant updates to match the new hashes. Mechanical churn — the planner should call out the exact files needing constant updates.

### Version-resolver awareness

Each image currently has a single version resolver (`CodexVersionResolver` in codex package). With both CLIs installed in both images, each image's recipe needs BOTH versions pinned. Decision for planner: extend the existing resolvers to know about both providers, OR introduce a `CrossProviderImageVersions` aggregate that callers populate. The dev's preference (per `feedback_drop_ceremony_trim`): smallest-diff path.

### Per-project binding requirement

Auto-routing requires BOTH providers to be bound to the project. README needs a paragraph saying: "to enable cross-provider tool use (e.g., Claude Code agents calling `codex exec`), run `valv account bind <claude-name> --provider claude` AND `valv account bind <codex-name> --provider codex` in the project once." This README work could land here OR defer to DROP_11_E2E_AND_RELEASE (which owns the README rewrite). Planner decides.
