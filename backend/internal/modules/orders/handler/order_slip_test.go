package handler

import (
	"context"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	ord "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/orders/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// A single-row source lets the real service/handler scope check run without
// starting a renderer. Any attempt to load parties or lines fails the test.
type orderSlipDB struct {
	t     *testing.T
	order db.Order
	calls int
}

func (d *orderSlipDB) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	d.t.Fatal("unexpected write")
	return pgconn.CommandTag{}, nil
}
func (d *orderSlipDB) Query(context.Context, string, ...any) (pgx.Rows, error) {
	d.t.Fatal("unauthorized line query")
	return nil, nil
}
func (d *orderSlipDB) QueryRow(_ context.Context, _ string, args ...any) pgx.Row {
	d.calls++
	if d.calls != 1 {
		d.t.Fatal("unauthorized party/template query")
	}
	if args[0] != d.order.Uuid {
		d.t.Fatal("wrong source")
	}
	return orderSlipRow{d.order}
}

type orderSlipRow struct{ order db.Order }

func (r orderSlipRow) Scan(dest ...any) error {
	v := reflect.ValueOf(r.order)
	for i, p := range dest {
		reflect.ValueOf(p).Elem().Set(v.Field(i))
	}
	return nil
}

func TestOrderSlipScopeReturns404BeforeRendering(t *testing.T) {
	for _, tc := range []struct {
		name  string
		brand int64
		orgs  []int64
	}{{"unrelated organization", 1, []int64{99}}, {"other brand", 2, []int64{10, 20}}, {"empty scope", 1, nil}} {
		t.Run(tc.name, func(t *testing.T) {
			source := &orderSlipDB{t: t, order: db.Order{Uuid: uuid.New(), BrandID: 1, SellerOrgID: 10, BuyerOrgID: 20, OrderNo: "OTHER-ORG-SECRET"}}
			h := New(ord.New(nil, db.New(source), nil, nil))
			r := httptest.NewRequest("GET", "/v1/orders/"+source.order.Uuid.String()+"/order-slip.pdf?lang=ar&tz=Asia/Dubai", nil)
			r.SetPathValue("uuid", source.order.Uuid.String())
			ctx := authctx.WithPrincipal(r.Context(), authctx.Principal{})
			ctx = orgctx.WithScope(ctx, orgctx.Scope{InternalID: 99, BrandID: tc.brand})
			ctx = scopefilter.With(ctx, scopefilter.Filter{Scope: rbac.ScopeManaged, OrgIDs: tc.orgs})
			w := httptest.NewRecorder()
			h.OrderSlip(w, r.WithContext(ctx))
			if w.Code != 404 {
				t.Fatalf("status %d: %s", w.Code, w.Body.String())
			}
			if source.calls != 1 {
				t.Fatalf("queries = %d", source.calls)
			}
		})
	}
}
