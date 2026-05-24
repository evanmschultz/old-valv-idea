verdict: pass

# DROP_15 — PLAN_QA_PROOF, Round 5

## Section 0 — SEMI-FORMAL REASONING

### Proposal

- **Premises**: Round 4 falsification produced five concrete asks (F1.1 macOS gate, F1.2 byte-preservation, F1.3 open-mode setup-not-called assertion, YAGNI 2.1 cut NetworkConnect helpers, hidden-dep 3.1 orphan-cleanup contract). Round 5 must verify each fix is materially present in `PLAN.md`, that no orphan symbols persist, that evidence cites still resolve, and that the `blocked_by` graph remains acyclic and matches the orchestrator brief.
- **Evidence**: `drops/DROP_15_NETWORK_POLICY/PLAN.md` lines 33, 35, 39, 98, 100, 121, 151, 155-156, 184, 191-192, 232; `internal/cli/operator_helpers.go:421-464` (live `resolveProjectImage` raw-cwd call at line 441); `internal/services/images/service_test.go:1535-1593` (NoCache rebuild seam present and matches plan cite).
- **Trace**: Read PLAN.md in full; ripgrep verified absence of stale `NetworkConnect`/`ConnectNetwork` references in `internal/` and in the plan; confirmed each R4 mitigation is present at the location the orchestrator brief identifies.
- **Conclusion**: PASS — all five R4 fixes are present with the contractual language required; evidence cites resolve.
- **Unknowns**: A1 reachability is a build/QA-phase empirical question, not a plan-phase question — the plan now correctly routes it as a Docker Desktop macOS gate on both Unit 15.2.5 and Unit 15.3.

### QA Proof Pass

- **Premises**: Every premise in the plan must trace to (a) source code evidence at the cited line range, (b) external vendor docs cited inline, or (c) a prior approved drop PLAN.md file.
- **Evidence**: Two spot-checks ran clean — `internal/cli/operator_helpers.go:441` still does raw `tools.Resolve(workingDir)` (grounds Unit 15.0); `internal/services/images/service_test.go:1535-1593` confirms `EnsureProjectRequest.NoCache = true` forces `--no-cache` rebuild (grounds Unit 15.2.5 integration test approach).
- **Trace**: Decision 5 cites docker.com docs for `--internal`, none driver, and gateway selection — three distinct URLs, each load-bearing. Decision 9 cites `code.claude.com/docs/en/corporate-proxy`, `openai/codex#16079`, and `openai/codex#14080` for the launcher-doesn't-ship-closed-default decision. All inline and dated 2026-05-22.
- **Conclusion**: Evidence completeness holds for every claim Round 5 must verify.
- **Unknowns**: Vendor URLs and GitHub issue numbers are not re-fetched in this pass; they are pinned by the planner and the dev approved them at Round 3.

### QA Falsification Pass

Attacks on the PASS verdict, each mitigated or accepted:

- **Attack F1**: Could the orphan-cleanup contract be a YAGNI add since no current Valv code path SIGKILLs itself? Mitigation: deferred-cleanup races are real in any signal-cancellable runtime; the cost of adding a deterministic label per managed network/proxy is small and the contract is testable. Mitigated by the explicit test bullet in Unit 15.2.5 acceptance ("a stale labeled network from a prior killed process is detected and cleaned/reused").
- **Attack F2**: Does the macOS gate language actually force builder action, or is it advisory? Mitigation: Unit 15.2.5 acceptance says "If the test is run only on Linux CI during build, that result does not satisfy the unit. Either automate macOS-runner coverage OR mark the test as manual-validation-required ... and confirm validation before unit close." The "before unit close" anchor makes it a gate, not advisory.
- **Attack F3**: Is the open-mode setup-not-called assertion testable when `networkpolicy` service is also exercised by closed-mode tests in the same file? Mitigation: the acceptance bullet specifies "assert via mocked policy service that `Provision` / setup methods are NOT called when `mode == open`" — a mock counter on the policy service interface is a standard table-test pattern; no risk of false green.
- **Attack F4**: Does the YAGNI cut of `NetworkConnect*` leave any unit referencing those symbols? Mitigation: ripgrep over PLAN.md and `internal/` found zero matches. Confirmed clean.
- **Attack F5**: Does the byte-preservation contract specify enough golden-fixture cases to actually catch a regression? Mitigation: Unit 15.1 acceptance lists four named edge cases (file-preamble comment, inline comment on `[allowlist]` header, divider comment between `[allowlist]` and `[tools]`, `[env]` block with interleaved comments). Round-trip rewrite must reproduce all four byte-for-byte. This is concrete and testable.
- **Attack F6**: Does the `blocked_by` graph have any cycle? Trace: 15.0→none; 15.1→none; 15.2→none; 15.2.5→{15.0, 15.1, 15.2}; 15.3→{15.1, 15.2, 15.2.5, DROP_13, DROP_14}; 15.4→{15.0, 15.1, 15.3, DROP_13}. Topological order: 15.0/15.1/15.2 first, then 15.2.5, then 15.3 (after DROP_13/14 land), then 15.4. Acyclic. Matches Round 4.

No unmitigated counterexample to the PASS verdict.

### Convergence

- (a) QA Falsification produced no unmitigated counterexample; all six attacks mitigated.
- (b) QA Proof confirmed evidence completeness: two spot-checks resolved, all R4 fixes present with required contractual language.
- (c) Remaining Unknown (A1 macOS reachability) is explicitly routed to the build/QA phase via the Docker Desktop macOS gate language on Units 15.2.5 and 15.3.

Convergence declared.

## 1. Findings

