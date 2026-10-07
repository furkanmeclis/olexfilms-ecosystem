"use client";

import {
  ArrowLeft,
  Building2,
  Lock,
  LockOpen,
  PauseCircle,
  PlayCircle,
  PowerOff,
} from "lucide-react";
import { useEffect, useMemo, useRef, useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { StatusChip } from "@/components/common/status-chip";
import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { AIRunsSheet } from "@/features/conversations/components/ai-runs-sheet";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import { permissions } from "@/config/permissions";
import { ConversationHandoffDialog } from "@/features/conversations/components/conversation-handoff-dialog";
import { MessageComposer } from "@/features/conversations/components/message-composer";
import { MessageThread } from "@/features/conversations/components/message-thread";
import {
  useConversation,
  useConversationMessages,
  useMarkConversationRead,
  usePatchConversation,
  usePlatformAdmins,
  useReplyConversation,
} from "@/features/conversations/hooks/use-conversations";
import {
  STATUS_TONE,
  conversationInitials,
  conversationTitle,
  flattenMessages,
} from "@/features/conversations/lib/conversations";
import type {
  Conversation,
  ConversationAIMode,
} from "@/features/conversations/services/conversations.service";
import { useAuth } from "@/providers/auth-provider";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

const UNASSIGNED = "__none__";

const AI_MODE_ICONS = {
  auto: PlayCircle,
  paused: PauseCircle,
  off: PowerOff,
} as const;

type ConversationPanelProps = {
  uuid: string;
  /** Row data shown until the detail request answers. */
  initial?: Conversation | null;
  onClose?: () => void;
};

export function ConversationPanel({
  uuid,
  initial,
  onClose,
}: ConversationPanelProps) {
  const { t, format } = useLocale();
  const { can } = usePermission();
  const { user } = useAuth();
  const canRead = can(permissions.conversations.read);
  const canReply = can(permissions.conversations.reply);
  const canManage = can(permissions.conversations.manage);

  const detail = useConversation(uuid);
  const conversation = detail.data ?? initial ?? null;
  const messagesQuery = useConversationMessages(uuid);
  const messages = useMemo(
    () => flattenMessages(messagesQuery.data),
    [messagesQuery.data],
  );
  const reply = useReplyConversation(uuid);
  const patch = usePatchConversation(uuid);
  const markRead = useMarkConversationRead();
  const admins = usePlatformAdmins(canManage);
  const [handoffOpen, setHandoffOpen] = useState(false);

  // Opening a conversation clears its unread counter once.
  const readFor = useRef<string | null>(null);
  const unread = conversation?.unread_count ?? 0;
  useEffect(() => {
    if (unread > 0 && readFor.current !== uuid) {
      readFor.current = uuid;
      markRead.mutate(uuid);
    }
  }, [markRead, unread, uuid]);

  if (detail.isError && !conversation) {
    return (
      <ErrorState
        title={t("common.error_generic")}
        description={t("conversations.detail_not_found")}
        onRetry={() => void detail.refetch()}
        retryLabel={t("common.retry")}
      />
    );
  }
  if (!conversation) {
    return (
      <div className="text-muted-foreground flex h-full items-center justify-center text-sm">
        {t("common.loading")}
      </div>
    );
  }

  const closed = conversation.status === "closed";
  const assigneeOptions = admins.data ?? [];
  const assignedUuid = conversation.assigned_user?.uuid ?? UNASSIGNED;
  const assigneeKnown =
    assignedUuid === UNASSIGNED ||
    assigneeOptions.some((option) => option.value === assignedUuid);

  const setAIMode = (mode: string) => {
    if (!mode || mode === conversation.ai_mode) return;
    patch.mutate({ ai_mode: mode as ConversationAIMode });
  };

  return (
    <section
      className="bg-card flex h-full min-h-0 flex-col overflow-hidden rounded-lg border"
      data-testid="conversation-panel"
      aria-label={conversationTitle(conversation)}
    >
      <header className="space-y-3 border-b p-3">
        <div className="flex items-start gap-3">
          {onClose ? (
            <Button
              type="button"
              variant="ghost"
              size="icon"
              className="shrink-0 lg:hidden"
              aria-label={t("conversations.back")}
              onClick={onClose}
            >
              <ArrowLeft className="size-4 rtl:-scale-x-100" />
            </Button>
          ) : null}
          <Avatar className="size-10 shrink-0">
            <AvatarFallback>
              {conversationInitials(conversation)}
            </AvatarFallback>
          </Avatar>
          <div className="min-w-0 flex-1">
            <h2 className="truncate font-semibold">
              {conversationTitle(conversation)}
            </h2>
            <p
              className="text-muted-foreground truncate text-xs tabular-nums"
              dir="ltr"
            >
              {conversation.contact_e164}
            </p>
            <div className="mt-1 flex flex-wrap items-center gap-1.5">
              <Badge variant="outline" className="font-normal">
                {t(`conversations.identity.${conversation.identity_kind}`)}
              </Badge>
              {conversation.identity_org ? (
                <Badge variant="secondary" className="font-normal">
                  {conversation.identity_org.name}
                </Badge>
              ) : null}
              <StatusChip
                label={t(`conversations.status_value.${conversation.status}`)}
                tone={STATUS_TONE[conversation.status]}
              />
              {conversation.assigned_org ? (
                <Badge variant="outline" className="gap-1 font-normal">
                  <Building2 className="size-3" />
                  {conversation.assigned_org.name}
                </Badge>
              ) : null}
            </div>
          </div>
        </div>

        <div className="flex flex-wrap items-center gap-2">
          <ToggleGroup
            type="single"
            variant="outline"
            size="sm"
            value={conversation.ai_mode}
            onValueChange={setAIMode}
            disabled={!canManage || patch.isPending}
            aria-label={t("conversations.ai_mode")}
            data-testid="ai-mode-toggle"
          >
            {(["auto", "paused", "off"] as const).map((mode) => {
              const Icon = AI_MODE_ICONS[mode];
              return (
                <ToggleGroupItem
                  key={mode}
                  value={mode}
                  aria-label={t(`conversations.ai_mode_value.${mode}`)}
                  data-testid={`ai-mode-${mode}`}
                  className="gap-1 px-2"
                >
                  <Icon className="size-3.5" />
                  <span className="hidden sm:inline">
                    {t(`conversations.ai_mode_value.${mode}`)}
                  </span>
                </ToggleGroupItem>
              );
            })}
          </ToggleGroup>

          {canManage ? (
            <Select
              value={assigneeKnown ? assignedUuid : undefined}
              onValueChange={(value) =>
                patch.mutate({
                  assigned_user_uuid: value === UNASSIGNED ? null : value,
                })
              }
              disabled={patch.isPending}
            >
              <SelectTrigger
                size="sm"
                className="w-44"
                aria-label={t("conversations.assigned_user")}
              >
                <SelectValue
                  placeholder={
                    conversation.assigned_user?.name ??
                    t("conversations.assign")
                  }
                />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value={UNASSIGNED}>
                  {t("conversations.unassigned")}
                </SelectItem>
                {assigneeOptions.map((option) => (
                  <SelectItem key={option.value} value={option.value}>
                    {option.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          ) : conversation.assigned_user ? (
            <span className="text-muted-foreground text-xs">
              {conversation.assigned_user.name}
            </span>
          ) : null}

          {canRead ? <AIRunsSheet conversationUuid={uuid} /> : null}

          {canManage ? (
            <>
              <Button
                type="button"
                variant="outline"
                size="sm"
                disabled={patch.isPending}
                onClick={() =>
                  patch.mutate({ status: closed ? "open" : "closed" })
                }
                data-testid="conversation-toggle-closed"
              >
                {closed ? (
                  <LockOpen className="size-4" />
                ) : (
                  <Lock className="size-4" />
                )}
                {closed
                  ? t("conversations.actions.reopen")
                  : t("conversations.actions.close")}
              </Button>
              <Button
                type="button"
                variant="outline"
                size="sm"
                onClick={() => setHandoffOpen(true)}
              >
                <Building2 className="size-4" />
                {t("conversations.handoff.action")}
              </Button>
            </>
          ) : null}
        </div>
        {conversation.ai_mode === "paused" && conversation.ai_paused_until ? (
          <p className="text-muted-foreground text-xs">
            {t("conversations.ai_paused_until", {
              time: format.dateTime(conversation.ai_paused_until),
            })}
          </p>
        ) : null}
      </header>

      <MessageThread
        conversationUuid={uuid}
        messages={messages}
        isLoading={messagesQuery.isLoading}
        hasOlder={Boolean(messagesQuery.hasNextPage)}
        isFetchingOlder={messagesQuery.isFetchingNextPage}
        onLoadOlder={() => void messagesQuery.fetchNextPage()}
      />

      {canReply ? (
        <MessageComposer
          aiPausesOnReply={conversation.ai_mode !== "off"}
          onSend={(body, file) =>
            reply.mutateAsync({
              body,
              file,
              sender: user ? { uuid: user.uuid, name: user.fullName } : null,
            })
          }
        />
      ) : null}

      {canManage ? (
        <ConversationHandoffDialog
          conversation={conversation}
          open={handoffOpen}
          onOpenChange={setHandoffOpen}
          onSubmit={(orgUuid) =>
            patch.mutateAsync({ assigned_org_uuid: orgUuid })
          }
        />
      ) : null}
    </section>
  );
}
