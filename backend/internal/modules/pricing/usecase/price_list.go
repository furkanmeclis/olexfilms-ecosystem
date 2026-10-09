package usecase

// TEC-506 (F5-09b): the recommended retail price list PDF. After a
// publication (and when scheduled prices take effect) worker-docs renders
// the price_list document of each country / currency in the languages of
// the organizations it reaches (Gotenberg) and adds it as a new version of
// the same document center (library) item: system actor = the brand center,
// folder "Fiyat listeleri", access all_network (distributors + dealers),
// country / currency tags. The library belongs to the announcements module:
// when it is off for the center the publication is skipped with a warning.

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	docmodel "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/documents/model"
	library "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/library/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/features"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/pdfrender"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// PriceListFolder is the document center folder of the price lists.
const PriceListFolder = "Fiyat listeleri"

// PriceListTag is the tag every price list item carries.
const PriceListTag = "price-list"

// PriceListRenderer renders a registered document kind as the system
// (documents.Service.RenderSource).
type PriceListRenderer interface {
	RenderSource(ctx context.Context, kind, sourceID, locale string) ([]byte, error)
}

// PriceListLibrary is the part of the library service the publisher uses.
type PriceListLibrary interface {
	ListFolders(ctx context.Context, actor library.Actor) ([]library.FolderView, error)
	CreateFolder(ctx context.Context, actor library.Actor, in library.FolderInput) (library.FolderView, error)
	ListItems(ctx context.Context, actor library.Actor, in library.ListInput) ([]library.ItemView, int64, error)
	CreateItem(ctx context.Context, actor library.Actor, in library.ItemInput) (library.ItemView, error)
	AddVersion(ctx context.Context, actor library.Actor, itemID uuid.UUID, in library.UploadInput) (library.VersionView, error)
}

// FeatureChecker reports whether a module is on for an organization.
type FeatureChecker interface {
	Enabled(ctx context.Context, organizationID int64, key string) (bool, error)
}

// PriceListPublisher publishes price list PDFs to the document center.
type PriceListPublisher struct {
	q        *db.Queries
	renderer PriceListRenderer
	library  PriceListLibrary
	features FeatureChecker
	now      func() time.Time
	log      *slog.Logger
}

// NewPriceListPublisher creates the publisher.
func NewPriceListPublisher(q *db.Queries, renderer PriceListRenderer, lib PriceListLibrary, checker FeatureChecker, log *slog.Logger) *PriceListPublisher {
	if log == nil {
		log = slog.Default()
	}
	return &PriceListPublisher{q: q, renderer: renderer, library: lib, features: checker, now: time.Now, log: log}
}

// SetClock replaces the clock (tests).
func (p *PriceListPublisher) SetClock(now func() time.Time) { p.now = now }

// PriceListResult reports one publication.
type PriceListResult struct {
	Skipped   bool
	ItemUUID  uuid.UUID
	Languages []string
}

