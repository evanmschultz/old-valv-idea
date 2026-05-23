verdict: pass

# DROP_15 — Plan QA Proof, Round 4

Round 4 plan-QA proof pass. Independent of falsification.

## 1. Per-R3-finding audit

### 1.1 R3.F1 — fallback topology egress leak — ADDRESSED

Round 4 has three explicit, mutually reinforcing scope cuts that close the F1 hole:

- **Schema Decision 5** (PLAN.md line 35): "Do NOT ship a `docker network connect bridge` fallback ... Existing repo evidence only proves `host.docker.internal` is already used by the Codex bridge ... and provides no gateway-priority or post-attach egress-proof seam, so DROP_15 accepts a platform limitation instead of a speculative second-network fallback."
- **Notes A1** (line 232): "If `host.docker.internal` does not resolve/reach the host-local proxy from the single `--internal` network topology on Docker Desktop macOS, block DROP_15 on that platform. Do **not** attach `bridge` or any second network in DROP_15 as a fallback."
- **Unit 15.2.5 acceptance** (line 151): "Do NOT attach a second Docker network in DROP_15. If validation shows the proxy is unreachable from that topology on supported macOS, this unit must fail/block the drop rather than `docker network connect bridge`."
- **Unit 15.3 acceptance** (line 188): "The integration proof MUST use the same single-network topology the product ships. Do NOT attach bridge or any second Docker network in DROP_15. If the proxy is unreachable from that topology on supported macOS, this unit blocks the drop."
- **Drop acceptance criterion 6** (line 48): "If `host.docker.internal` is unreachable from that `--internal` build topology on supported macOS, the unit blocks the drop rather than attaching a second network that can change default egress."

The plan converts the F1 leak path into a deterministic build-time block, not a silent fallback that could change default egress. Closed cleanly.

### 1.2 R3.F2 — `valv network deny` on built-in misleading — ADDRESSED

- **Schema Decision 7** (line 37): "Attempting to deny one of the built-in defaults MUST return a deterministic error, not a success or warning, because the effective policy would remain unchanged."
- **Drop acceptance criterion 8** (line 50): "`valv network deny <built-in-default>` returns a deterministic error and leaves the effective allowlist unchanged; tests cover that exact case."
- **Unit 15.4 acceptance** (line 213): "`deny` is idempotent for missing user-added hosts but MUST return a deterministic error when the target is one of the built-in default hosts, because the effective policy would remain unchanged. Built-in default hosts remain effective and are not subtractable in DROP_15."
- **Unit 15.4 test** (line 223): "deterministic error on denying one of the four built-in defaults".

Closed cleanly. The CLI contract is consistent with union semantics, and the test surface pins it.

### 1.3 R3.F3 — WriteAllowlistSection contract under-tested — ADDRESSED

- **Schema Decision 3** (line 33): "Supported input is: UTF-8 text, LF line endings, no UTF-8 BOM, and no TOML multi-line strings outside `[allowlist]`. Within that supported shape it MUST preserve `[tools]` and `[env]` block order and content VERBATIM, including comments and blank lines within those sections. Unsupported shapes return a deterministic error instead of best-effort rewrite."
- **Drop acceptance criterion 3** (line 45): "On supported input it preserves `[tools]` and `[env]` block order/content verbatim, including comments and blank lines within those sections; unsupported shapes return a deterministic error."
- **Unit 15.1 acceptance** (line 98-99): supported manifest shape constraint plus "deterministic rejection of BOM, CRLF, and multi-line-string/header-collision inputs".
- **Notes** (line 236): "Keep the `WriteAllowlistSection` contract literal: reject BOM/CRLF/multi-line-string unsupported shapes rather than normalizing or best-effort rewriting them."

The contract is now precise (supported-shape enumeration) and its rejection cases are explicit tests. Closed cleanly.

### 1.4 R3.F4 — provider-CLI proxy ship gate — ADDRESSED

