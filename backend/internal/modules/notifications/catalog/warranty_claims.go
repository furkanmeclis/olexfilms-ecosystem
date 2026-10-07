package catalog

import "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/msgtemplate"

const (
	EventWarrantyClaimOpened        = "WARRANTY_CLAIM_OPENED"
	EventWarrantyClaimStatusChanged = "WARRANTY_CLAIM_STATUS_CHANGED"
	EventWarrantyClaimResult        = "WARRANTY_CLAIM_RESULT"
	EventWarrantyClaimReopened      = "WARRANTY_CLAIM_REOPENED"
)

var warrantyClaimStaffChannels = []string{ChannelInapp}
var warrantyClaimResultChannels = []string{ChannelWhatsApp}

var warrantyClaimOpenedTexts = map[string]localizedText{
	"tr":    {"Yeni garanti talebi", "{{claim_no}} numaralı garanti talebi {{organization_name}} tarafından açıldı. Ürün: {{product_name}}, araç: {{plate}}."},
	"en":    {"New warranty claim", "Warranty claim {{claim_no}} was opened by {{organization_name}}. Product: {{product_name}}, vehicle: {{plate}}."},
	"bg":    {"Нова гаранционна претенция", "Гаранционна претенция {{claim_no}} беше отворена от {{organization_name}}. Продукт: {{product_name}}, автомобил: {{plate}}."},
	"de":    {"Neuer Garantiefall", "Garantiefall {{claim_no}} wurde von {{organization_name}} eröffnet. Produkt: {{product_name}}, Fahrzeug: {{plate}}."},
	"el":    {"Νέο αίτημα εγγύησης", "Το αίτημα εγγύησης {{claim_no}} άνοιξε από {{organization_name}}. Προϊόν: {{product_name}}, όχημα: {{plate}}."},
	"uk":    {"Нова гарантійна заявка", "Гарантійну заявку {{claim_no}} відкрила організація {{organization_name}}. Продукт: {{product_name}}, авто: {{plate}}."},
	"ru":    {"Новая гарантийная заявка", "Гарантийная заявка {{claim_no}} открыта организацией {{organization_name}}. Продукт: {{product_name}}, авто: {{plate}}."},
	"fr":    {"Nouvelle demande de garantie", "La demande de garantie {{claim_no}} a été ouverte par {{organization_name}}. Produit : {{product_name}}, véhicule : {{plate}}."},
	"es":    {"Nueva reclamación de garantía", "{{organization_name}} abrió la reclamación de garantía {{claim_no}}. Producto: {{product_name}}, vehículo: {{plate}}."},
	"it":    {"Nuova richiesta di garanzia", "La richiesta di garanzia {{claim_no}} è stata aperta da {{organization_name}}. Prodotto: {{product_name}}, veicolo: {{plate}}."},
	"zh-CN": {"新的质保申请", "{{organization_name}} 已提交质保申请 {{claim_no}}。产品：{{product_name}}，车辆：{{plate}}。"},
	"az":    {"Yeni zəmanət müraciəti", "{{organization_name}} tərəfindən {{claim_no}} nömrəli zəmanət müraciəti açıldı. Məhsul: {{product_name}}, avtomobil: {{plate}}."},
	"ar":    {"طلب ضمان جديد", "تم فتح طلب الضمان {{claim_no}} بواسطة {{organization_name}}. المنتج: {{product_name}}، المركبة: {{plate}}."},
}

