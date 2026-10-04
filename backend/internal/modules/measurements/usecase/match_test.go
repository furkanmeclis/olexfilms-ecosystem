package usecase

import (
	"testing"
	"time"
)

var matchStart = time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)

func at(d time.Duration) time.Time { return matchStart.Add(d) }

func ids(list []MatchCandidate) []int64 {
	out := make([]int64, 0, len(list))
	for _, c := range list {
		out = append(out, c.ID)
	}
	return out
}

// TEC-296: one measurement before and one after the service start are
// linked automatically into the right phase.
func TestMatchOneBeforeOneAfterAutoLinks(t *testing.T) {
	plan := Match(MatchService{Status: "processing", StartedAt: matchStart}, []MatchCandidate{
		{ID: 1, At: at(-2 * time.Hour)}, {ID: 2, At: at(5 * time.Hour)},
	})
	if plan.Auto[PhaseBefore].ID != 1 || plan.Auto[PhaseAfter].ID != 2 || len(plan.Auto) != 2 {
		t.Fatalf("auto = %+v", plan.Auto)
	}
}

// TEC-296: two candidates in a phase are suggested (before newest first,
// after oldest first) and none is linked.
func TestMatchTwoCandidatesOnlySuggest(t *testing.T) {
	plan := Match(MatchService{Status: "ready", StartedAt: matchStart}, []MatchCandidate{
		{ID: 1, At: at(-48 * time.Hour)}, {ID: 2, At: at(-time.Hour)},
		{ID: 3, At: at(30 * time.Hour)}, {ID: 4, At: at(2 * time.Hour)},
	})
	if len(plan.Auto) != 0 {
		t.Fatalf("auto = %+v, want none", plan.Auto)
	}
	if got := ids(plan.Suggestions[PhaseBefore]); len(got) != 2 || got[0] != 2 || got[1] != 1 {
		t.Fatalf("before = %v", got)
	}
	if got := ids(plan.Suggestions[PhaseAfter]); len(got) != 2 || got[0] != 4 || got[1] != 3 {
		t.Fatalf("after = %v", got)
	}
}

// TEC-296: an occupied phase (e.g. a confirmed link) is never linked by the
// rule, the new measurement is only suggested.
func TestMatchOccupiedPhaseNotLinked(t *testing.T) {
	plan := Match(MatchService{Status: "processing", StartedAt: matchStart, Linked: map[string]bool{PhaseBefore: true}},
		[]MatchCandidate{{ID: 7, At: at(-time.Hour)}})
	if _, ok := plan.Auto[PhaseBefore]; ok {
		t.Fatalf("occupied phase linked: %+v", plan.Auto)
	}
	if got := ids(plan.Suggestions[PhaseBefore]); len(got) != 1 || got[0] != 7 {
		t.Fatalf("before = %v", got)
	}
}

// TEC-296: the windows: before at most 14 days old, after only from
// processing on and at most 7 days after completion.
func TestMatchWindows(t *testing.T) {
	old := MatchCandidate{ID: 1, At: at(-BeforeWindow - time.Minute)}
	edge := MatchCandidate{ID: 2, At: at(-BeforeWindow)}
	later := MatchCandidate{ID: 3, At: at(time.Hour)}
	if plan := Match(MatchService{Status: "pending", StartedAt: matchStart}, []MatchCandidate{old, edge, later}); len(plan.Suggestions[PhaseAfter]) != 0 ||
		len(ids(plan.Suggestions[PhaseBefore])) != 1 || plan.Auto[PhaseBefore].ID != 2 {
		t.Fatalf("pending plan = %+v", plan)
	}
	done := at(24 * time.Hour)
	in := MatchCandidate{ID: 4, At: done.Add(AfterWindow)}
	out := MatchCandidate{ID: 5, At: done.Add(AfterWindow + time.Minute)}
	plan := Match(MatchService{Status: "completed", StartedAt: matchStart, CompletedAt: &done}, []MatchCandidate{in, out})
	if got := ids(plan.Suggestions[PhaseAfter]); len(got) != 1 || got[0] != 4 || plan.Auto[PhaseAfter].ID != 4 {
		t.Fatalf("completed plan = %+v", plan)
	}
}
