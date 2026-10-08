package pipeline

// visitor.go (TEC-397, F4-02e) is the unidentified WhatsApp visitor flow
// on top of the pipeline:
//
//	location: a shared WhatsApp location is answered without a model call
//	  with the three nearest dealers (distance order) of the brand;
//	  visitors and customers
//	visitor_limit: at most VisitorDailyTurns AI turns per visitor number
//	  per day, and all visitors together at most ai.visitor_daily_token_cap
//	  tokens of the system pool per day; above → a fixed text with the
//	  dealer finder link, no model call
//	request_dealer_contact (visitor tool): an appointment / offer request
//	  becomes a customer lead (leads module, source whatsapp, public lead
//	  routing). The first call only sends the KVKK notice in the visitor's
//	  language; a lead is written only by a call in a later turn, after the
//	  visitor answered the notice. One lead per number per 30 days (a later
//	  request updates it); conversations.visitor_lead_id links it.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	aitools "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/ai/tools"
	leadsusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/leads/usecase"
	orgusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/organizations/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/whatsapp/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/whatsapp"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

var dealerReferralRe = regexp.MustCompile(`(?:^|\s)#([a-z0-9][a-z0-9-]{0,99})(?:\s|$)`)

// Visitor flow limits.
const (
	// VisitorDailyTurns is how many AI turns one visitor number gets per
	// day (the business day of the brand center's time zone).
	VisitorDailyTurns = 30
	// LocationDealers is how many dealers a shared location is answered
	// with.
	LocationDealers = 3
	// dealerFinderPath is the public dealer finder of the frontend.
	dealerFinderPath = "/portal/dealers"
)

// Visitor stages and skip reasons.
const (
	StageLocation     = "location"
	StageVisitorLimit = "visitor_limit"
	// StageKVKKNotice marks the run that sent the KVKK notice of a lead
	// request; StageLead the run that stored the lead.
	StageKVKKNotice = "kvkk_notice"
	StageLead       = "lead"

	ReasonVisitorTurns  = "visitor_daily_turns"
	ReasonVisitorTokens = "visitor_daily_tokens"
)

// VisitorLeads stores visitor lead requests (*leadsusecase.Applications).
type VisitorLeads interface {
	UpsertVisitorLead(ctx context.Context, brandID int64, in leadsusecase.VisitorLeadInput) (leadsusecase.VisitorLeadResult, error)
}

// VisitorSettings are the system settings of the visitor limits
// (sysconfig.Service).
type VisitorSettings interface {
	AIVisitorDailyTokenCap(ctx context.Context) int64
}

// --- location -------------------------------------------------------------------

type locationMeta struct {
	Type      string   `json:"type"`
	Caption   string   `json:"caption"`
	Latitude  *float64 `json:"latitude"`
	Longitude *float64 `json:"longitude"`
}

func locationOf(m db.Message) (locationMeta, bool) {
	if len(m.Media) == 0 {
		return locationMeta{}, false
	}
	var l locationMeta
	if err := json.Unmarshal(m.Media, &l); err != nil || l.Type != whatsapp.MediaLocation || l.Latitude == nil || l.Longitude == nil {
		return locationMeta{}, false
	}
	return l, true
}

// location answers a shared location (the newest message of the batch) of
// a visitor or customer with the nearest dealers; done means the run ends.
func (p *Pipeline) location(ctx context.Context, r *run, a actor, batch []db.Message) (bool, error) {
	if p.d.Dealers == nil || a.principal.Org != nil {
		return false, nil
	}
	loc, ok := locationOf(batch[len(batch)-1])
	if !ok {
		return false, nil
	}
	rows, err := p.d.Dealers.NearbyDealers(ctx, a.brand.ID, orgusecase.NearbyInput{
		Lat: *loc.Latitude, Lng: *loc.Longitude, RadiusKm: orgusecase.NearbyMaxRadiusKm,
	})
	if err != nil {
		return true, fmt.Errorf("whatsapp ai: nearby dealers: %w", err)
	}
	if len(rows) > LocationDealers {
		rows = rows[:LocationDealers]
	}
	r.stage(p.now(), StageLocation, map[string]any{"dealers": len(rows)})
	if len(rows) == 0 {
		return true, p.sendSystem(ctx, r, textf(r.locale, textLocationNone, p.dealerFinderURL()))
	}
	return true, p.sendSystem(ctx, r, p.dealerList(r.locale, rows))
}

