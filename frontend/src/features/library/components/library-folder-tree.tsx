"use client";

import { Folder, FolderOpen, Library, Pencil, Trash2 } from "lucide-react";

import { Button } from "@/components/ui/button";
import type { FolderNode } from "@/features/library/lib/library";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

/** Folder tree; "All documents" clears the folder filter. */
export function LibraryFolderTree({
  tree,
  selected,
  onSelect,
  canManage,
  onRename,
  onDelete,
}: {
  tree: FolderNode[];
  selected: string | null;
  onSelect: (uuid: string | null) => void;
  canManage: boolean;
  onRename: (folder: FolderNode) => void;
  onDelete: (folder: FolderNode) => void;
}) {
  const { t } = useLocale();

  const renderNode = (node: FolderNode) => {
    const active = node.uuid === selected;
    const Icon = active ? FolderOpen : Folder;
    return (
      <li key={node.uuid} role="treeitem" aria-selected={active}>
        <div
          className={cn(
            "group flex items-center gap-1 rounded-md pe-1",
            active ? "bg-muted" : "hover:bg-muted/50",
          )}
          style={{ paddingInlineStart: `${node.depth * 0.75}rem` }}
        >
          <button
            type="button"
            data-testid="library-folder"
            data-uuid={node.uuid}
            className="flex min-w-0 flex-1 items-center gap-2 px-2 py-1.5 text-start text-sm"
            onClick={() => onSelect(node.uuid)}
          >
            <Icon className="text-muted-foreground size-4 shrink-0" />
            <span className={cn("truncate", active && "font-medium")}>
              {node.name}
            </span>
          </button>
          {canManage ? (
            <span className="flex shrink-0 opacity-70 group-hover:opacity-100">
              <Button
                type="button"
                variant="ghost"
                size="icon-xs"
                data-testid="library-folder-rename"
                aria-label={t("library.folders.rename")}
                onClick={() => onRename(node)}
              >
                <Pencil className="size-3.5" />
              </Button>
              <Button
                type="button"
                variant="ghost"
                size="icon-xs"
                data-testid="library-folder-delete"
                aria-label={t("library.folders.delete")}
                className="text-destructive hover:text-destructive"
                onClick={() => onDelete(node)}
              >
                <Trash2 className="size-3.5" />
              </Button>
            </span>
          ) : null}
        </div>
        {node.children.length ? (
          <ul role="group">{node.children.map(renderNode)}</ul>
        ) : null}
      </li>
    );
  };

  return (
    <nav aria-label={t("library.folders.title")} className="space-y-1">
      <button
        type="button"
        data-testid="library-folder-all"
        aria-pressed={selected === null}
        className={cn(
          "flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-start text-sm",
          selected === null ? "bg-muted font-medium" : "hover:bg-muted/50",
        )}
        onClick={() => onSelect(null)}
      >
        <Library className="text-muted-foreground size-4" />
        {t("library.folders.all")}
      </button>
      {tree.length ? (
        <ul role="tree" data-testid="library-folder-tree">
          {tree.map(renderNode)}
        </ul>
      ) : (
        <p className="text-muted-foreground px-2 text-xs">
          {t("library.folders.empty")}
        </p>
      )}
    </nav>
  );
}
