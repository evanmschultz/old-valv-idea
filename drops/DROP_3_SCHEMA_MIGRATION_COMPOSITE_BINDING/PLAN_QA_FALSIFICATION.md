# DROP_3 Plan-QA Falsification — Round 2

**Target:** `drops/DROP_3_SCHEMA_MIGRATION_COMPOSITE_BINDING/PLAN.md` (revision commit `78fcb7f`)
**Round:** 2
**Reviewer role:** plan-QA falsification
**Verdict:** pass

## Headline

Revised plan holds under Round 2 attack across all nine falsification surfaces. F1 concurrent-Bootstrap race is fully mitigated by the compound `user_version` + `sqlite_master` shape-probe guards. F4 legacy-test column enumeration is complete against `store.go:57-65`. F6 package-lock + unit ordering survives. F7 re-attacks on Round 1 REFUTED surfaces produce no new counterexamples. F8 multi-tx crash-recovery gap is benign given both guards plus single-writer CLI invariant. Two minor advisories remain (F2, F5) that do not block build kickoff.

## 1. Attack Results

### 1.1 F1 — Concurrent-Bootstrap race fix robustness

- **Attack:** Under `modernc.org/sqlite` shared-cache mode, two concurrent Bootstraps could both read `user_version=0` during the DEFERRED transaction's SHARED-lock phase, then race on the EXCLUSIVE upgrade. The revised plan's "PRAGMA user_version first in BeginTx" claim assumes transaction-level serialization prevents double-rebuild.
- **Evidence:** Revised plan Unit 3.2 path-edit step (5) adds `sqlite_master` shape-probe BEFORE running the rebuild — checks for absence of `PRIMARY KEY (project_id, provider)`. This gives a second, shape-based gate independent of `user_version`.
- **Trace:** If process A commits `user_version=1` + rebuild first, process B's second-read-then-probe sees composite-shape tables and skips rebuild (step 7). If the two Bootstraps truly interleave mid-rebuild, SQLite's per-database write lock serializes them — one gets BUSY and retries. On retry, the probe + user_version check both see the rebuilt state and short-circuit.
- **Conclusion:** REFUTED. The compound user_version + shape-probe guard is idempotent-safe even under pathological concurrent Bootstrap.
- **Unknowns:** None blocking.

### 1.2 F2 — Acceptance grep loophole via zero-value variable

- **Attack:** Can a builder pass a broken empty-string argument while evading both grep patterns `domain.Provider("")` and `domain.Provider("`? Counterexample: `var p domain.Provider` (zero-value declaration) followed by `BindingByProjectID(ctx, id, p)`. Neither grep catches this.
- **Conclusion:** REFUTED as a hard break, ADVISORY as a tightening opportunity. Line 60's prose requirement catches it on interpretive review. Recommend appending "zero-value variables of type `domain.Provider` are not permitted" to line 60 for explicitness.
- **Unknowns:** None blocking.

### 1.3 F3 — Idempotency test blind spot

- **Attack:** Can a buggy implementation satisfy all three observable post-conditions (staging-table absent, values unchanged, COUNT=1) while still having a broken `user_version` guard?
- **Conclusion:** REFUTED at blocker level, ADVISORY for test strengthening. The end-state property is what the dev actually cares about for MVP. Optional improvement: add `PRAGMA user_version == 1` assertion before AND after Bootstrap #2.
- **Unknowns:** None blocking.

### 1.4 F4 — Legacy-test setup column completeness

- **Evidence:** Cross-checked revised PLAN.md line 76 against `internal/adapters/sqlite/store.go:43-65`. Zero CHECK constraints on any of the three tables. All NOT NULLs accounted for.
- **Conclusion:** REFUTED. Plan is complete.

### 1.5 F5 — sqlite_master shape-probe robustness

