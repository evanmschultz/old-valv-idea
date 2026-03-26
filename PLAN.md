# Valv Plan

Date: 2026-03-23

## Product Goal

`valv` should provide one stable `valv/v1/api` control plane for running local AI CLIs as remote-capable runtimes.

Primary use cases:

- expose Codex through a remote API endpoint
- route requests by `provider + auth profile + project + mode`
- support project-aware local CLI launching without forcing global account mutation
- keep room for Claude and Gemini without changing the API namespace
- provide a Valv-owned operator TUI for project/session/profile control

Current platform scope:

- macOS only

The local architecture notes in [valv_architecture_notes.md](./valv_architecture_notes.md) remain the source of truth for the runtime model. This plan consolidates repo-style, runtime, switching, and CLI/TUI research.

## Repository Topology

Valv should use the same bare-root plus worktree pattern already used elsewhere in this workspace.

Required shape:

- the repository root is a bare Git repository
- `main/` is the primary worktree for implementation
- agents work in `main/`, not in the bare root
- root `.codex/config.toml` points `gopls mcp` at `main/`
- agent-local worklogs live in `.worklog/` inside the active worktree and remain gitignored

This keeps the control root, Git plumbing, and worktree model aligned with the existing local workflow.

## References Read

Local notes:

- [valv_architecture_notes.md](./valv_architecture_notes.md)
- [VALV_ACCOUNT_SWITCH_PLAN.md](./VALV_ACCOUNT_SWITCH_PLAN.md)
- [VALV_REPO_PLAN.md](./VALV_REPO_PLAN.md)

Reference repos under `.tmp/`:

- `.tmp/autent` - docs style, `AGENTS.md`, `Justfile`, CI shape
- `.tmp/blick` - CLI composition, output policy, styling, Fang v2 usage
- `.tmp/claudebox` - runtime isolation, per-project state mounts
- `.tmp/cc-account-switcher` - simple global Claude account swap
- `.tmp/ccswitch-account` - more robust global Claude account swap
- `.tmp/ccx` - cleaner architecture for global switching, but still global-switch oriented
- `.tmp/claude-accounts` - direct Claude config rewriting; deprecated

Local CLI surfaces checked:

- `codex --help`
- `codex exec --help`
- `codex login --help`
- `codex app-server --help`
- `claude --help`
- `claude auth --help`

External docs checked:

- Docker bind mounts and container isolation docs

## Core Product Direction

The right model for Valv is not "switch the user's global account like `ccx` or `ccswitch`". The right model is:

1. Detect project from CWD or explicit project id.
2. Resolve that project's bound provider and auth profile.
3. Launch the provider inside a Valv-owned runtime boundary.
4. Keep the identity switch scoped to that runtime boundary only.

That means the default design is profile binding plus runtime isolation, not repeated mutation of one global auth file.

For `valv codex`, Valv should not reinterpret Codex arguments or invent Codex-specific UX semantics. Everything after `codex` should be passed through to the containerized Codex process as directly as possible.

## Runtime Boundary Decision

Docker should be treated as required from the start for the API and headless path.

Reasons:

- the API path must be non-global by definition
- workspace access should vary by mode, and Docker gives a clean way to mount or omit the project tree
- provider-owned caches, configs, auth, and session artifacts can live in isolated profile homes or volumes
- the same runtime model can serve both headless API execution and attached local CLI execution
- this avoids building the MVP around host-global credential swapping

Important nuance:

- Docker is required for the runtime boundary
- tmux is not required for the runtime boundary
- an operator TUI is useful for human UX and session control, but it is not what creates isolation

Containers give process, filesystem, and mount isolation. tmux only gives terminal multiplexing. The TUI is an orchestration layer, not the isolation boundary.

## Client Images And Updates

Valv should treat provider client binaries inside Docker images as Valv-managed runtime dependencies.

Current Codex observation from the local machine:

- `codex --version` reports `codex-cli 0.116.0`
- the current local CLI help does not expose a dedicated `codex update` subcommand

Design implication:

- Valv should not depend on in-session self-update flows as the primary update mechanism
- Valv should own image rebuild/update behavior itself

Recommended update model:

