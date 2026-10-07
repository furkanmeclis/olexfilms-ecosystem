"use client";

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
import { Label } from "@/components/ui/label";
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group";
import { Switch } from "@/components/ui/switch";
import { QuotaInput } from "@/features/ai-admin/components/quota-input";
import type {
  AIOrgQuota,
  AIOrgQuotaUpdate,
} from "@/features/ai-admin/services/ai-admin.service";
import { useLocale } from "@/providers/locale-provider";

type Mode = "default" | "custom";

type OrgQuotaDialogProps = {
  row: AIOrgQuota | null;
  /** Platform default monthly quota (0 = unlimited), shown as a hint. */
  defaultQuota?: number;
  isSaving?: boolean;
  onOpenChange: (open: boolean) => void;
  onSubmit: (uuid: string, body: AIOrgQuotaUpdate) => void;
};

/** "Kota düzenle": override (or the platform default) and the AI switch. */
export function OrgQuotaDialog({
  row,
  defaultQuota,
  isSaving,
  onOpenChange,
  onSubmit,
}: OrgQuotaDialogProps) {
  const { t } = useLocale();
  return (
    <Dialog open={Boolean(row)} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        {row ? (
          <OrgQuotaForm
            key={row.organization.uuid}
            row={row}
            defaultQuota={defaultQuota}
            isSaving={isSaving}
            onCancel={() => onOpenChange(false)}
            onSubmit={onSubmit}
          />
        ) : (
          <DialogTitle className="sr-only">
            {t("ai_admin.quota_dialog.title")}
          </DialogTitle>
        )}
      </DialogContent>
    </Dialog>
  );
}

function OrgQuotaForm({
  row,
  defaultQuota,
  isSaving,
  onCancel,
  onSubmit,
}: {
  row: AIOrgQuota;
  defaultQuota?: number;
  isSaving?: boolean;
  onCancel: () => void;
  onSubmit: (uuid: string, body: AIOrgQuotaUpdate) => void;
}) {
  const { t, format } = useLocale();
  const [mode, setMode] = useState<Mode>(
    row.quota_override == null ? "default" : "custom",
  );
  const [quota, setQuota] = useState<number | null>(
    row.quota_override ?? row.quota,
  );
  const [enabled, setEnabled] = useState(row.enabled);
  const missing = mode === "custom" && quota == null;

  return (
    <form
      className="space-y-5"
      noValidate
      onSubmit={(event) => {
        event.preventDefault();
        if (missing) return;
        onSubmit(row.organization.uuid, {
          enabled,
          monthly_token_quota: mode === "default" ? null : quota,
        });
      }}
    >
      <DialogHeader>
        <DialogTitle>{t("ai_admin.quota_dialog.title")}</DialogTitle>
        <DialogDescription>{row.organization.name}</DialogDescription>
      </DialogHeader>

      <RadioGroup
        value={mode}
        onValueChange={(value) => setMode(value as Mode)}
        className="gap-3"
      >
        <div className="flex items-start gap-2">
          <RadioGroupItem id="quota-mode-default" value="default" />
          <Label htmlFor="quota-mode-default" className="flex-col items-start">
            <span>{t("ai_admin.quota_dialog.use_default")}</span>
            <span className="text-muted-foreground text-xs font-normal">
              {defaultQuota == null
                ? t("ai_admin.quota_dialog.use_default_hint")
                : defaultQuota === 0
                  ? t("ai_admin.quota.unlimited")
                  : t("ai_admin.quota.tokens_per_month", {
                      value: format.number(defaultQuota),
                    })}
            </span>
          </Label>
        </div>
        <div className="flex items-start gap-2">
          <RadioGroupItem id="quota-mode-custom" value="custom" />
          <Label htmlFor="quota-mode-custom">
            {t("ai_admin.quota_dialog.custom")}
          </Label>
        </div>
      </RadioGroup>

      {mode === "custom" ? (
        <div className="space-y-2">
          <Label htmlFor="org-quota-input">{t("ai_admin.columns.quota")}</Label>
          <QuotaInput
            id="org-quota-input"
            value={quota}
            onChange={setQuota}
            invalid={missing}
          />
          {missing ? (
            <p className="text-destructive text-sm" role="alert">
              {t("ai_admin.errors.quota_required")}
            </p>
          ) : null}
        </div>
      ) : null}

      <div className="flex items-center justify-between gap-4">
        <Label htmlFor="org-quota-enabled" className="flex-col items-start">
          <span>{t("ai_admin.quota_dialog.enabled")}</span>
          <span className="text-muted-foreground text-xs font-normal">
            {t("ai_admin.quota_dialog.enabled_hint")}
          </span>
        </Label>
        <Switch
          id="org-quota-enabled"
          checked={enabled}
          onCheckedChange={setEnabled}
        />
      </div>

      <DialogFooter>
        <Button type="button" variant="outline" onClick={onCancel}>
          {t("common.cancel")}
        </Button>
        <Button type="submit" disabled={isSaving || missing}>
          {t("ai_admin.actions.save")}
        </Button>
      </DialogFooter>
    </form>
  );
}
