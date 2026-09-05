package plan

import (
	"strings"
	"testing"
	"time"

	"github.com/fleetdm/fleet/v4/adm/capability"
	"github.com/fleetdm/fleet/v4/adm/device"
	"github.com/fleetdm/fleet/v4/adm/intent"
	"github.com/fleetdm/fleet/v4/adm/risk"
)

func fleet(n int) []device.Host {
	var hosts []device.Host
	plats := []intent.Platform{intent.PlatformDarwin, intent.PlatformWindows, intent.PlatformLinux}
	for i := 1; i <= n; i++ {
		hosts = append(hosts, device.Host{ID: uint(i), Hostname: "h", Platform: plats[i%3], Fleet: "Engineering", Labels: []string{"laptops"}, Online: true})
	}
	return hosts
}

func testPlanner() *Planner {
	p := New(capability.Default(), risk.DefaultPolicy())
	p.Now = func() time.Time { return time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC) }
	n := 0
	p.NewID = func() string { n++; return "plan-" + string(rune('a'+n)) }
	return p
}

func TestCompileCrossPlatform(t *testing.T) {
	in := &intent.Intent{ID: "i1", Text: "block removable storage on engineering laptops", Author: "alice",
		Scope:      intent.Scope{Fleets: []string{"Engineering"}, Labels: []string{"laptops"}},
		Desired:    []intent.Predicate{{Capability: "storage.removable.block", Params: map[string]any{"mode": "block"}}},
		Provenance: intent.Provenance{Confidence: 0.95}}
	p, err := testPlanner().Compile(in, fleet(30), Options{FleetSize: 1000})
	if err != nil {
		t.Fatal(err)
	}
	if p.Hosts != 30 || len(p.Steps) != 3 {
		t.Fatalf("hosts=%d steps=%d", p.Hosts, len(p.Steps))
	}
	for _, s := range p.Steps {
		if s.Mechanism == capability.MechScript {
			t.Errorf("step %s uses a script", s.ID)
		}
		if s.Verify == nil {
			t.Errorf("step %s has no verification", s.ID)
		}
	}
	if len(p.Rollback) != 3 || !p.Rollback[0].Revert {
		t.Fatalf("rollback %+v", p.Rollback)
	}
	if p.Risk.Tier != risk.TierAuto {
		t.Fatalf("30 hosts of 1000, reversible, native: expected auto got %s (%v)", p.Risk.Tier, p.Risk.Reasons)
	}
	if len(p.Waves) != 1 {
		t.Fatalf("expected a single wave for a small auto plan, got %+v", p.Waves)
	}
	if !strings.Contains(p.GitOps, "storage.removable.block") || !strings.Contains(p.GitOps, "policies:") {
		t.Fatalf("gitops rendering incomplete:\n%s", p.GitOps)
	}
	ok, why := p.Executable()
	if !ok {
		t.Fatalf("expected executable, got %s", why)
	}
}

func TestCompileLargeRolloutGetsCanaryAndWaves(t *testing.T) {
	in := &intent.Intent{ID: "i2", Text: "firewall on everywhere", Author: "alice",
		Scope:       intent.Scope{},
		Constraints: intent.Constraints{AllowFleetWide: true, CanaryPercent: 5, MaxWaveHosts: 200},
		Desired:     []intent.Predicate{{Capability: "firewall.enable"}},
		Provenance:  intent.Provenance{Confidence: 1}}
	p, err := testPlanner().Compile(in, fleet(1000), Options{FleetSize: 1000})
	if err != nil {
		t.Fatal(err)
	}
	if p.Risk.Tier < risk.TierCanary {
		t.Fatalf("fleet-wide should be at least canary, got %s (%v)", p.Risk.Tier, p.Risk.Reasons)
	}
	if p.Waves[0].Name != "canary" || len(p.Waves[0].Targets) != 50 || !p.Waves[0].Pause {
		t.Fatalf("canary wave wrong: %+v", p.Waves[0])
	}
	if len(p.Waves) != 1+5 { // 950 remaining / 200 = 5 waves
		t.Fatalf("expected 6 waves, got %d", len(p.Waves))
	}
	if p.Waves[len(p.Waves)-1].Pause {
		t.Fatal("last wave should not pause")
	}
	sim := Simulate(p)
	if sim.Hosts != 1000 || sim.Waves != 6 || sim.ByPlatform[intent.PlatformLinux] == 0 {
		t.Fatalf("sim %+v", sim)
	}
}

