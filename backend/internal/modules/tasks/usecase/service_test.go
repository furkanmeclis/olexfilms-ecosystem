package usecase

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
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
	if _, _, err := s.List(context.Background(), c, Filter{Statuses: []string{"closed"}}); !errors.As(err, &ve) || ve.Field != "status" {
		t.Fatalf("status filter: %v", err)
	}
	if _, _, err := s.List(context.Background(), c, Filter{Priorities: []string{"p0"}}); !errors.As(err, &ve) || ve.Field != "priority" {
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

func TestParseListFilter(t *testing.T) {
	me := uuid.New()
	f, err := ParseListFilter(url.Values{
		"status": {"active,done"}, "priority": {"high,urgent"}, "source": {"auto"}, "q": {"50%"}, "sort": {"-due_at"},
		"mine": {"true"}, "due_from": {"2026-10-01"}, "due_to": {"2026-10-02"}, "created_from": {"2026-09-01"},
	}, &me)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(f.Statuses, ",") != "done,in_progress,open" || strings.Join(f.Priorities, ",") != "high,urgent" {
		t.Fatalf("enums: %v %v", f.Statuses, f.Priorities)
	}
	if strings.Join(f.Sources, ",") != "auto" {
		t.Fatalf("source: %v", f.Sources)
	}
	if f.Sort.Key != "due_at" || !f.Sort.Desc || f.AssigneeUUID == nil || *f.AssigneeUUID != me {
		t.Fatalf("sort/mine: %+v", f)
	}
	if !f.DueBefore.Equal(time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC)) || f.CreatedFrom == nil {
		t.Fatalf("ranges: %+v", f)
	}
	if textArg(f.Q).String != `50\%` {
		t.Fatalf("q not escaped: %q", textArg(f.Q).String)
	}
	def, _ := ParseListFilter(url.Values{}, nil)
	if def.Sort.Key != "created_at" || !def.Sort.Desc {
		t.Fatalf("default sort: %+v", def.Sort)
	}
	for _, bad := range []url.Values{
		{"status": {"closed"}}, {"priority": {"p0"}}, {"source": {"rule"}}, {"sort": {"comment_count"}},
		{"subject_organization_uuid": {"x"}}, {"mine": {"true"}}, {"due_after": {"2026-10-01"}},
		{"due_after": {"2026-10-01T00:00:00Z"}, "due_from": {"2026-10-01"}},
	} {
		var ve *apiquery.ValidationError
		if _, err := ParseListFilter(bad, nil); !errors.As(err, &ve) {
			t.Fatalf("%v: got %v", bad, err)
		}
	}
}
