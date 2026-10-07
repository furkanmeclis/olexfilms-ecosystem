package usecase

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/whatsapp/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/llm"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Identity resolution of a WhatsApp contact (TEC-394, F4-02b). The E.164
// number resolves to (1) a panel user through its usable memberships, (2) a
// customer user, or (3) a visitor. An inbound WhatsApp message proves
// ownership of the number, except for K26 "unverified" migrated records,
// which are not claimed over WhatsApp: the contact is treated as a visitor
// and offered the portal OTP login (QUESTIONS #12).

// IdentityStore is the sqlc surface of the resolver.
type IdentityStore interface {
	GetUserByPhone(ctx context.Context, phoneE164 pgtype.Text) (db.User, error)
	GetUserByID(ctx context.Context, id int64) (db.User, error)
	ListWhatsAppIdentityMemberships(ctx context.Context, userID int64) ([]db.ListWhatsAppIdentityMembershipsRow, error)
	IsCustomerUser(ctx context.Context, userID int64) (bool, error)
	SetConversationIdentity(ctx context.Context, arg db.SetConversationIdentityParams) (db.Conversation, error)
	SetConversationLocale(ctx context.Context, arg db.SetConversationLocaleParams) (db.Conversation, error)
	GetAISettings(ctx context.Context) (db.AiSetting, error)
}

// LocaleUsageRecorder books the tokens of a language detection call (system
// pool, purpose locale). The AI pipeline wires it; nil skips booking.
type LocaleUsageRecorder interface {
	RecordLocaleUsage(ctx context.Context, conv db.Conversation, model string, usage llm.Usage) error
}

// IdentitySnapshot is what a number resolves to, cached per E.164. Access
// windows are kept raw and checked at resolve time.
type IdentitySnapshot struct {
	UserID int64  `json:"user_id,omitempty"`
	Status string `json:"status,omitempty"`
	Locale string `json:"locale,omitempty"`
	// Unverified is a K26 migrated record: never claimed over WhatsApp.
	Unverified bool             `json:"unverified,omitempty"`
	Customer   bool             `json:"customer,omitempty"`
	Members    []IdentityMember `json:"members,omitempty"`
}

// IdentityMember is one panel membership of the snapshot user.
type IdentityMember struct {
	OrgID        int64      `json:"org_id"`
	BrandID      int64      `json:"brand_id"`
	OrgName      string     `json:"org_name"`
	OrgType      string     `json:"org_type"`
	OrgStatus    string     `json:"org_status"`
	OrgLocale    string     `json:"org_locale,omitempty"`
	AccessStarts *time.Time `json:"access_starts,omitempty"`
	AccessEnds   *time.Time `json:"access_ends,omitempty"`
}

// usable mirrors the organization middleware (orgAccessAllowed): suspended
// and expired organizations and those outside their access window are not
// usable at all; read_only ones are (writes off).
func (m IdentityMember) usable(now time.Time) bool {
	if m.OrgStatus == "suspended" || m.OrgStatus == "expired" {
		return false
	}
	if m.AccessStarts != nil && m.AccessStarts.After(now) {
		return false
	}
	if m.AccessEnds != nil && !m.AccessEnds.After(now) {
		return false
	}
	return true
}

// IdentityOption is one entry of the identity choice menu.
type IdentityOption struct {
	Kind     string
	OrgID    int64
	BrandID  int64
	OrgName  string
	ReadOnly bool
	locale   string
}

// Identity is the outcome of Resolve for one inbound message.
type Identity struct {
	// Kind is panel_user, customer or visitor; unknown while the contact
	// still has to pick from the menu.
	Kind    string
	UserID  int64
	OrgID   int64
	BrandID int64
	OrgName string
	// Locale is the conversation locale (conversations.locale form, zh_CN).
	Locale string
	// WritesDisabled turns the write tools off: read_only organization
	// (contract ended, K23) or a user that is not active.
	WritesDisabled bool
	// Suspended is a disabled user: Reply carries the fixed short answer and
	// the AI does not run.
	Suspended bool
	// OfferPortalOTP marks a K26 unverified record answered as a visitor.
	OfferPortalOTP bool
	// AwaitingChoice: Reply carries the numbered menu.
	AwaitingChoice bool
	Options        []IdentityOption
	// Reset reports a "change identity" command.
	Reset bool
	// Reply, when set, is sent instead of an AI answer.
	Reply        string
	Conversation db.Conversation
}

