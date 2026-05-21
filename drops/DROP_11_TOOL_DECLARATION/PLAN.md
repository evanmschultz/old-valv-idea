# DROP_11 — TOOL_DECLARATION

**State:** planning
**Blocked by:** DROP_10 (done)
**Paths (expected):** new package `main/internal/tools/` for `.valv/tools.toml` parsing + validation; possibly `main/internal/cli/tools.go` for a `valv tools` subcommand surface; testdata fixtures for parser coverage
**Packages (expected):** `internal/tools/` (new), `internal/cli/`, possibly `cmd/valv/`
**PLAN.md ref:** main/PLAN.md → DROP_11_TOOL_DECLARATION row
**Workflow:** main/drops/WORKFLOW.md
**Started:** 2026-05-20
**Closed:** —

## Scope

Add a declarative per-project toolchain schema. Projects place a `.valv/tools.toml` file at their root declaring what tools the Valv container should make available (e.g. `mage`, `gh`, `ta`, `go`, language toolchains). This drop is **schema + parser + per-project resolution only** — no image build, no install action. The output is structured data: "this project declares these tools." DROP_12 reads that data and produces a per-project layered image.

This is the first slice in the sandbox-direction sequence (DROP_11 → DROP_12 → DROP_13 → DROP_14 → DROP_15 → DROP_16 release) confirmed by the dev 2026-05-20 after the OSS sandbox survey.

## Pre-Summit Research (2026-05-20 OSS sandbox survey, abbreviated)

Survey reviewed 8 projects (devcontainer.json, mise, asdf, nix-shell/flakes, GitHub Codespaces, Gitpod, ddev, claudebox). Load-bearing findings for THIS drop's schema design:

- **Declarative root-file is universal.** Every project uses some variant: `devcontainer.json`, `.tool-versions` (asdf), `mise.toml`, `flake.nix`, `.gitpod.yml`, `.ddev/config.yaml`, claudebox profile `.ini`s.
- **Two dominant declarative patterns:**
  - **devcontainer.json Features** — versioned OCI artifacts referenced by name (`ghcr.io/devcontainers/features/node:1`); pulls a remote-defined install script at build time. Layered on top of a base image. Used by Codespaces.
  - **mise.toml `[tools]`** — `node = "20"`, `python = "3.12"`, `go = "1.22"`. Local on-demand install into per-user cache; PATH augmentation.
- **Tool/version scoping is bottom-up.** Project file overrides user file overrides system file. mise and asdf both honor nested-directory precedence (a `mise.toml` in a subdir overrides the project root for that subtree). Out of scope for v1 of this drop — single root-level `.valv/tools.toml` only.
- **Multi-arch.** Docker images on Apple Silicon may be either `linux/arm64` or `linux/amd64` depending on the base image and what binaries are available; tools.toml should be arch-agnostic at the declaration level (image-build layer handles arch resolution).
- **claudebox uses per-project profile `.ini` files** that stack — e.g. `core + build-tools + python + ml`. Distinct from per-tool declaration; this drop chooses per-tool over per-profile because per-tool is more granular and composes more cleanly with arbitrary tool sets.

## Planner

### Objective

Add a declarative per-project toolchain schema to Valv. Projects place a `.valv/tools.toml` file at their project root declaring what tools the Valv container should provide. This drop is **schema + parser + validation + per-project resolution + introspection CLI only** — no image build changes, no install actions. The output is structured data: "this project declares these tools at these versions." DROP_12 reads that data to produce per-project layered images.

### Schema Decisions (locked by planner, Round 2)

```toml
# .valv/tools.toml
[tools]
mage = "latest"
gh = "latest"
go = "1.22"
ta = { source = "github.com/evanmschultz/ta@main", install = "go install" }

# DROP_14/DROP_15 own the typed decode of these sections.
# DROP_11 captures them as toml.Primitive fields — no action taken on contents.
[allowlist]
hosts = ["github.com", "proxy.golang.org"]

[env]
GOPRIVATE = "github.com/evanmschultz/*"
```