- **Schema Decision 9** (line 39): cites Anthropic enterprise corporate-proxy docs ("currently state support for `HTTP_PROXY`, `HTTPS_PROXY`, and `NO_PROXY`") and OpenAI Codex open proxy regressions `openai/codex#16079` (2026-03-28) and `openai/codex#14080` (2026-03-09); concludes "DROP_15 keeps `valv claude` / `valv codex` on open-mode defaults and does not make them the ship gate for closed-default readiness."
- **Drop acceptance criterion 4** (line 46): "`valv claude` / `valv codex` remain open-mode callers in DROP_15."
- **Unit 15.3 acceptance** (line 175): "`valv claude` / `valv codex` remain open-mode callers in this drop and are not flipped here."
- **Unit 15.4 acceptance** (line 216): "those launchers remain open-mode callers in DROP_15 and do not flip default egress here."
- **Notes A6** (line 233): "Provider-launcher proxy compliance is not a DROP_15 ship gate ... Do not silently flip them to closed-default without a separate launcher-level proof pass after Codex proxy behavior is proven stable."

The deferral is sourced in concrete upstream issue numbers and a vendor-doc citation rather than design hand-waving. Closed cleanly.

## 2. Per-claim audit of NEW Round 4 content vs the actual repo

All cited repo locations resolved as described:

### 2.1 `internal/cli/operator_helpers.go:421-464` — VERIFIED

`resolveProjectImage` exists at line 440 with `manifest, err := tools.Resolve(workingDir)` at line 441 — exactly the raw-cwd call Unit 15.0 targets. The function range matches; Unit 15.0's diagnosis is correct.

### 2.2 `internal/tools/tools.go:75-130` — VERIFIED

`ToolManifest` is at lines 75-79 with `Allowlist toml.Primitive` at line 77 and `Env toml.Primitive` at line 78. `Load` at line 89 explicitly notes Allowlist/Env are decoded into discarded targets "DROP_14 and DROP_15 will re-call PrimitiveDecode with their typed targets" (lines 106-112) — Unit 15.1's promotion plan is correctly grounded.

### 2.3 `internal/services/images/overlay.go:30-70` — VERIFIED

`canonicalManifest` (line 35) and `OverlayHash` (line 60) operate only on `manifest.Tools` (line 36, 39) — confirming Decision 2 that "overlay image identity stays `[tools]`-only" and that adding allowlist data must not change the hash.

### 2.4 `internal/adapters/docker/ops.go:9-21,38-72` — VERIFIED

`ImageBuildRequest` struct at lines 9-21 declares `Network string` at line 17. `BuildImageArgs` at line 38 emits `--network` at lines 70-72. Unit 15.2.5's reuse of the existing `ImageBuildRequest.Network` field is valid.

### 2.5 `internal/adapters/docker/types.go:43-60,141-218` — VERIFIED

`ContainerRunRequest` struct at lines 43-60 declares `Network string` at line 58. `buildRunLikeArgs` at lines 141-219 emits `--network` at lines 171-173. Unit 15.3's reuse of the existing `ContainerRunRequest.Network` field is valid.

### 2.6 `internal/services/images/service_test.go:1535-1595` — VERIFIED

`TestEnsureProjectImage_NoCacheForcesRebuild` exists at line 1535. It sets `EnsureProjectRequest.NoCache: true` (line 1564), proves the rebuild path, and asserts `--no-cache` in the build call (lines 1583-1592). Unit 15.2.5's reliance on `EnsureProjectRequest.NoCache = true` for the rebuild seam is valid evidence.

### 2.7 DROP_13 cross-drop refs — VERIFIED

- `drops/DROP_13_GENERIC_RUN/PLAN.md:36-43,59-96` confirms `internal/services/run` as shared launch seam.
- `drops/DROP_13_GENERIC_RUN/PLAN.md:78-96` confirms Unit 13.2 places `valv run` in `internal/cli/run.go`.
- DROP_13 PLAN.md acceptance criterion (line 65) confirms `VALV_<PROVIDER>_IMAGE` short-circuit behavior preserved.

### 2.8 DROP_14 cross-drop refs — VERIFIED

`drops/DROP_14_ENV_VARS/PLAN.md` Unit 14.4 (around line 80-83) is the env-var re-home into `internal/services/run` after DROP_13 — the gate that Unit 15.3's "Cross-drop env-merge gate" references. Cite "69-70" in DROP_15 is slightly off line-number-wise (Unit 14.4 starts around line 79), but the substance (env merging re-home into `internal/services/run`) is exactly what DROP_14 Unit 14.4 declares.

### 2.9 Project-root detection — VERIFIED

