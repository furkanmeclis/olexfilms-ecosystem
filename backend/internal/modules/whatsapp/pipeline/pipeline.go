// Package pipeline is the WhatsApp AI pipeline worker (TEC-396, F4-02c).
//
// whatsapp.message.received → whatsapp:ai_reply (ProcessIn 4 s + Unique per
// conversation, so a burst becomes one task) → Process under a Redis lock
// per conversation. One run handles every unanswered inbound message (all
// contact messages after the last run's trigger) and writes one
// conversation_ai_runs row; the newest message is the trigger, whose
// UNIQUE constraint makes a redelivered task a no-op.
//
// Stages, in order (each recorded on the run):
//
//	keywords (no AI, localized): DUR/STOP → marketing opt-out;
//	  İNSAN/AGENT → AI paused, conversation pending, staff notified
//	gate: ai_mode off / paused, a staff message within
//	  whatsapp.ai_staff_pause_minutes, AI opt-out, own number
//	identity (F4-02b): panel user / customer / visitor; menu or fixed reply
//	modules: whatsapp_gateway and ai_assistant of the center (and the
//	  user's organization), a configured provider, ai.use for panel users
//	location (TEC-397): a shared location → the nearest dealers, no AI
//	consent (K22): no AI answer before EVET; EVET records the consent and
//	  answers the question that waited for it
//	visitor_limit (TEC-397): turns per visitor number and the visitor
//	  token cap per day → fixed text with the dealer finder link
//	quota: panel user → own organization; customer / visitor → the
//	  center's system pool (QUESTIONS S1); spent → fixed text + handover
//	confirmation: EVET / HAYIR on an open card → Confirm / Cancel (F4-01e)
//	media: voice → fixed reply, documents / video → receipt, images →
//	  vision blocks
//	agent (F4-01f core via aiusecase.Chat.RunAgent): realm tool set, a
//	  write tool becomes a text card "… EVET / HAYIR"
//	guard: secret leak → not sent, internal ids masked, Markdown →
//	  WhatsApp, ≤ 4000 characters per message; failures → localized
//	  "could not process" text
package pipeline

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	aimodel "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/ai/model"
	aitools "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/ai/tools"
	aiusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/ai/usecase"
	authusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/auth/usecase"
	legal "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/legal/usecase"
	notifmodel "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/notifications/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/whatsapp/model"
	wausecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/whatsapp/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/brandctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/features"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/jwt"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/llm"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/whatsapp"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Limits of a run.
const (
	// MaxBatch is how many unanswered messages one run reads (newest).
	MaxBatch = 20
	// PendingWindow: older unanswered messages are not answered any more.
	PendingWindow = 24 * time.Hour
	// HistoryMessages is how many earlier messages the model sees.
	HistoryMessages = 30
	// lockTTL bounds a run (above the task timeout).
	lockTTL = 4 * time.Minute
	// busyRetry defers a task whose conversation is being processed.
	busyRetry = 3 * time.Second
	// consentSource marks a WhatsApp consent (consents.user_agent).
	consentSource   = "whatsapp"
	defaultTimezone = "Europe/Istanbul"
)

// Stage names of conversation_ai_runs.stages.
const (
	StageKeyword  = "keyword"
	StageGate     = "gate"
	StageIdentity = "identity"
	StageModules  = "modules"
	StageConsent  = "consent"
	StageQuota    = "quota"
	StageConfirm  = "confirm"
	StageMedia    = "media"
	StageAgent    = "agent"
	StageGuard    = "guard"
	StageDeliver  = "deliver"
)

// Skip reasons (stage detail "reason").
const (
	ReasonAIOff           = "ai_off"
	ReasonAIPaused        = "ai_paused"
	ReasonStaffActive     = "staff_active"
	ReasonOptedOut        = "ai_opt_out"
	ReasonOwnNumber       = "own_number"
	ReasonFeatureDisabled = "feature_disabled"
	ReasonUnavailable     = "provider_unavailable"
	ReasonNotAllowed      = "not_allowed"
)

// BusyError defers a task while another run holds the conversation; it
// implements queue.RetryAfterError (no retry is consumed).
type BusyError struct{}

func (BusyError) Error() string { return "whatsapp ai: conversation is being processed" }

// RetryAfter implements queue.RetryAfterError.
func (BusyError) RetryAfter() time.Duration { return busyRetry }

// Agent is the AI chat core (*aiusecase.Chat).
type Agent interface {
	RunAgent(ctx context.Context, in aiusecase.AgentInput) (aiusecase.AgentResult, error)
	QuotaExceeded(ctx context.Context, orgID int64, pool string) (bool, error)
	ModuleOn(ctx context.Context, orgID int64) (bool, error)
	ProviderEnabled() bool
}

// Actions decides confirmation cards (*aiusecase.Actions).
type Actions interface {
	ListPending(ctx context.Context, p aitools.Principal) ([]aiusecase.Card, error)
	Confirm(ctx context.Context, p aitools.Principal, id uuid.UUID, edits map[string]any) (aiusecase.Outcome, error)
	Cancel(ctx context.Context, p aitools.Principal, id uuid.UUID) (aiusecase.Outcome, error)
}

// IdentityResolver resolves the contact (*wausecase.IdentityResolver).
type IdentityResolver interface {
	Resolve(ctx context.Context, conv db.Conversation, text string) (wausecase.Identity, error)
}

// Sender queues outgoing messages (*wausecase.Messaging).
type Sender interface {
	Queue(ctx context.Context, in wausecase.OutgoingMessage) (db.Message, error)
}

