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
