package catalog

import "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/msgtemplate"

const (
	EventAppointmentCreated   = "APPOINTMENT_CREATED"
	EventAppointmentCancelled = "APPOINTMENT_CANCELLED"
	EventAppointmentReminder  = "APPOINTMENT_REMINDER"
)

var AppointmentChannels = []string{ChannelWhatsApp}

var appointmentCreatedTexts = map[string]localizedText{
	"tr":    {"Randevunuz oluşturuldu", "Merhaba, {{organization_name}} randevunuz {{starts_at}} için oluşturuldu. Araç: {{plate}}"},
	"en":    {"Appointment created", "Hello, your appointment with {{organization_name}} is set for {{starts_at}}. Vehicle: {{plate}}"},
	"bg":    {"Записан час", "Здравейте, часът ви при {{organization_name}} е за {{starts_at}}. Автомобил: {{plate}}"},
	"de":    {"Termin erstellt", "Hallo, Ihr Termin bei {{organization_name}} ist für {{starts_at}} geplant. Fahrzeug: {{plate}}"},
	"el":    {"Το ραντεβού δημιουργήθηκε", "Γεια σας, το ραντεβού σας με {{organization_name}} ορίστηκε για {{starts_at}}. Όχημα: {{plate}}"},
	"uk":    {"Запис створено", "Вітаємо, ваш візит до {{organization_name}} заплановано на {{starts_at}}. Авто: {{plate}}"},
	"ru":    {"Запись создана", "Здравствуйте, ваш визит в {{organization_name}} назначен на {{starts_at}}. Авто: {{plate}}"},
	"fr":    {"Rendez-vous créé", "Bonjour, votre rendez-vous avec {{organization_name}} est prévu le {{starts_at}}. Véhicule : {{plate}}"},
	"es":    {"Cita creada", "Hola, su cita con {{organization_name}} está programada para {{starts_at}}. Vehículo: {{plate}}"},
	"it":    {"Appuntamento creato", "Salve, l'appuntamento con {{organization_name}} è fissato per {{starts_at}}. Veicolo: {{plate}}"},
	"zh-CN": {"预约已创建", "您好，您在 {{organization_name}} 的预约时间为 {{starts_at}}。车辆：{{plate}}"},
	"az":    {"Görüş yaradıldı", "Salam, {{organization_name}} görüşünüz {{starts_at}} vaxtına təyin edildi. Avtomobil: {{plate}}"},
	"ar":    {"تم إنشاء الموعد", "مرحبًا، تم تحديد موعدك لدى {{organization_name}} في {{starts_at}}. المركبة: {{plate}}"},
}

var appointmentCancelledTexts = map[string]localizedText{
	"tr":    {"Randevunuz iptal edildi", "Merhaba, {{organization_name}} randevunuz iptal edildi. Saat: {{starts_at}} Araç: {{plate}}"},
	"en":    {"Appointment cancelled", "Hello, your appointment with {{organization_name}} has been cancelled. Time: {{starts_at}} Vehicle: {{plate}}"},
	"bg":    {"Часът е отменен", "Здравейте, часът ви при {{organization_name}} е отменен. Час: {{starts_at}} Автомобил: {{plate}}"},
	"de":    {"Termin storniert", "Hallo, Ihr Termin bei {{organization_name}} wurde storniert. Zeit: {{starts_at}} Fahrzeug: {{plate}}"},
	"el":    {"Το ραντεβού ακυρώθηκε", "Γεια σας, το ραντεβού σας με {{organization_name}} ακυρώθηκε. Ώρα: {{starts_at}} Όχημα: {{plate}}"},
	"uk":    {"Запис скасовано", "Вітаємо, ваш візит до {{organization_name}} скасовано. Час: {{starts_at}} Авто: {{plate}}"},
	"ru":    {"Запись отменена", "Здравствуйте, ваш визит в {{organization_name}} отменен. Время: {{starts_at}} Авто: {{plate}}"},
	"fr":    {"Rendez-vous annulé", "Bonjour, votre rendez-vous avec {{organization_name}} a été annulé. Heure : {{starts_at}} Véhicule : {{plate}}"},
	"es":    {"Cita cancelada", "Hola, su cita con {{organization_name}} fue cancelada. Hora: {{starts_at}} Vehículo: {{plate}}"},
	"it":    {"Appuntamento annullato", "Salve, l'appuntamento con {{organization_name}} è stato annullato. Ora: {{starts_at}} Veicolo: {{plate}}"},
	"zh-CN": {"预约已取消", "您好，您在 {{organization_name}} 的预约已取消。时间：{{starts_at}} 车辆：{{plate}}"},
	"az":    {"Görüş ləğv edildi", "Salam, {{organization_name}} görüşünüz ləğv edildi. Vaxt: {{starts_at}} Avtomobil: {{plate}}"},
	"ar":    {"تم إلغاء الموعد", "مرحبًا، تم إلغاء موعدك لدى {{organization_name}}. الوقت: {{starts_at}} المركبة: {{plate}}"},
}

