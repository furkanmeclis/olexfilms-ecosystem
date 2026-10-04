"use client";

import { useQuery } from "@tanstack/react-query";
import { Megaphone, Pin } from "lucide-react";
import Link from "next/link";

import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { routes } from "@/config/routes";
import {
  ANNOUNCEMENT_DASHBOARD_LIMIT,
  sortedAnnouncements,
} from "@/features/announcements/lib/announcements";
import {
  announcementKeys,
  announcementsService,
  type LocaleCode,
} from "@/features/announcements/services/announcements.service";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

export function RecentAnnouncementsWidget({ slug }: { slug: string }) {
  const { t, locale, format } = useLocale();
  const query = {
    locale: locale as LocaleCode,
    limit: ANNOUNCEMENT_DASHBOARD_LIMIT,
    offset: 0,
  };
  const list = useQuery({
    queryKey: announcementKeys.list(query),
    queryFn: () => announcementsService.list(query),
  });
  const items = sortedAnnouncements(list.data?.items ?? []).slice(
    0,
    ANNOUNCEMENT_DASHBOARD_LIMIT,
  );

  return (
    <Card>
      <CardHeader className="flex flex-row items-center justify-between gap-3">
        <CardTitle className="flex items-center gap-2">
          <Megaphone className="size-4" />
          {t("announcements.dashboard.title")}
        </CardTitle>
        <Link
          href={routes.tenant.announcements.list(slug)}
          className="text-primary text-xs font-medium hover:underline"
        >
          {t("announcements.dashboard.all")}
        </Link>
      </CardHeader>
      <CardContent>
        {items.length === 0 ? (
          <p className="text-muted-foreground text-sm">
            {list.isLoading
              ? t("announcements.detail.loading")
              : t("announcements.dashboard.empty")}
          </p>
        ) : (
          <ul className="divide-y" data-testid="recent-announcements">
            {items.map((item) => (
              <li key={item.uuid} className="py-3 first:pt-0 last:pb-0">
                <Link
                  href={routes.tenant.announcements.list(slug)}
                  className={cn(
                    "block rounded-sm focus-visible:ring-2 focus-visible:outline-none",
                    !item.read_at && "font-medium",
                  )}
                >
                  <span className="flex items-center gap-2 text-sm">
                    {item.pinned ? <Pin className="size-3.5" /> : null}
                    <span className="line-clamp-1">{item.title}</span>
                  </span>
                  <span className="text-muted-foreground mt-1 line-clamp-1 text-xs">
                    {item.publish_at
                      ? format.dateTime(item.publish_at)
                      : item.body}
                  </span>
                </Link>
              </li>
            ))}
          </ul>
        )}
      </CardContent>
    </Card>
  );
}
