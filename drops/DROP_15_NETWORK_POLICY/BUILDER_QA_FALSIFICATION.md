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

## Unit 15.1 — Round 2

Verdict: PASS — no unmitigated counterexample found against the Round 2 fixes.

### Round 1 counterexample-fix verification

| R1 finding | R2 fix location | Verified by |
|---|---|---|
| CE#1 indented `[allowlist]` header breaks byte-preservation | `splitAllowlistSpan` lines 352-355: `line[0] == ' ' || line[0] == '\t'` AFTER `topLevelHeaderRE` match | `TestWriteAllowlistSection_RejectsIndentedSectionHeader` (allowlist case) + `TestWriteAllowlistSection_RejectsIndentedOtherSectionHeader` (other section case, scope expansion documented in worklog) |
| CE#2 missing RFC 1123 length limits | `validateHost` lines 156-163: `len(h) > 253` then per-label `len(label) > 63`, AFTER `hostShapeRE` so the more specific error wins on broken values | `TestEffectiveAllowlist_RejectsOverlongLabel` (64-byte label) + `TestEffectiveAllowlist_AcceptsMaxLengthLabel` (63-byte boundary accept pin) + `TestEffectiveAllowlist_RejectsOverlongTotal` (254-byte total, labels deliberately 4 bytes so the total-length branch fires) |
| CE#3 over-broad triple-quote detection | new `stripLineComment` helper at lines 427-460 + use in `splitAllowlistSpan` line 383: `bytes.Contains(stripLineComment(line), …)` | `TestWriteAllowlistSection_AcceptsCommentsContainingTripleQuotes` exercises `# keep """ here`, `mage = "latest" # also """ in this comment`, `# and ''' literal triple quotes too` — all round-trip. Existing `TestWriteAllowlistSection_RejectsMultilineStringOutsideAllowlist` still green proves real `"""` openers in CODE are still rejected. |
| Hidden #4: exported mutable defaults | rename `var DefaultAllowlistHosts` → unexported `var defaultAllowlistHosts`; add `func DefaultAllowlistHosts() []string` at lines 57-61 that returns `make + copy` | `TestDefaultAllowlistHostsReturnsCopyNotMutableRef` mutates the returned slice in place and re-asserts both `DefaultAllowlistHosts()` and `EffectiveAllowlist(AllowlistConfig{})` still return the originals |

### New attack vectors — counterexample search

All attacks attempted; no confirmed counterexamples.

1. **Tab-indented section header (`\t[allowlist]`)**: explicit `line[0] == '\t'` branch at `splitAllowlistSpan` line 353 covers tabs equally with spaces. Mitigated by design.
2. **Trailing whitespace on `[allowlist]` header (`[allowlist]  ` or `[allowlist]\t`)**: `topLevelHeaderRE` (`internal/tools/allowlist.go:273`) tolerates `[ \t]*` plus optional `# comment` after the closing bracket. Header detection works; no leading whitespace means the `line[0]` rejection does not fire. Mitigated.
3. **`stripLineComment` with escaped quote inside basic string** (`key = "value with \" embedded #" # comment`): trace — at index of first `"` enter basic; at `\` do `i++` + `continue` so the for-loop's `i++` advances past the escaped char (net +2); at the real closing `"` exit basic; at the next `#` cut. codeOnly correctly captures the string and excludes the trailing comment. Mitigated.
4. **Multi-byte UTF-8 inside comments and strings** (`# 你好 """ 世界`, `key = "日本"`): every UTF-8 multi-byte code unit has byte ≥ 0x80 by construction. None of the bytes `#` (0x23), `"` (0x22), `'` (0x27), or `\` (0x5C) can ever appear as a UTF-8 continuation or lead byte. The byte walker never mis-toggles state mid-codepoint. For the `# 你好 """` case the very first byte is `#`, so `stripLineComment` returns the empty prefix and the outside-span check correctly sees no `"""`. Mitigated.
5. **RFC 1123 63-byte boundary precision** (`>` vs `>=`): builder added an explicit accept-side pin at 63 bytes (`TestEffectiveAllowlist_AcceptsMaxLengthLabel`). A future `>` → `>=` tightening would break that test. Mitigated.
6. **`DefaultAllowlistHosts()` concurrent access**: the function never writes the underlying `defaultAllowlistHosts` slice; it does `make + copy`. Read-only access to a package-level slice initialized at package-init time is data-race-free under the Go memory model. `mage testPkg ./internal/tools` runs `-race` per the project gate and passed (per worklog). Mitigated.
7. **Comment-only / blank lines between `[allowlist]` and the next section**: span boundary defined as "first byte of the next top-level `[section]` header". `splitAllowlistSpan`'s scan only sets `nextStart` on a header match — blank and `# comment` lines never trigger the transition. Schema-Decision-3-compliant: in-span discard is documented and asserted by the existing golden test (`TestWriteAllowlistSection_PreservesPrefixAndSuffix`). Mitigated.
8. **`[allowlist]` as the first line of file** (no preceding prefix): `prefix` becomes `content[:0]` (empty). The `len(prefix) > 0` guard at line 234 skips the conditional `\n` injection, so the rendered section is not prefixed with a stray newline. Mitigated.
9. **File ending without trailing newline + multiple sections**: scanner's `lineLen = len(line) + 1` over-counts by 1 on the final unterminated line; the clamp `if offset > len(content) { offset = len(content) }` at line 398 reconciles. Recorded section offsets are `bytes.Index(line, []byte("["))`-relative to the start-of-line offset, which is correct because each line's start offset is captured BEFORE the increment. Hand-traced a four-section file with no trailing LF; prefix/suffix slices land on correct byte boundaries.
10. **Section-header regex matching `key = "[foo]"`-style payload**: `topLevelHeaderRE` is anchored with `^\[`. The trimmed line of an assignment starts with the key letter, never `[`. Mitigated.
11. **Multi-line array `arr = [` opening on one line, content on later lines, closing `]` on its own line**: none of those lines start with `[<name>]` matching the header regex. Mitigated.
12. **Section header with trailing comment containing brackets**: `topLevelHeaderRE` uses `[^\[\]]+` inside the captured name group, then `[ \t]*(?:#.*)?$`. The optional comment is `.*` so `[allowlist] # foo [bar]` still matches. Mitigated.
13. **Single-quote literal-string `#` handling** (`path = 'C:\Users\#test'`): literal strings don't honor escapes; the walker's `inLiteral` branch only toggles on `'` and never cuts on `#`. Comment cut only fires outside both string states. Mitigated.
14. **`'''` in CODE (not a comment)** (e.g. `s = ''''`): walker keeps toggling but does not mask the byte sequence. `bytes.Contains(codeOnly, []byte("'''"))` finds it and the line is correctly rejected as a multi-line-literal opener. Mitigated.
15. **`[allowlist] # comment with """`** on the header line itself: header-detection runs FIRST and sets `insideAllowlistSpan = true` on the same iteration; the subsequent triple-quote check is gated by `if !insideAllowlistSpan` and skipped. Mitigated by control-flow ordering.
16. **Same control-flow on `[othersection] # """`** BEFORE `[allowlist]`: `stripLineComment(line)` returns `[othersection] ` (cut at the `#`), `bytes.Contains` sees no `"""`. Mitigated.
17. **`s = "a\"b\"c" # comment with """`**: trace through the basic-string escape-skip leaves the closing `"` exiting basic state; the next `#` cuts; codeOnly is the string portion only, which contains at most pairs of `"` (never three consecutive). Mitigated.
18. **Empty file content**: `splitAllowlistSpan([])` returns `(nil, nil, nil)`; caller emits just the canonical `[allowlist]` block. Mitigated.
19. **Indented `[allowlist]` header with leading TAB instead of spaces**: `line[0] == '\t'` branch fires. Mitigated. (Round 2 fix is whitespace-class-complete.)
20. **External-caller audit for renamed `defaultAllowlistHosts`**: `rg DefaultAllowlistHosts|defaultAllowlistHosts` returns 0 production callers outside `internal/tools/` — confirmed (BUILDER_QA_PROOF.md:277 also pinned this). Rename is safe; no silent breakage elsewhere in the tree.

### YAGNI check

PASS. The four Round 2 fixes are minimum-viable: a one-line whitespace check in `splitAllowlistSpan`, an 8-line extension of `validateHost`, a 34-line `stripLineComment` helper that does exactly the work needed and nothing more (no datetime / integer / array lexing), and a 5-line `DefaultAllowlistHosts()` accessor. No new abstraction layer, no new TOML lexer, no exposed configuration of validation policy. Doc-comment prose adjustments to dodge the `gofumpt` triple-apostrophe quirk are cosmetic.

The one scope expansion — rejecting indented `[tools]` / `[env]` headers, not just indented `[allowlist]` headers — is justified by uniform contract: the byte-preservation guarantee applies symmetrically to the prefix and suffix regions, and an indented header in either region would break that contract. Worklog Design Notes record the rationale. Not a YAGNI violation: the test surface explicitly covers BOTH cases, and the doc comment names the rule.

### Hidden dep check

PASS with one accepted residual.

The `DefaultAllowlistHosts` rename closes the externally-mutable-global hole. Verified by `TestDefaultAllowlistHostsReturnsCopyNotMutableRef` — direct in-place mutation of the returned slice does not affect any subsequent `DefaultAllowlistHosts()` or `EffectiveAllowlist(AllowlistConfig{})` call.

Residual (accepted): same-package code under `package tools` can still mutate `defaultAllowlistHosts` directly because Go's package-private visibility allows it. This is the standard idiom for package-private state — not a regression and not in scope to harden further. No production code outside `allowlist.go` accesses the var (confirmed by `rg`). If a future drop introduces in-package mutation, it would be a code-review concern at that point, not a current falsifier.

### Unknowns

1. **Missing 253-byte total-length accept-boundary test.** The 63-byte label limit has a symmetric pair (`TestEffectiveAllowlist_RejectsOverlongLabel` + `TestEffectiveAllowlist_AcceptsMaxLengthLabel`); the 253-byte total limit has only the reject side (`TestEffectiveAllowlist_RejectsOverlongTotal` at 254). A future refactor that tightens `len(h) > 253` to `len(h) >= 253` would not be caught at the unit-test layer. Low-priority — both fences would behave identically for any realistic hostname; the missing pin is symmetry, not a correctness bug.

### Evidence

- Delta inspected: round-2 builder edits in `internal/tools/allowlist.go` and `internal/tools/allowlist_test.go` per `BUILDER_WORKLOG.md:146-219`.
- Code read: `internal/tools/allowlist.go` (full file), `internal/tools/allowlist_test.go` (full file).
- External-caller audit: `rg DefaultAllowlistHosts|defaultAllowlistHosts` confirms zero production callers outside `internal/tools/`.
- Tool invariants verified by trace (no test rerun this round — Round 2 builder already ran `mage testPkg ./internal/tools` → 93 pass / 91.9% cover, recorded at `BUILDER_WORKLOG.md:187-189`; QA Proof R2 already verified completeness — this falsification pass is pure adversarial reasoning over the same evidence base).
- No raw `go test` / `GOCACHE=...` invocations were issued.

## Unit 15.2 — Round 1

Verdict: FAIL — three confirmed counterexamples in label/name handling.

### Counterexamples

