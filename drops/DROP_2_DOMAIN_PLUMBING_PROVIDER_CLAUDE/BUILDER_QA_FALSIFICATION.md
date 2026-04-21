# DROP_2 — Builder QA Falsification

Append a `## Unit N.M — Round K` section per falsification review. See `main/drops/WORKFLOW.md` § "Phase 5 — Build QA (per unit)".

## Unit 2.1 — Round 1

**Verdict:** pass

- **Reviewer:** go-qa-falsification-agent
- **Target commit:** 300984b `feat(domain): add ProviderClaude enum value and parser branch`
- **Files reviewed:** `internal/domain/types.go`, `internal/domain/types_test.go`
- **Mage targets run:** `mage testPkg ./internal/domain` — 26 tests pass, 85.2% coverage, race-clean

### Attacks attempted

1. **mitigated** — **Silent acceptance of `ProviderClaude` by call sites before Units 2.2/2.3 guard them.** Walked every `ProviderCodex` reference in `internal/` (55 source-file hits across `internal/cli`, `internal/services`, `internal/adapters`, `internal/tui`, `internal/domain`). Every provider-dispatching site falls into one of three safe categories: (a) strict `if provider != ProviderCodex { return error }` that now cleanly errors for Claude (`globalswitch/service.go:86`, `cli/manage.go:1083`, `services/codex/service.go:172,223,234`); (b) `switch provider { case ProviderCodex: ...; default: <sane-default> }` where the default is either a clean error (`services/manage/service.go:159-160 DefaultHostProfile` returns `"unsupported provider"` error) or a benign no-op (`cli/account_auth.go:39,48,57` `default: return nil`; `cli/operator_helpers.go:216-221 readAccountIdentity` returns `"unknown"/"(unavailable)"`); (c) defaults like `services/images/service.go:201-202,204-205` that gate Codex-only logic (`resolver` only attached when `provider == ProviderCodex`) and guard downstream usage with an explicit nil-check (`service.go:294 EnsureLatest` returns `"latest-version resolver is required"` error). No naked `if codex { A } else { B }` pattern with a broken else branch exists. Unit 2.2 and 2.3 are scope-deferred refinements (explicit branches + `supportedProviders()` extension), not latent-bug fixes. Additionally, no Claude profile can actually be created through the CLI today because `DefaultHostProfile(ProviderClaude)` errors and `supportedProviders()` still returns `[]{ProviderCodex}`, so any path that would consume a stored Claude profile is unreachable.

2. **mitigated** — **Case-insensitivity gap for uppercase `"CLAUDE"`.** `ParseProvider` applies `strings.ToLower(strings.TrimSpace(value))` before the switch (`types.go:16`), then compares against the lowercase-literal `ProviderClaude = "claude"` constant. `"CLAUDE"` → `"claude"` → matches. The test table covers `"claude"` (plain) and `" Claude "` (trimmed + mixed case); the latter exercises both `TrimSpace` and `ToLower`. A dedicated `"CLAUDE"` all-caps row would add no new code coverage because it exercises the same normalization path.

3. **mitigated** — **Codex-path coverage regression.** `git diff HEAD~1 HEAD` on `types.go` is purely additive (+2 lines: new const + new case). On `types_test.go`: the `{input:"claude", wantErr:true}` row flipped to `{want: ProviderClaude}` (required by the acceptance criteria since `ProviderClaude` is now valid), the `" Codex "` and `"codex"` Codex-path rows are untouched, a new `"claude trimmed"` row was added, and a new `{input:"openai", wantErr:true}` row preserves the error-branch coverage that the flipped "claude" row used to provide. `mage testPkg ./internal/domain` reports 26 tests (up from 24), all pass, 85.2% coverage — no regression. All test additions in `TestParseProvider`; `TestParseOutputFormat` and `TestParseOutputStyle` untouched.

4. **mitigated** — **New branches not actually exercised by `mage testPkg ./internal/domain`.** The `case ProviderClaude:` branch is hit by both `{input:"claude", want: ProviderClaude}` and `{input:" Claude ", want: ProviderClaude}` test rows. The `default` branch is still exercised by `{input:"openai", wantErr:true}`. Coverage is 85.2% against a 60% floor. Branch coverage confirmed by the non-zero pass count for the new rows (24 → 26) and the coverage floor gate passing.

