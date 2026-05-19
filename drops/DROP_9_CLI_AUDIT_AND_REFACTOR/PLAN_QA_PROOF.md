# DROP_9 — Plan QA Proof — Round 1

**Verdict: PASS with findings (3 substantive, 2 minor)** — decomposition is grounded in source; citations verify; serial chain is justified by the `internal/cli` package lock. Recommend Phase-3 plan revision to address findings before Phase 4 build dispatch.

**Reviewer:** go-qa-proof-agent (Round 1)
**Date:** 2026-05-18
**Files reviewed:** `main/drops/DROP_9_CLI_AUDIT_AND_REFACTOR/PLAN.md` (committed at 85b7dc8)
**Source evidence:** Hylla artifact `github.com/evanmschultz/valv@main` snapshot 2 + direct file reads.

---

## 1. Source citation verification

All planner-cited line numbers / symbol references were verified against source:

| Citation | Status | Note |
|---|---|---|
| `root.go:129` — `accountCmd := newManageAccountCommand(...)` | PASS | Exact match |
| `root.go:131-132` — `manageCmd` registration | PASS | Confirmed |
| `manage.go:49` — `newManageCommand` calls `AddCommand(newManageAccountCommand(...))` | PASS | Exact match |
| `manage.go:105` — `Aliases: []string{"whoami"}` | PASS | Exact match inside `newManageAccountInspectCommand` |
| `manage.go:362` — `--provider` flag definition | PASS (off-by-one) | Actual line is 361 — trivial |
| `manage.go:900-973` — `resolveAccountSwitchTarget` with collision detection | PASS | Function spans exactly these lines; multi-match error at 966-971 |
| `claude_auth.go` `ensureClaudeAccountReady` lines 141-148 (cred check) + 164-166 (RunInContainer error) | PASS | Confirmed |
| `claude_auth.go` `loginClaudeAccount` lines 198-201 (RunInContainer error) | PASS | Confirmed |
| `claude_auth_test.go:75` — `stubAuthContainerExecutor` | PASS | Exact match |
| `images/service.go Service.Build()` already sets `io.valv.managed=true` label | PASS | Hylla node content confirms `Labels: map[string]string{"io.valv.managed": "true", "io.valv.provider": ..., "io.valv.scope": ..., "io.valv.version": ..., recipeHashLabel: ...}` — no new Dockerfile LABEL needed |

Conclusion: every concrete claim about the existing tree verifies.

---

## 2. Spec adherence (audit-proposal vs plan)

Cross-reference against `project_valv_cli_audit_proposal.md`:

| Spec item | Plan unit | Status |
|---|---|---|
| Delete `valv manage` namespace, redistribute children | 9.1 | Covered |
| `valv account bind <name>` | 9.2 | Covered |
| `valv account unbind` | 9.2 | Covered (with caveat — see Finding F1) |
| `valv image update [provider]` | 9.3 | Covered |
| `valv image cleanup` (flag-driven, dry-run default) | 9.3 | Covered (with caveat — see Finding F2) |
| `valv image inspect [provider]` | 9.3 | Covered |
| Flat `valv status` replacing `manage status` + `manage project list` | 9.4 | Partial — see Finding F3 |
| `--provider` flag on account verbs (with positional fallback) | 9.5 | Covered |
| Drop `whoami` alias | 9.5 | Covered |
| `--provider` required on cross-provider name collision (uniform) | 9.6 | Covered (see minor M1 re: 9.5/9.6 collapse) |
| CONCERN A — static `ContainerRunRequest` field assertions | 9.7 | Covered |
| CONCERN B — Ctrl-C × 2 exit hardening | 9.8 | Covered |
| **`--account` override on `valv claude` / `valv codex`** | — | **Not in scope — already shipped by DROP_8.** Plan correctly does NOT re-implement (verified `stripAccountFlag` at `claude.go:60` from DROP_8). |
| Dockerfile `LABEL valv=true` instruction | — | **Correctly dropped** — `Service.Build()` already sets the label via `docker build --label` args; new Dockerfile instruction would be redundant. |
| `account env` (set/unset/list) | — | Out of scope (per memory: "FUTURE", post-DROP_9). Correctly excluded. |
| `account cleanup` stale-home absorbed into `delete` | — | Notes section addresses: `account cleanup` is on the top-level `account` namespace (not `manage`), so it survives the deletion. Correct per spec ("absorbed" refers to the `manage account cleanup` duplicate path, not the top-level command). |

