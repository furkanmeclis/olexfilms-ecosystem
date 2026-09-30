"use client";

import { Controller, useFormContext } from "react-hook-form";

import { FormFieldShell } from "@/components/forms/form-field";
import {
  AsyncCombobox,
  type AsyncComboboxProps,
  type ComboboxOption,
} from "@/components/ui/async-combobox";

type AppComboboxProps = {
  name: string;
  label?: string;
  description?: string;
  className?: string;
  /** Called after RHF field update (e.g. cascade clears). */
  onValueChange?: (value: string) => void;
} & Omit<AsyncComboboxProps, "value" | "onValueChange" | "id" | "aria-invalid">;

/** RHF combobox — static `options` or async `loadOptions` (API). */
export function AppCombobox({
  name,
  label,
  description,
  className,
  onValueChange,
  ...props
}: AppComboboxProps) {
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
          <AsyncCombobox
            id={name}
            value={field.value ?? ""}
            onValueChange={(value) => {
              field.onChange(value);
              onValueChange?.(value);
            }}
            aria-invalid={Boolean(error)}
            {...props}
          />
        )}
      />
    </FormFieldShell>
  );
}

export type { ComboboxOption };
