package usecase

import (
	"fmt"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/whatsapp/model"
)

// Fixed WhatsApp texts of the identity step, per conversation locale (13
// languages, en fallback).
const (
	textMenuHeader = iota
	textMenuCustomer
	textMenuFooter
	textSuspended
	textPortalClaim
)

var identityTexts = map[string][5]string{
	"tr": {
		"Bu numara birden fazla hesaba bağlı. Hangisiyle devam etmek istersiniz? Numarasını yazın:",
		"Müşteri",
		"Daha sonra değiştirmek için \"kimlik değiştir\" yazabilirsiniz.",
		"Hesabınız askıya alınmış olduğu için bu kanaldan işlem yapılamıyor. Lütfen bağlı olduğunuz merkezle iletişime geçin.",
		"Bu numara henüz doğrulanmamış bir müşteri kaydına bağlı. Kaydınıza erişmek için portala telefonunuzla giriş yapıp doğrulayın: %s",
	},
	"en": {
		"This number is linked to more than one account. Which one would you like to continue with? Reply with its number:",
		"Customer",
		"Type \"change identity\" to switch later.",
		"Your account is suspended, so no actions can be taken through this channel. Please contact your center.",
		"This number is linked to an unverified customer record. To access it, sign in to the portal with your phone and verify it: %s",
	},
	"bg": {
		"Този номер е свързан с повече от един акаунт. С кой искате да продължите? Отговорете с номера му:",
		"Клиент",
		"Напишете \"change identity\", за да смените по-късно.",
		"Акаунтът ви е спрян, затова през този канал не могат да се извършват действия. Моля, свържете се с вашия център.",
		"Този номер е свързан с непотвърден клиентски запис. За достъп влезте в портала с телефона си и го потвърдете: %s",
	},
	"de": {
		"Diese Nummer ist mit mehreren Konten verknüpft. Mit welchem möchten Sie fortfahren? Antworten Sie mit der Nummer:",
		"Kunde",
		"Schreiben Sie \"change identity\", um später zu wechseln.",
		"Ihr Konto ist gesperrt, daher sind über diesen Kanal keine Vorgänge möglich. Bitte wenden Sie sich an Ihre Zentrale.",
		"Diese Nummer gehört zu einem nicht bestätigten Kundendatensatz. Melden Sie sich zum Zugriff mit Ihrem Telefon im Portal an und bestätigen Sie es: %s",
	},
	"el": {
		"Αυτός ο αριθμός συνδέεται με περισσότερους από έναν λογαριασμούς. Με ποιον θέλετε να συνεχίσετε; Απαντήστε με τον αριθμό του:",
		"Πελάτης",
		"Γράψτε \"change identity\" για να αλλάξετε αργότερα.",
		"Ο λογαριασμός σας έχει ανασταλεί, επομένως δεν είναι δυνατές ενέργειες μέσω αυτού του καναλιού. Επικοινωνήστε με το κέντρο σας.",
		"Αυτός ο αριθμός συνδέεται με μη επαληθευμένη εγγραφή πελάτη. Για πρόσβαση, συνδεθείτε στην πύλη με το τηλέφωνό σας και επαληθεύστε το: %s",
	},
	"uk": {
		"Цей номер пов'язаний з кількома обліковими записами. З яким ви хочете продовжити? Надішліть його номер:",
		"Клієнт",
		"Напишіть \"change identity\", щоб змінити пізніше.",
		"Ваш обліковий запис призупинено, тому через цей канал дії неможливі. Зверніться до свого центру.",
		"Цей номер пов'язаний з непідтвердженим записом клієнта. Щоб отримати доступ, увійдіть на портал за номером телефону та підтвердьте його: %s",
	},
	"ru": {
		"Этот номер связан с несколькими учётными записями. С какой вы хотите продолжить? Отправьте её номер:",
		"Клиент",
		"Напишите \"change identity\", чтобы сменить позже.",
		"Ваша учётная запись приостановлена, поэтому через этот канал действия невозможны. Обратитесь в свой центр.",
		"Этот номер связан с неподтверждённой записью клиента. Чтобы получить доступ, войдите на портал по номеру телефона и подтвердите его: %s",
	},
	"fr": {
		"Ce numéro est associé à plusieurs comptes. Avec lequel souhaitez-vous continuer ? Répondez avec son numéro :",
		"Client",
		"Écrivez \"change identity\" pour changer plus tard.",
		"Votre compte est suspendu, aucune opération n'est donc possible par ce canal. Veuillez contacter votre centre.",
		"Ce numéro est associé à une fiche client non vérifiée. Pour y accéder, connectez-vous au portail avec votre téléphone et validez-le : %s",
	},
	"es": {
		"Este número está vinculado a más de una cuenta. ¿Con cuál quiere continuar? Responda con su número:",
		"Cliente",
		"Escriba \"change identity\" para cambiar más tarde.",
		"Su cuenta está suspendida, por lo que no se pueden realizar operaciones por este canal. Póngase en contacto con su centro.",
		"Este número está vinculado a un registro de cliente sin verificar. Para acceder, inicie sesión en el portal con su teléfono y verifíquelo: %s",
	},
	"it": {
		"Questo numero è collegato a più account. Con quale vuole continuare? Risponda con il suo numero:",
		"Cliente",
		"Scriva \"change identity\" per cambiare in seguito.",
		"Il suo account è sospeso, quindi non è possibile eseguire operazioni tramite questo canale. Contatti il suo centro.",
		"Questo numero è collegato a una scheda cliente non verificata. Per accedervi, entri nel portale con il suo telefono e lo verifichi: %s",
	},
	"zh_CN": {
		"此号码关联了多个账户。您想使用哪一个继续？请回复对应的编号：",
		"客户",
		"以后如需切换，请输入 \"change identity\"。",
		"您的账户已被暂停，无法通过此渠道办理业务。请联系您所属的中心。",
		"此号码关联了一条未验证的客户记录。如需访问，请使用手机号登录门户并完成验证：%s",
	},
	"az": {
		"Bu nömrə birdən çox hesaba bağlıdır. Hansı ilə davam etmək istəyirsiniz? Nömrəsini yazın:",
		"Müştəri",
		"Sonra dəyişmək üçün \"change identity\" yazın.",
		"Hesabınız dayandırıldığı üçün bu kanal vasitəsilə əməliyyat aparmaq mümkün deyil. Zəhmət olmasa, mərkəzinizlə əlaqə saxlayın.",
		"Bu nömrə təsdiqlənməmiş müştəri qeydinə bağlıdır. Ona daxil olmaq üçün telefonunuzla portala daxil olub təsdiqləyin: %s",
	},
	"ar": {
		"هذا الرقم مرتبط بأكثر من حساب. بأي حساب تريد المتابعة؟ أرسل رقمه:",
		"عميل",
		"اكتب \"change identity\" للتبديل لاحقًا.",
		"حسابك معلّق، لذلك لا يمكن إجراء أي عملية عبر هذه القناة. يرجى التواصل مع المركز التابع لك.",
		"هذا الرقم مرتبط بسجل عميل غير موثّق. للوصول إليه، سجّل الدخول إلى البوابة برقم هاتفك ووثّقه: %s",
	},
}

func identityText(locale string, key int) string {
	t, ok := identityTexts[locale]
	if !ok {
		t = identityTexts[FallbackConversationLocale]
	}
	return t[key]
}

// MenuText renders the numbered identity menu:
// "1) Olex Bayi Kadıköy\n2) Müşteri".
func MenuText(locale string, options []IdentityOption) string {
	var sb strings.Builder
	sb.WriteString(identityText(locale, textMenuHeader))
	for i, o := range options {
		label := o.OrgName
		if o.Kind == model.IdentityCustomer {
			label = identityText(locale, textMenuCustomer)
		}
		fmt.Fprintf(&sb, "\n%d) %s", i+1, label)
	}
	sb.WriteString("\n\n")
	sb.WriteString(identityText(locale, textMenuFooter))
	return sb.String()
}

// PortalClaimText is the visitor hint for a K26 unverified record: sign in
// to the portal with the phone (OTP) to claim it.
func PortalClaimText(locale, portalURL string) string {
	return fmt.Sprintf(identityText(locale, textPortalClaim), portalURL)
}
