"use client";

import { Controller, useFormContext } from "react-hook-form";

import { FormFieldShell } from "@/components/forms/form-field";
import { Label } from "@/components/ui/label";
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group";

type Option = { label: string; value: string; description?: string };

type AppRadioGroupProps = {
  name: string;
  label?: string;
  description?: string;
  options: Option[];
  className?: string;
  orientation?: "vertical" | "horizontal";
};

export function AppRadioGroup({
  name,
  label,
  description,
  options,
  className,
  orientation = "vertical",
}: AppRadioGroupProps) {
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
          <RadioGroup
            value={field.value}
            onValueChange={field.onChange}
            className={
              orientation === "horizontal"
                ? "flex flex-wrap gap-4"
                : "grid gap-3"
            }
            aria-invalid={Boolean(error)}
          >
            {options.map((option) => (
              <div key={option.value} className="flex items-start gap-2">
                <RadioGroupItem
                  value={option.value}
                  id={`${name}-${option.value}`}
                  className="mt-0.5"
                />
                <div className="grid gap-0.5">
                  <Label
                    htmlFor={`${name}-${option.value}`}
                    className="font-normal"
                  >
                    {option.label}
                  </Label>
                  {option.description ? (
                    <p className="text-muted-foreground text-xs">
                      {option.description}
                    </p>
                  ) : null}
                </div>
              </div>
            ))}
          </RadioGroup>
        )}
      />
    </FormFieldShell>
  );
}
