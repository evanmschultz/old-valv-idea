# DROP_11 Plan QA Proof — Round 2

**Verdict:** pass
**Reviewer:** go-qa-proof-agent
**Reviewed at:** 2026-05-20T00:00:00Z

## Round 1 Delta Verification

### P2 — `ErrToolsNotFound` sentinel moved to Unit 11.1

PASS. Unit 11.1 "Schema types, parser, and ErrToolsNotFound sentinel" lists `internal/domain/errors.go` in Paths (line 112) and includes `internal/domain/` in Packages (line 114). Acceptance bullet at line 117 specifies `domain.ErrToolsNotFound = errors.New("tools file not found")` added alongside `ErrConfigNotFound`. Pattern matches the existing `internal/domain/errors.go` sentinel block (`ErrConfigNotFound  = errors.New("config file not found")` at line 6). Unit 11.1 acceptance also requires `mage testPkg ./internal/domain/` passes (line 127). Delta landed.

### P3 — Unit 11.3 binds only to `errors.Is(err, domain.ErrToolsNotFound)`

PASS. Unit 11.3 acceptance line 171: "Returns `ToolManifest{}, nil` when `errors.Is(err, domain.ErrToolsNotFound)` ... Does NOT fall back to `os.IsNotExist` — bind to sentinel only." Notes For Builder Agents line 208 reinforces: "Sentinel-only absent-file check: `Resolve` uses `errors.Is(err, domain.ErrToolsNotFound)` exclusively. Do not use `os.IsNotExist` as an alternative path." No fork. Delta landed.

### P4 — Tool name regex locked + test cases enumerated

PASS. Schema Decisions line 59: regex `^[a-zA-Z0-9][a-zA-Z0-9._/-]*$` documented with permitted (alphanumeric + `.`/`_`/`/`/`-`) and forbidden (empty, leading `-`, whitespace, `!`) cases. Unit 11.2 acceptance line 147 restates the regex. Test cases at line 152 enumerate both valid (string-form, object-form, github-style `"github.com/foo/bar"`, go version `"go-1.22"`, empty map, count=0) and invalid (`""`, `"with space"`, `"-leading-dash"`, `"bad!char"`, `" name"`, count=51). Delta landed.

### P5 — Empty-manifest behavior for `valv tools validate`

PASS. Unit 11.4 acceptance line 193: "Exits 0 and prints `\"no tools declared\"` when manifest is empty (absent file or empty `[tools]` map)." Test case explicitly listed at line 196: "empty manifest, missing file". Drop-level Acceptance Criterion 7 also covers this contract. Delta landed.

### F1 — `cmd` variable + `cmd.AddCommand(...)` + correct GroupID

PASS, with strong evidence. Verified live against `internal/cli/root.go`:

- Line 47: `cmd := &cobra.Command{...}` — local var is `cmd`, not `rootCmd`. Plan correctly names `cmd`.
- Lines 108-112: groups declared are `"inspect"`, `"runtime"`, `"account"`. Plan correctly names `"runtime"`.
- Lines 126-129: `codexCmd.GroupID = "runtime"` and `claudeCmd.GroupID = "runtime"`. Parallelism with these two runtime commands is the planner's stated reasoning; correct.
- Line 134-135: `imageCmd.GroupID = "account"` — confirming the planner's TL;DR note that imageCmd uses `"account"`, not `"runtime"`. Plan picked the right group for `valv tools` (runtime tool, like `codex`/`claude`, not an account-management command).
- Line 137: single `cmd.AddCommand(pathsCmd, versionCmd, statusCmd, codexCmd, claudeCmd, accountCmd, globalCmd, imageCmd)` — exactly the pattern Unit 11.4 acceptance line 195 cites.

Plan tells the builder to add `toolsCmd` to the variadic list and set `toolsCmd.GroupID = "runtime"`. Delta landed correctly.

### F2 — `invalid_object_missing_install.toml` belongs to Unit 11.2

PASS. Unit 11.2 Paths line 140 lists `internal/tools/testdata/invalid_object_missing_install.toml` with the explicit note "belongs in this unit because `Load` alone does not reject it — `Validate` does." Unit 11.1 Paths line 109-111 lists only `valid_simple.toml`, `valid_objects.toml`, `invalid_unknown_key.toml` — `invalid_object_missing_install.toml` is not duplicated there. Delta landed.

### U1 / U3 — `path:` overrides + `"latest"` semantics deferred to DROP_12

