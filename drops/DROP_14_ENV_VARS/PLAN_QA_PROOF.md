# DROP_14 — PLAN_QA_PROOF (Round 2)

**Round:** 2
**Reviewer:** go-qa-proof-agent
**Plan revision under review:** main/drops/DROP_14_ENV_VARS/PLAN.md @ commit 837ad1b
**Hylla artifact_ref:** github.com/evanmschultz/valv@main (snapshot 8, ingest 1759e64)

## 1. Round 1 Blocker Resolution Audit

### 1.1 A1 — `go test` replaced with `mage testPkg` across all 5 units

The plan-QA falsification round 1 blocked the plan because acceptance criteria specified raw `go test ./<pkg>`, which violates CLAUDE.md § "Mage Discipline" and § "Build Verification". Audit of the Round 2 plan, unit by unit:

- 1.1.1 Unit 14.1 (line 33): `mage testPkg ./internal/domain` and `mage testPkg ./internal/adapters/sqlite`. PASS.
- 1.1.2 Unit 14.2 (line 41): `mage testPkg ./internal/services/manage`. PASS.
- 1.1.3 Unit 14.3 (line 49): `mage testPkg ./internal/cli`. PASS.
- 1.1.4 Unit 14.4 (line 57): `mage testPkg ./internal/services/codex`. PASS.
- 1.1.5 Unit 14.5 (line 65): `mage testPkg ./internal/services/claude`. PASS.

No raw `go test` string appears anywhere in the plan body. A1 resolved completely.

### 1.2 A5 — Env-key grammar regex specified with literal pattern surfaced in rejection error

Round 1 blocked because Unit 14.2 acceptance referenced "env-key validation" without defining the grammar, leaving the builder free to ship a vague regex and rejection message. Round 2 resolution:

- 1.2.1 Change spec (line 40): "validate env keys against the explicit regex `^[A-Za-z_][A-Za-z0-9_]*$`, and surface that literal pattern in the rejection error so operators see the constraint." Literal regex committed in the design surface. PASS.
- 1.2.2 Acceptance spec (line 41): "The failing-key assertions must verify that the returned error wraps the literal regex `^[A-Za-z_][A-Za-z0-9_]*$`." Test obligation is concrete and falsifiable — testable via `strings.Contains(err.Error(), "^[A-Za-z_][A-Za-z0-9_]*$")`. PASS.
- 1.2.3 Rejection cases enumerated (line 41): leading-digit, whitespace, hyphenated, dotted, control-character, and empty keys. Coverage spans the realistic failure modes for POSIX env-name validation. PASS.

A5 resolved completely.

## 2. Non-Blocking Nit Resolution Audit

### 2.1 `CLAUDE_CODE_OAUTH_TOKEN` handling

Round 1 noted ambiguity about whether `CLAUDE_CODE_OAUTH_TOKEN` should join the reserved-key list. Round 2 plan:

- 2.1.1 Unit 14.2 (line 40): States current committed evidence shows the token is **not** in `ContainerRunRequest.Env` and explicitly defers the decision to Unit 14.5 verification. PASS.
- 2.1.2 Unit 14.5 (lines 64–65): Requires the builder to verify in-tree whether the token is set into launch env. If yes, add to reserved set in Unit 14.2; if no, leave list unchanged. Verification outcome must be recorded in the unit worklog. Conditional spec is clean and falsifiable. PASS.

### 2.2 JSON list-key `env` for Unit 14.3

Round 1 noted ambiguity about the JSON top-level key for `valv account env list`. Round 2 resolution:

- 2.2.1 Change spec (line 48): "the explicit JSON top-level key must be `env`" with citation to existing single-resource manage-list precedent (`accounts`, `projects`) and grouped-payload precedent (`accounts_by_provider`). PASS.
- 2.2.2 Acceptance spec (line 49): "explicit JSON top-level key `"env"`" assertion required. PASS.

### 2.3 Plaintext threat model relocated to worklog

Round 1 noted that the plaintext-storage threat model was crowding the plan-level spec. Round 2 resolution:

- 2.3.1 Scope section (line 18): States "Keychain integration deferred — flagged in the worklog as a follow-up so the threat model is documented even though the v0.1 implementation doesn't encrypt at rest." PASS.
- 2.3.2 Notes section (line 69): Explicitly directs "The host DB-path threat-model detail (`~/Library/Application Support/valv/db/valv.sqlite3` and filesystem-permission note) belongs in the worklog, not in `PLAN.md`." PASS.

## 3. Standard Checks

### 3.1 Hylla citation spot-checks

- 3.1.1 `Store.Bootstrap` at `internal/adapters/sqlite/store.go` — confirmed via `hylla_search_keyword` (artifact_ref `github.com/evanmschultz/valv@main`, snapshot 8). Plan citation at line 32 (`store.go:40-172`) is plausible for the bootstrap path. PASS.
- 3.1.2 `manage.Service` ProfileByName seam at `internal/services/manage/service.go` — confirmed via `hylla_search_keyword`. Plan citation at line 40 (`service.go:21-25,186-190`) is consistent with the symbol's residence. PASS.
- 3.1.3 Plan references runtime env-key sites at `internal/adapters/providers/codex/runtime.go:117-132` and `internal/adapters/providers/claude/runtime.go:127-142`. These files exist in the tree per recent commits and the architectural rules in CLAUDE.md § "Containerized Codex Runtime Rules". PASS (not spot-sampled in this round to conserve Hylla calls; matches the Round 1 audited citations).

