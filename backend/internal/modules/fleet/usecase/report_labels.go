package usecase

import (
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
)

// reportLabels are the texts of the fleet report tables and e-mail in one
// language (TEC-476). The headings around the tables live in the
// fleet_report document template (one per language, editable). Mail texts
// use {fleet}, {period} and {url} placeholders; Quarter uses {q} and {year}.
type reportLabels struct {
	Months        [12]string
	Quarter       string
	DateLayout    string
	Dealer        string
	Vehicle       string
	Plate         string
	Services      string
	Part          string
	Count         string
	Product       string
	Items         string
	Pieces        string
	Meters        string
	Ends          string
	ServiceAmount string
	Payments      string
	Balance       string
	Empty         string
	MailSubject   string
	MailBody      string
	MailLink      string
}

var reportTexts = map[i18n.Locale]reportLabels{
	i18n.LocaleTR: {
		Months:  [12]string{"Ocak", "Şubat", "Mart", "Nisan", "Mayıs", "Haziran", "Temmuz", "Ağustos", "Eylül", "Ekim", "Kasım", "Aralık"},
		Quarter: "{year} {q}. çeyrek", DateLayout: "02.01.2006",
		Dealer: "Bayi", Vehicle: "Araç", Plate: "Plaka", Services: "Hizmet", Part: "Parça", Count: "Adet",
		Product: "Ürün", Items: "Kalem", Pieces: "Parça adedi", Meters: "Metre", Ends: "Bitiş",
		ServiceAmount: "Hizmet tutarı", Payments: "Ödemeler", Balance: "Bakiye", Empty: "Bu dönemde kayıt yok.",
		MailSubject: "{fleet} filo raporu: {period}",
		MailBody:    "Merhaba,\n\n{fleet} filosunun {period} dönemine ait raporu ektedir.",
		MailLink:    "Rapor dosyası e-posta eki için çok büyük. Raporu portaldan indirebilirsiniz: {url}",
	},
	i18n.LocaleEN: {
		Months:  [12]string{"January", "February", "March", "April", "May", "June", "July", "August", "September", "October", "November", "December"},
		Quarter: "Q{q} {year}", DateLayout: "2006-01-02",
		Dealer: "Dealer", Vehicle: "Vehicle", Plate: "Plate", Services: "Services", Part: "Part", Count: "Count",
		Product: "Product", Items: "Items", Pieces: "Pieces", Meters: "Meters", Ends: "Ends",
		ServiceAmount: "Service amount", Payments: "Payments", Balance: "Balance", Empty: "No records in this period.",
		MailSubject: "{fleet} fleet report: {period}",
		MailBody:    "Hello,\n\nPlease find attached the report of the {fleet} fleet for {period}.",
		MailLink:    "The report file is too large for an e-mail attachment. You can download it from the portal: {url}",
	},
	i18n.LocaleBG: {
		Months:  [12]string{"януари", "февруари", "март", "април", "май", "юни", "юли", "август", "септември", "октомври", "ноември", "декември"},
		Quarter: "{q}. тримесечие {year}", DateLayout: "02.01.2006",
		Dealer: "Дилър", Vehicle: "Автомобил", Plate: "Рег. номер", Services: "Услуги", Part: "Част", Count: "Брой",
		Product: "Продукт", Items: "Позиции", Pieces: "Бройки", Meters: "Метри", Ends: "Изтича",
		ServiceAmount: "Сума за услуги", Payments: "Плащания", Balance: "Салдо", Empty: "Няма записи за този период.",
		MailSubject: "Отчет за автопарка {fleet}: {period}",
		MailBody:    "Здравейте,\n\nПриложен е отчетът на автопарка {fleet} за {period}.",
		MailLink:    "Файлът на отчета е твърде голям за прикачване. Можете да го изтеглите от портала: {url}",
	},
	i18n.LocaleDE: {
		Months:  [12]string{"Januar", "Februar", "März", "April", "Mai", "Juni", "Juli", "August", "September", "Oktober", "November", "Dezember"},
		Quarter: "{q}. Quartal {year}", DateLayout: "02.01.2006",
		Dealer: "Händler", Vehicle: "Fahrzeug", Plate: "Kennzeichen", Services: "Dienstleistungen", Part: "Bauteil", Count: "Anzahl",
		Product: "Produkt", Items: "Positionen", Pieces: "Stück", Meters: "Meter", Ends: "Endet am",
		ServiceAmount: "Leistungsbetrag", Payments: "Zahlungen", Balance: "Saldo", Empty: "Keine Einträge in diesem Zeitraum.",
		MailSubject: "Flottenbericht {fleet}: {period}",
		MailBody:    "Guten Tag,\n\nim Anhang finden Sie den Bericht der Flotte {fleet} für {period}.",
		MailLink:    "Die Berichtsdatei ist für einen E-Mail-Anhang zu groß. Sie können sie im Portal herunterladen: {url}",
	},
	i18n.LocaleEL: {
		Months:  [12]string{"Ιανουάριος", "Φεβρουάριος", "Μάρτιος", "Απρίλιος", "Μάιος", "Ιούνιος", "Ιούλιος", "Αύγουστος", "Σεπτέμβριος", "Οκτώβριος", "Νοέμβριος", "Δεκέμβριος"},
		Quarter: "{q}ο τρίμηνο {year}", DateLayout: "02/01/2006",
		Dealer: "Αντιπρόσωπος", Vehicle: "Όχημα", Plate: "Πινακίδα", Services: "Υπηρεσίες", Part: "Τμήμα", Count: "Πλήθος",
		Product: "Προϊόν", Items: "Γραμμές", Pieces: "Τεμάχια", Meters: "Μέτρα", Ends: "Λήξη",
		ServiceAmount: "Ποσό υπηρεσιών", Payments: "Πληρωμές", Balance: "Υπόλοιπο", Empty: "Δεν υπάρχουν εγγραφές σε αυτή την περίοδο.",
		MailSubject: "Αναφορά στόλου {fleet}: {period}",
		MailBody:    "Γεια σας,\n\nΕπισυνάπτεται η αναφορά του στόλου {fleet} για την περίοδο {period}.",
		MailLink:    "Το αρχείο της αναφοράς είναι πολύ μεγάλο για συνημμένο. Μπορείτε να το κατεβάσετε από την πύλη: {url}",
	},
	i18n.LocaleUK: {
		Months:  [12]string{"січень", "лютий", "березень", "квітень", "травень", "червень", "липень", "серпень", "вересень", "жовтень", "листопад", "грудень"},
		Quarter: "{q} квартал {year}", DateLayout: "02.01.2006",
		Dealer: "Дилер", Vehicle: "Автомобіль", Plate: "Номерний знак", Services: "Послуги", Part: "Деталь", Count: "Кількість",
		Product: "Продукт", Items: "Позиції", Pieces: "Штуки", Meters: "Метри", Ends: "Закінчується",
		ServiceAmount: "Сума послуг", Payments: "Платежі", Balance: "Сальдо", Empty: "За цей період записів немає.",
		MailSubject: "Звіт автопарку {fleet}: {period}",
		MailBody:    "Вітаємо,\n\nдо листа додано звіт автопарку {fleet} за {period}.",
		MailLink:    "Файл звіту завеликий для вкладення. Ви можете завантажити його на порталі: {url}",
	},
	i18n.LocaleRU: {
		Months:  [12]string{"январь", "февраль", "март", "апрель", "май", "июнь", "июль", "август", "сентябрь", "октябрь", "ноябрь", "декабрь"},
		Quarter: "{q} квартал {year}", DateLayout: "02.01.2006",
		Dealer: "Дилер", Vehicle: "Автомобиль", Plate: "Госномер", Services: "Услуги", Part: "Деталь", Count: "Количество",
		Product: "Продукт", Items: "Позиции", Pieces: "Штуки", Meters: "Метры", Ends: "Окончание",
		ServiceAmount: "Сумма услуг", Payments: "Платежи", Balance: "Сальдо", Empty: "За этот период записей нет.",
		MailSubject: "Отчёт автопарка {fleet}: {period}",
		MailBody:    "Здравствуйте,\n\nво вложении отчёт автопарка {fleet} за {period}.",
		MailLink:    "Файл отчёта слишком велик для вложения. Вы можете скачать его на портале: {url}",
	},
	i18n.LocaleFR: {
		Months:  [12]string{"janvier", "février", "mars", "avril", "mai", "juin", "juillet", "août", "septembre", "octobre", "novembre", "décembre"},
		Quarter: "{q}e trimestre {year}", DateLayout: "02/01/2006",
		Dealer: "Concessionnaire", Vehicle: "Véhicule", Plate: "Immatriculation", Services: "Prestations", Part: "Pièce", Count: "Nombre",
		Product: "Produit", Items: "Lignes", Pieces: "Pièces", Meters: "Mètres", Ends: "Fin",
		ServiceAmount: "Montant des prestations", Payments: "Paiements", Balance: "Solde", Empty: "Aucun enregistrement sur cette période.",
		MailSubject: "Rapport de flotte {fleet} : {period}",
		MailBody:    "Bonjour,\n\nvous trouverez ci-joint le rapport de la flotte {fleet} pour {period}.",
		MailLink:    "Le fichier du rapport est trop volumineux pour une pièce jointe. Vous pouvez le télécharger sur le portail : {url}",
	},
	i18n.LocaleES: {
		Months:  [12]string{"enero", "febrero", "marzo", "abril", "mayo", "junio", "julio", "agosto", "septiembre", "octubre", "noviembre", "diciembre"},
		Quarter: "{q}.º trimestre de {year}", DateLayout: "02/01/2006",
		Dealer: "Concesionario", Vehicle: "Vehículo", Plate: "Matrícula", Services: "Servicios", Part: "Pieza", Count: "Cantidad",
		Product: "Producto", Items: "Líneas", Pieces: "Unidades", Meters: "Metros", Ends: "Vence",
		ServiceAmount: "Importe de servicios", Payments: "Pagos", Balance: "Saldo", Empty: "No hay registros en este periodo.",
		MailSubject: "Informe de la flota {fleet}: {period}",
		MailBody:    "Hola:\n\nadjuntamos el informe de la flota {fleet} correspondiente a {period}.",
		MailLink:    "El archivo del informe es demasiado grande para adjuntarlo. Puede descargarlo en el portal: {url}",
	},
	i18n.LocaleIT: {
		Months:  [12]string{"gennaio", "febbraio", "marzo", "aprile", "maggio", "giugno", "luglio", "agosto", "settembre", "ottobre", "novembre", "dicembre"},
		Quarter: "{q}° trimestre {year}", DateLayout: "02/01/2006",
		Dealer: "Concessionario", Vehicle: "Veicolo", Plate: "Targa", Services: "Servizi", Part: "Parte", Count: "Quantità",
		Product: "Prodotto", Items: "Righe", Pieces: "Pezzi", Meters: "Metri", Ends: "Scadenza",
		ServiceAmount: "Importo dei servizi", Payments: "Pagamenti", Balance: "Saldo", Empty: "Nessuna registrazione in questo periodo.",
		MailSubject: "Report della flotta {fleet}: {period}",
		MailBody:    "Buongiorno,\n\nin allegato trova il report della flotta {fleet} relativo a {period}.",
		MailLink:    "Il file del report è troppo grande per un allegato. Può scaricarlo dal portale: {url}",
	},
	i18n.LocaleZhCN: {
		Months:  [12]string{"1月", "2月", "3月", "4月", "5月", "6月", "7月", "8月", "9月", "10月", "11月", "12月"},
		Quarter: "{year}年第{q}季度", DateLayout: "2006-01-02",
		Dealer: "经销商", Vehicle: "车辆", Plate: "车牌", Services: "服务", Part: "部位", Count: "数量",
		Product: "产品", Items: "条目", Pieces: "件数", Meters: "米数", Ends: "到期",
		ServiceAmount: "服务金额", Payments: "付款", Balance: "余额", Empty: "本期没有记录。",
		MailSubject: "{fleet} 车队报告：{period}",
		MailBody:    "您好，\n\n附件是 {fleet} 车队 {period} 的报告。",
		MailLink:    "报告文件过大，无法作为附件发送。您可以在门户中下载：{url}",
	},
	i18n.LocaleAZ: {
		Months:  [12]string{"Yanvar", "Fevral", "Mart", "Aprel", "May", "İyun", "İyul", "Avqust", "Sentyabr", "Oktyabr", "Noyabr", "Dekabr"},
		Quarter: "{year}, {q}-cü rüb", DateLayout: "02.01.2006",
		Dealer: "Diler", Vehicle: "Avtomobil", Plate: "Nömrə nişanı", Services: "Xidmətlər", Part: "Hissə", Count: "Say",
		Product: "Məhsul", Items: "Sətirlər", Pieces: "Ədəd", Meters: "Metr", Ends: "Bitmə tarixi",
		ServiceAmount: "Xidmət məbləği", Payments: "Ödənişlər", Balance: "Qalıq", Empty: "Bu dövrdə qeyd yoxdur.",
		MailSubject: "{fleet} avtopark hesabatı: {period}",
		MailBody:    "Salam,\n\n{fleet} avtoparkının {period} dövrü üzrə hesabatı əlavədədir.",
		MailLink:    "Hesabat faylı e-poçt əlavəsi üçün çox böyükdür. Onu portaldan yükləyə bilərsiniz: {url}",
	},
	i18n.LocaleAR: {
		Months:  [12]string{"يناير", "فبراير", "مارس", "أبريل", "مايو", "يونيو", "يوليو", "أغسطس", "سبتمبر", "أكتوبر", "نوفمبر", "ديسمبر"},
		Quarter: "الربع {q} من {year}", DateLayout: "2006-01-02",
		Dealer: "الوكيل", Vehicle: "المركبة", Plate: "رقم اللوحة", Services: "الخدمات", Part: "الجزء", Count: "العدد",
		Product: "المنتج", Items: "البنود", Pieces: "القطع", Meters: "الأمتار", Ends: "ينتهي في",
		ServiceAmount: "مبلغ الخدمات", Payments: "المدفوعات", Balance: "الرصيد", Empty: "لا توجد سجلات في هذه الفترة.",
		MailSubject: "تقرير الأسطول {fleet}: {period}",
		MailBody:    "مرحبًا،\n\nمرفق تقرير أسطول {fleet} عن الفترة {period}.",
		MailLink:    "ملف التقرير كبير جدًا لإرفاقه بالبريد. يمكنك تنزيله من البوابة: {url}",
	},
}

// reportLabelsFor returns the labels of a report locale (fleet_reports
// stores "zh-CN"); an unknown locale reads as the default (tr).
func reportLabelsFor(locale string) (reportLabels, i18n.Locale) {
	l := i18n.Normalize(locale)
	if t, ok := reportTexts[l]; ok {
		return t, l
	}
	return reportTexts[i18n.FallbackLocale], i18n.FallbackLocale
}

// fill replaces {key} placeholders.
func fill(s string, vars map[string]string) string {
	for k, v := range vars {
		s = strings.ReplaceAll(s, "{"+k+"}", v)
	}
	return s
}
