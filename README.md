# Valv

Valv is a macOS-first control plane for per-account isolated, containerized agentic-dev workloads. Each workload runs in a Docker container with its own provider credential home, project bindings, and mounts, so multiple identities (for example `personal` and `work`) stay isolated per project.

Launching AI coding CLIs is the first shipped case — not the whole product. `valv codex` and `valv claude` are containerized pass-through launchers, and both are thin adapters over a generic per-account run primitive, `valv run`, which carries the same isolation model: per-account credential homes, sibling-path-aware mounts, per-project overlay images, and cross-provider in-container routing.

Current scope:

- macOS only
- Docker required (Docker Desktop)
- Provider launchers shipped: `valv codex` (the first primary target) and `valv claude`
- `valv run --account <name> [--provider <provider>] <command>` is the generic primitive the launchers are derived from

## Usage

Launch a provider CLI inside its isolated container. When the project has a single account for that provider, Valv auto-binds it; with several, it shows a picker:

```bash
valv codex
valv claude
```

Run an arbitrary command in a named account's isolated container — the generic primitive the launchers are built on:

```bash
valv run --account personal bash
valv run --account work --provider claude claude --version
```

`valv run` resolves the account across providers and does not mutate project bindings; `--provider` disambiguates when the same account name exists for both providers. `valv codex` and `valv claude` are first-class adapters over this same launch path.

The isolation model is shared by `valv run`, `valv codex`, and `valv claude`:

- Each managed account has its own provider credential home; the selected account's home is bind-mounted into the container, so identities stay isolated per project.
- When both a Codex and a Claude account are bound to a project, the other provider's credential home is cross-mounted, so an in-container agent can invoke the other CLI (for example a `valv claude` session running `codex exec`).
- A per-project toolchain declared in `.valv/tools.toml` composes a per-project overlay image on top of the base provider image.

## Prerequisites

- Go from `go.mod`
- Docker Desktop or another local Docker daemon
- Mage

Install Mage:

```bash
go install github.com/magefile/mage@v1.17.0
```

## Common Commands

```bash
mage build
mage test
mage golden
mage integration
```

Build and run the CLI directly:

```bash
mage run "status"
mage run "codex --help"
```

Use the disposable dev-home workflow:

```bash
mage dev:home
mage dev:run "image update"
mage dev:run "status"
mage dev:clean
```

The disposable dev-home path is separate from your real home directory and keeps local validation from polluting host-backed provider state.

## Claude OAuth

When you run `valv account add claude <name>`, Valv starts a managed container
running the Claude CLI and bind-mounts the account's home directory. Claude
auto-detects the missing credentials and prints an OAuth URL in the terminal.

To complete the flow:

1. Press `c` in the Claude TUI — this copies the URL to the clipboard via
   OSC-52, bypassing terminal line-wrap whitespace that would corrupt the URL
   if you tried to mouse-select it.
2. Paste the URL into your browser and complete the OAuth flow.
3. Paste the authorization code back into the terminal when prompted.
4. Press Ctrl-C twice to exit the container. Claude prints
   `Press Ctrl-C again to exit` between the two presses.

The bind-mounted `.credentials.json` is written by the container natively and
persists in the account's managed home directory. Subsequent `valv claude`
launches read it without re-auth.

## Repository Layout

This repo uses a bare-root Git layout with worktrees.

- bare root: Git control data and shared repo metadata
- `main/`: primary implementation worktree
- lane worktrees: task-specific branches created from the integration checkout
- `.worklog/`: local-only agent worklogs inside the active worktree

## Testing

The canonical local gate is:

```bash
mage test
mage build
```

Additional targeted commands:

```bash
mage testPkg ./internal/output
mage goldenUpdate
mage integration
```

`mage test` runs `gofumpt` first, then one combined `go test -race -cover ./...` pass with laslig/gotestout rendering and coverage enforcement. Package goldens that live in normal `go test` packages run there automatically. Docker-backed transcript goldens stay separate in `mage integration`.