5. **mitigated** — **YAGNI pressure on redundant `"claude trimmed"` test row.** The existing `" Codex "` case already exercises `strings.TrimSpace` + `strings.ToLower`, so `" Claude "` adds no unique normalization coverage. However, the plan's Unit 2.1 acceptance line (`internal/domain/types_test.go ... add a "trimmed claude" case for parity with the existing " Codex " case`) explicitly required it. Noted as scope-accepted redundancy per plan, not blocking.

6. **mitigated** — **Hylla staleness masking a real call-site affected by the new enum value.** Hylla ingest for Valv is drop-end only per `main/CLAUDE.md` § "Hylla Baseline". For this review I relied on `Grep` across the checkout (55 `ProviderCodex` hits across `.go` files) rather than Hylla, which is the correct substrate for uncommitted/just-committed deltas. No call site was missed — the review matched the planner's own audit (`PLAN.md` lines 22-29) plus confirmed the two additional call sites the planner listed as `no change required this drop` (`services/images/service.go:200-206`, `cli/operator_helpers.go:202-222`) remain safe. No Hylla miss in the sense the project tracks it (the builder correctly noted N/A in the worklog).

7. **mitigated** — **Scope-guard violation: edits outside `internal/domain/`.** `git diff --name-only HEAD~1 HEAD` returns exactly four paths — `internal/domain/types.go`, `internal/domain/types_test.go`, plus the two drop-tracking markdown files `drops/DROP_2_.../BUILDER_WORKLOG.md` and `drops/DROP_2_.../PLAN.md`. The markdown changes are the builder's own state update (Planner section Unit 2.1 `state: done` + round-1 worklog append), which the Agent Spawn Contract explicitly authorizes. No Go source edits outside `internal/domain/`.

8. **mitigated** — **Nil-interface / error-swallow / goroutine / context attack surfaces.** Unit 2.1 is a pure enum + switch edit with zero interface surface, no error handling path beyond the existing `fmt.Errorf(... %q ...)` wrapping (which is retained and still reports the offending value), no goroutines, no context usage. Not applicable.

### Counterexamples

None. No CONFIRMED counterexample produced after eight attack passes.

### Hylla Feedback

None — Hylla was not queried for this review. The attack surface (enum addition + call-site blast radius) is evaluable directly from `git diff`, `Grep` over the just-committed tree, and targeted `Read` of the specific call sites the planner's audit already enumerated. Drop-end ingest has not yet run, so any Hylla query would be answering from pre-DROP_2 state anyway. No fallback miss worth recording.

## Unit 2.2 — Round 1

**Verdict:** pass

- **Reviewer:** go-qa-falsification-agent
- **Target commit:** 15df905 `feat(cli): add ProviderClaude stub branches in account auth dispatchers`
- **Files reviewed:** `internal/cli/account_auth.go`, `internal/cli/account_auth_test.go`
- **Mage targets run:** `mage testPkg ./internal/cli` — 101 tests pass, 71.9% coverage (above the 60% mage target floor; AGENTS.md 70% drop-end gate also cleared), race-clean

### Attacks attempted

1. **mitigated** — **`case domain.ProviderClaude: return nil` is a silent no-op smell.** Confirmed intentional stub. `PLAN.md` line 14 Scope ("the goal is a tree that compiles and tests green with `ProviderClaude` as a recognized enum value plus whatever stub behavior is needed for every provider-dispatching switch to stay exhaustive without exploding at runtime for the new value. DROP_5 flips the stubs to real Claude adapter calls") and line 96 No-touch list (`internal/adapters/providers/claude/**` explicitly forbidden this drop; DROP_5 scope) both authorize the stub. The Unit 2.2 acceptance criterion line 58 spells out exactly this shape: "each contain an explicit `case domain.ProviderClaude:` branch returning `nil` before the `default` branch." Three dispatchers × one-liner Claude branch, matching the spec verbatim. Not a latent bug; a scope-deferred placeholder with a fixed removal date (DROP_5).

