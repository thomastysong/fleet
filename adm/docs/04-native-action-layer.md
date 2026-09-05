# 04 — Native action layer

## Why not scripts

Fleet, Intune and Xavier all reach for a script when a setting has no
profile: `/bin/sh` and `powershell -MTA -ExecutionPolicy Bypass -File` in
fleetd's runner, with a five-minute cap and a five-thousand-host batch limit.
Scripts are the universal solvent and the worst mechanism on every axis
ADM cares about:

| Axis | Script | Native call |
|---|---|---|
| Latency | Seconds to minutes: download, spawn interpreter, run. | Milliseconds: one API call in a resident process. |
| Observability | Exit code and captured stdout. | Typed return values, and the OS's own event channel confirms the change. |
| Reversibility | Whatever the author remembered to write. | `Check` captures prior state; `Revert` restores it. |
| Idempotence | Whatever the author remembered to write. | `Check` before `Apply`, by contract. |
| Blast radius | Anything the interpreter can do, as SYSTEM or root. | Exactly the interface called, with its own access checks. |
| Security review | Read the script. | Read the adapter once; it is code with tests. |

ADM keeps `script.run` in the catalog as a capability with a script
mechanism, a high base risk, an approve-tier floor and a "no fleet-wide
scripts" invariant, so it is always available and always visible. Every
plan that needs it is scored for it.

## The adapter contract

```go
type Adapter interface {
    ID() string
    Capability() string
    Platform() intent.Platform
    Check(ctx, Target, params) (State, error)   // observe; State carries "satisfied"
    Apply(ctx, Target, params) (Result, error)  // converge
    Revert(ctx, Target, params, prior State) (Result, error)
}
```

The executor never calls `Apply` without `Check`, so a device already in the
desired state is skipped and counted, and `Revert` always has the prior
state it needs. `Result.Native` records the exact call that was made, so the
ledger and `explain` can show it.

## Windows

The Windows adapters (`adm/native/windows`) speak the language the OS
already uses for management: SyncML against Configuration Service Providers,
applied through the local MDM management stack (`mdmlocalmanagement.dll`,
`ApplyLocalManagementSyncML`). fleetd already loads this DLL for its
`mdm_bridge` osquery table; ADM composes the same `<SyncBody>` documents,
in pure Go, and parses the same responses. A `Replace` that answers 404 is
retried as `Add`, which is how CSP nodes are created.

