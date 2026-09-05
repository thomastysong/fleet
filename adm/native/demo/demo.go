// Package demo provides in-memory adapters for every catalog binding and a
// verification responder for the in-memory substrate, so the whole control
// plane can be exercised end to end without a device: `admctl --demo`.
package demo

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/fleetdm/fleet/v4/adm/capability"
	"github.com/fleetdm/fleet/v4/adm/device"
	"github.com/fleetdm/fleet/v4/adm/intent"
	"github.com/fleetdm/fleet/v4/adm/native"
	"github.com/fleetdm/fleet/v4/adm/plan"
	"github.com/fleetdm/fleet/v4/adm/substrate"
)

// World is the simulated device state shared by the adapters and the
// substrate responder.
type World struct {
	mu sync.Mutex
	// applied maps capability -> host -> params last applied.
	applied map[string]map[uint]map[string]any
	// Flaky lists hosts whose Apply fails, to demonstrate rollback.
	Flaky map[uint]bool
	// Latency is added to every Apply to make timing visible.
	Latency time.Duration
}

// NewWorld returns an empty world.
func NewWorld() *World {
	return &World{applied: map[string]map[uint]map[string]any{}, Flaky: map[uint]bool{}}
}

func (w *World) get(cap string, host uint) (map[string]any, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	p, ok := w.applied[cap][host]
	return p, ok
}

func (w *World) set(cap string, host uint, params map[string]any) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.applied[cap] == nil {
		w.applied[cap] = map[uint]map[string]any{}
	}
	if params == nil {
		delete(w.applied[cap], host)
		return
	}
	w.applied[cap][host] = params
}

// Snapshot returns capability -> number of hosts converged.
func (w *World) Snapshot() map[string]int {
	w.mu.Lock()
	defer w.mu.Unlock()
	out := map[string]int{}
	for cap, hosts := range w.applied {
		out[cap] = len(hosts)
	}
	return out
}

type adapter struct {
	world   *World
	cap     capability.Capability
	plat    intent.Platform
	binding capability.Binding
}

func (a *adapter) ID() string                { return "demo." + a.cap.Name + "@" + string(a.plat) }
func (a *adapter) Capability() string        { return a.cap.Name }
func (a *adapter) Platform() intent.Platform { return a.plat }

func (a *adapter) Check(_ context.Context, t native.Target, params map[string]any) (native.State, error) {
	cur, ok := a.world.get(a.cap.Name, t.HostID)
	return native.State{"applied": ok, "params": cur, native.Satisfied: ok && fmt.Sprint(cur) == fmt.Sprint(params)}, nil
}

func (a *adapter) Apply(_ context.Context, t native.Target, params map[string]any) (native.Result, error) {
	if a.world.Flaky[t.HostID] {
		return native.Result{}, fmt.Errorf("simulated failure on host %d", t.HostID)
	}
	if a.world.Latency > 0 {
		time.Sleep(a.world.Latency)
	}
	if params == nil {
		params = map[string]any{}
	}
	a.world.set(a.cap.Name, t.HostID, params)
	return native.Result{Changed: true, Native: string(a.binding.Mechanism) + ": " + a.binding.Native, Duration: a.world.Latency}, nil
}

func (a *adapter) Revert(_ context.Context, t native.Target, _ map[string]any, prior native.State) (native.Result, error) {
	if applied, _ := prior["applied"].(bool); applied {
		p, _ := prior["params"].(map[string]any)
		a.world.set(a.cap.Name, t.HostID, p)
	} else {
		a.world.set(a.cap.Name, t.HostID, nil)
	}
	return native.Result{Changed: true, Native: "demo revert"}, nil
}

// Install registers a demo adapter for every catalog binding and a
// verification responder on the substrate that answers from the world.
func Install(reg *native.Registry, mem *substrate.Memory, cat *capability.Catalog, world *World) {
	for _, c := range cat.All() {
		for p, b := range c.Bindings {
			if c.ReadOnly {
				continue
			}
			_ = reg.Register(&adapter{world: world, cap: c, plat: p, binding: b})
		}
	}
	mem.RespondAll(func(sql string, h device.Host) ([]map[string]string, bool) {
		for _, c := range cat.All() {
			v, ok := c.Verify[h.Platform]
			if !ok {
				continue
			}
			params, applied := world.get(c.Name, h.ID)
			rendered := plan.RenderTemplate(v.Query, params)
			if rendered != sql {
				continue
			}
			satisfied := applied
			if v.Expect == "rows>0" {
				if satisfied {
					return []map[string]string{{"1": "1"}}, true
				}
				return nil, true
			}
			// rows==0 expectations: rows appear only while drifted.
			if satisfied {
				return nil, true
			}
			return []map[string]string{{"drift": "1"}}, true
		}
		return nil, false
	})
}
