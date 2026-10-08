package httpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	sfuc "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stockforecast/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/features"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestIntegrationStockForecastOrderDraftAppendAndInsufficientData(t *testing.T) {
	it := newIntegration(t)
	ctx := context.Background()
	center := it.brandCenter("olex")
	dist := it.org("t485-dist", "distributor", center)
	dealer := it.org("t485-dealer", "dealer", dist)
	otherDist := it.org("t485-other-dist", "distributor", center)
	otherDealer := it.org("t485-other-dealer", "dealer", otherDist)
	if _, err := it.srv.features.SetByAdmin(ctx, 0, dist.ID, features.ModuleStockForecast, true); err != nil {
		t.Fatal(err)
	}
	if _, err := it.srv.features.SetByAdmin(ctx, 0, dealer.ID, features.ModuleStockForecast, true); err != nil {
		t.Fatal(err)
	}
	owner, pw := it.user("t485-dealer-owner")
	it.member(dealer, owner, "owner")
	tok := it.loginOrg(owner, pw, dealer)
	distOwner, dpw := it.user("t485-dist-owner")
	it.member(dist, distOwner, "owner")
	distTok := it.loginOrg(distOwner, dpw, dist)
	p := it.product(center, "T485")
	it.setListPrice(p, "TRY", "10")
	if _, err := it.q.UpsertDistributorDealerPrice(ctx, db.UpsertDistributorDealerPriceParams{
		ProductID: p.ID, BrandID: p.BrandID, DistributorOrgID: dist.ID, Currency: "TRY", Price: "12",
	}); err != nil {
		t.Fatal(err)
	}
	seedForecast(t, it.q, dealer, p, sfuc.StatusCritical, 2)

	body := map[string]any{"items": []any{map[string]any{"product_uuid": p.Uuid.String(), "quantity": 2}}}
	code, env := it.do("POST", "/v1/stock-forecasts/order-draft", hostOlex, tok, body)
	if code != http.StatusCreated {
		msg := ""
		if env.Error != nil {
			msg = env.Error.Message
		}
		t.Fatalf("first draft = %d %s %s", code, errCode(env), msg)
	}
	var first struct {
		Created bool      `json:"created"`
		Order   orderView `json:"order"`
	}
	if err := json.Unmarshal(env.Data, &first); err != nil {
		t.Fatal(err)
	}
	code, env = it.do("POST", "/v1/stock-forecasts/order-draft", hostOlex, tok,
		map[string]any{"items": []any{map[string]any{"product_uuid": p.Uuid.String(), "quantity": 3}}})
	if code != http.StatusCreated {
		t.Fatalf("second draft = %d %s", code, errCode(env))
	}
	var second struct {
		Created bool      `json:"created"`
		Order   orderView `json:"order"`
	}
	if err := json.Unmarshal(env.Data, &second); err != nil {
		t.Fatal(err)
	}
	if !first.Created || second.Created || second.Order.UUID != first.Order.UUID || len(second.Order.Items) != 1 ||
		second.Order.Items[0].Quantity == nil || *second.Order.Items[0].Quantity != 5 {
		t.Fatalf("draft merge first=%+v second=%+v", first, second)
	}

	noData := it.product(center, "T485N")
	it.setListPrice(noData, "TRY", "10")
	if _, err := it.q.UpsertDistributorDealerPrice(ctx, db.UpsertDistributorDealerPriceParams{
		ProductID: noData.ID, BrandID: noData.BrandID, DistributorOrgID: dist.ID, Currency: "TRY", Price: "12",
	}); err != nil {
		t.Fatal(err)
	}
	seedForecast(t, it.q, dealer, noData, sfuc.StatusInsufficientData, 0)
	code, env = it.do("POST", "/v1/stock-forecasts/order-draft", hostOlex, tok,
		map[string]any{"items": []any{map[string]any{"product_uuid": noData.Uuid.String(), "quantity": 1}}})
	if code != http.StatusUnprocessableEntity || errCode(env) != sfuc.CodeInsufficientData {
		t.Fatalf("insufficient data = %d %s", code, errCode(env))
	}

	seedForecast(t, it.q, otherDealer, p, sfuc.StatusCritical, 2)
	code, env = it.do("GET", "/v1/stock-forecasts/subtree", hostOlex, distTok, nil)
	if code != http.StatusOK {
		t.Fatalf("subtree = %d %s", code, errCode(env))
	}
	var page struct {
		Items []struct {
			OrganizationUUID string `json:"organization_uuid"`
		} `json:"items"`
	}
	if err := json.Unmarshal(env.Data, &page); err != nil {
		t.Fatal(err)
	}
	for _, row := range page.Items {
		if row.OrganizationUUID == otherDealer.Uuid.String() {
			t.Fatalf("foreign subtree dealer leaked: %+v", page.Items)
		}
	}
}

func seedForecast(t *testing.T, q *db.Queries, org db.Organization, p db.Product, status string, suggested int32) {
	t.Helper()
	arg := db.UpsertStockForecastSnapshotParams{
		OrganizationID: org.ID, BrandID: org.BrandID, ProductID: p.ID,
		ComputedOn: pgtype.Date{Time: time.Now().UTC(), Valid: true}, IsLatest: true,
		OnHandQty: 1, OnHandMeters: scanNum("0"), AvgDaily30: scanNum("1.0000"),
		AvgDaily90: scanNum("1.0000"), SeasonalityFactor: scanNum("1.000"),
		DaysLeft: scanNum("1.00"), DataDays: 120, Status: status,
	}
	if suggested > 0 {
		arg.SuggestedQty = pgtype.Int4{Int32: suggested, Valid: true}
	}
	if _, err := q.UpsertStockForecastSnapshot(context.Background(), arg); err != nil {
		t.Fatal(err)
	}
}

func scanNum(s string) pgtype.Numeric {
	var n pgtype.Numeric
	_ = n.Scan(s)
	return n
}
