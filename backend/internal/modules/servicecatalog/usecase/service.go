// Package usecase implements the non-product service catalog (TEC-306).
package usecase

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	pricing "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/pricing/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

const (
	CategoryModuleBundle = "module_bundle"
)

var (
	ErrForbidden       = errors.New("service catalog: forbidden")
	ErrNotFound        = errors.New("service catalog: not found")
	ErrInUse           = errors.New("service catalog: item has subscriptions")
	ErrModuleOnly      = errors.New("service catalog: item is not a module bundle")
	ErrInvalidBuyerOrg = errors.New("service catalog: invalid buyer organization")

	currencyRe = regexp.MustCompile(`^[A-Z]{3}$`)
	priceRe    = regexp.MustCompile(`^[0-9]{1,16}(\.[0-9]{1,2})?$`)
)

type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string { return e.Field + ": " + e.Message }

func invalid(field, msg string) error { return &ValidationError{Field: field, Message: msg} }

type Store interface {
	CreateServiceCatalogItem(ctx context.Context, arg db.CreateServiceCatalogItemParams) (db.ServiceCatalogItem, error)
	GetServiceCatalogItemByUUID(ctx context.Context, arg db.GetServiceCatalogItemByUUIDParams) (db.ServiceCatalogItem, error)
	GetServiceCatalogItem(ctx context.Context, arg db.GetServiceCatalogItemParams) (db.ServiceCatalogItem, error)
	ListServiceCatalogItems(ctx context.Context, arg db.ListServiceCatalogItemsParams) ([]db.ServiceCatalogItem, error)
	UpdateServiceCatalogItem(ctx context.Context, arg db.UpdateServiceCatalogItemParams) (db.ServiceCatalogItem, error)
	AddServiceCatalogModule(ctx context.Context, arg db.AddServiceCatalogModuleParams) error
	DeleteServiceCatalogModules(ctx context.Context, itemID int64) (int64, error)
	ListServiceCatalogModules(ctx context.Context, itemID int64) ([]string, error)
	UpsertServicePriceOverride(ctx context.Context, arg db.UpsertServicePriceOverrideParams) (db.ServicePriceOverride, error)
	DeleteServicePriceOverride(ctx context.Context, arg db.DeleteServicePriceOverrideParams) (int64, error)
	GetServicePriceOverride(ctx context.Context, arg db.GetServicePriceOverrideParams) (db.ServicePriceOverride, error)
	ListServicePriceOverrides(ctx context.Context, arg db.ListServicePriceOverridesParams) ([]db.ServicePriceOverride, error)
	ListServicePriceOverridesForItems(ctx context.Context, arg db.ListServicePriceOverridesForItemsParams) ([]db.ServicePriceOverride, error)
	CountServiceSubscriptionsByItem(ctx context.Context, arg db.CountServiceSubscriptionsByItemParams) (int64, error)
	GetContractTemplateByID(ctx context.Context, id int64) (db.ContractTemplate, error)
	GetContractTemplateLocale(ctx context.Context, arg db.GetContractTemplateLocaleParams) (db.ContractTemplateLocale, error)
	GetContractInstanceByID(ctx context.Context, id int64) (db.ContractInstance, error)
	CountActiveServiceModuleSubscriptions(ctx context.Context, arg db.CountActiveServiceModuleSubscriptionsParams) (int64, error)
	CreateServiceSubscription(ctx context.Context, arg db.CreateServiceSubscriptionParams) (db.ServiceSubscription, error)
	SetServiceSubscriptionContract(ctx context.Context, arg db.SetServiceSubscriptionContractParams) (db.ServiceSubscription, error)
	GetServiceSubscriptionByUUID(ctx context.Context, arg db.GetServiceSubscriptionByUUIDParams) (db.ServiceSubscription, error)
	ListServiceSubscriptionsPage(ctx context.Context, arg db.ListServiceSubscriptionsPageParams) ([]db.ListServiceSubscriptionsPageRow, error)
	CountServiceSubscriptionsPage(ctx context.Context, arg db.CountServiceSubscriptionsPageParams) (int64, error)
	ListServiceSubscriptionCancelRequestsPage(ctx context.Context, arg db.ListServiceSubscriptionCancelRequestsPageParams) ([]db.ListServiceSubscriptionCancelRequestsPageRow, error)
	CountServiceSubscriptionCancelRequestsPage(ctx context.Context, arg db.CountServiceSubscriptionCancelRequestsPageParams) (int64, error)
	SetServiceSubscriptionCancelRequested(ctx context.Context, arg db.SetServiceSubscriptionCancelRequestedParams) (db.ServiceSubscription, error)
	SetServiceSubscriptionStatus(ctx context.Context, arg db.SetServiceSubscriptionStatusParams) (db.ServiceSubscription, error)
	CreateServiceSubscriptionCancelRequest(ctx context.Context, arg db.CreateServiceSubscriptionCancelRequestParams) (db.ServiceSubscriptionCancelRequest, error)
	GetServiceSubscriptionCancelRequestByUUID(ctx context.Context, arg db.GetServiceSubscriptionCancelRequestByUUIDParams) (db.ServiceSubscriptionCancelRequest, error)
	DecideServiceSubscriptionCancelRequest(ctx context.Context, arg db.DecideServiceSubscriptionCancelRequestParams) (db.ServiceSubscriptionCancelRequest, error)
	GetOrganizationByUUID(ctx context.Context, argUuid uuid.UUID) (db.Organization, error)
	GetOrganizationByID(ctx context.Context, id int64) (db.Organization, error)
	GetUserByID(ctx context.Context, id int64) (db.User, error)
	GetPrimaryOrganizationOwnerForServiceContract(ctx context.Context, organizationID int64) (db.User, error)
	SupplierOf(ctx context.Context, id int64) (db.Organization, error)
	Descendants(ctx context.Context, id int64) ([]db.Organization, error)
	ListOrganizationOwnerUserIDs(ctx context.Context, organizationID int64) ([]int64, error)
	ListUserIDsByRoleSlug(ctx context.Context, slug string) ([]int64, error)
}

