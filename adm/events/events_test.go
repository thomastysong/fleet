package events

import (
	"context"
	"testing"
	"time"

	"github.com/fleetdm/fleet/v4/adm/intent"
)

func TestMemoryBusRoutesByType(t *testing.T) {
	b := NewMemoryBus()
	storage, cancel := b.Subscribe(StorageAttached)
	defer cancel()
	all, cancelAll := b.Subscribe()
	defer cancelAll()
	ctx := context.Background()
	_ = b.Publish(ctx, Event{Type: HostOnline, HostID: 1})
	_ = b.Publish(ctx, Event{Type: StorageAttached, HostID: 1, Subject: "sdb"})
	select {
	case e := <-storage:
		if e.Type != StorageAttached || e.At.IsZero() {
			t.Fatalf("%+v", e)
		}
	case <-time.After(time.Second):
		t.Fatal("no storage event")
	}
	n := 0
	for i := 0; i < 2; i++ {
		select {
		case <-all:
			n++
		case <-time.After(time.Second):
		}
	}
	if n != 2 {
		t.Fatalf("wildcard subscriber got %d events", n)
	}
	select {
	case <-storage:
		t.Fatal("storage subscriber should not see host events")
	default:
	}
}

func TestBusDropsOldestWhenSlow(t *testing.T) {
	b := NewMemoryBus()
	b.Buffer = 2
	ch, cancel := b.Subscribe()
	defer cancel()
	for i := 0; i < 5; i++ {
		_ = b.Publish(context.Background(), Event{Type: Sweep, Subject: string(rune('a' + i))})
	}
	first := <-ch
	if first.Subject == "a" || first.Subject == "b" {
		t.Fatalf("oldest events should be dropped, got %s", first.Subject)
	}
}

func TestAffected(t *testing.T) {
	storage := &intent.Intent{State: intent.StateActive, Scope: intent.Scope{Platforms: []intent.Platform{intent.PlatformDarwin, intent.PlatformWindows}}, Desired: []intent.Predicate{{Capability: "storage.removable.block"}}}
	encrypt := &intent.Intent{State: intent.StateActive, Desired: []intent.Predicate{{Capability: "disk.encryption.enforce"}}}
	paused := &intent.Intent{State: intent.StatePaused, Desired: []intent.Predicate{{Capability: "storage.removable.block"}}}

	if re, _ := Affected(Event{Type: StorageAttached, Platform: intent.PlatformWindows}, storage, DefaultTriggers); !re {
		t.Fatal("storage attach should re-evaluate the storage intent")
	}
	if re, _ := Affected(Event{Type: StorageAttached, Platform: intent.PlatformLinux}, storage, DefaultTriggers); re {
		t.Fatal("linux event should not affect a darwin/windows intent")
	}
	if re, _ := Affected(Event{Type: StorageAttached}, encrypt, DefaultTriggers); re {
		t.Fatal("storage attach should not touch the encryption intent")
	}
	if re, _ := Affected(Event{Type: StorageAttached}, paused, DefaultTriggers); re {
		t.Fatal("paused intents are never re-evaluated")
	}
	re, rescope := Affected(Event{Type: HostEnrolled}, encrypt, DefaultTriggers)
	if !re || !rescope {
		t.Fatal("enrollment should re-evaluate and re-scope everything")
	}
	if re, _ := Affected(Event{Type: "unknown"}, encrypt, DefaultTriggers); re {
		t.Fatal("unknown events are ignored")
	}
}
