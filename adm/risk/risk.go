// Package risk is ADM's autonomy engine. It turns a proposed change into a
// numeric risk score, maps the score to an autonomy tier, enforces hard
// invariants that no score can override, and adapts the tier over time as a
// pattern of change earns trust ("the leash").
//
// The engine is deliberately deterministic: the language model proposes,
// this package decides how much autonomy the proposal gets.
package risk

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/fleetdm/fleet/v4/adm/intent"
)

// Tier is the autonomy level granted to a plan.
type Tier int

const (
	// TierObserve reports drift but never acts.
	TierObserve Tier = iota
	// TierAuto executes without a human.
	TierAuto
	// TierCanary executes on a canary wave, verifies, then continues.
	TierCanary
	// TierApprove waits for a human approval.
	TierApprove
	// TierCAB waits for a change-advisory approval recorded in a ticket.
	TierCAB
)

func (t Tier) String() string {
	switch t {
	case TierObserve:
		return "observe"
	case TierAuto:
		return "auto"
	case TierCanary:
		return "canary"
	case TierApprove:
		return "approve"
	case TierCAB:
		return "cab"
	}
	return fmt.Sprintf("tier(%d)", int(t))
}

// MarshalText implements encoding.TextMarshaler.
func (t Tier) MarshalText() ([]byte, error) { return []byte(t.String()), nil }

// UnmarshalText implements encoding.TextUnmarshaler.
func (t *Tier) UnmarshalText(b []byte) error {
	switch string(b) {
	case "observe":
		*t = TierObserve
	case "auto":
		*t = TierAuto
	case "canary":
		*t = TierCanary
	case "approve":
		*t = TierApprove
	case "cab":
		*t = TierCAB
	default:
		return fmt.Errorf("unknown tier %q", string(b))
	}
	return nil
}

// TierFromAutonomy maps an intent's autonomy cap to a tier. An empty cap
// means "no cap".
func TierFromAutonomy(a intent.Autonomy) (Tier, bool) {
	switch a {
	case intent.AutonomyObserve:
		return TierObserve, true
	case intent.AutonomyAuto:
		return TierAuto, true
	case intent.AutonomyCanary:
		return TierCanary, true
	case intent.AutonomyApprove:
		return TierApprove, true
	case intent.AutonomyCAB:
		return TierCAB, true
	}
	return TierAuto, false
}

// Factors are the inputs to the score.
type Factors struct {
	// BlastRadius is the number of hosts the plan touches.
	BlastRadius int
	// FleetSize is the number of hosts in scope of the tenant, for the fraction.
	FleetSize int
	// BaseRisk is the highest intrinsic risk among the capabilities used (0..40).
	BaseRisk    int
	Reversible  bool
	Destructive bool
	UserImpact  intent.UserImpact
	Privilege   string
	// Confidence is the compiler's confidence in the intent (0..1).
	Confidence float64
	// Novelty is 1 for a never-seen pattern and decays with accepted history.
	Novelty float64
	// Platforms is the number of distinct platforms touched.
	Platforms int
	// UsesScript is true when any step falls back to a shell script.
	UsesScript bool
	// InMaintenanceWindow is true when execution is scheduled inside a window.
	InMaintenanceWindow bool
}

// Assessment is the engine's decision.
type Assessment struct {
	Score   int      `json:"score"`
	Tier    Tier     `json:"tier"`
	Factors Factors  `json:"factors"`
	Reasons []string `json:"reasons"`
	// Floor is the minimum tier imposed by policy for the capabilities used.
	Floor Tier `json:"floor"`
	// LeashRelaxation is how many tiers the adaptive leash lowered the result.
	LeashRelaxation int `json:"leash_relaxation"`
}

// Policy configures thresholds, caps and invariants.
type Policy struct {
	// Score thresholds: <= AutoMax -> auto, <= CanaryMax -> canary,
	// <= ApproveMax -> approve, otherwise CAB.
	AutoMax    int
	CanaryMax  int
	ApproveMax int
	// MaxAutoHosts and MaxAutoFraction cap the blast radius of a TierAuto plan.
	MaxAutoHosts    int
	MaxAutoFraction float64
	// Floors are minimum tiers per capability name (prefix match allowed with
	// a trailing "*").
	Floors map[string]Tier
	// LeashMaxRelaxation bounds how far the adaptive leash may lower a tier.
	LeashMaxRelaxation int
	// AcceptancesPerStep is how many verified, accepted executions of a
	// pattern earn one tier of relaxation.
	AcceptancesPerStep int
	// IncidentCooldown is how long an incident on a pattern suspends its leash.
	IncidentCooldown time.Duration
	Invariants       []Invariant
}

