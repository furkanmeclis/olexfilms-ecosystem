package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	appointments "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/appointments/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/dealershowcase/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/features"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// PublicShowcase is the `showcase` block of GET /v1/public/dealers/{code}
// (TEC-467): the published snapshot in the requested locale. It is present
// only while the dealer_showcase module is on and a snapshot exists; it
// never carries a price (F5 S8).
type PublicShowcase struct {
	Locale             string            `json:"locale"`
	Headline           string            `json:"headline"`
	About              string            `json:"about"`
	WorkingHours       []PublicDay       `json:"working_hours"`
	OpenNow            *bool             `json:"open_now"`
	Timezone           string            `json:"timezone"`
	Services           []PublicService   `json:"services"`
	Photos             []PublicPhoto     `json:"photos"`
	SocialLinks        map[string]string `json:"social_links"`
	SeoKeywords        []string          `json:"seo_keywords"`
	GoogleRating       *float64          `json:"google_rating"`
	GoogleReviewCount  *int32            `json:"google_review_count"`
	GoogleRatingSource *string           `json:"google_rating_source"`
	GooglePlaceID      *string           `json:"google_place_id"`
	LeadFormEnabled    bool              `json:"lead_form_enabled"`
	WhatsAppChatURL    *string           `json:"whatsapp_chat_url"`
	PublishedAt        time.Time         `json:"published_at"`
}

// PublicDay is one weekday of the working hours (Monday first).
type PublicDay struct {
	Day     string                    `json:"day"`
	Windows []appointments.WorkWindow `json:"windows"`
}

// PublicService is one published service.
type PublicService struct {
	Kind        string `json:"kind"`
	Title       string `json:"title"`
	Description string `json:"description"`
}

// PublicPhoto is one published gallery photo.
type PublicPhoto struct {
	URL     string `json:"url"`
	Caption string `json:"caption"`
}

var weekdays = []time.Weekday{
	time.Monday, time.Tuesday, time.Wednesday, time.Thursday, time.Friday, time.Saturday, time.Sunday,
}

var e164 = regexp.MustCompile(`^\+[1-9][0-9]{6,14}$`)

// localeChain is the text fallback: requested locale → organization locale
// → tr.
func localeChain(requested, orgLocale string) []string {
	out := make([]string, 0, 3)
	for _, l := range []string{requested, orgLocale, string(i18n.DefaultLocale)} {
		if loc, ok := i18n.Parse(l); ok && !containsStr(out, string(loc)) {
			out = append(out, string(loc))
		}
	}
	return out
}

func containsStr(list []string, v string) bool {
	for _, s := range list {
		if s == v {
			return true
		}
	}
	return false
}

func pick(m map[string]string, chain []string) string {
	for _, l := range chain {
		if v := strings.TrimSpace(m[l]); v != "" {
			return v
		}
	}
	return ""
}

// PublicBlock returns the showcase block of the brand's dealer code in the
// requested locale, or nil when the module is off or nothing was published
// (the response then stays the F2 skeleton).
func (s *Service) PublicBlock(ctx context.Context, brandID int64, code, locale string) (any, error) {
	row, err := s.q.GetPublishedDealerShowcase(ctx, db.GetPublishedDealerShowcaseParams{Slug: code, BrandID: brandID})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if on, err := s.moduleOn(ctx, row.OrganizationID); err != nil || !on {
		return nil, err
	}
	snap, err := parseSnapshot(row.PublishedContent)
	if err != nil || snap == nil {
		return nil, err
	}
	org, err := s.q.GetOrganizationByID(ctx, row.OrganizationID)
	if err != nil {
		return nil, err
	}
	chain := localeChain(locale, row.OrganizationLocale)
	out := PublicShowcase{
		Locale: chain[0], WorkingHours: []PublicDay{}, Services: []PublicService{}, Photos: []PublicPhoto{},
		SocialLinks: map[string]string{}, SeoKeywords: snap.SeoKeywords,
		GoogleRating: numericPtr(row.GoogleRating), GoogleRatingSource: textPtr(row.GoogleRatingSource),
		GooglePlaceID: textPtr(row.GooglePlaceID), PublishedAt: row.PublishedAt.Time.UTC(),
	}
	if out.SeoKeywords == nil {
		out.SeoKeywords = []string{}
	}
	if row.GoogleReviewCount.Valid {
		n := row.GoogleReviewCount.Int32
		out.GoogleReviewCount = &n
	}
	content := map[string]map[string]string{}
	_ = json.Unmarshal(snap.Content, &content)
	headlines, abouts := map[string]string{}, map[string]string{}
	for l, f := range content {
		headlines[l], abouts[l] = f[model.ContentHeadline], f[model.ContentAbout]
	}
	out.Headline, out.About = pick(headlines, chain), pick(abouts, chain)
	_ = json.Unmarshal(snap.SocialLinks, &out.SocialLinks)

	loc, err := time.LoadLocation(org.Timezone)
	if err != nil || org.Timezone == "" {
		loc, _ = time.LoadLocation(i18n.DefaultTimezone)
	}
	out.Timezone = loc.String()
	days, open, err := workingDays(snap.WorkingHours, s.now().In(loc))
	if err != nil {
		return nil, err
	}
	out.WorkingHours, out.OpenNow = days, open

	for _, sv := range snap.Services {
		title := pick(textMap(sv.Title), chain)
		if title == "" {
			title = sv.CategoryName
		}
		out.Services = append(out.Services, PublicService{
			Kind: sv.Kind, Title: title, Description: pick(textMap(sv.Description), chain),
		})
	}
	for _, p := range snap.Photos {
		out.Photos = append(out.Photos, PublicPhoto{
			URL:     fmt.Sprintf("/v1/public/dealers/%s/photos/%s", code, p.UUID),
			Caption: pick(textMap(p.Caption), chain),
		})
	}
	if s.features != nil {
		if out.LeadFormEnabled, err = s.features.Enabled(ctx, org.ID, features.ModuleLeads); err != nil {
			return nil, err
		}
	} else {
		out.LeadFormEnabled = true
	}
	if e164.MatchString(org.Phone) {
		u := "https://wa.me/" + strings.TrimPrefix(org.Phone, "+")
		out.WhatsAppChatURL = &u
	}
	return out, nil
}

