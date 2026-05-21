# DROP_11 Plan QA Proof — Round 3

**Verdict:** pass
**Reviewer:** general-purpose acting as go-qa-proof-agent
**Reviewed at:** 2026-05-21T05:45:00Z

## Round 2 Delta Verification

### C1 (HIGH) — Reorder Unit 11.0 → Unit 11.5 (last)

**Pass.**

- PLAN.md § Units presents the chain `Unit 11.1` (line 90) → `Unit 11.2` (line 126) → `Unit 11.3` (line 153) → `Unit 11.4` (line 174) → `Unit 11.5` (line 207). No `Unit 11.0` heading remains.
- Unit 11.1 `Blocked by: nothing (first coding unit)` at line 122.
- Unit 11.2 `Blocked by: Unit 11.1` at line 149.
- Unit 11.3 `Blocked by: Unit 11.2` at line 170.
- Unit 11.4 `Blocked by: Unit 11.3` at line 203.
- Unit 11.5 `Blocked by: Unit 11.4` at line 222.
- Drop-level AC7 at line 82 enforces the 70% per-package coverage gate at Unit 11.4 completion. Drop-level AC8 at line 83 enforces `coverageThreshold = 70.0` + clean `mage test` at Unit 11.5. The 70% gate now lands at Unit 11.5 as the final gate (per the deltas listed in the spawn appendix), with Unit 11.4 enforcing the per-package coverage and Unit 11.5 enforcing the global magefile bump — split correctly across the two final units.
- § Notes For Builder Agents line 249 explicitly states `Unit ordering: 11.1 (schema+parser) → 11.2 (validation) → 11.3 (resolver) → 11.4 (CLI surface + 70% gate) → 11.5 (global coverage bump)`.

### C2 (MEDIUM) — TOML bare-key quoting

**Pass.**

- § Schema Decisions line 70 contains the dedicated **TOML bare-key quoting** bullet: explains bare-key restriction, requires quoting for dotted/slashed names, calls out that decoded result lands as `manifest.Tools["github.com/foo/bar"]` not as a nested table.
- Unit 11.1 Paths line 99 lists `internal/tools/testdata/valid_quoted_names.toml` with a description of the fixture content.
- Unit 11.1 Acceptance line 116 specifies the TestLoad assertion: exactly one entry under `manifest.Tools` with key `"github.com/foo/bar"`.
- Unit 11.1 Notes line 120 reiterates the quoting requirement.
- § Notes For Builder Agents line 229 has a `TOML bare-key quoting` bullet covering the same.

### C3 (MEDIUM) — Unit 11.4 test gaps filled

**Pass.**

- Unit 11.4 Acceptance lines 191-198 enumerate seven explicit `TestToolsValidate` cases, including:
  - Case 5 (line 196): zero-byte file using committed `testdata/zero_byte.toml`.
  - Case 6 (line 197): permission-denied using `t.TempDir()` + `chmod 000`.
  - Case 7 (line 198): directory-at-path using `t.TempDir()` + mkdir.
