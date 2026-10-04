package usecase

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/google/uuid"
)

const (
	ReviewQuestionRating1To5 = "rating_1_5"
	ReviewQuestionText       = "text"

	ReviewQuestionPlatform = "platform"
	ReviewQuestionDealer   = "dealer"
	ReviewQuestionProduct  = "product"

	MaxReviewQuestionTextLength = 500
)

var (
	ErrReviewQuestionLocked = errors.New("services: review question has answers")
)

type ReviewQuestionInput struct {
	QuestionKey  *string `json:"question_key"`
	QuestionType *string `json:"question_type"`
	Target       *string `json:"target"`
	IsRequired   *bool   `json:"is_required"`
	IsActive     *bool   `json:"is_active"`
	SortOrder    *int32  `json:"sort_order"`
}

type ReviewQuestionLocaleInput struct {
	Text string `json:"text"`
}

type ReviewQuestionLocaleView struct {
	Locale    string    `json:"locale"`
	Text      string    `json:"text"`
	UpdatedAt time.Time `json:"updated_at"`
}

type ReviewQuestionView struct {
	UUID         uuid.UUID                  `json:"uuid"`
	QuestionKey  string                     `json:"question_key"`
	QuestionType string                     `json:"question_type"`
	Target       string                     `json:"target"`
	IsRequired   bool                       `json:"is_required"`
	IsActive     bool                       `json:"is_active"`
	SortOrder    int32                      `json:"sort_order"`
	Locales      []ReviewQuestionLocaleView `json:"locales"`
	CreatedAt    time.Time                  `json:"created_at"`
	UpdatedAt    time.Time                  `json:"updated_at"`
}

func reviewQuestionView(q db.ReviewQuestion, locales []db.ReviewQuestionLocale) ReviewQuestionView {
	out := ReviewQuestionView{
		UUID:         q.Uuid,
		QuestionKey:  q.QuestionKey,
		QuestionType: q.QuestionType,
		Target:       q.Target,
		IsRequired:   q.IsRequired,
		IsActive:     q.IsActive,
		SortOrder:    q.SortOrder,
		Locales:      []ReviewQuestionLocaleView{},
		CreatedAt:    q.CreatedAt.Time,
		UpdatedAt:    q.UpdatedAt.Time,
	}
	for _, loc := range locales {
		out.Locales = append(out.Locales, ReviewQuestionLocaleView{
			Locale: loc.Locale, Text: loc.Text, UpdatedAt: loc.UpdatedAt.Time,
		})
	}
	return out
}

func normalizeReviewQuestion(in ReviewQuestionInput, current *db.ReviewQuestion) (db.CreateReviewQuestionParams, error) {
	var out db.CreateReviewQuestionParams
	if current != nil {
		out.QuestionKey = current.QuestionKey
		out.QuestionType = current.QuestionType
		out.Target = current.Target
		out.IsRequired = current.IsRequired
		out.IsActive = current.IsActive
		out.SortOrder = current.SortOrder
	}
	if in.QuestionKey != nil {
		key := strings.TrimSpace(*in.QuestionKey)
		if key == "" || utf8.RuneCountInString(key) > 64 {
			return out, invalid("question_key", "is required and must be at most 64 characters")
		}
		out.QuestionKey = key
	}
	if in.QuestionType != nil {
		typ := strings.TrimSpace(*in.QuestionType)
		if typ != ReviewQuestionRating1To5 && typ != ReviewQuestionText {
			return out, invalid("question_type", "must be rating_1_5 or text")
		}
		out.QuestionType = typ
	}
	if in.Target != nil {
		target := strings.TrimSpace(*in.Target)
		switch target {
		case ReviewQuestionPlatform, ReviewQuestionDealer, ReviewQuestionProduct:
			out.Target = target
		default:
			return out, invalid("target", "must be platform, dealer or product")
		}
	}
	if in.IsRequired != nil {
		out.IsRequired = *in.IsRequired
	}
	if in.IsActive != nil {
		out.IsActive = *in.IsActive
	}
	if in.SortOrder != nil {
		out.SortOrder = *in.SortOrder
	}
	if out.QuestionKey == "" {
		return out, invalid("question_key", "is required")
	}
	if out.QuestionType == "" {
		return out, invalid("question_type", "is required")
	}
	if out.Target == "" {
		return out, invalid("target", "is required")
	}
	return out, nil
}

func (s *Service) ListReviewQuestions(ctx context.Context, c Caller, activeOnly bool) ([]ReviewQuestionView, error) {
	rows, err := s.q.ListReviewQuestionsByBrand(ctx, db.ListReviewQuestionsByBrandParams{
		BrandID: c.Org.BrandID, ActiveOnly: activeOnly,
	})
	if err != nil {
		return nil, fmt.Errorf("services: list review questions: %w", err)
	}
	ids := make([]int64, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ID)
	}
	locs := map[int64][]db.ReviewQuestionLocale{}
	if len(ids) > 0 {
		all, err := s.q.ListReviewQuestionLocalesByQuestions(ctx, ids)
		if err != nil {
			return nil, fmt.Errorf("services: list review question locales: %w", err)
		}
		for _, loc := range all {
			locs[loc.QuestionID] = append(locs[loc.QuestionID], loc)
		}
	}
	out := make([]ReviewQuestionView, 0, len(rows))
	for _, row := range rows {
		out = append(out, reviewQuestionView(row, locs[row.ID]))
	}
	return out, nil
}

