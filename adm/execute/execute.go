// Package execute runs plans: wave by wave, host by host, through native
// adapters, with verification after every wave and automatic rollback when a
// wave fails. It never calls Apply without a Check, so converged hosts are
// left alone, and it records every outcome in the ledger.
package execute

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/fleetdm/fleet/v4/adm/capability"
	"github.com/fleetdm/fleet/v4/adm/learn"
	"github.com/fleetdm/fleet/v4/adm/native"
	"github.com/fleetdm/fleet/v4/adm/plan"
	"github.com/fleetdm/fleet/v4/adm/risk"
	"github.com/fleetdm/fleet/v4/adm/substrate"
)

// StepResult is the outcome of one step on one host.
type StepResult struct {
	StepID   string        `json:"step_id"`
	HostID   uint          `json:"host_id"`
	Changed  bool          `json:"changed"`
	Skipped  bool          `json:"skipped"` // already satisfied
	Verified *bool         `json:"verified,omitempty"`
	Error    string        `json:"error,omitempty"`
	Native   string        `json:"native,omitempty"`
	Duration time.Duration `json:"duration,omitempty"`
}

// WaveResult is the outcome of one wave.
type WaveResult struct {
	Wave       plan.Wave    `json:"wave"`
	Applied    int          `json:"applied"`
	Skipped    int          `json:"skipped"`
	Failed     int          `json:"failed"`
	Unverified int          `json:"unverified"`
	// Drifted counts hosts a dry run found out of the desired state.
	Drifted    int          `json:"drifted,omitempty"`
	RolledBack bool         `json:"rolled_back"`
	Results    []StepResult `json:"results"`
}

// Report is the outcome of an execution.
type Report struct {
	PlanID     string       `json:"plan_id"`
	DryRun     bool         `json:"dry_run"`
	Status     plan.Status  `json:"status"`
	Waves      []WaveResult `json:"waves"`
	StartedAt  time.Time    `json:"started_at"`
	FinishedAt time.Time    `json:"finished_at"`
	Summary    string       `json:"summary"`
	// Drift lists hosts that were not satisfied at check time (dry runs).
	Drift []uint `json:"drift,omitempty"`
}

// Options tune one execution.
type Options struct {
	// DryRun only checks; nothing is applied.
	DryRun bool
	Actor  string
	// PauseHook is consulted after a wave that pauses for verification. It
	// returns whether to continue. The default continues when verification
	// passed.
	PauseHook func(ctx context.Context, w WaveResult) (bool, error)
}

// Executor runs plans.
type Executor struct {
	Substrate substrate.Substrate
	Adapters  *native.Registry
	Ledger    learn.Ledger
	Now       func() time.Time
	// MaxFailureRate is the fraction of a wave's hosts that may fail (apply or
	// verification) before the wave is rolled back and the plan stops.
	MaxFailureRate float64
	// PersistVerification turns every step verification into a substrate
	// policy so drift stays visible after the execution ends.
	PersistVerification bool
}

// New returns an executor with defaults.
func New(s substrate.Substrate, adapters *native.Registry, ledger learn.Ledger) *Executor {
	return &Executor{Substrate: s, Adapters: adapters, Ledger: ledger, Now: time.Now, MaxFailureRate: 0.1, PersistVerification: true}
}

// ErrNotExecutable is returned when the plan may not run.
var ErrNotExecutable = errors.New("plan is not executable")

