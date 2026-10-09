package httpserver

import (
	"context"
	"strconv"
	"strings"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/documents/documentstest"
	exportusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/exports/usecase"
	servicesusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/services/usecase"
	warrantymodule "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warranty"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/pdfrender"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/storage"
)

// TEC-521 acceptance (panel): the export worker prints the issued-at line
// of a Gotenberg document in the requester's zone (K10: user ->
// organization -> brand center -> Europe/Istanbul), never in UTC.
func TestIntegrationPDFIssuedAtTimezone(t *testing.T) {
	gotb := documentstest.NewGotenberg(t, 0)
	store := storage.NewMemory()
	it := newIntegration(t)
	ctx := context.Background()

	center := it.brandCenter("olex")
	dist := it.org("t521-dist", "distributor", center)
	dealer := it.org("t521-dealer", "dealer", dist)
	owner, _ := it.user("t521-owner")
	it.member(dealer, owner, "owner")
	cust, veh := it.svcCustomer(dealer, "t521-cust", "34T521"+it.suffix[len(it.suffix)-4:])
	svc := it.directService(dealer, cust, veh, 1)

	cert := warrantymodule.NewCertificate(it.q, store, "https://olexfilms.test", nil)
	pdfSvc := servicesusecase.NewPDF(servicesusecase.New(it.pool, it.q, nil), cert, store, nil)
	worker := exportusecase.New(it.q, store, ioengine.NewRegistry(servicesusecase.NewPDFAdapter(pdfSvc)), nil, nil, nil, nil)
	worker.SetDocumentPDF(pdfrender.New(gotb.URL))
	render := func(t *testing.T) string {
		t.Helper()
		orgID := dealer.ID
		if _, err := worker.RequestExport(ctx, owner.ID, &orgID, servicesusecase.ResourcePDF, ioengine.ExportPDF, ioengine.ExportQuery{
			servicesusecase.PDFQueryServiceUUID: svc.Uuid.String(),
			servicesusecase.PDFQueryBrandID:     strconv.FormatInt(svc.BrandID, 10),
		}, "tr"); err != nil {
			t.Fatalf("export: %v", err)
		}
		h := gotb.LastHTML()
		if !strings.Contains(h, svc.ServiceNo) {
			t.Fatal("service pdf html missing")
		}
		if strings.Contains(h, " UTC") {
			t.Fatal("issued-at is printed in UTC")
		}
		return h
	}
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := it.pool.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	var centerTZ string
	if err := it.pool.QueryRow(ctx, `SELECT timezone FROM organizations WHERE id = $1`, center.ID).Scan(&centerTZ); err != nil {
		t.Fatal(err)
	}

	// The user inherits: the dealer organization's zone.
	exec(`UPDATE users SET timezone = NULL WHERE id = $1`, owner.ID)
	exec(`UPDATE organizations SET timezone = 'Asia/Baku' WHERE id = $1`, dealer.ID)
	if h := render(t); !strings.Contains(h, "(Asia/Baku)") {
		t.Fatal("issued-at is not in the organization zone")
	}
	// The user's own zone wins.
	exec(`UPDATE users SET timezone = 'America/New_York' WHERE id = $1`, owner.ID)
	if h := render(t); !strings.Contains(h, "(America/New_York)") {
		t.Fatal("issued-at is not in the user zone")
	}
	// An invalid organization zone falls through to the brand center
	// (Europe/Istanbul when it has none either).
	exec(`UPDATE users SET timezone = NULL WHERE id = $1`, owner.ID)
	exec(`UPDATE organizations SET timezone = 'Not/AZone' WHERE id = $1`, dealer.ID)
	if h := render(t); !strings.Contains(h, "("+pdfrender.Zone(centerTZ).String()+")") {
		t.Fatal("issued-at is not in the brand center zone")
	}
}