2. **mitigated** — **Tests assert against a shared fake path that would pass even if a dispatcher never routed the Claude case.** Read `TestProviderClaudeAccountAuthStubs` directly. Each subtest builds a fresh `cobra.Command`, calls **the dispatcher function directly** (`ensureManagedAccountReady`, `logoutManagedAccount`, `loginManagedAccount`), and asserts `err == nil`. No stub runner is installed for the Claude path — if the dispatcher silently fell through to `default: return nil` that would also return nil (same observable), BUT grep confirms the dispatchers each contain exactly one `case domain.ProviderClaude:` branch (line 39, 50, 61) **before** the `default:` branch (line 41, 52, 63). Go's `switch` evaluates cases top-to-bottom, so a `ProviderClaude` input hits line 39/50/61, not the default. Additionally — critical asymmetry — if a future edit removed the `ProviderCodex` case, the Codex-path tests would still pass (they'd hit `default: return nil` unexpectedly — a real weak-spot, but one that predates this unit and is explicitly out of scope per §6.2). For this unit's claim (the new Claude case executes), the assertion is correct as far as its observable goes: with `return nil` as the stub's specified behavior, the test can only distinguish "dispatcher returns nil for Claude" from "dispatcher panics / returns non-nil." The test confirms the former, which is the claim. Weak but sufficient for the specified stub contract.

3. **mitigated** — **Nil context or nil runner deref on the Claude path.** Read account_auth.go lines 35-66. On the `case domain.ProviderClaude:` arm the function returns `nil` immediately — no access to `cmd.Context()`, `codexAccountAuthFromContext`, `account.HomePath`, `account.Name`, or any runner interface. `Profile` is a struct (not a pointer — confirmed at `internal/domain/model.go:21-27`), so even if the caller passed `domain.Profile{}` with zero-value fields, the dispatcher never dereferences them on the Claude arm. `cmd.Context()` is also never called on the Claude arm. The test nonetheless sets `cmd.SetContext(context.Background())` defensively (line 213), so if a future refactor moved code above the Claude case, the test would still be safe. No nil deref risk.

4. **mitigated** — **Future-provider input routing through `case domain.ProviderClaude` by mistake (e.g. string coercion).** `Provider` is `type Provider string` (`internal/domain/types.go:8`), so coercion is by value. Only a string-valued `"claude"` matches `ProviderClaude = "claude"`. Any other enum value (future `ProviderOpenAI`, etc.) would have a different string literal and would land in the `default:` branch, not the Claude case. Go `switch` does not fall through between cases without an explicit `fallthrough` keyword, which is absent here (confirmed in the diff). The `default:` branch is still present in all three dispatchers (grep confirms 3 matches) — a future unknown provider is caught by `default: return nil`, which is the same pre-existing catch-all behavior. No routing mistake possible.

5. **mitigated** — **Coverage floor attack: `internal/cli` coverage dropped below 70% after the edits.** `mage testPkg ./internal/cli` reports 71.9% coverage across 101 tests, all passing. The mage target's own floor is 60%; AGENTS.md § 11 states 70% per-package floor applies at drop-end via `mage test`. 71.9% clears both. The builder-reported number matches my run exactly, so no drift between build and QA.

