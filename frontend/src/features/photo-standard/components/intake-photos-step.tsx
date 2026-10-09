"use client";

import { useQueryClient } from "@tanstack/react-query";
import {
  AlertTriangle,
  Camera,
  Car,
  CheckCircle2,
  RefreshCw,
} from "lucide-react";
import { useRef, useState, type ChangeEvent } from "react";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Progress } from "@/components/ui/progress";
import {
  angleName,
  localizedText,
  validatePhotoFile,
} from "@/features/photo-standard/lib/photo-standard";
import {
  photoSrc,
  photoStandardKeys,
  photoStandardService,
  type IntakePhotoAngle,
  type IntakePhotoList,
} from "@/features/photo-standard/services/photo-standard.service";
import { isApiError } from "@/lib/api";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

const T = "photo_standard.intake";

type CardState =
  | { kind: "idle" }
  | { kind: "uploading"; percent: number }
  | { kind: "error"; message: string };

function AngleCard({
  serviceUuid,
  item,
  flagged,
  disabled,
  onUploaded,
}: {
  serviceUuid: string;
  item: IntakePhotoAngle;
  flagged: boolean;
  disabled: boolean;
  onUploaded: (key: string) => void;
}) {
  const { t, locale, format } = useLocale();
  const input = useRef<HTMLInputElement>(null);
  const [state, setState] = useState<CardState>({ kind: "idle" });
  const { angle, photo } = item;
  const name = angleName(angle, locale);
  const hint = localizedText(angle.hint, locale);
  // Red only after the server refused the service (422); the counter
  // below already tells how many required angles are open.
  const missing = !photo && flagged;
  const uploading = state.kind === "uploading";

  const pick = async (e: ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    // The same file may be picked again after an error.
    e.target.value = "";
    if (!file) return;
    const invalid = validatePhotoFile(file);
    if (invalid) {
      setState({ kind: "error", message: t(`${T}.errors.${invalid}`) });
      return;
    }
    setState({ kind: "uploading", percent: 0 });
    try {
      await photoStandardService.uploadIntake(
        serviceUuid,
        angle.key,
        file,
        (percent) => setState({ kind: "uploading", percent }),
      );
      setState({ kind: "idle" });
      onUploaded(angle.key);
    } catch (error) {
      setState({
        kind: "error",
        message: isApiError(error)
          ? error.message
          : t(`${T}.errors.upload_failed`),
      });
    }
  };

  return (
    <li
      className={cn(
        "flex flex-col gap-3 rounded-lg border p-3",
        missing && "border-destructive bg-destructive/5",
      )}
      data-testid="intake-angle"
      data-angle={angle.key}
      data-missing={missing ? "true" : "false"}
    >
      <div className="bg-muted relative flex aspect-[4/3] items-center justify-center overflow-hidden rounded-md">
        {photo && photo.mime !== "image/heic" ? (
          // eslint-disable-next-line @next/next/no-img-element
          <img
            src={photoSrc(photo.url)}
            alt={name}
            className="size-full object-cover"
            data-testid="intake-photo"
          />
        ) : !photo && angle.example_url ? (
          // eslint-disable-next-line @next/next/no-img-element
          <img
            src={photoSrc(angle.example_url)}
            alt={t(`${T}.example_alt`, { name })}
            className="size-full object-contain opacity-40 grayscale"
            data-testid="intake-example"
          />
        ) : photo ? (
          <CheckCircle2 className="text-primary size-10" />
        ) : (
          <Car className="text-muted-foreground size-12" />
        )}
        {photo ? (
          <Badge className="absolute end-2 top-2" data-testid="intake-done">
            <CheckCircle2 className="size-3" />
            {t(`${T}.done`)}
          </Badge>
        ) : null}
      </div>
      <div className="space-y-1">
        <div className="flex flex-wrap items-center gap-2">
          <span className="font-medium">{name}</span>
          {item.required ? (
            <Badge variant="outline">{t(`${T}.required`)}</Badge>
          ) : (
            <Badge variant="secondary">{t(`${T}.optional`)}</Badge>
          )}
        </div>
        {hint ? <p className="text-muted-foreground text-xs">{hint}</p> : null}
        {photo ? (
          <p className="text-muted-foreground text-xs">
            {t(`${T}.taken_at`, {
              date: format.dateTime(photo.exif_taken_at ?? photo.created_at),
            })}
          </p>
        ) : null}
        {missing ? (
          <p
            className="text-destructive flex items-center gap-1 text-xs font-medium"
            data-testid="intake-missing"
          >
            <AlertTriangle className="size-3" />
            {t(`${T}.missing`)}
          </p>
        ) : null}
      </div>
      {uploading ? (
        <div className="space-y-1" data-testid="intake-progress">
          <Progress value={state.percent} />
          <p className="text-muted-foreground text-xs">
            {t(`${T}.uploading`, { percent: state.percent })}
          </p>
        </div>
      ) : null}
      {state.kind === "error" ? (
        <p
          className="text-destructive text-xs"
          role="alert"
          data-testid="intake-error"
        >
          {state.message}
        </p>
      ) : null}
      <input
        ref={input}
        type="file"
        accept="image/*"
        capture="environment"
        className="sr-only"
        tabIndex={-1}
        aria-label={t(`${T}.take_aria`, { name })}
        onChange={(e) => void pick(e)}
        disabled={disabled || uploading}
        data-testid="intake-input"
      />
      <Button
        type="button"
        variant={photo ? "outline" : "default"}
        className="mt-auto"
        disabled={disabled || uploading}
        onClick={() => input.current?.click()}
      >
        {photo ? (
          <RefreshCw className="size-4" />
        ) : (
          <Camera className="size-4" />
        )}
        {photo ? t(`${T}.retake`) : t(`${T}.take`)}
      </Button>
    </li>
  );
}

