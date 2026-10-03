package catalog

import "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/msgtemplate"

// Center task notifications (TEC-221, F1-11b). tasks.assigned (the task use
// case) and tasks.due_soon / tasks.overdue (tasks:due_scan) carry the
// recipients in notify_user_ids; these catalog events reach center members,
// so the defaults are in-app and e-mail. Default templates ship in all 13
// locales and are seeded with InsertNotificationTemplateIfMissing by
// SyncCatalog (no migration).
const (
	EventTaskAssigned = "TASK_ASSIGNED"
	EventTaskDueSoon  = "TASK_DUE_SOON"
	EventTaskOverdue  = "TASK_OVERDUE"
)

// TaskChannels are the default channels of the task events.
var TaskChannels = []string{ChannelInapp, ChannelEmail}

// TaskEvents lists the task catalog events.
var TaskEvents = []string{EventTaskAssigned, EventTaskDueSoon, EventTaskOverdue}

var taskTexts = map[string]map[string]localizedText{
	EventTaskAssigned: {
		"tr": {"Size bir görev atandı: {{task_title}}",
			"{{subject_name}} ile ilgili \"{{task_title}}\" görevi size atandı. Ayrıntılar için paneli açın."},
		"en": {"A task was assigned to you: {{task_title}}",
			"The task \"{{task_title}}\" about {{subject_name}} was assigned to you. Open the panel for details."},
		"bg": {"Възложена ви е задача: {{task_title}}",
			"Задачата \"{{task_title}}\" относно {{subject_name}} ви е възложена. Отворете панела за подробности."},
		"de": {"Ihnen wurde eine Aufgabe zugewiesen: {{task_title}}",
			"Die Aufgabe \"{{task_title}}\" zu {{subject_name}} wurde Ihnen zugewiesen. Details finden Sie im Panel."},
		"el": {"Σας ανατέθηκε μια εργασία: {{task_title}}",
			"Η εργασία \"{{task_title}}\" για {{subject_name}} σας ανατέθηκε. Ανοίξτε τον πίνακα για λεπτομέρειες."},
		"uk": {"Вам призначено завдання: {{task_title}}",
			"Завдання \"{{task_title}}\" щодо {{subject_name}} призначено вам. Відкрийте панель, щоб переглянути деталі."},
		"ru": {"Вам назначена задача: {{task_title}}",
			"Задача \"{{task_title}}\" по {{subject_name}} назначена вам. Откройте панель, чтобы посмотреть детали."},
		"fr": {"Une tâche vous a été attribuée : {{task_title}}",
			"La tâche \"{{task_title}}\" concernant {{subject_name}} vous a été attribuée. Ouvrez le panneau pour les détails."},
		"es": {"Se le ha asignado una tarea: {{task_title}}",
			"Se le ha asignado la tarea \"{{task_title}}\" sobre {{subject_name}}. Abra el panel para ver los detalles."},
		"it": {"Le è stata assegnata un'attività: {{task_title}}",
			"L'attività \"{{task_title}}\" relativa a {{subject_name}} le è stata assegnata. Apra il pannello per i dettagli."},
		"zh-CN": {"您被分配了一项任务：{{task_title}}",
			"关于 {{subject_name}} 的任务“{{task_title}}”已分配给您。请打开面板查看详情。"},
		"az": {"Sizə tapşırıq təyin edildi: {{task_title}}",
			"{{subject_name}} ilə bağlı \"{{task_title}}\" tapşırığı sizə təyin edildi. Ətraflı məlumat üçün paneli açın."},
		"ar": {"تم إسناد مهمة إليك: {{task_title}}",
			"تم إسناد المهمة \"{{task_title}}\" المتعلقة بـ {{subject_name}} إليك. افتح اللوحة للاطلاع على التفاصيل."},
	},
	EventTaskDueSoon: {
		"tr": {"Görevin son tarihi yaklaşıyor: {{task_title}}",
			"{{subject_name}} ile ilgili \"{{task_title}}\" görevinin son tarihi {{due_date}}. Lütfen zamanında tamamlayın."},
		"en": {"Task due soon: {{task_title}}",
			"The task \"{{task_title}}\" about {{subject_name}} is due on {{due_date}}. Please complete it on time."},
		"bg": {"Срокът на задачата наближава: {{task_title}}",
			"Срокът на задачата \"{{task_title}}\" относно {{subject_name}} е {{due_date}}. Моля, завършете я навреме."},
		"de": {"Aufgabe bald fällig: {{task_title}}",
			"Die Aufgabe \"{{task_title}}\" zu {{subject_name}} ist am {{due_date}} fällig. Bitte erledigen Sie sie rechtzeitig."},
		"el": {"Η προθεσμία της εργασίας πλησιάζει: {{task_title}}",
			"Η εργασία \"{{task_title}}\" για {{subject_name}} λήγει στις {{due_date}}. Ολοκληρώστε την εγκαίρως."},
		"uk": {"Термін завдання наближається: {{task_title}}",
			"Термін завдання \"{{task_title}}\" щодо {{subject_name}} — {{due_date}}. Будь ласка, виконайте його вчасно."},
		"ru": {"Срок задачи приближается: {{task_title}}",
			"Срок задачи \"{{task_title}}\" по {{subject_name}} — {{due_date}}. Пожалуйста, выполните её вовремя."},
		"fr": {"Tâche bientôt échue : {{task_title}}",
			"La tâche \"{{task_title}}\" concernant {{subject_name}} arrive à échéance le {{due_date}}. Veuillez la terminer à temps."},
		"es": {"Tarea próxima a vencer: {{task_title}}",
			"La tarea \"{{task_title}}\" sobre {{subject_name}} vence el {{due_date}}. Complétela a tiempo."},
		"it": {"Attività in scadenza: {{task_title}}",
			"L'attività \"{{task_title}}\" relativa a {{subject_name}} scade il {{due_date}}. La completi in tempo."},
		"zh-CN": {"任务即将到期：{{task_title}}",
			"关于 {{subject_name}} 的任务“{{task_title}}”将于 {{due_date}} 到期。请按时完成。"},
		"az": {"Tapşırığın son tarixi yaxınlaşır: {{task_title}}",
			"{{subject_name}} ilə bağlı \"{{task_title}}\" tapşırığının son tarixi {{due_date}}. Zəhmət olmasa vaxtında tamamlayın."},
		"ar": {"اقترب موعد استحقاق المهمة: {{task_title}}",
			"موعد استحقاق المهمة \"{{task_title}}\" المتعلقة بـ {{subject_name}} هو {{due_date}}. يرجى إنجازها في الوقت المحدد."},
	},
	EventTaskOverdue: {
		"tr": {"Görevin son tarihi geçti: {{task_title}}",
			"{{subject_name}} ile ilgili \"{{task_title}}\" görevinin son tarihi ({{due_date}}) geçti ve görev hâlâ açık."},
		"en": {"Task overdue: {{task_title}}",
			"The task \"{{task_title}}\" about {{subject_name}} was due on {{due_date}} and is still open."},
		"bg": {"Срокът на задачата е изтекъл: {{task_title}}",
			"Срокът на задачата \"{{task_title}}\" относно {{subject_name}} беше {{due_date}} и тя все още е отворена."},
		"de": {"Aufgabe überfällig: {{task_title}}",
			"Die Aufgabe \"{{task_title}}\" zu {{subject_name}} war am {{due_date}} fällig und ist noch offen."},
		"el": {"Η εργασία έχει καθυστερήσει: {{task_title}}",
			"Η εργασία \"{{task_title}}\" για {{subject_name}} έληγε στις {{due_date}} και είναι ακόμη ανοιχτή."},
		"uk": {"Термін завдання минув: {{task_title}}",
			"Термін завдання \"{{task_title}}\" щодо {{subject_name}} був {{due_date}}, а воно досі відкрите."},
		"ru": {"Срок задачи истёк: {{task_title}}",
			"Срок задачи \"{{task_title}}\" по {{subject_name}} был {{due_date}}, а она всё ещё открыта."},
		"fr": {"Tâche en retard : {{task_title}}",
			"La tâche \"{{task_title}}\" concernant {{subject_name}} était due le {{due_date}} et est toujours ouverte."},
		"es": {"Tarea vencida: {{task_title}}",
			"La tarea \"{{task_title}}\" sobre {{subject_name}} venció el {{due_date}} y sigue abierta."},
		"it": {"Attività scaduta: {{task_title}}",
			"L'attività \"{{task_title}}\" relativa a {{subject_name}} scadeva il {{due_date}} ed è ancora aperta."},
		"zh-CN": {"任务已逾期：{{task_title}}",
			"关于 {{subject_name}} 的任务“{{task_title}}”已于 {{due_date}} 到期，目前仍未完成。"},
		"az": {"Tapşırığın son tarixi keçdi: {{task_title}}",
			"{{subject_name}} ilə bağlı \"{{task_title}}\" tapşırığının son tarixi ({{due_date}}) keçib və tapşırıq hələ açıqdır."},
		"ar": {"تأخرت المهمة: {{task_title}}",
			"كان موعد استحقاق المهمة \"{{task_title}}\" المتعلقة بـ {{subject_name}} هو {{due_date}} وما زالت مفتوحة."},
	},
}

