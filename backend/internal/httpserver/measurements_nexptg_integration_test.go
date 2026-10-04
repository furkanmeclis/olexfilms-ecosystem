package httpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"testing"
)

// TEC-294 acceptance (F3-02b): a mobile upload is normalized in its insert
// transaction (readings, tires, measured_at, device registry); a broken raw
// is still 202 with parsed_at NULL; PATCH /v1/measurements/{uuid}/vin turns
// vin_pending into accepted, an invalid VIN is 400, another organization's
// measurement 404 and another VIN later 422.
func TestIntegrationMeasurementNormalizeAndVIN(t *testing.T) {
	it := newIntegration(t)
	ctx := context.Background()
	center := it.brandCenter("olex")
	orgA := it.org("t294a", "dealer", center)
	orgB := it.org("t294b", "dealer", center)
	owner, pw := it.user("t294-owner")
	other, otherPW := it.user("t294-other")
	it.member(orgA, owner, "owner")
	it.member(orgB, other, "owner")
	t.Cleanup(func() {
		bg := context.Background()
		_, _ = it.pool.Exec(bg, "DELETE FROM measurement_results WHERE organization_id IN ($1, $2)", orgA.ID, orgB.ID)
		_, _ = it.pool.Exec(bg, "DELETE FROM measurement_devices WHERE organization_id IN ($1, $2)", orgA.ID, orgB.ID)
		_, _ = it.pool.Exec(bg, "DELETE FROM refresh_tokens WHERE user_id IN ($1, $2)", owner.ID, other.ID)
	})

	raw, err := os.ReadFile("../modules/measurements/usecase/testdata/nexptg_mobile.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture map[string]any
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	fixture["client_measurement_id"] = "t294-" + it.suffix

	tp := it.mobileLogin(owner.Email.String, pw, orgA.Slug, "t294-dev")
	upload := func(body map[string]any) string {
		t.Helper()
		code, env, _ := it.doMobileKey("POST", "/v1/mobile/measurements", tp.AccessToken, "", body)
		if code != http.StatusAccepted {
			t.Fatalf("upload = %d %s", code, errCode(env))
		}
		var out struct {
			UUID   string `json:"uuid"`
			Status string `json:"status"`
		}
		if err := json.Unmarshal(env.Data, &out); err != nil || out.Status != "vin_pending" {
			t.Fatalf("upload answer = %+v, %v", out, err)
		}
		return out.UUID
	}
	type stored struct {
		parsed, device bool
		values, tires  int
		body, model    string
	}
	read := func(id string) stored {
		t.Helper()
		var s stored
		if err := it.pool.QueryRow(ctx, `SELECT mr.parsed_at IS NOT NULL, mr.device_id IS NOT NULL, COALESCE(mr.body_type, ''),
			COALESCE(md.model, ''),
			(SELECT count(*) FROM measurement_values v WHERE v.result_id = mr.id),
			(SELECT count(*) FROM measurement_tires t WHERE t.result_id = mr.id)
			FROM measurement_results mr LEFT JOIN measurement_devices md ON md.id = mr.device_id
			WHERE mr.uuid = $1`, id).Scan(&s.parsed, &s.device, &s.body, &s.model, &s.values, &s.tires); err != nil {
			t.Fatal(err)
		}
		return s
	}

	// 1. The NexPTG fixture is normalized on upload.
	good := upload(fixture)
	if s := read(good); !s.parsed || !s.device || s.values != 15 || s.tires != 4 || s.body != "SEDAN" || s.model != "Professional" {
		t.Fatalf("normalized = %+v", s)
	}

	// 2. A broken raw is accepted and stays unparsed.
	broken := upload(map[string]any{"raw": map[string]any{"device": "NexPTG"}})
	if s := read(broken); s.parsed || s.device || s.values != 0 {
		t.Fatalf("broken = %+v", s)
	}

	// 3. VIN completion.
	tokA := it.loginOrg(owner, pw, orgA)
	tokB := it.loginOrg(other, otherPW, orgB)
	patch := func(tok, id, vin string) (int, string, json.RawMessage) {
		t.Helper()
		code, env := it.do("PATCH", "/v1/measurements/"+id+"/vin", hostOlex, tok, map[string]any{"vin": vin})
		return code, errCode(env), env.Data
	}
	if code, ec, _ := patch(tokA, good, "WVWZZZ3CZKE01234O"); code != http.StatusBadRequest || ec != "VALIDATION_ERROR" {
		t.Fatalf("invalid vin = %d %s", code, ec)
	}
	if code, _, _ := patch(tokB, good, "WVWZZZ3CZKE012345"); code != http.StatusNotFound {
		t.Fatalf("foreign = %d", code)
	}
	code, ec, data := patch(tokA, good, "wvwzzz3czke012345")
	if code != http.StatusOK {
		t.Fatalf("complete = %d %s", code, ec)
	}
	var detail struct {
		Status string `json:"status"`
		VIN    string `json:"vin"`
		Values []any  `json:"values"`
	}
	if err := json.Unmarshal(data, &detail); err != nil || detail.Status != "accepted" || detail.VIN != "WVWZZZ3CZKE012345" ||
		len(detail.Values) != 15 {
		t.Fatalf("completed = %+v, %v", detail, err)
	}
	if code, ec, _ := patch(tokA, good, "WVWZZZ3CZKE099999"); code != http.StatusUnprocessableEntity || ec != "MEASUREMENT_VIN_ALREADY_SET" {
		t.Fatalf("other vin = %d %s", code, ec)
	}
}
