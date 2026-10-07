"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ExternalLink, Star } from "lucide-react";
import {
  type FormEvent,
  type ReactNode,
  useEffect,
  useId,
  useMemo,
  useRef,
  useState,
} from "react";

import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Checkbox } from "@/components/ui/checkbox";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { PortalApiError } from "@/features/portal/lib/portal-client";
import {
  missingRequired,
  reviewAnswers,
  reviewFormRows,
  type ReviewFormRow,
  type ReviewFormValues,
} from "@/features/portal/lib/review-form";
import { usePortalReadOnly } from "@/features/portal/lib/use-portal-read-only";
import {
  REVIEW_COMMENT_MAX,
  googleReviewUrl,
  portalServiceReviewApi,
  type PortalServiceReviewState,
} from "@/features/portal/services/portal-service-review.service";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

const RATINGS = [1, 2, 3, 4, 5] as const;

/** 1-5 star picker (radio group); read-only when onChange is omitted. */
function StarRating({
  label,
  value,
  onChange,
  name,
}: {
  label: ReactNode;
  value: number;
  onChange?: (v: number) => void;
  name: string;
}) {
  const { t } = useLocale();
  const labelId = useId();
  return (
    <div className="space-y-1.5">
      <span id={labelId} className="text-sm font-medium">
        {label}
      </span>
      <div
        role={onChange ? "radiogroup" : "img"}
        aria-labelledby={labelId}
        aria-label={onChange ? undefined : t("portal.review.star", { value })}
        // Flex follows the writing direction: star 1 sits at the start
        // (right in RTL) without physical classes.
        className="flex gap-1"
        data-rating={name}
      >
        {RATINGS.map((n) => {
          const on = n <= value;
          const icon = (
            <Star
              className={cn(
                "size-6",
                on ? "fill-amber-400 text-amber-400" : "text-muted-foreground",
              )}
            />
          );
          return onChange ? (
            <button
              key={n}
              type="button"
              role="radio"
              aria-checked={n === value}
              aria-label={t("portal.review.star", { value: n })}
              data-star={n}
              onClick={() => onChange(n)}
              className="focus-visible:ring-ring rounded outline-none focus-visible:ring-2"
            >
              {icon}
            </button>
          ) : (
            <span key={n} aria-hidden>
              {icon}
            </span>
          );
        })}
      </div>
    </div>
  );
}

/** Label of an admin question row: its text, plus the product when targeted. */
function rowLabel(row: ReviewFormRow, required: string) {
  const text = row.question.text || row.question.question_key;
  return (
    <>
      {text}
      {row.product ? (
        <span className="text-muted-foreground font-normal">
          {" "}
          · {row.product.name}
        </span>
      ) : null}
      {row.question.is_required ? (
        <>
          <span className="text-destructive" aria-hidden>
            {" "}
            *
          </span>
          <span className="sr-only"> ({required})</span>
        </>
      ) : null}
    </>
  );
}

/** One admin question row: star picker or text box. */
function QuestionField({
  row,
  value,
  onChange,
}: {
  row: ReviewFormRow;
  value: number | string | undefined;
  onChange: (value: number | string) => void;
}) {
  const { t } = useLocale();
  const id = useId();
  const label = rowLabel(row, t("portal.review.required"));
  if (row.question.question_type === "rating_1_5") {
    return (
      <div data-question={row.key}>
        <StarRating
          name={row.key}
          label={label}
          value={typeof value === "number" ? value : 0}
          onChange={onChange}
        />
      </div>
    );
  }
  return (
    <div className="space-y-1.5" data-question={row.key}>
      <Label htmlFor={id}>{label}</Label>
      <Textarea
        id={id}
        value={typeof value === "string" ? value : ""}
        maxLength={REVIEW_COMMENT_MAX}
        required={row.question.is_required}
        onChange={(e) => onChange(e.target.value)}
        rows={2}
      />
    </div>
  );
}

