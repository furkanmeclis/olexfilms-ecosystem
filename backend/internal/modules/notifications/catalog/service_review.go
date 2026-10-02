package catalog

import "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/msgtemplate"

// Service review request (TEC-192, F1-06h; TEC-98 decision 7). The delayed
// service:review_request task writes service.review_requested once per
// service; the bus handler dispatches this event to the service customer
// over WhatsApp (wuzapi, K16/K21). review_url is the dealer's
// google_business_url. Default templates ship in all 13 locales and are
// seeded with InsertNotificationTemplateIfMissing by SyncCatalog.
const EventServiceReviewRequest = "SERVICE_REVIEW_REQUEST"

// ServiceReviewChannels are the default channels of the review request.
var ServiceReviewChannels = []string{ChannelWhatsApp}

var serviceReviewRequestTexts = map[string]localizedText{
	"tr": {"Hizmetimizi değerlendirir misiniz?",
		"Merhaba, {{organization_name}} olarak aracınıza yaptığımız hizmetten memnun kaldıysanız birkaç saniyenizi ayırıp bizi Google'da değerlendirir misiniz? {{review_url}} Teşekkür ederiz!"},
	"en": {"How did we do?",
		"Hello, if you were happy with the service {{organization_name}} provided for your vehicle, could you take a few seconds to review us on Google? {{review_url}} Thank you!"},
	"bg": {"Как се справихме?",
		"Здравейте, ако сте доволни от услугата, която {{organization_name}} извърши за вашия автомобил, бихте ли отделили няколко секунди, за да ни оцените в Google? {{review_url}} Благодарим ви!"},
	"de": {"Wie zufrieden waren Sie?",
		"Hallo, wenn Sie mit dem Service von {{organization_name}} an Ihrem Fahrzeug zufrieden waren, würden Sie uns in wenigen Sekunden auf Google bewerten? {{review_url}} Vielen Dank!"},
	"el": {"Πώς τα πήγαμε;",
		"Γεια σας, αν μείνατε ικανοποιημένοι από την υπηρεσία που παρείχε η {{organization_name}} στο όχημά σας, θα αφιερώνατε λίγα δευτερόλεπτα για να μας αξιολογήσετε στη Google; {{review_url}} Σας ευχαριστούμε!"},
	"uk": {"Як ми впоралися?",
		"Вітаємо! Якщо ви задоволені послугою, яку {{organization_name}} надала для вашого автомобіля, приділіть, будь ласка, кілька секунд, щоб оцінити нас у Google: {{review_url}} Дякуємо!"},
	"ru": {"Как мы справились?",
		"Здравствуйте! Если вы довольны услугой, которую {{organization_name}} оказала для вашего автомобиля, уделите, пожалуйста, несколько секунд, чтобы оценить нас в Google: {{review_url}} Спасибо!"},
	"fr": {"Votre avis compte",
		"Bonjour, si vous êtes satisfait du service réalisé par {{organization_name}} sur votre véhicule, pourriez-vous prendre quelques secondes pour nous évaluer sur Google ? {{review_url}} Merci !"},
	"es": {"¿Qué tal lo hicimos?",
		"Hola, si quedó satisfecho con el servicio que {{organization_name}} realizó en su vehículo, ¿podría dedicar unos segundos a valorarnos en Google? {{review_url}} ¡Gracias!"},
	"it": {"Com'è andata?",
		"Salve, se è rimasto soddisfatto del servizio che {{organization_name}} ha eseguito sul suo veicolo, potrebbe dedicare qualche secondo a recensirci su Google? {{review_url}} Grazie!"},
	"zh-CN": {"请评价我们的服务",
		"您好，如果您对 {{organization_name}} 为您的车辆提供的服务感到满意，能否花几秒钟在 Google 上为我们评价？{{review_url}} 谢谢！"},
	"az": {"Xidmətimizi qiymətləndirərdiniz?",
		"Salam, {{organization_name}} olaraq avtomobilinizə göstərdiyimiz xidmətdən razı qaldınızsa, bir neçə saniyə ayırıb bizi Google-da qiymətləndirərdiniz? {{review_url}} Təşəkkür edirik!"},
	"ar": {"كيف كانت خدمتنا؟",
		"مرحبًا، إذا كنت راضيًا عن الخدمة التي قدمتها {{organization_name}} لمركبتك، هل يمكنك تخصيص بضع ثوانٍ لتقييمنا على Google؟ {{review_url}} شكرًا لك!"},
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
			ph("plate", "34 ABC 123", "34 ABC 123"),
			ph("service_no", "DS00001234", "DS00001234"),
		},
		UserConfigurable: true,
		Templates:        serviceReviewTemplates(),
	})
}
