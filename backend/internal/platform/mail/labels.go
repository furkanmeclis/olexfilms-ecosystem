package mail

import "strings"

// Labels are the fixed texts of the e-mail frame in one language.
type Labels struct {
	// Button is the generic primary button label ("Open").
	Button string
	// LinkFallback precedes the plain button URL in the secondary block.
	LinkFallback string
	// Reason is the footer "why you got this" line; {brand} is replaced.
	Reason string
	// Unsubscribe is the footer unsubscribe link label.
	Unsubscribe string
}

// LabelsFor returns the frame texts of a language (tr or en for now;
// every other language uses en).
func LabelsFor(lang string) Labels {
	if strings.HasPrefix(strings.ToLower(lang), "tr") {
		return Labels{
			Button:       "Görüntüle",
			LinkFallback: "Buton çalışmıyorsa bu bağlantıyı açın:",
			Reason:       "Bu e-postayı {brand} hesabınızla ilgili bir bildirim olduğu için aldınız.",
			Unsubscribe:  "Abonelikten çık",
		}
	}
	return Labels{
		Button:       "Open",
		LinkFallback: "If the button does not work, open this link:",
		Reason:       "You received this e-mail because of a notification about your {brand} account.",
		Unsubscribe:  "Unsubscribe",
	}
}

// ReasonFor is the Reason line with the brand name filled in.
func (l Labels) ReasonFor(brand string) string {
	return strings.ReplaceAll(l.Reason, "{brand}", BrandName(brand))
}
