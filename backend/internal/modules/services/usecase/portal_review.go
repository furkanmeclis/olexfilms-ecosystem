package usecase

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// TEC-244 (F2-03h): the portal service review form.
//
// The portal user who owns a service (portalService: its customer or a
// holder of one of its warranties) rates the platform and the product
// quality (1-5) with an optional comment, once per service. Anything that
// is not theirs is ErrNotFound; a second review is ErrAlreadyReviewed
// (409); a service that is not completed yet is ErrReviewNotCompleted
// (422). The dealer's google_business_url is returned with the review so
// the portal can show the "review us on Google" button. Processing and
// reporting of the answers come with F5.

// MaxReviewCommentLength is the limit of the review comment (characters).
const MaxReviewCommentLength = 2000

var (
	// ErrAlreadyReviewed: the service already has a review (409).
	ErrAlreadyReviewed = errors.New("services: service already reviewed")
	// ErrReviewNotCompleted: only a completed service can be reviewed (422).
	ErrReviewNotCompleted = errors.New("services: service is not completed")
)

// ServiceReviewInput is the body of POST /v1/portal/services/{uuid}/review.
// The ratings are pointers so a missing one is a validation error.
type ServiceReviewInput struct {
	PlatformRating *int                `json:"platform_rating"`
	ProductRating  *int                `json:"product_rating"`
	Comment        *string             `json:"comment"`
	IsAnonymous    *bool               `json:"is_anonymous"`
	Source         *string             `json:"source"`
	Answers        []ReviewAnswerInput `json:"answers"`
}

// ServiceReviewView is a stored review.
type ServiceReviewView struct {
	UUID           uuid.UUID          `json:"uuid"`
	PlatformRating int                `json:"platform_rating"`
	ProductRating  int                `json:"product_rating"`
	Comment        *string            `json:"comment"`
	IsAnonymous    bool               `json:"is_anonymous"`
	Source         string             `json:"source"`
	Answers        []ReviewAnswerView `json:"answers"`
	CreatedAt      time.Time          `json:"created_at"`
}

type ReviewProductView struct {
	UUID uuid.UUID `json:"uuid"`
	SKU  string    `json:"sku"`
	Name string    `json:"name"`
}

type ReviewFormQuestionView struct {
	UUID         uuid.UUID `json:"uuid"`
	QuestionKey  string    `json:"question_key"`
	QuestionType string    `json:"question_type"`
	Target       string    `json:"target"`
	IsRequired   bool      `json:"is_required"`
	SortOrder    int32     `json:"sort_order"`
	Text         string    `json:"text"`
}

type ReviewAnswerInput struct {
	QuestionUUID uuid.UUID  `json:"question_uuid"`
	ProductUUID  *uuid.UUID `json:"product_uuid"`
	Rating       *int       `json:"rating"`
	Text         *string    `json:"text"`
}

type ReviewAnswerView struct {
	QuestionUUID uuid.UUID  `json:"question_uuid"`
	ProductUUID  *uuid.UUID `json:"product_uuid"`
	Rating       *int       `json:"rating"`
	Text         *string    `json:"text"`
}

// PortalServiceReview is the review state of a service for the portal:
// the stored review (nil when none), whether the user can send one now and
// the dealer's Google review link (nil when the dealer has none).
type PortalServiceReview struct {
	Review            *ServiceReviewView       `json:"review"`
	CanReview         bool                     `json:"can_review"`
	GoogleBusinessURL *string                  `json:"google_business_url"`
	Questions         []ReviewFormQuestionView `json:"questions"`
	Products          []ReviewProductView      `json:"products"`
}

// normalizeReview validates the input (pure): both ratings 1-5, the
// comment trimmed, empty means none, at most MaxReviewCommentLength.
func normalizeReview(in ServiceReviewInput) (platform, product int16, comment pgtype.Text, err error) {
	rating := func(field string, v *int) (int16, error) {
		if v == nil {
			return 0, invalid(field, "is required")
		}
		if *v < 1 || *v > 5 {
			return 0, invalid(field, "must be between 1 and 5")
		}
		return int16(*v), nil
	}
	if platform, err = rating("platform_rating", in.PlatformRating); err != nil {
		return 0, 0, pgtype.Text{}, err
	}
	if product, err = rating("product_rating", in.ProductRating); err != nil {
		return 0, 0, pgtype.Text{}, err
	}
	if in.Comment != nil {
		c := strings.TrimSpace(*in.Comment)
		if utf8.RuneCountInString(c) > MaxReviewCommentLength {
			return 0, 0, pgtype.Text{}, invalid("comment", fmt.Sprintf("at most %d characters", MaxReviewCommentLength))
		}
		if c != "" {
			comment = pgtype.Text{String: c, Valid: true}
		}
	}
	return platform, product, comment, nil
}

