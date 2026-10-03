package usecase

import (
	"errors"
	"strings"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/jackc/pgx/v5/pgtype"
)

func intp(v int) *int       { return &v }
func strp(v string) *string { return &v }

func TestNormalizeReview(t *testing.T) {
	cases := []struct {
		name  string
		in    ServiceReviewInput
		field string // "" = valid
	}{
		{"valid", ServiceReviewInput{PlatformRating: intp(5), ProductRating: intp(1)}, ""},
		{"missing platform", ServiceReviewInput{ProductRating: intp(3)}, "platform_rating"},
		{"missing product", ServiceReviewInput{PlatformRating: intp(3)}, "product_rating"},
		{"platform zero", ServiceReviewInput{PlatformRating: intp(0), ProductRating: intp(3)}, "platform_rating"},
		{"product six", ServiceReviewInput{PlatformRating: intp(3), ProductRating: intp(6)}, "product_rating"},
		{"comment too long", ServiceReviewInput{PlatformRating: intp(3), ProductRating: intp(3),
			Comment: strp(strings.Repeat("ş", MaxReviewCommentLength+1))}, "comment"},
	}
	for _, c := range cases {
		_, _, _, err := normalizeReview(c.in)
		var ve *ValidationError
		switch {
		case c.field == "" && err != nil:
			t.Fatalf("%s: unexpected error %v", c.name, err)
		case c.field != "" && (!errors.As(err, &ve) || ve.Field != c.field):
			t.Fatalf("%s: err = %v, want validation error on %s", c.name, err, c.field)
		}
	}
}

func TestNormalizeReviewComment(t *testing.T) {
	in := ServiceReviewInput{PlatformRating: intp(4), ProductRating: intp(5)}
	in.Comment = strp("   ")
	if _, _, c, err := normalizeReview(in); err != nil || c.Valid {
		t.Fatalf("blank comment must be none: %+v %v", c, err)
	}
	in.Comment = strp("  güzel iş  ")
	p, q, c, err := normalizeReview(in)
	if err != nil || p != 4 || q != 5 || c.String != "güzel iş" {
		t.Fatalf("normalized = %d %d %+v %v", p, q, c, err)
	}
	in.Comment = strp(strings.Repeat("ğ", MaxReviewCommentLength))
	if _, _, _, err := normalizeReview(in); err != nil {
		t.Fatalf("a comment of exactly %d characters is allowed: %v", MaxReviewCommentLength, err)
	}
}

func TestGoogleURL(t *testing.T) {
	if googleURL(db.Organization{}) != nil {
		t.Fatal("unset url must be nil")
	}
	if googleURL(db.Organization{GoogleBusinessUrl: pgtype.Text{String: " ", Valid: true}}) != nil {
		t.Fatal("blank url must be nil")
	}
	u := googleURL(db.Organization{GoogleBusinessUrl: pgtype.Text{String: "https://g.page/r/x", Valid: true}})
	if u == nil || *u != "https://g.page/r/x" {
		t.Fatalf("url = %v", u)
	}
}
