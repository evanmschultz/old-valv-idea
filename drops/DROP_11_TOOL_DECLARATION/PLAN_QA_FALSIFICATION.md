# DROP_11 Plan QA Falsification — Round 1

**Verdict:** fail
**Reviewer:** go-qa-falsification-agent
**Reviewed at:** 2026-05-21T04:51Z

Two CONFIRMED counterexamples plus three YAGNI items. The TOML-mechanics attacks (the bulk of the prompt's targets) are all REFUTED — Context7 + reading BurntSushi/toml v1.6.0 source (`/Users/evanschultz/go/pkg/mod/github.com/!burnt!sushi/toml@v1.6.0/decode.go`) shows the planner's mixed-type decode strategy is mechanically sound. Falsification lands on two concrete plan-text errors plus YAGNI on lock-in of unconsumed schema.

## Counterexamples / Attacks

### 1. CONFIRMED — Unit 11.4 names a `rootCmd` symbol that does not exist in `internal/cli/root.go`

**Construction.** Unit 11.4 Paths section says:

> `internal/cli/root.go` (existing — add `rootCmd.AddCommand(newToolsCommand())`)

Read `/Users/evanschultz/Documents/Code/hylla/valv/main/internal/cli/root.go`. There is no package-level or function-scoped variable named `rootCmd`. The root command is a function-local variable named `cmd` inside `newRootCommandWithPaths` (declared at line 47, populated at line 137 by `cmd.AddCommand(pathsCmd, versionCmd, statusCmd, codexCmd, claudeCmd, accountCmd, globalCmd, imageCmd)`). A literal interpretation of the plan's instruction would not compile.

**Impact.** Minor — the builder will read the file and adapt the wiring (extend the existing `cmd.AddCommand(...)` list, mirroring `codexCmd`/`claudeCmd`). But the plan-as-written is wrong about the symbol shape, and a build-QA pass that audits "did the builder implement the plan literally" would flag it. The planner also does not specify a `GroupID` for the tools command; the three existing groups (`inspect`, `runtime`, `account`) do not naturally fit `tools`, so the command would land in cobra's default "Additional Commands" section unless the planner picks one or adds a new group. Both gaps should be resolved in the plan, not left to the builder to invent.

**Fix.** Rewrite Unit 11.4 Paths to:

> `internal/cli/root.go` (existing — extend the `cmd.AddCommand(...)` call at the bottom of `newRootCommandWithPaths` to include the new `toolsCmd`; assign `toolsCmd.GroupID = "inspect"` to slot it alongside `valv status` / `valv paths`)

Or pick a different group / add a new one — but pick one in the plan.

### 2. CONFIRMED — Unit 11.1 acceptance contradicts Unit 11.2 boundary on `invalid_object_missing_install.toml`

**Construction.** Unit 11.1 Paths lists `testdata/invalid_object_missing_install.toml` as a new fixture. Unit 11.1 acceptance bullet 6 says:

> `Load` returns error for `testdata/invalid_unknown_key.toml` (undecoded key present) and `testdata/invalid_object_missing_install.toml` when validation is wired (see Unit 11.2).

The qualifier "when validation is wired" makes the acceptance criterion ambiguous about Unit 11.1's own behavior. There are two readings:

- **Reading A** (favored by the prose): Unit 11.1's `Load` does NOT validate object-form completeness. It parses `{ source = "..." }` into `ToolSpec{Source: "...", Install: ""}` cleanly. The `invalid_object_missing_install.toml` fixture is shipped in 11.1 but the rejection only fires once Unit 11.2's `Validate` is wired into `Resolve` (Unit 11.3).
- **Reading B** (favored by the bullet's grammar): Unit 11.1 should test that `Load` returns error for that fixture — which it can't, because validation lives in 11.2.

Build-QA in Phase 5 will need to pick one. If the builder picks Reading B, Unit 11.1 cannot reach `done` without forward-referencing 11.2's `Validate` function. If the builder picks Reading A, the fixture is shipped in 11.1 but Unit 11.1's tests don't exercise it (it becomes test-only fodder for Unit 11.2's tests). Then 11.1's coverage gate is fine but the file is dead in 11.1 — a build-QA finding.

**Fix.** Split the fixture: ship `invalid_object_missing_install.toml` in Unit 11.2's Paths list, not 11.1's. Rewrite Unit 11.1 acceptance bullet 6 to drop the "when validation is wired" clause and only assert the `invalid_unknown_key.toml` rejection. Move the `invalid_object_missing_install.toml` rejection assertion entirely to Unit 11.2 acceptance.

### 3. REFUTED — `BurntSushi/toml` cannot decode `map[string]ToolSpec` heterogeneous string-or-table

**Attempted attack.** The planner's Notes For Builder Agents says "BurntSushi/toml cannot natively decode a `map[string]ToolSpec` where some values are strings and others are inline tables. The builder for Unit 11.1 must implement `(t *ToolSpec) UnmarshalTOML(...) error` ... or use an intermediate `map[string]toml.Primitive` + deferred decode." The attack: is `UnmarshalTOML` even invoked on per-value map elements? Or does the decoder require `Unmarshaler` on the top-level type only (as Context7's `Collection` example shows)?

