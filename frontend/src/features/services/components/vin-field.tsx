"use client";

import { useId } from "react";

import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  VIN_LENGTH,
  normalizeVin,
  validateVin,
  vinErrorKey,
} from "@/features/services/lib/vin";
import { useLocale } from "@/providers/locale-provider";

export type VinFieldProps = {
  value: string;
  onChange: (value: string) => void;
  required?: boolean;
  /** Show the validation error (after a submit attempt or once typed). */
  showError?: boolean;
  /** A server-side message for the field (400 `vin`). */
  serverError?: string | null;
  disabled?: boolean;
};

/** VIN input with the backend rules checked as the user types. */
export function VinField({
  value,
  onChange,
  required,
  showError,
  serverError,
  disabled,
}: VinFieldProps) {
  const { t } = useLocale();
  const id = useId();
  const error = validateVin(value, { required });
  const visibleError =
    (showError || normalizeVin(value).length >= VIN_LENGTH) && error
      ? t(vinErrorKey(error))
      : (serverError ?? null);
  const count = normalizeVin(value).length;

  return (
    <div className="space-y-2">
      <Label htmlFor={id}>
        {t("services.vin.label")}
        {required ? <span className="text-destructive ms-1">*</span> : null}
      </Label>
      <Input
        id={id}
        name="vin"
        dir="ltr"
        autoComplete="off"
        spellCheck={false}
        maxLength={24}
        className="font-mono uppercase"
        value={value}
        disabled={disabled}
        aria-invalid={visibleError ? true : undefined}
        aria-describedby={`${id}-hint`}
        onChange={(e) => onChange(e.target.value)}
      />
      <div
        id={`${id}-hint`}
        className="flex items-start justify-between gap-3 text-xs"
      >
        {visibleError ? (
          <p className="text-destructive" data-testid="vin-error">
            {visibleError}
          </p>
        ) : (
          <p className="text-muted-foreground">{t("services.vin.hint")}</p>
        )}
        <span className="text-muted-foreground shrink-0 tabular-nums">
          {count}/{VIN_LENGTH}
        </span>
      </div>
    </div>
  );
}