type Service struct {
	q       Store
	queries *db.Queries
	pool    TxBeginner
	out     Outbox
	rates   RateResolver
	feature FeatureService
	// TEC-308: period / cancellation fee accounting and the job clock.
	poster Poster
	now    func() time.Time
}

func New(q Store) *Service {
	s := &Service{q: q}
	if queries, ok := q.(*db.Queries); ok {
		s.queries = queries
	}
	return s
}

func platformViewer() pricing.Viewer {
	return pricing.Viewer{PurchaseRead: true, SaleRead: true}
}

type ItemInput struct {
	Name               *string
	Description        *string
	Category           *string
	DefaultPrice       *string
	Currency           *string
	Recurrence         *string
	CancellationFee    *string
	ContractTemplateID *int64
	IsActive           *bool
}

type OverrideInput struct {
	Price    string
	Currency string
}

type Price struct {
	Amount   string `json:"amount"`
	Currency string `json:"currency"`
	Source   string `json:"source"`
}

type ItemView struct {
	UUID               uuid.UUID          `json:"uuid"`
	Name               string             `json:"name"`
	Description        string             `json:"description"`
	Category           string             `json:"category"`
	DefaultPrice       *string            `json:"default_price,omitempty"`
	Currency           string             `json:"currency"`
	Recurrence         string             `json:"recurrence"`
	CancellationFee    *string            `json:"cancellation_fee,omitempty"`
	ContractTemplateID *int64             `json:"contract_template_id,omitempty"`
	IsActive           bool               `json:"is_active"`
	Modules            []string           `json:"modules,omitempty"`
	EffectivePrice     *Price             `json:"effective_price,omitempty"`
	CreatedAt          pgtype.Timestamptz `json:"created_at"`
	UpdatedAt          pgtype.Timestamptz `json:"updated_at"`
}

type OverrideView struct {
	OrganizationUUID uuid.UUID `json:"organization_uuid"`
	Price            string    `json:"price"`
	Currency         string    `json:"currency"`
}

// Categories and Recurrences are the fixed enum values of the catalog
// (list filters, TEC-369).
var (
	Categories  = []string{"advertising", "training", "setup", "software", CategoryModuleBundle, "other"}
	Recurrences = []string{"one_time", "monthly", "yearly"}
)

