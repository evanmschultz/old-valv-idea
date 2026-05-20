# DROP_10 — CROSS_PROVIDER_CONTAINER

**State:** done
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
**Closed:** 2026-05-19

## Scope

**Dogfood blocker.** Enable in-container cross-provider tool use: when `valv claude` runs in a project, the claude container has the `codex` CLI installed AND the project's pinned Codex profile home is mounted at `/home/valv/.codex` with `CODEX_HOME=/home/valv/.codex` set — so a Claude Code agent that runs `codex exec` auto-routes to that project's pinned Codex login. Mirror for `valv codex` containers. Three layers of change: (a) Dockerfile — both images install BOTH `@anthropic-ai/claude-code` + `@openai/codex`; (b) `PrepareRuntime` (claude + codex) — accept optional OTHER-provider profile home, add cross-mount + cross-env var when present; (c) `Service.Run` (claude + codex) — look up the project's binding for the OTHER provider via `BindingByProjectID`, pass the resolved profile home into `PrepareRequest`. Binding-fallback policy: when the OTHER provider is NOT bound to the project, skip the cross-mount and cross-env — the OTHER CLI is installed but unauthed, so cross-calls fail with the CLI's native "not logged in" message (Option A from 2026-05-19 dev discussion). One-time `valv account bind` per provider per project unlocks full auto-routing. Estimated ~300–500 LOC including tests. Image size and build time both grow ~2× (one-time, per-machine, at `mage image update`). Inserted ahead of the v0.1.0 release work because the dev's stated next action is `mage install` + dogfood-build other tools with Valv.

## Planner

### Scope confirmation

DROP_10 adds in-container cross-provider tool use across three change layers:
(a) both Dockerfiles install BOTH CLIs; (b) `PrepareRuntime` for each provider
accepts an optional OTHER-provider profile home and conditionally adds a second
mount + env var; (c) `Service.Run` for each provider looks up the OTHER
provider's binding for the same project and passes the resolved `HomePath`
through to `PrepareRequest`. When the OTHER provider is not bound, the
cross-mount is silently skipped (Option A — fail cleanly with CLI's native
"not logged in" error).

### Open questions resolved

**Version-resolver shape:** Smallest-diff path is a `CrossProviderVersion
string` field added to `images.BuildRequest`. `Build()` always emits both
`--build-arg CODEX_VERSION=<v>` and `--build-arg CLAUDE_VERSION=<v>` regardless
of provider. When `CrossProviderVersion` is empty the defaulting in `Build()`
substitutes `"latest"` — valid npm syntax, not reproducible but acceptable for
the cross-provider secondary install at dogfood stage. No second `VersionResolver`
is added to `images.Options`; callers that want a pinned cross-version set the
field explicitly. This avoids touching `Options`, `Service`, or `EnsureLatest`.

**Integration test scope:** The real in-container cross-call test (Claude Code
agent running `codex exec` against a mounted codex profile) requires an actual
container with both CLIs installed. That test is deferred to
DROP_11_E2E_AND_RELEASE, which already owns `claude_integration_test.go`.
DROP_10's acceptance is `mage test` green across all five touched packages.

**README paragraph:** Deferred to DROP_11 (full README rewrite). A note in this
file's Notes section records the deferral.

### Units

---

#### Unit 10.1 — Dockerfile dual-CLI install + Build() dual-arg + service test updates

| Field | Value |
|---|---|
| State | done (R1 — mage gates GREEN; both Dockerfiles install both CLIs; Build() emits both build-args) |
| Paths | `internal/services/images/service.go`, `internal/services/images/service_test.go` |
| Packages | `github.com/evanmschultz/valv/internal/services/images` |
| Blocked by | — |

**What changes:**

- `DefaultCodexDockerfile()`: add `ARG CLAUDE_VERSION` build-arg declaration,
  add `mkdir -p /home/valv/.claude` to the user-creation RUN block (alongside
  the existing `/home/valv/.codex`), and add a second `npm install` RUN layer:
  `RUN npm install --global "@anthropic-ai/claude-code@${CLAUDE_VERSION}"`.
  Keep entrypoint as `["codex"]`.