// dealerList renders the nearest dealers as a WhatsApp message.
func (p *Pipeline) dealerList(locale string, rows []orgusecase.NearbyDealer) string {
	var sb strings.Builder
	sb.WriteString(text(locale, textLocationDealers))
	for i, d := range rows {
		fmt.Fprintf(&sb, "\n\n%d. *%s*", i+1, strings.TrimSpace(d.Name))
		place := strings.Join(nonEmpty(d.District, d.City), ", ")
		if place != "" {
			fmt.Fprintf(&sb, "\n%s", place)
		}
		fmt.Fprintf(&sb, " (%s km)", formatKm(d.DistanceKm))
		if d.WhatsApp != nil && *d.WhatsApp != "" {
			fmt.Fprintf(&sb, "\nWhatsApp: %s", *d.WhatsApp)
		}
		if base := p.frontendURL(); base != "" && d.Slug != "" {
			fmt.Fprintf(&sb, "\n%s/bayi/%s", base, d.Slug)
		}
	}
	return sb.String()
}

func formatKm(km float64) string {
	if km < 10 {
		return fmt.Sprintf("%.1f", km)
	}
	return fmt.Sprintf("%.0f", km)
}

func nonEmpty(vals ...string) []string {
	var out []string
	for _, v := range vals {
		if v = strings.TrimSpace(v); v != "" {
			out = append(out, v)
		}
	}
	return out
}

func (p *Pipeline) frontendURL() string { return strings.TrimRight(p.d.FrontendURL, "/") }

// dealerFinderURL is the public dealer finder link of the fixed texts.
func (p *Pipeline) dealerFinderURL() string {
	if base := p.frontendURL(); base != "" {
		return base + dealerFinderPath
	}
	return dealerFinderPath
}

// --- abuse limits ---------------------------------------------------------------

// dayStart is the start of the current day in the center's time zone.
func (p *Pipeline) dayStart(tz string) time.Time {
	loc, err := time.LoadLocation(tz)
	if err != nil {
		loc, _ = time.LoadLocation(defaultTimezone)
	}
	if loc == nil {
		loc = time.UTC
	}
	n := p.now().In(loc)
	return time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, loc)
}

// visitorLimits applies the per number turn limit and the daily visitor
// token cap; done means the run ends with the fixed text.
func (p *Pipeline) visitorLimits(ctx context.Context, r *run, a actor) (bool, error) {
	if !a.facts.Visitor {
		return false, nil
	}
	since := pgtype.Timestamptz{Time: p.dayStart(a.facts.Timezone), Valid: true}
	q := p.d.Queries
	turns, err := q.CountConversationModelRunsSince(ctx, db.CountConversationModelRunsSinceParams{
		ConversationID: r.conv.ID, Since: since,
	})
	if err != nil {
		return true, err
	}
	reason := ""
	if turns >= VisitorDailyTurns {
		reason = ReasonVisitorTurns
	}
	var used, limit int64
	if reason == "" && p.d.VisitorSettings != nil {
		if limit = p.d.VisitorSettings.AIVisitorDailyTokenCap(ctx); limit > 0 {
			if used, err = q.SumVisitorWhatsAppTokensSince(ctx, db.SumVisitorWhatsAppTokensSinceParams{
				OrganizationID: a.quotaOrg, Since: since,
			}); err != nil {
				return true, err
			}
			if used >= limit {
				reason = ReasonVisitorTokens
			}
		}
	}
	detail := map[string]any{"turns": turns}
	if limit > 0 {
		detail["tokens"], detail["token_cap"] = used, limit
	}
	if reason == "" {
		r.stage(p.now(), StageVisitorLimit, detail)
		return false, nil
	}
	detail["reason"] = reason
	r.stage(p.now(), StageVisitorLimit, detail)
	r.status = model.RunSkipped
	return true, p.sendSystem(ctx, r, textf(r.locale, textVisitorLimit, p.dealerFinderURL()))
}

