# Valv — Per-Drop Workflow

Canonical lifecycle doc for one drop, start to finish. **`main/CLAUDE.md` and `main/PLAN.md` link here rather than duplicate these rules.** Authoritative source for: drop directory shape, file lifecycles, phase order, subagent spawn cadence, restart recovery.

If you change a phase, change it here once. Do not copy the rules into `CLAUDE.md` or `PLAN.md`.

## Drop Directory Layout

Every drop is a **directory** under `main/drops/`. The directory name mirrors the PLAN.md container row title (`ALL_UPPERCASE_WITH_UNDERSCORES` per `feedback_naming_conventions.md`).

```
main/drops/
├── WORKFLOW.md                     # this file
├── _TEMPLATE/                      # orch copies this for each new drop
│   ├── PLAN.md
│   └── BUILDER_WORKLOG.md
├── DROP_1_DELETE_API_WRAPPER/
│   ├── PLAN.md                     # durable
│   ├── PLAN_QA_PROOF.md            # transient — git rm between rounds
│   ├── PLAN_QA_FALSIFICATION.md    # transient — git rm between rounds
│   ├── BUILDER_WORKLOG.md          # durable
│   ├── BUILDER_QA_PROOF.md         # durable
│   └── BUILDER_QA_FALSIFICATION.md # durable
└── DROP_2_…/
    └── …
```

### File Lifecycle

| File | Lifecycle | Owner |
|---|---|---|
| `PLAN.md` | **durable** — refined across plan-QA rounds; final at close | planner subagent edits, orch + builder + QA read |
| `PLAN_QA_PROOF.md` | **transient** — `git rm` between plan-QA rounds | qa-proof subagent writes |
| `PLAN_QA_FALSIFICATION.md` | **transient** — `git rm` between plan-QA rounds | qa-falsification subagent writes |
| `BUILDER_WORKLOG.md` | **durable** — append `## Unit N.M — Round K` per build attempt | builder subagent appends |
| `BUILDER_QA_PROOF.md` | **durable** — append `## Unit N.M — Round K` per QA attempt | qa-proof subagent appends |
| `BUILDER_QA_FALSIFICATION.md` | **durable** — append `## Unit N.M — Round K` per QA attempt | qa-falsification subagent appends |

**Transient = audit via `git log -- <path>`.** The `git rm` records the deletion and the prior file content stays recoverable. Transient files signal active state by their presence — absent files mean "phase complete, no findings outstanding".

**Durable = append rounds.** Heading convention: `## Unit N.M — Round K` (e.g. `## Unit 1.3 — Round 2`). Plan-phase findings use `## Plan — Round K` instead of a unit number.

## Phase Order

Plan → Plan QA → Discuss + Cleanup → (loop until plan good) → Build (per unit) → Build QA (per unit) → (loop until unit good) → Verify → Close → next drop.

---

## Agent Spawn Contract

The agents in `~/.claude/agents/` (`go-builder-agent`, `go-planning-agent`, `go-qa-proof-agent`, `go-qa-falsification-agent`) are **global** — shared with other projects that may still use Tillsyn. Their agent definitions reference `till_*` tools, capability leases, capture_state, attention items, and handoffs. **Valv does not use any of that.** Every spawn from a Valv orchestrator overrides those instructions.

This section is the **single canonical source** for the override preamble. Every Phase 1–7 spawn pulls its preamble from here. Do not inline the preamble inside the phase sections — they all link back here. If the override needs to change, change it here once.

### Required preamble (paste verbatim into every spawn)

