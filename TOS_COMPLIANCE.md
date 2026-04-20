# Valv — Terms of Service Compliance

Status: analysis complete for Anthropic side · 2026-04-18
Scope covered: Valv-as-Docker-runtime hosting Claude Code (and, pending a separate OpenAI-side review, Codex) with per-container OAuth identity pinning.
Out of scope: the retracted earlier shape that wrapped `claude --bare -p` behind an OpenAI-compatible HTTP API. That scope is explicitly not covered here because it is no longer Valv's direction.

## 1. Purpose and Current Scope

Valv is a macOS-first control plane that runs AI coding-assistant CLIs inside Valv-managed Docker containers. For the scope this document covers:

- Each container hosts an ordinary, unmodified Claude Code (or Codex) CLI.
- Each container is pinned to exactly one OAuth identity at a time (for example, a work seat or a personal subscription, but never both in the same container).
- The user interacts with the CLI inside the container for ordinary coding-assistant work — writing, editing, reviewing, testing, and running code for the project the container is scoped to.
- No HTTP shim, reverse proxy, OpenAI-compatibility layer, or third-party credential routing sits between the user and the CLI.
- Valv runs on the user's own machine, against the user's own subscriptions; it is not a distributed product that other users log into with their own accounts.

This is the shape covered by this document. Any deviation from this shape (for example, reintroducing an inference-gateway HTTP surface, or routing multiple end users through a single Pro/Max credential) invalidates the analysis below and needs its own compliance review.

## 2. Conclusion

On the Anthropic side, this scope complies with Anthropic's Terms of Service, Acceptable Use Policy, and the Claude Code legal-and-compliance guidance. It is materially more aligned with Anthropic's published recommendations than running Claude Code directly on the bare host would be, because Anthropic's own permission-modes documentation points users toward isolated containers/VMs for agentic or permissive modes.

The Codex (OpenAI) side of Valv is not yet covered by this document; see §7.

## 3. Compliance Analysis

The analysis rests on four independent threads, each grounded in verbatim source material listed in §8.

### 3.1 Ordinary Use of Claude Code

Anthropic's Claude Code legal-and-compliance page states that OAuth authentication "is intended exclusively for purchasers of Claude Free, Pro, Max, Team, and Enterprise subscription plans and is designed to support ordinary use of Claude Code and other native Anthropic applications."

In the scope of this document, each Valv container runs Claude Code as a coding assistant for a specific project context. That is the native, marketed, documented purpose of the tool. No transformation, repurposing, or re-shaping of Claude Code into a different product occurs. The container is transport and isolation, not capability intermediation.

### 3.2 Docker Isolation Aligns With Anthropic's Permission-Modes Guidance

Anthropic's Claude Code permission-modes documentation states, regarding `bypassPermissions`: "Only use this mode in isolated environments like containers, VMs, or devcontainers without internet access, where Claude Code cannot damage your host system."

That recommendation also surfaces as general guidance for permissive or agentic Claude Code configurations across related pages. Running Claude Code inside a Docker container — as Valv does — operationalizes Anthropic's own recommendation rather than fighting it. The qualifier about internet access is specific to the `bypassPermissions` mode; for ordinary agentic modes such as `acceptEdits`, the value is host-isolation from Claude Code's filesystem writes and command execution, which Docker provides.

Valv's container-per-project model therefore reduces, rather than increases, the blast-radius concern Anthropic's documentation flags.

### 3.3 Per-Container OAuth Identity Separation Is Legitimate Multi-Account Use

Anthropic's Acceptable Use Policy prohibits multi-account activity only in specific, narrowly defined cases:

- "Circumvent a ban through the use of a different account, such as the creation of a new account, use of an existing account, or providing access to a person or entity that was previously banned."
- "Coordinate malicious activity across multiple accounts to avoid detection or circumvent product guardrails or generating identical or similar inputs that otherwise violate our Usage Policy."

Neither clause targets the legitimate professional pattern of holding distinct accounts for distinct contractual and operational contexts. A developer who holds (a) a work-provided Claude for Work / Team / Enterprise seat under Commercial Terms, and (b) a personal Pro or Max subscription under Consumer Terms, is operating under two separate contracts with two separate legitimate purposes and, typically, two separate payers. Keeping work code and credentials isolated from personal code and credentials is standard compliance hygiene for proprietary-code and NDA-bound work, not evasion.

