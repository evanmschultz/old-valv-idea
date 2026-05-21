# DROP_11 Plan QA Falsification — Round 2

**Verdict:** fail
**Reviewer:** go-qa-falsification-agent
**Reviewed at:** 2026-05-20T00:00:00Z

Round 2 introduced 3 new structural defects (Unit 11.0 ordering paradox, TOML bare-key vs validation-regex mismatch on the canonical `github.com/foo/bar` example, exit-code-vs-flow ambiguity in Unit 11.4) plus 1 minor regex anchor question. Round 1's CONFIRMED issues are properly closed; the new failures are independent.

## Round 1 Attack Re-verification

### R1-C1 (rootCmd doesn't exist) — RESOLVED

The plan's Unit 11.4 acceptance now states (line 195): "`root.go` line 137 is the single `cmd.AddCommand(...)` call; the local variable is `cmd` (line 47), not `rootCmd`."

Verified against live `/Users/evanschultz/Documents/Code/hylla/valv/main/internal/cli/root.go`:
- Line 47: `cmd := &cobra.Command{` — confirmed local variable name is `cmd`, not `rootCmd`.
- Line 137: `cmd.AddCommand(pathsCmd, versionCmd, statusCmd, codexCmd, claudeCmd, accountCmd, globalCmd, imageCmd)` — confirmed single AddCommand call.
- Lines 108-112: groups `"inspect"`, `"runtime"`, `"account"` declared.
- Lines 127, 129: `codexCmd.GroupID = "runtime"` and `claudeCmd.GroupID = "runtime"`.

The chosen `GroupID = "runtime"` for `toolsCmd` is consistent with the parallel-with-codex/claude framing. **Pass.**

### R1-C2 (cross-unit fixture ambiguity) — RESOLVED

Plan Unit 11.2 line 140 now owns `internal/tools/testdata/invalid_object_missing_install.toml`. Unit 11.1's fixture list (lines 109-111) covers only `valid_simple.toml`, `valid_objects.toml`, `invalid_unknown_key.toml`. No collision. **Pass.**

### R1-Y1 (object-form custom-install) KEEP — re-attack on DROP_12 forward-compat — PASS WITH NOTE

The plan locks: object form is `{ source, install }`, both required (lines 58, 148-149). DROP_12 will need to add at minimum an `Args` field, a `Path` for local sources, a `Version` interpretation rule. Two questions:

- **Will DROP_12 want to add fields to `ToolSpec`?** Yes — almost certainly. `ToolSpec{Version, Source, Install}` shipped today, plus arch hint, plus install args, plus checksum. Each new field is additive — no schema-breakage risk because Go struct field additions are non-breaking.
- **Will DROP_12 want to change the "both required" rule?** Possible — e.g. `{ source = "path:./local-bin" }` may not need an `install` since the binary is pre-built. That's a relaxation, not a contradiction. Validate rules can loosen forward-compat-cleanly.

Verdict: **no new counterexample**. Schema is forward-compat-extendable. **Pass.**

### R1-Y2 (named `toml.Primitive` fields) APPLY — Undecoded behavior verified — PASS (with one edge case captured)

Plan uses `Allowlist toml.Primitive` + `Env toml.Primitive` (line 62, lines 121-122).

Verified via Context7 `/burntsushi/toml`:
- The "Delayed TOML Decoding" example uses `DevConfig toml.Primitive` + `ProdConfig toml.Primitive` precisely this way.
- The "Strict TOML Decoding with Undecoded Key Detection" example confirms strict-check semantics: any top-level section NOT mapped to a struct field shows up in `metadata.Undecoded()`.

So: a `[allowlist]` section decodes into `ToolManifest.Allowlist` (the Primitive field) and does NOT show up as undecoded. A `[network]` section (no matching field) DOES show up — strict check fires, `Load` returns the wrapped error. This is the planner's intent. **Pass for the documented surface.**