// workingDays normalizes the working hours to Monday..Sunday and reports
// whether now (in the organization's zone) falls in a window; no hours at
// all leave open nil.
func workingDays(raw []byte, now time.Time) ([]PublicDay, *bool, error) {
	out := make([]PublicDay, 0, 7)
	any := false
	open := false
	for _, wd := range weekdays {
		windows, err := appointments.ParseWorkingHours(raw, wd)
		if err != nil {
			return nil, nil, err
		}
		if windows == nil {
			windows = []appointments.WorkWindow{}
		}
		any = any || len(windows) > 0
		if wd == now.Weekday() {
			minute := now.Hour()*60 + now.Minute()
			for _, w := range windows {
				start, ok1 := clockMinutes(w.Start)
				end, ok2 := clockMinutes(w.End)
				if ok1 && ok2 && minute >= start && minute < end {
					open = true
				}
			}
		}
		out = append(out, PublicDay{Day: strings.ToLower(wd.String()), Windows: windows})
	}
	if !any {
		return out, nil, nil
	}
	return out, &open, nil
}

func clockMinutes(v string) (int, bool) {
	h, m, ok := strings.Cut(strings.TrimSpace(v), ":")
	if !ok {
		return 0, false
	}
	hh, err1 := strconv.Atoi(h)
	mm, err2 := strconv.Atoi(m)
	if err1 != nil || err2 != nil {
		return 0, false
	}
	return hh*60 + mm, true
}

// PublicPhoto returns the storage key and type of a photo of the dealer's
// live snapshot (ErrNotFound otherwise, also with the module off).
func (s *Service) PublicPhoto(ctx context.Context, brandID int64, code string, id uuid.UUID) (string, string, error) {
	row, err := s.q.GetPublishedDealerShowcase(ctx, db.GetPublishedDealerShowcaseParams{Slug: code, BrandID: brandID})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", ErrNotFound
	}
	if err != nil {
		return "", "", err
	}
	if on, err := s.moduleOn(ctx, row.OrganizationID); err != nil {
		return "", "", err
	} else if !on {
		return "", "", ErrNotFound
	}
	snap, err := parseSnapshot(row.PublishedContent)
	if err != nil {
		return "", "", err
	}
	if snap != nil {
		for _, p := range snap.Photos {
			if p.UUID == id {
				return p.StorageKey, p.Mime, nil
			}
		}
	}
	return "", "", ErrNotFound
}

// Badges reports, for the given organizations of the brand, the ones that
// serve a published showcase with the module on, mapped to their live
// Google rating (nil when unrated). Nearby dealers use it.
func (s *Service) Badges(ctx context.Context, brandID int64, orgUUIDs []uuid.UUID) (map[uuid.UUID]*float64, error) {
	out := map[uuid.UUID]*float64{}
	if len(orgUUIDs) == 0 {
		return out, nil
	}
	rows, err := s.q.ListPublishedDealerShowcaseBadges(ctx, db.ListPublishedDealerShowcaseBadgesParams{
		BrandID: brandID, OrganizationUuids: orgUUIDs,
	})
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		on, err := s.moduleOn(ctx, r.OrganizationID)
		if err != nil {
			return nil, err
		}
		if on {
			out[r.OrganizationUuid] = numericPtr(r.GoogleRating)
		}
	}
	return out, nil
}

// PublishedDates maps the codes of the brand's live showcases (module on)
// to their publish time; the sitemap uses it as the page's last change.
func (s *Service) PublishedDates(ctx context.Context, brandID int64) (map[string]time.Time, error) {
	rows, err := s.q.ListPublishedDealerShowcaseDates(ctx, brandID)
	if err != nil {
		return nil, err
	}
	out := make(map[string]time.Time, len(rows))
	for _, r := range rows {
		on, err := s.moduleOn(ctx, r.OrganizationID)
		if err != nil {
			return nil, err
		}
		if on && r.PublishedAt.Valid {
			out[r.Slug] = r.PublishedAt.Time.UTC()
		}
	}
	return out, nil
}
