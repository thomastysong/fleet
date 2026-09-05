// Package windows realises ADM capabilities on Windows through Configuration
// Service Providers applied by the local MDM management stack
// (mdmlocalmanagement.dll ApplyLocalManagementSyncML, the same entry point
// fleetd's mdm_bridge osquery table uses), WMI/CIM classes and Win32 APIs.
// No PowerShell is involved: a change is a SyncML document handed to the OS,
// and it takes effect in milliseconds.
//
// The DLL call itself only compiles on Windows; this package composes and
// parses SyncML in pure Go so that the mapping from capability to CSP node is
// testable everywhere, and calls through the LocalMDM interface, which the
// agent satisfies with the real DLL binding.
package windows

import (
	"context"
	"encoding/xml"
	"fmt"
	"strings"
	"time"

	"github.com/fleetdm/fleet/v4/adm/intent"
	"github.com/fleetdm/fleet/v4/adm/native"
)

// LocalMDM applies a SyncML body to a device's management stack and returns
// the SyncML response. Inside the ADM agent it wraps
// ApplyLocalManagementSyncML for the local device; in the control plane the
// bridge package implements it over the substrate (mdm_bridge live queries
// for reads, queued MDM commands for writes).
type LocalMDM interface {
	Apply(ctx context.Context, t native.Target, syncBody string) (string, error)
}

// Format is the SyncML data format of a CSP node.
type Format string

const (
	FormatInt  Format = "int"
	FormatChr  Format = "chr"
	FormatBool Format = "bool"
	FormatXML  Format = "xml"
	FormatNull Format = "null"
)

// Setting is one CSP node and its desired value.
type Setting struct {
	LocURI string
	Format Format
	Value  string
}

// Verb is a SyncML command verb.
type Verb string

const (
	VerbGet     Verb = "Get"
	VerbAdd     Verb = "Add"
	VerbReplace Verb = "Replace"
	VerbDelete  Verb = "Delete"
	VerbExec    Verb = "Exec"
)

// Command is one SyncML command.
type Command struct {
	Verb    Verb
	Setting Setting
}

// SyncBody renders commands as the <SyncBody> fragment accepted by the local
// management stack (and by fleetd's mdm_bridge table).
func SyncBody(cmds ...Command) string {
	var b strings.Builder
	b.WriteString("<SyncBody>")
	for i, c := range cmds {
		fmt.Fprintf(&b, "<%s><CmdID>%d</CmdID><Item><Target><LocURI>%s</LocURI></Target>", c.Verb, i+1, escape(c.Setting.LocURI))
		if c.Verb != VerbGet && c.Verb != VerbDelete {
			if c.Setting.Format != "" {
				fmt.Fprintf(&b, "<Meta><Format xmlns=\"syncml:metinf\">%s</Format></Meta>", c.Setting.Format)
			}
			if c.Verb != VerbExec || c.Setting.Value != "" {
				fmt.Fprintf(&b, "<Data>%s</Data>", escape(c.Setting.Value))
			}
		}
		fmt.Fprintf(&b, "</Item></%s>", c.Verb)
	}
	b.WriteString("</SyncBody>")
	return b.String()
}

func escape(s string) string {
	var b strings.Builder
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

// Response is a parsed SyncML response.
type Response struct {
	// Status maps CmdRef to the SyncML status code ("200", "404", ...).
	Status map[string]string
	// Results maps LocURI to returned data for Get commands.
	Results map[string]string
}

type syncML struct {
	Body struct {
		Items []struct {
			XMLName xml.Name
			CmdRef  string `xml:"CmdRef"`
			Data    string `xml:"Data"`
			Item    []struct {
				Source string `xml:"Source>LocURI"`
				Data   string `xml:"Data"`
			} `xml:"Item"`
		} `xml:",any"`
	} `xml:"SyncBody"`
}

// ParseResponse extracts statuses and results from a SyncML response. It
// accepts either a full <SyncML> document or a bare <SyncBody>.
func ParseResponse(s string) (Response, error) {
	resp := Response{Status: map[string]string{}, Results: map[string]string{}}
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "<SyncBody") {
		s = "<SyncML>" + s + "</SyncML>"
	}
	var doc syncML
	d := xml.NewDecoder(strings.NewReader(s))
	if err := d.Decode(&doc); err != nil {
		return resp, fmt.Errorf("parse syncml: %w", err)
	}
	for _, it := range doc.Body.Items {
		switch it.XMLName.Local {
		case "Status":
			resp.Status[it.CmdRef] = it.Data
		case "Results":
			for _, item := range it.Item {
				resp.Results[item.Source] = item.Data
			}
		}
	}
	return resp, nil
}

