// Package intent defines the Intent IR: the platform-agnostic, declarative
// description of a desired fleet state that ADM compiles, risk-scores,
// executes and continuously reconciles.
//
// An Intent is the unit of management in ADM. Where a traditional MDM manages
// profiles, scripts and policies, ADM manages intents: "laptops stay encrypted",
// "no removable storage on finance machines", "Chrome is never more than two
// versions behind". Intents are persistent, versioned and re-evaluated at event
// speed (see package events), and every change they cause is written back to
// GitOps and to the outcome ledger (see package learn).
package intent

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Platform identifies an operating system family that ADM can act on.
type Platform string

const (
	PlatformDarwin   Platform = "darwin"
	PlatformWindows  Platform = "windows"
	PlatformLinux    Platform = "linux"
	PlatformIOS      Platform = "ios"
	PlatformIPadOS   Platform = "ipados"
	PlatformTVOS     Platform = "tvos"
	PlatformAndroid  Platform = "android"
	PlatformChromeOS Platform = "chromeos"
)

// AllPlatforms lists every platform ADM knows about, in a stable order.
var AllPlatforms = []Platform{
	PlatformDarwin, PlatformWindows, PlatformLinux,
	PlatformIOS, PlatformIPadOS, PlatformTVOS, PlatformAndroid, PlatformChromeOS,
}

// IsValid reports whether p is a known platform.
func (p Platform) IsValid() bool {
	for _, k := range AllPlatforms {
		if k == p {
			return true
		}
	}
	return false
}

// IsMobile reports whether the platform is managed purely through an MDM
// protocol (no agent, no shell).
func (p Platform) IsMobile() bool {
	switch p {
	case PlatformIOS, PlatformIPadOS, PlatformTVOS, PlatformAndroid:
		return true
	}
	return false
}

// Scope selects which devices an intent applies to. Selectors compose with AND
// semantics; an empty scope means "every device in the tenant" and is rejected
// by Validate unless AllowFleetWide is set on the intent.
type Scope struct {
	// Fleets are Fleet "teams" (the Fleet product calls them fleets).
	Fleets []string `json:"fleets,omitempty"`
	// Labels are Fleet label names (dynamic or manual).
	Labels []string `json:"labels,omitempty"`
	// ExcludeLabels removes hosts carrying any of these labels.
	ExcludeLabels []string `json:"exclude_labels,omitempty"`
	// Platforms restricts the intent to these OS families.
	Platforms []Platform `json:"platforms,omitempty"`
	// Query is an optional osquery SQL predicate evaluated live to refine
	// membership (for example "SELECT 1 FROM users WHERE username = 'ci'").
	Query string `json:"query,omitempty"`
	// HostIDs pins the intent to explicit Fleet host IDs.
	HostIDs []uint `json:"host_ids,omitempty"`
}

// IsEmpty reports whether the scope has no selector at all.
func (s Scope) IsEmpty() bool {
	return len(s.Fleets) == 0 && len(s.Labels) == 0 && len(s.Platforms) == 0 &&
		s.Query == "" && len(s.HostIDs) == 0
}

// Predicate is one desired-state clause: a capability from the capability
// catalog plus its parameters. The catalog decides how the predicate is
// realised on each platform (see package capability and package native).
type Predicate struct {
	Capability string         `json:"capability"`
	Params     map[string]any `json:"params,omitempty"`
	// Rationale is the human-readable "why" carried through to approvals,
	// GitOps commit messages and the outcome ledger.
	Rationale string `json:"rationale,omitempty"`
}

// Autonomy caps how autonomously ADM may act on this intent. The risk engine
// may always choose a more conservative tier, never a more permissive one.
type Autonomy string

const (
	// AutonomyObserve: never act, only report drift.
	AutonomyObserve Autonomy = "observe"
	// AutonomyAuto: execute without a human when the risk engine allows it.
	AutonomyAuto Autonomy = "auto"
	// AutonomyCanary: execute automatically on a canary wave, pause for
	// verification, then continue.
	AutonomyCanary Autonomy = "canary"
	// AutonomyApprove: always wait for a human approval before executing.
	AutonomyApprove Autonomy = "approve"
	// AutonomyCAB: require a change-advisory approval (ticket) before executing.
	AutonomyCAB Autonomy = "cab"
)

// UserImpact describes what the end user experiences when a change lands.
type UserImpact string

const (
	ImpactNone    UserImpact = "none"
	ImpactPrompt  UserImpact = "prompt"
	ImpactLogout  UserImpact = "logout"
	ImpactRestart UserImpact = "restart"
)

// Window is a recurring maintenance window expressed in the device's local time.
type Window struct {
	// Days uses time.Weekday names ("Monday"). Empty means every day.
	Days []string `json:"days,omitempty"`
	// Start and End are "HH:MM" 24h local times.
	Start string `json:"start"`
	End   string `json:"end"`
}

// Constraints bound how and when an intent may be realised.
type Constraints struct {
	MaxAutonomy       Autonomy   `json:"max_autonomy,omitempty"`
	MaintenanceWindow *Window    `json:"maintenance_window,omitempty"`
	CanaryPercent     int        `json:"canary_percent,omitempty"`
	MaxWaveHosts      int        `json:"max_wave_hosts,omitempty"`
	MaxUserImpact     UserImpact `json:"max_user_impact,omitempty"`
	Deadline          *time.Time `json:"deadline,omitempty"`
	// AllowFleetWide must be true for an intent whose Scope is empty.
	AllowFleetWide bool `json:"allow_fleet_wide,omitempty"`
}

