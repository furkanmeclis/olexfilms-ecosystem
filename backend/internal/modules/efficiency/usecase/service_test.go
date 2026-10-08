package usecase

import (
	"context"
	"errors"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
)

func TestDealerCannotReadStaffSummary(t *testing.T) {
	svc := New(nil)
	_, _, err := svc.Summary(context.Background(), Caller{
		Org:    orgctx.Scope{OrgType: rbac.OrgTypeDealer},
		Filter: scopefilter.Filter{Scope: rbac.ScopeManaged},
	}, "staff", AnalyticsFilter{})
	if err == nil {
		t.Fatal("expected validation error")
	}
	var ve *ValidationError
	if !errors.As(err, &ve) || ve.Field != "dimension" {
		t.Fatalf("expected dimension validation error, got %T %v", err, err)
	}
}
