# DROP_11 — Builder Worklog

Append a `## Unit 11.M — Round K` section per build attempt. See `main/drops/WORKFLOW.md` § "Phase 4 — Build (per unit)" for what each section should contain.

## Unit 11.1 — Round 1

- **Builder:** ta-go-builder
- **Started:** 2026-05-21
- **Files touched:**
  - `internal/domain/errors.go` (additive — added `ErrToolsNotFound` sentinel alongside `ErrConfigNotFound` in the existing `var` block)
  - `internal/tools/tools.go` (new — package doc, `ToolSpec`, `ToolSpec.UnmarshalTOML`, `ToolManifest`, `Load`)
  - `internal/tools/tools_test.go` (new — table-driven `TestLoad` + `TestToolSpec_UnmarshalTOML_RejectsBadValueShape`)
  - `internal/tools/testdata/valid_simple.toml` (new — string-only `[tools]` entries)
  - `internal/tools/testdata/valid_objects.toml` (new — mixed string + inline-table + `[allowlist]` + `[env]`)
  - `internal/tools/testdata/valid_quoted_names.toml` (new — `"github.com/foo/bar"` quoted key)
  - `internal/tools/testdata/invalid_unknown_key.toml` (new — `[network]` triggers strict undecoded check)
- **Mage targets run:**
  - `mage testPkg ./internal/tools/` — PASS, 8 tests, 89.2% coverage.
  - `mage testPkg ./internal/domain/` — PASS, 26 tests, 85.2% coverage (additive change is non-breaking).
- **Notes:**
  - **`PrimitiveDecode` discard target choice.** Per PLAN.md Critical Implementation Note 1, the discard targets for `meta.PrimitiveDecode(m.Allowlist, ...)` and `meta.PrimitiveDecode(m.Env, ...)` are local `map[string]any` values, NOT `toml.Primitive`. Empirical evidence in the plan (planner Round 4 scratch run) showed `toml.Primitive` as a discard leaves keys flagged undecoded; `map[string]any` correctly marks the sections decoded. The `invalid_unknown_key.toml` fixture's `[network]` section confirms the strict `meta.Undecoded()` check still fires on genuinely unknown top-level sections after the two `PrimitiveDecode` calls — i.e. the calls do not over-mark.
  - **`ToolSpec.UnmarshalTOML` signature.** Context7 query against `/burntsushi/toml` confirmed the `toml.Unmarshaler` interface is `UnmarshalTOML(data any) error`, dispatched on runtime type. String → `Version`, `map[string]interface{}` → `Source` + `Install`. Validation that the inline-table form has BOTH `source` and `install` belongs to Unit 11.2, not this unit — `Load` accepts partial object forms here.
  - **Sentinel pattern mirrors `internal/config/Load`.** Empty-path guard + `os.Stat` + `os.ErrNotExist` → wrap `domain.ErrToolsNotFound`; other stat errors wrap raw. `toml.DecodeFile` errors wrap with file path context.
  - **Strict undecoded error message** includes the offending keys joined by comma (e.g. `unknown keys: network, network.proxy`) so the operator can see exactly what was rejected. The `invalid_unknown_key.toml` test asserts the error contains both `unknown keys` and the `network` substring.
  - **Quoted-name fixture** verified to produce a single flat map entry `m.Tools["github.com/foo/bar"]` — not nested `[github][github.com][...]` tables. This proves TOML quoted-key behavior is honored end-to-end through the heterogeneous `UnmarshalTOML`.
  - **No `## Hylla Feedback` subsection needed.** Hylla was not queried in this round — the canonical pattern (`internal/config/Load`) is small enough that `Read` of `internal/config/config.go` plus the in-prompt PLAN.md notes provided full coverage. Context7 supplied the BurntSushi/toml API shape. No fallback miss to report.

## Unit 11.2 — Round 1

- **Builder:** ta-go-builder
- **Started:** 2026-05-21
- **Files touched:**
  - `internal/tools/validate.go` (new — `Validate(m ToolManifest) error`, package-level `toolNameRE`, `maxToolCount = 50`, internal `validateSpec` helper)
  - `internal/tools/validate_test.go` (new — table-driven tests covering name regex, string/object spec shape, empty manifest, count boundary, and loaded-fixture round-trip)
  - `internal/tools/testdata/invalid_object_missing_install.toml` (new — `ta = { source = "..." }` with no `install`, exercises `Load`-then-`Validate` rejection path)
