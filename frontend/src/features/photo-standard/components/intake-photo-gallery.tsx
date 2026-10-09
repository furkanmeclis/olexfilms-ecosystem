"use client";

import { useQuery } from "@tanstack/react-query";
import { Car, FileImage, MapPin } from "lucide-react";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { Badge } from "@/components/ui/badge";
import {
  angleName,
  canSeeIntakeLocation,
  mapLink,
} from "@/features/photo-standard/lib/photo-standard";
import {
  photoSrc,
  photoStandardKeys,
  photoStandardService,
} from "@/features/photo-standard/services/photo-standard.service";
import { useActiveOrganization } from "@/hooks/use-active-organization";
import { useAuth } from "@/providers/auth-provider";
import { useLocale } from "@/providers/locale-provider";

const T = "photo_standard.gallery";

/**
 * "Kabul fotoğrafları" (TEC-500): the intake photos of a service, one tile
 * per resolved angle, apart from the service images. The capture time is
 * the EXIF date (else the upload time); the location is a small map link
 * only for the roles allowed by KVKK (TEC-499).
 */
export function IntakePhotoGallery({
  slug,
  serviceUuid,
  serviceOrgUuid,
}: {
  slug: string;
  serviceUuid: string;
  serviceOrgUuid: string;
}) {
  const { t, locale, format } = useLocale();
  const { user } = useAuth();
  const org = useActiveOrganization(slug);
  const intake = useQuery({
    queryKey: photoStandardKeys.intake(serviceUuid),
    queryFn: () => photoStandardService.intake(serviceUuid),
  });
  const showLocation = canSeeIntakeLocation({
    isSuperAdmin: Boolean(user?.isSuperAdmin),
    orgType: org?.type,
    orgUuid: org?.uuid,
    orgRoles: user?.organizationRoles ?? [],
    serviceOrgUuid,
  });

  if (intake.isLoading) return <Loading />;
  if (intake.isError || !intake.data) {
    return (
      <ErrorState
        title={t("common.error_generic")}
        onRetry={() => void intake.refetch()}
        retryLabel={t("common.retry")}
      />
    );
  }
  const angles = intake.data.angles;
  if (!angles.some((a) => a.photo)) {
    return (
      <p
        className="text-muted-foreground text-sm"
        data-testid="intake-gallery-empty"
      >
        {t(`${T}.empty`)}
      </p>
    );
  }

  return (
    <ul
      className="grid grid-cols-2 gap-3 sm:grid-cols-3"
      data-testid="intake-gallery"
    >
      {angles.map(({ angle, photo, missing }) => {
        const name = angleName(angle, locale);
        const map = showLocation
          ? mapLink(photo?.exif_lat, photo?.exif_lng)
          : null;
        return (
          <li
            key={angle.key}
            className="space-y-1"
            data-testid="intake-gallery-item"
            data-angle={angle.key}
          >
            {photo ? (
              <a
                href={photoSrc(photo.url)}
                target="_blank"
                rel="noreferrer"
                className="bg-muted flex aspect-square items-center justify-center overflow-hidden rounded-md border"
              >
                {photo.mime === "image/heic" ? (
                  <FileImage className="text-muted-foreground size-10" />
                ) : (
                  // eslint-disable-next-line @next/next/no-img-element
                  <img
                    src={photoSrc(photo.url)}
                    alt={name}
                    loading="lazy"
                    className="size-full object-cover"
                  />
                )}
              </a>
            ) : (
              <div className="bg-muted/50 flex aspect-square items-center justify-center rounded-md border border-dashed">
                <Car className="text-muted-foreground size-8" />
              </div>
            )}
            <div className="flex flex-wrap items-center gap-1 text-sm font-medium">
              {name}
              {missing ? (
                <Badge variant="danger">{t(`${T}.missing`)}</Badge>
              ) : null}
            </div>
            {photo ? (
              <p className="text-muted-foreground text-xs">
                {photo.exif_taken_at
                  ? t(`${T}.taken_exif`, {
                      date: format.dateTime(photo.exif_taken_at),
                    })
                  : t(`${T}.taken_upload`, {
                      date: format.dateTime(photo.created_at),
                    })}
              </p>
            ) : null}
            {map ? (
              <a
                href={map}
                target="_blank"
                rel="noreferrer"
                className="text-primary inline-flex items-center gap-1 text-xs underline-offset-2 hover:underline"
                data-testid="intake-location"
              >
                <MapPin className="size-3" />
                {t(`${T}.location`)}
              </a>
            ) : null}
          </li>
        );
      })}
    </ul>
  );
}
