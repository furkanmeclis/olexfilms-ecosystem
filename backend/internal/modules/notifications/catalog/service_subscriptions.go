package catalog

import "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/msgtemplate"

const (
	EventServiceSubscriptionAssigned        = "SERVICE_SUBSCRIPTION_ASSIGNED"
	EventServiceSubscriptionCancelRequested = "SERVICE_SUBSCRIPTION_CANCEL_REQUESTED"
	EventServiceSubscriptionCancelled       = "SERVICE_SUBSCRIPTION_CANCELLED"
	EventServiceSubscriptionCancelRejected  = "SERVICE_SUBSCRIPTION_CANCEL_REJECTED"
	// TEC-308: expiry and the 30 / 7 day reminder.
	EventServiceSubscriptionExpired  = "SERVICE_SUBSCRIPTION_EXPIRED"
	EventServiceSubscriptionExpiring = "SERVICE_SUBSCRIPTION_EXPIRING"
)

var ServiceSubscriptionChannels = []string{ChannelInapp, ChannelEmail}

var serviceSubscriptionEvents = []string{
	EventServiceSubscriptionAssigned,
	EventServiceSubscriptionCancelRequested,
	EventServiceSubscriptionCancelled,
	EventServiceSubscriptionCancelRejected,
	EventServiceSubscriptionExpired,
	EventServiceSubscriptionExpiring,
}