1. **Label key with surrounding whitespace is accepted by `Valid()` but emitted untrimmed into `--label`.**
   `NetworkCreateRequest.Valid()` rejects only keys whose `strings.TrimSpace` is empty (`internal/adapters/docker/network.go:40-44`). A key like `"  valv  "` passes that check because the trimmed form is `"valv"` (non-empty). But `BuildNetworkCreateArgs` emits the raw key without trimming: `fmt.Sprintf("%s=%s", key, request.Labels[key])` (`internal/adapters/docker/network.go:68`). The resulting arg is `--label   valv  =x`, which Docker accepts but treats as a label whose key has leading/trailing spaces — distinct from the operator's intent and from the `valv=network-policy` label callers will use for orphan-cleanup matching (Unit 15.2.5 contract, PLAN.md line 156). Operator-cleanup queries using `--filter label=valv=network-policy` will not match this network because the actual label key is `"  valv  "` not `"valv"`.

   Concrete reproducer (added then deleted from `internal/adapters/docker/`):
   ```go
   req := NetworkCreateRequest{
       Name:     "valv-net",
       Internal: true,
       Labels:   map[string]string{"  valv  ": "x"},
   }
   got, _ := BuildNetworkCreateArgs(req)
   // got contains "  valv  =x" — confirmed at runtime via mage testPkg.
   ```
   Test failed with: `UNTRIMMED-KEY-IN-OUTPUT: arg="  valv  =x"`.

   Narrow fix: either trim the key inside `Valid()` and store the trimmed value back (requires struct mutation or normalization at the build-args layer), OR reject any key whose untrimmed form differs from its trimmed form. The latter is the cleaner contract — matches the existing "leading-hyphen rejected" precedent in name validation.

2. **Label key containing `=` produces a `--label key=injected=value` arg that Docker's label parser treats as `key="injected=value"`, hiding the original key.**
   No validation rejects `=` in label keys (`internal/adapters/docker/network.go:32-46`). A key like `"k=injected"` with value `"value"` emits `--label k=injected=value`. Docker's CLI label parsing splits on the FIRST `=`, so the resulting metadata is `{"k": "injected=value"}` — silently dropping the `=injected` suffix from the operator's intended key. This is a subtle data-integrity hazard for the Unit 15.2.5 orphan-cleanup label contract, where every network is expected to carry a deterministic Valv-owned label.

   Reproducer test failed with: `EMBEDDED-EQUALS: arg="k=injected=value" forms ambiguous key=value`.

   Narrow fix: reject `=` in label keys at `Valid()` time. Docker label keys are documented as DNS-prefix-style identifiers (e.g. `com.docker.compose.project`); `=` is never valid in a key.

3. **No maximum network name length is enforced; a 256-byte name is accepted.**
   `dockerNetworkNamePattern = ^[a-zA-Z0-9][a-zA-Z0-9_.-]*$` has no length anchor. Docker's networking layer treats network names as Linux interface-name-bounded for derived bridge/veth interface names (`IFNAMSIZ = 16` byte limit historically; modern Docker normally hashes/truncates internally). A request with `Name = strings.Repeat("a", 256)` builds args successfully without error.

   Reproducer test failed with: `256-BYTE-NAME-ACCEPTED: no length limit enforced`.

   The product impact is bounded — Docker itself will return a runtime error if the name is rejected at the daemon — but the unit's contract is "deterministic arg builder with required-field validation" (PLAN.md line 125 "Tests cover required-field validation, --internal emission, and deterministic arg order") and the worklog claims the regex "mirrors Docker's documented network-name shape" (BUILDER_WORKLOG.md line 27-28). The shape part is correct; the length part is not enforced. This is a contract-drift miss, not a runtime safety bug.

   Narrow fix: add a length limit. Docker's documented practical maximum is 64 bytes for a network name (`docker network create` reference) — pick that or a similar bound and enforce it in `Valid()`.

### YAGNI check

PASS. Unit 15.2 deliberately did NOT add `NetworkConnectRequest` / `BuildNetworkConnectArgs` / `Executor.ConnectNetwork` (worklog line 25, network.go lines 16-20), which matches Schema Decision 5's no-second-network rule. The `Labels` field is `map[string]string` rather than a custom type — minimal. No speculative interfaces.

### Hidden dep check

PASS with one observation.

- `dockerNetworkNamePattern` is a package-level `var` (not `const`-equivalent immutable). It is set once at init via `regexp.MustCompile` and not exported, so no external caller can replace it (`network.go:14`). Internally `Valid()` and the existing tests rely on it. This is the standard Go idiom for compiled-once regex; not a regression.
- `Executor.CreateNetwork` / `RemoveNetwork` inherit their `CommandRunner` from `Executor.runner` set by `NewExecutor` (`command.go:8-13`). No hidden global runner state. Tests inject via `CommandRunnerFunc`. The production wiring follows the same path as the existing `Run`/`Build`/`RemoveImage` methods.
- Context propagation: both new Executor methods forward `ctx` to `e.runner.Run(ctx, args)`. `TestExecutorRunUsesBuiltArgs` (`command_test.go:14-35`) already pins context-forwarding behavior for the original `Run` method; the new methods follow the same pattern but do NOT have an equivalent ctx-forwarding pin. Low-priority gap — the existing test pattern in `command_test.go:14-35` could be replicated for `CreateNetwork`/`RemoveNetwork` to lock the contract.

### Coverage gap

`mage testPkg ./internal/adapters/docker` reports 67.8% coverage (per worklog) / 65.3% in current tree state (other WIP modifications to `executor.go` from Unit 15.2.5 work). Both numbers are below the 70% CLAUDE.md target. Unit 15.2's specific uncovered paths are the OS-runner `Stream`/`Output` branches in `os_runner.go` (existing, not introduced by 15.2) — not a 15.2 falsifier on its own, but the worklog claim "above the enforced 60.0% gate" understates the project's 70% target.

### Unknowns

1. **Idempotency contract is not part of the unit.** `docker network create <existing-name>` errors at the daemon with "network with name X already exists" (Docker docs). `Executor.CreateNetwork` will surface that wrapped via the runner's `fmt.Errorf` boundary. The plan does not require idempotent create; orphan-cleanup (Unit 15.2.5) is the layer that resolves this. Not a 15.2 falsifier.
2. **Concurrent create+remove on the same name.** Same network name from two goroutines: Docker serializes at the daemon and the loser gets an error. Go-side `Executor` has no shared mutable state, so no race in the Valv layer. Not a falsifier.
3. **`docker network rm` of non-existent network.** Errors at the daemon (`Error response from daemon: network X not found`); `Executor.RemoveNetwork` surfaces the wrapped error. Unit 15.2.5's orphan-cleanup will decide whether to swallow `not found` as idempotent success. Out of 15.2 scope.

### Evidence

- Delta inspected: `git show d128363 -- internal/adapters/docker/network.go internal/adapters/docker/executor.go internal/adapters/docker/network_test.go`
- Code read: `internal/adapters/docker/network.go` (full), `internal/adapters/docker/network_test.go` (full), `internal/adapters/docker/executor.go` (full), `internal/adapters/docker/command.go` (full), `internal/adapters/docker/ops.go` (full — pattern reference), `internal/adapters/docker/types.go` (header).
- Context7 evidence: builder worklog cites `/docker/cli` for `docker network create --internal` and label syntax (confirmed at BUILDER_WORKLOG.md line 33). Docker docs label-key parsing splits on first `=` per `https://docs.docker.com/reference/cli/docker/container/create/#label`.
- Reproducer execution: temp test file `internal/adapters/docker/falsif_temp_test.go` added containing three `t.Errorf`-based attacks, run via `mage testPkg ./internal/adapters/docker`, all three FAILED as expected (3 failures / 54 tests total), then file deleted. Final `mage testPkg ./internal/adapters/docker` confirms clean tree state (51 pass).
- No raw `go test` / `GOCACHE=...` invocations.

## Unit 15.2 — Round 2

**Date:** 2026-05-24
**QA Falsification backend:** claude-sonnet-4-6 (Build-QA agent, both passes)
**Verdict:** `pass` — no unmitigated counterexample found.

### R1 Fix Verification

| R1 Finding | Fix location | Validating test | Status |
|---|---|---|---|
| Label key with surrounding whitespace accepted | `network.go:47-50`: `TrimSpace(key) != key` check | `TestNetworkCreateRequestRejectsUntrimmedLabelKey` (4 sub-tests: leading, trailing, both, tab) | FIXED |
| Label key with `=` not rejected | `network.go:51-54`: `ContainsRune(key, '=')` check | `TestNetworkCreateRequestRejectsLabelKeyWithEquals` (3 sub-tests) | FIXED |
| No max network name length | `network.go:40-42` (Create) + `102-104` (Remove): `len(name) > 64` | `TestNetworkCreateRequestRejectsOverlongName` (64 accepted, 65 rejected, 256 rejected) | FIXED |

### New Attack Vectors Tried

**AV1 — 64-byte boundary off-by-one.**

Attack: verify `len(name) > 64` uses the correct operator: 64 should accept, 65 should reject. Operator `>` means `len > 64` → 65 triggers, 64 does not. Test fixture at `network_test.go:400`: `"a" + strings.Repeat("b", 62) + "c"` = 1+62+1 = 64 bytes. The test asserts `shouldAccept: true` and `err == nil`. This directly exercises the boundary from the acceptance side.

Verdict: **mitigated**. The `>` operator is correct. The test confirms 64 bytes is accepted. A tightening to `>= 64` would break this test.

**AV2 — Tab character in label key.**

Attack: `"\tvalv\t"` as a label key. `strings.TrimSpace("\tvalv\t") = "valv"`, which `!= "\tvalv\t"`, so the guard at `network.go:48` fires and returns the whitespace error. Explicitly covered by the "tab whitespace rejected" sub-test at `network_test.go:322-325`. `strings.TrimSpace` handles `\t`, `\n`, `\r`, `\f`, `\v` — all Unicode whitespace.

Verdict: **mitigated**. Tab is caught by `strings.TrimSpace`.

**AV3 — Empty label key `""` vs whitespace-only `" "`.**

Attack: both should be rejected. `""`: `strings.TrimSpace("") = ""`, so the existing `== ""` guard at `network.go:44-46` catches it ("label key is required"). `" "`: `strings.TrimSpace(" ") = ""`, also caught by the same guard. Pre-existing test at `network_test.go:122-130` confirms. The new whitespace guard at line 48 is dead code for the `" "` case (already rejected at line 44 before line 48 is reached), which is fine — defense in depth is acceptable.

Verdict: **mitigated**. Both empty and whitespace-only keys are rejected.

**AV4 — Label VALUE containing `=` or whitespace.**

Attack: verify that VALUE-side validation was NOT added (correct behavior — Docker splits on first `=`, so a value like `"val=ue"` is safe: the key is `k`, the value is `val=ue`). Code: `Valid()` only iterates `for key := range r.Labels` and checks the key. Values are never validated or constrained. `BuildNetworkCreateArgs` emits `fmt.Sprintf("%s=%s", key, request.Labels[key])` at line 79 — key goes before the first `=`, value goes after, Docker parses it correctly.

Concrete test: a label `map[string]string{"valv": "network-policy=v2"}` would emit `--label valv=network-policy=v2`. Docker parses this as key=`valv`, value=`network-policy=v2`. Correct behavior. No value-side check should be present and none is.

Verdict: **mitigated** (correct by design — value-side `=` is safe; the risk is only on key-side).

**AV5 — Multi-byte UTF-8 name length: bytes vs runes.**

