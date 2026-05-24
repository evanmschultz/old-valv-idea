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

## Unit 15.1 — Round 1

Verdict: FAIL — confirmed counterexamples in `WriteAllowlistSection` span handling and `EffectiveAllowlist` host validation.

### Counterexamples

1. **Indented `[allowlist]` headers are valid TOML, but the rewrite breaks byte-preservation outside the section span.**
   The TOML spec treats indentation as whitespace and ignores it for table headers, so `  [allowlist]` is a valid table header. The implementation trims left whitespace before header detection (`internal/tools/allowlist.go:301-315`), then sets the replacement start at the `[` byte and preserves the preceding two spaces in `prefix` (`internal/tools/allowlist.go:192-205`, `261-355`). Because `prefix` now ends in spaces instead of `\n`, `WriteAllowlistSection` injects an extra newline before rendering the canonical section (`internal/tools/allowlist.go:199-205`). Concrete input:
   ```toml
   [tools]
   mage = "latest"

     [allowlist]
   hosts = ["stale.example.com"]
   [env]
   FOO = "bar"
   ```
   rewrites to a file containing `"\n  \n[allowlist]\n"` rather than preserving the outside-span bytes `"\n  [allowlist]\n"` verbatim. That breaks Schema Decision 3's byte-preservation contract for valid TOML input.
   Narrow fix: either reject leading-whitespace table headers as `ErrUnsupportedManifestShape`, or move the preserved prefix boundary to the start of the header line when rewriting so the whitespace stays attached to the section header.

2. **`EffectiveAllowlist` does not implement the RFC 1123-style validation the builder claims.**
   The builder worklog says `hostShapeRE` enforces RFC 1123-style validation (`drops/DROP_15_NETWORK_POLICY/BUILDER_WORKLOG.md:106,124`), but the actual validator only checks character shape (`internal/tools/allowlist.go:71,112-130`). It never enforces the 63-byte per-label limit or the 253-byte hostname limit. Concrete counterexample: `strings.Repeat("a", 64) + ".example.com"` passes `validateHost`, so `EffectiveAllowlist` will accept a host with an overlong label even though the claimed contract says RFC 1123-style validation.
   Narrow fix: after normalization, split on `.` and reject any label longer than 63 bytes; also reject total hostnames longer than 253 bytes before returning the effective allowlist.

3. **Multi-line-string detection is over-broad and rejects valid comments outside `[allowlist]`.**
   TOML comments consume the rest of the line after `#` unless inside a string. The implementation instead rejects any line outside `[allowlist]` containing the raw byte sequence `"""` or `'''` (`internal/tools/allowlist.go:325-330`). A valid comment such as `# keep triple quotes """ here` before `[allowlist]` will therefore return `ErrUnsupportedManifestShape` even though the file contains no multi-line string at all.
   Narrow fix: ignore comment text before scanning for triple-quote openers, or replace the substring heuristic with a minimal TOML-aware lexer for comment/string state.

### YAGNI check

PASS with one caveat. The unit did not introduce speculative interfaces or a second abstraction layer; the added helpers are the direct seam Unit 15.4 needs. The problem is not over-abstraction, it is contract drift inside the concrete helper implementation.

### Hidden dep check

FAIL. `DefaultAllowlistHosts` is an exported mutable slice (`internal/tools/allowlist.go:38-43`). Any in-repo caller can mutate it in place and silently change the effective policy for every later `EffectiveAllowlist` call. That is hidden global state in a security-sensitive path.
Narrow fix: make the defaults unexported and expose either a getter that returns a copy or a fixed `[4]string`/copy-on-read helper.

### Evidence

- Delta inspected: `git show 0611cf0 -- internal/tools internal/services/images`
- Code read: `internal/tools/allowlist.go`, `internal/tools/allowlist_test.go`, `internal/tools/tools.go`, `internal/services/images/overlay_test.go`
- Repo contract read: `drops/DROP_15_NETWORK_POLICY/PLAN.md`, `drops/DROP_15_NETWORK_POLICY/BUILDER_WORKLOG.md`
- External semantics: TOML v1.0.0 table/comment rules (`https://toml.io/en/v1.0.0`)
- Verification attempt: temporary falsification tests were added and removed, but `mage testPkg ./internal/tools` could not run in this sandbox because `go list` failed opening the default Go build cache under `/Users/evanschultz/Library/Caches/go-build/...` with `operation not permitted`
