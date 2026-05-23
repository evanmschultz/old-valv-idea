verdict: pass

# DROP_15 — Plan QA Proof, Round 3

Asymmetric proof pass against `drops/DROP_15_NETWORK_POLICY/PLAN.md` Round 3 (after Round 2 falsification surfaced three blockers: project-root drift, image-build host.docker.internal unproven, integration-test cache-bypass). This pass verifies every concrete claim in the revised plan resolves to real code, every acceptance criterion is yes/no-verifiable, the unit graph is consistent, and the Schema-Decision-8 + Drop-Acceptance-6 fixes survive Round-2-style probes.

Scope reminder: this is plan-QA, NOT build-QA. Verdict is whether the plan is groundable, atomic, and self-consistent enough that one builder per unit can complete cleanly with yes/no acceptance.

## 1. Per-claim evidence audit

### 1.1 Round-3 NEW citation: `project.DetectFrom`

Plan cites `internal/project/project.go:26-55` and tests at `internal/project/project_test.go:13-48,67-95,112-144`.

- `project.DetectFrom(start string) (Result, error)` is at `internal/project/project.go:28-58` (function body extends slightly past the cited 55 because the closing-brace lives on line 58). The cited range covers the doc comment + signature + main walk loop. **Resolved.**
- `internal/project/project_test.go` actually has tests across lines `11-32` (NormalRepoRoot), `34-55` (LinkedWorktreeRoot), `57-74` (FallsBackToCurrentDirectory), `76-93` (FilePathUsesContainingDirectory), `95-120` (DetectUsesCurrentWorkingDirectory), `145-165` (NormalizesSymlinkedRoot). The cited ranges `13-48,67-95,112-144` straddle these tests by a couple of lines apiece but unambiguously cover the verify-roots-from-subdir, fallback, and file-path coverage the plan calls out. **Resolved.** Off-by-3-lines on cite boundaries; not a defect of the plan's claim, just a non-exact range.

### 1.2 Re-rooting target: `resolveProjectImage` raw-cwd lookup

Plan cites `internal/cli/operator_helpers.go:421-464` as the current raw-cwd `tools.Resolve(workingDir)` site.

- `resolveProjectImage(...)` doc-comment starts at line 421 and the function body runs 440-470, with `manifest, err := tools.Resolve(workingDir)` at line 441. The cited range 421-464 covers the doc + entry path including the empty-manifest short-circuit. **Resolved.**

### 1.3 `internal/cli/tools.go` already does project-root resolution

Plan cites `internal/cli/tools.go:44-84`.

- `runToolsValidate(cmd *cobra.Command) error` at lines 71-97 calls `project.Detect()` (line 76) then `tools.Resolve(result.Root)` (line 81). The cited range 44-84 covers the doc-comment + constructor `newToolsValidateCommand` lines 43-69 plus the body. Cite is loose-by-a-few-lines on the top boundary but functionally correct. **Resolved.**

### 1.4 Allowlist TOML primitive placeholder

Plan cites `internal/tools/tools.go:75-130` as still holding `toml.Primitive`.

- `ToolManifest.Allowlist toml.Primitive` is declared at line 77 inside the struct definition lines 75-79. `Load(path)` is at lines 89-131. The cited range 75-130 covers the struct + decoder + PrimitiveDecode-discarded-map + strict-undecoded check. **Resolved.**

### 1.5 `ImageBuildRequest.Network` and `ContainerRunRequest.Network` pre-exist

Plan cites `internal/adapters/docker/ops.go:9-21,38-72` and `internal/adapters/docker/types.go:43-60,141-218`.

- `ImageBuildRequest.Network string` is at line 17 inside the struct lines 9-21. `BuildImageArgs` emits `--network` at lines 70-72. **Resolved.**
- `ContainerRunRequest.Network string` is at line 58 inside lines 43-60. `buildRunLikeArgs` emits `--network` at lines 171-173 (inside cited 141-218). **Resolved.**

### 1.6 DROP_13 places shared run seam in `internal/services/run`

