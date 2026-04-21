# DROP_2 — Builder QA Proof

Per-unit QA Proof results. Append a `## Unit N.M — Round K` section per build-QA round. See `main/drops/WORKFLOW.md` § "Phase 5 — Build-QA (per unit)".

## Unit 2.1 — Round 1

**Verdict:** pass

**HEAD commit reviewed:** `300984b feat(domain): add ProviderClaude enum value and parser branch`

**Diff scope:** `git diff --name-only HEAD~1..HEAD` returns `drops/DROP_2_DOMAIN_PLUMBING_PROVIDER_CLAUDE/BUILDER_WORKLOG.md`, `drops/DROP_2_DOMAIN_PLUMBING_PROVIDER_CLAUDE/PLAN.md`, `internal/domain/types.go`, `internal/domain/types_test.go` — all within the unit's declared `paths` (`internal/domain/*`) plus the drop's orchestration markdown. No out-of-scope Go edits.

### Findings

1. **`ProviderClaude` constant exists with value `"claude"`** — pass. `internal/domain/types.go:12` declares `ProviderClaude Provider = "claude"` within the same `const` block as `ProviderCodex`. `rg -n 'ProviderClaude\s+Provider\s*=\s*"claude"'` matches line 12.
2. **`ParseProvider` has exactly one `case ProviderClaude` branch** — pass. `internal/domain/types.go:19` adds `case ProviderClaude: return normalized, nil` between the `ProviderCodex` branch and `default`. `rg -n '\bcase ProviderClaude\b'` across `internal/domain/` returns exactly one match (types.go:19).
3. **`ParseProvider` still errors on non-codex/non-claude inputs** — pass. The `default` branch at `internal/domain/types.go:21-22` still returns `fmt.Errorf("parse provider %q: unsupported value", value)`. The `TestParseProvider` table retains a `wantErr: true` case (`{name: "invalid", input: "openai", wantErr: true}` at `types_test.go:89`) which exercised the default branch during the mage run (26 tests passed).
4. **Test table updated correctly** — pass. `TestParseProvider` table in `internal/domain/types_test.go:84-90` contains:
   - `{name: "codex", input: "codex", want: ProviderCodex}` (line 85)
   - `{name: "trimmed", input: " Codex ", want: ProviderCodex}` (line 86)
   - `{name: "claude", input: "claude", want: ProviderClaude}` (line 87) — required `want: ProviderClaude` case present.
   - `{name: "claude trimmed", input: " Claude ", want: ProviderClaude}` (line 88) — additional trim-parity case as planned.
   - `{name: "invalid", input: "openai", wantErr: true}` (line 89) — preserves the error branch.
   - `rg -n 'wantErr.*true.*claude|input.*"claude".*wantErr'` returns zero matches under `internal/domain/` — no stale `{input: "claude", wantErr: true}` rows remain.
