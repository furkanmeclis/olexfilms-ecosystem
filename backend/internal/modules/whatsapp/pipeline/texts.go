package pipeline

import "fmt"

// Fixed WhatsApp texts of the AI pipeline, per conversation locale (13
// languages, en fallback). They are sent without a model call.
const (
	// textConsent asks for the AI guidelines consent; %s is the guidelines
	// link or text.
	textConsent = iota
	// textConsentThanks answers an EVET without an open question.
	textConsentThanks
	textStopped
	textHandover
	textQuotaHandover
	textVoiceUnsupported
	textDocumentReceived
	textFailed
	// textConfirmHeader / textConfirmQuestion frame a confirmation card.
	textConfirmHeader
	textConfirmQuestion
	textActionDone
	textActionFailed
	textActionCancelled
	textActionExpired
	// textNotifyHandover / textNotifyQuota are the staff notification
	// bodies; %s is the contact. textNotifyTitle is their title.
	textNotifyTitle
	textNotifyHandover
	textNotifyQuota
	textCount
)

var texts = map[string][textCount]string{
	"tr": {
		"Merhaba! Sorularınızı yapay zekâ asistanımız yanıtlıyor. Devam etmeden önce yapay zekâ kullanım yönergesini kabul etmeniz gerekiyor:\n\n%s\n\nKabul ediyorsanız *EVET* yazın.",
		"Teşekkürler, onayınız kaydedildi. Size nasıl yardımcı olabilirim?",
		"Kampanya ve duyuru mesajlarından çıkarıldınız. Hizmet, randevu ve garanti bildirimleri gelmeye devam eder.",
		"Talebinizi bir temsilcimize ilettik; en kısa sürede size dönülecek.",
		"Şu an otomatik yanıt veremiyoruz. Mesajınızı ekibimize ilettik; en kısa sürede size dönülecek.",
		"Sesli mesajları şu an dinleyemiyoruz. Lütfen sorunuzu yazarak gönderin.",
		"Belgenizi aldık. İçeriğini buradan okuyamıyoruz; gerekirse ekibimiz inceleyecek.",
		"Mesajınızı şu an işleyemedik. Lütfen biraz sonra tekrar deneyin.",
		"Onayınızı bekleyen işlem:",
		"Onaylıyor musunuz? Onaylamak için *EVET*, vazgeçmek için *HAYIR* yazın.",
		"İşlem tamamlandı.",
		"İşlem tamamlanamadı. Lütfen kaydı uygulamadan kontrol edin.",
		"İşlem iptal edildi.",
		"Bu işlemin onay süresi doldu; isterseniz yeniden isteyebilirsiniz.",
		"WhatsApp konuşması size devredildi",
		"%s bir temsilciyle görüşmek istiyor.",
		"%s için yapay zekâ kotası doldu; konuşma ekibe devredildi.",
	},
	"en": {
		"Hello! Our AI assistant answers your questions. Before we continue, please accept the AI usage guidelines:\n\n%s\n\nReply *YES* to accept.",
		"Thank you, your consent is saved. How can I help you?",
		"You have been removed from campaign and announcement messages. Service, appointment and warranty notifications will still be sent.",
		"We have passed your request to one of our representatives; they will get back to you shortly.",
		"We cannot answer automatically right now. We have passed your message to our team; they will get back to you shortly.",
		"We cannot listen to voice messages yet. Please send your question as text.",
		"We received your document. We cannot read its content here; our team will review it if needed.",
		"We could not process your message right now. Please try again a little later.",
		"Action awaiting your approval:",
		"Do you approve? Reply *YES* to approve or *NO* to cancel.",
		"Done.",
		"The action could not be completed. Please check the record in the app.",
		"The action was cancelled.",
		"The approval time of this action has expired; you can ask again if you like.",
		"WhatsApp conversation handed over to you",
		"%s wants to talk to a representative.",
		"The AI quota for %s is used up; the conversation was handed over to the team.",
	},
	"bg": {
		"Здравейте! На въпросите ви отговаря нашият AI асистент. Преди да продължим, моля, приемете правилата за използване на AI:\n\n%s\n\nОтговорете с *ДА*, за да приемете.",
		"Благодарим, съгласието ви е записано. С какво мога да помогна?",
		"Премахнахме ви от съобщенията за кампании и обяви. Известията за услуги, часове и гаранции ще продължат да се изпращат.",
		"Предадохме заявката ви на наш представител; той ще се свърже с вас скоро.",
		"В момента не можем да отговорим автоматично. Предадохме съобщението ви на нашия екип; ще се свържем с вас скоро.",
		"Все още не можем да слушаме гласови съобщения. Моля, изпратете въпроса си като текст.",
		"Получихме документа ви. Не можем да прочетем съдържанието му тук; екипът ни ще го прегледа при нужда.",
		"В момента не успяхме да обработим съобщението ви. Моля, опитайте отново малко по-късно.",
		"Действие, очакващо вашето одобрение:",
		"Одобрявате ли? Отговорете с *ДА*, за да одобрите, или *НЕ*, за да откажете.",
		"Готово.",
		"Действието не можа да бъде завършено. Моля, проверете записа в приложението.",
		"Действието е отменено.",
		"Времето за одобрение на това действие изтече; можете да поискате отново.",
		"WhatsApp разговор е предаден на вас",
		"%s иска да говори с представител.",
		"AI квотата за %s е изчерпана; разговорът е предаден на екипа.",
	},
	"de": {
		"Hallo! Ihre Fragen beantwortet unser KI-Assistent. Bevor wir fortfahren, akzeptieren Sie bitte die KI-Nutzungsrichtlinien:\n\n%s\n\nAntworten Sie mit *JA*, um zuzustimmen.",
		"Vielen Dank, Ihre Zustimmung ist gespeichert. Wie kann ich Ihnen helfen?",
		"Sie erhalten keine Kampagnen- und Ankündigungsnachrichten mehr. Benachrichtigungen zu Service, Terminen und Garantie werden weiterhin gesendet.",
		"Wir haben Ihre Anfrage an einen unserer Mitarbeiter weitergeleitet; er meldet sich in Kürze bei Ihnen.",
		"Wir können gerade nicht automatisch antworten. Wir haben Ihre Nachricht an unser Team weitergeleitet; es meldet sich in Kürze bei Ihnen.",
		"Sprachnachrichten können wir noch nicht anhören. Bitte senden Sie Ihre Frage als Text.",
		"Wir haben Ihr Dokument erhalten. Den Inhalt können wir hier nicht lesen; unser Team prüft es bei Bedarf.",
		"Ihre Nachricht konnte gerade nicht verarbeitet werden. Bitte versuchen Sie es etwas später erneut.",
		"Vorgang, der auf Ihre Bestätigung wartet:",
		"Bestätigen Sie? Antworten Sie mit *JA* zum Bestätigen oder *NEIN* zum Abbrechen.",
		"Erledigt.",
		"Der Vorgang konnte nicht abgeschlossen werden. Bitte prüfen Sie den Datensatz in der App.",
		"Der Vorgang wurde abgebrochen.",
		"Die Bestätigungszeit für diesen Vorgang ist abgelaufen; Sie können ihn gern erneut anfragen.",
		"WhatsApp-Unterhaltung an Sie übergeben",
		"%s möchte mit einem Mitarbeiter sprechen.",
		"Das KI-Kontingent für %s ist aufgebraucht; die Unterhaltung wurde an das Team übergeben.",
	},
	"el": {
		"Γεια σας! Στις ερωτήσεις σας απαντά ο βοηθός τεχνητής νοημοσύνης μας. Πριν συνεχίσουμε, αποδεχτείτε τις οδηγίες χρήσης της τεχνητής νοημοσύνης:\n\n%s\n\nΑπαντήστε *ΝΑΙ* για αποδοχή.",
		"Ευχαριστούμε, η συγκατάθεσή σας αποθηκεύτηκε. Πώς μπορώ να σας βοηθήσω;",
		"Αφαιρεθήκατε από τα μηνύματα καμπανιών και ανακοινώσεων. Οι ειδοποιήσεις για υπηρεσίες, ραντεβού και εγγυήσεις θα συνεχίσουν να αποστέλλονται.",
		"Διαβιβάσαμε το αίτημά σας σε έναν εκπρόσωπό μας· θα επικοινωνήσει μαζί σας σύντομα.",
		"Δεν μπορούμε να απαντήσουμε αυτόματα αυτή τη στιγμή. Διαβιβάσαμε το μήνυμά σας στην ομάδα μας· θα επικοινωνήσει μαζί σας σύντομα.",
		"Δεν μπορούμε ακόμη να ακούσουμε φωνητικά μηνύματα. Στείλτε την ερώτησή σας γραπτώς.",
		"Λάβαμε το έγγραφό σας. Δεν μπορούμε να διαβάσουμε το περιεχόμενό του εδώ· η ομάδα μας θα το εξετάσει αν χρειαστεί.",
		"Δεν μπορέσαμε να επεξεργαστούμε το μήνυμά σας αυτή τη στιγμή. Δοκιμάστε ξανά λίγο αργότερα.",
		"Ενέργεια που περιμένει την έγκρισή σας:",
		"Εγκρίνετε; Απαντήστε *ΝΑΙ* για έγκριση ή *ΟΧΙ* για ακύρωση.",
		"Ολοκληρώθηκε.",
		"Η ενέργεια δεν ολοκληρώθηκε. Ελέγξτε την εγγραφή στην εφαρμογή.",
		"Η ενέργεια ακυρώθηκε.",
		"Ο χρόνος έγκρισης αυτής της ενέργειας έληξε· μπορείτε να τη ζητήσετε ξανά.",
		"Η συνομιλία WhatsApp σάς ανατέθηκε",
		"Ο/Η %s θέλει να μιλήσει με εκπρόσωπο.",
		"Το όριο τεχνητής νοημοσύνης για %s εξαντλήθηκε· η συνομιλία ανατέθηκε στην ομάδα.",
	},
	"uk": {
		"Вітаємо! На ваші запитання відповідає наш AI-асистент. Перш ніж продовжити, прийміть правила використання AI:\n\n%s\n\nНапишіть *ТАК*, щоб погодитися.",
		"Дякуємо, вашу згоду збережено. Чим можу допомогти?",
		"Вас вилучено з розсилки кампаній та оголошень. Сповіщення про послуги, записи та гарантії й надалі надходитимуть.",
		"Ми передали ваш запит нашому представнику; він незабаром зв'яжеться з вами.",
		"Зараз ми не можемо відповісти автоматично. Ми передали ваше повідомлення нашій команді; з вами незабаром зв'яжуться.",
		"Ми поки не можемо прослуховувати голосові повідомлення. Будь ласка, надішліть запитання текстом.",
		"Ми отримали ваш документ. Прочитати його вміст тут ми не можемо; за потреби його перегляне наша команда.",
		"Зараз не вдалося обробити ваше повідомлення. Спробуйте ще раз трохи згодом.",
		"Дія, що очікує вашого підтвердження:",
		"Підтверджуєте? Напишіть *ТАК*, щоб підтвердити, або *НІ*, щоб скасувати.",
		"Готово.",
		"Дію не вдалося завершити. Перевірте запис у застосунку.",
		"Дію скасовано.",
		"Час підтвердження цієї дії минув; за бажання попросіть ще раз.",
		"Розмову WhatsApp передано вам",
		"%s хоче поговорити з представником.",
		"AI-квоту для %s вичерпано; розмову передано команді.",
	},
	"ru": {
		"Здравствуйте! На ваши вопросы отвечает наш AI-ассистент. Прежде чем продолжить, примите правила использования AI:\n\n%s\n\nНапишите *ДА*, чтобы согласиться.",
		"Спасибо, ваше согласие сохранено. Чем могу помочь?",
		"Вы исключены из рассылки кампаний и объявлений. Уведомления об услугах, записях и гарантиях будут приходить по-прежнему.",
		"Мы передали ваш запрос нашему представителю; он скоро свяжется с вами.",
		"Сейчас мы не можем ответить автоматически. Мы передали ваше сообщение нашей команде; с вами скоро свяжутся.",
		"Мы пока не можем прослушивать голосовые сообщения. Пожалуйста, отправьте вопрос текстом.",
		"Мы получили ваш документ. Прочитать его содержимое здесь мы не можем; при необходимости его просмотрит наша команда.",
		"Сейчас не удалось обработать ваше сообщение. Попробуйте ещё раз чуть позже.",
		"Действие, ожидающее вашего подтверждения:",
		"Подтверждаете? Напишите *ДА*, чтобы подтвердить, или *НЕТ*, чтобы отменить.",
		"Готово.",
		"Действие не удалось завершить. Проверьте запись в приложении.",
		"Действие отменено.",
		"Время подтверждения этого действия истекло; при желании запросите его снова.",
		"Разговор WhatsApp передан вам",
		"%s хочет поговорить с представителем.",
		"AI-квота для %s исчерпана; разговор передан команде.",
	},
	"fr": {
		"Bonjour ! Notre assistant IA répond à vos questions. Avant de continuer, veuillez accepter les règles d'utilisation de l'IA :\n\n%s\n\nRépondez *OUI* pour accepter.",
		"Merci, votre consentement est enregistré. Comment puis-je vous aider ?",
		"Vous ne recevrez plus nos messages de campagnes et d'annonces. Les notifications de service, de rendez-vous et de garantie continueront d'être envoyées.",
		"Nous avons transmis votre demande à l'un de nos conseillers ; il vous répondra rapidement.",
		"Nous ne pouvons pas répondre automatiquement pour le moment. Nous avons transmis votre message à notre équipe ; elle vous répondra rapidement.",
		"Nous ne pouvons pas encore écouter les messages vocaux. Merci d'envoyer votre question par écrit.",
		"Nous avons bien reçu votre document. Nous ne pouvons pas en lire le contenu ici ; notre équipe l'examinera si nécessaire.",
		"Nous n'avons pas pu traiter votre message pour le moment. Veuillez réessayer un peu plus tard.",
		"Action en attente de votre validation :",
		"Confirmez-vous ? Répondez *OUI* pour confirmer ou *NON* pour annuler.",
		"C'est fait.",
		"L'action n'a pas pu être menée à bien. Veuillez vérifier la fiche dans l'application.",
		"L'action a été annulée.",
		"Le délai de validation de cette action a expiré ; vous pouvez la redemander si vous le souhaitez.",
		"Conversation WhatsApp qui vous est transférée",
		"%s souhaite parler à un conseiller.",
		"Le quota IA de %s est épuisé ; la conversation a été transférée à l'équipe.",
	},
	"es": {
		"¡Hola! Nuestro asistente de IA responde a sus preguntas. Antes de continuar, acepte las normas de uso de la IA:\n\n%s\n\nResponda *SÍ* para aceptar.",
		"Gracias, su consentimiento ha quedado registrado. ¿En qué puedo ayudarle?",
		"Ya no recibirá nuestros mensajes de campañas y anuncios. Las notificaciones de servicio, citas y garantía se seguirán enviando.",
		"Hemos trasladado su solicitud a uno de nuestros agentes; se pondrá en contacto con usted en breve.",
		"Ahora mismo no podemos responder automáticamente. Hemos trasladado su mensaje a nuestro equipo; se pondrá en contacto con usted en breve.",
		"Todavía no podemos escuchar mensajes de voz. Por favor, envíe su pregunta por escrito.",
		"Hemos recibido su documento. No podemos leer su contenido aquí; nuestro equipo lo revisará si es necesario.",
		"No hemos podido procesar su mensaje en este momento. Inténtelo de nuevo un poco más tarde.",
		"Acción pendiente de su aprobación:",
		"¿Lo aprueba? Responda *SÍ* para aprobar o *NO* para cancelar.",
		"Hecho.",
		"No se pudo completar la acción. Compruebe el registro en la aplicación.",
		"La acción se ha cancelado.",
		"El plazo de aprobación de esta acción ha vencido; puede volver a solicitarla si lo desea.",
		"Conversación de WhatsApp transferida a usted",
		"%s quiere hablar con un agente.",
		"La cuota de IA de %s se ha agotado; la conversación se ha transferido al equipo.",
	},
	"it": {
		"Salve! Alle sue domande risponde il nostro assistente IA. Prima di continuare, accetti le regole di utilizzo dell'IA:\n\n%s\n\nRisponda *SÌ* per accettare.",
		"Grazie, il suo consenso è stato registrato. Come posso aiutarla?",
		"Non riceverà più i nostri messaggi di campagne e annunci. Le notifiche su servizi, appuntamenti e garanzie continueranno a essere inviate.",
		"Abbiamo inoltrato la sua richiesta a un nostro operatore; la ricontatterà a breve.",
		"Al momento non possiamo rispondere automaticamente. Abbiamo inoltrato il suo messaggio al nostro team; la ricontatterà a breve.",
		"Non possiamo ancora ascoltare i messaggi vocali. Invii la sua domanda per iscritto.",
		"Abbiamo ricevuto il suo documento. Non possiamo leggerne il contenuto qui; il nostro team lo esaminerà se necessario.",
		"Al momento non siamo riusciti a elaborare il suo messaggio. Riprovi tra poco.",
		"Operazione in attesa della sua approvazione:",
		"Approva? Risponda *SÌ* per approvare o *NO* per annullare.",
		"Fatto.",
		"Non è stato possibile completare l'operazione. Controlli la scheda nell'app.",
		"L'operazione è stata annullata.",
		"Il tempo per approvare questa operazione è scaduto; può richiederla di nuovo se vuole.",
		"Conversazione WhatsApp assegnata a lei",
		"%s vuole parlare con un operatore.",
		"La quota IA di %s è esaurita; la conversazione è stata passata al team.",
	},
	"zh_CN": {
		"您好！我们的 AI 助手将回答您的问题。继续之前，请先接受 AI 使用准则：\n\n%s\n\n回复 *是* 表示接受。",
		"谢谢，您的同意已记录。请问有什么可以帮您？",
		"您已退订活动和公告消息。服务、预约和质保通知仍会照常发送。",
		"我们已将您的请求转交给客服代表，他们会尽快与您联系。",
		"目前无法自动回复。我们已将您的消息转交给团队，他们会尽快与您联系。",
		"我们暂时无法收听语音消息。请以文字形式发送您的问题。",
		"我们已收到您的文件。此处无法读取其内容；如有需要，我们的团队会进行查看。",
		"暂时无法处理您的消息。请稍后再试。",
		"等待您确认的操作：",
		"您是否确认？回复 *是* 确认，回复 *否* 取消。",
		"已完成。",
		"操作未能完成。请在应用中检查该记录。",
		"操作已取消。",
		"此操作的确认时间已过期；如有需要可以重新提出。",
		"WhatsApp 会话已转交给您",
		"%s 希望与客服代表沟通。",
		"%s 的 AI 配额已用完；会话已转交给团队。",
	},
	"az": {
		"Salam! Suallarınızı süni intellekt köməkçimiz cavablandırır. Davam etməzdən əvvəl süni intellektdən istifadə qaydalarını qəbul edin:\n\n%s\n\nQəbul etmək üçün *BƏLİ* yazın.",
		"Təşəkkür edirik, razılığınız qeydə alındı. Sizə necə kömək edə bilərəm?",
		"Kampaniya və elan mesajlarından çıxarıldınız. Xidmət, görüş və zəmanət bildirişləri göndərilməyə davam edəcək.",
		"Sorğunuzu nümayəndəmizə ötürdük; tezliklə sizinlə əlaqə saxlanılacaq.",
		"Hazırda avtomatik cavab verə bilmirik. Mesajınızı komandamıza ötürdük; tezliklə sizinlə əlaqə saxlanılacaq.",
		"Səsli mesajları hələ dinləyə bilmirik. Zəhmət olmasa, sualınızı yazılı göndərin.",
		"Sənədinizi aldıq. Onun məzmununu burada oxuya bilmirik; lazım olsa, komandamız nəzərdən keçirəcək.",
		"Mesajınızı hazırda emal edə bilmədik. Zəhmət olmasa, bir az sonra yenidən cəhd edin.",
		"Təsdiqinizi gözləyən əməliyyat:",
		"Təsdiq edirsiniz? Təsdiq üçün *BƏLİ*, imtina üçün *XEYR* yazın.",
		"Tamamlandı.",
		"Əməliyyat tamamlanmadı. Zəhmət olmasa, qeydi tətbiqdə yoxlayın.",
		"Əməliyyat ləğv edildi.",
		"Bu əməliyyatın təsdiq müddəti bitdi; istəsəniz yenidən xahiş edə bilərsiniz.",
		"WhatsApp söhbəti sizə ötürüldü",
		"%s nümayəndə ilə danışmaq istəyir.",
		"%s üçün süni intellekt limiti bitdi; söhbət komandaya ötürüldü.",
	},
	"ar": {
		"مرحبًا! يجيب مساعدنا بالذكاء الاصطناعي عن أسئلتك. قبل المتابعة، يرجى قبول إرشادات استخدام الذكاء الاصطناعي:\n\n%s\n\nأرسل *نعم* للموافقة.",
		"شكرًا لك، تم حفظ موافقتك. كيف يمكنني مساعدتك؟",
		"تمت إزالتك من رسائل الحملات والإعلانات. ستستمر إشعارات الخدمة والمواعيد والضمان في الوصول إليك.",
		"أحلنا طلبك إلى أحد ممثلينا، وسيتواصل معك قريبًا.",
		"لا يمكننا الرد تلقائيًا الآن. أحلنا رسالتك إلى فريقنا، وسيتواصل معك قريبًا.",
		"لا يمكننا الاستماع إلى الرسائل الصوتية بعد. يرجى إرسال سؤالك كتابةً.",
		"استلمنا مستندك. لا يمكننا قراءة محتواه هنا؛ سيراجعه فريقنا عند الحاجة.",
		"تعذّرت معالجة رسالتك الآن. يرجى المحاولة مرة أخرى بعد قليل.",
		"إجراء بانتظار موافقتك:",
		"هل توافق؟ أرسل *نعم* للموافقة أو *لا* للإلغاء.",
		"تم.",
		"تعذّر إكمال الإجراء. يرجى التحقق من السجل في التطبيق.",
		"تم إلغاء الإجراء.",
		"انتهت مهلة الموافقة على هذا الإجراء؛ يمكنك طلبه مجددًا إن رغبت.",
		"تمت إحالة محادثة واتساب إليك",
		"يريد %s التحدث إلى ممثل.",
		"نفدت حصة الذكاء الاصطناعي لـ %s؛ تمت إحالة المحادثة إلى الفريق.",
	},
}

// text returns a fixed text in the conversation locale (en fallback).
func text(locale string, key int) string {
	t, ok := texts[locale]
	if !ok {
		t = texts[fallbackLocale]
	}
	return t[key]
}

// textf formats a fixed text.
func textf(locale string, key int, args ...any) string {
	return fmt.Sprintf(text(locale, key), args...)
}

// fallbackLocale answers languages outside the 13 locales.
const fallbackLocale = "en"
