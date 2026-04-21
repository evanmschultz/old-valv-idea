# DROP_2 — DOMAIN PLUMBING PROVIDER CLAUDE

**State:** done
**Blocked by:** —
**Paths (expected):** `internal/domain/provider.go` (edit), `internal/domain/account_auth.go` (edit), `internal/domain/*_test.go` (edits + new tests), `internal/cli/manage.go` (edit — `allProviders`), plus any compile-sibling touches forced by the new enum value (`ParseProvider`, `DefaultHostProfile` stub, provider switch exhaustiveness)
**Packages (expected):** `internal/domain` (real edits — new enum value + stub), `internal/cli` (edit — `allProviders` extended), provider adapter packages NOT touched this drop (Codex unchanged, Claude adapter arrives in DROP_5)
**PLAN.md ref:** main/PLAN.md → DROP_2_DOMAIN_PLUMBING_PROVIDER_CLAUDE row
**Workflow:** main/drops/WORKFLOW.md
**Started:** 2026-04-19
**Closed:** 2026-04-20

## Scope

Extend the `Provider` domain type with `ProviderClaude` and thread compile-safe stubs through `ParseProvider`, `DefaultHostProfile`, `account_auth.go`, and `manage.go` `allProviders` per `VALV_CLAUDE_CODE_FOCUS_PLAN.md` §6.2 — no Docker work, no Claude CLI wiring, no provider-adapter package. Deliberately small: the goal is a tree that compiles and tests green with `ProviderClaude` as a recognized enum value plus whatever stub behavior is needed for every provider-dispatching switch to stay exhaustive without exploding at runtime for the new value. DROP_5 flips the stubs to real Claude adapter calls.

## Planner

**Scope confirmed:** domain-plumbing-only per `VALV_CLAUDE_CODE_FOCUS_PLAN.md` §6.2 (line 210). Add `ProviderClaude` enum value + `ParseProvider` branch, add compile-safe / test-safe branches to every provider-dispatch site that today matters, add sentinel-error stub for `DefaultHostProfile(ProviderClaude)`, extend `supportedProviders()` so the CLI aggregation surface includes the new provider. No Docker, no Claude adapter package (`internal/adapters/providers/claude/` stays absent until DROP_5), no schema work (DROP_3), no CLI `claude` command (DROP_6).

**Call-site audit** (evidence: committed-state file reads + `rg '\bProviderCodex\b'` fallback — Hylla `refs_find` on the `Provider` block returned zero edges and `hylla_search_keyword` on `ProviderCodex` only returned the declaration; recorded as Hylla misses in the worklog):

- `internal/domain/types.go:14-21` — `ParseProvider` switch. Requires explicit `case ProviderClaude` branch.
- `internal/domain/types_test.go:87` — existing test case `{name: "invalid", input: "claude", wantErr: true}` flips to `wantErr: false` when `ProviderClaude` is recognized. Must be updated or the test suite fails — this is load-bearing.
- `internal/cli/account_auth.go:37, 46, 55` — three switches (`ensureManagedAccountReady`, `logoutManagedAccount`, `loginManagedAccount`), each with `default: return nil`. Compile-safe today, but §6.2 requires explicit `case domain.ProviderClaude: return nil` so adding a future provider does not silently fall through.
- `internal/services/manage/service.go:151-162` — `DefaultHostProfile` switch. Requires `case domain.ProviderClaude` returning a sentinel-error per §6.2 (`errors.New("claude host profile not yet available")` or equivalent — to be flipped to `claudeprovider.DefaultHostProfile(...)` in DROP_5).
- `internal/cli/manage.go:984` (`supportedProviders()`) — the function `main/PLAN.md` container row calls "`allProviders`" is named `supportedProviders()` in committed code. Must add `domain.ProviderClaude` so `valv manage account list` (no-args form) iterates both providers.
- `internal/cli/operator_helpers.go:202-222` — `readAccountIdentity`'s `default` branch returns `"unknown" / "(unavailable)"`; `ProviderClaude` naturally routes through it and renders sanely. No change required this drop (DROP_5 will add a Claude-specific branch when the provider adapter lands).
- `internal/cli/manage.go:1082-1085` (`runManageUpdate`) and `internal/services/globalswitch/service.go:86` — both explicitly `if provider != domain.ProviderCodex { return error }`. `ProviderClaude` naturally falls into the error path, which aligns with DROP_3 / §7 open-question-4 scope deferral. No change required this drop.
- `internal/services/images/service.go:200-206` — defaults `provider` to `ProviderCodex` when empty, and conditionally attaches a Codex resolver. `ProviderClaude` passes through without a resolver; DROP_3 (6.3) is the slice that wires the Claude resolver. No change required this drop.

**Atomic decomposition:**

