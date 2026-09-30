"use client";

import { Controller, useFormContext } from "react-hook-form";

import { FormFieldShell } from "@/components/forms/form-field";
import { Checkbox } from "@/components/ui/checkbox";
import { FieldLabel } from "@/components/ui/field";

type AppCheckboxProps = {
  name: string;
  label: string;
  description?: string;
  className?: string;
};

export function AppCheckbox({
  name,
  label,
  description,
  className,
}: AppCheckboxProps) {
  const {
    control,
    formState: { errors },
  } = useFormContext();
  const error = errors[name]?.message as string | undefined;

  return (
    <FormFieldShell
      name={name}
      description={description}
      error={error}
      orientation="horizontal"
      className={className}
    >
      <Controller
        name={name}
        control={control}
        render={({ field }) => (
          <>
            <Checkbox
              id={name}
              checked={Boolean(field.value)}
              onCheckedChange={field.onChange}
              aria-invalid={Boolean(error)}
            />
            <FieldLabel htmlFor={name} className="font-normal">
              {label}
            </FieldLabel>
          </>
        )}
      />
    </FormFieldShell>
  );
}