```
Paradigm override: this project does NOT use Tillsyn. Drop coordination lives
in main/drops/DROP_N_<NAME>/ with the file lifecycle described in
main/drops/WORKFLOW.md. Ignore any instructions in your agent definition that
refer to till_*, capture_state, attention_item, handoff, capability_lease, or
auth_request. Read main/drops/WORKFLOW.md before acting. Edit only the files
your phase owns (see WORKFLOW.md "File Lifecycle" table).

Git discipline (LOAD-BEARING): you are NOT the committer. You may run
READ-ONLY git only — git diff, git status, git log, git show, git
rev-parse. You are FORBIDDEN from running ANY git command that mutates repo,
index, working-tree, or remote state: no git add, commit, push, restore,
reset, checkout, switch, stash, rm, mv, merge, rebase, cherry-pick, tag,
branch, or config. The ORCHESTRATOR is the sole actor that stages, commits,
and pushes. If your phase appears to need a commit, STOP and return that to
the orchestrator — do not commit yourself.

Hylla artifact_ref: when your tools allow mcp__hylla__* and you query valv's
own code, use artifact_ref=github.com/evanmschultz/valv@main (pinned to
branch, no float) exactly as the orchestrator passes it in this prompt. Do
NOT hardcode or invent a different ref; if no ref was passed and you need
Hylla, ask the orchestrator.

Render your response beginning with a `# Section 0 — SEMI-FORMAL REASONING`
block containing `## Planner`, `## Builder`, `## QA Proof`, `## QA
Falsification`, and `## Convergence` passes (or the 4-pass subagent variant
per ~/.claude/CLAUDE.md § "Semi-Formal Reasoning") before your final output.
Each pass uses the 5-field certificate (Premises / Evidence / Trace or cases
/ Conclusion / Unknowns) where applicable. Convergence must declare (a) QA
Falsification found no unmitigated counterexample, (b) QA Proof confirmed
evidence completeness, (c) remaining Unknowns are routed back to the
orchestrator. If any fail, loop back before Convergence.

Section 0 reasoning stays in the orchestrator-facing response only — do NOT
write Section 0 into PLAN.md, BUILDER_WORKLOG.md, BUILDER_QA_*.md,
PLAN_QA_*.md, or any other durable Valv artifact.
```

### Per-role appendix (concatenated after the preamble)

Each phase below specifies the per-role appendix it adds (drop's `PLAN.md` path, target output file, unit ID, round number, etc.). Phase sections do not repeat the preamble — they reference this section.

### When the global agents update

If `~/.claude/agents/go-*.md` change in a way that conflicts with the override (e.g. they start *requiring* a Tillsyn call rather than just suggesting one), update the preamble above and bump a `Last reviewed against ~/.claude/agents/go-*.md:` date footnote at the bottom of this section. Last reviewed: 2026-04-18.

---

## Phase 1 — Plan

**Goal:** turn the PLAN.md row into atomic units of work with paths, packages, acceptance criteria, and `blocked_by` ordering.

1. Orch copies `main/drops/_TEMPLATE/` → `main/drops/DROP_N_<NAME>/`. Sets `PLAN.md` header `state: planning`. Commits (`docs(drop-N): scaffold drop dir from template`).
2. Orch spawns `go-planning-agent` with the spawn preamble from § "Agent Spawn Contract" + the planner appendix from § "Per-Role Spawn Appendices". The planner reads `main/PLAN.md`, the drop's `PLAN.md`, `main/CLAUDE.md`, this file.
3. Planner fills `## Planner` section in the drop's `PLAN.md`: scope confirmation, atomic units (`N.1`, `N.2`, …), each with `paths`, `packages`, `acceptance`, `blocked_by`, `state: todo`. Returns control.
4. Orch commits the plan (`docs(drop-N): planner decompose into N units`). Move to Phase 2.

## Phase 2 — Plan QA

**Goal:** independent proof + falsification review of the planner's decomposition.

1. Orch spawns `go-qa-proof-agent` and `go-qa-falsification-agent` **in parallel** with the preamble from § "Agent Spawn Contract" + plan-QA appendix from § "Per-Role Spawn Appendices". Each reads the drop's `PLAN.md`, `main/CLAUDE.md`, `main/PLAN.md`, this file. Each writes its own file:
   - `go-qa-proof-agent` → `PLAN_QA_PROOF.md` with verdict (`pass` / `fail`) + findings
   - `go-qa-falsification-agent` → `PLAN_QA_FALSIFICATION.md` with verdict + counterexamples
