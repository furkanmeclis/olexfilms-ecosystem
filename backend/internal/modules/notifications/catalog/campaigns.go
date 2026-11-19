package catalog

import "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/msgtemplate"

// Campaign approval notifications (TEC-406, F4-04c). A submitted campaign
// reaches the approver organization's members holding campaigns.approve;
// the decision (approved, rejected, changes requested) reaches the creator.
// Both sides are panel users, so the defaults are in-app and e-mail. Default
// templates ship in all 15 locales and are seeded with
// InsertNotificationTemplateIfMissing by SyncCatalog (no migration).
const (
	EventCampaignApprovalRequested = "CAMPAIGN_APPROVAL_REQUESTED"
	EventCampaignApproved          = "CAMPAIGN_APPROVED"
	EventCampaignRejected          = "CAMPAIGN_REJECTED"
	EventCampaignChangesRequested  = "CAMPAIGN_CHANGES_REQUESTED"
)

// CampaignChannels are the default channels of the campaign events.
var CampaignChannels = []string{ChannelInapp, ChannelEmail}

// CampaignEvents lists the campaign catalog events.
var CampaignEvents = []string{
	EventCampaignApprovalRequested, EventCampaignApproved, EventCampaignRejected, EventCampaignChangesRequested,
}

var campaignTexts = map[string]map[string]localizedText{
	EventCampaignApprovalRequested: {
		"tr": {"Onay bekleyen kampanya: {{campaign_name}}",
			"{{organization_name}} \"{{campaign_name}}\" kampanyasını onayınıza sundu. Lütfen panelden inceleyip karar verin."},
		"en": {"Campaign awaiting approval: {{campaign_name}}",
			"{{organization_name}} submitted the campaign \"{{campaign_name}}\" for your approval. Please review it in the panel and decide."},
		"bg": {"Кампания, очакваща одобрение: {{campaign_name}}",
			"{{organization_name}} изпрати кампанията \"{{campaign_name}}\" за вашето одобрение. Моля, прегледайте я в панела и вземете решение."},
		"de": {"Kampagne wartet auf Freigabe: {{campaign_name}}",
			"{{organization_name}} hat die Kampagne \"{{campaign_name}}\" zur Freigabe eingereicht. Bitte prüfen Sie sie im Panel und entscheiden Sie."},
		"el": {"Καμπάνια σε αναμονή έγκρισης: {{campaign_name}}",
			"Ο οργανισμός {{organization_name}} υπέβαλε την καμπάνια \"{{campaign_name}}\" για έγκριση. Ελέγξτε την στον πίνακα και αποφασίστε."},
		"uk": {"Кампанія очікує схвалення: {{campaign_name}}",
			"{{organization_name}} надіслала кампанію \"{{campaign_name}}\" на ваше схвалення. Перегляньте її в панелі та ухваліть рішення."},
		"ru": {"Кампания ожидает одобрения: {{campaign_name}}",
			"{{organization_name}} отправила кампанию \"{{campaign_name}}\" на ваше одобрение. Просмотрите её в панели и примите решение."},
		"fr": {"Campagne en attente d'approbation : {{campaign_name}}",
			"{{organization_name}} a soumis la campagne \"{{campaign_name}}\" à votre approbation. Veuillez l'examiner dans le panneau et décider."},
		"es": {"Campaña pendiente de aprobación: {{campaign_name}}",
			"{{organization_name}} envió la campaña \"{{campaign_name}}\" para su aprobación. Revísela en el panel y decida."},
		"it": {"Campagna in attesa di approvazione: {{campaign_name}}",
			"{{organization_name}} ha inviato la campagna \"{{campaign_name}}\" per la sua approvazione. La esamini nel pannello e decida."},
		"zh-CN": {"待审批的营销活动：{{campaign_name}}",
			"{{organization_name}} 已提交营销活动“{{campaign_name}}”等待您审批。请在面板中查看并做出决定。"},
		"az": {"Təsdiq gözləyən kampaniya: {{campaign_name}}",
			"{{organization_name}} \"{{campaign_name}}\" kampaniyasını təsdiqinizə təqdim etdi. Zəhmət olmasa paneldə nəzərdən keçirib qərar verin."},
		"ar": {"حملة بانتظار الموافقة: {{campaign_name}}",
			"قدّمت {{organization_name}} الحملة \"{{campaign_name}}\" للحصول على موافقتك. يرجى مراجعتها في اللوحة واتخاذ القرار."},
		"fa": {"کمپین در انتظار تأیید: {{campaign_name}}",
			"{{organization_name}} کمپین «{{campaign_name}}» را برای تأیید شما ارسال کرده است. لطفاً آن را در پنل بررسی کنید و تصمیم بگیرید."},
		"hi": {"अनुमोदन की प्रतीक्षा में अभियान: {{campaign_name}}",
			"{{organization_name}} ने अभियान \"{{campaign_name}}\" आपके अनुमोदन के लिए भेजा है। कृपया पैनल में इसकी समीक्षा करें और निर्णय लें।"},
	},
	EventCampaignApproved: {
		"tr": {"Kampanyanız onaylandı: {{campaign_name}}",
			"\"{{campaign_name}}\" kampanyanız onaylandı. Artık panelden zamanlayabilir veya hemen gönderebilirsiniz."},
		"en": {"Your campaign was approved: {{campaign_name}}",
			"Your campaign \"{{campaign_name}}\" was approved. You can now schedule it or send it right away from the panel."},
		"bg": {"Кампанията ви е одобрена: {{campaign_name}}",
			"Кампанията ви \"{{campaign_name}}\" е одобрена. Вече можете да я насрочите или изпратите веднага от панела."},
		"de": {"Ihre Kampagne wurde freigegeben: {{campaign_name}}",
			"Ihre Kampagne \"{{campaign_name}}\" wurde freigegeben. Sie können sie jetzt im Panel planen oder sofort versenden."},
		"el": {"Η καμπάνια σας εγκρίθηκε: {{campaign_name}}",
			"Η καμπάνια σας \"{{campaign_name}}\" εγκρίθηκε. Μπορείτε τώρα να την προγραμματίσετε ή να την στείλετε αμέσως από τον πίνακα."},
		"uk": {"Вашу кампанію схвалено: {{campaign_name}}",
			"Вашу кампанію \"{{campaign_name}}\" схвалено. Тепер ви можете запланувати її або надіслати одразу з панелі."},
		"ru": {"Ваша кампания одобрена: {{campaign_name}}",
			"Ваша кампания \"{{campaign_name}}\" одобрена. Теперь вы можете запланировать её или сразу отправить из панели."},
		"fr": {"Votre campagne a été approuvée : {{campaign_name}}",
			"Votre campagne \"{{campaign_name}}\" a été approuvée. Vous pouvez maintenant la programmer ou l'envoyer immédiatement depuis le panneau."},
		"es": {"Su campaña fue aprobada: {{campaign_name}}",
			"Su campaña \"{{campaign_name}}\" fue aprobada. Ahora puede programarla o enviarla de inmediato desde el panel."},
		"it": {"La sua campagna è stata approvata: {{campaign_name}}",
			"La sua campagna \"{{campaign_name}}\" è stata approvata. Ora può programmarla o inviarla subito dal pannello."},
		"zh-CN": {"您的营销活动已获批准：{{campaign_name}}",
			"您的营销活动“{{campaign_name}}”已获批准。现在可以在面板中安排发送时间或立即发送。"},
		"az": {"Kampaniyanız təsdiqləndi: {{campaign_name}}",
			"\"{{campaign_name}}\" kampaniyanız təsdiqləndi. İndi onu paneldən planlaşdıra və ya dərhal göndərə bilərsiniz."},
		"ar": {"تمت الموافقة على حملتك: {{campaign_name}}",
			"تمت الموافقة على حملتك \"{{campaign_name}}\". يمكنك الآن جدولتها أو إرسالها فورًا من اللوحة."},
		"fa": {"کمپین شما تأیید شد: {{campaign_name}}",
			"کمپین «{{campaign_name}}» شما تأیید شد. اکنون می\u200cتوانید آن را از پنل زمان\u200cبندی کنید یا بی\u200cدرنگ ارسال کنید."},
		"hi": {"आपका अभियान स्वीकृत हुआ: {{campaign_name}}",
			"आपका अभियान \"{{campaign_name}}\" स्वीकृत हो गया है। अब आप पैनल से इसे शेड्यूल कर सकते हैं या तुरंत भेज सकते हैं।"},
	},
	EventCampaignRejected: {
		"tr": {"Kampanyanız reddedildi: {{campaign_name}}",
			"\"{{campaign_name}}\" kampanyanız reddedildi. Sebep: {{reason}}"},
		"en": {"Your campaign was rejected: {{campaign_name}}",
			"Your campaign \"{{campaign_name}}\" was rejected. Reason: {{reason}}"},
		"bg": {"Кампанията ви е отхвърлена: {{campaign_name}}",
			"Кампанията ви \"{{campaign_name}}\" е отхвърлена. Причина: {{reason}}"},
		"de": {"Ihre Kampagne wurde abgelehnt: {{campaign_name}}",
			"Ihre Kampagne \"{{campaign_name}}\" wurde abgelehnt. Begründung: {{reason}}"},
		"el": {"Η καμπάνια σας απορρίφθηκε: {{campaign_name}}",
			"Η καμπάνια σας \"{{campaign_name}}\" απορρίφθηκε. Αιτία: {{reason}}"},
		"uk": {"Вашу кампанію відхилено: {{campaign_name}}",
			"Вашу кампанію \"{{campaign_name}}\" відхилено. Причина: {{reason}}"},
		"ru": {"Ваша кампания отклонена: {{campaign_name}}",
			"Ваша кампания \"{{campaign_name}}\" отклонена. Причина: {{reason}}"},
		"fr": {"Votre campagne a été refusée : {{campaign_name}}",
			"Votre campagne \"{{campaign_name}}\" a été refusée. Motif : {{reason}}"},
		"es": {"Su campaña fue rechazada: {{campaign_name}}",
			"Su campaña \"{{campaign_name}}\" fue rechazada. Motivo: {{reason}}"},
		"it": {"La sua campagna è stata respinta: {{campaign_name}}",
			"La sua campagna \"{{campaign_name}}\" è stata respinta. Motivo: {{reason}}"},
		"zh-CN": {"您的营销活动已被拒绝：{{campaign_name}}",
			"您的营销活动“{{campaign_name}}”已被拒绝。原因：{{reason}}"},
		"az": {"Kampaniyanız rədd edildi: {{campaign_name}}",
			"\"{{campaign_name}}\" kampaniyanız rədd edildi. Səbəb: {{reason}}"},
		"ar": {"تم رفض حملتك: {{campaign_name}}",
			"تم رفض حملتك \"{{campaign_name}}\". السبب: {{reason}}"},
		"fa": {"کمپین شما رد شد: {{campaign_name}}",
			"کمپین «{{campaign_name}}» شما رد شد. دلیل: {{reason}}"},
		"hi": {"आपका अभियान अस्वीकृत हुआ: {{campaign_name}}",
			"आपका अभियान \"{{campaign_name}}\" अस्वीकृत कर दिया गया। कारण: {{reason}}"},
	},
	EventCampaignChangesRequested: {
		"tr": {"Kampanyanız için değişiklik istendi: {{campaign_name}}",
			"\"{{campaign_name}}\" kampanyanız taslağa geri gönderildi. İstenen değişiklik: {{reason}}. Düzenleyip yeniden onaya sunabilirsiniz."},
		"en": {"Changes requested for your campaign: {{campaign_name}}",
			"Your campaign \"{{campaign_name}}\" was sent back to draft. Requested change: {{reason}}. You can edit it and submit it again."},
		"bg": {"Поискани са промени в кампанията ви: {{campaign_name}}",
			"Кампанията ви \"{{campaign_name}}\" е върната в чернова. Поискана промяна: {{reason}}. Можете да я редактирате и изпратите отново."},
		"de": {"Änderungen an Ihrer Kampagne angefordert: {{campaign_name}}",
			"Ihre Kampagne \"{{campaign_name}}\" wurde in den Entwurf zurückgesetzt. Gewünschte Änderung: {{reason}}. Sie können sie bearbeiten und erneut einreichen."},
		"el": {"Ζητήθηκαν αλλαγές στην καμπάνια σας: {{campaign_name}}",
			"Η καμπάνια σας \"{{campaign_name}}\" επέστρεψε σε πρόχειρο. Ζητούμενη αλλαγή: {{reason}}. Μπορείτε να την επεξεργαστείτε και να την υποβάλετε ξανά."},
		"uk": {"Для вашої кампанії запитано зміни: {{campaign_name}}",
			"Вашу кампанію \"{{campaign_name}}\" повернуто в чернетку. Запитана зміна: {{reason}}. Ви можете відредагувати її та надіслати знову."},
		"ru": {"Для вашей кампании запрошены изменения: {{campaign_name}}",
			"Ваша кампания \"{{campaign_name}}\" возвращена в черновик. Запрошенное изменение: {{reason}}. Вы можете отредактировать её и отправить снова."},
		"fr": {"Modifications demandées pour votre campagne : {{campaign_name}}",
			"Votre campagne \"{{campaign_name}}\" a été renvoyée en brouillon. Modification demandée : {{reason}}. Vous pouvez la modifier et la soumettre à nouveau."},
		"es": {"Se solicitaron cambios en su campaña: {{campaign_name}}",
			"Su campaña \"{{campaign_name}}\" volvió a borrador. Cambio solicitado: {{reason}}. Puede editarla y enviarla de nuevo."},
		"it": {"Modifiche richieste per la sua campagna: {{campaign_name}}",
			"La sua campagna \"{{campaign_name}}\" è tornata in bozza. Modifica richiesta: {{reason}}. Può modificarla e inviarla di nuovo."},
		"zh-CN": {"您的营销活动需要修改：{{campaign_name}}",
			"您的营销活动“{{campaign_name}}”已退回草稿。要求的修改：{{reason}}。您可以编辑后重新提交审批。"},
		"az": {"Kampaniyanız üçün dəyişiklik istənildi: {{campaign_name}}",
			"\"{{campaign_name}}\" kampaniyanız qaralamaya qaytarıldı. İstənilən dəyişiklik: {{reason}}. Onu redaktə edib yenidən təqdim edə bilərsiniz."},
		"ar": {"طُلبت تعديلات على حملتك: {{campaign_name}}",
			"أُعيدت حملتك \"{{campaign_name}}\" إلى المسودة. التعديل المطلوب: {{reason}}. يمكنك تعديلها وتقديمها مرة أخرى."},
		"fa": {"درخواست اصلاح کمپین شما: {{campaign_name}}",
			"کمپین «{{campaign_name}}» شما به پیش\u200cنویس برگردانده شد. اصلاح درخواستی: {{reason}}. می\u200cتوانید آن را ویرایش و دوباره ارسال کنید."},
		"hi": {"आपके अभियान में बदलाव का अनुरोध: {{campaign_name}}",
			"आपका अभियान \"{{campaign_name}}\" ड्राफ़्ट में वापस भेज दिया गया है। अनुरोधित बदलाव: {{reason}}। आप इसे संपादित करके फिर से भेज सकते हैं।"},
	},
}

