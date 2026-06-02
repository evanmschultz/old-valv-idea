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

**State:** done

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

#### Unit 15.2.5 — Networkpolicy sidecar lifecycle + image-build egress (decomposed Round 1, R2 fold)

**State:** todo (sub-decomposed)
**Blocked by:** Unit 15.0 (done), Unit 15.1 (done), Unit 15.2 (done)

The prior single-unit spec (Round 0, commit `1c25df8`) is OVER BUDGET per `aa130dd`. Round 1 codex planner decomposed into 5 atomic droplets + 3 sub-planners. R1 plan-QA found 2 material findings; R2 orch-direct folds them: **(a) DELETE 15.2.5.F** — its production scope (proxy build-args + `policyMaterial.NetworkName` Network injection at `images/service.go:850-908`) is already committed in HEAD `6ddf5d7`; the future "swap external `proxyEndpoint` for sidecar-supplied alias" work absorbs into 15.2.5.D.1's `Service.Provision` rewrite. **(b) EXPAND 15.2.5.A** to add BOTH `RunContainerDetached` AND `ConnectNetwork` to the consumer-side `NetworkExecutor` interface (D.1 calls `ConnectNetwork`; no prior droplet owned that interface extension).

Locked design (do not re-litigate): sidecar-proxy topology with workload attached ONLY to `--internal` net (V7 per-project naming `sha256(projectRoot+allowlist)`), proxy sidecar attached to both `--internal` + `bridge` via `docker network connect --alias valv-proxy`, `NO_PROXY` carries only loopback + sidecar alias (NOT allowlist), allowlist enforced inside the proxy filter (HTTPS via SNI/CONNECT), `valv=network-policy` label on net AND container, concurrency-safe Provision with reclaim-on-collision.

##### Unit 15.2.5.A — Detached-run Executor seam + ConnectNetwork interface extension

- state: done
- blocked_by: none
- paths: `internal/adapters/docker/executor.go`, `internal/services/networkpolicy/service.go` (interface extension)
- packages: `./internal/adapters/docker`, `./internal/services/networkpolicy`
- change: (1) Add `Executor.RunContainerDetached(ctx, docker.ContainerRunRequest) (string, error)` (new — not yet in tree). Reuse existing `BuildRunArgs` + `ContainerRunRequest.Detached` + `Executor.RemoveContainer`. (2) Extend the networkpolicy consumer-side `NetworkExecutor` interface (currently `CreateNetwork`/`RemoveNetwork`/`ListNetworks` at service.go:58-65) with BOTH `RunContainerDetached` AND `ConnectNetwork` methods (both new — not yet on the interface; `ConnectNetwork` already exists on `docker.Executor` at executor.go:72, just absent from the consumer-side interface). Extend `fakeNetworkExecutor` (test-side, excluded from prod budget) with stubs for both new methods.
- acceptance: `mage testPkg ./internal/adapters/docker` + `mage testPkg ./internal/services/networkpolicy` GREEN. Tests prove detached run returns container id via runner output path; command errors wrap; networkpolicy fake executor compiles with both new methods.
- measurement: 3 production symbols (`Executor.RunContainerDetached` + 2 interface method additions on `NetworkExecutor`). At the 3-symbol ceiling but at the boundary — interface method additions are 1-line embeddings and the cohesive purpose is "expose Docker primitives needed by D.1's sidecar lifecycle". ~50 prod LOC, 2 prod files. **At-ceiling under budget**; if builder measures actual diff over 80 LOC, split into A.1 (Executor method) + A.2 (interface extension).

##### Unit 15.2.5.B — Proxy image/binary (SUB-PLANNER)

- state: todo (kind=plan)
- blocked_by: none
- scope: minimal Go HTTP CONNECT proxy + SNI/CONNECT host filter from effective allowlist + Dockerfile/image build wiring. Expected child droplets: B.1 (allowlist matcher / CONNECT decision), B.2 (proxy server main/handler), B.3 (Dockerfile/build wiring).
- expected paths: new proxy package + Dockerfile.
- measurement: estimated ≥3 prod symbols, >80 LOC, likely >3 files → emit sub-planner.
- consumer: D.1 sidecar launch.