var serviceSubscriptionTexts = map[string]map[string]localizedText{
	EventServiceSubscriptionAssigned: {
		"tr": {"Hizmet aboneliği atandı: {{item_name}}",
			"{{item_name}} hizmeti {{organization_name}} için başlatıldı. Bitiş tarihi: {{ends_on}}."},
		"en": {"Service subscription assigned: {{item_name}}",
			"{{item_name}} was assigned to {{organization_name}}. End date: {{ends_on}}."},
		"bg": {"Възложен абонамент за услуга: {{item_name}}",
			"Услугата {{item_name}} е активирана за {{organization_name}}. Крайна дата: {{ends_on}}."},
		"de": {"Service-Abonnement zugewiesen: {{item_name}}",
			"{{item_name}} wurde für {{organization_name}} aktiviert. Enddatum: {{ends_on}}."},
		"el": {"Ανατέθηκε συνδρομή υπηρεσίας: {{item_name}}",
			"Η υπηρεσία {{item_name}} ενεργοποιήθηκε για {{organization_name}}. Ημερομηνία λήξης: {{ends_on}}."},
		"uk": {"Призначено підписку на послугу: {{item_name}}",
			"Послугу {{item_name}} активовано для {{organization_name}}. Дата завершення: {{ends_on}}."},
		"ru": {"Назначена подписка на услугу: {{item_name}}",
			"Услуга {{item_name}} активирована для {{organization_name}}. Дата окончания: {{ends_on}}."},
		"fr": {"Abonnement de service attribué : {{item_name}}",
			"Le service {{item_name}} a été activé pour {{organization_name}}. Date de fin : {{ends_on}}."},
		"es": {"Suscripción de servicio asignada: {{item_name}}",
			"El servicio {{item_name}} se ha activado para {{organization_name}}. Fecha de finalización: {{ends_on}}."},
		"it": {"Abbonamento al servizio assegnato: {{item_name}}",
			"Il servizio {{item_name}} è stato attivato per {{organization_name}}. Data di scadenza: {{ends_on}}."},
		"zh-CN": {"已分配服务订阅：{{item_name}}",
			"已为 {{organization_name}} 开通 {{item_name}} 服务。结束日期：{{ends_on}}。"},
		"az": {"Xidmət abunəliyi təyin edildi: {{item_name}}",
			"{{item_name}} xidməti {{organization_name}} üçün aktivləşdirildi. Bitmə tarixi: {{ends_on}}."},
		"ar": {"تم تعيين اشتراك خدمة: {{item_name}}",
			"تم تفعيل خدمة {{item_name}} لـ {{organization_name}}. تاريخ الانتهاء: {{ends_on}}."},
		"fa": {"اشتراک خدمت تخصیص یافت: {{item_name}}",
			"خدمت {{item_name}} برای {{organization_name}} فعال شد. تاریخ پایان: {{ends_on}}."},
	},
	EventServiceSubscriptionCancelRequested: {
		"tr": {"Erken iptal talebi: {{item_name}}",
			"{{organization_name}}, {{item_name}} aboneliği için erken iptal talep etti. Gerekçe: {{reason}}"},
		"en": {"Early cancellation requested: {{item_name}}",
			"{{organization_name}} requested early cancellation for {{item_name}}. Reason: {{reason}}"},
		"bg": {"Заявка за предсрочно прекратяване: {{item_name}}",
			"{{organization_name}} поиска предсрочно прекратяване на абонамента {{item_name}}. Причина: {{reason}}"},
		"de": {"Vorzeitige Kündigung beantragt: {{item_name}}",
			"{{organization_name}} hat die vorzeitige Kündigung von {{item_name}} beantragt. Grund: {{reason}}"},
		"el": {"Αίτημα πρόωρης ακύρωσης: {{item_name}}",
			"Ο οργανισμός {{organization_name}} ζήτησε πρόωρη ακύρωση της συνδρομής {{item_name}}. Αιτιολογία: {{reason}}"},
		"uk": {"Запит на дострокове скасування: {{item_name}}",
			"{{organization_name}} надіслав запит на дострокове скасування підписки {{item_name}}. Причина: {{reason}}"},
		"ru": {"Запрос на досрочную отмену: {{item_name}}",
			"{{organization_name}} запросил досрочную отмену подписки {{item_name}}. Причина: {{reason}}"},
		"fr": {"Demande de résiliation anticipée : {{item_name}}",
			"{{organization_name}} a demandé la résiliation anticipée de {{item_name}}. Motif : {{reason}}"},
		"es": {"Solicitud de cancelación anticipada: {{item_name}}",
			"{{organization_name}} ha solicitado la cancelación anticipada de {{item_name}}. Motivo: {{reason}}"},
		"it": {"Richiesta di disdetta anticipata: {{item_name}}",
			"{{organization_name}} ha richiesto la disdetta anticipata di {{item_name}}. Motivo: {{reason}}"},
		"zh-CN": {"提前取消申请：{{item_name}}",
			"{{organization_name}} 申请提前取消 {{item_name}} 订阅。原因：{{reason}}"},
		"az": {"Vaxtından əvvəl ləğv sorğusu: {{item_name}}",
			"{{organization_name}} {{item_name}} abunəliyinin vaxtından əvvəl ləğvini tələb etdi. Səbəb: {{reason}}"},
		"ar": {"طلب إلغاء مبكر: {{item_name}}",
			"طلب {{organization_name}} الإلغاء المبكر لاشتراك {{item_name}}. السبب: {{reason}}"},
		"fa": {"درخواست لغو زودهنگام: {{item_name}}",
			"{{organization_name}} درخواست لغو زودهنگام اشتراک {{item_name}} را داده است. دلیل: {{reason}}"},
	},
	EventServiceSubscriptionCancelled: {
		"tr": {"Abonelik iptal edildi: {{item_name}}",
			"{{item_name}} aboneliği iptal edildi. Cayma bedeli: {{cancellation_fee}} {{currency}}."},
		"en": {"Subscription cancelled: {{item_name}}",
			"{{item_name}} was cancelled. Cancellation fee: {{cancellation_fee}} {{currency}}."},
		"bg": {"Абонаментът е прекратен: {{item_name}}",
			"Абонаментът {{item_name}} е прекратен. Такса за прекратяване: {{cancellation_fee}} {{currency}}."},
		"de": {"Abonnement gekündigt: {{item_name}}",
			"{{item_name}} wurde gekündigt. Kündigungsgebühr: {{cancellation_fee}} {{currency}}."},
		"el": {"Η συνδρομή ακυρώθηκε: {{item_name}}",
			"Η συνδρομή {{item_name}} ακυρώθηκε. Τέλος ακύρωσης: {{cancellation_fee}} {{currency}}."},
		"uk": {"Підписку скасовано: {{item_name}}",
			"Підписку {{item_name}} скасовано. Плата за скасування: {{cancellation_fee}} {{currency}}."},
		"ru": {"Подписка отменена: {{item_name}}",
			"Подписка {{item_name}} отменена. Плата за отмену: {{cancellation_fee}} {{currency}}."},
		"fr": {"Abonnement résilié : {{item_name}}",
			"{{item_name}} a été résilié. Frais de résiliation : {{cancellation_fee}} {{currency}}."},
		"es": {"Suscripción cancelada: {{item_name}}",
			"{{item_name}} se ha cancelado. Penalización por cancelación: {{cancellation_fee}} {{currency}}."},
		"it": {"Abbonamento disdetto: {{item_name}}",
			"{{item_name}} è stato disdetto. Penale di recesso: {{cancellation_fee}} {{currency}}."},
		"zh-CN": {"订阅已取消：{{item_name}}",
			"{{item_name}} 订阅已取消。取消费用：{{cancellation_fee}} {{currency}}。"},
		"az": {"Abunəlik ləğv edildi: {{item_name}}",
			"{{item_name}} abunəliyi ləğv edildi. Ləğv haqqı: {{cancellation_fee}} {{currency}}."},
		"ar": {"تم إلغاء الاشتراك: {{item_name}}",
			"تم إلغاء اشتراك {{item_name}}. رسوم الإلغاء: {{cancellation_fee}} {{currency}}."},
		"fa": {"اشتراک لغو شد: {{item_name}}",
			"اشتراک {{item_name}} لغو شد. جریمه لغو: {{cancellation_fee}} {{currency}}."},
	},
	EventServiceSubscriptionCancelRejected: {
		"tr": {"İptal talebi reddedildi: {{item_name}}",
			"{{item_name}} aboneliği için erken iptal talebi reddedildi."},
		"en": {"Cancellation request rejected: {{item_name}}",
			"The early cancellation request for {{item_name}} was rejected."},
		"bg": {"Заявката за прекратяване е отхвърлена: {{item_name}}",
			"Заявката за предсрочно прекратяване на абонамента {{item_name}} е отхвърлена."},
		"de": {"Kündigungsantrag abgelehnt: {{item_name}}",
			"Der Antrag auf vorzeitige Kündigung von {{item_name}} wurde abgelehnt."},
		"el": {"Το αίτημα ακύρωσης απορρίφθηκε: {{item_name}}",
			"Το αίτημα πρόωρης ακύρωσης της συνδρομής {{item_name}} απορρίφθηκε."},
		"uk": {"Запит на скасування відхилено: {{item_name}}",
			"Запит на дострокове скасування підписки {{item_name}} відхилено."},
		"ru": {"Запрос на отмену отклонён: {{item_name}}",
			"Запрос на досрочную отмену подписки {{item_name}} отклонён."},
		"fr": {"Demande de résiliation refusée : {{item_name}}",
			"La demande de résiliation anticipée de {{item_name}} a été refusée."},
		"es": {"Solicitud de cancelación rechazada: {{item_name}}",
			"Se ha rechazado la solicitud de cancelación anticipada de {{item_name}}."},
		"it": {"Richiesta di disdetta respinta: {{item_name}}",
			"La richiesta di disdetta anticipata di {{item_name}} è stata respinta."},
		"zh-CN": {"取消申请已被拒绝：{{item_name}}",
			"{{item_name}} 订阅的提前取消申请已被拒绝。"},
		"az": {"Ləğv sorğusu rədd edildi: {{item_name}}",
			"{{item_name}} abunəliyinin vaxtından əvvəl ləğv sorğusu rədd edildi."},
		"ar": {"تم رفض طلب الإلغاء: {{item_name}}",
			"تم رفض طلب الإلغاء المبكر لاشتراك {{item_name}}."},
		"fa": {"درخواست لغو رد شد: {{item_name}}",
			"درخواست لغو زودهنگام اشتراک {{item_name}} رد شد."},
	},
	EventServiceSubscriptionExpired: {
		"tr": {"Abonelik sona erdi: {{item_name}}",
			"{{organization_name}} için {{item_name}} aboneliği {{ends_on}} tarihinde sona erdi."},
		"en": {"Subscription ended: {{item_name}}",
			"The {{item_name}} subscription of {{organization_name}} ended on {{ends_on}}."},
		"bg": {"Абонаментът изтече: {{item_name}}",
			"Абонаментът {{item_name}} на {{organization_name}} изтече на {{ends_on}}."},
		"de": {"Abonnement beendet: {{item_name}}",
			"Das Abonnement {{item_name}} von {{organization_name}} ist am {{ends_on}} abgelaufen."},
		"el": {"Η συνδρομή έληξε: {{item_name}}",
			"Η συνδρομή {{item_name}} του οργανισμού {{organization_name}} έληξε στις {{ends_on}}."},
		"uk": {"Підписка завершилася: {{item_name}}",
			"Підписка {{item_name}} для {{organization_name}} завершилася {{ends_on}}."},
		"ru": {"Подписка завершилась: {{item_name}}",
			"Подписка {{item_name}} для {{organization_name}} завершилась {{ends_on}}."},
		"fr": {"Abonnement terminé : {{item_name}}",
			"L'abonnement {{item_name}} de {{organization_name}} a pris fin le {{ends_on}}."},
		"es": {"Suscripción finalizada: {{item_name}}",
			"La suscripción {{item_name}} de {{organization_name}} finalizó el {{ends_on}}."},
		"it": {"Abbonamento scaduto: {{item_name}}",
			"L'abbonamento {{item_name}} di {{organization_name}} è scaduto il {{ends_on}}."},
		"zh-CN": {"订阅已结束：{{item_name}}",
			"{{organization_name}} 的 {{item_name}} 订阅已于 {{ends_on}} 结束。"},
		"az": {"Abunəlik başa çatdı: {{item_name}}",
			"{{organization_name}} üçün {{item_name}} abunəliyi {{ends_on}} tarixində başa çatdı."},
		"ar": {"انتهى الاشتراك: {{item_name}}",
			"انتهى اشتراك {{item_name}} الخاص بـ {{organization_name}} في {{ends_on}}."},
		"fa": {"اشتراک به پایان رسید: {{item_name}}",
			"اشتراک {{item_name}} مربوط به {{organization_name}} در تاریخ {{ends_on}} به پایان رسید."},
	},
	EventServiceSubscriptionExpiring: {
		"tr": {"Abonelik {{days_before}} gün içinde bitiyor: {{item_name}}",
			"{{organization_name}} için {{item_name}} aboneliği {{ends_on}} tarihinde sona erecek."},
		"en": {"Subscription ends within {{days_before}} days: {{item_name}}",
			"The {{item_name}} subscription of {{organization_name}} ends on {{ends_on}}."},
		"bg": {"Абонаментът изтича до {{days_before}} дни: {{item_name}}",
			"Абонаментът {{item_name}} на {{organization_name}} изтича на {{ends_on}}."},
		"de": {"Abonnement endet in {{days_before}} Tagen: {{item_name}}",
			"Das Abonnement {{item_name}} von {{organization_name}} endet am {{ends_on}}."},
		"el": {"Η συνδρομή λήγει σε {{days_before}} ημέρες: {{item_name}}",
			"Η συνδρομή {{item_name}} του οργανισμού {{organization_name}} λήγει στις {{ends_on}}."},
		"uk": {"Підписка завершується протягом {{days_before}} дн.: {{item_name}}",
			"Підписка {{item_name}} для {{organization_name}} завершується {{ends_on}}."},
		"ru": {"Подписка заканчивается в течение {{days_before}} дн.: {{item_name}}",
			"Подписка {{item_name}} для {{organization_name}} заканчивается {{ends_on}}."},
		"fr": {"L'abonnement prend fin dans {{days_before}} jours : {{item_name}}",
			"L'abonnement {{item_name}} de {{organization_name}} prend fin le {{ends_on}}."},
		"es": {"La suscripción finaliza en {{days_before}} días: {{item_name}}",
			"La suscripción {{item_name}} de {{organization_name}} finaliza el {{ends_on}}."},
		"it": {"L'abbonamento scade entro {{days_before}} giorni: {{item_name}}",
			"L'abbonamento {{item_name}} di {{organization_name}} scade il {{ends_on}}."},
		"zh-CN": {"订阅将在 {{days_before}} 天内到期：{{item_name}}",
			"{{organization_name}} 的 {{item_name}} 订阅将于 {{ends_on}} 到期。"},
		"az": {"Abunəlik {{days_before}} gün ərzində bitir: {{item_name}}",
			"{{organization_name}} üçün {{item_name}} abunəliyi {{ends_on}} tarixində başa çatacaq."},
		"ar": {"ينتهي الاشتراك خلال {{days_before}} يومًا: {{item_name}}",
			"ينتهي اشتراك {{item_name}} الخاص بـ {{organization_name}} في {{ends_on}}."},
		"fa": {"اشتراک ظرف {{days_before}} روز به پایان می\u200cرسد: {{item_name}}",
			"اشتراک {{item_name}} مربوط به {{organization_name}} در تاریخ {{ends_on}} به پایان می\u200cرسد."},
	},
}

