// Package darwin realises ADM capabilities on macOS, iOS, iPadOS and tvOS
// through Apple's declarative device management (DDM) declarations,
// configuration profile payloads, MDM commands and, on macOS, the Endpoint
// Security and NetworkExtension frameworks running inside the ADM agent's
// system extension.
//
// Declarations are the preferred mechanism: the device evaluates them itself,
// re-applies them without a server round trip, and reports status
// asynchronously, which is exactly the device-side autonomy an agentic
// control plane wants. This package composes declarations as plain JSON so
// the mapping is testable anywhere and pushes them through a Pusher, which
// the substrate implements on top of Fleet's MDM server.
package darwin

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/fleetdm/fleet/v4/adm/intent"
	"github.com/fleetdm/fleet/v4/adm/native"
)

// Declaration is one DDM configuration declaration.
type Declaration struct {
	Type        string         `json:"Type"`
	Identifier  string         `json:"Identifier"`
	Payload     map[string]any `json:"Payload"`
	ServerToken string         `json:"ServerToken,omitempty"`
}

// Pusher delivers declarations (and legacy profiles) to a device and observes
// their status. The substrate implements it with Fleet's MDM endpoints.
type Pusher interface {
	PushDeclaration(ctx context.Context, t native.Target, d Declaration) error
	RemoveDeclaration(ctx context.Context, t native.Target, identifier string) error
	// DeclarationStatus returns "active", "pending", "failed" or "unknown".
	DeclarationStatus(ctx context.Context, t native.Target, identifier string) (string, error)
}

// Identifier derives a stable declaration identifier from its content.
func Identifier(capability string, payload map[string]any) string {
	b, _ := json.Marshal(payload)
	sum := sha256.Sum256(append([]byte(capability), b...))
	return "com.adm." + capability + "." + hex.EncodeToString(sum[:6])
}

// ---- declaration builders --------------------------------------------------

// DiskManagement builds com.apple.configuration.diskmanagement.settings
// (macOS 15+). mode is "block" (Disallowed) or "read_only" (ReadOnly).
func DiskManagement(mode string) (Declaration, error) {
	var ext string
	switch mode {
	case "block":
		ext = "Disallowed"
	case "read_only":
		ext = "ReadOnly"
	default:
		return Declaration{}, fmt.Errorf("storage.removable.block: mode must be block or read_only, got %q", mode)
	}
	payload := map[string]any{"Restrictions": map[string]any{"ExternalStorage": ext, "NetworkStorage": "Allowed"}}
	return Declaration{Type: "com.apple.configuration.diskmanagement.settings", Identifier: Identifier("storage.removable.block", payload), Payload: payload}, nil
}

// ScreenSaver builds com.apple.configuration.screensaver.settings with an idle
// timeout and password requirement.
func ScreenSaver(idleMinutes int) (Declaration, error) {
	if idleMinutes <= 0 {
		return Declaration{}, fmt.Errorf("screen.lock.enforce: idle_minutes must be a positive integer")
	}
	payload := map[string]any{"LoginWindowIdleTime": idleMinutes * 60}
	return Declaration{Type: "com.apple.configuration.screensaver.settings", Identifier: Identifier("screen.lock.enforce", payload), Payload: payload}, nil
}

// PasscodeGrace builds com.apple.configuration.passcode.settings requiring a
// password shortly after the screen saver starts.
func PasscodeGrace(graceMinutes int) Declaration {
	payload := map[string]any{"RequirePasscode": true, "MaximumGracePeriodInMinutes": graceMinutes}
	return Declaration{Type: "com.apple.configuration.passcode.settings", Identifier: Identifier("screen.lock.enforce.passcode", payload), Payload: payload}
}

// SoftwareUpdateEnforcement builds
// com.apple.configuration.softwareupdate.enforcement.specific.
func SoftwareUpdateEnforcement(version string, deadline time.Time) (Declaration, error) {
	if version == "" {
		return Declaration{}, fmt.Errorf("os.update.enforce: min_version is required")
	}
	payload := map[string]any{
		"TargetOSVersion":     version,
		"TargetLocalDateTime": deadline.Format("2006-01-02T15:04:05"),
		"DetailsURL":          "https://adm.local/updates",
	}
	return Declaration{Type: "com.apple.configuration.softwareupdate.enforcement.specific", Identifier: Identifier("os.update.enforce", payload), Payload: payload}, nil
}

