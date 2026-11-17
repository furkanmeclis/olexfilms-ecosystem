package catalog

import "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/msgtemplate"

// EventOrderReceiptDiscrepancy (TEC-1227): the buyer received an order
// with missing or damaged units. The seller's members holding
// orders.approve or orders.ship are told (notify_user_ids, resolved by the
// orders use case, the actor left out). In-app, web and Expo push, 13
// locales.
const EventOrderReceiptDiscrepancy = "ORDER_RECEIPT_DISCREPANCY"

var orderReceiptDiscrepancyTexts = map[string]localizedText{
	"tr": {"Teslimde fark bildirildi: {{order_no}}",
		"{{buyer_name}}, {{order_no}} numaralı siparişi teslim alırken {{missing_count}} eksik ve {{damaged_count}} hasarlı birim bildirdi. Fark kaydını sipariş detayında inceleyin."},
	"en": {"Receipt discrepancy reported: {{order_no}}",
		"{{buyer_name}} reported {{missing_count}} missing and {{damaged_count}} damaged units when receiving order {{order_no}}. Review the record on the order."},
	"bg": {"Отчетено несъответствие при получаване: {{order_no}}",
		"{{buyer_name}} отчете {{missing_count}} липсващи и {{damaged_count}} повредени единици при получаване на поръчка {{order_no}}. Прегледайте записа в поръчката."},
	"de": {"Abweichung beim Wareneingang gemeldet: {{order_no}}",
		"{{buyer_name}} hat beim Empfang der Bestellung {{order_no}} {{missing_count}} fehlende und {{damaged_count}} beschädigte Einheiten gemeldet. Prüfen Sie den Eintrag in der Bestellung."},
	"el": {"Αναφέρθηκε απόκλιση παραλαβής: {{order_no}}",
		"Ο {{buyer_name}} ανέφερε {{missing_count}} ελλείπουσες και {{damaged_count}} κατεστραμμένες μονάδες κατά την παραλαβή της παραγγελίας {{order_no}}. Ελέγξτε την καταγραφή στην παραγγελία."},
	"uk": {"Повідомлено про розбіжність при отриманні: {{order_no}}",
		"{{buyer_name}} повідомив про {{missing_count}} відсутніх і {{damaged_count}} пошкоджених одиниць під час отримання замовлення {{order_no}}. Перегляньте запис у замовленні."},
	"ru": {"Сообщено о расхождении при приёмке: {{order_no}}",
		"{{buyer_name}} сообщил о {{missing_count}} недостающих и {{damaged_count}} повреждённых единицах при приёмке заказа {{order_no}}. Проверьте запись в заказе."},
	"fr": {"Écart de réception signalé : {{order_no}}",
		"{{buyer_name}} a signalé {{missing_count}} unités manquantes et {{damaged_count}} endommagées à la réception de la commande {{order_no}}. Consultez le constat dans la commande."},
	"es": {"Diferencia en la recepción: {{order_no}}",
		"{{buyer_name}} informó {{missing_count}} unidades faltantes y {{damaged_count}} dañadas al recibir el pedido {{order_no}}. Revise el registro en el pedido."},
	"it": {"Discrepanza alla ricezione: {{order_no}}",
		"{{buyer_name}} ha segnalato {{missing_count}} unità mancanti e {{damaged_count}} danneggiate alla ricezione dell'ordine {{order_no}}. Controllate il verbale nell'ordine."},
	"zh-CN": {"收货差异已报告：{{order_no}}",
		"{{buyer_name}} 在接收订单 {{order_no}} 时报告了 {{missing_count}} 个缺失和 {{damaged_count}} 个损坏的单位。请在订单中查看记录。"},
	"az": {"Qəbulda fərq bildirildi: {{order_no}}",
		"{{buyer_name}} {{order_no}} nömrəli sifarişi qəbul edərkən {{missing_count}} əskik və {{damaged_count}} zədəli vahid bildirdi. Qeydi sifarişdə yoxlayın."},
	"ar": {"تم الإبلاغ عن فرق في الاستلام: {{order_no}}",
		"أبلغ {{buyer_name}} عن {{missing_count}} وحدات مفقودة و{{damaged_count}} وحدات تالفة عند استلام الطلب {{order_no}}. راجعوا السجل في الطلب."},
}

func init() {
	Register(Event{
		Code: EventOrderReceiptDiscrepancy, Module: "orders",
		DefaultChannels: OrderStatusChannels,
		AudienceRoles:   []string{RoleCenter, RoleDistributor},
		Placeholders: []msgtemplate.Placeholder{
			ph("order_no", "SP00000123", "SP00000123"),
			ph("buyer_name", "Kuzey Oto", "North Auto"),
			ph("missing_count", "2", "2"),
			ph("damaged_count", "1", "1"),
		},
		UserConfigurable: true,
		Templates:        orderStatusTemplates(orderReceiptDiscrepancyTexts, OrderStatusChannels),
	})
}