**Edge case worth a Unit 11.1 test:** what does `meta.Undecoded()` return when the file contains `[allowlist]` with no sub-keys (an empty section)? The section itself is mapped to `Allowlist toml.Primitive`, so it is decoded; no sub-keys means nothing further to decode. Expected behavior: `Undecoded()` returns empty. Plan doesn't explicitly enumerate this case in Unit 11.1's table (line 124 covers `valid_objects.toml` with allowlist present + populated, line 125 covers unknown-key rejection). **Recommendation: add a `valid_empty_sections.toml` fixture or a sub-case to Unit 11.1's acceptance that exercises an empty `[allowlist]` block. Not a fail — a precision gap.**

### R1-Y3 (`valv tools list` cut) APPLY — reachability gap check — PASS

The drop's observability surface is `valv tools validate`. Acceptance criterion 7 (lines 75-76) states success prints `"tools.toml is valid"`, empty prints `"no tools declared"`, failure exits 1 with error message.

Does the user have any way to inspect the parsed structure? No — `validate` only confirms validity. But:
- This drop is **schema + parser only**. The consumer is DROP_12. DROP_12 will read `Resolve(projectDir)` as a Go-side API call — no CLI introspection needed.
- A future drop CAN add `valv tools list` cheaply on top of the same `Resolve` API.

No reachability gap for DROP_11's scope. **Pass.**

## New Counterexamples (Round 2)

### C1 — Unit 11.0 ordering paradox (CONFIRMED, high severity)

**Construction:** Plan claims (line 98): "Unit 11.0 — foundational unit; must complete before any coding units begin." Plan claims (line 127): "Coverage gate for `internal/tools/` not yet enforced — that is enforced after Unit 11.3 completes the package."

But `magefile.go` line 196 invokes `runRepoTests` which calls `renderCoverage(printer, report, coverageThreshold)` against `./...` — meaning **every package** is checked against the threshold by `mage test`. After Unit 11.0 raises the threshold to 70.0, the next `mage test` invocation hits every package, including any new package created later.

Trace:
1. Unit 11.0 lands: `coverageThreshold = 70.0`. `mage test` is run. Per the acceptance, it must pass clean across ALL packages.
2. Unit 11.1 lands (BLOCKED by 11.0). Creates `internal/tools/` package with `Load` + test for `Load`. Acceptance line 127 says: "`mage testPkg ./internal/tools/` passes. … Coverage gate for `internal/tools/` not yet enforced — that is enforced after Unit 11.3."
3. **But `mage testPkg` ALSO uses `coverageThreshold`** — magefile line 142: `return renderCoverage(printer, report, coverageThreshold)`. After 11.0, that constant IS 70.0.
4. If Unit 11.1's `Load` + tests don't hit 70% coverage on the `internal/tools/` package (likely, since `Load` is only one of three functions — `Validate` and `Resolve` come in 11.2 and 11.3), then `mage testPkg ./internal/tools/` will **fail at the coverage gate** during 11.1's own build-QA.
5. Unit 11.1's acceptance line 127 explicitly says "Coverage gate for `internal/tools/` not yet enforced — that is enforced after Unit 11.3 completes the package." But the magefile cannot honor this carve-out — the threshold constant is global. There is no mechanism in the magefile to defer the gate per-package per-unit.

**File:line evidence:**
- `magefile.go:142` — `renderCoverage(printer, report, coverageThreshold)` inside `TestPkg`
- `magefile.go:200` — `renderCoverage(printer, report, coverageThreshold)` inside `runRepoTests`
- Plan `PLAN.md:127` — claim that the 70% gate is "not yet enforced" until 11.3

**Fix options:**
1. **Re-order Unit 11.0 to after Unit 11.3** — Unit 11.3 acceptance already requires ≥ 70% coverage on `internal/tools/`, so by then the package is at 70%+. Run 11.0 last, just before drop-end verify. This is the cleanest fix.
2. **Build 11.1+11.2+11.3 in a single unit** that lands a fully-tested `internal/tools/` package at ≥ 70% coverage from its first commit. Then run 11.0 at any time after.
3. **Add a `//+coverage:ignore` mechanism to the magefile** so newly-added packages can be exempt for a window. Heavy machinery — YAGNI.

