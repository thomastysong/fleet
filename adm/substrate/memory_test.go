package substrate

import (
	"context"
	"testing"

	"github.com/fleetdm/fleet/v4/adm/device"
	"github.com/fleetdm/fleet/v4/adm/intent"
)

func TestMemorySubstrate(t *testing.T) {
	m := NewMemory(DemoFleet()...)
	ctx := context.Background()
	hosts, err := m.ListHosts(ctx, intent.Scope{Labels: []string{"engineering"}})
	if err != nil || len(hosts) != 4 {
		t.Fatalf("%v %v", hosts, err)
	}
	n, _ := m.CountHosts(ctx)
	if n != 8 {
		t.Fatalf("count %d", n)
	}
	m.Respond("disk_encryption", func(h device.Host) []map[string]string {
		if h.Platform == intent.PlatformDarwin {
			return []map[string]string{{"1": "1"}}
		}
		return nil
	})
	res, err := m.LiveQuery(ctx, "SELECT 1 FROM disk_encryption", []uint{1, 4, 5, 99})
	if err != nil {
		t.Fatal(err)
	}
	if res.Responded != 2 || len(res.RowsFor(1)) != 1 || len(res.RowsFor(5)) != 0 || res.Errors[99] == "" {
		t.Fatalf("%+v", res)
	}
	if err := m.UpsertPolicy(ctx, Policy{Name: "p", Query: "q1"}); err != nil {
		t.Fatal(err)
	}
	if err := m.UpsertPolicy(ctx, Policy{Name: "p", Query: "q2"}); err != nil {
		t.Fatal(err)
	}
	if len(m.Policies) != 1 || m.Policies[0].Query != "q2" {
		t.Fatalf("upsert failed: %+v", m.Policies)
	}
	if _, err := m.RunScript(ctx, 1, "echo"); err != nil || len(m.Scripts) != 1 {
		t.Fatal("script not recorded")
	}
	if err := m.Nudge(ctx, []uint{1}); err != nil || len(m.Nudged) != 1 {
		t.Fatal("nudge not recorded")
	}
}