**Evidence.** Read `/Users/evanschultz/go/pkg/mod/github.com/!burnt!sushi/toml@v1.6.0/decode.go`:

- `unifyMap` (line 343-384) iterates map entries. For each entry, line 364 allocates a fresh value of `rv.Type().Elem()` (i.e. `ToolSpec`) via `reflect.New` (addressable). Line 366 calls `md.unify(v, indirect(rvval))`.
- `indirect` (line 608-621): if the value is non-pointer and `CanSet()`, it checks whether the **pointer** type satisfies `Unmarshaler` and returns the pointer if so. `reflect.New`'s output is addressable, so this branch fires.
- `unify` (line 224, dispatch at line 239-256): with the pointer in hand, `rvi.(Unmarshaler)` succeeds; line 241 calls `v.UnmarshalTOML(data)`. Line 247-254 marks subkeys decoded only when `data` is a `map[string]any` — for a primitive string value, no recursive marking happens, but the map-entry key itself was already marked at `unifyMap` line 361.

**Conclusion.** Pointer-receiver `(t *ToolSpec) UnmarshalTOML(any) error` on a `map[string]ToolSpec` field IS invoked per element. The planner's design is mechanically sound. REFUTED — no counterexample.

### 4. REFUTED — Strict `meta.Undecoded()` will reject `[allowlist]` / `[env]` even with stub structs

**Attempted attack.** The strict-check pattern in `internal/config/Load` line 68-70 rejects any undecoded key. Does declaring `Allowlist AllowlistConfig` with `toml:"allowlist"` (and inner `Hosts []string toml:"hosts"`) actually mark `allowlist.hosts` as decoded? Or does the strict check still flag `allowlist.hosts` since the planner shows no fields beyond `Hosts`?

**Evidence.** Same source file, `unifyStruct` lines 298-341. Line 327 (`md.decoded[md.context.add(key).String()] = struct{}{}`) marks each subkey as it descends into the field. So for a TOML file with `[allowlist] hosts = ["github.com"]`, the decoder marks both `allowlist` and `allowlist.hosts` as decoded. Stub design works.

**Conclusion.** REFUTED for the simple case. Note: if a user writes `[allowlist] hosts = [...] policy = "strict"`, the `policy` subkey IS undecoded and the strict check rejects the file. That is the desired behavior (typo protection + forward-version discipline) — when DROP_15 lands `policy`, the struct must be extended. Not a counterexample, but worth one sentence in the planner's design notes so the test fixture for `valid_objects.toml` doesn't accidentally include keys the struct doesn't declare.

### 5. REFUTED — Object-form ambiguity: `mage = { version = "latest" }` (planner's intermediate case)

**Attempted attack.** The planner's `ToolSpec` defines `Version`, `Source`, `Install`. String-form `mage = "latest"` → `{Version: "latest"}`. Object-form `mage = { source = "...", install = "..." }` → `{Source, Install}`. What about `mage = { version = "latest" }` (object with `version` key)? `UnmarshalTOML` will receive a `map[string]any` with key `version`. The planner's design has `Version string` as an exported field with implicit tag `toml:"version"`, so this would naturally decode — BUT Unit 11.2's validation says "Object-form tool: both `Source` and `Install` non-empty; `Version` must be empty". So this case would deserialize to `{Version: "latest", Source: "", Install: ""}` and then FAIL validation with a clear error.

**Conclusion.** REFUTED — the planner's design handles the intermediate case correctly. The validation rule is sharp enough to reject it. Builder should add a `testdata/invalid_object_version_only.toml` fixture to Unit 11.2 to lock the behavior in.

### 6. REFUTED — TOML reserved characters in tool names (e.g. `[tools."github.com/foo/bar"]`)

**Attempted attack.** Unit 11.2's "no whitespace" rule for tool names. What about TOML keys with dots like `[tools."github.com/foo/bar"]`? Or with special characters?

**Evidence.** TOML quoted bare keys allow any string. BurntSushi/toml will decode `[tools."github.com/foo/bar"]` as a map entry with key `"github.com/foo/bar"`. The planner's validation rule (non-empty, no whitespace) accepts it. Whether that's the intended UX is a separate question — the user might mean "the tool named `github.com/foo/bar`" (i.e. a URL-like identifier for a custom-install tool) — which is consistent with the planner's custom-install design (`ta = { source = "github.com/evanmschultz/ta@main", install = "go install" }` where the name is `ta`, not the source path).

