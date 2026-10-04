package db_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// TEC-350 (F3-09a): database-level guards of review questions and answers
// (migration 000091). Reuses the service fixture (rolled-back transaction,
// savepoint per failure).

func (f *serviceFixture) reviewQuestion(t *testing.T, key, typ, target string) db.ReviewQuestion {
	t.Helper()
	q, err := f.q.CreateReviewQuestion(f.ctx, db.CreateReviewQuestionParams{
		BrandID: f.brandID, QuestionKey: fmt.Sprintf("%s_%d", key, time.Now().UnixNano()),
		QuestionType: typ, Target: target, IsActive: true,
	})
	if err != nil {
		t.Fatalf("question %s: %v", key, err)
	}
	return q
}

func int2(v int16) pgtype.Int2  { return pgtype.Int2{Int16: v, Valid: true} }
func int8v(v int64) pgtype.Int8 { return pgtype.Int8{Int64: v, Valid: true} }

func TestReviewQuestionsSchema(t *testing.T) {
	f := newServiceFixture(t)
	ctx := f.ctx

	svc := f.service(t, f.dealer)
	rv, err := f.q.CreateServiceReview(ctx, db.CreateServiceReviewParams{
		OrganizationID: svc.OrganizationID, BrandID: svc.BrandID, ServiceID: svc.ID,
		CustomerUserID: f.customer.ID, PlatformRating: 5, ProductRating: 4,
	})
	if err != nil {
		t.Fatalf("review: %v", err)
	}
	if rv.IsAnonymous || rv.Source != "portal" || rv.ProcessedAt.Valid {
		t.Fatalf("review defaults = %+v", rv)
	}

	rating := f.reviewQuestion(t, "t350_rating", "rating_1_5", "dealer")
	textQ := f.reviewQuestion(t, "t350_text", "text", "platform")
	productQ := f.reviewQuestion(t, "t350_product", "rating_1_5", "product")

	answer := func(q db.ReviewQuestion) db.CreateServiceReviewAnswerParams {
		return db.CreateServiceReviewAnswerParams{
			ReviewID: rv.ID, OrganizationID: rv.OrganizationID, BrandID: rv.BrandID, QuestionID: q.ID,
		}
	}
	insert := func(arg db.CreateServiceReviewAnswerParams) func(sp pgx.Tx) error {
		return func(sp pgx.Tx) error {
			_, err := db.New(sp).CreateServiceReviewAnswer(ctx, arg)
			return err
		}
	}

	t.Run("rating outside 1-5 rejected", func(t *testing.T) {
		for _, v := range []int16{0, 6} {
			arg := answer(rating)
			arg.Rating = int2(v)
			f.expectConstraint(t, fmt.Sprintf("rating %d", v), "23514", "chk_service_review_answers_rating", insert(arg))
		}
	})

	t.Run("rating question needs rating and no text", func(t *testing.T) {
		f.expectTrigger(t, "no rating", insert(answer(rating)))
		arg := answer(rating)
		arg.Rating, arg.Text = int2(4), text("and a text")
		f.expectTrigger(t, "rating with text", insert(arg))
	})

	t.Run("text question rejects a rating", func(t *testing.T) {
		arg := answer(textQ)
		arg.Rating = int2(3)
		f.expectTrigger(t, "rating on text question", insert(arg))
		arg.Text = text("good")
		f.expectTrigger(t, "rating and text on text question", insert(arg))
		f.expectTrigger(t, "empty text answer", insert(answer(textQ)))
	})

	t.Run("product question needs product_id", func(t *testing.T) {
		arg := answer(productQ)
		arg.Rating = int2(5)
		f.expectTrigger(t, "no product", insert(arg))
		other := answer(rating)
		other.Rating, other.ProductID = int2(5), int8v(f.film.ID)
		f.expectTrigger(t, "product on dealer question", insert(other))
	})

	t.Run("question of another brand rejected", func(t *testing.T) {
		var other int64
		if err := f.tx.QueryRow(ctx, `SELECT id FROM brands WHERE id <> $1 ORDER BY id LIMIT 1`, f.brandID).Scan(&other); err != nil {
			t.Skipf("no second brand: %v", err)
		}
		q, err := f.q.CreateReviewQuestion(ctx, db.CreateReviewQuestionParams{
			BrandID: other, QuestionKey: fmt.Sprintf("t350_other_%d", time.Now().UnixNano()),
			QuestionType: "rating_1_5", Target: "dealer", IsActive: true,
		})
		if err != nil {
			t.Fatalf("other brand question: %v", err)
		}
		arg := answer(q)
		arg.Rating = int2(5)
		f.expectTrigger(t, "foreign question", insert(arg))
	})

	t.Run("one answer per review, question and product", func(t *testing.T) {
		arg := answer(rating)
		arg.Rating = int2(4)
		if _, err := f.q.CreateServiceReviewAnswer(ctx, arg); err != nil {
			t.Fatalf("first answer: %v", err)
		}
		// NULL product_id duplicates are rejected too (NULLS NOT DISTINCT).
		arg.Rating = int2(2)
		f.expectConstraint(t, "duplicate null product", "23505", "uq_service_review_answers", insert(arg))

		p := answer(productQ)
		p.Rating, p.ProductID = int2(5), int8v(f.film.ID)
		if _, err := f.q.CreateServiceReviewAnswer(ctx, p); err != nil {
			t.Fatalf("film answer: %v", err)
		}
		f.expectConstraint(t, "duplicate product", "23505", "uq_service_review_answers", insert(p))
		p.ProductID = int8v(f.piece.ID)
		if _, err := f.q.CreateServiceReviewAnswer(ctx, p); err != nil {
			t.Fatalf("second product answer: %v", err)
		}

		got, err := f.q.ListServiceReviewAnswersByReview(ctx, rv.ID)
		if err != nil || len(got) != 3 {
			t.Fatalf("answers by review = %d, %v", len(got), err)
		}
		byQ, err := f.q.ListServiceReviewAnswersByQuestion(ctx, db.ListServiceReviewAnswersByQuestionParams{
			QuestionID: productQ.ID, BrandID: f.brandID, PageLimit: 10,
		})
		if err != nil || len(byQ) != 2 {
			t.Fatalf("answers by question = %d, %v", len(byQ), err)
		}
		scoped, err := f.q.CountServiceReviewAnswersByQuestion(ctx, db.CountServiceReviewAnswersByQuestionParams{
			QuestionID: productQ.ID, BrandID: f.brandID, OrganizationIds: []int64{f.dist.ID},
		})
		if err != nil || scoped != 0 {
			t.Fatalf("answers outside scope = %d, %v", scoped, err)
		}
	})

	t.Run("locales, flags and processing", func(t *testing.T) {
		if _, err := f.q.UpsertReviewQuestionLocale(ctx, db.UpsertReviewQuestionLocaleParams{
			QuestionID: rating.ID, Locale: "tr", Text: "Bayi",
		}); err != nil {
			t.Fatalf("locale: %v", err)
		}
		loc, err := f.q.UpsertReviewQuestionLocale(ctx, db.UpsertReviewQuestionLocaleParams{
			QuestionID: rating.ID, Locale: "tr", Text: "Bayi hizmeti",
		})
		if err != nil || loc.Text != "Bayi hizmeti" {
			t.Fatalf("locale upsert = %+v, %v", loc, err)
		}
		f.expectConstraint(t, "unknown locale", "23514", "chk_review_question_locales_locale", func(sp pgx.Tx) error {
			_, err := db.New(sp).UpsertReviewQuestionLocale(ctx, db.UpsertReviewQuestionLocaleParams{
				QuestionID: rating.ID, Locale: "xx", Text: "x",
			})
			return err
		})

		flagged, err := f.q.SetServiceReviewFlags(ctx, db.SetServiceReviewFlagsParams{
			ID: rv.ID, IsAnonymous: true, Source: "whatsapp_link",
		})
		if err != nil || !flagged.IsAnonymous || flagged.Source != "whatsapp_link" {
			t.Fatalf("flags = %+v, %v", flagged, err)
		}
		f.expectConstraint(t, "bad source", "23514", "chk_service_reviews_source", func(sp pgx.Tx) error {
			_, err := db.New(sp).SetServiceReviewFlags(ctx, db.SetServiceReviewFlagsParams{ID: rv.ID, Source: "email"})
			return err
		})

		pending, err := f.q.ListUnprocessedServiceReviews(ctx, 1000)
		if err != nil || !containsReview(pending, rv.ID) {
			t.Fatalf("unprocessed before = %v", err)
		}
		if _, err := f.q.MarkServiceReviewProcessed(ctx, rv.ID); err != nil {
			t.Fatalf("mark processed: %v", err)
		}
		if _, err := f.q.MarkServiceReviewProcessed(ctx, rv.ID); err != pgx.ErrNoRows {
			t.Fatalf("second mark = %v, want ErrNoRows", err)
		}
		pending, err = f.q.ListUnprocessedServiceReviews(ctx, 1000)
		if err != nil || containsReview(pending, rv.ID) {
			t.Fatalf("unprocessed after = %v", err)
		}
	})
}

func containsReview(rows []db.ServiceReview, id int64) bool {
	for _, r := range rows {
		if r.ID == id {
			return true
		}
	}
	return false
}
