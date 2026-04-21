# DROP_2 — Builder Worklog

Append a `## Unit N.M — Round K` section per build attempt. See `main/drops/WORKFLOW.md` § "Phase 4 — Build (per unit)" for what each section should contain.

## Unit 2.1 — Round 1

- **Builder:** go-builder-agent
- **Started:** 2026-04-20
- **Files touched:** internal/domain/types.go, internal/domain/types_test.go
- **Mage targets run:** mage testPkg ./internal/domain (pass — 26 tests, 85.2% coverage on domain)
- **Notes:** Added `ProviderClaude Provider = "claude"` alongside `ProviderCodex` and added an explicit `case ProviderClaude:` branch in `ParseProvider` before the `default`. Flipped the `TestParseProvider` "invalid claude" row to `{name: "claude", input: "claude", want: ProviderClaude}`, added a "claude trimmed" row mirroring the existing " Codex " case, and added a fresh `{name: "invalid", input: "openai", wantErr: true}` row so the error branch stays exercised. No design choices beyond what the unit spec prescribed — purely additive enum + parser work, consistent with the existing `ParseOutputFormat` / `ParseOutputStyle` style in the same file (trim + ToLower then switch).

### Hylla Feedback

N/A — task touched Go files only but was a small additive edit inside a single already-read file; no Hylla queries needed, no fallback required.

## Unit 2.2 — Round 1

- **Builder:** go-builder-agent
- **Started:** 2026-04-20
- **Files touched:** internal/cli/account_auth.go, internal/cli/account_auth_test.go
- **Mage targets run:** `mage testPkg ./internal/cli` — pass (101 tests, 71.9% coverage on `internal/cli`, above the 60% per-package floor used by the mage target; AGENTS.md 70% remains the drop-end gate via `mage test`)
- **Notes:** Added `case domain.ProviderClaude: return nil` as the middle branch in each of the three provider switches (`ensureManagedAccountReady`, `logoutManagedAccount`, `loginManagedAccount`), retaining the existing `default: return nil` catch-all per unit acceptance criterion. The Codex path and all three `default:` branches are byte-for-byte unchanged; `git diff internal/cli/account_auth.go` shows only additions. Extended `account_auth_test.go` by appending a single `TestProviderClaudeAccountAuthStubs` table-driven test exercising all three dispatchers with `domain.Profile{Provider: domain.ProviderClaude}` — each subtest asserts `err == nil`. Zero existing test functions touched; `git diff internal/cli/account_auth_test.go` shows additions only. Imports already present (`bytes`, `context`, `testing`, `cobra`, `domain`) covered the new test body, so no import churn. Design is deliberately the minimum needed to satisfy §6.2 compile-safe-stub discipline — no Claude-specific logic, no side effects, no new helpers.

### Hylla Feedback

N/A — task was additive edits inside already-read Go files; every file needed was provided in the spawn appendix or confirmed via direct Read / Grep (e.g. `domain.Profile` field shape verified via `Grep` on `internal/domain/model.go`). No Hylla queries attempted, so no misses to record.

## Unit 2.3 — Round 1

- **Builder:** go-builder-agent
- **Started:** 2026-04-20
- **Files touched:** internal/services/manage/service.go, internal/services/manage/service_test.go, internal/cli/manage.go
- **Mage targets run:**
  - `mage testPkg ./internal/services/manage` — pass (23 tests, 76.4% coverage)
  - `mage testPkg ./internal/cli` — pass (101 tests, 72.0% coverage)
- **Notes:** Added `case domain.ProviderClaude:` branch in `DefaultHostProfile` returning `HostProfileSpec{}` + `fmt.Errorf("resolve default host profile for provider %q: not yet available", provider)`. Placed between the `ProviderCodex` case and the retained `default:` branch so `ProviderClaude` hits the explicit branch while any future-provider enum value continues to route through the generic `"unsupported provider"` default. Appended `TestDefaultHostProfileClaudeReturnsSentinelError` to `service_test.go` following the existing `TestDefaultHostProfileUsesConfiguredHomeDir` pattern (`testStore(t)` + `Options{Store, ProviderRoot, HomeDir: "/tmp/example-home"}`); asserts `spec == HostProfileSpec{}` via value comparison, `err != nil`, and `strings.Contains(err.Error(), "not yet available")`. `strings` already imported in `service_test.go`, so no import churn. Extended `supportedProviders()` in `internal/cli/manage.go` to return `[]domain.Provider{domain.ProviderCodex, domain.ProviderClaude}`; this is consumed only by `writeAccountsByProvider`, which iterates providers and calls `ListProfiles` — empty profile lists for Claude render cleanly as an empty section, confirmed by the 101 cli tests staying green. `seedProfileConfig` (line 448) already swallows `DefaultHostProfile` errors via `return nil`, so returning an error for Claude does not break existing Codex-path flows. Design is minimum needed to satisfy §6.2 compile-safe-stub discipline — no Claude-specific behavior, no new helpers.

### Hylla Feedback

N/A — task was additive edits inside files provided verbatim in the spawn appendix; every symbol touched was visible in the Read output. No Hylla queries attempted, so no misses to record.
