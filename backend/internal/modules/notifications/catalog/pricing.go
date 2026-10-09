package catalog

import "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/msgtemplate"

// TEC-506 (F5-09b): recommended price publication (owners of the
// distributors and dealers of the countries / currencies) and the weekly
// price discipline digest (center and distributors, one per group).
const (
	EventPricingRecommendedPublished = "PRICING_RECOMMENDED_PUBLISHED"
	EventPricingDisciplineDigest     = "PRICING_DISCIPLINE_DIGEST"
)

// PricingEvents lists the pricing notification codes.
var PricingEvents = []string{EventPricingRecommendedPublished, EventPricingDisciplineDigest}

// PricingChannels are the default channels of the pricing notifications.
var PricingChannels = []string{ChannelInapp, ChannelEmail}

var pricingPublishedTexts = map[string]localizedText{
	"tr":    {"Tavsiye satış fiyatları güncellendi", "{{currencies}} için {{price_count}} tavsiye satış fiyatı yayınlandı; {{effective_from}} tarihinden itibaren geçerli."},
	"en":    {"Recommended retail prices updated", "{{price_count}} recommended retail prices were published for {{currencies}}, effective from {{effective_from}}."},
	"bg":    {"Препоръчителните цени са обновени", "Публикувани са {{price_count}} препоръчителни цени за {{currencies}}, в сила от {{effective_from}}."},
	"de":    {"Preisempfehlungen aktualisiert", "{{price_count}} unverbindliche Preisempfehlungen für {{currencies}} wurden veröffentlicht, gültig ab {{effective_from}}."},
	"el":    {"Οι προτεινόμενες τιμές ενημερώθηκαν", "Δημοσιεύτηκαν {{price_count}} προτεινόμενες τιμές λιανικής για {{currencies}}, με ισχύ από {{effective_from}}."},
	"uk":    {"Рекомендовані ціни оновлено", "Опубліковано {{price_count}} рекомендованих роздрібних цін для {{currencies}}, чинних з {{effective_from}}."},
	"ru":    {"Рекомендованные цены обновлены", "Опубликовано {{price_count}} рекомендованных розничных цен для {{currencies}}, действующих с {{effective_from}}."},
	"fr":    {"Prix de vente conseillés mis à jour", "{{price_count}} prix de vente conseillés ont été publiés pour {{currencies}}, valables à partir du {{effective_from}}."},
	"es":    {"Precios de venta recomendados actualizados", "Se publicaron {{price_count}} precios de venta recomendados para {{currencies}}, vigentes desde el {{effective_from}}."},
	"it":    {"Prezzi di vendita consigliati aggiornati", "Sono stati pubblicati {{price_count}} prezzi di vendita consigliati per {{currencies}}, validi dal {{effective_from}}."},
	"zh-CN": {"建议零售价已更新", "已发布 {{currencies}} 的 {{price_count}} 个建议零售价，自 {{effective_from}} 起生效。"},
	"az":    {"Tövsiyə olunan satış qiymətləri yeniləndi", "{{currencies}} üçün {{price_count}} tövsiyə olunan satış qiyməti dərc edildi, {{effective_from}} tarixindən qüvvədədir."},
	"ar":    {"تم تحديث أسعار البيع الموصى بها", "تم نشر {{price_count}} من أسعار البيع الموصى بها لـ {{currencies}}، سارية اعتبارًا من {{effective_from}}."},
}

