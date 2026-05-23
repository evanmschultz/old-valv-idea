# DROP_13 — Plan QA Proof — Round 3

verdict: pass

## Per-claim audit

All file:line cites in the Round 3 plan resolve in the working tree
(`HEAD = cdb7cf3`, last code-affecting commit `1759e64`). Each Schema
Decision is grounded.

### Schema Decision 1 — shared `internal/services/run` package, slim provider services

- Plan claim: `internal/services/claude/service.go:128-222` is `Service.Run`. Verified — lines 123-223 in repo declare and close the function; the comment header starts at 123, the signature at 128, the closing `}` at 223 (`internal/services/claude/service.go:128-223`).
- Plan claim: `internal/services/codex/service.go:121-214` is `Service.Run`. Verified — comment header at 116-120, signature at 121, closing `}` at 215 (`internal/services/codex/service.go:121-215`).
- Plan claim: provider-specific deltas are limited to runtime prep, shared-home, label, notice copy. Verified by side-by-side read — Claude calls `clauderuntime.PrepareRuntime` with empty SharedHome and looks up `domain.ProviderCodex` for the other-provider mount; Codex calls `codexruntime.PrepareRuntime` with `sharedCodexStateHome` and looks up `domain.ProviderClaude` for the other-provider mount. Everything else (project detection, override branch, request build, attached/detached dispatch, error wrapping) is the same shape.
- Plan claim: `CLAUDE.md:9-16` states the architectural pivot. Verified — `CLAUDE.md:9` literally says "AI CLI launching … is the first-class case shipping today; a generic `valv run --account <name> <command>` primitive backs the provider-specific launchers (planned DROP_13)".

### Schema Decision 2 — do not introduce per-account image model

- Plan claim: `internal/cli/claude.go:115`, `internal/cli/codex.go:120` resolve image via `resolveProjectImage`. Verified at `internal/cli/claude.go:115` and `internal/cli/codex.go:120`, both calling `resolveProjectImage(cmd, paths, domain.ProviderXxx, workingDir, xxxImageRef())`.
- Plan claim: `internal/cli/operator_helpers.go:421-470` defines `resolveProjectImage`. Verified — comment header at 421, signature at 440, closing `}` at 470.

### Schema Decision 3 — explicit-account only, no auto-bind / no auto-create

- Plan claim: `internal/cli/claude_setup.go:23-54` treats explicit account as read-only. Verified — `ensureClaudeBindingReady` signature at line 40; the explicit-override branch at lines 48-54 calls `ProfileByName` and returns the profile with no `BindProject` call.
- Plan claim: `internal/cli/codex_setup.go:14-52` mirrors. Verified — `ensureCodexAccountReadyForLaunch` signature at line 36; explicit-override branch at lines 46-51 calls `ProfileByName` with no `BindProject` call.
- Plan claim: explicit-override launch converts missing project rows into `domain.ErrUnboundProject`. Verified at `internal/services/claude/service.go:142-148` and `internal/services/codex/service.go:135-141` — both branches wrap `domain.ErrNotFound` from `ProjectByRoot` as `domain.ErrUnboundProject`.

### Schema Decision 4 — command override via `--entrypoint`

- Plan claim: `internal/services/images/service.go:976` is `ENTRYPOINT ["codex"]`. Verified — line 976 reads exactly `ENTRYPOINT ["codex"]`.
- Plan claim: `internal/services/images/service.go:1042` is `ENTRYPOINT ["claude"]`. Verified — line 1042 reads exactly `ENTRYPOINT ["claude"]`.
- Plan claim: `internal/adapters/docker/types.go:43-60` declares `ContainerRunRequest` with `Extra`. Verified — struct at lines 43-60, `Extra []string` field at line 59.
- Plan claim: `internal/adapters/docker/types.go:141-218` shows `Extra` emitted before the image. Verified — `buildRunLikeArgs` body spans 141-219; `args = append(args, request.Extra...)` at line 215 followed by `args = append(args, request.Image.String())` at line 216 followed by `args = append(args, request.Args...)` at line 217. `--entrypoint <command[0]>` injected via `Extra` therefore lands before the image, which is exactly the docker-run flag position. Forwarded container args via `request.Args` land after the image, which matches docker-run semantics.

