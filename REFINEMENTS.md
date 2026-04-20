# REFINEMENTS

Post-drop refinement backlog — deferred findings and future-round work.

## DROP_0_DOCS_BOOTSTRAP — 2026-04-19

1. **Restore `coverageThreshold` to `70.0` in `magefile.go`.** Phase 6 of DROP_0 temporarily lowered the per-package coverage floor from 70.0 → 60.0 because two pre-existing packages sat below 70% (`internal/services/openaiapi` at 61.3%, `internal/adapters/docker` at 64.7%). `internal/services/openaiapi` is deleted by DROP_1 (self-resolving). `internal/adapters/docker` is load-bearing (used by both Codex and future Claude runtime paths) and needs real coverage lift. **Trigger for restoration:** `internal/adapters/docker` coverage ≥ 70% and `internal/services/openaiapi` no longer present. Scope candidate: fold into DROP_9 cleanup backlog or split its own coverage-raise drop.
2. **Codify falsification-agent append mechanics in `main/drops/WORKFLOW.md`.** The global `go-qa-falsification-agent` definition lacks `Edit`/`Write` tools; it can only append via `Bash` (`cat << 'EOF' >> path` pattern). This tripped U0.3 (agent returned draft text rather than appending) and was fixed reactively. Add an explicit line to the Per-Role Spawn Appendices section so the instruction travels with every falsification spawn.
