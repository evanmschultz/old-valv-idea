# DROP_14 — ENV_VARS

**State:** building
**Blocked by:** DROP_13 (building)
**Paths (expected):** `internal/domain/` (env-var domain type), `internal/adapters/sqlite/` (env-var schema migration + DSN busy policy), `internal/services/manage/` (account-scoped CRUD), `internal/cli/` (new `valv account env` subcommand tree), `internal/services/run/` (shared launch owner after DROP_13)
**Packages (expected):** `internal/domain/`, `internal/adapters/sqlite/`, `internal/services/manage/`, `internal/cli/`, `internal/services/run/`
**PLAN.md ref:** main/PLAN.md → DROP_14_ENV_VARS row
**Workflow:** main/drops/WORKFLOW.md
**Started:** 2026-05-22
**Closed:** —

## Scope

**Theme 1 from `project_valv_future_planning_post_drop8.md`.** Per-account env var map — arbitrary `key→value` pairs scoped to a managed account. Same env-var name allowed across accounts with different values (e.g. `ANTHROPIC_API_KEY = sk-aaa` for `personal`, `sk-bbb` for `hylla`). Threaded into container launch via `ContainerRunRequest.Env` alongside the existing `CLAUDE_CONFIG_DIR` / `CODEX_HOME` env vars set by the provider runtime adapters.

CLI surface: `valv account env set <name> <KEY=VALUE>`, `valv account env unset <name> <KEY>`, `valv account env list <name>`. The set/unset/list operations are account-scoped (one account, one env-var map).

Storage: plaintext-in-SQLite at v0.1. Keychain integration deferred — flagged in the worklog as a follow-up so the threat model is documented even though the v0.1 implementation doesn't encrypt at rest.

The DROP_13 generic-run primitive must accept the resolved env-var map identically to how `valv codex` / `valv claude` do today. In this revision that requirement is promoted from note to gate: DROP_14 assumes DROP_13 re-homes request construction into the shared launch owner and will not silently revive provider-specific env threading if DROP_13 closes differently.

## Planner

### Scope Confirmation
DROP_14 adds an account-scoped env map persisted in SQLite and injected into container launches via `ContainerRunRequest.Env`. The current committed seams are verified at `internal/domain/repository.go:5-32`, `internal/cli/store.go:11-24`, `internal/adapters/sqlite/open.go:32-70`, `internal/adapters/sqlite/store.go:35-153`, `internal/adapters/sqlite/store_test.go:504-668`, `internal/services/manage/service.go:19-25,166-190`, `internal/cli/manage.go:22-63,200-239,1348-1365`, `internal/services/codex/service.go:121-215,302-332`, `internal/services/claude/service.go:123-223,298-329`, `internal/adapters/providers/codex/runtime.go:117-132`, and `internal/adapters/providers/claude/runtime.go:127-142`. Hylla at `github.com/evanmschultz/valv@main` pinned to `1759e64` confirms the same callers/tests for `Store.Bootstrap`, `newManageAccountCommand`, and both provider `Service.Run` paths.

### Schema Decisions

