// Package bridge connects the native adapters to devices through the
// substrate when no ADM agent extension is present on the device yet.
//
// Windows: CSP reads go through fleetd's mdm_bridge osquery table as a live
// query (ApplyLocalManagementSyncML on the device, answered in seconds);
// CSP writes are enqueued as MDM commands through Fleet and delivered on the
// device's next OMA-DM session. Apple: declarations are delivered as Fleet
// profiles. With the agent extension both become local, millisecond calls.
package bridge

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/fleetdm/fleet/v4/adm/intent"
	"github.com/fleetdm/fleet/v4/adm/native"
	"github.com/fleetdm/fleet/v4/adm/native/darwin"
	"github.com/fleetdm/fleet/v4/adm/substrate"
)

// WindowsMDM implements windows.LocalMDM over a substrate.
type WindowsMDM struct {
	Substrate substrate.Substrate
}

// NewWindowsMDM returns a bridge.
func NewWindowsMDM(s substrate.Substrate) *WindowsMDM { return &WindowsMDM{Substrate: s} }

// Apply routes reads to mdm_bridge and writes to the MDM command queue.
func (w *WindowsMDM) Apply(ctx context.Context, t native.Target, syncBody string) (string, error) {
	if isReadOnly(syncBody) {
		sql := "SELECT raw_mdm_command_output FROM mdm_bridge WHERE mdm_command_input = '" + strings.ReplaceAll(syncBody, "'", "''") + "'"
		res, err := w.Substrate.LiveQuery(ctx, sql, []uint{t.HostID})
		if err != nil {
			return "", fmt.Errorf("mdm_bridge query: %w", err)
		}
		if e := res.Errors[t.HostID]; e != "" {
			return "", fmt.Errorf("mdm_bridge on host %d: %s", t.HostID, e)
		}
		rows := res.RowsFor(t.HostID)
		if len(rows) == 0 {
			return "", fmt.Errorf("mdm_bridge on host %d: no answer (offline?)", t.HostID)
		}
		return rows[0]["raw_mdm_command_output"], nil
	}
	if t.UUID == "" {
		return "", fmt.Errorf("host %d has no UUID; cannot enqueue an MDM command", t.HostID)
	}
	if err := w.Substrate.RunCommand(ctx, substrate.Command{Platform: intent.PlatformWindows, Raw: []byte(syncBody), HostUUIDs: []string{t.UUID}}); err != nil {
		return "", fmt.Errorf("enqueue MDM command: %w", err)
	}
	// The command is queued; report every command as accepted (202) so the
	// adapter proceeds to verification, which is what proves the change.
	return queuedResponse(syncBody), nil
}

func isReadOnly(body string) bool {
	lower := strings.ToLower(body)
	for _, v := range []string{"<replace>", "<add>", "<exec>", "<delete>", "<atomic>"} {
		if strings.Contains(lower, v) {
			return false
		}
	}
	return strings.Contains(lower, "<get>")
}

func queuedResponse(body string) string {
	n := strings.Count(body, "<CmdID>")
	var b strings.Builder
	b.WriteString("<SyncML xmlns=\"SYNCML:SYNCML1.2\"><SyncBody>")
	for i := 1; i <= n; i++ {
		fmt.Fprintf(&b, "<Status><CmdRef>%d</CmdRef><Data>202</Data></Status>", i)
	}
	b.WriteString("</SyncBody></SyncML>")
	return b.String()
}

// ApplePusher implements darwin.Pusher over a substrate by delivering
// declarations as Fleet profiles (Fleet accepts DDM JSON declarations on its
// profiles endpoint). Scope is the fleet and labels given; per-host targeting
// is a substrate roadmap item.
type ApplePusher struct {
	Substrate substrate.Substrate
	Fleet     string
	Labels    []string
}

// NewApplePusher returns a pusher scoped to a fleet and labels.
func NewApplePusher(s substrate.Substrate, fleet string, labels []string) *ApplePusher {
	return &ApplePusher{Substrate: s, Fleet: fleet, Labels: labels}
}

func (p *ApplePusher) PushDeclaration(ctx context.Context, t native.Target, d darwin.Declaration) error {
	b, err := json.Marshal(d)
	if err != nil {
		return err
	}
	return p.Substrate.AddProfile(ctx, substrate.Profile{Name: d.Identifier, Platform: t.Platform, Contents: b, Fleet: p.Fleet, Labels: p.Labels})
}

func (p *ApplePusher) RemoveDeclaration(ctx context.Context, t native.Target, identifier string) error {
	return fmt.Errorf("remove declaration %s: %w (revert the GitOps change instead)", identifier, substrate.ErrUnsupported)
}

// DeclarationStatus is unknown until the substrate exposes per-host
// declaration status; the executor then relies on the verification query.
func (p *ApplePusher) DeclarationStatus(ctx context.Context, t native.Target, identifier string) (string, error) {
	return "unknown", nil
}