export type IntakePhotosStepProps = {
  serviceUuid: string;
  intake: {
    data?: IntakePhotoList;
    isLoading: boolean;
    isError: boolean;
    refetch: () => unknown;
  };
  /** Angles a 422 PHOTO_STANDARD_INCOMPLETE reported missing. */
  flagged: readonly string[];
  /** The service no longer accepts intake photo changes. */
  locked?: boolean;
  onUploaded?: (key: string) => void;
  onBack: () => void;
  onNext: () => void;
};

/**
 * Wizard step "Fotoğraflar" (TEC-500): one card per resolved angle with the
 * example silhouette and hint; the mobile camera opens through
 * `capture="environment"`. Files over 12 MB are refused before upload, the
 * original is sent unchanged. "İleri" stays disabled (with the reason)
 * while a required angle is missing.
 */
export function IntakePhotosStep({
  serviceUuid,
  intake,
  flagged,
  locked = false,
  onUploaded,
  onBack,
  onNext,
}: IntakePhotosStepProps) {
  const { t } = useLocale();
  const queryClient = useQueryClient();

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

  const data = intake.data;
  const missing = data.missing ?? [];
  const blocked = missing.length > 0;

  return (
    <div className="space-y-6" data-testid="photos-step">
      <div className="space-y-1">
        <h2 className="text-lg font-semibold">{t(`${T}.title`)}</h2>
        <p className="text-muted-foreground text-sm">{t(`${T}.description`)}</p>
      </div>
      {data.angles.length === 0 ? (
        <p className="text-muted-foreground text-sm" data-testid="intake-empty">
          {t(`${T}.empty`)}
        </p>
      ) : (
        <ul className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {data.angles.map((item) => (
            <AngleCard
              key={item.angle.key}
              serviceUuid={serviceUuid}
              item={item}
              flagged={flagged.includes(item.angle.key)}
              disabled={locked}
              onUploaded={(key) => {
                onUploaded?.(key);
                void queryClient.invalidateQueries({
                  queryKey: photoStandardKeys.intake(serviceUuid),
                });
              }}
            />
          ))}
        </ul>
      )}
      {blocked ? (
        <Alert variant="destructive" data-testid="intake-counter">
          <AlertTriangle />
          <AlertDescription id="intake-blocked-reason">
            {t(`${T}.missing_count`, { count: missing.length })}
          </AlertDescription>
        </Alert>
      ) : (
        <Alert data-testid="intake-complete">
          <CheckCircle2 />
          <AlertDescription>{t(`${T}.complete`)}</AlertDescription>
        </Alert>
      )}
      <div className="flex flex-wrap justify-between gap-2">
        <Button type="button" variant="outline" onClick={onBack}>
          {t("services.wizard.back")}
        </Button>
        <Button
          type="button"
          disabled={blocked}
          aria-describedby={blocked ? "intake-blocked-reason" : undefined}
          title={blocked ? t(`${T}.next_blocked`) : undefined}
          onClick={onNext}
          data-testid="photos-next"
        >
          {t("services.wizard.next")}
        </Button>
      </div>
    </div>
  );
}
