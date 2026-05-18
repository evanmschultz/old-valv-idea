# DROP_8 — Plan QA Proof — Round 1

**Verdict:** **pass with concerns**

The plan is broadly sound: every cited line/symbol resolves, the six-unit decomposition is atomic at the right granularity, and `blocked_by` serialization (8.2 → 8.3 → 8.4 → 8.5) reflects real consumer-side dependency. Three independent units (8.1, 8.2, 8.6) can run in true parallel — verified disjoint paths. Source citations in the plan match the current `main` checkout.

Three medium-severity concerns and four low-severity polish items are listed below. None block Phase 3 entry — they sharpen acceptance criteria so a builder cannot honor the letter while violating intent.

---

## 1. Per-unit findings

### Unit 8.1 — Globalswitch Claude extension

Pass. All claims verified:

- `service.go:86` (planner's citation) confirmed: `func (s Service) Switch(...)` body begins `if provider != domain.ProviderCodex { return Result{}, fmt.Errorf("switch global profile %q: unsupported provider", provider) }`. Removing this guard is the surgical change.
- `service.go:90` confirmed: `s.isRunning(ctx, "codex")` is the host-process guard the AC requires us to extend to `"claude"`.
- `service.go:137` confirmed: backup root is `filepath.Join(s.stateDir, "global-switch", "codex", "backups")`. The AC's requirement to backup Claude under `global-switch/claude/backups/<timestamp>` is satisfied by parameterizing this string per provider — implementation surface is one function (`prepareTarget`).
- `service_test.go:92-102` (`TestSwitchRejectsUnsupportedProvider`) confirmed: currently rejects `"claude"`. AC requires updating this test to expect Claude pass + truly-unknown provider failure.
- `service.go:106` confirmed: `TargetPath: filepath.Join(s.homeDir, ".codex")` — Claude path will be `filepath.Join(s.homeDir, ".claude")`.

**1.1 [Axis: acceptance-criteria-coverage] [severity: low]** AC does not explicitly require parameterizing the `writeState` path key (`service.go:149`: `filepath.Join(s.stateDir, "global-switch", string(result.Provider), "current.json")`). The current code already uses `result.Provider`, so it's correct by inspection — but the AC could explicitly say "Result.Provider determines the state-file path under `state/global-switch/<provider>/current.json`" so the builder doesn't accidentally hardcode "codex". Polish, not blocker.

**1.2 [Axis: acceptance-criteria-coverage] [severity: low]** AC for the new Claude test (analogous to `TestSwitchCodexSymlinksSelectedProfileAndBacksUpExistingDir`) is implied ("`-race` passes") but not explicit. Recommend adding: "A `TestSwitchClaudeSymlinksSelectedProfileAndBacksUpExistingDir` mirrors the Codex coverage at `service_test.go:46-90` — pre-existing real `~/.claude` directory → backup; symlink → no backup; running `claude` process → reject."

### Unit 8.2 — `valv account switch` cross-provider with `--provider` flag

Pass with one substantive concern.

- `manage.go:333-357` confirmed: `newManageAccountSwitchCommand` is the right entry point. No `--provider` flag currently exists — AC additions are clean.
- `manage.go:822-857` confirmed: `resolveProfileSwitchTarget` is the resolver function the AC targets. Cases 0/1/2 args confirmed; defaultProvider hardcoded to `domain.ProviderCodex` at line 826.
- `manage.go:931-968` confirmed: `writeAccountsByProvider` iterates `supportedProviders()` (line 1001-1003 returns `[ProviderCodex, ProviderClaude]`). The AC's claim "`valv account list` already shows cross-provider" is accurate. Test-only addition (no functional change) is the right call.
- `root.go:129` confirmed: `newManageAccountCommand` is wired at BOTH `valv account` (root.go:129-130, 136) AND `valv manage account` (manage.go:49) via the same constructor — so editing `newManageAccountSwitchCommand` once propagates to both surfaces. The plan implicitly relies on this; worth noting in case DROP_9 changes it.

**2.1 [Axis: acceptance-criteria-coverage] [severity: medium]** The AC says "When the name exists in multiple providers, return a user-facing error that includes both provider names and instructs the user to use `--provider`." But `resolveProfileSwitchTarget`'s 1-arg branch currently tries `ParseProvider(args[0])` first (line 843) — if the arg looks like a provider name (e.g. a user creates a Codex account literally named "claude"), the function returns it as a provider rather than treating it as an account name. The cross-provider name-collision detection logic the AC requires is NEW behavior: the builder must add a "check both providers for an account named <arg>" pre-check that runs only when `ParseProvider` fails. Recommend tightening the AC to specify the order of checks: (1) if `--provider` set, use it; (2) else `ParseProvider(arg[0])` — if it parses as a provider, use it as today; (3) else iterate `supportedProviders()` and look up `ProfileByName(ctx, p, arg[0])` — if exactly one provider has it, return that pair; if 0, fall back to current-provider resolution; if 2+, error with both provider names. Without this, the builder may implement an ambiguous resolver.

**2.2 [Axis: acceptance-criteria-coverage] [severity: low]** AC for "When 0 args and no `--provider`, the picker opens showing all providers' accounts (or the provider defaults to the current binding's provider when the project is bound)" — these are two different behaviors joined by "or" with no disambiguation rule. Recommend stating the precedence explicitly: "If project is bound, picker scopes to bound provider; if unbound, picker shows all providers' accounts." Otherwise the builder picks one arbitrarily.

### Unit 8.3 — `--account` raw-arg interception

Pass with two substantive concerns.

- `claude.go:46-47` confirmed: `DisableFlagParsing: true`. Same on `codex.go:52-53`. The AC's note about preserving this flag is correct and load-bearing — cobra would otherwise eat `--account` before it reaches Claude/Codex.
- `claude.go:53-65` and `codex.go:59-77` confirmed: both `runClaudeCommand` and `runCodexCommand` receive `args []string` directly. `stripAccountFlag` insertion point (before the `*ArgsSkipProjectBinding` check) is the right boundary.

**3.1 [Axis: shipped-but-not-wired] [severity: medium]** The AC says "`runClaudeCommand` calls `stripAccountFlag` before the `claudeArgsSkipProjectBinding` check. The extracted `accountName` is threaded through to `runClaudeCommand`'s binding-ready call (introduced in Unit 8.4)." But Unit 8.4's `ensureClaudeBindingReady` receives `accountOverride` as a parameter, and `runClaudeCommand` then ALSO needs to use the resolved override profile to construct the `claudeservice.Options`. The current `runClaudeCommand` (claude.go:93-104) builds the service from store + paths and then calls `service.Run(ctx, workingDir, args)`. Where does the override profile actually flow into the Docker launch? The plan is silent on this. Risk: builder implements `stripAccountFlag` and `ensureClaudeBindingReady`, but the override profile is dropped on the floor before `service.Run`, because `claudeservice` resolves its account internally via `service.ValidateBinding` → store lookup using the bound profile. Recommend: AC for Unit 8.3 or Unit 8.4 explicitly names the call-graph surgery — either `claudeservice.New` gains an `OverrideProfile` option, or `runClaudeCommand` constructs the launch args using the override profile after bypassing `ValidateBinding`. Same concern for Codex in Unit 8.5 (current `codexservice.Run` similarly resolves via `ValidateBinding`).

**3.2 [Axis: acceptance-criteria-coverage] [severity: low]** AC enumerates `account_flag_test.go` cases including "`--account` as the last token (malformed — returns empty)". But Claude itself may legitimately accept a `--account` flag in some future version (Anthropic's CLI has `--print --output-format`, `--allowedTools`, etc.) — stripping silently rather than erroring is the right call here, but the AC doesn't say what happens when `stripAccountFlag` strips a `--account` that the user actually intended for Claude. There's no way to distinguish from the wrapper's side. This is a real intent ambiguity, not a bug — but the AC should say "the wrapper unconditionally consumes the first `--account` token; passing `--` before `--account` to forward it through is out of scope for this drop." Otherwise the builder may overthink and add escape sequences.

**3.3 [Axis: parallelization-graph] [severity: low]** Unit 8.3 has `Blocked by: 8.2`. Verified: 8.3 doesn't actually USE the new `valv account switch --provider` flag from 8.2 — `stripAccountFlag` is a self-contained helper. The dependency must be that the cross-provider account RESOLUTION logic added in 8.2 (`ProfileByName` lookup across providers) is the lookup `ensureClaudeBindingReady` / `ensureCodexBindingReady` will call for the override path. If so, the dependency is real but indirect. Recommend tightening: rename the dependency to "Unit 8.3 reuses the cross-provider account-lookup helper introduced in Unit 8.2; if 8.2's helper is a private function on `manageservice.Service`, 8.3 needs it exported or extracted." Otherwise a reader can't tell why 8.3 blocks on 8.2.

### Unit 8.4 — Claude binding UX

Pass with one concern; all source-citation claims verified.

- `manage/service.go:245` confirmed: `s.store.BindingByProjectID(ctx, projectRecord.ID, domain.ProviderCodex)` — hardcoded to Codex. The plan's note "must NOT use `service.Status()` for Claude" is accurate. The fall-through suggestion ("attempt `service.Status` path and fall through on `ErrUnboundProject` to the 0/1/2+ branching") would WRONGLY treat a Claude-bound project as unbound because `Status` only checks Codex bindings. The builder must NOT use that fall-through.
- `domain/repository.go:22` confirmed: `BindingByProjectID(ctx, projectID, Provider)` accepts a provider arg — so a direct call with `domain.ProviderClaude` works without changes to the repository contract.
- `manage/service.go:194-229` (`BindProject`) confirmed: provider-parameterized, no Codex hardcoding. AC's `service.BindProject(ctx, domain.ProviderClaude, profile.Name, workingDir)` call is valid as written.
- `manage/service.go:264-275` (`ListProfiles`) confirmed provider-parameterized.
- `claude.go:67-69` confirmed: existing comment "No ensureClaudeBindingReady" notes the deliberate gap that this unit fills. Aligned.

**4.1 [Axis: acceptance-criteria-coverage] [severity: medium]** The AC says "attempt `service.Status` path and fall through on `ErrUnboundProject` to the 0/1/2+ branching." This is wrong as written. `manage/service.go:245` hardcodes `domain.ProviderCodex` in the binding lookup, so calling `service.Status` for a Claude-bound project will return `ErrUnboundProject` even though the project IS bound to a Claude account. The builder must check the Claude binding directly via the store (`store.BindingByProjectID(ctx, projectID, domain.ProviderClaude)`) rather than going through `service.Status`. Recommend rewriting the bullet to: "Check Claude binding via `store.BindingByProjectID(ctx, projectID, domain.ProviderClaude)` — do NOT use `manage.Service.Status()` because it's hardcoded to `ProviderCodex`. On `ErrNotFound`, fall through to the 0/1/2+ branching on `ListProfiles(ctx, domain.ProviderClaude)`." This is the same point the planner already half-made but then weakened with the "or simply attempt Status" alternative — please remove the alternative.

**4.2 [Axis: acceptance-criteria-coverage] [severity: low]** AC says "if TTY available, launch picker via `pickProfile(cmd, domain.ProviderClaude, profiles)` then bind". `pickProfile` returns `(string, error)` (operator_helpers.go:157) — a profile NAME, not a `domain.Profile`. The builder must then call `service.ProfileByName(ctx, domain.ProviderClaude, name)` to get the full profile, then `service.BindProject`. Worth specifying in the AC so the builder doesn't try `BindProject(ctx, provider, name, ...)` and then look surprised at unused profile-list data. Minor — `BindProject` already takes a name string, so the existing API fits. Just make the test-coverage AC bullet say "auto-bind path and picker-bind path both result in `BindProject` being called with the selected name".

### Unit 8.5 — Codex binding UX parity

Pass.

- `codex_setup.go:22-46` (`ensureCodexBindingReady`) confirmed as the right entry point.
- `codex_setup.go:48-128` (`runCodexFirstRunSetup`) confirmed: existing 4-option menu (default / existing / new isolated / cancel). The AC's claim that the 2+ multi-account flow stays unchanged is true if the builder only short-circuits the 0 and 1 cases before falling through to `runCodexFirstRunSetup`.
- `codex_setup_test.go:17-149` confirmed: existing tests cover options 1 and 3 of the menu; no existing 0-account-error or 1-account-shortcut tests. The AC's "tests for the 0-account and 1-account shortcuts are added" is the correct delta.

**5.1 [Axis: acceptance-criteria-coverage] [severity: low]** AC says "0 Codex accounts and no TTY → return a user-facing error". But Unit 8.4's Claude AC also implies 0 + TTY-present → still error (you can't pick from zero). The current Codex AC is silent on 0 + TTY. The implicit answer is "also error" because `runCodexFirstRunSetup`'s option 1 ("default host-backed account") creates an account on the fly, which is different behavior than the new spec wants. Recommend explicit: "0 Codex accounts → always return the error pointing to `valv manage account add codex`, regardless of TTY. Do NOT fall through to `runCodexFirstRunSetup` for the 0 case." Otherwise the builder may keep the existing fall-through and let the menu's option 1 create the account, which violates parity with Claude.

**5.2 [Axis: spec-conformance] [severity: low]** AC says "1 Codex account → auto-bind the single account silently (call `service.CreateDefaultHostProfile` or `service.ProfileByName` + `service.BindProject`)". The "or" is wrong: `CreateDefaultHostProfile` ALWAYS creates the default host-backed profile (manage/service.go:171-184). If a single isolated profile already exists with no default host one, calling `CreateDefaultHostProfile` will create a SECOND profile rather than binding the existing one. Recommend striking `CreateDefaultHostProfile` from the AC and using `ProfileByName` against the single profile returned by `ListProfiles`. The AC for Claude (Unit 8.4) gets this right (`service.BindProject(ctx, provider, profile.Name, workingDir)`); the Codex AC should match.

### Unit 8.6 — TUI golden parity

Pass.

- `golden_test.go:25-39` confirmed: existing `TestProfilePickerGolden` constructs a Codex `ProfilePickerModel` with two profiles. AC's "mirror this with `domain.ProviderClaude`" is the surgical change.
- `testdata/` confirmed: `TestManageHomeGolden.golden` and `TestProfilePickerGolden.golden` exist. The new `TestProfilePickerGoldenClaude.golden` will be the third file there.
- AC's mage targets (`mage golden`, `mage testPkg github.com/evanmschultz/valv/internal/tui/manage`) confirmed against `magefile.go` discovery (table in main/CLAUDE.md § "Build Verification").

**6.1 [Axis: parallelization-graph] [severity: low]** Unit 8.6 (`internal/tui/manage/`) and Unit 8.4 (`internal/cli/`) touch disjoint packages, so `blocked_by: —` for 8.6 is correct. However, the picker emitted by 8.4's binding flow USES `pickProfile` → `managetui.NewProfilePicker` (operator_helpers.go:166) → the same `ProfilePickerModel` whose golden is being tested in 8.6. If 8.4 changes the picker visual at all (e.g. adds a header "Select Claude account for binding"), the 8.6 golden becomes stale. Recommend: 8.4's AC explicitly says "DO NOT modify `internal/tui/manage/picker.go` — reuse as-is." That preserves 8.6's parallelizability. Currently 8.4's AC says "Bubble Tea picker. Existing `internal/tui/manage/picker.go` is the prior art" in the Notes section, which is suggestive but not prescriptive.

## 2. Cross-cutting findings

**7.1 [Axis: shipped-but-not-wired] [severity: medium]** Unit 8.1's `Switch` API extension is necessary for the v0.1.0 globalswitch goal, but I see no unit that adds a CLI surface for `valv global switch claude <account>`. The current `runGlobalSwitch` flow is called from `runManageHome` (operator_helpers.go:151) hardcoded to `domain.ProviderCodex`. After Unit 8.1 lands, callers must be updated to dispatch the right provider. Is that out of scope for this drop? If so, Unit 8.1's globalswitch extension is "shipped but not wired" — useful tests, no user-visible behavior change. Recommend either: (a) add a Unit 8.7 wiring the CLI/TUI call sites to choose the provider, or (b) explicitly note in DROP_8 scope that CLI wiring lands in DROP_9 (CLI audit drop). Currently neither the drop scope nor any unit owns that wiring.

**7.2 [Axis: spec-conformance] [severity: low]** DROP_9 interlock: DROP_9's scope says "Ensure DROP_8's `--account <name>` override (added there, not here) wires correctly through the renamed command tree — do NOT re-implement." This works IFF the `--account` parsing lives in a shared helper (per Unit 8.3, `internal/cli/account_flag.go`), not embedded in `claude.go` / `codex.go` `RunE` bodies. Verified by the plan: `stripAccountFlag` is in its own file, both runners call it. After DROP_9 renames the command tree (e.g. delete `valv manage` namespace), the renamed runners just need to keep calling `stripAccountFlag` at the same boundary. Clean. Plan passes the DROP_9 interlock check.

**7.3 [Axis: parallelization-graph] [severity: low]** True parallelism check: Units 8.1, 8.2, 8.6 listed as independent (`blocked_by: —`). Verified disjoint:
- 8.1 → `internal/services/globalswitch/` only
- 8.2 → `internal/cli/manage.go` + tests only
- 8.6 → `internal/tui/manage/golden_test.go` + new testdata file only

No path overlap, no symbol overlap. Truly parallel. Sequential chain 8.2 → 8.3 → 8.4 → 8.5 verified: 8.3 introduces helper used by 8.4 + 8.5; 8.4 introduces Claude flow that 8.5 mirrors. Two of these dependencies are STRUCTURAL (8.4 and 8.5 are symmetric implementations; building one informs the other), one is genuine (8.3 helper consumed by 8.4 + 8.5). Could 8.4 and 8.5 be parallelized after 8.3 lands? They edit different files (`claude_setup.go` new vs `codex_setup.go` existing); the function signatures are pairwise. Recommend the planner consider parallelizing 8.4 and 8.5 by relaxing 8.5's `Blocked by: 8.4` to `Blocked by: 8.3`. Not a blocker — sequential is safe; parallel is faster.

**7.4 [Axis: multi-level-decomposition] [severity: low]** Plan is one level deep: 6 units at the top, no sub-decomposition. Each unit fits in one builder round (one file or two-file pair, scoped AC). Granularity check passes — no unit appears too large.

## 3. Constraint adherence

Verified:

- No command renames (DROP_9 territory). Confirmed: all new commands stay in current tree.
- No re-introduction of auto-open. Confirmed: no AC mentions opening URLs.
- No Tillsyn references. Confirmed.
- No new providers. Confirmed: only `domain.ProviderCodex` and `domain.ProviderClaude`.
- `mage testPkg ...` targets cited correctly per `magefile.go` table.

## 4. Hylla Feedback

N/A — Hylla MCP backend unreachable in this session per the spawn-prompt note. All verification done via direct `Read`. The miss is a known infrastructure outage, not a coverage gap to file.

## 5. Summary

Verdict: **pass with concerns**. The plan structure is sound and ready for build with the following findings addressed by the planner in Phase 3:

- **Medium**: 2.1, 3.1, 4.1, 7.1 (sharpen ACs around cross-provider name resolution, override profile flow into Docker launch, the `service.Status` fall-through trap, globalswitch CLI wiring).
- **Low**: 1.1, 1.2, 2.2, 3.2, 3.3, 4.2, 5.1, 5.2, 6.1, 7.2, 7.3, 7.4 (polish — disambiguation, test coverage spelling, parallelism relaxation).

No FAIL-class findings. Phase 3 (discuss + cleanup) can proceed; planner re-spawn should sharpen the four medium-severity ACs and decide whether 8.1's globalswitch extension needs a CLI-wiring sibling unit.
