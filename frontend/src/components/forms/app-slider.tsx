"use client";

import { Controller, useFormContext } from "react-hook-form";

import { FormFieldShell } from "@/components/forms/form-field";
import { Slider } from "@/components/ui/slider";

type AppSliderProps = {
  name: string;
  label?: string;
  description?: string;
  min?: number;
  max?: number;
  step?: number;
  showValue?: boolean;
  className?: string;
  formatValue?: (value: number) => string;
};

export function AppSlider({
  name,
  label,
  description,
  min = 0,
  max = 100,
  step = 1,
  showValue = true,
  className,
  formatValue = (v) => String(v),
}: AppSliderProps) {
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
        render={({ field }) => {
          const value = Number(field.value ?? min);
          return (
            <div className="space-y-3 pt-1">
              <Slider
                id={name}
                min={min}
                max={max}
                step={step}
                value={[value]}
                onValueChange={(vals) => field.onChange(vals[0] ?? min)}
                aria-invalid={Boolean(error)}
              />
              {showValue ? (
                <p className="text-muted-foreground text-sm tabular-nums">
                  {formatValue(value)}
                </p>
              ) : null}
            </div>
          );
        }}
      />
    </FormFieldShell>
  );
}
