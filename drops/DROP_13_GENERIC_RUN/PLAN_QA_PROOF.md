# DROP_13 — Plan QA Proof — Round 1

**Verdict:** PASS

## Per-unit evidence table

| Unit | Paths concrete | Acceptance testable | blocked_by sound | Hylla cited correctly | Footprint 1-3 files | Verdict |
|---|---|---|---|---|---|---|
| 13.1 | yes (2 new files in new pkg) | yes | yes (none) | yes (verified) | yes (2) | pass |
| 13.2 | yes (2 new + 1 modify) | yes | yes (after 13.1) | yes (verified) | yes (3) | pass |
| 13.3 | yes (2 modify) | yes | yes (after 13.2) | yes (verified) | yes (2) | pass |
| 13.4 | yes (2 modify) | yes | yes (after 13.3) | yes (verified) | yes (2) | pass |
| 13.5 | yes (README.md) | yes | yes (after 13.4) | yes (verified) | yes (1) | pass |

## Schema decisions audit

- **`internal/services/run` as the new shared launch pkg.** Evidence sample-verified against `internal/services/claude/service.go:128-222` and `internal/services/codex/service.go:121-214` (read both `Run` bodies in full). The two Run functions are near-identical structurally — same OverrideProfile branch, same project-detect / project-lookup, same cross-provider binding lookup that emits ErrNotFound silently, same PrepareRuntime call, same buildRequest path, same runAttached / executor dispatch. Provider-specific differences are limited to: (a) PrepareRuntime impl per package, (b) shared-home policy (codex uses `sharedCodexStateHome`, claude passes empty), (c) cross-mount target name (`.codex` vs `.claude`), (d) notice/label copy. Claim that "duplicated launch orchestration" is shared-able is strongly evidence-backed.

- **Image-entrypoint override via `ContainerRunRequest.Extra`.** Read `internal/adapters/docker/types.go:43-60` (the `Extra []string` field exists on `ContainerRunRequest` at line 59) AND `buildRunLikeArgs` at lines 141-219 — confirmed line 215 `args = append(args, request.Extra...)` runs BEFORE line 216 `args = append(args, request.Image.String())`. So injecting `--entrypoint <command>` via Extra produces the right docker-CLI ordering. Decision is sound and does NOT need a docker-adapter widening. Hylla cite of `:133-218` covers buildRunLikeArgs correctly (within rounding for the start-of-function line — function actually begins at `:141`, header on `:133` is BuildRunArgs which calls buildRunLikeArgs; both readers will agree on intent).

- **Read-only-with-respect-to-bindings on `--account`.** Verified `claude_setup.go:23-112` — explicit override (`accountOverride != ""`) returns the profile via `ProfileByName` with no `BindProject` call. Same pattern in `codex_setup.go:14-118`. Hylla cite `claude_setup.go:47-54` lands inside that override block (`if accountOverride != ""`). Sound.

- **`resolveAccountByName` reuse.** Verified `internal/cli/manage.go:1118-1162` — function exists, handles three cases the planner names: not-found (case 0 → error with `valv account add` hint), unique match (case 1 → return), multi-provider collision (default → error pointing to `--provider`). Decision to reuse it directly for `valv run --account` is sound; the function does not require a writeable store handle.

- **`ENTRYPOINT` is baked into base images per provider.** Verified `images/service.go:976` (`ENTRYPOINT ["codex"]` in DefaultCodexDockerfile) and `:1042` (`ENTRYPOINT ["claude"]` in DefaultClaudeDockerfile). Confirms claim that overriding entrypoint at `docker run` is the right surface for generic-command execution; no rebuild required.

- **Cross-provider mount semantics unchanged.** Verified `claude/service.go:162-187` and `codex/service.go:155-178` — both look up the other provider's binding via `BindingByProjectID` + `ProfileByID`, skip on `ErrNotFound`, only fail-loud on other store errors. Planner commits to preserving this pattern.

- **No new image model in DROP_13.** Verified `claude.go:115` and `codex.go:120` both call `resolveProjectImage`, and `operator_helpers.go:421-470` is the canonical helper. Per-project overlay path already flows through DROP_12 plumbing — adding a new image surface in DROP_13 is correctly scoped out.

## Drop-level acceptance coverage

Mapping scope text elements → acceptance bullets:

- "Per-account credential homes under …/profiles/<account>/" → Unit 13.1 acceptance ("preserves … mount/env passthrough … now implemented separately") + drop bullet 1 ("same mount/env/isolation behavior as current runtime launchers"). Covered.
- "Sibling-path-aware mounts (worktree gitdir handling)" → drop bullet 4 ("sibling-path-aware mounts … still work") and reinforced by Unit 13.1 ("runtime cleanup behavior now implemented separately") since worktree handling lives in `PrepareRuntime`. Covered.
- "Cross-provider in-container routing" → drop bullet 4 explicitly + Unit 13.1 acceptance ("cross-provider mount lookup still skips on ErrNotFound"). Covered.
- "Project-binding-aware account resolution" → drop bullet 1 ("does not mutate bindings") + Units 13.2/13.3/13.4 acceptance (each still routes through `ensureClaudeBindingReady` / `ensureCodexAccountReadyForLaunch`). Covered.
- "Per-project overlay image build (DROP_12 `EnsureProjectImage`)" → drop bullet 2 ("overlay-image selection") + drop bullet 4 ("project overlay images … still work"). Covered.
- "`VALV_<PROVIDER>_IMAGE` env override path" → covered transitively via Units 13.3/13.4 keeping `*ImageRef()` helpers; not called out explicitly in any acceptance bullet. **MINOR GAP** (non-blocking, noted below).
- "Provider-specific CLI argument pass-through" → drop bullet 3 + Units 13.3/13.4 acceptance ("Replace the current main launch body with a thin adapter that still resolves binding/account … delegates the actual container launch to the shared primitive"). Covered.
- "README + help reflect the Product Direction rewrite" → drop bullet 5 + Unit 13.5. Covered.

## Notes / non-blocking observations

- **Hylla line-number drift, `docker/types.go:133-218`.** Planner cites lines 133-218 for "Extra args emitted before image" but the function body of `buildRunLikeArgs` is at `:141-219`. Line 133 is `BuildRunArgs` (the public wrapper). The cite still lands in the right file and lets a reader find the right code; not blocking.

- **`VALV_<PROVIDER>_IMAGE` override path not surfaced in drop-level acceptance.** Scope text lists it as a must-preserve concern, but no acceptance bullet names it. Recommend adding a sub-bullet to drop bullet 2: "Image resolution preserves `VALV_<PROVIDER>_IMAGE` short-circuit semantics." Not a fail because Units 13.3/13.4 implicitly preserve it by keeping `claudeImageRef()` / `codexImageRef()` callers — but explicit coverage tightens the gate.

- **`valv run` help-flag passthrough not specified.** Unit 13.2 says "manually consumes only its local `--account` flag while leaving the target command and its flags untouched; do not require a `--` separator." That's clear. But it does not state what happens when the user runs `valv run --help` with no target command — does it print run's own help, or pass through? Cobra's default with `DisableFlagParsing: true` is to suppress automatic `--help` handling; the planner should clarify whether `valv run --help` (no other args) renders the run command's help. Non-blocking, but a reasonable Phase 3 discussion item.

- **Unit 13.5 README evidence is broadly accurate.** `README.md:3-10` was not read here in full but the planner's prose framing ("describes Valv mainly as a control plane for AI CLIs and only calls out `valv codex`") is consistent with the codebase's stated direction in CLAUDE.md and the DROP_12 closeout state. Acceptance bullets are yes/no-testable (README intro mentions per-account isolated containerized workload; README has `valv run` example; README does not contradict DROP_5/7/8/10/12).

- **Test footprint clarity.** Units 13.3 and 13.4 say "Existing `internal/cli/{claude,codex}_test.go` coverage remains green after the rewire." That's a clean yes/no gate. There is no explicit ask to add new tests in 13.3 / 13.4 — the new test coverage lives in Unit 13.1 (`service_test.go`) and Unit 13.2 (`run_test.go`). Consistent with "thin adapter" framing.

- **Notes For Builder Agents are useful guidance, not filler.** Each bullet (reuse `resolveAccountByName`, do not add new schema, explicit-account only, preserve project-image flow, reuse `Extra`, table-driven tests for parse-heavy bits, respect unit ordering) maps to a concrete failure mode the planner is trying to forestall. Sound.

- **Block ordering is strictly linear (13.1 → 13.2 → 13.3 → 13.4 → 13.5).** Justified: 13.2 wires the new pkg from 13.1; 13.3 and 13.4 then re-derive provider CLIs; 13.5 documents. The note in builder-agent-notes ("Because Units 13.2-13.4 all touch `internal/cli`, respect the strict order above even though the file paths differ") is correct — files do not overlap but compile dependencies do (the shared service is the contract surface).