- **Mage targets run:**
  - `mage testPkg ./internal/tools/` — PASS, 48 tests, 93.1% coverage.
- **Notes:**
  - **Regex placement.** `toolNameRE` is a package-level `regexp.MustCompile(...)` var so it compiles once at init time and is shared across all `Validate` calls. RE2-compatible (alternation + character class only; no lookahead).
  - **Validation rules verified against PLAN.md.** All planner-pinned accept-by-design cases (`a..b`, `a//b`, `a---b`, `github.com/foo/bar`, `go-1.22`, single-char `m`, internal `_`) pass; all planner-pinned reject-by-design cases (`_underscore` leading underscore, trailing dash/dot/slash, leading dash, `bad!char`, embedded/leading/trailing whitespace, non-ASCII `naïve`) fail with `invalid tool name` errors. Extra reject case `a@b` added for `@` outside the permitted set.
  - **Spec shape switch.** `validateSpec` dispatches on `(hasSource || hasInstall)` first so partial-object errors (`missing source` / `missing install`) and mixed-form errors (`version alongside source/install`) are produced before the string-form path. All-empty spec yields the `empty spec` message.
  - **Count boundary at 50.** `maxToolCount = 50`; the test confirms `count = 50` validates and `count = 51` errors with `exceeds max`. Names generated as `tool0`..`tool50` (all valid per regex).
  - **First-violation semantics.** `Validate` returns at the first violation rather than collecting a multi-error, matching `internal/config` style. Map iteration order in Go is non-deterministic — tests assert only the SHAPE of the error (substring), never which of two intentionally-invalid entries comes first.
  - **Round-trip fixture test.** `TestValidate_LoadedFixtureMissingInstall` loads `testdata/invalid_object_missing_install.toml` via `Load` (proving `Load` itself accepts the partial-object shape), then asserts `Validate` rejects it with `missing install`. This is the load-then-validate boundary the PLAN called out.
  - **No `## Hylla Feedback` subsection needed.** Hylla was not queried this round — the validation rules are fully specified in PLAN.md and the regex is a new construct with no prior repo pattern to grep. `Read` of `internal/tools/tools.go` and the unit's tests confirmed the existing types and style.

## Unit 11.3 — Round 1

- **Builder:** ta-go-builder
- **Started:** 2026-05-21
- **Files touched:**
  - `internal/tools/resolve.go` (new — `ToolsFilePath = ".valv/tools.toml"` constant + `Resolve(projectDir string) (ToolManifest, error)`)
  - `internal/tools/resolve_test.go` (new — table-light, scenario-named tests covering absent file, absent file with `.valv/` dir present, valid simple manifest, valid manifest with `[allowlist]`+`[env]`, Load parse error, Load unknown-key error, Validate error, and a constant-stability sanity check)
- **Mage targets run:**
  - `mage testPkg ./internal/tools/` — PASS, 56 tests (8 new), 95.5% coverage.
