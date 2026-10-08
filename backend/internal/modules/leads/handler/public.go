package handler

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	docmodel "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/documents/model"
	docusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/documents/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/leads/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/brandctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
	"github.com/google/uuid"
)

// Limiter buckets of the public dealer application form (TEC-317).
const (
	applicationIPAction    = "dealer_application_ip"
	applicationPhoneAction = "dealer_application_phone"
	quotePublicAction      = "quote_public"
	showcaseMinFill        = 3 * time.Second
	showcaseTokenTTL       = 30 * time.Minute
	maxApplicationBody     = 32 << 10
	maxShowcaseLeadBody    = 16 << 10
	applicationNotFound    = "Dealer applications are not available"
	quoteNotFound          = "Quote was not found"
)

// Limiter is the fixed window limiter (ratelimit.Limiter).
type Limiter interface {
	Allow(ctx context.Context, action, subject string, limit int, window time.Duration) (bool, time.Duration)
}

// Applications is the public form use case.
type Applications interface {
	Enabled(ctx context.Context) (bool, error)
	Validate(ctx context.Context, in usecase.ApplicationInput) (usecase.Application, error)
	Submit(ctx context.Context, brandID int64, app usecase.Application) (usecase.ApplicationResult, error)
	ShowcaseLeadConfig(ctx context.Context, brandID int64, code, lang, formToken, whatsappText string) (usecase.ShowcaseLeadConfig, error)
	ValidateShowcaseLead(ctx context.Context, brandID int64, code string, in usecase.ShowcaseLeadInput) (usecase.ShowcaseLead, db.GetShowcaseLeadTargetBySlugRow, error)
	SubmitShowcaseLead(ctx context.Context, target db.GetShowcaseLeadTargetBySlugRow, in usecase.ShowcaseLead) (usecase.ShowcaseLeadResult, error)
}

// PublicQuotes is the public read-only quote use case.
type PublicQuotes interface {
	PublicQuote(ctx context.Context, brandID int64, token uuid.UUID, pdfURL string) (usecase.PublicQuote, error)
	PublicQuoteOwner(ctx context.Context, brandID int64, token uuid.UUID) (uuid.UUID, int64, int64, error)
}

// QuoteFiles reads the bytes of a ready render (documents usecase); the
// public PDF file route streams it to anonymous visitors (TEC-320).
type QuoteFiles interface {
	Download(ctx context.Context, organizationID int64, id uuid.UUID) (io.ReadCloser, string, error)
}

// RateLimits caps the form: IPLimit hits per window per client IP (counted
// before validation) and PhoneLimit stored applications per window per
// E.164 phone. Zero disables a limit.
type RateLimits struct {
	IPLimit    int
	PhoneLimit int
	Window     time.Duration
}

// Public serves /v1/public/dealer-applications (no authentication).
type Public struct {
	apps    Applications
	quotes  PublicQuotes
	docs    DocumentRenderer
	files   QuoteFiles
	limiter Limiter
	limits  RateLimits
	secret  []byte
}

// NewPublic builds the handler.
func NewPublic(apps Applications, limiter Limiter, limits RateLimits) *Public {
	return &Public{apps: apps, limiter: limiter, limits: limits}
}

// WithShowcaseSecret enables signed form tokens for public showcase leads.
func (h *Public) WithShowcaseSecret(secret string) *Public {
	h.secret = []byte(secret)
	return h
}

// WithQuotes enables /v1/public/quotes/{token}. When docs can also read
// render bytes (QuoteFiles), /pdf/file serves the PDF itself.
func (h *Public) WithQuotes(quotes PublicQuotes, docs DocumentRenderer) *Public {
	h.quotes = quotes
	h.docs = docs
	if files, ok := docs.(QuoteFiles); ok {
		h.files = files
	}
	return h
}

type applicationConfig struct {
	Enabled bool `json:"enabled"`
}

// Config answers GET /v1/public/dealer-applications/config.
func (h *Public) Config(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if _, ok := brandctx.From(r.Context()); !ok {
		response.NotFound(w, r, applicationNotFound)
		return
	}
	on, err := h.apps.Enabled(r.Context())
	if err != nil {
		response.InternalErr(w, r, err, "dealer application config failed")
		return
	}
	response.JSON(w, r, http.StatusOK, applicationConfig{Enabled: on})
}

