// Package service is the ADM control plane facade: it wires the compiler,
// catalog, planner, risk engine, substrate, native adapters, executor,
// ledger and store into the operations every interface (CLI, MCP server,
// console, chat) exposes: propose, simulate, approve, apply, explain,
// reconcile.
package service

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/fleetdm/fleet/v4/adm/capability"
	"github.com/fleetdm/fleet/v4/adm/events"
	"github.com/fleetdm/fleet/v4/adm/execute"
	"github.com/fleetdm/fleet/v4/adm/intent"
	"github.com/fleetdm/fleet/v4/adm/learn"
	"github.com/fleetdm/fleet/v4/adm/native"
	"github.com/fleetdm/fleet/v4/adm/plan"
	"github.com/fleetdm/fleet/v4/adm/reason"
	"github.com/fleetdm/fleet/v4/adm/risk"
	"github.com/fleetdm/fleet/v4/adm/substrate"
)

// Service is the control plane.
type Service struct {
	Catalog   *capability.Catalog
	Policy    risk.Policy
	Compiler  reason.Compiler
	Planner   *plan.Planner
	Substrate substrate.Substrate
	Adapters  *native.Registry
	Ledger    learn.Ledger
	Store     Store
	Bus       events.Bus
	Triggers  map[events.Type]events.Trigger
	Now       func() time.Time
}

// Config builds a Service.
type Config struct {
	Catalog   *capability.Catalog
	Policy    *risk.Policy
	Compiler  reason.Compiler
	Substrate substrate.Substrate
	Adapters  *native.Registry
	Ledger    learn.Ledger
	Store     Store
	Bus       events.Bus
}

// New wires a service with sensible defaults for anything left nil.
func New(cfg Config) *Service {
	if cfg.Catalog == nil {
		cfg.Catalog = capability.Default()
	}
	pol := risk.DefaultPolicy()
	if cfg.Policy != nil {
		pol = *cfg.Policy
	}
	if cfg.Compiler == nil {
		cfg.Compiler = reason.NewDeterministic(cfg.Catalog)
	}
	if cfg.Substrate == nil {
		cfg.Substrate = substrate.NewMemory(substrate.DemoFleet()...)
	}
	if cfg.Adapters == nil {
		cfg.Adapters = native.NewRegistry()
	}
	if cfg.Ledger == nil {
		cfg.Ledger = learn.NewMemoryLedger()
	}
	if cfg.Store == nil {
		cfg.Store = NewMemoryStore()
	}
	if cfg.Bus == nil {
		cfg.Bus = events.NewMemoryBus()
	}
	return &Service{
		Catalog: cfg.Catalog, Policy: pol, Compiler: cfg.Compiler, Planner: plan.New(cfg.Catalog, pol),
		Substrate: cfg.Substrate, Adapters: cfg.Adapters, Ledger: cfg.Ledger, Store: cfg.Store, Bus: cfg.Bus,
		Triggers: events.DefaultTriggers, Now: time.Now,
	}
}

// Proposal is the result of Propose.
type Proposal struct {
	Intent         *intent.Intent  `json:"intent"`
	Plan           *plan.Plan      `json:"plan"`
	Simulation     plan.Simulation `json:"simulation"`
	Clarifications []string        `json:"clarifications,omitempty"`
	Summary        string          `json:"summary"`
	// NextStep tells the caller what has to happen for the plan to run.
	NextStep string `json:"next_step"`
}

// Propose compiles text to an intent and a plan. Nothing is executed.
func (s *Service) Propose(ctx context.Context, req reason.Request) (*Proposal, error) {
	if len(req.Fleets) == 0 || len(req.Labels) == 0 {
		fleets, labels := s.hints(ctx)
		if len(req.Fleets) == 0 {
			req.Fleets = fleets
		}
		if len(req.Labels) == 0 {
			req.Labels = labels
		}
	}
	res, err := s.Compiler.Compile(ctx, req)
	if err != nil {
		return nil, err
	}
	if err := reason.ValidateAgainstCatalog(res.Intent, s.Catalog); err != nil {
		return nil, err
	}
	p, err := s.compile(ctx, res.Intent)
	if err != nil {
		return nil, err
	}
	res.Intent.State = intent.StateCompiled
	res.Intent.UpdatedAt = s.Now()
	if err := s.Store.SaveIntent(res.Intent); err != nil {
		return nil, err
	}
	if err := s.Store.SavePlan(p); err != nil {
		return nil, err
	}
	s.record(learn.Outcome{Kind: learn.KindProposed, IntentID: res.Intent.ID, PlanID: p.ID, Pattern: res.Intent.PatternKey(), Fingerprint: res.Intent.Fingerprint(), Capabilities: p.Capabilities(), Tier: p.Risk.Tier, Score: p.Risk.Score, Hosts: p.Hosts, Actor: req.Author, Success: len(p.Violations) == 0, Message: res.Summary})
	return &Proposal{Intent: res.Intent, Plan: p, Simulation: plan.Simulate(p), Clarifications: res.Clarifications, Summary: res.Summary, NextStep: nextStep(p, res.Clarifications)}, nil
}

