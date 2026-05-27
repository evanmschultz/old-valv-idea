# DROP_15 Plan-QA — PROOF pass

**Verdict: pass**

Re-plan QA of the sidecar-proxy topology. Frozen units 15.0/15.1 (done+green) not
re-audited beyond confirming acceptance text is unchanged. Focus: 15.2 (re-add
NetworkConnect), 15.2.5 (sidecar lifecycle + NO_PROXY inversion + detached-run seam
+ orphan cleanup), 15.3 (workload internal-only).

## Per-claim audit

### P1 — Safety invariant sound (workload single internal-only; only proxy multi-homed) — CONFIRMED
- Scope §, Decision 5, Note A1, and Unit 15.3 acceptance all state the workload
  attaches to the `--internal` network ONLY (`ContainerRunRequest.Network = <internal>`,
  single string field — `types.go:58` is a single `Network string`, structurally
  cannot carry two networks in one request, so the workload cannot be multi-homed
  via the request type at all). The proxy is dual-attached via a SEPARATE
  `docker network connect` call (Unit 15.2 `ConnectNetwork`), not via the workload
  request. The forbidden Round-4 pattern was WORKLOAD-on-bridge; this plan never
  attaches the workload to bridge.
- Unit 15.3 `service_test.go` acceptance explicitly asserts "the workload
  `ContainerRunRequest` never attaches to `bridge` or any second network". This is a
  testable guard on the exact regression. PASS.
- Docker evidence (Context7 `/docker/docs`): `--internal` = "No external
  connectivity"; multi-homed default-gateway ambiguity (`gw-priority`) only arises
  for containers on >1 network — keeping the workload single-network sidesteps that
  footgun entirely. Confirms the invariant is not just asserted but architecturally
  enforced.

### P2 — NO_PROXY inversion flagged as required correction — CONFIRMED
- Decision 5 "NO_PROXY correction (required)" and Unit 15.2.5 "NO_PROXY inversion
  correction (required)" both explicitly flag the committed
  `PolicyMaterial.NoProxy = buildNoProxy(allowlist)` as WRONG for an internal-only
  workload and require `Provision`/`PolicyMaterial`/`buildNoProxy` revision so
  NO_PROXY carries only loopback + sidecar alias, with the allowlist enforced inside
  the proxy filter.
- Evidence resolves: `internal/services/networkpolicy/service.go:212` literally sets
  `NoProxy: buildNoProxy(request.Allowlist)` and `:256-272` `buildNoProxy` joins the
  allowlist verbatim. The package docstring `:11-14` and `ProvisionRequest.ProxyEndpoint`
  example `:105` (`host.docker.internal:18080`) confirm this is the now-dead
  host-local-proxy design. The plan correctly identifies the inversion and routes the
  fix to 15.2.5 with a test-proof requirement. PASS.

### P3 — Evidence cites resolve — CONFIRMED (spot-checked)
- `internal/adapters/docker/network.go` — has `NetworkCreateRequest`/
  `BuildNetworkCreateArgs`/`NetworkRemoveRequest`/`BuildNetworkRemoveArgs` (lines
  16-115). NO `NetworkConnectRequest`/`BuildNetworkConnectArgs`. Confirms 15.2 re-add
  is genuinely additive. The file's own doc comment (`:18-20`) even states "there is
  no `NetworkConnectRequest` symbol in this drop" — stale post-redesign but confirms
  the cut was real.
- `internal/adapters/docker/executor.go:47-102` — `CreateNetwork`/`RemoveNetwork`/
  `ListNetworks` present. LSP documentSymbol confirms the FULL Executor method set:
  Build, RemoveImage, RemoveContainer, PruneBuilder, CreateNetwork, RemoveNetwork,
  ListNetworks. NO container-run method, NO ConnectNetwork — confirms both the 15.2
  ConnectNetwork add AND the 15.2.5 detached-run seam are genuinely new (see P4).
