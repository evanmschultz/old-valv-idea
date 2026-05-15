# DROP_7_HOST_AUTH_TOKEN_FIX — Builder Worklog

Append a `## Unit 7.M — Round K` section per build attempt. See `main/drops/WORKFLOW.md` § "Phase 4 — Build (per unit)" for what each section should contain.

<!-- units filled in by planner, then by builder during Phase 4 -->

## Unit 7.2 — Round 1

**Date:** 2026-05-15
**State at start:** todo → in_progress → done

### Files touched

- `internal/services/claude/service.go` — added `encoding/json` import; added `readClaudeAuthToken` helper (~25 LOC); added token-injection block in `buildRequest` (~8 LOC).
- `internal/services/claude/service_test.go` — added `os` + `path/filepath` imports; added `TestRunSetsClaudeCodeOAuthTokenWhenCredentialsPresent` test case (~35 LOC).

### Mage targets run and result

- `mage testPkg ./internal/services/claude` — RED (1 failure, 17 pass) after test written, before implementation.
- `mage testPkg ./internal/services/claude` — GREEN (18/18 pass, 80.8% coverage) after implementation.

### Design notes

**JSON schema used:** `{"claudeAiAccessToken": "<token>"}` — single-key JSON. Key name `claudeAiAccessToken` is confirmed by PLAN.md (Design decisions locked by planner, line 57: `"The service reads this key"`; also lines 22, 154-155 of Unit 7.2 spec). Unit 7.1 BUILDER_WORKLOG had no entry at time of authoring this unit — schema chosen per PLAN.md as authoritative source.

**Cross-reference with Unit 7.1:** Unit 7.1 writes `.credentials.json` to `<managed-home>/.credentials.json` with key `claudeAiAccessToken`. Unit 7.2 reads that same key. If 7.1 deviates from the planner's locked schema, Unit 7.3 will surface the mismatch during its integration test phase.

**Graceful-skip semantics:** PLAN.md AC4 specifies "Run succeeds" on bad JSON (graceful skip). The `buildRequest` injection block logs debug on read/parse error and omits the env var rather than propagating the error from `Run`. This differs from the spawn-prompt description ("return the error wrapped in Run") — PLAN.md is the ground truth and was followed.

**Env map safety:** `prepared.Env` is initialized by `clauderuntime.PrepareRuntime` which already sets `CLAUDE_CONFIG_DIR`. Assigning `request.Env["CLAUDE_CODE_OAUTH_TOKEN"] = token` is safe — `request.Env` is the same map reference; no nil-map risk because `PrepareRuntime` already writes to it.

**No token logging:** The token value is never logged. Debug messages record "token present" (via `else if token != ""` branch executing with no explicit log) or "no claude credentials file found, container will run unauthed" (explicit debug log on empty-token path). The error path logs `"claude auth token unreadable"` with the error but not the token.

**`ps(1)` visibility comment:** placed inline at the injection site per PLAN.md instruction.

### Unknowns

- Unit 7.1 schema coordination: handled by using PLAN.md's locked schema. If 7.1 diverges, Unit 7.3's service_test.go additions will catch it.
- No other unknowns.

## Hylla Feedback

N/A — task touched non-Go files only in terms of Hylla indexing scope. The Go files edited were read directly via the `Read` tool; Hylla was not needed for committed state navigation on this small in-package change.
