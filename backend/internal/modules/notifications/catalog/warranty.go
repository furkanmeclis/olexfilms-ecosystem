package catalog

import "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/msgtemplate"

// Warranty customer notifications (TEC-187, TEC-98 decision 5). The
// warranty cron writes warranty.expiring_soon (days 30 or 7) and
// warranty.expired outbox events; the bus handler dispatches these catalog
// events to the warranty holder. Default templates ship in all 13 locales
// for WhatsApp (wuzapi, K16/K21) and in-app; the generic role covers a
// holder who is also an organization member.
const (
	EventWarrantyExpiringSoon = "WARRANTY_EXPIRING_SOON"
	EventWarrantyExpired      = "WARRANTY_EXPIRED"
)

// WarrantyChannels are the default channels of the warranty events.
var WarrantyChannels = []string{ChannelWhatsApp, ChannelInapp}

type localizedText struct{ subject, body string }

var warrantyExpiringSoonTexts = map[string]localizedText{
	"tr": {"Garantiniz {{days}} gün içinde sona eriyor",
		"{{plate}} plakalı aracınızdaki {{product_name}} garantisi {{end_date}} tarihinde sona erecek ({{days}} gün kaldı). Bilgi için {{organization_name}} ile iletişime geçebilirsiniz. Garanti: {{verify_url}}"},
	"en": {"Your warranty ends in {{days}} days",
		"The {{product_name}} warranty on your vehicle {{plate}} ends on {{end_date}} ({{days}} days left). Contact {{organization_name}} for details. Warranty: {{verify_url}}"},
	"bg": {"Гаранцията ви изтича след {{days}} дни",
		"Гаранцията за {{product_name}} на вашия автомобил {{plate}} изтича на {{end_date}} (остават {{days}} дни). За подробности се свържете с {{organization_name}}. Гаранция: {{verify_url}}"},
	"de": {"Ihre Garantie endet in {{days}} Tagen",
		"Die Garantie für {{product_name}} an Ihrem Fahrzeug {{plate}} endet am {{end_date}} (noch {{days}} Tage). Für Details wenden Sie sich an {{organization_name}}. Garantie: {{verify_url}}"},
	"el": {"Η εγγύησή σας λήγει σε {{days}} ημέρες",
		"Η εγγύηση {{product_name}} του οχήματός σας {{plate}} λήγει στις {{end_date}} (απομένουν {{days}} ημέρες). Για λεπτομέρειες επικοινωνήστε με {{organization_name}}. Εγγύηση: {{verify_url}}"},
	"uk": {"Ваша гарантія закінчується через {{days}} дн.",
		"Гарантія на {{product_name}} для вашого автомобіля {{plate}} закінчується {{end_date}} (залишилось днів: {{days}}). За подробицями звертайтеся до {{organization_name}}. Гарантія: {{verify_url}}"},
	"ru": {"Ваша гарантия истекает через {{days}} дн.",
		"Гарантия на {{product_name}} для вашего автомобиля {{plate}} истекает {{end_date}} (осталось дней: {{days}}). За подробностями обращайтесь в {{organization_name}}. Гарантия: {{verify_url}}"},
	"fr": {"Votre garantie expire dans {{days}} jours",
		"La garantie {{product_name}} de votre véhicule {{plate}} expire le {{end_date}} (encore {{days}} jours). Contactez {{organization_name}} pour plus d'informations. Garantie : {{verify_url}}"},
	"es": {"Su garantía vence en {{days}} días",
		"La garantía de {{product_name}} de su vehículo {{plate}} vence el {{end_date}} (quedan {{days}} días). Contacte con {{organization_name}} para más información. Garantía: {{verify_url}}"},
	"it": {"La sua garanzia scade tra {{days}} giorni",
		"La garanzia {{product_name}} del suo veicolo {{plate}} scade il {{end_date}} (mancano {{days}} giorni). Per informazioni contatti {{organization_name}}. Garanzia: {{verify_url}}"},
	"zh-CN": {"您的质保将在 {{days}} 天后到期",
		"您的车辆 {{plate}} 的 {{product_name}} 质保将于 {{end_date}} 到期（剩余 {{days}} 天）。详情请联系 {{organization_name}}。质保信息：{{verify_url}}"},
	"az": {"Zəmanətinizin bitməsinə {{days}} gün qalıb",
		"{{plate}} nömrəli avtomobilinizdəki {{product_name}} zəmanəti {{end_date}} tarixində bitir ({{days}} gün qalıb). Ətraflı məlumat üçün {{organization_name}} ilə əlaqə saxlayın. Zəmanət: {{verify_url}}"},
	"ar": {"ينتهي ضمانك خلال {{days}} يومًا",
		"ينتهي ضمان {{product_name}} لمركبتك {{plate}} في {{end_date}} (متبقٍ {{days}} يومًا). للتفاصيل تواصل مع {{organization_name}}. الضمان: {{verify_url}}"},
}