// Verification is an observable check that must hold after the intent is
// realised. It is expressed as osquery SQL so the same primitive that made the
// change also proves it, on the same device.
type Verification struct {
	Platform Platform `json:"platform,omitempty"`
	Query    string   `json:"query"`
	// Expect is "rows>0" (the query must return rows) or "rows==0".
	Expect string `json:"expect"`
	// Within bounds how long ADM waits for the verification to hold.
	Within time.Duration `json:"within,omitempty"`
}

// State is the lifecycle state of an intent.
type State string

const (
	StateDraft    State = "draft"
	StateCompiled State = "compiled"
	StateActive   State = "active"
	StatePaused   State = "paused"
	StateRetired  State = "retired"
)

// Source records where an intent came from.
type Source string

const (
	SourceNaturalLanguage Source = "nl"
	SourceGitOps          Source = "gitops"
	SourceAPI             Source = "api"
	SourceAgent           Source = "agent"
)

// Provenance records how an intent was produced so that every action can be
// traced back to a prompt, a model and a compiler version.
type Provenance struct {
	Compiler   string  `json:"compiler,omitempty"`
	Model      string  `json:"model,omitempty"`
	PromptHash string  `json:"prompt_hash,omitempty"`
	Confidence float64 `json:"confidence,omitempty"`
}

// Intent is the persistent, versioned statement of desired state.
type Intent struct {
	ID           string         `json:"id"`
	Text         string         `json:"text"`
	Author       string         `json:"author"`
	Source       Source         `json:"source"`
	Scope        Scope          `json:"scope"`
	Desired      []Predicate    `json:"desired"`
	Constraints  Constraints    `json:"constraints"`
	Verification []Verification `json:"verification,omitempty"`
	State        State          `json:"state"`
	Version      int            `json:"version"`
	Provenance   Provenance     `json:"provenance"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
	ExpiresAt    *time.Time     `json:"expires_at,omitempty"`
}

// ErrInvalid wraps every validation failure.
var ErrInvalid = errors.New("invalid intent")

// Validate performs structural validation. Capability names are validated
// against the catalog by the planner, not here, to keep this package free of
// dependencies.
func (in *Intent) Validate() error {
	var problems []string
	if strings.TrimSpace(in.Text) == "" {
		problems = append(problems, "text is required")
	}
	if len(in.Desired) == 0 {
		problems = append(problems, "at least one desired predicate is required")
	}
	for i, p := range in.Desired {
		if strings.TrimSpace(p.Capability) == "" {
			problems = append(problems, fmt.Sprintf("desired[%d]: capability is required", i))
		}
	}
	if in.Scope.IsEmpty() && !in.Constraints.AllowFleetWide {
		problems = append(problems, "scope is empty; set constraints.allow_fleet_wide to target every device")
	}
	for _, p := range in.Scope.Platforms {
		if !p.IsValid() {
			problems = append(problems, fmt.Sprintf("unknown platform %q", p))
		}
	}
	switch in.Constraints.MaxAutonomy {
	case "", AutonomyObserve, AutonomyAuto, AutonomyCanary, AutonomyApprove, AutonomyCAB:
	default:
		problems = append(problems, fmt.Sprintf("unknown autonomy %q", in.Constraints.MaxAutonomy))
	}
	if in.Constraints.CanaryPercent < 0 || in.Constraints.CanaryPercent > 100 {
		problems = append(problems, "canary_percent must be between 0 and 100")
	}
	for i, v := range in.Verification {
		if strings.TrimSpace(v.Query) == "" {
			problems = append(problems, fmt.Sprintf("verification[%d]: query is required", i))
		}
		if v.Expect != "rows>0" && v.Expect != "rows==0" {
			problems = append(problems, fmt.Sprintf("verification[%d]: expect must be rows>0 or rows==0", i))
		}
	}
	if len(problems) > 0 {
		return fmt.Errorf("%w: %s", ErrInvalid, strings.Join(problems, "; "))
	}
	return nil
}

// Fingerprint returns a stable hash of the intent's semantic content (scope,
// desired predicates and constraints). Two intents with the same fingerprint
// realise the same state; the risk engine keys its adaptive "leash" on it.
func (in *Intent) Fingerprint() string {
	type semantic struct {
		Scope       Scope       `json:"scope"`
		Desired     []Predicate `json:"desired"`
		Constraints Constraints `json:"constraints"`
	}
	b, _ := json.Marshal(semantic{in.Scope, in.Desired, in.Constraints})
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:8])
}

// PatternKey returns the key used for adaptive autonomy ("the leash"): the set
// of capabilities and platforms the intent touches, independent of scope size.
func (in *Intent) PatternKey() string {
	caps := make([]string, 0, len(in.Desired))
	for _, p := range in.Desired {
		caps = append(caps, p.Capability)
	}
	plats := make([]string, 0, len(in.Scope.Platforms))
	for _, p := range in.Scope.Platforms {
		plats = append(plats, string(p))
	}
	if len(plats) == 0 {
		plats = []string{"any"}
	}
	return strings.Join(caps, "+") + "@" + strings.Join(plats, ",")
}

// EffectivePlatforms returns the platforms the intent targets, defaulting to
// every platform when the scope does not restrict them.
func (in *Intent) EffectivePlatforms() []Platform {
	if len(in.Scope.Platforms) > 0 {
		return in.Scope.Platforms
	}
	return AllPlatforms
}

// PromptHash hashes the natural-language text that produced the intent.
func PromptHash(text string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(text)))
	return hex.EncodeToString(sum[:8])
}
