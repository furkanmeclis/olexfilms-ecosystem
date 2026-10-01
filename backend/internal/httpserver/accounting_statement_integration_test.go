package httpserver

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/config"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	accountingusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/accounting/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/documents/documentstest"
	exportusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/exports/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/pdfrender"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/storage"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/queue"
	"github.com/hibiken/asynq"
)

type stmtLine struct {
	Date        string `json:"date"`
	Direction   string `json:"direction"`
	Description string `json:"description"`
	SourceLabel string `json:"source_label"`
	Debit       string `json:"debit"`
	Credit      string `json:"credit"`
	Balance     string `json:"balance"`
}

type stmt struct {
	Currency       string     `json:"currency"`
	OpeningBalance string     `json:"opening_balance"`
	TotalDebit     string     `json:"total_debit"`
	TotalCredit    string     `json:"total_credit"`
	ClosingBalance string     `json:"closing_balance"`
	Lines          []stmtLine `json:"lines"`
}

type balReport struct {
	Cari []struct {
		UUID    string `json:"uuid"`
		Balance string `json:"balance"`
	} `json:"cari"`
	Accounts []struct {
		UUID    string `json:"uuid"`
		Balance string `json:"balance"`
	} `json:"accounts"`
	Totals struct {
		Receivable string `json:"receivable"`
		Payable    string `json:"payable"`
		Cash       string `json:"cash"`
	} `json:"totals"`
}

type exportJob struct {
	UUID        string  `json:"uuid"`
	Resource    string  `json:"resource"`
	Format      string  `json:"format"`
	Status      string  `json:"status"`
	DownloadURL *string `json:"download_url"`
}

// backdate writes a manual ledger row at a past instant (the API always
// writes "now"; finance_entries only refuses UPDATE/DELETE).
func (it *itest) backdate(org db.Organization, cariUUID, direction, category, amount, at string) {
	it.t.Helper()
	_, err := it.pool.Exec(context.Background(), `
		INSERT INTO finance_entries (organization_id, brand_id, cari_id, direction, category,
		    orig_currency, orig_amount, currency, amount, rate, rate_date,
		    source_type, source_uuid, description, created_at)
		SELECT $1::bigint, $2::bigint, c.id, $4::text, $5::text, $6::text, $7::numeric, $6::text, $7::numeric,
		       1, ($8::timestamptz)::date, 'manual', gen_random_uuid(), 'TEC-175 ' || $5::text, $8::timestamptz
		FROM cari_accounts c WHERE c.uuid = $3::uuid AND c.organization_id = $1::bigint`,
		org.ID, org.BrandID, cariUUID, direction, category, org.Currency, amount, at)
	if err != nil {
		it.t.Fatalf("backdate %s %s: %v", direction, amount, err)
	}
}

