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
