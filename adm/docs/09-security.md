# 09 — Security

ADM puts a language model, an event-driven reconciler and native OS
adapters between operators and every device in a fleet. The design assumes
each of those can be wrong, compromised or manipulated, and bounds the
damage structurally rather than by prompt.

## Threat model

| Actor | Goal | Structural control |
|---|---|---|
| A compromised or manipulated model output (prompt injection through ticket text, a device's hostname, a package description) | Make ADM execute something the operator did not ask for | The model can only emit an Intent IR; the IR is validated against the catalog; the plan is scored and tiered deterministically; invariants cannot be overridden; anything above `auto` waits for a person who is not the author. Device data, ticket text and repo content are data to the compiler, never instructions. |
| An operator with a valid role | Escalate (wipe, remove the agent, disable encryption) | Invariants; CAB floor with a ticket for destructive capabilities; second-person approval; every action attributed and in the ledger and in Fleet's activity feed. |
| An agent holding the ADM MCP | Act as an operator | The MCP runs as a principal; approvals are attributed to it; an agent cannot approve its own proposals; the MCP token is scoped like Fleet's API-only user model (least role). |
| A device (malicious local admin, malware) | Feed false state or false verification | Verification comes from osquery through Fleet's authenticated channel, not from the adapter's own return value; the local fast path holds a signed copy of intents and can only converge towards them, never away; reverts require the captured prior state. |
| The control plane itself compromised | Everything | ADM is separately deployable and killable from the substrate; a kill switch pauses all intents (state `paused`) in one call; the substrate's own RBAC still applies to every REST call; hard invariants are code. |
| The lakehouse or an external feed poisoned | Trigger harmful reconciliation | Events can only make intents stale; they cannot create intents above the tier the pattern has earned, and novel proposals from data always wait for a person. |

## Controls, in the order a change meets them

1. **Catalog validation.** Unknown capability or parameter: rejected.
2. **Scope rule.** Empty scope without `allow_fleet_wide`: rejected.
3. **Invariants.** Violated: the plan is blocked; it cannot be approved.
4. **Floors and caps.** Minimum tier per capability; blast-radius caps.
5. **Score and leash.** Deterministic; the leash only relaxes what the
   history supports and never for destructive changes.
6. **Approval.** Second person at `approve`; ticket at `cab`.
7. **Check before apply.** Nothing is written to a device already in the
   desired state.
8. **Waves and verification.** A canary first when the tier or size asks
   for one; verification on the touched hosts; rollback above the failure
   threshold.
9. **Ledger.** Every step above is a row, with the native call that was
   made.

## Identity and transport

- **Devices** authenticate to the substrate as they do today (node keys,
  host identity certificates with HTTP message signatures where enabled,
  MDM certificates). The ADM agent extension inherits fleetd's identity and
  adds nothing.
- **ADM to Fleet** uses an API-only user with the least role that covers
  the substrate calls it makes (observer for inventory and live query,
  maintainer for profiles, commands and policies, scoped to the fleets ADM
  manages), over TLS, with Fleet's audit attribution.
- **ADM to Microsoft Graph** uses an Entra app registration with the
  minimum application permission (`DeviceManagementServiceConfig.ReadWrite.All`)
  and refuses to follow links off the Graph origin.
- **ADM to the model** sends the catalog, tenant fleet and label names and
  the operator's text; never device inventory, never secrets, never tickets
  verbatim. Provider retention terms are part of the tenant's data
  classification decision; the deterministic compiler works with no model at
  all.
- **Agents to ADM MCP** authenticate with a bearer token bound to a
  principal; the server refuses approvals without a principal.

## Native adapters

- Adapters are code with tests, reviewed once, not scripts reviewed never.
- Each adapter uses the least interface that does the job (a CSP node, not
  a registry hive; a D-Bus method, not a shell).
- On Windows, ADM's SyncML goes through the OS's own local management stack
  with the same access checks the MDM channel has; on macOS, Endpoint
  Security and NetworkExtension run in a signed, notarised system extension
  with user-approved entitlements; on Linux, privileged calls go through
  polkit-governed D-Bus services where they exist.
- Scripts remain possible, flagged, approve-tier, capped in blast radius,
  and never fleet-wide without CAB.

## Audit and evidence

`explain` produces, for any plan: the intent text and author, the compiled
predicates, the native call per platform, the risk score and every reason,
the floor and cap that applied, the leash state, the approval (who, when,
where, ticket), every wave's results, verification per host, rollback if
any, and the ledger rows. That is the evidence package for a compliance
control ("all laptops encrypted", "removable storage blocked on finance
devices") and it is generated, not assembled.

## What ADM does not do

- It does not give the model a shell, a browser or a filesystem. Its tools
  are `emit_intent` and, for the reasoning agent, typed MCP tools with the
  same gates as a person.
- It does not store device secrets (recovery keys, LAPS passwords) itself;
  escrow stays in the substrate's encrypted assets.
- It does not weaken the substrate's own protections: Fleet's RBAC,
  Fleet's MDM certificate model and Apple's and Microsoft's protocol
  guarantees are untouched.
