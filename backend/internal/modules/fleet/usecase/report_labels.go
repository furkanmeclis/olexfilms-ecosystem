package usecase

import (
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
)

// reportLabels are the texts of the fleet report tables in one language
// (TEC-476). The headings around the tables live in the fleet_report
// document template (one per language, editable); the e-mail texts are the
// i18n email.fleet_report.* keys (TEC-1235). Quarter uses {q} and {year}.
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
}

var reportTexts = map[i18n.Locale]reportLabels{
	i18n.LocaleTR: {
		Months:  [12]string{"Ocak", "Şubat", "Mart", "Nisan", "Mayıs", "Haziran", "Temmuz", "Ağustos", "Eylül", "Ekim", "Kasım", "Aralık"},
		Quarter: "{year} {q}. çeyrek", DateLayout: "02.01.2006",
		Dealer: "Bayi", Vehicle: "Araç", Plate: "Plaka", Services: "Hizmet", Part: "Parça", Count: "Adet",
		Product: "Ürün", Items: "Kalem", Pieces: "Parça adedi", Meters: "Metre", Ends: "Bitiş",
		ServiceAmount: "Hizmet tutarı", Payments: "Ödemeler", Balance: "Bakiye", Empty: "Bu dönemde kayıt yok.",
	},
	i18n.LocaleEN: {
		Months:  [12]string{"January", "February", "March", "April", "May", "June", "July", "August", "September", "October", "November", "December"},
		Quarter: "Q{q} {year}", DateLayout: "2006-01-02",
		Dealer: "Dealer", Vehicle: "Vehicle", Plate: "Plate", Services: "Services", Part: "Part", Count: "Count",
		Product: "Product", Items: "Items", Pieces: "Pieces", Meters: "Meters", Ends: "Ends",
		ServiceAmount: "Service amount", Payments: "Payments", Balance: "Balance", Empty: "No records in this period.",
	},
	i18n.LocaleBG: {
		Months:  [12]string{"януари", "февруари", "март", "април", "май", "юни", "юли", "август", "септември", "октомври", "ноември", "декември"},
		Quarter: "{q}. тримесечие {year}", DateLayout: "02.01.2006",
		Dealer: "Дилър", Vehicle: "Автомобил", Plate: "Рег. номер", Services: "Услуги", Part: "Част", Count: "Брой",
		Product: "Продукт", Items: "Позиции", Pieces: "Бройки", Meters: "Метри", Ends: "Изтича",
		ServiceAmount: "Сума за услуги", Payments: "Плащания", Balance: "Салдо", Empty: "Няма записи за този период.",
	},
	i18n.LocaleDE: {
		Months:  [12]string{"Januar", "Februar", "März", "April", "Mai", "Juni", "Juli", "August", "September", "Oktober", "November", "Dezember"},
		Quarter: "{q}. Quartal {year}", DateLayout: "02.01.2006",
		Dealer: "Händler", Vehicle: "Fahrzeug", Plate: "Kennzeichen", Services: "Dienstleistungen", Part: "Bauteil", Count: "Anzahl",
		Product: "Produkt", Items: "Positionen", Pieces: "Stück", Meters: "Meter", Ends: "Endet am",
		ServiceAmount: "Leistungsbetrag", Payments: "Zahlungen", Balance: "Saldo", Empty: "Keine Einträge in diesem Zeitraum.",
	},
	i18n.LocaleEL: {
		Months:  [12]string{"Ιανουάριος", "Φεβρουάριος", "Μάρτιος", "Απρίλιος", "Μάιος", "Ιούνιος", "Ιούλιος", "Αύγουστος", "Σεπτέμβριος", "Οκτώβριος", "Νοέμβριος", "Δεκέμβριος"},
		Quarter: "{q}ο τρίμηνο {year}", DateLayout: "02/01/2006",
		Dealer: "Αντιπρόσωπος", Vehicle: "Όχημα", Plate: "Πινακίδα", Services: "Υπηρεσίες", Part: "Τμήμα", Count: "Πλήθος",
		Product: "Προϊόν", Items: "Γραμμές", Pieces: "Τεμάχια", Meters: "Μέτρα", Ends: "Λήξη",
		ServiceAmount: "Ποσό υπηρεσιών", Payments: "Πληρωμές", Balance: "Υπόλοιπο", Empty: "Δεν υπάρχουν εγγραφές σε αυτή την περίοδο.",
	},
	i18n.LocaleUK: {
		Months:  [12]string{"січень", "лютий", "березень", "квітень", "травень", "червень", "липень", "серпень", "вересень", "жовтень", "листопад", "грудень"},
		Quarter: "{q} квартал {year}", DateLayout: "02.01.2006",
		Dealer: "Дилер", Vehicle: "Автомобіль", Plate: "Номерний знак", Services: "Послуги", Part: "Деталь", Count: "Кількість",
		Product: "Продукт", Items: "Позиції", Pieces: "Штуки", Meters: "Метри", Ends: "Закінчується",
		ServiceAmount: "Сума послуг", Payments: "Платежі", Balance: "Сальдо", Empty: "За цей період записів немає.",
	},
	i18n.LocaleRU: {
		Months:  [12]string{"январь", "февраль", "март", "апрель", "май", "июнь", "июль", "август", "сентябрь", "октябрь", "ноябрь", "декабрь"},
		Quarter: "{q} квартал {year}", DateLayout: "02.01.2006",
		Dealer: "Дилер", Vehicle: "Автомобиль", Plate: "Госномер", Services: "Услуги", Part: "Деталь", Count: "Количество",
		Product: "Продукт", Items: "Позиции", Pieces: "Штуки", Meters: "Метры", Ends: "Окончание",
		ServiceAmount: "Сумма услуг", Payments: "Платежи", Balance: "Сальдо", Empty: "За этот период записей нет.",
	},
	i18n.LocaleFR: {
		Months:  [12]string{"janvier", "février", "mars", "avril", "mai", "juin", "juillet", "août", "septembre", "octobre", "novembre", "décembre"},
		Quarter: "{q}e trimestre {year}", DateLayout: "02/01/2006",
		Dealer: "Concessionnaire", Vehicle: "Véhicule", Plate: "Immatriculation", Services: "Prestations", Part: "Pièce", Count: "Nombre",
		Product: "Produit", Items: "Lignes", Pieces: "Pièces", Meters: "Mètres", Ends: "Fin",
		ServiceAmount: "Montant des prestations", Payments: "Paiements", Balance: "Solde", Empty: "Aucun enregistrement sur cette période.",
	},
	i18n.LocaleES: {
		Months:  [12]string{"enero", "febrero", "marzo", "abril", "mayo", "junio", "julio", "agosto", "septiembre", "octubre", "noviembre", "diciembre"},
		Quarter: "{q}.º trimestre de {year}", DateLayout: "02/01/2006",
		Dealer: "Concesionario", Vehicle: "Vehículo", Plate: "Matrícula", Services: "Servicios", Part: "Pieza", Count: "Cantidad",
		Product: "Producto", Items: "Líneas", Pieces: "Unidades", Meters: "Metros", Ends: "Vence",
		ServiceAmount: "Importe de servicios", Payments: "Pagos", Balance: "Saldo", Empty: "No hay registros en este periodo.",
	},
	i18n.LocaleIT: {
		Months:  [12]string{"gennaio", "febbraio", "marzo", "aprile", "maggio", "giugno", "luglio", "agosto", "settembre", "ottobre", "novembre", "dicembre"},
		Quarter: "{q}° trimestre {year}", DateLayout: "02/01/2006",
		Dealer: "Concessionario", Vehicle: "Veicolo", Plate: "Targa", Services: "Servizi", Part: "Parte", Count: "Quantità",
		Product: "Prodotto", Items: "Righe", Pieces: "Pezzi", Meters: "Metri", Ends: "Scadenza",
		ServiceAmount: "Importo dei servizi", Payments: "Pagamenti", Balance: "Saldo", Empty: "Nessuna registrazione in questo periodo.",
	},
	i18n.LocaleZhCN: {
		Months:  [12]string{"1月", "2月", "3月", "4月", "5月", "6月", "7月", "8月", "9月", "10月", "11月", "12月"},
		Quarter: "{year}年第{q}季度", DateLayout: "2006-01-02",
		Dealer: "经销商", Vehicle: "车辆", Plate: "车牌", Services: "服务", Part: "部位", Count: "数量",
		Product: "产品", Items: "条目", Pieces: "件数", Meters: "米数", Ends: "到期",
		ServiceAmount: "服务金额", Payments: "付款", Balance: "余额", Empty: "本期没有记录。",
	},
	i18n.LocaleAZ: {
		Months:  [12]string{"Yanvar", "Fevral", "Mart", "Aprel", "May", "İyun", "İyul", "Avqust", "Sentyabr", "Oktyabr", "Noyabr", "Dekabr"},
		Quarter: "{year}, {q}-cü rüb", DateLayout: "02.01.2006",
		Dealer: "Diler", Vehicle: "Avtomobil", Plate: "Nömrə nişanı", Services: "Xidmətlər", Part: "Hissə", Count: "Say",
		Product: "Məhsul", Items: "Sətirlər", Pieces: "Ədəd", Meters: "Metr", Ends: "Bitmə tarixi",
		ServiceAmount: "Xidmət məbləği", Payments: "Ödənişlər", Balance: "Qalıq", Empty: "Bu dövrdə qeyd yoxdur.",
	},
	i18n.LocaleAR: {
		Months:  [12]string{"يناير", "فبراير", "مارس", "أبريل", "مايو", "يونيو", "يوليو", "أغسطس", "سبتمبر", "أكتوبر", "نوفمبر", "ديسمبر"},
		Quarter: "الربع {q} من {year}", DateLayout: "2006-01-02",
		Dealer: "الوكيل", Vehicle: "المركبة", Plate: "رقم اللوحة", Services: "الخدمات", Part: "الجزء", Count: "العدد",
		Product: "المنتج", Items: "البنود", Pieces: "القطع", Meters: "الأمتار", Ends: "ينتهي في",
		ServiceAmount: "مبلغ الخدمات", Payments: "المدفوعات", Balance: "الرصيد", Empty: "لا توجد سجلات في هذه الفترة.",
	},
	i18n.LocaleFA: {
		Months:  [12]string{"ژانویه", "فوریه", "مارس", "آوریل", "مه", "ژوئن", "ژوئیه", "اوت", "سپتامبر", "اکتبر", "نوامبر", "دسامبر"},
		Quarter: "فصل {q} سال {year}", DateLayout: "2006-01-02",
		Dealer: "نمایندگی", Vehicle: "خودرو", Plate: "پلاک", Services: "سرویس\u200cها", Part: "قطعه", Count: "تعداد",
		Product: "محصول", Items: "اقلام", Pieces: "عدد", Meters: "متراژ", Ends: "پایان",
		ServiceAmount: "مبلغ سرویس\u200cها", Payments: "پرداخت\u200cها", Balance: "مانده", Empty: "در این دوره رکوردی وجود ندارد.",
	},
	i18n.LocaleHI: {
		Months:  [12]string{"जनवरी", "फ़रवरी", "मार्च", "अप्रैल", "मई", "जून", "जुलाई", "अगस्त", "सितंबर", "अक्टूबर", "नवंबर", "दिसंबर"},
		Quarter: "तिमाही {q}, {year}", DateLayout: "02-01-2006",
		Dealer: "डीलर", Vehicle: "वाहन", Plate: "नंबर प्लेट", Services: "सर्विस", Part: "भाग", Count: "संख्या",
		Product: "उत्पाद", Items: "मदें", Pieces: "नग", Meters: "मीटर", Ends: "समाप्ति",
		ServiceAmount: "सर्विस राशि", Payments: "भुगतान", Balance: "शेष राशि", Empty: "इस अवधि में कोई रिकॉर्ड नहीं है।",
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
