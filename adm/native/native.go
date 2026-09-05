// Package native defines the adapter contract for realising a capability on
// a device through the operating system's own interfaces (CSPs, WMI, DDM,
// Endpoint Security, D-Bus, udev, netlink) rather than through scripts.
//
// Every adapter is idempotent and reversible by contract: Check observes,
// Apply converges, Revert restores a prior State. The executor never calls
// Apply without a preceding Check, so a device that already satisfies the
// intent is left untouched and reported as such.
package native

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/fleetdm/fleet/v4/adm/intent"
)

// Target identifies the device an adapter acts on.
type Target struct {
	HostID   uint
	UUID     string
	Hostname string
	Platform intent.Platform
}

// State is an adapter's observation of the device, keyed by setting name.
// It must contain enough to Revert.
type State map[string]any

// Satisfied is the conventional key an adapter sets to true when the
// observed state already meets the desired parameters.
const Satisfied = "satisfied"

// IsSatisfied reports whether the state meets the desired parameters.
func (s State) IsSatisfied() bool {
	v, _ := s[Satisfied].(bool)
	return v
}

// Result is the outcome of Apply or Revert.
type Result struct {
	Changed  bool           `json:"changed"`
	Native   string         `json:"native"` // the concrete call that was made
	Evidence map[string]any `json:"evidence,omitempty"`
	Duration time.Duration  `json:"duration"`
}

// Adapter realises one capability on one platform.
type Adapter interface {
	ID() string
	Capability() string
	Platform() intent.Platform
	Check(ctx context.Context, t Target, params map[string]any) (State, error)
	Apply(ctx context.Context, t Target, params map[string]any) (Result, error)
	Revert(ctx context.Context, t Target, params map[string]any, prior State) (Result, error)
}

// Registry indexes adapters by capability and platform.
type Registry struct {
	mu sync.RWMutex
	m  map[string]Adapter
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry { return &Registry{m: map[string]Adapter{}} }

func key(capability string, p intent.Platform) string { return capability + "@" + string(p) }

// Register adds an adapter; a duplicate capability/platform pair is an error.
func (r *Registry) Register(a Adapter) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	k := key(a.Capability(), a.Platform())
	if _, dup := r.m[k]; dup {
		return fmt.Errorf("adapter already registered for %s", k)
	}
	r.m[k] = a
	return nil
}

// MustRegister registers or panics.
func (r *Registry) MustRegister(as ...Adapter) {
	for _, a := range as {
		if err := r.Register(a); err != nil {
			panic(err)
		}
	}
}

// Lookup returns the adapter for a capability on a platform.
func (r *Registry) Lookup(capability string, p intent.Platform) (Adapter, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	a, ok := r.m[key(capability, p)]
	return a, ok
}

// Len returns the number of registered adapters.
func (r *Registry) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.m)
}
