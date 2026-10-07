package catalog

import "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/msgtemplate"

// Dealer showcase review notifications (TEC-467, F5-01b). With
// showcase.approval_required on, a submitted showcase reaches the brand
// center's members holding platform.showcase.review; the decision
// (approved and published, or rejected with a note) reaches the owners of
// the dealer. Both sides are panel users, so the defaults are in-app and
// e-mail. Default templates ship in all 13 locales and are seeded with
// InsertNotificationTemplateIfMissing by SyncCatalog (no migration).
const (
	EventShowcaseReviewRequested = "SHOWCASE_REVIEW_REQUESTED"
	EventShowcaseApproved        = "SHOWCASE_APPROVED"
	EventShowcaseRejected        = "SHOWCASE_REJECTED"
)

// ShowcaseChannels are the default channels of the showcase events.
var ShowcaseChannels = []string{ChannelInapp, ChannelEmail}

// ShowcaseEvents lists the showcase catalog events.
var ShowcaseEvents = []string{EventShowcaseReviewRequested, EventShowcaseApproved, EventShowcaseRejected}

var showcaseTexts = map[string]map[string]localizedText{
	EventShowcaseReviewRequested: {
		"tr": {"Onay bekleyen vitrin: {{organization_name}}",
			"{{organization_name}} bayi vitrinini yayın onayınıza sundu. Lütfen panelden inceleyip onaylayın veya reddedin."},
		"en": {"Showcase awaiting review: {{organization_name}}",
			"{{organization_name}} submitted its dealer showcase for your approval. Please review it in the panel and approve or reject it."},
		"bg": {"Витрина, очакваща преглед: {{organization_name}}",
			"{{organization_name}} изпрати своята дилърска витрина за вашето одобрение. Моля, прегледайте я в панела и я одобрете или отхвърлете."},
		"de": {"Schaufenster wartet auf Prüfung: {{organization_name}}",
			"{{organization_name}} hat sein Händler-Schaufenster zur Freigabe eingereicht. Bitte prüfen Sie es im Panel und geben Sie es frei oder lehnen Sie es ab."},
		"el": {"Βιτρίνα σε αναμονή ελέγχου: {{organization_name}}",
			"Ο οργανισμός {{organization_name}} υπέβαλε τη βιτρίνα αντιπροσώπου για έγκριση. Ελέγξτε την στον πίνακα και εγκρίνετε ή απορρίψτε την."},
		"uk": {"Вітрина очікує перевірки: {{organization_name}}",
			"{{organization_name}} надіслала свою дилерську вітрину на ваше схвалення. Перегляньте її в панелі та схваліть або відхиліть."},
		"ru": {"Витрина ожидает проверки: {{organization_name}}",
			"{{organization_name}} отправила свою дилерскую витрину на ваше одобрение. Просмотрите её в панели и одобрите или отклоните."},
		"fr": {"Vitrine en attente de validation : {{organization_name}}",
			"{{organization_name}} a soumis sa vitrine de revendeur à votre approbation. Veuillez l'examiner dans le panneau puis l'approuver ou la refuser."},
		"es": {"Escaparate pendiente de revisión: {{organization_name}}",
			"{{organization_name}} envió su escaparate de distribuidor para su aprobación. Revíselo en el panel y apruébelo o rechácelo."},
		"it": {"Vetrina in attesa di revisione: {{organization_name}}",
			"{{organization_name}} ha inviato la sua vetrina rivenditore per la sua approvazione. La esamini nel pannello e la approvi o la rifiuti."},
		"zh-CN": {"待审核的经销商展示页：{{organization_name}}",
			"{{organization_name}} 已提交其经销商展示页等待您审批。请在面板中查看并批准或拒绝。"},
		"az": {"Yoxlama gözləyən vitrin: {{organization_name}}",
			"{{organization_name}} diler vitrinini təsdiqinizə təqdim etdi. Zəhmət olmasa paneldə nəzərdən keçirib təsdiqləyin və ya rədd edin."},
		"ar": {"واجهة عرض بانتظار المراجعة: {{organization_name}}",
			"قدّمت {{organization_name}} واجهة عرض الوكيل للحصول على موافقتك. يرجى مراجعتها في اللوحة والموافقة عليها أو رفضها."},
	},
	EventShowcaseApproved: {
		"tr": {"Vitrininiz onaylandı ve yayında",
			"{{organization_name}} vitrini merkez tarafından onaylandı ve artık herkese açık bayi sayfanızda yayında."},
		"en": {"Your showcase was approved and is live",
			"The {{organization_name}} showcase was approved by the center and is now live on your public dealer page."},
		"bg": {"Витрината ви е одобрена и публикувана",
			"Витрината на {{organization_name}} беше одобрена от центъра и вече е публикувана на публичната ви дилърска страница."},
		"de": {"Ihr Schaufenster wurde freigegeben und ist online",
			"Das Schaufenster von {{organization_name}} wurde von der Zentrale freigegeben und ist jetzt auf Ihrer öffentlichen Händlerseite sichtbar."},
		"el": {"Η βιτρίνα σας εγκρίθηκε και δημοσιεύτηκε",
			"Η βιτρίνα του {{organization_name}} εγκρίθηκε από το κέντρο και εμφανίζεται πλέον στη δημόσια σελίδα αντιπροσώπου σας."},
		"uk": {"Вашу вітрину схвалено й опубліковано",
			"Вітрину {{organization_name}} схвалив центр, і тепер вона опублікована на вашій публічній сторінці дилера."},
		"ru": {"Ваша витрина одобрена и опубликована",
			"Витрина {{organization_name}} одобрена центром и теперь опубликована на вашей публичной странице дилера."},
		"fr": {"Votre vitrine a été approuvée et est en ligne",
			"La vitrine de {{organization_name}} a été approuvée par le siège et est désormais visible sur votre page revendeur publique."},
		"es": {"Su escaparate fue aprobado y está publicado",
			"El escaparate de {{organization_name}} fue aprobado por la central y ya está publicado en su página pública de distribuidor."},
		"it": {"La sua vetrina è stata approvata ed è online",
			"La vetrina di {{organization_name}} è stata approvata dalla sede ed è ora visibile sulla sua pagina pubblica del rivenditore."},
		"zh-CN": {"您的展示页已通过审核并上线",
			"{{organization_name}} 的展示页已由总部批准，现已在您的公开经销商页面上线。"},
		"az": {"Vitrininiz təsdiqləndi və yayımdadır",
			"{{organization_name}} vitrini mərkəz tərəfindən təsdiqləndi və artıq açıq diler səhifənizdə yayımdadır."},
		"ar": {"تمت الموافقة على واجهة العرض ونشرها",
			"وافق المركز على واجهة عرض {{organization_name}} وأصبحت منشورة الآن على صفحة الوكيل العامة."},
	},
	EventShowcaseRejected: {
		"tr": {"Vitrininiz reddedildi",
			"{{organization_name}} vitrini merkez tarafından reddedildi. Not: {{reason}}. Düzenleyip yeniden onaya gönderebilirsiniz."},
		"en": {"Your showcase was rejected",
			"The {{organization_name}} showcase was rejected by the center. Note: {{reason}}. You can edit it and submit it again."},
		"bg": {"Витрината ви беше отхвърлена",
			"Витрината на {{organization_name}} беше отхвърлена от центъра. Бележка: {{reason}}. Можете да я редактирате и да я изпратите отново."},
		"de": {"Ihr Schaufenster wurde abgelehnt",
			"Das Schaufenster von {{organization_name}} wurde von der Zentrale abgelehnt. Hinweis: {{reason}}. Sie können es bearbeiten und erneut einreichen."},
		"el": {"Η βιτρίνα σας απορρίφθηκε",
			"Η βιτρίνα του {{organization_name}} απορρίφθηκε από το κέντρο. Σημείωση: {{reason}}. Μπορείτε να την επεξεργαστείτε και να την υποβάλετε ξανά."},
		"uk": {"Вашу вітрину відхилено",
			"Центр відхилив вітрину {{organization_name}}. Примітка: {{reason}}. Ви можете відредагувати її та надіслати повторно."},
		"ru": {"Ваша витрина отклонена",
			"Центр отклонил витрину {{organization_name}}. Примечание: {{reason}}. Вы можете отредактировать её и отправить повторно."},
		"fr": {"Votre vitrine a été refusée",
			"La vitrine de {{organization_name}} a été refusée par le siège. Remarque : {{reason}}. Vous pouvez la modifier et la soumettre à nouveau."},
		"es": {"Su escaparate fue rechazado",
			"La central rechazó el escaparate de {{organization_name}}. Nota: {{reason}}. Puede editarlo y enviarlo de nuevo."},
		"it": {"La sua vetrina è stata rifiutata",
			"La vetrina di {{organization_name}} è stata rifiutata dalla sede. Nota: {{reason}}. Può modificarla e inviarla di nuovo."},
		"zh-CN": {"您的展示页未通过审核",
			"{{organization_name}} 的展示页被总部拒绝。备注：{{reason}}。您可以修改后重新提交。"},
		"az": {"Vitrininiz rədd edildi",
			"{{organization_name}} vitrini mərkəz tərəfindən rədd edildi. Qeyd: {{reason}}. Onu redaktə edib yenidən təqdim edə bilərsiniz."},
		"ar": {"تم رفض واجهة العرض",
			"رفض المركز واجهة عرض {{organization_name}}. ملاحظة: {{reason}}. يمكنك تعديلها وتقديمها مرة أخرى."},
	},
}

