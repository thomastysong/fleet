package learn

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/fleetdm/fleet/v4/adm/risk"
)

func TestLeashFromHistory(t *testing.T) {
	l := NewMemoryLedger()
	now := time.Date(2026, 9, 5, 0, 0, 0, 0, time.UTC)
	pat := "web.block@darwin,windows"
	for i := 0; i < 6; i++ {
		_ = l.Record(Outcome{At: now.Add(time.Duration(i) * time.Hour), Kind: KindApproved, Pattern: pat, Tier: risk.TierApprove})
		_ = l.Record(Outcome{At: now.Add(time.Duration(i) * time.Hour), Kind: KindVerified, Pattern: pat, Tier: risk.TierApprove, Success: true})
	}
	st, err := Leash(l, pat, now)
	if err != nil {
		t.Fatal(err)
	}
	if st.Accepted != 6 || st.Verified != 6 || st.Incidents != 0 {
		t.Fatalf("%+v", st)
	}
	pol := risk.DefaultPolicy()
	if st.Relaxation(pol, now.Add(24*time.Hour)) != 1 {
		t.Fatalf("6 acceptances / 5 per step should relax one tier, got %d", st.Relaxation(pol, now))
	}
	// A verified unattended run counts as acceptance too.
	_ = l.Record(Outcome{At: now.Add(10 * time.Hour), Kind: KindVerified, Pattern: pat, Tier: risk.TierAuto, Success: true})
	st, _ = Leash(l, pat, now)
	if st.Accepted != 7 {
		t.Fatalf("auto verified should count as accepted: %+v", st)
	}
	// An incident suspends relaxation.
	_ = l.Record(Outcome{At: now.Add(11 * time.Hour), Kind: KindRolledBack, Pattern: pat, Tier: risk.TierAuto})
	rep, err := Explain(l, pol, pat, now.Add(12*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if rep.State.Incidents != 1 || rep.Relaxation != 0 {
		t.Fatalf("incident should suspend relaxation: %+v", rep)
	}
	pats, _ := Patterns(l)
	if len(pats) != 1 || pats[0] != pat {
		t.Fatalf("%v", pats)
	}
}

func TestFileLedgerRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "outcomes.jsonl")
	l, err := NewFileLedger(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := l.Record(Outcome{Kind: KindProposed, PlanID: "p1", Pattern: "x@any", Evidence: map[string]any{"k": "v"}}); err != nil {
		t.Fatal(err)
	}
	if err := l.Record(Outcome{Kind: KindExecuted, PlanID: "p1", Pattern: "x@any", Success: true}); err != nil {
		t.Fatal(err)
	}
	rows, err := l.Query(Filter{PlanID: "p1", Kinds: []Kind{KindExecuted}})
	if err != nil || len(rows) != 1 || !rows[0].Success {
		t.Fatalf("%v %v", rows, err)
	}
	all, _ := l.Query(Filter{})
	if len(all) != 2 || all[0].At.IsZero() {
		t.Fatalf("%v", all)
	}
	empty, err := NewFileLedger(filepath.Join(t.TempDir(), "new.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if rows, err := empty.Query(Filter{}); err != nil || len(rows) != 0 {
		t.Fatalf("%v %v", rows, err)
	}
}
