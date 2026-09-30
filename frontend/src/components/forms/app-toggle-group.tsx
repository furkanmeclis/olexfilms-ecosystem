"use client";

import { Controller, useFormContext } from "react-hook-form";

import { FormFieldShell } from "@/components/forms/form-field";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";

type Option = { label: string; value: string };

type AppToggleGroupProps = {
  name: string;
  label?: string;
  description?: string;
  options: Option[];
  type?: "single" | "multiple";
  className?: string;
};

export function AppToggleGroup({
  name,
  label,
  description,
  options,
  type = "single",
  className,
}: AppToggleGroupProps) {
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
        render={({ field }) =>
          type === "multiple" ? (
            <ToggleGroup
              type="multiple"
              variant="outline"
              spacing={0}
              value={Array.isArray(field.value) ? field.value : []}
              onValueChange={field.onChange}
              className="flex flex-wrap justify-start"
              aria-invalid={Boolean(error)}
            >
              {options.map((option) => (
                <ToggleGroupItem
                  key={option.value}
                  value={option.value}
                  className="px-3"
                >
                  {option.label}
                </ToggleGroupItem>
              ))}
            </ToggleGroup>
          ) : (
            <ToggleGroup
              type="single"
              variant="outline"
              spacing={0}
              value={field.value ?? ""}
              onValueChange={(v) => {
                if (v) field.onChange(v);
              }}
              className="flex flex-wrap justify-start"
              aria-invalid={Boolean(error)}
            >
              {options.map((option) => (
                <ToggleGroupItem
                  key={option.value}
                  value={option.value}
                  className="px-3"
                >
                  {option.label}
                </ToggleGroupItem>
              ))}
            </ToggleGroup>
          )
        }
      />
    </FormFieldShell>
  );
}
