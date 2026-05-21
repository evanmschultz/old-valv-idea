# DROP_11 Plan QA Proof — Round 4

**Verdict:** pass
**Reviewer:** ta-go-qa-proof
**Reviewed at:** 2026-05-21T06:44:44Z

## Round 3 Delta Verification

### NEW C2 [SHOWSTOPPER] — `toml.Primitive` requires explicit `PrimitiveDecode`

**Status:** PASS

Schema Decisions block has been fully reversed and now embeds the empirical evidence inline:

- Plan lines 51-54 (Schema Decisions code-block annotation): explicitly states "DROP_11 captures them as toml.Primitive fields and calls meta.PrimitiveDecode on each to mark them as decoded (the decoded value is discarded — DROP_11 takes no action on the contents)."
- Plan lines 71-78 (Schema Decisions narrative): states "`ToolManifest` declares `Allowlist toml.Primitive` and `Env toml.Primitive`. After `toml.DecodeFile`, the parser **explicitly calls `meta.PrimitiveDecode` on each `toml.Primitive` field**...". Empirical output is embedded directly:
  ```
  undecoded BEFORE PrimitiveDecode: [allowlist.hosts env.GOPRIVATE]
  undecoded AFTER  PrimitiveDecode: []
  ```
- Plan line 78 explicitly reverses the Round 2/3 claim: "The Round 2/3 Context7-inferred claim that `toml.Primitive` declaration alone marked the section as decoded was wrong; this Round 4 revision reverses it."
- Unit 11.1 acceptance (plan line 123): "uses `os.Stat` guard..., `toml.DecodeFile`, then — **before the strict `meta.Undecoded()` check** — calls `meta.PrimitiveDecode(m.Allowlist, &discardA)` and `meta.PrimitiveDecode(m.Env, &discardE)`...". The "before the strict `meta.Undecoded()` check" ordering is explicit.
- Unit 11.1 acceptance (plan line 124): MANDATORY callout with the same empirical sentence. "Builder MUST implement the two calls before the `Undecoded()` check; skipping them will reject valid forward-compat sections as unknown keys."
- Unit 11.1 acceptance (plan line 127): NEW TestLoad case — "a TOML file containing `[allowlist]` with `hosts = [...]` AND `[env]` with `GOPRIVATE = "..."` decodes cleanly — `meta.Undecoded()` returns the empty slice after the two `PrimitiveDecode` calls."
- Unit 11.1 acceptance (plan line 129): a dedicated TestLoad case for forward-compat sections, with the smoke-check "Builder verifies this case explicitly fails without the two `PrimitiveDecode` calls (smoke check during dev)."
- Notes For Builder Agents (plan line 247): full mandatory paragraph with the same empirical output and explicit Round 2/3 reversal.

All five required surfaces (Schema Decisions, Unit 11.1 acceptance, new TestLoad case, Notes For Builder Agents, reversal of prior claim) carry the empirical evidence inline. Round 3 SHOWSTOPPER fully resolved.

### NEW C1 [MINOR] — Unit 11.5 escalation distinction

**Status:** PASS

Plan lines 234-236 distinguish the two failure classes:

- "**New-package failure (`internal/tools/`):** the builder for THIS drop owns it. Route back to Unit 11.4 (which is the unit that finishes the package by exercising the CLI integration). The Unit 11.4 builder is responsible for raising `internal/tools/` coverage to ≥ 70% with additional tests before Unit 11.5 re-runs. Tightly scoped fix, same drop."
- "**Legacy-package failure (e.g. `internal/adapters/docker` — the known candidate from the pre-existing TODO in `magefile.go`):** out of scope for this drop's code units. Route to dev for triage. Dev decides between (a) raising that package's coverage as a new follow-on unit inside DROP_11 (e.g. Unit 11.6), or (b) rolling back the threshold bump and opening a separate coverage-cleanup drop."

Both branches are concrete and actionable. Round 3 finding resolved.

### NEW C3 [MINOR] — directory-at-path test wording

**Status:** PASS

Plan line 213 (Unit 11.4 case 7): "The acceptance assertion is `errors.Is(err, syscall.EISDIR)` OR the error string contains `"is a directory"`."

Empirical evidence is embedded inline ("Empirical evidence (planner Round 4 scratch run on macOS Darwin against the Go stdlib): `os.Open(dir)` returns `nil` error; `io.ReadAll(f)` returns `"read <path>: is a directory"` with `errors.Is(err, syscall.EISDIR) == true`."). No "not a regular file" wording survives. Round 3 finding resolved.

### NEW C4 [MINOR] — regex edge cases pinned

**Status:** PASS

Plan line 67 (Schema Decisions, accept-by-design): "multiple consecutive `.`, `/`, or `-` characters within the name (e.g. `a..b`, `a//b`, `a---b`). Path-like names (`github.com/foo/bar`) and namespaced names (`org.tool.subname`) are valid."