- each provider image has a Valv-owned client install recipe in the Docker build
- provider images should use explicit pinned client versions by default
- Valv stores built image version metadata in SQLite
- `valv manage update` and `valv m update` rebuild provider images for the clients the user has configured
- the management TUI should expose the same update action

Recommended user experience:

- if a running client reports that it is outdated, Valv should not try to mutate that running session in place
- Valv should detect that failure pattern where practical, log it, and after the process exits show a clear action like `valv manage update`
- API workers should be refreshed by rebuilding the image and then letting old warm workers age out or be restarted cleanly

Do not make users manually edit Dockerfiles just to update a client version.

Important update policy details:

- using `latest` in the Docker build does not update existing images
- Valv should not rely on a floating `latest` channel as the default update strategy
- if Valv wants newer client bits, Valv still needs to rebuild the image and restart or replace affected containers
- rebuilding on every `valv codex` launch would be slow, network-heavy, and brittle
- the right default is managed rebuilds, not implicit rebuild-on-run

Practical consequence:

- `valv codex ...` should run against an already-built local image
- `valv manage update` should rebuild provider images and safely roll warm API workers forward
- interactive attached launches should pick up the new image on the next run after update
- API warm containers must be replaced or restarted before they will use the updated client version

Build implementation preference:

- prefer `docker buildx build --load` for local image availability
- do not keep relying on the legacy builder path when a modern buildx path is available
- suppress avoidable npm update-notifier and similar package-manager chatter inside the provider image build where practical

## API Compatibility Surfaces

Valv should keep protocol compatibility separated from provider/runtime execution.

Rules:

- OpenAI-compatible request/response flows should live behind the OpenAI compatibility surface
- Anthropic-compatible request/response flows should live behind a separate Anthropic compatibility surface
- the underlying runtime manager may still route both surfaces to the same provider/runtime adapter when appropriate

Implication for future providers:

- Codex uses the OpenAI-compatible surface first
- Gemini should also use the OpenAI-compatible surface when Valv is exposing Gemini through an OpenAI-compatible API contract
- Claude and other Anthropic-compatible clients should use a separate Anthropic handler surface when that compatibility layer is added

Do not conflate provider identity with protocol compatibility. The handler should match the compatibility contract being served, not the marketing name of the backing provider.

## Modes

Valv should keep the mode vocabulary from the architecture notes:

- `fresh`
- `resume`
- `ephemeral`

In addition, the execution surface should distinguish between:

- `api/headless`
- `interactive/attached`

Recommended behavior:

- `api/headless + fresh`: isolated runtime, no provider resume, no workspace mount unless explicitly requested
- `api/headless + ephemeral`: even stricter; no persistent project artifacts, no workspace mount by default
- `interactive/attached + fresh`: isolated runtime with project bind mount so the CLI behaves like a normal local coding session
- `resume`: session-bound runtime with explicit persistence and provider session mapping

Recommended runtime shape:

- API/headless execution is a separate path from `valv codex`
- interactive `valv codex` sessions should not require long-lived containers to support resume
- interactive resume should rely on persisted provider session state, not on keeping old containers alive
- API/headless runtimes should keep warm containers alive for a configurable idle TTL so repeated calls do not constantly cold-start Docker
- after the idle TTL expires, Valv may stop the warm container and later recreate it on demand without losing persisted provider state

## Workspace And CWD Behavior

Docker does not prevent normal working-directory behavior.

If Valv bind-mounts the project into the container and starts the provider CLI with the container working directory set to the mounted path, the CLI can still:

- read project files
- edit project files
- write generated files
- inspect git state
- behave like a normal CLI running in that repo

That is the same basic pattern ClaudeBox uses for project isolation, but Valv should apply it with a cleaner Go control plane and provider-neutral runtime model.

The API/headless path should not automatically get workspace access. Workspace access should be deliberate and mode-dependent, with the default leaning toward no workspace mount unless explicitly enabled by project or request policy.

For the interactive `valv codex` path, Valv should aim to make Codex feel as close to native as possible:

- pass through Codex arguments after `valv codex`
- mount the real project path or a stable path derived from it
- avoid Valv-specific session semantics when Codex already has them
- let Codex believe it is running in a normal project working directory

Valv is adding the runtime boundary and profile selection, not replacing Codex's own session model.