func taskTemplates(code string) []DefaultTemplate {
	texts := taskTexts[code]
	out := make([]DefaultTemplate, 0, len(texts)*len(TaskChannels))
	for _, lang := range msgtemplate.Locales {
		t, ok := texts[lang]
		if !ok {
			continue // TestTaskTemplatesCoverEveryLocale reports it
		}
		for _, ch := range TaskChannels {
			out = append(out, DefaultTemplate{
				Role: RoleGeneric, Channel: ch, Language: lang,
				Subject: t.subject, Body: t.body, Format: "text",
			})
		}
	}
	return out
}

func taskPlaceholders(withDue bool) []msgtemplate.Placeholder {
	out := []msgtemplate.Placeholder{
		ph("task_title", "Aylık ziyaret", "Monthly visit"),
		ph("subject_name", "Tech Oto", "Tech Oto"),
	}
	if withDue {
		out = append(out, ph("due_date", "2026-10-31 17:00", "2026-10-31 17:00"))
	}
	return out
}

func init() {
	for _, code := range TaskEvents {
		Register(Event{
			Code: code, Module: "tasks",
			DefaultChannels:  TaskChannels,
			AudienceRoles:    []string{RoleCenter},
			Placeholders:     taskPlaceholders(code != EventTaskAssigned),
			UserConfigurable: true,
			Templates:        taskTemplates(code),
		})
	}
}
