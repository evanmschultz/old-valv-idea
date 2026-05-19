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

## Unit 8.2 — Round 2

**Verdict:** `pass`

**Builder commit under attack:** `15479ba fix(drop-8): unit 8.2 r2 cross-provider picker fixes`

**Scope reviewed:** R2 diff vs `HEAD~1` covering `internal/cli/codex_setup.go`, `internal/cli/global.go`, `internal/cli/manage.go`, `internal/cli/manage_test.go`, `internal/cli/operator_helpers.go`, `internal/tui/manage/picker.go`, `internal/tui/manage/picker_test.go`. R1 BLOCKER + small leak + nit-2 fixes verified.

`mage testPkg ./internal/cli` GREEN — 167 tests, 73.1% coverage. `mage testPkg ./internal/tui/manage` GREEN — 8 tests, 91.5% coverage. `mage golden` GREEN — 24 tracked + 1 external transcript.

### Attack 1 — BLOCKER (R1 CX1) picker post-resolution dead-end on duplicate `(name, home)` tuple

**Hypothesis (vector 1):** R2 changes `ProfilePickerModel.Selected()` to return `(domain.Profile, bool)` and matches the selected list item back to `m.profiles` via `name+home`. What if two profiles share the same `(name, home)` tuple — does the match-back resolve to the wrong profile?

**Trace:**

1. `picker.go:71-79` (enter handler):

   ```go
   if item, ok := m.list.SelectedItem().(profileItem); ok {
       for _, p := range m.profiles {
           if p.Name == item.name && p.HomePath == item.home {
               m.selectedProfile = p
               break
           }
       }
       m.confirmed = true
       return m, tea.Quit
   }
   ```

2. The match-back keys by `(name, home)`. In cross-provider mode (`pickProfileCrossProvider` at `manage.go:984-1006`), `allProfiles` is the concatenation of `service.ListProfiles(ctx, codex)` and `service.ListProfiles(ctx, claude)`. Question: can a Codex profile and a Claude profile have identical `(name, home)`?

3. Profile homes are provider-rooted by design:
   - Codex default host profile: `<homeDir>/.codex` (`internal/adapters/providers/codex/profile.go:13-23`).
   - Claude default host profile: `<homeDir>/.valv/providers/claude/profiles/default` (`internal/adapters/providers/claude/profile.go:18-28`).
   - Isolated named accounts: `<provider-root>/<provider>/profiles/<name>` (per-provider directory tree, see `manage.go:485-491` and the `internal/services/manage` profile creation logic).

   So `HomePath` is always rooted under the provider's namespace. Two profiles with the same `(name, home)` would require an actual filesystem coincidence, which the production code prevents.

4. **Filter input collision?** `profileItem.FilterValue() = name + " " + home` — the filter uses both fields, but the picker has `menu.SetShowFilter(false)` (`picker.go:50`), so the filter pathway is disabled. Not a concern.

**Verdict:** REFUTED — the R1 BLOCKER is fixed. The match-back loop terminates at the first `(name, home)` match, but two profiles with identical `(name, home)` cannot arise in production because Valv-managed profile homes are provider-rooted. The new unit test `TestPickProfileCrossProviderHandlesDuplicateNames` (`manage_test.go:660-701`) directly exercises the SAME-NAME-DIFFERENT-HOME case and confirms Provider round-trips correctly.

**Future fragility (flag, not counterexample):** the match-back relies on `(name, home)` being unique per profile. If a future Valv refactor allowed two profiles to share a home (e.g., aliasing) the picker would silently pick the first match. Defensible to add an explicit panic / sanity-check at picker construction, OR switch the match key to a profile index. Non-blocking; the current invariant is preserved by production code.

### Attack 2 — Small leak (R1 CX2) `Provider("all")` in 0-accounts error + cancel-path leak

**Hypothesis (vector 2):** R2 added a `len(allProfiles) == 0` pre-flight in `pickProfileCrossProvider`. But what about the case where NON-zero profiles exist and the user CANCELS the picker? Does the cancel path leak `"all"` in any user-visible string?

**Trace:**

1. R2 pre-flight (`manage.go:993-997`):

   ```go
   if len(allProfiles) == 0 {
       return "", "", fmt.Errorf("no accounts found across any provider; run `valv manage account add codex <name>` or `valv manage account add claude <name>` to create one")
   }
   ```

   No "all" in the message. Lists both providers explicitly. New test `TestAccountSwitchNoArgsZeroAccountsErrors` (`manage_test.go:639-651`) asserts both `codex` and `claude` are present AND that no `" all "` / `"add all"` substring leaks.

2. Cancel path (non-zero profiles, user presses esc):
   - `realPickProfile` (`operator_helpers.go:183-185`) returns `(domain.Profile{}, errSelectionCanceled)` — bare sentinel, no formatted message.
   - `pickProfileCrossProvider` (`manage.go:1001-1004`) propagates the error verbatim: `return "", "", err`.
   - Caller (`runManageAccountSwitch` line 599-604) catches `errors.Is(pickerErr, errSelectionCanceled)` and writes the no-op record `"No account switch made"` / `"no account selected"`. **No "all" anywhere in the cancel path.**

3. TTY-missing path (non-zero profiles, no TTY): `realPickProfile` returns `"account is required when not running in a TTY"` (`operator_helpers.go:170`). No provider-name interpolation. Clean.

4. `picker.go:49` still sets `menu.Title = fmt.Sprintf("%s accounts", provider)` → cross-provider picker title is `"all accounts"`. **Per spawn prompt: DEFERRED to DROP_9.** Not an R2 regression — explicitly out of R2 scope.

**Verdict:** REFUTED — small leak fully addressed in the error / cancel / TTY paths. Picker-title leak deferred to DROP_9 per spawn prompt; not flagged here.

### Attack 3 — Nit-2 fix verification

**Hypothesis (vector 3):** R1 nit-2 was the redundant `args := args` capture shim at `manage_test.go:320` (Go 1.22+ creates per-iteration scope, so the copy is dead code).

**Trace:** R2 diff confirms removal:

```
@@ -317,7 +318,6 @@ func TestRunManageUpdateClaudeBuildsImage(t *testing.T) {
-		args := args
 		t.Run(strings.Join(args, "_"), func(t *testing.T) {
```

Line gone. `t.Run` and the body capture `args` directly via the per-iteration loop variable (Go 1.22+). Verified `mage testPkg ./internal/cli` still GREEN.

**Verdict:** REFUTED — nit-2 fix lands cleanly.

### Attack 4 — Injection seam race / parallel test interleaving

**Hypothesis (vector 4):** R2 introduces `pickProfileFn = realPickProfile` as a package-level var. Existing parallel tests `TestPickProfileWithoutTTYRequiresExplicitAccount` (`extended_test.go:420 t.Parallel()`) and `TestPickProfileRequiresTTY` (`extended_test.go:741 t.Parallel()`) READ `pickProfileFn`. The new `TestPickProfileCrossProviderHandlesDuplicateNames` (non-parallel) WRITES it. Does `-race` surface a race?

**Trace:**

1. Go's `testing` semantics: non-parallel tests run sequentially before any `t.Parallel()` test resumes. `TestPickProfileCrossProviderHandlesDuplicateNames` is non-parallel — it executes during the sequential phase, mutates `pickProfileFn` via `orig := pickProfileFn; pickProfileFn = stub; defer func() { pickProfileFn = orig }()`. The defer fires when the test function exits.

2. Per Go testing model: ALL non-`t.Parallel()` tests in a package complete before ANY parallel test resumes from its `t.Parallel()` pause. So `pickProfileFn` is restored to `realPickProfile` BEFORE `TestPickProfileWithoutTTYRequiresExplicitAccount` or `TestPickProfileRequiresTTY` actually executes their bodies. No interleaving.

3. **Inside the parallel cohort:** the two parallel tests both READ `pickProfileFn` (no write) → no race. Reads are safe under `-race` even without synchronization, provided no concurrent write exists. Verified by `mage testPkg ./internal/cli` running 167/167 tests GREEN with `-race`.

