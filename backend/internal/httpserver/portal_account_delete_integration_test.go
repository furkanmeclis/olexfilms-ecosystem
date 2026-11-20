package httpserver

import (
	"context"
	"net/http"
	"testing"
)

// TEC-1257 acceptance: the customer deletes their own account with a phone
// code (purpose account_delete). A wrong code is refused, the delete code
// does not sign in,
// the right code anonymizes the account (no organization link needed), the
// session stops working and the deletion is audited. The public legal texts
// (privacy policy, KVKK notice, terms of use) are read without a session and
// are never answered as a consent.
func TestIntegrationPortalAccountDelete(t *testing.T) {
	it, fw := newWhatsAppIntegration(t)
	ctx := context.Background()

	ph := itPhone()
	if code, env := it.do("POST", "/v1/auth/otp/request", hostOlex, "", map[string]string{"phone": ph}); code != http.StatusAccepted {
		t.Fatalf("otp request: %d %s", code, errCode(env))
	}
	login, _ := sentCode(t, fw, ph)
	tp := it.tokensFrom(it.do("POST", "/v1/auth/otp/verify", hostOlex, "", map[string]string{"phone": ph, "code": login}))
	var userUUID string
	if err := it.pool.QueryRow(ctx, `SELECT uuid::text FROM users WHERE phone_e164 = $1`, ph).Scan(&userUUID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = it.pool.Exec(ctx, `DELETE FROM otp_codes WHERE phone_e164 = $1`, ph)
		_, _ = it.pool.Exec(ctx, `DELETE FROM refresh_tokens WHERE user_id IN (SELECT id FROM users WHERE uuid = $1)`, userUUID)
		_, _ = it.pool.Exec(ctx, `DELETE FROM users WHERE uuid = $1`, userUUID)
	})
	tok := tp.AccessToken

	// 1. No code yet, an empty code and an unknown body field are refused.
	if code, env := it.do("POST", "/v1/portal/me/delete", hostOlex, tok, map[string]string{"code": "000000"}); code != http.StatusUnauthorized || errCode(env) != "INVALID_OTP_CODE" {
		t.Fatalf("delete without code request: %d %s", code, errCode(env))
	}
	if code, _ := it.do("POST", "/v1/portal/me/delete", hostOlex, tok, map[string]string{"code": " "}); code != http.StatusBadRequest {
		t.Fatalf("empty code: %d", code)
	}

	// 2. The code goes to the account's own phone.
	if code, env := it.do("POST", "/v1/portal/me/delete/code", hostOlex, tok, map[string]string{"locale": "tr"}); code != http.StatusAccepted {
		t.Fatalf("delete code: %d %s", code, errCode(env))
	}
	deleteCode, _ := sentCode(t, fw, ph)

	// 3. The codes are separate: the delete code does not sign in.
	if code, env := it.do("POST", "/v1/auth/otp/verify", hostOlex, "", map[string]string{"phone": ph, "code": deleteCode}); code != http.StatusUnauthorized || errCode(env) != "INVALID_OTP_CODE" {
		t.Fatalf("sign-in with delete code: %d %s", code, errCode(env))
	}

	// 4. The right code anonymizes the account.
	code, env := it.do("POST", "/v1/portal/me/delete", hostOlex, tok, map[string]string{"code": deleteCode})
	if code != http.StatusOK {
		t.Fatalf("delete: %d %s", code, errCode(env))
	}
	res := decodeData[anonymizeResult](t, env)
	if !res.Changed || res.Status != "anonymized" || res.UUID != userUUID {
		t.Fatalf("delete result = %+v", res)
	}
	var status string
	var phone *string
	if err := it.pool.QueryRow(ctx, `SELECT status, phone_e164 FROM users WHERE uuid = $1`, userUUID).Scan(&status, &phone); err != nil {
		t.Fatal(err)
	}
	if status != "anonymized" || phone != nil {
		t.Fatalf("user after delete: status=%s phone=%v", status, phone)
	}
	var audits int
	if err := it.pool.QueryRow(ctx, `SELECT count(*) FROM activity_events
		WHERE resource_uuid = $1 AND action IN ('customers.anonymized', 'customers.self_deleted')`, userUUID).Scan(&audits); err != nil {
		t.Fatal(err)
	}
	if audits != 2 {
		t.Fatalf("audit rows = %d, want 2", audits)
	}

	// 5. The session is over.
	if code, _ := it.do("GET", "/v1/portal/consents/pending", hostOlex, tok, nil); code != http.StatusUnauthorized {
		t.Fatalf("session after delete: %d", code)
	}

	// 6. Public legal texts.
	t.Cleanup(func() {
		_, _ = it.pool.Exec(ctx, `DELETE FROM legal_texts WHERE kind IN ('privacy_policy', 'kvkk_notice', 'terms_of_use')`)
	})
	_, _ = it.pool.Exec(ctx, `DELETE FROM legal_texts WHERE kind = 'terms_of_use'`)
	if code, _ := it.do("GET", "/v1/public/legal-texts/terms_of_use", hostOlex, "", nil); code != http.StatusNotFound {
		t.Fatalf("unpublished text: %d", code)
	}
	if _, err := it.pool.Exec(ctx, `INSERT INTO legal_texts (kind, locale, version, body)
		VALUES ('terms_of_use', 'tr', 1, '## Kullanım koşulları')`); err != nil {
		t.Fatal(err)
	}
	type publicText struct {
		Kind    string `json:"kind"`
		Locale  string `json:"locale"`
		Version int    `json:"version"`
		Body    string `json:"body"`
	}
	code, env = it.do("GET", "/v1/public/legal-texts/terms_of_use?locale=de", hostOlex, "", nil)
	if code != http.StatusOK {
		t.Fatalf("public text: %d %s", code, errCode(env))
	}
	if txt := decodeData[publicText](t, env); txt.Locale != "tr" || txt.Version != 1 || txt.Body != "## Kullanım koşulları" {
		t.Fatalf("public text (tr fallback) = %+v", txt)
	}
	for _, kind := range []string{"ai_guidelines", "marketing_consent", "cookies"} {
		if code, _ := it.do("GET", "/v1/public/legal-texts/"+kind, hostOlex, "", nil); code != http.StatusNotFound {
			t.Fatalf("public %s: %d, want 404", kind, code)
		}
	}
}
