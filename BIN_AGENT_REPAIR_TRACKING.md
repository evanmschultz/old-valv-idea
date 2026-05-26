# BIN AGENT REPAIR + METHODOLOGY ALIGNMENT — tracking doc (2026-05-26)

Working tracker for getting the multi-provider agent system (codex / `claude -p` / built-in) + the
cascade methodology (design-down / build-up + parallelization) **actually working and proven**, then
propagating the proven state to `valv/main`, `sand/main`, `ta/main`. Authority for the sandbox recipe =
the 4 rebuttals + `SAND_E2E_PROOF.md` + `AGENT_SANDBOX_SPEC.md`. Authority for methodology =
`CASCADE_METHODOLOGY.md` (cp'd from `tillsyn/main`) + tillsyn CLAUDE.md/personas.

## 0. Honest root-cause: why codex was flaky (NOT a recipe bug)

- The codex **sandbox recipe** (hermetic `CODEX_HOME`, execpolicy git-block, `approval_policy=never`,
  `project_doc_max_bytes=0`, `skills.bundled.enabled=false`) is **PROVEN** — `SAND_E2E_PROOF.md §2.3` +
  this session's run #2 (a full codex falsification that completed and caught a real bug).
- The **MCP-injection leg was NEVER proven**: `SAND_E2E_PROOF.md §4` explicitly flags *"role-conditional
  MCP injection ... wasn't asserted green this run"* and *"hylla's dgraph was down."*
- Observed this session (drop_012 12_4): codex build-qa-falsification — run #1 killed mid-work (120s
  default Bash timeout), run #2 completed (600s timeout), run #3 died at **startup** (no output → MCP-init
  hang → SIGTERM at timeout), run #4 ran long then died. Pattern = **MCP-init hang / slow startup**, not
  the sandbox recipe.
- Heaviest startup MCP on the role that needs it least: build-qa codex was mounting **context7** (HTTP
  remote — startup network call) + **gopls** (module index) even though build-qa is reading-based.

## 1. Fix log