**Conclusion.** REFUTED — the name field is unconstrained except for whitespace, by design. No counterexample.

### 7. REFUTED — Unit 11.3's `domain.ErrToolsNotFound` forward-reference into Unit 11.1

**Attempted attack.** Does Unit 11.1 reference `domain.ErrToolsNotFound`? If yes, 11.1 won't compile because 11.3 adds that sentinel.

**Evidence.** Unit 11.1 acceptance bullets only mention `os.Stat` guard + `toml.DecodeFile` + `meta.Undecoded()` + `fmt.Errorf` wrapping. No reference to `ErrToolsNotFound`. Unit 11.3 acceptance bullet 2 says `Resolve` uses `errors.Is(err, domain.ErrToolsNotFound)` or `os.IsNotExist` — it's `Resolve`'s concern, and `Resolve` is in `resolve.go` (Unit 11.3's file). The error wrapping in `Load` (Unit 11.1) can return a wrapped `os.ErrNotExist`, which `Resolve` then detects with `errors.Is(err, os.ErrNotExist)` — sentinel-free path also works.

**Conclusion.** REFUTED. Unit ordering is compile-safe.

### 8. REFUTED — Unit 11.4 `tools.Resolve(cwd)` signature drift

**Attempted attack.** Does Unit 11.4's call to `tools.Resolve(cwd)` match Unit 11.3's `Resolve(projectDir string) (ToolManifest, error)` signature?

**Evidence.** Unit 11.3 acceptance: `Resolve(projectDir string) (ToolManifest, error)`. Unit 11.4: `tools.Resolve(cwd)` where `cwd` is `os.Getwd()`. `os.Getwd()` returns `(string, error)` — builder handles the error, passes the string. Signature matches.

**Conclusion.** REFUTED.

### 9. REFUTED — Coverage-gate timing across 11.1+11.2+11.3

**Attempted attack.** "Coverage gate enforced after Unit 11.3" — what if 11.1+11.2 land with paths 11.3 doesn't exercise? Then 11.3's QA fails not because of 11.3's own work but because of earlier units' coverage gap.

**Evidence.** Unit 11.1 ships `tools.go` + `tools_test.go` with table-driven tests for `Load`. Unit 11.2 ships `validate.go` + `validate_test.go` with table-driven tests for `Validate`. Each unit's tests cover the unit's own file. Unit 11.3's coverage requirement is the **package-level** coverage at 70% — if 11.1+11.2 each ship adequately tested code, the package coverage at 11.3 will be ≥70% as long as 11.3's own tests cover `Resolve`. The planner's gate timing is sound IF the builder for 11.1 and 11.2 honors table-driven test coverage discipline (which is the project standard per AGENTS.md § 11).

**Conclusion.** REFUTED — gate timing is realistic. Add a per-unit note to 11.1 + 11.2 that test files must hit each branch (object-form, string-form, validation rule) to keep the package coverage at ≥70% by the time 11.3 closes.

### 10. REFUTED — File-absent edge cases (`.valv/` exists but `tools.toml` missing; zero-byte file)

**Attempted attack.** Unit 11.3 acceptance says "directory with no `.valv/` dir" returns empty manifest. What about:
- `.valv/` exists but `tools.toml` does not?
- `.valv/tools.toml` exists but is zero-byte?

**Evidence.** Both are file-existence questions handled by `os.Stat` semantics:
- `.valv/` exists, no `tools.toml` → `os.Stat(.valv/tools.toml)` returns `os.ErrNotExist` → `Load` returns wrapped not-found → `Resolve` returns empty manifest.
- Zero-byte `tools.toml` → `os.Stat` succeeds → `toml.DecodeFile` on empty file → empty `ToolManifest{}` cleanly. `meta.Undecoded()` returns empty slice. `Load` returns `(ToolManifest{}, nil)`. `Validate` of empty manifest is valid (tool count 0 ≤ 50). Returns empty manifest.

**Conclusion.** REFUTED. Both edges fall out of the planner's design correctly. Builder should add `testdata/empty.toml` (zero-byte) as a `Load` test case to lock it in — minor planner improvement, not a counterexample.

### 11. REFUTED — Interface-change spillover (`feedback_interface_change_runs_full_mage_test.md`)

**Attempted attack.** Does any unit add a method to an existing interface? Spawns to fakeStore in sibling test packages?

**Evidence.** All four units are additive: new package `internal/tools/`, new commands in `internal/cli/tools.go`, new sentinel in `internal/domain/errors.go`. No interface modifications. No method additions to existing types. Sibling-package mocks unaffected.

**Conclusion.** REFUTED.

### 12. REFUTED — Integration-tag spillover (`feedback_mage_integration_when_deleting_symbols.md`)

**Attempted attack.** Does any unit delete a symbol that `mage testPkg` alone won't catch (integration-build-tag breakage)?

**Evidence.** All four units are additive. No deletions. No symbol renames.

**Conclusion.** REFUTED.

## YAGNI Pressure

### Y1. Custom-install support `{source, install}` in DROP_11

**Argument.** DROP_11 is "schema + parser + validation + per-project resolution + introspection CLI only — no image build, no install action." DROP_12 is the consumer that actually performs install. DROP_11's validation locks two object-form rules: `Source` and `Install` both required; `Version` must be empty. This is **schema lock-in before the consumer exists**. When DROP_12 lands, the actual install constraints emerge: does `install` need `{cmd, version_pin, env}`? Does `source` need branch/commit semantics? Does the install action need to know the target binary name? Any answer that requires more fields forces DROP_11 to ship a v2 schema and migrate existing fixtures — defeating the "forward compatible" justification.

**Recommendation.** Cut object-form from DROP_11. Ship string-form only (`mage = "latest"`, `go = "1.22"`). The schema for "well-known tool with version pin" is universal and not under DROP_12's design pressure. Ship custom-install as part of DROP_12 where its constraints are concrete.

**Counter-defense the planner could give.** "Forward compat means DROP_12 doesn't need a schema migration." Rebuttal: DROP_12 hasn't been planned. Its actual schema needs are speculation. Better to ship the minimum DROP_11 actually needs (string-form only) and add object-form when its shape is empirically driven by DROP_12.

### Y2. `[allowlist]` and `[env]` forward stubs in DROP_11

**Argument.** Same shape as Y1, weaker case. DROP_14 (env vars) and DROP_15 (network policy) are the consumers. The planner correctly notes that without stubs, the strict `Undecoded()` check would reject any user file that adopts the forward keys. But: (a) no user has a `.valv/tools.toml` yet — DROP_11 ships the schema. The forward-compat argument is hypothetical; (b) the planner could scope the strict check to the `[tools]` section only (use `toml.Primitive` for the unknown remainder), preserving forward flexibility without locking the substruct shape.

**Recommendation.** Either cut `[allowlist]` + `[env]` stubs from DROP_11 (their consumers will add them with the right shape), or use `toml.Primitive` for those sections so the substruct shape isn't locked. Locking `AllowlistConfig{Hosts []string}` means DROP_15 must extend that struct AND migrate every test fixture that uses the stub.

**Counter-defense the planner could give.** "Consistency with `internal/config/Load`'s strict pattern." Rebuttal: `internal/config/Load` is strict because its schema is fully owned. `.valv/tools.toml`'s schema spans four drops (11/14/15 + DROP_15's allowlist editing UX). The strict pattern fights the multi-drop trajectory.