Option 1 is the minimal-change fix. Currently Unit 11.0 is `blocked_by: nothing` and 11.1 is `blocked_by: Unit 11.0`. Swap: 11.0 should be `blocked_by: Unit 11.3`, 11.1 should be `blocked_by: nothing`.

**Severity:** This will cause Unit 11.1's build-QA to fail OR cause the orchestrator to delete tests that haven't been written yet. **CONFIRMED counterexample.**

### C2 — TOML bare-key syntax vs validation regex contradiction (CONFIRMED, medium severity)

**Construction:** Plan line 59 states the tool-name validation regex `^[a-zA-Z0-9][a-zA-Z0-9._/-]*$` permits `github.com/foo/bar`-style names. Plan line 152 confirms this with a test case: `"valid github-style name `\"github.com/foo/bar\"`"`.

But TOML's own key-syntax rules (Context7 `/toml-lang/toml`):
- **Bare keys** allow only `[A-Za-z0-9_-]` — letters, digits, underscores, dashes.
- **Dotted keys** (`a.b.c`) create nested tables. `github.com` written bare in TOML becomes nested table `github` → `com`.
- **Quoted keys** (`"github.com/foo/bar"`) allow arbitrary string content as a single key.
- **Slashes** in bare keys are not allowed.

So the user MUST write `[tools]` then `"github.com/foo/bar" = "..."` (with quotes) to get the string `github.com/foo/bar` as a single map key. If they write `github.com/foo/bar = "..."` without quotes, the TOML parser rejects it (slash not allowed in bare key).

The validation regex itself is fine — it runs against the post-decode `map[string]ToolSpec` key, which is the de-quoted Go string. But the plan's documentation surface needs to:
1. **Tell the user** that `github.com/foo/bar`-style names require quoting in the TOML source.
2. **Add a fixture test** that the regex accepts the post-decode string AND that BurntSushi/toml correctly decodes a quoted dotted+slashed key into a single map entry.

Worse: the canonical example in plan line 44 — `ta = { source = "github.com/evanmschultz/ta@main", install = "go install" }` — uses `ta` as the bare key. That's fine. But the plan never shows the syntax for a tool whose **name itself** is `github.com/foo/bar`. If a user reads plan line 59 ("permits `github.com/foo/bar`-style names") and tries to write:

```toml
[tools]
github.com/foo/bar = "v1.0.0"
```

This TOML is **invalid** — the BurntSushi parser will return a decode error. The user must write:

```toml
[tools]
"github.com/foo/bar" = "v1.0.0"
```

**File:line evidence:**
- Plan `PLAN.md:59` — "Permits `github.com/foo/bar`-style names" (claim)
- Plan `PLAN.md:152` — test case `"valid github-style name `\"github.com/foo/bar\"`"` (claim)
- Context7 `/toml-lang/toml` "TOML Bare Key Examples" — bare keys are ASCII letters/digits/underscores/dashes only
- Context7 `/toml-lang/toml` "TOML Quoted Key Examples" — quoted keys required for `"127.0.0.1"`-style

**Fix:** Plan should add a sub-bullet under the schema decisions explicitly stating: "Tool names containing `.`, `/`, or other non-bare-key characters must be quoted in the TOML source: `\"github.com/foo/bar\" = \"...\"`." Plan should also add a `valid_quoted_names.toml` fixture in Unit 11.1 to prove the round-trip works.

**Severity:** Will cause real-user confusion AND will silently miss the test surface where TOML quoted keys land in the Go map. The regex test in 11.2 will pass against a Go string `"github.com/foo/bar"` constructed in test code, but the end-to-end "user writes this file and it parses" path is untested for this case. **CONFIRMED counterexample.**

### C3 — Unit 11.4 exit-code ambiguity on validation failure (CONFIRMED, medium severity)

**Construction:** Plan line 193 states: "Exits 0 and prints `"tools.toml is valid"` on a valid non-empty manifest. Exits 0 and prints `"no tools declared"` when manifest is empty (absent file or empty `[tools]` map). Exits 1 with a human-readable error message on parse or validation failure."

