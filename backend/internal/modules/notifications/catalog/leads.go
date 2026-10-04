package catalog

import "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/msgtemplate"

// Dealer application notifications (TEC-317, F3-03f). A public dealer
// application opens a lead in the territory distributor (or the brand
// center); the members of that organization holding leads.read are told
// over in-app and e-mail. Default templates ship in all 13 locales and are
// seeded with InsertNotificationTemplateIfMissing by SyncCatalog.
const EventLeadDealerApplication = "LEAD_DEALER_APPLICATION"

// LeadApplicationChannels are the default channels of the event.
var LeadApplicationChannels = []string{ChannelInapp, ChannelEmail}

var leadApplicationTexts = map[string]localizedText{
	"tr": {"Yeni bayi başvurusu: {{company_name}}",
		"{{company_name}} ({{contact_name}}, {{location}}) bayilik başvurusu yaptı. Başvuru potansiyel müşteri listenize eklendi; lütfen panelden inceleyin."},
	"en": {"New dealer application: {{company_name}}",
		"{{company_name}} ({{contact_name}}, {{location}}) applied to become a dealer. The application was added to your leads; please review it in the panel."},
	"bg": {"Нова заявка за дилър: {{company_name}}",
		"{{company_name}} ({{contact_name}}, {{location}}) кандидатства да стане дилър. Заявката е добавена към вашите потенциални клиенти; моля, прегледайте я в панела."},
	"de": {"Neue Händlerbewerbung: {{company_name}}",
		"{{company_name}} ({{contact_name}}, {{location}}) hat sich als Händler beworben. Die Bewerbung wurde Ihren Leads hinzugefügt; bitte prüfen Sie sie im Panel."},
	"el": {"Νέα αίτηση αντιπροσώπου: {{company_name}}",
		"Η {{company_name}} ({{contact_name}}, {{location}}) υπέβαλε αίτηση για να γίνει αντιπρόσωπος. Η αίτηση προστέθηκε στους υποψήφιους πελάτες σας· ελέγξτε τη στον πίνακα."},
	"uk": {"Нова заявка дилера: {{company_name}}",
		"{{company_name}} ({{contact_name}}, {{location}}) подала заявку на дилерство. Заявку додано до ваших лідів; перегляньте її в панелі."},
	"ru": {"Новая заявка дилера: {{company_name}}",
		"{{company_name}} ({{contact_name}}, {{location}}) подала заявку на дилерство. Заявка добавлена в ваши лиды; проверьте её в панели."},
	"fr": {"Nouvelle candidature de revendeur : {{company_name}}",
		"{{company_name}} ({{contact_name}}, {{location}}) a postulé pour devenir revendeur. La candidature a été ajoutée à vos prospects ; veuillez l'examiner dans le panneau."},
	"es": {"Nueva solicitud de distribuidor: {{company_name}}",
		"{{company_name}} ({{contact_name}}, {{location}}) solicitó ser distribuidor. La solicitud se añadió a sus clientes potenciales; revísela en el panel."},
	"it": {"Nuova candidatura rivenditore: {{company_name}}",
		"{{company_name}} ({{contact_name}}, {{location}}) si è candidata come rivenditore. La candidatura è stata aggiunta ai suoi lead; la verifichi nel pannello."},
	"zh-CN": {"新的经销商申请：{{company_name}}",
		"{{company_name}}（{{contact_name}}，{{location}}）申请成为经销商。该申请已加入您的潜在客户列表，请在面板中查看。"},
	"az": {"Yeni diler müraciəti: {{company_name}}",
		"{{company_name}} ({{contact_name}}, {{location}}) diler olmaq üçün müraciət etdi. Müraciət potensial müştərilərinizə əlavə olundu; zəhmət olmasa paneldə nəzərdən keçirin."},
	"ar": {"طلب وكيل جديد: {{company_name}}",
		"تقدّمت {{company_name}} ({{contact_name}}، {{location}}) بطلب لتصبح وكيلاً. أُضيف الطلب إلى العملاء المحتملين لديك؛ يرجى مراجعته في اللوحة."},
}

func leadApplicationTemplates() []DefaultTemplate {
	out := make([]DefaultTemplate, 0, len(leadApplicationTexts)*len(LeadApplicationChannels))
	for _, lang := range msgtemplate.Locales {
		t, ok := leadApplicationTexts[lang]
		if !ok {
			continue // TestLeadApplicationTemplatesCoverEveryLocale reports it
		}
		for _, ch := range LeadApplicationChannels {
			out = append(out, DefaultTemplate{
				Role: RoleGeneric, Channel: ch, Language: lang,
				Subject: t.subject, Body: t.body, Format: "text",
			})
		}
	}
	return out
}

func init() {
	Register(Event{
		Code: EventLeadDealerApplication, Module: "leads",
		DefaultChannels: LeadApplicationChannels,
		AudienceRoles:   []string{RoleDistributor, RoleCenter},
		Placeholders: []msgtemplate.Placeholder{
			ph("company_name", "Kuzey Oto Film", "North Auto Film"),
			ph("contact_name", "Ayşe Yılmaz", "Jane Doe"),
			ph("location", "Berlin, Almanya", "Berlin, Germany"),
		},
		UserConfigurable: true,
		Templates:        leadApplicationTemplates(),
	})
}