Valv's per-container OAuth identity pinning implements that hygiene. Work containers use work credentials; personal containers use personal credentials. Valv does not share credentials across containers, does not rotate between accounts within a single session to multiply quota, and does not use multiple accounts to coordinate a single user's workload.

Constraint that must be maintained: the "work" account referred to in Valv's design must be a legitimately separate contractual relationship (employer-provided Team/Enterprise seat, or otherwise clearly delineated from a personal subscription), not a second personal Max bought by the same user specifically to increase available throughput. See §6.

### 3.4 Valv Is Not A Third-Party Product Routing Others' Credentials

The Claude Code legal-and-compliance page draws a specific line against products that route other users' credentials: "Developers building products or services that interact with Claude's capabilities, including those using the Agent SDK, should use API key authentication through Claude Console or a supported cloud provider. Anthropic does not permit third-party developers to offer Claude.ai login or to route requests through Free, Pro, or Max plan credentials on behalf of their users."

Valv, in the scope of this document, does not do any of that:

- Valv does not offer a login surface for other users' Anthropic accounts.
- Valv does not route any user's requests through another user's credentials.
- Valv runs on the user's own machine and uses the user's own subscriptions to do the user's own coding work.
- Valv is a thin runtime — it starts, stops, isolates, and pins credentials into containers; it does not sit between Claude and a consumer of Claude's capabilities as an intermediary service.

The thin-runtime framing is structurally closer to `docker run` or `docker compose` than to an API-wrapper product. Docker itself is not treated as "a product that interacts with Claude's capabilities" simply because someone runs Claude Code inside it; the same reasoning applies to Valv in this scope.

## 4. Explicitly Out Of Scope — Shapes Valv Does Not Implement

The compliance conclusion above depends on Valv *not* doing the following. These are listed explicitly so that any future work that reintroduces them triggers a fresh compliance review.

- No OpenAI-compatible HTTP surface that fronts `claude --bare -p` or any other Claude Code entry point. Reshaping Claude Code into a general-purpose inference backend for arbitrary apps is not covered by "ordinary use of Claude Code" and falls under the legal-and-compliance directive that products interacting with Claude's capabilities should use API-key authentication. This shape was explored earlier in Valv's design and has been retracted.
- No PTY keystroke injection into a Claude Code TUI controlled by a wrapper process. The documented automation surface for Claude Code is its headless CLI flags (`--bare`, `-p`, `setup-token`) and the Agent SDK, not simulated-human input against its interactive TUI. This shape is not part of Valv's design.
- No distribution model in which other users log into Valv with their own Pro/Max credentials. Valv is a single-user tool running on the user's own machine.
- No quota-multiplication via multiple personal subscriptions owned by the same user. The multi-account posture Valv supports is distinct contractual relationships (work vs personal), not parallel copies of the same relationship.
- No circumvention of any account ban, rate limit, content policy, or safety guardrail. Valv does not disable, strip, or bypass any Claude Code guardrail; it runs the CLI as shipped.

## 5. Compliance-Relevant Constraints Valv Must Maintain

These constraints follow from the analysis in §3 and must be preserved as Valv evolves. Each is stated as a requirement on the system so that a future code change touching it surfaces as a compliance-relevant decision rather than a silent drift.

1. **One OAuth identity per container, pinned at container start.** A container must not rotate between accounts mid-session; that pattern looks like quota-multiplication rather than identity separation.
2. **No inter-container credential sharing.** Work-container credentials must not be reachable from personal-container processes and vice versa. This is both a ToS posture and a security posture.
3. **No HTTP surface fronting Claude Code's CLI output.** Valv may expose management surfaces (start, stop, status, update) to the host, but must not expose an OpenAI-compatible or generic LLM-inference endpoint that routes to a Claude Code CLI instance.
4. **No distribution as a multi-user service.** If Valv is ever packaged so that multiple end users log into one Valv instance with their own Claude subscriptions, the third-party-product clause binds and the Agent-SDK-with-API-key path becomes the required form factor.
5. **Claude Code runs unmodified.** Valv does not modify, patch, or intercept Claude Code's behavior. It provides the container, the credentials, and the file-system boundary; the CLI inside is stock.
6. **Ordinary-use framing preserved in documentation.** Valv's own documentation describes its purpose as per-project isolation and identity separation for ordinary coding-assistant work, not as an inference-backend or automation-reshaping tool. The documented framing matters because it signals intent if the compliance posture is ever reviewed.

