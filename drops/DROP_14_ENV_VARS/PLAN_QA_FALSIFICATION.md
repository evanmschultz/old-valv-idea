# DROP_14 — Plan QA Falsification, Round 1

- Verdict: fail
- Reviewed: `main/drops/DROP_14_ENV_VARS/PLAN.md` @ `37ce350`
- Code evidence: `github.com/evanmschultz/valv@main` @ `1759e64`

## Attacks

### A1 — Acceptance criteria bypass repo build discipline
- Probe: Compare Unit 14.1-14.5 acceptance commands with `main/CLAUDE.md` Mage Discipline + Build Verification rules.
- Result: **CONFIRMED counterexample**
- Evidence:
  - `drops/DROP_14_ENV_VARS/PLAN.md:33,41,49,57,65` all require raw `go test ...`.
  - `CLAUDE.md:447-452` requires per-unit verification via `mage testPkg <pkg>` and explicitly bans raw `go test`.
  - This is not a stylistic preference; it changes the gate because `mage testPkg` also enforces the repo's format and coverage rules.
- Impact: a builder could satisfy the written DROP_14 acceptance criteria while violating the project's required verification path.

### A2 — Schema migration v1 -> v2 with an existing DB that already has bindings
- Probe: Read `Store.Bootstrap` via Hylla `node_full`, then cross-check the current sqlite migration tests.
- Result: mitigated
- Evidence:
  - `internal/adapters/sqlite/store.go:40-112` bootstraps base tables and then delegates to `migrateProjectBindings`.
  - `internal/adapters/sqlite/store.go:115-171` only rebuilds `project_bindings` when `user_version < 1`; once the DB is at v1 it exits early.
  - Existing tests already cover the "legacy DB with non-empty bindings" path and idempotence (`internal/adapters/sqlite/store_test.go:471-538`, `540-660`).
- Conclusion: there is no current counterexample against preserving existing bindings, but the v2 work should be a new `user_version` branch, not a second attempt to reuse the v0 -> v1 branch verbatim.

### A3 — `profile_id + env_key` really does preserve ownership across renames
- Probe: Trace `Profile.ID` creation, persistence, rename, and binding lookup.
- Result: mitigated
- Evidence:
  - `domain.NewProfile` generates a stable ID once (`internal/domain/model.go:77-96`).
  - `Store.CreateProfile` persists that ID (`internal/adapters/sqlite/store.go:269-281`).
  - `Store.UpdateProfileName` updates only `name`, not `id` (`internal/adapters/sqlite/store.go:366-385`).
  - Bindings already key ownership by `profile_id`, and `manage.Service` resolves bindings back through `ProfileByID` (`internal/services/manage/service.go:260-317`).
- Conclusion: the rename-stability claim is real if DROP_14 stores env rows by `profile_id`.

### A4 — Reserved runtime key list matches the current provider runtime env writers
- Probe: Search current hardcoded env-set sites in provider runtime prep and launcher entrypoints.
- Result: mitigated
- Evidence:
  - Codex runtime sets `CODEX_HOME`, `HOME`, `LOGNAME`, `TERM`, `USER`, and conditionally `CLAUDE_CONFIG_DIR` (`internal/adapters/providers/codex/runtime.go:117-132`).
  - Claude runtime sets `CLAUDE_CONFIG_DIR`, `HOME`, `LOGNAME`, `TERM`, `USER`, and conditionally `CODEX_HOME` (`internal/adapters/providers/claude/runtime.go:127-142`).
  - Image-only/auth launcher paths use the same runtime-owned keys, not a broader set (`internal/cli/codex.go:168-173`, `internal/cli/claude.go:163-168`, `internal/cli/claude_auth.go:77-83`).
- Conclusion: the reserved-key list in Unit 14.2 matches the current runtime-owned key surface.