- **Notes:**
  - **Sentinel-only absent-file binding.** `Resolve` checks `errors.Is(err, domain.ErrToolsNotFound)` exclusively and returns `ToolManifest{}, nil` on that branch. There is no `os.IsNotExist` fallback — Load is responsible for translating raw `os.ErrNotExist` into `domain.ErrToolsNotFound` (Unit 11.1 already does this), so Resolve gets a clean sentinel surface. The two absent-file tests (`TestResolve_AbsentFile`, `TestResolve_AbsentFile_DotValvDirExistsButNoToml`) exercise both the "no `.valv/` dir at all" path and the "`.valv/` dir exists but `tools.toml` missing" path — both produce the same sentinel-wrapped error inside Load, both flow through the same nil-error branch in Resolve.
  - **Error-wrapping shape.** Non-absent Load errors and Validate errors are wrapped with `fmt.Errorf("resolve tools %q: %w", projectDir, err)`. This preserves `errors.Is` chains (Load's sentinel chain stays intact if a future caller wants it; the `unknown keys` substring from Load's strict undecoded check stays reachable; Validate's `invalid tool name` substring stays reachable). Tests assert on the substring `resolve tools` to confirm wrapping, plus on the inner error substrings (`unknown keys`, `invalid tool name`, `_underscore`) to confirm chain preservation.
  - **`filepath.Join` choice.** `filepath.Join(projectDir, ToolsFilePath)` is the standard path composition. For empty `projectDir`, `filepath.Join("", ".valv/tools.toml")` returns `".valv/tools.toml"` (relative path), which Load then resolves against CWD. The PLAN explicitly leaves empty-projectDir behavior to callers — there is no Resolve-side guard. No test exercises that path because behavior is intentionally CWD-dependent and brittle to assert on.
  - **Real filesystem, no mocks.** All tests use `t.TempDir()` per `main/CLAUDE.md` § "Tests" — real `os.MkdirAll` + `os.WriteFile` against scratch dirs. The `writeFixture` helper centralizes the `.valv/` dir + `tools.toml` write. No mock filesystem.
  - **Forward-compat sections coverage.** `TestResolve_ValidManifest_WithForwardCompatSections` writes an inline fixture mirroring `testdata/valid_objects.toml` so Resolve exercises Load's `PrimitiveDecode` path end-to-end. This guards against a future regression where Resolve might silently bypass the `[allowlist]`/`[env]` decode behavior.
  - **Constant-stability test.** `TestResolve_ToolsFilePathConstant` pins `ToolsFilePath` to `".valv/tools.toml"`. Unit 11.4 (CLI) and DROP_12 (image build) both consume this constant; the test makes a future accidental rename loudly visible.
  - **No `## Hylla Feedback` subsection needed.** Hylla was not queried this round — Unit 11.1 + 11.2 are uncommitted-since-last-ingest (Hylla would miss `Load`, `Validate`, `ToolSpec`, `ToolManifest`, `domain.ErrToolsNotFound`). The prompt explicitly directed `Read` fallback for those symbols, which provided full coverage. No miss to feed back.

## Unit 11.4 — Round 1

- **Builder:** ta-go-builder
- **Started:** 2026-05-21
- **Files touched:**
  - `internal/cli/tools.go` (new — `newToolsCommand`, `newToolsValidateCommand`, `runToolsValidate` handler)
  - `internal/cli/tools_test.go` (new — table-light scenario tests covering all 7 PLAN cases plus a root-tree registration guard)
  - `internal/cli/testdata/zero_byte.toml` (new — committed zero-byte fixture for Case 5)
  - `internal/cli/root.go` (existing — registered `toolsCmd` with `GroupID = "runtime"` and added to the `cmd.AddCommand(...)` variadic at line 137)
- **Mage targets run:**
  - `mage testPkg ./internal/tools/` — PASS, 56 tests, **95.5% coverage** (no change from Unit 11.3; the CLI tests do not re-run the tools package).
  - `mage testPkg ./internal/cli/` — PASS, 228 tests (10 new), **67.6% coverage**. Threshold met against the current 60% gate.
- **Notes:**
  - **`project.Detect()` integration.** Per PLAN brief, `runToolsValidate` calls `project.Detect()` (no args, reads `os.Getwd()` internally) and passes `result.Root` to `tools.Resolve`. No raw `os.Getwd()`. Tests use the existing claude/codex pattern: `os.Chdir` into a `t.TempDir()` with a stub `.git/` so `project.Detect()` anchors on the temp dir. The `chdirToProjectRoot` helper in `tools_test.go` centralizes that setup and restores CWD via `t.Cleanup`.
  - **Output style — direct `fmt.Fprintln`.** The PLAN says "`laslig`-style output consistent with rest of `internal/cli/`" but `WriteRecord` is overkill for a one-line status that has no fields to render in JSON/plain/human modes. Each of the two success messages (`"tools.toml is valid"` and `"no tools declared"`) is a single literal string and is tested by substring match. Direct `fmt.Fprintln(cmd.OutOrStdout(), ...)` keeps the wire intent obvious and avoids a JSON envelope key the operator never asked for. If a future drop wants structured `tools validate` output (e.g. `--format json`), it can swap to `WriteRecord` with a single `status` field cleanly.
  - **`SilenceUsage: true` on `validate`.** Matches `claude.go` line 48. Without this, cobra dumps the full usage block on every error return, drowning the actual error in noise.
  - **Empty-manifest predicate.** `len(manifest.Tools) == 0` covers all three cases that should print `"no tools declared"`: (a) absent file (Resolve returns empty manifest, nil error), (b) present file with empty `[tools]` table, (c) zero-byte file (decodes cleanly, no entries). Verified explicitly by `TestToolsValidate_NoValvDir`, `TestToolsValidate_EmptyToolsTable`, and `TestToolsValidate_ZeroByteFile`.
  - **Zero-byte fixture.** `testdata/zero_byte.toml` is a committed zero-byte file (`wc -c` confirms 0 bytes). The test resolves the absolute path BEFORE chdir (chdir invalidates the relative `testdata/...` reference), reads the contents, and writes them into `<tempdir>/.valv/tools.toml`. The intermediate read also asserts the fixture is still zero bytes — guards against an accidental editor-save inserting a trailing newline.
  - **Permission-denied case.** Skips on `runtime.GOOS == "windows"` (no POSIX mode semantics) and `os.Geteuid() == 0` (root bypasses mode checks). Restores `0o644` on the file before TempDir cleanup so the temp tree removes cleanly. Accepts either `errors.Is(err, syscall.EACCES)` or substring `"permission denied"` — the wrapped chain typically surfaces both.
  - **Directory-at-path case.** Creates `.valv/tools.toml` as a directory via `os.MkdirAll`. Per PLAN brief C3: accept either `errors.Is(err, syscall.EISDIR)` or substring `"is a directory"`. `toml.DecodeFile` calls `os.Open` (succeeds on a dir on macOS) then reads (returns `EISDIR`), so the wrapped error surfaces both indicators.
  - **Registration guard.** `TestToolsValidate_RegisteredOnRoot` walks the root command tree, asserts the `tools` branch is present with `GroupID == "runtime"`, and verifies the `validate` subcommand is registered on it. Guards against a future regression where the new `toolsCmd` is silently dropped from the `cmd.AddCommand(...)` variadic.
  - **`TestVisibleCommandsDefineLongAndExample` passes for the new commands.** Both `tools` and `tools validate` define non-empty `Long` and `Example` per the existing root-test invariant.
  - **No `## Hylla Feedback` subsection needed.** Hylla was not queried this round — Unit 11.1–11.3 code is uncommitted-since-last-ingest. The prompt directed `Read` fallback for `tools.Resolve`, `ToolsFilePath`, `domain.ErrToolsNotFound`, and `project.Detect`/`Result.Root`. Those reads provided full coverage. No miss to feed back.

### Unknown — `internal/cli/` coverage below 70%

- **Observation:** `internal/cli/` ended Unit 11.4 at **67.6%** coverage. Pre-existing baseline (per prompt brief, "currently around 67.4%") was already below the 70% gate Unit 11.5 will enforce. My additions (`tools.go` + tests) at high local coverage pushed the package up 0.2pp, NOT down — this unit did not push the package below the gate; it was already below.
- **Classification per Unit 11.5 escalation table:** Legacy-package failure (not new-package). `internal/cli/` is a pre-existing package. Per Unit 11.5 acceptance: "out of scope for this drop's code units. Route to dev for triage."
- **Route:** Orchestrator → dev to decide between (a) adding a follow-on coverage-raise unit (e.g. Unit 11.6) inside DROP_11 to bring `internal/cli/` above 70%, OR (b) deferring the `coverageThreshold` bump in Unit 11.5 and opening a separate `internal/cli` coverage-cleanup drop. Either decision is recorded as a comment update in PLAN.md per the Unit 11.5 acceptance text.
- **Unit 11.4 acceptance status:** Acceptance criterion "`mage testPkg ./internal/cli/` passes" IS met against the current 60% gate. The 70% gate is Unit 11.5's concern.