// DefaultPolicy returns conservative defaults suitable for a new tenant.
func DefaultPolicy() Policy {
	return Policy{
		AutoMax:         25,
		CanaryMax:       45,
		ApproveMax:      70,
		MaxAutoHosts:    50,
		MaxAutoFraction: 0.05,
		Floors: map[string]Tier{
			"host.wipe":         TierCAB,
			"host.lock":         TierApprove,
			"identity.*":        TierApprove,
			"compliance.signal": TierCanary,
			"os.update.enforce": TierCanary,
			"script.run":        TierApprove,
			"enrollment.*":      TierAuto,
		},
		LeashMaxRelaxation: 2,
		AcceptancesPerStep: 5,
		IncidentCooldown:   30 * 24 * time.Hour,
		Invariants:         DefaultInvariants(),
	}
}

// FloorFor returns the policy floor for a capability.
func (p Policy) FloorFor(capability string) Tier {
	floor := TierObserve
	for pattern, tier := range p.Floors {
		if matchPattern(pattern, capability) && tier > floor {
			floor = tier
		}
	}
	return floor
}

func matchPattern(pattern, name string) bool {
	if strings.HasSuffix(pattern, "*") {
		return strings.HasPrefix(name, strings.TrimSuffix(pattern, "*"))
	}
	return pattern == name
}

// Score computes the 0..100 risk score for the factors.
func Score(f Factors) (int, []string) {
	var reasons []string
	score := float64(clamp(f.BaseRisk, 0, 40))
	reasons = append(reasons, fmt.Sprintf("base risk %d", f.BaseRisk))

	// Blast radius: 0..20, log-scaled on fraction of fleet and absolute count.
	if f.BlastRadius > 0 {
		frac := 1.0
		if f.FleetSize > 0 {
			frac = math.Min(1, float64(f.BlastRadius)/float64(f.FleetSize))
		}
		radius := 10*frac + 10*math.Min(1, math.Log10(float64(f.BlastRadius)+1)/4)
		score += radius
		reasons = append(reasons, fmt.Sprintf("blast radius %d hosts (%.0f%% of fleet) +%.0f", f.BlastRadius, frac*100, radius))
	}
	if !f.Reversible {
		score += 15
		reasons = append(reasons, "irreversible +15")
	}
	if f.Destructive {
		score += 30
		reasons = append(reasons, "destructive +30")
	}
	switch f.UserImpact {
	case intent.ImpactPrompt:
		score += 5
		reasons = append(reasons, "prompts the user +5")
	case intent.ImpactLogout:
		score += 10
		reasons = append(reasons, "logs the user out +10")
	case intent.ImpactRestart:
		score += 15
		reasons = append(reasons, "restarts the device +15")
	}
	switch f.Privilege {
	case "system", "mdm":
		score += 5
		reasons = append(reasons, "privileged change +5")
	}
	if f.Confidence > 0 && f.Confidence < 1 {
		pen := (1 - f.Confidence) * 15
		score += pen
		reasons = append(reasons, fmt.Sprintf("compiler confidence %.2f +%.0f", f.Confidence, pen))
	}
	if f.Novelty > 0 {
		pen := f.Novelty * 10
		score += pen
		reasons = append(reasons, fmt.Sprintf("pattern novelty %.2f +%.0f", f.Novelty, pen))
	}
	if f.Platforms > 1 {
		score += float64(f.Platforms-1) * 2
		reasons = append(reasons, fmt.Sprintf("%d platforms +%d", f.Platforms, (f.Platforms-1)*2))
	}
	if f.UsesScript {
		score += 10
		reasons = append(reasons, "falls back to a script +10")
	}
	if f.InMaintenanceWindow {
		score -= 5
		reasons = append(reasons, "inside maintenance window -5")
	}
	return clamp(int(math.Round(score)), 0, 100), reasons
}