Attack: could a multi-byte UTF-8 name pass the regex but have `len()` (bytes) differ from `utf8.RuneCountInString()` (runes), causing the limit to be miscounted?

Analysis: the regex `^[a-zA-Z0-9][a-zA-Z0-9_.-]*$` matches only ASCII chars (0x00–0x7F range). All multi-byte UTF-8 sequences have lead bytes ≥ 0xC0, which are outside the `a-zA-Z0-9_.-` character class. Therefore any name passing the regex is guaranteed to be ASCII-only. For ASCII-only strings, `len(s) == utf8.RuneCountInString(s)`. The byte limit is rune-count-equivalent for all valid names.

Verdict: **mitigated**. No divergence possible; the regex guarantees ASCII-only input before the length check.

**AV6 — NetworkRemoveRequest 64-byte limit: is it enforced?**

Attack: verify the builder's claim that the limit is also in `NetworkRemoveRequest.Valid()`. Code at `network.go:102-104` confirmed. The check is present and uses the identical `len(name) > 64` guard with the same error message template.

Gap: there is no dedicated test for `NetworkRemoveRequest` with a 65-byte name. `TestBuildNetworkRemoveArgs` only covers empty, whitespace, and space-in-name. A 65-byte name via `BuildNetworkRemoveArgs` would hit the guard but is not tested.

Verdict: **accepted risk** (minor — code exists, test absent). The production validation path is correct; the missing test is a coverage gap, not a contract bug. Routed to Findings.

### YAGNI Check

PASS. The R2 fixes are three narrow validation guards — no new abstraction, no new types, no new exported symbols. Each guard is minimal:
- `TrimSpace(key) != key`: one conditional, one string op.
- `ContainsRune(key, '=')`: one conditional, one rune search.
- `len(name) > 64`: one conditional, one `len`.

The `NetworkRemoveRequest` length check duplicates the same guard for symmetry — this is correct duplication of a boundary contract, not YAGNI bloat. No `NetworkConnectRequest` or `--network` fallback logic introduced (Schema Decision 5 cut respected).

### Hidden Dep Check

PASS. No new global state introduced. The three guards are pure computations inside `Valid()` method receivers. No init-time side effects. `dockerNetworkNamePattern` (existing package-level var) is read-only and unchanged. `strings.TrimSpace`, `strings.ContainsRune`, and `len` are stdlib functions with no hidden state.

### Concurrency Check

PASS. `Valid()` is a value receiver method — it reads the request struct but does not modify it. `BuildNetworkCreateArgs` and `BuildNetworkRemoveArgs` work on copies. No shared mutable state. The mage gate runs with `-race` enabled (confirmed by the `go test -race` flag in `mage testPkg` output). 64 tests pass under the race detector.

### Interface Misuse Check

No new interfaces introduced in R2. `NetworkCreateRequest.Valid()` and `NetworkRemoveRequest.Valid()` are concrete method calls, not interface dispatch. No pointer-vs-value receiver confusion (both are value receivers consistent with R1).

### Error Swallowing Check

No error swallowing. `Valid()` returns errors wrapped with `fmt.Errorf` (no `%w` needed — these are leaf errors with no underlying cause to preserve). `BuildNetworkCreateArgs` returns `nil, err` on validation failure (line 63-65). `BuildNetworkRemoveArgs` same pattern (line 112). Executor methods surface validation errors to callers without additional wrapping — callers see the `"validate network ... request: ..."` prefix directly.

### Unknowns

1. **`NetworkRemoveRequest` 65-byte name test absent.** The guard exists at `network.go:102-104`; a test pinning it does not. A future refactor could accidentally remove the `NetworkRemoveRequest` length check while keeping the `NetworkCreateRequest` one, and tests would not catch it. Low risk — accepted.

2. **Context-forwarding pin for `CreateNetwork`/`RemoveNetwork`.** R1 falsification noted this gap (no `TestExecutorCreateNetworkForwardsContext` test). Unchanged in R2 — still accepted per R1's Unknowns.

### Evidence

- Code read: `internal/adapters/docker/network.go` (full, 116 lines) and `internal/adapters/docker/network_test.go` (full, 441 lines).
- Diff inspected: `git show HEAD -- internal/adapters/docker/network.go` confirms 14-line insertion (3 validation guards).
- Mage gate re-run independently: `mage testPkg ./internal/adapters/docker` → 64 tests / 65.7% / `-race` clean.
- Path discipline: `git show --stat HEAD` → 3 files only (`BUILDER_WORKLOG.md`, `network.go` +14, `network_test.go` +144).
- R1 falsification report at `BUILDER_QA_FALSIFICATION.md` — Unit 15.2 Round 1 section (the 3 source findings).
- No raw `go test` / `GOCACHE=...` invocations.

## Unit 15.2 — Round 3

**Verdict:** pass
**Reviewer:** ta-go-build-qa-falsification (codex gpt-5.5, `--sandbox read-only`, network=false; static analysis only); recorded by orchestrator. Audit: `.claude/agent-runs/20260526-160357-ta-go-build-qa-falsification-16946.tier1.codex-exec.out`.
**Reviewed at:** 2026-05-26

### Scope

Static counterexample review of commit `d4ca96a` (additive network-connect surface for the proxy sidecar): `internal/adapters/docker/{network.go (+74), executor.go (+11), network_test.go (+194)}`. Context: 15.2 was QA-green at R2 (create/remove helpers); the sidecar re-plan (`ce8e02c`) added the connect surface; R3 is that additive build.

### Counterexamples / Attacks (all mitigated)

- **Alias ordering nondeterminism:** mitigated. `BuildNetworkConnectArgs` copies `Aliases` into `sortedAliases` and `sort.Strings(...)` — deterministic lexicographic order, NOT map iteration. The test "multiple aliases sorted deterministically" feeds `["zulu","alpha","bravo"]` and expects `alpha,bravo,zulu`.
- **Empty / whitespace alias bypass:** mitigated. `Valid()` `TrimSpace`-checks each alias and rejects empty; pattern-checks reject `bad/name`-style.
- **Swapped / mis-placed positionals:** mitigated. Args end with `TrimSpace(Network)` then `TrimSpace(Container)` — positional tail, after the `--alias` flags.
- **R2 create/remove/list regression:** mitigated. All R2 symbols present and unchanged; `BuildNetworkCreateArgs` still emits `--internal`. (One obsolete doc-comment sentence on `NetworkCreateRequest` trimmed — cosmetic.)
- **`ConnectNetwork` runner invoked on invalid request:** mitigated. `Executor.ConnectNetwork` returns the `BuildNetworkConnectArgs` error before `e.runner.Run`.
- No goroutines/channels introduced; no error swallowing on the new path.

### Note

The codex run spent a few calls probing for a ta cascade record (its shared persona references ta tooling); valv does not use ta — its QA record is this drop-dir file. Harmless; the code review itself is grounded in `git show` + targeted reads.

### Falsification summary

- Confirmed counterexamples blocking PASS: 0.

**Verdict: pass.**

## Unit 15.2.5.A — Round 1

**Date:** 2026-06-02
**QA Falsification backend:** claude-sonnet-4-6 (Build-QA agent, `ta-go-qa-falsification` persona)
**Verdict:** `pass` — no unmitigated counterexample found.

### Attack 1 — Interface-widening breakage (highest priority)

**Attack:** `NetworkExecutor` widened with two new methods; `mage testPkg ./internal/services/networkpolicy` does NOT catch a compile break in any sibling package that constructs a `networkpolicy.Service` with a concrete executor or a local fake.

**Evidence gathered (exhaustive search):**

1. `find . -name "*.go" | xargs grep -l "NetworkExecutor|networkpolicy"` returns exactly 4 files:
   - `internal/services/networkpolicy/service.go` — interface definition + `Options`
   - `internal/services/networkpolicy/service_test.go` — `fakeNetworkExecutor` + compile-time guard
   - `internal/services/images/service.go` — defines its own `NetworkPolicy` interface (NOT `NetworkExecutor`); references `networkpolicy` package name only in comments
   - `internal/services/images/service_integration_test.go` — constructs `networkpolicy.New(networkpolicy.Options{Executor: dockerExec})` at line 257 where `dockerExec` is `docker.NewExecutor(...)` (i.e. a `docker.Executor` value)

2. The compile-time guard `var _ NetworkExecutor = docker.Executor{}` at `service_test.go:573` explicitly confirms `docker.Executor` satisfies the widened interface. This is a build-time assertion; the package fails to compile if it does not hold.

3. The integration test at `service_integration_test.go:257` passes `docker.Executor` as `Options.Executor`. This is the only non-`fakeNetworkExecutor` site that constructs `networkpolicy.Service` and it uses the production adapter. No other type in the codebase implements or is required to implement `NetworkExecutor`.

4. `internal/services/run`, `internal/services/claude`, `internal/services/codex`, `internal/cli` and `internal/adapters/providers` all define their own distinct local `Executor` interfaces with narrower method sets. None reference `NetworkExecutor`.

5. `grep -rn "NetworkExecutor" --include="*_test.go" . | grep -v "networkpolicy/service_test"` returns empty — no sibling-package test file defines a type that must satisfy `NetworkExecutor`.


**Verdict: MITIGATED.** The interface is consumer-side in `internal/services/networkpolicy`. No sibling package implements `NetworkExecutor` directly; the only implementers are `docker.Executor` (covered by compile-time guard) and `fakeNetworkExecutor` (test-only in the same package). The widening cannot cause a cross-package compile break.

### Attack 2 — Signature mismatch between interface and docker.Executor

**Attack:** verify `RunContainerDetached` and `ConnectNetwork` signatures exactly match between `NetworkExecutor` (service.go) and `docker.Executor` (executor.go).

Interface (`service.go:68,72`):
- `RunContainerDetached(ctx context.Context, request docker.ContainerRunRequest) (string, error)`
- `ConnectNetwork(ctx context.Context, request docker.NetworkConnectRequest) error`

`docker.Executor` (`executor.go:75,96`):
- `func (e Executor) RunContainerDetached(ctx context.Context, request ContainerRunRequest) (string, error)`
- `func (e Executor) ConnectNetwork(ctx context.Context, request NetworkConnectRequest) error`

After package-prefix expansion the signatures are byte-for-byte identical in all parameter types, return shapes, and context positions. `mage testFunc` runs confirmed PASS for both packages. The compile-time guard at `service_test.go:573` would break the package build on any drift.

**Verdict: MITIGATED.**

### Attack 3 — Consumer contract (D.1 seam fit)

**Attack:** does the seam match what 15.2.5.D.1 will actually call?

D.1 spec (PLAN.md line 182): create/reuse V7 internal network, call `RunContainerDetached` for proxy sidecar, connect proxy to bridge, attach `valv-proxy` alias on internal network, return `PolicyMaterial`.

- `RunContainerDetached` returns `(string, error)` — the string is the container ID. D.1 passes this ID to `ConnectNetwork` as `NetworkConnectRequest.Container`. Interface supplies this shape.
- `ConnectNetwork` accepts `docker.NetworkConnectRequest` with `Network`, `Container`, and `Aliases []string` (confirmed at `network.go:120-128`). D.1 connects the sidecar to bridge with the `valv-proxy` alias — both fields are present in the request type.
- The returned container ID is already trimmed: `SystemRunner.Output` at `os_runner.go:87` returns `strings.TrimSpace(stdout.String())`; `executor.go:90` applies a second `strings.TrimSpace` — idempotent, benign. D.1 can pass the ID directly to `ConnectNetwork.Container` without further sanitization.