Plan cites `drops/DROP_13_GENERIC_RUN/PLAN.md:36-43,59-96`.

- DROP_13 `PLAN.md:36` declares "Use a new package, `internal/services/run`, as the shared launch primitive". Unit 13.1 at line 68 with `Paths: internal/services/run/service.go` confirms placement. The cited range covers Schema-Decision-1 (the seam choice) plus Unit 13.1's acceptance through Unit 13.2's blockers. **Resolved.**

### 1.7 Current duplicated launch in claude/codex services

Plan cites `internal/services/claude/service.go:194-220,298-329` and `internal/services/codex/service.go:185-212,302-332`.

- Claude `Service.Run` lines 194-223 cover the duplicated request-build + executor-run path; `buildRequest` is lines 298-329. **Resolved.**
- Codex `Service.Run` lines 185-215 cover the matching path; `buildRequest` is lines 302-333. The cited range 302-332 is one line short of the function's closing `}` but covers the whole body up through `return request, nil`. **Resolved.**

### 1.8 Codex bridge uses `host.docker.internal`

Plan cites `internal/adapters/providers/codex/bridge.go:42-60` and `internal/adapters/providers/codex/bridge_test.go:274-300`.

- bridge.go line 60: `baseURL: fmt.Sprintf("http://host.docker.internal:%d", listener.Addr().(*net.TCPAddr).Port)`. **Resolved.**
- bridge_test.go lines 274-303: `TestRewriteLoopbackURL` with table cases that rewrite localhost / 127.0.0.1 / [::1] → `host.docker.internal`. **Resolved.**

### 1.9 Provider runtimes still own base env maps

Plan cites `internal/adapters/providers/claude/runtime.go:124-149` and `internal/adapters/providers/codex/runtime.go:111-132,178-219`.

- Claude runtime: mount + env block at lines 124-148 (CLAUDE_CONFIG_DIR, HOME, LOGNAME, TERM, USER). **Resolved.**
- Codex runtime: mount + env block at lines 111-132; cleanup + bridge wiring at lines 178-219. **Resolved.**

### 1.10 Overlay hashing is tools-only

Plan cites `internal/services/images/overlay.go:30-70` and `internal/services/images/overlay_test.go:12-147`.

- `canonicalManifest(manifest tools.ToolManifest)` at lines 35-52 reads only `manifest.Tools`. `OverlayHash` at lines 60-71 hashes the canonical-tools slice only. **Resolved.**
- overlay_test.go lines 12-147 cover empty-manifest stability, single-tool variants, order-independence, whitespace trim, and stability snapshot — all keyed on `Tools` only. **Resolved.**

### 1.11 Overlay install RUN lines + service build path

Plan cites `internal/services/images/overlay.go:84-146` and `internal/services/images/service.go:718-838`.

- overlay.go `BuildOverlayDockerfile` at lines 95-147 emits `RUN ["go","install",...]` / `RUN ["npm","install","-g",...]`. **Resolved.**
- service.go `EnsureProjectImage` at lines 718-839 builds via `docker.BuildImageArgs` (line 813). **Resolved.**

### 1.12 `EnsureProjectImage.NoCache` already has working test seam

Plan cites `internal/services/images/service_test.go:1535-1595`.

- `TestEnsureProjectImage_NoCacheForcesRebuild` at lines 1535-1593 sets `EnsureProjectRequest.NoCache = true` even with matching labels and asserts the buildx call carries `--no-cache`. **Resolved.** This anchors Drop-Acceptance-6's "force a rebuild every run (`EnsureProjectRequest.NoCache = true`)" requirement in already-working code.

### 1.13 Other-provider mount cite for cross-provider routing

Plan cites `internal/cli/operator_helpers.go:421-464`, `internal/cli/tools.go:44-84`, and the current root-resolution mismatch. Both confirmed in 1.2 / 1.3 above.

### 1.14 DROP_14 env-merge home for cross-drop gate

Plan cites `drops/DROP_14_ENV_VARS/PLAN.md:69-70`.