##### Unit 15.2.5.C — PolicyMaterial NO_PROXY inversion

- state: done
- blocked_by: none
- paths: `internal/services/networkpolicy/service.go`
- packages: `./internal/services/networkpolicy`
- change: Change existing `buildNoProxy` to return only loopback entries + sidecar alias `valv-proxy`. Allowlist hosts NEVER appear in `NO_PROXY`.
- acceptance: `mage testPkg ./internal/services/networkpolicy` GREEN. Table-driven tests cover empty allowlist, non-empty allowlist, existing loopback entries.
- measurement: 1 prod symbol, ~20 prod LOC, 1 prod file. Under budget.

##### Unit 15.2.5.D.1 — Provision sidecar core lifecycle

- state: todo
- blocked_by: 15.2.5.A, 15.2.5.B, 15.2.5.C
- paths: `internal/services/networkpolicy/service.go`
- packages: `./internal/services/networkpolicy`
- change: Change existing `Service.Provision` to create/reuse V7 internal network → call `RunContainerDetached` for proxy sidecar → connect proxy to bridge → attach `valv-proxy` alias on internal network → return `PolicyMaterial` with sidecar-alias proxy URLs.
- acceptance: `mage testPkg ./internal/services/networkpolicy` GREEN. Tests assert operation order: net create/reuse, detached run, bridge connect, internal alias, returned `PolicyMaterial.NetworkName` + proxy URLs + `NO_PROXY`.
- measurement: 1 changed prod symbol (`Service.Provision`), ~75 prod LOC, 1 prod file. Near LOC ceiling but under budget.
- qa-routed precondition (from 15.2.5.C falsification, commit `da7ffb5`): promote the `valv-proxy` string literal to a shared `const ProxyAlias = "valv-proxy"` in the networkpolicy package and use it in BOTH `buildNoProxy` (already shipped with the literal — refactor it) AND this unit's `ConnectNetwork` alias, so the `NO_PROXY` exclusion and the real sidecar alias can never diverge. If this pushes D.1 over the LOC ceiling, split the const-introduction into its own micro-unit D.0.

##### Unit 15.2.5.D.2 — Provision readiness probe

- state: todo
- blocked_by: 15.2.5.D.1
- paths: `internal/services/networkpolicy/service.go`
- packages: `./internal/services/networkpolicy`
- change: Add readiness wait/probe helper (new — not yet in tree) called before returning `PolicyMaterial`. Wait until sidecar is listening (e.g. TCP connect to proxy port).
- acceptance: `mage testPkg ./internal/services/networkpolicy` GREEN. Tests cover ready/timeout/probe-error propagation without real Docker.
- measurement: 1 prod symbol, ~55 prod LOC, 1 prod file. Under budget.

##### Unit 15.2.5.E — Orphan cleanup + concurrency-safe Provision (SUB-PLANNER)

- state: todo (kind=plan)
- blocked_by: 15.2.5.D.1
- scope: label `valv=network-policy` on net AND proxy container; startup sweep + reclaim-on-collision; teardown stops/removes proxy THEN removes network. Expected children: E.1 (label material propagation), E.2 (reclaim-on-collision + stale sweep), E.3 (cleanup ordering).
- expected paths: `internal/services/networkpolicy/service.go`, possibly `internal/adapters/docker` types if labels are missing.
- measurement: expected ≥3 prod symbols, >80 LOC → emit sub-planner.

##### Unit 15.2.5.F — DELETED (already-shipped work)

R2 plan-QA finding CF-1: `internal/services/images/service.go:850-908` already contains the complete proxy build-args + Network-field injection F claimed to add (committed in a prior round alongside the initial networkpolicy skeleton). F is removed from the build plan. The future "swap external `proxyEndpoint` for sidecar-supplied alias" work absorbs into 15.2.5.D.1's `Service.Provision` rewrite (which sets `PolicyMaterial.HTTPProxyURL` from the sidecar alias instead of `request.ProxyEndpoint`). Test coverage for the post-D.1 PolicyMaterial shape lives in D.1's existing test scope. If `s.proxyEndpoint` becomes obsolete post-D.1, that field-removal is captured by D.1's test that asserts the new PolicyMaterial uses the internal alias.

