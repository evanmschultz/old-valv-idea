# DROP_11 — TOOL_DECLARATION

**State:** building
**Blocked by:** DROP_10 (done)
**Paths (expected):** new package `main/internal/tools/` for `.valv/tools.toml` parsing + validation; `main/internal/cli/tools.go` for the `valv tools` subcommand surface; testdata fixtures for parser coverage
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

### Schema Decisions (locked by planner, Round 3)

```toml
# .valv/tools.toml
[tools]
mage = "latest"
gh = "latest"
go = "1.22"
ta = { source = "github.com/evanmschultz/ta@main", install = "go install" }

# Tool names with dots, slashes, or other non-bare characters MUST be quoted in TOML.
# Example: "github.com/foo/bar" = { source = "github.com/foo/bar@main", install = "go install" }
# After BurntSushi/toml decodes this, the key in manifest.Tools is the unquoted string
# "github.com/foo/bar" — not a nested table.

# DROP_14/DROP_15 own the typed decode of these sections.
# DROP_11 captures them as toml.Primitive fields and calls meta.PrimitiveDecode
# on each to mark them as decoded (the decoded value is discarded — DROP_11
# takes no action on the contents).
[allowlist]
hosts = ["github.com", "proxy.golang.org"]

[env]
GOPRIVATE = "github.com/evanmschultz/*"
```

- **File format:** TOML — Valv already uses `BurntSushi/toml v1.6.0`; mise.toml also uses TOML per OSS survey.
- **`[tools]` map value type:** either a plain string (version shorthand) or an inline table `{ source = "...", install = "..." }` for custom tools not in a well-known registry. Requires a custom `UnmarshalTOML` or two-pass decode because BurntSushi/toml cannot natively decode a `map[string]T` where values are heterogeneous (string vs. hash).
- **Version spec format:** any non-empty string is valid at declaration time (`"latest"`, `"1.22"`, `"stable"`, git shas). DROP_12 enforces install-time resolution semantics. `"latest"` and `path:`-style local overrides deferred to DROP_12 — DROP_11 stores any version string verbatim.
- **Custom install support (v1):** YES — `{ source = "github.com/evanmschultz/ta@main", install = "go install" }` is valid in v1. Both `source` and `install` are required when using object form; validation errors if only one is present.
- **Tool name validation:** regex `^[a-zA-Z0-9]+$|^[a-zA-Z0-9][a-zA-Z0-9._/-]*[a-zA-Z0-9]$` — single-char alphanumeric, OR starts-and-ends alphanumeric with permitted chars (`.`, `_`, `/`, `-`) in between. Permits `github.com/foo/bar`-style names. RE2-compatible (alternation + character classes only; no lookahead).
  - **Accept by design (planner pin):** multiple consecutive `.`, `/`, or `-` characters within the name (e.g. `a..b`, `a//b`, `a---b`). Path-like names (`github.com/foo/bar`) and namespaced names (`org.tool.subname`) are valid. The regex is intentionally permissive about run-length of separator characters because doubling those characters is uncommon and not worth a more complex regex to catch.
  - **Reject by design (planner pin):** empty string; trailing punctuation (`bad-name-`, `name.`, `name/`); leading dash; leading `_` (must start alphanumeric — chosen for clarity, `_` would suggest internal/private semantics inappropriate for a public tool list); whitespace anywhere; `!` and other non-permitted characters; non-ASCII names (RE2 `[a-zA-Z0-9]` is ASCII-only by design; full Unicode tool names require explicit support — YAGNI for v1).
