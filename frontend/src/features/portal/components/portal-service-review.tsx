"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ExternalLink, Star } from "lucide-react";
import { type FormEvent, useId, useState } from "react";

import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { PortalApiError } from "@/features/portal/lib/portal-client";
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
  label: string;
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

/** Review form body (exported for tests). */
export function PortalServiceReviewView({
  serviceUuid,
  state,
}: {
  serviceUuid: string;
  state: PortalServiceReviewState;
}) {
  const { t } = useLocale();
  const qc = useQueryClient();
  const commentId = useId();
  const [platform, setPlatform] = useState(0);
  const [product, setProduct] = useState(0);
  const [comment, setComment] = useState("");
  const [error, setError] = useState<string | null>(null);
  const google = googleReviewUrl(state.google_business_url);
  const queryKey = ["portal", "services", "review", serviceUuid];

  const submit = useMutation({
    mutationFn: () =>
      portalServiceReviewApi.create(serviceUuid, {
        platform_rating: platform,
        product_rating: product,
        comment: comment.trim() || null,
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
    setError(null);
    submit.mutate();
  };

  const review = state.review;

  return (
    <Card data-testid="portal-service-review">
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
            {review.comment ? (
              <p className="text-muted-foreground text-sm whitespace-pre-line">
                {review.comment}
              </p>
            ) : null}
          </div>
        ) : state.can_review ? (
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
            {error ? (
              <p className="text-destructive text-sm" role="alert">
                {error}
              </p>
            ) : null}
            <Button type="submit" disabled={submit.isPending}>
              {submit.isPending
                ? t("portal.review.submitting")
                : t("portal.review.submit")}
            </Button>
          </form>
        ) : (
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
 * Portal > service > review (TEC-244): platform and product quality
 * ratings with a comment, once per service, and the dealer's Google review
 * link when it is set (GET / POST /v1/portal/services/{uuid}/review). A
 * failed load hides the card; the rest of the service page stays usable.
 */
export function PortalServiceReview({ serviceUuid }: { serviceUuid: string }) {
  const review = useQuery({
    queryKey: ["portal", "services", "review", serviceUuid],
    queryFn: () => portalServiceReviewApi.get(serviceUuid),
    retry: (count, error) =>
      !(error instanceof PortalApiError && error.status === 404) && count < 2,
  });
  if (!review.data) return null;
  return (
    <PortalServiceReviewView serviceUuid={serviceUuid} state={review.data} />
  );
}
