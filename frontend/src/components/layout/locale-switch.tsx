"use client";

import { Check, Languages } from "lucide-react";

import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import type { AppLocale } from "@/config/i18n";
import { authService } from "@/services/auth.service";
import { useAuth } from "@/providers/auth-provider";
import { useLocale } from "@/providers/locale-provider";
import { cn } from "@/lib/utils";
import { mapMeToAuthUser } from "@/lib/auth/types";

const options: { value: AppLocale; label: string }[] = [
  { value: "tr", label: "Türkçe" },
  { value: "en", label: "English" },
];

export function LocaleSwitch() {
  const { locale, setLocale, t } = useLocale();
  const { isAuthenticated, setUser, user } = useAuth();

  const applyLocale = async (next: AppLocale) => {
    setLocale(next);
    if (!isAuthenticated) return;
    try {
      const me = await authService.updateProfile({ locale: next });
      if (user) {
        setUser(mapMeToAuthUser(me));
      }
    } catch {
      // local preference still applied
    }
  };

  return (
    <DropdownMenu modal={false}>
      <DropdownMenuTrigger asChild>
        <Button variant="ghost" size="icon" className="rounded-full">
          <Languages className="size-[1.15rem]" />
          <span className="sr-only">{t("common.language")}</span>
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        {options.map((option) => (
          <DropdownMenuItem
            key={option.value}
            onClick={() => void applyLocale(option.value)}
          >
            {option.label}
            <Check
              size={14}
              className={cn("ms-auto", locale !== option.value && "hidden")}
            />
          </DropdownMenuItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