var appointmentReminderTexts = map[string]localizedText{
	"tr":    {"Randevu hatırlatması", "Merhaba, {{organization_name}} randevunuz {{starts_at}} için yaklaşıyor. Araç: {{plate}}"},
	"en":    {"Appointment reminder", "Hello, your appointment with {{organization_name}} is coming up at {{starts_at}}. Vehicle: {{plate}}"},
	"bg":    {"Напомняне за час", "Здравейте, часът ви при {{organization_name}} наближава: {{starts_at}}. Автомобил: {{plate}}"},
	"de":    {"Terminerinnerung", "Hallo, Ihr Termin bei {{organization_name}} steht bevor: {{starts_at}}. Fahrzeug: {{plate}}"},
	"el":    {"Υπενθύμιση ραντεβού", "Γεια σας, το ραντεβού σας με {{organization_name}} πλησιάζει: {{starts_at}}. Όχημα: {{plate}}"},
	"uk":    {"Нагадування про візит", "Вітаємо, ваш візит до {{organization_name}} наближається: {{starts_at}}. Авто: {{plate}}"},
	"ru":    {"Напоминание о визите", "Здравствуйте, ваш визит в {{organization_name}} скоро: {{starts_at}}. Авто: {{plate}}"},
	"fr":    {"Rappel de rendez-vous", "Bonjour, votre rendez-vous avec {{organization_name}} approche : {{starts_at}}. Véhicule : {{plate}}"},
	"es":    {"Recordatorio de cita", "Hola, su cita con {{organization_name}} se acerca: {{starts_at}}. Vehículo: {{plate}}"},
	"it":    {"Promemoria appuntamento", "Salve, l'appuntamento con {{organization_name}} si avvicina: {{starts_at}}. Veicolo: {{plate}}"},
	"zh-CN": {"预约提醒", "您好，您在 {{organization_name}} 的预约即将开始：{{starts_at}}。车辆：{{plate}}"},
	"az":    {"Görüş xatırlatması", "Salam, {{organization_name}} görüşünüz yaxınlaşır: {{starts_at}}. Avtomobil: {{plate}}"},
	"ar":    {"تذكير بالموعد", "مرحبًا، موعدك لدى {{organization_name}} يقترب: {{starts_at}}. المركبة: {{plate}}"},
}

func appointmentTemplates(texts map[string]localizedText) []DefaultTemplate {
	out := make([]DefaultTemplate, 0, len(texts)*len(AppointmentChannels))
	for _, lang := range msgtemplate.Locales {
		t, ok := texts[lang]
		if !ok {
			continue
		}
		for _, ch := range AppointmentChannels {
			out = append(out, DefaultTemplate{
				Role: RoleGeneric, Channel: ch, Language: lang,
				Subject: t.subject, Body: t.body, Format: "text",
			})
		}
	}
	return out
}

func appointmentPlaceholders() []msgtemplate.Placeholder {
	return []msgtemplate.Placeholder{
		ph("organization_name", "Tech Oto", "Tech Oto"),
		ph("starts_at", "2026-10-06 09:00", "2026-10-06 09:00"),
		ph("plate", "34 ABC 123", "34 ABC 123"),
	}
}

func init() {
	for _, ev := range []Event{
		{
			Code: EventAppointmentCreated, Module: "appointments",
			DefaultChannels: AppointmentChannels, AudienceRoles: []string{RoleCustomer},
			Placeholders: appointmentPlaceholders(), UserConfigurable: true,
			Templates: appointmentTemplates(appointmentCreatedTexts),
		},
		{
			Code: EventAppointmentCancelled, Module: "appointments",
			DefaultChannels: AppointmentChannels, AudienceRoles: []string{RoleCustomer},
			Placeholders: appointmentPlaceholders(), UserConfigurable: true,
			Templates: appointmentTemplates(appointmentCancelledTexts),
		},
		{
			Code: EventAppointmentReminder, Module: "appointments",
			DefaultChannels: AppointmentChannels, AudienceRoles: []string{RoleCustomer},
			Placeholders: appointmentPlaceholders(), UserConfigurable: true,
			Templates: appointmentTemplates(appointmentReminderTexts),
		},
	} {
		Register(ev)
	}
}
