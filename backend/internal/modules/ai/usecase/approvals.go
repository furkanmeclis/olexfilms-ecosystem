package usecase

// approvals.go (TEC-403, F4-03d) is the panel screen "pending AI actions":
// write tools proposed outside the panel chat (MCP clients, WhatsApp) are
// listed there and confirmed or cancelled with a plain request. Chat cards
// (panel / portal) stay in their conversation, because confirming them
// resumes the paused turn (Chat.PrepareAction).

import (
	"context"
	"net/url"
	"slices"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/ai/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/ai/tools"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// ApprovalSources are the sources whose actions the approvals screen
// lists and decides.
var ApprovalSources = []string{model.SourceMCP, model.SourceWhatsApp}

// PendingActionsSortSpec is the sort whitelist of GET /v1/ai/pending-actions.
var PendingActionsSortSpec = apiquery.SortSpec{
	Columns: apiquery.SortColumns{"created_at": "created_at", "expires_at": "expires_at", "tool_name": "tool_name"},
	Default: apiquery.SortField{Field: "created_at", Desc: true},
}

// ApprovalFilter is a parsed list query of the approvals screen.
type ApprovalFilter struct {
	Q string
	// Sources narrows ApprovalSources; empty means all of them.
	Sources       []string
	Sort          apiquery.ResolvedSort
	Limit, Offset int32
}

// ApprovalStore is the paged read of the screen (ai/repository.Store).
type ApprovalStore interface {
	PendingActionsPage(ctx context.Context, p db.ListAIPendingActionsPageParams) ([]db.AiPendingAction, int64, error)
}

// Approvals lists and decides the caller's pending actions of
// ApprovalSources in the active organization.
type Approvals struct {
	store   ApprovalStore
	actions *Actions
}

// NewApprovals builds the screen use case.
func NewApprovals(store ApprovalStore, actions *Actions) *Approvals {
	return &Approvals{store: store, actions: actions}
}

// ParseApprovalFilter reads q, source (CSV mcp,whatsapp), sort, limit and
// offset; unknown values are a *apiquery.ValidationError.
func ParseApprovalFilter(values url.Values) (ApprovalFilter, error) {
	q := apiquery.Parse(values)
	f := ApprovalFilter{Q: strings.TrimSpace(q.Q), Limit: q.Limit, Offset: q.Offset}
	var err error
	if f.Sort, err = apiquery.ResolveSort(q.Sort, PendingActionsSortSpec); err != nil {
		return f, err
	}
	f.Sources, err = apiquery.EnumList(values, "source", ApprovalSources...)
	return f, err
}

// List returns one page of the caller's open cards.
func (a *Approvals) List(ctx context.Context, p tools.Principal, f ApprovalFilter) ([]Card, int64, error) {
	if p.Org == nil {
		return []Card{}, 0, nil
	}
	sources := f.Sources
	if len(sources) == 0 {
		sources = ApprovalSources
	}
	var q pgtype.Text
	if f.Q != "" {
		q = pgtype.Text{String: f.Q, Valid: true}
	}
	rows, total, err := a.store.PendingActionsPage(ctx, db.ListAIPendingActionsPageParams{
		OrganizationID: p.Org.InternalID, UserID: p.Auth.UserInternal, Sources: sources, Q: q,
		SortKey: f.Sort.Key, SortDesc: f.Sort.Desc, RowLimit: f.Limit, RowOffset: f.Offset,
	})
	if err != nil {
		return nil, 0, err
	}
	out := make([]Card, 0, len(rows))
	for _, r := range rows {
		out = append(out, cardOf(r))
	}
	return out, total, nil
}

// Decide confirms (confirm true) or cancels the caller's action. Chat
// cards are not found here: they are decided in their conversation.
// Errors are those of Actions.Confirm / Cancel (ErrorStatus).
func (a *Approvals) Decide(ctx context.Context, p tools.Principal, id uuid.UUID, confirm bool) (Outcome, error) {
	card, err := a.actions.Lookup(ctx, p, id)
	if err != nil {
		return Outcome{}, err
	}
	if !slices.Contains(ApprovalSources, card.Source) {
		return Outcome{}, ErrActionNotFound
	}
	if confirm {
		return a.actions.Confirm(ctx, p, id, nil)
	}
	return a.actions.Cancel(ctx, p, id)
}
