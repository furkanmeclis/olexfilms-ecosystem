package catalog

import "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/msgtemplate"

// EventServiceReviewLowScore notifies dealer owners when a submitted service
// review is at or below the low-score threshold.
const EventServiceReviewLowScore = "SERVICE_REVIEW_LOW_SCORE"

var ServiceReviewLowScoreChannels = []string{ChannelInapp, ChannelEmail}

var serviceReviewLowScoreTexts = map[string]localizedText{
	"tr":    {"Düşük değerlendirme alındı", "{{organization_name}} için {{service_no}} hizmetinde düşük puan alındı. Platform: {{platform_rating}}, ürün: {{product_rating}}."},
	"en":    {"Low review score received", "{{organization_name}} received a low score for service {{service_no}}. Platform: {{platform_rating}}, product: {{product_rating}}."},
	"bg":    {"Получена ниска оценка", "{{organization_name}} получи ниска оценка за услуга {{service_no}}. Платформа: {{platform_rating}}, продукт: {{product_rating}}."},
	"de":    {"Niedrige Bewertung erhalten", "{{organization_name}} hat für Service {{service_no}} eine niedrige Bewertung erhalten. Plattform: {{platform_rating}}, Produkt: {{product_rating}}."},
	"el":    {"Λήφθηκε χαμηλή αξιολόγηση", "Η {{organization_name}} έλαβε χαμηλή βαθμολογία για την υπηρεσία {{service_no}}. Πλατφόρμα: {{platform_rating}}, προϊόν: {{product_rating}}."},
	"uk":    {"Отримано низьку оцінку", "{{organization_name}} отримала низьку оцінку за послугу {{service_no}}. Платформа: {{platform_rating}}, продукт: {{product_rating}}."},
	"ru":    {"Получена низкая оценка", "{{organization_name}} получила низкую оценку за услугу {{service_no}}. Платформа: {{platform_rating}}, продукт: {{product_rating}}."},
	"fr":    {"Note faible reçue", "{{organization_name}} a reçu une note faible pour le service {{service_no}}. Plateforme : {{platform_rating}}, produit : {{product_rating}}."},
	"es":    {"Se recibió una valoración baja", "{{organization_name}} recibió una puntuación baja por el servicio {{service_no}}. Plataforma: {{platform_rating}}, producto: {{product_rating}}."},
	"it":    {"Ricevuta valutazione bassa", "{{organization_name}} ha ricevuto un punteggio basso per il servizio {{service_no}}. Piattaforma: {{platform_rating}}, prodotto: {{product_rating}}."},
	"zh-CN": {"收到低评分", "{{organization_name}} 的服务 {{service_no}} 收到低评分。平台：{{platform_rating}}，产品：{{product_rating}}。"},
	"az":    {"Aşağı qiymət alındı", "{{organization_name}} {{service_no}} xidməti üçün aşağı qiymət aldı. Platforma: {{platform_rating}}, məhsul: {{product_rating}}."},
	"ar":    {"تم استلام تقييم منخفض", "تلقت {{organization_name}} تقييمًا منخفضًا للخدمة {{service_no}}. المنصة: {{platform_rating}}، المنتج: {{product_rating}}."},
}

func serviceReviewLowScoreTemplates() []DefaultTemplate {
	out := make([]DefaultTemplate, 0, len(serviceReviewLowScoreTexts)*len(ServiceReviewLowScoreChannels))
	for _, lang := range msgtemplate.Locales {
		t, ok := serviceReviewLowScoreTexts[lang]
		if !ok {
			continue
		}
		for _, ch := range ServiceReviewLowScoreChannels {
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
		Code: EventServiceReviewLowScore, Module: "services",
		DefaultChannels: ServiceReviewLowScoreChannels,
		AudienceRoles:   []string{RoleDealer},
		Placeholders: []msgtemplate.Placeholder{
			ph("organization_name", "Tech Oto", "Tech Oto"),
			ph("service_no", "DS00001234", "DS00001234"),
			ph("platform_rating", "2", "2"),
			ph("product_rating", "1", "1"),
			ph("min_rating", "1", "1"),
		},
		UserConfigurable: true,
		Templates:        serviceReviewLowScoreTemplates(),
	})
}
