"use client";

import { ChevronRight } from "lucide-react";
import Link from "next/link";

import { useLocale } from "@/providers/locale-provider";

export type BreadcrumbItem = {
  label: string;
  href?: string;
};

export function Breadcrumb({ items }: { items: BreadcrumbItem[] }) {
  const { t } = useLocale();
  return (
    <nav
      aria-label={t("layout.breadcrumb_label")}
      className="flex items-center gap-1 text-sm"
    >
      {items.map((item, index) => {
        const last = index === items.length - 1;
        return (
          <div
            key={`${item.label}-${index}`}
            className="flex items-center gap-1"
          >
            {index > 0 ? (
              <ChevronRight className="text-muted-foreground h-3.5 w-3.5" />
            ) : null}
            {item.href && !last ? (
              <Link
                href={item.href}
                className="text-muted-foreground hover:text-foreground transition-colors"
              >
                {item.label}
              </Link>
            ) : (
              <span
                className={last ? "text-foreground" : "text-muted-foreground"}
              >
                {item.label}
              </span>
            )}
          </div>
        );
      })}
    </nav>
  );
}