- **F1: handle the SQLITE_BUSY gap inside Unit 14.1; do not add Unit 14.0.** The DSN builder and the v2 migration both live in the same sqlite/domain boundary, so splitting them into a standalone precondition unit would create a fifth task without creating an independently buildable seam. The fix is DSN-level busy policy, not a separate retry loop: add `_pragma=busy_timeout(5000)` alongside `_pragma=foreign_keys(1)` in `internal/adapters/sqlite/open.go`, then advance `Store.Bootstrap` to `user_version = 2` for the new env-var table. This is directly supported by the driver contract that `_pragma` query params are executed as `PRAGMA ...` statements (`modernc.org/sqlite/driver.go:45-52`) and that `busy_timeout` is intentionally applied first when multiple pragmas are present (`modernc.org/sqlite/sqlite.go:143-165`). The DSN dedup rule is by **exact pragma-name parsing**, NOT by prefix-string matching. The dedup helper MUST: (1) trim leading/trailing whitespace from each existing `_pragma` value; (2) split each value at the first `(` or `=` character to isolate the pragma name; (3) lowercase that pragma name; (4) compare for exact string equality with `busy_timeout` / `foreign_keys`. Prefix matching is wrong because it false-positives on caller-supplied values like `_pragma=busy_timeout_pragma=foo` (a legal value with no real `busy_timeout(...)` pragma) and would silently skip appending the required busy policy. The acceptance for Unit 14.1 therefore includes a two-connection first-open migration test against a v1 DB needing v2 upgrade, asserting "one waits, both succeed" instead of allowing `SQLITE_BUSY`.
- **F1a: choose Option B and drop legacy `user_version = 0` support in DROP_14; bound the supported version window explicitly.** Hylla at `github.com/evanmschultz/valv@main` pinned to `1759e64` confirms the committed store still carries a `user_version < 1` migration branch in `Store.migrateProjectBindings` and that the sqlite tests still seed `PRAGMA user_version = 0` (`internal/adapters/sqlite/store.go:95-153`, `internal/adapters/sqlite/store_test.go:504-668`). LSP confirms `Store.Bootstrap` is invoked on every CLI store open from `internal/cli/store.go:19`, so preserving that branch would preserve a real first-open path that DROP_14 would otherwise stop testing concurrently. `git tag --list` returned no tags on 2026-05-23, so there is no tagged public-release evidence for pre-DROP_14 databases; treat v0 compatibility as removable legacy, not a required support floor. Unit 14.1 must therefore: (a) delete the v0 migration branch entirely (including removing or rewriting the v0-seeded tests at `internal/adapters/sqlite/store_test.go:504-668`); (b) introduce a new sentinel `domain.ErrUnsupportedSchema` in `internal/domain/errors.go` (new, not yet in tree); (c) bound the supported schema window as `user_version >= 1 AND user_version <= 2`; (d) reject BOTH `user_version = 0` (too old) AND `user_version > 2` (too new) by wrapping `domain.ErrUnsupportedSchema` with `fmt.Errorf("unsupported schema: got user_version=%d, supported window [1, 2]: %w", got, domain.ErrUnsupportedSchema)`; (e) tests use `errors.Is(err, domain.ErrUnsupportedSchema)` for both directions, never string-match the error message.
- **F2: replace provider-specific launch units with one shared-launch unit against DROP_13's launch owner.** DROP_13's accepted architecture moves shared launch orchestration into `internal/services/run` (`drops/DROP_13_GENERIC_RUN/PLAN.md:36-37,72-82,113-143`), so DROP_14 must add env threading at that shared owner rather than at `internal/services/codex` and `internal/services/claude`. This promotes the cross-drop dependency from note-only to acceptance gate: Unit 14.4 is blocked by DROP_13, and if DROP_13 lands with a different `ContainerRunRequest.Env` owner, the builder must stop and re-plan instead of patching stale provider-specific services.
- **F3: JSON list output is redacted by default with explicit metadata.** `valv account env list <name>` uses top-level key `env` (new, not yet in tree) whose entries are `{"key":"FOO","value":"***","redacted":true}` by default and `{"key":"FOO","value":"raw","redacted":false}` when `--reveal` is present. This is intentionally more specific than the generic list-item envelope used by existing manage commands (`internal/output/output.go:82-93`, `internal/cli/extended_test.go:136-153`): env values need machine-readable redaction state so consumers do not have to infer policy from the literal string `"***"`.