// OK reports whether every status is 2xx.
func (r Response) OK() bool {
	if len(r.Status) == 0 {
		return false
	}
	for _, s := range r.Status {
		if !strings.HasPrefix(s, "2") {
			return false
		}
	}
	return true
}

// CSPAdapter is a generic adapter that converges a list of CSP settings.
// Capability-specific adapters are thin wrappers that map params to settings.
type CSPAdapter struct {
	id         string
	capability string
	mdm        LocalMDM
	settings   func(params map[string]any) ([]Setting, error)
	// exec, when set, makes Apply issue Exec commands (RemoteLock, RemoteWipe).
	exec bool
}

// NewCSPAdapter builds an adapter from a settings mapper.
func NewCSPAdapter(id, capability string, mdm LocalMDM, settings func(map[string]any) ([]Setting, error)) *CSPAdapter {
	return &CSPAdapter{id: id, capability: capability, mdm: mdm, settings: settings}
}

func (a *CSPAdapter) ID() string                { return a.id }
func (a *CSPAdapter) Capability() string        { return a.capability }
func (a *CSPAdapter) Platform() intent.Platform { return intent.PlatformWindows }

// Check reads every node and reports whether it already holds the value.
func (a *CSPAdapter) Check(ctx context.Context, t native.Target, params map[string]any) (native.State, error) {
	settings, err := a.settings(params)
	if err != nil {
		return nil, err
	}
	if a.exec {
		return native.State{native.Satisfied: false}, nil
	}
	cmds := make([]Command, len(settings))
	for i, s := range settings {
		cmds[i] = Command{Verb: VerbGet, Setting: Setting{LocURI: s.LocURI}}
	}
	raw, err := a.mdm.Apply(ctx, t, SyncBody(cmds...))
	if err != nil {
		return nil, fmt.Errorf("csp get: %w", err)
	}
	resp, err := ParseResponse(raw)
	if err != nil {
		return nil, err
	}
	state := native.State{}
	satisfied := true
	for _, s := range settings {
		cur, ok := resp.Results[s.LocURI]
		state[s.LocURI] = cur
		if !ok || cur != s.Value {
			satisfied = false
		}
	}
	state[native.Satisfied] = satisfied
	return state, nil
}

// Apply replaces every node with its desired value (Add on a 404).
func (a *CSPAdapter) Apply(ctx context.Context, t native.Target, params map[string]any) (native.Result, error) {
	settings, err := a.settings(params)
	if err != nil {
		return native.Result{}, err
	}
	verb := VerbReplace
	if a.exec {
		verb = VerbExec
	}
	return a.run(ctx, t, verb, settings)
}

// Revert restores the values captured by Check.
func (a *CSPAdapter) Revert(ctx context.Context, t native.Target, params map[string]any, prior native.State) (native.Result, error) {
	settings, err := a.settings(params)
	if err != nil {
		return native.Result{}, err
	}
	if a.exec {
		return native.Result{Changed: false, Native: "exec commands cannot be reverted"}, nil
	}
	var restore []Setting
	for _, s := range settings {
		prev, ok := prior[s.LocURI].(string)
		if !ok {
			continue
		}
		restore = append(restore, Setting{LocURI: s.LocURI, Format: s.Format, Value: prev})
	}
	if len(restore) == 0 {
		return native.Result{Changed: false, Native: "nothing to restore"}, nil
	}
	return a.run(ctx, t, VerbReplace, restore)
}

func (a *CSPAdapter) run(ctx context.Context, t native.Target, verb Verb, settings []Setting) (native.Result, error) {
	start := time.Now()
	cmds := make([]Command, len(settings))
	for i, s := range settings {
		cmds[i] = Command{Verb: verb, Setting: s}
	}
	body := SyncBody(cmds...)
	raw, err := a.mdm.Apply(ctx, t, body)
	if err != nil {
		return native.Result{}, fmt.Errorf("csp %s: %w", verb, err)
	}
	resp, err := ParseResponse(raw)
	if err != nil {
		return native.Result{}, err
	}
	// Nodes that do not exist yet answer 404 to Replace; retry those with Add.
	if verb == VerbReplace {
		var adds []Command
		for i, s := range settings {
			if resp.Status[fmt.Sprint(i+1)] == "404" {
				adds = append(adds, Command{Verb: VerbAdd, Setting: s})
			}
		}
		if len(adds) > 0 {
			raw2, err := a.mdm.Apply(ctx, t, SyncBody(adds...))
			if err != nil {
				return native.Result{}, fmt.Errorf("csp add: %w", err)
			}
			resp2, err := ParseResponse(raw2)
			if err != nil {
				return native.Result{}, err
			}
			if !resp2.OK() {
				return native.Result{}, fmt.Errorf("csp add failed: %v", resp2.Status)
			}
			return native.Result{Changed: true, Native: body, Duration: time.Since(start), Evidence: map[string]any{"status": resp2.Status}}, nil
		}
	}
	if !resp.OK() {
		return native.Result{}, fmt.Errorf("csp %s failed: %v", verb, resp.Status)
	}
	return native.Result{Changed: true, Native: body, Duration: time.Since(start), Evidence: map[string]any{"status": resp.Status}}, nil
}

