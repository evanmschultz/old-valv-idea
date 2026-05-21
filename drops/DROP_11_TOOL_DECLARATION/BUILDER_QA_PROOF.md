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

## Unit 11.2 — Round 1

**Verdict:** pass
**Reviewer:** ta-go-qa-proof
**Reviewed at:** 2026-05-21 UTC

### Acceptance Criteria Verification

#### AC1 — `Validate(m ToolManifest) error` exported

**Verdict:** pass

`internal/tools/validate.go:43` exports
`func Validate(m ToolManifest) error`. Doc comment starts with the
identifier name (`Validate checks...`, lines 34-42) per project rule. Returns
`nil` for a valid manifest (line 57); returns wrapped `fmt.Errorf` for each
violation (lines 45, 50, 74, 77, 80, 88).

#### AC2 — Tool name regex

**Verdict:** pass

`internal/tools/validate.go:32`:

```go
var toolNameRE = regexp.MustCompile(`^[a-zA-Z0-9]+$|^[a-zA-Z0-9][a-zA-Z0-9._/-]*[a-zA-Z0-9]$`)
```

- Package-level `var` — compiled once at init, reused across all `Validate`
  calls. Confirmed not lazily compiled inside the function.
- Pattern matches the PLAN.md pin character-for-character.
- RE2-compatible (alternation + character class only; no lookahead).
- Used via `toolNameRE.MatchString(name)` at `validate.go:49`.

#### AC3 — Accept-by-design names all pass `Validate`

**Verdict:** pass

`internal/tools/validate_test.go:19-29` (`TestValidate_ToolName`) declares the
following accept cases — every PLAN-pinned name plus structurally similar
extras — all with `wantErr: false`:

- `mage` (line 19) — simple word.
- `m` (line 21) — single-char alphanumeric.
- `github.com/foo/bar` (line 23) — path-like name.
- `go-1.22` (line 24) — version suffix.
- `a.b.c` (line 25) — namespaced.
- `a..b` (line 26) — double dot, accepted by design.
- `a//b` (line 27) — double slash, accepted by design.
- `a---b` (line 28) — multiple dashes, accepted by design.

All eight PLAN-pinned accept-by-design names are present, each in a
named-subtest row, each spec-wrapped in `ToolSpec{Version: "latest"}` so the
name check is exercised in isolation. Live run: all 48 tests pass.

#### AC4 — Reject-by-design names all fail `Validate`

**Verdict:** pass

`internal/tools/validate_test.go:32-43` declares every PLAN-pinned reject
case with `wantErr: true`:

- `""` (line 32) — empty string.
- `with space` (line 33) — embedded space.
- `-bad` (line 34) — leading dash.
- `_underscore` (line 36) — leading underscore.
- `bad-name-` (line 35) — trailing dash.
- `name.` (line 37) — trailing dot.
- `name/` (line 38) — trailing slash.
- `bad!char` (line 39) — `!` outside permitted set.
- ` name` (line 40) — leading whitespace.

All nine PLAN-pinned reject-by-design names are present. Each test asserts
the error message contains `"invalid tool name"` (line 65) and (for
non-empty cases) mentions the offending name (line 69). Two bonus reject
cases (`trailing_whitespace`, `at_disallowed`, `non_ascii`) tighten the net
further. Live run: all reject cases produce errors as expected.

#### AC5 — Object-form rules

**Verdict:** pass

`internal/tools/validate_test.go:81-166` (`TestValidate_SpecShape`) covers
the object-form quadrant:

- `{Source: "github.com/evanmschultz/ta@main", Install: "go install"}` —
  valid (lines 101-105, `object_form_valid`, `wantErr: false`).
- `{Source: "github.com/evanmschultz/ta@main"}` (missing install) — invalid
  (lines 106-110, `errContains: "missing install"`).
- `{Install: "go install"}` (missing source) — invalid (lines 112-116,
  `errContains: "missing source"`).
- `{Version: "1.0", Source: ..., Install: ...}` (mixed) — invalid (lines
  118-122, `errContains: "version alongside source/install"`).

Implementation at `validate.go:65-90` matches: the `hasSource || hasInstall`
guard at line 71 enters the object-form branch first; missing-source check
at line 73, missing-install at line 76, mixed-version check at line 79.

#### AC6 — String-form rules

**Verdict:** pass