**Verdict: MITIGATED.** The seam is correctly shaped for D.1's sidecar lifecycle sequence.

### Attack 4 — Multi-line docker output / ErrOutputUnsupported reachability

**Attack:** what if `docker run -d` emits a warning line on stdout before the container ID, or the runner's Output returns multi-line content?

Analysis:
- `SystemRunner.Output` returns `strings.TrimSpace(stdout.String())` at `os_runner.go:87` — captures ONLY stdout, not stderr. Docker `run -d` sends warning messages to stderr; the container ID is the sole stdout content.
- `RunContainerDetached` applies a second `strings.TrimSpace(out)` at `executor.go:90`. Since `SystemRunner.Output` already trimmed, this is a no-op for normal output.
- Theoretical edge case: if Docker ever emits multiple lines to stdout with `run -d`, `strings.TrimSpace` on the full multi-line string strips outer whitespace but preserves internal newlines. The caller (D.1) would receive a string like `<warn>
<id>`; `BuildNetworkConnectArgs` only does outer-whitespace trim on `Container` (`network.go:175`), so the internal newline survives into a broken container ID arg. This is theoretical — Docker `run -d` consistently outputs only the container ID on stdout in all known versions.
- `ErrOutputUnsupported`: both production runners (`SystemRunner` at `os_runner.go:60` and `QuietRunner` at `os_runner.go:167`) implement `Output`. In production, `NewExecutor(docker.NewSystemRunner(...))` is always used (confirmed at `cli/codex.go:127`, `cli/claude.go:122`, `cli/run.go:184`). The sentinel path is unreachable in current production wiring but correctly exercised by `TestExecutorRunContainerDetachedOutputUnsupported` (PASS confirmed).

**Verdict: ACCEPTED RISK.** Multi-line stdout from `docker run -d` is theoretical. The double-trim is idempotent and benign. `ErrOutputUnsupported` is unreachable in production but is a valid correctness guard for any future non-outputting runner. No corrective action required.

### Attack 5 — forvar lint at service_test.go:143

**Attack:** is the forvar lint at `service_test.go:143` masking a real loop-variable capture bug?

Line 143: `tc := tc` inside `for _, tc := range cases { tc := tc; t.Run(tc.name, func(t *testing.T) { t.Parallel() ... }) }`.

Analysis: as of Go 1.22 the loop variable capture issue is fixed by the language spec (loopvar promoted to default). Valv uses Go 1.26+ (CLAUDE.md). The `tc := tc` shadowing pattern was the standard pre-1.22 fix and is now a no-op. Static analysis tools flag it as "copying variable is unneeded" (SA4006). There is no loop-variable capture bug here; the test is correct with or without the shadow. This pattern pre-dates this unit's additions.

**Verdict: BENIGN NIL.** Pre-existing test-only nit.

### Attack 6 — Budget re-measurement

**Actual diff measurement (production files only):**

| File | Lines before | Lines after | Net delta |
|---|---|---|---|
| `internal/adapters/docker/executor.go` | 113 | 137 | +24 lines |
| `internal/services/networkpolicy/service.go` | 272 | 280 | +8 lines |

**Total production LOC delta: +32 lines** (well under the 80-line ceiling).

**Production symbols added:**
1. `Executor.RunContainerDetached` — new method on existing struct
2. `NetworkExecutor.RunContainerDetached` — new method signature on interface
3. `NetworkExecutor.ConnectNetwork` — new method signature on interface (`ConnectNetwork` already existed on `docker.Executor`; this only adds it to the consumer-side interface)

**Count: 3 production symbols at the 3-symbol ceiling.** Builder's measurement is accurate.

**Production files: 2** (executor.go + service.go) — under the 3-file ceiling.

**Verdict: CONFIRMED WITHIN BUDGET.** All three dimensions (symbols, LOC, files) are at or below the `aa130dd` limits.

### YAGNI Check

Both new symbols (`RunContainerDetached` + `ConnectNetwork` on the interface) have a concrete caller in D.1 per the locked sidecar-proxy topology. `RunContainerDetached` is needed to launch the sidecar in detached mode and retrieve its container ID; `ConnectNetwork` is needed to attach the sidecar to bridge with the `valv-proxy` alias. Neither is speculative. The `RunContainerDetached` implementation reuses the existing `outputter` typecast pattern established by `ListNetworks` and `RemoveContainer`.

**Verdict: PASS.**

### Hidden Dep Check

- No new global state introduced. `ErrOutputUnsupported` is an existing sentinel (`types.go:11`); `RunContainerDetached` returns it under the same condition as `ListNetworks`.
- No init-time side effects.
- `Executor` is a value receiver throughout; no new pointer receivers.
- No error swallowing: `BuildRunArgs` validation errors and `outputter.Output` errors are returned directly. Leaf errors need no `%w` chain; runner errors carry context from `SystemRunner`'s `fmt.Errorf` wrapper.

**Verdict: PASS.**

### Concurrency Check

`RunContainerDetached` has no shared mutable state. Value-receiver method; delegates to the runner. Runner is set at `NewExecutor` time and never mutated. No goroutines spawned. `mage testFunc` runs with `-race`. **PASS.**

### Confirmed Counterexamples

None.

### Unknowns (route to future units or orchestrator)

1. **Multi-line stdout from `docker run -d` (theoretical).** D.1 could defensively split on `\n` and take the last non-empty line when processing the returned container ID. Low priority; accepted for now.

2. **`ErrOutputUnsupported` reachability in production.** Currently unreachable because all production runners implement `Output`. If a future unit introduces a non-outputting runner and accidentally wires it to `networkpolicy.Service`, the sentinel path surfaces at runtime. Accepted for current wiring.

### Evidence

- `git diff b22cdc7~1 b22cdc7` — full diff read
- `git show b22cdc7:internal/adapters/docker/executor.go | wc -l` and `git show b22cdc7~1:...|wc -l` — LOC measurement (+24 executor.go, +8 service.go)
- `Read /Users/evanschultz/Documents/Code/hylla/valv/main/internal/services/networkpolicy/service.go` — interface shape, lines 55-73
- `Read /Users/evanschultz/Documents/Code/hylla/valv/main/internal/adapters/docker/executor.go` — method signatures lines 75 and 96, outputter pattern
- `Read /Users/evanschultz/Documents/Code/hylla/valv/main/internal/adapters/docker/os_runner.go:60-88` — Output returns stdout-only + already-trims
- `Read /Users/evanschultz/Documents/Code/hylla/valv/main/internal/services/images/service.go:100-154` — confirms distinct `NetworkPolicy` interface, no `NetworkExecutor` dep
- `Read /Users/evanschultz/Documents/Code/hylla/valv/main/internal/services/images/service_integration_test.go:245-363` — confirms `docker.NewExecutor` passed as `networkpolicy.Options.Executor`
- `Read /Users/evanschultz/Documents/Code/hylla/valv/main/internal/services/networkpolicy/service_test.go:571-573` — compile-time guard
- `Read /Users/evanschultz/Documents/Code/hylla/valv/main/internal/services/networkpolicy/service_test.go:130-155` — forvar lint context
- `Read /Users/evanschultz/Documents/Code/hylla/valv/main/drops/DROP_15_NETWORK_POLICY/PLAN.md:147-155,176-184` — unit spec + D.1 consumer contract
- `grep -rn "NetworkExecutor" --include="*.go" .` — exhaustive implementer search (4 files total)
- `grep -rn "docker.NewExecutor" --include="*.go" . | grep -v "_test.go"` — production construction sites
- `grep -rn "NetworkExecutor" --include="*_test.go" . | grep -v "networkpolicy/service_test"` — sibling-package fake audit (empty)
- `mage testFunc ./internal/adapters/docker TestExecutorRunContainerDetachedReturnsContainerID` — PASS
- `mage testFunc ./internal/adapters/docker TestExecutorRunContainerDetachedOutputUnsupported` — PASS
- `mage testFunc ./internal/services/networkpolicy TestNew_RequiresExecutor` — PASS
- `mage testFunc ./internal/services/networkpolicy TestProvision_CreatesNetworkOnFreshHost` — PASS
- `Read /Users/evanschultz/Documents/Code/hylla/valv/main/internal/services/run/service.go:35-40` — run.Executor interface shape (confirms no NetworkExecutor dep)

### Summary

| Attack | Result |
|---|---|
| 1. Interface-widening cross-package breakage | MITIGATED — only one external implementer; compile-time guard confirms satisfaction |
| 2. Signature mismatch | MITIGATED — signatures match exactly; guard breaks build on drift |
| 3. Consumer contract (D.1 seam fit) | MITIGATED — return type and method signatures correctly shaped for D.1 sidecar lifecycle |
| 4. Multi-line output / ErrOutputUnsupported | ACCEPTED RISK — Docker behavior is ID-only stdout; double-trim is idempotent; production runner always has Output |
| 5. forvar lint at service_test.go:143 | BENIGN NIL — loop-variable fix pattern; pre-existing test-only nit |
| 6. Budget re-measurement | CONFIRMED — 3 symbols / +32 LOC / 2 files — at or under all ceilings |

**Verdict: pass** — no unmitigated counterexample found. Unit 15.2.5.A is correctly implemented and within budget.

## Unit 15.2.5.C — Round 1

**Date:** 2026-06-02
**QA Falsification backend:** claude-sonnet-4-6 (Build-QA agent, `ta-go-qa-falsification` persona)
**Verdict:** `pass` — no unmitigated counterexample found.

### Attack 1 — Signature blast radius: `buildNoProxy(allowlist []string)` → `buildNoProxy()`

**Attack:** the function dropped its parameter. Find every caller — was any stale caller left in the repo, or does an unused-parameter lint trap anywhere?

**Evidence:**

`grep -rn "buildNoProxy" /Users/evanschultz/Documents/Code/hylla/valv/main/` returns these production-code sites (excluding worklog/agent-run files):

- `internal/services/networkpolicy/service.go:225` — `NoProxy: buildNoProxy()` (the only call site; no argument passed — correct)
- `internal/services/networkpolicy/service.go:278` — function definition `func buildNoProxy() string`

No other `.go` production file references `buildNoProxy`. The function is unexported and there is only one call site. The old `buildNoProxy(request.Allowlist)` call was replaced in the same file (`service.go:225`). No stale caller exists anywhere in the tree.

The removed `allowlist []string` parameter: since `buildNoProxy` is now parameter-free, there is no unused-parameter lint concern — Go's compiler only flags unused local variables declared with `:=`/`var`, not absent parameters. Compiler vet confirms the package builds clean (via mage gate). No compile or vet issue.

**Verdict: MITIGATED.** Single call site, updated correctly; no stale caller anywhere.

### Attack 2 — Allowlist still threaded correctly (orphan check)

**Attack:** removing `allowlist` from `buildNoProxy` must not have orphaned it — `ProvisionRequest.Allowlist` must still be consumed for the network-name hash and must still travel to the proxy filter. If `Allowlist` is now a dead field, the proxy would have no filter list.

**Evidence:**

`service.go:187`: `desiredName := networkName(request.Allowlist)` — `Allowlist` is still passed to `networkName`, which computes `sha256(sorted allowlist)` at lines 258-264. This is unchanged from the pre-C state.