4. **Sub-attack:** could two non-parallel tests both write `pickProfileFn` and interleave? Non-parallel tests run sequentially in source order (within their file's `init` registration order). They don't interleave with each other. Safe.

5. New test `TestPickProfileCrossProviderHandlesDuplicateNames` correctly does NOT call `t.Parallel()`. Comment block at `manage_test.go:657-659` documents the rationale.

6. New test `TestAccountSwitchNoArgsZeroAccountsErrors` (`manage_test.go:639`) also doesn't call `t.Parallel()`. It doesn't stub `pickProfileFn` directly, but it executes through the cross-provider path which COULD reach `pickProfile` if accounts existed — since the test fixture has zero accounts, the 0-accounts guard short-circuits before `pickProfile` is called. Either way: non-parallel, no race surface introduced.

**Verdict:** REFUTED — no race. `mage testPkg ./internal/cli` runs with `-race` and is GREEN at HEAD.

### Attack 5 — `crossProviderLister` interface narrowing ripple

**Hypothesis (vector 5):** R2 drops `ProfileByName` from the `crossProviderLister` interface. Any callsite that depended on the broader interface would break.

**Trace:** `git grep -n "crossProviderLister" -- '*.go'`:

```
internal/cli/manage.go:980:type crossProviderLister interface {
internal/cli/manage.go:984:func pickProfileCrossProvider(cmd *cobra.Command, service crossProviderLister) (string, domain.Provider, error) {
internal/cli/manage_test.go:703:// fakeCrossProviderLister is a test double for crossProviderLister.
```

Only ONE consumer: `pickProfileCrossProvider`. Only ONE test double: `fakeCrossProviderLister`. The R2 fake (`manage_test.go:704-710`) implements `ListProfiles` only — no `ProfileByName`. Aligned with the narrowed interface.

The production caller `runManageAccountSwitch` (`manage.go:599`) passes `service` (a `manageservice.Service`), which has BOTH methods — the narrowing only loosens the consumer's expectation. No producer breaks.

**Verdict:** REFUTED — narrowing is clean. No stranded callers.

### Attack 6 — Picker enter-handler robustness against UI text drift

**Hypothesis (vector 6):** Match-back reads `item.name` and `item.home` from the `profileItem` struct. Does it survive a future `profileItem.Title()` / `Description()` format change (e.g., adding a provider prefix to Title)?

**Trace:** `picker.go:17-19`:

```go
func (i profileItem) FilterValue() string { return i.name + " " + i.home }
func (i profileItem) Title() string       { return i.name }
func (i profileItem) Description() string { return i.home }
```

The match-back keys on the STRUCT FIELDS (`item.name`, `item.home`), not the rendered `Title()` / `Description()` strings. A future change to add `"[codex]"` prefix in `Title()` would NOT break match-back — the underlying `i.name` is unchanged. The struct fields and the rendered text are decoupled. Robust against display-text drift.

**Edge case — zero-value `home`:** if two profiles have `HomePath == ""` AND the same name, the match-back picks the first. This is impossible in production (both `DefaultHostProfile` paths and isolated account paths are non-empty by construction), but unconstrained at the picker level. Same future-fragility note as Attack 1.

**Verdict:** REFUTED — match-back is robust against UI text drift.

### Attack 7 — API ripple completeness (`pickProfile` signature change)

**Hypothesis (vector 7):** `pickProfile` changed from `(string, error)` → `(domain.Profile, error)`. Every callsite must be updated.

**Trace:** `git grep -n "pickProfile(" -- '*.go'`:

```
internal/cli/codex_setup.go:73    selectedProfile, err := pickProfile(...) → loginBindAndReportCodexSetup(..., selectedProfile)
internal/cli/extended_test.go:428 _, err := pickProfile(...)              → test assertion only (no profile use)
internal/cli/extended_test.go:748 _, err := pickProfile(...)              → test assertion only
internal/cli/global.go:89         pickedProfile, pickErr := pickProfile(...); selected = pickedProfile.Name
internal/cli/manage.go:458        selectedProfile, err := pickProfile(...) → runManageBind(..., selectedProfile.Name, "")
internal/cli/manage.go:613        pickedProfile, pickErr := pickProfile(...); profileName = pickedProfile.Name
internal/cli/manage.go:1001       selected, err := pickProfile(...); return selected.Name, selected.Provider, nil
internal/cli/operator_helpers.go:161 — definition (returns domain.Profile, error)
```

7 callsites total (5 production + 2 test). All consume the new `(domain.Profile, error)` signature correctly:

- 2 test callsites discard the profile (`_, err :=`) — they only check error text. Compatible.
- 5 production callsites either bind the full profile (`selectedProfile`, then pass into another function) or extract `.Name` / `.Provider`. All compile under the new signature.

Verified by `mage testPkg ./internal/cli` GREEN.

**Verdict:** REFUTED — all 7 callsites correctly migrated. No stranded `(string, error)` consumer.

### Attack 8 — Picker test fixture parity

**Hypothesis (vector 8):** Did the API ripple correctly update `picker_test.go`? Are golden tests still consistent?

**Trace:**

1. `picker_test.go:1-29` (renamed `TestProfilePickerSelectsFirst` → `TestProfilePickerSelectsProfile`):
   - Profiles now include `Provider: domain.ProviderCodex` explicitly.
   - Assertion changed from `selected != "dev"` → `selected.Name != "dev"` and added `selected.Provider != domain.ProviderCodex`.
   - Test verifies the Provider round-trips through `Selected()` correctly.

2. `golden_test.go:25-55` (`TestProfilePickerGolden`, `TestProfilePickerGoldenClaude`): these tests do `final.View().Content` rendering — they don't call `Selected()`. The signature change to `Selected()` doesn't touch the View rendering pathway. Goldens unchanged. `mage golden` confirms 24/24 tracked goldens GREEN, including both Codex and Claude picker goldens.

3. `model_test.go:62, 100` calls `updated.Selected()` on the OTHER `Model` type (the action picker, not the profile picker). Different type. Unchanged.

**Verdict:** REFUTED — picker tests updated correctly; goldens unaffected because the View path is independent of `Selected()`.

### Attack 9 — Hardcoded provider names in 0-accounts error (future fragility)

**Hypothesis (vector 9):** R2's 0-accounts error hardcodes `"codex"` and `"claude"`. If a third provider lands, the message becomes incomplete.

**Trace:** `manage.go:996`:

```go
return "", "", fmt.Errorf("no accounts found across any provider; run `valv manage account add codex <name>` or `valv manage account add claude <name>` to create one")
```

True — hardcoded two-provider list. Adding `ProviderAnthropicConsole` or any future third provider would require updating this string. Could be rewritten to iterate `supportedProviders()` dynamically.

**Verdict:** REFUTED as R2 regression; flagged as FUTURE FRAGILITY for post-v0.1.0 multi-provider work. Per spawn prompt: "Acceptable for now (pre-v0.1.0 scope) but flag as future fragility." Not a blocker. Suggest a TODO comment routed to DROP_9 (CLI audit) or whichever drop adds the third provider.

### Attack 10 — `pickProfileFn` default-init order

**Hypothesis (vector 10):** Is `pickProfileFn = realPickProfile` guaranteed to initialize before any function references it?

**Trace:** Both `pickProfileFn` (var) and `realPickProfile` (func) live in the SAME file (`operator_helpers.go`). Go function declarations are available at package-initialization time regardless of textual order — functions are first-class values with their addresses resolved at compile time, not at init time. Package-level var initialization with `var x = someFunc` (where `someFunc` is a package-level function declaration) is always safe.

Additionally, even if `realPickProfile` were in another file, Go's spec guarantees that variable initialization expressions referencing package-level function declarations work — functions are bound before var initializers run.

**Verdict:** REFUTED — no init-order hazard. Functions are bound before var init begins.

### Mage verification

- `mage testPkg ./internal/cli` — PASS (167 tests, 73.1% coverage, `-race` on).
- `mage testPkg ./internal/tui/manage` — PASS (8 tests, 91.5% coverage, `-race` on).
- `mage golden` — PASS (24 tracked + 1 external transcript).

### Summary

| # | Attack | Verdict |
|---|---|---|
| 1 | BLOCKER (R1 CX1) picker dead-end on duplicate `(name, home)` | REFUTED (impossible in production; provider-rooted homes) |
| 2 | Small leak (R1 CX2) `Provider("all")` in 0-accounts / cancel | REFUTED (clean error + clean cancel; picker-title deferred to DROP_9) |
| 3 | Nit-2 `args := args` removal | REFUTED (line gone, tests green) |
| 4 | Injection seam race / parallel test interleaving | REFUTED (non-parallel test serialized by Go testing semantics) |
| 5 | `crossProviderLister` interface narrowing ripple | REFUTED (one consumer; aligned) |
| 6 | Picker enter-handler robustness against UI text drift | REFUTED (matches struct fields, not rendered text) |
| 7 | API ripple completeness | REFUTED (7/7 callsites migrated) |
| 8 | Picker test fixture parity | REFUTED (picker_test.go updated; goldens unaffected) |
| 9 | Hardcoded provider names in 0-accounts error | REFUTED as regression (future fragility flagged) |
| 10 | `pickProfileFn` default-init order | REFUTED (Go spec guarantees) |

**No CONFIRMED counterexamples.** All R1 BLOCKER + small leak + nit-2 fixes land cleanly. R2 introduces no new defects.

**Unknowns:** none.

**Future fragility flags (non-blocking, do NOT block R2 closure):**

1. Picker match-back keys on `(name, home)` tuple — if a future Valv refactor allows two profiles to share that tuple, the picker silently picks the first match. Defensible to add a uniqueness invariant check at picker construction OR switch match key to a profile index. Route to DROP_9 audit if dev wants hardening.
2. 0-accounts error hardcodes `codex` and `claude`. Iterate `supportedProviders()` when a third provider is added.
3. Picker title still says `"all accounts"` (DEFERRED to DROP_9 per R1 routing — flagged only for orchestrator visibility, NOT raised as R2 issue).

## Hylla Feedback (Unit 8.2 Round 2)

N/A — Hylla unreachable per spawn paradigm override; all evidence gathered via `Read`, `git grep`, `git diff`, Context7-not-needed (no external library questions), and `mage testPkg` / `mage golden`. No tool-shape gripes.

## Unit 8.3 — Round 1

**Verdict:** `pass-with-concerns`

**Builder commit under attack:** `25f252b feat(cli): unit 8.3 strip --account flag and thread override` (HEAD).

**Scope reviewed:** `internal/cli/account_flag.go` (new), `internal/cli/account_flag_test.go` (new), `internal/cli/claude.go`, `internal/cli/codex.go`, `internal/cli/claude_auth.go`, `internal/cli/codex_setup.go`, `internal/cli/account_auth.go`, and the three mechanical test updates (`claude_auth_test.go`, `codex_setup_test.go`, `codex_test.go`).

**Build verification:** `mage testPkg ./internal/cli` → 184 tests, 0 failed, coverage 72.6% (above the 60% floor). Confirmed prior baseline by checking out `HEAD~1`: 167 tests at 73.1%. Coverage delta is −0.5% across +17 net tests.

### Attack 1 — `stripAccountFlag` edge cases beyond the test matrix

**Hypothesis:** First-match-wins, malformed inputs, and unusual values produce incorrect behavior or panic.

**Verdict:** REFUTED (all sub-cases). Live edge-case traces:

| Input | Output | Correct? |
|---|---|---|
| `["--account", "work", "--account", "other"]` | `("work", ["--account", "other"])` | First wins; second preserved. Matches the documented contract and `account_flag_test.go:73`. |
| `["foo", "--account"]` | `("", ["foo", "--account"])` | Last-token-malformed returns original args via `return "", args` (line 42). Tested at `account_flag_test.go:49`. |
| `["--account="]` | `("", ["--account="])` | `len(arg) > 10` is false (it's exactly 10), then `arg == "--account"` is also false. Falls through to `return "", args` at line 54. Test at `account_flag_test.go:55`. |
| `["--account=--account"]` | `("--account", [])` | `value = "--account"` is non-empty; treats the literal string `--account` as the account name. Downstream `ProfileByName("--account")` will return `ErrNotFound` — surfaces as user-facing error, no panic. |
| `["--account", ""]` | `("", ["--account", ""])` | Empty value branch (line 45) returns original args. Edge case not tested explicitly but covered structurally. |
| `["--account=", "--account", "work"]` | `("work", ["--account="])` | Malformed `--account=` is skipped (passes the strict `>` length check at line 25), `--account work` resolves at i=1. Acceptable behavior; the malformed token survives in remaining and is passed to claude/codex unchanged. |
| `["foo", "--account=", "bar"]` | `("", ["foo", "--account=", "bar"])` | Malformed `--account=` in middle preserves args entirely (no flag extraction). Untested but consistent with the `>` length check. |

**Concrete trace of input-slice non-mutation:** `remaining = make([]string, 0, len(args)-1)` allocates a new backing array, then `append(remaining, args[:i]...)` + `append(remaining, args[i+1:]...)` copies. No aliasing. Confirmed by `TestStripAccountFlagDoesNotMutateInputSlice`. No race risk.

### Attack 2 — `--` escape hatch correctness

**Hypothesis:** Terminator handling diverges between forms or positions.

**Verdict:** REFUTED. Three cases verified:

1. `["--", "--account", "work"]` → returns `("", ["--", "--account", "work"])`. Test at `account_flag_test.go:84-88`.
2. `["foo", "--", "--account", "work"]` → at i=1 `arg == "--"`, return original args. Untested but trivially correct (early return at line 21).
3. `["--account", "work", "--"]` → at i=0 `arg == "--account"`, next=1, value="work". Returns `("work", ["--"])`. Tested at `account_flag_test.go:96` (variant with `"passthrough"` after `--`). The terminator-AFTER-flag case lands in the extracted branch BEFORE the terminator check, so behavior is correct (account stripped, `--` and trailing args preserved).

### Attack 3 — Override profile resolution failure path (ErrNotFound)

**Hypothesis:** `service.ProfileByName(ctx, Provider, override)` returning `ErrNotFound` panics, leaks DB handles, or surfaces an unreadable error.

**Verdict:** REFUTED. Trace for Claude path (claude.go:75 → claude_auth.go:132):

1. `openManageService(cmd, options.Paths)` is called (claude_auth.go:127) — error propagates wrapped as `"initialize manage service for account override: ..."`.
2. On success, `defer closeStore()` registered.
3. `service.ProfileByName(ctx, ProviderClaude, accountOverride)` returns `(domain.Profile{}, wrapErr)` where `wrapErr` wraps `domain.ErrNotFound` via `manage/service.go:189`.
4. Wrapped twice more: `ensureClaudeAccountReady` wraps with `"resolve override account %q: %w"`; `runClaudeCommand` wraps with `"run claude command: %w"`. Total surface: `"run claude command: resolve override account "missing": lookup profile "claude"/"missing": not found"`.
5. Store closes via deferred `closeStore()`. No leak. No panic.

Same trace for Codex path (codex.go:230 and codex_setup.go:37). All three paths preserve `errors.Is(err, domain.ErrNotFound)` chain integrity via `%w`. REFUTED.

### Attack 4 — `account_auth.go:42` caller passes `""`

**Hypothesis:** `ensureManagedAccountReady` dispatches `ensureClaudeAccountReady(cmd, account, options, "")` — but should it pass `account.Name` to ALWAYS resolve via override path?

**Verdict:** REFUTED — `""` is correct.

`ensureManagedAccountReady` is called from `loginBindAndReportCodexSetup` (codex_setup.go:141), `manage.go:623` (manage account login), `manage.go:729`, `manage.go:1032`. In every call site, the `account domain.Profile` has already been resolved (looked up by name via `ProfileByName` or constructed via `CreateProfile`). Passing the name as override would force a redundant store lookup and double-wrap errors. Passing `""` correctly causes `ensureClaudeAccountReady` to use the already-resolved `account` value directly.

### Attack 5 — `runClaudeCommand` threads `domain.Profile{}` when override is non-empty

**Hypothesis:** `claude.go:75` calls `ensureClaudeAccountReady(cmd, domain.Profile{}, …, accountName)` with a zero `domain.Profile`. If anything in the override branch reads `account.HomePath` or `account.Name` BEFORE `account = resolved` at claude_auth.go:136, the zero profile causes silent misbehavior.

**Verdict:** REFUTED. Trace:

`claude_auth.go:125` entry → `if accountOverride != ""` true → lines 127-137 resolve and replace `account = resolved`. Only AFTER line 137 does any code read `account.HomePath` (line 141), `account.Name` (line 147), or pass `account` to runner/logger. The branch correctly fully replaces the zero profile before use. REFUTED.

### Attack 6 — Multiple `--account` flags with mixed positional args

**Hypothesis:** `["claude", "--account", "work", "other-arg", "--account=different"]` produces incorrect remaining or skip-binding detection.

**Verdict:** REFUTED.

Trace: i=0 "claude" no match; i=1 "--account" matches, next=2, value="work". Returns `("work", ["claude", "other-arg", "--account=different"])`. First wins, second `--account=different` is preserved in remaining and forwarded to the upstream claude CLI inside the container. Claude CLI does NOT recognize `--account` so it would surface as an unknown-arg error from claude itself. That's a UX-surprise edge case but explicitly within the documented "first match wins" contract — design choice, not a bug. Documented at `account_flag.go:10-11`.

Secondary trace for `claudeArgsSkipProjectBinding(args)` after strip: `args = ["claude", "other-arg", "--account=different"]`. No `--help`/`-h`/`--version`/`-V`/`help` match. Skip-binding returns false. Correct — flag-with-override should still attempt project bind/runtime.

### Attack 7 — `DisableFlagParsing: true` preservation

**Hypothesis:** Builder accidentally toggled `DisableFlagParsing` to false during the refactor.

**Verdict:** REFUTED. Confirmed via `git grep DisableFlagParsing -- internal/cli/`:
- `claude.go:46`: `DisableFlagParsing: true` (unchanged from HEAD~1 — confirmed via `git diff HEAD~1 -- internal/cli/claude.go` shows only RunE-internal changes, no field toggles).
- `codex.go:52`: `DisableFlagParsing: true` (unchanged).

### Attack 8 — `service.ProfileByName` signature

**Hypothesis:** Function does not exist with the signature `(ctx, Provider, name) (Profile, error)`.

**Verdict:** REFUTED. `internal/services/manage/service.go:186`:
```go
func (s Service) ProfileByName(ctx context.Context, provider domain.Provider, name string) (domain.Profile, error)
```
All three new call sites (claude_auth.go:132, codex.go:230, codex_setup.go:37) match.

### Attack 9 — Coverage delta investigation

**Hypothesis:** New override-resolution branches in `ensureClaudeAccountReady`, `ensureBoundCodexAccountReady`, and `ensureCodexBindingReady` are LIVE production code today but lack tests, accounting for the −0.5% coverage drop.

**Verdict:** CONFIRMED (concern, not blocker).

**Repro:** `git grep -nE "accountOverride" -- internal/cli/*_test.go` returns zero matches. Every test in the diff (`claude_auth_test.go`, `codex_setup_test.go`, `codex_test.go`) only updates the trailing arg to `""` — all 17 new tests are in `account_flag_test.go` exercising `stripAccountFlag` in isolation.

**Uncovered live branches:**

| File:Line | Branch | Test coverage |
|---|---|---|
| `claude.go:74-78` | `if accountName != "" { ensureClaudeAccountReady(...) }` | none |
| `claude.go:83` | `skipValidate := accountName != ""` (and `if !skipValidate` at 130) | none |
| `claude_auth.go:126-137` | `if accountOverride != ""` — both success (resolved!=err) and error (`ProfileByName` returns `ErrNotFound`) | none |
| `codex.go:82` | `ensureBoundCodexAccountReady(..., accountName)` with non-empty accountName | none |
| `codex.go:129` | `if accountName == ""` skip-ValidateBinding gating | none |
| `codex.go:229-238` | `accountOverride != ""` resolution including `ensureManagedAccountReady` invocation | none |
| `codex_setup.go:36-41` | `accountOverride != ""` ProfileByName branch | none |

**Why this is `pass-with-concerns` not `fail`:** Unit 8.3's acceptance criteria at PLAN.md line 92 explicitly enumerate the test cases required ("no flag present, `--account work`, `--account=work`, `--account` as the last token (malformed), `--account` in the middle of other args, and the case where `--account` appears multiple times (first match wins)") — all about `stripAccountFlag` in isolation. The override-resolution branches' test coverage is explicitly deferred to Unit 8.4 (PLAN.md line 129: "Tests in `claude_setup_test.go` cover: ... `accountOverride` path") and Unit 8.5 (line 165: "Cover: override (unbound project), override (bound project), …"). Unit 8.3 meets its stated acceptance bar.

**Remediation routing:** Surface to Unit 8.4 and 8.5 planning: ensure their `accountOverride` tests actually exercise `ensureClaudeAccountReady`'s override-resolution branch and `ensureCodexAccountReadyForLaunch`'s override-resolution branch (not just the higher-level `ensureClaudeBindingReady` / `ensureCodexAccountReadyForLaunch` wrappers). The 7 branches listed above must reach coverage by end of 8.5 or sooner.

### Attack 10 — Override + already-authed combination

**Hypothesis:** When override is non-empty AND override profile has valid `.credentials.json`, the function might double-call docker or short-circuit incorrectly.

**Verdict:** REFUTED.

Trace `ensureClaudeAccountReady`:
1. Line 126: `accountOverride != ""` true. Lines 127-137 resolve via store, replace `account = resolved`.
2. Line 138: `SkipLogin` false (Paths-only options).
3. Line 141: `credPath = resolved.HomePath/.credentials.json`. `os.Stat` succeeds, size > 0.
4. Line 143: return nil. Container auth skipped. Correct.

Trace `ensureBoundCodexAccountReady`:
1. Line 218: skip-account-ready check first (args-based).
2. Line 221: `openManageService` ok.
3. Line 229: `accountOverride != ""` true. Resolve profile.
4. Line 234: `ensureManagedAccountReady` → `ensureCodexAccountReady` → `runner.LoginStatus(account.HomePath)`. If logged in, returns nil at line 83.

Both paths correctly short-circuit on already-authed. REFUTED.

### Attack 11 — Builder mid-round fix-up (duplicate declaration)

**Hypothesis:** Prompt mentions "mid-round fix-up that resolved a duplicate-declaration compile error" — this might indicate sloppy refactor with residual issues.

**Verdict:** REFUTED. Final commit compiles cleanly (`mage testPkg ./internal/cli` succeeds, 184 tests, 0 failures). The fix-up is squashed into commit `25f252b` (no separate fix commit between R1 builder commit and current HEAD per `git log --oneline -5`). No residual issue.

### Attack 12 — codex.go ordering: ValidateBinding vs override skip

**Hypothesis:** `codex.go:129-133` is the skip-ValidateBinding guard for override; but `service.Run` at line 135 might internally call ValidateBinding again, defeating the skip.

**Verdict:** REFUTED. Reading `internal/services/codex/service.go` (not modified in this unit), `service.Run(ctx, workingDir, args)` does not internally call `ValidateBinding` — that is a separate explicit method. The override-skip is correct.

**However a related concern:** when `accountName != ""`, `ensureBoundCodexAccountReady` (codex.go:82) runs BEFORE `service.Run` (line 135). The override branch in `ensureBoundCodexAccountReady` (line 229-238) calls `ensureManagedAccountReady` on the override profile — which runs `LoginStatus` against the override profile's `HomePath`. That's correct: the override identity gets the host-auth check it needs. But `service.Run` will then proceed using the project's BOUND profile (not the override) because no override-threading exists in `codexservice.Options` yet — that's Unit 8.5's job. Today, a `valv codex --account work` invocation with a project bound to "other" will:
- Auth-check `work`'s HomePath on the host.
- Then run codex inside docker pointed at `other`'s HomePath via `service.Run`'s reading of the binding.

**Result:** transitional mis-wiring — `--account` is partially honored (host auth check uses override) but the container itself uses the bound account's home. Unit 8.5 will fix this by threading the resolved profile into `codexservice.Options`. Same gap exists for claude (Unit 8.4).

This is NOT a Unit 8.3 acceptance violation: Unit 8.3 PLAN.md explicitly defers the runtime threading to Units 8.4/8.5 ("The extracted `accountName` is threaded through to ... binding-ready call (introduced in Unit 8.4)" / "merged `ensureCodexAccountReadyForLaunch` function (introduced in Unit 8.5)"). But it IS a live UX gap between Unit 8.3 closing and 8.4/8.5 closing — a user who runs `valv claude --account work` mid-drop will see the override host-auth-checked but container-launched with the bound profile. The override appears partially honored.

**Remediation:** orchestrator should ensure no dogfooded `valv claude --account` / `valv codex --account` happens between now and Unit 8.4/8.5 completion. Drop's `state: building` already enforces this. Documented as a known transitional gap, not a counterexample.

### Convergence

- **Attack families exhausted:** stripAccountFlag edge cases (1), `--` escape (2), override resolution failure (3), caller correctness (4-5), multi-flag precedence (6), `DisableFlagParsing` preservation (7), signature verification (8), coverage delta (9), already-authed combination (10), residual-defect probe (11), runtime mis-wiring (12).
- **CONFIRMED counterexample:** Attack 9 — uncovered override-resolution branches account for the −0.5% coverage drop. Not a blocker because Unit 8.3's stated acceptance covers `stripAccountFlag` only, and override-branch test coverage is explicitly deferred to Units 8.4/8.5.
- **Transitional gap:** Attack 12 — host auth-check uses override profile but `service.Run` uses bound profile until Units 8.4/8.5 thread the override into `claudeservice.Options` / `codexservice.Options`. Acceptable as a closed-drop transitional state.
- **Remaining Unknowns:** none. All applicable attack vectors evaluated against concrete code, not memory.

### Verdict & remediation

**Verdict:** `pass-with-concerns`.

Unit 8.3 meets every stated acceptance criterion (PLAN.md lines 87-96). `mage testPkg ./internal/cli` green. `stripAccountFlag` test matrix complete (15 cases + non-mutation invariant). `DisableFlagParsing: true` preserved. `ProfileByName` signature confirmed. Override resolution wired into `runClaudeCommand` / `runCodexCommand`.

**Concerns flagged for orchestrator routing:**

1. **C1 (CONCERN, route to 8.4 / 8.5 planning):** 7 live override-resolution branches across 3 functions have no test coverage today. Unit 8.4 + 8.5 acceptance criteria already require override-path tests; orchestrator must verify those tests EXERCISE the actual `accountOverride != ""` branches in `ensureClaudeAccountReady` / `ensureBoundCodexAccountReady` / `ensureCodexBindingReady` (not just the higher-level wrappers that 8.4/8.5 introduce).
2. **C2 (CONCERN, document as known transitional gap):** Between Unit 8.3 close and Unit 8.4/8.5 close, `valv claude --account X` / `valv codex --account X` will host-auth-check `X` but container-launch with the project's bound profile (because runtime threading is 8.4/8.5's responsibility). No dogfood smoke test of `--account` should occur until 8.4/8.5 land. Drop's `state: building` already gates this.

## Hylla Feedback (Unit 8.3 Round 1)

N/A — Hylla unreachable per spawn paradigm override; all evidence gathered via `Read`, `git grep`, `git diff`, and `mage testPkg`. Context7 not needed (no external library questions — cobra `DisableFlagParsing` semantics are documented inline in the unit). No tool-shape gripes.

## Unit 8.4 — Round 1

**Verdict:** `pass-with-concerns`

**Builder commit under attack:** `aa11233 feat(cli): unit 8.4 claude binding UX + override threading`

**Scope reviewed:** `internal/cli/claude.go`, `internal/cli/claude_setup.go` (NEW), `internal/cli/claude_setup_test.go` (NEW), `internal/cli/claude_test.go`, `internal/services/manage/service.go`, `internal/services/manage/service_test.go`, `internal/services/claude/service.go`, `internal/services/claude/service_test.go`. Also read `internal/adapters/providers/claude/runtime.go` (for vector 1 mount-time semantics), `internal/cli/operator_helpers.go` (vector 8 picker cancel), `internal/cli/codex_setup.go` (vector 5 / standards-violation cross-check), `internal/adapters/sqlite/store.go::BindingByProjectID` (vector 2 SQL-level provider filter), `internal/domain/errors.go` (vector 5 sentinel text), `internal/services/manage/service.go::BindProject` (vector 9 upsert semantics).

### Attack 1 — Override + skip-ValidateBinding without HomePath existence check

**Hypothesis:** New override path skips `service.ValidateBinding`. If the resolved override profile's `HomePath` doesn't exist on disk (manually deleted, never created), the container launch fails at mount time. Override path doesn't validate existence before launching.

**Verdict:** REFUTED.

**Trace:**
- `ValidateBinding` (`claude/service.go:219-222`) just calls `resolveBinding` — DB lookups only, **does NOT check HomePath existence on disk**. So the premise of the attack ("ValidateBinding would have caught a missing HomePath") was false.
- `PrepareRuntime` (`internal/adapters/providers/claude/runtime.go:82`) does `os.MkdirAll(sharedHome, 0o755)` on the profile home before mount construction. This silently CREATES the directory if missing. Both bound and override paths go through the same `PrepareRuntime` → both share the same `MkdirAll`-on-missing semantics.
- Result: a typo'd / missing-on-disk override HomePath does not fail at mount time; instead Docker mounts a freshly-created empty directory. This is suboptimal UX (user mounts the wrong dir silently) but **is NOT a regression introduced by 8.4** — it is the pre-existing behavior of the bound-profile launch path. The override path inherits it.

**Counterexample status:** none against 8.4. Pre-existing UX gap (not unit 8.4's responsibility). No remediation required by 8.4.

### Attack 2 — `StatusForProvider` line-by-line symmetry with `Status`

**Hypothesis:** Builder claims surgical clone. Look for divergence beyond the provider parameter.

**Verdict:** REFUTED.

**Trace:**
- `Status` (`internal/services/manage/service.go:231-262`) vs `StatusForProvider` (`:264-302`). Diff: only line that differs is `BindingByProjectID(..., domain.ProviderCodex)` → `BindingByProjectID(..., provider)`.
- Error wrapping format strings identical (`"manage status: project %q: %w"`, etc.).
- Project-record lookup identical (`ProjectByRoot`, `ErrNotFound` → `ErrUnboundProject` wrap).
- `BindingByProjectID` SQL filters by `(project_id, provider)` (`internal/adapters/sqlite/store.go:429`) — so the provider parameter is correctly propagated all the way to the SQL `WHERE` clause.
- Identity-attribute population: `Profile` populated via `ProfileByID(binding.ProfileID)` — same code path for both.
- Tests `TestStatusForProviderReturnsBoundProjectDetailsForClaude` and `TestStatusForProviderReturnsUnboundProjectWhenCodexBoundButNotClaude` cover the positive and negative provider-filter cases.

**Counterexample status:** none. Clean clone with sole intended divergence.

### Attack 3 — `unboundProjectNoAccountsError` helper format / sentinel leak

**Hypothesis:** Verify same error format across providers; mentions `valv manage account add <provider>` correctly; does NOT leak any sentinel like `Provider("all")`.

**Verdict:** REFUTED.

**Trace:**
- Helper (`claude_setup.go:17-22`): `fmt.Errorf("project is not bound; no %s accounts found — run \`valv manage account add %s\` to create one", provider, provider)`.
- Single call site in the diff: `unboundProjectNoAccountsError(domain.ProviderClaude)` (`claude_setup.go:79`). Sub-output: `"project is not bound; no claude accounts found — run \`valv manage account add claude\` to create one"`. Correct.
- No `Provider("all")` literal exists in the diff. `git grep` for `Provider("all")` in `internal/cli/` returns zero matches. No sentinel leak.
- Helper is reusable for Codex (`unboundProjectNoAccountsError(domain.ProviderCodex)` would produce equivalent codex-flavored message), satisfying the 8.4/8.5 cross-provider format requirement.

**Counterexample status:** none.

### Attack 4 — `OverrideProfile` field nil-safety in `Run`

**Hypothesis:** Are there code paths where the nil check is missed? Especially in `Run`'s prelude.

**Verdict:** REFUTED.

**Trace:**
- `Run` (`internal/services/claude/service.go:128-157`) opens with `var resolved resolvedLaunchBinding; if s.overrideProfile != nil { ... } else { resolved, err = s.resolveBinding(...) ... }`. The nil check is the FIRST thing `Run` does — no code path touches `s.overrideProfile` without the guard.
- Only `s.overrideProfile != nil` branch dereferences via `*s.overrideProfile` (line 149). Safe.
- `New` simply assigns `options.OverrideProfile` into `Service.overrideProfile` (line 119) — zero-value is `nil`, so callers who don't set `OverrideProfile` get the resolveBinding path automatically. Backwards-compatible.

**Counterexample status:** none.

### Attack 5 — Non-`ErrUnboundProject` error wrapping test coverage

**Hypothesis:** Builder claim — non-unbound errors wrap and return immediately. Find the test. Does it actually inject a non-unbound error and assert the wrapped message?

**Verdict:** **CONFIRMED counterexample, severity LOW (CONCERN).**

**Trace:**
- `claude_setup.go:65-67` — branch `if !errors.Is(err, domain.ErrUnboundProject) && !strings.Contains(err.Error(), domain.ErrUnboundProject.Error())` wraps as `"detect claude binding: %w"`.
- Six tests in `claude_setup_test.go` cover: override-unbound, override-bound, zero-accounts, one-account-auto-bind, multi-non-TTY, already-bound, helper-format. **NO test injects a non-`ErrUnboundProject` error** (e.g., a `domain.ErrIO`-style store failure) to verify the `"detect claude binding: %w"` wrap fires correctly.
- Why this branch is hard to test: `ensureClaudeBindingReady` opens a real SQLite store via `openManageService`. A store failure path is reproducible by closing the store before the call or by injecting a corrupted DB file, but no test does either.
- This is the same kind of uncovered-branch concern raised in 8.3 C1, scoped tighter (one branch in one new function).

**Remediation:** add a test that pre-corrupts `paths.DatabasePath` (e.g., `os.WriteFile(paths.DatabasePath, []byte("not a sqlite file"), 0o644)` AFTER `testCodexPaths` returns) and asserts the error wraps with `"detect claude binding"`. LOW severity because the branch is short, defensive, and the wrap format matches existing patterns. Route to drop close-out, not to a new build round.

### Attack 6 — C2 end-to-end test fidelity (`TestRunUsesOverrideProfileHomePath`)

**Hypothesis:** Test must go through the FULL `Run` path (build `ContainerRunRequest` from override profile) and assert mounts, not just check that `OverrideProfile.HomePath != ""` at some intermediate step.

**Verdict:** REFUTED.

**Trace:**
- Test (`service_test.go:553-614`) constructs a service with `boundClaudeStore(project, boundHome)` (different from override home) and `OverrideProfile: &overrideProfile` (HomePath = `overrideHome`).
- Calls `service.Run(context.Background(), "/tmp/project", []string{"--prompt", "hello"})`. This is the REAL `Run` method, not a stub.
- `Run` → override branch (`overrideProfile != nil`) → `s.detect()` → `s.store.ProjectByRoot()` → `PrepareRuntime(ProfileHome: overrideHome)` → `buildRequest(...)` → `executor.Run(request)`.
- Assertion: walks `executor.got.Mounts`, fails if any mount's `Source` equals `boundHome`, fails if no mount's `Source` equals `overrideHome`. **The mount spec is the actual container-launch input, not an intermediate value.** This is end-to-end fidelity.
- `filepath.EvalSymlinks` is applied to both home tempdirs (handles macOS `/var` → `/private/var` symlink discrepancy that would otherwise produce false negatives). Good defensive code.

**C2 carry-forward (from 8.3):** RESOLVED.

### Attack 7 — 2+ accounts non-TTY error doesn't list available accounts

**Hypothesis:** AC says: non-TTY → error pointing to `valv manage bind claude <name>`. Does the error LIST the available accounts so the user knows the names?

**Verdict:** REFUTED-with-polish-item.

**Trace:**
- `claude_setup.go:95-99`: error format is `"project is not bound to a Claude account; run \`valv manage bind claude <name>\` to bind one"`. No account names listed.
- `TestEnsureClaudeBindingReadyMultipleAccountsNonTTY` asserts substring `"valv manage bind claude"` — passes.
- AC (PLAN.md unit 8.4 lines around `non-TTY → error`) — verified to require only the `valv manage bind claude` pointer, not enumeration. The implementation matches the spec exactly.
- Polish-item rationale: user can `valv manage list claude` (or whatever the list command is) to find names. Two-step user flow is acceptable for a non-TTY error path; this is not on the happy path.

**Counterexample status:** spec-conformant. Polish item routed to DROP_9 CLI audit (see project memory: `project_valv_cli_audit_proposal`).

### Attack 8 — Picker cancel handling in `ensureClaudeBindingReady`

**Hypothesis:** `pickProfile` now returns `(domain.Profile, error)` per 8.2 R2. Does `ensureClaudeBindingReady`'s picker branch correctly distinguish picker-cancel from bind-failure?

**Verdict:** REFUTED-with-polish-item.

**Trace:**
- `realPickProfile` (`operator_helpers.go:165-188`) returns `errSelectionCanceled` on cancel.
- `ensureClaudeBindingReady` line 101: `if err != nil { return domain.Profile{}, fmt.Errorf("select claude account: %w", err) }` — wraps the cancel error with `"select claude account: %w"`, preserving the underlying `errSelectionCanceled` via `%w`. Caller (`runClaudeCommand`) returns wrapped as `"run claude command: %w"`. Final user-visible message: `"run claude command: select claude account: <cancel string>"`.
- This is informative but not pretty. A cleaner approach would be: `if errors.Is(err, errSelectionCanceled) { return domain.Profile{}, errSelectionCanceled }` — let Cobra suppress the user-canceled trace. **Not a correctness issue, UX polish.**

**Counterexample status:** Picker-cancel is correctly distinguishable via `errors.Is(err, errSelectionCanceled)` because `%w` preserves it. Polish item: consider passing cancel through unwrapped in 8.5 / future polish round.

### Attack 9 — Concurrent first-run bind race

**Hypothesis:** Two concurrent `valv claude` invocations in the same project race on first-run bind. Does `BindProject` use upsert?

**Verdict:** REFUTED with documented gap.

**Trace:**
- `BindProject` (`internal/services/manage/service.go:194-229`):
  - Line 200: `ProjectByRoot` — read.
  - Line 209: `s.store.CreateProject(...)` if not found — **NOT upsert**. Concurrent invocations on a never-created project race here: one wins, the other gets a uniqueness-violation error.
  - Line 223: `s.store.UpsertProjectBinding(binding)` — **upsert**. Concurrent invocations on an existing project (binding row write) are safe; last-writer-wins on the profile assignment.
- Concrete race window: two concurrent `valv claude` invocations from a project that has never been bound AND never had a project record. One creates the project; the other gets `CreateProject` failure. The failing invocation's wrap: `"bind project: persist project %q: %w"` — wraps the SQLite uniqueness-violation error.
- Probability: very low in practice (`valv claude` is an interactive CLI; users rarely launch two concurrently as the very-first action in a project). User-visible: the loser gets a confusing SQL-looking error.
- 8.4 acceptance criteria don't mention concurrency. This is a pre-existing concern in `BindProject`, not introduced by 8.4.

**Counterexample status:** documented pre-existing gap. ACCEPT for 8.4. Route to a future cleanup drop if user reports.

### Attack 10 — `OverrideProfile` set even for already-bound case (bypasses `Run`'s validation)

**Hypothesis:** `runClaudeCommand` passes `OverrideProfile: &resolvedProfile` UNCONDITIONALLY. This bypasses `Run`'s `resolveBinding` validation EVEN for bound projects, which the attack prompt claims is wrong.

**Verdict:** REFUTED on the surface (intentional design), but a defensive-check gap is CONFIRMED.

**Trace:**
- `runClaudeCommand` (`claude.go` post-diff line 109): `OverrideProfile: &resolvedProfile` — always non-nil.
- This is **the new design**: `ensureClaudeBindingReady` does the resolution; `Run` trusts the profile. The old `service.ValidateBinding` call (8.3 carryover) was REMOVED in this diff.
- Semantic equivalence check: old path resolves via `resolveBinding` (project + binding-by-(id, provider) + profile-by-ID + provider checks); new path for bound projects resolves via `StatusForProvider` (project + binding-by-(id, provider) + profile-by-ID). The DB queries are identical except `resolveBinding` adds two redundant provider-equality assertions on lines 257 and 268 of the old code, which `StatusForProvider`'s SQL-level provider filter already enforces for `binding.Provider`. **`profile.Provider` is NOT checked** by `StatusForProvider`, but neither is it checked by the override `Run` path. This is the defensive-check gap.
- Defensive-check gap rationale: `Run`'s override branch (lines 130-150) trusts that `s.overrideProfile.Provider == domain.ProviderClaude`. Today's only caller (`runClaudeCommand`) only ever passes Claude profiles (sourced from `ProfileByName(ProviderClaude)`, `StatusForProvider(...,ProviderClaude).Profile`, or `ListProfiles(ProviderClaude)`). A future caller could pass a non-Claude profile and `Run` would silently mount it. CONCERN, severity LOW.

**Counterexample status:** the intentional-design claim is correct (no behavioral regression). Defensive-check gap is a real CONCERN, routed as a hardening item.

### Attack 11 — `strings.Contains(err.Error(), ErrUnboundProject.Error())` violates AGENTS.md § 6

**Hypothesis:** AGENTS.md § 6 — "never string-match an error." `claude_setup.go:65` uses both `errors.Is` AND `strings.Contains(err.Error(), domain.ErrUnboundProject.Error())`. The string-match clause is a "belt-and-suspenders" defense, per the builder's worklog.

**Verdict:** **CONFIRMED counterexample against project standards, severity LOW (CONCERN).**

**Trace:**
- `domain.ErrUnboundProject.Error()` = `"project is not bound"` (`internal/domain/errors.go:8`).
- `unboundProjectNoAccountsError(provider).Error()` starts with `"project is not bound; no claude accounts found..."` — also contains `"project is not bound"`.
- The non-TTY 2+ accounts error starts with `"project is not bound to a Claude account..."` — also contains `"project is not bound"`.
- Any future error message accidentally containing `"project is not bound"` will be MIS-CLASSIFIED as `ErrUnboundProject` by the `strings.Contains` guard, triggering the unbound-project codepath erroneously.
- `errors.Is` alone is correct because `StatusForProvider` wraps via `%w` (lines 280 / 287 / 295 of `service.go`). The `strings.Contains` clause is genuinely redundant AND introduces a future-correctness risk.
- Builder admits this in the worklog: "the string-contains guard is a belt-and-suspenders defense." But the same pattern exists in `internal/cli/codex_setup.go:45` (pre-existing), so this is propagated tech debt, not new tech debt unique to 8.4.

**Remediation:** delete the `strings.Contains(...)` clause from BOTH `claude_setup.go:65` AND `codex_setup.go:45`. Keep only `errors.Is(err, domain.ErrUnboundProject)`. If a Codex test starts failing after that, the wrap chain is broken and needs fixing at the wrap site, not the receiver site. Route as a CONCERN for drop close-out polish, not a build-blocker on 8.4 alone (because deleting it from `claude_setup.go` only would create asymmetry with the codex side).

### Attack 12 — `DisableFlagParsing: true` invariant preservation

**Hypothesis:** 8.3's invariant — `DisableFlagParsing: true` on the claude command — must remain true after 8.4's rewire.

**Verdict:** REFUTED.

**Trace:**
- `git diff HEAD~1 -- internal/cli/claude.go` shows zero changes to `newClaudeCommand`'s `DisableFlagParsing` setting. The rewire is purely inside `runClaudeCommand`'s body.
- `TestNewClaudeCommandHelp` and `TestNewClaudeCommandVersion` (unchanged) still pass per builder's `mage testPkg ./internal/cli` report — verifying the help/version flags are still routed correctly.

**Counterexample status:** none.

### Attack 13 — Auto-open re-introduction

**Hypothesis:** Builder may have re-introduced auto-open machinery in some form (per project-memory item `feedback_manual_workflow_is_the_decision`).

**Verdict:** REFUTED.

**Trace:**
- `git diff HEAD~1 -- internal/cli/ internal/services/` — no `open.Run`, `exec.Command("open"`, browser-launch, or similar. The auto-bind path (1-account case) writes a binding row and a CLI notice, then returns. No external command spawned for "open browser" purposes.
- `writeCLINotice` (called on `claude_setup.go:84`) is a structured laslig notice writer — it writes to stderr, not a browser or external command.

**Counterexample status:** none.

### Attack family exhaustion summary

| # | Attack | Status |
|---|---|---|
| 1 | Override + skip-Validate HomePath check | REFUTED (premise was wrong; pre-existing MkdirAll behavior) |
| 2 | `StatusForProvider` divergence from `Status` | REFUTED |
| 3 | `unboundProjectNoAccountsError` sentinel leak | REFUTED |
| 4 | `OverrideProfile` nil-safety in `Run` | REFUTED |
| 5 | Non-`ErrUnboundProject` error test coverage | **CONFIRMED (CONCERN)** |
| 6 | C2 end-to-end test fidelity | REFUTED — C2 resolved |
| 7 | Non-TTY error doesn't list accounts | REFUTED (spec-conformant; polish) |
| 8 | Picker cancel handling | REFUTED (`errors.Is` works via `%w`; polish) |
| 9 | Concurrent first-run bind race | REFUTED (pre-existing gap, ACCEPT) |
| 10 | `OverrideProfile` always-non-nil bypass | REFUTED on design; defensive-check gap CONCERN |
| 11 | `strings.Contains` error match (AGENTS.md § 6) | **CONFIRMED (CONCERN)** |
| 12 | `DisableFlagParsing: true` preserved | REFUTED |
| 13 | Auto-open re-introduction | REFUTED |

**C1 (test gap from 8.3) — RESOLVED.** `TestEnsureClaudeBindingReadyOverrideUnboundProject` and `TestEnsureClaudeBindingReadyOverrideBoundProject` directly exercise the override branches in the new `ensureClaudeBindingReady`, asserting both behavior (resolved profile name) and side-effect absence (no binding row written for override path).

**C2 (runtime mis-wiring from 8.3) — RESOLVED.** `TestRunUsesOverrideProfileHomePath` exercises `Run` end-to-end with `OverrideProfile` set, asserting the executor's mount source is the override home and explicitly failing if the bound home leaks into mounts.

### Verdict & remediation

**Verdict:** `pass-with-concerns`.

Unit 8.4 meets every stated acceptance criterion. `mage testPkg` green across all three touched packages (manage / claude / cli) at builder report (75.5% / 79.4% / 71.8% coverage). C1 and C2 from 8.3 are both fully closed. `DisableFlagParsing: true` invariant preserved. No auto-open re-introduction. Override threading reaches `Run`'s mount construction (verified end-to-end via real `Run` invocation, not stubs).

**Concerns flagged for orchestrator routing:**

1. **C3 (CONCERN, route to drop close-out polish):** `claude_setup.go:65` and `codex_setup.go:45` use `errors.Is(err, ErrUnboundProject) || strings.Contains(err.Error(), ErrUnboundProject.Error())`. The `strings.Contains` clause violates AGENTS.md § 6 ("never string-match an error") and creates a future-correctness risk: any error whose message contains `"project is not bound"` (which includes `unboundProjectNoAccountsError`'s own output) is mis-classified as `ErrUnboundProject`. `errors.Is` alone is correct because `StatusForProvider` wraps via `%w`. Remediation: delete the `strings.Contains(...)` clause from BOTH files in a single follow-up commit. Verify by running `mage testPkg ./internal/cli` after deletion.

2. **C4 (CONCERN, route to a future Claude `Run` hardening drop):** `Run`'s override branch (`internal/services/claude/service.go:130-150`) trusts that `s.overrideProfile.Provider == domain.ProviderClaude`. Today's only caller passes Claude profiles. A future caller could mis-wire a non-Claude profile; `Run` would silently mount it. Remediation: add a defensive check `if s.overrideProfile != nil && s.overrideProfile.Provider != domain.ProviderClaude { return fmt.Errorf("run claude launch service: override profile provider %q: expected %q", s.overrideProfile.Provider, domain.ProviderClaude) }` near line 130. Low priority — no caller mis-wires it today.

3. **C5 (CONCERN, route to drop close-out test polish):** The non-`ErrUnboundProject` error branch in `ensureClaudeBindingReady` (`claude_setup.go:65-67`) has no test coverage. Remediation: add a test that corrupts `paths.DatabasePath` after `testCodexPaths` to force a non-unbound store error, then asserts the wrap `"detect claude binding: ..."` fires. Low severity — defensive branch with stable wrap format.

4. **Polish items (NOT BLOCK):**
   - Non-TTY 2+ accounts error does not enumerate available account names (Attack 7).
   - Picker-cancel error chain is informative but verbose — could pass `errSelectionCanceled` through unwrapped (Attack 8).
   - `BindProject`'s `CreateProject` is not upsert; two concurrent never-bound-project `valv claude` invocations race on the project-create (Attack 9). Pre-existing; not on 8.4's hook.

**No build round required for 8.4** — concerns are all polish-level and route to drop close-out (C3/C5) or a future drop (C4). Unit can flip to `done` after orchestrator review of these concerns.

## Hylla Feedback (Unit 8.4 Round 1)

N/A — Hylla unreachable per spawn paradigm override; all evidence gathered via `Read`, `git diff HEAD~1`, `git grep`, and direct file inspection. Context7 not needed (cobra `DisableFlagParsing`, `errors.Is`/`%w` semantics are stdlib-canonical and the diff is self-contained). No tool-shape gripes.

---

## Unit 8.5 — Round 1

**QA Falsification — `go-qa-falsification-agent`**
**Date:** 2026-05-18
**Verdict:** PASS — no CONFIRMED counterexamples. 1 CONCERN (intentional behavior shift worth surfacing) + 1 minor symmetric gap acknowledged.

**Evidence sources used:**
- `git diff HEAD~1 --stat` (8 files, 542 inserts / 311 deletes).
- `git diff HEAD~1` on `internal/cli/codex.go`, `internal/cli/codex_setup.go`, `internal/services/codex/service.go`, `internal/services/codex/service_test.go`.
- Full `Read` of post-merge `internal/cli/codex_setup.go` (120 lines), `internal/cli/codex.go` (291 lines), `internal/cli/claude.go` (224 lines), `internal/cli/claude_setup.go` (114 lines), `internal/services/codex/service.go` (385 lines), and `internal/services/claude/service.go` lines 115-174.
- `Read` of `internal/cli/codex_setup_test.go` (277 lines, 7 tests).
- `git show HEAD~1:internal/cli/codex.go` for the deleted `ensureBoundCodexAccountReady`.
- `git grep` over `internal/` for `readPrompt`, `errCodexSetupCanceled`, `runCodexFirstRunSetup`, `writeCodexSetupIntro` — all return ZERO matches in source.
- `mage testPkg ./internal/cli` — 195/195 PASS, 73.3% coverage, -race clean.
- `mage golden` — 24/24 tracked + 1/1 transcript PASS.
- `mage test` (full project verification) — 476/476 PASS across 20 packages.
- `internal/cli/operator_helpers.go` `pickProfile` + `realPickProfile` (line 161+) for picker cancel semantics.

**Attack-vector results (in spawn-prompt order):**

### V1 — Merge correctness (REFUTED)

Walked both old functions line-by-line against the merged function.

| Old responsibility | Old loc | New loc | Result |
|---|---|---|---|
| `ensureCodexBindingReady`: override → `ProfileByName` + return (no auth) | `codex_setup.go:31-37` (HEAD~1) | `codex_setup.go:46-51` then step 4 at line 114 | Covered — override now also runs through `ensureManagedAccountReady`, MATCHING the prior `ensureBoundCodexAccountReady` override branch (old `codex.go:229-238`). |
| `ensureCodexBindingReady`: bound (Status no error) → return | `codex_setup.go:39` | `codex_setup.go:55-58` | Covered — profile is captured for step 4. |
| `ensureCodexBindingReady`: non-unbound Status error → wrap | `codex_setup.go:40-42` | `codex_setup.go:59-60` | Covered — `errors.Is` only (justified — see V11). |
| `ensureCodexBindingReady`: unbound → `runCodexFirstRunSetup` (4-option menu) | `codex_setup.go:44-54` | `codex_setup.go:63-105` (0/1/2+ branching, auto-bind, picker) | REPLACED per PLAN.md §149-159 decision to drop the 4-option menu — matches Claude UX after Unit 8.4. |
| `ensureBoundCodexAccountReady`: skip-guard short-circuit | `codex.go:218-220` (HEAD~1) | `codex_setup.go:111-113` (step 5) | Covered — but moved AFTER profile resolution (see Concerns 1 below). |
| `ensureBoundCodexAccountReady`: override → `ProfileByName` + `ensureManagedAccountReady` | `codex.go:229-238` | `codex_setup.go:46-51` + step 4 line 114 | Covered. |
| `ensureBoundCodexAccountReady`: bound → `Status` + `ensureManagedAccountReady` | `codex.go:240-246` | `codex_setup.go:55-58` + step 4 line 114 | Covered. |

**No dropped responsibility.** REFUTED.

### V2 — `codexArgsSkipAccountReady` guard honored as step 5 (REFUTED)

`codex_setup.go:111`:
```go
if codexArgsSkipAccountReady(args) {
    return profile, nil
}
if err := ensureManagedAccountReady(cmd, profile.Provider, profile, accountAuthOptions{}); err != nil { ... }
```

Skip-guard short-circuits BEFORE `ensureManagedAccountReady`. Test exists at `codex_setup_test.go:245 TestEnsureCodexAccountReadyForLaunchSkipAccountReadyGuard`:
- Binds project to `bound-account`.
- Installs `installStubCodexAccountAuth` and explicitly asserts `stub.statusHits == 0` after calling with `args = ["login"]`.
- Verifies profile name is still returned (`bound-account`).

Test passes (`mage testPkg ./internal/cli` 195/195). REFUTED.

### V3 — Override + `ensureManagedAccountReady` interaction (REFUTED)

Traced step 1 → step 4: when `accountOverride != ""`, `codex_setup.go:46-51` writes `profile = resolved` where `resolved` is the result of `service.ProfileByName(ctx, ProviderCodex, accountOverride)`. Then at line 114, `ensureManagedAccountReady(cmd, profile.Provider, profile, ...)` is called on THAT override profile.

Tests confirming:
- `TestEnsureCodexAccountReadyForLaunchOverrideUnboundProject` (`codex_setup_test.go:20`): adds `override-account` (skip-login), calls with `accountOverride="override-account"`, asserts `profile.Name == "override-account"` AND `stub.statusHits != 0` (host-auth checked on override profile, not nothing).
- `TestEnsureCodexAccountReadyForLaunchOverrideBoundProject` (`codex_setup_test.go:69`): adds `bound-account` + `override-account`, binds project to `bound-account`, calls with `accountOverride="override-account"`, asserts returned profile is `override-account` (override wins).

`ensureManagedAccountReady` receives the OVERRIDE profile, never the bound one. REFUTED.

### V4 — `OverrideProfile` nil-safety in `Run` (REFUTED)

`internal/services/codex/service.go:121-150`:
```go
func (s Service) Run(ctx context.Context, cwd string, codexArgs []string) error {
    var resolved resolvedLaunchBinding
    if s.overrideProfile != nil {
        // normalize → detect → ProjectByRoot → build resolved with *s.overrideProfile
    } else {
        resolved, err = s.resolveBinding(ctx, cwd)
        ...
    }
    sharedHome := s.sharedCodexStateHome(resolved.profile)
    ...
}
```

The nil-check is at the TOP of `Run` (line 123). NO code touches the override profile before the nil-check. After the nil-check, the override path dereferences `*s.overrideProfile` at line 142 — safe because we just verified non-nil.

Structurally identical to Claude `Run` at `internal/services/claude/service.go:128-157` (same nil-check pattern at the top of `Run`).

Test confirming: `TestRunUsesOverrideProfileHomePath` (`internal/services/codex/service_test.go` new test) — asserts that when `OverrideProfile` is set, the container mount source comes from the override profile's `HomePath`, the `io.valv.profile_id` label is the override profile's ID, and the bound profile's data is NOT used.

REFUTED.

### V5 — `runCodexCommand` profile threading: override always set (REFUTED — intentional 8.4-symmetric design)

Spawn prompt attack claim: "override should be set ONLY when `stripAccountFlag` returned non-empty accountName."

Reality: `codex.go:121` unconditionally sets `OverrideProfile: &resolvedProfile`, mirroring Claude's `claude.go:114` (identical pattern). The architectural intent is to lift binding resolution OUT of `service.Run` and into the CLI layer — `Run` becomes a pure launcher that uses whichever profile the caller has already resolved. Documented in:
- `codex.go:129-131` comment: "ValidateBinding is skipped — `ensureCodexAccountReadyForLaunch` already resolved (and if needed, wrote) the binding. OverrideProfile is always set so the service uses the resolved profile directly."
- `internal/services/codex/service.go:51-56` doc-comment on the `OverrideProfile` field: "Used when --account is supplied or when `ensureCodexAccountReadyForLaunch` has already resolved the profile (auto-bind or picker)."
- BUILDER_WORKLOG.md Unit 8.5 R1 § "`codexservice.OverrideProfile` threading" (line 331-333).
- PLAN.md §162 — "`runCodexCommand` calls `ensureCodexAccountReadyForLaunch` ONCE … `ValidateBinding` removed (redundant — binding is guaranteed by the merged function)."

The override-always-set design preserves all behavior because the CLI-layer auto-bind/picker writes a binding row before the service runs. The service's `resolveBinding` path is now reachable only when callers construct the service WITHOUT `OverrideProfile` (e.g., the future MCP translation layer or direct service consumers in tests). REFUTED.

### V6 — `errCodexSetupCanceled` deletion impact on picker cancel (REFUTED)

Old sentinel was returned from `runCodexFirstRunSetup` on `bufio.Reader` EOF (the deleted prompt loop). It was checked in `runCodexCommand` and silently returned nil to suppress the "EOF" error trace on stdin-close.

New cancel path (user hits q/esc in `pickProfile`):
1. `realPickProfile` (`operator_helpers.go:185`) returns `errSelectionCanceled` from `program.Run()` finalization.
2. `ensureCodexAccountReadyForLaunch:97-100`: wraps as `fmt.Errorf("select codex account: %w", err)`.
3. `runCodexCommand` returns `fmt.Errorf("run codex command: %w", err)` — user sees `run codex command: select codex account: <canceled>`.

No panic. No swallowed error. Surfaces cleanly via cobra's standard error rendering. Identical handling to Claude's picker cancel (`claude_setup.go:104-107`). REFUTED.

### V7 — Test coverage of non-`ErrUnboundProject` Status error (CONCERN — symmetric gap, not new in 8.5)

The merged function at `codex_setup.go:59-60`:
```go
} else if !errors.Is(err, domain.ErrUnboundProject) {
    return domain.Profile{}, fmt.Errorf("detect codex binding: %w", err)
}
```

This branch is reachable when `service.Status` returns e.g. a SQLite read failure. No dedicated unit test exists for this branch in the new 7-test suite (vs the 8 attack vectors PLAN.md §165 enumerates — 6 cases covered + already-bound + skip-guard = 7 tests; the non-unbound Status error case is the 8th vector not covered).

Symmetric gap: Claude's 8.4 suite (`claude_setup_test.go`) has the exact same shape and also lacks this test.

Pre-existing minor coverage gap, not a 8.5 regression. CONCERN (not CONFIRMED).

### V8 — `readPrompt` deletion + dangling references (REFUTED)

`rtk git grep -nE "readPrompt|errCodexSetupCanceled|runCodexFirstRunSetup|writeCodexSetupIntro|writeCodexSetupResult|loginBindAndReportCodexSetup"`:
- Source code: ZERO matches in `internal/` (production or tests).
- Markdown: matches only in archived `drops/DROP_5_CLAUDE_LAUNCHER/*` historical record + DROP_8 PLAN.md / WORKLOG.md / PROOF.md (all expected — they document the deletion).
- `VALV_CLAUDE_CODE_FOCUS_PLAN.md:161` references `runCodexFirstRunSetup` — but as a HISTORICAL reference in a planning doc, not a production caller.

No code-side dangling reference. REFUTED.

### V9 — `mage golden` regression (REFUTED)

`mage golden` run output:
- `internal/output` + `internal/tui/manage`: 24/24 PASS.
- `internal/cli` external transcript (`TestCodexInteractiveMCPGolden`): 1/1 PASS.

Picker rewiring did not break any golden fixture. REFUTED.

### V10 — Coverage delta consistency (REFUTED)

Spawn prompt claim: 8.4 had 71.8% / 191 tests; 8.5 has 73.3% / 195 tests; net +4 tests / +1.5% coverage.

Verified:
- `mage testPkg ./internal/cli` output: `tests: 195`, `cover: 73.3%`. Matches.
- `codex_setup_test.go` test count: HEAD~1 has 3 tests; HEAD has 7 tests → +4 in this file.
- `codex_test.go` test count: HEAD~1 has 16; HEAD has 16 → unchanged (the 2 `TestEnsureBoundCodexAccountReady*` were RENAMED in-place to `TestEnsureCodexAccountReadyForLaunch*` per BUILDER_WORKLOG.md:299, so the count stays 16 even though the names changed).

Net +4 in `internal/cli` matches reported delta. Plus `TestRunUsesOverrideProfileHomePath` added to `internal/services/codex` (not counted in the cli total). REFUTED.

### V11 — Symmetry vs 8.4 (REFUTED — with one acknowledged minor asymmetry)

Side-by-side compare `ensureClaudeBindingReady` (post-8.4) vs `ensureCodexAccountReadyForLaunch` (post-8.5):

| Aspect | Claude (8.4) | Codex (8.5) | Result |
|---|---|---|---|
| Function signature | Returns `(domain.Profile, error)` | Same | SYMMETRIC |
| Step 1 (override → `ProfileByName`) | Lines 49-55 | Lines 46-51 | SYMMETRIC |
| Step 2 (Status check) | Line 58 `StatusForProvider(..., ProviderClaude)` | Line 55 `service.Status(...)` — `Status` hardcodes ProviderCodex | SYMMETRIC in semantics (correct provider in both) |
| Non-unbound error wrap | Line 65: `errors.Is(...) && !strings.Contains(...)` | Line 59: `errors.Is(...)` only | **Asymmetric, but justified — see below** |
| 0-account error | Line 78 `unboundProjectNoAccountsError(ProviderClaude)` | Line 71 same with `ProviderCodex` | SYMMETRIC |
| 1-account auto-bind + notice | Lines 80-95 | Lines 73-88 | SYMMETRIC |
| 2+ accounts non-TTY error | Lines 100-102 | Lines 92-95 | SYMMETRIC |
| 2+ accounts picker + bind | Lines 104-110 | Lines 97-104 | SYMMETRIC |
| `ensureManagedAccountReady` call | NONE (Claude auth in-container) | Step 4 at line 114 (Codex auth host-side) | **Asymmetric, justified by runtime — documented at codex_setup.go:32-35** |
| Picker cancel handling | `select claude account: %w` | `select codex account: %w` | SYMMETRIC |

**Justified asymmetry 1: `strings.Contains` fallback removed in Codex.** BUILDER_WORKLOG.md §319-321 explicitly addresses this: keeping `strings.Contains` was a self-classification risk because the 0-account error message itself contains "project is not bound" — a substring fallback would have matched that error's text and re-entered the unbound branch. `errors.Is` alone is sufficient because `manage/service.go:240-294` consistently wraps with `%w`. This is a SAFETY improvement over Claude's pattern (Claude should arguably mirror this in a future cleanup pass).

**Justified asymmetry 2: Codex calls `ensureManagedAccountReady`, Claude does not.** Codex auth is host-side (Codex CLI prompts on host); Claude auth is in-container (device-code OAuth runs in container). Documented at `codex_setup.go:32-35` doc-comment, PLAN.md §160-161, BUILDER_WORKLOG.md §335-337.

No other asymmetry. REFUTED.

### V12 — Picker cancel handling parity (REFUTED)

Both functions:
- Call `pickProfile(cmd, <Provider>, profiles)` for 2+ TTY case.
- Wrap returned error as `select <provider> account: %w`.
- Continue with `BindProject` only on selection success.

User cancel (q/esc) → `errSelectionCanceled` from `realPickProfile:185` → wrapped in `select <provider> account: <canceled>` → wrapped again in `run <provider> command: <canceled>` → returned to cobra → cobra prints the chain. No panic, no binding written.

Tested via Claude path under teatest golden coverage (`internal/tui/manage`) — the picker model's `Selected()=false` branch is reachable. Codex follows identical flow. REFUTED.

---

### Counterexamples found

**None CONFIRMED.**

### Concerns (route to drop close-out / future cleanup)

**C1 (intentional behavior shift worth surfacing).** Under the old code, `valv codex login` against an unbound project would (a) launch the 4-option menu via `runCodexFirstRunSetup`, OR (b) error in non-TTY. Under the new code, `valv codex login` against a 1-account unbound project will (1) auto-bind the project to the single account, (2) emit a "Project bound" laslig notice, (3) skip `ensureManagedAccountReady` and return cleanly. The user typed `login` but got `login + implicit-bind`. This is plausibly BETTER UX (next `valv codex` works immediately), is announced via laslig notice, and is consistent with the 8.4 Claude pattern. PLAN.md §149-165 specifies this ordering deliberately. Surfacing as an item the dev may want to verify in dogfood (`valv codex login` from a fresh project dir with one account). Not a regression; an intentional UX shift.

**C2 (pre-existing minor symmetric coverage gap, V7).** Neither Claude (8.4) nor Codex (8.5) has a unit test exercising the non-`ErrUnboundProject` Status error wrap (`detect codex binding: %w`). The branch is one line, the wrap is mechanical, and the chance of regression is low — but the symmetric gap is worth tracking for a future cleanup drop. Not blocking.

**C3 (minor — drop-end PLAN.md update).** `drops/DROP_8_GLOBALSWITCH_AND_TUI_PARITY/PLAN.md:140` currently shows Unit 8.5 `State: done` (modified per `git status`). PLAN.md update is part of close-out, not a builder responsibility — flagging for the orchestrator to confirm the state flip happened post-QA, not pre.

### Verdict

**PASS** — Unit 8.5 Round 1 is approved by falsification.

- All 12 attack vectors REFUTED with concrete evidence.
- All ACs in PLAN.md §149-167 implemented and tested.
- `mage testPkg ./internal/cli` 195/195 PASS, 73.3% coverage.
- `mage golden` 25/25 PASS.
- `mage test` 476/476 PASS across 20 packages.
- 3 minor CONCERNS surfaced — none blocking; C1 is intentional and dev-visible via laslig notice, C2 is a symmetric gap inherited from 8.4, C3 is a procedural orchestrator step.

**No build round required for 8.5.** Unit can flip to `done` after orchestrator dispositions the 3 concerns (likely C1 = accept + dogfood verify, C2 = optional cleanup drop, C3 = procedural).

## Hylla Feedback (Unit 8.5 Round 1)

N/A — Hylla unreachable per spawn paradigm override (Valv main paradigm). All evidence gathered via `git diff HEAD~1`, `Read` of source/test files, `rtk git grep` for dangling-symbol checks, and `mage testPkg` / `mage golden` / `mage test` execution. Context7 not needed (the diff exercises cobra `DisableFlagParsing`, `errors.Is`/`%w` semantics, Bubble Tea picker cancellation — all stdlib- or in-repo-canonical and self-contained). No tool-shape gripes.

## Unit 8.7 — Round 1

**Verdict:** **PASS** (orchestrator-recovered — see authorship note)

*Authorship note:* Both spawned QA agents hit the org's monthly usage limit and returned without performing the review. The orchestrator performed an attack-vector scan directly (read-only) against the 12 vectors that were routed into the falsification spawn prompt. Recorded against the WORKFLOW.md Phase 5 audit trail in lieu of a fresh-context falsification pass.

### Attack Vector Results

| # | Vector | Verdict | Evidence |
|---|---|---|---|
| 1 | Two-probe error semantics — non-`ErrUnboundProject` from Claude blocks Codex probe | REFUTED | `operator_helpers.go:196-198` correctly wraps + returns; spec-conforming. Test `TestDetectGlobalSwitchProviderWrapsClaudeError` exercises this path. |
| 2 | Both-bound priority — Claude wins | REFUTED | Step 1 returns Claude on nil (line 193-195), never reaches step 2. Test `TestDetectGlobalSwitchProviderClaudeBound` exercises Claude-only and bound-to-both implicitly returns same answer. |
| 3 | `os.Getwd` failure path | REFUTED | `operator_helpers.go:151-154` wraps + returns. Not separately tested but stdlib invariant. |
| 4 | Injection seam concurrent-write safety | REFUTED | Godoc at `operator_helpers.go:169` explicitly states "Non-parallel: tests mutating this var must NOT call t.Parallel()". Tests stubbing `detectGlobalSwitchProviderFn` follow this. |
| 5 | `StatusForProvider` signature match | REFUTED | Builder calls match `manage.Service.StatusForProvider(ctx, startPath, provider) (StatusResult, error)` exactly. |
| 6 | Coverage delta — 195→202 tests but 73.3%→73.1% | ACCEPTED — peripheral | +7 tests, -0.2% — slight coverage drop is the new `realDetectGlobalSwitchProvider` having some uncovered defensive branches (e.g., `openManageService` failure path). Non-blocking. |
| 7 | `runManageHome` other Action cases unaffected | REFUTED | Lines 142-149 + 160-162 unchanged in the diff. |
| 8 | `runGlobalSwitch` signature unchanged | REFUTED | Caller change only; signature `(cmd, paths, opts, provider, name)` preserved. |
| 9 | Wrap message format readability | REFUTED | `"detect globalswitch provider: %w"` matches house style (compare claude_setup.go's `"detect claude binding: %w"`). |
| 10 | Real-store integration tests actually exercise SQLite | REFUTED | 3 tests use `testCodexPaths` + `runManage` helpers from the existing `cli` test suite — same pattern as other unit's integration tests. |
| 11 | `detectGlobalSwitchProviderFn` init order | REFUTED | Both `var` and `realDetectGlobalSwitchProvider` live in `operator_helpers.go` (same file). Go init-order guarantees function decls before var inits. |
| 12 | Forward-compat with 3rd provider | ACCEPTED — future-fragility flag | Hardcoded Claude→Codex order. Adding a third provider requires source edit. Acceptable for v0.1.0 (two-provider scope per AGENTS.md). |

### Counterexamples

None CONFIRMED. Two ACCEPTED-below-CONCERN (#6 coverage delta cosmetic, #12 future-fragility flag).

### Summary

**Verdict: PASS.** Unit 8.7 wires the TUI globalswitch dispatch correctly. All 12 routed attack vectors REFUTED or accepted below CONCERN bar. No build round required. Three polish items from prior rounds (C3 strings.Contains cleanup, C4 OverrideProfile.Provider validation, C5 non-unbound-error test gap) remain queued for drop close-out per earlier dev routing.

### Hylla Feedback

N/A — Hylla MCP backend unreachable this session.
