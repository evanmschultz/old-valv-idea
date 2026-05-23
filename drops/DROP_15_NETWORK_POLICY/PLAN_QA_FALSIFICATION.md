verdict: fail

# DROP_15 — Plan QA Falsification, Round 3

Round 3 closes the two prior falsification findings on project-root drift and image-build cache bypass, but it still leaves four new counterexamples unmitigated. This pass attacked the revised plan against committed repo state pinned by Hylla to `github.com/evanmschultz/valv@main` at `1759e64`, plus current external Docker / provider-CLI docs where the plan relies on runtime semantics outside the repo.

## 1. Counterexamples

### 1.1 Fallback topology can silently re-open default egress

- Scenario: `host.docker.internal` is unreachable from a `--internal` network, so the builder follows the plan's fallback example and attaches the workload or build container to `bridge` via `docker network connect bridge` in order to reach a host-local proxy.
- Repo evidence:
  - The fallback example is currently accepted in the plan at `drops/DROP_15_NETWORK_POLICY/PLAN.md:47`, `149`, and `228`.
  - Runtime and build both intend to rely on `host.docker.internal` (`drops/DROP_15_NETWORK_POLICY/PLAN.md:35`, `170-185`).
  - Codex bridge traffic already uses `http://host.docker.internal:<port>` in committed code at `internal/adapters/providers/codex/bridge.go:42-68`.
- External evidence:
  - Docker docs say containers can be attached to multiple networks, and packets to non-directly-connected destinations go through a default gateway selected by Docker; that default gateway "may change whenever a container's network connections change."
  - Docker docs also show the default `bridge` network has internet access by NAT/masquerading.
- Counterexample:
  - A container attached to both an internal network and `bridge` can regain a default route through `bridge` unless the fallback explicitly constrains gateway selection and then proves non-HTTP egress is still blocked after the second attachment.
  - The current plan only says "for example `docker network connect bridge` plus custom routing." That is not a safety proof. It is a leakage shape.
- Narrow fix:
  - Upgrade the fallback from example text to a required acceptance invariant: if a second network is attached, the plan must require explicit gateway-priority control plus a post-attach proof that direct external egress is still blocked for both runtime and build.
  - If the team does not want to own that routing proof in DROP_15, accept the limitation and block closed-default on platforms where `host.docker.internal` is not reachable from `--internal`.

### 1.2 `valv network deny <default-host>` is operator-misleading

- Scenario: an operator runs `valv network deny github.com` expecting GitHub egress to be blocked.
- Repo evidence:
  - Built-in hosts are always unioned in at `drops/DROP_15_NETWORK_POLICY/PLAN.md:31`, `43`, and `94`.
  - The plan explicitly says built-in defaults are not subtractable at `drops/DROP_15_NETWORK_POLICY/PLAN.md:37` and `210`.
  - Unit 15.4 only requires `deny` to be idempotent and remove user-added hosts; its planned tests cover only "deny-no-op on missing hosts" at `drops/DROP_15_NETWORK_POLICY/PLAN.md:214-222`.
- Counterexample:
  - `deny github.com` can succeed as a no-op while `github.com` remains in the effective allowlist.
  - That violates normal operator expectation for a command spelled `deny`, and the current plan does not require any warning, error, or explicit output showing that the effective policy did not change.
- Narrow fix:
  - Make this behavior explicit in the command contract: denying a built-in host must either return a deterministic error or emit a deterministic warning that the host remains effective because DROP_15 has no subtractive override.
  - Add a planned test for that exact case instead of only testing generic deny idempotence.

### 1.3 `WriteAllowlistSection`'s preservation contract is under-specified relative to the stated guarantee

- Scenario: `.valv/tools.toml` already exists with CRLF endings, a UTF-8 BOM, comments interleaved through `[tools]` or `[env]`, or a multi-line TOML string that contains text resembling a section header.
- Repo evidence:
  - The plan promises verbatim preservation of `[tools]` and `[env]` block order and content at `drops/DROP_15_NETWORK_POLICY/PLAN.md:33`, `44`, and `96`.
  - The current parser only reads TOML; there is no committed writer/editor seam yet in `internal/tools/tools.go:70-130` or `internal/tools/resolve.go:11-44`.
  - Unit 15.1's planned tests mention only generic "section-preservation behavior" at `drops/DROP_15_NETWORK_POLICY/PLAN.md:97`.
