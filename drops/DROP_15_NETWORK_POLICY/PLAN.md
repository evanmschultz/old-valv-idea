# DROP_15 — NETWORK_POLICY

**State:** planning
**Blocked by:** DROP_14 (todo)
**Paths (expected):** `internal/domain/` (network-policy domain type — allowlist shape), `internal/tools/` (extend `.valv/tools.toml` `[allowlist]` parsing — currently `toml.Primitive` placeholder), `internal/adapters/providers/` (Docker `--network` / `--add-host` wiring), `internal/services/networkpolicy/` (new — resolution + closed-default enforcement), `internal/cli/` (allowlist edit subcommands or `valv network` namespace)
**Packages (expected):** `internal/domain/`, `internal/tools/`, `internal/adapters/providers/claude/`, `internal/adapters/providers/codex/`, possibly new `internal/services/networkpolicy/`, `internal/cli/`
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

Ship DROP_15 as a closed-by-default, macOS-compatible network policy layer by moving policy enforcement onto the post-DROP_13 generic-run seam. Use an ephemeral `--internal` Docker network for the workload and a host-local HTTP/HTTPS allowlist proxy reached via `host.docker.internal`. Do not use literal `--network none` (Docker docs: loopback only) and do not use iptables/ipset (Docker Desktop on macOS routes container traffic through its VM/backend process, not a native Linux host stack).

### Units

#### Unit 15.1 — Typed allowlist schema and section-safe `.valv/tools.toml` editing

**State:** todo

**Paths:**
- `internal/tools/tools.go`
- `internal/tools/allowlist_test.go` (new)
- `internal/services/images/overlay_test.go`

**Packages:** `internal/tools`, `internal/services/images`

**Evidence:** `internal/tools/tools.go:70-130` still stores `Allowlist toml.Primitive` and manually `PrimitiveDecode`s it; `internal/services/images/overlay.go:35-70` hashes only `manifest.Tools`, so network policy must not perturb image tags.

**Acceptance:**
- `ToolManifest.Allowlist` is promoted to a typed `AllowlistConfig` (new, not yet in tree) with `Hosts []string`.
- `Load(path)` keeps strict unknown-key behavior by decoding `[allowlist]` into the typed struct while leaving `[env]` deferred for DROP_14. Unknown top-level sections still fail.
- Allowlist host syntax is intentionally narrow for v1: exact hostnames / Docker aliases only, normalized to lowercase, deduped, with no CIDR support, no URL prefixes, and no scheme/path/port parsing.
- New helper `WriteAllowlistSection(path string, cfg AllowlistConfig) error` (new, not yet in tree) rewrites only the `[allowlist]` section or creates `.valv/tools.toml` when absent. It must preserve existing `[tools]` and `[env]` text instead of full-file TOML re-encode.
- `OverlayHash` stays tool-only: same `Tools` with different allowlists produce the same hash in `internal/services/images/overlay_test.go`.

**Blocked by:** nothing

---

#### Unit 15.2 — Docker network lifecycle helpers

**State:** todo

**Paths:**
- `internal/adapters/docker/network.go` (new)
- `internal/adapters/docker/network_test.go` (new)

**Packages:** `internal/adapters/docker`

**Evidence:** `internal/adapters/docker/types.go:43-59` already carries `ContainerRunRequest.Network`; `internal/adapters/docker/command.go:16-82` exposes container lifecycle only. Docker docs for `docker network create --internal`, `docker network connect --alias`, and `docker network rm` prove the missing plumbing surface.

**Acceptance:**
- Add typed requests and builders for `docker network create`, `docker network connect`, and `docker network rm`:
  - `NetworkCreateRequest` (new, not yet in tree)
  - `NetworkConnectRequest` (new, not yet in tree)
  - `NetworkRemoveRequest` (new, not yet in tree)
- Add `Executor.CreateNetwork`, `Executor.ConnectNetwork`, and `Executor.RemoveNetwork` (new, not yet in tree) in `internal/adapters/docker/network.go`.
- `BuildNetworkCreateArgs` emits `docker network create --internal ...` with deterministic label ordering.
- `BuildNetworkConnectArgs` emits `docker network connect --alias <name> ...` for the workload-visible proxy alias.
- Tests cover validation failures, alias ordering, `--internal`, and cleanup arg shapes.

**Blocked by:** nothing

---

#### Unit 15.3 — Generic-run closed-default runtime policy

**State:** todo