- **File format:** TOML — Valv already uses `BurntSushi/toml v1.6.0`; mise.toml also uses TOML per OSS survey.
- **`[tools]` map value type:** either a plain string (version shorthand) or an inline table `{ source = "...", install = "..." }` for custom tools not in a well-known registry. Requires a custom `UnmarshalTOML` or two-pass decode because BurntSushi/toml cannot natively decode a `map[string]T` where values are heterogeneous (string vs. hash).
- **Version spec format:** any non-empty string is valid at declaration time (`"latest"`, `"1.22"`, `"stable"`, git shas). DROP_12 enforces install-time resolution semantics. `"latest"` and `path:`-style local overrides deferred to DROP_12 — DROP_11 stores any version string verbatim.
- **Custom install support (v1):** YES — `{ source = "github.com/evanmschultz/ta@main", install = "go install" }` is valid in v1. Both `source` and `install` are required when using object form; validation errors if only one is present.
- **Tool name validation:** regex `^[a-zA-Z0-9][a-zA-Z0-9._/-]*$` — alphanumeric start, then alphanumeric plus `.`/`_`/`/`/`-`. Permits `github.com/foo/bar`-style names. Empty string and names starting with `-`, and names containing whitespace or `!` are invalid.
- **Reserved tool names:** none in v1. DROP_12 decides what it can install.
- **Max tool count:** 50. Validation returns a clear error if exceeded.
- **`[allowlist]` and `[env]` blocks:** captured as `toml.Primitive` fields. `ToolManifest` declares `Allowlist toml.Primitive \`toml:"allowlist"\`` and `Env toml.Primitive \`toml:"env"\``. During initial `toml.DecodeFile`, these fields receive their raw TOML values and are marked as decoded — `meta.Undecoded()` returns empty (strict check satisfied). DROP_11 takes no action on their contents. DROP_14/DROP_15 call `meta.PrimitiveDecode` to interpret them when those drops land. A file with a top-level section whose name is NOT `tools`, `allowlist`, or `env` still triggers the undecoded-key error (intentional — schema is explicit). Evidence: Context7 `/burntsushi/toml` "Delayed TOML Decoding" example confirms `toml.Primitive` fields satisfy the undecoded check.
- **File absent:** `Resolve` returns `ToolManifest{}, nil`. A project without `.valv/tools.toml` is valid — it declares no tools.
- **Package shape:** flat `internal/tools/` — consistent with `internal/config/`'s flat layout. No subpackages; YAGNI.
- **CLI surface:** single subcommand `valv tools validate` (parse + validate, exit 0 on success, exit 1 with error on failure). `valv tools list` is cut (dev decision Y3). Wired via `internal/cli/tools.go` + `internal/cli/root.go`.

### Acceptance Criteria (drop-level)

1. `magefile.go` `coverageThreshold` constant is `70.0` with TODO removed. `mage test` passes clean across all packages.
2. `internal/tools/` package compiles with zero vet warnings (`mage testPkg ./internal/tools/`).
3. `Load(path)` returns a correctly-typed `ToolManifest` for a well-formed `.valv/tools.toml` covering string-value tools, object-value tools, `[allowlist]`, and `[env]` sections.
4. `Load(path)` returns a wrapped error for malformed TOML, unknown top-level keys (beyond `tools`/`allowlist`/`env`), missing fields on object-form tools, and tools count > 50.
5. `Resolve(projectDir)` returns empty manifest (nil error) when `.valv/tools.toml` is absent (uses `errors.Is(err, domain.ErrToolsNotFound)`); returns parsed + validated manifest when present.
6. `Validate(m ToolManifest)` returns nil for a valid manifest; returns a descriptive error for each invalid shape.
7. `valv tools validate` exits 0 and prints `"tools.toml is valid"` on success; exits 1 with a human-readable error on parse or validation failure; exits 0 and prints `"no tools declared"` when manifest is empty (no `.valv/tools.toml` or empty `[tools]` map).
8. `mage testPkg ./internal/tools/` reports ≥ 70% coverage for the package.
9. `mage test` passes clean across all packages after all units are done.

### Units

---

#### Unit 11.0 — Coverage threshold bump

**State:** todo

