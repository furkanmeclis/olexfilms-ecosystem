package catalog

import "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/msgtemplate"

// Fleet notifications (TEC-473, F5-02b). A dealer's link request reaches
// the fleet users (portal) who accept or reject it there; the decision
// reaches the requesting dealer user; an invited fleet user is told which
// fleet they joined (the password code arrives in the reset e-mail).
// Defaults are in-app and e-mail in all 13 locales, seeded by SyncCatalog.
const (
	EventFleetLinkRequested = "FLEET_LINK_REQUESTED"
	EventFleetLinkAccepted  = "FLEET_LINK_ACCEPTED"
	EventFleetLinkRejected  = "FLEET_LINK_REJECTED"
	EventFleetUserInvited   = "FLEET_USER_INVITED"
)

// FleetChannels are the default channels of the fleet events.
var FleetChannels = []string{ChannelInapp, ChannelEmail}

// FleetEvents lists the fleet catalog events.
var FleetEvents = []string{
	EventFleetLinkRequested, EventFleetLinkAccepted, EventFleetLinkRejected, EventFleetUserInvited,
}

var fleetTexts = map[string]map[string]localizedText{
	EventFleetLinkRequested: {
		"tr": {"Yeni bayi bağlantı isteği: {{dealer_name}}",
			"{{dealer_name}}, {{fleet_name}} filonuza hizmet vermek için bağlantı isteği gönderdi. Portalda isteği onaylayabilir veya reddedebilirsiniz."},
		"en": {"New dealer link request: {{dealer_name}}",
			"{{dealer_name}} asked to serve your fleet {{fleet_name}}. You can accept or reject the request in the portal."},
		"bg": {"Нова заявка за връзка от дилър: {{dealer_name}}",
			"{{dealer_name}} поиска да обслужва вашия автопарк {{fleet_name}}. Можете да приемете или отхвърлите заявката в портала."},
		"de": {"Neue Verknüpfungsanfrage eines Händlers: {{dealer_name}}",
			"{{dealer_name}} möchte Ihre Flotte {{fleet_name}} betreuen. Sie können die Anfrage im Portal annehmen oder ablehnen."},
		"el": {"Νέο αίτημα σύνδεσης αντιπροσώπου: {{dealer_name}}",
			"Ο αντιπρόσωπος {{dealer_name}} ζήτησε να εξυπηρετεί τον στόλο σας {{fleet_name}}. Μπορείτε να αποδεχτείτε ή να απορρίψετε το αίτημα στην πύλη."},
		"uk": {"Новий запит на зв'язок від дилера: {{dealer_name}}",
			"{{dealer_name}} хоче обслуговувати ваш автопарк {{fleet_name}}. Ви можете прийняти або відхилити запит на порталі."},
		"ru": {"Новый запрос на связь от дилера: {{dealer_name}}",
			"{{dealer_name}} хочет обслуживать ваш автопарк {{fleet_name}}. Вы можете принять или отклонить запрос на портале."},
		"fr": {"Nouvelle demande de liaison d'un concessionnaire : {{dealer_name}}",
			"{{dealer_name}} souhaite prendre en charge votre flotte {{fleet_name}}. Vous pouvez accepter ou refuser la demande sur le portail."},
		"es": {"Nueva solicitud de vínculo de un concesionario: {{dealer_name}}",
			"{{dealer_name}} solicitó dar servicio a su flota {{fleet_name}}. Puede aceptar o rechazar la solicitud en el portal."},
		"it": {"Nuova richiesta di collegamento da un concessionario: {{dealer_name}}",
			"{{dealer_name}} ha chiesto di servire la sua flotta {{fleet_name}}. Può accettare o rifiutare la richiesta nel portale."},
		"zh-CN": {"新的经销商关联请求：{{dealer_name}}",
			"{{dealer_name}} 申请为您的车队 {{fleet_name}} 提供服务。您可以在门户中接受或拒绝该请求。"},
		"az": {"Dilerdən yeni əlaqə sorğusu: {{dealer_name}}",
			"{{dealer_name}} {{fleet_name}} avtoparkınıza xidmət göstərmək üçün sorğu göndərdi. Sorğunu portalda qəbul və ya rədd edə bilərsiniz."},
		"ar": {"طلب ربط جديد من وكيل: {{dealer_name}}",
			"طلب {{dealer_name}} تقديم الخدمة لأسطولك {{fleet_name}}. يمكنك قبول الطلب أو رفضه في البوابة."},
	},
	EventFleetLinkAccepted: {
		"tr": {"Filo bağlantınız onaylandı: {{fleet_name}}",
			"{{fleet_name}} filosu bağlantı isteğinizi onayladı. Filonun araçlarına artık hizmet verebilirsiniz."},
		"en": {"Your fleet link was accepted: {{fleet_name}}",
			"The fleet {{fleet_name}} accepted your link request. You can now serve the fleet's vehicles."},
		"bg": {"Връзката ви с автопарка е приета: {{fleet_name}}",
			"Автопаркът {{fleet_name}} прие заявката ви за връзка. Вече можете да обслужвате превозните му средства."},
		"de": {"Ihre Flottenverknüpfung wurde angenommen: {{fleet_name}}",
			"Die Flotte {{fleet_name}} hat Ihre Verknüpfungsanfrage angenommen. Sie können die Fahrzeuge der Flotte jetzt betreuen."},
		"el": {"Η σύνδεσή σας με τον στόλο έγινε αποδεκτή: {{fleet_name}}",
			"Ο στόλος {{fleet_name}} αποδέχτηκε το αίτημα σύνδεσής σας. Μπορείτε πλέον να εξυπηρετείτε τα οχήματά του."},
		"uk": {"Ваш зв'язок з автопарком прийнято: {{fleet_name}}",
			"Автопарк {{fleet_name}} прийняв ваш запит на зв'язок. Тепер ви можете обслуговувати його транспортні засоби."},
		"ru": {"Ваша связь с автопарком принята: {{fleet_name}}",
			"Автопарк {{fleet_name}} принял ваш запрос на связь. Теперь вы можете обслуживать его транспортные средства."},
		"fr": {"Votre liaison avec la flotte a été acceptée : {{fleet_name}}",
			"La flotte {{fleet_name}} a accepté votre demande de liaison. Vous pouvez désormais entretenir ses véhicules."},
		"es": {"Su vínculo con la flota fue aceptado: {{fleet_name}}",
			"La flota {{fleet_name}} aceptó su solicitud de vínculo. Ya puede dar servicio a sus vehículos."},
		"it": {"Il collegamento con la flotta è stato accettato: {{fleet_name}}",
			"La flotta {{fleet_name}} ha accettato la sua richiesta di collegamento. Ora può servire i suoi veicoli."},
		"zh-CN": {"您的车队关联已被接受：{{fleet_name}}",
			"车队 {{fleet_name}} 已接受您的关联请求。您现在可以为该车队的车辆提供服务。"},
		"az": {"Avtopark əlaqəniz qəbul edildi: {{fleet_name}}",
			"{{fleet_name}} avtoparkı əlaqə sorğunuzu qəbul etdi. Artıq onun nəqliyyat vasitələrinə xidmət göstərə bilərsiniz."},
		"ar": {"تم قبول ربطك بالأسطول: {{fleet_name}}",
			"قبل الأسطول {{fleet_name}} طلب الربط الخاص بك. يمكنك الآن تقديم الخدمة لمركباته."},
	},
	EventFleetLinkRejected: {
		"tr": {"Filo bağlantınız reddedildi: {{fleet_name}}",
			"{{fleet_name}} filosu bağlantı isteğinizi reddetti. Gerekirse filo yetkilisiyle görüşüp yeniden istek gönderebilirsiniz."},
		"en": {"Your fleet link was rejected: {{fleet_name}}",
			"The fleet {{fleet_name}} rejected your link request. If needed, talk to the fleet contact and send a new request."},
		"bg": {"Връзката ви с автопарка е отхвърлена: {{fleet_name}}",
			"Автопаркът {{fleet_name}} отхвърли заявката ви за връзка. При нужда говорете с отговорника на автопарка и изпратете нова заявка."},
		"de": {"Ihre Flottenverknüpfung wurde abgelehnt: {{fleet_name}}",
			"Die Flotte {{fleet_name}} hat Ihre Verknüpfungsanfrage abgelehnt. Sprechen Sie bei Bedarf mit dem Ansprechpartner der Flotte und senden Sie eine neue Anfrage."},
		"el": {"Η σύνδεσή σας με τον στόλο απορρίφθηκε: {{fleet_name}}",
			"Ο στόλος {{fleet_name}} απέρριψε το αίτημα σύνδεσής σας. Αν χρειάζεται, μιλήστε με τον υπεύθυνο του στόλου και στείλτε νέο αίτημα."},
		"uk": {"Ваш зв'язок з автопарком відхилено: {{fleet_name}}",
			"Автопарк {{fleet_name}} відхилив ваш запит на зв'язок. За потреби зверніться до контактної особи автопарку й надішліть новий запит."},
		"ru": {"Ваша связь с автопарком отклонена: {{fleet_name}}",
			"Автопарк {{fleet_name}} отклонил ваш запрос на связь. При необходимости свяжитесь с контактным лицом автопарка и отправьте новый запрос."},
		"fr": {"Votre liaison avec la flotte a été refusée : {{fleet_name}}",
			"La flotte {{fleet_name}} a refusé votre demande de liaison. Si besoin, contactez le responsable de la flotte et envoyez une nouvelle demande."},
		"es": {"Su vínculo con la flota fue rechazado: {{fleet_name}}",
			"La flota {{fleet_name}} rechazó su solicitud de vínculo. Si es necesario, hable con el responsable de la flota y envíe una nueva solicitud."},
		"it": {"Il collegamento con la flotta è stato rifiutato: {{fleet_name}}",
			"La flotta {{fleet_name}} ha rifiutato la sua richiesta di collegamento. Se necessario, contatti il referente della flotta e invii una nuova richiesta."},
		"zh-CN": {"您的车队关联被拒绝：{{fleet_name}}",
			"车队 {{fleet_name}} 拒绝了您的关联请求。如有需要，请与车队联系人沟通后重新发送请求。"},
		"az": {"Avtopark əlaqəniz rədd edildi: {{fleet_name}}",
			"{{fleet_name}} avtoparkı əlaqə sorğunuzu rədd etdi. Lazım olarsa, avtoparkın məsul şəxsi ilə danışıb yenidən sorğu göndərə bilərsiniz."},
		"ar": {"تم رفض ربطك بالأسطول: {{fleet_name}}",
			"رفض الأسطول {{fleet_name}} طلب الربط الخاص بك. عند الحاجة، تواصل مع مسؤول الأسطول وأرسل طلبًا جديدًا."},
	},
	EventFleetUserInvited: {
		"tr": {"{{fleet_name}} filo hesabına davet edildiniz",
			"{{dealer_name}} sizi {{fleet_name}} filo hesabına ekledi. Portala e-posta adresinizle giriş yapın; şifrenizi ayrı e-postayla gelen kodla belirleyin."},
		"en": {"You were invited to the fleet account {{fleet_name}}",
			"{{dealer_name}} added you to the fleet account {{fleet_name}}. Sign in to the portal with your e-mail address; set your password with the code sent in a separate e-mail."},
		"bg": {"Поканени сте в акаунта на автопарка {{fleet_name}}",
			"{{dealer_name}} ви добави към акаунта на автопарка {{fleet_name}}. Влезте в портала с имейл адреса си; задайте паролата си с кода от отделния имейл."},
		"de": {"Sie wurden zum Flottenkonto {{fleet_name}} eingeladen",
			"{{dealer_name}} hat Sie zum Flottenkonto {{fleet_name}} hinzugefügt. Melden Sie sich mit Ihrer E-Mail-Adresse im Portal an; legen Sie Ihr Passwort mit dem Code aus einer separaten E-Mail fest."},
		"el": {"Προσκληθήκατε στον λογαριασμό στόλου {{fleet_name}}",
			"Ο αντιπρόσωπος {{dealer_name}} σας πρόσθεσε στον λογαριασμό στόλου {{fleet_name}}. Συνδεθείτε στην πύλη με τη διεύθυνση email σας· ορίστε τον κωδικό πρόσβασης με τον κωδικό που λάβατε σε ξεχωριστό email."},
		"uk": {"Вас запрошено до облікового запису автопарку {{fleet_name}}",
			"{{dealer_name}} додав вас до облікового запису автопарку {{fleet_name}}. Увійдіть на портал зі своєю електронною адресою; встановіть пароль за кодом з окремого листа."},
		"ru": {"Вы приглашены в учётную запись автопарка {{fleet_name}}",
			"{{dealer_name}} добавил вас в учётную запись автопарка {{fleet_name}}. Войдите на портал со своим адресом электронной почты; задайте пароль с помощью кода из отдельного письма."},
		"fr": {"Vous êtes invité au compte de flotte {{fleet_name}}",
			"{{dealer_name}} vous a ajouté au compte de flotte {{fleet_name}}. Connectez-vous au portail avec votre adresse e-mail ; définissez votre mot de passe avec le code reçu dans un e-mail séparé."},
		"es": {"Ha sido invitado a la cuenta de flota {{fleet_name}}",
			"{{dealer_name}} le añadió a la cuenta de flota {{fleet_name}}. Inicie sesión en el portal con su correo electrónico; establezca su contraseña con el código enviado en un correo aparte."},
		"it": {"È stato invitato all'account flotta {{fleet_name}}",
			"{{dealer_name}} l'ha aggiunta all'account flotta {{fleet_name}}. Acceda al portale con il suo indirizzo e-mail; imposti la password con il codice inviato in un'e-mail separata."},
		"zh-CN": {"您已被邀请加入车队账户 {{fleet_name}}",
			"{{dealer_name}} 已将您添加到车队账户 {{fleet_name}}。请使用您的电子邮箱登录门户，并使用另一封邮件中的验证码设置密码。"},
		"az": {"{{fleet_name}} avtopark hesabına dəvət olundunuz",
			"{{dealer_name}} sizi {{fleet_name}} avtopark hesabına əlavə etdi. Portala e-poçt ünvanınızla daxil olun; şifrənizi ayrıca e-poçtla gələn kodla təyin edin."},
		"ar": {"تمت دعوتك إلى حساب الأسطول {{fleet_name}}",
			"أضافك {{dealer_name}} إلى حساب الأسطول {{fleet_name}}. سجّل الدخول إلى البوابة بعنوان بريدك الإلكتروني، وعيّن كلمة المرور بالرمز المرسل في رسالة منفصلة."},
	},
}

