package catalog

import "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/msgtemplate"

const EventStockForecastLow = "STOCK_FORECAST_LOW"

var StockForecastChannels = []string{ChannelInapp, ChannelEmail}

var stockForecastLowTexts = map[string]localizedText{
	"tr":    {"Stok azalıyor: {{product_name}}", "{{organization_name}} için {{product_name}} stoğu {{days_left}} gün kaldı ({{status}})."},
	"en":    {"Stock running low: {{product_name}}", "{{product_name}} stock at {{organization_name}} has {{days_left}} days left ({{status}})."},
	"bg":    {"Ниска наличност: {{product_name}}", "Наличността на {{product_name}} в {{organization_name}} стига за {{days_left}} дни ({{status}})."},
	"de":    {"Bestand wird knapp: {{product_name}}", "{{product_name}} bei {{organization_name}} reicht noch {{days_left}} Tage ({{status}})."},
	"el":    {"Χαμηλό απόθεμα: {{product_name}}", "Το απόθεμα {{product_name}} στον οργανισμό {{organization_name}} επαρκεί για {{days_left}} ημέρες ({{status}})."},
	"uk":    {"Запас закінчується: {{product_name}}", "Запас {{product_name}} в {{organization_name}} залишився на {{days_left}} днів ({{status}})."},
	"ru":    {"Запас заканчивается: {{product_name}}", "Запаса {{product_name}} в {{organization_name}} осталось на {{days_left}} дней ({{status}})."},
	"fr":    {"Stock faible : {{product_name}}", "Le stock de {{product_name}} chez {{organization_name}} couvre encore {{days_left}} jours ({{status}})."},
	"es":    {"Stock bajo: {{product_name}}", "El stock de {{product_name}} en {{organization_name}} queda para {{days_left}} días ({{status}})."},
	"it":    {"Scorte basse: {{product_name}}", "La scorta di {{product_name}} presso {{organization_name}} resta per {{days_left}} giorni ({{status}})."},
	"zh-CN": {"库存偏低：{{product_name}}", "{{organization_name}} 的 {{product_name}} 库存还可用 {{days_left}} 天（{{status}}）。"},
	"az":    {"Stok azalır: {{product_name}}", "{{organization_name}} üzrə {{product_name}} stoku {{days_left}} gün qalıb ({{status}})."},
	"ar":    {"انخفاض المخزون: {{product_name}}", "تبقى مخزون {{product_name}} لدى {{organization_name}} لمدة {{days_left}} يومًا ({{status}})."},
}

func stockForecastLowTemplates() []DefaultTemplate {
	out := make([]DefaultTemplate, 0, len(stockForecastLowTexts)*len(StockForecastChannels))
	for _, lang := range msgtemplate.Locales {
		t, ok := stockForecastLowTexts[lang]
		if !ok {
			continue
		}
		for _, ch := range StockForecastChannels {
			out = append(out, DefaultTemplate{Role: RoleGeneric, Channel: ch, Language: lang, Subject: t.subject, Body: t.body, Format: "text"})
		}
	}
	return out
}

func stockForecastPlaceholders() []msgtemplate.Placeholder {
	return []msgtemplate.Placeholder{
		ph("product_name", "PPF Şeffaf Film", "Clear PPF"),
		ph("organization_name", "Tech Oto", "Tech Oto"),
		ph("days_left", "6", "6"),
		ph("status", "critical", "critical"),
	}
}

func init() {
	Register(Event{
		Code: EventStockForecastLow, Module: "stock_forecast",
		DefaultChannels: StockForecastChannels, AudienceRoles: []string{RoleDealer, RoleDistributor, RoleCenter},
		Placeholders: stockForecastPlaceholders(), UserConfigurable: true,
		Templates: stockForecastLowTemplates(),
	})
}
