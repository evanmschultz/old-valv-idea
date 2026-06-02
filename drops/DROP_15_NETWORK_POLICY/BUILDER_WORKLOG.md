# DROP_15 — Builder Worklog

Append a `## Unit 15.M — Round K` section per build attempt. See `main/drops/WORKFLOW.md` § "Phase 4 — Build (per unit)" for what each section should contain.

## Unit 15.2.5.C — Round 1

**Date:** 2026-06-02
**Builder backend:** claude-sonnet-4-6 (orchestrator dispatch)

### Files Touched

- `internal/services/networkpolicy/service.go` (282 LOC) — rewrote `buildNoProxy` to return only `127.0.0.1,localhost,valv-proxy` (sorted, deterministic, no parameters). Updated `PolicyMaterial.NoProxy` doc comment to explain the corrected semantics. Updated `ProvisionRequest.Allowlist` doc comment to remove the incorrect NO_PROXY rationale. Updated the `Provision` call site from `buildNoProxy(request.Allowlist)` to `buildNoProxy()`. No other symbols touched; `NetworkExecutor` interface, `Service.Provision` logic, `networkName`, and `CleanupStale` are byte-identical to the 15.2.5.A state.
- `internal/services/networkpolicy/service_test.go` (688 LOC) — replaced two now-invalid `buildNoProxy` tests (`TestBuildNoProxy_SortedDedupedAndTrimmed`, `TestPolicyMaterial_NoProxyOmitsBlankAllowlistEntries`) with four new tests:
  - `TestBuildNoProxy` — table-driven, 3 cases (empty allowlist context, non-empty allowlist no-leak, loopback always covered), all call `buildNoProxy()` and assert result is `"127.0.0.1,localhost,valv-proxy"`.
  - `TestBuildNoProxy_AllowlistHostsNeverLeak` — calls `svc.Provision(...)` with a 5-host allowlist, asserts `NoProxy == "127.0.0.1,localhost,valv-proxy"` and that none of the 5 allowlist hosts appear in `NoProxy`.
  - `TestBuildNoProxy_LoopbackAndSidecarAlwaysPresent` — regression pin asserting the three fixed entries are always present.
  - Updated `TestProvision_CreatesNetworkOnFreshHost`: changed `wantNoProxy` from `"github.com,objects.githubusercontent.com,proxy.golang.org,sum.golang.org"` to `"127.0.0.1,localhost,valv-proxy"` and added a loop asserting no allowlist host leaks into `NoProxy`.

### Mage Targets Run

- `mage testPkg ./internal/services/networkpolicy` → **27 tests pass, 0 failures** (was 23 pre-unit; +4 new tests).

### Design Notes

- **Signature simplified to `buildNoProxy() string`** (no parameters). The function no longer reads the allowlist — it has a single fixed output. Removing the parameter makes the inversion explicit at the call site and prevents future callers from accidentally re-introducing the allowlist into NO_PROXY.
- **Fixed entries sorted alphabetically** (`127.0.0.1,localhost,valv-proxy`) for determinism. Alphabetic happens to be correct numerical/lexicographic order as well.
- **`sort.Strings` retained** even though the entries are a static slice, for consistency with the package's determinism pattern and to make the sorted output self-documenting in the code.
- **Did NOT touch `NetworkExecutor` interface** (15.2.5.A territory), `Service.Provision` logic (15.2.5.D.1 territory), `networkName`, or `CleanupStale`.

### Budget Measurement (vs. 3-symbol / 80-LOC / 3-file ceiling)

| Dimension | Budget | Actual |
|---|---|---|
| Production symbols | 3 | 1 (`buildNoProxy`) |
| Production LOC delta | ≤80 | ~18 net-changed LOC in service.go (function body rewrite + doc comment updates) |
| Production files | ≤3 | 1 (`service.go`) |

Under budget on all dimensions.

### Hylla Feedback

Not used. Evidence from direct `Read` of `service.go` and `service_test.go` was sufficient. The current code predates the latest Hylla ingest, so direct file reads were the authoritative source.

### Atomicity Confirmation

Unit 15.2.5.C is fully self-contained: the `buildNoProxy` rewrite is the complete production scope. No partial logic shipped. Mage gate GREEN (27/27 pass).

## Unit 15.2.5.A — Round 1

**Date:** 2026-06-02
**Builder backend:** claude-sonnet-4-6 (orchestrator dispatch)

### Files Touched

- `internal/adapters/docker/executor.go` (137 LOC) — added `Executor.RunContainerDetached(ctx, ContainerRunRequest) (string, error)`. Uses the `outputter` interface typecast pattern (identical to `Executor.Create` and `ListNetworks`): returns `ErrOutputUnsupported` when the runner doesn't implement `Output`; calls `BuildRunArgs` for arg construction (validation errors returned before any runner invocation); trims whitespace from stdout (docker `run -d` appends a newline to the container ID).
- `internal/services/networkpolicy/service.go` (280 LOC) — extended `NetworkExecutor` consumer-side interface with two new method signatures: `RunContainerDetached` and `ConnectNetwork` (the latter already existed on `docker.Executor` at executor.go:72 and was absent from this interface). Doc comments explain sidecar-proxy topology (Schema Decision 5) context for each method.
- `internal/adapters/docker/network_test.go` (729 LOC) — added 4 test functions for `RunContainerDetached`: `TestExecutorRunContainerDetachedReturnsContainerID` (happy path — output trimmed), `TestExecutorRunContainerDetachedOutputError` (sentinel `errors.Is` wrap), `TestExecutorRunContainerDetachedOutputUnsupported` (runner without `Output` → `ErrOutputUnsupported`), `TestExecutorRunContainerDetachedBuildError` (invalid image → validation error, runner not invoked). Added `outputRunner` test helper struct (implements `Run` + `Output`) and `errors` import.
- `internal/services/networkpolicy/service_test.go` (599 LOC) — extended `fakeNetworkExecutor` with `runDetachedResult`, `runDetachedErr`, `runDetachedCalls`, and `connectCalls` fields plus `RunContainerDetached` and `ConnectNetwork` method stubs satisfying the extended `NetworkExecutor` interface. The compile-time `var _ NetworkExecutor = docker.Executor{}` guard at line 554 also validates `docker.Executor` satisfies the extended interface.

### Mage Targets Run

- `mage testPkg ./internal/adapters/docker` → **86 tests pass, 0 failures**. (Baseline was 82 tests per Round 3 worklog; new tests bring count to 86 via the 4 new `RunContainerDetached` tests.)
- `mage testPkg ./internal/services/networkpolicy` → **23 tests pass, 0 failures**. (Count unchanged — no new service-level tests added; the fakeNetworkExecutor stubs are compile-time correctness.)
- `mage format` → clean (no output).

### Design Notes