| Capability | Native interface | Instant event |
|---|---|---|
| `storage.removable.block` | `Policy/Config/Storage/RemovableDiskDenyWriteAccess`; `ADMX_RemovableStorage/RemovableDisks_DenyRead_Access_2` and `_DenyExecute_Access_2` | WMI `Win32_VolumeChangeEvent` |
| `firewall.enable` | `./Vendor/MSFT/Firewall/MdmStore/{Domain,Private,Public}Profile/EnableFirewall` | ETW `Microsoft-Windows-Windows Firewall With Advanced Security` |
| `screen.lock.enforce` | `Policy/Config/DeviceLock/MaxInactivityTimeDeviceLock`, `DevicePasswordEnabled` | registry change notification on `PolicyManager\current` |
| `web.block` | ADMX-ingested `microsoft_edge~Policy~microsoft_edge/URLBlocklist`; Defender Network Protection indicators via `./Vendor/MSFT/Defender` | — |
| `ai.tools.govern` | `./Vendor/MSFT/AppLocker/ApplicationLaunchRestrictions/{group}/EXE/Policy` plus `web.block` | ETW `Microsoft-Windows-AppLocker` |
| `disk.encryption.enforce` | WMI `Win32_EncryptableVolume.Encrypt` (as fleetd's BitLocker code does today) plus `BitLocker/RequireDeviceEncryption` | WMI `__InstanceModificationEvent` |
| `os.update.enforce` | Windows Update Agent COM (`IUpdateSession`) for search/download/install; `Policy/Config/Update/ConfigureDeadlineForQualityUpdates` | — |
| `host.lock` / `host.wipe` | `./Vendor/MSFT/RemoteLock/Lock`, `./Device/Vendor/MSFT/RemoteWipe/doWipe` (Exec) | — |
| `identity.local_admin.manage` | `./Device/Vendor/MSFT/LAPS/Policies`, `Accounts/Users`; `NetUserAdd` for immediate creation | — |
| `network.wifi.configure` | `./Vendor/MSFT/WiFi/Profile/{SSID}/WlanXml` | — |
| `signage.kiosk` | `./Device/Vendor/MSFT/AssignedAccess/Configuration` | — |
| `software.install` | Windows Installer COM; winget WinRT `Microsoft.Management.Deployment.PackageManager` | — |

Two delivery paths exist for a CSP write, and the executor uses the fastest
one available: the ADM agent extension applies it locally in milliseconds;
without the extension, the same SyncML is enqueued as an MDM command through
Fleet and kicked with the `windows_mdm_sync_request` notification (seconds
to a minute). Reads always go through `mdm_bridge` live queries today.

## macOS, iOS, iPadOS, tvOS

Apple's declarative device management is the native mechanism ADM prefers,
because the device evaluates declarations itself, re-applies them without a
server round trip and reports status asynchronously: exactly the device-side
autonomy an agentic control plane wants. Fleet supports configuration
declarations today (not status subscriptions, assets or activation
predicates; ADM's roadmap adds those to the substrate).

| Capability | Native interface | Instant event / enforcement |
|---|---|---|
| `storage.removable.block` | `com.apple.configuration.diskmanagement.settings` (`ExternalStorage: Disallowed / ReadOnly`, macOS 15+) | Endpoint Security `ES_EVENT_TYPE_AUTH_MOUNT` deny in the ADM system extension |
| `screen.lock.enforce` | `com.apple.configuration.screensaver.settings`, `com.apple.configuration.passcode.settings` | — |
| `os.update.enforce` | `com.apple.configuration.softwareupdate.enforcement.specific` | — |
| `web.block` | `com.apple.webcontent-filter` (built-in deny list) or an `NEFilterDataProvider` in the ADM extension; `com.apple.dnsProxy.managed` | filter decisions are the event |
| `ai.tools.govern` | Endpoint Security `ES_EVENT_TYPE_AUTH_EXEC` deny by signing ID or path; `web.block` for providers | `NOTIFY_EXEC` |
| `disk.encryption.enforce` | `com.apple.MCX.FileVault2` plus `FDERecoveryKeyEscrow`; `fdesetup` invoked directly | osquery `disk_encryption` |
| `firewall.enable` | `com.apple.security.firewall`; `socketfilterfw` invoked directly | — |
| `identity.local_admin.manage` | `sysadminctl`; MDM `AccountConfiguration` at ADE | — |
| `certificate.issue` | `com.apple.security.acme` (hardware attestation), SCEP | — |
| `signage.kiosk` (iPadOS, tvOS) | `com.apple.app.lock` single-app mode on supervised devices | — |
| `host.lock` / `host.wipe` | `DeviceLock`, `EraseDevice` MDM commands | — |

Apple-provided binaries are invoked as argument vectors from the agent,
never through a shell, and only where no framework or declaration exists.

## Linux

Linux has no MDM protocol, which is why every vendor scripts it. It does
have a rich set of local management interfaces, and ADM uses them:

| Capability | Native interface | Instant event |
|---|---|---|
| `storage.removable.block` | udev rule (`ATTR{authorized}="0"` for USB mass storage; `UDISKS_IGNORE`), USBGuard `org.usbguard.Policy1.appendRule` | udev netlink block add/remove; `/sys/block/*/removable` |
| `screen.lock.enforce` | dconf lockdown files under `/etc/dconf/db/local.d` with locks; `dconf update` | inotify on the lockdown directory |
| `firewall.enable` | nftables over netlink (`nf_tables`), or firewalld `org.fedoraproject.FirewallD1` | netlink `NFNLGRP_NFTABLES` |
| `host.lock` | `org.freedesktop.login1.Manager.LockSessions`; PAM lockout | — |
| `edr.sensor.ensure`, services | `org.freedesktop.systemd1.Manager.StartUnit` and unit state signals | D-Bus `PropertiesChanged` |
| `software.install`, `os.update.enforce` | `org.freedesktop.PackageKit.Transaction` | PackageKit signals |
| `network.wifi.configure` | `org.freedesktop.NetworkManager.Settings.AddConnection` | NetworkManager signals |
| `identity.local_admin.manage` | `org.freedesktop.Accounts.CreateUser` | — |
| `disk.encryption.enforce` | LUKS at provisioning (autoinstall / kickstart); escrow via fleetd's existing LUKS code | — |
| `web.block` | `org.freedesktop.resolve1` per-link DNS policy; nftables sets | — |
| `ai.tools.govern` | fapolicyd rules or AppArmor deny profiles; `web.block` | fanotify |

The scaffold's Linux adapters take a `Runner` (argument vector execution, no
interpreter) and a `DBus` interface, so they are pure and testable; the agent
provides the real system-bus and privileged-exec implementations.

## Android

On Android the native interface is the Android Management API policy
itself: `mountPhysicalMediaDisabled`, `usbFileTransferDisabled`,
`kioskCustomization`, `applications[].installType`, `passwordPolicies`,
`systemUpdate`, `openNetworkConfiguration`, and the `LOCK` / wipe commands.
Fleet already speaks AMAPI and receives Pub/Sub push; ADM's adapters map
capability parameters to policy fields and rely on Fleet to patch the policy.

## The `mdm_bridge` lesson

fleetd's `mdm_bridge` table is the existence proof for the whole layer: a
Windows management call (`ApplyLocalManagementSyncML`) exposed through the
same osquery plumbing that reads inventory, callable from the server with a
live query, answering in seconds, no PowerShell in sight. ADM generalises
it: every adapter is that idea, for every capability, on every platform,
with `Check`, `Apply`, `Revert` and a verification query.
