verdict: pass

# DROP_13 — Plan QA Proof, Round 4

## 1. Per-R3-finding audit

All three Round 3 falsification findings addressed with multi-anchor edits.

### F1 (shared-home seam input gap) — addressed

- Schema Decision: PLAN.md reframes the seam as accepting "provider-prepared runtime state ... rather than trying to derive Codex shared-home policy itself".
- Unit 13.1 acceptance: wrappers responsible for "Codex shared-home derivation from `realHome` and the other-provider profile lookup that feeds cross-provider mounts".
- Notes For Builder Agents: "Keep provider-specific runtime prep on the provider side of the seam: Claude/Codex wrappers should hand `internal/services/run` a provider-prepared runtime value instead of teaching the shared service Codex-only `realHome` / shared-home policy."

Grounded against:
- `internal/services/codex/service.go:170-178, 228-239` (verified — Codex `sharedHome` derived from `realHome` before `PrepareRuntime`)
- `internal/services/claude/service.go:177-187` (verified — Claude `PrepareRuntime` call with `SharedHome: ""`)

### F2 (VALV_*_IMAGE override coverage on `valv run`) — addressed

- Schema Decision (VALV override): updated to name `valv run` explicitly ("by making `valv run` and the provider launchers stay on the existing `claudeImageRef` / `codexImageRef` -> `resolveProjectImage` path").
- Drop-level acceptance: adds "including through the new `valv run` entrypoint: the override still short-circuits overlay-image building instead of being swallowed by the shared-launch extraction".
- Unit 13.2 acceptance: "Tests explicitly cover the override launch path through `valv run` for both providers: with a non-empty `.valv/tools.toml` and `VALV_<PROVIDER>_IMAGE` set, launch emits exactly one override warning, makes zero overlay-image docker calls, and passes the override-derived base image into the shared run service."
- Notes: "add command-level coverage for both providers with non-empty manifests, one warning, zero overlay docker calls, and launch using the override-derived base image."

Grounded against:
- `internal/cli/operator_helpers.go:421-470` (verified — `resolveProjectImage` override short-circuit)
- `internal/cli/claude_project_image_test.go:114-154` (verified — VALV_CLAUDE_IMAGE override pin)
- `internal/cli/codex_project_image_test.go:100-137` (verified — VALV_CODEX_IMAGE override pin)

### F3 (cross-provider silent-skip on ProfileByID fail) — addressed

- Schema Decision (cross-provider routing): "mount the other provider's bound profile for the same detected project when both the binding lookup and profile lookup succeed; otherwise skip silently."
- Unit 13.3 acceptance: "Tests explicitly cover the status-quo silent-skip branch where a Codex binding exists but `ProfileByID` for that binding fails: Claude launch still succeeds, without the Codex mount or `CODEX_HOME` env."
- Unit 13.4 acceptance: mirror for Claude side ("Codex launch still succeeds, without the Claude mount or `CLAUDE_CONFIG_DIR` env").
- Notes: "Preserve current DROP_10 silent-skip semantics when the other-provider binding exists but `ProfileByID` fails: launch should continue without the cross-mount/env, and new tests in both service packages should assert that status quo."

Grounded against:
- `internal/services/claude/service.go:162-175` (verified — `otherProfileHome` only set on successful `ProfileByID`)
- `internal/services/codex/service.go:155-168` (verified — same)
- `internal/services/claude/service_test.go:652-699` (verified — `crossProfileErr` fixture available)
- `internal/services/codex/service_test.go:700-746` (verified — `crossProfileErr` fixture available)

## 2. Per-claim audit (NEW Round 4 cites)

