# 08 — Interfaces

Every interface is a front for the same service (`adm/service`): propose,
simulate, approve, reject, apply, explain, reconcile, leash. Nothing is
possible in one that is not gated identically in the others.

## Console

The ADM console extends Fleet's UI with four surfaces:

- **Intents.** The list of intents with state, scope, the plan's tier and
  the drift count from the last reconciliation. An intent's page shows the
  plain-language text, the compiled predicates, the native step per
  platform, the verification, the GitOps rendering and a timeline of every
  ledger row.
- **Proposals inbox.** Plans waiting for approval, grouped by tier, with the
  risk reasons, the simulation (hosts by platform, drift, waves, estimated
  time to effect), the explanation and the diff. Approve, reject, or ask the
  compiler to revise.
- **Ask.** A text box. The answer is a proposal, not a chat: the compiled
  intent, its plan and what happens next.
- **Autonomy.** Per pattern: the leash state, what it would take to relax
  it, the last incident, and the tenant's floors and invariants.

Fleet's existing pages stay as they are; ADM's verification policies appear
in Fleet's policies view, and ADM's actions appear in Fleet's activity feed.

## CLI: `admctl`

```
admctl [--state-dir DIR] [--demo | --fleet-url URL --fleet-token TOKEN] [--claude] [--principal NAME] <command>

  capabilities                       list the catalog
  propose "<text>" [--author NAME]   compile plain language to an intent and a plan
  plans                              list plans
  plan <id>                          show a plan (JSON)
  simulate <id>                      dry-run: which hosts drift, what would change
  explain <id>                       narrate what, where, how, why this tier, how to undo
  approve <id> [--note] [--ticket]   approve as --principal (a second person)
  reject <id> [--note]               reject
  apply <id> [--dry-run]             execute wave by wave with verification and rollback
  leash <pattern>                    adaptive-autonomy state of a pattern
  event --type T [--host N --platform P --subject S]
                                     feed an event to the reconciler
  mcp                                serve the MCP write plane over stdio
```

## MCP

Two servers, one principle:

- **Fleet MCP** (`cmd/fleet-mcp`, upstream) is the read plane: hosts,
  inventory, policies, vulnerabilities, live queries. Every tool is annotated
  read-only; `run_live_query` is the only one that touches devices, to read.
- **ADM MCP** (`adm/mcp`) is the write plane: `adm_list_capabilities`,
  `adm_propose`, `adm_list_plans`, `adm_get_plan`, `adm_simulate`,
  `adm_explain`, `adm_approve`, `adm_reject`, `adm_apply`, `adm_leash`,
  `adm_event`. `adm_apply` is annotated destructive; it refuses a plan whose
  tier needs an approval it does not have. `adm_approve` acts as the server's
  principal (the human who launched it), so an agent cannot approve as
  someone else and cannot approve its own approve-tier proposals.

A Claude Code or Claude Desktop configuration:

```json
{
  "mcpServers": {
    "fleet": { "command": "/path/to/fleet-mcp", "args": ["-transport", "stdio"], "env": { "FLEET_BASE_URL": "...", "FLEET_API_KEY": "...", "MCP_AUTH_TOKEN": "..." } },
    "adm":   { "command": "/path/to/admctl", "args": ["--fleet-url", "...", "--fleet-token", "...", "--principal", "alice", "mcp"] }
  }
}
```

The conversation an operator has with their agent then reads naturally:
*"Which finance laptops still allow USB drives?"* (Fleet MCP, a live query)
→ *"Block it on those, ask before applying"* (ADM MCP, `adm_propose`) →
*"Explain the plan"* → *"Approve"* (as the operator) → *"Apply"* → *"Did it
verify?"* (`adm_explain`, the ledger).

## Chat

Slack and Teams get the same four verbs (propose, explain, approve, apply)
as slash commands and as buttons on proposal notifications. Approvals from
chat carry the chat identity as the approver and land in the same
`plan.Approval`. A CAB-tier plan posts to the change channel with the
ticket link and waits.

## GitOps

An intent written by hand in the `adm:` block of the GitOps repo is as valid
as one compiled from language: `fleetctl gitops` (extended) applies it, the
planner compiles it, and the risk engine tiers it. ADM's own changes are
merge requests against the same repo; a merged MR is the persistent act.

## API and webhooks

The service is exposed as a REST API beside Fleet's (`/api/adm/v1/intents`,
`/plans`, `/plans/{id}/approve`, `/plans/{id}/apply`, `/events`) with the
same API-only user model, and it publishes webhooks for proposals awaiting
approval, verified plans, rollbacks and incidents, which is how ITSM
integration and the lakehouse sink attach.
