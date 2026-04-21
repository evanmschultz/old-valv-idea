# DROP_3 — Builder QA Falsification

Append a `## Unit N.M — Round K` section per QA attempt. See `main/drops/WORKFLOW.md` § "Phase 5 — Build QA (per unit)" for what each section should contain.

## Unit 3.1 — Round 1

- **QA agent:** go-qa-falsification-agent
- **Commit under review:** `2ad62a4 feat(drop-3): thread provider arg through BindingByProjectID`
- **Verdict:** pass (with 1 ADVISORY)

### Attacks attempted

1. **Missed `BindingByProjectID` call sites — REFUTED.** `grep -rn 'BindingByProjectID' internal/` returns exactly 16 lines across 10 files (2 are duplicate `t.Fatalf` error-message references, 2 are interface/method declarations at `internal/domain/repository.go:22` and `internal/services/codex/service_test.go:76`, 1 is a `ProfileID` compare log line). Every function-call invocation — `internal/services/codex/service.go:216`, `internal/services/manage/service.go:240`, `internal/adapters/sqlite/store_test.go:82`, `:178`, `internal/services/manage/service_test.go:411`, `internal/cli/codex_setup_test.go:68`, `internal/cli/manage_test.go:79` — passes `domain.ProviderCodex` as the third argument. The interface decl in `domain/repository.go:22` carries the new `Provider` type; the method decl in `codex/service_test.go:76` carries the new `domain.Provider` parameter. Grep over `cmd/` returns zero hits — no CLI-entry call sites. No missed caller.

2. **Bare-string-cast escape via `domain.Provider("..."`) — REFUTED (no new violations).** `grep -F 'domain.Provider("' internal/` returns exactly one hit: `internal/services/globalswitch/service_test.go:100` (`domain.Provider("claude")` inside `TestSwitchRejectsUnsupportedProvider`). Pre-existing; the orchestrator has already deferred this as out-of-scope per the builder worklog Unknowns and the spawn prompt attack-7. No new bare-string casts introduced by Unit 3.1. The other `domain.Provider(...)` conversions that `grep -n 'domain\.Provider\b'` turns up — `internal/adapters/sqlite/store.go:202,227,255,347,382,496,554` — are `domain.Provider(providerValue)` where `providerValue` is a `string` variable scanned from a DB column inside `Row.Scan`; these are legitimate deserialization conversions, not ad-hoc literal casts at call sites, and match the rule's stated intent ("this bars ad-hoc string-literal casts at call sites"). Unit 3.1 acceptance met on substance. See ADVISORY A-1 below for the literal-reading ambiguity.

3. **Zero-value `domain.Provider` variable passed to `BindingByProjectID` — REFUTED.** `grep -nE 'var\s+\w+\s+domain\.Provider\b' internal/` returns zero hits. Every one of the 7 call-site invocations passes the named constant `domain.ProviderCodex` (verified via attack 1 grep). No declared-but-unassigned variable sneaks in via a trailing arg.

4. **SELECT `WHERE project_id = ? AND provider = ?` arg-order bug — REFUTED.** `internal/adapters/sqlite/store.go:333-336`: placeholders appear left-to-right as `project_id = ?` then `AND provider = ?`; positional args passed are `projectID` (line 334) then `string(provider)` (line 335). Order matches. `modernc.org/sqlite` positional `?` binding is 1-based left-to-right per `database/sql` contract. The scan-target order at line 341 (`&binding.ProjectID, &binding.ProfileID, &providerValue, &createdAt, &modifiedAt`) matches the SELECT column order — independent check, also correct.

5. **Scope creep into Unit 3.2 territory — REFUTED.** `git diff 2ad62a4^..2ad62a4 -- internal/adapters/sqlite/store.go` shows exactly one hunk at `@@ -327,11 +327,12 @@`: (a) `BindingByProjectID` signature, (b) SELECT string, (c) added `string(provider)` bind arg. `Bootstrap` (`store.go:40-111`), `UpsertProjectBinding` (`store.go:310-328`), and the `project_bindings` DDL inside the `Bootstrap` statements slice (`store.go:57-65`) are **not** touched. Unit 3.2 territory clean.

6. **Test coverage of the new `AND provider = ?` filter — REFUTED (no false claim).** Builder worklog does not claim provider-mismatch-rejection test coverage; PLAN.md Unit 3.1 acceptance also does not require it (explicitly Unit 3.2 scope per `TestStoreCompositeBindingsCoexistByProvider`). `TestStoreProfileAndBindingLifecycle` and `TestStoreNotFoundErrors` exercise the happy path (matching provider) and the not-found path (with matching provider) — both pass `domain.ProviderCodex`. No test falsely claims provider-mismatch-rejection coverage. Intent matches behavior.

