package catalog

import "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/msgtemplate"

// TEC-1034: customer facing events also write the in-app row of the
// portal/mobile inbox (TEC-1021). The WhatsApp texts carry links; the in-app
// texts below drop them because the row's action link opens the screen.

// channelTemplates builds generic text templates from per-channel texts
// (channel -> locale -> text) in locale order; a missing locale is left to
// the per-event coverage tests.
func channelTemplates(channels []string, texts map[string]map[string]localizedText) []DefaultTemplate {
	out := make([]DefaultTemplate, 0, len(channels)*len(msgtemplate.Locales))
	for _, lang := range msgtemplate.Locales {
		for _, ch := range channels {
			t, ok := texts[ch][lang]
			if !ok {
				continue
			}
			out = append(out, DefaultTemplate{
				Role: RoleGeneric, Channel: ch, Language: lang,
				Subject: t.subject, Body: t.body, Format: "text",
			})
		}
	}
	return out
}

var serviceReviewRequestInappTexts = map[string]localizedText{
	"tr":    {"Hizmetimizi değerlendirir misiniz?", "{{plate}} plakalı aracınıza {{organization_name}} tarafından yapılan hizmeti değerlendirebilirsiniz."},
	"en":    {"How did we do?", "You can review the service {{organization_name}} performed on {{plate}}."},
	"bg":    {"Как се справихме?", "Можете да оцените услугата, извършена от {{organization_name}} на {{plate}}."},
	"de":    {"Wie zufrieden waren Sie?", "Sie können den Service von {{organization_name}} an {{plate}} bewerten."},
	"el":    {"Πώς τα πήγαμε;", "Μπορείτε να αξιολογήσετε την υπηρεσία της {{organization_name}} στο όχημα {{plate}}."},
	"uk":    {"Як ми впоралися?", "Ви можете оцінити послугу {{organization_name}} для авто {{plate}}."},
	"ru":    {"Как мы справились?", "Вы можете оценить услугу {{organization_name}} для авто {{plate}}."},
	"fr":    {"Votre avis compte", "Vous pouvez évaluer la prestation de {{organization_name}} sur {{plate}}."},
	"es":    {"¿Qué tal lo hicimos?", "Puede valorar el servicio de {{organization_name}} en {{plate}}."},
	"it":    {"Com'è andata?", "Può valutare il servizio di {{organization_name}} su {{plate}}."},
	"zh-CN": {"请评价我们的服务", "您可以评价 {{organization_name}} 为车辆 {{plate}} 提供的服务。"},
	"az":    {"Xidmətimizi qiymətləndirərdiniz?", "{{organization_name}} tərəfindən {{plate}} avtomobilinə göstərilən xidməti qiymətləndirə bilərsiniz."},
	"ar":    {"كيف كانت خدمتنا؟", "يمكنك تقييم الخدمة التي قدمتها {{organization_name}} للمركبة {{plate}}."},
}

