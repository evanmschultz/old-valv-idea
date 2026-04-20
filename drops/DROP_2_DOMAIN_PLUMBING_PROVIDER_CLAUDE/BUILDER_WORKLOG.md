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
