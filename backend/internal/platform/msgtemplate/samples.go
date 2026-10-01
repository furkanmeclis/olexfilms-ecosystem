package msgtemplate

import "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"

// localizedSamples are preview values for placeholders in the locales beyond
// tr/en (TEC-138), keyed by placeholder key. Placeholder.Sample uses them
// before falling back to the en sample; values that are the same everywhere
// (plates, links, job ids, currency codes) are not listed.
var localizedSamples = map[string]map[i18n.Locale]string{
	"customer_name": {
		i18n.LocaleDE: "Max Mustermann", i18n.LocaleFR: "Jean Dupont", i18n.LocaleES: "Juan García",
		i18n.LocaleIT: "Mario Rossi", i18n.LocaleRU: "Иван Петров", i18n.LocaleUK: "Олег Коваленко",
	},
	"name": {
		i18n.LocaleDE: "Max Mustermann", i18n.LocaleFR: "Jean Dupont", i18n.LocaleES: "Juan García",
		i18n.LocaleIT: "Mario Rossi", i18n.LocaleRU: "Иван Петров", i18n.LocaleUK: "Олег Коваленко",
	},
	"created_by_name": {
		i18n.LocaleDE: "Lukas Schmidt", i18n.LocaleFR: "Paul Martin", i18n.LocaleES: "Carlos López",
		i18n.LocaleIT: "Luca Bianchi", i18n.LocaleRU: "Сергей Смирнов", i18n.LocaleUK: "Андрій Бондаренко",
	},
	"assignee_name": {
		i18n.LocaleDE: "Anna Müller", i18n.LocaleFR: "Marie Durand", i18n.LocaleES: "Laura Martínez",
		i18n.LocaleIT: "Giulia Romano", i18n.LocaleRU: "Анна Кузнецова", i18n.LocaleUK: "Олена Шевченко",
	},
	"amount": {
		i18n.LocaleDE: "1.250,00", i18n.LocaleFR: "1 250,00", i18n.LocaleES: "1250,00",
		i18n.LocaleIT: "1.250,00", i18n.LocaleRU: "1 250,00", i18n.LocaleUK: "1 250,00",
	},
	"total_amount": {
		i18n.LocaleDE: "12.500,00 TRY", i18n.LocaleFR: "12 500,00 TRY", i18n.LocaleES: "12.500,00 TRY",
		i18n.LocaleIT: "12.500,00 TRY", i18n.LocaleRU: "12 500,00 TRY", i18n.LocaleUK: "12 500,00 TRY",
	},
	"contract_title": {
		i18n.LocaleDE: "Dienstleistungsvertrag", i18n.LocaleFR: "Contrat de prestation", i18n.LocaleES: "Contrato de servicio",
		i18n.LocaleIT: "Contratto di servizio", i18n.LocaleRU: "Договор на оказание услуг", i18n.LocaleUK: "Договір про надання послуг",
	},
	"valid_until": {
		i18n.LocaleDE: "30.09.2026", i18n.LocaleFR: "30/09/2026", i18n.LocaleES: "30/09/2026",
		i18n.LocaleIT: "30/09/2026", i18n.LocaleRU: "30.09.2026", i18n.LocaleUK: "30.09.2026",
	},
	"due_at": {
		i18n.LocaleDE: "25.09.2026 14:30", i18n.LocaleFR: "25/09/2026 14:30", i18n.LocaleES: "25/09/2026 14:30",
		i18n.LocaleIT: "25/09/2026 14:30", i18n.LocaleRU: "25.09.2026 14:30", i18n.LocaleUK: "25.09.2026 14:30",
	},
	"due_in": {
		i18n.LocaleDE: "in 15 Minuten", i18n.LocaleFR: "dans 15 minutes", i18n.LocaleES: "en 15 minutos",
		i18n.LocaleIT: "tra 15 minuti", i18n.LocaleRU: "через 15 минут", i18n.LocaleUK: "через 15 хвилин",
	},
	"todo_title": {
		i18n.LocaleDE: "Keramikversiegelung prüfen 34 ABC 123", i18n.LocaleFR: "Contrôle du revêtement céramique 34 ABC 123",
		i18n.LocaleES: "Revisión del recubrimiento cerámico 34 ABC 123", i18n.LocaleIT: "Controllo trattamento ceramico 34 ABC 123",
		i18n.LocaleRU: "Проверка керамического покрытия 34 ABC 123", i18n.LocaleUK: "Перевірка керамічного покриття 34 ABC 123",
	},
	"todo_notes": {
		i18n.LocaleDE: "Kunden anrufen.", i18n.LocaleFR: "Appeler le client.", i18n.LocaleES: "Llamar al cliente.",
		i18n.LocaleIT: "Chiamare il cliente.", i18n.LocaleRU: "Позвонить клиенту.", i18n.LocaleUK: "Зателефонувати клієнту.",
	},
	"note": {
		i18n.LocaleDE: "Bitte freischalten.", i18n.LocaleFR: "Merci de l'activer.", i18n.LocaleES: "Por favor, actívelo.",
		i18n.LocaleIT: "Per favore, attivatelo.", i18n.LocaleRU: "Пожалуйста, включите.", i18n.LocaleUK: "Будь ласка, увімкніть.",
	},
	"intent": {
		i18n.LocaleDE: "Preis", i18n.LocaleFR: "tarif", i18n.LocaleES: "precio",
		i18n.LocaleIT: "prezzo", i18n.LocaleRU: "цена", i18n.LocaleUK: "ціна",
	},
}

// localizedSample returns the preview value of key for locale, if any.
func localizedSample(key string, locale i18n.Locale) (string, bool) {
	v, ok := localizedSamples[key][locale]
	return v, ok && v != ""
}
