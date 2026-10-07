"use client";

import {
  AlertCircle,
  Bot,
  Check,
  CheckCheck,
  Clock,
  FileText,
  Loader2,
} from "lucide-react";
import { useEffect, useLayoutEffect, useRef } from "react";

import { Button } from "@/components/ui/button";
import { messageMedia } from "@/features/conversations/lib/conversations";
import {
  conversationsService,
  type ConversationMessage,
} from "@/features/conversations/services/conversations.service";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

type MessageThreadProps = {
  conversationUuid: string;
  messages: ConversationMessage[];
  isLoading?: boolean;
  hasOlder?: boolean;
  isFetchingOlder?: boolean;
  onLoadOlder?: () => void;
};

function DeliveryTicks({ message }: { message: ConversationMessage }) {
  const { t } = useLocale();
  const label = t(`conversations.delivery.${message.status}`);
  const common = "size-3.5 shrink-0";
  let icon;
  switch (message.status) {
    case "queued":
      icon = <Clock className={common} />;
      break;
    case "sent":
      icon = <Check className={common} />;
      break;
    case "delivered":
      icon = <CheckCheck className={common} />;
      break;
    case "read":
      icon = <CheckCheck className={cn(common, "text-sky-500")} />;
      break;
    case "failed":
      icon = <AlertCircle className={cn(common, "text-destructive")} />;
      break;
    default:
      return null;
  }
  return (
    <span
      role="img"
      aria-label={label}
      title={
        message.status === "failed" && message.failure_reason
          ? `${label}: ${message.failure_reason}`
          : label
      }
      data-testid="message-status"
      data-status={message.status}
      className="inline-flex"
    >
      {icon}
    </span>
  );
}

function MessageMedia({
  conversationUuid,
  message,
}: {
  conversationUuid: string;
  message: ConversationMessage;
}) {
  const { t } = useLocale();
  const media = messageMedia(message);
  if (!media) return null;
  const optimistic = message.uuid.startsWith("optimistic-");
  if (!message.has_stored_media) {
    return (
      <div className="text-muted-foreground flex items-center gap-1.5 text-xs italic">
        <FileText className="size-3.5" />
        {optimistic
          ? (media.fileName ?? t("conversations.media.uploading"))
          : t("conversations.media.not_stored")}
      </div>
    );
  }
  const src = conversationsService.mediaUrl(conversationUuid, message.uuid);
  if (media.isImage) {
    return (
      <a href={src} target="_blank" rel="noreferrer" className="block">
        {/* eslint-disable-next-line @next/next/no-img-element -- BFF media, auth cookie */}
        <img
          src={src}
          alt={
            media.caption ?? media.fileName ?? t("conversations.media.image")
          }
          loading="lazy"
          className="max-h-64 max-w-full rounded-md object-contain"
          data-testid="message-image"
        />
      </a>
    );
  }
  return (
    <a
      href={src}
      target="_blank"
      rel="noreferrer"
      className="bg-background/60 flex items-center gap-2 rounded-md border px-2.5 py-2 text-sm hover:underline"
      data-testid="message-document"
    >
      <FileText className="size-4 shrink-0" />
      <span className="truncate">
        {media.fileName ?? t("conversations.media.document")}
      </span>
    </a>
  );
}

