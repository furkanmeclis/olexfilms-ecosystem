"use client";

import { Eye, EyeOff, Lock } from "lucide-react";
import { useState, type ReactNode } from "react";

import { AppInput, InputGroupButton } from "@/components/forms/app-input";
import { useLocale } from "@/providers/locale-provider";

type AppPasswordProps = {
  name: string;
  label?: string;
  labelAction?: ReactNode;
  description?: string;
  placeholder?: string;
  autoComplete?: string;
  className?: string;
};

export function AppPassword({
  name,
  label,
  labelAction,
  description,
  placeholder,
  autoComplete = "current-password",
  className,
}: AppPasswordProps) {
  const { t } = useLocale();
  const [visible, setVisible] = useState(false);

  return (
    <AppInput
      name={name}
      label={label}
      labelAction={labelAction}
      description={description}
      placeholder={placeholder}
      autoComplete={autoComplete}
      type={visible ? "text" : "password"}
      startIcon={Lock}
      className={className}
      endAction={
        <InputGroupButton
          type="button"
          size="icon-xs"
          aria-label={
            visible ? t("form.hide_password") : t("form.show_password")
          }
          onClick={() => setVisible((v) => !v)}
        >
          {visible ? <EyeOff /> : <Eye />}
        </InputGroupButton>
      }
    />
  );
}