func TestCompileBlocksInvariantViolations(t *testing.T) {
	in := &intent.Intent{ID: "i3", Text: "wipe two laptops", Author: "alice",
		Scope:      intent.Scope{HostIDs: []uint{1, 2}},
		Desired:    []intent.Predicate{{Capability: "host.wipe"}},
		Provenance: intent.Provenance{Confidence: 1}}
	p, err := testPlanner().Compile(in, fleet(10), Options{FleetSize: 10})
	if err != nil {
		t.Fatal(err)
	}
	if p.Status != StatusBlocked || len(p.Violations) == 0 {
		t.Fatalf("expected blocked plan, got status=%s violations=%v", p.Status, p.Violations)
	}
	if p.Risk.Tier != risk.TierCAB {
		t.Fatalf("wipe must be CAB, got %s", p.Risk.Tier)
	}
	if ok, _ := p.Executable(); ok {
		t.Fatal("blocked plan must not be executable")
	}
}

func TestApprovalGate(t *testing.T) {
	in := &intent.Intent{ID: "i4", Text: "lock host 3", Author: "alice",
		Scope:      intent.Scope{HostIDs: []uint{3}},
		Desired:    []intent.Predicate{{Capability: "host.lock", Params: map[string]any{"message": "call IT"}}},
		Provenance: intent.Provenance{Confidence: 1}}
	p, err := testPlanner().Compile(in, fleet(10), Options{FleetSize: 10})
	if err != nil {
		t.Fatal(err)
	}
	if p.Risk.Tier != risk.TierApprove {
		t.Fatalf("host.lock floor is approve, got %s", p.Risk.Tier)
	}
	if ok, why := p.Executable(); ok || !strings.Contains(why, "requires approval") {
		t.Fatalf("expected approval gate, got ok=%v why=%q", ok, why)
	}
	p.Approval = &Approval{By: "bob", Channel: "cli"}
	if ok, why := p.Executable(); !ok {
		t.Fatalf("approved plan should execute: %s", why)
	}
}

func TestUnsupportedPlatformsReported(t *testing.T) {
	hosts := append(fleet(3), device.Host{ID: 99, Platform: intent.PlatformChromeOS, Fleet: "Engineering", Labels: []string{"laptops"}})
	in := &intent.Intent{ID: "i5", Text: "screen lock", Author: "alice",
		Scope:      intent.Scope{Fleets: []string{"Engineering"}},
		Desired:    []intent.Predicate{{Capability: "screen.lock.enforce", Params: map[string]any{"idle_minutes": 5}}},
		Provenance: intent.Provenance{Confidence: 1}}
	p, err := testPlanner().Compile(in, hosts, Options{FleetSize: 4})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Unsupported) != 1 || p.Unsupported[0].Platform != intent.PlatformChromeOS {
		t.Fatalf("unsupported %+v", p.Unsupported)
	}
	if p.Hosts != 3 {
		t.Fatalf("hosts %d", p.Hosts)
	}
}

func TestCompileRejectsUnknownCapabilityAndBadParams(t *testing.T) {
	pl := testPlanner()
	in := &intent.Intent{ID: "x", Text: "t", Author: "a", Scope: intent.Scope{HostIDs: []uint{1}}, Desired: []intent.Predicate{{Capability: "nope"}}}
	if _, err := pl.Compile(in, fleet(3), Options{}); err == nil {
		t.Fatal("expected unknown capability error")
	}
	in.Desired = []intent.Predicate{{Capability: "web.block"}}
	if _, err := pl.Compile(in, fleet(3), Options{}); err == nil {
		t.Fatal("expected missing param error")
	}
}

func TestRenderTemplate(t *testing.T) {
	got := RenderTemplate("SELECT 1 FROM ai_tools WHERE name IN ({{.block_list}}) AND x = '{{.title}}'", map[string]any{"block": []any{"claude-code", "o'malley"}, "title": "Chrome"})
	want := "SELECT 1 FROM ai_tools WHERE name IN ('claude-code','o''malley') AND x = 'Chrome'"
	if got != want {
		t.Fatalf("got %q", got)
	}
}

func TestGitOpsProjectsControlsAndPolicies(t *testing.T) {
	in := &intent.Intent{ID: "i6", Text: "encrypt everything", Author: "alice",
		Scope:      intent.Scope{Platforms: []intent.Platform{intent.PlatformDarwin, intent.PlatformWindows}},
		Desired:    []intent.Predicate{{Capability: "disk.encryption.enforce", Rationale: "SOC 2 CC6.1"}},
		Provenance: intent.Provenance{Confidence: 1}}
	p, err := testPlanner().Compile(in, fleet(6), Options{FleetSize: 6})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"enable_disk_encryption: true", "ADM disk.encryption.enforce (darwin)", "platform: windows", "SOC 2 CC6.1", "max_autonomy"} {
		if !strings.Contains(p.GitOps, want) && want != "max_autonomy" {
			t.Errorf("gitops missing %q:\n%s", want, p.GitOps)
		}
	}
}
