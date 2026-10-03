package legacymobile

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	svcuc "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/services/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
)

// legacyRef is an {id, name} relation of MobileServiceResource.
type legacyRef struct {
	ID   *int64  `json:"id"`
	Name *string `json:"name"`
}

type legacyCustomer struct {
	ID    *int64  `json:"id"`
	Name  *string `json:"name"`
	Phone *string `json:"phone"`
	Email *string `json:"email"`
}

type legacyStockItem struct {
	ID          *int64  `json:"id"`
	Barcode     *string `json:"barcode"`
	SKU         *string `json:"sku"`
	ProductName *string `json:"product_name"`
}

type legacyItem struct {
	ID             *int64           `json:"id"`
	StockItemID    *int64           `json:"stock_item_id"`
	UsageType      string           `json:"usage_type"`
	UsageTypeLabel string           `json:"usage_type_label"`
	Notes          *string          `json:"notes"`
	StockItem      *legacyStockItem `json:"stock_item"`
}

type legacyImage struct {
	ID    *int64  `json:"id"`
	URL   string  `json:"url"`
	Title *string `json:"title"`
	Order int32   `json:"order"`
}

type legacyWarranty struct {
	ID            *int64           `json:"id"`
	StockItemID   *int64           `json:"stock_item_id"`
	StartDate     string           `json:"start_date"`
	EndDate       string           `json:"end_date"`
	IsActive      bool             `json:"is_active"`
	TotalDays     int              `json:"total_days"`
	DaysRemaining int              `json:"days_remaining"`
	IsExpired     bool             `json:"is_expired"`
	StockItem     *legacyStockItem `json:"stock_item"`
}

// legacyService is the hub's App\Http\Resources\Api\MobileServiceResource.
// The list loads dealer, customer, car brand/model and the counts; the
// detail loads creator, items, images and warranties instead of the
// counts (whenLoaded / whenCounted), so those keys are omitted where the
// hub omitted them. uuid is additive.
type legacyService struct {
	ID                     int64             `json:"id"`
	UUID                   uuid.UUID         `json:"uuid"`
	ServiceNo              string            `json:"service_no"`
	DealerID               *int64            `json:"dealer_id"`
	CustomerID             *int64            `json:"customer_id"`
	UserID                 *int64            `json:"user_id"`
	CarBrandID             *int64            `json:"car_brand_id"`
	CarModelID             *int64            `json:"car_model_id"`
	Year                   *int16            `json:"year"`
	VIN                    *string           `json:"vin"`
	Plate                  *string           `json:"plate"`
	PlateCountry           *string           `json:"plate_country"`
	KM                     *int32            `json:"km"`
	Package                *string           `json:"package"`
	AppliedParts           []string          `json:"applied_parts"`
	Notes                  *string           `json:"notes"`
	Status                 string            `json:"status"`
	StatusLabel            string            `json:"status_label"`
	CompletedAt            *string           `json:"completed_at"`
	ReviewRequestSMSSentAt *string           `json:"review_request_sms_sent_at"`
	CanSendReviewRequest   bool              `json:"can_send_review_request"`
	HasWarrantyRecords     *bool             `json:"has_warranty_records,omitempty"`
	Warranties             *[]legacyWarranty `json:"warranties,omitempty"`
	Dealer                 legacyRef         `json:"dealer"`
	Customer               legacyCustomer    `json:"customer"`
	Creator                *legacyRef        `json:"creator,omitempty"`
	CarBrand               legacyRef         `json:"car_brand"`
	CarModel               legacyRef         `json:"car_model"`
	Items                  *[]legacyItem     `json:"items,omitempty"`
	ItemsCount             *int              `json:"items_count,omitempty"`
	Images                 *[]legacyImage    `json:"images,omitempty"`
	ImagesCount            *int              `json:"images_count,omitempty"`
	CreatedAt              string            `json:"created_at"`
	UpdatedAt              string            `json:"updated_at"`
}

// iso writes a time like Carbon::toIso8601String (seconds, numeric offset).
func iso(t time.Time) string { return t.Format("2006-01-02T15:04:05-07:00") }

