# 06 — Capability packs

The catalog (`adm/capability`) is the vocabulary of ADM. This document maps
it to what Fleet and Xavier do today, and says what ADM adds. Every row is a
capability with native bindings; "server" means the work happens in the
control plane rather than on the device.

## Parity with Xavier Device Management

Xavier's announcement listed what "most MDMs don't" do. ADM's answer for
each:

| Xavier feature | ADM capability | How it works in ADM |
|---|---|---|
| Manages Homebrew, npm, pip and RubyGems across the fleet | `package.manager.upgrade`, `dependency.cve.scan` | Inventory from osquery's `homebrew_packages`, `npm_packages`, `python_packages` and an ADM `ruby_gems` table; CVE matching through Fleet's OSV processor; upgrades by invoking the package manager directly, per package, as the owning user. "Only vulnerable" is the default. |
| Scans dependencies for known CVEs, flags language runtimes past end-of-life | `dependency.cve.scan`, `runtime.eol.flag` | Read-only capabilities that turn feed matches (OSV, NVD, endoflife.date) into facts and `vuln.published` / `runtime.eol` events; software intents react to the events. |
| Watches upstream releases, verifies checksums, imports into Munki | `software.release.watch`, `software.version.floor` | A control-plane watcher over GitHub releases, Homebrew cask JSON, winget manifests and vendor feeds; signature and checksum verification; a GitOps merge request that updates the Fleet-maintained app (or generates `munkiimport` metadata for a Munki repo). A version floor intent turns the release into an install plan automatically, at the tier the pattern has earned. |
| Audits and blocks removable storage on macOS and Windows | `storage.removable.audit`, `storage.removable.block` | Native: DDM disk-management declaration plus Endpoint Security mount authorisation on macOS; Storage and RemovableStorage CSP nodes on Windows; udev and USBGuard on Linux; AMAPI on Android. Instant events from ES, WMI and udev. |
| Blocks websites and applications, including AI providers | `web.block`, `ai.tools.govern` | Content filter and DNS proxy payloads on Apple; Edge URL blocklist and Defender network protection on Windows; resolved and nftables on Linux; AppLocker, ES exec denial and fapolicyd for applications. |
| Read-only MCP server for AI agents | Fleet's `cmd/fleet-mcp` (read) plus ADM's MCP (write, gated) | Agents read through Fleet's server and act through ADM's, where every action is an intent with a tier and an approval gate the agent cannot bypass. |
| Digital signage on iPads, Apple TVs and Android tablets | `signage.kiosk` | Single-app mode on supervised iPadOS and tvOS, AMAPI kiosk on Android, AssignedAccess on Windows; the signage content service is a playlist URL the kiosk app loads. |
| One console for macOS, iOS, iPadOS, tvOS, Android, Windows | The ADM console over Fleet's inventory | Linux is first-class from day one because fleetd already is; ChromeOS follows Fleet's extension. |
| Enrollment: ABM/ASM, Knox zero-touch, QR, OMA-DM | `enrollment.*` | Document 05. Autopilot is added through Graph. |

## Parity with Fleet

Everything Fleet does remains available, because Fleet is the substrate.
What changes is how it is driven:

| Fleet feature | ADM's use |
|---|---|
| Live query | The perception primitive and the verification primitive. |
| Policies | Every ADM verification is also a Fleet policy, so drift is visible in Fleet and Fleet automations still fire. |
| Profiles and declarations | Delivered by ADM adapters; per-host declaration targeting is on the substrate roadmap. |
| Scripts | `script.run`, flagged, approve-tier floor. |
| Software installers, Fleet-maintained apps, VPP | `software.install`, `software.version.floor`, driven by the release watcher and by intents. |
| Vulnerability processing | `dependency.cve.scan` consumes it; intents react to `vuln.published`. |
| Disk encryption | `disk.encryption.enforce` with Fleet's escrow. |
| Conditional access (Entra, Okta) | `compliance.signal` with a score and reasons instead of a boolean. |
| GitOps | The persistent form of every intent. |
| Setup experience | Where the baseline lands during enrollment. |
| Activities | Every ADM action is also a Fleet activity where the substrate exposes one, and always a ledger row. |
| Fleet MCP | The read plane. |
| `ai_tools`, `mcp_listening_servers` tables | The inventory behind `ai.tools.govern`. |

## Beyond both

| Capability | What is new |
|---|---|
| Intents in plain language, cross-platform | Neither product has an intent layer; both manage settings. |
| Graduated autonomy with an adaptive leash | Neither product acts without a human; Fleet automations are fixed rules. |
| Event-speed reconciliation with a local fast path | Both reconcile on timers (Fleet: 30 s profile reconcilers, 1 h policies and vulnerabilities). |
| Native action layer with check / apply / revert | Both fall back to scripts for anything without a profile. |
| Verification as part of the change | Fleet's profile statuses (pending, verifying, verified, failed) exist for profiles only; ADM verifies every capability with a query. |
| Outcome ledger and learning | Neither product learns from what happened. |
| A write plane for agents with the same gates as humans | Both offer read-only MCP. |
| Autopilot write side | Fleet reads Autopilot identities; ADM registers, tags, assigns and syncs. |
| Risk score and reasons to the IdP | Both send a boolean. |
| Runtime end-of-life and dependency CVEs as events | Xavier reports them; ADM acts on them. |

## The full catalog

Run `admctl capabilities` (or the `adm_list_capabilities` MCP tool) for the
live list. At the time of writing: `disk.encryption.enforce`,
`firewall.enable`, `screen.lock.enforce`, `edr.sensor.ensure`,
`storage.removable.block`, `storage.removable.audit`, `web.block`,
`network.wifi.configure`, `software.install`, `software.version.floor`,
`software.release.watch`, `package.manager.upgrade`, `dependency.cve.scan`,
`runtime.eol.flag`, `ai.tools.govern`, `os.update.enforce`,
`identity.local_admin.manage`, `certificate.issue`, `compliance.signal`,
`signage.kiosk`, `host.lock`, `host.wipe`, `enrollment.autopilot.register`,
`enrollment.ade.assign`, `enrollment.android.provision`, `telemetry.query`,
`script.run`.

Adding a capability is adding a catalog entry (name, params, per-platform
binding, verification, base risk) and an adapter per platform. The compilers
pick it up from the catalog; nothing else changes.
