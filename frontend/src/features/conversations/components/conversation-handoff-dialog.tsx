"use client";

import { useCallback, useState } from "react";

import {
  AsyncCombobox,
  type ComboboxOption,
} from "@/components/ui/async-combobox";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { organizationsService } from "@/features/organizations/services/organizations.service";
import type { Conversation } from "@/features/conversations/services/conversations.service";
import { useLocale } from "@/providers/locale-provider";

type ConversationHandoffDialogProps = {
  conversation: Conversation;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** `null` gives the conversation back to the center. */
  onSubmit: (orgUuid: string | null) => Promise<unknown>;
};

/** "Bayiye devret": hands the conversation to a dealer organization. */
export function ConversationHandoffDialog({
  conversation,
  open,
  onOpenChange,
  onSubmit,
}: ConversationHandoffDialogProps) {
  const { t } = useLocale();
  const [orgUuid, setOrgUuid] = useState("");
  const [saving, setSaving] = useState(false);

  const loadOptions = useCallback(async (query: string) => {
    const result = await organizationsService.list({
      limit: 20,
      offset: 0,
      q: query.trim() || undefined,
      type: "dealer",
      status: "active",
    });
    return result.items.map((org): ComboboxOption => ({
      value: org.uuid,
      label: org.name,
    }));
  }, []);

  const run = async (value: string | null) => {
    setSaving(true);
    try {
      await onSubmit(value);
      onOpenChange(false);
      setOrgUuid("");
    } catch {
      // Error toast comes from the global API error handler.
    } finally {
      setSaving(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t("conversations.handoff.title")}</DialogTitle>
          <DialogDescription>
            {t("conversations.handoff.description")}
          </DialogDescription>
        </DialogHeader>
        {conversation.assigned_org ? (
          <p className="text-sm">
            {t("conversations.handoff.current", {
              name: conversation.assigned_org.name,
            })}
          </p>
        ) : null}
        <AsyncCombobox
          value={orgUuid}
          onValueChange={setOrgUuid}
          loadOptions={loadOptions}
          placeholder={t("conversations.handoff.placeholder")}
          searchPlaceholder={t("conversations.handoff.search")}
          emptyText={t("conversations.handoff.empty")}
        />
        <DialogFooter className="gap-2">
          {conversation.assigned_org ? (
            <Button
              type="button"
              variant="outline"
              disabled={saving}
              onClick={() => void run(null)}
            >
              {t("conversations.handoff.back_to_center")}
            </Button>
          ) : null}
          <Button
            type="button"
            disabled={!orgUuid || saving}
            onClick={() => void run(orgUuid)}
          >
            {t("conversations.handoff.submit")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
