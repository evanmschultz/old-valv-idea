# DROP_15 Round-1 Plan-QA Proof — 15.2.5 + 15.3 + 15.4

Round 1 proof verdicts on the R1 sub-decompositions of 15.2.5 / 15.3 / 15.4 (committed at `9d22c25` — `docs(drop-15): planner r1 decompose 15.2.5 + 15.3 + 15.4`). Orch transcribed (QA personas are READ-ONLY). Replaces the prior sidecar-re-plan QA artifact at this path (the stale file was never Phase-3 cleared from a prior round per the 2026-05-26 handoff note).

## §15.2.5 — PASS-WITH-NITS

Agent: `ta-go-plan-qa-proof` (built-in opus). Tool-call audit ✓ (14 tool uses, Read + read-only Bash, no Edit/Write/git-mutate/mage).

Measurement re-count (orch confirmed): A=2 symbols/~45 LOC/2 files; C=1/~20/1; D.1=1/~75/1 (near 80 ceiling); D.2=1/~55/1; F=1/~65/1. Sub-planners B/E/G correctly emitted as over-budget. Symbol grounding ✓ via Read on `internal/services/networkpolicy/service.go`, `internal/adapters/docker/{executor.go,types.go,network.go}`, `internal/services/images/service.go`. Blocker graph correct.

NITs:
- N1 — 15.2.5.F scope partly overlaps already-shipped code at `images/service.go:850-908` (build-args + Network injection committed). Tighten or remove (see falsif CF-1 + orch ruling).
- N2 — D.1 at ~75 prod LOC near 80 ceiling. Builder re-measures actual diff; if over, split off the orphan-sweep loop into a helper.
- N3 — D.2 probe substrate ambiguous (fake net.Listener vs testcontainer). Disambiguate in builder spec.

## §15.3 — PASS-WITH-NITS

Agent: `ta-go-plan-qa-proof` (built-in opus). Tool-call audit ✓ (7 tool uses, Read × 3 + read-only Bash × 4).

Measurement re-count: A=2/~45/1; B=2/~60/1; C=2/~50/1; D=0/0/0 (test-only). All under budget by stated count. Symbol grounding ✓: `LaunchRequest.AccountEnv` confirmed at service.go:132, `buildRequest` at :234, `Options` field set at :89-99 (no Store, no Policy), `networkpolicy.Provision`+`ProvisionRequest`+`PolicyMaterial` confirmed. Blocker graph correct; serialization on shared file lock confirmed.

NITs:
- N1 — 15.3.B `Provisioner` interface tag missing. Mark as `[NEW: emerges from 15.2.5.D.1]`.
- N2 — 15.3.A's "2-cluster claim" borderline. Defensible.
- N3 — `LaunchRequest` vs `Options` placement of `NetworkPolicy` field underspecified. Commit to `LaunchRequest.NetworkPolicy` (per-launch) per repo idiom.

## §15.4 — PASS

Agent: `ta-go-plan-qa-proof` (built-in opus). Tool-call audit ✓ (6 tool uses).

All 5 cited symbols confirmed in committed source at exact signatures (`tools.WriteAllowlistSection` @ allowlist.go:203, `tools.EffectiveAllowlist` @ :101 — NOT `LoadEffectiveAllowlist`, `tools.DefaultAllowlistHosts` @ :57 with 4 hosts, `project.Detect`/`DetectFrom`, `newRunCommand`/`stripRunLocalFlags`/`parsedRunFlags`). Measurement under budget. Sub-planner D deferral confirmed: `git log -- internal/services/run/` shows no 15.3 work landed.

NITs (cosmetic):
- N1 — A's root.go registration touch wording could be more explicit.
- N2 — B's spec should explicitly name `tools.Load` (or equivalent) as the manifest accessor.
- N3 — "BUILT" vs "plan accepted" wording readability.