**Paths:**
- `magefile.go` (existing — change `coverageThreshold = 60.0` to `70.0`, remove TODO comment)

**Packages:** none (magefile, not a Go import package)

**Acceptance:**
- `magefile.go` lines 22-24: constant is `coverageThreshold = 70.0`. The `// TODO: restore to 70.0 after raising internal/adapters/docker coverage (see main/REFINEMENTS.md).` comment is removed.
- Builder runs `mage test` from `main/` immediately after the edit and reports the result verbatim in `BUILDER_WORKLOG.md`.
- If any package reports < 70% coverage, the builder does NOT silently fix it. Instead: list every failing package with its coverage percentage in the worklog, set unit state to `blocked`, and return to the orchestrator. The orchestrator routes to dev: either raise that package's coverage in a follow-on unit, or roll back the bump and open a coverage-only drop.
- If `mage test` passes clean at 70%, unit is done.

**Blocked by:** nothing (foundational unit; must complete before any coding units begin)

---

#### Unit 11.1 — Schema types, parser, and ErrToolsNotFound sentinel

**State:** todo

**Paths:**
- `internal/tools/tools.go` (new — package declaration, exported types, `Load` function)
- `internal/tools/tools_test.go` (new — table-driven tests for `Load`)
- `internal/tools/testdata/valid_simple.toml` (new — simple string-value tools fixture)
- `internal/tools/testdata/valid_objects.toml` (new — mixed string + object tools + allowlist + env fixture)
- `internal/tools/testdata/invalid_unknown_key.toml` (new — unknown top-level key, e.g. `[network]` section)
- `internal/domain/errors.go` (existing — add `ErrToolsNotFound` sentinel)

**Packages:** `internal/tools/` (new), `internal/domain/` (existing — additive only)

**Acceptance:**
- `domain.ErrToolsNotFound = errors.New("tools file not found")` (new, not yet in tree) added to `internal/domain/errors.go` in the existing `var` block alongside `ErrConfigNotFound`. Pattern: `errors.New("...")`.
- `ToolSpec` type (new, not yet in tree) exported from `internal/tools/`. Holds `Version string`, `Source string`, `Install string`. A plain string TOML value maps to `ToolSpec{Version: "<value>"}`. An inline table maps to `ToolSpec{Source: "...", Install: "..."}`.
- `ToolManifest` type (new) with fields:
  - `Tools map[string]ToolSpec \`toml:"tools"\``
  - `Allowlist toml.Primitive \`toml:"allowlist"\``
  - `Env toml.Primitive \`toml:"env"\``
- `Load(path string) (ToolManifest, error)` (new): uses `os.Stat` guard (returns `fmt.Errorf("...: %w", domain.ErrToolsNotFound)` when file absent), `toml.DecodeFile`, `meta.Undecoded()` strict check (error if any key is undecoded), `fmt.Errorf("...: %w", err)` wrapping at each boundary. Mirrors `internal/config/Load` — use Hylla node `github.com/evanmschultz/valv/internal/config/Load` as the canonical pattern.
- `Load` handles both string-value and inline-table-value entries under `[tools]` via `(t *ToolSpec) UnmarshalTOML(fn func(interface{}) error) error` implementing `toml.Unmarshaler`. Confirmed by `TestLoad` passing `testdata/valid_objects.toml`.
- `Load` returns error for `testdata/invalid_unknown_key.toml` (undecoded key triggers strict check).
- `Load` accepts files with `[allowlist]` and `[env]` sections without error (they decode into `toml.Primitive` fields — not undecoded).
- `mage testPkg ./internal/tools/` passes. `mage testPkg ./internal/domain/` passes (new sentinel is additive; existing tests unaffected). Coverage gate for `internal/tools/` not yet enforced — that is enforced after Unit 11.3 completes the package.

**Blocked by:** Unit 11.0

---

#### Unit 11.2 — Validation logic

**State:** todo

**Paths:**
- `internal/tools/validate.go` (new — `Validate` function + validation rules)
- `internal/tools/validate_test.go` (new — table-driven tests for all validation rules)
- `internal/tools/testdata/invalid_object_missing_install.toml` (new — object form with `source` but missing `install` field; belongs in this unit because `Load` alone does not reject it — `Validate` does)