### Unit 14.1
- state: done
- blocked_by: none
- paths: `internal/domain/repository.go`, `internal/adapters/sqlite/open.go`, `internal/adapters/sqlite/open_test.go`, `internal/adapters/sqlite/store.go`, `internal/adapters/sqlite/store_test.go`
- packages: `internal/domain`, `internal/adapters/sqlite`
- change: Add an account-env repository contract (new, not yet in tree) in `internal/domain/repository.go` and implement it in `sqlite.Store`. Extend `Open` so every SQLite connection carries `_pragma=busy_timeout(5000)` as well as `_pragma=foreign_keys(1)` for both path- and URI-based opens, then advance `Store.Bootstrap` from `user_version = 1` to `2` with a forward-only new account-env table (new, not yet in tree) keyed by `profile_id + env_key` and storing plaintext `env_value`. The table must be owned by `profile_id`, not account name, so duplicate keys across accounts are legal and account renames remain stable because `Profile.ID` does not change. Evidence: `internal/cli/store.go:11-24` bootstraps on every CLI store open; `internal/adapters/sqlite/open.go:32-70` currently adds only `_pragma=foreign_keys(1)`; `modernc.org/sqlite/driver.go:45-52` and `modernc.org/sqlite/sqlite.go:143-165` prove `_pragma` support and `busy_timeout` ordering; `internal/adapters/sqlite/store.go:35-153` owns bootstrap + `PRAGMA user_version`; `internal/adapters/sqlite/store_test.go:504-668` already proves migration/idempotence behavior for the current schema path.
- acceptance: `mage testPkg ./internal/domain` and `mage testPkg ./internal/adapters/sqlite` pass. Extend sqlite coverage to prove:
  - upgrade from `user_version = 1` to `2`;
  - idempotence on repeated bootstrap at v2;
  - duplicate-key support across two different profiles;
  - set/list/unset CRUD at the store layer, with `set/list/unset` ordering proven deterministic by `ORDER BY env_key ASC` at the store-layer query — table-driven test asserts two keys inserted in different orders render alphabetically;
  - rename stability after `UpdateProfileName` because env ownership is keyed by unchanged `profile_id`;
  - DSN construction adds both `_pragma=busy_timeout(5000)` and `_pragma=foreign_keys(1)` without duplicating either pragma when already present. Dedup is by **exact pragma-name parsing** (trim whitespace; split at first `(` or `=`; lowercase; equality compare against `busy_timeout` / `foreign_keys`), NOT by prefix-string matching. Tests cover: (a) opening with no existing pragmas → both appended once; (b) opening with `_pragma=busy_timeout(30000)` (different numeric arg) → existing busy_timeout preserved, no second one appended; (c) opening with `_pragma=foreign_keys(0)` → existing preserved, no second one appended; (d) opening with `_pragma=busy_timeout_pragma=foo` (false-prefix value) → existing value preserved AND `busy_timeout(5000)` IS appended because the parsed pragma name `busy_timeout_pragma` does not equal `busy_timeout`;
  - a seeded `user_version = 0` database is rejected with an error matching `errors.Is(err, domain.ErrUnsupportedSchema)` and whose message includes the found and supported version range;
  - a seeded `user_version = 99` database (forward-incompatible) is rejected with the same `errors.Is(err, domain.ErrUnsupportedSchema)` contract;
  - a two-connection first-open upgrade from a v1 DB to v2 results in "one waits, both succeed", no `SQLITE_BUSY`, and final `PRAGMA user_version = 2`.

### Unit 14.2
- state: done
- blocked_by: 14.1
- paths: `internal/services/manage/service.go`, `internal/services/manage/service_test.go`
- packages: `internal/services/manage`
- change: Extend `manage.Service` and its `Store` aggregation with account-env CRUD methods (new, not yet in tree) that resolve accounts through the existing `ProfileByName` seam, wrap repository errors in the current manage-service style, validate env keys against the explicit regex `^[A-Za-z_][A-Za-z0-9_]*$`, and surface that literal pattern in the rejection error so operators see the constraint. Reject the six runtime-owned keys that the committed launch env builders already own: `CODEX_HOME`, `CLAUDE_CONFIG_DIR`, `HOME`, `LOGNAME`, `TERM`, and `USER`. Evidence: `internal/services/manage/service.go:19-25,166-190` shows the current service/store seam and `ProfileByName` lookup path; `internal/services/manage/service_test.go:660-703` shows rename stability is already a first-class invariant; `internal/adapters/providers/codex/runtime.go:117-132` and `internal/adapters/providers/claude/runtime.go:127-142` define the current runtime-owned env keys that DROP_14 must reserve.
- acceptance: `mage testPkg ./internal/services/manage` passes with table-driven coverage for:
  - set/unset/list happy paths on one account;
  - missing-account errors;
  - reserved-key rejection for all six protected keys;
  - same key with different values on different accounts;
  - rename stability through `Profile.ID`;
  - env-key rejection for leading-digit, whitespace, hyphenated, dotted, control-character, and empty keys.
  The failing-key assertions must verify that the returned error wraps the literal regex `^[A-Za-z_][A-Za-z0-9_]*$`.