var warrantyClaimStatusTexts = map[string]localizedText{
	"tr":    {"Garanti talebi durumu değişti", "{{claim_no}} numaralı garanti talebi {{from}} durumundan {{to}} durumuna geçti."},
	"en":    {"Warranty claim status changed", "Warranty claim {{claim_no}} moved from {{from}} to {{to}}."},
	"bg":    {"Статусът на гаранционната претенция се промени", "Гаранционна претенция {{claim_no}} премина от {{from}} към {{to}}."},
	"de":    {"Status des Garantiefalls geändert", "Garantiefall {{claim_no}} wechselte von {{from}} zu {{to}}."},
	"el":    {"Άλλαξε η κατάσταση αιτήματος εγγύησης", "Το αίτημα εγγύησης {{claim_no}} μετακινήθηκε από {{from}} σε {{to}}."},
	"uk":    {"Статус гарантійної заявки змінено", "Гарантійна заявка {{claim_no}} перейшла зі статусу {{from}} у {{to}}."},
	"ru":    {"Статус гарантийной заявки изменён", "Гарантийная заявка {{claim_no}} перешла из {{from}} в {{to}}."},
	"fr":    {"Statut de garantie modifié", "La demande de garantie {{claim_no}} est passée de {{from}} à {{to}}."},
	"es":    {"Estado de garantía actualizado", "La reclamación {{claim_no}} pasó de {{from}} a {{to}}."},
	"it":    {"Stato garanzia aggiornato", "La richiesta {{claim_no}} è passata da {{from}} a {{to}}."},
	"zh-CN": {"质保申请状态已更新", "质保申请 {{claim_no}} 已从 {{from}} 变为 {{to}}。"},
	"az":    {"Zəmanət müraciətinin statusu dəyişdi", "{{claim_no}} müraciəti {{from}} statusundan {{to}} statusuna keçdi."},
	"ar":    {"تغيّرت حالة طلب الضمان", "انتقل طلب الضمان {{claim_no}} من {{from}} إلى {{to}}."},
}

var warrantyClaimResultTexts = map[string]localizedText{
	"tr":    {"Garanti talebiniz sonuçlandı", "{{claim_no}} numaralı garanti talebiniz {{to}} olarak sonuçlandı. Ürün: {{product_name}}, araç: {{plate}}."},
	"en":    {"Your warranty claim is complete", "Your warranty claim {{claim_no}} was marked {{to}}. Product: {{product_name}}, vehicle: {{plate}}."},
	"bg":    {"Гаранционната ви претенция приключи", "Вашата претенция {{claim_no}} беше отбелязана като {{to}}. Продукт: {{product_name}}, автомобил: {{plate}}."},
	"de":    {"Ihr Garantiefall ist abgeschlossen", "Ihr Garantiefall {{claim_no}} wurde als {{to}} markiert. Produkt: {{product_name}}, Fahrzeug: {{plate}}."},
	"el":    {"Το αίτημα εγγύησής σας ολοκληρώθηκε", "Το αίτημα {{claim_no}} σημειώθηκε ως {{to}}. Προϊόν: {{product_name}}, όχημα: {{plate}}."},
	"uk":    {"Вашу гарантійну заявку завершено", "Вашу заявку {{claim_no}} позначено як {{to}}. Продукт: {{product_name}}, авто: {{plate}}."},
	"ru":    {"Ваша гарантийная заявка завершена", "Ваша заявка {{claim_no}} отмечена как {{to}}. Продукт: {{product_name}}, авто: {{plate}}."},
	"fr":    {"Votre demande de garantie est terminée", "Votre demande {{claim_no}} est marquée {{to}}. Produit : {{product_name}}, véhicule : {{plate}}."},
	"es":    {"Su reclamación de garantía finalizó", "Su reclamación {{claim_no}} quedó como {{to}}. Producto: {{product_name}}, vehículo: {{plate}}."},
	"it":    {"La sua richiesta di garanzia è conclusa", "La richiesta {{claim_no}} è stata segnata come {{to}}. Prodotto: {{product_name}}, veicolo: {{plate}}."},
	"zh-CN": {"您的质保申请已完成", "您的质保申请 {{claim_no}} 已标记为 {{to}}。产品：{{product_name}}，车辆：{{plate}}。"},
	"az":    {"Zəmanət müraciətiniz tamamlandı", "{{claim_no}} müraciətiniz {{to}} kimi qeyd edildi. Məhsul: {{product_name}}, avtomobil: {{plate}}."},
	"ar":    {"اكتمل طلب الضمان الخاص بك", "تم وضع طلبك {{claim_no}} بالحالة {{to}}. المنتج: {{product_name}}، المركبة: {{plate}}."},
}

