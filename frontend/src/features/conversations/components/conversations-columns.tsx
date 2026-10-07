"use client";

import { useMemo } from "react";
import type { ColumnDef } from "@tanstack/react-table";
import { CheckCheck, Lock, LockOpen, MessageSquare } from "lucide-react";

import { StatusChip } from "@/components/common/status-chip";
import { EntityRowActions } from "@/components/entity/entity-row-actions";
import { Badge } from "@/components/ui/badge";
import { createColumn } from "@/components/tables";
import { permissions } from "@/config/permissions";
import {
  AI_MODE_TONE,
  CONVERSATION_AI_MODES,
  CONVERSATION_IDENTITY_KINDS,
  CONVERSATION_STATUSES,
  STATUS_TONE,
  conversationTitle,
} from "@/features/conversations/lib/conversations";
import type { Conversation } from "@/features/conversations/services/conversations.service";
import { useLocale } from "@/providers/locale-provider";

export type ConversationRowHandlers = {
  onOpen: (conversation: Conversation) => void;
  onMarkRead: (conversation: Conversation) => void;
  onToggleClosed: (conversation: Conversation) => void;
};

export function useConversationsColumns({
  handlers,
  assigneeOptions,
}: {
  handlers: ConversationRowHandlers;
  /** Platform admins (value = user uuid, the CSV `assigned_user_uuid`). */
  assigneeOptions: { value: string; label: string }[];
}) {
  const { t, format } = useLocale();

  return useMemo(
    () =>
      [
        createColumn<Conversation>({
          id: "contact",
          accessorFn: (row) => conversationTitle(row),
          labelKey: "conversations.contact_name",
          // Name / number are searched through the toolbar `q`.
          enableSorting: false,
          enableColumnFilter: false,
          enableHiding: false,
          gridPrimary: true,
          cell: ({ row }) => (
            <div className="flex min-w-0 flex-col">
              <span className="truncate font-medium">
                {conversationTitle(row.original)}
              </span>
              <span
                className="text-muted-foreground truncate text-xs tabular-nums"
                dir="ltr"
              >
                {row.original.contact_e164}
              </span>
            </div>
          ),
        }),
        createColumn<Conversation>({
          accessorKey: "identity_kind",
          labelKey: "conversations.identity_kind",
          enableSorting: false,
          filterVariant: "faceted",
          param: "identity_kind",
          filterOptions: CONVERSATION_IDENTITY_KINDS.map((value) => ({
            value,
            labelKey: `conversations.identity.${value}`,
            label: value,
          })),
          gridSecondary: true,
          cell: ({ row }) => (
            <Badge variant="outline" className="font-normal">
              {t(`conversations.identity.${row.original.identity_kind}`)}
            </Badge>
          ),
        }),
        createColumn<Conversation>({
          accessorKey: "status",
          labelKey: "conversations.status",
          enableSorting: false,
          filterVariant: "faceted",
          param: "status",
          filterOptions: CONVERSATION_STATUSES.map((value) => ({
            value,
            labelKey: `conversations.status_value.${value}`,
            label: value,
          })),
          cell: ({ row }) => (
            <StatusChip
              label={t(`conversations.status_value.${row.original.status}`)}
              tone={STATUS_TONE[row.original.status]}
            />
          ),
        }),
        createColumn<Conversation>({
          id: "assigned_user",
          accessorFn: (row) => row.assigned_user?.name ?? "",
          labelKey: "conversations.assigned_user",
          enableSorting: false,
          filterVariant: "faceted",
          param: "assigned_user_uuid",
          filterOptions: assigneeOptions,
          enableColumnFilter: assigneeOptions.length > 0,
          cell: ({ row }) =>
            row.original.assigned_user ? (
              <span className="text-sm">{row.original.assigned_user.name}</span>
            ) : (
              <span className="text-muted-foreground text-sm">
                {t("conversations.unassigned")}
              </span>
            ),
        }),
        createColumn<Conversation>({
          accessorKey: "ai_mode",
          labelKey: "conversations.ai_mode",
          enableSorting: false,
          filterVariant: "faceted",
          param: "ai_mode",
          filterOptions: CONVERSATION_AI_MODES.map((value) => ({
            value,
            labelKey: `conversations.ai_mode_value.${value}`,
            label: value,
          })),
          cell: ({ row }) => (
            <StatusChip
              label={t(`conversations.ai_mode_value.${row.original.ai_mode}`)}
              tone={AI_MODE_TONE[row.original.ai_mode]}
            />
          ),
        }),
        createColumn<Conversation>({
          accessorKey: "last_message_at",
          labelKey: "conversations.last_message_at",
          enableSorting: true,
          filterVariant: "date-range",
          param: "last_message",
          cell: ({ row }) =>
            row.original.last_message_at ? (
              <span
                className="text-sm tabular-nums"
                title={format.dateTime(row.original.last_message_at)}
              >
                {format.relative(row.original.last_message_at)}
              </span>
            ) : (
              <span className="text-muted-foreground text-sm">—</span>
            ),
        }),
        createColumn<Conversation>({
          accessorKey: "unread_count",
          labelKey: "conversations.unread_count",
          enableSorting: true,
          // Boolean filter → `unread=true|false`.
          filterVariant: "boolean",
          param: "unread",
          cell: ({ row }) =>
            row.original.unread_count > 0 ? (
              <Badge
                variant="danger"
                className="tabular-nums"
                data-testid="conversation-unread"
              >
                {format.number(row.original.unread_count)}
              </Badge>
            ) : (
              <span className="text-muted-foreground text-sm">0</span>
            ),
        }),
        createColumn<Conversation>({
          accessorKey: "created_at",
          labelKey: "conversations.created_at",
          enableSorting: true,
          enableColumnFilter: false,
          defaultHidden: true,
          cell: ({ row }) => (
            <span className="text-sm tabular-nums">
              {format.dateTime(row.original.created_at)}
            </span>
          ),
        }),
        createColumn<Conversation>({
          id: "actions",
          labelKey: "conversations.actions.label",
          enableSorting: false,
          enableHiding: false,
          enableResizing: false,
          enableColumnFilter: false,
          cell: ({ row }) => (
            <EntityRowActions
              actions={[
                {
                  id: "open",
                  label: t("conversations.actions.open"),
                  icon: MessageSquare,
                  onSelect: () => handlers.onOpen(row.original),
                },
                {
                  id: "read",
                  label: t("conversations.actions.mark_read"),
                  icon: CheckCheck,
                  disabled: row.original.unread_count === 0,
                  onSelect: () => handlers.onMarkRead(row.original),
                },
                {
                  id: "toggle-closed",
                  label:
                    row.original.status === "closed"
                      ? t("conversations.actions.reopen")
                      : t("conversations.actions.close"),
                  icon: row.original.status === "closed" ? LockOpen : Lock,
                  permission: permissions.conversations.manage,
                  onSelect: () => handlers.onToggleClosed(row.original),
                },
              ]}
            />
          ),
        }),
      ] as ColumnDef<Conversation, unknown>[],
    [assigneeOptions, format, handlers, t],
  );
}