// TEC-175 acceptance: opening, period lines, running and closing balance
// for known rows; rows before the period are in the opening balance; a
// dealer reads its own statement but never another organization's; the PDF
// export job lands on the exports queue (worker-docs) and renders through a
// mock Gotenberg in the user language (ar: RTL).
func TestIntegrationAccountingStatement(t *testing.T) {
	gotb := documentstest.NewGotenberg(t, 0)
	store := storage.NewMemory()
	mr := miniredis.RunT(t)
	qc := queue.NewClient(config.RedisConfig{Addr: mr.Addr()})
	t.Cleanup(func() { _ = qc.Close() })
	it := newIntegrationWithDeps(t,
		func(c *config.Config) { c.Gotenberg.URL = gotb.URL },
		func(d *Deps) { d.Storage = store; d.Queue = qc })

	center := it.brandCenter("olex")
	dist := it.org("t175-dist", "distributor", center)
	dealer := it.org("t175-dealer", "dealer", dist)
	accUser, apw := it.user("t175-acc")
	it.member(center, accUser, "staff", rbac.RoleCenterAccounting)
	dealerOwner, rpw := it.user("t175-dealer-owner")
	it.member(dealer, dealerOwner, "owner")
	tok := it.loginOrg(accUser, apw, center)

	// The cari opens with a manual charge today (1500), then known rows in
	// 2020: charge 1000 (Jan), collection 300 (Feb) before the period;
	// payment 50 and expense 200 in March; charge 400 in April after it.
	charge := decodeData[accEntry](t, it.accDo("POST", "/v1/accounting/entries", tok, map[string]any{
		"direction": "charge", "category": "opening_balance", "amount": "1500.00",
		"counterparty_organization_uuid": dist.Uuid.String(),
	}, http.StatusCreated))
	cariUUID := *charge.CariUUID
	it.backdate(center, cariUUID, "charge", "opening_balance", "1000.00", "2020-01-10T09:00:00Z")
	it.backdate(center, cariUUID, "collection", "collection", "300.00", "2020-02-15T09:00:00Z")
	it.backdate(center, cariUUID, "payment", "payment", "50.00", "2020-03-05T09:00:00Z")
	it.backdate(center, cariUUID, "expense", "rent", "200.00", "2020-03-20T09:00:00Z")
	it.backdate(center, cariUUID, "charge", "adjustment", "400.00", "2020-04-02T09:00:00Z")
	stmtPath := "/v1/accounting/cari/" + cariUUID + "/statement"

	// 1. March 2020: opening = 1000 - 300; payment is a debit, expense a credit.
	st := decodeData[stmt](t, it.accDo("GET", stmtPath+"?from=2020-03-01&to=2020-03-31&locale=en", tok, nil, http.StatusOK))
	if st.OpeningBalance != "700.00" || st.TotalDebit != "50.00" || st.TotalCredit != "200.00" || st.ClosingBalance != "550.00" {
		t.Fatalf("march statement = %+v", st)
	}
	if len(st.Lines) != 2 ||
		st.Lines[0].Direction != "payment" || st.Lines[0].Debit != "50.00" || st.Lines[0].Credit != "0.00" || st.Lines[0].Balance != "750.00" ||
		st.Lines[1].Direction != "expense" || st.Lines[1].Credit != "200.00" || st.Lines[1].Balance != "550.00" {
		t.Fatalf("march lines = %+v", st.Lines)
	}
	if st.Lines[0].SourceLabel != "Manual entry" || st.Lines[0].Date[:10] != "2020-03-05" {
		t.Fatalf("march line labels = %+v", st.Lines[0])
	}

	// 2. Whole ledger: opening 0, six rows in ledger order, running sum.
	all := decodeData[stmt](t, it.accDo("GET", stmtPath, tok, nil, http.StatusOK))
	wantBal := []string{"1000.00", "700.00", "750.00", "550.00", "950.00", "2450.00"}
	if all.OpeningBalance != "0.00" || all.ClosingBalance != "2450.00" || len(all.Lines) != len(wantBal) {
		t.Fatalf("full statement = %+v", all)
	}
	for i, b := range wantBal {
		if all.Lines[i].Balance != b {
			t.Fatalf("line %d balance = %s, want %s", i, all.Lines[i].Balance, b)
		}
	}
	if all.TotalDebit != "2950.00" || all.TotalCredit != "500.00" {
		t.Fatalf("full totals = %s / %s", all.TotalDebit, all.TotalCredit)
	}
	// Only "from": everything before it is the opening balance.
	after := decodeData[stmt](t, it.accDo("GET", stmtPath+"?from=2020-04-01", tok, nil, http.StatusOK))
	if after.OpeningBalance != "550.00" || after.ClosingBalance != "2450.00" || len(after.Lines) != 2 {
		t.Fatalf("statement from april = %+v", after)
	}
	it.accDo("GET", stmtPath+"?from=2020-04-01&to=2020-03-01", tok, nil, http.StatusBadRequest)

	// 3. Balance report: now and as of 2020-03-31.
	cash := decodeData[accAccount](t, it.accDo("POST", "/v1/accounting/accounts", tok, map[string]any{
		"type": "cash", "name": "Cash t175 " + it.suffix,
	}, http.StatusCreated))
	it.accDo("POST", "/v1/accounting/collections", tok, map[string]any{
		"account_uuid": cash.UUID, "cari_uuid": cariUUID, "amount": "100.00",
	}, http.StatusCreated)
	findCari := func(r balReport) string {
		for _, c := range r.Cari {
			if c.UUID == cariUUID {
				return c.Balance
			}
		}
		return "missing"
	}
	findCash := func(r balReport) string {
		for _, a := range r.Accounts {
			if a.UUID == cash.UUID {
				return a.Balance
			}
		}
		return "missing"
	}
	now := decodeData[balReport](t, it.accDo("GET", "/v1/accounting/reports/balances", tok, nil, http.StatusOK))
	if findCari(now) != "2350.00" || findCash(now) != "100.00" {
		t.Fatalf("balances now: cari %s cash %s", findCari(now), findCash(now))
	}
	past := decodeData[balReport](t, it.accDo("GET", "/v1/accounting/reports/balances?as_of=2020-03-31", tok, nil, http.StatusOK))
	if findCari(past) != "550.00" || findCash(past) != "0.00" {
		t.Fatalf("balances as of 2020-03-31: cari %s cash %s", findCari(past), findCash(past))
	}

	// 4. Dealer scope: its own cari reads; the center's cari and book do not.
	var dealerCari string
	if err := it.pool.QueryRow(context.Background(), `
		INSERT INTO cari_accounts (organization_id, brand_id, counterparty_type, counterparty_org_id, currency)
		VALUES ($1, $2, 'organization', $3, $4) RETURNING uuid::text`,
		dealer.ID, dealer.BrandID, dist.ID, dealer.Currency).Scan(&dealerCari); err != nil {
		t.Fatalf("dealer cari: %v", err)
	}
	it.backdate(dealer, dealerCari, "expense", "purchase", "300.00", "2020-05-01T09:00:00Z")
	dealerTok := it.loginOrg(dealerOwner, rpw, dealer)
	own := decodeData[stmt](t, it.accDo("GET", "/v1/accounting/cari/"+dealerCari+"/statement", dealerTok, nil, http.StatusOK))
	if own.ClosingBalance != "-300.00" || len(own.Lines) != 1 || own.Lines[0].Credit != "300.00" {
		t.Fatalf("dealer own statement = %+v", own)
	}
	it.accDo("GET", stmtPath, dealerTok, nil, http.StatusNotFound)
	it.accDo("GET", stmtPath+"?organization_uuid="+center.Uuid.String(), dealerTok, nil, http.StatusNotFound)
	it.accDo("GET", "/v1/accounting/reports/balances?organization_uuid="+dist.Uuid.String(), dealerTok, nil, http.StatusNotFound)
	it.accDo("POST", stmtPath+"/export", dealerTok, map[string]any{"format": "pdf"}, http.StatusNotFound)
	// The center reads the distributor's book (below it).
	it.accDo("GET", "/v1/accounting/reports/balances?organization_uuid="+dist.Uuid.String(), tok, nil, http.StatusOK)

	// 5. Export jobs go to the exports queue (worker-docs); a worker with a
	// mock Gotenberg renders them.
	it.accDo("POST", stmtPath+"/export", tok, map[string]any{"format": "docx"}, http.StatusBadRequest)
	pdfJob := decodeData[exportJob](t, it.accDo("POST", stmtPath+"/export", tok, map[string]any{
		"format": "pdf", "from": "2020-03-01", "to": "2020-03-31", "locale": "ar",
	}, http.StatusAccepted))
	csvJob := decodeData[exportJob](t, it.accDo("POST", stmtPath+"/export", tok, map[string]any{
		"format": "csv", "from": "2020-03-01", "to": "2020-03-31", "locale": "tr",
	}, http.StatusAccepted))
	balJob := decodeData[exportJob](t, it.accDo("POST", "/v1/accounting/reports/balances/export", tok, map[string]any{
		"format": "xlsx", "as_of": "2020-03-31",
	}, http.StatusAccepted))
	if pdfJob.Resource != accountingusecase.ResourceCariStatement || pdfJob.Status == "completed" {
		t.Fatalf("pdf job = %+v", pdfJob)
	}
	insp := asynq.NewInspector(asynq.RedisClientOpt{Addr: mr.Addr()})
	t.Cleanup(func() { _ = insp.Close() })
	tasks, err := insp.ListPendingTasks(queue.QueueExports)
	if err != nil || len(tasks) != 3 {
		t.Fatalf("pending export tasks = %d (%v)", len(tasks), err)
	}

	acctSvc := accountingusecase.New(it.pool, it.q, nil, nil)
	reg := ioengine.NewRegistry(accountingusecase.NewStatementAdapter(acctSvc), accountingusecase.NewBalancesAdapter(acctSvc))
	worker := exportusecase.New(it.q, store, reg, nil, nil, nil, nil)
	worker.SetDocumentPDF(pdfrender.New(gotb.URL))
	for _, task := range tasks {
		if task.Type != queue.TaskExportProcess {
			t.Fatalf("task type %s", task.Type)
		}
		p, err := queue.ParseExportProcessPayload(task.Payload)
		if err != nil {
			t.Fatal(err)
		}
		if err := worker.ProcessExport(context.Background(), p.ExportJobID); err != nil {
			t.Fatalf("process export %d: %v", p.ExportJobID, err)
		}
	}
	if gotb.Calls.Load() < 1 {
		t.Fatal("gotenberg was not called for the pdf export")
	}
	arHTML := gotb.LastHTML()
	if !strings.Contains(arHTML, `dir="rtl"`) || !strings.Contains(arHTML, "كشف حساب") || !strings.Contains(arHTML, "550.00") || !strings.Contains(arHTML, center.Name) {
		t.Fatal("arabic statement document missing (rtl, title, closing balance, letterhead)")
	}

	download := func(job exportJob, token string) (int, string, string) {
		done := decodeData[exportJob](t, it.accDo("GET", "/v1/accounting/exports/"+job.UUID, token, nil, http.StatusOK))
		if done.Status != "completed" || done.DownloadURL == nil || *done.DownloadURL != "/v1/accounting/exports/"+job.UUID+"/download" {
			t.Fatalf("job %s = %+v", job.UUID, done)
		}
		rec := it.raw("GET", *done.DownloadURL, token, "", nil, nil)
		return rec.Code, rec.Header().Get("Content-Type"), rec.Body.String()
	}
	if code, ct, body := download(pdfJob, tok); code != http.StatusOK || ct != "application/pdf" || body != documentstest.PDF {
		t.Fatalf("pdf download = %d %s %q", code, ct, body)
	}
	if code, _, body := download(csvJob, tok); code != http.StatusOK ||
		!strings.Contains(body, "Tarih,Açıklama,Kaynak,Borç,Alacak,Bakiye") || !strings.Contains(body, "Kapanış bakiyesi") ||
		!strings.Contains(body, "550.00") {
		t.Fatalf("csv download = %d %q", code, body)
	}
	if code, ct, _ := download(balJob, tok); code != http.StatusOK || !strings.Contains(ct, "spreadsheet") {
		t.Fatalf("xlsx download = %d %s", code, ct)
	}
	// Another organization never sees the job.
	it.accDo("GET", "/v1/accounting/exports/"+pdfJob.UUID, dealerTok, nil, http.StatusNotFound)
	if rec := it.raw("GET", "/v1/accounting/exports/"+pdfJob.UUID+"/download", dealerTok, "", nil, nil); rec.Code != http.StatusNotFound {
		t.Fatalf("dealer download of center job = %d", rec.Code)
	}
}
