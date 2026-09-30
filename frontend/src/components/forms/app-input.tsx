"use client";

import type { LucideIcon } from "lucide-react";
import type { ReactNode } from "react";
import { useFormContext } from "react-hook-form";

import { FormFieldShell } from "@/components/forms/form-field";
import {
  InputGroup,
  InputGroupAddon,
  InputGroupButton,
  InputGroupInput,
} from "@/components/ui/input-group";
import { Input } from "@/components/ui/input";
import { cn } from "@/lib/utils";

type AppInputProps = {
  name: string;
  label?: string;
  labelAction?: ReactNode;
  description?: string;
  /** Leading Lucide icon — preferred over raw adornment */
  startIcon?: LucideIcon;
  /** Trailing Lucide icon */
  endIcon?: LucideIcon;
  /** Custom trailing control (e.g. clear / reveal password) */
  endAction?: ReactNode;
  className?: string;
} & Omit<React.InputHTMLAttributes<HTMLInputElement>, "name">;

export function AppInput({
  name,
  label,
  labelAction,
  description,
  startIcon: StartIcon,
  endIcon: EndIcon,
  endAction,
  className,
  ...props
}: AppInputProps) {
  const {
    register,
    formState: { errors },
  } = useFormContext();
  const error = errors[name]?.message as string | undefined;
  const hasAddon = Boolean(StartIcon || EndIcon || endAction);

  return (
    <FormFieldShell
      name={name}
      label={label}
      labelAction={labelAction}
      description={description}
      error={error}
      className={className}
    >
      {hasAddon ? (
        <InputGroup
          className={cn(error && "border-destructive")}
          data-disabled={props.disabled ? true : undefined}
        >
          {StartIcon ? (
            <InputGroupAddon align="inline-start">
              <StartIcon className="text-muted-foreground size-4" aria-hidden />
            </InputGroupAddon>
          ) : null}
          <InputGroupInput
            id={name}
            aria-invalid={Boolean(error)}
            {...register(name)}
            {...props}
          />
          {EndIcon || endAction ? (
            <InputGroupAddon align="inline-end">
              {endAction ??
                (EndIcon ? (
                  <EndIcon
                    className="text-muted-foreground size-4"
                    aria-hidden
                  />
                ) : null)}
            </InputGroupAddon>
          ) : null}
        </InputGroup>
      ) : (
        <Input
          id={name}
          className={cn(error && "border-destructive")}
          aria-invalid={Boolean(error)}
          {...register(name)}
          {...props}
        />
      )}
    </FormFieldShell>
  );
}

export { InputGroupButton };
