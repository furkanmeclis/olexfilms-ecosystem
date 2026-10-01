"use client";

import { Check, Languages } from "lucide-react";

import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import {
  LOCALE_NAMES,
  SUPPORTED_LOCALES,
  localeDir,
  type AppLocale,
} from "@/config/i18n";
import { authService } from "@/services/auth.service";
import { useAuth } from "@/providers/auth-provider";
import { useLocale } from "@/providers/locale-provider";
import { cn } from "@/lib/utils";
import { mapMeToAuthUser } from "@/lib/auth/types";

/** Every supported language, each written in its own name. */
export const localeOptions: { value: AppLocale; label: string }[] =
  SUPPORTED_LOCALES.map((value) => ({ value, label: LOCALE_NAMES[value] }));

export function LocaleSwitch() {
  const { locale, setLocale, t } = useLocale();
  const { isAuthenticated, setUser, user } = useAuth();

  const applyLocale = async (next: AppLocale) => {
    await setLocale(next);
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
        <Button
          variant="ghost"
          size="icon"
          className="rounded-full"
          data-testid="locale-switch"
        >
          <Languages className="size-[1.15rem]" />
          <span className="sr-only">{t("common.language")}</span>
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="max-h-96 overflow-y-auto">
        {localeOptions.map((option) => (
          <DropdownMenuItem
            key={option.value}
            lang={option.value}
            dir={localeDir(option.value)}
            data-locale={option.value}
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
