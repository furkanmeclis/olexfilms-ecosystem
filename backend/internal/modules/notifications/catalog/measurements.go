package catalog

import "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/msgtemplate"

// EventMeasurementDiffCheckRequired tells dealer owners that the service's
// before/after micron difference needs review (TEC-297).
const EventMeasurementDiffCheckRequired = "MEASUREMENT_DIFF_CHECK_REQUIRED"

var measurementDiffTexts = map[string]localizedText{
	"tr":    {"Ölçüm kontrolü gerekli", "{{service_no}} hizmetinde {{deviation_count}} parçada mikron farkı tolerans dışında. Ölçüm fark tablosunu kontrol edin."},
	"en":    {"Measurement check required", "Service {{service_no}} has micron differences outside tolerance on {{deviation_count}} part(s). Please review the measurement diff table."},
	"bg":    {"Необходима е проверка на измерването", "Услуга {{service_no}} има разлики в микрони извън толеранса за {{deviation_count}} части. Проверете таблицата с разликите."},
	"de":    {"Messung prüfen", "Service {{service_no}} hat bei {{deviation_count}} Teil(en) Mikronabweichungen außerhalb der Toleranz. Bitte prüfen Sie die Differenztabelle."},
	"el":    {"Απαιτείται έλεγχος μέτρησης", "Η υπηρεσία {{service_no}} έχει αποκλίσεις μικρών εκτός ανοχής σε {{deviation_count}} μέρη. Ελέγξτε τον πίνακα διαφορών."},
	"uk":    {"Потрібна перевірка вимірювання", "У сервісі {{service_no}} є відхилення мікронів поза допуском для {{deviation_count}} деталей. Перевірте таблицю різниць."},
	"ru":    {"Требуется проверка измерения", "В услуге {{service_no}} есть отклонения микронов вне допуска по {{deviation_count}} деталям. Проверьте таблицу разниц."},
	"fr":    {"Contrôle de mesure requis", "Le service {{service_no}} présente des écarts en microns hors tolérance sur {{deviation_count}} pièce(s). Vérifiez le tableau des écarts."},
	"es":    {"Revisión de medición requerida", "El servicio {{service_no}} tiene diferencias de micras fuera de tolerancia en {{deviation_count}} pieza(s). Revise la tabla de diferencias."},
	"it":    {"Controllo misura richiesto", "Il servizio {{service_no}} ha differenze in micron fuori tolleranza su {{deviation_count}} parte/i. Controlla la tabella differenze."},
	"zh-CN": {"需要检查测量", "服务 {{service_no}} 有 {{deviation_count}} 个部位的微米差异超出容差。请检查测量差异表。"},
	"az":    {"Ölçüm yoxlaması tələb olunur", "{{service_no}} xidmətində {{deviation_count}} hissədə mikron fərqi tolerantlıqdan kənardır. Ölçüm fərqi cədvəlini yoxlayın."},
	"ar":    {"يلزم فحص القياس", "الخدمة {{service_no}} لديها فروقات ميكرون خارج حدود السماحية في {{deviation_count}} جزء. يرجى مراجعة جدول الفروقات."},
}

func measurementDiffTemplates() []DefaultTemplate {
	out := make([]DefaultTemplate, 0, len(msgtemplate.Locales))
	for _, lang := range msgtemplate.Locales {
		t, ok := measurementDiffTexts[lang]
		if !ok {
			continue
		}
		out = append(out, DefaultTemplate{
			Role: RoleDealer, Channel: ChannelInapp, Language: lang,
			Subject: t.subject, Body: t.body, Format: "text",
		})
	}
	return out
}

func init() {
	Register(Event{
		Code: EventMeasurementDiffCheckRequired, Module: "measurements",
		DefaultChannels: []string{ChannelInapp},
		AudienceRoles:   []string{RoleDealer},
		Placeholders: []msgtemplate.Placeholder{
			ph("service_no", "DS00001234", "DS00001234"),
			ph("deviation_count", "2", "2"),
		},
		UserConfigurable: true,
		Templates:        measurementDiffTemplates(),
	})
}
