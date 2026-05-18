# Plan QA Falsification — DROP_8 Round 4

**Drop:** DROP_8_GLOBALSWITCH_AND_TUI_PARITY
**Phase:** 2 (Plan QA, Round 4)
**Verdict:** **pass**

R4 diff is +16/-4 on `drops/DROP_8_GLOBALSWITCH_AND_TUI_PARITY/PLAN.md` only — no other files touched. R4 applied the five R3-mandated edits (Unit 8.5 paths add, Unit 8.5 errCodexSetupCanceled deletion, Unit 8.5 step-flow rewrite, Unit 8.5 test migration AC, Unit 8.7 three-case error enumeration + both-bound policy). Every R3 BLOCK/CONCERN/SURGICAL is concretely addressed and survives counterexample attack with no unmitigated finding. The two genuine R4-introduced ambiguities (test-migration "or deleted" discretion, error-message tone) are below the BLOCK/CONCERN threshold and the planner can ship as-is.

---

## R3-Refutation Attempts

### R3-B1 — Unit 8.5 test path coverage (REFUTED, no counterexample)

**Claim under attack:** R4 added `internal/cli/codex_test.go` to Unit 8.5 paths and added a migration AC for `codex_test.go:194` + `:223`. Are there OTHER callers of `ensureBoundCodexAccountReady` beyond those two lines + the function definition surface?

**Evidence:** `git grep -n ensureBoundCodexAccountReady`:

- `internal/cli/codex.go:78` — production call site (covered by Unit 8.5 caller-change AC).
- `internal/cli/codex.go:204` — function definition itself (covered by merge AC).
- `internal/cli/codex_test.go:194,195` — test caller (covered by R4 migration AC).
- `internal/cli/codex_test.go:223,224` — test caller (covered by R4 migration AC).

Plus historical markdown references in DROP_5/6/8 docs — not source code.

**Conclusion:** No additional callers exist. The R4 path + AC pair is exhaustive. REFUTED.

### R3-C1 — Unit 8.7 error discrimination + wrap depth (REFUTED, no counterexample)

**Claim under attack:** Does the planner's wrap-on-other-error pattern produce a chain too deep for downstream consumers?

**Trace:**

1. `StatusForProvider` may itself wrap: e.g. `fmt.Errorf("status %s: %w", provider, err)` if the implementation mirrors `Status`. Final caller wraps as `fmt.Errorf("detect globalswitch provider: %w", err)`. Chain becomes `detect globalswitch provider: status claude: <root>`.
2. `errors.Is(err, domain.ErrUnboundProject)` unwraps through `%w` chains by stdlib contract — verified semantics (Go `errors` package). Wrap depth is irrelevant to discrimination correctness.
3. The R4 enumeration explicitly checks via `errors.Is(err, domain.ErrUnboundProject)` (line 206), so even if `StatusForProvider` internally wraps `ErrUnboundProject`, the check still fires correctly.
4. The only downstream consumer of the wrapped error is the human reading the error message; readable chain depth ≤3 layers is well within Go convention.

**Conclusion:** Wrap depth not load-bearing. REFUTED.

### R3-S1 — Both-bound dispatch policy wording (REFUTED, no counterexample)

**Claim under attack:** Could a builder interpret "Claude wins" as "Claude-bound takes precedence over Codex-bound for binding resolution" vs "the dispatch always returns ProviderClaude when both exist"?

**Trace:** The R4 line (PLAN.md:213) reads: *"Dispatch policy: when both Claude and Codex bindings exist for the same project, Claude wins. The step-1-first probe order encodes this policy."*

The second sentence is the disambiguator — it explicitly ties the policy to the step ordering (step 1 = Claude probe). Any builder following the step structure literally lands on the correct behavior regardless of how they read "wins" semantically. The structural encoding (probe Claude first, return on nil) makes interpretation moot.

**Conclusion:** Wording-plus-structure removes interpretive ambiguity. REFUTED.

### R3-S2 — `errCodexSetupCanceled` deletion scope (REFUTED, no counterexample)

**Claim under attack:** Are there consumers of `errCodexSetupCanceled` beyond `codex.go:72-74`?

**Evidence:** `git grep -n errCodexSetupCanceled`:

- `internal/cli/codex.go:73` — the `errors.Is` consumer (covered by R4 deletion AC).
- `internal/cli/codex_setup.go:20` — sentinel declaration (covered by R4 deletion AC).
- `internal/cli/codex_setup.go:57,76,98,118` — `return errCodexSetupCanceled` sites inside `runCodexFirstRunSetup` and friends.