### Unit 2.1 — Add `ProviderClaude` enum value + parser branch + domain test updates

- `state`: `done`
- `paths`:
  - `internal/domain/types.go` (edit)
  - `internal/domain/types_test.go` (edit — flip the "claude" case from `wantErr: true` to `wantErr: false` with `want: ProviderClaude`, add a "trimmed claude" case for parity with the existing " Codex " case, and — if no other "invalid" case exists after the flip — add a fresh unambiguously-invalid input such as `"openai"` to keep the `wantErr: true` branch exercised)
- `packages`: `github.com/evanmschultz/valv/internal/domain`
- `blocked_by`: none (foundation; must land first)
- `acceptance`:
  - `ProviderClaude` is declared as a `Provider` constant in `internal/domain/types.go` with string value `"claude"`. Grep `\bProviderClaude\b` in `internal/domain/types.go` returns at least one match and `rg -n 'ProviderClaude\s+Provider\s*=\s*"claude"' internal/domain/types.go` returns a match.
  - `ParseProvider` accepts `"claude"`, `" claude "`, and `"Claude"` and returns `(ProviderClaude, nil)`. Grep `\bcase ProviderClaude\b` in `internal/domain/types.go` returns exactly one match.
  - `ParseProvider` still returns an error for inputs that are neither `"codex"` nor `"claude"` (verified via the retained / added `wantErr: true` table case).
  - `internal/domain/types_test.go` table in `TestParseProvider` contains a `want: ProviderClaude` case and no remaining `input: "claude", wantErr: true` case (grep `wantErr.*true.*claude|input.*"claude".*wantErr: true` returns zero matches in `internal/domain/types_test.go`).
  - `mage testPkg ./internal/domain` passes (gofumpt + race + coverage 70%).
  - Nothing outside `internal/domain/` is edited in this unit (grep confirmation: `git diff --name-only` for this unit returns only paths under `internal/domain/`).

### Unit 2.2 — Explicit `ProviderClaude` branches in `internal/cli/account_auth.go` + new unit test

- `state`: `done`
- `paths`:
  - `internal/cli/account_auth.go` (edit)
  - `internal/cli/account_auth_test.go` (edit — file already exists at 226 lines with Codex-path tests; extend with ProviderClaude table-driven coverage while preserving existing Codex coverage; builder must Read first, never Write-clobber)
- `packages`: `github.com/evanmschultz/valv/internal/cli`
- `blocked_by`: `2.1` (imports `domain.ProviderClaude` — must exist before this unit compiles)
- `acceptance`:
  - `ensureManagedAccountReady`, `logoutManagedAccount`, `loginManagedAccount` each contain an explicit `case domain.ProviderClaude:` branch returning `nil` before the `default` branch. Grep `\bcase domain\.ProviderClaude\b` in `internal/cli/account_auth.go` returns exactly three matches.
  - The `default:` branch in each of the three functions is retained (grep `\bdefault:\b` in `internal/cli/account_auth.go` returns three matches — catch-all for any future provider added without a compile-time check).
  - Existing `internal/cli/account_auth_test.go` is extended with table-driven `ProviderClaude` coverage for `ensureManagedAccountReady`, `logoutManagedAccount`, `loginManagedAccount`, each asserting `err == nil` when invoked with a throwaway `domain.Profile{Provider: domain.ProviderClaude}` (the functions must not dereference provider-specific state for `ProviderClaude` in the stub path). Existing Codex-path tests in the same file must remain present and passing — `git diff internal/cli/account_auth_test.go` must show additions only, no deletions of existing test functions.
  - Codex paths in the three switches remain behaviorally unchanged — all existing Codex-path tests in `internal/cli/account_auth_test.go` must still pass (`mage testPkg ./internal/cli` green is the gate).
  - `mage testPkg ./internal/cli` passes (gofumpt + race + coverage 70%).
  - Nothing outside `internal/cli/account_auth.go` + `internal/cli/account_auth_test.go` is edited in this unit.

### Unit 2.3 — `DefaultHostProfile` sentinel-error stub + extend `supportedProviders()` + manage-service test

- `state`: `done`
- `paths`:
  - `internal/services/manage/service.go` (edit — extend `DefaultHostProfile` switch at lines 151-162)
  - `internal/services/manage/service_test.go` (edit — add a `TestDefaultHostProfileClaudeReturnsSentinelError` that asserts `DefaultHostProfile(domain.ProviderClaude)` returns `HostProfileSpec{}` plus a non-nil error with a stable sentinel substring like `"not yet available"`)
  - `internal/cli/manage.go` (edit — extend the `supportedProviders()` return slice at line 984-986 to include `domain.ProviderClaude`)
- `packages`:
  - `github.com/evanmschultz/valv/internal/services/manage`
  - `github.com/evanmschultz/valv/internal/cli`