No scope drift detected. No regression of feedback_manual_workflow_is_the_decision (no auto-open of `account-add` reintroduced; the picker is from DROP_8 and is the canonical UX).

---

## 3. Findings

### F1 [Axis: spec-conformance] [severity: high] — Unit 9.2 `paths` are incomplete for a real `unbind` implementation

**Claim**: Unit 9.2 lists `paths: [internal/cli/manage.go, internal/cli/manage_test.go]` only. Acceptance 9.2.2 says: "Implementation: call `service.UnbindProject(ctx, provider, projectPath)`. If the manage service does not expose `UnbindProject` yet, the builder may stub as `not yet implemented`."

**Evidence**:
- Hylla `manage.Service` node lists methods: `BindProject`, `ListBindings`, `Status`, `StatusForProvider`, profile management. **No `UnbindProject` method.**
- `domain.BindingRepository` content (direct read): `Upsert`, `BindingByProjectID`, `ListBindings`. **No Delete/Remove method at all.**
- `sqlite.Store` exposes `UpsertProjectBinding`, `ListProjects`, `CreateProfile`, `Bootstrap`, `Close` — **no binding-removal method.**
- Implementing real unbind requires:
  1. Add `DeleteProjectBinding(ctx, projectID, provider) error` to `domain.BindingRepository`.
  2. Implement on `sqlite.Store` in `internal/adapters/sqlite/store.go`.
  3. Add `UnbindProject(ctx, provider, projectPath) error` to `manage.Service` in `internal/services/manage/service.go`.
- Three files cross **three layers** none of which are in Unit 9.2's `paths`.

**Fix hint**: pick one of:
- **(preferred)** Expand Unit 9.2's `paths` to `[internal/cli/manage.go, internal/cli/manage_test.go, internal/domain/repository.go, internal/adapters/sqlite/store.go, internal/services/manage/service.go, internal/services/manage/service_test.go, internal/adapters/sqlite/store_test.go]` and rewrite acceptance 9.2.2 to mandate the real implementation, listing the three new methods as required additions.
- **(alternate)** Split Unit 9.2 into 9.2a (cobra wiring + stub) and 9.2b (real unbind plumbing through domain/sqlite/services). 9.2b would have the wide paths.
- **(weakest)** Accept stub-only, add a follow-up unit to DROP_11 (cleanup) for the real implementation, and note that `valv account unbind` is documentation-only until DROP_11.

The spec (audit proposal §"Changes summary" line 109 + §"Gaps" line 54) treats `unbind` as a real feature filling a gap dev hit in production (had to drop into SQL). Stub-only contradicts spec intent.

### F2 [Axis: spec-conformance] [severity: medium] — Unit 9.4 acceptance is silent on `manage project list` content

**Claim**: Audit proposal §"Proposed normalized tree" line 75: `valv status` "Replaces `manage status` + `manage project list`." Plan Unit 9.4 acceptance covers `valv status` for the *current* project binding but does NOT pin the disposition of `manage project list` content (list of ALL bindings).

**Evidence**:
- Acceptance 9.4.1: "shows the current project's binding status. Same output as the current `manage status`."
- Design notes line 132 hedge: "Builder decides the minimal path; can keep it as a separate `valv account projects` command or fold as a flag."
- Without an acceptance pin, the builder might:
  - (a) Add `--all` flag to `valv status` → shows all bindings (spec-aligned).
  - (b) Add separate `valv account projects` command → spec-aligned but new surface.
  - (c) Drop `manage project list` content entirely → **regression** (loses functionality).

**Fix hint**: add Acceptance 9.4.6 (or split into 9.4a + 9.4b): pin one of:
- "`valv status` accepts `--all` (or `--list`) flag that emits the previous `manage project list` content."
- "`manage project list` content is moved to `valv account projects` (new sibling command in `account` namespace), preserving column-for-column output."