// Consents is the AI guidelines gate (*legal.Service).
type Consents interface {
	AIConsentRequired(ctx context.Context, userID int64, locale i18n.Locale) (*legal.Text, error)
	Decide(ctx context.Context, userID int64, in legal.DecideInput) (legal.Consent, error)
}

// AccessResolver resolves stored permissions (*authusecase.AuthUseCase).
type AccessResolver interface {
	ResolveStoredAccess(ctx context.Context, userID int64, orgUUID *uuid.UUID) (authusecase.Access, error)
}

// FeatureChecker reports module states (features.Service).
type FeatureChecker interface {
	Enabled(ctx context.Context, organizationID int64, key string) (bool, error)
}

// Settings are the system settings of the pipeline (sysconfig.Service).
type Settings interface {
	WhatsAppAIStaffPauseMinutes(ctx context.Context) int
	WhatsAppAIGuidelinesURL(ctx context.Context) string
}

// Deps wires the pipeline. Features, Notifier, Media and Downloader may
// be nil.
type Deps struct {
	Queries    *db.Queries
	Agent      Agent
	Actions    Actions
	Identity   IdentityResolver
	Sender     Sender
	Consents   Consents
	Access     AccessResolver
	Features   FeatureChecker
	Settings   Settings
	Locker     Locker
	Notifier   wausecase.Notifier
	Media      llm.ObjectReader
	Downloader whatsapp.MediaDownloader
	// DefaultBrandSlug is the brand of contacts with no brand of their own.
	DefaultBrandSlug string
	Log              *slog.Logger
	// TEC-397 visitor flow (each may be nil / empty): the dealer directory
	// of shared locations, the visitor lead use case, the daily visitor
	// token cap and the public frontend origin of the dealer links.
	Dealers         aitools.DealerFinder
	Leads           VisitorLeads
	VisitorSettings VisitorSettings
	FrontendURL     string
}

// Pipeline is the WhatsApp AI reply use case.
type Pipeline struct {
	d   Deps
	log *slog.Logger
	now func() time.Time
}

// New builds the pipeline.
func New(d Deps) *Pipeline {
	log := d.Log
	if log == nil {
		log = slog.Default()
	}
	if d.Locker == nil {
		d.Locker = &MemoryLocker{}
	}
	return &Pipeline{d: d, log: log, now: time.Now}
}

// SetClock replaces the clock (tests).
func (p *Pipeline) SetClock(now func() time.Time) { p.now = now }

// --- run state ----------------------------------------------------------------

type stage struct {
	Stage  string         `json:"stage"`
	At     time.Time      `json:"at"`
	MS     int64          `json:"ms"`
	Detail map[string]any `json:"detail,omitempty"`
}

// run collects one conversation_ai_runs row.
type run struct {
	row       db.ConversationAiRun
	conv      db.Conversation
	locale    string
	last      time.Time
	stages    []stage
	toolCalls []aiusecase.AgentToolCall
	status    string
	model     string
	usage     llm.Usage
	errText   string
}

func (r *run) stage(now time.Time, name string, detail map[string]any) {
	r.stages = append(r.stages, stage{Stage: name, At: now.UTC(), MS: now.Sub(r.last).Milliseconds(), Detail: detail})
	r.last = now
}

func (r *run) skip(now time.Time, name, reason string) {
	r.stage(now, name, map[string]any{"reason": reason})
	r.status = model.RunSkipped
}

// --- process ------------------------------------------------------------------