// ListFilter narrows the service catalog list (TEC-369): Categories and
// Recurrences are any of; Q matches name and description. The list stays a
// full array (small brand catalog, client-side table).
type ListFilter struct {
	Q           string
	Categories  []string
	Recurrences []string
	Active      *bool
}

func (f ListFilter) params(brandID int64) db.ListServiceCatalogItemsParams {
	return db.ListServiceCatalogItemsParams{
		BrandID: brandID, Categories: f.Categories, Recurrences: f.Recurrences,
		IsActive: boolArg(f.Active), Q: textArg(f.Q),
	}
}

func (s *Service) ListPlatform(ctx context.Context, org orgctx.Scope, f ListFilter) ([]ItemView, error) {
	if org.OrgType != pricing.OrgCenter {
		return nil, ErrForbidden
	}
	items, err := s.q.ListServiceCatalogItems(ctx, f.params(org.BrandID))
	if err != nil {
		return nil, err
	}
	return s.views(ctx, items, nil, platformViewer()), nil
}

func (s *Service) Create(ctx context.Context, org orgctx.Scope, in ItemInput) (ItemView, error) {
	if org.OrgType != pricing.OrgCenter {
		return ItemView{}, ErrForbidden
	}
	p, err := buildCreate(org, in)
	if err != nil {
		return ItemView{}, err
	}
	if err := s.validateContractTemplate(ctx, org.BrandID, p.ContractTemplateID); err != nil {
		return ItemView{}, err
	}
	item, err := s.q.CreateServiceCatalogItem(ctx, p)
	if err != nil {
		return ItemView{}, mapDBError(err)
	}
	return s.view(ctx, item, nil, platformViewer()), nil
}

func (s *Service) GetPlatform(ctx context.Context, org orgctx.Scope, id uuid.UUID) (ItemView, error) {
	if org.OrgType != pricing.OrgCenter {
		return ItemView{}, ErrForbidden
	}
	item, err := s.item(ctx, org.BrandID, id)
	if err != nil {
		return ItemView{}, err
	}
	return s.view(ctx, item, nil, platformViewer()), nil
}

func (s *Service) Update(ctx context.Context, org orgctx.Scope, id uuid.UUID, in ItemInput) (ItemView, error) {
	if org.OrgType != pricing.OrgCenter {
		return ItemView{}, ErrForbidden
	}
	cur, err := s.item(ctx, org.BrandID, id)
	if err != nil {
		return ItemView{}, err
	}
	p, err := buildUpdate(cur, in)
	if err != nil {
		return ItemView{}, err
	}
	if err := s.validateContractTemplate(ctx, org.BrandID, p.ContractTemplateID); err != nil {
		return ItemView{}, err
	}
	item, err := s.q.UpdateServiceCatalogItem(ctx, p)
	if err != nil {
		return ItemView{}, mapDBError(err)
	}
	return s.view(ctx, item, nil, platformViewer()), nil
}

func (s *Service) Delete(ctx context.Context, org orgctx.Scope, id uuid.UUID) (ItemView, error) {
	if org.OrgType != pricing.OrgCenter {
		return ItemView{}, ErrForbidden
	}
	cur, err := s.item(ctx, org.BrandID, id)
	if err != nil {
		return ItemView{}, err
	}
	n, err := s.q.CountServiceSubscriptionsByItem(ctx, db.CountServiceSubscriptionsByItemParams{ItemID: cur.ID, BrandID: org.BrandID})
	if err != nil {
		return ItemView{}, err
	}
	if n > 0 {
		return ItemView{}, ErrInUse
	}
	off := false
	return s.Update(ctx, org, id, ItemInput{IsActive: &off})
}

