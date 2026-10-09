package catalog

import "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/msgtemplate"

// EventFeaturesModuleRequestDecided tells the requester that a module request
// was approved (by the level above, or automatically because the module
// opened some other way) or rejected (TEC-508).
const EventFeaturesModuleRequestDecided = "features.module_request_decided"

// ModuleRequestChannels are the default channels of the decision event.
var ModuleRequestChannels = []string{ChannelInapp, ChannelEmail}

var moduleRequestDecidedTexts = map[string]localizedText{
	"tr":    {"Modül talebi sonuçlandı: {{module_key}}", "{{organization_name}} için {{module_key}} modül talebi: {{decision}}. {{decision_note}}"},
	"en":    {"Module request decided: {{module_key}}", "The {{module_key}} module request of {{organization_name}}: {{decision}}. {{decision_note}}"},
	"bg":    {"Заявката за модул е решена: {{module_key}}", "Заявката за модул {{module_key}} на {{organization_name}}: {{decision}}. {{decision_note}}"},
	"de":    {"Modulanfrage entschieden: {{module_key}}", "Die Anfrage für das Modul {{module_key}} von {{organization_name}}: {{decision}}. {{decision_note}}"},
	"el":    {"Το αίτημα λειτουργίας εξετάστηκε: {{module_key}}", "Το αίτημα για τη λειτουργία {{module_key}} του οργανισμού {{organization_name}}: {{decision}}. {{decision_note}}"},
	"uk":    {"Запит на модуль розглянуто: {{module_key}}", "Запит на модуль {{module_key}} для {{organization_name}}: {{decision}}. {{decision_note}}"},
	"ru":    {"Запрос на модуль рассмотрен: {{module_key}}", "Запрос на модуль {{module_key}} для {{organization_name}}: {{decision}}. {{decision_note}}"},
	"fr":    {"Demande de module traitée : {{module_key}}", "La demande du module {{module_key}} pour {{organization_name}} : {{decision}}. {{decision_note}}"},
	"es":    {"Solicitud de módulo resuelta: {{module_key}}", "La solicitud del módulo {{module_key}} de {{organization_name}}: {{decision}}. {{decision_note}}"},
	"it":    {"Richiesta di modulo decisa: {{module_key}}", "La richiesta del modulo {{module_key}} per {{organization_name}}: {{decision}}. {{decision_note}}"},
	"zh-CN": {"模块申请已处理：{{module_key}}", "{{organization_name}} 的 {{module_key}} 模块申请：{{decision}}。{{decision_note}}"},
	"az":    {"Modul sorğusu üzrə qərar verildi: {{module_key}}", "{{organization_name}} üçün {{module_key}} modul sorğusu: {{decision}}. {{decision_note}}"},
	"ar":    {"تم البت في طلب الوحدة: {{module_key}}", "طلب الوحدة {{module_key}} الخاص بـ {{organization_name}}: {{decision}}. {{decision_note}}"},
}

// moduleRequestDecisionLabels is the {{decision}} value per locale and
// status (approved, rejected).
var moduleRequestDecisionLabels = map[string][2]string{
	"tr":    {"onaylandı", "reddedildi"},
	"en":    {"approved", "rejected"},
	"bg":    {"одобрена", "отхвърлена"},
	"de":    {"genehmigt", "abgelehnt"},
	"el":    {"εγκρίθηκε", "απορρίφθηκε"},
	"uk":    {"схвалено", "відхилено"},
	"ru":    {"одобрен", "отклонён"},
	"fr":    {"approuvée", "refusée"},
	"es":    {"aprobada", "rechazada"},
	"it":    {"approvata", "respinta"},
	"zh-CN": {"已批准", "已拒绝"},
	"az":    {"təsdiqləndi", "rədd edildi"},
	"ar":    {"تمت الموافقة", "تم الرفض"},
}

// ModuleRequestDecisionLabel returns the localized {{decision}} value of a
// request status (approved or rejected), en when the locale is unknown.
func ModuleRequestDecisionLabel(locale, status string) string {
	l, ok := moduleRequestDecisionLabels[locale]
	if !ok {
		l = moduleRequestDecisionLabels["en"]
	}
	if status == "approved" {
		return l[0]
	}
	return l[1]
}

func moduleRequestTemplates() []DefaultTemplate {
	out := make([]DefaultTemplate, 0, len(moduleRequestDecidedTexts)*len(ModuleRequestChannels))
	for _, lang := range msgtemplate.Locales {
		t, ok := moduleRequestDecidedTexts[lang]
		if !ok {
			continue
		}
		for _, ch := range ModuleRequestChannels {
			out = append(out, DefaultTemplate{Role: RoleGeneric, Channel: ch, Language: lang, Subject: t.subject, Body: t.body, Format: "text"})
		}
	}
	return out
}

func init() {
	Register(Event{
		Code: EventFeaturesModuleRequestDecided, Module: "modules",
		DefaultChannels: ModuleRequestChannels,
		AudienceRoles:   []string{RoleDealer, RoleDistributor},
		Placeholders: []msgtemplate.Placeholder{
			ph("organization_name", "Tech Oto", "Tech Oto"),
			ph("module_key", "stock_forecast", "stock_forecast"),
			ph("decision", "onaylandı", "approved"),
			ph("decision_note", "Bu ay açıldı.", "Switched on this month."),
		},
		UserConfigurable: true,
		Templates:        moduleRequestTemplates(),
	})
}