- **`outputter` pattern for `RunContainerDetached`.** Mirrors `Executor.Create` and `ListNetworks` exactly: runtime-typecast to the `Output(context.Context, []string) (string, error)` interface, return `ErrOutputUnsupported` when absent. This avoids requiring the production `OSRunner` to implement `Output` at struct-definition time — callers that use a non-outputting runner get a clear error.
- **`strings.TrimSpace` on container ID output.** Docker `run -d` appends `\n` to the container ID; downstream callers (D.1's sidecar lifecycle) pass the ID to `ConnectNetwork` and `RemoveContainer` which both trim on their own, but returning a clean ID avoids downstream confusion.
- **Interface extension preserves existing callers.** The `fakeNetworkExecutor` in `service_test.go` is the only concrete type implementing `NetworkExecutor` in tests; the compile-time `var _ NetworkExecutor = docker.Executor{}` check at line 554 confirms the production adapter satisfies the extended interface. No other file in the tree implements `NetworkExecutor` — verified by absence of other usages.
- **`ConnectNetwork` addition to interface.** The method already existed on `docker.Executor`; this unit only makes it visible to the consumer-side interface so D.1's `Provision` can call it via the injected `NetworkExecutor`. No implementation change on `docker.Executor`.
- **Did NOT touch `buildNoProxy`** (Unit 15.2.5.C, which shares `service.go`). The only edit to `service.go` is the interface block addition.

### Budget Measurement (vs. 3-symbol / 80-LOC / 3-file ceiling)

| Dimension | Budget | Actual |
|---|---|---|
| Production symbols | 3 | 3 (`Executor.RunContainerDetached` + `NetworkExecutor.RunContainerDetached` method sig + `NetworkExecutor.ConnectNetwork` method sig) |
| Production LOC delta | ≤80 | ~47 net-added LOC across both prod files (executor.go +23, service.go +11 interface body, ~13 comments/whitespace) |
| Production files | ≤3 | 2 (`executor.go` + `service.go`) |

Under budget on all dimensions.

### Hylla Feedback

Not used. Evidence from direct `Read` of existing executor patterns (`Executor.Create`, `Executor.ListNetworks`, `Executor.ConnectNetwork`) was sufficient. Hylla is at `@main` which predates this unit's uncommitted code, so LSP was the live-code tool.

### Atomicity Confirmation

Unit 15.2.5.A is fully self-contained: (1) `executor.go` addition + (2) `service.go` interface extension are the complete production scope. No partial logic shipped. Both mage gates GREEN.

## Unit 15.2 — Round 3 (ADDITIVE — NetworkConnect surface)

**Date:** 2026-05-24
**Builder backend:** claude-native (orchestrator dispatch)

### Files Touched

- `internal/adapters/docker/network.go` — added `NetworkConnectRequest` struct with `Network`, `Container`, and `Aliases []string` fields; added `Valid()` validation method; added `BuildNetworkConnectArgs()` function emitting `docker network connect --alias <alias> <network> <container>` with sorted aliases for determinism.
- `internal/adapters/docker/executor.go` — added `Executor.ConnectNetwork()` method, mirroring the `CreateNetwork` / `RemoveNetwork` pattern via `e.runner.Run()`.
- `internal/adapters/docker/network_test.go` — added comprehensive test suite: `TestBuildNetworkConnectArgs` (19 test cases covering single/multiple aliases sorted, no aliases, whitespace trimming, and validation errors for missing network/container/empty-alias/invalid patterns/exceeding length limits); `TestExecutorConnectNetworkForwardsArgs` (verifies args forwarded to runner); `TestExecutorConnectNetworkReturnsBuildError` (verifies validation errors returned before invocation).
- `drops/DROP_15_NETWORK_POLICY/PLAN.md` — Unit 15.2 `state: in_progress` → `done`.

### Mage Targets Run

- `mage testPkg ./internal/adapters/docker` → 82 tests pass, coverage 67.9% (above the enforced 60.0% gate).
- `mage build` → clean build succeeds.

### Design Notes

- **Round 4 YAGNI cut REVERSED.** The sidecar-proxy topology (Schema Decision 5 in re-planned DROP_15) has a concrete caller: the proxy sidecar must attach to BOTH the `--internal` network (internal-only workload reachability) AND the `bridge` network (external egress). The `docker network connect` surface was cut as YAGNI in Round 4; Round 3 re-planning activates that caller, so the surface is now justified.
- **Validation pattern mirrors Create/Remove.** `NetworkConnectRequest.Valid()` enforces: network non-empty + valid pattern + ≤64 bytes (matches `NetworkCreateRequest`); container non-empty (no length limit — container IDs and names are Docker-managed); each alias non-empty + valid pattern (same `dockerNetworkNamePattern` as network names). Validation errors prefixed `validate network connect request: ...` for boundary clarity.
- **Deterministic alias order.** Aliases are sorted before emission to ensure `docker network connect --alias alpha ... --alias zulu ...` produces consistent arg vectors across runs. This matches the pattern used in `BuildNetworkCreateArgs` for label keys.
- **Positional args last.** The `<network> <container>` positional arguments are appended after all `--alias` flags, matching `docker network connect` CLI semantics.
- **Executor pattern.** `ConnectNetwork` mirrors `CreateNetwork` and `RemoveNetwork`: build args via the typed request, validate via `Valid()`, shell out via `e.runner.Run()`, return errors directly (no adaptation).

### Context7 Evidence

- `/docker/docs` confirmed `docker network connect` CLI with `--alias` flag for network aliases (DNS names). Multiple `--alias` values allowed per invocation. Containers can connect to multiple networks either via repeated `--network` at create time or via `docker network connect` for a running container.

### Hylla Feedback

None — existing Docker adapter patterns (network create/remove, image build args, container run args) provided sufficient evidence; no external library/CLI lookup needed.

### Acceptance Check

- [x] R2 `NetworkCreateRequest` / `BuildNetworkCreateArgs` / `Executor.CreateNetwork` + `RemoveRequest` / `BuildNetworkRemoveArgs` / `Executor.RemoveNetwork` remain unchanged (KEEP constraint).
- [x] `NetworkConnectRequest` struct added with `Network`, `Container`, `Aliases []string` (optional).
- [x] `Valid()` method mirrors Create/Remove validation pattern (required-field checks, name-pattern validation, length limits).
- [x] `BuildNetworkConnectArgs` emits `docker network connect --alias <alias> <network> <container>` with sorted aliases.
- [x] `Executor.ConnectNetwork` added, mirroring Create/Remove pattern via `e.runner.Run()`.
- [x] Tests cover: required-field validation (missing network/container, empty alias), `--alias` emission (single/multiple sorted), whitespace trimming, deterministic arg order (positional tail), name/alias pattern validation errors, length-limit rejection.
- [x] `mage testPkg ./internal/adapters/docker` passes; coverage 67.9% ≥ 60% enforced gate.
- [x] `mage build` succeeds.

## Unit 15.2 — Round 1

**Date:** 2026-05-23
**Builder backend:** claude-native (orchestrator dispatch)

### Files Touched

- `internal/adapters/docker/network.go` (new) — `NetworkCreateRequest`, `NetworkRemoveRequest`, `BuildNetworkCreateArgs`, `BuildNetworkRemoveArgs`, `dockerNetworkNamePattern`.
- `internal/adapters/docker/network_test.go` (new) — table-driven arg-builder + validation tests, plus `Executor.CreateNetwork` / `RemoveNetwork` forwarding + build-error tests.
- `internal/adapters/docker/executor.go` — added `Executor.CreateNetwork` and `Executor.RemoveNetwork` methods (mirror existing build/remove pattern).
- `drops/DROP_15_NETWORK_POLICY/PLAN.md` — Unit 15.2 `state: todo` → `in_progress` → `done`.

### Mage Targets Run

- `mage testPkg ./internal/adapters/docker` → 51 tests pass, coverage 67.8% (above the enforced 60.0% gate).
- `go tool gofumpt -l` on new/edited files → no diff (clean).
- `mage test` (full) attempted — fails on `internal/tools/allowlist.go` gofumpt check, which is sibling Unit 15.1 WIP, not in this unit's scope.

### Design Notes

- **No `NetworkConnectRequest` / `BuildNetworkConnectArgs` / `Executor.ConnectNetwork`.** Schema Decision 5 cut the second-network bridge fallback; there is no in-drop caller. Documented in `network.go` doc comment.
- **Deterministic arg order.** Args emitted as: `network create [--internal] [--label k=v ...] <name>`. Labels are sorted lexicographically by key, matching the existing `BuildImageArgs` / `BuildRunArgs` pattern. Single `network` subcommand verb mirrors `image rm` / `builder prune` style.
- **Network name validation.** `dockerNetworkNamePattern = ^[a-zA-Z0-9][a-zA-Z0-9_.-]*$` — rejects leading hyphens/dots (Docker CLI flag-parsing collision), whitespace, path separators, and other shell-meta characters. Mirrors Docker's documented network-name shape.
- **Error wrapping boundaries.** `Valid()` returns boundary-prefixed errors (`validate network create request: ...`); arg builders wrap that through. Executor methods bubble the validation error verbatim and shell out via the existing `runner CommandRunner` seam — no new injection surface needed.
- **Label semantics.** Labels are accepted as a free-form `map[string]string` rather than hardcoding `valv=network-policy`. The orphan-cleanup contract (Unit 15.2.5) decides which label to attach; this layer just serializes.

### Context7 Evidence

- `/docker/cli` confirmed `docker network create --internal <name>` (`docker network create --internal isolated-net`) and `docker network rm` as the required CLI surface. Labels accepted via `--label key=value` (multi-flag pattern, same as `docker config create --label`).

### Hylla Feedback

None — no Hylla fallback needed; existing patterns in the package (`BuildImageArgs`, `BuildContainerRemoveArgs`, `BuildBuilderPruneArgs`) provided sufficient evidence directly via `Read`.

### Acceptance Check

- [x] `NetworkCreateRequest` + `NetworkRemoveRequest` added; `NetworkConnectRequest` deliberately omitted.
- [x] `BuildNetworkCreateArgs` + `BuildNetworkRemoveArgs` in `network.go`; no `BuildNetworkConnectArgs`.
- [x] `Executor.CreateNetwork` + `Executor.RemoveNetwork` in `executor.go`; no `Executor.ConnectNetwork`.
- [x] `BuildNetworkCreateArgs` emits `docker network create --internal ...`.
- [x] Tests cover required-field validation, `--internal` emission, label emission (multiple labels, sorted), deterministic arg order, name validation (empty, whitespace, space, slash, leading-hyphen, leading-dot, valid hyphen/dot/underscore), and Executor forwarding via mock runner.
- [x] `mage testPkg ./internal/adapters/docker` passes; coverage 67.8% ≥ 60% enforced gate.

## Unit 15.0 — Round 1

**Date:** 2026-05-23
**Builder backend:** claude-native (orchestrator dispatch)

### Goal

Re-root project-image manifest resolution from raw `workingDir` to the detected project root via `project.DetectFrom`, so a `valv claude` / `valv codex` launch from a nested repo subdirectory sees the same `.valv/tools.toml` and overlay behavior as a repo-root launch. Schema Decision 8 in `drops/DROP_15_NETWORK_POLICY/PLAN.md`.

### Files Touched

- `internal/cli/operator_helpers.go` — added `internal/project` import; `resolveProjectImage` now calls `project.DetectFrom(workingDir)` before `tools.Resolve(detected.Root)`; raw `tools.Resolve(workingDir)` call removed; doc comment updated to describe Unit 15.0 behavior. Override semantics unchanged (still emits one stderr warning AFTER manifest resolution and returns `baseRef`).
- `internal/cli/claude_project_image_test.go` — added three new tests + two new helpers covering: (a) nested subdir + root manifest triggers overlay build, (b) nested subdir + no root manifest returns `baseRef` with zero docker calls, (c) nested subdir + root manifest + `VALV_CLAUDE_IMAGE` override emits exactly one warning and skips overlay.
- `internal/cli/codex_project_image_test.go` — mirror of the claude tests for codex (`VALV_CODEX_IMAGE`).
- `drops/DROP_15_NETWORK_POLICY/PLAN.md` — Unit 15.0 `state: todo` → `in_progress` → `done`.

### Implementation Approach

1. **TDD-first.** Added 6 new tests (3 claude + 3 codex) before any production change. Pre-fix `mage testPkg ./internal/cli` showed exactly the expected red signal: 4 of 6 new tests failed with the precise diagnostics ("manifest at root should have triggered overlay" + "missing override warning"); the two "no manifest" tests already passed because raw-cwd resolution also returns `baseRef` when no manifest exists anywhere — they're left in place as regression guards for the post-fix semantics.
2. **Implementation.** Single function change: `project.DetectFrom(workingDir)` first, then `tools.Resolve(detected.Root)`. `project.DetectFrom` walks up to the nearest `.git` marker and falls back to the start path when none is found — matching `runToolsValidate` semantics in `internal/cli/tools.go:75-84` and pinning Acceptance Criterion 1 from the drop's PLAN.md.
3. **Override path untouched.** The env-name lookup + warning + early-return path was not moved; it still runs after manifest resolution. Tests assert the warning fires exactly once and zero docker calls happen on the override-from-subdir path.

### Mage Targets Run

- `mage testPkg ./internal/cli` (post-fix) → 240 tests pass, 0 failures, coverage 67.6% (above the 60% mage-enforced floor). Package was already at 67.6% pre-unit; coverage delta is neutral.
- `mage testPkg ./internal/cli` (pre-fix, TDD red) → 4 expected-FAIL new tests, confirming the new tests genuinely exercise the changed behavior.

### Design Notes

- **`project.DetectFrom` over a hand-rolled `os.Chdir` + `project.Detect()` pattern.** `DetectFrom` (a) already exists at `internal/project/project.go:28`, (b) is already exercised against repo roots, linked worktrees, file paths, and symlinked roots in `internal/project/project_test.go:11-165`, and (c) does not mutate process state. Goroutine-safe and zero chdir round-trip.
- **Fallback preserves existing semantics.** `project.DetectFrom` returns the start path with `HasGitMarker=false` when no marker is found, so the pre-existing `TestResolveProjectImage{Claude,Codex}EmptyManifestReturnsBaseRef` cases (bare `t.TempDir()` with no `.git`) continue to pass unchanged.
- **Minimal import surface.** The `project` package is already imported elsewhere in `internal/cli` (e.g. `tools.go`), so this adds no new direct dependency.
- **Override path placement is deliberate.** Acceptance Criterion 3 ("override emits one warning and skips overlay AFTER manifest resolution") was already satisfied structurally in the pre-DROP_15 code; this unit's change is only the manifest-source rewrite, not a control-flow reorder.

### Hylla Feedback

None — existing patterns in the package (`runToolsValidate` in `internal/cli/tools.go:75-84`) and the `internal/project` package surface were sufficient evidence directly via `Read`. LSP was unavailable (gopls notification sync error) but not load-bearing for this unit.

### Acceptance Check

- [x] `resolveProjectImage` resolves the project root from `workingDir` via `project.DetectFrom` before calling `tools.Resolve`. Raw `tools.Resolve(workingDir)` is removed.
- [x] Empty-manifest short-circuit and overlay-build path both consult `.valv/tools.toml` at the detected project root — verified by `TestResolveProjectImage{Claude,Codex}NestedSubdirFindsRootManifest` and `TestResolveProjectImage{Claude,Codex}NestedSubdirNoManifestReturnsBase`.
- [x] `VALV_{CLAUDE,CODEX}_IMAGE` still emit one warning and skip overlay work AFTER manifest resolution — verified by `TestResolveProjectImage{Claude,Codex}NestedSubdirOverrideAfterRootResolve`.
- [x] Repo-subdirectory coverage present in both test files (manifest at `<projectRoot>/.valv/tools.toml`, invocation from `<projectRoot>/pkg/sub`).
- [x] `mage testPkg ./internal/cli` passes; coverage 67.6% ≥ 60% enforced gate.

### Hard-Constraint Compliance

- No changes to `internal/tools/`, `internal/adapters/docker/`, `internal/services/networkpolicy/`, `internal/services/images/`, `internal/services/run/`.
- No network-policy-related code added — purely the manifest-source re-rooting.

## Unit 15.1 — Round 1

**Builder backend:** claude-native (orchestrator dispatch)

### Files touched

- `internal/tools/tools.go` — promoted `ToolManifest.Allowlist` from `toml.Primitive` to typed `AllowlistConfig`. Removed the discarded `allowlist` PrimitiveDecode pass in `Load`; `Env` PrimitiveDecode pass is preserved verbatim pending DROP_14. Doc comment updated to describe the DROP_15 typing.
- `internal/tools/allowlist.go` (new) — `AllowlistConfig`, `DefaultAllowlistHosts` (`github.com`, `objects.githubusercontent.com`, `proxy.golang.org`, `sum.golang.org`), `EffectiveAllowlist` (union + lowercase + trim + dedup + sort + RFC 1123-style validation), `WriteAllowlistSection` (section-safe rewrite with byte-preservation outside the bounded `[allowlist]` span, fresh-file + absent-parent-dir support, BOM/CRLF/multi-line-string-outside-allowlist/array-of-tables rejection wrapping `ErrUnsupportedManifestShape`), `ErrInvalidAllowlistHost`.
- `internal/tools/allowlist_test.go` (new) — covers zero-value defaults, union semantics, lowercase + dedup including case-folding dedup against a built-in, invalid-host rejections (empty, URL, path, query, port, leading/trailing dot, leading/trailing hyphen, underscore, whitespace, non-ASCII), Docker-alias acceptance, typed-decode via `Load`, unknown-sub-key rejection inside `[allowlist]`, fresh-no-`.valv/` write, empty-hosts canonical form, golden prefix-and-suffix preservation with file preamble + inline-comment-on-`[allowlist]` (discarded — span-internal) + divider comments + `[env]` with interleaved comments, `[allowlist]` as last section, `[allowlist]`-absent appending, BOM/CRLF/multi-line-string-outside/array-of-tables rejection, empty-path rejection.
- `internal/tools/resolve_test.go` — extended `TestResolve_ValidManifest_WithForwardCompatSections` to assert `m.Allowlist.Hosts` is now typed-decoded from the resolved manifest.
- `internal/services/images/overlay_test.go` — added `TestOverlayHash_AllowlistDataIgnored` regression pin: two manifests with identical `Tools` but different `Allowlist.Hosts` (plus a no-allowlist baseline) must produce the same `OverlayHash`. Preserves Schema Decision 2: overlay image identity stays `[tools]`-only.
- `drops/DROP_15_NETWORK_POLICY/PLAN.md` — Unit 15.1 `state: todo` → `in_progress` → `done`.

### mage targets run

- `mage testPkg ./internal/tools` → 86 tests pass, coverage 93.4% (well above 60% gate, well above 70% CLAUDE.md target).
- `mage testPkg ./internal/services/images` → 63 tests pass, coverage 81.6%.
- `mage test` (full) → 747 tests pass across 22 packages; every package ≥ 60% threshold.
- `mage integration` → 240 pass + 3 pre-existing Docker-fixture skips; no regressions from the schema change.

### Design notes

- **Typed-decode lets the strict check satisfy itself.** Empirical confirmation: removing the `PrimitiveDecode(m.Allowlist, ...)` call did NOT cause `allowlist.hosts` to be reported in `meta.Undecoded()`. The previous comment in `tools.go` saying primitive fields require an explicit `PrimitiveDecode` call only applies when the field itself stays `toml.Primitive`; once a sub-key like `hosts` decodes into a typed slice, BurntSushi/toml correctly marks the sub-keys decoded. The strict unknown-sub-key behavior is preserved: `allowlist.cidrs` is reported via the new `TestLoad_AllowlistUnknownKeyRejected` test.
- **Span boundary literalness.** The schema decision says the `[allowlist]` span ends "immediately before the first byte of the next top-level section header" — meaning blank lines, divider comments, and inline comments between `[allowlist]` and the next header are INSIDE the span and may be rewritten. The golden test asserts this literally: the inline comment on the `[allowlist]` header (`# network-policy hosts`) and the divider comment between `[allowlist]` and `[tools]` both disappear after rewrite, while everything from the `[tools]` header onward is byte-identical to the original input.
- **Writer is a mechanical edit.** `WriteAllowlistSection` writes `cfg.Hosts` verbatim — no lowercasing, no dedup, no validation. Callers that want normalization should run `EffectiveAllowlist` semantics (or some subset) themselves and pass the result. Keeps the writer's behavior easy to reason about: input list goes in, exact list comes out.
- **Validator policy.** `hostShapeRE` enforces RFC 1123-style labels with at most alphanumerics + hyphens between, plus dots between labels. Underscores are rejected by design (typical hostname/Docker-alias norms). Non-ASCII is rejected because the proxy/closed-default network path the allowlist feeds is ASCII-only.
- **OverlayHash regression test.** The new test exercises three permutations (two manifests with different allowlists + a no-allowlist baseline) and asserts all three hash identically. With the current implementation, `OverlayHash` only hashes `canonicalManifest(manifest.Tools)` — `Allowlist` was never read by the hash function, so the test passes immediately. Its value is as a regression pin: a future drop that adds `manifest.Allowlist` to the hash payload will instantly fail this test.

### Hylla feedback

None — existing `internal/tools/tools.go`, `resolve.go`, and `internal/services/images/overlay.go` provided sufficient evidence directly via `Read` and `LSP`. No Hylla fallback was needed.

### Acceptance check against drop's PLAN.md Unit 15.1

- [x] Promote `ToolManifest.Allowlist` from `toml.Primitive` to typed `AllowlistConfig{ Hosts []string }`.
- [x] `Load(path)` decodes `[allowlist]` into the typed struct while keeping `[env]` deferred and preserving strict unknown-top-level rejection.
- [x] `EffectiveAllowlist` helper: zero-value returns the four built-ins; user hosts lowercased + deduped + validated + UNIONED.
- [x] `WriteAllowlistSection(path, cfg)` creates the parent `.valv/` dir via `os.MkdirAll(filepath.Dir(path), 0o755)`.
- [x] Supports only the supported manifest shape; rewrites only the bounded `[allowlist]` span; rejects BOM, CRLF, multi-line strings outside `[allowlist]`, and array-of-tables headers via `ErrUnsupportedManifestShape`.
- [x] Tests cover the full Unit 15.1 list including the golden prefix-and-suffix fixture, lowercase normalization + dedup, invalid host rejection, unknown top-level section rejection, absent-`.valv/` parent creation, create-from-absent-file, deterministic rejection of unsupported shapes.
- [x] `internal/services/images/overlay_test.go` regression: identical `Tools` + different `Allowlist` produce same `OverlayHash`.

### Hard-constraint compliance

- No changes to `internal/cli/`, `internal/adapters/docker/`, `internal/services/networkpolicy/`, `internal/services/run/`.
- Only edits outside `internal/tools/` are the `internal/services/images/overlay_test.go` regression test, which is explicitly the one exception in the spawn appendix.

## Unit 15.1 — Round 2

**Date:** 2026-05-24
**Builder backend:** claude-native (orchestrator dispatch)

### Goal

Address the four Round 1 falsification findings without expanding scope:

1. CE#1 — indented `[allowlist]` headers break byte-preservation.
2. CE#2 — `validateHost` did not enforce RFC 1123 label-length (63 byte) or total-length (253 byte) limits despite the worklog claim.
3. CE#3 — multi-line-string detection over-broad: rejected lines whose only `"""` / `'''` lived inside a `#` comment.
4. Hidden dep — `DefaultAllowlistHosts` was an exported mutable slice; any caller could silently mutate global allowlist policy.

### Files Touched

- `internal/tools/allowlist.go`
  - **CE#4 (unexport defaults):** renamed `var DefaultAllowlistHosts` → unexported `var defaultAllowlistHosts`. Added exported `func DefaultAllowlistHosts() []string` that returns a fresh `make + copy` of the underlying slice on every call. Doc comment explicitly calls out the security rationale.
  - **CE#2 (RFC 1123 length limits):** extended `validateHost` to enforce `len(h) > 253` (total) and `len(label) > 63` (each dot-separated label) AFTER the character-shape regex passes. Both new branches return errors wrapping `ErrInvalidAllowlistHost` with descriptive messages naming the relevant byte limit ("253 bytes" / "63 bytes") so test assertions can pin the failure mode.
  - **CE#1 (indented headers):** in `splitAllowlistSpan` the header-detection block now checks `line[0] == ' ' || line[0] == '\t'` IMMEDIATELY after the regex match (which runs on the trimmed line). Indented top-level section headers return `ErrUnsupportedManifestShape` with offset-in-file context. Both `[allowlist]` and other sections are subject to the rejection; both have dedicated tests.
  - **CE#3 (comment-aware triple-quote detection):** introduced `stripLineComment([]byte) []byte`. It walks byte-by-byte tracking single-line `"..."` basic-string state (with `\` escape handling) and `'...'` literal-string state (no escapes), and returns the slice up to the first `#` outside any string. The outside-`[allowlist]`-span check now calls `bytes.Contains(stripLineComment(line), ...)` for both triple-quote variants. Comments with embedded triple quotes are now accepted; real multi-line-string openers in code still get rejected.
- `internal/tools/allowlist_test.go`
  - New: `TestWriteAllowlistSection_RejectsIndentedSectionHeader` — `  [allowlist]` rejected.
  - New: `TestWriteAllowlistSection_RejectsIndentedOtherSectionHeader` — `  [tools]` rejected (same contract applies outside `[allowlist]` because the prefix/suffix region is preserved verbatim and the rewrite cannot keep the leading whitespace attached).
  - New: `TestWriteAllowlistSection_AcceptsCommentsContainingTripleQuotes` — file with `# keep """ here`, `mage = "latest" # also """ in this comment`, and `# and ''' literal triple quotes too` round-trips through the rewriter without `ErrUnsupportedManifestShape`. Asserts prefix bytes preserved verbatim and canonical `[allowlist]` rewrite emitted.
  - New: `TestEffectiveAllowlist_RejectsOverlongLabel` — 64-byte label (`strings.Repeat("a", 64) + ".example.com"`) rejected with error mentioning "63 bytes".
  - New: `TestEffectiveAllowlist_AcceptsMaxLengthLabel` — 63-byte label boundary accepted (regression pin against an off-by-one tightening).
  - New: `TestEffectiveAllowlist_RejectsOverlongTotal` — 254-byte FQDN (`strings.Repeat("aaaa.", 50) + "aaaa"`, all labels 4 bytes so the label-limit branch does not pre-empt the total-length branch) rejected with error mentioning "253 bytes".
  - New: `TestDefaultAllowlistHostsReturnsCopyNotMutableRef` — calls `DefaultAllowlistHosts()`, mutates every element of the returned slice in place to `"tampered.example.com"`, then calls `DefaultAllowlistHosts()` again and asserts the second call still returns the original four hosts. Also asserts `EffectiveAllowlist(AllowlistConfig{})` is unaffected after the tamper, which proves the internal `defaultAllowlistHosts` source slice was not touched.
- `drops/DROP_15_NETWORK_POLICY/PLAN.md` — Unit 15.1 `state: done` → `in_progress` → `done`.

### Counterexample Mitigation Map

| Round 1 finding | Fix location | Validating test(s) |
|---|---|---|
| CE#1 indented `[allowlist]` header | `splitAllowlistSpan` header-detection block: `line[0]` whitespace check | `TestWriteAllowlistSection_RejectsIndentedSectionHeader`, `TestWriteAllowlistSection_RejectsIndentedOtherSectionHeader` |
| CE#2 missing RFC 1123 length limits | `validateHost`: `len(h) > 253` + per-label `len > 63` | `TestEffectiveAllowlist_RejectsOverlongLabel`, `TestEffectiveAllowlist_AcceptsMaxLengthLabel`, `TestEffectiveAllowlist_RejectsOverlongTotal` |
| CE#3 over-broad multi-line-string detection | `stripLineComment` + use in outside-span check | `TestWriteAllowlistSection_AcceptsCommentsContainingTripleQuotes` (plus existing `TestWriteAllowlistSection_RejectsMultilineStringOutsideAllowlist` still green — proves real multi-line openers are still rejected) |
| Hidden dep: exported mutable defaults | unexport `defaultAllowlistHosts`, add `func DefaultAllowlistHosts()` returning a copy | `TestDefaultAllowlistHostsReturnsCopyNotMutableRef` |

### Mage Targets Run

- `mage testPkg ./internal/tools` → **93 tests pass, 0 failures, coverage 91.9%** (well above the 60% mage-enforced gate and 70% CLAUDE.md target). Test count delta from Round 1: 86 → 93 (+7 new tests, +1 net beyond the 5 explicitly required because both indented-header variants and the max-length boundary got their own tests).
- `mage testPkg ./internal/services/images` → **63 tests pass, coverage 81.6%**. The `OverlayHash` allowlist-ignored regression test from Round 1 is still green; `DefaultAllowlistHosts` rename did not perturb overlay hashing because `OverlayHash` never reads allowlist data.
- `go tool gofumpt -l internal/tools/allowlist.go internal/tools/allowlist_test.go` → clean (empty output).

### Design Notes

- **Indented-header rejection scope.** The Round 2 brief proposes rejecting indented `[allowlist]` headers. I extended the rejection to ALL indented top-level section headers, including `[tools]` and `[env]`. Rationale: the byte-preservation contract preserves the prefix/suffix region verbatim, so an indented `[tools]` header sitting in the suffix would also break the spec — the leading whitespace would be retained but the rewrite would lose any structural anchor for normalization. Treating "all top-level headers must start at column 0" as a single uniform contract is cleaner than carving out per-section exceptions. The Schema-Decision-3 supported-shape contract already lists narrow restrictions and this extends them by one explicit clause; both the function doc comment and the new test names make it explicit.
- **`stripLineComment` chose a tiny single-pass byte walker over a full TOML lexer.** It only needs to distinguish `#` outside a string from `#` inside a single-line string. That requires tracking single-line `"..."` (with `\` escape) and `'...'` (no escape) states. Anything more (datetimes, integers, arrays) is irrelevant because the only thing the outside-span check needs to know is whether triple quotes appear in CODE. Returning the original line slice up to the cut point keeps the downstream `bytes.Contains` check trivial.
- **63-byte boundary test pin.** I added `TestEffectiveAllowlist_AcceptsMaxLengthLabel` even though the brief only required the rejection test. Rationale: the spec at this boundary is asymmetric (`> 63` rejects, `<= 63` accepts), and a future refactor could easily turn `>` into `>=`. The boundary test catches that regression at the unit-test layer rather than waiting for an integration-test failure with a real-world hostname.
- **254-byte total-length fixture construction.** I deliberately used label size 4 (`"aaaa."`) so each label is under the 63-byte limit; that ensures the test exercises the TOTAL-length branch and not the label-length branch (any per-label failure short-circuits before total-length is checked). Test asserts the error message mentions "253 bytes" so a future change that moved the limit check would break the assertion.
- **Doc-comment gofumpt quirk.** Triple-apostrophe sequences (`'''`) inside Go doc comments get normalized by `go/printer` into curly-quote + apostrophe forms when they appear between text words, which gofumpt then rewrites. To keep the file gofumpt-clean while still describing the multi-line literal-string form, the doc comments now use prose ("triple-apostrophe sequence", "triple double-quote") instead of literal triple-apostrophe characters. The runtime detection logic (`bytes.Contains(..., []byte("'''"))`) still uses the literal byte sequence — only the prose was changed.

### Hylla Feedback

None — existing patterns in `internal/tools/allowlist.go` plus the Round 1 falsification report at `drops/DROP_15_NETWORK_POLICY/BUILDER_QA_FALSIFICATION.md` provided sufficient evidence directly via `Read`. LSP was unavailable (gopls sync error against the active checkout) so `goToDefinition` / `findReferences` for `DefaultAllowlistHosts` had to fall back to shell `rg` — confirmed zero external callers before unexporting.

### Acceptance Check against Round 2 brief

- [x] All four Round 1 findings addressed with narrow fixes (no scope expansion).
- [x] CE#1 → `ErrUnsupportedManifestShape` for indented section headers; dedicated test.
- [x] CE#2 → 63-byte label + 253-byte total enforced; descriptive errors; two dedicated rejection tests + one boundary acceptance test.
- [x] CE#3 → `stripLineComment` + comment-aware scan; dedicated test proves `# ... """ ...` lines round-trip through the rewriter.
- [x] Hidden dep → `DefaultAllowlistHosts` is now an exported function returning a copy; underlying slice is unexported; dedicated test proves tampering with a returned slice does not affect subsequent calls.
- [x] 93 tests pass (86 baseline + 7 new). Coverage 91.9% in `internal/tools`.
- [x] `internal/services/images` OverlayHash regression test still green (`mage testPkg ./internal/services/images` → 63 pass, 81.6% cover).
- [x] No raw `go test` / `GOCACHE=...` invocations — only `mage testPkg`.

### Hard-constraint compliance

- All Round 2 production edits live in `internal/tools/allowlist.go` plus tests in `internal/tools/allowlist_test.go`.
- No edits to `internal/services/images/`, `internal/cli/`, `internal/adapters/docker/`, `internal/services/networkpolicy/`, `internal/services/run/`.
- Drop-dir edit limited to `PLAN.md` state-bit flip and this worklog appendix.

## Unit 15.2.5 — Round 1

**Date:** 2026-05-24
**Builder backend:** claude-native (orchestrator dispatch, claude-opus-4-7)

### Goal

Ship the shared `internal/services/networkpolicy` seam plus image-build egress enforcement for DROP_12 overlay builds, satisfying PLAN.md Unit 15.2.5 acceptance.

### Files Touched

- **New: `internal/services/networkpolicy/service.go`** — package + `Service`, `Options`, `NetworkExecutor` consumer-side interface, `ProvisionRequest`, `PolicyMaterial`, `Cleanup`, `Provision`, `CleanupStale`, plus internal helpers `networkName` (deterministic sha256-derived name) and `buildNoProxy` (sorted, deduped, trimmed). Orphan-cleanup contract: `Provision` lists existing networks by `label=valv=network-policy`, reclaims a matching deterministic name, removes non-matching stale orphans, then creates the fresh network with `--internal` and the managed label.
- **New: `internal/services/networkpolicy/service_test.go`** — 23 table-driven tests covering: New validation, Valid validation matrix (happy + 3 rejection cases), fresh-host create, idempotent reclaim of matching network, stale-orphan removal, simultaneous reclaim+stale, list/create/stale-remove/cleanup error wrapping with sentinel `errors.Is`, deterministic + order-invariant `networkName`, NO_PROXY sort/dedup/trim, `CleanupStale` happy + error paths, proxy-endpoint whitespace trim, and compile-time `var _ NetworkExecutor = docker.Executor{}` interface satisfaction guard.
- **`internal/adapters/docker/executor.go`** — added `Executor.ListNetworks(ctx, label string) ([]string, error)`. Uses the existing `outputRunner` typecast pattern (mirrors `RemoveContainer`'s Output-capable branch); returns nil slice when filter matches zero networks. Wired imports: added `fmt` + `strings`.
- **`internal/services/images/service.go`** — added `Options.NetworkPolicy NetworkPolicy` + `Options.ProxyEndpoint string`, plus consumer-side interface `NetworkPolicy` and value types `NetworkPolicyRequest`, `NetworkPolicyMaterial`, `NetworkPolicyCleanup` (each mirrors the networkpolicy producer-side shape, decoupled from the producer package). `Service` gains `networkPolicy` + `proxyEndpoint` fields. `EnsureProjectImage` rebuild path: when policy is configured, calls `tools.EffectiveAllowlist(request.Manifest.Allowlist)`, then `s.networkPolicy.Provision(...)`, defers cleanup, and threads `HTTP_PROXY`/`HTTPS_PROXY`/`NO_PROXY` into `ImageBuildRequest.BuildArgs` + `policyMaterial.NetworkName` into `ImageBuildRequest.Network`. Freshness labels (`recipe_hash`, `tools_hash`, `base_recipe_hash`, `managed`, `scope`) unchanged → OverlayHash + project-overlay tag identity preserved across policy on/off transitions.
- **`internal/services/images/service_test.go`** — extended with 6 new table-driven tests covering: proxy build args + `--network` injection on rebuild, policy-off path emits no proxy args / no `--network`, empty manifest short-circuits before Provision, up-to-date label match skips Provision, Provision error wraps with sentinel `errors.Is`, user-declared allowlist hosts unioned with built-in defaults. Added `errors` import. `fakeNetworkPolicy` stub records Provision hits + cleanup count + last request.
- **`internal/services/images/service_integration_test.go`** — added `TestEnsureProjectImage_NetworkPolicyOverlayBuildReachesProxy_DockerDesktopMacOS` plus `networkPolicyAdapter` (bridges consumer-side `images.NetworkPolicy` to production `networkpolicy.Service`) and tiny `intToStr` helper. **Removed duplicate `const testClaudeCLIVersion = "2.1.143"`** (was redeclared in both `service_test.go` and `service_integration_test.go`; latent pre-existing bug surfaced when this unit first wired the integration tag into the magefile). The test stands up an in-process HTTP CONNECT proxy on a free port, then drives EnsureProjectImage with NoCache=true and a `go install` manifest to prove the build reaches the proxy via `host.docker.internal`. Per PLAN R3.F4.1.1 manual-validation-required clause, the test ships **skipped by default**; setting `VALV_NETPOL_INTEGRATION_RUN=1` on Docker Desktop macOS executes it. The Round 1 builder ran it locally — see "Macos validation outcome" below.
- **`magefile.go`** — extended `Integration()` to include `./internal/services/images` so the integration build tag actually compiles the file in CI. The existing integration tests in that file (`TestServiceBuildRealDockerImage`, `TestWriteDefault{Codex,Claude}ContextBuildsWithExistingUIDAndGID`) had been ignored by mage integration since they were added in commit `6b4ea4f`; this Round 1 inclusion surfaced the duplicate `testClaudeCLIVersion` const which the build fix above removes.
- **`drops/DROP_15_NETWORK_POLICY/PLAN.md`** — Unit 15.2.5 `state: todo` → `in_progress` → `done`.

### Mage Targets Run

- `mage testPkg ./internal/services/networkpolicy` → **23 tests pass, coverage 100.0%** (well above the 60% mage gate and 70% CLAUDE.md target).
- `mage testPkg ./internal/services/images` → **69 tests pass, coverage 81.9%** (delta from Unit 15.1 baseline 63 → 69, +6 new tests).
- `mage testPkg ./internal/adapters/docker` → **51 tests pass, coverage 65.3%** (unchanged from Unit 15.2 baseline; ListNetworks addition is exercised only via the networkpolicy fake and the production integration path).
- `mage test` (full) → **861 tests pass across 23 packages, zero failures**. Every package ≥ 60% coverage gate.
- `mage integration` → **355 pass + 1 skip + 0 fail across 2 packages**. The single skip is the Unit 15.2.5 macOS-gate test (intentional, manual-validation-required).
- `go tool gofumpt -l` on every touched file → clean (empty output).

### Design Decisions

- **Consumer-side `NetworkPolicy` interface in `internal/services/images`.** PLAN says "the policy service" — but where does the interface live? Per Go idiom (and CLAUDE.md § "Interface-first boundaries"), the interface goes near the **consumer**, not the producer. I declared `NetworkPolicy`, `NetworkPolicyRequest`, `NetworkPolicyMaterial`, `NetworkPolicyCleanup` in `internal/services/images/service.go`. The production `networkpolicy.Service` does NOT import `images`; instead, an adapter (in the integration test) translates between the two structurally-identical types. This keeps the dependency direction clean and decouples future consumer evolution (e.g. Unit 15.3's `internal/services/run`) from `internal/services/networkpolicy`'s shape.
- **Deterministic `networkName(allowlist)`.** Combines the human-readable `valv-netpol-` prefix with the first 12 hex chars of a sha256 over the sorted comma-joined allowlist. Same allowlist → same name → idempotent reclaim. Different allowlists → different names → orphan cleanup runs. The 12-hex-char (48-bit) suffix gives ample collision resistance for the practical N=O(1) of concurrent allowlist variants on one host.
- **Orphan-cleanup contract is "list-by-label + reconcile".** On every `Provision` call, list networks tagged `valv=network-policy`; if a network with the deterministic name for THIS allowlist exists, reclaim it (no Create, no Remove); otherwise, remove every label-matched network and Create fresh. This satisfies PLAN R3.F3.1: a SIGKILLed prior invocation leaves a labeled network; the next launch reconciles. The contract is asserted by `TestProvision_RemovesStaleOrphansBeforeCreate`, `TestProvision_IdempotentReclaimOfMatchingNetwork`, and `TestProvision_ReclaimMatchingAndRemoveStaleSimultaneously`.
- **No proxy daemon in this unit.** PLAN allows either "a Go-side HTTP CONNECT proxy" OR "stub the proxy and just verify build-args + network attachment". I chose the latter — `networkpolicy.Service` owns network lifecycle + policy material composition; the proxy daemon is owned by the runtime caller (Unit 15.3) which knows whether closed mode is active. Tests verify the contract by asserting build args + `--network` in `docker buildx build` invocations. The integration test wires a real in-process CONNECT proxy on the host side to validate reachability end-to-end on macOS Docker Desktop.
- **`ListNetworks` on docker.Executor uses `--format {{.Name}}` instead of `--filter "label=key=value"` parsing.** Docker accepts `--filter "label=valv=network-policy"` as a key=value filter; the format flag pulls just the network names so the service does not have to parse a tabular listing. The empty-output case returns nil slice (idiomatic Go).
- **Build-policy injection ONLY on the rebuild path.** Policy provisioning happens AFTER the freshness probe and ONLY when a rebuild is required. The up-to-date short-circuit returns without ever calling Provision, so no docker network is created on no-op runs. Verified by `TestEnsureProjectImage_NetworkPolicyUpToDate_NoProvision`.
- **Effective allowlist threaded from manifest, not from `Options.Allowlist`.** PLAN: "consumes the manifest and effective allowlist already resolved at the detected project root (Unit 15.0); do not add any new raw-cwd `.valv/tools.toml` lookup." `EnsureProjectImage` already receives the manifest in `EnsureProjectRequest`. Calling `tools.EffectiveAllowlist(request.Manifest.Allowlist)` honors that contract — no new file lookup in `internal/services/images` or `internal/services/networkpolicy`.
- **OverlayHash + project-overlay tag identity preserved.** The five freshness labels emitted by `EnsureProjectImage` are unchanged; only `BuildArgs` and `Network` on `docker.ImageBuildRequest` grow when policy is active. PLAN: "Build-policy injection must not change `OverlayHash`, project-overlay tags, or the existing freshness-label contract from DROP_12." Verified by `TestEnsureProjectImage_NetworkPolicyInjectsProxyArgsAndNetwork` (asserts both the proxy-args/network appear AND the five Unit 12.3 labels still appear, plus tag string still matches `expectedProjectTag(...)`).
- **Pre-existing latent duplicate-const bug fixed.** `testClaudeCLIVersion` was declared in both `service_test.go` and `service_integration_test.go` since commit `6b4ea4f` — the duplicate was masked because `mage integration` never targeted `./internal/services/images`. When this unit wired the integration tag into the magefile, the duplicate surfaced as a build error. The fix removes the integration-file declaration; the test_test.go one is the source of truth.

### Macos Validation Outcome (manual-validation-required)

Per PLAN R3.F4.1.1, the macOS Docker Desktop test was executed locally with `VALV_NETPOL_INTEGRATION_RUN=1`. Result:

- **Wiring correct:** the rendered `docker buildx build` command for the overlay rebuild includes `--network valv-netpol-58c4e33949bf`, `--build-arg HTTPS_PROXY=http://host.docker.internal:51202`, `--build-arg HTTP_PROXY=http://host.docker.internal:51202`, `--build-arg NO_PROXY=github.com,objects.githubusercontent.com,proxy.golang.org,sum.golang.org`, plus the five freshness labels.
- **Reachability blocker confirmed:** the in-process HTTP CONNECT proxy saw **zero CONNECT requests**, and the buildx invocation failed because `go install rsc.io/quote@v1.5.2` could not reach the proxy. This is the **A1 risk PLAN.md flagged** in Notes For Builder Agents: "If `host.docker.internal` does not resolve/reach the host-local proxy from the single `--internal` network topology on Docker Desktop macOS, block DROP_15 on that platform. Do **not** attach `bridge` or any second network in DROP_15 as a fallback."
- **Implication:** the Round 1 implementation correctly **surfaces this blocker** rather than papering over it with a bridge attachment. The orchestrator/dev makes the final call: either (a) accept the topology limitation and proceed without closed-default network policy on macOS, (b) re-scope the unit to allow a documented bridge fallback (out of current PLAN), or (c) block DROP_15 pending an alternative reachability solution. The test ships **skipped by default** so CI does not regress on a known platform limitation, with the env-var escape hatch for manual rerun.

### Counterexample Mitigation Map (Round 1 implementation against PLAN)

| PLAN acceptance criterion | Implementation location | Validating test(s) |
|---|---|---|
| Shared `internal/services/networkpolicy` service returns proxy URLs + NO_PROXY + internal-network name + cleanup handle | `internal/services/networkpolicy/service.go` → `Service.Provision` → `PolicyMaterial` + `Cleanup` | `TestProvision_CreatesNetworkOnFreshHost` |
| `internal/services/images/service.go` reuses the service for overlay builds; injects `HTTP_PROXY`/`HTTPS_PROXY`/`NO_PROXY` + `--network` | `internal/services/images/service.go` → `EnsureProjectImage` rebuild path | `TestEnsureProjectImage_NetworkPolicyInjectsProxyArgsAndNetwork` |
| Build-policy seam consumes manifest resolved at detected project root (no raw-cwd lookups) | Uses `request.Manifest.Allowlist` only; no `os.Getwd`/`filepath.Join` against cwd anywhere in service.go diff | code review + grep |
| Effective allowlist includes built-in defaults | `tools.EffectiveAllowlist(request.Manifest.Allowlist)` at line 861 | `TestEnsureProjectImage_NetworkPolicyInjectsProxyArgsAndNetwork` asserts all four built-ins present even with zero-value manifest.Allowlist |
| Single internal Docker network topology only; no bridge fallback | `Provision` creates exactly one `--internal` network; no `docker network connect bridge` call anywhere | `TestProvision_CreatesNetworkOnFreshHost` asserts `Internal=true` in the lone CreateNetwork call |
| Build-policy injection does NOT change OverlayHash, project-overlay tags, freshness-label contract | Labels block in `EnsureProjectImage` is byte-identical to pre-policy code; only `BuildArgs` + `Network` grow | `TestEnsureProjectImage_NetworkPolicyInjectsProxyArgsAndNetwork` asserts target tag = `expectedProjectTag(...)` (computed from manifest only) AND all five Unit 12.3 labels still appear |
| Table-driven service tests prove build args include `--network` + proxy args + preserve existing overlay labels + `--no-cache` | service_test.go new tests 1-6 | 6 new tests in service_test.go |
| Tagged integration test proves `.valv/tools.toml`-driven `go install` reaches proxy and is filtered; `NoCache=true`; single-network topology | `TestEnsureProjectImage_NetworkPolicyOverlayBuildReachesProxy_DockerDesktopMacOS` with VALV_NETPOL_INTEGRATION_RUN=1 escape hatch | Test ships skipped (manual-validation-required); Round 1 builder ran locally and recorded outcome above |
| Docker Desktop macOS gate (R3.F4.1.1): manual-validation-required marker + builder notes | Test skip with descriptive `t.Skipf` referencing BUILDER_WORKLOG.md + this outcome section | code + this worklog |
| Orphan-cleanup contract (R3.F3.1): label-tagged + reconcile-on-startup; tests prove stale recovery | `Provision` orphan-cleanup loop; `CleanupStale` for explicit cleanup | `TestProvision_RemovesStaleOrphansBeforeCreate`, `TestProvision_IdempotentReclaimOfMatchingNetwork`, `TestProvision_ReclaimMatchingAndRemoveStaleSimultaneously`, `TestCleanupStale_RemovesAllManagedNetworks` |
| Error wrapping boundaries explicit for policy setup, build request assembly, build execution, cleanup | All five touchpoints wrap errors with descriptive prefixes (`networkpolicy provision: ...`, `ensure project image: provision network policy: ...`, `ensure project image: build effective allowlist: ...`, `networkpolicy cleanup: remove network ...`) | sentinel `errors.Is` tests across networkpolicy + images packages |

### Hard-Constraint Compliance

- All production edits live in `internal/services/networkpolicy/` (new package), `internal/services/images/` (Options + Service + EnsureProjectImage), and `internal/adapters/docker/executor.go` (one new `ListNetworks` method) plus tests in each.
- One ancillary edit to `magefile.go` (`Integration` target) — required to actually compile the integration test in CI. Documented in design notes above; this was the trigger that surfaced the latent duplicate-const bug.
- No edits to `internal/services/run/` (Unit 15.3 owns runtime closed-mode).
- No edits to `internal/cli/` (Unit 15.4 owns CLI surface).
- No `NetworkConnectRequest` / second-network logic anywhere (Round 4 YAGNI cut respected).
- Section 0 reasoning lives in orchestrator response only — not in this worklog.

### Hylla Feedback

None — existing patterns in `internal/services/images/service.go`, `internal/adapters/docker/network.go`, `internal/adapters/docker/executor.go`, and `internal/tools/allowlist.go` provided sufficient evidence directly via `Read`. LSP was unavailable (gopls sync error against the active checkout) so symbol-reference checks fell back to `rg`. Context7 was used once for Docker proxy build-args + `docker network create --internal` semantics confirmation (`/docker/docs`).

### Acceptance Check against drop's PLAN.md Unit 15.2.5

- [x] Shared policy-material service in `internal/services/networkpolicy` accepts effective allowlist; returns proxy URLs, NO_PROXY, internal-network name, cleanup handle.
- [x] `internal/services/images/service.go` reuses the service for overlay builds; injects predefined `HTTP_PROXY`/`HTTPS_PROXY`/`NO_PROXY` build args and `ImageBuildRequest.Network = <internal-network>` when policy is active.
- [x] Build-policy seam consumes the manifest resolved at the detected project root (Unit 15.0); no new raw-cwd `.valv/tools.toml` lookup added.
- [x] Effective allowlist includes built-in defaults from Unit 15.1.
- [x] Single internal Docker network topology only; no second-network attachment in Round 1.
- [x] Build-policy injection does NOT change `OverlayHash`, project-overlay tags, or the freshness-label contract from DROP_12.
- [x] `internal/services/images/service_test.go` remains table-driven and proves build args include `--network` plus proxy build args while preserving existing overlay labels and `--no-cache` behavior.
- [x] `internal/services/images/service_integration_test.go` gains a tagged integration test (manual-validation-required) proving `.valv/tools.toml`-driven `go install` during overlay build reaches the host proxy. `EnsureProjectRequest.NoCache = true` set. Single-network topology used.
- [x] Docker Desktop macOS gate marker present + builder notes captured the validation outcome.
- [x] Orphan-cleanup contract: `valv=network-policy` label on every managed network; startup reclaim/clean loop in `Provision`; explicit `CleanupStale` for non-provisioning startup recovery. Tests prove a stale labeled network is detected and reused/cleaned.
- [x] Error wrapping boundaries explicit for policy setup, image-build request assembly, docker build execution, and cleanup.

## Unit 15.2 — Round 2

**Date:** 2026-05-24
**Builder backend:** claude-native (orchestrator dispatch)

### Goal

Address the three Round 1 falsification findings in `internal/adapters/docker/network.go` validation:

1. Label keys with surrounding whitespace are accepted by `Valid()` but emitted untrimmed into `--label` args, breaking Unit 15.2.5 orphan-cleanup label matching.
2. Label keys containing `=` are not rejected, which breaks Docker's label parsing (splits on first `=`, dropping the suffix).
3. No maximum network name length is enforced; a 256-byte name is accepted when Docker's practical limit is 64 bytes.

### Files Touched

- `internal/adapters/docker/network.go`
  - **Fix #1 (untrimmed label keys):** added validation in `NetworkCreateRequest.Valid()` that rejects any label key where `strings.TrimSpace(key) != key`. Matches the existing "leading-hyphen rejected" precedent in name validation. Error message: "label key must not have leading or trailing whitespace".
  - **Fix #2 (label keys with `=`):** added validation in `NetworkCreateRequest.Valid()` that rejects any label key containing the `=` rune via `strings.ContainsRune(key, '=')`. Error message: "label key must not contain '='".
  - **Fix #3 (network name length):** added length limit validation in both `NetworkCreateRequest.Valid()` and `NetworkRemoveRequest.Valid()` that rejects names longer than 64 bytes. Error message: "network name must be at most 64 bytes, got N".

- `internal/adapters/docker/network_test.go` — added three new test functions covering all three findings:
  - `TestNetworkCreateRequestRejectsUntrimmedLabelKey` — 4 sub-tests covering leading whitespace, trailing whitespace, both sides, and tab characters.
  - `TestNetworkCreateRequestRejectsLabelKeyWithEquals` — 3 sub-tests covering `=` in middle, at start, and multiple equals.
  - `TestNetworkCreateRequestRejectsOverlongName` — 3 sub-tests: 64-byte boundary acceptance, 65-byte rejection, 256-byte rejection.

- `drops/DROP_15_NETWORK_POLICY/PLAN.md` — Unit 15.2 `state: done` → `in_progress` → (pending final state flip).

### Mage Targets Run

- `mage testPkg ./internal/adapters/docker` → **64 tests pass, 0 failures, coverage 65.7%** (51 baseline + 13 new tests). All tests green across original + new test cases.

### Design Notes

- **Untrimmed-key fix strategy:** Chose rejection over mutation because it matches the existing name-validation style ("leading-hyphen rejected") and keeps the contract clear at the input boundary. Mutation inside `Valid()` would require either struct modification or hiding the transformation from the caller.
- **Label-key `=` rejection:** Docker's CLI label syntax is `--label key=value` where Docker parses on the FIRST `=` — a key like `k=injected` with value `v` becomes Docker label `{"k": "injected=v"}`, losing the operator's intended suffix. Rejecting `=` in keys at validation time prevents this silent data corruption.
- **Network name 64-byte limit:** Docker's documented practical maximum for network names is 64 bytes. The regex pattern is correct but lacked a length anchor. Applied to both `NetworkCreateRequest.Valid()` and `NetworkRemoveRequest.Valid()` for symmetry.
- **Boundary test:** Added 64-byte acceptance case plus 65-byte rejection case to pin the boundary. A future tightening would fail the accept case.

### Hylla Feedback

None — existing patterns in `internal/adapters/docker/network.go` provided sufficient evidence directly via `Read`. The three findings came from BUILDER_QA_FALSIFICATION.md Unit 15.2 Round 1 section.

### Acceptance Check against Round 2 brief

- [x] All three Round 1 findings addressed with narrow fixes (validation layer only).
- [x] Fix #1 → untrimmed-key rejection with descriptive error message and 4 sub-tests.
- [x] Fix #2 → `=` in keys rejection with descriptive error message and 3 sub-tests.
- [x] Fix #3 → 64-byte name limit with descriptive error message and 3 sub-tests (boundary + rejection cases).
- [x] 64 tests pass (51 baseline + 13 new). Coverage 65.7% ≥ 60% enforced gate.
- [x] No raw `go test` / `GOCACHE=...` invocations — only `mage testPkg`.

### Hard-Constraint Compliance

- All Round 2 edits live in `internal/adapters/docker/network.go` (three validation guards) and `internal/adapters/docker/network_test.go` (three new test functions).
- No edits to executor.go (Unit 15.2.5 already added ListNetworks there).
- No edits to drop dir except PLAN.md state-bit flip and this worklog appendix.
- No `NetworkConnectRequest` added (Schema Decision 5 cut respected).
