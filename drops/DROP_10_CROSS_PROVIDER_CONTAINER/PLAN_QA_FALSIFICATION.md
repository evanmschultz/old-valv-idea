# DROP_10 — Plan QA Falsification

**Round:** 1
**Verdict:** PASS (no CONFIRMED counterexample; 4 advisory findings recorded)

## Method

For each of the twelve attack vectors enumerated in the orchestrator spawn brief, I attempted to construct a counterexample by reading the actual touched files: `internal/services/images/service.go`, `internal/services/images/service_test.go`, `internal/adapters/providers/claude/runtime.go`, `internal/adapters/providers/claude/runtime_test.go`, `internal/adapters/providers/codex/runtime.go`, `internal/adapters/providers/codex/runtime_test.go`, `internal/services/claude/service.go`, `internal/services/claude/service_test.go`, `internal/services/codex/service.go`. Cross-referenced call graph via Hylla `refs_find` on both `PrepareRuntime` symbols. The plan claims `~300–500 LOC including tests` over five units (10.1–10.5). I treat each unit as a separate attack target.

Findings are classified `CONFIRMED` (concrete repro available), `REFUTED` (attack landed on a contradiction the plan already handles), or `ADVISORY` (no test-bench failure but plan-language sharpening would reduce future-round risk).

## Per-vector findings

### V1 — `OtherProviderProfileHome` empty-string semantics — REFUTED

`Profile.HomePath` empty would only arise if a profile row in the SQLite store had `home_path = ''`. The store's profile schema treats `HomePath` as a required field at creation (`internal/services/manage/service.go` is the only producer that calls `CreateProfile`, and DROP_2/DROP_3 work made this non-empty). Furthermore the plan's reasoning matches Option A semantics — empty string means "no cross-mount" — which is the same fallback the plan applies when the binding lookup hits `ErrNotFound`. An empty `HomePath` returned by the store is a pre-existing data-corruption invariant, not a new gap introduced by DROP_10. Plan's `strings.TrimSpace(request.OtherProviderProfileHome) != ""` predicate handles this correctly.

### V2 — `PrepareRequest` default-field semantics for existing callers — REFUTED

Hylla `refs_find` on `claude.PrepareRuntime` and `codex.PrepareRuntime` shows the **only** production caller of each is the corresponding `Service.Run` (`internal/services/claude/service.go:165` and `internal/services/codex/service.go:156`). All other callers are tests in the same `runtime_test.go` files. There is no auth-runner indirection that builds a `PrepareRequest{}` literal — `internal/cli/account_auth_test.go` does not call `PrepareRuntime`; it calls a separate auth-runner type whose surface area is unaffected by DROP_10. The new field defaults to zero-value empty string for any caller that does not set it. Plan is correct that this is non-breaking.

### V3 — `Build()` arg ordering vs `reflect.DeepEqual` tests — ADVISORY

The plan says (10.1): `providerVersionBuildArg()` first, then the cross arg. **The existing tests' expected slices are alphabetically/insertion-order-stable** because `docker.BuildImageArgs` consumes a `map[string]string` and emits args in **sorted-key** order — verifiable by inspecting the existing expected arrays in `TestServiceBuildAddsVersionAndUsesDefaultImageInfo` (line 110), which puts `--build-arg CODEX_VERSION=...` before `VALV_GID` and `VALV_UID` (`C` < `V`). After DROP_10, the codex case will have both `CLAUDE_VERSION` and `CODEX_VERSION` keys in the map; sorted order will then be `CLAUDE_VERSION` → `CODEX_VERSION` → `VALV_GID` → `VALV_UID`. The plan's "providerVersionBuildArg() first" language is wrong for the codex case — for a codex image, `CLAUDE_VERSION` will appear *before* `CODEX_VERSION` because of sorted-key emission, not after.

**Why this is ADVISORY not CONFIRMED**: the builder updating the tests will discover this empirically by running `mage testPkg` and observing the failing assertion. Test fixtures are deterministic data. The plan's *implementation* (just add the cross arg to the BuildArgs map) is correct; only the plan *prose* describing ordering is misleading. The builder must verify the actual sorted-key emission order rather than relying on plan prose.

Suggested plan-prose fix: replace "the order is providerVersionBuildArg() first, then the cross-provider arg" with "both build-args appear in sorted-key order: `CLAUDE_VERSION` before `CODEX_VERSION`, both before `VALV_GID`/`VALV_UID`".

### V4 — `CrossProviderVersion` default `"latest"` reproducibility — ADVISORY

