package capability

import "github.com/fleetdm/fleet/v4/adm/intent"

const (
	darwin  = intent.PlatformDarwin
	windows = intent.PlatformWindows
	linux   = intent.PlatformLinux
	ios     = intent.PlatformIOS
	ipados  = intent.PlatformIPadOS
	tvos    = intent.PlatformTVOS
	android = intent.PlatformAndroid
)

// Default returns the built-in ADM capability catalog. Every binding names a
// native interface; MechScript appears only where no native path exists yet
// and is flagged as such so the risk engine and the roadmap can see it.
func Default() *Catalog {
	return NewCatalog(
		// ----------------------------------------------------------------- security
		Capability{
			Name: "disk.encryption.enforce", Domain: DomainSecurity,
			Summary:    "Full-disk encryption on with the recovery key escrowed to ADM.",
			Params:     []Param{{Name: "escrow", Type: "bool", Description: "Escrow the recovery key", Default: true}},
			Reversible: false, UserImpact: intent.ImpactPrompt, Privilege: PrivilegeMDM, BaseRisk: 12,
			Bindings: map[intent.Platform]Binding{
				darwin:  {Mechanism: MechMDMProfile, Native: "com.apple.MCX.FileVault2 + com.apple.security.FDERecoveryKeyEscrow", Adapter: "darwin.filevault", Latency: LatencyMinutes, Event: "osquery disk_encryption", Notes: "Immediate action through the Apple-provided fdesetup binary when a user session exists."},
				windows: {Mechanism: MechWMI, Native: "root\\CIMV2\\Security\\MicrosoftVolumeEncryption Win32_EncryptableVolume.Encrypt + BitLocker CSP ./Device/Vendor/MSFT/BitLocker/RequireDeviceEncryption", Adapter: "windows.bitlocker", Latency: LatencySeconds, Event: "WMI __InstanceModificationEvent Win32_EncryptableVolume"},
				linux:   {Mechanism: MechDBus, Native: "LUKS2 via org.freedesktop.UDisks2.Encrypted + provisioning-time autoinstall/kickstart", Adapter: "linux.luks", Latency: LatencyMinutes, Notes: "Enforced at provisioning; post-install encryption of a live root volume is not attempted."},
				android: {Mechanism: MechAMAPI, Native: "policy.encryptionPolicy=ENABLED_WITH_PASSWORD", Latency: LatencyMinutes},
				ios:     {Mechanism: MechMDMProfile, Native: "com.apple.mobiledevice.passwordpolicy (data protection follows passcode)", Latency: LatencyMinutes},
			},
			Verify: map[intent.Platform]Verify{
				darwin:  {Query: "SELECT 1 FROM disk_encryption WHERE encrypted = 1 AND name = '/'", Expect: "rows>0"},
				windows: {Query: "SELECT 1 FROM bitlocker_info WHERE drive_letter = 'C:' AND protection_status = 1", Expect: "rows>0"},
				linux:   {Query: "SELECT 1 FROM disk_encryption de JOIN mounts m ON de.name = m.device WHERE m.path = '/' AND de.encrypted = 1", Expect: "rows>0"},
			},
			Parity: []string{"fleet", "intune", "xavier"},
		},
		Capability{
			Name: "firewall.enable", Domain: DomainSecurity,
			Summary:    "Host firewall on for every network profile.",
			Reversible: true, UserImpact: intent.ImpactNone, Privilege: PrivilegeSystem, BaseRisk: 6,
			Bindings: map[intent.Platform]Binding{
				darwin:  {Mechanism: MechMDMProfile, Native: "com.apple.security.firewall (EnableFirewall, BlockAllIncoming)", Adapter: "darwin.firewall", Latency: LatencySeconds, Notes: "Immediate through /usr/libexec/ApplicationFirewall/socketfilterfw."},
				windows: {Mechanism: MechCSP, Native: "./Vendor/MSFT/Firewall/MdmStore/{Domain,Private,Public}Profile/EnableFirewall", Adapter: "windows.csp", Latency: LatencyMillis, Event: "ETW Microsoft-Windows-Windows Firewall With Advanced Security"},
				linux:   {Mechanism: MechNetlink, Native: "nftables ruleset via netlink (nf_tables) or firewalld org.fedoraproject.FirewallD1", Adapter: "linux.nftables", Latency: LatencyMillis, Event: "netlink NFNLGRP_NFTABLES"},
			},
			Verify: map[intent.Platform]Verify{
				darwin:  {Query: "SELECT 1 FROM alf WHERE global_state >= 1", Expect: "rows>0"},
				windows: {Query: "SELECT 1 FROM windows_security_center WHERE firewall = 'Good'", Expect: "rows>0"},
				linux:   {Query: "SELECT 1 FROM iptables WHERE chain = 'INPUT' AND policy = 'DROP' LIMIT 1", Expect: "rows>0"},
			},
			Parity: []string{"fleet", "intune", "xavier"},
		},
		Capability{
			Name: "screen.lock.enforce", Domain: DomainSecurity,
			Summary:    "Screen locks after an idle timeout and requires a password to unlock.",
			Params:     []Param{{Name: "idle_minutes", Type: "int", Required: true, Description: "Idle minutes before lock"}},
			Reversible: true, UserImpact: intent.ImpactNone, Privilege: PrivilegeMDM, BaseRisk: 4,
			Bindings: map[intent.Platform]Binding{
				darwin:  {Mechanism: MechDDM, Native: "com.apple.configuration.screensaver.settings (LoginWindowIdleTime) + com.apple.configuration.passcode.settings", Adapter: "darwin.ddm", Latency: LatencySeconds},
				windows: {Mechanism: MechCSP, Native: "./Device/Vendor/MSFT/Policy/Config/DeviceLock/MaxInactivityTimeDeviceLock + DevicePasswordEnabled", Adapter: "windows.csp", Latency: LatencyMillis},
				linux:   {Mechanism: MechDconf, Native: "/etc/dconf/db/local.d lockdown: org.gnome.desktop.session idle-delay, org.gnome.desktop.screensaver lock-enabled", Adapter: "linux.dconf", Latency: LatencySeconds},
				ios:     {Mechanism: MechDDM, Native: "com.apple.configuration.passcode.settings (MaximumInactivityInMinutes)", Latency: LatencyMinutes},
				android: {Mechanism: MechAMAPI, Native: "policy.passwordPolicies[].maximumTimeToLock", Latency: LatencyMinutes},
			},
			Verify: map[intent.Platform]Verify{
				darwin:  {Query: "SELECT 1 FROM screenlock WHERE enabled = 1", Expect: "rows>0"},
				windows: {Query: "SELECT 1 FROM registry WHERE path = 'HKEY_LOCAL_MACHINE\\SOFTWARE\\Microsoft\\PolicyManager\\current\\device\\DeviceLock\\MaxInactivityTimeDeviceLock'", Expect: "rows>0"},
				linux:   {Query: "SELECT 1 FROM dconf_read WHERE key = '/org/gnome/desktop/screensaver/lock-enabled' AND value = 'true'", Expect: "rows>0"},
			},
			Parity: []string{"fleet", "intune", "xavier"},
		},
		Capability{
			Name: "edr.sensor.ensure", Domain: DomainSecurity,
			Summary:    "A named endpoint security sensor is installed and running.",
			Params:     []Param{{Name: "sensor", Type: "string", Required: true, Description: "santa, crowdstrike, defender, ..."}},
			Reversible: true, UserImpact: intent.ImpactNone, Privilege: PrivilegeSystem, BaseRisk: 8,
			Bindings: map[intent.Platform]Binding{
				darwin:  {Mechanism: MechServer, Native: "software.install via Fleet-maintained app or custom package; ES client health from santa/crowdstrike osquery tables", Latency: LatencyMinutes},
				windows: {Mechanism: MechServer, Native: "software.install (MSI) + WMI Win32_Service state; Defender via ./Vendor/MSFT/Defender", Latency: LatencyMinutes},
				linux:   {Mechanism: MechDBus, Native: "org.freedesktop.systemd1.Manager.StartUnit for the sensor unit", Adapter: "linux.systemd", Latency: LatencySeconds},
			},
			Verify: map[intent.Platform]Verify{
				darwin:  {Query: "SELECT 1 FROM processes WHERE name IN ('santad','falcond','com.crowdstrike.falcon.Agent')", Expect: "rows>0"},
				windows: {Query: "SELECT 1 FROM services WHERE name IN ('CSFalconService','WinDefend','Sense') AND status = 'RUNNING'", Expect: "rows>0"},
				linux:   {Query: "SELECT 1 FROM processes WHERE name IN ('falcon-sensor','falcond')", Expect: "rows>0"},
			},
			Parity: []string{"fleet", "intune"},
		},
		// ------------------------------------------------------------------ storage
		Capability{
			Name: "storage.removable.block", Domain: DomainStorage,
			Summary:    "Removable storage cannot be mounted, or is read-only.",
			Params:     []Param{{Name: "mode", Type: "string", Required: true, Description: "block or read_only"}},
			Reversible: true, UserImpact: intent.ImpactNone, Privilege: PrivilegeSystem, BaseRisk: 10,
			Bindings: map[intent.Platform]Binding{
				darwin:  {Mechanism: MechDDM, Native: "com.apple.configuration.diskmanagement.settings Restrictions.ExternalStorage=Disallowed|ReadOnly (macOS 15+) + Endpoint Security ES_EVENT_TYPE_AUTH_MOUNT deny", Adapter: "darwin.ddm", Latency: LatencyMillis, Event: "ES_EVENT_TYPE_NOTIFY_MOUNT"},
				windows: {Mechanism: MechCSP, Native: "./Device/Vendor/MSFT/Policy/Config/Storage/RemovableDiskDenyWriteAccess (read_only) or ADMX_RemovableStorage/RemovableDisks_DenyRead_Access_2 (block)", Adapter: "windows.csp", Latency: LatencyMillis, Event: "WMI Win32_VolumeChangeEvent"},
				linux:   {Mechanism: MechUdev, Native: "udev rule ENV{ID_BUS}==\"usb\" SUBSYSTEM==\"block\" ENV{UDISKS_IGNORE}=\"1\" + org.usbguard.Policy1.appendRule", Adapter: "linux.udev", Latency: LatencyMillis, Event: "udev netlink add/remove block"},
				android: {Mechanism: MechAMAPI, Native: "policy.mountPhysicalMediaDisabled=true, policy.usbFileTransferDisabled=true", Latency: LatencyMinutes},
				ios:     {Mechanism: MechMDMProfile, Native: "com.apple.applicationaccess allowFilesUSBDriveAccess=false", Latency: LatencyMinutes},
			},
			Verify: map[intent.Platform]Verify{
				darwin:  {Query: "SELECT 1 FROM mounts WHERE path LIKE '/Volumes/%' AND type NOT IN ('apfs','hfs') AND device LIKE '/dev/disk%'", Expect: "rows==0"},
				windows: {Query: "SELECT 1 FROM registry WHERE path LIKE 'HKEY_LOCAL_MACHINE\\SOFTWARE\\Microsoft\\PolicyManager\\current\\device\\Storage\\RemovableDiskDenyWriteAccess' AND data = '1'", Expect: "rows>0"},
				linux:   {Query: "SELECT 1 FROM mounts m JOIN block_devices b ON m.device = b.name WHERE b.parent LIKE '/dev/sd%' AND m.path LIKE '/media/%'", Expect: "rows==0"},
			},
			Parity: []string{"xavier", "intune"},
		},
		Capability{
			Name: "storage.removable.audit", Domain: DomainStorage, ReadOnly: true,
			Summary:    "Inventory removable storage attach/detach events in real time.",
			Reversible: true, UserImpact: intent.ImpactNone, Privilege: PrivilegeSystem, BaseRisk: 0,
			Bindings: map[intent.Platform]Binding{
				darwin:  {Mechanism: MechEndpointSec, Native: "ES_EVENT_TYPE_NOTIFY_MOUNT / NOTIFY_UNMOUNT + osquery disk_events", Adapter: "darwin.es", Latency: LatencyMillis, Event: "ES_EVENT_TYPE_NOTIFY_MOUNT"},
				windows: {Mechanism: MechWMI, Native: "WMI event subscription Win32_VolumeChangeEvent + osquery usb_devices", Adapter: "windows.wmi", Latency: LatencyMillis, Event: "Win32_VolumeChangeEvent"},
				linux:   {Mechanism: MechUdev, Native: "udev netlink block add/remove + /sys/block/*/removable", Adapter: "linux.sysfs", Latency: LatencyMillis, Event: "udev netlink"},
			},
			Parity: []string{"xavier"},
		},
		// ------------------------------------------------------------------ network
		Capability{
			Name: "web.block", Domain: DomainNetwork,
			Summary:    "Named domains or URL patterns are unreachable from browsers and, where supported, from every process.",
			Params:     []Param{{Name: "domains", Type: "[]string", Required: true, Description: "Domains or URL patterns to block"}, {Name: "scope", Type: "string", Description: "browser or system", Default: "system"}},
			Reversible: true, UserImpact: intent.ImpactNone, Privilege: PrivilegeSystem, BaseRisk: 8,
			Bindings: map[intent.Platform]Binding{
				darwin:  {Mechanism: MechNetExt, Native: "com.apple.webcontent-filter (built-in DenyListURLs) or NEFilterDataProvider system extension; DNS via com.apple.dnsProxy.managed", Adapter: "darwin.contentfilter", Latency: LatencySeconds},
				windows: {Mechanism: MechCSP, Native: "./Device/Vendor/MSFT/Policy/Config/microsoft_edge~Policy~microsoft_edge/URLBlocklist (ADMX-ingested) + Defender Network Protection custom indicators via ./Vendor/MSFT/Defender", Adapter: "windows.csp", Latency: LatencyMillis},
				linux:   {Mechanism: MechDBus, Native: "org.freedesktop.resolve1 per-link DNS policy + nftables set of resolved addresses", Adapter: "linux.resolved", Latency: LatencyMillis},
				android: {Mechanism: MechAMAPI, Native: "policy.applications[chrome].managedConfiguration URLBlocklist", Latency: LatencyMinutes},
				ios:     {Mechanism: MechMDMProfile, Native: "com.apple.webcontent-filter (supervised)", Latency: LatencyMinutes},
			},
			Verify: map[intent.Platform]Verify{
				darwin:  {Query: "SELECT 1 FROM managed_policies WHERE domain = 'com.apple.webcontent-filter'", Expect: "rows>0"},
				windows: {Query: "SELECT 1 FROM registry WHERE path LIKE 'HKEY_LOCAL_MACHINE\\SOFTWARE\\Policies\\Microsoft\\Edge\\URLBlocklist\\%'", Expect: "rows>0"},
				linux:   {Query: "SELECT 1 FROM dns_resolvers WHERE type = 'nameserver' AND address = '127.0.0.53'", Expect: "rows>0"},
			},
			Parity: []string{"xavier", "intune"},
		},
		Capability{
			Name: "network.wifi.configure", Domain: DomainNetwork,
			Summary:    "A corporate Wi-Fi network is configured with its credentials or certificate.",
			Params:     []Param{{Name: "ssid", Type: "string", Required: true, Description: "Network SSID"}, {Name: "security", Type: "string", Description: "wpa2-enterprise, wpa2-psk", Default: "wpa2-enterprise"}},
			Reversible: true, UserImpact: intent.ImpactNone, Privilege: PrivilegeMDM, BaseRisk: 6,
			Bindings: map[intent.Platform]Binding{
				darwin:  {Mechanism: MechMDMProfile, Native: "com.apple.wifi.managed", Latency: LatencySeconds},
				windows: {Mechanism: MechCSP, Native: "./Device/Vendor/MSFT/WiFi/Profile/{SSID}/WlanXml", Adapter: "windows.csp", Latency: LatencyMillis},
				linux:   {Mechanism: MechDBus, Native: "org.freedesktop.NetworkManager.Settings.AddConnection", Adapter: "linux.networkmanager", Latency: LatencyMillis},
				ios:     {Mechanism: MechMDMProfile, Native: "com.apple.wifi.managed", Latency: LatencySeconds},
				android: {Mechanism: MechAMAPI, Native: "policy.openNetworkConfiguration", Latency: LatencyMinutes},
			},
			Verify: map[intent.Platform]Verify{
				darwin:  {Query: "SELECT 1 FROM wifi_networks WHERE ssid = '{{.ssid}}'", Expect: "rows>0"},
				windows: {Query: "SELECT 1 FROM wifi_networks WHERE ssid = '{{.ssid}}'", Expect: "rows>0"},
			},
			Parity: []string{"fleet", "intune", "xavier"},
		},
		// ------------------------------------------------------------------- software
		Capability{
			Name: "software.install", Domain: DomainSoftware,
			Summary:    "A software title is installed at or above a version.",
			Params:     []Param{{Name: "title", Type: "string", Required: true, Description: "Software title (Fleet-maintained app slug, VPP app or package)"}, {Name: "min_version", Type: "string", Description: "Minimum acceptable version"}},
			Reversible: true, UserImpact: intent.ImpactNone, Privilege: PrivilegeSystem, BaseRisk: 8,
			Bindings: map[intent.Platform]Binding{
				darwin:  {Mechanism: MechServer, Native: "Fleet software installer / Fleet-maintained app (Homebrew cask metadata) / VPP; device side uses /usr/sbin/installer directly", Latency: LatencyMinutes},
				windows: {Mechanism: MechWin32, Native: "Windows Installer COM (WindowsInstaller.Installer.InstallProduct) / winget WinRT Microsoft.Management.Deployment.PackageManager", Adapter: "windows.installer", Latency: LatencyMinutes},
				linux:   {Mechanism: MechDBus, Native: "org.freedesktop.PackageKit.Transaction.InstallFiles / InstallPackages", Adapter: "linux.packagekit", Latency: LatencyMinutes},
				ios:     {Mechanism: MechMDMCommand, Native: "InstallApplication (VPP)", Latency: LatencyMinutes},
				android: {Mechanism: MechAMAPI, Native: "policy.applications[].installType=FORCE_INSTALLED", Latency: LatencyMinutes},
			},
			Verify: map[intent.Platform]Verify{
				darwin:  {Query: "SELECT 1 FROM apps WHERE name = '{{.title}}'", Expect: "rows>0"},
				windows: {Query: "SELECT 1 FROM programs WHERE name LIKE '{{.title}}%'", Expect: "rows>0"},
				linux:   {Query: "SELECT 1 FROM deb_packages WHERE name = '{{.title}}' UNION SELECT 1 FROM rpm_packages WHERE name = '{{.title}}'", Expect: "rows>0"},
			},
			Parity: []string{"fleet", "intune", "xavier"},
		},
		Capability{
			Name: "software.version.floor", Domain: DomainSoftware,
			Summary:    "A software title is never more than N releases behind upstream; upgrades are proposed automatically as releases ship.",
			Params:     []Param{{Name: "title", Type: "string", Required: true, Description: "Software title"}, {Name: "max_versions_behind", Type: "int", Description: "How many releases behind is tolerated", Default: 2}},
			Reversible: true, UserImpact: intent.ImpactPrompt, Privilege: PrivilegeSystem, BaseRisk: 10,
			Bindings: map[intent.Platform]Binding{
				darwin:  {Mechanism: MechServer, Native: "release watcher -> Fleet-maintained app update -> software.install", Latency: LatencyMinutes},
				windows: {Mechanism: MechServer, Native: "release watcher (winget manifests) -> software.install", Latency: LatencyMinutes},
				linux:   {Mechanism: MechServer, Native: "release watcher (distro repo metadata) -> software.install", Latency: LatencyMinutes},
			},
			Parity: []string{"xavier", "fleet"},
		},
		Capability{
			Name: "software.release.watch", Domain: DomainSoftware, ReadOnly: true,
			Summary:    "Watch upstream releases for a title, verify checksums and signatures, and open a GitOps change (Fleet-maintained app or Munki import) when a new release ships.",
			Params:     []Param{{Name: "title", Type: "string", Required: true, Description: "Software title"}, {Name: "source", Type: "string", Description: "github, homebrew, winget, vendor-feed", Default: "homebrew"}, {Name: "munki_repo", Type: "string", Description: "Optional Munki repo to import into"}},
			Reversible: true, UserImpact: intent.ImpactNone, Privilege: PrivilegeSystem, BaseRisk: 2,
			Bindings: map[intent.Platform]Binding{
				darwin:  {Mechanism: MechServer, Native: "control-plane watcher; ee/maintained-apps ingesters; munkiimport metadata generation", Latency: LatencyMinutes},
				windows: {Mechanism: MechServer, Native: "control-plane watcher; winget manifest ingester", Latency: LatencyMinutes},
			},
			Parity: []string{"xavier", "fleet"},
		},
		Capability{
			Name: "package.manager.upgrade", Domain: DomainSoftware,
			Summary:    "Developer package managers (Homebrew, npm, pip, RubyGems) have no outdated packages with known vulnerabilities.",
			Params:     []Param{{Name: "managers", Type: "[]string", Description: "brew, npm, pip, gem", Default: []string{"brew", "npm", "pip", "gem"}}, {Name: "only_vulnerable", Type: "bool", Description: "Upgrade only packages with a known CVE", Default: true}},
			Reversible: true, UserImpact: intent.ImpactNone, Privilege: PrivilegeUser, BaseRisk: 12,
			Bindings: map[intent.Platform]Binding{
				darwin:  {Mechanism: MechPackageMgr, Native: "brew upgrade <formula>, npm install -g <pkg>@<ver>, pip install -U <pkg>, gem update <gem> invoked directly per package (no shell)", Adapter: "unix.pkgmgr", Latency: LatencyMinutes, Notes: "Runs as the owning user; inventory from homebrew_packages, npm_packages, python_packages and the ADM ruby_gems table."},
				linux:   {Mechanism: MechPackageMgr, Native: "same as darwin (Linuxbrew supported)", Adapter: "unix.pkgmgr", Latency: LatencyMinutes},
				windows: {Mechanism: MechPackageMgr, Native: "npm/pip/gem invoked directly; winget via WinRT PackageManager", Adapter: "windows.pkgmgr", Latency: LatencyMinutes},
			},
			Verify: map[intent.Platform]Verify{
				darwin: {Query: "SELECT 1 FROM npm_packages WHERE name = '{{.package}}' AND version = '{{.version}}'", Expect: "rows>0"},
			},
			Parity: []string{"xavier"},
		},
		Capability{
			Name: "dependency.cve.scan", Domain: DomainSoftware, ReadOnly: true,
			Summary:    "Language dependencies (lockfiles and installed packages) are matched against OSV/NVD; findings become facts the planner can act on.",
			Reversible: true, UserImpact: intent.ImpactNone, Privilege: PrivilegeSystem, BaseRisk: 0,
			Bindings: map[intent.Platform]Binding{
				darwin:  {Mechanism: MechOsquery, Native: "npm_packages, python_packages, homebrew_packages, ruby_gems (ADM) + control-plane OSV matcher (cmd/osv-processor)", Latency: LatencyMinutes},
				windows: {Mechanism: MechOsquery, Native: "npm_packages, python_packages, chocolatey_packages + OSV matcher", Latency: LatencyMinutes},
				linux:   {Mechanism: MechOsquery, Native: "npm_packages, python_packages, deb/rpm packages + OVAL/OSV matcher", Latency: LatencyMinutes},
			},
			Parity: []string{"xavier", "fleet"},
		},
		Capability{
			Name: "runtime.eol.flag", Domain: DomainSoftware, ReadOnly: true,
			Summary:    "Language runtimes past end-of-life (Python, Node, Ruby, Java, Go) are flagged, using the endoflife.date feed.",
			Reversible: true, UserImpact: intent.ImpactNone, Privilege: PrivilegeSystem, BaseRisk: 0,
			Bindings: map[intent.Platform]Binding{
				darwin:  {Mechanism: MechOsquery, Native: "homebrew_packages, apps, python_packages + endoflife.date product cycles", Latency: LatencyMinutes},
				windows: {Mechanism: MechOsquery, Native: "programs + endoflife.date", Latency: LatencyMinutes},
				linux:   {Mechanism: MechOsquery, Native: "deb_packages/rpm_packages + endoflife.date", Latency: LatencyMinutes},
			},
			Parity: []string{"xavier"},
		},
		// ------------------------------------------------------------------- AI
		Capability{
			Name: "ai.tools.govern", Domain: DomainAI,
			Summary:    "AI apps, coding agents, MCP servers and browser extensions are inventoried; tools on the block list cannot run or reach their provider.",
			Params:     []Param{{Name: "block", Type: "[]string", Description: "AI tools to block (names from the ai_tools knowledge base)"}, {Name: "allow", Type: "[]string", Description: "Explicitly approved tools"}, {Name: "block_providers", Type: "[]string", Description: "Provider domains to block network access to"}},
			Reversible: true, UserImpact: intent.ImpactNone, Privilege: PrivilegeSystem, BaseRisk: 10,
			Bindings: map[intent.Platform]Binding{
				darwin:  {Mechanism: MechEndpointSec, Native: "inventory: ai_tools + mcp_listening_servers osquery tables; enforce: ES_EVENT_TYPE_AUTH_EXEC deny by signing ID / path, web.block for providers", Adapter: "darwin.es", Latency: LatencyMillis, Event: "ES_EVENT_TYPE_NOTIFY_EXEC"},
				windows: {Mechanism: MechCSP, Native: "inventory: ai_tools; enforce: ./Vendor/MSFT/AppLocker/ApplicationLaunchRestrictions/{group}/EXE/Policy + web.block", Adapter: "windows.csp", Latency: LatencyMillis, Event: "ETW Microsoft-Windows-AppLocker"},
				linux:   {Mechanism: MechDBus, Native: "inventory: ai_tools; enforce: fapolicyd rules / AppArmor deny profiles + web.block", Adapter: "linux.fapolicyd", Latency: LatencyMillis},
			},
			Verify: map[intent.Platform]Verify{
				darwin:  {Query: "SELECT 1 FROM ai_tools WHERE running = 1 AND name IN ({{.block_list}})", Expect: "rows==0"},
				windows: {Query: "SELECT 1 FROM ai_tools WHERE running = 1 AND name IN ({{.block_list}})", Expect: "rows==0"},
				linux:   {Query: "SELECT 1 FROM ai_tools WHERE running = 1 AND name IN ({{.block_list}})", Expect: "rows==0"},
			},
			Parity: []string{"xavier", "fleet"},
		},
		// ------------------------------------------------------------------- OS
		Capability{
			Name: "os.update.enforce", Domain: DomainOS,
			Summary:    "The OS is at or above a minimum version by a deadline, with the update staged natively rather than nagged.",
			Params:     []Param{{Name: "min_version", Type: "string", Required: true, Description: "Minimum OS version"}, {Name: "deadline_days", Type: "int", Description: "Days until forced install", Default: 7}},
			Reversible: false, UserImpact: intent.ImpactRestart, Privilege: PrivilegeMDM, BaseRisk: 18,
			Bindings: map[intent.Platform]Binding{
				darwin:  {Mechanism: MechDDM, Native: "com.apple.configuration.softwareupdate.enforcement.specific (TargetOSVersion, TargetLocalDateTime)", Adapter: "darwin.ddm", Latency: LatencyMinutes},
				windows: {Mechanism: MechWin32, Native: "Windows Update Agent COM IUpdateSession (search/download/install) + ./Device/Vendor/MSFT/Policy/Config/Update/ConfigureDeadlineForQualityUpdates", Adapter: "windows.wua", Latency: LatencyMinutes},
				linux:   {Mechanism: MechDBus, Native: "org.freedesktop.PackageKit.Transaction.UpdatePackages (kernel/security) + unattended-upgrades / dnf-automatic config", Adapter: "linux.packagekit", Latency: LatencyMinutes},
				ios:     {Mechanism: MechDDM, Native: "com.apple.configuration.softwareupdate.enforcement.specific", Latency: LatencyMinutes},
				android: {Mechanism: MechAMAPI, Native: "policy.systemUpdate (WINDOWED/AUTOMATIC)", Latency: LatencyMinutes},
			},
			Verify: map[intent.Platform]Verify{
				darwin:  {Query: "SELECT 1 FROM os_version WHERE version >= '{{.min_version}}'", Expect: "rows>0"},
				windows: {Query: "SELECT 1 FROM os_version WHERE build >= '{{.min_version}}'", Expect: "rows>0"},
				linux:   {Query: "SELECT 1 FROM kernel_info WHERE version >= '{{.min_version}}'", Expect: "rows>0"},
			},
			Parity: []string{"fleet", "intune", "xavier"},
		},
		// ------------------------------------------------------------------- identity
		Capability{
			Name: "identity.local_admin.manage", Domain: DomainIdentity,
			Summary:    "A managed local administrator account exists with a rotated, escrowed password; end users are standard users.",
			Params:     []Param{{Name: "username", Type: "string", Required: true, Description: "Managed admin account name"}, {Name: "rotate_days", Type: "int", Description: "Password rotation period", Default: 30}},
			Reversible: true, UserImpact: intent.ImpactNone, Privilege: PrivilegeSystem, BaseRisk: 16,
			Bindings: map[intent.Platform]Binding{
				darwin:  {Mechanism: MechAppleBinary, Native: "sysadminctl -addUser / -resetPasswordFor; MDM AccountConfiguration at ADE", Adapter: "darwin.sysadminctl", Latency: LatencySeconds},
				windows: {Mechanism: MechCSP, Native: "./Device/Vendor/MSFT/LAPS/Policies + ./Device/Vendor/MSFT/Accounts/Users; NetUserAdd (netapi32) for immediate creation", Adapter: "windows.accounts", Latency: LatencyMillis},
				linux:   {Mechanism: MechDBus, Native: "org.freedesktop.Accounts.CreateUser + SetPassword", Adapter: "linux.accountsservice", Latency: LatencyMillis},
			},
			Verify: map[intent.Platform]Verify{
				darwin:  {Query: "SELECT 1 FROM users u JOIN user_groups ug ON u.uid = ug.uid JOIN groups g ON ug.gid = g.gid WHERE u.username = '{{.username}}' AND g.groupname = 'admin'", Expect: "rows>0"},
				windows: {Query: "SELECT 1 FROM users WHERE username = '{{.username}}' AND type = 'local'", Expect: "rows>0"},
				linux:   {Query: "SELECT 1 FROM users WHERE username = '{{.username}}'", Expect: "rows>0"},
			},
			Parity: []string{"fleet", "intune"},
		},
		Capability{
			Name: "certificate.issue", Domain: DomainIdentity,
			Summary:    "A device or user identity certificate is issued from the configured CA (SCEP, ACME with attestation, EST).",
			Params:     []Param{{Name: "ca", Type: "string", Required: true, Description: "Certificate authority name"}, {Name: "subject", Type: "string", Description: "Subject template"}},
			Reversible: true, UserImpact: intent.ImpactNone, Privilege: PrivilegeMDM, BaseRisk: 8,
			Bindings: map[intent.Platform]Binding{
				darwin:  {Mechanism: MechMDMProfile, Native: "com.apple.security.scep / com.apple.security.acme (hardware attestation)", Latency: LatencySeconds},
				windows: {Mechanism: MechCSP, Native: "./Vendor/MSFT/ClientCertificateInstall/SCEP", Adapter: "windows.csp", Latency: LatencySeconds},
				linux:   {Mechanism: MechDBus, Native: "org.fedorahosted.certmonger.request / direct SCEP client into /etc/pki", Adapter: "linux.certmonger", Latency: LatencySeconds},
				ios:     {Mechanism: MechMDMProfile, Native: "com.apple.security.acme", Latency: LatencySeconds},
				android: {Mechanism: MechAMAPI, Native: "policy.choosePrivateKeyRules + delegated scope CERT_INSTALL", Latency: LatencyMinutes},
			},
			Parity: []string{"fleet", "intune", "xavier"},
		},
		Capability{
			Name: "compliance.signal", Domain: DomainIdentity,
			Summary:    "The device's compliance state and risk score are published to the identity provider for conditional access.",
			Params:     []Param{{Name: "provider", Type: "string", Required: true, Description: "entra or okta"}},
			Reversible: true, UserImpact: intent.ImpactLogout, Privilege: PrivilegeSystem, BaseRisk: 14,
			Bindings: map[intent.Platform]Binding{
				darwin:  {Mechanism: MechServer, Native: "Fleet conditional access (Entra device compliance / Okta device trust IdP) extended with ADM risk score and reason codes", Latency: LatencySeconds},
				windows: {Mechanism: MechServer, Native: "Entra device compliance via Fleet proxy; native Windows compliance through Intune when co-managed", Latency: LatencySeconds},
				linux:   {Mechanism: MechServer, Native: "Okta device trust via Fleet IdP", Latency: LatencySeconds},
			},
			Parity: []string{"fleet", "intune"},
		},
		// ------------------------------------------------------------------- signage
		Capability{
			Name: "signage.kiosk", Domain: DomainSignage,
			Summary:    "The device runs a single signage app full-screen and self-heals into it after reboot.",
			Params:     []Param{{Name: "app", Type: "string", Required: true, Description: "Bundle ID / package name / AUMID of the signage app"}, {Name: "content_url", Type: "string", Description: "Playlist or content URL served by ADM signage"}},
			Reversible: true, UserImpact: intent.ImpactLogout, Privilege: PrivilegeMDM, BaseRisk: 12,
			Bindings: map[intent.Platform]Binding{
				ipados:  {Mechanism: MechMDMProfile, Native: "com.apple.app.lock (Single App Mode, supervised) + com.apple.applicationaccess", Latency: LatencySeconds},
				tvos:    {Mechanism: MechMDMProfile, Native: "com.apple.app.lock (Single App Mode) on tvOS", Latency: LatencySeconds},
				android: {Mechanism: MechAMAPI, Native: "policy.kioskCustomization + applications[].installType=KIOSK", Latency: LatencyMinutes},
				windows: {Mechanism: MechCSP, Native: "./Device/Vendor/MSFT/AssignedAccess/Configuration", Adapter: "windows.csp", Latency: LatencyMillis},
			},
			Parity: []string{"xavier"},
		},
		// ------------------------------------------------------------------- lifecycle
		Capability{
			Name: "host.lock", Domain: DomainLifecycle,
			Summary:    "Lock the device immediately with a PIN or message.",
			Params:     []Param{{Name: "message", Type: "string", Description: "Lock screen message"}},
			Reversible: true, UserImpact: intent.ImpactLogout, Privilege: PrivilegeMDM, BaseRisk: 20,
			Bindings: map[intent.Platform]Binding{
				darwin:  {Mechanism: MechMDMCommand, Native: "DeviceLock (PIN)", Latency: LatencySeconds},
				windows: {Mechanism: MechCSP, Native: "./Vendor/MSFT/RemoteLock/Lock (Exec)", Adapter: "windows.csp", Latency: LatencySeconds},
				linux:   {Mechanism: MechDBus, Native: "org.freedesktop.login1.Manager.LockSessions + pam_faillock", Adapter: "linux.login1", Latency: LatencyMillis},
				ios:     {Mechanism: MechMDMCommand, Native: "DeviceLock", Latency: LatencySeconds},
				android: {Mechanism: MechAMAPI, Native: "devices.issueCommand LOCK", Latency: LatencySeconds},
			},
			Parity: []string{"fleet", "intune", "xavier"},
		},
		Capability{
			Name: "host.wipe", Domain: DomainLifecycle,
			Summary:    "Erase the device.",
			Reversible: false, Destructive: true, UserImpact: intent.ImpactRestart, Privilege: PrivilegeMDM, BaseRisk: 40,
			Bindings: map[intent.Platform]Binding{
				darwin:  {Mechanism: MechMDMCommand, Native: "EraseDevice", Latency: LatencySeconds},
				windows: {Mechanism: MechCSP, Native: "./Device/Vendor/MSFT/RemoteWipe/doWipe (Exec)", Adapter: "windows.csp", Latency: LatencySeconds},
				linux:   {Mechanism: MechDBus, Native: "LUKS header erase via org.freedesktop.UDisks2 (key destruction) then reboot", Adapter: "linux.luks", Latency: LatencySeconds},
				ios:     {Mechanism: MechMDMCommand, Native: "EraseDevice", Latency: LatencySeconds},
				android: {Mechanism: MechAMAPI, Native: "devices.delete (wipe)", Latency: LatencySeconds},
			},
			Parity: []string{"fleet", "intune", "xavier"},
		},
		// ------------------------------------------------------------------- enrollment
		Capability{
			Name: "enrollment.autopilot.register", Domain: DomainEnrollment,
			Summary:    "A Windows device is registered with Windows Autopilot (hardware hash), tagged, and assigned a deployment profile so it enrolls into ADM out of the box.",
			Params:     []Param{{Name: "serial", Type: "string", Required: true, Description: "Device serial"}, {Name: "hardware_hash", Type: "string", Required: true, Description: "OA3 hardware hash"}, {Name: "group_tag", Type: "string", Description: "Autopilot group tag"}},
			Reversible: true, UserImpact: intent.ImpactNone, Privilege: PrivilegeMDM, BaseRisk: 6,
			Bindings: map[intent.Platform]Binding{
				windows: {Mechanism: MechServer, Native: "Microsoft Graph POST /deviceManagement/importedWindowsAutopilotDeviceIdentities; windowsAutopilotDeploymentProfiles assignment; Entra automatic enrollment discovery hosted by Fleet", Latency: LatencyMinutes},
			},
			Parity: []string{"intune", "fleet"},
		},
		Capability{
			Name: "enrollment.ade.assign", Domain: DomainEnrollment,
			Summary:    "An Apple device in ABM/ASM is assigned an enrollment profile so it enrolls into ADM at activation.",
			Params:     []Param{{Name: "serial", Type: "string", Required: true, Description: "Device serial"}, {Name: "fleet", Type: "string", Description: "Fleet (team) to land in"}},
			Reversible: true, UserImpact: intent.ImpactNone, Privilege: PrivilegeMDM, BaseRisk: 6,
			Bindings: map[intent.Platform]Binding{
				darwin: {Mechanism: MechServer, Native: "Apple DEP API profile assignment via Fleet (apple_mdm_dep_profile_assigner)", Latency: LatencyMinutes},
				ios:    {Mechanism: MechServer, Native: "Apple DEP API profile assignment", Latency: LatencyMinutes},
				tvos:   {Mechanism: MechServer, Native: "Apple DEP API profile assignment", Latency: LatencyMinutes},
			},
			Parity: []string{"fleet", "intune", "xavier"},
		},
		Capability{
			Name: "enrollment.android.provision", Domain: DomainEnrollment,
			Summary:    "An Android device is provisioned zero-touch (Knox Mobile Enrollment or Google zero-touch) or by QR with an enrollment token.",
			Params:     []Param{{Name: "method", Type: "string", Description: "zero_touch, knox, qr", Default: "zero_touch"}, {Name: "fully_managed", Type: "bool", Description: "Fully managed vs work profile", Default: true}},
			Reversible: true, UserImpact: intent.ImpactNone, Privilege: PrivilegeMDM, BaseRisk: 6,
			Bindings: map[intent.Platform]Binding{
				android: {Mechanism: MechAMAPI, Native: "enterprises.enrollmentTokens.create (+ zero-touch DPC extras / Knox KME profile pointing at the token)", Latency: LatencyMinutes},
			},
			Parity: []string{"fleet", "xavier", "intune"},
		},
		// ------------------------------------------------------------------- telemetry
		Capability{
			Name: "telemetry.query", Domain: DomainTelemetry, ReadOnly: true,
			Summary:    "Ask any device any question with osquery and get an answer in seconds.",
			Params:     []Param{{Name: "sql", Type: "string", Required: true, Description: "osquery SQL"}},
			Reversible: true, UserImpact: intent.ImpactNone, Privilege: PrivilegeSystem, BaseRisk: 1,
			Bindings: map[intent.Platform]Binding{
				darwin:  {Mechanism: MechOsquery, Native: "Fleet live query (distributed/read nudged over the agent WebSocket, ADR-0011)", Latency: LatencySeconds},
				windows: {Mechanism: MechOsquery, Native: "Fleet live query", Latency: LatencySeconds},
				linux:   {Mechanism: MechOsquery, Native: "Fleet live query", Latency: LatencySeconds},
			},
			Parity: []string{"fleet"},
		},
		Capability{
			Name: "script.run", Domain: DomainLifecycle,
			Summary:    "Run a shell or PowerShell script. Last resort: ADM prefers a native capability and flags every plan that needs this.",
			Params:     []Param{{Name: "script", Type: "string", Required: true, Description: "Script body"}},
			Reversible: false, UserImpact: intent.ImpactNone, Privilege: PrivilegeSystem, BaseRisk: 22,
			Bindings: map[intent.Platform]Binding{
				darwin:  {Mechanism: MechScript, Native: "/bin/sh via Fleet script runner (5 minute cap)", Latency: LatencySeconds},
				windows: {Mechanism: MechScript, Native: "powershell -MTA -ExecutionPolicy Bypass -File via Fleet script runner", Latency: LatencySeconds},
				linux:   {Mechanism: MechScript, Native: "/bin/sh via Fleet script runner", Latency: LatencySeconds},
			},
			Parity: []string{"fleet", "intune", "xavier"},
		},
	)
}