- **Reserved tool names:** none in v1. DROP_12 decides what it can install.
- **Max tool count:** 50. Validation returns a clear error if exceeded.
- **`[allowlist]` and `[env]` blocks:** captured as `toml.Primitive` fields. `ToolManifest` declares `Allowlist toml.Primitive \`toml:"allowlist"\`` and `Env toml.Primitive \`toml:"env"\``. After `toml.DecodeFile`, the parser **explicitly calls `meta.PrimitiveDecode` on each `toml.Primitive` field** (with a discarded target — DROP_11 takes no action on contents). This is required because declaring `toml.Primitive` fields alone is **NOT** enough — empirical testing against `BurntSushi/toml v1.6.0` (planner-run scratch program against the schema in this drop) confirms `meta.Undecoded()` still flags the inline keys (`allowlist.hosts`, `env.GOPRIVATE`) until `PrimitiveDecode` is called:

  ```
  undecoded BEFORE PrimitiveDecode: [allowlist.hosts env.GOPRIVATE]
  undecoded AFTER  PrimitiveDecode: []
  ```

  The Round 2/3 Context7-inferred claim that `toml.Primitive` declaration alone marked the section as decoded was wrong; this Round 4 revision reverses it. DROP_14/DROP_15 will re-call `meta.PrimitiveDecode` with typed targets when they implement those sections. A file with a top-level section whose name is NOT `tools`, `allowlist`, or `env` still triggers the undecoded-key error (intentional — schema is explicit).
- **File absent:** `Resolve` returns `ToolManifest{}, nil`. A project without `.valv/tools.toml` is valid — it declares no tools.
- **Empty manifest predicate:** `len(m.Tools) == 0`. The presence of `[allowlist]` or `[env]` sections does NOT make the manifest non-empty for DROP_11's purposes — "empty" means no tools declared, regardless of forward-compat sections.
- **TOML bare-key quoting:** TOML bare keys permit only `[A-Za-z0-9_-]`. Any tool name containing `.`, `/`, or other non-bare characters MUST be quoted in `.valv/tools.toml` (e.g. `"github.com/foo/bar" = { ... }`). After `toml.DecodeFile`, the key in `manifest.Tools` is the unquoted string `github.com/foo/bar` — not a nested table. Builders and users must be aware: an unquoted `github.com/foo/bar = ...` in TOML creates nested tables, not a single map entry, and will trigger an unknown-key error.
- **Package shape:** flat `internal/tools/` — consistent with `internal/config/`'s flat layout. No subpackages; YAGNI.
- **CLI surface:** single subcommand `valv tools validate` (parse + validate, exit 0 on success, exit 1 with error on failure). `valv tools list` is cut (dev decision Y3). Wired via `internal/cli/tools.go` + `internal/cli/root.go`.

### Acceptance Criteria (drop-level)

1. `internal/tools/` package compiles with zero vet warnings (`mage testPkg ./internal/tools/`).
2. `Load(path)` returns a correctly-typed `ToolManifest` for a well-formed `.valv/tools.toml` covering string-value tools, object-value tools, `[allowlist]`, and `[env]` sections.
3. `Load(path)` returns a wrapped error for malformed TOML, unknown top-level keys (beyond `tools`/`allowlist`/`env`), missing fields on object-form tools, and tools count > 50.
4. `Resolve(projectDir)` returns empty manifest (nil error) when `.valv/tools.toml` is absent (uses `errors.Is(err, domain.ErrToolsNotFound)`); returns parsed + validated manifest when present.
5. `Validate(m ToolManifest)` returns nil for a valid manifest; returns a descriptive error for each invalid shape.
6. `valv tools validate` exits 0 and prints `"tools.toml is valid"` on success; exits 1 with a human-readable error on parse or validation failure; exits 0 and prints `"no tools declared"` when manifest is empty (`len(m.Tools) == 0`).
7. `mage testPkg ./internal/tools/` reports ≥ 70% coverage for the package (enforced at Unit 11.4 completion).
8. `magefile.go` `coverageThreshold` constant is `70.0` with TODO removed. `mage test` passes clean across all packages (enforced at Unit 11.5, the final unit).
9. `mage test` passes clean across all packages after all units are done.

### Units

---

#### Unit 11.1 — Schema types, parser, and ErrToolsNotFound sentinel

**State:** done

