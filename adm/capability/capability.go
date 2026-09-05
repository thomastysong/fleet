// Package capability holds the capability catalog: the vocabulary of things
// ADM can observe and change on a device, and how each of them is realised
// natively on every platform.
//
// A capability is deliberately more abstract than a profile, a CSP node or a
// script. "storage.removable.block" is one capability; on macOS it becomes a
// declarative disk-management declaration plus an Endpoint Security mount
// authorisation, on Windows a Policy CSP node applied through the local MDM
// management stack, on Linux a udev rule and a USBGuard policy, on Android an
// AMAPI policy field. The catalog is what lets an operator's plain-language
// intent compile to native, verifiable actions on every platform at once.
package capability

import (
	"fmt"
	"sort"

	"github.com/fleetdm/fleet/v4/adm/intent"
)

// Domain groups capabilities for navigation and for risk defaults.
type Domain string

const (
	DomainSecurity   Domain = "security"
	DomainSoftware   Domain = "software"
	DomainNetwork    Domain = "network"
	DomainStorage    Domain = "storage"
	DomainIdentity   Domain = "identity"
	DomainAI         Domain = "ai"
	DomainSignage    Domain = "signage"
	DomainOS         Domain = "os"
	DomainEnrollment Domain = "enrollment"
	DomainTelemetry  Domain = "telemetry"
	DomainLifecycle  Domain = "lifecycle"
)

// Mechanism names the class of native interface a binding uses.
type Mechanism string

const (
	MechDDM         Mechanism = "ddm"               // Apple declarative device management
	MechMDMProfile  Mechanism = "mdm-profile"       // Apple configuration profile payload
	MechMDMCommand  Mechanism = "mdm-command"       // Apple MDM command
	MechCSP         Mechanism = "csp"               // Windows CSP via OMA-DM or local MDM stack
	MechWMI         Mechanism = "wmi"               // Windows WMI/CIM class call
	MechWin32       Mechanism = "win32"             // Windows Win32 / WinRT / COM API
	MechEndpointSec Mechanism = "endpoint-security" // macOS Endpoint Security framework
	MechNetExt      Mechanism = "network-extension" // macOS NetworkExtension
	MechAppleBinary Mechanism = "apple-binary"      // vendor-provided binary invoked directly (no shell)
	MechDBus        Mechanism = "dbus"              // Linux D-Bus service call
	MechUdev        Mechanism = "udev"              // Linux udev rules and events
	MechNetlink     Mechanism = "netlink"           // Linux netlink (nftables, routes)
	MechDconf       Mechanism = "dconf"             // Linux GNOME configuration database
	MechAMAPI       Mechanism = "amapi"             // Android Management API policy
	MechPackageMgr  Mechanism = "package-manager"   // direct package manager invocation
	MechServer      Mechanism = "server"            // realised in the control plane, not on device
	MechOsquery     Mechanism = "osquery"           // observe via osquery table
	MechScript      Mechanism = "script"            // shell/PowerShell fallback (last resort)
)

// Latency is the order-of-magnitude time-to-effect of a binding.
type Latency string

const (
	LatencyMillis  Latency = "ms"
	LatencySeconds Latency = "s"
	LatencyMinutes Latency = "min"
)

// Binding is how a capability is realised on one platform.
type Binding struct {
	Mechanism Mechanism `json:"mechanism"`
	// Native is the concrete API, CSP node, declaration type or D-Bus method.
	Native string `json:"native"`
	// Adapter is the identifier of the native adapter that implements this
	// binding (see package native). Empty when the binding is server-side.
	Adapter string  `json:"adapter,omitempty"`
	Latency Latency `json:"latency"`
	// Event is the native event source that reports drift instantly, if any.
	Event string `json:"event,omitempty"`
	Notes string `json:"notes,omitempty"`
}

// Verify describes how ADM proves the capability holds on a platform, using
// the same osquery primitive that observes the device.
type Verify struct {
	Query  string `json:"query"`
	Expect string `json:"expect"` // "rows>0" or "rows==0"
}

// Param documents one parameter accepted by a capability.
type Param struct {
	Name        string `json:"name"`
	Type        string `json:"type"` // string, int, bool, []string, duration
	Required    bool   `json:"required,omitempty"`
	Description string `json:"description"`
	Default     any    `json:"default,omitempty"`
}

// Privilege is the level at which the change is made.
type Privilege string

