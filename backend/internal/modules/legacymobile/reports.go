package legacymobile

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
)

var legacyVIN = regexp.MustCompile(`^[A-Z0-9]{11,17}$`)

// legacyDate reads the hub's "nullable|date" values (RFC 3339, or the
// MySQL / date-only forms Carbon also parsed) as RFC 3339; "" when none.
func legacyDate(s string) string {
	s = strings.TrimSpace(s)
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05", "2006-01-02 15:04:05", "2006-01-02"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.Format(time.RFC3339)
		}
	}
	return ""
}

// reportKeys are the NexptgReport fields the hub's StoreNexptgReportRequest
// accepted; the answer echoes them (MobileNexptgReportResource keys).
var reportKeys = []string{
	"name", "date", "calibration_date", "device_serial_number", "car_brand_id", "car_model_id",
	"body_type", "capacity", "power", "vin", "fuel_type", "year", "unit_of_measure", "comment", "extra_fields",
}

// storeReport is POST {Prefix}/nexptg-reports: NexptgReportController::store,
// the measurement upload of the old app.
//
//	request  {body_type, car_brand_id, car_model_id, year, name?, date?,
//	          device_serial_number?, vin?, ..., measurements: [{part_type,
//	          place_id, position, value?, interpretation?, substrate_type?,
//	          is_inside?, timestamp?}]}
//	response 201 {success, message: "NexPTG raporu kaydedildi.", data: report}
//
// It becomes a POST /v1/mobile/measurements upload (K28, TEC-233): the old
// body is kept whole as raw; vin, date and device_serial_number fill the
// columns when they are valid for them (an invalid VIN leaves the row
// vin_pending instead of refusing the upload). The old report is not tied
// to a service (the hub matched it later), so neither is the upload. The
// answer echoes the report fields with the stored row's id; the parsed
// views of the hub (labels, visualization, tires) are not rebuilt here.
func (a *adapters) storeReport(w http.ResponseWriter, r *http.Request) {
	obj, okBody := readObject(r)
	if !okBody {
		validationFailed(w, "measurements", "En az bir ölçüm gönderilmelidir.")
		return
	}
	var measurements []json.RawMessage
	if raw, found := obj["measurements"]; !found || json.Unmarshal(raw, &measurements) != nil || len(measurements) == 0 {
		validationFailed(w, "measurements", "En az bir ölçüm gönderilmelidir.")
		return
	}
	clientID := "legacy-" + uuid.NewString()
	body := map[string]any{"client_measurement_id": clientID, "raw": obj}
	if vin := strings.ToUpper(str(obj, "vin")); legacyVIN.MatchString(vin) {
		body["vin"] = vin
	}
	if at := legacyDate(str(obj, "date")); at != "" {
		body["measured_at"] = at
	}
	if serial := str(obj, "device_serial_number"); serial != "" && utf8.RuneCountInString(serial) <= 64 {
		body["device"] = map[string]string{"serial": serial}
	}
	c := run(a.h.CreateMeasurement, withJSON(r, body))
	var accepted struct {
		UUID   uuid.UUID `json:"uuid"`
		Status string    `json:"status"`
	}
	if !ok(c, &accepted) {
		passThrough(w, c)
		return
	}
	out := map[string]any{
		"id": nil, "uuid": accepted.UUID, "status": accepted.Status, "external_id": nil,
		"brand": nil, "model": nil, "car_brand": nil, "car_model": nil, "type_of_body": nil,
		"measurements": measurements, "measurements_count": len(measurements),
		"tires": []any{}, "tires_count": 0,
		"is_matched": false, "service_id": nil, "service_match_type": nil, "service_match_type_label": nil,
		"api_user": nil, "visualization": nil,
	}
	for _, k := range reportKeys {
		if v, found := obj[k]; found {
			out[k] = v
		} else {
			out[k] = nil
		}
	}
	now := iso(time.Now())
	out["created_at"], out["updated_at"] = now, now
	if p, found := authctx.PrincipalFrom(r.Context()); found {
		out["user"] = map[string]any{"id": p.UserInternal, "name": nil, "email": p.Email}
	}
	if a.store != nil {
		if sc, found := orgctx.ScopeFrom(r.Context()); found {
			row, err := a.store.FindMeasurementResultByKeys(r.Context(), db.FindMeasurementResultByKeysParams{
				OrganizationID: sc.InternalID, ClientMeasurementID: pgtype.Text{String: clientID, Valid: true},
				IdempotencyKey: pgtype.Text{String: strings.TrimSpace(r.Header.Get("Idempotency-Key")),
					Valid: strings.TrimSpace(r.Header.Get("Idempotency-Key")) != ""},
			})
			if err == nil {
				out["id"] = row.ID
				if row.CreatedAt.Valid {
					out["created_at"], out["updated_at"] = iso(row.CreatedAt.Time), iso(row.CreatedAt.Time)
				}
			}
		}
	}
	writeSuccess(w, http.StatusCreated, message(legacyLocale(r), msgReportSaved), out)
}

// putPushToken is PUT {Prefix}/push-token: PushTokenController::upsert.
//
//	request  {expo_push_token, platform: ios|android, device_name?}
//	response {success, message: "Push token kaydedildi.", data: null}
func (a *adapters) putPushToken(w http.ResponseWriter, r *http.Request) {
	obj, _ := readObject(r)
	locale := legacyLocale(r)
	token, platform := str(obj, "expo_push_token"), strings.ToLower(str(obj, "platform"))
	switch {
	case token == "":
		validationFailed(w, "expo_push_token", message(locale, msgPushRequired))
		return
	case platform == "":
		validationFailed(w, "platform", message(locale, msgPlatformMissing))
		return
	case platform != "ios" && platform != "android":
		validationFailed(w, "platform", message(locale, msgPlatformIn))
		return
	}
	body := map[string]string{"expo_push_token": token, "platform": platform}
	if n := str(obj, "device_name"); n != "" {
		body["device_name"] = n
	}
	c := run(a.h.PutPushToken, withJSON(r, body))
	if !ok(c, nil) {
		passThrough(w, c)
		return
	}
	writeSuccess(w, http.StatusOK, message(locale, msgPushSaved), nil)
}

// deletePushToken is DELETE {Prefix}/push-token: PushTokenController::destroy.
//
//	request  {expo_push_token}
//	response {success, message: "Push token silindi.", data: null}
func (a *adapters) deletePushToken(w http.ResponseWriter, r *http.Request) {
	obj, _ := readObject(r)
	locale := legacyLocale(r)
	token := str(obj, "expo_push_token")
	if token == "" {
		validationFailed(w, "expo_push_token", message(locale, msgPushRequired))
		return
	}
	c := run(a.h.DeletePushToken, withJSON(r, map[string]string{"expo_push_token": token}))
	if !ok(c, nil) {
		passThrough(w, c)
		return
	}
	writeSuccess(w, http.StatusOK, message(locale, msgPushDeleted), nil)
}
