package execute

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/fleetdm/fleet/v4/adm/capability"
	"github.com/fleetdm/fleet/v4/adm/device"
	"github.com/fleetdm/fleet/v4/adm/intent"
	"github.com/fleetdm/fleet/v4/adm/learn"
	"github.com/fleetdm/fleet/v4/adm/native"
	"github.com/fleetdm/fleet/v4/adm/plan"
	"github.com/fleetdm/fleet/v4/adm/risk"
	"github.com/fleetdm/fleet/v4/adm/substrate"
)

// fakeAdapter converges a per-host flag; failHosts fail on Apply.
type fakeAdapter struct {
	cap       string
	platform  intent.Platform
	state     map[uint]bool
	failHosts map[uint]bool
	reverts   []uint
}

func (f *fakeAdapter) ID() string                { return "fake." + f.cap }
func (f *fakeAdapter) Capability() string        { return f.cap }
func (f *fakeAdapter) Platform() intent.Platform { return f.platform }
func (f *fakeAdapter) Check(_ context.Context, t native.Target, _ map[string]any) (native.State, error) {
	return native.State{"on": f.state[t.HostID], native.Satisfied: f.state[t.HostID]}, nil
}
func (f *fakeAdapter) Apply(_ context.Context, t native.Target, _ map[string]any) (native.Result, error) {
	if f.failHosts[t.HostID] {
		return native.Result{}, errors.New("boom")
	}
	f.state[t.HostID] = true
	return native.Result{Changed: true, Native: "fake"}, nil
}
func (f *fakeAdapter) Revert(_ context.Context, t native.Target, _ map[string]any, prior native.State) (native.Result, error) {
	f.state[t.HostID], _ = prior["on"].(bool)
	f.reverts = append(f.reverts, t.HostID)
	return native.Result{Changed: true}, nil
}

func setup(t *testing.T, n int, canary int) (*plan.Plan, *substrate.Memory, *fakeAdapter, *learn.MemoryLedger) {
	t.Helper()
	var hosts []device.Host
	for i := 1; i <= n; i++ {
		hosts = append(hosts, device.Host{ID: uint(i), Platform: intent.PlatformLinux, Fleet: "Eng", Online: true})
	}
	mem := substrate.NewMemory(hosts...)
	ad := &fakeAdapter{cap: "firewall.enable", platform: intent.PlatformLinux, state: map[uint]bool{}, failHosts: map[uint]bool{}}
	// Verification answers from the adapter's state.
	mem.Respond("iptables", func(h device.Host) []map[string]string {
		if ad.state[h.ID] {
			return []map[string]string{{"1": "1"}}
		}
		return nil
	})
	pl := plan.New(capability.Default(), risk.DefaultPolicy())
	in := &intent.Intent{ID: "i", Text: "firewall on", Author: "a", Scope: intent.Scope{Fleets: []string{"Eng"}},
		Constraints: intent.Constraints{CanaryPercent: canary, MaxAutonomy: intent.AutonomyCanary},
		Desired:     []intent.Predicate{{Capability: "firewall.enable"}}, Provenance: intent.Provenance{Confidence: 1}}
	p, err := pl.Compile(in, hosts, plan.Options{FleetSize: n})
	if err != nil {
		t.Fatal(err)
	}
	return p, mem, ad, learn.NewMemoryLedger()
}

