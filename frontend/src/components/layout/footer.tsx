"use client";

import { AppLogo } from "@/components/brand";
import { useLocale } from "@/providers/locale-provider";

export function Footer() {
  const { t } = useLocale();
  return (
    <footer className="border-border mt-auto border-t px-4 py-3 md:px-6">
      <div className="text-muted-foreground flex items-center gap-2.5 text-xs">
        <AppLogo variant="mark" className="size-3.5 opacity-80" />
        <span>{t("layout.footer")}</span>
      </div>
    </footer>
  );
}