Operationally, that means:

- `valv codex ...` resolves project/profile/runtime context
- Valv starts the Dockerized Codex process
- Valv passes all arguments after `codex` through to Codex unchanged
- if Codex fails, Valv should surface the Codex failure cleanly with Valv's output/logging conventions
- Valv should avoid leaking raw Docker noise to the user unless Docker itself is the real root cause
- `valv codex ...` should not stop to open a Valv picker or management flow; if required setup is missing, it should fail clearly and direct the user to `valv manage`

## CLI Alias Policy

Valv should support selective short command aliases, not blanket abbreviation of every command.

Recommended aliases:

- `valv h` for help
- `valv m` for manage
- `valv g` for global

Do not auto-generate one-letter aliases for every command. The command tree already has natural conflicts like `serve`, `status`, and `switch`, and blanket abbreviation would make the CLI less predictable.

## Dev Mode Versus Release Mode

Valv should support a clean development workflow that does not force developers to write profile/config/log/database state into their real home directory.

Current development policy:

- dev-mode commands should run with a temp-home root
- dev-mode provider homes should live under that temp-home root unless explicitly overridden
- dev-mode local filesystem state should be disposable in one cleanup step
- dev-mode should still preserve access to the host Docker CLI configuration/plugins so modern Docker features like `buildx` keep working

Important limitation to preserve and address:

- Docker images live in the host Docker daemon, not inside the temp home
- a true clean dev/release split therefore also needs separate dev image naming so dev cleanup can remove only dev-tagged images

Development command direction:

- keep normal `build` for the default local binary
- provide a dev wrapper path that runs `./valv` with a temp-home root and a dev-specific image override
- provide a dev cleanup path that removes the temp-home state and the dev-tagged image set
- default developer validation should prefer the disposable `just dev ...` path over writing into the real home directory

Current validation follow-ups to preserve:

- `just dev-reset` and other dev-mode entrypoints must clearly explain that the printed root is one disposable temp-home path, not a scary permanent system path
- dev-mode output should make it obvious which paths are disposable, which images are dev-only, and how to clean them up
- help menus should grow more explanatory as the command tree gets deeper
- help menus should include realistic examples, and output-producing commands should explain the meaning of their key output fields
- help/examples should prefer unambiguous placeholder profile names like `profile-name` or `alternate-profile`; avoid example names like `dev` that can be confused with environment modes
- the CLI should support `<command> help` and `<command> h` where that is unambiguous and does not collide with real subcommands
- the CLI should not reinterpret trailing help-like args on leaf/pass-through commands such as `valv codex`
- output paths and record/list rendering should stay DRY and closer to the `blick` reference shape
- `manage update` output should explain what provider/client version was built, which image tags were updated, and whether anything actually changed
- Valv-managed Docker containers, images, and related runtime artifacts should carry a clear `valv-...` naming scheme so developers can distinguish them from unrelated Docker workloads
- cleanup behavior should align with that naming scheme, but actual cleanup authority should come from Valv labels, not broad `valv-...` prefix matching; normal cleanup must not remove unrelated unlabeled containers or anonymous volumes
- API serve help and runtime output must make it explicit that `runtime-ttl` is an idle runtime lease timeout, not an automatic server shutdown timer; warm runtimes should still be swept down after expiry
- API startup output must only announce a listening address after a real successful bind, and non-positive runtime TTL values should fail clearly instead of being silently coerced
- API runtime sweeps should use the server lifecycle context and quiet Docker execution so shutdown is clean and HTTP-facing stdout/stderr stay stable
- Codex profile-home mounting and Codex-native resume/session behavior need explicit validation on macOS Docker Desktop so the containerized runtime preserves Codex's own `.codex` state model rather than introducing Valv-owned session semantics
- using the literal host `~/.codex` path should remain an explicit profile choice, not an implicit default; default Valv profiles should preserve Codex-native state within the selected profile home
- mounted host Codex homes can preserve auth and resume state, but host-oriented MCP config cannot be reused blindly inside Linux containers
- containerized Codex needs a Valv-managed MCP overlay or translation layer for host-specific entries such as macOS absolute binary paths and host-loopback URLs
- host-loopback MCP URLs used from inside Docker Desktop containers should be translated to `host.docker.internal` where appropriate
- local stdio MCP tools must not be hardcoded provider-by-provider; Valv should classify them generically and either run them in-container when the command is available there, or expose them through a Valv-managed host bridge when they depend on host-only binaries or absolute host paths
- containerized interactive Codex needs a coherent in-container user and `HOME`; do not rely on the host absolute profile path doubling as the Linux home directory
- the selected profile home should remain the durable Codex state source, but Valv should mount it into a normalized in-container home path and generate a container-safe config overlay for each run
- container-safe overlays should merge profile-level and project-level Codex config with project-level precedence
- Valv MCP host bridges must have their own lifecycle and remain alive for the life of the prepared runtime; they must not be tied to a single API request context when the runtime is warm and reusable
- if a host stdio MCP bridge cannot be created, Valv should omit that translated MCP entry from the container overlay and emit a clear warning instead of writing a malformed server entry
- if a warm runtime container starts successfully but Valv fails to persist the final running status, Valv must best-effort remove that container and close its prepared runtime artifacts before returning the error
- attached interactive Codex runs need terminal-integration coverage in addition to Bubble Tea golden tests, because TeaTest only covers Valv-owned TUI screens and cannot prove the attached `docker run` TTY behavior
- attached `docker run` TTY handling must work with real terminal stdin/stdout/stderr descriptors even when they are distinct file descriptors for the same terminal device; do not restrict the controlling-TTY path to the exact same `*os.File` object
- the repo must expose explicit local recipes for golden/TUI regression coverage rather than relying on ad hoc package test commands
- containerized auth UX should prefer device-code login for isolated profiles unless Valv explicitly publishes or relays the localhost callback port used by browser-based login

