package risk

import (
	"testing"
	"time"

	"github.com/fleetdm/fleet/v4/adm/intent"
)

func TestScoreMonotonic(t *testing.T) {
	base := Factors{BaseRisk: 10, BlastRadius: 10, FleetSize: 1000, Reversible: true, Confidence: 1}
	s0, _ := Score(base)
	big := base
	big.BlastRadius = 900
	s1, _ := Score(big)
	if s1 <= s0 {
		t.Fatalf("bigger blast radius should score higher: %d vs %d", s1, s0)
	}
	irr := base
	irr.Reversible = false
	if s2, _ := Score(irr); s2 <= s0 {
		t.Fatal("irreversible should score higher")
	}
	des := base
	des.Destructive = true
	if s3, _ := Score(des); s3 <= s0+25 {
		t.Fatal("destructive should add a large penalty")
	}
	scr := base
	scr.UsesScript = true
	if s4, _ := Score(scr); s4 <= s0 {
		t.Fatal("scripts should score higher than native")
	}
	if s, _ := Score(Factors{BaseRisk: 40, Destructive: true, BlastRadius: 100000, UserImpact: intent.ImpactRestart}); s != 100 {
		t.Fatalf("score should clamp at 100, got %d", s)
	}
}

func TestAssessTiersByScore(t *testing.T) {
	p := DefaultPolicy()
	low := p.Assess(Input{Factors: Factors{BaseRisk: 4, BlastRadius: 5, FleetSize: 1000, Reversible: true, Confidence: 1}, Capabilities: []string{"screen.lock.enforce"}})
	if low.Tier != TierAuto {
		t.Fatalf("low-risk small change should be auto, got %s (%v)", low.Tier, low.Reasons)
	}
	wide := p.Assess(Input{Factors: Factors{BaseRisk: 4, BlastRadius: 900, FleetSize: 1000, Reversible: true, Confidence: 1}, Capabilities: []string{"screen.lock.enforce"}})
	if wide.Tier < TierCanary {
		t.Fatalf("fleet-wide change should be at least canary, got %s (%v)", wide.Tier, wide.Reasons)
	}
	wipe := p.Assess(Input{Factors: Factors{BaseRisk: 40, BlastRadius: 1, FleetSize: 1000, Destructive: true, Confidence: 1}, Capabilities: []string{"host.wipe"}})
	if wipe.Tier != TierCAB {
		t.Fatalf("wipe should be CAB, got %s", wipe.Tier)
	}
}

func TestFloorsAndCaps(t *testing.T) {
	p := DefaultPolicy()
	// identity.* floor is approve even for a tiny, reversible change.
	a := p.Assess(Input{Factors: Factors{BaseRisk: 2, BlastRadius: 1, FleetSize: 10, Reversible: true, Confidence: 1}, Capabilities: []string{"identity.local_admin.manage"}})
	if a.Tier != TierApprove || a.Floor != TierApprove {
		t.Fatalf("expected approve floor, got tier=%s floor=%s", a.Tier, a.Floor)
	}
	// An intent cap can only make things more conservative.
	capTier := TierCAB
	b := p.Assess(Input{Factors: Factors{BaseRisk: 2, BlastRadius: 1, FleetSize: 10, Reversible: true, Confidence: 1}, Capabilities: []string{"firewall.enable"}, Cap: &capTier})
	if b.Tier != TierCAB {
		t.Fatalf("cap should raise tier to CAB, got %s", b.Tier)
	}
	obs := TierObserve
	c := p.Assess(Input{Factors: Factors{BaseRisk: 2, BlastRadius: 1, FleetSize: 10, Reversible: true, Confidence: 1}, Capabilities: []string{"firewall.enable"}, Cap: &obs})
	if c.Tier != TierObserve {
		t.Fatalf("observe cap should force observe, got %s", c.Tier)
	}
}