- `internal/adapters/docker/types.go:43-60` — `ContainerRunRequest` has `Detached`(52),
  `Labels`(49), `Network`(58), `Extra`(59). Confirmed.
- `internal/adapters/docker/ops.go:9-21` — `ImageBuildRequest` has `BuildArgs map[string]string`
  (14) and `Network string` (17); `BuildImageArgs:70-72` emits `--network <name>`;
  `:78-87` emits sorted `--build-arg`. Confirmed.
- `internal/services/networkpolicy/service.go` — current `PolicyMaterial` shape
  (`:131-146`) and `NoProxy=allowlist` (`:212`) confirmed as in P2.
- `internal/services/images/overlay.go:84-147` — `BuildOverlayDockerfile` emits
  exec-form `RUN ["go","install",...]` / `RUN ["npm","install","-g",...]` (`:126-141`)
  that execute at `docker buildx build` time, outside runtime launch. Confirms the A2
  build-egress claim. PASS.

### P4 — Detached-run seam identified as new + routed — CONFIRMED
- LSP documentSymbol on executor.go proves there is NO container-run method of any
  kind on `Executor`. Unit 15.2.5 acceptance "Detached-container-run seam (new, not
  yet in tree)" requires extending the consumer-side `NetworkExecutor` interface AND
  `docker.Executor` with `RunContainerDetached(ctx, ContainerRunRequest) (string, error)`
  backed by `BuildRunArgs`, plus `ConnectNetwork` and a container-remove path
  (`RemoveContainer` already exists at executor.go:25 — minor: plan says "a
  container-remove path" without noting `RemoveContainer` already exists; see NIT-1).
  The seam is correctly marked new and routed to 15.2.5. PASS.

### P5 — blocked_by chain acyclic — CONFIRMED
- 15.0←nothing; 15.1←nothing; 15.2←nothing; 15.2.5←{15.0,15.1,15.2};
  15.3←{15.1,15.2,15.2.5,D13,D14}; 15.4←{15.0,15.1,15.3,D13}.
- Topological order exists: 15.0,15.1,15.2 → 15.2.5 → 15.3 → 15.4. No back edges,
  no cycle. Cross-drop deps (D13,D14) are external and both in-flight per main/PLAN.md;
  ordering is sound. PASS.
- Note: 15.3 does not list 15.0 directly, but 15.2.5 (which 15.3 depends on) depends
  on 15.0, so 15.0 is transitively covered. Not a gap.

### P6 — 15.0/15.1 acceptance unchanged (frozen) — CONFIRMED
- 15.0 state=done, 15.1 state=done. Acceptance bodies are the project-root resolution
  + typed-allowlist/WriteAllowlistSection contracts respectively; nothing in the
  sidecar redesign touches those acceptance bullets. The redesign only consumes their
  outputs (effective allowlist, project-root manifest). PASS.

## Atomicity / well-formedness
- Each unit has Objective-equivalent + testable Acceptance + Evidence + blocked_by.
  Units are package-scoped and within atomic budget EXCEPT 15.2.5 (see Falsification
  F-A; flagged there as accepted-risk-with-recommendation, not a proof failure).
- Open questions (buildx-RUN sidecar reachability, proxy image choice) are routed via
  Decision 6 empirical-validation requirement + Notes, not buried. PASS for routing.

## Tools Used
- Read: WORKFLOW.md; DROP_15 PLAN.md; DROP_13 PLAN.md:36-97; DROP_14 PLAN.md:60-79;
  network.go; networkpolicy/service.go; executor.go; types.go:40-99; ops.go:1-95;
  overlay.go:84-147.
- LSP documentSymbol: internal/adapters/docker/executor.go (confirmed no run/connect method).
- mcp__hylla__hylla_search_keyword: run/network/proxy symbols (snapshot 8 baseline; noted run pkg currency).
- mcp__plugin_context7_context7__query-docs `/docker/docs`: internal network, multi-network connect+alias, buildx --network.
