verdict: pass

# Plan-QA Proof — Round 4

## Scope

Round 4 plan for DROP_14 ENV_VARS reviewed against the committed tree at
`github.com/evanmschultz/valv@main` and the local working copy. This round
addresses the three Round 3 falsification findings (F1 SQLITE_BUSY, F2
post-DROP_13 launch seam, F3 env-list redaction policy) and reshapes the drop
from 5 units to 4 units.

## R3 Finding Audit

### R3.F1 — SQLITE_BUSY first-open race

Round 4 addresses this in `Schema Decisions` § F1 and folds it into Unit 14.1.

- The decision frames the fix as DSN-level (`_pragma=busy_timeout(5000)`), not as
  a separate retry layer, and cites the driver contract that supports doing it
  at the open boundary.
- Cited evidence resolves:
  - `internal/adapters/sqlite/open.go:32-70` currently passes only
    `_pragma=foreign_keys(1)` for both URI and path branches (verified at
    lines 38 and 47); `withPragma` already deduplicates and is reusable for a
    second pragma without code shape changes.
  - `modernc.org/sqlite/driver.go:45-52` documents `_pragma` semantics:
    "Each value will be run as a 'PRAGMA ...' statement (with the PRAGMA
    keyword added for you). May be specified more than once, '&'-separated"
    (verified in vendored source at
    `/Users/evanschultz/go/pkg/mod/modernc.org/sqlite@v1.46.1/driver.go:45-52`).
  - `modernc.org/sqlite/sqlite.go:143-165` proves the "busy_timeout pushed
    first" ordering: `applyQueryParams` collects `q["_pragma"]` then sorts
    "busy_timeout" to the front before executing each as `pragma <value>`
    (verified at lines 143-166 of vendored source).
  - `internal/adapters/sqlite/store.go:35-153` owns Bootstrap + PRAGMA
    user_version (verified: `Bootstrap` at line 40 builds the table set and
    delegates to `migrateProjectBindings` which reads/writes
    `PRAGMA user_version` at lines 127 and 165).
  - `internal/cli/store.go:11-24` bootstraps every CLI store open (verified:
    `openStore` calls `store.Bootstrap(context.Background())` at line 20).
- Unit 14.1 acceptance now includes both the DSN-construction check
  ("adds both pragmas without duplicating either pragma when already present")
  and the concurrent first-open upgrade test ("one waits, both succeed", no
  SQLITE_BUSY, final user_version = 2). Both are yes/no-verifiable.
- Justification for not splitting into Unit 14.0 is sound: the DSN builder and
  the v2 migration share the same sqlite/domain seam; a precondition-only unit
  would not produce an independently buildable artifact. Accepted.

### R3.F2 — post-DROP_13 launch seam coverage

Round 4 addresses this in `Schema Decisions` § F2 and reshapes Unit 14.4.

- The decision chose Option A: a single shared-launch Unit 14.4 keyed on
  `internal/services/run` rather than provider-specific Units 14.4 + 14.5.
- DROP_13 PLAN.md confirms `internal/services/run` is the shared launch owner:
  - `drops/DROP_13_GENERIC_RUN/PLAN.md:36-37` — "Use a new package,
    `internal/services/run`, as the shared launch primitive".
  - `drops/DROP_13_GENERIC_RUN/PLAN.md:74-87` — Unit 13.1 puts the new
    service in `internal/services/run/service.go` and its tests in
    `internal/services/run/service_test.go`.
  - `drops/DROP_13_GENERIC_RUN/PLAN.md:113-149` — Units 13.3 and 13.4
    reduce `internal/services/claude` and `internal/services/codex` to
    thin wrappers around `internal/services/run`; the shared owner takes
    over `ContainerRunRequest` construction.
