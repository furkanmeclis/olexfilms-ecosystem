package catalog

import "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/msgtemplate"

// Customer welcome (TEC-164, F1-08d). An organization created a new
// customer: customer.created goes through the outbox to the notifications
// bus, which sends this WhatsApp message (wuzapi, K16/K21) with the portal
// link. Customers without a phone are skipped. Default templates ship in all
// 13 locales and are seeded with InsertNotificationTemplateIfMissing by
// SyncCatalog.
const EventCustomerWelcome = "CUSTOMER_WELCOME"

// CustomerWelcomeChannels are the default channels of the welcome message.
var CustomerWelcomeChannels = []string{ChannelWhatsApp}

var customerWelcomeTexts = map[string]localizedText{
	"tr": {"Hoş geldiniz",
		"Merhaba {{customer_name}}, {{organization_name}} müşteri kaydınızı oluşturdu. Araçlarınızı, hizmetlerinizi ve garantilerinizi müşteri portalından takip edebilirsiniz: {{portal_url}} Giriş, telefonunuza WhatsApp ile gelen tek kullanımlık kodla yapılır."},
	"en": {"Welcome",
		"Hello {{customer_name}}, {{organization_name}} has created your customer account. You can follow your vehicles, services and warranties on the customer portal: {{portal_url}} You sign in with a one-time code sent to your phone over WhatsApp."},
	"bg": {"Добре дошли",
		"Здравейте, {{customer_name}}, {{organization_name}} създаде вашия клиентски профил. Можете да следите автомобилите, услугите и гаранциите си в клиентския портал: {{portal_url}} Влизането става с еднократен код, изпратен на телефона ви чрез WhatsApp."},
	"de": {"Willkommen",
		"Hallo {{customer_name}}, {{organization_name}} hat Ihr Kundenkonto angelegt. Ihre Fahrzeuge, Services und Garantien können Sie im Kundenportal verfolgen: {{portal_url}} Die Anmeldung erfolgt mit einem Einmalcode, den Sie per WhatsApp auf Ihr Telefon erhalten."},
	"el": {"Καλώς ήρθατε",
		"Γεια σας {{customer_name}}, η {{organization_name}} δημιούργησε τον λογαριασμό πελάτη σας. Μπορείτε να παρακολουθείτε τα οχήματα, τις υπηρεσίες και τις εγγυήσεις σας στην πύλη πελατών: {{portal_url}} Η σύνδεση γίνεται με κωδικό μίας χρήσης που λαμβάνετε στο τηλέφωνό σας μέσω WhatsApp."},
	"uk": {"Ласкаво просимо",
		"Вітаємо, {{customer_name}}! {{organization_name}} створила ваш обліковий запис клієнта. Стежте за своїми автомобілями, послугами та гарантіями на клієнтському порталі: {{portal_url}} Вхід виконується за одноразовим кодом, який надходить на ваш телефон у WhatsApp."},
	"ru": {"Добро пожаловать",
		"Здравствуйте, {{customer_name}}! {{organization_name}} создала вашу учётную запись клиента. Следите за своими автомобилями, услугами и гарантиями на клиентском портале: {{portal_url}} Вход выполняется по одноразовому коду, который приходит на ваш телефон в WhatsApp."},
	"fr": {"Bienvenue",
		"Bonjour {{customer_name}}, {{organization_name}} a créé votre compte client. Vous pouvez suivre vos véhicules, prestations et garanties sur le portail client : {{portal_url}} La connexion se fait avec un code à usage unique envoyé sur votre téléphone via WhatsApp."},
	"es": {"Bienvenido",
		"Hola {{customer_name}}, {{organization_name}} ha creado su cuenta de cliente. Puede seguir sus vehículos, servicios y garantías en el portal de clientes: {{portal_url}} El acceso se realiza con un código de un solo uso que recibirá en su teléfono por WhatsApp."},
	"it": {"Benvenuto",
		"Salve {{customer_name}}, {{organization_name}} ha creato il suo account cliente. Può seguire i suoi veicoli, servizi e garanzie sul portale clienti: {{portal_url}} L'accesso avviene con un codice monouso inviato al suo telefono tramite WhatsApp."},
	"zh-CN": {"欢迎",
		"您好 {{customer_name}}，{{organization_name}} 已为您创建客户账户。您可以在客户门户中查看您的车辆、服务和质保：{{portal_url}} 登录时使用通过 WhatsApp 发送到您手机的一次性验证码。"},
	"az": {"Xoş gəlmisiniz",
		"Salam {{customer_name}}, {{organization_name}} müştəri hesabınızı yaratdı. Avtomobillərinizi, xidmətlərinizi və zəmanətlərinizi müştəri portalında izləyə bilərsiniz: {{portal_url}} Giriş telefonunuza WhatsApp ilə göndərilən birdəfəlik kodla edilir."},
	"ar": {"مرحبًا بك",
		"مرحبًا {{customer_name}}، أنشأت {{organization_name}} حساب العميل الخاص بك. يمكنك متابعة مركباتك وخدماتك وضماناتك عبر بوابة العملاء: {{portal_url}} يتم تسجيل الدخول برمز لمرة واحدة يصل إلى هاتفك عبر WhatsApp."},
}

func customerWelcomeTemplates() []DefaultTemplate {
	out := make([]DefaultTemplate, 0, len(customerWelcomeTexts)*len(CustomerWelcomeChannels))
	for _, lang := range msgtemplate.Locales {
		t, ok := customerWelcomeTexts[lang]
		if !ok {
			continue // TestCustomerWelcomeTemplatesCoverEveryLocale reports it
		}
		for _, ch := range CustomerWelcomeChannels {
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
		Code: EventCustomerWelcome, Module: "customers",
		DefaultChannels: CustomerWelcomeChannels,
		AudienceRoles:   []string{RoleCustomer},
		Placeholders: []msgtemplate.Placeholder{
			ph("customer_name", "Ahmet Yılmaz", "John Smith"),
			ph("organization_name", "Tech Oto", "Tech Oto"),
			ph("portal_url", "https://olexfilms.app/portal", "https://olexfilms.app/portal"),
		},
		UserConfigurable: true,
		Templates:        customerWelcomeTemplates(),
	})
}