PASS. Schema Decisions line 57: "`\"latest\"` and `path:`-style local overrides deferred to DROP_12 — DROP_11 stores any version string verbatim." Notes line 219 restates the deferral: "`path:` local-source overrides and `\"latest\"` resolution semantics are deferred to DROP_12 (U1, U3). DROP_11 stores any version string verbatim." Delta landed.

### P1 → Unit 11.0 — coverage threshold bump + dev-routing

PASS. Unit 11.0 "Coverage threshold bump" inserted at line 83 ahead of all other units. Acceptance:

- Line 93: "lines 22-24: constant is `coverageThreshold = 70.0`. The `// TODO: ...` comment is removed." Confirmed against live `magefile.go` — the TODO is at line 23 and `coverageThreshold = 60.0` is at line 24, matching the plan's "lines 22-24" claim exactly.
- Line 95: "If any package reports < 70% coverage, the builder does NOT silently fix it. Instead: list every failing package with its coverage percentage in the worklog, set unit state to `blocked`, and return to the orchestrator. The orchestrator routes to dev: either raise that package's coverage in a follow-on unit, or roll back the bump and open a coverage-only drop."

Explicit dev-routing language present — not "ensure tests pass". The "does NOT silently fix" wording mirrors the orchestrator brief verbatim. Delta landed cleanly.

### Y1 — object-form `{ source, install }` survives in v1

PASS. Schema Decisions line 58: "Custom install support (v1): YES — `{ source = \"github.com/evanmschultz/ta@main\", install = \"go install\" }` is valid in v1. Both `source` and `install` are required when using object form; validation errors if only one is present." Unit 11.1 acceptance line 118 defines `ToolSpec` with `Version string`, `Source string`, `Install string`. Delta landed.

### Y2 — `toml.Primitive` named fields (load-bearing correction)

PASS, with Context7 verification. The planner replaced the brief's `Unknown map[string]toml.Primitive` catch-all with explicit named `toml.Primitive` fields (`Allowlist toml.Primitive` and `Env toml.Primitive`). Verified via Context7 `/burntsushi/toml` "Delayed TOML Decoding with Primitive Values in Go" example:

```go
type Config struct {
    Mode       string
    DevConfig  toml.Primitive
    ProdConfig toml.Primitive
}
var config Config
metadata, err := toml.Decode(tomlData, &config)
```

The example uses named `toml.Primitive` fields exactly the same way. After `toml.Decode`, the example proceeds directly to `metadata.PrimitiveDecode(config.ProdConfig, ...)` — there is no `Undecoded()` check failure in that flow. Cross-referenced with the "Strict TOML Decoding" Context7 example: there, `extra_field` (no struct field) and `logging` (no struct field) appear in `metadata.Undecoded()`. By contrast, named struct fields — including `toml.Primitive`-typed ones — are NOT flagged as undecoded because they receive their values during the initial decode pass.

Conclusion: a `ToolManifest` with `Allowlist toml.Primitive \`toml:"allowlist"\`` and `Env toml.Primitive \`toml:"env"\`` WILL satisfy `meta.Undecoded()` for those sections. The planner's correction is factually correct and the strict-mode parser will not reject files containing `[allowlist]` and `[env]` blocks.

Delta landed correctly. Notes For Builder Agents line 206 also restates this with the right semantics ("both fields receive their raw TOML values and are marked decoded — `meta.Undecoded()` returns empty (strict check satisfied)").

### Y3 — `valv tools list` cut; only `validate` ships

PASS. Schema Decisions line 65: "single subcommand `valv tools validate` ... `valv tools list` is cut (dev decision Y3)." Unit 11.4 acceptance line 192: "cobra command `tools` with one subcommand `validate`. No `list` subcommand (cut per dev decision Y3)." Notes line 218: "`valv tools list` is cut (dev decision, Y3). Only `valv tools validate` ships in DROP_11." Three sites all agree. Delta landed.

### U2 — `valv tools` top-level, GroupID `"runtime"`

PASS. Schema Decisions line 65 ("Wired via `internal/cli/tools.go` + `internal/cli/root.go`") and Unit 11.4 acceptance lines 195 ("inside `newRootCommandWithPaths`, add `toolsCmd := newToolsCommand()` and `toolsCmd.GroupID = \"runtime\"`. Include `toolsCmd` in the existing `cmd.AddCommand(pathsCmd, versionCmd, statusCmd, codexCmd, claudeCmd, accountCmd, globalCmd, imageCmd)` call — add `toolsCmd` to that variadic list."). Top-level placement + `"runtime"` GroupID match the dev decision. Delta landed.

