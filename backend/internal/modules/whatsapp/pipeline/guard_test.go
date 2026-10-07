package pipeline

import (
	"context"
	"strings"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/google/uuid"
)

func TestGuardBlocksSecrets(t *testing.T) {
	for _, s := range []string{
		"anahtar: sk-ant-api03-abcdefghijklmnop",
		"token eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dozjgNryP4J3jVmNHl0w5N",
		"Authorization: Bearer abcdefghijklmnopqrstuvwxyz",
		"api_key = 1234567890abcdef",
	} {
		if g := guard(s); !g.Blocked {
			t.Errorf("not blocked: %q", s)
		}
	}
}

func TestGuardMasksInternalIDsKeepsLinks(t *testing.T) {
	id := uuid.NewString()
	g := guard("Kayıt " + id + " (organization_id: 42). Link: https://olex.test/p/" + id)
	if g.Blocked || g.Redacted != 2 {
		t.Fatalf("guard = %+v", g)
	}
	if strings.Count(g.Text, id) != 1 || strings.Contains(g.Text, "organization_id") {
		t.Fatalf("text = %q", g.Text)
	}
}

func TestToWhatsApp(t *testing.T) {
	in := "## Sonuç\n**Önemli** bilgi ve [portal](https://olex.test/p) linki.\n- bir\n* iki\n~~eski~~\n---\n<b>x</b>"
	want := "*Sonuç*\n*Önemli* bilgi ve portal: https://olex.test/p linki.\n• bir\n• iki\n~eski~\nx"
	if got := toWhatsApp(in); got != want {
		t.Fatalf("got\n%q\nwant\n%q", got, want)
	}
}

func TestSplitMessage(t *testing.T) {
	long := strings.Repeat("kelime ", 1500)
	parts := splitMessage(long, MaxPartRunes)
	if len(parts) != 3 {
		t.Fatalf("parts = %d", len(parts))
	}
	if strings.Join(parts, " ") != strings.TrimSpace(long) {
		t.Fatal("split lost text")
	}
	for _, p := range parts {
		if len([]rune(p)) > MaxPartRunes {
			t.Fatalf("part of %d runes", len([]rune(p)))
		}
	}
	if got := splitMessage("kısa", MaxPartRunes); len(got) != 1 || got[0] != "kısa" {
		t.Fatalf("short = %v", got)
	}
}

func TestKeywords(t *testing.T) {
	cases := map[string]int{
		"DUR": kwStop, "Stop!": kwStop, "İNSAN": kwHuman, "agent": kwHuman, "EVET": kwYes, "evet.": kwYes,
		"Yes": kwYes, "HAYIR": kwNo, "hayir": kwNo, "Да": kwYes, "durum nedir": kwNone, "evet ama": kwNone,
	}
	for in, want := range cases {
		if got := keywordOf(in).kind; got != want {
			t.Errorf("%q = %d, want %d", in, got, want)
		}
	}
}

type recordQueue struct{ got []uuid.UUID }

func (q *recordQueue) EnqueueAIReply(_ context.Context, c uuid.UUID) error {
	q.got = append(q.got, c)
	return nil
}

func TestMessageReceivedEnqueuesReply(t *testing.T) {
	bus := events.NewBus(nil)
	q := &recordQueue{}
	RegisterEventHandlers(bus, q, nil)
	conv := uuid.New()
	ev := events.New(events.WhatsAppMessageReceived)
	ev.Payload["conversation_uuid"] = conv.String()
	if err := bus.Publish(context.Background(), ev); err != nil {
		t.Fatal(err)
	}
	if len(q.got) != 1 || q.got[0] != conv {
		t.Fatalf("enqueued %v", q.got)
	}
}