Cases:
- **Case A: file absent** → `Resolve` returns `ToolManifest{}, nil` → `valv tools validate` prints "no tools declared", exits 0. Clear.
- **Case B: file present, parses cleanly, empty `[tools]` map** → `Resolve` returns `ToolManifest{Tools: nil or empty map}, nil` → prints "no tools declared", exits 0. Clear.
- **Case C: file present, parses cleanly, has tools, all valid** → `Resolve` returns full manifest, nil → prints "tools.toml is valid", exits 0. Clear.
- **Case D: file present, malformed TOML** → `Load` returns wrapped TOML error → `Resolve` returns wrapped error → exit 1 with TOML error. Clear.
- **Case E: file present, parses cleanly, has tools, validation fails (e.g. tool name has space)** → `Resolve` returns wrapped error from `Validate` → exit 1 with validation error. Clear.
- **Case F: file present, ZERO BYTES** → `toml.DecodeFile` on a zero-byte file: per BurntSushi/toml, an empty TOML file is valid (zero tables, zero keys). Decodes to zero-value `ToolManifest`. `meta.Undecoded()` returns empty. → empty manifest. → prints "no tools declared", exits 0. Probably correct, but **not in the test table**.
- **Case G: file present, UNREADABLE due to permissions (chmod 000)** → `os.Stat` succeeds (returns file info), `toml.DecodeFile` fails on `os.Open` with permission denied. → `Load` returns `fmt.Errorf("decode ...: %w", err)`. → exit 1 with permission error. Probably correct but **not in the test table**.
- **Case H: file present, valid TOML but file is a directory** (`.valv/tools.toml` is somehow a directory) → `os.Stat` succeeds, `os.IsNotExist` is false, sentinel mapping doesn't fire. `toml.DecodeFile` opens it → fails with "is a directory" error. → exit 1. Edge case, not in the test table.

**File:line evidence:**
- Plan `PLAN.md:193` — exit-code spec for `valv tools validate`
- Plan `PLAN.md:196` — `TestToolsValidate` test cases: "valid manifest, empty manifest, missing file, invalid manifest (fails validation)"

The acceptance criterion's test cases enumerate 4 cases (matching A/B/C+E roughly), but skip:
- Case F (zero-byte file)
- Case G (permission error)
- Case H (directory at the path)

For a CLI surface, the test table should explicitly cover at minimum zero-byte and permission errors. These are real user-facing failure modes — corrupted file (Case F) and `chmod`-ed file (Case G).

**Severity:** Mid. CLI gives correct-looking exit codes in the happy path but the failure-mode coverage isn't on the test table. **CONFIRMED — the gap is in Unit 11.4 acceptance bullet 7 (test cases enumeration).** Fix: extend Unit 11.4's `TestToolsValidate` table to cover zero-byte and unreadable-due-to-permissions explicitly.

### C4 — Regex `^[a-zA-Z0-9][a-zA-Z0-9._/-]*$` accepts trailing punctuation (LOW SEVERITY, design question)

**Construction:** The regex anchors only the START. It accepts:
- `bad-name-` (trailing dash)
- `bad.name.` (trailing dot)
- `bad/name/` (trailing slash)
- `bad_name_` (trailing underscore)

It also accepts middle-only-punctuation like `a---b`, `a...b`, `a/.-/_.b`.

The prompt suggested anchoring the END too: `^[a-zA-Z0-9]$|^[a-zA-Z0-9][a-zA-Z0-9._/-]*[a-zA-Z0-9]$`.

**Is this a real defect?** Reasonable people disagree. Trailing punctuation in a tool name is bizarre but not catastrophic. The user-facing surface is "tool was declared as `mage-`" — DROP_12 may struggle to find an install action for that name, but DROP_11 doesn't break.

**Severity:** Low / design question. Either:
1. Tighten the regex to disallow trailing punctuation (plan should update the regex string and add test cases `"trailing-dash-"` invalid, `"trailing.dot."` invalid).
2. Document intent: "Trailing punctuation is permitted because the regex is for safety (reject whitespace/control chars), not naming style. DROP_12 may reject names DROP_11 accepts." — that's also fine.

