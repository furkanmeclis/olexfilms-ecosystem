package catalog

import "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/msgtemplate"

// Quote sent notifications (TEC-315). Quotes are delivered over the single
// WhatsApp provider number (K16/K21); the use case writes quote.sent with a
// recipient phone and public short URL.
const EventQuoteSent = "QUOTE_SENT"

var QuoteSentChannels = []string{ChannelWhatsApp}

var quoteSentTexts = map[string]localizedText{
	"tr":    {"Teklifiniz hazır", "Merhaba {{recipient_name}}, {{organization_name}} teklifiniz hazır: {{quote_url}} Toplam: {{total_amount}}"},
	"en":    {"Your quote is ready", "Hello {{recipient_name}}, your quote from {{organization_name}} is ready: {{quote_url}} Total: {{total_amount}}"},
	"bg":    {"Вашата оферта е готова", "Здравейте {{recipient_name}}, офертата ви от {{organization_name}} е готова: {{quote_url}} Общо: {{total_amount}}"},
	"de":    {"Ihr Angebot ist bereit", "Hallo {{recipient_name}}, Ihr Angebot von {{organization_name}} ist bereit: {{quote_url}} Gesamt: {{total_amount}}"},
	"el":    {"Η προσφορά σας είναι έτοιμη", "Γεια σας {{recipient_name}}, η προσφορά σας από {{organization_name}} είναι έτοιμη: {{quote_url}} Σύνολο: {{total_amount}}"},
	"uk":    {"Ваша пропозиція готова", "Вітаємо, {{recipient_name}}! Ваша пропозиція від {{organization_name}} готова: {{quote_url}} Разом: {{total_amount}}"},
	"ru":    {"Ваше предложение готово", "Здравствуйте, {{recipient_name}}! Ваше предложение от {{organization_name}} готово: {{quote_url}} Итого: {{total_amount}}"},
	"fr":    {"Votre devis est prêt", "Bonjour {{recipient_name}}, votre devis de {{organization_name}} est prêt : {{quote_url}} Total : {{total_amount}}"},
	"es":    {"Su presupuesto está listo", "Hola {{recipient_name}}, su presupuesto de {{organization_name}} está listo: {{quote_url}} Total: {{total_amount}}"},
	"it":    {"Il suo preventivo è pronto", "Salve {{recipient_name}}, il preventivo di {{organization_name}} è pronto: {{quote_url}} Totale: {{total_amount}}"},
	"zh-CN": {"您的报价已准备好", "您好 {{recipient_name}}，{{organization_name}} 的报价已准备好：{{quote_url}} 总计：{{total_amount}}"},
	"az":    {"Təklifiniz hazırdır", "Salam {{recipient_name}}, {{organization_name}} təklifiniz hazırdır: {{quote_url}} Cəmi: {{total_amount}}"},
	"ar":    {"عرض السعر جاهز", "مرحبًا {{recipient_name}}، عرض السعر من {{organization_name}} جاهز: {{quote_url}} الإجمالي: {{total_amount}}"},
}

func quoteSentTemplates() []DefaultTemplate {
	out := make([]DefaultTemplate, 0, len(quoteSentTexts)*len(QuoteSentChannels))
	for _, lang := range msgtemplate.Locales {
		t, ok := quoteSentTexts[lang]
		if !ok {
			continue
		}
		for _, ch := range QuoteSentChannels {
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
		Code: EventQuoteSent, Module: "leads",
		DefaultChannels: QuoteSentChannels,
		AudienceRoles:   []string{RoleCustomer},
		Placeholders: []msgtemplate.Placeholder{
			ph("recipient_name", "Ayşe Yılmaz", "Jane Doe"),
			ph("organization_name", "Tech Oto", "Tech Auto"),
			ph("quote_url", "https://olexfilms.app/s/AbCdEfGhIj", "https://olexfilms.app/s/AbCdEfGhIj"),
			ph("total_amount", "12.500,00 TRY", "TRY 12,500.00"),
		},
		UserConfigurable: true,
		Templates:        quoteSentTemplates(),
	})
}