### Y3. `valv tools list` subcommand

**Argument.** Per `feedback_manual_workflow_is_the_decision.md`, the dev's pattern is "the manual workflow IS the decision." `cat .valv/tools.toml` shows declared tools. `valv tools list` adds a thin parsing-and-formatting layer with no informational gain (the only enrichment is "no tools declared" notice when the file is absent — which `cat` also handles with "file not found"). `valv tools validate` is genuinely useful (exit-code-driven CI / pre-commit) — keep it. `list` is YAGNI bait.

**Recommendation.** Cut `valv tools list` from DROP_11. Ship `valv tools validate` only. Re-add `list` if/when a user actually requests structured output (CSV / JSON) for tooling integration — and at that point the JSON shape will be the design driver.

**Counter-defense the planner could give.** "Consistency with `valv account list` / `valv status` UX pattern." Rebuttal: those commands surface state Valv owns (DB rows, runtime status). `.valv/tools.toml` is a user-owned file the user already has open in their editor. The asymmetry is real.

## Hylla Feedback

None — Hylla was not queried for this review. All evidence came from `Read` (BurntSushi/toml source in `$GOMODCACHE`, drop's `PLAN.md`, `internal/cli/root.go`, `internal/config/config.go`, `internal/domain/errors.go`, `WORKFLOW.md`, `PLAN.md`) plus Context7 (`/burntsushi/toml`) plus `go doc` (`Unmarshaler`, `MetaData`, `MetaData.Undecoded`). Hylla is Go-only and this review was schema/plan-level rather than call-site blast-radius — non-Go markdown was the bulk. Falls under the "N/A — action item touched non-Go files only" carve-out for the plan documents themselves; the Go-source touchpoints (root.go, config.go, errors.go) were small enough that `Read` was more direct than a Hylla query.