**Paths:**
- `internal/tools/tools.go` (new — package declaration, exported types, `Load` function)
- `internal/tools/tools_test.go` (new — table-driven tests for `Load`)
- `internal/tools/testdata/valid_simple.toml` (new — simple string-value tools fixture)
- `internal/tools/testdata/valid_objects.toml` (new — mixed string + object tools + allowlist + env fixture)
- `internal/tools/testdata/valid_quoted_names.toml` (new — fixture with a quoted-key entry, e.g. `"github.com/foo/bar" = { source = "github.com/foo/bar@main", install = "go install" }`, proving the decode produces a single flat map entry in `manifest.Tools`, not a nested table)
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
- `Load(path string) (ToolManifest, error)` (new): uses `os.Stat` guard (returns `fmt.Errorf("...: %w", domain.ErrToolsNotFound)` when file absent), `toml.DecodeFile`, then — **before the strict `meta.Undecoded()` check** — calls `meta.PrimitiveDecode(m.Allowlist, &discardA)` and `meta.PrimitiveDecode(m.Env, &discardE)` where `discardA` and `discardE` are local `map[string]any`. **Do NOT use `toml.Primitive` itself as the discard target** — empirical testing confirmed this leaves the inline keys flagged undecoded (defeating the purpose). The decoded `map[string]any` values are intentionally discarded; DROP_11 takes no action on their contents. After both `PrimitiveDecode` calls, `meta.Undecoded()` is the strict-check oracle — error if any key remains undecoded. `fmt.Errorf("...: %w", err)` wrapping at each boundary. Mirrors `internal/config/Load` — use Hylla node `github.com/evanmschultz/valv/internal/config/Load` as the canonical pattern.
- **`PrimitiveDecode` is mandatory.** Empirical run against `BurntSushi/toml v1.6.0` (planner Round 4 scratch program) confirmed that declaring `toml.Primitive` fields alone is NOT enough — `meta.Undecoded()` still returns `[allowlist.hosts env.GOPRIVATE]` for the canonical schema example until both `PrimitiveDecode` calls run. Builder MUST implement the two calls before the `Undecoded()` check; skipping them will reject valid forward-compat sections as unknown keys.
- `Load` handles both string-value and inline-table-value entries under `[tools]` via `(t *ToolSpec) UnmarshalTOML(v interface{}) error` implementing `toml.Unmarshaler`. Confirmed by `TestLoad` passing `testdata/valid_objects.toml`.
- `Load` returns error for `testdata/invalid_unknown_key.toml` (undecoded key triggers strict check).
- `Load` accepts files with `[allowlist]` and `[env]` sections without error after the two `PrimitiveDecode` calls run. Specifically: a TOML file containing `[allowlist]` with `hosts = [...]` AND `[env]` with `GOPRIVATE = "..."` decodes cleanly — `meta.Undecoded()` returns the empty slice after the two `PrimitiveDecode` calls.
- `TestLoad` table cases include `testdata/valid_quoted_names.toml`: the fixture contains `"github.com/foo/bar" = { source = "github.com/foo/bar@main", install = "go install" }`. The test asserts that `manifest.Tools` has exactly one entry with key `"github.com/foo/bar"` (the unquoted string). This proves TOML quoted-key handling is correct — the key is not split into nested tables.
- `TestLoad` table cases include a dedicated case for forward-compat sections — fixture `testdata/valid_objects.toml` (which already includes `[allowlist]` + `[env]` sections per its description) loads without error and `len(manifest.Tools)` matches its declared tool count. Builder verifies this case explicitly fails without the two `PrimitiveDecode` calls (smoke check during dev).
- `mage testPkg ./internal/tools/` passes. `mage testPkg ./internal/domain/` passes (new sentinel is additive; existing tests unaffected). Coverage gate for `internal/tools/` not yet enforced — that is enforced after Unit 11.4 completes the package.

**Notes:**
- TOML bare keys permit only `[A-Za-z0-9_-]`. Tool names containing `.`, `/`, or other non-bare characters MUST be quoted in `.valv/tools.toml` (e.g. `"github.com/foo/bar" = { ... }`). An unquoted `github.com/foo/bar = ...` would create nested TOML tables (`[github]`, `[github.com]`, etc.) and NOT produce the expected flat map key — it would trigger the unknown-key error. The `valid_quoted_names.toml` fixture exercises this boundary explicitly.

**Blocked by:** nothing (first coding unit)

---

#### Unit 11.2 — Validation logic

**State:** done

**Paths:**
- `internal/tools/validate.go` (new — `Validate` function + validation rules)
- `internal/tools/validate_test.go` (new — table-driven tests for all validation rules)
- `internal/tools/testdata/invalid_object_missing_install.toml` (new — object form with `source` but missing `install` field; belongs in this unit because `Load` alone does not reject it — `Validate` does)

