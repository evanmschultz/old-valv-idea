# DROP_11 Build QA Proof

## Unit 11.1 — Round 1

**Verdict:** pass
**Reviewer:** ta-go-qa-proof
**Reviewed at:** 2026-05-21 UTC

### Acceptance Criteria Verification

#### AC1 — `domain.ErrToolsNotFound` sentinel

**Verdict:** pass

`internal/domain/errors.go:7` declares
`ErrToolsNotFound = errors.New("tools file not found")` inside the existing
`var (...)` block immediately after `ErrConfigNotFound` (line 6). String
literal matches the PLAN.md pin. Sentinel is exported, used by `internal/tools`
via `errors.Is`.

#### AC2 — `ToolSpec` type

**Verdict:** pass

`internal/tools/tools.go:26-37` declares the type with `Version string`,
`Source string`, `Install string`. Doc comment starts with the identifier name
per project rule. `UnmarshalTOML` dispatch at `tools.go:44-68`:

- `case string` → `t.Version = val` (line 47).
- `case map[string]interface{}` → reads `source` and `install` keys with
  per-key string-type assertions (lines 49-63).
- `default` → returns `fmt.Errorf("tool value must be a string or inline table, got %T", v)` (line 66).

Pointer receiver `(t *ToolSpec)` per `toml.Unmarshaler` mutation requirement.

#### AC3 — `ToolManifest` type

**Verdict:** pass

`internal/tools/tools.go:75-79`:

```go
type ToolManifest struct {
    Tools     map[string]ToolSpec `toml:"tools"`
    Allowlist toml.Primitive      `toml:"allowlist"`
    Env       toml.Primitive      `toml:"env"`
}
```

All three field tags + types match the PLAN.md pin exactly. Doc comment
explains forward-compat behavior.

#### AC4 — `Load(path string) (ToolManifest, error)`

**Verdict:** pass — including the Round 3 showstopper fix.

- Empty-path guard at `tools.go:90-92` returns wrapped `ErrToolsNotFound`.
- `os.Stat` guard at lines 93-98: `errors.Is(err, os.ErrNotExist)` wraps
  `ErrToolsNotFound` (line 95); other stat errors wrap the underlying
  error with `%w` (line 97).
- `toml.DecodeFile` at line 101, error wrapped at line 103.
- **Showstopper verified:** `meta.PrimitiveDecode(m.Allowlist, &discardAllowlist)`
  at line 114 and `meta.PrimitiveDecode(m.Env, &discardEnv)` at line 118 BOTH
  run BEFORE `meta.Undecoded()` at line 122. Discard targets are
  `map[string]any` (`tools.go:113` and `tools.go:117`), NOT `toml.Primitive`.
  This is exactly the fix the Round 3 plan QA demanded.
- The `valid_objects_with_forward_compat_sections` test case
  (`tools_test.go:44-72`) loads a fixture with `[allowlist]` + `[env]` sections
  and asserts `err == nil`. Without the two `PrimitiveDecode` calls,
  `meta.Undecoded()` would return `[allowlist.hosts env.GOPRIVATE]` and the
  test would fail — so the test is load-bearing on the fix.
- Strict `meta.Undecoded()` check at lines 122-128: collects offending keys via
  `k.String()` and returns `fmt.Errorf("decode tools %q: unknown keys: %s", ...)`
  — wrapped, file-context preserved.
- All four error returns inside `Load` use `%w`.

#### AC5 — `(t *ToolSpec) UnmarshalTOML(v interface{}) error`

**Verdict:** pass

`internal/tools/tools.go:44-68`. Pointer receiver. Signature
`UnmarshalTOML(v interface{}) error` matches the AC pin. Runtime dispatch
via `switch val := v.(type)` over `string`, `map[string]interface{}`, and
`default`. Per-key string assertions guard against non-string `source`/`install`
values. Exercised by `TestToolSpec_UnmarshalTOML_RejectsBadValueShape`
(`tools_test.go:150-182`) which validates non-string source, non-string install,
wholly wrong type (`int64`), and string-success paths.

> Note: PLAN.md § "Notes For Builder Agents" line 245 mentions
> `(t *ToolSpec) UnmarshalTOML(fn func(interface{}) error) error` — a stale
> annotation. The AC line on `Unit 11.1` (line 125 of PLAN.md) pins the
> `UnmarshalTOML(v interface{}) error` signature, which is the current
> `toml.Unmarshaler` interface per Context7 (confirmed in worklog Round 1
> notes). Builder followed the AC + Context7. Implementation is correct for
> the live `BurntSushi/toml v1.6.0` API.

#### AC6 — `testdata/valid_quoted_names.toml` + flat-entry assertion

**Verdict:** pass

Fixture `internal/tools/testdata/valid_quoted_names.toml`:

```toml
[tools]
"github.com/foo/bar" = { source = "github.com/foo/bar@main", install = "go install" }
```

Test case `valid_quoted_names` (`tools_test.go:73-95`) asserts:

- `len(m.Tools) == 1` (line 81) — NOT nested tables.
- `m.Tools["github.com/foo/bar"]` exists with the unquoted-string key (line 84).
- `spec.Source == "github.com/foo/bar@main"` (line 88).
- `spec.Install == "go install"` (line 91).

The flat-entry property is the load-bearing assertion the AC demanded.

#### AC7 — `testdata/invalid_unknown_key.toml` + Load error

**Verdict:** pass

Fixture contains `[network]\nproxy = "http://example.com"` as the unknown
top-level section. Test case `invalid_unknown_key` (`tools_test.go:96-111`)
asserts:

- `err != nil` (line 101).
- `err.Error()` contains `"unknown keys"` (line 104).
- `err.Error()` contains `"network"` (line 107).

Confirms the strict `meta.Undecoded()` check fires AFTER the two
`PrimitiveDecode` calls — i.e. the `PrimitiveDecode` calls do not over-mark
genuinely unknown sections.

#### AC8 — Coverage gates

**Verdict:** pass

Both mage runs reproduced live during this review:

- `mage testPkg ./internal/tools/` → 8 tests, all pass, 89.2% coverage.
- `mage testPkg ./internal/domain/` → 26 tests, all pass, 85.2% coverage.

Both numbers match `BUILDER_WORKLOG.md` § "Mage targets run" exactly. Both
are well above the active 60% gate and the future 70% gate that Unit 11.5
enables.

### Findings

None. All eight acceptance criteria pass with file:line evidence + reproduced
mage runs. The Round 3 falsification showstopper (`PrimitiveDecode` target
type + ordering) is correctly fixed at `tools.go:113-120`.

### Hylla Feedback

Not used in this review. The implementation is brand-new uncommitted code
that Hylla will only see at drop-end reingest. `Read` of the current files,
the worklog, and the live mage runs provided complete coverage. No Hylla
fallback miss to record.
