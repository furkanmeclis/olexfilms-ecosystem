"use client";

import { useFormContext } from "react-hook-form";

import { FormFieldShell } from "@/components/forms/form-field";

export const nativeSelectClass =
  "border-input bg-background h-9 w-full rounded-md border px-3 text-sm";

/**
 * A native select bound to the surrounding AppForm. Scanner terminals and
 * tests handle it better than a popover listbox.
 */
export function NativeSelectField({
  name,
  label,
  options,
  placeholder,
  description,
  testId,
}: {
  name: string;
  label: string;
  options: { value: string; label: string }[];
  placeholder?: string;
  description?: string;
  testId?: string;
}) {
  const {
    register,
    formState: { errors },
  } = useFormContext();
  const error = errors[name]?.message as string | undefined;
  return (
    <FormFieldShell
      name={name}
      label={label}
      description={description}
      error={error}
    >
      <select
        id={name}
        className={nativeSelectClass}
        aria-invalid={Boolean(error)}
        data-testid={testId}
        {...register(name)}
      >
        {placeholder !== undefined ? (
          <option value="">{placeholder}</option>
        ) : null}
        {options.map((o) => (
          <option key={o.value} value={o.value}>
            {o.label}
          </option>
        ))}
      </select>
    </FormFieldShell>
  );
}