**Packages:** `internal/tools/` (same package as 11.1)

**Acceptance:**
- `Validate(m ToolManifest) error` (new, not yet in tree) exported from `internal/tools/`. Returns nil for a valid manifest.
- Validation rules enforced:
  - Tool name matches `^[a-zA-Z0-9]+$|^[a-zA-Z0-9][a-zA-Z0-9._/-]*[a-zA-Z0-9]$` — single-char alphanumeric, OR starts-and-ends alphanumeric with permitted chars in between. See Schema Decisions block above for the full accept/reject pin.
  - Object-form tool: both `Source` and `Install` non-empty; `Version` must be empty (implicit in `source` ref).
  - String-form tool: `Version` non-empty; `Source` and `Install` must be empty.
  - Total tool count ≤ 50.
- Returns first violation as a descriptive `fmt.Errorf(...)` — not a multi-error (consistent with `internal/config` approach).
- `TestValidate` table cases:
  - **Valid:** string-form tools (`"mage" = "latest"`); object-form tools; empty tools map; `"github.com/foo/bar"` (path-like name); `"go-1.22"`; `"a..b"` (double dot — accepted by design); `"a---b"` (multiple consecutive dashes — accepted by design); `"a/b/c"` (multi-segment path); count = 0; count = 50 (boundary).
  - **Invalid:** missing `install` field on object-form; embedded space `"with space"`; leading dash `"-bad"`; `!` character `"bad!char"`; trailing dash `"bad-name-"`; trailing dot `"name."`; trailing slash `"name/"`; leading underscore `"_underscore"` (rejected by design — must start alphanumeric); count = 51 (boundary).
- The regex is `regexp.MustCompile(`^[a-zA-Z0-9]+$|^[a-zA-Z0-9][a-zA-Z0-9._/-]*[a-zA-Z0-9]$`)`. Builder must verify it compiles and that ALL of the above valid/invalid cases match the expected verdict. The doubled-separator accept cases (`"a..b"`, `"a---b"`) and the leading-underscore reject case are the planner-pinned edge cases — they explicitly test that the design choice in the Schema Decisions block is honored by the implementation.
- `mage testPkg ./internal/tools/` passes. Coverage gate not yet enforced (enforced after Unit 11.4).

**Blocked by:** Unit 11.1

---

#### Unit 11.3 — Per-project resolver

**State:** done

**Paths:**
- `internal/tools/resolve.go` (new — `Resolve` function + `ToolsFilePath` constant)
- `internal/tools/resolve_test.go` (new — table-driven tests for absent file, valid file, invalid file)

**Packages:** `internal/tools/` (same package as 11.1 + 11.2)

**Acceptance:**
- `ToolsFilePath = ".valv/tools.toml"` constant (new, not yet in tree) exported from `internal/tools/`.
- `Resolve(projectDir string) (ToolManifest, error)` (new, not yet in tree): joins `projectDir + "/" + ToolsFilePath`, calls `Load`, then calls `Validate`. Returns `ToolManifest{}, nil` when `errors.Is(err, domain.ErrToolsNotFound)` — file absent is not an error. Any other error from `Load` or any error from `Validate` is returned wrapped. Does NOT fall back to `os.IsNotExist` — bind to sentinel only.
- `TestResolve` table cases: temp dir with no `.valv/` dir (returns empty manifest, nil error), temp dir with valid fixture (returns parsed manifest), temp dir with invalid fixture (returns wrapped error from `Validate`).
- `mage testPkg ./internal/tools/` passes. Coverage gate not yet enforced at this unit — 70% gate is enforced at Unit 11.4 which completes the package with CLI-side integration.
- `mage testPkg ./internal/domain/` still passes (no domain changes in this unit).

**Blocked by:** Unit 11.2

---

#### Unit 11.4 — CLI surface

**State:** done

**Paths:**
- `internal/cli/tools.go` (new — `newToolsCommand`, `newToolsValidateCommand`)
- `internal/cli/tools_test.go` (new — table-driven tests for `validate` subcommand)
- `internal/cli/root.go` (existing — add `toolsCmd` with `GroupID = "runtime"` and include in `cmd.AddCommand(...)`)

