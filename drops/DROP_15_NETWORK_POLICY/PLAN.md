# DROP_15 — NETWORK_POLICY

**State:** planning
**Blocked by:** DROP_14 (todo)
**Paths (expected):** `internal/domain/` (network-policy domain type — allowlist shape), `internal/tools/` (extend `.valv/tools.toml` `[allowlist]` parsing — currently `toml.Primitive` placeholder), `internal/adapters/providers/` (Docker `--network` / `--add-host` wiring), `internal/services/networkpolicy/` (new — resolution + closed-default enforcement), `internal/services/images/` (image-build egress enforcement), `internal/cli/` (allowlist edit subcommands or `valv network` namespace)
**Packages (expected):** `internal/domain/`, `internal/tools/`, `internal/adapters/providers/claude/`, `internal/adapters/providers/codex/`, new `internal/services/networkpolicy/`, `internal/services/images/`, `internal/cli/`
**PLAN.md ref:** main/PLAN.md → DROP_15_NETWORK_POLICY row
**Workflow:** main/drops/WORKFLOW.md
**Started:** 2026-05-22
**Closed:** —

## Scope

Closed-by-default outbound network for the container, with per-project allowlist (claudebox pattern identified in the 2026-05-20 OSS survey — see `main/CLAUDE.md` § "Product Direction"). The allowlist is editable two ways:

- Via CLI command (e.g. `valv network allow <host>` / `valv network deny <host>` / `valv network list`).
- Via `.valv/tools.toml` `[allowlist]` section (DROP_11 reserved this section's parsing as `toml.Primitive` — DROP_15 promotes it to a typed schema).

Opt-out flag for projects that need open egress (`--network open` or equivalent on `valv claude` / `valv codex` / `valv run`).

Implementation surface: Docker container runtime accepts `--network` and `--add-host` flags via the existing `ContainerRunRequest` plumbing. The closed-default behavior likely requires either a custom Docker network or `--network none` plus an HTTP/HTTPS proxy sidecar with allowlist filtering. The planner must choose between approaches based on operational simplicity vs. Docker-Desktop-on-macOS quirks.

## Planner

### Objective

Ship DROP_15 as a closed-by-default, macOS-compatible network policy layer across BOTH runtime container egress and DROP_12 overlay-image build egress. Project-scoped `.valv/tools.toml` reads that affect overlay/image policy MUST resolve from the detected project root rather than raw cwd so repo-subdir launches see the same allowlist and overlay behavior as repo-root launches. Keep the shared policy seam at post-DROP_13 `internal/services/run`, but add a distinct image-build unit because `BuildOverlayDockerfile` emits `RUN ["go","install",...]` / `RUN ["npm","install","-g",...]` lines that execute during `docker buildx build`, outside runtime launch policy (`internal/services/images/overlay.go:84-146`, `internal/services/images/service.go:718-838`). Use a typed `.valv/tools.toml` allowlist with built-in Go-module defaults, a host-local HTTP/HTTPS allowlist proxy, and an internal Docker network; keep `--network open` as the operator escape hatch.

### Schema Decisions

1. **Allowlist semantics are exact-host, lowercase, deduped, and union-based.** The typed `[allowlist]` schema remains exact hostname / Docker alias only: no CIDR, no URL prefixes, no scheme/path/port parsing, no wildcard syntax. DROP_15 now ships four built-in default hosts in the effective allowlist: `proxy.golang.org`, `sum.golang.org`, `objects.githubusercontent.com`, and `github.com`. User-declared `[allowlist].hosts` entries are UNIONED with these defaults, not replacements. Evidence: `internal/tools/tools.go:75-130` still has no typed allowlist today; A3 locks the zero-value/default behavior.
2. **`[allowlist]` typing happens in `internal/tools`, but overlay image identity stays `[tools]`-only.** `OverlayHash` still hashes only `manifest.Tools` (`internal/services/images/overlay.go:30-70`; tests at `internal/services/images/overlay_test.go:12-147`), so DROP_15 must keep allowlist/env policy out of project-image tags to avoid rebuild churn from non-tool edits.
3. **`.valv/tools.toml` editing is section-local, not whole-file re-encode.** New helper `WriteAllowlistSection(path string, cfg AllowlistConfig) error` (new, not yet in tree) rewrites only the `[allowlist]` section. It MUST preserve `[tools]` and `[env]` block order and content VERBATIM, including comments and blank lines within those sections. It MUST create the parent `.valv/` directory when absent via `os.MkdirAll(filepath.Dir(path), 0o755)` (`go doc os.MkdirAll`, `go doc path/filepath.Dir`; `internal/tools/resolve.go:11-30` fixes the manifest path at `.valv/tools.toml`).
4. **Runtime enforcement stays on the DROP_13 shared run seam.** Closed-default network policy is a property of the generic run service, not provider-specific launchers. Evidence: DROP_13 already plans `internal/services/run/service.go` as the shared launch seam (`drops/DROP_13_GENERIC_RUN/PLAN.md:36-43,59-96`), while current Claude/Codex services still duplicate request assembly (`internal/services/claude/service.go:194-220,298-329`; `internal/services/codex/service.go:185-212,302-332`).
5. **Closed mode uses an internal Docker network plus a host-local HTTP/HTTPS allowlist proxy.** Do NOT use literal `--network none`: Docker docs show it leaves loopback only. Do NOT use Linux-host iptables/ipset as the primary control plane for a macOS-first product. Use a host-local proxy reachable from containers via `host.docker.internal`, inject `HTTP_PROXY`, `HTTPS_PROXY`, and `NO_PROXY`, and place the workload on an internal network so non-HTTP/HTTPS egress has no default external route. Evidence: existing Codex bridge already exposes host services at `host.docker.internal` (`internal/adapters/providers/codex/bridge.go:42-60`) and rewrites loopback URLs to that host alias (`internal/adapters/providers/codex/bridge_test.go:274-300`); Context7 `/docker/docs` confirms `--internal` isolates egress and `--network none` is loopback-only.
6. **Image-build egress is a distinct policy surface and uses Docker's predefined proxy build args.** The overlay build path must inject the same allowlist policy into `docker buildx build` because DROP_12 tool install `RUN` lines execute at image-build time, before runtime launch. Use Docker's predefined proxy build args (`HTTP_PROXY`, `HTTPS_PROXY`, `NO_PROXY`) plus `ImageBuildRequest.Network = <internal-network>`; Docker docs confirm these proxy args do not require explicit Dockerfile `ARG` declarations. Evidence: `internal/adapters/docker/ops.go:9-21,38-72` already supports `ImageBuildRequest.Network`, and Context7 `/docker/docs` confirms both build proxy args and controlled network modes for build `RUN` instructions.
7. **There is no subtractive override for built-in defaults in DROP_15.** Because A3 locks union semantics, `valv network deny` removes only user-added hosts; it does not remove the built-in Go-module defaults. If dev later wants subtractive overrides, that is a follow-up drop, not hidden v1 behavior.
8. **Overlay/image-policy manifest resolution is project-root-based, not raw cwd-based.** `resolveProjectImage` currently does `tools.Resolve(workingDir)` (`internal/cli/operator_helpers.go:421-464`), while `runToolsValidate` already resolves root through `project.Detect()` before `tools.Resolve` (`internal/cli/tools.go:44-84`), and `project.DetectFrom(start)` already exists for explicit subdir callers (`internal/project/project.go:26-55`; tests at `internal/project/project_test.go:13-48,67-95,112-144`). DROP_15 must align every `.valv/tools.toml` read that affects overlay/image policy with that root-detection contract.

### Acceptance Criteria (drop-level)

1. `resolveProjectImage` and any other CLI path that consumes `.valv/tools.toml` for overlay/image policy resolve the manifest from the detected project root, not raw cwd, so repo-subdir launches and repo-root launches see the same overlay/allowlist policy.
2. `internal/tools.Load` / `Resolve` type-decode `[allowlist]` while keeping strict unknown-top-level rejection. The effective allowlist for a zero-value config is exactly the four built-in defaults; user entries are unioned with them.
3. `WriteAllowlistSection` creates the parent `.valv/` directory if absent and rewrites only `[allowlist]`, preserving `[tools]` and `[env]` block order/content verbatim, including comments and blank lines within those sections.
4. Closed mode is the default for the shared run path: it provisions policy material, injects proxy env vars, sets `ContainerRunRequest.Network`, and blocks non-allowlisted HTTP/HTTPS egress plus non-HTTP/HTTPS default egress. Open mode preserves today's unconstrained behavior.
5. DROP_15.3 explicitly depends on DROP_14 having re-homed env-var merging into `internal/services/run` per `drops/DROP_14_ENV_VARS/PLAN.md:69-70`; if env merging still lives in `internal/services/claude` and `internal/services/codex` when build starts, Unit 15.3 must include that re-home or block on a DROP_14 amendment.
6. The DROP_12 overlay build path threads the same effective allowlist policy through `docker buildx build` using the manifest resolved from the detected project root. An integration test forces a rebuild every run (`EnsureProjectRequest.NoCache = true` or a unique repository/tag) and proves a `.valv/tools.toml`-driven `go install` during image build reaches the proxy and is filtered. If `host.docker.internal` is unreachable from the `--internal` build topology, the image-build path must use the same validated fallback topology as runtime (for example `docker network connect bridge` plus custom routing) rather than leaving build-time egress on an unproven host-alias path.
7. CLI surface ships as `valv network allow <host>`, `valv network deny <host>`, `valv network list`, plus `valv run --network open`. Pre-DROP_13 `valv claude` / `valv codex` must not gain duplicated flag parsing; they inherit policy through the shared run seam once DROP_13 lands.

### Units

---

#### Unit 15.0 — Re-root project-image manifest resolution to the detected project root

**State:** todo

**Paths:**
- `internal/cli/operator_helpers.go`
- `internal/cli/codex_project_image_test.go`
- `internal/cli/claude_project_image_test.go`

**Packages:** `internal/cli`

**Evidence:** `internal/cli/operator_helpers.go:421-464` currently calls `tools.Resolve(workingDir)` directly inside `resolveProjectImage`; `internal/cli/tools.go:44-84` already uses `project.Detect()` then `tools.Resolve(result.Root)` for the same manifest surface; `internal/project/project.go:26-55` and `internal/project/project_test.go:13-48,67-95,112-144` already provide and verify explicit start-path detection for repo roots, linked worktrees, and subdirectories; `resolveProjectImage` is the live overlay/image-policy seam for both providers (`internal/cli/claude.go:115`, `internal/cli/codex.go:120`).

**Acceptance:**
- `resolveProjectImage` resolves the project root from the supplied `workingDir` via `project.DetectFrom(workingDir)` or an equivalent shared helper before calling `tools.Resolve`; raw `tools.Resolve(workingDir)` is removed.
- The empty-manifest short-circuit and the overlay-build path both consult `.valv/tools.toml` at the detected project root, so invoking `valv claude` / `valv codex` from a nested repo subdirectory still sees a root-level manifest.
- Existing override semantics remain unchanged: `VALV_CLAUDE_IMAGE` / `VALV_CODEX_IMAGE` still emit one warning and skip overlay work after the manifest has been resolved from the detected root.
- Tests in `internal/cli/codex_project_image_test.go` and `internal/cli/claude_project_image_test.go` add repo-subdirectory coverage proving a root-level manifest triggers overlay resolution/build from a nested working directory and that absence at the root still returns the base image.

**Blocked by:** nothing

---

#### Unit 15.1 — Typed allowlist schema, default hosts, and section-safe `.valv/tools.toml` editing

**State:** todo

**Paths:**
- `internal/tools/tools.go`
- `internal/tools/resolve_test.go`
- `internal/tools/allowlist_test.go` (new, not yet in tree)
- `internal/services/images/overlay_test.go`

**Packages:** `internal/tools`, `internal/services/images`

**Evidence:** `internal/tools/tools.go:75-130` still stores `ToolManifest.Allowlist` as `toml.Primitive` and decodes it into a discard map; `internal/tools/resolve.go:11-30` fixes the manifest path at `.valv/tools.toml`; `internal/tools/resolve_test.go:10-42,69-100,162-184` already exercises absent `.valv/`, forward-compat sections, and path invariants; `internal/services/images/overlay.go:30-70` and `internal/services/images/overlay_test.go:12-147` prove overlay hashing is tools-only today.

**Acceptance:**
- Promote `ToolManifest.Allowlist` from `toml.Primitive` to a typed `AllowlistConfig` (new, not yet in tree) with `Hosts []string`.
- `Load(path)` decodes `[allowlist]` into the typed struct while keeping `[env]` deferred for DROP_14 and preserving strict unknown-top-level rejection.
- Add a new effective-allowlist helper on the tools layer (new, not yet in tree): zero-value config returns exactly `proxy.golang.org`, `sum.golang.org`, `objects.githubusercontent.com`, and `github.com`; user hosts are lowercased, deduped, validated as exact hosts / Docker aliases, and UNIONED with the defaults.
- Add `WriteAllowlistSection(path string, cfg AllowlistConfig) error` (new, not yet in tree). It MUST call `os.MkdirAll(filepath.Dir(path), 0o755)` when the parent `.valv/` directory is absent.
- `WriteAllowlistSection` rewrites only `[allowlist]`. It MUST preserve `[tools]` and `[env]` block order and content VERBATIM, including comments and blank lines within those sections.
- Tests cover: fresh project root with no `.valv/` directory, create-from-absent-file behavior, default-host union semantics, normalization/dedupe, invalid host rejection, unknown top-level section rejection, and section-preservation behavior.
- `internal/services/images/overlay_test.go` gains a regression asserting that two manifests with identical `[tools]` but different `[allowlist]` data produce the same `OverlayHash`.

**Blocked by:** nothing

---

#### Unit 15.2 — Docker network lifecycle helpers

**State:** todo

**Paths:**
- `internal/adapters/docker/network.go` (new, not yet in tree)
- `internal/adapters/docker/executor.go`
- `internal/adapters/docker/network_test.go` (new, not yet in tree)

**Packages:** `internal/adapters/docker`

**Evidence:** `internal/adapters/docker/ops.go:9-21,38-72` already carries `ImageBuildRequest.Network`; `internal/adapters/docker/types.go:43-60,141-218` already carries `ContainerRunRequest.Network`; `internal/adapters/docker/executor.go:5-40` currently exposes build/remove/prune helpers but no network lifecycle methods. Context7 `/docker/docs` confirms the required CLI surface: `docker network create --internal`, `docker network connect --alias`, and `docker network rm`.

**Acceptance:**
- Add typed request structs `NetworkCreateRequest`, `NetworkConnectRequest`, and `NetworkRemoveRequest` (all new, not yet in tree).
- Add arg builders `BuildNetworkCreateArgs`, `BuildNetworkConnectArgs`, and `BuildNetworkRemoveArgs` (all new, not yet in tree) in `internal/adapters/docker/network.go`.
- Add `Executor.CreateNetwork`, `Executor.ConnectNetwork`, and `Executor.RemoveNetwork` (all new, not yet in tree) in `internal/adapters/docker/executor.go`.
- `BuildNetworkCreateArgs` emits `docker network create --internal ...`.
- `BuildNetworkConnectArgs` emits `docker network connect --alias <name> ...`.
- Tests cover required-field validation, `--internal` emission, alias emission, and deterministic arg order.

**Blocked by:** nothing

---

#### Unit 15.2.5 — Shared networkpolicy service and image-build egress enforcement

**State:** todo

**Paths:**
- `internal/services/networkpolicy/service.go` (new, not yet in tree)
- `internal/services/networkpolicy/service_test.go` (new, not yet in tree)
- `internal/services/images/service.go`
- `internal/services/images/service_test.go`
- `internal/services/images/service_integration_test.go`

**Packages:** `internal/services/networkpolicy`, `internal/services/images`

**Evidence:** `internal/services/images/overlay.go:84-146` emits overlay install `RUN` lines that execute at image-build time; `internal/services/images/service.go:718-838` builds overlays through `docker.BuildImageArgs`; `internal/adapters/docker/ops.go:38-72` already supports `--network`; `internal/services/images/service_test.go:1535-1595` proves `EnsureProjectImage` already has a working `NoCache` rebuild seam; `internal/cli/operator_helpers.go:421-464` and `internal/cli/tools.go:44-84` show the current root-resolution mismatch that this unit must not reintroduce; Context7 `/docker/docs` confirms predefined proxy build args (`HTTP_PROXY`, `HTTPS_PROXY`, `NO_PROXY`) apply to build `RUN` steps without Dockerfile `ARG` declarations. This is the A2 blocker: DROP_12 tool installs run outside runtime network policy.

**Acceptance:**
- Introduce a shared policy-material service in `internal/services/networkpolicy` (new, not yet in tree) that accepts the effective allowlist and returns the build/runtime policy inputs needed by callers: proxy URLs, `NO_PROXY`, internal-network name, and cleanup handle.
- `internal/services/images/service.go` reuses that service for overlay builds. When policy is active, the overlay `docker buildx build` request injects predefined proxy build args `HTTP_PROXY`, `HTTPS_PROXY`, and `NO_PROXY`, and sets `ImageBuildRequest.Network = <internal-network>`.
- The build-policy seam consumes the manifest and effective allowlist already resolved at the detected project root (Unit 15.0); do not add any new raw-cwd `.valv/tools.toml` lookup in `internal/services/images` or `internal/services/networkpolicy`.
- The effective allowlist used here includes the built-in defaults from Unit 15.1, so plain `go install github.com/x/y` works without extra user allowlist entries.
- If empirical validation in this unit or Unit 15.3 shows `host.docker.internal` is unreachable from the `--internal` build container, image-build policy MUST switch to the same validated fallback topology as runtime (for example `docker network connect bridge` plus custom routing) instead of shipping a build-only hole.
- Build-policy injection must not change `OverlayHash`, project-overlay tags, or the existing freshness-label contract from DROP_12.
- `internal/services/images/service_test.go` remains table-driven and proves build args now include `--network` plus proxy build args while preserving existing overlay labels and `--no-cache` behavior.
- `internal/services/images/service_integration_test.go` gains a tagged integration test proving a `.valv/tools.toml`-driven `go install` during overlay build reaches the host proxy and is filtered. To prevent a false green from overlay reuse, the test MUST set `EnsureProjectRequest.NoCache = true` or use a unique repository/tag per run to force rebuild every execution. The builder should avoid a live-internet dependency if practical by using a disposable local fixture/proxy path; at minimum the test must prove one allowlisted path succeeds and one blocked path fails via the proxy rather than unconstrained direct egress.
- Error wrapping boundaries are explicit for policy setup, image-build request assembly, docker build execution, and cleanup.

**Blocked by:** Unit 15.0, Unit 15.1, Unit 15.2

---

#### Unit 15.3 — Generic-run closed-default runtime policy

**State:** todo

**Paths:**
- `internal/services/run/service.go` (new, not yet in tree)
- `internal/services/run/service_test.go` (new, not yet in tree)
- `internal/services/run/service_integration_test.go` (new, not yet in tree; `//go:build integration`)

**Packages:** `internal/services/run`

**Evidence:** `drops/DROP_13_GENERIC_RUN/PLAN.md:36-43,59-96` already chooses `internal/services/run` as the shared runtime seam; `internal/services/claude/service.go:194-220,298-329` and `internal/services/codex/service.go:185-212,302-332` show today's duplicated launch/request-building path; provider runtimes still supply the base env maps (`internal/adapters/providers/claude/runtime.go:124-149`, `internal/adapters/providers/codex/runtime.go:111-132,178-219`); existing Codex bridge infrastructure already routes container-visible host URLs through `host.docker.internal` (`internal/adapters/providers/codex/bridge.go:42-60`, `internal/adapters/providers/codex/bridge_test.go:274-300`).

**Acceptance:**
- Closed mode is the default. The shared run service provisions policy material through Unit 15.2.5, injects `HTTP_PROXY`, `HTTPS_PROXY`, and `NO_PROXY`, and sets `ContainerRunRequest.Network = <internal-network>` for the workload container.
- Open mode preserves current behavior: no proxy env injection, no internal network, default Docker egress.
- The runtime policy is exact-host allowlist only. Non-HTTP/HTTPS traffic remains blocked by lack of a default external route. No SOCKS, no raw TCP allowlisting, no CIDR support in DROP_15.
- Error wrapping boundaries are explicit for: policy setup, runtime preparation, request build, docker run/create, and cleanup teardown.
- `service_test.go` is table-driven and proves:
  - default closed mode injects proxy env and internal network
  - open mode injects neither
  - cleanup runs on both success and failure paths
  - DROP_14 env vars are merged, not overwritten
- **Cross-drop env-merge gate (F1):** this unit depends on DROP_14 having re-homed env-var merging into `internal/services/run` per `drops/DROP_14_ENV_VARS/PLAN.md:69-70`; if DROP_14 still has env merging in `internal/services/claude` and `internal/services/codex` when build starts, Unit 15.3 must include that re-home or block on a DROP_14 amendment.
- `service_integration_test.go` uses `testcontainers-go` and no real-internet dependency to prove:
  - an allowlisted host service is reachable from closed mode through the proxy via `host.docker.internal`
  - a non-allowlisted hostname is denied by the proxy before external egress
- This unit remains the ship gate for "closed by default" runtime behavior.

**Blocked by:** Unit 15.1, Unit 15.2, Unit 15.2.5, DROP_13, DROP_14

---

#### Unit 15.4 — CLI network management and open-egress opt-out

**State:** todo

**Paths:**
- `internal/cli/network.go` (new, not yet in tree)
- `internal/cli/network_test.go` (new, not yet in tree)
- `internal/cli/root.go`
- `internal/cli/run.go` (new, not yet in tree; expected from DROP_13)

**Packages:** `internal/cli`

**Evidence:** `internal/cli/tools.go:17-40,43-97` is the existing project-scoped manifest command pattern; `internal/cli/root.go:108-139` is the root registration/grouping point; `internal/cli/operator_helpers.go:421-464` is the existing overlay/image-policy manifest seam that currently needs rerooting; `drops/DROP_13_GENERIC_RUN/PLAN.md:78-96` already places `valv run` in `internal/cli/run.go`.

**Acceptance:**
- Add `valv network allow <host>`, `valv network deny <host>`, and `valv network list`, all project-scoped via `project.Detect()` so they work from subdirectories.
- Every CLI read/write of `.valv/tools.toml` in this unit uses the detected project root (matching `tools validate` and Unit 15.0); never join against raw cwd. Repo-subdir invocations must mutate and report the root manifest.
- `allow` creates `.valv/tools.toml` when absent and updates only the `[allowlist]` section through `WriteAllowlistSection`.
- `deny` is idempotent and removes only user-added hosts from the persisted allowlist. Built-in default hosts remain effective and are not subtractable in DROP_15.
- `list` reports the effective allowlist for the current project, including built-in defaults and user additions.
- Add `--network open` to `valv run` (new, not yet in tree). Default remains closed when the flag is omitted.
- Do not add duplicated flag parsing to today's pre-DROP_13 `claude.go` / `codex.go`; this unit assumes those commands inherit policy through `run.go` once DROP_13 lands.
- `network_test.go` is table-driven and proves:
  - subdirectory project detection
  - fresh project root with no `.valv/` directory
  - create-from-absent-file behavior
  - lowercase normalization and dedupe
  - deny-no-op on missing hosts
  - preservation of existing `[tools]` / `[env]` block text
  - `list` includes the four built-in defaults even when no user hosts are present
  - `valv run --network open` toggles the open-mode request path

**Blocked by:** Unit 15.0, Unit 15.1, Unit 15.3, DROP_13

### Notes For Builder Agents

- **A1 — `host.docker.internal` over internal networking remains unproven.** Units 15.2.5 and 15.3 are the gates. If `host.docker.internal` does not resolve from `--internal`-networked build or runtime containers on Docker Desktop macOS, both the overlay-build path and the workload runtime must fall back to the same validated topology (for example `docker network connect <bridge-network>` plus custom routing). Empirical validation before committing the closed-default architecture is mandatory.
- **A6 — Claude/Codex CLI proxy compliance remains unproven.** Closed default changes `valv claude` / `valv codex` default behavior. The integration suite must verify that BOTH AI CLIs successfully reach Anthropic/OpenAI through the proxy. If they do not honor `HTTP_PROXY`, `--network open` becomes mandatory by default and DROP_15 must reconsider its default mode.
- Any image-build proxy integration test must force a rebuild (`EnsureProjectRequest.NoCache = true` or unique repository/tag). A freshness-label cache hit is not evidence that build-time egress is filtered.
- Keep the A3 semantics literal: built-in defaults are always present in the effective allowlist. Do not silently implement replace/subtract behavior.
- Prefer the shared `internal/services/networkpolicy` seam for both runtime and image-build policy material. Do not duplicate proxy/network setup logic between `internal/services/run` and `internal/services/images`.
- Keep the project-root contract literal: any `.valv/tools.toml` lookup that affects overlay/image policy must resolve against the detected project root, not raw cwd.
- Respect package/file blocking strictly: `15.2.5` is ordered after `15.0`, `15.1`, and `15.2`; `15.4` is ordered after `15.0` and `15.3` because these units share `internal/cli` / the DROP_13 run seam.
