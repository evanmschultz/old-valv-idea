# DROP_15 — Builder QA Proof

Append a `## Unit 15.M — Round K` section per QA Proof review. See `main/drops/WORKFLOW.md` § "Phase 5 — Build QA (per unit)".

## Unit 15.0 — Round 1

**Date:** 2026-05-24
**QA Proof backend:** claude-native (orchestrator dispatch, opus)
**Verdict:** `pass`

### Per-Acceptance Audit

**A1. `resolveProjectImage` resolves the project root via `project.DetectFrom(workingDir)` before `tools.Resolve`; raw `tools.Resolve(workingDir)` is removed.**

- Verified at `internal/cli/operator_helpers.go:448-457`:
  ```
  func resolveProjectImage(cmd *cobra.Command, paths config.Paths, provider domain.Provider, workingDir string, baseRef dockeradapter.ImageRef) (dockeradapter.ImageRef, error) {
      detected, err := project.DetectFrom(workingDir)
      if err != nil {
          return dockeradapter.ImageRef{}, fmt.Errorf("resolve project image: %w", err)
      }

      manifest, err := tools.Resolve(detected.Root)
  ```
- `project` import added at `internal/cli/operator_helpers.go:20`.
- Removal verified by ripgrep: `rg "tools.Resolve\(workingDir\)" internal/cli/` returns 0 matches. The only remaining `tools.Resolve(...)` call site in the file is the new `tools.Resolve(detected.Root)` at line 454.
- API signature confirmed: `project.DetectFrom(start string) (Result, error)` at `internal/project/project.go:28`, with `Result.Root` field at `internal/project/project.go:13`. Fallback semantics (no `.git` marker → returns start path) at `internal/project/project.go:49-55`.

**A2. Empty-manifest short-circuit and overlay-build path both consult `.valv/tools.toml` at the detected project root.**

- Code path: `tools.Resolve(detected.Root)` → if `len(manifest.Tools) == 0` short-circuits at `internal/cli/operator_helpers.go:458-460`; otherwise the override branch and `EnsureProjectImage` branch consume the same `manifest`.
- Verified by `TestResolveProjectImageClaudeNestedSubdirFindsRootManifest` at `internal/cli/claude_project_image_test.go:234-267`:
  - Fixture: `writeClaudeNestedToolsManifest` creates `<root>/.git/`, `<root>/.valv/tools.toml`, and nested `<root>/pkg/sub/` (lines 184-208).
  - Asserts `got.String() != baseRef.String()` (line 253), `proj-` tag prefix (line 256), and `buildx build --load` recorded (line 264).
- Mirror at `TestResolveProjectImageCodexNestedSubdirFindsRootManifest` at `internal/cli/codex_project_image_test.go:213-246`.
- Absent-manifest-at-root case verified by `TestResolveProjectImageClaudeNestedSubdirNoManifestReturnsBase` at `internal/cli/claude_project_image_test.go:274-301`: `writeClaudeNestedNoManifest` creates `<root>/.git/` and `<root>/pkg/sub/` but no `.valv/tools.toml` (lines 215-226); asserts `got.String() == baseRef.String()` (line 292), zero stderr (line 295), and zero docker calls (lines 298-300). Mirror at `internal/cli/codex_project_image_test.go:251-278`.

**A3. Override semantics unchanged: `VALV_CLAUDE_IMAGE` / `VALV_CODEX_IMAGE` still emit one warning and skip overlay AFTER manifest resolution from the detected root.**

- Code: override check at `internal/cli/operator_helpers.go:462-467` runs AFTER `tools.Resolve(detected.Root)` (line 454) and AFTER the empty-manifest short-circuit (line 458). One `fmt.Fprintln` warning + early `return baseRef, nil`.
- Verified by `TestResolveProjectImageClaudeNestedSubdirOverrideAfterRootResolve` at `internal/cli/claude_project_image_test.go:310-343`:
  - `t.Setenv("VALV_CLAUDE_IMAGE", "test/claude:override")` (line 311) + nested subdir fixture (line 313).
  - Asserts `got.String() == baseRef.String()` (line 329), the exact warning string (lines 333-336), `strings.Count(stderr, wantWarning) == 1` (line 337-339), and zero docker calls (lines 340-342). All four QA-spec checks present.
- Mirror at `TestResolveProjectImageCodexNestedSubdirOverrideAfterRootResolve` at `internal/cli/codex_project_image_test.go:284-317` with identical `strings.Count == 1` assertion at line 311-313.
- Pre-existing override tests (non-nested) also retained: `TestResolveProjectImageClaudeOverrideSkipsOverlay` at lines 118-154 and `TestResolveProjectImageCodexOverrideSkipsOverlay` at codex test lines 103-137, both with `strings.Count == 1` assertions.

**A4. Repo-subdirectory coverage added to both `claude_project_image_test.go` and `codex_project_image_test.go`.**

- Three new tests + two new fixture helpers per file:
  - Claude: `TestResolveProjectImageClaudeNestedSubdirFindsRootManifest` (line 234), `TestResolveProjectImageClaudeNestedSubdirNoManifestReturnsBase` (line 274), `TestResolveProjectImageClaudeNestedSubdirOverrideAfterRootResolve` (line 310); helpers `writeClaudeNestedToolsManifest` (line 184) and `writeClaudeNestedNoManifest` (line 215).
  - Codex: `TestResolveProjectImageCodexNestedSubdirFindsRootManifest` (line 213), `TestResolveProjectImageCodexNestedSubdirNoManifestReturnsBase` (line 251), `TestResolveProjectImageCodexNestedSubdirOverrideAfterRootResolve` (line 284); helpers `writeCodexNestedToolsManifest` (line 166) and `writeCodexNestedNoManifest` (line 196).
- Fixture pattern confirmed: each helper creates `.git/` dir at projectRoot so `project.DetectFrom` climbs back, then invokes from `<projectRoot>/pkg/sub`. Matches the documented `project.DetectFrom` behavior verified in `internal/project/project_test.go:11-30`.

### Mage Results (run by QA Proof)

```
mage testPkg ./internal/cli
[PKG PASS] github.com/evanmschultz/valv/internal/cli (6.79s)
  tests: 240
  passed: 240
  failed: 0
  package coverage: 67.6% (above 60% gate)
```

Matches builder's claim of 240 tests / 67.6% coverage exactly.

### Scope Compliance

`git diff-tree --no-commit-id --name-only -r 01ba002` returns exactly:

- `drops/DROP_15_NETWORK_POLICY/BUILDER_WORKLOG.md`
- `drops/DROP_15_NETWORK_POLICY/PLAN.md`
- `internal/cli/claude_project_image_test.go`
- `internal/cli/codex_project_image_test.go`
- `internal/cli/operator_helpers.go`

No edits to `internal/tools/`, `internal/adapters/docker/`, `internal/services/networkpolicy/`, `internal/services/images/`, or `internal/services/run/`. Hard-constraint compliance holds.

### Findings

- F1. `project.DetectFrom` is the correct API surface for Unit 15.0: it takes an explicit start path (vs `Detect()` which uses `os.Getwd()`), is goroutine-safe, walks up to nearest `.git` marker, and falls back to the start path when no marker is found. Confirmed by `internal/project/project.go:28-58` and existing test coverage at `internal/project/project_test.go:11-165` (5 cases including nested subdir, linked worktree, no-marker fallback, file path, symlinked root).
- F2. The two pre-existing empty-manifest tests (`TestResolveProjectImageClaudeEmptyManifestReturnsBaseRef`, `TestResolveProjectImageCodexEmptyManifestReturnsBaseRef`) continue to pass because `project.DetectFrom` on a bare `t.TempDir()` (no `.git`) falls back to returning the start path itself, preserving the original semantic.
- F3. TDD red-then-green claim is plausible but not independently re-verifiable post-commit: the change is squashed into commit `01ba002`. Builder worklog at `BUILDER_WORKLOG.md:66` documents the red signal explicitly (4 of 6 new tests failed with the expected "manifest at root should have triggered overlay" + "missing override warning" diagnostics) — this is consistent with the diff (the nested-FindsRoot and nested-Override tests genuinely depend on the new `project.DetectFrom` behavior to pass).
- F4. The override warning assertion uses `strings.Count(stderr.String(), wantWarning) == 1` in all four override tests (`claude` lines 145-147 + 337-339, `codex` lines 130-132 + 311-313), satisfying the "exactly one warning" specification verbatim.
- F5. No control-flow reorder of the override path. The override check still runs AFTER manifest resolution but before `openImagesService`. This matches Acceptance Criterion 3 ("after the manifest has been resolved from the detected root") and the pre-existing DROP_12 Unit 12.4 semantics.

### Missing Evidence

None. All four acceptance criteria are satisfied with file:line citations, scope is clean, mage gate passes at the documented coverage level, and the test fixtures exercise the precise pre/post code path that Unit 15.0 changed.

### Summary

Verdict: **pass**.

Unit 15.0 cleanly re-roots `resolveProjectImage` from raw-cwd manifest resolution to detected-project-root manifest resolution via the existing `project.DetectFrom` API. The change is minimal (one new import + four-line function preamble), preserves override semantics structurally (no control-flow reorder), and is exhaustively covered by six new tests (three per provider) plus the four pre-existing tests left in place as regression guards. Hard-constraint compliance (no edits outside `internal/cli/`) holds. `mage testPkg ./internal/cli` reproduces 240 tests passing at 67.6% coverage.

## Unit 15.1 — Round 1

**Date:** 2026-05-24
**QA Proof backend:** claude-native (orchestrator dispatch, opus)
**Verdict:** `pass`

### Per-Acceptance Audit

**A1. Promote `ToolManifest.Allowlist` from `toml.Primitive` to typed `AllowlistConfig{ Hosts []string }`.**

- Verified at `internal/tools/allowlist.go:25-27`:
  ```
  type AllowlistConfig struct {
      Hosts []string `toml:"hosts"`
  }
  ```