##### Unit 15.2.5.G — Shipped topology integration test (SUB-PLANNER)

- state: todo (kind=plan)
- blocked_by: 15.2.5.D.2, 15.2.5.F (+ 15.2.5.E for stable repeated runs)
- scope: `//go:build integration` test with `EnsureProjectRequest.NoCache=true` + `.valv/tools.toml` `go install`; prove workload is internal-only and reaches sidecar on `valv-proxy`; proxy is on internal+bridge. Empirical Docker Desktop macOS validation. Expected children: G.1 (fixture + Docker availability harness), G.2 (topology assertion), G.3 (successful proxy egress during overlay `go install`).
- expected paths: integration test files under `internal/services/images` and/or `internal/services/networkpolicy`.
- measurement: integration test scope spans multiple behavioral assertions → emit sub-planner.
- qa-routed (from 15.2.5.C falsification, commit `da7ffb5`): when wiring the images service to the real `networkpolicy.Service`, update the stale `NoProxy` fixture in `internal/services/images/service_test.go:1801,1862` — it still hardcodes the OLD inverted value (`github.com,objects.githubusercontent.com,proxy.golang.org,sum.golang.org`); the test passes only because the fake echoes its own input. Replace with `127.0.0.1,localhost,valv-proxy`.

##### Build order (R2 fold — F removed)

- **Level 0** (parallel): 15.2.5.A, 15.2.5.B (sub-planner), 15.2.5.C
- **Level 1**: 15.2.5.D.1 (after A, B, C)
- **Level 2** (parallel): 15.2.5.D.2, 15.2.5.E (sub-planner)
- **Level 3**: 15.2.5.G (sub-planner) after D.2 (+ E for stable repeated runs)

---

#### Unit 15.3 — Generic-run closed-default runtime policy (decomposed Round 1, R2 fold)

**State:** todo (sub-decomposed)
**Blocked by:** Unit 15.0 (done), Unit 15.1 (done), Unit 15.2 (done), Unit 15.2.5 (todo — see per-droplet blockers; A unblocked, B.1/B.2/C/D blocked by 15.2.5 close), DROP_13 (done), DROP_14 (done)

