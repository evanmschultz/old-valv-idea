# DROP_15 Round-1 Plan-QA Falsification — 15.2.5 + 15.3 + 15.4

Round 1 falsification verdicts on the R1 sub-decompositions (committed at `9d22c25`). Orch transcribed (QA personas are READ-ONLY). Replaces the prior sidecar-re-plan artifact at this path. Codex falsifications skipped per documented 10-min SIGTERM failure mode at scale; all 3 ran via Agent-tool sonnet fallback (the established `CODEX_EXHAUSTED` substitute).

## §15.2.5 — PASS-WITH-FINDINGS

Agent: `ta-go-plan-qa-falsification` (Agent tool, model=sonnet). Tool-call audit ✓ (11 tool uses, Read × 7 + Bash ls, no Edit/Write/git-mutate/mage).

### CF-1 — 15.2.5.F production scope already committed

`internal/services/images/service.go:850-908` (committed at HEAD `6ddf5d7`, verified by orch direct Read) contains the complete proxy build-args + Network-field injection F claims to add as new work:
- :861-866 — `tools.EffectiveAllowlist` + `s.networkPolicy.Provision` call
- :881-892 — `buildArgs["HTTP_PROXY"]/["HTTPS_PROXY"]/["NO_PROXY"]` set from `policyMaterial` + `buildNetwork = policyMaterial.NetworkName`
- :894-908 — `docker.ImageBuildRequest{BuildArgs, Network, ...}`

**Orch ruling (2026-05-26 dev call): DELETE 15.2.5.F entirely.** The future "swap external `proxyEndpoint` for sidecar-supplied endpoint" work absorbs into 15.2.5.D.1's `Service.Provision` rewrite (which now sets `PolicyMaterial.HTTPProxyURL` from the sidecar alias, not from `request.ProxyEndpoint`). Any test-coverage gap for the new PolicyMaterial shape lives in D.1's existing test scope.

### CF-2 — `ConnectNetwork` interface extension orphaned

`networkpolicy.NetworkExecutor` (`networkpolicy/service.go:58-65`) lists only `CreateNetwork`/`RemoveNetwork`/`ListNetworks`. 15.2.5.A spec adds `RunContainerDetached` to this interface, but 15.2.5.D.1's `Provision` rewrite ALSO calls `ConnectNetwork` (which exists on `docker.Executor` at executor.go:72 but is NOT in the consumer-side `NetworkExecutor` interface). No droplet explicitly owns adding `ConnectNetwork` to `NetworkExecutor`.

**Orch fix:** expand 15.2.5.A's scope to add BOTH `RunContainerDetached` AND `ConnectNetwork` to the `NetworkExecutor` interface.

### NITs
- N1 — 15.2.5.E scope overlaps committed code (network labels already exist via `Provision` at :197-203). E's NEW scope is sidecar-container labels + stop/remove ordering + container-scoped sweep. Tighten E's spec.
- N2 — Proxy port coordination between B and D.1 unresolved (B defines port; D.1 builds proxy URL). Route as open question for B's sub-planner.
- N3 — 15.2.5.G as sub-planner is arguably over-decomposed (0 prod symbols — test-only). Kept as sub-planner pending its own decomposition pass.
- N4 — `fakeNetworkExecutor` test-side updates are implicit in A's scope — test-side so excluded from prod budget.

## §15.3 — **FAIL**

Agent: `ta-go-plan-qa-falsification` (Agent tool, model=sonnet). Tool-call audit ✓ (10 tool uses, Read × 3 + Bash ls/rg × 7).

### CF-1 — 15.3.B claims 2 production symbols but touches 4-5

Concrete symbol re-count for B's spec:
1. `Options` — add `Policy` field to existing struct
2. `Service` — add unexported mirror field
3. `New` — add validation + assignment in constructor (`service.go:145-175`)
4. `Service.Run` — add closed-mode branch + defer cleanup (`service.go:181-219`)
5. New consumer-side `Policy` interface type in `services/run` (not yet in tree; `networkpolicy` exports `Service` as concrete value, not `Provisioner`)

That's 4-5 changed/new top-level production symbols, exceeding the ≥3-symbol FAIL threshold per `aa130dd`. The "Service/Options deps cluster" labeling is the documented "one coherent concern" rationalization the rule prohibits.

**Orch fix:** split B into:
- **15.3.B.1** (DI wiring): `Options` field add + `Service` mirror + `New` validation + the new consumer-side `Policy` interface type. 2-3 symbols, ~30 prod LOC, 1 prod file. Under budget.
- **15.3.B.2** (Run flow): `Service.Run` closed-mode branch + defer cleanup. 1 symbol, ~30 prod LOC, 1 prod file. blocked_by B.1.

### CF-2 — 15.3.A over-serialized

A's `blocked_by: ALL 15.2.5.* closed` is wrong. A adds only `NetworkPolicyMode` type + 2 constants + a selector field on `LaunchRequest`. A has ZERO import dependency on `internal/services/networkpolicy`.

**Orch fix:** drop A's `blocked_by` to `15.2.5 plan accepted` (per Rule 4 — blocked_by gates BUILDS on real dependencies; A has none).

### NITs
- N1 — LOC ambiguity on C ("~50 prod LOC" — net-added vs total `buildRequest` body). Clarify as net-added.
- N2 — `mage integration` target does NOT cover `./internal/services/run`. D's acceptance must either add it to the magefile integration target OR specify `mage testPkg ./internal/services/run` with integration build tag.

## §15.4 — PASS-WITH-FINDINGS

Agent: `ta-go-plan-qa-falsification` (Agent tool, model=sonnet). Tool-call audit ✓ (8 tool uses).

### Finding — 15.4.A acceptance gap on root-cmd registration

A's spec mentions "root-command registration touch included as cohesive cluster" but A's acceptance criteria test `runNetworkList` handler logic ONLY — not the cobra tree wiring. Builder could create `newNetworkCommand` and forget to add it to `root.go:141`'s `AddCommand` list; all 3 acceptance bullets still pass.

**Orch fix:** tighten A's acceptance to require one cobra-tree-execute test: `cmd.SetArgs([]string{"network", "list"}); cmd.Execute()` succeeds and stdout contains expected built-in hosts.

### NITs
- N1 — A's "2 prod files" is wrong (actual: 3 — `network.go` new, `network_test.go` new, `root.go` edit). Fix to "3 prod files". Under `>3` threshold so not a budget fail.
- N2 — `networkCmd` `GroupID` unspecified. Explicitly state `networkCmd.GroupID = "runtime"` per `root.go:131` pattern.
- N3 — 15.4.D sub-planner rationale mixes "API not verified yet" (weak per Rule 4) with "≥3 prod symbols" (correct trigger). Clarify trigger is budget.
- N4 — B's "preserve `[tools]` + `[env]` block bytes" acceptance bullet is redundant (15.1's `WriteAllowlistSection` proves it).

## Cross-cutting: G's sub-planner status

`15.2.5.G` kept as sub-planner pending its own decomposition pass; the sub-planner itself may legitimately return "1 droplet sufficient".