`internal/cli/tools.go:44-84` (Round 4 reference) confirms `project.Detect()` then `tools.Resolve(result.Root)` pattern. `internal/project/project.go:26-55` confirms `DetectFrom(start)` is available for explicit subdir start paths. Unit 15.0 alignment with that pattern is well-grounded.

### 2.10 Codex bridge — VERIFIED

`internal/adapters/providers/codex/bridge.go:42-60` shows `newBridgeManager` constructs `baseURL: fmt.Sprintf("http://host.docker.internal:%d", ...)` (line 60) — confirming Decision 5's claim that `host.docker.internal` is already in use by the Codex bridge and is the topology DROP_15 must rely on.

## 3. Unit-by-unit completeness check

### 3.1 Unit 15.0 — Re-root project-image manifest resolution

- Paths: `internal/cli/operator_helpers.go`, `internal/cli/codex_project_image_test.go`, `internal/cli/claude_project_image_test.go` — all exist.
- Packages: `internal/cli` — single-package scope.
- Evidence cites resolve (see 2.1, 2.9).
- Acceptance: yes/no-verifiable — "raw `tools.Resolve(workingDir)` is removed", "tests add repo-subdirectory coverage".
- `blocked_by`: nothing — appropriate for a CLI-only re-root.

### 3.2 Unit 15.1 — Typed allowlist + WriteAllowlistSection

- Paths include new `allowlist_test.go` flagged "new, not yet in tree" — accurate.
- Packages: `internal/tools`, `internal/services/images` — touches existing `overlay_test.go` for the regression.
- Evidence cites resolve (see 2.2, 2.3).
- Acceptance: yes/no-verifiable — supported-shape enumeration, deterministic rejection cases, OverlayHash invariance.
- `blocked_by`: nothing — file-format + writer plumbing has no upstream dependency.

### 3.3 Unit 15.2 — Docker network lifecycle helpers

- Paths: new `network.go` + `network_test.go` flagged "new, not yet in tree"; existing `executor.go` touched.
- Packages: `internal/adapters/docker` — single-package.
- Evidence cite confirmed (see 2.4, 2.5).
- Acceptance: yes/no-verifiable — typed structs + arg builders + executor methods + deterministic arg order test.
- `blocked_by`: nothing.

### 3.4 Unit 15.2.5 — Shared networkpolicy service + image-build egress

- Paths: new `internal/services/networkpolicy/` + existing `internal/services/images/service.go`, `service_test.go`, `service_integration_test.go`.
- Packages: `internal/services/networkpolicy`, `internal/services/images`.
- Evidence cite confirmed (see 2.6); ties to Unit 15.0's project-root manifest seam ("do not add any new raw-cwd `.valv/tools.toml` lookup").
- Acceptance: yes/no-verifiable — proxy build args injected, internal network attached, OverlayHash unchanged, freshness labels preserved, integration test forces rebuild via `NoCache=true`, single-network topology only.
- `blocked_by`: 15.0, 15.1, 15.2 — explained by the policy material flowing from 15.1's typed allowlist, the network helpers from 15.2, and the root-rerooting from 15.0.

### 3.5 Unit 15.3 — Generic-run closed-default runtime policy

- Paths: all new under `internal/services/run/` (expected from DROP_13).
- Packages: `internal/services/run`.
- Evidence cite confirmed (DROP_13 plan; current duplicated provider service paths; Codex bridge `host.docker.internal`).
- Acceptance: yes/no-verifiable — closed/open mode, proxy env injection, internal network, error wrapping, DROP_14 env merge gate, integration test using single-network topology, no second-network fallback.
- `blocked_by`: 15.1, 15.2, 15.2.5, DROP_13, DROP_14 — the policy material (15.1, 15.2.5), the docker network helpers (15.2), the shared seam (DROP_13), and the env-var re-home (DROP_14).

### 3.6 Unit 15.4 — CLI network management + open-egress opt-out

- Paths: new `internal/cli/network.go` + `network_test.go`; existing `root.go`; new `run.go` (expected from DROP_13).
- Packages: `internal/cli`.
- Evidence cite confirmed (existing `tools.go` pattern; `root.go:108-139` registration point; `operator_helpers.go:421-464` from Unit 15.0; DROP_13 `valv run` placement).
- Acceptance: yes/no-verifiable — subcommands, project-root resolution, deterministic-error on built-in deny, `list` reports unioned effective allowlist, `--network open` toggle, no duplicated flag parsing in pre-DROP_13 launchers.
- `blocked_by`: 15.0, 15.1, 15.3, DROP_13 — 15.0 for the root-rerooting CLI seam, 15.1 for the typed allowlist + WriteAllowlistSection, 15.3 for the run-service open/closed seam, DROP_13 for `valv run`'s existence.

