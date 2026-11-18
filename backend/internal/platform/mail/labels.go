package mail

import "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"

// Labels are the fixed texts of the e-mail frame in one language (backend
// i18n catalog, email.* keys).
type Labels struct {
	locale i18n.Locale
	// Button is the generic primary button label ("Open").
	Button string
	// LinkFallback precedes the plain button URL in the secondary block.
	LinkFallback string
	// Unsubscribe is the footer unsubscribe link label.
	Unsubscribe string
	// UnsubscribeIntro precedes the unsubscribe link.
	UnsubscribeIntro string
}

// LabelsFor returns the frame texts of a language (i18n.Normalize: an
// empty or unknown value is tr).
func LabelsFor(lang string) Labels {
	l := i18n.Normalize(lang)
	return Labels{
		locale:           l,
		Button:           i18n.Translate(l, "email.button.open"),
		LinkFallback:     i18n.Translate(l, "email.link_fallback"),
		Unsubscribe:      i18n.Translate(l, "email.unsubscribe.label"),
		UnsubscribeIntro: i18n.Translate(l, "email.unsubscribe.intro"),
	}
}

// ReasonFor is the footer line of account notifications for a brand.
func (l Labels) ReasonFor(brand string) string {
	return i18n.TranslateParams(l.locale, "email.footer.reason", map[string]string{"brand": BrandName(brand)})
}

// CampaignReasonFor is the footer line of campaign e-mails for a brand.
func (l Labels) CampaignReasonFor(brand string) string {
	return i18n.TranslateParams(l.locale, "email.footer.reason_campaign", map[string]string{"brand": BrandName(brand)})
}