// Recompile rebuilds the plan for a stored intent (after edits, or when the
// fleet changed).
func (s *Service) Recompile(ctx context.Context, intentID string) (*plan.Plan, error) {
	in, err := s.Store.GetIntent(intentID)
	if err != nil {
		return nil, err
	}
	p, err := s.compile(ctx, in)
	if err != nil {
		return nil, err
	}
	if err := s.Store.SavePlan(p); err != nil {
		return nil, err
	}
	return p, nil
}

func (s *Service) compile(ctx context.Context, in *intent.Intent) (*plan.Plan, error) {
	hosts, err := s.Substrate.ListHosts(ctx, in.Scope)
	if err != nil {
		return nil, fmt.Errorf("list hosts: %w", err)
	}
	total, err := s.Substrate.CountHosts(ctx)
	if err != nil {
		return nil, fmt.Errorf("count hosts: %w", err)
	}
	leash, err := learn.Leash(s.Ledger, in.PatternKey(), s.Now())
	if err != nil {
		return nil, err
	}
	return s.Planner.Compile(in, hosts, plan.Options{FleetSize: total, Leash: &leash, InMaintenanceWindow: inWindow(in.Constraints.MaintenanceWindow, s.Now())})
}

func (s *Service) hints(ctx context.Context) (fleets, labels []string) {
	hosts, err := s.Substrate.ListHosts(ctx, intent.Scope{})
	if err != nil {
		return nil, nil
	}
	fs, ls := map[string]bool{}, map[string]bool{}
	for _, h := range hosts {
		if h.Fleet != "" {
			fs[h.Fleet] = true
		}
		for _, l := range h.Labels {
			ls[l] = true
		}
	}
	for f := range fs {
		fleets = append(fleets, f)
	}
	for l := range ls {
		labels = append(labels, l)
	}
	sort.Strings(fleets)
	sort.Strings(labels)
	return fleets, labels
}

// Approve records a human decision on a plan.
func (s *Service) Approve(ctx context.Context, planID, by, channel, note, ticket string) (*plan.Plan, error) {
	p, err := s.Store.GetPlan(planID)
	if err != nil {
		return nil, err
	}
	if len(p.Violations) > 0 {
		return nil, fmt.Errorf("plan %s violates an invariant and cannot be approved: %v", planID, p.Violations)
	}
	if p.Risk.Tier == risk.TierCAB && ticket == "" {
		return nil, fmt.Errorf("plan %s is at the CAB tier and needs a change ticket", planID)
	}
	if by == "" {
		return nil, errors.New("approver identity is required")
	}
	if by == p.Intent.Author && p.Risk.Tier >= risk.TierApprove && p.Intent.Author != "" {
		return nil, fmt.Errorf("plan %s needs a second person: the author cannot approve their own %s-tier change", planID, p.Risk.Tier)
	}
	p.Approval = &plan.Approval{By: by, At: s.Now(), Channel: channel, Note: note, Ticket: ticket}
	p.Status = plan.StatusApproved
	if err := s.Store.SavePlan(p); err != nil {
		return nil, err
	}
	s.record(learn.Outcome{Kind: learn.KindApproved, IntentID: p.IntentID, PlanID: p.ID, Pattern: p.Intent.PatternKey(), Fingerprint: p.Intent.Fingerprint(), Capabilities: p.Capabilities(), Tier: p.Risk.Tier, Score: p.Risk.Score, Hosts: p.Hosts, Actor: by, Success: true, Message: note})
	return p, nil
}

// Reject records a declined plan; the pattern's leash learns from it.
func (s *Service) Reject(ctx context.Context, planID, by, note string) (*plan.Plan, error) {
	p, err := s.Store.GetPlan(planID)
	if err != nil {
		return nil, err
	}
	p.Status = plan.StatusRejected
	if err := s.Store.SavePlan(p); err != nil {
		return nil, err
	}
	s.record(learn.Outcome{Kind: learn.KindRejected, IntentID: p.IntentID, PlanID: p.ID, Pattern: p.Intent.PatternKey(), Fingerprint: p.Intent.Fingerprint(), Capabilities: p.Capabilities(), Tier: p.Risk.Tier, Actor: by, Message: note})
	return p, nil
}

