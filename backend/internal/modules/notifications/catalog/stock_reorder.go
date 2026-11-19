package catalog

import "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/msgtemplate"

// EventStockReorderLow (TEC-1229): the available quantity of a product in
// one warehouse fell to its reorder point or below its minimum.
const EventStockReorderLow = "STOCK_REORDER_LOW"

// EventStockReorderMetersLow (TEC-1243, K14): the available meters (open
// rolls and cut pieces, remnants excluded) of a product in one warehouse
// fell below the rule's meter threshold. It runs apart from the closed box
// alert (STOCK_REORDER_LOW).
const EventStockReorderMetersLow = "STOCK_REORDER_METERS_LOW"

var stockReorderMetersLowTexts = map[string]localizedText{
	"tr":    {"Kritik stok (metre): {{product_name}}", "{{warehouse_name}} deposunda {{product_name}} açık rulo ve kesik parça metresi {{available_meters}} m (eşik {{threshold}} m)."},
	"en":    {"Critical stock (meters): {{product_name}}", "Open roll and cut piece meters of {{product_name}} in warehouse {{warehouse_name}}: {{available_meters}} m (threshold {{threshold}} m)."},
	"bg":    {"Критична наличност (метри): {{product_name}}", "Метри в отворени ролки и отрязани парчета от {{product_name}} в склад {{warehouse_name}}: {{available_meters}} м (праг {{threshold}} м)."},
	"de":    {"Kritischer Bestand (Meter): {{product_name}}", "Meter in offenen Rollen und Zuschnitten von {{product_name}} im Lager {{warehouse_name}}: {{available_meters}} m (Schwelle {{threshold}} m)."},
	"el":    {"Κρίσιμο απόθεμα (μέτρα): {{product_name}}", "Μέτρα σε ανοιχτά ρολά και κομμένα τεμάχια του {{product_name}} στην αποθήκη {{warehouse_name}}: {{available_meters}} μ (όριο {{threshold}} μ)."},
	"uk":    {"Критичний запас (метри): {{product_name}}", "Метри у відкритих рулонах і відрізах {{product_name}} на складі {{warehouse_name}}: {{available_meters}} м (поріг {{threshold}} м)."},
	"ru":    {"Критический запас (метры): {{product_name}}", "Метры в открытых рулонах и отрезах {{product_name}} на складе {{warehouse_name}}: {{available_meters}} м (порог {{threshold}} м)."},
	"fr":    {"Stock critique (mètres) : {{product_name}}", "Mètres en rouleaux ouverts et coupes de {{product_name}} dans l'entrepôt {{warehouse_name}} : {{available_meters}} m (seuil {{threshold}} m)."},
	"es":    {"Stock crítico (metros): {{product_name}}", "Metros en rollos abiertos y piezas cortadas de {{product_name}} en el almacén {{warehouse_name}}: {{available_meters}} m (umbral {{threshold}} m)."},
	"it":    {"Scorta critica (metri): {{product_name}}", "Metri in rotoli aperti e pezzi tagliati di {{product_name}} nel magazzino {{warehouse_name}}: {{available_meters}} m (soglia {{threshold}} m)."},
	"zh-CN": {"库存告急（米）：{{product_name}}", "{{warehouse_name}} 仓库中 {{product_name}} 的已开卷和裁切段米数为 {{available_meters}} 米（阈值 {{threshold}} 米）。"},
	"az":    {"Kritik stok (metr): {{product_name}}", "{{warehouse_name}} anbarında {{product_name}} üzrə açıq rulon və kəsik hissə metri {{available_meters}} m-dir (hədd {{threshold}} m)."},
	"ar":    {"مخزون حرج (أمتار): {{product_name}}", "أمتار اللفائف المفتوحة والقطع المقصوصة من {{product_name}} في المستودع {{warehouse_name}}: {{available_meters}} م (الحد {{threshold}} م)."},
	"fa":    {"موجودی بحرانی (متر): {{product_name}}", "متراژ رول\u200cهای باز و قطعه\u200cهای بریده\u200cشده {{product_name}} در انبار {{warehouse_name}}: {{available_meters}} متر (آستانه {{threshold}} متر)."},
}

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
	return reorderTemplates(stockReorderLowTexts)
}

func reorderTemplates(texts map[string]localizedText) []DefaultTemplate {
	out := make([]DefaultTemplate, 0, len(texts)*len(StockForecastChannels))
	for _, lang := range msgtemplate.Locales {
		t, ok := texts[lang]
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
	Register(Event{
		Code: EventStockReorderMetersLow, Module: "stock",
		DefaultChannels: StockForecastChannels, AudienceRoles: []string{RoleDealer, RoleDistributor, RoleCenter},
		Placeholders: []msgtemplate.Placeholder{
			ph("product_name", "PPF Şeffaf Film", "Clear PPF"),
			ph("warehouse_name", "Ana depo", "Main warehouse"),
			ph("available_meters", "12.50", "12.50"),
			ph("threshold", "30.00", "30.00"),
		},
		UserConfigurable: true,
		Templates:        reorderTemplates(stockReorderMetersLowTexts),
	})
}