Plan acceptance is "valid npm syntax, not reproducible but acceptable for dogfood stage." This is a defensible call but means:
1. Two consecutive `mage image update` invocations against the codex image will silently produce different baked-in claude CLI versions if the upstream `latest` tag advances between calls.
2. The recipe-hash will *not* change between those two builds (the Dockerfile string is identical — `"latest"` is in the build-arg, not the dockerfile). So `EnsureLatest`'s recipe-hash cache check will treat the image as up-to-date even though the upstream cross-CLI moved.

**Why this is ADVISORY not CONFIRMED**: the dev consciously chose Option A semantics at dogfood stage. The asymmetry is real but not breakage. Plan should either (a) note in §Notes that the cross-CLI version is not pinned and floats with `npm latest` until a future drop wires a second `VersionResolver`, or (b) pin `CrossProviderVersion` from the existing resolver of the cross provider (smaller diff than introducing a separate resolver — the existing `NewClaudeVersionResolver` / `NewCodexVersionResolver` already exist and are reused-friendly).

Suggested plan addition under "Open questions resolved": explicit acknowledgement that recipe-hash cache will not invalidate on upstream cross-CLI bumps with `"latest"`, and a forward-pointer ("DROP_11 may pin via the cross-provider's resolver").

### V5 — `fakeStore` cross-binding dispatch feasibility — REFUTED

Read `internal/services/claude/service_test.go:71`:
```go
func (f fakeStore) BindingByProjectID(context.Context, string, domain.Provider) (domain.ProjectBinding, error) {
    return f.binding, f.bindingErr
}
```
The signature already takes a `domain.Provider` argument — but the current implementation ignores it. The plan's proposed extension (dispatch on the `provider` arg, return `crossBinding`/`crossBindingErr` when it's `ProviderCodex`) is mechanical and doesn't break existing tests because they only ever exercise the primary-provider path (`ProviderClaude` for claude tests, `ProviderCodex` for codex tests). The existing `boundClaudeStore` helper (line 136) returns a `fakeStore` with only the primary fields set; the cross fields would default to `crossBindingErr = nil` and `crossBinding = domain.ProjectBinding{}` (zero ProjectID). The plan must also handle "zero-value binding" (empty ProfileID) — the proposed implementation falls through to `ProfileByID` with an empty ID. Today's `fakeStore.ProfileByID` returns `f.profile, f.profileErr` ignoring the ID, so the cross path would unintentionally re-use the primary profile.

**However**: the plan's 10.4 already requires `ProfileByID to also dispatch on profile ID between primary and cross profile`. With that dispatch in place, an empty cross-profile-ID lookup returns the zero `domain.Profile` (HomePath = ""), which then hits the V1 guard in `PrepareRuntime` and skips the cross-mount. So existing tests (which set only primary fields) get cross-mount-absent behavior. The plan's mechanical dispatch is feasible without breaking regression tests, provided **both** `BindingByProjectID` and `ProfileByID` dispatch is implemented in lockstep. The plan does mention both, so this is REFUTED.

Risk note: a builder who implements only the `BindingByProjectID` dispatch and forgets `ProfileByID` will break every existing claude/codex service test because the cross-binding lookup will return zero-binding (no error) → `ProfileByID("")` returns the primary profile → mount with target `/home/valv/.codex` materializes in every test → `TestRunSucceedsWithBoundProject` mount-count assertions / no-cross-mount expectations break. The plan should call this out explicitly as a single-atomic-change requirement.

Suggested plan-language sharpening (10.4 & 10.5): "The `fakeStore` extension MUST land in one diff — `BindingByProjectID` dispatch and `ProfileByID` dispatch together. Splitting them across rounds breaks regression tests because a zero-value cross-binding routes through the primary `ProfileByID` and materializes an unintended cross-mount."

### V6 — Cleanup contract for cross-mount session-state writes — REFUTED

The cross-mount is the raw host profile home (e.g. `/Users/dev/.valv-managed/codex/work` mounted at `/home/valv/.codex` inside the claude container). Codex session writes (`~/.codex/sessions/...`) land directly in the host profile home — same place native codex containers write them. This is the *desired* semantic: a Claude agent running `codex exec` builds session state that the user can later inspect via native codex tooling against the same profile home. Plan's "no cleanup needed" is correct because there is no staged copy to sync back.

**Counterfactual**: if `PrepareRuntime` instead staged the cross-mount via `copyDirContents` (mirroring the primary claude home in `runtimeClaudeHome`), then sync-back would be needed and the plan would have a bug. But the plan correctly says `dockeradapter.NewMountSpec(otherHome, ..., false)` — direct mount, no staging. REFUTED.