func campaignTemplates(code string) []DefaultTemplate {
	texts := campaignTexts[code]
	out := make([]DefaultTemplate, 0, len(texts)*len(CampaignChannels))
	for _, lang := range msgtemplate.Locales {
		t, ok := texts[lang]
		if !ok {
			continue // TestCampaignTemplatesCoverEveryLocale reports it
		}
		for _, ch := range CampaignChannels {
			out = append(out, DefaultTemplate{
				Role: RoleGeneric, Channel: ch, Language: lang,
				Subject: t.subject, Body: t.body, Format: "text",
			})
		}
	}
	return out
}

func campaignPlaceholders(code string) []msgtemplate.Placeholder {
	out := []msgtemplate.Placeholder{
		ph("campaign_name", "Kış bakım kampanyası", "Winter care campaign"),
		ph("organization_name", "Tech Oto", "Tech Oto"),
	}
	if code == EventCampaignRejected || code == EventCampaignChangesRequested {
		out = append(out, ph("reason", "Görsel eksik", "The image is missing"))
	}
	return out
}

func init() {
	for _, code := range CampaignEvents {
		audience := []string{RoleDealer, RoleDistributor, RoleCenter}
		if code == EventCampaignApprovalRequested {
			audience = []string{RoleDistributor, RoleCenter}
		}
		Register(Event{
			Code: code, Module: "campaigns",
			DefaultChannels:  CampaignChannels,
			AudienceRoles:    audience,
			Placeholders:     campaignPlaceholders(code),
			UserConfigurable: true,
			Templates:        campaignTemplates(code),
		})
	}
}