func isoPtr(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := iso(*t)
	return &s
}

func i64(v int64) *int64 { return &v }

func strPtr(s string) *string { return &s }

// usageLabel is enums.service_item_usage_types (tr, else en).
func usageLabel(locale, kind string) string {
	tr := strings.HasPrefix(strings.ToLower(locale), "tr")
	switch kind {
	case svcuc.KindFull:
		if tr {
			return "Tamamı"
		}
		return "Full"
	case svcuc.KindPartial:
		if tr {
			return "Parça"
		}
		return "Partial"
	}
	return kind
}

// legacyServiceFrom maps one new service view. row is the service row
// (zero when the store is not wired), items and images its rows (for the
// integer ids and the counts); detail selects the detail keys.
func legacyServiceFrom(locale string, v svcuc.ServiceView, row db.Service, items []db.ServiceItem,
	images []db.ServiceImage, warrantyIDs map[int64]int64, detail bool) legacyService {
	name := strings.TrimSpace(v.Customer.Name + " " + v.Customer.Surname)
	brand, model := v.CarBrand.Name, v.CarModel.Name
	org := v.Organization.Name
	out := legacyService{
		ID: row.ID, UUID: v.UUID, ServiceNo: v.ServiceNo,
		Year: v.ModelYear, VIN: v.VIN, Plate: v.Plate, PlateCountry: v.PlateCountry, KM: v.KM,
		Package: v.Package, Notes: v.Notes, Status: v.Status, StatusLabel: v.StatusLabel,
		CompletedAt: isoPtr(v.CompletedAt), AppliedParts: []string{},
		Dealer:    legacyRef{Name: &org},
		Customer:  legacyCustomer{Name: &name, Phone: v.Customer.Phone},
		CarBrand:  legacyRef{Name: &brand},
		CarModel:  legacyRef{Name: &model},
		CreatedAt: iso(v.CreatedAt), UpdatedAt: iso(v.UpdatedAt),
	}
	if row.ID != 0 {
		out.DealerID, out.Dealer.ID = i64(row.OrganizationID), i64(row.OrganizationID)
		out.CustomerID, out.Customer.ID = i64(row.CustomerUserID), i64(row.CustomerUserID)
		out.CarBrandID, out.CarBrand.ID = i64(row.CarBrandID), i64(row.CarBrandID)
		out.CarModelID, out.CarModel.ID = i64(row.CarModelID), i64(row.CarModelID)
		if row.CreatedByUserID.Valid {
			out.UserID = i64(row.CreatedByUserID.Int64)
		}
		if row.ReviewRequestSentAt.Valid {
			t := row.ReviewRequestSentAt.Time
			out.ReviewRequestSMSSentAt = isoPtr(&t)
		}
	}
	// The hub could send the review request SMS once, after completion.
	out.CanSendReviewRequest = v.Status == svcuc.StatusCompleted && out.ReviewRequestSMSSentAt == nil
	// applied_parts was a column of the service; it now lives on the items.
	seen := map[string]bool{}
	for _, it := range items {
		var parts []string
		_ = json.Unmarshal(it.AppliedParts, &parts)
		for _, p := range parts {
			if !seen[p] {
				seen[p] = true
				out.AppliedParts = append(out.AppliedParts, p)
			}
		}
	}
	if !detail {
		n, m := len(items), len(images)
		out.ItemsCount, out.ImagesCount = &n, &m
		return out
	}
	itemRows := map[uuid.UUID]db.ServiceItem{}
	for _, it := range items {
		itemRows[it.Uuid] = it
	}
	legacyItems := make([]legacyItem, 0, len(v.Items))
	for _, it := range v.Items {
		li := legacyItem{
			UsageType: it.Kind, UsageTypeLabel: usageLabel(locale, it.Kind), Notes: it.Notes,
			StockItem: &legacyStockItem{Barcode: strPtr(it.Barcode), SKU: strPtr(it.Product.SKU), ProductName: strPtr(it.Product.Name)},
		}
		if rowIt, found := itemRows[it.UUID]; found {
			li.ID, li.StockItemID, li.StockItem.ID = i64(rowIt.ID), i64(rowIt.UnitID), i64(rowIt.UnitID)
		}
		legacyItems = append(legacyItems, li)
	}
	out.Items = &legacyItems
	imageRows := map[uuid.UUID]db.ServiceImage{}
	for _, im := range images {
		imageRows[im.Uuid] = im
	}
	legacyImages := make([]legacyImage, 0, len(v.Images))
	for _, im := range v.Images {
		li := legacyImage{URL: im.URL, Title: im.Title, Order: im.SortOrder}
		if rowIm, found := imageRows[im.UUID]; found {
			li.ID = i64(rowIm.ID)
		}
		legacyImages = append(legacyImages, li)
	}
	out.Images = &legacyImages
	itemsByUUID := map[uuid.UUID]svcuc.ItemView{}
	for _, it := range v.Items {
		itemsByUUID[it.UUID] = it
	}
	warranties := make([]legacyWarranty, 0, len(v.Warranties))
	now := time.Now()
	for _, wv := range v.Warranties {
		lw := legacyWarranty{
			StartDate: wv.StartAt.Format("2006-01-02"), EndDate: wv.EndAt.Format("2006-01-02"),
			IsActive:  wv.Status == "active",
			TotalDays: int(math.Round(wv.EndAt.Sub(wv.StartAt).Hours() / 24)),
			IsExpired: wv.Status == "expired" || (wv.VoidedAt == nil && now.After(wv.EndAt)),
			StockItem: &legacyStockItem{ProductName: strPtr(wv.ProductName)},
		}
		if d := int(math.Ceil(wv.EndAt.Sub(now).Hours() / 24)); d > 0 {
			lw.DaysRemaining = d
		}
		if it, found := itemsByUUID[wv.ServiceItemUUID]; found {
			lw.StockItem.Barcode, lw.StockItem.SKU = strPtr(it.Barcode), strPtr(it.Product.SKU)
		}
		if rowIt, found := itemRows[wv.ServiceItemUUID]; found {
			lw.StockItemID, lw.StockItem.ID = i64(rowIt.UnitID), i64(rowIt.UnitID)
			if id, found := warrantyIDs[rowIt.ID]; found {
				lw.ID = i64(id)
			}
		}
		warranties = append(warranties, lw)
	}
	out.Warranties = &warranties
	has := len(warranties) > 0
	out.HasWarrantyRecords = &has
	out.Creator = &legacyRef{}
	return out
}

