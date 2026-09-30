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
} from "lucide-react";

import {
  ContextMenu,
  ContextMenuContent,
  ContextMenuItem,
  ContextMenuSeparator,
  ContextMenuTrigger,
} from "@/components/ui/context-menu";
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

export function StorageContextMenu({
  object: item,
  canWrite,
  trash,
  children,
  onAction,
}: {
  object: StorageObject;
  canWrite: boolean;
  trash?: boolean;
  children: ReactNode;
  onAction: (action: StorageAction, object: StorageObject) => void;
}) {
  const { t } = useLocale();
  const folder = item.kind === "folder";

  if (trash) {
    return (
      <ContextMenu>
        <ContextMenuTrigger asChild>{children}</ContextMenuTrigger>
        <ContextMenuContent>
          {canWrite ? (
            <>
              <ContextMenuItem onSelect={() => onAction("restore", item)}>
                {t("storage.restore")}
              </ContextMenuItem>
              <ContextMenuItem
                variant="destructive"
                onSelect={() => onAction("purge", item)}
              >
                {t("storage.purge")}
              </ContextMenuItem>
            </>
          ) : null}
        </ContextMenuContent>
      </ContextMenu>
    );
  }

  return (
    <ContextMenu>
      <ContextMenuTrigger asChild>{children}</ContextMenuTrigger>
      <ContextMenuContent className="w-56">
        <ContextMenuItem onSelect={() => onAction("open", item)}>
          <FolderOpen /> {t("storage.open")}
        </ContextMenuItem>
        {!folder ? (
          <ContextMenuItem onSelect={() => onAction("preview", item)}>
            <ExternalLink /> {t("storage.preview")}
          </ContextMenuItem>
        ) : null}
        {!folder ? (
          <ContextMenuItem onSelect={() => onAction("download", item)}>
            <Download /> {t("storage.download")}
          </ContextMenuItem>
        ) : null}
        {!folder ? (
          <>
            <ContextMenuSeparator />
            <ContextMenuItem onSelect={() => onAction("copy-link", item)}>
              <Copy /> {t("storage.copy_link")}
            </ContextMenuItem>
            {canWrite ? (
              <>
                <ContextMenuItem onSelect={() => onAction("public", item)}>
                  <Link2 /> {t("storage.generate_public")}
                </ContextMenuItem>
                <ContextMenuItem onSelect={() => onAction("signed", item)}>
                  <Link2 /> {t("storage.generate_signed")}
                </ContextMenuItem>
                <ContextMenuItem onSelect={() => onAction("qr", item)}>
                  <QrCode /> {t("storage.generate_qr")}
                </ContextMenuItem>
              </>
            ) : null}
          </>
        ) : null}
        {canWrite ? (
          <>
            <ContextMenuSeparator />
            <ContextMenuItem onSelect={() => onAction("rename", item)}>
              <Pencil /> {t("storage.rename")}
            </ContextMenuItem>
            <ContextMenuItem onSelect={() => onAction("move", item)}>
              <FolderInput /> {t("storage.move")}
            </ContextMenuItem>
            <ContextMenuItem onSelect={() => onAction("copy", item)}>
              <Copy /> {t("storage.copy")}
            </ContextMenuItem>
            <ContextMenuItem onSelect={() => onAction("share", item)}>
              <Share2 /> {t("storage.share")}
            </ContextMenuItem>
          </>
        ) : null}
        <ContextMenuItem onSelect={() => onAction("star", item)}>
          <Star /> {item.is_starred ? t("storage.unstar") : t("storage.star")}
        </ContextMenuItem>
        <ContextMenuItem onSelect={() => onAction("details", item)}>
          <Info /> {t("storage.details")}
        </ContextMenuItem>
        {!folder ? (
          <ContextMenuItem onSelect={() => onAction("versions", item)}>
            <History /> {t("storage.versions")}
          </ContextMenuItem>
        ) : null}
        {canWrite ? (
          <>
            <ContextMenuSeparator />
            <ContextMenuItem
              variant="destructive"
              onSelect={() => onAction("delete", item)}
            >
              <Trash2 /> {t("storage.delete")}
            </ContextMenuItem>
          </>
        ) : null}
      </ContextMenuContent>
    </ContextMenu>
  );
}
