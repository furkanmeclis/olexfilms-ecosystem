package handler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/rebuild"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// maxReportDiffs caps the differences returned by the HTTP check; the CLI
// prints all of them.
const maxReportDiffs = 1000

// RebuildChecker runs a dry-run projection scan.
type RebuildChecker interface {
	Check(ctx context.Context, orgID int64) (rebuild.Report, error)
}

// OrganizationLookup resolves an organization uuid.
type OrganizationLookup interface {
	GetOrganizationByUUID(ctx context.Context, id uuid.UUID) (db.Organization, error)
}

// Rebuild serves the super_admin projection drift check (TEC-156).
type Rebuild struct {
	svc  RebuildChecker
	orgs OrganizationLookup
}

// NewRebuild creates the handler.
func NewRebuild(svc RebuildChecker, orgs OrganizationLookup) *Rebuild {
	return &Rebuild{svc: svc, orgs: orgs}
}

type rebuildCheckRequest struct {
	OrganizationUUID *string `json:"organization_uuid"`
}

// RebuildCheckResponse is the dry-run report.
type RebuildCheckResponse struct {
	rebuild.Report
	DiffsTruncated bool `json:"diffs_truncated"`
}

// Check serves POST /v1/platform/stock/rebuild-check: replay the ledger and
// report the drift of the projections. It never writes; repairs run from
// cmd/inventory-rebuild -apply.
func (h *Rebuild) Check(w http.ResponseWriter, r *http.Request) {
	var in rebuildCheckRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&in); err != nil && !errors.Is(err, io.EOF) {
		response.BadRequest(w, r, response.CodeValidationError, "request body is invalid")
		return
	}
	var orgID int64
	if in.OrganizationUUID != nil && strings.TrimSpace(*in.OrganizationUUID) != "" {
		id, err := uuid.Parse(strings.TrimSpace(*in.OrganizationUUID))
		if err != nil {
			response.BadRequest(w, r, response.CodeValidationError, "organization_uuid is invalid")
			return
		}
		org, err := h.orgs.GetOrganizationByUUID(r.Context(), id)
		if errors.Is(err, pgx.ErrNoRows) {
			response.NotFound(w, r, "Organization not found")
			return
		}
		if err != nil {
			response.InternalErr(w, r, err, "stock rebuild check failed")
			return
		}
		orgID = org.ID
	}
	rep, err := h.svc.Check(r.Context(), orgID)
	if err != nil {
		response.InternalErr(w, r, err, "stock rebuild check failed")
		return
	}
	out := RebuildCheckResponse{Report: rep}
	if len(out.Diffs) > maxReportDiffs {
		out.Diffs, out.DiffsTruncated = out.Diffs[:maxReportDiffs], true
	}
	response.JSON(w, r, http.StatusOK, out)
}