- Field on the manifest at `internal/tools/tools.go:79`: `Allowlist AllowlistConfig \`toml:"allowlist"\`` (replacing the prior `toml.Primitive`). `Env toml.Primitive \`toml:"env"\`` preserved verbatim at line 80 pending DROP_14.
- The previously-discarded `PrimitiveDecode(m.Allowlist, ...)` call from DROP_11 has been removed in `Load` (`internal/tools/tools.go:94-128`); only the `Env` PrimitiveDecode pass remains (lines 115-118). Type-decode of `[allowlist].hosts` into a typed slice causes BurntSushi/toml to mark the sub-keys as decoded, so the strict `meta.Undecoded()` check still rejects unknown sub-keys (verified empirically by `TestLoad_AllowlistUnknownKeyRejected` at `internal/tools/allowlist_test.go:169-201`).

**A2. `Load` decodes `[allowlist]` into the typed struct while keeping `[env]` deferred and preserving strict unknown-top-level rejection.**

- `Load` flow at `internal/tools/tools.go:94-128`: stat → `toml.DecodeFile` into typed `ToolManifest` → `PrimitiveDecode(m.Env, &discardEnv)` → `meta.Undecoded()` check that wraps the unknown keys into an error.
- Strict rejection coverage:
  - Unknown sub-key inside `[allowlist]` rejected: `TestLoad_AllowlistUnknownKeyRejected` at `internal/tools/allowlist_test.go:169-201` (writes `cidrs = ["10.0.0.0/8"]` under `[allowlist]`; asserts the error wraps "unknown keys" and mentions `allowlist.cidrs`).
  - Typed decode of `[allowlist].hosts`: `TestLoad_AllowlistTypedDecode` at `allowlist_test.go:138-167` and the updated `TestResolve_ValidManifest_WithForwardCompatSections` at `internal/tools/resolve_test.go:97-105` (asserts `m.Allowlist.Hosts == [github.com, proxy.golang.org]`).
- `Env` still `toml.Primitive` (line 80 of `tools.go`) — unchanged from DROP_11, deferred for DROP_14. Verified.

**A3. `EffectiveAllowlist` returns zero-value defaults and union semantics; lowercase + dedup + validation.**

- Function at `internal/tools/allowlist.go:81-107`.
- Defaults: `DefaultAllowlistHosts` at lines 38-43 contains exactly `[github.com, objects.githubusercontent.com, proxy.golang.org, sum.golang.org]` (4 hosts, lexicographically sorted).
- Zero-value behavior verified by `TestEffectiveAllowlist_ZeroValue` at `allowlist_test.go:12-28` (asserts the 4-host result exactly).
- Union + lowercase + dedup verified by `TestEffectiveAllowlist_UnionWithUserHosts` (lines 30-51) and `TestEffectiveAllowlist_LowercaseAndDedup` (lines 53-80) — the latter also case-folds `Github.com` against the built-in `github.com` default.
- Validation regex at `allowlist.go:71` (`hostShapeRE`): RFC 1123-style labels, alphanumerics + hyphens, dot-separated. Fast-path rejections in `validateHost` (lines 112-133) for whitespace, `://`, `/?#`, and `:`. `TestEffectiveAllowlist_InvalidHost` at lines 82-116 covers 12 invalid-host cases (empty, whitespace, URL, path, query, port, leading/trailing dot, leading/trailing hyphen, underscore, space, non-ASCII).
- Docker alias acceptance verified by `TestEffectiveAllowlist_DockerAliasAccepted` at lines 118-136.

**A4. `WriteAllowlistSection` creates parent `.valv/` via `os.MkdirAll(filepath.Dir(path), 0o755)` and writes a fresh manifest on absent-file.**

- Function at `internal/tools/allowlist.go:171-217`.
- `os.MkdirAll(filepath.Dir(path), 0o755)` at line 175. Confirmed.
- Empty-path rejection at line 172-174 returns `"write allowlist section: empty path"` — verified by `TestWriteAllowlistSection_EmptyPath` at lines 548-553.
- Fresh-file path at lines 179-190: `os.ReadFile` returning `os.ErrNotExist` → `renderAllowlistSection(cfg)` → `os.WriteFile`. Verified by `TestWriteAllowlistSection_FreshProjectNoDir` (lines 203-237) which creates `<tmp>/.valv/tools.toml` from a project root that has no `.valv/` directory and round-trips through `Load`.

**A5. Section-safe rewrite: byte-preservation outside the bounded `[allowlist]` span.**

- Implementation at `internal/tools/allowlist.go:192-216` (calls `splitAllowlistSpan` at line 192). Splitter at lines 261-355.
- Span boundary literalness — PLAN.md Schema Decision 3 says span begins at `[` of `[allowlist]` and ends "immediately before the first byte of the next top-level section header" (or EOF). Implementation matches exactly:
  - `allowlistStart` = byte offset of `[` of `[allowlist]` (line 309-314).
  - `nextStart` = byte offset of `[` of the next top-level header after `[allowlist]` (line 316-318).
  - prefix = `content[:allowlistStart]` (lines 348/352/354).
  - suffix = `content[nextStart:]` (line 354) or empty when `[allowlist]` is last (line 352).
- Top-level header detection regex at line 241 (`^\[([^\[\]]+)\][ \t]*(?:#.*)?$`) correctly:
  - matches `[allowlist] # network-policy hosts` (because `[ \t]*(?:#.*)?$` accepts trailing comments)
  - matches `[tools]`
  - rejects array-of-tables headers (`[[…]]` excluded by `[^\[\]]+`); array-of-tables is separately fast-rejected earlier at line 302.
- The renderer at lines 220-233 emits canonical form: `[allowlist]\n` + (`hosts = []\n` when empty OR `hosts = [\n  "h1",\n  "h2",\n]\n` otherwise).

**A6. Golden fixture round-trips byte-preservation contract.**

- `TestWriteAllowlistSection_PreservesPrefixAndSuffix` at `internal/tools/allowlist_test.go:260-371` covers the exact PLAN.md spec:
  - File preamble: 2 comment lines + blank line (`allowlist_test.go:272-274`) — outside the span, must round-trip verbatim. Asserted by `strings.HasPrefix(got, wantPrefix)` at line 308.
  - Inline comment on `[allowlist]` header (`# network-policy hosts`) — INSIDE the span per the spec, legitimately discarded. Asserted absent at line 356: `if strings.Contains(got, "# network-policy hosts") { t.Errorf(...) }`. Test comment at lines 314-329 explicitly cites the PLAN.md spec for why this is correct.
  - Divider comment between `[allowlist]` and `[tools]` (`# --- tools ---`) — INSIDE the span per the spec, discarded. The `wantSuffix` (line 330-338) begins at `[tools]`, not at the divider — matching the implementation's `content[nextStart:]` where `nextStart` is the byte offset of `[` in `[tools]`.
  - `[env]` block + interleaved comments (`# preserve me too`) — OUTSIDE the span, must round-trip verbatim. Captured in `wantSuffix` (lines 333-338), asserted by `strings.HasSuffix(got, wantSuffix)` at line 339.
- Additional shape coverage:
  - Last-section: `TestWriteAllowlistSection_AllowlistAsLastSection` (lines 373-410) — proves suffix-empty path with `[allowlist]` as the last section.
  - Absent-`[allowlist]`-appends: `TestWriteAllowlistSection_AllowlistAbsentAppendsAfterTrailingBytes` (lines 412-448) — proves prefix=whole-file behavior.

**A7. Unsupported shapes return `ErrUnsupportedManifestShape`.**

- Sentinel at `internal/tools/allowlist.go:50`: `var ErrUnsupportedManifestShape = errors.New(...)`.
- BOM rejection: `splitAllowlistSpan` lines 262-264 → `TestWriteAllowlistSection_RejectsBOM` at `allowlist_test.go:450-470` (asserts `errors.Is(err, ErrUnsupportedManifestShape)`).
- CRLF rejection: lines 265-267 → `TestWriteAllowlistSection_RejectsCRLF` at lines 472-492.
- Multi-line string outside `[allowlist]`: lines 327-331 (`bytes.Contains(line, """) || ...`) → `TestWriteAllowlistSection_RejectsMultilineStringOutsideAllowlist` at lines 494-519.
- Array-of-tables header: lines 302-304 (`bytes.HasPrefix(trimmed, [[)`) → `TestWriteAllowlistSection_RejectsArrayOfTablesHeader` at lines 521-546.
- Duplicate `[allowlist]` header: lines 311-313 → not directly tested but exercised through the structural invariant; not in the acceptance bullet but a defensive guard. Acceptable.

**A8. `internal/services/images/overlay_test.go` regression: identical `Tools` + different `Allowlist` → same `OverlayHash`.**

- Test at `internal/services/images/overlay_test.go:149-187`: `TestOverlayHash_AllowlistDataIgnored`.
- Permutations:
  - `base` with `Allowlist.Hosts = [a.example.com]` (line 163).
  - `other` with `Allowlist.Hosts = [b.example.org, c.example.net, d.example.io]` (lines 170-172).
  - `empty` with no Allowlist field set (lines 178-183).
- Assertions: `OverlayHash(base) == OverlayHash(other)` (line 174) AND `OverlayHash(base) == OverlayHash(empty)` (line 184).
- Implementation check: `OverlayHash` at `internal/services/images/overlay.go:60-71` feeds `canonicalManifest(manifest)` (lines 35-52). `canonicalManifest` only reads `manifest.Tools` — `manifest.Allowlist` is never referenced by the hash computation. The regression test is a forward-pin: any future drop that adds `manifest.Allowlist` to the hash payload will instantly fail.

### Mage Results (run by QA Proof)

```
mage testPkg ./internal/tools
[PKG PASS] github.com/evanmschultz/valv/internal/tools (1.29s)
  tests: 86
  passed: 86
  failed: 0
  package coverage: 93.4% (above 60.0% gate, above 70% CLAUDE.md target)
```

```
mage testPkg ./internal/services/images
[PKG PASS] github.com/evanmschultz/valv/internal/services/images (1.37s)
  tests: 63
  passed: 63
  failed: 0
  package coverage: 81.6% (above 60.0% gate, above 70% CLAUDE.md target)
```