- Unit 11.4 Paths line 199 adds `internal/tools/testdata/zero_byte.toml` to the testdata fixtures.
- § Notes For Builder Agents line 237 explicitly states zero-byte uses committed fixture; permission-denied + directory-at-path use `t.TempDir()` (states don't survive `git add`).

### C4 (LOW) — Tool name regex + invalid case additions

**Pass.**

- § Schema Decisions line 64 declares the regex `^[a-zA-Z0-9]+$|^[a-zA-Z0-9][a-zA-Z0-9._/-]*[a-zA-Z0-9]$` with the explicit rationale "Rejects trailing punctuation (`bad-name-`, `name.`, `name/`)." Notes RE2-compatible (alternation + character classes only; no lookahead).
- Unit 11.2 Acceptance line 140 references the regex; line 145 lists invalid cases including `"bad-name-"`, `"name."`, `"name/"`.
- Unit 11.2 Acceptance line 146 requires the builder to verify the three trailing-punctuation cases are rejected.
- § Notes For Builder Agents line 234 repeats the regex and reiterates RE2 compatibility + trailing-punctuation rejection requirement.

**Regex correctness check (mental trace):** Branch 1 `^[a-zA-Z0-9]+$` matches strings of one or more alphanumeric chars. Branch 2 `^[a-zA-Z0-9][a-zA-Z0-9._/-]*[a-zA-Z0-9]$` requires alphanumeric anchors at both ends. Trace cases:
- `bad-name-` → branch 1 fails (`-`); branch 2 fails (trailing `-` not in `[a-zA-Z0-9]`). Rejected.
- `name.` → branch 2 trailing `.` rejected.
- `name/` → branch 2 trailing `/` rejected.
- `github.com/foo/bar` → branch 2: `g`...`r` with middle `[a-zA-Z0-9._/-]`. Accepted.
- `mage`, `m`, `go` → branch 1 accepts.
- `-bad` → branch 2 leading `-` rejected.
- `bad!char` → both branches fail (`!` not in class).
- empty → both branches fail.

RE2-compatible: no lookaround, only character classes + alternation + anchors. Will compile cleanly via `regexp.MustCompile`.

### C5 (MEDIUM) — Unit 11.4 uses `project.Detect()` from `internal/project/`

**Pass.**

- Unit 11.4 Packages line 183 includes `internal/project/` as read-only dependency.
- Unit 11.4 Acceptance line 187 specifies `project.Detect()` (signature `func Detect() (project.Result, error)`, source `internal/project/project.go:19`), then `result.Root` passed to `tools.Resolve`. Explicitly forbids raw `os.Getwd()`.
- § Notes For Builder Agents line 235 reiterates the binding with line cite.

**Source-of-truth verification:** I read `/Users/evanschultz/Documents/Code/hylla/valv/main/internal/project/project.go` lines 1-40 directly:
- Line 12-16: `type Result struct { Root string; GitMarker string; HasGitMarker bool }`.
- Line 19: `func Detect() (Result, error)` (no parameters; calls `os.Getwd()` internally; delegates to `DetectFrom`).

Plan claim matches source exactly.

### C6 (VERY LOW) — `len(m.Tools) == 0` named explicitly

**Pass.**

- § Schema Decisions line 69 declares the empty-manifest predicate `len(m.Tools) == 0` with explicit rationale: presence of `[allowlist]` or `[env]` sections does NOT make the manifest non-empty.
- Drop-level Acceptance Criterion 6 line 81 names `len(m.Tools) == 0` for the `"no tools declared"` branch.
- Unit 11.4 Acceptance line 188 names `len(m.Tools) == 0` for both empty-file and missing-file cases (and that `[allowlist]`/`[env]` presence does not change the predicate).
- § Notes For Builder Agents line 236 has a dedicated **Empty-manifest predicate (Unit 11.4)** bullet.

## Re-verification of Y2 (`toml.Primitive` claim)

**Pass.**

- § Schema Decisions line 67 declares `Allowlist toml.Primitive` and `Env toml.Primitive` as `ToolManifest` fields. States that during initial `toml.DecodeFile`, these fields receive raw TOML values and are marked decoded — `meta.Undecoded()` returns empty (strict check satisfied). States that a top-level section whose name is NOT `tools`, `allowlist`, or `env` still triggers the undecoded-key error.
- Unit 11.1 Acceptance lines 110-111 declare both fields with toml struct tags.
- Unit 11.1 Acceptance line 115: `Load` accepts files with `[allowlist]` and `[env]` sections without error (they decode into `toml.Primitive` fields — not undecoded).
- Unit 11.1 Paths line 100 adds `invalid_unknown_key.toml` (unknown top-level key, e.g. `[network]` section) as a negative-test fixture.
- Unit 11.1 Acceptance line 114: `Load` returns error for `invalid_unknown_key.toml` (undecoded key triggers strict check).
- § Notes For Builder Agents line 230 reiterates the typed claim and that DROP_11 does NOT call `meta.PrimitiveDecode`.

**Context7 evidence re-check:** queried `/burntsushi/toml` for "toml.Primitive field unmarshaling and meta.Undecoded interaction". Returned the **"Delayed TOML Decoding with Primitive Values in Go"** canonical example showing `Config { Mode string; DevConfig toml.Primitive; ProdConfig toml.Primitive }` with both `[dev_config]` and `[prod_config]` sections decoded via `toml.Decode` — the typed `toml.Primitive` fields capture the raw section payloads, which is the documented usage pattern Round 2 falsification already accepted as evidence. Round 2's empirical test (separate fixture run) corroborates this. Claim survives Round 3 unchanged and remains evidence-grounded.

## Re-verification of F1 (root.go integration)

**Pass.**

- Unit 11.4 Acceptance line 190 specifies: inside `newRootCommandWithPaths`, add `toolsCmd := newToolsCommand()` + `toolsCmd.GroupID = "runtime"`, append `toolsCmd` to the existing variadic `cmd.AddCommand(...)` call at line 137. Cites `cmd` as local var name (line 47).
- **Source-of-truth verification:** I read `/Users/evanschultz/Documents/Code/hylla/valv/main/internal/cli/root.go`:
  - Line 47: `cmd := &cobra.Command{...}` — local variable is `cmd`, not `rootCmd`. Match.
  - Lines 108-112: `cmd.AddGroup(...)` declares groups `"inspect"`, `"runtime"`, `"account"`. Match.
  - Line 127: `codexCmd.GroupID = "runtime"`. Line 129: `claudeCmd.GroupID = "runtime"`. Match — runtime is the correct group for `tools`.
  - Line 137: `cmd.AddCommand(pathsCmd, versionCmd, statusCmd, codexCmd, claudeCmd, accountCmd, globalCmd, imageCmd)` — single variadic call. Match.

Plan claim matches source exactly.

## New Findings (Round 3)

### N1 (LOW) — Round 3 produced clean, no unmitigated new issues

After full re-read of PLAN.md and audit against the five new-issue checks in the spawn appendix:

- **Unit 11.5 acceptance routes failures to dev (not silently fix)?** Confirmed. Lines 219: "If any package reports < 70% coverage, the builder does NOT silently fix it. Instead: list every failing package with its coverage percentage in the worklog, set unit state to `blocked`, and return to the orchestrator. The orchestrator routes to dev: either raise that package's coverage in a follow-on unit, or roll back the bump and open a coverage-only drop." Plan handles this correctly.
- **Coverage gate timing across Units 11.1-11.4?** Consistent. Unit 11.1 Acceptance line 117: "Coverage gate for `internal/tools/` not yet enforced — that is enforced after Unit 11.4 completes the package." Unit 11.2 Acceptance line 147: "Coverage gate not yet enforced (enforced after Unit 11.4)." Unit 11.3 Acceptance line 167: "Coverage gate not yet enforced at this unit — 70% gate is enforced at Unit 11.4 which completes the package with CLI-side integration." Unit 11.4 Acceptance line 200: "≥ 70% coverage across the whole package (this unit completes the package by exercising the full `Resolve` → `Validate` path via CLI tests — 70% gate enforced here)." Note: the global `magefile.go coverageThreshold` constant stays at 60.0 throughout Units 11.1-11.4, then bumps to 70.0 at Unit 11.5. Plan handles the dual-gate distinction (per-package enforcement vs. global magefile constant) cleanly.
- **Regex compiles in Go RE2?** Yes — see C4 trace above; no lookaround, only character classes + alternation + anchors.
- **Stale Round 1/2 text?** Audited: no `Unit 11.0`, no `AllowlistConfig` claim (only explicit "NOT `AllowlistConfig`" callouts), no `valv tools list` claim (only "is cut" callouts), no `os.IsNotExist` fork (only "do NOT use" callouts). All historical anti-patterns are framed as explicit prohibitions.

### N2 (VERY LOW — informational, not blocking) — Fixture content presentation ambiguity in Unit 11.1

**Observation:** Unit 11.1 Paths line 99 describes `valid_quoted_names.toml` content as `"github.com/foo/bar" = { source = "github.com/foo/bar@main", install = "go install" }` without explicitly showing a parent `[tools]` table header. The TestLoad assertion at line 116 asserts the entry lands inside `manifest.Tools`, which requires the fixture entry to be under a `[tools]` table. A careful builder will infer the required structure from the test assertion, but the fixture description could be tightened to remove the inference burden.

**Why not blocking:** The acceptance criterion at line 116 is unambiguous about the assertion ("`manifest.Tools` has exactly one entry with key `"github.com/foo/bar"`"). The builder must produce a fixture that satisfies that assertion — meaning the quoted key must live under a `[tools]` parent. The plan is constructively correct; the prose could just be clearer. Falsification did not flag this in Round 2, and it does not introduce ambiguity at the acceptance level.

**Recommendation (optional, not a fail):** if the planner revises, consider rewriting line 99 to read "fixture with a quoted-key entry under `[tools]`, e.g. `[tools]\n\"github.com/foo/bar\" = { source = ..., install = ... }`". Round 3 verdict is **pass** without this revision.

## Hylla Feedback

No Hylla MCP calls were required for this proof review — verification used direct `Read` of `internal/cli/root.go` and `internal/project/project.go` (both visible, non-search-dependent file inspection of specific known files), and Context7 query for `toml.Primitive` semantics. The plan cites specific files + line numbers that are stable visible artifacts, so Hylla search was not the right tool. No misses to report.