| # | Change | File | Status |
|---|---|---|---|
| 1 | **ROOT-CAUSE FIX**: `startup_timeout_sec=15` on every injected MCP server (ta/hylla/gopls/context7/playwright). codex's first turn awaits ALL servers' `initialize`+`tools/list` (codex bug [#19556]/[#21318]; default 30s, hung past it on macOS `exec`) → a slow gopls-index / context7-HTTP stalled the first turn → 600s SIGTERM. Now codex drops a laggard after 15s + proceeds. | `bin/agent-dispatch.sh` `dispatch_codex` | DONE — ✅ **PROVEN 2/2** (full-MCP plan-qa-falsif, concurrent) |
| 2 | Strip context7 + gopls from **build-qa** codex (ta-only — leanest set for the reading-based axis) | `bin/agent-dispatch.sh` `dispatch_codex` | DONE — ✅ PROVEN (build-qa codex completed) |
| 3 | (matrix note) build-qa codex deviates from HYLLA_BIN §3 (no gopls/context7) — reading-based axis | HYLLA_BIN.md | TODO update §3 |
| 4 | settings.json allow-rules (mage/go doc/grep/rg/dispatcher/read-only git/gh) to stop the approval flood | `.claude/settings.json` | DONE (needs CC restart to load) |
| 6 | **Codex git-block (the codex-channel equivalent of fix #5; proven live 2026-05-26)**: codex can't use the hook (#16732), so every codex role is blocked by TWO codex-native layers — (a) `--sandbox read-only` (OS capability denial: no fs writes → commit/add/merge/reset/etc. fail; no network → push/fetch/pull fail; **invocation-agnostic**), + (b) hermetic execpolicy `prefix_rule(pattern=["git","<verb>"],forbidden)` for all git-mutation verbs (**added fetch/pull/remote/apply** to match the hook's `_GIT_MUTATION_VERBS`). Live probe: `git diff/status/log` worked; `git commit`+`git push` blocked by execpolicy; the `git -C . commit` global-flag bypass slipped past execpolicy's prefix-match but was caught by the read-only sandbox (`index.lock: Operation not permitted`); HEAD unchanged. **Codex agents = read-only git ONLY.** | `bin/agent-dispatch.sh` `dispatch_codex` | DONE — proven live |
| 5 | **Gate-hook git-mutation BASELINE**: `ta_action_gate.py` now denies git-mutation verbs (commit/push/add/fetch/pull/rebase/merge/reset/checkout/…) for EVERY scoped agent — hardcoded, **independent of the passed `bash_deny`** (so an orch that forgets git in bash_deny still can't let an agent commit). No over-restriction (read-only git + go doc + mage still allowed). Pairs with codex execpolicy (codex channel). Tested 4 cases ✓; cp'd to ta/sand/valv/tillsyn (md5). **This — not narrowing the persona `tools:` line — is the universal "agents never commit/push" lever.** ta's `tools:`-narrowing (git-read-only Bash on ta-go-planning) is over-restrictive (blocks go doc/mage) → ta should REVERT it. | `.claude/hooks/ta_action_gate.py` | DONE — tested + propagated |

## 2. Test matrix — prove EVERY channel (fill as tested)

| Channel | Role | Sandbox/gate | Result |
|---|---|---|---|
| codex-exec | build-qa-falsification (ta-only MCP) | read-only + execpolicy | ✅ **PROVEN** 2026-05-26 — full falsification completed on codex (VERDICT pass, 113K tool trace) after stripping context7+gopls. Root cause = MCP-init hang confirmed. |
| codex-exec | planning (ta+hylla-ro+gopls+context7) | read-only + execpolicy | ✅ **PROVEN** 2026-05-26 — completed, decomposed `droplet_12_5`, trace shows real `mcp: hylla.search.keyword` + `gopls.go_search` + `ta.create` (honest gopls fallback for fresh uncommitted CSS pkg). |
| codex-exec | plan-qa-falsification (ta+hylla-ro+gopls+context7) | read-only + execpolicy | ✅ **PROVEN 2/2** 2026-05-26 — TWO concurrent full-MCP runs both completed on codex (VERDICT pass + pass-with-findings) after the `startup_timeout_sec=15` fix; before the fix it hung ~50% (MCP-init blocking the first turn). |
| codex-exec | plan-qa-proof (built-in opus, NOT codex) | edit:[] gate | ✅ ran (opus) — caught a real cascade-state FAIL (12_5↔12_4 blocker-graph); hylla degraded this session (orch MCP conn dropped) → fell back to Read, as designed. |
| built-in Agent | builder (per-file edit gate) | ta_action_gate.py | ✅ 12_4 R1+R2 builds gated to 2 files |
| built-in Agent | build-qa-proof / plan-qa-proof | edit:[] gate | ✅ 12_4 proof PASS (sonnet) |
| claude -p | (NOT used in hylla/ta/valv per dev rule) | n/a | N/A — sand/tillsyn config option only |

## 3. Methodology alignment (design-down / build-up + parallelization)

- [x] cp `tillsyn/main/CASCADE_METHODOLOGY.md` → `hylla/.../CASCADE_METHODOLOGY.md` (543 lines)
- [x] Read canonical CASCADE_METHODOLOGY.md — **CORRECTION**: methodology says NO child cap; recurse on
      ATOMICITY (1-2 blocks). "3-4 per leaf" is the typical RESULT, not a rule. Aligned to that, not a cap.
- [x] CLAUDE.md "Drops And Droplets" — replaced "add more droplets to parent" nudge with
      recurse-on-atomicity / no-cap / sub-plan-when-non-atomic / asymmetric-tree + blockers-for-parallelism.
- [x] CLAUDE.md "Coordination Model" — added Plan-Down-Build-Up + the per-branch parallelism LOAD-BEARING
      rule (keep every unblocked node moving; QA twins ALWAYS parallel pair) + local methodology pointer.
- [x] Persona light-touch DONE: `ta-go-planning` + `ta-fe-planning` got a "Parallelism + asymmetric tree"
      bullet (code-independent siblings = no blockers → concurrent; serialize only on real deps; per-branch
      asymmetric depth; minimize blocker chains) citing `CASCADE_METHODOLOGY.md`. plan-qa personas (proof:
      "disjoint siblings must run parallel"; falsif: "over-blockers suppresses parallelism") already aligned —
      no change. (field name is `prompt`, not `body`, for claude_agents.agent records.)

## 4. Propagation checklist (AFTER proven here)

Apply the proven dispatcher + hook + personas + CASCADE_METHODOLOGY.md + CLAUDE.md methodology sections to:
- [x] **Universal files cp'd 2026-05-26** (no git — each repo's orchestrator commits): `bin/agent-dispatch.sh`
      (startup_timeout_sec=15 on all MCP servers + build-qa ta-only strip) + `CASCADE_METHODOLOGY.md` →
      `ta/main`, `sand/main`, `valv/main` — md5-verified identical to hylla (`39a2ea3…` / `69cc476…`).
      (Pre-cp diff confirmed the 3 were at the pre-fix baseline — clean apply, no divergence clobbered.)
- [ ] Per-repo CLAUDE.md methodology sections (plan-down/build-up + per-branch parallelism + recurse-on-atomicity)
      + planning-persona parallelism bullet — each repo's orchestrator applies (repo-specific content; guided by
      this tracker + HYLLA_BIN.md). valv = Go-only/no-FE; sand = Go MCP translator (full why); ta = full fe+go.
- [ ] (tillsyn/main is the methodology SOURCE — reconcile, don't overwrite.)
- [ ] hook (`ta_action_gate.py`) + chains (`agent-chains.sh`) — unchanged this session; already synced. Re-cp only if a repo drifted.

## 5. Open items / risks

- hylla MCP currently OFFLINE (dgraph down) — blocks testing codex planning/plan-qa roles (they inject
  hylla-ro). Bring hylla MCP up before testing those.
- If ta-only build-qa codex STILL flakes → the hang is codex/ta-MCP or codex API itself, not context7/gopls;
  next step would be an MCP-init timeout guard in the dispatcher.