func (s *Service) SetModules(ctx context.Context, org orgctx.Scope, id uuid.UUID, modules []string) (ItemView, error) {
	if org.OrgType != pricing.OrgCenter {
		return ItemView{}, ErrForbidden
	}
	item, err := s.item(ctx, org.BrandID, id)
	if err != nil {
		return ItemView{}, err
	}
	if item.Category != CategoryModuleBundle {
		return ItemView{}, ErrModuleOnly
	}
	seen := map[string]bool{}
	keys := make([]string, 0, len(modules))
	for i, raw := range modules {
		key := strings.TrimSpace(raw)
		if key == "" {
			return ItemView{}, invalid("modules", "module key cannot be empty")
		}
		if !seen[key] {
			seen[key] = true
			keys = append(keys, key)
		} else if i < 0 {
			return ItemView{}, nil
		}
	}
	if _, err := s.q.DeleteServiceCatalogModules(ctx, item.ID); err != nil {
		return ItemView{}, err
	}
	for _, key := range keys {
		if err := s.q.AddServiceCatalogModule(ctx, db.AddServiceCatalogModuleParams{ItemID: item.ID, ModuleKey: key}); err != nil {
			return ItemView{}, mapDBError(err)
		}
	}
	return s.view(ctx, item, keys, platformViewer()), nil
}

func (s *Service) UpsertOverride(ctx context.Context, org orgctx.Scope, itemID, distID uuid.UUID, in OverrideInput) (OverrideView, error) {
	if org.OrgType != pricing.OrgCenter {
		return OverrideView{}, ErrForbidden
	}
	item, err := s.item(ctx, org.BrandID, itemID)
	if err != nil {
		return OverrideView{}, err
	}
	dist, err := s.distributor(ctx, org.BrandID, distID)
	if err != nil {
		return OverrideView{}, err
	}
	price, err := parsePrice(in.Price)
	if err != nil {
		return OverrideView{}, err
	}
	cur, err := normalizeCurrency(in.Currency)
	if err != nil {
		return OverrideView{}, err
	}
	ov, err := s.q.UpsertServicePriceOverride(ctx, db.UpsertServicePriceOverrideParams{
		ItemID: item.ID, OrganizationID: dist.ID, BrandID: org.BrandID, Price: price, Currency: cur,
	})
	if err != nil {
		return OverrideView{}, mapDBError(err)
	}
	return overrideView(dist.Uuid, ov), nil
}

func (s *Service) GetOverride(ctx context.Context, org orgctx.Scope, itemID, distID uuid.UUID) (OverrideView, error) {
	if org.OrgType != pricing.OrgCenter {
		return OverrideView{}, ErrForbidden
	}
	item, err := s.item(ctx, org.BrandID, itemID)
	if err != nil {
		return OverrideView{}, err
	}
	dist, err := s.distributor(ctx, org.BrandID, distID)
	if err != nil {
		return OverrideView{}, err
	}
	ov, err := s.q.GetServicePriceOverride(ctx, db.GetServicePriceOverrideParams{ItemID: item.ID, OrganizationID: dist.ID})
	if errors.Is(err, pgx.ErrNoRows) {
		return OverrideView{}, ErrNotFound
	}
	if err != nil {
		return OverrideView{}, err
	}
	return overrideView(dist.Uuid, ov), nil
}

