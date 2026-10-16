package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	exportusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/exports/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stockforecast/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
)

type fakeExports struct {
	calls    int
	orgID    int64
	resource string
	format   ioengine.ExportFormat
	query    ioengine.ExportQuery
}

func (f *fakeExports) RequestExport(_ context.Context, _ int64, organizationID *int64, resource string,
	format ioengine.ExportFormat, query ioengine.ExportQuery, _ string) (exportusecase.ExportJobView, error) {
	f.calls++
	f.orgID, f.resource, f.format, f.query = *organizationID, resource, format, query
	return exportusecase.ExportJobView{Resource: resource, Format: string(format), Status: "pending"}, nil
}

func exportRequest(body string) *http.Request {
	r := httptest.NewRequest(http.MethodPost, "/v1/stock-forecasts/export", strings.NewReader(body))
	ctx := authctx.WithPrincipal(r.Context(), authctx.Principal{UserInternal: 7})
	ctx = orgctx.WithScope(ctx, orgctx.Scope{InternalID: 42, OrgType: "dealer", BrandID: 1})
	return r.WithContext(ctx)
}

// TEC-755: POST /v1/stock-forecasts/export queues the list export of the
// active organization with the validated list parameters.
func TestRequestListExport(t *testing.T) {
	t.Run("queues the list resource", func(t *testing.T) {
		ex := &fakeExports{}
		h := New(nil, nil).WithExports(ex)
		w := httptest.NewRecorder()
		h.RequestListExport(w, exportRequest(`{"format":"CSV","query":{"status":"critical,warning","q":"film","sort":"-days_left"}}`))
		if w.Code != http.StatusAccepted {
			t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
		}
		if ex.resource != usecase.ResourceListExport || ex.format != ioengine.ExportCSV || ex.orgID != 42 {
			t.Fatalf("export = %+v", ex)
		}
		if ex.query["status"] != "critical,warning" || ex.query["q"] != "film" || ex.query["sort"] != "-days_left" {
			t.Fatalf("query = %v", ex.query)
		}
	})
	for name, body := range map[string]string{
		"pdf is not offered": `{"format":"pdf","query":{}}`,
		"bad status":         `{"format":"xlsx","query":{"status":"bogus"}}`,
		"bad sort":           `{"format":"xlsx","query":{"sort":"on_hand_qty"}}`,
	} {
		t.Run(name, func(t *testing.T) {
			ex := &fakeExports{}
			w := httptest.NewRecorder()
			New(nil, nil).WithExports(ex).RequestListExport(w, exportRequest(body))
			if w.Code != http.StatusUnprocessableEntity && w.Code != http.StatusBadRequest {
				t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
			}
			if ex.calls != 0 {
				t.Fatal("invalid request must not queue a job")
			}
		})
	}
}
