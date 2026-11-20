package db_test

import (
	"encoding/json"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// TEC-1259: anonymize_customer_contacts (migration 000202) leaves no
// personal data of the person in conversations, messages, AI runs, the
// WhatsApp LID map, notifications, leads and lead events; another person's
// rows stay untouched; a second call changes nothing.
func TestAnonymizeCustomerContactsLeavesNoPersonalData(t *testing.T) {
	f := newCustomerFixture(t)
	const phone, email = "+905551590001", "kisi.t1259@example.test"
	const otherPhone = "+905551590099"
	if _, err := f.tx.Exec(f.ctx, `UPDATE users SET email = $2 WHERE id = $1`, f.user.ID, email); err != nil {
		t.Fatal(err)
	}
	other := f.newUser(t, otherPhone)
	org, brand := f.olex.ID, f.olex.BrandID

	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := f.tx.Exec(f.ctx, sql, args...); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
	}
	scalar := func(sql string, args ...any) int64 {
		t.Helper()
		var n int64
		if err := f.tx.QueryRow(f.ctx, sql, args...).Scan(&n); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
		return n
	}
	conv := func(contact, name string, user *int64) int64 {
		t.Helper()
		var id int64
		if err := f.tx.QueryRow(f.ctx, `
			INSERT INTO conversations (organization_id, brand_id, channel, contact_e164, contact_name, user_id)
			VALUES ($1, $2, 'whatsapp', $3, $4, $5) RETURNING id`, org, brand, contact, name, user).Scan(&id); err != nil {
			t.Fatalf("conversation: %v", err)
		}
		return id
	}
	msg := func(convID int64, ext, body string) int64 {
		t.Helper()
		var id int64
		if err := f.tx.QueryRow(f.ctx, `
			INSERT INTO messages (conversation_id, organization_id, brand_id, channel, direction, external_id, body,
			                      media, raw, media_storage_key, media_mime, media_size)
			VALUES ($1, $2, $3, 'whatsapp', 'in', $4, $5, '{"caption":"x"}', '{"from":"x"}', 'wa/' || $4, 'image/jpeg', 10)
			RETURNING id`, convID, org, brand, ext, body).Scan(&id); err != nil {
			t.Fatalf("message: %v", err)
		}
		return id
	}

	// The person: one conversation by phone (no account link), one linked
	// to the account under an old number.
	byPhone := conv(phone, "Ayşe Kaya", nil)
	byUser := conv("+905551590002", "Ayşe K.", &f.user.ID)
	m1 := msg(byPhone, "t1259-1", "Plakam 34 ABC 123")
	msg(byUser, "t1259-2", "Adresim Gizli Sokak 7")
	exec(`INSERT INTO conversation_ai_runs (conversation_id, trigger_message_id, organization_id, brand_id, status, stages, tool_calls, error, finished_at)
	      VALUES ($1, $2, $3, $4, 'failed', '[{"text":"Ayşe"}]', '[{"phone":"+905551590001"}]', 'Ayşe', NOW())`, byPhone, m1, org, brand)
	exec(`INSERT INTO whatsapp_lid_map (lid, phone_e164) VALUES ('t1259lid@lid', $1), ('t1259other@lid', $2)`, phone, otherPhone)
	exec(`INSERT INTO notifications (user_id, channel, title, body, payload, recipient, action_url, last_error)
	      VALUES ($1, 'inapp', 'Merhaba Ayşe', 'Aracınız hazır', '{"name":"Ayşe"}', NULL, '/portal/x', 'err Ayşe')`, f.user.ID)
	exec(`INSERT INTO notifications (user_id, channel, title, body, payload, recipient)
	      VALUES (NULL, 'email', 'Teklif', 'Sayın Ayşe', '{"x":1}', upper($1))`, email)
	exec(`INSERT INTO notifications (user_id, channel, title, body, payload, recipient)
	      VALUES (NULL, 'whatsapp', 'OTP', 'Kod 123456', '{}', $1)`, phone)
	var custLead, candLead int64
	if err := f.tx.QueryRow(f.ctx, `
		INSERT INTO leads (organization_id, brand_id, target_type, customer_user_id, source, notes)
		VALUES ($1, $2, 'customer', $3, 'walk_in', 'Ayşe hanım PPF istiyor') RETURNING id`, org, brand, f.user.ID).Scan(&custLead); err != nil {
		t.Fatalf("customer lead: %v", err)
	}
	if err := f.tx.QueryRow(f.ctx, `
		INSERT INTO leads (organization_id, brand_id, target_type, candidate_company_name, candidate_contact_name,
		                   candidate_phone_e164, candidate_email, source, status, lost_reason, notes)
		VALUES ($1, $2, 'dealer_candidate', 'Kaya Oto', 'Ayşe Kaya', $3, NULL, 'website', 'lost', 'Ayşe vazgeçti', 'not')
		RETURNING id`, org, brand, phone).Scan(&candLead); err != nil {
		t.Fatalf("candidate lead: %v", err)
	}
	for _, l := range []int64{custLead, candLead} {
		exec(`INSERT INTO lead_events (lead_id, organization_id, brand_id, event_type, payload)
		      VALUES ($1, $2, $3, 'note', '{"body":"Ayşe aradı"}')`, l, org, brand)
	}

	// Another person: must stay as is.
	otherConv := conv(otherPhone, "Mehmet", &other.ID)
	msg(otherConv, "t1259-3", "Mehmet'in mesajı")
	exec(`INSERT INTO notifications (user_id, channel, title, body) VALUES ($1, 'inapp', 'Mehmet', 'Mehmet')`, other.ID)
	var otherLead int64
	if err := f.tx.QueryRow(f.ctx, `
		INSERT INTO leads (organization_id, brand_id, target_type, candidate_contact_name, candidate_phone_e164, source, notes)
		VALUES ($1, $2, 'customer', 'Mehmet', $3, 'website', 'Mehmet notu') RETURNING id`, org, brand, otherPhone).Scan(&otherLead); err != nil {
		t.Fatalf("other lead: %v", err)
	}
	exec(`INSERT INTO lead_events (lead_id, organization_id, brand_id, event_type, payload)
	      VALUES ($1, $2, $3, 'note', '{"body":"Mehmet"}')`, otherLead, org, brand)

	// The service reads media keys and lead uuids before masking.
	keys, err := f.q.ListCustomerContactMediaKeys(f.ctx, db.ListCustomerContactMediaKeysParams{
		UserID: pgtype.Int8{Int64: f.user.ID, Valid: true}, Phone: text(phone),
	})
	if err != nil || len(keys) != 2 {
		t.Fatalf("media keys = %v, %v; want 2", keys, err)
	}
	leads, err := f.q.ListCustomerContactLeadUUIDs(f.ctx, db.ListCustomerContactLeadUUIDsParams{
		UserID: pgtype.Int8{Int64: f.user.ID, Valid: true}, Phone: text(phone), Email: text(email),
	})
	if err != nil || len(leads) != 2 {
		t.Fatalf("lead uuids = %v, %v; want 2", leads, err)
	}

	params := db.AnonymizeCustomerContactsParams{
		UserID: pgtype.Int8{Int64: f.user.ID, Valid: true}, Phone: text(phone), Email: text(email),
	}
	raw, err := f.q.AnonymizeCustomerContacts(f.ctx, params)
	if err != nil {
		t.Fatalf("anonymize: %v", err)
	}
	var counts map[string]int
	if err := json.Unmarshal(raw, &counts); err != nil {
		t.Fatal(err)
	}
	want := map[string]int{
		"conversations": 2, "messages": 2, "conversation_ai_runs": 1, "whatsapp_lid_map": 1,
		"notifications": 3, "leads": 2, "lead_events": 2,
	}
	for k, v := range want {
		if counts[k] != v {
			t.Errorf("counts[%s] = %d, want %d (%v)", k, counts[k], v, counts)
		}
	}

	// Per table: no personal data of the person is left.
	convIDs := []int64{byPhone, byUser}
	leadIDs := []int64{custLead, candLead}
	checks := []struct {
		table string
		sql   string
		args  []any
	}{
		{"conversations", `SELECT COUNT(*) FROM conversations WHERE id = ANY($1)
			AND (contact_e164 LIKE '+%' OR contact_name IS NOT NULL OR status <> 'closed' OR ai_mode <> 'off')`, []any{convIDs}},
		{"conversations (phone)", `SELECT COUNT(*) FROM conversations WHERE contact_e164 = $1`, []any{phone}},
		{"messages", `SELECT COUNT(*) FROM messages WHERE conversation_id = ANY($1)
			AND (body IS NOT NULL OR media IS NOT NULL OR raw IS NOT NULL OR media_storage_key IS NOT NULL OR media_mime IS NOT NULL)`, []any{convIDs}},
		{"conversation_ai_runs", `SELECT COUNT(*) FROM conversation_ai_runs WHERE conversation_id = ANY($1)
			AND (stages <> '[]' OR tool_calls <> '[]' OR error IS NOT NULL)`, []any{convIDs}},
		{"whatsapp_lid_map", `SELECT COUNT(*) FROM whatsapp_lid_map WHERE phone_e164 = $1`, []any{phone}},
		{"notifications", `SELECT COUNT(*) FROM notifications
			WHERE (user_id = $1 OR recipient = $2 OR lower(recipient) = $3 OR body LIKE '%Ayşe%')
			  AND (title <> '' OR body <> '' OR payload <> '{}' OR recipient IS NOT NULL OR action_url IS NOT NULL OR last_error IS NOT NULL)`,
			[]any{f.user.ID, phone, email}},
		{"leads", `SELECT COUNT(*) FROM leads WHERE id = ANY($1)
			AND (candidate_company_name IS NOT NULL OR candidate_contact_name IS NOT NULL OR candidate_phone_e164 IS NOT NULL
			     OR candidate_email IS NOT NULL OR notes <> '' OR lost_reason <> '[anonymized]')`, []any{leadIDs}},
		{"lead_events", `SELECT COUNT(*) FROM lead_events WHERE lead_id = ANY($1)
			AND event_type IN ('note', 'call', 'message') AND payload <> '{"anonymized": true}'`, []any{leadIDs}},
	}
	for _, c := range checks {
		if n := scalar(c.sql, c.args...); n != 0 {
			t.Errorf("%s: %d rows still hold personal data", c.table, n)
		}
	}
	// Rows are kept (statistics, foreign keys).
	if n := scalar(`SELECT COUNT(*) FROM messages WHERE conversation_id = ANY($1)`, convIDs); n != 2 {
		t.Errorf("messages kept = %d, want 2", n)
	}

	// The other person is untouched.
	if n := scalar(`SELECT COUNT(*) FROM conversations WHERE id = $1 AND contact_e164 = $2 AND contact_name = 'Mehmet'`, otherConv, otherPhone); n != 1 {
		t.Error("other conversation changed")
	}
	if n := scalar(`SELECT COUNT(*) FROM messages WHERE conversation_id = $1 AND body IS NOT NULL`, otherConv); n != 1 {
		t.Error("other message changed")
	}
	if n := scalar(`SELECT COUNT(*) FROM whatsapp_lid_map WHERE phone_e164 = $1`, otherPhone); n != 1 {
		t.Error("other lid mapping deleted")
	}
	if n := scalar(`SELECT COUNT(*) FROM notifications WHERE user_id = $1 AND body = 'Mehmet'`, other.ID); n != 1 {
		t.Error("other notification changed")
	}
	if n := scalar(`SELECT COUNT(*) FROM leads WHERE id = $1 AND candidate_contact_name = 'Mehmet' AND notes = 'Mehmet notu'`, otherLead); n != 1 {
		t.Error("other lead changed")
	}
	if n := scalar(`SELECT COUNT(*) FROM lead_events WHERE lead_id = $1 AND payload->>'body' = 'Mehmet'`, otherLead); n != 1 {
		t.Error("other lead event changed")
	}

	// Idempotent.
	raw, err = f.q.AnonymizeCustomerContacts(f.ctx, params)
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	if err := json.Unmarshal(raw, &counts); err != nil {
		t.Fatal(err)
	}
	for k, v := range counts {
		if v != 0 {
			t.Errorf("second call changed %s: %d", k, v)
		}
	}

	// lead_events stays append-only outside the anonymization.
	f.expectCode(t, "lead event update", func(sp pgx.Tx) error {
		_, err := sp.Exec(f.ctx, `UPDATE lead_events SET payload = '{"anonymized": true}' WHERE lead_id = $1`, otherLead)
		return err
	}, "P0001", "23001")
	f.expectCode(t, "lead event delete", func(sp pgx.Tx) error {
		_, err := sp.Exec(f.ctx, `DELETE FROM lead_events WHERE lead_id = $1`, otherLead)
		return err
	}, "P0001", "23001")
}
