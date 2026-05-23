verdict: fail

# DROP_15 — Plan QA Falsification, Round 4

Round 4 falsification pass against the revised planner scope. Committed repo evidence was checked against Hylla artifact `github.com/evanmschultz/valv@main` pinned to `1759e64`; `git diff --name-only` is empty, so there are no uncommitted local deltas changing the attacked surfaces.

## 1. Counterexamples

### 1.1 A Linux or generic-CI green does not prove the supported Docker Desktop macOS topology

The plan still treats a tagged integration test as sufficient evidence for A1, but it does not require that test to execute on the supported Docker Desktop macOS topology. The actual acceptance text only says the test must use the shipped single-network topology and must block if `host.docker.internal` is unreachable on supported macOS (`drops/DROP_15_NETWORK_POLICY/PLAN.md:48`, `:151`, `:154`, `:185-188`, `:232`). That leaves a concrete false-green path: a Linux CI runner can pass the integration test without ever proving Docker Desktop macOS behavior.

External evidence cuts directly against treating Linux CI as equivalent. Docker Desktop documents `host.docker.internal` as a special DNS name for reaching host services from a container (`https://docs.docker.com/desktop/features/networking/networking-how-tos/`, "Connect a container to a service on the host"), while Docker's Linux examples use different host-access patterns such as host networking rather than the Docker Desktop DNS contract. That means "passes on CI" and "works on supported macOS topology" are not interchangeable pieces of evidence.

Narrow fix: add an explicit acceptance gate that Units 15.2.5 and 15.3 must be validated on Docker Desktop macOS, with Linux CI runs treated as non-evidence for A1. If that validation is manual rather than automated, say so explicitly in the acceptance text and in the builder notes.

### 1.2 `WriteAllowlistSection` still permits valid-TOML byte loss outside `[allowlist]`

Round 4 narrowed the writer contract, but it still preserves only `[tools]` and `[env]` blocks verbatim (`drops/DROP_15_NETWORK_POLICY/PLAN.md:33`, `:45`, `:98-99`, `:224`). That does not cover valid TOML comments, spacing, or alignment outside those two blocks. A concrete supported-shape file can still lose data while satisfying the current tests:

```toml
# repo-level note kept by operators
[allowlist] # keep this explanation
hosts = ["github.com"]   # inline rationale

# divider comment
[tools]
golangci-lint = { source = "github.com/golangci/golangci-lint/cmd/golangci-lint@latest", install = "go install" }
```

This file is UTF-8, LF, no BOM, and has no multi-line strings. Under the current contract, an implementation may rewrite `[allowlist]` and still legally drop or reformat the file preamble, the inline allowlist comment, or the divider comment before `[tools]`, because only `[tools]` and `[env]` are protected byte-for-byte. That is a fresh corruption path, not a repeat of the Round 3 unsupported-shape finding.

Narrow fix: make the supported-shape contract byte-preserving for every byte outside the lexical `[allowlist]` section span, or explicitly declare inline-comment / surrounding-comment allowlist forms unsupported and require a deterministic error for them.

### 1.3 Open-mode callers can still eagerly provision proxy infrastructure

The plan's scope cut says `valv run` is the only closed-default ship gate and that `valv claude` / `valv codex` remain open-mode callers (`drops/DROP_15_NETWORK_POLICY/PLAN.md:39`, `:46`, `:175-176`, `:216`, `:233`). But the actual open-mode acceptance only proves that open mode does not inject proxy env vars or an internal network into `ContainerRunRequest` (`drops/DROP_15_NETWORK_POLICY/PLAN.md:175-181`, `:226`). It does not forbid eager setup of the proxy daemon or managed network before the request is built.

That omission is material because the current repo already has one eager host-side helper in the Codex path: `PrepareRuntime` unconditionally constructs a `bridgeManager` before request assembly (`internal/adapters/providers/codex/runtime.go:134-170`), and that bridge immediately binds a listener and serves an HTTP endpoint rooted at `host.docker.internal` (`internal/adapters/providers/codex/bridge.go:42-68`). A DROP_15 implementation can therefore regress open-mode behavior in a way the current plan would still mark green: start the network-policy proxy or create the managed Docker network for every run, then simply omit `HTTP_PROXY` and `--network` when `open` is selected.

Narrow fix: add explicit acceptance that open mode and provider launchers do not invoke policy setup, do not create managed Docker networks, and do not start a host proxy process. Require tests to assert "setup path not called" rather than only asserting missing env/network fields on the final request.

## 2. YAGNI Pressure Check

### 2.1 `NetworkConnectRequest` is still a speculative abstraction in Round 4

Unit 15.2 still requires `NetworkConnectRequest`, `BuildNetworkConnectArgs`, and `Executor.ConnectNetwork` (`drops/DROP_15_NETWORK_POLICY/PLAN.md:120-124`). Round 4 simultaneously removes the only obvious same-drop need for post-create attach operations: no second-network fallback is allowed (`drops/DROP_15_NETWORK_POLICY/PLAN.md:35`, `:151`, `:188`, `:232`), and the proxy is host-local rather than another container that must be attached later.

I could not find a committed or planned DROP_15 caller that needs `docker network connect` once the fallback was cut. That makes the connect API a fresh YAGNI pressure point: an extra abstraction and test surface with zero stated in-drop consumer.

Narrow fix: cut `NetworkConnectRequest` / `BuildNetworkConnectArgs` / `Executor.ConnectNetwork` from DROP_15 unless the planner names the exact same-drop caller and why `CreateNetwork` plus existing run/build request fields are insufficient.

## 3. Hidden Dependency Check

### 3.1 Orphaned proxy cleanup is still implicit

Unit 15.2.5 describes a shared policy-material service that returns a cleanup handle (`drops/DROP_15_NETWORK_POLICY/PLAN.md:147`), and Unit 15.3 tests cleanup on success and failure paths (`drops/DROP_15_NETWORK_POLICY/PLAN.md:179-183`). What is still missing is cleanup-on-startup or any other idempotent reclamation rule for the case where the Valv process is killed after policy setup but before deferred cleanup runs.

That is a hidden dependency on a "cleanup always executes" world that the process model cannot guarantee. A concrete failure path is straightforward: `valv run` provisions proxy/network state, the parent process dies with `SIGKILL`, and the next invocation starts without any explicit rule for reclaiming old managed resources. Depending on naming/port strategy, the next run can fail on collision or silently leak more host processes.

Narrow fix: make orphan reclamation explicit in the plan. Either require startup sweep of stale managed resources, or require deterministic labels/metadata plus idempotent "reuse or replace" behavior, with tests covering stale-resource recovery.

## 4. Summary

FAIL. I did not construct new counterexamples against worktree-root detection or against the four built-in hosts for the plan's narrow `go install github.com/x/y` claim: the current toolchain default is still `GOPROXY=https://proxy.golang.org,direct` with `GOSUMDB=sum.golang.org`, and `go help private` still documents those as the public-module defaults. But the three issues above remain unmitigated, the `network connect` seam is still premature after the fallback cut, and proxy orphan cleanup is still an unstated dependency rather than an explicit acceptance gate.
