# DROP_11 Plan QA Proof — Round 1

**Verdict:** fail
**Reviewer:** go-qa-proof-agent
**Reviewed at:** 2026-05-20T00:00:00Z

The plan is mostly sound — schema decisions are grounded in the OSS survey + repo state, the
4-unit serial chain has clean compile-safe boundaries, and the planner correctly identifies
existing repo facts (`internal/config/Load` template, `BurntSushi/toml v1.6.0` availability,
absence of `internal/tools/` + `ErrToolsNotFound`). However, the plan ships **three load-bearing
factual errors and one sequencing ambiguity** that QA would refuse to wave through at
build-QA. Listed below by severity.

## Findings

### 1. [Axis: spec-conformance] [severity: high] Coverage threshold is 60.0, not 70.0 — the per-package gate the plan keys off does not exist at the value the plan asserts

The planner asserts (drop-level Acceptance Criterion 8, Unit 11.3 acceptance, and Notes
closing line):

> `mage testPkg ./internal/tools/` reports ≥ 70% coverage for the package.
> The 70% per-package coverage gate applies to the new `internal/tools/` package from day
> one.

`main/magefile.go` line 22-24 declares:

```go
const (
    // TODO: restore to 70.0 after raising internal/adapters/docker coverage (see main/REFINEMENTS.md).
    coverageThreshold = 60.0
```

`TestPkg` (magefile.go:117–143) calls `renderCoverage(printer, report, coverageThreshold)`
on every invocation, so the **per-package gate is enforced** — but at **60.0%**, not 70%.
`mage testPkg ./internal/tools/` reporting 65% would PASS today.

This breaks the auditability of the drop's acceptance: QA cannot use `mage testPkg` to verify
"≥ 70%" because the gate's at 60%. Three viable fixes — planner picks one:

1. Restate AC8 as "≥ 60% (current `coverageThreshold`) per the magefile gate" and add a
   forward-compat note about the planned restore-to-70 TODO.
2. Restate AC8 as "≥ 70% verified by the builder reading the coverage table emitted by
   `mage testPkg`" (gate passes at 60%, but builder reports actual % in
   `BUILDER_WORKLOG.md`).