**Paths:**
- `internal/services/run/service.go` (new, not yet in tree; expected from DROP_13)
- `internal/services/run/service_test.go` (new, not yet in tree)
- `internal/services/run/service_integration_test.go` (new, not yet in tree; `//go:build integration`)

**Packages:** `internal/services/run` (new, not yet in tree)

**Evidence:** `drops/DROP_13_GENERIC_RUN/PLAN.md:5-18` makes generic run the shared runtime seam; `internal/services/claude/service.go:180-329` and `internal/services/codex/service.go:171-332` show today's duplicated request-building path. Docker docs show `--internal` blocks external routes while still allowing host/gateway communication, and Docker Desktop resolves `host.docker.internal` to the host's internal IP.

**Acceptance:**
- Closed mode is the default. The run service creates an ephemeral internal bridge network, starts a host-local HTTP/HTTPS allowlist proxy on a random loopback port, injects `HTTP_PROXY`, `HTTPS_PROXY`, and `NO_PROXY`, and runs the workload container with `ContainerRunRequest.Network = <internal-network>`.
- Open mode preserves current behavior: no proxy env injection, no internal network, and default Docker egress.
- The host proxy is exact-host allowlist only; non-HTTP/HTTPS traffic remains blocked by lack of a default route. No SOCKS, no raw TCP allowlisting, no CIDR matching in this drop.
- Error wrapping boundaries are explicit for: network create, proxy listen, request build, container run, and cleanup teardown.
- `service_test.go` is table-driven and proves:
  - default closed mode injects proxy env and internal network
  - open mode injects neither
  - cleanup runs on both success and failure paths
  - DROP_14 env vars are merged, not overwritten
- `service_integration_test.go` uses `testcontainers-go` and no real-internet dependency:
  - host `httptest.Server` is allowlisted and reachable from the closed-mode container via `host.docker.internal`
  - a non-allowlisted hostname is denied by the proxy before external egress
- This unit owns the ship gate for "closed by default"; if it is incomplete, DROP_15 is not releasable.

**Blocked by:** Unit 15.1, Unit 15.2, DROP_13, DROP_14

---

#### Unit 15.4 — CLI network management and open-egress opt-out

**State:** todo

**Paths:**
- `internal/cli/network.go` (new)
- `internal/cli/network_test.go` (new)
- `internal/cli/run.go` (new, not yet in tree; expected from DROP_13)

**Packages:** `internal/cli`

**Evidence:** `internal/cli/tools.go:21-93` is the existing per-project manifest command pattern; `internal/cli/manage.go:22-63` is the existing multi-subcommand branch pattern; `drops/DROP_13_GENERIC_RUN/PLAN.md:14` makes `valv run --account <name> <command>` the post-pivot entrypoint.

**Acceptance:**
- Add `valv network allow <host>`, `valv network deny <host>`, and `valv network list`, all project-scoped via `project.Detect()` so they work from subdirectories.
- `allow` creates `.valv/tools.toml` when absent and updates only the `[allowlist]` section via the new tools helper.
- `deny` is idempotent and removes the host from the allowlist; removing the last host may delete the `[allowlist]` section entirely.
- `list` reports the effective stored host list for the current project.
- Add `--network open` to `valv run` (new, not yet in tree). Default remains closed when the flag is omitted.
- Do not add duplicated flag parsing to today's pre-DROP_13 `claude.go` / `codex.go`; this unit assumes those commands are thin adapters over `run.go` by the time DROP_15 builds.
- `network_test.go` is table-driven and proves:
  - subdirectory project detection
  - create-from-absent-file behavior
  - dedupe and lowercase normalization
  - deny-no-op on missing hosts
  - preservation of existing `[tools]` / `[env]` text
  - `valv run --network open` toggles the open-mode request path

**Blocked by:** Unit 15.1, Unit 15.3, DROP_13

### Notes

- Preferred design choice: exact-host allowlist, not CIDR and not URL-prefix matching. That aligns with proxy-mediated HTTP/HTTPS enforcement and keeps the v1 parser small.
- Rejected design choice: literal `docker run --network none` for the workload. Official Docker docs show the `none` driver leaves only loopback, so the workload could not reach a proxy at all.
- Rejected design choice: Linux-specific iptables/ipset control. Official Docker Desktop docs show macOS networking is mediated by Docker Desktop's VM/backend process, so that path is the wrong portability target for Valv's macOS-only scope.
- Residual risk to carry into QA: real Claude/Codex proxy-env compliance is not proven by repo evidence. The integration suite can prove the runtime policy with `curl`; operator escape hatch is `--network open`.