// WebContentFilter builds the built-in content filter payload as a
// declaration-wrapped legacy profile (com.apple.configuration.legacy) so it can
// be delivered over the same channel until Apple ships a native declaration.
func WebContentFilter(domains []string) (Declaration, error) {
	if len(domains) == 0 {
		return Declaration{}, fmt.Errorf("web.block: domains is required")
	}
	deny := make([]string, 0, len(domains))
	for _, d := range domains {
		deny = append(deny, "https://"+d, "http://"+d)
	}
	payload := map[string]any{
		"PayloadType":       "com.apple.webcontent-filter",
		"FilterType":        "BuiltIn",
		"AutoFilterEnabled": false,
		"DenyListURLs":      deny,
	}
	return Declaration{Type: "com.apple.configuration.legacy", Identifier: Identifier("web.block", payload), Payload: payload}, nil
}

// ---- adapter ----------------------------------------------------------------

// DDMAdapter converges one declaration per capability through a Pusher.
type DDMAdapter struct {
	id         string
	capability string
	platform   intent.Platform
	pusher     Pusher
	build      func(params map[string]any) (Declaration, error)
}

// NewDDMAdapter builds an adapter for a capability on a platform.
func NewDDMAdapter(id, capability string, platform intent.Platform, pusher Pusher, build func(map[string]any) (Declaration, error)) *DDMAdapter {
	return &DDMAdapter{id: id, capability: capability, platform: platform, pusher: pusher, build: build}
}

func (a *DDMAdapter) ID() string                { return a.id }
func (a *DDMAdapter) Capability() string        { return a.capability }
func (a *DDMAdapter) Platform() intent.Platform { return a.platform }

// Check reports satisfied when the declaration is already active.
func (a *DDMAdapter) Check(ctx context.Context, t native.Target, params map[string]any) (native.State, error) {
	d, err := a.build(params)
	if err != nil {
		return nil, err
	}
	status, err := a.pusher.DeclarationStatus(ctx, t, d.Identifier)
	if err != nil {
		return nil, err
	}
	return native.State{"identifier": d.Identifier, "status": status, native.Satisfied: status == "active"}, nil
}

// Apply pushes the declaration.
func (a *DDMAdapter) Apply(ctx context.Context, t native.Target, params map[string]any) (native.Result, error) {
	d, err := a.build(params)
	if err != nil {
		return native.Result{}, err
	}
	start := time.Now()
	if err := a.pusher.PushDeclaration(ctx, t, d); err != nil {
		return native.Result{}, err
	}
	return native.Result{Changed: true, Native: d.Type + " " + d.Identifier, Duration: time.Since(start), Evidence: map[string]any{"declaration": d}}, nil
}

// Revert removes the declaration; the device restores its default state.
func (a *DDMAdapter) Revert(ctx context.Context, t native.Target, params map[string]any, prior native.State) (native.Result, error) {
	d, err := a.build(params)
	if err != nil {
		return native.Result{}, err
	}
	if err := a.pusher.RemoveDeclaration(ctx, t, d.Identifier); err != nil {
		return native.Result{}, err
	}
	return native.Result{Changed: true, Native: "remove " + d.Identifier}, nil
}

// NewAdapters returns the Apple adapters bound to a Pusher.
func NewAdapters(p Pusher) []native.Adapter {
	removable := func(params map[string]any) (Declaration, error) {
		mode, _ := params["mode"].(string)
		return DiskManagement(mode)
	}
	screen := func(params map[string]any) (Declaration, error) {
		n, _ := asInt(params["idle_minutes"])
		return ScreenSaver(n)
	}
	update := func(params map[string]any) (Declaration, error) {
		v, _ := params["min_version"].(string)
		days, ok := asInt(params["deadline_days"])
		if !ok || days <= 0 {
			days = 7
		}
		return SoftwareUpdateEnforcement(v, time.Now().Add(time.Duration(days)*24*time.Hour))
	}
	web := func(params map[string]any) (Declaration, error) {
		return WebContentFilter(asStrings(params["domains"]))
	}
	return []native.Adapter{
		NewDDMAdapter("darwin.ddm.removable", "storage.removable.block", intent.PlatformDarwin, p, removable),
		NewDDMAdapter("darwin.ddm.screenlock", "screen.lock.enforce", intent.PlatformDarwin, p, screen),
		NewDDMAdapter("darwin.ddm.osupdate", "os.update.enforce", intent.PlatformDarwin, p, update),
		NewDDMAdapter("darwin.ddm.webfilter", "web.block", intent.PlatformDarwin, p, web),
		NewDDMAdapter("ios.ddm.osupdate", "os.update.enforce", intent.PlatformIOS, p, update),
	}
}

func asInt(v any) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case int64:
		return int(n), true
	case float64:
		return int(n), true
	}
	return 0, false
}

func asStrings(v any) []string {
	switch vv := v.(type) {
	case []string:
		return vv
	case []any:
		out := make([]string, 0, len(vv))
		for _, e := range vv {
			if s, ok := e.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}
