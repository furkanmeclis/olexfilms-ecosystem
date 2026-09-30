"use client";

import { Controller, useFormContext } from "react-hook-form";

import { FormFieldShell } from "@/components/forms/form-field";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";

type Option = { label: string; value: string };

type AppSelectProps = {
  name: string;
  label?: string;
  description?: string;
  placeholder?: string;
  options: Option[];
  className?: string;
  disabled?: boolean;
};

export function AppSelect({
  name,
  label,
  description,
  placeholder,
  options,
  className,
  disabled,
}: AppSelectProps) {
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
          <Select
            value={field.value}
            onValueChange={field.onChange}
            disabled={disabled}
          >
            <SelectTrigger
              id={name}
              aria-invalid={Boolean(error)}
              className="w-full"
              disabled={disabled}
            >
              <SelectValue placeholder={placeholder} />
            </SelectTrigger>
            <SelectContent>
              {options.map((option) => (
                <SelectItem key={option.value} value={option.value}>
                  {option.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        )}
      />
    </FormFieldShell>
  );
}