The four sites at `codex_setup.go:57,76,98,118` are inside `runCodexFirstRunSetup` / `loginBindAndReportCodexSetup` / `writeCodexSetupIntro` / `writeCodexSetupResult` — the same functions explicitly deleted by Unit 8.5's "Dead-code cleanup" AC (PLAN.md:163). When those functions are deleted, the four `return errCodexSetupCanceled` sites vanish with them. The single remaining consumer (`codex.go:73`) is what the R4 deletion AC explicitly targets.

No test, no other production package, no doc test, no `_test.go` reference. REFUTED.

### R3-Polish — Unit 8.5 step-flow merge correctness (REFUTED, no counterexample)

**Claim under attack:** Does the R4 rewrite leave the override-case (step 1) ambiguously routed? Does it short-circuit through `ensureManagedAccountReady` or just return?

**Trace:** R4 wording (PLAN.md:154): *"Store the resolved profile and proceed to step 4 (`ensureManagedAccountReady`). Does NOT write a binding row."*

- Step 1 → step 4 (explicit jump).
- Step 4 (PLAN.md:160): *"Ensure managed account is ready: call `ensureManagedAccountReady(...)` on the resolved profile (whether from override, existing binding, auto-bind, or picker)."* — the "or override" branch is in the enumeration.
- Step 5 (PLAN.md:161): skip when `codexArgsSkipAccountReady(args)` returns true.

The flow is: override → resolve → skip writing binding → run `ensureManagedAccountReady` → return profile. This matches the documented contract that override behaves "auth-only, no binding row written." No short-circuit-out-of-the-function in the override case; the function continues through step 4/5. The non-contiguous jump (1→4) is explicit and visible.

**Conclusion:** Flow is unambiguous and correctly merges the two prior functions. REFUTED.

---

## R4 Fresh-Attack Vectors

### R4-V1 — Line-number reference for `codex.go:72-74` (REFUTED, no counterexample)

**Claim under attack:** The R4 AC says delete `errors.Is(err, errCodexSetupCanceled)` branch at `internal/cli/codex.go:72-74`. Does this match the actual file?

**Evidence:** Direct read of `internal/cli/codex.go:72-77`:

```
72:	if err := ensureCodexBindingReady(cmd, paths, workingDir); err != nil {
73:		if errors.Is(err, errCodexSetupCanceled) {
74:			return nil
75:		}
76:		return fmt.Errorf("run codex command: %w", err)
77:	}
```

The R4 reference `:72-74` is correct (lines 72-74 span the `if` guard, the `errors.Is` test, and the `return nil`). A pedant could argue the full branch including the closing brace runs through line 75, but the AC's intent is unambiguous: delete the inner `errors.Is` block. The builder will land on the right edit. REFUTED.

### R4-V2 — Migration vs deletion AC vagueness (ACCEPTED — below CONCERN bar)

**Claim under attack:** The AC says "migrated to invoke the new merged function `ensureCodexAccountReadyForLaunch` directly (or deleted if redundant)" — builder discretion may produce divergent outcomes.

**Trace:** The two outcomes are:

- **Migrate:** rewrite `codex_test.go:194` + `:223` to call `ensureCodexAccountReadyForLaunch` with override="" and the same args. Preserves the codex_test.go-side integration coverage.
- **Delete:** remove the two tests entirely if `codex_setup_test.go`'s new test suite covers the same scenarios.

Both outcomes satisfy the verification gate (`git grep ensureBoundCodexAccountReady` returns no matches in `internal/cli/` after the change) and both keep mage testPkg green. The new `codex_setup_test.go` test suite enumeration (PLAN.md:165) explicitly covers override-bound, override-unbound, no-override+0/1/2+ accounts, TTY/non-TTY, already-bound — a superset of what `codex_test.go:194,223` exercise (which were `resume --last` and `login` arg-skip cases).

