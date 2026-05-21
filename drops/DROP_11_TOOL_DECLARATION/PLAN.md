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

Recommended schema shape for planner consideration (NOT final — planner decides):

```toml
# .valv/tools.toml
[tools]
mage = "latest"
gh = "latest"
go = "1.22"
ta = { source = "github.com/evanmschultz/ta@main", install = "go install" }

[allowlist]
hosts = ["github.com", "proxy.golang.org", "sum.golang.org"]

[env]
GOPRIVATE = "github.com/evanmschultz/*"
```

Schema decisions for planner to nail down:

- TOML vs JSON vs YAML (recommend TOML — Valv already uses BurntSushi/toml).
- Version spec format (`latest` / `"1.22"` / git refs / `path:` local — borrow asdf's `.tool-versions` pattern?).
- Custom install commands vs only registry/well-known tools (the `ta` example above shows a custom `go install`).
- Allowlist + env keys in this drop or deferred to DROP_15/DROP_14 (probably defer — keep this drop scoped to tool declaration; cross-reference only).
- Validation rules (required fields, version syntax, max tool count, reserved names).

## Planner

### Objective

Add a declarative per-project toolchain schema to Valv. Projects place a `.valv/tools.toml` file at their project root declaring what tools the Valv container should provide. This drop is **schema + parser + validation + per-project resolution + introspection CLI only** — no image build changes, no install actions. The output is structured data: "this project declares these tools at these versions." DROP_12 reads that data to produce per-project layered images.

### Schema Decisions (locked by planner)

- **File format:** TOML — Valv already uses `BurntSushi/toml v1.6.0`; mise.toml also uses TOML per OSS survey.
- **`[tools]` map value type:** either a plain string (version shorthand) or an inline table `{ source = "...", install = "..." }` for custom tools not in a well-known registry. Requires a custom `UnmarshalTOML` or two-pass decode in the parser struct because BurntSushi/toml cannot natively decode a `map[string]T` where values are heterogeneous (string vs. hash).
- **Version spec format:** any non-empty string is valid at declaration time (`"latest"`, `"1.22"`, `"stable"`, git shas). DROP_12 enforces install-time resolution semantics.
- **Custom install support (v1):** YES — `{ source = "github.com/evanmschultz/ta@main", install = "go install" }` is valid in v1. Both `source` and `install` are required when using object form; validation errors if only one is present.
- **Reserved tool names:** none in v1. DROP_12 decides what it can install.
- **Max tool count:** 50. Validation returns a clear error if exceeded.
- **`[allowlist]` and `[env]` blocks:** included as **forward-compatible stubs**. The parser struct includes `Allowlist AllowlistConfig` and `Env map[string]string` with matching `toml:"allowlist"` / `toml:"env"` tags. The DROP_11 CLI does NOT act on them. Required so that the strict `meta.Undecoded()` check does not reject files that already declare these sections.
- **File absent:** `Resolve` returns empty `ToolManifest{}`, nil error. A project without `.valv/tools.toml` is valid — it declares no tools.
- **Package shape:** flat `internal/tools/` — consistent with `internal/config/`'s flat layout. No subpackages; YAGNI pressure is high here.
- **CLI surface:** `valv tools list` (print declared tools for the project in cwd) and `valv tools validate` (parse + validate, exit 0 on success, exit 1 with error on failure). Wired via `internal/cli/tools.go` + `internal/cli/root.go`.

### Acceptance Criteria (drop-level)

1. `internal/tools/` package compiles with zero vet warnings (`mage testPkg ./internal/tools/`).
2. `Load(path)` returns a correctly-typed `ToolManifest` for a well-formed `.valv/tools.toml` covering string-value tools, object-value tools, `[allowlist]`, and `[env]` sections.
3. `Load(path)` returns a wrapped error for malformed TOML, unknown keys, missing fields on object-form tools, and tools count > 50.
4. `Resolve(projectDir)` returns empty manifest (nil error) when `.valv/tools.toml` is absent; returns parsed manifest when present.
5. `Validate(m ToolManifest)` returns nil for a valid manifest; returns a descriptive error for each invalid shape.
6. `valv tools list` prints declared tools (name + version/source) for the project in the current working directory. Prints a "no tools declared" notice when `.valv/tools.toml` is absent.
7. `valv tools validate` exits 0 on success; exits 1 with a human-readable error on parse or validation failure.
8. `mage testPkg ./internal/tools/` reports ≥ 70% coverage for the package.
9. `mage test` passes clean across all packages after all units are done.

### Units

---

#### Unit 11.1 — Schema types and parser

**State:** todo

**Paths:**
- `internal/tools/tools.go` (new — package declaration, exported types, `Load` function)
- `internal/tools/tools_test.go` (new — table-driven tests for `Load`)
- `internal/tools/testdata/valid_simple.toml` (new — simple string-value tools fixture)
- `internal/tools/testdata/valid_objects.toml` (new — mixed string + object tools + allowlist + env fixture)
- `internal/tools/testdata/invalid_unknown_key.toml` (new — unknown top-level key)
- `internal/tools/testdata/invalid_object_missing_install.toml` (new — object form missing `install`)

**Packages:** `internal/tools/` (new)

**Acceptance:**
- `ToolSpec` type (new, not yet in tree) exported from `internal/tools/`. Holds `Version string`, `Source string`, `Install string`. A plain string value in TOML maps to `ToolSpec{Version: "<value>"}`. An inline table maps to `ToolSpec{Source: "...", Install: "..."}`.
- `AllowlistConfig` type (new) with `Hosts []string \`toml:"hosts"\``.
- `ToolManifest` type (new) with `Tools map[string]ToolSpec \`toml:"tools"\``, `Allowlist AllowlistConfig \`toml:"allowlist"\``, `Env map[string]string \`toml:"env"\``.
- `Load(path string) (ToolManifest, error)` (new): mirrors `internal/config/Load` — `os.Stat` guard, `toml.DecodeFile`, `meta.Undecoded()` strict check, `fmt.Errorf("...: %w", err)` wrapping.
- `Load` handles both string-value and inline-table-value entries under `[tools]` via a custom `UnmarshalTOML` or intermediate decode approach. Confirmed by `TestLoad` passing `testdata/valid_objects.toml`.
- `Load` returns error for `testdata/invalid_unknown_key.toml` (undecoded key present) and `testdata/invalid_object_missing_install.toml` when validation is wired (see Unit 11.2).
- `mage testPkg ./internal/tools/` passes. Coverage not yet required at 70% — that gate is enforced after Unit 11.3 completes the package.

**Blocked by:** nothing (foundational unit)

---

#### Unit 11.2 — Validation logic

**State:** todo

**Paths:**
- `internal/tools/validate.go` (new — `Validate` function + validation rules)
- `internal/tools/validate_test.go` (new — table-driven tests for all validation rules)
- `internal/tools/testdata/invalid_object_missing_install.toml` (may already exist from 11.1; validate.go adds semantic checks on top)

**Packages:** `internal/tools/` (same package as 11.1)

**Acceptance:**
- `Validate(m ToolManifest) error` (new, not yet in tree) exported from `internal/tools/`. Returns nil for a valid manifest.
- Validation rules enforced:
  - Tool name must be a non-empty string with no whitespace.
  - Object-form tool: both `Source` and `Install` non-empty; `Version` must be empty (version is implicit in `source` ref for custom tools).
  - String-form tool: `Version` non-empty; `Source` and `Install` empty.
  - Total tool count ≤ 50.
- Returns a descriptive `fmt.Errorf(...)` for each violation — not a multi-error; returns first violation encountered (consistent with `internal/config` approach).
- `TestValidate` covers: valid manifest, empty tools (valid), missing install field, name with spaces, count = 51.
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
- `Resolve(projectDir string) (ToolManifest, error)` (new, not yet in tree): joins `projectDir + "/" + ToolsFilePath`, calls `Load`, then calls `Validate`. Returns `ToolManifest{}, nil` when file is absent (uses `errors.Is(err, domain.ErrToolsNotFound)` or `os.IsNotExist` pattern — builder decides; `domain.ErrToolsNotFound` is new and must be added to `internal/domain/errors.go`).
- `domain.ErrToolsNotFound` sentinel (new, not yet in tree) added to `internal/domain/errors.go`.
- `TestResolve` table cases: directory with no `.valv/` dir (returns empty manifest, nil), directory with valid fixture, directory with invalid fixture (returns wrapped error).
- `mage testPkg ./internal/tools/` passes with ≥ 70% coverage across the whole package.
- `mage testPkg ./internal/domain/` still passes (new sentinel does not break existing tests).

**Blocked by:** Unit 11.2

---

#### Unit 11.4 — CLI surface

**State:** todo

**Paths:**
- `internal/cli/tools.go` (new — `newToolsCommand`, `newToolsListCommand`, `newToolsValidateCommand`)
- `internal/cli/tools_test.go` (new — table-driven tests for both subcommands)
- `internal/cli/root.go` (existing — add `rootCmd.AddCommand(newToolsCommand())`)

**Packages:** `internal/cli/` (existing package), `internal/tools/` (read-only dependency)

**Acceptance:**
- `newToolsCommand() *cobra.Command` (new, not yet in tree): cobra command `tools` with two subcommands `list` and `validate`.
- `valv tools list`: calls `tools.Resolve(cwd)` where `cwd` is `os.Getwd()`. Prints each tool as `<name>: <version>` (string-form) or `<name>: <source> (custom)` (object-form). Prints `"no tools declared — add .valv/tools.toml to declare the project toolchain"` when manifest is empty.
- `valv tools validate`: calls `tools.Resolve(cwd)`. Exits 0 and prints `"tools.toml is valid"` on success. Exits 1 with error message on parse or validation failure. Uses `laslig`-style output (consistent with rest of `internal/cli/`).
- `valv tools --help` works end-to-end (command is registered in cobra tree via `root.go`).
- `TestToolsList` and `TestToolsValidate` use `testdata/` fixtures (or temp dirs) — real file I/O, no mocks.
- `mage testPkg ./internal/cli/` passes. Full `mage test` passes clean.

**Blocked by:** Unit 11.3

---

### Notes For Builder Agents

- **Mixed-type TOML decode:** BurntSushi/toml cannot natively decode a `map[string]ToolSpec` where some values are strings and others are inline tables. The builder for Unit 11.1 must implement `(t *ToolSpec) UnmarshalTOML(fn func(interface{}) error) error` (the `toml.Unmarshaler` interface) or use an intermediate `map[string]toml.Primitive` + deferred decode. See Context7 `/burntsushi/toml` for API reference.
- **Strict undecoded-key check:** `internal/config/Load` rejects any undecoded key. Unit 11.1's `Load` must do the same. This means `ToolManifest` MUST declare `Allowlist` and `Env` fields with toml tags — otherwise any file containing those sections would be rejected.
- **`domain.ErrToolsNotFound` is new:** Unit 11.3 adds this to `internal/domain/errors.go`. Unit 11.3's builder must also run `mage testPkg ./internal/domain/` after adding it.
- **Evidence pattern for `internal/config/Load`:** Hylla node `github.com/evanmschultz/valv/internal/config/Load` at `internal/config/config.go` — the exact function body is the canonical template.
- **CLI output style:** use `laslig` or `internal/output` patterns for rendering. Check existing `internal/cli/claude.go` and `internal/cli/global.go` for the output helper pattern used in this codebase.

## Notes

- Survey output is the evidence base for schema design. Planner should ground the schema in devcontainer.json + mise.toml patterns (cited above) rather than inventing from scratch.
- This drop is intentionally narrow: parser + per-project resolution + simple `valv tools` introspection command. No image-build changes (DROP_12), no `valv run` adapter (DROP_13), no env-var injection (DROP_14), no network policy enforcement (DROP_15). The schema MAY include `[allowlist]` / `[env]` blocks as forward-compatible stubs the parser accepts but DROP_11 takes no action on.
- The 70% per-package coverage gate applies to the new `internal/tools/` package from day one.
