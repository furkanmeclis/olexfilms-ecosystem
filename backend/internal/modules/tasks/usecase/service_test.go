package usecase

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestNonCenterCallerIsRejected(t *testing.T) {
	s := New(nil, nil, nil)
	ctx := context.Background()
	for _, typ := range []string{"distributor", "dealer", ""} {
		c := Caller{UserID: 1, Org: orgctx.Scope{InternalID: 7, OrgType: typ, BrandID: 1}}
		id := uuid.New()
		if _, _, err := s.List(ctx, c, Filter{}); !errors.Is(err, ErrCenterOnly) {
			t.Fatalf("%s list: %v", typ, err)
		}
		if _, err := s.Get(ctx, c, id); !errors.Is(err, ErrCenterOnly) {
			t.Fatalf("%s get: %v", typ, err)
		}
		if _, err := s.Create(ctx, c, CreateInput{Title: "x", SubjectOrgUUID: id}); !errors.Is(err, ErrCenterOnly) {
			t.Fatalf("%s create: %v", typ, err)
		}
		if _, err := s.Update(ctx, c, id, UpdateInput{}); !errors.Is(err, ErrCenterOnly) {
			t.Fatalf("%s update: %v", typ, err)
		}
		if _, _, err := s.ListComments(ctx, c, id, 10, 0); !errors.Is(err, ErrCenterOnly) {
			t.Fatalf("%s comments: %v", typ, err)
		}
		if _, err := s.AddComment(ctx, c, id, "hi"); !errors.Is(err, ErrCenterOnly) {
			t.Fatalf("%s comment: %v", typ, err)
		}
	}
}

func TestCreateValidatesBeforeTouchingTheDatabase(t *testing.T) {
	s := New(nil, nil, nil)
	c := Caller{UserID: 1, Org: orgctx.Scope{InternalID: 7, OrgType: "center", BrandID: 1}}
	cases := map[string]CreateInput{
		"title":       {Title: "   "},
		"description": {Title: "ok", Description: strings.Repeat("a", maxDescription+1)},
		"priority":    {Title: "ok", Priority: "asap"},
	}
	for field, in := range cases {
		_, err := s.Create(context.Background(), c, in)
		var ve *ValidationError
		if !errors.As(err, &ve) || ve.Field != field {
			t.Fatalf("%s: got %v", field, err)
		}
	}
	if _, err := cleanTitle(strings.Repeat("ç", maxTitle)); err != nil {
		t.Fatalf("title of %d runes: %v", maxTitle, err)
	}
	if _, err := cleanTitle(strings.Repeat("ç", maxTitle+1)); err == nil {
		t.Fatal("title too long accepted")
	}
}

func TestListAndCommentValidation(t *testing.T) {
	s := New(nil, nil, nil)
	c := Caller{UserID: 1, Org: orgctx.Scope{InternalID: 7, OrgType: "center", BrandID: 1}}
	var ve *ValidationError
	if _, _, err := s.List(context.Background(), c, Filter{Status: "closed"}); !errors.As(err, &ve) || ve.Field != "status" {
		t.Fatalf("status filter: %v", err)
	}
	if _, _, err := s.List(context.Background(), c, Filter{Priority: "p0"}); !errors.As(err, &ve) || ve.Field != "priority" {
		t.Fatalf("priority filter: %v", err)
	}
	if _, err := s.AddComment(context.Background(), c, uuid.New(), "  "); !errors.As(err, &ve) || ve.Field != "body" {
		t.Fatalf("empty comment: %v", err)
	}
	if _, err := s.AddComment(context.Background(), c, uuid.New(), strings.Repeat("a", maxComment+1)); !errors.As(err, &ve) || ve.Field != "body" {
		t.Fatalf("long comment: %v", err)
	}
}

func TestStatusHelpers(t *testing.T) {
	for _, st := range []string{StatusOpen, StatusInProgress, StatusDone, StatusCancelled} {
		if !validStatus(st) {
			t.Fatalf("%s invalid", st)
		}
	}
	if validStatus("closed") || validPriority("p0") {
		t.Fatal("unknown values accepted")
	}
	if !closedStatus(StatusDone) || !closedStatus(StatusCancelled) || closedStatus(StatusInProgress) {
		t.Fatal("closedStatus")
	}
	a := pgtype.Timestamptz{}
	if !sameTime(a, pgtype.Timestamptz{}) {
		t.Fatal("two nulls differ")
	}
}