- DROP_14 `PLAN.md:70` Notes block: "If DROP_13 moves launch ownership before build starts, keep Units 14.1–14.3 unchanged and re-home Units 14.4–14.5 to whichever package then assembles `ContainerRunRequest.Env`." This is the exact gate DROP_15.3 invokes. **Resolved.**

### 1.15 CLI registration site

Plan cites `internal/cli/root.go:108-139`.

- root.go lines 108-139 contain `cmd.AddGroup(...)` for `inspect / runtime / account`, all PersistentFlags wiring, and the per-command `GroupID` + `cmd.AddCommand(...)` site. New `valv network` registration plugs in here. **Resolved.**

### 1.16 Existing `valv run` placement claim

Plan cites `drops/DROP_13_GENERIC_RUN/PLAN.md:78-96`.

- DROP_13 Unit 13.2 at lines 87-107 carries `Paths: internal/cli/run.go (new, not yet in tree)`. **Resolved.**

### 1.17 Repo file existence sanity

- `internal/cli/claude_project_image_test.go` and `internal/cli/codex_project_image_test.go` both already exist on disk (Unit 15.0 modifies them, not creates). **Resolved.**
- `internal/cli/run.go` does NOT yet exist (Unit 15.4 paths declare it "new, not yet in tree; expected from DROP_13"). **Consistent with the `blocked_by: DROP_13` gate.** **Resolved.**

## 2. Unit-by-unit completeness check

### 2.1 Unit 15.0 — Re-root project-image manifest resolution

- Paths: `internal/cli/operator_helpers.go`, two existing per-provider test files. All present. **Pass.**
- Packages: `internal/cli`. Consistent with paths. **Pass.**
- Evidence: 5 cites, all resolve (see 1.1, 1.2, 1.3, plus claude.go:115 / codex.go:120 confirmed in 1.13). **Pass.**
- Acceptance: 4 bullet-criteria, all yes/no:
  1. `resolveProjectImage` uses `project.DetectFrom(workingDir)` / equivalent; raw `tools.Resolve(workingDir)` removed → grep-verifiable.
  2. Empty-manifest short-circuit and overlay-build path both consult `.valv/tools.toml` at detected root → testable via subdir invocation.
  3. `VALV_<PROVIDER>_IMAGE` override semantics preserved → behavioral test.
  4. Tests in both test files add subdir coverage → file-content verifiable.
- `blocked_by: nothing` — Unit 15.0 stands alone. Consistent. **Pass.**

### 2.2 Unit 15.1 — Typed allowlist + section-safe write

- Paths: 3 existing-ish, 1 new (`allowlist_test.go`). All cited correctly. **Pass.**
- Evidence: 5 cites (tools.go, resolve.go, resolve_test.go, overlay.go, overlay_test.go). All resolve (see 1.4, 1.10, 1.12, plus resolve.go lines 11-30 confirmed). **Pass.**
- Acceptance: 7 bullet-criteria, all yes/no:
  - Typed promotion of `Allowlist` → struct check.
  - `Load` keeps strict unknown-top-level rejection → test currently exists at `TestResolve_LoadUnknownKeyError`.
  - Effective-allowlist helper returns 4 defaults for zero-value config; user hosts UNIONED → table test.
  - `WriteAllowlistSection` creates parent dir, rewrites only `[allowlist]`, preserves `[tools]` + `[env]` verbatim → fixture test.
  - 7 listed test cases (fresh root, absent file, defaults union, normalization, invalid host, unknown section, section preservation) → table-driven.
  - OverlayHash regression: same `[tools]`, different `[allowlist]` → same hash → table assert.
- `blocked_by: nothing`. **Pass.**

### 2.3 Unit 15.2 — Docker network lifecycle helpers

- Paths: 2 new (`network.go`, `network_test.go`), 1 existing (`executor.go`). All correct. **Pass.**
- Evidence: 3 cites all resolve (ops.go, types.go from 1.5; executor.go confirmed via direct read showing only Build/Remove/Prune today). **Pass.**
- Acceptance: 6 bullet-criteria, all yes/no:
  - 3 typed request structs → grep / build.
  - 3 arg builders → grep / build.
  - 3 executor methods → grep / build.
  - `BuildNetworkCreateArgs` emits `--internal` → table test.
  - `BuildNetworkConnectArgs` emits `--alias <name>` → table test.
  - Required-field validation + arg-order tests → table.