func serviceSubscriptionTemplates(code string) []DefaultTemplate {
	texts := completeServiceSubscriptionTexts(serviceSubscriptionTexts[code])
	out := make([]DefaultTemplate, 0, len(texts)*len(ServiceSubscriptionChannels))
	for _, lang := range msgtemplate.Locales {
		t, ok := texts[lang]
		if !ok {
			continue
		}
		for _, ch := range ServiceSubscriptionChannels {
			out = append(out, DefaultTemplate{
				Role: RoleGeneric, Channel: ch, Language: lang,
				Subject: t.subject, Body: t.body, Format: "text",
			})
		}
	}
	return out
}

func completeServiceSubscriptionTexts(base map[string]localizedText) map[string]localizedText {
	out := make(map[string]localizedText, len(msgtemplate.Locales))
	en := base["en"]
	for _, lang := range msgtemplate.Locales {
		out[lang] = en
	}
	for lang, text := range base {
		out[lang] = text
	}
	return out
}

func serviceSubscriptionPlaceholders() []msgtemplate.Placeholder {
	return []msgtemplate.Placeholder{
		ph("item_name", "Eğitim paketi", "Training package"),
		ph("organization_name", "Tech Oto", "Tech Oto"),
		ph("ends_on", "2027-10-04", "2027-10-04"),
		ph("reason", "Kullanılmıyor", "No longer used"),
		ph("cancellation_fee", "100.00", "100.00"),
		ph("currency", "EUR", "EUR"),
		ph("days_before", "30", "30"),
	}
}

func init() {
	for _, code := range serviceSubscriptionEvents {
		Register(Event{
			Code: code, Module: "service_subscriptions",
			DefaultChannels:  ServiceSubscriptionChannels,
			AudienceRoles:    []string{RoleDealer, RoleDistributor, RoleCenter},
			Placeholders:     serviceSubscriptionPlaceholders(),
			UserConfigurable: true,
			Templates:        serviceSubscriptionTemplates(code),
		})
	}
}
