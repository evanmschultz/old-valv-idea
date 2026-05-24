verdict: pass

# Plan QA Proof — Round 5

## Scope

Round 5 verifies the Round 4 hybrid response (planner-fixed F4.1 via Option B; orch-direct-fixed F4.2/F4.3/F4.4) against the committed tree at HEAD `00eab7a0` and Hylla artifact `github.com/evanmschultz/valv@main` pinned to `1759e64`. Vendored sqlite source at `/Users/evanschultz/go/pkg/mod/modernc.org/sqlite@v1.46.1/` was inspected directly because the `_pragma` execution and `busy_timeout` ordering claims are load-bearing.

## R4.F4.1 audit — Option B (drop v0)

- `git tag --list` returned no output: there is no tagged public-release evidence for pre-DROP_14 databases. Treating v0 as removable legacy stands.
- `internal/adapters/sqlite/store.go:115-172` still carries the legacy migration branch: line 130 early-exits when `userVersion >= 1`; lines 137-167 contain the `isLegacyProjectBindingsShape` rebuild path and set `PRAGMA user_version = 1`. So Option B has concrete code to delete; it is not an empty rule.
- `internal/cli/store.go:20` calls `store.Bootstrap(context.Background())` on every CLI store open. Every CLI invocation against a legacy v0 DB therefore hits the new unsupported-schema error.
- PLAN.md embeds Option B in three places: `F1a` (line 30) gives the rationale; Unit 14.1 acceptance line 47 requires "a seeded `user_version = 0` database is rejected with a clear unsupported-schema error"; Notes For Builder Agents line 102 requires the builder to "Delete the legacy `user_version = 0` migration branch from `Store.Bootstrap`". Coverage is complete.
- Minor non-blocking observation: `internal/adapters/sqlite/store_test.go` carries two existing tests that seed `PRAGMA user_version = 0` and assert the legacy migration: `TestStoreMigrationPreservesLegacyCodexBinding` (line 422) and `TestStoreBootstrapIsIdempotentAfterMigration` (line 540). Deleting the v0 branch will break both. The plan's Notes For Builder Agents bullet implies the builder will remove or rewrite them, but does not call them out by name. A capable builder will rewrite one of them into the new "v0 DB is rejected" assertion. Acceptable.

Gap closed. F4.1 verdict: pass.

## R4.F4.2 audit — Dedup by pragma name

- Vendored `modernc.org/sqlite@v1.46.1/driver.go:47,50` documents that `_pragma` values are "run as a 'PRAGMA ...' statement" and "may be specified more than once, '&'-separated". `applyQueryParams` in `sqlite.go:143-166` iterates every `_pragma` value, sorts so any entry whose lowered/trimmed form starts with `busy_timeout` comes first (lines 149-159), then runs each as `pragma <value>`. The driver tolerates multiple entries — silent duplicate appends from `withPragma` would actually execute both pragmas, with the second one applied last. This is exactly the failure mode F4.2 raised.
- Current `internal/adapters/sqlite/open.go:53-72` deduplicates only on exact string equality (line 65: `if existing == pragma`). PLAN.md `F1` (line 29) raises the rule to "case-insensitive prefix match" — `busy_timeout(30000)` blocks any further `busy_timeout(...)` append, mirror for `foreign_keys`. Driver's own sort uses `strings.HasPrefix(x, "busy_timeout")` after `strings.ToLower(strings.TrimSpace(...))`, so the plan's rule matches driver semantics.
- Unit 14.1 acceptance line 46 lists three test cases: (a) no existing pragmas → both appended once; (b) `_pragma=busy_timeout(30000)` → existing preserved, no second `busy_timeout`; (c) `_pragma=foreign_keys(0)` → existing preserved. Case (b) directly exercises the exact counterexample F4.2 constructed.

Gap closed. F4.2 verdict: pass.

## R4.F4.3 audit — Env-list ordering

- `internal/adapters/sqlite/store.go:333-338` (`ListProfilesByProvider`) uses `ORDER BY name ASC` as the existing deterministic-order pattern. The plan's `ORDER BY env_key ASC` mirrors that.
- PLAN.md embeds ordering at three layers: Unit 14.1 line 44 requires the store-layer query to sort by `env_key ASC` with a table-driven test asserting alphabetical rendering for two keys inserted in different orders; Unit 14.3 lines 72-77 require alphabetical ordering in human, JSON, AND plain output for both default and `--reveal` modes.
- Minor non-blocking observation: SQLite default `ASC` is binary collation, so mixed-case keys sort uppercase-before-lowercase (`Foo` < `bar`). The reserved-key validation regex `^[A-Za-z_][A-Za-z0-9_]*$` (Unit 14.2 line 55) permits mixed case. The plan says "alphabetical" without explicit collation. Realistic env-var practice is uppercase-only by convention, so this is unlikely to bite, but a builder ambiguity exists. Acceptable.