type applicationBody struct {
	CompanyName string `json:"company_name"`
	ContactName string `json:"contact_name"`
	Phone       string `json:"phone"`
	Email       string `json:"email"`
	CountryID   int64  `json:"country_id"`
	ProvinceID  *int64 `json:"province_id"`
	DistrictID  *int64 `json:"district_id"`
	Message     string `json:"message"`
	KVKKConsent bool   `json:"kvkk_consent"`
	Language    string `json:"language"`
	// Website is the honeypot: hidden in the form, filled only by bots.
	Website string `json:"website"`
}

type applicationReceived struct {
	Received bool `json:"received"`
}

type showcaseLeadBody struct {
	Name             string   `json:"name"`
	Phone            string   `json:"phone"`
	Email            string   `json:"email"`
	VehicleBrand     string   `json:"vehicle_brand"`
	VehicleModel     string   `json:"vehicle_model"`
	Interested       []string `json:"interested_services"`
	Message          string   `json:"message"`
	PreferredChannel string   `json:"preferred_channel"`
	KVKKConsent      bool     `json:"kvkk_consent"`
	Language         string   `json:"language"`
	FormToken        string   `json:"form_token"`
	Website          string   `json:"website"`
}

// Submit answers POST /v1/public/dealer-applications: 404 while the form
// is closed, 429 + Retry-After over the IP or phone limit, 400
// VALIDATION_ERROR on bad input, else 202 {received: true}. The response
// never tells where the application was routed; a filled honeypot gets
// the same 202 and stores nothing.
func (h *Public) Submit(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	b, ok := brandctx.From(r.Context())
	if !ok {
		response.NotFound(w, r, applicationNotFound)
		return
	}
	on, err := h.apps.Enabled(r.Context())
	if err != nil {
		response.InternalErr(w, r, err, "dealer application config failed")
		return
	}
	if !on {
		response.NotFound(w, r, applicationNotFound)
		return
	}
	if !h.allow(w, r, applicationIPAction, clientIP(r), h.limits.IPLimit) {
		return
	}
	var body applicationBody
	r.Body = http.MaxBytesReader(w, r.Body, maxApplicationBody)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&body); err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "Invalid request body")
		return
	}
	app, err := h.apps.Validate(r.Context(), usecase.ApplicationInput{
		CompanyName: body.CompanyName, ContactName: body.ContactName, Phone: body.Phone, Email: body.Email,
		CountryID: body.CountryID, ProvinceID: body.ProvinceID, DistrictID: body.DistrictID,
		Message: body.Message, KVKKConsent: body.KVKKConsent, Language: body.Language, Honeypot: body.Website,
	})
	if err != nil {
		var ve *usecase.ValidationError
		if errors.As(err, &ve) {
			response.ValidationError(w, r, []response.Detail{{Field: ve.Field, Message: ve.Message, Code: ve.Code}})
			return
		}
		response.InternalErr(w, r, err, "dealer application failed")
		return
	}
	if !app.Spam && !h.allow(w, r, applicationPhoneAction, app.PhoneE164, h.limits.PhoneLimit) {
		return
	}
	if _, err := h.apps.Submit(r.Context(), b.ID, app); err != nil {
		response.InternalErr(w, r, err, "dealer application failed")
		return
	}
	response.JSON(w, r, http.StatusAccepted, applicationReceived{Received: true})
}

// ShowcaseConfig answers GET /v1/public/dealers/{code}/lead-form/config.
func (h *Public) ShowcaseConfig(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	b, ok := brandctx.From(r.Context())
	if !ok {
		response.NotFound(w, r, "Dealer was not found")
		return
	}
	code := r.PathValue("code")
	token := h.signShowcaseToken(code, time.Now().UTC())
	waText := url.QueryEscape(showcaseWhatsAppText(r.URL.Query().Get("lang"), code))
	cfg, err := h.apps.ShowcaseLeadConfig(r.Context(), b.ID, code, r.URL.Query().Get("lang"), token, waText)
	if err != nil {
		if errors.Is(err, usecase.ErrNotFound) {
			response.NotFound(w, r, "Dealer was not found")
			return
		}
		response.InternalErr(w, r, err, "showcase lead config failed")
		return
	}
	response.JSON(w, r, http.StatusOK, cfg)
}

