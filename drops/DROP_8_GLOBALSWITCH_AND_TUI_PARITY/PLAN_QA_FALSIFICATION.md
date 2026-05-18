# DROP_8 Plan QA Falsification — Round 1

**Verdict:** fail (1 BLOCK, 7 CONCERN, 4 REFUTED, several documentation gaps that should be tightened before Phase 4)

The plan is broadly sound and the unit decomposition is reasonable, but Unit 8.1's acceptance criteria leak a hardcoded-codex path that the unit's own scope does not surface as a fix, and Unit 8.4's binding-resolution prose contradicts itself in a way that will make the builder guess. The remaining findings are concerns the planner can address with small, surgical wording changes in PLAN.md.

---

## Attack 1 — `--account` raw-arg interception ambiguity

**Status:** CONCERN

**Evidence**
- `internal/cli/claude.go:46` and `internal/cli/codex.go:52` both set `DisableFlagParsing: true` with `Args: cobra.ArbitraryArgs`.
- `go doc github.com/spf13/cobra Command.DisableFlagParsing` confirms the documented contract: *"DisableFlagParsing disables the flag parsing. If this is true all flags will be passed to the command as arguments."* That means cobra does NOT consume `--`; it lands in `args` as a literal token.
- Context7 query against `/anthropics/claude-code` returned no evidence that the upstream `claude` binary accepts an `--account` flag. Same for `codex` (the binary's surface is `login`, `logout`, `resume`, `exec`, etc.). So there is no first-party collision today.
- Unit 8.3 acceptance says "first match wins" for `--account`.

**Counterexample attempts**
- *Collision with claude/codex CLI flags*: REFUTED. Neither vendor CLI defines `--account`. The attack lands only if Anthropic or OpenAI add one later, at which point Valv has a forward-compatibility issue regardless.
- *User wants to pass `--account` literally to the inner CLI*: PARTIALLY CONFIRMED as a forward-fragility concern. With `DisableFlagParsing: true`, cobra treats `--` as a literal arg, so a user *could* write `valv claude -- --account foo` expecting `--account foo` to reach claude, but `stripAccountFlag` as specified scans the WHOLE arg slice and would still strip the first `--account` it finds — including the one after the literal `--`. The plan does not require `stripAccountFlag` to respect a `--` separator and stop scanning past it.
- *Multiple `--account` instances*: planner says first match wins. But what if the user *intends* `--account work` for Valv AND happens to later add a literal `--account` arg the inner CLI accepts? The plan's "first match wins" rule cannot disambiguate.

**Remediation**
- Add to Unit 8.3 acceptance: `stripAccountFlag` MUST stop scanning at a literal `--` token and pass everything after `--` through verbatim. If no `--` is present, the existing first-match-wins rule applies.
- Add a Note to Unit 8.3 documenting that this is the escape hatch: `valv claude -- --account foo` passes `--account foo` to claude unmolested.
- Add at least one `account_flag_test.go` case: `["--account", "work", "--", "--account", "literal"]` → `("work", ["--", "--account", "literal"])`.

---

## Attack 2 — Race between picker selection and `BindProject` write

**Status:** REFUTED (no plan change needed; small AC clarification recommended)

**Evidence**
- `internal/services/manage/service.go:194-228` shows `BindProject` uses `store.UpsertProjectBinding` (line 223). `UpsertProjectBinding` lives at `internal/adapters/sqlite/store.go:407`, indexed by `(project_id, provider)` — idempotent on conflict.
- The race is two concurrent `valv claude` invocations in the same cwd each picking a (possibly different) account.

**Counterexample**
- If both invocations pick the same account, no race (idempotent upsert).
- If they pick different accounts, last-write-wins is fine because each invocation also uses the in-memory picked profile for its OWN launch (not a re-read from the DB after the bind). So neither invocation gets stomped at launch time, even though the persisted binding may reflect either one. No data loss; binding row converges.

**Conclusion**
- Not a counterexample. The semantics are intentional and defensible.
- Minor: planner could add a one-liner to Unit 8.4 acceptance — "`BindProject` is idempotent via UpsertProjectBinding; concurrent invocations in the same project converge on last-write-wins for the binding row, but each invocation launches with its own picked account." Not a blocker.

---

## Attack 3 — `accountOverride` vs `service.ValidateBinding` semantics

**Status:** CONFIRMED (CONCERN — Unit 8.4 self-contradiction)

**Evidence**
- Unit 8.4 final bullet says: "`runClaudeCommand` calls `ensureClaudeBindingReady` immediately after resolving `workingDir`, before `service.ValidateBinding`. When `accountOverride` is non-empty, skip `service.ValidateBinding` (the override profile is used directly)."
- `internal/services/claude/service.go:184` defines `ValidateBinding(ctx, cwd)`. It validates that the cwd's project has a Claude binding in the store — purely a database lookup that returns `domain.ErrUnboundProject` if not bound. It is NOT an auth-token refresh; it is NOT an account-existence check. (See `internal/services/codex/service.go:185` for the parallel method.)
- Unit 8.4 ALSO says: "When `accountOverride != \"\"`: the function resolves the named Claude account from the store (`service.ProfileByName`). It does NOT write a binding row. It returns the resolved profile to the caller so `runClaudeCommand` can use it as the launch account."

**Counterexample**
- The plan never says where `runClaudeCommand` GETS the resolved override profile back. `ensureClaudeBindingReady` returns `error`, not `(domain.Profile, error)`. The "return the resolved profile to the caller" promise has no signature on which to land.
- After this, claudeservice.Run launches a container with `workingDir` — but it does not take an explicit profile arg. Looking at `internal/cli/claude.go:117`, `service.Run(cmd.Context(), workingDir, args)` derives the bind from `workingDir` again internally. If `ValidateBinding` is skipped, claudeservice still has to know which profile to mount. The plan is silent on the wiring.

**Remediation**
- Change Unit 8.4 acceptance to either:
  - (a) `ensureClaudeBindingReady(...) (domain.Profile, error)` returning the resolved profile, AND a follow-on edit to `claudeservice.Service.Run` / `ValidateBinding` to accept an explicit profile override; OR
  - (b) `ensureClaudeBindingReady` writes a *temporary* binding row in a transaction the launcher reads via the existing `ValidateBinding` path (least invasive but mutates the DB the spec said it would not).
- Either way, the planner must specify which approach. Today's wording promises (a) but does not change the function signature or the service surface; the builder will improvise.
- Apply the same fix to Unit 8.5 (Codex parity) — `ensureCodexBindingReady` has the same `error`-only return today.

---

## Attack 4 — 0-account error message string fragility across DROP_9

**Status:** CONCERN

**Evidence**
- Unit 8.4 hardcodes `"run \`valv manage account add claude\` to create one"`.
- Unit 8.5 hardcodes `"run \`valv manage account add codex\` to create one"`.
- DROP_9 (per project memory `project_valv_cli_audit_proposal.md` + the planner's Notes section at PLAN.md:166-167) deletes `valv manage` and adds `valv account add`. After DROP_9, both error strings become misleading.

**Counterexample**
- DROP_9 must touch BOTH `claude_setup.go` AND `codex_setup.go` strings. If either is missed, the user sees outdated guidance.

**Remediation**
- Either:
  - (a) Surface this in PLAN.md's `## Notes` section as an explicit DROP_9 sweep item ("DROP_9 must update the 0-account error string in `claude_setup.go` and `codex_setup.go` to point at `valv account add <provider>`").
  - (b) Hoist the string to a small helper, e.g. `noAccountsForProviderError(domain.Provider)` in `internal/cli/operator_helpers.go`, so DROP_9 only touches one site. The existing `pickProfile` helper at `operator_helpers.go:157-160` already builds a similar string from the provider name; reusing/extending that helper is the natural shape.
- Recommend (b) as the load-bearing surgical fix. Add a one-liner to Unit 8.4 acceptance: "Reuse or extend the existing helper that constructs the 'no <provider> accounts found' error; do not duplicate the string across `claude_setup.go` and `codex_setup.go`."

---

## Attack 5 — Unit 8.1 `prepareTarget` blast radius + hardcoded codex backup path

**Status:** CONFIRMED — BLOCK

**Evidence**
- `internal/services/globalswitch/service.go:126-146` defines `prepareTarget`:
  - Line 137: `backupRoot := filepath.Join(s.stateDir, "global-switch", "codex", "backups")`. The path segment `"codex"` is HARDCODED — not derived from `result.Provider`.
- The only caller is `Service.Switch` at line 111 (verified via `rg -n "prepareTarget"` — single caller). Blast radius outside the package: zero. Good.
- Unit 8.1 acceptance bullet 2 says: "The backup path for a pre-existing real `~/.claude` directory goes under `global-switch/claude/backups/<timestamp>`, not the existing `codex` subdirectory." That is correct in intent.
- BUT the plan does NOT name `prepareTarget`'s signature change. The function currently takes only `target string`; to thread the provider, the builder must change the signature to e.g. `prepareTarget(provider domain.Provider, target string)` OR move the backup-path construction up into `Switch`.
- `writeState` at line 148-149 ALSO uses `string(result.Provider)` (already provider-keyed, correct). But the `result.TargetPath` at line 106 is hardcoded `filepath.Join(s.homeDir, ".codex")` regardless of provider — another codex-specific path the unit acceptance does name (target = `.claude`) but the unit acceptance does NOT call out that the existing `.codex` derivation in `Switch` itself must be provider-keyed.

**Counterexample**
- A builder reading Unit 8.1 in isolation may correctly fix the backup-path bug (it is explicitly called out) but miss that the `TargetPath` construction on line 106 ALSO hardcodes `.codex`. The acceptance bullet "creates a symlink at `~/.claude` (relative to the configured `HomeDir`) pointing to the selected profile's `HomePath`" implies it, but the acceptance does NOT name `service.go:106` as the site to edit.
- An over-eager builder might extract a provider-keyed config struct (e.g. `providerConfig{targetName, processName, backupSegment}`) — fine — but if the builder takes the minimum-change path, they need to know that `prepareTarget` AND the `TargetPath` line BOTH have hardcoded "codex" assumptions.

**Remediation**
- Tighten Unit 8.1 acceptance with a sub-bullet: "The provider-specific target name (`.codex` vs `.claude`), backup path segment (`global-switch/<provider>/backups`), and host-process name (`codex` vs `claude`) are derived from the provider arg, not hardcoded. Refactor as small as possible — a `providerConfig` lookup table keyed on `domain.Provider` is sufficient; do not over-engineer."
- Add a regression assertion to the existing Codex test: "`TestSwitchCodexSymlinksSelectedProfileAndBacksUpExistingDir` continues to write its backup under `global-switch/codex/backups` (not the new path)." Today the test at `service_test.go:46-90` does NOT assert the backup path's `codex` segment, so a refactor that accidentally writes Codex backups to e.g. `global-switch/claude/backups` would pass the existing suite.

---

## Attack 6 — Host-process guard process name (`claude` vs `Claude.app`)

**Status:** UNKNOWN — defer to builder, but tighten the AC

**Evidence**
- Unit 8.1 acceptance says: "The host-process guard for Claude checks for a running `\"claude\"` process (not `\"codex\"`)."
- The host-process guard uses `pgrep -x -U <uid> <name>` (`service.go:178`). `-x` requires exact match against the comm/argv-0 name.
- On macOS, the user-installed `claude` CLI from `npm i -g @anthropic-ai/claude-code` runs as `claude` (the bin shim is `claude`). But `Claude.app` (the GUI) runs under a different process name. The planner's "Unknowns" section is empty — the assumption was apparently that "claude" is correct.

**Counterexample**
- If the builder accepts the AC verbatim and ships `pgrep -x claude`, a user who has *only* `Claude.app` open (not the CLI) will get past the guard and proceed with the symlink swap. That is arguably correct — `Claude.app` does not read `~/.claude`, the CLI does — but it is the kind of detail that should be validated, not assumed.

**Remediation**
- Add an explicit Unknown to Unit 8.1 (or to the Notes section): "Verify the macOS process name for the `claude` CLI matches `pgrep -x claude` before declaring Unit 8.1 done. Manual verification: with `claude` running, run `pgrep -lx claude` and confirm a PID is returned. If the process name differs (e.g. claude-code), update the constant accordingly."
- This is a builder-side verification step, not a plan-side block, but the plan should *name* it so it does not get silently skipped.

---

## Attack 7 — Cobra `DisableFlagParsing` + `--` separator interaction

**Status:** REFUTED, but Unit 8.3 should add one explanatory line

**Evidence**
- `go doc github.com/spf13/cobra Command.DisableFlagParsing`: "If this is true all flags will be passed to the command as arguments." cobra does NOT process `--`; it falls through to `args` as a literal token.
- `Args: cobra.ArbitraryArgs` confirms cobra does no arg validation either.

**Conclusion**
- The planner's note in Unit 8.3 — "the `stripAccountFlag` function operates on the raw `args []string` before any cobra parsing, which is the correct interception point" — is correct.
- BUT the planner does not say anything about how `stripAccountFlag` should treat `--`. See Attack 1's remediation. The fix is small: a single sentence in Unit 8.3 acceptance documenting the `--` escape hatch (so the builder doesn't add `--` handling redundantly OR forget it entirely).

---

## Attack 8 — Symmetry assertion gap between Unit 8.4 (Claude) and Unit 8.5 (Codex)

**Status:** CONFIRMED (CONCERN — asymmetry is structural)

**Evidence**
- Unit 8.4 (Claude binding UX) is ~22 acceptance lines with explicit 0/1/2+ branching, TTY check, notice level, store wiring.
- Unit 8.5 (Codex parity) is ~7 acceptance lines that say "implement the spec-mandated 0/1/2+ logic" + references back to "same semantics as Unit 8.4".
- The plan says (Notes section, PLAN.md:166): "Symmetry across providers is load-bearing."
- BUT existing `ensureCodexBindingReady` (codex_setup.go:22-46) has a fundamentally different shape:
  - It calls `service.Status` (codex-only, see manage/service.go:245 hardcoded to `ProviderCodex`).
  - It calls `runCodexFirstRunSetup` — a 4-option menu (1 = default host account, 2 = pick existing, 3 = new isolated, 4 = cancel).
  - It calls `ensureManagedAccountReady` in `loginBindAndReportCodexSetup` (codex_setup.go:131) — a host-side auth check that Claude (in-container auth) does not need.
- Unit 8.5 says "2+ Codex accounts → existing interactive menu flow (`runCodexFirstRunSetup`) continues unchanged." But the existing menu IS the multi-option setup that runs even when the user has only 1 account. Unit 8.5 short-circuits the menu when 1 account exists — that is a behaviour change to the existing flow that the AC says "continues unchanged" for 2+ accounts but actually changes for the 0- and 1-account paths.

**Counterexample**
- A Codex user with one existing host account today sees the 4-option menu, which lets them pick option 3 to create a NEW isolated account. After Unit 8.5, the same user gets auto-bound to the single existing account with no opportunity to create an isolated one. That's a *behaviour change* the AC frames as "shortcut" but is actually a UX regression for the "I want to add a second account" path.
- For Claude (Unit 8.4), the equivalent UX is not specified because Claude has no equivalent "default host account" concept — the 4-option Codex menu is structurally different from anything Claude offers.

**Remediation**
- Unit 8.5 acceptance should explicitly call out: "For the 1-account auto-bind shortcut, the user retains the ability to override by passing `--account <name>` (handled by Unit 8.3) or by running `valv manage account add codex <name>` ahead of time. The 4-option menu only fires when 2+ accounts exist, NOT when the user wants to create an additional account during a `valv codex` run."
- OR: explicitly accept the UX change and update the project_valv_binding_ux_spec memory in a follow-up. Either is fine; the plan should not be silent.
- Symmetry note: Unit 8.4's auto-bind path is "auto-bind the only Claude account silently"; Unit 8.5's is similar. Make sure both emit a `laslig.NoticeInfoLevel` notice with the SAME format (e.g. "Bound project <root> to <provider> account <name>") so the dogfood UX is consistent. Today only Unit 8.4 explicitly says "emit a `laslig.NoticeInfoLevel` notice"; Unit 8.5 says the same but does not specify a parallel format.

---

## Attack 9 — DROP_9 interlock: `--account` flag location coupling

**Status:** CONCERN

**Evidence**
- Unit 8.3 puts `stripAccountFlag` in a new file `internal/cli/account_flag.go`. Good — single home for the helper.
- Unit 8.3 ALSO modifies `claude.go` and `codex.go` to call `stripAccountFlag`.
- Unit 8.4 has `ensureClaudeBindingReady` accept `accountOverride`. Unit 8.5 has `ensureCodexBindingReady` accept `accountOverride`. Both files reference the override in their signatures.
- DROP_9 (per project memory `project_valv_cli_audit_proposal.md`) will likely rename commands but should not need to move `stripAccountFlag`.

**Counterexample**
- If DROP_9 ALSO restructures `claude.go`/`codex.go` (e.g. extracts the cobra command builders into a separate provider-keyed factory), the `stripAccountFlag` call sites move too. That is not a DROP_8 problem per se, but DROP_8's surface should make it cheap for DROP_9 to find and update.

**Remediation**
- Add a Note to PLAN.md: "`stripAccountFlag` is the single API DROP_9 must reference if command files move. The function is a stable contract — `(args []string) (accountName string, remaining []string)`. DROP_9 should not re-implement the flag stripper."
- Surgical only. Not a blocker.

---

## Attack 10 — TUI golden parity: format divergence between providers

**Status:** CONCERN

**Evidence**
- `internal/tui/manage/picker.go:48` builds the title via `menu.Title = fmt.Sprintf("%s accounts", provider)`. So Claude's picker title is "claude accounts" vs Codex's "codex accounts".
- `profileItem.Title()` returns `i.name` and `Description()` returns `i.home` (picker.go:18-19). NO provider-specific fields — both providers render `name` + `home`.
- The existing `TestProfilePickerGolden` (golden_test.go:25) is parameterized on `domain.ProviderCodex` and two specific account names + home paths. The proposed `TestProfilePickerGoldenClaude` uses the same shape with `alpha-profile` / `beta-profile` and `/tmp/alpha-profile` / `/tmp/beta-profile`.
- Unit 8.6 acceptance does NOT specify any Claude-specific fields. There aren't any to specify — the picker is provider-agnostic except for the title string.

**Conclusion**
- REFUTED in its strong form: there is no hidden Claude-specific format the planner missed.
- BUT: the proposed `TestProfilePickerGoldenClaude` golden file will differ from the Codex one ONLY in the title line and the account names. That's a fragile test — if `picker.go` changes the title format (e.g. drops the trailing "accounts" suffix or changes case), the Codex golden updates may pass while the Claude golden silently passes too because both got regenerated.

**Remediation**
- Add to Unit 8.6 acceptance: "The Claude golden differs from the Codex golden in (a) the title string ('claude accounts' vs 'codex accounts'), (b) the account names, and (c) the home paths. Diff the two `.golden` files in code review to confirm those are the ONLY differences."
- Surgical only.

---

## Attack 11 — Test injection seams for binding UX (Units 8.4, 8.5)

**Status:** CONCERN — surface a builder decision point in the AC

**Evidence**
- Unit 8.4's tests need to control: (a) is the project bound? (b) how many Claude accounts exist? (c) is stdin/stdout a TTY?
- `internal/cli/claude_test.go:106-167` already shows the patterns in use: `runManage` helper to create projects/accounts on a real SQLite store via `testCodexPaths`, `cmd.SetIn`/`SetOut`/`SetErr` for byte-buffer streams (which are by definition non-TTY).
- `internal/cli/codex_setup_test.go` exists (per `ls` output above) — should follow the same shape.
- Unit 8.4 acceptance lists test cases but does not name the injection mechanism.

**Counterexample**
- A builder unfamiliar with the codebase might introduce a package-level injection variable (e.g. `var listClaudeProfiles = service.ListProfiles`) to swap in a stub, then forget to restore it in a `t.Cleanup`. The existing pattern uses a real SQLite store via `testCodexPaths(t)` — no package vars, no global state.
- A builder may add a `func newClaudeSetup(opts) ClaudeSetup` factory to inject the service — over-engineering vs the existing pattern.

**Remediation**
- Add a Note to Unit 8.4 (and 8.5): "Test injection follows the existing pattern — real SQLite store via `testCodexPaths(t)`, real `runManage` helper to seed accounts/bindings, `bytes.Buffer` streams for non-TTY assertions. Do NOT introduce package-level injection vars or a setup factory."
- Surgical only.

---

## Attack 12 — Hylla outage planning gap

**Status:** UNKNOWN — accept, no plan change needed

**Evidence**
- The Hylla outage limits cross-package usage-graph queries.
- Plan-QA falsification was able to verify all claims via `Read` + `rg` against the local filesystem. No claim in the plan required a usage graph the orchestrator could not reconstruct via grep + read.

**Conclusion**
- The plan does not depend on Hylla being available. Builder verification per-unit will follow the same fallback pattern. Not a finding.

---

## Attack 13 (additional, not in spawn list) — `ensureClaudeBindingReady` MUST avoid `service.Status` per Unit 8.4 note, but the alternative is underspecified

**Status:** CONCERN

**Evidence**
- Unit 8.4 acceptance bullet 5 (the "Note" line about `manage/service.Status` being codex-hardcoded) acknowledges the problem: `manage.Service.Status` at `manage/service.go:245` hardcodes `domain.ProviderCodex` in its `BindingByProjectID` call. So `Status` cannot be used to look up a Claude binding.
- The same bullet then offers TWO alternatives: "check binding via `store.BindingByProjectID` if needed, **or** simply attempt `service.Status` path and fall through on `ErrUnboundProject` to the 0/1/2+ branching."
- The second alternative is WRONG. `service.Status` will return `ErrUnboundProject` even when the project IS bound to Claude (just not Codex), because Status only ever checks the Codex binding. So "fall through on `ErrUnboundProject`" would mean: a project bound to Claude triggers the 1/2+ picker AGAIN, double-binding.

**Counterexample**
- Dogfood scenario: user runs `valv claude` in a project already bound to Claude (per the binding row written on first use). With option-2 wiring, `service.Status(workingDir)` returns `ErrUnboundProject` because the codex binding lookup misses. `ensureClaudeBindingReady` then falls into the 0/1/2+ branch, picker fires (in TTY case), and on selection `BindProject` UPDATES the binding via upsert. End state: identical Claude binding (idempotent), but the user saw an unexpected picker. Worse: if multiple Claude accounts exist and the user picks a different one, the binding silently swaps without confirmation.

**Remediation**
- Unit 8.4 acceptance must remove the second alternative ("or simply attempt `service.Status`..."). Leave ONLY the first: "Check Claude binding via `store.BindingByProjectID(projectID, domain.ProviderClaude)` directly, after detecting the project via `projectdetect.DetectFrom`."
- Optional follow-up: update `manage.Service.Status` to take an explicit `provider domain.Provider` arg. That's a separate refactor — flag it in PLAN.md's Notes section as a future cleanup that DROP_9's CLI audit could fold in, but do NOT add it to Unit 8.4's scope.

---

## Summary of remediation asks (ordered by severity)

1. **BLOCK — Unit 8.1:** Tighten acceptance to name `service.go:106` (hardcoded `.codex` target path) AND `service.go:137` (hardcoded `codex` backup segment) AND the host-process name as the three sites that must become provider-keyed. Add a regression assertion to the existing Codex test for the backup path.
2. **CONCERN — Unit 8.4 (Attack 3):** Resolve the `accountOverride` flow contradiction. Either change `ensureClaudeBindingReady` to return `(domain.Profile, error)` and thread the profile to the launcher, or document a temporary-binding-row alternative. Apply the same fix to Unit 8.5.
3. **CONCERN — Unit 8.4 (Attack 13):** Remove the wrong second alternative for Claude binding lookup. Leave only `store.BindingByProjectID` direct call.
4. **CONCERN — Unit 8.5 (Attack 8):** Specify the 1-account auto-bind UX trade-off explicitly (loses the 4-option menu for the "create an additional account" path). Decide accept or reject; do not leave silent.
5. **CONCERN — Unit 8.3 (Attack 1):** Document `--` escape hatch behaviour. Add a test case.
6. **CONCERN — Unit 8.4/8.5 (Attack 4):** Hoist the 0-account error string into a shared helper so DROP_9 only touches one site.
7. **CONCERN — Unit 8.4/8.5 (Attack 11):** Document the test injection pattern (real SQLite store via `testCodexPaths`, no package vars).
8. **UNKNOWN — Unit 8.1 (Attack 6):** Explicitly mark macOS `claude` process name verification as a builder-side step.
9. **Minor — PLAN.md Notes (Attack 9):** Add a DROP_9 interlock line for `stripAccountFlag` API stability.
10. **Minor — Unit 8.6 (Attack 10):** Document that the Claude vs Codex golden file differences are bounded to title + names + home paths.
11. **Minor — Unit 8.4 (Attack 2):** Add a one-liner clarifying upsert idempotency under concurrent invocations.

The plan can advance to Phase 4 once items 1–4 are addressed (the BLOCK plus the three "contradiction" CONCERNs). Items 5–11 can either be folded into the same revision pass or accepted as builder-side wisdom; none of them gates the build.

## Hylla Feedback

N/A — Hylla MCP was unreachable for this round; all evidence gathered via filesystem `Read` + `rg`. No Hylla queries attempted, so no miss to record.
