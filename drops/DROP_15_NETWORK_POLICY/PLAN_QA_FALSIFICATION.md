verdict: fail

# DROP_15 — Plan QA Falsification, Round 5

Round 5 falsification pass against the post-Round-4 plan text. Committed baseline was cross-checked against Hylla artifact metadata `github.com/evanmschultz/valv@main` pinned to `1759e64`; `git diff -- drops/DROP_15_NETWORK_POLICY/PLAN.md` is empty, so there is no uncommitted delta on the attacked plan surface.

## 1. Counterexamples

### 1.1 The byte-preservation contract still leaves the `[allowlist]` span boundary ambiguous

Decision 3 and Unit 15.1 now require `WriteAllowlistSection` to preserve every byte outside the lexical `[allowlist]` section span verbatim (`drops/DROP_15_NETWORK_POLICY/PLAN.md:33`, `:98-100`). But the plan still never defines where that lexical span ends. A supported-shape manifest can therefore admit two incompatible implementations:

```toml
[allowlist]
hosts = ["github.com"]

# separator that operators expect to survive
[tools]
golangci-lint = "github.com/golangci/golangci-lint/cmd/golangci-lint@latest"
```

Implementation A can treat the span as the `[allowlist]` header plus its key/value lines only, leaving the blank line and separator comment outside the span. Implementation B can treat the span as everything from the `[allowlist]` header through the byte immediately before the next top-level section header, consuming the blank line and separator comment as part of the rewritten span. Both implementations can claim compliance with the current wording, yet only one preserves the separator bytes. That makes the Round 4 golden-fixture assertion non-deterministic: two builders can satisfy the same plan text while producing different byte-for-byte outputs.

Narrow fix: define the span precisely. For example: the `[allowlist]` span starts at the first byte of the `[allowlist]` header line and ends immediately before the first byte of the next top-level `[section]` header, or EOF if `[allowlist]` is last. State explicitly whether intervening blank lines and comments belong to that span or must be preserved outside it.

## 2. YAGNI Pressure Check

### 2.1 No new YAGNI counterexample confirmed

Round 4's prior pressure point is addressed. Unit 15.2 now explicitly forbids `NetworkConnectRequest` in DROP_15 and keeps the Docker seam to create/remove only (`drops/DROP_15_NETWORK_POLICY/PLAN.md:120-125`). I did not find a new speculative abstraction in the Round 5 attack set.

## 3. Hidden Dependency Check

### 3.1 No new hidden dependency counterexample confirmed

Round 4's orphan-cleanup concern is now explicit. Unit 15.2.5 requires deterministic Valv-owned metadata on managed resources plus startup reclaim/reuse or cleanup, and it requires stale-resource tests (`drops/DROP_15_NETWORK_POLICY/PLAN.md:155-157`). The host-proxy tracking mechanism is still an implementation choice, but it is no longer an implicit dependency hidden from the plan.

## 4. Summary

FAIL. One new unmitigated counterexample remains: the `[allowlist]` lexical-span boundary is still underspecified, so the byte-preservation contract is not mechanically testable. The other Round 5 attacks did not produce new falsifiers: the open-mode "setup-not-called" seam is now explicit (`drops/DROP_15_NETWORK_POLICY/PLAN.md:177-186`), the macOS gate is committed as automate-or-manual-required (`:155`, `:192`), orphan cleanup is now explicit (`:156`), Docker's `--network` flag already attaches the container at create/run time so DROP_15 does not need a separate connect API, and the plan already scopes closed-default shipping to `valv run` plus overlay-build egress rather than provider launchers (`:39`, `:148-155`, `:177-193`).
