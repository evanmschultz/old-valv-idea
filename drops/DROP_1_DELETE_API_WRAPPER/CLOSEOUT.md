# DROP_1 — Closeout

Written at drop close. See `main/drops/WORKFLOW.md` § "Phase 7 — Closeout" for the full step list.

- **Closed:** 2026-04-19
- **Final commit:** 4be62eeefe906a0f3ca199c7d8eff1ae474dd032 (`docs(drop-1): unit 1.5 qa green`)
- **CI run:** https://github.com/evanmschultz/valv/actions/runs/24657620454 (green — test 1m19s macos-latest, integration 1m9s ubuntu-latest)

## Hylla Feedback Aggregation

None. All five units (1.1–1.5) recorded their Hylla Feedback subsection as N/A. Units 1.1–1.4 were Go-file deletions against a file set changing relative to the latest ingest; per `main/CLAUDE.md` § "Code Understanding Rules" item 2 the correct evidence source for files changing since last ingest is `git diff` / direct `Read`/`Grep`, not Hylla. Unit 1.5 touched non-Go files only (markdown doc scrubs + one markdown delete); per item 3 Hylla is not the correct source for markdown. No Hylla query was forced into a fallback — Hylla was correctly not invoked.

## Refinements

1. **Planner acceptance-regex discipline for deletion drops.** Unit 1.1's acceptance criterion initially used the grep pattern `internal/api/` (trailing slash), which missed the bare token `internal/api` referenced at `main/CLAUDE.md:133` and left unused orphan test symbols (`TestRunAPIRuntimeSweeperPrunesUntilContextCancel`, `pruneRecorder`) in `internal/cli/extended_test.go`. Patched inline mid-build by tightening to `grep -nE "internal/api\b"` and routing the orphan-symbol collateral through the builder. **Trigger:** future deletion drops should write their primary sentinel grep with `\b` word boundaries (not trailing `/`) and require a second "orphan symbols" acceptance criterion that scans for test helpers whose only callers were in the deleted file(s). Not a WORKFLOW.md change — a planner discipline note worth mentioning in a future planning-agent prompt appendix.
2. **Falsification agent append-mechanics worked cleanly on DROP_1.** The `cat << 'EOF' >> path` Bash-append pattern added to the U0.2 spawn during DROP_0 was re-issued in every DROP_1 falsification spawn and every subagent appended its `## Unit N.M — Round K` section on first try. Codify this into `main/drops/WORKFLOW.md` § "Per-Role Spawn Appendices" under the "Build QA (proof / falsification)" bullet so the instruction no longer has to be re-typed per spawn. Carried over from DROP_0's existing refinement #2 — the fix pattern is now proven across two drops.
3. **Coverage-floor bend self-resolved for `openaiapi` (deleted by this drop); `internal/adapters/docker` still at 64.7%.** `internal/services/openaiapi` no longer appears in the `mage test` coverage table (confirmed at drop-end). The remaining 70→60 bend is gated solely on `internal/adapters/docker`'s 64.7% coverage. DROP_0 refinement #1 unchanged.

## Ledger Entry

**DROP_1_DELETE_API_WRAPPER — closed 2026-04-19.** Stripped the local OpenAI-compat HTTP wrapper per `VALV_CLAUDE_CODE_FOCUS_PLAN.md` §2: removed `internal/cli/api.go` + the `valv api` command wiring, deleted `internal/services/openaiapi`, deleted `internal/api/openai` + the parent `internal/api/` dir, deleted the root-level `compatibility.go` / `compatibility_test.go` / `codex-openai-compatibility.json` valvcompat surface, and scrubbed every trace of OpenAI-compat prose from `README.md` / `CONTRIBUTING.md` / `AGENTS.md` / `CLAUDE.md` / `VALV_REPO_PLAN.md` / `valv_architecture_notes.md` while preserving the one legitimate "cheapest OpenAI-compatible model" mention and the architecture-notes supersede pointer. Five build units, all green on round 1 across plan-QA (one round) and build-QA (parallel proof + falsification per unit). Codex-in-Docker runtime, multi-account SQL profile/project-path binding, account management, TUI, and all Docker adapter code untouched — the removal was surgical to the local HTTP wrapper only. CI run: https://github.com/evanmschultz/valv/actions/runs/24657620454.

## Wiki Changelog

2026-04-19 — DROP_1 deleted the local OpenAI-compat HTTP wrapper; Valv's surface is now pass-through vendor CLIs only. No best-practice shift — `WIKI.md` stays minimal.

## Hylla Ingest

- **Triggered:** 2026-04-19 (after CI green)
- **Mode:** full_enrichment
- **Source:** github.com/evanmschultz/valv.git @ commit 4be62eeefe906a0f3ca199c7d8eff1ae474dd032
- **Result:** completed (task id `task-a251ee0f14944ea9`) in ~47s.

## WIKI.md Updates

None — no best-practice shift. DROP_1 was a deletion drop: it simplified the runtime surface but did not change the drop workflow, Go standards, mage discipline, or QA patterns that `AGENTS.md` / `main/CLAUDE.md` / `main/drops/WORKFLOW.md` already own.