**Packages:** `internal/tools/` (same package as 11.1)

**Acceptance:**
- `Validate(m ToolManifest) error` (new, not yet in tree) exported from `internal/tools/`. Returns nil for a valid manifest.
- Validation rules enforced:
  - Tool name matches `^[a-zA-Z0-9][a-zA-Z0-9._/-]*$`. Invalid names: empty string, `"with space"`, `"-leading-dash"`, `"bad!char"`, `" name"` (leading unicode space).
  - Object-form tool: both `Source` and `Install` non-empty; `Version` must be empty (implicit in `source` ref).
  - String-form tool: `Version` non-empty; `Source` and `Install` must be empty.
  - Total tool count ≤ 50.
- Returns first violation as a descriptive `fmt.Errorf(...)` — not a multi-error (consistent with `internal/config` approach).
- `TestValidate` table cases: valid manifest with string-form tools, valid manifest with object-form tools, empty tools map (valid), missing `install` field, name with embedded space `"with space"`, leading-dash name `"-bad"`, name with `!` character `"bad!char"`, count = 51, count = 0 (valid), valid github-style name `"github.com/foo/bar"`, valid go version name `"go-1.22"`.
- `mage testPkg ./internal/tools/` passes. Coverage gate not yet enforced (enforced after 11.3).

**Blocked by:** Unit 11.1

---

#### Unit 11.3 — Per-project resolver

**State:** todo

**Paths:**
- `internal/tools/resolve.go` (new — `Resolve` function + `ToolsFilePath` constant)
- `internal/tools/resolve_test.go` (new — table-driven tests for absent file, valid file, invalid file)

**Packages:** `internal/tools/` (same package as 11.1 + 11.2)

**Acceptance:**
- `ToolsFilePath = ".valv/tools.toml"` constant (new, not yet in tree) exported from `internal/tools/`.
- `Resolve(projectDir string) (ToolManifest, error)` (new, not yet in tree): joins `projectDir + "/" + ToolsFilePath`, calls `Load`, then calls `Validate`. Returns `ToolManifest{}, nil` when `errors.Is(err, domain.ErrToolsNotFound)` — file absent is not an error. Any other error from `Load` or any error from `Validate` is returned wrapped. Does NOT fall back to `os.IsNotExist` — bind to sentinel only.
- `TestResolve` table cases: temp dir with no `.valv/` dir (returns empty manifest, nil error), temp dir with valid fixture (returns parsed manifest), temp dir with invalid fixture (returns wrapped error from `Validate`).
- `mage testPkg ./internal/tools/` passes with ≥ 70% coverage across the whole package (this unit completes the package — 70% gate enforced here).
- `mage testPkg ./internal/domain/` still passes (no domain changes in this unit).

**Blocked by:** Unit 11.2

---

#### Unit 11.4 — CLI surface

**State:** todo

**Paths:**
- `internal/cli/tools.go` (new — `newToolsCommand`, `newToolsValidateCommand`)
- `internal/cli/tools_test.go` (new — table-driven tests for `validate` subcommand)
- `internal/cli/root.go` (existing — add `toolsCmd` with `GroupID = "runtime"` and include in `cmd.AddCommand(...)`)

**Packages:** `internal/cli/` (existing package), `internal/tools/` (read-only dependency)

