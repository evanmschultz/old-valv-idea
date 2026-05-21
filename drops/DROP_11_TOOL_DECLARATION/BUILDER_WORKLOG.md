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
