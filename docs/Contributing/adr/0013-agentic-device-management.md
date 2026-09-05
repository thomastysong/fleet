# ADR-0013: Agentic Device Management (ADM) as a control plane on the Fleet substrate

## Status

Proposed

## Date

2026-09-05

## Context

Fleet manages devices the way every MDM has: settings (profiles,
declarations, policies, scripts, software) are assigned to groups and
reconciled by timers. In this codebase the reconcilers run on fixed
intervals (`mdm_apple_profile_manager`, `mdm_windows_profile_manager` and
`mdm_android_profile_manager` every 30 seconds, with a comment asking to
re-evaluate the interval at scale; `vulnerabilities` and `maintained_apps`
hourly; osquery policies, labels and host vitals hourly; orbit configuration
polled every 30 seconds). Actions on hosts are shell and PowerShell scripts
through fleetd's runner (five-minute cap, 5,000-host batches) apart from a
handful of native writes (BitLocker through WMI, the Windows `mdm_bridge`
table's `ApplyLocalManagementSyncML`, LUKS escrow, the managed local
account). There is no intent layer, no risk model (policies are boolean and
conditional access sends one `compliant` bit), no feedback loop from
outcomes to behaviour, and the MCP server (`cmd/fleet-mcp`) is read-only by
construction. ADR-0011 has started the move from polling to push with the
agent WebSocket "check now" nudge.

Xavier Device Management, a contemporary single-console MDM, adds developer
package-manager management (Homebrew, npm, pip, RubyGems), dependency CVE
and runtime end-of-life scanning, upstream release watching with checksum
verification and Munki import, removable-storage audit and blocking,
website and application blocking including AI providers, a strictly
read-only MCP server, and digital signage on iPads, Apple TVs and Android
tablets. It, too, is settings on timers operated by people.

The organisation sponsoring this fork already runs an internal agent that
perceives from a lakehouse, Fleet live queries, ITSM and Git, reasons with a
model, scores risk against a change model, acts through merge requests,
tickets, staged releases and the Fleet MCP server, auto-executes low-risk
changes, waits for a human on medium and high, and writes every outcome
back so accepted suggestions loosen the leash for that pattern. Fleet made
two parts of that loop cheaper (live query and GitOps); the rest was built
around it. The question is whether that loop should be the product.

Constraints:

- A language model must never be the component that decides what runs on
  a fleet unattended.
- Fleet's substrate (MDM protocols, osquery, GitOps, RBAC, activities) must
  stay intact and upstreamable; the fork tracks upstream `main`.
- Windows Autopilot, the out-of-box experience and Entra conditional access
  are Microsoft's; a design that ignores them is not deployable in a
  Windows estate.
- Scripts are sometimes the only option; a design that forbids them is not
  deployable either.

## Decision

We will build ADM (Agentic Device Management) as a control plane that
drives the Fleet substrate and Microsoft Graph, with these commitments:

1. **Intents are the unit of management.** An intent is a platform-agnostic,
   persistent, versioned statement of desired state (`adm/intent`),
   compiled from plain language by two compilers (deterministic rules and
   a Claude-backed compiler that must answer through one typed tool and is
   validated against the catalog) and rendered to GitOps as the reviewable,
   revertible form.
2. **A capability catalog binds intents to native OS interfaces.** Every
   capability names the CSP node, WMI class, Win32 API, DDM declaration,
   Endpoint Security or NetworkExtension hook, D-Bus method, udev rule,
   netlink family, dconf key or AMAPI field it uses on each platform, with
   an osquery verification and a base risk. Scripts exist only as the
   `script.run` capability, flagged and floored at the approve tier.
3. **A deterministic risk engine assigns autonomy.** Score, tiers
   (observe, auto, canary, approve, CAB), policy floors, blast-radius caps,
   the intent's own cap, hard invariants that no score or approval can
   override, and an adaptive leash keyed on the pattern of change, derived
   from the outcome ledger: autonomy is earned per pattern per tenant and
   revoked by incidents.
4. **Execution is check-before-apply, in waves, verified on the devices it
   touched, and rolled back on failure.** Every verification is also
   persisted as a Fleet policy.