func (s *Service) CreateReviewQuestion(ctx context.Context, c Caller, in ReviewQuestionInput) (ReviewQuestionView, error) {
	n, err := normalizeReviewQuestion(in, nil)
	if err != nil {
		return ReviewQuestionView{}, err
	}
	row, err := s.q.CreateReviewQuestion(ctx, db.CreateReviewQuestionParams{
		BrandID: c.Org.BrandID, QuestionKey: n.QuestionKey, QuestionType: n.QuestionType,
		Target: n.Target, IsRequired: n.IsRequired, IsActive: n.IsActive, SortOrder: n.SortOrder,
	})
	if err != nil {
		return ReviewQuestionView{}, fmt.Errorf("services: create review question: %w", err)
	}
	return reviewQuestionView(row, nil), nil
}

func (s *Service) UpdateReviewQuestion(ctx context.Context, c Caller, id uuid.UUID, in ReviewQuestionInput) (ReviewQuestionView, error) {
	current, err := s.q.GetReviewQuestionByUUID(ctx, db.GetReviewQuestionByUUIDParams{Uuid: id, BrandID: c.Org.BrandID})
	if err != nil {
		return ReviewQuestionView{}, ErrNotFound
	}
	n, err := normalizeReviewQuestion(in, &current)
	if err != nil {
		return ReviewQuestionView{}, err
	}
	if n.QuestionType != current.QuestionType || n.Target != current.Target {
		has, err := s.q.ReviewQuestionHasAnswers(ctx, current.ID)
		if err != nil {
			return ReviewQuestionView{}, fmt.Errorf("services: review question answers: %w", err)
		}
		if has {
			return ReviewQuestionView{}, ErrReviewQuestionLocked
		}
	}
	var row db.ReviewQuestion
	if in.IsActive != nil && n.QuestionKey == current.QuestionKey && n.QuestionType == current.QuestionType &&
		n.Target == current.Target && n.IsRequired == current.IsRequired && n.SortOrder == current.SortOrder {
		row, err = s.q.SetReviewQuestionActive(ctx, db.SetReviewQuestionActiveParams{
			ID: current.ID, BrandID: c.Org.BrandID, IsActive: n.IsActive,
		})
	} else {
		row, err = s.q.UpdateReviewQuestion(ctx, db.UpdateReviewQuestionParams{
			ID: current.ID, BrandID: c.Org.BrandID, QuestionKey: n.QuestionKey,
			QuestionType: n.QuestionType, Target: n.Target, IsRequired: n.IsRequired, SortOrder: n.SortOrder,
		})
		if err == nil && n.IsActive != current.IsActive {
			row, err = s.q.SetReviewQuestionActive(ctx, db.SetReviewQuestionActiveParams{
				ID: current.ID, BrandID: c.Org.BrandID, IsActive: n.IsActive,
			})
		}
	}
	if err != nil {
		return ReviewQuestionView{}, err
	}
	locs, err := s.q.ListReviewQuestionLocales(ctx, row.ID)
	if err != nil {
		return ReviewQuestionView{}, fmt.Errorf("services: review question locales: %w", err)
	}
	return reviewQuestionView(row, locs), nil
}

func (s *Service) PutReviewQuestionLocale(
	ctx context.Context,
	c Caller,
	id uuid.UUID,
	locale string,
	in ReviewQuestionLocaleInput,
) (ReviewQuestionLocaleView, error) {
	q, err := s.q.GetReviewQuestionByUUID(ctx, db.GetReviewQuestionByUUIDParams{Uuid: id, BrandID: c.Org.BrandID})
	if err != nil {
		return ReviewQuestionLocaleView{}, ErrNotFound
	}
	loc := normalizeReviewLocale(locale)
	if loc == "" {
		return ReviewQuestionLocaleView{}, invalid("locale", "unsupported locale")
	}
	text := strings.TrimSpace(in.Text)
	if text == "" || utf8.RuneCountInString(text) > MaxReviewQuestionTextLength {
		return ReviewQuestionLocaleView{}, invalid("text", fmt.Sprintf("is required and must be at most %d characters", MaxReviewQuestionTextLength))
	}
	row, err := s.q.UpsertReviewQuestionLocale(ctx, db.UpsertReviewQuestionLocaleParams{
		QuestionID: q.ID, Locale: loc, Text: text,
	})
	if err != nil {
		return ReviewQuestionLocaleView{}, fmt.Errorf("services: upsert review question locale: %w", err)
	}
	return ReviewQuestionLocaleView{Locale: row.Locale, Text: row.Text, UpdatedAt: row.UpdatedAt.Time}, nil
}

func normalizeReviewLocale(locale string) string {
	loc := strings.ReplaceAll(strings.TrimSpace(locale), "_", "-")
	if loc == "zh-CN" {
		return loc
	}
	loc = strings.ToLower(loc)
	if loc == "zh-cn" {
		return "zh-CN"
	}
	if slices.Contains([]string{"tr", "en", "bg", "de", "el", "uk", "ru", "fr", "es", "it", "az", "ar"}, loc) {
		return loc
	}
	return ""
}

func reviewScopeOrgIDs(c Caller, slug string) []int64 {
	scope, ok := c.Principal.ScopeFor(slug)
	if !ok {
		return []int64{}
	}
	switch scope {
	case rbac.ScopeAll, rbac.ScopeBrand:
		return nil
	default:
		return c.Filter.OrgIDsArg()
	}
}
