package catalog

import "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/msgtemplate"

// EventStockReorderLow (TEC-1229): the available quantity of a product in
// one warehouse fell to its reorder point or below its minimum.
const EventStockReorderLow = "STOCK_REORDER_LOW"

var stockReorderLowTexts = map[string]localizedText{
	"tr":    {"Kritik stok: {{product_name}}", "{{warehouse_name}} deposunda {{product_name}} kullanılabilir stoğu {{available_quantity}} adet (eşik {{threshold}})."},
	"en":    {"Critical stock: {{product_name}}", "Available stock of {{product_name}} in warehouse {{warehouse_name}} is {{available_quantity}} (threshold {{threshold}})."},
	"bg":    {"Критична наличност: {{product_name}}", "Наличността на {{product_name}} в склад {{warehouse_name}} е {{available_quantity}} бр. (праг {{threshold}})."},
	"de":    {"Kritischer Bestand: {{product_name}}", "Verfügbarer Bestand von {{product_name}} im Lager {{warehouse_name}}: {{available_quantity}} (Schwelle {{threshold}})."},
	"el":    {"Κρίσιμο απόθεμα: {{product_name}}", "Το διαθέσιμο απόθεμα του {{product_name}} στην αποθήκη {{warehouse_name}} είναι {{available_quantity}} (όριο {{threshold}})."},
	"uk":    {"Критичний запас: {{product_name}}", "Доступний запас {{product_name}} на складі {{warehouse_name}}: {{available_quantity}} (поріг {{threshold}})."},
	"ru":    {"Критический запас: {{product_name}}", "Доступный запас {{product_name}} на складе {{warehouse_name}}: {{available_quantity}} (порог {{threshold}})."},
	"fr":    {"Stock critique : {{product_name}}", "Le stock disponible de {{product_name}} dans l'entrepôt {{warehouse_name}} est de {{available_quantity}} (seuil {{threshold}})."},
	"es":    {"Stock crítico: {{product_name}}", "El stock disponible de {{product_name}} en el almacén {{warehouse_name}} es {{available_quantity}} (umbral {{threshold}})."},
	"it":    {"Scorta critica: {{product_name}}", "La scorta disponibile di {{product_name}} nel magazzino {{warehouse_name}} è {{available_quantity}} (soglia {{threshold}})."},
	"zh-CN": {"库存告急：{{product_name}}", "{{warehouse_name}} 仓库中 {{product_name}} 的可用库存为 {{available_quantity}}（阈值 {{threshold}}）。"},
	"az":    {"Kritik stok: {{product_name}}", "{{warehouse_name}} anbarında {{product_name}} üzrə mövcud stok {{available_quantity}} ədəddir (hədd {{threshold}})."},
	"ar":    {"مخزون حرج: {{product_name}}", "المخزون المتاح من {{product_name}} في المستودع {{warehouse_name}} هو {{available_quantity}} (الحد {{threshold}})."},
	"fa":    {"موجودی بحرانی: {{product_name}}", "موجودی در دسترس {{product_name}} در انبار {{warehouse_name}} برابر {{available_quantity}} است (آستانه {{threshold}})."},
}

func stockReorderLowTemplates() []DefaultTemplate {
	out := make([]DefaultTemplate, 0, len(stockReorderLowTexts)*len(StockForecastChannels))
	for _, lang := range msgtemplate.Locales {
		t, ok := stockReorderLowTexts[lang]
		if !ok {
			continue
		}
		for _, ch := range StockForecastChannels {
			out = append(out, DefaultTemplate{Role: RoleGeneric, Channel: ch, Language: lang, Subject: t.subject, Body: t.body, Format: "text"})
		}
	}
	return out
}

func init() {
	Register(Event{
		Code: EventStockReorderLow, Module: "stock",
		DefaultChannels: StockForecastChannels, AudienceRoles: []string{RoleDealer, RoleDistributor, RoleCenter},
		Placeholders: []msgtemplate.Placeholder{
			ph("product_name", "PPF Şeffaf Film", "Clear PPF"),
			ph("warehouse_name", "Ana depo", "Main warehouse"),
			ph("available_quantity", "3", "3"),
			ph("threshold", "5", "5"),
		},
		UserConfigurable: true,
		Templates:        stockReorderLowTemplates(),
	})
}