## Account Switching Research

The account-switcher repos are useful, but they solve the wrong primary problem for Valv.

What they do:

- back up the currently active Claude credentials
- restore another account's credentials and config
- mutate the single active global Claude identity

Useful ideas to reuse:

- lock files
- atomic writes
- backup and restore discipline
- clear account metadata and aliases
- explicit process stop before swap

What not to copy into Valv core:

- repeated rewriting of one global provider home
- Claude-specific assumptions in the core runtime
- coupling profile selection to host-global state mutation

Conclusion:

- global switching should exist as an optional convenience tool
- global switching should not be the primary runtime architecture
- the main `valv codex` path should launch in Docker with Valv-managed project/profile/runtime context

## Global Switch Option

Global switching is simple enough that it should exist from the start as an optional operator feature.

Its role:

- convenience for users who explicitly want to swap the host-global active account
- escape hatch for providers or local setups that do not isolate cleanly
- migration aid for people coming from `ccx` or `ccswitch`

Its non-role:

- not the main API runtime path
- not the default project binding model
- not the source of truth for non-global profile routing

Valv core should stay centered on isolated profile homes and containerized runtimes. The global switch path should be an adapter-style feature beside that core.

Recommended command direction:

- `valv codex` launches the default project-bound attached Codex runtime in Docker
- `valv global switch` or `valv g switch` manages host-global provider auth as a convenience flow
- global switching should support both interactive list selection and explicit CLI arguments
- `valv global switch codex <profile-name>` is a Valv-owned operator command, not Codex pass-through

Global switch syntax direction:

- `valv g switch codex <profile-name>`
- `valv global switch codex <profile-name>`

This is the one place where Valv intentionally does not behave like pure provider pass-through.

## Provider-Specific Conclusions

### Codex

Codex is the best first target for the main Valv use case.

Locally verified:

- `codex` exposes `exec`, `resume`, `--profile`, `--cd`, and `--ephemeral`
- `codex app-server` exists but is marked experimental

Implication:

- Valv should use Codex as an isolated runtime behind its own API
- for the main path, prefer launching Codex in a Valv-owned container rather than depending on the host-global Codex state
- the CLI `--profile` flag is config-oriented, not a full auth-profile system by itself
- OpenAI-compatible HTTP should be the first API compatibility surface

So for Codex, Valv should own the auth-profile mapping and runtime layout. Codex's own config profile flag can still be useful inside a Valv-managed profile home, but it is not sufficient as the primary isolation mechanism.

### Claude

Claude is more awkward for strict local account isolation, especially with OAuth/keychain behavior on macOS.

