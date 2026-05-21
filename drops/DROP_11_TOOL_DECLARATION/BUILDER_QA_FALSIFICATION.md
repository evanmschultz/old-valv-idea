# DROP_11 Build QA Falsification

## Unit 11.1 — Round 1

**Verdict:** pass
**Reviewer:** ta-go-qa-falsification
**Reviewed at:** 2026-05-21T00:00:00Z

Reviewed against `main/drops/DROP_11_TOOL_DECLARATION/PLAN.md` Unit 11.1 acceptance and the falsification appendix's 7 attack groups. All attacks were exercised empirically via a scratch test file (`internal/tools/falsification_scratch_test.go`, 14 attack cases) that ran cleanly under `mage testPkg ./internal/tools/`. The scratch file was deleted before this report; the tree is pristine (`mage testPkg ./internal/tools/` post-cleanup reports 8 tests / 89.2% coverage matching the builder worklog).

### Counterexamples / Attacks

#### Attack 1 — `PrimitiveDecode` MANDATORY rule

Three sub-attacks; all **mitigated**.

- **Discard target type.** Lines 113 + 117 of `internal/tools/tools.go` declare `var discardAllowlist map[string]any` and `var discardEnv map[string]any` — matches PLAN.md requirement. NOT `toml.Primitive`. PASS.
- **No `[allowlist]` and no `[env]` in input.** Scratch test wrote `[tools]\nmage = "latest"\n` only. `Load` succeeded; both `PrimitiveDecode` calls on zero-value `toml.Primitive` returned nil. No panic, no error. PASS.
- **Only one of `[allowlist]`/`[env]` present.** Scratch tests `TestFalsification_OnlyAllowlist` and `TestFalsification_OnlyEnv` both succeeded. The unconditional double-call is safe.

#### Attack 2 — `ToolSpec.UnmarshalTOML` heterogeneous decode

Four sub-attacks; all **mitigated**.

- **`mage = 42` (number).** Scratch test wrote that input. `Load` returned a wrapped error from the type switch's `default` arm: `tool value must be a string or inline table, got int64`. PASS.
- **`mage = ["a","b"]` (array).** Scratch test confirmed `Load` returned an error from the same `default` arm. PASS.
- **Inline table missing `source` (install-only).** Scratch test wrote `ta = { install = "go install" }`. `Load` accepted it as `ToolSpec{Install: "go install"}` — Source and Version both empty. Per PLAN.md Notes "Load accepts partial; Validate rejects" — explicit U11.2 handoff. PASS.
- **Extra inline-table keys.** Scratch test wrote `ta = { source = "x", install = "y", extra = "z" }`. Diagnostic file confirmed `Load` returned nil error; the resulting `ToolSpec` is `{Version:"" Source:"x" Install:"y"}` — the `extra` key was silently dropped. This is acceptable for Unit 11.1: the `UnmarshalTOML` implementation only reads `source` and `install` keys from the map, so unknown inline keys are not flagged. **Minor risk**: a user typo like `instal = "..."` would be silently dropped and produce `Source="..." Install=""`, which Validate at U11.2 would then reject for missing `Install`. The error message at U11.2 should be clear enough for the user to spot the typo. Not a blocker for U11.1.

#### Attack 3 — `Load` error paths

Six sub-attacks; all **mitigated**.

- **Empty path string.** Line 90-92 of `tools.go`: explicit `strings.TrimSpace(path) == ""` guard returns wrapped `ErrToolsNotFound`. Tested in `tools_test.go` `empty_path_returns_sentinel`. PASS.
- **Permission denied.** Scratch test created a `chmod 000` file. `Load` returned a wrapped error from `toml.DecodeFile`. PASS.
- **Directory at path.** Scratch test created a directory at the target path. `Load` returned a wrapped error containing `is a directory` (matches the planner Round 4 empirical finding from PLAN.md Unit 11.4 §"Directory-at-path"). The path is consistent — `os.Stat` succeeds on dirs, then `toml.DecodeFile` opens + reads → `EISDIR`. PASS.
- **Invalid TOML syntax.** Scratch test wrote `[tools\nmage = "latest`. `Load` returned a wrapped error: `decode tools "...": ...`. The decode-tools context prefix is present. PASS.
- **Zero-byte file.** Scratch test wrote 0 bytes. `Load` succeeded with empty manifest. PASS — matches PLAN.md Unit 11.4 acceptance criterion 5 (zero-byte file is a valid empty manifest).
- **Nonexistent file.** Original test `absent_file_returns_sentinel` covers this. PASS.

