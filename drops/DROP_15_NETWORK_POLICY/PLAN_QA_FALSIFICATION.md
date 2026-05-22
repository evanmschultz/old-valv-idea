# DROP_15 — Plan QA Falsification, Round 1

- Verdict: fail
- Reviewed: `main/drops/DROP_15_NETWORK_POLICY/PLAN.md` @ `37ce350`

## Attacks

### A1 — `--internal` + `host.docker.internal` transport assumption
- Probe: attack the planner's core transport claim at `PLAN.md:27` and `PLAN.md:92-106` using Docker docs, local Docker CLI help, and current Valv host-bridge usage.
- Result: open, high risk
- Evidence:
  - `docker network create --help` documents `--internal` as `Restrict external access to the network`.
  - Context7 Docker docs say internal networks have "No external connectivity" and are isolated from the host's network interfaces.
  - Context7 Docker Desktop docs separately say `host.docker.internal` resolves to the host's internal IP from a container.
  - Current Valv code already assumes `host.docker.internal` from normal containers for MCP bridging: `internal/adapters/providers/codex/bridge.go:42-68`.
  - None of the Docker evidence loaded in this pass proves the intersection the plan relies on: that a container attached only to an `--internal` network can still resolve and reach `host.docker.internal`.
  - Local experiment was not possible in this session: `docker version` returned only the client version (`29.4.3`) and no running server, so I could not run the decisive container/network repro.
- Why this matters:
  - Unit 15.3's design is built around a host-local proxy on loopback reached through `host.docker.internal`. If that path does not survive `--internal`, the default closed mode cannot reach its own allowlist proxy.

### A2 — Runtime-only proxy design leaves DROP_12 overlay builds outside policy
- Probe: trace where Valv actually performs `go install` / `npm install -g` today and compare that path to Unit 15.3's runtime-only enforcement.
- Result: confirmed counterexample
- Evidence:
  - The plan scopes enforcement onto the post-DROP_13 run seam and injects proxy env only into the workload container: `drops/DROP_15_NETWORK_POLICY/PLAN.md:27`, `:95-97`.
  - DROP_12 overlay builds install project tools during image build, not during `valv run` container startup:
    - `internal/services/images/overlay.go:84-146` emits Dockerfile `RUN ["go","install", ...]` and `RUN ["npm","install","-g", ...]`.
    - `internal/adapters/docker/ops.go:9-102` shows image builds go through `docker buildx build`; the build path is separate from `ContainerRunRequest`.
  - The plan contains no unit that threads allowlist/proxy/network policy into `ImageBuildRequest` or the overlay build service.
- Counterexample:
  - A project with `.valv/tools.toml` declaring `ta = { source = "github.com/evanmschultz/ta@latest", install = "go install" }` triggers outbound network during `docker buildx build` before the runtime container even starts.
  - That egress is outside the proposed `--internal` runtime network and outside the host-local proxy env injection path.
- Conclusion:
  - "Closed by default" is false on the end-to-end user path that first builds a project overlay image, unless DROP_15 also covers image-build egress or explicitly narrows scope to "runtime container only."

### A3 — Go install host set is under-specified for exact-host closed mode
- Probe: attack the assumption that generic `go install` can be made to work by "proxy env injection" alone, and that exact-host allowlisting is operationally complete without a documented host set.
- Result: confirmed plan gap
- Evidence:
  - `go help environment` documents module-download controls as `GOPROXY`, `GOPRIVATE`, `GONOPROXY`, `GONOSUMDB`, `GOSUMDB`, and `GOAUTH`; it does not describe module resolution in terms of `HTTP_PROXY` / `HTTPS_PROXY`.
  - `go env` in this workspace reports:
    - `GOPROXY=https://proxy.golang.org,direct`
    - `GOSUMDB=sum.golang.org`
    - `GOAUTH=netrc`
  - Go source/help also states download behavior is configured by `GOPROXY`, `GOSUMDB`, `GOPRIVATE`, and related env vars (`go help environment`; `cmd/go/internal/modload/help.go` surfaced by repo-local grep).
  - The plan's allowlist model is exact-host only with no derived-host behavior: `drops/DROP_15_NETWORK_POLICY/PLAN.md:47`, `:97`, `:145`.
- Counterexample:
  - A public tool install such as `go install golang.org/x/tools/gopls@latest` does not match the repo's current `GOPRIVATE` pattern and therefore can require both the configured module proxy and the checksum database.
  - An allowlist that includes only `github.com` or only `proxy.golang.org` is insufficient; exact-host closed mode can still fail on `sum.golang.org` even when the HTTP proxy transport itself works.
