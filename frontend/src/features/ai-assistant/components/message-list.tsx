"use client";

import { AlertTriangle, Loader2, RotateCcw, Search } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Markdown } from "@/features/portal/lib/markdown";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

import { toolLabel } from "../lib/labels";
import type {
  AIActionCard,
  AssistantTransport,
  ChatBlock,
  ChatMessage,
} from "../lib/types";
import { ActionConfirmCard, ActionOutcome } from "./action-card";

/** Text of an error block: the server message, else the code's label. */
function errorText(
  t: (key: string) => string,
  block: Extract<ChatBlock, { type: "error" }>,
) {
  const known = [
    "provider_error",
    "turn_limit",
    "refusal",
    "max_tokens",
    "quota_exceeded",
    "network",
  ];
  if (known.includes(block.code)) return t(`ai.errors.${block.code}`);
  return block.message || t("ai.errors.generic");
}

export function MessageList({
  messages,
  streaming,
  showToolChips,
  transport,
  onConfirm,
  onCancel,
  onRetry,
}: {
  messages: ChatMessage[];
  streaming: boolean;
  /** Panel shows "searching…" chips; the portal hides tool activity. */
  showToolChips: boolean;
  transport: AssistantTransport;
  onConfirm: (card: AIActionCard, edits?: Record<string, unknown>) => void;
  onCancel: (card: AIActionCard) => void;
  onRetry: () => void;
}) {
  const { t } = useLocale();
  const lastId = messages[messages.length - 1]?.id;

  return (
    <ol className="space-y-4" data-testid="ai-messages">
      {messages.map((m) => {
        const mine = m.role === "user";
        const visible = m.blocks.filter(
          (b) => showToolChips || b.type !== "tool",
        );
        const waiting =
          m.role === "assistant" &&
          m.status === "pending" &&
          !visible.some((b) => b.type === "text");
        return (
          <li
            key={m.id}
            className={cn("flex", mine ? "justify-end" : "justify-start")}
            data-testid={`ai-message-${m.role}`}
          >
            <div
              className={cn(
                "max-w-[85%] min-w-0 space-y-2 rounded-2xl px-4 py-2.5",
                mine
                  ? "bg-primary text-primary-foreground rounded-ee-sm"
                  : "bg-muted rounded-es-sm",
              )}
            >
              {visible.map((b, i) => {
                switch (b.type) {
                  case "text":
                    return mine ? (
                      <p key={i} className="text-sm whitespace-pre-wrap">
                        {b.text}
                      </p>
                    ) : (
                      <div key={i} data-testid="ai-text">
                        <Markdown source={b.text} />
                      </div>
                    );
                  case "tool":
                    return (
                      <span
                        key={i}
                        className="bg-background text-muted-foreground inline-flex items-center gap-1.5 rounded-full border px-2.5 py-0.5 text-xs"
                        data-testid="ai-tool-chip"
                      >
                        {b.status === "running" ? (
                          <Loader2 className="size-3 animate-spin" />
                        ) : (
                          <Search className="size-3" />
                        )}
                        {t("ai.chat.tool_running", {
                          tool: toolLabel(t, b.name),
                        })}
                      </span>
                    );
                  case "confirm":
                    return (
                      <ActionConfirmCard
                        key={b.card.action_uuid}
                        card={b.card}
                        disabled={streaming}
                        onConfirm={(edits) => onConfirm(b.card, edits)}
                        onCancel={() => onCancel(b.card)}
                      />
                    );
                  case "action":
                    return (
                      <ActionOutcome
                        key={i}
                        action={b.action}
                        href={
                          b.action.link
                            ? transport.recordHref(
                                b.action.link.kind,
                                b.action.link.uuid,
                              )
                            : null
                        }
                      />
                    );
                  case "error":
                    return (
                      <div
                        key={i}
                        className="text-destructive flex flex-wrap items-center gap-2 text-sm"
                        data-testid="ai-error"
                      >
                        <AlertTriangle className="size-4" />
                        <span>{errorText(t, b)}</span>
                        {m.id === lastId && !streaming ? (
                          <Button
                            size="sm"
                            variant="outline"
                            onClick={onRetry}
                            data-testid="ai-retry"
                          >
                            <RotateCcw className="size-3" />
                            {t("ai.chat.retry")}
                          </Button>
                        ) : null}
                      </div>
                    );
                  default:
                    return null;
                }
              })}
              {waiting ? (
                <span className="text-muted-foreground inline-flex items-center gap-2 text-sm">
                  <Loader2 className="size-4 animate-spin" />
                  {t("ai.chat.thinking")}
                </span>
              ) : null}
              {m.status === "cancelled" ? (
                <p className="text-muted-foreground text-xs italic">
                  {t("ai.chat.stopped")}
                </p>
              ) : null}
            </div>
          </li>
        );
      })}
    </ol>
  );
}
