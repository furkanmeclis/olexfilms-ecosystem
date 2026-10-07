package usecase

import "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"

// emailTexts are the fixed parts of a campaign e-mail.
type emailTexts struct {
	details          string
	unsubscribeIntro string
	unsubscribe      string
}

var campaignEmailTexts = map[i18n.Locale]emailTexts{
	"tr":    {"Ayrıntılar", "Kampanya mesajları almak istemiyorsanız:", "Abonelikten çık"},
	"en":    {"Details", "Don't want campaign messages?", "Unsubscribe"},
	"bg":    {"Подробности", "Не желаете да получавате съобщения за кампании?", "Отписване"},
	"de":    {"Details", "Sie möchten keine Kampagnennachrichten erhalten?", "Abmelden"},
	"el":    {"Λεπτομέρειες", "Δεν θέλετε να λαμβάνετε μηνύματα προωθητικών ενεργειών;", "Απεγγραφή"},
	"uk":    {"Докладніше", "Не бажаєте отримувати повідомлення про акції?", "Відписатися"},
	"ru":    {"Подробнее", "Не хотите получать сообщения об акциях?", "Отписаться"},
	"fr":    {"En savoir plus", "Vous ne souhaitez plus recevoir nos offres ?", "Se désabonner"},
	"es":    {"Más información", "¿No desea recibir mensajes de campañas?", "Darse de baja"},
	"it":    {"Scopri di più", "Non desideri ricevere messaggi promozionali?", "Annulla l'iscrizione"},
	"zh-CN": {"查看详情", "不想再收到活动消息？", "退订"},
	"az":    {"Ətraflı", "Kampaniya mesajları almaq istəmirsiniz?", "Abunəlikdən çıx"},
	"ar":    {"التفاصيل", "لا ترغب في تلقي رسائل الحملات؟", "إلغاء الاشتراك"},
}

func emailTextsFor(locale string) emailTexts {
	if t, ok := campaignEmailTexts[i18n.Normalize(locale)]; ok {
		return t
	}
	return campaignEmailTexts[i18n.DefaultLocale]
}
