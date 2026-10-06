"use client";

import { ChevronDown, Layers } from "lucide-react";
import { useMemo, useState } from "react";

import type {
  BulkActionDef,
  BulkResource,
  SelectionScope,
} from "@/features/bulk-engine/types";
import { buildBulkTarget } from "@/features/bulk-engine/hooks/use-bulk-selection";
import { useBulkMutation } from "@/features/bulk-engine/hooks/use-bulk-mutation";
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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { useLocale } from "@/providers/locale-provider";
import { useDialogs } from "@/providers/dialog-provider";
import { usePermission } from "@/providers/permission-provider";
import { cn } from "@/lib/utils";

type BulkActionMenuProps = {
  resource: BulkResource;
  actions: BulkActionDef[];
  scope: SelectionScope;
  selectedCount: number;
  onComplete?: () => void;
  /**
   * Choices for parameterized actions, keyed by param key (e.g. the
   * category of `set_category`). A param with options renders a select.
   */
  paramOptions?: Record<string, { value: string; label: string }[]>;
};

export function BulkActionMenu({
  resource,
  actions,
  scope,
  selectedCount,
  onComplete,
  paramOptions,
}: BulkActionMenuProps) {
  const { t } = useLocale();
  const { can } = usePermission();
  const { confirm, confirmDelete } = useDialogs();
  const bulkMutation = useBulkMutation();
  const [pending, setPending] = useState<BulkActionDef | null>(null);
  const [paramValues, setParamValues] = useState<Record<string, string>>({});

  const allowed = useMemo(
    () => actions.filter((action) => can(action.permission)),
    [actions, can],
  );

  if (selectedCount <= 0 || allowed.length === 0) return null;

  const confirmAction = async (action: BulkActionDef) => {
    const description = action.confirm_key
      ? t(action.confirm_key, { count: selectedCount })
      : t("bulk.confirm_generic", { count: selectedCount });
    const confirmed = action.destructive
      ? await confirmDelete({ title: t(action.label_key), description })
      : await confirm({ title: t(action.label_key), description });
    return confirmed;
  };

  const runAction = async (
    action: BulkActionDef,
    params?: Record<string, string>,
  ) => {
    if (action.params?.length && !params) {
      const initial: Record<string, string> = {};
      for (const param of action.params) initial[param.key] = "";
      setParamValues(initial);
      setPending(action);
      return;
    }
    if (action.confirm_key || action.destructive) {
      const ok = await confirmAction(action);
      if (!ok) return;
    }
    const target = buildBulkTarget(scope, params);
    await bulkMutation.mutateAsync({
      resource,
      action: action.id,
      target,
      onComplete,
    });
  };

  const submitParams = async () => {
    if (!pending) return;
    for (const param of pending.params ?? []) {
      if (param.required && !paramValues[param.key]?.trim()) return;
    }
    const action = pending;
    const params = paramValues;
    setPending(null);
    await runAction(action, params);
  };

  const paramsDialog = (
    <Dialog
      open={Boolean(pending)}
      onOpenChange={(open) => {
        if (!open) setPending(null);
      }}
    >
      <DialogContent className="max-w-sm">
        <DialogHeader>
          <DialogTitle>{pending ? t(pending.label_key) : ""}</DialogTitle>
          <DialogDescription>
            {t("bulk.params.description", { count: selectedCount })}
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-3">
          {(pending?.params ?? []).map((param) => {
            const options = paramOptions?.[param.key];
            if (options) {
              return (
                <div key={param.key} className="flex flex-col gap-1.5 text-sm">
                  <span className="font-medium">{t(param.label_key)}</span>
                  <Select
                    value={paramValues[param.key] ?? ""}
                    onValueChange={(value) =>
                      setParamValues((prev) => ({
                        ...prev,
                        [param.key]: value,
                      }))
                    }
                  >
                    <SelectTrigger aria-label={t(param.label_key)}>
                      <SelectValue placeholder={t(param.label_key)} />
                    </SelectTrigger>
                    <SelectContent>
                      {options.map((option) => (
                        <SelectItem key={option.value} value={option.value}>
                          {option.label}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </div>
              );
            }
            return (
              <label key={param.key} className="flex flex-col gap-1.5 text-sm">
                <span className="font-medium">{t(param.label_key)}</span>
                <Input
                  type="number"
                  step="any"
                  value={paramValues[param.key] ?? ""}
                  placeholder={
                    param.kind === "percent"
                      ? t("bulk.params.percent_placeholder")
                      : t("bulk.params.delta_placeholder")
                  }
                  onChange={(event) =>
                    setParamValues((prev) => ({
                      ...prev,
                      [param.key]: event.target.value,
                    }))
                  }
                />
              </label>
            );
          })}
        </div>
        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            onClick={() => setPending(null)}
          >
            {t("common.cancel")}
          </Button>
          <Button type="button" onClick={() => void submitParams()}>
            {t("common.confirm")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );

  if (allowed.length === 1) {
    const action = allowed[0]!;
    const Icon = action.icon;
    return (
      <>
        <Button
          type="button"
          size="sm"
          variant={action.destructive ? "destructive" : "secondary"}
          disabled={bulkMutation.isPending}
          className="h-8 gap-1.5"
          onClick={() => void runAction(action)}
        >
          <Icon className="size-3.5" />
          {t(action.label_key)}
        </Button>
        {paramsDialog}
      </>
    );
  }

  return (
    <>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button
            type="button"
            size="sm"
            variant="secondary"
            disabled={bulkMutation.isPending}
            className="h-8 gap-1.5"
          >
            <Layers className="size-3.5" />
            {t("table.bulk")}
            <ChevronDown className="size-3.5 opacity-70" />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end" className="min-w-44">
          {allowed.map((action) => {
            const Icon = action.icon;
            return (
              <DropdownMenuItem
                key={action.id}
                className={cn(
                  action.destructive &&
                    "text-destructive focus:text-destructive",
                )}
                onClick={() => void runAction(action)}
              >
                <Icon className="size-4" />
                {t(action.label_key)}
              </DropdownMenuItem>
            );
          })}
        </DropdownMenuContent>
      </DropdownMenu>
      {paramsDialog}
    </>
  );
}
