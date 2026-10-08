package catalog

import "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/msgtemplate"

const (
	EventCertificateExpiring = "CERTIFICATE_EXPIRING"
	EventCertificateExpired  = "CERTIFICATE_EXPIRED"
)

var CertificateChannels = []string{ChannelInapp, ChannelEmail}

var certificateTexts = map[string]map[string]localizedText{
	EventCertificateExpiring: {
		"tr":    {"Sertifika süresi yaklaşıyor: {{type_name}}", "{{organization_name}} ekibindeki {{type_name}} sertifikası {{expires_date}} tarihinde sona erecek."},
		"en":    {"Certificate expiring soon: {{type_name}}", "The {{type_name}} certificate in {{organization_name}} expires on {{expires_date}}."},
		"bg":    {"Сертификатът скоро изтича: {{type_name}}", "Сертификатът {{type_name}} в {{organization_name}} изтича на {{expires_date}}."},
		"de":    {"Zertifikat läuft bald ab: {{type_name}}", "Das Zertifikat {{type_name}} bei {{organization_name}} läuft am {{expires_date}} ab."},
		"el":    {"Το πιστοποιητικό λήγει σύντομα: {{type_name}}", "Το πιστοποιητικό {{type_name}} στον οργανισμό {{organization_name}} λήγει στις {{expires_date}}."},
		"uk":    {"Сертифікат скоро закінчується: {{type_name}}", "Сертифікат {{type_name}} в {{organization_name}} закінчується {{expires_date}}."},
		"ru":    {"Сертификат скоро истекает: {{type_name}}", "Сертификат {{type_name}} в {{organization_name}} истекает {{expires_date}}."},
		"fr":    {"Certificat bientôt expiré : {{type_name}}", "Le certificat {{type_name}} de {{organization_name}} expire le {{expires_date}}."},
		"es":    {"Certificado próximo a vencer: {{type_name}}", "El certificado {{type_name}} de {{organization_name}} vence el {{expires_date}}."},
		"it":    {"Certificato in scadenza: {{type_name}}", "Il certificato {{type_name}} di {{organization_name}} scade il {{expires_date}}."},
		"zh-CN": {"证书即将到期：{{type_name}}", "{{organization_name}} 的 {{type_name}} 证书将于 {{expires_date}} 到期。"},
		"az":    {"Sertifikatın vaxtı yaxınlaşır: {{type_name}}", "{{organization_name}} üzrə {{type_name}} sertifikatının müddəti {{expires_date}} tarixində bitəcək."},
		"ar":    {"ستنتهي صلاحية الشهادة قريبًا: {{type_name}}", "ستنتهي صلاحية شهادة {{type_name}} في {{organization_name}} بتاريخ {{expires_date}}."},
	},
	EventCertificateExpired: {
		"tr":    {"Sertifika süresi doldu: {{type_name}}", "{{organization_name}} ekibindeki {{type_name}} sertifikasının süresi {{expires_date}} tarihinde doldu; açık hizmetler yeniden değerlendirildi."},
		"en":    {"Certificate expired: {{type_name}}", "The {{type_name}} certificate in {{organization_name}} expired on {{expires_date}}; open services were rechecked."},
		"bg":    {"Сертификатът изтече: {{type_name}}", "Сертификатът {{type_name}} в {{organization_name}} изтече на {{expires_date}}; отворените услуги бяха проверени отново."},
		"de":    {"Zertifikat abgelaufen: {{type_name}}", "Das Zertifikat {{type_name}} bei {{organization_name}} ist am {{expires_date}} abgelaufen; offene Services wurden erneut geprüft."},
		"el":    {"Το πιστοποιητικό έληξε: {{type_name}}", "Το πιστοποιητικό {{type_name}} στον οργανισμό {{organization_name}} έληξε στις {{expires_date}} και οι ανοιχτές υπηρεσίες ελέγχθηκαν ξανά."},
		"uk":    {"Сертифікат закінчився: {{type_name}}", "Сертифікат {{type_name}} в {{organization_name}} закінчився {{expires_date}}; відкриті послуги перевірено повторно."},
		"ru":    {"Сертификат истёк: {{type_name}}", "Сертификат {{type_name}} в {{organization_name}} истёк {{expires_date}}; открытые услуги были проверены повторно."},
		"fr":    {"Certificat expiré : {{type_name}}", "Le certificat {{type_name}} de {{organization_name}} a expiré le {{expires_date}}; les services ouverts ont été revérifiés."},
		"es":    {"Certificado vencido: {{type_name}}", "El certificado {{type_name}} de {{organization_name}} venció el {{expires_date}}; los servicios abiertos se revisaron de nuevo."},
		"it":    {"Certificato scaduto: {{type_name}}", "Il certificato {{type_name}} di {{organization_name}} è scaduto il {{expires_date}}; i servizi aperti sono stati ricontrollati."},
		"zh-CN": {"证书已到期：{{type_name}}", "{{organization_name}} 的 {{type_name}} 证书已于 {{expires_date}} 到期；未完成服务已重新检查。"},
		"az":    {"Sertifikatın müddəti bitdi: {{type_name}}", "{{organization_name}} üzrə {{type_name}} sertifikatının müddəti {{expires_date}} tarixində bitdi; açıq xidmətlər yenidən yoxlandı."},
		"ar":    {"انتهت صلاحية الشهادة: {{type_name}}", "انتهت صلاحية شهادة {{type_name}} في {{organization_name}} بتاريخ {{expires_date}}؛ تمت إعادة فحص الخدمات المفتوحة."},
	},
}

func certificateTemplates(code string) []DefaultTemplate {
	texts := certificateTexts[code]
	out := make([]DefaultTemplate, 0, len(texts)*len(CertificateChannels))
	for _, lang := range msgtemplate.Locales {
		t, ok := texts[lang]
		if !ok {
			continue
		}
		for _, ch := range CertificateChannels {
			out = append(out, DefaultTemplate{Role: RoleGeneric, Channel: ch, Language: lang, Subject: t.subject, Body: t.body, Format: "text"})
		}
	}
	return out
}

func certificatePlaceholders() []msgtemplate.Placeholder {
	return []msgtemplate.Placeholder{
		ph("type_name", "PPF Uygulama", "PPF application"),
		ph("organization_name", "Tech Oto", "Tech Oto"),
		ph("expires_date", "2026-11-06", "2026-11-06"),
	}
}

func init() {
	Register(Event{
		Code: EventCertificateExpiring, Module: "certificates",
		DefaultChannels: CertificateChannels, AudienceRoles: []string{RoleDealer, RoleDistributor, RoleCenter},
		Placeholders: certificatePlaceholders(), UserConfigurable: true,
		Templates: certificateTemplates(EventCertificateExpiring),
	})
	Register(Event{
		Code: EventCertificateExpired, Module: "certificates",
		DefaultChannels: CertificateChannels, AudienceRoles: []string{RoleDealer, RoleDistributor, RoleCenter},
		Placeholders: certificatePlaceholders(), UserConfigurable: true,
		Templates: certificateTemplates(EventCertificateExpired),
	})
}
