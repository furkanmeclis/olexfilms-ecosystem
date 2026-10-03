package catalog

import "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/msgtemplate"

// Stock transfer notifications (TEC-200, F1-04f3, K13). Every transfers.*
// outbox event of a sibling stock transfer request maps to one catalog
// event; the transfer use case resolves the recipients (notify_user_ids:
// members of the notified sides holding the transfer permission, minus the
// actor) and the organization names into the event payload. Default
// templates ship in all 13 locales for in-app and WhatsApp (K16/K21) and are
// seeded with InsertNotificationTemplateIfMissing by SyncCatalog.
const (
	EventTransferRequested = "TRANSFER_REQUESTED"
	EventTransferApproved  = "TRANSFER_APPROVED"
	EventTransferRejected  = "TRANSFER_REJECTED"
	EventTransferShipped   = "TRANSFER_SHIPPED"
	EventTransferReceived  = "TRANSFER_RECEIVED"
	EventTransferCancelled = "TRANSFER_CANCELLED"
)

// TransferChannels are the default channels of the transfer events.
var TransferChannels = []string{ChannelInapp, ChannelWhatsApp}

// TransferEvents lists the transfer catalog events in flow order.
var TransferEvents = []string{
	EventTransferRequested, EventTransferApproved, EventTransferRejected,
	EventTransferShipped, EventTransferReceived, EventTransferCancelled,
}