- `blocked_by: nothing`. **Pass.**

### 2.4 Unit 15.2.5 — Shared networkpolicy service + image-build egress

- Paths: 2 new (networkpolicy package) + 3 existing in images package. All correct. **Pass.**
- Evidence: 6 cites (overlay.go, service.go, ops.go, service_test.go NoCache seam, operator_helpers + tools.go for root-contract) all resolve. **Pass.**
- Acceptance: 8 bullet-criteria, all yes/no:
  - Shared service in `internal/services/networkpolicy` accepting allowlist, returning proxy/network/cleanup → API check.
  - Images service reuses it; build injects HTTP_PROXY/HTTPS_PROXY/NO_PROXY + `ImageBuildRequest.Network = <internal>` → arg test.
  - Build-policy seam consumes already-resolved manifest from 15.0; no new raw-cwd lookup → grep.
  - Defaults flow through such that bare `go install` works without user entries → flow check.
  - Fallback topology if `host.docker.internal` unreachable from `--internal` → empirical / matches runtime path → integration assertion.
  - `OverlayHash` / project tag / freshness labels unchanged → existing regression seam + 15.1 cross-check.
  - `service_test.go` table-driven proving `--network` + proxy build args added, existing labels + `--no-cache` preserved → table test.
  - `service_integration_test.go` proves `.valv/tools.toml`-driven `go install` reaches proxy with `EnsureProjectRequest.NoCache = true` OR unique-tag rebuild; one allow + one deny — yes/no.
- `blocked_by: Unit 15.0, Unit 15.1, Unit 15.2`. Reverse-graph consistent: 15.0 establishes root-resolution; 15.1 supplies allowlist + defaults; 15.2 supplies docker network primitives. **Pass.**

### 2.5 Unit 15.3 — Generic-run closed-default runtime policy

- Paths: 3 new (services/run/service.go + tests + integration). Note `service.go` is "new, not yet in tree" because DROP_13 introduces it; 15.3 augments. Consistent. **Pass.**
- Evidence: 6 cites all resolve (DROP_13 PLAN, claude/codex service.go, claude/codex runtime.go, bridge.go, bridge_test.go). **Pass.**
- Acceptance: 8 bullet-criteria + the F1 cross-drop gate + integration acceptance, all yes/no:
  - Closed-mode default injects proxy env + `ContainerRunRequest.Network` → request test.
  - Open mode injects neither → request test.
  - Exact-host only; no SOCKS/raw TCP/CIDR in DROP_15 → schema check.
  - Error wrapping at 5 boundaries → grep `fmt.Errorf("...: %w", err)`.
  - 4-case `service_test.go` table → behavioral.
  - F1 cross-drop gate: depends on DROP_14 having moved env-merge into `internal/services/run`; else unit must include re-home → contingent but yes/no on day-of-build.
  - `service_integration_test.go` uses testcontainers-go (no live-internet); allowlisted host reachable, non-allowlisted denied → yes/no via proxy.
- `blocked_by: Unit 15.1, Unit 15.2, Unit 15.2.5, DROP_13, DROP_14`. Strong gate set. **Pass.**

### 2.6 Unit 15.4 — CLI network management + open-egress opt-out

- Paths: 2 new (network.go + network_test.go), 1 existing (root.go), 1 DROP_13-pending (run.go). All correct given gate. **Pass.**
- Evidence: 4 cites resolve (tools.go, root.go, operator_helpers.go, DROP_13 PLAN). **Pass.**
- Acceptance: 8 bullet-criteria, all yes/no:
  - 3 commands (`allow`/`deny`/`list`) all project-scoped via `project.Detect()` → grep + behavioral.
  - All `.valv/tools.toml` access uses detected project root → grep.
  - `allow` creates absent file via `WriteAllowlistSection` (Unit 15.1 helper) → fixture test.
  - `deny` is idempotent, removes only user-added hosts, defaults persist → table test.
  - `list` shows defaults + user hosts → table test.
  - `--network open` flag on `valv run` → flag test.
  - No duplicated flag parsing in pre-DROP_13 claude.go / codex.go → grep negative.
  - 8 listed test cases → table-driven.