func (s *Service) DeleteOverride(ctx context.Context, org orgctx.Scope, itemID, distID uuid.UUID) error {
	if org.OrgType != pricing.OrgCenter {
		return ErrForbidden
	}
	item, err := s.item(ctx, org.BrandID, itemID)
	if err != nil {
		return err
	}
	dist, err := s.distributor(ctx, org.BrandID, distID)
	if err != nil {
		return err
	}
	n, err := s.q.DeleteServicePriceOverride(ctx, db.DeleteServicePriceOverrideParams{ItemID: item.ID, OrganizationID: dist.ID, BrandID: org.BrandID})
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// ListVisible lists the active catalog items a tenant may buy; f.Active is
// ignored (always active).
func (s *Service) ListVisible(ctx context.Context, org orgctx.Scope, viewer pricing.Viewer, f ListFilter) ([]ItemView, error) {
	if org.OrgType != pricing.OrgDistributor && org.OrgType != pricing.OrgDealer && org.OrgType != pricing.OrgCenter {
		return nil, ErrForbidden
	}
	t := true
	f.Active = &t
	items, err := s.q.ListServiceCatalogItems(ctx, f.params(org.BrandID))
	if err != nil {
		return nil, err
	}
	var buyer *db.Organization
	if org.OrgType != pricing.OrgCenter {
		b := db.Organization{ID: org.InternalID, Type: org.OrgType, BrandID: org.BrandID}
		if org.OrgType == pricing.OrgDealer {
			supplier, err := s.q.SupplierOf(ctx, org.InternalID)
			if err != nil {
				return nil, err
			}
			b.ParentID = pgtype.Int8{Int64: supplier.ID, Valid: true}
		}
		buyer = &b
	}
	return s.views(ctx, items, buyer, viewer), nil
}

// ResolvePrice resolves the price charged to a distributor or dealer. A
// dealer inherits its parent distributor's override; no distributor margin is
// added.
func (s *Service) ResolvePrice(ctx context.Context, item db.ServiceCatalogItem, buyerOrg db.Organization) (Price, error) {
	if buyerOrg.BrandID != item.BrandID {
		return Price{}, ErrInvalidBuyerOrg
	}
	overrideOrgID := int64(0)
	switch buyerOrg.Type {
	case pricing.OrgDistributor:
		overrideOrgID = buyerOrg.ID
	case pricing.OrgDealer:
		if buyerOrg.ParentID.Valid {
			overrideOrgID = buyerOrg.ParentID.Int64
		} else {
			parent, err := s.q.SupplierOf(ctx, buyerOrg.ID)
			if err != nil {
				return Price{}, ErrInvalidBuyerOrg
			}
			overrideOrgID = parent.ID
		}
	default:
		return Price{}, ErrInvalidBuyerOrg
	}
	if overrideOrgID != 0 {
		ov, err := s.q.GetServicePriceOverride(ctx, db.GetServicePriceOverrideParams{ItemID: item.ID, OrganizationID: overrideOrgID})
		if err == nil && ov.BrandID == item.BrandID {
			return Price{Amount: numText(ov.Price), Currency: ov.Currency, Source: "override"}, nil
		}
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return Price{}, err
		}
	}
	return Price{Amount: numText(item.DefaultPrice), Currency: item.Currency, Source: "default"}, nil
}

func (s *Service) item(ctx context.Context, brandID int64, id uuid.UUID) (db.ServiceCatalogItem, error) {
	item, err := s.q.GetServiceCatalogItemByUUID(ctx, db.GetServiceCatalogItemByUUIDParams{Uuid: id, BrandID: brandID})
	if errors.Is(err, pgx.ErrNoRows) {
		return db.ServiceCatalogItem{}, ErrNotFound
	}
	return item, err
}

func (s *Service) distributor(ctx context.Context, brandID int64, id uuid.UUID) (db.Organization, error) {
	org, err := s.q.GetOrganizationByUUID(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) || org.BrandID != brandID || org.Type != pricing.OrgDistributor {
		return db.Organization{}, ErrNotFound
	}
	return org, err
}

func (s *Service) validateContractTemplate(ctx context.Context, brandID int64, id pgtype.Int8) error {
	if !id.Valid {
		return nil
	}
	tpl, err := s.q.GetContractTemplateByID(ctx, id.Int64)
	if errors.Is(err, pgx.ErrNoRows) || tpl.BrandID != brandID {
		return invalid("contract_template_id", "is invalid")
	}
	if err != nil {
		return err
	}
	if tpl.Kind != "service_sale" {
		return invalid("contract_template_id", "must reference a service_sale template")
	}
	return nil
}

func (s *Service) views(ctx context.Context, items []db.ServiceCatalogItem, buyer *db.Organization, viewer pricing.Viewer) []ItemView {
	out := make([]ItemView, 0, len(items))
	for _, item := range items {
		var price *Price
		if buyer != nil && viewer.PurchaseRead {
			if p, err := s.ResolvePrice(ctx, item, *buyer); err == nil {
				price = &p
			}
		} else if buyer == nil && viewer.SaleRead {
			p := Price{Amount: numText(item.DefaultPrice), Currency: item.Currency, Source: "default"}
			price = &p
		}
		out = append(out, s.view(ctx, item, nil, viewer).withPrice(price))
	}
	return out
}

