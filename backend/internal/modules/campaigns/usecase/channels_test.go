package usecase

import (
	"context"
	"strings"
	"testing"
)

// TEC-1235: the details link is the primary button, the unsubscribe link
// sits in the footer; both appear in the text part. A non-http link stays
// in the body.
func TestCampaignEmailMessage(t *testing.T) {
	msg, err := MailEmail{}.Message(context.Background(), EmailMessage{
		Locale: "ar", Subject: "حملة", Body: "مرحبا",
		Deeplink: "https://app.test/c/1", UnsubscribeURL: "https://app.test/abonelik-iptal/tok",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(msg.HTMLBody, "v:roundrect") || !strings.Contains(msg.HTMLBody, `href="https://app.test/c/1"`) ||
		!strings.Contains(msg.HTMLBody, `dir="rtl"`) || !strings.Contains(msg.HTMLBody, "إلغاء الاشتراك") {
		t.Fatalf("html: %s", msg.HTMLBody)
	}
	if !strings.Contains(msg.Body, "التفاصيل: https://app.test/c/1") || !strings.Contains(msg.Body, "إلغاء الاشتراك: https://app.test/abonelik-iptal/tok") {
		t.Fatalf("text: %q", msg.Body)
	}
	msg, err = MailEmail{}.Message(context.Background(), EmailMessage{Locale: "en", Subject: "S", Body: "B", Deeplink: "olex://campaign/1"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(msg.HTMLBody, "v:roundrect") || !strings.Contains(msg.Body, "olex://campaign/1") {
		t.Fatalf("app link: %q", msg.Body)
	}
}
