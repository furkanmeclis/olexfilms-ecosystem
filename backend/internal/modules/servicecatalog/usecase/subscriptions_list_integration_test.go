package usecase

import (
	"errors"
	"net/url"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
)

func mustSubFilter(t *testing.T, raw string) SubscriptionListFilter {
	t.Helper()
	values, _ := url.ParseQuery(raw)
	f, err := ParseSubscriptionListFilter(values)
	if err != nil {
		t.Fatalf("parse %q: %v", raw, err)
	}
	return f
}

func TestSubscriptionListContract(t *testing.T) {
	e := newSubEnv(t)
	start, end := subDates()
	cheap := e.item(t, "software", "50.00")
	dear := e.item(t, "training", "300.00")
	center := e.caller(e.center, rbac.ScopeBrand, rbac.PermServiceSubscriptionsAssign)
	assign := func(item db.ServiceCatalogItem, org db.Organization) SubscriptionView {
		t.Helper()
		v, err := e.svc.Assign(e.ctx, center, SubscriptionInput{ItemUUID: item.Uuid, OrganizationUUID: org.Uuid, StartsOn: start, EndsOn: end})
		if err != nil {
			t.Fatalf("assign: %v", err)
		}
		return v
	}
	a := assign(cheap, e.dealer)
	b := assign(dear, e.dealer)
	c := assign(cheap, e.other)

	read := e.caller(e.center, rbac.ScopeBrand, rbac.PermServiceSubscriptionsRead)
	scope := "&organization_uuid=" + e.dealer.Uuid.String() + "," + e.other.Uuid.String()
	items, total, err := e.svc.ListSubscriptions(e.ctx, read, mustSubFilter(t, "sort=price"+scope))
	if err != nil || total != 3 || len(items) != 3 {
		t.Fatalf("list = %d/%d, %v", len(items), total, err)
	}
	if items[0].Price != "50.00" || items[2].UUID != b.UUID {
		t.Fatalf("price asc order = %+v", items)
	}
	// Equal prices: id tiebreak keeps assignment order.
	if items[0].UUID != a.UUID || items[1].UUID != c.UUID {
		t.Fatalf("tiebreak order = %s %s", items[0].UUID, items[1].UUID)
	}
	if items[0].ItemName != cheap.Name || items[0].OrganizationName != e.dealer.Name {
		t.Fatalf("names = %q / %q", items[0].ItemName, items[0].OrganizationName)
	}
	items, _, _ = e.svc.ListSubscriptions(e.ctx, read, mustSubFilter(t, "sort=-price"+scope))
	if items[0].UUID != b.UUID || items[1].UUID != c.UUID {
		t.Fatalf("price desc order = %s %s", items[0].UUID, items[1].UUID)
	}

	items, total, _ = e.svc.ListSubscriptions(e.ctx, read, mustSubFilter(t, "item_uuid="+dear.Uuid.String()+scope))
	if total != 1 || items[0].UUID != b.UUID {
		t.Fatalf("item filter = %d", total)
	}
	_, total, _ = e.svc.ListSubscriptions(e.ctx, read, mustSubFilter(t, "q=other"+scope))
	if total != 1 {
		t.Fatalf("q filter total = %d, want 1", total)
	}
	_, total, _ = e.svc.ListSubscriptions(e.ctx, read, mustSubFilter(t, "ends_on_from=2027-10-05"+scope))
	if total != 0 {
		t.Fatalf("ends_on_from filter total = %d, want 0", total)
	}

	if _, err := e.svc.RequestCancel(e.ctx, e.caller(e.dealer, rbac.ScopeManaged, rbac.PermServiceSubscriptionsCancelRequest),
		a.UUID, CancelRequestInput{Reason: "budget"}); err != nil {
		t.Fatalf("cancel request: %v", err)
	}
	items, total, _ = e.svc.ListSubscriptions(e.ctx, read, mustSubFilter(t, "status=cancel_requested,cancelled"+scope))
	if total != 1 || items[0].UUID != a.UUID {
		t.Fatalf("status filter = %d", total)
	}

	// The dealer's managed scope only reaches its own subscriptions.
	dealerRead := e.caller(e.dealer, rbac.ScopeManaged, rbac.PermServiceSubscriptionsRead)
	_, total, _ = e.svc.ListSubscriptions(e.ctx, dealerRead, mustSubFilter(t, ""))
	if total != 2 {
		t.Fatalf("dealer total = %d, want 2", total)
	}

	values, _ := url.ParseQuery("sort=reason")
	var ve *apiquery.ValidationError
	if _, err := ParseSubscriptionListFilter(values); !errors.As(err, &ve) {
		t.Fatalf("unknown sort = %v, want validation", err)
	}
	values, _ = url.ParseQuery("status=bogus")
	if _, err := ParseSubscriptionListFilter(values); !errors.As(err, &ve) {
		t.Fatalf("bad status = %v, want validation", err)
	}
}

