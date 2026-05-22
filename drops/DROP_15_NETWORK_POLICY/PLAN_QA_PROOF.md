# DROP_15 — Plan QA Proof — Round 1

**Verdict:** PASS WITH FINDINGS

**Reviewer:** ta-go-qa-proof (orchestrator-direct write — Agent returned content but reported its Write tool was denied; orchestrator applied)
**Plan file:** main/drops/DROP_15_NETWORK_POLICY/PLAN.md @ 37ce350
**Hylla artifact_ref:** github.com/evanmschultz/valv@main (1759e64)
**Scope:** per-unit paths concrete, acceptance testable, blocked_by sound, Hylla evidence cited correctly, design choice (closed-default proxy + `--internal` network) internally consistent, reject-list reasoning sound, AllowlistConfig schema adequate for v1.

## 1. Hylla / Source Evidence Verification

- 1.1 `internal/tools/tools.go:70-130` cited for `Allowlist toml.Primitive` + `PrimitiveDecode` placeholder — accurate. `ToolManifest` declared lines 75-79 with `Allowlist toml.Primitive` and `Env toml.Primitive`; `meta.PrimitiveDecode(m.Allowlist, &discardAllowlist)` at 113-116. Doc-comment 70-74 explicitly reserves DROP_15 for type-decoding `[allowlist]`.
- 1.2 `internal/services/images/overlay.go:35-70` cited for tools-only hashing — accurate. `canonicalManifest` iterates only over `manifest.Tools`; `OverlayHash` marshals only that canonical slice. Allowlist + Env fields never touched in hash computation.
- 1.3 `internal/adapters/docker/types.go:43-59` cited for `ContainerRunRequest.Network` — accurate (field at line 58 between User and Extra). Emission at `command.go:171-173` within `buildRunLikeArgs`.
- 1.4 `internal/adapters/docker/command.go:16-82` "container lifecycle only" — accurate. Executor exposes Run/Create/Inspect/Exec/ExecStream/Start/Attach — zero network-operation methods.
- 1.5 `internal/cli/tools.go:21-93` project-scoped manifest command pattern — accurate.
- 1.6 `internal/cli/manage.go:22-63` multi-subcommand branch pattern — accurate.
- 1.7 `drops/DROP_13_GENERIC_RUN/PLAN.md:5-18` shared runtime seam — accurate; DROP_13 introduces `internal/services/run/service.go` (Unit 13.1) and `internal/cli/run.go` (Unit 13.2).
- 1.8 `internal/services/claude/service.go:180-329` and `internal/services/codex/service.go:171-332` for duplicated request-building path — line ranges plausible but not personally re-verified. DROP_13 PLAN cites `:128-222` and `:121-214` for the same Service.Run boundary. **Finding F2: cross-drop line-range drift — same identified duplication, slightly different line ranges between drops.**

## 2. Design Choice — `--internal` + Proxy

- 2.1 Docker docs (Context7) verified:
  - `docker network create --internal` "completely isolates containers on a network from any communications external to that network" — confirms closed-default.
  - `--network none` provides "only the loopback device" — confirms rejection.
  - `host.docker.internal` is "the internal IP address used by the host" — canonical Docker-Desktop host-reach.
- 2.2 Planner's claim that `--internal` blocks external routes while allowing host/gateway communication is consistent with Docker's documented model. **Residual: whether `host.docker.internal` resolves AND routes successfully from a container attached only to an `--internal` network on Docker Desktop for macOS is not directly proven by Context7 excerpts.** Empirical confirmation is part of Unit 15.3's integration acceptance. Plan correctly routes the risk to the integration test.
- 2.3 Rejection of iptables/ipset well-grounded for Docker Desktop on macOS: container traffic exits Linux VM and is mediated by Docker Desktop's macOS-side networking. CLAUDE.md § Platform Scope confirms macOS-only.

## 3. AllowlistConfig Schema (v1 surface)

- 3.1 Narrowed to exact hostnames / Docker aliases only, normalized to lowercase, deduped, no CIDR/URL prefixes/scheme/path/port. Consistent with HTTP/HTTPS proxy enforcement.
- 3.2 Schema constraint testable: lowercase normalization, deduplication, rejection of CIDR / `://` / `/path` are unit-testable invariants.
- 3.3 HTTPS host-allowlist via SNI/CONNECT-target matching implementable without TLS termination.

## 4. blocked_by Edges