R1 plan-QA FAILED on 15.3.B (4-5 prod symbols, not 2) and flagged 15.3.A as over-blocked. R2 orch-direct folds: **(a) SPLIT 15.3.B into B.1 (DI wiring) + B.2 (Run flow)**; **(b) DROP 15.3.A's blocked_by to "15.2.5 plan accepted"** (A has zero functional dep on 15.2.5 — only B/C/D do); **(c) commit to `LaunchRequest.NetworkPolicy` placement** (per-launch, not Options-wide); **(d) clarify the new consumer-side `Policy` interface as `[NEW: emerges from 15.2.5.D.1]`**; **(e) fix D's mage target** (`mage integration` covers only `./internal/cli` — D's integration test in `./internal/services/run` needs the mage target extended OR use a per-package integration invocation).

DROP_14 already shipped `LaunchRequest.AccountEnv` + run-service env merge — 15.3 doesn't need to re-home env work.

##### Unit 15.3.A — Network policy mode inputs

- state: todo
- blocked_by: 15.2.5 plan accepted (NOT all 15.2.5.* closed — A has zero import dep on `networkpolicy`); cross-unit: DROP_14 (done)
- paths: `internal/services/run/service.go`, `internal/services/run/service_test.go`
- packages: `./internal/services/run`
- change: Add `NetworkPolicyMode` type + constants `NetworkPolicyOpen` and `NetworkPolicyClosed` (new — not yet in tree). Add `NetworkPolicy NetworkPolicyMode` field to **`LaunchRequest`** (NOT `Options` — per-launch state per repo idiom; matches `Args`/`Command` placement).
- acceptance: `mage testPkg ./internal/services/run` GREEN. Table-driven tests: zero-value default is open (`NetworkPolicyOpen == ""` or zero-value), explicit open preserves current request shape, explicit closed sets the mode value but does NOT yet trigger provisioning (that wires in B.2).
- measurement: 2 prod symbols (type+const cluster + `LaunchRequest` field), ~30 prod LOC, 1 prod file. Under budget. **A can fire in parallel with 15.2.5 building** since it imports no 15.2.5 symbols.

##### Unit 15.3.B.1 — DI wiring: Policy provisioner injection

- state: todo
- blocked_by: 15.3.A; ALL 15.2.5.* closed (B.1 references the new consumer-side `Policy` interface whose shape depends on 15.2.5.D.1's final `Service.Provision` signature)
- paths: `internal/services/run/service.go`, `internal/services/run/service_test.go`
- packages: `./internal/services/run`
- change: (1) Define new consumer-side interface `Policy` in `services/run` (e.g. `type Policy interface { Provision(ctx context.Context, req networkpolicy.ProvisionRequest) (networkpolicy.PolicyMaterial, networkpolicy.Cleanup, error) }` — exact shape from 15.2.5.D.1's signature; new — not yet in tree). (2) Add `Policy Policy` field to existing `Options` struct. (3) Add corresponding unexported mirror field on `Service` struct. (4) Extend `New` constructor with field validation (`Policy` may be nil only when no closed-mode launches occur).
- acceptance: `mage testPkg ./internal/services/run` GREEN. Tests cover: `New` rejects/accepts the new field per validation rules; `Service` struct holds the policy reference; existing constructor tests still pass.
- measurement: 3 prod symbols (`Policy` interface + `Options` field-add cluster + `Service` field-add cluster + `New` validation as cohesive DI cluster — counted as 2 distinct top-level symbol clusters since they share the "DI plumbing" purpose). ~30 prod LOC, 1 prod file. **At ceiling**; if builder measures >3 distinct symbols on the actual diff, split off the interface type to its own file `services/run/policy.go`.

##### Unit 15.3.B.2 — Run flow: closed-mode provisioning call + cleanup defer

- state: todo
- blocked_by: 15.3.B.1; ALL 15.2.5.* closed
- paths: `internal/services/run/service.go`, `internal/services/run/service_test.go`
- packages: `./internal/services/run`
- change: Edit existing `Service.Run` (service.go:181-219) to add closed-mode branch: when `launch.NetworkPolicy == NetworkPolicyClosed`, call `s.policy.Provision(ctx, req)` → defer cleanup on success → propagate provision-error with run-service context (NO Docker call). Open mode unchanged.
- acceptance: `mage testPkg ./internal/services/run` GREEN. Tests: closed mode invokes provisioning with the expected `ProvisionRequest` shape (allowlist sourced from somewhere reasonable — see open-question below); open mode does NOT invoke provisioning; provision errors wrap with run-service context without calling Docker; deferred cleanup runs on both success and failure paths.
- measurement: 1 prod symbol (`Service.Run` method edit cluster), ~30 prod LOC, 1 prod file. Under budget.
- open question (route to dev or B.2 builder): where does the `ProvisionRequest.Allowlist` come from? `LaunchRequest` doesn't currently carry it. Possible sources: (a) wire allowlist into `LaunchRequest` via a new field (would push A's spec — likely defer to B.2); (b) inject an allowlist resolver into `Options`; (c) caller passes effective allowlist via `LaunchRequest.NetworkPolicyAllowlist []string` (new field, additive to A's enum). B.2 builder must surface this before coding.

##### Unit 15.3.C — Apply PolicyMaterial to workload request

- state: todo
- blocked_by: 15.3.B.2; ALL 15.2.5.* closed
- paths: `internal/services/run/service.go`, `internal/services/run/service_test.go`
- packages: `./internal/services/run`
- change: Change `buildRequest` (service.go:234-298) to merge `PolicyMaterial` into the existing env-merge cluster: inject `HTTP_PROXY`/`HTTPS_PROXY`/`NO_PROXY` as account-env-style entries that `Prepared.Env` STILL wins over on collision (preserves 14.4.B's invariant); set `ContainerRunRequest.Network = PolicyMaterial.NetworkName` (single-network safety invariant — workload attaches ONLY to internal-net).
- acceptance: `mage testPkg ./internal/services/run` GREEN. Tests: closed-mode injects `HTTP_PROXY`/`HTTPS_PROXY`/`NO_PROXY` at expected map keys; existing account/prepared env precedence preserved (`Prepared.Env` wins on collision per 14.4 invariant); workload `Network == material.NetworkName` (single-network); open mode keeps empty `Network` + no proxy env.
- measurement: 2 prod symbols (`buildRequest` body edit + optional helper for proxy-env injection if extracted), ~25 NET-ADDED prod LOC (existing `buildRequest` is 64 LOC, after edit ≤85 LOC TOTAL but only ~25 of that is new). 1 prod file. Under budget (net-added measurement).

##### Unit 15.3.D — Integration proof for closed-mode reachability

- state: todo
- blocked_by: 15.3.C; ALL 15.2.5.* closed
- paths: `internal/services/run/service_integration_test.go` (new — not yet in tree; `//go:build integration`)
- packages: `./internal/services/run`
- change: Integration tests ONLY (no production symbols). Uses testcontainers-go: start 15.2.5 sidecar, launch workload on internal-net only, verify allowed host reaches through proxy, denied host blocked, workload inspection proves single-network attachment.
- acceptance: D's mage target needs amendment — `mage integration` currently runs `go test -tags=integration -count=1 ./internal/cli` only. D requires EITHER (a) extending `magefile.go`'s `Integration` target to also cover `./internal/services/run`, OR (b) running `mage testPkg ./internal/services/run -- -tags=integration` (if mage supports build-tag passthrough). Builder must add option (a) — extend the existing `Integration` target — as part of D's commit, so `mage integration` covers both packages. New mage-target line is a magefile-side production edit (one symbol in `magefile.go`) — D becomes 1 prod symbol via this magefile extension. Tests themselves are 0 prod-symbol integration coverage.
- measurement: 1 prod symbol (magefile.go `Integration` target extension), ~3 prod LOC, 1 prod file. Under budget.

##### Build order (R2 fold)

- **Level 0** (parallel with 15.2.5 building): 15.3.A
- **Level 1** (after ALL 15.2.5.* closed): 15.3.B.1
- **Level 2**: 15.3.B.2 (after B.1)
- **Level 3**: 15.3.C (after B.2)
- **Level 4**: 15.3.D (after C)

---

#### Unit 15.4 — CLI network management + open-egress opt-out (decomposed Round 1)

**State:** todo (sub-decomposed)
**Blocked by:** Unit 15.0 (done), Unit 15.1 (done), Unit 15.3 (todo — only the SUB-PLANNER 15.4.D actually waits for 15.3 built; 15.4.A/B/C only need 15.3 plan accepted)

Round 1 codex planner decomposed into 3 atomic droplets (network list / allow / deny — serialized on shared file) + 1 sub-planner (`valv run --network open` blocked on 15.3 built since the runtime API isn't verified yet). Confirmed live symbols via Hylla + targeted source reads:
- `tools.WriteAllowlistSection(path string, cfg AllowlistConfig) error`
- `tools.EffectiveAllowlist(cfg AllowlistConfig) ([]string, error)` (NOT `LoadEffectiveAllowlist`)
- `tools.DefaultAllowlistHosts() []string` (the 4 built-ins)
- `project.Detect()` / `project.DetectFrom(start string) (Result, error)`
- `internal/cli/run.go newRunCommand` (existing — flag parsing via `stripRunLocalFlags` → `parsedRunFlags`)

##### Unit 15.4.A — `valv network list`

- state: todo
- blocked_by: 15.0 (done), 15.1 (done), 15.3 plan accepted (NOT 15.3 built)
- paths: `internal/cli/network.go` (new — not yet in tree), `internal/cli/network_test.go` (new — not yet in tree), `internal/cli/root.go` (edit — add `cmd.AddCommand(newNetworkCommand(paths, opts))` to existing AddCommand list at root.go:141)
- packages: `./internal/cli`
- change: Create `newNetworkCommand` (new) + `runNetworkList` (new). The new command sets `networkCmd.GroupID = "runtime"` matching the pattern at root.go:131 (`runCmd.GroupID = "runtime"`). Root registration is a single-line additive edit to existing `cmd.AddCommand(...)` list, not a new symbol. Use `tools.Load` (or current manifest accessor — `internal/cli/operator_helpers.go` for pattern) to read the existing manifest, then `tools.EffectiveAllowlist` to compute the merged built-in + user host set for output.
- acceptance: `mage testPkg ./internal/cli` GREEN. Tests:
  - **Cobra-tree wiring proof (R2 added per plan-QA finding):** `cmd.SetArgs([]string{"network", "list"}); cmd.Execute()` succeeds without error and stdout contains all 4 built-in hosts (`github.com`, `objects.githubusercontent.com`, `proxy.golang.org`, `sum.golang.org`). This catches root.go registration omissions that handler-only tests would miss.
  - List includes built-ins for fresh project with no `.valv/`.
  - Subdir project detection: invoking from a nested directory finds the root-level manifest.
  - User-added hosts appear unioned with built-ins.
- measurement: 2 prod symbols (`newNetworkCommand` + `runNetworkList` — the root.go AddCommand edit is an existing-symbol body edit, not a new symbol), ~70 prod LOC, **3 prod files** (network.go new + network_test.go new + root.go edited). Under budget (≤3 files at boundary).

##### Unit 15.4.B — `valv network allow <host>`

- state: todo
- blocked_by: 15.4.A
- paths: `internal/cli/network.go`, `internal/cli/network_test.go`
- packages: `./internal/cli`
- change: Extend `newNetworkCommand` with `allow` subcommand + `runNetworkAllow` (new). Use `project.Detect()`, load existing user allowlist via `tools.Load` (the manifest reader at `internal/tools/`), normalize lowercase/dedupe per `tools.EffectiveAllowlist` semantics, persist user section via `tools.WriteAllowlistSection`.
- acceptance: `mage testPkg ./internal/cli` GREEN. Tests: create from absent manifest; lowercase normalization; dedupe. Byte-preservation outside `[allowlist]` is already guaranteed by 15.1's `WriteAllowlistSection` (test coverage exists in `internal/tools/allowlist_test.go`); 15.4.B's tests rely on that helper rather than re-proving it.
- measurement: 2 prod symbols, ~70 prod LOC, 2 prod files (same files as A — file lock forces serialization on B blocked_by A). Under budget.

##### Unit 15.4.C — `valv network deny <host>`

- state: todo
- blocked_by: 15.4.B
- paths: `internal/cli/network.go`, `internal/cli/network_test.go`
- packages: `./internal/cli`
- change: Extend `newNetworkCommand` with `deny` subcommand + `runNetworkDeny` (new). Remove only user-added hosts; return deterministic error for built-in defaults from `tools.DefaultAllowlistHosts()`.
- acceptance: `mage testPkg ./internal/cli` GREEN. Tests: no-op on missing user host; deterministic error on built-in default; preserve `[tools]` + `[env]` block bytes outside `[allowlist]`.
- measurement: 2 prod symbols, ~60 prod LOC, 2 prod files (same files). Under budget.

##### Unit 15.4.D — `valv run --network open` (SUB-PLANNER)

- state: todo (kind=plan)
- blocked_by: 15.3 BUILT (D droplet must verify 15.3's runtime API exists), 15.4.C
- scope: Add `--network open` flag wiring on `valv run`. Default remains closed when flag omitted. Sub-planner is justified by the **measured ≥3 prod symbols** required across `parsedRunFlags` + `stripRunLocalFlags` + `runRunCommand` (single droplet would breach `aa130dd`'s 3-symbol budget). The "15.3 API not yet verified" timing concern is a secondary consideration — decomposition can run against 15.3.A's spec'd shape (`LaunchRequest.NetworkPolicy`) per Rule 4, but build must wait for 15.3 BUILT.
- expected paths: `internal/cli/run.go`, `internal/cli/run_test.go`, plus 15.3 runtime integration paths.
- measurement: prod symbols unknown until 15.3 API exists → emit sub-planner.

##### Build order

15.4.A → 15.4.B → 15.4.C (all serialized on `internal/cli/network.go`) → 15.4.D sub-planner (after 15.3 built + 15.4.C).

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

---

## Round-2 Sub-decompositions (15.2.5.B/E/G + 15.4.D)

Codex sub-planners returned (4 dispatches, each ≤4 hylla calls; most hit indexing failures and fell back to read-only rg/sed on committed source — fallback acceptable per evidence-order rule). Tool-call audits ✓ on all 4; no git/edit/mage out-of-scope.

### 15.2.5.B — Proxy image/binary (4 droplets, serialized)

New package `internal/cmd/valv-proxy/`. Env-injected runtime config: port via `VALV_PROXY_ADDR` (default `:8080`); allowlist via `VALV_PROXY_ALLOWLIST` (comma-sep exact hosts). Out of scope: wildcards, TLS interception, auth, config-file mounts.

- **15.2.5.B.1** — state: done — `internal/cmd/valv-proxy/allowlist.go` + test: `parseAllowlist` + `hostAllowed` (exact-host matcher, case/port-normalized, no wildcards). 2 prod symbols, ~45 LOC, 1 file. blocked_by: none.
- **15.2.5.B.2** — state: done — `internal/cmd/valv-proxy/main.go` + test: proxy `main` + HTTP/CONNECT handler. Reads `VALV_PROXY_ALLOWLIST` + `VALV_PROXY_ADDR`. 2 prod symbols, ~75 LOC, 1 file. blocked_by: B.1.
- **15.2.5.B.3** — `internal/cmd/valv-proxy/Dockerfile`: image runs the binary with env-injected config. 0 Go prod symbols, ~20 LOC, 1 file. blocked_by: B.2.
- **15.2.5.B.4** — `magefile.go` proxy-image build target. 1 build-target symbol, ~35 LOC, 1 file. blocked_by: B.3.
- **Cross-drop:** 15.2.5.D.1 build is now ALSO blocked_by 15.2.5.B.4 (image must exist locally before sidecar integration tests).

### 15.2.5.E — Orphan cleanup + concurrency-safe Provision (3 droplets, serialized)

NEW work only — network labels already exist (committed at `Provision` :196-203); E adds proxy-container labels + container-scoped sweep + cleanup ordering.

- **15.2.5.E.1** — Proxy container managed-label propagation (`Labels: {ManagedLabelKey: ManagedLabelValue}` on the sidecar `ContainerRunRequest` in D.1's Provision). 1 edit cluster, ~40 LOC including tests, 2 files (`networkpolicy/service.go` + test). blocked_by: 15.2.5.D.1.
- **15.2.5.E.2** — Container-label sweep + reclaim. Adds `docker.Executor.ListContainersByLabel(ctx, label string) ([]string, error)` using existing `BuildContainerListArgs`; extends `NetworkExecutor` interface; extends `Provision` + `CleanupStale` to enumerate + remove stale managed sidecars before creating new. 2 prod symbols, ~70-80 LOC, 3 prod files (`adapters/docker/executor.go` + `networkpolicy/service.go` + tests). blocked_by: 15.2.5.D.1, E.1.
- **15.2.5.E.3** — Cleanup ordering (Provision-returned `Cleanup` stops/removes proxy BEFORE rm network). 1 service edit cluster, ≤60 LOC, 1 prod file (+ test). blocked_by: 15.2.5.D.1, E.2. NOTE: if `docker.Executor` lacks an explicit stop-container method after E.2 lands, E.3 splits into E.3a (add stop) + E.3b (ordering); builder re-measures.

### 15.2.5.G — Shipped topology integration test (1 droplet, COLLAPSED)

Sub-planner concluded "1 droplet sufficient" (test-only, 0 prod symbols, file lock on `service_integration_test.go` makes split serialization pointless; existing pattern already keeps integration scenarios cohesive). Confirms R2 falsif NIT.

- **15.2.5.G** — Complete the skipped placeholder `TestEnsureProjectImage_NetworkPolicyOverlayBuildReachesProxy_DockerDesktopMacOS` at `internal/services/images/service_integration_test.go`. 0 prod symbols, 150-250 test LOC, 1 file. `//go:build integration`. Proves: workload internal-only; reaches sidecar on `valv-proxy`; proxy on internal+bridge; allowed-host egress succeeds; non-allowed-host blocked. Acceptance: `mage integration` GREEN. blocked_by: 15.2.5.D.2, 15.2.5.E.3.

### 15.4.D — `valv run --network open` (3 droplets, serialized)

All 3 droplets share `internal/cli/run.go` + `internal/cli/run_test.go` (serialized). **Critical decision: CLI must EXPLICITLY set `NetworkPolicyClosed` as default** (do NOT rely on 15.3.A's open zero-value — DROP_15 scope requires closed-default for `valv run`).

- **15.4.D.1** — Extend `parsedRunFlags` with `network string` + `networkExplicit bool` fields. Update `TestStripRunLocalFlags` existing rows to assert zero-value `network == ""`. 1 changed prod symbol (struct field-add cluster), ~15 prod LOC, 2 files. blocked_by: 15.3.A BUILT.
- **15.4.D.2** — Parse prefix-only `--network` in `stripRunLocalFlags` (recognizes `--network=open` and `--network open` only in leading prefix; preserves trailing/malformed forms). New test rows for both forms + pass-through case. 1 changed prod symbol, ~45 prod LOC, 2 files. blocked_by: D.1, 15.3.A BUILT.
- **15.4.D.3** — Wire CLI network policy into `runRunCommand`'s `LaunchRequest`: explicit `launch.NetworkPolicy = runservice.NetworkPolicyClosed` by default; `runservice.NetworkPolicyOpen` only when `--network open` parsed. Deterministic CLI error for invalid `--network <value>`. New tests: `TestRunCommandNetworkPolicyDefaultClosed` + `TestRunCommandNetworkPolicyOpenEquals` + `TestRunCommandNetworkPolicyOpenSpaceSeparated`. 1 changed prod symbol, ~60 prod LOC, 2 files. blocked_by: D.2, 15.4.C BUILT, 15.3.A BUILT.

## Full DROP_15 Build Tree (R2 fold, all sub-decomps absorbed)

23 atomic build droplets. **Level-by-level parallelism (siblings parallel within a level; serialized across levels):**

- **L0 (parallel — no intra-DROP blockers):** 15.2.5.A, 15.2.5.C, 15.2.5.B.1, 15.3.A, 15.4.A
  - 15.2.5.A + 15.2.5.C share `networkpolicy/service.go` → **C blocked_by A** (file lock).
  - 15.4.A blocked_by 15.0/15.1 (done) + 15.3 plan accepted (now). Touches different files than 15.3.A (cli/network.go new vs services/run/service.go).
- **L1:** 15.2.5.B.2 (after B.1), 15.4.B (after 15.4.A)
- **L2:** 15.2.5.B.3 (after B.2), 15.4.C (after 15.4.B), 15.3.B.1 (after ALL 15.2.5.* closed — gated until L8+)
- **L3:** 15.2.5.B.4 (after B.3)
- **L4:** 15.2.5.D.1 (after 15.2.5.A, B.4, C — needs the image built + executor seam + NO_PROXY inversion)
- **L5 (parallel):** 15.2.5.D.2, 15.2.5.E.1 (both after D.1)
- **L6:** 15.2.5.E.2 (after E.1)
- **L7:** 15.2.5.E.3 (after E.2)
- **L8:** 15.2.5.G (after D.2, E.3) — closes all 15.2.5.*
- **L9:** 15.3.B.1 (after ALL 15.2.5.* closed)
- **L10:** 15.3.B.2 (after B.1)
- **L11:** 15.3.C (after B.2)
- **L12:** 15.3.D (after C)
- **L13:** 15.4.D.1 (after 15.3.A BUILT — possible MUCH earlier in L9 since only needs 15.3.A built, not all 15.3.*)
- **L14:** 15.4.D.2 (after D.1)
- **L15:** 15.4.D.3 (after D.2, 15.4.C BUILT, 15.3.A BUILT)

NOTE: 15.4.D.1/D.2/D.3 only need 15.3.A built (not the entire 15.3 chain). 15.3.A unblocks earlier in the L0 wave per the R2 fold. Once 15.3.A closes, 15.4.D.1 can fire in parallel with the deeper 15.3.B.1+ chain.

## Cross-drop blockers (final)

- DROP_15 close blocked_by ALL 15.2.5.* / 15.3.* / 15.4.* closed + drop-end gate (`mage test` + `mage integration` + push + CI + `mage build` + Hylla reingest from remote).