3. Add Unit 11.0 (or fold into 11.3) raising the constant to 70.0 AND raising
   `internal/adapters/docker` coverage to clear the gate. (Largest scope creep; probably
   reject — that's a separate drop.)

Recommend option 2 — builder reports actual coverage in the worklog, QA verifies the number
visually, the existing 60% gate continues to enforce the floor. Cheapest and accurate.

### 2. [Axis: spec-conformance] [severity: high] Unit 11.1 mirrors `config.Load`'s error-on-missing-file pattern, but the sentinel for that error is introduced in Unit 11.3

`internal/config/Load` (verified at `internal/config/config.go:52-72` via Hylla node `internal/config/Load`) returns
`fmt.Errorf("load config %q: %w", path, domain.ErrConfigNotFound)` on `os.ErrNotExist`.

Unit 11.1 says `Load` "mirrors `internal/config/Load` — `os.Stat` guard, `toml.DecodeFile`,
`meta.Undecoded()` strict check, `fmt.Errorf("...: %w", err)` wrapping". Mirroring implies
returning a wrapped sentinel on file-absent — but `domain.ErrToolsNotFound` is declared NEW
in Unit 11.3, two units downstream.

Builder for 11.1 will hit this ambiguity immediately: do they wrap with
`os.ErrNotExist` directly, invent a local sentinel, defer the wrap to 11.3, or
add `ErrToolsNotFound` to `internal/domain/errors.go` early? Each choice has rework downstream.

Two viable fixes:

1. **Move the `ErrToolsNotFound` declaration into Unit 11.1.** The sentinel is one
   `var` declaration in `internal/domain/errors.go`; adding it in 11.1 keeps `Load` and its
   sentinel in the same unit. Unit 11.3 then just consumes the existing sentinel via
   `errors.Is`. This is the natural ordering and what `internal/config` does (Load + its
   sentinel live in the same drop's logical scope).
2. **Have Unit 11.1's `Load` return `os.ErrNotExist`-wrapped error**; Unit 11.3 introduces
   `ErrToolsNotFound` and `Resolve` translates `os.ErrNotExist` → `ErrToolsNotFound` at the
   boundary. Adds a translation layer for no semantic benefit.

Strongly recommend fix 1. The current plan leaves Unit 11.1's builder guessing.

### 3. [Axis: acceptance-criteria-coverage] [severity: medium] Unit 11.3 acceptance double-counts `os.IsNotExist` as an acceptable alternative to `errors.Is(err, domain.ErrToolsNotFound)`, weakening the test contract

Unit 11.3 acceptance:

> Returns `ToolManifest{}, nil` when file is absent (uses `errors.Is(err, domain.ErrToolsNotFound)` or `os.IsNotExist` pattern — builder decides; `domain.ErrToolsNotFound` is new and must be added to `internal/domain/errors.go`).

"Builder decides" + listing the sentinel AND `os.IsNotExist` as alternatives is a fork that
breaks test reproducibility — a falsification reviewer (sibling QA) can construct a case
where `Load` wraps `os.ErrNotExist` directly, `Resolve` checks `errors.Is(err, os.ErrNotExist)`,
and `domain.ErrToolsNotFound` is added but never read — silent dead code. The acceptance
should bind one shape:

> Returns `ToolManifest{}, nil` when `.valv/tools.toml` is absent at
> `projectDir + "/" + ToolsFilePath`. Detected via `errors.Is(err, domain.ErrToolsNotFound)`
> against the error returned by `Load`. `ErrToolsNotFound` is the canonical sentinel for
> "tools file absent" — no `os.IsNotExist` shortcut.

(This also resolves Finding 2: the sentinel is the contract surface, declared in 11.1,
consumed in 11.3.)

### 4. [Axis: specify-block-well-formedness] [severity: medium] Unit 11.2 acceptance asserts "Tool name must be a non-empty string with no whitespace" but never specifies what counts as whitespace, nor whether unicode whitespace is rejected

Acceptance bullet:

> Tool name must be a non-empty string with no whitespace.

Test surface (`TestValidate` covers "name with spaces") leaves uncovered: tabs, leading/trailing whitespace
that the user might think is fine, unicode whitespace (U+00A0, U+200B), zero-width name
(`""` already covered by "non-empty" but worth being explicit), and dots / slashes / OS-reserved
characters (the name will eventually become an install target in DROP_12). Yes/no-verifiability
fails: two reasonable builders would write two different `strings.ContainsAny`/`unicode.IsSpace`
checks and both would believe they pass the AC.

Sharpen to one of:

1. "Tool name matches regex `^[a-zA-Z0-9._-]+$`. Test cases: valid `mage`, `go-1.22`, `ta`;
   invalid empty string, `with space`, `with\ttab`, `with nbsp`, `with/slash`,
   `with.dot.allowed` (test asserts dot allowed)."
2. "Tool name is non-empty after `strings.TrimSpace` AND contains no ASCII whitespace
   (validated via `strings.ContainsAny(name, \" \\t\\n\\r\")`). Test cases: spaces, tabs,
   newlines, empty, all-whitespace."

Pick one and write it. The current bullet is vague enough that build-QA can't gate on it.

### 5. [Axis: specify-block-well-formedness] [severity: low] Unit 11.4 acceptance says `valv tools list` should print `<name>: <version>` for string-form and `<name>: <source> (custom)` for object-form — but does NOT specify the empty-tools formatting test for `validate` subcommand

`TestToolsValidate` is required to "cover" both subcommands but the empty-tools success
path for `validate` is unstated: does `valv tools validate` exit 0 with
`"tools.toml is valid"` when `.valv/tools.toml` is absent? The list subcommand has an
explicit empty-state string; the validate subcommand inherits "exits 0 on success" but the
empty manifest case is the boundary the planner did NOT spell out. Likely behavior: yes,
because `Resolve` returns `ToolManifest{}, nil`. But the AC should say so:

> `valv tools validate` exits 0 with `"tools.toml is valid"` when `.valv/tools.toml` exists
> and parses/validates cleanly. Exits 0 with `"no tools declared — .valv/tools.toml is
> absent"` (or equivalent, matching list's notice) when the file is absent. Exits 1 with
> error message on parse or validation failure.

Otherwise the falsification reviewer will construct a "validate" call in an empty project
and assert undefined behavior.

## Confirmed Sound (no finding required)

These planner claims I verified and they hold:

- **`BurntSushi/toml v1.6.0` availability**: confirmed in `go.mod` line 10.
- **`BurntSushi/toml` cannot natively decode heterogeneous `map[string]T`**: confirmed via
  Context7 `/burntsushi/toml` — the library exposes `UnmarshalTOML` and `toml.Primitive`
  precisely because the reflection-based decoder cannot handle sum-type values. The
  planner's two stated approaches (custom `UnmarshalTOML` or `map[string]toml.Primitive` +
  deferred decode) are both documented patterns.
- **`internal/config/Load` template**: confirmed at `internal/config/config.go:52-72` —
  exact `os.Stat` guard → `toml.DecodeFile` → `meta.Undecoded()` → wrapped errors shape
  the planner cites. Builder can copy the structure directly.
- **`internal/tools/` package is new**: confirmed via filesystem (`ls main/internal/`
  returns adapters, cli, config, domain, logging, output, pathutil, progress, project,
  services, tui — no `tools/`).
- **`internal/cli/tools.go` is new**: confirmed via filesystem (`ls internal/cli/tools*`
  returns nothing).
- **`ErrToolsNotFound` is new**: confirmed via Hylla `hylla_search_keyword` (zero results)
  AND `internal/domain/errors.go` Read (only `ErrConfigNotFound`, `ErrUnsupportedOS`,
  `ErrUnboundProject`, `ErrInvalidOutput`, `ErrInvalidLogLevel`, `ErrInvalidDBPath`,
  `ErrNotFound` exist today).
- **`internal/cli/global.go` reference for output helper pattern**: confirmed exists, uses
  cobra + `internal/output`. Builder guidance is grounded.
- **4-unit serial chain is compile-safe at each boundary**:
  - 11.1 → 11.2: 11.2's `Validate(m ToolManifest)` consumes the type 11.1 defines. Boundary
    holds.
  - 11.2 → 11.3: 11.3's `Resolve` calls `Load` (11.1) then `Validate` (11.2). Boundary holds
    *if* finding 2 is resolved (sentinel sequencing).
  - 11.3 → 11.4: 11.4's CLI commands consume `Resolve` from 11.3. Boundary holds.
- **`mage testPkg` per-package coverage gate**: confirmed at `magefile.go:117-143` —
  `TestPkg` does enforce the gate, but at `coverageThreshold = 60.0`, not 70 (Finding 1).
- **Drop-level acceptance criteria 1-7, 9 map to unit acceptance**: AC1 → 11.1+11.2+11.3
  pkg compile; AC2/AC3 → 11.1 `Load` acceptance; AC4 → 11.3 `Resolve` acceptance; AC5 →
  11.2 `Validate` acceptance; AC6/AC7 → 11.4 CLI acceptance; AC9 → drop-end verify (Phase
  6, not a unit). Coverage is contiguous. AC8 alone is broken (Finding 1).

## Recommendation

Loop back to planner for one revision pass focused on:

1. Restate AC8 (Finding 1) — pick option 2 unless dev wants the threshold-restore drop scope.
2. Move `ErrToolsNotFound` declaration to Unit 11.1 (Finding 2 + tightens Finding 3).
3. Bind Unit 11.3 to `errors.Is(err, domain.ErrToolsNotFound)` only — drop the `os.IsNotExist` alternative (Finding 3).
4. Sharpen Unit 11.2's tool-name validation rule (Finding 4).
5. Add empty-manifest behavior to Unit 11.4's `valv tools validate` acceptance (Finding 5).

The structural plan is sound — these are sharpening edits, not a redo.

## Hylla Feedback

None — Hylla answered everything needed. `hylla_search_keyword` for `ErrToolsNotFound`
correctly returned zero results (proving absence); `hylla_node_full` for
`internal/config/Load` returned the full function body with all dependency relations and
external docs context (`fmt.Errorf`, `os.Stat`, `toml.DecodeFile`, `MetaData.Undecoded`).
The Context7 query for `/burntsushi/toml` surfaced both `UnmarshalTOML` and `toml.Primitive`
patterns relevant to the heterogeneous-map claim verification.
