// Package substrate abstracts the device-management substrate ADM drives.
// Fleet is the primary substrate: osquery inventory and live query, the MDM
// protocols for Apple, Windows and Android, scripts as a last resort, and
// the agent notification channel. Microsoft Graph is a second substrate for
// Windows Autopilot (see package enroll). An in-memory substrate backs tests
// and the offline demo.
package substrate

import (
	"context"
	"errors"

	"github.com/fleetdm/fleet/v4/adm/device"
	"github.com/fleetdm/fleet/v4/adm/intent"
)

// ErrUnsupported is returned by substrates that cannot perform an operation.
var ErrUnsupported = errors.New("operation not supported by this substrate")

// QueryResult is the outcome of a live query across hosts.
type QueryResult struct {
	Rows      map[uint][]map[string]string `json:"rows"`
	Errors    map[uint]string              `json:"errors,omitempty"`
	Targeted  int                          `json:"targeted"`
	Responded int                          `json:"responded"`
}

// RowsFor returns the rows a host returned.
func (q QueryResult) RowsFor(hostID uint) []map[string]string { return q.Rows[hostID] }

// Profile is an MDM configuration profile or declaration to deliver.
type Profile struct {
	Name     string          `json:"name"`
	Platform intent.Platform `json:"platform"`
	// Contents is the raw profile (mobileconfig XML, SyncML or DDM JSON).
	Contents []byte `json:"contents"`
	// Fleet scopes the profile to a Fleet (team); empty means the default fleet.
	Fleet string `json:"fleet,omitempty"`
	// Labels scope the profile to hosts carrying all of these labels.
	Labels []string `json:"labels,omitempty"`
}

// Command is an MDM command to enqueue.
type Command struct {
	Platform intent.Platform `json:"platform"`
	// Raw is the command body: an Apple plist or a Windows SyncML fragment.
	Raw       []byte   `json:"raw"`
	HostUUIDs []string `json:"host_uuids"`
}

// Policy is a substrate policy (a Fleet policy) used to make verification
// persistent and visible.
type Policy struct {
	Name        string          `json:"name"`
	Query       string          `json:"query"`
	Description string          `json:"description,omitempty"`
	Resolution  string          `json:"resolution,omitempty"`
	Platform    intent.Platform `json:"platform,omitempty"`
	Fleet       string          `json:"fleet,omitempty"`
}

// ScriptResult is the outcome of the script fallback.
type ScriptResult struct {
	ExitCode int    `json:"exit_code"`
	Output   string `json:"output"`
}

// Substrate is what ADM needs from the platform underneath it.
type Substrate interface {
	Name() string
	// ListHosts returns hosts matching the locally evaluable parts of scope.
	ListHosts(ctx context.Context, scope intent.Scope) ([]device.Host, error)
	// CountHosts returns the tenant's total host count.
	CountHosts(ctx context.Context) (int, error)
	// LiveQuery runs osquery SQL on the hosts and waits for answers.
	LiveQuery(ctx context.Context, sql string, hostIDs []uint) (QueryResult, error)
	// AddProfile delivers an MDM profile or declaration.
	AddProfile(ctx context.Context, p Profile) error
	// RunCommand enqueues an MDM command.
	RunCommand(ctx context.Context, c Command) error
	// UpsertPolicy creates or updates a policy.
	UpsertPolicy(ctx context.Context, p Policy) error
	// RunScript runs a script on one host and waits for the result (fallback).
	RunScript(ctx context.Context, hostID uint, script string) (ScriptResult, error)
	// Nudge asks the agents on the hosts to check in now.
	Nudge(ctx context.Context, hostIDs []uint) error
}
