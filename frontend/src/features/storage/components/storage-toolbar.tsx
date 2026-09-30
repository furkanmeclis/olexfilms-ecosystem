"use client";

import {
  Filter,
  FolderPlus,
  LayoutGrid,
  LayoutList,
  Plus,
  Upload,
} from "lucide-react";

import { Button } from "@/components/ui/button";
import { DatePicker } from "@/components/ui/date-picker";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Input } from "@/components/ui/input";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Can } from "@/components/common/can";
import { permissions } from "@/config/permissions";
import type { StorageView } from "@/features/storage/types";
import { useLocale } from "@/providers/locale-provider";

export function StorageToolbar({
  view,
  query,
  sort,
  viewMode,
  kind,
  access,
  modifiedFrom,
  modifiedTo,
  onQueryChange,
  onSortChange,
  onViewModeChange,
  onKindChange,
  onAccessChange,
  onModifiedFromChange,
  onModifiedToChange,
  onNewFolder,
  onUpload,
}: {
  view: StorageView;
  query: string;
  sort: string;
  viewMode: "list" | "grid";
  kind: string;
  access: string;
  modifiedFrom: string;
  modifiedTo: string;
  onQueryChange: (value: string) => void;
  onSortChange: (value: string) => void;
  onViewModeChange: (value: "list" | "grid") => void;
  onKindChange: (value: string) => void;
  onAccessChange: (value: string) => void;
  onModifiedFromChange: (value: string) => void;
  onModifiedToChange: (value: string) => void;
  onNewFolder: () => void;
  onUpload: () => void;
}) {
  const { t } = useLocale();
  const filtered = Boolean(kind || access || modifiedFrom || modifiedTo);

  return (
    <div className="flex flex-wrap items-center gap-2">
      <Input
        value={query}
        onChange={(event) => onQueryChange(event.target.value)}
        placeholder={t("storage.search_placeholder")}
        className="h-9 max-w-sm"
      />
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
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button type="button" size="sm" variant="outline">
              {t("storage.sort")}
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end">
            <DropdownMenuRadioGroup value={sort} onValueChange={onSortChange}>
              <DropdownMenuRadioItem value="name">
                {t("storage.sort_name_asc")}
              </DropdownMenuRadioItem>
              <DropdownMenuRadioItem value="-name">
                {t("storage.sort_name_desc")}
              </DropdownMenuRadioItem>
              <DropdownMenuRadioItem value="size">
                {t("storage.sort_size_asc")}
              </DropdownMenuRadioItem>
              <DropdownMenuRadioItem value="-size">
                {t("storage.sort_size_desc")}
              </DropdownMenuRadioItem>
              <DropdownMenuRadioItem value="-updated_at">
                {t("storage.sort_new")}
              </DropdownMenuRadioItem>
              <DropdownMenuRadioItem value="updated_at">
                {t("storage.sort_old")}
              </DropdownMenuRadioItem>
              <DropdownMenuRadioItem value="type">
                {t("storage.sort_type")}
              </DropdownMenuRadioItem>
            </DropdownMenuRadioGroup>
          </DropdownMenuContent>
        </DropdownMenu>
        <Popover>
          <PopoverTrigger asChild>
            <Button
              type="button"
              size="sm"
              variant={filtered ? "secondary" : "outline"}
            >
              <Filter /> {t("storage.filter")}
            </Button>
          </PopoverTrigger>
          <PopoverContent align="end" className="w-72 space-y-3">
            <Select
              value={kind || "all"}
              onValueChange={(v) => onKindChange(v === "all" ? "" : v)}
            >
              <SelectTrigger>
                <SelectValue placeholder={t("storage.filter_type")} />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">{t("storage.filter_type")}</SelectItem>
                {[
                  "image",
                  "video",
                  "audio",
                  "pdf",
                  "document",
                  "spreadsheet",
                  "archive",
                  "code",
                ].map((item) => (
                  <SelectItem key={item} value={item}>
                    {t(`storage.kind_${item}`)}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <Select
              value={access || "all"}
              onValueChange={(v) => onAccessChange(v === "all" ? "" : v)}
            >
              <SelectTrigger>
                <SelectValue placeholder={t("storage.filter_access")} />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">
                  {t("storage.filter_access")}
                </SelectItem>
                {["private", "public", "shared"].map((item) => (
                  <SelectItem key={item} value={item}>
                    {t(`storage.access_${item}`)}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <DatePicker
              value={modifiedFrom}
              onChange={onModifiedFromChange}
              placeholder={t("storage.filter_from")}
            />
            <DatePicker
              value={modifiedTo}
              onChange={onModifiedToChange}
              placeholder={t("storage.filter_to")}
            />
            {filtered ? (
              <Button
                type="button"
                variant="ghost"
                size="sm"
                onClick={() => {
                  onKindChange("");
                  onAccessChange("");
                  onModifiedFromChange("");
                  onModifiedToChange("");
                }}
              >
                {t("storage.clear_filters")}
              </Button>
            ) : null}
          </PopoverContent>
        </Popover>
        <div className="flex rounded-md border">
          <Button
            type="button"
            size="sm"
            variant={viewMode === "list" ? "secondary" : "ghost"}
            className="rounded-e-none"
            onClick={() => onViewModeChange("list")}
          >
            <LayoutList />
          </Button>
          <Button
            type="button"
            size="sm"
            variant={viewMode === "grid" ? "secondary" : "ghost"}
            className="rounded-s-none"
            onClick={() => onViewModeChange("grid")}
          >
            <LayoutGrid />
          </Button>
        </div>
      </div>
    </div>
  );
}