func reviewView(r db.ServiceReview, answers []ReviewAnswerView) *ServiceReviewView {
	return &ServiceReviewView{
		UUID: r.Uuid, PlatformRating: int(r.PlatformRating), ProductRating: int(r.ProductRating),
		Comment: textPtr(r.Comment), IsAnonymous: r.IsAnonymous, Source: r.Source,
		Answers: answers, CreatedAt: r.CreatedAt.Time,
	}
}

// googleURL is the dealer's review link (nil when unset).
func googleURL(org db.Organization) *string {
	if !org.GoogleBusinessUrl.Valid {
		return nil
	}
	u := strings.TrimSpace(org.GoogleBusinessUrl.String)
	if u == "" {
		return nil
	}
	return &u
}

// PortalGetReview returns the review state of a service the user owns.
func (s *Service) PortalGetReview(ctx context.Context, brandID, userID int64, id uuid.UUID) (PortalServiceReview, error) {
	svc, _, err := s.portalService(ctx, brandID, userID, id)
	if err != nil {
		return PortalServiceReview{}, err
	}
	org, err := s.q.GetOrganizationByID(ctx, svc.OrganizationID)
	if err != nil {
		return PortalServiceReview{}, fmt.Errorf("services: review organization: %w", err)
	}
	questions, products, err := s.reviewForm(ctx, svc.BrandID, svc.ID, userLocale(ctx))
	if err != nil {
		return PortalServiceReview{}, err
	}
	out := PortalServiceReview{GoogleBusinessURL: googleURL(org), Questions: questions, Products: products}
	r, err := s.q.GetServiceReviewByService(ctx, svc.ID)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		out.CanReview = svc.Status == StatusCompleted
	case err != nil:
		return PortalServiceReview{}, fmt.Errorf("services: get review: %w", err)
	default:
		answers, err := s.reviewAnswers(ctx, svc.BrandID, r.ID)
		if err != nil {
			return PortalServiceReview{}, err
		}
		out.Review = reviewView(r, answers)
	}
	return out, nil
}

