# DROP_11 Plan QA Falsification — Round 4

**Verdict:** pass
**Reviewer:** ta-go-qa-falsification
**Reviewed at:** 2026-05-21T00:00:00Z

Round 4 closes all four Round 3 counterexamples (1 SHOWSTOPPER + 3 minor) with empirical grounding. Re-attacked each fix with a fresh scratch Go program built against `BurntSushi/toml v1.6.0` plus the schema in PLAN.md. All Round 4 claims survive. Three small new wording-tightening suggestions flagged below; none rise to blocker level. One YAGNI re-pressure item dismissed.

---

## Round 3 Attack Re-verification

### NEW C1 (Unit 11.5 escalation distinction) — RESOLVED

Round 4 lines 235-237 split routing:

- **New-pkg failure** (`internal/tools/`) → route back to Unit 11.4 (which owns the per-package gate). Same drop, tightly scoped fix.
- **Legacy-pkg failure** (e.g. `internal/adapters/docker`) → route to dev for triage. Dev decides either follow-on unit inside DROP_11 or rollback the bump.

Unambiguous. The two failure classes have distinct owners. PASS.

Residual minor: the line 219 phrasing in older drafts ("does NOT silently fix it") survived into Round 4 line 234 verbatim — fine; just noting the wording is consistent across the unit.

### NEW C2 (SHOWSTOPPER — `PrimitiveDecode` mandatory) — RESOLVED, EMPIRICALLY CONFIRMED

Round 4 reverses the Round 2/3 claim. Plan now requires:

- `ToolManifest.Allowlist` and `ToolManifest.Env` are `toml.Primitive` (PLAN.md line 71).
- `Load` calls `meta.PrimitiveDecode` on each, with a `map[string]any` discard target, **before** the `meta.Undecoded()` strict check (PLAN.md line 123-124).
- The empirical evidence panel at line 73-76 reproduces what falsification reported in Round 3.
- The Notes-for-builder block at line 247 elevates this from "recommendation" to "MANDATORY" and explicitly reverses the prior wrong claim.

Empirical re-run (scratch program against `BurntSushi/toml v1.6.0`, manifest schema from PLAN.md, then deleted):

```
undecoded BEFORE PrimitiveDecode: [allowlist.hosts env.GOPRIVATE]
undecoded AFTER  PrimitiveDecode: []
```

Matches the plan's quoted evidence verbatim. PASS.

**Sub-attacks I tried against the Round 4 fix:**