func (p *Pipeline) dealerReferral(ctx context.Context, r *run, a actor, batch []db.Message) error {
	if !a.facts.Visitor || r.conv.ReferredDealerOrgID.Valid {
		return nil
	}
	for _, m := range batch {
		match := dealerReferralRe.FindStringSubmatch(strings.ToLower(messageText(m)))
		if len(match) != 2 {
			continue
		}
		org, err := p.d.Queries.GetActiveDealerBySlug(ctx, db.GetActiveDealerBySlugParams{
			Slug: match[1], BrandID: a.brand.ID,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		conv, err := p.d.Queries.SetConversationReferredDealer(ctx, db.SetConversationReferredDealerParams{
			ID: r.conv.ID, ReferredDealerOrgID: pgtype.Int8{Int64: org.ID, Valid: true},
		})
		if err != nil {
			return err
		}
		r.conv = conv
		r.stage(p.now(), "dealer_referral", map[string]any{"dealer_code": org.Slug, "organization_id": org.ID})
		return nil
	}
	return nil
}

// --- lead tool -------------------------------------------------------------------

// visitorTurn is the conversation state of an agent turn, for the tools of
// this package (request_dealer_contact).
type visitorTurn struct {
	p *Pipeline
	r *run
	a actor
	// noticeSent: this turn already sent the KVKK notice.
	noticeSent bool
}

type visitorTurnKey struct{}

func withVisitorTurn(ctx context.Context, t *visitorTurn) context.Context {
	return context.WithValue(ctx, visitorTurnKey{}, t)
}

func visitorTurnFrom(ctx context.Context) (*visitorTurn, bool) {
	t, ok := ctx.Value(visitorTurnKey{}).(*visitorTurn)
	return t, ok && t != nil
}

// RequestDealerContact is the visitor tool that turns an appointment or
// offer request into a lead. It only works inside a WhatsApp pipeline turn.
type RequestDealerContact struct{}

// Spec implements aitools.Tool.
func (RequestDealerContact) Spec() aitools.Spec {
	str := func(desc string, max int) map[string]any {
		return map[string]any{"type": "string", "maxLength": max, "description": desc}
	}
	return aitools.Spec{
		Name: "request_dealer_contact",
		Description: "Forward the visitor's appointment, offer or call-back request to the brand's dealer network as a " +
			"lead. Needs the visitor's name and city; add district, vehicle (brand, model, year) and a short note. " +
			"The first call only sends the privacy (KVKK) notice to the visitor and stores nothing: ask them to " +
			"confirm, and call again in a later turn after they replied. A request within 30 days updates the earlier one.",
		InputSchema: map[string]any{
			"type": "object", "additionalProperties": false, "required": []any{"name", "city"},
			"properties": map[string]any{
				"name":     str("The visitor's name.", 200),
				"city":     str("City (province) name.", 200),
				"district": str("Optional district.", 200),
				"vehicle":  str("Vehicle brand, model and year.", 500),
				"intent": map[string]any{"type": "string", "enum": []any{
					leadsusecase.IntentAppointment, leadsusecase.IntentQuote, leadsusecase.IntentInfo,
				}, "description": "appointment, quote (offer) or info (call back)."},
				"note": str("Optional short note: the product or service asked about.", 500),
			},
		},
		Kind:  aitools.KindSelf,
		Realm: aitools.RealmVisitor,
	}
}

// Lead tool statuses (result field status).
const (
	LeadStatusNoticeSent    = "kvkk_notice_sent"
	LeadStatusAwaitingReply = "awaiting_visitor_reply"
	LeadStatusCreated       = "created"
	LeadStatusUpdated       = "updated"
)

type leadResult struct {
	Status string `json:"status"`
	Note   string `json:"note"`
}

// Run implements aitools.Tool.
func (RequestDealerContact) Run(ctx context.Context, _ aitools.Env, raw json.RawMessage) (aitools.Result, error) {
	t, ok := visitorTurnFrom(ctx)
	if !ok || t.p.d.Leads == nil {
		return aitools.ErrorResult(aitools.CodeToolNotAllowed,
			"request_dealer_contact is only available in a WhatsApp conversation. Direct the person to the nearest dealer."), nil
	}
	var in struct {
		Name     string `json:"name"`
		City     string `json:"city"`
		District string `json:"district"`
		Vehicle  string `json:"vehicle"`
		Intent   string `json:"intent"`
		Note     string `json:"note"`
	}
	if err := json.Unmarshal(raw, &in); err != nil {
		return aitools.ErrorResult(aitools.CodeInvalidInput, "invalid input: "+err.Error()), nil
	}
	return t.request(ctx, leadsusecase.VisitorLeadInput{
		Name: in.Name, City: in.City, District: in.District, Vehicle: in.Vehicle,
		Intent: in.Intent, Message: in.Note,
	})
}

func (t *visitorTurn) request(ctx context.Context, in leadsusecase.VisitorLeadInput) (aitools.Result, error) {
	p, r := t.p, t.r
	if t.noticeSent {
		return aitools.JSONResult(leadResult{Status: LeadStatusAwaitingReply,
			Note: "The privacy notice was just sent. Wait for the visitor's reply; nothing was stored."})
	}
	notice, err := p.noticeFor(ctx, r)
	if err != nil {
		return aitools.Result{}, err
	}
	since := p.now().Add(-leadsusecase.VisitorLeadWindow)
	prior, err := p.d.Queries.CountConversationRunsWithStageBefore(ctx, db.CountConversationRunsWithStageBeforeParams{
		ConversationID: r.conv.ID, BeforeRunID: r.row.ID,
		Since: pgtype.Timestamptz{Time: since, Valid: true}, Stage: StageKVKKNotice,
	})
	if err != nil {
		return aitools.Result{}, err
	}
	if prior == 0 {
		// KVKK first: the notice in the visitor's language, nothing stored.
		if err := p.sendSystem(ctx, r, textf(r.locale, textKVKKNotice, toWhatsApp(strings.TrimSpace(notice.Body)))); err != nil {
			return aitools.Result{}, err
		}
		t.noticeSent = true
		r.stage(p.now(), StageKVKKNotice, map[string]any{"locale": notice.Locale, "version": notice.Version})
		return aitools.JSONResult(leadResult{Status: LeadStatusNoticeSent,
			Note: "The privacy (KVKK) notice was sent to the visitor as a separate message; nothing was stored yet. " +
				"Ask them to read it and confirm they want to be contacted; call request_dealer_contact again after they confirm."})
	}

	in.PhoneE164 = r.conv.ContactE164
	in.Language = strings.ReplaceAll(r.locale, "_", "-")
	in.ConversationUUID = r.conv.Uuid.String()
	if r.conv.ReferredDealerOrgID.Valid {
		in.ReferredDealerOrgID = r.conv.ReferredDealerOrgID.Int64
	}
	in.Notice = &leadsusecase.KVKKNotice{Locale: notice.Locale, Version: notice.Version, SentAt: p.now()}
	res, err := p.d.Leads.UpsertVisitorLead(ctx, t.a.brand.ID, in)
	var verr *leadsusecase.ValidationError
	switch {
	case errors.As(err, &verr):
		return aitools.ErrorResult(aitools.CodeInvalidInput, "invalid "+verr.Field+": "+verr.Message+
			". Ask the visitor for it and call again."), nil
	case err != nil:
		return aitools.Result{}, err
	}
	conv, err := p.d.Queries.SetConversationVisitorLead(ctx, db.SetConversationVisitorLeadParams{
		ID: r.conv.ID, VisitorLeadID: pgtype.Int8{Int64: res.Lead.ID, Valid: true},
	})
	if err != nil {
		return aitools.Result{}, err
	}
	r.conv = conv
	status := LeadStatusUpdated
	if res.Created {
		status = LeadStatusCreated
	}
	r.stage(p.now(), StageLead, map[string]any{"status": status, "routed_by": res.RoutedBy})
	return aitools.JSONResult(leadResult{Status: status,
		Note: "The request was saved and forwarded to the dealer network; a dealer will contact the visitor. " +
			"Do not promise a time or a price."})
}

// noticeFor is the newest KVKK notice in the conversation language, else
// English, else Turkish.
func (p *Pipeline) noticeFor(ctx context.Context, r *run) (db.KvkkNotice, error) {
	locales := nonEmpty(strings.ReplaceAll(r.locale, "_", "-"), r.locale, "en", "tr")
	for _, l := range locales {
		n, err := p.d.Queries.GetLatestKVKKNotice(ctx, l)
		if err == nil {
			return n, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return db.KvkkNotice{}, err
		}
	}
	return db.KvkkNotice{}, errors.New("whatsapp ai: no KVKK notice is published")
}