// PortalCreateReview stores the review of a completed service the user
// owns and returns the new review state.
func (s *Service) PortalCreateReview(ctx context.Context, brandID, userID int64, id uuid.UUID,
	in ServiceReviewInput,
) (PortalServiceReview, error) {
	svc, _, err := s.portalService(ctx, brandID, userID, id)
	if err != nil {
		return PortalServiceReview{}, err
	}
	platform, product, comment, err := normalizeReview(in)
	if err != nil {
		return PortalServiceReview{}, err
	}
	if svc.Status != StatusCompleted {
		return PortalServiceReview{}, ErrReviewNotCompleted
	}
	org, err := s.q.GetOrganizationByID(ctx, svc.OrganizationID)
	if err != nil {
		return PortalServiceReview{}, fmt.Errorf("services: review organization: %w", err)
	}
	source := normalizeReviewSource(in.Source)
	isAnonymous := in.IsAnonymous != nil && *in.IsAnonymous
	questions, products, err := s.reviewForm(ctx, svc.BrandID, svc.ID, userLocale(ctx))
	if err != nil {
		return PortalServiceReview{}, err
	}
	productIDs, err := s.reviewProductIDs(ctx, svc.ID)
	if err != nil {
		return PortalServiceReview{}, err
	}
	if err := validateReviewAnswers(in.Answers, questions, productIDs); err != nil {
		return PortalServiceReview{}, err
	}
	var review db.ServiceReview
	if err := s.inTx(ctx, func(q *db.Queries, tx pgx.Tx) error {
		var err error
		review, err = q.CreateServiceReview(ctx, db.CreateServiceReviewParams{
			OrganizationID: svc.OrganizationID, BrandID: svc.BrandID, ServiceID: svc.ID,
			CustomerUserID: userID, PlatformRating: platform, ProductRating: product, Comment: comment,
			IsAnonymous: isAnonymous, Source: source,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrAlreadyReviewed
		}
		if err != nil {
			return fmt.Errorf("services: create review: %w", err)
		}
		if err := s.createReviewAnswers(ctx, q, review, in.Answers, questions, productIDs); err != nil {
			return err
		}
		if s.out != nil {
			id, u := review.ID, review.Uuid
			ev := events.New(events.ServiceReviewed).
				WithTenant(review.OrganizationID).
				WithEntity("service_review", &id, &u).
				WithPayload(map[string]any{
					"review_id":       review.ID,
					"review_uuid":     review.Uuid.String(),
					"service_id":      svc.ID,
					"service_uuid":    svc.Uuid.String(),
					"organization_id": review.OrganizationID,
					"brand_id":        review.BrandID,
					"is_anonymous":    review.IsAnonymous,
					"source":          review.Source,
				})
			if err := s.out.Enqueue(ctx, tx, ev); err != nil {
				return fmt.Errorf("services: review outbox: %w", err)
			}
		}
		return nil
	}); err != nil {
		return PortalServiceReview{}, err
	}
	answers, err := s.reviewAnswers(ctx, svc.BrandID, review.ID)
	if err != nil {
		return PortalServiceReview{}, err
	}
	return PortalServiceReview{
		Review: reviewView(review, answers), GoogleBusinessURL: googleURL(org),
		Questions: questions, Products: products,
	}, nil
}

func normalizeReviewSource(source *string) string {
	if source == nil {
		return "portal"
	}
	switch strings.TrimSpace(*source) {
	case "whatsapp_link":
		return "whatsapp_link"
	default:
		return "portal"
	}
}

func userLocale(ctx context.Context) string { return string(i18n.FromContext(ctx).Locale) }

func (s *Service) reviewForm(ctx context.Context, brandID, serviceID int64, locale string) ([]ReviewFormQuestionView, []ReviewProductView, error) {
	rows, err := s.q.ListReviewQuestionsByBrand(ctx, db.ListReviewQuestionsByBrandParams{BrandID: brandID, ActiveOnly: true})
	if err != nil {
		return nil, nil, fmt.Errorf("services: review questions: %w", err)
	}
	ids := make([]int64, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ID)
	}
	texts := map[int64]map[string]string{}
	if len(ids) > 0 {
		locs, err := s.q.ListReviewQuestionLocalesByQuestions(ctx, ids)
		if err != nil {
			return nil, nil, fmt.Errorf("services: review question locales: %w", err)
		}
		for _, loc := range locs {
			if texts[loc.QuestionID] == nil {
				texts[loc.QuestionID] = map[string]string{}
			}
			texts[loc.QuestionID][loc.Locale] = loc.Text
		}
	}
	questions := make([]ReviewFormQuestionView, 0, len(rows))
	for _, row := range rows {
		text := texts[row.ID][normalizeReviewLocale(locale)]
		if text == "" {
			text = texts[row.ID]["tr"]
		}
		questions = append(questions, ReviewFormQuestionView{
			UUID: row.Uuid, QuestionKey: row.QuestionKey, QuestionType: row.QuestionType,
			Target: row.Target, IsRequired: row.IsRequired, SortOrder: row.SortOrder, Text: text,
		})
	}
	productRows, err := s.q.ListServiceReviewProducts(ctx, serviceID)
	if err != nil {
		return nil, nil, fmt.Errorf("services: review products: %w", err)
	}
	products := make([]ReviewProductView, 0, len(productRows))
	for _, p := range productRows {
		products = append(products, ReviewProductView{UUID: p.Uuid, SKU: p.Sku, Name: p.Name})
	}
	return questions, products, nil
}

func (s *Service) reviewProductIDs(ctx context.Context, serviceID int64) (map[uuid.UUID]int64, error) {
	rows, err := s.q.ListServiceReviewProducts(ctx, serviceID)
	if err != nil {
		return nil, fmt.Errorf("services: review products: %w", err)
	}
	out := make(map[uuid.UUID]int64, len(rows))
	for _, row := range rows {
		out[row.Uuid] = row.ID
	}
	return out, nil
}