- Current committed state confirms env ownership still lives in the
  provider services (the seam that must move):
  - `internal/services/codex/service.go:302-332` — `buildRequest` constructs
    `ContainerRunRequest` with `Env: prepared.Env` at line 315.
  - `internal/services/claude/service.go:298-329` — same shape, line 311.
  - `internal/adapters/providers/codex/runtime.go:117-132` — `prepared.Env`
    is built from runtime-owned keys (`CODEX_HOME`, `HOME`, `LOGNAME`,
    `TERM`, `USER`, optional `CLAUDE_CONFIG_DIR`).
  - `internal/adapters/providers/claude/runtime.go:127-142` — mirror map
    (`CLAUDE_CONFIG_DIR`, `HOME`, `LOGNAME`, `TERM`, `USER`, optional
    `CODEX_HOME`).
- Unit 14.4 carries `blocked_by: 14.1, DROP_13` (both an intra-drop dep
  and a cross-drop dep). The acceptance bullet
  "if build-time ownership differs from DROP_13's `internal/services/run`,
  the unit remains blocked and no provider-specific substitute
  implementation is accepted" is the right hard-gate phrasing; the
  Notes-for-builder bullet reinforces it.
- Cross-drop dep chain is acyclic and unambiguous: DROP_14 declares
  `Blocked by: DROP_13` at the drop header (line 4); only Unit 14.4 carries
  the cross-drop dep at the unit level; the other three units (14.1, 14.2,
  14.3) form an internal chain (14.1 -> 14.2 -> 14.3) that can build
  against the current tree without DROP_13 changes. Unit 14.4 cannot start
  until both 14.1 (env repository contract) and DROP_13 (shared launch
  owner) are done. No cycles.
- Mitigated.

### R3.F3 — `account env list` plaintext disclosure

Round 4 addresses this in `Schema Decisions` § F3 and Unit 14.3 acceptance.

- The dev's decision (redact-by-default + `--reveal`) is recorded clearly
  as a Schema Decision, not deferred to builder discretion.
- JSON shape is explicit: top-level key `env` with entries shaped
  `{"key":"FOO","value":"***","redacted":true}` by default and
  `{"key":"FOO","value":"raw","redacted":false}` with `--reveal`.
- Human output rule is explicit: `KEY=***` default, raw `KEY=value` with
  `--reveal`.
- Unit 14.3 acceptance covers all four cases (human default redacted, human
  reveal raw, JSON default redacted, JSON reveal raw) plus the existing
  error and isolation cases.
