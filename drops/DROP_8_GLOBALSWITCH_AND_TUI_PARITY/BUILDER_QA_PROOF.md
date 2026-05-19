# DROP_8 — Builder QA Proof

Append a `## Unit N.M — Round K` section per QA attempt. See `main/drops/WORKFLOW.md` § "Phase 5 — Build QA (per unit)".

## Unit 8.1 — Round 1

**Verdict:** PASS

**Reviewer:** go-qa-proof-agent
**Commit reviewed:** `b6951fe feat(drop-8): unit 8.1 + 8.2 + 8.6 parallel batch`
**Files reviewed:**
- `internal/services/globalswitch/service.go` (diff vs HEAD~1: +15/-7)
- `internal/services/globalswitch/service_test.go` (diff vs HEAD~1: +104/-2)

### Reproducibility

`mage testPkg github.com/evanmschultz/valv/internal/services/globalswitch`:

- 10/10 tests passed (`-race` on; the mage `testPkg` target runs `-race -cover -count=1` per `magefile.go`).
- Package coverage: **82.1%** (gate is the package floor of 60% configured in mage `testPkg`; AGENTS.md § 11's 70% floor also satisfied).
- Total runtime: 1.47s.

Matches builder's BUILDER_WORKLOG.md claim exactly.

### Per-AC findings

| # | AC (paraphrased from PLAN.md Unit 8.1) | Status | Evidence |
|---|---|---|---|
| 1 | `Switch(ctx, ProviderClaude, profile)` creates `~/.claude` symlink | PASS | `service.go:88-90` sets `targetDotDir=".claude"`; `service.go:114` joins `s.homeDir, targetDotDir`; `service.go:124` calls `os.Symlink(profile.HomePath, result.TargetPath)`. `TestSwitchClaudeTargetPath` (`service_test.go:200-250`) asserts the symlink via `os.Readlink`. |
| 2 | Provider switch at the equivalent of old `service.go:106` selecting `.claude` vs `.codex`; new `TestSwitchClaudeTargetPath` AND existing Codex tests pass | PASS | Switch block at `service.go:86-96`. `TestSwitchCodexSymlinksSelectedProfileAndBacksUpExistingDir`, `TestSwitchReplacesExistingSymlinkWithoutBackup`, `TestSwitchRejectsWhenHostCodexIsRunning`, `TestSwitchSkipsHostProcessGuardForDisposableHome`, `TestSwitchPropagatesProfileLookupFailure` all green in the 10/10 mage run. |
| 3 | `prepareTarget` gains `provider domain.Provider` param; `backupRoot` becomes `filepath.Join(s.stateDir, "global-switch", string(provider), "backups")`; all call-sites pass `provider` | PASS | New signature at `service.go:134`. `backupRoot` at `service.go:145` uses `string(provider)`. Only call-site at `service.go:119` passes `provider`. `string(ProviderCodex) == "codex"` (`domain/types.go:11`) so the existing Codex backup path layout is preserved bit-exact. |
| 4 | Backup for pre-existing real `~/.claude` lands at `global-switch/claude/backups/<timestamp>` | PASS | `TestSwitchClaudeBacksUpExistingDir` (`service_test.go:252-300`) creates a real `.claude` dir with a `settings.json`, runs Switch with a fixed `Now`, asserts `BackupPath` has the prefix `<stateDir>/global-switch/claude/backups` AND the original `settings.json` is in the backup. Green in the mage run. |
| 5 | Pre-existing `~/.claude` symlink replaced without backup (mirrors Codex) | PASS | `prepareTarget` at `service.go:142-144` returns `("", os.Remove(target))` for symlinks regardless of provider — provider-agnostic branch, so Codex behavior automatically applies to Claude. Indirect: `TestSwitchReplacesExistingSymlinkWithoutBackup` (Codex variant) green; the logic is provider-agnostic. **Minor gap (non-blocking):** no Claude-specific symlink-replacement test, but the branch is exercised by the Codex test against the same code path. Documented in §"Findings" below. |
| 6 | Host-process guard for Claude checks `"claude"` (not `"codex"`); name verified on a host with Claude installed | PASS (with note) | `service.go:90` `processName = "claude"`. **Verified empirically:** `pgrep -lx claude` on this host returns 4 live processes (PIDs 2411, 3325, 4763, 5283 — all running Claude `2.1.143`). The CLI binary is at `~/.local/bin/claude`. `pgrep -x` (used by `processRunning` at `service.go:186`) matches the COMM name exactly — so `"claude"` is the correct constant. Builder's WORKLOG flagged this as unverified (Bash blocked in agent context); QA verified it here. |
| 7 | Codex test suite continues to pass unchanged | PASS | All 5 pre-existing Codex tests + `TestNewRequiresStoreAndPaths` + `TestProcessRunningUsesPgrepExitStatus` green in the 10/10 mage run. No diff to those test bodies. |
| 8 | `TestSwitchRejectsUnsupportedProvider` updated: no longer expects Claude error; expects error for `Provider("unknown")` | PASS | `service_test.go:100` asserts `service.Switch(ctx, domain.Provider("unknown"), "work")` returns an error. Diff confirms the prior `domain.Provider("claude")` literal was replaced with `domain.Provider("unknown")`. Switch's `default` branch at `service.go:94-96` returns `unsupported provider` for that input. |
| 9 | `mage testPkg internal/services/globalswitch` passes with `-race` | PASS | Reproduced above — 10/10, 82.1%, 1.47s, `-race` on. |

### Falsification attacks considered

- **Unsupported-provider host-process guard leak.** Could `Switch` call `s.isRunning(ctx, "")` for an unknown provider? No — the `default` case at `service.go:94-96` returns BEFORE `requiresHostProcessGuard()` is consulted. Mitigated.
- **Backup-path provider collision.** Could a Codex switch clobber a Claude backup? No — `string(provider)` produces `"codex"` vs `"claude"` paths, and `domain/types.go:11-12` confirms those exact lowercase strings. Mitigated.
- **`pgrep -x` matching the wrong binary.** Could `pgrep -lx claude` match `claude-code`, `claude-desktop`, or other Anthropic tooling? `-x` enforces exact COMM match, not substring — verified by `TestProcessRunningUsesPgrepExitStatus` which asserts the `-x -U` flags are present. Mitigated.
- **`prepareTarget` parameter ordering regression.** Single call-site at `service.go:119` passes `(result.TargetPath, provider)` in the new order; no other callers exist (grep confirms). Mitigated.

### Findings

**1.1 [Axis: acceptance-criteria-coverage] [severity: low] AC #5 (Claude symlink replacement without backup) has no dedicated Claude test, only indirect coverage via the provider-agnostic branch in `prepareTarget`.** Evidence: `service.go:142-144` is provider-agnostic — the same code runs for both Codex and Claude paths, so the Codex test `TestSwitchReplacesExistingSymlinkWithoutBackup` proves the logic. Fix hint (non-blocking): a follow-up unit could add `TestSwitchReplacesExistingClaudeSymlinkWithoutBackup` to make the symmetry explicit, but coverage is logically complete today. **Recommendation: accept as-is.**

### Missing evidence

None blocking. All 9 ACs map to either a dedicated test or a provider-agnostic code path with Codex test coverage.

### Unknowns

None remaining. The Claude process-name Unknown flagged in BUILDER_WORKLOG.md is now empirically verified — `pgrep -lx claude` returns live Claude CLI processes on this host.

### Hylla Feedback

N/A — Hylla unreachable per spawn-prompt paradigm override. Evidence gathered via direct `Read`, `git diff HEAD~1`, `Bash` (pgrep / which), and `mage testPkg` reproduction. No Hylla queries attempted.

### TL;DR

PASS. All 9 ACs satisfied. 10/10 tests green at 82.1% coverage with `-race`. Builder's unverified Claude-process-name assumption is now empirically confirmed: `pgrep -lx claude` returns 4 live Claude `2.1.143` processes on the host. One non-blocking observation about indirect symlink-replacement coverage for Claude — accept as-is.

## Unit 8.6 — Round 1

**Verdict:** PASS

**Reviewer:** go-qa-proof-agent
**Commit reviewed:** `b6951fe feat(drop-8): unit 8.1 + 8.2 + 8.6 parallel batch`
**Files reviewed:**
- `internal/tui/manage/golden_test.go` (diff vs HEAD~1: +16/-0 — new `TestProfilePickerGoldenClaude` at lines 41-55)
- `internal/tui/manage/testdata/TestProfilePickerGoldenClaude.golden` (new, 22 lines)

**Files NOT modified (verified):** `internal/tui/manage/picker.go` — `git diff HEAD~1 --name-only` does not list it. Unit 8.6 is correctly scoped to golden-only addition.

### Reproducibility

`mage golden`:

- 24/24 tests passed across 2 packages (tracked Bubble Tea goldens) + 1/1 external transcript golden.
- All three relevant goldens green: `TestManageHomeGolden`, `TestProfilePickerGolden` (Codex), `TestProfilePickerGoldenClaude` (new).

`mage testPkg github.com/evanmschultz/valv/internal/tui/manage`:

- 8/8 tests passed (`-race -cover -count=1`).
- Package coverage: **91.3%** (well above the 60% mage floor and AGENTS.md § 11's 70% floor).
- Total runtime: 1.36s.

Matches builder's claim from the spawn prompt exactly.

### Per-AC findings

| # | AC (paraphrased from PLAN.md Unit 8.6) | Status | Evidence |
|---|---|---|---|
| 1 | `TestProfilePickerGoldenClaude` added to `golden_test.go` calling `NewProfilePicker(domain.ProviderClaude, alpha + beta)`, running through `teatest`, asserting via `teatest.RequireEqualOutput` on `final.View().Content` | PASS | `golden_test.go:41-55`. Line 42 constructs with `domain.ProviderClaude`. Lines 43-44 use `alpha-profile` + `beta-profile` identical to the Codex test. Line 54 calls `teatest.RequireEqualOutput(t, []byte(final.View().Content))`. Structure mirrors `TestProfilePickerGolden` (lines 25-39) exactly with only the provider literal swapped. |
| 2 | `testdata/TestProfilePickerGoldenClaude.golden` generated via `mage goldenUpdate` and committed | PASS | File exists at 22 lines (matches Codex golden line count). `git diff HEAD~1 --name-only` confirms it's a new tracked file in commit `b6951fe`. `mage golden` re-validates against the file with `teatest.RequireEqualOutput` — green. |
| 3 | `TestManageHomeGolden` and `TestProfilePickerGolden` (Codex) continue to pass unchanged | PASS | Both still pass in the `mage golden` 24/24 run. No diff to their function bodies in `golden_test.go` (Read confirms lines 13-39 unchanged from prior tree). |
| 4 | Format mirrors Codex golden exactly, modulo provider-specific identity fields (e.g., provider label) | PASS | Line-by-line file comparison: lines 1-4 and 6-22 byte-identical between `TestProfilePickerGolden.golden` and `TestProfilePickerGoldenClaude.golden`. Only line 5 differs — Codex: `codex accounts` (15 chars), Claude: `claude accounts` (16 chars). Trailing whitespace inside the box absorbs the 1-char label-length delta; box outer width (50 chars between `│ … │`) preserved. ANSI styling identical. Bound met. |
| 5 | `mage golden` passes (all three golden tests green) | PASS | Reproduced — 24/24 tests green across the two golden-bearing packages, plus 1/1 external transcript. |
| 6 | `mage testPkg internal/tui/manage` passes with `-race` | PASS | Reproduced — 8/8, 91.3% coverage, 1.36s, `-race` on. |

### Falsification attacks considered

- **Could the new test silently target the wrong provider?** No — `golden_test.go:42` literal `domain.ProviderClaude`, and `claude accounts` label on line 5 of the new golden matches what the picker renders for that provider. Mitigated.
- **Could the golden file have been hand-edited rather than `mage goldenUpdate`-generated?** Possible in principle, but `mage golden` re-validates `teatest.RequireEqualOutput` against the live model output and is green. If the file were hand-edited inconsistently the test would fail. Mitigated.
- **Could the diff exceed line 5?** Direct file-byte comparison shows lines 1-4 and 6-22 byte-identical. Only line 5 differs, only in the provider-label substring + adjacent trailing whitespace (mechanical from label length). Bound met.
- **Could `picker.go` have been silently modified?** No — `git diff HEAD~1 --name-only` enumerates 8 files; `internal/tui/manage/picker.go` is NOT among them. Mitigated.
- **Could the alpha/beta fixture data drift from the Codex test?** No — same `[]domain.Profile{{"alpha-profile", "/tmp/alpha-profile"}, {"beta-profile", "/tmp/beta-profile"}}` literal in both tests (lines 26-29 Codex, 42-45 Claude). The two goldens differ only because of the provider label, not fixture-data drift. Mitigated.
- **Could `mage goldenUpdate` regenerate the file with a different terminal size and pass spuriously?** Both Codex and Claude tests use `WithInitialTermSize(96, 24)` plus `WindowSizeMsg{Width: 96, Height: 24}`. Identical render dimensions. Mitigated.

No unmitigated counterexamples.

### Findings

None.

### Missing evidence

None. All 6 ACs map to direct file evidence + reproduced mage gates.

### Unknowns

None.

### Hylla Feedback

N/A — Hylla unreachable per spawn-prompt paradigm override. Evidence gathered via direct `Read`, `git diff HEAD~1`, and `mage golden` / `mage testPkg` reproduction. No Hylla queries attempted.

### TL;DR

PASS. All 6 ACs satisfied. `mage golden` 24/24 green, `mage testPkg internal/tui/manage` 8/8 @ 91.3% coverage with `-race`. New Claude golden differs from Codex golden only on line 5 (provider label + mechanical trailing-whitespace adjustment for label length); lines 1-4 and 6-22 byte-identical. `picker.go` untouched as required for a golden-only unit. No findings.

## Unit 8.2 — Round 1

**Verdict:** PASS

**Reviewer:** go-qa-proof-agent
**Commit reviewed:** `b6951fe feat(drop-8): unit 8.1 + 8.2 + 8.6 parallel batch`
**Files reviewed:**
- `internal/cli/manage.go` (diff vs HEAD~1: +174/-12)
- `internal/cli/manage_test.go` (diff vs HEAD~1: +175/-0)

### Reproducibility

`mage testPkg github.com/evanmschultz/valv/internal/cli`:

- 165/165 tests passed (`-race` on; mage `testPkg` runs `-race -cover -count=1`).
- Package coverage: **71.4%** (above the AGENTS.md § 11 70% per-package floor; mage's configured 60% floor also satisfied).
- gofumpt format check: clean.

### Acceptance criteria — per-AC verdict

**AC-1:** `newManageAccountSwitchCommand` gains `--provider <provider>` flag; ParseProvider; error if invalid. → PASS.
Evidence: `manage.go:336` declares `var providerFlag string`; `manage.go:361` registers `cmd.Flags().StringVar(&providerFlag, "provider", "", "...")`; `manage.go:907-911` calls `domain.ParseProvider(providerFlag)` and returns the error. Test `TestAccountSwitchWithInvalidProviderFlagErrors` (manage_test.go:517) asserts an invalid value produces an error mentioning the bad value. `TestAccountSwitchWithProviderFlagSucceeds` (manage_test.go:492) asserts the happy path resolves account `work` under provider `codex`.

**AC-2:** Four-step resolution order in `resolveAccountSwitchTarget`. → PASS, all four substeps verified:

- **Step 1** (`--provider` overrides everything): `manage.go:906-919` — when `providerFlag != ""`, parses flag, ignores positional arg as provider claim, uses positional[0] (if present) as account name, errors on 2+ positionals with `--provider` set. Returns `(p, name, nil)` where `name` may be `""` to signal picker-for-provider. Evidence: `TestAccountSwitchWithProviderFlagSucceeds` covers the 1-positional case; `TestAccountSwitchWithInvalidProviderFlagErrors` covers the invalid-flag case.
- **Step 2** (positional parses as provider): `manage.go:921-927` covers legacy 2-arg form `codex work`; `manage.go:935-939` covers 1-arg form where the positional itself is a provider name (returns `(p, "", nil)` for per-provider picker). `domain.ParseProvider` is case-insensitive (`internal/domain/types.go:15-24`). Evidence: `TestAccountSwitchTwoArgBackcompat` (manage_test.go:532) covers the 2-arg legacy path.
- **Step 3** (cross-provider name search): `manage.go:941-971` iterates `supportedProviders()` (Codex first, Claude second per manage.go:1001 and worklog confirmation), calls `service.ProfileByName`, filters `errors.Is(err, domain.ErrNotFound)` for no-match continue, surfaces unexpected errors, returns single match for `len(matches)==1`, returns user-facing multi-match error listing all `(provider, account)` pairs + `--provider` hint, returns not-found error for `len(matches)==0`. Evidence: `TestAccountSwitchNameUniqueAcrossProviders` (manage_test.go:557), `TestAccountSwitchNameMultiMatchErrors` (manage_test.go:583) asserts substrings `"shared", "codex", "claude", "--provider"` are present in the multi-match error, `TestAccountSwitchNameNotFoundErrors` (manage_test.go:603) covers no-match.
- **Step 4** (0 args + no flag): `manage.go:930-933` returns `("", "", nil)`; caller branch at `manage.go:598-607` opens `pickProfileCrossProvider`, which collects profiles across `supportedProviders()` (manage.go:986-993), opens one picker with the synthetic `domain.Provider("all")` label, then re-resolves the selected name's owning provider using identical match logic to Step 3 (manage.go:1000-1018). Not covered by a dedicated test (TUI picker path is hard to drive headless), but the resolver-side code path is exercised by `TestAccountSwitchWithProviderFlagSucceeds` with 0 positionals via `--provider`; the picker-launch helper is reachable and reads through cleanly. **Minor note (non-blocking):** when 0 accounts exist anywhere, `pickProfile` (operator_helpers.go:159) emits `"no all accounts found; run \`valv manage account add all\`"` — cosmetic only, fires in a degenerate state, not in AC scope.

**AC-3:** `valv account switch codex work` (two-arg form) continues to work via step 1 or 2. → PASS.
Evidence: `manage.go:921-927` handles the 2-arg form (step 2 legacy branch); `TestAccountSwitchTwoArgBackcompat` (manage_test.go:532) exercises it end-to-end and asserts `"Project binding updated", "provider=codex", "account=hylla"` in the output.

**AC-4:** `valv account list` (no args) cross-provider via `writeAccountsByProvider`. → PASS.
Evidence: `writeAccountsByProvider` (manage.go:1093) iterates `supportedProviders()` — unchanged behavior. `TestManageAccountListNoArgsShowsCrossProvider` (manage_test.go:619) added as regression guard, asserting `"codex accounts", "dev-codex", "claude accounts", "dev-claude"` are all present in the output.

**AC-5:** `mage testPkg github.com/evanmschultz/valv/internal/cli` passes with `-race`. → PASS.
Evidence: re-ran the target locally — 165/165 GREEN, 71.4% coverage, `-race` flag enabled by mage `testPkg` target per `magefile.go`. gofumpt format check clean.

### Cross-cutting findings

- **Pre-existing extended test still passes:** `TestManageAccountSwitchMissingAccountShowsActionableGuidance` (extended_test.go:208-233) asserts substring `"run \`valv manage account add codex work\`"` against the switch error for an unknown name. The new Step 3 zero-match error (manage.go:961) is composed `"account %q not found in any provider; run \`valv manage account add codex %s\` or ..."` — the substring is contiguous. Worklog calls this out explicitly. Empirically confirmed by GREEN mage run.

- **`BindProject` upsert idempotency:** `service.BindProject` → `store.UpsertProjectBinding` (per worklog at manage/service.go:194/223). Concurrent `account switch` from two processes against the same project is safe. Verified in worklog; not exercised by a concurrent test, but the upsert primitive is correct.

### Findings

- 1.1 [Axis: completion-checklist-audit] [severity: low] LSP-flagged `forvar` "copying variable is unneeded" at `internal/cli/manage_test.go:320` (line `args := args` inside `TestRunManageUpdateCodexRegression`) → `git blame internal/cli/manage_test.go -L 318,322` shows the line was authored on 2026-04-21 in commit 413db81c, NOT introduced by Unit 8.2. Go 1.26.1 (`go.mod:3`) is well past the Go 1.22 per-iteration loop-variable capture change, so the explicit shadow is redundant but harmless. Non-blocking polish; belongs in a janitor sweep, not Unit 8.2 scope.

### Missing Evidence

- 2.1 [Axis: acceptance-criteria-coverage] [severity: low] Step 4 cross-provider picker UI behavior is not directly asserted by a dedicated test (the picker is TTY-driven and hard to script headless). The resolver-side branch returning `("", "", nil)` is reachable and inspected; the `pickProfileCrossProvider` helper is reachable from `runManageAccountSwitch`. The 2+ accounts cross-provider picker live behavior depends on `pickProfile` (which is reused unchanged). Acceptable coverage gap for this unit — picker headless harness work belongs to Unit 8.6 (TUI golden) or future TUI parity work, not 8.2 logic verification.

### Worklog accuracy

`BUILDER_WORKLOG.md` Unit 8.2 entry (lines 59-108) accurately describes:
- The new function `resolveAccountSwitchTarget` and its `accountSwitchResolver` interface kept disjoint from `resolveProfileSwitchTarget` (still consumed by `runManageAccountInspect` / `resolveManagedAccount`). Confirmed at manage.go:879-897 and pre-existing callers.
- `--provider` flag wiring via `StringVar`. Confirmed at manage.go:336/361.
- Cross-provider picker collection and post-pick re-resolution. Confirmed at manage.go:985-1018.
- The error-message reorder for substring compatibility (worklog § "Error message substring compatibility"). Confirmed against extended_test.go:230 + manage.go:961.
- `BindProject` upsert idempotency note.
- `supportedProviders()` deterministic Codex-first order.
- `valv account list` no-arg cross-provider regression guard via new test at manage_test.go:619.

No inaccuracies.

### Hylla Feedback

N/A — Hylla unreachable per spawn-prompt paradigm override. Evidence gathered via direct `Read`, `git diff HEAD~1`, `git blame`, `rg`, and `mage testPkg` reproduction. No Hylla queries attempted.

### TL;DR

PASS. All 5 ACs satisfied. `mage testPkg internal/cli` 165/165 GREEN @ 71.4% coverage with `-race`. Four-step resolution order implemented correctly with explicit `domain.ErrNotFound` filtering, deterministic `supportedProviders()` iteration, multi-match error listing `(provider, account)` pairs, and `--provider` flag overriding all positional-as-provider parsing. Pre-existing `TestManageAccountSwitchMissingAccountShowsActionableGuidance` still passes because the new Step 3 zero-match error preserves the `"run \`valv manage account add codex <name>\`"` substring. The LSP `forvar` flag at line 320 is pre-existing legacy code (commit 413db81c, 2026-04-21), not introduced by this build — non-blocking polish only.

## Unit 8.2 — Round 2

**Verdict: PASS.**

R2 builder commit `15479ba2 fix(drop-8): unit 8.2 r2 cross-provider picker fixes` resolves all three R1 findings (1 BLOCKER + 1 CONCERN/leak + 1 nit-2), passes all three mage targets, and ships a measurable test for each fix. No regressions detected. The `pickProfileFn` injection seam (DROP-7 pattern) is the right ergonomic choice for a non-TTY unit test of cross-provider behavior and doesn't compromise production paths.

### Per-fix mapping

**FIX 1 (BLOCKER) — `ProfilePickerModel.Selected()` returns `(domain.Profile, bool)`:**

- `internal/tui/manage/picker.go:101-106` — signature confirmed `func (m ProfilePickerModel) Selected() (domain.Profile, bool)`; returns full profile struct with `Provider` field set.
- Call sites verified via `git grep "\.Selected()"`:
  - `internal/cli/operator_helpers.go:183` — `selected, ok := model.Selected()`; uses `selected` directly as `domain.Profile` (no name re-resolution). Returns `selected, nil` at line 187.
  - `internal/tui/manage/picker_test.go:21-26` — destructures into `selected, ok` and asserts `selected.Provider == domain.ProviderCodex`, exercising the new field-bearing API.
- The remaining `.Selected()` references at `operator_helpers.go:137` and `model_test.go:62,100` are on the unrelated `Model.Selected() (Action, bool)` method — distinct type, distinct contract; not affected.
- `pickProfile` wrapper signature is now `func pickProfile(...) (domain.Profile, error)` (operator_helpers.go:161) — single return + error replaces R1's `(domain.Profile, bool)` shape mismatch.
- `pickProfileCrossProvider` (manage.go:984-1006) reads `selected.Name, selected.Provider` directly from the picker result — post-resolution `ProfileByName` lookup loop is gone, `crossProviderLister` interface (manage.go:980-982) is `ListProfiles`-only as the comment promises.

**FIX 2 (small leak) — 0-accounts cross-provider error:**

- `internal/cli/manage.go:995-997` — explicit zero-accounts guard fires before `pickProfile(..., domain.Provider("all"), ...)` is reached:
  - Error literal: `"no accounts found across any provider; run \`valv manage account add codex <name>\` or \`valv manage account add claude <name>\` to create one"`.
  - Contains both `codex` and `claude` keywords.
  - Does not contain the literal `"all"` (verified by inspection: substring `" all "` and `"add all"` both absent).
- `TestAccountSwitchNoArgsZeroAccountsErrors` (manage_test.go:638-651) executes `valv manage account switch` with both providers empty, asserts `codex` + `claude` in the error, asserts `" all "` and `"add all"` are absent. PASSES.

**FIX 3 (nit-2) — `args := args` shadow:**

- `internal/cli/manage_test.go:320` — for-range now reads `for _, args := range [][]string{{"update"}, {"update", "codex"}}` with no redundant inner `args := args` shadow. Go 1.22+ semantics auto-fresh-scope `args` per iteration; the LSP `forvar` diagnostic does not fire on this form.

### Mage target reproducibility

| Target | Builder claim | This run | Status |
|---|---|---|---|
| `mage testPkg ./internal/tui/manage` | 8/8 @ 91.5%, `-race` | 8/8 @ 91.5%, `-race` (1.37s) | MATCH |
| `mage testPkg ./internal/cli` | 167/167 @ 73.1%, `-race` | 167/167 @ 73.1%, `-race` (5.45s) | MATCH |
| `mage golden` | 24 tests + external transcript GREEN | 24 tests + 1 external transcript GREEN (12.77s) | MATCH |

### `pickProfileFn` injection seam safety

- Package-level var: `var pickProfileFn = realPickProfile` (`operator_helpers.go:159`). Production code goes through `pickProfile()` (line 161) which dispatches to `pickProfileFn`. Default is `realPickProfile`; no production caller can observe a different impl.
- Test mutation discipline: `TestPickProfileCrossProviderHandlesDuplicateNames` (manage_test.go:660-704) is non-parallel (no `t.Parallel()`) per the comment at lines 657-659. Restores with `defer func() { pickProfileFn = orig }()`. Parallel tests that call `pickProfile` (`extended_test.go:420` and `:748`) only exercise the early-return branches of `realPickProfile` (`len(profiles) == 0`, non-TTY guidance) and run after sequential tests complete per Go testing semantics — no race window.
- `mage testPkg ./internal/cli -race` finished clean (167/167) — empirical confirmation no race triggered.

### Duplicate-name test trace

`TestPickProfileCrossProviderHandlesDuplicateNames` (manage_test.go:660-704):

1. Sets up `codexWork = {work, codex}` and `claudeWork = {work, claude}` in a `fakeCrossProviderLister` keyed by provider.
2. Stubs `pickProfileFn` to scan the picker input for `p.Provider == ProviderClaude && p.Name == "work"` and return it; `t.Fatalf`s otherwise — this asserts the cross-provider function aggregated BOTH profiles into the picker call.
3. Calls `pickProfileCrossProvider(cmd, fakeLister)`.
4. Asserts `provider == domain.ProviderClaude` and `name == "work"`.

This is exactly the failure shape the R1 BLOCKER named: same name, different providers, with no ambiguity in the returned provider. PASSES.

### QA Falsification pass

Attacked the verdict on five vectors:

1. **Unmigrated `.Selected()` caller?** — `git grep "\.Selected()"` returns five lines, all in migrated or unrelated-type files (Model.Selected on the manage Model type). No leakage.
2. **`pickProfileFn` race with parallel tests?** — sequential mutator runs before parallel readers; defer-restore inside sequential phase. `-race` clean. Mitigated.
3. **0-accounts error contains `"all"` indirectly?** — full literal inspected: `"no accounts found across any provider; run \`valv manage account add codex <name>\` or \`valv manage account add claude <name>\` to create one"` — no `all`. Mitigated.
4. **Stripping `ProfileByName` from `crossProviderLister` interface broke a caller?** — interface is local to manage.go (line 980-982), unexported, only consumer is `pickProfileCrossProvider`. The `manageservice.Service` concrete type still has `ProfileByName` for other callers (`runManageAccountSwitch:623`). Mitigated.
5. **R1 BLOCKER fully resolved or just papered over?** — The R1 finding was that `pickProfileCrossProvider` returned only `Name` and tried to re-resolve provider via name lookup, which failed when the same name existed in both providers. R2 returns `(name, provider)` derived from the picker's authoritative `domain.Profile`. No name-based ambiguity remains. Mitigated.

No unmitigated counterexample.

### Findings

None. R2 is clean across all three fixes plus the added testability seam.

### Missing evidence

None.

### Hylla Feedback

N/A — Hylla unreachable per spawn-prompt paradigm override. Evidence gathered via `git diff HEAD~1`, direct `Read`, `git grep "\.Selected()"`, `git grep pickProfile`, `git grep ProfileByName`, and `mage testPkg` / `mage golden` reproduction.

### TL;DR

PASS. All three R2 fixes verified end-to-end. Selected() returns `(domain.Profile, bool)` with no unmigrated callers; 0-accounts cross-provider error names both providers and contains no `"all"` sentinel; `args := args` shadow gone at manage_test.go:320. Three mage targets reproduce builder's exact pass counts and coverage (`mage testPkg ./internal/tui/manage` 8/8 @ 91.5%, `mage testPkg ./internal/cli` 167/167 @ 73.1%, `mage golden` 24 + 1 external). New `pickProfileFn = realPickProfile` injection seam is the standard DROP-7 pattern and is provably safe (defaults right, non-parallel mutators, `-race` clean). Unit 8.2 ready to close.

## Unit 8.3 — Round 1

**Verdict:** PASS

**Reviewer:** go-qa-proof-agent
**Commit reviewed:** `25f252b feat(cli): unit 8.3 strip --account flag and thread override` (HEAD)
**Files reviewed (diff vs HEAD~1):**
- `internal/cli/account_flag.go` (new, +55)
- `internal/cli/account_flag_test.go` (new, +137)
- `internal/cli/claude.go` (+27/-1)
- `internal/cli/claude_auth.go` (+27/-7)
- `internal/cli/claude_auth_test.go` (+3/-3)
- `internal/cli/codex.go` (+30/-7)
- `internal/cli/codex_setup.go` (+14/-2)
- `internal/cli/codex_setup_test.go` (+1/-1)
- `internal/cli/codex_test.go` (+2/-2)
- `internal/cli/account_auth.go` (+1/-1)

### Reproducibility

- `mage testPkg github.com/evanmschultz/valv/internal/cli` — **PASS** 184/184 tests, **72.6%** package coverage, `-race` enforced by mage `testPkg`. Matches builder claim #10 exactly.
- `mage build` — **PASS**, built `./valv`. Matches builder claim #11.

### Per-claim findings

1. **`stripAccountFlag` shape + case coverage (claim #1)** — PASS.
   - `internal/cli/account_flag.go:15` declares `stripAccountFlag(args []string) (accountName string, remaining []string)` matching the spec.
   - `--` sentinel hard-stops the scan at L20-22 (`if arg == "--" { return "", args }`).
   - `--account=<value>` form at L25-35; `--account <value>` form at L38-52.
   - Malformed `--account` as last token returns `("", args)` (L40-43).
   - Malformed `--account=` returns `("", args)` (L27-30).
   - First-match-wins: after assembling `remaining` from `args[:i]` + `args[i+1:]` (or `args[next+1:]`), the loop returns, leaving any later `--account` tokens preserved in `args[i+1:]` / `args[next+1:]`.
   - `account_flag_test.go` table covers all 15 named cases (no-flag, nil, empty, space-form, equals-form, malformed-last, malformed-empty, mid-args x2, multi space, multi equals, `--`-stop x2, `--account` before `--`, mixed ordered args) plus the `DoesNotMutateInputSlice` test = **16 test cases** (builder claim of "17+" is a slight overcount but immaterial — coverage is complete).

2. **`runClaudeCommand` calls `stripAccountFlag` before skip-binding check (claim #2)** — PASS.
   - `claude.go:60`: `accountName, args := stripAccountFlag(args)`.
   - `claude.go:62`: `if claudeArgsSkipProjectBinding(args) { ... }` runs AFTER the strip. Confirmed ordering.

3. **`runCodexCommand` calls `stripAccountFlag` before skip-binding check (claim #3)** — PASS.
   - `codex.go:66`: `accountName, args := stripAccountFlag(args)`.
   - `codex.go:68`: `if codexArgsSkipProjectBinding(args) { ... }` runs AFTER. Confirmed.

4. **`ensureClaudeAccountReady` 4-arg signature + override short-circuit (claim #4)** — PASS.
   - `claude_auth.go:125`: `func ensureClaudeAccountReady(cmd *cobra.Command, account domain.Profile, options accountAuthOptions, accountOverride string) error`. Fourth parameter present.
   - `claude_auth.go:126-137`: when `accountOverride != ""`, calls `openManageService(cmd, options.Paths)`, then `service.ProfileByName(cmd.Context(), domain.ProviderClaude, accountOverride)`, then `account = resolved`. **No `BindProject` call** anywhere in the override branch — store is queried read-only via `ProfileByName`. Override then falls through to the normal credential-check + auth flow with the resolved profile. **No binding row written.**
   - `git grep "BindProject"` confirms no call to `BindProject` exists in `claude.go` / `claude_auth.go`.

5. **`ensureCodexBindingReady` 4-arg signature + override short-circuit (claim #5)** — PASS.
   - `codex_setup.go:27`: `func ensureCodexBindingReady(cmd *cobra.Command, paths config.Paths, workingDir string, accountOverride string) error`. Fourth parameter present.
   - `codex_setup.go:36-41`: when `accountOverride != ""`, calls `service.ProfileByName(cmd.Context(), domain.ProviderCodex, accountOverride)`, returns nil. The default `service.Status` / `runCodexFirstRunSetup` / `BindProject` path is **never entered**. No binding row written on the override branch.

6. **`ensureBoundCodexAccountReady` accepts `accountOverride` (claim #6)** — PASS.
   - `codex.go:217`: `func ensureBoundCodexAccountReady(cmd *cobra.Command, paths config.Paths, workingDir string, args []string, accountOverride string) error`. Fifth parameter (`accountOverride`) added.
   - `codex.go:229-238`: when `accountOverride != ""`, resolves via `ProfileByName` and short-circuits to `ensureManagedAccountReady` with the resolved profile. Bound-status path (`codex.go:240-247`) skipped.

7. **`account_auth.go:42` passes `""` (claim #7)** — PASS.
   - `account_auth.go:42`: `return ensureClaudeAccountReady(cmd, account, options, "")`. No-override path.

8. **`account_flag_test.go` coverage (claim #8)** — PASS (with the minor count correction above: 15 table cases + 1 standalone = 16, not 17+; still covers all required scenarios including the `--` terminator at L83-94).

9. **`DisableFlagParsing: true` unchanged on both cobra commands (claim #9)** — PASS.
   - `claude.go:46`: `DisableFlagParsing: true` on `newClaudeCommand`.
   - `codex.go:52`: `DisableFlagParsing: true` on `newCodexCommand`.
   - Cobra semantics: `DisableFlagParsing` causes cobra to forward all argv tokens to `RunE` verbatim — required for `stripAccountFlag` to see `--account` at all (otherwise cobra would error on the unknown flag, since neither command registers `--account`).

10. **Mage test reproducibility (claim #10)** — PASS.
    - `mage testPkg github.com/evanmschultz/valv/internal/cli` reproduces exactly: 184/184 tests, 72.6% coverage, `-race` flag visible in mage `Tests` log line.

11. **`mage build` (claim #11)** — PASS. Reproduced clean.

### Callsite completeness (4-arg / 5-arg consistency)

`git grep "ensureClaudeAccountReady\|ensureCodexBindingReady\|ensureBoundCodexAccountReady"` enumerates every callsite. Every call passes the correct arity:

- `ensureClaudeAccountReady` (4 args required):
  - `account_auth.go:42` — `(cmd, account, options, "")` — 4 args.
  - `claude.go:75` — `(cmd, domain.Profile{}, accountAuthOptions{Paths: paths}, accountName)` — 4 args.
  - `claude_auth_test.go:98`, `:126`, `:157` — all 4 args with `""`.
- `ensureCodexBindingReady` (4 args required):
  - `codex.go:76` — `(cmd, paths, workingDir, accountName)` — 4 args.
  - `codex_setup_test.go:137` — `(cmd, paths, projectRoot, "")` — 4 args.
- `ensureBoundCodexAccountReady` (5 args required):
  - `codex.go:82` — `(cmd, paths, workingDir, args, accountName)` — 5 args.
  - `codex_test.go:194`, `:223` — both 5 args with trailing `""`.

No stale 3-arg / 4-arg callsites remain. The R1 fix-up cleanly closed the duplicate-declaration compile error and the test-build callsite gaps.

### `--` escape hatch — direct verification

The test case at `account_flag_test.go:83-94` (`"-- terminator stops scan (account after -- preserved)"`):

```go
args:            []string{"--", "--account", "work"},
wantAccountName: "",
wantRemaining:   []string{"--", "--account", "work"},
```

Matches the spawn prompt's required behaviour: `stripAccountFlag(["--", "--account", "work"])` returns `("", ["--", "--account", "work"])`. Source logic at `account_flag.go:20-22` (`if arg == "--" { return "", args }`) confirms — the original slice is returned unmodified, so the caller forwards `--` and everything after it unchanged to the provider CLI. Test passed in the reproducibility run.

### BUILDER_WORKLOG.md accuracy check

The worklog `## Unit 8.3 — Round 1` section (L154-216) accurately describes:
- Files touched (the actual diff shows the listed files plus the small `account_auth.go` and `claude_auth.go` edits added in the fix-up, which the `### Fix-up` subsection at L217-234 explicitly enumerates).
- The threading-approach choice (option `(a)` — add `accountOverride` parameter immediately).
- The fix-up rationale and steps (duplicate-declaration root cause; 4-step structural fix correctly recorded — actually 6 numbered steps in the worklog, slightly over-counted as "4-step" in the prose summary; immaterial).
- Mage results (`mage build` PASS, `mage testPkg internal/cli` 184/184 @ 72.6%) — match this reviewer's independent reproduction.

The fix-up correctly identifies and resolves the gap: it deleted the R1 stub in `claude.go`, modified the existing `claude_auth.go:125` declaration to add `accountOverride`, and patched all 6 stale callsites. `claude_auth.go:125-137` shows the override short-circuit using `openManageService`, `ProfileByName`, and `account = resolved`, with no `BindProject` write.

### Certificate

- **Premises:** (a) `stripAccountFlag` correctly extracts and removes `--account` per spec including `--` terminator and malformed forms; (b) every callsite of the three modified helpers compiles with the new arity; (c) override short-circuit never writes a binding row; (d) `DisableFlagParsing: true` preserved on both cobra commands.
- **Evidence:** direct Read of all changed files; `git grep` callsite enumeration; `git diff HEAD~1 --stat` scope; `mage testPkg internal/cli` 184/184 PASS @ 72.6% with `-race`; `mage build` PASS; cobra `DisableFlagParsing` semantics verified inline (raw argv flows to `RunE`).
- **Trace:** `valv claude --account work resume` → `runClaudeCommand(args=["--account","work","resume"])` → `stripAccountFlag` returns `("work", ["resume"])` → `claudeArgsSkipProjectBinding(["resume"])` false → `accountName != ""` true → `ensureClaudeAccountReady(cmd, Profile{}, {Paths: paths}, "work")` → opens manage service → `ProfileByName(Claude, "work")` → `account = resolved` → credential-check + auth flow with resolved profile → `skipValidate=true` → `service.ValidateBinding` skipped → image check → `service.Run(ctx, workingDir, ["resume"])`. No `BindProject` call on this path.
- **Conclusion:** PASS. All 11 builder claims verified. Override branch is read-only against the store (no binding-row write). All callsites compile. `--` escape hatch is correct. Mage targets reproduce green at exact builder-reported numbers.
- **Unknowns:** none for this proof axis. Unit 8.4 (Claude `OverrideProfile` threading into `claudeservice.Options`) and Unit 8.5 (Codex equivalent + merge into `ensureCodexAccountReadyForLaunch`) are out-of-scope for 8.3.

### Hylla Feedback

N/A — Hylla was unreachable this session (per spawn-prompt paradigm override). All evidence gathered via direct `Read`, `git diff HEAD~1 --stat`, `git grep`, and `mage testPkg` / `mage build` reproduction. No fallback miss to log.

### TL;DR

PASS. R1 (post-fixup) implements `--account` flag handling exactly per spec: `stripAccountFlag` with `--` terminator + first-match-wins + malformed-token tolerance; 4-arg `ensureClaudeAccountReady` / `ensureCodexBindingReady` and 5-arg `ensureBoundCodexAccountReady` with consistent callsites and no orphaned 3-arg call; override short-circuit is read-only (`ProfileByName` only, no `BindProject`); `DisableFlagParsing: true` preserved on both cobra commands; `mage testPkg internal/cli` reproduces 184/184 @ 72.6% with `-race`; `mage build` PASS. Unit 8.3 ready to close pending QA Falsification.