- `DefaultClaudeDockerfile()`: mirror — add `ARG CODEX_VERSION`, extend the
  `mkdir -p` to include `/home/valv/.codex`, add
  `RUN npm install --global "@openai/codex@${CODEX_VERSION}"`. Keep entrypoint
  as `["claude"]`. Also add `CODEX_HOME=/home/valv/.codex` to the `ENV` block
  (the directory exists; the env var makes it discoverable).

- `Build()`: after the primary build-arg (`CODEX_VERSION` or `CLAUDE_VERSION`),
  add the cross-provider build-arg. Add `CrossProviderVersion string` to
  `BuildRequest`. When `CrossProviderVersion` is empty, use `"latest"` as the
  default cross-version. Always emit both `--build-arg CODEX_VERSION=<v>` and
  `--build-arg CLAUDE_VERSION=<v>` in `BuildRequest.BuildArgs`.

- `service_test.go` arg-comparison tests: three existing tests compare the
  exact `--build-arg` slice via `reflect.DeepEqual` —
  `TestServiceBuildAddsVersionAndUsesDefaultImageInfo`,
  `TestBuildIncludesExtraTags`,
  `TestServiceBuildFallsBackToLegacyBuildWhenBuildxUnavailable`. Each must be
  updated to include both `CODEX_VERSION` and `CLAUDE_VERSION` args in the
  expected slice. `docker.BuildImageArgs` sorts the `BuildArgs` map keys
  alphabetically via `sort.Strings(keys)` (confirmed: `internal/adapters/docker/ops.go`
  `BuildImageArgs`). Alphabetic order of the four keys is:
  `CLAUDE_VERSION` < `CODEX_VERSION` < `VALV_GID` < `VALV_UID`. The expected
  `want` slice must therefore be ordered:
  `[]string{"--build-arg", "CLAUDE_VERSION=<v>", "--build-arg", "CODEX_VERSION=<v>", "--build-arg", "VALV_GID=<gid>", "--build-arg", "VALV_UID=<uid>", ...}`
  regardless of which provider is being built. (The "new, not yet in tree"
  symbol: `BuildRequest.CrossProviderVersion`.)

- `TestWriteDefaultCodexContextWritesDockerfile` and
  `TestWriteDefaultClaudeContextWritesDockerfile`: add assertions for the new
  cross-CLI install line and the cross-home dir. Recipe-hash tests that call
  `DefaultCodexDockerfile()` / `DefaultClaudeDockerfile()` directly
  (`fakeCodexRecipeHash`, `fakeClaudeRecipeHash`, `svcRecipeHashForTest`,
  `TestServiceBuildRecipeHashMatchesProviderDockerfile`) auto-update because they
  call the functions dynamically — no manual constant change needed.

**Acceptance:**
- `DefaultCodexDockerfile()` output contains both `@openai/codex@${CODEX_VERSION}`
  and `@anthropic-ai/claude-code@${CLAUDE_VERSION}`, and both `/home/valv/.codex`
  and `/home/valv/.claude` in the `mkdir -p` line.
- `DefaultClaudeDockerfile()` mirrors symmetrically.
- `Build()` args include `CODEX_VERSION=<v>` and `CLAUDE_VERSION=<v>` in every
  invocation regardless of provider.
- `mage testPkg github.com/evanmschultz/valv/internal/services/images` passes
  green (including coverage gate).
- Note: `internal/cli` tests that call `fakeCodexRecipeHash()` /
  `fakeClaudeRecipeHash()` will also auto-pass because those functions delegate
  to the updated Dockerfile functions; verify with `mage test` at drop-end.

---

#### Unit 10.2 — claude.PrepareRuntime cross-provider mount + test flip

| Field | Value |
|---|---|
| State | done (R1 — mage gates GREEN; cross-mount + CODEX_HOME conditional path landed) |
| Paths | `internal/adapters/providers/claude/runtime.go`, `internal/adapters/providers/claude/runtime_test.go` |
| Packages | `github.com/evanmschultz/valv/internal/adapters/providers/claude` |
| Blocked by | — |

**What changes:**

- `PrepareRequest` gains `OtherProviderProfileHome string` (new field, not yet
  in tree).