func (s *Service) view(ctx context.Context, item db.ServiceCatalogItem, modules []string, viewer pricing.Viewer) ItemView {
	if modules == nil {
		modules, _ = s.q.ListServiceCatalogModules(ctx, item.ID)
	}
	var tpl *int64
	if item.ContractTemplateID.Valid {
		tpl = &item.ContractTemplateID.Int64
	}
	var def, fee *string
	if viewer.SaleRead || viewer.PurchaseRead {
		def = strPtr(numText(item.DefaultPrice))
		fee = strPtr(numText(item.CancellationFee))
	}
	return ItemView{
		UUID: item.Uuid, Name: item.Name, Description: item.Description, Category: item.Category,
		DefaultPrice: def, Currency: item.Currency, Recurrence: item.Recurrence,
		CancellationFee: fee, ContractTemplateID: tpl, IsActive: item.IsActive, Modules: modules,
		CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt,
	}
}

func (v ItemView) withPrice(p *Price) ItemView {
	v.EffectivePrice = p
	return v
}

func buildCreate(org orgctx.Scope, in ItemInput) (db.CreateServiceCatalogItemParams, error) {
	name, err := requiredText("name", in.Name, 200)
	if err != nil {
		return db.CreateServiceCatalogItemParams{}, err
	}
	cat, err := category(in.Category)
	if err != nil {
		return db.CreateServiceCatalogItemParams{}, err
	}
	price, err := requiredPrice("default_price", in.DefaultPrice)
	if err != nil {
		return db.CreateServiceCatalogItemParams{}, err
	}
	cur, err := requiredCurrency(in.Currency)
	if err != nil {
		return db.CreateServiceCatalogItemParams{}, err
	}
	rec, err := recurrence(in.Recurrence)
	if err != nil {
		return db.CreateServiceCatalogItemParams{}, err
	}
	fee, err := optionalPriceArg("cancellation_fee", in.CancellationFee, "0")
	if err != nil {
		return db.CreateServiceCatalogItemParams{}, err
	}
	active := true
	if in.IsActive != nil {
		active = *in.IsActive
	}
	return db.CreateServiceCatalogItemParams{
		OrganizationID: org.InternalID, BrandID: org.BrandID, Name: name, Description: optionalText(in.Description, 10000),
		Category: cat, DefaultPrice: price, Currency: cur, Recurrence: rec, CancellationFee: fee,
		ContractTemplateID: int8Arg(in.ContractTemplateID), IsActive: active,
	}, nil
}

func buildUpdate(cur db.ServiceCatalogItem, in ItemInput) (db.UpdateServiceCatalogItemParams, error) {
	name := cur.Name
	if in.Name != nil {
		v, err := requiredText("name", in.Name, 200)
		if err != nil {
			return db.UpdateServiceCatalogItemParams{}, err
		}
		name = v
	}
	desc := cur.Description
	if in.Description != nil {
		desc = optionalText(in.Description, 10000)
	}
	cat := cur.Category
	if in.Category != nil {
		v, err := category(in.Category)
		if err != nil {
			return db.UpdateServiceCatalogItemParams{}, err
		}
		cat = v
	}
	price := cur.DefaultPrice
	if in.DefaultPrice != nil {
		v, err := requiredPrice("default_price", in.DefaultPrice)
		if err != nil {
			return db.UpdateServiceCatalogItemParams{}, err
		}
		price = v
	}
	curCode := cur.Currency
	if in.Currency != nil {
		v, err := requiredCurrency(in.Currency)
		if err != nil {
			return db.UpdateServiceCatalogItemParams{}, err
		}
		curCode = v
	}
	rec := cur.Recurrence
	if in.Recurrence != nil {
		v, err := recurrence(in.Recurrence)
		if err != nil {
			return db.UpdateServiceCatalogItemParams{}, err
		}
		rec = v
	}
	fee := cur.CancellationFee
	if in.CancellationFee != nil {
		v, err := optionalPriceArg("cancellation_fee", in.CancellationFee, "0")
		if err != nil {
			return db.UpdateServiceCatalogItemParams{}, err
		}
		fee = v
	}
	active := cur.IsActive
	if in.IsActive != nil {
		active = *in.IsActive
	}
	tpl := cur.ContractTemplateID
	if in.ContractTemplateID != nil {
		tpl = int8Arg(in.ContractTemplateID)
	}
	return db.UpdateServiceCatalogItemParams{
		Name: name, Description: desc, Category: cat, DefaultPrice: price, Currency: curCode, Recurrence: rec,
		CancellationFee: fee, ContractTemplateID: tpl, IsActive: active, ID: cur.ID, BrandID: cur.BrandID,
	}, nil
}