// ---- capability mappers ----------------------------------------------------

const (
	policyRoot   = "./Device/Vendor/MSFT/Policy/Config/"
	firewallRoot = "./Vendor/MSFT/Firewall/MdmStore/"
)

// RemovableStorageSettings maps storage.removable.block params to CSP nodes.
func RemovableStorageSettings(params map[string]any) ([]Setting, error) {
	mode, _ := params["mode"].(string)
	switch mode {
	case "read_only":
		return []Setting{{LocURI: policyRoot + "Storage/RemovableDiskDenyWriteAccess", Format: FormatInt, Value: "1"}}, nil
	case "block":
		return []Setting{
			{LocURI: policyRoot + "Storage/RemovableDiskDenyWriteAccess", Format: FormatInt, Value: "1"},
			{LocURI: policyRoot + "ADMX_RemovableStorage/RemovableDisks_DenyRead_Access_2", Format: FormatChr, Value: "<enabled/>"},
			{LocURI: policyRoot + "ADMX_RemovableStorage/RemovableDisks_DenyExecute_Access_2", Format: FormatChr, Value: "<enabled/>"},
		}, nil
	}
	return nil, fmt.Errorf("storage.removable.block: mode must be block or read_only, got %q", mode)
}

// FirewallSettings enables the firewall on every profile.
func FirewallSettings(map[string]any) ([]Setting, error) {
	var out []Setting
	for _, p := range []string{"DomainProfile", "PrivateProfile", "PublicProfile"} {
		out = append(out, Setting{LocURI: firewallRoot + p + "/EnableFirewall", Format: FormatBool, Value: "true"})
	}
	return out, nil
}

// ScreenLockSettings maps screen.lock.enforce params to DeviceLock nodes.
func ScreenLockSettings(params map[string]any) ([]Setting, error) {
	minutes, ok := asInt(params["idle_minutes"])
	if !ok || minutes <= 0 {
		return nil, fmt.Errorf("screen.lock.enforce: idle_minutes must be a positive integer")
	}
	return []Setting{
		{LocURI: policyRoot + "DeviceLock/DevicePasswordEnabled", Format: FormatInt, Value: "0"}, // 0 = enabled in this CSP
		{LocURI: policyRoot + "DeviceLock/MaxInactivityTimeDeviceLock", Format: FormatInt, Value: fmt.Sprint(minutes)},
	}, nil
}

// URLBlocklistSettings maps web.block params to the ADMX-ingested Edge policy.
func URLBlocklistSettings(params map[string]any) ([]Setting, error) {
	domains := asStrings(params["domains"])
	if len(domains) == 0 {
		return nil, fmt.Errorf("web.block: domains is required")
	}
	var data strings.Builder
	data.WriteString("<enabled/>")
	for i, d := range domains {
		fmt.Fprintf(&data, "<data id=\"URLBlocklistDesc\" value=\"%d&#xF000;%s\"/>", i+1, escape(d))
	}
	return []Setting{{LocURI: policyRoot + "microsoft_edge~Policy~microsoft_edge/URLBlocklist", Format: FormatChr, Value: data.String()}}, nil
}

// RemoteLockSettings issues the RemoteLock exec.
func RemoteLockSettings(map[string]any) ([]Setting, error) {
	return []Setting{{LocURI: "./Vendor/MSFT/RemoteLock/Lock", Format: FormatNull}}, nil
}

// NewAdapters returns the Windows adapters bound to a LocalMDM.
func NewAdapters(mdm LocalMDM) []native.Adapter {
	lock := NewCSPAdapter("windows.csp.remotelock", "host.lock", mdm, RemoteLockSettings)
	lock.exec = true
	return []native.Adapter{
		NewCSPAdapter("windows.csp.removable", "storage.removable.block", mdm, RemovableStorageSettings),
		NewCSPAdapter("windows.csp.firewall", "firewall.enable", mdm, FirewallSettings),
		NewCSPAdapter("windows.csp.screenlock", "screen.lock.enforce", mdm, ScreenLockSettings),
		NewCSPAdapter("windows.csp.urlblocklist", "web.block", mdm, URLBlocklistSettings),
		lock,
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