- Conclusion:
  - The plan needs either:
    - explicit Go-host documentation / UX around required host entries,
    - automated host discovery/defaults for Go installs,
    - or a narrowed claim that closed-default does not guarantee generic DROP_12 Go tool installs without manual allowlist expansion.

### A4 — `WriteAllowlistSection` absent-parent-dir behavior is not actually covered
- Probe: attack Unit 15.1 / 15.4's file-creation path for the case where `.valv/` does not exist at all.
- Result: confirmed counterexample
- Evidence:
  - Current tools resolution treats "project has no `.valv/` dir" as a normal absent-manifest case: `internal/tools/resolve.go:14-37`, `internal/tools/resolve_test.go:69-100`.
  - The plan requires:
    - `WriteAllowlistSection(path string, cfg AllowlistConfig) error` to "create `.valv/tools.toml` when absent" (`drops/DROP_15_NETWORK_POLICY/PLAN.md:48`)
    - CLI `allow` to create `.valv/tools.toml` when absent (`drops/DROP_15_NETWORK_POLICY/PLAN.md:128`)
    - tests for "create-from-absent-file behavior" (`drops/DROP_15_NETWORK_POLICY/PLAN.md:135`)
  - But no acceptance item explicitly requires creating the missing parent directory.
- Counterexample:
  - On a fresh project root with no `.valv/` directory, `WriteAllowlistSection("<project>/.valv/tools.toml", cfg)` will fail with `ENOENT` unless the implementation also `mkdir -p`'s the parent.
  - The current acceptance text can be satisfied by a test that starts with `.valv/` already present but `tools.toml` absent, which misses the real fresh-project case.
- Conclusion:
  - The plan must explicitly require parent-directory creation and test the "no `.valv/` directory exists" case, not just "file absent."

### A5 — "Open mode preserves current behavior" ambiguity
- Probe: check whether current behavior already passes through host proxy env vars, which would make the plan's `no proxy env injection` wording a regression.
- Result: mitigated
- Evidence:
  - Current Codex runtime passthrough is limited to terminal/UI vars plus translated MCP header env vars: `internal/adapters/providers/codex/runtime.go:178-179`, `:557-569`.
  - No current `HTTP_PROXY`, `HTTPS_PROXY`, or `NO_PROXY` passthrough exists in the runtime adapter.
- Conclusion:
  - The falsification attack does not land here. "Open mode injects neither" is consistent with today's runtime behavior.

### A6 — Primary workload proxy compliance is still unproven
- Probe: attack the ship gate by asking whether the plan proves `valv codex` and `valv claude` themselves can use the default closed mode.
- Result: open, release-blocking unknown
- Evidence:
  - The plan itself records the risk: `drops/DROP_15_NETWORK_POLICY/PLAN.md:148`.
  - Unit 15.3 proposes integration proof with an `httptest.Server` and a deny case, but not with the actual first-class workloads (`codex` / `claude`).
  - The current repo evidence in this pass proves only generic container env/network plumbing and current host-bridge rewriting, not vendor CLI proxy behavior.
- Why this matters:
  - DROP_15 changes the default mode for Valv's primary user-facing commands. If either CLI ignores the proxy path, default behavior breaks even if the proxy itself is correct.

## Verdict Summary

Plan fails falsification on confirmed scope and acceptance gaps.

The strongest confirmed break is A2: the proposed policy lives only on the runtime-container seam, while DROP_12 project overlay builds still perform `go install` / `npm install -g` during `docker buildx build` outside that enforcement path. As written, the plan cannot honestly claim "closed by default" for the actual end-to-end project launch path.

A4 is a smaller but concrete acceptance miss: the file-writing helper and CLI acceptance text do not explicitly cover the fresh-project case where `.valv/` itself is absent, even though current repo behavior treats that case as normal and common.

A3 is a confirmed operational gap for exact-host allowlisting with Go installs: Go module downloads are governed by `GOPROXY` / `GOSUMDB` / `GOAUTH` / `GOPRIVATE` semantics, so generic public `go install` requires a more explicit host-policy story than the current plan provides.

Two core risks remain unresolved rather than falsified outright:
- A1: Docker docs loaded in this pass do not prove that `host.docker.internal` remains reachable from a container attached only to an `--internal` network.
- A6: actual Claude/Codex proxy compliance is still unproven, which is material because closed mode is the proposed default.

Recommended plan changes before build:
- either extend DROP_15 to cover overlay-build egress explicitly, or narrow the scope text so "closed by default" means runtime workload only;
- strengthen Unit 15.1 / 15.4 acceptance to require creating a missing `.valv/` parent directory;
- add an explicit Go-host policy note for public tool installs (at minimum `GOPROXY` and `GOSUMDB` implications);
- prove or reject the `--internal` + `host.docker.internal` transport with an actual Docker Desktop experiment before treating Unit 15.3 as implementation-ready.