**Packages:** `internal/cli/` (existing package), `internal/tools/` (read-only dependency), `internal/project/` (read-only dependency — `project.Detect()`)

**Acceptance:**
- `newToolsCommand() *cobra.Command` (new, not yet in tree): cobra command `tools` with one subcommand `validate`. No `list` subcommand (cut per dev decision Y3).
- `valv tools validate`: calls `project.Detect()` (signature: `func Detect() (project.Result, error)`, source: `internal/project/project.go:19`) to resolve the project root from CWD, then passes `result.Root` to `tools.Resolve(result.Root)`. Does NOT use raw `os.Getwd()` — using `project.Detect()` means running `valv tools validate` from a subdirectory finds the project's `.valv/tools.toml` at the git root, matching the behavior of `valv claude` and `valv codex`. If `project.Detect()` returns an error, exits 1 with a wrapped error message.
- Exits 0 and prints `"tools.toml is valid"` on a valid non-empty manifest (`len(m.Tools) > 0`). Exits 0 and prints `"no tools declared"` when manifest is empty (`len(m.Tools) == 0` — this covers both absent file and present-but-empty `[tools]` map; presence of `[allowlist]` or `[env]` sections does not make the manifest non-empty). Exits 1 with a human-readable error message on parse or validation failure. Uses `laslig`-style output consistent with rest of `internal/cli/` — check `internal/cli/claude.go` and `internal/cli/global.go` for the pattern.
- `valv tools --help` works end-to-end (command registered in cobra tree).
- `internal/cli/root.go` integration: inside `newRootCommandWithPaths`, add `toolsCmd := newToolsCommand()` and `toolsCmd.GroupID = "runtime"`. Include `toolsCmd` in the existing `cmd.AddCommand(pathsCmd, versionCmd, statusCmd, codexCmd, claudeCmd, accountCmd, globalCmd, imageCmd)` call — add `toolsCmd` to that variadic list. Evidence: `root.go` line 137 is the single `cmd.AddCommand(...)` call; the local variable is `cmd` (line 47), not `rootCmd`. Groups confirmed: `"inspect"`, `"runtime"`, `"account"` declared at lines 108-112; `codexCmd` and `claudeCmd` use `GroupID = "runtime"`.
- `TestToolsValidate` uses temp dirs for fixtures — real file I/O, no mocks. Cases:
  1. Valid manifest (non-empty tools) — exits 0, prints `"tools.toml is valid"`.
  2. Empty manifest (present file, empty `[tools]` map) — exits 0, prints `"no tools declared"`.
  3. Missing file (no `.valv/tools.toml`) — exits 0, prints `"no tools declared"`.
  4. Invalid manifest (fails validation) — exits 1, error message contains failure detail.
  5. Zero-byte file (`.valv/tools.toml` exists, 0 bytes) — exits 0, prints `"no tools declared"`. Use `testdata/zero_byte.toml` (a zero-byte committed file) for this case.
  6. Permission-denied (`.valv/tools.toml` exists, `chmod 000`) — exits 1, error message contains wrapped permission error. Use `t.TempDir()` for this case (permission state does not survive `git add`).
  7. Directory-at-path (`.valv/tools.toml` is a directory, not a file) — exits 1 with a wrapped error originating from `os.Open` / read. The acceptance assertion is `errors.Is(err, syscall.EISDIR)` OR the error string contains `"is a directory"`. Empirical evidence (planner Round 4 scratch run on macOS Darwin against the Go stdlib): `os.Open(dir)` returns `nil` error; `io.ReadAll(f)` returns `"read <path>: is a directory"` with `errors.Is(err, syscall.EISDIR) == true`. `toml.DecodeFile` calls `os.Open` then reads, so it surfaces the same `EISDIR` error. The plan does NOT require `Load` to pre-check `info.IsDir()` — letting the underlying syscall produce the error is more idiomatic Go and the wrapped form is informative. Use `t.TempDir()` for this case (mkdir at the target path; permission state and dir-at-path do not survive `git add` cleanly).
