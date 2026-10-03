package legacymobile

import "strings"

// The old hub's "message" strings (olexfilms lang/{tr,en}/mobile_api.php),
// only the ones the aliases answer with. The old app shows them as is; the
// hub translated them into tr and en (other locales fell back to en), and
// so do the aliases.
const (
	msgLoginSuccess        = "auth.login_success"
	msgCredentialsMismatch = "auth.credentials_mismatch"
	msgAccountInactive     = "errors.account_inactive"
	msgPushSaved           = "push_tokens.saved"
	msgPushDeleted         = "push_tokens.deleted"
	msgReportSaved         = "nexptg_reports.saved"
	msgRequired            = "validation.required"
	msgPushRequired        = "push_tokens.required"
	msgPlatformMissing     = "push_tokens.platform_required"
	msgPlatformIn          = "validation.platform_in"
	msgEmail               = "validation.email"
)

var messages = map[string]map[string]string{
	"tr": {
		msgLoginSuccess:        "Giriş başarılı.",
		msgCredentialsMismatch: "Girdiğiniz bilgiler kayıtlarımızla eşleşmiyor.",
		msgAccountInactive:     "Hesabınız veya bağlı olduğunuz bayi pasif durumda. Lütfen yönetici ile iletişime geçin.",
		msgPushSaved:           "Push token kaydedildi.",
		msgPushDeleted:         "Push token silindi.",
		msgReportSaved:         "NexPTG raporu kaydedildi.",
		msgPushRequired:        "Push token zorunludur.",
		msgPlatformMissing:     "Platform zorunludur.",
		msgPlatformIn:          "Platform ios veya android olmalıdır.",
		msgRequired:            "Bu alan zorunludur.",
		msgEmail:               "Geçerli bir e-posta adresi giriniz.",
	},
	"en": {
		msgLoginSuccess:        "Login successful.",
		msgCredentialsMismatch: "The credentials you entered do not match our records.",
		msgAccountInactive:     "Your account or associated dealer is inactive. Please contact an administrator.",
		msgPushSaved:           "Push token saved.",
		msgPushDeleted:         "Push token deleted.",
		// Hard-coded Turkish in NexptgReportController::store and the push
		// token FormRequest messages(), whatever the locale.
		msgReportSaved:     "NexPTG raporu kaydedildi.",
		msgPushRequired:    "Push token zorunludur.",
		msgPlatformMissing: "Platform zorunludur.",
		msgPlatformIn:      "Platform ios veya android olmalıdır.",
		msgRequired:        "This field is required.",
		msgEmail:           "Enter a valid email address.",
	},
}

// message returns key in locale ("tr", "tr-TR", "tr_TR" are Turkish; any
// other locale is English, the hub's fallback).
func message(locale, key string) string {
	lang := "en"
	if l := strings.ToLower(strings.TrimSpace(locale)); strings.HasPrefix(l, "tr") {
		lang = "tr"
	}
	return messages[lang][key]
}
