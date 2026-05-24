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