- `{Version: "latest"}` valid — `validate_test.go:91-95` (`string_form_valid`,
  `wantErr: false`).
- `{Version: "1.22"}` valid — `validate_test.go:96-100`
  (`string_form_pinned_version`).
- `{}` (all-empty) invalid — `validate_test.go:130-134` (`all_empty`,
  `errContains: "empty spec"`).

Implementation at `validate.go:83-89`: string-form falls through the switch
to the `hasVersion` case (line 83), then the `default` (line 86) catches
all-empty with `"empty spec"`. The `default` matches the AC: when both
`Source`/`Install` and `Version` are empty, the manifest entry is invalid.

#### AC7 — Max count (50 valid, 51 invalid)

**Verdict:** pass

`internal/tools/validate_test.go:179-220` (`TestValidate_ToolCount`):

- `count_50_boundary` (line 189) — 50 tools named `tool0`..`tool49`, all
  alphanumeric and regex-valid, `wantErr: false`.
- `count_51_over_boundary` (line 190) — 51 tools, `wantErr: true`,
  `errContains: "exceeds max"`.

Implementation at `validate.go:44-46`: `if len(m.Tools) > maxToolCount`
(where `maxToolCount = 50` per line 13) returns
`"validate tools: tool count %d exceeds max of %d"`. The count check runs
BEFORE the per-tool iteration loop, so an over-limit manifest fails on the
count check even if every name is valid — confirmed by the test where all
51 names match the regex.

#### AC8 — First violation only, single `fmt.Errorf`

**Verdict:** pass

`Validate` returns from the first violation it encounters:

- `validate.go:45` — count-exceeded return.
- `validate.go:50` — invalid-name return.
- `validate.go:52-54` — propagates first error from `validateSpec` (which
  itself returns at first violation: lines 74, 77, 80, 88).

No `errors.Join`, no slice accumulation, no multi-error. Every error is a
single `fmt.Errorf("validate tools: ...", ...)` call. Tests acknowledge
non-deterministic map iteration order and assert only the SHAPE of the
error (substring match), never which of two intentionally-invalid entries
comes first — see `validate_test.go:42` notes in worklog Round 1.

#### AC9 — `testdata/invalid_object_missing_install.toml` round-trip

**Verdict:** pass

Fixture `internal/tools/testdata/invalid_object_missing_install.toml`:

```toml
[tools]
ta = { source = "github.com/evanmschultz/ta@main" }
```

Test `TestValidate_LoadedFixtureMissingInstall`
(`validate_test.go:222-253`):

- Line 226: `m, err := Load(path)` — loads via the real `Load` function,
  proving `Load` accepts the partial-object form (validation belongs to
  `Validate`, not `Load`).
- Lines 231-241: sanity-checks parsed shape — `Tools["ta"].Source` set,
  `Tools["ta"].Install` empty.
- Lines 243-246: `Validate(m)` returns a non-nil error.
- Lines 247-252: error message contains both `"missing install"` and
  `"ta"`.

This is the `Load`-then-`Validate` boundary the PLAN explicitly demands.

#### AC10 — `mage testPkg ./internal/tools/` passes with reported counts

**Verdict:** pass

Live reproduction during this review:

```
[INFO] Started go test -json (-count=1 -race -cover ./internal/tools/)
[PKG PASS] github.com/evanmschultz/valv/internal/tools (1.29s)
  tests: 48
  passed: 48
  failed: 0
  ...
  github.com/evanmschultz/valv/internal/tools | 93.1%
```

48 tests / 93.1% coverage match worklog Round 1 exactly. Both are well
above the active 60% gate and the future 70% gate (Unit 11.5). `-race`
and `-cover` are on per the mage target.

### Findings

None. All ten acceptance criteria pass with file:line evidence and a
reproduced mage run. The implementation matches the PLAN-pinned regex,
spec-shape rules, count boundary, error format (first-violation
`fmt.Errorf`), and round-trip fixture path. Tests are table-driven,
parallelized, and assert error-message shape via substring (correctly
avoiding ordering assumptions over Go map iteration).

### Hylla Feedback

Not used in this review. Unit 11.2 is brand-new uncommitted code that
Hylla will only see at drop-end reingest; `Read` of the source, tests,
fixture, and worklog plus the live mage run provided complete coverage.
No Hylla fallback miss to record.