**Recommendation:** raise to dev decision. Not a CONFIRMED failure of DROP_11's scope.

### C5 — `Resolve` does not invoke `internal/project.Detect` to find the project root (CONFIRMED, medium severity)

**Construction:** Plan line 193 says `valv tools validate` "calls `tools.Resolve(os.Getwd())`."

But `internal/project/project.go` defines `Detect()` and `DetectFrom()` — the existing canonical project-root detection logic. It walks parent directories looking for `.git`. So if a user runs `valv tools validate` from `/path/to/project/sub/dir/`, the existing convention would find the project root at `/path/to/project/` and look for `.valv/tools.toml` there. Using `os.Getwd()` raw means the user must `cd` to the project root first.

The plan says (line 14): "Projects place a `.valv/tools.toml` file at their project root." But the CLI implementation never resolves "project root" — it uses CWD verbatim.

**File:line evidence:**
- Plan `PLAN.md:193` — `tools.Resolve(os.Getwd())`
- `internal/project/project.go:19-25` — `Detect()` resolves project root via git-marker walk
- `internal/project/project.go:28-58` — `DetectFrom(start)` for explicit root

**Inconsistency:** The drop introduces a per-project file but doesn't use the existing project-root detection. Either:
1. **Intentional, simple:** "DROP_11 looks at CWD only. Subdirectory invocations get 'no tools declared'. User runs from project root." — needs to be stated explicitly in plan + reflected in CLI help text. Add to acceptance criterion 7.
2. **Bug:** Use `project.Detect()` to find the project root, then `Resolve(result.Root)`. Aligns with existing convention.

**Severity:** Medium. Inconsistent UX vs `valv account`/`valv codex` (which Detect-resolve). At minimum, the plan needs to explicitly state which behavior is intended. **CONFIRMED — gap in Unit 11.4 acceptance.**

### C6 — Acceptance criterion 7 contradicts Unit 11.4 line 193 on "empty `[tools]` map" emptiness check

**Construction:** Drop-level acceptance criterion 7 (line 75): "exits 0 and prints `"no tools declared"` when manifest is empty (no `.valv/tools.toml` or empty `[tools]` map)."

Unit 11.4 line 193 matches this. But neither defines how `Resolve` signals "empty manifest" to the CLI. `Resolve` returns `ToolManifest{}, nil` for the absent-file case. For the present-but-empty case, `Resolve` calls `Load` (which returns `ToolManifest{Tools: nil or empty}`), then `Validate` (which the plan says returns nil for "empty tools map (valid)" per line 152).

Both cases produce a `ToolManifest` with no tools. The CLI distinguishes "empty" from "valid non-empty" by checking `len(m.Tools) == 0`. That's not explicitly written into the plan but is implied.

**Severity:** Very low — implicit but obvious. Plan could add one sentence to acceptance criterion 7: "Emptiness check: `len(m.Tools) == 0`."

**Recommendation:** add to plan for clarity. Not a CONFIRMED defect.

## YAGNI Re-pressure

### Y1-R2 — Could Unit 11.0 collapse into Unit 11.3? (KEEP as proposed, but with C1's reordering)

The prompt suggested folding Unit 11.0 into Unit 11.1's first commit. But per C1 above, the correct fix is to move Unit 11.0 to AFTER Unit 11.3, not before Unit 11.1. The bump is genuinely independent of the new package's coverage — but the bump must fire only when every package, including `internal/tools/`, is already at 70%+.

So: **don't fold; reorder.** Keep Unit 11.0 as a standalone unit (it's a 1-line change, deserves its own commit), but flip its position: `blocked_by: Unit 11.3` rather than `blocked_by: nothing`. Drop-end verify still runs after 11.0 — order becomes 11.1 → 11.2 → 11.3 → 11.0 → 11.4.

