# Contributing

## Workflow

Work in an active worktree, not in the bare repository root.

- keep feature work in `main/` or a dedicated lane worktree
- keep local-only notes in `.worklog/`
- do not commit `.worklog/`

Read [AGENTS.md](./AGENTS.md) and [PLAN.md](./PLAN.md) before making non-trivial changes.

Compatibility change policy:

- if Codex adds/changes CLI flags or payload semantics that affect `valv api`, update [`codex-openai-compatibility.json`](./codex-openai-compatibility.json) before merging and open/track an issue when runtime mapping is missing or outdated in this repo.
- check `generated_at_utc`, `last_reviewed_utc`, and `review_interval_days` in the manifest before touching API compatibility changes; if the file is stale and no issue is being handled, schedule a compatibility sweep in the next change.

## Tooling

Install Mage:

```bash
go install github.com/magefile/mage@v1.17.0
```

Primary local commands:

```bash
mage build
mage test
mage testPkg ./internal/output
mage golden
mage integration
```

`mage test` runs `gofumpt` before the combined `go test -race -cover ./...` gate. Normal package goldens run through that same `go test` pass, while Docker-backed transcript goldens stay in `mage integration`.

Run the final local binary validation separately after the CI-equivalent gate:

```bash
mage test
mage build
```

## Disposable Dev-Home Flow

Use the disposable dev-home workflow for local runtime validation that should not touch your real provider home:

```bash
mage dev:home
mage dev:run "manage update"
mage dev:run "codex --help"
mage dev:reset
mage dev:clean
```

`mage dev:run` builds the local binary, points `HOME` at a disposable temp directory, preserves access to the host Docker CLI configuration, and uses a dev-tagged Valv Codex image.

## Goldens And Integration

Run the full golden suite or refresh all tracked goldens with:

```bash
mage golden
mage goldenUpdate
```

Docker-backed integration coverage stays in:

```bash
mage integration
```

When a change can affect visible Codex runtime behavior, run `mage golden` and `mage integration`.

## Hooks

There are no repo-local contributor hook wrappers in the worktree. If you use local Git hooks, point them at `mage test` or a narrower Mage target that matches the scope of the hook.
