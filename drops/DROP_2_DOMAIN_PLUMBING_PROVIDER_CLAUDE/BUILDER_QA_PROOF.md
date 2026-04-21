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
