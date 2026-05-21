# DROP_11 Plan QA Falsification — Round 3

**Verdict:** fail
**Reviewer:** general-purpose acting as go-qa-falsification-agent
**Reviewed at:** 2026-05-21T05:49:10Z

Summary up front: one empirical SHOWSTOPPER (NEW C2) directly contradicts a load-bearing claim the planner inherited and amplified in Round 3. Three smaller new counterexamples; two Round 2 mitigations confirmed effective; one Round 2 mitigation partially incorrect; one YAGNI re-pressure item.

---

## Round 2 Attack Re-verification

### R2-C1 — Unit ordering paradox (coverage bump last)

**Status:** MITIGATED.

Round 3 renamed Unit 11.0 → Unit 11.5 and made it the final unit. The Notes (line 246) explicitly call out: *"Bumping the global gate before all packages pass is a build-break risk."* The fix is correct.

**Residual sub-attack — new package failing the new gate (covered as NEW C1 below):** the acceptance criteria for Unit 11.5 do not distinguish "the known candidate `internal/adapters/docker` is below 70%" from "the newly created `internal/tools/` package is below 70%". Both surface the same way through `renderCoverage` (verified at `main/magefile.go:552-590` — `belowThreshold` accumulates all failing packages in one slice and the error message lists them all). Promoted to NEW C1.

### R2-C2 — TOML quoted-key handling

**Status:** PARTIALLY MITIGATED. Plan documents the requirement; one empirical claim in the plan is wrong.

Empirical test via `BurntSushi/toml@v1.6.0`:

| Input | Decode result |
|---|---|
| `"github.com/foo/bar" = { source = "...", install = "..." }` | single flat key `"github.com/foo/bar"` in `manifest.Tools`. No nested tables. |
| `github.com/foo/bar = { source = "...", install = "..." }` (unquoted) | **parse error**: `toml: line 2 (last key "tools"): expected '.' or '=', but got '/' instead` |

The plan's Note (Unit 11.1, line 120) says an unquoted dotted name *"would create nested TOML tables ... and NOT produce the expected flat map key — it would trigger the unknown-key error."* This is empirically half-wrong: with `/` in the name (as in the planner's own example `github.com/foo/bar`), it errors at PARSE time with a syntax error — not at the undecoded-key strict check. Pure dotted names (`foo.bar`) WOULD nest. The plan conflates two different failure modes.