#### Attack 4 — TOML quoted-key behavior

**Mitigated.**

- `internal/tools/testdata/valid_quoted_names.toml` exists with content `[tools]\n"github.com/foo/bar" = { source = "github.com/foo/bar@main", install = "go install" }\n`. The fixture file matches PLAN.md spec.
- The test case in `tools_test.go` asserts:
  - `len(m.Tools) == 1` (NOT split into nested tables)
  - `m.Tools["github.com/foo/bar"]` is populated with `Source` and `Install`
- Empirically verified to pass under `mage testPkg`. PASS.

#### Attack 5 — Unknown-key strict check

Three sub-attacks; all **mitigated**.

- **`[network]` top-level section.** Fixture `invalid_unknown_key.toml` triggers this. Test asserts error contains both `unknown keys` and `network`. PASS.
- **Invalid inline value under `[allowlist]` (e.g. `hosts = "not-an-array"`).** Scratch test `TestFalsification_InvalidAllowlistContents` wrote that input; `Load` accepted it silently (DROP_15 will validate at type-decode time). Matches PLAN.md scope: "DROP_11 takes no action on contents (silent pass-through)". PASS.
- **`[tools.subgroup]` nested table.** Scratch test confirmed: `Load` does NOT error; `m.Tools["subgroup"]` is populated as `ToolSpec{}` (all empty) because `UnmarshalTOML` runs on `map[string]interface{}{"mage":"latest"}` and finds neither `source` nor `install`. This is acceptable — Validate at U11.2 will reject the empty `ToolSpec`. Not a U11.1 blocker.

#### Attack 6 — `ErrToolsNotFound` sentinel

Two sub-attacks; both **mitigated**.

- **Absent file.** Original test `absent_file_returns_sentinel` asserts `errors.Is(err, domain.ErrToolsNotFound)`. PASS.
- **Symlink to nonexistent file.** Scratch test created `ln -s /nope /tmp/tools.toml` then called `Load`. `os.Stat` follows the symlink and returns `os.ErrNotExist`; the error chain correctly wraps to `domain.ErrToolsNotFound`. PASS.

#### Attack 7 — Test coverage gaps

**Mitigated with one accepted miss.**

- **`[allowlist]` + `[env]` co-existence.** Covered by `valid_objects_with_forward_compat_sections` (fixture `valid_objects.toml`). PASS.
- **No forward-compat sections at all.** Scratch test `TestFalsification_NoAllowlistNoEnv_PrimitiveDecodeSafe` confirms the zero-value `toml.Primitive` `PrimitiveDecode` call is safe. The original test suite covers this indirectly via `valid_simple.toml` which has no `[allowlist]`/`[env]` blocks and passes — so the canonical test suite already exercises the zero-value path.
- **10.8% coverage gap.** Visual inspection of `tools.go` and the test suite: the unreached lines are most likely the error-return arms of the two `meta.PrimitiveDecode` calls (lines 115 and 119). These are unreachable with valid inputs because `PrimitiveDecode` only errors on internal Primitive corruption, not on input shape. Accepted miss; not a falsification-blocking gap.

### YAGNI Pressure

None. The package is minimal: `ToolSpec`, `ToolManifest`, `Load`, `UnmarshalTOML`. No interfaces, no premature abstractions, no internal subpackages. The `toml.Primitive` fields with `PrimitiveDecode` discard are intentional forward-compat that the planner pinned and the dev approved through Round 4. No surplus surface.

### Hylla Feedback

No Hylla queries needed — the package is brand-new and the test suite is small enough to read directly. Matches the worklog's note: the canonical pattern (`internal/config/Load`) was read directly.

### Falsification Summary

- **Confirmed counterexamples blocking PASS: 0.**
- **Mitigated risks noted for U11.2:** silent-drop of unknown inline-table keys (e.g. `extra = "z"`) and nested-table-as-tool-key (`[tools.subgroup]`) both produce well-formed but semantically invalid `ToolSpec` values that Validate at U11.2 must reject. The U11.2 builder should ensure the regex + presence checks catch these. The PLAN.md U11.2 acceptance already requires Source+Install both non-empty for object-form and Version non-empty for string-form, which covers both cases.
- **Scratch verification overhead:** 14 attack tests added + run via `mage testPkg`, then deleted. Pristine tree confirmed via post-cleanup `mage testPkg` returning to 8 tests / 89.2%.

Verdict: **pass**.