Locally verified:

- `claude` supports `--settings`, `--setting-sources`, `--resume`, `--continue`
- `claude --bare` explicitly avoids keychain and OAuth reads and says auth is strictly `ANTHROPIC_API_KEY` or `apiKeyHelper`

Implication:

- for remote/API use, Claude should probably run in a containerized Linux-style runtime
- API-key or helper-based auth is a better fit than host-global OAuth mutation
- if strict OAuth separation is still desired later, container runtime is much better than host-global switch scripts

### Gemini

Gemini remains in-scope architecturally, but this plan does not yet include concrete Gemini auth/runtime findings.

## Session And Resume Model

Valv should own session records instead of leaving the user to manage raw provider ids manually.

Recommended command and UX direction:

- `valv codex` should behave like a containerized Codex launcher with profile/project resolution
- Valv should have a separate management surface for profile binding, runtime inspection, global switching, and other operator actions
- arguments after `valv codex` should be passed through to Codex as directly as possible

Recommended CLI shape:

- `valv codex [codex-args...]`
- `valv codex resume <session> [codex-args...]`
- `valv codex -- <raw-codex-args...>` as an escape hatch if needed
- a separate Valv management surface should exist, but Codex pass-through should be the default mental model

Non-goals for `valv codex`:

- no Valv-owned resume browser
- no Valv-owned provider session picker
- no interactive first-run setup picker in the direct pass-through path
- no update workflow in the direct pass-through path

Container lifecycle recommendation:

- each attached Codex launch should normally get a fresh container
- when the attached Codex process exits, Valv should stop that container
- resumability should come from persisted Codex session state under the Valv-managed profile home, not from leaving old containers running
- if the user sends `Ctrl+C` while attached, that signal should go directly to Codex; Valv should only resume control after the Codex process exits

Recommended interactive resume mechanics:

- Valv persists the provider home for the selected auth profile
- Valv mounts the project at a stable, project-unique path inside the container
- to resume, Valv starts a new clean container with the same mounted profile home and project path, then invokes `codex resume <provider-session-id>`

This keeps the container lifecycle and the Codex resume lifecycle separate, which is the correct model.

Important detail:

- do not mount every project at the same in-container path like `/workspace` if Codex session filtering depends on cwd
- use a stable project-unique mount path derived from the real project path, or mount the real absolute host path when practical

That preserves Codex's own project/session logic while still letting Valv isolate profiles in Docker.

The TUI should not try to own Codex session discovery or wrap the Codex TUI while Codex is active. It should hand the terminal to Codex completely, wait for the process to end, and then restore the Valv screen when Valv is the active surface.

## CLI And TUI Direction

Valv should reuse the `blick` approach for CLI quality:

- `fang v2` for command/help presentation
- `lipgloss v2` for human output styling
- TTY-aware output policy with `human`, `plain`, and `json`
- structured boundary logging

Valv should also ship an operator TUI and interactive selectors, but without `huh`.

Use:

- `charm.land/bubbletea/v2`
- `bubbles` directly
- `charm.land/lipgloss/v2`
- `fang v2`

Do not use:

- `huh`
- legacy Charm module paths

The key `blick` patterns to copy are:

- a separate output policy layer
- deterministic plain/json rendering
- styled human output only when appropriate
- config-driven output defaults
- stable machine-readable JSON keys chosen by the command surface, not inferred from human-facing headings

The TUI should be used for:

- project/profile binding and repair flows
- profile switching
- runtime inspection and stop/start actions
- provider image update actions

Operator cleanup should support distinct scopes for:

- local state only
- provider images only
- docker/build state
- full cleanup combining all of the above

Cleanup should be idempotent in normal operator use. Missing cache paths or already-removed provider images should not turn cleanup into a failure mode.

The TUI does not need to be the terminal isolation mechanism. It is the control surface above the Docker runtime.

TUI testing policy:

- use `github.com/charmbracelet/x/exp/teatest/v2` for Bubble Tea regression coverage
- keep golden fixtures for user-visible layout/styling where frame sizing, borders, or list rendering can regress
- TUI changes are not complete until the golden coverage is updated or explicitly shown to be unaffected

Recommended naming:

- `valv manage`
- `valv m`

This avoids collision with `help`, reads clearly, and fits the role better than `home`.

## Containers vs Virtual Machines

Current conclusion:

- containers are enough for the MVP runtime boundary
- a full VM is not required as the default architecture

Why containers are enough:

- the provider process only sees the mounts and environment Valv gives it
- provider auth/config/session state can live in a dedicated profile volume or directory
- on Docker Desktop, containers run inside a Linux VM and only see host files that are explicitly shared and bind-mounted

When a VM might still matter later:

- if a provider depends on host-native keychain or desktop integrations that do not containerize cleanly
- if we want stronger isolation than "container plus controlled mounts"
- if a future provider requires OS-level features that Docker cannot expose safely

So the right default answer is:

- start with Docker
- do not jump to full VMs unless a concrete provider constraint forces it

## State Model

SQLite should be the operational authority for Valv metadata and runtime state.

Planned SQLite responsibilities:

- auth profile records
- project records
- project-to-profile bindings
- default mode and workspace policy
- runtime/session metadata
- provider session ids for explicit resume flows
- audit and operator events

Open design tension:

- whether raw provider credentials should also be persisted in SQLite
- or whether SQLite should store profile metadata while provider-owned auth/config stays in mounted profile homes or volumes

Current leaning:

- store Valv metadata and mappings in SQLite
- let provider-owned auth/config remain in provider-specific profile homes or volumes
- start with host directories for provider homes because they are easier to inspect and debug than Docker named volumes
- add Docker named volumes later only if they solve a concrete persistence or isolation problem

That matches the reality that these CLIs expect their own file/config layouts.

## Project Detection And Binding UX

Desired behavior:

- auto-detect the project from CWD whenever possible
- if no project binding exists yet, `valv codex ...` should fail clearly and direct the user to `valv manage`
- allow explicit non-interactive selection by flag
- keep setup, binding, and repair flows in `valv manage`, not in the direct Codex pass-through path

Recommended flow:

1. detect git root or Valv project marker
2. look up project binding in SQLite
3. if found, use it
4. if missing, return a clear actionable error that points to `valv manage`
5. `valv manage` owns the interactive binding and selection flows

## Proposed MVP Shape

API namespace:

- `valv/v1/api`

Core components:

- Go control plane
- SQLite runtime store
- Docker runtime manager
- provider profile homes under the Valv app-support root
- Docker named volumes for provider/runtime storage where that improves persistence and debuggability
- provider adapters
- CLI with high-quality human/plain/json output
- optional interactive selectors for first-run binding and operator flows

Primary shipped path:

- Codex-backed remote API execution with non-global auth-profile isolation

Also included from the start:

- operator TUI for project/profile/session control
- optional host-global switch helper for convenience

## macOS Storage Layout

Use standard macOS per-user locations, with one deliberate exception for short-lived runtime socket paths.

Recommended defaults:

- database:
  - `~/Library/Application Support/valv/db/valv.sqlite3`
- provider homes:
  - `~/Library/Application Support/valv/providers/<provider>/profiles/<profile>/`
- durable Valv state files:
  - `~/Library/Application Support/valv/state/`
- exported config and policy files:
  - `~/Library/Application Support/valv/config/`
- logs:
  - `~/Library/Logs/valv/`
- caches and rebuild scratch:
  - `~/Library/Caches/valv/`
- transient runtime files, pids, lockfiles, short Unix socket paths:
  - `/tmp/valv-<uid>/`

Recommended details:

- keep the SQLite database under Application Support because it is durable control-plane state
- keep provider-owned auth/session/config files in provider homes under Application Support so they survive container restarts and are easy to inspect while building
- keep logs separate under `~/Library/Logs/valv/` so cleanup and troubleshooting are straightforward
- keep cacheable build artifacts and non-authoritative scratch under `~/Library/Caches/valv/`
- use `/tmp/valv-<uid>/` for sockets, pid files, and other short-lived runtime files because Unix socket path length can become a real issue under deep `~/Library/...` paths

Recommended initial sublayout:

- `~/Library/Application Support/valv/db/valv.sqlite3`
- `~/Library/Application Support/valv/providers/codex/profiles/<profile>/`
- `~/Library/Application Support/valv/state/runtimes/`
- `~/Library/Application Support/valv/config/policies/`
- `~/Library/Logs/valv/valv.log`
- `~/Library/Logs/valv/docker.log`
- `~/Library/Caches/valv/build/`
- `~/Library/Caches/valv/tmp/`
- `/tmp/valv-<uid>/pids/`
- `/tmp/valv-<uid>/locks/`
- `/tmp/valv-<uid>/sockets/`

## Full Architecture

The clean shape is a thin CLI surface over a Go control plane, with Docker and provider adapters pushed to the edges.

Recommended top-level repo structure:

- `cmd/valv/`
- `internal/app/`
- `internal/domain/`
- `internal/services/`
- `internal/adapters/`
- `internal/cli/`
- `internal/tui/`
- `internal/api/`
- `internal/config/`
- `internal/platform/`
- `docs/`

Recommended component responsibilities:

- `cmd/valv/`
  - Fang command wiring
  - process entrypoint only
- `internal/app/`
  - command handlers and use-case orchestration
  - `codex` pass-through launch flow
  - `manage` command actions
  - `global switch` actions
  - `update` actions
- `internal/domain/`
  - pure types and rules for `project`, `profile`, `provider`, `runtime`, `image`, `policy`, `mode`
  - no Docker, SQLite, or filesystem code here
- `internal/services/`
  - project detection
  - profile binding resolution
  - runtime planning
  - image update policy
  - API runtime leasing and TTL cleanup
  - global-switch orchestration rules
- `internal/adapters/sqlite/`
  - database schema
  - repositories for projects, profiles, bindings, runtimes, images, events
  - use `modernc.org/sqlite` only; no CGO SQLite dependency
- `internal/adapters/fs/`
  - macOS path resolution
  - provider home creation
  - lockfiles
  - log file targets
- `internal/adapters/docker/`
  - image build/rebuild
  - container launch/stop/inspect
  - warm runtime rotation
  - mount planning
- `internal/adapters/providers/codex/`
  - Codex-specific command assembly
  - Codex image install recipe
  - Codex error classification
  - optional global switch implementation for Codex host state
- `internal/cli/`
  - output policy
  - Fang/Lipgloss rendering
  - structured error presentation
  - use `.tmp/blick` as the style and structure reference for Fang v2, output policy, and renderer shape
- `internal/tui/manage/`
  - Bubble Tea operator screens
  - project binding flows
  - runtime inspection
  - profile switching
  - update actions
  - use Bubble Tea, Bubbles, and Lipgloss directly with Charm v2 modules only
- `internal/api/openai/`
  - OpenAI-compatible request/response surface
  - request validation
  - request-to-runtime mapping
- `internal/config/`
  - Valv config file parsing
  - defaults
  - policy loading
- `internal/platform/`
  - OS-specific helpers kept deliberately small
  - macOS-only behavior for now

Recommended runtime call chain for `valv codex ...`:

1. CLI parses `valv codex ...`
2. project detector resolves the current project from CWD
3. binding resolver fetches the bound provider/profile from SQLite
4. runtime planner resolves mount paths, image ref, container env, and working directory
5. Docker adapter launches a fresh Codex container
6. Codex adapter passes all user args through unchanged after `codex`
7. terminal is handed directly to Codex
8. on exit, Valv logs full boundary context, renders success/error, and stops the container

Recommended runtime call chain for API requests:

1. OpenAI-compatible API handler validates the request
2. request mapper resolves project/profile/mode/policy
3. runtime lease service finds or creates a warm API runtime keyed by the appropriate binding tuple
4. Docker adapter ensures the correct image generation is in use
5. Codex adapter runs the headless provider path
6. runtime lease service updates TTL and stale-generation state

## Parallel Implementation Plan

This can be built safely in parallel if the foundation contracts are defined first.

Foundation work that should happen first:

- define the domain types and naming
- define the SQLite schema and repository interfaces
- define the Docker runtime interface
- define the CLI output/error interface
- define config loading and defaults
- define logging setup, log paths, and boundary logging conventions

These should be treated as shared foundation, not as late polish. The reason is simple:

- every parallel track needs the same config package
- every parallel track needs the same logger setup and error conventions
- every command and adapter should render and log consistently from the start
- retrofitting config and logging after broad parallel work will cause avoidable churn

