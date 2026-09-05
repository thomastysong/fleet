# 10 — Roadmap

## What exists in this scaffold

Working, tested Go (`go test ./...` in `adm/`):

- The Intent IR with validation, fingerprints and pattern keys.
- The capability catalog: 27 capabilities with native bindings on up to
  seven platforms each, verification queries and base risks; a test
  guarantees no binding uses a script except `script.run`.
- Two compilers: deterministic rules for the common asks, and the Claude
  compiler (beta Messages API, cached system prompt, `emit_intent` tool,
  server-side refusal fallback, catalog validation of the output).
- The planner: steps per platform, rollback, waves, risk, invariants,
  GitOps rendering, simulation.
- The risk engine: score, tiers, floors, caps, invariants, the leash.
- Native adapters: Windows CSP over the local MDM stack (SyncML compose and
  parse, Replace-then-Add, Check / Apply / Revert for removable storage,
  firewall, screen lock, URL blocklist, remote lock); Apple DDM declarations
  (disk management, screen saver, passcode, software update enforcement,
  content filter) behind a `Pusher`; Linux udev, dconf and login1 adapters
  behind `Runner` and `DBus` interfaces; demo adapters for every binding.
- The executor: check before apply, waves, nudge, verification by live
  query, rollback on failure, dry run, persistence of verifications as Fleet
  policies, the ledger.
- The events bus and the trigger table; the service's `Reconcile` producing
  one-host micro-plans that run unattended when the tier allows.
- The ledger (memory and JSON lines) and leash calibration.
- The Fleet REST substrate client (hosts, count, fleets, labels, live query
  through a temporary saved report, sync scripts, MDM commands, profiles,
  policies) and an in-memory substrate with a demo fleet.
- The Windows Autopilot Graph client (import, poll, tag, profiles, assign,
  sync, delete).
- The service facade and the file store.
- The MCP server (stdio, 11 tools, annotations, principal-bound approvals).
- `admctl`.

## Phases

### Phase 1 — Control plane beside Fleet (next)

- Wire the real adapters end to end through the substrate: Windows CSP
  writes as MDM commands with the `windows_mdm_sync_request` kick, reads via
  `mdm_bridge` live queries; Apple declarations through Fleet's profiles API
  with per-host targeting (a substrate change: label-per-host today, a
  `host_uuids` parameter on the batch endpoint tomorrow).
- The lakehouse sink (Parquet writer, Iceberg or Delta table) for
  `adm_outcomes`, `adm_intents`, `adm_plans`, `adm_leash`.
- The release watcher and Munki import for `software.release.watch`.
- The `ruby_gems` osquery table and the endoflife.date connector.
- Chat (Slack) approvals; ITSM (ServiceNow, Jira) change requests for the
  CAB tier.
- The console: intents, proposals inbox, ask, autonomy.
- Fleet substrate additions, upstreamable: DDM status subscriptions and
  activation predicates; a `nudge` API over the agent WebSocket; policy
  results with reason codes for conditional access.

### Phase 2 — The agent extension

- fleetd extension with OS watchers (ETW/WMI, Endpoint Security, udev,
  D-Bus), the local intent cache with signed intents, the local fast path
  for proven patterns, and local native writes (`ApplyLocalManagementSyncML`,
  WMI, Win32; Apple system extension with ES and a content filter; Linux
  system bus and privileged exec).
- Bidirectional agent WebSocket (events upstream), building on ADR-0011.
- Regression detectors over telemetry after changes ("phenomena"), with
  automatic rollback inside waves and incidents outside them.
- Autopilot: group-tag-per-fleet automation, ESP-gated baseline, device
  preparation policies; Apple ADE intent-driven assignment; Knox and
  zero-touch orchestration.

### Phase 3 — Bounded context in Fleet

- Move intents, plans and the ledger into the Fleet server as an `adm`
  bounded context per the modular-monolith conventions (bootstrap, api,
  internal), with its own activities and RBAC actions, keeping the IR and
  the catalog unchanged.
- The reasoning agent as a first-class Fleet feature: proposals from
  vulnerabilities, end-of-life, drift patterns and enrollment.
- ChromeOS through the Fleet extension; Linux profile equivalents for the
  remaining capabilities.

## Open decisions

1. **Where the lakehouse lives.** Tenant-owned (recommended: ADM writes
   Parquet to the tenant's bucket) versus ADM-hosted.
2. **Model hosting.** The Anthropic API by default; Bedrock, Vertex or
   Foundry clients where data residency requires it; the deterministic
   compiler alone for tenants that allow no model.
3. **Per-host declaration targeting in Fleet.** Label per host is a
   workaround; a first-class host-scoped delivery is the right substrate
   change and needs an upstream conversation.
4. **Leash defaults.** Five acceptances per relaxation step and a 30-day
   incident cooldown are starting values; the ledger will say what the
   right ones are per tenant.
5. **The agent's write path on macOS.** A system extension with Endpoint
   Security needs Apple entitlements and a notarised distribution; the
   fallback until then is declarations plus Fleet's MDM channel.