// Process is the whatsapp:ai_reply task of one conversation.
func (p *Pipeline) Process(ctx context.Context, convUUID uuid.UUID) error {
	release, ok, err := p.d.Locker.Acquire(ctx, "conversation:"+convUUID.String(), lockTTL)
	if err != nil {
		return fmt.Errorf("whatsapp ai: lock: %w", err)
	}
	if !ok {
		return BusyError{}
	}
	defer release()

	q := p.d.Queries
	conv, err := q.GetConversationByUUID(ctx, convUUID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	after, err := q.MaxConversationAIRunTrigger(ctx, conv.ID)
	if err != nil {
		return err
	}
	batch, err := p.inbound(ctx, conv.ID, after)
	if err != nil || len(batch) == 0 {
		return err
	}
	trigger := batch[len(batch)-1]
	row, err := q.CreateConversationAIRun(ctx, db.CreateConversationAIRunParams{
		ConversationID: conv.ID, TriggerMessageID: trigger.ID,
		OrganizationID: conv.OrganizationID, BrandID: conv.BrandID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil // this message already has its run
	}
	if err != nil {
		return err
	}
	r := &run{row: row, conv: conv, last: p.now(), status: model.RunCompleted, locale: convLocale(conv)}
	herr := p.handle(ctx, r, batch)
	if herr != nil {
		p.log.ErrorContext(ctx, "whatsapp_ai_run_failed", "conversation", conv.Uuid, "run", row.Uuid, "error", herr)
		r.status, r.errText = model.RunFailed, herr.Error()
		_ = p.sendFixed(ctx, r, textFailed)
	}
	return p.finish(ctx, r)
}

// inbound returns the contact messages after afterID within the pending
// window, oldest first.
func (p *Pipeline) inbound(ctx context.Context, convID, afterID int64) ([]db.Message, error) {
	rows, err := p.d.Queries.ListInboundMessagesAfter(ctx, db.ListInboundMessagesAfterParams{
		ConversationID: convID, AfterID: afterID,
		Since: pgtype.Timestamptz{Time: p.now().Add(-PendingWindow), Valid: true}, LimitCount: MaxBatch,
	})
	if err != nil {
		return nil, err
	}
	slices.Reverse(rows)
	return rows, nil
}

func (p *Pipeline) finish(ctx context.Context, r *run) error {
	ctx = context.WithoutCancel(ctx)
	stages, _ := json.Marshal(r.stages)
	if r.stages == nil {
		stages = []byte("[]")
	}
	calls := []byte("[]")
	if len(r.toolCalls) > 0 {
		calls, _ = json.Marshal(r.toolCalls)
	}
	params := db.FinishConversationAIRunParams{
		ID: r.row.ID, Status: r.status, Stages: stages, ToolCalls: calls, Model: truncate(r.model, 128),
		InputTokens: r.usage.InputTokens, OutputTokens: r.usage.OutputTokens,
		CacheReadTokens: r.usage.CacheReadTokens, CacheWriteTokens: r.usage.CacheWriteTokens,
		FinishedAt: pgtype.Timestamptz{Time: p.now(), Valid: true},
	}
	if r.errText != "" {
		params.Error = pgtype.Text{String: truncate(r.errText, 5000), Valid: true}
	}
	_, err := p.d.Queries.FinishConversationAIRun(ctx, params)
	return err
}

// handle runs the stages; a returned error fails the run (the contact gets
// the localized "could not process" text).
func (p *Pipeline) handle(ctx context.Context, r *run, batch []db.Message) error {
	q := p.d.Queries
	conv := r.conv

	// Keywords first: they work whatever the AI state.
	if done, err := p.keywords(ctx, r, batch); done || err != nil {
		return err
	}

	// Gate.
	if st, err := q.GetWhatsAppSettings(ctx); err == nil && st.PhoneE164.Valid && st.PhoneE164.String == conv.ContactE164 {
		r.skip(p.now(), StageGate, ReasonOwnNumber)
		return nil
	}
	switch conv.AiMode {
	case model.AIModeOff:
		r.skip(p.now(), StageGate, ReasonAIOff)
		return nil
	case model.AIModePaused:
		if !conv.AiPausedUntil.Valid || conv.AiPausedUntil.Time.After(p.now()) {
			r.skip(p.now(), StageGate, ReasonAIPaused)
			return nil
		}
	}
	if mins := p.staffPause(ctx); mins > 0 {
		at, err := q.LastConversationMessageAtBySender(ctx, db.LastConversationMessageAtBySenderParams{
			ConversationID: conv.ID, SenderType: model.SenderStaff,
		})
		if err != nil {
			return err
		}
		if at.Valid && at.Time.After(p.now().Add(-time.Duration(mins)*time.Minute)) {
			r.skip(p.now(), StageGate, ReasonStaffActive)
			return nil
		}
	}
	if st, err := q.GetContactOptOutState(ctx, db.GetContactOptOutStateParams{
		ContactE164: conv.ContactE164, Scope: model.OptOutScopeAI,
	}); err == nil && st.OptedOut {
		r.skip(p.now(), StageGate, ReasonOptedOut)
		return nil
	} else if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	r.stage(p.now(), StageGate, nil)

	// Identity (F4-02b).
	last := batch[len(batch)-1]
	id, err := p.d.Identity.Resolve(ctx, conv, messageText(last))
	if err != nil {
		return err
	}
	r.conv, conv = id.Conversation, id.Conversation
	if id.Locale != "" {
		r.locale = id.Locale
	}
	r.stage(p.now(), StageIdentity, map[string]any{"kind": id.Kind})
	if id.Reply != "" {
		return p.sendSystem(ctx, r, id.Reply)
	}

	// Modules, provider and the principal of the realm.
	act, ok, err := p.actor(ctx, r, id)
	if err != nil || !ok {
		return err
	}

	if err := p.dealerReferral(ctx, r, act, batch); err != nil {
		return err
	}
	conv = r.conv

	// A shared location: the nearest dealers (TEC-397, no model call).
	if done, err := p.location(ctx, r, act, batch); done || err != nil {
		return err
	}

	// Consent (K22).
	turn, done, err := p.consent(ctx, r, act, batch)
	if done || err != nil {
		return err
	}

	// Visitor abuse limits (TEC-397).
	if done, err := p.visitorLimits(ctx, r, act); done || err != nil {
		return err
	}

	// Quota (S1: customer / visitor → the center's system pool).
	if spent, err := p.d.Agent.QuotaExceeded(ctx, act.quotaOrg, act.pool); err != nil {
		return err
	} else if spent {
		r.stage(p.now(), StageQuota, map[string]any{"pool": act.pool, "exceeded": true})
		return p.quotaHandover(ctx, r)
	}
	r.stage(p.now(), StageQuota, map[string]any{"pool": act.pool})

	// EVET / HAYIR on an open confirmation card.
	if done, err := p.decideCard(ctx, r, act, batch); done || err != nil {
		return err
	}

	// Media.
	content, ok, err := p.content(ctx, r, turn)
	if err != nil || !ok {
		return err
	}

	// Agent turn.
	history, err := p.history(ctx, conv, turn[0])
	if err != nil {
		return err
	}
	return p.answer(ctx, r, act, history, content)
}

// --- keywords -------------------------------------------------------------------

func (p *Pipeline) keywords(ctx context.Context, r *run, batch []db.Message) (bool, error) {
	var kw keyword
	for _, m := range batch {
		k := keywordOf(messageText(m))
		if k.kind == kwStop || (k.kind == kwHuman && kw.kind != kwStop) {
			kw = k
		}
	}
	if kw.kind == kwNone {
		return false, nil
	}
	if r.locale == "" {
		r.locale = kw.locale
	}
	q := p.d.Queries
	conv := r.conv
	switch kw.kind {
	case kwStop:
		if _, err := q.InsertContactOptOut(ctx, db.InsertContactOptOutParams{
			ContactE164: conv.ContactE164, Scope: model.OptOutScopeMarketing, Action: model.OptOutActionOut,
			Source: model.OptOutSourceWhatsApp, ConversationID: pgtype.Int8{Int64: conv.ID, Valid: true},
		}); err != nil {
			return true, err
		}
		r.stage(p.now(), StageKeyword, map[string]any{"keyword": "stop"})
		return true, p.sendFixed(ctx, r, textStopped)
	default:
		if err := p.handover(ctx, r, pgtype.Timestamptz{}, textNotifyHandover); err != nil {
			return true, err
		}
		r.stage(p.now(), StageKeyword, map[string]any{"keyword": "human"})
		return true, p.sendFixed(ctx, r, textHandover)
	}
}

// handover pauses the AI (until is empty: until staff resumes it), moves
// the conversation to pending and notifies the assignee, else the center's
// WhatsApp admins (F4 decision S2).
func (p *Pipeline) handover(ctx context.Context, r *run, until pgtype.Timestamptz, notify int) error {
	q := p.d.Queries
	conv, err := q.SetConversationAIMode(ctx, db.SetConversationAIModeParams{
		ID: r.conv.ID, AiMode: model.AIModePaused, AiPausedUntil: until,
	})
	if err != nil {
		return err
	}
	if conv, err = q.SetConversationStatus(ctx, db.SetConversationStatusParams{ID: conv.ID, Status: model.StatusPending}); err != nil {
		return err
	}
	r.conv = conv
	p.notifyStaff(ctx, conv, notify)
	return nil
}

func (p *Pipeline) notifyStaff(ctx context.Context, conv db.Conversation, key int) {
	if p.d.Notifier == nil {
		return
	}
	type recipient struct {
		id     int64
		locale string
	}
	var to []recipient
	if conv.AssignedUserID.Valid {
		if u, err := p.d.Queries.GetUserByID(ctx, conv.AssignedUserID.Int64); err == nil {
			to = append(to, recipient{u.ID, u.Locale.String})
		}
	}
	if len(to) == 0 {
		rows, err := p.d.Queries.ListWhatsAppAlarmRecipients(ctx)
		if err != nil {
			p.log.WarnContext(ctx, "whatsapp_ai_handover_recipients_failed", "error", err)
			return
		}
		for _, u := range rows {
			to = append(to, recipient{u.ID, u.Locale.String})
		}
	}
	contact := conv.ContactE164
	if n := strings.TrimSpace(conv.ContactName.String); n != "" {
		contact = n + " (" + conv.ContactE164 + ")"
	}
	for _, rc := range to {
		uid := rc.id
		locale := wausecaseLocale(rc.locale)
		if _, err := p.d.Notifier.Enqueue(ctx, notifmodel.EnqueueInput{
			UserID: &uid, Channels: []string{notifmodel.ChannelInapp}, Priority: notifmodel.PriorityHigh,
			Title: text(locale, textNotifyTitle), Body: textf(locale, key, contact),
			SourceEvent: "conversation.handover", Language: strings.ReplaceAll(locale, "_", "-"),
			Payload: map[string]any{"conversation_uuid": conv.Uuid.String()},
		}); err != nil {
			p.log.WarnContext(ctx, "whatsapp_ai_handover_notify_failed", "user", uid, "error", err)
		}
	}
}

func (p *Pipeline) quotaHandover(ctx context.Context, r *run) error {
	now := p.now().UTC()
	next := time.Date(now.Year(), now.Month()+1, 1, 0, 0, 0, 0, time.UTC)
	if err := p.handover(ctx, r, pgtype.Timestamptz{Time: next, Valid: true}, textNotifyQuota); err != nil {
		return err
	}
	return p.sendFixed(ctx, r, textQuotaHandover)
}

func (p *Pipeline) staffPause(ctx context.Context) int {
	if p.d.Settings == nil {
		return 30
	}
	return p.d.Settings.WhatsAppAIStaffPauseMinutes(ctx)
}

// --- actor --------------------------------------------------------------------

// actor is who the AI acts for in this run.
type actor struct {
	id        wausecase.Identity
	principal aitools.Principal
	// quotaOrg / pool own the tokens; brandID / center the brand.
	quotaOrg int64
	pool     string
	brand    db.Brand
	center   db.Organization
	facts    aiusecase.AgentFacts
	readOnly bool
}

// actor resolves the brand, checks the modules and builds the tool
// principal; ok is false when the run was skipped.
func (p *Pipeline) actor(ctx context.Context, r *run, id wausecase.Identity) (actor, bool, error) {
	q := p.d.Queries
	a := actor{id: id, readOnly: id.WritesDisabled}
	brandID := id.BrandID
	if brandID == 0 && r.conv.BrandID.Valid {
		brandID = r.conv.BrandID.Int64
	}
	var err error
	if brandID != 0 {
		a.brand, err = q.GetBrandByID(ctx, brandID)
	} else {
		a.brand, err = q.GetBrandBySlug(ctx, p.d.DefaultBrandSlug)
	}
	if err != nil {
		return a, false, fmt.Errorf("whatsapp ai: brand: %w", err)
	}
	if a.center, err = q.GetBrandCenter(ctx, a.brand.ID); err != nil {
		return a, false, fmt.Errorf("whatsapp ai: brand center: %w", err)
	}
	orgs := []int64{a.center.ID}
	if id.Kind == model.IdentityPanelUser && id.OrgID != a.center.ID {
		orgs = append(orgs, id.OrgID)
	}
	for _, org := range orgs {
		on, err := p.modulesOn(ctx, org)
		if err != nil {
			return a, false, err
		}
		if !on {
			r.skip(p.now(), StageModules, ReasonFeatureDisabled)
			return a, false, nil
		}
	}
	if !p.d.Agent.ProviderEnabled() {
		r.skip(p.now(), StageModules, ReasonUnavailable)
		return a, false, nil
	}
	brandRef := &brandctx.Brand{ID: a.brand.ID, Slug: a.brand.Slug, Name: a.brand.Name, Status: a.brand.Status}
	a.facts = aiusecase.AgentFacts{Brand: a.brand.Name, Locale: r.locale, Timezone: firstNonEmpty(a.center.Timezone, defaultTimezone)}
	switch id.Kind {
	case model.IdentityPanelUser:
		ok, err := p.panelActor(ctx, &a, id)
		if err != nil {
			return a, false, err
		}
		if !ok {
			r.skip(p.now(), StageModules, ReasonNotAllowed)
			return a, false, nil
		}
	case model.IdentityCustomer:
		user, err := q.GetUserByID(ctx, id.UserID)
		if err != nil {
			return a, false, fmt.Errorf("whatsapp ai: user: %w", err)
		}
		acc, err := p.d.Access.ResolveStoredAccess(ctx, user.ID, nil)
		if err != nil {
			return a, false, fmt.Errorf("whatsapp ai: access: %w", err)
		}
		auth := authctx.Principal{UserID: user.Uuid, UserInternal: user.ID, Email: user.Email.String}
		fill(&auth, acc, jwt.AudiencePortal)
		a.principal = aitools.Principal{Auth: auth, Realm: aitools.RealmCustomer, Brand: brandRef}
		a.quotaOrg, a.pool = a.center.ID, aimodel.PoolSystem
		a.facts.Customer = true
		a.facts.User = strings.TrimSpace(user.Name + " " + user.Surname)
	default:
		a.principal = aitools.Principal{Realm: aitools.RealmVisitor, Brand: brandRef}
		a.quotaOrg, a.pool = a.center.ID, aimodel.PoolSystem
		a.facts.Visitor = true
	}
	r.stage(p.now(), StageModules, map[string]any{"realm": string(a.principal.Realm), "pool": a.pool})
	return a, true, nil
}

func (p *Pipeline) panelActor(ctx context.Context, a *actor, id wausecase.Identity) (bool, error) {
	q := p.d.Queries
	user, err := q.GetUserByID(ctx, id.UserID)
	if err != nil {
		return false, fmt.Errorf("whatsapp ai: user: %w", err)
	}
	org, err := q.GetOrganizationByID(ctx, id.OrgID)
	if err != nil {
		return false, fmt.Errorf("whatsapp ai: organization: %w", err)
	}
	acc, err := p.d.Access.ResolveStoredAccess(ctx, user.ID, &org.Uuid)
	if err != nil {
		return false, fmt.Errorf("whatsapp ai: access: %w", err)
	}
	role := ""
	if m, err := q.GetOrganizationMemberByUserAndOrgUUID(ctx, db.GetOrganizationMemberByUserAndOrgUUIDParams{UserID: user.ID, Uuid: org.Uuid}); err == nil {
		role = m.Role
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return false, fmt.Errorf("whatsapp ai: membership: %w", err)
	}
	auth := authctx.Principal{UserID: user.Uuid, UserInternal: user.ID, Email: user.Email.String}
	fill(&auth, acc, jwt.AudiencePanel)
	orgUUID := org.Uuid
	auth.OrganizationUUID = &orgUUID
	if !auth.HasPermission(rbac.PermAIUse) {
		return false, nil
	}
	a.principal = aitools.Principal{
		Auth: auth, Realm: aitools.RealmPanel,
		Org: &orgctx.Scope{
			InternalID: org.ID, UUID: org.Uuid, Slug: org.Slug, Name: org.Name, MemberRole: role,
			Status: org.Status, OrgType: org.Type, BrandID: org.BrandID, BrandSlug: a.brand.Slug,
		},
	}
	a.quotaOrg, a.pool = org.ID, aimodel.PoolOrg
	a.facts.Org, a.facts.OrgType, a.facts.Role = org.Name, org.Type, role
	a.facts.User = strings.TrimSpace(user.Name + " " + user.Surname)
	a.facts.Timezone = firstNonEmpty(org.Timezone, a.facts.Timezone)
	return true, nil
}

func fill(p *authctx.Principal, acc authusecase.Access, realm string) {
	p.Roles, p.Permissions, p.PermissionScopes, p.IsSuperAdmin = acc.Roles, acc.Permissions, acc.Grants, acc.IsSuperAdmin
	p.Realm = realm
}

// modulesOn: whatsapp_gateway and ai_assistant (with ai_org_settings) are
// on for the organization.
func (p *Pipeline) modulesOn(ctx context.Context, orgID int64) (bool, error) {
	if p.d.Features != nil {
		on, err := p.d.Features.Enabled(ctx, orgID, features.ModuleWhatsApp)
		if err != nil || !on {
			return false, err
		}
	}
	return p.d.Agent.ModuleOn(ctx, orgID)
}

// --- consent ------------------------------------------------------------------

// consent is the K22 gate. It returns the messages of the AI turn: the
// batch, or after an EVET the question that waited for the consent. done
// means the run ends here (prompt or thanks sent).
func (p *Pipeline) consent(ctx context.Context, r *run, a actor, batch []db.Message) ([]db.Message, bool, error) {
	required, err := p.consentText(ctx, r, a)
	if err != nil {
		return nil, true, err
	}
	if required == nil {
		return batch, false, nil
	}
	accepted := false
	for _, m := range batch {
		if keywordOf(messageText(m)).kind == kwYes {
			accepted = true
		}
	}
	if !accepted {
		r.stage(p.now(), StageConsent, map[string]any{"required": true, "version": required.Version})
		return nil, true, p.sendConsentPrompt(ctx, r, *required)
	}
	q := p.d.Queries
	if a.id.UserID > 0 && a.id.Kind != model.IdentityVisitor {
		if _, err := p.d.Consents.Decide(ctx, a.id.UserID, legal.DecideInput{
			Kind: legal.KindAIGuidelines, Locale: required.Locale, Version: required.Version,
			Accepted: true, UserAgent: consentSource,
		}); err != nil {
			return nil, true, fmt.Errorf("whatsapp ai: consent: %w", err)
		}
	}
	conv, err := q.SetConversationAIConsent(ctx, db.SetConversationAIConsentParams{
		ID: r.conv.ID, AiConsentAt: pgtype.Timestamptz{Time: p.now(), Valid: true},
	})
	if err != nil {
		return nil, true, err
	}
	r.conv = conv
	r.stage(p.now(), StageConsent, map[string]any{"accepted": true, "version": required.Version, "locale": required.Locale})

	// The question that waited: unanswered contact messages since the last
	// AI answer, without the consent answers themselves.
	lastAI, err := q.MaxConversationMessageIDBySender(ctx, db.MaxConversationMessageIDBySenderParams{
		ConversationID: conv.ID, SenderType: model.SenderAI,
	})
	if err != nil {
		return nil, true, err
	}
	waiting, err := p.inbound(ctx, conv.ID, lastAI)
	if err != nil {
		return nil, true, err
	}
	var question []db.Message
	for _, m := range waiting {
		if k := keywordOf(messageText(m)).kind; k == kwYes || k == kwNo {
			continue
		}
		question = append(question, m)
	}
	if len(question) == 0 {
		return nil, true, p.sendFixed(ctx, r, textConsentThanks)
	}
	return question, false, nil
}

// consentText returns the guidelines text still to accept, or nil. Users
// answer per account (consents, like the portal); visitors per
// conversation (ai_consent_at, no user to record it on).
func (p *Pipeline) consentText(ctx context.Context, r *run, a actor) (*legal.Text, error) {
	if p.d.Consents == nil {
		return nil, nil
	}
	locale := i18nLocale(r.locale)
	if a.id.Kind == model.IdentityVisitor || a.id.UserID == 0 {
		if r.conv.AiConsentAt.Valid {
			return nil, nil
		}
		// No user: the current text (nil when none is published).
		return p.d.Consents.AIConsentRequired(ctx, 0, locale)
	}
	return p.d.Consents.AIConsentRequired(ctx, a.id.UserID, locale)
}

func (p *Pipeline) sendConsentPrompt(ctx context.Context, r *run, t legal.Text) error {
	guide := ""
	if p.d.Settings != nil {
		guide = strings.TrimSpace(p.d.Settings.WhatsAppAIGuidelinesURL(ctx))
	}
	if guide == "" {
		guide = toWhatsApp(strings.TrimSpace(t.Body))
	}
	return p.sendSystem(ctx, r, textf(r.locale, textConsent, guide))
}

// --- confirmation cards -------------------------------------------------------

// decideCard confirms or cancels the open card of this conversation when
// the newest message is EVET / HAYIR (panel users; write tools are panel
// tools).
func (p *Pipeline) decideCard(ctx context.Context, r *run, a actor, batch []db.Message) (bool, error) {
	if a.principal.Org == nil || p.d.Actions == nil {
		return false, nil
	}
	k := keywordOf(messageText(batch[len(batch)-1])).kind
	if k != kwYes && k != kwNo {
		return false, nil
	}
	cards, err := p.d.Actions.ListPending(ctx, a.principal)
	if err != nil {
		return true, err
	}
	var card *aiusecase.Card
	for i := range cards {
		c := cards[i]
		if c.Source == aimodel.SourceWhatsApp && c.SourceRef == r.conv.Uuid.String() && c.Status == aimodel.ActionPending &&
			(card == nil || c.CreatedAt.After(card.CreatedAt)) {
			card = &c
		}
	}
	if card == nil {
		return false, nil
	}
	var out aiusecase.Outcome
	if k == kwYes {
		out, err = p.d.Actions.Confirm(ctx, a.principal, card.ActionUUID, nil)
	} else {
		out, err = p.d.Actions.Cancel(ctx, a.principal, card.ActionUUID)
	}
	detail := map[string]any{"action": card.ActionUUID.String(), "tool": card.ToolName, "confirm": k == kwYes}
	switch {
	case errors.Is(err, aiusecase.ErrActionExpired):
		detail["result"] = "expired"
		r.stage(p.now(), StageConfirm, detail)
		return true, p.sendFixed(ctx, r, textActionExpired)
	case errors.Is(err, aiusecase.ErrActionResolved), errors.Is(err, aiusecase.ErrActionNotFound):
		detail["result"] = "resolved"
		r.stage(p.now(), StageConfirm, detail)
		return true, nil
	case errors.Is(err, aiusecase.ErrActionForbidden):
		detail["result"] = aimodel.ActionFailed
		r.stage(p.now(), StageConfirm, detail)
		return true, p.sendFixed(ctx, r, textActionFailed)
	case err != nil:
		return true, err
	}
	detail["result"] = out.Status
	r.stage(p.now(), StageConfirm, detail)
	r.toolCalls = append(r.toolCalls, aiusecase.AgentToolCall{Name: card.ToolName, Status: out.Status})
	switch out.Status {
	case aimodel.ActionConfirmed:
		return true, p.sendFixed(ctx, r, textActionDone)
	case aimodel.ActionCancelled:
		return true, p.sendFixed(ctx, r, textActionCancelled)
	default:
		return true, p.sendFixed(ctx, r, textActionFailed)
	}
}

// cardText renders a confirmation card as a WhatsApp question in the
// conversation language (TEC-461): the summary and the field labels come
// from the backend catalog, a language outside it gets en.
func cardText(locale string, c aiusecase.Card) string {
	loc := cardLocale(locale)
	var sb strings.Builder
	sb.WriteString(text(locale, textConfirmHeader))
	if s := strings.TrimSpace(c.Preview.LocalizedSummary(loc)); s != "" {
		sb.WriteString("\n*")
		sb.WriteString(s)
		sb.WriteString("*")
	}
	for _, f := range c.Preview.Fields {
		if strings.TrimSpace(f.Value) == "" {
			continue
		}
		fmt.Fprintf(&sb, "\n• %s: %s", aitools.FieldLabel(loc, f.Key), f.Value)
	}
	sb.WriteString("\n\n")
	sb.WriteString(text(locale, textConfirmQuestion))
	return sb.String()
}

// --- media and history ----------------------------------------------------------

type mediaMeta struct {
	Type     string `json:"type"`
	FileName string `json:"file_name"`
	Caption  string `json:"caption"`
}

func mediaOf(m db.Message) (mediaMeta, bool) {
	if len(m.Media) == 0 {
		return mediaMeta{}, false
	}
	var mm mediaMeta
	if err := json.Unmarshal(m.Media, &mm); err != nil || mm.Type == "" {
		return mediaMeta{}, false
	}
	return mm, true
}

// messageText is the text of a message, or the caption of its media.
func messageText(m db.Message) string {
	if t := strings.TrimSpace(m.Body.String); t != "" {
		return t
	}
	if mm, ok := mediaOf(m); ok {
		return strings.TrimSpace(mm.Caption)
	}
	return ""
}

// content builds the user content of the turn. Voice messages get the
// fixed "not supported" reply, documents and videos a receipt; ok is false
// when nothing is left for the model.
func (p *Pipeline) content(ctx context.Context, r *run, msgs []db.Message) ([]llm.Block, bool, error) {
	var (
		blocks         []llm.Block
		voice, docs    int
		images, failed int
	)
	for _, m := range msgs {
		mm, hasMedia := mediaOf(m)
		txt := messageText(m)
		if loc, ok := locationOf(m); ok {
			// TEC-397: a location followed by more messages goes to the
			// model (find_nearest_dealers takes the coordinates).
			blocks = append(blocks, llm.TextBlock(fmt.Sprintf("[The user shared a location: latitude %.6f, longitude %.6f %s]",
				*loc.Latitude, *loc.Longitude, strings.TrimSpace(loc.Caption))))
			continue
		}
		switch {
		case hasMedia && mm.Type == "audio":
			voice++
			continue
		case hasMedia && (mm.Type == "document" || mm.Type == "video"):
			docs++
			if txt != "" {
				blocks = append(blocks, llm.TextBlock(txt+"\n[The user attached a "+mm.Type+"; its content cannot be read.]"))
			}
			continue
		}
		if txt != "" {
			blocks = append(blocks, llm.TextBlock(txt))
		}
		if hasMedia && (mm.Type == "image" || mm.Type == "sticker") {
			b, err := p.image(ctx, m)
			if err != nil {
				failed++
				p.log.WarnContext(ctx, "whatsapp_ai_image_failed", "message", m.Uuid, "error", err)
				blocks = append(blocks, llm.TextBlock("[The user sent an image that could not be read.]"))
				continue
			}
			images++
			blocks = append(blocks, b)
		}
	}
	r.stage(p.now(), StageMedia, map[string]any{"voice": voice, "documents": docs, "images": images, "image_errors": failed})
	if voice > 0 {
		if err := p.sendFixed(ctx, r, textVoiceUnsupported); err != nil {
			return nil, false, err
		}
	}
	if docs > 0 {
		if err := p.sendFixed(ctx, r, textDocumentReceived); err != nil {
			return nil, false, err
		}
	}
	return blocks, len(blocks) > 0, nil
}

// image loads an inbound image: the stored copy, else from the gateway.
func (p *Pipeline) image(ctx context.Context, m db.Message) (llm.Block, error) {
	if m.MediaStorageKey.Valid && p.d.Media != nil {
		return llm.ImageFromStorage(ctx, p.d.Media, m.MediaStorageKey.String)
	}
	if p.d.Downloader == nil {
		return llm.Block{}, errors.New("no media downloader")
	}
	var meta whatsapp.InboundMedia
	if err := json.Unmarshal(m.Media, &meta); err != nil {
		return llm.Block{}, err
	}
	data, err := p.d.Downloader.DownloadMedia(ctx, meta, llm.MaxImageSourceBytes)
	if err != nil {
		return llm.Block{}, err
	}
	return llm.ImageBlock(data)
}

// history maps the messages before the turn to model messages: contact →
// user, AI and staff → assistant (fixed system texts are left out).
func (p *Pipeline) history(ctx context.Context, conv db.Conversation, first db.Message) ([]llm.Message, error) {
	rows, err := p.d.Queries.ListConversationMessagesBefore(ctx, db.ListConversationMessagesBeforeParams{
		ConversationID: conv.ID, CursorAt: first.CreatedAt,
		CursorID: pgtype.Int8{Int64: first.ID, Valid: true}, LimitCount: HistoryMessages,
	})
	if err != nil {
		return nil, err
	}
	slices.Reverse(rows)
	var out []llm.Message
	for _, m := range rows {
		txt := messageText(m)
		if mm, ok := mediaOf(m); ok && txt == "" {
			txt = "[" + mm.Type + "]"
		}
		if txt == "" {
			continue
		}
		switch {
		case m.Direction == "in":
			out = append(out, llm.Message{Role: llm.RoleUser, Content: []llm.Block{llm.TextBlock(txt)}})
		case m.SenderType == model.SenderAI:
			out = append(out, llm.Message{Role: llm.RoleAssistant, Content: []llm.Block{llm.TextBlock(txt)}})
		case m.SenderType == model.SenderStaff:
			out = append(out, llm.Message{Role: llm.RoleAssistant, Content: []llm.Block{llm.TextBlock("[staff] " + txt)}})
		}
	}
	return out, nil
}

// --- agent --------------------------------------------------------------------

func (p *Pipeline) answer(ctx context.Context, r *run, a actor, history []llm.Message, content []llm.Block) error {
	a.facts.Locale = r.locale
	// TEC-461: the confirmation card summary is written in this language.
	a.principal.Locale = cardLocale(r.locale)
	if a.facts.Visitor {
		// The visitor tools of this package read the conversation.
		ctx = withVisitorTurn(ctx, &visitorTurn{p: p, r: r, a: a})
	}
	res, err := p.d.Agent.RunAgent(ctx, aiusecase.AgentInput{
		Principal: a.principal, OrgID: a.quotaOrg, BrandID: a.brand.ID, Pool: a.pool,
		Source: aimodel.SourceWhatsApp, SourceRef: r.conv.Uuid.String(),
		Facts: a.facts, ReadOnly: a.readOnly, History: history, Content: content,
	})
	if err != nil {
		return err
	}
	r.model, r.usage, r.toolCalls = res.Model, res.Usage, append(r.toolCalls, res.ToolCalls...)
	detail := map[string]any{"status": res.Status, "stop_reason": res.StopReason}
	if res.ErrorCode != "" {
		detail["error_code"] = res.ErrorCode
	}
	if res.Card != nil {
		detail["action"] = res.Card.ActionUUID.String()
	}
	r.stage(p.now(), StageAgent, detail)
	if res.QuotaExceeded && res.Text == "" {
		return p.quotaHandover(ctx, r)
	}

	answer := res.Text
	if res.Card != nil {
		answer = strings.TrimSpace(answer + "\n\n" + cardText(r.locale, *res.Card))
	}
	if strings.TrimSpace(answer) == "" {
		r.status, r.errText = model.RunFailed, firstNonEmpty(res.Error, res.ErrorCode, "empty answer")
		return p.sendFixed(ctx, r, textFailed)
	}
	g := guard(answer)
	r.stage(p.now(), StageGuard, map[string]any{"blocked": g.Blocked, "redacted": g.Redacted})
	if g.Blocked {
		r.status, r.errText = model.RunFailed, "guard: secret leak blocked"
		return p.sendFixed(ctx, r, textFailed)
	}
	if res.Status == aimodel.MessageError {
		// A partial answer before a provider error: send it, record the
		// failure.
		r.status, r.errText = model.RunFailed, firstNonEmpty(res.Error, res.ErrorCode)
	}
	return p.send(ctx, r, model.SenderAI, g.Text)
}

// --- delivery -------------------------------------------------------------------

func (p *Pipeline) sendFixed(ctx context.Context, r *run, key int) error {
	return p.sendSystem(ctx, r, text(r.locale, key))
}

func (p *Pipeline) sendSystem(ctx context.Context, r *run, body string) error {
	return p.send(ctx, r, model.SenderSystem, body)
}

// send queues body (split at MaxPartRunes) on the outgoing queue (F4-02d).
func (p *Pipeline) send(ctx context.Context, r *run, sender, body string) error {
	ctx = context.WithoutCancel(ctx)
	runID := r.row.ID
	parts := splitMessage(body, MaxPartRunes)
	for _, part := range parts {
		if _, err := p.d.Sender.Queue(ctx, wausecase.OutgoingMessage{
			ConversationID: r.conv.ID, SenderType: sender, AIRunID: &runID, Body: part,
		}); err != nil {
			return fmt.Errorf("whatsapp ai: deliver: %w", err)
		}
	}
	r.stage(p.now(), StageDeliver, map[string]any{"sender": sender, "parts": len(parts)})
	return nil
}

// --- helpers --------------------------------------------------------------------

func convLocale(c db.Conversation) string {
	if !c.Locale.Valid {
		return ""
	}
	l, _ := wausecase.ConversationLocale(c.Locale.String)
	return l
}

// wausecaseLocale maps a user locale to the conversation locale form.
func wausecaseLocale(raw string) string {
	if l, ok := wausecase.ConversationLocale(raw); ok {
		return l
	}
	return fallbackLocale
}

func i18nLocale(conversationLocale string) i18n.Locale {
	l, ok := i18n.Parse(strings.ReplaceAll(conversationLocale, "_", "-"))
	if !ok {
		return i18n.DefaultLocale
	}
	return l
}

// cardLocale maps a conversation locale to the backend catalog locale;
// unlike i18nLocale an unknown language is en (fallbackLocale), not tr.
func cardLocale(conversationLocale string) i18n.Locale {
	if l, ok := i18n.Parse(strings.ReplaceAll(conversationLocale, "_", "-")); ok {
		return l
	}
	return i18n.Locale(fallbackLocale)
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func truncate(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}
