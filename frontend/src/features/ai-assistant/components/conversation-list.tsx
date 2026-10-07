"use client";

import { Check, MessageSquarePlus, Pencil, Trash2, X } from "lucide-react";
import { useState } from "react";

import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

import type { AIConversation } from "../lib/types";

/**
 * Last 50 conversations, newest first (a time-ordered history, not a record
 * list: DataTable exception). Rename inline, delete after a confirmation.
 */
export function ConversationList({
  items,
  activeId,
  disabled,
  onOpen,
  onNew,
  onRename,
  onDelete,
}: {
  items: AIConversation[];
  activeId: string | null;
  disabled?: boolean;
  onOpen: (uuid: string) => void;
  onNew: () => void;
  onRename: (uuid: string, title: string) => Promise<void>;
  onDelete: (uuid: string) => Promise<void>;
}) {
  const { t, format } = useLocale();
  const [editing, setEditing] = useState<string | null>(null);
  const [draft, setDraft] = useState("");
  const [deleting, setDeleting] = useState<AIConversation | null>(null);

  const save = async (uuid: string) => {
    const title = draft.trim();
    if (title) await onRename(uuid, title).catch(() => undefined);
    setEditing(null);
  };

  return (
    <div className="flex h-full min-h-0 flex-col gap-2">
      <Button
        variant="outline"
        className="justify-start"
        onClick={onNew}
        disabled={disabled}
        data-testid="ai-new-conversation"
      >
        <MessageSquarePlus className="size-4" />
        {t("ai.history.new")}
      </Button>
      <p className="text-muted-foreground px-1 text-xs font-medium">
        {t("ai.history.title")}
      </p>
      {items.length === 0 ? (
        <p className="text-muted-foreground px-1 text-sm">
          {t("ai.history.empty")}
        </p>
      ) : null}
      <ul className="min-h-0 flex-1 space-y-1 overflow-y-auto">
        {items.map((c) => (
          <li key={c.uuid} className="group">
            {editing === c.uuid ? (
              <form
                className="flex items-center gap-1"
                onSubmit={(e) => {
                  e.preventDefault();
                  void save(c.uuid);
                }}
              >
                <Input
                  autoFocus
                  value={draft}
                  maxLength={200}
                  onChange={(e) => setDraft(e.target.value)}
                  aria-label={t("ai.history.rename")}
                  className="h-8"
                />
                <Button
                  type="submit"
                  size="icon"
                  variant="ghost"
                  className="size-8"
                  aria-label={t("ai.history.save")}
                >
                  <Check className="size-4" />
                </Button>
                <Button
                  type="button"
                  size="icon"
                  variant="ghost"
                  className="size-8"
                  onClick={() => setEditing(null)}
                  aria-label={t("ai.history.cancel")}
                >
                  <X className="size-4" />
                </Button>
              </form>
            ) : (
              <div
                className={cn(
                  "hover:bg-muted flex items-center gap-1 rounded-md pe-1",
                  activeId === c.uuid && "bg-muted",
                )}
              >
                <button
                  type="button"
                  className="min-w-0 flex-1 px-2 py-1.5 text-start disabled:opacity-50"
                  onClick={() => onOpen(c.uuid)}
                  disabled={disabled}
                  aria-current={activeId === c.uuid ? "true" : undefined}
                >
                  <span className="block truncate text-sm">
                    {c.title || t("ai.history.untitled")}
                  </span>
                  <span className="text-muted-foreground block text-xs">
                    {format.dateTime(c.last_message_at ?? c.updated_at)}
                  </span>
                </button>
                <Button
                  size="icon"
                  variant="ghost"
                  className="size-7 opacity-0 group-hover:opacity-100 focus-visible:opacity-100"
                  onClick={() => {
                    setDraft(c.title);
                    setEditing(c.uuid);
                  }}
                  aria-label={t("ai.history.rename")}
                >
                  <Pencil className="size-3.5" />
                </Button>
                <Button
                  size="icon"
                  variant="ghost"
                  className="size-7 opacity-0 group-hover:opacity-100 focus-visible:opacity-100"
                  onClick={() => setDeleting(c)}
                  aria-label={t("ai.history.delete")}
                >
                  <Trash2 className="size-3.5" />
                </Button>
              </div>
            )}
          </li>
        ))}
      </ul>

      <Dialog
        open={deleting !== null}
        onOpenChange={(open) => !open && setDeleting(null)}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("ai.history.delete_title")}</DialogTitle>
            <DialogDescription>
              {t("ai.history.delete_body", {
                title: deleting?.title || t("ai.history.untitled"),
              })}
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setDeleting(null)}>
              {t("ai.history.cancel")}
            </Button>
            <Button
              variant="destructive"
              onClick={() => {
                const target = deleting;
                setDeleting(null);
                if (target) void onDelete(target.uuid).catch(() => undefined);
              }}
            >
              {t("ai.history.delete")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
