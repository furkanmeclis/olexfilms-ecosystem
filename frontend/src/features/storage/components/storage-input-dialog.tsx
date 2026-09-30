"use client";

import { useCallback, useState } from "react";

import { Button } from "@/components/ui/button";
import {
  AsyncCombobox,
  type ComboboxOption,
} from "@/components/ui/async-combobox";
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { useShareFile } from "@/features/storage/hooks/use-storage-sharing";
import type { StorageObject } from "@/features/storage/types";
import { userFullName } from "@/features/users/lib/user-display";
import { usersService } from "@/features/users/services/users.service";
import { useLocale } from "@/providers/locale-provider";

type InputMode = "folder" | "rename" | "move" | "copy" | "share" | null;

function initialValue(
  mode: InputMode,
  object: StorageObject | null,
  prefix: string,
) {
  if (mode === "rename" && object) return object.name;
  if (mode === "move" || mode === "copy") return prefix;
  return "";
}

function StorageInputForm({
  mode,
  object,
  prefix,
  onOpenChange,
  onSubmit,
}: {
  mode: Exclude<InputMode, null>;
  object: StorageObject | null;
  prefix: string;
  onOpenChange: (open: boolean) => void;
  onSubmit: (value: string, role?: string) => void;
}) {
  const { t } = useLocale();
  const [value, setValue] = useState(() => initialValue(mode, object, prefix));
  const [role, setRole] = useState<"viewer" | "editor" | "owner">("viewer");
  const [optionCache, setOptionCache] = useState<ComboboxOption[]>([]);
  const share = useShareFile();

  const loadUsers = useCallback(async (query: string) => {
    const result = await usersService.list({
      limit: 20,
      offset: 0,
      q: query.trim() || undefined,
      status: "active",
    });
    const options = result.items.map((user): ComboboxOption => ({
      value: user.uuid,
      label: `${userFullName(user)} · ${user.email}`,
    }));
    setOptionCache((prev) => {
      const byValue = new Map(prev.map((opt) => [opt.value, opt]));
      for (const opt of options) byValue.set(opt.value, opt);
      return [...byValue.values()];
    });
    return options;
  }, []);

  const title =
    mode === "folder"
      ? t("storage.new_folder")
      : mode === "rename"
        ? t("storage.rename")
        : mode === "move"
          ? t("storage.move")
          : mode === "copy"
            ? t("storage.copy")
            : t("storage.share");

  const label =
    mode === "folder"
      ? t("storage.folder_name")
      : mode === "rename"
        ? t("storage.file_name")
        : mode === "share"
          ? t("storage.share_user")
          : t("storage.destination");

  return (
    <>
      <DialogHeader>
        <DialogTitle>{title}</DialogTitle>
      </DialogHeader>
      <div className="space-y-3">
        {mode === "share" ? (
          <div className="space-y-1">
            <Label htmlFor="storage-share-user">{label}</Label>
            <AsyncCombobox
              id="storage-share-user"
              value={value}
              onValueChange={setValue}
              loadOptions={loadUsers}
              options={optionCache}
              placeholder={t("users.picker.placeholder")}
              searchPlaceholder={t("users.picker.search")}
              emptyText={t("users.picker.empty")}
            />
          </div>
        ) : (
          <div className="space-y-1">
            <Label htmlFor="storage-input">{label}</Label>
            <Input
              id="storage-input"
              value={value}
              onChange={(event) => setValue(event.target.value)}
            />
          </div>
        )}
        {mode === "share" ? (
          <div className="space-y-1">
            <Label>{t("storage.share_role")}</Label>
            <Select
              value={role}
              onValueChange={(v) => setRole(v as typeof role)}
            >
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="viewer">
                  {t("storage.role_viewer")}
                </SelectItem>
                <SelectItem value="editor">
                  {t("storage.role_editor")}
                </SelectItem>
                <SelectItem value="owner">{t("storage.role_owner")}</SelectItem>
              </SelectContent>
            </Select>
          </div>
        ) : null}
      </div>
      <DialogFooter>
        <Button
          type="button"
          variant="outline"
          onClick={() => onOpenChange(false)}
        >
          {t("storage.cancel")}
        </Button>
        <Button
          type="button"
          disabled={!value.trim() || share.isPending}
          onClick={() => {
            if (mode === "share" && object) {
              share.mutate(
                { key: object.key, user_uuid: value.trim(), role },
                { onSuccess: () => onOpenChange(false) },
              );
              return;
            }
            onSubmit(value.trim(), role);
            onOpenChange(false);
          }}
        >
          {mode === "folder" ? t("storage.create") : t("storage.save")}
        </Button>
      </DialogFooter>
    </>
  );
}

export function StorageInputDialog({
  mode,
  object,
  prefix,
  open,
  onOpenChange,
  onSubmit,
}: {
  mode: InputMode;
  object: StorageObject | null;
  prefix: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onSubmit: (value: string, role?: string) => void;
}) {
  const formKey =
    mode && open ? `${mode}:${object?.key ?? "none"}:${prefix}` : "closed";

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        {mode && open ? (
          <StorageInputForm
            key={formKey}
            mode={mode}
            object={object}
            prefix={prefix}
            onOpenChange={onOpenChange}
            onSubmit={onSubmit}
          />
        ) : null}
      </DialogContent>
    </Dialog>
  );
}

export type { InputMode };