### Schema Decision 5 — cross-provider routing semantics

- Plan claim: `internal/services/claude/service.go:162-187` does the codex binding lookup and skips on ErrNotFound. Verified — lines 162-175 implement the lookup with `errors.Is(err, domain.ErrNotFound)` skip and fatal-on-other-error behavior; lines 180-187 pass the resulting `otherProfileHome` into `clauderuntime.PrepareRuntime`.
- Plan claim: `internal/services/codex/service.go:155-178` mirrors. Verified — lookup at 155-168 (uses `domain.ProviderClaude` and identical skip pattern), `PrepareRuntime` call at 171-178.

### Schema Decision 6 — prefix-only local-flag extraction (Round 2 N2 fix)

- Plan claim: `internal/cli/account_flag.go:3-15` shows the current scan-until-`--` behavior. Verified — comment header at 3-14, function signature at 15; the loop at 16-53 stops only on `--` sentinel, otherwise scans the whole slice.
- Plan claim: `internal/cli/account_flag_test.go:60-106` proves mid-argv extraction is the current contract. Verified — table case at 60-64 (`"--account in the middle of other args"`) shows `["resume", "--account", "work", "--last"]` extracting `work` and returning `["resume", "--last"]`; case at 102-106 (`"leading and trailing unrelated args preserved in order"`) shows the same mid-argv extraction with multiple surrounding flags.
- Plan claim: `go doc github.com/spf13/cobra.Command.DisableFlagParsing` says "all flags will be passed to the command as arguments". Verified verbatim against `go doc` output.
- Round 2 falsification N2 (the planned helper could steal target-command `--account` / `--provider`) is explicitly addressed by Decision 6's "prefix-only" rule plus the Unit 13.2 line 105 test pair `(--account A cmd --account B)` and `(cmd --account A)`. Round 3 mitigation: pass.

### Schema Decision 7 — provider-context-preserving bind suggestion (Round 2 N1 fix)

- Plan claim: `internal/cli/manage.go:343-410` is `newManageAccountBindCommand`. Verified — `cobra.Command` declaration at 346, RunE body through 410. Confirms the bind command takes either `<account>` (with optional `--provider` flag) or explicit positional provider plus account. The cross-provider collision case (where one-arg form fails until `--provider` is supplied) is implemented at 391-403 via `resolveAccountByName`, which matches the Round 2 N1 reproduction.
- Plan claim: `internal/cli/claude_setup.go:96-101` shows provider-qualified bind suggestion. Verified — lines 99-101 return the error `"project is not bound to a Claude account; run \`valv account bind <name> --provider claude\` to bind one"`.
- Plan claim: `internal/cli/codex_setup.go:90-95` mirrors. Verified — lines 93-95 return the error with `--provider codex`.
- Round 2 falsification N1 (recovery message dropped provider context for ambiguous accounts) is addressed by Decision 7 + Acceptance Criterion 3 (line 59) + Unit 13.2 line 103: "use `valv account bind <name> --provider <provider>` when `--provider` was explicitly supplied at runtime, and use `valv account bind <name>` when the provider came unambiguously from the resolved account". Round 3 mitigation: pass.

### Schema Decision 8 — preserve `VALV_<PROVIDER>_IMAGE` short-circuit

- Plan claim: `internal/cli/claude.go:111-117` resolves provider base image before launch. Verified — `ensureClaudeImageCurrent` at line 111-113, `resolveProjectImage` at 115-118.
- Plan claim: `internal/cli/codex.go:116-123` mirrors. Verified — `ensureCodexImageCurrent` at 116-118, `resolveProjectImage` at 120-123.
- Plan claim: `internal/cli/operator_helpers.go:429-452` returns baseRef unchanged when `VALV_<PROVIDER>_IMAGE` is set. Verified — comment description at 429-434, override branch at 449-453 returns `baseRef` after emitting the stderr warning.