func TestExecuteConvergesAndVerifies(t *testing.T) {
	p, mem, ad, ledger := setup(t, 10, 20)
	ad.state[3] = true // already satisfied
	reg := native.NewRegistry()
	reg.MustRegister(ad)
	ex := New(mem, reg, ledger)
	rep, err := ex.Execute(context.Background(), p, Options{Actor: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Status != plan.StatusVerified || p.Status != plan.StatusVerified {
		t.Fatalf("status %s: %s", rep.Status, rep.Summary)
	}
	applied, skipped := count(rep.Waves, func(r StepResult) bool { return r.Changed }), count(rep.Waves, func(r StepResult) bool { return r.Skipped })
	if applied != 9 || skipped != 1 {
		t.Fatalf("applied %d skipped %d", applied, skipped)
	}
	for _, w := range rep.Waves {
		for _, r := range w.Results {
			if r.Verified == nil || !*r.Verified {
				t.Fatalf("host %d not verified: %+v", r.HostID, r)
			}
		}
	}
	if len(rep.Waves) != 2 || rep.Waves[0].Wave.Name != "canary" {
		t.Fatalf("waves %+v", rep.Waves)
	}
	if len(mem.Policies) != 1 || len(mem.Nudged) != 2 {
		t.Fatalf("verification policy %d nudges %d", len(mem.Policies), len(mem.Nudged))
	}
	rows, _ := ledger.Query(learn.Filter{PlanID: p.ID, Kinds: []learn.Kind{learn.KindVerified}})
	if len(rows) != 1 || !rows[0].Success {
		t.Fatalf("ledger %+v", rows)
	}
}

func TestExecuteRollsBackFailedCanary(t *testing.T) {
	p, mem, ad, ledger := setup(t, 10, 30)
	// Canary is hosts 1..3; host 2 fails to apply, so 1/3 > 10% -> rollback.
	ad.failHosts[2] = true
	reg := native.NewRegistry()
	reg.MustRegister(ad)
	ex := New(mem, reg, ledger)
	rep, err := ex.Execute(context.Background(), p, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Status != plan.StatusRolledBack || !rep.Waves[0].RolledBack || len(rep.Waves) != 1 {
		t.Fatalf("expected rollback after canary: %+v", rep)
	}
	if ad.state[1] || ad.state[3] {
		t.Fatal("changed canary hosts should be reverted")
	}
	if len(ad.reverts) != 2 {
		t.Fatalf("reverts %v", ad.reverts)
	}
	if ad.state[4] {
		t.Fatal("later waves must not run after a rollback")
	}
	rows, _ := ledger.Query(learn.Filter{PlanID: p.ID, Kinds: []learn.Kind{learn.KindRolledBack}})
	if len(rows) != 1 {
		t.Fatal("rollback should be in the ledger")
	}
}

func TestExecuteFailsVerificationRollsBack(t *testing.T) {
	p, mem, ad, ledger := setup(t, 4, 50)
	reg := native.NewRegistry()
	reg.MustRegister(ad)
	// The device claims success but the verification query disagrees.
	mem.Respond("iptables", func(device.Host) []map[string]string { return nil })
	ex := New(mem, reg, ledger)
	rep, err := ex.Execute(context.Background(), p, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Status != plan.StatusRolledBack {
		t.Fatalf("expected rollback on failed verification, got %s (%s)", rep.Status, rep.Summary)
	}
}

func TestDryRunReportsDrift(t *testing.T) {
	p, mem, ad, ledger := setup(t, 5, 20)
	ad.state[1], ad.state[2] = true, true
	reg := native.NewRegistry()
	reg.MustRegister(ad)
	ex := New(mem, reg, ledger)
	rep, err := ex.Execute(context.Background(), p, Options{DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Drift) != 3 || rep.Drift[0] != 3 {
		t.Fatalf("drift %v", rep.Drift)
	}
	if ad.state[3] {
		t.Fatal("dry run must not apply")
	}
	if len(mem.Policies) != 0 {
		t.Fatal("dry run must not persist policies")
	}
}

func TestExecuteRefusesUnapprovedAndUsesPauseHook(t *testing.T) {
	p, mem, ad, ledger := setup(t, 10, 20)
	p.Risk.Tier = risk.TierApprove
	reg := native.NewRegistry()
	reg.MustRegister(ad)
	ex := New(mem, reg, ledger)
	if _, err := ex.Execute(context.Background(), p, Options{}); !errors.Is(err, ErrNotExecutable) {
		t.Fatalf("expected ErrNotExecutable, got %v", err)
	}
	p.Approval = &plan.Approval{By: "bob", At: time.Now(), Channel: "cli"}
	paused := 0
	rep, err := ex.Execute(context.Background(), p, Options{PauseHook: func(_ context.Context, w WaveResult) (bool, error) {
		paused++
		return false, nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	if paused != 1 || len(rep.Waves) != 1 || rep.Summary != "paused after canary" {
		t.Fatalf("pause hook not honoured: paused=%d waves=%d summary=%q", paused, len(rep.Waves), rep.Summary)
	}
}

func TestMissingAdapterAndScriptFallback(t *testing.T) {
	hosts := []device.Host{{ID: 1, Platform: intent.PlatformDarwin, Fleet: "Eng", Online: true}}
	mem := substrate.NewMemory(hosts...)
	pl := plan.New(capability.Default(), risk.DefaultPolicy())
	in := &intent.Intent{ID: "i", Text: "run a script", Author: "a", Scope: intent.Scope{Fleets: []string{"Eng"}},
		Desired: []intent.Predicate{{Capability: "script.run", Params: map[string]any{"script": "echo hi"}}}, Provenance: intent.Provenance{Confidence: 1}}
	p, err := pl.Compile(in, hosts, plan.Options{FleetSize: 1})
	if err != nil {
		t.Fatal(err)
	}
	p.Approval = &plan.Approval{By: "bob", At: time.Now(), Channel: "cli"}
	ex := New(mem, native.NewRegistry(), learn.NewMemoryLedger())
	rep, err := ex.Execute(context.Background(), p, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(mem.Scripts) != 1 || rep.Status != plan.StatusVerified {
		t.Fatalf("script fallback not used: %+v %s", mem.Scripts, rep.Status)
	}

	// A native capability with no registered adapter fails cleanly.
	in2 := &intent.Intent{ID: "i2", Text: "fw", Author: "a", Scope: intent.Scope{Fleets: []string{"Eng"}},
		Desired: []intent.Predicate{{Capability: "firewall.enable"}}, Provenance: intent.Provenance{Confidence: 1}}
	p2, _ := pl.Compile(in2, hosts, plan.Options{FleetSize: 1})
	rep2, err := ex.Execute(context.Background(), p2, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if rep2.Status != plan.StatusRolledBack || rep2.Waves[0].Results[0].Error == "" {
		t.Fatalf("expected clean failure: %+v", rep2)
	}
	_ = fmt.Sprint
}
