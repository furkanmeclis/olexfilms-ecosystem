package ioengine

import (
	"context"
	"fmt"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
)

// JobOrgCovers reports whether an export job of organization jobOrgID may
// read a record of organization orgID in brand brandID: the same
// organization, one below it, or any organization of the brand when the job
// organization is the brand center. Workers call it to re-authorize a job
// whose HTTP request was already checked (TEC-188, TEC-196).
func JobOrgCovers(ctx context.Context, q *db.Queries, jobOrgID, orgID, brandID int64) (bool, error) {
	if jobOrgID <= 0 {
		return false, nil
	}
	if jobOrgID == orgID {
		return true, nil
	}
	org, err := q.GetOrganizationByID(ctx, jobOrgID)
	if err != nil {
		return false, fmt.Errorf("export scope: job organization: %w", err)
	}
	if org.BrandID != brandID {
		return false, nil
	}
	if org.Type == "center" {
		return true, nil
	}
	below, err := q.Descendants(ctx, jobOrgID)
	if err != nil {
		return false, fmt.Errorf("export scope: descendants: %w", err)
	}
	for _, o := range below {
		if o.ID == orgID {
			return true, nil
		}
	}
	return false, nil
}
