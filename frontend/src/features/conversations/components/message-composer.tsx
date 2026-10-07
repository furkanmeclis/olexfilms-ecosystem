"use client";

import { Info, Paperclip, Send, X } from "lucide-react";
import { useRef, useState, type KeyboardEvent } from "react";

import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import {
  ATTACHMENT_ACCEPT,
  STAFF_REPLY_AI_PAUSE_MINUTES,
  validateAttachment,
  type AttachmentError,
} from "@/features/conversations/lib/conversations";
import { useLocale } from "@/providers/locale-provider";

type MessageComposerProps = {
  /** Resolves when the reply was accepted; rejects to keep the draft. */
  onSend: (body: string, file: File | null) => Promise<unknown>;
  disabled?: boolean;
  /** Show the "AI paused for 30 minutes" note after a reply. */
  aiPausesOnReply?: boolean;
  /** Required text (start dialog); replies may send only a file. */
  requireBody?: boolean;
};

export function MessageComposer({
  onSend,
  disabled,
  aiPausesOnReply = true,
  requireBody = false,
}: MessageComposerProps) {
  const { t, format } = useLocale();
  const inputRef = useRef<HTMLInputElement>(null);
  const [body, setBody] = useState("");
  const [file, setFile] = useState<File | null>(null);
  const [fileError, setFileError] = useState<AttachmentError | null>(null);
  const [sending, setSending] = useState(false);
  const [pausedNotice, setPausedNotice] = useState(false);

  const trimmed = body.trim();
  const canSend =
    !disabled &&
    !sending &&
    (requireBody ? Boolean(trimmed) : Boolean(trimmed || file));

  const pickFile = (picked: File | undefined) => {
    if (inputRef.current) inputRef.current.value = "";
    if (!picked) return;
    const error = validateAttachment(picked);
    setFileError(error);
    setFile(error ? null : picked);
  };

  const submit = async () => {
    if (!canSend) return;
    const text = trimmed;
    const attachment = file;
    setSending(true);
    setBody("");
    setFile(null);
    try {
      await onSend(text, attachment);
      setPausedNotice(aiPausesOnReply);
    } catch {
      setBody(text);
      setFile(attachment);
    } finally {
      setSending(false);
    }
  };

  const onKeyDown = (event: KeyboardEvent<HTMLTextAreaElement>) => {
    if (
      event.key === "Enter" &&
      !event.shiftKey &&
      !event.nativeEvent.isComposing
    ) {
      event.preventDefault();
      void submit();
    }
  };

  return (
    <div className="space-y-2 border-t p-3" data-testid="message-composer">
      {pausedNotice ? (
        <p
          className="text-muted-foreground flex items-center gap-1.5 text-xs"
          data-testid="ai-paused-notice"
        >
          <Info className="size-3.5 shrink-0" />
          {t("conversations.composer.ai_paused", {
            minutes: STAFF_REPLY_AI_PAUSE_MINUTES,
          })}
        </p>
      ) : null}
      {fileError ? (
        <p
          role="alert"
          className="text-destructive text-xs"
          data-testid="attachment-error"
        >
          {fileError === "too_large"
            ? t("conversations.composer.file_too_large", { size: 16 })
            : t("conversations.composer.file_type")}
        </p>
      ) : null}
      {file ? (
        <div className="bg-muted flex w-fit max-w-full items-center gap-2 rounded-md px-2 py-1 text-xs">
          <Paperclip className="size-3.5 shrink-0" />
          <span className="truncate" data-testid="attachment-name">
            {file.name}
          </span>
          <span className="text-muted-foreground tabular-nums">
            {format.number(file.size / (1024 * 1024), {
              maximumFractionDigits: 1,
            })}{" "}
            MB
          </span>
          <Button
            type="button"
            variant="ghost"
            size="icon"
            className="size-5"
            aria-label={t("conversations.composer.remove_file")}
            onClick={() => setFile(null)}
          >
            <X className="size-3" />
          </Button>
        </div>
      ) : null}
      <div className="flex items-end gap-2">
        <input
          ref={inputRef}
          type="file"
          accept={ATTACHMENT_ACCEPT}
          className="hidden"
          data-testid="attachment-input"
          onChange={(event) => pickFile(event.target.files?.[0])}
        />
        <Button
          type="button"
          variant="outline"
          size="icon"
          aria-label={t("conversations.composer.attach")}
          title={t("conversations.composer.attach_hint", { size: 16 })}
          disabled={disabled || sending}
          onClick={() => inputRef.current?.click()}
        >
          <Paperclip className="size-4" />
        </Button>
        <Textarea
          value={body}
          onChange={(event) => setBody(event.target.value)}
          onKeyDown={onKeyDown}
          placeholder={t("conversations.composer.placeholder")}
          aria-label={t("conversations.composer.placeholder")}
          maxLength={4096}
          rows={2}
          disabled={disabled}
          className="max-h-40 min-h-10 flex-1 resize-none"
          data-testid="composer-input"
        />
        <Button
          type="button"
          size="icon"
          aria-label={t("conversations.composer.send")}
          disabled={!canSend}
          onClick={() => void submit()}
          data-testid="composer-send"
        >
          <Send className="size-4 rtl:-scale-x-100" />
        </Button>
      </div>
    </div>
  );
}