After that, these tracks can move largely in parallel:

- Track A: CLI and output foundation
  - `cmd/valv/`
  - `internal/cli/`
  - Fang command tree
  - human/plain/json rendering
- Track B: storage and config
  - `internal/adapters/sqlite/`
  - `internal/adapters/fs/`
  - `internal/config/`
  - macOS path layout
- Track C: Docker runtime manager
  - `internal/adapters/docker/`
  - image build/update logic
  - mount planning
  - container lifecycle
- Track D: Codex provider adapter
  - `internal/adapters/providers/codex/`
  - pass-through launch assembly
  - error pattern handling
  - host-global switch helper
- Track E: management TUI
  - `internal/tui/manage/`
  - profile/project binding flows
  - runtime inspection/update screens
- Track F: OpenAI-compatible API surface
  - `internal/api/openai/`
  - request validation
  - runtime leasing integration
- Track G: observability and maintenance
  - structured logging
  - event/audit writing
  - TTL cleanup jobs
  - log cleanup policy

Recommended parallel-agent split:

- Agent 1: CLI/output foundation
- Agent 2: SQLite + filesystem layout
- Agent 3: Docker runtime manager
- Agent 4: Codex provider adapter
- Agent 5: OpenAI-compatible API layer
- Agent 6: management TUI
- Agent 7: logging, cleanup, and operational tooling
- Agent 8: docs, `Justfile`, CI, and repo hygiene

Best dependency order:

1. foundation contracts plus config/logging/output foundations
2. Tracks A, B, and C in parallel
3. Tracks D, E, and F in parallel once B and C interfaces settle
4. Track G starts immediately and threads through the others
5. docs/CI can proceed in parallel as soon as command names and repo layout stabilize

Main merge/conflict risks:

- command tree churn between CLI and TUI tracks
- runtime interface churn between Docker and provider adapter tracks
- schema churn between storage and API/TUI tracks
- output/error contract churn between command handlers and adapters

Best way to reduce conflict:

- define interfaces and DTOs before implementation
- assign each track a disjoint write scope
- keep provider adapters from reaching directly into SQLite or TUI code
- keep the API layer thin and dependent on services, not on Docker directly

## Logging And Error Handling

Valv should adopt the `blick` logging and output discipline from the start.

Requirements:

- idiomatic Go error handling with wrapped errors bubbling to the boundary
- structured logging via `github.com/charmbracelet/log`
- debug logging throughout the runtime boundary, process launch, profile resolution, storage operations, and API execution paths
- clean user-facing error rendering via Fang/Lipgloss-backed CLI output
- log retention and cleanup policy suitable for a local control-plane/MCP-style CLI app
- macOS-native app-support paths for logs, config, database, and profile metadata

Important boundary rule:

- users should primarily see Codex-originated failures or Valv-originated actionable failures
- raw Docker errors should be logged in detail, but only surfaced directly when Docker is truly the failing component

## Execution Standards

Implementation should be treated as complete only when all of these are true:

- the active plan scope is fully implemented
- all tests pass
- each package reaches at least 70% test coverage
- TDD expectations were followed for new work
- docs, logs, and command behavior are consistent with the implementation

Parallel execution standards:

- shared foundations land first: config, logging, output, domain interfaces
- every build agent gets two independent QA passes before the work is considered done
- every agent keeps a local worklog in `main/.worklog/`
- each build or QA agent should use Context7 first when relevant library docs are available

## Open Questions

1. Which macOS app-support path layout should be the default for the database, logs, provider homes, and caches?

## Source Links

- Docker bind mounts: https://docs.docker.com/engine/storage/bind-mounts/
- Docker Desktop container isolation: https://docs.docker.com/security/faqs/containers/
- Docker build best practices: https://docs.docker.com/build/building/best-practices/
- Docker image pull semantics: https://docs.docker.com/reference/cli/docker/image/pull/
- Docker run pull policy: https://docs.docker.com/reference/cli/docker/container/run/
- Codex CLI docs: https://developers.openai.com/codex/cli
- Codex CLI reference: https://developers.openai.com/codex/cli/reference
- Codex changelog: https://developers.openai.com/codex/changelog