// legacyServiceOf loads the rows behind one view and maps it.
func (a *adapters) legacyServiceOf(ctx context.Context, locale string, v svcuc.ServiceView, detail bool) legacyService {
	var (
		row         db.Service
		items       []db.ServiceItem
		images      []db.ServiceImage
		warrantyIDs = map[int64]int64{}
	)
	if a.store != nil {
		if sc, found := orgctx.ScopeFrom(ctx); found {
			if r, err := a.store.GetServiceByUUID(ctx, db.GetServiceByUUIDParams{Uuid: v.UUID, BrandID: sc.BrandID}); err == nil {
				row = r
				items, _ = a.store.ListServiceItems(ctx, row.ID)
				images, _ = a.store.ListServiceImages(ctx, row.ID)
			}
		}
		if detail {
			for _, it := range items {
				if wr, err := a.store.GetWarrantyByServiceItem(ctx, it.ID); err == nil {
					warrantyIDs[it.ID] = wr.ID
				}
			}
		}
	}
	out := legacyServiceFrom(locale, v, row, items, images, warrantyIDs, detail)
	if detail && a.store != nil && row.ID != 0 {
		if u, err := a.store.GetUserByID(ctx, row.CustomerUserID); err == nil && u.Email.Valid {
			e := u.Email.String
			out.Customer.Email = &e
		}
		if row.CreatedByUserID.Valid {
			out.Creator.ID = i64(row.CreatedByUserID.Int64)
			if u, err := a.store.GetUserByID(ctx, row.CreatedByUserID.Int64); err == nil {
				n := strings.TrimSpace(u.Name + " " + u.Surname)
				out.Creator.Name = &n
			}
		}
	}
	return out
}

