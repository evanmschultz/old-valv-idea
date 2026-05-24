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
