# DROP_13 — Plan QA Proof — Round 5

verdict: pass
round: 5
date: 2026-05-23

## Scope

Round 5 verifies the Round 4 hybrid revisions (planner pass for R4.F1, orch-direct
pass for R4.F2) actually close the two falsification gaps cited in
`PLAN_QA_FALSIFICATION.md` Round 4.

## R4.F1 Audit — Runtime contract surface

**Gap from R4.F1:** the shared run service was described as accepting "provider-
prepared runtime state" without enumerating the minimum contract, leaving the
builder free to pick a shape that drops `EnvPassthrough` / `Warnings` / worktree
gitdir mounts.

**Round 4 hybrid fixes claimed in spawn prompt:**

1. Schema Decision (PLAN.md line 38) now reads: "...with a minimum contract of
   `Env`, `EnvPassthrough`, `Mounts`, `Warnings`, and a `Close()`-style cleanup
   method..."
2. Unit 13.1 acceptance bullet (PLAN.md line 81) enumerates the same five-field
   contract with provenance: "...per `internal/adapters/providers/claude/runtime.go:41-59`
   and `internal/adapters/providers/codex/runtime.go:39-52`..."
3. Unit 13.1 tests bullet (PLAN.md line 86) requires:
   - `EnvPassthrough` names flow through into `ContainerRunRequest.EnvPassthrough`
   - `prepared.Warnings` propagated to notices (cited at
     `internal/services/claude/service.go:191-194`,
     `internal/services/codex/service.go:182-185`)
   - Provider-prepared `Mounts` pass through unchanged, including worktree gitdir
     mount sourced from `pathutil.ResolveWorktreeGitDir` (cited at
     `internal/adapters/providers/claude/runtime.go:143-145`,
     `internal/adapters/providers/codex/runtime.go:113-115`)
   - `Close()` cleanup runs on both success and failure
4. Notes For Builder Agents (PLAN.md line 175) restates the five-field contract
   and the worktree-gitdir provenance.

**Repo verification of cited evidence:**

| Cite in plan | Actual repo location | Verifies |
|---|---|---|
| `claude/runtime.go:41-59` | lines 41-60 — `PreparedRuntime` struct + `Close()` method | Fields present: `ContainerHome`, `Env`, `EnvPassthrough`, `Mounts`, `Warnings`, unexported `cleanup`; method `Close()` invokes cleanup. Minimum contract grounded. |
| `codex/runtime.go:39-52` | lines 39-53 — same struct shape + `Close()` method | Same fields, same method. Minimum contract grounded. |
| `claude/service.go:191-194` | lines 191-194 — `defer prepared.Close()` then `s.emitNotices(resolved.profile, prepared.Warnings, claudeArgs)` | Warnings propagation site confirmed. |
| `codex/service.go:182-185` | lines 182-185 — same `defer prepared.Close()` + `s.emitNotices(..., prepared.Warnings, ...)` pattern | Warnings propagation site confirmed. |
| `claude/runtime.go:143-145` | actual `pathutil.ResolveWorktreeGitDir(projectRoot)` + mount append is at lines 144-146 (1-line drift) | Worktree gitdir mount logic confirmed at the cited range; off-by-one is immaterial. |
| `codex/runtime.go:113-115` | actual logic at lines 114-116 (1-line drift) | Worktree gitdir mount logic confirmed; off-by-one is immaterial. |

**Verdict on R4.F1:** the three plan locations (Schema, Unit 13.1 acceptance, Unit
13.1 tests, plus Notes echo) collectively pin the minimum contract to a named
five-field surface, anchor the surface to specific struct lines in both runtime
packages, and force tests to exercise each contract field (EnvPassthrough,
Warnings, Mounts including worktree gitdir, Close on both paths). The R4.F1 gap
is closed.

## R4.F2 Audit — VALV_*_IMAGE collision+override test missing