Both reproduce the builder's documented numbers exactly (86 tests / 93.4% for tools, 63 tests / 81.6% for images).

`mage test` (full suite) was attempted but fails on the CURRENT working tree because of uncommitted DROP_13/DROP_14 work (`internal/services/run/service_test.go`, `internal/adapters/sqlite/store.go`, etc.) — those failures are NOT scoped to Unit 15.1. At commit `0611cf0` itself the tree was clean and the builder documented `mage test 747/747 green` plus `mage integration 240 pass + 3 pre-existing skips`. The current working-tree failures are out-of-scope for this Unit 15.1 review and route to the appropriate DROP_13/DROP_14 build-QA cycles.

### Scope Compliance

`git diff-tree --no-commit-id --name-only -r 0611cf0` returns exactly:

- `internal/services/images/overlay_test.go`
- `internal/tools/allowlist.go`
- `internal/tools/allowlist_test.go`
- `internal/tools/resolve_test.go`
- `internal/tools/tools.go`

No edits to `internal/cli/`, `internal/adapters/`, `internal/services/networkpolicy/`, `internal/services/run/`. The single allowed cross-package edit (`internal/services/images/overlay_test.go` regression test) is explicitly the exception per the Unit 15.1 spawn appendix.

### Findings

- F1. `AllowlistConfig` typing strategy is correct. Removing the `PrimitiveDecode(m.Allowlist, ...)` call from `Load` is safe because BurntSushi/toml's strict-undecoded check inspects sub-keys: typed-decoding `[allowlist].hosts` into `[]string` marks `allowlist.hosts` as decoded, while leaving unknown sub-keys (e.g. `allowlist.cidrs`) in `meta.Undecoded()`. Empirically verified by `TestLoad_AllowlistUnknownKeyRejected`.
- F2. Span boundary semantics match Schema Decision 3 verbatim. The implementation uses `content[:allowlistStart]` for prefix and `content[nextStart:]` for suffix — both byte offsets are computed against the literal `[` characters of the headers, exactly as the spec requires. Blank lines and divider comments INSIDE the span (between `[allowlist]` and the next top-level header) are legitimately discarded; the test explicitly cites this rule.
- F3. `DefaultAllowlistHosts` ordering is lexicographic (`github.com`, `objects.githubusercontent.com`, `proxy.golang.org`, `sum.golang.org`) which matches the alphabetic-sort result of `EffectiveAllowlist`. The zero-value test asserts exactly this order, and the dedup-test confirms case-folded user inputs collapse against the same defaults.
- F4. The host validator regex enforces RFC 1123-style labels with explicit underscore rejection. This is documented as a design choice in `allowlist.go:124`. Underscores are NOT a valid hostname character per RFC 1123 and would create proxy-config ambiguity downstream.
- F5. The writer is mechanical (no normalization). `cfg.Hosts` is written verbatim (`renderAllowlistSection` at line 220-233 just emits each host with `%q`). Callers that want normalization must run `EffectiveAllowlist` first. This is documented at `allowlist.go:168-170`. Acceptable design — separation of concerns.
- F6. Test count is 17 functions in `allowlist_test.go`, not the "12" the builder appendix mentioned in its summary. The actual count is HIGHER than claimed, which strengthens coverage; this is a minor description drift in the builder note, not a verdict-affecting issue.
- F7. The duplicate `[allowlist]` header guard (lines 311-313) returns `ErrUnsupportedManifestShape` but is not directly exercised by a test. Defensive-only; not in the PLAN.md acceptance bullet.

### Missing Evidence

None for the unit's documented scope. All 8 PLAN.md acceptance bullets are satisfied with file:line citations. Per-package mage results reproduce exactly.

### Summary

Verdict: **pass**.

Unit 15.1 cleanly promotes `[allowlist]` from `toml.Primitive` to typed `AllowlistConfig`, ships an effective-allowlist helper with built-in defaults + union semantics + RFC 1123 validation, and adds a section-safe `WriteAllowlistSection` writer with the explicit bounded-span byte-preservation contract from PLAN.md Schema Decision 3. The strict unknown-key check still works after removing the discarded `PrimitiveDecode(m.Allowlist, ...)` call because BurntSushi/toml correctly tracks sub-key decode state when the field itself is typed. The 17 test functions in `allowlist_test.go` (5 more than the builder note claimed) plus the `TestOverlayHash_AllowlistDataIgnored` regression pin cover every acceptance bullet, every unsupported-shape rejection, and the byte-preservation golden fixture. `mage testPkg ./internal/tools` reproduces 86 tests passing at 93.4% coverage; `mage testPkg ./internal/services/images` reproduces 63 tests passing at 81.6% coverage. Hard-constraint compliance holds (only edits outside `internal/tools/` are the documented `overlay_test.go` regression).

## Unit 15.1 — Round 2

### Verdict

**pass**

