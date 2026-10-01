"use client";

import { useLocale } from "@/providers/locale-provider";

/**
 * Hero preview from the backend `hero_url` (`/vehicle-heroes/*`, model →
 * brand → default fallback, TEC-149). `inherited` notes that the image comes
 * from the fallback chain rather than an own upload.
 */
export function VehicleHeroImage({
  src,
  alt,
  inherited,
}: {
  src: string;
  alt: string;
  inherited?: boolean;
}) {
  const { t } = useLocale();
  return (
    <figure className="flex flex-col gap-1">
      {/* eslint-disable-next-line @next/next/no-img-element -- fixed public URL, cached by ETag */}
      <img
        src={src}
        alt={alt}
        loading="lazy"
        decoding="async"
        className="bg-muted aspect-video w-full max-w-sm rounded-md object-contain"
      />
      {inherited ? (
        <figcaption className="text-muted-foreground text-xs">
          {t("vehicles.image.hero_inherited")}
        </figcaption>
      ) : null}
    </figure>
  );
}