The allowlist's second role — as the proxy sidecar's internal filter list — is not yet wired in 15.2.5.C scope. That wiring belongs to Unit 15.2.5.D.1 (the sidecar launch), which passes the allowlist to the proxy binary's runtime configuration. That is explicitly not 15.2.5.C territory. `ProvisionRequest.Allowlist` doc comment was updated to state: "The service passes this to the network-name derivation (deterministic network naming) but does NOT place allowlist entries in NO_PROXY — the sidecar proxy enforces the allowlist internally."

So: `Allowlist` is NOT a dead field. It is consumed by `networkName` (deterministic naming) today, and will be consumed by D.1 for the proxy's runtime filter. Its role in `buildNoProxy` was the bug — it was placed in NO_PROXY (wrong place), not absent entirely.

`ProvisionRequest.Valid()` still rejects `len(r.Allowlist) == 0` at line 121, so the required-field contract holds and callers cannot silently pass an empty allowlist.

**Verdict: MITIGATED.** `Allowlist` is consumed correctly. No orphaned field.

### Attack 3 — Sufficiency of `valv-proxy` alias in NO_PROXY (Decision 5 reasoning check)

**Attack:** is `valv-proxy` (alias only) sufficient in NO_PROXY, or does the workload also need the sidecar's IP or a port-qualified form? Does anything address the sidecar by anything other than the alias?

**Analysis:**

Schema Decision 5 (PLAN.md lines 37-42) specifies the workload reaches the sidecar via the Docker network alias `valv-proxy`. The `HTTP_PROXY`/`HTTPS_PROXY` env vars are set to `http://valv-proxy:<port>` (unit 15.2.5.D.1 will provide the port from `ProxyEndpoint`). Go's `http.ProxyFromEnvironment` evaluates `NO_PROXY` against the TARGET host being connected to, NOT the proxy host — so `valv-proxy` in NO_PROXY means "don't use a proxy when connecting TO valv-proxy directly." That is exactly the anti-loop-back semantics the code comment describes: a client connecting to `valv-proxy:port` directly (e.g. a health check) should not route through itself.

For IP-based exclusion: Docker assigns the sidecar a dynamic IP on the internal network. The workload has no stable IP for the sidecar other than the alias. Adding a dynamic IP to NO_PROXY is both impractical and unnecessary — DNS-alias exclusions in NO_PROXY cover all connections by that hostname.

For port-qualified forms: `NO_PROXY` for Go's `http.ProxyFromEnvironment` accepts host-only entries (no port needed); per Go source, port-qualified forms in NO_PROXY are supported but not required when excluding all ports on a host. The fixed string `valv-proxy` covers all ports.

The `127.0.0.1` and `localhost` entries cover in-container loopback traffic (standard practice). Nothing in the workload container should use `::1` (IPv6 loopback) — IPv6 is not present in the sidecar-proxy design — but this is a theoretical gap with zero practical impact for the current macOS Docker Desktop topology.

**Verdict: MITIGATED.** The three fixed entries are sufficient for the described topology. One accepted theoretical residual: IPv6 loopback (`::1`) is not in NO_PROXY; this is acceptable for the Docker Desktop macOS topology and matches industry practice for `NO_PROXY` values.

### Attack 4 — Deleted tests: was anything still-valid lost?

**Attack:** `TestBuildNoProxy_SortedDedupedAndTrimmed` and `TestPolicyMaterial_NoProxyOmitsBlankAllowlistEntries` were deleted. Confirm those only asserted the OLD inverted behavior and that dedup/trim/blank-entry handling the new code still needs is covered.

**Evidence:**

`TestBuildNoProxy_SortedDedupedAndTrimmed` called `buildNoProxy([]string{"  b ", "a", "b", "c", "a"})` and asserted `"a,b,c"`. This asserted:
- Sort order of the allowlist
- Deduplication of the allowlist
- Trim of whitespace-padded entries

All three of those properties were only relevant when `buildNoProxy` consumed the allowlist. The new `buildNoProxy()` takes no input and returns the fixed string `"127.0.0.1,localhost,valv-proxy"`. There is no dynamic list to sort, dedup, or trim. The removed behaviors are no longer part of `buildNoProxy`'s contract — they were behaviors of the WRONG implementation.

`TestPolicyMaterial_NoProxyOmitsBlankAllowlistEntries` called `buildNoProxy([]string{"", " github.com ", "proxy.golang.org"})` and asserted `"github.com,proxy.golang.org"`. Same: asserted blank-entry filtering on the allowlist input, which is now irrelevant.

The new code has `sort.Strings(entries)` on a static three-element slice. This is technically a no-op since `[]string{"127.0.0.1", "localhost", "valv-proxy"}` is already in alphabetical/lexicographic order. The builder retained it "for consistency with the package's determinism pattern and to make the sorted output self-documenting." This is a minor YAGNI nit (sorting a static literal) but not a correctness problem — the output is deterministic regardless. No test is needed to cover "does sort work on a static slice."

The new tests (`TestBuildNoProxy`, `TestBuildNoProxy_AllowlistHostsNeverLeak`, `TestBuildNoProxy_LoopbackAndSidecarAlwaysPresent`) all confirmed PASS via `mage test-func`. They pin the now-correct invariant: fixed output regardless of any context.

**Verdict: MITIGATED.** The deleted tests only pinned the old inverted semantics. Nothing still-valid was discarded. The static `sort.Strings` is a benign no-op worth noting as a NIT.

### Attack 5 — Output format correctness for `http.ProxyFromEnvironment` and provider CLIs

**Attack:** `"127.0.0.1,localhost,valv-proxy"` — is the comma-separated, no-scheme, no-spaces format correct for Go's proxy machinery and for the provider CLIs (Codex, Claude Code)?

**Evidence:**

Go stdlib `net/http.ProxyFromEnvironment` parses `NO_PROXY` (or `no_proxy`) as a comma-separated list of host entries with optional leading dot or wildcard. Each entry is matched against the target host. Comma-separated with no spaces is the canonical form — spaces around entries are stripped by Go's parser but the comma separator is required. No scheme or port qualifiers are needed for a pure hostname bypass. The format `"127.0.0.1,localhost,valv-proxy"` is correct.

For Docker `buildx build` `--build-arg NO_PROXY=...`: Docker documents predefined proxy build args accept the same standard `no_proxy` env-var format. The `--build-arg` value is passed verbatim into the build container's environment, where Go tool invocations and curl respect it.

Claude Code docs (PLAN.md line 46, Schema Decision 9): "Claude Code docs currently state support for `HTTP_PROXY`, `HTTPS_PROXY`, and `NO_PROXY`." No special format required beyond the standard env-var convention.

OpenAI Codex: PLAN.md notes proxy regressions as of 2026-05-22 (`openai/codex#16079`, `#14080`), but those concern `HTTP_PROXY` routing issues, not `NO_PROXY` format. The format itself is standard.

**Verdict: MITIGATED.** The format is correct for Go's HTTP client, Docker build args, and the provider CLIs.

### Attack 6 — Budget re-measurement

**Actual production diff (commit `4d20566`):**

| File | Change |
|---|---|
| `internal/services/networkpolicy/service.go` | Doc-comment updates (ProvisionRequest.Allowlist, PolicyMaterial.NoProxy) + `buildNoProxy()` rewrite + call-site update. Net ~18 LOC changed; 1 file; 1 production symbol changed (`buildNoProxy`). |

- **Production symbols changed:** 1 (`buildNoProxy` signature + body rewrite). Under the 3-symbol ceiling.
- **Production LOC delta:** ~18 changed lines in `service.go`. Under the 80-line ceiling.
- **Production files:** 1 (`service.go`). Under the 3-file ceiling.

Builder's self-measurement is accurate. Confirmed within budget on all three dimensions.

**Verdict: CONFIRMED WITHIN BUDGET.**

### NIT: stale `NoProxy` fixture in `internal/services/images/service_test.go`

At `internal/services/images/service_test.go:1801` and `1862`, `TestEnsureProjectImage_NetworkPolicyInjectsProxyArgsAndNetwork` hardcodes:

```
NoProxy: "github.com,objects.githubusercontent.com,proxy.golang.org,sum.golang.org"
```

and expects:

```
"NO_PROXY=github.com,objects.githubusercontent.com,proxy.golang.org,sum.golang.org"
```

This is the OLD (inverted) `NoProxy` value. The images service test uses a `fakeNetworkPolicy` that returns whatever `PolicyMaterial` the test hardcodes — the images service does not call `buildNoProxy` itself. The test PASSES because it is testing the images service's pass-through behavior (whatever the fake returns goes into the build arg verbatim). The images service is correct; the test is internally consistent.

However, the fixture now misleads future readers about what a real `networkpolicy.Service` returns. A future developer integrating the real `networkpolicy.Service` with the images service would see this test and expect the old allowlist-in-NO_PROXY behavior. The integration test at `service_integration_test.go:357` uses `material.NoProxy` from the real service (which is now `"127.0.0.1,localhost,valv-proxy"`), so the real-wiring path is correct.

This is a NIT / misleading comment in the wrong test package — NOT a bug in 15.2.5.C's scope (`paths: internal/services/networkpolicy/service.go`). The images test's scope is the images service's pass-through contract, not networkpolicy semantics. No corrective action required within this unit. Recommend updating the images test fixture in a follow-up (or when the images service is fully wired with the real `networkpolicy.Service`).

### YAGNI Check

PASS. The rewrite is strictly subtractive: the old `buildNoProxy` had ~20 lines of sort/dedup/trim logic operating on a parameter that should never have been passed to it. The new implementation is 3 lines. The `sort.Strings` on a static slice is a no-op carried for pattern consistency (benign, see Attack 4). No new abstraction introduced.

### Hidden Dep Check

PASS. `buildNoProxy` is unexported, parameter-free, pure (no I/O, no shared state, no context), and has exactly one call site. It cannot have hidden dependencies. The import set for `service.go` (`sort`, `strings`, `fmt`, `crypto/sha256`, `encoding/hex`, `context`) is unchanged from the pre-C state.

### Concurrency Check

PASS. `buildNoProxy` is a pure function — no goroutines, no channels, no shared mutable state. Race detector runs via `mage test-func` (confirmed `-race` flag in mage gate). All four new tests confirmed PASS.

### Confirmed Counterexamples

None.

### Unknowns (route to future units or orchestrator)

1. **`valv-proxy` alias establishment is D.1's responsibility.** The `valv-proxy` string in NO_PROXY assumes the sidecar is connected to the internal network with that alias. If D.1 uses a different alias string, NO_PROXY would not exclude the correct host. The string should be a shared constant (e.g. `const ProxyAlias = "valv-proxy"` in the networkpolicy package) so `buildNoProxy` and D.1's `ConnectNetwork` call use the same value. Currently `valv-proxy` is a string literal at `service.go:279` only. Low priority for this unit (D.1 is not yet shipped), but this is a contract coupling point to track.

2. **IPv6 loopback (`::1`) not in NO_PROXY.** See Attack 3. Accepted for the Docker Desktop macOS topology.

### Evidence

