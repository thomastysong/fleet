package device

import (
	"testing"

	"github.com/fleetdm/fleet/v4/adm/intent"
)

func TestNormalizePlatform(t *testing.T) {
	cases := map[string]intent.Platform{"ubuntu": intent.PlatformLinux, "rhel": intent.PlatformLinux, "darwin": intent.PlatformDarwin, "Windows": intent.PlatformWindows, "ipados": intent.PlatformIPadOS, "chrome": intent.PlatformChromeOS}
	for in, want := range cases {
		if got := NormalizePlatform(in); got != want {
			t.Errorf("%s -> %s want %s", in, got, want)
		}
	}
}

func TestFilter(t *testing.T) {
	hosts := []Host{
		{ID: 1, Platform: intent.PlatformDarwin, Fleet: "Engineering", Labels: []string{"laptops", "m1"}},
		{ID: 2, Platform: intent.PlatformWindows, Fleet: "Finance", Labels: []string{"laptops"}},
		{ID: 3, Platform: intent.PlatformLinux, Fleet: "Engineering", Labels: []string{"servers"}},
		{ID: 4, Platform: intent.PlatformDarwin, Fleet: "Engineering", Labels: []string{"laptops", "exec"}},
	}
	got := Filter(hosts, intent.Scope{Fleets: []string{"engineering"}, Labels: []string{"laptops"}, ExcludeLabels: []string{"exec"}})
	if len(got) != 1 || got[0].ID != 1 {
		t.Fatalf("got %+v", got)
	}
	got = Filter(hosts, intent.Scope{Platforms: []intent.Platform{intent.PlatformWindows, intent.PlatformLinux}})
	if len(got) != 2 {
		t.Fatalf("got %+v", got)
	}
	got = Filter(hosts, intent.Scope{HostIDs: []uint{4}})
	if len(got) != 1 || got[0].ID != 4 {
		t.Fatalf("got %+v", got)
	}
	if len(Filter(hosts, intent.Scope{})) != 4 {
		t.Fatal("empty scope should match all")
	}
	if p := Platforms(hosts); len(p) != 3 || p[0] != intent.PlatformDarwin {
		t.Fatalf("platforms %v", p)
	}
	if ids := IDs(Filter(hosts, intent.Scope{Fleets: []string{"Engineering"}})); len(ids) != 3 || ids[2] != 4 {
		t.Fatalf("ids %v", ids)
	}
}