- Counterexample:
  - A naive section-surgery implementation can pass the currently planned tests while still corrupting real files that use BOM, CRLF, interleaved comments, or multi-line strings.
  - That would violate the plan's stated "VERBATIM" contract without being caught by the acceptance suite the plan currently describes.
- Narrow fix:
  - Either narrow the contract to a simpler supported file shape, or strengthen the planned tests with explicit golden fixtures covering BOM, CRLF, interleaved comments, trailing whitespace, and multi-line-string cases.
  - Without one of those two moves, the contract is stronger than the acceptance proof.

### 1.4 The provider-CLI proxy ship gate is still only a note, not a unit-owned acceptance criterion

- Scenario: DROP_15 passes all planned unit tests, but `valv codex` or `valv claude` still fails under closed-default proxying in a real launcher flow.
- Repo evidence:
  - The drop-level acceptance still claims closed mode is the default at `drops/DROP_15_NETWORK_POLICY/PLAN.md:45`.
  - Unit 15.3 proves only proxy injection and a local allow/deny test via a host-local service at `drops/DROP_15_NETWORK_POLICY/PLAN.md:173-186`; it does not require an actual `valv claude` or `valv codex` launch under closed mode.
  - The only place that mentions real provider-CLI proxy behavior is builder note A6 at `drops/DROP_15_NETWORK_POLICY/PLAN.md:229`.
- External evidence:
  - Anthropic's current enterprise network docs say Claude Code respects `HTTP_PROXY`, `HTTPS_PROXY`, and `NO_PROXY` (`https://code.claude.com/docs/en/corporate-proxy`, lines 97-113 as opened on 2026-05-22).
  - OpenAI's official Codex repo has a still-open proxy failure report dated March 28, 2026: issue `openai/codex#16079` says Codex CLI fails behind an HTTP proxy even when `curl` works from the same shell and proxy env vars are passed explicitly.
  - A separate Codex issue dated March 9, 2026 (`openai/codex#14080`) reports that explicit proxy env vars work on macOS while other proxy modes do not, which reinforces that proxy behavior is transport- and environment-sensitive rather than something this plan can assume.
- Counterexample:
  - The plan can go green without ever proving the user-facing launcher paths survive closed-default proxying, even though one of the two supported CLIs has active official proxy regressions.
  - That is not just "unproven"; it is an external dependency with live failure evidence and no owning unit acceptance.
- Narrow fix:
  - Move A6 out of notes and into owned acceptance criteria.
  - Either add launcher-level integration acceptance for `valv claude` and `valv codex` under closed mode on the supported macOS topology, or explicitly narrow DROP_15 so closed-default ships only for `valv run` until both provider adapters are proven.

## 2. YAGNI Pressure

- 2.1 `internal/services/networkpolicy` and the Docker network helper surface are justified. The plan has two concrete consumers, not one: runtime (`15.3`) and overlay-image build (`15.2.5`). This is not premature abstraction.
- 2.2 Root-scoped manifest resolution is also justified, but it needs an explicit limitation statement. `project.DetectFrom` walks up to the nearest `.git` marker and returns that root (`internal/project/project.go:27-58`), so a nested `.valv/tools.toml` under a repo subdirectory will be ignored once `15.0` lands. If monorepo subproject manifests are out of scope, say so explicitly; do not leave precedence to inference.

## 3. Hidden Dependency Check

- 3.1 DROP_14 env-merge re-home is explicit, not hidden. `drops/DROP_15_NETWORK_POLICY/PLAN.md:46` and `182` call it out, and DROP_14 already notes that Units 14.4-14.5 may need to move to whichever package assembles `ContainerRunRequest.Env`.
- 3.2 Docker fallback routing behavior is still a hidden dependency unless the plan upgrades it from an example to a required proof. Right now the safety of the fallback depends on undocumented builder judgment.
- 3.3 Provider CLI proxy behavior is still a hidden ship dependency because A6 is advisory text, not unit-owned acceptance. That is the most material remaining hidden dependency.

## 4. Verdict

Fail. Round 3 fixed the prior root-resolution and build-cache issues, but the plan still permits:

- a fallback topology that can leak unrestricted egress,
- a misleading `deny` UX for built-in hosts,
- a preservation contract stronger than its planned test proof,
- and a user-facing proxy dependency that is acknowledged in notes but not owned by any acceptance gate.

The narrow path to a PASS is:

- make the multi-network fallback prove route safety,
- give `deny` an explicit built-in-host outcome,
- either narrow or fully test the verbatim writer contract,
- and promote provider-launcher proxy compatibility from note A6 into owned acceptance.