Plan line 68 (Schema Decisions, reject-by-design): "empty string; trailing punctuation (`bad-name-`, `name.`, `name/`); leading dash; leading `_` (must start alphanumeric — chosen for clarity, `_` would suggest internal/private semantics inappropriate for a public tool list); whitespace anywhere; `!` and other non-permitted characters; non-ASCII names".

Unit 11.2 test cases updated (plan lines 159-160):
- Valid (new): `"a..b"`, `"a---b"`, `"a/b/c"`.
- Invalid (new): `"_underscore"` "(rejected by design — must start alphanumeric)".

Plan line 161 reinforces: "The doubled-separator accept cases (`"a..b"`, `"a---b"`) and the leading-underscore reject case are the planner-pinned edge cases — they explicitly test that the design choice in the Schema Decisions block is honored by the implementation."

Round 3 finding resolved.

### F1 [LOAD-BEARING] — root.go integration

**Status:** PASS (survives Round 4 unchanged)

Plan line 205 (Unit 11.4 acceptance): full F1 spec is intact. "inside `newRootCommandWithPaths`, add `toolsCmd := newToolsCommand()` and `toolsCmd.GroupID = "runtime"`. Include `toolsCmd` in the existing `cmd.AddCommand(pathsCmd, versionCmd, statusCmd, codexCmd, claudeCmd, accountCmd, globalCmd, imageCmd)` call — add `toolsCmd` to that variadic list. Evidence: `root.go` line 137 is the single `cmd.AddCommand(...)` call; the local variable is `cmd` (line 47), not `rootCmd`. Groups confirmed: `"inspect"`, `"runtime"`, `"account"` declared at lines 108-112; `codexCmd` and `claudeCmd` use `GroupID = "runtime"`."

Spot-verified against current `internal/cli/root.go`:
- Line 137 is the single `cmd.AddCommand(pathsCmd, versionCmd, statusCmd, codexCmd, claudeCmd, accountCmd, globalCmd, imageCmd)` call — exact match.
- Lines 108-112 declare the three groups (`inspect`, `runtime`, `account`) — exact match.
- Variable name is `cmd`, not `rootCmd` — confirmed.
- `codexCmd.GroupID = "runtime"` (line 127), `claudeCmd.GroupID = "runtime"` (line 129) — confirmed.

F1 anchor is still correct against HEAD. No drift.

## New Findings (Round 4)

### N1 [MINOR] — `PrimitiveDecode` safety on zero-value `toml.Primitive` not explicitly pinned

**Severity:** minor (not a blocker)

Unit 11.1's acceptance (plan line 123) requires unconditional calls to `meta.PrimitiveDecode(m.Allowlist, &discardA)` and `meta.PrimitiveDecode(m.Env, &discardE)` regardless of whether the input TOML actually contains `[allowlist]` or `[env]` sections. When neither section is present, `m.Allowlist` and `m.Env` are zero-value `toml.Primitive` values.

The plan's embedded empirical evidence (lines 73-76) only covers the WITH-sections case (`undecoded BEFORE: [allowlist.hosts env.GOPRIVATE]` → `undecoded AFTER: []`). The WITHOUT-sections path through `PrimitiveDecode` (where the field is zero-valued) is NOT empirically pinned, and Unit 11.1 acceptance does not explicitly state "PrimitiveDecode is a safe no-op on zero-value Primitive."

In practice this is expected to be a safe no-op — a zero-value `toml.Primitive` has empty `undecoded` and nil context, so `PrimitiveDecode` iterates over nothing and returns nil — but the plan does not pin this.

**Why this is not a blocker:** Unit 11.1 paths list `testdata/valid_simple.toml` (a "simple string-value tools fixture" — implicitly without `[allowlist]` / `[env]`). If `PrimitiveDecode` on a zero-value `toml.Primitive` were to error, the unconditional calls in `Load` would fail every test that uses `valid_simple.toml`, which would surface immediately during Unit 11.1 build. The bug-trap is real even though the pinning is implicit.

**Recommended (optional) revision:** add to Unit 11.1 acceptance a one-line note: "`PrimitiveDecode` calls are unconditional and safe — a zero-value `toml.Primitive` (absent section) is a no-op. The TestLoad case using `valid_simple.toml` (no `[allowlist]` / `[env]` sections) confirms this path."

If the planner does not want to revise further, this finding can be accepted as-is — the empirical trap via `valid_simple.toml` will catch any divergence at Unit 11.1 build time.

## Hylla Feedback

No Hylla queries were required for this Round 4 proof — all verification was against the in-tree `PLAN.md` and a single `Read` of `internal/cli/root.go` to spot-check the F1 anchor. Context7 was queried once for `BurntSushi/toml` `PrimitiveDecode` semantics on zero-value `Primitive` (relevant to N1) — Context7 returned the canonical `PrimitiveDecode` example which only covers the section-present case, leaving the zero-value-Primitive behavior implicit.