Wait — does 11.4 depend on 11.0? 11.4 is the CLI surface. If Unit 11.4 lands without 11.0, `mage testPkg ./internal/cli/` enforces 60% (current threshold). If 11.0 lands before 11.4, threshold is 70% — 11.4's new `tools.go` + `tools_test.go` must be at 70% from their first commit. The existing `internal/cli/` package was at >60% (otherwise the current main wouldn't build); is it at >70%? Unknown without running coverage. Safest order: 11.1 → 11.2 → 11.3 → 11.4 → 11.0 → drop-end verify. 11.0 is the LAST unit.

If 11.0 runs last and fails (some package OTHER than `internal/tools/` is at <70%), Unit 11.0 acceptance line 95 already says: "list every failing package with its coverage percentage in the worklog, set unit state to `blocked`, and return to the orchestrator." Clean route. **Recommendation: reorder.**

### Y2-R2 — Are the four validation rules all in scope?

Rules:
1. Tool name regex
2. Object-form: both Source and Install non-empty
3. String-form: Version non-empty; Source/Install empty
4. Tool count ≤ 50

Are any in DROP_12's territory?

- Rule 1 (name regex): in scope. DROP_11 is the parser; rejecting invalid names is parser concern.
- Rule 2 (object form completeness): in scope. The schema declares object-form requires both fields. Rejecting partial is parser-validation.
- Rule 3 (mutex between string/object form): in scope. Same reason.
- Rule 4 (count ≤ 50): borderline. This is a resource-protection check, not a schema-validity check. DROP_12 (image build) would care because each tool layers the image. But DROP_11's `validate` command can usefully report "too many tools" before DROP_12 lands. **KEEP — small, fits cleanly.**

All four pass YAGNI re-attack.

### Y3-R2 — `Validate` as separate from `Load`?

Plan keeps `Load` (parse only, plus the file-absent sentinel) separate from `Validate` (rules). Could they fold?

**Argument for fold:** simpler — one call site, one error path.

**Argument against fold:** the prompt's specific question — "what if a user wants to call `Load` standalone (for diagnostics) and gets a parsed-but-not-yet-validated manifest?" — has a real use case: DROP_12's image-build path may want to see what the user declared even if validation fails, so it can produce a better error message ("you declared 51 tools; the limit is 50, drop one of: ...").

Also, the `internal/config` package (the reference pattern, line 209 of plan) has a single `Load` that does parse + format check. It does NOT have separate `Validate`. So this drop is choosing a different pattern from the reference.

**Verdict:** keep separation. The two-call API is mildly more verbose but gives DROP_12 the freedom to consume manifests at different validation stages. **No YAGNI violation; the separation is justified.**

## Hylla Feedback

N/A — this review needed only live file reads + Context7 docs queries. No Hylla queries issued (the drop's PLAN.md and a handful of existing-file references were the evidence base). The package under review is new (`internal/tools/`) — Hylla has no nodes for it yet.

---

## Summary for Orchestrator

- **Verdict: fail.** 3 CONFIRMED counterexamples (C1, C2, C3, C5), 2 lower-priority CONFIRMED gaps (C4, C6), 1 YAGNI-routed reordering recommendation (Y1-R2).
- **Most-critical:** C1 (Unit 11.0 ordering paradox — the 70% bump fires globally and will fail Unit 11.1 build-QA before `internal/tools/` is fully tested). Fix: move 11.0 to AFTER 11.3 (or after 11.4 to also catch any `internal/cli/` regression). New order: `11.1 → 11.2 → 11.3 → 11.4 → 11.0 → drop-end verify`.
- **High-impact UX:** C2 (TOML bare-key vs validation-regex mismatch — `github.com/foo/bar` is shown as a valid name but requires TOML quoting; not documented), C5 (CLI uses `os.Getwd()` instead of `project.Detect()`; inconsistent with rest of `valv` UX).
- **Moderate:** C3 (Unit 11.4 test table omits zero-byte and permission-error cases).
- **Design questions to route to dev:** C4 (regex anchoring END too?), C6 (explicit `len(m.Tools) == 0` check?), Y1-R2 (reorder confirms).
