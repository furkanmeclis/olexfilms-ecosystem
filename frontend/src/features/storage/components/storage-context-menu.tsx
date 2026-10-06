"use client";

import type { ReactNode } from "react";
import {
  Copy,
  Download,
  ExternalLink,
  FolderOpen,
  Info,
  Link2,
  QrCode,
  Pencil,
  Share2,
  Star,
  Trash2,
  FolderInput,
  History,
  MoreHorizontal,
  RotateCcw,
  type LucideIcon,
} from "lucide-react";

import {
  ContextMenu,
  ContextMenuContent,
  ContextMenuItem,
  ContextMenuSeparator,
  ContextMenuTrigger,
} from "@/components/ui/context-menu";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import type { StorageObject } from "@/features/storage/types";
import { useLocale } from "@/providers/locale-provider";

export type StorageAction =
  | "open"
  | "preview"
  | "download"
  | "copy-link"
  | "public"
  | "signed"
  | "qr"
  | "rename"
  | "move"
  | "copy"
  | "share"
  | "star"
  | "details"
  | "versions"
  | "delete"
  | "restore"
  | "purge";

type StorageMenuEntry = {
  action: StorageAction;
  labelKey: string;
  icon?: LucideIcon;
  destructive?: boolean;
  /** Separator before this entry */
  separator?: boolean;
};

/** Actions offered for one object (shared by the context and row menus). */
export function storageMenuEntries(
  item: StorageObject,
  { canWrite, trash }: { canWrite: boolean; trash?: boolean },
): StorageMenuEntry[] {
  if (trash) {
    return canWrite
      ? [
          { action: "restore", labelKey: "storage.restore", icon: RotateCcw },
          {
            action: "purge",
            labelKey: "storage.purge",
            icon: Trash2,
            destructive: true,
          },
        ]
      : [];
  }

  const folder = item.kind === "folder";
  const entries: StorageMenuEntry[] = [
    { action: "open", labelKey: "storage.open", icon: FolderOpen },
  ];
  if (!folder) {
    entries.push(
      { action: "preview", labelKey: "storage.preview", icon: ExternalLink },
      { action: "download", labelKey: "storage.download", icon: Download },
      {
        action: "copy-link",
        labelKey: "storage.copy_link",
        icon: Copy,
        separator: true,
      },
    );
    if (canWrite) {
      entries.push(
        {
          action: "public",
          labelKey: "storage.generate_public",
          icon: Link2,
        },
        {
          action: "signed",
          labelKey: "storage.generate_signed",
          icon: Link2,
        },
        { action: "qr", labelKey: "storage.generate_qr", icon: QrCode },
      );
    }
  }
  if (canWrite) {
    entries.push(
      {
        action: "rename",
        labelKey: "storage.rename",
        icon: Pencil,
        separator: true,
      },
      { action: "move", labelKey: "storage.move", icon: FolderInput },
      { action: "copy", labelKey: "storage.copy", icon: Copy },
      { action: "share", labelKey: "storage.share", icon: Share2 },
    );
  }
  entries.push(
    {
      action: "star",
      labelKey: item.is_starred ? "storage.unstar" : "storage.star",
      icon: Star,
    },
    { action: "details", labelKey: "storage.details", icon: Info },
  );
  if (!folder) {
    entries.push({
      action: "versions",
      labelKey: "storage.versions",
      icon: History,
    });
  }
  if (canWrite) {
    entries.push({
      action: "delete",
      labelKey: "storage.delete",
      icon: Trash2,
      destructive: true,
      separator: true,
    });
  }
  return entries;
}

type StorageMenuProps = {
  object: StorageObject;
  canWrite: boolean;
  trash?: boolean;
  onAction: (action: StorageAction, object: StorageObject) => void;
};

/** Right-click menu around a row / card. */
export function StorageContextMenu({
  object: item,
  canWrite,
  trash,
  children,
  onAction,
}: StorageMenuProps & { children: ReactNode }) {
  const { t } = useLocale();
  const entries = storageMenuEntries(item, { canWrite, trash });

  return (
    <ContextMenu>
      <ContextMenuTrigger asChild>{children}</ContextMenuTrigger>
      <ContextMenuContent className="w-56" data-no-row-click="">
        {entries.map((entry) => {
          const Icon = entry.icon;
          return [
            entry.separator ? (
              <ContextMenuSeparator key={`${entry.action}-sep`} />
            ) : null,
            <ContextMenuItem
              key={entry.action}
              variant={entry.destructive ? "destructive" : undefined}
              onSelect={() => onAction(entry.action, item)}
            >
              {Icon ? <Icon /> : null} {t(entry.labelKey)}
            </ContextMenuItem>,
          ];
        })}
      </ContextMenuContent>
    </ContextMenu>
  );
}

/** Row actions button (same entries as the context menu). */
export function StorageActionsMenu({
  object: item,
  canWrite,
  trash,
  onAction,
}: StorageMenuProps) {
  const { t } = useLocale();
  const entries = storageMenuEntries(item, { canWrite, trash });
  if (!entries.length) return null;

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          type="button"
          variant="ghost"
          size="icon"
          className="size-8"
          aria-label={t("storage.col_actions")}
        >
          <MoreHorizontal className="size-4" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-56" data-no-row-click="">
        {entries.map((entry) => {
          const Icon = entry.icon;
          return [
            entry.separator ? (
              <DropdownMenuSeparator key={`${entry.action}-sep`} />
            ) : null,
            <DropdownMenuItem
              key={entry.action}
              variant={entry.destructive ? "destructive" : undefined}
              onSelect={() => onAction(entry.action, item)}
            >
              {Icon ? <Icon /> : null} {t(entry.labelKey)}
            </DropdownMenuItem>,
          ];
        })}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
