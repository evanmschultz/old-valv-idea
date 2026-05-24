# DROP_15 — NETWORK_POLICY

**State:** building
**Blocked by:** DROP_14 (building)
**Paths (expected):** `internal/domain/` (network-policy domain type — allowlist shape), `internal/tools/` (extend `.valv/tools.toml` `[allowlist]` parsing — currently `toml.Primitive` placeholder), `internal/adapters/providers/` (Docker `--network` / `--add-host` wiring), `internal/services/networkpolicy/` (new — resolution + closed-default enforcement), `internal/services/images/` (image-build egress enforcement), `internal/cli/` (allowlist edit subcommands or `valv network` namespace)
**Packages (expected):** `internal/domain/`, `internal/tools/`, `internal/adapters/providers/claude/`, `internal/adapters/providers/codex/`, new `internal/services/networkpolicy/`, `internal/services/images/`, `internal/cli/`
**PLAN.md ref:** main/PLAN.md → DROP_15_NETWORK_POLICY row
**Workflow:** main/drops/WORKFLOW.md
**Started:** 2026-05-22
**Closed:** —

## Scope

Closed-by-default outbound network for the container and DROP_12 overlay-image build, with per-project allowlist (claudebox pattern identified in the 2026-05-20 OSS survey — see `main/CLAUDE.md` § "Product Direction"). The allowlist is editable two ways:

