# DROP_13 — Plan QA Falsification — Round 2

**Verdict:** FAIL (concrete counterexamples remain)

## Round 1 blocker resolution audit

- A1 — partially resolved, no current blocker. The revised plan now explicitly requires `internal/services/claude` and `internal/services/codex` to become thin wrappers over `internal/services/run` while preserving the public `Service.Run` API (`drops/DROP_13_GENERIC_RUN/PLAN.md:36-37`, `117-119`). The current Claude/Codex service diff still shows a clean shared seam: the duplicated launch flow is the same except for provider id, other-provider lookup target, runtime preparer, Codex shared-home derivation, and notice copy (`internal/services/claude/service.go`, `internal/services/codex/service.go`).
- A6 — partially resolved, still broken. The planner now correctly states that `valv run` must not auto-create a project row and that `BindProject` can create the missing row when the user explicitly binds (`drops/DROP_13_GENERIC_RUN/PLAN.md:40-41`, `55`, `99`; `internal/services/manage/service.go:194-220`; `internal/services/manage/service_test.go:379-414`). The remaining problem is the suggested recovery command; see confirmed attack N1 below.
- A7 — resolved. The plan now adds `--provider` and explicitly ties it to `resolveAccountByName` semantics (`drops/DROP_13_GENERIC_RUN/PLAN.md:46-47`, `54`, `95-100`). The current helper and tests already prove the intended collision and explicit-provider behavior (`internal/cli/manage_test.go:1003-1079`).

## New attacks attempted

### N1 — no-project-row recovery message still suggests the wrong bind command

- Hypothesis:
  After the Round 2 revision, `valv run --account <name> --provider <provider>` can still return a recovery command that is not actionable.
- Evidence:
  The revised acceptance requires the no-project-row path to suggest `valv account bind <name>` (`drops/DROP_13_GENERIC_RUN/PLAN.md:55`, `99`). The real bind command does create a missing project row (`internal/services/manage/service.go:194-220`), but the bare one-arg form is not always valid: when the same account name exists in both providers, `account bind <name>` fails and instructs the user to add `--provider` (`internal/cli/manage.go:343-410`; `internal/cli/manage_test.go:1119-1140`). Explicit-provider resolution is already a first-class case in the same plan (`drops/DROP_13_GENERIC_RUN/PLAN.md:54`, `97`).
- Outcome:
  CONFIRMED blocker.
- Reproduction:
  1. Create account `work` in both Codex and Claude.
  2. Run the planned command from an unbound project: `valv run --account work --provider claude <command>`.
  3. The plan says the error should suggest `valv account bind work`.
  4. Existing repo behavior proves that `valv account bind work` is ambiguous in exactly this setup and fails until `--provider claude` (or positional `claude`) is supplied.
- Why this breaks the claim:
  Round 1 asked whether `valv account bind <name>` was the right suggestion. The revision only proved that bind can create the project row; it did not prove that the suggested command is valid for the resolved account. The recovery text needs to preserve provider context.

### N2 — the planned flag-stripping model can still steal target-command flags

- Hypothesis:
  The revised `valv run` surface still admits an implementation that violates its own passthrough contract for target commands that use `--account` or `--provider`.
- Evidence:
  Unit 13.2 says `valv run` manually consumes local `--account` and `--provider` flags without requiring `--`, while leaving target-command flags untouched (`drops/DROP_13_GENERIC_RUN/PLAN.md:95-100`). Its evidence points at the existing `stripAccountFlag` helper (`drops/DROP_13_GENERIC_RUN/PLAN.md:91`), but that helper intentionally strips `--account` from the middle of the arg list, not just from a leading local-flags prefix (`internal/cli/account_flag.go:3-55`; `internal/cli/account_flag_test.go:60-105`).
- Outcome:
  CONFIRMED blocker.
- Reproduction:
  1. Consider a target command that legitimately owns `--provider`, for example `some-tool --provider staging`.
  2. Invoke the planned surface: `valv run --account solo some-tool --provider staging`.
  3. A parser built in the shape of the cited helper will consume the target command's `--provider staging` as Valv-local input unless the planner also defines a hard boundary such as "local flags only before the first command token" or adds explicit collision tests.
- Why this breaks the claim:
  The plan promises both "no `--` separator required" and "leave target flags untouched," but the cited extraction model does not satisfy that promise for same-named flags. Without a stricter parsing rule and tests for target-owned `--account` / `--provider`, a broken implementation remains plan-compliant.

### N3 — `DisableFlagParsing` help routing is implementable

- Hypothesis:
  Cobra's `DisableFlagParsing` makes `valv run --help` versus `valv run <command> --help` impossible to route reliably.
- Evidence:
  `go doc github.com/spf13/cobra.Command.DisableFlagParsing` confirms only that all flags are passed through as args. The current provider launchers already implement manual help/version routing above Cobra parsing (`internal/cli/claude.go:46-63`, `181-194`; `internal/cli/codex.go:51-68`, `195-218`).
- Outcome:
  Mitigated. Manual arg inspection plus `cmd.Help()` is enough here.

### N4 — shared-service wrapper shape can preserve cross-mount and provider quirks

- Hypothesis:
  Slimming Claude/Codex into wrappers around `internal/services/run` will force either a bloated provider-policy struct or loss of provider-specific behavior.
- Evidence:
  The current service bodies share the same launch skeleton; the provider-specific deltas are small and explicit: provider id, other-provider binding lookup target, runtime preparer, Codex shared-home derivation, and notice text (`internal/services/claude/service.go`, `internal/services/codex/service.go`). The plan already requires the wrappers to keep ownership of provider-specific runtime prep / notice / label policy (`drops/DROP_13_GENERIC_RUN/PLAN.md:75-79`, `117-119`).
- Outcome:
  Mitigated. No concrete counterexample found on this surface.

## Verdict Summary

- FAIL. Round 1 blocker A7 is resolved and A1 is now specified cleanly enough, but A6 is only partially fixed: the no-project-row recovery command still drops provider context and can be wrong for the exact disambiguated-account case the revision adds.
- FAIL. Unit 13.2 still mixes "no separator required" with an extraction model that scans through passthrough args, so target-owned `--account` / `--provider` flags remain vulnerable unless the planner tightens the parsing rule and test matrix.
