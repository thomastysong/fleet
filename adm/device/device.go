// Package device holds the minimal inventory view ADM needs of a managed
// device. The authoritative inventory lives in the substrate (Fleet); this is
// the projection the planner and executor reason about.
package device

import (
	"sort"
	"strings"

	"github.com/fleetdm/fleet/v4/adm/intent"
)

// Host is one managed device.
type Host struct {
	ID        uint            `json:"id"`
	UUID      string          `json:"uuid,omitempty"`
	Hostname  string          `json:"hostname"`
	Serial    string          `json:"serial,omitempty"`
	Platform  intent.Platform `json:"platform"`
	OSVersion string          `json:"os_version,omitempty"`
	Fleet     string          `json:"fleet,omitempty"`
	Labels    []string        `json:"labels,omitempty"`
	Online    bool            `json:"online"`
}

// HasLabel reports whether the host carries the label.
func (h Host) HasLabel(name string) bool {
	for _, l := range h.Labels {
		if strings.EqualFold(l, name) {
			return true
		}
	}
	return false
}

// NormalizePlatform maps a substrate platform string (Fleet reports Linux
// hosts by distribution: "ubuntu", "rhel", ...) to an ADM platform.
func NormalizePlatform(s string) intent.Platform {
	switch p := strings.ToLower(strings.TrimSpace(s)); p {
	case "darwin", "macos", "mac":
		return intent.PlatformDarwin
	case "windows":
		return intent.PlatformWindows
	case "ios", "iphone":
		return intent.PlatformIOS
	case "ipados", "ipad":
		return intent.PlatformIPadOS
	case "tvos", "appletv":
		return intent.PlatformTVOS
	case "android":
		return intent.PlatformAndroid
	case "chrome", "chromeos":
		return intent.PlatformChromeOS
	case "linux", "ubuntu", "debian", "rhel", "centos", "fedora", "amzn", "arch", "gentoo", "sles", "opensuse", "opensuse-leap", "opensuse-tumbleweed", "nixos", "pop", "kali", "linuxmint", "manjaro", "void", "rocky", "almalinux", "oracle", "tuxedo", "neon":
		return intent.PlatformLinux
	}
	return intent.Platform(s)
}

// Filter applies the locally evaluable parts of a scope (fleets, labels,
// exclusions, platforms, explicit host IDs). The osquery Query selector is
// evaluated live by the substrate, not here.
func Filter(hosts []Host, scope intent.Scope) []Host {
	ids := map[uint]bool{}
	for _, id := range scope.HostIDs {
		ids[id] = true
	}
	plats := map[intent.Platform]bool{}
	for _, p := range scope.Platforms {
		plats[p] = true
	}
	var out []Host
	for _, h := range hosts {
		if len(ids) > 0 && !ids[h.ID] {
			continue
		}
		if len(plats) > 0 && !plats[h.Platform] {
			continue
		}
		if len(scope.Fleets) > 0 && !containsFold(scope.Fleets, h.Fleet) {
			continue
		}
		if !hasAll(h, scope.Labels) {
			continue
		}
		if hasAny(h, scope.ExcludeLabels) {
			continue
		}
		out = append(out, h)
	}
	return out
}

func hasAll(h Host, labels []string) bool {
	for _, l := range labels {
		if !h.HasLabel(l) {
			return false
		}
	}
	return true
}

func hasAny(h Host, labels []string) bool {
	for _, l := range labels {
		if h.HasLabel(l) {
			return true
		}
	}
	return false
}

func containsFold(list []string, v string) bool {
	for _, s := range list {
		if strings.EqualFold(s, v) {
			return true
		}
	}
	return false
}

// GroupByPlatform buckets hosts by platform.
func GroupByPlatform(hosts []Host) map[intent.Platform][]Host {
	out := map[intent.Platform][]Host{}
	for _, h := range hosts {
		out[h.Platform] = append(out[h.Platform], h)
	}
	return out
}

// IDs returns the host IDs, sorted.
func IDs(hosts []Host) []uint {
	out := make([]uint, 0, len(hosts))
	for _, h := range hosts {
		out = append(out, h.ID)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// Platforms returns the distinct platforms among the hosts, sorted.
func Platforms(hosts []Host) []intent.Platform {
	seen := map[intent.Platform]bool{}
	var out []intent.Platform
	for _, h := range hosts {
		if !seen[h.Platform] {
			seen[h.Platform] = true
			out = append(out, h.Platform)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}
