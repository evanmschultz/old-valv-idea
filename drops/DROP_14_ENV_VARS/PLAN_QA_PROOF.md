verdict: pass

# DROP_14 — Plan QA Proof, Round 3

## Round context

Round 3 planner verified the Round 2 plan is sound as-is and made no edits.
This proof re-verifies every plan claim against current `main` HEAD `cdb7cf3`
(post-DROP_12 close, ingest snapshot `1759e64`). Drop is `state: planning`
with empty `BUILDER_WORKLOG.md`, so build has not started.

## 1. Per-claim audit (plan → repo)

| Plan claim | Cite | Verified at |
|---|---|---|
| No env repository today | `internal/domain/repository.go:5-25` | repository.go:5-25 — ProjectRepository, ProfileRepository, BindingRepository, RuntimeRepository, ProviderImageRepository only |
| Bootstrap owns `user_version` and forward-only rebuild pattern | `internal/adapters/sqlite/store.go:40-172` | Bootstrap at 40-113 creates base tables; `migrateProjectBindings` at 115-172 owns user_version gate at line 127 + rebuild block 142-167 + `PRAGMA user_version = 1` at 165 |
| Sqlite migration coverage exists | `internal/adapters/sqlite/store_test.go:504-660` | `TestStoreBootstrapMigratesLegacyProjectBindings` reads PRAGMA user_version pre/post (504-523) + `TestStoreBootstrapIsIdempotentAfterMigration` runs second Bootstrap and asserts row+staging counts (540-668) |
| manage.Service Store seam aggregates Project/Binding/Profile | `internal/services/manage/service.go:21-25` | Store interface at 21-25 |
| `ProfileByName` lookup exists | `internal/services/manage/service.go:186-190` | `Service.ProfileByName` at 186-192 |
| Rename stability is a first-class invariant | `internal/services/manage/service_test.go:229-239,660-703` | `TestRenameProfile*` 229-239 + `TestListProfilesPrefersRenamedPrimaryHostAccountOverLegacyAlias` 660-703 |
| Manage account tree wires 10 subcommands, no `env` | `internal/cli/manage.go:22-63` | `newManageAccountCommand` 22-64; subcommands at 52-62 (add/bind/unbind/inspect/login/logout/list/rename/cleanup/delete/switch) |
| List command uses `"accounts"` JSON key | `internal/cli/manage.go:220-236` | line 234: `output.WriteListWithKey(... "accounts", listItemsForAccounts(...))` |
| Grouped manage list uses `accounts_by_provider` | `internal/cli/manage.go:1333-1365` | `writeAccountsByProvider` at 1333-1370; JSON tag `accounts_by_provider` at line 1348 |
| `"projects"` key for binding lists | `internal/cli/manage.go:1475` | `runStatusAll` calls `WriteListWithKey(... "projects", ...)` at 1475 |
| JSON shape test pins `"accounts"` key | `internal/cli/extended_test.go:136-153` | Test constructs `newManageAccountCommand` (136), runs `--format json list codex` (141), asserts wantJSON starts with `{ "accounts": [...] }` (150) |
| Caller-owned list key | `internal/output/output.go:82-93,188-192` | `WriteListWithKey` 82-127 with payload built at 84-86 calling `listJSONKey` at 188-192 (defaults to `"items"` if empty) |
| Codex binding lookup + PrepareRuntime call | `internal/services/codex/service.go:155-185` | Cross-provider claude lookup 155-168; `PrepareRuntime` invocation 171-178 |
| Codex `ContainerRunRequest` builds with `prepared.Env` | `internal/services/codex/service.go:302-332` | `buildRequest` 302-333; `Env: prepared.Env` at line 315 |
| Codex runtime owns env keys | `internal/adapters/providers/codex/runtime.go:117-132` | map literal at 117-123 sets `CODEX_HOME, HOME, LOGNAME, TERM, USER`; conditional `CLAUDE_CONFIG_DIR` cross-mount at 125-132 |
| Codex service test asserts CODEX_HOME, HOME | `internal/services/codex/service_test.go:269-274` | Assertions at 269-273 |
| Codex test asserts CLAUDE_CONFIG_DIR cross-provider | `internal/services/codex/service_test.go:792-798` | Conditional `wantClaudeEnv` assertions at 792-799 |
| Claude PrepareRuntime call site | `internal/services/claude/service.go:162-194` | Cross-provider codex lookup 162-175; `PrepareRuntime` invocation 180-187 |
| Claude `ContainerRunRequest` builds with `prepared.Env` | `internal/services/claude/service.go:298-329` | `buildRequest` 298-330; `Env: prepared.Env` at line 311 |
| Claude runtime owns env keys | `internal/adapters/providers/claude/runtime.go:127-142` | map literal 127-133 sets `CLAUDE_CONFIG_DIR, HOME, LOGNAME, TERM, USER`; conditional `CODEX_HOME` cross-mount 135-142 |

All plan cites resolve.

## 2. Reserved-key set verification

The plan declares 6 reserved keys: `CODEX_HOME`, `CLAUDE_CONFIG_DIR`,
`HOME`, `LOGNAME`, `TERM`, `USER`.

Union of runtime-set keys across both providers:

- codex (`internal/adapters/providers/codex/runtime.go:117-132`): always
  `CODEX_HOME, HOME, LOGNAME, TERM, USER`; conditionally `CLAUDE_CONFIG_DIR`
  on cross-mount.