func TestCancelRequestQueueAndPricePreview(t *testing.T) {
	e := newSubEnv(t)
	start, end := subDates()
	item := e.item(t, "software", "100.00")
	if _, err := e.q.UpsertServicePriceOverride(e.ctx, db.UpsertServicePriceOverrideParams{
		ItemID: item.ID, OrganizationID: e.dist.ID, BrandID: item.BrandID, Price: num("80.00"), Currency: "TRY",
	}); err != nil {
		t.Fatalf("override: %v", err)
	}
	centerAssign := e.caller(e.center, rbac.ScopeBrand, rbac.PermServiceSubscriptionsAssign)
	p, err := e.svc.PreviewPrice(e.ctx, centerAssign, item.Uuid, e.dealer.Uuid)
	if err != nil || p.Amount != "80.00" || p.Source != "override" {
		t.Fatalf("dealer preview = %+v, %v", p, err)
	}
	p, err = e.svc.PreviewPrice(e.ctx, centerAssign, item.Uuid, e.other.Uuid)
	if err != nil || p.Amount != "100.00" || p.Source != "default" {
		t.Fatalf("other preview = %+v, %v", p, err)
	}
	dist := e.caller(e.dist, rbac.ScopeSubtree, rbac.PermServiceSubscriptionsAssign)
	if _, err := e.svc.PreviewPrice(e.ctx, dist, item.Uuid, e.other.Uuid); !errors.Is(err, ErrNotFound) {
		t.Fatalf("distributor preview outside subtree = %v", err)
	}
	if _, err := e.svc.PreviewPrice(e.ctx, e.caller(e.dealer, rbac.ScopeManaged, rbac.PermServiceSubscriptionsAssign),
		item.Uuid, e.dealer.Uuid); !errors.Is(err, ErrForbidden) {
		t.Fatalf("dealer preview = %v", err)
	}

	sub1, err := e.svc.Assign(e.ctx, centerAssign, SubscriptionInput{ItemUUID: item.Uuid, OrganizationUUID: e.dealer.Uuid, StartsOn: start, EndsOn: end})
	if err != nil {
		t.Fatal(err)
	}
	sub2, err := e.svc.Assign(e.ctx, centerAssign, SubscriptionInput{ItemUUID: item.Uuid, OrganizationUUID: e.dealer.Uuid, StartsOn: start, EndsOn: end})
	if err != nil {
		t.Fatal(err)
	}
	dealer := e.caller(e.dealer, rbac.ScopeManaged, rbac.PermServiceSubscriptionsCancelRequest)
	r1, err := e.svc.RequestCancel(e.ctx, dealer, sub1.UUID, CancelRequestInput{Reason: "first reason " + e.suffix})
	if err != nil {
		t.Fatal(err)
	}
	r2, err := e.svc.RequestCancel(e.ctx, dealer, sub2.UUID, CancelRequestInput{Reason: "second reason " + e.suffix})
	if err != nil {
		t.Fatal(err)
	}
	center := e.caller(e.center, rbac.ScopeBrand, rbac.PermServiceSubscriptionsCancelApprove)
	if _, err := e.svc.RejectCancel(e.ctx, center, r2.UUID, DecisionInput{}); err != nil {
		t.Fatal(err)
	}

	parse := func(raw string) CancelRequestListFilter {
		values, _ := url.ParseQuery(raw)
		f, err := ParseCancelRequestListFilter(values)
		if err != nil {
			t.Fatalf("parse %q: %v", raw, err)
		}
		return f
	}
	rows, total, err := e.svc.ListCancelRequests(e.ctx, center, parse("q="+e.suffix+"&sort=created_at"))
	if err != nil || total != 2 || rows[0].UUID != r1.UUID || rows[1].UUID != r2.UUID {
		t.Fatalf("queue asc = %d, %v", total, err)
	}
	if rows[0].OrganizationName != e.dealer.Name || rows[0].ItemName != item.Name || rows[0].SubscriptionUUID != sub1.UUID {
		t.Fatalf("queue row = %+v", rows[0])
	}
	rows, _, _ = e.svc.ListCancelRequests(e.ctx, center, parse("q="+e.suffix+"&sort=-created_at"))
	if rows[0].UUID != r2.UUID {
		t.Fatalf("queue desc first = %s", rows[0].UUID)
	}
	rows, total, _ = e.svc.ListCancelRequests(e.ctx, center, parse("q="+e.suffix+"&status=pending"))
	if total != 1 || rows[0].UUID != r1.UUID || rows[0].SubscriptionStatus != StatusCancelRequested {
		t.Fatalf("pending queue = %d", total)
	}
	if _, _, err := e.svc.ListCancelRequests(e.ctx, e.caller(e.dist, rbac.ScopeSubtree, rbac.PermServiceSubscriptionsCancelApprove),
		parse("")); !errors.Is(err, ErrForbidden) {
		t.Fatalf("distributor queue = %v, want ErrForbidden", err)
	}
	values, _ := url.ParseQuery("sort=reason")
	if _, err := ParseCancelRequestListFilter(values); err == nil {
		t.Fatal("unknown sort accepted")
	}
}
