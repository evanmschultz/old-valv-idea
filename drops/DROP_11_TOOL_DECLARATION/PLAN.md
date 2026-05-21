# DROP_11 — TOOL_DECLARATION

**State:** planning
**Blocked by:** DROP_10 (done)
**Paths (expected):** new package `main/internal/tools/` for `.valv/tools.toml` parsing + validation; possibly `main/internal/cli/tools.go` for a `valv tools` subcommand surface; testdata fixtures for parser coverage
**Packages (expected):** `internal/tools/` (new), `internal/cli/`, possibly `cmd/valv/`
**PLAN.md ref:** main/PLAN.md → DROP_11_TOOL_DECLARATION row
**Workflow:** main/drops/WORKFLOW.md
**Started:** 2026-05-20
**Closed:** —

## Scope

Add a declarative per-project toolchain schema. Projects place a `.valv/tools.toml` file at their root declaring what tools the Valv container should make available (e.g. `mage`, `gh`, `ta`, `go`, language toolchains). This drop is **schema + parser + per-project resolution only** — no image build, no install action. The output is structured data: "this project declares these tools." DROP_12 reads that data and produces a per-project layered image.

This is the first slice in the sandbox-direction sequence (DROP_11 → DROP_12 → DROP_13 → DROP_14 → DROP_15 → DROP_16 release) confirmed by the dev 2026-05-20 after the OSS sandbox survey.

## Pre-Summit Research (2026-05-20 OSS sandbox survey, abbreviated)

Survey reviewed 8 projects (devcontainer.json, mise, asdf, nix-shell/flakes, GitHub Codespaces, Gitpod, ddev, claudebox). Load-bearing findings for THIS drop's schema design:

- **Declarative root-file is universal.** Every project uses some variant: `devcontainer.json`, `.tool-versions` (asdf), `mise.toml`, `flake.nix`, `.gitpod.yml`, `.ddev/config.yaml`, claudebox profile `.ini`s.
- **Two dominant declarative patterns:**
  - **devcontainer.json Features** — versioned OCI artifacts referenced by name (`ghcr.io/devcontainers/features/node:1`); pulls a remote-defined install script at build time. Layered on top of a base image. Used by Codespaces.
  - **mise.toml `[tools]`** — `node = "20"`, `python = "3.12"`, `go = "1.22"`. Local on-demand install into per-user cache; PATH augmentation.
- **Tool/version scoping is bottom-up.** Project file overrides user file overrides system file. mise and asdf both honor nested-directory precedence (a `mise.toml` in a subdir overrides the project root for that subtree). Out of scope for v1 of this drop — single root-level `.valv/tools.toml` only.
- **Multi-arch.** Docker images on Apple Silicon may be either `linux/arm64` or `linux/amd64` depending on the base image and what binaries are available; tools.toml should be arch-agnostic at the declaration level (image-build layer handles arch resolution).
- **claudebox uses per-project profile `.ini` files** that stack — e.g. `core + build-tools + python + ml`. Distinct from per-tool declaration; this drop chooses per-tool over per-profile because per-tool is more granular and composes more cleanly with arbitrary tool sets.

Recommended schema shape for planner consideration (NOT final — planner decides):

```toml
# .valv/tools.toml
[tools]
mage = "latest"
gh = "latest"
go = "1.22"
ta = { source = "github.com/evanmschultz/ta@main", install = "go install" }

[allowlist]
hosts = ["github.com", "proxy.golang.org", "sum.golang.org"]

[env]
GOPRIVATE = "github.com/evanmschultz/*"
```

Schema decisions for planner to nail down:

- TOML vs JSON vs YAML (recommend TOML — Valv already uses BurntSushi/toml).
- Version spec format (`latest` / `"1.22"` / git refs / `path:` local — borrow asdf's `.tool-versions` pattern?).
- Custom install commands vs only registry/well-known tools (the `ta` example above shows a custom `go install`).
- Allowlist + env keys in this drop or deferred to DROP_15/DROP_14 (probably defer — keep this drop scoped to tool declaration; cross-reference only).
- Validation rules (required fields, version syntax, max tool count, reserved names).

## Planner

<Filled by go-planning-agent in Phase 1. Atomic units of work below. Each unit's state is mutated in place by the builder during Phase 4. See main/drops/WORKFLOW.md § "Phase 1 — Plan" for deliverable rules.>

## Notes

- Survey output is the evidence base for schema design. Planner should ground the schema in devcontainer.json + mise.toml patterns (cited above) rather than inventing from scratch.
- This drop is intentionally narrow: parser + per-project resolution + simple `valv tools` introspection command. No image-build changes (DROP_12), no `valv run` adapter (DROP_13), no env-var injection (DROP_14), no network policy enforcement (DROP_15). The schema MAY include `[allowlist]` / `[env]` blocks as forward-compatible stubs the parser accepts but DROP_11 takes no action on.
- The 70% per-package coverage gate applies to the new `internal/tools/` package from day one.
