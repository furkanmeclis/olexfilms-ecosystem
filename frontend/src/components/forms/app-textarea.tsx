"use client";

import { useFormContext } from "react-hook-form";

import { FormFieldShell } from "@/components/forms/form-field";
import { Textarea } from "@/components/ui/textarea";
import { cn } from "@/lib/utils";

type AppTextareaProps = {
  name: string;
  label?: string;
  description?: string;
  className?: string;
} & Omit<React.TextareaHTMLAttributes<HTMLTextAreaElement>, "name">;

export function AppTextarea({
  name,
  label,
  description,
  className,
  ...props
}: AppTextareaProps) {
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
      className={className}
    >
      <Textarea
        id={name}
        className={cn(error && "border-destructive")}
        aria-invalid={Boolean(error)}
        {...register(name)}
        {...props}
      />
    </FormFieldShell>
  );
}