// Execute runs the plan.
func (e *Executor) Execute(ctx context.Context, p *plan.Plan, opts Options) (*Report, error) {
	if !opts.DryRun {
		if ok, why := p.Executable(); !ok {
			return nil, fmt.Errorf("%w: %s", ErrNotExecutable, why)
		}
	}
	if opts.PauseHook == nil {
		opts.PauseHook = func(context.Context, WaveResult) (bool, error) { return true, nil }
	}
	rep := &Report{PlanID: p.ID, DryRun: opts.DryRun, StartedAt: e.Now(), Status: plan.StatusExecuting}
	if !opts.DryRun {
		p.Status = plan.StatusExecuting
		e.record(learn.Outcome{Kind: learn.KindExecuted, PlanID: p.ID, IntentID: p.IntentID, Pattern: p.Intent.PatternKey(), Fingerprint: p.Intent.Fingerprint(), Capabilities: p.Capabilities(), Tier: p.Risk.Tier, Score: p.Risk.Score, Hosts: p.Hosts, Actor: opts.Actor, Success: true, Message: "execution started"})
		if e.PersistVerification {
			e.persistVerification(ctx, p)
		}
	}

	targets := map[uint]native.Target{}
	for _, s := range p.Steps {
		for _, id := range s.Targets {
			targets[id] = native.Target{HostID: id, Platform: s.Platform}
		}
	}

	var drift []uint
	for _, w := range p.Waves {
		wr := WaveResult{Wave: w}
		inWave := map[uint]bool{}
		for _, id := range w.Targets {
			inWave[id] = true
		}
		priors := map[string]native.State{}
		for _, step := range p.Steps {
			adapter, ok := e.Adapters.Lookup(step.Capability, step.Platform)
			for _, id := range step.Targets {
				if !inWave[id] {
					continue
				}
				r := StepResult{StepID: step.ID, HostID: id}
				t := targets[id]
				switch {
				case ok:
					state, err := adapter.Check(ctx, t, step.Params)
					if err != nil {
						r.Error = "check: " + err.Error()
						break
					}
					if state.IsSatisfied() {
						r.Skipped = true
						break
					}
					if opts.DryRun {
						drift = append(drift, id)
						break
					}
					priors[step.ID+"/"+fmt.Sprint(id)] = state
					res, err := adapter.Apply(ctx, t, step.Params)
					r.Duration = res.Duration
					r.Native = res.Native
					if err != nil {
						r.Error = "apply: " + err.Error()
						break
					}
					r.Changed = res.Changed
				case step.Mechanism == capability.MechScript:
					if opts.DryRun {
						drift = append(drift, id)
						break
					}
					script, _ := step.Params["script"].(string)
					sr, err := e.Substrate.RunScript(ctx, id, script)
					r.Native = "script"
					if err != nil {
						r.Error = "script: " + err.Error()
					} else if sr.ExitCode != 0 {
						r.Error = fmt.Sprintf("script exited %d: %s", sr.ExitCode, truncate(sr.Output, 200))
					} else {
						r.Changed = true
					}
				default:
					r.Error = fmt.Sprintf("no native adapter registered for %s on %s", step.Capability, step.Platform)
				}
				switch {
				case r.Error != "":
					wr.Failed++
				case r.Skipped:
					wr.Skipped++
				case r.Changed:
					wr.Applied++
				default:
					wr.Drifted++
				}
				wr.Results = append(wr.Results, r)
			}
		}

		if !opts.DryRun {
			// Ask the agents on the wave to check in now; verification follows.
			if err := e.Substrate.Nudge(ctx, w.Targets); err != nil && !errors.Is(err, substrate.ErrUnsupported) {
				e.record(learn.Outcome{Kind: learn.KindDrift, PlanID: p.ID, Pattern: p.Intent.PatternKey(), Message: "nudge failed: " + err.Error()})
			}
			e.verifyWave(ctx, p, &wr)
		}

		failureRate := 0.0
		if n := len(w.Targets); n > 0 {
			failureRate = float64(wr.Failed+wr.Unverified) / float64(n)
		}
		if !opts.DryRun && failureRate > e.MaxFailureRate {
			e.rollbackWave(ctx, p, &wr, priors, targets)
			rep.Waves = append(rep.Waves, wr)
			rep.Status = plan.StatusRolledBack
			p.Status = plan.StatusRolledBack
			rep.FinishedAt = e.Now()
			rep.Summary = fmt.Sprintf("%s failed on %d of %d host(s) (%.0f%% > %.0f%% allowed); wave rolled back and plan stopped", w.Name, wr.Failed+wr.Unverified, len(w.Targets), failureRate*100, e.MaxFailureRate*100)
			e.record(learn.Outcome{Kind: learn.KindRolledBack, PlanID: p.ID, IntentID: p.IntentID, Pattern: p.Intent.PatternKey(), Fingerprint: p.Intent.Fingerprint(), Capabilities: p.Capabilities(), Tier: p.Risk.Tier, Hosts: len(w.Targets), Actor: opts.Actor, Message: rep.Summary})
			return rep, nil
		}
		rep.Waves = append(rep.Waves, wr)
		if !opts.DryRun {
			e.record(learn.Outcome{Kind: learn.KindExecuted, PlanID: p.ID, IntentID: p.IntentID, Pattern: p.Intent.PatternKey(), Tier: p.Risk.Tier, Hosts: len(w.Targets), Actor: opts.Actor, Success: wr.Failed == 0, Message: fmt.Sprintf("%s: applied %d, skipped %d, failed %d, unverified %d", w.Name, wr.Applied, wr.Skipped, wr.Failed, wr.Unverified)})
		}
		if w.Pause && !opts.DryRun {
			cont, err := opts.PauseHook(ctx, wr)
			if err != nil {
				return rep, err
			}
			if !cont {
				rep.Status = plan.StatusExecuting
				rep.FinishedAt = e.Now()
				rep.Summary = fmt.Sprintf("paused after %s", w.Name)
				return rep, nil
			}
		}
	}

	rep.FinishedAt = e.Now()
	if opts.DryRun {
		sort.Slice(drift, func(i, j int) bool { return drift[i] < drift[j] })
		rep.Drift = drift
		rep.Status = plan.StatusProposed
		rep.Summary = fmt.Sprintf("dry run: %d host(s) drifted from the intent, %d already satisfied", len(drift), count(rep.Waves, func(r StepResult) bool { return r.Skipped }))
		return rep, nil
	}
	rep.Status = plan.StatusVerified
	p.Status = plan.StatusVerified
	rep.Summary = fmt.Sprintf("verified: applied %d, skipped %d, failed %d across %d wave(s)", count(rep.Waves, func(r StepResult) bool { return r.Changed }), count(rep.Waves, func(r StepResult) bool { return r.Skipped }), count(rep.Waves, func(r StepResult) bool { return r.Error != "" }), len(rep.Waves))
	e.record(learn.Outcome{Kind: learn.KindVerified, PlanID: p.ID, IntentID: p.IntentID, Pattern: p.Intent.PatternKey(), Fingerprint: p.Intent.Fingerprint(), Capabilities: p.Capabilities(), Tier: p.Risk.Tier, Score: p.Risk.Score, Hosts: p.Hosts, Actor: opts.Actor, Success: true, Message: rep.Summary, Duration: rep.FinishedAt.Sub(rep.StartedAt)})
	return rep, nil
}