- claude (`internal/adapters/providers/claude/runtime.go:127-142`):
  always `CLAUDE_CONFIG_DIR, HOME, LOGNAME, TERM, USER`; conditionally
  `CODEX_HOME` on cross-mount.

Union = exactly the 6 keys the plan reserves. Correct.

## 3. `CLAUDE_CODE_OAUTH_TOKEN` re-verification (load-bearing)

The Round 2 plan claims this env var is NOT currently part of the
committed Claude launch path. Re-verify with `rg` across all internal Go
sources:

- Command: `rg -n 'CLAUDE_CODE_OAUTH_TOKEN' internal/ --type go`
- Result: 0 matches across all `internal/` Go files (production AND
  test). Stronger than the plan needs (plan only claims absence from
  production launch path).

Unit 14.5's claim "`CLAUDE_CODE_OAUTH_TOKEN` is NOT in the
container-launch env path at the time of this plan; it lives only in
host-side auth subprocess env" is correctly grounded. Future drops
that introduce the token into `ContainerRunRequest.Env` own the
reserved-key extension. Out-of-scope assertion holds.

## 4. Unit-by-unit completeness

### Unit 14.1 — schema

- state: todo
- blocked_by: none — atomic, foundational
- paths resolve: `internal/domain/repository.go` (45 lines today),
  `internal/adapters/sqlite/store.go` (existing Bootstrap to extend),
  `internal/adapters/sqlite/store_test.go` (existing migration suite)
- packages: `internal/domain`, `internal/adapters/sqlite`
- acceptance criteria yes/no-verifiable: upgrade to `user_version = 2`,
  idempotence on repeated bootstrap, duplicate-key support across two
  profiles, set/list/unset CRUD, rename stability via `Profile.ID`. Each
  is a discrete table-driven test assertion.

### Unit 14.2 — manage CRUD

- state: todo
- blocked_by: 14.1 (needs the repo contract)
- paths resolve: `internal/services/manage/service.go` (Store seam at
  21-25), `internal/services/manage/service_test.go` (existing
  table-driven pattern)
- packages: `internal/services/manage`
- acceptance yes/no-verifiable: missing account, reserved-key
  rejection (six discrete keys named), same key across two accounts,
  rename stability through `Profile.ID`, regex rejection for leading-
  digit / whitespace / hyphenated / dotted / control-character / empty
  keys; errors wrap the literal regex `^[A-Za-z_][A-Za-z0-9_]*$`

### Unit 14.3 — CLI subtree

- state: todo
- blocked_by: 14.2 (CLI calls service CRUD)
- paths resolve: `internal/cli/manage.go` (existing
  `newManageAccountCommand` 22-63), `internal/cli/manage_test.go`,
  `internal/cli/extended_test.go` (existing JSON pinning pattern at
  136-153)
- packages: `internal/cli`
- acceptance yes/no-verifiable: human + JSON `list` output, JSON
  top-level `"env"` key, malformed `KEY=VALUE`, reserved-key
  propagation, missing-account error, two-account separation.
- production registration site cross-check: `newManageAccountCommand`
  registered at exactly one production site (`internal/cli/root.go:130`).
  No double-registration risk inherited from pre-DROP_9 era.

### Unit 14.4 — codex merge

- state: todo
- blocked_by: 14.1 (needs env-map read)
- paths resolve: `internal/services/codex/service.go` (line 311 is
  current `Env: prepared.Env`), `internal/services/codex/service_test.go`
  (existing env assertions at 269-274 and 792-798)
- packages: `internal/services/codex`
- acceptance yes/no-verifiable: `executor.got.Env` contains account env,
  `CODEX_HOME, HOME` preserved, cross-provider `CLAUDE_CONFIG_DIR`
  preserved when present, reserved-key override attempts are no-ops.

### Unit 14.5 — claude merge

- state: todo
- blocked_by: 14.1 (parallel with 14.4)
- paths resolve: `internal/services/claude/service.go` (line 311 is
  current `Env: prepared.Env`), `internal/services/claude/service_test.go`
- packages: `internal/services/claude`
- acceptance yes/no-verifiable: `executor.got.Env` contains account env,
  `CLAUDE_CONFIG_DIR, HOME` preserved, cross-provider `CODEX_HOME`
  preserved when present.
- `CLAUDE_CODE_OAUTH_TOKEN` exclusion from reserved-key set is correctly
  scoped — see §3.

### `blocked_by` chain coherence

- 14.1 (none) → 14.2 → 14.3
- 14.1 → 14.4
- 14.1 → 14.5
- 14.2 ∥ 14.4 ∥ 14.5 after 14.1 closes; 14.3 after 14.2.
- No cycles. No orphan units. No `blocked_by` references missing units.

## 5. Drop-level acceptance criteria

Scope (line 14): per-account env-var map, arbitrary `key→value`, same
name across accounts with different values, threaded into
`ContainerRunRequest.Env` alongside CLAUDE_CONFIG_DIR/CODEX_HOME →
covered by 14.1 (`profile_id + env_key` composite key) + 14.4/14.5
(merge into prepared.Env clone, preserve runtime-owned keys).

CLI surface (line 16): `valv account env set/unset/list` — covered by
14.3.

Storage (line 18): plaintext-in-SQLite v0.1 with keychain deferred —
explicit, worklog follow-up.

DROP_13 forward-compat (line 20): generic-run primitive inherits same
`ContainerRunRequest.Env` threading — Unit 14.4/14.5 modify the field
directly, not provider-specific code paths.

## 6. Findings

no findings.

Verdict: pass.
