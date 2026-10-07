"use client";

import { MessageCircle } from "lucide-react";
import { useRouter } from "next/navigation";
import { useState } from "react";

import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { MessageComposer } from "@/features/conversations/components/message-composer";
import { useStartConversation } from "@/features/conversations/hooks/use-conversations";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

type StartConversationButtonProps = {
  userUuid: string;
  userLabel: string;
};

/**
 * "WhatsApp'ta yaz" (TEC-399): opens the user's conversation (created when
 * missing) with a first staff message, then shows it in the inbox.
 * Platform admin only (conversations.reply, S2).
 */
export function StartConversationButton({
  userUuid,
  userLabel,
}: StartConversationButtonProps) {
  const { t } = useLocale();
  const router = useRouter();
  const { can } = usePermission();
  const start = useStartConversation();
  const [open, setOpen] = useState(false);
  const [error, setError] = useState<string | null>(null);

  if (!can(permissions.conversations.reply)) return null;

  const send = async (body: string, file: File | null) => {
    setError(null);
    try {
      const result = await start.mutateAsync({ userUuid, body, file });
      setOpen(false);
      router.push(
        routes.platform.conversations.detail(result.conversation.uuid),
      );
    } catch (err) {
      setError(
        isApiError(err) && err.code === "CONTACT_PHONE_MISSING"
          ? t("conversations.start.no_phone")
          : t("conversations.start.error"),
      );
      throw err;
    }
  };

  return (
    <>
      <Button
        type="button"
        size="sm"
        variant="outline"
        onClick={() => setOpen(true)}
        data-testid="start-conversation"
      >
        <MessageCircle className="size-4" />
        {t("conversations.start.action")}
      </Button>
      <Dialog
        open={open}
        onOpenChange={(next) => {
          setOpen(next);
          if (!next) setError(null);
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("conversations.start.title")}</DialogTitle>
            <DialogDescription>
              {t("conversations.start.description", { name: userLabel })}
            </DialogDescription>
          </DialogHeader>
          {error ? (
            <p role="alert" className="text-destructive text-sm">
              {error}
            </p>
          ) : null}
          <MessageComposer onSend={send} requireBody aiPausesOnReply={false} />
        </DialogContent>
      </Dialog>
    </>
  );
}