// verifyWave runs each step's verification query on the wave's hosts and
// marks results.
func (e *Executor) verifyWave(ctx context.Context, p *plan.Plan, wr *WaveResult) {
	byStep := map[string]*plan.Step{}
	for i := range p.Steps {
		byStep[p.Steps[i].ID] = &p.Steps[i]
	}
	hostsByStep := map[string][]uint{}
	for _, r := range wr.Results {
		if r.Error != "" {
			continue
		}
		hostsByStep[r.StepID] = append(hostsByStep[r.StepID], r.HostID)
	}
	verdicts := map[string]bool{}
	for stepID, hosts := range hostsByStep {
		step := byStep[stepID]
		if step == nil || step.Verify == nil {
			continue
		}
		res, err := e.Substrate.LiveQuery(ctx, step.Verify.Query, hosts)
		if err != nil {
			for _, h := range hosts {
				verdicts[stepID+"/"+fmt.Sprint(h)] = false
			}
			continue
		}
		for _, h := range hosts {
			if _, responded := res.Rows[h]; !responded {
				continue // offline hosts are verified when they come back (events.HostOnline)
			}
			rows := len(res.RowsFor(h))
			ok := (step.Verify.Expect == "rows>0" && rows > 0) || (step.Verify.Expect == "rows==0" && rows == 0)
			verdicts[stepID+"/"+fmt.Sprint(h)] = ok
		}
	}
	for i := range wr.Results {
		r := &wr.Results[i]
		if v, ok := verdicts[r.StepID+"/"+fmt.Sprint(r.HostID)]; ok {
			vv := v
			r.Verified = &vv
			if !v {
				wr.Unverified++
				e.record(learn.Outcome{Kind: learn.KindFailed, PlanID: p.ID, IntentID: p.IntentID, Pattern: p.Intent.PatternKey(), HostID: r.HostID, Tier: p.Risk.Tier, Message: "verification failed for " + r.StepID})
			}
		}
	}
}

