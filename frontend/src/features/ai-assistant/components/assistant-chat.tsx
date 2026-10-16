"use client";

import {
  ArrowLeft,
  Bot,
  Gauge,
  Loader2,
  Send,
  ShieldCheck,
  Square,
} from "lucide-react";
import Link from "next/link";
import { useEffect, useRef, useState, type KeyboardEvent } from "react";

import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { permissions } from "@/config/permissions";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

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
  onOpenFullPage,
}: {
  transport: AssistantTransport;
  /** Query-cache scope (realm + organization). */
  scope: string;
  layout?: "page" | "sheet";
  showToolChips: boolean;
  fullPageHref?: string;
  onOpenFullPage?: () => void;
}) {
  const { t, dir } = useLocale();
  const { can } = usePermission();
  const chat = useAssistantChat(transport, scope);
  const [input, setInput] = useState("");
  const historyRef = useRef<HTMLElement | null>(null);
  const bottomRef = useRef<HTMLDivElement | null>(null);
  const inputRef = useRef<HTMLTextAreaElement | null>(null);
  const followRef = useRef(true);
  const [historyOpen, setHistoryOpen] = useState(false);
  const focusComposer = () => inputRef.current?.focus();
  const openConversation = async (uuid: string | null) => {
    setHistoryOpen(false);
    followRef.current = true;
    await chat.open(uuid);
    focusComposer();
  };

  const lastMessage = chat.messages[chat.messages.length - 1];
  const lastLength =
    lastMessage?.blocks.reduce(
      (n, b) => n + (b.type === "text" ? b.text.length : 1),
      0,
    ) ?? 0;
  useEffect(() => {
    if (followRef.current)
      bottomRef.current?.scrollIntoView?.({ block: "end" });
  }, [chat.messages.length, lastLength, chat.loadingConversation]);

  useEffect(() => {
    if (historyOpen) historyRef.current?.querySelector("button")?.focus();
    if (!historyOpen && chat.status.data?.allowed && !chat.consentRequired) {
      inputRef.current?.focus();
    }
  }, [historyOpen, chat.status.data?.allowed, chat.consentRequired]);

  const status = chat.status.data;
  if (chat.status.isLoading) {
    return (
      <div className="text-muted-foreground flex items-center gap-2 p-6 text-sm">
        <Loader2 className="size-4 animate-spin" />
        {t("ai.chat.loading")}
      </div>
    );
  }
  if (chat.status.isError) {
    return (
      <Alert variant="destructive" role="alert">
        <AlertTitle>{t("ai.errors.generic")}</AlertTitle>
        <Button variant="outline" onClick={() => void chat.status.refetch()}>
          {t("ai.chat.retry")}
        </Button>
      </Alert>
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
  const inputDisabled =
    chat.consentRequired ||
    chat.quotaExceeded ||
    chat.loadingConversation ||
    chat.conversationError;
  const canConfirmActions =
    transport.realm !== "panel" || can(permissions.ai.actionsConfirm);

  const submit = async () => {
    const text = input.trim();
    if (!text || inputDisabled || chat.streaming) return;
    followRef.current = true;
    setInput("");
    focusComposer();
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
      <aside
        ref={historyRef}
        aria-label={t("ai.history.title")}
        className={cn(
          "min-h-0 w-full shrink-0 md:block md:w-64 md:border-e md:pe-4",
          !historyOpen && "hidden",
        )}
      >
        <Button
          variant="ghost"
          className="mb-2 md:hidden"
          onClick={() => {
            setHistoryOpen(false);
            focusComposer();
          }}
        >
          <ArrowLeft className="size-4 rtl:-scale-x-100" />
          {t("ai.chat.back")}
        </Button>
        {chat.conversations.isError ? (
          <Button
            variant="outline"
            onClick={() => void chat.conversations.refetch()}
          >
            {t("ai.chat.retry")}
          </Button>
        ) : null}
        <ConversationList
          items={chat.conversations.data ?? []}
          activeId={chat.activeId}
          disabled={chat.streaming}
          onOpen={(uuid) => void openConversation(uuid)}
          onNew={() => void openConversation(null)}
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
      <section
        className={cn(
          "min-h-0 min-w-0 flex-1 flex-col gap-3 md:flex",
          layout === "page" && historyOpen ? "hidden" : "flex",
        )}
      >
        <div
          className="sr-only"
          role="status"
          aria-live="polite"
          aria-atomic="true"
        >
          {chat.streaming
            ? t("ai.chat.thinking")
            : lastMessage?.status === "cancelled"
              ? t("ai.chat.stopped")
              : ""}
        </div>
        <div className="flex flex-wrap items-center gap-2">
          {layout === "page" ? (
            <Button
              variant="ghost"
              size="sm"
              className="md:hidden"
              onClick={() => setHistoryOpen(true)}
            >
              <ArrowLeft className="size-4 rtl:-scale-x-100" />
              {t("ai.history.title")}
            </Button>
          ) : null}
          {layout === "sheet" ? (
            <Button
              variant="outline"
              size="sm"
              onClick={() => void openConversation(null)}
              disabled={chat.streaming}
            >
              {t("ai.history.new")}
            </Button>
          ) : null}
          {layout === "sheet" && fullPageHref ? (
            <Button variant="ghost" size="sm" asChild>
              <Link href={fullPageHref} onClick={onOpenFullPage}>
                {t("ai.chat.open_full")}
              </Link>
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

        <div
          className="min-h-0 flex-1 overflow-y-auto overscroll-contain pe-1"
          data-testid="ai-scroll"
          onScroll={(event) => {
            const el = event.currentTarget;
            followRef.current =
              el.scrollHeight - el.scrollTop - el.clientHeight < 64;
          }}
        >
          {chat.loadingConversation ? (
            <div className="text-muted-foreground flex items-center gap-2 text-sm">
              <Loader2 className="size-4 animate-spin" />
              {t("ai.chat.loading")}
            </div>
          ) : chat.conversationError ? (
            <Alert variant="destructive" role="alert">
              <AlertTitle>{t("ai.errors.generic")}</AlertTitle>
              <Button
                variant="outline"
                onClick={() => void openConversation(chat.activeId)}
              >
                {t("ai.chat.retry")}
              </Button>
            </Alert>
          ) : chat.messages.length === 0 ? (
            <div className="text-muted-foreground flex h-full flex-col items-center justify-center gap-2 text-center text-sm">
              <Bot className="size-8" />
              <p>{t("ai.chat.empty")}</p>
              <div className="mt-2 flex max-w-md flex-wrap justify-center gap-2">
                {["first", "second", "third"].map((suggestion) => {
                  const text = t(
                    `ai.suggestions.${transport.realm}.${suggestion}`,
                  );
                  return (
                    <Button
                      key={suggestion}
                      type="button"
                      variant="outline"
                      className="h-auto text-start whitespace-normal"
                      disabled={inputDisabled}
                      onClick={() => {
                        setInput(text);
                        focusComposer();
                      }}
                      data-testid="ai-suggestion"
                    >
                      {text}
                    </Button>
                  );
                })}
              </div>
            </div>
          ) : (
            <MessageList
              messages={chat.messages}
              streaming={chat.streaming}
              showToolChips={showToolChips}
              canConfirmActions={canConfirmActions}
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
            ref={inputRef}
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
