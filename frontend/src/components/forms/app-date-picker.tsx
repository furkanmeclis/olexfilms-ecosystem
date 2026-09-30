"use client";

import { Controller, useFormContext } from "react-hook-form";

import { FormFieldShell } from "@/components/forms/form-field";
import { DatePicker } from "@/components/ui/date-picker";

type AppDatePickerProps = {
  name: string;
  label?: string;
  description?: string;
  placeholder?: string;
  className?: string;
  disabled?: boolean;
};

/** Form-bound date field. Value stored as `yyyy-MM-dd`. */
export function AppDatePicker({
  name,
  label,
  description,
  placeholder,
  className,
  disabled,
}: AppDatePickerProps) {
  const {
    control,
    formState: { errors },
  } = useFormContext();
  const error = errors[name]?.message as string | undefined;

  return (
    <FormFieldShell
      name={name}
      label={label}
      description={description}
      error={error}
      className={className}
    >
      <Controller
        name={name}
        control={control}
        render={({ field }) => (
          <DatePicker
            id={name}
            value={typeof field.value === "string" ? field.value : ""}
            onChange={field.onChange}
            placeholder={placeholder}
            disabled={disabled}
            aria-invalid={Boolean(error)}
            className={error ? "border-destructive" : undefined}
          />
        )}
      />
    </FormFieldShell>
  );
}
