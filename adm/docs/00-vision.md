# 00 — Vision

## The short version

Device management has been the same shape for fifteen years: an admin
decides on a setting, encodes it as a profile, a policy or a script, assigns
it to a group, and a server pushes it to devices on a timer. The device
reports back on another timer. If something drifts, a human notices in a
dashboard, or does not.

Every generation improved a piece of that loop. Jamf and AirWatch made the
push reliable. Intune added identity and conditional access, and with
Autopilot made the out-of-box experience zero-touch. Fleet replaced opaque
inventory with osquery, made "ask the device a question" take seconds
instead of hours, and made configuration a diffable file in Git. Xavier put
developer package managers, dependency CVEs, removable storage, AI tools and
digital signage under one console with its own MDM server.

None of them changed the shape. The unit of work is still a setting. The
cadence is still a cron. The actor is still a person clicking or writing
YAML. The action is still, more often than not, a script.

ADM changes the shape:

- **The unit of work is an intent.** "Laptops stay encrypted." "No
  removable storage on finance machines." "Chrome is never more than two
  versions behind." "No unapproved AI coding agents on engineering
  workstations." An intent is persistent and cross-platform; ADM compiles it
  to whatever each OS needs.
- **The cadence is the event.** A USB stick is inserted, a process starts,
  a CVE is published, a user changes IdP group, a device enrolls: the intents
  that event makes stale are re-evaluated for that device, then. Sweeps still
  exist, as a safety net.
- **The actor is an agent, on a leash.** A language model proposes plans
  from plain language and from telemetry. A deterministic risk engine decides
  how much autonomy the plan gets: run it, canary it, wait for a person, or
  wait for a change board. The leash loosens as patterns prove out and
  tightens the moment something goes wrong.
- **The action is native.** Windows exposes Configuration Service Providers,
  WMI and Win32; macOS exposes declarative management, Endpoint Security and
  NetworkExtension; Linux exposes D-Bus, udev, netlink and dconf. Calling
  those directly is faster, safer, reversible and observable. PowerShell and
  shell scripts are the fallback of last resort, and every plan that needs
  one says so.

## What "the fleet is secure" becomes

An operator types `I want my fleet to be secure`. ADM:

1. Compiles it to an intent with three predicates: disk encryption enforced
   with escrow, host firewall on, screen lock at ten minutes. Scope: every
   device. Confidence: high, because the phrase is a known baseline.
2. Compiles the intent to a plan: FileVault plus escrow on macOS, BitLocker
   through WMI and the BitLocker CSP on Windows, LUKS verification on Linux;
   the firewall through the `Firewall` CSP, the `com.apple.security.firewall`
   payload, and nftables; screen lock through a DDM declaration, the
   `DeviceLock` CSP, and a dconf lockdown. Each step has an osquery
   verification and a rollback.
3. Scores it: base risk moderate, blast radius the whole fleet, a pattern
   never run in this tenant. Tier: canary. Ten percent of devices first,
   verification, then waves of five hundred.
4. Renders the persistent form: an ADM intent block plus the Fleet GitOps
   projection (`enable_disk_encryption: true`, a policy per verification) as
   a change the operator reviews as a diff.
5. Runs it when the operator says go, verifies every wave on the devices it
   touched, rolls back a wave that fails, and writes every outcome to the
   ledger. Next quarter, the same pattern on a new office's devices runs
   unattended, because it has earned it.

A month later a laptop's firewall is turned off by a local admin. The ADM
agent's OS watcher sees the change in milliseconds, the intent is
re-evaluated for that one host, the tier is auto (one host, a proven
pattern), the firewall is back on before the user notices, and the incident
is a ledger row, a Fleet activity and, if the pattern repeats, a signal.

## What ADM is not

- Not a replacement for the substrate. Fleet's MDM protocols, osquery and
  GitOps stay. ADM drives them, and drives Microsoft Graph for what only
  Intune can do (Autopilot). Where a substrate is the right tool it is used
  as is; ADM does not re-implement APNs.
- Not an autonomous agent with root on your fleet. The model proposes; a
  deterministic engine disposes. Hard invariants (never disable encryption,
  never wipe more than one device per plan, never remove the agent, never
  touch break-glass accounts) cannot be overridden by any score, approval or
  prompt.
- Not a chat bot in front of a console. The console, the CLI, the MCP
  server, chat and GitOps are all fronts for the same intent, plan, risk and
  ledger machinery.

## Design principles

1. **Intent over mechanism.** Operators state outcomes; ADM owns the how.
2. **Native over scripted.** Every capability names the OS interface it
   uses on each platform; scripts are a flagged fallback.
3. **Verify on the device you changed.** The same primitive that observes
   (osquery) proves the change; verification is part of the plan, not a
   dashboard.
4. **Reversible by construction.** Check before apply, capture prior state,
   revert on failure. Irreversible changes are scored as such.
5. **Autonomy is earned, per pattern, per tenant.** Nothing runs unattended
   until it has run attended, and an incident revokes what was earned.
6. **Persistent and reviewable.** Every intent has a GitOps form; every
   plan has a diff; every action has a ledger row.
7. **Human language in, human explanation out.** `explain` narrates what,
   where, how, why this tier, how to undo, and what happened, for every plan.
8. **Open by default.** The catalog, the IR, the risk policy and the ledger
   schema are documented data, not product secrets.