Whichever the planner picks, write it as an acceptance criterion so the builder doesn't have to design at build time.

### F3 [Axis: atomic-decomposition] [severity: medium] — Unit 9.3 acceptance lacks flag-precedence rules for `valv image cleanup`

**Claim**: Acceptance 9.3.2 enumerates flags (`--images`, `--containers`, `--state`, `--build-cache`, `--all`) and the no-arg dry-run default, but does NOT specify combination semantics.

**Evidence**: Builder is left to decide:
- Does `--all` override individual flags or stack with them?
- Is `--all --images` an error or redundant-but-tolerated?
- Does `--images --containers` AND together?
- Does dry-run dispatch happen when ALL flags are zero, or also when ANY scope flag is set without an explicit `--apply` / `--commit`?

The audit proposal memory (line 98) only specifies "default: dry-run preview" — silent on combination.

**Fix hint**: add acceptance bullets pinning:
- "If no scope flag is set, the command emits a dry-run preview (what would be cleaned, by category, with counts) and exits 0 without deleting."
- "If `--all` is set, all scope flags are implied (treat as union); explicit scope flags alongside `--all` are tolerated (no error)."
- "If any scope flag is set without `--all`, only that scope is processed."
- "Add an `--apply` flag (or `--yes` / `-y`) to commit a destructive run when scope flags are present." (or whatever convention the planner picks; just pin it).

---

## 4. Minor improvements (not blockers)

### M1 — Unit 9.5/9.6 collapse decision belongs in Phase 3, not Phase 4

**Claim**: Acceptance 9.6 note (line 179): "If the builder can do both in one pass, 9.6 may be collapsed into 9.5 (orchestrator decides at build time)."

WORKFLOW.md does not define a "collapse at build time" verb. Phase 4 hands one Unit row to one builder; collapse should happen at plan-revision time (Phase 3).

**Suggested fix**: either
- **Collapse now**: merge 9.5 and 9.6 into one unit "9.5 — Normalize `--provider` flag + uniform collision enforcement + drop `whoami` alias" with the combined acceptance criteria, OR
- **Keep separate** and remove the "may collapse at build time" hedge — strict serial blocking.

### M2 — Unit 9.7 (test-only) could `blocked_by: 9.1` instead of `blocked_by: 9.6`