7. **Interface-satisfaction regression (`*sqlite.Store` vs `domain.BindingRepository`, `services/codex.Store`, `services/manage.Store`) — REFUTED.** `domain.BindingRepository` at `internal/domain/repository.go:22` is `BindingByProjectID(context.Context, string, Provider) (ProjectBinding, error)`. `*Store.BindingByProjectID` at `internal/adapters/sqlite/store.go:330` is `(ctx context.Context, projectID string, provider domain.Provider) (domain.ProjectBinding, error)` — matches (named vs unnamed parameters identical at the type level). `services/codex.Store` (line 23-27) and `services/manage.Store` (line 20-24) both embed `domain.BindingRepository`, so the contract cascades. Compile-time proof: `mage testPkg ./internal/adapters/sqlite` green (16 tests); `mage testPkg ./internal/services/codex` green (11 tests, production `s.store.BindingByProjectID` call at `service.go:216` compiles against the real store); `mage testPkg ./internal/services/manage` green (23 tests, same at `service.go:240`).

8. **PLAN.md line 61 ambiguity creates future blind spot — ADVISORY (not a blocker).** The acceptance rule reads: ``grep -F 'domain.Provider("' internal/ returns zero hits outside of the `domain` package's own tests``. The pre-existing hit at `internal/services/globalswitch/service_test.go:100` is outside `domain`, so the rule as literally written is **violated** on `main` after this commit — not satisfied. The orchestrator has accepted this as "pre-existing, deferred" per the builder worklog Unknown. Risk: future grep runs that scope the rule literally will keep hitting this one, and a real new violation in a later drop blends into the noise. Remediation options: (a) amend PLAN.md acceptance to scope the grep to the Unit 3.1 9-path list; (b) tiny follow-up unit replacing `domain.Provider("claude")` with `domain.ProviderClaude` (both constants exist at `internal/domain/types.go:11-12`); (c) add an explicit "known pre-existing" carve-out in PLAN.md wording. Recommended: (b) — smallest diff, eliminates the ambiguity at the source. Not a blocker because the hit is pre-existing and the builder correctly surfaced it.

9. **Defense-in-depth `binding.Provider != domain.ProviderCodex` check at `services/codex/service.go:223` — REFUTED (preserved as planned).** PLAN.md Unit 3.1 paths note: "the existing `binding.Provider != domain.ProviderCodex` check at line 223 stays as-is". Confirmed unchanged — the `git show 2ad62a4 -- internal/services/codex/service.go` diff touches only line 216. With the new `AND provider = ?` filter the post-fetch check is redundant-but-harmless; matches PLAN intent.

10. **Compilation-only false pass (tests thread the arg but don't exercise semantics) — REFUTED for Unit 3.1 acceptance.** Unit 3.1's acceptance bar is signature-threading + call-site correctness + grep-rule compliance, NOT provider-mismatch-rejection coverage (that is Unit 3.2). The 4 named sqlite tests all still pass — PLAN.md-specified bar met.

### Blockers

None.

### Advisories

- **A-1:** PLAN.md line 61 grep-rule ambiguity vs pre-existing `internal/services/globalswitch/service_test.go:100` bare-string cast (`domain.Provider("claude")`). See attack 8. Recommend a 1-line follow-up unit that replaces the literal with `domain.ProviderClaude` before DROP_3 closes, so the Unit 3.1-acceptance invariant actually holds on `main`. Not a blocker for Unit 3.1 — pre-existing, explicitly deferred by the orchestrator.

### Evidence summary

- **Premises:** Signature threading is atomic; every call site passes a named `domain.Provider` constant; no bare-string casts or zero-value vars; SELECT arg order correct; Unit 3.2 territory untouched; interface satisfaction holds; all existing named tests still green.
- **Evidence:** `git show 2ad62a4` (11 files, +42/-18); `grep -rn 'BindingByProjectID' internal/ cmd/` (16 lines internal, 0 in cmd); `grep -F 'domain.Provider("' internal/` (1 pre-existing hit, no new); `grep -nE 'var\s+\w+\s+domain\.Provider\b' internal/` (0 hits); `Read` on `internal/adapters/sqlite/store.go:40-111,310-359` (Bootstrap/Upsert/Select bodies); `mage testPkg` on all 6 affected packages (sqlite 16/16 79.2%, codex 11/11 75.2%, manage 23/23 76.4%, cli 101/101 72.0%, domain 26/26 85.2%, globalswitch 8/8 80.8%).
- **Trace or cases:** 10 attacks — 9 REFUTED, 1 ADVISORY (attack 8, A-1), 0 CONFIRMED.
- **Conclusion:** pass.
- **Unknowns:** none. A-1 routed as an advisory.

## Hylla Feedback

N/A — task touched only Go files whose committed state was fully visible via `Read` / `Grep` on the 10 scoped files plus `git show`. No Hylla queries attempted, no fallbacks needed for this review's purpose (signature-threading + grep-rule compliance + scope-creep check are all directly answerable from `git show` + `grep`). Hylla MCP is more useful for call-site blast-radius questions than for single-commit diff review.
