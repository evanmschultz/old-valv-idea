# DROP_15 — Plan QA Proof — Round 2

**Verdict:** PASS

**Reviewer:** ta-go-qa-proof (orchestrator-direct write — Agent reported Write tool not in its function list; orchestrator persisted the returned content)
**Plan file:** main/drops/DROP_15_NETWORK_POLICY/PLAN.md @ 837ad1b
**Hylla artifact_ref:** github.com/evanmschultz/valv@main (1759e64)

## 1. Findings

- 1.1 Round 1 blocker A2 (image-build egress) fully resolved by new Unit 15.2.5. Cites `internal/services/images/overlay.go:84-146` and `service.go:718-838`. Threads predefined Docker proxy build args + `ImageBuildRequest.Network`. Includes integration-test gate. Preserves `OverlayHash` invariance under policy changes.
- 1.2 Round 1 blocker A3 (default allowlist hosts) fully resolved. Union semantics with four built-in defaults (`proxy.golang.org`, `sum.golang.org`, `objects.githubusercontent.com`, `github.com`) stated consistently at Schema Decision 1, drop-level Acceptance 1, Unit 15.1 acceptance, and Unit 15.4 acceptance. Subtractive overrides explicitly disallowed in DROP_15 (Schema 7 + Unit 15.4).
- 1.3 Round 1 blocker A4 (mkdir parent `.valv/`) fully resolved with explicit `os.MkdirAll(filepath.Dir(path), 0o755)` text in Schema Decision 3 and Unit 15.1 acceptance. Test coverage cited at Unit 15.1 + Unit 15.4.
- 1.4 Round 1 finding F1 (cross-drop env-merge gate to DROP_14) resolved at both drop-level Acceptance 4 and Unit 15.3 acceptance. Cited line range `drops/DROP_14_ENV_VARS/PLAN.md:69-70` verified.
- 1.5 Round 1 finding F2 (citation drift) resolved. Sampled ten distinct citations across `internal/tools/`, `internal/services/images/`, `internal/adapters/docker/`, `internal/adapters/providers/codex/`, `internal/services/claude/`, `internal/services/codex/`, plus DROP_13's and DROP_14's PLAN.md targets — all resolved to claimed symbols at claimed ranges.
- 1.6 Round 1 finding F3 (section-preservation scope) resolved with capitalized VERBATIM language and explicit "including comments and blank lines" at three layers (Schema 3, Acceptance 2, Unit 15.1).
- 1.7 Open architectural risks A1 (`host.docker.internal` from `--internal` networks on Docker Desktop macOS) and A6 (Claude/Codex CLI proxy compliance) routed correctly in Notes For Builder Agents. Both give the builder a concrete fallback architecture (A1) or escalation path (A6). Correct disposition — integration-test gates, not plan-level blockers.
- 1.8 Unit count is 5 (15.1, 15.2, 15.2.5, 15.3, 15.4). Numbering does not conflict. Block-edge graph acyclic: 15.1 ←(none), 15.2 ←(none), 15.2.5 ←{15.1, 15.2}, 15.3 ←{15.1, 15.2, 15.2.5, DROP_13, DROP_14}, 15.4 ←{15.1, 15.3, DROP_13}. Package-overlap ordering justified at Notes.
- 1.9 Schema decisions evidence-grounded with cited file ranges (verified) and Context7 `/docker/docs` references for external Docker semantics.
- 1.10 Unit acceptance criteria atomic per builder. Each unit's scope is a clean package boundary with explicit `Blocked by` clauses and test-coverage lists.

## 2. Missing Evidence

- 2.1 None on the plan content side. Every premise backed by a verified citation.
- 2.2 No independent Context7 `/docker/docs` re-query in this round for `--internal` network behavior, `--network none` semantics, or predefined proxy build args. Plan's Context7 references match documented Docker behaviors; Unit 15.3's integration test is the empirical gate.

## 3. Verdict

**PASS.** Round 2 resolves every Round 1 blocker (A2, A3, A4) and finding (F1, F2, F3) with concrete schema decisions, acceptance criteria, and test gates. Cross-drop dependency on DROP_14's env-merge re-home explicitly gated. New Unit 15.2.5 cleanly closes the image-build egress gap with shared `internal/services/networkpolicy` seam preventing duplication. Open risks A1 and A6 correctly routed as integration-test gates with concrete fallback architectures. Plan ready for Phase 3 (discuss + cleanup) with the dev.
