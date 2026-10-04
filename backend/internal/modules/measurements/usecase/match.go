package usecase

import (
	"sort"
	"time"
)

// Measurement phases of a service (service_measurements.phase).
const (
	PhaseBefore = "before"
	PhaseAfter  = "after"
)

// Link sources (service_measurements.link_source).
const (
	LinkSourceAuto   = "auto"
	LinkSourceManual = "manual"
)

// Matching windows (TEC-296, K28). Conservative defaults kept in code: a
// before measurement is at most 14 days older than the service start, an
// after measurement at most 7 days younger than the completion.
const (
	BeforeWindow = 14 * 24 * time.Hour
	AfterWindow  = 7 * 24 * time.Hour
)

// Service statuses the after phase is looked for in.
var afterStatuses = map[string]bool{"processing": true, "ready": true, "completed": true}

// MatchService is the part of a service the matching rule reads.
type MatchService struct {
	Status      string
	StartedAt   time.Time  // services.created_at
	CompletedAt *time.Time // services.completed_at
	// Linked is the phases that already have a measurement (auto or manual,
	// confirmed or not). The rule never links into an occupied phase.
	Linked map[string]bool
}

// MatchCandidate is one unlinked accepted measurement of the same
// organization and VIN. At is the device time, falling back to the upload
// time.
type MatchCandidate struct {
	ID int64
	At time.Time
}

// MatchPlan is the outcome of the rule for one service.
type MatchPlan struct {
	// Suggestions lists the window candidates per phase: before newest
	// first, after oldest first.
	Suggestions map[string][]MatchCandidate
	// Auto is the measurement to link automatically per phase: only when
	// the phase is empty and exactly one candidate is in its window.
	Auto map[string]MatchCandidate
}

// Match is the before/after matching rule (TEC-296):
//   - before: the measurements in the 14 days before the service start;
//   - after: once the service is processing / ready / completed, the
//     measurements from the service start on, at most 7 days after the
//     completion;
//   - a phase is linked automatically only when it is empty and has exactly
//     one candidate; with several candidates nothing is linked and all are
//     suggested.
func Match(svc MatchService, candidates []MatchCandidate) MatchPlan {
	plan := MatchPlan{Suggestions: map[string][]MatchCandidate{}, Auto: map[string]MatchCandidate{}}
	start := svc.StartedAt
	for _, c := range candidates {
		switch {
		case c.At.Before(start):
			if !c.At.Before(start.Add(-BeforeWindow)) {
				plan.Suggestions[PhaseBefore] = append(plan.Suggestions[PhaseBefore], c)
			}
		case afterStatuses[svc.Status]:
			if svc.CompletedAt != nil && c.At.After(svc.CompletedAt.Add(AfterWindow)) {
				continue
			}
			plan.Suggestions[PhaseAfter] = append(plan.Suggestions[PhaseAfter], c)
		}
	}
	before := plan.Suggestions[PhaseBefore]
	sort.SliceStable(before, func(i, j int) bool { return before[i].At.After(before[j].At) })
	after := plan.Suggestions[PhaseAfter]
	sort.SliceStable(after, func(i, j int) bool { return after[i].At.Before(after[j].At) })
	for phase, list := range plan.Suggestions {
		if len(list) == 1 && !svc.Linked[phase] {
			plan.Auto[phase] = list[0]
		}
	}
	return plan
}
