// Package plan compiles an intent into an executable, risk-scored,
// reversible plan: native steps per platform, verification checks, rollback
// steps, rollout waves and the persistent GitOps rendering.
package plan

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/fleetdm/fleet/v4/adm/capability"
	"github.com/fleetdm/fleet/v4/adm/device"
	"github.com/fleetdm/fleet/v4/adm/intent"
	"github.com/fleetdm/fleet/v4/adm/risk"
)

// Status is the lifecycle of a plan.
type Status string

const (
	StatusProposed   Status = "proposed"
	StatusBlocked    Status = "blocked" // an invariant is violated
	StatusApproved   Status = "approved"
	StatusRejected   Status = "rejected"
	StatusExecuting  Status = "executing"
	StatusVerified   Status = "verified"
	StatusFailed     Status = "failed"
	StatusRolledBack Status = "rolled_back"
)

// Step is one native change on one platform for a set of hosts.
type Step struct {
	ID         string               `json:"id"`
	Capability string               `json:"capability"`
	Platform   intent.Platform      `json:"platform"`
	Mechanism  capability.Mechanism `json:"mechanism"`
	Native     string               `json:"native"`
	Adapter    string               `json:"adapter,omitempty"`
	Params     map[string]any       `json:"params,omitempty"`
	Targets    []uint               `json:"targets"`
	Reversible bool                 `json:"reversible"`
	Latency    capability.Latency   `json:"latency"`
	// Verify is the rendered verification for this step, if the catalog has one.
	Verify *capability.Verify `json:"verify,omitempty"`
	// Revert marks a rollback step.
	Revert bool `json:"revert,omitempty"`
}

// Wave is one rollout batch. Waves execute in order; a wave that pauses for
// verification blocks the next until its verification holds.
type Wave struct {
	Index   int    `json:"index"`
	Name    string `json:"name"`
	Targets []uint `json:"targets"`
	Pause   bool   `json:"pause_for_verification"`
}

// Unsupported records a platform the intent could not be compiled for.
type Unsupported struct {
	Capability string          `json:"capability"`
	Platform   intent.Platform `json:"platform"`
	Hosts      int             `json:"hosts"`
	Reason     string          `json:"reason"`
}

// Approval records a human or CAB decision.
type Approval struct {
	By      string    `json:"by"`
	At      time.Time `json:"at"`
	Channel string    `json:"channel"`
	Note    string    `json:"note,omitempty"`
	Ticket  string    `json:"ticket,omitempty"`
}

// Plan is the compiled, scored, executable form of an intent.
type Plan struct {
	ID          string           `json:"id"`
	IntentID    string           `json:"intent_id"`
	Intent      intent.Intent    `json:"intent"`
	Steps       []Step           `json:"steps"`
	Rollback    []Step           `json:"rollback"`
	Waves       []Wave           `json:"waves"`
	Risk        risk.Assessment  `json:"risk"`
	Violations  []risk.Violation `json:"violations,omitempty"`
	Unsupported []Unsupported    `json:"unsupported,omitempty"`
	GitOps      string           `json:"gitops"`
	Status      Status           `json:"status"`
	Approval    *Approval        `json:"approval,omitempty"`
	CreatedAt   time.Time        `json:"created_at"`
	// Hosts is the total number of distinct targets.
	Hosts int `json:"hosts"`
}

// Capabilities lists the distinct capabilities the plan uses, sorted.
func (p *Plan) Capabilities() []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range p.Steps {
		if !seen[s.Capability] {
			seen[s.Capability] = true
			out = append(out, s.Capability)
		}
	}
	sort.Strings(out)
	return out
}

