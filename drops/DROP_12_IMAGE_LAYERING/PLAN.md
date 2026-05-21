# DROP_12 — IMAGE_LAYERING

**State:** planning
**Blocked by:** DROP_11 (done)
**Paths (expected):** `internal/services/images/` (extend), `internal/cli/` (wire per-project image into `valv claude` / `valv codex` launch path), possibly `internal/adapters/docker/` (build composition helpers)
**Packages (expected):** `internal/services/images/`, `internal/cli/`, possibly `internal/adapters/docker/`
**PLAN.md ref:** main/PLAN.md → DROP_12_IMAGE_LAYERING row
**Workflow:** main/drops/WORKFLOW.md
**Started:** 2026-05-21
**Closed:** —

## Scope

Per-project layered Docker image build. Extend `internal/services/images` to compose `valv-<provider>:<project-hash>` from the base `valv-<provider>:dev` image plus a per-project overlay layer that installs the tools declared in the project's `.valv/tools.toml` (DROP_11). Cache the layered image by a tools-hash, mirroring the existing recipe-hash pattern used for base-image rebuild detection.

**This drop is the dogfood unblock.** Once it closes, `ta` CLI and similar per-project Go tools become installable inside Valv containers, which is the gap that surfaced 2026-05-20 when the dev tried to use `valv claude` against the `ta` worktree and the container had no `ta` binary.

This is the second slice in the sandbox-direction sequence (DROP_11 → DROP_12 → DROP_13 → DROP_14 → DROP_15 → DROP_16 release) confirmed by the dev 2026-05-20 after the OSS sandbox survey.

## Pre-Summit Research (carry-forward from DROP_11)

The 2026-05-20 OSS survey identified two dominant declarative-toolchain → image patterns:

- **devcontainer.json Features** — versioned OCI artifacts (`ghcr.io/devcontainers/features/node:1`) layered atop a base image at build time. Used by Codespaces. Build-time install.
- **mise.toml `[tools]`** — local on-demand install into per-user cache, PATH augmentation at shell activation. Runtime install.
- **claudebox** — per-project Docker image `claudebox-<project>` with profile-specific tools layered + intelligent layer caching.

DROP_12 follows the **build-time Docker layering** approach (matching devcontainer Features + claudebox). Rationale: Valv already builds base provider images via `internal/services/images`; extending that path is more architecturally honest than introducing runtime install. The cache-by-tools-hash pattern mirrors the existing recipe-hash invalidation already in `images.Service`.

## DROP_11 outputs available to DROP_12

- `internal/tools/ToolManifest` — parsed schema with `Tools map[string]ToolSpec`, `Allowlist toml.Primitive`, `Env toml.Primitive`.
- `internal/tools/ToolSpec` — `Version string`, `Source string`, `Install string`. String-form vs object-form.
- `internal/tools/Resolve(projectDir string) (ToolManifest, error)` — returns empty manifest + nil if absent; parsed + validated manifest if present.
- `internal/tools/ToolsFilePath = ".valv/tools.toml"`.

## Schema decisions for the planner to nail down

These are the load-bearing design calls. The planner OWNs them; orch will route only the architectural ones to dev.

1. **Per-project image tag format.** `valv-claude:<project-hash>` vs `valv-claude:<project-name>-<tools-hash>` vs other. Survey claudebox: `claudebox-<project-name>`. Recommend `valv-<provider>:<short-tools-hash>` (deterministic, content-addressed; project name is for operator display, hash for cache key).
2. **What's "tools-hash"?** A stable hash over the parsed manifest content — order-independent for `Tools map`, includes versions and source/install strings. Use `crypto/sha256` against a canonical JSON or TOML re-encode of the manifest. Two projects with identical tool declarations share the same overlay image.
3. **Cache invalidation chain.** Three sources can trigger a rebuild: (a) base image rebuilt (existing recipe-hash logic), (b) tools.toml content changed (new tools-hash), (c) Dockerfile overlay template changed (separate "overlay-recipe-hash"). All three should compose into the per-project image's identity.
4. **Overlay Dockerfile template.** Single canonical template `Dockerfile.overlay` (or generated dynamically per manifest) that takes a list of tools and emits the install commands. For string-form tools (`mage = "latest"`): assume `go install`-style or pre-known registry; for object-form (`{ source, install }`): execute the `install` command verbatim against `source`. Network policy (closed-by-default outbound) is DROP_15's concern — DROP_12 assumes open network during build.
5. **What install commands does DROP_12 support?** Minimum viable: `go install <source>`, `npm install -g <source>`, `pip install <source>`. Object-form tools specify their own `install` command. String-form tools need a registry lookup — recommend deferring registry resolution to a later drop and supporting only string-form for **specific well-known names** (e.g. `mage`, `gh`, `go`) where DROP_12 hardcodes the install command, OR requiring all tools in DROP_12 to use object-form (verbose but unambiguous). Planner picks; orch routes to dev if ambiguous.
6. **Where does the per-project image build trigger?** Options: (a) lazily at `valv claude` / `valv codex` launch, mirroring the existing `EnsureLatest` recipe-hash check for the base image; (b) explicit `valv image build` command. Recommend (a) — auto-build-on-launch matches the existing UX where users don't think about images.
7. **Cleanup.** When does a per-project image get garbage collected? When the project is deleted? When tools-hash changes (orphan old images)? Recommend (b) — bump tools-hash means a NEW image; the old one becomes orphaned and is candidate for `valv image cleanup`.
8. **`valv claude` / `valv codex` launch path integration.** The launcher needs to use the per-project image (`valv-claude:<tools-hash>`) when one exists, falling back to base (`valv-claude:dev`) when `.valv/tools.toml` is absent. The fallback ensures projects without a tools.toml continue to work unchanged.

## Planner

<Filled by ta-go-planning agent in Phase 1. Atomic units of work below per WORKFLOW.md § "Phase 1 — Plan".>

## Notes

- DROP_11 establishes the schema + parser foundation; DROP_12 consumes it.
- Network policy enforcement (closed-by-default + allowlist) is DROP_15's concern. DROP_12 assumes Docker build can reach the open internet to install tools.
- `valv run --account X <cmd>` generic primitive is DROP_13's concern. DROP_12's launcher changes are scoped to existing `valv claude` / `valv codex` providers.
- Per-account env var injection is DROP_14's concern. DROP_12 doesn't thread custom env into the build or runtime.
- The 70% per-package coverage floor still applies (and is met at 60% gate per the deferred-from-DROP_11 Unit 11.5). New packages added in DROP_12 should ship at ≥70% local coverage from day one.