var warrantyExpiredTexts = map[string]localizedText{
	"tr": {"Garantinizin süresi doldu",
		"{{plate}} plakalı aracınızdaki {{product_name}} garantisinin süresi {{end_date}} tarihinde doldu. Yenileme için {{organization_name}} ile iletişime geçebilirsiniz. Garanti: {{verify_url}}"},
	"en": {"Your warranty has expired",
		"The {{product_name}} warranty on your vehicle {{plate}} expired on {{end_date}}. Contact {{organization_name}} to renew it. Warranty: {{verify_url}}"},
	"bg": {"Гаранцията ви е изтекла",
		"Гаранцията за {{product_name}} на вашия автомобил {{plate}} изтече на {{end_date}}. За подновяване се свържете с {{organization_name}}. Гаранция: {{verify_url}}"},
	"de": {"Ihre Garantie ist abgelaufen",
		"Die Garantie für {{product_name}} an Ihrem Fahrzeug {{plate}} ist am {{end_date}} abgelaufen. Für eine Erneuerung wenden Sie sich an {{organization_name}}. Garantie: {{verify_url}}"},
	"el": {"Η εγγύησή σας έληξε",
		"Η εγγύηση {{product_name}} του οχήματός σας {{plate}} έληξε στις {{end_date}}. Για ανανέωση επικοινωνήστε με {{organization_name}}. Εγγύηση: {{verify_url}}"},
	"uk": {"Термін вашої гарантії закінчився",
		"Гарантія на {{product_name}} для вашого автомобіля {{plate}} закінчилася {{end_date}}. Для продовження звертайтеся до {{organization_name}}. Гарантія: {{verify_url}}"},
	"ru": {"Срок вашей гарантии истёк",
		"Гарантия на {{product_name}} для вашего автомобиля {{plate}} истекла {{end_date}}. Для продления обращайтесь в {{organization_name}}. Гарантия: {{verify_url}}"},
	"fr": {"Votre garantie a expiré",
		"La garantie {{product_name}} de votre véhicule {{plate}} a expiré le {{end_date}}. Contactez {{organization_name}} pour la renouveler. Garantie : {{verify_url}}"},
	"es": {"Su garantía ha vencido",
		"La garantía de {{product_name}} de su vehículo {{plate}} venció el {{end_date}}. Contacte con {{organization_name}} para renovarla. Garantía: {{verify_url}}"},
	"it": {"La sua garanzia è scaduta",
		"La garanzia {{product_name}} del suo veicolo {{plate}} è scaduta il {{end_date}}. Per rinnovarla contatti {{organization_name}}. Garanzia: {{verify_url}}"},
	"zh-CN": {"您的质保已到期",
		"您的车辆 {{plate}} 的 {{product_name}} 质保已于 {{end_date}} 到期。如需续保，请联系 {{organization_name}}。质保信息：{{verify_url}}"},
	"az": {"Zəmanətinizin müddəti bitdi",
		"{{plate}} nömrəli avtomobilinizdəki {{product_name}} zəmanətinin müddəti {{end_date}} tarixində bitdi. Yeniləmək üçün {{organization_name}} ilə əlaqə saxlayın. Zəmanət: {{verify_url}}"},
	"ar": {"انتهت صلاحية ضمانك",
		"انتهى ضمان {{product_name}} لمركبتك {{plate}} في {{end_date}}. لتجديده تواصل مع {{organization_name}}. الضمان: {{verify_url}}"},
}

func warrantyTemplates(texts map[string]localizedText) []DefaultTemplate {
	out := make([]DefaultTemplate, 0, len(texts)*len(WarrantyChannels))
	for _, lang := range msgtemplate.Locales {
		t, ok := texts[lang]
		if !ok {
			continue // TestWarrantyTemplatesCoverEveryLocale reports it
		}
		for _, ch := range WarrantyChannels {
			out = append(out, DefaultTemplate{
				Role: RoleGeneric, Channel: ch, Language: lang,
				Subject: t.subject, Body: t.body, Format: "text",
			})
		}
	}
	return out
}

func warrantyPlaceholders(withDays bool) []msgtemplate.Placeholder {
	out := []msgtemplate.Placeholder{
		ph("plate", "34 ABC 123", "34 ABC 123"),
		ph("product_name", "PPF Parlak", "PPF Gloss"),
		ph("end_date", "2027-03-31", "2027-03-31"),
		ph("organization_name", "Tech Oto", "Tech Oto"),
		ph("verify_url", "https://olexfilms.app/garanti/Ab3dEf6hIj9k", "https://olexfilms.app/garanti/Ab3dEf6hIj9k"),
	}
	if withDays {
		out = append(out, ph("days", "30", "30"))
	}
	return out
}

func init() {
	Register(Event{
		Code: EventWarrantyExpiringSoon, Module: "warranty",
		DefaultChannels:  WarrantyChannels,
		AudienceRoles:    []string{RoleCustomer},
		Placeholders:     warrantyPlaceholders(true),
		UserConfigurable: true,
		Templates:        warrantyTemplates(warrantyExpiringSoonTexts),
	})
	Register(Event{
		Code: EventWarrantyExpired, Module: "warranty",
		DefaultChannels:  WarrantyChannels,
		AudienceRoles:    []string{RoleCustomer},
		Placeholders:     warrantyPlaceholders(false),
		UserConfigurable: true,
		Templates:        warrantyTemplates(warrantyExpiredTexts),
	})
}