// legacyPageLink is one meta.links entry of a Laravel paginator.
type legacyPageLink struct {
	URL    *string `json:"url"`
	Label  string  `json:"label"`
	Active bool    `json:"active"`
}

type legacyPageLinks struct {
	First *string `json:"first"`
	Last  *string `json:"last"`
	Prev  *string `json:"prev"`
	Next  *string `json:"next"`
}

type legacyPageMeta struct {
	CurrentPage int              `json:"current_page"`
	From        *int             `json:"from"`
	LastPage    int              `json:"last_page"`
	Links       []legacyPageLink `json:"links"`
	Path        string           `json:"path"`
	PerPage     int              `json:"per_page"`
	To          *int             `json:"to"`
	Total       int64            `json:"total"`
}

// legacyPage is ResourceCollection::response()->getData(true) of a
// LengthAwarePaginator: {data, links, meta}.
type legacyPage struct {
	Data  []legacyService `json:"data"`
	Links legacyPageLinks `json:"links"`
	Meta  legacyPageMeta  `json:"meta"`
}

// requestPath is the absolute URL of the alias as the client called it
// (the paginator's path), behind the BFF when X-Forwarded-* say so.
func requestPath(r *http.Request) string {
	scheme := "https"
	if p := r.Header.Get("X-Forwarded-Proto"); p != "" {
		scheme = strings.TrimSpace(strings.Split(p, ",")[0])
	} else if r.TLS == nil {
		scheme = "http"
	}
	host := r.Host
	if h := r.Header.Get("X-Forwarded-Host"); h != "" {
		host = strings.TrimSpace(strings.Split(h, ",")[0])
	}
	path := r.URL.Path
	if p := r.Header.Get("X-Forwarded-Prefix"); p != "" {
		path = strings.TrimRight(p, "/") + path
	}
	return scheme + "://" + host + path
}

// paginate builds the Laravel paginator around data.
func paginate(base string, query url.Values, data []legacyService, total int64, page, perPage int) legacyPage {
	last := int(math.Max(1, math.Ceil(float64(total)/float64(perPage))))
	link := func(p int) *string {
		q := url.Values{}
		for k, v := range query {
			q[k] = v
		}
		q.Set("page", strconv.Itoa(p))
		s := base + "?" + q.Encode()
		return &s
	}
	out := legacyPage{Data: data, Meta: legacyPageMeta{
		CurrentPage: page, LastPage: last, Path: base, PerPage: perPage, Total: total,
	}}
	if out.Data == nil {
		out.Data = []legacyService{}
	}
	if len(data) > 0 {
		from, to := (page-1)*perPage+1, (page-1)*perPage+len(data)
		out.Meta.From, out.Meta.To = &from, &to
	}
	out.Links.First, out.Links.Last = link(1), link(last)
	if page > 1 {
		out.Links.Prev = link(page - 1)
	}
	if page < last {
		out.Links.Next = link(page + 1)
	}
	out.Meta.Links = append(out.Meta.Links, legacyPageLink{URL: out.Links.Prev, Label: "&laquo; Previous"})
	for p := 1; p <= last; p++ {
		// Laravel's window: the pages around the current one, plus the
		// first and last two, "..." for the gaps.
		near := p <= 2 || p > last-2 || (p >= page-3 && p <= page+3)
		if !near {
			if len(out.Meta.Links) > 0 && out.Meta.Links[len(out.Meta.Links)-1].Label != "..." {
				out.Meta.Links = append(out.Meta.Links, legacyPageLink{Label: "..."})
			}
			continue
		}
		out.Meta.Links = append(out.Meta.Links, legacyPageLink{URL: link(p), Label: strconv.Itoa(p), Active: p == page})
	}
	out.Meta.Links = append(out.Meta.Links, legacyPageLink{URL: out.Links.Next, Label: "Next &raquo;"})
	return out
}

