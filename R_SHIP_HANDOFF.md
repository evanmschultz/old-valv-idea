# R-SHIP-VALV — Handoff to Dev

**Date:** 2026-05-30
**Tillsyn refinement:** `8910ba94-4c8e-4e06-b969-a41d486625f5` (R-SHIP-VALV)
**Source-of-truth sibling:** `ta` (architecture) + `tillsyn` (CASCADE_METHODOLOGY.md canon)
**Memory rule:** `feedback_no_sibling_git_mutations` — orch wrote files only; ALL git is yours.

valv is an existing Go-only sibling. Orch sync'd the agent architecture (Batch 1) + canonicalized the magefile (Batch 2). No git touched.

---

## Batch 1 — agent infrastructure (cp from ta, byte-identical)

Same set as the other siblings: `bin/agent-dispatch.sh`, `bin/agent-audit-toon.py`, `.claude/hooks/ta_action_gate.py`, `.claude/hooks/post_tooluse_agent_audit.py`, 7 Go-only persona `settings.json` + `.md` (Path B 2.2.A), `.claude/settings.json` (PostToolUse Agent matcher added), `CASCADE_METHODOLOGY.md` (sha256 `87708e81…`).

## Batch 2 — magefile canonicalized

`magefile.go` now exposes the canonical 12-target shape (verified via `mage -l` — 22 targets). valv's laslig/gotestout rendering, rich per-package coverage gate, `Dev` namespace (Home/Reset/Clean/Run), Docker integration, `Golden`/`GoldenUpdate`/`Integration`/`Run`/`Build`/`Install` all preserved.

- **Rename:** the rich `Test` gate → `CI` (the `check` alias is preserved). A NEW plain `Test` = `go test -count=1 ./...` now exists per the canonical contract.
- **`TestPkg`** changed from the heavy format+race+cover+threshold per-package gate → canonical plain `go test -count=1 <pkg>`. The rich coverage machinery survives in `Cover` + the CI gate.
- **New targets:** `TestFunc(pkg, testName)`, `Race`, `RacePkg`, `Cover` (standalone race+cover+threshold), `Format`, `FormatFile`, `FormatCheck`, `Vet`, `VetPkg`, `Tidy`. Aliases: `fmt`, `fmt-check`, `format-check`, `format-file`, `test-func`, `test-pkg`, `race-pkg`, `vet-pkg`, `golden-update`, `check`.
- **Dead code removed:** `packageGoFiles` (its only caller was the old `TestPkg`).

### ⚠️ NEW CI stages may surface preexisting issues

valv's CI gate now runs **Vet** and **Tidy** stages it did NOT run before:

- `mage ci` Vet stage = `go vet ./...`. valv had NO vet gate previously — this may surface preexisting vet diagnostics.
- `mage ci` Tidy stage = `go mod tidy` + drift check. May surface go.mod/go.sum drift.

If `mage ci` fails on Vet or Tidy, that's a **preexisting issue newly caught**, not a regression from the magefile change. Two options:
1. Fix inline (small) + commit.
2. Spawn a fix cascade in valv via `ta` (the same way tillsyn's P-VET-FIX cascade fixed its race-detector findings) — track it as a `ta` cascade drop.

valv's coverage threshold stays at its current `60.0` (with the existing TODO to restore to 70.0) — orch did not change it.

---

## Verify + commit (YOUR hands)

```sh
cd /Users/evanschultz/Documents/Code/hylla/valv/main
git status
mage ci                          # FormatCheck + Vet + Cover(race+cover+threshold) + Tidy
# If Vet/Tidy surface preexisting issues: fix inline or spawn a ta cascade (see above).
git add bin/ .claude/ CASCADE_METHODOLOGY.md magefile.go R_SHIP_HANDOFF.md
git commit -m "chore: sync agent infra from ta + canonical magefile (12-target shape + vet/tidy gates)"
git push origin main
gh run watch --exit-status
```

**Hylla ingest** (after CI green):
```
mcp__hylla__hylla_ingest(source_url="https://github.com/evanmschultz/valv.git", ref="<SHA>", branch="main", enrichment_mode="full_enrichment", stream=true)
```

Then tell orch the SHA + ingest task id so R-SHIP-VALV closes.

## Note: CLAUDE.md + GH workflow

- **valv's `CLAUDE.md` was caveman'd in the P5 pass (2026-05-30): 51,501 → ~29,210 chars (−43%), 32 → 24 sections.** Fixed: undated the `(LOAD-BEARING)` header; "never TaskCreate" → dual-use; 4 stale `go-*-agent` persona names that contradicted the Agent Bindings table (→ `ta-go-*`); collapsed the verbose 7-rule cascade block (dup of the canon valv points to) → terse. The big size win came from killing the **WORKFLOW.md/CASCADE_METHODOLOGY.md restatement**, the **testing/standards quadruple-overlap** (Go-Rules §Tests + Delivery Standards + Build Verification + Sandbox/Go-Tooling + Repository Standards all restated mage/testcontainers/no-mock/no-GOCACHE), and the **duplicated Orchestrator Role Boundaries** (stated twice) — NOT by cutting product behavior. Every product/runtime/MCP/Auth/CLI-TUI behavioral rule is retained as a terse bullet (verified: host.docker.internal, CODEX_HOME, modernc.org/sqlite/no-CGO, DeleteProfile, device-code, ResolveWorktreeGitDir all present). valv's CLAUDE.md is a plain markdown file (valv doesn't use the ta substrate) — no `ta index rebuild` needed. Stage `CLAUDE.md` with this commit.
- valv's `.github/workflows/ci.yml` still calls whatever gate it called before — confirm it invokes `mage ci` (the `check` alias still maps to CI, so `mage check` also works). If it calls `mage test`, note that `Test` is now the PLAIN target (not the gate); update the workflow to `mage ci`.
