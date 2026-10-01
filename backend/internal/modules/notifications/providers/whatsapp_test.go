package providers

import (
	"context"
	"errors"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/whatsapp/fake"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestWhatsAppProviderDeliver(t *testing.T) {
	wa := &fake.Provider{}
	p := WhatsAppProvider{WA: wa}
	id := uuid.New()
	res, err := p.Deliver(context.Background(), db.Notification{
		Uuid: id, Title: "Servis hazır", Body: "Aracınız teslime hazır.",
		Recipient: pgtype.Text{String: "0555 123 45 67", Valid: true},
	}, nil)
	if err != nil || res.Provider != "fake" || res.ProviderReference == "" || p.Channel() != "whatsapp" {
		t.Fatalf("deliver: %+v %v", res, err)
	}
	m, _ := wa.Last()
	if m.To != "+905551234567" || m.Body != "*Servis hazır*\nAracınız teslime hazır." || len(m.ID) != 32 {
		t.Fatalf("sent = %+v", m)
	}
	if _, err := p.Deliver(context.Background(), db.Notification{}, nil); !errors.Is(err, ErrNoRecipient) {
		t.Fatalf("no recipient: %v", err)
	}
}