// IdentityResolver resolves WhatsApp contacts.
type IdentityResolver struct {
	store  IdentityStore
	cache  IdentityCache
	llm    llm.Provider
	models llm.Models
	usage  LocaleUsageRecorder
	log    *slog.Logger
	now    func() time.Time
}

// NewIdentityResolver builds the resolver. cache nil = no cache, provider
// nil = no language detection (en).
func NewIdentityResolver(store IdentityStore, cache IdentityCache, provider llm.Provider, models llm.Models, log *slog.Logger) *IdentityResolver {
	if cache == nil {
		cache = NoIdentityCache{}
	}
	if provider == nil {
		provider = llm.Disabled{}
	}
	if log == nil {
		log = slog.Default()
	}
	return &IdentityResolver{store: store, cache: cache, llm: provider, models: models, log: log, now: time.Now}
}

// WithUsageRecorder sets the language detection usage booking.
func (r *IdentityResolver) WithUsageRecorder(u LocaleUsageRecorder) *IdentityResolver {
	r.usage = u
	return r
}

// Cache returns the snapshot cache (invalidation wiring).
func (r *IdentityResolver) Cache() IdentityCache { return r.cache }

// Resolve resolves the contact of conv for an inbound message text and
// stores the outcome on the conversation (identity_* and locale).
func (r *IdentityResolver) Resolve(ctx context.Context, conv db.Conversation, text string) (Identity, error) {
	snap, err := r.Snapshot(ctx, conv.ContactE164)
	if err != nil {
		return Identity{}, err
	}
	out := Identity{Conversation: conv}
	if IsIdentityResetCommand(text) {
		out.Reset = true
		if conv.IdentityKind != model.IdentityUnknown {
			if out.Conversation, err = r.setIdentity(ctx, conv.ID, model.IdentityUnknown, 0, 0); err != nil {
				return Identity{}, err
			}
		}
		text = ""
	}

	options := snap.options(r.now())
	if len(options) == 0 {
		return r.visitor(ctx, out, snap, text)
	}
	out.UserID = snap.UserID
	if snap.Status == "disabled" {
		out.Kind = out.Conversation.IdentityKind
		out.Suspended = true
		out.WritesDisabled = true
		out.Locale = r.userLocale(snap, IdentityOption{}, out.Conversation)
		out.Reply = identityText(out.Locale, textSuspended)
		return out, nil
	}

	choice, ok := currentChoice(out.Conversation, snap.UserID, options)
	switch {
	case ok:
	case len(options) == 1:
		choice, ok = options[0], true
	default:
		if n, isNum := menuNumber(text); isNum && n >= 1 && n <= len(options) {
			choice, ok = options[n-1], true
		}
	}
	if !ok {
		// Several identities and no valid pick yet: the numbered menu.
		if out.Conversation.IdentityKind != model.IdentityUnknown {
			if out.Conversation, err = r.setIdentity(ctx, conv.ID, model.IdentityUnknown, 0, 0); err != nil {
				return Identity{}, err
			}
		}
		out.Kind = model.IdentityUnknown
		out.AwaitingChoice = true
		out.Options = options
		out.Locale = r.userLocale(snap, IdentityOption{}, out.Conversation)
		if out.Locale == "" {
			if out.Locale, out.Conversation, err = r.detectAndStore(ctx, out.Conversation, text); err != nil {
				return Identity{}, err
			}
		}
		out.Reply = MenuText(out.Locale, options)
		return out, nil
	}

	orgID := choice.OrgID
	if choice.Kind != model.IdentityPanelUser {
		orgID = 0
	}
	if !sameIdentity(out.Conversation, choice.Kind, snap.UserID, orgID) {
		if out.Conversation, err = r.setIdentity(ctx, conv.ID, choice.Kind, snap.UserID, orgID); err != nil {
			return Identity{}, err
		}
	}
	out.Kind = choice.Kind
	out.OrgID, out.BrandID, out.OrgName = orgID, choice.BrandID, choice.OrgName
	out.WritesDisabled = choice.ReadOnly || snap.Status != "active"
	locale := r.userLocale(snap, choice, out.Conversation)
	if locale == "" {
		if locale, out.Conversation, err = r.detectAndStore(ctx, out.Conversation, text); err != nil {
			return Identity{}, err
		}
	} else if out.Conversation, err = r.storeLocale(ctx, out.Conversation, locale); err != nil {
		return Identity{}, err
	}
	out.Locale = locale
	return out, nil
}