/** Review form body (exported for tests). */
export function PortalServiceReviewView({
  serviceUuid,
  state,
  readOnly = false,
  fromLink = false,
}: {
  serviceUuid: string;
  state: PortalServiceReviewState;
  /** Read-only fleet session (TEC-245): no form, the API answers 403. */
  readOnly?: boolean;
  /** Opened from the WhatsApp review link: focus the form, source=whatsapp_link. */
  fromLink?: boolean;
}) {
  const { t } = useLocale();
  const qc = useQueryClient();
  const commentId = useId();
  const anonymousId = useId();
  const cardRef = useRef<HTMLDivElement>(null);
  const [platform, setPlatform] = useState(0);
  const [product, setProduct] = useState(0);
  const [comment, setComment] = useState("");
  const [answers, setAnswers] = useState<ReviewFormValues>({});
  const [anonymous, setAnonymous] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const google = googleReviewUrl(state.google_business_url);
  const queryKey = ["portal", "services", "review", serviceUuid];
  const rows = useMemo(
    () => reviewFormRows(state.questions ?? [], state.products ?? []),
    [state.questions, state.products],
  );
  const showForm = !state.review && state.can_review && !readOnly;
  const incomplete =
    platform < 1 || product < 1 || missingRequired(rows, answers);

  // The WhatsApp link lands on the service page: bring the form into view.
  useEffect(() => {
    if (!fromLink || !showForm) return;
    cardRef.current?.scrollIntoView?.({ block: "start" });
  }, [fromLink, showForm]);

  const submit = useMutation({
    mutationFn: () =>
      portalServiceReviewApi.create(serviceUuid, {
        platform_rating: platform,
        product_rating: product,
        comment: comment.trim() || null,
        is_anonymous: anonymous,
        source: fromLink ? "whatsapp_link" : "portal",
        // Legacy body (two ratings) when the brand has no admin question.
        ...(rows.length ? { answers: reviewAnswers(rows, answers) } : {}),
      }),
    onSuccess: (next) => {
      setError(null);
      qc.setQueryData(queryKey, next);
    },
    onError: (err) => {
      if (err instanceof PortalApiError && err.status === 409) {
        setError(t("portal.review.already"));
        void qc.invalidateQueries({ queryKey });
        return;
      }
      setError(t("portal.review.failed"));
    },
  });

  const onSubmit = (e: FormEvent) => {
    e.preventDefault();
    if (platform < 1 || product < 1) {
      setError(t("portal.review.rating_required"));
      return;
    }
    if (missingRequired(rows, answers)) {
      setError(t("portal.review.answers_required"));
      return;
    }
    setError(null);
    submit.mutate();
  };

  const review = state.review;
  const answerRows = review
    ? reviewFormRows(state.questions ?? [], state.products ?? []).filter(
        (row) =>
          review.answers.some(
            (a) =>
              a.question_uuid === row.question.uuid &&
              (a.product_uuid ?? null) === (row.product?.uuid ?? null),
          ),
      )
    : [];

  return (
    <Card
      ref={cardRef}
      id="review"
      className="scroll-mt-4"
      data-testid="portal-service-review"
      data-source={fromLink ? "whatsapp_link" : "portal"}
    >
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-base">
          <Star className="size-5" />
          {review ? t("portal.review.your_review") : t("portal.review.title")}
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        {review ? (
          <div className="space-y-3" data-testid="portal-review-done">
            <p className="text-sm">{t("portal.review.thanks")}</p>
            <div className="flex flex-wrap gap-6">
              <StarRating
                name="platform"
                label={t("portal.review.platform_rating")}
                value={review.platform_rating}
              />
              <StarRating
                name="product"
                label={t("portal.review.product_rating")}
                value={review.product_rating}
              />
            </div>
            {answerRows.map((row) => {
              const answer = review.answers.find(
                (a) =>
                  a.question_uuid === row.question.uuid &&
                  (a.product_uuid ?? null) === (row.product?.uuid ?? null),
              );
              return answer?.rating ? (
                <StarRating
                  key={row.key}
                  name={row.key}
                  label={rowLabel(row, t("portal.review.required"))}
                  value={answer.rating}
                />
              ) : answer?.text ? (
                <div key={row.key} className="space-y-1 text-sm">
                  <p className="font-medium">
                    {rowLabel(row, t("portal.review.required"))}
                  </p>
                  <p className="text-muted-foreground whitespace-pre-line">
                    {answer.text}
                  </p>
                </div>
              ) : null;
            })}
            {review.comment ? (
              <p className="text-muted-foreground text-sm whitespace-pre-line">
                {review.comment}
              </p>
            ) : null}
            {review.is_anonymous ? (
              <p className="text-muted-foreground text-xs">
                {t("portal.review.sent_anonymous")}
              </p>
            ) : null}
          </div>
        ) : showForm ? (
          <form
            className="space-y-4"
            onSubmit={onSubmit}
            data-testid="portal-review-form"
            noValidate
          >
            <p className="text-muted-foreground text-sm">
              {t("portal.review.description")}
            </p>
            <div className="flex flex-wrap gap-6">
              <StarRating
                name="platform"
                label={t("portal.review.platform_rating")}
                value={platform}
                onChange={setPlatform}
              />
              <StarRating
                name="product"
                label={t("portal.review.product_rating")}
                value={product}
                onChange={setProduct}
              />
            </div>
            {rows.length ? (
              <div
                className="space-y-4 border-t pt-4"
                data-testid="portal-review-questions"
              >
                {rows.map((row) => (
                  <QuestionField
                    key={row.key}
                    row={row}
                    value={answers[row.key]}
                    onChange={(value) =>
                      setAnswers((prev) => ({ ...prev, [row.key]: value }))
                    }
                  />
                ))}
              </div>
            ) : null}
            <div className="space-y-1.5">
              <Label htmlFor={commentId}>{t("portal.review.comment")}</Label>
              <Textarea
                id={commentId}
                name="comment"
                value={comment}
                maxLength={REVIEW_COMMENT_MAX}
                onChange={(e) => setComment(e.target.value)}
              />
              <p className="text-muted-foreground text-xs">
                {t("portal.review.comment_hint", { max: REVIEW_COMMENT_MAX })}
              </p>
            </div>
            <div className="flex items-start gap-2">
              <Checkbox
                id={anonymousId}
                checked={anonymous}
                onCheckedChange={(v) => setAnonymous(v === true)}
                data-testid="portal-review-anonymous"
                className="mt-0.5"
              />
              <div className="space-y-0.5">
                <Label htmlFor={anonymousId}>
                  {t("portal.review.anonymous")}
                </Label>
                <p className="text-muted-foreground text-xs">
                  {t("portal.review.anonymous_hint")}
                </p>
              </div>
            </div>
            {error ? (
              <p className="text-destructive text-sm" role="alert">
                {error}
              </p>
            ) : null}
            <Button
              type="submit"
              disabled={submit.isPending || incomplete}
              data-testid="portal-review-submit"
            >
              {submit.isPending
                ? t("portal.review.submitting")
                : t("portal.review.submit")}
            </Button>
          </form>
        ) : readOnly ? null : (
          <p className="text-muted-foreground text-sm">
            {t("portal.review.not_completed")}
          </p>
        )}
        {google ? (
          <div className="space-y-2 border-t pt-4">
            <p className="text-muted-foreground text-sm">
              {t("portal.review.google_hint")}
            </p>
            <Button asChild variant="outline">
              <a
                href={google}
                target="_blank"
                rel="noreferrer"
                data-testid="review-google"
              >
                <ExternalLink className="size-4" />
                {t("portal.review.google")}
              </a>
            </Button>
          </div>
        ) : null}
      </CardContent>
    </Card>
  );
}

/**
 * Portal > service > review (TEC-244, TEC-353): platform and product
 * quality ratings, the admin questions (stars / text, product questions
 * once per product), a comment and "send anonymously", once per service,
 * and the dealer's Google review link when it is set (GET / POST
 * /v1/portal/services/{uuid}/review). A failed load hides the card; the
 * rest of the service page stays usable.
 */
export function PortalServiceReview({
  serviceUuid,
  fromLink = false,
}: {
  serviceUuid: string;
  fromLink?: boolean;
}) {
  const readOnly = usePortalReadOnly();
  const review = useQuery({
    queryKey: ["portal", "services", "review", serviceUuid],
    queryFn: () => portalServiceReviewApi.get(serviceUuid),
    retry: (count, error) =>
      !(error instanceof PortalApiError && error.status === 404) && count < 2,
  });
  const data = review.data;
  if (!data) return null;
  // A fleet session sees a stored review and the Google link, never the form.
  if (readOnly && !data.review && !googleReviewUrl(data.google_business_url)) {
    return null;
  }
  return (
    <PortalServiceReviewView
      serviceUuid={serviceUuid}
      state={data}
      readOnly={readOnly}
      fromLink={fromLink}
    />
  );
}
