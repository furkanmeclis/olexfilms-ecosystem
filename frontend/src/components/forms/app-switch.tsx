"use client";

import { Controller, useFormContext } from "react-hook-form";

import {
  Field,
  FieldContent,
  FieldDescription,
  FieldError,
  FieldLabel,
} from "@/components/ui/field";
import { Switch } from "@/components/ui/switch";
import { cn } from "@/lib/utils";

type AppSwitchProps = {
  name: string;
  label: string;
  description?: string;
  className?: string;
  disabled?: boolean;
};

export function AppSwitch({
  name,
  label,
  description,
  className,
  disabled,
}: AppSwitchProps) {
  const {
    control,
    formState: { errors },
  } = useFormContext();
  const error = errors[name]?.message as string | undefined;

  return (
    <Field
      data-invalid={Boolean(error) || undefined}
      orientation="horizontal"
      className={cn("items-start justify-between gap-4", className)}
    >
      <FieldContent className="min-w-0 flex-1 gap-0.5">
        <FieldLabel htmlFor={name}>{label}</FieldLabel>
        {description && !error ? (
          <FieldDescription className="text-xs">{description}</FieldDescription>
        ) : null}
        {error ? <FieldError>{error}</FieldError> : null}
      </FieldContent>
      <Controller
        name={name}
        control={control}
        render={({ field }) => (
          <Switch
            id={name}
            className="mt-0.5 shrink-0"
            checked={Boolean(field.value)}
            onCheckedChange={field.onChange}
            disabled={disabled}
            aria-invalid={Boolean(error)}
          />
        )}
      />
    </Field>
  );
}