var pricingDigestTexts = map[string]localizedText{
	"tr":    {"Fiyat disiplini haftalık özeti", "{{snapshot_date}} itibarıyla {{org_count}} organizasyonun satış fiyatı tavsiye fiyattan en az %{{threshold_pct}} sapıyor: {{org_names}}."},
	"en":    {"Weekly price discipline digest", "As of {{snapshot_date}}, {{org_count}} organizations sell at least {{threshold_pct}}% away from the recommended price: {{org_names}}."},
	"bg":    {"Седмичен отчет за ценова дисциплина", "Към {{snapshot_date}} {{org_count}} организации продават с поне {{threshold_pct}}% отклонение от препоръчителната цена: {{org_names}}."},
	"de":    {"Wöchentliche Übersicht zur Preisdisziplin", "Stand {{snapshot_date}} verkaufen {{org_count}} Organisationen mindestens {{threshold_pct}} % abweichend von der Preisempfehlung: {{org_names}}."},
	"el":    {"Εβδομαδιαία σύνοψη πειθαρχίας τιμών", "Στις {{snapshot_date}}, {{org_count}} οργανισμοί πωλούν με απόκλιση τουλάχιστον {{threshold_pct}}% από την προτεινόμενη τιμή: {{org_names}}."},
	"uk":    {"Щотижневий звіт про цінову дисципліну", "Станом на {{snapshot_date}} {{org_count}} організацій продають з відхиленням щонайменше {{threshold_pct}}% від рекомендованої ціни: {{org_names}}."},
	"ru":    {"Еженедельная сводка ценовой дисциплины", "На {{snapshot_date}} {{org_count}} организаций продают с отклонением не менее {{threshold_pct}}% от рекомендованной цены: {{org_names}}."},
	"fr":    {"Synthèse hebdomadaire de la discipline tarifaire", "Au {{snapshot_date}}, {{org_count}} organisations vendent avec un écart d'au moins {{threshold_pct}} % par rapport au prix conseillé : {{org_names}}."},
	"es":    {"Resumen semanal de disciplina de precios", "A {{snapshot_date}}, {{org_count}} organizaciones venden con una desviación de al menos el {{threshold_pct}} % respecto al precio recomendado: {{org_names}}."},
	"it":    {"Riepilogo settimanale della disciplina dei prezzi", "Al {{snapshot_date}}, {{org_count}} organizzazioni vendono con uno scostamento di almeno il {{threshold_pct}}% dal prezzo consigliato: {{org_names}}."},
	"zh-CN": {"价格纪律每周汇总", "截至 {{snapshot_date}}，有 {{org_count}} 个组织的售价偏离建议价格至少 {{threshold_pct}}%：{{org_names}}。"},
	"az":    {"Qiymət intizamının həftəlik xülasəsi", "{{snapshot_date}} tarixinə {{org_count}} təşkilat tövsiyə qiymətindən ən azı {{threshold_pct}}% fərqlə satır: {{org_names}}."},
	"ar":    {"الملخص الأسبوعي لانضباط الأسعار", "حتى {{snapshot_date}}، تبيع {{org_count}} مؤسسات بانحراف لا يقل عن {{threshold_pct}}% عن السعر الموصى به: {{org_names}}."},
}

func pricingTemplates(texts map[string]localizedText) []DefaultTemplate {
	out := make([]DefaultTemplate, 0, len(texts)*len(PricingChannels))
	for _, lang := range msgtemplate.Locales {
		t, ok := texts[lang]
		if !ok {
			continue
		}
		for _, ch := range PricingChannels {
			out = append(out, DefaultTemplate{Role: RoleGeneric, Channel: ch, Language: lang, Subject: t.subject, Body: t.body, Format: "text"})
		}
	}
	return out
}

func init() {
	Register(Event{
		Code: EventPricingRecommendedPublished, Module: "pricing",
		DefaultChannels: PricingChannels, AudienceRoles: []string{RoleDealer, RoleDistributor},
		Placeholders: []msgtemplate.Placeholder{
			ph("price_count", "12", "12"),
			ph("currencies", "TRY, EUR", "TRY, EUR"),
			ph("effective_from", "2026-11-01", "2026-11-01"),
		},
		UserConfigurable: true,
		Templates:        pricingTemplates(pricingPublishedTexts),
	})
	Register(Event{
		Code: EventPricingDisciplineDigest, Module: "pricing",
		DefaultChannels: PricingChannels, AudienceRoles: []string{RoleDistributor, RoleCenter},
		Placeholders: []msgtemplate.Placeholder{
			ph("snapshot_date", "2026-10-12", "2026-10-12"),
			ph("org_count", "3", "3"),
			ph("threshold_pct", "15", "15"),
			ph("org_names", "Tech Oto, Kadıköy Bayi", "Tech Oto, Kadikoy Dealer"),
		},
		UserConfigurable: true,
		Templates:        pricingTemplates(pricingDigestTexts),
	})
}
