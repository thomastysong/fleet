package service

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fleetdm/fleet/v4/adm/device"
	"github.com/fleetdm/fleet/v4/adm/events"
	"github.com/fleetdm/fleet/v4/adm/intent"
	"github.com/fleetdm/fleet/v4/adm/learn"
	"github.com/fleetdm/fleet/v4/adm/native"
	"github.com/fleetdm/fleet/v4/adm/plan"
	"github.com/fleetdm/fleet/v4/adm/reason"
	"github.com/fleetdm/fleet/v4/adm/risk"
	"github.com/fleetdm/fleet/v4/adm/substrate"
)

// flagAdapter converges a flag per host for one capability/platform.
type flagAdapter struct {
	cap   string
	plat  intent.Platform
	state map[uint]bool
}

func (f *flagAdapter) ID() string                { return f.cap + "@" + string(f.plat) }
func (f *flagAdapter) Capability() string        { return f.cap }
func (f *flagAdapter) Platform() intent.Platform { return f.plat }
func (f *flagAdapter) Check(_ context.Context, t native.Target, _ map[string]any) (native.State, error) {
	return native.State{"on": f.state[t.HostID], native.Satisfied: f.state[t.HostID]}, nil
}
func (f *flagAdapter) Apply(_ context.Context, t native.Target, _ map[string]any) (native.Result, error) {
	f.state[t.HostID] = true
	return native.Result{Changed: true}, nil
}
func (f *flagAdapter) Revert(_ context.Context, t native.Target, _ map[string]any, prior native.State) (native.Result, error) {
	f.state[t.HostID], _ = prior["on"].(bool)
	return native.Result{Changed: true}, nil
}

func newTestService(t *testing.T) (*Service, *substrate.Memory, map[string]*flagAdapter) {
	t.Helper()
	mem := substrate.NewMemory(substrate.DemoFleet()...)
	reg := native.NewRegistry()
	adapters := map[string]*flagAdapter{}
	for _, p := range []intent.Platform{intent.PlatformDarwin, intent.PlatformWindows, intent.PlatformLinux} {
		a := &flagAdapter{cap: "firewall.enable", plat: p, state: map[uint]bool{}}
		adapters[string(p)] = a
		reg.MustRegister(a)
	}
	mem.Respond("alf", func(h device.Host) []map[string]string { return rows(adapters["darwin"].state[h.ID]) })
	mem.Respond("windows_security_center", func(h device.Host) []map[string]string { return rows(adapters["windows"].state[h.ID]) })
	mem.Respond("iptables", func(h device.Host) []map[string]string { return rows(adapters["linux"].state[h.ID]) })
	svc := New(Config{Substrate: mem, Adapters: reg})
	now := time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC)
	svc.Now = func() time.Time { return now }
	return svc, mem, adapters
}

func rows(on bool) []map[string]string {
	if on {
		return []map[string]string{{"1": "1"}}
	}
	return nil
}