function MessageBubble({
  conversationUuid,
  message,
}: {
  conversationUuid: string;
  message: ConversationMessage;
}) {
  const { t, format } = useLocale();
  const sender = message.sender_type;

  if (sender === "system") {
    return (
      <div
        className="flex justify-center"
        data-testid="message"
        data-sender="system"
      >
        <div className="bg-muted text-muted-foreground max-w-[85%] rounded-full px-3 py-1 text-center text-xs">
          {message.body}
        </div>
      </div>
    );
  }

  const outgoing = sender !== "contact";
  return (
    <div
      className={cn("flex", outgoing ? "justify-end" : "justify-start")}
      data-testid="message"
      data-sender={sender}
      data-uuid={message.uuid}
    >
      <div
        className={cn(
          "flex max-w-[85%] flex-col gap-1 rounded-2xl px-3 py-2 text-sm shadow-xs sm:max-w-[75%]",
          sender === "contact" && "bg-muted rounded-ss-sm",
          sender === "staff" &&
            "bg-primary text-primary-foreground rounded-se-sm",
          sender === "ai" &&
            "rounded-se-sm border border-violet-500/30 bg-violet-500/10",
        )}
      >
        {sender !== "contact" ? (
          <span
            className={cn(
              "flex items-center gap-1 text-[11px] font-medium",
              sender === "staff"
                ? "text-primary-foreground/80"
                : "text-violet-700 dark:text-violet-300",
            )}
          >
            {sender === "ai" ? <Bot className="size-3" /> : null}
            {sender === "ai"
              ? t("conversations.sender.ai")
              : (message.sender_user?.name ?? t("conversations.sender.staff"))}
          </span>
        ) : null}
        <MessageMedia conversationUuid={conversationUuid} message={message} />
        {message.body ? (
          <p className="break-words whitespace-pre-wrap">{message.body}</p>
        ) : null}
        <span
          className={cn(
            "flex items-center justify-end gap-1 text-[11px] tabular-nums",
            sender === "staff"
              ? "text-primary-foreground/70"
              : "text-muted-foreground",
          )}
        >
          <time
            dateTime={message.created_at}
            title={format.dateTime(message.created_at)}
          >
            {format.time(message.created_at)}
          </time>
          {message.direction === "out" ? (
            <DeliveryTicks message={message} />
          ) : null}
        </span>
      </div>
    </div>
  );
}

/**
 * Timeline of one conversation (time-flow exception to the DataTable rule).
 * Oldest first; scrolling to the top loads older messages (cursor).
 */
export function MessageThread({
  conversationUuid,
  messages,
  isLoading,
  hasOlder,
  isFetchingOlder,
  onLoadOlder,
}: MessageThreadProps) {
  const { t } = useLocale();
  const scrollRef = useRef<HTMLDivElement>(null);
  const prevHeight = useRef<number | null>(null);
  const lastUuid = useRef<string | null>(null);
  const firstUuid = useRef<string | null>(null);

  // Keep the viewport stable when older messages are prepended, and stick
  // to the bottom when a new message arrives.
  useLayoutEffect(() => {
    const el = scrollRef.current;
    if (!el) return;
    const first = messages[0]?.uuid ?? null;
    const last = messages[messages.length - 1]?.uuid ?? null;
    if (prevHeight.current !== null && first !== firstUuid.current) {
      el.scrollTop += el.scrollHeight - prevHeight.current;
      prevHeight.current = null;
    } else if (last !== lastUuid.current) {
      el.scrollTop = el.scrollHeight;
    }
    firstUuid.current = first;
    lastUuid.current = last;
  }, [messages]);

  useEffect(() => {
    lastUuid.current = null;
    firstUuid.current = null;
  }, [conversationUuid]);

  const loadOlder = () => {
    if (!hasOlder || isFetchingOlder || !onLoadOlder) return;
    prevHeight.current = scrollRef.current?.scrollHeight ?? null;
    onLoadOlder();
  };

  return (
    <div
      ref={scrollRef}
      onScroll={(event) => {
        if (event.currentTarget.scrollTop < 48) loadOlder();
      }}
      className="flex min-h-0 flex-1 flex-col gap-2 overflow-y-auto px-3 py-4 sm:px-4"
      data-testid="message-thread"
      aria-live="polite"
    >
      {hasOlder ? (
        <div className="flex justify-center">
          <Button
            type="button"
            variant="ghost"
            size="sm"
            onClick={loadOlder}
            disabled={isFetchingOlder}
          >
            {isFetchingOlder ? (
              <Loader2 className="size-4 animate-spin" />
            ) : null}
            {t("conversations.thread.load_older")}
          </Button>
        </div>
      ) : null}
      {isLoading ? (
        <div className="text-muted-foreground flex flex-1 items-center justify-center text-sm">
          <Loader2 className="me-2 size-4 animate-spin" />
          {t("common.loading")}
        </div>
      ) : messages.length === 0 ? (
        <div className="text-muted-foreground flex flex-1 items-center justify-center text-sm">
          {t("conversations.thread.empty")}
        </div>
      ) : (
        messages.map((message) => (
          <MessageBubble
            key={message.uuid}
            conversationUuid={conversationUuid}
            message={message}
          />
        ))
      )}
    </div>
  );
}
