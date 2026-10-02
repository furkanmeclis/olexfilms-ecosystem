package usecase

import (
	"context"
	"errors"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/activity"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/google/uuid"
)

// The refusals that need no database: a non-center organization and a
// self-merge (checked before the transaction starts).
func TestMergeRefusalsBeforeTransaction(t *testing.T) {
	s := New(nil, nil, nil, nil)
	ctx := context.Background()
	a, b := uuid.New(), uuid.New()

	dealer := Caller{UserID: 1}
	dealer.Org.OrgType = rbac.OrgTypeDealer
	if _, err := s.PreviewMerge(ctx, dealer, a, b); !errors.Is(err, ErrForbidden) {
		t.Fatalf("dealer preview: %v", err)
	}
	if _, err := s.MergeCustomer(ctx, dealer, a, b, activity.Meta{}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("dealer merge: %v", err)
	}

	center := Caller{UserID: 1}
	center.Org.OrgType = rbac.OrgTypeCenter
	if _, err := s.PreviewMerge(ctx, center, a, a); !errors.Is(err, ErrMergeSelf) {
		t.Fatalf("self preview: %v", err)
	}
	if _, err := s.MergeCustomer(ctx, center, a, a, activity.Meta{}); !errors.Is(err, ErrMergeSelf) {
		t.Fatalf("self merge: %v", err)
	}
}

func TestMergePayloadHasNoPersonalData(t *testing.T) {
	r := MergeResult{SourceUUID: uuid.New(), TargetUUID: uuid.New(), Profile: ProfileMoved}
	r.Moved.Vehicles, r.Moved.Warranties = 2, 1
	p := mergePayload(r)
	if p["target_uuid"] != r.TargetUUID.String() || p["vehicles"] != int64(2) || p["warranties"] != int64(1) || p["profile"] != ProfileMoved {
		t.Fatalf("payload = %v", p)
	}
	for _, k := range []string{"phone", "email", "name"} {
		if _, ok := p[k]; ok {
			t.Fatalf("payload carries %s", k)
		}
	}
}