- 4.1 Within DROP_15: Unit 15.3 → {15.1, 15.2}; Unit 15.4 → {15.1, 15.3}. Sound. 15.1 and 15.2 independent.
- 4.2 Cross-drop: 15.3 blocked_by DROP_13 AND DROP_14; 15.4 blocked_by DROP_13.
- 4.3 **Finding F1 (most important):** Unit 15.3 acceptance asserts "DROP_14 env vars are merged, not overwritten" inside `internal/services/run/service.go`. But DROP_14 PLAN Units 14.4 + 14.5 place env-var merging in `internal/services/codex/service.go` and `internal/services/claude/service.go`. DROP_14 line 70 explicitly anticipates re-homing if DROP_13 moves launch ownership. By the time DROP_15 builds, DROP_13 will have moved launch into `internal/services/run` and DROP_14 will need to re-home env merging there — but this is a plan ordering assumption, not a verified contract. **Recommend planner adds one explicit line to Unit 15.3 acceptance:** "depends on DROP_14 having re-homed env-var merging into `internal/services/run` per DROP_14 PLAN line 70."

## 5. Per-Unit Concreteness

- 5.1 Unit 15.1 — paths concrete. `WriteAllowlistSection(path, cfg AllowlistConfig) error` signature given. Section-preserving rewrite is a strong constraint. **Finding F3:** BurntSushi/toml encoder does not preserve comments / whitespace / block ordering, so section-preserve must be hand-rolled splice. Acceptance does not specify whether COMMENT preservation is required. Recommend explicit treatment.
- 5.2 Unit 15.2 — paths concrete. Three new request types, three executor methods. Tests cover validation, alias ordering, `--internal`, cleanup. Acceptance testable without Docker (pure arg-builder tests).
- 5.3 Unit 15.3 — paths concrete but two are new in DROP_13. Acceptance includes table-driven unit + testcontainers integration. Strong ship-gate framing. Carries F4 residual.
- 5.4 Unit 15.4 — paths concrete. Acceptance covers subdir detection, create-from-absent, dedupe + lowercase, deny-no-op, section preservation, `--network open` passthrough.

## 6. Open Egress Opt-Out

- 6.1 `--network open` placed on `valv run` (DROP_13 entrypoint), not retrofitted to `valv claude`/`valv codex`. Line 132 explicitly states the dependency.
- 6.2 Two-mode contract (closed/open) well-defined; no third mode introduced.

## 7. Coverage of Scope Items

| Scope item | Result |
|---|---|
| 1. Per-unit paths concrete | Pass (F3 minor) |
| 2. Acceptance testable | Pass |
| 3. blocked_by sound (intra + cross-drop) | Pass with F1 |
| 4. Hylla evidence cited correctly | Pass with F2 |
| 5. `--internal` + proxy design consistent | Pass (Context7-verified; F4 routed to integration test) |
| 6. Reject-list reasoning sound | Pass |
| 7. AllowlistConfig schema adequate v1 | Pass |

## 8. Findings Summary

- **F1** Cross-drop coupling assumption: Unit 15.3 acceptance assumes DROP_14 env merging has been re-homed to `internal/services/run`. Recommend explicit cross-drop note linking to DROP_14 PLAN line 70.
- **F2** Citation line-range drift: minor inconsistency between DROP_15 PLAN's `:180-329` / `:171-332` and DROP_13 PLAN's `:128-222` / `:121-214` for the same Service.Run boundary.
- **F3** Section-preserving TOML rewrite: acceptance does not specify comment preservation. Recommend explicit treatment.
- **F4** Residual integration risk: `host.docker.internal` reachable from `--internal` network on macOS Docker Desktop not directly proven by docs. Plan correctly routes to integration test.

None of F1–F4 are blockers.

## 9. Verdict

**PASS WITH FINDINGS.** Plan is internally consistent, Hylla-grounded, Docker-doc-grounded, atomically decomposed. F1–F4 are pre-build sharpening recommendations. F1 (cross-drop env-var re-homing) is most important — recommend planner adds the one-line explicit cross-drop note. F2/F3/F4 minor.

The closed-by-default design is sound: `--internal` blocks external routes per Docker docs; `host.docker.internal` is canonical Docker-Desktop host-reach; rejection of `--network none` (loopback-only) and Linux iptables (wrong macOS portability target) is well-grounded.

Ship-gate framing in Unit 15.3 ("if it is incomplete, DROP_15 is not releasable") correctly identifies the workload-level "closed by default" property as the drop's defining commitment.

No unmitigated counterexample to the verdict.