func requiredText(field string, v *string, max int) (string, error) {
	if v == nil {
		return "", invalid(field, "is required")
	}
	s := strings.TrimSpace(*v)
	if s == "" || len(s) > max {
		return "", invalid(field, "is invalid")
	}
	return s, nil
}

func optionalText(v *string, max int) string {
	if v == nil {
		return ""
	}
	s := strings.TrimSpace(*v)
	if len(s) > max {
		return s[:max]
	}
	return s
}

func category(v *string) (string, error) {
	if v == nil {
		return "", invalid("category", "is required")
	}
	c := strings.TrimSpace(*v)
	switch c {
	case "advertising", "training", "setup", "software", CategoryModuleBundle, "other":
		return c, nil
	default:
		return "", invalid("category", "is invalid")
	}
}

func recurrence(v *string) (string, error) {
	if v == nil {
		return "", invalid("recurrence", "is required")
	}
	r := strings.TrimSpace(*v)
	switch r {
	case "one_time", "monthly", "yearly":
		return r, nil
	default:
		return "", invalid("recurrence", "is invalid")
	}
}

func requiredCurrency(v *string) (string, error) {
	if v == nil {
		return "", invalid("currency", "is required")
	}
	return normalizeCurrency(*v)
}

func normalizeCurrency(raw string) (string, error) {
	c := strings.ToUpper(strings.TrimSpace(raw))
	if !currencyRe.MatchString(c) {
		return "", invalid("currency", "must be a three-letter ISO-4217 code")
	}
	return c, nil
}

func requiredPrice(field string, v *string) (pgtype.Numeric, error) {
	if v == nil {
		return pgtype.Numeric{}, invalid(field, "is required")
	}
	return parsePrice(*v)
}

func optionalPriceArg(field string, v *string, def string) (pgtype.Numeric, error) {
	raw := def
	if v != nil {
		raw = *v
	}
	return parsePriceField(field, raw)
}

func parsePrice(raw string) (pgtype.Numeric, error) {
	return parsePriceField("price", raw)
}

func parsePriceField(field, raw string) (pgtype.Numeric, error) {
	p := strings.TrimSpace(raw)
	if !priceRe.MatchString(p) {
		return pgtype.Numeric{}, invalid(field, "must be a non-negative decimal with at most 2 fractional digits")
	}
	var n pgtype.Numeric
	if err := n.Scan(p); err != nil {
		return pgtype.Numeric{}, invalid(field, "is invalid")
	}
	return n, nil
}

func numText(n pgtype.Numeric) string {
	v, err := n.Value()
	if err != nil || v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func textArg(v string) pgtype.Text {
	v = strings.TrimSpace(v)
	if v == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: v, Valid: true}
}

func boolArg(v *bool) pgtype.Bool {
	if v == nil {
		return pgtype.Bool{}
	}
	return pgtype.Bool{Bool: *v, Valid: true}
}

func int8Arg(v *int64) pgtype.Int8 {
	if v == nil {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: *v, Valid: true}
}

func strPtr(s string) *string { return &s }

func pgText(s string) pgtype.Text {
	s = strings.TrimSpace(s)
	if s == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: s, Valid: true}
}

func sha256Hex(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

func overrideView(org uuid.UUID, ov db.ServicePriceOverride) OverrideView {
	return OverrideView{OrganizationUUID: org, Price: numText(ov.Price), Currency: ov.Currency}
}

func mapDBError(err error) error {
	var pgerr *pgconn.PgError
	if errors.As(err, &pgerr) {
		switch pgerr.Code {
		case "23503", "23514", "23001":
			return invalid("request", "violates service catalog constraints")
		}
	}
	return err
}
