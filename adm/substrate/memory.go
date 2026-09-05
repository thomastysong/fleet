package substrate

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/fleetdm/fleet/v4/adm/device"
	"github.com/fleetdm/fleet/v4/adm/intent"
)

// Memory is an in-memory substrate for tests and the offline demo. Live
// queries are answered from canned responders keyed by a substring of the
// SQL; everything else is recorded.
type Memory struct {
	mu         sync.Mutex
	hosts      []device.Host
	responders []responder
	hooks      []Hook
	Profiles   []Profile
	Commands   []Command
	Policies   []Policy
	Scripts    []string
	Nudged     [][]uint
}

type responder struct {
	match string
	fn    func(h device.Host) []map[string]string
}

// Hook answers any SQL; it reports false to fall through to the keyed
// responders.
type Hook func(sql string, h device.Host) ([]map[string]string, bool)

// NewMemory returns a substrate holding the hosts.
func NewMemory(hosts ...device.Host) *Memory { return &Memory{hosts: hosts} }

// Name implements Substrate.
func (m *Memory) Name() string { return "memory" }

// Respond registers a live-query responder for SQL containing match.
func (m *Memory) Respond(match string, fn func(h device.Host) []map[string]string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	// Latest registration wins so tests and demos can override answers.
	m.responders = append([]responder{{match, fn}}, m.responders...)
}

// RespondAll registers a hook consulted before keyed responders.
func (m *Memory) RespondAll(h Hook) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.hooks = append(m.hooks, h)
}

// SetHosts replaces the inventory.
func (m *Memory) SetHosts(hosts ...device.Host) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.hosts = hosts
}

func (m *Memory) ListHosts(_ context.Context, scope intent.Scope) ([]device.Host, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := device.Filter(m.hosts, scope)
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (m *Memory) CountHosts(context.Context) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.hosts), nil
}

func (m *Memory) LiveQuery(_ context.Context, sql string, hostIDs []uint) (QueryResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	res := QueryResult{Rows: map[uint][]map[string]string{}, Errors: map[uint]string{}, Targeted: len(hostIDs)}
	byID := map[uint]device.Host{}
	for _, h := range m.hosts {
		byID[h.ID] = h
	}
	for _, id := range hostIDs {
		h, ok := byID[id]
		if !ok {
			res.Errors[id] = "unknown host"
			continue
		}
		if !h.Online {
			continue
		}
		res.Responded++
		res.Rows[id] = nil
		answered := false
		for _, hk := range m.hooks {
			if rows, ok := hk(sql, h); ok {
				res.Rows[id] = rows
				answered = true
				break
			}
		}
		if answered {
			continue
		}
		for _, r := range m.responders {
			if strings.Contains(sql, r.match) {
				res.Rows[id] = r.fn(h)
				break
			}
		}
	}
	return res, nil
}

func (m *Memory) AddProfile(_ context.Context, p Profile) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Profiles = append(m.Profiles, p)
	return nil
}

func (m *Memory) RunCommand(_ context.Context, c Command) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Commands = append(m.Commands, c)
	return nil
}

func (m *Memory) UpsertPolicy(_ context.Context, p Policy) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, existing := range m.Policies {
		if existing.Name == p.Name && existing.Fleet == p.Fleet {
			m.Policies[i] = p
			return nil
		}
	}
	m.Policies = append(m.Policies, p)
	return nil
}

func (m *Memory) RunScript(_ context.Context, hostID uint, script string) (ScriptResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Scripts = append(m.Scripts, fmt.Sprintf("%d:%s", hostID, script))
	return ScriptResult{ExitCode: 0, Output: "ok"}, nil
}

func (m *Memory) Nudge(_ context.Context, hostIDs []uint) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Nudged = append(m.Nudged, hostIDs)
	return nil
}

// DemoFleet returns a small, mixed inventory for the offline demo.
func DemoFleet() []device.Host {
	return []device.Host{
		{ID: 1, UUID: "u1", Hostname: "mbp-alice", Serial: "C02XA1", Platform: intent.PlatformDarwin, OSVersion: "15.6", Fleet: "Engineering", Labels: []string{"laptops", "engineering"}, Online: true},
		{ID: 2, UUID: "u2", Hostname: "mbp-bo", Serial: "C02XB2", Platform: intent.PlatformDarwin, OSVersion: "15.5", Fleet: "Engineering", Labels: []string{"laptops", "engineering"}, Online: true},
		{ID: 3, UUID: "u3", Hostname: "win-carol", Serial: "5CD1", Platform: intent.PlatformWindows, OSVersion: "11 24H2", Fleet: "Finance", Labels: []string{"laptops", "finance"}, Online: true},
		{ID: 4, UUID: "u4", Hostname: "win-dev", Serial: "5CD2", Platform: intent.PlatformWindows, OSVersion: "11 24H2", Fleet: "Engineering", Labels: []string{"laptops", "engineering"}, Online: false},
		{ID: 5, UUID: "u5", Hostname: "ubuntu-eve", Serial: "LX1", Platform: intent.PlatformLinux, OSVersion: "24.04", Fleet: "Engineering", Labels: []string{"laptops", "engineering"}, Online: true},
		{ID: 6, UUID: "u6", Hostname: "lobby-ipad", Serial: "DMPX1", Platform: intent.PlatformIPadOS, OSVersion: "18.6", Fleet: "Signage", Labels: []string{"signage"}, Online: true},
		{ID: 7, UUID: "u7", Hostname: "lobby-tv", Serial: "TV01", Platform: intent.PlatformTVOS, OSVersion: "18.6", Fleet: "Signage", Labels: []string{"signage"}, Online: true},
		{ID: 8, UUID: "u8", Hostname: "pixel-frank", Serial: "PX1", Platform: intent.PlatformAndroid, OSVersion: "15", Fleet: "Finance", Labels: []string{"mobile", "finance"}, Online: true},
	}
}
