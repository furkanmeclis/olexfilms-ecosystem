package pipeline

import (
	"strings"
	"testing"

	aitools "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/ai/tools"
	aiusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/ai/usecase"
)

// TEC-461: the card is rendered in the conversation language; a language
// outside the 13 locales (or none) gets en, not the tr default.
func TestCardTextLocale(t *testing.T) {
	card := aiusecase.Card{ToolName: "create_appointment", Preview: aitools.Preview{
		Action: "create_appointment", Summary: "stored summary",
		SummaryArgs: map[string]string{"name": "Ali Veli", "date": "2026-10-09", "time": "10:30"},
		Fields:      []aitools.Field{{Key: "customer", Value: "Ali Veli"}, {Key: "unknown_key", Value: "x"}},
	}}
	cases := []struct{ locale, summary, field string }{
		{"tr", "Ali Veli için 2026-10-09 10:30 tarihine randevu oluşturulsun.", "• Müşteri: Ali Veli"},
		{"ar", "حجز موعد لـ Ali Veli بتاريخ 2026-10-09 الساعة 10:30.", "• العميل: Ali Veli"},
		{"zh_CN", "为 Ali Veli 预约 2026-10-09 10:30。", "• 客户: Ali Veli"},
		{"ja", "Book an appointment for Ali Veli on 2026-10-09 at 10:30.", "• Customer: Ali Veli"},
		{"", "Book an appointment for Ali Veli on 2026-10-09 at 10:30.", "• Customer: Ali Veli"},
	}
	for _, tc := range cases {
		got := cardText(tc.locale, card)
		if !strings.Contains(got, "*"+tc.summary+"*") || !strings.Contains(got, tc.field) || !strings.Contains(got, "• unknown key: x") {
			t.Errorf("%q:\n%s", tc.locale, got)
		}
	}

	// A preview without a catalog template keeps its stored summary.
	plain := aiusecase.Card{Preview: aitools.Preview{Action: "custom_tool", Summary: "Custom summary."}}
	if got := cardText("tr", plain); !strings.Contains(got, "*Custom summary.*") {
		t.Errorf("plain card:\n%s", got)
	}
}
