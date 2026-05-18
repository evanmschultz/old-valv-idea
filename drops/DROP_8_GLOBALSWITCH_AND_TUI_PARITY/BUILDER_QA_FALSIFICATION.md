# DROP_8 — Builder QA Falsification

## Unit 8.1 — Round 1

**Verdict:** `pass`

**Builder commit under attack:** `b6951fe feat(drop-8): unit 8.1 + 8.2 + 8.6 parallel batch`

**Scope reviewed:** `internal/services/globalswitch/service.go` and `service_test.go` deltas only (Unit 8.1 surface; 8.2 / 8.6 are sibling units, not in this round's scope).

### Attack 1 — Claude CLI process-name assumption

**Hypothesis:** Builder hardcoded `processName = "claude"` for `domain.ProviderClaude` based on first-principles guessing; the real macOS Claude Code CLI process could be named `claude-code`, `anthropic-claude`, or be a Node parent process.

**Verdict:** REFUTED.

**Evidence:** Anthropic's official Claude Code documentation (Context7 `/anthropics/claude-code`, primary source `github.com/anthropics/claude-code/README.md`) documents the launch invocation as:

```bash
cd my-project
claude
```

Across all four supported install paths — `curl claude.ai/install.sh`, `brew install --cask claude-code`, Windows `winget install Anthropic.ClaudeCode`, and the (deprecated) `npm install -g @anthropic-ai/claude-code` — the produced executable is named `claude`. Builder's hardcoded `processName = "claude"` matches the documented binary name. `pgrep -x -U $UID claude` will match the macOS process exactly the same way the existing `pgrep -x -U $UID codex` matches the Codex CLI.

**Caveat (not a counterexample):** if the dev has a shell alias `claude=…` pointing at a wrapper binary, `pgrep -x` would miss it. Same caveat already applies to Codex — symmetric risk, not a regression introduced by this unit.

### Attack 2 — `prepareTarget` parameterization scope (residual `"codex"` literals)

**Hypothesis:** Builder added `provider` parameter to `prepareTarget` and parameterized `backupRoot`, but other hardcoded `"codex"` / `".codex"` literals remain in `service.go` and the production path silently keeps Codex semantics for Claude.

**Verdict:** REFUTED.

**Evidence:** `git grep -nE '"\.codex"|"\.claude"|"codex"|"claude"' internal/services/globalswitch/` shows zero literal occurrences in production code (`service.go`). All remaining occurrences are:

- `service.go` lines 89-93 — inside the explicit `switch provider {…}` block where literals are correct.
- `service_test.go` — test fixture paths (`.codex` / `.claude` directories under `t.TempDir()`), which are intentional.
- `service_test.go:312` — `processRunning(ctx, "codex")` inside `TestProcessRunningUsesPgrepExitStatus`, which tests the lower-level pgrep wrapper, not the provider-aware Switch path. Naming "codex" there is fine — the test is generic.

The `writeState` function uses `string(result.Provider)` at line 157 — already parameterized correctly. `requiresHostProcessGuard` is provider-agnostic by design (compares `homeDir` vs `realHomeDir`). No residual hardcoded provider literal in production logic.

### Attack 3 — Pre-existing target edge cases (file, symlink, dir)

**Hypothesis:** AC covers (a) pre-existing real dir → backup, (b) pre-existing symlink → replace without backup. What about pre-existing **regular file** at `~/.claude` (or `~/.codex`)? The `prepareTarget` Lstat path matters.

**Verdict:** REFUTED as counterexample; flagged as UNDOCUMENTED but NON-BROKEN behavior.

**Evidence:** `service.go:134-153`:

```go
info, err := os.Lstat(target)
if err != nil {
    if os.IsNotExist(err) { return "", nil }
    return "", err
}
if info.Mode()&os.ModeSymlink != 0 {
    return "", os.Remove(target)  // symlink replace path
}
// fall-through: real dir OR real file
backupRoot := filepath.Join(s.stateDir, "global-switch", string(provider), "backups")
... os.Rename(target, backupPath)
```

For a pre-existing regular file at `~/.claude`:
- `Lstat` succeeds (no `IsNotExist`).
- `info.Mode()&os.ModeSymlink` is 0 (regular file).
- Falls through to the rename branch. `os.Rename` of a regular file into a new path inside `backupRoot` works on macOS/Linux without trouble.
- Result: the regular file is moved to `<stateDir>/global-switch/<provider>/backups/<timestamp>` as a single file (no longer a directory of files). Then `os.Symlink` succeeds against the now-empty `~/.claude` slot.

Behavior is internally consistent and reversible. It is not currently exercised by a unit test, but the AC doesn't require it — the AC only specifies the dir-vs-symlink cases. Flag for the closeout review whether dev wants an explicit FILE-case test added in Unit 8.10 or deferred.

**Remediation suggestion (optional):** add a one-line note to `prepareTarget`'s doc-comment that regular files are treated like directories (moved into backup root). Non-blocking.

### Attack 4 — `-race` regression under concurrent `Switch` calls

**Hypothesis:** Globalswitch may hold package-level state that races under concurrent `Switch` calls from different goroutines.

**Verdict:** REFUTED.

**Evidence:**
- `git grep -nE '^var |^const ' internal/services/globalswitch/` returns zero matches.
- `Service` is passed and stored by value (line 60 returns `Service{…}`), every field (`store`, `homeDir`, `realHomeDir`, `stateDir`, `logger`, `now`, `isRunning`) is read-only after construction.
- `Switch` allocates local variables (`targetDotDir`, `processName`, `profile`, `result`); no shared mutation.
- Filesystem side effects (`os.MkdirAll`, `os.Rename`, `os.Symlink`, `os.WriteFile`) are tied to caller-provided `homeDir` / `stateDir`. Two concurrent `Switch` calls against the SAME `homeDir` would race on the filesystem — but that is an expected caller-level constraint, not a service bug. Per-call test isolation uses `t.TempDir()`, which is unique.
- `mage testPkg ./internal/services/globalswitch` ran clean with `-race` (10/10 tests, 82.1% coverage, no race warnings).

### Attack 5 — Forward compatibility with future providers

**Hypothesis:** The provider switch is extension-hostile (e.g., no `default` arm, or a `default` arm that silently treats unknown providers as Codex).

**Verdict:** REFUTED.

**Evidence:** `service.go:86-96`:

```go
switch provider {
case domain.ProviderClaude:
    targetDotDir = ".claude"
    processName = "claude"
case domain.ProviderCodex:
    targetDotDir = ".codex"
    processName = "codex"
default:
    return Result{}, fmt.Errorf("switch global profile %q: unsupported provider", provider)
}
```

Pattern is identical to `domain.ParseProvider` (`types.go:15-24`) — explicit per-provider case plus a default arm that errors. Adding a hypothetical `domain.ProviderAnthropicConsole` would be a localized one-case addition in both `ParseProvider` and `Switch`. The two locations are co-evolved — typing `ProviderXxx` in one but forgetting the other surfaces as a `default`-arm error at runtime, not silent fallthrough. Extension-ready.

### Attack 6 — `TestSwitchRejectsUnsupportedProvider` semantics

**Hypothesis:** Builder changed the test from rejecting `domain.Provider("claude")` to rejecting `domain.Provider("unknown")`. Does this still exercise the `Switch` rejection PATH, or has it slipped into testing `ParseProvider`?

**Verdict:** REFUTED — test correctly exercises the `Switch` `default` arm.

**Evidence:** `service_test.go:100`:

```go
if _, err := service.Switch(context.Background(), domain.Provider("unknown"), "work"); err == nil {
    t.Fatal("Switch() error = nil, want unsupported-provider failure for unknown provider")
}
```

The test invokes `service.Switch(…)` **directly** with `domain.Provider("unknown")` constructed by type-casting a string literal. No `ParseProvider` is called. The unknown value flows into the `switch provider {…}` in `Switch` (service.go:87) and hits the `default` arm (line 94), returning the `"unsupported provider"` error. This is exactly the rejection path; the only thing the rename changed is the placeholder value, because `domain.Provider("claude")` is now a VALID provider and would no longer be rejected. The semantic intent — "Switch must reject providers not in its case list" — is preserved.

**Sub-attack (REFUTED):** could `domain.Provider("unknown")` accidentally collide with a future ProviderXxx? Only if a future `Provider` constant has the literal string value `"unknown"`, which would be perverse and a reviewer would catch in the same diff that introduced it. Low-cost test, defensible choice.

### Attack 7 — AC coverage gap: Claude symlink-replace path

**Hypothesis (self-initiated):** AC parity demands symmetry. Codex has `TestSwitchReplacesExistingSymlinkWithoutBackup` (service_test.go:118-150) testing the symlink-replacement path. Does Claude have an equivalent?

**Verdict:** UNKNOWN — not a hard counterexample, but a parity asymmetry worth surfacing.

**Evidence:** Two new Claude-specific tests landed:
- `TestSwitchClaudeTargetPath` — fresh symlink creation (no pre-existing target).
- `TestSwitchClaudeBacksUpExistingDir` — pre-existing real dir → backup.

There is NO `TestSwitchClaudeReplacesExistingSymlinkWithoutBackup`. The symlink-replace logic in `prepareTarget` is provider-agnostic (lines 142-144 — `Mode()&os.ModeSymlink` check returns before consulting `provider`), so behaviorally Claude inherits the symlink-replace path correctly. But the AC for Unit 8.1 says "tests cover Claude target path, backup behavior, and symlink replacement" (paraphrased from PLAN.md Unit 8.1 acceptance).

**Remediation suggestion:** add `TestSwitchClaudeReplacesExistingSymlinkWithoutBackup` mirroring lines 118-150 with `domain.ProviderClaude` and `~/.claude`. ~25 LOC. Non-blocking for R1 (the logic IS exercised for the Codex path and is provider-agnostic), but recommended for closeout symmetry. Routed to dev for triage.

### Hylla Feedback

N/A — action item touched Go files but Hylla was declared unreachable for this round per spawn prompt; I used `Read` / `git grep` directly. No mid-review fallback occurred against a Hylla query.

### Summary

| # | Attack | Verdict |
|---|---|---|
| 1 | Claude process-name `"claude"` | REFUTED (confirmed via Anthropic docs) |
| 2 | Residual `"codex"` literals in production code | REFUTED |
| 3 | Pre-existing FILE (not dir/symlink) at target | REFUTED (works, undocumented) |
| 4 | `-race` regressions / pkg-level state | REFUTED |
| 5 | Forward compat with future providers | REFUTED (extension-ready) |
| 6 | `TestSwitchRejectsUnsupportedProvider` semantics | REFUTED |
| 7 | Claude symlink-replace test parity | UNKNOWN (parity gap, non-blocking) |

No CONFIRMED counterexamples. Unit 8.1 passes falsification with one routing item (attack 7) for dev triage / Unit 8.10 closeout.

**Remediation recommendations (none blocking):**

1. Add a Claude symlink-replace test (`TestSwitchClaudeReplacesExistingSymlinkWithoutBackup`) for symmetry with the Codex equivalent. Suggested placement: between `TestSwitchClaudeBacksUpExistingDir` and `TestProcessRunningUsesPgrepExitStatus`.
2. Optional: doc-comment on `prepareTarget` clarifying that regular files at the target path are moved into the backup root the same way directories are.

## Unit 8.6 — Round 1

**Verdict:** `pass`

**Builder commit under attack:** `b6951fe feat(drop-8): unit 8.1 + 8.2 + 8.6 parallel batch`

**Scope reviewed:** `internal/tui/manage/golden_test.go` + new `internal/tui/manage/testdata/TestProfilePickerGoldenClaude.golden`. Unit 8.6 surface only; Units 8.1 / 8.2 are sibling units, not in this round's scope.

### Attack 1 — Identity-substitution only (golden-diff bounding)

**Hypothesis:** Builder claimed the two goldens differ only on line 5's "claude accounts" vs "codex accounts" substring. Any other byte-level divergence (whitespace, ordering, profile names) violates the strict golden-diff bounding AC.

**Verdict:** REFUTED.

**Evidence:** `cmp -l <codex.golden> <claude.golden>` reports exactly 30 differing bytes across two files of identical total size (3426 bytes each). All 30 diff bytes fall in the contiguous offset range 620–650, which maps to line 5 of the golden (the title row). Concretely, that range contains:
- the literal label swap (`codex` → `claude`, +1 char), and
- a compensating shift in the trailing pad-spaces inside the title cell (32 spaces → 31 spaces) so the surrounding box width remains constant.

Lines 1–4 (header) and lines 6–22 (account rows, footer, help line) are byte-identical between the two goldens. The label sits inside an ANSI sandwich `[48;5;62m [m[38;5;230;48;5;62m<label>[m[48;5;62m [m` that is preserved structurally in both files; only the label-text bytes and one trailing space change. This is exactly the "modulo provider-specific identity fields" carve-out the AC permits ("the test fails if the format diverges beyond those identity-field substitutions"). No extraneous divergence detected.

### Attack 2 — Brittle golden on width/style change

**Hypothesis:** The picker is width-locked. A future picker-style tweak (padding, border, list-delegate styling) would force regenerating both goldens, undermining a "stable parity" claim.

**Verdict:** REFUTED.

**Evidence:** `internal/tui/manage/model.go:13` declares `defaultWidth = 80` and `defaultHeight = 24` as package-level constants. `picker.go:42-47` instantiates the list with width = `defaultWidth - styles.Frame.GetHorizontalFrameSize()`. The terminal size sent in the test (`tea.WindowSizeMsg{Width: 96, Height: 24}`) pads the rendered viewport to 96 cols; the picker itself stays width-locked at `defaultWidth`. Any picker-style change would indeed force regenerating both goldens — but this is a SHARED brittleness present in the pre-existing Codex test before Unit 8.6 landed. Builder mirrored the existing brittleness symmetrically; he did not introduce new brittleness. The unit's AC does not forbid brittle goldens; it requires format parity. Width-lock is therefore part of the contract being mirrored, not a violation of it. The risk surfaces equally for both providers (regenerate both files together), which is acceptable for a parity test pair.

### Attack 3 — `teatest.RequireEqualOutput` vs `final.View().Content`

**Hypothesis:** Builder uses `teatest.RequireEqualOutput(t, []byte(final.View().Content))` — final-view-only assertion. If the picker has any intermediate state worth asserting, this API choice misses it.

**Verdict:** REFUTED.

**Evidence:** The new `TestProfilePickerGoldenClaude` (golden_test.go:41-55) uses the IDENTICAL API surface as the existing `TestProfilePickerGolden` (golden_test.go:25-39) — same `teatest.NewTestModel` + `tea.WindowSizeMsg` + `tm.Quit()` + `tm.FinalModel` + `teatest.RequireEqualOutput([]byte(final.View().Content))`. The existing Codex test is established prior art; mirroring its API choice for Claude is correct symmetry, not a regression. The picker has no meaningful intermediate-state surface in this test path: the test sends a `WindowSizeMsg`, immediately calls `Quit`, and asserts on the resulting final view. The unit's AC (line 182 of PLAN.md) explicitly calls for `teatest.RequireEqualOutput` on `final.View().Content` — so the implementation matches the spec verbatim. If transcript-level coverage (filter typing, navigation, selection rendering) were desired, that would be a separate test under a separate AC, not a defect of Unit 8.6.

### Attack 4 — Provider label hardcoding

**Hypothesis:** "claude accounts" / "codex accounts" is hardcoded per provider inside `picker.go`. A third provider would require source changes.

**Verdict:** REFUTED.

**Evidence:** `picker.go:48` renders the title via `menu.Title = fmt.Sprintf("%s accounts", provider)`. The `provider` parameter is `domain.Provider`, declared at `internal/domain/types.go:8` as `type Provider string` with `ProviderCodex = "codex"` (line 11) and `ProviderClaude = "claude"` (line 12). The `%s` verb renders the underlying string value directly; the title is provider-driven, not branch-hardcoded. A third provider added to `domain` (e.g. `ProviderFoo = "foo"`) would render `"foo accounts"` automatically — no `picker.go` source changes required. Builder honored the "do not modify picker.go" constraint, and the rendering pathway is genuinely provider-agnostic. The label-render is a textbook identity substitution.

### Attack 5 — Test data choices

**Hypothesis:** Builder used `alpha-profile` / `beta-profile` as Claude test data; if these names differ from the Codex test's data, the goldens have structural divergence beyond provider label.

**Verdict:** REFUTED.

**Evidence:** Both tests use the identical profile fixture:

- `TestProfilePickerGolden` (golden_test.go:27): `{{Name: "alpha-profile", HomePath: "/tmp/alpha-profile"}, {Name: "beta-profile", HomePath: "/tmp/beta-profile"}}`.
- `TestProfilePickerGoldenClaude` (golden_test.go:43): `{{Name: "alpha-profile", HomePath: "/tmp/alpha-profile"}, {Name: "beta-profile", HomePath: "/tmp/beta-profile"}}`.

Same names, same home paths, same order, same count. The fixture is structurally identical across the two tests, which is why the byte-cmp output concentrates all 30 differing bytes on line 5 (title row) — lines 9-13 (the rendered profile rows) are byte-identical between the two goldens, confirmed via the cmp 30-byte total figure and the byte-offset clustering.

### Attack 6 — Golden filename / test-name convention

**Hypothesis:** teatest's golden lookup derives the filename from `t.Name()`. If the test name and filename diverge, `mage golden` would either miss the comparison or compare against the wrong file.

**Verdict:** REFUTED.

**Evidence:** Test name: `TestProfilePickerGoldenClaude` (golden_test.go:41). Golden filename: `testdata/TestProfilePickerGoldenClaude.golden`. Convention `<TestName>.golden` matches. `mage golden` and `mage testPkg ./internal/tui/manage` both pass with the new test included (24 tracked tests + 1 external transcript green in `mage golden`; 8/8 tests + 91.3% coverage in `mage testPkg`). The naming pathway is verified end-to-end by the green build.

### Attack 7 — ANSI escape preservation around the label

**Hypothesis:** The label is wrapped in ANSI background/foreground escapes. A 1-char label-width shift could break the ANSI sandwich or leak styling into surrounding cells.

**Verdict:** REFUTED.

**Evidence:** Both goldens preserve the identical ANSI sandwich on line 5: `[48;5;62m [m[38;5;230;48;5;62m<label>[m[48;5;62m [m`. The opening pad-space (background-only) and closing pad-space (background-only) are identical in both files. Only the label-text bytes differ, plus the trailing layout pad-space count after the closing `[m`. No styling leaks into the box border (`[38;2;77;114;138m│[m`) on either side. Layout integrity preserved.

### Mage verification

- `mage testPkg github.com/evanmschultz/valv/internal/tui/manage` — PASS (8 tests, 91.3% package coverage, `-race` on).
- `mage golden` — PASS (24 tracked tests + 1 external Codex transcript, all green).
- Worklog also records `mage goldenUpdate` was used to generate the new fixture before commit. Verified by file presence + commit `b6951fe`.

### Summary

All seven attack vectors (5 from the appendix + 2 bonus probes) REFUTED with concrete file:line evidence and a passing mage verification gate. The Unit 8.6 work survives falsification.

- Identity-substitution bounding holds: 30 differing bytes, all on line 5, all explainable as label-text swap + 1-char trailing-pad compensation.
- Width-lock brittleness is shared between Codex and Claude tests, not introduced.
- API surface (`teatest.RequireEqualOutput` on `View().Content`) matches the AC verbatim and mirrors prior-art Codex test.
- Provider label is `fmt.Sprintf("%s accounts", provider)` — provider-driven, not hardcoded.
- Test fixtures are byte-identical between Codex and Claude tests.

**Remediation recommendations:** none. The implementation matches the AC and the worklog claim ("All ANSI color codes, layout, profile names, and footer text are identical") is accurate; the trailing-pad-space count differs by exactly 1 to compensate for the 1-char label-length difference, which is a mechanical consequence of the identity substitution, not an extraneous layout divergence.

**Unknowns:** none.

## Hylla Feedback (Unit 8.6)

N/A — Unit 8.6 touches only test files and a generated golden fixture (markdown + Go test + ANSI text). Hylla is unreachable this session per spawn paradigm override. All evidence gathered via direct `Read` (golden files, picker.go, model.go, manage.go, picker_test.go, domain/types.go) plus mage verification.

## Unit 8.2 — Round 1

**Verdict:** `fail (1 BLOCK + 1 CONCERN + 3 NIT)`

**Builder commit under attack:** `b6951fe feat(drop-8): unit 8.1 + 8.2 + 8.6 parallel batch`

**Scope reviewed:** `internal/cli/manage.go` (Unit 8.2 deltas: `newManageAccountSwitchCommand`, `runManageAccountSwitch`, `resolveAccountSwitchTarget`, `pickProfileCrossProvider`) and `internal/cli/manage_test.go` (six new tests + helpers). Sibling units 8.1 and 8.6 are out of scope here.

`mage testPkg github.com/evanmschultz/valv/internal/cli` and `mage test` both GREEN at HEAD (`b6951fe`). Coverage 71.4% in `internal/cli` (above 60% floor; meets AGENTS.md § 11 70% target).

### Attack 1 — `domain.Provider("all")` magic-string collision and side-channel UX

**Hypothesis (vector 1):** Builder uses `domain.Provider("all")` as a sentinel passed to `pickProfile` from `pickProfileCrossProvider` (`internal/cli/manage.go:995`). Risks: (a) future collision if a real provider named `all` is added to `internal/domain/types.go`; (b) the sentinel leaks into user-facing strings because `pickProfile` interpolates the provider into error text and into the picker title.

**Trace:**

1. `domain.ParseProvider("all")` (`internal/domain/types.go:15-24`) currently returns `"parse provider \"all\": unsupported value"`. A future change adding `ProviderAll Provider = "all"` would have to round-trip the value through `ParseProvider`, at which point the author would notice the collision. Collision risk is LOW (one-line const block).
2. **Side-channel leak — CONFIRMED counterexample (small).** When both providers have zero accounts and the user runs `valv account switch` with no args, `pickProfileCrossProvider` calls `pickProfile(cmd, domain.Provider("all"), allProfiles)` with `len(allProfiles)==0`. `pickProfile` (`internal/cli/operator_helpers.go:158-160`) returns:

   ```
   no all accounts found; run `valv manage account add all` for the default host-backed account or `valv manage account add all account-name` for an isolated account first
   ```

   The remediation `valv manage account add all` is invalid (`ParseProvider` rejects "all"). User gets nonsense guidance — visible UX bug surfaced via the magic-string sentinel.

3. **Picker title leak — NIT.** `managetui.NewProfilePicker` (`internal/tui/manage/picker.go:48`) sets `menu.Title = fmt.Sprintf("%s accounts", provider)` → cross-provider picker title becomes `"all accounts"` — passable but inconsistent with the per-provider titles (`"codex accounts"`, `"claude accounts"`).

**Verdict:** **NIT** for the magic-string itself; **CONFIRMED counterexample (small)** for the zero-account error leak (`"no all accounts found... run valv manage account add all"`). Suggest typed sentinel: introduce `domain.ProviderUnspecified Provider = ""` or pass an `Optional[Provider]` (or a separate `pickProfileFromMixedList` that takes profiles, not a provider label), and special-case the cross-provider picker title to `"All accounts"` while keeping the zero-account error generic.

### Attack 2 — Cross-provider picker post-resolution dead-end on duplicate names

**Hypothesis (vector 4):** `pickProfileCrossProvider` aggregates all profiles across providers, lets the user pick by NAME only, then post-resolves the selected name via `ProfileByName` across `supportedProviders()`. If the same account name exists in two providers AND both rows appear in the picker, the user picks ONE specific row but the post-resolution sees the name in both providers and trips the `len(matches)>1` branch — returning the multi-match error and instructing the user to re-run with `--provider`.

**Trace:**

1. `pickProfileCrossProvider` (`internal/cli/manage.go:985-1019`) collects `allProfiles` from both providers, hands them to `pickProfile`.
2. `pickProfile` (`internal/cli/operator_helpers.go:166`) instantiates `managetui.NewProfilePicker(provider, sorted)`.
3. `NewProfilePicker` (`internal/tui/manage/picker.go:38-41`) builds `profileItem{name: profile.Name, home: profile.HomePath}`. `Title()` returns name only; `Description()` returns the home path; provider identity is discarded at this layer.
4. `ProfilePickerModel.Selected()` (`internal/tui/manage/picker.go:95-100`) returns `(string, bool)` — just the name. Provider identity of the selected row is lost.
5. Back in `pickProfileCrossProvider`, the post-resolution loop (`internal/cli/manage.go:1001-1018`) iterates `supportedProviders()` calling `ProfileByName(ctx, p, selected)`. If the same name exists in both providers, `len(matches)==2`, returning:

   ```
   account "work" found in multiple providers: (codex, work), (claude, work); use --provider to specify which one
   ```

   **The user successfully picked a row in the TUI** and got told to re-invoke the CLI with a different flag. UX dead-end.

**Concrete repro (test does not exist today):**

```go
func TestAccountSwitchCrossProviderPickerDuplicateNames(t *testing.T) {
    // Pre-condition: "work" exists in both codex and claude.
    testCreateAccount(t, paths, domain.ProviderCodex, "work")
    testCreateAccount(t, paths, domain.ProviderClaude, "work")
    // Open picker (no args), pick "work" via teatest harness.
    // Expected: switch succeeds, bound to picked row's provider.
    // Actual: returns multi-match error, user has to re-invoke with --provider.
}
```

No test in the six new tests covers this path. `TestAccountSwitchNameMultiMatchErrors` (`manage_test.go:583-599`) covers the **CLI step-3** multi-match case (where the multi-match error is correct because the CLI invocation gave no disambiguator) — that test does NOT cover the **picker-post-resolution** path, which is broken precisely because the picker UI gives the user a disambiguator (selecting one specific row) and the code throws it away.

**Verdict:** **CONFIRMED counterexample. BLOCKER.** Remediation options, in order of correctness:

1. (Preferred) Change `ProfilePickerModel.Selected()` to return `(domain.Profile, bool)` (or `(name, provider, ok)`) so identity round-trips through the picker. `pickProfileCrossProvider` then short-circuits the post-resolution loop when the picker already disambiguated.
2. (Cheaper, partial) Pre-dedup or pre-disambiguate names in `pickProfileCrossProvider` before calling `pickProfile`: e.g., when a name collides, render as `"work (codex)"` / `"work (claude)"` in the picker's `Title()` AND in the post-resolution lookup table. Risk: picker `Title()` text becomes a lookup key — fragile.
3. (Worst) Just document the limitation. Rejected — the failure mode is silent and only fires when the user has duplicate names, which is exactly the case the cross-provider picker UX was supposed to solve.

### Attack 3 — `resolveProfileSwitchTarget` retained for inspect / resolveManagedAccount asymmetry

**Hypothesis (vector 2):** Builder kept the old `resolveProfileSwitchTarget` (`internal/cli/manage.go:842-877`) for `runManageAccountInspect` (line 716) and `resolveManagedAccount` (line 1034), while `account switch` got the new 4-step `resolveAccountSwitchTarget`. Is the divergence intentional or a missed migration?

**Trace:**

1. Unit 8.2 plan acceptance (`drops/DROP_8_*/PLAN.md:52-71`) scopes the 4-step resolution explicitly to `account switch`. `account inspect` is not mentioned.
2. `resolveProfileSwitchTarget` has different semantics: in the 0-arg case it returns the **currently bound** project's provider (via `service.Status(...)`); in 1-arg case it falls back to current-binding provider if the arg doesn't parse as a provider. That binding-aware fallback makes sense for `account inspect` (which wants to inspect the current project's account when called bare) but would be **wrong** for `account switch` (where the new design wants a cross-provider name search).
3. Asymmetry: `valv account inspect foo` currently uses current-binding provider as the fallback when `foo` isn't a provider name; `valv account switch foo` does a cross-provider name search and surfaces a clearer multi-match / not-found error. A new user calling `inspect foo` on a name that exists in multiple providers gets a less helpful experience than the `switch` equivalent.

**Verdict:** **CONCERN, not blocker.** Out of Unit 8.2 acceptance scope (PLAN.md never asked for `inspect` to get the 4-step). Dev should decide whether to queue a follow-up for inspect / resolveManagedAccount in the post-DROP_8 CLI audit (DROP_9). Recommend a brief comment on `resolveProfileSwitchTarget` noting "intentionally retains legacy binding-aware fallback for inspect / managed-account flows; `account switch` uses `resolveAccountSwitchTarget` for the cross-provider design".

### Attack 4 — `forvar` lint at manage_test.go:320

**Hypothesis (vector 3):** Spawn prompt names a "`forvar` lint at `manage_test.go:320:3`. 'copying variable is unneeded' lint." Suggested as the classic `tc := tc` pre-Go-1.22 capture pattern, now redundant.

**Trace:**

1. `manage_test.go:319-322` contains `for _, args := range [][]string{...}` followed by `args := args` (line 320). Go 1.26 (per AGENTS.md and `go.mod`) creates a fresh loop variable per iteration since 1.22, so the copy is dead code.
2. `mage test` GREEN at HEAD — `go vet` (which is bundled into `go test`) does NOT flag this. No project-level `staticcheck` / `gocritic` / `golangci-lint` is wired into the build gate that would surface this either (`magefile.go` `Test` target = gofumpt + `go test -race -cover` only).
3. The "lint at `manage_test.go:320:3`" described in the spawn prompt isn't produced by any tool the build runs today. If the dev's editor flags it via a personal `gopls` / `staticcheck` config, it's an IDE-side warning, not a build-gate failure.

**Verdict:** **NIT.** The line IS dead code (`args := args` is unneeded in Go 1.22+) and SHOULD be removed for cleanliness, but no `mage` gate fails on it. Builder should delete the `args := args` line in a tidy-up commit. Not a falsification of any Unit 8.2 claim.

### Attack 5 — Step-3 error message quality

**Hypothesis (vector 5):** Multi-match and no-match error strings — are they actionable for a new user?

**Trace:**

1. Multi-match (`manage.go:970`): `"account \"shared\" found in multiple providers: (codex, shared), (claude, shared); use --provider to specify which one"` — names the matches, names the fix flag. Clear.
2. No-match (`manage.go:961`): `"account \"foo\" not found in any provider; run `valv manage account add codex foo` or `valv manage account list` to see all available accounts"`.

   **Sub-issue — CONFIRMED nit:** the remediation always suggests `valv manage account add codex foo`, hardcoding `codex` regardless of which provider the user actually uses. A Claude-only user gets advice to add an account under `codex`. Minor UX inconsistency; not blocking but worth a tweak.

3. The two error strings are NOT covered by `mage test` for exact string content beyond the substrings checked in the new tests (`TestAccountSwitchNameMultiMatchErrors` checks `["shared", "codex", "claude", "--provider"]`; `TestAccountSwitchNameNotFoundErrors` checks `["does-not-exist"]`). Both tests are non-vacuous.

**Verdict:** **NIT.** Suggest dropping the provider name from the no-match remediation to: `"run \`valv manage account add <provider> foo\` (provider = codex|claude) or \`valv manage account list\` to see available accounts"`. Not blocking.

### Attack 6 — `writeAccountsByProvider` regression test non-vacuity

**Hypothesis (vector 6):** Verify `TestManageAccountListNoArgsShowsCrossProvider` actually exercises BOTH providers' output.

**Trace:**

1. `manage_test.go:619-632` calls `testCreateAccount(t, paths, domain.ProviderCodex, "dev-codex")` AND `testCreateAccount(t, paths, domain.ProviderClaude, "dev-claude")` — non-vacuous setup.
2. Output assertions: `["codex accounts", "dev-codex", "claude accounts", "dev-claude"]` — covers both section headings AND both account names. Would fail if either provider's section is dropped from `writeAccountsByProvider` output.
3. `writeAccountsByProvider` (`manage.go:1093-1130`) iterates `supportedProviders()` and writes one section per provider. Confirmed the regression test pins both.

**Verdict:** **REFUTED.** Test is non-vacuous and adequately pins the cross-provider behavior for `account list`. No falsification.

### Summary

**Verdict:** **FAIL** — Attack 2 is a CONFIRMED BLOCKER counterexample (cross-provider picker dead-ends on duplicate names; user picks a row, gets told to use `--provider`). Attack 1 surfaces a CONFIRMED small UX leak (zero-account error message says `"valv manage account add all"`). Attack 3 is a non-blocking CONCERN about CLI asymmetry between `switch` and `inspect`. Attacks 4 and 5 are NITs. Attack 6 REFUTED.

**Remediation routing (orchestrator decision):**

1. **BLOCKER (Attack 2):** Decide whether to fix in a Unit 8.2 R2 builder pass NOW, or defer to a tightly-scoped follow-up unit (e.g. 8.2b) within DROP_8. Fix shape: change `ProfilePickerModel.Selected()` to round-trip the selected profile's provider, so `pickProfileCrossProvider` can short-circuit the multi-match check when the picker already disambiguated. Requires a `internal/tui/manage/picker.go` change plus a `internal/tui/manage/picker_test.go` update plus a new `internal/cli/manage_test.go` integration test for the duplicate-name picker path.
2. **CONFIRMED small (Attack 1, zero-account leak):** Cheapest fix is to make `pickProfileCrossProvider` short-circuit on `len(allProfiles)==0` and return its own error before calling `pickProfile`: `"no accounts found in any provider; run \`valv manage account add codex <name>\` or \`valv manage account add claude <name>\` to create one"`. Bundle with the BLOCKER fix.
3. **CONCERN (Attack 3):** Surface to dev; route to DROP_9 (CLI audit) if not addressed inline.
4. **NIT (Attack 4):** Delete `args := args` at `manage_test.go:320` in a tidy-up commit.
5. **NIT (Attack 5):** Tweak no-match remediation string to not hardcode `codex`.

**Unknowns:** Whether the dev wants the BLOCKER fix in R2 or in a 8.2b follow-up; both are reasonable given DROP_8 size.

## Hylla Feedback (Unit 8.2)

N/A — Hylla unreachable per spawn paradigm override; all evidence gathered via `Read`, `Grep` (`rg`), `git diff`, and `mage testPkg` / `mage test`. No tool-shape gripes to report.