2. Disjoint files, no merge race. Both subagents return verdicts to orch via the `Agent` tool result.
3. Orch commits both QA outputs (`docs(drop-N): plan qa round K`).

## Phase 3 — Discuss + Cleanup

**Goal:** synthesize QA findings with dev, hand a planner brief back, clean working tree.

1. Orch summarizes both QA mds for dev (one short numbered list per file). Dev decides accept / reject / defer per finding.
2. Orch synthesizes accepted findings into a planner brief (in-conversation; no scratch file).
3. **Orch `git rm`s `PLAN_QA_PROOF.md` + `PLAN_QA_FALSIFICATION.md`** and commits (`docs(drop-N): clear plan qa round K, route to planner`). Audit lives in `git log -- main/drops/DROP_N_<NAME>/PLAN_QA_PROOF.md`.
4. Orch re-spawns `go-planning-agent` (preamble + planner appendix again, plus the brief). Planner edits `PLAN.md` (revises units, adjusts `blocked_by`, sharpens acceptance). Heading convention: append `## Planner — Round K` if the round count matters for postmortem; otherwise edit in place and let `git log -- main/drops/DROP_N_<NAME>/PLAN.md` carry the audit. Default to in-place edit; bump round headings only when reviewers explicitly request the prior version stay visible.
5. Loop back to Phase 2. Exit when both plan-QA pass without dev-rejected findings.
6. On exit: orch flips drop's `PLAN.md` header `state: building`, commits (`docs(drop-N): plan accepted, advance to building`). Move to Phase 4.

## Phase 4 — Build (per unit)

**Goal:** implement one atomic unit cleanly.

1. Orch picks the next eligible unit (`state: todo` and `blocked_by` empty or all `done`).
2. Orch spawns `go-builder-agent` against unit `N.M` with the preamble from § "Agent Spawn Contract" + builder appendix from § "Per-Role Spawn Appendices".
3. Builder edits unit's `state` in `PLAN.md` to `in_progress` immediately. Implements. Edits `state` to `done` at end. Appends `## Unit N.M — Round 1` section to `BUILDER_WORKLOG.md`: files touched, mage targets run, design notes, `## Hylla Feedback` subsection if any miss forced a fallback.
4. Orch commits (`feat(<scope>): <unit-summary>` per `main/CLAUDE.md` § "Git Commit Format"). Move to Phase 5 for this unit.

## Phase 5 — Build QA (per unit)

**Goal:** independent proof + falsification review of the unit's implementation.

1. Orch spawns `go-qa-proof-agent` + `go-qa-falsification-agent` **in parallel** against unit `N.M` with the preamble from § "Agent Spawn Contract" + build-QA appendix from § "Per-Role Spawn Appendices". Each reads the drop's `PLAN.md`, `BUILDER_WORKLOG.md`, the actual changed code (Hylla / `git diff` / Read), `main/CLAUDE.md`, this file.
2. Each appends `## Unit N.M — Round K` section to its own durable md (`BUILDER_QA_PROOF.md`, `BUILDER_QA_FALSIFICATION.md`) with verdict + findings.
3. **Append-not-overwrite.** Two parallel subagents writing to different files — no merge race. If both write to the same file via append-edit and one fails because the other won the write, orch retries the loser.
4. Pass + pass → orch commits (`docs(drop-N): unit N.M qa green`). Next unit (back to Phase 4 with `N.M+1`).
5. Fail (either) → orch summarizes findings to dev, dev decides direction, orch respawns `go-builder-agent` for the same unit, builder appends `## Unit N.M — Round 2` to `BUILDER_WORKLOG.md`, QA appends `## Unit N.M — Round 2` to its files. Loop until both pass.

## Phase 6 — Verify

**Goal:** machine-checkable confirmation the drop's surface area still builds, tests, and lints clean.

**Per-unit verification** (during Phase 5, before declaring a unit pass): builder runs `mage build` + `mage test` for the touched packages. QA mds note the targets run + result.

