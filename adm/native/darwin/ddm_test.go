package darwin

import (
	"context"
	"testing"
	"time"

	"github.com/fleetdm/fleet/v4/adm/native"
)

type fakePusher struct {
	active map[string]Declaration
}

func (f *fakePusher) PushDeclaration(_ context.Context, _ native.Target, d Declaration) error {
	f.active[d.Identifier] = d
	return nil
}
func (f *fakePusher) RemoveDeclaration(_ context.Context, _ native.Target, id string) error {
	delete(f.active, id)
	return nil
}
func (f *fakePusher) DeclarationStatus(_ context.Context, _ native.Target, id string) (string, error) {
	if _, ok := f.active[id]; ok {
		return "active", nil
	}
	return "unknown", nil
}

func TestBuilders(t *testing.T) {
	d, err := DiskManagement("block")
	if err != nil || d.Type != "com.apple.configuration.diskmanagement.settings" || d.Payload["Restrictions"].(map[string]any)["ExternalStorage"] != "Disallowed" {
		t.Fatalf("%+v %v", d, err)
	}
	if _, err := DiskManagement("maybe"); err == nil {
		t.Fatal("expected error")
	}
	s, err := ScreenSaver(10)
	if err != nil || s.Payload["LoginWindowIdleTime"] != 600 {
		t.Fatalf("%+v %v", s, err)
	}
	u, err := SoftwareUpdateEnforcement("15.6", time.Date(2026, 9, 12, 9, 0, 0, 0, time.UTC))
	if err != nil || u.Payload["TargetLocalDateTime"] != "2026-09-12T09:00:00" {
		t.Fatalf("%+v %v", u, err)
	}
	w, err := WebContentFilter([]string{"chat.openai.com"})
	if err != nil || len(w.Payload["DenyListURLs"].([]string)) != 2 {
		t.Fatalf("%+v %v", w, err)
	}
	// Identifiers are content-addressed and stable.
	d2, _ := DiskManagement("block")
	if d.Identifier != d2.Identifier {
		t.Fatal("identifier should be stable")
	}
	ro, _ := DiskManagement("read_only")
	if ro.Identifier == d.Identifier {
		t.Fatal("identifier should change with payload")
	}
}

func TestDDMAdapterLifecycle(t *testing.T) {
	p := &fakePusher{active: map[string]Declaration{}}
	reg := native.NewRegistry()
	reg.MustRegister(NewAdapters(p)...)
	a, ok := reg.Lookup("storage.removable.block", "darwin")
	if !ok {
		t.Fatal("adapter missing")
	}
	ctx := context.Background()
	params := map[string]any{"mode": "read_only"}
	st, err := a.Check(ctx, native.Target{HostID: 1}, params)
	if err != nil || st.IsSatisfied() {
		t.Fatalf("before: %v %v", st, err)
	}
	res, err := a.Apply(ctx, native.Target{HostID: 1}, params)
	if err != nil || !res.Changed {
		t.Fatalf("apply: %+v %v", res, err)
	}
	st, _ = a.Check(ctx, native.Target{HostID: 1}, params)
	if !st.IsSatisfied() {
		t.Fatalf("after: %v", st)
	}
	if _, err := a.Revert(ctx, native.Target{HostID: 1}, params, st); err != nil {
		t.Fatal(err)
	}
	if len(p.active) != 0 {
		t.Fatal("revert should remove the declaration")
	}
	if _, err := a.Apply(ctx, native.Target{}, map[string]any{"mode": "nope"}); err == nil {
		t.Fatal("expected param error")
	}
}