// Publish renders and stores the price list of one country / currency.
func (p *PriceListPublisher) Publish(ctx context.Context, task PriceListTask) (PriceListResult, error) {
	center, err := p.q.GetBrandCenter(ctx, task.BrandID)
	if errors.Is(err, pgx.ErrNoRows) {
		return PriceListResult{Skipped: true}, nil
	}
	if err != nil {
		return PriceListResult{}, fmt.Errorf("price list: center: %w", err)
	}
	if p.features != nil {
		on, err := p.features.Enabled(ctx, center.ID, features.ModuleAnnouncements)
		if err != nil {
			return PriceListResult{}, fmt.Errorf("price list: feature: %w", err)
		}
		if !on {
			p.log.Warn("pricing_price_list_skipped_library_off", "brand_id", task.BrandID,
				"country_id", task.CountryID, "currency", task.Currency)
			return PriceListResult{Skipped: true}, nil
		}
	}
	country := pgtype.Int8{Int64: task.CountryID, Valid: task.CountryID != 0}
	iso := ""
	if country.Valid {
		c, err := p.q.GetCountryByID(ctx, task.CountryID)
		if err != nil {
			return PriceListResult{}, fmt.Errorf("price list: country: %w", err)
		}
		iso = strings.TrimSpace(c.Iso2)
	}
	langs, err := p.languages(ctx, center, country, task.Currency)
	if err != nil {
		return PriceListResult{}, err
	}
	actor := library.Actor{OrganizationID: center.ID, BrandID: center.BrandID, OrgType: OrgCenter}
	item, err := p.item(ctx, actor, iso, task.Currency)
	if err != nil {
		return PriceListResult{}, err
	}
	today := LocalDay(p.now(), center.Timezone)
	source := priceListSourceID(task.BrandID, task.CountryID, task.Currency, today)
	label := iso
	if label == "" {
		label = "ALL"
	}
	for _, lang := range langs {
		pdf, err := p.renderer.RenderSource(ctx, docmodel.KindPriceList, source, lang)
		if err != nil {
			return PriceListResult{}, fmt.Errorf("price list: render %s: %w", lang, err)
		}
		name := fmt.Sprintf("price-list-%s-%s-%s-%s.pdf", label, task.Currency, today.Format(time.DateOnly), lang)
		if _, err := p.library.AddVersion(ctx, actor, item.UUID, library.UploadInput{
			Locale: lang, Filename: name, Size: int64(len(pdf)), Body: bytes.NewReader(pdf),
		}); err != nil {
			return PriceListResult{}, fmt.Errorf("price list: library version %s: %w", lang, err)
		}
	}
	p.log.Info("pricing_price_list_published", "brand_id", task.BrandID, "country", label,
		"currency", task.Currency, "languages", strings.Join(langs, ","))
	return PriceListResult{ItemUUID: item.UUID, Languages: langs}, nil
}

// Task is the queue entry point (pricing:price_list).
func (p *PriceListPublisher) Task(ctx context.Context, task PriceListTask) error {
	_, err := p.Publish(ctx, task)
	return err
}

// languages are the document languages of the organizations the list
// reaches plus the center's, in model.Languages order.
func (p *PriceListPublisher) languages(ctx context.Context, center db.Organization, country pgtype.Int8, currency string) ([]string, error) {
	locales, err := p.q.ListPriceListLocales(ctx, db.ListPriceListLocalesParams{BrandID: center.BrandID, CountryID: country, Currency: currency})
	if err != nil {
		return nil, fmt.Errorf("price list: locales: %w", err)
	}
	set := map[string]bool{}
	for _, l := range append(locales, center.Locale) {
		if n := docmodel.NormalizeLanguage(l); n != "" {
			set[n] = true
		}
	}
	if len(set) == 0 {
		set[docmodel.FallbackLanguage] = true
	}
	out := make([]string, 0, len(set))
	for _, l := range docmodel.Languages {
		if set[l] {
			out = append(out, l)
		}
	}
	return out, nil
}

func priceListItemTag(iso, currency string) string {
	if iso == "" {
		iso = "all"
	}
	return fmt.Sprintf("%s:%s:%s", PriceListTag, strings.ToLower(iso), strings.ToLower(currency))
}