Advisory observation: this is asymmetric with the primary mount. Primary claude profile gets `runtimeClaudeHome` (staged + sync-back; `.credentials.json` excluded from sync-back to protect host keychain extraction). Cross codex profile gets raw home (no staging, no exclusion). The asymmetry is intentional — `.credentials.json` is the claude-side credential file shape, not the codex-side. Codex side uses `auth.json` which doesn't need exclusion because the codex profile home is the authoritative store. Plan should mention this asymmetry once in §Notes to forestall future-reader confusion.

### V7 — Mage gate coverage — REFUTED

Per memory `feedback_interface_change_runs_full_mage_test` and `feedback_mage_integration_when_deleting_symbols`:
- Full `mage test` required when interfaces change.
- `mage integration` required when symbols deleted / argv changed.

Plan touches: `PrepareRequest` (adds field — *not* an interface; struct field addition is non-breaking for zero-value callers). `Service.Run` signature unchanged. `BuildRequest` (adds field — same non-breaking). No symbols deleted. No CLI argv changes. The only interface in the touched surface is `Store` in claude/codex service packages (`BindingRepository`, `ProfileRepository`) — *signatures unchanged*; the plan only adds dispatch logic inside `fakeStore` for tests.

**Therefore**: `mage test` is sufficient at drop end (plan correct). `mage integration` not needed for DROP_10's scope. REFUTED.

### V8 — Recipe-hash fake update propagation — REFUTED

Verified via Hylla raw node read on `github.com/evanmschultz/valv/internal/cli/fakeCodexRecipeHash`:
```go
func fakeCodexRecipeHash() string {
    sum := sha256.Sum256([]byte(imagesservice.DefaultCodexDockerfile()))
    return hex.EncodeToString(sum[:])
}
```
This is a **runtime delegation** — it hashes whatever `DefaultCodexDockerfile()` returns at test time. When DROP_10 changes that string, the fake recomputes the new hash automatically. `fakeClaudeRecipeHash` mirrors. Plan §Notes already says this; the assertion is correct.

The plan's §Notes line "Cache-busting for image rebuild" is slightly misleading where it says "Tests that hard-code `fakeClaudeRecipeHash` / `fakeCodexRecipeHash` (in `internal/cli/extended_test.go`) need constant updates to match the new hashes" — they do NOT need constant updates; they're dynamic. The mechanical churn the plan §Notes mentions is **zero** for these particular fakes. ADVISORY-grade: plan §Notes prose is internally inconsistent (Planner section says "auto-update because they call the functions dynamically — no manual constant change needed" — correct — while §Notes says they "need constant updates" — incorrect). Fix the §Notes prose.

### V9 — Order of mounts vs len(mounts) assertions — REFUTED for default test path; ADVISORY for codex package

`internal/adapters/providers/codex/runtime_test.go` has two assertions on mount count:
- Line 81: `if len(prepared.Mounts) != 3` in `TestPrepareRuntimeNormalizesEnvAndTranslatesConfig`.
- Line 217: `if len(prepared.Mounts) != 1` in `TestPrepareRuntimePassesThroughRemoteMCPHeaderEnvWithoutOverlay`.

Both tests **do not set `OtherProviderProfileHome`**. Under the plan's `strings.TrimSpace(...) != ""` guard, the cross-mount is skipped → count stays 3 / 1 respectively. **No regression**.

`internal/services/claude/service_test.go` `TestRunSucceedsWithBoundProject` iterates mounts looking for `ContainerClaudeDir` target (line 247-255) — does NOT assert count. `TestRunUsesOverrideProfileHomePath` iterates looking for the override source (line 642-649) — does NOT assert count. Existing claude service tests have no mount-count assertions to update.

REFUTED at the regression-risk level. ADVISORY: the plan's 10.4 row "codex bound" must assert a mount with target `/home/valv/.codex` AND verify the **primary** claude mount (`/home/valv/.claude`) is still present — i.e. four mounts in the request total (project root, claude home, codex home + whatever else `prepared.Mounts` includes). The plan's stated assertion shape ("mount with target `/home/valv/.codex` and `CODEX_HOME` in Env") covers the cross side but is silent on primary-not-displaced. Tighten plan prose to "primary mount present AND cross mount present" so a future builder cannot accidentally replace the primary with the cross.

### V10 — Image size + build-time risk — REFUTED

Plan's Scope already calls this out: "Image size and build time both grow ~2× (one-time, per-machine, at `mage image update`)." No `mage` target has a build-time gate. `mage integration` runs `testcontainers-go` against built images but uses cached layers when possible. No acceptance-check time bound is needed at this drop-level — Docker layer caching makes the second-CLI install incremental in steady state. REFUTED.

