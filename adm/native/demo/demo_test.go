package demo

import (
	"context"
	"testing"

	"github.com/fleetdm/fleet/v4/adm/capability"
	"github.com/fleetdm/fleet/v4/adm/native"
	"github.com/fleetdm/fleet/v4/adm/plan"
	"github.com/fleetdm/fleet/v4/adm/reason"
	"github.com/fleetdm/fleet/v4/adm/risk"
	"github.com/fleetdm/fleet/v4/adm/service"
	"github.com/fleetdm/fleet/v4/adm/substrate"
)

func TestDemoEndToEnd(t *testing.T) {
	cat := capability.Default()
	mem := substrate.NewMemory(substrate.DemoFleet()...)
	reg := native.NewRegistry()
	world := NewWorld()
	Install(reg, mem, cat, world)
	if reg.Len() < 40 {
		t.Fatalf("expected adapters for every binding, got %d", reg.Len())
	}
	svc := service.New(service.Config{Catalog: cat, Substrate: mem, Adapters: reg})
	ctx := context.Background()
	prop, err := svc.Propose(ctx, reason.Request{Text: "block usb drives on finance laptops automatically", Author: "alice"})
	if err != nil {
		t.Fatal(err)
	}
	rep, err := svc.Apply(ctx, prop.Plan.ID, "alice", false)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Status != plan.StatusVerified {
		t.Fatalf("%s: %s", rep.Status, rep.Summary)
	}
	if world.Snapshot()["storage.removable.block"] != 1 { // finance laptops: win-carol
		t.Fatalf("world %v", world.Snapshot())
	}
	// A flaky host makes the canary fail and roll back.
	world.Flaky[1] = true
	prop2, err := svc.Propose(ctx, reason.Request{Text: "make external drives read-only on engineering macs, roll out gradually", Author: "alice"})
	if err != nil {
		t.Fatal(err)
	}
	if prop2.Plan.Risk.Tier != risk.TierCanary {
		t.Fatalf("tier %s", prop2.Plan.Risk.Tier)
	}
	rep2, err := svc.Apply(ctx, prop2.Plan.ID, "alice", false)
	if err != nil {
		t.Fatal(err)
	}
	if rep2.Status != plan.StatusRolledBack {
		t.Fatalf("expected rollback, got %s: %s", rep2.Status, rep2.Summary)
	}
}
