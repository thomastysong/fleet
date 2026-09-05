package capability

import (
	"testing"

	"github.com/fleetdm/fleet/v4/adm/intent"
)

func TestDefaultCatalogIsWellFormed(t *testing.T) {
	c := Default()
	if c.Len() < 20 {
		t.Fatalf("expected a rich catalog, got %d capabilities", c.Len())
	}
	for _, cap := range c.All() {
		if cap.Name == "" || cap.Summary == "" || cap.Domain == "" {
			t.Errorf("capability %+v missing name/summary/domain", cap)
		}
		if len(cap.Bindings) == 0 {
			t.Errorf("capability %s has no bindings", cap.Name)
		}
		if cap.BaseRisk < 0 || cap.BaseRisk > 40 {
			t.Errorf("capability %s base risk %d out of 0..40", cap.Name, cap.BaseRisk)
		}
		for p, b := range cap.Bindings {
			if !p.IsValid() {
				t.Errorf("capability %s: invalid platform %q", cap.Name, p)
			}
			if b.Mechanism == "" || b.Native == "" || b.Latency == "" {
				t.Errorf("capability %s/%s: incomplete binding %+v", cap.Name, p, b)
			}
		}
		for p, v := range cap.Verify {
			if !cap.Supports(p) {
				t.Errorf("capability %s: verify for unsupported platform %s", cap.Name, p)
			}
			if v.Expect != "rows>0" && v.Expect != "rows==0" {
				t.Errorf("capability %s/%s: bad expect %q", cap.Name, p, v.Expect)
			}
		}
		if cap.Destructive && cap.Reversible {
			t.Errorf("capability %s: destructive capabilities cannot be reversible", cap.Name)
		}
	}
}

func TestOnlyScriptRunUsesScripts(t *testing.T) {
	for _, cap := range Default().All() {
		for p, b := range cap.Bindings {
			if b.Mechanism == MechScript && cap.Name != "script.run" {
				t.Errorf("capability %s/%s uses a script binding; ADM bindings must be native", cap.Name, p)
			}
		}
	}
}

func TestXavierAndFleetParityCovered(t *testing.T) {
	c := Default()
	// The announcement in the design brief lists these Xavier differentiators.
	for _, name := range []string{
		"package.manager.upgrade", "dependency.cve.scan", "runtime.eol.flag",
		"software.release.watch", "storage.removable.block", "web.block",
		"ai.tools.govern", "signage.kiosk", "telemetry.query",
		"enrollment.autopilot.register", "enrollment.ade.assign", "enrollment.android.provision",
	} {
		if _, ok := c.Get(name); !ok {
			t.Errorf("catalog missing %s", name)
		}
	}
}

func TestValidateParams(t *testing.T) {
	c := Default()
	cap, _ := c.Get("screen.lock.enforce")
	if err := cap.ValidateParams(map[string]any{}); err == nil {
		t.Fatal("expected missing required param error")
	}
	if err := cap.ValidateParams(map[string]any{"idle_minutes": "ten"}); err == nil {
		t.Fatal("expected type error")
	}
	if err := cap.ValidateParams(map[string]any{"idle_minutes": 10}); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	web, _ := c.Get("web.block")
	if err := web.ValidateParams(map[string]any{"domains": []any{"a.example", "b.example"}}); err != nil {
		t.Fatalf("[]any of strings should validate: %v", err)
	}
	if err := web.ValidateParams(map[string]any{"domains": []any{1}}); err == nil {
		t.Fatal("expected list type error")
	}
}

func TestCatalogLookups(t *testing.T) {
	c := Default()
	names := c.Names()
	for i := 1; i < len(names); i++ {
		if names[i-1] >= names[i] {
			t.Fatal("names not sorted")
		}
	}
	if len(c.ByDomain(DomainAI)) == 0 {
		t.Fatal("expected AI domain capabilities")
	}
	wipe, _ := c.Get("host.wipe")
	if !wipe.Destructive || wipe.Supports(intent.PlatformChromeOS) {
		t.Fatal("host.wipe should be destructive and not support chromeos yet")
	}
	if got := wipe.Platforms(); len(got) != 5 {
		t.Fatalf("host.wipe platforms = %v", got)
	}
}