// LeashState is the adaptive-autonomy record for one pattern of change
// (see intent.PatternKey). It is persisted in the outcome ledger.
type LeashState struct {
	Pattern      string    `json:"pattern"`
	Accepted     int       `json:"accepted"`
	Rejected     int       `json:"rejected"`
	Verified     int       `json:"verified"`
	Failed       int       `json:"failed"`
	Incidents    int       `json:"incidents"`
	LastIncident time.Time `json:"last_incident,omitempty"`
	LastSeen     time.Time `json:"last_seen,omitempty"`
}

// Novelty returns 1 for an unseen pattern and decays towards 0 as verified,
// accepted executions accumulate.
func (l LeashState) Novelty() float64 {
	if l.Verified+l.Accepted == 0 {
		return 1
	}
	return 1 / (1 + float64(l.Verified+l.Accepted)/3)
}

// Relaxation returns how many tiers the leash may lower a plan for this
// pattern under the policy. A recent incident or a poor verification record
// suspends relaxation entirely.
func (l LeashState) Relaxation(p Policy, now time.Time) int {
	if p.AcceptancesPerStep <= 0 {
		return 0
	}
	if l.Incidents > 0 && now.Sub(l.LastIncident) < p.IncidentCooldown {
		return 0
	}
	if l.Failed > 0 && l.Verified < 3*l.Failed {
		return 0
	}
	earned := l.Accepted / p.AcceptancesPerStep
	if earned > p.LeashMaxRelaxation {
		earned = p.LeashMaxRelaxation
	}
	return earned
}

// Input bundles everything Assess needs.
type Input struct {
	Factors      Factors
	Capabilities []string
	// Cap is the intent's maximum autonomy, if any.
	Cap *Tier
	// Leash is the pattern's adaptive state; nil means unseen.
	Leash *LeashState
	Now   time.Time
}

// Assess computes the score, applies floors, blast-radius caps, the intent's
// own cap and the adaptive leash, and returns the resulting tier.
func (p Policy) Assess(in Input) Assessment {
	f := in.Factors
	if in.Leash != nil && f.Novelty == 0 {
		f.Novelty = in.Leash.Novelty()
	}
	score, reasons := Score(f)

	tier := TierAuto
	switch {
	case score > p.ApproveMax:
		tier = TierCAB
	case score > p.CanaryMax:
		tier = TierApprove
	case score > p.AutoMax:
		tier = TierCanary
	}
	reasons = append(reasons, fmt.Sprintf("score %d -> %s", score, tier))

	// Blast-radius caps: a TierAuto plan may not touch more than the cap.
	if tier == TierAuto && f.BlastRadius > 0 {
		frac := 1.0
		if f.FleetSize > 0 {
			frac = float64(f.BlastRadius) / float64(f.FleetSize)
		}
		if f.BlastRadius > p.MaxAutoHosts || frac > p.MaxAutoFraction {
			tier = TierCanary
			reasons = append(reasons, fmt.Sprintf("blast radius exceeds auto cap (%d hosts / %.0f%%) -> canary", p.MaxAutoHosts, p.MaxAutoFraction*100))
		}
	}

	// Adaptive leash: lower the tier for proven patterns, never below auto and
	// never below the policy floor, and never for destructive changes.
	relax := 0
	if in.Leash != nil && !f.Destructive {
		relax = in.Leash.Relaxation(p, in.Now)
		if relax > 0 {
			lowered := tier - Tier(relax)
			if lowered < TierAuto {
				lowered = TierAuto
			}
			if lowered < tier {
				reasons = append(reasons, fmt.Sprintf("leash: %d accepted, %d verified, %d incidents -> relax %d tier(s)", in.Leash.Accepted, in.Leash.Verified, in.Leash.Incidents, int(tier-lowered)))
				relax = int(tier - lowered)
				tier = lowered
			} else {
				relax = 0
			}
		}
	}

	// Policy floors always win.
	floor := TierObserve
	for _, c := range in.Capabilities {
		if fl := p.FloorFor(c); fl > floor {
			floor = fl
		}
	}
	if tier < floor {
		reasons = append(reasons, fmt.Sprintf("policy floor for %s -> %s", strings.Join(in.Capabilities, ","), floor))
		tier = floor
	}

	// The intent's own cap can only make things more conservative.
	if in.Cap != nil && *in.Cap > tier {
		reasons = append(reasons, fmt.Sprintf("intent cap -> %s", *in.Cap))
		tier = *in.Cap
	}
	if in.Cap != nil && *in.Cap == TierObserve {
		tier = TierObserve
	}

	return Assessment{Score: score, Tier: tier, Factors: f, Reasons: reasons, Floor: floor, LeashRelaxation: relax}
}

