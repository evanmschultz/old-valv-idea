## Unit 15.0 — Round 1

Verdict: PASS — no unmitigated counterexample found.

### Counterexamples

None confirmed.

Attacks attempted:

- No-` .git` fallback: `project.DetectFrom` falls back to the normalized start path, and when `start == ""` it first resolves `os.Getwd()` (`internal/project/project.go:28-81`). A targeted repro against `resolveProjectImage("", ...)` from a no-git directory with a local `.valv/tools.toml` still built the overlay, matching the builder's "degenerates to old behavior" claim.
- Non-root manifest: `resolveProjectImage` now calls `tools.Resolve(detected.Root)`, not `tools.Resolve(workingDir)` (`internal/cli/operator_helpers.go:448-477`). A targeted repro with `.valv/tools.toml` only in the nested subdirectory produced `baseRef` and zero docker calls, which matches the drop plan's root-only contract.
- Empty `workingDir`: same targeted repro above covered the empty-string path through `project.DetectFrom("")` and found no break.
- Worktree linkfile: committed coverage already proves `.git` linkfiles resolve to the worktree root rather than the admin gitdir target (`internal/project/project_test.go:34-55`), and `go test ./internal/project -run 'TestDetectFrom(LinkedWorktreeRoot|NormalizesSymlinkedRoot|FallsBackToCurrentDirectory|FilePathUsesContainingDirectory)$'` passed locally.
- Override ordering and env precedence: the override warning still happens only after manifest resolution and only for the provider-specific env var (`internal/cli/operator_helpers.go:454-466`). A targeted repro with both `VALV_CLAUDE_IMAGE` and `VALV_CODEX_IMAGE` set during a Claude launch used only `VALV_CLAUDE_IMAGE` and made zero docker calls.

### YAGNI check

PASS. The change is the minimal reuse move: `resolveProjectImage` now delegates root discovery to the already-existing `project.DetectFrom` helper and keeps all prior overlay/override behavior in place (`internal/cli/operator_helpers.go:448-477`). No new abstraction, cache, or provider-specific fork was introduced.

### Hidden dep check

Observed, but not a Unit 15.0 regression: project detection inherits `pathutil.Normalize`, which resolves symlinks before ancestor scanning (`internal/project/project.go:60-81`, `internal/pathutil/pathutil.go:12-35`). The Claude and Codex launch services already share that same normalize-then-detect contract and reject working directories outside the resolved project root (`internal/services/claude/service.go:128-147`, `internal/services/claude/service.go:298-344`, `internal/services/codex/service.go:121-140`, `internal/services/codex/service.go:302-346`). Concretely, a lexical repo path that traverses a symlink to an external directory is treated as external everywhere, not just in `resolveProjectImage`. That's a cross-cutting assumption to keep explicit, not a new falsifier for this unit.

### Evidence

- Delta inspected: `git show 01ba002 -- internal/cli/operator_helpers.go internal/cli/claude_project_image_test.go internal/cli/codex_project_image_test.go`
- Targeted existing tests: `GOCACHE=/private/tmp/valv-go-cache go test ./internal/project -run 'TestDetectFrom(LinkedWorktreeRoot|NormalizesSymlinkedRoot|FallsBackToCurrentDirectory|FilePathUsesContainingDirectory)$'`
- Targeted repros (temporary test file added then deleted): `GOCACHE=/private/tmp/valv-go-cache go test ./internal/cli -run 'TestResolveProjectImage(SymlinkedLexicalSubdirOutsideRepoDoesNotUseRootManifest|NoGitEmptyWorkingDirFallsBackToCWD|IgnoresNestedManifestBelowDetectedRoot|ClaudeBothOverrideVarsUseClaudeOnly)$'`