// Apply executes a plan (or dry-runs it).
func (s *Service) Apply(ctx context.Context, planID, actor string, dryRun bool) (*execute.Report, error) {
	p, err := s.Store.GetPlan(planID)
	if err != nil {
		return nil, err
	}
	ex := execute.New(s.Substrate, s.Adapters, s.Ledger)
	ex.Now = s.Now
	rep, err := ex.Execute(ctx, p, execute.Options{DryRun: dryRun, Actor: actor})
	if err != nil {
		return nil, err
	}
	if !dryRun {
		if err := s.Store.SavePlan(p); err != nil {
			return nil, err
		}
		if in, err := s.Store.GetIntent(p.IntentID); err == nil {
			switch rep.Status {
			case plan.StatusVerified:
				in.State = intent.StateActive
			case plan.StatusRolledBack:
				in.State = intent.StatePaused
			}
			in.UpdatedAt = s.Now()
			_ = s.Store.SaveIntent(in)
		}
		_ = s.Bus.Publish(ctx, events.Event{Type: events.IntentChanged, Source: "adm", Subject: p.IntentID, Payload: map[string]any{"plan": p.ID, "status": string(rep.Status)}})
	}
	return rep, nil
}

// Explanation is the human-readable "why" of a plan.
type Explanation struct {
	Plan        *plan.Plan      `json:"plan"`
	Simulation  plan.Simulation `json:"simulation"`
	Leash       learn.Report    `json:"leash"`
	History     []learn.Outcome `json:"history"`
	Narrative   []string        `json:"narrative"`
	RollbackHow []string        `json:"rollback_how"`
}

// Explain narrates a plan: what, where, how (natively), why this tier, how
// to undo it, and what happened so far.
func (s *Service) Explain(ctx context.Context, planID string) (*Explanation, error) {
	p, err := s.Store.GetPlan(planID)
	if err != nil {
		return nil, err
	}
	leash, err := learn.Explain(s.Ledger, s.Policy, p.Intent.PatternKey(), s.Now())
	if err != nil {
		return nil, err
	}
	history, _ := s.Ledger.Query(learn.Filter{PlanID: planID})
	ex := &Explanation{Plan: p, Simulation: plan.Simulate(p), Leash: leash, History: history}
	ex.Narrative = append(ex.Narrative, fmt.Sprintf("Intent %s by %s: %q", p.IntentID, p.Intent.Author, strings.Join(strings.Fields(p.Intent.Text), " ")))
	ex.Narrative = append(ex.Narrative, fmt.Sprintf("Compiled by %s (confidence %.2f) into %d step(s) on %d host(s) across %v.", p.Intent.Provenance.Compiler, p.Intent.Provenance.Confidence, len(p.Steps), p.Hosts, p.Platforms()))
	for _, st := range p.Steps {
		ex.Narrative = append(ex.Narrative, fmt.Sprintf("- %s on %s (%d host(s)): %s via %s", st.Capability, st.Platform, len(st.Targets), st.Native, st.Mechanism))
	}
	ex.Narrative = append(ex.Narrative, fmt.Sprintf("Risk score %d -> tier %s. %s", p.Risk.Score, p.Risk.Tier, strings.Join(p.Risk.Reasons, "; ")))
	if leash.Rows > 0 {
		ex.Narrative = append(ex.Narrative, fmt.Sprintf("Pattern %s has %d accepted, %d verified, %d rejected, %d incident(s); leash relaxation %d.", leash.Pattern, leash.State.Accepted, leash.State.Verified, leash.State.Rejected, leash.State.Incidents, leash.Relaxation))
	} else {
		ex.Narrative = append(ex.Narrative, "This pattern of change has never run in this tenant.")
	}
	for _, v := range p.Violations {
		ex.Narrative = append(ex.Narrative, "BLOCKED: "+v.Error())
	}
	for _, u := range p.Unsupported {
		ex.Narrative = append(ex.Narrative, fmt.Sprintf("Not compiled for %d %s host(s): %s (%s)", u.Hosts, u.Platform, u.Reason, u.Capability))
	}
	if len(p.Rollback) == 0 {
		ex.RollbackHow = append(ex.RollbackHow, "No automatic rollback: the change is not reversible; restore from the prior state manually.")
	}
	for _, rb := range p.Rollback {
		ex.RollbackHow = append(ex.RollbackHow, fmt.Sprintf("revert %s on %s (%d host(s)) by restoring the state captured before apply", rb.Capability, rb.Platform, len(rb.Targets)))
	}
	return ex, nil
}