// item finds (or creates) the library item of a country / currency in the
// price list folder.
func (p *PriceListPublisher) item(ctx context.Context, actor library.Actor, iso, currency string) (library.ItemView, error) {
	folders, err := p.library.ListFolders(ctx, actor)
	if err != nil {
		return library.ItemView{}, fmt.Errorf("price list: folders: %w", err)
	}
	var folder *library.FolderView
	for i := range folders {
		if folders[i].ParentUUID == nil && strings.EqualFold(folders[i].Name, PriceListFolder) {
			folder = &folders[i]
			break
		}
	}
	if folder == nil {
		name := PriceListFolder
		f, err := p.library.CreateFolder(ctx, actor, library.FolderInput{Name: &name})
		if err != nil {
			return library.ItemView{}, fmt.Errorf("price list: folder: %w", err)
		}
		folder = &f
	}
	tag := priceListItemTag(iso, currency)
	items, _, err := p.library.ListItems(ctx, actor, library.ListInput{FolderUUID: &folder.UUID, Tags: []string{tag}, Limit: 1})
	if err != nil {
		return library.ItemView{}, fmt.Errorf("price list: items: %w", err)
	}
	if len(items) > 0 {
		return items[0], nil
	}
	label := iso
	countryTag := "country:" + strings.ToLower(iso)
	if iso == "" {
		label = "Tüm ülkeler"
		countryTag = "country:all"
	}
	name := fmt.Sprintf("Tavsiye satış fiyat listesi · %s · %s", label, currency)
	desc := "Tavsiye fiyat yayınından sonra otomatik güncellenir."
	access := library.AccessAllNetwork
	created, err := p.library.CreateItem(ctx, actor, library.ItemInput{
		FolderUUID: &folder.UUID, Name: &name, Description: &desc, AccessLevel: &access,
		Tags: []string{PriceListTag, tag, countryTag, "currency:" + strings.ToLower(currency)},
	})
	if err != nil {
		return library.ItemView{}, fmt.Errorf("price list: item: %w", err)
	}
	return created, nil
}

func priceListSourceID(brandID, countryID int64, currency string, day time.Time) string {
	return fmt.Sprintf("%d:%d:%s:%s", brandID, countryID, currency, day.Format(time.DateOnly))
}

// --- Document source -------------------------------------------------------------

// PriceListLoader is the price_list document source: the source id is
// "brand:country(0 = currency-wide):currency:YYYY-MM-DD". Only the system
// (worker) loads it.
type PriceListLoader struct{ q *db.Queries }

// DocumentLoader returns the price_list source loader.
func (p *PriceListPublisher) DocumentLoader() PriceListLoader { return PriceListLoader{q: p.q} }

// NewPriceListLoader creates the loader.
func NewPriceListLoader(q *db.Queries) PriceListLoader { return PriceListLoader{q: q} }

// SourceType implements docmodel.SourceLoader.
func (PriceListLoader) SourceType() string { return "price_list" }

// Load implements docmodel.SourceLoader.
func (l PriceListLoader) Load(ctx context.Context, viewer docmodel.Viewer, sourceID, locale string) (docmodel.Source, error) {
	if !viewer.System {
		return docmodel.Source{}, docmodel.ErrSourceNotFound
	}
	parts := strings.Split(sourceID, ":")
	if len(parts) != 4 {
		return docmodel.Source{}, docmodel.ErrSourceNotFound
	}
	brandID, err1 := strconv.ParseInt(parts[0], 10, 64)
	countryID, err2 := strconv.ParseInt(parts[1], 10, 64)
	day, err3 := time.Parse(time.DateOnly, parts[3])
	currency := parts[2]
	if err1 != nil || err2 != nil || err3 != nil || len(currency) != 3 {
		return docmodel.Source{}, docmodel.ErrSourceNotFound
	}
	center, err := l.q.GetBrandCenter(ctx, brandID)
	if err != nil {
		return docmodel.Source{}, docmodel.ErrSourceNotFound
	}
	lang := docmodel.NormalizeLanguage(locale)
	t := priceListLabelsFor(lang)
	countryName := t.AllCountries
	country := pgtype.Int8{}
	if countryID != 0 {
		c, err := l.q.GetCountryByID(ctx, countryID)
		if err != nil {
			return docmodel.Source{}, docmodel.ErrSourceNotFound
		}
		country = pgtype.Int8{Int64: c.ID, Valid: true}
		countryName = c.NameEn
		if lang == "tr" {
			countryName = c.NameTr
		}
	}
	rows, err := l.q.ListPriceListRows(ctx, db.ListPriceListRowsParams{
		BrandID: brandID, CountryID: country, Currency: currency, AsOf: pgDate(day),
	})
	if err != nil {
		return docmodel.Source{}, fmt.Errorf("price list: rows: %w", err)
	}
	table := make([][]string, 0, len(rows))
	for _, r := range rows {
		next, from := "", ""
		if r.NextEffectiveFrom.Valid {
			next = money(r.NextPrice)
			from = r.NextEffectiveFrom.Time.Format(t.DateLayout)
		}
		table = append(table, []string{r.Sku, r.Name, money(r.Price), next, from})
	}
	vars := map[string]string{
		"country_name":   countryName,
		"currency":       currency,
		"effective_date": day.Format(t.DateLayout),
		"document_date":  day.Format(t.DateLayout),
		"prices_table": pdfrender.Table([]pdfrender.Column{
			{Label: t.SKU}, {Label: t.Product}, {Label: t.Price, Numeric: true},
			{Label: t.NextPrice, Numeric: true}, {Label: t.NextFrom},
		}, table),
	}
	return docmodel.Source{
		OrganizationID: center.ID, BrandID: brandID,
		Version: sourceID + ":" + strconv.Itoa(len(rows)), Vars: vars,
		Title: fmt.Sprintf("%s %s %s", t.Title, countryName, currency),
	}, nil
}

