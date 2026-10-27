package catalog

import "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/msgtemplate"

// Order status notifications after preparation (TEC-1024/1028 follow-up):
// the seller shipped or delivered the order (the buyer's members holding
// orders.read are told) and the buyer received it (the seller's members
// holding orders.approve are told). The orders use case resolves the
// recipients (notify_user_ids, minus the actor) and the party names into
// the orders.shipped / orders.delivered / orders.received payloads. They go
// in-app and as web and Expo push, in all 13 locales.
const (
	EventOrderShipped   = "ORDER_SHIPPED"
	EventOrderDelivered = "ORDER_DELIVERED"
	EventOrderReceived  = "ORDER_RECEIVED"
)

// OrderStatusChannels are the default channels of the status events (and,
// since the dealer is told on the phone too, of ORDER_CREATED_BY_SELLER
// plus WhatsApp).
var OrderStatusChannels = []string{ChannelInapp, ChannelWebPush, ChannelExpoPush}

var orderShippedTexts = map[string]localizedText{
	"tr": {"Siparişiniz yola çıktı: {{order_no}}",
		"{{seller_name}}, {{order_no}} numaralı siparişinizi sevk etti. Ürünler size ulaşınca siparişi \"Alındı\" olarak işaretleyin."},
	"en": {"Your order is on its way: {{order_no}}",
		"{{seller_name}} shipped order {{order_no}}. Mark the order as \"Received\" once the goods arrive."},
	"bg": {"Поръчката ви е изпратена: {{order_no}}",
		"{{seller_name}} изпрати поръчка {{order_no}}. Отбележете поръчката като „Получена“, когато стоката пристигне."},
	"de": {"Ihre Bestellung ist unterwegs: {{order_no}}",
		"{{seller_name}} hat die Bestellung {{order_no}} versendet. Markieren Sie die Bestellung als „Erhalten“, sobald die Ware eintrifft."},
	"el": {"Η παραγγελία σας απεστάλη: {{order_no}}",
		"Η {{seller_name}} απέστειλε την παραγγελία {{order_no}}. Σημειώστε την ως «Παραλήφθηκε» μόλις φτάσουν τα προϊόντα."},
	"uk": {"Ваше замовлення відправлено: {{order_no}}",
		"{{seller_name}} відвантажив замовлення {{order_no}}. Позначте замовлення як «Отримано», коли товар надійде."},
	"ru": {"Ваш заказ отправлен: {{order_no}}",
		"{{seller_name}} отгрузил заказ {{order_no}}. Отметьте заказ как «Получен», когда товар прибудет."},
	"fr": {"Votre commande est en route : {{order_no}}",
		"{{seller_name}} a expédié la commande {{order_no}}. Marquez la commande comme « Reçue » à l'arrivée des produits."},
	"es": {"Su pedido está en camino: {{order_no}}",
		"{{seller_name}} envió el pedido {{order_no}}. Marque el pedido como «Recibido» cuando lleguen los productos."},
	"it": {"Il vostro ordine è in viaggio: {{order_no}}",
		"{{seller_name}} ha spedito l'ordine {{order_no}}. Contrassegnate l'ordine come «Ricevuto» all'arrivo della merce."},
	"zh-CN": {"您的订单已发货：{{order_no}}",
		"{{seller_name}} 已发出订单 {{order_no}}。货物到达后请将订单标记为“已收货”。"},
	"az": {"Sifarişiniz yola çıxdı: {{order_no}}",
		"{{seller_name}} {{order_no}} nömrəli sifarişinizi göndərdi. Məhsullar çatanda sifarişi \"Qəbul edildi\" kimi qeyd edin."},
	"ar": {"طلبكم في الطريق: {{order_no}}",
		"قام {{seller_name}} بشحن الطلب {{order_no}}. ضعوا علامة \"تم الاستلام\" على الطلب عند وصول البضاعة."},
}