func fleetTemplates(code string) []DefaultTemplate {
	texts := fleetTexts[code]
	out := make([]DefaultTemplate, 0, len(texts)*len(FleetChannels))
	for _, lang := range msgtemplate.Locales {
		t, ok := texts[lang]
		if !ok {
			continue // TestFleetTemplatesCoverEveryLocale reports it
		}
		for _, ch := range FleetChannels {
			out = append(out, DefaultTemplate{
				Role: RoleGeneric, Channel: ch, Language: lang,
				Subject: t.subject, Body: t.body, Format: "text",
			})
		}
	}
	return out
}

func fleetPlaceholders() []msgtemplate.Placeholder {
	return []msgtemplate.Placeholder{
		ph("fleet_name", "Ankara Lojistik A.Ş.", "Ankara Logistics Ltd."),
		ph("dealer_name", "Tech Oto", "Tech Oto"),
	}
}

func init() {
	for _, code := range FleetEvents {
		audience := []string{RoleDealer, RoleDistributor}
		if code == EventFleetLinkRequested || code == EventFleetUserInvited {
			audience = []string{RoleCustomer}
		}
		Register(Event{
			Code: code, Module: "fleet",
			DefaultChannels:  FleetChannels,
			AudienceRoles:    audience,
			Placeholders:     fleetPlaceholders(),
			UserConfigurable: code != EventFleetUserInvited,
			Templates:        fleetTemplates(code),
		})
	}
}