var warrantyClaimReopenedTexts = map[string]localizedText{
	"tr":    {"Garanti talebi yeniden açıldı", "{{claim_no}} numaralı garanti talebi yeniden approved durumuna alındı. Sebep: {{reason}}"},
	"en":    {"Warranty claim reopened", "Warranty claim {{claim_no}} was moved back to approved. Reason: {{reason}}"},
	"bg":    {"Гаранционната претенция е отворена отново", "Гаранционна претенция {{claim_no}} е върната към approved. Причина: {{reason}}"},
	"de":    {"Garantiefall wieder geöffnet", "Garantiefall {{claim_no}} wurde wieder auf approved gesetzt. Grund: {{reason}}"},
	"el":    {"Το αίτημα εγγύησης άνοιξε ξανά", "Το αίτημα εγγύησης {{claim_no}} επέστρεψε σε approved. Αιτία: {{reason}}"},
	"uk":    {"Гарантійну заявку відкрито повторно", "Гарантійну заявку {{claim_no}} повернуто в approved. Причина: {{reason}}"},
	"ru":    {"Гарантийная заявка открыта повторно", "Гарантийная заявка {{claim_no}} возвращена в approved. Причина: {{reason}}"},
	"fr":    {"Demande de garantie rouverte", "La demande {{claim_no}} est repassée en approved. Motif : {{reason}}"},
	"es":    {"Reclamación de garantía reabierta", "La reclamación {{claim_no}} volvió a approved. Motivo: {{reason}}"},
	"it":    {"Richiesta in garanzia riaperta", "La richiesta {{claim_no}} è tornata ad approved. Motivo: {{reason}}"},
	"zh-CN": {"质保申请已重新打开", "质保申请 {{claim_no}} 已恢复为 approved。原因：{{reason}}"},
	"az":    {"Zəmanət müraciəti yenidən açıldı", "{{claim_no}} müraciəti yenidən approved statusuna qaytarıldı. Səbəb: {{reason}}"},
	"ar":    {"أُعيد فتح طلب الضمان", "تمت إعادة طلب الضمان {{claim_no}} إلى approved. السبب: {{reason}}"},
}

func warrantyClaimTemplates(texts map[string]localizedText, channels []string, role string) []DefaultTemplate {
	out := make([]DefaultTemplate, 0, len(texts)*len(channels))
	for _, lang := range msgtemplate.Locales {
		t, ok := texts[lang]
		if !ok {
			continue
		}
		for _, ch := range channels {
			out = append(out, DefaultTemplate{
				Role: role, Channel: ch, Language: lang, Subject: t.subject, Body: t.body, Format: "text",
			})
		}
	}
	return out
}

func warrantyClaimPlaceholders() []msgtemplate.Placeholder {
	return []msgtemplate.Placeholder{
		ph("claim_no", "42", "42"),
		ph("from", "dealer_review", "dealer_review"),
		ph("to", "approved", "approved"),
		ph("organization_name", "Tech Oto", "Tech Oto"),
		ph("product_name", "PPF Parlak", "PPF Gloss"),
		ph("plate", "34 ABC 123", "34 ABC 123"),
		ph("reason", "İptal düzeltildi", "Cancellation corrected"),
	}
}

func init() {
	Register(Event{
		Code: EventWarrantyClaimOpened, Module: "warranty_claims",
		DefaultChannels: warrantyClaimStaffChannels,
		AudienceRoles:   []string{RoleDistributor, RoleCenter},
		Placeholders:    warrantyClaimPlaceholders(), UserConfigurable: true,
		Templates: warrantyClaimTemplates(warrantyClaimOpenedTexts, warrantyClaimStaffChannels, RoleGeneric),
	})
	Register(Event{
		Code: EventWarrantyClaimStatusChanged, Module: "warranty_claims",
		DefaultChannels: warrantyClaimStaffChannels,
		AudienceRoles:   []string{RoleDealer, RoleDistributor},
		Placeholders:    warrantyClaimPlaceholders(), UserConfigurable: true,
		Templates: warrantyClaimTemplates(warrantyClaimStatusTexts, warrantyClaimStaffChannels, RoleGeneric),
	})
	Register(Event{
		Code: EventWarrantyClaimResult, Module: "warranty_claims",
		DefaultChannels: warrantyClaimResultChannels,
		AudienceRoles:   []string{RoleCustomer},
		Placeholders:    warrantyClaimPlaceholders(), UserConfigurable: true,
		Templates: warrantyClaimTemplates(warrantyClaimResultTexts, warrantyClaimResultChannels, RoleGeneric),
	})
	Register(Event{
		Code: EventWarrantyClaimReopened, Module: "warranty_claims",
		DefaultChannels: warrantyClaimStaffChannels,
		AudienceRoles:   []string{RoleDealer, RoleCenter},
		Placeholders:    warrantyClaimPlaceholders(), UserConfigurable: true,
		Templates: warrantyClaimTemplates(warrantyClaimReopenedTexts, warrantyClaimStaffChannels, RoleGeneric),
	})
}