### Schema Decision 9 — reuse `resolveAccountByName`

- Plan claim: `internal/cli/manage.go:1118-1162` covers unique match, explicit provider, not-found, multi-provider collision. Verified — signature at 1118, explicit-provider branch at 1123-1133, cross-provider loop at 1135-1163, multi-provider collision error at 1156-1162. Plan re-use is direct and avoids inventing a second helper.

## Unit-by-unit completeness check

### Unit 13.1 — shared launch service

- Paths: `internal/services/run/service.go`, `internal/services/run/service_test.go`. Verified neither exists — `internal/services/` contains `claude/`, `cleanup/`, `codex/`, `globalswitch/`, `images/`, `manage/`. "New, not yet in tree" is accurate.
- Packages: `internal/services/run`. Consistent with path.
- Evidence cites: all four resolve (see Schema Decisions 1, 4 above).
- Acceptance bullets are yes/no:
  - "introduce one shared service that accepts X" — yes/no by reading the new file's exported type.
  - "no override → preserves provider-image entrypoint" — yes/no by test.
  - "override → emits `--entrypoint <command[0]>` via `Extra` and forwards remaining tokens" — yes/no by test asserting Extra contents and Args contents.
  - "preserves within-project-root guard, labels, mount/env, cross-provider lookup, cleanup" — yes/no per existing behavior tests retained or ported.
  - "tests prove both provider descriptors work, ErrNotFound skip, no Docker type changes" — yes/no by test list + diff check.
- `blocked_by: none` — consistent (this is the foundation unit).

### Unit 13.2 — `valv run` + root wiring