**Severity:** minor (the fixture exercises the correct quoted form; the note's wording just mis-describes the failure path for the unquoted counterexample). Builder will see this immediately if they try the unquoted form. Worth fixing in the note, not blocking.

### R2-C3 — Unit 11.4 filesystem-edge test cases

**Status:** EMPIRICALLY VERIFIED on macOS — plan acceptance correct in outcome, but acceptance wording for the directory case is loose.

Tested locally:

- **chmod-000**: `os.ReadFile` returns `"open .../tools.toml: permission denied"`. `os.Stat` SUCCEEDS (returns valid FileInfo) — Stat is not a permission gate. If `Load` mirrors `internal/config/Load` (`os.Stat` then `toml.DecodeFile`), the Stat succeeds, then `toml.DecodeFile`'s internal `os.Open` returns the permission error. Bubbles up wrapped. Acceptance satisfiable.
- **directory-at-path**: `os.Stat` succeeds and returns `IsDir() = true`, `IsRegular() = false`. If `Load` does not pre-check `IsRegular()`, `toml.DecodeFile` will call `os.Open` (succeeds for a directory) and the subsequent `Read` returns `"read .../tools.toml: is a directory"`. Acceptance #7 of Unit 11.4 says the error must *"indicate path is not a regular file"*. The actual bubbled error reads *"is a directory"* — adjacent but not identical wording. **Plan should either** (a) specify that `Load` pre-checks `info.Mode().IsRegular()` and returns a Valv-authored error, or (b) loosen the acceptance to "error indicates the path is not a usable file (directory / permission / etc.)".

**Severity:** minor — promoted to NEW C3.

CI-runner concern: macOS host-builder verification confirms chmod-000 produces a real permission-denied error here. Linux CI runners running as root (e.g. some GitHub Actions container images) ignore chmod 000 — `os.ReadFile` succeeds anyway. Valv CI on `ubuntu-latest` runs as a non-root user by default, so this is fine in normal CI. Flagging as a residual risk: if Valv CI ever moves to a root-in-container test runner, this test silently regresses. Worth a one-line comment in the test file.

### R2-C4 — Regex edge cases

**Status:** MITIGATED for documented cases; UNDEFINED behavior worth pinning down.

Empirical test (`regexp.MustCompile(`^[a-zA-Z0-9]+$|^[a-zA-Z0-9][a-zA-Z0-9._/-]*[a-zA-Z0-9]$`).MatchString(...)`):

| Input | Match | Plan expectation |
|---|---|---|
| `a` | true | accept (implicit) |
| `github.com/foo/bar` | true | accept (Unit 11.2) |
| `go-1.22` | true | accept (Unit 11.2) |
| `a..b` | **true** | UNDEFINED — Round 2 raised this; plan still does not state |
| `a//b` | **true** | UNDEFINED — Round 2 raised this; plan still does not state |
| `a---b` | **true** | UNDEFINED — Round 2 raised this; plan still does not state |
| `_underscore` | **false** | UNDEFINED — leading `_` is rejected (because anchor class excludes `_`) |
| `日本語` | false | implicit (non-ASCII rejected) |
| `bad-name-` | false | reject (Unit 11.2 acceptance) |
| `name.` | false | reject (Unit 11.2 acceptance) |
| `name/` | false | reject (Unit 11.2 acceptance) |
| `-bad` | false | reject (Unit 11.2 acceptance) |
| `bad!char` | false | reject (Unit 11.2 acceptance) |
| `1go` | true | not stated — accepted because anchor class includes `0-9` |
| `a-` | false | reject — implicit in trailing-punct rule |
| `a` (single char) | true | accept — first alternation handles single char |

**Three previously-flagged edge cases (`a..b`, `a//b`, `a---b`) all MATCH the regex.** The plan accepts them by silence. No Unit 11.2 test case asserts behavior for double-dot / double-slash / repeated-dash. If accepted-by-silence is intentional, plan should say so. If unintentional, regex needs tightening (e.g., disallow consecutive punctuation), which RE2 can do with `[a-zA-Z0-9](?:[._/-]?[a-zA-Z0-9])*` style. Promoted to NEW C4.

**Leading-underscore `_underscore` is rejected** because the anchor class `[a-zA-Z0-9]` excludes `_`. Plan never says whether names starting with `_` should be allowed. Some Go-style names use underscores (`go_module`); the regex permits `_` mid-name but not as the first character. Worth one acceptance bullet pinning this down.

### R2-C5 — `project.Detect()` signature + non-project CWD

**Status:** MITIGATED — verified empirically at `main/internal/project/project.go:19-58`.

- Signature: `func Detect() (Result, error)`. Matches plan line 235.
- `Result.Root` is **never empty** when called from a valid CWD. If no `.git` marker is found walking up to `/`, `Result.Root = fallback = normalized start = CWD`. So `result.Root` for `valv tools validate` run from `/tmp` is `/tmp`, and `tools.Resolve("/tmp")` will look for `/tmp/.valv/tools.toml` (absent → `ToolManifest{}, nil`).
- `Result.HasGitMarker = false` when not inside a git tree. **Unit 11.4 acceptance does not consult `HasGitMarker`** — it always proceeds to `tools.Resolve(result.Root)`. The "no tools declared" output will fire for any CWD outside a project. That's the correct behavior (graceful empty state), but the plan should clarify: running `valv tools validate` from a non-project directory prints `"no tools declared"` and exits 0, NOT an error about "not in a project". Worth pinning in Unit 11.4's acceptance #3 wording.

### R2-C6 — `len(m.Tools) == 0` predicate ambiguity (nil vs empty map)

**Status:** EMPIRICALLY MITIGATED.

Tested both decode paths:

- TOML with no `[tools]` key at all → `m.Tools` is **nil**, `len(nil) == 0` is `true`. Go spec: `len` of a nil map returns 0. Predicate safe.
- TOML with `[tools]` declared but empty (`[tools]\n`) → `m.Tools` is **non-nil empty map**, `len() == 0`. Predicate safe.

Both paths print `"no tools declared"` and exit 0 in Unit 11.4. No bug.

---

## New Counterexamples (Round 3)

### NEW C1 — Unit 11.5 acceptance cannot distinguish "new package failing new gate" from "legacy docker failing new gate"

**Severity:** medium — plan-level wording gap, not a code bug.

**Evidence:** `main/magefile.go:552-590` — `renderCoverage` accumulates ALL packages below threshold into `belowThreshold` slice and returns one error: `fmt.Errorf("coverage below %.1f%% for: %s", threshold, strings.Join(belowThreshold, ", "))`.

**Counterexample trace:** Suppose at Unit 11.5 the post-bump `mage test` reports:
- `internal/adapters/docker` = 47.0% (pre-existing low)
- `internal/tools` = 64.0% (new package, just below 70%)

Unit 11.5's acceptance (line 219) says: *"If any package reports < 70% coverage, the builder does NOT silently fix it. Instead: list every failing package with its coverage percentage in the worklog, set unit state to `blocked`, and return to the orchestrator."*

This handles both packages identically. But Unit 11.4's acceptance (line 200) says: *"≥ 70% coverage across the whole package (this unit completes the package by exercising the full `Resolve` → `Validate` path via CLI tests — 70% gate enforced here)."*

If Unit 11.4 passed its per-package coverage gate cleanly via `mage testPkg ./internal/tools/`, but Unit 11.5's global `mage test` reports `internal/tools` < 70%, **something is wrong** — `mage testPkg` and `mage test` should report identical per-package coverage for the same package. Either:

(a) `mage testPkg` and `mage test` use different `-cover` flags / package selectors, OR  
(b) Unit 11.4's "70% gate enforced here" was satisfied by some package-local coverage measurement that doesn't match the global one.

Plan should either prove `mage testPkg ./internal/tools/` coverage = `mage test`'s `internal/tools` row coverage, or accept that Unit 11.5 may legitimately reveal a coverage regression and add a "this means Unit 11.4's gate was lying" diagnostic to the worklog template.

**Mitigation path:** Add to Unit 11.5 acceptance: *"If `internal/tools` itself is listed below 70%, route back to Unit 11.4 as a failed-gate finding — do not treat as a legacy-package coverage issue. Only `internal/adapters/docker` and other pre-existing packages are treated as known low-coverage candidates."*

### NEW C2 — SHOWSTOPPER: `toml.Primitive` does NOT auto-satisfy `meta.Undecoded()` strict check

**Severity:** SHOWSTOPPER — drop-level acceptance #2 is unsatisfiable as written.

**Evidence:** empirical test against `github.com/BurntSushi/toml v1.6.0` (the version Valv uses):

```go
type Manifest struct {
    Tools     map[string]ToolSpec `toml:"tools"`
    Allowlist toml.Primitive      `toml:"allowlist"`
    Env       toml.Primitive      `toml:"env"`
}

// Input:
//   [tools]
//   mage = "latest"
//
//   [allowlist]
//   hosts = ["github.com"]
//
//   [env]
//   GOPRIVATE = "foo"

// Result:
//   undecoded BEFORE PrimitiveDecode: [allowlist.hosts env.GOPRIVATE]
//   undecoded AFTER PrimitiveDecode: []
```

**The Round 3 plan claims (line 67, repeated line 230):** *"During initial `toml.DecodeFile`, these fields receive their raw TOML values and are marked as decoded — `meta.Undecoded()` returns empty (strict check satisfied)."* And: *"DROP_11 does not call `PrimitiveDecode` at all."*

**This is empirically FALSE.** Only the top-level keys `allowlist` and `env` are decoded; their CHILD KEYS (`allowlist.hosts`, `env.GOPRIVATE`) remain in `meta.Undecoded()` until `PrimitiveDecode` is called on them. Without `PrimitiveDecode`, the strict undecoded check will fire on every valid file that uses `[allowlist]` or `[env]`.

**Consequences:**

1. **Drop-level acceptance #2 fails:** *"`Load(path)` returns a correctly-typed `ToolManifest` for a well-formed `.valv/tools.toml` covering string-value tools, object-value tools, `[allowlist]`, and `[env]` sections."* — with the plan's current `Load` flow, this errors on any file containing `[allowlist]` or `[env]` content.
2. **Unit 11.1 test `testdata/valid_objects.toml` (which is documented as containing both sections per line 98) will FAIL the strict undecoded check.**
3. **Acceptance line 116** says: *"`Load` accepts files with `[allowlist]` and `[env]` sections without error (they decode into `toml.Primitive` fields — not undecoded)."* — empirically wrong.
4. The plan's Context7 citation *"Delayed TOML Decoding example confirms `toml.Primitive` fields satisfy the undecoded check"* is being misread. The Context7 example shows how to USE `PrimitiveDecode` to defer interpretation. It does not claim `Primitive` fields are auto-decoded for strict-check purposes — and empirically they are not.

**Three possible fixes, all of which the planner must pick from in Round 4:**

(a) **Call `PrimitiveDecode` in `Load` itself**, but discard the result (or decode into a discard struct). This satisfies the strict check while still leaving DROP_14/DROP_15 to redefine the semantics later. Trivial to implement; trivially documented.

(b) **Filter the undecoded check** to ignore keys under `allowlist.*` and `env.*` prefixes. More fragile, hand-rolls a special case, makes the code harder to read.

(c) **Drop the strict undecoded check entirely** for DROP_11 and add it back in DROP_14/DROP_15 once those drops own the typed decode. Permissive — loses the schema-explicitness benefit. Plan's whole point about catching `[network]` as an unknown-key error (line 67) would regress.

(d) **Defer `[allowlist]` and `[env]` to DROP_14/DROP_15** — don't declare them in `ToolManifest` at all in DROP_11. Files containing them will fail Load with "undecoded key allowlist". Loses forward-compat ergonomics; users can't put those sections in until DROP_14 lands.

**My recommendation:** (a). One line added to `Load`. Documented in Notes. Preserves the strict-check semantics the plan correctly wants.

**Required plan changes:**

- Rewrite line 67's bullet on `[allowlist]` and `[env]`.
- Rewrite line 230's Notes-for-builder.
- Add to Unit 11.1 acceptance: *"`Load` calls `meta.PrimitiveDecode(manifest.Allowlist, &discard)` and `meta.PrimitiveDecode(manifest.Env, &discard)` where `discard` is a local `map[string]interface{}` — this marks the contents as decoded for the strict-check sweep without binding their schema. Errors from `PrimitiveDecode` are wrapped and returned. The plan's claim that 'DROP_11 does not call PrimitiveDecode' is reversed."*

### NEW C3 — Unit 11.4 directory-at-path acceptance wording vs. actual error

**Severity:** minor.

**Evidence:** empirical macOS test. With a directory at `.valv/tools.toml`, `os.Stat` succeeds and `info.IsDir() == true`. If `Load` does not pre-check `IsRegular()`, the bubbled error is `"read .../tools.toml: is a directory"` from the lower-level `Read` syscall, wrapped by `toml.DecodeFile`.

**Counterexample to Unit 11.4 acceptance #7** (line 198): *"exits 1, error message indicates path is not a regular file"*. The actual error says "is a directory", not "not a regular file". Both convey the same idea, but the acceptance wording prescribes a phrasing the implementation does not produce.

**Two mitigation paths:**

(a) Plan specifies `Load` calls `info.Mode().IsRegular()` after `os.Stat` and returns `fmt.Errorf("%s: not a regular file: %w", path, domain.ErrSomething)` — Valv-authored wording matches the acceptance.

(b) Plan loosens the acceptance to *"error message indicates the path is not a usable regular file (directory or otherwise non-file)"*.

Either is fine; planner should pick.

### NEW C4 — Regex accepts undocumented edge cases (`a..b`, `a//b`, `a---b`)

**Severity:** minor.

**Evidence:** empirical regex test. All three forms MATCH. No Unit 11.2 test case asserts behavior for double-punctuation. The plan documents `bad-name-`, `name.`, `name/` as REJECT cases but says nothing about `a..b` / `a//b` / `a---b`.

**Why this matters for falsification:** acceptance-by-silence is a counterexample seed for build-QA Round K. A future build-QA reviewer can reasonably ask *"why is `foo..bar` accepted?"* and the answer must already be in the plan. If the plan is silent, the builder will guess and either:
- write a test case asserting acceptance (lock in current behavior), or
- tighten the regex (drift from the plan).

**Mitigation:** Plan should add one of these to Unit 11.2 acceptance:

(a) *"Names with consecutive punctuation (`a..b`, `a//b`, `a---b`) are accepted as a side-effect of the simple regex. Acceptable for v1 because no installer in DROP_12 will produce such names."*

(b) *"Tighten the regex to `^[a-zA-Z0-9](?:[._/-]?[a-zA-Z0-9])+$|^[a-zA-Z0-9]$` to forbid consecutive punctuation. RE2-compatible."* This also rejects `_underscore` cleanly because the start anchor still excludes `_`.

Either is fine; planner picks.

Also: plan does not state behavior for **leading underscore** (`_module`). Currently rejected. Worth one explicit bullet.

---

## YAGNI Re-pressure

### YAGNI-1 — Unit 11.5 as its own atomic unit

**Counter-pressure:** the unit is "edit one constant on line 24 + run `mage test` + report results". Phase 6 of `main/drops/WORKFLOW.md` already runs `mage test` from `main/` as the drop-end verification gate. Why not fold the edit into Phase 6 itself, or into Unit 11.4 (which is the last "real" unit)?

**Arguments for keeping it as 11.5:**

1. **Atomicity of failure routing.** If the bump fails, the failure has a single owner (Unit 11.5) and a single rollback target. Folding it into 11.4 or Phase 6 muddies the rollback story.
2. **QA gate per unit.** WORKFLOW.md Phase 5 requires per-unit build-QA. A bump that surfaces a coverage regression on `internal/tools` is a genuine code-quality finding that benefits from QA-Proof + QA-Falsification review independent from Unit 11.4's review.
3. **Memory `feedback_interface_change_runs_full_mage_test.md`** says interface-method additions can break sibling-package mocks that `mage testPkg` misses. Unit 11.4 only runs `mage testPkg ./internal/tools/` and `mage testPkg ./internal/cli/`. Unit 11.5 is the FIRST time the full `mage test` runs post-DROP_11. That's a load-bearing gate even if it's mechanically trivial.

**Verdict:** keep Unit 11.5. The triviality of the edit is the POINT — it isolates one cross-package risk into one unit.

But: **the plan should explicitly cite reason (3)** in Unit 11.5's Notes. As-written, it only justifies the unit by appealing to coverage-bump risk on `internal/adapters/docker`. The broader cross-package-mock risk is the real reason it stands alone.

### YAGNI-2 — `valv tools validate` CLI command vs. `mage test`

**Counter-pressure:** the dev runs `mage test` regularly, which already exercises the `internal/tools/` package. A standalone `valv tools validate` adds a separate CLI surface (~50 lines) for a use case (`"my .valv/tools.toml is broken"`) that `mage test` does not address — but neither does `valv tools validate` for a non-Valv-dev user.

**Counter-counter:** `valv tools validate` runs in a project's CWD against the project's own `.valv/tools.toml`. `mage test` runs against fixtures inside `internal/tools/testdata/`. They are NOT redundant. `valv tools validate` is the user-facing diagnostic for a user who has authored their own `.valv/tools.toml` and wants to check it. That's a real, non-substitutable use case.

Memory `feedback_manual_workflow_is_the_decision.md` does not apply here — the dev hasn't proposed a manual workflow. The CLI command is the canonical UX.

**Verdict:** keep.

### YAGNI-3 — `valid_quoted_names.toml` fixture

**Counter-pressure:** the doc bullet (Unit 11.1 line 120, Notes line 229) documents the requirement that dotted/slashed tool names MUST be quoted. Is a separate testdata fixture necessary if the doc already says it?

**Counter-counter:** the fixture EXECUTES the test. Documentation alone is not a regression gate. Without the fixture, a future BurntSushi/toml upgrade that changed quoted-key handling would silently regress and only surface when a user filed a bug. The fixture is small (one file, three lines), runs in <1ms, and locks behavior.

Plus: NEW C2 shows that empirical behavior of BurntSushi/toml does not always match plan claims. The fixture is exactly the kind of guardrail needed.

**Verdict:** keep.

---

## Hylla Feedback

Hylla was not consulted in this review — every empirical attack was answered via `Read`, local Go test programs against the project's `BurntSushi/toml` version, or direct file inspection. No Hylla miss to report.

---

## Verdict Rationale

**fail.** NEW C2 alone is sufficient: drop-level acceptance #2 (Load handles `[allowlist]` and `[env]` cleanly) is empirically unsatisfiable as the plan currently specifies. The fix is small (one `PrimitiveDecode` call per Primitive field in `Load`) but the plan must be revised to reflect it, and the Notes-for-builder (line 230) must be reversed.

NEW C1, C3, C4 are smaller plan-wording issues that would not by themselves fail the plan, but compound the picture: the plan has accumulated several places where prescriptive claims about empirical behavior have drifted from what the libraries actually do. Round 4 should sweep all four together.

Round 2's six counterexamples are otherwise well-handled in Round 3 — the ordering paradox, regex documentation, `project.Detect` signature, and `len(m.Tools)` predicate are all solidly resolved.

---

## Summary for Orchestrator

- **Verdict:** fail
- **New counterexamples:** 4 (C1 medium, C2 SHOWSTOPPER, C3 minor, C4 minor)
- **YAGNI items raised:** 3 — all dismissed in favor of keeping the current unit/fixture/CLI shape; one yields a wording suggestion for Unit 11.5 Notes
- **Round 2 attacks:** 6 re-verified — 4 fully mitigated (C1, C5, C6, mostly C3), 2 partially mitigated with wording gaps (C2, C3, C4)
- **Recommendation:** route C2 to planner Round 4 with explicit instruction to revise Unit 11.1 acceptance + Notes line 230 + line 67. Bundle C1/C3/C4 wording fixes in the same round.
