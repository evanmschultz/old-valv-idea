# Valv — Cross-Provider Quickstart

Run Claude Code and Codex CLIs in Docker, isolated by login per project, with **cross-provider auto-routing**: when Claude Code inside `valv claude` runs `codex exec` or `codex mcp-serve`, codex authenticates as that project's pinned codex account. Mirror direction works too.

## What you need

- macOS with Docker Desktop running.
- Go 1.26+ (for `mage install`) and the Valv repo cloned locally.
- Both CLIs installed on host (`npm install -g @anthropic-ai/claude-code @openai/codex`) — used for one-time auth flows.

## One-time setup (per machine)

```bash
cd /path/to/valv/main
mage install                # puts `valv` on $GOBIN (typically ~/go/bin)
valv version                # sanity check
```

Add at least one account per provider you want to use:

```bash
valv account add codex personal     # follow OAuth prompt (browser opens)
valv account add codex work         # second login → different ChatGPT account
valv account add claude hylla       # follow setup-token flow (browser opens)
```

List what you have:

```bash
valv account list
```

The first `valv claude` or `valv codex` call you make will trigger a one-time Docker image rebuild (~60–90s each) — both images get BOTH CLIs baked in, so cross-calls work. No manual rebuild needed; it just happens on first launch after install/upgrade.

## Per-project setup (one-time per project)

In any project where you want cross-provider routing, bind both providers:

```bash
cd /path/to/your/project    # must have a .git or similar project marker
valv account bind <claude-name> --provider claude
valv account bind <codex-name>  --provider codex
```

Verify:

```bash
valv status --all           # shows bindings for this project across both providers
```

## Run

```bash
valv claude     # Claude Code inside container with codex CLI + project-pinned codex login mounted
valv codex      # Codex inside container with claude CLI + project-pinned claude login mounted
```

That's it. Inside the container:

- `valv claude` → Claude Code can run `codex exec "..."` or `codex mcp-serve` and they auto-authenticate as that project's pinned codex account.
- `valv codex` → Codex can run `claude ...` and it auto-authenticates as that project's pinned claude account.

Different project → different bindings → different identities. The container has no access to your host `~/.codex` or `~/.claude`; only the project-pinned managed-home profiles are mounted.

## Override per-call

If you want a specific account just for one invocation (without rebinding):

```bash
valv claude --account hylla
valv codex --account work
```

This does NOT change the project's stored binding — it's a one-shot override.

## What if I haven't bound both?

If only one provider is bound to the project, the OTHER CLI is in the container but unauthed. Cross-call fails with that CLI's native "not logged in" message — easy to diagnose. Fix by running `valv account bind` for the other provider in that project.

## Useful commands

```bash
valv account list                              # all accounts across providers
valv account list --provider codex             # filter by provider
valv account inspect <name> --provider claude  # show one account's details
valv account switch <name> --provider claude   # globally switch (symlinks ~/.claude)
valv account bind <name>   --provider <p>      # bind to current project
valv account unbind                            # remove this project's binding
valv status                                    # current project's binding
valv status --all                              # all project bindings on this machine

valv image inspect codex                       # show codex image state
valv image inspect claude                      # show claude image state
valv image update codex                        # force rebuild (rarely needed; auto on launch)
valv image cleanup --apply                     # prune Valv-managed Docker artifacts
```

## Troubleshooting quickies

- **`unknown command` from valv** → you have a stale binary. Rerun `mage install` from the Valv repo, confirm with `which valv`.
- **`codex: not logged in` inside container** → the OTHER provider isn't bound to the project. `valv account bind <name> --provider codex`.
- **`Provider image not installed` from `valv image inspect`** → first launch hasn't rebuilt yet. Just run `valv claude` or `valv codex` and it builds on demand.
- **Permission errors on mount paths** → make sure Docker Desktop has filesystem access to `~/Library/Application Support/valv/`.

## What's pinned where

Valv stores managed account homes under:

```
~/Library/Application Support/valv/providers/<provider>/profiles/<account-name>/
```

`valv` mounts the project-pinned account's home into the container at the standard config dir (`/home/valv/.claude` or `/home/valv/.codex`) and sets the CLI's config-dir env var so the right credentials get loaded. No global state pollution between projects.