**Acceptance:**
- `newToolsCommand() *cobra.Command` (new, not yet in tree): cobra command `tools` with one subcommand `validate`. No `list` subcommand (cut per dev decision Y3).
- `valv tools validate`: calls `tools.Resolve(os.Getwd())`. Exits 0 and prints `"tools.toml is valid"` on a valid non-empty manifest. Exits 0 and prints `"no tools declared"` when manifest is empty (absent file or empty `[tools]` map). Exits 1 with a human-readable error message on parse or validation failure. Uses `laslig`-style output consistent with rest of `internal/cli/` — check `internal/cli/claude.go` and `internal/cli/global.go` for the pattern.
- `valv tools --help` works end-to-end (command registered in cobra tree).
- `internal/cli/root.go` integration: inside `newRootCommandWithPaths`, add `toolsCmd := newToolsCommand()` and `toolsCmd.GroupID = "runtime"`. Include `toolsCmd` in the existing `cmd.AddCommand(pathsCmd, versionCmd, statusCmd, codexCmd, claudeCmd, accountCmd, globalCmd, imageCmd)` call — add `toolsCmd` to that variadic list. Evidence: `root.go` line 137 is the single `cmd.AddCommand(...)` call; the local variable is `cmd` (line 47), not `rootCmd`. Groups confirmed: `"inspect"`, `"runtime"`, `"account"` declared at lines 108-112; `codexCmd` and `claudeCmd` use `GroupID = "runtime"`.
- `TestToolsValidate` uses temp dirs for fixtures — real file I/O, no mocks. Cases: valid manifest, empty manifest, missing file, invalid manifest (fails validation).
- `mage testPkg ./internal/cli/` passes. Full `mage test` passes clean.

**Blocked by:** Unit 11.3

---

### Notes For Builder Agents

- **Mixed-type TOML decode:** BurntSushi/toml cannot natively decode a `map[string]ToolSpec` where values are heterogeneous (string vs. hash). The builder for Unit 11.1 must implement `(t *ToolSpec) UnmarshalTOML(fn func(interface{}) error) error` (the `toml.Unmarshaler` interface). See Context7 `/burntsushi/toml` for API reference.
- **`toml.Primitive` for forward-compat sections:** `ToolManifest.Allowlist` and `ToolManifest.Env` are typed `toml.Primitive`, NOT `AllowlistConfig` or `map[string]string`. During initial `toml.DecodeFile`, both fields receive their raw TOML values and are marked decoded — `meta.Undecoded()` returns empty (strict check satisfied). Context7 `/burntsushi/toml` "Delayed TOML Decoding" example confirms this pattern. DROP_14/DROP_15 will call `meta.PrimitiveDecode` on these fields when they interpret the contents. DROP_11 does not call `PrimitiveDecode` at all.
- **`domain.ErrToolsNotFound` is new:** Unit 11.1 adds this to `internal/domain/errors.go`. Unit 11.1's builder must run `mage testPkg ./internal/domain/` after adding it to confirm no breakage.
- **Sentinel-only absent-file check:** `Resolve` uses `errors.Is(err, domain.ErrToolsNotFound)` exclusively. Do not use `os.IsNotExist` as an alternative path.
- **Evidence pattern for `internal/config/Load`:** Hylla node `github.com/evanmschultz/valv/internal/config/Load` at `internal/config/config.go` — the exact function body is the canonical template for `Load` in Unit 11.1.
- **CLI output style:** use `laslig` or `internal/output` patterns. Check `internal/cli/claude.go` and `internal/cli/global.go` for the output helper pattern. The `validate` command prints to stdout on success; errors go to stderr via cobra's error return.
- **Unit 11.0 risk:** `magefile.go` currently has `coverageThreshold = 60.0` with a TODO noting `internal/adapters/docker` is the known low-coverage package. If `mage test` fails after the bump, the builder surfaces the failure to the orchestrator and does not fix it inline — coverage work on an existing package is a separate concern.

## Notes

- Survey output is the evidence base for schema design. Schema is grounded in devcontainer.json + mise.toml patterns (cited above).
- This drop is intentionally narrow: parser + per-project resolution + `valv tools validate` CLI only. No image-build changes (DROP_12), no `valv run` adapter (DROP_13), no env-var injection (DROP_14), no network policy enforcement (DROP_15). `[allowlist]` and `[env]` sections are captured as `toml.Primitive` fields — parser accepts them, DROP_11 takes no action on their contents.
- The 70% per-package coverage gate applies to the new `internal/tools/` package from day one (enforced at Unit 11.3 completion).
- `valv tools list` is cut (dev decision, Y3). Only `valv tools validate` ships in DROP_11.
- `path:` local-source overrides and `"latest"` resolution semantics are deferred to DROP_12 (U1, U3). DROP_11 stores any version string verbatim.
