# API Compatibility Execution Plan

## Current problems

- `valv api serve` uses stdlib `net/http`, but its runtime error translation is still Valv-internal rather than OpenAI-compatible.
- Executor failures from `codex exec --json` are flattened into `server_error`, even when Codex returned a structured `invalid_request_error` with `status`, `param`, and `code`.
- The API path currently inherits profile-level Codex defaults such as `model_reasoning_effort = "xhigh"`, which can make otherwise valid API model requests fail.
- `stream=true` currently emits a synthetic two-chunk SSE response only after `Complete()` succeeds. It is not a true event bridge from Codex execution.
- `api serve` startup/shutdown UX is still too quiet for a long-running local server. Runtime lifecycle logs are debug-only, and `Ctrl-C` exits without a final clean shutdown notice.

## Compatibility target

Based on the official OpenAI Chat Completions docs:

- normal errors should use the standard error object shape:
  - `error.type`
  - `error.message`
  - `error.param`
  - `error.code`
- streaming responses should use `chat.completion.chunk` SSE payloads and terminate with `data: [DONE]`
- request-time validation failures may still return plain JSON error bodies instead of a streamed success envelope

## Implementation plan

### 1. Preserve upstream structured errors

- Parse Codex JSON event output from failed `codex exec --json` runs.
- If Codex returned a structured API-style error payload, translate it into Valv `RequestError` instead of wrapping it as a generic Go error string.
- Preserve upstream `status`, `type`, `message`, `param`, and `code` when available.
- Keep generic `server_error` only for true Valv-side failures.

### 2. Stop leaking incompatible reasoning defaults into API requests

- For API execution, do not blindly inherit profile-level reasoning defaults.
- Resolve a request reasoning effort from:
  - explicit API request value when supported
  - otherwise the requested model's default reasoning level from the local Codex model cache
  - otherwise a safe Valv fallback
- Pass the resolved reasoning effort to Codex as an explicit per-run config override so the API path is stable and model-compatible.
- Update the manifest and request validation/docs to reflect the supported mapping.

### 3. Improve streaming behavior

- Replace the current synthetic "success-only" stream path with a Codex event aware path.
- Parse JSONL emitted by `codex exec --json`.
- Emit OpenAI-style SSE chunks for assistant message events as they arrive.
- If execution fails before any assistant chunk is emitted, return a normal JSON error with the preserved upstream error object.
- If execution fails after streaming started, terminate the SSE stream cleanly with the best compatible behavior possible.

### 4. Improve `api serve` runtime UX

- Keep bind-before-warmup ordering.
- Promote API runtime lifecycle messages from debug-only to visible server logs:
  - runtime create
  - runtime start
  - runtime reuse
  - runtime stop
  - request begin / request end / request failure
- On shutdown, show a final laslig success notice once API runtime cleanup has completed.
- Ensure terminal-close and `Ctrl-C` both go through the same clean shutdown path.

## Tests

- add service tests for structured upstream error extraction
- add service tests for reasoning-effort resolution and Codex arg/config override generation
- add handler tests for preserved `invalid_request_error` payloads from executor failures
- add streaming tests for:
  - successful SSE chunk path
  - streamed request that fails before content
- add CLI tests for shutdown success notice and visible runtime lifecycle logging

## Deferred

- account-scoped `valv api serve --account <name>` without project binding
- multi-account routing behind a single listening port
- broader Chat Completions field parity beyond the current manifest-backed subset
