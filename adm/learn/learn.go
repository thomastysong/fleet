// Package learn closes the loop. Every plan, approval, execution,
// verification and incident is an Outcome written to a ledger; the ledger is
// what the lakehouse ingests and what calibrates the adaptive leash so that
// accepted patterns earn autonomy and incidents take it away.
package learn

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/fleetdm/fleet/v4/adm/risk"
)

// Kind is the type of outcome.
type Kind string

const (
	KindProposed   Kind = "proposed"
	KindApproved   Kind = "approved"
	KindRejected   Kind = "rejected"
	KindExecuted   Kind = "executed"
	KindVerified   Kind = "verified"
	KindFailed     Kind = "failed"
	KindRolledBack Kind = "rolled_back"
	KindIncident   Kind = "incident"
	KindDrift      Kind = "drift"
	KindFeedback   Kind = "feedback"
)

// Outcome is one ledger row. It is flat on purpose: this is the schema of
// the lakehouse table adm_outcomes.
type Outcome struct {
	At           time.Time      `json:"at"`
	Kind         Kind           `json:"kind"`
	IntentID     string         `json:"intent_id,omitempty"`
	PlanID       string         `json:"plan_id,omitempty"`
	Pattern      string         `json:"pattern,omitempty"`
	Fingerprint  string         `json:"fingerprint,omitempty"`
	Capabilities []string       `json:"capabilities,omitempty"`
	Tier         risk.Tier      `json:"tier"`
	Score        int            `json:"score,omitempty"`
	Hosts        int            `json:"hosts,omitempty"`
	HostID       uint           `json:"host_id,omitempty"`
	Platform     string         `json:"platform,omitempty"`
	Actor        string         `json:"actor,omitempty"`
	Success      bool           `json:"success"`
	Message      string         `json:"message,omitempty"`
	Duration     time.Duration  `json:"duration,omitempty"`
	Evidence     map[string]any `json:"evidence,omitempty"`
}

// Ledger stores outcomes.
type Ledger interface {
	Record(o Outcome) error
	// Query returns outcomes matching the filter, oldest first.
	Query(f Filter) ([]Outcome, error)
}

// Filter selects outcomes.
type Filter struct {
	Pattern  string
	IntentID string
	PlanID   string
	Kinds    []Kind
	Since    time.Time
}

func (f Filter) match(o Outcome) bool {
	if f.Pattern != "" && o.Pattern != f.Pattern {
		return false
	}
	if f.IntentID != "" && o.IntentID != f.IntentID {
		return false
	}
	if f.PlanID != "" && o.PlanID != f.PlanID {
		return false
	}
	if !f.Since.IsZero() && o.At.Before(f.Since) {
		return false
	}
	if len(f.Kinds) > 0 {
		ok := false
		for _, k := range f.Kinds {
			if o.Kind == k {
				ok = true
				break
			}
		}
		if !ok {
			return false
		}
	}
	return true
}

// MemoryLedger keeps outcomes in memory.
type MemoryLedger struct {
	mu   sync.Mutex
	rows []Outcome
}

// NewMemoryLedger returns an empty ledger.
func NewMemoryLedger() *MemoryLedger { return &MemoryLedger{} }

func (m *MemoryLedger) Record(o Outcome) error {
	if o.At.IsZero() {
		o.At = time.Now()
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rows = append(m.rows, o)
	return nil
}

func (m *MemoryLedger) Query(f Filter) ([]Outcome, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []Outcome
	for _, o := range m.rows {
		if f.match(o) {
			out = append(out, o)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].At.Before(out[j].At) })
	return out, nil
}

// FileLedger appends JSON lines to a file: the simplest lakehouse-friendly
// sink (one row per line, ready for a Parquet/Iceberg loader).
type FileLedger struct {
	path string
	mu   sync.Mutex
}

// NewFileLedger opens or creates the file.
func NewFileLedger(path string) (*FileLedger, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	_ = f.Close()
	return &FileLedger{path: path}, nil
}

func (l *FileLedger) Record(o Outcome) error {
	if o.At.IsZero() {
		o.At = time.Now()
	}
	b, err := json.Marshal(o)
	if err != nil {
		return err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	f, err := os.OpenFile(l.path, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(append(b, '\n'))
	return err
}

func (l *FileLedger) Query(filter Filter) ([]Outcome, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	f, err := os.Open(l.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()
	var out []Outcome
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 1<<20), 16<<20)
	for sc.Scan() {
		var o Outcome
		if err := json.Unmarshal(sc.Bytes(), &o); err != nil {
			return nil, fmt.Errorf("corrupt ledger line: %w", err)
		}
		if filter.match(o) {
			out = append(out, o)
		}
	}
	return out, sc.Err()
}

// Leash derives the adaptive-autonomy state of a pattern from its history.
// Accepted counts approvals and verified auto-executions (a human or the
// verifier said "yes"); Rejected counts declined plans; Incidents counts
// rollbacks and incidents; Failed counts failed verifications.
func Leash(l Ledger, pattern string, now time.Time) (risk.LeashState, error) {
	rows, err := l.Query(Filter{Pattern: pattern})
	if err != nil {
		return risk.LeashState{}, err
	}
	st := risk.LeashState{Pattern: pattern}
	for _, o := range rows {
		st.LastSeen = o.At
		switch o.Kind {
		case KindApproved:
			st.Accepted++
		case KindRejected:
			st.Rejected++
		case KindVerified:
			st.Verified++
			if o.Tier <= risk.TierCanary {
				// An unattended execution that verified is an acceptance too.
				st.Accepted++
			}
		case KindFailed:
			st.Failed++
		case KindRolledBack, KindIncident:
			st.Incidents++
			if o.At.After(st.LastIncident) {
				st.LastIncident = o.At
			}
		case KindFeedback:
			if !o.Success {
				st.Rejected++
			}
		}
	}
	_ = now
	return st, nil
}

// Report summarises a pattern's history for humans.
type Report struct {
	Pattern    string          `json:"pattern"`
	State      risk.LeashState `json:"state"`
	Relaxation int             `json:"relaxation"`
	Novelty    float64         `json:"novelty"`
	Rows       int             `json:"rows"`
}

// Explain returns the leash report for a pattern under a policy.
func Explain(l Ledger, pol risk.Policy, pattern string, now time.Time) (Report, error) {
	st, err := Leash(l, pattern, now)
	if err != nil {
		return Report{}, err
	}
	rows, _ := l.Query(Filter{Pattern: pattern})
	return Report{Pattern: pattern, State: st, Relaxation: st.Relaxation(pol, now), Novelty: st.Novelty(), Rows: len(rows)}, nil
}

// Patterns lists every pattern seen in the ledger, sorted.
func Patterns(l Ledger) ([]string, error) {
	rows, err := l.Query(Filter{})
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var out []string
	for _, o := range rows {
		if o.Pattern != "" && !seen[o.Pattern] {
			seen[o.Pattern] = true
			out = append(out, o.Pattern)
		}
	}
	sort.Strings(out)
	return out, nil
}
