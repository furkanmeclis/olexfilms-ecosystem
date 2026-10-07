package catalog

import "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/msgtemplate"

// AI quota threshold notification (TEC-389, F4-01g). The usage booking
// writes ai.quota.threshold when a pool crosses 80 % and 100 % of its
// monthly quota; the event id is fixed per organization, pool, month and
// threshold, so each threshold notifies once a month. Recipients: the
// organization's ai.usage.read holders (org pool) or the platform admins
// (the brand center's system pool). Panel users only, so in-app and e-mail.
// Templates ship in all 13 locales and are seeded by SyncCatalog with
// InsertNotificationTemplateIfMissing (no migration); only admins edit them.
const EventAIQuotaThreshold = "ai.quota.threshold"

// AIQuotaChannels are the default channels of the quota event.
var AIQuotaChannels = []string{ChannelInapp, ChannelEmail}

var aiQuotaTexts = map[string]localizedText{
	"tr": {"AI kotasının %{{threshold}} kadarı kullanıldı",
		"{{organization_name}}, {{period}} dönemi AI token kotasının %{{threshold}} kadarını kullandı ({{used_tokens}} / {{quota_tokens}} token). Kota dolduğunda asistan bir sonraki aya kadar yanıt vermez."},
	"en": {"{{threshold}}% of the AI quota used",
		"{{organization_name}} has used {{threshold}}% of its AI token quota for {{period}} ({{used_tokens}} / {{quota_tokens}} tokens). Once the quota is used up, the assistant stops answering until next month."},
	"bg": {"Използвани са {{threshold}}% от квотата за ИИ",
		"{{organization_name}} използва {{threshold}}% от квотата си за ИИ токени за {{period}} ({{used_tokens}} / {{quota_tokens}} токена). Когато квотата се изчерпи, асистентът спира да отговаря до следващия месец."},
	"de": {"{{threshold}} % des KI-Kontingents verbraucht",
		"{{organization_name}} hat {{threshold}} % des KI-Token-Kontingents für {{period}} verbraucht ({{used_tokens}} / {{quota_tokens}} Token). Ist das Kontingent aufgebraucht, antwortet der Assistent erst im nächsten Monat wieder."},
	"el": {"Χρησιμοποιήθηκε το {{threshold}}% του ορίου AI",
		"Ο οργανισμός {{organization_name}} χρησιμοποίησε το {{threshold}}% του ορίου διακριτικών AI για την περίοδο {{period}} ({{used_tokens}} / {{quota_tokens}} διακριτικά). Όταν εξαντληθεί το όριο, ο βοηθός σταματά να απαντά μέχρι τον επόμενο μήνα."},
	"uk": {"Використано {{threshold}}% квоти ШІ",
		"{{organization_name}} використала {{threshold}}% квоти токенів ШІ за {{period}} ({{used_tokens}} / {{quota_tokens}} токенів). Коли квоту буде вичерпано, асистент не відповідатиме до наступного місяця."},
	"ru": {"Использовано {{threshold}}% квоты ИИ",
		"{{organization_name}} использовала {{threshold}}% квоты токенов ИИ за {{period}} ({{used_tokens}} / {{quota_tokens}} токенов). Когда квота исчерпана, ассистент не отвечает до следующего месяца."},
	"fr": {"{{threshold}} % du quota IA utilisé",
		"{{organization_name}} a utilisé {{threshold}} % de son quota de jetons IA pour {{period}} ({{used_tokens}} / {{quota_tokens}} jetons). Une fois le quota épuisé, l'assistant ne répond plus jusqu'au mois prochain."},
	"es": {"{{threshold}} % de la cuota de IA utilizada",
		"{{organization_name}} ha utilizado el {{threshold}} % de su cuota de tokens de IA para {{period}} ({{used_tokens}} / {{quota_tokens}} tokens). Cuando se agote la cuota, el asistente dejará de responder hasta el mes siguiente."},
	"it": {"Utilizzato il {{threshold}}% della quota IA",
		"{{organization_name}} ha utilizzato il {{threshold}}% della quota di token IA per {{period}} ({{used_tokens}} / {{quota_tokens}} token). Esaurita la quota, l'assistente non risponde fino al mese successivo."},
	"zh-CN": {"AI 配额已使用 {{threshold}}%",
		"{{organization_name}} 已使用 {{period}} 期间 AI 令牌配额的 {{threshold}}%（{{used_tokens}} / {{quota_tokens}} 个令牌）。配额用尽后，助手将停止回复，直到下个月。"},
	"az": {"Süni intellekt kvotasının {{threshold}}%-i istifadə edildi",
		"{{organization_name}} {{period}} dövrü üçün süni intellekt token kvotasının {{threshold}}%-ni istifadə etdi ({{used_tokens}} / {{quota_tokens}} token). Kvota bitdikdə köməkçi növbəti aya qədər cavab vermir."},
	"ar": {"تم استخدام {{threshold}}% من حصة الذكاء الاصطناعي",
		"استخدمت {{organization_name}} نسبة {{threshold}}% من حصة رموز الذكاء الاصطناعي للفترة {{period}} ({{used_tokens}} / {{quota_tokens}} رمز). عند نفاد الحصة يتوقف المساعد عن الرد حتى الشهر التالي."},
}

func aiQuotaTemplates() []DefaultTemplate {
	out := make([]DefaultTemplate, 0, len(aiQuotaTexts)*len(AIQuotaChannels))
	for _, lang := range msgtemplate.Locales {
		t, ok := aiQuotaTexts[lang]
		if !ok {
			continue // TestAIQuotaTemplatesCoverEveryLocale reports it
		}
		for _, ch := range AIQuotaChannels {
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
		Code: EventAIQuotaThreshold, Module: "ai",
		DefaultChannels: AIQuotaChannels,
		AudienceRoles:   []string{RoleDealer, RoleDistributor, RoleCenter},
		Placeholders: []msgtemplate.Placeholder{
			ph("organization_name", "Tech Oto", "Tech Oto"),
			ph("threshold", "80", "80"),
			ph("used_tokens", "1600000", "1600000"),
			ph("quota_tokens", "2000000", "2000000"),
			ph("period", "2026-10", "2026-10"),
		},
		UserConfigurable: true,
		Templates:        aiQuotaTemplates(),
	})
}
