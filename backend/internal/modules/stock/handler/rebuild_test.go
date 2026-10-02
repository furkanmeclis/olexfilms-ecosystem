package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/rebuild"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type fakeChecker struct {
	org   int64
	diffs int
}

func (f *fakeChecker) Check(_ context.Context, orgID int64) (rebuild.Report, error) {
	f.org = orgID
	rep := rebuild.Report{OrganizationID: orgID, DiffCount: f.diffs, Diffs: make([]rebuild.Diff, f.diffs)}
	return rep, nil
}

type fakeOrgs struct{ known uuid.UUID }

func (f fakeOrgs) GetOrganizationByUUID(_ context.Context, id uuid.UUID) (db.Organization, error) {
	if id != f.known {
		return db.Organization{}, pgx.ErrNoRows
	}
	return db.Organization{ID: 77}, nil
}

func TestRebuildCheck(t *testing.T) {
	known := uuid.New()
	cases := []struct {
		name    string
		body    string
		diffs   int
		status  int
		wantOrg int64
	}{
		{"empty body scans everything", "", 0, http.StatusOK, 0},
		{"organization scope", `{"organization_uuid":"` + known.String() + `"}`, 2, http.StatusOK, 77},
		{"invalid uuid", `{"organization_uuid":"x"}`, 0, http.StatusBadRequest, -1},
		{"unknown organization", `{"organization_uuid":"` + uuid.NewString() + `"}`, 0, http.StatusNotFound, -1},
		{"invalid json", `{`, 0, http.StatusBadRequest, -1},
		{"truncated", `{}`, maxReportDiffs + 5, http.StatusOK, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			chk := &fakeChecker{org: -1, diffs: tc.diffs}
			h := NewRebuild(chk, fakeOrgs{known: known})
			req := httptest.NewRequest(http.MethodPost, "/v1/platform/stock/rebuild-check", strings.NewReader(tc.body))
			rec := httptest.NewRecorder()
			h.Check(rec, req)
			if rec.Code != tc.status {
				t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
			}
			if chk.org != tc.wantOrg {
				t.Fatalf("checked organization %d, want %d", chk.org, tc.wantOrg)
			}
			if tc.status != http.StatusOK {
				return
			}
			var env struct {
				Data RebuildCheckResponse `json:"data"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
				t.Fatal(err)
			}
			wantTrunc := tc.diffs > maxReportDiffs
			if env.Data.DiffsTruncated != wantTrunc || len(env.Data.Diffs) != min(tc.diffs, maxReportDiffs) ||
				env.Data.DiffCount != tc.diffs || env.Data.Applied {
				t.Fatalf("report = truncated %v, %d diffs of %d", env.Data.DiffsTruncated, len(env.Data.Diffs), env.Data.DiffCount)
			}
		})
	}
}