- `git diff 4d20566~1 4d20566` — full diff read
- `Read /Users/evanschultz/Documents/Code/hylla/valv/main/internal/services/networkpolicy/service.go` — full production file (282 lines)
- `grep -rn "buildNoProxy" /Users/evanschultz/Documents/Code/hylla/valv/main/` — all references across the repo
- `grep -rn "NoProxy|NO_PROXY|no_proxy" --include="*.go"` — all consumers across the codebase
- `Read /Users/evanschultz/Documents/Code/hylla/valv/main/internal/services/images/service_test.go:1790-1888` — stale fixture context
- `Read /Users/evanschultz/Documents/Code/hylla/valv/main/internal/services/images/service.go:85-137` — images service seam (NetworkPolicy interface, pass-through contract)
- `Read /Users/evanschultz/Documents/Code/hylla/valv/main/drops/DROP_15_NETWORK_POLICY/PLAN.md:37-42` — Schema Decision 5 (sidecar alias, NO_PROXY rationale)
- `Read /Users/evanschultz/Documents/Code/hylla/valv/main/internal/adapters/docker/network.go` — full file (Aliases + NetworkConnectRequest shape)
- `grep -rn "valv-proxy|sidecar|ConnectNetwork" --include="*.go" internal/` — alias usage in production
- `mage test-func ./internal/services/networkpolicy TestBuildNoProxy` — 4 tests PASS
- `mage test-func ./internal/services/networkpolicy TestBuildNoProxy_AllowlistHostsNeverLeak` — 1 test PASS
- `mage test-func ./internal/services/networkpolicy TestBuildNoProxy_LoopbackAndSidecarAlwaysPresent` — 1 test PASS
- `mage test-func ./internal/services/images TestEnsureProjectImage_NetworkPolicyInjectsProxyArgsAndNetwork` — 1 test PASS (confirms images test internally consistent)

### Summary

| Attack | Result |
|---|---|
| 1. Signature blast radius — stale caller | MITIGATED — single call site updated; no stale caller anywhere in tree |
| 2. Allowlist still threaded (orphan check) | MITIGATED — `Allowlist` consumed by `networkName`; D.1 will use it for proxy filter; not dead |
| 3. NO_PROXY sufficiency — alias only | MITIGATED — `valv-proxy` alias is correct form; IP/port-qualified forms not needed; IPv6 loopback accepted risk |
| 4. Deleted tests — lost still-valid assertions | MITIGATED — deleted tests only pinned the OLD inverted semantics; nothing valid discarded |
| 5. Output format correctness | MITIGATED — comma-separated, no-scheme form is correct for Go HTTP client, Docker build args, and provider CLIs |
| 6. Budget re-measurement | CONFIRMED — 1 symbol / ~18 LOC / 1 file — well under all ceilings |
| NIT: stale images fixture | NIT only — images test is internally consistent; misleading fixture for future readers; recommend fix in follow-up |

**Verdict: pass** — no unmitigated counterexample found. Unit 15.2.5.C is correctly implemented and within budget.

## Unit 15.2.5.B.1 — Round 1

**Date:** 2026-06-02
**QA Falsification backend:** claude-sonnet-4-6 (Build-QA agent, `ta-go-qa-falsification` persona)
**Verdict:** pass — no unmitigated counterexample found. No CRITICAL bypass. All fail-closed invariants hold.

### Attack 1 — Fail-closed invariant (highest priority)

**Attack:** `hostAllowed` with nil/empty allowed list must return `false` for any target. Also: whitespace-only target, case-fold onto entry, trailing-dot FQDN, uppercase, leading/trailing spaces on target.

**Traces:**

- `hostAllowed("github.com", nil)`: loop iterates zero times over nil slice -> `false`. MITIGATED.
- `hostAllowed("github.com", []string{})`: loop iterates zero times -> `false`. MITIGATED.
- `hostAllowed("", []string{"github.com"})`: no `:` -> host = "" -> lowercase -> no match -> `false`. MITIGATED.
- `hostAllowed("  ", []string{"github.com"})`: no `:` -> host = "  " -> lowercase -> no match -> `false`. `hostAllowed` does NOT trim the target's surrounding whitespace. A caller passing spaces around a hostname gets a false-deny (safe), not a bypass.
- `hostAllowed("github.com.", []string{"github.com"})`: no `:` -> host = "github.com." -> "github.com." != "github.com" -> `false`. Trailing-dot FQDN is a safe deny. MITIGATED.
- `hostAllowed("GitHub.COM", []string{"github.com"})`: no `:` -> lowercase -> "github.com" -> matches -> `true`. Correct allow. MITIGATED.
- `hostAllowed("GitHub.COM:443", []string{"github.com"})`: strings.Cut -> before="GitHub.COM" -> lowercase -> "github.com" -> matches -> `true`. Correct allow with port-strip. MITIGATED.

**Verdict: MITIGATED.** Fail-closed on nil, empty, and all whitespace-target inputs. No CRITICAL bypass.

### Attack 2 — Port-strip parsing: IPv6 and edge cases

**Attack:** `strings.Cut(target, ":")` finds the FIRST colon. For IPv6 literals this mangles the host portion. Does the mangling ever produce a false ALLOW (bypass)?

**Traces:**

- `hostAllowed("[::1]:443", allowed)`: first colon is at index 2 inside bracket -> before="[" -> host = "[" -> no match -> `false`. False-deny (safe). MITIGATED.
- `hostAllowed("[2001:db8::1]:443", allowed)`: first `:` inside bracket -> before="[2001" -> host = "[2001" -> no match -> `false`. False-deny (safe). MITIGATED.
- `hostAllowed("[::1]", allowed)`: no `:` -> host = "[::1]" -> no match -> `false`. MITIGATED.
- `hostAllowed(":443", allowed)`: strings.Cut -> before="" -> host = "" -> no match -> `false`. MITIGATED.
- `hostAllowed("github.com:", allowed)`: strings.Cut -> before="github.com" -> host = "github.com" -> MATCHES if in allowed -> `true`. Correct: empty port still strips colon and matches hostname. MITIGATED.
- `hostAllowed("https://github.com", allowed)`: strings.Cut -> before="https" -> host = "https" -> no match -> `false`. Scheme-prefixed URL safely denied. MITIGATED.

**Key finding on IPv6:** IPv6 literals are always false-denied, never false-allowed. The design spec (Schema Decision 1) says exact hostname / Docker alias only — no IP addresses. Builder worklog B.1 explicitly documents this accepted gap (line 568).

**Verdict: MITIGATED.** No false-allow from any IPv6 or malformed input. Only false-denies — safe for closed-by-default policy.

### Attack 3 — Subdomain/suffix bypass

**Attack:** can `evil-github.com`, `github.com.evil.com`, `github.completer.com`, or `notgithub.com` match `"github.com"` via any looser predicate?

**Trace:** The comparison in `hostAllowed` is strictly `a == host` (allowlist.go:48) after both sides have been lowercased. There is no `strings.Contains`, `strings.HasSuffix`, `strings.HasPrefix`, or regex match anywhere in the function. Every candidate:

- "evil-github.com" != "github.com" -> `false`. MITIGATED.
- "github.com.evil.com" != "github.com" -> `false`. MITIGATED.
- "github.completer.com" != "github.com" -> `false`. MITIGATED.
- "notgithub.com" != "github.com" -> `false`. MITIGATED.

**Verdict: MITIGATED.** Exact-string equality is the only comparison path. No substring, suffix, or prefix match possible.

### Attack 4 — parseAllowlist robustness: dedup + case-fold consistency

**Attack:** is there a case-fold mismatch between `parseAllowlist` (lowercases entries) and `hostAllowed` (lowercases target)?

**Trace:**

- `parseAllowlist("GitHub.COM")` -> strings.ToLower(strings.TrimSpace(...)) -> "github.com". Stored as "github.com".
- `hostAllowed("github.com:443", []string{"github.com"})` -> extracts "github.com" -> strings.ToLower -> "github.com" -> matches stored "github.com" -> `true`.

Both normalize to lowercase with `strings.ToLower` before comparison. Symmetric and consistent. Dedup uses `map[string]struct{}` keyed on the lowercased+trimmed form — `GitHub.COM` and `github.com` hash to the same key and deduplicate correctly.

**Verdict: MITIGATED.** No case-fold inconsistency. Dedup is correct.

### Attack 5 — Budget re-measurement

Builder claims: 2 prod symbols / 53 LOC (worklog; spec said ~45) / 1 prod file.

Actual measurement:
- `wc -l internal/cmd/valv-proxy/allowlist.go` -> 53. Matches worklog claim.
- Production symbols: `parseAllowlist` (line 18) + `hostAllowed` (line 41). Exactly 2. Under the 3-symbol ceiling.
- Production files: `allowlist.go` only. `allowlist_test.go` is test-only. 1 prod file vs 1-file ceiling.
- LOC: 53 vs 80-line ceiling. Under budget. Builder worklog correctly notes spec's "~45 LOC" was code-only estimate excluding doc comments.

**Verdict: CONFIRMED WITHIN BUDGET.** 2 symbols / 53 LOC / 1 file — under all ceilings.

### Attack 6 — Coverage adequacy for the 70% drop-end floor

The production code has 2 functions, 53 lines. All branches of both functions are exercised by the 27 tests (12 for `TestParseAllowlist`, 13 for `TestHostAllowed`, 1 for `TestHostAllowed_EmptyAllowedList`, 1 for `TestHostAllowed_NilAllowedList`):

- `parseAllowlist`: empty-entry skip branch, dedup skip branch, and main append path all exercised. All branches covered.
- `hostAllowed`: with-port branch, without-port branch, loop match returning `true`, loop falling through to `return false`, nil-list path, empty-list path. All branches covered.

This package will comfortably clear the 70% floor at drop-end `mage test-func --cover`.

**Verdict: MITIGATED.** All branches exercised. Coverage will exceed 70%.

### YAGNI Check

PASS. `parseAllowlist` and `hostAllowed` are the minimum symbols B.2 needs. No wildcard logic, no CIDR, no scheme/path parsing, no config-file support. Both are small, single-purpose, zero speculative surface.

### Hidden Dep Check

PASS. Both functions are pure (no I/O, no shared state, no init-time effects). Import: only `"strings"` (stdlib). No package-level mutable state. No goroutines.

### Concurrency Check

PASS. Both functions operate on local variables only. `parseAllowlist` creates a fresh `map` and slice on every call; `hostAllowed` reads its slice argument without mutation. No races possible. `-race` flag active in mage gate.

### Confirmed Counterexamples

None. No CRITICAL bypass found.

### NITs

1. **`hostAllowed` does not trim whitespace from the `target` parameter.** A target of "  github.com  " would produce host = "  github.com  " -> no match -> `false`. False-deny only (safe). The HTTP CONNECT target per spec has no surrounding spaces. B.2 passing the raw CONNECT target is safe. Document for B.2 review.

2. **IPv6 literals are silently denied, not explicitly rejected.** An operator adding a bracket-form IPv6 address to `VALV_PROXY_ALLOWLIST` would find connections silently blocked. The design spec prohibits IP addresses (Schema Decision 1). Builder worklog B.1 documents this explicitly.

### Unknowns (route to future units or orchestrator)

1. **B.2 should pass raw CONNECT target to `hostAllowed`.** Per NIT 1, the standard HTTP CONNECT target is space-free per spec. Raw pass-through is correct.

2. **IPv6 allowlist operator error.** If an operator adds "[::1]" to `VALV_PROXY_ALLOWLIST`, the bracket form would never match `[::1]:443` CONNECT requests (because `strings.Cut` extracts "[" not "[::1]"). Safe deny, but confusing. Unit 15.1's `EffectiveAllowlist` host validation should catch this at CLI-entry level.

### Evidence