## 6. The Work-vs-Personal Nuance

The multi-account compliance story depends on the work and personal accounts being distinct contractual relationships, not two copies of the same consumer subscription.

Clean case — fully in-bounds:
- Work account is a Claude for Work, Team, or Enterprise seat, paid by the employer on Commercial Terms.
- Personal account is a Pro or Max subscription on Consumer Terms.
- Two distinct contracts, two distinct payers, two distinct legitimate purposes.

Also clean — employer-sponsored personal-tier seat:
- Work account is a Pro or Max seat the employer provides for work use.
- Personal account is a separate Pro or Max the developer pays for.
- Contractual separation is thinner than the Team/Enterprise split, but the accounts still exist for different legitimate purposes with different payers.

Concerning case — requires a separate judgement call:
- Both accounts are personal-tier subscriptions paid by the same individual, held in order to run more parallel agents or ingest jobs.
- The AUP does not explicitly forbid this, but the "coordinate ... inputs that otherwise violate our Usage Policy" language reads against it when the pattern is specifically quota-multiplication.

Valv does not need to enforce this distinction mechanically, but users of Valv should be aware that the compliance conclusion above assumes the clean case.

## 7. Codex / OpenAI Side — Deferred

Valv also supports Codex containers. The OpenAI terms governing Codex CLI use on a subscription account have not yet been reviewed against Valv's scope. The Anthropic-side reasoning in this document does not carry over to Codex automatically; OpenAI's Service Terms, Usage Policies, and Codex-specific documentation need their own pass.

The expected shape of that review is analogous — ordinary use, Docker isolation, legitimate work-vs-personal identity separation, no third-party credential routing — but the verbatim clauses and their scope differ between Anthropic and OpenAI, and the conclusions cannot be assumed equivalent without evidence.

This section is intentionally a placeholder until the OpenAI-side review is performed.

## 8. Sources

All Anthropic-side sources were fetched on 2026-04-17 during the research pass that grounded this document. Verbatim quotes below are the specific clauses relied on in §3.

### 8.1 Anthropic Consumer Terms of Service

URL: https://www.anthropic.com/legal/consumer-terms

Automation clause (verbatim):
> "Except when you are accessing our Services via an Anthropic API Key or where we otherwise explicitly permit it, to access the Services through automated or non-human means, whether through a bot, script, or otherwise."

Relevance: establishes that automation requires either API-key access or explicit permission. Claude Code is an explicitly permitted Anthropic native application for CLI automation, scripting, and multi-agent orchestration as shipped.

### 8.2 Anthropic Acceptable Use Policy

URL: https://www.anthropic.com/legal/aup

Agentic use (verbatim):
> "Agentic use cases must still comply with the Usage Policy."

Guardrail bypass (verbatim):
> "Intentionally bypass capabilities, restrictions, or guardrails established within our products for the purposes of instructing the model to produce harmful outputs (e.g., jailbreaking or prompt injection) without prior authorization from Anthropic."

Multi-account evasion (verbatim):
> "Circumvent a ban through the use of a different account, such as the creation of a new account, use of an existing account, or providing access to a person or entity that was previously banned."

Malicious coordination (verbatim):
> "Coordinate malicious activity across multiple accounts to avoid detection or circumvent product guardrails or generating identical or similar inputs that otherwise violate our Usage Policy."

Relevance: defines the narrow scope of prohibited multi-account activity. Valv's work-vs-personal pattern does not match either clause.

### 8.3 Anthropic Commercial Terms of Service

URL: https://www.anthropic.com/legal/commercial-terms

Relevance: governs Claude for Work, Team, and Enterprise seats, which is the contract under which a work account in the Valv design typically operates. No competing-product clause in Commercial Terms is triggered by Valv in this scope, because Valv is not building or offering a competing LLM service.

### 8.4 Claude Code — Legal and Compliance

URL: https://code.claude.com/docs/en/legal-and-compliance