// priceListLabels are the table headings of the price list in one language.
type priceListLabels struct {
	Title, SKU, Product, Price, NextPrice, NextFrom, AllCountries, DateLayout string
}

var priceListLabelSet = map[string]priceListLabels{
	"tr":    {"Tavsiye Satış Fiyat Listesi", "Stok kodu", "Ürün", "Fiyat", "Sonraki fiyat", "Geçerlilik", "Tüm ülkeler", "02.01.2006"},
	"en":    {"Recommended Retail Price List", "SKU", "Product", "Price", "Next price", "Effective from", "All countries", "2006-01-02"},
	"bg":    {"Ценова листа с препоръчителни цени", "Код", "Продукт", "Цена", "Следваща цена", "В сила от", "Всички държави", "02.01.2006"},
	"de":    {"Preisliste der unverbindlichen Preisempfehlungen", "Artikelnr.", "Produkt", "Preis", "Nächster Preis", "Gültig ab", "Alle Länder", "02.01.2006"},
	"el":    {"Τιμοκατάλογος προτεινόμενων τιμών λιανικής", "Κωδικός", "Προϊόν", "Τιμή", "Επόμενη τιμή", "Ισχύει από", "Όλες οι χώρες", "02/01/2006"},
	"uk":    {"Прайс-лист рекомендованих роздрібних цін", "Артикул", "Товар", "Ціна", "Наступна ціна", "Діє з", "Усі країни", "02.01.2006"},
	"ru":    {"Прайс-лист рекомендованных розничных цен", "Артикул", "Товар", "Цена", "Следующая цена", "Действует с", "Все страны", "02.01.2006"},
	"fr":    {"Liste des prix de vente conseillés", "Référence", "Produit", "Prix", "Prix suivant", "À partir du", "Tous les pays", "02/01/2006"},
	"es":    {"Lista de precios de venta recomendados", "Referencia", "Producto", "Precio", "Próximo precio", "Desde", "Todos los países", "02/01/2006"},
	"it":    {"Listino prezzi di vendita consigliati", "Codice", "Prodotto", "Prezzo", "Prezzo successivo", "Dal", "Tutti i paesi", "02/01/2006"},
	"zh_CN": {"建议零售价目表", "货号", "产品", "价格", "下一价格", "生效日期", "所有国家", "2006-01-02"},
	"az":    {"Tövsiyə olunan satış qiymətləri siyahısı", "Məhsul kodu", "Məhsul", "Qiymət", "Növbəti qiymət", "Qüvvəyə minir", "Bütün ölkələr", "02.01.2006"},
	"ar":    {"قائمة أسعار البيع الموصى بها", "رمز المنتج", "المنتج", "السعر", "السعر التالي", "اعتبارًا من", "جميع الدول", "2006-01-02"},
}

func priceListLabelsFor(lang string) priceListLabels {
	if t, ok := priceListLabelSet[lang]; ok {
		return t
	}
	return priceListLabelSet[docmodel.FallbackLanguage]
}
