"use client";

import { Warehouse as WarehouseIcon } from "lucide-react";
import Link from "next/link";

import { Card, CardContent } from "@/components/ui/card";
import {
  useWarehouseAccess,
  WarehouseShell,
} from "@/features/warehouse/components/warehouse-shell";
import { visibleSections } from "@/features/warehouse/lib/sections";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

/**
 * Warehouse hub (TEC-231): one card per screen of the section registry.
 * Screens of the next slice (TEC-232: transfers, counts, end of day) are
 * listed as planned until their routes land.
 */
export function WarehouseHomePage({ slug }: { slug: string }) {
  const { t } = useLocale();
  const access = useWarehouseAccess(slug);
  const sections = visibleSections(access.can, access.orgType);

  return (
    <WarehouseShell
      slug={slug}
      access={access}
      title={t("warehouse.page.title")}
      description={t("warehouse.page.description")}
      icon={<WarehouseIcon className="size-6" />}
    >
      <div
        className="grid gap-4 sm:grid-cols-2 xl:grid-cols-3"
        data-testid="warehouse-sections"
      >
        {sections.map((s) => {
          const Icon = s.icon;
          const body = (
            <Card
              className={cn(
                "h-full transition-colors",
                s.href ? "hover:bg-accent/50" : "opacity-60",
              )}
            >
              <CardContent className="flex gap-3 pt-6">
                <Icon className="text-muted-foreground mt-0.5 size-5 shrink-0" />
                <div className="space-y-1">
                  <p className="font-medium">{t(`${s.key}.title`)}</p>
                  <p className="text-muted-foreground text-sm">
                    {t(`${s.key}.description`)}
                  </p>
                  {s.href ? null : (
                    <p className="text-muted-foreground text-xs">
                      {t("warehouse.sections.planned")}
                    </p>
                  )}
                </div>
              </CardContent>
            </Card>
          );
          return s.href ? (
            <Link
              key={s.id}
              href={s.href(slug)}
              data-testid="warehouse-section"
              data-section={s.id}
            >
              {body}
            </Link>
          ) : (
            <div
              key={s.id}
              aria-disabled
              data-testid="warehouse-section"
              data-section={s.id}
            >
              {body}
            </div>
          );
        })}
      </div>
    </WarehouseShell>
  );
}