**Gap from R4.F2:** Unit 13.2 covered VALV_*_IMAGE override behavior under
`valv run` but did not cover the duplicate-account-name + explicit-`--provider` +
single-provider-override combined scenario. Builder could ship without exercising
the path where `resolveAccountByName` narrows by `--provider` AND `resolveProjectImage`
short-circuits via `VALV_CLAUDE_IMAGE` (or mirror Codex case).

**Round 4 hybrid fix claimed in spawn prompt:** Unit 13.2 gained a new acceptance
bullet (PLAN.md line 110) covering:

- create an account named `work` in both Claude and Codex
- run `valv run --account work --provider claude <cmd>` with a non-empty
  `.valv/tools.toml`, with `VALV_CLAUDE_IMAGE` set and `VALV_CODEX_IMAGE` unset
- launch must emit exactly one Claude override warning, make zero overlay-image
  docker calls, and pass the Claude override-derived base image into the shared
  run service
- mirror Codex case (`--provider codex` + `VALV_CODEX_IMAGE` set) must succeed
  identically

**Repo verification of `resolveAccountByName` behavior the test relies on:**

`internal/cli/manage.go:1118-1162` confirmed:
- lines 1123-1133: when `providerFlag != ""`, lookup is provider-narrowed via
  `service.ProfileByName(ctx, p, accountName)` — the test's `--provider claude`
  branch selects the Claude profile of the duplicate-named pair deterministically.
- lines 1140-1150: cross-provider scan only fires when `providerFlag == ""`, so
  the explicit-`--provider` path bypasses the collision branch.
- lines 1156-1162: collision branch only matters in the no-`--provider` case the
  test does NOT enter, so collision semantics do not interfere.

`resolveProjectImage` override short-circuit (`internal/cli/operator_helpers.go:421-470`,
already verified in earlier rounds) returns the base ref unchanged when
`VALV_<PROVIDER>_IMAGE` is set, which is the "zero overlay-image docker calls"
behavior the new test asserts.

**Verdict on R4.F2:** the new Unit 13.2 acceptance bullet covers the exact
duplicate-name + explicit-provider + single-provider-override scenario that R4.F2
called out, and includes the symmetric Codex mirror. The R4.F2 gap is closed.

## Sanity checks

- **blocked_by chain acyclic:** 13.1 (none) → 13.2 (13.1) → 13.3 (13.2) →
  13.4 (13.3) → 13.5 (13.4). Linear, no cycles.
- **No new units introduced** between Round 4 and Round 5. Five units remain.
- **Drop-level acceptance unchanged structurally:** Acceptance Criteria block
  (lines 57-66) still covers the same eight bullets that survived Round 4
  hybrid revisions; the override-preservation bullet (line 65) remains intact.
- **Notes For Builder Agents** expansion is consistent with Schema and Unit
  acceptance — no contradiction across the three locations.

## Findings

- F1 (note, not blocker): Two cites in Unit 13.1 tests bullet have a 1-line
  off-by-one drift relative to the actual repo (`claude/runtime.go:143-145`
  vs actual `144-146`; `codex/runtime.go:113-115` vs actual `114-116`). The
  cited concept (worktree-gitdir mount via `pathutil.ResolveWorktreeGitDir`) is
  unambiguously present at the cited range. Builder agents using `Read` with
  these cites will find the right code instantly. Suggest minor cleanup in the
  next planner pass if any other revision happens; do not block on this alone.

## Summary

verdict: **pass**

R4.F1 closed via four-location convergence (Schema Decision, Unit 13.1
acceptance, Unit 13.1 tests, Notes For Builder Agents) all naming the same
five-field minimum contract (`Env`, `EnvPassthrough`, `Mounts`, `Warnings`,
`Close()`) anchored to real struct lines in both `PreparedRuntime` types.

R4.F2 closed via one new acceptance bullet in Unit 13.2 covering duplicate-name
+ explicit-provider + single-provider-override + Codex mirror, grounded in
`resolveAccountByName` semantics that the existing repo source already supports.

blocked_by chain remains acyclic, no new units introduced, drop-level
acceptance unchanged. One minor off-by-one cite-drift noted (non-blocking).
