"use client";

import { Car } from "lucide-react";
import { useState } from "react";

import { brandLogoUrl } from "@/features/vehicle-catalog/lib/images";
import { cn } from "@/lib/utils/index";

export type VehicleBrandLogoProps = {
  /** Brand uuid; without it only the placeholder renders. */
  uuid?: string | null;
  /** Brand name, used as alt text. */
  name?: string;
  /** `?v=` cache-buster (from the brand's logo_url) so a new upload shows. */
  version?: string | null;
  /** Rendered height in px (brand logo_height when set). */
  height?: number;
  className?: string;
};

/**
 * Car brand logo from the fixed public URL `/brand-logos/{uuid}` (TEC-150).
 * A plain lazy <img> (no next/image: the URL is already cached by ETag and
 * also used in e-mails and PDFs). The backend answers a placeholder SVG for
 * brands without a logo; a load failure falls back to a local placeholder.
 * Shared: the platform catalog pages and, later, the service wizard use it.
 */
export function VehicleBrandLogo({
  uuid,
  name,
  version,
  height = 32,
  className,
}: VehicleBrandLogoProps) {
  const src = uuid ? brandLogoUrl(uuid, version) : null;
  const [failedSrc, setFailedSrc] = useState<string | null>(null);

  if (!src || failedSrc === src) {
    return (
      <span
        role="img"
        aria-label={name ?? undefined}
        aria-hidden={name ? undefined : true}
        data-slot="vehicle-brand-logo-placeholder"
        className={cn(
          "bg-muted text-muted-foreground inline-flex shrink-0 items-center justify-center rounded-md",
          className,
        )}
        style={{ height, width: height }}
      >
        <Car className="size-1/2" />
      </span>
    );
  }

  return (
    // eslint-disable-next-line @next/next/no-img-element -- fixed public URL, cached by ETag
    <img
      src={src}
      alt={name ?? ""}
      loading="lazy"
      decoding="async"
      data-slot="vehicle-brand-logo"
      className={cn("w-auto max-w-40 shrink-0 object-contain", className)}
      style={{ height }}
      onError={() => setFailedSrc(src)}
    />
  );
}