func validateReviewAnswers(answers []ReviewAnswerInput, questions []ReviewFormQuestionView, products map[uuid.UUID]int64) error {
	byUUID := make(map[uuid.UUID]ReviewFormQuestionView, len(questions))
	seen := map[string]bool{}
	for _, q := range questions {
		byUUID[q.UUID] = q
	}
	for i, answer := range answers {
		q, ok := byUUID[answer.QuestionUUID]
		if !ok {
			return invalid(fmt.Sprintf("answers[%d].question_uuid", i), "unknown question")
		}
		key := answer.QuestionUUID.String()
		if q.Target == ReviewQuestionProduct {
			if answer.ProductUUID == nil {
				return invalid(fmt.Sprintf("answers[%d].product_uuid", i), "is required")
			}
			if _, ok := products[*answer.ProductUUID]; !ok {
				return invalid(fmt.Sprintf("answers[%d].product_uuid", i), "is not in this service")
			}
			key += ":" + answer.ProductUUID.String()
		} else if answer.ProductUUID != nil {
			return invalid(fmt.Sprintf("answers[%d].product_uuid", i), "is only allowed for product questions")
		}
		if seen[key] {
			return invalid(fmt.Sprintf("answers[%d]", i), "duplicate answer")
		}
		seen[key] = true
		if q.QuestionType == ReviewQuestionRating1To5 {
			if answer.Rating == nil || *answer.Rating < 1 || *answer.Rating > 5 || answer.Text != nil {
				return invalid(fmt.Sprintf("answers[%d].rating", i), "must be between 1 and 5")
			}
			continue
		}
		if answer.Text == nil || strings.TrimSpace(*answer.Text) == "" || answer.Rating != nil {
			return invalid(fmt.Sprintf("answers[%d].text", i), "is required")
		}
	}
	for _, q := range questions {
		if !q.IsRequired {
			continue
		}
		if q.Target != ReviewQuestionProduct {
			if !seen[q.UUID.String()] {
				return invalid("answers", "required question is missing")
			}
			continue
		}
		for productUUID := range products {
			if !seen[q.UUID.String()+":"+productUUID.String()] {
				return invalid("answers", "required product question is missing")
			}
		}
	}
	return nil
}

func (s *Service) createReviewAnswers(
	ctx context.Context,
	q *db.Queries,
	review db.ServiceReview,
	answers []ReviewAnswerInput,
	questions []ReviewFormQuestionView,
	products map[uuid.UUID]int64,
) error {
	questionRows, err := q.ListReviewQuestionsByBrand(ctx, db.ListReviewQuestionsByBrandParams{
		BrandID: review.BrandID, ActiveOnly: true,
	})
	if err != nil {
		return fmt.Errorf("services: review questions: %w", err)
	}
	questionIDs := make(map[uuid.UUID]int64, len(questionRows))
	for _, row := range questionRows {
		questionIDs[row.Uuid] = row.ID
	}
	for _, answer := range answers {
		arg := db.CreateServiceReviewAnswerParams{
			ReviewID: review.ID, OrganizationID: review.OrganizationID, BrandID: review.BrandID,
			QuestionID: questionIDs[answer.QuestionUUID],
		}
		if answer.ProductUUID != nil {
			arg.ProductID = pgtype.Int8{Int64: products[*answer.ProductUUID], Valid: true}
		}
		if answer.Rating != nil {
			arg.Rating = pgtype.Int2{Int16: int16(*answer.Rating), Valid: true}
		}
		if answer.Text != nil {
			arg.Text = pgtype.Text{String: strings.TrimSpace(*answer.Text), Valid: true}
		}
		if _, err := q.CreateServiceReviewAnswer(ctx, arg); err != nil {
			return fmt.Errorf("services: create review answer: %w", err)
		}
	}
	_ = questions
	return nil
}

func (s *Service) reviewAnswers(ctx context.Context, brandID, reviewID int64) ([]ReviewAnswerView, error) {
	rows, err := s.q.ListServiceReviewAnswersByReview(ctx, reviewID)
	if err != nil {
		return nil, fmt.Errorf("services: review answers: %w", err)
	}
	out := make([]ReviewAnswerView, 0, len(rows))
	for _, row := range rows {
		q, err := s.q.GetReviewQuestionByID(ctx, db.GetReviewQuestionByIDParams{ID: row.QuestionID, BrandID: brandID})
		if err != nil {
			return nil, fmt.Errorf("services: review answer question: %w", err)
		}
		a := ReviewAnswerView{QuestionUUID: q.Uuid, Text: textPtr(row.Text)}
		if row.ProductID.Valid {
			p, err := s.q.GetProduct(ctx, db.GetProductParams{ID: row.ProductID.Int64, BrandID: brandID})
			if err != nil {
				return nil, fmt.Errorf("services: review answer product: %w", err)
			}
			a.ProductUUID = &p.Uuid
		}
		if row.Rating.Valid {
			r := int(row.Rating.Int16)
			a.Rating = &r
		}
		out = append(out, a)
	}
	return out, nil
}

func ReviewFormTarget(serviceUUID uuid.UUID) string {
	v := url.Values{}
	v.Set("source", "whatsapp_link")
	return "/portal/services/" + serviceUUID.String() + "/review?" + v.Encode()
}