func TestLeashRelaxesProvenPatternsButNotAfterIncident(t *testing.T) {
	p := DefaultPolicy()
	now := time.Now()
	f := Factors{BaseRisk: 12, BlastRadius: 300, FleetSize: 1000, Reversible: true, Confidence: 0.9, Platforms: 2}
	fresh := p.Assess(Input{Factors: f, Capabilities: []string{"web.block"}, Leash: &LeashState{Pattern: "web.block@darwin,windows"}, Now: now})
	if fresh.Tier < TierCanary {
		t.Fatalf("novel wide change should not be auto, got %s (%v)", fresh.Tier, fresh.Reasons)
	}
	proven := &LeashState{Pattern: "web.block@darwin,windows", Accepted: 12, Verified: 12}
	relaxed := p.Assess(Input{Factors: f, Capabilities: []string{"web.block"}, Leash: proven, Now: now})
	if relaxed.Tier >= fresh.Tier {
		t.Fatalf("proven pattern should relax: fresh=%s relaxed=%s (%v)", fresh.Tier, relaxed.Tier, relaxed.Reasons)
	}
	if relaxed.LeashRelaxation == 0 {
		t.Fatal("expected leash relaxation to be recorded")
	}
	burned := &LeashState{Pattern: "web.block@darwin,windows", Accepted: 12, Verified: 12, Incidents: 1, LastIncident: now.Add(-24 * time.Hour)}
	tight := p.Assess(Input{Factors: f, Capabilities: []string{"web.block"}, Leash: burned, Now: now})
	if tight.Tier != fresh.Tier {
		t.Fatalf("recent incident should suspend relaxation: got %s want %s", tight.Tier, fresh.Tier)
	}
	// Destructive changes never relax.
	wipe := p.Assess(Input{Factors: Factors{BaseRisk: 40, BlastRadius: 1, FleetSize: 10, Destructive: true, Confidence: 1}, Capabilities: []string{"host.wipe"}, Leash: proven, Now: now})
	if wipe.Tier != TierCAB {
		t.Fatalf("destructive must stay CAB, got %s", wipe.Tier)
	}
}

func TestLeashNovelty(t *testing.T) {
	if n := (LeashState{}).Novelty(); n != 1 {
		t.Fatalf("unseen pattern novelty = %v", n)
	}
	if n := (LeashState{Verified: 30}).Novelty(); n > 0.1 {
		t.Fatalf("well-proven pattern novelty too high: %v", n)
	}
	l := LeashState{Accepted: 10, Verified: 10, Failed: 5}
	if l.Relaxation(DefaultPolicy(), time.Now()) != 0 {
		t.Fatal("poor verification record should suspend relaxation")
	}
}

func TestInvariants(t *testing.T) {
	p := DefaultPolicy()
	v := p.Evaluate([]Action{
		{Capability: "disk.encryption.enforce", Params: map[string]any{"enabled": false}, Targets: 1, Tier: TierAuto},
		{Capability: "host.wipe", Targets: 3, Tier: TierApprove},
		{Capability: "script.run", Params: map[string]any{"script": "launchctl unload /Library/LaunchDaemons/com.fleetdm.orbit.plist"}, Targets: 1, Tier: TierApprove},
		{Capability: "identity.local_admin.manage", Params: map[string]any{"username": "Administrator"}, Targets: 1, Tier: TierApprove},
		{Capability: "script.run", Params: map[string]any{"script": "echo hi"}, Targets: 500, Tier: TierApprove},
	})
	got := map[string]bool{}
	for _, x := range v {
		got[x.Invariant] = true
	}
	for _, want := range []string{"never-disable-encryption", "wipe-requires-cab", "wipe-single-host", "never-remove-agent", "protected-accounts", "no-fleet-wide-scripts"} {
		if !got[want] {
			t.Errorf("missing violation %s (got %v)", want, v)
		}
	}
	if len(p.Evaluate([]Action{{Capability: "firewall.enable", Targets: 10, Tier: TierAuto}})) != 0 {
		t.Fatal("benign action should not violate")
	}
}

func TestTierText(t *testing.T) {
	var tier Tier
	if err := tier.UnmarshalText([]byte("canary")); err != nil || tier != TierCanary {
		t.Fatalf("unmarshal: %v %s", err, tier)
	}
	if err := tier.UnmarshalText([]byte("nope")); err == nil {
		t.Fatal("expected error")
	}
	if got, _ := TierFromAutonomy(intent.AutonomyCAB); got != TierCAB {
		t.Fatal("autonomy mapping wrong")
	}
}