Gap closed. F4.3 verdict: pass.

## R4.F4.4 audit — `--format plain` mode

- `internal/cli/root.go:115` confirms `--format` is a persistent root flag accepting `auto | human | plain | json`. Plain mode is a first-class output contract.
- `internal/output/output.go:96-117` shows the existing plain-list branch: writes `heading\n`, then per-item `- <title>\n` plus indented `  <field-key>=<field-value>\n`. The plan's plain-mode acceptance for env list (Unit 14.3 line 76) is `KEY=VALUE` (or `KEY=***`) per line with "no envelope". This diverges from the generic plain-list shape, but the divergence is intentional and parallels the JSON divergence already justified in F3: env values need a dedicated machine-readable shape because the generic envelope does not carry redaction state. The plain shape is also pragmatically useful (pipeable to `eval` / `xargs -L1`). Reasonable design choice, and acceptance is unambiguous on the literal shape.
- Flag-vs-positional precedence is pinned by Unit 14.3 line 77: both `list <name> --reveal` and `list --reveal <name>` must resolve identically per cobra's standard handling.

Gap closed. F4.4 verdict: pass.

## Sanity — drop tree shape

- 4 units: 14.1, 14.2, 14.3, 14.4. No inadvertent unit additions.
- `blocked_by` chain: 14.1 (none) → 14.2 (14.1) → 14.3 (14.2); 14.4 (14.1, DROP_13). Acyclic. Topological order 14.1 → 14.2 → 14.3 → 14.4 (after DROP_13) is buildable in series, with 14.4 deferrable until DROP_13 lands.
- Main `PLAN.md` row for DROP_14 is `planning | blocked_by: DROP_13` — consistent with the drop-level dependency.

## Findings

- 1.1 Minor: Two existing tests in `internal/adapters/sqlite/store_test.go` (`TestStoreMigrationPreservesLegacyCodexBinding` line 422, `TestStoreBootstrapIsIdempotentAfterMigration` line 540) seed `user_version = 0` and assert legacy migration. They will break when Unit 14.1 deletes the v0 branch. The plan's Notes For Builder Agents implicitly requires their removal/rewrite but does not name them. Routine builder activity, not a plan defect.
- 1.2 Minor: `ORDER BY env_key ASC` against SQLite default binary collation sorts uppercase before lowercase. The reserved-key regex `^[A-Za-z_][A-Za-z0-9_]*$` allows mixed case. Real-world env vars are uppercase-by-convention so unlikely to bite, but a builder ambiguity exists if a test seeds `Foo` alongside `bar`. Acceptance language could optionally clarify "case-sensitive alphabetical (SQLite default `ASC` collation)" for unambiguous tests, but the current acceptance is workable.

No blocking findings.

## TL;DR

T1. `git tag --list` empty confirmed; v0 migration branch at `store.go:115-167` still exists for builder to delete; PLAN.md F1a + Unit 14.1 + Notes embed Option B fully — F4.1 closed.
T2. Vendored driver evidence confirms `_pragma` repeats execute additively and driver's own sort uses lower+prefix-match for `busy_timeout`; PLAN.md F1 + Unit 14.1 line 46 test case (b) covers the `busy_timeout(30000)` counterexample exactly — F4.2 closed.
T3. `ORDER BY env_key ASC` mirrors the existing `ListProfilesByProvider` pattern; ordering required at store, human, JSON, AND plain layers — F4.3 closed (minor case-collation observation flagged).
T4. `--format plain` is a first-class root flag; Unit 14.3 line 76 pins the `KEY=VALUE` per-line shape for default and `--reveal`; intentional divergence from generic plain-list envelope mirrors the JSON divergence already justified — F4.4 closed.
T5. 4 units, acyclic blocked_by chain, main PLAN.md row consistent — drop tree sane.
T6. Two minor non-blocking findings flagged (legacy test cleanup, case-collation ambiguity); verdict: pass.
