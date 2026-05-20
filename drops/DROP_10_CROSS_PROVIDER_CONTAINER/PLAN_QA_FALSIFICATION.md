# DROP_10 — Plan QA Falsification — Round 2

**Date:** 2026-05-19
**Drop:** DROP_10_CROSS_PROVIDER_CONTAINER
**Round:** 2
**Reviewer:** go-qa-falsification-agent
**Verdict:** **PASS** (no CONFIRMED counterexample; advisories remain)

R1 dev-accepted findings F1/F2/F3/F4 applied by the planner. This round attacks
the R2-revised PLAN.md (commit `8967680`) for (1) new contradictions or drift
introduced by R2, (2) residual R1 advisories V4 / V5 / V11, and (3) the F3
line-range sharpening for Unit 10.3.

---

## 1. Attack surface — new R2 prose

### 1.1 BuildArgs sort order (R2 F3 fix for Unit 10.1)

**Claim under attack:** R2 rewrote the args-comparison guidance to assert
`docker.BuildImageArgs` sorts keys alphabetically via `sort.Strings(keys)` and
that the expected slice is
`CLAUDE_VERSION` → `CODEX_VERSION` → `VALV_GID` → `VALV_UID`.

**Attack:**

- `internal/adapters/docker/ops.go:83` confirms `sort.Strings(keys)` is applied
  to `BuildArgs` keys before `--build-arg` emission. Stable alphabetic.
- `internal/services/images/service_test.go:110` today's `want` slice already
  has `CODEX_VERSION` < `VALV_GID` < `VALV_UID` alphabetic. Inserting
  `CLAUDE_VERSION` at index 0 (before `CODEX_VERSION`) is the correct collision
  point.
- R2 claim correct: REFUTED.

### 1.2 Unit 10.3 insertion site (F3 sharpening)

**Claim under attack:** R2 sharpened Unit 10.3's `PrepareRuntime` insertion
site to "immediately after the initial `mounts := []dockeradapter.MountSpec{...}`
+ `env := map[string]string{...}` block, BEFORE the `newBridgeManager` call".

**Attack against actual `internal/adapters/providers/codex/runtime.go`:**

- Lines 104–106: `mounts := []dockeradapter.MountSpec{...}` initialised with
  `runtimeCodexHome → ContainerCodexDir`.
- Lines 107–113: `env := map[string]string{...}` initialised with `CODEX_HOME`,
  `HOME`, `LOGNAME`, `TERM`, `USER`.
- Line 115: `bridgeManager, err := newBridgeManager(ctx, request.Logger)`.

  The sharpened insertion site (between line 113 and line 115) is the **actual
  right insertion site**. Placing the cross-mount block here gives codex
  `PrepareRuntime` the same shape as claude `PrepareRuntime` (where the
  insertion is also right after the initial `mounts/env` block).

- Counter-attack: could the cross-mount appear *after* `bridgeManager` /
  `translateConfigFile` / project-overlay logic instead? The cross-mount needs
  none of those (no overlay translation, no project config parsing), so placing
  it earlier minimizes cleanup-on-error scope. The proposed site is correct.

- F3 claim correct: REFUTED.

### 1.3 Recipe-hash auto-update claim (R2 F4 fix for Cache-busting Notes)

**Claim under attack:** R2 rewrote the Cache-busting Notes paragraph to say
recipe-hash fakes (`fakeClaudeRecipeHash`, `fakeCodexRecipeHash`) "auto-update
because they call the functions dynamically — no constant churn needed."

**Attack:** Need to verify `fakeClaudeRecipeHash` / `fakeCodexRecipeHash` are
indeed function-call delegations, not hard-coded sha256 string constants.

- Searched `internal/cli/extended_test.go`: the recipe-hash fakes ARE function
  delegations to `DefaultCodexDockerfile()` / `DefaultClaudeDockerfile()` via
  `svcRecipeHashForTest`. Since Unit 10.1 modifies the dockerfile-returning
  functions, the hashes recompute dynamically. The R2 claim survives.
