"use client";

import { Bot, Gauge, Loader2, Send, ShieldCheck, Square } from "lucide-react";
import Link from "next/link";
import { useEffect, useRef, useState, type KeyboardEvent } from "react";

import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

import { useAssistantChat } from "../hooks/use-assistant-chat";
import { quotaRemainingPercent } from "../lib/chat-state";
import type { AssistantTransport } from "../lib/types";
import { AssistantConsentDialog } from "./assistant-consent-dialog";
import { ConversationList } from "./conversation-list";
import { MessageList } from "./message-list";

const MAX_CHARS = 8000;

/**
 * The assistant chat shared by the panel (page and header sheet) and the
 * portal. `layout="page"` adds the conversation history column; the sheet
 * offers a new chat and a link to the full page instead.
 */
export function AssistantChat({
  transport,
  scope,
  layout = "page",
  showToolChips,
  fullPageHref,
}: {
  transport: AssistantTransport;
  /** Query-cache scope (realm + organization). */
  scope: string;
  layout?: "page" | "sheet";
  showToolChips: boolean;
  fullPageHref?: string;
}) {
  const { t, dir } = useLocale();
  const chat = useAssistantChat(transport, scope);
  const [input, setInput] = useState("");
  const bottomRef = useRef<HTMLDivElement | null>(null);

  const lastMessage = chat.messages[chat.messages.length - 1];
  const lastLength =
    lastMessage?.blocks.reduce(
      (n, b) => n + (b.type === "text" ? b.text.length : 1),
      0,
    ) ?? 0;
  useEffect(() => {
    bottomRef.current?.scrollIntoView?.({ block: "end" });
  }, [chat.messages.length, lastLength]);

  const status = chat.status.data;
  if (chat.status.isLoading) {
    return (
      <div className="text-muted-foreground flex items-center gap-2 p-6 text-sm">
        <Loader2 className="size-4 animate-spin" />
        {t("ai.chat.loading")}
      </div>
    );
  }
  if (!status || !status.enabled || !status.allowed) {
    return (
      <Alert data-testid="ai-unavailable">
        <Bot className="size-4" />
        <AlertTitle>{t("ai.unavailable.title")}</AlertTitle>
        <AlertDescription>
          {t(
            status && !status.allowed
              ? "ai.unavailable.forbidden"
              : "ai.unavailable.body",
          )}
        </AlertDescription>
      </Alert>
    );
  }

  const percent = quotaRemainingPercent(status.quota);
  const inputDisabled = chat.consentRequired || chat.quotaExceeded;

  const submit = async () => {
    const text = input.trim();
    if (!text || inputDisabled || chat.streaming) return;
    setInput("");
    const sent = await chat.send(text);
    if (!sent) setInput(text);
  };

  const onKeyDown = (e: KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === "Enter" && !e.shiftKey && !e.nativeEvent.isComposing) {
      e.preventDefault();
      void submit();
    }
  };

  const conversationList =
    layout === "page" ? (
      <aside className="hidden w-64 shrink-0 border-e pe-4 md:block">
        <ConversationList
          items={chat.conversations.data ?? []}
          activeId={chat.activeId}
          disabled={chat.streaming}
          onOpen={(uuid) => void chat.open(uuid)}
          onNew={() => void chat.open(null)}
          onRename={chat.rename}
          onDelete={chat.remove}
        />
      </aside>
    ) : null;

  return (
    <div
      dir={dir}
      className={cn(
        "flex min-h-0 flex-1 gap-4",
        layout === "page" ? "h-[calc(100svh-12rem)]" : "h-full",
      )}
      data-testid="ai-assistant-chat"
    >
      {conversationList}
      <section className="flex min-h-0 min-w-0 flex-1 flex-col gap-3">
        <div className="flex flex-wrap items-center gap-2">
          {layout === "sheet" ? (
            <Button
              variant="outline"
              size="sm"
              onClick={() => void chat.open(null)}
              disabled={chat.streaming}
            >
              {t("ai.history.new")}
            </Button>
          ) : null}
          {layout === "sheet" && fullPageHref ? (
            <Button variant="ghost" size="sm" asChild>
              <Link href={fullPageHref}>{t("ai.chat.open_full")}</Link>
            </Button>
          ) : null}
          {percent !== null ? (
            <span
              className="text-muted-foreground ms-auto inline-flex items-center gap-1 text-xs"
              data-testid="ai-quota"
            >
              <Gauge className="size-3.5" />
              {t("ai.quota.remaining", { percent })}
            </span>
          ) : null}
        </div>

        {chat.quotaExceeded ? (
          <Alert variant="destructive" data-testid="ai-quota-band">
            <Gauge className="size-4" />
            <AlertTitle>{t("ai.quota.exceeded_title")}</AlertTitle>
            <AlertDescription>{t("ai.quota.exceeded_body")}</AlertDescription>
          </Alert>
        ) : null}

        {chat.consentRequired ? (
          <Alert data-testid="ai-consent-band">
            <ShieldCheck className="size-4" />
            <AlertTitle>{t("ai.consent.band_title")}</AlertTitle>
            <AlertDescription className="flex flex-wrap items-center gap-2">
              {t("ai.consent.band_body")}
              <Button
                size="sm"
                variant="outline"
                onClick={() => chat.setConsentOpen(true)}
              >
                {t("ai.consent.review")}
              </Button>
            </AlertDescription>
          </Alert>
        ) : null}

        <div className="min-h-0 flex-1 overflow-y-auto pe-1">
          {chat.loadingConversation ? (
            <div className="text-muted-foreground flex items-center gap-2 text-sm">
              <Loader2 className="size-4 animate-spin" />
              {t("ai.chat.loading")}
            </div>
          ) : chat.messages.length === 0 ? (
            <div className="text-muted-foreground flex h-full flex-col items-center justify-center gap-2 text-center text-sm">
              <Bot className="size-8" />
              <p>{t("ai.chat.empty")}</p>
            </div>
          ) : (
            <MessageList
              messages={chat.messages}
              streaming={chat.streaming}
              showToolChips={showToolChips}
              transport={transport}
              onConfirm={(card, edits) => void chat.confirm(card, edits)}
              onCancel={(card) => void chat.cancel(card)}
              onRetry={chat.retry}
            />
          )}
          <div ref={bottomRef} />
        </div>

        <form
          className="flex items-end gap-2"
          onSubmit={(e) => {
            e.preventDefault();
            void submit();
          }}
        >
          <Textarea
            value={input}
            onChange={(e) => setInput(e.target.value)}
            onKeyDown={onKeyDown}
            placeholder={t("ai.chat.placeholder")}
            aria-label={t("ai.chat.placeholder")}
            maxLength={MAX_CHARS}
            rows={2}
            disabled={inputDisabled}
            className="max-h-40 min-h-11 resize-none"
            data-testid="ai-input"
          />
          {chat.streaming ? (
            <Button
              type="button"
              variant="outline"
              onClick={chat.stop}
              data-testid="ai-stop"
            >
              <Square className="size-4" />
              {t("ai.chat.stop")}
            </Button>
          ) : (
            <Button
              type="submit"
              disabled={inputDisabled || !input.trim()}
              data-testid="ai-send"
            >
              <Send className="size-4 rtl:-scale-x-100" />
              {t("ai.chat.send")}
            </Button>
          )}
        </form>
      </section>

      <AssistantConsentDialog
        key={`${chat.consentOpen}-${chat.consentText?.version ?? 0}`}
        text={chat.consentText}
        open={chat.consentOpen}
        onOpenChange={chat.setConsentOpen}
        onAccept={chat.acceptConsent}
      />
    </div>
  );
}