| Plan claim | Cite | Resolved |
|---|---|---|
| Codex `sharedHome` derived from `realHome` before `PrepareRuntime` | `internal/services/codex/service.go:170-178, 228-239` | yes |
| Claude PrepareRuntime call with `SharedHome: ""` | `internal/services/claude/service.go:177-187` | yes |
| Claude only sets `otherProfileHome` on successful `ProfileByID` | `internal/services/claude/service.go:162-175` | yes |
| Codex only sets `otherProfileHome` on successful `ProfileByID` | `internal/services/codex/service.go:155-168` | yes |
| `crossProfileErr` fixtures exist in both service tests | `internal/services/claude/service_test.go:652-699`, `internal/services/codex/service_test.go:700-746` | yes |
| VALV_CLAUDE_IMAGE override short-circuit pin | `internal/cli/claude_project_image_test.go:114-154` | yes |
| VALV_CODEX_IMAGE override short-circuit pin | `internal/cli/codex_project_image_test.go:100-137` | yes |
| `resolveProjectImage` override short-circuit (1 stderr warning, zero docker) | `internal/cli/operator_helpers.go:421-470` | yes |
| `resolveAccountByName` cross-provider resolution | `internal/cli/manage.go:1118-1162` | yes |
| `account bind` accepts `<account>` plus optional `--provider` | `internal/cli/manage.go:343-410` | yes |
| `stripAccountFlag` scans until `--`, strips mid-argv | `internal/cli/account_flag.go:3-15`, `internal/cli/account_flag_test.go:60-106` | yes |
| Provider-specific non-TTY bind suggestions | `internal/cli/claude_setup.go:96-101`, `internal/cli/codex_setup.go:90-95` | yes |
| Explicit-override path converts missing project to `ErrUnboundProject` | `internal/services/claude/service.go:128-148`, `internal/services/codex/service.go:121-141` | yes |
| ENTRYPOINT `["codex"]` / `["claude"]` in base images | `internal/services/images/service.go:976, 1042` | yes |
| `ContainerRunRequest.Extra` emitted before image token | `internal/adapters/docker/types.go:43-60, 141-218` | yes |
| `DisableFlagParsing: true`, help/version fast paths | `internal/cli/claude.go:46-63`, `internal/cli/codex.go:51-68` | yes |
| Runtime command registration pattern | `internal/cli/root.go:120-139` | yes |
| Product direction text on `valv run` | `main/CLAUDE.md:9-16` | yes |
| README still describes Valv mainly as AI-CLI control plane | `main/README.md:3-10` | yes |

## 3. Unit-by-unit completeness

- **Unit 13.1 (shared run service):** new files flagged correctly (`internal/services/run/service.go`, test). Acceptance has six concrete yes/no bullets: signature, no-override entrypoint preservation, override emits `Extra=["--entrypoint", cmd[0]]`, mount/env passthrough from supplied runtime state, provider-prep ownership stays on wrappers, tests cover all of these. `blocked_by: none`.
- **Unit 13.2 (`valv run` + root wiring):** covers `internal/cli/run.go` (new), `run_test.go` (new), and three existing files (`root.go`, `account_flag.go`, `account_flag_test.go`). Acceptance has eight yes/no bullets including new R3.F2 override-coverage tests with measurable criteria (1 warning, 0 docker calls, override-derived base image flows through). `blocked_by: 13.1`.
- **Unit 13.3 (Claude thin adapter):** paths cover both CLI and service plus tests. Six bullets including new R3.F3 silent-skip test (Claude launch succeeds, no Codex mount, no `CODEX_HOME`). `blocked_by: 13.2`.
- **Unit 13.4 (Codex thin adapter):** mirror of 13.3 with R3.F3 mirror case (Codex launch succeeds, no Claude mount, no `CLAUDE_CONFIG_DIR`). `blocked_by: 13.3`.
- **Unit 13.5 (README):** `README.md` only. Three concrete content bullets. `blocked_by: 13.4`.

`blocked_by` chain linear (none→13.1→13.2→13.3→13.4→13.5), justified by Notes line "Units 13.2-13.4 all touch `internal/cli`, respect the strict order above even though the file paths differ".

## 4. Drop-level acceptance criteria check

All eight criteria concrete and yes/no-verifiable. Notable Round 4 strengthening:
- Criterion explicitly names "through the new `valv run` entrypoint" (R3.F2 mitigation).
- Cross-provider routing criterion preserves "in-container routing still work" — matches verified `otherProfileHome` happy-path behavior.

## 5. Findings

no findings.

Verdict: pass.