5. **`mage testPkg ./internal/domain` passes** — pass. Run from `main/`. Output: `[PKG PASS] github.com/evanmschultz/valv/internal/domain (1.21s)`, 26 tests passed, 0 failed, coverage **85.2 %** on `github.com/evanmschultz/valv/internal/domain` (well above the 60.0 % `mage testPkg` threshold and the project's 70 % per-package floor from AGENTS.md § 11). gofumpt check is implicit in mage's per-package target and did not complain.
6. **Scope clean** — pass. Only four files touched in HEAD; the two Go files sit under `internal/domain/` (matching the unit's declared `paths`), and the two markdown files are the drop's own orchestration artifacts. No edits to `internal/cli`, `internal/services`, `internal/adapters`, `cmd/valv`, or any forbidden directory (`internal/adapters/providers/claude/**`, etc.).

### Notes

- Builder's `BUILDER_WORKLOG.md` round-1 entry matches the committed diff exactly (files touched, mage target, coverage %).
- Style alignment: `ParseProvider` now mirrors the two-case explicit pattern rather than the collapsed `case X, Y:` style used by `ParseOutputFormat` / `ParseOutputStyle` elsewhere in `types.go`. Both styles are idiomatic Go and the explicit form is clearer for a 2-element enum with two distinct consumers downstream — not a finding.
- No Hylla queries needed; fully verifiable from the small committed diff plus a mage run.

### Hylla Feedback

N/A — task touched Go files only but was verifiable via direct reads of the tiny committed diff plus one mage command; no Hylla queries issued, no fallback required.

## Unit 2.2 — Round 1

**Verdict:** pass

**HEAD commit reviewed:** `15df905 feat(cli): add ProviderClaude stub branches in account auth dispatchers`

**Diff scope:** `git diff --name-only HEAD~1..HEAD` returns `drops/DROP_2_DOMAIN_PLUMBING_PROVIDER_CLAUDE/BUILDER_WORKLOG.md`, `drops/DROP_2_DOMAIN_PLUMBING_PROVIDER_CLAUDE/PLAN.md`, `internal/cli/account_auth.go`, `internal/cli/account_auth_test.go` — all within the unit's declared `paths` (`internal/cli/account_auth.go` + `internal/cli/account_auth_test.go`) plus the drop's orchestration markdown. No out-of-scope Go edits.

### Findings

1. **Exactly three `case domain.ProviderClaude:` branches in `internal/cli/account_auth.go`** — pass. `rg -n '\bcase domain\.ProviderClaude\b' internal/cli/account_auth.go` returns three matches at lines 39, 50, 61 — one per dispatcher (`ensureManagedAccountReady`, `logoutManagedAccount`, `loginManagedAccount` respectively). Each branch returns `nil` (stub per §6.2 compile-safe discipline, not a call into Codex-shaped code — falsification check clean).
2. **Exactly three `default:` branches retained in `internal/cli/account_auth.go`** — pass. `rg -n '\bdefault:\b' internal/cli/account_auth.go` returns three matches at lines 41, 52, 63 — each immediately after the new `ProviderClaude` branch inside its respective switch. All three still return `nil`, preserving the pre-existing catch-all for any future provider added without an exhaustive-switch check.
3. **New test covers all three dispatchers with `ProviderClaude` and asserts `err == nil`** — pass. `TestProviderClaudeAccountAuthStubs` in `internal/cli/account_auth_test.go:181-226` uses a table (`dispatchers` slice) with three entries named `ensureManagedAccountReady`, `logoutManagedAccount`, `loginManagedAccount`, each wrapping the real dispatcher call. Subtest body builds `domain.Profile{Provider: domain.ProviderClaude}`, invokes `tc.call(cmd, account)`, and calls `t.Fatalf` unless the returned error is nil — exact shape required by the acceptance criterion.
4. **Test diff is additions-only** — pass. `git diff HEAD~1..HEAD -- internal/cli/account_auth_test.go` shows a single `@@` hunk with 45 `+` lines and zero `-` lines. `TestProviderClaudeAccountAuthStubs` is inserted between `TestSystemCodexAccountAuthRunnerLogoutUsesCODEXHOME` and the `installFakeHostCodex` helper. No existing test function is modified or removed.
5. **`mage testPkg ./internal/cli` passes with coverage recorded** — pass. Ran `mage testPkg ./internal/cli` from `main/`. Result: `[PKG PASS] github.com/evanmschultz/valv/internal/cli (186.12s)`, **101 tests passed / 0 failed / 0 skipped**, coverage **71.9 %** on `github.com/evanmschultz/valv/internal/cli` (above the `mage testPkg` 60.0 % floor, also above AGENTS.md § 11's 70 % per-package drop-end gate). gofumpt format check (implicit in mage) did not complain. Matches the worklog's reported 71.9 %.
6. **Scope clean — only the allowed files** — pass. `git diff --name-only HEAD~1..HEAD` returns exactly four entries: the two Go files inside the unit's declared paths (`internal/cli/account_auth.go`, `internal/cli/account_auth_test.go`) plus the two drop-orchestration markdown files (`PLAN.md`, `BUILDER_WORKLOG.md`). No edits to `internal/domain/**`, `internal/services/**`, `internal/adapters/**`, `cmd/valv/**`, or any of the no-touch scope-guard directories (`internal/adapters/providers/claude/**`, etc.).

### Notes

- Falsification sanity pass: each of the three Claude branches returns `nil` (stub), not a call into `ensureCodexAccountReady` / `logoutCodexAccount` / `loginCodexAccount` — no copy-paste dispatch bug. Codex-path bodies in lines 37-38, 48-49, 59-60 are byte-for-byte unchanged relative to HEAD~1 per the diff.
- Test imports unchanged — `bytes`, `context`, `testing`, `cobra`, `domain` were already imported before this change (the diff has no `+` lines in the import block), so the new test compiles without import churn as the worklog claims.
- Codex-path tests in `account_auth_test.go` remain intact and were exercised during the mage run (101 tests passed; the file's existing Codex coverage is inside that count).
- Unit 2.3 is still `todo` — this review covers unit 2.2 only; DROP_2 cannot close until 2.3 lands and passes build-QA plus Phase 6 drop-end verification.

### Hylla Feedback

N/A — task touched Go files only but was fully verifiable via the small committed diff plus one mage run; no Hylla queries issued, no fallback required.

## Unit 2.3 — Round 1

**Verdict:** pass

**HEAD commit reviewed:** `26b5a58 feat(manage): stub DefaultHostProfile claude branch and list provider`

**Diff scope:** `git diff --name-only HEAD~1..HEAD` returns `drops/DROP_2_DOMAIN_PLUMBING_PROVIDER_CLAUDE/BUILDER_WORKLOG.md`, `drops/DROP_2_DOMAIN_PLUMBING_PROVIDER_CLAUDE/PLAN.md`, `internal/cli/manage.go`, `internal/services/manage/service.go`, `internal/services/manage/service_test.go` — exactly the unit's declared `paths` plus orchestration markdown.

### Findings

1. **Exactly one `case domain.ProviderClaude:` in `internal/services/manage/service.go`** — pass. Single match at line 159, between `ProviderCodex` (line 153) and retained `default:` (line 161). Returns `HostProfileSpec{}` plus wrapped error.
2. **Sentinel substring `"not yet available"` stable** — pass. Line 160: `fmt.Errorf("resolve default host profile for provider %q: not yet available", provider)`. Literal in format string, not runtime-derived.
3. **`supportedProviders()` returns both providers** — pass. `internal/cli/manage.go:984-986` returns `[]domain.Provider{domain.ProviderCodex, domain.ProviderClaude}`.
4. **`TestDefaultHostProfileClaudeReturnsSentinelError` exists with all three assertions** — pass. `internal/services/manage/service_test.go:144-163`. Asserts `err != nil`, `spec == HostProfileSpec{}`, `strings.Contains(err.Error(), "not yet available")`.
5. **`mage testPkg ./internal/services/manage` passes** — pass. 23 tests / 0 failures, coverage **76.4 %** (above 60 % mage floor + 70 % AGENTS.md gate).
6. **`mage testPkg ./internal/cli` passes** — pass. 101 tests / 0 failures, coverage **72.0 %**.
7. **Scope clean** — pass. Five files touched: three allowed Go + two drop markdown. No out-of-scope edits.

### Notes

- Explicit Claude branch ordered before `default:` so Claude hits the `"not yet available"` message, not the generic `"unsupported provider"` fallback.
- `seedProfileConfig` at line 449-453 swallows `DefaultHostProfile` errors, so Claude-path error returns do not break existing Codex flows.
- DROP_2 still needs Phase 6 drop-end verification (`mage test`, push, CI green).

### Hylla Feedback

N/A — verifiable from the small committed diff plus two mage runs.