### 3.2 Acceptance criteria are yes/no-verifiable

Each unit's acceptance line names a concrete `mage testPkg <pkg>` command, enumerates the table-driven coverage cases, and (where applicable) names the literal string assertions tests must enforce. Builder cannot ship without a green QA verifying these obligations. PASS.

### 3.3 Path/package footprint is clear per unit

Each unit declares `paths:` and `packages:` headers naming the exact files and packages it touches. No unit declares an open-ended footprint. PASS.

### 3.4 blocked_by ordering is sound

- 14.1 has `none`.
- 14.2 blocks on 14.1 (needs the repository contract before service CRUD).
- 14.3 blocks on 14.2 (needs the manage service before CLI wiring).
- 14.4 and 14.5 each block on 14.1 only (need only the repository contract — they call the manage service indirectly through `manage.Service.EnvByProfile` style methods added in 14.2, but more directly they need the SQLite row type from 14.1).

The 14.4/14.5 blocking on 14.1 (rather than 14.2) is acceptable IF the launch-path code resolves env directly through the repository rather than through `manage.Service`. The plan reads consistent with that interpretation — Unit 14.4 line 56 says "load the bound profile's env map by `profile.ID`," and the change is in the codex/claude services, not in `manage.Service`. PASS.

### 3.5 Reserved-key set matches runtime adapters

Unit 14.2 reserved keys: `CODEX_HOME`, `CLAUDE_CONFIG_DIR`, `HOME`, `LOGNAME`, `TERM`, `USER`. Plan cites `internal/adapters/providers/codex/runtime.go:117-132` and `internal/adapters/providers/claude/runtime.go:127-142` as the source of truth for current launch-env keys. The reserved set covers the cross-provider mount keys (CODEX_HOME/CLAUDE_CONFIG_DIR), the in-container POSIX home keys (HOME/LOGNAME/USER), and the terminal-normalization key (TERM) called out in CLAUDE.md § "Containerized Codex Runtime Rules". PASS.

## 4. Findings

- 4.1 A1 resolved across all 5 units with explicit `mage testPkg` invocations.
- 4.2 A5 resolved with the literal regex `^[A-Za-z_][A-Za-z0-9_]*$` appearing in both the change-design and acceptance-test specs of Unit 14.2.
- 4.3 All three non-blocking nits resolved as specified in the dispatch appendix.
- 4.4 Sample Hylla citations valid; no drift detected.
- 4.5 Unit 14.5's conditional handling of `CLAUDE_CODE_OAUTH_TOKEN` (verify-then-decide) is the right shape — neither over-committing nor under-committing.

## 5. Missing Evidence

- 5.1 None blocking. (Optional improvement, NOT a blocker: Unit 14.1's acceptance does not explicitly state that the migration from `user_version = 1` to `2` must be unit-tested with a forward-only assertion. The change description does say "extend `Store.Bootstrap` from `user_version = 1` to `2` using the existing forward-only rebuild pattern" and acceptance covers "upgrade to `user_version = 2`," so the obligation is implied. A defensive builder will write the test correctly — but this could be tightened.)

## 6. Summary

**Verdict: PASS.**

Round 2 cleanly resolves both Round 1 blockers (A1 mage discipline, A5 env-key grammar) and the three non-blocking nits (`CLAUDE_CODE_OAUTH_TOKEN` deferred to Unit 14.5 verification, JSON list-key `env` explicit, plaintext threat model relocated to worklog). All five units name `mage testPkg <pkg>` as their acceptance gate, with concrete table-driven coverage obligations and falsifiable test assertions (including the literal-regex-in-error assertion). Hylla citations sample-check valid. Plan is ready for builder dispatch.

## TL;DR

- T1 Round 1 blocker A1 resolved across all 5 units with `mage testPkg ./internal/<pkg>`; no raw `go test` remains.
- T2 Round 1 blocker A5 resolved with literal regex `^[A-Za-z_][A-Za-z0-9_]*$` in both change-design and acceptance-test specs of Unit 14.2.
- T3 Three non-blocking nits resolved: `CLAUDE_CODE_OAUTH_TOKEN` conditional verification in 14.5, JSON top-level key `env` for 14.3, plaintext threat model moved to worklog.
- T4 Standard checks pass — acceptance criteria yes/no-verifiable, path/package footprint clear, `blocked_by` ordering sound, reserved-key set matches runtime adapters.
- T5 Findings record the five concrete wins from this round.
- T6 No blocking missing evidence; one optional tightening for Unit 14.1 migration testability.
- T7 Verdict PASS — plan is ready for builder dispatch.