All four Round 1 findings (CE#1, CE#2, CE#3, Hidden #4) are fixed at the code level with new tests that exercise the boundary cases. Independent `mage testPkg ./internal/tools` reports 93 tests passing at 91.9% coverage (up from 86 / 93.4% in R1 — 7 added tests). Independent `mage testPkg ./internal/services/images` still reports 63 tests passing at 81.6% coverage, so the cross-package OverlayHash regression pin holds. No edits leak outside `internal/tools/` + the drop dir. No `GOCACHE=...` / raw `go test` discipline violations in the diff.

### Per-claim audit

**CE#1 — Indented [allowlist] header (`splitAllowlistSpan`).**

- Code: `internal/tools/allowlist.go:352-355` — after `topLevelHeaderRE.FindSubmatch(trimmed)` matches, the function immediately checks `line[0] == ' ' || line[0] == '\t'` and returns `ErrUnsupportedManifestShape` with byte-offset context. This fires BEFORE the section-state transitions at lines 358-370, so indented headers cannot silently advance `allowlistStart` / `nextStart`.
- Applies uniformly to `[allowlist]` AND other top-level sections (the indentation guard happens before the `name == "allowlist"` branch).
- Tests: `TestWriteAllowlistSection_RejectsIndentedSectionHeader` at `allowlist_test.go:556` (indented `[allowlist]`) and `TestWriteAllowlistSection_RejectsIndentedOtherSectionHeader` at `allowlist_test.go:587` (indented `[tools]`). Both assert `errors.Is(err, ErrUnsupportedManifestShape)`.
- Verdict: fix is correct, test coverage matches the contract widening.

**CE#2 — RFC 1123 length limits (`validateHost`).**

- Code: `internal/tools/allowlist.go:156-163` — after `hostShapeRE.MatchString(h)` passes, the helper enforces `len(h) > 253` (total) then iterates `strings.Split(h, ".")` checking `len(label) > 63` per label. Error messages name the exact byte limit ("253 bytes" / "63 bytes"). Doc comment at line 130-133 documents the byte-limit contract.
- Length-checks fire AFTER character-shape (line 154 comment), so obviously broken inputs still get the specific underscore / port / scheme error.
- Tests: `TestEffectiveAllowlist_RejectsOverlongLabel` at `allowlist_test.go:675` uses `strings.Repeat("a", 64) + ".example.com"` (64-byte label); `TestEffectiveAllowlist_AcceptsMaxLengthLabel` at line 694 uses `strings.Repeat("a", 63) + ".example.com"` (63-byte boundary accepted); `TestEffectiveAllowlist_RejectsOverlongTotal` at line 716 constructs `strings.Repeat("aaaa.", 50) + "aaaa"` and asserts `len(host) == 254` before checking rejection. All three assert `errors.Is(err, ErrInvalidAllowlistHost)` and the over-length tests grep the error message for "63 bytes" / "253 bytes" so the specific limit surfaces.
- Verdict: boundary coverage is correct — 63 ok, 64 fail; total-length test is genuinely 254 bytes (asserted by the test).

**CE#3 — Multi-line-string detection (`stripLineComment`).**

- Code: `internal/tools/allowlist.go:427-460` — single-pass byte walker over `line`, tracking `inBasic` (`"..."`) and `inLiteral` (`'...'`) state. Inside basic strings, a `\` consumes the next byte (escape handling); inside literal strings there are no escapes (line 444-446 comment matches TOML spec). Outside both, `#` returns `line[:i]`.
- Applied at `allowlist.go:383` — `codeOnly := stripLineComment(line); bytes.Contains(codeOnly, []byte("\"\"\""))` etc. Only the comment-stripped slice is scanned for triple-quote openers, so a `"""` that appears only inside a `#` comment no longer trips the rejection.
- Test: `TestWriteAllowlistSection_AcceptsCommentsContainingTripleQuotes` at `allowlist_test.go:619` includes (a) a top-of-file comment containing `"""`, (b) a trailing-comment `mage = "latest" # also """ in this comment` (real basic string preceding a comment containing triples), and (c) a comment with `'''` literal triples. The test asserts the rewrite succeeds AND the prefix bytes are preserved verbatim.
- Implementation is a byte walker, not a regex. Matches the spec.
- Verdict: fix is correct; the trailing-comment-after-real-string case is the load-bearing one and is covered.

**Hidden #4 — Exported mutable defaults.**

- Code: `internal/tools/allowlist.go:43` — `var defaultAllowlistHosts = []string{...}` is unexported. Doc comment at lines 29-42 explains the rationale: "Kept package-private to prevent external callers from mutating the global allowlist policy in place".
- Code: `internal/tools/allowlist.go:57-61` — exported `func DefaultAllowlistHosts() []string` allocates via `make([]string, len(defaultAllowlistHosts))` + `copy(out, defaultAllowlistHosts)` then returns `out`. The returned slice's backing array is independent of the package state.
- Test: `TestDefaultAllowlistHostsReturnsCopyNotMutableRef` at `allowlist_test.go:743` — first call returns the four built-in hosts, mutates `first[i] = "tampered.example.com"` for every index, then a second `DefaultAllowlistHosts()` call asserts `reflect.DeepEqual(second, want)` where `want` is the original four hosts. Finally `EffectiveAllowlist(AllowlistConfig{})` is re-checked to confirm the internal source-of-truth slice is not tampered.
- External-caller audit: `rg "DefaultAllowlistHosts|defaultAllowlistHosts"` finds 15 matches across exactly 2 files — `internal/tools/allowlist.go` (10 matches) + `internal/tools/allowlist_test.go` (5 matches). No external production callers, no external test callers. Renaming the var was safe.
- Verdict: fix is correct, backing-array isolation is verified by the in-place mutation + second-call equality assertion.

### Out-of-scope-edit audit

- `git diff HEAD~1 HEAD --stat` reports exactly three files: `internal/tools/allowlist.go`, `internal/tools/allowlist_test.go`, `drops/DROP_15_NETWORK_POLICY/BUILDER_WORKLOG.md`. Nothing outside `internal/tools/` was touched.
- `internal/services/images/overlay_test.go` was NOT modified — the existing `TestOverlayHash_AllowlistDataIgnored` continues to pass against the renamed `defaultAllowlistHosts` because it uses `tools.AllowlistConfig` directly and `OverlayHash` is `[tools]`-only.

### Mage results (reproduced independently)

`mage testPkg ./internal/tools`:

```
[INFO] Started go test -json (-count=1 -race -cover ./internal/tools)
[PKG PASS] github.com/evanmschultz/valv/internal/tools (1.22s)

Test summary
  tests: 93
  passed: 93
  failed: 0

  github.com/evanmschultz/valv/internal/tools | 91.9%
```

`mage testPkg ./internal/services/images`:

```
[INFO] Started go test -json (-count=1 -race -cover ./internal/services/images)
[PKG PASS] github.com/evanmschultz/valv/internal/services/images (1.28s)

Test summary
  tests: 63
  passed: 63
  failed: 0

  github.com/evanmschultz/valv/internal/services/images | 81.6%
```

Coverage threshold (60% per package; the project's wider `mage test` enforces 70%, also met for `internal/tools` at 91.9%) holds.

### Findings (informational, non-blocking)

- F8. The doc comment for `stripLineComment` at `allowlist.go:413-426` correctly notes that multi-line strings are deliberately NOT recognized by the helper — that responsibility stays in `splitAllowlistSpan`'s outside-span check on `bytes.Contains(codeOnly, []byte(\`"""\`))`. The split between "mask comments" and "detect triple-quote openers" is clean.
- F9. The fix for CE#1 extends the contract: per the new doc comment at `allowlist.go:296-301`, ALL top-level section headers must start at column 0, not just `[allowlist]`. This is a slightly broader rejection than R1's CE#1 strictly required, but the rationale ("the rewrite cannot keep the leading whitespace attached to the canonical replacement section") applies symmetrically. Documented + tested. Acceptable widening.
- F10. The 254-byte FQDN test fixture (`allowlist_test.go:727`) asserts its own length via `if len(host) != 254 { t.Fatalf(...) }` before exercising the validator. Self-checking test fixture — good defensive practice for boundary tests.
- F11. The CR-detection guard at `allowlist.go:307-309` is unchanged from R1 and remains correct (single `bytes.IndexByte(content, '\r')` catches CRLF AND solo CR). Not part of R1 findings but verified incidentally.

### Missing evidence

None. All four R1 counterexamples are mitigated at the code level, all four mitigations have boundary-case tests in `allowlist_test.go`, and both mage gates reproduce green. Builder discipline (mage-only, scoped edits) is clean.

### Summary

Verdict: **pass**.

Round 2 fixes the three QA-Falsification counterexamples (CE#1 indented section headers, CE#2 RFC 1123 length limits, CE#3 multi-line-string-vs-comment ambiguity) and the QA-Proof hidden-dep finding (#4 exported mutable defaults) cleanly. The `splitAllowlistSpan` rewrite tightens the contract uniformly across all top-level sections, the new `stripLineComment` is a correct byte-walker (basic-string escape handling + literal-string no-escape semantics + comment introducer outside strings), `validateHost` enforces RFC 1123 byte limits with specific error messages, and the public `DefaultAllowlistHosts()` accessor returns a fresh copy verified by in-place mutation. 7 new tests bring the package to 93 tests / 91.9% coverage. The `OverlayHash` regression pin in `internal/services/images` still passes. Unit 15.1 is ready to close.

## Unit 15.2 — Round 1

**Date:** 2026-05-24
**QA Proof backend:** claude-native (orchestrator dispatch, opus)
**Verdict:** `pass`

### Per-Acceptance Audit

**A1. Add typed request structs `NetworkCreateRequest` and `NetworkRemoveRequest`; do NOT add `NetworkConnectRequest`.**

- `NetworkCreateRequest` at `internal/adapters/docker/network.go:21-29` with fields `Name string`, `Internal bool`, `Labels map[string]string`. Doc comment at lines 16-20 explicitly states "there is no `NetworkConnectRequest` symbol in this drop" per Schema Decision 5.
- `NetworkRemoveRequest` at `internal/adapters/docker/network.go:77-80` with field `Name string`.
- Forbidden-symbol audit: `rg --glob '*.go' "ConnectNetwork|NetworkConnect|BuildNetworkConnect"` across the entire repo returns ZERO matches in any `.go` file. The only mentions are deliberate negative statements in the drop dir (`BUILDER_WORKLOG.md:25,41-43`, `PLAN.md:121`) and a single doc-comment line at `internal/adapters/docker/network.go:20` documenting the absence. Verdict: Schema-Decision-5 cut applied verbatim.

**A2. Add arg builders `BuildNetworkCreateArgs` and `BuildNetworkRemoveArgs` in `internal/adapters/docker/network.go`.**

- `BuildNetworkCreateArgs` at `network.go:51-74` — calls `request.Valid()` first, then emits `["network", "create", (maybe "--internal"), (sorted --label k=v pairs), <name>]`.
- `BuildNetworkRemoveArgs` at `network.go:96-101` — calls `request.Valid()`, returns `["network", "rm", <trimmed-name>]`.
- Validators `NetworkCreateRequest.Valid()` (lines 32-46) and `NetworkRemoveRequest.Valid()` (lines 83-92) both wrap errors with `"validate network ... request: ..."` boundary prefix.

**A3. Add `Executor.CreateNetwork` and `Executor.RemoveNetwork` in `internal/adapters/docker/executor.go`.**

- `Executor.CreateNetwork(ctx, request)` at `executor.go:46-52` — `context.Context` first param; calls `BuildNetworkCreateArgs(request)` then `e.runner.Run(ctx, args)`. Mirrors the existing `Build` / `RemoveImage` / `PruneBuilder` pattern at lines 5-41 verbatim.
- `Executor.RemoveNetwork(ctx, request)` at `executor.go:57-63` — same shape. Both methods bubble validation errors from the arg-builder without wrapping (so callers see the underlying `"validate network ..."` prefix directly).
- `Executor` type at `internal/adapters/docker/command.go:8-14`; `CommandRunnerFunc` at `internal/adapters/docker/types.go:127-131`. Both seams predate this unit and are reused unchanged.

**A4. `BuildNetworkCreateArgs` emits `docker network create --internal ...`.**

- Code: `network.go:57-59` — `if request.Internal { args = append(args, "--internal") }`. Conditional on the `Internal bool` field at struct line 26.
- Tests pinning the `--internal` arg appearance:
  - `TestBuildNetworkCreateArgs/internal network with default valv label` at `network_test.go:19-32` — asserts `--internal` is the third arg after `"network", "create"`.
  - `TestBuildNetworkCreateArgs/multiple labels sorted deterministically` at lines 34-54 — same.
  - `TestBuildNetworkCreateArgs/no labels emits only required args` at lines 68-79 — asserts `["network", "create", "--internal", "valv-bare"]` exactly.
- Tests pinning the `--internal` arg ABSENCE when `Internal: false`:
  - `TestBuildNetworkCreateArgs/internal false omits flag` at lines 56-67 — asserts `["network", "create", "--label", "valv=network-policy", "valv-open-net"]` with NO `--internal`.

**A5. Tests cover required-field validation, `--internal` emission, and deterministic arg order.**

- Required-field validation:
  - `empty name rejected` (`network_test.go:93-96`) → "name is required".
  - `whitespace-only name rejected` (lines 98-101) → "name is required".
  - `label with empty key rejected` (lines 122-130) → "label key is required".
  - `RemoveArgs/empty name rejected` (lines 177-180) + `whitespace name rejected` (lines 182-185).
- `--internal` emission: A4 above.
- Deterministic arg order:
  - `TestBuildNetworkCreateArgs/multiple labels sorted deterministically` at `network_test.go:34-54` — input map has keys `{zebra, alpha, valv, project}`; assertion at lines 46-53 requires `alpha → project → valv → zebra` in lexicographic order. The implementation sorts keys at `network.go:62-67` via `sort.Strings(keys)`.
  - Confirms `BuildNetworkCreateArgs` is map-iteration-stable.
- Network name validation regex (`dockerNetworkNamePattern = ^[a-zA-Z0-9][a-zA-Z0-9_.-]*$` at `network.go:14`):
  - Acceptance: hyphens + dots + underscores (`network_test.go:81-91`).
  - Rejection: space (`104-106`), slash (`108-111`), leading hyphen (`113-116`), leading dot (`118-121`). All return "invalid network name".

### Executor forwarding audit

- `TestExecutorCreateNetworkForwardsArgs` at `network_test.go:217-243` — installs `CommandRunnerFunc` that captures `args` (`got = append([]string(nil), args...)`), invokes `exec.CreateNetwork(ctx, ...)`, asserts the captured args equal `["network", "create", "--internal", "--label", "valv=network-policy", "valv-netpolicy-abc"]`. Proves the executor forwards verbatim what the arg builder produced.
- `TestExecutorCreateNetworkReturnsBuildError` at lines 245-260 — installs runner with `t.Fatal("runner should not be invoked when build fails")`, invokes with `NetworkCreateRequest{Name: ""}`, asserts the validation error contains "name is required". Proves the executor SHORT-CIRCUITS on `BuildNetworkCreateArgs` error and does NOT call the runner.
- Mirror tests for remove: `TestExecutorRemoveNetworkForwardsArgs` (262-279), `TestExecutorRemoveNetworkReturnsBuildError` (281-296).
- `context.Context` is the first parameter of both executor methods (matches CLAUDE.md § "Go Development Rules" context-propagation rule). `context.Background()` is passed in the tests; `_ context.Context` is ignored inside the mock CommandRunnerFunc, which is fine because the executor-forwarding contract is "build args + forward to runner", and `Run(ctx, args)` is the runner-side seam that does the actual ctx propagation.

### Mage Results (run by QA Proof)

```
mage testPkg ./internal/adapters/docker
[PKG PASS] github.com/evanmschultz/valv/internal/adapters/docker (2.24s)
  tests: 51
  passed: 51
  failed: 0
  package coverage: 67.8% (above 60.0% gate)
```

Reproduces the builder-claimed 51-test / 67.8%-coverage signal exactly.

### Scope Compliance

`git show --stat d128363` returns exactly:

- `internal/adapters/docker/executor.go` (+22 lines — added `CreateNetwork` / `RemoveNetwork` methods)
- `internal/adapters/docker/network.go` (+101 lines — new file)
- `internal/adapters/docker/network_test.go` (+296 lines — new file)

No edits to `internal/cli/`, `internal/services/`, `internal/tools/`, `internal/adapters/sqlite/`, `internal/adapters/providers/`, drop dir, or anywhere else. Unit 15.2's hard-constraint compliance ("only edits inside `internal/adapters/docker/`") holds. PLAN.md was state-bit flipped to `done` in a separate commit per drop-state convention — confirmed via `git status` clean.

### Findings

- F1. Network-name regex matches Docker's actual accepted shape. Doc comment at `network.go:10-13` cites the "docker network create reference" as source. Pattern `^[a-zA-Z0-9][a-zA-Z0-9_.-]*$` rejects leading hyphens (which would collide with Docker CLI flag-parsing), whitespace, and path separators while allowing the typical `valv-netpolicy-<hash>` shape. Tests cover both directions of the boundary.
- F2. Label emission uses `sort.Strings` over the key set, which is the canonical Go pattern for deterministic map iteration. Matches existing `BuildBuilderPruneArgs` / `BuildImageArgs` style in the same package — no new pattern introduced.
- F3. Label key validation only rejects empty/whitespace keys; values are accepted as-is (including empty values). This matches Docker's own `--label key=value` behavior where an empty value is legal (`--label valv=`). Defensible — not in the acceptance bullet, but a reasonable design.
- F4. Both validators trim the name (`strings.TrimSpace`) before regex match AND before emitting it as the positional arg (`network.go:72`, `network.go:100`). Surrounding whitespace is silently stripped — this is documented behavior because `name = strings.TrimSpace(r.Name)` is the canonical form fed to the regex.
- F5. `Valid()` is a public method on both request types, which means external callers can pre-validate before constructing a builder call. This is consistent with the existing `ContainerRunRequest.Valid()` / `ImageBuildRequest.Valid()` pattern in `types.go`.
- F6. `Internal: false` path is tested (`network_test.go:56-67`) and produces a network with default (bridge) connectivity. This is not the path DROP_15 actually uses (Schema Decision 5 always sets `Internal: true`) but the unit's interface keeps the flag flexible, and the test pins the omission behavior so a future caller toggling the flag works as advertised.
- F7. Coverage drift from package-baseline: the file `internal/adapters/docker` package was previously above the 60% gate. Adding `network.go` (101 lines, mostly covered) plus 4 executor methods (CreateNetwork / RemoveNetwork are tested directly, but the existing `Build` / `RemoveImage` / `PruneBuilder` etc. cover paths are unchanged) brings the package to 67.8% — well above the gate. The builder's claim is reproduced.

### Missing Evidence

None. All five PLAN.md acceptance bullets (A1–A5) are satisfied with file:line citations, scope is clean per `git show --stat d128363` (3 files all inside `internal/adapters/docker/`), and the mage gate reproduces independently at 51 tests / 67.8% coverage.

### Summary

Verdict: **pass**.

Unit 15.2 cleanly adds Docker network create/remove lifecycle helpers (typed `NetworkCreateRequest` + `NetworkRemoveRequest`, deterministic arg builders, Executor methods) entirely inside `internal/adapters/docker/`. Schema-Decision-5 cut applied verbatim: no `NetworkConnectRequest`, no `BuildNetworkConnectArgs`, no `Executor.ConnectNetwork` anywhere in the repo's Go sources. `--internal` flag emission, deterministic label sorting, and required-field validation are each pinned by dedicated test cases. Executor forwarding is verified through a `CommandRunnerFunc` mock that proves args round-trip verbatim and validation errors short-circuit before the runner is invoked. `mage testPkg ./internal/adapters/docker` reproduces 51 tests passing at 67.8% coverage. Hard-constraint compliance (only edits inside `internal/adapters/docker/`) holds. Ready for QA Falsification review.

## Unit 15.2 — Round 2

**Date:** 2026-05-24
**QA Proof backend:** claude-sonnet-4-6 (Build-QA agent, both passes)
**Verdict:** `pass`

### R1 Finding Verification

**Finding #1 — Label key with surrounding whitespace accepted by `Valid()`.**

- Fix at `internal/adapters/docker/network.go:47-50`: after the existing `strings.TrimSpace(key) == ""` empty-key check (line 44-46), a second guard checks `strings.TrimSpace(key) != key` and returns `"label key must not have leading or trailing whitespace"`. This fires for any key where the trimmed form differs from the raw form — catches leading space, trailing space, both, and tab.
- Test at `network_test.go:298-346` (`TestNetworkCreateRequestRejectsUntrimmedLabelKey`): 4 sub-tests — leading whitespace, trailing whitespace, both sides, tab (`"\tvalv\t"`). All assert `"leading or trailing whitespace"` in the error.
- The original R1 falsification counterexample (`"  valv  "` key) is explicitly covered by the third sub-test ("both sides rejected" at line 313-319). Counterexample is mitigated.

**Finding #2 — Label key containing `=` not rejected.**

- Fix at `network.go:51-54`: `strings.ContainsRune(key, '=')` check after the whitespace guard. Error message: `"label key must not contain '='"`.
- Test at `network_test.go:348-387` (`TestNetworkCreateRequestRejectsLabelKeyWithEquals`): 3 sub-tests — `=` in middle (`"k=injected"`), `=` at start (`"=k"`), multiple equals (`"k=v=w"`). All assert the error contains `"="`.
- Assertion at line 382 checks `strings.Contains(err.Error(), "=")`. The actual error message is `"label key must not contain '='"` which contains `=`. PASSES.
- R1 counterexample (`"k=injected"` key silently emitting `--label k=injected=value`) is mitigated.

**Finding #3 — No maximum network name length enforced.**

- Fix at `network.go:40-42` (`NetworkCreateRequest.Valid()`) and `network.go:102-104` (`NetworkRemoveRequest.Valid()`): `if len(name) > 64` returns `"network name must be at most 64 bytes, got N"`. Applied to BOTH request types per builder claim.
- Length check fires AFTER the regex match (line 37-39), so the regex already guarantees the chars are ASCII-only — meaning `len()` byte count == rune count for any name that passes the regex. No multi-byte concern.
- Test at `network_test.go:389-440` (`TestNetworkCreateRequestRejectsOverlongName`): 3 sub-tests — 64 bytes accepted (boundary: `"a" + strings.Repeat("b", 62) + "c"` = 64 chars), 65 bytes rejected, 256 bytes rejected. The 64-byte test has `shouldAccept: true` and asserts `err == nil`. The `> 64` operator at line 40 means 64 accepts, 65 rejects — CORRECT boundary.
- Operator: `len(name) > 64` → 64 passes, 65 fails. Test fixture confirmed: `"a" + strings.Repeat("b", 62) + "c"` = 1+62+1 = 64 bytes. VERIFIED.

**NetworkRemoveRequest 64-byte enforcement:**

- Builder claims it also applies to `NetworkRemoveRequest`. Verified at `network.go:102-104`. No dedicated test for this in Round 2 (the R2 tests only cover `NetworkCreateRequest`). The `TestBuildNetworkRemoveArgs` table (lines 157-214) does not include a 65-byte name case. This is a minor gap documented under Findings.

### Mage Gate (independently run)

```
mage testPkg ./internal/adapters/docker

[INFO] Started go test -json (-count=1 -race -cover ./internal/adapters/docker)
[PKG PASS] github.com/evanmschultz/valv/internal/adapters/docker (2.27s)

Test summary
  tests: 64
  passed: 64
  failed: 0
  skipped: 0

  github.com/evanmschultz/valv/internal/adapters/docker | 65.7%
  Minimum package coverage: 60.0%.
  [SUCCESS] Coverage threshold met
```

Builder claimed 64 tests / 65.7% coverage. QA independently confirmed: **64 tests / 65.7% coverage**, -race enabled. Gate is GREEN.

### Scope Compliance

`git show --stat HEAD` for commit `70e10cd` returns exactly:

- `drops/DROP_15_NETWORK_POLICY/BUILDER_WORKLOG.md`
- `internal/adapters/docker/network.go` (+14 lines — three validation guards)
- `internal/adapters/docker/network_test.go` (+144 lines — three new test functions)

No edits to `executor.go` (builder's claim: "No edits to executor.go" — confirmed by `git show --stat HEAD`). No edits to `internal/cli/`, `internal/services/`, `internal/tools/`, or anywhere outside the declared paths. Hard-constraint compliance holds.

### Findings

- F1. **`NetworkRemoveRequest` 64-byte limit has no dedicated test.** The validation code is present at `network.go:102-104`, but no Round 2 test exercises a 65-byte name via `BuildNetworkRemoveArgs`. The guard exists; the test pin does not. This is a minor coverage gap — not a correctness bug — because any name that reaches the 64-byte check in `NetworkCreateRequest` would similarly hit it in `NetworkRemoveRequest`. Risk: accepted with NIT routing.
- F2. **Validation order in `NetworkCreateRequest.Valid()`.** The length check at line 40 runs AFTER the regex match at line 37. The regex `^[a-zA-Z0-9][a-zA-Z0-9_.-]*$` rejects all non-ASCII chars, so `len()` (byte count) equals rune count for any name passing the regex. No multi-byte divergence possible. Order is safe.
- F3. **Label-key validation runs BEFORE `=` injection is possible.** `BuildNetworkCreateArgs` calls `request.Valid()` first (line 63-65) and only proceeds to `fmt.Sprintf("%s=%s", key, ...)` (line 79) if `Valid()` returned nil. So a key containing `=` is rejected at validation time before any arg is built. Clean.
- F4. **Existing 51 R1 tests still pass.** Test count went from 51 (R1) to 64 (R2), all passing. No regressions introduced.

### Missing Evidence

None that is verdict-affecting. All three R1 findings have code-level fixes with tests at the exact counterexample inputs. The mage gate independently reproduces. The `NetworkRemoveRequest` 64-byte test gap is informational (F1 above).

### Summary

Verdict: **pass**.

All three Round 1 falsification findings are fixed with narrow, correctly-bounded validation guards in `NetworkCreateRequest.Valid()` and `NetworkRemoveRequest.Valid()`. New tests cover the label-key whitespace (4 sub-tests including tab), label-key `=` injection (3 sub-tests), and network name 64-byte limit (boundary accept + 65- and 256-byte rejection). The mage gate independently confirms 64 tests / 65.7% coverage with `-race` enabled. Scope is limited to the declared paths (`network.go` + `network_test.go` + drop worklog). Unit 15.2 is ready to close.

## Unit 15.2 — Round 3

**Verdict:** pass-with-nits
**Reviewer:** `ta-go-build-qa-proof` (built-in, sonnet, read-only persona — verdict transcribed by orchestrator).
**Reviewed at:** 2026-05-26

### Scope

Commit `d4ca96a` — additive network-connect surface for the proxy sidecar (after the `ce8e02c` re-plan reset 15.2 from its R2-green create/remove state). Touched: `internal/adapters/docker/network.go` (+74), `executor.go` (+11), `network_test.go` (+194), PLAN.md (state flip), BUILDER_WORKLOG.md. Acceptance: `PLAN.md:127-132`.

### Mage gate (proof-agent run, independent)

- `mage testPkg ./internal/adapters/docker` — 82/82 pass, 67.9% coverage, `-race` clean. GREEN at the 60% project floor.

### Per-acceptance audit (all five bullets MET, file:line)

- R2 surface UNCHANGED: `NetworkCreateRequest`/`BuildNetworkCreateArgs`/`NetworkRemoveRequest`/`BuildNetworkRemoveArgs` (`network.go:16-114`), `Executor.CreateNetwork`/`RemoveNetwork`/`ListNetworks` (`executor.go:50-88`); `BuildNetworkCreateArgs` still emits `--internal`. (One obsolete doc-comment sentence trimmed — cosmetic.)
- `NetworkConnectRequest{Network (req), Container (req), Aliases []string (opt)}` — `network.go:120-128`.
- `BuildNetworkConnectArgs` (`network.go:164-185`): `["network","connect"]`, `sort.Strings` alias order, one `--alias <a>` per alias, positional `TrimSpace(Network), TrimSpace(Container)` last. Validation (`network.go:131-158`) mirrors create/remove: non-empty + pattern + length on network; non-empty container (no pattern — correct, container IDs are SHA hashes); each alias non-empty + pattern.
- `Executor.ConnectNetwork` (`executor.go:72-78`) mirrors `CreateNetwork`: build args → return err → `e.runner.Run`.
- Tests: 14-case `TestBuildNetworkConnectArgs` (missing network, missing container, empty/whitespace/invalid alias, single + sorted-multiple emission, positional tail, trim, 64-byte limit) + `TestExecutorConnectNetworkForwardsArgs` + `TestExecutorConnectNetworkReturnsBuildError`.

### NIT (pre-existing, non-blocking)

- N1: package coverage 67.9% < CLAUDE.md's 70% aspiration. The mage gate is set to 60% via a documented TODO (`magefile.go:23`); this is pre-existing project debt tracked for DROP_17 / Unit 11.5 (coverage-floor bump), not introduced by 15.2-R3. Gate is green.

**Verdict: pass-with-nits** — all five acceptance bullets met; gate green; sole NIT is tracked pre-existing coverage debt.

## Unit 15.2.5.A — Round 1

**Date:** 2026-06-02
**QA Proof backend:** claude-native (orchestrator dispatch, opus)
**Verdict:** `pass`
**Commit:** `b22cdc7` (`git diff b22cdc7~1 b22cdc7`)

### Path Discipline

`git diff b22cdc7~1 b22cdc7 --stat` touches exactly: `internal/adapters/docker/executor.go` (+24), `internal/services/networkpolicy/service.go` (+8), `internal/adapters/docker/network_test.go` (+95), `internal/services/networkpolicy/service_test.go` (+22/-3), plus drop-dir `PLAN.md` (state flip) + `BUILDER_WORKLOG.md`. Production files match the spec's declared `paths` exactly (`executor.go` + `service.go`). No out-of-scope production edits.

### Per-Acceptance Audit

**A1. `Executor.RunContainerDetached(ctx, ContainerRunRequest) (string, error)` exists, reuses `BuildRunArgs` + `ContainerRunRequest.Detached`, returns trimmed container id via runner output path, wraps command errors.**

- `internal/adapters/docker/executor.go:75-91`: signature `func (e Executor) RunContainerDetached(ctx context.Context, request ContainerRunRequest) (string, error)`.
- Reuses `BuildRunArgs` at line 82 (`args, err := BuildRunArgs(request)`). `BuildRunArgs` → `buildRunLikeArgs("run", request, true)` (`types.go:133-134`) which emits `-d` when `request.Detached` is set (`types.go:147-149`). The `Detached` reuse is structural — the method does not re-derive the flag.
- Returns trimmed id: line 90 `return strings.TrimSpace(out), nil` over the `outputter.Output` result (line 86). Correct: docker `run -d` appends a newline to the printed container id.
- Command-error path: line 86-89 returns the runner's `Output` error directly (`out, err := outputter.Output(...); if err != nil { return "", err }`). PASS.

**A2. `outputter`/`ErrOutputUnsupported` path correct (sentinel when runner lacks Output).**

- `executor.go:76-81`: runtime type-asserts `e.runner` to `interface{ Output(context.Context, []string) (string, error) }`; `if !ok { return "", ErrOutputUnsupported }`. Mirrors the committed `RemoveContainer` (line 30-32) and `ListNetworks` (line 113-117) patterns exactly. `ErrOutputUnsupported` is the package sentinel at `types.go:11`. The check fires BEFORE `BuildRunArgs`, so a non-outputting runner returns the sentinel even with a valid request (matches `TestExecutorRunContainerDetachedOutputUnsupported`, which uses a valid `valv-proxy:latest` req + `CommandRunnerFunc`). PASS.

**A3. `NetworkExecutor` interface includes BOTH `RunContainerDetached` AND `ConnectNetwork`; signatures match concrete `docker.Executor` exactly.**

- `service.go:58-73`: interface now lists `CreateNetwork`/`RemoveNetwork`/`ListNetworks` (pre-existing) + `RunContainerDetached(ctx context.Context, request docker.ContainerRunRequest) (string, error)` (line 68) + `ConnectNetwork(ctx context.Context, request docker.NetworkConnectRequest) error` (line 72).
- Signature match vs concrete: `Executor.RunContainerDetached` (`executor.go:75`) = `(ctx context.Context, request ContainerRunRequest) (string, error)` — identical modulo the `docker.` qualifier from the consumer package. `Executor.ConnectNetwork` (`executor.go:96`) = `(ctx context.Context, request NetworkConnectRequest) error` — identical. Exact-match is machine-proven by the compile-time guard (A4) + green gate. PASS.

**A4. `fakeNetworkExecutor` satisfies the widened interface; `var _ NetworkExecutor = docker.Executor{}` guard present and valid.**

- `service_test.go:71-77` (`RunContainerDetached`) + `:79-82` (`ConnectNetwork`) add the two new stubs; the fake records `runDetachedCalls`/`connectCalls` and honors `runDetachedErr`/`runDetachedResult` fields (`:24-32`). All five interface methods present on the fake.
- Compile-time guard at `service_test.go:573`: `var _ NetworkExecutor = docker.Executor{}`. This is a hard compile assertion that the production `docker.Executor` satisfies the WIDENED interface (incl. both new methods). It compiled — `mage testPkg ./internal/services/networkpolicy` GREEN proves both the fake and the production adapter satisfy the interface. PASS.

**A5. The 4 new tests genuinely cover happy-path id return, output-error wrapping, ErrOutputUnsupported, build/validation error.**

- `TestExecutorRunContainerDetachedReturnsContainerID` (`network_test.go:646-665`): `outputRunner{output: "abc123def456\n"}` → asserts trimmed `"abc123def456"`. Covers happy-path id return + trim. GENUINE.
- `TestExecutorRunContainerDetachedOutputError` (`:667-685`): `outputRunner{outErr: sentinel}` → asserts `errors.Is(err, sentinel)`. Covers output-error propagation. GENUINE (note: the implementation returns the runner error verbatim, not `%w`-wrapped — `errors.Is` still holds for an identity-returned sentinel; the test is correct for the actual contract).
- `TestExecutorRunContainerDetachedOutputUnsupported` (`:687-704`): `CommandRunnerFunc` (no `Output`) + valid req → asserts `errors.Is(err, ErrOutputUnsupported)`. Covers the sentinel branch. GENUINE.
- `TestExecutorRunContainerDetachedBuildError` (`:706-728`): empty `Image.Repository` → asserts substring `"image is required"` (the `ContainerRunRequest.Valid()` error at `types.go:110`). Covers validation-before-runner. GENUINE — and the empty-repo path is reached only because the outputter check passes first (the `outputRunner` does implement `Output`), so this genuinely exercises the `BuildRunArgs` error return at `executor.go:82-85`.
- `outputRunner` helper (`:638-645`) implements both `Run` and `Output`, so the build-error test isolates the validation path. All 4 are real, non-tautological. PASS.

### Gate Re-Run (QA-verified, not builder-trusted)

- `mage testPkg ./internal/adapters/docker` → **86 tests, 86 passed, 0 failed** (matches worklog claim of 86; baseline 82 + 4 new).
- `mage testPkg ./internal/services/networkpolicy` → **23 tests, 23 passed, 0 failed** (count unchanged — fake stubs are compile-time only, as the worklog states).

### Budget / Symbol Grounding

- 3 production symbols: `Executor.RunContainerDetached` (new) + `NetworkExecutor.RunContainerDetached` + `NetworkExecutor.ConnectNetwork` (interface method additions). At the 3-symbol ceiling but within it. `ConnectNetwork` on the concrete `docker.Executor` was pre-existing (`executor.go:96`, committed R2) — this unit only exposed it on the consumer interface, not re-implemented. Production LOC delta +32 (executor +24, service +8), 2 production files. Under the 80-LOC / 3-file budget.

### Verdict

`verdict: pass` — all five acceptance items proven against committed code at `b22cdc7` with file:line evidence; both mage gates re-run GREEN; path discipline clean; interface↔concrete signature match machine-proven by the compile-time guard.

---

## Unit 15.2.5.C — Round 1

**Date:** 2026-06-02
**QA backend:** claude-opus (build-qa-proof, orchestrator dispatch)
**Committed at:** `4d20566` (`git diff 4d20566~1 4d20566`)
**Mage gate:** `mage testPkg ./internal/services/networkpolicy` → 27 tests pass, 0 fail (re-run by QA, GREEN)

### Acceptance Criterion 1 — buildNoProxy returns ONLY loopback + sidecar alias, deterministic, no allowlist leak

`service.go:278-281`:

```go
func buildNoProxy() string {
	entries := []string{"127.0.0.1", "localhost", "valv-proxy"}
	sort.Strings(entries)
	return strings.Join(entries, ",")
}
```

- Signature is now parameterless (`buildNoProxy()`), so no input path can introduce an allowlist host. The output is a static slice of exactly `127.0.0.1`, `localhost`, `valv-proxy`.
- `sort.Strings` over a fixed slice yields a deterministic `"127.0.0.1,localhost,valv-proxy"` on every call. No input → no input-dependent variation possible. PROVEN.

### Acceptance Criterion 2 — call site + doc comments corrected; Allowlist still used where it should be

- Call site `service.go:225`: `NoProxy: buildNoProxy(),` — no longer `buildNoProxy(request.Allowlist)`. PROVEN by diff hunk.
- `PolicyMaterial.NoProxy` doc `service.go:144-152` (diff): now states it contains "ONLY loopback addresses (localhost, 127.0.0.1) and the sidecar proxy alias (valv-proxy)" and that "Allowlist hosts are intentionally absent". Matches corrected semantics. PROVEN.
- `ProvisionRequest.Allowlist` doc `service.go:~97-105` (diff): rewritten to state allowlist feeds "network-name derivation … but does NOT place allowlist entries in NO_PROXY". PROVEN.
- Allowlist STILL correctly used: `service.go:187` `desiredName := networkName(request.Allowlist)` (network-name hash) and `networkName` at `service.go:258-264` hashes the sorted allowlist. The allowlist is therefore retained for network naming (and per PLAN §356 the proxy filter), removed only from NO_PROXY. PROVEN.

### Acceptance Criterion 3 — table-driven tests + Provision leak test

- `service_test.go:499` `TestBuildNoProxy` — table-driven, 3 cases (`empty_allowlist`, `non_empty_allowlist_no_leak`, `loopback_entries_already_covered`), each asserts `buildNoProxy() == "127.0.0.1,localhost,valv-proxy"`. Covers empty/non-empty allowlist + loopback presence per acceptance. PROVEN.
- `service_test.go:537` `TestBuildNoProxy_AllowlistHostsNeverLeak` — end-to-end: calls `svc.Provision(...)` with a 5-host allowlist (`service_test.go:563-570`), asserts `NoProxy == "127.0.0.1,localhost,valv-proxy"` AND loops `strings.Contains(material.NoProxy, host)` over all 5 hosts asserting none leak. This is the Provision leak test. PROVEN.
- `service_test.go:637` `TestBuildNoProxy_LoopbackAndSidecarAlwaysPresent` — regression pin: confirms each of the three fixed entries is present.
- `TestProvision_CreatesNetworkOnFreshHost` `service_test.go:157,211-217` — `wantNoProxy` updated to `"127.0.0.1,localhost,valv-proxy"` and adds a loop asserting no allowlist host leaks. PROVEN.

### Path Discipline

`git diff --stat 4d20566~1 4d20566`: only `service.go` + `service_test.go` (declared `paths`) + drop mds (PLAN.md state flip, BUILDER_WORKLOG.md). No out-of-scope code. `NetworkExecutor`, `Service.Provision` flow, `networkName`, `CleanupStale` byte-identical except the single `buildNoProxy()` call-site change. CLEAN.

### Verdict

`verdict: pass` — all three acceptance criteria proven against committed code at `4d20566` with file:line + quoted evidence; allowlist correctly retained for `networkName` hash and removed only from NO_PROXY; mage gate re-run GREEN (27/27); path discipline clean.

## Unit 15.2.5.B.1 — Round 1

`verdict: pass`

Committed at `ea18fc5` (`feat(valv-proxy): allowlist matcher (parseAllowlist + hostAllowed)`). Scope: `internal/cmd/valv-proxy/allowlist.go` (53 LOC) + `allowlist_test.go` (188 LOC). Re-confirmed independently: `mage testPkg ./internal/cmd/valv-proxy` → 27/27 PASS, 0 fail.

### Criterion 1 — parseAllowlist comma-split + lowercase + trim + dedup + discard-empty (Schema Decision 1: exact-host, lowercase, deduped)

`allowlist.go:18-34`. `strings.Split(raw, ",")` (line 19) comma-splits; per entry `h := strings.ToLower(strings.TrimSpace(p))` (line 23) lowercases + trims; `if h == "" { continue }` (line 24) discards empties (leading/trailing/consecutive commas, whitespace-only); `seen` map (lines 20,27-30) dedups; `out` preserves first-appearance order. Matches Schema Decision 1 (PLAN.md:33 "exact-host, lowercase, deduped"). PROVEN.

### Criterion 2 — hostAllowed strips :port, case-folds, EXACT-match (no wildcard/subdomain/CIDR)

`allowlist.go:41-53`. `host, _, hasPort := strings.Cut(target, ":")` + `if !hasPort { host = target }` (lines 42-45) strips `:port`; `host = strings.ToLower(host)` (line 46) case-folds; loop `if a == host { return true }` (lines 47-51) is exact string equality only — no `strings.HasSuffix`, no wildcard glob, no `net.ParseCIDR`. So `api.github.com` ≠ `github.com` and `*.github.com` never matches. PROVEN.

### Criterion 3 — test coverage of all required cases

`allowlist_test.go`:
- exact match — `TestHostAllowed` "exact match" (95-98) → true.
- case-insensitivity — "case insensitive target upper" (110-113), "...mixed with port" (115-118); `TestParseAllowlist` "lowercases entries" (35-38).
- port-stripping `github.com:443`→`github.com` — "exact match with port stripped" (100-103), "exact match port 80" (105-108).
- wildcard NON-match — "wildcard pattern is not matched" `*.github.com`→false (130-133).
- subdomain NON-match `api.github.com`≠`github.com` — "subdomain does not match parent" (120-123) + with port (125-128) → false.
- empty/blank input — `TestParseAllowlist` "empty string" (15-18), "blank whitespace only" (20-23); `TestHostAllowed` "empty target" (145-148); empty-list `TestHostAllowed_EmptyAllowedList` (172-180) + nil `TestHostAllowed_NilAllowedList` (183-188).
- dedup — "deduplicates exact entries" (45-48), "deduplicates case-folded entries" (50-53), "empty entries from consecutive commas discarded" (55-58).

All required cases present and GREEN. PROVEN.

### Path Discipline

`git diff --stat ea18fc5^ ea18fc5 -- internal/cmd/valv-proxy/`: only `allowlist.go` + `allowlist_test.go` (declared `paths`). Full commit also touches PLAN.md (state flip todo→done) + BUILDER_WORKLOG.md. No out-of-scope code. CLEAN.

### Tools Used

- `git show --stat ea18fc5`; `git diff --stat ea18fc5^ ea18fc5 -- internal/cmd/valv-proxy/`
- `Read` allowlist.go (53 LOC), allowlist_test.go (188 LOC), PLAN.md (Schema Decision 1 line 33, unit spec line 378), BUILDER_WORKLOG.md (502-570), WORKFLOW.md
- `wc -l` allowlist.go=53, allowlist_test.go=188
- `mage testPkg ./internal/cmd/valv-proxy` → 27/27 PASS
- `grep` for unit refs + section headings

### Verdict

`verdict: pass` — all three acceptance criteria proven against committed code at `ea18fc5` with file:line + quoted evidence; matcher is exact-host/lowercase/port-stripped/no-wildcard per Schema Decision 1; all required test cases present; mage gate re-run GREEN (27/27); path discipline clean.

## Unit 15.2.5.B.2 — Round 1

**verdict: pass**

Committed at `2f70e89` (`feat(valv-proxy): fail-closed CONNECT/HTTP allowlist proxy server`). Files: `internal/cmd/valv-proxy/main.go` (103 LOC) + `internal/cmd/valv-proxy/main_test.go` (255 LOC), both new. `allowlist.go` (53 LOC, B.1) untouched — `hostAllowed`/`parseAllowlist` consumed as the committed B.1 contract.

### Acceptance Criterion 1 — `hostAllowed` checked BEFORE any network I/O on both paths; denied → 403/close, no dial

**CONNECT path.** `main.go:30-35` — the first statement inside the CONNECT branch is the allowlist check, before any `WriteHeader`, `Hijack`, or `Dial`:

```go
if r.Method == http.MethodConnect {
    if !hostAllowed(r.Host, allowed) {
        http.Error(w, "forbidden by allowlist", http.StatusForbidden)
        return
    }
    w.WriteHeader(http.StatusOK)   // line 36 — only reached on allow
    ...
    targetConn, err := net.Dial("tcp", r.Host)  // line 50 — dial only after allow
```

The `return` at line 34 guarantees no `net.Dial` (line 50) executes on deny. 403 via `http.StatusForbidden`.

**Plain HTTP path.** `main.go:77-80` — first statement after the CONNECT branch, before the reverse-proxy dial:

```go
if !hostAllowed(r.Host, allowed) {
    http.Error(w, "forbidden by allowlist", http.StatusForbidden)
    return
}
...
httputil.NewSingleHostReverseProxy(target).ServeHTTP(w, r)  // line 87 — only on allow
```

`hostAllowed` is the committed B.1 matcher (`allowlist.go:41-53`): port-stripped (`strings.Cut(target, ":")`), lowercased, exact-match, no wildcards — so `r.Host` of form `github.com:443` is correctly normalized. No host pre-processing before the call (B.1 owns port-strip). FAIL-CLOSED: every path not passing `hostAllowed` returns 403 or silently closes.

### Acceptance Criterion 2 — CONNECT: 200 + hijack + net.Dial + bidirectional io.Copy with correct goroutine/WaitGroup/CloseWrite teardown

`main.go:36-73`:
- `w.WriteHeader(http.StatusOK)` (line 36) + optional `Flusher.Flush()` (37-39).
- Hijack via `w.(http.Hijacker)` (40-47); non-hijackable or hijack error → `return` (silent close), no dial. `defer clientConn.Close()` (48).
- `net.Dial("tcp", r.Host)` (50); dial error → `return` (52), `defer targetConn.Close()` (54).
- Bidirectional copy: `wg.Add(2)` (57), two goroutines each `defer wg.Done()` (58-71), `io.Copy(targetConn, clientConn)` and `io.Copy(clientConn, targetConn)`. Each goroutine calls `CloseWrite()` on its destination `*net.TCPConn` after the copy completes (61-63, 68-70) so the peer sees EOF and its copy terminates. `wg.Wait()` (72) blocks until both finish before the deferred closes fire — no premature teardown.

Test proof: `TestNewProxyHandler_CONNECT_AllowedHostTunnels` (`main_test.go:39-81`) stands up a real TCP backend, issues a raw CONNECT, asserts 200 (69-71), and reads a sentinel back through the tunnel (74-80) — proves the full hijack+dial+copy round-trip.

### Acceptance Criterion 3 — main() reads VALV_PROXY_ADDR (default :8080) + VALV_PROXY_ALLOWLIST, parses via parseAllowlist, serves the handler

`main.go:91-103`:
```go
addr := os.Getenv("VALV_PROXY_ADDR")
if addr == "" { addr = ":8080" }                          // default :8080 (line 92-95)
allowed := parseAllowlist(os.Getenv("VALV_PROXY_ALLOWLIST"))  // line 96
...
http.ListenAndServe(addr, newProxyHandler(allowed))       // line 99
```
Matches PLAN.md line 376/379 env contract exactly. `parseAllowlist` is the committed B.1 helper. Handler injected with the parsed slice (testable without env reads).

### Acceptance Criterion 4 — test coverage

| Required case | Test | Evidence |
|---|---|---|
| CONNECT allowed → tunnels to backend | `TestNewProxyHandler_CONNECT_AllowedHostTunnels` | `main_test.go:39-81` real TCP backend, 200 + sentinel through tunnel |
| CONNECT denied → 403, no dial | `TestNewProxyHandler_CONNECT_DeniedHost` | `main_test.go:85-130` asserts 403 (124) AND `dialCount == 0` (127-129) |
| plain HTTP allowed | `TestNewProxyHandler_PlainHTTP_AllowedHost` | `main_test.go:134-170` proxied GET returns backend body |
| plain HTTP denied | `TestNewProxyHandler_PlainHTTP_DeniedHost` | `main_test.go:174-208` asserts 403 |
| empty allowlist denies all | `TestNewProxyHandler_EmptyAllowlistDeniesAll` | `main_test.go:212-255` two sub-tests (CONNECT + PlainHTTP) both 403 |

The CONNECT-denied test's `dialCount == 0` assertion is strong fail-closed evidence: the backend's `Accept` goroutine never fires.

### Mage Gate (re-run independently)

- `mage testPkg ./internal/cmd/valv-proxy` → **34/34 pass, 0 fail** (confirms builder's 34/34 claim).
- Per-function via `mage test-func` (full import path `github.com/evanmschultz/valv/internal/cmd/valv-proxy`): all 5 named functions pass individually — `..._CONNECT_AllowedHostTunnels` (1/1), `..._CONNECT_DeniedHost` (1/1), `..._PlainHTTP_AllowedHost` (1/1), `..._PlainHTTP_DeniedHost` (1/1), `..._EmptyAllowlistDeniesAll` (3/3 incl. sub-tests).

### Path Discipline

`git diff-tree 2f70e89` production files: only `internal/cmd/valv-proxy/main.go` + `main_test.go` (+ tracking mds PLAN.md/BUILDER_WORKLOG.md). No out-of-scope edits. 2 prod symbols (`newProxyHandler` + `main`), 1 prod file — within the 15.2.5.B.2 budget (~75 LOC / 2 symbols / 1 file).

### Tools Used

- `git show --stat 2f70e89`, `git show 2f70e89 --stat -- internal/cmd/valv-proxy/`, `git diff-tree --no-commit-id --name-only -r 2f70e89` — commit + path discipline.
- `Read` main.go (103 LOC), main_test.go (255 LOC), allowlist.go (53 LOC), PLAN.md sections 370-414, BUILDER_WORKLOG.md lines 1-50, WORKFLOW.md.
- `grep` for `hostAllowed`/`parseAllowlist` defs + unit refs + verdict anchors; `wc -l` for LOC counts.
- `mage testPkg ./internal/cmd/valv-proxy` -> 34/34 GREEN.
- `mage test-func github.com/evanmschultz/valv/internal/cmd/valv-proxy <Func>` x5 — all named functions GREEN individually.

### Verdict

`verdict: pass` — fail-closed allowlist check precedes all network I/O on both CONNECT and plain-HTTP paths (no dial on deny, proven by `dialCount==0`); CONNECT teardown (hijack/dial/dual-goroutine io.Copy/CloseWrite/WaitGroup) correct; `main()` env contract matches spec; all 4 required test cases present; mage gate independently GREEN (34/34, plus 5/5 per-function); path discipline clean.

## Unit 15.2.5.D.1 — Round 1

**verdict: pass** (build axis; orch-written per friction-free QA model). `Service.Provision` satisfies all four acceptance bullets at `e20ae31`:
- AC1: `const ProxyAlias = "valv-proxy"` (`service.go:55`) used in BOTH `buildNoProxy` (`:315`) and the `ConnectNetwork` alias (`:252`) + proxy URL (`:257`) — no divergence.
- AC2: order create/reuse net → `RunContainerDetached` (Image `proxyImageRef()`, Detached, env `VALV_PROXY_ALLOWLIST`+`VALV_PROXY_ADDR`, managed label, bridge default `:229-241`) → `ConnectNetwork(internal, sidecarID, [ProxyAlias])` `:249-255`.
- AC3: `PolicyMaterial{NetworkName, HTTPProxyURL/HTTPSProxyURL == "http://valv-proxy:8080", NoProxy == buildNoProxy()}` (`:258-263`), exact field names confirmed.
- AC4: tests assert order (`TestProvision_OperationOrder`), args, and error path (RunContainerDetached err → wrapped, `connectCalls==0`).
- Gate: `mage testPkg ./internal/services/networkpolicy` 35/35 GREEN (orch independently re-ran + `mage vet` whole-tree clean).

NITs (non-blocking): stale `images/service.go:118` doc; builder ran `mage format` + `mage testPkg ./internal/services/images` (out of builder gate scope — dispatch-audit note). NOTE: proof axis is pass, but see BUILDER_QA_FALSIFICATION.md Round 1 — falsification found an integration compile break (F-1) that gates D.1 → Round 2 required before D.1 is green.

## Unit 15.2.5.D.2 — Rounds 1-2

**verdict: PASS** (orch-written/orch-verified). `Provision` waits for sidecar readiness before returning. R1 added `waitForSidecar` + `defaultReadyProbe` but asserted an interface the prod executor didn't satisfy (silent no-op, F-2 below). R2 fixed it. Acceptance proven at `a478d67`:
- Readiness is HOST-reachable: `docker.Executor.ContainerRunning` (`executor.go:116`, value receiver) shells `docker inspect --format {{.State.Running}} <id>` via the runner `Output` type-assert (same pattern as `Create`); no internal-network TCP. `defaultReadyProbe` asserts the narrow `containerChecker` interface (`service.go:176`) that `docker.Executor` satisfies.
- Regression-proof: compile guard `var _ containerChecker = docker.Executor{}` (`service_test.go:802`) — build fails if `ContainerRunning` is dropped/renamed.
- `NetworkExecutor` interface UNCHANGED (5 methods) — `ContainerRunning` is additive on the concrete type + a separate narrow interface; no sibling break, no `mage integration` needed.
- Gates orch-re-ran: `mage testPkg ./internal/adapters/docker` 90/90 + `./internal/services/networkpolicy` 43/43 GREEN. Budget 2 symbols / ~47 LOC / 2 files.