### V11 — Per-call vs cached image during upgrade transition — ADVISORY

Plan correctly identifies cross-call requires both CLIs in both images. But it does **not** specify upgrade behavior. Scenario: user upgrades Valv to DROP_10 release, runs `valv claude` against a project bound to both claude and codex profiles. The existing claude image (built pre-DROP_10) does NOT have `codex` installed. A Claude Code agent attempting `codex exec` will fail with `codex: command not found`. The recipe-hash change in DROP_10 *should* trigger `EnsureLatest` to rebuild — verified by `TestEnsureLatestRebuildsWhenRecipeHashDiffers` — so the next `valv claude` invocation will trigger a rebuild and end-state is correct. But the **first** `valv claude` invocation after upgrade may surprise the user with a rebuild stall.

**Why this is ADVISORY not CONFIRMED**: `EnsureLatest` is on the existing launch path; the rebuild is automatic. There is no broken state. The user surprise is a UX nit, not breakage.

Suggested plan §Notes addition: "Recipe-hash change → first `valv claude` / `valv codex` after upgrade triggers an image rebuild via `EnsureLatest`. User sees a one-time build stall. README (DROP_11) should mention this transition behavior."

### V12 — DROP_10 → DROP_11 handoff — REFUTED (with one missing item)

Plan §Notes "Per-project binding requirement" already says: "This README work could land here OR defer to DROP_11_E2E_AND_RELEASE." The "Open questions resolved" → "Integration test scope" paragraph defers the real cross-call test to DROP_11. The "README paragraph" paragraph defers README to DROP_11. Two distinct handoff items.

**Missing**: V11's upgrade-transition note (rebuild-on-first-run) is not yet routed to DROP_11. Otherwise the handoff is well-flagged.

## Cross-cutting attacks (not in spawn brief)

### CC1 — Service.Run cross-binding lookup fires BEFORE Override profile path is decided — REFUTED

Plan 10.4 says "after `resolved` is fully populated (at the end of both the `overrideProfile` branch and the `else` branch)". Confirmed correct against `internal/services/claude/service.go:128-160`: both branches converge into `resolved` at line 153/156. The cross-binding lookup keys off `resolved.project.ID`, which is populated in both branches. The override path uses a project-by-root lookup (no provider keying), so the cross-binding lookup adds one new store call to the override path — costless. REFUTED.

### CC2 — Auth-runner side effects on the cross profile — REFUTED

Auth runners (`internal/cli/account_auth*.go`) construct `valv-codex-auth-*` / `valv-claude-auth-*` containers separately from launch containers and do **not** call `PrepareRuntime`. They use their own command shape that mounts only the *primary* profile being authed. DROP_10 doesn't touch auth runners. No auth-side regression risk. REFUTED.

### CC3 — `domain.ErrNotFound` wrap shape — REFUTED

Plan 10.4/10.5: `errors.Is(err, domain.ErrNotFound)` checks. SQLite store wraps with `fmt.Errorf("...: %w", domain.ErrNotFound)` per existing pattern (verified in `internal/services/claude/service.go:144,247`). `errors.Is` walks the chain. REFUTED.

## Verdict

**PASS** — no CONFIRMED counterexample. Four ADVISORY items (V3, V4, V8, V9, V11) that sharpen plan prose without changing the implementation contract. Plan is implementable as written; advisories reduce builder churn.

## Recommended plan-prose patches (not blocking)

1. **V3**: rewrite the 10.1 "the order is `providerVersionBuildArg()` first, then the cross-provider arg" sentence to reflect sorted-key emission.
2. **V4**: add to "Open questions resolved" that `"latest"` cross-version + recipe-hash interaction is intentional dogfood-stage debt.
3. **V5**: in 10.4/10.5 mark `fakeStore` dispatch as a single-atomic-change requirement (both `BindingByProjectID` and `ProfileByID` together).
4. **V8**: fix the internal contradiction in §Notes "Cache-busting for image rebuild" — these fakes are dynamic, not hard-coded.
5. **V9**: in 10.4/10.5 acceptance rows, tighten the primary-mount-not-displaced expectation.
6. **V11**: route the rebuild-on-first-run UX note to DROP_11 §Notes.

## Hylla Feedback

None — Hylla answered every needed lookup (`refs_find` on both `PrepareRuntime` symbols, `node_full` on `fakeCodexRecipeHash`, `search_keyword` for cross-cutting helpers). The Bash `grep` permission denial pushed me to Hylla for symbol locations earlier than I would have otherwise — minor friction in this case, but Hylla's `refs_find` results were complete and correctly distinguished test vs non-test callers.