// Platforms lists the distinct platforms the plan touches, sorted.
func (p *Plan) Platforms() []intent.Platform {
	seen := map[intent.Platform]bool{}
	var out []intent.Platform
	for _, s := range p.Steps {
		if !seen[s.Platform] {
			seen[s.Platform] = true
			out = append(out, s.Platform)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// NeedsApproval reports whether the plan's tier requires a human before it runs.
func (p *Plan) NeedsApproval() bool {
	return p.Risk.Tier >= risk.TierApprove
}

// Executable reports whether the plan may run right now.
func (p *Plan) Executable() (bool, string) {
	switch {
	case len(p.Violations) > 0:
		return false, "plan violates an invariant"
	case p.Risk.Tier == risk.TierObserve:
		return false, "intent is observe-only"
	case len(p.Steps) == 0:
		return false, "plan has no steps"
	case p.NeedsApproval() && p.Approval == nil:
		return false, fmt.Sprintf("tier %s requires approval", p.Risk.Tier)
	case p.Risk.Tier == risk.TierCAB && (p.Approval == nil || p.Approval.Ticket == ""):
		return false, "CAB tier requires an approval with a change ticket"
	case p.Status == StatusRejected:
		return false, "plan was rejected"
	}
	return true, ""
}

// Planner compiles intents against a catalog and a risk policy.
type Planner struct {
	Catalog *capability.Catalog
	Policy  risk.Policy
	Now     func() time.Time
	NewID   func() string
}

// New returns a planner with defaults.
func New(cat *capability.Catalog, pol risk.Policy) *Planner {
	return &Planner{Catalog: cat, Policy: pol, Now: time.Now, NewID: NewPlanID}
}

// NewPlanID returns a time-prefixed, random plan ID that is unique across
// processes.
func NewPlanID() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return fmt.Sprintf("plan-%d-%s", time.Now().Unix(), hex.EncodeToString(b))
}

// Options tune a compilation.
type Options struct {
	// FleetSize is the tenant's total host count, for blast-radius fractions.
	FleetSize int
	// Leash is the adaptive-autonomy state for the intent's pattern.
	Leash *risk.LeashState
	// DefaultCanaryPercent applies when the intent does not set one.
	DefaultCanaryPercent int
	// DefaultWaveHosts applies when the intent does not set MaxWaveHosts.
	DefaultWaveHosts int
	// InMaintenanceWindow is true when the execution time falls in a window.
	InMaintenanceWindow bool
}

// Compile turns an intent plus the hosts it selects into a plan.
func (pl *Planner) Compile(in *intent.Intent, hosts []device.Host, opts Options) (*Plan, error) {
	if err := in.Validate(); err != nil {
		return nil, err
	}
	if opts.DefaultCanaryPercent <= 0 {
		opts.DefaultCanaryPercent = 10
	}
	if opts.DefaultWaveHosts <= 0 {
		opts.DefaultWaveHosts = 500
	}
	now := pl.Now()
	p := &Plan{ID: pl.NewID(), IntentID: in.ID, Intent: *in, Status: StatusProposed, CreatedAt: now}

	selected := device.Filter(hosts, in.Scope)
	byPlatform := device.GroupByPlatform(selected)
	platforms := in.EffectivePlatforms()

	var (
		baseRisk    int
		reversible  = true
		destructive bool
		impact      intent.UserImpact
		privilege   string
		usesScript  bool
		targets     = map[uint]bool{}
	)
	stepN := 0
	for _, pred := range in.Desired {
		cap, ok := pl.Catalog.Get(pred.Capability)
		if !ok {
			return nil, fmt.Errorf("unknown capability %q", pred.Capability)
		}
		if err := cap.ValidateParams(pred.Params); err != nil {
			return nil, err
		}
		if cap.BaseRisk > baseRisk {
			baseRisk = cap.BaseRisk
		}
		if !cap.Reversible {
			reversible = false
		}
		if cap.Destructive {
			destructive = true
		}
		impact = maxImpact(impact, cap.UserImpact)
		privilege = maxPrivilege(privilege, string(cap.Privilege))

		for _, platform := range platforms {
			ph := byPlatform[platform]
			if len(ph) == 0 {
				continue
			}
			binding, ok := cap.Bindings[platform]
			if !ok {
				p.Unsupported = append(p.Unsupported, Unsupported{Capability: cap.Name, Platform: platform, Hosts: len(ph), Reason: "no native binding for this platform yet"})
				continue
			}
			if binding.Mechanism == capability.MechScript {
				usesScript = true
			}
			ids := device.IDs(ph)
			for _, id := range ids {
				targets[id] = true
			}
			stepN++
			step := Step{
				ID:         fmt.Sprintf("%s-s%d", p.ID, stepN),
				Capability: cap.Name,
				Platform:   platform,
				Mechanism:  binding.Mechanism,
				Native:     binding.Native,
				Adapter:    binding.Adapter,
				Params:     cloneParams(pred.Params),
				Targets:    ids,
				Reversible: cap.Reversible,
				Latency:    binding.Latency,
			}
			if v, ok := cap.Verify[platform]; ok {
				rendered := capability.Verify{Query: RenderTemplate(v.Query, pred.Params), Expect: v.Expect}
				step.Verify = &rendered
			}
			p.Steps = append(p.Steps, step)
			if cap.Reversible && !cap.ReadOnly {
				rb := step
				rb.ID = step.ID + "-revert"
				rb.Revert = true
				p.Rollback = append(p.Rollback, rb)
			}
		}
	}
	// Rollback runs in reverse order.
	for i, j := 0, len(p.Rollback)-1; i < j; i, j = i+1, j-1 {
		p.Rollback[i], p.Rollback[j] = p.Rollback[j], p.Rollback[i]
	}
	p.Hosts = len(targets)

	// Risk.
	var capNames []string
	for _, c := range p.Capabilities() {
		capNames = append(capNames, c)
	}
	for _, pred := range in.Desired { // include capabilities with no steps so floors still apply
		if !contains(capNames, pred.Capability) {
			capNames = append(capNames, pred.Capability)
		}
	}
	sort.Strings(capNames)
	factors := risk.Factors{
		BlastRadius:         p.Hosts,
		FleetSize:           opts.FleetSize,
		BaseRisk:            baseRisk,
		Reversible:          reversible,
		Destructive:         destructive,
		UserImpact:          impact,
		Privilege:           privilege,
		Confidence:          in.Provenance.Confidence,
		Platforms:           len(p.Platforms()),
		UsesScript:          usesScript,
		InMaintenanceWindow: opts.InMaintenanceWindow,
	}
	var capTier *risk.Tier
	if t, ok := risk.TierFromAutonomy(in.Constraints.MaxAutonomy); ok {
		capTier = &t
	}
	p.Risk = pl.Policy.Assess(risk.Input{Factors: factors, Capabilities: capNames, Cap: capTier, Leash: opts.Leash, Now: now})

	// Invariants are evaluated per step and per capability across the whole
	// plan, so a rule about blast radius sees the plan-level total.
	var actions []risk.Action
	totals := map[string]*risk.Action{}
	for _, s := range p.Steps {
		actions = append(actions, risk.Action{Capability: s.Capability, Platform: s.Platform, Params: s.Params, Targets: len(s.Targets), Tier: p.Risk.Tier})
		agg, ok := totals[s.Capability]
		if !ok {
			agg = &risk.Action{Capability: s.Capability, Params: s.Params, Tier: p.Risk.Tier}
			totals[s.Capability] = agg
		}
		agg.Targets += len(s.Targets)
	}
	for _, name := range p.Capabilities() {
		if agg := totals[name]; agg != nil && len(byPlatformSteps(p.Steps, name)) > 1 {
			actions = append(actions, *agg)
		}
	}
	p.Violations = dedupe(pl.Policy.Evaluate(actions))
	if len(p.Violations) > 0 {
		p.Status = StatusBlocked
	}

	// Waves.
	p.Waves = buildWaves(sortedKeys(targets), in.Constraints, p.Risk.Tier, opts)

	// Persistent form.
	p.GitOps = RenderGitOps(p, pl.Catalog)
	return p, nil
}

func buildWaves(ids []uint, c intent.Constraints, tier risk.Tier, opts Options) []Wave {
	if len(ids) == 0 {
		return nil
	}
	canaryPct := c.CanaryPercent
	if canaryPct == 0 {
		canaryPct = opts.DefaultCanaryPercent
	}
	waveSize := c.MaxWaveHosts
	if waveSize <= 0 {
		waveSize = opts.DefaultWaveHosts
	}
	var waves []Wave
	rest := ids
	// A canary wave is used whenever the tier asks for one or the rollout is
	// large enough to be worth staging.
	if tier >= risk.TierCanary || len(ids) > waveSize {
		n := len(ids) * canaryPct / 100
		if n < 1 {
			n = 1
		}
		if n < len(ids) {
			waves = append(waves, Wave{Index: 0, Name: "canary", Targets: ids[:n], Pause: true})
			rest = ids[n:]
		}
	}
	for i := 0; i < len(rest); i += waveSize {
		end := i + waveSize
		if end > len(rest) {
			end = len(rest)
		}
		idx := len(waves)
		waves = append(waves, Wave{Index: idx, Name: fmt.Sprintf("wave-%d", idx+1), Targets: rest[i:end], Pause: end < len(rest)})
	}
	return waves
}

// RenderTemplate substitutes {{.name}} placeholders with params. A []string
// or []any param renders as a quoted, comma-separated SQL list.
func RenderTemplate(tmpl string, params map[string]any) string {
	out := tmpl
	for k, v := range params {
		out = strings.ReplaceAll(out, "{{."+k+"}}", sqlValue(v))
	}
	// Convenience alias used by list-shaped params.
	if v, ok := params["block"]; ok {
		out = strings.ReplaceAll(out, "{{.block_list}}", sqlValue(v))
	}
	return out
}

func sqlValue(v any) string {
	switch vv := v.(type) {
	case []string:
		q := make([]string, len(vv))
		for i, s := range vv {
			q[i] = "'" + strings.ReplaceAll(s, "'", "''") + "'"
		}
		return strings.Join(q, ",")
	case []any:
		q := make([]string, 0, len(vv))
		for _, e := range vv {
			q = append(q, "'"+strings.ReplaceAll(fmt.Sprint(e), "'", "''")+"'")
		}
		return strings.Join(q, ",")
	case string:
		return strings.ReplaceAll(vv, "'", "''")
	default:
		return fmt.Sprint(vv)
	}
}

func cloneParams(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func sortedKeys(m map[uint]bool) []uint {
	out := make([]uint, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func byPlatformSteps(steps []Step, capability string) []Step {
	var out []Step
	for _, s := range steps {
		if s.Capability == capability {
			out = append(out, s)
		}
	}
	return out
}

func dedupe(vs []risk.Violation) []risk.Violation {
	seen := map[string]bool{}
	var out []risk.Violation
	for _, v := range vs {
		k := v.Invariant + "|" + v.Message
		if !seen[k] {
			seen[k] = true
			out = append(out, v)
		}
	}
	return out
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

var impactOrder = map[intent.UserImpact]int{"": 0, intent.ImpactNone: 0, intent.ImpactPrompt: 1, intent.ImpactLogout: 2, intent.ImpactRestart: 3}

func maxImpact(a, b intent.UserImpact) intent.UserImpact {
	if impactOrder[b] > impactOrder[a] {
		return b
	}
	if a == "" {
		return b
	}
	return a
}

var privOrder = map[string]int{"": 0, "user": 1, "system": 2, "mdm": 2}

func maxPrivilege(a, b string) string {
	if privOrder[b] > privOrder[a] {
		return b
	}
	return a
}

// Simulation is a dry-run summary of a plan.
type Simulation struct {
	Hosts       int                     `json:"hosts"`
	ByPlatform  map[intent.Platform]int `json:"by_platform"`
	Steps       int                     `json:"steps"`
	Waves       int                     `json:"waves"`
	Tier        risk.Tier               `json:"tier"`
	Score       int                     `json:"score"`
	Reversible  bool                    `json:"reversible"`
	Unsupported []Unsupported           `json:"unsupported,omitempty"`
	// Estimate is the order-of-magnitude time to effect for the slowest step.
	Estimate string   `json:"estimate"`
	Summary  []string `json:"summary"`
}

// Simulate summarises what a plan would do without touching any device.
func Simulate(p *Plan) Simulation {
	s := Simulation{Hosts: p.Hosts, ByPlatform: map[intent.Platform]int{}, Steps: len(p.Steps), Waves: len(p.Waves), Tier: p.Risk.Tier, Score: p.Risk.Score, Reversible: len(p.Rollback) > 0 && len(p.Rollback) == len(p.Steps), Unsupported: p.Unsupported}
	slowest := capability.LatencyMillis
	for _, st := range p.Steps {
		s.ByPlatform[st.Platform] += len(st.Targets)
		if rank(st.Latency) > rank(slowest) {
			slowest = st.Latency
		}
		s.Summary = append(s.Summary, fmt.Sprintf("%s on %d %s host(s) via %s (%s)", st.Capability, len(st.Targets), st.Platform, st.Mechanism, st.Native))
	}
	switch slowest {
	case capability.LatencyMillis:
		s.Estimate = "milliseconds after the agent is nudged"
	case capability.LatencySeconds:
		s.Estimate = "seconds after the agent is nudged"
	default:
		s.Estimate = "minutes (native staging, restarts or installs)"
	}
	for _, w := range p.Waves {
		s.Summary = append(s.Summary, fmt.Sprintf("%s: %d host(s)%s", w.Name, len(w.Targets), pauseNote(w.Pause)))
	}
	return s
}

func pauseNote(pause bool) string {
	if pause {
		return ", pauses for verification"
	}
	return ""
}

func rank(l capability.Latency) int {
	switch l {
	case capability.LatencyMillis:
		return 0
	case capability.LatencySeconds:
		return 1
	}
	return 2
}