- `internal/tools/testdata/zero_byte.toml` (new, zero-byte file — committed to testdata; zero-byte files survive `git add` cleanly).
- `mage testPkg ./internal/tools/` passes with ≥ 70% coverage across the whole package (this unit completes the package by exercising the full `Resolve` → `Validate` path via CLI tests — 70% gate enforced here).
- `mage testPkg ./internal/cli/` passes.

**Blocked by:** Unit 11.3

---

#### Unit 11.5 — Coverage threshold bump (drop-end verify gate)

**State:** todo

**Paths:**
- `magefile.go` (existing — change `coverageThreshold = 60.0` to `70.0`, remove TODO comment)

**Packages:** none (magefile, not a Go import package)

**Acceptance:**
- `magefile.go` lines 22-24: constant is `coverageThreshold = 70.0`. The `// TODO: restore to 70.0 after raising internal/adapters/docker coverage (see main/REFINEMENTS.md).` comment is removed.
- Builder runs `mage test` from `main/` immediately after the edit and reports the result verbatim in `BUILDER_WORKLOG.md`.
- If any package reports < 70% coverage, the builder does NOT silently fix it. Instead: list every failing package with its coverage percentage in the worklog, set unit state to `blocked`, and return to the orchestrator. The orchestrator then routes by failure class:
  - **New-package failure (`internal/tools/`):** the builder for THIS drop owns it. Route back to Unit 11.4 (which is the unit that finishes the package by exercising the CLI integration). The Unit 11.4 builder is responsible for raising `internal/tools/` coverage to ≥ 70% with additional tests before Unit 11.5 re-runs. Tightly scoped fix, same drop.
  - **Legacy-package failure (e.g. `internal/adapters/docker` — the known candidate from the pre-existing TODO in `magefile.go`):** out of scope for this drop's code units. Route to dev for triage. Dev decides between (a) raising that package's coverage as a new follow-on unit inside DROP_11 (e.g. Unit 11.6), or (b) rolling back the threshold bump and opening a separate coverage-cleanup drop. Either decision is recorded as a comment update in this PLAN.md.
- If `mage test` passes clean at 70%, unit is done and the drop is ready for Phase 6 close.

**Blocked by:** Unit 11.4

---

### Notes For Builder Agents

