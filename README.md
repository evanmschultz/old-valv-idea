# Valv

Valv is a macOS-first control plane for running AI CLIs inside Valv-managed Docker runtimes.

Current scope:

- macOS only
- Docker required
- Codex is the first primary provider target
- `valv codex` stays a pass-through containerized Codex launcher

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