- Via CLI command (e.g. `valv network allow <host>` / `valv network deny <host>` / `valv network list`).
- Via `.valv/tools.toml` `[allowlist]` section (DROP_11 reserved this section's parsing as `toml.Primitive` — DROP_15 promotes it to a typed schema).

Operator escape hatch for open egress ships on `valv run --network open`. DROP_15 does **not** flip `valv claude` / `valv codex` to closed-by-default; those launchers remain open-mode callers until launcher proxy compliance is separately proven.

Implementation surface: Docker container runtime accepts `--network` and `--add-host` flags via the existing `ContainerRunRequest` plumbing. The closed-default behavior uses a host-local HTTP/HTTPS proxy with allowlist filtering plus a single internal Docker network attachment. A second-network fallback is explicitly out of scope for DROP_15 because it can silently change the container's default egress route.

## Planner

### Objective

Ship DROP_15 as a closed-by-default, macOS-compatible network policy layer across BOTH runtime container egress and DROP_12 overlay-image build egress. Project-scoped `.valv/tools.toml` reads that affect overlay/image policy MUST resolve from the detected project root rather than raw cwd so repo-subdir launches see the same allowlist and overlay behavior as repo-root launches. Keep the shared policy seam at post-DROP_13 `internal/services/run`, but make `valv run` the only closed-default runtime ship gate in DROP_15; `valv claude` / `valv codex` remain open-mode callers until launcher proxy compliance is proven. Add a distinct image-build unit because `BuildOverlayDockerfile` emits `RUN ["go","install",...]` / `RUN ["npm","install","-g",...]` lines that execute during `docker buildx build`, outside runtime launch policy (`internal/services/images/overlay.go:84-146`, `internal/services/images/service.go:718-838`). Use a typed `.valv/tools.toml` allowlist with built-in Go-module defaults, a host-local HTTP/HTTPS allowlist proxy, and one internal Docker network attachment; keep `valv run --network open` as the operator escape hatch.

### Schema Decisions

1. **Allowlist semantics are exact-host, lowercase, deduped, and union-based.** The typed `[allowlist]` schema remains exact hostname / Docker alias only: no CIDR, no URL prefixes, no scheme/path/port parsing, no wildcard syntax. DROP_15 ships four built-in default hosts in the effective allowlist: `proxy.golang.org`, `sum.golang.org`, `objects.githubusercontent.com`, and `github.com`. User-declared `[allowlist].hosts` entries are UNIONED with these defaults, not replacements. Evidence: `internal/tools/tools.go:75-130` still has no typed allowlist today; A3 locks the zero-value/default behavior.
2. **`[allowlist]` typing happens in `internal/tools`, but overlay image identity stays `[tools]`-only.** `OverlayHash` still hashes only `manifest.Tools` (`internal/services/images/overlay.go:30-70`; tests at `internal/services/images/overlay_test.go:12-147`), so DROP_15 must keep allowlist/env policy out of project-image tags to avoid rebuild churn from non-tool edits.
3. **`.valv/tools.toml` editing is section-local only for a supported file shape, with byte-preservation outside an explicitly-bounded `[allowlist]` section span.** New helper `WriteAllowlistSection(path string, cfg AllowlistConfig) error` (new, not yet in tree) rewrites only the `[allowlist]` section. Supported input is: UTF-8 text, LF line endings, no UTF-8 BOM, and no TOML multi-line strings outside `[allowlist]`. **The lexical `[allowlist]` section span is bounded as: the span begins at the first byte of the `[allowlist]` header line (the `[` character) and ends immediately before the first byte of the next top-level section header (`[<name>]` at line start), OR ends at EOF if `[allowlist]` is the last section in the file.** Within that span, the helper may rewrite freely. Outside that span — meaning the bytes from the start of file up to the first byte of the `[allowlist]` header, AND (if applicable) the bytes from the first byte of the next top-level section header through EOF — the helper MUST preserve every byte verbatim, including the file preamble (comments and whitespace before the first section), divider comments between sections, inline comments on section headers other than `[allowlist]`, and the entire `[tools]` and `[env]` block bytes. Blank lines and comment lines that fall between the `[allowlist]` header and the next top-level section header are INSIDE the span and may be rewritten as part of the `[allowlist]` rewrite. Unsupported shapes return a deterministic error instead of best-effort rewrite. It MUST create the parent `.valv/` directory when absent via `os.MkdirAll(filepath.Dir(path), 0o755)` (`go doc os.MkdirAll`, `go doc path/filepath.Dir`; `internal/tools/resolve.go:11-30` fixes the manifest path at `.valv/tools.toml`). Evidence: `internal/tools/tools.go:71-130` and `internal/tools/resolve.go:11-30` provide a parser/resolve seam today, but there is no committed writer/editor seam yet.
4. **Runtime enforcement stays on the DROP_13 shared run seam.** Closed-default network policy remains a property of the generic run service, not a provider-specific launcher copy. Evidence: DROP_13 already plans `internal/services/run/service.go` as the shared launch seam (`drops/DROP_13_GENERIC_RUN/PLAN.md:36-43,59-96`), while current Claude/Codex services still duplicate request assembly and execution (`internal/services/claude/service.go:194-220,298-329`; `internal/services/codex/service.go:185-212,302-332`).
5. **Closed mode uses a single internal Docker network plus a host-local HTTP/HTTPS allowlist proxy.** Do NOT use literal `--network none`: Docker docs show it leaves loopback only (`https://docs.docker.com/engine/network/drivers/none/`). The only supported DROP_15 topology is one `--internal` network attachment that can still reach the host-local proxy through `host.docker.internal`; Docker docs say internal networks have no default external route and may still reach the gateway / appropriately configured host services (`https://docs.docker.com/reference/cli/docker/network/create/`, `--internal`). Do NOT ship a `docker network connect bridge` fallback: Docker docs also say the default gateway on multi-network containers is selected by Docker and may change whenever network connections change unless `gw-priority` is managed (`https://docs.docker.com/network/`). Existing repo evidence only proves `host.docker.internal` is already used by the Codex bridge (`internal/adapters/providers/codex/bridge.go:42-60`) and provides no gateway-priority or post-attach egress-proof seam, so DROP_15 accepts a platform limitation instead of a speculative second-network fallback.
6. **Image-build egress is a distinct policy surface and uses Docker's predefined proxy build args on the same single-network topology.** The overlay build path must inject the same allowlist policy into `docker buildx build` because DROP_12 tool install `RUN` lines execute at image-build time, before runtime launch. Use Docker's predefined proxy build args (`HTTP_PROXY`, `HTTPS_PROXY`, `NO_PROXY`) plus `ImageBuildRequest.Network = <internal-network>`; Docker docs confirm these proxy args do not require explicit Dockerfile `ARG` declarations (`https://docs.docker.com/manuals/build/building/variables.md`). Evidence: `internal/adapters/docker/ops.go:9-21,38-72` already supports `ImageBuildRequest.Network`, and the accepted topology constraint from Decision 5 applies equally here.
7. **There is no subtractive override for built-in defaults in DROP_15, and `valv network deny` must error on them.** Because Decision 1 locks union semantics, `valv network deny` removes only user-added hosts; it does not remove the built-in Go-module defaults. Attempting to deny one of the built-in defaults MUST return a deterministic error, not a success or warning, because the effective policy would remain unchanged. Evidence: current plan already fixed union semantics (`drops/DROP_15_NETWORK_POLICY/PLAN.md` Round 3), and the CLI contract has no subtractive mechanism elsewhere in the repo.
8. **Overlay/image-policy manifest resolution is project-root-based, not raw cwd-based.** `resolveProjectImage` currently does `tools.Resolve(workingDir)` (`internal/cli/operator_helpers.go:421-464`), while `runToolsValidate` already resolves root through `project.Detect()` before `tools.Resolve` (`internal/cli/tools.go:44-84`), and `project.DetectFrom(start)` already exists for explicit subdir callers (`internal/project/project.go:26-55`; tests at `internal/project/project_test.go:13-48,67-95,112-144`). DROP_15 must align every `.valv/tools.toml` read that affects overlay/image policy with that root-detection contract.
9. **DROP_15's closed-default ship gate is `valv run` plus overlay-build egress, not provider launchers.** Claude Code docs currently state support for `HTTP_PROXY`, `HTTPS_PROXY`, and `NO_PROXY` (`https://code.claude.com/docs/en/corporate-proxy`, accessed 2026-05-22). OpenAI Codex still has open proxy regressions as of 2026-05-22: issue `openai/codex#16079` opened 2026-03-28 reports HTTP-proxy failures while `curl` works, and issue `openai/codex#14080` opened 2026-03-09 reports macOS/system-proxy websocket instability while explicit env-proxy works. Repo evidence: provider launchers still build and execute their own request paths today (`internal/services/claude/service.go:194-220,298-329`; `internal/services/codex/service.go:185-212,302-332`). Therefore DROP_15 keeps `valv claude` / `valv codex` on open-mode defaults and does not make them the ship gate for closed-default readiness.

### Acceptance Criteria (drop-level)

1. `resolveProjectImage` and any other CLI path that consumes `.valv/tools.toml` for overlay/image policy resolve the manifest from the detected project root, not raw cwd, so repo-subdir launches and repo-root launches see the same overlay/allowlist policy.
2. `internal/tools.Load` / `Resolve` type-decode `[allowlist]` while keeping strict unknown-top-level rejection. The effective allowlist for a zero-value config is exactly the four built-in defaults; user entries are unioned with them.
3. `WriteAllowlistSection` creates the parent `.valv/` directory if absent and rewrites only `[allowlist]` for the supported manifest shape from Decision 3. On supported input it preserves `[tools]` and `[env]` block order/content verbatim, including comments and blank lines within those sections; unsupported shapes return a deterministic error.
4. Closed mode is the default for `valv run` and the shared run-service closed-mode path: it provisions policy material, injects proxy env vars, sets `ContainerRunRequest.Network`, and blocks non-allowlisted HTTP/HTTPS egress plus non-HTTP/HTTPS default egress. Open mode preserves today's unconstrained behavior. `valv claude` / `valv codex` remain open-mode callers in DROP_15.
5. DROP_15.3 explicitly depends on DROP_14 having re-homed env-var merging into `internal/services/run` per `drops/DROP_14_ENV_VARS/PLAN.md:69-70`; if env merging still lives in `internal/services/claude` and `internal/services/codex` when build starts, Unit 15.3 must include that re-home or block on a DROP_14 amendment.
6. The DROP_12 overlay build path threads the same effective allowlist policy through `docker buildx build` using the manifest resolved from the detected project root and the single-network topology from Decision 5. An integration test forces a rebuild every run (`EnsureProjectRequest.NoCache = true` or a unique repository/tag) and proves a `.valv/tools.toml`-driven `go install` during image build reaches the proxy and is filtered. If `host.docker.internal` is unreachable from that `--internal` build topology on supported macOS, the unit blocks the drop rather than attaching a second network that can change default egress.
7. CLI surface ships as `valv network allow <host>`, `valv network deny <host>`, `valv network list`, plus `valv run --network open`. Pre-DROP_13 `valv claude` / `valv codex` must not gain duplicated flag parsing, and those launchers do not flip to closed-default in this drop.
8. `valv network deny <built-in-default>` returns a deterministic error and leaves the effective allowlist unchanged; tests cover that exact case.

### Units

---

#### Unit 15.0 — Re-root project-image manifest resolution to the detected project root

**State:** done

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

**State:** done

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
- `WriteAllowlistSection` supports only the manifest shape from Decision 3: UTF-8, LF, no UTF-8 BOM, and no TOML multi-line strings outside `[allowlist]`. Within that supported shape it rewrites only `[allowlist]` and MUST preserve every byte outside the lexical `[allowlist]` section span verbatim, including file preamble (comments and whitespace before the first section), divider comments between sections, inline comments on other section headers, and entire `[tools]` and `[env]` block bytes. Unsupported shapes MUST return a deterministic error instead of best-effort rewrite.
- Tests cover: fresh project root with no `.valv/` directory, create-from-absent-file behavior, default-host union semantics, normalization/dedupe, invalid host rejection, unknown top-level section rejection, supported-shape section-preservation behavior, and deterministic rejection of BOM, CRLF, and multi-line-string/header-collision inputs.
- Tests explicitly assert byte-preservation outside `[allowlist]` against a golden fixture containing: a file-preamble comment line, an inline comment on the `[allowlist]` header itself, a divider comment between `[allowlist]` and `[tools]`, and an `[env]` block with interleaved comments — round-trip rewrite must reproduce all of those bytes verbatim.
- `internal/services/images/overlay_test.go` gains a regression asserting that two manifests with identical `[tools]` but different `[allowlist]` data produce the same `OverlayHash`.

**Blocked by:** nothing

---

#### Unit 15.2 — Docker network lifecycle helpers

**State:** done

**Paths:**
- `internal/adapters/docker/network.go` (new, not yet in tree)
- `internal/adapters/docker/executor.go`
- `internal/adapters/docker/network_test.go` (new, not yet in tree)

**Packages:** `internal/adapters/docker`

**Evidence:** `internal/adapters/docker/ops.go:9-21,38-72` already carries `ImageBuildRequest.Network`; `internal/adapters/docker/types.go:43-60,141-218` already carries `ContainerRunRequest.Network`; `internal/adapters/docker/executor.go:5-40` currently exposes build/remove/prune helpers but no network lifecycle methods. Context7 `/docker/docs` confirms the required CLI surface: `docker network create --internal`, `docker network rm`, and related network-management commands.

**Acceptance:**
- Add typed request structs `NetworkCreateRequest` and `NetworkRemoveRequest` (all new, not yet in tree). Do NOT add a `NetworkConnectRequest` in DROP_15 — the second-network fallback was cut from Decision 5, leaving no in-drop caller for `docker network connect`. If a future drop requires post-create attach (e.g. for sidecars), it owns adding the connect surface.
- Add arg builders `BuildNetworkCreateArgs` and `BuildNetworkRemoveArgs` (all new, not yet in tree) in `internal/adapters/docker/network.go`.
- Add `Executor.CreateNetwork` and `Executor.RemoveNetwork` (all new, not yet in tree) in `internal/adapters/docker/executor.go`.
- `BuildNetworkCreateArgs` emits `docker network create --internal ...`.
- Tests cover required-field validation, `--internal` emission, and deterministic arg order.

**Blocked by:** nothing

---

#### Unit 15.2.5 — Shared networkpolicy service and image-build egress enforcement

**State:** done

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
- The accepted build topology is a single internal Docker network attachment that can still reach the host-local proxy via `host.docker.internal`. Do NOT attach a second Docker network in DROP_15. If validation shows the proxy is unreachable from that topology on supported macOS, this unit must fail/block the drop rather than `docker network connect bridge`.
- Build-policy injection must not change `OverlayHash`, project-overlay tags, or the existing freshness-label contract from DROP_12.
- `internal/services/images/service_test.go` remains table-driven and proves build args now include `--network` plus proxy build args while preserving existing overlay labels and `--no-cache` behavior.
- `internal/services/images/service_integration_test.go` gains a tagged integration test proving a `.valv/tools.toml`-driven `go install` during overlay build reaches the host proxy and is filtered. To prevent a false green from overlay reuse, the test MUST set `EnsureProjectRequest.NoCache = true` or use a unique repository/tag per run to force rebuild every execution. The builder should avoid a live-internet dependency if practical by using a disposable local fixture/proxy path; at minimum the test must prove one allowlisted path succeeds and one blocked path fails via the proxy. The test MUST run on the same single-network topology the product ships; a passing multi-network fallback is not valid evidence.
- **Docker Desktop macOS gate (Round 4 R3.F4.1.1 mitigation):** A1's `host.docker.internal` reachability claim is platform-specific. The integration test must be validated on Docker Desktop macOS; Linux CI runs are NOT evidence for A1 because `host.docker.internal` semantics differ across Docker Engine deployments. If the test is run only on Linux CI during build, that result does not satisfy the unit. Either automate macOS-runner coverage OR mark the test as manual-validation-required on Docker Desktop macOS in builder notes and confirm validation before unit close.
- **Orphan-cleanup contract (Round 4 R3.F3.1 mitigation):** `internal/services/networkpolicy` MUST handle the case where a prior Valv invocation was SIGKILLed after policy setup but before deferred cleanup ran. Approach: tag every managed Docker network and host proxy process with a deterministic Valv-owned label/metadata (e.g. `label=valv=network-policy` on networks; pidfile or label-equivalent on host proxy), AND on every startup either (a) reclaim/reuse a matching existing resource or (b) deterministically clean stale resources before creating new ones. Tests prove: a stale labeled network from a prior killed process is detected and cleaned/reused; the next launch does not fail on naming/port collision.
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
- The shared run service supports both open and closed policy modes. DROP_15 wires `valv run` to closed mode by default: it provisions policy material through Unit 15.2.5, injects `HTTP_PROXY`, `HTTPS_PROXY`, and `NO_PROXY`, and sets `ContainerRunRequest.Network = <internal-network>`. `valv claude` / `valv codex` remain open-mode callers in this drop and are not flipped here.
- Open mode preserves current behavior: no proxy env injection, no internal network, default Docker egress.
- The runtime policy is exact-host allowlist only. Non-HTTP/HTTPS traffic remains blocked by lack of a default external route. No SOCKS, no raw TCP allowlisting, no CIDR support in DROP_15.
- Error wrapping boundaries are explicit for: policy setup, runtime preparation, request build, docker run/create, and cleanup teardown.
- `service_test.go` is table-driven and proves:
  - closed-mode path injects proxy env and internal network
  - open-mode path injects neither
  - **open-mode path does NOT invoke `internal/services/networkpolicy` setup, does NOT create a managed Docker network, and does NOT start a host proxy process** (Round 4 R3.F1.3 mitigation — assert via mocked policy service that `Provision` / setup methods are NOT called when `mode == open`)
  - cleanup runs on both success and failure paths
  - DROP_14 env vars are merged, not overwritten
- **Cross-drop env-merge gate (F1):** this unit depends on DROP_14 having re-homed env-var merging into `internal/services/run` per `drops/DROP_14_ENV_VARS/PLAN.md:69-70`; if DROP_14 still has env merging in `internal/services/claude` and `internal/services/codex` when build starts, Unit 15.3 must include that re-home or block on a DROP_14 amendment.
- `service_integration_test.go` uses `testcontainers-go` and no real-internet dependency to prove:
  - an allowlisted host service is reachable from closed mode through the proxy via `host.docker.internal`
  - a non-allowlisted hostname is denied by the proxy before external egress
- The integration proof MUST use the same single-network topology the product ships. Do NOT attach bridge or any second Docker network in DROP_15. If the proxy is unreachable from that topology on supported macOS, this unit blocks the drop.
- **Docker Desktop macOS gate (Round 4 R3.F4.1.1 mitigation):** the integration test must be validated on Docker Desktop macOS; Linux CI runs are NOT sufficient evidence because the `host.docker.internal` reachability semantics differ across Docker Engine deployments. Automate macOS-runner coverage if available; otherwise mark the test as manual-validation-required and confirm before unit close.
- This unit remains the ship gate for `valv run` closed-default runtime behavior.

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
- `deny` is idempotent for missing user-added hosts but MUST return a deterministic error when the target is one of the built-in default hosts, because the effective policy would remain unchanged. Built-in default hosts remain effective and are not subtractable in DROP_15.
- `list` reports the effective allowlist for the current project, including built-in defaults and user additions.
- Add `--network open` to `valv run` (new, not yet in tree). Default remains closed when the flag is omitted.
- Do not add duplicated flag parsing to today's pre-DROP_13 `claude.go` / `codex.go`; those launchers remain open-mode callers in DROP_15 and do not flip default egress here.
- `network_test.go` is table-driven and proves:
  - subdirectory project detection
  - fresh project root with no `.valv/` directory
  - create-from-absent-file behavior
  - lowercase normalization and dedupe
  - deny-no-op on missing user hosts
  - deterministic error on denying one of the four built-in defaults
  - preservation of existing `[tools]` / `[env]` block text for the supported manifest shape
  - `list` includes the four built-in defaults even when no user hosts are present
  - `valv run --network open` toggles the open-mode request path

**Blocked by:** Unit 15.0, Unit 15.1, Unit 15.3, DROP_13

### Notes For Builder Agents

- **A1 — `host.docker.internal` over the shipped internal-network topology remains unproven.** Units 15.2.5 and 15.3 are the gates. If `host.docker.internal` does not resolve/reach the host-local proxy from the single `--internal` network topology on Docker Desktop macOS, block DROP_15 on that platform. Do **not** attach `bridge` or any second network in DROP_15 as a fallback.
- **A6 — Provider-launcher proxy compliance is not a DROP_15 ship gate.** `valv claude` / `valv codex` stay open-mode defaults in this drop. Do not silently flip them to closed-default without a separate launcher-level proof pass after Codex proxy behavior is proven stable.
- Any image-build proxy integration test must force a rebuild (`EnsureProjectRequest.NoCache = true` or unique repository/tag). A freshness-label cache hit is not evidence that build-time egress is filtered.
- Keep the A3 semantics literal: built-in defaults are always present in the effective allowlist, and denying one of them must return a deterministic error rather than a no-op success.
- Keep the `WriteAllowlistSection` contract literal: reject BOM/CRLF/multi-line-string unsupported shapes rather than normalizing or best-effort rewriting them.
- Prefer the shared `internal/services/networkpolicy` seam for both runtime and image-build policy material. Do not duplicate proxy/network setup logic between `internal/services/run` and `internal/services/images`.
- Keep the project-root contract literal: any `.valv/tools.toml` lookup that affects overlay/image policy must resolve against the detected project root, not raw cwd.
- Respect package/file blocking strictly: `15.2.5` is ordered after `15.0`, `15.1`, and `15.2`; `15.4` is ordered after `15.0` and `15.3` because these units share `internal/cli` / the DROP_13 run seam.
