"use client";

import { TriangleAlert } from "lucide-react";
import { useState, type ReactNode } from "react";

import { FilePickButton } from "@/components/common/file-pick-button";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import {
  VEHICLE_IMAGE_ACCEPT,
  vehicleImageProblem,
  type VehicleImageKind,
} from "@/features/vehicle-catalog/lib/images";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

const PROBLEM_KEYS = {
  svg: "vehicles.image.svg_refused",
  type: "vehicles.image.type_refused",
  size: "vehicles.image.size_refused",
} as const;

/**
 * Logo / hero picker for the vehicle catalog. Checks the file before upload:
 * SVG (refused by the backend) and other types get an inline warning and a
 * toast, never a request.
 */
export function VehicleImageField({
  kind,
  label,
  preview,
  hasImage,
  pending,
  disabled,
  onUpload,
  onRemove,
}: {
  kind: VehicleImageKind;
  label: string;
  preview?: ReactNode;
  hasImage: boolean;
  pending?: boolean;
  disabled?: boolean;
  onUpload: (file: File) => void;
  onRemove?: () => void;
}) {
  const { t } = useLocale();
  const [warning, setWarning] = useState<string | null>(null);
  const hint = t(
    kind === "logo" ? "vehicles.image.logo_hint" : "vehicles.image.hero_hint",
  );

  return (
    <div className="flex flex-col gap-3" data-slot="vehicle-image-field">
      <span className="text-sm font-medium">{label}</span>
      {preview}
      <div className="flex flex-wrap items-center gap-2">
        <FilePickButton
          accept={VEHICLE_IMAGE_ACCEPT}
          label={t("vehicles.image.choose")}
          hint={hint}
          pending={pending}
          disabled={disabled}
          onFile={(file) => {
            const problem = vehicleImageProblem(file, kind);
            if (problem) {
              const message = t(PROBLEM_KEYS[problem]);
              setWarning(message);
              appToast.warning(message);
              return;
            }
            setWarning(null);
            onUpload(file);
          }}
        />
        {hasImage && onRemove ? (
          <Button
            type="button"
            variant="ghost"
            size="sm"
            disabled={disabled || pending}
            onClick={onRemove}
          >
            {t("vehicles.image.remove")}
          </Button>
        ) : null}
      </div>
      {warning ? (
        <Alert variant="destructive" data-slot="vehicle-image-warning">
          <TriangleAlert />
          <AlertDescription>{warning}</AlertDescription>
        </Alert>
      ) : null}
    </div>
  );
}
