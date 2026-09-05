package native

import (
	"context"
	"testing"

	"github.com/fleetdm/fleet/v4/adm/intent"
)

type stub struct{ cap string }

func (s stub) ID() string                { return "stub." + s.cap }
func (s stub) Capability() string        { return s.cap }
func (s stub) Platform() intent.Platform { return intent.PlatformLinux }
func (s stub) Check(context.Context, Target, map[string]any) (State, error) {
	return State{Satisfied: true}, nil
}
func (s stub) Apply(context.Context, Target, map[string]any) (Result, error) { return Result{}, nil }
func (s stub) Revert(context.Context, Target, map[string]any, State) (Result, error) {
	return Result{}, nil
}

func TestRegistry(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(stub{"a"}); err != nil {
		t.Fatal(err)
	}
	if err := r.Register(stub{"a"}); err == nil {
		t.Fatal("duplicate should fail")
	}
	if _, ok := r.Lookup("a", intent.PlatformLinux); !ok {
		t.Fatal("lookup failed")
	}
	if _, ok := r.Lookup("a", intent.PlatformDarwin); ok {
		t.Fatal("wrong platform matched")
	}
	if !(State{Satisfied: true}).IsSatisfied() || (State{}).IsSatisfied() {
		t.Fatal("IsSatisfied wrong")
	}
}
