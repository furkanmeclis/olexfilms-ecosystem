import { CircleAlert, Clock3, SearchX } from "lucide-react";
import Link from "next/link";

import { buttonVariants } from "@/components/ui/button";
import type { AppLocale } from "@/config/i18n";
import type { ShortUrlUnavailableReason } from "@/features/short-urls/lib/short-url";
import { translate } from "@/lib/i18n/messages";

const ICONS = {
  expired: Clock3,
  not_found: SearchX,
  error: CircleAlert,
} as const;

/**
 * Body of `/link-unavailable` (TEC-249). Server rendered, no session;
 * logical properties only (RTL for ar).
 */
export function ShortUrlUnavailable({
  reason,
  locale,
  dir,
}: {
  reason: ShortUrlUnavailableReason;
  locale: AppLocale;
  dir: "ltr" | "rtl";
}) {
  const t = (key: string) => translate(locale, `common.short_link.${key}`);
  const Icon = ICONS[reason];
  return (
    <main
      lang={locale}
      dir={dir}
      data-slot="short-url-unavailable"
      className="bg-muted/30 text-foreground flex min-h-dvh items-center px-4 py-6"
    >
      <section
        data-reason={reason}
        role={reason === "error" ? "alert" : undefined}
        className="bg-card mx-auto flex w-full max-w-md flex-col items-center gap-3 rounded-2xl border p-6 text-center shadow-sm"
      >
        <span className="bg-muted text-muted-foreground flex size-14 items-center justify-center rounded-full">
          <Icon className="size-8" aria-hidden />
        </span>
        <h1 className="text-lg font-semibold">{t(`${reason}_title`)}</h1>
        <p className="text-muted-foreground text-sm">{t(`${reason}_body`)}</p>
        <Link href="/" className={buttonVariants({ variant: "outline" })}>
          {t("home")}
        </Link>
      </section>
    </main>
  );
}