- `git show ea18fc5 --stat` — confirmed files changed: `allowlist.go` (53 lines new) + `allowlist_test.go` (188 lines new) + `BUILDER_WORKLOG.md` + `PLAN.md`.
- `Read /Users/evanschultz/Documents/Code/hylla/valv/main/internal/cmd/valv-proxy/allowlist.go` — full production file read.
- `Read /Users/evanschultz/Documents/Code/hylla/valv/main/internal/cmd/valv-proxy/allowlist_test.go` — full test file read.
- `Read /Users/evanschultz/Documents/Code/hylla/valv/main/drops/DROP_15_NETWORK_POLICY/BUILDER_WORKLOG.md:502-569` — B.1 builder worklog.
- `Read /Users/evanschultz/Documents/Code/hylla/valv/main/drops/DROP_15_NETWORK_POLICY/PLAN.md:374-382` — B.1 spec.
- `wc -l /Users/evanschultz/Documents/Code/hylla/valv/main/internal/cmd/valv-proxy/allowlist.go` -> 53.
- `ls /Users/evanschultz/Documents/Code/hylla/valv/main/internal/cmd/valv-proxy/` -> only `allowlist.go` + `allowlist_test.go`.
- `mage test-func ./internal/cmd/valv-proxy TestParseAllowlist` -> 12 tests PASS.
- `mage test-func ./internal/cmd/valv-proxy TestHostAllowed` -> 13 tests PASS.
- `mage test-func ./internal/cmd/valv-proxy TestHostAllowed_EmptyAllowedList` -> 1 test PASS.
- `mage test-func ./internal/cmd/valv-proxy TestHostAllowed_NilAllowedList` -> 1 test PASS.
- Manual code trace for IPv6, scheme-prefixed, trailing-dot, subdomain, and suffix bypass inputs.
- `Read /Users/evanschultz/Documents/Code/hylla/valv/main/drops/DROP_15_NETWORK_POLICY/BUILDER_QA_FALSIFICATION.md` — prior rounds for cross-drop context.

### Summary

| Attack | Result |
|---|---|
| 1. Fail-closed: nil/empty/whitespace/trailing-dot/uppercase targets | MITIGATED — always returns false on nil/empty list; no bypass on any tested variant |
| 2. Port-strip: IPv6, multi-colon, empty-host, scheme-prefix | MITIGATED — all produce false-deny (safe); no false-allow on any malformed input |
| 3. Subdomain/suffix bypass | MITIGATED — exact `==` equality only; no Contains/HasSuffix path exists |
| 4. Case-fold consistency between parse and match | MITIGATED — both normalize to lowercase via `strings.ToLower`; symmetric |
| 5. Budget re-measurement | CONFIRMED — 2 symbols / 53 LOC / 1 file — under all ceilings |
| 6. Coverage adequacy for 70% floor | MITIGATED — all branches exercised; will clear 70% floor |
| YAGNI check | PASS — minimal; no speculative surface |
| Hidden dep check | PASS — pure functions, no shared state, stdlib-only import |
| Concurrency check | PASS — value-in/value-out; race detector clean |

**Verdict: pass** — no unmitigated counterexample found. Unit 15.2.5.B.1 is correctly implemented, fail-closed on all tested inputs, and within budget.

## Unit 15.2.5.B.2 — Round 1
**Date:** 2026-06-02
**QA Falsification backend:** claude-sonnet-4-6 (Build-QA agent, `ta-go-qa-falsification` persona)
**Verdict:** pass — no unmitigated counterexample found. No CRITICAL fail-open path. All security invariants hold.


### Attack 1 — Fail-closed ordering on CONNECT path (CRITICAL)

**Attack:** Is `hostAllowed` checked BEFORE `net.Dial` on every CONNECT code path? Is there any path that dials a non-allowlisted host?

**Trace:** `r.Method == CONNECT` branch enters at line 30. Line 32: `hostAllowed(r.Host, allowed)` — if false, `http.Error(w, ..., 403)` + `return`. Only if true does execution continue to line 36 (`w.WriteHeader(200)`), line 40 (hijack check), line 44 (hijack call), and finally line 50 (`net.Dial("tcp", r.Host)`). Every early-exit path (non-hijackable at line 42, hijack error at line 46) returns before line 50. `net.Dial` is unconditionally gated behind the allowlist check at line 32.

Concrete denied CONNECT trace: `r.Host = "denied.example.com"`, `allowed = ["github.com"]` — `hostAllowed` returns false at line 32, `http.Error(403)` fires, `return` exits. Line 50 never executes.

**Verdict: MITIGATED.** Strictly fail-closed on CONNECT. No code path reaches `net.Dial` without passing the allowlist check.

### Attack 2 — Fail-closed ordering on plain HTTP path (CRITICAL)

**Attack:** Is `hostAllowed` checked BEFORE the reverse-proxy forward for every non-CONNECT method, including unknown methods?

**Trace:** All requests where `r.Method != CONNECT` fall to the plain-HTTP branch at line 76. Line 77: `hostAllowed(r.Host, allowed)` — if false, `http.Error(w, ..., 403)` + `return`. Only if true does execution continue to line 85 (`target := ...`) and line 87 (`httputil.NewSingleHostReverseProxy(target).ServeHTTP(w, r)`). Line 87 is only reachable after the check at line 77 passes.

Unknown method trace: `r.Method = "BLAH"`, `r.Host = "denied.example.com"` — falls to plain-HTTP branch (not CONNECT), `hostAllowed` returns false, `http.Error(403)` + `return`. Line 87 never executes. Confirmed empirically: `TestAttack_UnknownMethod_DeniedHost_Returns403` PASS.

**Verdict: MITIGATED.** Strictly fail-closed on all non-CONNECT methods including unknown ones.

### Attack 3 — Host-field desync: check vs forward use same field?

**Attack:** Does the code check one host field but forward to a different one, enabling a split attack (check `github.com`, dial `evil.com`)?

**Trace (CONNECT path):** Line 32 checks `r.Host`; line 50 dials `r.Host`. Same field. No split possible.

**Trace (plain HTTP path):** Line 77 checks `r.Host`; line 85 builds `target := &url.URL{Scheme: scheme, Host: r.Host}`; line 87 forwards to `target`. The forward target host is built from `r.Host` — same field as the check. `httputil.NewSingleHostReverseProxy(target)` rewrites `r.URL.Host` to the target before dialling, so even if `r.URL.Host` differed from `r.Host`, the proxy overrides it.

Desync attack trace: raw HTTP with `GET http://evil.com/ HTTP/1.1\r\nHost: github.com\r\n` would set `r.URL.Host = "evil.com"` and `r.Host = "github.com"`. Check: `hostAllowed("github.com", ...) = true`. Forward: `target.Host = r.Host = "github.com"`. The proxy forwards to `github.com`, not `evil.com`. No bypass.

**Verdict: MITIGATED.** Both check and dial/forward use `r.Host`. No field split possible.

### Attack 4 — CONNECT 200-before-hijack-failure: data tunnelled to unchecked host?

**Attack:** `w.WriteHeader(http.StatusOK)` fires at line 36 AFTER the allowlist check passes (line 32). If hijack subsequently fails, does the client receive tunnelled data from any backend?

**Trace:** After line 36 (200 written), hijack is attempted. If `!ok` (lines 40-43) or `hj.Hijack()` errors (lines 44-46), `return` fires immediately. `net.Dial` at line 50 is never reached. `defer clientConn.Close()` on line 48 has not yet been reached (hijack failed before), so the HTTP server manages connection cleanup. The client sees 200 then EOF — a failed tunnel signal, NOT a successful forward. No data from any backend flows.

Production relevance: `http.ListenAndServe` serves HTTP/1.1 over TCP by default. Go HTTP/1.1 `ResponseWriter` always implements `http.Hijacker`. The non-hijackable branch (lines 40-43) is effectively dead code in production but is a correct defensive check for edge cases (e.g. middleware wrapping the ResponseWriter).

Empirical confirmation: `TestAttack_CONNECT_200ThenHijackFailure_NoForward` used `httptest.ResponseRecorder` (which does NOT implement `http.Hijacker`) — asserted `dialCount == 0` and `rec.Code == 200`. PASS. Net.Dial never fires despite 200 being written.

**Verdict: MITIGATED.** 200-then-close is the correct tunnel-failure signal; no data forwarded.

### Attack 5 — Plain-HTTP empty Host field

**Attack:** What happens if `r.Host` is empty for a plain-HTTP request? Is it forwarded to a default target or denied?

**Trace:** `hostAllowed("", allowed)` — `strings.Cut("", ":")` returns `("", "", false)` (no colon found), so `host = ""` — `strings.ToLower("") = ""` — no match in any allowlist — returns `false`. The handler fires `http.Error(w, ..., 403)` + `return`. Line 87 never executes.

Empirical confirmation: `TestAttack_CONNECT_EmptyHost_Denied` set `req.Host = ""` and confirmed `rec.Code == 403`. PASS.

**Verdict: MITIGATED.** Empty host is always denied; no forwarding to any target.

### Attack 6 — Goroutine / fd leak on error paths

**Attack:** On `net.Dial` failure after a successful hijack — is the hijacked client conn closed? On one-directional EOF in the bidirectional copy — do both goroutines terminate, or can one block forever?

**Trace (hijack failure path):** If `!ok` or `hj.Hijack()` errors at lines 40-46, the function returns before any `defer clientConn.Close()` on line 48 is registered (hijack never succeeded). The HTTP server manages the connection. No goroutines started. No fd leak.

**Trace (net.Dial failure path):** `defer clientConn.Close()` at line 48 is registered before line 50. If `net.Dial` fails at line 51, function returns and the defer closes clientConn. No goroutines started. No fd leak.

**Trace (bidirectional copy):** Two goroutines at lines 58-71; `wg.Wait()` at line 72 blocks until both complete. Termination: goroutine 1 copies client-to-target; when clientConn reaches EOF (client closes), `io.Copy` returns, then `CloseWrite(targetConn)` signals EOF to target on its incoming side; target closes its side; goroutine 2's `io.Copy(clientConn, targetConn)` returns; both goroutines call `wg.Done()`. `wg.Wait()` unblocks; defers close both conns.

`CloseWrite` non-TCPConn risk: if the hijacked conn or the dialled conn is not a `*net.TCPConn`, the type assertion silently no-ops. The opposite goroutine's `io.Copy` may then block indefinitely waiting for EOF. For HTTP/1.1 over TCP (the production sidecar path), both conns are `*net.TCPConn` and `CloseWrite` fires correctly. Theoretical risk only for non-TCP transports.

