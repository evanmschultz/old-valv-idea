# DROP_15 Plan-QA — FALSIFICATION pass

**Verdict: pass-with-findings** (no unmitigated FAIL-trigger counterexample; several
under-specifications routed as accepted-risk + recommendations for the builder/dev.)

Sidecar-proxy redesign attacked along the 7 named vectors plus added vectors. Each is
mitigated / accepted-risk / FAILURE.

## Named attack vectors

### V1 — buildx build-RUN reachability to sidecar on `--internal` — ACCEPTED-RISK (mitigated by hedge)
- Counterexample attempt: buildx ephemeral build containers attach via
  `--network=<name>` differently than `docker run`; on Docker Desktop macOS the
  build container's bridge-vs-internal routing may not see the sidecar alias.
- Context7 `/docker/docs` confirms buildx `--network=<named>` IS a supported mode
  (named network for RUN steps) — so the path is real, not invented. But it does NOT
  prove a sidecar alias on an `--internal` net is DNS-resolvable from the
  buildkit build container on Docker Desktop macOS specifically.
- Mitigation: Decision 6 + AC6 + Unit 15.2.5 flag this as "empirical-validation
  required" with a concrete integration test (`NoCache=true`, one allowlisted path
  succeeds + one blocked fails, MUST use shipped sidecar topology). Fallback
  ("build-specific equivalent that still routes through the sidecar filter") is
  NOT fully concrete — it names a constraint (no workload/build bridge attach) but
  not a mechanism. This is acceptable for a plan (the empirical result determines the
  mechanism) but the builder should be told the fallback is undefined until the
  empirical result lands. NOT a FAIL: the plan correctly refuses to commit to an
  unvalidated mechanism. RECOMMENDATION: if the empirical test fails, that becomes a
  blocking re-plan, not a builder-discretion patch.