func TestProposeApplyExplainRoundTrip(t *testing.T) {
	svc, mem, adapters := newTestService(t)
	ctx := context.Background()
	prop, err := svc.Propose(ctx, reason.Request{Text: "turn on the firewall on Engineering laptops automatically", Author: "alice"})
	if err != nil {
		t.Fatal(err)
	}
	// Four of the eight demo hosts is half the fleet on a never-seen pattern:
	// the risk engine stages it as a canary even though the operator said
	// "automatically" (an intent cap can only make things more conservative).
	if prop.Plan.Hosts != 4 || len(prop.Plan.Steps) != 3 || prop.Plan.Risk.Tier != risk.TierCanary {
		t.Fatalf("plan hosts=%d steps=%d tier=%s reasons=%v", prop.Plan.Hosts, len(prop.Plan.Steps), prop.Plan.Risk.Tier, prop.Plan.Risk.Reasons)
	}
	if prop.Plan.Waves[0].Name != "canary" || len(prop.Plan.Waves[0].Targets) != 1 {
		t.Fatalf("waves %+v", prop.Plan.Waves)
	}
	if !strings.HasPrefix(prop.NextStep, "apply") {
		t.Fatalf("next step %q", prop.NextStep)
	}
	rep, err := svc.Apply(ctx, prop.Plan.ID, "alice", false)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Status != plan.StatusVerified {
		t.Fatalf("%s: %s", rep.Status, rep.Summary)
	}
	// Offline host 4 (windows) was applied but cannot verify until it returns.
	if !adapters["windows"].state[4] || !adapters["darwin"].state[1] || !adapters["linux"].state[5] {
		t.Fatalf("adapters not converged: %+v", adapters)
	}
	if len(mem.Policies) != 3 {
		t.Fatalf("expected a persistent verification policy per platform, got %d", len(mem.Policies))
	}
	in, _ := svc.Store.GetIntent(prop.Intent.ID)
	if in.State != intent.StateActive {
		t.Fatalf("intent state %s", in.State)
	}
	ex, err := svc.Explain(ctx, prop.Plan.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(ex.Narrative) < 5 || len(ex.History) < 2 || len(ex.RollbackHow) != 3 {
		t.Fatalf("explanation incomplete: %+v", ex)
	}
}

func TestApprovalGateAndSecondPerson(t *testing.T) {
	svc, _, _ := newTestService(t)
	ctx := context.Background()
	prop, err := svc.Propose(ctx, reason.Request{Text: "lock the device mbp-alice in Engineering", Author: "alice"})
	if err != nil {
		t.Fatal(err)
	}
	if prop.Plan.Risk.Tier != risk.TierApprove {
		t.Fatalf("tier %s", prop.Plan.Risk.Tier)
	}
	if _, err := svc.Apply(ctx, prop.Plan.ID, "alice", false); err == nil {
		t.Fatal("unapproved plan must not run")
	}
	if _, err := svc.Approve(ctx, prop.Plan.ID, "alice", "cli", "", ""); err == nil {
		t.Fatal("author must not approve their own change")
	}
	p, err := svc.Approve(ctx, prop.Plan.ID, "bob", "cli", "ok", "")
	if err != nil || p.Status != plan.StatusApproved || p.Approval.By != "bob" {
		t.Fatalf("%v %+v", err, p)
	}
	rows, _ := svc.Ledger.Query(learn.Filter{PlanID: p.ID, Kinds: []learn.Kind{learn.KindApproved}})
	if len(rows) != 1 {
		t.Fatal("approval not in ledger")
	}
	wipe, err := svc.Propose(ctx, reason.Request{Text: "wipe the device mbp-bo in Engineering", Author: "alice"})
	if err != nil {
		t.Fatal(err)
	}
	if wipe.Plan.Risk.Tier != risk.TierCAB {
		t.Fatalf("wipe tier %s", wipe.Plan.Risk.Tier)
	}
	if _, err := svc.Approve(ctx, wipe.Plan.ID, "bob", "cli", "", ""); err == nil {
		t.Fatal("CAB tier needs a ticket")
	}
}

func TestReconcileMicroPlanOnEvent(t *testing.T) {
	svc, _, adapters := newTestService(t)
	ctx := context.Background()
	prop, err := svc.Propose(ctx, reason.Request{Text: "turn on the firewall on Engineering laptops automatically", Author: "alice"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Apply(ctx, prop.Plan.ID, "alice", false); err != nil {
		t.Fatal(err)
	}
	// Drift: host 5 turns its firewall off; the OS watcher reports it.
	adapters["linux"].state[5] = false
	plans, err := svc.Reconcile(ctx, events.Event{Type: events.ConfigDrift, HostID: 5, Platform: intent.PlatformLinux, Subject: "firewall.enable"})
	if err != nil {
		t.Fatal(err)
	}
	if len(plans) != 1 || plans[0].Hosts != 1 || plans[0].Status != plan.StatusVerified {
		t.Fatalf("expected a verified one-host micro-plan, got %+v", plans)
	}
	if !adapters["linux"].state[5] {
		t.Fatal("drift was not corrected")
	}
	// Events for other platforms or unrelated capabilities do nothing.
	plans, _ = svc.Reconcile(ctx, events.Event{Type: events.StorageAttached, HostID: 5, Platform: intent.PlatformLinux})
	if len(plans) != 0 {
		t.Fatalf("unrelated event produced plans: %+v", plans)
	}
}

func TestFileStorePersists(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "state")
	fs, err := NewFileStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	svc := New(Config{Store: fs})
	prop, err := svc.Propose(context.Background(), reason.Request{Text: "enable the firewall everywhere", Author: "alice"})
	if err != nil {
		t.Fatal(err)
	}
	fs2, err := NewFileStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	p, err := fs2.GetPlan(prop.Plan.ID)
	if err != nil || p.IntentID != prop.Intent.ID || len(p.Steps) == 0 {
		t.Fatalf("%v %+v", err, p)
	}
	if _, err := fs2.GetPlan("nope"); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}
