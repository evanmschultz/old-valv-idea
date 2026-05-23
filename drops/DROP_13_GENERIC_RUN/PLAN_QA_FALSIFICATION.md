verdict: fail

# Plan QA Falsification — Round 3

Committed-code grounding was checked against Hylla artifact `github.com/evanmschultz/valv@main` pinned to `1759e64`. Local `git diff` for `drops/DROP_13_GENERIC_RUN/PLAN.md` was empty during this pass.

## Counterexamples

### 1. Shared-service seam drops Codex shared-home input

Scenario:
`internal/services/codex.Service.Run` currently depends on `realHome` to derive the shared host `~/.codex` state home before calling `PrepareRuntime`. The Round 3 Unit 13.1 contract says the new shared service accepts resolved profile, cwd, provider descriptor, image ref, tty/stdin/user/temp-root/logger/notices, and optional command override, but it does not include `realHome`, `sharedHome`, or a prebuilt runtime. Unit 13.4 simultaneously says the Codex wrapper should own shared-home policy only.

Repo evidence:
- `drops/DROP_13_GENERIC_RUN/PLAN.md:79-83`
- `drops/DROP_13_GENERIC_RUN/PLAN.md:139-143`
- `internal/services/codex/service.go:170-178`
- `internal/services/codex/service.go:228-239`

Why this breaks the claim:
The Unit 13.1 seam is underspecified for a behavior Unit 13.4 explicitly requires to survive. Either the shared service must accept Codex-specific shared-home inputs, or the wrapper must keep more orchestration than "thin wrapper" currently claims.

Narrow fix:
Pick one seam and state it explicitly:
- Add `realHome`/`sharedHome` (or a prebuilt prepared-runtime input) to Unit 13.1.
- Or narrow Unit 13.1 so Codex runtime prep/shared-home stays outside the shared service.

### 2. `VALV_<PROVIDER>_IMAGE` is only protected for old launcher paths, not `valv run`

Scenario:
The plan says override behavior stays intact, but the explicit protection is "keep provider launchers on the existing `claudeImageRef` / `codexImageRef` -> `resolveProjectImage` path." The new `valv run` command is a new launch path. Existing tests pin override short-circuit only at the shared helper level, not through the new command entrypoint.

Repo evidence:
- `drops/DROP_13_GENERIC_RUN/PLAN.md:52-53`
- `drops/DROP_13_GENERIC_RUN/PLAN.md:97-105`
- `internal/cli/operator_helpers.go:440-469`
- `internal/cli/codex_project_image_test.go:100-137`
- `internal/cli/claude_project_image_test.go:114-154`

Concrete counterexample:
With a non-empty `.valv/tools.toml` and `VALV_CODEX_IMAGE` or `VALV_CLAUDE_IMAGE` set, a buggy `valv run` implementation could still invoke overlay-image resolution/build and still satisfy the current Unit 13.2 tests, because those tests never assert the generic command's override path.

Narrow fix:
Add Unit 13.2 acceptance/tests for both providers covering:
- non-empty manifest
- `VALV_<PROVIDER>_IMAGE` set
- one warning
- zero overlay docker calls
- launch uses the override-derived base image

### 3. Cross-provider mount behavior is still under-specified when the other binding exists but its profile lookup fails

Scenario:
Current Claude and Codex services silently skip the cross-provider mount if the other-provider binding lookup succeeds but `ProfileByID` for that binding fails. The Round 3 plan only protects the `BindingByProjectID == ErrNotFound` skip case.

Repo evidence:
- `drops/DROP_13_GENERIC_RUN/PLAN.md:44-45`
- `drops/DROP_13_GENERIC_RUN/PLAN.md:82-83`
- `internal/services/claude/service.go:166-172`
- `internal/services/codex/service.go:159-165`
- `internal/services/claude/service_test.go:652-699`
- `internal/services/codex/service_test.go:700-746`

Concrete counterexample:
Project has both providers bound. Launch-time provider differs from the other binding. The other binding row exists, but its profile row is missing/corrupt. Current code silently launches without the cross-provider mount. The plan does not say whether the refactor must preserve that silent skip or tighten it to a hard failure, and the existing fixtures already support this case but do not test it.

Narrow fix:
Make the behavior explicit and test it:
- either preserve silent skip on other-profile lookup failure
- or intentionally make it fatal and record that DROP_10 semantic change in the plan

## YAGNI Pressure

- The new `provider descriptor` is justified only because there are already three consumers (`valv run`, Claude launcher, Codex launcher). Keep it as a small internal data struct; do not add interface layering or future-provider hooks in DROP_13.
- No extra per-account image model, config surface, or persistence was added in the plan. That part remains appropriately scoped.

## Hidden Dependency Check

- No hard dependency on DROP_14 env vars was found. The plan still routes image/env decisions through existing provider-specific launch behavior.
- No hard dependency on DROP_15 network policy was found. "Preserve current network semantics" reads as preserving today's runtime behavior, not pre-implementing closed-by-default egress.
- The shared-service seam findings above are internal DROP_13 contract gaps, not later-drop dependencies. They must be resolved in this plan before build work starts.
