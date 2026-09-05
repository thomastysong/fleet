package reason

import (
	"context"
	"errors"
	"testing"

	"github.com/fleetdm/fleet/v4/adm/capability"
	"github.com/fleetdm/fleet/v4/adm/intent"
)

func compile(t *testing.T, text string) *Result {
	t.Helper()
	d := NewDeterministic(capability.Default())
	res, err := d.Compile(context.Background(), Request{Text: text, Author: "alice", Fleets: []string{"Engineering", "Finance"}, Labels: []string{"laptops", "servers"}})
	if err != nil {
		t.Fatalf("%q: %v", text, err)
	}
	if err := ValidateAgainstCatalog(res.Intent, capability.Default()); err != nil {
		t.Fatalf("%q: catalog validation: %v", text, err)
	}
	return res
}

func caps(in *intent.Intent) []string {
	var out []string
	for _, d := range in.Desired {
		out = append(out, d.Capability)
	}
	return out
}

func TestSecurityBaselineInPlainLanguage(t *testing.T) {
	res := compile(t, "I want my fleet to be secure")
	got := caps(res.Intent)
	if len(got) != 3 || got[0] != "disk.encryption.enforce" || got[1] != "firewall.enable" || got[2] != "screen.lock.enforce" {
		t.Fatalf("got %v", got)
	}
	if !res.Intent.Constraints.AllowFleetWide {
		t.Fatal("'my fleet' should mean fleet-wide")
	}
}

func TestRemovableStorageOnFinanceWindows(t *testing.T) {
	res := compile(t, "Block USB drives on Finance Windows laptops, ask me before you apply it")
	in := res.Intent
	if len(in.Desired) != 1 || in.Desired[0].Capability != "storage.removable.block" || in.Desired[0].Params["mode"] != "block" {
		t.Fatalf("%+v", in.Desired)
	}
	if len(in.Scope.Fleets) != 1 || in.Scope.Fleets[0] != "Finance" || len(in.Scope.Labels) != 1 || in.Scope.Labels[0] != "laptops" {
		t.Fatalf("scope %+v", in.Scope)
	}
	if len(in.Scope.Platforms) != 1 || in.Scope.Platforms[0] != intent.PlatformWindows {
		t.Fatalf("platforms %v", in.Scope.Platforms)
	}
	if in.Constraints.MaxAutonomy != intent.AutonomyApprove {
		t.Fatalf("autonomy %s", in.Constraints.MaxAutonomy)
	}
	ro := compile(t, "make external drives read-only on macs everywhere")
	if ro.Intent.Desired[0].Params["mode"] != "read_only" || ro.Intent.Scope.Platforms[0] != intent.PlatformDarwin {
		t.Fatalf("%+v", ro.Intent)
	}
}

func TestAIGovernance(t *testing.T) {
	res := compile(t, "No ChatGPT or Gemini on Engineering machines, Claude is allowed")
	in := res.Intent
	if len(in.Desired) != 1 || in.Desired[0].Capability != "ai.tools.govern" {
		t.Fatalf("%+v", in.Desired)
	}
	block := in.Desired[0].Params["block"].([]string)
	if len(block) != 3 { // the rule blocks every mentioned tool; allow-listing is a clarification for the LLM compiler
		t.Fatalf("block %v", block)
	}
	if _, hasWeb := in.Desired[0].Params["block_providers"]; !hasWeb {
		t.Fatal("providers should be blocked too")
	}
	if len(in.Scope.Fleets) != 1 || in.Scope.Fleets[0] != "Engineering" {
		t.Fatalf("scope %+v", in.Scope)
	}
	vague := compile(t, "block ai agents on all laptops")
	if len(vague.Clarifications) == 0 {
		t.Fatal("expected a clarifying question about which tools")
	}
}

func TestWebBlockAndSoftwareFloor(t *testing.T) {
	res := compile(t, "block facebook.com and tiktok.com on all devices")
	if res.Intent.Desired[0].Capability != "web.block" || len(res.Intent.Desired[0].Params["domains"].([]string)) != 2 {
		t.Fatalf("%+v", res.Intent.Desired)
	}
	sw := compile(t, "Keep Chrome updated, never more than 1 version behind, on every device, roll it out gradually")
	if sw.Intent.Desired[0].Capability != "software.version.floor" || sw.Intent.Desired[0].Params["max_versions_behind"] != 1 || sw.Intent.Desired[0].Params["title"] != "Google Chrome" {
		t.Fatalf("%+v", sw.Intent.Desired)
	}
	if sw.Intent.Constraints.MaxAutonomy != intent.AutonomyCanary {
		t.Fatalf("autonomy %s", sw.Intent.Constraints.MaxAutonomy)
	}
	pkg := compile(t, "upgrade outdated npm and pip packages with CVEs on engineering macs")
	if pkg.Intent.Desired[0].Capability != "package.manager.upgrade" || pkg.Intent.Desired[0].Params["only_vulnerable"] != true {
		t.Fatalf("%+v", pkg.Intent.Desired)
	}
}

func TestOSUpdateAndLifecycle(t *testing.T) {
	res := compile(t, "all macs must run macOS 15.6 or newer within 14 days, update them")
	d := res.Intent.Desired[0]
	if d.Capability != "os.update.enforce" || d.Params["min_version"] != "15.6" || d.Params["deadline_days"] != 14 {
		t.Fatalf("%+v", d)
	}
	wipe := compile(t, "wipe the device with serial C02XA1 in Finance")
	if wipe.Intent.Desired[0].Capability != "host.wipe" {
		t.Fatalf("%+v", wipe.Intent.Desired)
	}
	lock := compile(t, "lock the device mbp-alice in Engineering")
	if lock.Intent.Desired[0].Capability != "host.lock" {
		t.Fatalf("%+v", lock.Intent.Desired)
	}
	obs := compile(t, "just watch for USB drives on all laptops, report only")
	if obs.Intent.Desired[0].Capability != "storage.removable.audit" || obs.Intent.Constraints.MaxAutonomy != intent.AutonomyObserve {
		t.Fatalf("%+v", obs.Intent)
	}
}

func TestNoMatchAndScopeClarification(t *testing.T) {
	d := NewDeterministic(capability.Default())
	if _, err := d.Compile(context.Background(), Request{Text: "make me a sandwich"}); !errors.Is(err, ErrNoMatch) {
		t.Fatalf("expected ErrNoMatch, got %v", err)
	}
	res, err := d.Compile(context.Background(), Request{Text: "turn on the firewall"})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Clarifications) == 0 || res.Intent.Provenance.Confidence >= 0.7 {
		t.Fatalf("scope-less ask should ask which devices and lower confidence: %+v %v", res.Clarifications, res.Intent.Provenance)
	}
}

type failing struct{}

func (failing) Name() string { return "failing" }
func (failing) Compile(context.Context, Request) (*Result, error) {
	return nil, errors.New("model unavailable")
}

func TestChainFallsBack(t *testing.T) {
	c := &Chain{Compilers: []Compiler{failing{}, NewDeterministic(capability.Default())}}
	res, err := c.Compile(context.Background(), Request{Text: "enable the firewall everywhere"})
	if err != nil || res.Intent.Provenance.Compiler != "deterministic" {
		t.Fatalf("%v %+v", err, res)
	}
	c2 := &Chain{Compilers: []Compiler{failing{}}}
	if _, err := c2.Compile(context.Background(), Request{Text: "x"}); err == nil {
		t.Fatal("expected joined error")
	}
}