// visitor stores the visitor identity and the locale (detected from the
// first message when the conversation has none).
func (r *IdentityResolver) visitor(ctx context.Context, out Identity, snap IdentitySnapshot, text string) (Identity, error) {
	var err error
	if out.Conversation.IdentityKind != model.IdentityVisitor {
		if out.Conversation, err = r.setIdentity(ctx, out.Conversation.ID, model.IdentityVisitor, 0, 0); err != nil {
			return Identity{}, err
		}
	}
	out.Kind = model.IdentityVisitor
	out.OfferPortalOTP = snap.Unverified
	out.Locale = convLocale(out.Conversation)
	if out.Locale == "" {
		if out.Locale, out.Conversation, err = r.detectAndStore(ctx, out.Conversation, text); err != nil {
			return Identity{}, err
		}
	}
	return out, nil
}

// options lists the identities a snapshot offers: usable panel memberships
// (by organization name) then the customer identity. Anonymized users and
// K26 unverified records offer none (visitor).
func (s IdentitySnapshot) options(now time.Time) []IdentityOption {
	if s.UserID == 0 || s.Unverified || s.Status == "anonymized" {
		return nil
	}
	var out []IdentityOption
	for _, m := range s.Members {
		if !m.usable(now) {
			continue
		}
		out = append(out, IdentityOption{
			Kind: model.IdentityPanelUser, OrgID: m.OrgID, BrandID: m.BrandID, OrgName: m.OrgName,
			ReadOnly: m.OrgStatus == "read_only", locale: m.OrgLocale,
		})
	}
	if s.Customer {
		out = append(out, IdentityOption{Kind: model.IdentityCustomer})
	}
	return out
}

// currentChoice returns the conversation's stored identity when it is still
// one of the options.
func currentChoice(conv db.Conversation, userID int64, options []IdentityOption) (IdentityOption, bool) {
	if !conv.IdentityUserID.Valid || conv.IdentityUserID.Int64 != userID {
		return IdentityOption{}, false
	}
	for _, o := range options {
		switch {
		case o.Kind == model.IdentityCustomer && conv.IdentityKind == model.IdentityCustomer:
			return o, true
		case o.Kind == model.IdentityPanelUser && conv.IdentityKind == model.IdentityPanelUser &&
			conv.IdentityOrgID.Valid && conv.IdentityOrgID.Int64 == o.OrgID:
			return o, true
		}
	}
	return IdentityOption{}, false
}

func sameIdentity(conv db.Conversation, kind string, userID, orgID int64) bool {
	return conv.IdentityKind == kind && conv.IdentityResolvedAt.Valid &&
		conv.IdentityUserID.Valid == (userID != 0) && conv.IdentityUserID.Int64 == userID &&
		conv.IdentityOrgID.Valid == (orgID != 0) && conv.IdentityOrgID.Int64 == orgID
}

func (r *IdentityResolver) setIdentity(ctx context.Context, convID int64, kind string, userID, orgID int64) (db.Conversation, error) {
	arg := db.SetConversationIdentityParams{ID: convID, IdentityKind: kind}
	if userID != 0 {
		arg.IdentityUserID = pgtype.Int8{Int64: userID, Valid: true}
	}
	if orgID != 0 {
		arg.IdentityOrgID = pgtype.Int8{Int64: orgID, Valid: true}
	}
	if kind != model.IdentityUnknown {
		arg.IdentityResolvedAt = pgtype.Timestamptz{Time: r.now().UTC(), Valid: true}
	}
	conv, err := r.store.SetConversationIdentity(ctx, arg)
	if err != nil {
		return db.Conversation{}, fmt.Errorf("whatsapp: set identity: %w", err)
	}
	return conv, nil
}

