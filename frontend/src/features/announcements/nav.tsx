"use client";

import { useQuery } from "@tanstack/react-query";

import { createNavAdornment } from "@/features/nav-engine";
import {
  announcementKeys,
  announcementsService,
  type LocaleCode,
} from "@/features/announcements/services/announcements.service";
import { useLocale } from "@/providers/locale-provider";

function useAnnouncementsNavAdornment() {
  const { locale } = useLocale();
  const count = useQuery({
    queryKey: announcementKeys.unreadCount(locale as LocaleCode),
    queryFn: () => announcementsService.unreadCount(locale as LocaleCode),
    staleTime: 30_000,
  });
  return {
    badges: [
      {
        kind: "count" as const,
        value: count.data?.unread_count ?? 0,
        variant: "default" as const,
        max: 99,
      },
    ],
  };
}

export const announcementsNavAdornment = createNavAdornment(
  useAnnouncementsNavAdornment,
);
