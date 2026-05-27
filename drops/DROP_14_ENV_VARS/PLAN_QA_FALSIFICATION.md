# DROP_14 Unit 14.4 — Plan-QA Falsification — Round 1

Agent: `ta-go-plan-qa-falsification` (Agent tool, model=sonnet — fallback after codex-exec primary SIGTERM'd at the 10-min Bash cap; documented `CODEX_EXHAUSTED` path). Transcribed by orchestrator — the persona is READ-ONLY (no Edit/Write). Tool-call audit by orch: Read + ls only; 25 tool uses, all within persona allowlist + TA_ALLOWLIST embed. No Edit/Write, no mage, no git mutations.

Audits the Round-2 decomposition of Unit 14.4 (commit `21e402b`).

## 1. Verdict

**FAIL**

The plan's 14.4.2 droplet claims 2 production symbols ("edit existing methods `claude.Service.Run` and `codex.Service.Run` only"), but both `claude.Store` (service.go:23–27) and `codex.Store` (service.go:22–26) do NOT currently embed `domain.AccountEnvRepository`. For `s.store.ListAccountEnv(ctx, profileID)` to compile in either wrapper, the builder must widen both Store interfaces — two additional top-level production symbol changes. Real symbol count: `claude.Store`, `codex.Store`, `claude.Service.Run`, `codex.Service.Run` = 4 distinct production symbols. This is over the ≥3-symbol FAIL threshold from `aa130dd` (measured-atomic-sizing rule).

This counterexample is concrete and reproducible: attempting to compile 14.4.2 as specified will produce `s.store.ListAccountEnv undefined (type Store has no field or method ListAccountEnv)` in both `internal/services/claude/service.go` and `internal/services/codex/service.go` without interface widening.

## 2. Attack Vectors Tried

- **Vector 1 — Measured budget 14.4.1:** MITIGATED. `LaunchRequest` type edit + `buildRequest` method edit are two production symbols by the letter of `aa130dd`, but they form a cohesive same-purpose edit cluster ("add field, merge it"). The planner's count of 1 is aggressive but defensible under the methodology's "one cohesive same-purpose edit cluster" carve-out. NIT-level, not a FAIL.

- **Vector 2 — Hallucinated/wrong symbols:** PARTIALLY MITIGATED. Core symbols verified: `run.LaunchRequest` (no `AccountEnv` — correctly marked new), `run.Options` (no Store — confirmed), `claude.Service.Run` (confirmed line 127), `codex.Service.Run` (confirmed line 120), 6 reserved keys (confirmed in `adapters/providers/claude/runtime.go:127–141`). Potential hallucination flagged on "manage.Service.ListAccountEnv" — RESOLVED post-hoc by orch grep: the method DOES exist at `internal/services/manage/service.go:683` (added by Unit 14.2). No hallucination.

- **Vector 3 — Hidden production work / Store interface widening:** FAILURE. Both `claude.Store` (embeds `ProjectRepository`, `BindingRepository`, `ProfileRepository` only) and `codex.Store` (same three, no `AccountEnvRepository`) must be widened for 14.4.2 to compile as described. The plan says "Edit existing methods … only" and "Do NOT add a new top-level production helper symbol" — but says nothing about the two Store interface widenings. That is two hidden production symbol changes not counted in the budget, pushing 14.4.2 to 4 production symbols (≥3 = FAIL). The plan's "Design decision" note acknowledges the wrappers "own Store" but never names the widening as production work.

- **Vector 4 — Shipped-but-not-wired / missing droplet:** **FAILURE (post-hoc upgrade by orch).** The agent flagged this as Unknown (OQ-1) due to its tool-call budget cap. Orch grep resolved it: `internal/cli/run.go:202` constructs `runservice.LaunchRequest{}` directly — a THIRD construction site beyond the two wrapper `Service.Run` methods. The Round-2 plan covers only `internal/services/{claude,codex}/service.go` and never edits `internal/cli/run.go`, so `valv run --account` would silently not populate `AccountEnv`. Either 14.4.2 widens to include `internal/cli/run.go` (pushing prod files toward 3 and adding ≥1 more symbol) OR an additional droplet 14.4.x covers the CLI construction site.

## 3. Critical Findings

### CF-1 — 14.4.2 Store Interface Widening: Hidden Over-Budget Production Work (FAIL)

Concrete counterexample:

- `claude.Store` is defined at `internal/services/claude/service.go:23–27` and embeds exactly `domain.ProjectRepository`, `domain.BindingRepository`, `domain.ProfileRepository`. It does NOT include `domain.AccountEnvRepository`.
- `codex.Store` is defined at `internal/services/codex/service.go:22–26` with the identical three-interface embedding.
- `domain.AccountEnvRepository` is defined at `internal/domain/repository.go:43–56` with the `ListAccountEnv(ctx, profileID string) ([]AccountEnvEntry, error)` method the plan directs the builder to call.

Consequence: attempting to build 14.4.2 exactly as specced — "edit existing methods `claude.Service.Run` and `codex.Service.Run` only" and call `s.store.ListAccountEnv(ctx, resolved.profile.ID)` — will fail compilation with `s.store.ListAccountEnv undefined (type Store has no field or method ListAccountEnv)` in both files.

Budget recount for 14.4.2:
- `claude.Store` interface widening — 1 production symbol
- `codex.Store` interface widening — 1 production symbol
- `claude.Service.Run` method edit — 1 production symbol
- `codex.Service.Run` method edit — 1 production symbol

Total: 4 distinct production symbols, 2 files. Threshold is ≥3 = FAIL per `aa130dd`.

Required action: the planner must sub-plan 14.4.2. One natural decomposition:
- 14.4.2a — widen `claude.Store` + `codex.Store` to embed `domain.AccountEnvRepository` (2 interface symbols, interface-only change, no behavior).
- 14.4.2b — edit `claude.Service.Run` + `codex.Service.Run` to call the accessor and populate `LaunchRequest.AccountEnv` (2 method symbols, blocked_by 14.4.2a and 14.4.1).

### CF-2 — Missing droplet for `internal/cli/run.go` LaunchRequest construction site (FAIL, post-hoc orch grep)

`internal/cli/run.go:202` constructs `runservice.LaunchRequest{}` directly for the generic `valv run --account` primitive. The Round-2 plan does not edit this file, so account env would silently NOT propagate on the generic run path despite 14.4.1/14.4.2 landing the wrapper changes. Add a droplet 14.4.x covering `internal/cli/run.go` AccountEnv load+pass.

## 4. NITs

- **NIT-1 — 14.4.1 symbol count:** The plan counts `LaunchRequest` as the single production symbol but `buildRequest` is also edited (a second top-level method on `Service`). Under the strictest reading of `aa130dd`, this is 2 symbols. The "cohesive same-purpose edit cluster" interpretation covers it, but the planner should make this explicit in the measurement note rather than just citing `LaunchRequest`.

- **NIT-2 — `manage.Service.ListAccountEnv` naming:** RESOLVED — orch grep confirmed the method exists at services/manage/service.go:683. No further action.

- **NIT-3 — Duplicate conversion logic policy is asymmetric:** The plan says "If the same conversion logic appears verbatim in both wrappers, the builder MAY introduce a test-only helper but NEVER a production helper." This is a reasonable policy but creates a risk: the `[]domain.AccountEnvEntry` → `map[string]string` conversion is non-trivial (iterate, assign) and will be copy-pasted verbatim in both wrappers (and now in `internal/cli/run.go` too — per CF-2). When that conversion logic is later found to have a bug, the fix must be applied in three places. Accepted design trade-off, but worth flagging — a tiny shared helper in `internal/domain` (e.g. `AccountEnvEntriesToMap`) would fold the three call sites into one production symbol owned by the existing domain package without adding a new top-level production symbol to the wrappers.

## 5. Open Questions

- **OQ-1:** RESOLVED — see CF-2 above.
- **OQ-2:** RESOLVED — see Vector 2 above. `manage.Service.ListAccountEnv` exists at services/manage/service.go:683.

## 6. Tools Used

Direct file reads (sonnet via Agent tool):
- `drops/DROP_14_ENV_VARS/PLAN.md`
- `internal/services/run/service.go` (full)
- `internal/services/manage/service.go` (lines 1–310)
- `internal/services/claude/service.go` (lines 1–280)
- `internal/services/codex/service.go` (lines 1–260)
- `internal/domain/repository.go` (full)
- `internal/adapters/providers/claude/runtime.go` (lines 1–160)

Directory listings:
- `ls internal/services/manage/`
- `ls internal/domain/`

Git: none. Context7: none. No mage. No Edit/Write. No git mutations.

Orch post-hoc grep (resolved OQ-1 + OQ-2): `rg -n "runservice.LaunchRequest\{|run.LaunchRequest\{" internal/`, `rg -n "func \(s \*?Service\) ListAccountEnv" internal/services/manage/`, `rg -n "AccountEnvRepository" internal/services/...`.

## TL;DR

- T1 (Verdict): **FAIL** — 14.4.2 is over budget due to uncounted Store interface widening (CF-1), AND a third LaunchRequest construction site at `internal/cli/run.go:202` is missing from the plan (CF-2, post-hoc orch grep).
- T2 (Attack Vectors): Budget 14.4.1 mitigated (borderline but defensible); symbol existence confirmed (incl. orch-resolved `manage.Service.ListAccountEnv`); Store widening is a concrete compile-time FAIL on 14.4.2; the cli/run.go construction site is a real missing droplet (orch-upgraded from Unknown to Failure).
- T3 (Critical Findings): CF-1 — split 14.4.2 into 14.4.2a (Store widening) + 14.4.2b (method edits). CF-2 — add a droplet covering `internal/cli/run.go` AccountEnv population.
- T4 (NITs): 14.4.1 symbol count is borderline; duplicate conversion-logic policy could fold into a single domain helper to eliminate triplicate copy-paste risk now that there are 3 call sites.
- T5 (Open Questions): All resolved by orch post-hoc grep.
- T6 (Hylla Feedback): Hylla MCP unavailable in this session; all evidence sourced from direct Read calls. Key miss: agent could not run `hylla_refs_find` to find all `LaunchRequest` construction sites — orch grep filled that gap.
