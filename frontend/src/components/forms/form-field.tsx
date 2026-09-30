"use client";

import type { ReactNode } from "react";

import {
  Field,
  FieldDescription,
  FieldError,
  FieldLabel,
} from "@/components/ui/field";

type FormFieldShellProps = {
  name: string;
  label?: ReactNode;
  /** Trailing control next to the label (e.g. forgot-password link). */
  labelAction?: ReactNode;
  description?: string;
  error?: string;
  children: ReactNode;
  orientation?: "vertical" | "horizontal" | "responsive";
  className?: string;
};

/** Shared label / description / error chrome for App* fields */
export function FormFieldShell({
  name,
  label,
  labelAction,
  description,
  error,
  children,
  orientation = "vertical",
  className,
}: FormFieldShellProps) {
  return (
    <Field
      data-invalid={Boolean(error) || undefined}
      orientation={orientation}
      className={className}
    >
      {label ? (
        labelAction ? (
          <div className="flex w-full items-center">
            <FieldLabel htmlFor={name}>{label}</FieldLabel>
            {labelAction}
          </div>
        ) : (
          <FieldLabel htmlFor={name}>{label}</FieldLabel>
        )
      ) : null}
      {children}
      {description && !error ? (
        <FieldDescription>{description}</FieldDescription>
      ) : null}
      {error ? <FieldError>{error}</FieldError> : null}
    </Field>
  );
}