- 1.1 **R4.F1.1 macOS gate — PRESENT.** Unit 15.2.5 acceptance line 155 contains the literal language "Docker Desktop macOS gate" and the explicit non-evidence clause "Linux CI runs are NOT evidence for A1." Unit 15.3 acceptance line 192 mirrors the contract with the same non-evidence clause. The "before unit close" anchor in both makes this a gate, not advisory. Verdict: closes R4.F1.1.
- 1.2 **R4.F1.2 byte-preservation — PRESENT.** Decision 3 (line 33) tightened to "every byte outside the lexical `[allowlist]` section span verbatim, including the file preamble (comments and whitespace before the first section), divider comments between sections, inline comments on section headers other than `[allowlist]`, and the entire `[tools]` and `[env]` block bytes." Unit 15.1 acceptance line 98 mirrors the contract; line 100 lists the four required golden-fixture cases (preamble comment, `[allowlist]`-header inline comment, divider comment, `[env]` block with interleaved comments). Verdict: closes R4.F1.2.
- 1.3 **R4.F1.3 open-mode setup-not-called — PRESENT.** Unit 15.3 acceptance line 184 specifies "open-mode path does NOT invoke `internal/services/networkpolicy` setup, does NOT create a managed Docker network, and does NOT start a host proxy process ... assert via mocked policy service that `Provision` / setup methods are NOT called when `mode == open`." This is testable via interface counter; no false-green risk. Verdict: closes R4.F1.3.
- 1.4 **R4 YAGNI 2.1 NetworkConnect cut — PRESENT.** Unit 15.2 acceptance line 121 contains the literal carve-out "Do NOT add a `NetworkConnectRequest` in DROP_15 — the second-network fallback was cut from Decision 5, leaving no in-drop caller for `docker network connect`." Ripgrep confirms zero occurrences of `NetworkConnect`/`ConnectNetwork` anywhere else in PLAN.md and zero in `internal/`. Verdict: closes R4 YAGNI 2.1.
- 1.5 **R4 hidden-dep 3.1 orphan-cleanup — PRESENT.** Unit 15.2.5 acceptance line 156 contains the orphan-cleanup contract with the deterministic label requirement (`label=valv=network-policy` on networks; pidfile or label-equivalent on host proxy), the reclaim-or-clean choice, and a testable invariant ("a stale labeled network from a prior killed process is detected and cleaned/reused; the next launch does not fail on naming/port collision"). Verdict: closes R4 hidden-dep 3.1.
- 1.6 **Evidence cites still resolve.** Spot-check 1: `internal/cli/operator_helpers.go:441` still does `tools.Resolve(workingDir)` directly — grounds Unit 15.0 rerooting requirement. Spot-check 2: `internal/services/images/service_test.go:1535-1593` is `TestEnsureProjectImage_NoCacheForcesRebuild`, exactly matching the plan's "working `NoCache` rebuild seam" cite. Decision 5 still cites three docker.com URLs (none driver, network create, gateway selection). Decision 9 still cites `code.claude.com/docs/en/corporate-proxy`, `openai/codex#16079`, `openai/codex#14080`.
- 1.7 **blocked_by graph acyclic and matches Round 4.** Verified: 15.0→none, 15.1→none, 15.2→none, 15.2.5→{15.0, 15.1, 15.2}, 15.3→{15.1, 15.2, 15.2.5, DROP_13, DROP_14}, 15.4→{15.0, 15.1, 15.3, DROP_13}. Topological order exists; no cycle.
- 1.8 **Schema Decisions mutually consistent.** Decision 3 tightening (byte-preservation outside `[allowlist]`) does not conflict with Decision 1 (union semantics for built-in defaults), Decision 2 (overlay hash stays `[tools]`-only — confirmed by Unit 15.1 regression test bullet at line 101), Decision 5 (single-internal-network topology — Unit 15.2 line 121 explicitly cut the second-network helpers to match), Decision 7 (subtractive-deny error on built-ins — Unit 15.4 acceptance line 217 mirrors), or Decision 9 (launchers stay open-mode — Unit 15.3 line 177 and Notes line 237 mirror). All nine decisions hold together.

## 2. Missing Evidence

- 2.1 None. All five R4 fixes are materially present with the contractual language the falsification round required, evidence cites that I spot-checked resolve, the blocked_by graph is acyclic and matches the brief, and the nine Schema Decisions remain mutually consistent.

## 3. Summary

PASS. Round 5 finds the plan in a green state for build dispatch. The five Round 4 falsification asks (macOS gate, byte-preservation contract, open-mode setup-not-called assertion, NetworkConnect helper cut, orphan-cleanup contract) are all present with concrete, testable acceptance language anchored to "before unit close" gates. Evidence cites still resolve against committed code. The `blocked_by` graph is acyclic and matches the orchestrator brief. The only remaining Unknown (A1 `host.docker.internal` reachability over the `--internal` topology on Docker Desktop macOS) is correctly routed to the build/QA phase via the Docker Desktop macOS gate language on Units 15.2.5 and 15.3, not absorbed into a plan-level claim.

## TL;DR

- T1: All five Round 4 mitigations (macOS gate, byte-preservation, open-mode setup-not-called, NetworkConnect cut, orphan-cleanup) are present in PLAN.md with the required contractual language; spot-checked evidence cites resolve; blocked_by graph acyclic; nine Schema Decisions mutually consistent. No remaining findings.
- T2: No missing evidence.
- T3: PASS — plan is green for build dispatch; the one Unknown (Docker Desktop macOS reachability for `host.docker.internal` over `--internal`) is correctly gated to build/QA, not absorbed into the plan.