### A5 — Env-key validation is too loose to prevent bad keys from reaching Docker args
- Probe: Compare the plan's stated validation/tests with the actual launch plumbing that consumes `ContainerRunRequest.Env`.
- Result: **CONFIRMED counterexample**
- Evidence:
  - Unit 14.2 says "validate env-key syntax" but does not define a grammar.
  - Unit 14.3 acceptance only names malformed `KEY=VALUE` and reserved-key cases (`drops/DROP_14_ENV_VARS/PLAN.md:40-49`).
  - Current Docker arg construction blindly formats each stored key as `KEY=VALUE` with no extra validation (`internal/adapters/docker/types.go:173-180`).
  - The current services also pass `prepared.Env` straight through into `ContainerRunRequest.Env` (`internal/services/codex/service.go:311-316`, `internal/services/claude/service.go:307-312`).
- Concrete counterexample:
  - If the builder implements the plan literally and only rejects `=` plus reserved keys, a stored key like `A B` or `A\tB` can still flow into `request.Env` and then into Docker `-e` args unchanged.
- Impact: the plan can "pass" while still accepting undefined/ambiguous env names. The validator contract and tests need an explicit allowed-key grammar.

### A6 — Plaintext SQLite location is under the user's home, but Valv does not itself guarantee private perms
- Probe: Trace the DB path and directory creation behavior; verify permission semantics with `go doc`.
- Result: informational
- Evidence:
  - Valv resolves the DB path to `~/Library/Application Support/valv/db/valv.sqlite3` (`internal/config/paths.go:42-52`).
  - `openStore` always ensures that path and opens the sqlite store there (`internal/cli/store.go:11-24`).
  - `Paths.Ensure` creates the directories with `0o755` before umask (`internal/config/paths.go:66-85`; `go doc os.MkdirAll`).
- Conclusion: the DB is not in `/tmp`; it lives under the user's home-library tree. But the code here does not itself prove "user-private by construction", so the threat-model note should cite the path and avoid overstating privacy guarantees.

### A7 — Provider-runtime debug logging is avoided by the planned merge point, but one adjacent leak path remains
- Probe: Search current log statements and error builders that touch env-bearing launch requests.
- Result: mitigated for the named attack, with an adjacent risk
- Evidence:
  - Both provider runtime adapters currently debug-log their prepared `env` maps (`internal/adapters/providers/codex/runtime.go:220-226`, `internal/adapters/providers/claude/runtime.go:171-177`).
  - The plan's merge point is after `PrepareRuntime`, inside the service layer, so those specific provider-runtime debug logs would not see account env values.
  - However, the Docker runner returns errors containing the joined command args on failure, and those args are built from `request.Env` (`internal/adapters/docker/executor.go:15-21`, `internal/adapters/docker/os_runner.go:47-49`, `internal/adapters/docker/types.go:173-180`).
- Conclusion: the plan's stated mitigation is correct for provider-runtime debug logs, but it does not eliminate every failure-path secret exposure.

### A8 — JSON list-key convention is only partially specified
- Probe: Compare current JSON list output shapes with Unit 14.3's "reuse conventions" wording.
- Result: informational
- Evidence:
  - Shared list rendering uses one explicit top-level key (`internal/output/output.go:82-94`).
  - Existing manage-list commands use named keys such as `accounts`, `projects`, or `accounts_by_provider` (`internal/cli/manage.go:1346-1365`, `1475`).
- Conclusion: current conventions do support a single stable top-level key, but DROP_14 never names which key `account env list` should emit. That is a review-gap, not a structural blocker.

## Verdict Summary

Plan falsification fails on two unmitigated points:

1. The acceptance criteria currently bypass the repo's mandatory `mage` verification path.
2. The env-key validation contract is under-specified; whitespace/special-character keys can still flow into Docker env args if the builder implements the plan literally.

The remaining scoped attacks either held up under source review (migration with existing bindings, rename stability, reserved runtime key list, service-layer merge point for provider-runtime debug logs) or surfaced clarifications that should be recorded but do not by themselves block the plan (DB path/privacy wording, exact JSON list key, non-debug failure-path leakage).
