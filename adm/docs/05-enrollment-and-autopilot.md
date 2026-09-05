# 05 — Enrollment and Autopilot

## Zero-touch on every platform

The intent for onboarding is the same everywhere: *a device that belongs to
us enrolls into ADM before a person sees the desktop, and lands with its
baseline applied.* The mechanism differs per platform, and ADM drives each
one through the substrate that owns it.

| Platform | Program | What ADM does | Substrate |
|---|---|---|---|
| macOS, iOS, iPadOS, tvOS | Apple Business Manager / Apple School Manager (Automated Device Enrollment) | Assign the enrollment profile to the serial in the right fleet; supervise; run the setup experience (IdP auth, software, scripts); apply the fleet's intents at first check-in. | Fleet's DEP sync (`apple_mdm_dep_profile_assigner`, every minute) and setup experience. |
| Windows | Windows Autopilot (Entra ID automatic enrollment) | Register the hardware hash, tag the device, assign the deployment profile, hold the Enrollment Status Page until the baseline is applied. | Microsoft Graph (write side, `adm/enroll/autopilot`) plus Fleet's Entra automatic-enrollment endpoints and Windows setup experience. |
| Android | Google zero-touch, Samsung Knox Mobile Enrollment, QR provisioning | Create the enrollment token; point the zero-touch or KME configuration at it; fully managed or work profile. | Fleet's AMAPI integration (`enterprises.enrollmentTokens.create`). |
| Linux | Autoinstall (Ubuntu), kickstart (RHEL family), cloud-init | Ship fleetd and the ADM extension in the image with LUKS at install; enroll on first boot; apply the baseline. | fleetd; the ADM agent. |
| ChromeOS | Google Admin zero-touch (future) | Enroll the Fleet Chrome extension. | Fleet's ChromeOS extension. |

Onboarding is itself an intent (`enrollment.autopilot.register`,
`enrollment.ade.assign`, `enrollment.android.provision` are catalog
capabilities), so "new engineering laptops get the engineering baseline"
is one line in GitOps and one `host.enrolled` event away from happening.

## Windows Autopilot in depth

### What exists today

Fleet supports Windows automatic enrollment through Entra ID: Fleet hosts the
MDM discovery and terms-of-use endpoints, validates the Entra-issued token
(audience host and tenant IDs from `windows_entra_tenant_ids` and
`windows_entra_client_ids`), issues the WSTEP certificate, and, for Autopilot
and Entra-join-during-OOBE enrollments, holds the device at the Enrollment
Status Page while the Windows setup experience (IdP authentication, software
installs) runs. Fleet's `server/microsoft/msgraph` reads Autopilot device
identities (`/deviceManagement/windowsAutopilotDeviceIdentities`) on a cron
and maps them to hosts. The Autopilot deployment profile itself is created
in Intune, which requires Entra ID P1 plus an Intune or "alternative MDM"
subscription for the end users.

This is the part of the Intune stack that the design brief correctly says
Intune still wins: the OOBE, the ESP and the identity join are Microsoft's,
and no third-party MDM replaces them. ADM does not try. It drives them.

### What ADM adds

```mermaid
sequenceDiagram
    participant OEM as OEM / IT (hardware hash)
    participant ADM
    participant G as Microsoft Graph
    participant E as Entra ID / Intune
    participant D as Device (OOBE)
    participant F as Fleet (MDM)

    OEM->>ADM: serial + OA3 hash (CSV, OEM feed, or Get-WindowsAutopilotInfo)
    ADM->>G: POST importedWindowsAutopilotDeviceIdentities (groupTag by fleet)
    ADM->>G: poll until deviceImportStatus = complete
    ADM->>G: updateDeviceProperties (groupTag) ; profile assignment to the tag's dynamic group
    ADM->>G: POST windowsAutopilotSettings/sync
    D->>E: OOBE: Autopilot profile lookup by hash
    E-->>D: profile (user type standard, skip pages, hide EULA)
    D->>E: Entra join + automatic MDM enrollment (discovery URL = Fleet)
    D->>F: MDM enroll (WSTEP), ESP hold
    F->>ADM: host.enrolled event
    ADM->>F: setup experience: IdP auth, baseline software, ADM agent
    ADM->>D: intents for the fleet (CSP steps, native), verified
    ADM->>F: release ESP
    D-->>D: desktop, compliant from the first second
```

Concretely, `adm/enroll/autopilot` implements the write side against Graph
with an Entra app registration (client credentials,
`DeviceManagementServiceConfig.ReadWrite.All`):

- `ImportDevice` posts an `importedWindowsAutopilotDeviceIdentity` with the
  serial, the OA3 hardware hash and a group tag; `WaitForImport` polls the
  asynchronous import until it completes or errors.
- `SetGroupTag` re-tags a registered device; deployment profiles are
  targeted at dynamic Entra groups keyed on the tag, so the tag is how a
  device is routed to a fleet.
- `ListProfiles` and `AssignProfileToGroup` manage deployment profile
  assignments.
- `Sync` forces the Autopilot service sync instead of waiting for its
  schedule.
- `ListDevices` and `DeleteDevice` complete the lifecycle (Fleet's read-side
  cron continues to map identities to hosts).

The design choices:

1. **Group tags are fleets.** ADM derives the tag from the target fleet
   (`adm-engineering`), creates the dynamic group and the profile assignment
   once per fleet, and every subsequent device is one import away from the
   right baseline.
2. **ESP is the gate for the baseline.** The Windows setup experience Fleet
   already holds at the ESP is where ADM applies the fleet's intents. A
   device leaves OOBE compliant, not "will be compliant after the next 30
   second sync".
3. **Autopilot v2 / device preparation** (the newer Intune flow with
   just-in-time configuration) is supported the same way: ADM manages the
   preparation policy through Graph and applies intents in the same window.
4. **Hybrid join is out of scope.** Entra-only join; hybrid Autopilot is a
   Microsoft-specific dead end and ADM does not extend it.
5. **Conditional access uses ADM's signal.** Fleet's Entra integration
   sends a compliance boolean; ADM's `compliance.signal` capability publishes
   the risk score and reason codes as well, through Fleet's proxy, so a
   device that drifts loses access for a stated reason.

### Licensing, stated plainly

Autopilot needs Entra ID P1 (or P2) for the users and an Intune or
alternative-MDM subscription; Fleet counts as the alternative MDM. ADM does
not change this. It removes the Intune console from the daily loop, not the
Intune licence from the invoice.

## Apple ADE

ADM keeps Fleet's flow and adds intent-driven assignment: an
`enrollment.ade.assign` predicate with a serial (or a serial pattern from a
purchase order) selects the fleet, and the DEP profile assignment happens on
the next sync. During setup, Fleet's macOS setup experience (bootstrap
package, IdP auth, software, script) runs; ADM's contribution is that the
fleet's intents are applied and verified as part of it, and that the device
carries the ADM extension so its watchers are live before the user is.

## Android

Fully managed devices come through zero-touch (Google) or KME (Samsung)
pointed at an ADM enrollment token; BYOD gets a work profile through a QR or
a link. `enrollment.android.provision` chooses the method and whether the
device is fully managed. Kiosk and signage devices are enrolled fully
managed with `signage.kiosk` in the fleet's intents.

## Linux

The image carries fleetd, the ADM extension and a LUKS-encrypted root, and
enrolls on first boot with a fleet-scoped enroll secret. There is no OOBE
to hold, so the baseline is applied at first check-in, and the setup
experience (IdP auth, software) runs in the browser as Fleet's Linux setup
experience does today.
