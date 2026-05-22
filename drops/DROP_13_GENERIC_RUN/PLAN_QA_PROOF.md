# DROP_13 — Plan QA Proof — Round 2

**Verdict:** PASS

**Reviewer:** ta-go-qa-proof (orchestrator-direct write — Agent reported Write tool not in its function list; orchestrator persisted the returned content)
**Plan file:** main/drops/DROP_13_GENERIC_RUN/PLAN.md @ 837ad1b
**Hylla artifact_ref:** github.com/evanmschultz/valv@main (1759e64)

## Round 1 blocker resolution audit

### A1 services-retirement — Resolved

Round 1 falsification flagged that the original plan let `internal/services/claude/service.go` and `internal/services/codex/service.go` keep their full launch orchestration alongside the new `internal/services/run` primitive. Round 2 resolves this in three explicit places:

- Schema Decisions, line 36-37: "Use a new package, `internal/services/run`, as the shared launch primitive, and slim `internal/services/claude` / `internal/services/codex` to thin wrappers over it while preserving each package's public `Service.Run` API."
- Unit 13.3 acceptance: "Reduce `internal/services/claude/service.go` to a thin wrapper around `internal/services/run` while preserving the existing public `claudeservice.Service.Run(ctx, cwd, args)` API."
- Unit 13.4 acceptance: mirror language for codex.

Both source bodies verified at `claude/service.go:128-222` and `codex/service.go:121-214` — same shape today, ripe for shared extraction.

### A6 no-project semantics — Resolved

Round 1 flagged that `valv run --account <name>` in an unbound project had ambiguous semantics. Round 2 picks "error with valv account bind guidance" explicitly:

- Acceptance criteria: "If `valv run --account <name>` is invoked from a directory whose detected project has no persisted Valv project row, the command errors with a clear message suggesting `valv account bind <name>`. It does **not** auto-create or auto-bind."
- Unit 13.2 acceptance: "When the resolved launch path reports `domain.ErrUnboundProject` ... the command returns a clear user-facing error suggesting `valv account bind <name>`."

Service-level conversion already exists: `claude/service.go:144-145` and `codex/service.go:137-138` wrap `ProjectByRoot → ErrNotFound` into `domain.ErrUnboundProject` on the explicit-override path.

### A7 `--provider` flag — Resolved

Round 1 flagged cross-provider account collision. Round 2 adds the flag in five places:

- Objective signature: `valv run --account <name> [--provider <provider>] <command>`.
- Schema Decisions: "Reuse `resolveAccountByName` for cross-provider account resolution and `--provider` semantics."
- Acceptance criteria: tests cover `valv run --account work --provider claude` resolving correctly when `work` exists in both providers.
- Unit 13.2 acceptance: manually consumes both flags, no `--` separator required.
- Notes for Builder Agents: semantics aligned with existing `account switch --provider`.

Existing helper at `manage.go:1118-1162` already covers all four cases (explicit provider, single match, no match, multi-provider collision).

## New acceptance + nits verification

### `valv run --help` semantics

Unit 13.2 acceptance: "`valv run --help` prints `valv run`'s own help when no target command remains after local-flag stripping, while `valv run <command> --help` passes `--help` through to the target command." Builds on verified `stripAccountFlag` stop-at-`--` scan (`account_flag.go:3-15`) and existing `DisableFlagParsing: true` pattern (claude/codex commands).

### `VALV_<PROVIDER>_IMAGE` short-circuit

Three-way closure:
- Drop acceptance: "the override still short-circuits overlay-image building."
- Schema Decisions: cites existing `claudeImageRef` / `codexImageRef` / `resolveProjectImage` path; verified at `operator_helpers.go:449-453` returns `baseRef` unchanged when env override is set.
- Notes for Builder Agents reinforces.

### Unit count = 5

Verified: 13.1 → 13.2 → 13.3 → 13.4 → 13.5. Linear chain. No new units, no merged units.

## Standard plan-QA checks

### Paths concrete

All five units list explicit file paths. `internal/services/run` package verified absent today (`ls internal/services/` shows claude, cleanup, codex, globalswitch, images, manage). Footprint sizes 2/4/4/4/1 — three 4-file units justified as paired CLI + service changes that must land atomically.

### Acceptance testable

Every unit lists test assertions as concrete behaviors. Unit 13.2 has an 8-item test list — every item is constructible.

### blocked_by sound

Linear chain 13.1 → 13.2 → 13.3 → 13.4 → 13.5. Justified at Notes line 170 because 13.2-13.4 all touch `internal/cli/root.go` overlap.

### Hylla citations accurate

14 distinct spot-checks executed; zero drift detected. Key verifications:
- `claude/service.go:128-222` and `codex/service.go:121-214` — `Service.Run` spans verified.
- `manage.go:1118-1162` — `resolveAccountByName` verified.
- `account_flag.go:3-15` — `stripAccountFlag` doc + signature verified.
- `images/service.go:976` / `1042` — `ENTRYPOINT ["codex"]` / `["claude"]` verified.
- `docker/types.go:43-60` and `141-218` — `ContainerRunRequest.Extra` field + buildRunLikeArgs emit-before-image verified.
- `operator_helpers.go:429-452` — VALV_*_IMAGE short-circuit verified.
- `root.go:120-139` — runtime command registration pattern verified.
- `claude.go:46-63` and `codex.go:51-68` — `DisableFlagParsing: true` pattern verified.
- `claude/service.go:128-148` and `codex/service.go:121-141` — `ErrUnboundProject` conversion verified.
- `CLAUDE.md:9-16` — product-direction language verified.

### Schema Decisions sound

Six decisions, each with verified evidence cite. None invent new abstractions: every decision points to existing helpers, fields, or semantics. Smallest-concrete-design respected.

## Verdict

**PASS.** All three Round 1 falsification blockers (A1, A6, A7) resolved with concrete acceptance language and verified evidence cites. Both non-blocking nits (`valv run --help` semantics, `VALV_<PROVIDER>_IMAGE` short-circuit) incorporated. Unit count stays at 5. Footprints reasonable. Hylla citations accurate across 14 spot-checks. Shared-service seam (13.1) is sound; CLI seam (13.2) reuses existing helpers; adapter retirements (13.3/13.4) explicitly slim existing services to thin wrappers. Ready for build phase pending falsification clearance.

## Unknowns (routed, out of plan)

- Test-double policy for new `internal/services/run` package — gated by `mage test` coverage at build-QA.
- Exact prose template for no-project error wording — builder-time call; acceptance pins content not wording.
