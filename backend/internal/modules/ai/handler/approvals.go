package handler

// TEC-403 (F4-03d): the panel screen "pending AI actions" (MCP and
// WhatsApp write tools waiting for the user's approval).

import (
	"net/http"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/ai/tools"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/ai/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
)

// Approvals serves /v1/ai/pending-actions.
type Approvals struct {
	uc *usecase.Approvals
}

// NewApprovals builds the handler.
func NewApprovals(uc *usecase.Approvals) *Approvals {
	return &Approvals{uc: uc}
}

func approvalPrincipal(r *http.Request) tools.Principal {
	p := tools.Principal{Auth: authctx.MustPrincipal(r.Context()), Realm: tools.RealmPanel}
	if s, ok := orgctx.ScopeFrom(r.Context()); ok {
		p.Org = &s
	}
	return p
}

// List serves GET /v1/ai/pending-actions.
func (h *Approvals) List(w http.ResponseWriter, r *http.Request) {
	f, err := usecase.ParseApprovalFilter(r.URL.Query())
	if err != nil {
		writeError(w, r, err)
		return
	}
	items, total, err := h.uc.List(r.Context(), approvalPrincipal(r), f)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, f.Limit, f.Offset))
}

// Confirm serves POST /v1/ai/pending-actions/{uuid}/confirm.
func (h *Approvals) Confirm(w http.ResponseWriter, r *http.Request) {
	h.decide(w, r, true)
}

// Cancel serves POST /v1/ai/pending-actions/{uuid}/cancel.
func (h *Approvals) Cancel(w http.ResponseWriter, r *http.Request) {
	h.decide(w, r, false)
}

func (h *Approvals) decide(w http.ResponseWriter, r *http.Request, confirm bool) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	out, err := h.uc.Decide(r.Context(), approvalPrincipal(r), id, confirm)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}
