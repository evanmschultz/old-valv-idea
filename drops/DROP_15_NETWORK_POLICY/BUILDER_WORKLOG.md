# DROP_15 — Builder Worklog

Append a `## Unit 15.M — Round K` section per build attempt. See `main/drops/WORKFLOW.md` § "Phase 4 — Build (per unit)" for what each section should contain.

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
