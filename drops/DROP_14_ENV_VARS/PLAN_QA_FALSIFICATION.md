verdict: fail

## Counterexamples

### 1. Concurrent first-open migration can fail with `SQLITE_BUSY`

Scenario:
Two Valv commands start against the same DB during the first post-DROP_14 open. Both paths call `openStore()` and therefore `Bootstrap()` eagerly. One connection acquires the write lock first; the other can fail immediately with `database is locked` instead of waiting and completing the migration.

Repo evidence:
- `internal/cli/store.go:11-24` bootstraps on every CLI store open.
- `internal/adapters/sqlite/open.go:19-71` adds only `_pragma=foreign_keys(1)` to the DSN; there is no busy-timeout or retry policy.
- `internal/adapters/sqlite/store.go:40-112` runs bootstrap DDL in a write transaction, and `internal/adapters/sqlite/store.go:115-171` runs the version-gated migration in a second write transaction.
- SQLite’s isolation/transaction docs say there is only one writer at a time and a competing write can return `SQLITE_BUSY` while another write transaction is active:
  - https://www.sqlite.org/isolation.html
  - https://www.sqlite.org/lang_transaction.html
- Disposable repro was run in this workspace and then deleted per workflow hygiene.
  Observed output: `bootstrap sqlite store: exec statement: database is locked (5) (SQLITE_BUSY)`

Why this breaks the claim:
Round 3 explicitly asked whether the v1->v2 migration has an interleaving hazard. The partial-visibility version of that hazard does **not** reproduce, but the startup race still breaks real behavior: one command can fail during the migration window.

Narrow fix:
- Add an explicit busy policy for the SQLite DSN and/or a bounded retry wrapper around bootstrap/migration.
- Extend Unit 14.1 acceptance with a two-connection test against a v1 DB that needs the v2 upgrade, asserting “one waits, both succeed” rather than “one may fail with `SQLITE_BUSY`”.

### 2. DROP_14 does not actually cover the post-DROP_13 `valv run` seam

Scenario:
DROP_14 scope requires the generic primitive to receive the account env map “identically” to `valv codex` / `valv claude`, but the unit list only patches provider-specific services. If DROP_13 closes as planned, launch ownership moves into `internal/services/run`, so DROP_14 can pass its codex/claude unit tests while leaving `valv run --account <name> ...` unchanged.

Repo evidence:
- `drops/DROP_14_ENV_VARS/PLAN.md:20` makes generic-run parity part of DROP_14 scope.
- `drops/DROP_14_ENV_VARS/PLAN.md:51-65` only assigns env-threading work to `internal/services/codex` and `internal/services/claude`.
- `drops/DROP_13_GENERIC_RUN/PLAN.md:57-64` makes `valv run` the generic primitive.
- `drops/DROP_13_GENERIC_RUN/PLAN.md:68-145` moves shared launch behavior into `internal/services/run` and turns the provider services into thin adapters.
- `drops/DROP_14_ENV_VARS/PLAN.md:69-70` mentions re-homing as a note only; it is not acceptance-gated.

Why this breaks the claim:
The unchanged plan can succeed on its listed unit paths while still violating the drop-level requirement for `valv run`. That is a hidden dependency on DROP_13’s final code shape, not a builder detail.

Narrow fix:
- Replace Units 14.4/14.5 with one shared-launch unit against whichever package owns `ContainerRunRequest.Env` after DROP_13, plus thin provider assertions if needed.
- At minimum, add explicit acceptance that `valv run --account <name> env` (or equivalent request capture) receives the account env map after DROP_13 lands.

### 3. `account env list` adds a plaintext disclosure surface beyond at-rest SQLite storage

Scenario:
The drop scope uses API-key examples and requires `list <name>`, but the plan never states whether `list` is supposed to print secret values verbatim. As written, human/plain/JSON list output would expose stored credentials directly to terminal scrollback, shell capture, and CI logs.

Repo evidence:
- `drops/DROP_14_ENV_VARS/PLAN.md:14-18` frames the feature around arbitrary env vars and gives secret-bearing examples like `ANTHROPIC_API_KEY`.
- `drops/DROP_14_ENV_VARS/PLAN.md:43-49` requires `valv account env list <name>` in both human and JSON modes.
- `internal/output/output.go:82-110` serializes list item fields verbatim; there is no built-in redaction concept in the current output layer.

Why this breaks the claim:
“Plaintext in SQLite” is already accepted, but `list` creates a second disclosure path that is not called out anywhere in the plan. That is a threat-model gap, not just an implementation detail.

Narrow fix or explicit limitation:
- Preferred: redact values by default and add an explicit reveal flag.
- Minimal: make the plan say plainly that `list` returns raw values in v0.1 and that this is an accepted operator-footgun.

## YAGNI pressure

- The header still allows “`internal/services/manage/` or new `internal/services/accountenv/`”. A new `accountenv` service package would be premature here. Current consumers are the manage CLI and launch path, and `manage.Service` already owns account-scoped CRUD. Keep the repository contract in `internal/domain`/`sqlite`, but avoid a second service layer unless a real second consumer appears.

## Hidden dependency check

- Hidden dependency confirmed: DROP_14 depends on the **post-DROP_13 launch-owner package**, not specifically on today’s `internal/services/codex` and `internal/services/claude` files.
- No fresh hidden dependency found from Unit 14.4/14.5 to Unit 14.2/14.3. Once the repository contract exists, the launch path can read env rows directly; the service/CLI CRUD units are not required first.
- No fresh JSON-key collision found for top-level `"env"` in existing manage-command JSON shapes. Current single-resource keys are command-owned (`"accounts"`, `"projects"`) and grouped output uses `"accounts_by_provider"`.
- No confirmed partial-visibility migration race found. Current bootstrap/migration structure keeps schema rewrites and `PRAGMA user_version` changes inside a transaction, and SQLite’s default connection isolation hides uncommitted changes from other connections. The real concurrency failure is `SQLITE_BUSY`, captured above.