- Paths: `internal/cli/run.go` (verified absent), `internal/cli/run_test.go` (verified absent), `internal/cli/root.go` (exists), `internal/cli/account_flag.go` (exists, will be updated), `internal/cli/account_flag_test.go` (exists).
- Packages: `internal/cli`. Consistent.
- Evidence cites: all eight resolve (Schema Decisions 6, 7, 9 above + `root.go:120-139` for runtime group registration verified at lines 126-129 where `codexCmd.GroupID = "runtime"` and `claudeCmd.GroupID = "runtime"` are set and at 139 where they're added).
- Acceptance bullets are yes/no:
  - "register `valv run` in root.go" — yes/no by AST or `cmd.Commands()` enumeration.
  - "prefix-only flag extraction" — yes/no by the two explicit test pairs at line 105.
  - "`valv run --help` vs `valv run <command> --help`" — yes/no by test.
  - "requires `--account`, accepts `--provider`, resolves via `resolveAccountByName`, runs `ensureManagedAccountReady`, resolves image via existing helpers, invokes shared run service with explicit command override" — yes/no by integration test or mock-based unit test.
  - "no auto-bind / no auto-create" — yes/no by absence of `BindProject` call.
  - "ErrUnboundProject path returns bind suggestion with both variants" — yes/no by two parallel test cases.
  - "tests cover the list at line 104-106" — yes/no by test count.
- `blocked_by: 13.1` — consistent (run.go imports the new services/run package).

### Unit 13.3 — Claude adapter slimming

- Paths: all four exist (`internal/cli/claude.go`, `internal/cli/claude_test.go`, `internal/services/claude/service.go`, `internal/services/claude/service_test.go`).
- Packages: `internal/cli`, `internal/services/claude`. Consistent.
- Evidence cites: all four resolve (covered above).
- Acceptance bullets are yes/no:
  - "keep `newClaudeCommand`, `claudeArgsSkipProjectBinding`, `runClaudeImageOnlyCommand`" — yes/no by symbol presence.
  - "replace launch body to delegate to shared primitive" — yes/no by diff inspection + behavior tests.
  - "reduce service to thin wrapper around `internal/services/run`" — yes/no by line count + structural inspection.
  - "preserve Claude-specific behavior: in-container auth, label/help copy, image resolution, cross-mount of Codex" — yes/no by retained test coverage.
  - "existing tests stay green" — yes/no by `mage testPkg`.
- `blocked_by: 13.2` — consistent.

### Unit 13.4 — Codex adapter slimming

- Paths: all four exist (`internal/cli/codex.go`, `internal/cli/codex_test.go`, `internal/services/codex/service.go`, `internal/services/codex/service_test.go`).
- Packages: `internal/cli`, `internal/services/codex`. Consistent.
- Evidence cites: all five resolve (covered above; `codex.go:208-218` for `codexArgsSkipAccountReady` confirmed at lines 208-218 with exact signature).
- Acceptance bullets are yes/no (mirrors 13.3).
- `blocked_by: 13.3` — consistent (both touch `internal/cli`; serial ordering is required per Notes line 177).

### Unit 13.5 — README rewrite

- Paths: `README.md`. Exists.
- Packages: none. Consistent.
- Evidence cites: `README.md:3-10` verified to describe Valv as control plane for AI CLIs only mentioning codex; `CLAUDE.md:9-20` verified to contain the updated product direction.
- Acceptance bullets are yes/no by editor inspection (intro describes containerized workload runner, examples include `valv run`, no contradictions with shipped behavior).
- `blocked_by: 13.4` — conservative serial ordering; README describes shipped behavior so it should follow the implementation. Acceptable.

## Drop-level acceptance criteria check

All eight drop-level criteria (PLAN.md lines 57-64) are yes/no-verifiable and each has at least one unit advancing it:

1. "`valv run --account <name> [--provider <provider>] <command>` resolves … without mutating bindings" → 13.1 + 13.2.
2. "`--provider` disambiguation, tests cover collision case" → 13.2.
3. "no-project-row error with bind guidance preserving provider context" → 13.2 (Schema Decision 7).
4. "resolved provider drives auth, image, overlay, cross-mount" → 13.1 + 13.2.
5. "`valv claude` / `valv codex` keep help/version + `--account`, delegate launch" → 13.3 + 13.4.
6. "DROP_10/12 behavior intact: sibling mounts, overlay images, cross-provider routing" → 13.1 + 13.3 + 13.4.
7. "`VALV_*_IMAGE` override behavior intact" → 13.3 + 13.4 (Schema Decision 8).
8. "README + CLI help reflect product direction" → 13.5.

`blocked_by: DROP_12 (done)` (PLAN.md line 4) matches `main/PLAN.md` row at line 36 — DROP_12 state is `done`.

## Round 2 falsification resolution audit

- N1 (no-project-row recovery message drops provider context for disambiguated accounts) — addressed by Schema Decision 7, Acceptance Criterion 3, and Unit 13.2 acceptance line 103. Both bind-suggestion variants are explicitly required and explicitly tested.
- N2 (extraction model can steal target-command `--account` / `--provider`) — addressed by Schema Decision 6's "prefix-only" rule, Unit 13.2 acceptance line 99, Unit 13.2 explicit test pair line 105, and Notes For Builder Agents line 171.
- N3 (DisableFlagParsing help routing) — already mitigated in Round 2; Unit 13.2 line 100 carries the help-routing requirement.
- N4 (shared-service wrapper shape) — already mitigated in Round 2; Units 13.3 and 13.4 preserve provider-specific seams (runtime prep, shared-home, notice copy) without bloating the shared service.

## Findings

No findings. The Round 3 plan resolves Round 2 falsification blockers N1 and N2 with grounded Schema Decisions, expanded acceptance criteria, and explicit tests. Every cited file:line resolves in the tree; every Schema Decision is supported by repo evidence; every unit has paths, packages, evidence, yes/no acceptance, and a consistent `blocked_by` edge; every drop-level acceptance criterion is yes/no and is advanced by at least one unit.