| Probe | Result | Verdict |
|---|---|---|
| Discard target = `map[string]any` (plan's choice) | undecoded → `[]`; values stored in discard map | PASS |
| Discard target = `interface{}` | undecoded → `[]`; same result | PASS (alternative works) |
| Discard target = `toml.Primitive` (re-Primitive) | undecoded **STILL** `[allowlist.hosts]` — does NOT clear strict check | **FOOTGUN** — but plan picked `map[string]any`, so doesn't bite |
| Discard target = empty struct `struct{}` | undecoded → `[]`; no error | PASS (alternative works) |
| Zero-valued `toml.Primitive` (no `[allowlist]` in file) | `PrimitiveDecode` succeeds, no panic, discard map is empty | PASS — Load can unconditionally call PrimitiveDecode |
| `[allowlist]` with no fields inside | `PrimitiveDecode` succeeds; undecoded already empty pre-call | PASS |
| Malformed section (`hosts = "string"` instead of array) | `PrimitiveDecode` to `map[string]any` SILENTLY succeeds with `{hosts: "string"}` — DROP_14 will reject when it adds its typed target | PASS — DROP_11 correctly defers typed validation |
| Idempotence: call `PrimitiveDecode` once with discard, then again with typed `AllowlistConfig` target | Both calls succeed; typed target gets the real value; undecoded stays empty | PASS — DROP_14 CAN re-call after DROP_11's discard call |

**Critical idempotence finding:** DROP_14/DROP_15 can re-call `meta.PrimitiveDecode` with a typed target after DROP_11's discard call. Empirically verified. The plan implies this at line 71 but does not state the idempotence guarantee explicitly — see "New Counterexamples (Round 4) → R4-1" below for a minor wording suggestion.

### NEW C3 (directory-at-path) — RESOLVED, EMPIRICALLY CONFIRMED

Round 4 line 213 accepts `errors.Is(err, syscall.EISDIR)` OR string contains `"is a directory"`.

Empirical (macOS Darwin 25.3.0, Go 1.26.3):

| Call | Result |
|---|---|
| `os.Open(dir)` | returns valid `*File`, `err == nil` (NOT an error) |
| `io.ReadAll(file)` on dir | error: `"read <path>: is a directory"`, `errors.Is(err, syscall.EISDIR) == true` |
| `os.ReadFile(dir)` | same: `"read <path>: is a directory"`, `EISDIR == true` |
| `toml.DecodeFile(dir, &m)` | same: `"read <path>: is a directory"`, `EISDIR == true` |

The plan's claim at line 213 is accurate: `toml.DecodeFile` opens then reads, surfacing `EISDIR`. The acceptance disjunction (`errors.Is(err, syscall.EISDIR)` OR string `"is a directory"`) is permissive enough that any of the three implementation paths (use `os.Open + ReadAll`, `os.ReadFile`, or `toml.DecodeFile` directly) all satisfy it. PASS.

### NEW C4 (regex edge cases pinned) — RESOLVED, EMPIRICALLY CONFIRMED

Round 4 line 67 adds:

- **Accept by design:** multiple consecutive `.`, `/`, or `-` (`a..b`, `a//b`, `a---b`); path-like names; namespaced names.
- **Reject by design:** empty string; trailing punctuation; leading dash; leading `_`; whitespace; `!`; non-ASCII.

Empirical regex run against the planner's pinned pattern `^[a-zA-Z0-9]+$|^[a-zA-Z0-9][a-zA-Z0-9._/-]*[a-zA-Z0-9]$`:

| Input | Matches | Plan expectation | Verdict |
|---|---|---|---|
| `mage`, `go`, `a`, `github.com/foo/bar`, `go-1.22` | true | accept | PASS |
| `a..b` | true | accept-by-design | PASS |
| `a//b` | true | accept-by-design | PASS |
| `a---b` | true | accept-by-design | PASS |
| `mage.` (single trailing dot) | false | reject (per the second alternative — fails trailing-alphanumeric anchor) | PASS |
| `..mage` (leading two dots) | false | reject (branch 1 fails: has `.`; branch 2 fails: leading `.` not in start class) | PASS |
| `m.` (length 2, trailing dot) | false | reject (branch 1 fails: has `.`; branch 2 needs alphanumeric trailing — `.` is not) | PASS |
| `_underscore` | false | reject-by-design (leading `_` not in start class) | PASS |
| `bad-name-`, `name.`, `name/`, `-bad`, `bad!char`, `""` | false | reject (each per the plan) | PASS |

All Round 4 pins match RE2 behavior exactly. PASS.

Minor coverage gap: `a//b` is in the accept-by-design list (PLAN.md line 67) but Unit 11.2's TestValidate cases at line 159 list only `a..b` and `a---b`. If the planner wants the test to lock all three accept-by-design forms, adding `a//b` to the test cases would close that gap — but the acceptance is satisfied without it because the regex behavior is fully constrained by the pinned pattern. Minor wording-only suggestion; not a counterexample.

---

## New Counterexamples (Round 4)

Three small wording-tightening suggestions. None are blockers; collectively they would tighten the plan's empirical grounding.

### R4-1 (MINOR) — `PrimitiveDecode` idempotence is empirically guaranteed but not stated

**Severity:** minor — a subtle DROP_14/DROP_15 reader could miss this.

**Evidence:** scratch Go program against `BurntSushi/toml v1.6.0`. First call: `meta.PrimitiveDecode(m.Allowlist, &discardA)` where `discardA` is `map[string]any` — succeeds, undecoded sweeps to `[]`. Second call (simulating DROP_14): `meta.PrimitiveDecode(m.Allowlist, &typed)` where `typed` is a typed `AllowlistConfig` struct — succeeds, `typed.Hosts` is populated with the real `[]string{"github.com"}`, undecoded stays empty.

**Why it matters:** the plan promises DROP_14 will "re-call `meta.PrimitiveDecode` with typed targets" (line 71). A reader could reasonably worry that the discard call consumes the Primitive (one-shot). Empirically it does NOT — `toml.Primitive` carries its raw value across multiple decodes against the SAME `MetaData`. The plan would benefit from one sentence stating this:

> `toml.Primitive` is re-decodable: DROP_14/DROP_15 can call `meta.PrimitiveDecode` again with their typed targets after DROP_11's discard call has already run. Empirically verified against `BurntSushi/toml v1.6.0`.

**Severity rationale:** the absence of this sentence does not break DROP_11 — DROP_11's behavior is correct as written. The risk is forward-looking ergonomics for DROP_14.

**Mitigation path:** one sentence added to PLAN.md line 71 or to the Notes-for-builder block (line 247).

### R4-2 (MINOR) — Discard target shape is "map[string]any (or any throwaway target)" — too permissive

**Severity:** minor — wording could mislead a builder.

**Evidence:** PLAN.md line 123-124 says:

> "calls `meta.PrimitiveDecode(m.Allowlist, &discardA)` and `meta.PrimitiveDecode(m.Env, &discardE)` where `discardA` and `discardE` are local `map[string]any` (or any throwaway target)"

The parenthetical "(or any throwaway target)" is too permissive. My empirical test (Test 6 in the scratch run) showed:

| Discard target type | Undecoded after call |
|---|---|
| `map[string]any` | `[]` — correct |
| `interface{}` | `[]` — correct |
| empty `struct{}` | `[]` — correct |
| `toml.Primitive` (re-Primitive) | **`[allowlist.hosts]`** — DOES NOT clear the strict check |

A builder reading "any throwaway target" might pick `toml.Primitive` as a "neutral" pass-through — which would silently break the strict-check sweep. The fix is one-line tightening: explicitly say `map[string]any` (or some other concrete decodable shape) and explicitly forbid `toml.Primitive` as the discard target.

**Mitigation path:** PLAN.md line 123 — replace "(or any throwaway target)" with "(use `map[string]any` specifically; do NOT use `toml.Primitive` as the discard target — empirically it leaves the inner keys undecoded)".

### R4-3 (MINOR) — Malformed-section pass-through is silent

**Severity:** minor — flagged for build-QA reviewer awareness, not a plan-breaker.

**Evidence:** scratch test 4 — a `.valv/tools.toml` with `[allowlist]\nhosts = "string-not-array"` (wrong type for the future field) decodes successfully through DROP_11's `Load`. `PrimitiveDecode` with `map[string]any` target accepts ANY shape and stores `{hosts: "string-not-array"}` in the discard map. The strict-check sweep is satisfied. DROP_11 returns no error.

This is **correct** for DROP_11 (the plan defers typed validation to DROP_14/DROP_15), but the plan doesn't explicitly call out the pass-through behavior. A user with a malformed forward-compat section gets no DROP_11 error and may be surprised when DROP_14 lands and starts rejecting it.

**Mitigation path:** PLAN.md Notes section (line 247) — add: "DROP_11's `map[string]any` discard target accepts any shape under `[allowlist]` / `[env]` (e.g. `hosts = "string"` instead of `hosts = ["array"]`). Typed-shape validation lands in DROP_14/DROP_15. DROP_11's contract is structural ('the section exists and is named correctly'), not semantic."

**Why this is small:** the alternative ("DROP_11 must validate `[allowlist]` / `[env]` shapes") would require typed structs that DROP_11 explicitly defers. The pass-through IS the right design — it just deserves one explicit sentence.

---

## YAGNI Re-pressure

### YAGNI-1 — `valid_quoted_names.toml` fixture after regex test cases

**Dismissed.** The regex tests (Unit 11.2) verify Valv's *name-validation* policy against strings. The fixture (Unit 11.1) verifies BurntSushi/toml's *quoted-key decode behavior* — that the string `"github.com/foo/bar"` under `[tools]` lands as a single flat key in the resulting `map[string]ToolSpec`. These two things are independent contracts:

- Regex test: "if you give Validate a string, does it accept or reject?" (Valv-owned regex)
- Fixture test: "if you give BurntSushi/toml a quoted dotted key, does it produce one map entry or nested tables?" (external library contract)

Without the fixture, a future `BurntSushi/toml` upgrade that changed quoted-key handling would silently regress. The fixture is the regression gate. The regex tests do NOT exercise this path.

**Verdict:** keep both. Plan correctly retains the fixture.

### YAGNI-2 — `meta.PrimitiveDecode(field, &discard interface{})` vs. typed-nil-pointer

**Dismissed.** The planner picked `map[string]any` as the discard target. Empirical test confirms it works correctly. A "typed nil pointer" alternative (`var discard *AllowlistConfig; PrimitiveDecode(field, &discard)`) would still decode the value INTO the pointed-to struct (after allocating it), which is more work than necessary and ALSO commits DROP_11 to knowing the typed shape — which is exactly what DROP_11 is trying NOT to do.

The plan's choice of `map[string]any` is the minimal-commitment shape. **Verdict:** correct as-is. (R4-2 above suggests tightening the *prose* but not changing the *choice*.)

---

## Cross-Reference Memory Audit

### `feedback_interface_change_runs_full_mage_test.md`

DROP_11 adds `domain.ErrToolsNotFound` to `internal/domain/errors.go`. This is a **new sentinel variable**, not a new interface method. New sentinels are purely additive — they do not break sibling-package mocks because mocks satisfy interfaces, not sentinel-error switches.

Audit of services packages that reference `domain.Err*`:

- `internal/services/manage/`, `internal/services/codex/`, `internal/services/claude/`, `internal/services/images/` — all reference `domain.ErrConfigNotFound` or `domain.ErrUnboundProject` but none exhaustive-switch over the `Err*` set. Adding a new sentinel is additive.

**Verdict:** no risk. Unit 11.5's full `mage test` will catch any unforeseen breakage anyway; the memory's specific concern (interface-method addition) does not apply.

### `feedback_check_official_docs_and_working_projects_first.md`

This memory is directly relevant. Round 4 reversed the Round 2/3 Context7-grounded claim about `toml.Primitive` because empirical Go behavior contradicted the docs reading. The plan now correctly grounds in empirical scratch-program output (line 73-76).

**Audit of other plan claims that rely solely on Context7 / docs without empirical verification:**

| Claim | Source | Empirically verified? |
|---|---|---|
| `toml.Primitive` requires `PrimitiveDecode` (PLAN.md line 71) | Round 4 scratch program | YES |
| `os.Stat` succeeds for chmod-000 (Unit 11.4 case 6) | Round 3 macOS test | YES |
| `os.Open(dir)` returns no error; `Read` returns EISDIR (Unit 11.4 case 7) | Round 4 (this review) | YES |
| `project.Detect()` signature + behavior (Unit 11.4) | Direct file Read of `internal/project/project.go:19` | YES |
| `root.go cmd.AddCommand` variadic site at line 137 (Unit 11.4) | Direct file Read of `internal/cli/root.go:137` | YES |
| Regex behavior on `a..b`, `mage.`, `_underscore` (Unit 11.2) | Round 3 + Round 4 regex test | YES |
| `len(nil-map) == 0` (Unit 11.4 / Schema Decisions) | Go spec (well-established) | spec-grounded |
| `BurntSushi/toml` mixed-value decode via `UnmarshalTOML` (Unit 11.1) | Context7 reference + would need empirical check at build time | partially verified — Context7 docs the API but the actual `UnmarshalTOML` integration is Unit 11.1 builder's job |

**Residual claim with partial empirical coverage:** the mixed-value `[tools]` decode (string-vs-inline-table) via `(t *ToolSpec) UnmarshalTOML` — this is the planner-cited critical mechanism from Unit 11.1, but my Round 4 scratch program only exercised `map[string]any` discard targets and didn't fully exercise the heterogeneous-value path. The Round 2 falsification documented this empirically against `BurntSushi/toml v1.6.0` and it's been the load-bearing assumption since Round 1. The plan documents the requirement clearly (line 245). **No new counterexample**, but flagging as a residual area where build-time `TestLoad` against `valid_objects.toml` is the actual gate.

### Other memories checked

- `feedback_drop_ceremony_trim.md`: Not applicable to plan QA.
- `feedback_trimmed_cascade_for_mechanical_drops.md`: DROP_11 is novel-logic (new package + parser semantics + forward-compat fields) — full cascade is appropriate. Not applicable.
- `feedback_manual_workflow_is_the_decision.md`: Not applicable — dev has not proposed a manual workflow for this drop.

---

## Hylla Feedback

No Hylla calls were required for this review. All Round 4 attacks were answerable via:

- direct `Read` of `main/drops/DROP_11_TOOL_DECLARATION/PLAN.md` (the plan under attack)
- direct `Read` of `main/drops/DROP_11_TOOL_DECLARATION/PLAN_QA_FALSIFICATION.md` (Round 3 to verify what changed)
- scratch Go program against `BurntSushi/toml v1.6.0` (for `PrimitiveDecode` semantics, regex behavior, directory-at-path error chain)
- `rg` for sentinel/mock cross-references in `internal/services/`
- direct `Read` of `magefile.go` for `coverageThreshold` line context

The plan cites specific file paths and line numbers, all of which are stable visible artifacts. Hylla vector/keyword search wasn't the right tool for empirically verifying TOML library behavior — scratch Go programs are the canonical evidence path there.

**Hylla miss to report:** none.

---

## Verdict Rationale

**pass.** Round 4 closes:

- **R3 NEW C2** (the SHOWSTOPPER): plan now mandates `PrimitiveDecode` on each `toml.Primitive` field with empirical evidence cited. Re-verified via scratch program — claim survives.
- **R3 NEW C1** (Unit 11.5 routing): split into new-pkg-vs-legacy-pkg routes. Unambiguous.
- **R3 NEW C3** (directory-at-path): acceptance loosened to `errors.Is(err, syscall.EISDIR) OR string contains "is a directory"` — empirically validated against `toml.DecodeFile` path on macOS.
- **R3 NEW C4** (regex edge cases): explicit accept-by-design and reject-by-design pins added at line 67. All planner-pinned cases confirmed empirically.

Three minor wording suggestions (R4-1, R4-2, R4-3) flagged but **not blocking**. The plan is implementable, the builder has unambiguous acceptance criteria, and the empirical evidence base survives independent re-verification.

The drop is ready to enter Phase 4 (build).

---

## Summary for Orchestrator

- **Verdict:** pass
- **New counterexamples:** 3 minor (R4-1 idempotence wording, R4-2 discard-target wording, R4-3 malformed-section pass-through wording) — none blocking; all are one-line clarifications the planner can fold in or accept as build-QA reviewer awareness items
- **YAGNI items raised:** 2 — both dismissed (keep `valid_quoted_names.toml` fixture; keep `map[string]any` discard target)
- **Round 3 attacks:** 4 re-verified — all 4 fully mitigated (the SHOWSTOPPER NEW C2 fix is empirically grounded)
- **Hylla feedback:** none — scratch Go program against `BurntSushi/toml v1.6.0` was the right evidence path
- **Recommendation:** plan passes. Three minor wording suggestions can be folded in pre-build at planner discretion or left for build-QA to surface as worklog comments — no blocker either way.