### V2 — sidecar startup ordering / bridge-connect race — FINDING (under-specified) — accepted-risk
- Counterexample: workload (or build) starts before the sidecar's `docker network
  connect bridge` completes → first egress attempts fail or hang.
- Plan says (Decision 6, 15.2.5 step 3) "The sidecar MUST be running before the build
  starts" but specifies NO readiness check — only ordering of calls. `docker run -d`
  returning a container ID does NOT mean the in-container proxy process is listening,
  nor that the bridge interface has an IP.
- Mitigation present? Partial: Provision is synchronous and ordered (create net →
  launch sidecar → connect bridge → return material), so the call sequence is correct.
  But "running" != "ready to proxy". RECOMMENDATION: 15.2.5 acceptance should add a
  proxy readiness probe (e.g. poll the proxy port from the host bridge side, or a
  short retry loop) before returning PolicyMaterial. Routed as a dev/builder decision.
  accepted-risk for plan approval; flag for the builder.

### V3 — sidecar alias DNS resolution timing — accepted-risk (subsumed by V2)
- Docker embedded DNS registers the alias at `network connect` time; there can be a
  brief window before the alias resolves on the internal net. Same class as V2 —
  covered by the same readiness-probe recommendation. No separate FAIL.

### V4 — proxy filter implementation gap (image/binary unspecified; HTTPS CONNECT SNI-only) — FINDING — accepted-risk
- The plan says "HTTP/HTTPS allowlist filter" and "running an HTTP/HTTPS allowlist
  filter configured from the effective allowlist" but does NOT name the proxy
  image/binary. Two real consequences:
  1. Builder-decision vs plan-named: the plan leaves the proxy implementation to the
     15.2.5 builder. Given Valv is Go + "smallest concrete design", a minimal Go
     CONNECT proxy in a slim image is the natural choice, but the plan does not say
     so. RECOMMENDATION: name it (Go-based filtering CONNECT proxy in a minimal
     image) so the builder does not pull a third-party proxy image (tinyproxy/squid)
     that adds a non-Go dependency surface inconsistent with CLAUDE.md "no Go Docker
     SDK / pure-Go" ethos. Routed as dev decision.
  2. HTTPS CONNECT can only filter by SNI/host, NOT full URL/path — exactly what the
     allowlist needs (exact-host). The plan's Decision 1 allowlist semantics are
     EXACT-HOST (no path/scheme/port), which ALIGNS with CONNECT/SNI host-only
     filtering — so this is actually consistent. But the plan never explicitly states
     "HTTPS is filtered by CONNECT host / SNI, not URL". A builder could wrongly try
     MITM/URL inspection. RECOMMENDATION: state the host-only HTTPS filtering
     mechanism explicitly. NOT a FAIL (semantics happen to align) but a real
     clarity gap. accepted-risk.

### V5 — orphan cleanup race under concurrent `valv run` — FINDING — accepted-risk
- Counterexample: two concurrent `valv run` both call Provision; both `ListNetworks`
  the managed label, both see the same desiredName, one creates it, the other's
  `CreateNetwork` fails on name collision (or both try to remove the other's
  "stale" proxy container). The committed `Provision` (service.go:169-222) is NOT
  concurrency-safe: the list→reconcile→create sequence has a TOCTOU window. The
  redesign ADDS a proxy container to that same unsynchronized sequence, widening it.
- Plan coverage: 15.2.5 orphan-cleanup acceptance covers SEQUENTIAL stale reclaim
  (prior killed process) but says nothing about CONCURRENT provision. The deterministic
  network name is per-allowlist (sha256), so two runs with the same allowlist target
  the same network/sidecar — reuse is intended, but the reclaim-or-remove branch could
  race (run A removes a "stale" container that run B just launched).
- Mitigation: none in plan. RECOMMENDATION: 15.2.5 should either (a) make reclaim
  idempotent-on-collision (treat "already exists" create errors as reuse) or (b)
  document per-invocation isolation removes the shared-name race (see V7). accepted-risk
  for plan approval; this is a real concurrency gap to route to the builder/dev. Does
  not block because DROP_15 ships `valv run` single-invocation; concurrent runs are an
  edge the dev can accept as out-of-scope for the drop.

### V6 — cleanup completeness (SIGKILL after sidecar, before workload) + network-rm blocked by live proxy — FINDING — mitigated-partially
- Counterexample: process SIGKILLed after sidecar launch but before workload start →
  next run must remove BOTH the proxy container AND the network; a live proxy holding
  the network blocks `docker network rm`.
- Plan coverage: 15.2.5 orphan-cleanup acceptance explicitly extends cleanup to "stop+rm
  the proxy container, rm the network" and requires a test that a stale labeled network
  AND stale labeled proxy container are both reclaimed/cleaned. Ordering (stop+rm
  container THEN rm network) is correctly implied by the listed sequence — addresses the
  "live proxy blocks network rm" footgun. GOOD. Residual: the plan should make the
  container-before-network teardown ORDER explicit in acceptance (currently order is
  inferable but not asserted). Minor. Largely mitigated.

### V7 — per-invocation vs shared sidecar+network — FINDING (UNDER-SPECIFIED) — accepted-risk, recommend resolve before build
- The plan is INTERNALLY AMBIGUOUS here:
  - Decision 6 + 15.2.5: "the networkpolicy service provisions it ONCE and both runtime
    and build callers REUSE the same sidecar + network" → implies SHARED.
  - networkName is sha256(allowlist) → same allowlist = same network = shared across
    invocations/projects with identical allowlists; DIFFERENT allowlists = different
    networks. So isolation is per-allowlist, NOT per-project and NOT per-invocation.
- Consequence: two different projects with identical allowlists share one sidecar+network.
  That is fine for egress filtering (same policy) but means cross-project workloads share
  an internal L2 segment — a workload could reach a sibling project's workload on the same
  internal net by IP. For a closed-network SECURITY feature this is a real isolation
  question. Conversely per-invocation would multiply teardown cost.
- Plan coverage: the plan picks SHARED-by-allowlist implicitly via the deterministic name
  but never discusses the cross-project same-net exposure. RECOMMENDATION (route to dev):
  decide explicitly — (a) accept shared-by-allowlist (document the same-policy-same-net
  exposure as acceptable since policy is identical), or (b) make the network name
  per-project (incorporate project root into the hash) to isolate L2 even under identical
  allowlists. This is the single most load-bearing unrouted decision. accepted-risk for
  plan structure (no acyclic/atomicity violation) but SHOULD be resolved before 15.2.5
  build. Closest thing to a smart-default footgun in this re-plan.

## Added vectors

### V8 — shipped-but-not-wired — MITIGATED
- 15.2 ConnectNetwork has a concrete consumer (15.2.5 sidecar dual-attach) — not built
  in isolation. 15.2.5 sidecar is consumed by 15.3 (runtime proxy env) and images
  service (build egress). 15.3 is the `valv run` ship gate. 15.4 wires CLI + `--network
  open`. Every built thing has an end-to-end consumer + integration test. No orphan.

### V9 — hallucinated symbols — MITIGATED
- All cited committed symbols verified to exist (PROOF P3). All new symbols
  (`NetworkConnectRequest`, `BuildNetworkConnectArgs`, `ConnectNetwork`,
  `RunContainerDetached`, `WriteAllowlistSection`, `AllowlistConfig`,
  `internal/services/run`, `internal/cli/run.go`, `internal/cli/network.go`) are marked
  "new, not yet in tree". `RemoveContainer` already exists (executor.go:25) — the plan's
  "container-remove path (new)" phrasing slightly overclaims; see PROOF NIT-1. No
  hallucination.

### V10 — methodology drift (CLAUDE.md hard rules) — MITIGATED
- No Go Docker SDK introduced (shell-out preserved). No CGO. macOS-only (Docker Desktop
  macOS is the validation target). Consumer-side interface (`NetworkExecutor`) for test
  injection matches "interfaces near the consumer". `testcontainers-go` integration tests
  required. Real-Docker-over-mocks honored. No drift. NOTE: proxy-image choice (V4) must
  not smuggle in a non-Go/3rd-party dependency without a CLAUDE.md rule — flag for builder.

### V11 — under-decomposition: 15.2.5 over the 2-block atomic budget — FINDING — accepted-risk
- 15.2.5 spans TWO packages (`networkpolicy` + `images`) and does ~6 distinct things:
  redesign Provision to launch+dual-attach+teardown a sidecar; invert NO_PROXY; add a
  detached-run seam to the docker Executor + interface; reuse for image builds; orphan
  cleanup of containers; integration test. By the strict cascade "1-2 small blocks ≤80
  LOC" droplet budget this is OVERSIZE and would normally trigger a "convert to
  sub-planner" directive.
- WHY NOT A FAIL HERE: Valv's CLAUDE.md/WORKFLOW.md define "atomic" as "one builder can
  finish a single unit cleanly, acceptance is yes/no-verifiable, paths/packages clear" —
  a looser per-project budget than the generic cascade 2-block rule, and explicitly says
  "add more units inside PLAN.md rather than stretching one unit". 15.2.5 IS stretched.
  RECOMMENDATION: split 15.2.5 into 15.2.5a (docker Executor detached-run seam +
  ConnectNetwork wiring — pure adapter) and 15.2.5b (networkpolicy sidecar lifecycle +
  NO_PROXY inversion + images reuse + integration). The detached-run seam is an adapter
  concern cleanly separable from the service redesign, and the split would let the
  adapter unit land + be QA'd before the heavier service rework. accepted-risk: the plan
  is shippable as-is but decomposition discipline favors the split. Route to dev.

### V12 — `ProvisionRequest.ProxyEndpoint` / `Valid()` redesign coherence — MITIGATED
- Current `ProvisionRequest` requires a `ProxyEndpoint` (host-local-proxy design) and
  `Valid()` rejects empty allowlist with "use open mode". Under the sidecar redesign the
  ENDPOINT is no longer caller-supplied (the sidecar alias is service-internal). The plan's
  15.2.5 "Return policy material: the sidecar internal-network alias + port..." implies
  ProxyEndpoint becomes service-derived, but the plan does NOT explicitly say to remove/
  repurpose the `ProxyEndpoint` input field. Minor: the redesign acceptance is broad
  enough to cover it ("Revise Provision/PolicyMaterial/buildNoProxy accordingly"), but the
  ProvisionRequest input contract change is left implicit. RECOMMENDATION: state that
  `ProvisionRequest.ProxyEndpoint` is removed/replaced by a service-chosen sidecar port.
  accepted-risk.

## Convergence
- (a) No unmitigated counterexample produces a hard FAIL: safety invariant holds
  structurally (single Network field), NO_PROXY inversion is flagged, cites resolve,
  chain acyclic.
- (b) Proof confirmed evidence completeness for the 6 proof properties.
- (c) Routed Unknowns: V1 fallback undefined-until-empirical; V2/V3 sidecar readiness
  probe; V4 proxy image + HTTPS-host-only filtering statement; V5 concurrent-provision
  TOCTOU; V7 shared-by-allowlist L2 exposure (load-bearing dev decision); V11 split
  15.2.5; V12 ProvisionRequest field cleanup. None block plan approval; all are
  builder/dev directives for the build phase.

## Tools Used
- Read: WORKFLOW.md; DROP_15/13/14 PLAN.md; network.go; networkpolicy/service.go;
  executor.go; types.go; ops.go; overlay.go.
- LSP documentSymbol: executor.go (no run/connect method present).
- mcp__hylla__hylla_search_keyword: run/network/proxy symbol grounding (snapshot 8).
- mcp__plugin_context7_context7__query-docs `/docker/docs`: internal-network isolation,
  multi-network connect + --alias, buildx --network named-network RUN, gw-priority.
