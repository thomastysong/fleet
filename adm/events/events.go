// Package events is the nervous system of ADM: a bus of typed device,
// substrate and external events, and the trigger table that decides which
// intents an event makes stale. Reconciliation in ADM is event-driven; the
// periodic sweep exists only as a safety net for events that were missed.
package events

import (
	"context"
	"sync"
	"time"

	"github.com/fleetdm/fleet/v4/adm/intent"
)

// Type classifies an event.
type Type string

const (
	// Device lifecycle (from the substrate).
	HostEnrolled     Type = "host.enrolled"
	HostOnline       Type = "host.online"
	HostOffline      Type = "host.offline"
	HostUnenrolled   Type = "host.unenrolled"
	HostLabelChanged Type = "host.label.changed"
	// Native OS events (from the ADM agent's watchers).
	StorageAttached Type = "storage.attached"
	StorageDetached Type = "storage.detached"
	ProcessStarted  Type = "process.started"
	ConfigDrift     Type = "config.drift"
	SoftwareChanged Type = "software.changed"
	UserLogin       Type = "user.login"
	// Control-plane and external events.
	VulnPublished    Type = "vuln.published"
	ReleasePublished Type = "release.published"
	RuntimeEOL       Type = "runtime.eol"
	IdPGroupChanged  Type = "idp.group.changed"
	TicketApproved   Type = "ticket.approved"
	IntentChanged    Type = "intent.changed"
	PolicyFailing    Type = "policy.failing"
	Sweep            Type = "sweep"
)

// Event is one occurrence.
type Event struct {
	Type     Type            `json:"type"`
	Source   string          `json:"source"`
	At       time.Time       `json:"at"`
	HostID   uint            `json:"host_id,omitempty"`
	Platform intent.Platform `json:"platform,omitempty"`
	// Subject names what changed: a capability, a label, a CVE, a title.
	Subject string         `json:"subject,omitempty"`
	Payload map[string]any `json:"payload,omitempty"`
}

// Bus delivers events to subscribers.
type Bus interface {
	Publish(ctx context.Context, e Event) error
	Subscribe(types ...Type) (<-chan Event, func())
}

// MemoryBus is an in-process bus. Production deployments back this with
// Redis streams, which Fleet already runs.
type MemoryBus struct {
	mu   sync.RWMutex
	subs map[int]*subscription
	next int
	// Buffer is the per-subscriber channel size; a slow subscriber drops the
	// oldest event and relies on the sweep to catch up.
	Buffer int
}

type subscription struct {
	types map[Type]bool
	ch    chan Event
}

// NewMemoryBus returns a bus.
func NewMemoryBus() *MemoryBus { return &MemoryBus{subs: map[int]*subscription{}, Buffer: 256} }

func (b *MemoryBus) Publish(ctx context.Context, e Event) error {
	if e.At.IsZero() {
		e.At = time.Now()
	}
	b.mu.RLock()
	defer b.mu.RUnlock()
	for _, s := range b.subs {
		if len(s.types) > 0 && !s.types[e.Type] {
			continue
		}
		select {
		case s.ch <- e:
		default:
			// drop oldest, then enqueue
			select {
			case <-s.ch:
			default:
			}
			select {
			case s.ch <- e:
			default:
			}
		}
	}
	return ctx.Err()
}

func (b *MemoryBus) Subscribe(types ...Type) (<-chan Event, func()) {
	b.mu.Lock()
	defer b.mu.Unlock()
	id := b.next
	b.next++
	s := &subscription{types: map[Type]bool{}, ch: make(chan Event, b.Buffer)}
	for _, t := range types {
		s.types[t] = true
	}
	b.subs[id] = s
	return s.ch, func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		if cur, ok := b.subs[id]; ok {
			delete(b.subs, id)
			close(cur.ch)
		}
	}
}

// Trigger maps an event type to the capability domains it makes stale. When
// an event arrives, every active intent using one of those capabilities on
// the affected host is re-evaluated immediately (a one-host micro-plan), and
// intents whose scope may have changed are re-scoped.
type Trigger struct {
	Capabilities []string // capability name prefixes; "*" means every intent
	Rescope      bool     // the event may change which hosts an intent selects
	Immediate    bool     // reconcile on the affected host without waiting
}

// DefaultTriggers is the built-in trigger table.
var DefaultTriggers = map[Type]Trigger{
	HostEnrolled:     {Capabilities: []string{"*"}, Rescope: true, Immediate: true},
	HostOnline:       {Capabilities: []string{"*"}, Immediate: true},
	HostLabelChanged: {Capabilities: []string{"*"}, Rescope: true, Immediate: true},
	IdPGroupChanged:  {Capabilities: []string{"*"}, Rescope: true},
	StorageAttached:  {Capabilities: []string{"storage."}, Immediate: true},
	ProcessStarted:   {Capabilities: []string{"ai.tools", "edr."}, Immediate: true},
	ConfigDrift:      {Capabilities: []string{"*"}, Immediate: true},
	SoftwareChanged:  {Capabilities: []string{"software.", "package.manager", "ai.tools", "runtime."}, Immediate: true},
	UserLogin:        {Capabilities: []string{"screen.lock", "identity."}, Immediate: true},
	VulnPublished:    {Capabilities: []string{"software.", "package.manager", "dependency.", "os.update"}},
	ReleasePublished: {Capabilities: []string{"software.version.floor", "software.release.watch"}},
	RuntimeEOL:       {Capabilities: []string{"runtime.", "software."}},
	PolicyFailing:    {Capabilities: []string{"*"}, Immediate: true},
	TicketApproved:   {Capabilities: []string{"*"}},
	IntentChanged:    {Capabilities: []string{"*"}, Rescope: true},
	Sweep:            {Capabilities: []string{"*"}, Rescope: true},
}

// Affected reports whether an intent should be re-evaluated for the event,
// and whether it should be re-scoped.
func Affected(e Event, in *intent.Intent, triggers map[Type]Trigger) (reevaluate, rescope bool) {
	tr, ok := triggers[e.Type]
	if !ok {
		return false, false
	}
	if in.State != intent.StateActive && in.State != intent.StateCompiled {
		return false, false
	}
	// Platform-scoped intents ignore events from other platforms.
	if e.Platform != "" && len(in.Scope.Platforms) > 0 {
		match := false
		for _, p := range in.Scope.Platforms {
			if p == e.Platform {
				match = true
				break
			}
		}
		if !match {
			return false, false
		}
	}
	for _, c := range tr.Capabilities {
		if c == "*" {
			return true, tr.Rescope
		}
		for _, d := range in.Desired {
			if len(d.Capability) >= len(c) && d.Capability[:len(c)] == c {
				return true, tr.Rescope
			}
		}
	}
	return false, tr.Rescope
}
