# Valv Architecture Notes

## Core direction

Valv should present a clean API surface while routing requests to sandboxed CLI runtimes such as Claude Code, Codex, and Gemini CLI.

Recommended defaults:

- **Fresh by default**: do not pass provider resume flags unless explicitly requested.
- **Session namespaces** keyed by **provider + profile + project**.
- **Optional resume mode** for continuing provider-native sessions.
- **Optional cwd/workspace mode** when the caller wants project files mounted.
- **Profile-based auth** for multi-login support.

A better term than “stateless” for the default behavior is **fresh** or **non-resume**. The key idea is that the default should *not* continue old conversations, even if the runtime keeps profile login state and optional warm workers.

---

## Runtime model

### Recommended layers

1. **Router / control plane**
   - Go CLI / server
   - profile selection
   - project selection
   - session lifecycle
   - provider routing
   - logs / diagnostics / config

2. **Provider adapters**
   - Claude adapter
   - Codex adapter
   - Gemini adapter

3. **Sandboxed runtimes**
   - separate container images per provider
   - explicit profile mounts
   - optional project mounts
   - optional session persistence

---

## Default behavior

### Fresh mode
Default behavior should be:

- no `resume` flag passed to provider CLI
- route by `(provider, profile, project)`
- fresh conversation execution
- optional warm container reuse
- optional project mount only if requested
- login state can persist at the profile level
- conversation state does **not** continue unless requested

### Resume mode
Opt-in behavior:

- continue a prior provider-native session
- routed by `(provider, profile, project, session_id)`
- uses provider-native resume support where available

### Ephemeral mode
Stricter than fresh:

- no resume
- no project memory/config mount
- no persistent project artifacts
- one-off invocation

---

## Multi-login profiles

### What a profile represents

A **profile** should represent an **identity/account**, not a project.

Examples:

- `claude-personal`
- `claude-work`
- `codex-personal`
- `codex-work`

One profile can back many projects.

That means:

- one login per profile
- many projects can reuse that profile
- each project can still keep separate fresh runs and optional resume sessions

### Correct model

Do **not** make each project container own its login forever.

Instead:

- login once per **provider-profile**
- store that auth in a **persistent profile store**
- mount that profile store into whichever project/session container needs it

This is the cleanest design for multi-login support.

---

## First-time login flow

Best practice is to separate **profile enrollment** from ordinary task execution.

Recommended commands:

- `valv profile login claude-work`
- `valv profile login codex-personal`
- `valv profile status`
- `valv profile logout claude-work`

### How login should work

For a new profile:

1. Start a **dedicated enrollment container** for the selected provider/profile.
2. Mount that profile’s persistent storage.
3. Let the provider’s official login flow run.
4. Persist the resulting auth state in the profile store.
5. Reuse that profile store in future request/session containers.

### Important behavior

- browser login usually happens **once per profile**
- not once per project
- not once per request

If the provider supports device auth, that can be used instead of direct browser launch.

---

## Persistence across restarts

Login state should persist across restarts **only if the profile store is persistent**.

This means:

- do **not** keep auth only in an ephemeral container filesystem
- use a persistent Docker volume or host-managed profile directory
- any new container can reuse the same login if it mounts the same profile store

So the answer is:

- **persist across restart?** yes, if the profile store persists
- **persist across container replacement?** yes, if the new container mounts the same profile store
- **persist forever?** not guaranteed; provider auth can expire or be revoked

---

## Projects and sessions

### Project mapping

Each project should point to:

- provider
- profile
- endpoint
- default mode (`fresh`, `resume`, or `ephemeral`)
- optional cwd/workspace policy

The user’s app or client can store JSON config per project so that the project automatically uses the correct Valv endpoint, profile, and project mapping.

### Recommended routing key

Use:

`provider + profile + project`

not just `profile + project`

This avoids collisions between providers and keeps runtime behavior explicit.

### Session key for resume

Use:

`provider + profile + project + session_id`

for explicit resume flows.

---

## Container strategy

### Per-request cold container
Pros:
- strongest isolation

Cons:
- slower startup
- likely too slow as the default

### Warm worker pool
Pros:
- lower latency
- good compromise

Cons:
- requires careful reset logic

### Session containers
Pros:
- ideal for explicit resume behavior
- maps naturally to provider-native session continuity

Cons:
- more lifecycle management

### Recommendation

Use:

- **fresh mode**: warm worker or per-project worker without resume
- **resume mode**: session-based container
- **ephemeral mode**: stricter one-off execution

The concern that mattered most was provider resume behavior. For that, **session-based containers** are the best fit.

---

## Separate provider images

Use separate images for:

- Claude Code
- Codex
- Gemini CLI

Reasons:

- different auth/config models
- different state/cache layouts
- different cwd behavior
- easier isolation
- easier debugging

Keep the public API unified, but keep provider runtimes separate.

---

## README notes

The README should clearly state:

- this tool is intended for **single-user account use**
- it is **not** intended for shared personal subscription accounts
- users are responsible for following the terms and conditions of Claude Code, Codex, Gemini CLI, and any other integrated client
- technical capability does not guarantee permitted usage under provider terms

Suggested wording:

> Valv is intended for single-user account usage. It is not designed as a way to share personal subscription accounts across multiple users. Users are responsible for ensuring that their use of Claude Code, Codex, Gemini CLI, and any other integrated client complies with the applicable terms and conditions of those services.

---

## Recommended mental model

The cleanest mental model is:

- **Profiles are identities**
- **Projects attach to profiles**
- **Fresh runs do not resume**
- **Resume is explicit**
- **Auth persists at the profile level**
- **Conversation continuity persists at the session level**
- **Project state is scoped by provider + profile + project**

That gives a practical and robust architecture for Valv.
