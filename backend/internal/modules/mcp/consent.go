package mcp

import (
	"context"
	"errors"

	aitools "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/ai/tools"
	oauthmodel "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/oauth/model"
)

// ToolLister lists the tools of a principal (*aitools.Registry).
type ToolLister interface {
	Available(ctx context.Context, p aitools.Principal) ([]aitools.Tool, error)
}

// ToolCounter counts the tools a connection would get, for the consent
// screen summary (TEC-403): the same principal and tools/list result as a
// token of that user, organization and endpoint.
type ToolCounter struct {
	Principals Resolver
	Tools      ToolLister
}

// CountTools implements the oauth usecase ToolCounter; a user who could
// not use the endpoint there gets 0.
func (c ToolCounter) CountTools(ctx context.Context, userID, orgID, brandID int64, resource string) (int, error) {
	p, err := c.Principals.Resolve(ctx, oauthmodel.AccessToken{
		UserID: userID, OrganizationID: orgID, BrandID: brandID, Resource: resource,
	})
	if errors.Is(err, ErrForbidden) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	ts, err := c.Tools.Available(ctx, p)
	if err != nil {
		return 0, err
	}
	return len(ts), nil
}