5. **Reconciliation is event-driven.** OS watchers, substrate events and
   external feeds publish typed events; affected intents are re-evaluated
   for the affected host as one-host micro-plans; a periodic sweep is the
   safety net. The ADM agent extension to fleetd carries watchers, a signed
   local intent cache and the local fast path.
6. **Every outcome is a ledger row with a lakehouse schema**, read by the
   risk engine (the leash), by regression detectors (telemetry after a
   change), by the compiler (retrieval of accepted intents) and by humans
   (`explain`).
7. **Zero-touch is intent-driven on every platform**, and Windows Autopilot
   is driven on its write side through Microsoft Graph (hardware-hash
   import, group tags as fleets, deployment-profile assignment, sync) on
   top of Fleet's Entra automatic enrollment and ESP-gated setup experience.
8. **AI agents get a write plane with the same gates as humans.** Fleet's
   MCP server remains the read plane; ADM's MCP server exposes propose,
   simulate, explain, approve (as a bound principal, never the author) and
   apply.
9. **ADM ships first as a separate module and process beside Fleet
   (`adm/`, like `cmd/fleet-mcp`)**, talking to Fleet over its REST API with
   an API-only user, so it can be deployed, upgraded and killed
   independently of the MDM substrate. Moving it into the Fleet server as
   an `adm` bounded context, per the modular-monolith direction, is a later
   phase that changes no interface.

## Consequences

Positive:

- Operators state outcomes; ADM owns the mechanism on every platform.
- Changes land in milliseconds to seconds through native interfaces, are
  verified where they land, and are reversible by construction.
- Autonomy grows with evidence and shrinks with incidents, per pattern, with
  a deterministic and inspectable engine, so the language model's role is
  bounded to understanding language.
- Every Fleet and Xavier capability in scope has a place in the catalog,
  and the additions (intents, autonomy, event speed, native actions,
  learning, Autopilot write side, agent write plane) are additive to the
  substrate rather than a fork of it.
- Xavier's differentiators become catalog entries rather than a second
  product.

Negative and debts:

- Two systems of record during phase 1 (ADM's store and Fleet's tables)
  until the bounded-context phase.
- Per-host delivery of Apple declarations needs a substrate change
  (label per host today); Windows CSP writes go through the MDM command
  queue until the agent extension exists.
- The Apple agent extension needs Endpoint Security entitlements and a
  notarised system extension; until then macOS enforcement is declarative
  only.
- Leash defaults (five acceptances per relaxation step, 30-day incident
  cooldown) are starting values that the ledger must validate.
- A model in the loop adds a data-classification decision per tenant; the
  deterministic compiler is the no-model path.

## Alternatives considered

- **Extend Fleet's policy automations into an "agent".** Rejected: fixed
  rules with no intent layer, no risk model and script-based remediation
  reproduce the cron-and-script shape with a new name.
- **Build ADM on Intune / Graph only.** Rejected: no live query (inventory
  hours old), imperative and eventually-consistent configuration with no
  diffable form, and no Linux. Retained for what only it does (Autopilot,
  conditional access), driven through Graph.
- **Fork Xavier's approach: a new MDM server and console.** Rejected:
  re-implementing APNs, OMA-DM and AMAPI buys nothing that Fleet does not
  already provide; the differentiators are capabilities, not a server.
- **A conversational assistant over Fleet with tool use.** Rejected: an
  agent with direct tools and no intent, risk or ledger layer has exactly
  the failure modes ADM is designed to remove.
- **Put ADM inside the Fleet server from day one.** Deferred: the model in
  the loop argues for independent deployability first; the bounded-context
  conventions are followed so the move is mechanical.

## References

- `adm/docs/` (design documents 00–10) and `adm/` (the scaffold).
- ADR-0007 (activity bounded context), ADR-0011 (agent WebSocket
  transport), ADR-0012 (conditional requests for the osquery config
  endpoint).
- `cmd/fleet-mcp` (Fleet MCP server), `orbit/pkg/table/mdm_bridge`,
  `orbit/pkg/table/ai_tools`, `server/microsoft/msgraph`,
  `server/service/conditional_access_microsoft.go`.
- Xavier Device Management announcement (September 2026) listing its
  differentiators; the earlier open-source Xavier project (a frontend for
  MicroMDM and NanoMDM, later its own Node.js MDM server) is no longer
  public.
