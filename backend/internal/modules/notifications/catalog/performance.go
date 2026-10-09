package catalog

import "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/msgtemplate"

const (
	EventPerformanceWeakDealer  = "PERFORMANCE_WEAK_DEALER"
	EventPerformanceBelowTarget = "PERFORMANCE_BELOW_TARGET"
)

var performanceChannels = []string{ChannelInapp, ChannelEmail}

var performanceWeakDealerTexts = map[string]localizedText{
	"tr":    {"Zayıf bayi kuralı: {{dealer_name}}", "{{dealer_name}} için {{rule_name}} kuralı {{period}} döneminde tetiklendi. {{metric}}={{metric_value}}, eşik {{threshold}}."},
	"en":    {"Weak dealer rule: {{dealer_name}}", "{{rule_name}} was triggered for {{dealer_name}} in {{period}}. {{metric}}={{metric_value}}, threshold {{threshold}}."},
	"bg":    {"Правило за слаб дилър: {{dealer_name}}", "Правилото {{rule_name}} се задейства за {{dealer_name}} през {{period}}. {{metric}}={{metric_value}}, праг {{threshold}}."},
	"de":    {"Schwacher-Händler-Regel: {{dealer_name}}", "Die Regel {{rule_name}} wurde für {{dealer_name}} im Zeitraum {{period}} ausgelöst. {{metric}}={{metric_value}}, Schwelle {{threshold}}."},
	"el":    {"Κανόνας χαμηλής απόδοσης: {{dealer_name}}", "Ο κανόνας {{rule_name}} ενεργοποιήθηκε για {{dealer_name}} στην περίοδο {{period}}. {{metric}}={{metric_value}}, όριο {{threshold}}."},
	"uk":    {"Правило слабкого дилера: {{dealer_name}}", "Правило {{rule_name}} спрацювало для {{dealer_name}} за період {{period}}. {{metric}}={{metric_value}}, поріг {{threshold}}."},
	"ru":    {"Правило слабого дилера: {{dealer_name}}", "Правило {{rule_name}} сработало для {{dealer_name}} за период {{period}}. {{metric}}={{metric_value}}, порог {{threshold}}."},
	"fr":    {"Regle concessionnaire faible : {{dealer_name}}", "La regle {{rule_name}} s'est declenchee pour {{dealer_name}} sur {{period}}. {{metric}}={{metric_value}}, seuil {{threshold}}."},
	"es":    {"Regla de bajo rendimiento: {{dealer_name}}", "La regla {{rule_name}} se activo para {{dealer_name}} en {{period}}. {{metric}}={{metric_value}}, umbral {{threshold}}."},
	"it":    {"Regola dealer debole: {{dealer_name}}", "La regola {{rule_name}} si e attivata per {{dealer_name}} nel periodo {{period}}. {{metric}}={{metric_value}}, soglia {{threshold}}."},
	"zh-CN": {"弱经销商规则：{{dealer_name}}", "{{dealer_name}} 在 {{period}} 触发了 {{rule_name}}。{{metric}}={{metric_value}}，阈值 {{threshold}}。"},
	"az":    {"Zəif bayi qaydası: {{dealer_name}}", "{{dealer_name}} üçün {{period}} dövründə {{rule_name}} qaydası işə düşdü. {{metric}}={{metric_value}}, hədd {{threshold}}."},
	"ar":    {"قاعدة أداء منخفض: {{dealer_name}}", "تم تفعيل قاعدة {{rule_name}} لـ {{dealer_name}} خلال {{period}}. {{metric}}={{metric_value}}، الحد {{threshold}}."},
}

var performanceBelowTargetTexts = map[string]localizedText{
	"tr":    {"Hedef takibi: {{period}}", "{{period}} döneminde {{metric}} değeriniz {{metric_value}}. Hedefinizi izlemek için performans panelini kontrol edin."},
	"en":    {"Target tracking: {{period}}", "Your {{metric}} value is {{metric_value}} for {{period}}. Check the performance dashboard to follow your target."},
	"bg":    {"Проследяване на цел: {{period}}", "Вашата стойност {{metric}} е {{metric_value}} за {{period}}. Проверете панела за представяне."},
	"de":    {"Zielverfolgung: {{period}}", "Ihr Wert fur {{metric}} liegt in {{period}} bei {{metric_value}}. Prufen Sie das Performance-Dashboard."},
	"el":    {"Παρακολούθηση στόχου: {{period}}", "Η τιμή {{metric}} για την περίοδο {{period}} είναι {{metric_value}}. Ελέγξτε τον πίνακα απόδοσης."},
	"uk":    {"Відстеження цілі: {{period}}", "Ваше значення {{metric}} за {{period}} становить {{metric_value}}. Перевірте панель ефективності."},
	"ru":    {"Отслеживание цели: {{period}}", "Ваше значение {{metric}} за {{period}} составляет {{metric_value}}. Проверьте панель эффективности."},
	"fr":    {"Suivi d'objectif : {{period}}", "Votre valeur {{metric}} est {{metric_value}} pour {{period}}. Consultez le tableau de performance."},
	"es":    {"Seguimiento de objetivo: {{period}}", "Tu valor de {{metric}} es {{metric_value}} en {{period}}. Revisa el panel de rendimiento."},
	"it":    {"Monitoraggio obiettivo: {{period}}", "Il valore {{metric}} per {{period}} e {{metric_value}}. Controlla il pannello performance."},
	"zh-CN": {"目标跟踪：{{period}}", "您在 {{period}} 的 {{metric}} 数值为 {{metric_value}}。请查看绩效面板跟踪目标。"},
	"az":    {"Hədəf izləmə: {{period}}", "{{period}} dövründə {{metric}} dəyəriniz {{metric_value}}. Hədəfinizi performans panelində izləyin."},
	"ar":    {"متابعة الهدف: {{period}}", "قيمة {{metric}} لديك في {{period}} هي {{metric_value}}. راجع لوحة الأداء لمتابعة الهدف."},
}

func performanceTemplates(code string, texts map[string]localizedText) []DefaultTemplate {
	out := make([]DefaultTemplate, 0, len(texts)*len(performanceChannels))
	for _, lang := range msgtemplate.Locales {
		t, ok := texts[lang]
		if !ok {
			continue
		}
		for _, ch := range performanceChannels {
			out = append(out, DefaultTemplate{Role: RoleGeneric, Channel: ch, Language: lang, Subject: t.subject, Body: t.body, Format: "text"})
		}
	}
	return out
}

func performancePlaceholders() []msgtemplate.Placeholder {
	return []msgtemplate.Placeholder{
		ph("dealer_name", "Tech Oto", "Tech Auto"),
		ph("rule_name", "Hedef %50 altı", "Below 50% target"),
		ph("period", "2026-10", "2026-10"),
		ph("metric", "target_achievement", "target_achievement"),
		ph("metric_value", "42.00", "42.00"),
		ph("threshold", "50.00", "50.00"),
	}
}

func init() {
	Register(Event{Code: EventPerformanceWeakDealer, Module: "performance", DefaultChannels: performanceChannels,
		AudienceRoles: []string{RoleCenter, RoleDistributor}, Placeholders: performancePlaceholders(),
		UserConfigurable: true, Templates: performanceTemplates(EventPerformanceWeakDealer, performanceWeakDealerTexts)})
	Register(Event{Code: EventPerformanceBelowTarget, Module: "performance", DefaultChannels: performanceChannels,
		AudienceRoles: []string{RoleDealer}, Placeholders: performancePlaceholders(),
		UserConfigurable: true, Templates: performanceTemplates(EventPerformanceBelowTarget, performanceBelowTargetTexts)})
}
