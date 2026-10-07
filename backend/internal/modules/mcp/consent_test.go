package mcp

import (
	"context"
	"errors"
	"testing"

	aitools "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/ai/tools"
	oauthmodel "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/oauth/model"
	oauthusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/oauth/usecase"
)

var _ oauthusecase.ToolCounter = ToolCounter{}

type recordingResolver struct {
	got oauthmodel.AccessToken
	err error
}

func (r *recordingResolver) Resolve(_ context.Context, tok oauthmodel.AccessToken) (aitools.Principal, error) {
	r.got = tok
	return aitools.Principal{Realm: aitools.RealmPanel}, r.err
}

type countLister struct{ n int }

func (l countLister) Available(context.Context, aitools.Principal) ([]aitools.Tool, error) {
	return make([]aitools.Tool, l.n), nil
}

// TEC-403: the consent tool count is the tools/list size of a token of
// that user, organization and endpoint; a refused principal counts 0.
func TestToolCounter(t *testing.T) {
	res := &recordingResolver{}
	c := ToolCounter{Principals: res, Tools: countLister{n: 12}}
	n, err := c.CountTools(context.Background(), 7, 20, 1, oauthmodel.ResourceDealer)
	if err != nil || n != 12 {
		t.Fatalf("count = %d, %v", n, err)
	}
	if res.got.UserID != 7 || res.got.OrganizationID != 20 || res.got.BrandID != 1 || res.got.Resource != oauthmodel.ResourceDealer {
		t.Fatalf("token = %+v", res.got)
	}
	res.err = ErrForbidden
	if n, err := c.CountTools(context.Background(), 7, 20, 1, oauthmodel.ResourceDealer); err != nil || n != 0 {
		t.Fatalf("forbidden count = %d, %v", n, err)
	}
	res.err = errors.New("db down")
	if _, err := c.CountTools(context.Background(), 7, 20, 1, oauthmodel.ResourceDealer); err == nil {
		t.Fatal("resolver error swallowed")
	}
}