6. **mitigated** — **Goroutine leak / race under `-race` on concurrent tests.** `TestProviderClaudeAccountAuthStubs` uses `t.Parallel()` at both the outer test and each subtest (lines 182, 210). The dispatcher functions contain zero goroutines on the Claude path (pure value-return). `mage testPkg ./internal/cli` runs with `-race -count=1`; output reports no race data (the mage-reported `[PKG PASS]` implies `-race` clean — the Magefile's `Test` function fails on any race). No goroutine leak surface.

7. **mitigated** — **Scope-guard violation: deleted or renamed pre-existing Codex test functions.** `git show 15df905^:internal/cli/account_auth_test.go` vs `git show 15df905:internal/cli/account_auth_test.go`, filtered to `^func Test`, show the same 9 pre-existing Codex test functions in both (`TestEnsureCodexAccountReadyRejectsNonTTYLogin`, `TestEnsureCodexAccountReadySkipsAuthenticatedAccount`, `TestEnsureCodexAccountReadySkipsWhenTestBypassIsSet`, `TestShouldSkipHostCodexLoginCheck`, 4× `TestSystemCodexAccountAuthRunner*`, `TestSystemCodexAccountAuthRunnerLogoutUsesCODEXHOME`) plus the single new `TestProviderClaudeAccountAuthStubs` on the post side. `git diff --stat 15df905^..15df905` shows +6/−0 lines in `account_auth.go` and +45/−0 lines in `account_auth_test.go` — purely additive on both files. No renames, no deletions. `git log --diff-filter=D` on this commit returns nothing.

8. **mitigated** — **Error swallowing.** Every one of the three dispatchers had a pre-existing `default: return nil` catch-all before this unit. Adding `case domain.ProviderClaude: return nil` does not introduce a new error-swallow path — it splits one existing `nil`-return path into two identical `nil`-return paths. The `ensureCodexAccountReady`, `loginCodexAccount`, `logoutCodexAccount` implementations below are unchanged and still wrap their errors with `fmt.Errorf("...: %w", err)`. No regression.

9. **mitigated** — **Footprint violation outside the two declared paths.** `git diff --stat 15df905^..15df905` shows exactly four files: the two declared Go files (`internal/cli/account_auth.go`, `internal/cli/account_auth_test.go`) plus the two drop-tracking markdown files (`drops/DROP_2_.../PLAN.md` with unit state flip to `done`, `drops/DROP_2_.../BUILDER_WORKLOG.md` with Round 1 append). Markdown state-flip + worklog append are authorized by WORKFLOW.md's Agent Spawn Contract. No stray Go edits.

10. **mitigated** — **Raw `go` command usage / mage bypass.** Builder worklog (Unit 2.2 Round 1) reports only `mage testPkg ./internal/cli`. No `go test`, `go build`, `go vet`, `gofumpt`, or other raw go-tool invocation appears. Mage-first discipline observed.

### Counterexamples

None. No CONFIRMED counterexample produced after ten attack passes.

### Hylla Feedback

None — Hylla was not queried for this review. Unit 2.2 is a localized edit inside two files with a 6-line source diff; `git diff`, direct `Read`, and `Grep` over the just-committed tree (plus `mage testPkg` to exercise the runtime path) covered the full attack surface. Drop-end Hylla ingest has not yet run, so any Hylla query would be answering from pre-DROP_2 state and would not reflect the new `case domain.ProviderClaude:` arms. No fallback miss worth recording.

## Unit 2.3 — Round 1

- **Reviewer:** go-qa-falsification-agent
- **Target commit:** 26b5a58 `feat(manage): stub DefaultHostProfile claude branch and list provider`
- **Verdict:** pass

### Attempted counterexamples

1. **REFUTED** — **Sentinel error at user surface via `valv manage account add claude` / `--provider claude`.** `runManageAccountAdd` at `internal/cli/manage.go:462-491` routes `ProviderClaude` through `DefaultHostProfile(provider)` (line 479, when `name == "" && homePath != ""`) or `CreateDefaultHostProfile(ctx, provider)` (line 485, when `name == "" && homePath == ""`). Both now return the new sentinel error. Final wrapped message at the user surface is `manage account add: resolve default host profile for provider "claude": not yet available` — unambiguous, names the provider, communicates temporariness. Acceptable for the stub phase (DROP_5 flips to real). The third branch (line 488, `name != ""`) calls `CreateProfile` directly and does not hit `DefaultHostProfile`, so `valv manage account add claude explicit-name --home /some/path` would succeed and persist a Claude profile — but that's a pre-existing code path and consistent with §6.2 stub discipline; not a regression.
2. **REFUTED** — **Internal callers of `DefaultHostProfile` that discard the error.** Three sites in `internal/services/manage/service.go` swallow the error via `_`: line 374 (`CleanupDuplicateAliases`), line 450 (`seedProfileConfig`), line 485 (`presentableProfiles`). For `ProviderClaude`, each receives `hostSpec == HostProfileSpec{}` (all zero fields). Tried to construct a counterexample where a zero `HostProfileSpec` corrupts presentation / cleanup:
   - `CleanupDuplicateAliases(ProviderClaude)`: `shouldPreferPresentableProfile` compares `candidate.HomePath == hostSpec.HomePath` — would match any profile with empty `HomePath`. But the new `domain.Profile.HomePath` field is non-empty by construction (`CreateProfile` validates, `NewProfile` rejects empty), so no profile can match. Moreover, `ListProfilesByProvider(ctx, ProviderClaude)` returns empty slice today because no persistence path creates a Claude profile — the `for _, group := range grouped` loop has zero iterations. Unreachable corruption.
   - `seedProfileConfig(profile)` for a `ProviderClaude` profile: error is swallowed with `return nil` at line 451-453 — existing behavior preserved. Harmless skip is exactly the documented intent.
   - `presentableProfiles(ProviderClaude, ...)`: same `HomePath == ""` trap, same unreachability because no Claude profile exists.
   All three are latent footguns, but none reachable in DROP_2 state. Documented here for DROP_5 awareness.
3. **REFUTED** — **Sentinel substring stability.** `"not yet available"` is the single literal; grep in `internal/services/manage/service.go` returns one match at line 160, test at `service_test.go:160-162` asserts on the same literal via `strings.Contains`. No other downstream consumer (no `log | grep`, no TUI decoder, no other test) depends on that substring. Builder's choice of literal is free to drift in DROP_5 without cross-package breakage.
4. **REFUTED** — **`default:` retention in `DefaultHostProfile`.** Line 161 of `service.go` retains `default: return HostProfileSpec{}, fmt.Errorf(... "unsupported provider", provider)`. A future `ProviderFoo` enum value would route through this default, not through an implicit nil-return or panic. Correct exhaustiveness posture.
5. **REFUTED** — **`valv manage account list` (no-args) rendering.** `writeAccountsByProvider` at `internal/cli/manage.go:914-951` iterates both providers via `supportedProviders()`. For human/plain output, two sections render (`codex accounts` then `claude accounts`), each calls `output.WriteListWithKey` — `ListProfiles(ctx, ProviderClaude)` returns an empty profile slice, which renders as `(none)` (evidenced by `TestManageAccountListShowsEmptyState` at line 159-168 exercising that path for Codex). For JSON output, the `accounts_by_provider` array gains a second entry with empty `Items`. No existing test uses `strings.Contains` that would flip on this addition (reviewed `TestManageAccountListWithoutProviderGroupsByProvider` at line 112-126 — positive-only assertions; `TestManageAccountListJSONUsesCommandKey` at line 128-157 uses `account list codex` explicit provider, not no-args, so the JSON exact-match is not affected). Coverage of the new branch is indirectly proven by the 101-tests-pass, 72.0%-coverage `mage testPkg ./internal/cli` result.
6. **REFUTED** — **Coverage gates.** Ran both targets locally:
   - `mage testPkg ./internal/services/manage` — 23 pass, 76.4% coverage (AGENTS.md 70% floor met; `mage testPkg` internal floor is 60%, drop-end `mage test` floor is 70%).
   - `mage testPkg ./internal/cli` — 101 pass, 72.0% coverage.
   Builder's worklog numbers reproduced exactly.
7. **REFUTED** — **Scope bleed outside declared paths.** `git diff 76d6bc7..26b5a58 --name-only` returns 5 files: 3 source (`internal/services/manage/service.go`, `internal/services/manage/service_test.go`, `internal/cli/manage.go`) + 2 docs (`PLAN.md` state flip + `BUILDER_WORKLOG.md` entry). No touches to `internal/adapters/providers/claude/**`, `internal/services/images/service.go`, `internal/services/globalswitch/service.go`, `internal/adapters/sqlite/store.go`, `internal/cli/root.go`, or any new `internal/cli/claude.go`. No-touch scope guard list clean.

### Blocking findings

None.

### Mitigated / advisory notes

- **Latent `HostProfileSpec{}` trap for future Claude profiles** (from attack 2). Today unreachable because no persistence path creates a `ProviderClaude` profile. When DROP_3 (schema) / DROP_5 (adapter) land, `shouldPreferPresentableProfile` will start receiving real Claude profiles and the three `_ = err` sites will suddenly have a zero `HostProfileSpec` to compare against — that's when the drift shows up. Not a DROP_2 bug; flagging so DROP_5 QA catches the flip-over.
- **Third `runManageAccountAdd` branch (line 488, explicit `name`) does not route through `DefaultHostProfile`.** `valv manage account add claude my-name --home /some/path` would call `CreateProfile` and succeed today, persisting a Claude profile. §6.2 does not list this as a stub gate — consistent with the plan's scope ("compile-safe and test-safe for every *provider-dispatching switch*"); not a regression.

### Hylla Feedback

None — Hylla was not queried for this review. Unit 2.3 is a 4-line source diff across three files, plus a 20-line test. `git diff`, direct `Read`, and `Grep` over the committed tree (plus `mage testPkg` for runtime proof) covered the full attack surface. Drop-end Hylla ingest has not yet run, so any Hylla query would be answering from pre-DROP_2 state and would not reflect the new `case domain.ProviderClaude:` arm or the extended `supportedProviders()` — no fallback miss worth recording.