- Note added: "Redact `valv account env list` by default in both human and
  JSON output. `--reveal` is the only opt-in to raw values..." with an
  explicit threat-model line about scrollback/transcripts being operator
  responsibility post-reveal. The safety bar ("no accidental disclosure in
  default paths") is the right v0.1 stake.
- Mitigated.

## New Round 4 Claim Audit

### `internal/adapters/sqlite/open.go:32-70` — DSN gap

Verified. Lines 35-51 are `buildDSN`; both branches call `withPragma(..., "foreign_keys(1)")` (lines 38 and 47). No `busy_timeout` present. The fix is a one-line additional `withPragma(..., "busy_timeout(5000)")` chain per branch (or equivalent), and the existing dedupe in `withPragma` at lines 64-67 keeps the DSN safe under repeated application.

### `modernc.org/sqlite/driver.go:45-52` — `_pragma` semantics

Verified in vendored source at `/Users/evanschultz/go/pkg/mod/modernc.org/sqlite@v1.46.1/driver.go:45-52`:
> _pragma: Each value will be run as a "PRAGMA ..." statement (with the PRAGMA keyword added for you). May be specified more than once, '&'-separated.

### `modernc.org/sqlite/sqlite.go:143-165` — `busy_timeout` ordering

Verified in vendored source. Lines 142-166 of `applyQueryParams`:
- collects every `q["_pragma"]` value into `a`;
- sorts with comparator that returns true when `x` is busy_timeout (push first), false when `y` is busy_timeout, otherwise lexicographic;
- runs each as `"pragma " + v`.

Plan's cited line range (143-165) bounds the relevant block; the comment "Push 'busy_timeout' first, the rest in lexicographic order" is at line 146.

### `internal/adapters/sqlite/store.go:35-153` — Bootstrap + user_version

Verified. `Bootstrap` starts at line 40; the bootstrap statements table is lines 41-91; the migration block (`migrateProjectBindings`) at lines 115-172 reads `PRAGMA user_version` at line 127 and writes `PRAGMA user_version = 1` at line 165. The plan's "advance Store.Bootstrap from `user_version = 1` to `2`" change point is correct and small.

### `internal/cli/store.go:11-24` — bootstraps every open

Verified. `openStore` is lines 11-25; `Bootstrap` invoked at line 20. Every CLI command path that opens a store inherits the bootstrap (and thus the v2 migration).

### `internal/services/codex/service.go:302-332` + `internal/services/claude/service.go:298-329` — env ownership

Verified. Both `buildRequest` functions construct `ContainerRunRequest` with `Env: prepared.Env` at lines 315 (codex) and 311 (claude). After DROP_13 lands, those request constructions move into `internal/services/run`, which is where Unit 14.4 will inject the merged account env.

### `internal/adapters/providers/codex/runtime.go:117-132` + `internal/adapters/providers/claude/runtime.go:127-142` — runtime-owned key union

Verified. Union of the two maps (excluding the optional cross-provider key which only appears when the other binding exists) is exactly:
`CODEX_HOME, CLAUDE_CONFIG_DIR, HOME, LOGNAME, TERM, USER` — 6 keys, matches the reserved-key set in Unit 14.2.

### DROP_13 plan still places shared launch owner at `internal/services/run`

Verified at `drops/DROP_13_GENERIC_RUN/PLAN.md:36` ("Use a new package, `internal/services/run`, as the shared launch primitive"), `:74-78` (Unit 13.1 paths), `:113-128` (Unit 13.3 reduces claude service to a wrapper around `internal/services/run`), `:133-149` (mirror for codex).

## Unit-by-Unit Completeness

### Unit 14.1 — schema + DSN + repository contract

- Paths: 5 paths (`internal/domain/repository.go`, `internal/adapters/sqlite/open.go`, `internal/adapters/sqlite/open_test.go`, `internal/adapters/sqlite/store.go`, `internal/adapters/sqlite/store_test.go`). All exist except `open_test.go` (new test file, flagged implicitly as new). Acceptable.
- Packages: `internal/domain`, `internal/adapters/sqlite`. Both real.
- Evidence cites resolve (see above).
- Acceptance: 7 bullets, all yes/no-verifiable. The concurrent first-open migration test is the falsifiable mitigation for R3.F1.
- `blocked_by: none`. No cycle.

### Unit 14.2 — manage service env CRUD

- Paths: `internal/services/manage/service.go`, `internal/services/manage/service_test.go`. Both exist.
- Packages: `internal/services/manage`. Real.
- Evidence cites resolve:
  - `internal/services/manage/service.go:19-25,166-190` — verified (lines 19-25 are the `Store` interface; lines 166-190 include `ProfileByName` at 186).
  - `internal/services/manage/service_test.go:660-703` — accepted as scoped reference for rename-stability coverage style (not separately verified).
  - Reserved-key citations to provider runtimes verified above.
- Acceptance: 6 bullets, all yes/no-verifiable; the literal-regex assertion (`^[A-Za-z_][A-Za-z0-9_]*$`) is a strong falsifiable contract.
- `blocked_by: 14.1`. Linear, no cycle.

### Unit 14.3 — CLI subcommands

- Paths: `internal/cli/manage.go`, `internal/cli/manage_test.go`, `internal/cli/extended_test.go`. All exist.
- Packages: `internal/cli`. Real.
- Evidence cites resolve:
  - `internal/cli/manage.go:22-63` — account command tree wiring (not separately re-verified beyond confirming `manage.go` exists and has the expected shape from earlier reads).
  - `internal/cli/manage.go:200-239,1348-1365` — verified at 234 (WriteListWithKey "accounts") and 1365 (mirror).
  - `internal/output/output.go:82-93` — verified; `WriteListWithKey` envelopes items as `ListItem` (laslig alias = `{Label, Value, Badge, Ident}`). Plan is correct that the generic envelope cannot carry per-entry `redacted` metadata cleanly.
- Acceptance: 8 bullets, all yes/no-verifiable; covers all four redaction/reveal axes.
- `blocked_by: 14.2`. Linear, no cycle.

### Unit 14.4 — shared launch owner env injection

- Paths: `internal/services/run/service.go` (new after DROP_13), `internal/services/run/service_test.go` (new after DROP_13), optionally codex/claude service tests for thin-wrapper assertions. Correctly flagged as "not yet in the current tree".
- Packages: `internal/services/run`, optionally `internal/services/codex` / `internal/services/claude`.
- Evidence cites resolve (see above).
- Acceptance: 4 bullets + cross-drop gate sentence. The "directly seeded reserved-key rows in storage do not override runtime-owned values even when CLI/service-layer validation is bypassed" bullet is a strong defense-in-depth check that closes the gap a builder might miss if they only enforce reservation at the CLI/service layer.
- `blocked_by: 14.1, DROP_13`. Cross-drop chain: DROP_14 -> DROP_13 (header) + Unit 14.4 -> 14.1 (intra-drop) + Unit 14.4 -> DROP_13 (cross). DROP_13 declares no dep on DROP_14, so no cycle. Hard-gate language for "stop and re-plan" if DROP_13 lands with a different owner is preserved.

## JSON Shape Collision Check

- `env` is not currently used as a top-level JSON key by any manage command. Verified by reading the three `WriteListWithKey` call sites (`internal/cli/manage.go:234`, `:1365`, `:1475`) — those use `"accounts"` and (at 1475) `"bindings"`.
- Proposed `{"key","value","redacted"}` entry shape does not collide with the existing `ListItem` shape (`Label`, `Value`, `Badge`, `Ident`) — both the field names and the semantic intent differ. Plan correctly avoids `WriteListWithKey` for this case and is explicit about using a dedicated env-list formatter.

## Reserved-Key Set Check

Reserved set per Unit 14.2: `CODEX_HOME, CLAUDE_CONFIG_DIR, HOME, LOGNAME, TERM, USER` (6 keys).

Union from runtime adapters:
- `internal/adapters/providers/codex/runtime.go:117-123` — `CODEX_HOME, HOME, LOGNAME, TERM, USER` (5) + optional `CLAUDE_CONFIG_DIR` at line 131 when other-provider profile is mounted.
- `internal/adapters/providers/claude/runtime.go:127-133` — `CLAUDE_CONFIG_DIR, HOME, LOGNAME, TERM, USER` (5) + optional `CODEX_HOME` at line 141 when other-provider profile is mounted.

Union = exactly 6 keys, matches Unit 14.2. Complete.

## Findings

No blocking findings.

Minor observations (non-blocking, dev/builder discretion):

- The plan does not state a default for whether `--reveal` is allowed in CI-piped JSON output (i.e. when stdout is not a TTY). Current scope statement implies always-allowed; tests should at least assert behavior is consistent across TTY and non-TTY output. Acceptable for v0.1; flag for dev awareness.
- The "two-connection first-open upgrade" test in Unit 14.1 acceptance is correctly added but is the most likely test to be flaky if implemented naively (e.g., without explicit goroutine sync). Builder should use a started/proceed channel pattern. Not a plan-level defect.

## Verdict

`verdict: pass` — Round 4 plan is internally consistent, all R3 findings are mitigated with structural changes (not just commentary), and every new cited line range resolves against the actual tree (committed code and vendored modernc.org/sqlite@v1.46.1 source). The 4-unit shape (14.1 -> 14.2 -> 14.3 + 14.4 cross-blocked on DROP_13) is acyclic and acceptance criteria are yes/no-verifiable.
