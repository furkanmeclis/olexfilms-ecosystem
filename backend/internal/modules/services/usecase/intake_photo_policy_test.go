package usecase

import (
	"context"
	"errors"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	psmodel "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/photostandard/model"
)

type fakeIntakeGate struct {
	missing []string
	calls   int
}

func (g *fakeIntakeGate) RequireComplete(_ context.Context, _ *db.Queries, _ psmodel.ServiceRef) error {
	g.calls++
	if len(g.missing) > 0 {
		return &psmodel.IncompleteError{Missing: g.missing}
	}
	return nil
}

// TEC-499: leaving draft (except to cancelled) and completing need the full
// intake photo set; later moves and cancelling do not.
func TestNeedsIntakePhotos(t *testing.T) {
	cases := []struct {
		from, to string
		want     bool
	}{
		{StatusDraft, StatusPending, true},
		{StatusDraft, StatusProcessing, true},
		{StatusDraft, StatusCompleted, true},
		{StatusDraft, StatusCancelled, false},
		{StatusPending, StatusProcessing, false},
		{StatusPending, StatusReady, false},
		{StatusProcessing, StatusReady, false},
		{StatusProcessing, StatusCompleted, true},
		{StatusReady, StatusCompleted, true},
		{StatusReady, StatusCancelled, false},
	}
	for _, tc := range cases {
		if got := needsIntakePhotos(tc.from, tc.to); got != tc.want {
			t.Fatalf("needsIntakePhotos(%s, %s) = %v, want %v", tc.from, tc.to, got, tc.want)
		}
	}
}

func TestCheckIntakePhotos(t *testing.T) {
	ctx := context.Background()
	draft := db.Service{ID: 1, OrganizationID: 2, BrandID: 3, Status: StatusDraft}

	// No add-on wired (module off at wiring level): no rule.
	if err := New(nil, nil, nil).checkIntakePhotos(ctx, nil, draft, StatusPending); err != nil {
		t.Fatalf("no gate = %v", err)
	}

	gate := &fakeIntakeGate{missing: []string{"front", "rear"}}
	s := New(nil, nil, nil).WithIntakePhotoGate(gate)
	err := s.checkIntakePhotos(ctx, nil, draft, StatusPending)
	var inc *psmodel.IncompleteError
	if !errors.As(err, &inc) || len(inc.Missing) != 2 {
		t.Fatalf("draft -> pending with missing angles = %v", err)
	}
	if err := s.checkIntakePhotos(ctx, nil, draft, StatusCancelled); err != nil || gate.calls != 1 {
		t.Fatalf("cancel = %v (calls %d)", err, gate.calls)
	}
	pending := draft
	pending.Status = StatusPending
	if err := s.checkIntakePhotos(ctx, nil, pending, StatusProcessing); err != nil {
		t.Fatalf("pending -> processing = %v", err)
	}
	ready := draft
	ready.Status = StatusReady
	if err := s.checkIntakePhotos(ctx, nil, ready, StatusCompleted); !errors.As(err, &inc) {
		t.Fatalf("ready -> completed with missing angles = %v", err)
	}

	gate.missing = nil
	if err := s.checkIntakePhotos(ctx, nil, draft, StatusPending); err != nil {
		t.Fatalf("complete set = %v", err)
	}
}