**Claim**: Unit 9.7 touches only `claude_auth_test.go`. Its mechanical dependencies are:
- Package `internal/cli` must compile (which requires 9.1's deletion of `newManageCommand` to leave a sound package).
- `stubAuthContainerExecutor`, `systemClaudeAccountAuthRunner`, `claudeprovider.ContainerClaudeDir`, etc. — none of these are touched by 9.2-9.6.

Plan currently `blocked_by: 9.6`. Could safely `blocked_by: 9.1`, allowing 9.7 to run in parallel with 9.4/9.5/9.6 **if** the orchestrator wanted lateral throughput.

**However**: since the entire chain is serial via the package lock (single-builder-per-package convention), and the chain only contains 8 units total, the extra latency saved is negligible. Plan's conservative choice is acceptable; this is a minor efficiency note only.

---

## 5. Confirmed non-issues (falsification attacks that failed)

- **Auto-open machinery reintroduction**: No unit re-introduces auto-launch of `account-add` (the rejected pattern from `feedback_manual_workflow_is_the_decision`). DROP_8's picker via `ensureClaudeBindingReady` (claude.go:76) is the canonical UX and is untouched.
- **DROP_8 `--account` override re-implementation**: Verified `stripAccountFlag` exists at `claude.go:60` and `ensureClaudeBindingReady` at `claude.go:76`. No unit touches `claude.go` / `codex.go` flag plumbing. Plan correctly threads DROP_8's work as a pre-condition.
- **`--provider` on `DisableFlagParsing` commands**: Only `valv claude` and `valv codex` have `DisableFlagParsing: true`. Account subcommands use normal cobra flag parsing, so `--provider` is mechanically safe.
- **`t.Parallel()` in 9.7**: `stubAuthContainerExecutor` is instance-scoped state, not package-global. Safe.
- **9.1 acceptance 3 vs 9.2 modifications of `newManageAccountCommand`**: The "unchanged" descriptor in 9.1.3 refers to the registration mechanism (where the command sits in the cobra tree), not to the function body — which 9.2 legitimately extends.
- **Test file routing breakage on 9.1**: Acceptance 9.1.5 + Risk note explicitly call out the `newManageCommand → newManageAccountCommand` rewrite across `manage_test.go`, `root_test.go`, `extended_test.go`. Planner already covered.

---

## 6. Routed unknowns

The plan flags 3 unknowns for builder resolution. Verdict:

| Unknown | Builder-resolvable? | Recommendation |
|---|---|---|
| 9.2 — `UnbindProject` service method existence | **NO** — see Finding F1; needs planner to widen paths first | Resolve in Phase 3 revision |
| 9.1 — `extended_test.go` full scope of `manage` references | YES — builder can scan + rewrite | Acceptable to leave |
| 9.3 — cheapest path for `image inspect` (EnsureLatest vs new method) | YES — builder can pick based on cost | Acceptable to leave |

Only F1's unknown actually escalates; the other two are within builder authority.

---

## 7. Hylla Feedback

**Misses encountered (2):**

1. **Query**: `hylla_search_keyword(query="resolveAccountSwitchTarget resolveProfileSwitchTarget resolveManagedAccount", fields=["content"], visibility_mode=default)` → 0 results.
   - **Missed because**: default `visibility_mode: public_only` filtered out the private functions. These are unexported helpers — exactly the kind of symbol QA needs to verify against citations.
   - **Worked via**: same query with `visibility_mode: include_private` → all 5 functions returned.
   - **Suggestion**: when `id_search_mode: tail_symbol` is used and the search term is a Go identifier with private-looking semantics (lowercase first letter), Hylla could auto-relax `visibility_mode` to `include_private` (with a `did_relax: true` flag in the response) so the caller doesn't have to know the symbol's visibility before querying. Alternatively, document the visibility default more prominently in `hylla_search_keyword` parameter docs — easy to forget that public-only is default when the tool is otherwise so permissive.

2. **Query**: `hylla_search_keyword(query="UnbindProject DeleteBinding manage Service", fields=["content","summary"])` → returned generic top hits (GoldenUpdate, cleanup/New, etc.) — no signal about whether `UnbindProject` exists.
   - **Missed because**: keyword search across `content` and `summary` couldn't distinguish "this method exists" from "this method does not exist." The query was effectively asking for *absence* of a symbol, which keyword indexing cannot answer directly.
   - **Worked via**: `hylla_node_full(node_id=".../internal/services/manage")` returned the full `contains_ids` list — definitive proof of absence (no `UnbindProject` in the contains list).
   - **Suggestion**: this is fundamentally a Hylla strength (negative-evidence queries via package contents enumeration). The miss is on my side — should have gone straight to `hylla_node_full` for "does method X exist on service Y." Minor doc improvement: `hylla_search_keyword`'s description could note "use `hylla_node_full` to confirm symbol *absence* — keyword search can only confirm presence."

No bug-class issues; both misses are ergonomic / workflow-discipline hints.

---

## TL;DR

**Verdict: PASS with findings.** Plan is well-grounded in source — every cited line/symbol verifies. Three substantive findings need Phase-3 revision before Phase 4 build dispatch:

- **F1 (high)**: Unit 9.2 paths don't cover the cross-layer plumbing required for real `unbind` (domain + sqlite + services/manage). Stub-only contradicts spec intent.
- **F2 (medium)**: Unit 9.4 acceptance is silent on `manage project list` content (regression risk).
- **F3 (medium)**: Unit 9.3 acceptance lacks `valv image cleanup` flag-precedence rules.

Plus 2 minor polish items (M1 — pre-decide 9.5/9.6 collapse; M2 — could relax 9.7 blocked_by). Spec adherence is otherwise complete, DROP_8 `--account` work is correctly NOT re-implemented, and no `feedback_manual_workflow_is_the_decision` regression.