// Action is the minimal description of a planned change that invariants
// inspect. It is kept independent of package plan to avoid an import cycle.
type Action struct {
	Capability string
	Platform   intent.Platform
	Params     map[string]any
	Targets    int
	Tier       Tier
}

// Violation is a broken invariant.
type Violation struct {
	Invariant string `json:"invariant"`
	Message   string `json:"message"`
}

func (v Violation) Error() string { return v.Invariant + ": " + v.Message }

// Invariant is a hard rule evaluated on every action regardless of score.
type Invariant struct {
	Name  string
	Check func(a Action) *Violation
}

// Evaluate runs every invariant over every action and returns violations,
// sorted for stable output.
func (p Policy) Evaluate(actions []Action) []Violation {
	var out []Violation
	for _, a := range actions {
		for _, inv := range p.Invariants {
			if v := inv.Check(a); v != nil {
				out = append(out, *v)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Invariant != out[j].Invariant {
			return out[i].Invariant < out[j].Invariant
		}
		return out[i].Message < out[j].Message
	})
	return out
}

// protectedAccounts are never created, modified or removed by ADM.
var protectedAccounts = map[string]bool{"root": true, "administrator": true, "breakglass": true, "break-glass": true}

// DefaultInvariants returns the rules that hold in every tenant.
func DefaultInvariants() []Invariant {
	return []Invariant{
		{
			Name: "never-disable-encryption",
			Check: func(a Action) *Violation {
				if strings.HasPrefix(a.Capability, "disk.encryption") {
					if v, ok := a.Params["enabled"].(bool); ok && !v {
						return &Violation{"never-disable-encryption", "an intent may not turn disk encryption off"}
					}
				}
				return nil
			},
		},
		{
			Name: "wipe-requires-cab",
			Check: func(a Action) *Violation {
				if a.Capability == "host.wipe" && a.Tier < TierCAB {
					return &Violation{"wipe-requires-cab", "host.wipe must run at the CAB tier"}
				}
				return nil
			},
		},
		{
			Name: "wipe-single-host",
			Check: func(a Action) *Violation {
				if a.Capability == "host.wipe" && a.Targets > 1 {
					return &Violation{"wipe-single-host", fmt.Sprintf("host.wipe may target one host per plan, got %d", a.Targets)}
				}
				return nil
			},
		},
		{
			Name: "never-remove-agent",
			Check: func(a Action) *Violation {
				if a.Capability == "software.uninstall" || a.Capability == "script.run" {
					s := strings.ToLower(fmt.Sprint(a.Params["title"], a.Params["script"]))
					for _, agent := range []string{"fleetd", "orbit", "osquery", "adm-agent"} {
						if strings.Contains(s, agent) {
							return &Violation{"never-remove-agent", "an intent may not remove or stop the management agent (" + agent + ")"}
						}
					}
				}
				return nil
			},
		},
		{
			Name: "protected-accounts",
			Check: func(a Action) *Violation {
				if strings.HasPrefix(a.Capability, "identity.") {
					if u, ok := a.Params["username"].(string); ok && protectedAccounts[strings.ToLower(u)] {
						return &Violation{"protected-accounts", "account " + u + " is protected"}
					}
				}
				return nil
			},
		},
		{
			Name: "no-fleet-wide-scripts",
			Check: func(a Action) *Violation {
				if a.Capability == "script.run" && a.Targets > 100 && a.Tier < TierCAB {
					return &Violation{"no-fleet-wide-scripts", "scripts on more than 100 hosts require the CAB tier"}
				}
				return nil
			},
		},
	}
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