- Recipe-hash claim correct: REFUTED.

### 1.4 Unit 10.3 acceptance criteria symmetry vs Unit 10.2

**Claim under attack:** R2 sharpened both Unit 10.2 and Unit 10.3 acceptance
criteria to mirror each other (assert `*_DIR` env var set AND mount target).

**Attack:** Read both acceptance blocks side-by-side. 10.2 acceptance asserts
`CODEX_HOME` absent/set and `/home/valv/.codex` mount absent/present. 10.3
acceptance asserts `CLAUDE_CONFIG_DIR` absent/set and `/home/valv/.claude`
mount absent/present. Symmetric.

- The env var names differ between providers (`CODEX_HOME` for codex CLI vs
  `CLAUDE_CONFIG_DIR` for claude CLI) — this asymmetry is intentional (per
  Unit 10.2's existing claude runtime code at line 119 `"CLAUDE_CONFIG_DIR"` and
  codex runtime line 108 `"CODEX_HOME"`). The cross-provider mount inverts these:
  in the claude container the cross-env is `CODEX_HOME`, in the codex container
  the cross-env is `CLAUDE_CONFIG_DIR`. R2 has this correct.
- Symmetry claim correct: REFUTED.

### 1.5 New R2 prose drift hunt

**Attack:** Did the R2 edits introduce any new contradiction with surrounding
prose?

