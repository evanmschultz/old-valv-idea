## Plan — Round 2

**Verdict:** PASS

Proof-oriented review of the revised DROP_4 plan against the Round 1 synthesis brief. All seven accepted findings (F1/F2/F3/F4/F5/F8 + proof-side spinner wording) are applied in the correct locations with correct content, and the Round 1 plan structure is preserved.

### 1. Synthesis-Brief Compliance

- **1.1 F1 — `recipeHash()` Codex-hardcoded regression.** PASS. Unit 4.1 description item 4 (line 92) adds the unexported `providerDockerfileContent()` helper on `Service` with a `s.provider == domain.ProviderClaude` switch returning `DefaultClaudeDockerfile()` and defaulting to `DefaultCodexDockerfile()`. Item 5 (lines 93-107) replaces the unconditional `content := DefaultCodexDockerfile()` seed with `content := s.providerDockerfileContent()` while preserving the custom-Dockerfile disk-read fallback. The Paths list at line 74 explicitly scopes the `recipeHash()` edit to `service.go` inside the images package.
- **1.2 F2 — regression test for F1.** PASS. Unit 4.1 description (line 113) adds `TestServiceBuildRecipeHashMatchesProviderDockerfile`, explicitly table-driven with two cases (`ProviderCodex`, `ProviderClaude`), reusing the existing `*runnerRecorder` fake runner, extracting the recorded `--label io.valv.recipe_hash=<hex>` argument and asserting equality with `hex.EncodeToString(sha256.Sum256([]byte(<provider-default-dockerfile-content>)))` per case. Acceptance criterion at line 125 binds both provider hashes, and criterion at line 126 asserts the `providerDockerfileContent()` helper is the single source of truth with no remaining Codex seed.
- **1.3 F3 — TUI home `ActionUpdate` deferral.** PASS. The "Explicitly deferred (NOT in DROP_4)" block (line 37) names `internal/cli/operator_helpers.go:122-127` and states "remains Codex-hardcoded after DROP_4," explicitly routing the MVP Claude update path through `valv manage update claude` via cobra.
- **1.4 F4 — npm deprecation fallback.** PASS. Notes (line 219) describes the primary path (`npm install --global @anthropic-ai/claude-code@${CLAUDE_VERSION}`), the native-installer fallback (`curl -fsSL https://claude.ai/install.sh | bash`), and the builder's requirement to record a pivot (Dockerfile change + Context7 recheck timestamp) in `BUILDER_WORKLOG.md`.
- **1.5 F5 — `supportedProviders` drift.** PASS. Committed-state audit line 51 now reads `manage.supportedProviders` at `internal/cli/manage.go:984` (correct symbol name and line). The R1 falsification's flagged `allProviders:985` error is no longer present — a scan of the file found no remaining `allProviders` occurrence in the planner section.
- **1.6 Proof F8 — spinner wording.** PASS. Unit 4.3 description item 3 (line 193) preserves the Codex spinner strings verbatim (`"Checking provider image"` / `"Provider image check complete"` / `"Provider image update failed"`) and specifies Claude strings (`"Building provider image"` / `"Provider image built"` / `"Provider image build failed"`). The requirement to wrap `service.Build(...)` in the `runWithCLIQuietSpinner` closure mirroring the Codex `EnsureLatest` wrapper at `manage.go:1096-1106` is explicit, and acceptance criterion at line 207 re-asserts the wrapper.

### 2. Acceptance Verifiability + Package-Lock

- **2.1 Acceptance yes/no-verifiable.** PASS. Unit 4.1 (119-129) acceptance items are each mechanical checks: `go doc` non-empty, `mage testPkg` green, hash-equality assertion binding both providers, helper-exists assertion, grep presence for three Dockerfile strings, integration-build-tag gating. Unit 4.2 (170-176): `openImagesService` returns non-zero, file exists at provider-keyed cache path, existing Codex suite stays green, new helpers exist at file:line, mage green, no cache-dir collision. Unit 4.3 (202-209): output record fields match, mage green, spinner-wrapper presence, no new exported symbol, pinned version recorded in worklog.
- **2.2 Package-lock preserved.** PASS. Line 213 restates the serialization: 4.2/4.3 both touch `internal/cli` and are serialized via `blocked_by`; 4.1 is on the disjoint `internal/services/images` package but is consumed by 4.2/4.3, making the chain effectively linear.

### 3. Round 1 Structure Preservation

- **3.1 Three-unit decomposition intact.** PASS. Headings at lines 71 (Unit 4.1), 133 (Unit 4.2), 180 (Unit 4.3) unchanged from the Round 1 skeleton.
- **3.2 `blocked_by` chain intact.** PASS. 4.1 blocked_by "—" (line 76), 4.2 blocked_by "4.1" (line 138), 4.3 blocked_by "4.2" (line 185).
- **3.3 Integration test gating intact.** PASS. Line 129 retains `//go:build integration` gating for `TestWriteDefaultClaudeContextBuildsWithExistingUIDAndGID`, ensuring `mage testPkg` stays green on hosts without Docker while `mage integration` exercises the test when Docker is available.

### 4. Advisory (Non-Blocking)

- **4.1 `recipeHash()` code sample uses current constant name.** The proposed code block at lines 93-107 references `defaultCodexDockerfile` in the custom-Dockerfile fallback branch. This is the existing unexported constant already present in `service.go` (verified in R1 audit at `service.go:441-451`), so the sample is faithful to current source. If DROP_5 later renames that constant to a provider-agnostic `defaultDockerfile`, it will be a one-line change there — not a DROP_4 scope issue. Noting so the builder treats the constant as pre-existing rather than re-declaring it.

### 5. Routed Unknowns

- **5.1 None.** All seven accepted findings were addressed in-plan and the three structural invariants were preserved. No new Unknowns produced by this proof pass.