## New Findings (Round 2)

### Round 2 internal-consistency check

- **Unit 11.0 → 11.1 blocking chain.** Unit 11.0 `Blocked by: nothing` (line 98). Unit 11.1 `Blocked by: Unit 11.0` (line 129). Unit 11.2 `Blocked by: Unit 11.1` (line 155). Unit 11.3 `Blocked by: Unit 11.2` (line 176). Unit 11.4 `Blocked by: Unit 11.3` (line 199). Chain is linear, no gaps, no cycles. PASS.

- **Coverage gate deferral consistency.** Unit 11.1 acceptance line 127: "Coverage gate for `internal/tools/` not yet enforced — that is enforced after Unit 11.3 completes the package." Unit 11.2 acceptance line 153: "Coverage gate not yet enforced (enforced after 11.3)." Unit 11.3 acceptance line 173: "`mage testPkg ./internal/tools/` passes with ≥ 70% coverage across the whole package (this unit completes the package — 70% gate enforced here)." Drop-level Acceptance Criterion 8 (line 76) restates the 70% gate. Three units + drop-level criterion all align on Unit 11.3 as the gate-enforcement point. PASS.

- **No `AllowlistConfig` typed stub survivors.** Searched the plan for "AllowlistConfig" — found only in Notes For Builder Agents line 206 as an explicit NEGATIVE ("are typed `toml.Primitive`, NOT `AllowlistConfig` or `map[string]string`"). No stale references suggesting a typed Allowlist struct in v1. PASS.

- **`os.IsNotExist` not referenced.** Searched the plan for "IsNotExist" — found only in Unit 11.3 acceptance line 171 ("Does NOT fall back to `os.IsNotExist`") and Notes line 208 ("Do not use `os.IsNotExist` as an alternative path"). Both are explicit negatives. No residual `os.IsNotExist` usage proposed. PASS.

- **`valv tools list` not referenced as a deliverable.** Searched for "list" in CLI context — found only the explicit cut statements in lines 65, 192, 218. No leftover acceptance bullet referencing a `list` subcommand. PASS.

- **`magefile.go` line numbers.** Plan line 93 cites "lines 22-24" of `magefile.go`. Verified live: line 22 is `const (`, line 23 is the TODO comment, line 24 is `coverageThreshold = 60.0`. Exact match. PASS.

### Minor observations (not findings, recorded for orchestrator)

- **Tool name regex `^[a-zA-Z0-9][a-zA-Z0-9._/-]*$` and the example `"github.com/foo/bar"`.** The regex permits `/` in non-leading positions, so `github.com/foo/bar` is valid (starts with `g`, then alphanumeric + `.`/`/`). The unicode-space negative test case `" name"` (line 152) is sensible since `\s` characters are outside `[a-zA-Z0-9]`. No issue — regex and tests align.

- **`Resolve(projectDir)` uses string concatenation `projectDir + "/" + ToolsFilePath`.** Acceptance line 171 phrasing. For platform consistency `filepath.Join(projectDir, ToolsFilePath)` would be more idiomatic, but Valv is macOS+Docker-only per AGENTS.md § 3 so the `"/"` separator works. Not a blocking issue; builder may use `filepath.Join` at their discretion since the plan does not lock the join mechanism.

- **`ToolManifest.Tools` map struct tag.** Acceptance line 120 uses `\`toml:"tools"\``. Standard BurntSushi/toml pattern, matches `internal/config/Config`'s pattern (`Output OutputConfig \`toml:"output"\``). PASS.

## Hylla Feedback

None — only consulted Read tool against live `internal/cli/root.go`, `internal/domain/errors.go`, `internal/config/config.go`, and `magefile.go` (the planner already cited specific line numbers; verification was direct file reads, no Hylla needed). Context7 query for the `/burntsushi/toml` Primitive semantics returned exactly the load-bearing example needed.

## Summary

All 12 Round 1 deltas verified clean. The Y2 `toml.Primitive` correction is technically sound per Context7. The F1 root.go integration is grounded in the actual live file. Unit 11.0's dev-routing language is explicit and matches the orchestrator brief. The plan is internally consistent: blocking chain is linear, coverage-gate deferral is uniform across units 11.1-11.3, no residual references to cut features (`valv tools list`, `AllowlistConfig`, `os.IsNotExist`).

**Verdict: PASS.** Plan is ready to proceed to Phase 3 discussion or directly to builder dispatch.
