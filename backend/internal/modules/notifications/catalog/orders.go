package catalog

import "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/msgtemplate"

// Order notifications (TEC-1024): the center opened an approved order for a
// dealer directly under it. The orders use case resolves the recipients
// (notify_user_ids: the dealer's members holding orders.read, minus the
// actor) and the party names into the event payload. Default templates ship
// in all 13 locales for in-app, WhatsApp, web push and Expo push.
const EventOrderCreatedBySeller = "ORDER_CREATED_BY_SELLER"

// EventOrderWarehouseNew (TEC-1028): a new approved order fell into a
// seller warehouse. The orders use case resolves the warehouse staff
// (notify_user_ids: members of the warehouse holding orders.ship, minus the
// actor). It goes in-app and as web and Expo push, in all 13 locales.
const EventOrderWarehouseNew = "ORDER_WAREHOUSE_NEW"

// OrderWarehouseChannels are the default channels of ORDER_WAREHOUSE_NEW.
var OrderWarehouseChannels = []string{ChannelInapp, ChannelWebPush, ChannelExpoPush}

// OrderChannels are the default channels of ORDER_CREATED_BY_SELLER: the
// dealer is told in-app, on WhatsApp and on the phone (web and Expo push).
var OrderChannels = []string{ChannelInapp, ChannelWhatsApp, ChannelWebPush, ChannelExpoPush}

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

var orderWarehouseTexts = map[string]localizedText{
	"tr": {"Yeni sipariş hazırlanacak: {{order_no}}",
		"{{buyer_name}} için {{order_no}} numaralı sipariş onaylandı ve {{warehouse_name}} deposuna düştü. Hazırlamaya başlayabilirsiniz."},
	"en": {"New order to prepare: {{order_no}}",
		"Order {{order_no}} for {{buyer_name}} was approved and assigned to the {{warehouse_name}} warehouse. You can start preparing it."},
	"bg": {"Нова поръчка за подготовка: {{order_no}}",
		"Поръчка {{order_no}} за {{buyer_name}} е одобрена и насочена към склад {{warehouse_name}}. Можете да започнете подготовката."},
	"de": {"Neue Bestellung zur Vorbereitung: {{order_no}}",
		"Die Bestellung {{order_no}} für {{buyer_name}} wurde genehmigt und dem Lager {{warehouse_name}} zugewiesen. Sie können mit der Vorbereitung beginnen."},
	"el": {"Νέα παραγγελία προς προετοιμασία: {{order_no}}",
		"Η παραγγελία {{order_no}} για {{buyer_name}} εγκρίθηκε και ανατέθηκε στην αποθήκη {{warehouse_name}}. Μπορείτε να ξεκινήσετε την προετοιμασία."},
	"uk": {"Нове замовлення до підготовки: {{order_no}}",
		"Замовлення {{order_no}} для {{buyer_name}} схвалено та призначено на склад {{warehouse_name}}. Можна починати підготовку."},
	"ru": {"Новый заказ к подготовке: {{order_no}}",
		"Заказ {{order_no}} для {{buyer_name}} одобрен и назначен на склад {{warehouse_name}}. Можно начинать подготовку."},
	"fr": {"Nouvelle commande à préparer : {{order_no}}",
		"La commande {{order_no}} pour {{buyer_name}} a été approuvée et affectée à l'entrepôt {{warehouse_name}}. Vous pouvez commencer la préparation."},
	"es": {"Nuevo pedido por preparar: {{order_no}}",
		"El pedido {{order_no}} para {{buyer_name}} fue aprobado y asignado al almacén {{warehouse_name}}. Puede empezar a prepararlo."},
	"it": {"Nuovo ordine da preparare: {{order_no}}",
		"L'ordine {{order_no}} per {{buyer_name}} è stato approvato e assegnato al magazzino {{warehouse_name}}. Può iniziare la preparazione."},
	"zh-CN": {"新订单待备货：{{order_no}}",
		"{{buyer_name}} 的订单 {{order_no}} 已批准并分配到 {{warehouse_name}} 仓库，可以开始备货。"},
	"az": {"Hazırlanacaq yeni sifariş: {{order_no}}",
		"{{buyer_name}} üçün {{order_no}} nömrəli sifariş təsdiqləndi və {{warehouse_name}} anbarına təyin olundu. Hazırlamağa başlaya bilərsiniz."},
	"ar": {"طلب جديد للتجهيز: {{order_no}}",
		"تمت الموافقة على الطلب {{order_no}} لصالح {{buyer_name}} وإسناده إلى مستودع {{warehouse_name}}. يمكنكم البدء في تجهيزه."},
}

func orderWarehouseTemplates() []DefaultTemplate {
	out := make([]DefaultTemplate, 0, len(orderWarehouseTexts)*len(OrderWarehouseChannels))
	for _, lang := range msgtemplate.Locales {
		t, ok := orderWarehouseTexts[lang]
		if !ok {
			continue // TestOrderWarehouseTemplatesCoverEveryLocale reports it
		}
		for _, ch := range OrderWarehouseChannels {
			out = append(out, DefaultTemplate{
				Role: RoleGeneric, Channel: ch, Language: lang,
				Subject: t.subject, Body: t.body, Format: "text",
			})
		}
	}
	return out
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
	Register(Event{
		Code: EventOrderWarehouseNew, Module: "orders",
		DefaultChannels: OrderWarehouseChannels,
		AudienceRoles:   []string{RoleCenter, RoleDistributor},
		Placeholders: []msgtemplate.Placeholder{
			ph("order_no", "SP00000123", "SP00000123"),
			ph("buyer_name", "Kuzey Oto", "North Auto"),
			ph("warehouse_name", "AFM Florya Merkez", "Main Warehouse"),
		},
		UserConfigurable: true,
		Templates:        orderWarehouseTemplates(),
	})
}