const (
	PrivilegeUser   Privilege = "user"
	PrivilegeSystem Privilege = "system"
	PrivilegeMDM    Privilege = "mdm"
)

// Capability is one entry of the catalog.
type Capability struct {
	Name        string            `json:"name"`
	Domain      Domain            `json:"domain"`
	Summary     string            `json:"summary"`
	Params      []Param           `json:"params,omitempty"`
	Reversible  bool              `json:"reversible"`
	Destructive bool              `json:"destructive"`
	UserImpact  intent.UserImpact `json:"user_impact"`
	Privilege   Privilege         `json:"privilege"`
	// BaseRisk is the intrinsic risk of the change on a 0-40 scale, before
	// blast radius and context are considered (see package risk).
	BaseRisk int `json:"base_risk"`
	// ReadOnly capabilities observe and never change device state.
	ReadOnly bool                        `json:"read_only,omitempty"`
	Bindings map[intent.Platform]Binding `json:"bindings"`
	Verify   map[intent.Platform]Verify  `json:"verify,omitempty"`
	// Parity records which products already offer this (fleet, xavier, intune).
	Parity []string `json:"parity,omitempty"`
}

// Supports reports whether the capability has a binding for the platform.
func (c Capability) Supports(p intent.Platform) bool {
	_, ok := c.Bindings[p]
	return ok
}

// Platforms returns the platforms the capability supports, sorted.
func (c Capability) Platforms() []intent.Platform {
	out := make([]intent.Platform, 0, len(c.Bindings))
	for p := range c.Bindings {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// ValidateParams checks required parameters and basic types.
func (c Capability) ValidateParams(params map[string]any) error {
	for _, p := range c.Params {
		v, ok := params[p.Name]
		if !ok {
			if p.Required {
				return fmt.Errorf("capability %s: missing required param %q", c.Name, p.Name)
			}
			continue
		}
		if err := checkType(p, v); err != nil {
			return fmt.Errorf("capability %s: %w", c.Name, err)
		}
	}
	return nil
}

func checkType(p Param, v any) error {
	switch p.Type {
	case "string":
		if _, ok := v.(string); !ok {
			return fmt.Errorf("param %q must be a string", p.Name)
		}
	case "bool":
		if _, ok := v.(bool); !ok {
			return fmt.Errorf("param %q must be a bool", p.Name)
		}
	case "int":
		switch v.(type) {
		case int, int64, float64:
		default:
			return fmt.Errorf("param %q must be an int", p.Name)
		}
	case "[]string":
		switch vv := v.(type) {
		case []string:
		case []any:
			for _, e := range vv {
				if _, ok := e.(string); !ok {
					return fmt.Errorf("param %q must be a list of strings", p.Name)
				}
			}
		default:
			return fmt.Errorf("param %q must be a list of strings", p.Name)
		}
	}
	return nil
}

// Catalog is an indexed set of capabilities.
type Catalog struct {
	byName map[string]Capability
	order  []string
}

// NewCatalog builds a catalog from the given capabilities.
func NewCatalog(caps ...Capability) *Catalog {
	c := &Catalog{byName: make(map[string]Capability, len(caps))}
	for _, cap := range caps {
		if _, dup := c.byName[cap.Name]; dup {
			panic("duplicate capability " + cap.Name)
		}
		c.byName[cap.Name] = cap
		c.order = append(c.order, cap.Name)
	}
	sort.Strings(c.order)
	return c
}

// Get returns the named capability.
func (c *Catalog) Get(name string) (Capability, bool) {
	cap, ok := c.byName[name]
	return cap, ok
}

// Names returns every capability name, sorted.
func (c *Catalog) Names() []string {
	out := make([]string, len(c.order))
	copy(out, c.order)
	return out
}

// All returns every capability, sorted by name.
func (c *Catalog) All() []Capability {
	out := make([]Capability, 0, len(c.order))
	for _, n := range c.order {
		out = append(out, c.byName[n])
	}
	return out
}

// ByDomain returns the capabilities in a domain, sorted by name.
func (c *Catalog) ByDomain(d Domain) []Capability {
	var out []Capability
	for _, cap := range c.All() {
		if cap.Domain == d {
			out = append(out, cap)
		}
	}
	return out
}

// Len returns the number of capabilities.
func (c *Catalog) Len() int { return len(c.order) }
