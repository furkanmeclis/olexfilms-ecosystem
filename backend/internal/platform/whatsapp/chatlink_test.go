package whatsapp

import (
	"net/url"
	"strings"
	"testing"
)

func TestDealerChatURL(t *testing.T) {
	got := DealerChatURL("+905466194284", "tr", "İstanbul Detailing", "Demo-Istanbul-Detailing")
	if !strings.HasPrefix(got, "https://wa.me/905466194284?text=") {
		t.Fatalf("url = %s", got)
	}
	text, _ := url.QueryUnescape(strings.TrimPrefix(got, "https://wa.me/905466194284?text="))
	if text != "Merhaba, İstanbul Detailing hakkında bilgi almak istiyorum. #demo-istanbul-detailing" {
		t.Fatalf("text = %q", text)
	}
	if en := DealerChatText("en", "3K Studio", "3k-studio"); en != "Hello, I would like information about 3K Studio. #3k-studio" {
		t.Fatalf("en = %q", en)
	}
	if ar := DealerChatText("ar", "X", "x"); !strings.HasSuffix(ar, " #x") || strings.Contains(ar, "showcase.") {
		t.Fatalf("ar = %q", ar)
	}
	for _, bad := range []string{"", "905466194284", "+90 546", "+90abc"} {
		if u := DealerChatURL(bad, "tr", "X", "x"); u != "" {
			t.Fatalf("%q must give no link, got %s", bad, u)
		}
	}
}