- `blocked_by`: `2.1` (imports `domain.ProviderClaude` from both touched packages)
- `acceptance`:
  - `DefaultHostProfile` in `internal/services/manage/service.go` contains an explicit `case domain.ProviderClaude:` branch that returns `HostProfileSpec{}` + a non-nil error. Grep `\bcase domain\.ProviderClaude\b` in `internal/services/manage/service.go` returns exactly one match.
  - The sentinel error message is stable enough to assert on in a test — a fixed substring such as `not yet available` appears in the returned error. Grep `"not yet available"|"not implemented"` (single literal chosen by builder, but must be stable) in `internal/services/manage/service.go` returns at least one match, and the test in `service_test.go` asserts on the same literal via `strings.Contains(err.Error(), <literal>)`.
  - The existing `default:` branch is removed or retained at the builder's discretion, but `ProviderClaude` must hit the explicit branch, not the default (verify by test: assert error message contains the stable sentinel substring, not the generic `"unsupported provider"` text from the default branch).
  - `supportedProviders()` in `internal/cli/manage.go` returns `[]domain.Provider{domain.ProviderCodex, domain.ProviderClaude}`. Grep `\bdomain\.ProviderClaude\b` in `internal/cli/manage.go` returns at least one match.
  - `valv manage account list` with no provider argument iterates both providers (covered indirectly by `mage testPkg ./internal/cli` — the `writeAccountsByProvider` integration path must stay green; existing no-arg `list` coverage in `manage_test.go` is permitted to be extended, not required).
  - `mage testPkg ./internal/services/manage` passes.
  - `mage testPkg ./internal/cli` passes.
  - `mage test` (root-level) passes — this is the drop-end gate and must be confirmed in Phase 6 before closing DROP_2, but per-unit Phase 5 QA only needs the two package-scoped targets above.
  - Nothing outside the three listed files is edited in this unit.

**Unit ordering + blocking rationale:**
- 2.1 is the foundation — it introduces the symbol every other unit imports. No sibling can compile without it.
- 2.2 and 2.3 are parallel consumers of 2.1. They edit disjoint files in disjoint packages — 2.2 touches only `internal/cli/account_auth.go` + new `account_auth_test.go`; 2.3 touches `internal/services/manage/service.go`, `internal/services/manage/service_test.go`, and `internal/cli/manage.go`. No file-footprint collision between 2.2 and 2.3.
- Both 2.2 and 2.3 edit files in `internal/cli` — but different files (`account_auth.go` vs `manage.go`). Go package lock is at the package level for mage test determinism; if the orch runs 2.2 and 2.3 builders truly in parallel, both will edit files in the same package and one will need to rebase. Recommend sequential: 2.1 → 2.2 → 2.3 to keep the tree linear. Sibling `blocked_by`: 2.3 `blocked_by: 2.2` for this reason.

**No-touch scope-guard list** (must NOT be edited in this drop; independent falsification check):
- `internal/adapters/providers/codex/**` — no Codex adapter changes in this drop.
- `internal/adapters/providers/claude/**` — forbidden directory (DROP_5 scope).
- `internal/adapters/sqlite/store.go` — schema migration is DROP_3 scope.
- Any `valv claude` CLI command registration (`internal/cli/root.go`, new `internal/cli/claude.go`) — DROP_6 scope.
- `internal/services/images/service.go` — Claude Docker image is DROP_3 scope.
- `internal/services/globalswitch/service.go` — Claude support explicitly deferred (§7 open-question 4).

## Notes

- `VALV_CLAUDE_CODE_FOCUS_PLAN.md` §6.2 is the authoritative scope source; read it before decomposing.
- Stubs for `ProviderClaude` must be compile-safe AND test-safe: any existing test that enumerates `allProviders` or iterates `Provider` values must still pass, either by accepting the new value or by being extended with a table-driven entry. Planner should audit existing tests in `internal/domain` + `internal/cli` that enumerate providers and decide which need extension.
- `DefaultHostProfile` for Claude: this drop only needs a compile-safe stub that returns something reasonable (e.g. a zero-or-placeholder `domain.Profile` value, or an error like "claude host profile not implemented until DROP_5"). Planner should decide which form is less risky for Phase 5 QA — erroring is more honest but may break any call path that currently assumes `DefaultHostProfile` always succeeds; returning a placeholder may hide bugs. Default: error return, with every caller updated to propagate the error.
- Nothing in `internal/adapters/providers/` gets a `claude/` subdir in this drop — that is DROP_5 scope per the container plan.
- `valv-claude:dev` Docker image is NOT built in this drop — that is DROP_4 scope.
- Schema migration (`project_bindings` composite key) is NOT touched in this drop — that is DROP_3 scope.