**Drop-end verification** (after all units have passed Phase 5):
1. Orch (or builder, by spawn) runs `mage ci` from `main/`. Must pass clean.
2. `git push` once `mage ci` is green.
3. `gh run watch --exit-status` until CI green.
4. If any step fails, treat as build-QA fail on whichever unit owns the breakage — back to Phase 5 for that unit.

## Phase 7 — Close

**Goal:** advance PLAN.md, refresh Hylla, move on. Git history + CI run + per-drop dir are the audit trail. No aggregation docs.

1. Flip drop's `PLAN.md` header `state: done`. Flip the drop's row in `main/PLAN.md` to `state: done`. One commit covers both (`docs(drop-N): close, advance plan`); subject may include CI run URL.
2. Run `hylla_ingest` (full enrichment, from remote, pinned to the final commit hash, **only after CI green**).
3. Move to next drop (back to Phase 1 for `DROP_N+1`).

No `CLOSEOUT.md`, no `LEDGER.md` entry, no `REFINEMENTS.md` entry, no `WIKI_CHANGELOG.md` line, no `HYLLA_FEEDBACK.md` aggregation, no `WIKI.md` update. The drop dir + git log + CI run + Hylla ingest task id record everything a future reader needs.

---

## Per-Role Spawn Appendices

The preamble lives in § "Agent Spawn Contract" above. Each role appends the fields below after the preamble:

- **Planner**: drop's `PLAN.md` path, the PLAN.md container row excerpt, scope sentence from dev, round number if Phase 3 re-spawn.
- **Plan QA (proof / falsification)**: drop's `PLAN.md` path, target output path (`PLAN_QA_PROOF.md` or `PLAN_QA_FALSIFICATION.md`), round number.
- **Builder**: unit ID (e.g. `1.3`), unit row excerpt from drop's `PLAN.md`, drop's `BUILDER_WORKLOG.md` path, round number, working dir.
- **Build QA (proof / falsification)**: unit ID, drop's `PLAN.md` + `BUILDER_WORKLOG.md` paths, target append file (`BUILDER_QA_PROOF.md` or `BUILDER_QA_FALSIFICATION.md`), round number.

## Recovery After Restart

No Tillsyn calls. Recovery is filesystem + git:

1. `git status` — uncommitted work.
2. `git log --oneline -20` — recent commits.
3. Read `main/PLAN.md` — container states (which drop is `in_progress`).
4. List `main/drops/*/PLAN.md` headers — per-drop phase state (`planning` / `building` / `done` / `blocked`).
5. Per active drop: presence of `PLAN_QA_*.md` files = mid-plan-QA loop (Phase 2 or 3); absence + `BUILDER_WORKLOG.md` exists = mid-build (Phase 4 or 5); drop's `PLAN.md` header `state: done` = drop closed.
6. Per active unit: scan latest `## Unit N.M — Round K` heading in `BUILDER_WORKLOG.md` + both `BUILDER_QA_*.md` to figure out whether build, build-QA, or fix is next.

## File State Diagrams

### Drop's `PLAN.md` header `state`

```
planning ──(plan-QA both pass)──▶ building ──(all units done + CI green)──▶ done
   │                                  │
   │                                  └──(blocker discovered)──▶ blocked
   │
   └──(blocker discovered)──▶ blocked
```

`blocked` is orthogonal — entered from any state when a discovered blocker stops forward progress; exited back to whichever state was active.

### Per-unit `state` (inside `PLAN.md` Planner section)

```
todo ──(builder claims)──▶ in_progress ──(builder + both QA pass)──▶ done
                              │
                              └──(blocker)──▶ blocked
```

## Notes on Heading Conventions

- Plan-QA fresh files per round → no round suffix needed in headings.
- Builder + build-QA append rounds → `## Unit N.M — Round K`.
- Planner re-runs default to in-place edit; add `## Planner — Round K` only when reviewers explicitly want the prior version preserved in the file (rare — git log usually suffices).