func showcaseTemplates(code string) []DefaultTemplate {
	texts := showcaseTexts[code]
	out := make([]DefaultTemplate, 0, len(texts)*len(ShowcaseChannels))
	for _, lang := range msgtemplate.Locales {
		t, ok := texts[lang]
		if !ok {
			continue // TestShowcaseTemplatesCoverEveryLocale reports it
		}
		for _, ch := range ShowcaseChannels {
			out = append(out, DefaultTemplate{
				Role: RoleGeneric, Channel: ch, Language: lang,
				Subject: t.subject, Body: t.body, Format: "text",
			})
		}
	}
	return out
}

func showcasePlaceholders(code string) []msgtemplate.Placeholder {
	out := []msgtemplate.Placeholder{ph("organization_name", "Tech Oto", "Tech Oto")}
	if code == EventShowcaseRejected {
		out = append(out, ph("reason", "Kapak fotoğrafı bulanık", "The cover photo is blurry"))
	}
	return out
}

func init() {
	for _, code := range ShowcaseEvents {
		audience := []string{RoleDealer, RoleDistributor}
		if code == EventShowcaseReviewRequested {
			audience = []string{RoleCenter}
		}
		Register(Event{
			Code: code, Module: "dealer_showcase",
			DefaultChannels:  ShowcaseChannels,
			AudienceRoles:    audience,
			Placeholders:     showcasePlaceholders(code),
			UserConfigurable: true,
			Templates:        showcaseTemplates(code),
		})
	}
}