- **Attack:** SQLite's `sqlite_master.sql` stores CREATE TABLE strings verbatim. A naive string-match probe could miss variants: `PRIMARY KEY(project_id, provider)` (no space), `PRIMARY KEY (provider, project_id)` (swapped order), mixed-case, comment noise.
- **Conclusion:** CONFIRMED soft gap — ADVISORY, not blocker. Recommend the plan specify the probe implementation as `pragma_table_info('project_bindings')` checking `pk` column values (a row with `pk=1` on `project_id` alone → legacy; rows with `pk>0` on both → composite). Option (a) is more robust and SQLite-idiomatic.
- **Unknowns:** None blocking.

### 1.6 F6 — Unit ordering / package-lock

- **Evidence:** Unit 3.1 touches five packages; Unit 3.2 touches ONLY `internal/adapters/sqlite`. `blocked_by: 3.1` declared.
- **Conclusion:** REFUTED. Package-lock rule holds.

### 1.7 F7 — Re-attack Round 1 REFUTED surfaces

- **Call-site coverage (10th caller):** Re-ran `\.BindingByProjectID\(` grep across `internal/`. Seven call sites found + fake. No missed caller.
- **Interface blast radius:** No additional `type Store interface { ... BindingByProjectID(...) }` declaration exists outside `internal/domain/repository.go:22`.
- **Forward-only vs destructive recreate:** Plan chose forward-only with `user_version=1`; aligned with §6.2a line 221 + dev MVP directive.
- **Conclusion:** All REFUTED stays.

### 1.8 F8 — Multi-tx crash-recovery gap

- **Attack:** Between DDL-loop tx #1 commit and migration tx #2 start, if the process crashes, `user_version` is still 0 but tables may exist. Does a restart see a broken state?
- **Trace:** Restart after crash-between-tx: tx #1 no-ops (IF NOT EXISTS). tx #2 reads `user_version=0`, probes `sqlite_master`, branches correctly based on observed shape. Either path correct.
- **Conclusion:** REFUTED. Dual-guard (`user_version` + shape probe) combined with Valv's single-writer CLI invariant makes the two-tx window benign.

### 1.9 F9 — Round 1 blind spots

- **`UpsertProjectBinding ON CONFLICT(project_id, provider)` breaking existing Codex flow:** No break — re-binding same `(project_id, provider)` still triggers ON CONFLICT → update. Only behavior delta: different provider now INSERTs a second row (stated intent). REFUTED.
- **DROP_2 stub interference:** No DROP_2 artifact touches `BindingByProjectID`, `UpsertProjectBinding`, or `project_bindings` schema. REFUTED.
- **New minor concern — acceptance language:** Unit 3.1 line 60's grep specification says "every call site's trailing argument" — the grep pattern will also match interface declaration + fake method signature as non-call-site matches. Builder should interpret "call sites" literally (invocation expressions). Minor advisory.

## 2. Advisories (Non-Blocking)

- **F2 tightening:** Append to PLAN.md line 60: "Zero-value variables of type `domain.Provider` (e.g. `var p domain.Provider`) are not permitted as the trailing argument."
- **F5 probe implementation:** Specify in PLAN.md Unit 3.2 step (5) the preferred probe implementation — `pragma_table_info('project_bindings')` inspecting `pk` column values. More robust and SQLite-idiomatic.
- **F3 test strengthening (optional):** Consider adding a `PRAGMA user_version == 1` assertion before AND after Bootstrap #2 in `TestStoreBootstrapIsIdempotentAfterMigration`.
- **F9 acceptance language:** Clarify PLAN.md line 60 to distinguish "call sites" (invocation expressions) from grep matches that include interface declarations or fake method signatures.

## 3. Summary

All nine attack surfaces addressed. Zero hard counterexamples that block build. Two soft-advisory findings (F2, F5) on acceptance-language and probe-implementation precision. Verdict: **pass**. Planner may choose to apply advisories in-place (low cost, no round 3 needed) or accept them as builder judgment calls.

## TL;DR

- T1 All nine falsification surfaces attacked; F1/F4/F6/F7/F8 cleanly REFUTED; F2/F3/F5/F9 yielded soft advisories only. No blocker-class counterexample produced. Verdict: **pass** with four non-blocking advisories the planner may optionally apply before build kickoff.