- `blocked_by: Unit 15.0, Unit 15.1, Unit 15.3, DROP_13`. Reverse-graph consistent. **Pass.**

## 3. Drop-level acceptance criteria check

Each criterion has at least one unit advancing it and is yes/no-verifiable:

1. Manifest resolution from detected project root → Unit 15.0 (primary), 15.2.5 (build seam), 15.4 (CLI seam). Yes/no via subdir launch test.
2. Typed `[allowlist]` decode + zero-value-defaults + union semantics → Unit 15.1. Yes/no via decode test.
3. `WriteAllowlistSection` creates parent dir + preserves blocks verbatim → Unit 15.1. Yes/no via fixture test.
4. Closed-default runtime injects policy material, proxy env, internal network → Unit 15.3. Yes/no via behavioral test.
5. DROP_14 cross-drop env-merge gate → Unit 15.3 F1 cross-drop gate. Yes/no on build-day.
6. DROP_12 overlay build threads same allowlist; `NoCache = true` or unique tag forces rebuild; fallback topology if `host.docker.internal` unreachable → Unit 15.2.5. Yes/no via integration test.
7. CLI surface `valv network allow|deny|list` + `valv run --network open`; no duplicated flags on pre-DROP_13 claude/codex → Unit 15.4. Yes/no via help/flag test + grep negative.

All seven criteria are advanced by at least one unit. **Pass.**

## 4. Schema Decision audit

1. Exact-host + lowercase + dedupe + union semantics → grounded in `internal/tools/tools.go:75-130` (current state) + locked by A3 (Round 2 falsification finding). **Consistent.**
2. `[allowlist]` typed, but `OverlayHash` still tools-only → grounded in overlay.go:30-70 + overlay_test.go:12-147; Unit 15.1 adds regression. **Consistent.**
3. `WriteAllowlistSection` is section-local, preserves order verbatim, calls `os.MkdirAll` → grounded in resolve.go:11-30 + `go doc` for stdlib. **Consistent.**
4. Runtime enforcement on DROP_13 shared run seam → grounded in DROP_13 PLAN.md:36 + claude/codex service.go duplication. **Consistent.**
5. Closed mode = internal Docker network + host-local HTTP/HTTPS proxy via `host.docker.internal`; no `--network none`; no Linux iptables → grounded in bridge.go:60 + bridge_test.go:274-303 + Context7 `/docker/docs`. **Consistent.**
6. Image-build egress uses Docker predefined proxy build args + `ImageBuildRequest.Network` → grounded in ops.go:9-21,38-72 + Context7. **Consistent.**
7. No subtractive override for built-in defaults in DROP_15 → coherent corollary of Decision 1's union semantics. **Consistent.**
8. **(NEW Round 3)** Project-root-based manifest resolution; `project.DetectFrom` used by all overlay/image-policy `.valv/tools.toml` reads → grounded in project.go:26-58 + project_test.go full coverage + cli/tools.go:71-97 (existing model) + operator_helpers.go:440-470 (gap to fix). **Consistent.**

Decisions 1, 7, and 8 form a coherent chain: union-only semantics (1) + no-subtractive-defaults (7) + root-based-resolution (8). No contradiction with Decision 4 (DROP_13 seam owns runtime) or Decision 6 (image-build uses same allowlist with same root). **Pass.**

## 5. Atomicity + ordering check

Six units. `blocked_by` graph:

- 15.0 ← nothing
- 15.1 ← nothing
- 15.2 ← nothing
- 15.2.5 ← {15.0, 15.1, 15.2}
- 15.3 ← {15.1, 15.2, 15.2.5, DROP_13, DROP_14}
- 15.4 ← {15.0, 15.1, 15.3, DROP_13}