// rollbackWave reverts every changed host in the wave using captured priors.
func (e *Executor) rollbackWave(ctx context.Context, p *plan.Plan, wr *WaveResult, priors map[string]native.State, targets map[uint]native.Target) {
	byStep := map[string]*plan.Step{}
	for i := range p.Steps {
		byStep[p.Steps[i].ID] = &p.Steps[i]
	}
	for i := len(wr.Results) - 1; i >= 0; i-- {
		r := wr.Results[i]
		if !r.Changed {
			continue
		}
		step := byStep[r.StepID]
		adapter, ok := e.Adapters.Lookup(step.Capability, step.Platform)
		if !ok || !step.Reversible {
			continue
		}
		prior := priors[r.StepID+"/"+fmt.Sprint(r.HostID)]
		if _, err := adapter.Revert(ctx, targets[r.HostID], step.Params, prior); err != nil {
			e.record(learn.Outcome{Kind: learn.KindIncident, PlanID: p.ID, IntentID: p.IntentID, Pattern: p.Intent.PatternKey(), HostID: r.HostID, Tier: p.Risk.Tier, Message: "rollback failed: " + err.Error()})
			continue
		}
	}
	wr.RolledBack = true
}

func (e *Executor) persistVerification(ctx context.Context, p *plan.Plan) {
	for _, s := range p.Steps {
		if s.Verify == nil {
			continue
		}
		query := s.Verify.Query
		if s.Verify.Expect == "rows==0" {
			query = "SELECT 1 WHERE NOT EXISTS (" + query + ")"
		}
		pol := substrate.Policy{
			Name:        fmt.Sprintf("ADM %s (%s)", s.Capability, s.Platform),
			Query:       query,
			Description: "Verifies ADM intent " + p.IntentID + ": " + strings.Join(strings.Fields(p.Intent.Text), " "),
			Resolution:  "ADM reconciles this automatically; see adm explain " + p.ID,
			Platform:    s.Platform,
		}
		if len(p.Intent.Scope.Fleets) == 1 {
			pol.Fleet = p.Intent.Scope.Fleets[0]
		}
		if err := e.Substrate.UpsertPolicy(ctx, pol); err != nil && !errors.Is(err, substrate.ErrUnsupported) {
			e.record(learn.Outcome{Kind: learn.KindDrift, PlanID: p.ID, Pattern: p.Intent.PatternKey(), Message: "persist verification: " + err.Error()})
		}
	}
}

func (e *Executor) record(o learn.Outcome) {
	if e.Ledger == nil {
		return
	}
	if o.At.IsZero() {
		o.At = e.Now()
	}
	_ = e.Ledger.Record(o)
}

func count(waves []WaveResult, pred func(StepResult) bool) int {
	n := 0
	for _, w := range waves {
		for _, r := range w.Results {
			if pred(r) {
				n++
			}
		}
	}
	return n
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// TierAllowsUnattended reports whether a tier may run without a human in the loop.
func TierAllowsUnattended(t risk.Tier) bool { return t == risk.TierAuto || t == risk.TierCanary }