- `PrepareRuntime`: after building `mounts` and `env` for the primary Claude
  home, add a conditional block:

  ```go
  if strings.TrimSpace(request.OtherProviderProfileHome) != "" {
      otherHome, err := pathutil.Normalize(request.OtherProviderProfileHome)
      if err != nil {
          return PreparedRuntime{}, fmt.Errorf("prepare claude runtime: normalize other provider home: %w", err)
      }
      mounts = append(mounts, dockeradapter.NewMountSpec(otherHome, "/home/valv/.codex", false))
      env["CODEX_HOME"] = "/home/valv/.codex"
  }
  ```

  The `OtherProviderProfileHome` path is the raw host path (e.g. the codex
  profile's `HomePath`); it is mounted read-write so that codex can write session
  state back just as it would in a native codex container. (Cleanup for the cross-
  mount is NOT added — the cross-mount target is the raw profile home, not a
  staged copy; no sync-back is needed for the cross-provider home.)

- `runtime_test.go` changes:
  - Rename `TestPrepareRuntimeHasNoCodexEnv` →
    `TestPrepareRuntimeSkipsCodexMountWhenNotProvided`. Update its body: pass an
    empty `OtherProviderProfileHome` (as today) and assert `CODEX_HOME` is absent
    from `Env` AND that no mount with target `/home/valv/.codex` exists.
  - Add `TestPrepareRuntimeMountsCodexHomeWhenProvided`: pass a non-empty
    `OtherProviderProfileHome` (a `t.TempDir()`), assert `CODEX_HOME` is set to
    `/home/valv/.codex` in `Env` AND a mount exists with target
    `/home/valv/.codex`.

**Acceptance:**
- `TestPrepareRuntimeSkipsCodexMountWhenNotProvided` passes: when
  `OtherProviderProfileHome` is empty, `CODEX_HOME` is absent from `Env` AND
  no mount with `Target == "/home/valv/.codex"` exists.
- `TestPrepareRuntimeMountsCodexHomeWhenProvided` passes: when
  `OtherProviderProfileHome` is a valid dir, `Env["CODEX_HOME"] == "/home/valv/.codex"`
  AND `mounts` contains a `MountSpec` with `Target == "/home/valv/.codex"`.
- All existing claude runtime tests still pass (no regressions).
- `mage testPkg github.com/evanmschultz/valv/internal/adapters/providers/claude`
  passes green.

---

#### Unit 10.3 — codex.PrepareRuntime cross-provider mount + new tests

| Field | Value |
|---|---|
| State | done (R1 — mage gates GREEN; cross-mount + CLAUDE_CONFIG_DIR conditional path landed) |
| Paths | `internal/adapters/providers/codex/runtime.go`, `internal/adapters/providers/codex/runtime_test.go` |
| Packages | `github.com/evanmschultz/valv/internal/adapters/providers/codex` |
| Blocked by | — |

**What changes:**

- `PrepareRequest` gains `OtherProviderProfileHome string` (new field, not yet
  in tree). Mirror of 10.2.

- `PrepareRuntime`: immediately after the initial `mounts := []dockeradapter.MountSpec{...}` +
  `env := map[string]string{...}` block, BEFORE the `newBridgeManager` call, add:

  ```go
  if strings.TrimSpace(request.OtherProviderProfileHome) != "" {
      otherHome, err := pathutil.Normalize(request.OtherProviderProfileHome)
      if err != nil {
          return PreparedRuntime{}, fmt.Errorf("prepare codex runtime: normalize other provider home: %w", err)
      }
      mounts = append(mounts, dockeradapter.NewMountSpec(otherHome, "/home/valv/.claude", false))
      env["CLAUDE_CONFIG_DIR"] = "/home/valv/.claude"
  }
  ```

- `runtime_test.go` additions (no existing tests need renaming in the codex
  package — there is no `TestPrepareRuntimeHasNoClaude*` test to flip):
  - Add `TestPrepareRuntimeSkipsClaudeMountWhenNotProvided`: empty
    `OtherProviderProfileHome`, assert `CLAUDE_CONFIG_DIR` absent, no mount
    with target `/home/valv/.claude`.
  - Add `TestPrepareRuntimeMountsClaudeHomeWhenProvided`: non-empty
    `OtherProviderProfileHome`, assert `CLAUDE_CONFIG_DIR=/home/valv/.claude`
    and mount present.

**Acceptance:**
- `TestPrepareRuntimeSkipsClaudeMountWhenNotProvided` passes: when
  `OtherProviderProfileHome` is empty, `CLAUDE_CONFIG_DIR` is absent from `Env`
  AND no mount with `Target == "/home/valv/.claude"` exists.
- `TestPrepareRuntimeMountsClaudeHomeWhenProvided` passes: when
  `OtherProviderProfileHome` is a valid dir, `Env["CLAUDE_CONFIG_DIR"] == "/home/valv/.claude"`
  AND `mounts` contains a `MountSpec` with `Target == "/home/valv/.claude"`.
- All existing codex runtime tests still pass.
- `mage testPkg github.com/evanmschultz/valv/internal/adapters/providers/codex`
  passes green.

---

#### Unit 10.4 — claude.Service.Run cross-binding lookup + service tests

| Field | Value |
|---|---|
| State | done (R1 — mage gates GREEN; cross-binding lookup + fakeStore opt-out defaults applied) |
| Paths | `internal/services/claude/service.go`, `internal/services/claude/service_test.go` |
| Packages | `github.com/evanmschultz/valv/internal/services/claude` |
| Blocked by | 10.2 |

**What changes:**

- `Service.Run`: after `resolved` is fully populated (at the end of both the
  `overrideProfile` branch and the `else` branch), and before calling
  `clauderuntime.PrepareRuntime`, add a cross-binding lookup:

  ```go
  var otherProfileHome string
  otherBinding, err := s.store.BindingByProjectID(ctx, resolved.project.ID, domain.ProviderCodex)
  if err == nil {
      otherProfile, profileErr := s.store.ProfileByID(ctx, otherBinding.ProfileID)
      if profileErr == nil {
          otherProfileHome = otherProfile.HomePath
      }
  } else if !errors.Is(err, domain.ErrNotFound) {
      return fmt.Errorf("run claude launch service: lookup codex binding for project %q: %w", resolved.project.Root, err)
  }
  ```

  Then pass `OtherProviderProfileHome: otherProfileHome` to `PrepareRequest`.

  Error handling: only `ErrNotFound` is silently skipped; unexpected store errors
  are returned as fatal. This follows the existing pattern in `resolveBinding`.

- `service_test.go`:
  - Extend `fakeStore` to support provider-keyed bindings. Add
    `crossBinding domain.ProjectBinding` and `crossBindingErr error` and
    `crossProfile domain.Profile` and `crossProfileErr error`. Override
    `BindingByProjectID` to return `f.crossBinding, f.crossBindingErr` when
    the requested provider is `domain.ProviderCodex`, and `f.binding, f.bindingErr`
    for `domain.ProviderClaude`. (The current single-field approach needs this
    disambiguation.) Add `ProfileByID` to also dispatch on profile ID between
    primary and cross profile.
  - Add table-driven `TestRunCrossProviderMountWhenCodexBound`:
    - Row "codex bound": cross binding resolves → `PreparedRuntime` has mount
      with target `/home/valv/.codex` and `CODEX_HOME` in Env.
    - Row "codex not bound (ErrNotFound)": `BindingByProjectID` for codex returns
      `ErrNotFound` → Run succeeds, no `/home/valv/.codex` mount in request.
    - Row "codex store error": `BindingByProjectID` for codex returns a non-
      `ErrNotFound` error → Run returns an error.
  - Existing `TestRunSucceedsWithBoundProject` and similar must still pass
    (no cross binding wired → cross mount absent → container request unchanged
    from today's shape).

**Acceptance:**
- `TestRunCrossProviderMountWhenCodexBound` passes all three rows.
- All existing claude service tests pass (regression-free).
- `mage testPkg github.com/evanmschultz/valv/internal/services/claude` passes
  green.

---

#### Unit 10.5 — codex.Service.Run cross-binding lookup + service tests

| Field | Value |
|---|---|
| State | done (R1 — mage gates GREEN; cross-binding lookup + fakeStore opt-out defaults applied) |
| Paths | `internal/services/codex/service.go`, `internal/services/codex/service_test.go` |
| Packages | `github.com/evanmschultz/valv/internal/services/codex` |
| Blocked by | 10.3 |

**What changes:**

- `Service.Run`: mirror of 10.4's change. After `resolved` is populated, add
  cross-binding lookup for `domain.ProviderClaude`:

  ```go
  var otherProfileHome string
  otherBinding, err := s.store.BindingByProjectID(ctx, resolved.project.ID, domain.ProviderClaude)
  if err == nil {
      otherProfile, profileErr := s.store.ProfileByID(ctx, otherBinding.ProfileID)
      if profileErr == nil {
          otherProfileHome = otherProfile.HomePath
      }
  } else if !errors.Is(err, domain.ErrNotFound) {
      return fmt.Errorf("run codex launch service: lookup claude binding for project %q: %w", resolved.project.Root, err)
  }
  ```

  Then pass `OtherProviderProfileHome: otherProfileHome` to codexruntime's
  `PrepareRequest`.

- `service_test.go`:
  - Extend codex `fakeStore` with provider-keyed binding dispatch, same pattern
    as 10.4's extension to claude `fakeStore`.
  - Add table-driven `TestRunCrossProviderMountWhenClaudeBound`:
    - Row "claude bound": cross binding resolves → mount with target
      `/home/valv/.claude` and `CLAUDE_CONFIG_DIR` in Env.
    - Row "claude not bound (ErrNotFound)": silent skip → Run succeeds, no
      `/home/valv/.claude` mount.
    - Row "claude store error": non-`ErrNotFound` → Run returns error.
  - Existing codex service tests pass unchanged.

**Acceptance:**
- `TestRunCrossProviderMountWhenClaudeBound` passes all three rows.
- All existing codex service tests pass.
- `mage testPkg github.com/evanmschultz/valv/internal/services/codex` passes
  green.

---

### Drop-end verification

After all five units pass per-unit QA:
- `mage test` from `main/` (full test + coverage + gofumpt check) must pass clean.
- No `mage integration` needed for this drop (no symbol deletions, no CLI argv
  changes, no Docker-backed code paths changed in a way that would alter the
  integration test surface).
- After `mage test` green: `git push`, `gh run watch --exit-status`, then
  `mage build` before handing back to the dev.

## Notes

### Design constraint — cross-provider auth file shapes

- **Claude side stores credentials as `<managed-home>/.credentials.json`** per DROP_7 (host-side `claude setup-token` + keychain extraction).
- **Codex side stores credentials as `<managed-home>/auth.json`** per DROP_5 (`codex login --home <managed-home>` writes there) plus optional `<managed-home>/config.toml` for project config.
- Mounting the OTHER provider's managed-home at the container's standard config dir (`/home/valv/.claude` or `/home/valv/.codex`) gives the OTHER CLI access to its native auth file shape — no extraction or transformation needed.

### `TestPrepareRuntimeHasNoCodexEnv` is a v1 boundary check that must flip

`internal/adapters/providers/claude/runtime_test.go:TestPrepareRuntimeHasNoCodexEnv` deliberately asserts the claude runtime has NO codex env. DROP_10 inverts this: when an OTHER profile home is provided, the runtime MUST set `CODEX_HOME` and add the cross-mount. New test: `TestPrepareRuntimeMountsCodexHomeWhenProvided`. Keep an asymmetric `TestPrepareRuntimeSkipsCodexMountWhenNotProvided` to cover the "OTHER provider not bound" path.

### Cache-busting for image rebuild

Both Dockerfile recipes change → recipe-hash changes → `mage image update claude` / `mage image update codex` naturally rebuilds. Tests that delegate to `DefaultCodexDockerfile()` / `DefaultClaudeDockerfile()` (including `fakeClaudeRecipeHash` / `fakeCodexRecipeHash` in `internal/cli/extended_test.go`) auto-update because they call the functions dynamically — no constant churn needed.

### Version-resolver awareness

Each image currently has a single version resolver (`CodexVersionResolver` in codex package). With both CLIs installed in both images, each image's recipe needs BOTH versions pinned. Decision for planner: extend the existing resolvers to know about both providers, OR introduce a `CrossProviderImageVersions` aggregate that callers populate. The dev's preference (per `feedback_drop_ceremony_trim`): smallest-diff path.

### Per-project binding requirement

Auto-routing requires BOTH providers to be bound to the project. README needs a paragraph saying: "to enable cross-provider tool use (e.g., Claude Code agents calling `codex exec`), run `valv account bind <claude-name> --provider claude` AND `valv account bind <codex-name> --provider codex` in the project once." This README work could land here OR defer to DROP_11_E2E_AND_RELEASE (which owns the README rewrite). Planner decides.