### Unit 14.3
- state: done
- blocked_by: 14.2
- paths: `internal/cli/manage.go`, `internal/cli/manage_test.go`, `internal/cli/extended_test.go`
- packages: `internal/cli`
- change: Add a new `account env` branch under `newManageAccountCommand` (new subcommands, not yet in tree) with `set <name> <KEY=VALUE>`, `unset <name> <KEY>`, and `list <name>`. Reuse `openManageService`, the existing output-mode detection, and the current account-command wiring, but use a dedicated env-list formatter for JSON instead of `output.WriteListWithKey`: env listing needs explicit `key/value/redacted` entries rather than the generic `title/fields` envelope. Human output for `valv account env list <name>` is `KEY=***` by default and raw `KEY=value` only with `--reveal`. JSON output uses top-level key `env` with entries shaped as `{"key":"FOO","value":"***","redacted":true}` by default and raw values plus `redacted:false` when `--reveal` is set. Evidence: `internal/cli/manage.go:22-63` wires the existing account tree; `internal/cli/manage.go:200-239,1348-1365` shows current output-mode and command-owned JSON-key conventions; `internal/cli/extended_test.go:136-153` pins the existing command-owned JSON key pattern; `internal/output/output.go:82-93` shows why the generic list envelope is not sufficient for machine-readable redaction metadata.
- acceptance: `mage testPkg ./internal/cli` passes with coverage for:
  - human `list` output redacted by default as `KEY=***`, sorted alphabetically by env key;
  - human `list --reveal` output showing raw values, sorted alphabetically by env key;
  - JSON `list` output redacted by default with top-level key `"env"` and per-entry `redacted: true`, with entries ordered alphabetically by `key`;
  - JSON `list --reveal` output showing raw values with `redacted: false`, with entries ordered alphabetically by `key`;
  - `--format plain` output for both default and `--reveal` modes: emits one `KEY=VALUE` (or `KEY=***`) per line, no envelope, sorted alphabetically by env key; same redaction semantics as human/JSON apply;
  - `--reveal` is a flag on the `list` subcommand specifically: both `valv account env list <name> --reveal` and `valv account env list --reveal <name>` resolve identically per cobra's standard flag-vs-positional precedence;
  - malformed `KEY=VALUE` input;
  - reserved-key errors propagated from the service layer;
  - missing-account errors;
  - same env-key separation across two accounts.

### Unit 14.4 (Round 3 — orch-direct edit folding Round 1 plan-QA findings)

Round 1 plan-QA (commit `ec96e8a`) FAILED on two concrete counterexamples that the Round-2 two-droplet decomposition (commit `21e402b`) missed:

- **CF-1**: `claude.Store` (services/claude/service.go:23-27) and `codex.Store` (services/codex/service.go:22-26) do not embed `domain.AccountEnvRepository`. For wrappers to call `s.store.ListAccountEnv(...)` they must widen both interfaces — 2 uncounted production symbols pushing 14.4.2 from 2 → 4 prod symbols (≥3 = FAIL per `aa130dd`).
- **CF-2** (orch grep resolved falsif OQ-1): `internal/cli/run.go:202` is a THIRD `runservice.LaunchRequest{}` construction site beyond the two wrapper `Service.Run` methods. The Round-2 plan never edited cli/run.go, so `valv run --account` would silently skip env merge.

Round 3 splits into **5 atomic droplets** with a small domain helper (NIT-3 fold) to eliminate triplicate `[]AccountEnvEntry`→map copy-paste across the 3 call sites. `internal/cli/run.go` uses the full `*sqlite.Store` directly (verified at run.go:123) plus `manage.Service` (run.go:93-99) — it does NOT use the narrow wrapper Store interfaces, so 14.4.E is not blocked on the Store widening (14.4.C).

