"use client";

import { MessagesSquare } from "lucide-react";

import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { useUnreadConversations } from "@/features/conversations/hooks/use-conversations";
import { useConversationsRealtime } from "@/features/conversations/hooks/use-conversations-realtime";
import {
  createNavAdornment,
  defineNavItem,
  type NavAdornment,
} from "@/features/nav-engine";
import { useLocale } from "@/providers/locale-provider";

function useConversationsNavAdornment(): NavAdornment {
  const { t } = useLocale();
  // Mounted only while the item is visible (conversations.read).
  useConversationsRealtime(true);
  const unread = useUnreadConversations(true);
  const count = unread.data ?? 0;
  return {
    badges: [
      { kind: "count", value: count, variant: "danger", hiddenWhenZero: true },
    ],
    info: unread.data
      ? {
          title: t("conversations.nav_info.title"),
          rows: [{ label: t("conversations.nav_info.unread"), value: count }],
        }
      : null,
  };
}

export const ConversationsNavAdornment = createNavAdornment(
  useConversationsNavAdornment,
);

/**
 * "Konuşmalar" (TEC-399): platform panel only. Per S2 only the platform
 * admin reads WhatsApp conversations, so tenant panels never list it.
 */
export const conversationsNavItem = defineNavItem({
  id: "conversations",
  titleKey: "conversations.nav",
  href: routes.platform.conversations.root,
  icon: MessagesSquare,
  permission: permissions.conversations.read,
  Adornment: ConversationsNavAdornment,
});
