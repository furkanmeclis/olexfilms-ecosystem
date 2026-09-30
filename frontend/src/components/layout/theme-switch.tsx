"use client";

import { Moon, Sun } from "lucide-react";
import { useTheme } from "next-themes";
import { useEffect } from "react";

import { Button } from "@/components/ui/button";
import { brand } from "@/config/brand";
import { useLocale } from "@/providers/locale-provider";

export function ThemeSwitch() {
  const { theme, setTheme, resolvedTheme } = useTheme();
  const { t } = useLocale();

  useEffect(() => {
    const color =
      resolvedTheme === "dark" ? brand.colors.ink : brand.colors.mist;
    document
      .querySelector("meta[name='theme-color']")
      ?.setAttribute("content", color);
  }, [resolvedTheme]);

  useEffect(() => {
    if (theme === "system") {
      setTheme(resolvedTheme === "dark" ? "dark" : "light");
    }
  }, [resolvedTheme, setTheme, theme]);

  return (
    <Button
      type="button"
      variant="ghost"
      size="icon"
      className="relative scale-95 rounded-full"
      aria-label={t("common.theme")}
      onClick={() => setTheme(resolvedTheme === "dark" ? "light" : "dark")}
    >
      <Sun className="size-[1.2rem] scale-100 rotate-0 transition-all dark:scale-0 dark:-rotate-90" />
      <Moon className="absolute size-[1.2rem] scale-0 rotate-90 transition-all dark:scale-100 dark:rotate-0" />
      <span className="sr-only">{t("common.theme")}</span>
    </Button>
  );
}