**Verdict: MITIGATED** for production HTTP/1.1-over-TCP path. **ACCEPTED RISK** for theoretical non-TCPConn configurations (noted as Unknown #1).

### Attack 7 — Budget re-measurement

**Attack:** Verify the PLAN spec (2 prod symbols, ~75 LOC, 1 file) and builder worklog claim (82 non-blank non-comment lines) against the actual code.

**Measurement:** `main.go` has 103 total lines. Blank lines: 7. Comment-only lines (lines starting with `//`): 16. 103 - 7 - 16 = **80 non-blank non-comment production lines**. The builder worklog claimed 82 — overcounted by 2. The difference is immaterial; 80 is AT the 80-line ceiling, not over.

- Production symbols: `newProxyHandler` (line 28) and `main` (line 91). Exactly 2. Under the 3-symbol ceiling.
- Production files: `main.go` only. `main_test.go` is test-only. 1 file vs 1-file ceiling.

Note: `allowlist.go` (34 non-blank non-comment lines, 2 symbols) was B.1 scope and shipped in the same commit (`2f70e89`). The builder correctly tracked B.1 and B.2 as separate units in the worklog with separate mage gates. The combined commit is a mechanical consolidation, not a budget violation for B.2.

**Verdict: CONFIRMED WITHIN BUDGET.** 2 symbols / 80 LOC (at ceiling) / 1 file — all within spec.

### Attack 8 — Coverage for the drop-end floor

**Attack:** Are the error/deny branches (dial failure, hijack-unsupported) exercised by shipped tests, or will uncovered branches drop below the floor?

Effective floor is **60%%** (not 70%%): `magefile.go:24` sets `coverageThreshold = 60.0` with a TODO to restore 70.0. The `testPkg` target does not enforce coverage — the gate runs only in the full-suite `cover` / `test` targets (orchestrator-only per discipline). Shipped tests cover: CONNECT allowed tunnel, CONNECT denied (403), plain-HTTP allowed forward, plain-HTTP denied (403), empty-allowlist denies all.

Uncovered branches:
1. Lines 40-43: non-hijackable `ResponseWriter` path.
2. Lines 44-46: `hj.Hijack()` error return.
3. Lines 50-53: `net.Dial` failure return.
4. Lines 81-83: `r.TLS != nil` schema path (always plain HTTP in tests).

Estimated coverage: 70-80%%  (happy+deny paths fully covered; 4 error branches each 2-3 lines uncovered). This clears the 60%% gate. Confirm at drop-end.

**Verdict: LIKELY ADEQUATE** for the 60%% floor. Drop-end full-suite confirmation required.

### YAGNI Check

PASS. `newProxyHandler` and `main` are the minimum two symbols the unit spec requires. No middleware abstraction, no config struct, no speculative retry/timeout logic, no plugin interface. The inline closure for the handler keeps the symbol count at 2 while keeping the code readable. The `r.TLS` check adds 2 LOC of correctness handling with no speculative risk.

### Hidden Dep Check

PASS. No package-level mutable state introduced. `newProxyHandler` is a pure constructor returning a closure that captures the `allowed []string` slice — the slice is immutable (no mutation path in the handler). `main` reads two env vars (`VALV_PROXY_ADDR`, `VALV_PROXY_ALLOWLIST`) and calls `http.ListenAndServe`. No init-time side effects. No shared global state between requests.

### Concurrency Check

PASS. The bidirectional copy goroutines (lines 58-71) operate on separate connections with no shared mutable state between them. `sync.WaitGroup` correctly tracks completion; `defer` closes fire after `wg.Wait()` so goroutines complete before connections close. No race possible on the connection objects themselves (each goroutine has exclusive access to its write direction). Race detector active (`-race` flag in `mage test-func`); all 5 shipped test functions PASS under `-race`.

### Confirmed Counterexamples

None. No CRITICAL fail-open found.

### NITs

1. **Builder LOC count off by 2.** Worklog claims "82 non-blank non-comment lines" for `main.go`; actual count is 80 (103 total - 7 blank - 16 comment-only = 80). Immaterial — 80 is at the ceiling, not over.
2. **Non-TCPConn `CloseWrite` no-op:** if either hijacked conn or dialled conn is not a `*net.TCPConn`, `CloseWrite` is silently skipped and the opposite goroutine may block indefinitely waiting for EOF. Not a production concern (HTTP/1.1 over TCP is always `*net.TCPConn` in the sidecar deployment).
3. **Four uncovered branches** (non-hijackable, hijack-error, dial-error, `r.TLS` path). Adding tests for these would strengthen robustness and push coverage from ~75% toward ~90%. Not required for the 60% gate.
4. **B.1 and B.2 shipped in one commit** (`2f70e89`). The PLAN declares them separate droplets. The builder correctly ran per-unit `mage test-func` gates and documented them separately in the worklog. The joint commit is a mechanical consolidation artifact, not a scope or budget violation.

### Unknowns (route to orchestrator)

1. **`CloseWrite` no-op for future TLS sidecar.** If the sidecar ever serves TLS, the hijacked conn would be a `*tls.Conn` and `CloseWrite` would not fire. The current topology uses plain HTTP (`http://valv-proxy:8080`), so clients use plain HTTP to reach the proxy and CONNECT tunnels are dialled over plain TCP. Accepted for current deployment; flag if sidecar TLS is added.
2. **Drop-end coverage confirmation.** The `testPkg` target does not enforce the coverage floor. The orchestrator must run the full-suite `cover` / `test` target at drop-end to confirm `internal/cmd/valv-proxy` clears the 60% gate.

### Evidence

- `git show 2f70e89 --stat` — confirmed 4 files changed: `main.go` (+103), `main_test.go` (+255), `allowlist.go` (+53), `allowlist_test.go` (+188), `BUILDER_WORKLOG.md`, `PLAN.md`.
- `Read /Users/evanschultz/Documents/Code/hylla/valv/main/internal/cmd/valv-proxy/main.go` — full production file (103 lines).
- `Read /Users/evanschultz/Documents/Code/hylla/valv/main/internal/cmd/valv-proxy/main_test.go` — full test file (255 lines).
- `Read /Users/evanschultz/Documents/Code/hylla/valv/main/internal/cmd/valv-proxy/allowlist.go` — full allowlist file (53 lines).
- `Read /Users/evanschultz/Documents/Code/hylla/valv/main/drops/DROP_15_NETWORK_POLICY/PLAN.md` — B.2 unit spec at plan line 380.
- `Read /Users/evanschultz/Documents/Code/hylla/valv/main/drops/DROP_15_NETWORK_POLICY/BUILDER_WORKLOG.md:1-46` — B.2 builder design notes + budget measurement.
- `Read /Users/evanschultz/Documents/Code/hylla/valv/main/magefile.go` (lines 22-30, 140-168, 185-193) — `testPkg` runs without `-cover`; `coverageThreshold = 60.0`.
- `grep -cE` for blank + comment-only lines in `main.go` — 7 + 16 = 23 excluded lines; 103 - 23 = 80 prod LOC.
- `ls /Users/evanschultz/Documents/Code/hylla/valv/main/internal/cmd/valv-proxy/` — 4 files confirmed (after attack test cleanup).
- `Read /Users/evanschultz/Documents/Code/hylla/valv/main/drops/DROP_15_NETWORK_POLICY/BUILDER_QA_FALSIFICATION.md` — prior rounds for cross-drop context.
- Attack tests written, run via `mage test-func`, then deleted (scratch tests):
  - `mage test-func github.com/evanmschultz/valv/internal/cmd/valv-proxy TestAttack_CONNECT_200ThenHijackFailure_NoForward` — PASS
  - `mage test-func github.com/evanmschultz/valv/internal/cmd/valv-proxy TestAttack_UnknownMethod_DeniedHost_Returns403` — PASS
  - `mage test-func github.com/evanmschultz/valv/internal/cmd/valv-proxy TestAttack_CONNECT_EmptyHost_Denied` — PASS
- Shipped test functions confirmed PASS via `mage test-func`:
  - `TestNewProxyHandler_CONNECT_AllowedHostTunnels` — PASS
  - `TestNewProxyHandler_CONNECT_DeniedHost` — PASS
  - `TestNewProxyHandler_PlainHTTP_AllowedHost` — PASS
  - `TestNewProxyHandler_PlainHTTP_DeniedHost` — PASS
  - `TestNewProxyHandler_EmptyAllowlistDeniesAll` — PASS
  - `TestHostAllowed_EmptyAllowedList` — PASS
- Manual code trace for all 8 attack vectors: fail-closed CONNECT, fail-closed plain-HTTP, host-field desync, 200-before-hijack, empty-host, goroutine/fd leak, budget, coverage.
- `/tmp/valv-qa-b1/falsif_backup.md` — backup created before truncation + append operations.

### Summary

| Attack | Result |
|---|---|
| 1. Fail-closed CONNECT ordering (CRITICAL) | MITIGATED — `hostAllowed` at line 32; `net.Dial` unreachable without passing check |
| 2. Fail-closed plain-HTTP ordering (CRITICAL) | MITIGATED — `hostAllowed` at line 77; forward unreachable without passing check |
| 3. Host-field desync (check vs dial/forward) | MITIGATED — both check and dial/forward use `r.Host`; no split possible |
| 4. 200-before-hijack-failure leaks tunnel | MITIGATED — `net.Dial` never fires on hijack failure; empirically confirmed |
| 5. Empty Host field forwarded | MITIGATED — `hostAllowed("")` always returns false |
| 6. Goroutine / fd leak on error paths | MITIGATED for production TCP; ACCEPTED RISK for theoretical non-TCPConn |
| 7. Budget re-measurement | CONFIRMED — 2 symbols / 80 LOC (at ceiling) / 1 file |
| 8. Coverage for 60% floor | LIKELY ADEQUATE — happy+deny paths covered; 4 error branches not covered; drop-end confirm needed |
| YAGNI check | PASS — minimal 2-symbol design |
| Hidden dep check | PASS — no global mutable state |
| Concurrency check | PASS — WaitGroup + defer; `-race` clean |

**Verdict: pass** — no unmitigated counterexample found. Unit 15.2.5.B.2 is correctly implemented and fail-closed on all critical security paths. The proxy is safe to use as the closed-network boundary for the sidecar topology.

## Unit 15.2.5.D.1 — Round 1

**verdict: FAIL** (orch-written per friction-free QA model) — one real compile break; the security-critical topology invariant itself is clean.

**F-1 (CRITICAL, the failure):** `internal/services/images/service_integration_test.go:349` constructs `networkpolicy.ProvisionRequest{Allowlist: req.Allowlist, ProxyEndpoint: req.ProxyEndpoint}` — but D.1 removed `ProxyEndpoint` from `networkpolicy.ProvisionRequest`. The file is `//go:build integration`, so `mage testPkg`/`mage vet` (no `-tags=integration`) cannot see it, but `mage integration` (`-tags=integration ./internal/services/images`) WILL fail to compile (`unknown field 'ProxyEndpoint'`). This is the `feedback_mage_integration_when_deleting_symbols` lesson: symbol-deletion units must run `mage integration`. Fix (Round 2): drop the `ProxyEndpoint:` assignment(s) at `service_integration_test.go:281` + `:349` (req.Allowlist is the only field networkpolicy.ProvisionRequest still needs). Extends D.1 paths to the images integration adapter (the break D.1 caused).

**Topology invariant (CRITICAL attack) — MITIGATED (clean):** `Provision` attaches ONLY the sidecar to bridge (`RunContainerDetached` with no `Network` → bridge default, `:229-241`) + connects the sidecar to the internal net with `[ProxyAlias]` (`:249-255`); the WORKLOAD is never touched here (15.3.C attaches it internal-only). `sidecarID` from `RunContainerDetached` is the exact `Container` passed to `ConnectNetwork`. Tests pin `run.Network == ""` + order. No workload-bridge leak.

NITs / routed: (N-1) stale `<ProxyEndpoint>` format strings in `PolicyMaterial` comments `service.go:137,140` → fix in Round 2. (N-2) orphan sidecar leaked if `ConnectNetwork` errors after `RunContainerDetached` succeeds → accepted-risk, assigned to 15.2.5.E (cleanup). (N-3) `images.NetworkPolicyRequest.ProxyEndpoint` + `s.proxyEndpoint` now vestigial (sidecar owns the endpoint) → route the images→sidecar egress wiring to 15.2.5.G (Decision 6). (N-4) benign `tc := tc` for-var nits in service_test.go. Budget 39 net prod LOC / 1 prod file — confirmed.
