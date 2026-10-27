package catalog

import "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/msgtemplate"

// Order notifications (TEC-1024): the center opened an approved order for a
// dealer directly under it. The orders use case resolves the recipients
// (notify_user_ids: the dealer's members holding orders.read, minus the
// actor) and the party names into the event payload. Default templates ship
// in all 13 locales for in-app and WhatsApp, like the transfer events.
const EventOrderCreatedBySeller = "ORDER_CREATED_BY_SELLER"

// OrderChannels are the default channels of the order events.
var OrderChannels = []string{ChannelInapp, ChannelWhatsApp}

var orderTexts = map[string]localizedText{
	"tr": {"Merkez sizin için sipariş açtı: {{order_no}}",
		"{{seller_name}}, {{buyer_name}} adına {{order_no}} numaralı siparişi oluşturdu ve onayladı. Sipariş hazırlanıp size sevk edilecek."},
	"en": {"An order was opened for you: {{order_no}}",
		"{{seller_name}} created and approved order {{order_no}} for {{buyer_name}}. It will be prepared and shipped to you."},
	"bg": {"Създадена е поръчка за вас: {{order_no}}",
		"{{seller_name}} създаде и одобри поръчка {{order_no}} за {{buyer_name}}. Тя ще бъде подготвена и изпратена до вас."},
	"de": {"Für Sie wurde eine Bestellung angelegt: {{order_no}}",
		"{{seller_name}} hat die Bestellung {{order_no}} für {{buyer_name}} angelegt und genehmigt. Sie wird vorbereitet und an Sie versendet."},
	"el": {"Δημιουργήθηκε παραγγελία για εσάς: {{order_no}}",
		"Η {{seller_name}} δημιούργησε και ενέκρινε την παραγγελία {{order_no}} για {{buyer_name}}. Θα προετοιμαστεί και θα σας αποσταλεί."},
	"uk": {"Для вас створено замовлення: {{order_no}}",
		"{{seller_name}} створив і схвалив замовлення {{order_no}} для {{buyer_name}}. Його буде підготовлено та відвантажено вам."},
	"ru": {"Для вас создан заказ: {{order_no}}",
		"{{seller_name}} создал и одобрил заказ {{order_no}} для {{buyer_name}}. Он будет подготовлен и отгружен вам."},
	"fr": {"Une commande a été ouverte pour vous : {{order_no}}",
		"{{seller_name}} a créé et approuvé la commande {{order_no}} pour {{buyer_name}}. Elle sera préparée puis expédiée."},
	"es": {"Se abrió un pedido para usted: {{order_no}}",
		"{{seller_name}} creó y aprobó el pedido {{order_no}} para {{buyer_name}}. Se preparará y se le enviará."},
	"it": {"È stato aperto un ordine per voi: {{order_no}}",
		"{{seller_name}} ha creato e approvato l'ordine {{order_no}} per {{buyer_name}}. Sarà preparato e spedito a voi."},
	"zh-CN": {"已为您创建订单：{{order_no}}",
		"{{seller_name}} 已为 {{buyer_name}} 创建并批准订单 {{order_no}}，将备货后发给您。"},
	"az": {"Sizin üçün sifariş açıldı: {{order_no}}",
		"{{seller_name}} {{buyer_name}} üçün {{order_no}} nömrəli sifarişi yaratdı və təsdiqlədi. Sifariş hazırlanıb sizə göndəriləcək."},
	"ar": {"تم فتح طلب لكم: {{order_no}}",
		"أنشأ {{seller_name}} الطلب {{order_no}} لصالح {{buyer_name}} ووافق عليه. سيتم تجهيزه وشحنه إليكم."},
}

func orderTemplates() []DefaultTemplate {
	out := make([]DefaultTemplate, 0, len(orderTexts)*len(OrderChannels))
	for _, lang := range msgtemplate.Locales {
		t, ok := orderTexts[lang]
		if !ok {
			continue // TestOrderTemplatesCoverEveryLocale reports it
		}
		for _, ch := range OrderChannels {
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
		Code: EventOrderCreatedBySeller, Module: "orders",
		DefaultChannels: OrderChannels,
		AudienceRoles:   []string{RoleDealer},
		Placeholders: []msgtemplate.Placeholder{
			ph("order_no", "SP00000123", "SP00000123"),
			ph("seller_name", "Olex Merkez", "Olex Center"),
			ph("buyer_name", "Kuzey Oto", "North Auto"),
		},
		UserConfigurable: true,
		Templates:        orderTemplates(),
	})
}