var orderDeliveredTexts = map[string]localizedText{
	"tr": {"Siparişiniz teslim edildi: {{order_no}}",
		"{{seller_name}}, {{order_no}} numaralı siparişi teslim edildi olarak işaretledi. Kontrol edip siparişi \"Alındı\" olarak onaylayın."},
	"en": {"Your order was delivered: {{order_no}}",
		"{{seller_name}} marked order {{order_no}} as delivered. Check the goods and confirm the order as \"Received\"."},
	"bg": {"Поръчката ви е доставена: {{order_no}}",
		"{{seller_name}} отбеляза поръчка {{order_no}} като доставена. Проверете стоката и потвърдете поръчката като „Получена“."},
	"de": {"Ihre Bestellung wurde zugestellt: {{order_no}}",
		"{{seller_name}} hat die Bestellung {{order_no}} als zugestellt markiert. Prüfen Sie die Ware und bestätigen Sie die Bestellung als „Erhalten“."},
	"el": {"Η παραγγελία σας παραδόθηκε: {{order_no}}",
		"Η {{seller_name}} σημείωσε την παραγγελία {{order_no}} ως παραδομένη. Ελέγξτε τα προϊόντα και επιβεβαιώστε ως «Παραλήφθηκε»."},
	"uk": {"Ваше замовлення доставлено: {{order_no}}",
		"{{seller_name}} позначив замовлення {{order_no}} як доставлене. Перевірте товар і підтвердьте замовлення як «Отримано»."},
	"ru": {"Ваш заказ доставлен: {{order_no}}",
		"{{seller_name}} отметил заказ {{order_no}} как доставленный. Проверьте товар и подтвердите заказ как «Получен»."},
	"fr": {"Votre commande a été livrée : {{order_no}}",
		"{{seller_name}} a marqué la commande {{order_no}} comme livrée. Vérifiez les produits et confirmez la commande comme « Reçue »."},
	"es": {"Su pedido fue entregado: {{order_no}}",
		"{{seller_name}} marcó el pedido {{order_no}} como entregado. Revise los productos y confirme el pedido como «Recibido»."},
	"it": {"Il vostro ordine è stato consegnato: {{order_no}}",
		"{{seller_name}} ha segnato l'ordine {{order_no}} come consegnato. Controllate la merce e confermate l'ordine come «Ricevuto»."},
	"zh-CN": {"您的订单已送达：{{order_no}}",
		"{{seller_name}} 已将订单 {{order_no}} 标记为已送达。请检查货物并将订单确认为“已收货”。"},
	"az": {"Sifarişiniz təhvil verildi: {{order_no}}",
		"{{seller_name}} {{order_no}} nömrəli sifarişi təhvil verilmiş kimi qeyd etdi. Məhsulları yoxlayıb sifarişi \"Qəbul edildi\" kimi təsdiqləyin."},
	"ar": {"تم تسليم طلبكم: {{order_no}}",
		"وضع {{seller_name}} علامة \"تم التسليم\" على الطلب {{order_no}}. تحققوا من البضاعة وأكدوا الطلب كـ \"تم الاستلام\"."},
}

