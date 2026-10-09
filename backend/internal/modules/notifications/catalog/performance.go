package catalog

import "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/msgtemplate"

const (
	EventPerformanceWeakDealer      = "PERFORMANCE_WEAK_DEALER"
	EventPerformanceBelowTarget     = "PERFORMANCE_BELOW_TARGET"
	EventPerformanceBonusCalculated = "PERFORMANCE_BONUS_CALCULATED"
	EventPerformanceBonusApproved   = "PERFORMANCE_BONUS_APPROVED"
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

var performanceBonusCalculatedTexts = map[string]localizedText{
	"tr":    {"Prim tahakkuklari hazir: {{period}}", "{{period}} donemi icin {{count}} prim tahakkuku owner onayini bekliyor."},
	"en":    {"Bonus accruals ready: {{period}}", "{{count}} bonus accruals for {{period}} are waiting for owner approval."},
	"bg":    {"Бонусите са изчислени: {{period}}", "{{count}} бонус начисления за {{period}} чакат одобрение от owner."},
	"de":    {"Bonusabgrenzungen bereit: {{period}}", "{{count}} Bonusabgrenzungen fur {{period}} warten auf Owner-Freigabe."},
	"el":    {"Έτοιμες προμήθειες: {{period}}", "{{count}} εγγραφές bonus για {{period}} περιμένουν έγκριση owner."},
	"uk":    {"Бонуси нараховано: {{period}}", "{{count}} бонусних нарахувань за {{period}} очікують схвалення owner."},
	"ru":    {"Бонусы рассчитаны: {{period}}", "{{count}} бонусных начислений за {{period}} ожидают одобрения owner."},
	"fr":    {"Primes calculees : {{period}}", "{{count}} provisions de prime pour {{period}} attendent l'approbation owner."},
	"es":    {"Bonos calculados: {{period}}", "{{count}} devengos de bono de {{period}} esperan aprobacion del owner."},
	"it":    {"Bonus calcolati: {{period}}", "{{count}} maturazioni bonus per {{period}} attendono approvazione owner."},
	"zh-CN": {"奖金已计算：{{period}}", "{{period}} 的 {{count}} 条奖金计提正在等待 owner 审批。"},
	"az":    {"Prim hesablamalari hazirdir: {{period}}", "{{period}} dovru ucun {{count}} prim hesablamasi owner tesdiqini gozleyir."},
	"ar":    {"تم حساب المكافآت: {{period}}", "{{count}} استحقاقات مكافأة للفترة {{period}} بانتظار موافقة المالك."},
}

var performanceBonusApprovedTexts = map[string]localizedText{
	"tr":    {"Priminiz onaylandi", "{{period}} donemi icin {{amount}} {{currency}} priminiz onaylandi. Odeme gunu: {{paid_on}}."},
	"en":    {"Your bonus was approved", "Your {{amount}} {{currency}} bonus for {{period}} was approved. Payment day: {{paid_on}}."},
	"bg":    {"Бонусът ви е одобрен", "Вашият бонус {{amount}} {{currency}} за {{period}} е одобрен. Ден за плащане: {{paid_on}}."},
	"de":    {"Ihr Bonus wurde freigegeben", "Ihr Bonus uber {{amount}} {{currency}} fur {{period}} wurde freigegeben. Zahlungstag: {{paid_on}}."},
	"el":    {"Το bonus εγκρίθηκε", "Το bonus {{amount}} {{currency}} για {{period}} εγκρίθηκε. Ημέρα πληρωμής: {{paid_on}}."},
	"uk":    {"Ваш бонус схвалено", "Ваш бонус {{amount}} {{currency}} за {{period}} схвалено. День оплати: {{paid_on}}."},
	"ru":    {"Ваш бонус одобрен", "Ваш бонус {{amount}} {{currency}} за {{period}} одобрен. День выплаты: {{paid_on}}."},
	"fr":    {"Votre prime est approuvee", "Votre prime de {{amount}} {{currency}} pour {{period}} est approuvee. Jour de paiement : {{paid_on}}."},
	"es":    {"Tu bono fue aprobado", "Tu bono de {{amount}} {{currency}} para {{period}} fue aprobado. Dia de pago: {{paid_on}}."},
	"it":    {"Il tuo bonus e approvato", "Il bonus di {{amount}} {{currency}} per {{period}} e approvato. Giorno pagamento: {{paid_on}}."},
	"zh-CN": {"您的奖金已审批", "您 {{period}} 的 {{amount}} {{currency}} 奖金已审批。付款日：{{paid_on}}。"},
	"az":    {"Priminiz tesdiqlendi", "{{period}} dovru ucun {{amount}} {{currency}} priminiz tesdiqlendi. Odeme gunu: {{paid_on}}."},
	"ar":    {"تمت الموافقة على مكافأتك", "تمت الموافقة على مكافأة {{amount}} {{currency}} للفترة {{period}}. يوم الدفع: {{paid_on}}."},
}

func performanceTemplates(code string, texts map[string]localizedText) []DefaultTemplate {
	return performanceTemplatesForChannels(code, texts, performanceChannels)
}

func performanceTemplatesForChannels(code string, texts map[string]localizedText, channels []string) []DefaultTemplate {
	out := make([]DefaultTemplate, 0, len(texts)*len(performanceChannels))
	for _, lang := range msgtemplate.Locales {
		t, ok := texts[lang]
		if !ok {
			continue
		}
		for _, ch := range channels {
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

func performanceBonusPlaceholders() []msgtemplate.Placeholder {
	return []msgtemplate.Placeholder{
		ph("period", "2026-10", "2026-10"),
		ph("count", "3", "3"),
		ph("amount", "500.00", "500.00"),
		ph("currency", "TRY", "TRY"),
		ph("paid_on", "2026-11-05", "2026-11-05"),
	}
}

func init() {
	Register(Event{Code: EventPerformanceWeakDealer, Module: "performance", DefaultChannels: performanceChannels,
		AudienceRoles: []string{RoleCenter, RoleDistributor}, Placeholders: performancePlaceholders(),
		UserConfigurable: true, Templates: performanceTemplates(EventPerformanceWeakDealer, performanceWeakDealerTexts)})
	Register(Event{Code: EventPerformanceBelowTarget, Module: "performance", DefaultChannels: performanceChannels,
		AudienceRoles: []string{RoleDealer}, Placeholders: performancePlaceholders(),
		UserConfigurable: true, Templates: performanceTemplates(EventPerformanceBelowTarget, performanceBelowTargetTexts)})
	Register(Event{Code: EventPerformanceBonusCalculated, Module: "performance", DefaultChannels: performanceChannels,
		AudienceRoles: []string{RoleDealer}, Placeholders: performanceBonusPlaceholders(),
		UserConfigurable: true, Templates: performanceTemplates(EventPerformanceBonusCalculated, performanceBonusCalculatedTexts)})
	Register(Event{Code: EventPerformanceBonusApproved, Module: "performance", DefaultChannels: []string{ChannelInapp},
		AudienceRoles: []string{RoleDealer}, Placeholders: performanceBonusPlaceholders(),
		UserConfigurable: true, Templates: performanceTemplatesForChannels(EventPerformanceBonusApproved, performanceBonusApprovedTexts, []string{ChannelInapp})})
}
