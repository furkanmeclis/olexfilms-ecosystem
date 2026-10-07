package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/ai/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/ai/repository"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/ai/tools"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
)

var _ ApprovalStore = (*repository.Store)(nil)

// pageStore records the page query and answers from the fake action rows.
type pageStore struct {
	rows *fakeStore
	got  db.ListAIPendingActionsPageParams
}

func (s *pageStore) PendingActionsPage(_ context.Context, p db.ListAIPendingActionsPageParams) ([]db.AiPendingAction, int64, error) {
	s.got = p
	s.rows.mu.Lock()
	defer s.rows.mu.Unlock()
	var out []db.AiPendingAction
	for _, r := range s.rows.rows {
		for _, src := range p.Sources {
			if r.Source == src && r.Status == model.ActionPending && r.UserID == p.UserID {
				out = append(out, *r)
			}
		}
	}
	return out, int64(len(out)), nil
}

func (h *harness) proposeFrom(p tools.Principal, source, toolUseID string) Card {
	h.t.Helper()
	out, err := h.actions.Propose(h.ctx, ProposeCall{Principal: p, Source: source, SourceRef: "ref-" + source,
		ToolUseID: toolUseID, ToolName: "create_note", Input: json.RawMessage(`{"title":"From ` + source + `"}`)})
	if err != nil || out.Card == nil {
		h.t.Fatalf("propose: %+v %v", out, err)
	}
	return *out.Card
}

func TestParseApprovalFilter(t *testing.T) {
	f, err := ParseApprovalFilter(url.Values{"q": {" lead "}, "source": {"mcp"}, "sort": {"expires_at"}, "limit": {"5"}, "offset": {"10"}})
	if err != nil || f.Q != "lead" || len(f.Sources) != 1 || f.Sources[0] != "mcp" ||
		f.Sort.Key != "expires_at" || f.Sort.Desc || f.Limit != 5 || f.Offset != 10 {
		t.Fatalf("filter: %+v %v", f, err)
	}
	if f, err := ParseApprovalFilter(url.Values{}); err != nil || f.Sort.Key != "created_at" || !f.Sort.Desc {
		t.Fatalf("default sort: %+v %v", f, err)
	}
	var qe *apiquery.ValidationError
	if _, err := ParseApprovalFilter(url.Values{"source": {"panel"}}); !errors.As(err, &qe) {
		t.Fatalf("panel source must be rejected: %v", err)
	}
	if _, err := ParseApprovalFilter(url.Values{"sort": {"input"}}); !errors.As(err, &qe) {
		t.Fatalf("unknown sort must be rejected: %v", err)
	}
}

// TEC-403: the approvals screen lists only MCP / WhatsApp cards and
// decides them without a conversation; chat cards are not found there.
func TestApprovalsListAndDecide(t *testing.T) {
	h := newHarness(t, nil)
	owner := user(7, rbac.PermTasksWrite)
	store := &pageStore{rows: h.store}
	ap := NewApprovals(store, h.actions)

	chat := h.proposeFrom(owner, model.SourcePanel, "toolu_chat")
	mcp := h.proposeFrom(owner, model.SourceMCP, "toolu_mcp")
	wa := h.proposeFrom(owner, model.SourceWhatsApp, "toolu_wa")

	list, total, err := ap.List(h.ctx, owner, ApprovalFilter{Sort: apiquery.ResolvedSort{Key: "created_at", Desc: true}, Limit: 20})
	if err != nil || total != 2 || len(list) != 2 {
		t.Fatalf("list: %+v %d %v", list, total, err)
	}
	if got := store.got; got.OrganizationID != 10 || got.UserID != 7 || len(got.Sources) != 2 ||
		got.SortKey != "created_at" || !got.SortDesc || got.Q.Valid || got.RowLimit != 20 {
		t.Fatalf("page params: %+v", got)
	}
	if _, _, err := ap.List(h.ctx, owner, ApprovalFilter{Q: "lead", Sources: []string{model.SourceMCP}}); err != nil ||
		!store.got.Q.Valid || store.got.Q.String != "lead" || len(store.got.Sources) != 1 {
		t.Fatalf("filtered params: %+v %v", store.got, err)
	}

	if _, err := ap.Decide(h.ctx, owner, chat.ActionUUID, true); !errors.Is(err, ErrActionNotFound) {
		t.Fatalf("chat card on the approvals screen: %v", err)
	}
	if _, err := ap.Decide(h.ctx, user(8, rbac.PermTasksWrite), mcp.ActionUUID, true); !errors.Is(err, ErrActionNotFound) {
		t.Fatalf("another user's card: %v", err)
	}
	out, err := ap.Decide(h.ctx, owner, mcp.ActionUUID, true)
	if err != nil || out.Status != model.ActionConfirmed || out.Source != model.SourceMCP {
		t.Fatalf("confirm mcp: %+v %v", out, err)
	}
	if n := h.writes.Load(); n != 1 {
		t.Fatalf("writes = %d, want 1", n)
	}
	out, err = ap.Decide(h.ctx, owner, wa.ActionUUID, false)
	if err != nil || out.Status != model.ActionCancelled {
		t.Fatalf("cancel whatsapp: %+v %v", out, err)
	}
	if _, err := ap.Decide(h.ctx, owner, wa.ActionUUID, true); !errors.Is(err, ErrActionResolved) {
		t.Fatalf("confirm after cancel: %v", err)
	}
}

// An expired MCP card is refused with AI_ACTION_EXPIRED.
func TestApprovalsExpired(t *testing.T) {
	h := newHarness(t, nil)
	owner := user(7, rbac.PermTasksWrite)
	ap := NewApprovals(&pageStore{rows: h.store}, h.actions)
	card := h.proposeFrom(owner, model.SourceMCP, "toolu_old")
	h.now = h.now.Add(PendingActionTTL + 1)
	if _, err := ap.Decide(h.ctx, owner, card.ActionUUID, true); !errors.Is(err, ErrActionExpired) {
		t.Fatalf("expired: %v", err)
	}
	if n := h.writes.Load(); n != 0 {
		t.Fatalf("writes = %d, want 0", n)
	}
}
