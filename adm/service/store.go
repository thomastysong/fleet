package service

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"github.com/fleetdm/fleet/v4/adm/intent"
	"github.com/fleetdm/fleet/v4/adm/plan"
)

// ErrNotFound is returned for unknown intents and plans.
var ErrNotFound = errors.New("not found")

// Store persists intents and plans.
type Store interface {
	SaveIntent(in *intent.Intent) error
	GetIntent(id string) (*intent.Intent, error)
	ListIntents() ([]*intent.Intent, error)
	SavePlan(p *plan.Plan) error
	GetPlan(id string) (*plan.Plan, error)
	ListPlans() ([]*plan.Plan, error)
}

// MemoryStore keeps everything in memory.
type MemoryStore struct {
	mu      sync.RWMutex
	intents map[string]*intent.Intent
	plans   map[string]*plan.Plan
}

// NewMemoryStore returns an empty store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{intents: map[string]*intent.Intent{}, plans: map[string]*plan.Plan{}}
}

func (s *MemoryStore) SaveIntent(in *intent.Intent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := *in
	s.intents[in.ID] = &cp
	return nil
}

func (s *MemoryStore) GetIntent(id string) (*intent.Intent, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	in, ok := s.intents[id]
	if !ok {
		return nil, ErrNotFound
	}
	cp := *in
	return &cp, nil
}

func (s *MemoryStore) ListIntents() ([]*intent.Intent, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*intent.Intent, 0, len(s.intents))
	for _, in := range s.intents {
		cp := *in
		out = append(out, &cp)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

func (s *MemoryStore) SavePlan(p *plan.Plan) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := *p
	s.plans[p.ID] = &cp
	return nil
}

func (s *MemoryStore) GetPlan(id string) (*plan.Plan, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.plans[id]
	if !ok {
		return nil, ErrNotFound
	}
	cp := *p
	return &cp, nil
}

func (s *MemoryStore) ListPlans() ([]*plan.Plan, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*plan.Plan, 0, len(s.plans))
	for _, p := range s.plans {
		cp := *p
		out = append(out, &cp)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

// FileStore persists to JSON files in a directory (the CLI's state dir).
type FileStore struct {
	dir string
	mem *MemoryStore
	mu  sync.Mutex
}

// NewFileStore loads or creates the store in dir.
func NewFileStore(dir string) (*FileStore, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	fs := &FileStore{dir: dir, mem: NewMemoryStore()}
	if err := fs.load(); err != nil {
		return nil, err
	}
	return fs, nil
}

func (f *FileStore) load() error {
	var intents []*intent.Intent
	if err := readJSON(filepath.Join(f.dir, "intents.json"), &intents); err != nil {
		return err
	}
	for _, in := range intents {
		_ = f.mem.SaveIntent(in)
	}
	var plans []*plan.Plan
	if err := readJSON(filepath.Join(f.dir, "plans.json"), &plans); err != nil {
		return err
	}
	for _, p := range plans {
		_ = f.mem.SavePlan(p)
	}
	return nil
}

func (f *FileStore) flush() error {
	intents, _ := f.mem.ListIntents()
	if err := writeJSON(filepath.Join(f.dir, "intents.json"), intents); err != nil {
		return err
	}
	plans, _ := f.mem.ListPlans()
	return writeJSON(filepath.Join(f.dir, "plans.json"), plans)
}

func (f *FileStore) SaveIntent(in *intent.Intent) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.mem.SaveIntent(in); err != nil {
		return err
	}
	return f.flush()
}

func (f *FileStore) GetIntent(id string) (*intent.Intent, error) { return f.mem.GetIntent(id) }
func (f *FileStore) ListIntents() ([]*intent.Intent, error)      { return f.mem.ListIntents() }

func (f *FileStore) SavePlan(p *plan.Plan) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.mem.SavePlan(p); err != nil {
		return err
	}
	return f.flush()
}

func (f *FileStore) GetPlan(id string) (*plan.Plan, error) { return f.mem.GetPlan(id) }
func (f *FileStore) ListPlans() ([]*plan.Plan, error)      { return f.mem.ListPlans() }

func readJSON(path string, v any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	if len(b) == 0 {
		return nil
	}
	return json.Unmarshal(b, v)
}

func writeJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
