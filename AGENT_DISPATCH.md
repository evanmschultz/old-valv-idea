# Agent Dispatch Model — valv/main

> valv is a **Go-only** repo using the **ta** cascade substrate (`mcp__ta__*`).
> The **canonical model** lives in `sand/main/AGENT_DISPATCH.md` — read it for the full
> dual-path + hermetic-codex + MCP-injection spec. This file is valv's project profile.

## Profile

- **7 agents** (Go-only): `ta-closeout` + 6 `ta-go-*` (planning, plan-qa-proof,
  plan-qa-falsification, builder, build-qa-proof, build-qa-falsification). No `fe-*` personas.
- **Tool matrix**: planning + plan-QA get hylla read-only + context7 + LSP(gopls) + WebSearch;
  **build-QA gets NO hylla**; builder keeps hylla. **No Playwright** (no FE surface).
- **Substrate**: ta (`mcp__ta__*`).

## Dispatch (see sand keystone §3–§4)

- OAuth claude (builder/qa-proof/closeout) → **built-in Agent tool only** (never `claude -p`).
- codex (planning/qa-falsification) → **hermetic** `codex exec`: `--ignore-user-config` +
  `-c project_doc_max_bytes=0` + `--ignore-rules` + hermetic `CODEX_HOME` + `web_search="live"`
  + role-conditional inline MCP injection (ta always; hylla-ro for planning/plan-qa; context7
  always; gopls for the Go roles). The FE/playwright injection in the shared dispatcher is
  inert here (no `*-fe-*` roles).
- Today: `bin/agent-dispatch.sh` + `.claude/agent-chains.sh`, synced from ta canonical.
  When sand is working, sand generates this as TOML (Go-only example = keystone §8b).

## Capabilities, not skills

Per the keystone §5: capabilities come from MCP injection, NOT codex skills. gopls is injected
as a `gopls mcp` server for codex roles (the `LSP` tool for claude-native). Nothing is copied
into `.agents/skills` or `.codex/`.