- **Mixed-type TOML decode:** BurntSushi/toml cannot natively decode a `map[string]ToolSpec` where values are heterogeneous (string vs. hash). The builder for Unit 11.1 must implement `(t *ToolSpec) UnmarshalTOML(fn func(interface{}) error) error` (the `toml.Unmarshaler` interface). See Context7 `/burntsushi/toml` for API reference.
- **TOML bare-key quoting:** TOML bare keys allow only `[A-Za-z0-9_-]`. Tool names with `.`, `/`, or other non-bare characters MUST be quoted in `.valv/tools.toml` (e.g. `"github.com/foo/bar" = { ... }`). An unquoted dotted name creates TOML nested tables and will fail the undecoded-key strict check. Unit 11.1's `valid_quoted_names.toml` fixture verifies this case explicitly — builder must include a `TestLoad` case for it.
- **`toml.Primitive` for forward-compat sections (MANDATORY `PrimitiveDecode`):** `ToolManifest.Allowlist` and `ToolManifest.Env` are typed `toml.Primitive`, NOT `AllowlistConfig` or `map[string]string`. **DROP_11's `Load` MUST call `meta.PrimitiveDecode` on each of these fields after `toml.DecodeFile` returns, BEFORE the `meta.Undecoded()` strict check.** Empirical run against `BurntSushi/toml v1.6.0` (planner Round 4) showed that declaring `toml.Primitive` fields alone does NOT mark the inline keys as decoded — `meta.Undecoded()` still returns `[allowlist.hosts env.GOPRIVATE]` for the canonical schema example until both `PrimitiveDecode` calls run. The decoded targets are discarded throwaway `map[string]any` — DROP_11 takes no action on the contents. DROP_14/DROP_15 will re-call `meta.PrimitiveDecode` with their typed targets when they implement those sections. (The Round 2/3 plan, citing Context7 "Delayed TOML Decoding," claimed the declaration alone was sufficient — that was wrong; this Round 4 revision reverses it.)
- **`domain.ErrToolsNotFound` is new:** Unit 11.1 adds this to `internal/domain/errors.go`. Unit 11.1's builder must run `mage testPkg ./internal/domain/` after adding it to confirm no breakage.
- **Sentinel-only absent-file check:** `Resolve` uses `errors.Is(err, domain.ErrToolsNotFound)` exclusively. Do not use `os.IsNotExist` as an alternative path.
- **Evidence pattern for `internal/config/Load`:** Hylla node `github.com/evanmschultz/valv/internal/config/Load` at `internal/config/config.go` — the exact function body is the canonical template for `Load` in Unit 11.1.
- **Tool name regex (Unit 11.2):** `^[a-zA-Z0-9]+$|^[a-zA-Z0-9][a-zA-Z0-9._/-]*[a-zA-Z0-9]$`. Use `regexp.MustCompile(...)` at package init or as a package-level var. RE2-compatible — no lookahead. The trailing-punctuation cases (`"bad-name-"`, `"name."`, `"name/"`) must all be rejected.
- **`project.Detect()` for project root (Unit 11.4):** `internal/project/project.go` exports `func Detect() (Result, error)` (line 19 — takes no arguments, calls `os.Getwd()` internally). The CLI's `runToolsValidate` calls `project.Detect()`, reads `result.Root`, then passes it to `tools.Resolve(result.Root)`. Do not use raw `os.Getwd()` — `project.Detect()` walks up to the git root, matching the behavior of `valv claude` and `valv codex`.
- **Empty-manifest predicate (Unit 11.4):** `len(m.Tools) == 0`. The presence of `[allowlist]` or `[env]` sections does NOT constitute a non-empty manifest for DROP_11's purposes.
- **Unit 11.4 test filesystem cases:** zero-byte file uses committed `testdata/zero_byte.toml` (zero-byte files survive `git add`). Permission-denied and directory-at-path cases use `t.TempDir()` — those states do not survive `git add` cleanly and must be created in-test.
- **CLI output style:** use `laslig` or `internal/output` patterns. Check `internal/cli/claude.go` and `internal/cli/global.go` for the output helper pattern. The `validate` command prints to stdout on success; errors go to stderr via cobra's error return.
- **Unit 11.5 risk:** `magefile.go` currently has `coverageThreshold = 60.0` with a TODO noting `internal/adapters/docker` is the known low-coverage package. Unit 11.5 is intentionally last — ALL coding units complete before this bump runs. If `mage test` fails after the bump, the builder surfaces the failure to the orchestrator and does not fix it inline — coverage work on an existing package is a separate concern.

## Notes

- Survey output is the evidence base for schema design. Schema is grounded in devcontainer.json + mise.toml patterns (cited above).
- This drop is intentionally narrow: parser + per-project resolution + `valv tools validate` CLI only. No image-build changes (DROP_12), no `valv run` adapter (DROP_13), no env-var injection (DROP_14), no network policy enforcement (DROP_15). `[allowlist]` and `[env]` sections are captured as `toml.Primitive` fields — parser accepts them and calls `meta.PrimitiveDecode` (discarding the decoded value) to satisfy the strict undecoded check. DROP_11 takes no further action on their contents.
- The 70% per-package coverage gate applies to the new `internal/tools/` package from day one (enforced at Unit 11.4 completion, which closes the package's test coverage after the CLI integration).
- The global `coverageThreshold` bump from 60% to 70% is intentionally deferred to Unit 11.5 (last unit). Bumping the global gate before all packages pass is a build-break risk.
- `valv tools list` is cut (dev decision, Y3). Only `valv tools validate` ships in DROP_11.
- `path:` local-source overrides and `"latest"` resolution semantics are deferred to DROP_12 (U1, U3). DROP_11 stores any version string verbatim.
- Unit ordering: 11.1 (schema+parser) → 11.2 (validation) → 11.3 (resolver) → 11.4 (CLI surface + 70% gate) → 11.5 (global coverage bump). Units 11.1–11.3 are unblocked by the magefile change. Unit 11.4 unblocked by 11.3. Unit 11.5 unblocked by 11.4.