**Counterexample search:** Could a builder pick "delete" when "migrate" was needed (or vice versa) and break coverage? The `codex_test.go:194,223` tests use `args=["resume", "--last"]` and `args=["login"]` — both of which trigger `codexArgsSkipAccountReady` per Unit 8.5 step 5 (the same logic that's being preserved). The new test suite covers "already-bound short-circuit" which subsumes the arg-skip paths. Deletion-vs-migration is genuinely an aesthetic choice with no functional divergence.

**Conclusion:** This is intentional discretion delegated to the builder. Below CONCERN threshold; the AC's verification gate (grep returns no matches + mage testPkg passes) catches any wrong call. ACCEPTED.

### R4-V3 — Wrapped error message string tone (ACCEPTED — below CONCERN bar)

**Claim under attack:** Is `fmt.Errorf("detect globalswitch provider: %w", err)` user-facing or debug-only? Does it need polishing?

**Trace:** The error returned from `runManageHome`'s `ActionGlobalSwitch` case propagates up to cobra, which prints `Error: <chain>` to stderr. So yes, user-facing.

Convention check across the codebase:

- `internal/cli/codex.go:70` uses `fmt.Errorf("run codex command: resolve working directory: %w", err)` — same lowercase, colon-separated, no terminal period pattern.
- `internal/cli/codex.go:76,79,100,117,120,123,127` all follow the same `<verb> <noun>: %w` pattern.
- AGENTS.md § 6 requires `fmt.Errorf("context: %w", err)` at every boundary that adds information.

The R4 wording matches house style. The lowercase imperative ("detect globalswitch provider") fits the existing wrap-chain pattern when prepended to `<root>`. A user seeing `Error: detect globalswitch provider: <some sqlite error>` gets a readable trace.

**Could it be more user-friendly?** Sure — something like "could not determine which provider is bound to this project" reads better in isolation. But that breaks the wrap-chain convention used everywhere else in this file. Consistency wins; this is a debug-flavored wrap and that's intentional.

**Conclusion:** Matches house convention. Below CONCERN bar. ACCEPTED.

### R4-V4 — 3-case enumeration verbosity (REFUTED — verbosity is the point)

**Claim under attack:** Two probe steps × three cases = six case-bullets; could this be collapsed?

**Trace:** The R3 CONCERN explicitly demanded "all three error cases enumerated explicitly per probe." The R4 expansion to six case-bullets is the direct response. Collapsing now would re-create the R3 finding.

Could a collapsed form be just as clear? E.g.:

> For each probe: nil → use that provider; ErrUnboundProject → continue; otherwise → wrap-and-return.

That works syntactically, but:

- It hides the asymmetry: step-2's `ErrUnboundProject` branch falls back to Codex (backwards compat), not "continue." A single rule would mask this.
- The enumeration is the spec the builder will literally implement as switch/if-else. A 1:1 mapping from AC bullet to Go branch is desirable for a verification surface.

**Conclusion:** Verbosity is structurally load-bearing. The asymmetry at step 2 forbids collapsing. REFUTED.

---

## Cross-Cutting Constraint Re-checks

- **Symmetry across providers** — Unit 8.5 merge is Codex-only; Unit 8.4 keeps Claude flat. The asymmetry justification (PLAN.md:238) survived R3 and is untouched in R4. No new asymmetry introduced.
- **Current command tree (pre-DROP_9)** — R4 changes do not introduce new command names. `valv manage *` and `valv account *` paths untouched. CLEAR.
- **`--account` flag is DROP_8 territory** — R4 changes do not add or remove flag wiring; the override parameter is internal to `ensureCodexAccountReadyForLaunch`. CLEAR.
- **No re-introduction of auto-open** — R4 changes do not touch Claude OAuth flow. CLEAR.
- **No Tillsyn / Hylla unreachable** — N/A for this round, no Hylla queries attempted.

---

## Verdict

**pass.**

All five R3 findings refuted by R4 with concrete evidence. The four R4-introduced fresh attack vectors either landed in REFUTED (V1, V4) or ACCEPTED-below-CONCERN (V2, V3). No remediation required. Planner can route to discuss + proceed to Phase 4 (build) once dev approves.

### Suggestions (non-blocking, take or leave)

- **R4-V2 (migrate vs delete):** if the planner wants to remove all builder discretion, pick one. The cheaper-to-verify outcome is "delete" — the new test suite already covers the cases. But this is fine to leave to the builder.
- **R4-V3 (error tone):** consider whether `detect globalswitch provider` will confuse end users in stderr. If "globalswitch" isn't part of the user vocabulary, "detect bound provider for global switch" reads better while staying convention-compliant. Cosmetic.

---

## Hylla Feedback

N/A — no Hylla queries attempted this round (Hylla unreachable per round-4 prompt + all attack surfaces resolvable via `git grep` and direct Read of `internal/cli/codex.go`).
