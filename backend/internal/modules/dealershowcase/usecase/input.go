package usecase

import (
	"encoding/json"
	"net/url"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/appointments/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/dealershowcase/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/msgtemplate"
	"github.com/jackc/pgx/v5/pgtype"
)

// Input limits (characters); the DB CHECKs mirror the structural ones.
const (
	MaxHeadline     = 160
	MaxAbout        = 5000
	MaxKeywords     = 30
	MaxKeyword      = 100
	MaxLink         = 500
	MaxPlaceID      = 255
	MaxServiceTitle = 200
	MaxServiceDesc  = 2000
	MaxCaption      = 300
)

// LocaleContent is the showcase text of one locale.
type LocaleContent struct {
	Headline string `json:"headline"`
	About    string `json:"about"`
}

// Input is the body of PUT /v1/showcase: the whole draft.
type Input struct {
	Content       map[string]LocaleContent `json:"content"`
	WorkingHours  json.RawMessage          `json:"working_hours"`
	SocialLinks   map[string]string        `json:"social_links"`
	SeoKeywords   []string                 `json:"seo_keywords"`
	GooglePlaceID *string                  `json:"google_place_id"`
}

func knownLocale(l string) bool { return slices.Contains(msgtemplate.Locales, l) }

func (in Input) params() (db.UpsertDealerShowcaseParams, error) {
	var p db.UpsertDealerShowcaseParams
	content := map[string]map[string]string{}
	for loc, c := range in.Content {
		if !knownLocale(loc) {
			return p, invalid("content", "unknown locale "+loc)
		}
		h, a := strings.TrimSpace(c.Headline), strings.TrimSpace(c.About)
		if utf8.RuneCountInString(h) > MaxHeadline {
			return p, invalid("content."+loc+".headline", "too long")
		}
		if utf8.RuneCountInString(a) > MaxAbout {
			return p, invalid("content."+loc+".about", "too long")
		}
		if h == "" && a == "" {
			continue
		}
		fields := map[string]string{}
		if h != "" {
			fields[model.ContentHeadline] = h
		}
		if a != "" {
			fields[model.ContentAbout] = a
		}
		content[loc] = fields
	}
	var err error
	if p.Content, err = json.Marshal(content); err != nil {
		return p, err
	}

	hours := []byte(strings.TrimSpace(string(in.WorkingHours)))
	if len(hours) == 0 || string(hours) == "null" {
		hours = []byte("{}")
	}
	if err := usecase.ValidateWorkingHours(hours); err != nil {
		return p, invalid("working_hours", err.Error())
	}
	p.WorkingHours = hours

	links := map[string]string{}
	for k, v := range in.SocialLinks {
		if !slices.Contains(model.SocialNetworks, k) {
			return p, invalid("social_links", "unknown network "+k)
		}
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		u, err := url.Parse(v)
		if err != nil || u.Scheme != "https" || u.Host == "" || strings.ContainsAny(v, " \t\n") || len(v) > MaxLink {
			return p, invalid("social_links."+k, "must be an https URL")
		}
		links[k] = v
	}
	if p.SocialLinks, err = json.Marshal(links); err != nil {
		return p, err
	}

	p.SeoKeywords = []string{}
	seen := map[string]bool{}
	for _, k := range in.SeoKeywords {
		k = strings.TrimSpace(k)
		if k == "" || seen[strings.ToLower(k)] {
			continue
		}
		if utf8.RuneCountInString(k) > MaxKeyword {
			return p, invalid("seo_keywords", "a keyword is too long")
		}
		seen[strings.ToLower(k)] = true
		p.SeoKeywords = append(p.SeoKeywords, k)
	}
	if len(p.SeoKeywords) > MaxKeywords {
		return p, invalid("seo_keywords", "at most 30 keywords")
	}

	if in.GooglePlaceID != nil {
		if id := strings.TrimSpace(*in.GooglePlaceID); id != "" {
			if len(id) > MaxPlaceID {
				return p, invalid("google_place_id", "too long")
			}
			p.GooglePlaceID = pgtype.Text{String: id, Valid: true}
		}
	}
	return p, nil
}

// localeTexts validates a locale → text map (service titles and
// descriptions, captions) and drops empty entries.
func localeTexts(field string, in map[string]string, max int) ([]byte, map[string]string, error) {
	out := map[string]string{}
	for loc, v := range in {
		if !knownLocale(loc) {
			return nil, nil, invalid(field, "unknown locale "+loc)
		}
		v = strings.TrimSpace(v)
		if utf8.RuneCountInString(v) > max {
			return nil, nil, invalid(field+"."+loc, "too long")
		}
		if v != "" {
			out[loc] = v
		}
	}
	raw, err := json.Marshal(out)
	return raw, out, err
}