var transferTexts = map[string]map[string]localizedText{
	EventTransferRequested: {
		"tr": {"Yeni stok transfer talebi {{transfer_no}}",
			"{{sender_name}}, {{receiver_name}} için stok transferi talep etti ({{transfer_no}}). Lütfen panelden inceleyin."},
		"en": {"New stock transfer request {{transfer_no}}",
			"{{sender_name}} requested a stock transfer to {{receiver_name}} ({{transfer_no}}). Please review it in the panel."},
		"bg": {"Нова заявка за прехвърляне на наличност {{transfer_no}}",
			"{{sender_name}} заяви прехвърляне на наличност към {{receiver_name}} ({{transfer_no}}). Моля, прегледайте я в панела."},
		"de": {"Neue Bestandsumbuchungsanfrage {{transfer_no}}",
			"{{sender_name}} hat eine Bestandsumbuchung an {{receiver_name}} angefragt ({{transfer_no}}). Bitte prüfen Sie sie im Panel."},
		"el": {"Νέο αίτημα μεταφοράς αποθέματος {{transfer_no}}",
			"Η {{sender_name}} ζήτησε μεταφορά αποθέματος προς {{receiver_name}} ({{transfer_no}}). Ελέγξτε το στον πίνακα."},
		"uk": {"Новий запит на переміщення запасів {{transfer_no}}",
			"{{sender_name}} запросив переміщення запасів до {{receiver_name}} ({{transfer_no}}). Перегляньте його в панелі."},
		"ru": {"Новый запрос на перемещение запасов {{transfer_no}}",
			"{{sender_name}} запросил перемещение запасов в {{receiver_name}} ({{transfer_no}}). Проверьте его в панели."},
		"fr": {"Nouvelle demande de transfert de stock {{transfer_no}}",
			"{{sender_name}} a demandé un transfert de stock vers {{receiver_name}} ({{transfer_no}}). Veuillez l'examiner dans le panneau."},
		"es": {"Nueva solicitud de transferencia de stock {{transfer_no}}",
			"{{sender_name}} solicitó una transferencia de stock a {{receiver_name}} ({{transfer_no}}). Revísela en el panel."},
		"it": {"Nuova richiesta di trasferimento di stock {{transfer_no}}",
			"{{sender_name}} ha richiesto un trasferimento di stock verso {{receiver_name}} ({{transfer_no}}). Lo verifichi nel pannello."},
		"zh-CN": {"新的库存调拨申请 {{transfer_no}}",
			"{{sender_name}} 申请向 {{receiver_name}} 调拨库存（{{transfer_no}}）。请在面板中审核。"},
		"az": {"Yeni stok transfer sorğusu {{transfer_no}}",
			"{{sender_name}} {{receiver_name}} üçün stok transferi tələb etdi ({{transfer_no}}). Zəhmət olmasa paneldə nəzərdən keçirin."},
		"ar": {"طلب نقل مخزون جديد {{transfer_no}}",
			"طلب {{sender_name}} نقل مخزون إلى {{receiver_name}} ({{transfer_no}}). يرجى مراجعته في اللوحة."},
	},
	EventTransferApproved: {
		"tr": {"Stok transferi {{transfer_no}} onaylandı",
			"{{sender_name}} → {{receiver_name}} stok transferi ({{transfer_no}}) onaylandı. Gönderen artık sevk edebilir."},
		"en": {"Stock transfer {{transfer_no}} approved",
			"The stock transfer from {{sender_name}} to {{receiver_name}} ({{transfer_no}}) was approved. The sender can now ship it."},
		"bg": {"Прехвърлянето на наличност {{transfer_no}} е одобрено",
			"Прехвърлянето на наличност от {{sender_name}} към {{receiver_name}} ({{transfer_no}}) е одобрено. Изпращачът вече може да го изпрати."},
		"de": {"Bestandsumbuchung {{transfer_no}} genehmigt",
			"Die Bestandsumbuchung von {{sender_name}} an {{receiver_name}} ({{transfer_no}}) wurde genehmigt. Der Absender kann sie jetzt versenden."},
		"el": {"Η μεταφορά αποθέματος {{transfer_no}} εγκρίθηκε",
			"Η μεταφορά αποθέματος από {{sender_name}} προς {{receiver_name}} ({{transfer_no}}) εγκρίθηκε. Ο αποστολέας μπορεί πλέον να την αποστείλει."},
		"uk": {"Переміщення запасів {{transfer_no}} схвалено",
			"Переміщення запасів від {{sender_name}} до {{receiver_name}} ({{transfer_no}}) схвалено. Відправник тепер може його відвантажити."},
		"ru": {"Перемещение запасов {{transfer_no}} одобрено",
			"Перемещение запасов от {{sender_name}} в {{receiver_name}} ({{transfer_no}}) одобрено. Отправитель теперь может его отгрузить."},
		"fr": {"Transfert de stock {{transfer_no}} approuvé",
			"Le transfert de stock de {{sender_name}} vers {{receiver_name}} ({{transfer_no}}) a été approuvé. L'expéditeur peut maintenant l'expédier."},
		"es": {"Transferencia de stock {{transfer_no}} aprobada",
			"La transferencia de stock de {{sender_name}} a {{receiver_name}} ({{transfer_no}}) fue aprobada. El remitente ya puede enviarla."},
		"it": {"Trasferimento di stock {{transfer_no}} approvato",
			"Il trasferimento di stock da {{sender_name}} a {{receiver_name}} ({{transfer_no}}) è stato approvato. Il mittente può ora spedirlo."},
		"zh-CN": {"库存调拨 {{transfer_no}} 已批准",
			"从 {{sender_name}} 到 {{receiver_name}} 的库存调拨（{{transfer_no}}）已批准。发货方现在可以发货。"},
		"az": {"Stok transferi {{transfer_no}} təsdiqləndi",
			"{{sender_name}} → {{receiver_name}} stok transferi ({{transfer_no}}) təsdiqləndi. Göndərən artıq göndərə bilər."},
		"ar": {"تمت الموافقة على نقل المخزون {{transfer_no}}",
			"تمت الموافقة على نقل المخزون من {{sender_name}} إلى {{receiver_name}} ({{transfer_no}}). يمكن للمرسل الآن شحنه."},
	},
	EventTransferRejected: {
		"tr": {"Stok transferi {{transfer_no}} reddedildi",
			"{{sender_name}} → {{receiver_name}} stok transferi ({{transfer_no}}) reddedildi."},
		"en": {"Stock transfer {{transfer_no}} rejected",
			"The stock transfer from {{sender_name}} to {{receiver_name}} ({{transfer_no}}) was rejected."},
		"bg": {"Прехвърлянето на наличност {{transfer_no}} е отхвърлено",
			"Прехвърлянето на наличност от {{sender_name}} към {{receiver_name}} ({{transfer_no}}) е отхвърлено."},
		"de": {"Bestandsumbuchung {{transfer_no}} abgelehnt",
			"Die Bestandsumbuchung von {{sender_name}} an {{receiver_name}} ({{transfer_no}}) wurde abgelehnt."},
		"el": {"Η μεταφορά αποθέματος {{transfer_no}} απορρίφθηκε",
			"Η μεταφορά αποθέματος από {{sender_name}} προς {{receiver_name}} ({{transfer_no}}) απορρίφθηκε."},
		"uk": {"Переміщення запасів {{transfer_no}} відхилено",
			"Переміщення запасів від {{sender_name}} до {{receiver_name}} ({{transfer_no}}) відхилено."},
		"ru": {"Перемещение запасов {{transfer_no}} отклонено",
			"Перемещение запасов от {{sender_name}} в {{receiver_name}} ({{transfer_no}}) отклонено."},
		"fr": {"Transfert de stock {{transfer_no}} refusé",
			"Le transfert de stock de {{sender_name}} vers {{receiver_name}} ({{transfer_no}}) a été refusé."},
		"es": {"Transferencia de stock {{transfer_no}} rechazada",
			"La transferencia de stock de {{sender_name}} a {{receiver_name}} ({{transfer_no}}) fue rechazada."},
		"it": {"Trasferimento di stock {{transfer_no}} rifiutato",
			"Il trasferimento di stock da {{sender_name}} a {{receiver_name}} ({{transfer_no}}) è stato rifiutato."},
		"zh-CN": {"库存调拨 {{transfer_no}} 已拒绝",
			"从 {{sender_name}} 到 {{receiver_name}} 的库存调拨（{{transfer_no}}）已被拒绝。"},
		"az": {"Stok transferi {{transfer_no}} rədd edildi",
			"{{sender_name}} → {{receiver_name}} stok transferi ({{transfer_no}}) rədd edildi."},
		"ar": {"تم رفض نقل المخزون {{transfer_no}}",
			"تم رفض نقل المخزون من {{sender_name}} إلى {{receiver_name}} ({{transfer_no}})."},
	},
	EventTransferShipped: {
		"tr": {"Stok transferi {{transfer_no}} sevk edildi",
			"{{sender_name}}, {{receiver_name}} için {{transfer_no}} numaralı stok transferini sevk etti. Ürünler ulaştığında panelden teslim alın."},
		"en": {"Stock transfer {{transfer_no}} shipped",
			"{{sender_name}} shipped stock transfer {{transfer_no}} to {{receiver_name}}. Confirm receipt in the panel when it arrives."},
		"bg": {"Прехвърлянето на наличност {{transfer_no}} е изпратено",
			"{{sender_name}} изпрати прехвърляне на наличност {{transfer_no}} към {{receiver_name}}. Потвърдете получаването в панела, когато пристигне."},
		"de": {"Bestandsumbuchung {{transfer_no}} versendet",
			"{{sender_name}} hat die Bestandsumbuchung {{transfer_no}} an {{receiver_name}} versendet. Bestätigen Sie den Empfang im Panel, sobald sie ankommt."},
		"el": {"Η μεταφορά αποθέματος {{transfer_no}} απεστάλη",
			"Η {{sender_name}} απέστειλε τη μεταφορά αποθέματος {{transfer_no}} προς {{receiver_name}}. Επιβεβαιώστε την παραλαβή στον πίνακα όταν φτάσει."},
		"uk": {"Переміщення запасів {{transfer_no}} відвантажено",
			"{{sender_name}} відвантажив переміщення запасів {{transfer_no}} до {{receiver_name}}. Підтвердьте отримання в панелі, коли воно надійде."},
		"ru": {"Перемещение запасов {{transfer_no}} отгружено",
			"{{sender_name}} отгрузил перемещение запасов {{transfer_no}} в {{receiver_name}}. Подтвердите получение в панели, когда оно прибудет."},
		"fr": {"Transfert de stock {{transfer_no}} expédié",
			"{{sender_name}} a expédié le transfert de stock {{transfer_no}} vers {{receiver_name}}. Confirmez la réception dans le panneau à son arrivée."},
		"es": {"Transferencia de stock {{transfer_no}} enviada",
			"{{sender_name}} envió la transferencia de stock {{transfer_no}} a {{receiver_name}}. Confirme la recepción en el panel cuando llegue."},
		"it": {"Trasferimento di stock {{transfer_no}} spedito",
			"{{sender_name}} ha spedito il trasferimento di stock {{transfer_no}} a {{receiver_name}}. Confermi la ricezione nel pannello all'arrivo."},
		"zh-CN": {"库存调拨 {{transfer_no}} 已发货",
			"{{sender_name}} 已将库存调拨 {{transfer_no}} 发往 {{receiver_name}}。到货后请在面板中确认收货。"},
		"az": {"Stok transferi {{transfer_no}} göndərildi",
			"{{sender_name}} {{transfer_no}} nömrəli stok transferini {{receiver_name}} üçün göndərdi. Məhsullar çatanda paneldə qəbulu təsdiqləyin."},
		"ar": {"تم شحن نقل المخزون {{transfer_no}}",
			"قام {{sender_name}} بشحن نقل المخزون {{transfer_no}} إلى {{receiver_name}}. أكد الاستلام في اللوحة عند الوصول."},
	},
	EventTransferReceived: {
		"tr": {"Stok transferi {{transfer_no}} teslim alındı",
			"{{receiver_name}}, {{sender_name}} tarafından gönderilen {{transfer_no}} numaralı stok transferini teslim aldı. Transfer tutarı cariye işlendi."},
		"en": {"Stock transfer {{transfer_no}} received",
			"{{receiver_name}} received stock transfer {{transfer_no}} from {{sender_name}}. The transfer amount was booked on the cari account."},
		"bg": {"Прехвърлянето на наличност {{transfer_no}} е получено",
			"{{receiver_name}} получи прехвърляне на наличност {{transfer_no}} от {{sender_name}}. Сумата на прехвърлянето е осчетоводена по текущата сметка."},
		"de": {"Bestandsumbuchung {{transfer_no}} empfangen",
			"{{receiver_name}} hat die Bestandsumbuchung {{transfer_no}} von {{sender_name}} empfangen. Der Betrag wurde auf dem Kontokorrent gebucht."},
		"el": {"Η μεταφορά αποθέματος {{transfer_no}} παραλήφθηκε",
			"Η {{receiver_name}} παρέλαβε τη μεταφορά αποθέματος {{transfer_no}} από {{sender_name}}. Το ποσό καταχωρίστηκε στον τρεχούμενο λογαριασμό."},
		"uk": {"Переміщення запасів {{transfer_no}} отримано",
			"{{receiver_name}} отримав переміщення запасів {{transfer_no}} від {{sender_name}}. Суму переміщення проведено на поточному рахунку."},
		"ru": {"Перемещение запасов {{transfer_no}} получено",
			"{{receiver_name}} получил перемещение запасов {{transfer_no}} от {{sender_name}}. Сумма перемещения проведена по текущему счёту."},
		"fr": {"Transfert de stock {{transfer_no}} reçu",
			"{{receiver_name}} a reçu le transfert de stock {{transfer_no}} de {{sender_name}}. Le montant a été enregistré sur le compte courant."},
		"es": {"Transferencia de stock {{transfer_no}} recibida",
			"{{receiver_name}} recibió la transferencia de stock {{transfer_no}} de {{sender_name}}. El importe se registró en la cuenta corriente."},
		"it": {"Trasferimento di stock {{transfer_no}} ricevuto",
			"{{receiver_name}} ha ricevuto il trasferimento di stock {{transfer_no}} da {{sender_name}}. L'importo è stato registrato sul conto corrente."},
		"zh-CN": {"库存调拨 {{transfer_no}} 已收货",
			"{{receiver_name}} 已收到来自 {{sender_name}} 的库存调拨 {{transfer_no}}。调拨金额已记入往来账户。"},
		"az": {"Stok transferi {{transfer_no}} qəbul edildi",
			"{{receiver_name}} {{sender_name}} tərəfindən göndərilən {{transfer_no}} nömrəli stok transferini qəbul etdi. Transfer məbləği cari hesaba yazıldı."},
		"ar": {"تم استلام نقل المخزون {{transfer_no}}",
			"استلم {{receiver_name}} نقل المخزون {{transfer_no}} من {{sender_name}}. تم قيد مبلغ النقل في الحساب الجاري."},
	},
	EventTransferCancelled: {
		"tr": {"Stok transferi {{transfer_no}} iptal edildi",
			"{{sender_name}} → {{receiver_name}} stok transferi ({{transfer_no}}) iptal edildi."},
		"en": {"Stock transfer {{transfer_no}} cancelled",
			"The stock transfer from {{sender_name}} to {{receiver_name}} ({{transfer_no}}) was cancelled."},
		"bg": {"Прехвърлянето на наличност {{transfer_no}} е отменено",
			"Прехвърлянето на наличност от {{sender_name}} към {{receiver_name}} ({{transfer_no}}) е отменено."},
		"de": {"Bestandsumbuchung {{transfer_no}} storniert",
			"Die Bestandsumbuchung von {{sender_name}} an {{receiver_name}} ({{transfer_no}}) wurde storniert."},
		"el": {"Η μεταφορά αποθέματος {{transfer_no}} ακυρώθηκε",
			"Η μεταφορά αποθέματος από {{sender_name}} προς {{receiver_name}} ({{transfer_no}}) ακυρώθηκε."},
		"uk": {"Переміщення запасів {{transfer_no}} скасовано",
			"Переміщення запасів від {{sender_name}} до {{receiver_name}} ({{transfer_no}}) скасовано."},
		"ru": {"Перемещение запасов {{transfer_no}} отменено",
			"Перемещение запасов от {{sender_name}} в {{receiver_name}} ({{transfer_no}}) отменено."},
		"fr": {"Transfert de stock {{transfer_no}} annulé",
			"Le transfert de stock de {{sender_name}} vers {{receiver_name}} ({{transfer_no}}) a été annulé."},
		"es": {"Transferencia de stock {{transfer_no}} cancelada",
			"La transferencia de stock de {{sender_name}} a {{receiver_name}} ({{transfer_no}}) fue cancelada."},
		"it": {"Trasferimento di stock {{transfer_no}} annullato",
			"Il trasferimento di stock da {{sender_name}} a {{receiver_name}} ({{transfer_no}}) è stato annullato."},
		"zh-CN": {"库存调拨 {{transfer_no}} 已取消",
			"从 {{sender_name}} 到 {{receiver_name}} 的库存调拨（{{transfer_no}}）已取消。"},
		"az": {"Stok transferi {{transfer_no}} ləğv edildi",
			"{{sender_name}} → {{receiver_name}} stok transferi ({{transfer_no}}) ləğv edildi."},
		"ar": {"تم إلغاء نقل المخزون {{transfer_no}}",
			"تم إلغاء نقل المخزون من {{sender_name}} إلى {{receiver_name}} ({{transfer_no}})."},
	},
}

func transferTemplates(code string) []DefaultTemplate {
	texts := transferTexts[code]
	out := make([]DefaultTemplate, 0, len(texts)*len(TransferChannels))
	for _, lang := range msgtemplate.Locales {
		t, ok := texts[lang]
		if !ok {
			continue // TestTransferTemplatesCoverEveryLocale reports it
		}
		for _, ch := range TransferChannels {
			out = append(out, DefaultTemplate{
				Role: RoleGeneric, Channel: ch, Language: lang,
				Subject: t.subject, Body: t.body, Format: "text",
			})
		}
	}
	return out
}

func init() {
	for _, code := range TransferEvents {
		Register(Event{
			Code: code, Module: "transfers",
			DefaultChannels: TransferChannels,
			AudienceRoles:   []string{RoleDealer, RoleDistributor},
			Placeholders: []msgtemplate.Placeholder{
				ph("transfer_no", "TR00000123", "TR00000123"),
				ph("sender_name", "Tech Oto", "Tech Oto"),
				ph("receiver_name", "Kuzey Oto", "North Auto"),
			},
			UserConfigurable: true,
			Templates:        transferTemplates(code),
		})
	}
}
