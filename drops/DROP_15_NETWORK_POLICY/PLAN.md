# DROP_15 — NETWORK_POLICY

**State:** building
**Blocked by:** DROP_14 (building)
**Paths (expected):** `internal/domain/` (network-policy domain type — allowlist shape), `internal/tools/` (extend `.valv/tools.toml` `[allowlist]` parsing — currently `toml.Primitive` placeholder), `internal/adapters/providers/` (Docker `--network` wiring), `internal/adapters/docker/` (network create/remove/connect lifecycle + detached sidecar run), `internal/services/networkpolicy/` (new — sidecar-proxy provisioning + closed-default enforcement), `internal/services/images/` (image-build egress enforcement), `internal/cli/` (allowlist edit subcommands or `valv network` namespace)
**Packages (expected):** `internal/domain/`, `internal/tools/`, `internal/adapters/docker/`, `internal/adapters/providers/claude/`, `internal/adapters/providers/codex/`, new `internal/services/networkpolicy/`, `internal/services/images/`, `internal/cli/`
**PLAN.md ref:** main/PLAN.md → DROP_15_NETWORK_POLICY row
**Workflow:** main/drops/WORKFLOW.md
**Started:** 2026-05-22
**Closed:** —

## Scope

Closed-by-default outbound network for the container and DROP_12 overlay-image build, with per-project allowlist (claudebox pattern identified in the 2026-05-20 OSS survey — see `main/CLAUDE.md` § "Product Direction"). The allowlist is editable two ways:

- Via CLI command (e.g. `valv network allow <host>` / `valv network deny <host>` / `valv network list`).
- Via `.valv/tools.toml` `[allowlist]` section (DROP_11 reserved this section's parsing as `toml.Primitive` — DROP_15 promotes it to a typed schema).

Operator escape hatch for open egress ships on `valv run --network open`. DROP_15 does **not** flip `valv claude` / `valv codex` to closed-by-default; those launchers remain open-mode callers until launcher proxy compliance is separately proven.

Implementation surface: Docker container runtime accepts `--network` flags via the existing `ContainerRunRequest` plumbing (`internal/adapters/docker/types.go:43-60,133-218` — `BuildRunArgs` emits `--network <name>`). The closed-default behavior uses a **proxy sidecar container** running an HTTP/HTTPS allowlist filter. The workload container attaches ONLY to a single `--internal` Docker network (no external route, cannot bypass). The proxy sidecar attaches to BOTH the `--internal` network (reachable by the workload via a stable network alias such as `valv-proxy`) AND the default `bridge` network (so the proxy itself has external egress). The workload's `HTTP_PROXY` / `HTTPS_PROXY` point at the sidecar's internal-network alias, NOT `host.docker.internal`.

**Safety invariant (load-bearing):** only the PROXY spans two networks. The WORKLOAD stays purely internal — it has no route to anything except through the sidecar. The forbidden pattern remains attaching the WORKLOAD to `bridge`; attaching only the proxy to bridge does not give the workload a default external route, so it does not violate the closed-by-default guarantee. This sidecar design replaces the prior host-local-proxy + `host.docker.internal` topology, which DROP_15.2.5 empirically proved UNREACHABLE from a container on an `--internal` network on Docker Desktop macOS.

## Planner

### Objective

Ship DROP_15 as a closed-by-default, macOS-compatible network policy layer across BOTH runtime container egress and DROP_12 overlay-image build egress, using a **proxy sidecar container** topology. Project-scoped `.valv/tools.toml` reads that affect overlay/image policy MUST resolve from the detected project root rather than raw cwd so repo-subdir launches see the same allowlist and overlay behavior as repo-root launches. Keep the shared policy seam at post-DROP_13 `internal/services/run`, but make `valv run` the only closed-default runtime ship gate in DROP_15; `valv claude` / `valv codex` remain open-mode callers until launcher proxy compliance is proven. Add a distinct image-build unit because `BuildOverlayDockerfile` emits `RUN ["go","install",...]` / `RUN ["npm","install","-g",...]` lines that execute during `docker buildx build`, outside runtime launch policy (`internal/services/images/overlay.go:84-146`, `internal/services/images/service.go:718-838`). Use a typed `.valv/tools.toml` allowlist with built-in Go-module defaults, a proxy sidecar container that filters HTTP/HTTPS by allowlist, and a single `--internal` Docker network the workload attaches to exclusively; keep `valv run --network open` as the operator escape hatch.

### Schema Decisions

1. **Allowlist semantics are exact-host, lowercase, deduped, and union-based.** The typed `[allowlist]` schema remains exact hostname / Docker alias only: no CIDR, no URL prefixes, no scheme/path/port parsing, no wildcard syntax. DROP_15 ships four built-in default hosts in the effective allowlist: `proxy.golang.org`, `sum.golang.org`, `objects.githubusercontent.com`, and `github.com`. User-declared `[allowlist].hosts` entries are UNIONED with these defaults, not replacements. Evidence: `internal/tools/tools.go:75-130` still has no typed allowlist today; A3 locks the zero-value/default behavior.
2. **`[allowlist]` typing happens in `internal/tools`, but overlay image identity stays `[tools]`-only.** `OverlayHash` still hashes only `manifest.Tools` (`internal/services/images/overlay.go:30-70`; tests at `internal/services/images/overlay_test.go:12-147`), so DROP_15 must keep allowlist/env policy out of project-image tags to avoid rebuild churn from non-tool edits.
3. **`.valv/tools.toml` editing is section-local only for a supported file shape, with byte-preservation outside an explicitly-bounded `[allowlist]` section span.** New helper `WriteAllowlistSection(path string, cfg AllowlistConfig) error` (new, not yet in tree) rewrites only the `[allowlist]` section. Supported input is: UTF-8 text, LF line endings, no UTF-8 BOM, and no TOML multi-line strings outside `[allowlist]`. **The lexical `[allowlist]` section span is bounded as: the span begins at the first byte of the `[allowlist]` header line (the `[` character) and ends immediately before the first byte of the next top-level section header (`[<name>]` at line start), OR ends at EOF if `[allowlist]` is the last section in the file.** Within that span, the helper may rewrite freely. Outside that span — meaning the bytes from the start of file up to the first byte of the `[allowlist]` header, AND (if applicable) the bytes from the first byte of the next top-level section header through EOF — the helper MUST preserve every byte verbatim, including the file preamble (comments and whitespace before the first section), divider comments between sections, inline comments on section headers other than `[allowlist]`, and the entire `[tools]` and `[env]` block bytes. Blank lines and comment lines that fall between the `[allowlist]` header and the next top-level section header are INSIDE the span and may be rewritten as part of the `[allowlist]` rewrite. Unsupported shapes return a deterministic error instead of best-effort rewrite. It MUST create the parent `.valv/` directory when absent via `os.MkdirAll(filepath.Dir(path), 0o755)` (`go doc os.MkdirAll`, `go doc path/filepath.Dir`; `internal/tools/resolve.go:11-30` fixes the manifest path at `.valv/tools.toml`). Evidence: `internal/tools/tools.go:71-130` and `internal/tools/resolve.go:11-30` provide a parser/resolve seam today, but there is no committed writer/editor seam yet.
4. **Runtime enforcement stays on the DROP_13 shared run seam.** Closed-default network policy remains a property of the generic run service, not a provider-specific launcher copy. Evidence: DROP_13 already plans `internal/services/run/service.go` as the shared launch seam (`drops/DROP_13_GENERIC_RUN/PLAN.md:36-43,59-96`), while current Claude/Codex services still duplicate request assembly and execution (`internal/services/claude/service.go:194-220,298-329`; `internal/services/codex/service.go:185-212,302-332`).
5. **Closed mode uses a single `--internal` Docker network plus a proxy SIDECAR CONTAINER (sidecar-proxy topology).** Do NOT use literal `--network none`: Docker docs show it leaves loopback only (`https://docs.docker.com/engine/network/drivers/none/`). The supported DROP_15 topology is:
   - The **workload container** attaches to ONE `--internal` Docker network and nothing else. Docker docs confirm an `--internal` network has no external connectivity (`https://docs.docker.com/reference/cli/docker/network/create/` `--internal`; `https://docs.docker.com/manuals/compose/how-tos/networking/` `internal: true` "No external connectivity"). The workload therefore CANNOT bypass the proxy.
   - The **proxy sidecar container** attaches to BOTH the `--internal` network AND the default `bridge`. Docker docs confirm a container can connect to multiple networks either by passing `--network` multiple times at create time or via `docker network connect` for a running container, and that a network alias is set with `--alias` (`https://docs.docker.com/manuals/engine/network/` "IP address and hostname"; `https://docs.docker.com/reference/cli/docker/network/connect/`). The workload reaches the sidecar by a stable network alias on the internal network (e.g. `valv-proxy`); the sidecar reaches the internet over its bridge interface.
   - The workload's `HTTP_PROXY` / `HTTPS_PROXY` point at `http://<sidecar-alias>:<port>` (e.g. `http://valv-proxy:8080`), NOT `host.docker.internal`. The empirical reason: DROP_15.2.5 proved `host.docker.internal` is unreachable from an `--internal` network on Docker Desktop macOS, but an intra-network DNS alias IS reachable because both containers share the internal network's subnet.
   - **`NO_PROXY` correction (required):** the workload routes ALL HTTP/HTTPS egress THROUGH the sidecar — the allowlist is enforced INSIDE the proxy, not via `NO_PROXY`. `NO_PROXY` must therefore contain only loopback / intra-cluster exclusions (e.g. `localhost,127.0.0.1` and the sidecar alias itself), and MUST NOT contain the allowlist. The committed `PolicyMaterial.NoProxy = <allowlist>` semantics (`internal/services/networkpolicy/service.go:139-146,256-272`) are INVERTED for an internal-only workload — putting the allowlist in `NO_PROXY` would tell the client to reach those hosts directly, but the internal-only workload has no direct route, so every allowlisted host would fail. Unit 15.2.5 must correct this so the allowlist is the proxy filter, not a `NO_PROXY` bypass list.
   - **Safety invariant:** only the PROXY spans bridge; the workload never gains a default external route. This is materially different from the Round-4-forbidden bridge fallback, which concerned the WORKLOAD regaining a default route under multi-network `gw-priority` ambiguity (`https://docs.docker.com/network/`). Keeping the workload single-network and internal preserves the closed-by-default guarantee.
6. **Image-build egress reuses the same sidecar-proxy topology.** The overlay build path must inject the same allowlist policy into `docker buildx build` because DROP_12 tool install `RUN` lines execute at image-build time, before runtime launch. Use Docker's predefined proxy build args (`HTTP_PROXY`, `HTTPS_PROXY`, `NO_PROXY`) plus `ImageBuildRequest.Network = <internal-network>` so build `RUN` steps attach to the internal network and reach the sidecar by its alias; Docker docs confirm these proxy args do not require explicit Dockerfile `ARG` declarations (`https://docs.docker.com/manuals/build/building/variables.md`) and that `buildx build --network=<name>` runs build containers on the named network. The proxy build args point at the sidecar alias (`http://<sidecar-alias>:<port>`), matching Decision 5. The sidecar MUST be running before the build starts; the networkpolicy service provisions it once and both runtime and build callers reuse the same sidecar + network. Evidence: `internal/adapters/docker/ops.go:9-21,38-92` already supports `ImageBuildRequest.BuildArgs` (map) and `ImageBuildRequest.Network`. **Empirical-validation requirement:** if buildx build-RUN steps cannot reach a sidecar on the `--internal` network on Docker Desktop macOS (ephemeral build containers attach differently than `docker run`), Unit 15.2.5 must adopt a build-specific equivalent that still routes build egress through the sidecar-filtered path rather than reverting to a workload bridge attachment.
7. **There is no subtractive override for built-in defaults in DROP_15, and `valv network deny` must error on them.** Because Decision 1 locks union semantics, `valv network deny` removes only user-added hosts; it does not remove the built-in Go-module defaults. Attempting to deny one of the built-in defaults MUST return a deterministic error, not a success or warning, because the effective policy would remain unchanged. Evidence: current plan already fixed union semantics (Round 3), and the CLI contract has no subtractive mechanism elsewhere in the repo.
8. **Overlay/image-policy manifest resolution is project-root-based, not raw cwd-based.** `resolveProjectImage` currently does `tools.Resolve(workingDir)` (`internal/cli/operator_helpers.go:421-464`), while `runToolsValidate` already resolves root through `project.Detect()` before `tools.Resolve` (`internal/cli/tools.go:44-84`), and `project.DetectFrom(start)` already exists for explicit subdir callers (`internal/project/project.go:26-55`; tests at `internal/project/project_test.go:13-48,67-95,112-144`). DROP_15 must align every `.valv/tools.toml` read that affects overlay/image policy with that root-detection contract.
9. **DROP_15's closed-default ship gate is `valv run` plus overlay-build egress, not provider launchers.** Claude Code docs currently state support for `HTTP_PROXY`, `HTTPS_PROXY`, and `NO_PROXY` (`https://code.claude.com/docs/en/corporate-proxy`, accessed 2026-05-22). OpenAI Codex still has open proxy regressions as of 2026-05-22: issue `openai/codex#16079` opened 2026-03-28 reports HTTP-proxy failures while `curl` works, and issue `openai/codex#14080` opened 2026-03-09 reports macOS/system-proxy websocket instability while explicit env-proxy works. Repo evidence: provider launchers still build and execute their own request paths today (`internal/services/claude/service.go:194-220,298-329`; `internal/services/codex/service.go:185-212,302-332`). Therefore DROP_15 keeps `valv claude` / `valv codex` on open-mode defaults and does not make them the ship gate for closed-default readiness.

### Acceptance Criteria (drop-level)

1. `resolveProjectImage` and any other CLI path that consumes `.valv/tools.toml` for overlay/image policy resolve the manifest from the detected project root, not raw cwd, so repo-subdir launches and repo-root launches see the same overlay/allowlist policy.
2. `internal/tools.Load` / `Resolve` type-decode `[allowlist]` while keeping strict unknown-top-level rejection. The effective allowlist for a zero-value config is exactly the four built-in defaults; user entries are unioned with them.
3. `WriteAllowlistSection` creates the parent `.valv/` directory if absent and rewrites only `[allowlist]` for the supported manifest shape from Decision 3. On supported input it preserves `[tools]` and `[env]` block order/content verbatim, including comments and blank lines within those sections; unsupported shapes return a deterministic error.
4. Closed mode is the default for `valv run` and the shared run-service closed-mode path: it provisions a proxy sidecar + `--internal` network via Unit 15.2.5, attaches the workload to the `--internal` network ONLY, injects proxy env vars pointing at the sidecar alias, and blocks non-allowlisted HTTP/HTTPS egress (filtered in the proxy) plus all non-HTTP/HTTPS egress (no default route on the internal-only workload). Open mode preserves today's unconstrained behavior. `valv claude` / `valv codex` remain open-mode callers in DROP_15.
5. DROP_15.3 explicitly depends on DROP_14 having re-homed env-var merging into `internal/services/run` per `drops/DROP_14_ENV_VARS/PLAN.md:69-70`; if env merging still lives in `internal/services/claude` and `internal/services/codex` when build starts, Unit 15.3 must include that re-home or block on a DROP_14 amendment.
6. The DROP_12 overlay build path threads the same effective allowlist policy through `docker buildx build` using the manifest resolved from the detected project root and the sidecar-proxy topology from Decisions 5–6: build `RUN` steps attach to the `--internal` network (`ImageBuildRequest.Network`) and reach the sidecar by alias via the proxy build args. An integration test forces a rebuild every run (`EnsureProjectRequest.NoCache = true` or a unique repository/tag) and proves a `.valv/tools.toml`-driven `go install` during image build reaches the sidecar proxy and is filtered (one allowlisted path succeeds, one blocked path fails). The sidecar reachability is validated on Docker Desktop macOS; if buildx build-RUN steps cannot reach the sidecar on the internal network, the unit adopts the Decision 6 build-specific equivalent rather than attaching the workload/build to bridge.
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

#### Unit 15.2 — Docker network lifecycle helpers (create/remove/connect)

**State:** todo

**Paths:**
- `internal/adapters/docker/network.go`
- `internal/adapters/docker/executor.go`
- `internal/adapters/docker/network_test.go`

**Packages:** `internal/adapters/docker`

**Evidence:** `internal/adapters/docker/network.go:16-115` already carries `NetworkCreateRequest`/`BuildNetworkCreateArgs`/`NetworkRemoveRequest`/`BuildNetworkRemoveArgs` (committed R2); `internal/adapters/docker/executor.go:47-102` already carries `Executor.CreateNetwork`/`RemoveNetwork`/`ListNetworks` (committed R2); `internal/adapters/docker/types.go:43-60,133-218` carries `ContainerRunRequest` with `Detached`/`Labels`/`Network`/`Extra` and `BuildRunArgs`/`BuildCreateArgs` (single `--network`). Context7 `/docker/docs` confirms `docker network connect`, the `--alias` flag for network aliases, and that a container connects to multiple networks via repeated `--network` at create or `docker network connect` for a running container.

**Acceptance (ADDITIVE — Round-4 cut REVERSED with a now-concrete caller):**
- KEEP the committed `NetworkCreateRequest`/`BuildNetworkCreateArgs`/`Executor.CreateNetwork` and `NetworkRemoveRequest`/`BuildNetworkRemoveArgs`/`Executor.RemoveNetwork` (R2) and `Executor.ListNetworks` UNCHANGED. The `BuildNetworkCreateArgs` still emits `docker network create --internal ...`.
- ADD `NetworkConnectRequest` (new, not yet in tree) with at least `Network string` (required), `Container string` (required), and `Aliases []string` (optional). The Round-4 YAGNI cut of the connect surface is REVERSED: the sidecar-proxy redesign (Decision 5) has a concrete caller — the proxy sidecar must be `docker network connect`ed to BOTH the internal network and bridge, with a stable alias on the internal network.
- ADD `BuildNetworkConnectArgs` (new, not yet in tree) in `internal/adapters/docker/network.go`. It MUST emit `docker network connect --alias <alias> <network> <container>` (one `--alias <value>` per alias, in deterministic order), with the positional `<network> <container>` last. Required-field validation mirrors the create/remove pattern (non-empty, name-pattern-valid network; non-empty container; each alias non-empty and pattern-valid).
- ADD `Executor.ConnectNetwork` (new, not yet in tree) in `internal/adapters/docker/executor.go`, mirroring `CreateNetwork`'s shell-out pattern through `e.runner.Run`.
- Tests cover: required-field validation (missing network, missing container, empty alias), `--alias` emission with single and multiple aliases in deterministic order, and deterministic arg order for the positional `<network> <container>` tail.

**Blocked by:** nothing

---

#### Unit 15.2.5 — Shared networkpolicy service: proxy-sidecar lifecycle + image-build egress enforcement

**State:** todo

**Paths:**
- `internal/services/networkpolicy/service.go`
- `internal/services/networkpolicy/service_test.go`
- `internal/services/images/service.go`
- `internal/services/images/service_test.go`
- `internal/services/images/service_integration_test.go`

**Packages:** `internal/services/networkpolicy`, `internal/services/images`

**Evidence:** `internal/services/networkpolicy/service.go:55-272` currently provisions a single `--internal` network + `PolicyMaterial{HTTPProxyURL,HTTPSProxyURL,NoProxy,NetworkName}` with `ProxyEndpoint` example `host.docker.internal:18080` and `NO_PROXY = allowlist` — the host-local-proxy + `host.docker.internal` design DROP_15.2.5 proved unreachable from `--internal` on Docker Desktop macOS; `internal/adapters/docker/executor.go:47-102` provides `CreateNetwork`/`RemoveNetwork`/`ListNetworks` and (Unit 15.2 additive) `ConnectNetwork`; `internal/adapters/docker/types.go:43-60,133-218` provides `ContainerRunRequest{Detached,Labels,Network,Extra}` + `BuildRunArgs` for launching a detached labeled sidecar; `internal/services/images/overlay.go:84-146` emits overlay install `RUN` lines that execute at image-build time; `internal/services/images/service.go:769-849` builds overlays through `BuildOverlayDockerfile` + the rebuild path; `internal/adapters/docker/ops.go:9-21,38-92` supports `ImageBuildRequest.BuildArgs` (map) + `Network`; `internal/services/images/service_test.go:1535-1595` proves `EnsureProjectImage` has a working `NoCache` rebuild seam. Context7 `/docker/docs` confirms predefined proxy build args apply to build `RUN` steps without Dockerfile `ARG`, `--internal` networks have no external connectivity, and multi-network attach via `docker network connect --alias`. This is the A2 blocker: DROP_12 tool installs run outside runtime network policy.

**Acceptance (sidecar-proxy redesign):**
- The `internal/services/networkpolicy` service manages a **proxy SIDECAR CONTAINER lifecycle**, not just a network. `Provision` MUST:
  1. Create (or idempotently reclaim) the single `--internal` Docker network with the managed label `valv=network-policy` (existing committed behavior — keep).
  2. Launch the proxy sidecar as a detached container (`ContainerRunRequest{Detached:true, Labels:{valv:network-policy}, Network:<internal-net>, ...}`) running an HTTP/HTTPS allowlist filter configured from the effective allowlist, attached FIRST to the `--internal` network.
  3. `docker network connect` the sidecar to `bridge` (its external egress) AND ensure a stable alias on the `--internal` network (e.g. `valv-proxy`) via `--alias` (Unit 15.2 `ConnectNetwork`). The alias is what the workload's proxy env points at.
  4. Return policy material: the sidecar internal-network alias + port, the composed `HTTP_PROXY`/`HTTPS_PROXY` URLs (`http://<sidecar-alias>:<port>`), a corrected `NO_PROXY` (loopback + sidecar alias only — NOT the allowlist; see Decision 5 NO_PROXY correction), the internal-network name, and a cleanup handle.
- **NO_PROXY inversion correction (required):** the committed `PolicyMaterial.NoProxy = buildNoProxy(allowlist)` is WRONG for an internal-only workload. The allowlist is enforced INSIDE the proxy filter; `NO_PROXY` must carry only loopback/sidecar-internal exclusions so that ALL external HTTP/HTTPS egress is routed through the sidecar. Revise `Provision`/`PolicyMaterial`/`buildNoProxy` accordingly and prove the new semantics in tests.
- **Detached-container-run seam (new, not yet in tree):** the `networkpolicy` service currently depends only on `CreateNetwork`/`RemoveNetwork`/`ListNetworks` via the consumer-side `NetworkExecutor` interface. Extend that interface (and `docker.Executor`) with a detached-container run capability (e.g. `RunContainerDetached(ctx, docker.ContainerRunRequest) (containerID string, err error)` backed by `BuildRunArgs`) plus `ConnectNetwork` and a container-remove path, so the service can launch + dual-attach + tear down the sidecar. Define the new methods consumer-side on the interface so tests inject a fake.
- `internal/services/images/service.go` reuses the service for overlay builds. When policy is active, the overlay `docker buildx build` request injects predefined proxy build args `HTTP_PROXY`, `HTTPS_PROXY`, `NO_PROXY` (pointing at the sidecar alias) and sets `ImageBuildRequest.Network = <internal-network>` so build `RUN` steps reach the sidecar (Decision 6). The same provisioned sidecar serves both runtime and build callers.
- The build-policy seam consumes the manifest and effective allowlist already resolved at the detected project root (Unit 15.0); do not add any new raw-cwd `.valv/tools.toml` lookup in `internal/services/images` or `internal/services/networkpolicy`.
- The effective allowlist includes the built-in defaults from Unit 15.1, so plain `go install github.com/x/y` works without extra user allowlist entries.
- Build-policy injection must not change `OverlayHash`, project-overlay tags, or the existing freshness-label contract from DROP_12.
- `internal/services/images/service_test.go` remains table-driven and proves build args now include `--network <internal-net>` plus proxy build args (pointing at the sidecar alias) while preserving existing overlay labels and `--no-cache` behavior.
- `internal/services/images/service_integration_test.go` gains a tagged integration test proving a `.valv/tools.toml`-driven `go install` during overlay build reaches the sidecar proxy and is filtered. To prevent a false green from overlay reuse, the test MUST set `EnsureProjectRequest.NoCache = true` or use a unique repository/tag per run. The test must prove one allowlisted path succeeds and one blocked path fails via the sidecar. The test MUST use the sidecar topology the product ships; a passing workload/build bridge attachment is NOT valid evidence.
- **Docker Desktop macOS validation (A1 RESOLVED by sidecar):** the sidecar internal-network alias is reachable from an `--internal` network because both containers share the internal subnet — unlike `host.docker.internal`. The integration test validates the SIDECAR reachability path on Docker Desktop macOS. If buildx build-RUN steps cannot reach the sidecar on the internal network (ephemeral build-container attach semantics), adopt the Decision 6 build-specific equivalent that still routes through the sidecar filter; do NOT attach the workload/build to bridge.
- **Orphan-cleanup contract (covers the proxy CONTAINER now):** the service MUST handle a prior Valv invocation SIGKILLed after sidecar setup but before deferred cleanup. Tag the managed network AND the proxy sidecar container with `valv=network-policy`. On every `Provision` (and `CleanupStale`), scan for stale labeled networks AND stale labeled proxy containers; reclaim a matching live sidecar/network or deterministically remove stale ones (stop+rm the proxy container, rm the network) before creating fresh. Tests prove: a stale labeled network AND a stale labeled proxy container from a prior killed process are detected and cleaned/reused; the next launch does not fail on name/port/network collision.
- Error wrapping boundaries are explicit for: network provision, sidecar launch, network connect, policy material assembly, image-build request assembly, docker build execution, and cleanup (network + sidecar container).

**Blocked by:** Unit 15.0, Unit 15.1, Unit 15.2

---

#### Unit 15.3 — Generic-run closed-default runtime policy (workload internal-only + sidecar proxy env)

**State:** todo

**Paths:**
- `internal/services/run/service.go` (new, not yet in tree)
- `internal/services/run/service_test.go` (new, not yet in tree)
- `internal/services/run/service_integration_test.go` (new, not yet in tree; `//go:build integration`)

**Packages:** `internal/services/run`

**Evidence:** `drops/DROP_13_GENERIC_RUN/PLAN.md:36-43,59-96` already chooses `internal/services/run` as the shared runtime seam; `internal/services/claude/service.go:194-220,298-329` and `internal/services/codex/service.go:185-212,302-332` show today's duplicated launch/request-building path; provider runtimes still supply the base env maps (`internal/adapters/providers/claude/runtime.go:124-149`, `internal/adapters/providers/codex/runtime.go:111-132,178-219`); `internal/services/networkpolicy/service.go` (Unit 15.2.5 redesign) supplies the sidecar alias + proxy material; `internal/adapters/docker/types.go:43-60,171-173` shows `ContainerRunRequest.Network` emits a single `--network`.

**Acceptance:**
- The shared run service supports both open and closed policy modes. DROP_15 wires `valv run` to closed mode by default: it provisions the sidecar + internal network through Unit 15.2.5, attaches the WORKLOAD to the `--internal` network ONLY (`ContainerRunRequest.Network = <internal-network>`, no second network on the workload), and injects `HTTP_PROXY`/`HTTPS_PROXY` pointing at the sidecar alias plus the corrected `NO_PROXY` (loopback + sidecar alias only). `valv claude` / `valv codex` remain open-mode callers in this drop and are not flipped here.
- Open mode preserves current behavior: no proxy env injection, no internal network, no sidecar provision, default Docker egress.
- The runtime policy is exact-host allowlist only, enforced inside the sidecar proxy. Non-HTTP/HTTPS traffic remains blocked by the workload's lack of any default external route (internal-only). No SOCKS, no raw TCP allowlisting, no CIDR support in DROP_15.
- Error wrapping boundaries are explicit for: policy/sidecar provision, runtime preparation, request build, docker run/create, and cleanup teardown (sidecar container + network).
- `service_test.go` is table-driven and proves:
  - closed-mode path attaches the workload to the internal network ONLY and injects proxy env at the sidecar alias
  - open-mode path injects neither and attaches no internal network
  - **open-mode path does NOT invoke `internal/services/networkpolicy` setup, does NOT create a managed Docker network, and does NOT launch a proxy sidecar container** (assert via mocked policy service that `Provision`/setup methods are NOT called when `mode == open`)
  - the workload `ContainerRunRequest` never attaches to `bridge` or any second network (safety invariant: only the proxy is multi-homed)
  - cleanup runs on both success and failure paths
  - DROP_14 env vars are merged, not overwritten
- **Cross-drop env-merge gate (F1):** this unit depends on DROP_14 having re-homed env-var merging into `internal/services/run` per `drops/DROP_14_ENV_VARS/PLAN.md:69-70`; if DROP_14 still has env merging in `internal/services/claude` and `internal/services/codex` when build starts, Unit 15.3 must include that re-home or block on a DROP_14 amendment.
- `service_integration_test.go` uses `testcontainers-go` and no real-internet dependency to prove:
  - an allowlisted host service is reachable from closed mode through the sidecar proxy via the sidecar alias
  - a non-allowlisted hostname is denied by the sidecar proxy before external egress
- The integration proof MUST use the sidecar topology the product ships (workload internal-only; proxy on internal+bridge). Do NOT attach the workload to bridge or any second Docker network.
- **Docker Desktop macOS validation (A1 resolved):** the integration test validates the sidecar-alias reachability path on Docker Desktop macOS; the sidecar alias is reachable from `--internal` (shared subnet), unlike `host.docker.internal`. Automate macOS-runner coverage if available; otherwise mark manual-validation-required on Docker Desktop macOS and confirm before unit close.
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

**Evidence:** `internal/cli/tools.go:17-40,43-97` is the existing project-scoped manifest command pattern; `internal/cli/root.go:108-139` is the root registration/grouping point; `internal/cli/operator_helpers.go:421-464` is the existing overlay/image-policy manifest seam; `drops/DROP_13_GENERIC_RUN/PLAN.md:78-96` already places `valv run` in `internal/cli/run.go`.

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

- **A1 — RESOLVED by the sidecar-proxy topology.** The workload reaches the proxy via an `--internal`-network alias (e.g. `valv-proxy`), which is reachable on Docker Desktop macOS because both containers share the internal network's subnet. This replaces the dead `host.docker.internal`-over-`--internal` design (DROP_15.2.5 proved that unreachable on Docker Desktop macOS). The forbidden pattern remains attaching the WORKLOAD to `bridge` or any second network; only the PROXY sidecar spans internal+bridge. Keeping the workload single-network/internal preserves closed-by-default.
- **NO_PROXY semantics are inverted from the committed version.** The allowlist is enforced INSIDE the sidecar proxy filter, NOT via `NO_PROXY`. `NO_PROXY` carries only loopback + sidecar-alias exclusions. Putting the allowlist in `NO_PROXY` on an internal-only workload would make every allowlisted host unreachable (no direct route). See Decision 5.
- **A6 — Provider-launcher proxy compliance is not a DROP_15 ship gate.** `valv claude` / `valv codex` stay open-mode defaults in this drop. Do not silently flip them to closed-default without a separate launcher-level proof pass after Codex proxy behavior is proven stable.
- Any image-build proxy integration test must force a rebuild (`EnsureProjectRequest.NoCache = true` or unique repository/tag). A freshness-label cache hit is not evidence that build-time egress is filtered.
- Keep the A3 semantics literal: built-in defaults are always present in the effective allowlist, and denying one of them must return a deterministic error rather than a no-op success.
- Keep the `WriteAllowlistSection` contract literal: reject BOM/CRLF/multi-line-string unsupported shapes rather than normalizing or best-effort rewriting them.
- Prefer the shared `internal/services/networkpolicy` seam for both runtime and image-build policy material AND the sidecar lifecycle. Do not duplicate proxy/network/sidecar setup logic between `internal/services/run` and `internal/services/images`.
- Keep the project-root contract literal: any `.valv/tools.toml` lookup that affects overlay/image policy must resolve against the detected project root, not raw cwd.
- Respect package/file blocking strictly: `15.2.5` is ordered after `15.0`, `15.1`, and `15.2`; `15.4` is ordered after `15.0` and `15.3` because these units share `internal/cli` / the DROP_13 run seam.
- Unit 15.2 reverses the Round-4 YAGNI cut of the `docker network connect` surface — the sidecar (which must attach to BOTH internal and bridge with a stable alias) is the now-concrete caller that justifies it.