var customerWelcomeInappTexts = map[string]localizedText{
	"tr":    {"Hoş geldiniz", "Merhaba {{customer_name}}, {{organization_name}} müşteri kaydınızı oluşturdu. Araçlarınızı, hizmetlerinizi ve garantilerinizi buradan takip edebilirsiniz."},
	"en":    {"Welcome", "Hello {{customer_name}}, {{organization_name}} has created your customer account. You can follow your vehicles, services and warranties here."},
	"bg":    {"Добре дошли", "Здравейте, {{customer_name}}, {{organization_name}} създаде вашия клиентски профил. Тук можете да следите автомобилите, услугите и гаранциите си."},
	"de":    {"Willkommen", "Hallo {{customer_name}}, {{organization_name}} hat Ihr Kundenkonto angelegt. Hier können Sie Ihre Fahrzeuge, Services und Garantien verfolgen."},
	"el":    {"Καλώς ήρθατε", "Γεια σας {{customer_name}}, η {{organization_name}} δημιούργησε τον λογαριασμό πελάτη σας. Εδώ μπορείτε να παρακολουθείτε τα οχήματα, τις υπηρεσίες και τις εγγυήσεις σας."},
	"uk":    {"Ласкаво просимо", "Вітаємо, {{customer_name}}! {{organization_name}} створила ваш обліковий запис клієнта. Тут ви можете стежити за своїми автомобілями, послугами та гарантіями."},
	"ru":    {"Добро пожаловать", "Здравствуйте, {{customer_name}}! {{organization_name}} создала вашу учётную запись клиента. Здесь вы можете следить за своими автомобилями, услугами и гарантиями."},
	"fr":    {"Bienvenue", "Bonjour {{customer_name}}, {{organization_name}} a créé votre compte client. Vous pouvez suivre ici vos véhicules, prestations et garanties."},
	"es":    {"Bienvenido", "Hola {{customer_name}}, {{organization_name}} ha creado su cuenta de cliente. Aquí puede seguir sus vehículos, servicios y garantías."},
	"it":    {"Benvenuto", "Salve {{customer_name}}, {{organization_name}} ha creato il suo account cliente. Qui può seguire i suoi veicoli, servizi e garanzie."},
	"zh-CN": {"欢迎", "您好 {{customer_name}}，{{organization_name}} 已为您创建客户账户。您可以在这里查看您的车辆、服务和质保。"},
	"az":    {"Xoş gəlmisiniz", "Salam {{customer_name}}, {{organization_name}} müştəri hesabınızı yaratdı. Avtomobillərinizi, xidmətlərinizi və zəmanətlərinizi burada izləyə bilərsiniz."},
	"ar":    {"مرحبًا بك", "مرحبًا {{customer_name}}، أنشأت {{organization_name}} حساب العميل الخاص بك. يمكنك هنا متابعة مركباتك وخدماتك وضماناتك."},
}

var quoteSentInappTexts = map[string]localizedText{
	"tr":    {"Teklifiniz hazır", "{{organization_name}} teklifiniz hazır. Toplam: {{total_amount}}. Ayrıntıları görmek için açın."},
	"en":    {"Your quote is ready", "Your quote from {{organization_name}} is ready. Total: {{total_amount}}. Open it to see the details."},
	"bg":    {"Вашата оферта е готова", "Офертата ви от {{organization_name}} е готова. Общо: {{total_amount}}. Отворете я, за да видите подробностите."},
	"de":    {"Ihr Angebot ist bereit", "Ihr Angebot von {{organization_name}} ist bereit. Gesamt: {{total_amount}}. Öffnen Sie es für die Details."},
	"el":    {"Η προσφορά σας είναι έτοιμη", "Η προσφορά σας από {{organization_name}} είναι έτοιμη. Σύνολο: {{total_amount}}. Ανοίξτε την για λεπτομέρειες."},
	"uk":    {"Ваша пропозиція готова", "Ваша пропозиція від {{organization_name}} готова. Разом: {{total_amount}}. Відкрийте її, щоб переглянути деталі."},
	"ru":    {"Ваше предложение готово", "Ваше предложение от {{organization_name}} готово. Итого: {{total_amount}}. Откройте его, чтобы посмотреть детали."},
	"fr":    {"Votre devis est prêt", "Votre devis de {{organization_name}} est prêt. Total : {{total_amount}}. Ouvrez-le pour voir le détail."},
	"es":    {"Su presupuesto está listo", "Su presupuesto de {{organization_name}} está listo. Total: {{total_amount}}. Ábralo para ver los detalles."},
	"it":    {"Il suo preventivo è pronto", "Il preventivo di {{organization_name}} è pronto. Totale: {{total_amount}}. Lo apra per vedere i dettagli."},
	"zh-CN": {"您的报价已准备好", "{{organization_name}} 的报价已准备好。总计：{{total_amount}}。打开查看详情。"},
	"az":    {"Təklifiniz hazırdır", "{{organization_name}} təklifiniz hazırdır. Cəmi: {{total_amount}}. Təfərrüatlara baxmaq üçün açın."},
	"ar":    {"عرض السعر جاهز", "عرض السعر من {{organization_name}} جاهز. الإجمالي: {{total_amount}}. افتحه لعرض التفاصيل."},
}