// listServices is GET {Prefix}/services: ServiceController::index.
//
//	query    search, status, customer_id, per_page (1..100, 20), page
//	response {success, message: null, data: {data: [service], links, meta}}
//
// dealer_id, sort and direction are accepted and ignored: the list is the
// active organization's, newest first (the hub's default order).
func (a *adapters) listServices(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	locale := legacyLocale(r)
	perPage, page := 20, 1
	if s := strings.TrimSpace(q.Get("per_page")); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil || n < 1 || n > 100 {
			validationFailed(w, "per_page", "The per page field must be between 1 and 100.")
			return
		}
		perPage = n
	}
	if s := strings.TrimSpace(q.Get("page")); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 1 {
			page = n
		}
	}
	nq := url.Values{}
	nq.Set("limit", strconv.Itoa(perPage))
	nq.Set("offset", strconv.Itoa((page-1)*perPage))
	if s := strings.TrimSpace(q.Get("search")); s != "" {
		nq.Set("q", s)
	}
	if s := strings.TrimSpace(q.Get("status")); s != "" {
		nq.Set("status", s)
	}
	if s := strings.TrimSpace(q.Get("customer_id")); s != "" {
		id, err := strconv.ParseInt(s, 10, 64)
		var u db.User
		if err == nil && a.store != nil {
			u, err = a.store.GetUserByID(r.Context(), id)
		}
		if err != nil || a.store == nil {
			validationFailed(w, "customer_id", "The selected customer id is invalid.")
			return
		}
		nq.Set("customer_uuid", u.Uuid.String())
	}
	r2 := r.Clone(r.Context())
	r2.URL.RawQuery = nq.Encode()
	c := run(a.h.ListServices, r2)
	var pg apiquery.Page[svcuc.ServiceView]
	if !ok(c, &pg) {
		passThrough(w, c)
		return
	}
	data := make([]legacyService, 0, len(pg.Items))
	for _, v := range pg.Items {
		data = append(data, a.legacyServiceOf(r.Context(), locale, v, false))
	}
	keep := url.Values{}
	for k, v := range q {
		if k != "page" {
			keep[k] = v
		}
	}
	writeSuccess(w, http.StatusOK, "", paginate(requestPath(r), keep, data, pg.Total, page, perPage))
}

// errServiceNotFound is a {service} that is neither a uuid nor an id of
// the active brand.
var errServiceNotFound = errors.New("legacymobile: service not found")

// serviceUUID resolves the old {service} path value: the integer id the
// list returned, or a uuid.
func (a *adapters) serviceUUID(r *http.Request) (uuid.UUID, error) {
	raw := strings.TrimSpace(r.PathValue("service"))
	if id, err := uuid.Parse(raw); err == nil {
		return id, nil
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || n <= 0 || a.store == nil {
		return uuid.Nil, errServiceNotFound
	}
	sc, found := orgctx.ScopeFrom(r.Context())
	if !found {
		return uuid.Nil, errServiceNotFound
	}
	row, err := a.store.GetService(r.Context(), db.GetServiceParams{ID: n, BrandID: sc.BrandID})
	if err != nil {
		return uuid.Nil, errServiceNotFound
	}
	return row.Uuid, nil
}

// getService is GET {Prefix}/services/{service}: ServiceController::show,
// data = MobileServiceResource with items, images and warranties. Another
// organization's service is 404, as on /v1/services/{uuid}.
func (a *adapters) getService(w http.ResponseWriter, r *http.Request) {
	id, err := a.serviceUUID(r)
	if err != nil {
		response.NotFound(w, r, "Service not found")
		return
	}
	r2 := r.Clone(r.Context())
	r2.SetPathValue("uuid", id.String())
	c := run(a.h.GetService, r2)
	var v svcuc.ServiceView
	if !ok(c, &v) {
		passThrough(w, c)
		return
	}
	sort.SliceStable(v.Images, func(i, j int) bool { return v.Images[i].SortOrder < v.Images[j].SortOrder })
	writeSuccess(w, http.StatusOK, "", a.legacyServiceOf(r.Context(), legacyLocale(r), v, true))
}
