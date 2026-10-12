package whatsapp

import (
	"net/url"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
)

// Dealer click-to-chat (TEC-468, TEC-638): every dealer page links to the
// center's single WhatsApp number (K16) with a prefilled, localized message
// ending in "#<dealer code>". The conversation pipeline reads the tag, refers
// the conversation to that dealer and opens the lead in its list; the chat
// itself stays with the system admin.

// DealerChatText is the prefilled message for a dealer page in a locale.
func DealerChatText(locale, dealerName, code string) string {
	text := i18n.TranslateParams(i18n.Normalize(locale), "showcase.whatsapp_prefill",
		map[string]string{"name": strings.TrimSpace(dealerName)})
	return text + " #" + strings.ToLower(strings.TrimSpace(code))
}

// ChatURL is the wa.me link of an E.164 number with a prefilled text; empty
// when the number is not E.164.
func ChatURL(e164, text string) string {
	digits := strings.TrimPrefix(strings.TrimSpace(e164), "+")
	if digits == "" || !strings.HasPrefix(strings.TrimSpace(e164), "+") {
		return ""
	}
	for _, r := range digits {
		if r < '0' || r > '9' {
			return ""
		}
	}
	u := "https://wa.me/" + digits
	if text != "" {
		u += "?text=" + url.QueryEscape(text)
	}
	return u
}

// DealerChatURL links a dealer page to the center number; empty when the
// center number is not set (callers then fall back to the dealer's phone).
func DealerChatURL(centerE164, locale, dealerName, code string) string {
	return ChatURL(centerE164, DealerChatText(locale, dealerName, code))
}
