# Plan QA Falsification — DROP_9_CLI_AUDIT_AND_REFACTOR

**Round:** 4
**Verdict:** **FAIL** — one surgical finding (F1: cross-unit AC contradiction between 9.1 AC #6 and 9.4.5 ownership split). All 8 prompt-supplied attack vectors evaluated; 7 REFUTED, 1 surfaces F1.

R4 applied F1 + F2 fixes from R3 (split AC #1 into #1a/#1b/#1c + added manage.go to 9.4.5 paths + clarified ownership). The regex itself catches the 4 documented bare-form hits and no false positives; the 68-count is accurate; serialization via blocker chain is sound. The remaining bug is an inherited inconsistency between 9.1's verification gate and 9.4.5's claimed scope — surfaced (not introduced) by R4's clearer ownership doc.

---

## 1. R3 refutation attempts

### 1.1 Vector 1 — AC #1c regex catches the right set (REFUTED)

`git grep -E '"manage [a-z]+|manage [a-z]+"' -- magefile.go README.md` returns exactly:

```
README.md:36:mage run "manage status"
README.md:44:mage dev:run "manage update"
README.md:45:mage dev:run "manage status"
magefile.go:682:    {Label: "bootstrap", Value: `mage dev:run "manage update"`, Identifier: true},
```

4 hits, matching exactly what the planner documented in PLAN.md line 223. Broader scan (`git grep "manage" -- README.md magefile.go`) confirms there are no prose / unquoted / backticked `manage X` variants in scope (the only other matches are `managed containers` / `managed home directory` prose — correctly NOT caught). No false-positives, no misses.

### 1.2 Vector 2 — 68-count accurate, gate path works (REFUTED for the count itself; see F1 for the gate)

`git grep -c "valv manage" -- internal/cli/manage.go` returns **68** today. After 9.1 deletes `newManageCommand` (lines 22-57, ~7 hits inside its Example block) and fixes the 3 fmt.Errorf strings at 626/962/996 (~4 hits), the residue lands in the surviving constructor Example blocks (the 16 `Example:` literal sites at lines 34/70/112/137/161/202/237/279/300/321/347/383/407/435/1179/1232/1341), which is what 9.4.5 sweeps. The arithmetic is consistent with the 9.4.5 design note's "~16 account-* and image-* constructors". The count itself is fine — but see F1 for the ownership gate problem.

### 1.3 Vector 3 — 9.1 / 9.4.5 path overlap on manage.go is serialized safely (REFUTED)

Both 9.1 and 9.4.5 touch `internal/cli/manage.go`. PLAN.md line 237 declares 9.4.5 `Blocked by: 9.1, 9.2, 9.3, 9.4`. WORKFLOW.md Phase 4 picks `state: todo` units only when `blocked_by` is empty or all `done`, so 9.4.5 cannot start until 9.1's commits land. No race.

### 1.4 Vector 4 — AC numbering change does not break references (REFUTED)

`git grep -nE "AC #[0-9]|criterion #[0-9]|acceptance criterion [0-9]|AC[0-9]" -- drops/DROP_9_CLI_AUDIT_AND_REFACTOR/PLAN.md` returns only `AC #1a`, `AC #1b`, `AC #1c` references — all inside 9.4.5 itself. No stale `AC #7` or numeric AC refs anywhere else in PLAN.md.

### 1.5 Vector 5 — no combined `mage plan-check` provided, but design note documents the 3-grep workflow (REFUTED)

Planner left 3 separate greps; design note at PLAN.md line 245 explicitly says "run AC #1a, #1b, #1c greps individually, address every hit, rerun all three to confirm zero, then `mage testPkg ./internal/cli`". Mechanical and documented. Not a counterexample, just minor builder friction.

### 1.6 Vector 6 — AC #1c regex alternatives are NOT identical (REFUTED)

The regex `"manage [a-z]+|manage [a-z]+"` has two alternatives with different anchoring:

- Alt 1: `"manage [a-z]+` — leading literal `"`, then `manage `, then `[a-z]+`. **No trailing quote required.**
- Alt 2: `manage [a-z]+"` — `manage `, then `[a-z]+`, then trailing literal `"`. **No leading quote required.**

They share text but anchor differently — alt 1 catches anything with a leading quote (handy when the trailing quote is far away from `manage`, e.g., `"manage status --flag foo"`), alt 2 catches anything with a trailing quote (e.g., a literal cut off at `manage update"` after concatenation). Both alternatives independently match the actual 4 hits (which have BOTH quotes adjacent), so the redundancy is harmless. Not a typo.

### 1.7 Vector 7 — README.md double-edit risk REFUTED via DROP_10 sequencing

DROP_10 (`main/PLAN.md` line 33) is `state: todo, blocked_by: DROP_9` and explicitly scopes "rewrite `README.md` around 'two providers, pass-through only' (and the post-DROP_9 normalized command tree)". DROP_9 closes before DROP_10 starts. 9.4.5's minimal README.md substitution (3 lines: 36, 44, 45) lands first, DROP_10's full rewrite supersedes it. No conflict.

### 1.8 Vector 8 — deleted-constructor Examples already accounted for (REFUTED for arithmetic; surfaces F1)

`newManageCommand`'s Example block (manage.go lines 34-42, 7 `valv manage` hits) is deleted by 9.1 when the constructor is deleted. `newManageProjectCommand`'s Example block goes with the constructor in 9.4 (PLAN.md line 173 says "`newManageProjectCommand` and `runManageProjectList` are deleted"). The 9.4.5 design note correctly identifies the residue ("the ~16 account-* and image-* constructors whose Example blocks still reference the old `valv manage ...` form").

The 68 = (deleted-with-newManageCommand ~7) + (deleted-with-newManageProjectCommand ~2) + (3 fmt.Errorf at 626/962/996) + (surviving-constructor Example blocks ~56). The arithmetic checks out within roughly ±5 (depends on exact Example-block per-constructor hit counts).

But this cross-examination surfaced **F1** below.

---

## 2. Counterexamples (CONFIRMED)

### 2.1 F1 — 9.1 AC #6 verification is stronger than 9.1's claimed scope per 9.4.5 design

**The contradiction:**

PLAN.md line 61 (Unit 9.1, AC #6):
> 6. All `fmt.Errorf` and other user-facing error/help strings in `manage.go` (and any helpers moved out of it) reference the new normalized command tree. Concrete examples: ... **Verify via `git grep "valv manage" -- internal/cli/manage.go` returning zero hits after this unit lands.**

PLAN.md line 243 (Unit 9.4.5 design note):
> Ownership split with unit 9.1: unit 9.1 owns (a) structural deletion of `newManageCommand` + redistribution of children, (b) the 3 `fmt.Errorf` runtime strings at manage.go:626/962/996, and (c) root.go examples block. **Unit 9.4.5 owns the sweep of cobra `Example:` string literals in ALL surviving manage.go constructors.**

These are inconsistent. If 9.4.5 owns the Example sweep, then immediately after 9.1's commits land:

```
git grep "valv manage" -- internal/cli/manage.go
```

will STILL return ~50+ hits (the surviving constructor Example blocks at lines 70/112/137/161/202/237/279/300/321/347/383/407/435/1179/1232/1341).

The cobra `Example:` field is rendered by `--help` and is unambiguously a "user-facing help string" — so 9.1 AC #6 (which says "all fmt.Errorf and other user-facing error/help strings") arguably DOES include Example blocks. The verification clause is unambiguous: zero hits. The two statements cannot both be true.

**Why this matters:**

A builder picking up 9.1 will either:
- Read AC #6 strictly → also rewrite all surviving Example blocks → 9.4.5 has nothing left to do (its scope is empty / dead).
- Read 9.4.5 design strictly → leave Example blocks alone → 9.1 AC #6 verification FAILS.

Either way, the QA loop will catch it, but the planner should resolve the ambiguity up front so the loop doesn't have to.

**Repro / evidence:**

`git grep -c "valv manage" -- internal/cli/manage.go` returns 68 today. The 3 fmt.Errorf strings are at lines 626, 962, 996. The `newManageCommand` Example block at lines 34-42 has 7 `valv manage` hits. 68 − (3 + 7 + ~2 from `newManageProjectCommand` deletion) ≈ 56 residue. The verification check at 9.1 AC #6 will see those 56 residue hits and fail.

**Surgical remediation (planner pick one):**

Option A — Tighten 9.1 AC #6 to exclude Example blocks:
> Verify via `git grep "valv manage" -- internal/cli/manage.go` returns zero hits **outside cobra `Example:` string literals** (the Example-block sweep is owned by unit 9.4.5).

Option B — Move all Example-block ownership to 9.1 (kill the split):
> 9.1 owns Example-block sweep in manage.go. 9.4.5 then only sweeps the other 9 cli helper files + magefile.go + README.md.

Option C — Reorder so 9.4.5 runs concurrently with 9.1 on disjoint manage.go regions:
> Not recommended (re-introduces path-overlap-without-blocker risk).

Option A is the cheapest and matches the planner's apparent intent (9.4.5 design note line 243 reads as canonical). Recommend Option A.

---

## 3. Summary

**Verdict: FAIL** — one surgical finding (F1). R4 cleanly resolved R3's two findings; the remaining bug is an inherited inconsistency between 9.1 AC #6's verification gate and 9.4.5's claimed scope, exposed by R4's clearer ownership documentation. The planner needs one of:

- F1: pick Option A (tighten 9.1 AC #6 to exclude Example blocks) — recommended, single-line edit.

All 8 prompt-supplied attack vectors: 7 REFUTED with evidence (V1 regex behavior, V2 count accuracy, V3 serialization, V4 numbering, V5 workflow doc, V6 regex structure, V7 README sequencing), 1 (V8) refuted for arithmetic but surfaced F1.
