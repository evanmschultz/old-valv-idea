# DROP_14 — Plan-QA Proof (Round 1)

**Verdict:** PASS

**Reviewer:** ta-go-qa-proof (orchestrator-direct write — Agent reported Write tool denied; orchestrator applied returned content)
**Plan file:** main/drops/DROP_14_ENV_VARS/PLAN.md @ commit 37ce350
**Hylla artifact_ref:** github.com/evanmschultz/valv@main (snapshot 8, pinned to 1759e64)
**Scope:** 8 proof checks (paths concrete, acceptance testable, blocked_by sound, Hylla citations verified, 1-3 files per unit, scope-text covered, DROP_13 contingency handled, reserved-key list justified).

## 1. Per-Unit Paths Concrete

- 1.1 Unit 14.1 — `internal/domain/repository.go` (Hylla `ProfileRepository` confirms); `internal/adapters/sqlite/store.go:40` (`Store.Bootstrap`); `internal/adapters/sqlite/store_test.go:14` (`newBootstrappedStore`).
- 1.2 Unit 14.2 — `internal/services/manage/service.go` (Hylla `Service.ProfileByName`); `service_test.go` exists.
- 1.3 Unit 14.3 — `internal/cli/manage.go:22` (`newManageAccountCommand`); `manage_test.go` + `extended_test.go` (12 call sites).
- 1.4 Unit 14.4 — `internal/services/codex/service.go:155-178, 302` (cross-provider lookup + `buildRequest`).
- 1.5 Unit 14.5 — `internal/services/claude/service.go:162-187, 298` (mirror).

Note: header line 5 lists "or new `internal/services/accountenv/`" as a possibility; Planner section commits to extending `manage` instead. Internally consistent.

## 2. Acceptance Criteria Testable

All five units' acceptance reduces to `go test ./<pkg>` plus enumerated table rows. `executor.got.Env` pattern (14.4 + 14.5) matches `service_test.go:100` fakeExecutor convention.

## 3. blocked_by Sound

- 3.1 Within DROP_14: 14.1→none, 14.2→14.1, 14.3→14.2, 14.4→14.1, 14.5→14.1.
- 3.2 14.4/14.5 independent of 14.2/14.3 — direct repository read by `profile.ID`, not via manage.Service. Parallel-eligible.
- 3.3 Cross-drop: `Blocked by: DROP_13 (todo)` matches `main/PLAN.md`.

## 4. Hylla Evidence Cited Correctly

- 4.1 `ContainerRunRequest.Env` — Hylla hit; `internal/adapters/docker/types.go:43, 47`.
- 4.2 `ProfileByName` — Hylla returned both store + service summaries.
- 4.3 `Store.Bootstrap` + `user_version` — `store.go:40, 127, 165` confirm forward-only `migrateProjectBindings` pattern at 115-172.
- 4.4 `newManageAccountCommand` — Hylla keyword miss; `git grep` confirms `manage.go:22` + 12 test sites. **Hylla feedback for builder worklog later.**

## 5. Footprint 1-3 Files Per Unit — PASS for all 5 units.

## 6. Scope-Text Coverage

- 6.1 Persistence + cross-account same-key → Unit 14.1.
- 6.2 CLI surface → Unit 14.3.
- 6.3 `ContainerRunRequest.Env` threading → Units 14.4 + 14.5.

Plaintext + keychain-deferred captured in Notes 67-69.

## 7. DROP_13 Launch-Ownership Contingency

DROP_13 Units 13.1, 13.3, 13.4 move launch into `internal/services/run` and re-derive `claude` + `codex` services as thin adapters. DROP_14 Notes explicitly re-homes 14.4 + 14.5 if needed; 14.1-14.3 stay put. Sensible scoping — env-merge call site exists in either layout.

## 8. Reserved-Key List Justified

- 8.1 `HOME`, `LOGNAME`, `TERM`, `USER` — both provider runtimes (`claude/runtime.go:128-132`, `codex/runtime.go:118-122`), CLI launch (`claude.go:165-168`, `codex.go:170-173`), auth (`claude_auth.go:79-83`).
- 8.2 `CLAUDE_CONFIG_DIR` — `claude/runtime.go:128` unconditional, `codex/runtime.go:131` cross-provider, image Dockerfile `images/service.go:1031`.
- 8.3 `CODEX_HOME` — `codex/runtime.go:118` unconditional, `claude/runtime.go:141` cross-provider, host auth `account_auth.go:171`, image Dockerfile `images/service.go:1032`.

No 7th Valv-owned runtime key found in literal sweep.

## 9. Conclusion

**Verdict: PASS.** No proof-side blocker.

## 10. Unknowns (Routed)

- 10.1 `CLAUDE_CODE_OAUTH_TOKEN` injection scope unverified — DROP_7 lands a host-side OAuth token. Whether it injects into `ContainerRunRequest.Env` (would warrant reserved-key entry #7) or only into host subprocess env was not traced exhaustively. **Routed: builder to `git grep CLAUDE_CODE_OAUTH_TOKEN` before writing the env-merge code in unit 14.5.** Not plan-blocking.
- 10.2 Plaintext threat model — accepted per plan Notes 67-69; keychain integration deferred.
- 10.3 `modernc.org/sqlite` Context7 lookup not performed — accepted. Existing `migrateProjectBindings` precedent (`store.go:115-172`) already proves the pattern works under Valv's driver.

## TL;DR

- T1 Paths concrete + source-verified.
- T2 Acceptance binary-verifiable.
- T3 blocked_by DAG sound; cross-drop edge matches main/PLAN.md.
- T4 Hylla citations verified (1 keyword miss confirmed via git grep).
- T5 Footprint 1-3 files per unit.
- T6 Scope text fully covered.
- T7 DROP_13 contingency correctly scoped to 14.4 + 14.5 only.
- T8 Reserved-key list (6 keys) traced to set-sites.
- T9 PASS verdict.
- T10 Three Unknowns routed; none plan-stopping.
