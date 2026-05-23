verdict: fail

# Plan QA Falsification — Round 4

## Counterexamples

### F4.1 Legacy first-open migration is still a live path, but the new concurrency gate only covers `v1 -> v2`

- Plan claim under attack: Unit 14.1 closes the `SQLITE_BUSY` first-open gap by adding `_pragma=busy_timeout(5000)` and proving a two-connection `v1 -> v2` upgrade succeeds without `SQLITE_BUSY` (`drops/DROP_14_ENV_VARS/PLAN.md:29`, `drops/DROP_14_ENV_VARS/PLAN.md:39-46`).
- Repo evidence: the committed store still carries a legacy `user_version = 0` migration path and explicitly tests it. `Store.Bootstrap` only early-exits when `user_version >= 1`, so `0` still takes the older migration branch (`internal/adapters/sqlite/store.go:95-153`). The existing tests seed `user_version = 0` and verify migration/idempotence (`internal/adapters/sqlite/store_test.go:504-523`, `internal/adapters/sqlite/store_test.go:540-668`).
- Counterexample: a real older install can first-open a legacy `user_version = 0` database, which means the concurrent race is not limited to the new `v1 -> v2` path. Round 4 can still pass even if `v0 -> v2` is the path that returns `SQLITE_BUSY`, because the required concurrency acceptance never exercises it.
- Narrow fix: extend Unit 14.1 acceptance to require the same two-connection "one waits, both succeed" proof for a seeded legacy `user_version = 0` database upgrading all the way to `2`, or explicitly drop `v0` support in the same unit and delete the legacy migration path.

### F4.2 The new "_no duplicate pragma" claim is false for URI callers that already set `busy_timeout`

- Plan claim under attack: Unit 14.1 will add both pragmas "without duplicating either pragma when already present" (`drops/DROP_14_ENV_VARS/PLAN.md:45`).
- Repo evidence: the current helper only deduplicates exact string matches, not pragma names (`internal/adapters/sqlite/open.go:53-70`).
- Library evidence: `modernc.org/sqlite` explicitly allows `_pragma` to appear more than once (`driver.go:45-52`), then executes every `_pragma` value in order (`sqlite.go:142-165`).
- Counterexample: `Open(OpenOptions{URI: "file:/tmp/valv.db?_pragma=busy_timeout(30000)"})` still receives an added `_pragma=busy_timeout(5000)` because `busy_timeout(30000) != busy_timeout(5000)`. The driver accepts both values and executes both pragmas, so the Round 4 dedup acceptance is falsified for an existing supported caller shape.
- Narrow fix: normalize `_pragma` entries by pragma key, not exact string, and define precedence. The safest plan-level rule is "if any existing `_pragma` starts with `busy_timeout`, do not append another one."

### F4.3 Env-list ordering is unspecified, so human and JSON output can still drift

- Plan claim under attack: Unit 14.1/14.3 is sufficiently specified for `set/list/unset` and list output (`drops/DROP_14_ENV_VARS/PLAN.md:39-46`, `drops/DROP_14_ENV_VARS/PLAN.md:69-77`).
- Repo evidence: existing list surfaces pin deterministic ordering in the store layer; `ListProfilesByProvider` uses `ORDER BY name ASC` (`internal/adapters/sqlite/store.go:333-338`).
- Counterexample: if account env rows are listed without `ORDER BY env_key ASC`, two keys inserted in different orders can render in different orders depending on row layout. Human users lose alphabetical output, and JSON snapshots become order-sensitive without a plan requirement catching it.
- Narrow fix: require the store-layer env list query to sort by `env_key ASC`, then add sqlite and CLI assertions that the returned/rendered order is deterministic.

### F4.4 `--format plain` can regress and still satisfy the current acceptance

- Plan claim under attack: Unit 14.3 fully specifies CLI output behavior for the new command (`drops/DROP_14_ENV_VARS/PLAN.md:68-77`).
- Repo evidence: `--format plain` is a root-level persistent flag (`internal/cli/root.go:114-116`), and the shared output package has a distinct plain-mode branch for lists (`internal/output/output.go:82-118`). Existing CLI tests already pin command-owned output keys and output-mode behavior (`internal/cli/extended_test.go:136-153`).
- Counterexample: Unit 14.3 intentionally introduces a custom env-list formatter instead of reusing `output.WriteListWithKey` for JSON, but its acceptance only mentions human and JSON. A builder can implement human + JSON, forget plain mode entirely, and still satisfy the Round 4 test list while `valv account env list <name> --format plain` errors or emits inconsistent ad hoc text.
- Narrow fix: add explicit plain-mode acceptance for `list` with and without `--reveal`, or explicitly declare plain unsupported for this command and require a stable rejection path.

## YAGNI Pressure Check

- The shared `internal/services/run` seam in Unit 14.4 is justified by the accepted DROP_13 architecture and serves three launch surfaces (`valv run`, `valv codex`, `valv claude`), so the abstraction itself is not premature.
- The plan still keeps scope tight: no keychain work, no extra provider forks, no new config model. YAGNI is tolerable.

## Hidden Dependency Check

- The DROP_13 dependency is explicit in the plan (`drops/DROP_14_ENV_VARS/PLAN.md:20`, `drops/DROP_14_ENV_VARS/PLAN.md:30`, `drops/DROP_14_ENV_VARS/PLAN.md:81-90`, `drops/DROP_14_ENV_VARS/PLAN.md:97-98`).
- Two hidden dependencies are not explicit enough yet:
  - the still-supported legacy `user_version = 0` migration path;
  - the repo-wide `plain` output contract exposed by the root command.
- Verdict remains `fail` until those dependencies are either covered in acceptance or intentionally removed from scope.