OAuth intent (verbatim):
> "OAuth authentication is intended exclusively for purchasers of Claude Free, Pro, Max, Team, and Enterprise subscription plans and is designed to support ordinary use of Claude Code and other native Anthropic applications."

Third-party developer directive (verbatim):
> "Developers building products or services that interact with Claude's capabilities, including those using the Agent SDK, should use API key authentication through Claude Console or a supported cloud provider. Anthropic does not permit third-party developers to offer Claude.ai login or to route requests through Free, Pro, or Max plan credentials on behalf of their users."

Relevance: the two clauses that the earlier Valv scope would have run up against. The current Valv scope does not, because Valv does not route other users' requests through consumer credentials and does not sit as a product intermediary between Claude and a downstream consumer.

### 8.5 Claude Code — Permission Modes

URL: https://code.claude.com/docs/en/permission-modes

`bypassPermissions` isolation recommendation (verbatim):
> "Only use this mode in isolated environments like containers, VMs, or devcontainers without internet access, where Claude Code cannot damage your host system."

Relevance: Anthropic's own recommendation that permissive modes run inside containers/VMs. Valv's container-per-project design operationalizes this recommendation.

### 8.6 Claude Code — CLI Reference and Headless

URLs:
- https://code.claude.com/docs/en/cli-reference
- https://code.claude.com/docs/en/headless

Relevance: enumerates the documented automation surface for Claude Code (`--bare`, `-p`, `setup-token`, Agent SDK). Valv runs Claude Code as shipped and does not extend or alter this surface.

### 8.7 Claude Code — Sub-Agents and Agent Teams

URLs:
- https://code.claude.com/docs/en/sub-agents
- https://code.claude.com/docs/en/agent-teams

Agent-teams framing (verbatim):
> "Agent teams let you coordinate multiple Claude Code instances working together. One session acts as the team lead, coordinating work, assigning tasks, and synthesizing results."

Experimental status (verbatim):
> "Agent teams are experimental and disabled by default."

Relevance: confirms multi-agent Claude Code use is a documented, marketed feature. The container-per-project pattern Valv implements is consistent with the same multi-instance use case Anthropic describes, just with stronger isolation.

### 8.8 Claude Code — Authentication

URL: https://code.claude.com/docs/en/authentication

Relevance: documents OAuth and API-key authentication paths. Valv uses OAuth within each container for subscription-backed use, which is the intended path for ordinary Claude Code use on consumer subscriptions.

### 8.9 Privacy and Training Opt-Out

URL: https://privacy.claude.com/en/articles/10023580-is-my-data-used-for-model-training

Training opt-out (verbatim):
> "Users can disable model improvement through Privacy Settings."

Scope (verbatim):
> "This page applies to Claude Free, Pro, Max and when accounts from those plans use Claude Code. Session data handling follows the same rules as regular chats—used for improvement only if opted in, except for safety flagged content."

Relevance: a Valv user on a Pro or Max subscription can opt their personal account out of training-data use through Privacy Settings. This does not affect ToS compliance but is relevant to the overall privacy posture Valv users may want.

### 8.10 Consumer Terms and Privacy Policy Update

URL: https://www.anthropic.com/news/updates-to-our-consumer-terms

Relevance: confirms the training-data opt-out scope and mechanics referenced in §8.9.

### 8.11 Anthropic Usage Policy Update News

URL: https://www.anthropic.com/news/usage-policy-update

Relevance: points to published Help Center guidance on agentic use. The specific Help Center article that most directly covers agentic compliance was not reachable at a canonical URL during the 2026-04-17 research pass; this news item is the auxiliary reference for that guidance.

### 8.12 Claude Code — Overview

URL: https://code.claude.com/docs/en/

Marketing (verbatim):
> "Run agent teams and build custom agents. Spawn multiple Claude Code agents that work on different parts of a task simultaneously."

Relevance: multi-agent Claude Code use is explicitly a marketed capability. Valv's container-per-project model is consistent with this capability, applied for isolation and identity separation.

## 9. Revision Notes

- 2026-04-18 — initial version. Covers the pivoted Valv scope (Docker runtime, per-container OAuth identity separation) for ordinary Claude Code coding-assistant use. Explicitly excludes the retracted OpenAI-compatible-shim scope. OpenAI / Codex side deferred.
