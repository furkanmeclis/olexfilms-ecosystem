package catalog

import "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/msgtemplate"

// Dealer application notifications (TEC-317, F3-03f). A public dealer
// application opens a lead in the territory distributor (or the brand
// center); the members of that organization holding leads.read are told
// over in-app and e-mail. Default templates ship in all 13 locales and are
// seeded with InsertNotificationTemplateIfMissing by SyncCatalog.
const EventLeadDealerApplication = "LEAD_DEALER_APPLICATION"
const EventLeadWebsiteReceived = "LEAD_WEBSITE_RECEIVED"

// LeadApplicationChannels are the default channels of the event.
var LeadApplicationChannels = []string{ChannelInapp, ChannelEmail}
var LeadWebsiteChannels = []string{ChannelInapp, ChannelEmail, ChannelWhatsApp}

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

var leadWebsiteTexts = map[string]localizedText{
	"tr":    {"Yeni vitrin talebi: {{contact_name}}", "{{contact_name}} ({{phone}}) #{{dealer_code}} vitrin formundan teklif istedi. Lead listenize eklendi."},
	"en":    {"New showcase request: {{contact_name}}", "{{contact_name}} ({{phone}}) requested a quote from showcase #{{dealer_code}}. It was added to your leads."},
	"bg":    {"Ново запитване от витрина: {{contact_name}}", "{{contact_name}} ({{phone}}) поиска оферта от витрина #{{dealer_code}}. Добавено е към вашите лийдове."},
	"de":    {"Neue Showcase-Anfrage: {{contact_name}}", "{{contact_name}} ({{phone}}) hat über Showcase #{{dealer_code}} ein Angebot angefragt. Der Lead wurde hinzugefügt."},
	"el":    {"Νέο αίτημα βιτρίνας: {{contact_name}}", "Ο/Η {{contact_name}} ({{phone}}) ζήτησε προσφορά από τη βιτρίνα #{{dealer_code}}. Προστέθηκε στους υποψήφιους πελάτες."},
	"uk":    {"Новий запит з вітрини: {{contact_name}}", "{{contact_name}} ({{phone}}) попросив/ла пропозицію з вітрини #{{dealer_code}}. Лід додано."},
	"ru":    {"Новый запрос с витрины: {{contact_name}}", "{{contact_name}} ({{phone}}) запросил(а) предложение с витрины #{{dealer_code}}. Лид добавлен."},
	"fr":    {"Nouvelle demande vitrine : {{contact_name}}", "{{contact_name}} ({{phone}}) a demandé un devis depuis la vitrine #{{dealer_code}}. Le prospect a été ajouté."},
	"es":    {"Nueva solicitud de vitrina: {{contact_name}}", "{{contact_name}} ({{phone}}) pidió un presupuesto desde la vitrina #{{dealer_code}}. El lead se añadió."},
	"it":    {"Nuova richiesta vetrina: {{contact_name}}", "{{contact_name}} ({{phone}}) ha richiesto un preventivo dalla vetrina #{{dealer_code}}. Il lead è stato aggiunto."},
	"zh-CN": {"新的展示页请求：{{contact_name}}", "{{contact_name}}（{{phone}}）从展示页 #{{dealer_code}} 请求报价。已加入线索列表。"},
	"az":    {"Yeni vitrin sorğusu: {{contact_name}}", "{{contact_name}} ({{phone}}) #{{dealer_code}} vitrinindən təklif istədi. Lead siyahınıza əlavə olundu."},
	"ar":    {"طلب واجهة جديد: {{contact_name}}", "طلب {{contact_name}} ({{phone}}) عرض سعر من الواجهة #{{dealer_code}}. تمت إضافته إلى قائمة العملاء المحتملين."},
}

func leadWebsiteTemplates() []DefaultTemplate {
	out := make([]DefaultTemplate, 0, len(leadWebsiteTexts)*len(LeadWebsiteChannels))
	for _, lang := range msgtemplate.Locales {
		t, ok := leadWebsiteTexts[lang]
		if !ok {
			continue
		}
		for _, ch := range LeadWebsiteChannels {
			out = append(out, DefaultTemplate{Role: RoleGeneric, Channel: ch, Language: lang, Subject: t.subject, Body: t.body, Format: "text"})
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
	Register(Event{
		Code: EventLeadWebsiteReceived, Module: "leads",
		DefaultChannels: LeadWebsiteChannels,
		AudienceRoles:   []string{RoleDealer},
		Placeholders: []msgtemplate.Placeholder{
			ph("contact_name", "Ayşe Yılmaz", "Jane Doe"),
			ph("phone", "+905551234567", "+491701234567"),
			ph("dealer_code", "ankara-ppf", "berlin-ppf"),
		},
		UserConfigurable: true,
		Templates:        leadWebsiteTemplates(),
	})
}