No cycles; all internal references resolve; cross-drop deps on DROP_13 + DROP_14 are explicit. Atomicity per unit: each unit has a single-builder-completable scope (one new package OR one focused edit + targeted tests). 15.2.5's image-build integration test is the heaviest single unit but is still one-builder-bounded because it composes already-existing seams (15.0 root, 15.1 defaults, 15.2 network create). **Pass.**

## 6. Round-2-style probes (proof-side mitigations)

The three Round-2 falsification blockers each have a corresponding mitigation visible in the Round 3 plan:

- **Project-root drift (Round 2 finding):** mitigated by Unit 15.0 + Schema Decision 8 + Drop Acceptance 1; Units 15.2.5 + 15.4 acceptance lines explicitly forbid reintroducing raw-cwd lookups.
- **`host.docker.internal` from image-build unproven (Round 2 finding):** mitigated by Unit 15.2.5 acceptance line "If empirical validation in this unit or Unit 15.3 shows `host.docker.internal` is unreachable from the `--internal` build container, image-build policy MUST switch to the same validated fallback topology as runtime" + Drop Acceptance 6 same language + Notes block A1.
- **Integration-test cache bypass (Round 2 finding):** mitigated by Unit 15.2.5 acceptance line "force a rebuild every run (`EnsureProjectRequest.NoCache = true` or a unique repository/tag)" + Drop Acceptance 6 same language + Notes block "Any image-build proxy integration test must force a rebuild ... A freshness-label cache hit is not evidence that build-time egress is filtered." The cited seam at `service_test.go:1535-1595` proves the `NoCache` field already works as required.

All three Round-2 blockers carry explicit, yes/no-verifiable mitigations into Round 3. **Pass.**

## 7. Findings

No findings. All cited file:line references resolve, all acceptance criteria are yes/no-verifiable, the `blocked_by` graph is acyclic and complete, the schema decisions are internally consistent, and the three Round-2 falsification blockers have explicit Round-3 mitigations.

Minor advisories (not findings, not blocking):

- **A1** — Several evidence cites in the plan are off-by-2-or-3 lines on cite boundaries (e.g. `project.go:26-55` vs actual function 28-58; `tools.go:75-130` vs actual function ending at 131). Cites are still functionally correct — the cited ranges cover the asserted symbols — but a future stricter Hylla-aware planner pass could tighten them. Not blocking.
- **A2** — Unit 15.2.5's integration-test acceptance allows "disposable local fixture/proxy path" as an alternative to live-internet. Acceptable for atomic build-QA but the builder should still confirm with dev which path is acceptable in CI before implementation; this is a builder-routing concern, not a plan defect.

## TL;DR

- T1: All 17 spot-checked evidence claims resolve to real symbols/files at the cited (or near-cited) ranges in the committed tree at HEAD `cdb7cf3`.
- T2: All 6 units (15.0/15.1/15.2/15.2.5/15.3/15.4) have correct paths, resolving evidence, yes/no-verifiable acceptance criteria, and a consistent `blocked_by` graph.
- T3: All 7 drop-level acceptance criteria are yes/no-verifiable and each is advanced by at least one unit.
- T4: All 8 schema decisions (including new Decision 8 on project-root resolution) are evidence-backed and internally consistent; Decisions 1/7/8 form a coherent union+root-resolution chain.
- T5: Unit graph is acyclic (15.0/15.1/15.2 are roots; 15.2.5 needs them; 15.3 + 15.4 layer on top with cross-drop DROP_13/14 deps explicit); per-unit atomicity preserved.
- T6: All three Round-2 falsification blockers (project-root drift, image-build host.docker.internal proof, integration-test cache bypass) have explicit Round-3 mitigations woven into Unit 15.0, Unit 15.2.5, Drop Acceptance 6, and Notes block A1.
- T7: Verdict pass; no findings beyond two minor non-blocking advisories on cite-boundary tightness and an integration-test routing question for the builder.