// GetPlan returns a plan.
func (s *Service) GetPlan(_ context.Context, planID string) (*plan.Plan, error) {
	return s.Store.GetPlan(planID)
}

// ListPlans returns every plan, oldest first.
func (s *Service) ListPlans(_ context.Context) ([]*plan.Plan, error) { return s.Store.ListPlans() }

// ListIntents returns every intent, oldest first.
func (s *Service) ListIntents(_ context.Context) ([]*intent.Intent, error) {
	return s.Store.ListIntents()
}

// SetIntentState pauses, resumes or retires an intent.
func (s *Service) SetIntentState(_ context.Context, intentID string, state intent.State) (*intent.Intent, error) {
	in, err := s.Store.GetIntent(intentID)
	if err != nil {
		return nil, err
	}
	in.State = state
	in.UpdatedAt = s.Now()
	return in, s.Store.SaveIntent(in)
}

// Reconcile handles one event: every active intent the event makes stale is
// recompiled for the affected host (a one-host micro-plan) and, when its tier
// allows unattended execution, applied immediately. Higher tiers produce a
// plan waiting for approval. This is the event-speed path; the periodic
// sweep publishes events.Sweep to catch anything missed.
func (s *Service) Reconcile(ctx context.Context, e events.Event) ([]*plan.Plan, error) {
	intents, err := s.Store.ListIntents()
	if err != nil {
		return nil, err
	}
	var out []*plan.Plan
	for _, in := range intents {
		re, rescope := events.Affected(e, in, s.Triggers)
		if !re && !rescope {
			continue
		}
		scoped := *in
		if e.HostID != 0 && !rescope {
			// Micro-plan for the one host that changed.
			scoped.Scope.HostIDs = []uint{e.HostID}
		}
		p, err := s.compile(ctx, &scoped)
		if err != nil {
			s.record(learn.Outcome{Kind: learn.KindDrift, IntentID: in.ID, Pattern: in.PatternKey(), HostID: e.HostID, Message: "reconcile compile failed: " + err.Error()})
			continue
		}
		if p.Hosts == 0 {
			continue
		}
		p.Intent = *in
		if err := s.Store.SavePlan(p); err != nil {
			return out, err
		}
		s.record(learn.Outcome{Kind: learn.KindDrift, IntentID: in.ID, PlanID: p.ID, Pattern: in.PatternKey(), Fingerprint: in.Fingerprint(), Capabilities: p.Capabilities(), Tier: p.Risk.Tier, Hosts: p.Hosts, HostID: e.HostID, Message: fmt.Sprintf("event %s triggered reconciliation", e.Type)})
		if execute.TierAllowsUnattended(p.Risk.Tier) && len(p.Violations) == 0 {
			if _, err := s.Apply(ctx, p.ID, "adm-reconciler", false); err != nil {
				s.record(learn.Outcome{Kind: learn.KindFailed, IntentID: in.ID, PlanID: p.ID, Pattern: in.PatternKey(), Message: "reconcile apply failed: " + err.Error()})
			}
			p, _ = s.Store.GetPlan(p.ID)
		}
		out = append(out, p)
	}
	return out, nil
}

// Leash reports the adaptive-autonomy state of a pattern.
func (s *Service) Leash(_ context.Context, pattern string) (learn.Report, error) {
	return learn.Explain(s.Ledger, s.Policy, pattern, s.Now())
}

func (s *Service) record(o learn.Outcome) {
	if o.At.IsZero() {
		o.At = s.Now()
	}
	_ = s.Ledger.Record(o)
}

func nextStep(p *plan.Plan, clar []string) string {
	switch {
	case len(clar) > 0:
		return "answer the clarifying question(s) and propose again"
	case len(p.Violations) > 0:
		return "blocked: " + p.Violations[0].Error()
	case p.Risk.Tier == risk.TierObserve:
		return "observe-only: run `apply --dry-run` to see drift"
	case p.Risk.Tier == risk.TierCAB:
		return "approve with a change ticket (CAB tier), then apply"
	case p.Risk.Tier == risk.TierApprove:
		return "approve (a second person), then apply"
	case p.Risk.Tier == risk.TierCanary:
		return "apply: the canary wave runs first and pauses for verification"
	}
	return "apply: low risk, runs unattended"
}

func inWindow(w *intent.Window, now time.Time) bool {
	if w == nil {
		return false
	}
	if len(w.Days) > 0 {
		ok := false
		for _, d := range w.Days {
			if strings.EqualFold(d, now.Weekday().String()) {
				ok = true
				break
			}
		}
		if !ok {
			return false
		}
	}
	hm := now.Format("15:04")
	return hm >= w.Start && hm <= w.End
}
