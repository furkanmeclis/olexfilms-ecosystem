package catalog

import "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/msgtemplate"

// Service review request (TEC-192, F1-06h; TEC-98 decision 7). The delayed
// service:review_request task writes service.review_requested once per
// service; the bus handler dispatches this event to the service customer
// over WhatsApp (wuzapi, K16/K21). form_url is the platform review form and
// review_url is the optional dealer Google link. Default templates ship in all 13 locales and are
// seeded with InsertNotificationTemplateIfMissing by SyncCatalog.
const EventServiceReviewRequest = "SERVICE_REVIEW_REQUEST"

// ServiceReviewChannels are the default channels of the review request.
var ServiceReviewChannels = []string{ChannelWhatsApp}

var serviceReviewRequestTexts = map[string]localizedText{
	"tr": {"Hizmetimizi değerlendirir misiniz?",
		"Merhaba, {{organization_name}} hizmetinizi değerlendirmek için formu açabilirsiniz: {{form_url}} Google yorum linkimiz: {{review_url}} Teşekkür ederiz!"},
	"en": {"How did we do?",
		"Hello, you can review the service from {{organization_name}} here: {{form_url}} Our Google review link: {{review_url}} Thank you!"},
	"bg": {"Как се справихме?",
		"Здравейте, можете да оцените услугата от {{organization_name}} тук: {{form_url}} Линк за Google отзив: {{review_url}} Благодарим ви!"},
	"de": {"Wie zufrieden waren Sie?",
		"Hallo, Sie können den Service von {{organization_name}} hier bewerten: {{form_url}} Unser Google-Bewertungslink: {{review_url}} Vielen Dank!"},
	"el": {"Πώς τα πήγαμε;",
		"Γεια σας, μπορείτε να αξιολογήσετε την υπηρεσία της {{organization_name}} εδώ: {{form_url}} Σύνδεσμος Google: {{review_url}} Ευχαριστούμε!"},
	"uk": {"Як ми впоралися?",
		"Вітаємо! Оцініть послугу від {{organization_name}} тут: {{form_url}} Посилання для Google-відгуку: {{review_url}} Дякуємо!"},
	"ru": {"Как мы справились?",
		"Здравствуйте! Оцените услугу от {{organization_name}} здесь: {{form_url}} Ссылка для отзыва в Google: {{review_url}} Спасибо!"},
	"fr": {"Votre avis compte",
		"Bonjour, vous pouvez évaluer le service de {{organization_name}} ici : {{form_url}} Lien d'avis Google : {{review_url}} Merci !"},
	"es": {"¿Qué tal lo hicimos?",
		"Hola, puede valorar el servicio de {{organization_name}} aquí: {{form_url}} Enlace de reseña en Google: {{review_url}} ¡Gracias!"},
	"it": {"Com'è andata?",
		"Salve, può valutare il servizio di {{organization_name}} qui: {{form_url}} Link recensione Google: {{review_url}} Grazie!"},
	"zh-CN": {"请评价我们的服务",
		"您好，您可以在这里评价 {{organization_name}} 的服务：{{form_url}} Google 评价链接：{{review_url}} 谢谢！"},
	"az": {"Xidmətimizi qiymətləndirərdiniz?",
		"Salam, {{organization_name}} xidmətini buradan qiymətləndirə bilərsiniz: {{form_url}} Google rəy linki: {{review_url}} Təşəkkür edirik!"},
	"ar": {"كيف كانت خدمتنا؟",
		"مرحبًا، يمكنك تقييم خدمة {{organization_name}} من هنا: {{form_url}} رابط تقييم Google: {{review_url}} شكرًا لك!"},
}

func serviceReviewTemplates() []DefaultTemplate {
	out := make([]DefaultTemplate, 0, len(serviceReviewRequestTexts)*len(ServiceReviewChannels))
	for _, lang := range msgtemplate.Locales {
		t, ok := serviceReviewRequestTexts[lang]
		if !ok {
			continue // TestServiceReviewTemplatesCoverEveryLocale reports it
		}
		for _, ch := range ServiceReviewChannels {
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
		Code: EventServiceReviewRequest, Module: "services",
		DefaultChannels: ServiceReviewChannels,
		AudienceRoles:   []string{RoleCustomer},
		Placeholders: []msgtemplate.Placeholder{
			ph("organization_name", "Tech Oto", "Tech Oto"),
			ph("review_url", "https://g.page/r/tech-oto/review", "https://g.page/r/tech-oto/review"),
			ph("form_url", "https://olexfilms.app/s/AbCdEfGhIj", "https://olexfilms.app/s/AbCdEfGhIj"),
			ph("plate", "34 ABC 123", "34 ABC 123"),
			ph("service_no", "DS00001234", "DS00001234"),
		},
		UserConfigurable: true,
		Templates:        serviceReviewTemplates(),
	})
}