// userLocale is the user's locale, then the chosen organization's, then the
// conversation's; empty when none is known.
func (r *IdentityResolver) userLocale(snap IdentitySnapshot, choice IdentityOption, conv db.Conversation) string {
	if l, ok := ConversationLocale(snap.Locale); ok {
		return l
	}
	if l, ok := ConversationLocale(choice.locale); ok {
		return l
	}
	return convLocale(conv)
}

func (r *IdentityResolver) storeLocale(ctx context.Context, conv db.Conversation, locale string) (db.Conversation, error) {
	if conv.Locale.Valid && conv.Locale.String == locale {
		return conv, nil
	}
	updated, err := r.store.SetConversationLocale(ctx, db.SetConversationLocaleParams{
		ID: conv.ID, Locale: pgtype.Text{String: locale, Valid: true},
	})
	if err != nil {
		return db.Conversation{}, fmt.Errorf("whatsapp: set locale: %w", err)
	}
	return updated, nil
}

// detectAndStore detects the language of text and stores it on the
// conversation. Without text or when detection fails the answer is en and
// nothing is stored, so a later message detects again.
func (r *IdentityResolver) detectAndStore(ctx context.Context, conv db.Conversation, text string) (string, db.Conversation, error) {
	if strings.TrimSpace(text) == "" {
		return FallbackConversationLocale, conv, nil
	}
	locale, err := r.DetectLocale(ctx, conv, text)
	if err != nil {
		r.log.Warn("whatsapp_locale_detect_failed", "conversation_id", conv.ID, "error", err)
		return FallbackConversationLocale, conv, nil
	}
	conv, err = r.storeLocale(ctx, conv, locale)
	return locale, conv, err
}

// FallbackConversationLocale answers languages outside the 13 locales.
const FallbackConversationLocale = "en"

// detectSystem is the language detection prompt of the fast model.
const detectSystem = "Identify the language of the user's message. " +
	"Answer with only its ISO 639-1 code in lower case (for Chinese answer zh_CN), nothing else."

// DetectLocale asks the fast model (ai_settings.fast_model, else
// AI_MODEL_FAST) for the language of text. A language outside the 13
// supported locales is en.
func (r *IdentityResolver) DetectLocale(ctx context.Context, conv db.Conversation, text string) (string, error) {
	configured := ""
	if st, err := r.store.GetAISettings(ctx); err == nil {
		configured = st.FastModel
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return "", fmt.Errorf("whatsapp: ai settings: %w", err)
	}
	modelID := r.models.ResolveFast(configured)
	text = strings.TrimSpace(text)
	if len([]rune(text)) > 500 {
		text = string([]rune(text)[:500])
	}
	resp, err := llm.Complete(ctx, r.llm, llm.Request{
		Model:     modelID,
		System:    []llm.SystemBlock{{Text: detectSystem}},
		Messages:  []llm.Message{{Role: llm.RoleUser, Content: []llm.Block{llm.TextBlock(text)}}},
		MaxTokens: 8,
	})
	if err != nil {
		return "", err
	}
	if r.usage != nil {
		used := resp.Model
		if used == "" {
			used = modelID
		}
		if err := r.usage.RecordLocaleUsage(ctx, conv, used, resp.Usage); err != nil {
			r.log.Warn("whatsapp_locale_usage_failed", "conversation_id", conv.ID, "error", err)
		}
	}
	code := strings.Trim(strings.TrimSpace(resp.Message.Text()), "\"'`.")
	if i := strings.IndexAny(code, " \n\t"); i >= 0 {
		code = code[:i]
	}
	if l, ok := ConversationLocale(code); ok {
		return l, nil
	}
	return FallbackConversationLocale, nil
}

