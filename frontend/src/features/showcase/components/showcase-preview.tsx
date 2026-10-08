"use client";

import { Star } from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import {
  showcaseService,
  type Showcase,
} from "@/features/showcase/services/showcase.service";
import { useLocale } from "@/providers/locale-provider";

const DAYS = [
  "monday",
  "tuesday",
  "wednesday",
  "thursday",
  "friday",
  "saturday",
  "sunday",
] as const;

function textFor(
  values: Record<string, string> | undefined,
  locale: string,
  fallback = "tr",
) {
  return values?.[locale] || values?.[fallback] || values?.en || "";
}

export function ShowcasePreview({
  showcase,
  draft = false,
}: {
  showcase: Showcase;
  draft?: boolean;
}) {
  const { t, locale, format } = useLocale();
  const content = showcase.content[locale] ?? showcase.content.tr ?? {};
  const services = showcase.services.filter((service) => service.visible);
  return (
    <div className="space-y-4" data-testid="showcase-preview">
      <section className="border-border bg-muted/30 rounded-lg border p-5">
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div>
            <p className="text-muted-foreground text-sm">
              {showcase.organization.city}
            </p>
            <h2 className="text-2xl font-semibold">
              {content.headline || showcase.organization.name}
            </h2>
          </div>
          {draft ? (
            <Badge variant="outline">{t("showcase.preview.draft")}</Badge>
          ) : null}
        </div>
        <p className="mt-3 max-w-3xl text-sm leading-6">
          {content.about || t("showcase.preview.empty_about")}
        </p>
      </section>

      <div className="grid gap-4 lg:grid-cols-[1.5fr_1fr]">
        <Card>
          <CardHeader>
            <CardTitle>{t("showcase.services.title")}</CardTitle>
          </CardHeader>
          <CardContent className="grid gap-3 sm:grid-cols-2">
            {services.length ? (
              services.map((service) => (
                <div
                  key={service.uuid}
                  className="border-border rounded-md border p-3"
                >
                  <p className="font-medium">
                    {textFor(service.title, locale, "tr") ||
                      service.category?.name ||
                      t("showcase.services.custom")}
                  </p>
                  <p className="text-muted-foreground mt-1 text-sm">
                    {textFor(service.description, locale, "tr")}
                  </p>
                </div>
              ))
            ) : (
              <p className="text-muted-foreground text-sm">
                {t("showcase.services.empty")}
              </p>
            )}
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>{t("showcase.hours.title")}</CardTitle>
          </CardHeader>
          <CardContent className="space-y-2 text-sm">
            {DAYS.map((day) => {
              const windows = showcase.working_hours[day] ?? [];
              return (
                <div key={day} className="flex justify-between gap-3">
                  <span>{t(`showcase.days.${day}`)}</span>
                  <span className="text-muted-foreground">
                    {windows.length
                      ? windows.map((w) => `${w.start}-${w.end}`).join(", ")
                      : t("showcase.hours.closed")}
                  </span>
                </div>
              );
            })}
          </CardContent>
        </Card>
      </div>

      <Card>
        <CardHeader>
          <CardTitle>{t("showcase.gallery.title")}</CardTitle>
        </CardHeader>
        <CardContent className="grid gap-3 sm:grid-cols-3 lg:grid-cols-4">
          {showcase.photos.length ? (
            showcase.photos.map((photo) => (
              <figure key={photo.uuid} className="space-y-2">
                {/* eslint-disable-next-line @next/next/no-img-element */}
                <img
                  src={showcaseService.photoUrl(photo.uuid)}
                  alt={textFor(photo.caption, locale)}
                  className="bg-muted aspect-[4/3] w-full rounded-md object-cover"
                />
                <figcaption className="text-muted-foreground text-xs">
                  {textFor(photo.caption, locale)}
                </figcaption>
              </figure>
            ))
          ) : (
            <p className="text-muted-foreground text-sm">
              {t("showcase.gallery.empty")}
            </p>
          )}
        </CardContent>
      </Card>

      <Card>
        <CardContent className="flex flex-wrap items-center gap-3 pt-6">
          <Star className="size-5 fill-amber-400 text-amber-500" aria-hidden />
          <span className="font-medium">
            {showcase.google_rating
              ? `${showcase.google_rating.toFixed(1)} / 5`
              : t("showcase.rating.none")}
          </span>
          {showcase.google_review_count !== null ? (
            <span className="text-muted-foreground text-sm">
              {t("showcase.rating.reviews", {
                count: showcase.google_review_count,
              })}
            </span>
          ) : null}
          {showcase.google_rating_updated_at ? (
            <span className="text-muted-foreground text-xs">
              {format.dateTime(showcase.google_rating_updated_at)}
            </span>
          ) : null}
        </CardContent>
      </Card>
    </div>
  );
}