// ShowcaseSubmit answers POST /v1/public/dealers/{code}/leads.
func (h *Public) ShowcaseSubmit(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	b, ok := brandctx.From(r.Context())
	if !ok {
		response.NotFound(w, r, "Dealer was not found")
		return
	}
	code := r.PathValue("code")
	ip := clientIP(r)
	if !h.allowWindow(w, r, usecase.WebsiteIPAction, ip, 5, time.Hour) {
		return
	}
	var body showcaseLeadBody
	r.Body = http.MaxBytesReader(w, r.Body, maxShowcaseLeadBody)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&body); err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "Invalid request body")
		return
	}
	issued, ok := h.verifyShowcaseToken(code, body.FormToken)
	if !ok {
		response.BadRequest(w, r, response.CodeValidationError, "form_token is invalid")
		return
	}
	if time.Since(issued) < showcaseMinFill {
		response.BadRequest(w, r, response.CodeValidationError, "form was submitted too quickly")
		return
	}
	lead, target, err := h.apps.ValidateShowcaseLead(r.Context(), b.ID, code, usecase.ShowcaseLeadInput{
		Name: body.Name, Phone: body.Phone, Email: body.Email, VehicleBrand: body.VehicleBrand,
		VehicleModel: body.VehicleModel, Interested: body.Interested, Message: body.Message,
		PreferredChannel: body.PreferredChannel, KVKKConsent: body.KVKKConsent, Language: body.Language,
		Honeypot: body.Website, TokenIssuedAt: issued, RemoteIP: ip, UserAgent: r.UserAgent(),
	})
	if err != nil {
		if errors.Is(err, usecase.ErrNotFound) {
			response.NotFound(w, r, "Dealer was not found")
			return
		}
		var ve *usecase.ValidationError
		if errors.As(err, &ve) {
			status := http.StatusBadRequest
			if ve.Field == "kvkk_consent" {
				status = http.StatusUnprocessableEntity
			}
			response.ErrorWithDetails(w, r, status, response.CodeValidationError, ve.Message,
				[]response.Detail{{Field: ve.Field, Message: ve.Message, Code: ve.Code}})
			return
		}
		response.InternalErr(w, r, err, "showcase lead failed")
		return
	}
	if !lead.Spam && !h.allowWindow(w, r, usecase.WebsitePhoneAction, lead.PhoneE164, 3, 24*time.Hour) {
		return
	}
	if _, err := h.apps.SubmitShowcaseLead(r.Context(), target, lead); err != nil {
		response.InternalErr(w, r, err, "showcase lead failed")
		return
	}
	if lead.Spam {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	response.JSON(w, r, http.StatusAccepted, applicationReceived{Received: true})
}

func (h *Public) signShowcaseToken(code string, issued time.Time) string {
	if len(h.secret) == 0 {
		return ""
	}
	payload := strings.ToLower(strings.TrimSpace(code)) + "|" + strconv.FormatInt(issued.Unix(), 10)
	mac := hmac.New(sha256.New, h.secret)
	_, _ = mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString([]byte(payload + "|" + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))))
}

func (h *Public) verifyShowcaseToken(code, token string) (time.Time, bool) {
	if len(h.secret) == 0 {
		return time.Now().Add(-showcaseMinFill), true
	}
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(token))
	if err != nil {
		return time.Time{}, false
	}
	parts := strings.Split(string(raw), "|")
	if len(parts) != 3 || parts[0] != strings.ToLower(strings.TrimSpace(code)) {
		return time.Time{}, false
	}
	sec, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return time.Time{}, false
	}
	issued := time.Unix(sec, 0).UTC()
	if time.Since(issued) < 0 || time.Since(issued) > showcaseTokenTTL {
		return time.Time{}, false
	}
	payload := parts[0] + "|" + parts[1]
	mac := hmac.New(sha256.New, h.secret)
	_, _ = mac.Write([]byte(payload))
	want := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return issued, hmac.Equal([]byte(want), []byte(parts[2]))
}

func showcaseWhatsAppText(lang, code string) string {
	if strings.TrimSpace(lang) == "en" {
		return "Hello, I would like information about your services. #" + code
	}
	return "Merhaba, hizmetleriniz hakkında bilgi almak istiyorum. #" + code
}