## 4. blocked_by graph audit

Edges:

- 15.0 → ∅
- 15.1 → ∅
- 15.2 → ∅
- 15.2.5 → {15.0, 15.1, 15.2}
- 15.3 → {15.1, 15.2, 15.2.5, DROP_13, DROP_14}
- 15.4 → {15.0, 15.1, 15.3, DROP_13}

Topological order: 15.0, 15.1, 15.2 → 15.2.5 → 15.3 → 15.4. No back-edges. Cross-drop deps (DROP_13, DROP_14) are upstream containers, not units in this drop, so they cannot induce a cycle. **Acyclic, confirmed.**

The graph is also minimal — no edge is shadowed by transitive closure that adds risk (e.g., 15.4's `{15.0, 15.1, 15.3, DROP_13}` keeps 15.1 explicit even though 15.3 transitively depends on 15.1, because 15.4 directly consumes `WriteAllowlistSection` from 15.1).

## 5. Schema Decision audit

Nine decisions, all evidence-grounded; consistency checks below.

### 5.1 Each decision has supporting evidence

- Decision 1 (union semantics, four default hosts): cites `internal/tools/tools.go:75-130` placeholder state.
- Decision 2 (allowlist outside OverlayHash): cites `overlay.go:30-70` and `overlay_test.go:12-147`.
- Decision 3 (WriteAllowlistSection, narrowed supported shape): cites `os.MkdirAll`, `filepath.Dir`, `tools/resolve.go:11-30`.
- Decision 4 (runtime enforcement on shared run seam): cites DROP_13 PLAN, current duplicated provider services.
- Decision 5 (single internal network, no bridge fallback): cites Docker docs `none`, `--internal`, `--gw-priority` semantics; cites Codex bridge `host.docker.internal` use.
- Decision 6 (image-build proxy build args): cites Docker docs predefined proxy build args.
- Decision 7 (deny errors on built-ins): cites Decision 1 union semantics.
- Decision 8 (project-root resolution): cites `operator_helpers.go:421-464`, `tools.go:44-84`, `project.go:26-55`, `project_test.go:13-48,67-95,112-144`.
- Decision 9 (`valv run`-only closed-default ship gate): cites Anthropic corporate-proxy docs, OpenAI Codex issues #16079 and #14080, current duplicated provider services.

### 5.2 Decision 1 (union semantics) ↔ Decision 7 (deny errors on built-ins) consistent

Decision 1 fixes the effective allowlist = built-in defaults ∪ user hosts (lowercased, deduped). Decision 7 says removing a built-in default would leave the effective allowlist unchanged because the union still includes it. The deterministic-error response is the only contract consistent with union semantics; warning or no-op would be semantically misleading. **Consistent.**

### 5.3 Decision 3 (narrowed writer contract) ↔ Decision 8 (project-root resolution) consistent

Decision 3 governs HOW the file is rewritten (supported-shape constraint, section preservation, deterministic rejection). Decision 8 governs WHERE the file is found (detected project root). Orthogonal axes; no constraint overlap. Both are enforced at the CLI layer (Unit 15.4) plus library layer (Unit 15.1 for the writer, Unit 15.0 for the root). **Consistent.**

### 5.4 Decision 5 (no bridge fallback) ↔ Decision 9 (`valv run`-only ship gate) consistent

Decision 5 is a topology constraint: single internal network only. Decision 9 is a scope constraint: only `valv run` plus overlay-build ship closed-default. They are independent dimensions. The fact that providers stay open-mode in DROP_15 does NOT reintroduce the bridge-fallback option — providers in open mode use today's default egress, not a connect-bridge path. **Consistent.**

### 5.5 No cross-decision contradictions

Scanned all 9 pairwise — no contradictions detected.

## 6. Findings

No findings. Round 4 closes all four R3 falsification findings cleanly with text, evidence, and acceptance tests pinned to specific repo seams; cited repo locations resolve; the dependency graph is acyclic; the nine schema decisions are mutually consistent.