var orderReceivedTexts = map[string]localizedText{
	"tr": {"Sipariş teslim alındı: {{order_no}}",
		"{{buyer_name}}, {{order_no}} numaralı siparişi teslim aldı. Tutar bayi carisine işlendi."},
	"en": {"Order received: {{order_no}}",
		"{{buyer_name}} received order {{order_no}}. The amount was booked to the dealer's account."},
	"bg": {"Поръчката е получена: {{order_no}}",
		"{{buyer_name}} получи поръчка {{order_no}}. Сумата е осчетоводена по сметката на дилъра."},
	"de": {"Bestellung erhalten: {{order_no}}",
		"{{buyer_name}} hat die Bestellung {{order_no}} erhalten. Der Betrag wurde auf dem Händlerkonto gebucht."},
	"el": {"Η παραγγελία παραλήφθηκε: {{order_no}}",
		"Ο {{buyer_name}} παρέλαβε την παραγγελία {{order_no}}. Το ποσό καταχωρήθηκε στον λογαριασμό του αντιπροσώπου."},
	"uk": {"Замовлення отримано: {{order_no}}",
		"{{buyer_name}} отримав замовлення {{order_no}}. Суму проведено на рахунок дилера."},
	"ru": {"Заказ получен: {{order_no}}",
		"{{buyer_name}} получил заказ {{order_no}}. Сумма проведена на счёт дилера."},
	"fr": {"Commande reçue : {{order_no}}",
		"{{buyer_name}} a reçu la commande {{order_no}}. Le montant a été comptabilisé sur le compte du revendeur."},
	"es": {"Pedido recibido: {{order_no}}",
		"{{buyer_name}} recibió el pedido {{order_no}}. El importe se registró en la cuenta del distribuidor."},
	"it": {"Ordine ricevuto: {{order_no}}",
		"{{buyer_name}} ha ricevuto l'ordine {{order_no}}. L'importo è stato registrato sul conto del rivenditore."},
	"zh-CN": {"订单已收货：{{order_no}}",
		"{{buyer_name}} 已收到订单 {{order_no}}，金额已记入经销商往来账。"},
	"az": {"Sifariş qəbul edildi: {{order_no}}",
		"{{buyer_name}} {{order_no}} nömrəli sifarişi qəbul etdi. Məbləğ diler hesabına yazıldı."},
	"ar": {"تم استلام الطلب: {{order_no}}",
		"استلم {{buyer_name}} الطلب {{order_no}}. تم قيد المبلغ في حساب الوكيل."},
}

// orderStatusTemplates builds the default templates of texts for channels.
func orderStatusTemplates(texts map[string]localizedText, channels []string) []DefaultTemplate {
	out := make([]DefaultTemplate, 0, len(texts)*len(channels))
	for _, lang := range msgtemplate.Locales {
		t, ok := texts[lang]
		if !ok {
			continue // TestOrderStatusTemplatesCoverEveryLocale reports it
		}
		for _, ch := range channels {
			out = append(out, DefaultTemplate{
				Role: RoleGeneric, Channel: ch, Language: lang,
				Subject: t.subject, Body: t.body, Format: "text",
			})
		}
	}
	return out
}

func orderStatusPlaceholders() []msgtemplate.Placeholder {
	return []msgtemplate.Placeholder{
		ph("order_no", "SP00000123", "SP00000123"),
		ph("seller_name", "Olex Merkez", "Olex Center"),
		ph("buyer_name", "Kuzey Oto", "North Auto"),
	}
}

func init() {
	Register(Event{
		Code: EventOrderShipped, Module: "orders",
		DefaultChannels:  OrderStatusChannels,
		AudienceRoles:    []string{RoleDealer, RoleDistributor},
		Placeholders:     orderStatusPlaceholders(),
		UserConfigurable: true,
		Templates:        orderStatusTemplates(orderShippedTexts, OrderStatusChannels),
	})
	Register(Event{
		Code: EventOrderDelivered, Module: "orders",
		DefaultChannels:  OrderStatusChannels,
		AudienceRoles:    []string{RoleDealer, RoleDistributor},
		Placeholders:     orderStatusPlaceholders(),
		UserConfigurable: true,
		Templates:        orderStatusTemplates(orderDeliveredTexts, OrderStatusChannels),
	})
	Register(Event{
		Code: EventOrderReceived, Module: "orders",
		DefaultChannels:  OrderStatusChannels,
		AudienceRoles:    []string{RoleCenter, RoleDistributor},
		Placeholders:     orderStatusPlaceholders(),
		UserConfigurable: true,
		Templates:        orderStatusTemplates(orderReceivedTexts, OrderStatusChannels),
	})
}