// Quote answers GET /v1/public/quotes/{token}; it never exposes recipient
// phone or e-mail.
func (h *Public) Quote(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Robots-Tag", "noindex, nofollow")
	if h.quotes == nil {
		response.NotFound(w, r, quoteNotFound)
		return
	}
	if !h.allow(w, r, quotePublicAction, clientIP(r), h.limits.IPLimit) {
		return
	}
	token, b, ok := h.publicQuoteToken(w, r)
	if !ok {
		return
	}
	out, err := h.quotes.PublicQuote(r.Context(), b.ID, token, r.URL.Path+"/pdf")
	if err != nil {
		if errors.Is(err, usecase.ErrQuoteNotFound) {
			response.NotFound(w, r, quoteNotFound)
			return
		}
		response.InternalErr(w, r, err, "quote public lookup failed")
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// QuotePDF answers GET /v1/public/quotes/{token}/pdf.
func (h *Public) QuotePDF(w http.ResponseWriter, r *http.Request) {
	v, ready, _, ok := h.quoteRender(w, r)
	if !ok {
		return
	}
	status := http.StatusAccepted
	if ready {
		status = http.StatusOK
	}
	response.JSON(w, r, status, v)
}

// QuotePDFFile answers GET /v1/public/quotes/{token}/pdf/file (TEC-320):
// the PDF bytes once the render is ready, else 202 with the queued render
// so the caller can retry. The tenant download_url needs a session, so the
// public page downloads through here. Same limit bucket as /pdf.
func (h *Public) QuotePDFFile(w http.ResponseWriter, r *http.Request) {
	if h.files == nil {
		w.Header().Set("Cache-Control", "no-store")
		response.NotFound(w, r, quoteNotFound)
		return
	}
	v, ready, orgID, ok := h.quoteRender(w, r)
	if !ok {
		return
	}
	if !ready {
		response.JSON(w, r, http.StatusAccepted, v)
		return
	}
	rc, name, err := h.files.Download(r.Context(), orgID, v.UUID)
	if err != nil {
		response.InternalErr(w, r, err, "quote public pdf failed")
		return
	}
	defer func() { _ = rc.Close() }()
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"; filename*=UTF-8''%s`, name, url.PathEscape(name)))
	_, _ = io.Copy(w, rc)
}

// quoteRender requests (or finds) the quote's PDF render for a public
// token; on false the response is already written.
func (h *Public) quoteRender(w http.ResponseWriter, r *http.Request) (docmodel.RenderView, bool, int64, bool) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Robots-Tag", "noindex, nofollow")
	if h.quotes == nil || h.docs == nil {
		response.NotFound(w, r, quoteNotFound)
		return docmodel.RenderView{}, false, 0, false
	}
	if !h.allow(w, r, quotePublicAction+"_pdf", clientIP(r), h.limits.IPLimit) {
		return docmodel.RenderView{}, false, 0, false
	}
	token, b, ok := h.publicQuoteToken(w, r)
	if !ok {
		return docmodel.RenderView{}, false, 0, false
	}
	quoteUUID, orgID, brandID, err := h.quotes.PublicQuoteOwner(r.Context(), b.ID, token)
	if err != nil {
		if errors.Is(err, usecase.ErrQuoteNotFound) {
			response.NotFound(w, r, quoteNotFound)
			return docmodel.RenderView{}, false, 0, false
		}
		response.InternalErr(w, r, err, "quote public pdf failed")
		return docmodel.RenderView{}, false, 0, false
	}
	v, ready, err := h.docs.RequestRender(r.Context(), docmodel.Viewer{
		OrganizationID: orgID, BrandID: brandID, System: true,
	}, docusecase.RenderInput{Kind: docmodel.KindQuote, SourceID: quoteUUID.String(), Locale: r.URL.Query().Get("locale")})
	if err != nil {
		response.InternalErr(w, r, err, "quote public pdf failed")
		return docmodel.RenderView{}, false, 0, false
	}
	return v, ready, orgID, true
}

func (h *Public) publicQuoteToken(w http.ResponseWriter, r *http.Request) (uuid.UUID, brandctx.Brand, bool) {
	b, ok := brandctx.From(r.Context())
	if !ok {
		response.NotFound(w, r, quoteNotFound)
		return uuid.Nil, brandctx.Brand{}, false
	}
	token, err := uuid.Parse(r.PathValue("token"))
	if err != nil {
		response.NotFound(w, r, quoteNotFound)
		return uuid.Nil, brandctx.Brand{}, false
	}
	return token, b, true
}

func (h *Public) allow(w http.ResponseWriter, r *http.Request, action, subject string, limit int) bool {
	return h.allowWindow(w, r, action, subject, limit, h.limits.Window)
}

func (h *Public) allowWindow(w http.ResponseWriter, r *http.Request, action, subject string, limit int, window time.Duration) bool {
	if h.limiter == nil || limit <= 0 || window <= 0 {
		return true
	}
	ok, retry := h.limiter.Allow(r.Context(), action, subject, limit, window)
	if ok {
		return true
	}
	secs := int(retry.Seconds())
	if secs < 1 {
		secs = 1
	}
	w.Header().Set("Retry-After", strconv.Itoa(secs))
	response.TooManyRequests(w, r, "")
	return false
}

// clientIP is the client address the BFF forwards (first X-Forwarded-For
// hop, same as the other public limits), else the socket peer.
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if ip := strings.TrimSpace(strings.Split(xff, ",")[0]); ip != "" {
			return ip
		}
	}
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}