- R2 inserted `BuildArgs` alphabetic detail in Unit 10.1 — surrounding prose
  still says "after the primary build-arg, add the cross-provider build-arg"
  which is now superseded by "always emit both" and the explicit ordering.
  **Mild redundancy, not a contradiction.** Unit 10.1 line 96 ("after the
  primary build-arg (`CODEX_VERSION` or `CLAUDE_VERSION`), add the
  cross-provider build-arg") talks about `BuildArgs` map insertion order, which
  is irrelevant because `BuildImageArgs` sorts. Could mislead a builder briefly
  into thinking the map-insertion order matters. **ADVISORY (not CONFIRMED) —
  not a blocker, builder will read the sort statement two paragraphs later.**

- R2 sharpened both Unit 10.2 and Unit 10.3 acceptance criteria but did NOT
  update the Notes section about `TestPrepareRuntimeHasNoCodexEnv` (lines
  375–377). That Notes paragraph still says "Keep an asymmetric
  `TestPrepareRuntimeSkipsCodexMountWhenNotProvided`" — which is now redundant
  with the rewritten acceptance criterion in Unit 10.2 itself. **Mild
  duplication, not a contradiction.** ADVISORY.

- No CONFIRMED counterexample on R2-introduced drift.

---

## 2. Residual R1 advisories — re-attack

### 2.1 V4 — `CrossProviderVersion` defaults to `"latest"`

**Status:** **NOT addressed in R2.** The R2 plan still has Unit 10.1 say
"When `CrossProviderVersion` is empty, use `"latest"` as the default
cross-version."

**Counter-attack — is this a real falsification?**

- Reproducibility cost: `npm install --global "@openai/codex@latest"` resolves
  at image build time to whatever npm registry returns. Two `mage image update
  claude` runs minutes apart could pull different codex versions. Pinned recipe
  hashes won't match across machines, even on identical Dockerfiles.
- Versus `Version` (primary): `Build()` line 296–298 requires
  `request.Version` non-empty; primary version IS pinned by
  `EnsureLatest` / `CodexVersionResolver` / `claudeVersionResolver`. The
  asymmetric default for the CROSS version is a deliberate planner trade-off
  (Notes lines 51–58: "valid npm syntax, not reproducible but acceptable for
  the cross-provider secondary install at dogfood stage").
- Dev called this stage "dogfood blocker" and signed off Option A. Asymmetric
  pinning is consistent with the dogfood scope.

**Verdict:** ADVISORY (not CONFIRMED). Pinned-cross-version is a v0.1.0
hardening item, not a plan falsification. Recommend the planner add an explicit
follow-up TODO in PLAN.md Notes pointing at DROP_11 / future drop to pin
cross-versions via the same resolver pattern. **Not a blocker.**

### 2.2 V5 — fakeStore atomic dispatch (BindingByProjectID + ProfileByID)

**Status:** **PARTIALLY addressed.** R2 keeps Unit 10.4's directive to
"Override `BindingByProjectID` to return `f.crossBinding, f.crossBindingErr`
when the requested provider is `domain.ProviderCodex`" AND "Add `ProfileByID`
to also dispatch on profile ID between primary and cross profile."

**Counter-attack — is mid-commit state really a falsification?**

- Per WORKFLOW.md Phase 4, builder edits the unit's `state` to `in_progress`,
  implements ALL of unit 10.4's `paths` (service.go + service_test.go),
  edits `state` to `done`, returns control. Orchestrator then commits with
  one `feat(...)` commit covering the whole unit. There is no
  "mid-commit half-finished fakeStore" intermediate state visible to QA or
  CI.
- However: if the builder modifies fakeStore's `BindingByProjectID` to
  dispatch on provider BEFORE updating `ProfileByID`, then re-runs any test in
  the same file, the test could fail mid-edit because the codex cross-profile
  case calls `ProfileByID(crossBinding.ProfileID)` but the existing single-
  field `ProfileByID` returns `f.profile` regardless of ID. This is a
  test-iteration-during-development concern, not a commit-state concern, and
  only affects `mage testPkg` runs during builder iteration.

**Verdict:** ADVISORY (not CONFIRMED). The single-unit-commit boundary makes
V5 a non-issue post-commit. The planner should consider adding an acceptance-
criterion sub-bullet to Unit 10.4 and Unit 10.5: "fakeStore.BindingByProjectID
AND fakeStore.ProfileByID both gain provider/ID-keyed dispatch in the same
edit." This is mild AC tightening — not a blocker. **Not CONFIRMED.**

### 2.3 V11 — post-upgrade first-run UX

**Status:** **NOT addressed in R2.** The R2 plan does not mention what happens
when a user upgrades Valv to a version with cross-provider images but has not
yet run `mage image update`.

**Counter-attack — is this actually broken?**

- Post-upgrade with stale image: the running container is the OLD image (only
  one CLI installed). Cross-call inside the container fails with the OLD
  container's shell's `command not found` for the other CLI — exactly the
  same UX as today's pre-DROP_10 state. No new regression.
- Post-`mage image update`: new image has both CLIs. Cross-call works (if
  binding exists) or fails with "not logged in" (if binding doesn't exist).
  Expected.
- Plan does NOT need to specify upgrade UX — the UX gracefully degrades to
  pre-DROP_10 behavior until `mage image update` runs. This is the expected
  Valv image-update model (see existing `mage image update` semantics in
  drops 5–7).

**Verdict:** ADVISORY (not CONFIRMED). README / changelog should mention
"after upgrade, run `mage image update` to enable cross-provider tool use."
That's a DROP_11 README task. **Not CONFIRMED.**

---

## 3. New attack surfaces (R2 fresh attacks)

### 3.1 fakeStore field rename impact on existing tests

**Attack:** Unit 10.4 adds `crossBinding`, `crossBindingErr`, `crossProfile`,
`crossProfileErr` fields to `fakeStore`. Does this break the existing
`fakeStore` literal initialisers in service_test.go?

- Read service_test.go lines 22–29: current fakeStore has six fields:
  `project`, `projectErr`, `binding`, `bindingErr`, `profile`, `profileErr`.
  Tests initialise via named-field literals (lines 148–152 in
  `boundClaudeStore`, line 167 in TestNewRequiresDependencies, etc.).
- Adding NEW fields with named-field literal initialisation is
  forward-compatible — new fields default to zero values; existing tests pass
  unchanged. The new `crossBinding` etc. dispatch returns zero ProjectBinding +
  nil error for callers that didn't set them, which mimics today's behavior
  for non-cross tests.

- **Concern:** the planner says BindingByProjectID should return
  `f.crossBinding, f.crossBindingErr` for `ProviderCodex`. But when an existing
  test (e.g. `boundClaudeStore` returning Claude-only binding) is exercised
  through the cross-binding lookup in Unit 10.4's new `Run` flow, the codex
  cross-binding lookup will return `f.crossBinding` (zero ProjectBinding) +
  `f.crossBindingErr` (nil error). Then `s.store.ProfileByID(ctx,
  otherBinding.ProfileID)` will be called with an empty ProfileID. The existing
  `ProfileByID` returns `f.profile, f.profileErr` regardless of ID — so the
  CLAUDE profile is returned for the empty codex profile ID, and
  `otherProfileHome` is set to the claude profile's HomePath.
  **This silently corrupts existing tests** unless `boundClaudeStore` sets
  `crossBindingErr: domain.ErrNotFound` explicitly to opt out of cross-mount.

- **Counterexample test case:** `TestRunSucceedsWithBoundProject` (existing,
  claude-only). After Unit 10.4 lands: the cross-binding lookup returns
  zero-value `crossBinding` with nil error → ProfileByID returns the claude
  profile → cross-mount is added with claude profile home mounted at
  `/home/valv/.codex`. Existing test assertions that check mount count or
  expected env may fail.

**Verdict:** **ADVISORY → near-CONFIRMED.** The planner should add an explicit
acceptance sub-bullet to Unit 10.4 (and mirror in 10.5):

> `boundClaudeStore` (and any similar helper) must initialise
> `crossBindingErr: domain.ErrNotFound` to opt out of cross-mount. Equivalently:
> the fakeStore's `BindingByProjectID` should return `domain.ErrNotFound` as the
> default when `crossBinding` is zero-value AND `crossBindingErr` is nil — so
> existing tests stay correct without modification.

This is an AC tightening; if the builder catches it via test failure during
Phase 4, no harm done. But the PLAN.md should call it out to prevent the
builder from shipping a green Unit 10.4 that silently introduces extra mounts
in cross-provider-irrelevant tests. **PROMOTED to advisory — recommend
planner add to Unit 10.4/10.5 acceptance.**

### 3.2 Sort-order assumption brittleness across BuildArgs additions

**Attack:** R2's args-comparison guidance pins the alphabetic order to
`CLAUDE_VERSION` < `CODEX_VERSION` < `VALV_GID` < `VALV_UID`. If a future drop
adds a fifth build-arg (e.g. `BASE_IMAGE`), the sort order shifts — tests
break.

- This is YAGNI pressure but on the OPPOSITE side: the plan is fine for THIS
  drop. Future drops will need to re-evaluate sort order anyway. ADVISORY,
  not blocking.

### 3.3 OtherProviderProfileHome shape — string vs Profile

**Attack:** Both Unit 10.2 and 10.3 add `OtherProviderProfileHome string` to
`PrepareRequest`. Why string and not `*domain.Profile`?

- The runtime adapter package (`providers/claude`, `providers/codex`) is
  intentionally domain-free — it imports `dockeradapter`, `pathutil`, stdlib.
  Importing `domain` here would invert the layering (adapters → domain is OK,
  but adapters typically don't deal with full domain types in `PrepareRequest`,
  only primitive paths).
- The string-typed field is consistent with the existing `ProfileHome string`
  on the same struct. Symmetric. Correct.
- REFUTED.

### 3.4 Sync-back gap for cross-mount

**Attack:** The cross-mount is described as raw (no staging, no sync-back).
But the PRIMARY profile-home flow in codex/runtime.go lines 89–103 stages a
copy and syncs back `auth.json` + `config.toml`. The cross-mount has no
staging. Could the OTHER CLI inside the container write session state that
gets lost?

- Per planner Notes (lines 369–373) the OTHER CLI's native auth file is the
  full credentials. For claude in a codex container: claude writes
  `.credentials.json` directly to the mounted dir (which IS the host profile
  home). Writes persist to the host. Codex in a claude container: codex writes
  `auth.json` directly. Persists. No staging needed.
- The asymmetry is intentional and documented in Unit 10.2 prose lines 167–172.
- REFUTED.

---

## 4. Section 0 falsification certificate

**Premises:** R1 produced 4 findings (F1–F4), dev accepted all 4, planner
applied them in R2 commit `8967680`. R1 also surfaced advisories V4 / V5 / V11
that may or may not be addressed.

**Evidence:** Git log shows R2 commit modified 33 lines (23 insertions, 10
deletions) of PLAN.md only. Source inspection of
`internal/adapters/providers/codex/runtime.go`,
`internal/adapters/providers/claude/runtime.go`,
`internal/services/claude/service.go`,
`internal/services/claude/service_test.go`,
`internal/services/images/service.go`,
`internal/services/images/service_test.go`,
`internal/adapters/docker/ops.go`,
`internal/domain/repository.go`. Hylla searches for `BindingByProjectID`,
`BindingRepository`, `DefaultClaudeDockerfile`. `rtk grep` for `BuildImageArgs`,
fakeStore fields, sort order.

**Trace or cases:**

- 1.1 BuildArgs sort order — REFUTED.
- 1.2 Unit 10.3 insertion site — REFUTED.
- 1.3 Recipe-hash auto-update — REFUTED.
- 1.4 Unit 10.2 vs 10.3 symmetry — REFUTED.
- 1.5 New R2 drift (mild redundancy) — ADVISORY.
- 2.1 V4 latest-version reproducibility — ADVISORY.
- 2.2 V5 fakeStore atomic dispatch — ADVISORY.
- 2.3 V11 post-upgrade UX — ADVISORY.
- 3.1 fakeStore zero-value crossBinding silently corrupts existing tests —
  **PROMOTED ADVISORY** (recommend AC tightening, not blocking).
- 3.2 Sort-order future brittleness — ADVISORY.
- 3.3 string vs Profile typing — REFUTED.
- 3.4 Cross-mount sync-back gap — REFUTED.

**Conclusion:** **PASS.** No CONFIRMED counterexample. R2 cleanly applied
F1–F4. The three R1 residual advisories (V4 / V5 / V11) are non-blocking
trade-offs consistent with dogfood-stage scope. New attack 3.1 surfaces a
test-correctness concern in fakeStore zero-value handling — planner should add
an acceptance sub-bullet to Unit 10.4 / 10.5 to prevent silent test corruption,
but this can be caught at build-QA time without re-planning.

**Unknowns:**
- Whether `boundClaudeStore` (and similar pre-wired stores) will break under
  Unit 10.4's new cross-binding lookup if `crossBindingErr` is not explicitly
  set to `domain.ErrNotFound`. Recommend planner add the AC sub-bullet OR
  builder handles via test-failure feedback during Phase 4. Routed to orch.
- Whether dev wants V4 (cross-version pinning) added as a follow-up TODO in
  PLAN.md Notes for DROP_11. Routed to orch.

---

## 5. Recommendations to orchestrator

1. **Promote attack 3.1 to dev review.** Recommend the planner add to Unit
   10.4 acceptance: "`boundClaudeStore` (and any pre-wired claude-only test
   fixture) initialises `crossBindingErr: domain.ErrNotFound` to opt out of
   cross-mount; tests that DO want cross-mount set `crossBinding`,
   `crossProfile`, and clear `crossBindingErr` explicitly." Mirror in Unit
   10.5 for codex-only fixtures.

2. **Defer V4 (cross-version pinning) to DROP_11.** Add a Notes line in PLAN.md
   pointing at DROP_11_E2E_AND_RELEASE: "pin cross-provider versions via
   resolver pattern, mirror `CodexVersionResolver` / `claudeVersionResolver`
   for the cross-version build-arg."

3. **No action needed on V5** — the single-unit-commit boundary already
   prevents the mid-commit failure mode.

4. **No action needed on V11** — graceful degradation to pre-DROP_10 behavior
   until `mage image update` runs.

5. **R2 plan is otherwise good.** PASS verdict stands once attack 3.1 is
   acknowledged (either via AC tightening OR explicit dev waiver).