// ConversationLocale maps a locale code to the conversations.locale form
// (zh-CN -> zh_CN); ok is false outside the 13 supported locales.
func ConversationLocale(raw string) (string, bool) {
	l, ok := i18n.Parse(raw)
	if !ok {
		return "", false
	}
	return strings.ReplaceAll(string(l), "-", "_"), true
}

func convLocale(conv db.Conversation) string {
	if !conv.Locale.Valid {
		return ""
	}
	l, _ := ConversationLocale(conv.Locale.String)
	return l
}

var menuNumberRe = regexp.MustCompile(`^\s*(\d{1,2})\s*[).]?\s*$`)

// menuNumber parses a menu answer ("2", "2)", " 2. ").
func menuNumber(text string) (int, bool) {
	m := menuNumberRe.FindStringSubmatch(text)
	if m == nil {
		return 0, false
	}
	n, err := strconv.Atoi(m[1])
	return n, err == nil
}

// identityResetCommands reset the chosen identity (folded: lower case,
// Turkish letters to ASCII, single spaces).
var identityResetCommands = map[string]struct{}{
	"kimlik degistir": {},
	"change identity": {},
	"switch identity": {},
}

// IsIdentityResetCommand reports the "kimlik değiştir" command.
func IsIdentityResetCommand(text string) bool {
	folded := strings.NewReplacer("İ", "i", "I", "ı").Replace(strings.TrimSpace(text))
	folded = strings.ToLower(folded)
	folded = strings.NewReplacer("ı", "i", "ğ", "g", "ü", "u", "ş", "s", "ö", "o", "ç", "c").Replace(folded)
	folded = strings.Join(strings.Fields(strings.Trim(folded, ".!\"'")), " ")
	_, ok := identityResetCommands[folded]
	return ok
}

// Snapshot loads (or reads from the cache) what a number resolves to.
func (r *IdentityResolver) Snapshot(ctx context.Context, e164 string) (IdentitySnapshot, error) {
	if snap, ok := r.cache.Get(ctx, e164); ok {
		return snap, nil
	}
	snap, err := r.load(ctx, e164)
	if err != nil {
		return IdentitySnapshot{}, err
	}
	r.cache.Set(ctx, e164, snap)
	return snap, nil
}

func (r *IdentityResolver) load(ctx context.Context, e164 string) (IdentitySnapshot, error) {
	user, err := r.store.GetUserByPhone(ctx, pgtype.Text{String: e164, Valid: true})
	if errors.Is(err, pgx.ErrNoRows) {
		return IdentitySnapshot{}, nil
	}
	if err != nil {
		return IdentitySnapshot{}, fmt.Errorf("whatsapp: user by phone: %w", err)
	}
	// A merged duplicate (K26 merge) answers as the surviving user.
	if user.MergedIntoUserID.Valid {
		user, err = r.store.GetUserByID(ctx, user.MergedIntoUserID.Int64)
		if errors.Is(err, pgx.ErrNoRows) {
			return IdentitySnapshot{}, nil
		}
		if err != nil {
			return IdentitySnapshot{}, fmt.Errorf("whatsapp: merged user: %w", err)
		}
	}
	snap := IdentitySnapshot{
		UserID: user.ID, Status: user.Status, Locale: user.Locale.String, Unverified: user.LegacyUnverified,
	}
	if snap.Unverified || snap.Status == "anonymized" {
		return snap, nil
	}
	rows, err := r.store.ListWhatsAppIdentityMemberships(ctx, user.ID)
	if err != nil {
		return IdentitySnapshot{}, fmt.Errorf("whatsapp: memberships: %w", err)
	}
	for _, m := range rows {
		snap.Members = append(snap.Members, IdentityMember{
			OrgID: m.OrganizationID, BrandID: m.BrandID, OrgName: m.Name, OrgType: m.Type,
			OrgStatus: m.Status, OrgLocale: m.Locale,
			AccessStarts: tsPtr(m.AccessStartsAt), AccessEnds: tsPtr(m.AccessEndsAt),
		})
	}
	if snap.Customer, err = r.store.IsCustomerUser(ctx, user.ID); err != nil {
		return IdentitySnapshot{}, fmt.Errorf("whatsapp: customer: %w", err)
	}
	return snap, nil
}
