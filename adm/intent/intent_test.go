package intent

import (
	"errors"
	"testing"
)

func valid() *Intent {
	return &Intent{
		ID:      "int-1",
		Text:    "keep all laptops encrypted",
		Author:  "alice",
		Source:  SourceNaturalLanguage,
		Scope:   Scope{Platforms: []Platform{PlatformDarwin, PlatformWindows, PlatformLinux}},
		Desired: []Predicate{{Capability: "disk.encryption.enforce"}},
	}
}

func TestValidateOK(t *testing.T) {
	if err := valid().Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateRejectsEmptyScope(t *testing.T) {
	in := valid()
	in.Scope = Scope{}
	err := in.Validate()
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("expected ErrInvalid, got %v", err)
	}
	in.Constraints.AllowFleetWide = true
	if err := in.Validate(); err != nil {
		t.Fatalf("fleet-wide opt-in should validate: %v", err)
	}
}

func TestValidateCollectsProblems(t *testing.T) {
	in := valid()
	in.Text = ""
	in.Desired = []Predicate{{Capability: ""}}
	in.Scope.Platforms = []Platform{"beos"}
	in.Constraints.MaxAutonomy = "yolo"
	in.Constraints.CanaryPercent = 200
	in.Verification = []Verification{{Query: "", Expect: "maybe"}}
	err := in.Validate()
	if err == nil {
		t.Fatal("expected error")
	}
	for _, want := range []string{"text is required", "capability is required", "unknown platform", "unknown autonomy", "canary_percent", "query is required", "expect must be"} {
		if !contains(err.Error(), want) {
			t.Errorf("error %q missing %q", err.Error(), want)
		}
	}
}

func TestFingerprintIsStableAndSemantic(t *testing.T) {
	a, b := valid(), valid()
	b.ID, b.Text, b.Author = "other", "different wording, same meaning", "bob"
	if a.Fingerprint() != b.Fingerprint() {
		t.Fatal("fingerprint should ignore id/text/author")
	}
	b.Desired[0].Params = map[string]any{"escrow": true}
	if a.Fingerprint() == b.Fingerprint() {
		t.Fatal("fingerprint should change with params")
	}
}

func TestPatternKey(t *testing.T) {
	in := valid()
	if got, want := in.PatternKey(), "disk.encryption.enforce@darwin,windows,linux"; got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	in.Scope.Platforms = nil
	if got, want := in.PatternKey(), "disk.encryption.enforce@any"; got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestPlatformHelpers(t *testing.T) {
	if !PlatformIOS.IsMobile() || PlatformLinux.IsMobile() {
		t.Fatal("IsMobile wrong")
	}
	if Platform("beos").IsValid() {
		t.Fatal("beos should be invalid")
	}
}

func contains(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
