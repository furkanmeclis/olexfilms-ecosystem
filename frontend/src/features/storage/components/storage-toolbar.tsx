"use client";

import { FolderPlus, Plus, Upload } from "lucide-react";

import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Can } from "@/components/common/can";
import { permissions } from "@/config/permissions";
import type { StorageView } from "@/features/storage/types";
import { useLocale } from "@/providers/locale-provider";

/**
 * Create actions of the explorer. Search, sort, kind/access/modified
 * filters and the list/grid toggle live in the file DataTable.
 */
export function StorageToolbar({
  view,
  onNewFolder,
  onUpload,
}: {
  view: StorageView;
  onNewFolder: () => void;
  onUpload: () => void;
}) {
  const { t } = useLocale();

  return (
    <div className="flex flex-wrap items-center gap-2">
      <div className="ms-auto flex flex-wrap items-center gap-2">
        <Can permission={permissions.storage.write}>
          {view !== "trash" ? (
            <>
              <Button
                type="button"
                size="sm"
                variant="outline"
                onClick={onNewFolder}
              >
                <FolderPlus /> {t("storage.new_folder")}
              </Button>
              <Button type="button" size="sm" onClick={onUpload}>
                <Upload /> {t("storage.upload")}
              </Button>
              <DropdownMenu>
                <DropdownMenuTrigger asChild>
                  <Button type="button" size="sm" variant="outline">
                    <Plus /> {t("storage.new")}
                  </Button>
                </DropdownMenuTrigger>
                <DropdownMenuContent align="end">
                  <DropdownMenuItem onSelect={onNewFolder}>
                    {t("storage.new_folder")}
                  </DropdownMenuItem>
                  <DropdownMenuItem onSelect={onUpload}>
                    {t("storage.upload")}
                  </DropdownMenuItem>
                </DropdownMenuContent>
              </DropdownMenu>
            </>
          ) : null}
        </Can>
      </div>
    </div>
  );
}
