"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Settings2 } from "lucide-react";
import { useState, type FormEvent } from "react";

import { ErrorState } from "@/components/common/error-state";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Permission } from "@/config/permissions";
import {
  useWarehouseAccess,
  WarehouseShell,
} from "@/features/warehouse/components/warehouse-shell";
import { warehouseErrorMessage } from "@/features/warehouse/lib/errors";
import {
  warehouseKeys,
  warehouseService,
  type WarehouseSettings,
} from "@/features/warehouse/services/warehouse.service";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

/** HH:MM (00:00-23:59); empty clears the cutoff. */
export const CUTOFF_PATTERN = /^([01]\d|2[0-3]):[0-5]\d$/;

/**
 * Warehouse > Settings (TEC-769): organization-level warehouse settings.
 * The end-of-day cutoff closes the business day at HH:MM in the
 * organization's time zone (day D = D-1 cutoff .. D cutoff); empty keeps
 * the calendar day. TEC-771 moves this form under the tenant settings
 * navigation later.
 */
export function WarehouseSettingsPage({ slug }: { slug: string }) {
  const { t } = useLocale();
  const access = useWarehouseAccess(slug);
  const settings = useQuery({
    queryKey: warehouseKeys.settings,
    queryFn: () => warehouseService.getSettings(),
    enabled: access.allowed,
  });
  return (
    <WarehouseShell
      slug={slug}
      access={access}
      title={t("warehouse.settings.title")}
      description={t("warehouse.settings.description")}
      icon={<Settings2 className="size-6" />}
    >
      {settings.isError ? (
        <ErrorState
          title={t("common.error_generic")}
          onRetry={() => void settings.refetch()}
        />
      ) : settings.data ? (
        <EodCutoffForm
          key={settings.data.eod_cutoff ?? "none"}
          settings={settings.data}
          canWrite={access.can(Permission.WarehouseWrite)}
        />
      ) : null}
    </WarehouseShell>
  );
}

export function EodCutoffForm({
  settings,
  canWrite,
}: {
  settings: WarehouseSettings;
  canWrite: boolean;
}) {
  const { t } = useLocale();
  const qc = useQueryClient();
  const [value, setValue] = useState(settings.eod_cutoff ?? "");
  const [error, setError] = useState<string | null>(null);
  const save = useMutation({
    mutationFn: (cutoff: string | null) =>
      warehouseService.updateSettings({ eod_cutoff: cutoff }),
    onSuccess: (next) => {
      qc.setQueryData(warehouseKeys.settings, next);
      appToast.success(t("warehouse.settings.saved"));
    },
  });

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    setError(null);
    const v = value.trim();
    if (v && !CUTOFF_PATTERN.test(v)) {
      setError(t("warehouse.settings.invalid_cutoff"));
      return;
    }
    try {
      await save.mutateAsync(v || null);
    } catch (err) {
      setError(warehouseErrorMessage(err, t, t("warehouse.form.error")));
    }
  };

  return (
    <Card data-testid="eod-cutoff-form">
      <CardHeader>
        <CardTitle>{t("warehouse.settings.eod_title")}</CardTitle>
      </CardHeader>
      <CardContent>
        <form onSubmit={submit} className="max-w-md space-y-4">
          <div className="space-y-2">
            <Label htmlFor="eod-cutoff">
              {t("warehouse.settings.eod_cutoff")}
            </Label>
            <Input
              id="eod-cutoff"
              type="time"
              step={60}
              value={value}
              disabled={!canWrite || save.isPending}
              onChange={(e) => setValue(e.target.value)}
              data-testid="eod-cutoff-input"
              dir="ltr"
            />
            <p className="text-muted-foreground text-sm">
              {value
                ? t("warehouse.settings.eod_hint_cutoff", {
                    time: value,
                    timezone: settings.timezone,
                  })
                : t("warehouse.settings.eod_hint_none", {
                    timezone: settings.timezone,
                  })}
            </p>
          </div>
          {error ? (
            <p role="alert" className="text-destructive text-sm">
              {error}
            </p>
          ) : null}
          {canWrite ? (
            <div className="flex gap-2">
              <Button
                type="submit"
                disabled={save.isPending}
                data-testid="eod-cutoff-save"
              >
                {t("warehouse.form.save")}
              </Button>
              {value ? (
                <Button
                  type="button"
                  variant="outline"
                  disabled={save.isPending}
                  onClick={() => setValue("")}
                  data-testid="eod-cutoff-clear"
                >
                  {t("warehouse.settings.clear")}
                </Button>
              ) : null}
            </div>
          ) : null}
        </form>
      </CardContent>
    </Card>
  );
}