**Design (unchanged from dev's prior ruling):** wrappers own Store; AccountEnv flows through a new `LaunchRequest.AccountEnv map[string]string` field; `internal/services/run` stays Store-free. The 6 runtime-owned keys (`CODEX_HOME`, `CLAUDE_CONFIG_DIR`, `HOME`, `LOGNAME`, `TERM`, `USER`) win on collision in the run service's merge.

**Build parallelization:**
- **Level 0** (no intra-14.4 blockers; 3 droplets parallel): 14.4.A (domain helper), 14.4.B (run-svc field+merge), 14.4.C (Store widening).
- **Level 1** (2 droplets, can fire in parallel; E does NOT wait for C): 14.4.D `blocked_by` A+B+C; 14.4.E `blocked_by` A+B.

#### Unit 14.4.A — Domain helper `AccountEnvEntriesToMap`

- state: done
- blocked_by: none
- paths: `internal/domain/account_env.go` (new — not yet in tree)
- packages: `./internal/domain`
- change: Add `domain.AccountEnvEntriesToMap(entries []AccountEnvEntry) map[string]string` (new — not yet in tree). Iterates the slice once, writes each `EnvKey`→`EnvValue` into a freshly allocated map (nil-safe: nil/empty slice returns nil). Single tiny helper consumed by 14.4.D (both wrappers) and 14.4.E (cli/run.go) to avoid triplicate inline conversion.
- acceptance: `mage testPkg ./internal/domain` passes. Add `internal/domain/account_env_test.go` (new — not yet in tree) with table-driven coverage:
  - `TestAccountEnvEntriesToMap` (new — not yet in tree): nil slice → nil map; empty slice → nil map; single entry → single-key map; two entries → two-key map; duplicate-key last-wins semantics (documented behavior; entries are pre-deduped by the SQLite UNIQUE constraint but the helper does not panic on hypothetical duplicates).
- measurement: distinct new/changed production symbols = 1 (`AccountEnvEntriesToMap`); production LOC ≈ 8; production files = 1 (new file). Under budget.
- commit subject (orchestrator): `feat(domain): unit 14.4.a account-env entries-to-map helper`.

#### Unit 14.4.B — Run service `LaunchRequest.AccountEnv` field + buildRequest merge

- state: done
- blocked_by: none
- paths: `internal/services/run/service.go`, `internal/services/run/service_test.go`
- packages: `./internal/services/run`
- change: One cohesive same-purpose edit cluster on `internal/services/run/service.go`:
  - Add field `AccountEnv map[string]string` to existing `LaunchRequest` struct (currently service.go:121-136).
  - Edit `buildRequest` (service.go:229-283, env-assembly at line 264 `Env: launch.Prepared.Env`) so the assembled env is the merge of `launch.AccountEnv` ⊕ `launch.Prepared.Env` with **`Prepared.Env` winning on collision** (runtime-owned keys are set by the provider runtime adapter into `Prepared.Env`, so this preserves them automatically).
  - Implementation pattern: `merged := make(map[string]string, len(launch.AccountEnv)+len(launch.Prepared.Env)); for k,v := range launch.AccountEnv { merged[k]=v }; for k,v := range launch.Prepared.Env { merged[k]=v }; <ContainerRunRequest>.Env = merged` (account env first, prepared overrides). Nil/empty `AccountEnv` and nil/empty `Prepared.Env` both handled without panic.
- acceptance: `mage testPkg ./internal/services/run` passes. Add to `internal/services/run/service_test.go`:
  - `TestRunMergesAccountEnvIntoContainerEnv` (new — not yet in tree): table-driven with one ordinary key (`API_KEY=secret`) seeded in `launch.AccountEnv`; assert it appears in `ContainerRunRequest.Env` exactly.
  - `TestRunPreservesRuntimeOwnedEnvOnAccountEnvCollision` (new — not yet in tree): table-driven over each of the 6 keys (`CODEX_HOME`, `CLAUDE_CONFIG_DIR`, `HOME`, `LOGNAME`, `TERM`, `USER`); each row seeds `Prepared.Env[K]="runtime"` AND `launch.AccountEnv[K]="account"`; assert `ContainerRunRequest.Env[K]=="runtime"`. The test seeds `Prepared.Env` directly via test fixture — does NOT depend on the cross-mount conditional in the adapter (per Round-1 NIT 3.3).
  - nil/empty `AccountEnv` case: `ContainerRunRequest.Env` equals `Prepared.Env` exactly (existing-behavior regression).
- measurement: distinct new/changed production symbols = 1 effective (LaunchRequest field add + buildRequest body edit form one cohesive same-purpose cluster — "add field, merge it"); production LOC ≈ 25; production files = 1. Under budget. NIT-1 from Round 1 falsif: the cohesive-cluster interpretation is explicit here, not inferred.
- commit subject (orchestrator): `feat(run): unit 14.4.b merge account env into launch`.

#### Unit 14.4.C — Widen wrapper Store interfaces to embed `AccountEnvRepository`

- state: done
- blocked_by: none
- paths: `internal/services/claude/service.go`, `internal/services/codex/service.go`, `internal/services/claude/service_test.go`, `internal/services/codex/service_test.go`
- packages: `./internal/services/claude`, `./internal/services/codex`
- change: Two parallel interface widenings (no behavior, no logic):
  - `internal/services/claude/service.go:23-27` — add `domain.AccountEnvRepository` to the `claude.Store` interface embed list (currently `domain.ProjectRepository`, `domain.BindingRepository`, `domain.ProfileRepository`).
  - `internal/services/codex/service.go:22-26` — same widening on `codex.Store`.
  - Test-side: extend the existing `fakeStore` (or equivalent test-double) in each package's `service_test.go` with the `AccountEnvRepository` methods (`ListAccountEnv`, `SetAccountEnv`, `UnsetAccountEnv` per `internal/domain/repository.go:43-56`) — return empty/nil by default so no existing test changes behavior. Test-side additions are NOT counted toward the production-symbol budget per `aa130dd` (tests excluded).
- acceptance: `mage testPkg ./internal/services/claude` and `mage testPkg ./internal/services/codex` both pass with no behavioral change. Existing tests must continue to pass with the widened fakeStore. No new production tests in this droplet (the interface change has no behavior to exercise; behavior tests land in 14.4.D).
- measurement: distinct new/changed production symbols = 2 (`claude.Store` interface widening + `codex.Store` interface widening); production LOC ≈ 2 (one embed line per interface); production files = 2. Under budget.
- commit subject (orchestrator): `feat(services): unit 14.4.c widen wrapper store with account env repository`.

#### Unit 14.4.D — Provider wrappers load account env and pass it into LaunchRequest

- state: todo
- blocked_by: 14.4.A (domain helper), 14.4.B (LaunchRequest field), 14.4.C (Store widening)
- paths: `internal/services/claude/service.go`, `internal/services/codex/service.go`, `internal/services/claude/service_test.go`, `internal/services/codex/service_test.go`
- packages: `./internal/services/claude`, `./internal/services/codex`
- change: Edit existing methods `claude.Service.Run` (service.go:127) and `codex.Service.Run` (service.go:120) only. After the existing profile/project resolution and before the `runservice.LaunchRequest{...}` construction (claude:225, codex:216):
  - call `entries, err := s.store.ListAccountEnv(ctx, resolved.profile.ID)` (or equivalent variable names matching the existing context naming in each file);
  - wrap any error with `fmt.Errorf("load account env for profile %q: %w", resolved.profile.ID, err)`;
  - convert via `domain.AccountEnvEntriesToMap(entries)` from 14.4.A;
  - set `AccountEnv:` on the existing `runservice.LaunchRequest{...}` literal.
- acceptance: `mage testPkg ./internal/services/claude` and `mage testPkg ./internal/services/codex` pass. Tests with fake-store seeded `AccountEnvEntry`s:
  - `TestRunPassesAccountEnvToSharedRunService` (new — not yet in tree, one per wrapper package): seed `fakeStore` with 2 entries (`FOO=bar`, `BAZ=qux`) for the resolved profile; assert the `runservice.LaunchRequest` reaching the executor has `AccountEnv == {"FOO":"bar","BAZ":"qux"}`.
  - No-entries case: `fakeStore.ListAccountEnv` returns nil; assert `LaunchRequest.AccountEnv` is nil and launch succeeds.
  - Store-error case: `fakeStore.ListAccountEnv` returns an error; assert `Service.Run` returns the wrapped error and does not call the executor.
- measurement: distinct new/changed production symbols = 2 (`claude.Service.Run`, `codex.Service.Run` method edit clusters); production LOC ≈ 40 (~20 per wrapper for load + convert + field set + error wrap); production files = 2. Under budget.
- commit subject (orchestrator): `feat(services): unit 14.4.d pass account env from claude+codex wrappers`.

#### Unit 14.4.E — CLI `valv run` loads account env and passes it into LaunchRequest

- state: todo
- blocked_by: 14.4.A (domain helper), 14.4.B (LaunchRequest field). NOT blocked by 14.4.C: `cli/run.go` uses `*sqlite.Store` directly (run.go:123) + `manage.Service` (run.go:93-99); it does NOT use the narrow wrapper Store interfaces being widened in 14.4.C.
- paths: `internal/cli/run.go`, `internal/cli/run_test.go`
- packages: `./internal/cli`
- change: Edit existing `runRunCommand` in `internal/cli/run.go`. After the profile resolution (run.go:99) and before the `runservice.LaunchRequest{...}` construction (run.go:202-210):
  - call `entries, err := store.ListAccountEnv(cmd.Context(), profile.ID)` (the `store` ref from run.go:123 is `*sqlite.Store` which already implements `AccountEnvRepository`);
  - wrap any error with `fmt.Errorf("run run command: load account env for profile %q: %w", profile.ID, err)`;
  - convert via `domain.AccountEnvEntriesToMap(entries)` from 14.4.A;
  - set `AccountEnv:` on the existing `launch := runservice.LaunchRequest{...}` literal (run.go:202).
- acceptance: `mage testPkg ./internal/cli` passes. Add to `internal/cli/run_test.go`:
  - `TestRunCommandPassesAccountEnvToLaunchRequest` (new — not yet in tree): with seeded `AccountEnvEntry`s for the resolved profile, assert the constructed `LaunchRequest.AccountEnv` matches; via the existing test stub `runRunFunc` injection seam (run.go:23-26) or by intercepting at the store level if a smaller seam exists.
  - No-entries case: nil `AccountEnv` does not fail; launch proceeds.
- measurement: distinct new/changed production symbols = 1 (`runRunCommand` method edit cluster); production LOC ≈ 15; production files = 1. Under budget.
- commit subject (orchestrator): `feat(cli): unit 14.4.e pass account env from valv run`.

#### Cross-drop note

This 5-droplet sequence is the cascade group that closes DROP_14. Drop-end gate (orchestrator's job): `mage test` + **`mage integration`** (required — env merge touches Docker-backed launch paths) + `git push` + `gh run watch --exit-status` + `mage build` + Hylla reingest from remote pinned to the final commit hash.

### Notes For Builder Agents

- Plaintext-in-SQLite is accepted for v0.1. Do not add keychain work, at-rest encryption, or secrets-manager plumbing in this drop.
- Redact `valv account env list` by default in both human and JSON output. `--reveal` is the only opt-in to raw values. Terminal scrollback, copied transcripts, redirected stdout/stderr, and shell capture after `--reveal` are operator responsibility; the plan's safety bar is "no accidental disclosure in default paths", not "no disclosure when explicitly requested".
- Keep the reserved-key list exactly to the six runtime-owned keys in this drop: `CODEX_HOME`, `CLAUDE_CONFIG_DIR`, `HOME`, `LOGNAME`, `TERM`, and `USER`.
- Keep the busy policy at the SQLite open boundary (`open.go`) so every caller that opens the DB inherits it. Do not add a second ad hoc retry layer unless the code proves `busy_timeout` alone is insufficient.
- Delete the legacy `user_version = 0` migration branch from `Store.Bootstrap`. DROP_14's minimum supported starting schema is v1; a seeded v0 DB must fail fast with a clear error instead of auto-migrating.
- Treat DROP_13's shared launch owner as a hard gate. If `ContainerRunRequest.Env` is no longer assembled in `internal/services/run` when DROP_14 build starts, stop and re-plan rather than reviving provider-specific env threading.
- Keep tests table-driven where the behavior is parse-heavy or policy-heavy (regex validation, reserved keys, redaction/reveal output, concurrent first-open migration).
